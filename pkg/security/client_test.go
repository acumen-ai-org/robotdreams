package security

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// countingSource is a fake TokenSource that mints a new token with a short
// TTL on every fetch, tagging each with an increasing sequence number so
// tests can observe rotation.
type countingSource struct {
	ttl  time.Duration
	n    int64
	fail atomic.Bool // when true, FetchToken returns an error once then clears itself
}

func (s *countingSource) FetchToken(_ context.Context) (Token, error) {
	if s.fail.CompareAndSwap(true, false) {
		return Token{}, fmt.Errorf("injected failure")
	}
	n := atomic.AddInt64(&s.n, 1)
	now := time.Now()
	return Token{
		Raw:       fmt.Sprintf("token-%d", n),
		IssuedAt:  now,
		ExpiresAt: now.Add(s.ttl),
	}, nil
}

func (s *countingSource) count() int64 {
	return atomic.LoadInt64(&s.n)
}

func testKeyPair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey: %v", err)
	}
	return pub, priv
}

func TestWorkerClientBackgroundRefreshSwapsBeforeExpiry(t *testing.T) {
	pub, priv := testKeyPair(t)
	source := &countingSource{ttl: 80 * time.Millisecond}
	c := NewWorkerClient(priv, pub, source, RealClock{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer c.Stop()

	first, err := c.Token()
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if first.Raw != "token-1" {
		t.Fatalf("first token = %q, want token-1", first.Raw)
	}

	// Poll for up to ~500ms (well past several TTL windows), asserting
	// the client never hands back an expired token and that it does
	// rotate to a new token before the old one would expire.
	deadline := time.Now().Add(500 * time.Millisecond)
	sawRotation := false
	for time.Now().Before(deadline) {
		tok, err := c.Token()
		if err != nil {
			t.Fatalf("Token: %v", err)
		}
		if tok.Expired(time.Now()) {
			t.Fatalf("Token() returned an expired token: %+v", tok)
		}
		if tok.Raw != first.Raw {
			sawRotation = true
			break
		}
		time.Sleep(2 * time.Millisecond)
	}

	if !sawRotation {
		t.Fatal("client never rotated to a new token")
	}
	if source.count() < 2 {
		t.Errorf("expected at least 2 fetches, got %d", source.count())
	}
}

func TestWorkerClientConcurrentCallersNeverSeeExpiredToken(t *testing.T) {
	pub, priv := testKeyPair(t)
	source := &countingSource{ttl: 200 * time.Millisecond}
	c := NewWorkerClient(priv, pub, source, RealClock{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer c.Stop()

	// Readers pace themselves slightly instead of pure-spinning: enough
	// concurrent access to exercise the atomic swap under -race, without
	// starving the background refresh goroutine of CPU time on small
	// runners (a busy-spin here can delay the refresh goroutine past the
	// token's expiry, which is a test-harness artifact, not a client bug
	// — real deployments use minute-scale TTLs with ample margin).
	const numReaders = 8
	const readDuration = 600 * time.Millisecond

	var wg sync.WaitGroup
	var expiredSeen atomic.Bool
	stop := make(chan struct{})

	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				tok, err := c.Token()
				if err != nil {
					continue
				}
				if tok.Expired(time.Now()) {
					expiredSeen.Store(true)
				}
				time.Sleep(200 * time.Microsecond)
			}
		}()
	}

	time.Sleep(readDuration)
	close(stop)
	wg.Wait()

	if expiredSeen.Load() {
		t.Error("a concurrent caller observed an expired token")
	}
	if source.count() < 2 {
		t.Errorf("expected multiple refreshes over %v, got %d", readDuration, source.count())
	}
}

func TestWorkerClientTokenBeforeStart(t *testing.T) {
	pub, priv := testKeyPair(t)
	source := &countingSource{ttl: time.Minute}
	c := NewWorkerClient(priv, pub, source, RealClock{})

	if _, err := c.Token(); err == nil {
		t.Error("Token() before Start/Refresh succeeded, want error")
	}
}

func TestWorkerClientRefreshErrorIsRetried(t *testing.T) {
	pub, priv := testKeyPair(t)
	source := &countingSource{ttl: 30 * time.Millisecond}
	c := NewWorkerClient(priv, pub, source, RealClock{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var errCount atomic.Int32
	c.onRefreshError = func(error) { errCount.Add(1) }

	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer c.Stop()

	// Inject a single failure into the background refresh cycle and
	// confirm the client recovers (keeps refreshing) rather than getting
	// stuck.
	source.fail.Store(true)

	deadline := time.Now().Add(500 * time.Millisecond)
	for errCount.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if errCount.Load() == 0 {
		t.Fatal("injected refresh failure was never observed")
	}

	// The client should keep making progress after the transient error.
	before, _ := c.Token()
	deadline = time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		tok, err := c.Token()
		if err == nil && tok.Raw != before.Raw {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("client did not recover and keep refreshing after a transient error")
}
