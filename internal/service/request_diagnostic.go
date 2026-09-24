package service

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"

	"cpa-usage-keeper/internal/entities"
)

const hiddenRequestBodyNotice = "Request and response bodies are hidden. Only redacted transport metadata is available."

func missingRequestDiagnostic(event *entities.UsageEvent) RequestLogResponse {
	content, _ := json.MarshalIndent(map[string]any{
		"evidence_source": "usage_record", "capture_status": "unavailable", "stream_status": "unknown",
		"note":       "Transport evidence was not captured or has been removed. Usage success does not prove stream completion.",
		"request_id": event.RequestID, "timestamp": event.Timestamp.Format(time.RFC3339Nano),
		"model": event.Model, "endpoint": event.Endpoint, "usage_failed": event.Failed, "latency_ms": event.LatencyMS,
	}, "", "  ")
	return RequestLogResponse{
		EventID: event.ID, RequestID: event.RequestID, Filename: "request-diagnostic-" + event.RequestID + ".log",
		Available: true, Previewable: true, Downloadable: true,
		Sections: []RequestLogSection{{Title: "CAPTURE UNAVAILABLE", Content: string(content)}, {Title: "PRIVACY", Content: hiddenRequestBodyNotice}},
	}
}

// diagnosticSections accepts CPA's bounded metadata artifact, never a raw log.
// Legacy logs stay hidden: parsing arbitrary prompt text as headers is unsafe.
func diagnosticSections(filename string, raw []byte, truncated bool) []RequestLogSection {
	sections := []RequestLogSection{{Title: "PRIVACY", Content: hiddenRequestBodyNotice}}
	const marker = "=== REQUEST DIAGNOSTICS ===\n"
	if truncated || !strings.HasPrefix(filename, "diagnostic-") || !strings.HasPrefix(string(raw), marker) {
		return append(sections, RequestLogSection{Title: "REQUEST DIAGNOSTICS", Content: "Metadata unavailable for this log. New requests require a CPA version with redacted diagnostics enabled."})
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw[len(marker):], &fields) != nil || string(fields["diagnostic_version"]) != "1" || string(fields["body_capture"]) != `"omitted"` {
		return append(sections, RequestLogSection{Title: "REQUEST DIAGNOSTICS", Content: "Unsupported diagnostic format; raw content hidden."})
	}
	sections = nil
	summary := make(map[string]json.RawMessage)
	for _, key := range strings.Fields(`request_id connection_request_id trace_id timestamp_unix_ms codex_session_id requested_model
		request_protocol request_kind status status_code completion_reason total_duration_ms request_bytes
		last_event terminal_event terminal_received generated_terminal_event data_frames response_started
		downstream_bytes_attempted downstream_bytes_written downstream_flush_error_observable
		write_error_kind cancel_error_kind request_context_error_kind first_downstream_write_ms last_downstream_write_ms
		write_byte_count_kind upstream_close_code upstream_attempts_total upstream_attempts_omitted`) {
		if value, ok := fields[key]; ok && diagnosticScalar(value) {
			summary[key] = value
		}
	}
	appendJSON := func(title string, value any) {
		if content, err := json.MarshalIndent(value, "", "  "); err == nil {
			sections = append(sections, RequestLogSection{Title: title, Content: string(content)})
		}
	}
	appendJSON("REQUEST DIAGNOSTICS", summary)
	if parameters := diagnosticParameters(fields["request_parameters"]); parameters != nil {
		appendJSON("REQUEST PARAMETERS", parameters)
	}
	for _, item := range []struct{ key, title string }{
		{"request_headers", "REQUEST HEADERS"}, {"response_headers", "RESPONSE HEADERS"},
	} {
		appendJSON(item.title, diagnosticHeaders(fields[item.key]))
	}
	appendUpstream := func(title string, raw json.RawMessage, attempt json.RawMessage) {
		var upstream map[string]json.RawMessage
		if json.Unmarshal(raw, &upstream) != nil || upstream == nil {
			return
		}
		metadata := map[string]any{"headers": diagnosticHeaders(upstream["headers"])}
		if diagnosticScalar(attempt) {
			metadata["attempt"] = attempt
		}
		for _, key := range strings.Fields(`method provider authority scheme status_code body_capture timestamp_unix_ms
			request_bytes headers_received headers_received_ms handshake_observed response_chunks response_bytes_observed
			byte_count_kind first_chunk_ms last_chunk_ms last_event terminal_event terminal_received reported_model
			error_kind error_stage error_observed_ms close_code error_status_code error_code error_type
			response_status incomplete_reason observation_duration_ms`) {
			if value, ok := upstream[key]; ok && diagnosticScalar(value) {
				metadata[key] = value
			}
		}
		if parameters := diagnosticParameters(upstream["parameters"]); parameters != nil {
			metadata["parameters"] = parameters
		}
		appendJSON(title, metadata)
	}
	var attempts []map[string]json.RawMessage
	if json.Unmarshal(fields["upstream_attempts"], &attempts) == nil && len(attempts) > 0 {
		if len(attempts) > 16 {
			attempts = attempts[len(attempts)-16:]
		}
		for _, attempt := range attempts {
			appendUpstream("UPSTREAM REQUEST", attempt["request"], attempt["attempt"])
			appendUpstream("UPSTREAM RESPONSE", attempt["response"], attempt["attempt"])
		}
	} else {
		appendUpstream("UPSTREAM REQUEST", fields["upstream_request"], nil)
		appendUpstream("UPSTREAM RESPONSE", fields["upstream_response"], nil)
	}
	return append(sections, RequestLogSection{Title: "PRIVACY", Content: hiddenRequestBodyNotice})
}

func diagnosticParameters(raw json.RawMessage) map[string]json.RawMessage {
	var parameters map[string]json.RawMessage
	if json.Unmarshal(raw, &parameters) != nil || parameters == nil {
		return nil
	}
	allowed := make(map[string]json.RawMessage)
	for _, key := range strings.Fields("model stream reasoning.effort service_tier max_output_tokens temperature top_p") {
		if value, ok := parameters[key]; ok && diagnosticScalar(value) {
			allowed[key] = value
		}
	}
	return allowed
}

func diagnosticScalar(value json.RawMessage) bool {
	value = bytes.TrimSpace(value)
	return len(value) > 0 && len(value) <= 4096 && value[0] != '{' && value[0] != '['
}

func diagnosticHeaders(raw json.RawMessage) map[string]string {
	var headers map[string]string
	if json.Unmarshal(raw, &headers) != nil {
		return map[string]string{}
	}
	for key := range headers {
		switch strings.ToLower(key) {
		case "content-type", "content-length", "content-encoding", "accept", "accept-encoding",
			"cache-control", "connection", "transfer-encoding", "date", "server", "retry-after",
			"user-agent", "x-request-id", "request-id", "x-cpa-trace-id", "session-id", "session_id",
			"x-session-id", "openai-version", "openai-processing-ms":
		default:
			headers[key] = "[REDACTED]"
		}
	}
	return headers
}
