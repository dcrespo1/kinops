package ical

import (
	"bytes"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/dcrespo1/kinops/internal/domain"
)

func TestWriteCalendar(t *testing.T) {
	stamp := time.Date(2026, time.July, 31, 14, 15, 16, 0, time.FixedZone("EDT", -4*60*60))
	feed := domain.CalendarFeed{Name: "Dylan, KinOps", Events: []domain.CalendarFeedEvent{
		{InstanceID: 2, DueDate: time.Date(2026, time.August, 2, 0, 0, 0, 0, time.UTC), Summary: "Mop"},
		{InstanceID: 1, DueDate: time.Date(2026, time.July, 31, 0, 0, 0, 0, time.UTC), Summary: "Dishes, pans; cups", Description: "Wash\r\nDry \\ rack", Category: "Kitchen, daily", UpdatedAt: stamp},
	}}
	var output bytes.Buffer
	if err := Write(&output, feed); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	got := output.String()
	for _, want := range []string{
		"BEGIN:VCALENDAR\r\nVERSION:2.0\r\n",
		"PRODID:-//KinOps//Household Calendar//EN\r\n",
		"X-WR-CALNAME:Dylan\\, KinOps\r\n",
		"UID:chore-instance-1@kinops.local\r\n",
		"DTSTAMP:20260731T181516Z\r\n",
		"DTSTART;VALUE=DATE:20260731\r\nDTEND;VALUE=DATE:20260801\r\n",
		"SUMMARY:Dishes\\, pans\\; cups\r\n",
		"DESCRIPTION:Wash\\nDry \\\\ rack\r\n",
		"CATEGORIES:Kitchen\\, daily\r\n",
		"END:VCALENDAR\r\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("calendar does not contain %q:\n%s", want, got)
		}
	}
	if strings.Index(got, "chore-instance-1") > strings.Index(got, "chore-instance-2") {
		t.Error("events are not ordered by due date and ID")
	}
	if strings.Contains(strings.ReplaceAll(got, "\r\n", ""), "\n") {
		t.Error("calendar contains a bare LF")
	}
}

func TestWriteFoldsUTF8LinesAt75Octets(t *testing.T) {
	feed := domain.CalendarFeed{Name: "KinOps", Events: []domain.CalendarFeedEvent{{
		InstanceID: 7,
		DueDate:    time.Date(2026, time.July, 31, 0, 0, 0, 0, time.UTC),
		Summary:    strings.Repeat("é", 45) + " finish",
	}}}
	var output bytes.Buffer
	if err := Write(&output, feed); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(output.String(), "\r\n"), "\r\n")
	foundFold := false
	for _, line := range lines {
		if len(line) > 75 {
			t.Errorf("physical line is %d octets: %q", len(line), line)
		}
		if !utf8.ValidString(line) {
			t.Errorf("physical line splits UTF-8: %q", line)
		}
		if strings.HasPrefix(line, " ") {
			foundFold = true
		}
	}
	if !foundFold {
		t.Fatal("expected a folded content line")
	}
	unfolded := strings.ReplaceAll(output.String(), "\r\n ", "")
	if !strings.Contains(unfolded, "SUMMARY:"+feed.Events[0].Summary+"\r\n") {
		t.Errorf("unfolded summary was changed: %q", unfolded)
	}
}

func TestWriteEmptyCalendar(t *testing.T) {
	var output bytes.Buffer
	if err := Write(&output, domain.CalendarFeed{Name: "KinOps"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "BEGIN:VEVENT") || !strings.HasSuffix(output.String(), "END:VCALENDAR\r\n") {
		t.Errorf("unexpected empty calendar: %q", output.String())
	}
}
