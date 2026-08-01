package scheduling

import (
	"testing"
	"time"

	"github.com/dcrespo1/kinops/internal/domain"
)

func TestOccurrences(t *testing.T) {
	location := time.UTC
	date := func(value string) time.Time {
		result, err := ParseDate(value, location)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	tests := []struct {
		name  string
		rule  domain.RecurrenceRule
		start string
		from  string
		to    string
		want  []string
	}{
		{"daily", domain.RecurrenceRule{Type: domain.RuleDaily}, "2026-01-01", "2026-01-03", "2026-01-05", []string{"2026-01-03", "2026-01-04", "2026-01-05"}},
		{"every N anchored to start", domain.RecurrenceRule{Type: domain.RuleEveryNDays, Interval: 3}, "2026-01-01", "2026-01-03", "2026-01-10", []string{"2026-01-04", "2026-01-07", "2026-01-10"}},
		{"weekly sorted", domain.RecurrenceRule{Type: domain.RuleWeeklyDays, Weekdays: []time.Weekday{time.Wednesday, time.Monday}}, "2026-01-01", "2026-01-01", "2026-01-12", []string{"2026-01-05", "2026-01-07", "2026-01-12"}},
		{"monthly skips missing day", domain.RecurrenceRule{Type: domain.RuleMonthlyDay, DayOfMonth: 31}, "2026-01-01", "2026-01-01", "2026-04-30", []string{"2026-01-31", "2026-03-31"}},
		{"one off", domain.RecurrenceRule{Type: domain.RuleOneOff}, "2026-02-15", "2026-02-01", "2026-02-28", []string{"2026-02-15"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Occurrences(tt.rule, date(tt.start), nil, date(tt.from), date(tt.to))
			if err != nil {
				t.Fatalf("Occurrences() error = %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("Occurrences() count = %d, want %d: %v", len(got), len(tt.want), got)
			}
			for index := range got {
				if value := got[index].Format(DateLayout); value != tt.want[index] {
					t.Errorf("occurrence %d = %s, want %s", index, value, tt.want[index])
				}
			}
		})
	}
}

func TestOccurrencesAcrossDSTUsesCalendarDates(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	start, _ := ParseDate("2026-03-07", location)
	end, _ := ParseDate("2026-03-10", location)
	got, err := Occurrences(domain.RecurrenceRule{Type: domain.RuleDaily}, start, nil, start, end)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("got %d occurrences, want 4", len(got))
	}
	for index, value := range []string{"2026-03-07", "2026-03-08", "2026-03-09", "2026-03-10"} {
		if got[index].Format(DateLayout) != value {
			t.Errorf("occurrence %d = %s, want %s", index, got[index].Format(DateLayout), value)
		}
	}
}
