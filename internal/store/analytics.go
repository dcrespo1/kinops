package store

import (
	"context"
	"fmt"
	"time"

	"github.com/dcrespo1/kinops/internal/domain"
	"github.com/dcrespo1/kinops/internal/scheduling"
)

func (s *SQLite) ListPersonDailyStats(ctx context.Context, from, through time.Time) ([]domain.PersonDailyStats, error) {
	rows, err := s.q.QueryContext(ctx, `
		SELECT assigned_person_id, due_date, COUNT(*),
		       SUM(CASE WHEN status = 'done' THEN 1 ELSE 0 END)
		FROM chore_instances
		WHERE due_date BETWEEN ? AND ?
		GROUP BY assigned_person_id, due_date
		ORDER BY due_date, assigned_person_id`, dbDate(from), dbDate(through))
	if err != nil {
		return nil, fmt.Errorf("list person daily analytics: %w", err)
	}
	defer rows.Close()
	var result []domain.PersonDailyStats
	for rows.Next() {
		var item domain.PersonDailyStats
		var dueDate string
		if err := rows.Scan(&item.PersonID, &dueDate, &item.Assigned, &item.Completed); err != nil {
			return nil, fmt.Errorf("scan person daily analytics: %w", err)
		}
		item.DueDate, err = time.Parse(scheduling.DateLayout, dueDate)
		if err != nil {
			return nil, fmt.Errorf("parse analytics due date: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list person daily analytics rows: %w", err)
	}
	return result, nil
}

func (s *SQLite) ListPersonOverdueCounts(ctx context.Context, before time.Time) (map[int64]int, error) {
	rows, err := s.q.QueryContext(ctx, `
		SELECT assigned_person_id, COUNT(*)
		FROM chore_instances
		WHERE due_date < ? AND status = 'pending'
		GROUP BY assigned_person_id
		ORDER BY assigned_person_id`, dbDate(before))
	if err != nil {
		return nil, fmt.Errorf("list overdue analytics: %w", err)
	}
	defer rows.Close()
	result := make(map[int64]int)
	for rows.Next() {
		var personID int64
		var count int
		if err := rows.Scan(&personID, &count); err != nil {
			return nil, fmt.Errorf("scan overdue analytics: %w", err)
		}
		result[personID] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list overdue analytics rows: %w", err)
	}
	return result, nil
}
