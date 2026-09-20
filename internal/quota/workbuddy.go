package quota

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type workbuddyCreditsCaller interface {
	FetchWorkBuddyCredits(context.Context, string) ([]byte, error)
}
type workbuddyProvider struct{ caller ManagementAPICaller }
type workbuddyCreditsAccount struct {
	AuthIndex string `json:"auth_index"`
	Error     string `json:"error"`
	Region    string `json:"region"`
	Plan      string `json:"plan"`
	Credits   *struct {
		TotalRemain *float64 `json:"total_remain"`
		TotalUsed   *float64 `json:"total_used"`
		TotalSize   *float64 `json:"total_size"`
		Packages    []struct {
			Name     string   `json:"name"`
			Remain   *float64 `json:"remain"`
			Used     *float64 `json:"used"`
			Size     *float64 `json:"size"`
			CycleEnd string   `json:"cycle_end"`
		} `json:"packages"`
	} `json:"credits"`
}

func (p workbuddyProvider) Check(ctx context.Context, input ProviderInput) (ProviderOutput, error) {
	caller, ok := p.caller.(workbuddyCreditsCaller)
	if !ok {
		return ProviderOutput{}, fmt.Errorf("WorkBuddy credits endpoint is unavailable")
	}
	body, err := caller.FetchWorkBuddyCredits(ctx, input.Identity.Identity)
	if err != nil {
		return ProviderOutput{}, err
	}
	var payload struct {
		Accounts []workbuddyCreditsAccount `json:"accounts"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ProviderOutput{}, err
	}
	for _, account := range payload.Accounts {
		if account.AuthIndex != input.Identity.Identity {
			continue
		}
		if account.Error != "" {
			return ProviderOutput{}, fmt.Errorf("WorkBuddy credits: %s", account.Error)
		}
		if account.Credits == nil {
			return ProviderOutput{}, fmt.Errorf("WorkBuddy credits are missing")
		}
		valid := func(v *float64) bool { return v != nil && *v >= 0 }
		cr := account.Credits
		if !valid(cr.TotalRemain) || !valid(cr.TotalUsed) || !valid(cr.TotalSize) {
			return ProviderOutput{}, fmt.Errorf("WorkBuddy totals are missing or invalid")
		}
		for _, pack := range cr.Packages {
			if !valid(pack.Remain) || !valid(pack.Used) || !valid(pack.Size) {
				return ProviderOutput{}, fmt.Errorf("WorkBuddy package values are missing or invalid")
			}
		}
		return ProviderOutput{Provider: "workbuddy", Result: account}, nil
	}
	return ProviderOutput{}, fmt.Errorf("WorkBuddy credits account was not returned")
}

func (c upstreamResponseRecordingCaller) FetchWorkBuddyCredits(ctx context.Context, authIndex string) ([]byte, error) {
	caller, ok := c.ManagementClient.(workbuddyCreditsCaller)
	if !ok {
		return nil, fmt.Errorf("WorkBuddy credits endpoint is unavailable")
	}
	return caller.FetchWorkBuddyCredits(ctx, authIndex)
}

func normalizeWorkBuddyQuotaRows(account workbuddyCreditsAccount) []QuotaRow {
	if account.Credits == nil {
		return nil
	}
	cr := account.Credits
	rows := []QuotaRow{{Key: "workbuddy.total", Label: "Total Credits", Scope: "credits", Metric: "credits", Used: cr.TotalUsed, Remaining: cr.TotalRemain, Limit: cr.TotalSize}}
	for i, pack := range cr.Packages {
		expiry, err := time.Parse(time.RFC3339, pack.CycleEnd)
		if err != nil && account.Region == "cn" {
			expiry, err = time.ParseInLocation("2006-01-02 15:04:05", pack.CycleEnd, time.FixedZone("CST", 8*60*60))
		}
		expiresAt := ""
		available := pack.Remain != nil && *pack.Remain > 0
		if err == nil {
			expiresAt = expiry.Format(time.RFC3339)
			available = available && expiry.After(time.Now())
		}
		rows = append(rows, QuotaRow{Key: fmt.Sprintf("workbuddy.package.%d", i), Label: pack.Name, Scope: "credits", Metric: "credits", Used: pack.Used, Remaining: pack.Remain, Limit: pack.Size, Allowed: boolPtr(available), ExpiresAt: expiresAt})
	}
	return rows
}
