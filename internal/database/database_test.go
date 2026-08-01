package database

import (
	"path/filepath"
	"testing"
)

func TestOpenCreatesDatabase(t *testing.T) {
	dbPath := filepath.Join(
		t.TempDir(),
		"nested",
		"kinops.db",
	)

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	if err := db.Ping(); err != nil {
		t.Fatalf("db.Ping() returned error: %v", err)
	}
}

func TestOpenEnablesForeignKeys(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "kinops.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	var enabled int

	if err := db.QueryRow(
		"PRAGMA foreign_keys",
	).Scan(&enabled); err != nil {
		t.Fatalf("query foreign_keys pragma: %v", err)
	}

	if enabled != 1 {
		t.Errorf("foreign_keys = %d, want 1", enabled)
	}
}
