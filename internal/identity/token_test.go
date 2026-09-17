package identity

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/acumen-ai-org/robotdreams/pkg/security"
)

func newTestIssuer(t *testing.T, clock Clock) *Issuer {
	t.Helper()
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	return NewIssuer(kp, "test-key-1", clock)
}

func TestMintAndValidateRoundtrip(t *testing.T) {
	clock := newFakeClock(time.Unix(1_700_000_000, 0))
	iss := newTestIssuer(t, clock)

	scopes := []string{"message:send", "storage:read:path/*"}
	tok, err := iss.Mint("worker-1", scopes, time.Minute)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if tok.Raw == "" {
		t.Fatal("Mint returned empty token")
	}

	claims, err := iss.Validate(tok.Raw)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if claims.WorkerID != "worker-1" {
		t.Errorf("WorkerID = %q, want %q", claims.WorkerID, "worker-1")
	}
	if claims.Subject != "worker-1" {
		t.Errorf("Subject = %q, want %q", claims.Subject, "worker-1")
	}
	if len(claims.Scopes) != len(scopes) {
		t.Fatalf("Scopes = %v, want %v", claims.Scopes, scopes)
	}
	for i, s := range scopes {
		if claims.Scopes[i] != s {
			t.Errorf("Scopes[%d] = %q, want %q", i, claims.Scopes[i], s)
		}
	}
	if !claims.HasScope("message:send") {
		t.Error("HasScope(message:send) = false, want true")
	}
	if claims.HasScope("message:delete") {
		t.Error("HasScope(message:delete) = true, want false")
	}
	if claims.ID == "" {
		t.Error("expected non-empty jti claim")
	}
}

func TestValidateRejectsExpiredToken(t *testing.T) {
	clock := newFakeClock(time.Unix(1_700_000_000, 0))
	iss := newTestIssuer(t, clock)

	tok, err := iss.Mint("worker-1", []string{"message:send"}, time.Minute)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}

	clock.Advance(2 * time.Minute)

	if _, err := iss.Validate(tok.Raw); err == nil {
		t.Error("Validate succeeded on an expired token, want error")
	}
}

func TestValidateRejectsTokenWithoutExpiry(t *testing.T) {
	clock := newFakeClock(time.Unix(1_700_000_000, 0))
	iss := newTestIssuer(t, clock)

	claims := security.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:  "worker-1",
			IssuedAt: jwt.NewNumericDate(clock.Now()),
			ID:       "no-exp",
		},
		WorkerID: "worker-1",
		Scopes:   []string{"message:send"},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	tok.Header["kid"] = iss.KeyID()
	raw, err := tok.SignedString(iss.signingKey)
	if err != nil {
		t.Fatalf("sign exp-less token: %v", err)
	}

	_, err = iss.Validate(raw)
	if err == nil {
		t.Fatal("Validate accepted a token with no exp claim, want error")
	}
	if !errors.Is(err, jwt.ErrTokenRequiredClaimMissing) {
		t.Errorf("Validate error = %v, want it to wrap jwt.ErrTokenRequiredClaimMissing", err)
	}
	if _, err := ValidatePublicKey(raw, iss.PublicKey(), clock); err == nil {
		t.Error("ValidatePublicKey accepted a token with no exp claim, want error")
	}
}

func TestValidateRejectsTamperedSignature(t *testing.T) {
	clock := newFakeClock(time.Unix(1_700_000_000, 0))
	iss := newTestIssuer(t, clock)

	tok, err := iss.Mint("worker-1", []string{"message:send"}, time.Minute)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}

	parts := strings.Split(tok.Raw, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 JWT segments, got %d", len(parts))
	}
	payload := []byte(parts[1])
	payload[0] = payload[0] ^ 0x01
	tampered := parts[0] + "." + string(payload) + "." + parts[2]

	if _, err := iss.Validate(tampered); err == nil {
		t.Error("Validate succeeded on a tampered token, want error")
	}
}

func TestValidateRejectsWrongKey(t *testing.T) {
	clock := newFakeClock(time.Unix(1_700_000_000, 0))
	iss := newTestIssuer(t, clock)
	other := newTestIssuer(t, clock)

	tok, err := iss.Mint("worker-1", []string{"message:send"}, time.Minute)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}

	if _, err := other.Validate(tok.Raw); err == nil {
		t.Error("Validate succeeded against the wrong issuer's key, want error")
	}
}
