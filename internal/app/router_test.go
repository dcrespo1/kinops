package app

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dcrespo1/kinops/internal/config"
	"github.com/dcrespo1/kinops/internal/database"
)

func TestRouter(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "kinops.db")

	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("database.Open() returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	if err := database.Migrate(db); err != nil {
		t.Fatalf("database.Migrate() returned error: %v", err)
	}

	logger := slog.New(
		slog.NewTextHandler(io.Discard, nil),
	)

	router := NewRouter(Dependencies{
		DB:     db,
		Logger: logger,
		Config: config.Config{
			ListenAddress: ":8080",
			DatabasePath:  dbPath,
			TimeZone:      "UTC",
		},
	})

	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "home",
			path:       "/",
			wantStatus: http.StatusOK,
			wantBody:   "Set up your household",
		},
		{
			name:       "invalid daily date",
			path:       "/daily?date=not-a-date",
			wantStatus: http.StatusBadRequest,
			wantBody:   "date must use YYYY-MM-DD",
		},
		{
			name:       "weekly",
			path:       "/weekly?date=2026-07-31",
			wantStatus: http.StatusOK,
			wantBody:   "Weekly view",
		},
		{
			name:       "monthly",
			path:       "/monthly?month=2026-07",
			wantStatus: http.StatusOK,
			wantBody:   "July 2026",
		},
		{
			name:       "kitchen daily disabled",
			path:       "/kitchen/daily?date=2026-08-02",
			wantStatus: http.StatusOK,
			wantBody:   "Connect Mealie",
		},
		{
			name:       "kitchen weekly disabled",
			path:       "/kitchen/weekly?date=2026-08-02",
			wantStatus: http.StatusOK,
			wantBody:   "Kitchen weekly",
		},
		{
			name:       "kitchen recipes disabled",
			path:       "/kitchen/recipes?date=2026-08-03",
			wantStatus: http.StatusOK,
			wantBody:   "Connect Mealie",
		},
		{
			name:       "kitchen groceries disabled",
			path:       "/kitchen/groceries",
			wantStatus: http.StatusOK,
			wantBody:   "Connect Mealie",
		},
		{
			name:       "new event preserves selected date",
			path:       "/events/new?date=2026-08-04",
			wantStatus: http.StatusOK,
			wantBody:   `name="start_date" required value="2026-08-04"`,
		},
		{
			name:       "new event seeds selected recurrence month",
			path:       "/events/new?date=2026-08-04",
			wantStatus: http.StatusOK,
			wantBody:   `<option value="8" selected>August</option>`,
		},
		{
			name:       "new event uses curated category dropdown",
			path:       "/events/new?date=2026-08-04",
			wantStatus: http.StatusOK,
			wantBody:   `<option value="General" selected>General</option>`,
		},
		{
			name:       "new event seeds selected recurrence day",
			path:       "/events/new?date=2026-08-04",
			wantStatus: http.StatusOK,
			wantBody:   `name="annual_day" required value="4"`,
		},
		{
			name:       "new event rejects invalid selected date",
			path:       "/events/new?date=not-a-date",
			wantStatus: http.StatusBadRequest,
			wantBody:   "date must use YYYY-MM-DD",
		},
		{
			name:       "health",
			path:       "/healthz",
			wantStatus: http.StatusOK,
			wantBody:   "ok",
		},
		{
			name:       "unknown calendar token",
			path:       "/calendar/cccccccccccccccccccccccccccccccc.ics",
			wantStatus: http.StatusNotFound,
			wantBody:   "404",
		},
		{
			name:       "not found",
			path:       "/does-not-exist",
			wantStatus: http.StatusNotFound,
			wantBody:   "404",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(
				http.MethodGet,
				tt.path,
				nil,
			)
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, request)

			if recorder.Code != tt.wantStatus {
				t.Errorf(
					"status code = %d, want %d",
					recorder.Code,
					tt.wantStatus,
				)
			}

			if !strings.Contains(
				recorder.Body.String(),
				tt.wantBody,
			) {
				t.Errorf(
					"body %q does not contain %q",
					recorder.Body.String(),
					tt.wantBody,
				)
			}
		})
	}
}
