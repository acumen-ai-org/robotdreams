// Package security is the worker-facing half of the identity model: capability-scoped tokens and a client that keeps them fresh.
package security

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims are the JWT claims minted for a worker's capability-scoped token.
type Claims struct {
	jwt.RegisteredClaims

	WorkerID string `json:"worker_id"`

	Scopes []string `json:"scopes"`

	DelegatedBy string `json:"delegated_by,omitempty"`
}

// Delegated reports whether a parent worker minted the token for a child rather than the control plane for a registered worker.
func (c Claims) Delegated() bool { return c.DelegatedBy != "" }

// HasScope reports whether the claims grant the given scope by exact string match.
func (c Claims) HasScope(scope string) bool {
	for _, s := range c.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// Token wraps a signed JWT string with the timestamps a client needs to manage its lifecycle without re-parsing it.
type Token struct {
	Raw string

	ExpiresAt time.Time

	IssuedAt time.Time
}

// Expired reports whether the token is expired as of now.
func (t Token) Expired(now time.Time) bool {
	return !now.Before(t.ExpiresAt)
}

// TTL returns the token's total lifetime.
func (t Token) TTL() time.Duration {
	return t.ExpiresAt.Sub(t.IssuedAt)
}
