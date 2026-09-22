package api

import (
	"bufio"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestEventHubBroadcastAndUnsubscribe(t *testing.T) {
	polled := make(chan struct{}, 8)
	hub := newEventHub(func(ctx context.Context) []sseEvent {
		select {
		case polled <- struct{}{}:
		default:
		}
		return []sseEvent{{Type: "ping", Data: map[string]string{"ok": "1"}}}
	})

	if got := hub.subscriberCount(); got != 0 {
		t.Fatalf("subscriberCount before subscribe = %d, want 0", got)
	}

	ch, unsubscribe := hub.subscribe()
	if got := hub.subscriberCount(); got != 1 {
		t.Fatalf("subscriberCount after subscribe = %d, want 1", got)
	}

	select {
	case <-polled:
	case <-time.After(pollInterval + time.Second):
		t.Fatal("poll loop did not run in time after first subscriber")
	}

	select {
	case ev := <-ch:
		if ev.Type != "ping" {
			t.Fatalf("event type = %q, want ping", ev.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no event delivered to subscriber")
	}

	unsubscribe()
	if got := hub.subscriberCount(); got != 0 {
		t.Fatalf("subscriberCount after unsubscribe = %d, want 0", got)
	}

	time.Sleep(50 * time.Millisecond)
	for len(polled) > 0 {
		<-polled
	}
	select {
	case <-polled:
		t.Fatal("poll loop kept running after the last subscriber unsubscribed (goroutine leak)")
	case <-time.After(pollInterval + 500*time.Millisecond):
	}
}

func TestEventHubDropsForSlowSubscriber(t *testing.T) {
	hub := newEventHub(func(ctx context.Context) []sseEvent { return nil })
	slow, unsubSlow := hub.subscribe()
	defer unsubSlow()
	_ = slow

	fast, unsubFast := hub.subscribe()
	defer unsubFast()

	for i := 0; i < 200; i++ {
		hub.broadcast(sseEvent{Type: "x", Data: i})
	}

	select {
	case <-fast:
	default:
		t.Fatal("fast subscriber received nothing despite 200 broadcasts")
	}
}

func TestEventsEndpointRequiresAdmin(t *testing.T) {
	e := newTestEnv(t, nil)
	w := e.connect("worker-1", "contributor", "")

	status, _ := e.do(http.MethodGet, "/api/events", "", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("no token: status %d, want 401", status)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.ts.URL+"/api/events", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+w.Token)
	resp, err := e.ts.Client().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("worker token: status %d, want 403", resp.StatusCode)
	}

	admin := e.adminToken("dashboard")
	ctx2, cancel2 := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel2()
	req2, err := http.NewRequestWithContext(ctx2, http.MethodGet, e.ts.URL+"/api/events", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req2.Header.Set("Authorization", "Bearer "+admin)
	resp2, err := e.ts.Client().Do(req2)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("admin token: status %d, want 200", resp2.StatusCode)
	}
	if ct := resp2.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}
}

func TestEventsEndpointStreamsWorkerConnect(t *testing.T) {
	e := newTestEnv(t, nil)
	admin := e.adminToken("dashboard")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.ts.URL+"/api/events", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+admin)
	resp, err := e.ts.Client().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}

	time.Sleep(pollInterval + 500*time.Millisecond)
	e.connect("worker-live", "contributor", "")

	scanner := bufio.NewScanner(resp.Body)
	var gotType, gotData string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			gotType = strings.TrimPrefix(line, "event: ")
			continue
		}
		if strings.HasPrefix(line, "data: ") {
			gotData = strings.TrimPrefix(line, "data: ")
			if gotType == "worker_connected" {
				break
			}
			gotType, gotData = "", ""
		}
	}
	if gotType != "worker_connected" {
		t.Fatalf("did not see worker_connected event (scanner err: %v)", scanner.Err())
	}
	if !strings.Contains(gotData, "worker-live") {
		t.Fatalf("worker_connected payload = %s, want it to mention worker-live", gotData)
	}
}

func TestPollEmitsWorkerRoleChanged(t *testing.T) {
	e := newTestEnv(t, nil)
	lead := e.connect("lead", "lead", "")
	e.connect("leaf", "contributor", "lead")

	state := newServerPollState(&serverDeps{Graph: e.srv.Graph})
	if got := state.pollWorkersLocked(); len(got) != 0 {
		t.Fatalf("first poll emitted %d events, want 0", len(got))
	}

	role := "architect"
	if status, raw := e.do(http.MethodPatch, "/api/workers/leaf", lead.Token, editWorkerRequest{Role: &role}); status != http.StatusOK {
		t.Fatalf("edit: status %d, body %s", status, raw)
	}

	var got sseEvent
	for _, ev := range state.pollWorkersLocked() {
		if ev.Type == "worker_role_changed" {
			got = ev
		}
	}
	if got.Type != "worker_role_changed" {
		t.Fatal("poll did not emit worker_role_changed after a role change")
	}
	data, ok := got.Data.(map[string]any)
	if !ok {
		t.Fatalf("event data = %T, want a map", got.Data)
	}
	if data["worker_id"] != "leaf" || data["old_role"] != "contributor" || data["new_role"] != "architect" {
		t.Fatalf("unexpected worker_role_changed payload: %v", data)
	}

	if evs := state.pollWorkersLocked(); len(evs) != 0 {
		t.Fatalf("a settled role change kept emitting %d events", len(evs))
	}
}
