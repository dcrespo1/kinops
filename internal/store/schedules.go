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

const scheduleColumns = `id, chore_id, rule_type, rule_params, start_date, end_date, assignment_mode, fixed_person_id, rotation_start_person_id, active, created_at, updated_at`

type scheduleScanner interface{ Scan(...any) error }

func scanSchedule(row scheduleScanner) (domain.Schedule, error) {
	var schedule domain.Schedule
	var ruleType, params, start, created, updated string
	var end sql.NullString
	var fixed, rotation sql.NullInt64
	var active int
	if err := row.Scan(&schedule.ID, &schedule.ChoreID, &ruleType, &params, &start, &end, &schedule.AssignmentMode, &fixed, &rotation, &active, &created, &updated); err != nil {
		return domain.Schedule{}, err
	}
	rule, err := scheduling.UnmarshalRule(domain.RuleType(ruleType), params)
	if err != nil {
		return domain.Schedule{}, err
	}
	schedule.Rule = rule
	schedule.StartDate, err = time.Parse(scheduling.DateLayout, start)
	if err != nil {
		return domain.Schedule{}, fmt.Errorf("parse schedule start date: %w", err)
	}
	if end.Valid {
		value, parseErr := time.Parse(scheduling.DateLayout, end.String)
		if parseErr != nil {
			return domain.Schedule{}, fmt.Errorf("parse schedule end date: %w", parseErr)
		}
		schedule.EndDate = &value
	}
	if fixed.Valid {
		schedule.FixedPersonID = &fixed.Int64
	}
	if rotation.Valid {
		schedule.RotationStartPersonID = &rotation.Int64
	}
	schedule.Active = active == 1
	schedule.CreatedAt, err = parseTime(created)
	if err != nil {
		return domain.Schedule{}, err
	}
	schedule.UpdatedAt, err = parseTime(updated)
	return schedule, err
}

func (s *SQLite) GetSchedule(ctx context.Context, id int64) (domain.Schedule, error) {
	schedule, err := scanSchedule(s.q.QueryRowContext(ctx, `SELECT `+scheduleColumns+` FROM schedules WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Schedule{}, fmt.Errorf("get schedule %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return domain.Schedule{}, fmt.Errorf("get schedule %d: %w", id, err)
	}
	return schedule, nil
}

func (s *SQLite) listSchedules(ctx context.Context, query string, args ...any) ([]domain.Schedule, error) {
	rows, err := s.q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list schedules: %w", err)
	}
	defer rows.Close()
	var schedules []domain.Schedule
	for rows.Next() {
		schedule, err := scanSchedule(rows)
		if err != nil {
			return nil, fmt.Errorf("scan schedule: %w", err)
		}
		schedules = append(schedules, schedule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list schedule rows: %w", err)
	}
	return schedules, nil
}

func (s *SQLite) ListSchedulesByChore(ctx context.Context, choreID int64, includeInactive bool) ([]domain.Schedule, error) {
	query := `SELECT ` + scheduleColumns + ` FROM schedules WHERE chore_id = ?`
	if !includeInactive {
		query += ` AND active = 1`
	}
	query += ` ORDER BY id`
	return s.listSchedules(ctx, query, choreID)
}

func (s *SQLite) ListActiveSchedules(ctx context.Context) ([]domain.Schedule, error) {
	return s.listSchedules(ctx, `SELECT `+scheduleColumns+` FROM schedules WHERE active = 1 ORDER BY id`)
}

func nullableDate(value *time.Time) any {
	if value == nil {
		return nil
	}
	return dbDate(*value)
}

func nullableID(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func scheduleValues(schedule domain.Schedule) ([]any, error) {
	params, err := scheduling.MarshalRule(schedule.Rule)
	if err != nil {
		return nil, err
	}
	return []any{schedule.ChoreID, schedule.Rule.Type, params, dbDate(schedule.StartDate), nullableDate(schedule.EndDate), schedule.AssignmentMode, nullableID(schedule.FixedPersonID), nullableID(schedule.RotationStartPersonID), boolInt(schedule.Active)}, nil
}

func (s *SQLite) CreateSchedule(ctx context.Context, schedule *domain.Schedule) error {
	values, err := scheduleValues(*schedule)
	if err != nil {
		return fmt.Errorf("create schedule: %w", err)
	}
	result, err := s.q.ExecContext(ctx, `INSERT INTO schedules (chore_id, rule_type, rule_params, start_date, end_date, assignment_mode, fixed_person_id, rotation_start_person_id, active) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, values...)
	if err != nil {
		return fmt.Errorf("create schedule: %w", err)
	}
	schedule.ID, err = result.LastInsertId()
	if err != nil {
		return fmt.Errorf("read schedule ID: %w", err)
	}
	stored, err := s.GetSchedule(ctx, schedule.ID)
	if err != nil {
		return err
	}
	*schedule = stored
	return nil
}

func (s *SQLite) UpdateSchedule(ctx context.Context, schedule domain.Schedule) error {
	values, err := scheduleValues(schedule)
	if err != nil {
		return fmt.Errorf("update schedule %d: %w", schedule.ID, err)
	}
	values = append(values, schedule.ID)
	result, err := s.q.ExecContext(ctx, `UPDATE schedules SET chore_id = ?, rule_type = ?, rule_params = ?, start_date = ?, end_date = ?, assignment_mode = ?, fixed_person_id = ?, rotation_start_person_id = ?, active = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`, values...)
	if err != nil {
		return fmt.Errorf("update schedule %d: %w", schedule.ID, err)
	}
	return changed(result, "schedule")
}

func (s *SQLite) DeactivateSchedule(ctx context.Context, id int64) error {
	result, err := s.q.ExecContext(ctx, `UPDATE schedules SET active = 0, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deactivate schedule %d: %w", id, err)
	}
	return changed(result, "schedule")
}
