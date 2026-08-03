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
	"github.com/dcrespo1/kinops/internal/views/pages"
)

type ManagementService interface {
	ListChores(context.Context) ([]domain.Chore, error)
	GetChore(context.Context, int64) (domain.Chore, []domain.Schedule, error)
	CreateChore(context.Context, domain.Chore) (domain.Chore, error)
	UpdateChore(context.Context, domain.Chore) error
	DeactivateChore(context.Context, int64) error
	ListPeople(context.Context) ([]domain.Person, error)
	CreatePerson(context.Context, domain.Person) (domain.Person, error)
	GetHouseholdSettings(context.Context) (domain.HouseholdSettings, error)
	UpdateHouseholdEventColor(context.Context, string) error
	GetSchedule(context.Context, int64) (domain.Schedule, error)
	CreateSchedule(context.Context, domain.Schedule) (domain.Schedule, error)
	UpdateSchedule(context.Context, domain.Schedule) error
	DeactivateSchedule(context.Context, int64) error
}

type ManagementHandler struct {
	service  ManagementService
	logger   *slog.Logger
	location *time.Location
}

func NewManagementHandler(service ManagementService, logger *slog.Logger, location *time.Location) *ManagementHandler {
	return &ManagementHandler{service: service, logger: logger, location: location}
}

func (h *ManagementHandler) ChoreIndex(w http.ResponseWriter, r *http.Request) {
	chores, err := h.service.ListChores(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.render(w, r, pages.Chores(chores), http.StatusOK)
}

func (h *ManagementHandler) ChoreNew(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, pages.ChoreForm(pages.ChoreFormData{}), http.StatusOK)
}

func (h *ManagementHandler) ChoreCreate(w http.ResponseWriter, r *http.Request) {
	chore := choreFromForm(r)
	created, err := h.service.CreateChore(r.Context(), chore)
	if err != nil {
		h.render(w, r, pages.ChoreForm(pages.ChoreFormData{Chore: chore, Error: err.Error()}), http.StatusUnprocessableEntity)
		return
	}
	h.redirect(w, r, fmt.Sprintf("/chores/%d", created.ID))
}

func (h *ManagementHandler) ChoreShow(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pathID(w, r, "choreID")
	if !ok {
		return
	}
	chore, schedules, err := h.service.GetChore(r.Context(), id)
	if err != nil {
		h.storeError(w, r, err)
		return
	}
	people, err := h.service.ListPeople(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.render(w, r, pages.ChoreDetail(chore, schedules, people), http.StatusOK)
}

func (h *ManagementHandler) ChoreEdit(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pathID(w, r, "choreID")
	if !ok {
		return
	}
	chore, _, err := h.service.GetChore(r.Context(), id)
	if err != nil {
		h.storeError(w, r, err)
		return
	}
	h.render(w, r, pages.ChoreForm(pages.ChoreFormData{Chore: chore, Edit: true}), http.StatusOK)
}

func (h *ManagementHandler) ChoreMutate(w http.ResponseWriter, r *http.Request) {
	switch r.FormValue("_method") {
	case "put":
		h.ChoreUpdate(w, r)
	case "delete":
		h.ChoreDelete(w, r)
	default:
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

func (h *ManagementHandler) ChoreUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pathID(w, r, "choreID")
	if !ok {
		return
	}
	chore := choreFromForm(r)
	chore.ID = id
	if err := h.service.UpdateChore(r.Context(), chore); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			h.storeError(w, r, err)
			return
		}
		h.render(w, r, pages.ChoreForm(pages.ChoreFormData{Chore: chore, Error: err.Error(), Edit: true}), http.StatusUnprocessableEntity)
		return
	}
	h.redirect(w, r, fmt.Sprintf("/chores/%d", id))
}

func (h *ManagementHandler) ChoreDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pathID(w, r, "choreID")
	if !ok {
		return
	}
	if err := h.service.DeactivateChore(r.Context(), id); err != nil {
		h.storeError(w, r, err)
		return
	}
	h.redirect(w, r, "/chores")
}

func choreFromForm(r *http.Request) domain.Chore {
	return domain.Chore{Name: r.FormValue("name"), Category: r.FormValue("category"), Description: r.FormValue("description")}
}

func (h *ManagementHandler) PeopleIndex(w http.ResponseWriter, r *http.Request) {
	people, err := h.service.ListPeople(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	settings, err := h.service.GetHouseholdSettings(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.render(w, r, pages.People(people, settings, ""), http.StatusOK)
}

func (h *ManagementHandler) PersonCreate(w http.ResponseWriter, r *http.Request) {
	_, err := h.service.CreatePerson(r.Context(), domain.Person{Name: r.FormValue("name"), Color: r.FormValue("color")})
	if err != nil {
		people, listErr := h.service.ListPeople(r.Context())
		if listErr != nil {
			h.internalError(w, r, listErr)
			return
		}
		settings, settingsErr := h.service.GetHouseholdSettings(r.Context())
		if settingsErr != nil {
			h.internalError(w, r, settingsErr)
			return
		}
		h.render(w, r, pages.People(people, settings, err.Error()), http.StatusUnprocessableEntity)
		return
	}
	h.redirect(w, r, "/people")
}

func (h *ManagementHandler) HouseholdEventColorUpdate(w http.ResponseWriter, r *http.Request) {
	if err := h.service.UpdateHouseholdEventColor(r.Context(), r.FormValue("household_event_color")); err != nil {
		people, listErr := h.service.ListPeople(r.Context())
		if listErr != nil {
			h.internalError(w, r, listErr)
			return
		}
		settings, settingsErr := h.service.GetHouseholdSettings(r.Context())
		if settingsErr != nil {
			h.internalError(w, r, settingsErr)
			return
		}
		h.render(w, r, pages.People(people, settings, err.Error()), http.StatusUnprocessableEntity)
		return
	}
	h.redirect(w, r, "/people")
}

func (h *ManagementHandler) ScheduleNew(w http.ResponseWriter, r *http.Request) {
	choreID, ok := h.pathID(w, r, "choreID")
	if !ok {
		return
	}
	if _, _, err := h.service.GetChore(r.Context(), choreID); err != nil {
		h.storeError(w, r, err)
		return
	}
	people, err := h.service.ListPeople(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	today := scheduling.Date(time.Now(), h.location)
	schedule := domain.Schedule{ChoreID: choreID, Rule: domain.RecurrenceRule{Type: domain.RuleDaily}, StartDate: today, AssignmentMode: domain.AssignmentFixed}
	h.render(w, r, pages.ScheduleForm(pages.ScheduleFormData{Schedule: schedule, People: people}), http.StatusOK)
}

func (h *ManagementHandler) ScheduleCreate(w http.ResponseWriter, r *http.Request) {
	choreID, ok := h.pathID(w, r, "choreID")
	if !ok {
		return
	}
	schedule, parseErr := h.scheduleFromForm(r, choreID, 0)
	if parseErr == nil {
		created, err := h.service.CreateSchedule(r.Context(), schedule)
		if err == nil {
			h.redirect(w, r, fmt.Sprintf("/chores/%d", created.ChoreID))
			return
		}
		parseErr = err
	}
	h.scheduleFormError(w, r, schedule, false, parseErr)
}

func (h *ManagementHandler) ScheduleEdit(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pathID(w, r, "scheduleID")
	if !ok {
		return
	}
	schedule, err := h.service.GetSchedule(r.Context(), id)
	if err != nil {
		h.storeError(w, r, err)
		return
	}
	people, err := h.service.ListPeople(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.render(w, r, pages.ScheduleForm(pages.ScheduleFormData{Schedule: schedule, People: people, Edit: true}), http.StatusOK)
}

func (h *ManagementHandler) ScheduleMutate(w http.ResponseWriter, r *http.Request) {
	switch r.FormValue("_method") {
	case "put":
		h.ScheduleUpdate(w, r)
	case "delete":
		h.ScheduleDelete(w, r)
	default:
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

func (h *ManagementHandler) ScheduleUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pathID(w, r, "scheduleID")
	if !ok {
		return
	}
	current, err := h.service.GetSchedule(r.Context(), id)
	if err != nil {
		h.storeError(w, r, err)
		return
	}
	schedule, parseErr := h.scheduleFromForm(r, current.ChoreID, id)
	if parseErr == nil {
		parseErr = h.service.UpdateSchedule(r.Context(), schedule)
	}
	if parseErr != nil {
		h.scheduleFormError(w, r, schedule, true, parseErr)
		return
	}
	h.redirect(w, r, fmt.Sprintf("/chores/%d", current.ChoreID))
}

func (h *ManagementHandler) ScheduleDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pathID(w, r, "scheduleID")
	if !ok {
		return
	}
	schedule, err := h.service.GetSchedule(r.Context(), id)
	if err != nil {
		h.storeError(w, r, err)
		return
	}
	if err := h.service.DeactivateSchedule(r.Context(), id); err != nil {
		h.storeError(w, r, err)
		return
	}
	h.redirect(w, r, fmt.Sprintf("/chores/%d", schedule.ChoreID))
}

func (h *ManagementHandler) scheduleFromForm(r *http.Request, choreID, scheduleID int64) (domain.Schedule, error) {
	schedule := domain.Schedule{ID: scheduleID, ChoreID: choreID, AssignmentMode: domain.AssignmentMode(r.FormValue("assignment_mode"))}
	start, err := scheduling.ParseDate(r.FormValue("start_date"), h.location)
	if err != nil {
		return schedule, err
	}
	schedule.StartDate = start
	if value := r.FormValue("end_date"); value != "" {
		end, err := scheduling.ParseDate(value, h.location)
		if err != nil {
			return schedule, err
		}
		schedule.EndDate = &end
	}
	schedule.Rule.Type = domain.RuleType(r.FormValue("rule_type"))
	switch schedule.Rule.Type {
	case domain.RuleEveryNDays:
		schedule.Rule.Interval, err = strconv.Atoi(r.FormValue("interval"))
	case domain.RuleWeeklyDays:
		for _, value := range r.Form["weekdays"] {
			day, parseErr := strconv.Atoi(value)
			if parseErr != nil || day < 1 || day > 7 {
				return schedule, errors.New("weekdays must be between 1 and 7")
			}
			if day == 7 {
				schedule.Rule.Weekdays = append(schedule.Rule.Weekdays, time.Sunday)
			} else {
				schedule.Rule.Weekdays = append(schedule.Rule.Weekdays, time.Weekday(day))
			}
		}
	case domain.RuleMonthlyDay:
		schedule.Rule.DayOfMonth, err = strconv.Atoi(r.FormValue("day"))
	}
	if err != nil {
		return schedule, errors.New("recurrence value must be a number")
	}
	if schedule.AssignmentMode == domain.AssignmentFixed {
		id, parseErr := strconv.ParseInt(r.FormValue("fixed_person_id"), 10, 64)
		if parseErr != nil {
			return schedule, errors.New("choose the person who always receives this chore")
		}
		schedule.FixedPersonID = &id
	} else if schedule.AssignmentMode == domain.AssignmentRotate {
		id, parseErr := strconv.ParseInt(r.FormValue("rotation_start_person_id"), 10, 64)
		if parseErr != nil {
			return schedule, errors.New("choose the first person in the rotation")
		}
		schedule.RotationStartPersonID = &id
	}
	return schedule, nil
}

func (h *ManagementHandler) scheduleFormError(w http.ResponseWriter, r *http.Request, schedule domain.Schedule, edit bool, err error) {
	people, listErr := h.service.ListPeople(r.Context())
	if listErr != nil {
		h.internalError(w, r, listErr)
		return
	}
	h.render(w, r, pages.ScheduleForm(pages.ScheduleFormData{Schedule: schedule, People: people, Error: err.Error(), Edit: edit}), http.StatusUnprocessableEntity)
}

func (h *ManagementHandler) pathID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return 0, false
	}
	return id, true
}

func (h *ManagementHandler) render(w http.ResponseWriter, r *http.Request, component templ.Component, status int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := component.Render(r.Context(), w); err != nil {
		h.logger.Error("render response", "error", err)
	}
}

func (h *ManagementHandler) redirect(w http.ResponseWriter, r *http.Request, location string) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", location)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, location, http.StatusSeeOther)
}

func (h *ManagementHandler) storeError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	h.internalError(w, r, err)
}

func (h *ManagementHandler) internalError(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.Error("request failed", "method", r.Method, "path", r.URL.Path, "error", err)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}
