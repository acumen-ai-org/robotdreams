package server

import (
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/acumen-ai-org/robotdreams/pkg/security"
)

func TestDelegatedWorkerIDRoundTrip(t *testing.T) {
	tests := []struct {
		parent, child string
	}{
		{"lead-1", "abc123"},
		{"a", "b"},
		{"host.example.com", "sub.task_1-x"},
	}
	for _, tc := range tests {
		id := DelegatedWorkerID(tc.parent, tc.child)
		if want := tc.parent + DelegationSeparator + tc.child; id != want {
			t.Fatalf("DelegatedWorkerID(%q, %q) = %q, want %q", tc.parent, tc.child, id, want)
		}
		parent, child, ok := DelegationParent(id)
		if !ok || parent != tc.parent || child != tc.child {
			t.Fatalf("DelegationParent(%q) = (%q, %q, %v), want (%q, %q, true)", id, parent, child, ok, tc.parent, tc.child)
		}

		if err := ValidateWorkerID(id); !errors.Is(err, ErrInvalidWorkerID) {
			t.Fatalf("ValidateWorkerID(%q) = %v, want ErrInvalidWorkerID", id, err)
		}
	}
}

func TestDelegationParentRejectsOrdinaryAndMalformedIDs(t *testing.T) {
	for _, id := range []string{"", "lead-1", "~child", "parent~", "~"} {
		if parent, child, ok := DelegationParent(id); ok || parent != "" || child != "" {
			t.Fatalf("DelegationParent(%q) = (%q, %q, %v), want (\"\", \"\", false)", id, parent, child, ok)
		}
	}

	parent, child, ok := DelegationParent("a~b~c")
	if !ok || parent != "a" || child != "b~c" {
		t.Fatalf("DelegationParent(a~b~c) = (%q, %q, %v)", parent, child, ok)
	}
}

func TestCutStorageScope(t *testing.T) {
	tests := []struct {
		scope         string
		verb, pattern string
		ok            bool
	}{
		{"storage:read:a/*", "read", "a/*", true},
		{"storage:write:workers/x/file.txt", "write", "workers/x/file.txt", true},
		{"storage:read:a:b", "read", "a:b", true},
		{"storage:read:", "", "", false},
		{"storage::a/*", "", "", false},
		{"storage:read", "", "", false},
		{"message:send", "", "", false},
		{"admin", "", "", false},
		{"", "", "", false},
		{"storage", "", "", false},
	}
	for _, tc := range tests {
		verb, pattern, ok := cutStorageScope(tc.scope)
		if verb != tc.verb || pattern != tc.pattern || ok != tc.ok {
			t.Errorf("cutStorageScope(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tc.scope, verb, pattern, ok, tc.verb, tc.pattern, tc.ok)
		}
	}
}

func TestScopeCovered(t *testing.T) {
	held := []string{
		"message:send",
		"storage:read:a/*",
		"storage:write:workers/w/*",
		"storage:read:exact/path.txt",
		"admin",
	}
	tests := []struct {
		name string
		want string
		ok   bool
	}{
		{"non-storage scope held outright", "message:send", true},
		{"non-storage scope not held", "message:receive", false},
		{"non-storage scope never narrows", "message", false},
		{"admin held outright is still covered by the raw check", "admin", true},

		{"storage wildcard identical", "storage:read:a/*", true},
		{"storage wildcard narrower prefix", "storage:read:a/b/*", true},
		{"storage wildcard much narrower", "storage:read:a/b/c/d/*", true},
		{"storage exact path under wildcard", "storage:read:a/b/c", true},
		{"storage exact path equal to wildcard base", "storage:read:a/", true},
		{"storage wider than held", "storage:read:*", false},
		{"storage sibling prefix", "storage:read:ab/*", false},
		{"storage different root", "storage:read:b/*", false},

		{"verb mismatch read vs write", "storage:write:a/*", false},
		{"verb mismatch write vs read", "storage:read:workers/w/*", false},
		{"write narrower under write wildcard", "storage:write:workers/w/out/*", true},
		{"write exact under write wildcard", "storage:write:workers/w/out.txt", true},

		{"exact held covers itself", "storage:read:exact/path.txt", true},
		{"exact held does not cover a wildcard below it", "storage:read:exact/path.txt/*", false},
		{"exact held does not cover a sibling", "storage:read:exact/other.txt", false},

		{"malformed storage scope", "storage:read:", false},
		{"unknown verb", "storage:delete:a/*", false},
		{"empty want", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := scopeCovered(held, tc.want); got != tc.ok {
				t.Fatalf("scopeCovered(held, %q) = %v, want %v", tc.want, got, tc.ok)
			}
		})
	}

	if scopeCovered(nil, "message:send") {
		t.Fatal("empty held set covered a scope")
	}
	if scopeCovered([]string{"message:send"}, "storage:read:a/*") {
		t.Fatal("a non-storage held scope covered a storage request")
	}
}

func TestNarrowScopes(t *testing.T) {
	parent := []string{"message:send", "message:receive", "storage:read:workers/p/*", "storage:write:workers/p/*", AdminScope}

	t.Run("empty request is everything but admin", func(t *testing.T) {
		got, err := narrowScopes(parent, nil)
		if err != nil {
			t.Fatalf("narrowScopes: %v", err)
		}
		want := []string{"message:send", "message:receive", "storage:read:workers/p/*", "storage:write:workers/p/*"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("empty request from an admin-only parent is empty, not nil-error", func(t *testing.T) {
		got, err := narrowScopes([]string{AdminScope}, nil)
		if err != nil || len(got) != 0 {
			t.Fatalf("got %v, %v; want [], nil", got, err)
		}
	})

	t.Run("explicit request keeps order and allows narrowing", func(t *testing.T) {
		got, err := narrowScopes(parent, []string{"storage:read:workers/p/out/*", "message:send", "storage:write:workers/p/x.txt"})
		if err != nil {
			t.Fatalf("narrowScopes: %v", err)
		}
		want := "storage:read:workers/p/out/*,message:send,storage:write:workers/p/x.txt"
		if strings.Join(got, ",") != want {
			t.Fatalf("got %v, want %s", got, want)
		}
	})

	refusals := []struct {
		name string
		want []string
		msg  string
	}{
		{"admin requested explicitly", []string{AdminScope}, "admin scope cannot be delegated"},
		{"admin among valid scopes", []string{"message:send", AdminScope}, "admin scope cannot be delegated"},
		{"non-storage scope not held", []string{"message:broadcast"}, `"message:broadcast" is not held`},
		{"storage scope wider than held", []string{"storage:read:workers/*"}, `"storage:read:workers/*" is not held`},
		{"storage verb not held", []string{"storage:admin:workers/p/*"}, "is not held"},
		{"one bad scope poisons the set", []string{"message:send", "storage:read:other/*"}, `"storage:read:other/*" is not held`},
	}
	for _, tc := range refusals {
		t.Run(tc.name, func(t *testing.T) {
			got, err := narrowScopes(parent, tc.want)
			if !errors.Is(err, ErrInvalidDelegation) {
				t.Fatalf("err = %v, want ErrInvalidDelegation", err)
			}
			if !strings.Contains(err.Error(), tc.msg) {
				t.Fatalf("err = %q, want it to mention %q", err, tc.msg)
			}
			if got != nil {
				t.Fatalf("scopes returned alongside an error: %v", got)
			}
		})
	}
}

func TestRandomChildNameIsValidAndUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		name := randomChildName()
		if len(name) != 8 || !childNameRe.MatchString(name) {
			t.Fatalf("randomChildName() = %q, want 8 hex characters matching the child name rule", name)
		}
		if seen[name] {
			t.Fatalf("randomChildName() repeated %q", name)
		}
		seen[name] = true
	}
}

func parentClaims(workerID string, exp *time.Time) security.Claims {
	c := security.Claims{WorkerID: workerID, Scopes: WorkerScopes(workerID)}
	if exp != nil {
		c.ExpiresAt = jwt.NewNumericDate(*exp)
	}
	return c
}

func TestMintDelegatedTokenDefaults(t *testing.T) {
	start := time.Date(2026, 9, 13, 9, 0, 0, 0, time.UTC)
	clock := newFakeClock(start)
	s := newTestServer(t, func(c *Config) { c.Clock = clock })

	exp := start.Add(s.TokenTTL())
	d, err := s.MintDelegatedToken(parentClaims("lead-1", &exp), DelegationRequest{})
	if err != nil {
		t.Fatalf("MintDelegatedToken: %v", err)
	}
	parent, child, ok := DelegationParent(d.WorkerID)
	if !ok || parent != "lead-1" {
		t.Fatalf("child id %q does not name the parent", d.WorkerID)
	}
	if !regexp.MustCompile(`^[0-9a-f]{8}$`).MatchString(child) {
		t.Fatalf("random child name %q, want 8 hex characters", child)
	}
	if d.DelegatedBy != "lead-1" {
		t.Fatalf("DelegatedBy = %q", d.DelegatedBy)
	}
	if strings.Join(d.Scopes, ",") != strings.Join(WorkerScopes("lead-1"), ",") {
		t.Fatalf("default scopes = %v, want the parent's %v", d.Scopes, WorkerScopes("lead-1"))
	}
	if got := d.Token.TTL(); got != s.DelegatedTokenTTL() {
		t.Fatalf("token TTL = %s, want the default %s", got, s.DelegatedTokenTTL())
	}

	claims, err := s.Issuer().Validate(d.Token.Raw)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if claims.WorkerID != d.WorkerID || claims.DelegatedBy != "lead-1" || !claims.Delegated() {
		t.Fatalf("claims = %+v", claims)
	}

	if _, err := s.MintDelegatedToken(claims, DelegationRequest{}); !errors.Is(err, ErrCannotDelegate) {
		t.Fatalf("child delegating: err = %v, want ErrCannotDelegate", err)
	}
}

func TestMintDelegatedTokenHonorsRequest(t *testing.T) {
	start := time.Date(2026, 9, 13, 9, 0, 0, 0, time.UTC)
	clock := newFakeClock(start)
	s := newTestServer(t, func(c *Config) {
		c.Clock = clock
		c.DelegatedTokenTTL = 10 * time.Minute
	})
	exp := start.Add(time.Hour)

	d, err := s.MintDelegatedToken(parentClaims("lead-1", &exp), DelegationRequest{
		Child:  "batch.42_x-y",
		Scopes: []string{"storage:read:workers/lead-1/in/*", "message:send"},
		TTL:    3 * time.Minute,
	})
	if err != nil {
		t.Fatalf("MintDelegatedToken: %v", err)
	}
	if d.WorkerID != "lead-1~batch.42_x-y" {
		t.Fatalf("WorkerID = %q", d.WorkerID)
	}
	if strings.Join(d.Scopes, ",") != "storage:read:workers/lead-1/in/*,message:send" {
		t.Fatalf("Scopes = %v", d.Scopes)
	}
	if d.Token.TTL() != 3*time.Minute {
		t.Fatalf("TTL = %s, want 3m", d.Token.TTL())
	}
	if !d.Token.IssuedAt.Equal(start) {
		t.Fatalf("IssuedAt = %s, want the fake clock's %s", d.Token.IssuedAt, start)
	}
}

func TestMintDelegatedTokenCapsAtParentExpiry(t *testing.T) {
	start := time.Date(2026, 9, 13, 9, 0, 0, 0, time.UTC)
	clock := newFakeClock(start)
	s := newTestServer(t, func(c *Config) { c.Clock = clock })

	exp := start.Add(90 * time.Second)
	d, err := s.MintDelegatedToken(parentClaims("lead-1", &exp), DelegationRequest{})
	if err != nil {
		t.Fatalf("MintDelegatedToken: %v", err)
	}
	if d.Token.TTL() != 90*time.Second {
		t.Fatalf("TTL = %s, want the parent's remaining 90s", d.Token.TTL())
	}
	if !d.Token.ExpiresAt.Equal(exp) {
		t.Fatalf("ExpiresAt = %s, want the parent's %s", d.Token.ExpiresAt, exp)
	}

	d, err = s.MintDelegatedToken(parentClaims("lead-1", nil), DelegationRequest{})
	if err != nil {
		t.Fatalf("MintDelegatedToken (no exp): %v", err)
	}
	if d.Token.TTL() != DefaultDelegatedTokenTTL {
		t.Fatalf("TTL = %s, want %s", d.Token.TTL(), DefaultDelegatedTokenTTL)
	}

	clock.Advance(2 * time.Minute)
	_, err = s.MintDelegatedToken(parentClaims("lead-1", &exp), DelegationRequest{})
	if !errors.Is(err, ErrInvalidDelegation) || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired parent: err = %v, want ErrInvalidDelegation mentioning expiry", err)
	}
}

func TestMintDelegatedTokenRefusals(t *testing.T) {
	start := time.Date(2026, 9, 13, 9, 0, 0, 0, time.UTC)
	s := newTestServer(t, func(c *Config) {
		c.Clock = newFakeClock(start)
		c.DelegatedTokenTTL = 2 * time.Minute
	})
	exp := start.Add(time.Hour)
	parent := parentClaims("lead-1", &exp)

	tests := []struct {
		name string
		p    security.Claims
		req  DelegationRequest
		want error
		msg  string
	}{
		{"delegated caller", security.Claims{WorkerID: "lead-1~c", DelegatedBy: "lead-1", Scopes: parent.Scopes}, DelegationRequest{}, ErrCannotDelegate, ""},
		{"caller without worker id", security.Claims{Scopes: parent.Scopes}, DelegationRequest{}, ErrInvalidDelegation, "no worker id"},
		{"child name with separator", parent, DelegationRequest{Child: "a~b"}, ErrInvalidDelegation, "child name"},
		{"child name with slash", parent, DelegationRequest{Child: "a/b"}, ErrInvalidDelegation, "child name"},
		{"child name leading dot", parent, DelegationRequest{Child: ".hidden"}, ErrInvalidDelegation, "child name"},
		{"child name with space", parent, DelegationRequest{Child: "a b"}, ErrInvalidDelegation, "child name"},
		{"child name too long", parent, DelegationRequest{Child: strings.Repeat("x", MaxDelegatedChildName+1)}, ErrInvalidDelegation, "child name"},
		{"admin scope", parent, DelegationRequest{Scopes: []string{AdminScope}}, ErrInvalidDelegation, "admin"},
		{"scope not held", parent, DelegationRequest{Scopes: []string{"storage:read:workers/other/*"}}, ErrInvalidDelegation, "not held"},
		{"ttl over the maximum", parent, DelegationRequest{TTL: 3 * time.Minute}, ErrInvalidDelegation, "exceeds the maximum"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d, err := s.MintDelegatedToken(tc.p, tc.req)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if tc.msg != "" && !strings.Contains(err.Error(), tc.msg) {
				t.Fatalf("err = %q, want it to mention %q", err, tc.msg)
			}
			if d.Token.Raw != "" {
				t.Fatal("a token was minted alongside the error")
			}
		})
	}

	if _, err := s.MintDelegatedToken(parent, DelegationRequest{Child: strings.Repeat("x", MaxDelegatedChildName), TTL: 2 * time.Minute}); err != nil {
		t.Fatalf("boundary request refused: %v", err)
	}
}
