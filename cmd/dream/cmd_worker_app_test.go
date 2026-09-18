package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/acumen-ai-org/robotdreams/internal/identity"
	"github.com/acumen-ai-org/robotdreams/internal/server"
)

func TestWorkerAppSetListClear(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	addr, srv := startTestServer(t)
	ctx := context.Background()
	connectPair(t, addr)

	out := mustRunDreamCmd(t, "worker", "app", "list", "--server", addr, "--worker-id", "lead")
	if strings.TrimSpace(out) != "no apps declared" {
		t.Fatalf("empty list = %q", out)
	}

	out = mustRunDreamCmd(t, "worker", "app", "set", "--server", addr, "--worker-id", "leaf",
		"--url", "http://127.0.0.1:9000/", "--description", "a counter page")
	if !strings.Contains(out, "leaf serves http://127.0.0.1:9000/") || !strings.Contains(out, "  a counter page") {
		t.Fatalf("set output = %q", out)
	}

	app, err := srv.WorkerApp(ctx, "leaf")
	if err != nil {
		t.Fatalf("server WorkerApp: %v", err)
	}
	if app.URL != "http://127.0.0.1:9000/" || app.Description != "a counter page" {
		t.Fatalf("server app = %+v", app)
	}

	out = mustRunDreamCmd(t, "worker", "app", "list", "--server", addr, "--worker-id", "lead")
	if !strings.Contains(out, "NODE") || !strings.Contains(out, "URL") || !strings.Contains(out, "DESCRIPTION") {
		t.Fatalf("list header missing:\n%s", out)
	}
	if !strings.Contains(out, "leaf") || !strings.Contains(out, "http://127.0.0.1:9000/") || !strings.Contains(out, "a counter page") {
		t.Fatalf("list row missing:\n%s", out)
	}

	out = mustRunDreamCmd(t, "worker", "app", "set", "--server", addr, "--worker-id", "leaf", "--url", "https://example.test/x")
	if strings.Contains(out, "  a counter page") {
		t.Fatalf("stale description printed after replace: %q", out)
	}
	apps, err := runWorkerAppList(ctx, workerAppOptions{Server: addr, AsWorker: "lead"})
	if err != nil {
		t.Fatalf("app list: %v", err)
	}
	if len(apps) != 1 || apps[0].WorkerID != "leaf" || apps[0].URL != "https://example.test/x" || apps[0].Description != "" {
		t.Fatalf("apps after replace = %+v", apps)
	}
	out = mustRunDreamCmd(t, "worker", "app", "list", "--server", addr, "--worker-id", "lead")
	if !strings.Contains(out, "https://example.test/x  -") {
		t.Fatalf("list should render an empty description as a dash:\n%s", out)
	}

	out = mustRunDreamCmd(t, "worker", "app", "clear", "--server", addr, "--worker-id", "leaf")
	if strings.TrimSpace(out) != "cleared" {
		t.Fatalf("clear output = %q", out)
	}
	apps, err = runWorkerAppList(ctx, workerAppOptions{Server: addr, AsWorker: "lead"})
	if err != nil {
		t.Fatalf("app list after clear: %v", err)
	}
	if len(apps) != 0 {
		t.Fatalf("apps after clear = %+v, want none", apps)
	}
}

func TestWorkerAppSetRejectsBadURLBeforeDialing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, u := range []string{"", "ftp://example.test/", "not a url", "javascript:alert(1)"} {
		_, err := runWorkerAppSet(context.Background(), workerAppOptions{Server: "127.0.0.1:1", AsWorker: "w", URL: u})
		if err == nil {
			t.Errorf("url %q accepted, want an error", u)
			continue
		}
		var apiErr *apiError
		if errors.As(err, &apiErr) {
			t.Errorf("url %q reached the server: %v", u, err)
		}
	}
	if _, err := runDreamCmd(t, "worker", "app", "set", "--server", "127.0.0.1:1"); err == nil {
		t.Fatal("`worker app set` without --url should fail")
	}
}

func TestWorkerConnectDeclaresApp(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	addr, srv := startTestServer(t)
	ctx := context.Background()

	if _, err := runWorkerConnect(ctx, workerConnectOptions{
		Server: addr, WorkerID: "with-app", Role: "worker",
		AppURL: "http://127.0.0.1:8080/ui", AppDescription: "the ui",
	}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	app, err := srv.WorkerApp(ctx, "with-app")
	if err != nil {
		t.Fatalf("server WorkerApp: %v", err)
	}
	if app.URL != "http://127.0.0.1:8080/ui" || app.Description != "the ui" {
		t.Fatalf("declared app = %+v", app)
	}

	if _, err := runWorkerConnect(ctx, workerConnectOptions{
		Server: addr, WorkerID: "bad-app", Role: "worker", AppURL: "gopher://nope",
	}); err != nil {
		t.Fatalf("connect with a bad app url must still succeed: %v", err)
	}
	apps, err := srv.WorkerApps(ctx)
	if err != nil {
		t.Fatalf("WorkerApps: %v", err)
	}
	for _, a := range apps {
		if a.WorkerID == "bad-app" {
			t.Fatalf("an invalid app url was declared: %+v", a)
		}
	}
	if len(apps) != 1 {
		t.Fatalf("apps = %+v, want only with-app", apps)
	}

	kp, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	declareApp(ctx, "http://127.0.0.1:1", "nobody", kp, "", "")
}

func TestWorkerDelegateMintsNarrowedChild(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	addr, _ := startTestServer(t)
	ctx := context.Background()
	connectPair(t, addr)

	d, err := runWorkerDelegate(ctx, workerDelegateOptions{Server: addr, AsWorker: "lead", Child: "fetch-1"})
	if err != nil {
		t.Fatalf("delegate: %v", err)
	}
	if d.WorkerID != server.DelegatedWorkerID("lead", "fetch-1") || d.DelegatedBy != "lead" {
		t.Fatalf("child = %+v", d)
	}
	if d.Token == "" || d.ExpiresAt.IsZero() {
		t.Fatalf("no credential minted: %+v", d)
	}
	for _, s := range d.Scopes {
		if s == server.AdminScope {
			t.Fatalf("child holds admin: %v", d.Scopes)
		}
	}
	for _, want := range server.DefaultScopes {
		found := false
		for _, s := range d.Scopes {
			if s == want {
				found = true
			}
		}
		if !found {
			t.Errorf("child scopes %v lack the parent's %q", d.Scopes, want)
		}
	}

	child := &apiClient{baseURL: baseURLFromAddr(addr), token: d.Token, hc: defaultHTTPClient()}
	var listed struct {
		Schedules []scheduleView `json:"schedules"`
	}
	if err := child.doJSON(ctx, http.MethodGet, "/api/schedules", nil, &listed); err != nil {
		t.Fatalf("child token rejected on a read: %v", err)
	}
	var apiErr *apiError
	err = child.doJSON(ctx, http.MethodPost, "/api/workers/delegate", map[string]any{}, nil)
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusForbidden {
		t.Fatalf("child delegating further: got %v, want 403", err)
	}
	err = child.doJSON(ctx, http.MethodPost, "/api/schedules", map[string]any{"cron": "* * * * *"}, nil)
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusForbidden {
		t.Fatalf("child registering a schedule: got %v, want 403", err)
	}

	d2, err := runWorkerDelegate(ctx, workerDelegateOptions{
		Server: addr, AsWorker: "lead", Child: "reader", Scopes: []string{"message:receive"}, TTL: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("delegate subset: %v", err)
	}
	if len(d2.Scopes) != 1 || d2.Scopes[0] != "message:receive" {
		t.Fatalf("subset scopes = %v", d2.Scopes)
	}
	if until := time.Until(d2.ExpiresAt); until > 31*time.Second || until < 0 {
		t.Fatalf("ttl not honored: expires in %s", until)
	}

	_, err = runWorkerDelegate(ctx, workerDelegateOptions{Server: addr, AsWorker: "lead", Scopes: []string{server.AdminScope}})
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusBadRequest || !strings.Contains(apiErr.Message, "admin") {
		t.Fatalf("delegating admin: got %v, want a 400 naming admin", err)
	}
	_, err = runWorkerDelegate(ctx, workerDelegateOptions{Server: addr, AsWorker: "lead", Scopes: []string{"storage:write:secret/*"}})
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("delegating an unheld scope: got %v, want 400", err)
	}
	_, err = runWorkerDelegate(ctx, workerDelegateOptions{Server: addr, AsWorker: "lead", Child: "bad name"})
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad child name: got %v, want 400", err)
	}
}

func TestWorkerDelegateCommandOutput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	addr, _ := startTestServer(t)
	connectPair(t, addr)

	root := newRootCmd()
	var stdout, stderr strings.Builder
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"worker", "delegate", "--server", addr, "--worker-id", "lead", "--child", "c1"})
	if err := root.Execute(); err != nil {
		t.Fatalf("delegate: %v\n%s", err, stderr.String())
	}
	token := strings.TrimSpace(stdout.String())
	if token == "" || strings.ContainsAny(token, " \n") {
		t.Fatalf("stdout should be exactly the token, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "delegated lead~c1 until ") || !strings.Contains(stderr.String(), "scopes: ") {
		t.Fatalf("stderr narration = %q", stderr.String())
	}

	out := mustRunDreamCmd(t, "worker", "delegate", "--server", addr, "--worker-id", "lead", "--child", "c2", "--json")
	var d delegateView
	if err := json.Unmarshal([]byte(out), &d); err != nil {
		t.Fatalf("--json output is not JSON: %v\n%s", err, out)
	}
	if d.WorkerID != "lead~c2" || d.Token == "" {
		t.Fatalf("--json credential = %+v", d)
	}
}
