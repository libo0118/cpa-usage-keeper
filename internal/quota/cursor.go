package quota

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"
)

type cursorQuotaCaller interface {
	FetchCursorQuota(context.Context, string) ([]byte, error)
}

type cursorQuotaProvider struct{ caller ManagementAPICaller }

type cursorQuotaResult struct {
	Subscription *SubscriptionInfo
	Rows         []QuotaRow
}

func (p cursorQuotaProvider) Check(ctx context.Context, input ProviderInput) (ProviderOutput, error) {
	caller, ok := p.caller.(cursorQuotaCaller)
	if !ok {
		return ProviderOutput{}, fmt.Errorf("Cursor quota endpoint is unavailable")
	}
	body, err := caller.FetchCursorQuota(ctx, input.Identity.Identity)
	if err != nil {
		return ProviderOutput{}, err
	}
	result, err := parseCursorQuota(body)
	if err != nil {
		return ProviderOutput{}, err
	}
	return ProviderOutput{Provider: "cursor", Result: result}, nil
}

func (c upstreamResponseRecordingCaller) FetchCursorQuota(ctx context.Context, authIndex string) ([]byte, error) {
	caller, ok := c.ManagementClient.(cursorQuotaCaller)
	if !ok {
		return nil, fmt.Errorf("Cursor quota endpoint is unavailable")
	}
	return caller.FetchCursorQuota(ctx, authIndex)
}

func resolveCursorSubscription(result any) *SubscriptionInfo {
	if value, ok := result.(cursorQuotaResult); ok {
		return value.Subscription
	}
	return nil
}

func parseCursorQuota(body []byte) (cursorQuotaResult, error) {
	var payload struct {
		Subscription *SubscriptionInfo
		Error        json.RawMessage
		Summary      []struct {
			Key      string
			Value    *float64
			Format   string
			Currency string
		}
		Groups []struct {
			DisplayName string
			Buckets     []struct {
				Window            string
				RemainingFraction *float64
				ResetTime         string
			}
		}
	}
	invalid := func() (cursorQuotaResult, error) {
		return cursorQuotaResult{}, fmt.Errorf("Cursor returned invalid quota data")
	}
	if json.Unmarshal(body, &payload) != nil || (len(payload.Error) > 0 && string(payload.Error) != "null") {
		return invalid()
	}
	amounts := make(map[string]*float64)
	for _, metric := range payload.Summary {
		if metric.Format != "currency" || metric.Currency != "USD" {
			continue
		}
		if _, duplicate := amounts[metric.Key]; duplicate {
			return invalid()
		}
		if metric.Value == nil {
			amounts[metric.Key] = nil
			continue
		}
		cents := *metric.Value * 100
		if cents < 0 || math.IsNaN(cents) || math.IsInf(cents, 0) {
			return invalid()
		}
		amounts[metric.Key] = &cents
	}
	byWindow := make(map[string]QuotaRow)
	for _, group := range payload.Groups {
		for _, bucket := range group.Buckets {
			if _, duplicate := byWindow[bucket.Window]; duplicate {
				return invalid()
			}
			fraction := bucket.RemainingFraction
			if fraction != nil && (*fraction < 0 || *fraction > 1 || math.IsNaN(*fraction) || math.IsInf(*fraction, 0)) {
				return invalid()
			}
			row := QuotaRow{Label: group.DisplayName, RemainingFraction: fraction}
			if reset, err := time.Parse(time.RFC3339, bucket.ResetTime); err == nil {
				row.ResetAt = reset.UTC().Format(time.RFC3339)
			}
			byWindow[bucket.Window] = row
		}
	}
	result := cursorQuotaResult{Subscription: payload.Subscription, Rows: make([]QuotaRow, 0, 4)}
	if result.Subscription != nil {
		result.Subscription.Provider = "cursor"
	}
	for _, window := range []struct{ key, label string }{
		{"plan", "Plan"},
		{"on_demand_individual", "On-demand (individual)"},
		{"on_demand_team", "On-demand (team)"},
		{"credit_grants", "Credit grants"},
	} {
		row := byWindow[window.key]
		row.Key, row.Scope, row.Metric = "cursor."+window.key, "cursor", "usd_cents"
		if row.Label == "" {
			row.Label = window.label
		}
		row.Used = amounts[window.key+"_used"]
		row.Limit = amounts[window.key+"_limit"]
		row.Remaining = amounts[window.key+"_remaining"]
		if row.Used != nil || row.Limit != nil || row.Remaining != nil || row.RemainingFraction != nil {
			result.Rows = append(result.Rows, row)
		}
	}
	if len(result.Rows) == 0 {
		return cursorQuotaResult{}, fmt.Errorf("Cursor quota data is unavailable")
	}
	return result, nil
}
