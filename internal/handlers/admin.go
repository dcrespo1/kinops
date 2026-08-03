package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/dcrespo1/kinops/internal/auth"
	"github.com/dcrespo1/kinops/internal/domain"
	"github.com/dcrespo1/kinops/internal/store"
	"github.com/dcrespo1/kinops/internal/views/pages"
)

const maxAdminFormBytes = 8 << 10

type AdminService interface {
	AdminDashboard(context.Context) (domain.AdminDashboard, error)
	RotateCalendarToken(context.Context, int64) (domain.Person, error)
}

type MealieStatusService interface {
	MealieStatus(context.Context) domain.MealieStatus
}

type AdminHandler struct {
	service AdminService
	auth    *auth.Manager
	logger  *slog.Logger
	mealie  MealieStatusService
}

func NewAdminHandler(service AdminService, authManager *auth.Manager, logger *slog.Logger, mealieStatus ...MealieStatusService) *AdminHandler {
	handler := &AdminHandler{service: service, auth: authManager, logger: logger}
	if len(mealieStatus) > 0 {
		handler.mealie = mealieStatus[0]
	}
	return handler
}

func (h *AdminHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
	h.noStore(w)
	if _, ok := h.auth.CSRFToken(r); ok {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	h.render(w, r, pages.AdminLogin(""), http.StatusOK)
}

func (h *AdminHandler) Login(w http.ResponseWriter, r *http.Request) {
	h.noStore(w)
	r.Body = http.MaxBytesReader(w, r.Body, maxAdminFormBytes)
	if err := r.ParseForm(); err != nil {
		h.render(w, r, pages.AdminLogin("Unable to sign in."), http.StatusBadRequest)
		return
	}
	if !h.auth.Authenticate(r.FormValue("username"), r.FormValue("password")) {
		h.render(w, r, pages.AdminLogin("Invalid username or password."), http.StatusUnauthorized)
		return
	}
	if _, err := h.auth.CreateSession(w); err != nil {
		h.internalError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (h *AdminHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	csrfToken, ok := h.auth.CSRFToken(r)
	if !ok {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}
	dashboard, err := h.service.AdminDashboard(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	mealieStatus := domain.MealieStatus{Message: "Mealie integration is not configured."}
	if h.mealie != nil {
		mealieStatus = h.mealie.MealieStatus(r.Context())
	}
	h.render(w, r, pages.Admin(pages.AdminPageData{Dashboard: dashboard, CSRFToken: csrfToken, BaseURL: requestBaseURL(r), Mealie: mealieStatus}), http.StatusOK)
}

func (h *AdminHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(w, r) {
		return
	}
	h.auth.DestroySession(w, r)
	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
}

func (h *AdminHandler) RotateCalendarToken(w http.ResponseWriter, r *http.Request) {
	if !h.validCSRF(w, r) {
		return
	}
	personID, err := strconv.ParseInt(chi.URLParam(r, "personID"), 10, 64)
	if err != nil || personID <= 0 {
		http.NotFound(w, r)
		return
	}
	if _, err := h.service.RotateCalendarToken(r.Context(), personID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		h.internalError(w, r, err)
		return
	}
	http.Redirect(w, r, "/admin#calendar", http.StatusSeeOther)
}

func (h *AdminHandler) validCSRF(w http.ResponseWriter, r *http.Request) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxAdminFormBytes)
	if err := r.ParseForm(); err != nil || !h.auth.VerifyCSRF(r, r.FormValue("csrf_token")) {
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return false
	}
	return true
}

func (h *AdminHandler) render(w http.ResponseWriter, r *http.Request, component templ.Component, status int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := component.Render(r.Context(), w); err != nil {
		h.logger.Error("render admin response", "error", err)
	}
}

func (h *AdminHandler) noStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
}

func (h *AdminHandler) internalError(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.Error("admin request failed", "method", r.Method, "route", "admin", "error", err)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

func requestBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if forwarded := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0])); forwarded == "http" || forwarded == "https" {
		scheme = forwarded
	}
	return fmt.Sprintf("%s://%s", scheme, r.Host)
}
