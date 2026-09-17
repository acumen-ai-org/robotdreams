package api

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/acumen-ai-org/robotdreams/internal/identity"
	"github.com/acumen-ai-org/robotdreams/internal/orgchart"
	"github.com/acumen-ai-org/robotdreams/internal/server"
	"github.com/acumen-ai-org/robotdreams/pkg/security"
)

const (
	challengeTTL         = 2 * time.Minute
	nonceSize            = 32
	revocationCacheTTL   = 60 * time.Second
	maxPendingChallenges = 10_000
	maxJSONBodyBytes     = 1 << 20
)

type API struct {
	srv        *server.Server
	clock      security.Clock
	challenges *challengeStore
	revCache   *revocationCache
	events     *eventHub
}

func New(srv *server.Server) *API {
	clock := srv.Clock()
	pollState := newServerPollState(&serverDeps{
		Graph: srv.Graph,
		ListObjs: func(ctx context.Context, prefix string) ([]objectMetaView, error) {
			metas, err := srv.Storage().List(ctx, prefix)
			if err != nil {
				return nil, err
			}
			out := make([]objectMetaView, 0, len(metas))
			for _, m := range metas {
				out = append(out, newObjectMetaView(m))
			}
			return out, nil
		},
		TailMsgs: srv.Messaging().Tail,
	})
	return &API{
		srv:        srv,
		clock:      clock,
		challenges: newChallengeStore(clock),
		revCache:   newRevocationCache(srv.Revocations(), clock),
		events:     newEventHub(pollState.poll),
	}
}

type challenge struct {
	nonce     []byte
	expiresAt time.Time
}

type challengeStore struct {
	clock security.Clock

	mu      sync.Mutex
	pending map[string]challenge
}

func newChallengeStore(clock security.Clock) *challengeStore {
	return &challengeStore{clock: clock, pending: make(map[string]challenge)}
}

func (c *challengeStore) issue(workerID string) []byte {
	nonce := randomBytes(nonceSize)

	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.clock.Now()
	c.pending[workerID] = challenge{
		nonce:     nonce,
		expiresAt: now.Add(challengeTTL),
	}
	c.sweepExpiredLocked(now)
	c.evictNearestExpiryLocked()
	return nonce
}

func (c *challengeStore) sweepExpiredLocked(now time.Time) {
	for id, ch := range c.pending {
		if now.After(ch.expiresAt) {
			delete(c.pending, id)
		}
	}
}

func (c *challengeStore) evictNearestExpiryLocked() {
	for len(c.pending) > maxPendingChallenges {
		oldestID, oldest := "", time.Time{}
		for id, ch := range c.pending {
			if oldest.IsZero() || ch.expiresAt.Before(oldest) {
				oldestID, oldest = id, ch.expiresAt
			}
		}
		delete(c.pending, oldestID)
	}
}

func (c *challengeStore) size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.pending)
}

func (c *challengeStore) consume(workerID string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ch, ok := c.pending[workerID]
	if !ok {
		return nil, false
	}
	delete(c.pending, workerID)
	if c.clock.Now().After(ch.expiresAt) {
		return nil, false
	}
	return ch.nonce, true
}

type revocationEntry struct {
	revoked   bool
	checkedAt time.Time
}

type revocationCache struct {
	store identity.RevocationStore
	clock security.Clock

	mu      sync.Mutex
	entries map[string]revocationEntry
}

func newRevocationCache(store identity.RevocationStore, clock security.Clock) *revocationCache {
	return &revocationCache{
		store:   store,
		clock:   clock,
		entries: make(map[string]revocationEntry),
	}
}

func (c *revocationCache) isRevoked(ctx context.Context, workerID string) (bool, error) {
	now := c.clock.Now()

	c.mu.Lock()
	entry, ok := c.entries[workerID]
	c.mu.Unlock()

	if ok && now.Sub(entry.checkedAt) < revocationCacheTTL {
		return entry.revoked, nil
	}

	revoked, err := c.store.IsRevoked(ctx, workerID)
	if err != nil {
		return false, err
	}

	c.mu.Lock()
	c.entries[workerID] = revocationEntry{revoked: revoked, checkedAt: now}
	c.mu.Unlock()
	return revoked, nil
}

func (c *revocationCache) invalidate(workerID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, workerID)
}

type contextKey struct{ name string }

var claimsContextKey = &contextKey{"api.claims"}

func ClaimsFromContext(ctx context.Context) (security.Claims, bool) {
	claims, ok := ctx.Value(claimsContextKey).(security.Claims)
	return claims, ok
}

func (a *API) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, ok := bearerToken(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing or malformed Authorization: Bearer header")
			return
		}

		claims, err := a.srv.Issuer().Validate(raw)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}

		revoked, err := a.revCache.isRevoked(r.Context(), claims.WorkerID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "revocation check failed")
			return
		}
		if revoked {
			writeError(w, http.StatusUnauthorized, "worker has been revoked")
			return
		}
		if claims.Delegated() {
			revoked, err := a.revCache.isRevoked(r.Context(), claims.DelegatedBy)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "revocation check failed")
				return
			}
			if revoked {
				writeError(w, http.StatusUnauthorized, "the delegating worker has been revoked")
				return
			}
		}

		ctx := context.WithValue(r.Context(), claimsContextKey, claims)
		next(w, r.WithContext(ctx))
	}
}

func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	return strings.TrimSpace(h[len(prefix):]), true
}

func isAdmin(claims security.Claims) bool {
	return claims.HasScope(server.AdminScope)
}

func hasStorageScope(claims security.Claims, verb, path string) bool {
	if isAdmin(claims) {
		return true
	}
	scopePrefix := "storage:" + verb + ":"
	for _, s := range claims.Scopes {
		pattern, ok := strings.CutPrefix(s, scopePrefix)
		if !ok {
			continue
		}
		if matchesStoragePattern(pattern, path) {
			return true
		}
	}
	return false
}

func matchesStoragePattern(pattern, path string) bool {
	if base, ok := strings.CutSuffix(pattern, "*"); ok {
		return strings.HasPrefix(path, base)
	}
	return pattern == path
}

func (a *API) requireEnrollmentAuth(w http.ResponseWriter, r *http.Request) bool {
	if a.srv.OpenEnrollment() {
		return true
	}

	raw, ok := bearerToken(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "worker registration requires an admin or enrollment bearer token")
		return false
	}

	if claims, err := a.srv.Issuer().Validate(raw); err == nil {
		if isAdmin(claims) {
			return true
		}
		writeError(w, http.StatusForbidden, "token does not carry the admin scope")
		return false
	}

	enrollTok := a.srv.EnrollmentToken()
	if enrollTok != "" && constantTimeEqual([]byte(raw), []byte(enrollTok)) {
		return true
	}

	writeError(w, http.StatusUnauthorized, "invalid admin or enrollment token")
	return false
}

func constantTimeEqual(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}

type challengeRequest struct {
	WorkerID string `json:"worker_id"`
}

type challengeResponse struct {
	Nonce string `json:"nonce"`
}

func (a *API) handleChallenge(w http.ResponseWriter, r *http.Request) {
	var req challengeRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.WorkerID == "" {
		writeError(w, http.StatusBadRequest, "worker_id is required")
		return
	}
	if len(req.WorkerID) > server.MaxWorkerIDLen {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("worker_id exceeds %d characters", server.MaxWorkerIDLen))
		return
	}

	nonce := a.challenges.issue(req.WorkerID)
	writeJSON(w, http.StatusOK, challengeResponse{
		Nonce: base64.StdEncoding.EncodeToString(nonce),
	})
}

type connectRequest struct {
	WorkerID  string            `json:"worker_id"`
	Role      string            `json:"role"`
	ReportsTo string            `json:"reports_to"`
	PublicKey string            `json:"public_key"`
	Nonce     string            `json:"nonce"`
	Signature string            `json:"signature"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type tokenResponse struct {
	WorkerID  string    `json:"worker_id"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (a *API) handleConnect(w http.ResponseWriter, r *http.Request) {
	if !a.requireEnrollmentAuth(w, r) {
		return
	}

	var req connectRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := server.ValidateWorkerID(req.WorkerID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	pub, err := decodePublicKey(req.PublicKey)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if !a.verifyChallenge(req.WorkerID, req.Nonce, req.Signature, pub) {
		writeError(w, http.StatusUnauthorized, "challenge verification failed")
		return
	}

	revoked, err := a.revCache.isRevoked(r.Context(), req.WorkerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "revocation check failed")
		return
	}
	if revoked {
		writeError(w, http.StatusForbidden, "worker id has been revoked")
		return
	}

	now := a.clock.Now()
	worker := orgchart.Worker{
		ID:          req.WorkerID,
		Role:        req.Role,
		ReportsTo:   req.ReportsTo,
		PublicKey:   pub,
		ConnectedAt: now,
		LastSeenAt:  now,
		Status:      orgchart.StatusConnected,
		Metadata:    req.Metadata,
	}

	if err := a.srv.ConnectWorker(r.Context(), worker); err != nil {
		switch {
		case errors.Is(err, orgchart.ErrDuplicateWorker):
			writeError(w, http.StatusConflict, "worker is already connected")
		case errors.Is(err, orgchart.ErrParentNotFound):
			writeError(w, http.StatusBadRequest, fmt.Sprintf("reports_to %q is not a registered worker", req.ReportsTo))
		case errors.Is(err, orgchart.ErrInvalidWorker):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "could not register worker")
		}
		return
	}

	tok, err := a.srv.MintWorkerToken(req.WorkerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not mint token")
		return
	}

	writeJSON(w, http.StatusCreated, tokenResponse{
		WorkerID:  req.WorkerID,
		Token:     tok.Raw,
		ExpiresAt: tok.ExpiresAt,
	})
}

type refreshRequest struct {
	WorkerID  string `json:"worker_id"`
	Nonce     string `json:"nonce"`
	Signature string `json:"signature"`
}

func (a *API) handleToken(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.WorkerID == "" {
		writeError(w, http.StatusBadRequest, "worker_id is required")
		return
	}

	worker, err := a.srv.Graph().Get(req.WorkerID)
	if err != nil {
		a.challenges.consume(req.WorkerID)
		writeError(w, http.StatusUnauthorized, "challenge verification failed")
		return
	}
	if len(worker.PublicKey) != ed25519.PublicKeySize {
		a.challenges.consume(req.WorkerID)
		writeError(w, http.StatusUnauthorized, "worker has no usable registered public key")
		return
	}

	if !a.verifyChallenge(req.WorkerID, req.Nonce, req.Signature, ed25519.PublicKey(worker.PublicKey)) {
		writeError(w, http.StatusUnauthorized, "challenge verification failed")
		return
	}

	revoked, err := a.revCache.isRevoked(r.Context(), req.WorkerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "revocation check failed")
		return
	}
	if revoked {
		writeError(w, http.StatusForbidden, "worker has been revoked")
		return
	}

	if err := a.srv.TouchWorker(r.Context(), req.WorkerID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not record worker activity")
		return
	}

	tok, err := a.srv.MintWorkerToken(req.WorkerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not mint token")
		return
	}

	writeJSON(w, http.StatusOK, tokenResponse{
		WorkerID:  req.WorkerID,
		Token:     tok.Raw,
		ExpiresAt: tok.ExpiresAt,
	})
}

func (a *API) verifyChallenge(workerID, nonceB64, signatureB64 string, pub ed25519.PublicKey) bool {
	issued, ok := a.challenges.consume(workerID)
	if !ok {
		return false
	}

	nonce, err := base64.StdEncoding.DecodeString(nonceB64)
	if err != nil || !constantTimeEqual(nonce, issued) {
		return false
	}

	sig, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil {
		return false
	}
	return identity.VerifyNonceSignature(pub, issued, sig)
}

func (a *API) handleJWKS(w http.ResponseWriter, r *http.Request) {
	iss := a.srv.Issuer()
	set := identity.NewJWKS(identity.NewJWK(iss.PublicKey(), iss.KeyID()))

	b, err := json.Marshal(set)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not encode JWKS")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}

func decodePublicKey(b64 string) (ed25519.PublicKey, error) {
	if b64 == "" {
		return nil, errors.New("public_key is required")
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, errors.New("public_key is not valid base64")
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("public_key must be %d bytes, got %d", ed25519.PublicKeySize, len(raw))
	}
	return ed25519.PublicKey(raw), nil
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("api: crypto/rand failed: %v", err))
	}
	return b
}

type errorResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("request body exceeds %d bytes", maxJSONBodyBytes))
			return false
		}
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON body: %v", err))
		return false
	}
	return true
}

func decodeOptionalJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if r.ContentLength == 0 {
		return true
	}
	return decodeJSON(w, r, dst)
}
