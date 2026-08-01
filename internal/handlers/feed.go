package handlers

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/dcrespo1/kinops/internal/domain"
	"github.com/dcrespo1/kinops/internal/ical"
	"github.com/dcrespo1/kinops/internal/service"
)

type CalendarFeedService interface {
	CalendarFeed(context.Context, string) (domain.CalendarFeed, error)
}

type CalendarFeedHandler struct {
	service CalendarFeedService
	logger  *slog.Logger
}

func NewCalendarFeedHandler(service CalendarFeedService, logger *slog.Logger) *CalendarFeedHandler {
	return &CalendarFeedHandler{service: service, logger: logger}
}

func (h *CalendarFeedHandler) Get(w http.ResponseWriter, r *http.Request) {
	feed, err := h.service.CalendarFeed(r.Context(), chi.URLParam(r, "personToken"))
	if errors.Is(err, service.ErrCalendarFeedNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	var body bytes.Buffer
	if err := ical.Write(&body, feed); err != nil {
		h.internalError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `inline; filename="kinops.ics"`)
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body.Bytes())
}

func (h *CalendarFeedHandler) internalError(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.Error("calendar feed request failed", "method", r.Method, "request_id", middleware.GetReqID(r.Context()), "error", err)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}
