package embedded

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver.

	"github.com/acumen-ai-org/robotdreams/pkg/messaging"
)

// schema creates the messages table if it does not already exist. rowid is
// SQLite's implicit, monotonically increasing integer primary key; it is
// used as the delivery cursor for polling subscriptions because it orders
// rows by insertion even when created_at timestamps collide.
const schema = `
CREATE TABLE IF NOT EXISTS messages (
	id               TEXT PRIMARY KEY,
	type             TEXT NOT NULL,
	from_worker      TEXT NOT NULL,
	to_worker        TEXT NOT NULL,
	subject          TEXT NOT NULL,
	body             TEXT NOT NULL,
	storage_ptr_json TEXT,
	created_at       INTEGER NOT NULL,
	causation_id     TEXT,
	acked_at         INTEGER
);
CREATE INDEX IF NOT EXISTS idx_messages_to_worker ON messages(to_worker);
`

// sqliteStore wraps a *sql.DB with the raw SQL and query helpers backing
// the embedded messaging.MessagingBackend. It holds no business logic
// beyond translating between Envelope/Ack values and rows; the
// MessagingBackend implementation lives in embedded.go.
type sqliteStore struct {
	db *sql.DB
}

// openStore opens (creating if necessary) a SQLite database at path and
// ensures the schema exists.
func openStore(path string) (*sqliteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("messaging/embedded: open %q: %w", path, err)
	}
	// SQLite handles a single writer at a time; serialize access at the
	// database/sql level rather than fighting SQLITE_BUSY errors under
	// concurrent goroutines.
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("messaging/embedded: migrate schema: %w", err)
	}
	return &sqliteStore{db: db}, nil
}

// insert writes env as a new row, along with its created_at unix-nano
// timestamp supplied by the caller (via the backend's Clock).
func (s *sqliteStore) insert(ctx context.Context, env messaging.Envelope, createdAtUnixNano int64) error {
	var storagePtrJSON sql.NullString
	if env.StoragePtr != nil {
		b, err := json.Marshal(env.StoragePtr)
		if err != nil {
			return fmt.Errorf("messaging/embedded: marshal storage pointer: %w", err)
		}
		storagePtrJSON = sql.NullString{String: string(b), Valid: true}
	}

	var causationID sql.NullString
	if env.CausationID != "" {
		causationID = sql.NullString{String: env.CausationID, Valid: true}
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO messages (id, type, from_worker, to_worker, subject, body, storage_ptr_json, created_at, causation_id, acked_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)`,
		env.ID, string(env.Type), env.From, env.To, env.Subject, string(env.Body), storagePtrJSON, createdAtUnixNano, causationID,
	)
	if err != nil {
		return fmt.Errorf("messaging/embedded: insert message %q: %w", env.ID, err)
	}
	return nil
}

// ack sets acked_at on the row matching messageID, provided that row is
// addressed to workerID — a message can only be acked by its recipient,
// and a row that exists but belongs to someone else reports found=false
// exactly like a missing one (see MessagingBackend.Ack). It is a no-op
// (not an error) if the message is already acked; see Backend.Ack for
// the not-found behavior.
func (s *sqliteStore) ack(ctx context.Context, messageID, workerID string, ackedAtUnixNano int64) (found bool, err error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE messages SET acked_at = ? WHERE id = ? AND to_worker = ? AND acked_at IS NULL`,
		ackedAtUnixNano, messageID, workerID,
	)
	if err != nil {
		return false, fmt.Errorf("messaging/embedded: ack message %q: %w", messageID, err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return true, nil
	}

	// Either the message isn't this worker's (or doesn't exist), or it
	// was already acked. Tell those apart so callers can decide what
	// "found" means for them.
	var exists int
	err = s.db.QueryRowContext(ctx, `SELECT 1 FROM messages WHERE id = ? AND to_worker = ?`, messageID, workerID).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("messaging/embedded: check message %q: %w", messageID, err)
	}
	return true, nil
}

// pollSince returns rows addressed to workerID, not yet acked, with rowid
// strictly greater than sinceRowID, ordered by rowid ascending. It also
// returns the highest rowid observed (0 if no rows), so the caller can
// advance its cursor.
func (s *sqliteStore) pollSince(ctx context.Context, workerID string, sinceRowID int64) ([]messaging.Envelope, int64, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT rowid, id, type, from_worker, to_worker, subject, body, storage_ptr_json, created_at, causation_id
		FROM messages
		WHERE to_worker = ? AND acked_at IS NULL AND rowid > ?
		ORDER BY rowid ASC`,
		workerID, sinceRowID,
	)
	if err != nil {
		return nil, sinceRowID, fmt.Errorf("messaging/embedded: poll: %w", err)
	}
	defer rows.Close()

	envs, maxRowID, err := scanEnvelopes(rows, sinceRowID)
	if err != nil {
		return nil, sinceRowID, err
	}
	return envs, maxRowID, nil
}

// tail returns rows matching filter, ordered by rowid descending (most
// recent first), limited to filter.Limit rows if positive.
func (s *sqliteStore) tail(ctx context.Context, filter messaging.TailFilter) ([]messaging.Envelope, error) {
	query := `
		SELECT rowid, id, type, from_worker, to_worker, subject, body, storage_ptr_json, created_at, causation_id
		FROM messages
		WHERE 1 = 1`
	var args []any

	if filter.WorkerID != "" {
		query += ` AND to_worker = ?`
		args = append(args, filter.WorkerID)
	}
	if !filter.Since.IsZero() {
		query += ` AND created_at >= ?`
		args = append(args, filter.Since.UnixNano())
	}
	query += ` ORDER BY rowid DESC`
	if filter.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, filter.Limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("messaging/embedded: tail: %w", err)
	}
	defer rows.Close()

	envs, _, err := scanEnvelopes(rows, 0)
	if err != nil {
		return nil, err
	}
	return envs, nil
}

// scanEnvelopes reads rows produced by the queries above, which all
// select the same column set. It returns the decoded envelopes and the
// highest rowid seen (or initial if rows was empty).
func scanEnvelopes(rows *sql.Rows, initial int64) ([]messaging.Envelope, int64, error) {
	maxRowID := initial
	var envs []messaging.Envelope

	for rows.Next() {
		var (
			rowID          int64
			id             string
			typ            string
			from           string
			to             string
			subject        string
			body           string
			storagePtrJSON sql.NullString
			createdAt      int64
			causationID    sql.NullString
		)
		if err := rows.Scan(&rowID, &id, &typ, &from, &to, &subject, &body, &storagePtrJSON, &createdAt, &causationID); err != nil {
			return nil, initial, fmt.Errorf("messaging/embedded: scan row: %w", err)
		}
		if rowID > maxRowID {
			maxRowID = rowID
		}

		env := messaging.Envelope{
			ID:          id,
			Type:        messaging.MessageType(typ),
			From:        from,
			To:          to,
			Subject:     subject,
			Body:        json.RawMessage(body),
			CreatedAt:   unixNanoToTime(createdAt),
			CausationID: causationID.String,
		}
		if storagePtrJSON.Valid {
			var ptr messaging.StoragePointer
			if err := json.Unmarshal([]byte(storagePtrJSON.String), &ptr); err != nil {
				return nil, initial, fmt.Errorf("messaging/embedded: unmarshal storage pointer for %q: %w", id, err)
			}
			env.StoragePtr = &ptr
		}
		envs = append(envs, env)
	}
	if err := rows.Err(); err != nil {
		return nil, initial, fmt.Errorf("messaging/embedded: iterate rows: %w", err)
	}
	return envs, maxRowID, nil
}

// ping checks the underlying database connection is usable.
func (s *sqliteStore) ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// close closes the underlying database connection.
func (s *sqliteStore) close() error {
	return s.db.Close()
}
