package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/dcrespo1/kinops/internal/domain"
	"github.com/dcrespo1/kinops/internal/scheduling"
	"github.com/dcrespo1/kinops/internal/store"
	"github.com/dcrespo1/kinops/internal/views/pages"
)

type PublicEventService interface {
	ListEvents(context.Context) ([]domain.HouseholdEvent, error)
	GetEvent(context.Context, int64) (domain.HouseholdEvent, error)
	CreateEvent(context.Context, domain.HouseholdEvent) (domain.HouseholdEvent, error)
	UpdateEvent(context.Context, domain.HouseholdEvent) error
	DeactivateEvent(context.Context, int64) error
	ListPeople(context.Context) ([]domain.Person, error)
}

type EventHandler struct {
	service  PublicEventService
	logger   *slog.Logger
	location *time.Location
}

func NewEventHandler(service PublicEventService, logger *slog.Logger, location *time.Location) *EventHandler {
	return &EventHandler{service: service, logger: logger, location: location}
}

func (h *EventHandler) Index(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListEvents(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.render(w, r, pages.Events(items), http.StatusOK)
}

func (h *EventHandler) New(w http.ResponseWriter, r *http.Request) {
	people, err := h.service.ListPeople(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	today := scheduling.Date(time.Now(), h.location)
	if value := r.URL.Query().Get("date"); value != "" {
		today, err = scheduling.ParseDate(value, h.location)
		if err != nil {
			http.Error(w, "date must use YYYY-MM-DD", http.StatusBadRequest)
			return
		}
	}
	event := domain.HouseholdEvent{
		Category:  domain.EventCategoryGeneral,
		AllDay:    true,
		StartDate: today,
		EndDate:   today.AddDate(0, 0, 1),
		Rule: domain.EventRecurrenceRule{
			Type:       domain.EventRuleOneOff,
			Month:      today.Month(),
			DayOfMonth: today.Day(),
		},
	}
	h.render(w, r, pages.EventForm(pages.EventFormData{Event: event, People: people}), http.StatusOK)
}

func (h *EventHandler) Edit(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pathID(w, r)
	if !ok {
		return
	}
	event, err := h.service.GetEvent(r.Context(), id)
	if err != nil {
		h.storeError(w, r, err)
		return
	}
	people, err := h.service.ListPeople(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.render(w, r, pages.EventForm(pages.EventFormData{Event: event, People: people, Edit: true}), http.StatusOK)
}

func (h *EventHandler) Create(w http.ResponseWriter, r *http.Request) {
	event, err := h.eventFromForm(r, 0)
	if err == nil {
		_, err = h.service.CreateEvent(r.Context(), event)
	}
	if err != nil {
		h.formError(w, r, event, false, err)
		return
	}
	h.redirect(w, r, "/events")
}

func (h *EventHandler) Mutate(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pathID(w, r)
	if !ok {
		return
	}
	switch r.FormValue("_method") {
	case "put":
		event, err := h.eventFromForm(r, id)
		if err == nil {
			err = h.service.UpdateEvent(r.Context(), event)
		}
		if err != nil {
			h.formError(w, r, event, true, err)
			return
		}
		h.redirect(w, r, "/events")
	case "delete":
		if err := h.service.DeactivateEvent(r.Context(), id); err != nil {
			h.storeError(w, r, err)
			return
		}
		h.redirect(w, r, "/events")
	default:
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

func (h *EventHandler) eventFromForm(r *http.Request, id int64) (domain.HouseholdEvent, error) {
	event := domain.HouseholdEvent{ID: id, Title: r.FormValue("title"), Description: r.FormValue("description"), Location: r.FormValue("location"), Category: r.FormValue("category"), AllDay: r.FormValue("all_day") == "1", StartTime: r.FormValue("start_time"), EndTime: r.FormValue("end_time")}
	var err error
	event.StartDate, err = scheduling.ParseDate(r.FormValue("start_date"), h.location)
	if err != nil {
		return event, errors.New("start date must use YYYY-MM-DD")
	}
	event.EndDate, err = scheduling.ParseDate(r.FormValue("end_date"), h.location)
	if err != nil {
		return event, errors.New("end date must use YYYY-MM-DD")
	}
	// Forms use the human-friendly inclusive final day. Persistence uses the
	// iCalendar-compatible exclusive end date for all-day events.
	if event.AllDay {
		event.EndDate = event.EndDate.AddDate(0, 0, 1)
	}
	if value := r.FormValue("recurrence_end_date"); value != "" {
		end, parseErr := scheduling.ParseDate(value, h.location)
		if parseErr != nil {
			return event, errors.New("recurrence end date must use YYYY-MM-DD")
		}
		event.RecurrenceEndDate = &end
	}
	event.Rule.Type = domain.EventRuleType(r.FormValue("rule_type"))
	switch event.Rule.Type {
	case domain.EventRuleEveryNDays:
		event.Rule.Interval, err = strconv.Atoi(r.FormValue("interval"))
	case domain.EventRuleWeeklyDays:
		for _, raw := range r.Form["weekdays"] {
			day, parseErr := strconv.Atoi(raw)
			if parseErr != nil || day < 0 || day > 6 {
				return event, errors.New("choose valid weekdays")
			}
			event.Rule.Weekdays = append(event.Rule.Weekdays, time.Weekday(day))
		}
	case domain.EventRuleMonthlyDay:
		event.Rule.DayOfMonth, err = strconv.Atoi(r.FormValue("day"))
	case domain.EventRuleAnnual:
		month, monthErr := strconv.Atoi(r.FormValue("month"))
		day, dayErr := strconv.Atoi(r.FormValue("annual_day"))
		if monthErr != nil || dayErr != nil {
			err = errors.New("annual month and day must be numbers")
		} else {
			event.Rule.Month, event.Rule.DayOfMonth = time.Month(month), day
		}
	}
	if err != nil {
		return event, errors.New("recurrence value must be a number")
	}
	for _, raw := range r.Form["audience_person_ids"] {
		personID, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || personID <= 0 {
			return event, errors.New("choose a valid audience")
		}
		event.AudiencePersonIDs = append(event.AudiencePersonIDs, personID)
	}
	return event, nil
}

func (h *EventHandler) formError(w http.ResponseWriter, r *http.Request, event domain.HouseholdEvent, edit bool, formErr error) {
	people, err := h.service.ListPeople(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.render(w, r, pages.EventForm(pages.EventFormData{Event: event, People: people, Edit: edit, Error: formErr.Error()}), http.StatusUnprocessableEntity)
}

func (h *EventHandler) pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "eventID"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return 0, false
	}
	return id, true
}

func (h *EventHandler) storeError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	h.internalError(w, r, err)
}

func (h *EventHandler) redirect(w http.ResponseWriter, r *http.Request, location string) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", location)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, location, http.StatusSeeOther)
}

func (h *EventHandler) render(w http.ResponseWriter, r *http.Request, component templ.Component, status int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := component.Render(r.Context(), w); err != nil {
		h.logger.Error("render event response", "error", err)
	}
}

func (h *EventHandler) internalError(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.Error("event request failed", "method", r.Method, "path", r.URL.Path, "error", err)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}
