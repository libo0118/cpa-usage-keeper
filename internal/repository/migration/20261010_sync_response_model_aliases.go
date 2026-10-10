package migration

import (
	"fmt"

	"gorm.io/gorm"
)

func syncResponseModelAliasesMigration(tx *gorm.DB) error {
	for _, table := range []string{"usage_events", "usage_events_archive"} {
		if !tx.Migrator().HasTable(table) || !tx.Migrator().HasColumn(table, "response_model") || !tx.Migrator().HasColumn(table, "upstream_response_model") {
			continue
		}
		// Both names carry observed metadata; fill missing aliases without replacing either observation.
		for _, columns := range [][2]string{{"response_model", "upstream_response_model"}, {"upstream_response_model", "response_model"}} {
			query := "UPDATE " + table + " SET " + columns[0] + " = " + columns[1] + " WHERE TRIM(COALESCE(" + columns[0] + ", '')) = '' AND TRIM(COALESCE(" + columns[1] + ", '')) <> ''"
			if err := tx.Exec(query).Error; err != nil {
				return fmt.Errorf("sync %s.%s response model alias: %w", table, columns[0], err)
			}
		}
	}
	return nil
}
