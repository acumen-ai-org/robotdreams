package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/acumen-ai-org/robotdreams/pkg/security"
)

var ErrTokenRevoked = errors.New("identity: token's worker has been revoked")

type Clock = security.Clock

type Claims = security.Claims

type RealClock = security.RealClock

type Issuer struct {
	signingKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
	keyID      string
	clock      Clock
}

func NewIssuer(kp KeyPair, keyID string, clock Clock) *Issuer {
	if clock == nil {
		clock = RealClock{}
	}
	return &Issuer{
		signingKey: kp.PrivateKey,
		publicKey:  kp.PublicKey,
		keyID:      keyID,
		clock:      clock,
	}
}

func (iss *Issuer) PublicKey() ed25519.PublicKey { return iss.publicKey }

func (iss *Issuer) KeyID() string { return iss.keyID }

func (iss *Issuer) Mint(workerID string, scopes []string, ttl time.Duration) (security.Token, error) {
	return iss.mint(workerID, "", scopes, ttl)
}

func (iss *Issuer) MintDelegated(workerID, delegatedBy string, scopes []string, ttl time.Duration) (security.Token, error) {
	if delegatedBy == "" {
		return security.Token{}, fmt.Errorf("identity: delegated token needs a parent")
	}
	return iss.mint(workerID, delegatedBy, scopes, ttl)
}

func (iss *Issuer) mint(workerID, delegatedBy string, scopes []string, ttl time.Duration) (security.Token, error) {
	now := iss.clock.Now()
	expiresAt := now.Add(ttl)

	claims := security.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   workerID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			ID:        newJTI(),
		},
		WorkerID:    workerID,
		Scopes:      append([]string(nil), scopes...),
		DelegatedBy: delegatedBy,
	}

	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	tok.Header["kid"] = iss.keyID

	raw, err := tok.SignedString(iss.signingKey)
	if err != nil {
		return security.Token{}, fmt.Errorf("identity: sign token: %w", err)
	}

	return security.Token{
		Raw:       raw,
		IssuedAt:  now,
		ExpiresAt: expiresAt,
	}, nil
}

func (iss *Issuer) Validate(raw string) (security.Claims, error) {
	return ValidatePublicKey(raw, iss.publicKey, iss.clock)
}

func ValidatePublicKey(raw string, pub ed25519.PublicKey, clock Clock) (security.Claims, error) {
	if clock == nil {
		clock = RealClock{}
	}

	var claims security.Claims
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{"EdDSA"}),
		jwt.WithTimeFunc(clock.Now),
		jwt.WithExpirationRequired(),
	)
	tok, err := parser.ParseWithClaims(raw, &claims, func(t *jwt.Token) (interface{}, error) {
		return pub, nil
	})
	if err != nil {
		return security.Claims{}, fmt.Errorf("identity: validate token: %w", err)
	}
	if !tok.Valid {
		return security.Claims{}, fmt.Errorf("identity: token is not valid")
	}
	return claims, nil
}

func newJTI() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("jti-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
