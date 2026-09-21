package service

import (
	"context"
	"cpa-usage-keeper/internal/cpa"
	"cpa-usage-keeper/internal/entities"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestKeyPolicyPricesUseSavedDecimalRates(t *testing.T) {
	multiplier := 1.2
	settings := []entities.ModelPriceSetting{{ID: 1, Model: "known", PromptPricePer1M: 0.1, CompletionPricePer1M: 3, CacheReadPricePer1M: 0.025, PriceMultiplier: &multiplier}, {ID: 2, Model: "dynamic", PromptPricePer1M: 5}}
	prices, unavailable := keyPolicyPrices(settings, []entities.ModelPriceRule{{ModelPriceSettingID: 2}})
	if len(prices) != 2 || prices[0].Input != "0.120000" || prices[0].Output != "3.600000" || prices[0].CacheRead != "0.030000" || prices[0].CacheWrite != "0.000000" {
		t.Fatalf("prices = %+v", prices)
	}
	if !prices[1].Unavailable || prices[1].Input != "" || len(unavailable) != 1 || unavailable[0] != "dynamic" {
		t.Fatalf("unsupported pricing = %+v / %v", prices[1], unavailable)
	}
	if rate, ok := keyPolicyRate(0.00000001, 1); !ok || rate != "0.000001" {
		t.Fatalf("small positive price became free: %q, %v", rate, ok)
	}
	for _, invalid := range []float64{-1, math.NaN(), math.Inf(1), 1e30} {
		if _, ok := keyPolicyRate(invalid, 1); ok {
			t.Fatalf("accepted invalid rate %v", invalid)
		}
	}
	if empty, _ := keyPolicyPrices(nil, nil); len(empty) != 0 {
		t.Fatal("fabricated missing prices")
	}
}

func TestKeyPolicyCyclesActualWindowsOnly(t *testing.T) {
	now := time.Now().UTC()
	row := entities.QuotaCycle{ID: 2, Provider: "codex", AuthIndex: "a", QuotaKey: "rate_limit.primary_window", WindowSeconds: 604800, WindowStartedAt: now.Add(-time.Hour), ResetAt: now.Add(time.Hour), LastObservedAt: now.Add(-time.Minute)}
	older := row
	older.ID = 1
	stale := row
	stale.AuthIndex = "deleted"
	expired := row
	expired.AuthIndex = "expired"
	expired.ResetAt = now
	secondary := row
	secondary.AuthIndex = "b"
	secondary.ID = 3
	secondary.QuotaKey = "rate_limit.secondary_window"
	cycles := keyPolicyCycles([]entities.QuotaCycle{secondary, row, older, stale, expired}, map[string]bool{"a": true, "b": true, "expired": true}, now)
	if len(cycles) != 3 {
		t.Fatalf("cycles = %+v", cycles)
	}
	if cycles[0].Period != "codex_weekly" || cycles[0].CycleID != "3" || cycles[1].Period != "codex_primary" || cycles[2].Period != "codex_weekly" || cycles[2].CycleID != "2" {
		t.Fatalf("wrong mapping: %+v", cycles)
	}
	if !cycles[2].StartsAt.Equal(row.WindowStartedAt) || !cycles[2].ObservedAt.Equal(row.LastObservedAt) {
		t.Fatal("fabricated window")
	}
	row.WindowSeconds = 18000
	cycles = keyPolicyCycles([]entities.QuotaCycle{row}, map[string]bool{"a": true}, now)
	if len(cycles) != 1 || cycles[0].Period != "codex_primary" {
		t.Fatal("fabricated weekly cycle")
	}
	if len(keyPolicyCycles(nil, nil, now)) != 0 {
		t.Fatal("fabricated missing cycle")
	}
}

func TestKeyPolicyManagementAuthAndSafeLegacyCPA(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer stored-management-secret" {
			t.Error("management auth not forwarded")
		}
		switch r.URL.Path {
		case "/v0/management/key-policies/report":
			if r.Method != http.MethodGet {
				t.Error("wrong report method")
			}
			http.Error(w, "private upstream detail", http.StatusNotFound)
		case "/v0/management/key-policies/sync":
			if r.Method != http.MethodPut {
				t.Error("wrong sync method")
			}
			var body map[string]any
			if json.NewDecoder(r.Body).Decode(&body) != nil || body["prices"] == nil {
				t.Error("invalid sync JSON")
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
		}
	}))
	defer server.Close()
	client := cpa.NewClient(server.URL, "stored-management-secret", time.Second, false)
	s := NewKeyPolicyService(nil, client)
	result, err := s.GetKeyPolicies(context.Background())
	if err != ErrKeyPoliciesUnavailable || len(result.Report) != 0 {
		t.Fatalf("legacy failure not sanitized: %+v, %v", result, err)
	}
	if status, _, err := s.syncOnce(context.Background()); status != 404 || err == nil {
		t.Fatalf("legacy sync should stop before DB: %v / %v", status, err)
	}
	if code, err := client.SyncKeyPolicies(context.Background(), map[string]any{"prices": []any{}}); err != nil || code != 204 {
		t.Fatalf("sync failed %d %v", code, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Run(ctx); err != nil {
		t.Fatal(err)
	}
}
