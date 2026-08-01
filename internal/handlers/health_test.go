package handlers

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/dcrespo1/kinops/internal/database"
)

func TestHealthHandlerReturnsOK(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "kinops.db")

	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("database.Open() returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	handler := NewHealthHandler(db)

	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recorder := httptest.NewRecorder()

	handler.Get(recorder, request)

	response := recorder.Result()
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Errorf(
			"status code = %d, want %d",
			response.StatusCode,
			http.StatusOK,
		)
	}

	if got := recorder.Body.String(); got != "ok\n" {
		t.Errorf("body = %q, want %q", got, "ok\n")
	}
}
