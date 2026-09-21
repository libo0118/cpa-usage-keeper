package migration

import (
	"cpa-usage-keeper/internal/entities"
	"fmt"
	"gorm.io/gorm"
)

func addUpstreamResponseModelMigration(tx *gorm.DB) error {
	for _, model := range []any{&entities.UsageEvent{}, &entities.UsageEventArchive{}} {
		if tx.Migrator().HasTable(model) && !tx.Migrator().HasColumn(model, "UpstreamResponseModel") {
			if err := tx.Migrator().AddColumn(model, "UpstreamResponseModel"); err != nil {
				return fmt.Errorf("add upstream response model: %w", err)
			}
		}
	}
	// Historical requests have no trustworthy response metadata. Leave them empty.
	return nil
}
