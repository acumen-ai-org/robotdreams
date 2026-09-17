// Package embedded is the zero-config messaging.MessagingBackend on an embedded SQLite database, registered as queue://sqlite/<path>.
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

const pollInterval = 300 * time.Millisecond

// Backend is a messaging.MessagingBackend backed by an embedded SQLite database.
type Backend struct {
	store *sqliteStore
	clock security.Clock

	closed    chan struct{}
	closeOnce sync.Once
}

// Option configures a Backend constructed by New.
type Option func(*Backend)

// WithClock overrides the clock used for CreatedAt and Ack timestamps.
func WithClock(clock security.Clock) Option {
	return func(b *Backend) {
		b.clock = clock
	}
}

// New opens or creates the SQLite database at dbPath and migrates its schema.
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

// Subscribe implements messaging.MessagingBackend.
func (b *Backend) Subscribe(ctx context.Context, workerID string) (<-chan messaging.Envelope, error) {
	ch := make(chan messaging.Envelope)

	go func() {
		defer close(ch)

		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		var lastDeliveredRowID int64

		for {
			select {
			case <-ctx.Done():
				return
			case <-b.closed:
				return
			case <-ticker.C:
				envs, maxRowID, err := b.store.pollSince(ctx, workerID, lastDeliveredRowID)
				if err != nil {
					continue
				}
				lastDeliveredRowID = maxRowID

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

// Ack implements messaging.MessagingBackend.
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

// Tail implements messaging.MessagingBackend.
func (b *Backend) Tail(ctx context.Context, filter messaging.TailFilter) ([]messaging.Envelope, error) {
	return b.store.tail(ctx, filter)
}

// Health implements messaging.MessagingBackend.
func (b *Backend) Health(ctx context.Context) error {
	return b.store.ping(ctx)
}

// Close implements messaging.MessagingBackend.
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
