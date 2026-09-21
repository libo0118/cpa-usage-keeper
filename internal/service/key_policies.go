package service

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"strconv"
	"sync"
	"time"

	"cpa-usage-keeper/internal/cpa"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/repository"
	"cpa-usage-keeper/internal/timeutil"
	"gorm.io/gorm"
)

var ErrKeyPoliciesUnavailable = errors.New("CPA key budgets unavailable")

type KeyPolicySyncStatus struct {
	LastAttempt       time.Time `json:"last_attempt"`
	LastSuccess       time.Time `json:"last_success"`
	Status            string    `json:"status"`
	UnavailableModels []string  `json:"unavailable_models"`
}

type KeyPolicyReport struct {
	Report json.RawMessage     `json:"report"`
	Sync   KeyPolicySyncStatus `json:"sync"`
}

type KeyPolicyService struct {
	db     *gorm.DB
	client *cpa.Client
	mu     sync.RWMutex
	status KeyPolicySyncStatus
}

func NewKeyPolicyService(db *gorm.DB, client *cpa.Client) *KeyPolicyService {
	return &KeyPolicyService{db: db, client: client, status: KeyPolicySyncStatus{Status: "pending", UnavailableModels: []string{}}}
}

func (s *KeyPolicyService) GetKeyPolicies(ctx context.Context) (KeyPolicyReport, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	report, _, err := s.client.FetchKeyPolicies(ctx)
	s.mu.RLock()
	status := s.status
	s.mu.RUnlock()
	result := KeyPolicyReport{Report: report, Sync: status}
	if err != nil {
		return result, ErrKeyPoliciesUnavailable
	}
	return result, nil
}

type keyPolicyPrice struct {
	Model       string `json:"model"`
	Input       string `json:"input_per_million,omitempty"`
	Output      string `json:"output_per_million,omitempty"`
	CacheRead   string `json:"cache_read_per_million,omitempty"`
	CacheWrite  string `json:"cache_write_per_million,omitempty"`
	Unavailable bool   `json:"unavailable,omitempty"`
}

type keyPolicyCycle struct {
	ResourceID string    `json:"resource_id"`
	Period     string    `json:"period"`
	CycleID    string    `json:"cycle_id"`
	StartsAt   time.Time `json:"starts_at"`
	ResetAt    time.Time `json:"reset_at"`
	ObservedAt time.Time `json:"observed_at"`
}

// Saved prices are floats in Keeper's existing schema. Convert each operand to
// its decimal representation before multiplication; the CPA ledger never receives floats.
func keyPolicyRate(price, multiplier float64) (string, bool) {
	a, ok := new(big.Rat).SetString(strconv.FormatFloat(price, 'f', -1, 64))
	if !ok || a.Sign() < 0 {
		return "", false
	}
	b, ok := new(big.Rat).SetString(strconv.FormatFloat(multiplier, 'f', -1, 64))
	if !ok || b.Sign() < 0 {
		return "", false
	}
	a.Mul(a, b)
	// Round upward at six decimal places so a positive saved rate never becomes free.
	a.Mul(a, big.NewRat(1000000, 1))
	units, rem := new(big.Int), new(big.Int)
	units.QuoRem(a.Num(), a.Denom(), rem)
	if rem.Sign() > 0 {
		units.Add(units, big.NewInt(1))
	}
	if !units.IsInt64() {
		return "", false
	}
	return new(big.Rat).SetFrac(units, big.NewInt(1000000)).FloatString(6), true
}

func keyPolicyPrices(settings []entities.ModelPriceSetting, rules []entities.ModelPriceRule) ([]keyPolicyPrice, []string) {
	blocked := map[int64]bool{}
	for _, rule := range rules {
		blocked[rule.ModelPriceSettingID] = true
	}
	prices := make([]keyPolicyPrice, 0, len(settings))
	unavailable := []string{}
	for _, setting := range settings {
		p := keyPolicyPrice{Model: setting.Model}
		multiplier := 1.0
		if setting.PriceMultiplier != nil {
			multiplier = *setting.PriceMultiplier
		}
		values := []float64{setting.PromptPricePer1M, setting.CompletionPricePer1M, setting.CacheReadPricePer1M, setting.CacheWritePricePer1M}
		rates := []*string{&p.Input, &p.Output, &p.CacheRead, &p.CacheWrite}
		p.Unavailable = blocked[setting.ID]
		for i, value := range values {
			rate, ok := keyPolicyRate(value, multiplier)
			if !ok {
				p.Unavailable = true
			}
			*rates[i] = rate
		}
		if p.Unavailable {
			p = keyPolicyPrice{Model: setting.Model, Unavailable: true}
			unavailable = append(unavailable, setting.Model)
		}
		prices = append(prices, p)
	}
	return prices, unavailable
}

func keyPolicyCycles(rows []entities.QuotaCycle, resources map[string]bool, now time.Time) []keyPolicyCycle {
	cycles := []keyPolicyCycle{}
	seen := map[string]bool{}
	// Rows arrive newest observation/ID first; expired or stale resources are never synthesized.
	for _, row := range rows {
		if row.Provider != "codex" || !resources[row.AuthIndex] || row.ID <= 0 || row.WindowSeconds <= 0 || row.WindowStartedAt.IsZero() || row.LastObservedAt.IsZero() || row.LastObservedAt.Before(row.WindowStartedAt) || row.LastObservedAt.After(now) || row.WindowStartedAt.After(now) || !row.ResetAt.After(now) || !row.WindowStartedAt.Before(row.ResetAt) {
			continue
		}
		periods := []string{}
		if row.QuotaKey == "rate_limit.primary_window" {
			periods = append(periods, "codex_primary")
		}
		if (row.QuotaKey == "rate_limit.primary_window" || row.QuotaKey == "rate_limit.secondary_window") && row.WindowSeconds == 7*24*60*60 {
			periods = append(periods, "codex_weekly")
		}
		for _, period := range periods {
			key := row.AuthIndex + ":" + period
			if seen[key] {
				continue
			}
			seen[key] = true
			cycles = append(cycles, keyPolicyCycle{row.AuthIndex, period, strconv.FormatInt(row.ID, 10), row.WindowStartedAt, row.ResetAt, row.LastObservedAt})
		}
	}
	return cycles
}

func (s *KeyPolicyService) syncOnce(ctx context.Context) (int, []string, error) {
	report, status, err := s.client.FetchKeyPolicies(ctx)
	if err != nil {
		return status, nil, err
	}
	var snapshot struct {
		Resources []struct {
			ID       string `json:"resource_id"`
			Disabled bool   `json:"disabled"`
		} `json:"resources"`
	}
	if err = json.Unmarshal(report, &snapshot); err != nil {
		return 0, nil, err
	}
	resources := map[string]bool{}
	for _, resource := range snapshot.Resources {
		resources[resource.ID] = !resource.Disabled
	}
	settings, err := repository.ListModelPriceSettings(s.db.WithContext(ctx))
	if err != nil {
		return 0, nil, err
	}
	rules, err := repository.ListModelPriceRules(s.db.WithContext(ctx))
	if err != nil {
		return 0, nil, err
	}
	now := time.Now().UTC()
	var rows []entities.QuotaCycle
	err = s.db.WithContext(ctx).Where("provider = ? AND reset_at > ?", "codex", timeutil.FormatSortableStorageTime(now)).Order("last_observed_at DESC, id DESC").Find(&rows).Error
	if err != nil {
		return 0, nil, err
	}
	prices, unavailable := keyPolicyPrices(settings, rules)
	status, err = s.client.SyncKeyPolicies(ctx, struct {
		Prices []keyPolicyPrice `json:"prices"`
		Cycles []keyPolicyCycle `json:"cycles"`
	}{prices, keyPolicyCycles(rows, resources, now)})
	return status, unavailable, err
}

func (s *KeyPolicyService) Run(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			return nil
		}
		attempt := time.Now().UTC()
		requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		code, unavailable, err := s.syncOnce(requestCtx)
		cancel()
		delay := time.Minute
		s.mu.Lock()
		s.status.LastAttempt = attempt
		s.status.UnavailableModels = unavailable
		if err == nil {
			s.status.LastSuccess = time.Now().UTC()
			s.status.Status = "ready"
			if len(unavailable) > 0 {
				s.status.Status = "unsupported_prices"
			}
		} else {
			s.status.Status = "unavailable"
			if code == http.StatusNotFound {
				s.status.Status = "unsupported_cpa"
				delay = 5 * time.Minute
			}
		}
		s.mu.Unlock()
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}
