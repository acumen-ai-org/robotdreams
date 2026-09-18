package api

import (
	"encoding/base64"
	"net/http"
	"os"
	"testing"

	"github.com/acumen-ai-org/robotdreams/internal/identity"
	"github.com/acumen-ai-org/robotdreams/internal/server"
)

func (e *testEnv) connectWithEnrollmentToken(workerID, tok string) (int, []byte) {
	e.t.Helper()
	kp, err := identity.GenerateKeyPair()
	if err != nil {
		e.t.Fatalf("GenerateKeyPair: %v", err)
	}
	nonce := e.challenge(workerID)
	return e.do(http.MethodPost, "/api/workers", tok, connectRequest{
		WorkerID:  workerID,
		Role:      "contributor",
		PublicKey: base64.StdEncoding.EncodeToString(kp.PublicKey),
		Nonce:     base64.StdEncoding.EncodeToString(nonce),
		Signature: base64.StdEncoding.EncodeToString(kp.SignNonce(nonce)),
	})
}

func TestEnrollmentTokenRotationTakesEffectImmediately(t *testing.T) {
	e := newTestEnv(t, nil)
	old := e.srv.EnrollmentToken()
	if old == "" {
		t.Fatal("server has no enrollment token configured")
	}

	if status, raw := e.connectWithEnrollmentToken("before-rotation", old); status != http.StatusCreated {
		t.Fatalf("connect with the current enrollment token: status %d, want 201; body %s", status, raw)
	}

	fresh, err := server.RotateEnrollmentToken(e.srv.EnrollmentTokenPath())
	if err != nil {
		t.Fatalf("RotateEnrollmentToken: %v", err)
	}
	if fresh == old {
		t.Fatal("rotation returned the same token")
	}

	if status, raw := e.connectWithEnrollmentToken("stale-credential", old); status != http.StatusUnauthorized {
		t.Fatalf("connect with the ROTATED-OUT token: status %d, want 401; body %s", status, raw)
	}
	if _, err := e.srv.Graph().Get("stale-credential"); err == nil {
		t.Fatal("a worker registered with the rotated-out enrollment token")
	}
	if status, raw := e.connectWithEnrollmentToken("after-rotation", fresh); status != http.StatusCreated {
		t.Fatalf("connect with the new enrollment token: status %d, want 201; body %s", status, raw)
	}

	if err := os.Remove(e.srv.EnrollmentTokenPath()); err != nil {
		t.Fatalf("remove enrollment token: %v", err)
	}
	if status, raw := e.connectWithEnrollmentToken("no-file", fresh); status != http.StatusUnauthorized {
		t.Fatalf("connect with the file removed: status %d, want 401; body %s", status, raw)
	}
	if status, raw := e.connectWithEnrollmentToken("no-file", ""); status != http.StatusUnauthorized {
		t.Fatalf("connect with an empty bearer and the file removed: status %d, want 401; body %s", status, raw)
	}
	if status, raw := e.connectWithEnrollmentToken("admin-still-works", e.adminToken("op")); status != http.StatusCreated {
		t.Fatalf("connect with an admin token and the file removed: status %d, want 201; body %s", status, raw)
	}
}
