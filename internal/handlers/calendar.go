package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/a-h/templ"

	"github.com/dcrespo1/kinops/internal/domain"
	"github.com/dcrespo1/kinops/internal/scheduling"
	"github.com/dcrespo1/kinops/internal/views/pages"
)

type CalendarService interface {
	WeeklyView(context.Context, time.Time) (domain.WeekView, error)
	MonthlyView(context.Context, time.Time) (domain.MonthView, error)
}

type CalendarHandler struct {
	service  CalendarService
	logger   *slog.Logger
	location *time.Location
	now      func() time.Time
}

func NewCalendarHandler(service CalendarService, logger *slog.Logger, location *time.Location) *CalendarHandler {
	return &CalendarHandler{service: service, logger: logger, location: location, now: time.Now}
}

func (h *CalendarHandler) Weekly(w http.ResponseWriter, r *http.Request) {
	date, err := h.dateParameter(r, "date")
	if err != nil {
		http.Error(w, "date must use YYYY-MM-DD", http.StatusBadRequest)
		return
	}
	view, err := h.service.WeeklyView(r.Context(), date)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.render(w, r, pages.Weekly(view))
}

func (h *CalendarHandler) Monthly(w http.ResponseWriter, r *http.Request) {
	value := r.URL.Query().Get("month")
	var month time.Time
	var err error
	if value == "" {
		now := h.now().In(h.location)
		month = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, h.location)
	} else {
		month, err = scheduling.ParseDate(value+"-01", h.location)
		if err == nil && month.Format("2006-01") != value {
			err = fmt.Errorf("month must use YYYY-MM")
		}
	}
	if err != nil {
		http.Error(w, "month must use YYYY-MM", http.StatusBadRequest)
		return
	}
	view, err := h.service.MonthlyView(r.Context(), month)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.render(w, r, pages.Monthly(view))
}

func (h *CalendarHandler) dateParameter(r *http.Request, name string) (time.Time, error) {
	value := r.URL.Query().Get(name)
	if value == "" {
		return scheduling.Date(h.now(), h.location), nil
	}
	return scheduling.ParseDate(value, h.location)
}

func (h *CalendarHandler) render(w http.ResponseWriter, r *http.Request, component templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := component.Render(r.Context(), w); err != nil {
		h.logger.Error("render calendar response", "error", err)
	}
}

func (h *CalendarHandler) internalError(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.Error("calendar request failed", "method", r.Method, "path", r.URL.Path, "error", err)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}
