package store

//nolint:revive // blank import registers the sqlite driver
import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/acumen-ai-org/robotdreams/internal/identity"
	"github.com/acumen-ai-org/robotdreams/internal/orgchart"
	"github.com/acumen-ai-org/robotdreams/pkg/security"
)

const DBFileName = "control.db"

const schema = `
CREATE TABLE IF NOT EXISTS workers (
	id            TEXT PRIMARY KEY,
	role          TEXT NOT NULL,
	reports_to    TEXT NOT NULL,
	public_key    BLOB,
	connected_at  INTEGER NOT NULL,
	last_seen_at  INTEGER NOT NULL,
	status        TEXT NOT NULL,
	metadata_json TEXT
);
CREATE INDEX IF NOT EXISTS idx_workers_reports_to ON workers(reports_to);

CREATE TABLE IF NOT EXISTS server_config (
	id            INTEGER PRIMARY KEY CHECK (id = 1),
	messaging_uri TEXT NOT NULL,
	storage_uri   TEXT NOT NULL,
	updated_at    INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS report_instances (
	id          INTEGER PRIMARY KEY,
	definition  TEXT NOT NULL,
	scope       TEXT NOT NULL,
	producer    TEXT NOT NULL DEFAULT '',
	produced_at TEXT NOT NULL,
	payload     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_report_instances_def_scope ON report_instances(definition, scope);

CREATE TABLE IF NOT EXISTS report_events (
	id         INTEGER PRIMARY KEY,
	definition TEXT NOT NULL,
	scope      TEXT NOT NULL,
	at         TEXT NOT NULL,
	type       TEXT NOT NULL,
	severity   TEXT NOT NULL,
	label      TEXT NOT NULL DEFAULT '',
	attrs      TEXT
);
CREATE INDEX IF NOT EXISTS idx_report_events_def_scope ON report_events(definition, scope);

CREATE TABLE IF NOT EXISTS update_announcements (
	id              TEXT PRIMARY KEY,
	kind            TEXT NOT NULL,
	version         TEXT NOT NULL,
	source          TEXT NOT NULL DEFAULT '',
	min_version     TEXT NOT NULL DEFAULT '',
	severity        TEXT NOT NULL DEFAULT '',
	notes           TEXT NOT NULL DEFAULT '',
	announced_by    TEXT NOT NULL DEFAULT '',
	announced_at    INTEGER NOT NULL,
	recipient_count INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_update_announcements_kind ON update_announcements(kind, announced_at);

CREATE TABLE IF NOT EXISTS worker_update_state (
	worker_id       TEXT NOT NULL,
	kind            TEXT NOT NULL,
	current_version TEXT NOT NULL DEFAULT '',
	announcement_id TEXT NOT NULL DEFAULT '',
	target_version  TEXT NOT NULL DEFAULT '',
	status          TEXT NOT NULL DEFAULT '',
	detail          TEXT NOT NULL DEFAULT '',
	reported_at     INTEGER NOT NULL,
	PRIMARY KEY (worker_id, kind)
);
CREATE INDEX IF NOT EXISTS idx_worker_update_state_kind ON worker_update_state(kind, current_version);

CREATE TABLE IF NOT EXISTS worker_apps (
	worker_id   TEXT PRIMARY KEY,
	url         TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	declared_at INTEGER NOT NULL
);
`

var ErrNotFound = errors.New("server/store: not found")

var ErrNoConfig = errors.New("server/store: server not configured")

var ErrUnsupportedBackend = errors.New("server/store: unsupported control-plane store backend")

type Store struct {
	db          *sql.DB
	clock       security.Clock
	revocations *identity.SQLiteRevocationStore
}

func Open(dataDir string, clock security.Clock) (*Store, error) {
	if dataDir == "" {
		return nil, fmt.Errorf("server/store: open: empty data directory")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("server/store: create data directory %q: %w", dataDir, err)
	}
	return openFile(filepath.Join(dataDir, DBFileName), clock)
}

func OpenURI(uri string, clock security.Clock) (*Store, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("server/store: invalid store URI %q: %w", uri, err)
	}

	switch u.Scheme {
	case "", "file", "sqlite":
		path := u.Path
		if u.Host != "" {
			path = u.Host + u.Path
		}
		if path == "" {
			path = u.Opaque
		}
		if path == "" {
			return nil, fmt.Errorf("server/store: store URI %q is missing a database path", uri)
		}
		if dir := filepath.Dir(path); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o700); err != nil {
				return nil, fmt.Errorf("server/store: create data directory %q: %w", dir, err)
			}
		}
		return openFile(path, clock)
	case "postgres", "postgresql":
		return nil, fmt.Errorf("server/store: postgres control-plane store not yet implemented: %w", ErrUnsupportedBackend)
	default:
		return nil, fmt.Errorf("server/store: unknown control-plane store scheme %q (want \"sqlite\" or \"postgres\"): %w", u.Scheme, ErrUnsupportedBackend)
	}
}

func openFile(path string, clock security.Clock) (*Store, error) {
	if clock == nil {
		clock = security.RealClock{}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("server/store: open %q: %w", path, err)
	}
	db.SetMaxOpenConns(1)

	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=5000",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("server/store: %s: %w", pragma, err)
		}
	}

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("server/store: migrate schema: %w", err)
	}
	if _, err := db.Exec(schedulesSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("server/store: migrate schedules schema: %w", err)
	}

	rev, err := identity.NewSQLiteRevocationStoreFromDB(db, clock)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("server/store: migrate revocation schema: %w", err)
	}

	return &Store{db: db, clock: clock, revocations: rev}, nil
}

func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) Revocations() *identity.SQLiteRevocationStore { return s.revocations }

func (s *Store) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("server/store: close: %w", err)
	}
	return nil
}

func (s *Store) UpsertWorker(ctx context.Context, w orgchart.Worker) error {
	if w.ID == "" {
		return fmt.Errorf("server/store: upsert worker: empty worker ID")
	}

	var metadataJSON sql.NullString
	if w.Metadata != nil {
		b, err := json.Marshal(w.Metadata)
		if err != nil {
			return fmt.Errorf("server/store: marshal metadata for %q: %w", w.ID, err)
		}
		metadataJSON = sql.NullString{String: string(b), Valid: true}
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO workers (id, role, reports_to, public_key, connected_at, last_seen_at, status, metadata_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			role          = excluded.role,
			reports_to    = excluded.reports_to,
			public_key    = excluded.public_key,
			connected_at  = excluded.connected_at,
			last_seen_at  = excluded.last_seen_at,
			status        = excluded.status,
			metadata_json = excluded.metadata_json`,
		w.ID, w.Role, w.ReportsTo, w.PublicKey,
		timeToUnixNano(w.ConnectedAt), timeToUnixNano(w.LastSeenAt),
		string(w.Status), metadataJSON,
	)
	if err != nil {
		return fmt.Errorf("server/store: upsert worker %q: %w", w.ID, err)
	}
	return nil
}

func (s *Store) GetWorker(ctx context.Context, id string) (orgchart.Worker, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, role, reports_to, public_key, connected_at, last_seen_at, status, metadata_json
		FROM workers WHERE id = ?`, id)

	w, err := scanWorker(row)
	if errors.Is(err, sql.ErrNoRows) {
		return orgchart.Worker{}, fmt.Errorf("server/store: get worker %q: %w", id, ErrNotFound)
	}
	if err != nil {
		return orgchart.Worker{}, fmt.Errorf("server/store: get worker %q: %w", id, err)
	}
	return w, nil
}

func (s *Store) ListWorkers(ctx context.Context) ([]orgchart.Worker, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, role, reports_to, public_key, connected_at, last_seen_at, status, metadata_json
		FROM workers ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("server/store: list workers: %w", err)
	}
	defer rows.Close()

	out := make([]orgchart.Worker, 0)
	for rows.Next() {
		w, err := scanWorker(rows)
		if err != nil {
			return nil, fmt.Errorf("server/store: list workers: %w", err)
		}
		out = append(out, w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("server/store: iterate worker rows: %w", err)
	}
	return out, nil
}

func (s *Store) DeleteWorker(ctx context.Context, id string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM workers WHERE id = ?`, id); err != nil {
		return fmt.Errorf("server/store: delete worker %q: %w", id, err)
	}
	if err := s.DeleteWorkerUpdateState(ctx, id); err != nil {
		return err
	}
	return s.DeleteWorkerApp(ctx, id)
}

type scanner interface {
	Scan(dest ...any) error
}

func scanWorker(sc scanner) (orgchart.Worker, error) {
	var (
		id           string
		role         string
		reportsTo    string
		publicKey    []byte
		connectedAt  int64
		lastSeenAt   int64
		status       string
		metadataJSON sql.NullString
	)
	if err := sc.Scan(&id, &role, &reportsTo, &publicKey, &connectedAt, &lastSeenAt, &status, &metadataJSON); err != nil {
		return orgchart.Worker{}, err
	}

	w := orgchart.Worker{
		ID:          id,
		Role:        role,
		ReportsTo:   reportsTo,
		PublicKey:   publicKey,
		ConnectedAt: unixNanoToTime(connectedAt),
		LastSeenAt:  unixNanoToTime(lastSeenAt),
		Status:      orgchart.Status(status),
	}
	if metadataJSON.Valid {
		var md map[string]string
		if err := json.Unmarshal([]byte(metadataJSON.String), &md); err != nil {
			return orgchart.Worker{}, fmt.Errorf("unmarshal metadata for %q: %w", id, err)
		}
		w.Metadata = md
	}
	return w, nil
}

func (s *Store) SetConfig(ctx context.Context, messagingURI, storageURI string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO server_config (id, messaging_uri, storage_uri, updated_at)
		VALUES (1, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			messaging_uri = excluded.messaging_uri,
			storage_uri   = excluded.storage_uri,
			updated_at    = excluded.updated_at`,
		messagingURI, storageURI, s.clock.Now().UnixNano(),
	)
	if err != nil {
		return fmt.Errorf("server/store: set config: %w", err)
	}
	return nil
}

func (s *Store) GetConfig(ctx context.Context) (messagingURI, storageURI string, err error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT messaging_uri, storage_uri FROM server_config WHERE id = 1`)

	err = row.Scan(&messagingURI, &storageURI)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", fmt.Errorf("server/store: get config: %w", ErrNoConfig)
	}
	if err != nil {
		return "", "", fmt.Errorf("server/store: get config: %w", err)
	}
	return messagingURI, storageURI, nil
}

func (s *Store) ConfigUpdatedAt(ctx context.Context) (time.Time, error) {
	var updatedAt int64
	err := s.db.QueryRowContext(ctx,
		`SELECT updated_at FROM server_config WHERE id = 1`).Scan(&updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, fmt.Errorf("server/store: config updated_at: %w", ErrNoConfig)
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("server/store: config updated_at: %w", err)
	}
	return unixNanoToTime(updatedAt), nil
}

func (s *Store) Health(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("server/store: health: %w", err)
	}
	return nil
}

func timeToUnixNano(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixNano()
}

func unixNanoToTime(nanos int64) time.Time {
	if nanos == 0 {
		return time.Time{}
	}
	return time.Unix(0, nanos).UTC()
}
