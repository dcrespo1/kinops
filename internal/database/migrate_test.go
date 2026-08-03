package database

import (
	"path/filepath"
	"testing"
)

func TestMigrateCreatesSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "kinops.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate() returned error: %v", err)
	}

	expectedTables := []string{
		"people",
		"chores",
		"schedules",
		"chore_instances",
		"completion_logs",
		"household_events",
		"event_audiences",
		"event_occurrences",
		"household_settings",
	}

	for _, table := range expectedTables {
		t.Run(table, func(t *testing.T) {
			var name string

			err := db.QueryRow(
				`
				SELECT name
				FROM sqlite_master
				WHERE type = 'table'
				  AND name = ?
				`,
				table,
			).Scan(&name)

			if err != nil {
				t.Fatalf(
					"table %q was not created: %v",
					table,
					err,
				)
			}
		})
	}
}

func TestMigrateCanRunTwice(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "kinops.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	if err := Migrate(db); err != nil {
		t.Fatalf("first Migrate() returned error: %v", err)
	}

	if err := Migrate(db); err != nil {
		t.Fatalf("second Migrate() returned error: %v", err)
	}
}
