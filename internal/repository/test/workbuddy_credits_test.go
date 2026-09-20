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

func TestWorkBuddyCreditsIngestAndArchive(t *testing.T) {
	db := openTestDatabase(t)
	event, _, err := service.DecodeRedisUsageMessage(`{"request_id":"wb-test","provider":"workbuddy","model":"hy3","workbuddy_credits":{"credits":0.11793750000000001,"secret":"drop"},"tokens":{}}`, time.Now())
	if err != nil || event.WorkBuddyCredits == nil {
		t.Fatalf("decode: %v", err)
	}
	if _, _, err := repository.InsertUsageEvents(db, []entities.UsageEvent{event}); err != nil {
		t.Fatal(err)
	}
	page, err := repository.ListUsageEventsWithFilter(db, dto.UsageQueryFilter{Page: 1, PageSize: 10}, emptyPricingResolverForTest())
	if err != nil || len(page.Events) != 1 || page.Events[0].WorkBuddyCredits == nil || *page.Events[0].WorkBuddyCredits != `{"credits":0.11793750000000001}` {
		t.Fatalf("billing lost: %v", err)
	}
	cols := entities.UsageEventStorageColumns
	if err := db.Exec("INSERT INTO usage_events_archive (" + cols + ") SELECT " + cols + " FROM usage_events").Error; err != nil {
		t.Fatal(err)
	}
	var archived entities.UsageEventArchive
	if err := db.First(&archived).Error; err != nil || archived.WorkBuddyCredits == nil || *archived.WorkBuddyCredits != *event.WorkBuddyCredits {
		t.Fatalf("archive: %v", err)
	}
	for _, body := range []string{`{"provider":"workbuddy","workbuddy_credits":{}}`, `{"provider":"workbuddy","workbuddy_credits":{"credits":-1}}`, `{"provider":"workbuddy","workbuddy_credits":{"credits":null}}`} {
		got, _, err := service.DecodeRedisUsageMessage(strings.Replace(body, "{", `{"request_id":"wb-invalid",`, 1), time.Now())
		if err != nil || got.WorkBuddyCredits == nil || *got.WorkBuddyCredits != "{}" {
			t.Fatal("invalid credit became a known value")
		}
	}
}
