package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dcrespo1/kinops/internal/domain"
)

type fakeCalendarService struct {
	week       domain.WeekView
	month      domain.MonthView
	weeklyDate time.Time
	monthDate  time.Time
	err        error
}

func (f *fakeCalendarService) WeeklyView(_ context.Context, date time.Time) (domain.WeekView, error) {
	f.weeklyDate = date
	return f.week, f.err
}

func (f *fakeCalendarService) MonthlyView(_ context.Context, date time.Time) (domain.MonthView, error) {
	f.monthDate = date
	return f.month, f.err
}

func TestCalendarHandlerWeekly(t *testing.T) {
	start := time.Date(2026, time.July, 27, 0, 0, 0, 0, time.UTC)
	service := &fakeCalendarService{week: domain.WeekView{StartDate: start, EndDate: start.AddDate(0, 0, 6), Today: start, HorizonEnd: start.AddDate(0, 0, 60)}}
	handler := NewCalendarHandler(service, discardLogger(), time.UTC)
	request := httptest.NewRequest(http.MethodGet, "/weekly?date=2026-07-31", nil)
	recorder := httptest.NewRecorder()
	handler.Weekly(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "Weekly view") || !strings.Contains(recorder.Body.String(), `aria-current="page"`) {
		t.Errorf("response = %d %s", recorder.Code, recorder.Body.String())
	}
	if got := service.weeklyDate.Format("2006-01-02"); got != "2026-07-31" {
		t.Errorf("service date = %s", got)
	}
}

func TestCalendarHandlerMonthlyAndInvalidParameters(t *testing.T) {
	month := time.Date(2028, time.February, 1, 0, 0, 0, 0, time.UTC)
	weeks := make([][]domain.MonthDay, 6)
	date := time.Date(2028, time.January, 31, 0, 0, 0, 0, time.UTC)
	for week := range weeks {
		weeks[week] = make([]domain.MonthDay, 7)
		for day := range weeks[week] {
			weeks[week][day] = domain.MonthDay{Date: date, InMonth: date.Month() == time.February}
			date = date.AddDate(0, 0, 1)
		}
	}
	service := &fakeCalendarService{month: domain.MonthView{Month: month, Today: month, GridStart: weeks[0][0].Date, GridEnd: weeks[5][6].Date, HorizonEnd: month.AddDate(0, 0, 60), Weeks: weeks}}
	handler := NewCalendarHandler(service, discardLogger(), time.UTC)
	request := httptest.NewRequest(http.MethodGet, "/monthly?month=2028-02", nil)
	recorder := httptest.NewRecorder()
	handler.Monthly(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "February 2028") {
		t.Errorf("response = %d %s", recorder.Code, recorder.Body.String())
	}
	badRequest := httptest.NewRequest(http.MethodGet, "/monthly?month=2028-2", nil)
	badRecorder := httptest.NewRecorder()
	handler.Monthly(badRecorder, badRequest)
	if badRecorder.Code != http.StatusBadRequest {
		t.Errorf("invalid month status = %d", badRecorder.Code)
	}
}
