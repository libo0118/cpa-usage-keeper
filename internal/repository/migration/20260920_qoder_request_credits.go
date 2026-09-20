package migration

import (
	"cpa-usage-keeper/internal/entities"
	"fmt"
	"gorm.io/gorm"
)

func addQoderRequestCreditsMigration(tx *gorm.DB) error {
	for _, model := range []any{&entities.UsageEvent{}, &entities.UsageEventArchive{}} {
		if tx.Migrator().HasTable(model) && !tx.Migrator().HasColumn(model, "QoderCredits") {
			if err := tx.Migrator().AddColumn(model, "QoderCredits"); err != nil {
				return fmt.Errorf("add optional Qoder credits: %w", err)
			}
		}
	}
	return nil
}
