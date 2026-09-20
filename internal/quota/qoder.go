package quota

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type qoderCreditsCaller interface {
	FetchQoderCredits(context.Context, string) ([]byte, error)
}

type qoderProvider struct{ caller ManagementAPICaller }

type qoderCreditsAccount struct {
	AuthIndex string `json:"auth_index"`
	Error     string `json:"error"`
	Plan      string `json:"plan"`
	Credits   *struct {
		Packages []struct {
			Kind      string   `json:"kind"`
			Name      string   `json:"name"`
			Remain    *float64 `json:"remain"`
			Used      *float64 `json:"used"`
			Size      *float64 `json:"size"`
			SizeKnown bool     `json:"size_known"`
			Available bool     `json:"available"`
			CycleEnd  string   `json:"cycle_end"`
		} `json:"packages"`
	} `json:"credits"`
}

func (p qoderProvider) Check(ctx context.Context, input ProviderInput) (ProviderOutput, error) {
	caller, ok := p.caller.(qoderCreditsCaller)
	if !ok {
		return ProviderOutput{}, fmt.Errorf("Qoder credits endpoint is unavailable")
	}
	body, err := caller.FetchQoderCredits(ctx, input.Identity.Identity)
	if err != nil {
		return ProviderOutput{}, err
	}
	var payload struct {
		Accounts []qoderCreditsAccount `json:"accounts"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ProviderOutput{}, err
	}
	for _, account := range payload.Accounts {
		if account.AuthIndex != input.Identity.Identity {
			continue
		}
		if account.Error != "" {
			return ProviderOutput{}, fmt.Errorf("Qoder credits: %s", account.Error)
		}
		if account.Credits == nil || len(account.Credits.Packages) == 0 {
			return ProviderOutput{}, fmt.Errorf("Qoder credits packages are missing")
		}
		for _, pack := range account.Credits.Packages {
			if pack.Remain == nil || pack.Used == nil || (pack.SizeKnown && (pack.Size == nil || *pack.Size < 0)) {
				return ProviderOutput{}, fmt.Errorf("Qoder credits package values are missing or invalid")
			}
		}
		return ProviderOutput{Provider: "qoder", Result: account}, nil
	}
	return ProviderOutput{}, fmt.Errorf("Qoder credits account was not returned")
}

func (c upstreamResponseRecordingCaller) FetchQoderCredits(ctx context.Context, authIndex string) ([]byte, error) {
	caller, ok := c.ManagementClient.(qoderCreditsCaller)
	if !ok {
		return nil, fmt.Errorf("Qoder credits endpoint is unavailable")
	}
	return caller.FetchQoderCredits(ctx, authIndex)
}

func normalizeQoderQuotaRows(account qoderCreditsAccount) []QuotaRow {
	rows := make([]QuotaRow, 0)
	if account.Credits == nil {
		return rows
	}
	for index, pack := range account.Credits.Packages {
		available := pack.Available
		if expiry, err := time.Parse(time.RFC3339, pack.CycleEnd); err == nil && !expiry.After(time.Now()) {
			available = false
		}
		row := QuotaRow{Key: fmt.Sprintf("qoder.%s.%d", pack.Kind, index), Label: pack.Name, Scope: "credits", Metric: "credits", Used: pack.Used, Remaining: pack.Remain, Allowed: boolPtr(available), ExpiresAt: pack.CycleEnd}
		if pack.SizeKnown {
			row.Limit = pack.Size
		}
		rows = append(rows, row)
	}
	return rows
}
