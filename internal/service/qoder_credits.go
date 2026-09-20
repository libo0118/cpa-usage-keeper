package service

import (
	"encoding/json"
	"math"
	"strings"
)

// nil is historical/uncollected; {} is captured but unavailable.
// Keep validated fields only and preserve their decimal JSON text.
func normalizeQoderCredits(provider string, raw json.RawMessage) *string {
	if !strings.EqualFold(strings.TrimSpace(provider), "qoder") || rawJSONMessageIsEmptyOrNull(raw) {
		return nil
	}
	fields := map[string]json.RawMessage{}
	clean := map[string]json.RawMessage{}
	if len(raw) <= 2048 && json.Unmarshal(raw, &fields) == nil {
		for _, key := range []string{"credits", "original_credits"} {
			value := fields[key]
			var number float64
			if len(value) > 0 && string(value) != "null" && json.Unmarshal(value, &number) == nil && number >= 0 && !math.IsInf(number, 0) && !math.IsNaN(number) {
				clean[key] = value
			}
		}
		if value := fields["billable"]; string(value) == "true" || string(value) == "false" {
			clean["billable"] = value
		}
	}
	encoded, _ := json.Marshal(clean)
	value := string(encoded)
	return &value
}
