package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/dcrespo1/kinops/internal/domain"
	"github.com/dcrespo1/kinops/internal/scheduling"
)

const instanceColumns = `id, chore_id, schedule_id, sequence_no, due_date, assigned_person_id, status, completed_at, created_at, updated_at`

type instanceScanner interface{ Scan(...any) error }

func scanInstance(row instanceScanner) (domain.ChoreInstance, error) {
	var instance domain.ChoreInstance
	var due, created, updated string
	var completed sql.NullString
	if err := row.Scan(&instance.ID, &instance.ChoreID, &instance.ScheduleID, &instance.SequenceNo, &due, &instance.AssignedPersonID, &instance.Status, &completed, &created, &updated); err != nil {
		return domain.ChoreInstance{}, err
	}
	var err error
	instance.DueDate, err = time.Parse(scheduling.DateLayout, due)
	if err != nil {
		return domain.ChoreInstance{}, fmt.Errorf("parse instance due date: %w", err)
	}
	if completed.Valid {
		value, parseErr := parseTime(completed.String)
		if parseErr != nil {
			return domain.ChoreInstance{}, parseErr
		}
		instance.CompletedAt = &value
	}
	instance.CreatedAt, err = parseTime(created)
	if err != nil {
		return domain.ChoreInstance{}, err
	}
	instance.UpdatedAt, err = parseTime(updated)
	return instance, err
}

func (s *SQLite) GetInstance(ctx context.Context, id int64) (domain.ChoreInstance, error) {
	instance, err := scanInstance(s.q.QueryRowContext(ctx, `SELECT `+instanceColumns+` FROM chore_instances WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ChoreInstance{}, fmt.Errorf("get instance %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return domain.ChoreInstance{}, fmt.Errorf("get instance %d: %w", id, err)
	}
	return instance, nil
}

const dailyInstanceColumns = `
	ci.id, ci.chore_id, ci.schedule_id, ci.sequence_no, ci.due_date,
	ci.assigned_person_id, ci.status, ci.completed_at, ci.created_at, ci.updated_at,
	c.id, c.name, c.description, c.category, c.active, c.created_at, c.updated_at,
	p.id, p.name, p.color, p.calendar_token, p.active, p.created_at, p.updated_at`

func scanDailyInstance(row instanceScanner) (domain.DailyInstance, error) {
	var item domain.DailyInstance
	var due, instanceCreated, instanceUpdated string
	var completed sql.NullString
	var choreActive, personActive int
	var choreCreated, choreUpdated, personCreated, personUpdated string
	if err := row.Scan(
		&item.Instance.ID, &item.Instance.ChoreID, &item.Instance.ScheduleID,
		&item.Instance.SequenceNo, &due, &item.Instance.AssignedPersonID,
		&item.Instance.Status, &completed, &instanceCreated, &instanceUpdated,
		&item.Chore.ID, &item.Chore.Name, &item.Chore.Description, &item.Chore.Category,
		&choreActive, &choreCreated, &choreUpdated,
		&item.Assignee.ID, &item.Assignee.Name, &item.Assignee.Color,
		&item.Assignee.CalendarToken, &personActive, &personCreated, &personUpdated,
	); err != nil {
		return domain.DailyInstance{}, err
	}
	var err error
	item.Instance.DueDate, err = time.Parse(scheduling.DateLayout, due)
	if err != nil {
		return domain.DailyInstance{}, fmt.Errorf("parse daily instance due date: %w", err)
	}
	if completed.Valid {
		value, parseErr := parseTime(completed.String)
		if parseErr != nil {
			return domain.DailyInstance{}, parseErr
		}
		item.Instance.CompletedAt = &value
	}
	item.Instance.CreatedAt, err = parseTime(instanceCreated)
	if err != nil {
		return domain.DailyInstance{}, err
	}
	item.Instance.UpdatedAt, err = parseTime(instanceUpdated)
	if err != nil {
		return domain.DailyInstance{}, err
	}
	item.Chore.Active = choreActive == 1
	item.Chore.CreatedAt, err = parseTime(choreCreated)
	if err != nil {
		return domain.DailyInstance{}, err
	}
	item.Chore.UpdatedAt, err = parseTime(choreUpdated)
	if err != nil {
		return domain.DailyInstance{}, err
	}
	item.Assignee.Active = personActive == 1
	item.Assignee.CreatedAt, err = parseTime(personCreated)
	if err != nil {
		return domain.DailyInstance{}, err
	}
	item.Assignee.UpdatedAt, err = parseTime(personUpdated)
	return item, err
}

const dailyInstanceJoins = `
	FROM chore_instances ci
	JOIN chores c ON c.id = ci.chore_id
	JOIN people p ON p.id = ci.assigned_person_id`

func (s *SQLite) GetDailyInstance(ctx context.Context, id int64) (domain.DailyInstance, error) {
	item, err := scanDailyInstance(s.q.QueryRowContext(ctx, `SELECT `+dailyInstanceColumns+dailyInstanceJoins+` WHERE ci.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.DailyInstance{}, fmt.Errorf("get daily instance %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return domain.DailyInstance{}, fmt.Errorf("get daily instance %d: %w", id, err)
	}
	return item, nil
}

func (s *SQLite) ListDailyInstances(ctx context.Context, date time.Time) ([]domain.DailyInstance, error) {
	dateValue := dbDate(date)
	rows, err := s.q.QueryContext(ctx, `SELECT `+dailyInstanceColumns+dailyInstanceJoins+`
		WHERE ci.due_date = ? OR (ci.due_date < ? AND ci.status = 'pending')
		ORDER BY p.id, ci.due_date, c.category, c.name, ci.id`, dateValue, dateValue)
	if err != nil {
		return nil, fmt.Errorf("list daily instances for %s: %w", dateValue, err)
	}
	defer rows.Close()
	var items []domain.DailyInstance
	for rows.Next() {
		item, err := scanDailyInstance(rows)
		if err != nil {
			return nil, fmt.Errorf("scan daily instance: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list daily instance rows: %w", err)
	}
	return items, nil
}

func (s *SQLite) ListScheduledInstances(ctx context.Context, from, through time.Time) ([]domain.ScheduledInstance, error) {
	fromValue := dbDate(from)
	throughValue := dbDate(through)
	rows, err := s.q.QueryContext(ctx, `SELECT `+dailyInstanceColumns+dailyInstanceJoins+`
		WHERE ci.due_date BETWEEN ? AND ?
		ORDER BY ci.due_date, p.id, c.category, c.name, ci.id`, fromValue, throughValue)
	if err != nil {
		return nil, fmt.Errorf("list scheduled instances from %s through %s: %w", fromValue, throughValue, err)
	}
	defer rows.Close()
	var items []domain.ScheduledInstance
	for rows.Next() {
		item, err := scanDailyInstance(rows)
		if err != nil {
			return nil, fmt.Errorf("scan scheduled instance: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list scheduled instance rows: %w", err)
	}
	return items, nil
}

func (s *SQLite) ListScheduledInstancesForPerson(ctx context.Context, personID int64, from, through time.Time) ([]domain.ScheduledInstance, error) {
	fromValue := dbDate(from)
	throughValue := dbDate(through)
	rows, err := s.q.QueryContext(ctx, `SELECT `+dailyInstanceColumns+dailyInstanceJoins+`
		WHERE ci.assigned_person_id = ? AND ci.due_date BETWEEN ? AND ?
		ORDER BY ci.due_date, ci.id`, personID, fromValue, throughValue)
	if err != nil {
		return nil, fmt.Errorf("list scheduled instances for person %d from %s through %s: %w", personID, fromValue, throughValue, err)
	}
	defer rows.Close()
	var items []domain.ScheduledInstance
	for rows.Next() {
		item, err := scanDailyInstance(rows)
		if err != nil {
			return nil, fmt.Errorf("scan scheduled instance for person %d: %w", personID, err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list scheduled instance rows for person %d: %w", personID, err)
	}
	return items, nil
}

func (s *SQLite) ListInstancesBySchedule(ctx context.Context, scheduleID int64) ([]domain.ChoreInstance, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT `+instanceColumns+` FROM chore_instances WHERE schedule_id = ? ORDER BY due_date, sequence_no`, scheduleID)
	if err != nil {
		return nil, fmt.Errorf("list schedule %d instances: %w", scheduleID, err)
	}
	defer rows.Close()
	var instances []domain.ChoreInstance
	for rows.Next() {
		instance, err := scanInstance(rows)
		if err != nil {
			return nil, fmt.Errorf("scan instance: %w", err)
		}
		instances = append(instances, instance)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list instance rows: %w", err)
	}
	return instances, nil
}

func (s *SQLite) DeletePendingInstancesFrom(ctx context.Context, scheduleID int64, from time.Time) error {
	if _, err := s.q.ExecContext(ctx, `DELETE FROM chore_instances WHERE schedule_id = ? AND status = 'pending' AND due_date >= ?`, scheduleID, dbDate(from)); err != nil {
		return fmt.Errorf("delete pending instances for schedule %d: %w", scheduleID, err)
	}
	return nil
}

func (s *SQLite) MaxInstanceSequence(ctx context.Context, scheduleID int64) (int64, error) {
	var maximum int64
	if err := s.q.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence_no), 0) FROM chore_instances WHERE schedule_id = ?`, scheduleID).Scan(&maximum); err != nil {
		return 0, fmt.Errorf("read maximum sequence for schedule %d: %w", scheduleID, err)
	}
	return maximum, nil
}

func (s *SQLite) InstanceDates(ctx context.Context, scheduleID int64, from, through time.Time) (map[string]bool, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT due_date FROM chore_instances WHERE schedule_id = ? AND due_date BETWEEN ? AND ?`, scheduleID, dbDate(from), dbDate(through))
	if err != nil {
		return nil, fmt.Errorf("list instance dates for schedule %d: %w", scheduleID, err)
	}
	defer rows.Close()
	dates := make(map[string]bool)
	for rows.Next() {
		var date string
		if err := rows.Scan(&date); err != nil {
			return nil, fmt.Errorf("scan instance date: %w", err)
		}
		dates[date] = true
	}
	return dates, rows.Err()
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return dbTime(*value)
}

func (s *SQLite) CreateInstance(ctx context.Context, instance *domain.ChoreInstance) error {
	result, err := s.q.ExecContext(ctx, `INSERT INTO chore_instances (chore_id, schedule_id, sequence_no, due_date, assigned_person_id, status, completed_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, instance.ChoreID, instance.ScheduleID, instance.SequenceNo, dbDate(instance.DueDate), instance.AssignedPersonID, instance.Status, nullableTime(instance.CompletedAt))
	if err != nil {
		return fmt.Errorf("create instance: %w", err)
	}
	instance.ID, err = result.LastInsertId()
	if err != nil {
		return fmt.Errorf("read instance ID: %w", err)
	}
	return nil
}

func (s *SQLite) TransitionInstance(ctx context.Context, id int64, from, to domain.InstanceStatus, completedAt *time.Time, updatedAt time.Time) (bool, error) {
	result, err := s.q.ExecContext(ctx, `
		UPDATE chore_instances
		SET status = ?, completed_at = ?, updated_at = ?
		WHERE id = ? AND status = ?`, to, nullableTime(completedAt), dbTime(updatedAt), id, from)
	if err != nil {
		return false, fmt.Errorf("transition instance %d from %s to %s: %w", id, from, to, err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read transitioned instance rows: %w", err)
	}
	return count == 1, nil
}
