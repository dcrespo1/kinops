package handlers

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/dcrespo1/kinops/internal/domain"
	"github.com/dcrespo1/kinops/internal/service"
)

type fakeCalendarFeedService struct {
	feed  domain.CalendarFeed
	err   error
	token string
}

func (f *fakeCalendarFeedService) CalendarFeed(_ context.Context, token string) (domain.CalendarFeed, error) {
	f.token = token
	return f.feed, f.err
}

func TestCalendarFeedHandler(t *testing.T) {
	token := strings.Repeat("a", 32)
	fake := &fakeCalendarFeedService{feed: domain.CalendarFeed{Name: "Dylan · KinOps", Events: []domain.CalendarFeedEvent{{
		InstanceID: 9,
		DueDate:    time.Date(2026, time.July, 31, 0, 0, 0, 0, time.UTC),
		Summary:    "Dishes",
	}}}}
	router := chi.NewRouter()
	router.Get("/calendar/{personToken}.ics", NewCalendarFeedHandler(fake, slog.Default()).Get)
	request := httptest.NewRequest(http.MethodGet, "/calendar/"+token+".ics", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if fake.token != token {
		t.Errorf("service token = %q", fake.token)
	}
	if got := recorder.Header().Get("Content-Type"); got != "text/calendar; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := recorder.Header().Get("Content-Disposition"); got != `inline; filename="kinops.ics"` {
		t.Errorf("Content-Disposition = %q", got)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "private, max-age=300" {
		t.Errorf("Cache-Control = %q", got)
	}
	if !strings.Contains(recorder.Body.String(), "SUMMARY:Dishes\r\n") {
		t.Errorf("body = %q", recorder.Body.String())
	}
}

func TestCalendarFeedHandlerNotFoundAndSafeInternalError(t *testing.T) {
	token := strings.Repeat("b", 32)
	for _, tt := range []struct {
		name string
		err  error
		want int
	}{
		{name: "not found", err: service.ErrCalendarFeedNotFound, want: http.StatusNotFound},
		{name: "internal", err: errors.New("database unavailable"), want: http.StatusInternalServerError},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&logs, nil))
			router := chi.NewRouter()
			router.Get("/calendar/{personToken}.ics", NewCalendarFeedHandler(&fakeCalendarFeedService{err: tt.err}, logger).Get)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/calendar/"+token+".ics", nil))
			if recorder.Code != tt.want {
				t.Errorf("status = %d, want %d", recorder.Code, tt.want)
			}
			if strings.Contains(logs.String(), token) {
				t.Errorf("log leaked token: %s", logs.String())
			}
		})
	}
}
