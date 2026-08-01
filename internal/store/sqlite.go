package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/dcrespo1/kinops/internal/scheduling"
)

type dbtx interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type SQLite struct {
	db *sql.DB
	q  dbtx
}

func NewSQLite(db *sql.DB) *SQLite { return &SQLite{db: db, q: db} }

func (s *SQLite) WithinTx(ctx context.Context, fn func(*SQLite) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	transactional := &SQLite{db: s.db, q: tx}
	if err := fn(transactional); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return errors.Join(err, fmt.Errorf("roll back transaction: %w", rollbackErr))
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func dbDate(value time.Time) string { return value.Format(scheduling.DateLayout) }

func dbTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

func parseTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse database timestamp %q: %w", value, err)
	}
	return parsed, nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func changed(result sql.Result, entity string) error {
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read %s affected rows: %w", entity, err)
	}
	if count == 0 {
		return fmt.Errorf("%s: %w", entity, ErrNotFound)
	}
	return nil
}
