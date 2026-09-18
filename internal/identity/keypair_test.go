package identity

import (
	"crypto/ed25519"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateKeyPair(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	if len(kp.PrivateKey) != ed25519.PrivateKeySize {
		t.Errorf("private key size = %d, want %d", len(kp.PrivateKey), ed25519.PrivateKeySize)
	}
	if len(kp.PublicKey) != ed25519.PublicKeySize {
		t.Errorf("public key size = %d, want %d", len(kp.PublicKey), ed25519.PublicKeySize)
	}

	kp2, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair (second): %v", err)
	}
	if kp.PublicKey.Equal(kp2.PublicKey) {
		t.Error("two generated keypairs unexpectedly equal")
	}
}

func TestSaveLoadKeyPairRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "identity.key")

	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	if err := SaveKeyPair(path, kp); err != nil {
		t.Fatalf("SaveKeyPair: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat key file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != KeyFilePerm {
		t.Errorf("key file mode = %04o, want %04o", perm, KeyFilePerm)
	}

	loaded, err := LoadKeyPair(path)
	if err != nil {
		t.Fatalf("LoadKeyPair: %v", err)
	}
	if !loaded.PrivateKey.Equal(kp.PrivateKey) {
		t.Error("loaded private key does not match saved private key")
	}
	if !loaded.PublicKey.Equal(kp.PublicKey) {
		t.Error("loaded public key does not match saved public key")
	}
}

func TestLoadKeyPairRejectsLoosePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "identity.key")

	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	if err := SaveKeyPair(path, kp); err != nil {
		t.Fatalf("SaveKeyPair: %v", err)
	}

	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	if _, err := LoadKeyPair(path); err == nil {
		t.Error("LoadKeyPair succeeded on a 0644 key file, want error")
	}
}

func TestLoadOrGenerateKeyPair(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "identity.key")

	first, err := LoadOrGenerateKeyPair(path)
	if err != nil {
		t.Fatalf("LoadOrGenerateKeyPair (generate): %v", err)
	}

	second, err := LoadOrGenerateKeyPair(path)
	if err != nil {
		t.Fatalf("LoadOrGenerateKeyPair (load): %v", err)
	}

	if !first.PrivateKey.Equal(second.PrivateKey) {
		t.Error("LoadOrGenerateKeyPair generated a new key instead of loading the existing one")
	}
}

func TestSignAndVerifyNonce(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	nonce := []byte("connect-challenge-nonce")

	sig := kp.SignNonce(nonce)
	if !VerifyNonceSignature(kp.PublicKey, nonce, sig) {
		t.Error("VerifyNonceSignature failed for a valid signature")
	}

	if VerifyNonceSignature(kp.PublicKey, []byte("different nonce"), sig) {
		t.Error("VerifyNonceSignature succeeded for a mismatched nonce")
	}

	other, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair (other): %v", err)
	}
	if VerifyNonceSignature(other.PublicKey, nonce, sig) {
		t.Error("VerifyNonceSignature succeeded against the wrong public key")
	}
}
