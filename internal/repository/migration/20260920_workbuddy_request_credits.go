package migration

import (
	"cpa-usage-keeper/internal/entities"
	"fmt"
	"gorm.io/gorm"
)

func addWorkBuddyRequestCreditsMigration(tx *gorm.DB) error {
	for _, model := range []any{&entities.UsageEvent{}, &entities.UsageEventArchive{}} {
		if tx.Migrator().HasTable(model) && !tx.Migrator().HasColumn(model, "WorkBuddyCredits") {
			if err := tx.Migrator().AddColumn(model, "WorkBuddyCredits"); err != nil {
				return fmt.Errorf("add optional WorkBuddy credits: %w", err)
			}
		}
	}
	return nil
}
