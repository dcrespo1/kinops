package pages

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dcrespo1/kinops/internal/domain"
)

func TestMonthlyMarksTodayAndOffersCrossOutOnlyForPastDays(t *testing.T) {
	location := time.FixedZone("household", -4*60*60)
	yesterday := time.Date(2026, time.August, 2, 0, 0, 0, 0, location)
	today := yesterday.AddDate(0, 0, 1)
	tomorrow := today.AddDate(0, 0, 1)
	view := domain.MonthView{
		Month:      time.Date(2026, time.August, 1, 0, 0, 0, 0, location),
		Today:      today,
		GridStart:  yesterday,
		GridEnd:    tomorrow,
		HorizonEnd: tomorrow.AddDate(0, 2, 0),
		Weeks: [][]domain.MonthDay{{
			{Date: yesterday, InMonth: true},
			{Date: today, InMonth: true, Events: []domain.ScheduledEvent{{Event: domain.HouseholdEvent{Title: "Dentist", AllDay: true}, Color: "#123456"}}},
			{Date: tomorrow, InMonth: true},
		}},
	}

	var output bytes.Buffer
	if err := monthlyContent(view).Render(context.Background(), &output); err != nil {
		t.Fatalf("render monthly view: %v", err)
	}
	html := output.String()

	for _, expected := range []string{
		`class="month-day-shell is-today"`,
		`class="today-badge">Today</span>`,
		`kinops:calendar:crossed:2026-08-02`,
		`aria-label="Toggle crossed-out state for August 2, 2026"`,
		`--event-color: #123456`,
	} {
		if !strings.Contains(html, expected) {
			t.Errorf("monthly view missing %q", expected)
		}
	}
	if count := strings.Count(html, `class="month-cross-toggle"`); count != 1 {
		t.Errorf("past-day cross-out controls = %d, want 1", count)
	}
}
