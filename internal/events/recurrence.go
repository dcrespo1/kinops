package events

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dcrespo1/kinops/internal/domain"
	"github.com/dcrespo1/kinops/internal/scheduling"
)

const HorizonDays = 400

func Validate(event domain.HouseholdEvent, location *time.Location) error {
	if strings.TrimSpace(event.Title) == "" {
		return errors.New("title is required")
	}
	if event.StartDate.IsZero() || event.EndDate.IsZero() {
		return errors.New("start and end dates are required")
	}
	startDate := scheduling.RebaseDate(event.StartDate, location)
	endDate := scheduling.RebaseDate(event.EndDate, location)
	if event.AllDay {
		if !endDate.After(startDate) {
			return errors.New("all-day end date must be after the start date")
		}
		if event.StartTime != "" || event.EndTime != "" {
			return errors.New("all-day events cannot have start or end times")
		}
	} else {
		startAt, err := localDateTime(startDate, event.StartTime, location)
		if err != nil {
			return fmt.Errorf("start time: %w", err)
		}
		endAt, err := localDateTime(endDate, event.EndTime, location)
		if err != nil {
			return fmt.Errorf("end time: %w", err)
		}
		if !endAt.After(startAt) {
			return errors.New("event end must be after its start")
		}
	}
	if event.RecurrenceEndDate != nil && scheduling.RebaseDate(*event.RecurrenceEndDate, location).Before(startDate) {
		return errors.New("recurrence end date cannot be before the start date")
	}
	switch event.Rule.Type {
	case domain.EventRuleOneOff, domain.EventRuleDaily:
	case domain.EventRuleEveryNDays:
		if event.Rule.Interval < 1 || event.Rule.Interval > 3650 {
			return errors.New("recurrence interval must be between 1 and 3650")
		}
	case domain.EventRuleWeeklyDays:
		if len(event.Rule.Weekdays) == 0 {
			return errors.New("choose at least one weekday")
		}
		seen := map[time.Weekday]bool{}
		for _, day := range event.Rule.Weekdays {
			if day < time.Sunday || day > time.Saturday || seen[day] {
				return errors.New("weekdays must be unique valid days")
			}
			seen[day] = true
		}
	case domain.EventRuleMonthlyDay:
		if event.Rule.DayOfMonth < 1 || event.Rule.DayOfMonth > 31 {
			return errors.New("day of month must be between 1 and 31")
		}
	case domain.EventRuleAnnual:
		if event.Rule.Month < time.January || event.Rule.Month > time.December || event.Rule.DayOfMonth < 1 || event.Rule.DayOfMonth > 31 {
			return errors.New("annual recurrence requires a valid month and day")
		}
		if !validMonthDay(2000, event.Rule.Month, event.Rule.DayOfMonth, location) {
			return errors.New("annual recurrence month and day are invalid")
		}
	default:
		return fmt.Errorf("unsupported event recurrence type %q", event.Rule.Type)
	}
	return nil
}

func Generate(event domain.HouseholdEvent, from, through time.Time, location *time.Location) ([]domain.EventOccurrence, error) {
	if err := Validate(event, location); err != nil {
		return nil, err
	}
	from = scheduling.RebaseDate(from, location)
	through = scheduling.RebaseDate(through, location)
	start := scheduling.RebaseDate(event.StartDate, location)
	var dates []time.Time
	if event.Rule.Type == domain.EventRuleAnnual {
		for year := from.Year(); year <= through.Year(); year++ {
			if !validMonthDay(year, event.Rule.Month, event.Rule.DayOfMonth, location) {
				continue
			}
			candidate := time.Date(year, event.Rule.Month, event.Rule.DayOfMonth, 0, 0, 0, 0, location)
			if candidate.Before(start) || candidate.Before(from) || candidate.After(through) {
				continue
			}
			if event.RecurrenceEndDate != nil && candidate.After(scheduling.RebaseDate(*event.RecurrenceEndDate, location)) {
				continue
			}
			dates = append(dates, candidate)
		}
	} else {
		rule, err := choreRule(event.Rule)
		if err != nil {
			return nil, err
		}
		dates, err = scheduling.Occurrences(rule, start, event.RecurrenceEndDate, from, through)
		if err != nil {
			return nil, err
		}
	}
	occurrences := make([]domain.EventOccurrence, 0, len(dates))
	for _, date := range dates {
		occurrence, err := occurrenceForDate(event, date, location)
		if err != nil {
			return nil, err
		}
		occurrences = append(occurrences, occurrence)
	}
	return occurrences, nil
}

func occurrenceForDate(event domain.HouseholdEvent, date time.Time, location *time.Location) (domain.EventOccurrence, error) {
	baseStart := scheduling.RebaseDate(event.StartDate, location)
	baseEnd := scheduling.RebaseDate(event.EndDate, location)
	dayOffset := calendarDayOffset(baseStart, baseEnd)
	occurrence := domain.EventOccurrence{EventID: event.ID, StartDate: date}
	if event.AllDay {
		occurrence.EndDate = date.AddDate(0, 0, dayOffset)
		return occurrence, nil
	}
	startAt, err := localDateTime(date, event.StartTime, location)
	if err != nil {
		return domain.EventOccurrence{}, err
	}
	endLocalDate := date.AddDate(0, 0, dayOffset)
	endAt, err := localDateTime(endLocalDate, event.EndTime, location)
	if err != nil {
		return domain.EventOccurrence{}, err
	}
	startUTC, endUTC := startAt.UTC(), endAt.UTC()
	occurrence.StartAt = &startUTC
	occurrence.EndAt = &endUTC
	occurrence.EndDate = endLocalDate.AddDate(0, 0, 1)
	return occurrence, nil
}

func calendarDayOffset(start, end time.Time) int {
	days := 0
	for cursor := start; cursor.Before(end); cursor = cursor.AddDate(0, 0, 1) {
		days++
	}
	return days
}

func choreRule(rule domain.EventRecurrenceRule) (domain.RecurrenceRule, error) {
	result := domain.RecurrenceRule{Interval: rule.Interval, Weekdays: append([]time.Weekday(nil), rule.Weekdays...), DayOfMonth: rule.DayOfMonth}
	switch rule.Type {
	case domain.EventRuleOneOff:
		result.Type = domain.RuleOneOff
	case domain.EventRuleDaily:
		result.Type = domain.RuleDaily
	case domain.EventRuleEveryNDays:
		result.Type = domain.RuleEveryNDays
	case domain.EventRuleWeeklyDays:
		result.Type = domain.RuleWeeklyDays
	case domain.EventRuleMonthlyDay:
		result.Type = domain.RuleMonthlyDay
	default:
		return result, fmt.Errorf("unsupported event recurrence type %q", rule.Type)
	}
	return result, nil
}

func localDateTime(date time.Time, value string, location *time.Location) (time.Time, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return time.Time{}, errors.New("must use HH:MM")
	}
	hour, hourErr := strconv.Atoi(parts[0])
	minute, minuteErr := strconv.Atoi(parts[1])
	if hourErr != nil || minuteErr != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return time.Time{}, errors.New("must use a valid HH:MM time")
	}
	return time.Date(date.Year(), date.Month(), date.Day(), hour, minute, 0, 0, location), nil
}

func validMonthDay(year int, month time.Month, day int, location *time.Location) bool {
	candidate := time.Date(year, month, day, 0, 0, 0, 0, location)
	return candidate.Month() == month && candidate.Day() == day
}

func NormalizeWeekdays(days []time.Weekday) []time.Weekday {
	result := append([]time.Weekday(nil), days...)
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}
