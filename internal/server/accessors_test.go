package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/acumen-ai-org/robotdreams/pkg/security"
	"github.com/acumen-ai-org/robotdreams/pkg/storage"
)

func TestAccessorsReflectConfigDefaults(t *testing.T) {
	s := newTestServer(t, nil)
	if s.TokenTTL() != DefaultTokenTTL {
		t.Fatalf("TokenTTL = %s, want %s", s.TokenTTL(), DefaultTokenTTL)
	}
	if s.DelegatedTokenTTL() != DefaultDelegatedTokenTTL {
		t.Fatalf("DelegatedTokenTTL = %s, want %s", s.DelegatedTokenTTL(), DefaultDelegatedTokenTTL)
	}
	if s.OpenEnrollment() {
		t.Fatal("OpenEnrollment defaults to true")
	}
	if s.ReportsDir() != "" {
		t.Fatalf("ReportsDir = %q, want empty", s.ReportsDir())
	}
	if _, ok := s.Clock().(security.RealClock); !ok {
		t.Fatalf("Clock = %T, want RealClock by default", s.Clock())
	}
	if s.Reports() == nil || len(s.Reports().List()) != 0 {
		t.Fatalf("Reports() = %v, want an empty, non-nil registry", s.Reports())
	}
	if s.Storage() == nil || s.Schedules() == nil || s.Revocations() == nil || s.Store() == nil || s.Issuer() == nil {
		t.Fatal("a core accessor returned nil")
	}

	ctx := context.Background()
	if revoked, err := s.Revocations().IsRevoked(ctx, "nobody"); err != nil || revoked {
		t.Fatalf("IsRevoked(nobody) = %v, %v", revoked, err)
	}

	if _, err := s.Storage().Put(ctx, "workers/x/a.txt", strings.NewReader("hi"), storage.PutOptions{}); err != nil {
		t.Fatalf("Storage().Put: %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.cfg.DataDir, "storage")); err != nil {
		t.Fatalf("default storage dir missing: %v", err)
	}
}

func TestAccessorsReflectConfigOverrides(t *testing.T) {
	clock := newFakeClock(time.Date(2026, 9, 13, 9, 0, 0, 0, time.UTC))
	s := newTestServer(t, func(c *Config) {
		c.Clock = clock
		c.TokenTTL = 42 * time.Second
		c.DelegatedTokenTTL = 7 * time.Second
		c.OpenEnrollment = true
	})
	if s.Clock() != clock {
		t.Fatal("Clock() is not the injected clock")
	}
	if s.TokenTTL() != 42*time.Second || s.DelegatedTokenTTL() != 7*time.Second {
		t.Fatalf("TTLs = %s / %s", s.TokenTTL(), s.DelegatedTokenTTL())
	}
	if !s.OpenEnrollment() {
		t.Fatal("OpenEnrollment override lost")
	}

	tok, err := s.MintWorkerToken("w")
	if err != nil {
		t.Fatalf("MintWorkerToken: %v", err)
	}
	if tok.TTL() != 42*time.Second || !tok.IssuedAt.Equal(clock.Now()) {
		t.Fatalf("token = %+v", tok)
	}
}

func TestLoadReportLibrary(t *testing.T) {
	lib, err := filepath.Abs(filepath.Join("..", "..", "reporting", "library"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(lib); err != nil {
		t.Skipf("report library not present: %v", err)
	}
	s := newTestServer(t, func(c *Config) { c.ReportsDir = lib })
	if s.ReportsDir() != lib {
		t.Fatalf("ReportsDir = %q, want %q", s.ReportsDir(), lib)
	}
	defs := s.Reports().List()
	if len(defs) == 0 {
		t.Fatal("no report definitions loaded")
	}
	if _, ok := s.Reports().Get("activity"); !ok {
		t.Fatal("the activity definition is missing from the registry")
	}

	empty := newTestServer(t, func(c *Config) { c.ReportsDir = t.TempDir() })
	if got := empty.Reports().List(); len(got) != 0 {
		t.Fatalf("empty dir loaded %d definitions", len(got))
	}
}

func TestLoadReportLibraryFailsStartup(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T) string
	}{
		{"missing directory", func(t *testing.T) string { return filepath.Join(t.TempDir(), "nope") }},
		{"invalid definition", func(t *testing.T) string {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte("version: v1alpha1\nkind: ReportDefinition\nname: [not a string]\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			return dir
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := tc.setup(t)
			s, err := New(Config{DataDir: t.TempDir(), ReportsDir: dir})
			if err == nil {
				s.Close()
				t.Fatal("New succeeded with a broken report library")
			}
			if !strings.Contains(err.Error(), "load report library") {
				t.Fatalf("err = %v, want it to name the report library", err)
			}
		})
	}
}
