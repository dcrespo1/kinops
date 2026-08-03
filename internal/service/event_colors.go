package service

import (
	"context"
	"fmt"

	"github.com/dcrespo1/kinops/internal/domain"
)

func (s *Service) colorScheduledEvents(ctx context.Context, items []domain.ScheduledEvent) error {
	if len(items) == 0 {
		return nil
	}
	settings, err := s.repository.GetHouseholdSettings(ctx)
	if err != nil {
		return fmt.Errorf("get household event color: %w", err)
	}
	for index := range items {
		items[index].Color = settings.HouseholdEventColor
		if len(items[index].Audience) == 1 {
			items[index].Color = items[index].Audience[0].Color
		}
	}
	return nil
}
