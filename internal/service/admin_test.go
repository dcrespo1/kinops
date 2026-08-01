package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dcrespo1/kinops/internal/domain"
	"github.com/dcrespo1/kinops/internal/store"
	"github.com/dcrespo1/kinops/internal/testutil"
)

func TestAdminDashboardAnalyticsAndCalendarPeople(t *testing.T) {
	db := testutil.NewTestDatabase(t)
	repository := store.NewSQLite(db)
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.July, 31, 15, 0, 0, 0, location)
	svc := NewWithClock(repository, location, func() time.Time { return now })
	ctx := context.Background()
	first := createTestPerson(t, svc, "Dylan", "#111111")
	second := createTestPerson(t, svc, "Amanda", "#222222")
	chore, err := svc.CreateChore(ctx, domain.Chore{Name: "Dishes"})
	if err != nil {
		t.Fatal(err)
	}
	firstSchedule := createAnalyticsSchedule(t, repository, chore.ID, first.ID, now.AddDate(0, 0, -400))
	secondSchedule := createAnalyticsSchedule(t, repository, chore.ID, second.ID, now.AddDate(0, 0, -1))

	firstDoneToday := createAnalyticsInstance(t, repository, chore.ID, firstSchedule.ID, 1, first.ID, now, domain.InstanceDone)
	createAnalyticsInstance(t, repository, chore.ID, firstSchedule.ID, 2, first.ID, now.AddDate(0, 0, -1), domain.InstanceDone)
	createAnalyticsInstance(t, repository, chore.ID, firstSchedule.ID, 3, first.ID, now.AddDate(0, 0, -3), domain.InstanceDone)
	createAnalyticsInstance(t, repository, chore.ID, firstSchedule.ID, 4, first.ID, now.AddDate(0, 0, -4), domain.InstancePending)
	createAnalyticsInstance(t, repository, chore.ID, firstSchedule.ID, 5, first.ID, now.AddDate(0, 0, -29), domain.InstanceSkipped)
	createAnalyticsInstance(t, repository, chore.ID, firstSchedule.ID, 6, first.ID, now.AddDate(0, 0, -400), domain.InstancePending)
	createAnalyticsInstance(t, repository, chore.ID, secondSchedule.ID, 1, second.ID, now, domain.InstancePending)
	secondDoneYesterday := createAnalyticsInstance(t, repository, chore.ID, secondSchedule.ID, 2, second.ID, now.AddDate(0, 0, -1), domain.InstanceDone)

	for _, log := range []domain.CompletionLog{
		{ChoreInstanceID: secondDoneYesterday.ID, PersonID: second.ID, EventType: domain.EventCompleted, OccurredAt: now.Add(-2 * time.Hour)},
		{ChoreInstanceID: firstDoneToday.ID, PersonID: first.ID, EventType: domain.EventCompleted, OccurredAt: now.Add(-time.Hour)},
	} {
		if err := repository.CreateCompletionLog(ctx, &log); err != nil {
			t.Fatal(err)
		}
	}

	dashboard, err := svc.AdminDashboard(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if dashboard.TimeZone != "America/New_York" || dashboard.HorizonDays != 60 || len(dashboard.CalendarPeople) != 2 {
		t.Errorf("dashboard configuration = %#v", dashboard)
	}
	if dashboard.SevenDay.Assigned != 6 || dashboard.SevenDay.Completed != 4 || dashboard.SevenDay.RatePercent != 67 {
		t.Errorf("seven-day analytics = %#v", dashboard.SevenDay)
	}
	if dashboard.ThirtyDay.Assigned != 7 || dashboard.ThirtyDay.Completed != 4 || dashboard.ThirtyDay.RatePercent != 57 {
		t.Errorf("thirty-day analytics = %#v", dashboard.ThirtyDay)
	}
	if dashboard.Overdue != 2 {
		t.Errorf("overdue = %d, want 2", dashboard.Overdue)
	}
	if len(dashboard.People) != 2 {
		t.Fatalf("people = %#v", dashboard.People)
	}
	if got := dashboard.People[0]; got.Assigned != 5 || got.Completed != 3 || got.RatePercent != 60 || got.Streak != 3 || got.Overdue != 2 {
		t.Errorf("first analytics = %#v", got)
	}
	if got := dashboard.People[1]; got.Assigned != 2 || got.Completed != 1 || got.RatePercent != 50 || got.Streak != 0 || got.Overdue != 0 {
		t.Errorf("second analytics = %#v", got)
	}
	if len(dashboard.Activity) != 2 || dashboard.Activity[0].PersonName != "Dylan" || dashboard.Activity[1].PersonName != "Amanda" {
		t.Errorf("activity = %#v", dashboard.Activity)
	}
}

func TestRotateCalendarTokenInvalidatesOldFeedURL(t *testing.T) {
	repository := store.NewSQLite(testutil.NewTestDatabase(t))
	svc := NewWithClock(repository, time.UTC, time.Now)
	person := createTestPerson(t, svc, "Dylan", "#111111")
	oldToken := person.CalendarToken
	rotated, err := svc.RotateCalendarToken(context.Background(), person.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.CalendarToken == oldToken || len(rotated.CalendarToken) != 48 {
		t.Errorf("rotated token = %q", rotated.CalendarToken)
	}
	if _, err := repository.GetActivePersonByCalendarToken(context.Background(), oldToken); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("old token lookup error = %v", err)
	}
	if resolved, err := repository.GetActivePersonByCalendarToken(context.Background(), rotated.CalendarToken); err != nil || resolved.ID != person.ID {
		t.Errorf("new token resolved %#v, error %v", resolved, err)
	}
}

func createAnalyticsSchedule(t *testing.T, repository *store.SQLite, choreID, personID int64, start time.Time) domain.Schedule {
	t.Helper()
	schedule := domain.Schedule{ChoreID: choreID, Rule: domain.RecurrenceRule{Type: domain.RuleDaily}, StartDate: start, AssignmentMode: domain.AssignmentFixed, FixedPersonID: &personID, Active: true}
	if err := repository.CreateSchedule(context.Background(), &schedule); err != nil {
		t.Fatal(err)
	}
	return schedule
}

func createAnalyticsInstance(t *testing.T, repository *store.SQLite, choreID, scheduleID, sequence, personID int64, dueDate time.Time, status domain.InstanceStatus) domain.ChoreInstance {
	t.Helper()
	instance := domain.ChoreInstance{ChoreID: choreID, ScheduleID: scheduleID, SequenceNo: sequence, DueDate: dueDate, AssignedPersonID: personID, Status: status}
	if status == domain.InstanceDone {
		completedAt := dueDate.Add(12 * time.Hour).UTC()
		instance.CompletedAt = &completedAt
	}
	if err := repository.CreateInstance(context.Background(), &instance); err != nil {
		t.Fatal(err)
	}
	return instance
}
