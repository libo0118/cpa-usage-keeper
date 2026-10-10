package migration

import (
	"fmt"

	"cpa-usage-keeper/internal/entities"
	"gorm.io/gorm"
)

func addUsageEventResponseModelMigration(tx *gorm.DB) error {
	for _, table := range []struct {
		model any
		name  string
	}{
		{model: &entities.UsageEvent{}, name: "usage_events"},
		{model: &entities.UsageEventArchive{}, name: "usage_events_archive"},
	} {
		if !tx.Migrator().HasTable(table.model) {
			continue
		}
		// SQLite HasColumn uses LIKE and can match upstream_response_model.
		var columnCount int
		if err := tx.Raw("SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ? COLLATE NOCASE", table.name, "response_model").Scan(&columnCount).Error; err != nil {
			return fmt.Errorf("inspect %s.response_model column: %w", table.name, err)
		}
		if columnCount > 0 {
			continue
		}
		if err := tx.Migrator().AddColumn(table.model, "ResponseModel"); err != nil {
			return fmt.Errorf("add %s.response_model column: %w", table.name, err)
		}
	}
	return nil
}
