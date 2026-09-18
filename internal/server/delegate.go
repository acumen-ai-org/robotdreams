package server

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/acumen-ai-org/robotdreams/pkg/security"
)

const DelegationSeparator = "~"

const MaxDelegatedChildName = 64

var childNameRe = workerIDRe

var ErrCannotDelegate = errors.New("server: delegated tokens cannot delegate further")

var ErrInvalidDelegation = errors.New("server: invalid delegation")

func DelegatedWorkerID(parentID, child string) string {
	return parentID + DelegationSeparator + child
}

func DelegationParent(workerID string) (parent, child string, ok bool) {
	parent, child, ok = strings.Cut(workerID, DelegationSeparator)
	if !ok || parent == "" || child == "" {
		return "", "", false
	}
	return parent, child, true
}

type DelegationRequest struct {
	Child string

	Scopes []string

	TTL time.Duration
}

type Delegation struct {
	WorkerID    string
	DelegatedBy string
	Scopes      []string
	Token       security.Token
}

func (s *Server) MintDelegatedToken(parent security.Claims, req DelegationRequest) (Delegation, error) {
	if parent.Delegated() {
		return Delegation{}, ErrCannotDelegate
	}
	if parent.WorkerID == "" {
		return Delegation{}, fmt.Errorf("%w: caller has no worker id", ErrInvalidDelegation)
	}

	child := req.Child
	if child == "" {
		child = randomChildName()
	}
	if len(child) > MaxDelegatedChildName || !childNameRe.MatchString(child) {
		return Delegation{}, fmt.Errorf("%w: child name %q must be [A-Za-z0-9._-]+ and at most %d characters", ErrInvalidDelegation, child, MaxDelegatedChildName)
	}
	childID := DelegatedWorkerID(parent.WorkerID, child)

	scopes, err := narrowScopes(parent.Scopes, req.Scopes)
	if err != nil {
		return Delegation{}, err
	}

	ttl := req.TTL
	if ttl <= 0 {
		ttl = s.cfg.DelegatedTokenTTL
	}
	if ttl > s.cfg.DelegatedTokenTTL {
		return Delegation{}, fmt.Errorf("%w: ttl %s exceeds the maximum %s", ErrInvalidDelegation, ttl, s.cfg.DelegatedTokenTTL)
	}

	if parent.ExpiresAt != nil {
		if left := parent.ExpiresAt.Sub(s.clock.Now()); left < ttl {
			ttl = left
		}
	}
	if ttl <= 0 {
		return Delegation{}, fmt.Errorf("%w: the parent token has expired", ErrInvalidDelegation)
	}

	tok, err := s.issuer.MintDelegated(childID, parent.WorkerID, scopes, ttl)
	if err != nil {
		return Delegation{}, err
	}
	return Delegation{WorkerID: childID, DelegatedBy: parent.WorkerID, Scopes: scopes, Token: tok}, nil
}

func narrowScopes(parent, want []string) ([]string, error) {
	if len(want) == 0 {
		out := make([]string, 0, len(parent))
		for _, s := range parent {
			if s != AdminScope {
				out = append(out, s)
			}
		}
		return out, nil
	}
	out := make([]string, 0, len(want))
	for _, w := range want {
		if w == AdminScope {
			return nil, fmt.Errorf("%w: the admin scope cannot be delegated", ErrInvalidDelegation)
		}
		if !scopeCovered(parent, w) {
			return nil, fmt.Errorf("%w: scope %q is not held by the parent", ErrInvalidDelegation, w)
		}
		out = append(out, w)
	}
	return out, nil
}

func scopeCovered(held []string, want string) bool {
	for _, h := range held {
		if h == want {
			return true
		}
		hv, hp, ok := cutStorageScope(h)
		if !ok {
			continue
		}
		wv, wp, ok := cutStorageScope(want)
		if !ok || wv != hv {
			continue
		}
		base, wild := strings.CutSuffix(hp, "*")
		if !wild {
			continue
		}
		if strings.HasPrefix(strings.TrimSuffix(wp, "*"), base) {
			return true
		}
	}
	return false
}

func cutStorageScope(scope string) (verb, pattern string, ok bool) {
	rest, ok := strings.CutPrefix(scope, "storage:")
	if !ok {
		return "", "", false
	}
	verb, pattern, ok = strings.Cut(rest, ":")
	if !ok || verb == "" || pattern == "" {
		return "", "", false
	}
	return verb, pattern, true
}

func randomChildName() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("server: crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}
