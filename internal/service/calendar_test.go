package service

import (
	"context"
	"testing"
	"time"

	"github.com/dcrespo1/kinops/internal/domain"
	"github.com/dcrespo1/kinops/internal/store"
	"github.com/dcrespo1/kinops/internal/testutil"
)

func TestWeeklyViewUsesMondayThroughSundayAndGroupsInstances(t *testing.T) {
	db := testutil.NewTestDatabase(t)
	repository := store.NewSQLite(db)
	now := time.Date(2026, time.December, 28, 12, 0, 0, 0, time.UTC)
	svc := NewWithClock(repository, time.UTC, func() time.Time { return now })
	ctx := context.Background()
	first := createTestPerson(t, svc, "Dylan", "#111111")
	createTestPerson(t, svc, "Wife", "#222222")
	chore, err := svc.CreateChore(ctx, domain.Chore{Name: "Dishes"})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, time.December, 28, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 6)
	if _, err := svc.CreateSchedule(ctx, domain.Schedule{
		ChoreID: chore.ID, Rule: domain.RecurrenceRule{Type: domain.RuleDaily},
		StartDate: start, EndDate: &end, AssignmentMode: domain.AssignmentFixed, FixedPersonID: &first.ID,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateEvent(ctx, domain.HouseholdEvent{
		Title: "Family visit", Category: domain.EventCategoryFamily, AllDay: true,
		StartDate: time.Date(2026, time.December, 30, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC),
		Rule:      domain.EventRecurrenceRule{Type: domain.EventRuleOneOff},
	}); err != nil {
		t.Fatal(err)
	}

	view, err := svc.WeeklyView(ctx, time.Date(2026, time.December, 30, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if got := view.StartDate.Format("2006-01-02"); got != "2026-12-28" {
		t.Errorf("start = %s", got)
	}
	if got := view.EndDate.Format("2006-01-02"); got != "2027-01-03" {
		t.Errorf("end = %s", got)
	}
	if len(view.People) != 2 || len(view.People[0].Days) != 7 {
		t.Fatalf("weekly people = %#v", view.People)
	}
	for index, day := range view.People[0].Days {
		if len(day.Instances) != 1 {
			t.Errorf("day %d instances = %d, want 1", index, len(day.Instances))
		}
	}
	if len(view.EventDays) != 7 {
		t.Fatalf("weekly event days = %d, want 7", len(view.EventDays))
	}
	for index, want := range []int{0, 0, 1, 1, 0, 0, 0} {
		if got := len(view.EventDays[index].Events); got != want {
			t.Errorf("event day %d count = %d, want %d", index, got, want)
		}
	}
}

func TestMonthlyViewBuildsSixWeekMondayFirstGrid(t *testing.T) {
	db := testutil.NewTestDatabase(t)
	svc := NewWithClock(store.NewSQLite(db), time.UTC, func() time.Time {
		return time.Date(2028, time.February, 10, 12, 0, 0, 0, time.UTC)
	})
	_, err := svc.CreateEvent(context.Background(), domain.HouseholdEvent{
		Title: "Family trip", Category: domain.EventCategoryTravel, AllDay: true,
		StartDate: time.Date(2028, time.February, 12, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2028, time.February, 14, 0, 0, 0, 0, time.UTC),
		Rule:      domain.EventRecurrenceRule{Type: domain.EventRuleOneOff},
	})
	if err != nil {
		t.Fatal(err)
	}
	view, err := svc.MonthlyView(context.Background(), time.Date(2028, time.February, 20, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Weeks) != 6 {
		t.Fatalf("weeks = %d, want 6", len(view.Weeks))
	}
	if got := view.GridStart.Format("2006-01-02"); got != "2028-01-31" {
		t.Errorf("grid start = %s", got)
	}
	if got := view.GridEnd.Format("2006-01-02"); got != "2028-03-12" {
		t.Errorf("grid end = %s", got)
	}
	if view.Weeks[0][0].InMonth {
		t.Error("January 31 marked in month")
	}
	if !view.Weeks[0][1].InMonth {
		t.Error("February 1 marked outside month")
	}
	if got := view.Weeks[4][1].Date.Format("2006-01-02"); got != "2028-02-29" {
		t.Errorf("leap day = %s", got)
	}
	for _, check := range []struct {
		date string
		want int
	}{{"2028-02-12", 1}, {"2028-02-13", 1}, {"2028-02-14", 0}} {
		day := findMonthDay(t, view, check.date)
		if len(day.Events) != check.want {
			t.Errorf("%s events = %d, want %d", check.date, len(day.Events), check.want)
		}
	}
}

func findMonthDay(t *testing.T, view domain.MonthView, date string) domain.MonthDay {
	t.Helper()
	for _, week := range view.Weeks {
		for _, day := range week {
			if day.Date.Format("2006-01-02") == date {
				return day
			}
		}
	}
	t.Fatalf("month day %s not found", date)
	return domain.MonthDay{}
}

func TestWeeklyViewAcrossDSTKeepsSevenCalendarDays(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	db := testutil.NewTestDatabase(t)
	svc := NewWithClock(store.NewSQLite(db), location, func() time.Time {
		return time.Date(2026, time.March, 8, 12, 0, 0, 0, location)
	})
	view, err := svc.WeeklyView(context.Background(), time.Date(2026, time.March, 8, 12, 0, 0, 0, location))
	if err != nil {
		t.Fatal(err)
	}
	if got := view.StartDate.Format("2006-01-02"); got != "2026-03-02" {
		t.Errorf("start = %s", got)
	}
	if got := view.EndDate.Format("2006-01-02"); got != "2026-03-08" {
		t.Errorf("end = %s", got)
	}
}
