package service

import (
	"context"
	"fmt"
	"time"

	"github.com/dcrespo1/kinops/internal/domain"
	"github.com/dcrespo1/kinops/internal/scheduling"
)

func (s *Service) WeeklyView(ctx context.Context, containingDate time.Time) (domain.WeekView, error) {
	date := scheduling.RebaseDate(containingDate, s.location)
	start := date.AddDate(0, 0, -mondayOffset(date.Weekday()))
	end := start.AddDate(0, 0, 6)
	people, err := s.repository.ListPeople(ctx, false)
	if err != nil {
		return domain.WeekView{}, fmt.Errorf("list weekly people: %w", err)
	}
	items, err := s.repository.ListScheduledInstances(ctx, start, end)
	if err != nil {
		return domain.WeekView{}, err
	}
	eventItems, err := s.repository.ListScheduledEvents(ctx, start, end)
	if err != nil {
		return domain.WeekView{}, fmt.Errorf("list weekly events: %w", err)
	}
	if err := s.colorScheduledEvents(ctx, eventItems); err != nil {
		return domain.WeekView{}, err
	}
	eventsByDate := groupEventsByDate(eventItems, start, end, s.location)
	today := scheduling.Date(s.now(), s.location)
	view := domain.WeekView{
		StartDate:  start,
		EndDate:    end,
		Today:      today,
		HorizonEnd: today.AddDate(0, 0, 60),
		People:     make([]domain.PersonWeek, 0, len(people)),
		EventDays:  make([]domain.WeekEventDay, 7),
	}
	for index := range view.EventDays {
		date := start.AddDate(0, 0, index)
		view.EventDays[index] = domain.WeekEventDay{Date: date, Events: eventsByDate[date.Format(scheduling.DateLayout)]}
	}
	personIndex := make(map[int64]int, len(people))
	for _, person := range people {
		personIndex[person.ID] = len(view.People)
		view.People = append(view.People, newPersonWeek(person, start))
	}
	for _, item := range items {
		index, exists := personIndex[item.Assignee.ID]
		if !exists {
			index = len(view.People)
			personIndex[item.Assignee.ID] = index
			view.People = append(view.People, newPersonWeek(item.Assignee, start))
		}
		dayIndex := int(item.Instance.DueDate.Sub(time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)).Hours() / 24)
		if dayIndex >= 0 && dayIndex < 7 {
			view.People[index].Days[dayIndex].Instances = append(view.People[index].Days[dayIndex].Instances, item)
		}
	}
	return view, nil
}

func (s *Service) MonthlyView(ctx context.Context, month time.Time) (domain.MonthView, error) {
	month = scheduling.RebaseDate(month, s.location)
	month = time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, s.location)
	gridStart := month.AddDate(0, 0, -mondayOffset(month.Weekday()))
	gridEnd := gridStart.AddDate(0, 0, 41)
	items, err := s.repository.ListScheduledInstances(ctx, gridStart, gridEnd)
	if err != nil {
		return domain.MonthView{}, err
	}
	byDate := make(map[string][]domain.ScheduledInstance)
	for _, item := range items {
		key := item.Instance.DueDate.Format(scheduling.DateLayout)
		byDate[key] = append(byDate[key], item)
	}
	eventItems, err := s.repository.ListScheduledEvents(ctx, gridStart, gridEnd)
	if err != nil {
		return domain.MonthView{}, fmt.Errorf("list monthly events: %w", err)
	}
	if err := s.colorScheduledEvents(ctx, eventItems); err != nil {
		return domain.MonthView{}, err
	}
	eventsByDate := groupEventsByDate(eventItems, gridStart, gridEnd, s.location)
	today := scheduling.Date(s.now(), s.location)
	view := domain.MonthView{
		Month:      month,
		Today:      today,
		GridStart:  gridStart,
		GridEnd:    gridEnd,
		HorizonEnd: today.AddDate(0, 0, 60),
		Weeks:      make([][]domain.MonthDay, 6),
	}
	date := gridStart
	for week := 0; week < 6; week++ {
		view.Weeks[week] = make([]domain.MonthDay, 7)
		for day := 0; day < 7; day++ {
			key := date.Format(scheduling.DateLayout)
			view.Weeks[week][day] = domain.MonthDay{
				Date:      date,
				InMonth:   date.Month() == month.Month(),
				Instances: byDate[key],
				Events:    eventsByDate[key],
			}
			date = date.AddDate(0, 0, 1)
		}
	}
	return view, nil
}

func groupEventsByDate(items []domain.ScheduledEvent, start, endInclusive time.Time, location *time.Location) map[string][]domain.ScheduledEvent {
	result := make(map[string][]domain.ScheduledEvent)
	endExclusive := endInclusive.AddDate(0, 0, 1)
	for _, item := range items {
		itemStart := scheduling.RebaseDate(item.Occurrence.StartDate, location)
		if itemStart.Before(start) {
			itemStart = start
		}
		itemEnd := scheduling.RebaseDate(item.Occurrence.EndDate, location)
		if itemEnd.After(endExclusive) {
			itemEnd = endExclusive
		}
		for date := itemStart; date.Before(itemEnd); date = date.AddDate(0, 0, 1) {
			key := date.Format(scheduling.DateLayout)
			result[key] = append(result[key], item)
		}
	}
	return result
}

func mondayOffset(day time.Weekday) int {
	return (int(day) + 6) % 7
}

func newPersonWeek(person domain.Person, start time.Time) domain.PersonWeek {
	week := domain.PersonWeek{Person: person, Days: make([]domain.WeekDay, 7)}
	for index := range week.Days {
		week.Days[index].Date = start.AddDate(0, 0, index)
	}
	return week
}
