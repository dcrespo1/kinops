package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/dcrespo1/kinops/internal/domain"
	"github.com/dcrespo1/kinops/internal/events"
	"github.com/dcrespo1/kinops/internal/scheduling"
	"github.com/dcrespo1/kinops/internal/store"
)

func (s *Service) ListEvents(ctx context.Context) ([]domain.HouseholdEvent, error) {
	return s.repository.ListEvents(ctx, false)
}

func (s *Service) GetEvent(ctx context.Context, id int64) (domain.HouseholdEvent, error) {
	if id <= 0 {
		return domain.HouseholdEvent{}, store.ErrNotFound
	}
	return s.repository.GetEvent(ctx, id)
}

func (s *Service) CreateEvent(ctx context.Context, event domain.HouseholdEvent) (domain.HouseholdEvent, error) {
	event.Active = true
	if err := s.normalizeAndValidateEvent(ctx, &event); err != nil {
		return domain.HouseholdEvent{}, err
	}
	if !domain.IsEventCategory(event.Category) {
		return domain.HouseholdEvent{}, errors.New("choose a valid event category")
	}
	err := s.repository.WithinTx(ctx, func(tx *store.SQLite) error {
		if err := tx.CreateEvent(ctx, &event); err != nil {
			return err
		}
		if err := tx.ReplaceEventAudience(ctx, event.ID, event.AudiencePersonIDs); err != nil {
			return err
		}
		return s.generateEvent(ctx, tx, event)
	})
	return event, err
}

func (s *Service) UpdateEvent(ctx context.Context, event domain.HouseholdEvent) error {
	if err := s.normalizeAndValidateEvent(ctx, &event); err != nil {
		return err
	}
	if !domain.IsEventCategory(event.Category) {
		current, err := s.repository.GetEvent(ctx, event.ID)
		if err != nil {
			return err
		}
		if event.Category != current.Category {
			return errors.New("choose a valid event category")
		}
	}
	today := scheduling.Date(s.now(), s.location)
	return s.repository.WithinTx(ctx, func(tx *store.SQLite) error {
		current, err := tx.GetEvent(ctx, event.ID)
		if err != nil {
			return err
		}
		event.Active = current.Active
		if err := tx.UpdateEvent(ctx, event); err != nil {
			return err
		}
		if err := tx.ReplaceEventAudience(ctx, event.ID, event.AudiencePersonIDs); err != nil {
			return err
		}
		if err := tx.DeleteEventOccurrencesFrom(ctx, event.ID, today); err != nil {
			return err
		}
		return s.generateEvent(ctx, tx, event)
	})
}

func (s *Service) DeactivateEvent(ctx context.Context, id int64) error {
	today := scheduling.Date(s.now(), s.location)
	return s.repository.WithinTx(ctx, func(tx *store.SQLite) error {
		if _, err := tx.GetEvent(ctx, id); err != nil {
			return err
		}
		if err := tx.DeactivateEvent(ctx, id); err != nil {
			return err
		}
		return tx.DeleteEventOccurrencesFrom(ctx, id, today)
	})
}

func (s *Service) normalizeAndValidateEvent(ctx context.Context, event *domain.HouseholdEvent) error {
	event.Title = strings.TrimSpace(event.Title)
	event.Description = strings.TrimSpace(event.Description)
	event.Location = strings.TrimSpace(event.Location)
	event.Category = strings.TrimSpace(event.Category)
	if event.Category == "" {
		event.Category = domain.EventCategoryGeneral
	}
	event.StartTime = strings.TrimSpace(event.StartTime)
	event.EndTime = strings.TrimSpace(event.EndTime)
	event.StartDate = scheduling.Date(event.StartDate, s.location)
	event.EndDate = scheduling.Date(event.EndDate, s.location)
	if event.RecurrenceEndDate != nil {
		value := scheduling.Date(*event.RecurrenceEndDate, s.location)
		event.RecurrenceEndDate = &value
	}
	if len(event.Title) > 200 {
		return errors.New("title must be 200 characters or fewer")
	}
	if event.AllDay {
		event.StartTime, event.EndTime = "", ""
	}
	event.Rule.Weekdays = events.NormalizeWeekdays(event.Rule.Weekdays)
	if err := events.Validate(*event, s.location); err != nil {
		return err
	}
	people, err := s.repository.ListPeople(ctx, false)
	if err != nil {
		return fmt.Errorf("list event audience people: %w", err)
	}
	active := make(map[int64]bool, len(people))
	for _, person := range people {
		active[person.ID] = true
	}
	seen := map[int64]bool{}
	ids := make([]int64, 0, len(event.AudiencePersonIDs))
	for _, id := range event.AudiencePersonIDs {
		if !active[id] {
			return errors.New("event audience contains an unknown or inactive person")
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	event.AudiencePersonIDs = ids
	return nil
}

func (s *Service) generateEvent(ctx context.Context, repository store.Events, event domain.HouseholdEvent) error {
	today := scheduling.Date(s.now(), s.location)
	through := today.AddDate(0, 0, events.HorizonDays)
	occurrences, err := events.Generate(event, today, through, s.location)
	if err != nil {
		return err
	}
	for i := range occurrences {
		if err := repository.CreateEventOccurrence(ctx, &occurrences[i]); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) EnsureEventHorizon(ctx context.Context) error {
	allEvents, err := s.repository.ListEvents(ctx, false)
	if err != nil {
		return err
	}
	for _, event := range allEvents {
		if err := s.repository.WithinTx(ctx, func(tx *store.SQLite) error {
			return s.generateEvent(ctx, tx, event)
		}); err != nil {
			return fmt.Errorf("ensure event %d horizon: %w", event.ID, err)
		}
	}
	return nil
}
