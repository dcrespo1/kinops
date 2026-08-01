package testutil

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/dcrespo1/kinops/internal/database"
)

func NewTestDatabase(t *testing.T) *sql.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "kinops.db")

	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
	})

	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}

	return db
}
