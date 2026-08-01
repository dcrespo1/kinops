package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/dcrespo1/kinops/internal/domain"
)

const choreColumns = `id, name, description, category, active, created_at, updated_at`

type choreScanner interface{ Scan(...any) error }

func scanChore(row choreScanner) (domain.Chore, error) {
	var chore domain.Chore
	var active int
	var created, updated string
	if err := row.Scan(&chore.ID, &chore.Name, &chore.Description, &chore.Category, &active, &created, &updated); err != nil {
		return domain.Chore{}, err
	}
	var err error
	chore.Active = active == 1
	chore.CreatedAt, err = parseTime(created)
	if err != nil {
		return domain.Chore{}, err
	}
	chore.UpdatedAt, err = parseTime(updated)
	return chore, err
}

func (s *SQLite) GetChore(ctx context.Context, id int64) (domain.Chore, error) {
	chore, err := scanChore(s.q.QueryRowContext(ctx, `SELECT `+choreColumns+` FROM chores WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Chore{}, fmt.Errorf("get chore %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return domain.Chore{}, fmt.Errorf("get chore %d: %w", id, err)
	}
	return chore, nil
}

func (s *SQLite) ListChores(ctx context.Context, includeInactive bool) ([]domain.Chore, error) {
	query := `SELECT ` + choreColumns + ` FROM chores`
	if !includeInactive {
		query += ` WHERE active = 1`
	}
	query += ` ORDER BY name, id`
	rows, err := s.q.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list chores: %w", err)
	}
	defer rows.Close()
	var chores []domain.Chore
	for rows.Next() {
		chore, err := scanChore(rows)
		if err != nil {
			return nil, fmt.Errorf("scan chore: %w", err)
		}
		chores = append(chores, chore)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list chore rows: %w", err)
	}
	return chores, nil
}

func (s *SQLite) CreateChore(ctx context.Context, chore *domain.Chore) error {
	result, err := s.q.ExecContext(ctx, `INSERT INTO chores (name, description, category, active) VALUES (?, ?, ?, ?)`, chore.Name, chore.Description, chore.Category, boolInt(chore.Active))
	if err != nil {
		return fmt.Errorf("create chore: %w", err)
	}
	chore.ID, err = result.LastInsertId()
	if err != nil {
		return fmt.Errorf("read chore ID: %w", err)
	}
	stored, err := s.GetChore(ctx, chore.ID)
	if err != nil {
		return err
	}
	*chore = stored
	return nil
}

func (s *SQLite) UpdateChore(ctx context.Context, chore domain.Chore) error {
	result, err := s.q.ExecContext(ctx, `UPDATE chores SET name = ?, description = ?, category = ?, active = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`, chore.Name, chore.Description, chore.Category, boolInt(chore.Active), chore.ID)
	if err != nil {
		return fmt.Errorf("update chore %d: %w", chore.ID, err)
	}
	return changed(result, "chore")
}

func (s *SQLite) DeactivateChore(ctx context.Context, id int64) error {
	result, err := s.q.ExecContext(ctx, `UPDATE chores SET active = 0, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deactivate chore %d: %w", id, err)
	}
	return changed(result, "chore")
}
