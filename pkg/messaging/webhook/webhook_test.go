package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/acumen-ai-org/robotdreams/pkg/messaging"
	"github.com/acumen-ai-org/robotdreams/pkg/messaging/testsuite"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(start time.Time) *fakeClock {
	return &fakeClock{now: start}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func TestConformance(t *testing.T) {
	testsuite.RunConformance(t, func() messaging.MessagingBackend {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(srv.Close)

		backend, err := New(WithDefaultCallbackURL(srv.URL), WithPollInterval(5*time.Millisecond))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		return backend
	})
}

func mustBackend(t *testing.T, opts ...Option) *Backend {
	t.Helper()
	b, err := New(opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { b.Close() })
	return b
}

func testEnvelope(id, to string) messaging.Envelope {
	return messaging.Envelope{
		ID:      id,
		Type:    messaging.TypeStatusUpdate,
		From:    "worker-a",
		To:      to,
		Subject: "test",
		Body:    json.RawMessage(`{}`),
	}
}

func TestEmit_SuccessDeliversImmediately(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	clock := newFakeClock(time.Unix(1000, 0))
	b := mustBackend(t, WithClock(clock), WithDefaultCallbackURL(srv.URL))

	env := testEnvelope("e1", "worker-x")
	if err := b.Emit(context.Background(), env); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("callback invoked %d times, want 1", got)
	}
	pending, failed := b.Stats()
	if pending != 0 || failed != 0 {
		t.Fatalf("Stats after success = (pending=%d, failed=%d), want (0,0)", pending, failed)
	}
}

func TestRetry_BackoffIncreasesAndEventuallySucceeds(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	start := time.Unix(2000, 0)
	clock := newFakeClock(start)
	policy := RetryPolicy{
		InitialDelay: 1 * time.Second,
		MaxDelay:     10 * time.Second,
		Multiplier:   2,
		MaxAttempts:  5,
	}
	b := mustBackend(t, WithClock(clock), WithDefaultCallbackURL(srv.URL), WithRetryPolicy(policy), WithPollInterval(time.Hour))

	env := testEnvelope("e2", "worker-x")
	if err := b.Emit(context.Background(), env); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("after Emit, callback invoked %d times, want 1 (first attempt)", got)
	}

	attempts, next1, delivered, failed, ok := b.Inspect("e2")
	if !ok {
		t.Fatal("Inspect: envelope not found in outbox after first failure")
	}
	if delivered || failed {
		t.Fatalf("after 1 failure: delivered=%v failed=%v, want both false", delivered, failed)
	}
	if attempts != 1 {
		t.Fatalf("after 1 failure: attempts=%d, want 1", attempts)
	}
	wantNext1 := start.Add(policy.InitialDelay)
	if !next1.Equal(wantNext1) {
		t.Fatalf("next attempt scheduled at %v, want %v", next1, wantNext1)
	}

	b.Tick(context.Background())
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("callback invoked %d times before backoff elapsed, want still 1", got)
	}

	clock.Advance(policy.InitialDelay)
	b.Tick(context.Background())
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("callback invoked %d times after 1st retry, want 2", got)
	}
	_, next2, delivered, failed, ok := b.Inspect("e2")
	if !ok || delivered || failed {
		t.Fatalf("after 2nd failure: ok=%v delivered=%v failed=%v", ok, delivered, failed)
	}
	delay1 := next1.Sub(start)
	delay2 := next2.Sub(clock.Now())
	if delay2 <= delay1 {
		t.Fatalf("backoff did not increase: delay1=%v delay2=%v", delay1, delay2)
	}

	clock.Advance(delay2)
	b.Tick(context.Background())
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("callback invoked %d times after 2nd retry, want 3", got)
	}
	_, _, delivered, failed, ok = b.Inspect("e2")
	if !ok || !delivered || failed {
		t.Fatalf("after success: ok=%v delivered=%v failed=%v, want ok=true delivered=true failed=false", ok, delivered, failed)
	}

	pending, failedCount := b.Stats()
	if pending != 0 || failedCount != 0 {
		t.Fatalf("Stats after eventual success = (pending=%d, failed=%d), want (0,0)", pending, failedCount)
	}
}

func TestRetry_ExceedsMaxAttemptsStopsRetrying(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	clock := newFakeClock(time.Unix(3000, 0))
	policy := RetryPolicy{
		InitialDelay: 1 * time.Second,
		MaxDelay:     4 * time.Second,
		Multiplier:   2,
		MaxAttempts:  3,
	}
	b := mustBackend(t, WithClock(clock), WithDefaultCallbackURL(srv.URL), WithRetryPolicy(policy), WithPollInterval(time.Hour))

	env := testEnvelope("e3", "worker-x")
	if err := b.Emit(context.Background(), env); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	for i := 0; i < policy.MaxAttempts; i++ {
		_, next, delivered, failed, ok := b.Inspect("e3")
		if !ok {
			t.Fatalf("Inspect: envelope not found (iteration %d)", i)
		}
		if failed {
			break
		}
		if delivered {
			t.Fatalf("envelope unexpectedly delivered by an always-failing callback")
		}
		if next.After(clock.Now()) {
			clock.Advance(next.Sub(clock.Now()))
		}
		b.Tick(context.Background())
	}

	attempts, _, delivered, failed, ok := b.Inspect("e3")
	if !ok {
		t.Fatal("Inspect: envelope not found after retry loop")
	}
	if delivered {
		t.Fatal("envelope marked delivered by an always-failing callback")
	}
	if !failed {
		t.Fatalf("envelope not marked failed after %d attempts (MaxAttempts=%d)", attempts, policy.MaxAttempts)
	}
	if attempts != policy.MaxAttempts {
		t.Fatalf("attempts=%d, want %d (MaxAttempts)", attempts, policy.MaxAttempts)
	}
	if got := atomic.LoadInt32(&calls); int(got) != policy.MaxAttempts {
		t.Fatalf("callback invoked %d times, want exactly %d (MaxAttempts, no over-retry)", got, policy.MaxAttempts)
	}

	clock.Advance(time.Hour)
	b.Tick(context.Background())
	if got := atomic.LoadInt32(&calls); int(got) != policy.MaxAttempts {
		t.Fatalf("callback invoked again after permanent failure: calls=%d, want still %d", got, policy.MaxAttempts)
	}

	pending, failedCount := b.Stats()
	if pending != 0 || failedCount != 1 {
		t.Fatalf("Stats = (pending=%d, failed=%d), want (0,1)", pending, failedCount)
	}
	if err := b.Health(context.Background()); err == nil {
		t.Fatal("Health: want error when an envelope has permanently failed, got nil")
	}
}

func TestClose_NoGoroutineLeak(t *testing.T) {
	b, err := New(WithPollInterval(time.Millisecond))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := b.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not return promptly; background loop may be leaking")
	}

	if err := b.Health(context.Background()); err == nil {
		t.Fatal("Health after Close: want error, got nil")
	}
}

func TestSubscribe_UnsubscribeRacingDeliveryNeverPanics(t *testing.T) {
	b := mustBackend(t, WithPollInterval(time.Hour))
	env := testEnvelope("race", "worker-x")

	const iterations = 2000
	for i := 0; i < iterations; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		ch, err := b.Subscribe(ctx, "worker-x")
		if err != nil {
			t.Fatalf("Subscribe: %v", err)
		}

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			cancel()
		}()
		go func() {
			defer wg.Done()
			b.onDelivered(env)
		}()
		wg.Wait()

		deadline := time.After(5 * time.Second)
		for {
			select {
			case _, ok := <-ch:
				if !ok {
					goto next
				}
			case <-deadline:
				t.Fatalf("iteration %d: subscribe channel never closed after cancel", i)
			}
		}
	next:
	}
}

func TestOutbox_TerminalItemsAreBounded(t *testing.T) {
	b := mustBackend(t,
		WithRetryPolicy(RetryPolicy{InitialDelay: time.Second, MaxDelay: time.Second, Multiplier: 1, MaxAttempts: 1}),
		WithPollInterval(time.Hour))

	const extra = 5
	for i := 0; i < maxRetainedTerminal+extra; i++ {
		if err := b.Emit(context.Background(), testEnvelope(fmt.Sprintf("f%d", i), "nobody")); err != nil {
			t.Fatalf("Emit %d: %v", i, err)
		}
	}

	b.out.mu.Lock()
	n := len(b.out.itemsByID)
	b.out.mu.Unlock()
	if n != maxRetainedTerminal {
		t.Fatalf("outbox retains %d terminal items, want exactly %d", n, maxRetainedTerminal)
	}
	if _, _, _, _, ok := b.Inspect("f0"); ok {
		t.Fatal("oldest failed envelope still inspectable past the retention cap")
	}
	if _, _, _, failed, ok := b.Inspect(fmt.Sprintf("f%d", maxRetainedTerminal+extra-1)); !ok || !failed {
		t.Fatalf("newest failed envelope: ok=%v failed=%v, want both true", ok, failed)
	}
	if err := b.Health(context.Background()); err == nil {
		t.Fatal("Health: want error while failed envelopes are retained, got nil")
	}
}
