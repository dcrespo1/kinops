package service

import (
	"context"
	"fmt"
	"time"

	"github.com/dcrespo1/kinops/internal/domain"
	"github.com/dcrespo1/kinops/internal/scheduling"
)

const (
	calendarHorizonDays = 60
	activityLimit       = 10
)

func (s *Service) AdminDashboard(ctx context.Context) (domain.AdminDashboard, error) {
	now := s.now().In(s.location)
	today := scheduling.Date(now, s.location)
	people, err := s.repository.ListPeople(ctx, false)
	if err != nil {
		return domain.AdminDashboard{}, fmt.Errorf("list admin people: %w", err)
	}
	stats, err := s.repository.ListPersonDailyStats(ctx, time.Date(1, time.January, 1, 0, 0, 0, 0, s.location), today)
	if err != nil {
		return domain.AdminDashboard{}, err
	}
	overdue, err := s.repository.ListPersonOverdueCounts(ctx, today)
	if err != nil {
		return domain.AdminDashboard{}, err
	}
	activity, err := s.repository.ListRecentCompletionActivity(ctx, activityLimit)
	if err != nil {
		return domain.AdminDashboard{}, err
	}

	view := domain.AdminDashboard{
		GeneratedAt:    now,
		TimeZone:       s.location.String(),
		HorizonDays:    calendarHorizonDays,
		SevenDay:       domain.AnalyticsWindow{Days: 7},
		ThirtyDay:      domain.AnalyticsWindow{Days: 30},
		People:         make([]domain.PersonAnalytics, len(people)),
		Activity:       activity,
		CalendarPeople: people,
	}
	personIndex := make(map[int64]int, len(people))
	personDays := make(map[int64][]domain.PersonDailyStats, len(people))
	for index, person := range people {
		personIndex[person.ID] = index
		view.People[index] = domain.PersonAnalytics{Person: person, Overdue: overdue[person.ID]}
		view.Overdue += overdue[person.ID]
	}
	sevenStart := today.AddDate(0, 0, -6).Format(scheduling.DateLayout)
	thirtyStart := today.AddDate(0, 0, -29).Format(scheduling.DateLayout)
	for _, stat := range stats {
		index, active := personIndex[stat.PersonID]
		if !active {
			continue
		}
		personDays[stat.PersonID] = append(personDays[stat.PersonID], stat)
		date := stat.DueDate.Format(scheduling.DateLayout)
		if date >= thirtyStart {
			view.ThirtyDay.Assigned += stat.Assigned
			view.ThirtyDay.Completed += stat.Completed
			view.People[index].Assigned += stat.Assigned
			view.People[index].Completed += stat.Completed
		}
		if date >= sevenStart {
			view.SevenDay.Assigned += stat.Assigned
			view.SevenDay.Completed += stat.Completed
		}
	}
	view.SevenDay.RatePercent = completionRate(view.SevenDay.Completed, view.SevenDay.Assigned)
	view.ThirtyDay.RatePercent = completionRate(view.ThirtyDay.Completed, view.ThirtyDay.Assigned)
	for index := range view.People {
		view.People[index].RatePercent = completionRate(view.People[index].Completed, view.People[index].Assigned)
		view.People[index].Streak = currentStreak(personDays[view.People[index].Person.ID])
	}
	return view, nil
}

func (s *Service) RotateCalendarToken(ctx context.Context, personID int64) (domain.Person, error) {
	if personID <= 0 {
		return domain.Person{}, fmt.Errorf("person ID must be positive")
	}
	token, err := generateCalendarToken()
	if err != nil {
		return domain.Person{}, err
	}
	if err := s.repository.RotatePersonCalendarToken(ctx, personID, token); err != nil {
		return domain.Person{}, err
	}
	return s.repository.GetPerson(ctx, personID)
}

func completionRate(completed, assigned int) int {
	if assigned == 0 {
		return 0
	}
	return (completed*100 + assigned/2) / assigned
}

func currentStreak(days []domain.PersonDailyStats) int {
	streak := 0
	for index := len(days) - 1; index >= 0; index-- {
		if days[index].Assigned == 0 {
			continue
		}
		if days[index].Completed != days[index].Assigned {
			break
		}
		streak++
	}
	return streak
}
