package test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"cpa-usage-keeper/internal/cpa/dto/apicall"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/quota"
)

func TestCodexProviderNormalizesCreditBalance(t *testing.T) {
	for _, tc := range []struct {
		name, credits string
		balance       *float64
		unlimited     bool
	}{
		{"positive string", `{"has_credits":true,"unlimited":false,"balance":"12345.6789012000"}`, floatPtr(12345.6789012), false},
		{"numeric", `{"balance":12.5}`, floatPtr(12.5), false},
		{"zero string", `{"has_credits":false,"balance":"0"}`, floatPtr(0), false},
		{"zero numeric", `{"balance":0}`, floatPtr(0), false},
		{"missing credits", `null`, nil, false},
		{"missing balance", `{}`, nil, false},
		{"null balance", `{"balance":null}`, nil, false},
		{"malformed balance", `{"balance":"unknown"}`, nil, false},
		{"NaN balance", `{"balance":"NaN"}`, nil, false},
		{"infinite balance", `{"balance":"Infinity"}`, nil, false},
		{"unlimited", `{"unlimited":true,"balance":null}`, nil, true},
		{"unlimited with balance", `{"unlimited":true,"balance":"0"}`, floatPtr(0), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := fetchCodexCreditBalance(t, tc.credits)
			rows := quota.NormalizeQuotaRows(output)
			wantRows := 2
			if tc.balance != nil || tc.unlimited {
				wantRows++
			}
			if len(rows) != wantRows {
				t.Fatalf("expected %d quota rows, got %d", wantRows, len(rows))
			}
			if wantRows == 2 {
				return
			}
			row := findQuotaRow(t, rows, "credits.balance")
			if row.Scope != "credits" || row.Metric != "credits" || row.Used != nil || row.Limit != nil || row.UsedPercent != nil || row.RemainingFraction != nil || row.Allowed != nil || row.LimitReached != nil || row.Window != nil || row.ResetAt != "" {
				t.Fatalf("balance must be informational, got %#v", row)
			}
			if (row.Remaining == nil) != (tc.balance == nil) || (tc.balance != nil && *row.Remaining != *tc.balance) {
				t.Fatalf("balance did not preserve upstream value: %#v", row.Remaining)
			}
			encoded, err := json.Marshal(row)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]any
			if err := json.Unmarshal(encoded, &fields); err != nil {
				t.Fatal(err)
			}
			if (fields["unlimited"] == true) != tc.unlimited {
				t.Fatalf("expected unlimited=%v, got %s", tc.unlimited, encoded)
			}
		})
	}
}

func fetchCodexCreditBalance(t *testing.T, credits string) quota.ProviderOutput {
	t.Helper()
	body := `{"plan_type":"plus","rate_limit":{"allowed":true,"limit_reached":false,"primary_window":{"used_percent":12,"limit_window_seconds":18000},"secondary_window":{"used_percent":22,"limit_window_seconds":604800}},"rate_limit_reset_credits":{"available_count":3},"credits":` + credits + `}`
	caller := &recordingManagementCaller{responses: []*apicall.Response{quotaAPIResponse(200, body)}}
	output, err := quota.NewCodexProvider(caller, quota.DefaultProviderConfigs().Codex).Check(context.Background(), quota.ProviderInput{Identity: entities.UsageIdentity{Identity: "codex-balance-test"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.requests) != 1 || caller.requests[0].Method != "GET" || caller.requests[0].URL != quota.DefaultProviderConfigs().Codex.URL {
		t.Fatal("balance refresh must only fetch usage")
	}
	usage := output.Result.(quota.CodexResult).Usage
	if usage.RateLimitResetCredits == nil || usage.RateLimitResetCredits.AvailableCount == nil || *usage.RateLimitResetCredits.AvailableCount != 3 {
		t.Fatal("spendable balance must not change reset credits")
	}
	return output
}

func TestCodexCreditBalanceSurvivesHeaderCacheUpdate(t *testing.T) {
	for _, credits := range []string{`{"balance":"0"}`, `{"unlimited":true}`} {
		db := openQuotaTestDatabase(t)
		seedUsageIdentity(t, db, entities.UsageIdentity{Identity: "codex-balance-test", Provider: "codex", Type: "codex", AuthType: entities.UsageIdentityAuthTypeAuthFile})
		service := quota.NewServiceWithRegistry(db, quota.NewProviderRegistry(nil), emptyPricingCatalogForTest())
		defer service.StopRefreshTasks()
		output := fetchCodexCreditBalance(t, credits)
		count := 3
		observedAt := time.Date(2026, 10, 10, 10, 0, 0, 0, time.Local)
		refreshTasks(service)["codex-balance-test"] = &quota.RefreshTaskRecord{
			AuthIndex: "codex-balance-test", Type: "codex", Status: quota.RefreshTaskStatusCompleted,
			Source: quota.RefreshSourceManual, RefreshedAt: observedAt,
			Quota: &quota.CheckResponse{ID: "codex-balance-test", Quota: quota.NormalizeQuotaRows(output), RateLimitResetCreditsAvailableCount: &count},
		}
		if !applyUsageHeaderSnapshot(service, context.Background(), codexUsageHeaderSnapshot("codex-balance-test", observedAt.Add(time.Minute), "4")) {
			t.Fatal("expected newer header snapshot to apply")
		}
		response := refreshTasks(service)["codex-balance-test"].Quota
		findQuotaRow(t, response.Quota, "credits.balance")
		if response.RateLimitResetCreditsAvailableCount == nil || *response.RateLimitResetCreditsAvailableCount != 3 {
			t.Fatal("header update must preserve reset credits separately")
		}
	}
}
