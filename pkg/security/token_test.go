package security

import (
	"testing"
	"time"
)

func TestClaimsHasScope(t *testing.T) {
	c := Claims{Scopes: []string{"message:send", "storage:read:path/*"}}

	if !c.HasScope("message:send") {
		t.Error("HasScope(message:send) = false, want true")
	}
	if c.HasScope("message:receive") {
		t.Error("HasScope(message:receive) = true, want false")
	}
}

func TestTokenExpired(t *testing.T) {
	issued := time.Unix(1_700_000_000, 0)
	tok := Token{
		IssuedAt:  issued,
		ExpiresAt: issued.Add(time.Minute),
	}

	if tok.Expired(issued) {
		t.Error("token reported expired at issuance time")
	}
	if tok.Expired(issued.Add(30 * time.Second)) {
		t.Error("token reported expired before its TTL elapsed")
	}
	if !tok.Expired(issued.Add(time.Minute)) {
		t.Error("token reported not expired exactly at ExpiresAt")
	}
	if !tok.Expired(issued.Add(2 * time.Minute)) {
		t.Error("token reported not expired well past ExpiresAt")
	}
}

func TestTokenTTL(t *testing.T) {
	issued := time.Unix(1_700_000_000, 0)
	tok := Token{IssuedAt: issued, ExpiresAt: issued.Add(90 * time.Second)}
	if got := tok.TTL(); got != 90*time.Second {
		t.Errorf("TTL() = %v, want %v", got, 90*time.Second)
	}
}
