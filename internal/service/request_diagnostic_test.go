package service

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDiagnosticSectionsNeverExposeBodiesOrCredentials(t *testing.T) {
	const raw = `=== REQUEST DIAGNOSTICS ===
{"diagnostic_version":1,"body_capture":"omitted","status":"incomplete","status_code":200,"terminal_received":false,"completion_reason":"upstream_closed_without_terminal","request_body":"secret-prompt","response_body":"secret-answer","request_parameters":{"model":"test","input":"secret-input"},"request_headers":{"Authorization":"secret-key","Cookie":"secret-cookie","X-Private":"secret-custom","Content-Type":"application/json"},"upstream_request":{"method":"POST","body":"secret-upstream","headers":{"Set-Cookie":"secret-set-cookie"}}}`
	sections := diagnosticSections("diagnostic-test.log", []byte(raw), false)
	encoded, err := json.Marshal(sections)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "secret-") {
		t.Fatalf("sensitive data leaked: %s", encoded)
	}
	if sections[0].Title != "REQUEST DIAGNOSTICS" || !strings.Contains(string(encoded), "upstream_closed_without_terminal") || !strings.Contains(string(encoded), "application/json") {
		t.Fatalf("metadata missing: %s", encoded)
	}
	for _, input := range []struct {
		name      string
		body      []byte
		truncated bool
	}{
		{"raw.log", []byte("=== HEADERS ===\nAuthorization: secret-key\n=== API RESPONSE ===\nsecret-answer"), false},
		{"diagnostic-test.log", []byte(raw), true},
		{"diagnostic-test.log", []byte("not-json-secret"), false},
	} {
		encoded, _ := json.Marshal(diagnosticSections(input.name, input.body, input.truncated))
		if strings.Contains(string(encoded), "secret") {
			t.Fatalf("raw content leaked: %s", encoded)
		}
	}
}

func TestDiagnosticSectionsKeepUpstreamAttemptsRedacted(t *testing.T) {
	const raw = `=== REQUEST DIAGNOSTICS ===
{"diagnostic_version":1,"body_capture":"omitted","upstream_attempts_total":2,"upstream_attempts_omitted":0,"upstream_attempts":[{"attempt":1,"request":{"method":"WEBSOCKET","parameters":{"model":"forwarded-model","reasoning.effort":"high","input":"secret-prompt"},"headers":{"Authorization":"secret-key"},"body":"secret-request"},"response":{"status_code":101,"close_code":1006,"terminal_received":false,"last_event":"response.compaction.compacting","error_kind":"unexpected_EOF","headers":{"Set-Cookie":"secret-cookie"},"body":"secret-answer","error_message":"secret-error"}},{"attempt":2,"request":{"method":"POST","request_bytes":100,"headers":{}},"response":{"status_code":200,"terminal_received":true,"terminal_event":"response.completed","reported_model":"actual-model","response_bytes_observed":50,"last_chunk_ms":2000,"headers":{"Content-Type":"text/event-stream"}}}]} `
	sections := diagnosticSections("diagnostic-test.log", []byte(raw), false)
	encoded, _ := json.Marshal(sections)
	if strings.Contains(string(encoded), "secret-") {
		t.Fatalf("upstream private data leaked: %s", encoded)
	}
	requests, responses := 0, 0
	for _, section := range sections {
		if section.Title == "UPSTREAM REQUEST" {
			requests++
		}
		if section.Title == "UPSTREAM RESPONSE" {
			responses++
		}
	}
	if requests != 2 || responses != 2 {
		t.Fatalf("missing attempts: %s", encoded)
	}
	for _, expected := range []string{"1006", "response.completed", "forwarded-model", "actual-model", "unexpected_EOF", "text/event-stream"} {
		if !strings.Contains(string(encoded), expected) {
			t.Fatalf("missing %s", expected)
		}
	}
}
