package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/acumen-ai-org/robotdreams/pkg/updates"
)

const UpdateAnnouncementRetention = 50

type WorkerUpdateState struct {
	WorkerID       string
	Kind           string
	CurrentVersion string
	AnnouncementID string
	TargetVersion  string
	Status         string
	Detail         string
	ReportedAt     time.Time
}

type WorkerUpdateFilter struct {
	WorkerID       string
	Kind           string
	AnnouncementID string
}

func (s *Store) InsertUpdateAnnouncement(ctx context.Context, a updates.Announcement, recipients int) error {
	if a.ID == "" {
		return fmt.Errorf("server/store: insert update announcement: empty ID")
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO update_announcements
			(id, kind, version, source, min_version, severity, notes, announced_by, announced_at, recipient_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.Kind, a.Version, a.Source, a.MinVersion, a.Severity, a.Notes,
		a.AnnouncedBy, timeToUnixNano(a.AnnouncedAt), recipients,
	); err != nil {
		return fmt.Errorf("server/store: insert update announcement %q: %w", a.ID, err)
	}

	return s.pruneUpdateAnnouncements(ctx, a.Kind)
}

func (s *Store) pruneUpdateAnnouncements(ctx context.Context, kind string) error {
	if _, err := s.db.ExecContext(ctx, `
		DELETE FROM update_announcements
		WHERE kind = ? AND id NOT IN (
			SELECT id FROM update_announcements
			WHERE kind = ?
			ORDER BY announced_at DESC, id DESC
			LIMIT ?)`,
		kind, kind, UpdateAnnouncementRetention,
	); err != nil {
		return fmt.Errorf("server/store: prune update announcements for kind %q: %w", kind, err)
	}
	return nil
}

func (s *Store) GetUpdateAnnouncement(ctx context.Context, id string) (updates.Announcement, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, kind, version, source, min_version, severity, notes, announced_by, announced_at
		FROM update_announcements WHERE id = ?`, id)

	a, err := scanAnnouncement(row)
	if errors.Is(err, sql.ErrNoRows) {
		return updates.Announcement{}, fmt.Errorf("server/store: update announcement %q: %w", id, ErrNotFound)
	}
	if err != nil {
		return updates.Announcement{}, fmt.Errorf("server/store: get update announcement %q: %w", id, err)
	}
	return a, nil
}

func (s *Store) ListUpdateAnnouncements(ctx context.Context, kind string, limit int) ([]updates.Announcement, error) {
	query := `
		SELECT id, kind, version, source, min_version, severity, notes, announced_by, announced_at
		FROM update_announcements`
	var args []any
	if kind != "" {
		query += " WHERE kind = ?"
		args = append(args, kind)
	}
	query += " ORDER BY announced_at DESC, id DESC"
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("server/store: list update announcements: %w", err)
	}
	defer rows.Close()

	var out []updates.Announcement
	for rows.Next() {
		a, err := scanAnnouncement(rows)
		if err != nil {
			return nil, fmt.Errorf("server/store: scan update announcement: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("server/store: list update announcements: %w", err)
	}
	return out, nil
}

func scanAnnouncement(sc scanner) (updates.Announcement, error) {
	var a updates.Announcement
	var announcedAt int64
	if err := sc.Scan(&a.ID, &a.Kind, &a.Version, &a.Source, &a.MinVersion,
		&a.Severity, &a.Notes, &a.AnnouncedBy, &announcedAt); err != nil {
		return updates.Announcement{}, err
	}
	a.AnnouncedAt = unixNanoToTime(announcedAt)
	return a, nil
}

func (s *Store) UpsertWorkerUpdateState(ctx context.Context, st WorkerUpdateState) error {
	if st.WorkerID == "" || st.Kind == "" {
		return fmt.Errorf("server/store: upsert worker update state: empty worker ID or kind")
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO worker_update_state
			(worker_id, kind, current_version, announcement_id, target_version, status, detail, reported_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(worker_id, kind) DO UPDATE SET
			current_version = CASE WHEN excluded.current_version != ''
			                       THEN excluded.current_version
			                       ELSE worker_update_state.current_version END,
			announcement_id = CASE WHEN excluded.announcement_id != ''
			                       THEN excluded.announcement_id
			                       ELSE worker_update_state.announcement_id END,
			target_version  = CASE WHEN excluded.announcement_id != ''
			                       THEN excluded.target_version
			                       ELSE worker_update_state.target_version END,
			status          = excluded.status,
			detail          = excluded.detail,
			reported_at     = excluded.reported_at`,
		st.WorkerID, st.Kind, st.CurrentVersion, st.AnnouncementID, st.TargetVersion,
		st.Status, st.Detail, timeToUnixNano(st.ReportedAt),
	)
	if err != nil {
		return fmt.Errorf("server/store: upsert update state for %q/%q: %w", st.WorkerID, st.Kind, err)
	}
	return nil
}

func (s *Store) ListWorkerUpdateState(ctx context.Context, f WorkerUpdateFilter) ([]WorkerUpdateState, error) {
	query := `
		SELECT worker_id, kind, current_version, announcement_id, target_version, status, detail, reported_at
		FROM worker_update_state`

	var conds []string
	var args []any
	if f.WorkerID != "" {
		conds = append(conds, "worker_id = ?")
		args = append(args, f.WorkerID)
	}
	if f.Kind != "" {
		conds = append(conds, "kind = ?")
		args = append(args, f.Kind)
	}
	if f.AnnouncementID != "" {
		conds = append(conds, "announcement_id = ?")
		args = append(args, f.AnnouncementID)
	}
	if len(conds) > 0 {
		query += " WHERE " + strings.Join(conds, " AND ") //nolint:gosec // G202: fixed fragments, values in args
	}
	query += " ORDER BY worker_id ASC, kind ASC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("server/store: list worker update state: %w", err)
	}
	defer rows.Close()

	var out []WorkerUpdateState
	for rows.Next() {
		var st WorkerUpdateState
		var reportedAt int64
		if err := rows.Scan(&st.WorkerID, &st.Kind, &st.CurrentVersion, &st.AnnouncementID,
			&st.TargetVersion, &st.Status, &st.Detail, &reportedAt); err != nil {
			return nil, fmt.Errorf("server/store: scan worker update state: %w", err)
		}
		st.ReportedAt = unixNanoToTime(reportedAt)
		out = append(out, st)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("server/store: list worker update state: %w", err)
	}
	return out, nil
}

func (s *Store) DeleteWorkerUpdateState(ctx context.Context, workerID string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM worker_update_state WHERE worker_id = ?`, workerID); err != nil {
		return fmt.Errorf("server/store: delete update state for %q: %w", workerID, err)
	}
	return nil
}
