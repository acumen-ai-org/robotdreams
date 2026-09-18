package identity

//nolint:revive // blank import registers the sqlite driver
import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

const sqliteSingleWriterConns = 1

const revocationSchema = `
CREATE TABLE IF NOT EXISTS revocations (
	worker_id  TEXT PRIMARY KEY,
	reason     TEXT NOT NULL,
	revoked_at INTEGER NOT NULL
);
`

type RevocationRecord struct {
	WorkerID  string
	Reason    string
	RevokedAt time.Time
}

type SQLiteRevocationStore struct {
	db     *sql.DB
	clock  Clock
	ownsDB bool
}

var _ RevocationStore = (*SQLiteRevocationStore)(nil)

func NewSQLiteRevocationStore(dbPath string, clock Clock) (*SQLiteRevocationStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("identity: open revocation db %q: %w", dbPath, err)
	}
	db.SetMaxOpenConns(sqliteSingleWriterConns)

	store, err := NewSQLiteRevocationStoreFromDB(db, clock)
	if err != nil {
		db.Close()
		return nil, err
	}
	store.ownsDB = true
	return store, nil
}

func NewSQLiteRevocationStoreFromDB(db *sql.DB, clock Clock) (*SQLiteRevocationStore, error) {
	if db == nil {
		return nil, fmt.Errorf("identity: nil database handle")
	}
	if clock == nil {
		clock = RealClock{}
	}
	if _, err := db.Exec(revocationSchema); err != nil {
		return nil, fmt.Errorf("identity: migrate revocation schema: %w", err)
	}
	return &SQLiteRevocationStore{db: db, clock: clock}, nil
}

func (s *SQLiteRevocationStore) IsRevoked(ctx context.Context, workerID string) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM revocations WHERE worker_id = ?`, workerID,
	).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("identity: check revocation for %q: %w", workerID, err)
	}
	return true, nil
}

func (s *SQLiteRevocationStore) Revoke(ctx context.Context, workerID string, reason string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO revocations (worker_id, reason, revoked_at)
		VALUES (?, ?, ?)
		ON CONFLICT(worker_id) DO UPDATE SET reason = excluded.reason, revoked_at = excluded.revoked_at`,
		workerID, reason, s.clock.Now().UnixNano(),
	)
	if err != nil {
		return fmt.Errorf("identity: revoke worker %q: %w", workerID, err)
	}
	return nil
}

func (s *SQLiteRevocationStore) Unrevoke(ctx context.Context, workerID string) error {
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM revocations WHERE worker_id = ?`, workerID,
	); err != nil {
		return fmt.Errorf("identity: unrevoke worker %q: %w", workerID, err)
	}
	return nil
}

func (s *SQLiteRevocationStore) ListRevoked(ctx context.Context) ([]RevocationRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT worker_id, reason, revoked_at
		FROM revocations
		ORDER BY revoked_at DESC, worker_id ASC`)
	if err != nil {
		return nil, fmt.Errorf("identity: list revocations: %w", err)
	}
	defer rows.Close()

	var records []RevocationRecord
	for rows.Next() {
		var (
			workerID  string
			reason    string
			revokedAt int64
		)
		if err := rows.Scan(&workerID, &reason, &revokedAt); err != nil {
			return nil, fmt.Errorf("identity: scan revocation row: %w", err)
		}
		records = append(records, RevocationRecord{
			WorkerID:  workerID,
			Reason:    reason,
			RevokedAt: time.Unix(0, revokedAt),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("identity: iterate revocation rows: %w", err)
	}
	return records, nil
}

func (s *SQLiteRevocationStore) Close() error {
	if !s.ownsDB {
		return nil
	}
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("identity: close revocation db: %w", err)
	}
	return nil
}
