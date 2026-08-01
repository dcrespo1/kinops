package service

import (
	"context"
	"testing"
	"time"

	"github.com/dcrespo1/kinops/internal/domain"
	"github.com/dcrespo1/kinops/internal/store"
	"github.com/dcrespo1/kinops/internal/testutil"
)

func TestFixedScheduleGenerationAndRegeneration(t *testing.T) {
	db := testutil.NewTestDatabase(t)
	repository := store.NewSQLite(db)
	now := time.Date(2026, time.July, 31, 15, 0, 0, 0, time.UTC)
	svc := NewWithClock(repository, time.UTC, func() time.Time { return now })
	ctx := context.Background()

	first := createTestPerson(t, svc, "Dylan", "#111111")
	second := createTestPerson(t, svc, "Wife", "#222222")
	chore, err := svc.CreateChore(ctx, domain.Chore{Name: "Dishes"})
	if err != nil {
		t.Fatal(err)
	}
	end := now.AddDate(0, 0, 2)
	schedule, err := svc.CreateSchedule(ctx, domain.Schedule{
		ChoreID:        chore.ID,
		Rule:           domain.RecurrenceRule{Type: domain.RuleDaily},
		StartDate:      now,
		EndDate:        &end,
		AssignmentMode: domain.AssignmentFixed,
		FixedPersonID:  &first.ID,
	})
	if err != nil {
		t.Fatalf("CreateSchedule() error = %v", err)
	}
	instances, err := repository.ListInstancesBySchedule(ctx, schedule.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(instances) != 3 {
		t.Fatalf("generated %d instances, want 3", len(instances))
	}
	for _, instance := range instances {
		if instance.AssignedPersonID != first.ID {
			t.Errorf("assignee = %d, want %d", instance.AssignedPersonID, first.ID)
		}
	}

	if _, err := db.Exec(`UPDATE chore_instances SET status = 'done', completed_at = ? WHERE id = ?`, now.UTC().Format(time.RFC3339Nano), instances[0].ID); err != nil {
		t.Fatal(err)
	}
	schedule.FixedPersonID = &second.ID
	if err := svc.UpdateSchedule(ctx, schedule); err != nil {
		t.Fatalf("UpdateSchedule() error = %v", err)
	}
	instances, err = repository.ListInstancesBySchedule(ctx, schedule.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(instances) != 3 {
		t.Fatalf("regenerated %d instances, want 3", len(instances))
	}
	if instances[0].Status != domain.InstanceDone || instances[0].AssignedPersonID != first.ID || instances[0].SequenceNo != 1 {
		t.Errorf("completed instance was changed: %#v", instances[0])
	}
	for _, instance := range instances[1:] {
		if instance.AssignedPersonID != second.ID {
			t.Errorf("regenerated assignee = %d, want %d", instance.AssignedPersonID, second.ID)
		}
	}
}

func TestRotatingScheduleIsStrictPerOccurrence(t *testing.T) {
	db := testutil.NewTestDatabase(t)
	repository := store.NewSQLite(db)
	now := time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC)
	svc := NewWithClock(repository, time.UTC, func() time.Time { return now })
	ctx := context.Background()
	first := createTestPerson(t, svc, "First", "#111111")
	second := createTestPerson(t, svc, "Second", "#222222")
	third := createTestPerson(t, svc, "Third", "#333333")
	chore, err := svc.CreateChore(ctx, domain.Chore{Name: "Vacuum"})
	if err != nil {
		t.Fatal(err)
	}
	end := now.AddDate(0, 0, 3)
	schedule, err := svc.CreateSchedule(ctx, domain.Schedule{ChoreID: chore.ID, Rule: domain.RecurrenceRule{Type: domain.RuleDaily}, StartDate: now, EndDate: &end, AssignmentMode: domain.AssignmentRotate, RotationStartPersonID: &second.ID})
	if err != nil {
		t.Fatal(err)
	}
	instances, err := repository.ListInstancesBySchedule(ctx, schedule.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []int64{second.ID, third.ID, first.ID, second.ID}
	for index, expected := range want {
		if instances[index].AssignedPersonID != expected {
			t.Errorf("instance %d assignee = %d, want %d", index, instances[index].AssignedPersonID, expected)
		}
	}
	if err := svc.EnsureHorizon(ctx); err != nil {
		t.Fatal(err)
	}
	again, err := repository.ListInstancesBySchedule(ctx, schedule.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != len(instances) {
		t.Errorf("EnsureHorizon duplicated instances: got %d, want %d", len(again), len(instances))
	}
}

func TestAddingPersonRegeneratesPendingRotation(t *testing.T) {
	db := testutil.NewTestDatabase(t)
	repository := store.NewSQLite(db)
	now := time.Date(2026, time.August, 1, 12, 0, 0, 0, time.UTC)
	svc := NewWithClock(repository, time.UTC, func() time.Time { return now })
	ctx := context.Background()
	first := createTestPerson(t, svc, "First", "#111111")
	second := createTestPerson(t, svc, "Second", "#222222")
	chore, err := svc.CreateChore(ctx, domain.Chore{Name: "Kitchen"})
	if err != nil {
		t.Fatal(err)
	}
	end := now.AddDate(0, 0, 3)
	schedule, err := svc.CreateSchedule(ctx, domain.Schedule{ChoreID: chore.ID, Rule: domain.RecurrenceRule{Type: domain.RuleDaily}, StartDate: now, EndDate: &end, AssignmentMode: domain.AssignmentRotate, RotationStartPersonID: &first.ID})
	if err != nil {
		t.Fatal(err)
	}
	third := createTestPerson(t, svc, "Third", "#333333")
	instances, err := repository.ListInstancesBySchedule(ctx, schedule.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []int64{first.ID, second.ID, third.ID, first.ID}
	if len(instances) != len(want) {
		t.Fatalf("instances = %d, want %d", len(instances), len(want))
	}
	for index, expected := range want {
		if instances[index].AssignedPersonID != expected {
			t.Errorf("instance %d assignee = %d, want %d", index, instances[index].AssignedPersonID, expected)
		}
	}
}

func createTestPerson(t *testing.T, svc *Service, name, color string) domain.Person {
	t.Helper()
	person, err := svc.CreatePerson(context.Background(), domain.Person{Name: name, Color: color})
	if err != nil {
		t.Fatalf("CreatePerson() error = %v", err)
	}
	return person
}
