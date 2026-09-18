package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type WorkerApp struct {
	WorkerID    string
	URL         string
	Description string
	DeclaredAt  time.Time
}

func (s *Store) UpsertWorkerApp(ctx context.Context, a WorkerApp) error {
	if a.WorkerID == "" {
		return fmt.Errorf("server/store: upsert app: empty worker ID")
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO worker_apps (worker_id, url, description, declared_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(worker_id) DO UPDATE SET
			url         = excluded.url,
			description = excluded.description,
			declared_at = excluded.declared_at`,
		a.WorkerID, a.URL, a.Description, timeToUnixNano(a.DeclaredAt),
	)
	if err != nil {
		return fmt.Errorf("server/store: upsert app for %q: %w", a.WorkerID, err)
	}
	return nil
}

func (s *Store) GetWorkerApp(ctx context.Context, workerID string) (WorkerApp, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT worker_id, url, description, declared_at FROM worker_apps WHERE worker_id = ?`, workerID)

	var a WorkerApp
	var declaredAt int64
	err := row.Scan(&a.WorkerID, &a.URL, &a.Description, &declaredAt)
	if errors.Is(err, sql.ErrNoRows) {
		return WorkerApp{}, fmt.Errorf("server/store: app for %q: %w", workerID, ErrNotFound)
	}
	if err != nil {
		return WorkerApp{}, fmt.Errorf("server/store: get app for %q: %w", workerID, err)
	}
	a.DeclaredAt = unixNanoToTime(declaredAt)
	return a, nil
}

func (s *Store) ListWorkerApps(ctx context.Context) ([]WorkerApp, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT worker_id, url, description, declared_at FROM worker_apps ORDER BY worker_id ASC`)
	if err != nil {
		return nil, fmt.Errorf("server/store: list apps: %w", err)
	}
	defer rows.Close()

	var out []WorkerApp
	for rows.Next() {
		var a WorkerApp
		var declaredAt int64
		if err := rows.Scan(&a.WorkerID, &a.URL, &a.Description, &declaredAt); err != nil {
			return nil, fmt.Errorf("server/store: scan app: %w", err)
		}
		a.DeclaredAt = unixNanoToTime(declaredAt)
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("server/store: list apps: %w", err)
	}
	return out, nil
}

func (s *Store) DeleteWorkerApp(ctx context.Context, workerID string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM worker_apps WHERE worker_id = ?`, workerID); err != nil {
		return fmt.Errorf("server/store: delete app for %q: %w", workerID, err)
	}
	return nil
}
