package service

import (
	"context"
	"testing"
	"time"

	"github.com/dcrespo1/kinops/internal/domain"
	"github.com/dcrespo1/kinops/internal/store"
	"github.com/dcrespo1/kinops/internal/testutil"
)

func TestCreateEventMaterializesDailyAgendaWithAudience(t *testing.T) {
	db := testutil.NewTestDatabase(t)
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 31, 9, 0, 0, 0, location)
	service := NewWithClock(store.NewSQLite(db), location, func() time.Time { return now })
	person, err := service.CreatePerson(context.Background(), domain.Person{Name: "Dylan", Color: "#123456"})
	if err != nil {
		t.Fatal(err)
	}
	event, err := service.CreateEvent(context.Background(), domain.HouseholdEvent{
		Title: "Dentist", Location: "Main Street", StartDate: dateIn(t, location, "2026-08-01"), EndDate: dateIn(t, location, "2026-08-01"), StartTime: "09:30", EndTime: "10:15", Rule: domain.EventRecurrenceRule{Type: domain.EventRuleOneOff}, AudiencePersonIDs: []int64{person.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if event.ID == 0 {
		t.Fatal("event ID was not assigned")
	}
	view, err := service.DailyView(context.Background(), dateIn(t, location, "2026-08-01"))
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Events) != 1 {
		t.Fatalf("events = %#v", view.Events)
	}
	if view.Events[0].Event.Title != "Dentist" || len(view.Events[0].Audience) != 1 || view.Events[0].Audience[0].ID != person.ID {
		t.Errorf("scheduled event = %#v", view.Events[0])
	}
	if got := view.Events[0].Occurrence.StartAt.In(location).Format("15:04"); got != "09:30" {
		t.Errorf("start = %s", got)
	}
}

func TestUpdateEventPreservesPastAndRegeneratesFuture(t *testing.T) {
	db := testutil.NewTestDatabase(t)
	location := time.UTC
	now := dateIn(t, location, "2026-08-01")
	service := NewWithClock(store.NewSQLite(db), location, func() time.Time { return now })
	event, err := service.CreateEvent(context.Background(), domain.HouseholdEvent{Title: "Vacation", AllDay: true, StartDate: now, EndDate: now.AddDate(0, 0, 1), Rule: domain.EventRecurrenceRule{Type: domain.EventRuleDaily}})
	if err != nil {
		t.Fatal(err)
	}
	now = dateIn(t, location, "2026-08-03")
	event.Title = "Trip"
	event.StartDate = dateIn(t, location, "2026-08-05")
	event.EndDate = dateIn(t, location, "2026-08-06")
	event.Rule = domain.EventRecurrenceRule{Type: domain.EventRuleOneOff}
	if err := service.UpdateEvent(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(`SELECT start_date FROM event_occurrences WHERE event_id = ? ORDER BY start_date`, event.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var dates []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			t.Fatal(err)
		}
		dates = append(dates, value)
	}
	want := []string{"2026-08-01", "2026-08-02", "2026-08-05"}
	if len(dates) != len(want) {
		t.Fatalf("dates = %#v, want %#v", dates, want)
	}
	for i := range want {
		if dates[i] != want[i] {
			t.Errorf("date[%d] = %s, want %s", i, dates[i], want[i])
		}
	}
}

func TestCreateEventRejectsCategoryOutsideCuratedList(t *testing.T) {
	db := testutil.NewTestDatabase(t)
	now := dateIn(t, time.UTC, "2026-08-01")
	service := NewWithClock(store.NewSQLite(db), time.UTC, func() time.Time { return now })
	_, err := service.CreateEvent(context.Background(), domain.HouseholdEvent{
		Title: "Mystery", Category: "Something new", AllDay: true,
		StartDate: now, EndDate: now.AddDate(0, 0, 1),
		Rule: domain.EventRecurrenceRule{Type: domain.EventRuleOneOff},
	})
	if err == nil {
		t.Fatal("CreateEvent() accepted a category outside the curated list")
	}
}

func dateIn(t *testing.T, location *time.Location, value string) time.Time {
	t.Helper()
	parsed, err := time.ParseInLocation("2006-01-02", value, location)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
