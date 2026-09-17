// Package security provides the worker-facing half of Robot Dreams'
// identity model: signed, capability-scoped tokens and a client that keeps
// a worker's token fresh in the background. See docs/vision/security.md for the
// full design; the server-side counterpart (minting, JWKS, revocation)
// lives in internal/identity.
package security

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims are the JWT claims Robot Dreams mints for a worker's
// capability-scoped token. Scopes are strings like "message:send" or
// "storage:read:path/*" per docs/vision/security.md.
type Claims struct {
	jwt.RegisteredClaims

	// WorkerID identifies the worker this token was minted for. It is
	// also carried in RegisteredClaims.Subject; WorkerID is provided as a
	// convenience, explicit alias.
	WorkerID string `json:"worker_id"`

	// Scopes lists the capabilities this token grants, e.g.
	// "message:send", "storage:read:path/*".
	Scopes []string `json:"scopes"`

	// DelegatedBy is set on a token a connected worker minted for a
	// short-lived child of its own (see the control plane's
	// POST /api/workers/delegate): the parent's worker ID. The child has
	// no keypair and no org-chart entry; it holds only this token, whose
	// scopes are a subset of the parent's and whose life is bounded by
	// the parent's own token. Empty on every other token.
	DelegatedBy string `json:"delegated_by,omitempty"`
}

// Delegated reports whether the token was minted for a child worker by
// its parent rather than for a registered worker by the control plane.
func (c Claims) Delegated() bool { return c.DelegatedBy != "" }

// HasScope reports whether the claims grant the given scope exactly.
// Prefix-style scopes (e.g. "storage:read:path/*") are matched literally
// here; callers that need wildcard matching should implement it against
// Scopes directly. Kept deliberately simple until a real caller needs
// more.
func (c Claims) HasScope(scope string) bool {
	for _, s := range c.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// Token wraps a signed JWT string along with the metadata a client needs
// to manage its lifecycle without re-parsing the token.
type Token struct {
	// Raw is the signed, encoded JWT (header.payload.signature).
	Raw string

	// ExpiresAt is when the token stops being valid.
	ExpiresAt time.Time

	// IssuedAt is when the token was minted.
	IssuedAt time.Time
}

// Expired reports whether the token is expired as of now.
func (t Token) Expired(now time.Time) bool {
	return !now.Before(t.ExpiresAt)
}

// TTL returns the token's total lifetime (ExpiresAt - IssuedAt).
func (t Token) TTL() time.Duration {
	return t.ExpiresAt.Sub(t.IssuedAt)
}
