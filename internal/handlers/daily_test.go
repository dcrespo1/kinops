package handlers

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/dcrespo1/kinops/internal/domain"
)

type fakeDailyService struct {
	view      domain.DailyView
	completed domain.DailyInstance
	reopened  domain.DailyInstance
	err       error
}

func (f *fakeDailyService) DailyView(context.Context, time.Time) (domain.DailyView, error) {
	return f.view, f.err
}

func (f *fakeDailyService) CompleteInstance(context.Context, int64) (domain.DailyInstance, error) {
	return f.completed, f.err
}

func (f *fakeDailyService) ReopenInstance(context.Context, int64) (domain.DailyInstance, error) {
	return f.reopened, f.err
}

func TestDailyHandlerRendersTwoPeople(t *testing.T) {
	date := time.Date(2026, time.July, 31, 0, 0, 0, 0, time.UTC)
	service := &fakeDailyService{view: domain.DailyView{Date: date, People: []domain.PersonDay{{Person: domain.Person{ID: 1, Name: "Dylan", Color: "#111111"}}, {Person: domain.Person{ID: 2, Name: "Wife", Color: "#222222"}}}}}
	handler := NewDailyHandler(service, discardLogger(), time.UTC)
	request := httptest.NewRequest(http.MethodGet, "/daily?date=2026-07-31", nil)
	recorder := httptest.NewRecorder()
	handler.Get(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	for _, value := range []string{"Friday, July 31", "Dylan", "Wife", "/events/new?date=2026-07-31"} {
		if !strings.Contains(recorder.Body.String(), value) {
			t.Errorf("body does not contain %q", value)
		}
	}
}

func TestDailyHandlerReturnsHTMXCardAfterCompletion(t *testing.T) {
	date := time.Date(2026, time.July, 31, 0, 0, 0, 0, time.UTC)
	completedAt := date.Add(12 * time.Hour)
	item := domain.DailyInstance{
		Instance: domain.ChoreInstance{ID: 9, DueDate: date, Status: domain.InstanceDone, CompletedAt: &completedAt},
		Chore:    domain.Chore{Name: "Dishes"},
		Assignee: domain.Person{ID: 1, Name: "Dylan", Color: "#111111"},
	}
	service := &fakeDailyService{completed: item}
	handler := NewDailyHandler(service, discardLogger(), time.UTC)
	router := chi.NewRouter()
	router.Patch("/instances/{instanceID}/complete", handler.Complete)
	request := httptest.NewRequest(http.MethodPatch, "/instances/9/complete?date=2026-07-31", nil)
	request.Header.Set("HX-Request", "true")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "Done") || !strings.Contains(recorder.Body.String(), "Undo") {
		t.Errorf("response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
