package temporal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"go.temporal.io/sdk/testsuite"

	"github.com/acumen-ai-org/robotdreams/pkg/messaging"
	msgtestsuite "github.com/acumen-ai-org/robotdreams/pkg/messaging/testsuite"
)

var (
	devServer *testsuite.DevServer
	hostPort  string
)

func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	srv, err := testsuite.StartDevServer(ctx, testsuite.DevServerOptions{})
	if err != nil {
		fmt.Fprintln(os.Stderr, "messaging/temporal: could not start Temporal dev server, skipping all tests in this package:", err)
		os.Exit(0)
	}
	devServer = srv
	hostPort = srv.FrontendHostPort()

	code := m.Run()

	if err := devServer.Stop(); err != nil {
		fmt.Fprintln(os.Stderr, "messaging/temporal: error stopping dev server:", err)
	}
	os.Exit(code)
}

const goroutineSlackAfterClose = 5

var mailboxCounter atomic.Int64

func newTestBackend(t *testing.T) *Backend {
	t.Helper()

	prefix := fmt.Sprintf("test-%d-%d", time.Now().UnixNano(), mailboxCounter.Add(1))
	b, err := New(
		WithHostPort(hostPort),
		WithNamespace("default"),
		WithMailboxPrefix(prefix),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return b
}

func TestConformance(t *testing.T) {
	msgtestsuite.RunConformance(t, func() messaging.MessagingBackend {
		return newTestBackend(t)
	})
}

func TestMailboxStartsOnFirstEmitAndPersists(t *testing.T) {
	b := newTestBackend(t)
	defer b.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	env1 := messaging.Envelope{
		ID: "e1", Type: messaging.TypeStatusUpdate, From: "a", To: "worker-1",
		Subject: "first", Body: json.RawMessage(`{}`), CreatedAt: time.Now(),
	}
	if err := b.Emit(ctx, env1); err != nil {
		t.Fatalf("Emit 1: %v", err)
	}

	env2 := messaging.Envelope{
		ID: "e2", Type: messaging.TypeStatusUpdate, From: "a", To: "worker-1",
		Subject: "second", Body: json.RawMessage(`{}`), CreatedAt: time.Now(),
	}
	if err := b.Emit(ctx, env2); err != nil {
		t.Fatalf("Emit 2: %v", err)
	}

	var got []messaging.Envelope
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		envs, err := b.Tail(ctx, messaging.TailFilter{WorkerID: "worker-1"})
		if err != nil {
			t.Fatalf("Tail: %v", err)
		}
		if len(envs) >= 2 {
			got = envs
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if len(got) < 2 {
		t.Fatalf("Tail returned %d envelopes after 2 Emits to the same worker, want 2 (single mailbox workflow): %+v", len(got), got)
	}
	ids := map[string]bool{}
	for _, e := range got {
		ids[e.ID] = true
	}
	if !ids["e1"] || !ids["e2"] {
		t.Fatalf("Tail result missing an envelope: got IDs %v", ids)
	}
}

func TestAckRemovesFromPending(t *testing.T) {
	b := newTestBackend(t)
	defer b.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	env := messaging.Envelope{
		ID: "ack-me", Type: messaging.TypeStatusUpdate, From: "a", To: "worker-ack",
		Subject: "ack test", Body: json.RawMessage(`{}`), CreatedAt: time.Now(),
	}
	if err := b.Emit(ctx, env); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	waitForTail(ctx, t, b, "worker-ack", func(envs []messaging.Envelope) bool {
		return containsID(envs, "ack-me")
	}, "envelope to appear in pending")

	if err := b.Ack(ctx, messaging.Ack{MessageID: "ack-me", ByWorker: "worker-ack", Action: "handled", At: time.Now()}); err != nil {
		t.Fatalf("Ack: %v", err)
	}

	waitForTail(ctx, t, b, "worker-ack", func(envs []messaging.Envelope) bool {
		return !containsID(envs, "ack-me")
	}, "envelope to be removed from pending after Ack")
}

func TestSubscribeOnlyDeliversAfterStartAndNoDuplicates(t *testing.T) {
	b := newTestBackend(t)
	defer b.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	pre := messaging.Envelope{
		ID: "pre-1", Type: messaging.TypeStatusUpdate, From: "a", To: "worker-sub",
		Subject: "pre", Body: json.RawMessage(`{}`), CreatedAt: time.Now(),
	}
	if err := b.Emit(ctx, pre); err != nil {
		t.Fatalf("Emit pre: %v", err)
	}

	subCtx, subCancel := context.WithCancel(ctx)
	defer subCancel()
	ch, err := b.Subscribe(subCtx, "worker-sub")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	post := messaging.Envelope{
		ID: "post-1", Type: messaging.TypeStatusUpdate, From: "a", To: "worker-sub",
		Subject: "post", Body: json.RawMessage(`{}`), CreatedAt: time.Now(),
	}
	if err := b.Emit(ctx, post); err != nil {
		t.Fatalf("Emit post: %v", err)
	}

	seen := map[string]int{}
	timeout := time.After(15 * time.Second)
	wantIDs := map[string]bool{"pre-1": true, "post-1": true}
collect:
	for {
		select {
		case env, ok := <-ch:
			if !ok {
				break collect
			}
			seen[env.ID]++
			delete(wantIDs, env.ID)
			if len(wantIDs) == 0 {
				break collect
			}
		case <-timeout:
			break collect
		}
	}

	if len(wantIDs) != 0 {
		t.Fatalf("Subscribe did not deliver expected envelopes, still missing: %v (saw: %v)", wantIDs, seen)
	}
	for id, count := range seen {
		if count > 1 {
			t.Fatalf("Subscribe delivered envelope %q %d times within one subscription lifetime, want at most once", id, count)
		}
	}
}

func TestCloseDoesNotLeakGoroutines(t *testing.T) {
	before := runtime.NumGoroutine()

	b := newTestBackend(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	subCtx, subCancel := context.WithCancel(ctx)
	ch, err := b.Subscribe(subCtx, "worker-leak-check")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	subCancel()
	drainDeadline := time.After(5 * time.Second)
drain:
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				break drain
			}
		case <-drainDeadline:
			t.Fatal("timed out waiting for Subscribe channel to close after context cancellation")
		}
	}

	if err := b.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	var after int
	for time.Now().Before(deadline) {
		runtime.GC()
		after = runtime.NumGoroutine()
		if after <= before+goroutineSlackAfterClose {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("goroutine count did not settle after Close: before=%d after=%d (threshold before+%d=%d)", before, after, goroutineSlackAfterClose, before+goroutineSlackAfterClose)
}

func waitForTail(ctx context.Context, t *testing.T, b *Backend, workerID string, done func([]messaging.Envelope) bool, what string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		envs, err := b.Tail(ctx, messaging.TailFilter{WorkerID: workerID})
		if err != nil {
			t.Fatalf("Tail: %v", err)
		}
		if done(envs) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func containsID(envs []messaging.Envelope, id string) bool {
	for _, e := range envs {
		if e.ID == id {
			return true
		}
	}
	return false
}
