package server

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/acumen-ai-org/robotdreams/internal/orgchart"
	"github.com/acumen-ai-org/robotdreams/pkg/messaging"
	"github.com/acumen-ai-org/robotdreams/pkg/security"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(start time.Time) *fakeClock { return &fakeClock{now: start} }

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

func newTestServer(t *testing.T, mutate func(*Config)) *Server {
	t.Helper()
	cfg := Config{DataDir: t.TempDir(), Clock: security.RealClock{}}
	if mutate != nil {
		mutate(&cfg)
	}
	s, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func connect(t *testing.T, s *Server, id, reportsTo string) {
	t.Helper()
	err := s.ConnectWorker(context.Background(), orgchart.Worker{
		ID:        id,
		Role:      "contributor",
		ReportsTo: reportsTo,
	})
	if err != nil {
		t.Fatalf("ConnectWorker(%q): %v", id, err)
	}
}

func TestNewUsesZeroConfigDefaults(t *testing.T) {
	dir := t.TempDir()
	s := newTestServer(t, func(c *Config) { c.DataDir = dir })

	msgURI, stgURI := s.BackendURIs()
	abs, _ := filepath.Abs(dir)
	if want := DefaultMessagingURI(abs); msgURI != want {
		t.Fatalf("messaging URI = %q, want %q", msgURI, want)
	}
	if want := DefaultStorageURI(abs); stgURI != want {
		t.Fatalf("storage URI = %q, want %q", stgURI, want)
	}
	if s.Addr() != DefaultAddr {
		t.Fatalf("Addr = %q, want %q", s.Addr(), DefaultAddr)
	}

	savedMsg, savedStg, err := s.Store().GetConfig(context.Background())
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if savedMsg != msgURI || savedStg != stgURI {
		t.Fatalf("config not persisted: %q / %q", savedMsg, savedStg)
	}

	info, err := os.Stat(filepath.Join(abs, ServerKeyFileName))
	if err != nil {
		t.Fatalf("stat server key: %v", err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Fatalf("server key mode = %04o, want no group/other bits", perm)
	}
}

func TestRestartPrefersPersistedConfig(t *testing.T) {
	dir := t.TempDir()
	customQueue := "queue://sqlite" + filepath.ToSlash(filepath.Join(dir, "custom-messages.db"))

	s1, err := New(Config{DataDir: dir, MessagingURI: customQueue})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s2, err := New(Config{DataDir: dir})
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	defer s2.Close()

	msgURI, _ := s2.BackendURIs()
	if msgURI != customQueue {
		t.Fatalf("restart reverted to %q, want the persisted %q", msgURI, customQueue)
	}
}

func TestRestartReloadsGraphInDependencyOrder(t *testing.T) {
	dir := t.TempDir()

	s1, err := New(Config{DataDir: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	connect(t, s1, "z-root", "")
	connect(t, s1, "m-lead", "z-root")
	connect(t, s1, "a-leaf", "m-lead")
	if err := s1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s2, err := New(Config{DataDir: dir})
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	defer s2.Close()

	if got := len(s2.Graph().List()); got != 3 {
		t.Fatalf("want 3 workers after restart, got %d", got)
	}
	target, err := s2.Graph().EscalationTarget("a-leaf")
	if err != nil {
		t.Fatalf("EscalationTarget: %v", err)
	}
	if target != "m-lead" {
		t.Fatalf("report-to edge lost across restart: got %q", target)
	}
}

func TestOrgChartFileLoadsOnlyOnFirstStartup(t *testing.T) {
	dir := t.TempDir()
	chartPath := filepath.Join(dir, "chart.yaml")

	chart := "workers:\n" +
		"  - id: worker-a\n    role: contributor\n    reports_to: lead-1\n" +
		"  - id: lead-1\n    role: lead\n    reports_to: \"\"\n"
	if err := os.WriteFile(chartPath, []byte(chart), 0o600); err != nil {
		t.Fatalf("write chart: %v", err)
	}

	s1, err := New(Config{DataDir: dir, OrgChartFile: chartPath})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := len(s1.Graph().List()); got != 2 {
		t.Fatalf("chart file not loaded: got %d workers", got)
	}

	if err := s1.ReassignWorker(context.Background(), "worker-a", "", "worker-a", false); err != nil {
		t.Fatalf("ReassignWorker: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s2, err := New(Config{DataDir: dir, OrgChartFile: chartPath})
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	defer s2.Close()

	target, err := s2.Graph().EscalationTarget("worker-a")
	if err != nil {
		t.Fatalf("EscalationTarget: %v", err)
	}
	if target != "" {
		t.Fatalf("chart file clobbered runtime reassignment: worker-a reports to %q", target)
	}
}

func TestEmitEscalationGoesExactlyOneHop(t *testing.T) {
	s := newTestServer(t, nil)
	ctx := context.Background()
	connect(t, s, "root", "")
	connect(t, s, "lead", "root")
	connect(t, s, "leaf", "lead")

	sent, err := s.EmitFromWorker(ctx, "leaf", messaging.Envelope{
		Type:    messaging.TypeEscalation,
		To:      "root",
		Subject: "stuck",
	})
	if err != nil {
		t.Fatalf("EmitFromWorker: %v", err)
	}
	if sent.To != "lead" {
		t.Fatalf("escalation routed to %q, want the immediate parent %q", sent.To, "lead")
	}
	if sent.From != "leaf" {
		t.Fatalf("From = %q, want the authenticated sender", sent.From)
	}
	if sent.ID == "" {
		t.Fatal("server did not assign a message ID")
	}

	leadMsgs, err := s.Messaging().Tail(ctx, messaging.TailFilter{WorkerID: "lead"})
	if err != nil {
		t.Fatalf("Tail(lead): %v", err)
	}
	if len(leadMsgs) != 1 {
		t.Fatalf("lead received %d messages, want 1", len(leadMsgs))
	}
	rootMsgs, err := s.Messaging().Tail(ctx, messaging.TailFilter{WorkerID: "root"})
	if err != nil {
		t.Fatalf("Tail(root): %v", err)
	}
	if len(rootMsgs) != 0 {
		t.Fatalf("root received %d messages, want 0 (no automatic forwarding)", len(rootMsgs))
	}
}

func TestEmitIgnoresClientSuppliedFrom(t *testing.T) {
	s := newTestServer(t, nil)
	connect(t, s, "lead", "")
	connect(t, s, "leaf", "lead")

	sent, err := s.EmitFromWorker(context.Background(), "leaf", messaging.Envelope{
		Type: messaging.TypeStatusUpdate,
		From: "lead",
	})
	if err != nil {
		t.Fatalf("EmitFromWorker: %v", err)
	}
	if sent.From != "leaf" {
		t.Fatalf("forged From survived: %q", sent.From)
	}
}

func TestEmitAtRootBehavior(t *testing.T) {
	s := newTestServer(t, nil)
	ctx := context.Background()
	connect(t, s, "top", "")

	t.Run("status update is dropped", func(t *testing.T) {
		sent, err := s.EmitFromWorker(ctx, "top", messaging.Envelope{Type: messaging.TypeStatusUpdate})
		if err != nil {
			t.Fatalf("want no error for a status update at root, got %v", err)
		}
		if sent.To != "" {
			t.Fatalf("want an empty To marking it undelivered, got %q", sent.To)
		}
	})

	t.Run("completed work is dropped", func(t *testing.T) {
		sent, err := s.EmitFromWorker(ctx, "top", messaging.Envelope{Type: messaging.TypeCompletedWork})
		if err != nil {
			t.Fatalf("want no error, got %v", err)
		}
		if sent.To != "" {
			t.Fatalf("want an empty To, got %q", sent.To)
		}
	})

	t.Run("escalation errors", func(t *testing.T) {
		_, err := s.EmitFromWorker(ctx, "top", messaging.Envelope{Type: messaging.TypeEscalation})
		if !errors.Is(err, ErrNoEscalationTarget) {
			t.Fatalf("want ErrNoEscalationTarget, got %v", err)
		}
	})

	t.Run("request for input errors", func(t *testing.T) {
		_, err := s.EmitFromWorker(ctx, "top", messaging.Envelope{Type: messaging.TypeRequestForInput})
		if !errors.Is(err, ErrNoEscalationTarget) {
			t.Fatalf("want ErrNoEscalationTarget, got %v", err)
		}
	})
}

func TestEmitCompletedWorkHonorsExplicitRecipient(t *testing.T) {
	s := newTestServer(t, nil)
	ctx := context.Background()
	connect(t, s, "lead", "")
	connect(t, s, "leaf", "lead")
	connect(t, s, "peer", "lead")

	sent, err := s.EmitFromWorker(ctx, "leaf", messaging.Envelope{
		Type:       messaging.TypeCompletedWork,
		To:         "peer",
		StoragePtr: &messaging.StoragePointer{Backend: "file", Path: "out/report.md"},
	})
	if err != nil {
		t.Fatalf("EmitFromWorker: %v", err)
	}
	if sent.To != "peer" {
		t.Fatalf("explicit recipient ignored: To = %q", sent.To)
	}

	sent, err = s.EmitFromWorker(ctx, "leaf", messaging.Envelope{Type: messaging.TypeCompletedWork})
	if err != nil {
		t.Fatalf("EmitFromWorker: %v", err)
	}
	if sent.To != "lead" {
		t.Fatalf("default recipient = %q, want the parent", sent.To)
	}
}

func TestEmitValidation(t *testing.T) {
	s := newTestServer(t, nil)
	ctx := context.Background()
	connect(t, s, "lead", "")
	connect(t, s, "leaf", "lead")

	t.Run("unknown type", func(t *testing.T) {
		_, err := s.EmitFromWorker(ctx, "leaf", messaging.Envelope{Type: "gossip"})
		if !errors.Is(err, ErrInvalidMessage) {
			t.Fatalf("want ErrInvalidMessage, got %v", err)
		}
	})

	t.Run("malformed storage pointer", func(t *testing.T) {
		_, err := s.EmitFromWorker(ctx, "leaf", messaging.Envelope{
			Type:       messaging.TypeCompletedWork,
			StoragePtr: &messaging.StoragePointer{Backend: "file"},
		})
		if !errors.Is(err, ErrInvalidMessage) {
			t.Fatalf("want ErrInvalidMessage, got %v", err)
		}
	})

	t.Run("unregistered sender", func(t *testing.T) {
		_, err := s.EmitFromWorker(ctx, "ghost", messaging.Envelope{Type: messaging.TypeStatusUpdate})
		if !errors.Is(err, orgchart.ErrNotFound) {
			t.Fatalf("want orgchart.ErrNotFound, got %v", err)
		}
	})

	t.Run("unregistered explicit recipient", func(t *testing.T) {
		_, err := s.EmitFromWorker(ctx, "leaf", messaging.Envelope{
			Type: messaging.TypeRequestForInput,
			To:   "ghost",
		})
		if !errors.Is(err, orgchart.ErrNotFound) {
			t.Fatalf("want orgchart.ErrNotFound, got %v", err)
		}
	})
}

func TestReassignAuthorization(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name     string
		caller   string
		isAdmin  bool
		wantAuth bool
	}{
		{name: "the worker itself", caller: "leaf", wantAuth: true},
		{name: "the current parent", caller: "lead", wantAuth: true},
		{name: "an admin", caller: "someone", isAdmin: true, wantAuth: true},
		{name: "an unrelated worker", caller: "peer", wantAuth: false},
		{name: "the incoming parent", caller: "other-lead", wantAuth: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServer(t, nil)
			connect(t, s, "lead", "")
			connect(t, s, "other-lead", "")
			connect(t, s, "leaf", "lead")
			connect(t, s, "peer", "lead")

			err := s.ReassignWorker(ctx, "leaf", "other-lead", tc.caller, tc.isAdmin)
			if tc.wantAuth {
				if err != nil {
					t.Fatalf("want the reassignment allowed, got %v", err)
				}
				target, gerr := s.Graph().EscalationTarget("leaf")
				if gerr != nil {
					t.Fatalf("EscalationTarget: %v", gerr)
				}
				if target != "other-lead" {
					t.Fatalf("reassignment not applied: leaf reports to %q", target)
				}
				stored, gerr := s.Store().GetWorker(ctx, "leaf")
				if gerr != nil {
					t.Fatalf("GetWorker: %v", gerr)
				}
				if stored.ReportsTo != "other-lead" {
					t.Fatalf("reassignment not persisted: stored reports_to = %q", stored.ReportsTo)
				}
				return
			}
			if !errors.Is(err, ErrNotAuthorized) {
				t.Fatalf("want ErrNotAuthorized, got %v", err)
			}
			target, gerr := s.Graph().EscalationTarget("leaf")
			if gerr != nil {
				t.Fatalf("EscalationTarget: %v", gerr)
			}
			if target != "lead" {
				t.Fatalf("refused reassignment still mutated the graph: leaf reports to %q", target)
			}
		})
	}
}

func TestReassignEmitsControlMessage(t *testing.T) {
	s := newTestServer(t, nil)
	ctx := context.Background()
	connect(t, s, "lead", "")
	connect(t, s, "other-lead", "")
	connect(t, s, "leaf", "lead")

	if err := s.ReassignWorker(ctx, "leaf", "other-lead", "lead", false); err != nil {
		t.Fatalf("ReassignWorker: %v", err)
	}

	for _, recipient := range []string{"leaf", "lead", "other-lead"} {
		msgs, err := s.Messaging().Tail(ctx, messaging.TailFilter{WorkerID: recipient})
		if err != nil {
			t.Fatalf("Tail(%q): %v", recipient, err)
		}
		if len(msgs) != 1 {
			t.Fatalf("%q got %d control messages, want 1", recipient, len(msgs))
		}
		env := msgs[0]
		if env.From != ControlWorkerID || env.Subject != SubjectWorkerReassigned {
			t.Fatalf("unexpected control message: from=%q subject=%q", env.From, env.Subject)
		}
		var body map[string]string
		if err := json.Unmarshal(env.Body, &body); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		if body["worker_id"] != "leaf" || body["old_parent"] != "lead" ||
			body["new_parent"] != "other-lead" || body["reassigned_by"] != "lead" {
			t.Fatalf("unexpected control body: %v", body)
		}
	}
}

func TestReassignToRootSkipsRootRecipient(t *testing.T) {
	s := newTestServer(t, nil)
	ctx := context.Background()
	connect(t, s, "lead", "")
	connect(t, s, "leaf", "lead")

	if err := s.ReassignWorker(ctx, "leaf", "", "leaf", false); err != nil {
		t.Fatalf("ReassignWorker: %v", err)
	}

	all, err := s.Messaging().Tail(ctx, messaging.TailFilter{})
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("want 2 control messages, got %d", len(all))
	}
	for _, env := range all {
		if env.To == "" {
			t.Fatal("a control message was addressed to the empty root")
		}
	}
}

func TestReassignUnknownWorker(t *testing.T) {
	s := newTestServer(t, nil)
	err := s.ReassignWorker(context.Background(), "ghost", "", "ghost", true)
	if !errors.Is(err, orgchart.ErrNotFound) {
		t.Fatalf("want orgchart.ErrNotFound, got %v", err)
	}
}

func TestConnectWorkerPersistsAndDefaults(t *testing.T) {
	clock := newFakeClock(time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC))
	s := newTestServer(t, func(c *Config) { c.Clock = clock })
	ctx := context.Background()

	if err := s.ConnectWorker(ctx, orgchart.Worker{ID: "w"}); err != nil {
		t.Fatalf("ConnectWorker: %v", err)
	}
	stored, err := s.Store().GetWorker(ctx, "w")
	if err != nil {
		t.Fatalf("GetWorker: %v", err)
	}
	if stored.Status != orgchart.StatusConnected {
		t.Fatalf("status = %q, want connected", stored.Status)
	}
	if !stored.ConnectedAt.Equal(clock.Now()) || !stored.LastSeenAt.Equal(clock.Now()) {
		t.Fatalf("timestamps not defaulted from the clock: %v / %v", stored.ConnectedAt, stored.LastSeenAt)
	}

	if err := s.ConnectWorker(ctx, orgchart.Worker{ID: "w"}); !errors.Is(err, orgchart.ErrDuplicateWorker) {
		t.Fatalf("want ErrDuplicateWorker on reconnect, got %v", err)
	}
	if err := s.ConnectWorker(ctx, orgchart.Worker{ID: "x", ReportsTo: "ghost"}); !errors.Is(err, orgchart.ErrParentNotFound) {
		t.Fatalf("want ErrParentNotFound, got %v", err)
	}
}

func TestTouchWorker(t *testing.T) {
	clock := newFakeClock(time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC))
	s := newTestServer(t, func(c *Config) { c.Clock = clock })
	ctx := context.Background()
	connect(t, s, "w", "")

	clock.Advance(30 * time.Minute)
	if err := s.TouchWorker(ctx, "w"); err != nil {
		t.Fatalf("TouchWorker: %v", err)
	}
	stored, err := s.Store().GetWorker(ctx, "w")
	if err != nil {
		t.Fatalf("GetWorker: %v", err)
	}
	if !stored.LastSeenAt.Equal(clock.Now()) {
		t.Fatalf("last_seen_at = %v, want %v", stored.LastSeenAt, clock.Now())
	}

	if err := s.TouchWorker(ctx, "ghost"); !errors.Is(err, orgchart.ErrNotFound) {
		t.Fatalf("want orgchart.ErrNotFound, got %v", err)
	}
}

func TestTokenMinting(t *testing.T) {
	clock := newFakeClock(time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC))
	s := newTestServer(t, func(c *Config) { c.Clock = clock })

	tok, err := s.MintWorkerToken("w")
	if err != nil {
		t.Fatalf("MintWorkerToken: %v", err)
	}
	if want := clock.Now().Add(DefaultTokenTTL); !tok.ExpiresAt.Equal(want) {
		t.Fatalf("expiry = %v, want %v", tok.ExpiresAt, want)
	}
	claims, err := s.Issuer().Validate(tok.Raw)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if claims.WorkerID != "w" {
		t.Fatalf("worker ID = %q", claims.WorkerID)
	}
	if claims.HasScope(AdminScope) {
		t.Fatal("a worker token carries the admin scope")
	}
	for _, scope := range DefaultScopes {
		if !claims.HasScope(scope) {
			t.Fatalf("missing default scope %q", scope)
		}
	}

	adminTok, err := s.MintAdminToken("operator", time.Hour)
	if err != nil {
		t.Fatalf("MintAdminToken: %v", err)
	}
	adminClaims, err := s.Issuer().Validate(adminTok.Raw)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if !adminClaims.HasScope(AdminScope) {
		t.Fatal("admin token lacks the admin scope")
	}

	clock.Advance(DefaultTokenTTL + time.Hour)
	if _, err := s.Issuer().Validate(tok.Raw); err == nil {
		t.Fatal("an expired token still validates")
	}
}

func TestNewRequiresDataDir(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("want an error for a missing DataDir, got nil")
	}
}

func TestNewRejectsUnknownBackendScheme(t *testing.T) {
	_, err := New(Config{DataDir: t.TempDir(), MessagingURI: "kafka://broker/topic"})
	if err == nil {
		t.Fatal("want an error for an unregistered messaging scheme, got nil")
	}
}

func TestHealth(t *testing.T) {
	s := newTestServer(t, nil)
	if err := s.Health(context.Background()); err != nil {
		t.Fatalf("Health: %v", err)
	}
}

func TestNewIDIsUnique(t *testing.T) {
	seen := make(map[string]bool, 1000)
	for i := 0; i < 1000; i++ {
		id := NewID()
		if seen[id] {
			t.Fatalf("duplicate ID %q", id)
		}
		seen[id] = true
	}
}

func TestValidateBackendURI(t *testing.T) {
	if err := ValidateBackendURI("queue://sqlite/tmp/m.db"); err != nil {
		t.Fatalf("want valid, got %v", err)
	}
	if err := ValidateBackendURI("/just/a/path"); err == nil {
		t.Fatal("want an error for a scheme-less URI, got nil")
	}
}

func TestValidateWorkerID(t *testing.T) {
	for _, id := range []string{"a", "music-playback-01", "host.example.com", "my_host", "7up", strings.Repeat("x", MaxWorkerIDLen)} {
		if err := ValidateWorkerID(id); err != nil {
			t.Errorf("ValidateWorkerID(%q) = %v, want nil", id, err)
		}
	}
	for _, id := range []string{"", "a/b", "/a", "a b", " a", "a\tb", "a\nb", "a\x00b", "a~b", "-a", ".a", "_a", "wörker", strings.Repeat("x", MaxWorkerIDLen+1)} {
		err := ValidateWorkerID(id)
		if !errors.Is(err, ErrInvalidWorkerID) {
			t.Errorf("ValidateWorkerID(%q) = %v, want ErrInvalidWorkerID", id, err)
		}
	}

	s := newTestServer(t, nil)
	err := s.ConnectWorker(context.Background(), orgchart.Worker{ID: "a/b", Role: "contributor"})
	if !errors.Is(err, ErrInvalidWorkerID) || !errors.Is(err, orgchart.ErrInvalidWorker) {
		t.Fatalf("ConnectWorker(a/b) = %v, want both ErrInvalidWorkerID and orgchart.ErrInvalidWorker", err)
	}
	if _, gerr := s.Graph().Get("a/b"); gerr == nil {
		t.Fatal("a/b was registered despite the invalid ID")
	}
}

func TestEnrollmentTokenRotation(t *testing.T) {
	s := newTestServer(t, nil)
	path := s.EnrollmentTokenPath()
	if path != EnrollmentTokenPath(s.cfg.DataDir) {
		t.Fatalf("EnrollmentTokenPath = %q, want %q", path, EnrollmentTokenPath(s.cfg.DataDir))
	}

	first := s.EnrollmentToken()
	if len(first) != 64 {
		t.Fatalf("initial enrollment token %q: want 32 hex-encoded bytes", first)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("enrollment token mode = %o, want 600", info.Mode().Perm())
	}

	second, err := RotateEnrollmentToken(path)
	if err != nil {
		t.Fatalf("RotateEnrollmentToken: %v", err)
	}
	if second == first || len(second) != 64 {
		t.Fatalf("rotated token %q is not a fresh 32-byte secret (old %q)", second, first)
	}
	if got := s.EnrollmentToken(); got != second {
		t.Fatalf("EnrollmentToken() after rotation = %q, want the new value %q", got, second)
	}
	info, err = os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s after rotation: %v", path, err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("rotated enrollment token mode = %o, want 600", info.Mode().Perm())
	}
	if leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "."+EnrollmentTokenFileName+".*")); len(leftovers) != 0 {
		t.Fatalf("rotation left temporary files behind: %v", leftovers)
	}

	third, err := s.RotateEnrollmentToken()
	if err != nil {
		t.Fatalf("Server.RotateEnrollmentToken: %v", err)
	}
	if got := s.EnrollmentToken(); got != third || third == second {
		t.Fatalf("EnrollmentToken() = %q after method rotation to %q (previous %q)", got, third, second)
	}

	if err := os.Remove(path); err != nil {
		t.Fatalf("remove %s: %v", path, err)
	}
	if got := s.EnrollmentToken(); got != "" {
		t.Fatalf("EnrollmentToken() with the file removed = %q, want empty", got)
	}
}
