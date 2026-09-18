package api

import (
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/acumen-ai-org/robotdreams/internal/identity"
	"github.com/acumen-ai-org/robotdreams/internal/server"
)

func (e *testEnv) tryConnect(workerID string) (int, []byte) {
	e.t.Helper()
	kp, err := identity.GenerateKeyPair()
	if err != nil {
		e.t.Fatalf("GenerateKeyPair: %v", err)
	}
	status, raw := e.do(http.MethodPost, "/api/workers/challenge", "", challengeRequest{WorkerID: workerID})
	if status != http.StatusOK {
		return status, raw
	}
	var ch challengeResponse
	decodeInto(e.t, raw, &ch)
	nonce, err := base64.StdEncoding.DecodeString(ch.Nonce)
	if err != nil {
		e.t.Fatalf("decode nonce: %v", err)
	}
	return e.do(http.MethodPost, "/api/workers", e.adminToken("test-enroller"), connectRequest{
		WorkerID:  workerID,
		Role:      "worker",
		PublicKey: base64.StdEncoding.EncodeToString(kp.PublicKey),
		Nonce:     base64.StdEncoding.EncodeToString(nonce),
		Signature: base64.StdEncoding.EncodeToString(kp.SignNonce(nonce)),
	})
}

func TestConnectRejectsMalformedWorkerIDs(t *testing.T) {
	e := newTestEnv(t, nil)

	rejected := map[string]string{
		"slash":         "a/b",
		"leading slash": "/a",
		"space":         "a b",
		"leading space": " a",
		"tab":           "a\tb",
		"newline":       "a\nb",
		"control":       "a\x01b",
		"tilde":         "a~b",
		"leading dash":  "-a",
		"leading dot":   ".a",
		"unicode":       "wörker",
		"too long":      strings.Repeat("a", server.MaxWorkerIDLen+1),
	}
	for name, id := range rejected {
		status, body := e.tryConnect(id)
		if status != http.StatusBadRequest {
			t.Errorf("%s (%q): status %d, want 400; body %s", name, id, status, body)
		}
	}

	accepted := []string{
		"music-playback-01",
		"host.example.com",
		"my_host",
		"a",
		"7up",
		strings.Repeat("a", server.MaxWorkerIDLen),
	}
	for _, id := range accepted {
		status, body := e.tryConnect(id)
		if status != http.StatusCreated {
			t.Errorf("%q: status %d, want 201; body %s", id, status, body)
		}
	}
}

func TestChallengeBoundsWorkerIDLength(t *testing.T) {
	e := newTestEnv(t, nil)

	long := strings.Repeat("x", server.MaxWorkerIDLen+1)
	status, body := e.do(http.MethodPost, "/api/workers/challenge", "", challengeRequest{WorkerID: long})
	if status != http.StatusBadRequest {
		t.Fatalf("challenge with %d-char id: status %d, want 400; body %s", len(long), status, body)
	}
	if e.api.challenges.size() != 0 {
		t.Fatalf("challenge store holds %d entries after a refused request, want 0", e.api.challenges.size())
	}

	ok := strings.Repeat("x", server.MaxWorkerIDLen)
	if status, body := e.do(http.MethodPost, "/api/workers/challenge", "", challengeRequest{WorkerID: ok}); status != http.StatusOK {
		t.Fatalf("challenge with %d-char id: status %d, want 200; body %s", len(ok), status, body)
	}
}

func TestChallengeStoreCapsPendingEntries(t *testing.T) {
	clock := newFakeClock(time.Unix(1_700_000_000, 0))
	c := newChallengeStore(clock)

	const extra = 5
	for i := 0; i < maxPendingChallenges+extra; i++ {
		c.issue("flood-" + strconv.Itoa(i))
		clock.Advance(time.Millisecond)
	}
	if got := c.size(); got != maxPendingChallenges {
		t.Fatalf("store size = %d, want exactly %d", got, maxPendingChallenges)
	}
	for i := 0; i < extra; i++ {
		if _, ok := c.consume("flood-" + strconv.Itoa(i)); ok {
			t.Errorf("entry %d (among the oldest) still present, want evicted", i)
		}
	}
	newest := "flood-" + strconv.Itoa(maxPendingChallenges+extra-1)
	if _, ok := c.consume(newest); !ok {
		t.Errorf("newest entry %s was evicted, want kept", newest)
	}
}
