package test

import (
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/repository"
	"cpa-usage-keeper/internal/repository/dto"
	"cpa-usage-keeper/internal/service"
	"strings"
	"testing"
	"time"
)

func TestQoderCreditsIngestPersistQuery(t *testing.T) {
	db := openTestDatabase(t)
	raw := `{"request_id":"qoder-credits-test","provider":"qoder","model":"lite","qoder_credits":{"credits":0.11793750000000001,"original_credits":0.11793750000000001,"billable":false},"tokens":{}}`
	event, _, err := service.DecodeRedisUsageMessage(raw, time.Now())
	if err != nil || event.QoderCredits == nil {
		t.Fatalf("decode: %v", err)
	}
	if _, _, err := repository.InsertUsageEvents(db, []entities.UsageEvent{event}); err != nil {
		t.Fatal(err)
	}
	page, err := repository.ListUsageEventsWithFilter(db, dto.UsageQueryFilter{Page: 1, PageSize: 10}, emptyPricingResolverForTest())
	if err != nil || len(page.Events) != 1 || page.Events[0].QoderCredits == nil {
		t.Fatalf("query: %v", err)
	}
	if !strings.Contains(*page.Events[0].QoderCredits, "0.11793750000000001") || !strings.Contains(*page.Events[0].QoderCredits, `"billable":false`) {
		t.Fatal("precision or billing lost")
	}
	for _, tc := range []struct {
		payload string
		want    string
	}{
		{`{}`, ""},
		{`{"provider":"codex","qoder_credits":{"billable":false}}`, ""},
		{`{"provider":"qoder","qoder_credits":{}}`, "{}"},
		{`{"provider":"qoder","qoder_credits":{"credits":-1,"billable":"false","secret":"discard"}}`, "{}"},
	} {
		body := `{"request_id":"qoder-missing",` + tc.payload[1:]
		body = strings.Replace(body, ",}", "}", 1)
		got, _, err := service.DecodeRedisUsageMessage(body, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if tc.want == "" {
			if got.QoderCredits != nil {
				t.Fatal("legacy/non-qoder should be nil")
			}
		} else if got.QoderCredits == nil || *got.QoderCredits != tc.want {
			t.Fatalf("unexpected billing: %v", got.QoderCredits)
		}
	}
}
