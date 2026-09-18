// Package testsuite is the conformance suite every messaging.MessagingBackend implementation runs.
package testsuite

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/acumen-ai-org/robotdreams/pkg/messaging"
)

// RunConformance runs the conformance subtests, each against a fresh backend from factory.
func RunConformance(t *testing.T, factory func() messaging.MessagingBackend) {
	t.Helper()

	t.Run("emit then subscribe roundtrip", func(t *testing.T) {
		backend := factory()
		defer backend.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		ch, err := backend.Subscribe(ctx, "worker-x")
		if err != nil {
			t.Fatalf("Subscribe: %v", err)
		}

		env := messaging.Envelope{
			ID:        "msg-1",
			Type:      messaging.TypeStatusUpdate,
			From:      "worker-a",
			To:        "worker-x",
			Subject:   "hello",
			Body:      json.RawMessage(`{"ok":true}`),
			CreatedAt: time.Now(),
		}
		if err := backend.Emit(ctx, env); err != nil {
			t.Fatalf("Emit: %v", err)
		}

		select {
		case got := <-ch:
			if got.ID != env.ID {
				t.Fatalf("got envelope ID %q, want %q", got.ID, env.ID)
			}
			if got.To != "worker-x" {
				t.Fatalf("got envelope To %q, want %q", got.To, "worker-x")
			}
		case <-ctx.Done():
			t.Fatal("timed out waiting for envelope on subscribe channel")
		}
	})

	t.Run("subscriber only sees its own messages", func(t *testing.T) {
		backend := factory()
		defer backend.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		env := messaging.Envelope{
			ID:        "msg-2",
			Type:      messaging.TypeStatusUpdate,
			From:      "worker-a",
			To:        "worker-y",
			Subject:   "for y only",
			Body:      json.RawMessage(`{}`),
			CreatedAt: time.Now(),
		}
		if err := backend.Emit(ctx, env); err != nil {
			t.Fatalf("Emit: %v", err)
		}

		ch, err := backend.Subscribe(ctx, "worker-x")
		if err != nil {
			t.Fatalf("Subscribe: %v", err)
		}

		waitCtx, waitCancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer waitCancel()

		select {
		case got, ok := <-ch:
			if ok {
				t.Fatalf("worker-x unexpectedly received envelope addressed to worker-y: %+v", got)
			}
		case <-waitCtx.Done():
		}
	})

	t.Run("ack does not error for a valid message", func(t *testing.T) {
		backend := factory()
		defer backend.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		env := messaging.Envelope{
			ID:        "msg-3",
			Type:      messaging.TypeStatusUpdate,
			From:      "worker-a",
			To:        "worker-x",
			Subject:   "ack me",
			Body:      json.RawMessage(`{}`),
			CreatedAt: time.Now(),
		}
		if err := backend.Emit(ctx, env); err != nil {
			t.Fatalf("Emit: %v", err)
		}

		ack := messaging.Ack{
			MessageID: env.ID,
			ByWorker:  "worker-x",
			Action:    "read",
			At:        time.Now(),
		}
		if err := backend.Ack(ctx, ack); err != nil {
			t.Fatalf("Ack: %v", err)
		}
	})

	t.Run("ack is bound to the recipient", func(t *testing.T) {
		backend := factory()
		defer backend.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		env := messaging.Envelope{
			ID:        "msg-4",
			Type:      messaging.TypeStatusUpdate,
			From:      "worker-a",
			To:        "worker-x",
			Subject:   "only x may ack",
			Body:      json.RawMessage(`{}`),
			CreatedAt: time.Now(),
		}
		if err := backend.Emit(ctx, env); err != nil {
			t.Fatalf("Emit: %v", err)
		}

		for _, by := range []string{"worker-a", "worker-z"} {
			err := backend.Ack(ctx, messaging.Ack{
				MessageID: env.ID,
				ByWorker:  by,
				Action:    "read",
				At:        time.Now(),
			})
			if !errors.Is(err, messaging.ErrNotFound) {
				t.Fatalf("Ack by %q of a message addressed to worker-x: err = %v, want ErrNotFound", by, err)
			}
		}

		err := backend.Ack(ctx, messaging.Ack{
			MessageID: env.ID,
			ByWorker:  "worker-x",
			Action:    "handled",
			At:        time.Now(),
		})
		if err != nil {
			t.Fatalf("Ack by recipient after rejected acks: %v", err)
		}
	})

	t.Run("subscribe respects context cancellation", func(t *testing.T) {
		backend := factory()
		defer backend.Close()

		subCtx, subCancel := context.WithCancel(context.Background())
		ch, err := backend.Subscribe(subCtx, "worker-x")
		if err != nil {
			subCancel()
			t.Fatalf("Subscribe: %v", err)
		}

		subCancel()

		timeout := time.NewTimer(5 * time.Second)
		defer timeout.Stop()

		for {
			select {
			case _, ok := <-ch:
				if !ok {
					return
				}
			case <-timeout.C:
				t.Fatal("timed out waiting for subscribe channel to close after context cancellation")
			}
		}
	})

	t.Run("close ends live subscriptions", func(t *testing.T) {
		backend := factory()

		ch, err := backend.Subscribe(context.Background(), "worker-x")
		if err != nil {
			backend.Close()
			t.Fatalf("Subscribe: %v", err)
		}

		if err := backend.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}

		timeout := time.NewTimer(5 * time.Second)
		defer timeout.Stop()

		for {
			select {
			case _, ok := <-ch:
				if !ok {
					return
				}
			case <-timeout.C:
				t.Fatal("timed out waiting for subscribe channel to close after Close")
			}
		}
	})

	t.Run("tail returns messages up to limit", func(t *testing.T) {
		backend := factory()
		defer backend.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		const total = 5
		for i := 0; i < total; i++ {
			env := messaging.Envelope{
				ID:        "tail-msg-" + string(rune('a'+i)),
				Type:      messaging.TypeStatusUpdate,
				From:      "worker-a",
				To:        "worker-tail",
				Subject:   "tail",
				Body:      json.RawMessage(`{}`),
				CreatedAt: time.Now(),
			}
			if err := backend.Emit(ctx, env); err != nil {
				t.Fatalf("Emit %d: %v", i, err)
			}
		}

		const limit = 3
		got, err := backend.Tail(ctx, messaging.TailFilter{WorkerID: "worker-tail", Limit: limit})
		if err != nil {
			t.Fatalf("Tail: %v", err)
		}
		if len(got) > limit {
			t.Fatalf("Tail returned %d envelopes, want at most %d", len(got), limit)
		}
		if len(got) == 0 {
			t.Fatal("Tail returned no envelopes, want at least one")
		}
		for _, env := range got {
			if env.To != "worker-tail" {
				t.Fatalf("Tail returned envelope for %q, want %q", env.To, "worker-tail")
			}
		}
	})

	t.Run("health succeeds", func(t *testing.T) {
		backend := factory()
		defer backend.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := backend.Health(ctx); err != nil {
			t.Fatalf("Health: %v", err)
		}
	})
}
