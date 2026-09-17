package security

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// TokenSource fetches a fresh token for a worker.
type TokenSource interface {
	FetchToken(ctx context.Context) (Token, error)
}

// TokenSourceFunc adapts a plain function to a TokenSource.
type TokenSourceFunc func(ctx context.Context) (Token, error)

// FetchToken implements TokenSource.
func (f TokenSourceFunc) FetchToken(ctx context.Context) (Token, error) {
	return f(ctx)
}

// RefreshFraction is how far into a token's TTL a WorkerClient schedules its next refresh.
const RefreshFraction = 0.7

// MinRefreshInterval is the shortest wait between two token fetches.
const MinRefreshInterval = 10 * time.Millisecond

// WorkerClient holds a worker's Ed25519 keypair and keeps its token fresh in the background.
type WorkerClient struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey

	source TokenSource
	clock  Clock

	current atomic.Pointer[Token]

	lifecycle sync.Mutex
	cancel    context.CancelFunc
	done      chan struct{}
	started   bool

	onRefreshError func(error)
}

// NewWorkerClient constructs a WorkerClient; a nil clock means RealClock.
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

// Token returns the current token, or an error before the first Refresh or Start.
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

// Start fetches an initial token, then refreshes it in the background until ctx ends or Stop is called.
func (c *WorkerClient) Start(ctx context.Context) error {
	c.lifecycle.Lock()
	if c.started {
		c.lifecycle.Unlock()
		return nil
	}
	c.started = true
	runCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	c.done = make(chan struct{})
	c.lifecycle.Unlock()

	if _, err := c.Refresh(runCtx); err != nil {
		c.lifecycle.Lock()
		c.started = false
		c.lifecycle.Unlock()
		cancel()
		return err
	}

	go c.refreshLoop(runCtx)
	return nil
}

// Stop halts the background refresh goroutine and waits for it to exit.
func (c *WorkerClient) Stop() {
	c.lifecycle.Lock()
	if !c.started {
		c.lifecycle.Unlock()
		return
	}
	cancel := c.cancel
	done := c.done
	c.lifecycle.Unlock()

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
			select {
			case <-ctx.Done():
				return
			case <-time.After(MinRefreshInterval):
			}
		}
	}
}

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
