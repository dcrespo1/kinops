package store

import (
	"context"
	"fmt"

	"github.com/dcrespo1/kinops/internal/domain"
)

func (s *SQLite) GetHouseholdSettings(ctx context.Context) (domain.HouseholdSettings, error) {
	var settings domain.HouseholdSettings
	var updated string
	if err := s.q.QueryRowContext(ctx, `
		SELECT household_event_color, updated_at
		FROM household_settings
		WHERE id = 1`).Scan(&settings.HouseholdEventColor, &updated); err != nil {
		return domain.HouseholdSettings{}, fmt.Errorf("get household settings: %w", err)
	}
	var err error
	settings.UpdatedAt, err = parseTime(updated)
	if err != nil {
		return domain.HouseholdSettings{}, fmt.Errorf("parse household settings: %w", err)
	}
	return settings, nil
}

func (s *SQLite) UpdateHouseholdEventColor(ctx context.Context, color string) error {
	result, err := s.q.ExecContext(ctx, `
		UPDATE household_settings
		SET household_event_color = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = 1`, color)
	if err != nil {
		return fmt.Errorf("update household event color: %w", err)
	}
	return changed(result, "household settings")
}
