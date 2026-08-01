package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dcrespo1/kinops/internal/domain"
	"github.com/dcrespo1/kinops/internal/testutil"
)

func TestSQLiteRoundTripsChoreScheduleAndInstance(t *testing.T) {
	db := testutil.NewTestDatabase(t)
	repository := NewSQLite(db)
	ctx := context.Background()
	person := domain.Person{Name: "Dylan", Color: "#123456", CalendarToken: "12345678901234567890123456789012", Active: true}
	if err := repository.CreatePerson(ctx, &person); err != nil {
		t.Fatal(err)
	}
	chore := domain.Chore{Name: "Dishes", Description: "Wash dishes", Category: "Kitchen", Active: true}
	if err := repository.CreateChore(ctx, &chore); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, time.July, 31, 0, 0, 0, 0, time.UTC)
	schedule := domain.Schedule{ChoreID: chore.ID, Rule: domain.RecurrenceRule{Type: domain.RuleEveryNDays, Interval: 2}, StartDate: start, AssignmentMode: domain.AssignmentFixed, FixedPersonID: &person.ID, Active: true}
	if err := repository.CreateSchedule(ctx, &schedule); err != nil {
		t.Fatal(err)
	}
	instance := domain.ChoreInstance{ChoreID: chore.ID, ScheduleID: schedule.ID, SequenceNo: 1, DueDate: start, AssignedPersonID: person.ID, Status: domain.InstancePending}
	if err := repository.CreateInstance(ctx, &instance); err != nil {
		t.Fatal(err)
	}

	gotSchedule, err := repository.GetSchedule(ctx, schedule.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotSchedule.Rule.Type != domain.RuleEveryNDays || gotSchedule.Rule.Interval != 2 {
		t.Errorf("rule = %#v", gotSchedule.Rule)
	}
	instances, err := repository.ListInstancesBySchedule(ctx, schedule.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(instances) != 1 || instances[0].AssignedPersonID != person.ID || instances[0].DueDate.Format("2006-01-02") != "2026-07-31" {
		t.Errorf("instances = %#v", instances)
	}
	rangeItems, err := repository.ListScheduledInstances(ctx, start, start)
	if err != nil {
		t.Fatal(err)
	}
	if len(rangeItems) != 1 || rangeItems[0].Chore.Name != "Dishes" || rangeItems[0].Assignee.Name != "Dylan" {
		t.Errorf("range items = %#v", rangeItems)
	}
	if err := repository.DeactivateChore(ctx, chore.ID); err != nil {
		t.Fatal(err)
	}
	rangeItems, err = repository.ListScheduledInstances(ctx, start, start)
	if err != nil {
		t.Fatal(err)
	}
	if len(rangeItems) != 1 || rangeItems[0].Chore.Active {
		t.Errorf("archived chore range items = %#v", rangeItems)
	}
}

func TestSQLiteNotFoundAndTransactionRollback(t *testing.T) {
	db := testutil.NewTestDatabase(t)
	repository := NewSQLite(db)
	ctx := context.Background()
	if _, err := repository.GetChore(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetChore() error = %v, want ErrNotFound", err)
	}
	wantErr := errors.New("stop")
	err := repository.WithinTx(ctx, func(tx *SQLite) error {
		chore := domain.Chore{Name: "Rolled back", Active: true}
		if err := tx.CreateChore(ctx, &chore); err != nil {
			return err
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("WithinTx() error = %v", err)
	}
	chores, err := repository.ListChores(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(chores) != 0 {
		t.Errorf("rollback left %d chores", len(chores))
	}
}

func TestSQLiteCalendarFeedQueriesAreActivePersonScopedAndInclusive(t *testing.T) {
	db := testutil.NewTestDatabase(t)
	repository := NewSQLite(db)
	ctx := context.Background()
	first := domain.Person{Name: "First", Color: "#111111", CalendarToken: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Active: true}
	second := domain.Person{Name: "Second", Color: "#222222", CalendarToken: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Active: true}
	for _, person := range []*domain.Person{&first, &second} {
		if err := repository.CreatePerson(ctx, person); err != nil {
			t.Fatal(err)
		}
	}
	resolved, err := repository.GetActivePersonByCalendarToken(ctx, first.CalendarToken)
	if err != nil || resolved.ID != first.ID {
		t.Fatalf("resolved person = %#v, error = %v", resolved, err)
	}
	chore := domain.Chore{Name: "Dishes", Active: true}
	if err := repository.CreateChore(ctx, &chore); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, time.July, 31, 0, 0, 0, 0, time.UTC)
	firstSchedule := domain.Schedule{ChoreID: chore.ID, Rule: domain.RecurrenceRule{Type: domain.RuleDaily}, StartDate: start, AssignmentMode: domain.AssignmentFixed, FixedPersonID: &first.ID, Active: true}
	secondSchedule := firstSchedule
	secondSchedule.FixedPersonID = &second.ID
	for _, schedule := range []*domain.Schedule{&firstSchedule, &secondSchedule} {
		if err := repository.CreateSchedule(ctx, schedule); err != nil {
			t.Fatal(err)
		}
	}
	instances := []domain.ChoreInstance{
		{ChoreID: chore.ID, ScheduleID: firstSchedule.ID, SequenceNo: 1, DueDate: start.AddDate(0, 0, -1), AssignedPersonID: first.ID, Status: domain.InstancePending},
		{ChoreID: chore.ID, ScheduleID: firstSchedule.ID, SequenceNo: 2, DueDate: start, AssignedPersonID: first.ID, Status: domain.InstanceSkipped},
		{ChoreID: chore.ID, ScheduleID: firstSchedule.ID, SequenceNo: 3, DueDate: start.AddDate(0, 0, 2), AssignedPersonID: first.ID, Status: domain.InstancePending},
		{ChoreID: chore.ID, ScheduleID: secondSchedule.ID, SequenceNo: 1, DueDate: start, AssignedPersonID: second.ID, Status: domain.InstancePending},
	}
	for index := range instances {
		if err := repository.CreateInstance(ctx, &instances[index]); err != nil {
			t.Fatal(err)
		}
	}
	items, err := repository.ListScheduledInstancesForPerson(ctx, first.ID, start, start.AddDate(0, 0, 2))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Instance.ID != instances[1].ID || items[1].Instance.ID != instances[2].ID {
		t.Errorf("person-scoped items = %#v", items)
	}
	if err := repository.DeactivatePerson(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.GetActivePersonByCalendarToken(ctx, first.CalendarToken); !errors.Is(err, ErrNotFound) {
		t.Errorf("inactive token lookup error = %v", err)
	}
	if _, err := repository.GetActivePersonByCalendarToken(ctx, strings.Repeat("c", 32)); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown token lookup error = %v", err)
	}
}
