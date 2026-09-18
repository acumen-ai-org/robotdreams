package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/acumen-ai-org/robotdreams/pkg/scheduling"
)

const schedulesSchema = `
CREATE TABLE IF NOT EXISTS schedules (
	id         TEXT PRIMARY KEY,
	owner      TEXT NOT NULL,
	to_worker  TEXT NOT NULL,
	cron       TEXT NOT NULL,
	subject    TEXT NOT NULL DEFAULT '',
	body       TEXT,
	enabled    INTEGER NOT NULL DEFAULT 1,
	next_at    INTEGER NOT NULL,
	last_at    INTEGER,
	created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_schedules_owner ON schedules(owner);
CREATE INDEX IF NOT EXISTS idx_schedules_to_worker ON schedules(to_worker);
CREATE INDEX IF NOT EXISTS idx_schedules_next_at ON schedules(enabled, next_at);
`

type ScheduleStore struct {
	db *sql.DB
}

var _ scheduling.Store = (*ScheduleStore)(nil)

func (s *Store) Schedules() *ScheduleStore { return &ScheduleStore{db: s.db} }

func (s *ScheduleStore) Upsert(ctx context.Context, sc scheduling.Schedule) error {
	if err := sc.Validate(); err != nil {
		return fmt.Errorf("server/store: upsert schedule: %w", err)
	}
	var body sql.NullString
	if len(sc.Body) > 0 {
		body = sql.NullString{String: string(sc.Body), Valid: true}
	}
	var lastAt sql.NullInt64
	if sc.LastAt != nil {
		lastAt = sql.NullInt64{Int64: sc.LastAt.UnixNano(), Valid: true}
	}
	createdAt := sc.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO schedules (id, owner, to_worker, cron, subject, body, enabled, next_at, last_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			owner     = excluded.owner,
			to_worker = excluded.to_worker,
			cron      = excluded.cron,
			subject   = excluded.subject,
			body      = excluded.body,
			enabled   = excluded.enabled,
			next_at   = excluded.next_at`,
		sc.ID, sc.Owner, sc.To, sc.Cron, sc.Subject, body, boolToInt(sc.Enabled),
		timeToUnixNano(sc.NextAt), lastAt, timeToUnixNano(createdAt),
	)
	if err != nil {
		return fmt.Errorf("server/store: upsert schedule %q: %w", sc.ID, err)
	}
	return nil
}

const scheduleColumns = `id, owner, to_worker, cron, subject, body, enabled, next_at, last_at, created_at`

func (s *ScheduleStore) Get(ctx context.Context, id string) (scheduling.Schedule, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+scheduleColumns+` FROM schedules WHERE id = ?`, id)
	sc, err := scanSchedule(row)
	if errors.Is(err, sql.ErrNoRows) {
		return scheduling.Schedule{}, fmt.Errorf("server/store: schedule %q: %w", id, scheduling.ErrNotFound)
	}
	if err != nil {
		return scheduling.Schedule{}, fmt.Errorf("server/store: get schedule %q: %w", id, err)
	}
	return sc, nil
}

func (s *ScheduleStore) List(ctx context.Context, f scheduling.Filter) ([]scheduling.Schedule, error) {
	query := `SELECT ` + scheduleColumns + ` FROM schedules WHERE 1 = 1`
	var args []any
	if f.Owner != "" {
		query += ` AND owner = ?`
		args = append(args, f.Owner)
	}
	if f.To != "" {
		query += ` AND to_worker = ?`
		args = append(args, f.To)
	}
	if f.Worker != "" {
		query += ` AND (owner = ? OR to_worker = ?)`
		args = append(args, f.Worker, f.Worker)
	}
	query += ` ORDER BY id`
	return s.query(ctx, query, args...)
}

func (s *ScheduleStore) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM schedules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("server/store: delete schedule %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("server/store: schedule %q: %w", id, scheduling.ErrNotFound)
	}
	return nil
}

func (s *ScheduleStore) ListDue(ctx context.Context, now time.Time) ([]scheduling.Schedule, error) {
	return s.query(ctx, `SELECT `+scheduleColumns+` FROM schedules
		WHERE enabled = 1 AND next_at <= ? ORDER BY next_at, id`, timeToUnixNano(now))
}

func (s *ScheduleStore) MarkFired(ctx context.Context, id string, lastAt, nextAt time.Time) error {
	var last sql.NullInt64
	if !lastAt.IsZero() {
		last = sql.NullInt64{Int64: lastAt.UnixNano(), Valid: true}
	}
	res, err := s.db.ExecContext(ctx, `UPDATE schedules SET last_at = ?, next_at = ? WHERE id = ?`,
		last, timeToUnixNano(nextAt), id)
	if err != nil {
		return fmt.Errorf("server/store: mark schedule %q fired: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("server/store: schedule %q: %w", id, scheduling.ErrNotFound)
	}
	return nil
}

func (s *ScheduleStore) query(ctx context.Context, query string, args ...any) ([]scheduling.Schedule, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("server/store: list schedules: %w", err)
	}
	defer rows.Close()

	var out []scheduling.Schedule
	for rows.Next() {
		sc, err := scanSchedule(rows)
		if err != nil {
			return nil, fmt.Errorf("server/store: list schedules: %w", err)
		}
		out = append(out, sc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("server/store: list schedules: %w", err)
	}
	return out, nil
}

func scanSchedule(sc scanner) (scheduling.Schedule, error) {
	var (
		s       scheduling.Schedule
		body    sql.NullString
		enabled int
		nextAt  int64
		lastAt  sql.NullInt64
		created int64
	)
	if err := sc.Scan(&s.ID, &s.Owner, &s.To, &s.Cron, &s.Subject, &body, &enabled, &nextAt, &lastAt, &created); err != nil {
		return scheduling.Schedule{}, err
	}
	if body.Valid && body.String != "" {
		s.Body = json.RawMessage(body.String)
	}
	s.Enabled = enabled != 0
	s.NextAt = unixNanoToTime(nextAt)
	if lastAt.Valid && lastAt.Int64 != 0 {
		t := unixNanoToTime(lastAt.Int64)
		s.LastAt = &t
	}
	s.CreatedAt = unixNanoToTime(created)
	return s, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
