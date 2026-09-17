package integration

import (
	"context"
	"encoding/base64"
	"net/http"
	"testing"

	"github.com/acumen-ai-org/robotdreams/internal/identity"
	"github.com/acumen-ai-org/robotdreams/internal/server"
)

func TestStorageScopeEnforcementOverHTTP(t *testing.T) {
	ctx := context.Background()
	ts := startServer(t, server.Config{})

	alice := connectWorker(ctx, t, ts, "alice", "contributor", "", nil)
	bob := connectWorker(ctx, t, ts, "bob", "contributor", "", nil)

	if _, err := putObject(ctx, alice, "workers/alice/secret.txt", []byte("alice's data"), "", ""); err != nil {
		t.Fatalf("alice put her own object: %v", err)
	}

	if _, _, err := getObject(ctx, bob, "workers/alice/secret.txt"); err == nil {
		t.Fatal("bob could read alice's owned object")
	} else if ae := asAPIErrT(t, err); ae.Status != http.StatusForbidden {
		t.Errorf("bob GET alice's object: status = %d, want 403", ae.Status)
	}
	if _, err := putObject(ctx, bob, "workers/alice/secret.txt", []byte("overwritten"), "", ""); err == nil {
		t.Fatal("bob could overwrite alice's owned object")
	} else if ae := asAPIErrT(t, err); ae.Status != http.StatusForbidden {
		t.Errorf("bob PUT over alice's object: status = %d, want 403", ae.Status)
	}
	if err := deleteObject(ctx, bob, "workers/alice/secret.txt"); err == nil {
		t.Fatal("bob could delete alice's owned object (the most acute part of the finding)")
	} else if ae := asAPIErrT(t, err); ae.Status != http.StatusForbidden {
		t.Errorf("bob DELETE alice's object: status = %d, want 403", ae.Status)
	}

	gotBytes, _, err := getObject(ctx, alice, "workers/alice/secret.txt")
	if err != nil || string(gotBytes) != "alice's data" {
		t.Fatalf("alice GET her own object: bytes=%q err=%v", gotBytes, err)
	}

	if _, err := putObject(ctx, bob, "shared/handoff/from-bob.txt", []byte("for alice"), "", ""); err != nil {
		t.Fatalf("bob put under shared/: %v", err)
	}
	gotBytes, _, err = getObject(ctx, alice, "shared/handoff/from-bob.txt")
	if err != nil || string(gotBytes) != "for alice" {
		t.Fatalf("alice reading bob's shared handoff: bytes=%q err=%v", gotBytes, err)
	}

	admin := newAdminActor(t, ts)
	adminGot, _, err := getObject(ctx, admin, "workers/alice/secret.txt")
	if err != nil || string(adminGot) != "alice's data" {
		t.Fatalf("admin GET alice's object: bytes=%q err=%v", adminGot, err)
	}
}

func TestEnrollmentAuthorizationOverHTTP(t *testing.T) {
	ctx := context.Background()
	ts := startServer(t, server.Config{})
	hc := httpClient()

	kp, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}
	nonce, nonceB64, err := requestChallenge(ctx, hc, ts.BaseURL, "unauthorized")
	if err != nil {
		t.Fatalf("challenge: %v", err)
	}
	var out tokenReply
	err = postJSON(ctx, hc, ts.BaseURL+"/api/workers", map[string]any{
		"worker_id":  "unauthorized",
		"public_key": base64.StdEncoding.EncodeToString(kp.PublicKey),
		"nonce":      nonceB64,
		"signature":  base64.StdEncoding.EncodeToString(kp.SignNonce(nonce)),
	}, &out)
	if err == nil {
		t.Fatal("connect with no enrollment credential succeeded")
	}
	if ae := asAPIErrT(t, err); ae.Status != http.StatusUnauthorized {
		t.Errorf("no-credential connect: status = %d, want 401", ae.Status)
	}

	admin := newAdminActor(t, ts)
	if err := revokeWorker(ctx, admin, "pre-revoked", "squatting test"); err != nil {
		t.Fatalf("pre-emptive revoke: %v", err)
	}
	nonce, nonceB64, err = requestChallenge(ctx, hc, ts.BaseURL, "pre-revoked")
	if err != nil {
		t.Fatalf("challenge: %v", err)
	}
	err = postJSONWithToken(ctx, hc, ts.BaseURL+"/api/workers", admin.tok, map[string]any{
		"worker_id":  "pre-revoked",
		"public_key": base64.StdEncoding.EncodeToString(kp.PublicKey),
		"nonce":      nonceB64,
		"signature":  base64.StdEncoding.EncodeToString(kp.SignNonce(nonce)),
	}, &out)
	if err == nil {
		t.Fatal("a revoked worker id was allowed to register")
	}
	if ae := asAPIErrT(t, err); ae.Status != http.StatusForbidden {
		t.Errorf("revoked-id connect: status = %d, want 403", ae.Status)
	}

	w := connectWorker(ctx, t, ts, "legitimate", "contributor", "", nil)
	if w.token() == "" {
		t.Fatal("legitimate connect returned no usable token")
	}
}

func deleteObject(ctx context.Context, w *worker, path string) error {
	resp, err := authedRequest(ctx, w.hc, w.baseURL, w.token(), http.MethodDelete, "/api/storage/objects?path="+urlQueryEscape(path), "", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &apiErr{Status: resp.StatusCode}
	}
	return nil
}

func asAPIErrT(t *testing.T, err error) *apiErr {
	t.Helper()
	var ae *apiErr
	if !asAPIErr(err, &ae) {
		t.Fatalf("got %v (%T), want an *apiErr", err, err)
	}
	return ae
}
