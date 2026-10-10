package test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"cpa-usage-keeper/internal/repository/migration"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestResponseModelCompatibilityMigrationPreservesHotAndArchiveMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, columns, values, response, upstream string
	}{
		{"custom database", ", upstream_response_model TEXT", ", 'custom-model'", "custom-model", "custom-model"},
		{"upstream database", ", response_model TEXT", ", 'official-model'", "official-model", "official-model"},
		{"both observations", ", response_model TEXT, upstream_response_model TEXT", ", 'official-model', 'custom-model'", "official-model", "custom-model"},
		{"unobserved", "", "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "existing.db")), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			closeMigrationTestDatabase(t, db)
			for _, table := range []string{"usage_events", "usage_events_archive"} {
				if err := db.Exec("CREATE TABLE " + table + " (id INTEGER PRIMARY KEY, model TEXT, qoder_credits TEXT, workbuddy_credits TEXT" + tc.columns + ")").Error; err != nil {
					t.Fatal(err)
				}
				if err := db.Exec("INSERT INTO " + table + " VALUES (1, 'requested-model', '{\"credits\":0.012300}', NULL" + tc.values + ")").Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := migration.MarkAllAsApplied(db); err != nil {
				t.Fatal(err)
			}
			versions := []string{"20260918_usage_event_response_model", "20260921_add_upstream_response_model", "20261010_sync_response_model_aliases"}
			if err := db.Table("schema_migrations").Where("version IN ?", versions).Delete(nil).Error; err != nil {
				t.Fatal(err)
			}
			for range 2 {
				if err := migration.Run(db); err != nil {
					t.Fatal(err)
				}
				for _, table := range []string{"usage_events", "usage_events_archive"} {
					var model, response, upstream, qoder string
					var workbuddy sql.NullString
					if err := db.Raw("SELECT model, response_model, upstream_response_model, qoder_credits, workbuddy_credits FROM "+table+" WHERE id = 1").Row().Scan(&model, &response, &upstream, &qoder, &workbuddy); err != nil {
						t.Fatal(err)
					}
					if model != "requested-model" || response != tc.response || upstream != tc.upstream || qoder != `{"credits":0.012300}` || workbuddy.Valid {
						t.Fatalf("%s metadata changed: model=%q response=%q upstream=%q qoder=%q workbuddy=%+v", table, model, response, upstream, qoder, workbuddy)
					}
				}
			}
		})
	}
}
