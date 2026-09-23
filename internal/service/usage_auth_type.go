package service

import (
	"fmt"
	"strings"

	"cpa-usage-keeper/internal/entities"
	"gorm.io/gorm"
)

// Resolve only missing attribution, using unique active identities rather than
// provider-name guesses. Called inside the event/inbox write transaction so the
// stored event, recent cache and credential aggregation receive the same type.
func resolveMissingUsageAuthTypes(db *gorm.DB, events []entities.UsageEvent) error {
	indices := make([]string, 0)
	seen := make(map[string]bool)
	for _, event := range events {
		kind, index := normalizeRedisAuthType(event.AuthType), strings.TrimSpace(event.AuthIndex)
		if (kind == "" || kind == "unknown") && index != "" && !seen[index] {
			seen[index] = true
			indices = append(indices, index)
		}
	}
	if len(indices) == 0 {
		return nil
	}
	if db == nil {
		return fmt.Errorf("database is nil")
	}
	identities := make(map[string]entities.UsageIdentity)
	ambiguous := make(map[string]bool)
	for start := 0; start < len(indices); start += redisUsageIdentityTypeLookupBatchSize {
		end := min(start+redisUsageIdentityTypeLookupBatchSize, len(indices))
		var rows []entities.UsageIdentity
		if err := db.Select("identity, auth_type, type, provider").Where("identity IN ? AND is_deleted = ?", indices[start:end], false).Find(&rows).Error; err != nil {
			return fmt.Errorf("resolve missing usage auth type: %w", err)
		}
		for _, row := range rows {
			if _, exists := identities[row.Identity]; exists {
				ambiguous[row.Identity] = true
			}
			identities[row.Identity] = row
		}
	}
	for i := range events {
		event := &events[i]
		kind := normalizeRedisAuthType(event.AuthType)
		if kind != "" && kind != "unknown" {
			continue
		}
		index := strings.TrimSpace(event.AuthIndex)
		identity, exists := identities[index]
		if !exists || ambiguous[index] {
			continue
		}
		provider := strings.TrimSpace(event.Provider)
		if provider != "" && !strings.EqualFold(provider, "unknown") && !strings.EqualFold(provider, strings.TrimSpace(identity.Type)) && !strings.EqualFold(provider, strings.TrimSpace(identity.Provider)) {
			continue
		}
		switch identity.AuthType {
		case entities.UsageIdentityAuthTypeAuthFile:
			event.AuthType = "oauth"
		case entities.UsageIdentityAuthTypeAIProvider:
			event.AuthType = "apikey"
		}
	}
	return nil
}
