package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/acumen-ai-org/robotdreams/internal/server"
)

func (e *testEnv) mintScoped(subject string, scopes ...string) string {
	e.t.Helper()
	tok, err := e.srv.Issuer().Mint(subject, scopes, time.Hour)
	if err != nil {
		e.t.Fatalf("Mint: %v", err)
	}
	return tok.Raw
}

func TestEmitRequiresMessageSendScope(t *testing.T) {
	e := newTestEnv(t, nil)
	e.connect("lead", "lead", "")
	leaf := e.connect("leaf", "contributor", "lead")
	body := emitRequest{Type: "status_update", Subject: "hello"}

	d := e.delegate(leaf.Token, map[string]any{"scopes": []string{"message:receive"}}, http.StatusCreated)
	status, raw := e.do(http.MethodPost, "/api/messages", d.Token, body)
	if status != http.StatusForbidden {
		t.Fatalf("emit without message:send: status %d, want 403: %s", status, raw)
	}

	status, raw = e.do(http.MethodPost, "/api/messages", e.mintScoped("leaf", "message:receive"), body)
	if status != http.StatusForbidden {
		t.Fatalf("emit without message:send: status %d, want 403: %s", status, raw)
	}

	status, raw = e.do(http.MethodPost, "/api/messages", leaf.Token, body)
	if status != http.StatusAccepted {
		t.Fatalf("emit with message:send: status %d, want 202: %s", status, raw)
	}

	status, raw = e.do(http.MethodPost, "/api/messages", e.mintScoped("leaf", server.AdminScope), body)
	if status != http.StatusAccepted {
		t.Fatalf("emit as admin without message:send: status %d, want 202: %s", status, raw)
	}
}

func TestSubscribeAndTailRequireMessageReceiveScope(t *testing.T) {
	e := newTestEnv(t, nil)
	e.connect("lead", "lead", "")
	leaf := e.connect("leaf", "contributor", "lead")

	d := e.delegate(leaf.Token, map[string]any{"scopes": []string{"message:send"}}, http.StatusCreated)
	for _, path := range []string{
		"/api/messages/subscribe",
		"/api/messages",
	} {
		status, raw := e.do(http.MethodGet, path, d.Token, nil)
		if status != http.StatusForbidden {
			t.Fatalf("GET %s without message:receive: status %d, want 403: %s", path, status, raw)
		}
		status, raw = e.do(http.MethodGet, path, e.mintScoped("leaf", "message:send"), nil)
		if status != http.StatusForbidden {
			t.Fatalf("GET %s without message:receive: status %d, want 403: %s", path, status, raw)
		}
	}

	if status, raw := e.do(http.MethodGet, "/api/messages", leaf.Token, nil); status != http.StatusOK {
		t.Fatalf("tail with message:receive: status %d, want 200: %s", status, raw)
	}
	admin := e.mintScoped("ops", server.AdminScope)
	if status, raw := e.do(http.MethodGet, "/api/messages?worker_id=lead", admin, nil); status != http.StatusOK {
		t.Fatalf("tail as admin without message:receive: status %d, want 200: %s", status, raw)
	}

	e.assertSubscribeOpens(admin, "lead")
}

func (e *testEnv) assertSubscribeOpens(token, workerID string) {
	e.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		e.ts.URL+"/api/messages/subscribe?worker_id="+workerID, nil)
	if err != nil {
		e.t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := e.ts.Client().Do(req)
	if err != nil {
		e.t.Fatalf("subscribe: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		e.t.Fatalf("subscribe: status %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		e.t.Fatalf("subscribe: content-type %q, want text/event-stream", ct)
	}
}
