package identity

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

type JWK struct {
	KeyType string `json:"kty"`
	Curve   string `json:"crv"`
	X       string `json:"x"`
	KeyID   string `json:"kid"`
	Use     string `json:"use,omitempty"`
}

type JWKS struct {
	Keys []JWK `json:"keys"`
}

func NewJWK(pub ed25519.PublicKey, keyID string) JWK {
	return JWK{
		KeyType: "OKP",
		Curve:   "Ed25519",
		X:       base64.RawURLEncoding.EncodeToString(pub),
		KeyID:   keyID,
		Use:     "sig",
	}
}

func (k JWK) PublicKey() (ed25519.PublicKey, error) {
	if k.KeyType != "OKP" || k.Curve != "Ed25519" {
		return nil, fmt.Errorf("identity: unsupported JWK kty/crv %q/%q (want OKP/Ed25519)", k.KeyType, k.Curve)
	}
	raw, err := base64.RawURLEncoding.DecodeString(k.X)
	if err != nil {
		return nil, fmt.Errorf("identity: decode JWK x: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("identity: JWK x has unexpected length %d (want %d)", len(raw), ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(raw), nil
}

func NewJWKS(current JWK, previous ...JWK) JWKS {
	keys := make([]JWK, 0, 1+len(previous))
	keys = append(keys, current)
	keys = append(keys, previous...)
	return JWKS{Keys: keys}
}

func (s JWKS) MarshalJSON() ([]byte, error) {
	type alias JWKS
	return json.Marshal(alias(s))
}

func UnmarshalJWKS(data []byte) (JWKS, error) {
	var s JWKS
	if err := json.Unmarshal(data, &s); err != nil {
		return JWKS{}, fmt.Errorf("identity: unmarshal JWKS: %w", err)
	}
	return s, nil
}

func (s JWKS) KeyByID(kid string) (JWK, bool) {
	for _, k := range s.Keys {
		if k.KeyID == kid {
			return k, true
		}
	}
	return JWK{}, false
}

func ValidateToken(raw string, set JWKS, clock Clock) (Claims, error) {
	kid, _ := jwtKeyID(raw)

	if kid != "" {
		if jwk, ok := set.KeyByID(kid); ok {
			pub, err := jwk.PublicKey()
			if err != nil {
				return Claims{}, err
			}
			return ValidatePublicKey(raw, pub, clock)
		}
	}

	var lastErr error
	for _, jwk := range set.Keys {
		pub, err := jwk.PublicKey()
		if err != nil {
			lastErr = err
			continue
		}
		claims, err := ValidatePublicKey(raw, pub, clock)
		if err == nil {
			return claims, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("identity: no keys in JWKS")
	}
	return Claims{}, fmt.Errorf("identity: token did not validate against any JWKS key: %w", lastErr)
}

func jwtKeyID(raw string) (string, error) {
	parser := jwt.NewParser()
	tok, _, err := parser.ParseUnverified(raw, jwt.MapClaims{})
	if err != nil {
		return "", err
	}
	kid, _ := tok.Header["kid"].(string)
	return kid, nil
}
