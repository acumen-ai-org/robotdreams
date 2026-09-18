package api

import (
	"encoding/base64"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/acumen-ai-org/robotdreams/internal/identity"
	"github.com/acumen-ai-org/robotdreams/internal/server"
	"github.com/acumen-ai-org/robotdreams/pkg/security"
)

func (e *testEnv) delegate(token string, body any, wantStatus int) delegateResponse {
	e.t.Helper()
	status, raw := e.do(http.MethodPost, "/api/workers/delegate", token, body)
	if status != wantStatus {
		e.t.Fatalf("delegate: status %d, want %d: %s", status, wantStatus, raw)
	}
	var out delegateResponse
	if status == http.StatusCreated {
		decodeInto(e.t, raw, &out)
	}
	return out
}

func TestDelegateMintsAChildTheParentVouchesFor(t *testing.T) {
	e := newTestEnv(t, nil)
	parent := e.connect("lead-1", "lead", "")

	d := e.delegate(parent.Token, map[string]any{"child": "fetch-1"}, http.StatusCreated)

	if want := server.DelegatedWorkerID("lead-1", "fetch-1"); d.WorkerID != want {
		t.Fatalf("worker_id %q, want %q", d.WorkerID, want)
	}
	if d.DelegatedBy != "lead-1" {
		t.Fatalf("delegated_by %q, want lead-1", d.DelegatedBy)
	}
	claims, err := e.srv.Issuer().Validate(d.Token)
	if err != nil {
		t.Fatalf("the child token does not validate: %v", err)
	}
	if !claims.Delegated() || claims.DelegatedBy != "lead-1" || claims.WorkerID != d.WorkerID {
		t.Fatalf("claims %+v", claims)
	}
	want := server.WorkerScopes("lead-1")
	if strings.Join(claims.Scopes, " ") != strings.Join(want, " ") {
		t.Fatalf("scopes %v, want %v", claims.Scopes, want)
	}

	status, _ := e.do(http.MethodGet, "/api/workers/"+d.WorkerID, parent.Token, nil)
	if status != http.StatusNotFound {
		t.Fatalf("a delegated child appeared in the org chart (status %d)", status)
	}
}

func TestDelegatedChildActsWithinTheParentsScopes(t *testing.T) {
	e := newTestEnv(t, nil)
	parent := e.connect("lead-1", "lead", "")
	d := e.delegate(parent.Token, nil, http.StatusCreated)

	meta := e.putObject(d.Token, "workers/lead-1/out.txt", "done", "", http.StatusOK)
	if meta.UpdatedBy != d.WorkerID {
		t.Fatalf("updated_by %q, want %q", meta.UpdatedBy, d.WorkerID)
	}
	e.putObject(d.Token, "workers/other/out.txt", "nope", "", http.StatusForbidden)

	status, _ := e.do(http.MethodGet, "/api/events", d.Token, nil)
	if status != http.StatusForbidden {
		t.Fatalf("a child reached an admin endpoint (status %d)", status)
	}
}

func TestDelegateNarrowsScopesAndRefusesEscalation(t *testing.T) {
	e := newTestEnv(t, nil)
	parent := e.connect("lead-1", "lead", "")

	d := e.delegate(parent.Token, map[string]any{
		"scopes": []string{"storage:read:workers/lead-1/inbox/*", "message:send"},
	}, http.StatusCreated)
	if strings.Join(d.Scopes, " ") != "storage:read:workers/lead-1/inbox/* message:send" {
		t.Fatalf("scopes %v", d.Scopes)
	}
	e.putObject(d.Token, "workers/lead-1/inbox/x", "no write scope", "", http.StatusForbidden)

	for _, scopes := range [][]string{
		{"storage:write:workers/other/*"},
		{"storage:write:*"},
		{server.AdminScope},
		{"message:send", "nonsense:scope"},
	} {
		e.delegate(parent.Token, map[string]any{"scopes": scopes}, http.StatusBadRequest)
	}

	admin := e.adminToken("ops")
	d = e.delegate(admin, nil, http.StatusCreated)
	for _, s := range d.Scopes {
		if s == server.AdminScope {
			t.Fatalf("admin was delegated: %v", d.Scopes)
		}
	}
}

func TestDelegatedTokenCannotDelegateOrOutliveItsParent(t *testing.T) {
	clock := &fakeClock{now: time.Now()}
	e := newTestEnv(t, func(c *server.Config) {
		c.Clock = clock
		c.TokenTTL = 10 * time.Minute
		c.DelegatedTokenTTL = 5 * time.Minute
	})
	parent := e.connect("lead-1", "lead", "")

	d := e.delegate(parent.Token, nil, http.StatusCreated)
	e.delegate(d.Token, nil, http.StatusForbidden)

	e.delegate(parent.Token, map[string]any{"ttl_seconds": 6 * 60}, http.StatusBadRequest)

	clock.Advance(8 * time.Minute)
	d = e.delegate(parent.Token, nil, http.StatusCreated)
	if left := d.ExpiresAt.Sub(clock.Now()); left > 2*time.Minute+time.Second || left < 2*time.Minute-time.Second {
		t.Fatalf("child expires in %s, want about 2m (capped by the parent)", left)
	}
}

func TestRevokingTheParentRevokesTheChild(t *testing.T) {
	e := newTestEnv(t, nil)
	parent := e.connect("lead-1", "lead", "")
	d := e.delegate(parent.Token, nil, http.StatusCreated)

	if status, _ := e.do(http.MethodGet, "/api/workers", d.Token, nil); status != http.StatusOK {
		t.Fatalf("child cannot read before revocation: %d", status)
	}
	admin := e.adminToken("ops")
	if status, raw := e.do(http.MethodPost, "/api/workers/lead-1/revoke", admin, map[string]string{"reason": "test"}); status != http.StatusOK {
		t.Fatalf("revoke: %d %s", status, raw)
	}
	if status, _ := e.do(http.MethodGet, "/api/workers", d.Token, nil); status != http.StatusUnauthorized {
		t.Fatalf("child still accepted after its parent was revoked: %d", status)
	}
}

func TestConnectRefusesADelegatedLookingID(t *testing.T) {
	e := newTestEnv(t, nil)
	kp, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	nonce := e.challenge("lead-1~child")
	status, raw := e.do(http.MethodPost, "/api/workers", e.adminToken("ops"), connectRequest{
		WorkerID:  "lead-1~child",
		Role:      "lead",
		PublicKey: base64.StdEncoding.EncodeToString(kp.PublicKey),
		Nonce:     base64.StdEncoding.EncodeToString(nonce),
		Signature: base64.StdEncoding.EncodeToString(kp.SignNonce(nonce)),
	})
	if status != http.StatusBadRequest {
		t.Fatalf("connect with a ~ in the id: %d %s", status, raw)
	}
}

func TestDelegatedClaimsRoundTrip(t *testing.T) {
	c := security.Claims{WorkerID: "a~b", DelegatedBy: "a"}
	if !c.Delegated() {
		t.Fatal("Delegated() false with a parent set")
	}
	if (security.Claims{WorkerID: "a"}).Delegated() {
		t.Fatal("Delegated() true with no parent")
	}
}
