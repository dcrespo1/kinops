package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/dcrespo1/kinops/internal/domain"
	"github.com/dcrespo1/kinops/internal/scheduling"
)

const eventColumns = `id, title, description, location, category, all_day, start_date, end_date, start_time, end_time, recurrence_type, recurrence_params, recurrence_end_date, active, created_at, updated_at`

type eventRuleParams struct {
	Interval   int   `json:"interval,omitempty"`
	Weekdays   []int `json:"weekdays,omitempty"`
	DayOfMonth int   `json:"day_of_month,omitempty"`
	Month      int   `json:"month,omitempty"`
}

func encodeEventRule(rule domain.EventRecurrenceRule) (string, error) {
	params := eventRuleParams{Interval: rule.Interval, DayOfMonth: rule.DayOfMonth, Month: int(rule.Month)}
	for _, day := range rule.Weekdays {
		params.Weekdays = append(params.Weekdays, int(day))
	}
	encoded, err := json.Marshal(params)
	if err != nil {
		return "", fmt.Errorf("encode event recurrence: %w", err)
	}
	return string(encoded), nil
}

func decodeEventRule(ruleType domain.EventRuleType, raw string) (domain.EventRecurrenceRule, error) {
	var params eventRuleParams
	if err := json.Unmarshal([]byte(raw), &params); err != nil {
		return domain.EventRecurrenceRule{}, fmt.Errorf("decode event recurrence: %w", err)
	}
	rule := domain.EventRecurrenceRule{Type: ruleType, Interval: params.Interval, DayOfMonth: params.DayOfMonth, Month: time.Month(params.Month)}
	for _, day := range params.Weekdays {
		rule.Weekdays = append(rule.Weekdays, time.Weekday(day))
	}
	return rule, nil
}

func scanEvent(row interface{ Scan(...any) error }) (domain.HouseholdEvent, error) {
	var event domain.HouseholdEvent
	var allDay, active int
	var startDate, endDate, ruleParams, createdAt, updatedAt string
	var startTime, endTime, recurrenceEnd sql.NullString
	if err := row.Scan(
		&event.ID, &event.Title, &event.Description, &event.Location, &event.Category,
		&allDay, &startDate, &endDate, &startTime, &endTime, &event.Rule.Type,
		&ruleParams, &recurrenceEnd, &active, &createdAt, &updatedAt,
	); err != nil {
		return domain.HouseholdEvent{}, err
	}
	event.AllDay = allDay == 1
	event.Active = active == 1
	event.StartTime = startTime.String
	event.EndTime = endTime.String
	var err error
	event.StartDate, err = time.Parse(scheduling.DateLayout, startDate)
	if err != nil {
		return event, fmt.Errorf("parse event start date: %w", err)
	}
	event.EndDate, err = time.Parse(scheduling.DateLayout, endDate)
	if err != nil {
		return event, fmt.Errorf("parse event end date: %w", err)
	}
	if recurrenceEnd.Valid {
		value, parseErr := time.Parse(scheduling.DateLayout, recurrenceEnd.String)
		if parseErr != nil {
			return event, fmt.Errorf("parse event recurrence end date: %w", parseErr)
		}
		event.RecurrenceEndDate = &value
	}
	event.Rule, err = decodeEventRule(event.Rule.Type, ruleParams)
	if err != nil {
		return event, err
	}
	event.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return event, err
	}
	event.UpdatedAt, err = parseTime(updatedAt)
	return event, err
}

func (s *SQLite) GetEvent(ctx context.Context, id int64) (domain.HouseholdEvent, error) {
	event, err := scanEvent(s.q.QueryRowContext(ctx, `SELECT `+eventColumns+` FROM household_events WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return event, fmt.Errorf("get event %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return event, fmt.Errorf("get event %d: %w", id, err)
	}
	event.AudiencePersonIDs, err = s.eventAudienceIDs(ctx, id)
	if err != nil {
		return event, err
	}
	return event, nil
}

func (s *SQLite) ListEvents(ctx context.Context, includeInactive bool) ([]domain.HouseholdEvent, error) {
	query := `SELECT ` + eventColumns + ` FROM household_events`
	if !includeInactive {
		query += ` WHERE active = 1`
	}
	query += ` ORDER BY start_date, title, id`
	rows, err := s.q.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()
	var result []domain.HouseholdEvent
	for rows.Next() {
		event, scanErr := scanEvent(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan event: %w", scanErr)
		}
		result = append(result, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list event rows: %w", err)
	}
	audiences, err := s.allEventAudienceIDs(ctx)
	if err != nil {
		return nil, err
	}
	for i := range result {
		result[i].AudiencePersonIDs = audiences[result[i].ID]
	}
	return result, nil
}

func (s *SQLite) CreateEvent(ctx context.Context, event *domain.HouseholdEvent) error {
	params, err := encodeEventRule(event.Rule)
	if err != nil {
		return err
	}
	result, err := s.q.ExecContext(ctx, `
		INSERT INTO household_events
		(title, description, location, category, all_day, start_date, end_date, start_time, end_time, recurrence_type, recurrence_params, recurrence_end_date, active)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.Title, event.Description, event.Location, event.Category, boolInt(event.AllDay),
		dbDate(event.StartDate), dbDate(event.EndDate), nullableString(event.StartTime), nullableString(event.EndTime),
		event.Rule.Type, params, nullableDate(event.RecurrenceEndDate), boolInt(event.Active),
	)
	if err != nil {
		return fmt.Errorf("create event: %w", err)
	}
	event.ID, err = result.LastInsertId()
	if err != nil {
		return fmt.Errorf("read event ID: %w", err)
	}
	return nil
}

func (s *SQLite) UpdateEvent(ctx context.Context, event domain.HouseholdEvent) error {
	params, err := encodeEventRule(event.Rule)
	if err != nil {
		return err
	}
	result, err := s.q.ExecContext(ctx, `
		UPDATE household_events SET
		title = ?, description = ?, location = ?, category = ?, all_day = ?,
		start_date = ?, end_date = ?, start_time = ?, end_time = ?, recurrence_type = ?,
		recurrence_params = ?, recurrence_end_date = ?, updated_at = ?
		WHERE id = ?`,
		event.Title, event.Description, event.Location, event.Category, boolInt(event.AllDay),
		dbDate(event.StartDate), dbDate(event.EndDate), nullableString(event.StartTime), nullableString(event.EndTime),
		event.Rule.Type, params, nullableDate(event.RecurrenceEndDate), dbTime(time.Now()), event.ID,
	)
	if err != nil {
		return fmt.Errorf("update event %d: %w", event.ID, err)
	}
	return changed(result, "event")
}

func (s *SQLite) DeactivateEvent(ctx context.Context, id int64) error {
	result, err := s.q.ExecContext(ctx, `UPDATE household_events SET active = 0, updated_at = ? WHERE id = ? AND active = 1`, dbTime(time.Now()), id)
	if err != nil {
		return fmt.Errorf("deactivate event %d: %w", id, err)
	}
	return changed(result, "event")
}

func (s *SQLite) ReplaceEventAudience(ctx context.Context, eventID int64, personIDs []int64) error {
	if _, err := s.q.ExecContext(ctx, `DELETE FROM event_audiences WHERE event_id = ?`, eventID); err != nil {
		return fmt.Errorf("clear event %d audience: %w", eventID, err)
	}
	seen := map[int64]bool{}
	for _, personID := range personIDs {
		if personID <= 0 || seen[personID] {
			continue
		}
		seen[personID] = true
		if _, err := s.q.ExecContext(ctx, `INSERT INTO event_audiences (event_id, person_id) VALUES (?, ?)`, eventID, personID); err != nil {
			return fmt.Errorf("add person %d to event %d audience: %w", personID, eventID, err)
		}
	}
	return nil
}

func (s *SQLite) DeleteEventOccurrencesFrom(ctx context.Context, eventID int64, from time.Time) error {
	if _, err := s.q.ExecContext(ctx, `DELETE FROM event_occurrences WHERE event_id = ? AND start_date >= ?`, eventID, dbDate(from)); err != nil {
		return fmt.Errorf("delete future occurrences for event %d: %w", eventID, err)
	}
	return nil
}

func (s *SQLite) CreateEventOccurrence(ctx context.Context, occurrence *domain.EventOccurrence) error {
	key := dbDate(occurrence.StartDate)
	if occurrence.StartAt != nil {
		key += "T" + occurrence.StartAt.UTC().Format("150405Z")
	}
	result, err := s.q.ExecContext(ctx, `
		INSERT INTO event_occurrences (event_id, occurrence_key, start_date, end_date, start_at, end_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(event_id, occurrence_key) DO NOTHING`, occurrence.EventID, key,
		dbDate(occurrence.StartDate), dbDate(occurrence.EndDate), nullableTime(occurrence.StartAt), nullableTime(occurrence.EndAt))
	if err != nil {
		return fmt.Errorf("create event occurrence: %w", err)
	}
	if count, countErr := result.RowsAffected(); countErr == nil && count == 1 {
		occurrence.ID, err = result.LastInsertId()
	}
	return err
}

func (s *SQLite) ListScheduledEvents(ctx context.Context, from, through time.Time) ([]domain.ScheduledEvent, error) {
	rows, err := s.q.QueryContext(ctx, `
		SELECT
		eo.id, eo.event_id, eo.start_date, eo.end_date, eo.start_at, eo.end_at, eo.created_at, eo.updated_at,
		e.id, e.title, e.description, e.location, e.category, e.all_day, e.start_date, e.end_date,
		e.start_time, e.end_time, e.recurrence_type, e.recurrence_params, e.recurrence_end_date, e.active, e.created_at, e.updated_at,
		p.id, p.name, p.color, p.calendar_token, p.active, p.created_at, p.updated_at
		FROM event_occurrences eo
		JOIN household_events e ON e.id = eo.event_id
		LEFT JOIN event_audiences ea ON ea.event_id = e.id
		LEFT JOIN people p ON p.id = ea.person_id
		WHERE e.active = 1 AND eo.start_date <= ? AND eo.end_date > ?
		ORDER BY e.all_day DESC, COALESCE(eo.start_at, ''), e.title, eo.id, p.id`, dbDate(through), dbDate(from))
	if err != nil {
		return nil, fmt.Errorf("list scheduled events: %w", err)
	}
	defer rows.Close()
	var result []domain.ScheduledEvent
	index := map[int64]int{}
	for rows.Next() {
		item, person, scanErr := scanScheduledEvent(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan scheduled event: %w", scanErr)
		}
		position, exists := index[item.Occurrence.ID]
		if !exists {
			position = len(result)
			index[item.Occurrence.ID] = position
			result = append(result, item)
		}
		if person != nil {
			result[position].Audience = append(result[position].Audience, *person)
		}
	}
	return result, rows.Err()
}

func scanScheduledEvent(row interface{ Scan(...any) error }) (domain.ScheduledEvent, *domain.Person, error) {
	var item domain.ScheduledEvent
	var occurrenceStart, occurrenceEnd, occurrenceCreated, occurrenceUpdated string
	var startAt, endAt sql.NullString
	var allDay, eventActive int
	var eventStart, eventEnd, params, eventCreated, eventUpdated string
	var startTime, endTime, recurrenceEnd sql.NullString
	var personID sql.NullInt64
	var personName, personColor, personToken, personCreated, personUpdated sql.NullString
	var personActive sql.NullInt64
	err := row.Scan(
		&item.Occurrence.ID, &item.Occurrence.EventID, &occurrenceStart, &occurrenceEnd, &startAt, &endAt, &occurrenceCreated, &occurrenceUpdated,
		&item.Event.ID, &item.Event.Title, &item.Event.Description, &item.Event.Location, &item.Event.Category, &allDay, &eventStart, &eventEnd,
		&startTime, &endTime, &item.Event.Rule.Type, &params, &recurrenceEnd, &eventActive, &eventCreated, &eventUpdated,
		&personID, &personName, &personColor, &personToken, &personActive, &personCreated, &personUpdated,
	)
	if err != nil {
		return item, nil, err
	}
	parseDate := func(value string) (time.Time, error) { return time.Parse(scheduling.DateLayout, value) }
	item.Occurrence.StartDate, err = parseDate(occurrenceStart)
	if err != nil {
		return item, nil, err
	}
	item.Occurrence.EndDate, err = parseDate(occurrenceEnd)
	if err != nil {
		return item, nil, err
	}
	item.Occurrence.CreatedAt, err = parseTime(occurrenceCreated)
	if err != nil {
		return item, nil, err
	}
	item.Occurrence.UpdatedAt, err = parseTime(occurrenceUpdated)
	if err != nil {
		return item, nil, err
	}
	if startAt.Valid {
		value, parseErr := parseTime(startAt.String)
		if parseErr != nil {
			return item, nil, parseErr
		}
		item.Occurrence.StartAt = &value
	}
	if endAt.Valid {
		value, parseErr := parseTime(endAt.String)
		if parseErr != nil {
			return item, nil, parseErr
		}
		item.Occurrence.EndAt = &value
	}
	item.Event.AllDay = allDay == 1
	item.Event.Active = eventActive == 1
	item.Event.StartTime, item.Event.EndTime = startTime.String, endTime.String
	item.Event.StartDate, err = parseDate(eventStart)
	if err != nil {
		return item, nil, err
	}
	item.Event.EndDate, err = parseDate(eventEnd)
	if err != nil {
		return item, nil, err
	}
	item.Event.Rule, err = decodeEventRule(item.Event.Rule.Type, params)
	if err != nil {
		return item, nil, err
	}
	if recurrenceEnd.Valid {
		value, parseErr := parseDate(recurrenceEnd.String)
		if parseErr != nil {
			return item, nil, parseErr
		}
		item.Event.RecurrenceEndDate = &value
	}
	item.Event.CreatedAt, err = parseTime(eventCreated)
	if err != nil {
		return item, nil, err
	}
	item.Event.UpdatedAt, err = parseTime(eventUpdated)
	if err != nil {
		return item, nil, err
	}
	if !personID.Valid {
		return item, nil, nil
	}
	person := &domain.Person{ID: personID.Int64, Name: personName.String, Color: personColor.String, CalendarToken: personToken.String, Active: personActive.Int64 == 1}
	person.CreatedAt, err = parseTime(personCreated.String)
	if err != nil {
		return item, nil, err
	}
	person.UpdatedAt, err = parseTime(personUpdated.String)
	return item, person, err
}

func (s *SQLite) eventAudienceIDs(ctx context.Context, eventID int64) ([]int64, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT person_id FROM event_audiences WHERE event_id = ? ORDER BY person_id`, eventID)
	if err != nil {
		return nil, fmt.Errorf("list event %d audience: %w", eventID, err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *SQLite) allEventAudienceIDs(ctx context.Context) (map[int64][]int64, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT event_id, person_id FROM event_audiences ORDER BY event_id, person_id`)
	if err != nil {
		return nil, fmt.Errorf("list event audiences: %w", err)
	}
	defer rows.Close()
	result := map[int64][]int64{}
	for rows.Next() {
		var eventID, personID int64
		if err := rows.Scan(&eventID, &personID); err != nil {
			return nil, err
		}
		result[eventID] = append(result[eventID], personID)
	}
	return result, rows.Err()
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func sortedIDs(values []int64) []int64 {
	result := append([]int64(nil), values...)
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}
