package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/dcrespo1/kinops/internal/domain"
	"github.com/dcrespo1/kinops/internal/scheduling"
	"github.com/dcrespo1/kinops/internal/store"
	"github.com/dcrespo1/kinops/internal/views/components"
	"github.com/dcrespo1/kinops/internal/views/pages"
)

type DailyService interface {
	DailyView(context.Context, time.Time) (domain.DailyView, error)
	CompleteInstance(context.Context, int64) (domain.DailyInstance, error)
	ReopenInstance(context.Context, int64) (domain.DailyInstance, error)
}

type DailyHandler struct {
	service  DailyService
	logger   *slog.Logger
	location *time.Location
	now      func() time.Time
}

func NewDailyHandler(service DailyService, logger *slog.Logger, location *time.Location) *DailyHandler {
	return &DailyHandler{service: service, logger: logger, location: location, now: time.Now}
}

func (h *DailyHandler) Get(w http.ResponseWriter, r *http.Request) {
	date, err := h.requestDate(r)
	if err != nil {
		http.Error(w, "date must use YYYY-MM-DD", http.StatusBadRequest)
		return
	}
	view, err := h.service.DailyView(r.Context(), date)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.render(w, r, pages.Daily(view), http.StatusOK)
}

func (h *DailyHandler) Complete(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, h.service.CompleteInstance, true)
}

func (h *DailyHandler) Reopen(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, h.service.ReopenInstance, false)
}

func (h *DailyHandler) transition(w http.ResponseWriter, r *http.Request, transition func(context.Context, int64) (domain.DailyInstance, error), completing bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "instanceID"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	date, err := h.requestDate(r)
	if err != nil {
		http.Error(w, "date must use YYYY-MM-DD", http.StatusBadRequest)
		return
	}
	item, err := transition(r.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			http.NotFound(w, r)
		case errors.Is(err, store.ErrConflict):
			http.Error(w, "This chore changed in another request. Refresh and try again.", http.StatusConflict)
		default:
			h.internalError(w, r, err)
		}
		return
	}
	if r.Header.Get("HX-Request") == "true" {
		view, viewErr := h.service.DailyView(r.Context(), date)
		if viewErr != nil {
			h.internalError(w, r, viewErr)
			return
		}
		// Completing an overdue item removes it because completed historical
		// instances are intentionally absent from the overdue list.
		removeCard := completing && item.Instance.DueDate.Format(scheduling.DateLayout) < date.Format(scheduling.DateLayout)
		h.render(w, r, components.TransitionResult(item, date, view, removeCard), http.StatusOK)
		return
	}
	http.Redirect(w, r, "/daily?date="+date.Format(scheduling.DateLayout), http.StatusSeeOther)
}

func (h *DailyHandler) requestDate(r *http.Request) (time.Time, error) {
	value := r.URL.Query().Get("date")
	if value == "" {
		return scheduling.Date(h.now(), h.location), nil
	}
	date, err := scheduling.ParseDate(value, h.location)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse daily date: %w", err)
	}
	return date, nil
}

func (h *DailyHandler) render(w http.ResponseWriter, r *http.Request, component templ.Component, status int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := component.Render(r.Context(), w); err != nil {
		h.logger.Error("render daily response", "error", err)
	}
}

func (h *DailyHandler) internalError(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.Error("daily request failed", "method", r.Method, "path", r.URL.Path, "error", err)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}
