package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/dcrespo1/kinops/internal/domain"
)

const personColumns = `id, name, color, calendar_token, active, created_at, updated_at`

type personScanner interface{ Scan(...any) error }

func scanPerson(row personScanner) (domain.Person, error) {
	var person domain.Person
	var active int
	var created, updated string
	if err := row.Scan(&person.ID, &person.Name, &person.Color, &person.CalendarToken, &active, &created, &updated); err != nil {
		return domain.Person{}, err
	}
	var err error
	person.Active = active == 1
	person.CreatedAt, err = parseTime(created)
	if err != nil {
		return domain.Person{}, err
	}
	person.UpdatedAt, err = parseTime(updated)
	return person, err
}

func (s *SQLite) GetPerson(ctx context.Context, id int64) (domain.Person, error) {
	person, err := scanPerson(s.q.QueryRowContext(ctx, `SELECT `+personColumns+` FROM people WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Person{}, fmt.Errorf("get person %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return domain.Person{}, fmt.Errorf("get person %d: %w", id, err)
	}
	return person, nil
}

func (s *SQLite) GetActivePersonByCalendarToken(ctx context.Context, token string) (domain.Person, error) {
	person, err := scanPerson(s.q.QueryRowContext(ctx, `SELECT `+personColumns+` FROM people WHERE calendar_token = ? AND active = 1`, token))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Person{}, fmt.Errorf("get active person by calendar token: %w", ErrNotFound)
	}
	if err != nil {
		return domain.Person{}, fmt.Errorf("get active person by calendar token: %w", err)
	}
	return person, nil
}

func (s *SQLite) ListPeople(ctx context.Context, includeInactive bool) ([]domain.Person, error) {
	query := `SELECT ` + personColumns + ` FROM people`
	if !includeInactive {
		query += ` WHERE active = 1`
	}
	query += ` ORDER BY id`
	rows, err := s.q.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list people: %w", err)
	}
	defer rows.Close()
	var people []domain.Person
	for rows.Next() {
		person, err := scanPerson(rows)
		if err != nil {
			return nil, fmt.Errorf("scan person: %w", err)
		}
		people = append(people, person)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list people rows: %w", err)
	}
	return people, nil
}

func (s *SQLite) CreatePerson(ctx context.Context, person *domain.Person) error {
	result, err := s.q.ExecContext(ctx, `INSERT INTO people (name, color, calendar_token, active) VALUES (?, ?, ?, ?)`, person.Name, person.Color, person.CalendarToken, boolInt(person.Active))
	if err != nil {
		return fmt.Errorf("create person: %w", err)
	}
	person.ID, err = result.LastInsertId()
	if err != nil {
		return fmt.Errorf("read person ID: %w", err)
	}
	stored, err := s.GetPerson(ctx, person.ID)
	if err != nil {
		return err
	}
	*person = stored
	return nil
}

func (s *SQLite) UpdatePerson(ctx context.Context, person domain.Person) error {
	result, err := s.q.ExecContext(ctx, `UPDATE people SET name = ?, color = ?, active = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`, person.Name, person.Color, boolInt(person.Active), person.ID)
	if err != nil {
		return fmt.Errorf("update person %d: %w", person.ID, err)
	}
	return changed(result, "person")
}

func (s *SQLite) DeactivatePerson(ctx context.Context, id int64) error {
	result, err := s.q.ExecContext(ctx, `UPDATE people SET active = 0, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deactivate person %d: %w", id, err)
	}
	return changed(result, "person")
}

func (s *SQLite) RotatePersonCalendarToken(ctx context.Context, id int64, token string) error {
	result, err := s.q.ExecContext(ctx, `UPDATE people SET calendar_token = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ? AND active = 1`, token, id)
	if err != nil {
		return fmt.Errorf("rotate calendar token for person %d: %w", id, err)
	}
	return changed(result, "person")
}
