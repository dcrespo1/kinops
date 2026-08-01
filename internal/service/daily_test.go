package service

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/dcrespo1/kinops/internal/domain"
	"github.com/dcrespo1/kinops/internal/store"
	"github.com/dcrespo1/kinops/internal/testutil"
)

func TestDailyViewGroupsTodayAndOverdueByPerson(t *testing.T) {
	db := testutil.NewTestDatabase(t)
	repository := store.NewSQLite(db)
	now := time.Date(2026, time.July, 31, 12, 0, 0, 0, time.UTC)
	svc := NewWithClock(repository, time.UTC, func() time.Time { return now })
	ctx := context.Background()
	first := createTestPerson(t, svc, "Dylan", "#111111")
	createTestPerson(t, svc, "Wife", "#222222")
	chore, err := svc.CreateChore(ctx, domain.Chore{Name: "Dishes", Category: "Kitchen"})
	if err != nil {
		t.Fatal(err)
	}
	schedule, err := svc.CreateSchedule(ctx, domain.Schedule{
		ChoreID: chore.ID, Rule: domain.RecurrenceRule{Type: domain.RuleOneOff},
		StartDate: now, AssignmentMode: domain.AssignmentFixed, FixedPersonID: &first.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	overdue := domain.ChoreInstance{
		ChoreID: chore.ID, ScheduleID: schedule.ID, SequenceNo: 2,
		DueDate: now.AddDate(0, 0, -1), AssignedPersonID: first.ID, Status: domain.InstancePending,
	}
	if err := repository.CreateInstance(ctx, &overdue); err != nil {
		t.Fatal(err)
	}

	view, err := svc.DailyView(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.People) != 2 {
		t.Fatalf("people = %d, want 2", len(view.People))
	}
	if len(view.People[0].Overdue) != 1 || len(view.People[0].Today) != 1 {
		t.Errorf("first person day = %#v", view.People[0])
	}
	if len(view.People[1].Overdue) != 0 || len(view.People[1].Today) != 0 {
		t.Errorf("second person day = %#v", view.People[1])
	}
}

func TestCompleteAndReopenInstanceAreLoggedAndIdempotent(t *testing.T) {
	db := testutil.NewTestDatabase(t)
	repository := store.NewSQLite(db)
	now := time.Date(2026, time.July, 31, 18, 30, 0, 0, time.UTC)
	svc := NewWithClock(repository, time.UTC, func() time.Time { return now })
	ctx := context.Background()
	person := createTestPerson(t, svc, "Dylan", "#111111")
	chore, err := svc.CreateChore(ctx, domain.Chore{Name: "Dishes"})
	if err != nil {
		t.Fatal(err)
	}
	schedule, err := svc.CreateSchedule(ctx, domain.Schedule{
		ChoreID: chore.ID, Rule: domain.RecurrenceRule{Type: domain.RuleOneOff},
		StartDate: now, AssignmentMode: domain.AssignmentFixed, FixedPersonID: &person.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	instances, err := repository.ListInstancesBySchedule(ctx, schedule.ID)
	if err != nil {
		t.Fatal(err)
	}
	id := instances[0].ID

	completed, err := svc.CompleteInstance(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Instance.Status != domain.InstanceDone || completed.Instance.CompletedAt == nil {
		t.Errorf("completed instance = %#v", completed.Instance)
	}
	if _, err := svc.CompleteInstance(ctx, id); err != nil {
		t.Fatalf("idempotent CompleteInstance() error = %v", err)
	}
	assertLogCount(t, db, id, "completed", 1)

	reopened, err := svc.ReopenInstance(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Instance.Status != domain.InstancePending || reopened.Instance.CompletedAt != nil {
		t.Errorf("reopened instance = %#v", reopened.Instance)
	}
	if _, err := svc.ReopenInstance(ctx, id); err != nil {
		t.Fatalf("idempotent ReopenInstance() error = %v", err)
	}
	assertLogCount(t, db, id, "reopened", 1)
}

func TestCreatePersonSupportsLargerHousehold(t *testing.T) {
	db := testutil.NewTestDatabase(t)
	svc := New(store.NewSQLite(db), time.UTC)
	createTestPerson(t, svc, "First", "#111111")
	createTestPerson(t, svc, "Second", "#222222")
	createTestPerson(t, svc, "Third", "#333333")
	createTestPerson(t, svc, "Fourth", "#444444")
	people, err := svc.ListPeople(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(people) != 4 {
		t.Fatalf("people = %d, want 4", len(people))
	}
}

func assertLogCount(t *testing.T, db interface{ QueryRow(string, ...any) *sql.Row }, instanceID int64, event string, want int) {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM completion_logs WHERE chore_instance_id = ? AND event_type = ?`, instanceID, event).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Errorf("%s logs = %d, want %d", event, count, want)
	}
}
