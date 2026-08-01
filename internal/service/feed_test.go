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

func TestCalendarFeedUsesPersistedPersonAssignmentsAndInclusiveHorizon(t *testing.T) {
	db := testutil.NewTestDatabase(t)
	repository := store.NewSQLite(db)
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.March, 8, 12, 0, 0, 0, location)
	svc := NewWithClock(repository, location, func() time.Time { return now })
	ctx := context.Background()
	person := domain.Person{Name: "Dylan", Color: "#123456", CalendarToken: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Active: true}
	other := domain.Person{Name: "Amanda", Color: "#654321", CalendarToken: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Active: true}
	for _, candidate := range []*domain.Person{&person, &other} {
		if err := repository.CreatePerson(ctx, candidate); err != nil {
			t.Fatal(err)
		}
	}
	chore := domain.Chore{Name: "Dishes", Description: "Wash up", Category: "Kitchen", Active: true}
	if err := repository.CreateChore(ctx, &chore); err != nil {
		t.Fatal(err)
	}
	schedule := domain.Schedule{ChoreID: chore.ID, Rule: domain.RecurrenceRule{Type: domain.RuleDaily}, StartDate: now, AssignmentMode: domain.AssignmentFixed, FixedPersonID: &person.ID, Active: true}
	if err := repository.CreateSchedule(ctx, &schedule); err != nil {
		t.Fatal(err)
	}
	completedAt := now.UTC()
	instances := []domain.ChoreInstance{
		{ChoreID: chore.ID, ScheduleID: schedule.ID, SequenceNo: 1, DueDate: now.AddDate(0, 0, -1), AssignedPersonID: person.ID, Status: domain.InstancePending},
		{ChoreID: chore.ID, ScheduleID: schedule.ID, SequenceNo: 2, DueDate: now, AssignedPersonID: person.ID, Status: domain.InstanceDone, CompletedAt: &completedAt},
		{ChoreID: chore.ID, ScheduleID: schedule.ID, SequenceNo: 3, DueDate: now.AddDate(0, 0, 60), AssignedPersonID: person.ID, Status: domain.InstanceSkipped},
		{ChoreID: chore.ID, ScheduleID: schedule.ID, SequenceNo: 4, DueDate: now.AddDate(0, 0, 61), AssignedPersonID: person.ID, Status: domain.InstancePending},
		{ChoreID: chore.ID, ScheduleID: schedule.ID, SequenceNo: 5, DueDate: now.AddDate(0, 0, 1), AssignedPersonID: other.ID, Status: domain.InstancePending},
	}
	for index := range instances {
		if err := repository.CreateInstance(ctx, &instances[index]); err != nil {
			t.Fatal(err)
		}
	}

	feed, err := svc.CalendarFeed(ctx, person.CalendarToken)
	if err != nil {
		t.Fatal(err)
	}
	if feed.Name != "Dylan · KinOps" || len(feed.Events) != 2 {
		t.Fatalf("feed = %#v", feed)
	}
	if feed.Events[0].InstanceID != instances[1].ID || feed.Events[1].InstanceID != instances[2].ID {
		t.Errorf("event IDs = %d, %d", feed.Events[0].InstanceID, feed.Events[1].InstanceID)
	}
	if got := feed.Events[1].DueDate.Format("2006-01-02"); got != "2026-05-07" {
		t.Errorf("DST-safe horizon date = %s, want 2026-05-07", got)
	}
	if feed.Events[0].Summary != "Dishes" || feed.Events[0].Description != "Wash up" || feed.Events[0].Category != "Kitchen" {
		t.Errorf("event metadata = %#v", feed.Events[0])
	}
}

func TestCalendarFeedRejectsInvalidUnknownAndInactiveTokens(t *testing.T) {
	repository := store.NewSQLite(testutil.NewTestDatabase(t))
	svc := NewWithClock(repository, time.UTC, func() time.Time { return time.Date(2026, time.July, 31, 0, 0, 0, 0, time.UTC) })
	ctx := context.Background()
	person := domain.Person{Name: "Dylan", Color: "#123456", CalendarToken: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Active: true}
	if err := repository.CreatePerson(ctx, &person); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"short", "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"} {
		if _, err := svc.CalendarFeed(ctx, token); !errors.Is(err, ErrCalendarFeedNotFound) {
			t.Errorf("CalendarFeed(%q) error = %v", token, err)
		}
	}
	if err := repository.DeactivatePerson(ctx, person.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CalendarFeed(ctx, person.CalendarToken); !errors.Is(err, ErrCalendarFeedNotFound) {
		t.Errorf("inactive CalendarFeed() error = %v", err)
	}
}
