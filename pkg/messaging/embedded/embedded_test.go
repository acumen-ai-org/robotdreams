package embedded

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/acumen-ai-org/robotdreams/pkg/messaging"
	"github.com/acumen-ai-org/robotdreams/pkg/messaging/testsuite"
)

// TestConformance runs the shared messaging.MessagingBackend conformance
// suite against a fresh SQLite-backed Backend per subtest.
func TestConformance(t *testing.T) {
	testsuite.RunConformance(t, func() messaging.MessagingBackend {
		dbPath := filepath.Join(t.TempDir(), "test.db")
		backend, err := New(dbPath)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		return backend
	})
}

// TestPersistenceAcrossReopen verifies that messages emitted before Close
// are still visible after reopening a Backend against the same database
// file path.
func TestPersistenceAcrossReopen(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "persist.db")

	backend, err := New(dbPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	env := messaging.Envelope{
		ID:        "persist-1",
		Type:      messaging.TypeStatusUpdate,
		From:      "worker-a",
		To:        "worker-x",
		Subject:   "survive a restart",
		Body:      json.RawMessage(`{"n":1}`),
		CreatedAt: time.Now(),
	}
	if err := backend.Emit(ctx, env); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if err := backend.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := New(dbPath)
	if err != nil {
		t.Fatalf("New (reopen): %v", err)
	}
	defer reopened.Close()

	got, err := reopened.Tail(ctx, messaging.TailFilter{WorkerID: "worker-x"})
	if err != nil {
		t.Fatalf("Tail after reopen: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Tail after reopen returned %d envelopes, want 1", len(got))
	}
	if got[0].ID != env.ID {
		t.Fatalf("Tail after reopen returned ID %q, want %q", got[0].ID, env.ID)
	}
	if got[0].Subject != env.Subject {
		t.Fatalf("Tail after reopen returned Subject %q, want %q", got[0].Subject, env.Subject)
	}
}

// TestAckedMessagesAreNotRedeliveredToANewSubscription pins the property a
// long-running listener depends on.
//
// Subscribe starts every subscription with a zero cursor and re-reads the
// backlog, so what keeps a reconnecting consumer from being handed work it
// has already done is the acked_at filter — nothing else. A node that runs
// an agent per message (see docs/templates.md) would otherwise re-run every
// historical message on each restart, and restarts are routine: the
// follower reconnects a second after any dropped stream.
//
// This lives here rather than in the shared conformance suite on purpose.
// It is real behaviour of this backend, but whether every
// messaging.MessagingBackend must guarantee it is a contract question the
// interface does not currently answer — the webhook backend pushes by HTTP
// POST rather than serving a pull subscription, so the same words may not
// mean the same thing there.
func TestAckedMessagesAreNotRedeliveredToANewSubscription(t *testing.T) {
	backend, err := New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer backend.Close()

	ctx := context.Background()
	for _, id := range []string{"m1", "m2"} {
		err := backend.Emit(ctx, messaging.Envelope{
			ID: id, Type: messaging.TypeStatusUpdate,
			From: "worker-a", To: "worker-x", Subject: id,
			Body: json.RawMessage(`{}`), CreatedAt: time.Now(),
		})
		if err != nil {
			t.Fatalf("Emit %s: %v", id, err)
		}
	}

	// A listener handles m1 and acks it, then dies.
	if err := backend.Ack(ctx, messaging.Ack{
		MessageID: "m1", ByWorker: "worker-x", Action: "handled", At: time.Now(),
	}); err != nil {
		t.Fatalf("Ack: %v", err)
	}

	// It restarts: a brand-new subscription, cursor back at zero.
	subCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ch, err := backend.Subscribe(subCtx, "worker-x")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	var got []string
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
collect:
	for {
		select {
		case env, ok := <-ch:
			if !ok {
				break collect
			}
			got = append(got, env.ID)
			if len(got) > 1 {
				break collect
			}
		case <-deadline.C:
			break collect
		}
	}

	for _, id := range got {
		if id == "m1" {
			t.Fatal("an acked message was redelivered to a new subscription; " +
				"a restarted listener would re-run work it had already done")
		}
	}
	if len(got) != 1 || got[0] != "m2" {
		t.Fatalf("redelivered %v, want exactly the still-unacked m2", got)
	}
}
