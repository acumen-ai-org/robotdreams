package identity

import (
	"testing"
	"time"
)

func TestJWKSMarshalUnmarshalRoundtrip(t *testing.T) {
	kp1, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	kp2, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	set := NewJWKS(NewJWK(kp1.PublicKey, "key-current"), NewJWK(kp2.PublicKey, "key-previous"))

	data, err := set.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	got, err := UnmarshalJWKS(data)
	if err != nil {
		t.Fatalf("UnmarshalJWKS: %v", err)
	}

	if len(got.Keys) != 2 {
		t.Fatalf("got %d keys, want 2", len(got.Keys))
	}

	jwk, ok := got.KeyByID("key-current")
	if !ok {
		t.Fatal("KeyByID(key-current) not found after roundtrip")
	}
	pub, err := jwk.PublicKey()
	if err != nil {
		t.Fatalf("PublicKey: %v", err)
	}
	if !pub.Equal(kp1.PublicKey) {
		t.Error("roundtripped public key does not match original")
	}
}

func TestValidateTokenAgainstJWKS(t *testing.T) {
	clock := newFakeClock(time.Unix(1_700_000_000, 0))

	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	iss := NewIssuer(kp, "the-key", clock)

	tok, err := iss.Mint("worker-1", []string{"message:send"}, time.Minute)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}

	set := NewJWKS(NewJWK(kp.PublicKey, "the-key"))

	claims, err := ValidateToken(tok.Raw, set, clock)
	if err != nil {
		t.Fatalf("ValidateToken with correct key: %v", err)
	}
	if claims.WorkerID != "worker-1" {
		t.Errorf("WorkerID = %q, want worker-1", claims.WorkerID)
	}

	wrongKP, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	wrongSet := NewJWKS(NewJWK(wrongKP.PublicKey, "the-key"))
	if _, err := ValidateToken(tok.Raw, wrongSet, clock); err == nil {
		t.Error("ValidateToken succeeded against a JWKS with the wrong key, want error")
	}
}

func TestValidateTokenFallsBackWithoutKid(t *testing.T) {
	clock := newFakeClock(time.Unix(1_700_000_000, 0))

	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	iss := NewIssuer(kp, "the-key", clock)
	tok, err := iss.Mint("worker-1", []string{"message:send"}, time.Minute)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}

	decoyKP, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	set := NewJWKS(NewJWK(decoyKP.PublicKey, "other-key"), NewJWK(kp.PublicKey, "the-key-renamed"))

	if _, err := ValidateToken(tok.Raw, set, clock); err != nil {
		t.Fatalf("ValidateToken fallback: %v", err)
	}
}
