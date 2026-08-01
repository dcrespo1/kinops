package scheduling

import (
	"reflect"
	"testing"
	"time"

	"github.com/dcrespo1/kinops/internal/domain"
)

func TestRuleJSONRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		rule domain.RecurrenceRule
		json string
	}{
		{"daily", domain.RecurrenceRule{Type: domain.RuleDaily}, `{}`},
		{"every N days", domain.RecurrenceRule{Type: domain.RuleEveryNDays, Interval: 3}, `{"interval":3}`},
		{"weekly", domain.RecurrenceRule{Type: domain.RuleWeeklyDays, Weekdays: []time.Weekday{time.Friday, time.Monday}}, `{"weekdays":[1,5]}`},
		{"monthly", domain.RecurrenceRule{Type: domain.RuleMonthlyDay, DayOfMonth: 31}, `{"day":31}`},
		{"one off", domain.RecurrenceRule{Type: domain.RuleOneOff}, `{}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := MarshalRule(tt.rule)
			if err != nil {
				t.Fatalf("MarshalRule() error = %v", err)
			}
			if encoded != tt.json {
				t.Fatalf("MarshalRule() = %s, want %s", encoded, tt.json)
			}
			decoded, err := UnmarshalRule(tt.rule.Type, encoded)
			if err != nil {
				t.Fatalf("UnmarshalRule() error = %v", err)
			}
			expected := tt.rule
			if expected.Type == domain.RuleWeeklyDays {
				expected.Weekdays = []time.Weekday{time.Monday, time.Friday}
			}
			if !reflect.DeepEqual(decoded, expected) {
				t.Errorf("round trip = %#v, want %#v", decoded, expected)
			}
		})
	}
}

func TestValidateRuleRejectsInvalidRules(t *testing.T) {
	tests := []domain.RecurrenceRule{
		{Type: "unknown"},
		{Type: domain.RuleEveryNDays, Interval: 0},
		{Type: domain.RuleWeeklyDays},
		{Type: domain.RuleWeeklyDays, Weekdays: []time.Weekday{time.Monday, time.Monday}},
		{Type: domain.RuleMonthlyDay, DayOfMonth: 32},
		{Type: domain.RuleDaily, Interval: 2},
	}
	for _, rule := range tests {
		if err := ValidateRule(rule); err == nil {
			t.Errorf("ValidateRule(%#v) returned nil", rule)
		}
	}
}

func TestUnmarshalRuleRejectsUnknownFields(t *testing.T) {
	if _, err := UnmarshalRule(domain.RuleEveryNDays, `{"interval":2,"extra":true}`); err == nil {
		t.Fatal("UnmarshalRule() returned nil error")
	}
}
