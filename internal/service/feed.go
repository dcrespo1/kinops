package service

import (
	"context"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/dcrespo1/kinops/internal/domain"
	"github.com/dcrespo1/kinops/internal/scheduling"
	"github.com/dcrespo1/kinops/internal/store"
)

var ErrCalendarFeedNotFound = errors.New("calendar feed not found")

func (s *Service) CalendarFeed(ctx context.Context, token string) (domain.CalendarFeed, error) {
	if !validCalendarToken(token) {
		return domain.CalendarFeed{}, ErrCalendarFeedNotFound
	}
	person, err := s.repository.GetActivePersonByCalendarToken(ctx, token)
	if errors.Is(err, store.ErrNotFound) {
		return domain.CalendarFeed{}, ErrCalendarFeedNotFound
	}
	if err != nil {
		return domain.CalendarFeed{}, fmt.Errorf("resolve calendar feed owner: %w", err)
	}
	today := scheduling.Date(s.now(), s.location)
	through := today.AddDate(0, 0, 60)
	items, err := s.repository.ListScheduledInstancesForPerson(ctx, person.ID, today, through)
	if err != nil {
		return domain.CalendarFeed{}, fmt.Errorf("list calendar feed instances: %w", err)
	}
	feed := domain.CalendarFeed{Name: person.Name + " · KinOps", Events: make([]domain.CalendarFeedEvent, 0, len(items))}
	for _, item := range items {
		feed.Events = append(feed.Events, domain.CalendarFeedEvent{
			InstanceID:  item.Instance.ID,
			DueDate:     item.Instance.DueDate,
			Summary:     item.Chore.Name,
			Description: item.Chore.Description,
			Category:    item.Chore.Category,
			UpdatedAt:   item.Instance.UpdatedAt,
		})
	}
	return feed, nil
}

func validCalendarToken(token string) bool {
	if len(token) < 32 || len(token) > 128 || !utf8.ValidString(token) {
		return false
	}
	for _, character := range token {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f') || (character >= 'A' && character <= 'F')) {
			return false
		}
	}
	return true
}
