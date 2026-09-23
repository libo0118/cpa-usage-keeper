package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/repository"
	"cpa-usage-keeper/internal/repository/dto"
)

func TestInboxResolvesOnlyUnambiguousMissingAuthType(t *testing.T) {
	for _, tc := range []struct {
		name, input, provider, want string
		types                       []entities.UsageIdentityAuthType
		deleted                     bool
	}{
		{"oauth", "unknown", "workbuddy", "oauth", []entities.UsageIdentityAuthType{1}, false},
		{"blank", "", "workbuddy", "oauth", []entities.UsageIdentityAuthType{1}, false},
		{"api key", "unknown", "workbuddy", "apikey", []entities.UsageIdentityAuthType{2}, false},
		{"ambiguous", "unknown", "workbuddy", "unknown", []entities.UsageIdentityAuthType{1, 2}, false},
		{"deleted", "unknown", "workbuddy", "unknown", []entities.UsageIdentityAuthType{1}, true},
		{"absent", "unknown", "workbuddy", "unknown", nil, false},
		{"wrong provider", "unknown", "qoder", "unknown", []entities.UsageIdentityAuthType{1}, false},
		{"explicit wins", "apikey", "workbuddy", "apikey", []entities.UsageIdentityAuthType{1}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openSyncTestDatabase(t)
			for _, kind := range tc.types {
				identity := entities.UsageIdentity{Identity: "target", AuthType: kind, Type: "workbuddy", Provider: "workbuddy", IsDeleted: tc.deleted}
				if err := db.Create(&identity).Error; err != nil {
					t.Fatal(err)
				}
			}
			raw, _ := json.Marshal(map[string]any{"request_id": "auth-kind-probe", "auth_type": tc.input, "auth_index": "target", "provider": tc.provider, "executor_type": "executorAdapter", "model": "test-model", "tokens": map[string]int{"input_tokens": 7, "output_tokens": 3, "total_tokens": 10}})
			if _, err := repository.InsertRedisUsageInboxMessages(db, []dto.RedisInboxInsert{{Source: redisUsageInboxTestSource, RawMessage: string(raw), PoppedAt: time.Now()}}); err != nil {
				t.Fatal(err)
			}
			svc := NewSyncServiceWithOptions(db, SyncServiceOptions{BaseURL: "https://cpa.example.com"})
			cache := &recordingRecentUsageAppender{allowed: true}
			svc.recentUsage = cache
			if _, err := svc.ProcessRedisUsageInbox(context.Background()); err != nil {
				t.Fatal(err)
			}
			event := loadUsageEventByKey(t, db, "auth-kind-probe")
			if event.AuthType != tc.want {
				t.Fatalf("auth_type=%q want %q", event.AuthType, tc.want)
			}
			if len(cache.events) != 1 || cache.events[0].AuthType != tc.want {
				t.Fatal("event cache attribution differs from storage")
			}
			if event.TotalTokens != 10 {
				t.Fatal("token totals changed")
			}
			if tc.want == "oauth" {
				var identity entities.UsageIdentity
				if err := db.Where("identity = ? AND auth_type = ?", "target", 1).First(&identity).Error; err != nil {
					t.Fatal(err)
				}
				if identity.TotalRequests != 1 || identity.TotalTokens != 10 {
					t.Fatalf("credential totals not aligned: %d requests / %d tokens", identity.TotalRequests, identity.TotalTokens)
				}
			}
		})
	}
}
