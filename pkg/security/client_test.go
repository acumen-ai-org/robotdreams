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

type countingSource struct {
	ttl           time.Duration
	n             int64
	failNextFetch atomic.Bool
}

func (s *countingSource) FetchToken(_ context.Context) (Token, error) {
	if s.failNextFetch.CompareAndSwap(true, false) {
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

	const numReaders = 8
	const readDuration = 600 * time.Millisecond
	const readerPauseSoRefreshGoroutineGetsCPU = 200 * time.Microsecond

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
				time.Sleep(readerPauseSoRefreshGoroutineGetsCPU)
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

	source.failNextFetch.Store(true)

	deadline := time.Now().Add(500 * time.Millisecond)
	for errCount.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if errCount.Load() == 0 {
		t.Fatal("injected refresh failure was never observed")
	}

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
