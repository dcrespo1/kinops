package pages

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/dcrespo1/kinops/internal/domain"
)

func TestScheduleFormDisablesInactiveConditionalControls(t *testing.T) {
	fixedPersonID := int64(1)
	data := ScheduleFormData{
		Schedule: domain.Schedule{
			ChoreID:        2,
			Rule:           domain.RecurrenceRule{Type: domain.RuleDaily},
			StartDate:      time.Date(2026, time.July, 31, 0, 0, 0, 0, time.UTC),
			AssignmentMode: domain.AssignmentFixed,
			FixedPersonID:  &fixedPersonID,
		},
		People: []domain.Person{{ID: 1, Name: "Dylan", Active: true}},
	}
	html := renderScheduleForm(t, data)
	interval := namedTag(t, html, "input", "interval")
	monthlyDay := namedTag(t, html, "input", "day")
	fixed := namedTag(t, html, "select", "fixed_person_id")
	rotating := namedTag(t, html, "select", "rotation_start_person_id")
	for name, tag := range map[string]string{"interval": interval, "monthly day": monthlyDay, "rotation": rotating} {
		if !hasBareDisabled(tag) {
			t.Errorf("inactive %s control is not initially disabled: %s", name, tag)
		}
		if !strings.Contains(tag, "x-bind:disabled=") {
			t.Errorf("%s control has no reactive disabled binding: %s", name, tag)
		}
	}
	if hasBareDisabled(fixed) || !strings.Contains(fixed, " required") {
		t.Errorf("active fixed assignment control = %s", fixed)
	}
	if !strings.Contains(interval, `value=""`) || !strings.Contains(monthlyDay, `value=""`) {
		t.Errorf("inactive numeric controls should not contain zero: %s %s", interval, monthlyDay)
	}
	weekly := regexp.MustCompile(`<fieldset[^>]*x-show="rule === 'weekly_days'"[^>]*>`).FindString(html)
	if weekly == "" || !hasBareDisabled(weekly) || !strings.Contains(weekly, "x-bind:disabled=") {
		t.Errorf("inactive weekly fieldset = %s", weekly)
	}
}

func TestScheduleFormEnablesOnlySelectedRecurrenceControl(t *testing.T) {
	personID := int64(1)
	data := ScheduleFormData{
		Schedule: domain.Schedule{
			ChoreID:        2,
			Rule:           domain.RecurrenceRule{Type: domain.RuleEveryNDays, Interval: 3},
			StartDate:      time.Date(2026, time.July, 31, 0, 0, 0, 0, time.UTC),
			AssignmentMode: domain.AssignmentFixed,
			FixedPersonID:  &personID,
		},
		People: []domain.Person{{ID: 1, Name: "Dylan", Active: true}},
	}
	html := renderScheduleForm(t, data)
	interval := namedTag(t, html, "input", "interval")
	monthlyDay := namedTag(t, html, "input", "day")
	if hasBareDisabled(interval) || !strings.Contains(interval, `value="3"`) || !strings.Contains(interval, " required") {
		t.Errorf("selected interval control = %s", interval)
	}
	if !hasBareDisabled(monthlyDay) {
		t.Errorf("inactive monthly control = %s", monthlyDay)
	}
}

func renderScheduleForm(t *testing.T, data ScheduleFormData) string {
	t.Helper()
	var output bytes.Buffer
	if err := ScheduleForm(data).Render(context.Background(), &output); err != nil {
		t.Fatalf("render ScheduleForm: %v", err)
	}
	return output.String()
}

func namedTag(t *testing.T, html, element, name string) string {
	t.Helper()
	pattern := regexp.MustCompile(`<` + element + `[^>]*name="` + regexp.QuoteMeta(name) + `"[^>]*>`)
	tag := pattern.FindString(html)
	if tag == "" {
		t.Fatalf("could not find %s named %s in %s", element, name, html)
	}
	return tag
}

func hasBareDisabled(tag string) bool {
	return regexp.MustCompile(`(?:\s|^)disabled(?:\s|>)`).MatchString(tag)
}
