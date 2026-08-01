package app

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dcrespo1/kinops/internal/auth"
	"github.com/dcrespo1/kinops/internal/config"
	"github.com/dcrespo1/kinops/internal/testutil"
)

func TestManagementFlowCreatesFixedSchedule(t *testing.T) {
	db := testutil.NewTestDatabase(t)
	passwordHash, err := auth.HashPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	adminAuth, err := auth.NewManager("admin", passwordHash, false)
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(Dependencies{DB: db, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Config: config.Config{Location: time.UTC, TimeZone: "UTC"}, AdminAuth: adminAuth})

	personResponse := postForm(t, router, "/people", url.Values{"name": {"Dylan"}, "color": {"#123456"}})
	if personResponse.Code != http.StatusSeeOther {
		t.Fatalf("create person status = %d", personResponse.Code)
	}
	secondPersonResponse := postForm(t, router, "/people", url.Values{"name": {"Amanda"}, "color": {"#654321"}})
	if secondPersonResponse.Code != http.StatusSeeOther {
		t.Fatalf("create second person status = %d", secondPersonResponse.Code)
	}
	thirdPersonResponse := postForm(t, router, "/people", url.Values{"name": {"Jordan"}, "color": {"#345678"}})
	if thirdPersonResponse.Code != http.StatusSeeOther {
		t.Fatalf("create third person status = %d body=%s", thirdPersonResponse.Code, thirdPersonResponse.Body.String())
	}
	choreResponse := postForm(t, router, "/chores", url.Values{"name": {"Dishes"}, "category": {"Kitchen"}})
	if choreResponse.Code != http.StatusSeeOther {
		t.Fatalf("create chore status = %d", choreResponse.Code)
	}

	var choreID, personID int64
	if err := db.QueryRow(`SELECT id FROM chores WHERE name = 'Dishes'`).Scan(&choreID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT id FROM people WHERE name = 'Dylan'`).Scan(&personID); err != nil {
		t.Fatal(err)
	}
	today := time.Now().UTC().Format("2006-01-02")
	scheduleResponse := postForm(t, router, "/chores/1/schedules", url.Values{"rule_type": {"one_off"}, "start_date": {today}, "assignment_mode": {"fixed"}, "fixed_person_id": {"1"}})
	if scheduleResponse.Code != http.StatusSeeOther {
		t.Fatalf("create schedule status = %d body=%s", scheduleResponse.Code, scheduleResponse.Body.String())
	}
	var instanceID, assignedID int64
	if err := db.QueryRow(`SELECT id, assigned_person_id FROM chore_instances WHERE chore_id = ?`, choreID).Scan(&instanceID, &assignedID); err != nil {
		t.Fatal(err)
	}
	if assignedID != personID {
		t.Errorf("assigned person = %d, want %d", assignedID, personID)
	}

	request := httptest.NewRequest(http.MethodGet, "/chores/1", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "Dishes") || !strings.Contains(recorder.Body.String(), "Always assigned to Dylan") {
		t.Errorf("detail response = %d %s", recorder.Code, recorder.Body.String())
	}

	dailyRequest := httptest.NewRequest(http.MethodGet, "/daily?date="+today, nil)
	dailyRecorder := httptest.NewRecorder()
	router.ServeHTTP(dailyRecorder, dailyRequest)
	if dailyRecorder.Code != http.StatusOK || !strings.Contains(dailyRecorder.Body.String(), "Dishes") || !strings.Contains(dailyRecorder.Body.String(), "Complete") {
		t.Fatalf("daily response = %d %s", dailyRecorder.Code, dailyRecorder.Body.String())
	}
	completeRequest := httptest.NewRequest(http.MethodPatch, "/instances/"+strconv.FormatInt(instanceID, 10)+"/complete?date="+today, nil)
	completeRequest.Header.Set("HX-Request", "true")
	completeRecorder := httptest.NewRecorder()
	router.ServeHTTP(completeRecorder, completeRequest)
	if completeRecorder.Code != http.StatusOK || !strings.Contains(completeRecorder.Body.String(), "Done") {
		t.Fatalf("complete response = %d %s", completeRecorder.Code, completeRecorder.Body.String())
	}
	var status string
	var logCount int
	if err := db.QueryRow(`SELECT status FROM chore_instances WHERE id = ?`, instanceID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM completion_logs WHERE chore_instance_id = ? AND event_type = 'completed'`, instanceID).Scan(&logCount); err != nil {
		t.Fatal(err)
	}
	if status != "done" || logCount != 1 {
		t.Errorf("after completion status=%s logs=%d", status, logCount)
	}
	for _, path := range []string{"/weekly?date=" + today, "/monthly?month=" + today[:7]} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "Dishes") {
			t.Errorf("calendar response %s = %d %s", path, recorder.Code, recorder.Body.String())
		}
	}
	var firstToken, secondToken string
	if err := db.QueryRow(`SELECT calendar_token FROM people WHERE name = 'Dylan'`).Scan(&firstToken); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT calendar_token FROM people WHERE name = 'Amanda'`).Scan(&secondToken); err != nil {
		t.Fatal(err)
	}
	firstFeed := httptest.NewRecorder()
	router.ServeHTTP(firstFeed, httptest.NewRequest(http.MethodGet, "/calendar/"+firstToken+".ics", nil))
	if firstFeed.Code != http.StatusOK || !strings.Contains(firstFeed.Body.String(), "SUMMARY:Dishes") {
		t.Errorf("first calendar feed = %d %s", firstFeed.Code, firstFeed.Body.String())
	}
	secondFeed := httptest.NewRecorder()
	router.ServeHTTP(secondFeed, httptest.NewRequest(http.MethodGet, "/calendar/"+secondToken+".ics", nil))
	if secondFeed.Code != http.StatusOK || strings.Contains(secondFeed.Body.String(), "SUMMARY:Dishes") || strings.Contains(secondFeed.Body.String(), "BEGIN:VEVENT") {
		t.Errorf("second calendar feed = %d %s", secondFeed.Code, secondFeed.Body.String())
	}
	unauthenticatedAdmin := httptest.NewRecorder()
	router.ServeHTTP(unauthenticatedAdmin, httptest.NewRequest(http.MethodGet, "/admin", nil))
	if unauthenticatedAdmin.Code != http.StatusSeeOther || unauthenticatedAdmin.Header().Get("Location") != "/admin/login" {
		t.Errorf("unauthenticated admin = %d %s", unauthenticatedAdmin.Code, unauthenticatedAdmin.Header().Get("Location"))
	}
	login := postForm(t, router, "/admin/login", url.Values{"username": {"admin"}, "password": {"secret"}})
	if login.Code != http.StatusSeeOther || len(login.Result().Cookies()) != 1 {
		t.Fatalf("admin login = %d %#v", login.Code, login.Result().Cookies())
	}
	adminCookie := login.Result().Cookies()[0]
	adminRequest := httptest.NewRequest(http.MethodGet, "/admin", nil)
	adminRequest.Host = "kinops.test"
	adminRequest.AddCookie(adminCookie)
	adminPage := httptest.NewRecorder()
	router.ServeHTTP(adminPage, adminRequest)
	if adminPage.Code != http.StatusOK || !strings.Contains(adminPage.Body.String(), "Completion overview") || !strings.Contains(adminPage.Body.String(), "Dishes") || !strings.Contains(adminPage.Body.String(), "http://kinops.test/calendar/"+firstToken+".ics") {
		t.Errorf("admin page = %d %s", adminPage.Code, adminPage.Body.String())
	}
	csrfRequest := httptest.NewRequest(http.MethodGet, "/admin", nil)
	csrfRequest.AddCookie(adminCookie)
	csrfToken, ok := adminAuth.CSRFToken(csrfRequest)
	if !ok {
		t.Fatal("admin session not found")
	}
	eventRequest := httptest.NewRequest(http.MethodPost, "/events", strings.NewReader(url.Values{
		"title": {"Family birthday"}, "category": {"Birthday"},
		"all_day": {"1"}, "start_date": {today}, "end_date": {today},
		"rule_type": {"one_off"}, "audience_person_ids": {strconv.FormatInt(personID, 10)},
	}.Encode()))
	eventRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	eventResponse := httptest.NewRecorder()
	router.ServeHTTP(eventResponse, eventRequest)
	if eventResponse.Code != http.StatusSeeOther || eventResponse.Header().Get("Location") != "/events" {
		t.Fatalf("create event response = %d %s", eventResponse.Code, eventResponse.Body.String())
	}
	eventsRequest := httptest.NewRequest(http.MethodGet, "/events", nil)
	eventsResponse := httptest.NewRecorder()
	router.ServeHTTP(eventsResponse, eventsRequest)
	if eventsResponse.Code != http.StatusOK || !strings.Contains(eventsResponse.Body.String(), "Family birthday") {
		t.Fatalf("events response = %d %s", eventsResponse.Code, eventsResponse.Body.String())
	}
	agendaResponse := httptest.NewRecorder()
	router.ServeHTTP(agendaResponse, httptest.NewRequest(http.MethodGet, "/daily?date="+today, nil))
	if agendaResponse.Code != http.StatusOK || !strings.Contains(agendaResponse.Body.String(), "Family birthday") || !strings.Contains(agendaResponse.Body.String(), "Dylan") {
		t.Fatalf("daily agenda response = %d %s", agendaResponse.Code, agendaResponse.Body.String())
	}
	monthlyEventsResponse := httptest.NewRecorder()
	router.ServeHTTP(monthlyEventsResponse, httptest.NewRequest(http.MethodGet, "/monthly?month="+today[:7], nil))
	if monthlyEventsResponse.Code != http.StatusOK || !strings.Contains(monthlyEventsResponse.Body.String(), "Family birthday") || !strings.Contains(monthlyEventsResponse.Body.String(), "1 events") {
		t.Fatalf("monthly events response = %d %s", monthlyEventsResponse.Code, monthlyEventsResponse.Body.String())
	}
	weeklyEventsResponse := httptest.NewRecorder()
	router.ServeHTTP(weeklyEventsResponse, httptest.NewRequest(http.MethodGet, "/weekly?date="+today, nil))
	if weeklyEventsResponse.Code != http.StatusOK || !strings.Contains(weeklyEventsResponse.Body.String(), "Family birthday") || !strings.Contains(weeklyEventsResponse.Body.String(), "1 events") {
		t.Fatalf("weekly events response = %d %s", weeklyEventsResponse.Code, weeklyEventsResponse.Body.String())
	}
	rotateRequest := httptest.NewRequest(http.MethodPost, "/admin/calendar/"+strconv.FormatInt(personID, 10)+"/rotate", strings.NewReader(url.Values{"csrf_token": {csrfToken}}.Encode()))
	rotateRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rotateRequest.AddCookie(adminCookie)
	rotateResponse := httptest.NewRecorder()
	router.ServeHTTP(rotateResponse, rotateRequest)
	if rotateResponse.Code != http.StatusSeeOther {
		t.Fatalf("rotate response = %d %s", rotateResponse.Code, rotateResponse.Body.String())
	}
	var rotatedToken string
	if err := db.QueryRow(`SELECT calendar_token FROM people WHERE id = ?`, personID).Scan(&rotatedToken); err != nil {
		t.Fatal(err)
	}
	if rotatedToken == firstToken {
		t.Error("calendar token did not rotate")
	}
}

func postForm(t *testing.T, handler http.Handler, path string, values url.Values) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(values.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}
