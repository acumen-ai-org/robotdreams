// Package embedded provides a zero-config messaging.MessagingBackend
// backed by an embedded SQLite database (via the pure-Go
// modernc.org/sqlite driver, chosen deliberately over mattn/go-sqlite3 so
// Robot Dreams cross-compiles without cgo). It is registered under the
// "queue" URI scheme with host "sqlite", e.g.
// "queue://sqlite/var/data/messages.db".
//
// A blank import of this package (e.g. in cmd/dream, added in a later
// phase) is enough to make that scheme available via
// pkg/messaging.New — this package must not be imported by
// pkg/messaging itself, keeping the registry decoupled from concrete
// backends.
package embedded

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/acumen-ai-org/robotdreams/pkg/messaging"
	"github.com/acumen-ai-org/robotdreams/pkg/security"
)

// pollInterval is how often a Subscribe goroutine checks for new
// messages. Polling trades sub-second delivery latency for
// implementation simplicity: it is an acceptable default for the
// zero-config embedded backend in this phase, since correctness (no
// missed or duplicated messages) matters more than immediacy here. A
// future phase may replace this with a proper notification mechanism
// (e.g. SQLite's update hook, or a different backend entirely) without
// changing the MessagingBackend contract.
const pollInterval = 300 * time.Millisecond

// Backend is a messaging.MessagingBackend backed by an embedded SQLite
// database. It is safe for concurrent use by multiple goroutines.
type Backend struct {
	store *sqliteStore
	clock security.Clock

	// closed is closed by Close so every Subscribe poller exits, even
	// when its own ctx is still live; without it a subscriber would keep
	// ticking against a closed database until its ctx ended.
	closed    chan struct{}
	closeOnce sync.Once
}

// Option configures a Backend constructed by New.
type Option func(*Backend)

// WithClock overrides the Clock used for CreatedAt/AckedAt timestamps.
// Defaults to security.RealClock{}.
func WithClock(clock security.Clock) Option {
	return func(b *Backend) {
		b.clock = clock
	}
}

// New opens (creating if necessary) a SQLite-backed messaging backend at
// dbPath, running schema migration as needed, and returns it ready for
// use. dbPath should be a filesystem path to a database file; a fresh
// temp file per test (e.g. filepath.Join(t.TempDir(), "test.db")) is the
// simplest way to get an isolated backend in tests.
func New(dbPath string, opts ...Option) (*Backend, error) {
	store, err := openStore(dbPath)
	if err != nil {
		return nil, err
	}

	b := &Backend{
		store:  store,
		clock:  security.RealClock{},
		closed: make(chan struct{}),
	}
	for _, opt := range opts {
		opt(b)
	}
	return b, nil
}

// Emit implements messaging.MessagingBackend.
func (b *Backend) Emit(ctx context.Context, env messaging.Envelope) error {
	if env.CreatedAt.IsZero() {
		env.CreatedAt = b.clock.Now()
	}
	return b.store.insert(ctx, env, env.CreatedAt.UnixNano())
}

// Subscribe implements messaging.MessagingBackend. It polls the
// underlying database on pollInterval for new, unacked messages
// addressed to workerID, tracking the highest rowid delivered so far in
// the polling goroutine's closure to avoid redelivering the same row on
// later ticks.
//
// The returned channel is closed, and the backing goroutine exits, once
// ctx is Done or the backend is Closed, whichever comes first. Any
// transient error encountered while polling is swallowed for that tick
// rather than crashing the goroutine or the caller. Only this goroutine
// ever sends on or closes ch, so the close can never race a send.
func (b *Backend) Subscribe(ctx context.Context, workerID string) (<-chan Envelope, error) {
	ch := make(chan messaging.Envelope)

	go func() {
		defer close(ch)

		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		var cursor int64 // highest rowid delivered so far.

		for {
			select {
			case <-ctx.Done():
				return
			case <-b.closed:
				return
			case <-ticker.C:
				envs, maxRowID, err := b.store.pollSince(ctx, workerID, cursor)
				if err != nil {
					// Transient error: skip this tick, keep polling
					// until ctx ends or the backend is closed.
					continue
				}
				cursor = maxRowID

				for _, env := range envs {
					select {
					case ch <- env:
					case <-ctx.Done():
						return
					case <-b.closed:
						return
					}
				}
			}
		}
	}()

	return ch, nil
}

// Envelope is an alias kept local to this file only for readability of
// the Subscribe signature above; it is messaging.Envelope.
type Envelope = messaging.Envelope

// Ack implements messaging.MessagingBackend. Acking an already-acked
// message is not an error (idempotent); acking a message ID that is
// unknown, or that is addressed to a worker other than ack.ByWorker,
// returns an error satisfying errors.Is(err, messaging.ErrNotFound) —
// the two cases are deliberately indistinguishable, per the interface.
func (b *Backend) Ack(ctx context.Context, ack messaging.Ack) error {
	at := ack.At
	if at.IsZero() {
		at = b.clock.Now()
	}
	found, err := b.store.ack(ctx, ack.MessageID, ack.ByWorker, at.UnixNano())
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("messaging/embedded: ack %q: %w", ack.MessageID, messaging.ErrNotFound)
	}
	return nil
}

// Tail implements messaging.MessagingBackend. Results are ordered most
// recent first (descending by insertion order), since callers typically
// want the latest activity for a worker.
func (b *Backend) Tail(ctx context.Context, filter messaging.TailFilter) ([]messaging.Envelope, error) {
	return b.store.tail(ctx, filter)
}

// Health implements messaging.MessagingBackend.
func (b *Backend) Health(ctx context.Context) error {
	return b.store.ping(ctx)
}

// Close implements messaging.MessagingBackend. It signals every live
// Subscribe goroutine to exit (closing its channel) and then closes the
// underlying database handle. A poller mid-query when the handle closes
// sees an error for that one tick and exits on the next select, so the
// order here is a courtesy, not a correctness requirement.
func (b *Backend) Close() error {
	b.closeOnce.Do(func() { close(b.closed) })
	return b.store.close()
}

func unixNanoToTime(nanos int64) time.Time {
	if nanos == 0 {
		return time.Time{}
	}
	return time.Unix(0, nanos).UTC()
}

func init() {
	messaging.Register("queue", dispatch)
}

// dispatch implements the "queue" scheme registered with pkg/messaging's
// registry. The URI host selects the variant:
//
//   - "sqlite": queue://sqlite/path/to/file.db — this package's embedded
//     SQLite backend. The remainder of the URI (host stripped) is used as
//     the database file path.
//   - "postgres": queue://postgres/... — not implemented in this phase;
//     returns a clear error rather than silently falling back to sqlite.
//
// This dispatcher lives here, not in pkg/messaging/registry.go, because
// that package must not import concrete backend packages; embedded
// self-registers for the "queue" scheme via this init().
func dispatch(u *url.URL) (messaging.MessagingBackend, error) {
	switch u.Host {
	case "sqlite":
		path := u.Path
		if path == "" {
			return nil, fmt.Errorf("messaging/embedded: queue://sqlite URI missing a database path, got %q", u.String())
		}
		return New(path)
	case "postgres":
		return nil, fmt.Errorf("messaging/embedded: queue://postgres backend is not yet implemented")
	default:
		return nil, fmt.Errorf("messaging/embedded: unknown queue variant %q (want \"sqlite\" or \"postgres\")", u.Host)
	}
}
