package scheduling

import (
	"fmt"
	"time"

	"github.com/dcrespo1/kinops/internal/domain"
)

// Occurrences returns matching dates in the inclusive requested window.
func Occurrences(rule domain.RecurrenceRule, start time.Time, end *time.Time, windowStart, windowEnd time.Time) ([]time.Time, error) {
	if err := ValidateRule(rule); err != nil {
		return nil, err
	}
	if windowEnd.Before(windowStart) {
		return nil, nil
	}
	from := windowStart
	if start.After(from) {
		from = start
	}
	through := windowEnd
	if end != nil && end.Before(through) {
		through = *end
	}
	if through.Before(from) {
		return nil, nil
	}

	var dates []time.Time
	switch rule.Type {
	case domain.RuleDaily:
		for date := from; !date.After(through); date = date.AddDate(0, 0, 1) {
			dates = append(dates, date)
		}
	case domain.RuleEveryNDays:
		date := start
		for date.Before(from) {
			date = date.AddDate(0, 0, rule.Interval)
		}
		for !date.After(through) {
			dates = append(dates, date)
			date = date.AddDate(0, 0, rule.Interval)
		}
	case domain.RuleWeeklyDays:
		allowed := make(map[time.Weekday]bool, len(rule.Weekdays))
		for _, day := range rule.Weekdays {
			allowed[day] = true
		}
		for date := from; !date.After(through); date = date.AddDate(0, 0, 1) {
			if allowed[date.Weekday()] {
				dates = append(dates, date)
			}
		}
	case domain.RuleMonthlyDay:
		month := time.Date(from.Year(), from.Month(), 1, 0, 0, 0, 0, from.Location())
		for !month.After(through) {
			candidate := time.Date(month.Year(), month.Month(), rule.DayOfMonth, 0, 0, 0, 0, month.Location())
			if candidate.Month() == month.Month() && !candidate.Before(from) && !candidate.After(through) {
				dates = append(dates, candidate)
			}
			month = month.AddDate(0, 1, 0)
		}
	case domain.RuleOneOff:
		if !start.Before(from) && !start.After(through) {
			dates = append(dates, start)
		}
	default:
		return nil, fmt.Errorf("unsupported recurrence rule %q", rule.Type)
	}
	return dates, nil
}
