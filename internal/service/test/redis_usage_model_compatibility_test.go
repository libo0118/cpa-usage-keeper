package test

import (
	"encoding/json"
	"testing"
	"time"

	"cpa-usage-keeper/internal/service"
)

func TestDecodeRedisUsageMessageResponseModelCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name, fields, response, upstream string
	}{
		{"custom", `"upstream_response_model":" custom-model "`, "custom-model", "custom-model"},
		{"upstream", `"response_model":" official-model "`, "official-model", "official-model"},
		{"both", `"response_model":"official-model","upstream_response_model":"custom-model"`, "official-model", "custom-model"},
		{"absent", `"upstream_response_model":null`, "", ""},
		{"invalid custom", `"upstream_response_model":{"unexpected":true}`, "", ""},
		{"invalid official", `"response_model":"model\u0000invalid"`, "", ""},
		{"invalid custom with official", `"upstream_response_model":false,"response_model":"official-model"`, "official-model", "official-model"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			message := `{"request_id":"model-compatibility","model":"requested-model","alias":"forwarded-model",` + tc.fields + `}`
			event, raw, err := service.DecodeRedisUsageMessage(message, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if event.ResponseModel != tc.response || event.UpstreamResponseModel != tc.upstream || event.Model != "requested-model" || event.ModelAlias == nil || *event.ModelAlias != "forwarded-model" {
				t.Fatalf("response metadata or request models changed: %+v", event)
			}
			if string(raw) != message || event.StatusCode != nil || event.Stream != nil || event.QoderCredits != nil || event.WorkBuddyCredits != nil {
				t.Fatalf("missing metadata must stay unknown and raw payload unchanged: %+v", event)
			}
		})
	}
}

func TestDecodeRedisUsageMessageCombinesCreditsAndNewMetadata(t *testing.T) {
	for _, provider := range []string{"qoder", "workbuddy"} {
		t.Run(provider, func(t *testing.T) {
			message := `{"request_id":"combined-metadata","provider":"` + provider + `","response_model":"reported-model","stream":false,"fail":{"status_code":429},"qoder_credits":{"credits":0.012300,"original_credits":0.123400,"billable":false},"workbuddy_credits":{"credits":0.045600}}`
			event, _, err := service.DecodeRedisUsageMessage(message, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if event.StatusCode == nil || *event.StatusCode != 429 || event.Stream == nil || *event.Stream || event.UpstreamResponseModel != "reported-model" {
				t.Fatalf("new metadata lost: %+v", event)
			}
			credits, other, want := event.QoderCredits, event.WorkBuddyCredits, "0.012300"
			if provider == "workbuddy" {
				credits, other, want = event.WorkBuddyCredits, event.QoderCredits, "0.045600"
			}
			if credits == nil || other != nil {
				t.Fatalf("credits must stay provider-specific: %+v", event)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal([]byte(*credits), &fields); err != nil || string(fields["credits"]) != want {
				t.Fatalf("upstream credit precision lost: %s, %v", *credits, err)
			}
		})
	}
}
