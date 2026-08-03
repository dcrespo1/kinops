package handlers

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/dcrespo1/kinops/internal/auth"
	"github.com/dcrespo1/kinops/internal/domain"
)

type fakeAdminService struct {
	dashboard domain.AdminDashboard
	rotatedID int64
	err       error
}

type fakeMealieStatusService struct{ status domain.MealieStatus }

func (f fakeMealieStatusService) MealieStatus(context.Context) domain.MealieStatus { return f.status }

func (f *fakeAdminService) AdminDashboard(context.Context) (domain.AdminDashboard, error) {
	return f.dashboard, f.err
}

func (f *fakeAdminService) RotateCalendarToken(_ context.Context, id int64) (domain.Person, error) {
	f.rotatedID = id
	return domain.Person{ID: id}, f.err
}

func TestAdminLoginDashboardCSRFAndRotation(t *testing.T) {
	hash, err := auth.HashPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	manager, err := auth.NewManager("admin", hash, false)
	if err != nil {
		t.Fatal(err)
	}
	person := domain.Person{ID: 7, Name: "Dylan", Color: "#123456", CalendarToken: strings.Repeat("a", 48), Active: true}
	fake := &fakeAdminService{dashboard: domain.AdminDashboard{
		GeneratedAt: time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC),
		TimeZone:    "UTC", HorizonDays: 60,
		SevenDay:       domain.AnalyticsWindow{Days: 7, Assigned: 4, Completed: 3, RatePercent: 75},
		People:         []domain.PersonAnalytics{{Person: person, Assigned: 4, Completed: 3, RatePercent: 75}},
		CalendarPeople: []domain.Person{person},
	}}
	router := adminTestRouterWithMealie(fake, manager, fakeMealieStatusService{status: domain.MealieStatus{Enabled: true, Connected: true, Version: "v3.22.0", Message: "Connected", CheckedAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)}})

	badLogin := postAdminForm(router, "/admin/login", url.Values{"username": {"admin"}, "password": {"wrong"}}, nil)
	if badLogin.Code != http.StatusUnauthorized || len(badLogin.Result().Cookies()) != 0 || !strings.Contains(badLogin.Body.String(), "Invalid username or password") {
		t.Errorf("bad login = %d %#v %s", badLogin.Code, badLogin.Result().Cookies(), badLogin.Body.String())
	}
	goodLogin := postAdminForm(router, "/admin/login", url.Values{"username": {"admin"}, "password": {"secret"}}, nil)
	if goodLogin.Code != http.StatusSeeOther || goodLogin.Header().Get("Location") != "/admin" {
		t.Fatalf("good login = %d %s", goodLogin.Code, goodLogin.Header().Get("Location"))
	}
	cookie := goodLogin.Result().Cookies()[0]

	dashboardRequest := httptest.NewRequest(http.MethodGet, "https://kinops.example/admin", nil)
	dashboardRequest.AddCookie(cookie)
	dashboard := httptest.NewRecorder()
	router.ServeHTTP(dashboard, dashboardRequest)
	if dashboard.Code != http.StatusOK || !strings.Contains(dashboard.Body.String(), "Completion overview") || !strings.Contains(dashboard.Body.String(), "https://kinops.example/calendar/"+person.CalendarToken+".ics") || !strings.Contains(dashboard.Body.String(), "v3.22.0") {
		t.Errorf("dashboard = %d %s", dashboard.Code, dashboard.Body.String())
	}

	csrfRequest := httptest.NewRequest(http.MethodGet, "/admin", nil)
	csrfRequest.AddCookie(cookie)
	csrfToken, ok := manager.CSRFToken(csrfRequest)
	if !ok {
		t.Fatal("login session was not stored")
	}
	forbidden := postAdminForm(router, "/admin/calendar/7/rotate", url.Values{"csrf_token": {"wrong"}}, cookie)
	if forbidden.Code != http.StatusForbidden || fake.rotatedID != 0 {
		t.Errorf("forbidden rotation = %d, ID %d", forbidden.Code, fake.rotatedID)
	}
	rotated := postAdminForm(router, "/admin/calendar/7/rotate", url.Values{"csrf_token": {csrfToken}}, cookie)
	if rotated.Code != http.StatusSeeOther || rotated.Header().Get("Location") != "/admin#calendar" || fake.rotatedID != 7 {
		t.Errorf("rotation = %d %s ID %d", rotated.Code, rotated.Header().Get("Location"), fake.rotatedID)
	}
	logout := postAdminForm(router, "/admin/logout", url.Values{"csrf_token": {csrfToken}}, cookie)
	if logout.Code != http.StatusSeeOther || logout.Result().Cookies()[0].MaxAge != -1 {
		t.Errorf("logout = %d %#v", logout.Code, logout.Result().Cookies())
	}
}

func TestAdminRoutesRequireSession(t *testing.T) {
	hash, err := auth.HashPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	manager, err := auth.NewManager("admin", hash, false)
	if err != nil {
		t.Fatal(err)
	}
	router := adminTestRouter(&fakeAdminService{}, manager)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/admin", nil))
	if recorder.Code != http.StatusSeeOther || recorder.Header().Get("Location") != "/admin/login" {
		t.Errorf("response = %d %s", recorder.Code, recorder.Header().Get("Location"))
	}
}

func adminTestRouter(service AdminService, manager *auth.Manager) http.Handler {
	return adminTestRouterWithMealie(service, manager, nil)
}

func adminTestRouterWithMealie(service AdminService, manager *auth.Manager, mealie MealieStatusService) http.Handler {
	handler := NewAdminHandler(service, manager, slog.New(slog.NewTextHandler(io.Discard, nil)), mealie)
	router := chi.NewRouter()
	router.Get("/admin/login", handler.LoginPage)
	router.Post("/admin/login", handler.Login)
	router.Group(func(protected chi.Router) {
		protected.Use(manager.Require)
		protected.Get("/admin", handler.Dashboard)
		protected.Post("/admin/logout", handler.Logout)
		protected.Post("/admin/calendar/{personID}/rotate", handler.RotateCalendarToken)
	})
	return router
}

func postAdminForm(handler http.Handler, path string, values url.Values, cookie *http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(values.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if cookie != nil {
		request.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}
