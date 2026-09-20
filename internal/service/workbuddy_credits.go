package service

import (
	"encoding/json"
	"math"
	"strings"
)

func normalizeWorkBuddyCredits(provider string, raw json.RawMessage) *string {
	if !strings.EqualFold(strings.TrimSpace(provider), "workbuddy") || rawJSONMessageIsEmptyOrNull(raw) {
		return nil
	}
	fields := map[string]json.RawMessage{}
	clean := map[string]json.RawMessage{}
	if len(raw) <= 2048 && json.Unmarshal(raw, &fields) == nil {
		value := fields["credits"]
		var number float64
		if len(value) > 0 && string(value) != "null" && json.Unmarshal(value, &number) == nil && number >= 0 && !math.IsInf(number, 0) && !math.IsNaN(number) {
			clean["credits"] = value
		}
	}
	encoded, _ := json.Marshal(clean)
	value := string(encoded)
	return &value
}
