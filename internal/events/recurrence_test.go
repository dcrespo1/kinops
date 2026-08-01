package events

import (
	"testing"
	"time"

	"github.com/dcrespo1/kinops/internal/domain"
)

func TestGenerateEventRecurrences(t *testing.T) {
	location := time.FixedZone("household", -5*60*60)
	date := func(value string) time.Time {
		parsed, err := time.ParseInLocation("2006-01-02", value, location)
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}
	tests := []struct {
		name string
		rule domain.EventRecurrenceRule
		want []string
	}{
		{name: "daily", rule: domain.EventRecurrenceRule{Type: domain.EventRuleDaily}, want: []string{"2026-08-01", "2026-08-02", "2026-08-03", "2026-08-04"}},
		{name: "every two days", rule: domain.EventRecurrenceRule{Type: domain.EventRuleEveryNDays, Interval: 2}, want: []string{"2026-08-01", "2026-08-03"}},
		{name: "weekly", rule: domain.EventRecurrenceRule{Type: domain.EventRuleWeeklyDays, Weekdays: []time.Weekday{time.Monday, time.Saturday}}, want: []string{"2026-08-01", "2026-08-03"}},
		{name: "monthly", rule: domain.EventRecurrenceRule{Type: domain.EventRuleMonthlyDay, DayOfMonth: 3}, want: []string{"2026-08-03"}},
		{name: "one off", rule: domain.EventRecurrenceRule{Type: domain.EventRuleOneOff}, want: []string{"2026-08-01"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event := domain.HouseholdEvent{ID: 4, Title: "Family event", AllDay: true, StartDate: date("2026-08-01"), EndDate: date("2026-08-02"), Rule: test.rule}
			got, err := Generate(event, date("2026-08-01"), date("2026-08-04"), location)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(test.want) {
				t.Fatalf("len = %d, want %d: %#v", len(got), len(test.want), got)
			}
			for i := range got {
				if value := got[i].StartDate.Format("2006-01-02"); value != test.want[i] {
					t.Errorf("date[%d] = %s, want %s", i, value, test.want[i])
				}
				if duration := calendarDayOffset(got[i].StartDate, got[i].EndDate); duration != 1 {
					t.Errorf("duration[%d] = %d days", i, duration)
				}
			}
		})
	}
}

func TestGenerateAnnualSkipsNonLeapYear(t *testing.T) {
	location := time.UTC
	event := domain.HouseholdEvent{Title: "Leap day", AllDay: true, StartDate: time.Date(2024, 2, 29, 0, 0, 0, 0, location), EndDate: time.Date(2024, 3, 1, 0, 0, 0, 0, location), Rule: domain.EventRecurrenceRule{Type: domain.EventRuleAnnual, Month: time.February, DayOfMonth: 29}}
	got, err := Generate(event, time.Date(2025, 1, 1, 0, 0, 0, 0, location), time.Date(2028, 12, 31, 0, 0, 0, 0, location), location)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].StartDate.Format("2006-01-02") != "2028-02-29" {
		t.Fatalf("occurrences = %#v", got)
	}
}

func TestTimedOccurrenceKeepsWallClockAcrossDST(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	event := domain.HouseholdEvent{Title: "Dinner", StartDate: time.Date(2026, 3, 7, 0, 0, 0, 0, location), EndDate: time.Date(2026, 3, 7, 0, 0, 0, 0, location), StartTime: "18:00", EndTime: "19:00", Rule: domain.EventRecurrenceRule{Type: domain.EventRuleDaily}}
	got, err := Generate(event, event.StartDate, event.StartDate.AddDate(0, 0, 2), location)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d", len(got))
	}
	for i, occurrence := range got {
		if value := occurrence.StartAt.In(location).Format("15:04"); value != "18:00" {
			t.Errorf("start[%d] = %s", i, value)
		}
		if value := occurrence.EndAt.In(location).Format("15:04"); value != "19:00" {
			t.Errorf("end[%d] = %s", i, value)
		}
	}
}
