package server

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestValidateAppURLRejectsClickableHazards(t *testing.T) {
	hazards := []string{
		"javascript:alert(1)",
		"JavaScript:alert(1)",
		"  javascript:alert(1)  ",
		"data:text/html,<script>alert(1)</script>",
		"vbscript:msgbox(1)",
		"file:///etc/passwd",
		"ftp://example.test/x",
		"chrome://settings",
		"//example.test/no-scheme",
		"example.test/no-scheme",
		"",
		"   ",
	}
	for _, raw := range hazards {
		t.Run(raw, func(t *testing.T) {
			got, err := ValidateAppURL(raw)
			if err == nil {
				t.Fatalf("ValidateAppURL(%q) = %q, want it refused", raw, got)
			}
			if !errors.Is(err, ErrInvalidApp) {
				t.Fatalf("error %v, want ErrInvalidApp", err)
			}
		})
	}
}

func TestValidateAppURLAcceptsRealAddresses(t *testing.T) {
	tests := []struct{ in, want string }{
		{"https://build.example.test", "https://build.example.test"},
		{"http://127.0.0.1:8080/dash", "http://127.0.0.1:8080/dash"},
		{"https://example.test/a/b?c=d#e", "https://example.test/a/b?c=d#e"},
		{"  https://example.test  ", "https://example.test"},

		{"HTTPS://example.test", "https://example.test"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, err := ValidateAppURL(tc.in)
			if err != nil {
				t.Fatalf("ValidateAppURL(%q): %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSetWorkerApp(t *testing.T) {
	s := newTestServer(t, nil)
	ctx := context.Background()
	connect(t, s, "w1", "")

	app, err := s.SetWorkerApp(ctx, "w1", "https://build.example.test", "The build dashboard")
	if err != nil {
		t.Fatalf("SetWorkerApp: %v", err)
	}
	if app.URL != "https://build.example.test" || app.Description != "The build dashboard" {
		t.Fatalf("app = %+v", app)
	}
	if app.DeclaredAt.IsZero() {
		t.Fatal("DeclaredAt was not stamped")
	}

	if _, err := s.SetWorkerApp(ctx, "w1", "https://new.example.test", ""); err != nil {
		t.Fatalf("second SetWorkerApp: %v", err)
	}
	got, err := s.WorkerApp(ctx, "w1")
	if err != nil {
		t.Fatalf("WorkerApp: %v", err)
	}
	if got.URL != "https://new.example.test" {
		t.Fatalf("url = %q, want the replacement", got.URL)
	}
	if got.Description != "" {
		t.Fatalf("description = %q, want the previous one replaced, not merged", got.Description)
	}
}

func TestSetWorkerAppRejectsUnknownWorker(t *testing.T) {
	s := newTestServer(t, nil)
	if _, err := s.SetWorkerApp(context.Background(), "ghost", "https://example.test", ""); err == nil {
		t.Fatal("declaring an app for an unregistered worker succeeded")
	}
}

func TestSetWorkerAppBoundsDescription(t *testing.T) {
	s := newTestServer(t, nil)
	ctx := context.Background()
	connect(t, s, "w1", "")

	_, err := s.SetWorkerApp(ctx, "w1", "https://example.test", strings.Repeat("x", maxAppDescription+1))
	if !errors.Is(err, ErrInvalidApp) {
		t.Fatalf("err = %v, want ErrInvalidApp for an over-long description", err)
	}
	if _, err := s.SetWorkerApp(ctx, "w1", "https://example.test", strings.Repeat("x", maxAppDescription)); err != nil {
		t.Fatalf("a description at the limit was refused: %v", err)
	}
}

func TestClearWorkerApp(t *testing.T) {
	s := newTestServer(t, nil)
	ctx := context.Background()
	connect(t, s, "w1", "")

	if _, err := s.SetWorkerApp(ctx, "w1", "https://example.test", ""); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := s.ClearWorkerApp(ctx, "w1"); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if _, err := s.WorkerApp(ctx, "w1"); err == nil {
		t.Fatal("the app survived being cleared")
	}

	if err := s.ClearWorkerApp(ctx, "w1"); err != nil {
		t.Fatalf("second clear: %v", err)
	}
}

func TestWorkerAppsListsFleet(t *testing.T) {
	s := newTestServer(t, nil)
	ctx := context.Background()
	connect(t, s, "w1", "")
	connect(t, s, "w2", "")
	connect(t, s, "w3", "")

	for _, id := range []string{"w1", "w3"} {
		if _, err := s.SetWorkerApp(ctx, id, "https://"+id+".example.test", "app for "+id); err != nil {
			t.Fatalf("set %s: %v", id, err)
		}
	}

	apps, err := s.WorkerApps(ctx)
	if err != nil {
		t.Fatalf("WorkerApps: %v", err)
	}
	if len(apps) != 2 {
		t.Fatalf("got %d apps, want 2 — a node without one contributes no row", len(apps))
	}
	if apps[0].WorkerID != "w1" || apps[1].WorkerID != "w3" {
		t.Fatalf("apps are not ordered by worker: %+v", apps)
	}
}
