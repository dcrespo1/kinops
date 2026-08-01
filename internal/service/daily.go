package service

import (
	"context"
	"fmt"
	"time"

	"github.com/dcrespo1/kinops/internal/domain"
	"github.com/dcrespo1/kinops/internal/scheduling"
	"github.com/dcrespo1/kinops/internal/store"
)

func (s *Service) DailyView(ctx context.Context, date time.Time) (domain.DailyView, error) {
	date = scheduling.RebaseDate(date, s.location)
	people, err := s.repository.ListPeople(ctx, false)
	if err != nil {
		return domain.DailyView{}, fmt.Errorf("list daily people: %w", err)
	}
	items, err := s.repository.ListDailyInstances(ctx, date)
	if err != nil {
		return domain.DailyView{}, err
	}
	eventItems, err := s.repository.ListScheduledEvents(ctx, date, date)
	if err != nil {
		return domain.DailyView{}, fmt.Errorf("list daily events: %w", err)
	}
	view := domain.DailyView{Date: date, People: make([]domain.PersonDay, 0, len(people)), Events: eventItems}
	personIndex := make(map[int64]int, len(people))
	for _, person := range people {
		personIndex[person.ID] = len(view.People)
		view.People = append(view.People, domain.PersonDay{Person: person})
	}
	dateValue := date.Format(scheduling.DateLayout)
	for _, item := range items {
		index, exists := personIndex[item.Assignee.ID]
		if !exists {
			index = len(view.People)
			personIndex[item.Assignee.ID] = index
			view.People = append(view.People, domain.PersonDay{Person: item.Assignee})
		}
		if item.Instance.DueDate.Format(scheduling.DateLayout) < dateValue {
			view.People[index].Overdue = append(view.People[index].Overdue, item)
		} else {
			view.People[index].Today = append(view.People[index].Today, item)
		}
	}
	return view, nil
}

func (s *Service) CompleteInstance(ctx context.Context, id int64) (domain.DailyInstance, error) {
	return s.transitionInstance(ctx, id, domain.InstancePending, domain.InstanceDone, domain.EventCompleted)
}

func (s *Service) ReopenInstance(ctx context.Context, id int64) (domain.DailyInstance, error) {
	return s.transitionInstance(ctx, id, domain.InstanceDone, domain.InstancePending, domain.EventReopened)
}

func (s *Service) transitionInstance(ctx context.Context, id int64, from, to domain.InstanceStatus, event domain.CompletionEventType) (domain.DailyInstance, error) {
	if id <= 0 {
		return domain.DailyInstance{}, fmt.Errorf("instance ID must be positive: %w", store.ErrNotFound)
	}
	occurredAt := s.now().UTC()
	err := s.repository.WithinTx(ctx, func(tx *store.SQLite) error {
		instance, err := tx.GetInstance(ctx, id)
		if err != nil {
			return err
		}
		if instance.Status == to {
			return nil
		}
		if instance.Status != from {
			return fmt.Errorf("cannot transition instance %d from %s to %s: %w", id, instance.Status, to, store.ErrConflict)
		}
		var completedAt *time.Time
		if to == domain.InstanceDone {
			completedAt = &occurredAt
		}
		changed, err := tx.TransitionInstance(ctx, id, from, to, completedAt, occurredAt)
		if err != nil {
			return err
		}
		if !changed {
			current, getErr := tx.GetInstance(ctx, id)
			if getErr == nil && current.Status == to {
				return nil
			}
			return fmt.Errorf("instance %d changed concurrently: %w", id, store.ErrConflict)
		}
		log := domain.CompletionLog{ChoreInstanceID: id, PersonID: instance.AssignedPersonID, EventType: event, OccurredAt: occurredAt}
		return tx.CreateCompletionLog(ctx, &log)
	})
	if err != nil {
		return domain.DailyInstance{}, err
	}
	item, err := s.repository.GetDailyInstance(ctx, id)
	if err != nil {
		return domain.DailyInstance{}, err
	}
	return item, nil
}
