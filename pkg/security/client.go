package security

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// TokenSource fetches a fresh token for a worker. In production this calls
// the server's token endpoint (wired up in a later phase); in tests it can
// be a fake that returns short-TTL tokens deterministically. Implementations
// should prove possession of the worker's private key as part of fetching
// (e.g. by signing a server-issued nonce) — that mechanics is left to the
// concrete implementation, not this interface.
type TokenSource interface {
	FetchToken(ctx context.Context) (Token, error)
}

// TokenSourceFunc adapts a plain function to a TokenSource.
type TokenSourceFunc func(ctx context.Context) (Token, error)

// FetchToken implements TokenSource.
func (f TokenSourceFunc) FetchToken(ctx context.Context) (Token, error) {
	return f(ctx)
}

// RefreshFraction is how far into a token's TTL a WorkerClient schedules
// its next refresh (e.g. 0.7 = refresh at 70% of the way to expiry).
const RefreshFraction = 0.7

// MinRefreshInterval bounds how frequently the client will retry fetching a
// token, so a pathological zero/negative TTL (or a source that keeps
// failing) can't spin a tight loop.
const MinRefreshInterval = 10 * time.Millisecond

// WorkerClient holds a worker's Ed25519 keypair and current token, and
// keeps the token fresh via a background goroutine. It is safe for
// concurrent use: Token() always returns a consistent snapshot even while
// a refresh is in flight, via an atomic pointer swap.
type WorkerClient struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey

	source TokenSource
	clock  Clock

	current atomic.Pointer[Token]

	mu      sync.Mutex // guards start/stop lifecycle below
	cancel  context.CancelFunc
	done    chan struct{}
	started bool

	// onRefreshError, if set, is called (from the background goroutine)
	// whenever a refresh attempt fails. Primarily for tests/observability;
	// nil is fine and errors are simply retried.
	onRefreshError func(error)
}

// NewWorkerClient constructs a WorkerClient for the given keypair and
// token source. clock defaults to RealClock{} if nil.
func NewWorkerClient(priv ed25519.PrivateKey, pub ed25519.PublicKey, source TokenSource, clock Clock) *WorkerClient {
	if clock == nil {
		clock = RealClock{}
	}
	return &WorkerClient{
		privateKey: priv,
		publicKey:  pub,
		source:     source,
		clock:      clock,
	}
}

// PublicKey returns the worker's public key.
func (c *WorkerClient) PublicKey() ed25519.PublicKey { return c.publicKey }

// Token returns the current token. It may block briefly on first use if no
// token has been fetched yet (via Start or an explicit Refresh).
func (c *WorkerClient) Token() (Token, error) {
	t := c.current.Load()
	if t == nil {
		return Token{}, fmt.Errorf("security: no token available yet; call Refresh or Start first")
	}
	return *t, nil
}

// Refresh synchronously fetches a new token and installs it.
func (c *WorkerClient) Refresh(ctx context.Context) (Token, error) {
	tok, err := c.source.FetchToken(ctx)
	if err != nil {
		return Token{}, err
	}
	c.current.Store(&tok)
	return tok, nil
}

// Start fetches an initial token synchronously, then launches a background
// goroutine that refreshes it at RefreshFraction of its TTL, indefinitely,
// until ctx is canceled or Stop is called. Calling Start twice is a no-op
// after the first call.
func (c *WorkerClient) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.started {
		c.mu.Unlock()
		return nil
	}
	c.started = true
	runCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	c.done = make(chan struct{})
	c.mu.Unlock()

	if _, err := c.Refresh(runCtx); err != nil {
		c.mu.Lock()
		c.started = false
		c.mu.Unlock()
		cancel()
		return err
	}

	go c.refreshLoop(runCtx)
	return nil
}

// Stop halts the background refresh goroutine and waits for it to exit.
func (c *WorkerClient) Stop() {
	c.mu.Lock()
	if !c.started {
		c.mu.Unlock()
		return
	}
	cancel := c.cancel
	done := c.done
	c.mu.Unlock()

	cancel()
	<-done
}

func (c *WorkerClient) refreshLoop(ctx context.Context) {
	defer close(c.done)

	for {
		tok, err := c.Token()
		var wait time.Duration
		if err != nil {
			wait = MinRefreshInterval
		} else {
			wait = c.nextRefreshDelay(tok)
		}

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}

		if _, err := c.Refresh(ctx); err != nil {
			if c.onRefreshError != nil {
				c.onRefreshError(err)
			}
			// back off minimally and retry rather than spinning.
			select {
			case <-ctx.Done():
				return
			case <-time.After(MinRefreshInterval):
			}
		}
	}
}

// nextRefreshDelay computes how long to wait before the next refresh,
// targeting RefreshFraction of the token's TTL measured from IssuedAt.
func (c *WorkerClient) nextRefreshDelay(tok Token) time.Duration {
	ttl := tok.TTL()
	if ttl <= 0 {
		return MinRefreshInterval
	}
	refreshAt := tok.IssuedAt.Add(time.Duration(float64(ttl) * RefreshFraction))
	delay := refreshAt.Sub(c.clock.Now())
	if delay < MinRefreshInterval {
		return MinRefreshInterval
	}
	return delay
}
