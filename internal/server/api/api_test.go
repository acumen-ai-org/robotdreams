package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/acumen-ai-org/robotdreams/internal/identity"
	"github.com/acumen-ai-org/robotdreams/internal/server"
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

type testEnv struct {
	t   *testing.T
	srv *server.Server
	api *API
	ts  *httptest.Server
}

func newTestEnv(t *testing.T, mutate func(*server.Config)) *testEnv {
	t.Helper()

	cfg := server.Config{DataDir: t.TempDir(), Clock: security.RealClock{}}
	if mutate != nil {
		mutate(&cfg)
	}
	srv, err := server.New(cfg)
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	a := New(srv)
	ts := httptest.NewServer(NewAPIRouter(a))
	t.Cleanup(ts.Close)

	return &testEnv{t: t, srv: srv, api: a, ts: ts}
}

func (e *testEnv) do(method, path, token string, body any) (int, []byte) {
	e.t.Helper()

	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			e.t.Fatalf("marshal request: %v", err)
		}
		rdr = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, e.ts.URL+path, rdr)
	if err != nil {
		e.t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := e.ts.Client().Do(req)
	if err != nil {
		e.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		e.t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, raw
}

func decodeInto(t *testing.T, raw []byte, dst any) {
	t.Helper()
	if err := json.Unmarshal(raw, dst); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
}

type testWorker struct {
	ID    string
	Keys  identity.KeyPair
	Token string
}

func (e *testEnv) challenge(workerID string) []byte {
	e.t.Helper()

	status, raw := e.do(http.MethodPost, "/api/workers/challenge", "", challengeRequest{WorkerID: workerID})
	if status != http.StatusOK {
		e.t.Fatalf("challenge: status %d, body %s", status, raw)
	}
	var resp challengeResponse
	decodeInto(e.t, raw, &resp)

	nonce, err := base64.StdEncoding.DecodeString(resp.Nonce)
	if err != nil {
		e.t.Fatalf("decode nonce: %v", err)
	}
	if len(nonce) != nonceSize {
		e.t.Fatalf("nonce length = %d, want %d", len(nonce), nonceSize)
	}
	return nonce
}

func (e *testEnv) connect(workerID, role, reportsTo string) testWorker {
	e.t.Helper()

	kp, err := identity.GenerateKeyPair()
	if err != nil {
		e.t.Fatalf("GenerateKeyPair: %v", err)
	}
	nonce := e.challenge(workerID)

	status, raw := e.do(http.MethodPost, "/api/workers", e.adminToken("test-enroller"), connectRequest{
		WorkerID:  workerID,
		Role:      role,
		ReportsTo: reportsTo,
		PublicKey: base64.StdEncoding.EncodeToString(kp.PublicKey),
		Nonce:     base64.StdEncoding.EncodeToString(nonce),
		Signature: base64.StdEncoding.EncodeToString(kp.SignNonce(nonce)),
	})
	if status != http.StatusCreated {
		e.t.Fatalf("connect %q: status %d, body %s", workerID, status, raw)
	}

	var resp tokenResponse
	decodeInto(e.t, raw, &resp)
	if resp.Token == "" {
		e.t.Fatalf("connect %q returned no token", workerID)
	}
	return testWorker{ID: workerID, Keys: kp, Token: resp.Token}
}

func (e *testEnv) adminToken(subject string) string {
	e.t.Helper()
	tok, err := e.srv.MintAdminToken(subject, time.Hour)
	if err != nil {
		e.t.Fatalf("MintAdminToken: %v", err)
	}
	return tok.Raw
}

func TestConnectHappyPath(t *testing.T) {
	e := newTestEnv(t, nil)

	w := e.connect("lead-1", "lead", "")

	status, raw := e.do(http.MethodGet, "/api/workers/lead-1", w.Token, nil)
	if status != http.StatusOK {
		t.Fatalf("GET worker: status %d, body %s", status, raw)
	}
	var view workerView
	decodeInto(t, raw, &view)
	if view.ID != "lead-1" || view.Role != "lead" || view.Status != "connected" {
		t.Fatalf("unexpected worker: %+v", view)
	}
	if view.PublicKey != base64.StdEncoding.EncodeToString(w.Keys.PublicKey) {
		t.Fatal("registered public key does not match the one presented at connect")
	}
	if view.ConnectedAt.IsZero() || view.LastSeenAt.IsZero() {
		t.Fatalf("connect timestamps not recorded: %+v", view)
	}

	if _, err := e.srv.Store().GetWorker(context.Background(), "lead-1"); err != nil {
		t.Fatalf("worker not persisted: %v", err)
	}
}

func TestConnectBadSignatureRejected(t *testing.T) {
	e := newTestEnv(t, nil)

	kp, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	attacker, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	nonce := e.challenge("impostor")
	status, raw := e.do(http.MethodPost, "/api/workers", e.adminToken("test-enroller"), connectRequest{
		WorkerID:  "impostor",
		Role:      "contributor",
		PublicKey: base64.StdEncoding.EncodeToString(kp.PublicKey),
		Nonce:     base64.StdEncoding.EncodeToString(nonce),
		Signature: base64.StdEncoding.EncodeToString(attacker.SignNonce(nonce)),
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body %s", status, raw)
	}

	if _, err := e.srv.Graph().Get("impostor"); err == nil {
		t.Fatal("a worker with a bad signature was added to the org chart")
	}
	if _, err := e.srv.Store().GetWorker(context.Background(), "impostor"); err == nil {
		t.Fatal("a worker with a bad signature was persisted")
	}
}

func TestConnectNonceIsSingleUse(t *testing.T) {
	e := newTestEnv(t, nil)

	kp, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	nonce := e.challenge("replay")
	req := connectRequest{
		WorkerID:  "replay",
		PublicKey: base64.StdEncoding.EncodeToString(kp.PublicKey),
		Nonce:     base64.StdEncoding.EncodeToString(nonce),
		Signature: base64.StdEncoding.EncodeToString(kp.SignNonce(nonce)),
	}

	adminTok := e.adminToken("test-enroller")
	if status, raw := e.do(http.MethodPost, "/api/workers", adminTok, req); status != http.StatusCreated {
		t.Fatalf("first connect: status %d, body %s", status, raw)
	}
	status, raw := e.do(http.MethodPost, "/api/workers", adminTok, req)
	if status != http.StatusUnauthorized {
		t.Fatalf("replayed nonce: status %d, want 401; body %s", status, raw)
	}
}

func TestConnectRejectsUnknownParent(t *testing.T) {
	e := newTestEnv(t, nil)

	kp, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	nonce := e.challenge("orphan")
	status, raw := e.do(http.MethodPost, "/api/workers", e.adminToken("test-enroller"), connectRequest{
		WorkerID:  "orphan",
		ReportsTo: "nobody",
		PublicKey: base64.StdEncoding.EncodeToString(kp.PublicKey),
		Nonce:     base64.StdEncoding.EncodeToString(nonce),
		Signature: base64.StdEncoding.EncodeToString(kp.SignNonce(nonce)),
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body %s", status, raw)
	}
}

func TestTokenRefresh(t *testing.T) {
	e := newTestEnv(t, nil)
	w := e.connect("refresher", "contributor", "")

	nonce := e.challenge("refresher")
	status, raw := e.do(http.MethodPost, "/api/token", "", refreshRequest{
		WorkerID:  "refresher",
		Nonce:     base64.StdEncoding.EncodeToString(nonce),
		Signature: base64.StdEncoding.EncodeToString(w.Keys.SignNonce(nonce)),
	})
	if status != http.StatusOK {
		t.Fatalf("refresh: status %d, body %s", status, raw)
	}
	var resp tokenResponse
	decodeInto(t, raw, &resp)
	if resp.Token == "" {
		t.Fatal("refresh returned no token")
	}
	if _, err := e.srv.Issuer().Validate(resp.Token); err != nil {
		t.Fatalf("refreshed token does not validate: %v", err)
	}

	other, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	nonce = e.challenge("refresher")
	status, raw = e.do(http.MethodPost, "/api/token", "", refreshRequest{
		WorkerID:  "refresher",
		Nonce:     base64.StdEncoding.EncodeToString(nonce),
		Signature: base64.StdEncoding.EncodeToString(other.SignNonce(nonce)),
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("wrong-key refresh: status %d, want 401; body %s", status, raw)
	}
}

func TestTokenRefreshUnknownWorker(t *testing.T) {
	e := newTestEnv(t, nil)
	nonce := e.challenge("ghost")
	kp, _ := identity.GenerateKeyPair()

	status, _ := e.do(http.MethodPost, "/api/token", "", refreshRequest{
		WorkerID:  "ghost",
		Nonce:     base64.StdEncoding.EncodeToString(nonce),
		Signature: base64.StdEncoding.EncodeToString(kp.SignNonce(nonce)),
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", status)
	}
}

func TestChallengeExpires(t *testing.T) {
	clock := newFakeClock(time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC))
	e := newTestEnv(t, func(c *server.Config) { c.Clock = clock })

	kp, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	nonce := e.challenge("slowpoke")
	clock.Advance(challengeTTL + time.Second)

	status, raw := e.do(http.MethodPost, "/api/workers", e.adminToken("test-enroller"), connectRequest{
		WorkerID:  "slowpoke",
		PublicKey: base64.StdEncoding.EncodeToString(kp.PublicKey),
		Nonce:     base64.StdEncoding.EncodeToString(nonce),
		Signature: base64.StdEncoding.EncodeToString(kp.SignNonce(nonce)),
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("expired challenge: status %d, want 401; body %s", status, raw)
	}
}

func TestJWKSIsPublicAndUsable(t *testing.T) {
	e := newTestEnv(t, nil)
	w := e.connect("keyholder", "contributor", "")

	status, raw := e.do(http.MethodGet, "/api/.well-known/jwks.json", "", nil)
	if status != http.StatusOK {
		t.Fatalf("jwks: status %d, body %s", status, raw)
	}

	set, err := identity.UnmarshalJWKS(raw)
	if err != nil {
		t.Fatalf("UnmarshalJWKS: %v", err)
	}
	claims, err := identity.ValidateToken(w.Token, set, e.srv.Clock())
	if err != nil {
		t.Fatalf("token does not validate against the published JWKS: %v", err)
	}
	if claims.WorkerID != "keyholder" {
		t.Fatalf("worker ID = %q", claims.WorkerID)
	}
}

func TestProtectedEndpointsRequireAuth(t *testing.T) {
	e := newTestEnv(t, nil)

	cases := []struct{ method, path string }{
		{http.MethodGet, "/api/workers"},
		{http.MethodGet, "/api/workers/x"},
		{http.MethodPost, "/api/workers/x/reassign"},
		{http.MethodPost, "/api/workers/x/revoke"},
		{http.MethodPost, "/api/messages"},
		{http.MethodGet, "/api/messages"},
		{http.MethodGet, "/api/messages/subscribe"},
		{http.MethodPost, "/api/messages/x/ack"},
		{http.MethodPost, "/api/storage/objects?path=p"},
		{http.MethodGet, "/api/storage/objects?path=p"},
		{http.MethodDelete, "/api/storage/objects?path=p"},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			if status, _ := e.do(tc.method, tc.path, "", nil); status != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", status)
			}
		})
	}
}

func TestGarbageTokenRejected(t *testing.T) {
	e := newTestEnv(t, nil)
	if status, _ := e.do(http.MethodGet, "/api/workers", "not-a-jwt", nil); status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", status)
	}
}

func TestRevokedWorkerIsRejectedImmediately(t *testing.T) {
	e := newTestEnv(t, nil)
	w := e.connect("compromised", "contributor", "")

	if status, raw := e.do(http.MethodGet, "/api/workers", w.Token, nil); status != http.StatusOK {
		t.Fatalf("pre-revoke call: status %d, body %s", status, raw)
	}

	admin := e.adminToken("operator")
	status, raw := e.do(http.MethodPost, "/api/workers/compromised/revoke", admin, revokeRequest{Reason: "key leaked"})
	if status != http.StatusOK {
		t.Fatalf("revoke: status %d, body %s", status, raw)
	}

	if _, err := e.srv.Issuer().Validate(w.Token); err != nil {
		t.Fatalf("precondition: token should still be signature-valid, got %v", err)
	}
	status, raw = e.do(http.MethodGet, "/api/workers", w.Token, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("post-revoke call: status %d, want 401; body %s", status, raw)
	}

	nonce := e.challenge("compromised")
	status, _ = e.do(http.MethodPost, "/api/token", "", refreshRequest{
		WorkerID:  "compromised",
		Nonce:     base64.StdEncoding.EncodeToString(nonce),
		Signature: base64.StdEncoding.EncodeToString(w.Keys.SignNonce(nonce)),
	})
	if status != http.StatusForbidden {
		t.Fatalf("post-revoke refresh: status %d, want 403", status)
	}
}

func TestRevokeRequiresAdminScope(t *testing.T) {
	e := newTestEnv(t, nil)
	lead := e.connect("lead", "lead", "")
	e.connect("leaf", "contributor", "lead")

	if status, _ := e.do(http.MethodPost, "/api/workers/leaf/revoke", lead.Token, nil); status != http.StatusForbidden {
		t.Fatalf("peer revoke: status %d, want 403", status)
	}
	if status, _ := e.do(http.MethodPost, "/api/workers/lead/revoke", lead.Token, nil); status != http.StatusForbidden {
		t.Fatalf("self revoke: status %d, want 403", status)
	}
	if status, _ := e.do(http.MethodGet, "/api/workers", lead.Token, nil); status != http.StatusOK {
		t.Fatalf("refused revoke had a side effect: status %d", status)
	}
}

func TestRevocationCacheWindow(t *testing.T) {
	clock := newFakeClock(time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC))
	e := newTestEnv(t, func(c *server.Config) { c.Clock = clock })
	w := e.connect("cached", "contributor", "")

	if status, _ := e.do(http.MethodGet, "/api/workers", w.Token, nil); status != http.StatusOK {
		t.Fatalf("warm-up call failed")
	}

	if err := e.srv.Revocations().Revoke(context.Background(), "cached", "out of band"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	clock.Advance(revocationCacheTTL / 2)
	if status, _ := e.do(http.MethodGet, "/api/workers", w.Token, nil); status != http.StatusOK {
		t.Fatalf("status = %d, want the cached answer to still allow the call", status)
	}
	clock.Advance(revocationCacheTTL)
	if status, _ := e.do(http.MethodGet, "/api/workers", w.Token, nil); status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 once the cache window has passed", status)
	}
}

func TestListWorkers(t *testing.T) {
	e := newTestEnv(t, nil)
	lead := e.connect("lead", "lead", "")
	e.connect("leaf", "contributor", "lead")

	status, raw := e.do(http.MethodGet, "/api/workers", lead.Token, nil)
	if status != http.StatusOK {
		t.Fatalf("status %d, body %s", status, raw)
	}
	var resp struct {
		Workers []workerView `json:"workers"`
	}
	decodeInto(t, raw, &resp)
	if len(resp.Workers) != 2 {
		t.Fatalf("want 2 workers, got %d", len(resp.Workers))
	}
	if resp.Workers[0].ID != "lead" || resp.Workers[1].ID != "leaf" {
		t.Fatalf("want results sorted by ID, got %q then %q", resp.Workers[0].ID, resp.Workers[1].ID)
	}
}

func TestGetUnknownWorker(t *testing.T) {
	e := newTestEnv(t, nil)
	w := e.connect("w", "contributor", "")
	if status, _ := e.do(http.MethodGet, "/api/workers/ghost", w.Token, nil); status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
}

func TestReassignAuthorizationOverHTTP(t *testing.T) {
	cases := []struct {
		name       string
		caller     string
		wantStatus int
	}{
		{name: "the worker itself", caller: "leaf", wantStatus: http.StatusOK},
		{name: "the current parent", caller: "lead", wantStatus: http.StatusOK},
		{name: "an admin", caller: "", wantStatus: http.StatusOK},
		{name: "an unrelated worker", caller: "peer", wantStatus: http.StatusForbidden},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newTestEnv(t, nil)
			lead := e.connect("lead", "lead", "")
			other := e.connect("other-lead", "lead", "")
			leaf := e.connect("leaf", "contributor", "lead")
			peer := e.connect("peer", "contributor", "lead")
			_ = other

			token := map[string]string{
				"lead": lead.Token,
				"leaf": leaf.Token,
				"peer": peer.Token,
			}[tc.caller]
			if tc.caller == "" {
				token = e.adminToken("mission-control")
			}

			status, raw := e.do(http.MethodPost, "/api/workers/leaf/reassign", token, reassignRequest{ReportsTo: "other-lead"})
			if status != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body %s", status, tc.wantStatus, raw)
			}

			wantParent := "lead"
			if tc.wantStatus == http.StatusOK {
				wantParent = "other-lead"
			}
			got, err := e.srv.Graph().Get("leaf")
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if got.ReportsTo != wantParent {
				t.Fatalf("leaf reports to %q, want %q", got.ReportsTo, wantParent)
			}
		})
	}
}

func TestReassignEmitsVisibleControlMessage(t *testing.T) {
	e := newTestEnv(t, nil)
	lead := e.connect("lead", "lead", "")
	e.connect("other-lead", "lead", "")
	e.connect("leaf", "contributor", "lead")

	if status, raw := e.do(http.MethodPost, "/api/workers/leaf/reassign", lead.Token, reassignRequest{ReportsTo: "other-lead"}); status != http.StatusOK {
		t.Fatalf("reassign: status %d, body %s", status, raw)
	}

	status, raw := e.do(http.MethodGet, "/api/messages?worker_id=lead", lead.Token, nil)
	if status != http.StatusOK {
		t.Fatalf("tail: status %d, body %s", status, raw)
	}
	var resp struct {
		Messages []envelopeView `json:"messages"`
	}
	decodeInto(t, raw, &resp)
	if len(resp.Messages) != 1 {
		t.Fatalf("want 1 control message for the old parent, got %d", len(resp.Messages))
	}
	if resp.Messages[0].Subject != server.SubjectWorkerReassigned || resp.Messages[0].From != server.ControlWorkerID {
		t.Fatalf("unexpected control message: %+v", resp.Messages[0])
	}
}

func TestReassignCycleRejected(t *testing.T) {
	e := newTestEnv(t, nil)
	admin := e.adminToken("mission-control")
	e.connect("lead", "lead", "")
	e.connect("leaf", "contributor", "lead")

	status, raw := e.do(http.MethodPost, "/api/workers/lead/reassign", admin, reassignRequest{ReportsTo: "leaf"})
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body %s", status, raw)
	}
}

func TestEscalationRoutesExactlyOneHop(t *testing.T) {
	e := newTestEnv(t, nil)
	root := e.connect("root", "lead", "")
	lead := e.connect("lead", "lead", "root")
	leaf := e.connect("leaf", "contributor", "lead")

	status, raw := e.do(http.MethodPost, "/api/messages", leaf.Token, emitRequest{
		Type:    string(messaging.TypeEscalation),
		To:      "root",
		Subject: "blocked on credentials",
	})
	if status != http.StatusAccepted {
		t.Fatalf("emit: status %d, body %s", status, raw)
	}
	var sent envelopeView
	decodeInto(t, raw, &sent)
	if sent.To != "lead" {
		t.Fatalf("escalation addressed to %q, want the immediate parent", sent.To)
	}
	if sent.From != "leaf" {
		t.Fatalf("From = %q, want the authenticated sender", sent.From)
	}
	if !sent.Delivered {
		t.Fatal("escalation reported as undelivered")
	}

	got := e.subscribeOne(lead, 5*time.Second)
	if got.ID != sent.ID || got.To != "lead" || got.Type != string(messaging.TypeEscalation) {
		t.Fatalf("subscriber received %+v, want the emitted escalation", got)
	}

	assertInboxSize(t, e, root, 0)
	assertInboxSize(t, e, leaf, 0)
}

func TestStatusUpdateAggregatesOneHop(t *testing.T) {
	e := newTestEnv(t, nil)
	root := e.connect("root", "lead", "")
	lead := e.connect("lead", "lead", "root")
	leaf := e.connect("leaf", "contributor", "lead")

	status, raw := e.do(http.MethodPost, "/api/messages", leaf.Token, emitRequest{
		Type:    string(messaging.TypeStatusUpdate),
		To:      "root",
		Subject: "60% done",
		Body:    json.RawMessage(`{"percent":60}`),
	})
	if status != http.StatusAccepted {
		t.Fatalf("emit: status %d, body %s", status, raw)
	}
	var sent envelopeView
	decodeInto(t, raw, &sent)
	if sent.To != "lead" {
		t.Fatalf("status update addressed to %q, want the immediate parent", sent.To)
	}

	got := e.subscribeOne(lead, 5*time.Second)
	if got.ID != sent.ID || got.Subject != "60% done" {
		t.Fatalf("subscriber received %+v", got)
	}
	assertInboxSize(t, e, root, 0)
}

func TestEscalationAtRootIsRefused(t *testing.T) {
	e := newTestEnv(t, nil)
	top := e.connect("top", "lead", "")

	status, raw := e.do(http.MethodPost, "/api/messages", top.Token, emitRequest{
		Type: string(messaging.TypeEscalation),
	})
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body %s", status, raw)
	}
}

func TestStatusUpdateAtRootIsDropped(t *testing.T) {
	e := newTestEnv(t, nil)
	top := e.connect("top", "lead", "")

	status, raw := e.do(http.MethodPost, "/api/messages", top.Token, emitRequest{
		Type: string(messaging.TypeStatusUpdate),
	})
	if status != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body %s", status, raw)
	}
	var sent envelopeView
	decodeInto(t, raw, &sent)
	if sent.Delivered || sent.To != "" {
		t.Fatalf("want an undelivered envelope with no recipient, got %+v", sent)
	}
	assertInboxSize(t, e, top, 0)
}

func TestEmitRejectsUnknownType(t *testing.T) {
	e := newTestEnv(t, nil)
	e.connect("lead", "lead", "")
	leaf := e.connect("leaf", "contributor", "lead")

	status, _ := e.do(http.MethodPost, "/api/messages", leaf.Token, emitRequest{Type: "gossip"})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
}

func TestAckMessage(t *testing.T) {
	e := newTestEnv(t, nil)
	lead := e.connect("lead", "lead", "")
	leaf := e.connect("leaf", "contributor", "lead")

	_, raw := e.do(http.MethodPost, "/api/messages", leaf.Token, emitRequest{
		Type:    string(messaging.TypeEscalation),
		Subject: "help",
	})
	var sent envelopeView
	decodeInto(t, raw, &sent)

	status, raw := e.do(http.MethodPost, "/api/messages/"+sent.ID+"/ack", lead.Token, ackRequest{Action: "handled"})
	if status != http.StatusOK {
		t.Fatalf("ack: status %d, body %s", status, raw)
	}
	if status, _ := e.do(http.MethodPost, "/api/messages/"+sent.ID+"/ack", lead.Token, ackRequest{Action: "handled"}); status != http.StatusOK {
		t.Fatalf("re-ack: status %d, want 200", status)
	}
	assertInboxSize(t, e, lead, 1)

	if status, _ := e.do(http.MethodPost, "/api/messages/no-such-id/ack", lead.Token, ackRequest{}); status != http.StatusNotFound {
		t.Fatalf("ack unknown: status = %d, want 404", status)
	}
}

func TestSubscribeToAnotherWorkerRequiresAdmin(t *testing.T) {
	e := newTestEnv(t, nil)
	lead := e.connect("lead", "lead", "")
	leaf := e.connect("leaf", "contributor", "lead")

	status, _ := e.do(http.MethodGet, "/api/messages/subscribe?worker_id=lead", leaf.Token, nil)
	if status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", status)
	}
	status, _ = e.do(http.MethodGet, "/api/messages?worker_id=lead", leaf.Token, nil)
	if status != http.StatusForbidden {
		t.Fatalf("tail: status = %d, want 403", status)
	}
	_ = lead
}

func TestStoragePutGetRoundtrip(t *testing.T) {
	e := newTestEnv(t, nil)
	w := e.connect("writer", "contributor", "")

	content := "the quick brown fox\n"
	meta := e.putObject(w.Token, "shared/notes/fox.txt", content, "", http.StatusOK)
	if meta.Path != "shared/notes/fox.txt" || meta.Size != int64(len(content)) {
		t.Fatalf("unexpected meta: %+v", meta)
	}
	if meta.Revision == "" {
		t.Fatal("no revision returned")
	}
	if meta.UpdatedBy != "writer" {
		t.Fatalf("updated_by = %q, want %q", meta.UpdatedBy, "writer")
	}

	req, err := http.NewRequest(http.MethodGet, e.ts.URL+"/api/storage/objects?path=shared/notes/fox.txt", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+w.Token)
	resp, err := e.ts.Client().Do(req)
	if err != nil {
		t.Fatalf("get object: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(body) != content {
		t.Fatalf("content = %q, want %q", body, content)
	}
	if got := resp.Header.Get("X-Revision"); got != meta.Revision {
		t.Fatalf("X-Revision = %q, want %q", got, meta.Revision)
	}
	if resp.Header.Get("X-Updated-At") == "" {
		t.Fatal("X-Updated-At missing")
	}
	if got := resp.Header.Get("X-Updated-By"); got != "writer" {
		t.Fatalf("X-Updated-By = %q, want %q", got, "writer")
	}

	status, raw := e.do(http.MethodGet, "/api/storage/objects?prefix=shared/notes/", w.Token, nil)
	if status != http.StatusOK {
		t.Fatalf("list: status %d, body %s", status, raw)
	}
	var listed struct {
		Objects []objectMetaView `json:"objects"`
	}
	decodeInto(t, raw, &listed)
	if len(listed.Objects) != 1 {
		t.Fatalf("list returned %d objects, want 1", len(listed.Objects))
	}
	if listed.Objects[0].UpdatedBy != "writer" {
		t.Fatalf("listed updated_by = %q, want %q", listed.Objects[0].UpdatedBy, "writer")
	}
}

func TestStorageIfMatchRevisionConflict(t *testing.T) {
	e := newTestEnv(t, nil)
	w := e.connect("writer", "contributor", "")

	first := e.putObject(w.Token, "shared/doc.txt", "v1", "", http.StatusOK)

	second := e.putObject(w.Token, "shared/doc.txt", "v2", first.Revision, http.StatusOK)
	if second.Revision == first.Revision {
		t.Fatal("revision did not change after a write")
	}

	status, raw := e.putObjectRaw(w.Token, "shared/doc.txt", "v3", first.Revision)
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body %s", status, raw)
	}
	var errBody errorResponse
	decodeInto(t, raw, &errBody)
	if errBody.Error == "" {
		t.Fatalf("want a JSON error body, got %s", raw)
	}

	status, raw = e.do(http.MethodGet, "/api/storage/objects?path=shared/doc.txt", w.Token, nil)
	if status != http.StatusOK || string(raw) != "v2" {
		t.Fatalf("object was modified by the refused write: status %d, body %q", status, raw)
	}
}

func TestStorageListByPrefix(t *testing.T) {
	e := newTestEnv(t, nil)
	w := e.connect("writer", "contributor", "")

	e.putObject(w.Token, "shared/a/one.txt", "1", "", http.StatusOK)
	e.putObject(w.Token, "shared/a/two.txt", "2", "", http.StatusOK)
	e.putObject(w.Token, "shared/b/three.txt", "3", "", http.StatusOK)

	status, raw := e.do(http.MethodGet, "/api/storage/objects?prefix=shared/a/", w.Token, nil)
	if status != http.StatusOK {
		t.Fatalf("status %d, body %s", status, raw)
	}
	var resp struct {
		Objects []objectMetaView `json:"objects"`
	}
	decodeInto(t, raw, &resp)
	if len(resp.Objects) != 2 {
		t.Fatalf("want 2 objects under shared/a/, got %d", len(resp.Objects))
	}

	status, raw = e.do(http.MethodGet, "/api/storage/objects?prefix=", w.Token, nil)
	if status != http.StatusForbidden {
		t.Fatalf("empty-prefix list by a non-admin worker: status %d, want 403; body %s", status, raw)
	}

	status, raw = e.do(http.MethodGet, "/api/storage/objects?prefix=", e.adminToken("test-admin"), nil)
	if status != http.StatusOK {
		t.Fatalf("admin empty-prefix list: status %d, body %s", status, raw)
	}
	decodeInto(t, raw, &resp)
	if len(resp.Objects) != 3 {
		t.Fatalf("want 3 objects overall for admin, got %d", len(resp.Objects))
	}
}

func TestStorageGetRequiresPathOrPrefix(t *testing.T) {
	e := newTestEnv(t, nil)
	w := e.connect("writer", "contributor", "")

	if status, _ := e.do(http.MethodGet, "/api/storage/objects", w.Token, nil); status != http.StatusBadRequest {
		t.Fatalf("neither: status = %d, want 400", status)
	}
	if status, _ := e.do(http.MethodGet, "/api/storage/objects?path=p&prefix=q", w.Token, nil); status != http.StatusBadRequest {
		t.Fatalf("both: status = %d, want 400", status)
	}
}

func TestStorageGetMissingObject(t *testing.T) {
	e := newTestEnv(t, nil)
	w := e.connect("writer", "contributor", "")
	if status, _ := e.do(http.MethodGet, "/api/storage/objects?path=shared/nope.txt", w.Token, nil); status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
}

func TestStorageDelete(t *testing.T) {
	e := newTestEnv(t, nil)
	w := e.connect("writer", "contributor", "")
	e.putObject(w.Token, "shared/tmp.txt", "x", "", http.StatusOK)

	if status, _ := e.do(http.MethodDelete, "/api/storage/objects?path=shared/tmp.txt", w.Token, nil); status != http.StatusNoContent {
		t.Fatalf("delete: status = %d, want 204", status)
	}
	if status, _ := e.do(http.MethodGet, "/api/storage/objects?path=shared/tmp.txt", w.Token, nil); status != http.StatusNotFound {
		t.Fatalf("get after delete: status = %d, want 404", status)
	}
}

func TestStorageScopeBlocksCrossWorkerAccess(t *testing.T) {
	e := newTestEnv(t, nil)
	alice := e.connect("alice", "contributor", "")
	bob := e.connect("bob", "contributor", "")

	meta := e.putObject(alice.Token, "workers/alice/secret.txt", "alice's data", "", http.StatusOK)
	if meta.Path != "workers/alice/secret.txt" {
		t.Fatalf("unexpected meta: %+v", meta)
	}

	if status, raw := e.do(http.MethodGet, "/api/storage/objects?path=workers/alice/secret.txt", bob.Token, nil); status != http.StatusForbidden {
		t.Fatalf("bob GET alice's object: status %d, want 403; body %s", status, raw)
	}
	if status, raw := e.putObjectRaw(bob.Token, "workers/alice/secret.txt", "overwritten by bob", ""); status != http.StatusForbidden {
		t.Fatalf("bob PUT over alice's object: status %d, want 403; body %s", status, raw)
	}
	if status, raw := e.do(http.MethodDelete, "/api/storage/objects?path=workers/alice/secret.txt", bob.Token, nil); status != http.StatusForbidden {
		t.Fatalf("bob DELETE alice's object: status %d, want 403; body %s", status, raw)
	}
	if status, raw := e.do(http.MethodGet, "/api/storage/objects?prefix=workers/alice/", bob.Token, nil); status != http.StatusForbidden {
		t.Fatalf("bob LIST alice's prefix: status %d, want 403; body %s", status, raw)
	}

	if status, raw := e.do(http.MethodGet, "/api/storage/objects?path=workers/alice/secret.txt", alice.Token, nil); status != http.StatusOK || string(raw) != "alice's data" {
		t.Fatalf("alice GET her own object: status %d, body %q", status, raw)
	}
}

func TestStorageSharedNamespaceEnablesHandoff(t *testing.T) {
	e := newTestEnv(t, nil)
	report := e.connect("report", "contributor", "")
	lead := e.connect("lead", "lead", "")

	meta := e.putObject(report.Token, "shared/handoff/deliverable.txt", "the work", "", http.StatusOK)

	status, raw := e.do(http.MethodGet, "/api/storage/objects?path=shared/handoff/deliverable.txt", lead.Token, nil)
	if status != http.StatusOK || string(raw) != "the work" {
		t.Fatalf("lead reading report's shared handoff: status %d, body %q", status, raw)
	}

	e.putObject(lead.Token, "shared/handoff/notes.txt", "looks good", "", http.StatusOK)
	status, raw = e.do(http.MethodGet, "/api/storage/objects?path=shared/handoff/notes.txt", report.Token, nil)
	if status != http.StatusOK || string(raw) != "looks good" {
		t.Fatalf("report reading lead's shared note: status %d, body %q", status, raw)
	}

	e.putObject(report.Token, "workers/report/private.txt", "not for the lead", "", http.StatusOK)
	if status, raw := e.do(http.MethodGet, "/api/storage/objects?path=workers/report/private.txt", lead.Token, nil); status != http.StatusForbidden {
		t.Fatalf("lead GET report's owned-prefix object: status %d, want 403; body %s", status, raw)
	}
	_ = meta
}

func TestAdminBypassesStorageScope(t *testing.T) {
	e := newTestEnv(t, nil)
	w := e.connect("owner", "contributor", "")
	e.putObject(w.Token, "workers/owner/data.txt", "owner's data", "", http.StatusOK)

	admin := e.adminToken("test-admin")
	if status, raw := e.do(http.MethodGet, "/api/storage/objects?path=workers/owner/data.txt", admin, nil); status != http.StatusOK || string(raw) != "owner's data" {
		t.Fatalf("admin GET: status %d, body %q", status, raw)
	}
	e.putObject(admin, "workers/owner/data.txt", "overwritten by admin", "", http.StatusOK)
	if status, _ := e.do(http.MethodDelete, "/api/storage/objects?path=workers/owner/data.txt", admin, nil); status != http.StatusNoContent {
		t.Fatalf("admin DELETE: status %d, want 204", status)
	}
}

func TestConnectRequiresEnrollmentAuth(t *testing.T) {
	e := newTestEnv(t, nil)

	kp, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	nonce := e.challenge("unauthorized-newcomer")
	status, raw := e.do(http.MethodPost, "/api/workers", "", connectRequest{
		WorkerID:  "unauthorized-newcomer",
		Role:      "contributor",
		PublicKey: base64.StdEncoding.EncodeToString(kp.PublicKey),
		Nonce:     base64.StdEncoding.EncodeToString(nonce),
		Signature: base64.StdEncoding.EncodeToString(kp.SignNonce(nonce)),
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("connect with no credential: status %d, want 401; body %s", status, raw)
	}
	if _, err := e.srv.Graph().Get("unauthorized-newcomer"); err == nil {
		t.Fatal("a worker registered with no enrollment credential was added to the org chart")
	}

	someWorker := e.connect("bystander", "contributor", "")
	nonce = e.challenge("still-unauthorized")
	status, raw = e.do(http.MethodPost, "/api/workers", someWorker.Token, connectRequest{
		WorkerID:  "still-unauthorized",
		PublicKey: base64.StdEncoding.EncodeToString(kp.PublicKey),
		Nonce:     base64.StdEncoding.EncodeToString(nonce),
		Signature: base64.StdEncoding.EncodeToString(kp.SignNonce(nonce)),
	})
	if status != http.StatusForbidden {
		t.Fatalf("connect with a non-admin worker token: status %d, want 403; body %s", status, raw)
	}
}

func TestConnectAcceptsLocalEnrollmentToken(t *testing.T) {
	e := newTestEnv(t, nil)
	enrollTok := e.srv.EnrollmentToken()
	if enrollTok == "" {
		t.Fatal("server has no enrollment token configured")
	}

	kp, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	nonce := e.challenge("newcomer")
	status, raw := e.do(http.MethodPost, "/api/workers", enrollTok, connectRequest{
		WorkerID:  "newcomer",
		Role:      "contributor",
		PublicKey: base64.StdEncoding.EncodeToString(kp.PublicKey),
		Nonce:     base64.StdEncoding.EncodeToString(nonce),
		Signature: base64.StdEncoding.EncodeToString(kp.SignNonce(nonce)),
	})
	if status != http.StatusCreated {
		t.Fatalf("connect with local enrollment token: status %d, want 201; body %s", status, raw)
	}
}

func TestConnectRejectsRevokedWorkerID(t *testing.T) {
	e := newTestEnv(t, nil)
	admin := e.adminToken("test-admin")

	status, raw := e.do(http.MethodPost, "/api/workers/never-connected/revoke", admin, revokeRequest{Reason: "pre-emptive"})
	if status != http.StatusOK {
		t.Fatalf("revoke never-registered id: status %d, body %s", status, raw)
	}

	kp, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	nonce := e.challenge("never-connected")
	status, raw = e.do(http.MethodPost, "/api/workers", admin, connectRequest{
		WorkerID:  "never-connected",
		Role:      "contributor",
		PublicKey: base64.StdEncoding.EncodeToString(kp.PublicKey),
		Nonce:     base64.StdEncoding.EncodeToString(nonce),
		Signature: base64.StdEncoding.EncodeToString(kp.SignNonce(nonce)),
	})
	if status != http.StatusForbidden {
		t.Fatalf("connect with a revoked worker id: status %d, want 403; body %s", status, raw)
	}
	if _, err := e.srv.Graph().Get("never-connected"); err == nil {
		t.Fatal("a revoked worker id was still added to the org chart")
	}
}

func TestOpenEnrollmentBypassesAuth(t *testing.T) {
	e := newTestEnv(t, func(c *server.Config) { c.OpenEnrollment = true })

	kp, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	nonce := e.challenge("dev-only")
	status, raw := e.do(http.MethodPost, "/api/workers", "", connectRequest{
		WorkerID:  "dev-only",
		Role:      "contributor",
		PublicKey: base64.StdEncoding.EncodeToString(kp.PublicKey),
		Nonce:     base64.StdEncoding.EncodeToString(nonce),
		Signature: base64.StdEncoding.EncodeToString(kp.SignNonce(nonce)),
	})
	if status != http.StatusCreated {
		t.Fatalf("connect under OpenEnrollment: status %d, want 201; body %s", status, raw)
	}
}

func TestHealthEndpoint(t *testing.T) {
	e := newTestEnv(t, nil)
	status, raw := e.do(http.MethodGet, "/api/health", "", nil)
	if status != http.StatusOK {
		t.Fatalf("status %d, body %s", status, raw)
	}
	var resp map[string]string
	decodeInto(t, raw, &resp)
	if resp["status"] != "ok" {
		t.Fatalf("status = %q", resp["status"])
	}
}

func (e *testEnv) putObject(token, path, content, ifMatch string, wantStatus int) objectMetaView {
	e.t.Helper()

	status, raw := e.putObjectRaw(token, path, content, ifMatch)
	if status != wantStatus {
		e.t.Fatalf("put %q: status %d, want %d; body %s", path, status, wantStatus, raw)
	}
	var meta objectMetaView
	decodeInto(e.t, raw, &meta)
	return meta
}

func (e *testEnv) putObjectRaw(token, path, content, ifMatch string) (int, []byte) {
	e.t.Helper()

	url := fmt.Sprintf("%s/api/storage/objects?path=%s", e.ts.URL, path)
	if ifMatch != "" {
		url += "&if_match_revision=" + ifMatch
	}

	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(content))
	if err != nil {
		e.t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := e.ts.Client().Do(req)
	if err != nil {
		e.t.Fatalf("put: %v", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		e.t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, raw
}

func (e *testEnv) subscribeOne(w testWorker, timeout time.Duration) envelopeView {
	e.t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		e.ts.URL+"/api/messages/subscribe?worker_id="+w.ID, nil)
	if err != nil {
		e.t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+w.Token)

	resp, err := e.ts.Client().Do(req)
	if err != nil {
		e.t.Fatalf("subscribe: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		e.t.Fatalf("subscribe: status %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		e.t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var env envelopeView
		decodeInto(e.t, []byte(strings.TrimPrefix(line, "data: ")), &env)
		return env
	}
	e.t.Fatalf("no SSE event arrived within %s (scanner err: %v)", timeout, scanner.Err())
	return envelopeView{}
}

func assertInboxSize(t *testing.T, e *testEnv, w testWorker, want int) {
	t.Helper()

	status, raw := e.do(http.MethodGet, "/api/messages?worker_id="+w.ID, w.Token, nil)
	if status != http.StatusOK {
		t.Fatalf("tail %q: status %d, body %s", w.ID, status, raw)
	}
	var resp struct {
		Messages []envelopeView `json:"messages"`
	}
	decodeInto(t, raw, &resp)
	if len(resp.Messages) != want {
		t.Fatalf("%q has %d messages, want %d: %+v", w.ID, len(resp.Messages), want, resp.Messages)
	}
}

func TestDecodeJSONRejectsOversizedBody(t *testing.T) {
	e := newTestEnv(t, nil)
	body := map[string]string{"worker_id": strings.Repeat("a", maxJSONBodyBytes)}
	status, raw := e.do(http.MethodPost, "/api/workers/challenge", "", body)
	if status != http.StatusRequestEntityTooLarge {
		t.Fatalf("status %d, want 413; body %s", status, raw)
	}
}

func TestDecodeJSONRejectsUnknownFields(t *testing.T) {
	e := newTestEnv(t, nil)
	body := map[string]string{"worker_id": "w1", "wroker_role": "lead"}
	status, raw := e.do(http.MethodPost, "/api/workers/challenge", "", body)
	if status != http.StatusBadRequest {
		t.Fatalf("status %d, want 400; body %s", status, raw)
	}
}
