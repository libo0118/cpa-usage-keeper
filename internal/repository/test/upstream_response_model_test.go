package test

import (
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/repository"
	"cpa-usage-keeper/internal/repository/dto"
	"cpa-usage-keeper/internal/service"
	"testing"
	"time"
)

func TestUpstreamModelIngestQueryAndArchive(t *testing.T) {
	db := openTestDatabase(t)
	event, _, err := service.DecodeRedisUsageMessage(`{"request_id":"model-report-test","provider":"codex","model":"sent","alias":"client-alias","upstream_response_model":"reported","tokens":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := repository.InsertUsageEvents(db, []entities.UsageEvent{event}); err != nil {
		t.Fatal(err)
	}
	page, err := repository.ListUsageEventsWithFilter(db, dto.UsageQueryFilter{Page: 1, PageSize: 10}, emptyPricingResolverForTest())
	if err != nil || len(page.Events) != 1 {
		t.Fatalf("query failed: %v", err)
	}
	row := page.Events[0]
	if row.Model != "sent" || row.ModelAlias != "client-alias" || row.UpstreamResponseModel != "reported" {
		t.Fatalf("model provenance lost: %+v", row)
	}
	columns := entities.UsageEventStorageColumns
	if err := db.Exec("INSERT INTO usage_events_archive (" + columns + ") SELECT " + columns + " FROM usage_events").Error; err != nil {
		t.Fatal(err)
	}
	var archived entities.UsageEventArchive
	if err := db.First(&archived).Error; err != nil || archived.UpstreamResponseModel != "reported" {
		t.Fatalf("archive metadata lost: %v", err)
	}
	for _, metadata := range []string{"", `,"upstream_response_model":null`, `,"upstream_response_model":{"invalid":true}`, `,"upstream_response_model":"bad\nname"`} {
		old, _, err := service.DecodeRedisUsageMessage(`{"request_id":"old-or-invalid","model":"sent","tokens":{"total_tokens":3}`+metadata+`}`, time.Now())
		if err != nil || old.UpstreamResponseModel != "" || old.TotalTokens != 3 {
			t.Fatalf("invalid metadata changed usage: %+v, %v", old, err)
		}
	}
}
