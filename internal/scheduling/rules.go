package scheduling

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/dcrespo1/kinops/internal/domain"
)

const DateLayout = "2006-01-02"

func ParseDate(value string, location *time.Location) (time.Time, error) {
	date, err := time.ParseInLocation(DateLayout, value, location)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse date %q: %w", value, err)
	}
	if date.Format(DateLayout) != value {
		return time.Time{}, fmt.Errorf("date %q must use YYYY-MM-DD", value)
	}
	return date, nil
}

func Date(value time.Time, location *time.Location) time.Time {
	local := value.In(location)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
}

// RebaseDate preserves a date's calendar fields while applying a location. It
// is used for date-only values loaded from SQLite, which otherwise parse in UTC.
func RebaseDate(value time.Time, location *time.Location) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, location)
}

func ValidateRule(rule domain.RecurrenceRule) error {
	switch rule.Type {
	case domain.RuleDaily, domain.RuleOneOff:
		if rule.Interval != 0 || len(rule.Weekdays) != 0 || rule.DayOfMonth != 0 {
			return fmt.Errorf("%s rule does not accept parameters", rule.Type)
		}
	case domain.RuleEveryNDays:
		if rule.Interval < 1 || rule.Interval > 3650 {
			return errors.New("every-N-days interval must be between 1 and 3650")
		}
		if len(rule.Weekdays) != 0 || rule.DayOfMonth != 0 {
			return errors.New("every-N-days rule contains unrelated parameters")
		}
	case domain.RuleWeeklyDays:
		if len(rule.Weekdays) == 0 {
			return errors.New("weekly rule requires at least one weekday")
		}
		seen := make(map[time.Weekday]bool, len(rule.Weekdays))
		for _, day := range rule.Weekdays {
			if day < time.Sunday || day > time.Saturday {
				return fmt.Errorf("invalid weekday %d", day)
			}
			if seen[day] {
				return fmt.Errorf("duplicate weekday %d", isoWeekday(day))
			}
			seen[day] = true
		}
		if rule.Interval != 0 || rule.DayOfMonth != 0 {
			return errors.New("weekly rule contains unrelated parameters")
		}
	case domain.RuleMonthlyDay:
		if rule.DayOfMonth < 1 || rule.DayOfMonth > 31 {
			return errors.New("monthly day must be between 1 and 31")
		}
		if rule.Interval != 0 || len(rule.Weekdays) != 0 {
			return errors.New("monthly rule contains unrelated parameters")
		}
	default:
		return fmt.Errorf("unsupported recurrence rule %q", rule.Type)
	}
	return nil
}

func ValidateSchedule(schedule domain.Schedule) error {
	if schedule.ChoreID <= 0 {
		return errors.New("chore is required")
	}
	if schedule.StartDate.IsZero() {
		return errors.New("start date is required")
	}
	if schedule.EndDate != nil && schedule.EndDate.Before(schedule.StartDate) {
		return errors.New("end date cannot precede start date")
	}
	if err := ValidateRule(schedule.Rule); err != nil {
		return fmt.Errorf("validate recurrence rule: %w", err)
	}
	if schedule.Rule.Type == domain.RuleOneOff && schedule.EndDate != nil {
		return errors.New("one-off schedules cannot have an end date")
	}
	switch schedule.AssignmentMode {
	case domain.AssignmentFixed:
		if schedule.FixedPersonID == nil || *schedule.FixedPersonID <= 0 {
			return errors.New("fixed assignment requires a person")
		}
		if schedule.RotationStartPersonID != nil {
			return errors.New("fixed assignment cannot have a rotation start person")
		}
	case domain.AssignmentRotate:
		if schedule.RotationStartPersonID == nil || *schedule.RotationStartPersonID <= 0 {
			return errors.New("rotating assignment requires a starting person")
		}
		if schedule.FixedPersonID != nil {
			return errors.New("rotating assignment cannot have a fixed person")
		}
	default:
		return fmt.Errorf("unsupported assignment mode %q", schedule.AssignmentMode)
	}
	return nil
}

func MarshalRule(rule domain.RecurrenceRule) (string, error) {
	if err := ValidateRule(rule); err != nil {
		return "", err
	}
	var params any = struct{}{}
	switch rule.Type {
	case domain.RuleEveryNDays:
		params = struct {
			Interval int `json:"interval"`
		}{rule.Interval}
	case domain.RuleWeeklyDays:
		days := make([]int, len(rule.Weekdays))
		for index, day := range rule.Weekdays {
			days[index] = isoWeekday(day)
		}
		sort.Ints(days)
		params = struct {
			Weekdays []int `json:"weekdays"`
		}{days}
	case domain.RuleMonthlyDay:
		params = struct {
			Day int `json:"day"`
		}{rule.DayOfMonth}
	}
	encoded, err := json.Marshal(params)
	if err != nil {
		return "", fmt.Errorf("marshal recurrence rule: %w", err)
	}
	return string(encoded), nil
}

func UnmarshalRule(ruleType domain.RuleType, params string) (domain.RecurrenceRule, error) {
	rule := domain.RecurrenceRule{Type: ruleType}
	decoder := json.NewDecoder(bytes.NewBufferString(params))
	decoder.DisallowUnknownFields()
	switch ruleType {
	case domain.RuleDaily, domain.RuleOneOff:
		if err := decoder.Decode(&struct{}{}); err != nil {
			return domain.RecurrenceRule{}, fmt.Errorf("decode %s parameters: %w", ruleType, err)
		}
	case domain.RuleEveryNDays:
		var value struct {
			Interval int `json:"interval"`
		}
		if err := decoder.Decode(&value); err != nil {
			return domain.RecurrenceRule{}, fmt.Errorf("decode every-N-days parameters: %w", err)
		}
		rule.Interval = value.Interval
	case domain.RuleWeeklyDays:
		var value struct {
			Weekdays []int `json:"weekdays"`
		}
		if err := decoder.Decode(&value); err != nil {
			return domain.RecurrenceRule{}, fmt.Errorf("decode weekly parameters: %w", err)
		}
		for _, day := range value.Weekdays {
			weekday, err := weekdayFromISO(day)
			if err != nil {
				return domain.RecurrenceRule{}, err
			}
			rule.Weekdays = append(rule.Weekdays, weekday)
		}
	case domain.RuleMonthlyDay:
		var value struct {
			Day int `json:"day"`
		}
		if err := decoder.Decode(&value); err != nil {
			return domain.RecurrenceRule{}, fmt.Errorf("decode monthly parameters: %w", err)
		}
		rule.DayOfMonth = value.Day
	default:
		return domain.RecurrenceRule{}, fmt.Errorf("unsupported recurrence rule %q", ruleType)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return domain.RecurrenceRule{}, errors.New("unexpected trailing recurrence parameters")
	}
	if err := ValidateRule(rule); err != nil {
		return domain.RecurrenceRule{}, err
	}
	return rule, nil
}

func isoWeekday(day time.Weekday) int {
	if day == time.Sunday {
		return 7
	}
	return int(day)
}

func weekdayFromISO(day int) (time.Weekday, error) {
	if day < 1 || day > 7 {
		return 0, fmt.Errorf("weekday must be between 1 and 7: %d", day)
	}
	if day == 7 {
		return time.Sunday, nil
	}
	return time.Weekday(day), nil
}
