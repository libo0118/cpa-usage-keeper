package quota

import "testing"

func TestCursorQuotaNormalization(t *testing.T) {
	result, err := parseCursorQuota([]byte(`{"subscription":{"plan":"Pro"},"summary":[{"key":"plan_used","value":12.34,"format":"currency","currency":"USD"},{"key":"plan_remaining","value":0,"format":"currency","currency":"USD"}],"groups":[{"displayName":"Plan","buckets":[{"window":"plan","remainingFraction":0,"resetTime":"2026-10-01T00:00:00Z"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	row := result.Rows[0]
	if len(result.Rows) != 1 || row.Used == nil || *row.Used != 1234 || row.Remaining == nil || *row.Remaining != 0 || row.Limit != nil || row.RemainingFraction == nil || *row.RemainingFraction != 0 || row.ResetAt != "2026-10-01T00:00:00Z" {
		t.Fatalf("incorrect amount, zero, or cycle: %#v", row)
	}
	output := ProviderOutput{Provider: "cursor", Result: result}
	if NormalizeSubscription(output).Plan != "Pro" || len(NormalizeQuotaRows(output)) != 1 {
		t.Fatal("Cursor normalization is not registered")
	}
	if _, ok := NewDefaultProviderRegistry(nil, ProviderConfigs{}).Provider("cursor"); !ok {
		t.Fatal("Cursor provider is not registered")
	}
	unknown, err := parseCursorQuota([]byte(`{"summary":[{"key":"plan_used","value":1,"format":"currency","currency":"USD"}]}`))
	if err != nil || unknown.Rows[0].RemainingFraction != nil || unknown.Rows[0].Remaining != nil {
		t.Fatal("unknown quota was invented")
	}
	for _, raw := range []string{`{}`, `{"error":"failed"}`, `{"groups":[{"buckets":[{"window":"plan","remainingFraction":2}]}]}`, `{"summary":[{"key":"plan_used","value":-1,"format":"currency","currency":"USD"}]}`} {
		if _, err := parseCursorQuota([]byte(raw)); err == nil {
			t.Fatalf("invalid quota accepted: %s", raw)
		}
	}
}
