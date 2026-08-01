package store

import (
	"context"
	"fmt"

	"github.com/dcrespo1/kinops/internal/domain"
)

func (s *SQLite) CreateCompletionLog(ctx context.Context, log *domain.CompletionLog) error {
	result, err := s.q.ExecContext(ctx, `
		INSERT INTO completion_logs (chore_instance_id, person_id, event_type, occurred_at)
		VALUES (?, ?, ?, ?)`, log.ChoreInstanceID, log.PersonID, log.EventType, dbTime(log.OccurredAt))
	if err != nil {
		return fmt.Errorf("create completion log: %w", err)
	}
	log.ID, err = result.LastInsertId()
	if err != nil {
		return fmt.Errorf("read completion log ID: %w", err)
	}
	return nil
}

func (s *SQLite) ListRecentCompletionActivity(ctx context.Context, limit int) ([]domain.CompletionActivity, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := s.q.QueryContext(ctx, `
		SELECT cl.id, c.name, p.name, cl.event_type, cl.occurred_at
		FROM completion_logs cl
		JOIN chore_instances ci ON ci.id = cl.chore_instance_id
		JOIN chores c ON c.id = ci.chore_id
		JOIN people p ON p.id = cl.person_id
		ORDER BY cl.occurred_at DESC, cl.id DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list recent completion activity: %w", err)
	}
	defer rows.Close()
	var result []domain.CompletionActivity
	for rows.Next() {
		var item domain.CompletionActivity
		var occurredAt string
		if err := rows.Scan(&item.ID, &item.ChoreName, &item.PersonName, &item.EventType, &occurredAt); err != nil {
			return nil, fmt.Errorf("scan recent completion activity: %w", err)
		}
		item.OccurredAt, err = parseTime(occurredAt)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list recent completion activity rows: %w", err)
	}
	return result, nil
}
