package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
)

const KeyFilePerm = 0o600

type KeyPair struct {
	PrivateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
}

func GenerateKeyPair() (KeyPair, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return KeyPair{}, fmt.Errorf("identity: generate keypair: %w", err)
	}
	return KeyPair{PrivateKey: priv, PublicKey: pub}, nil
}

func DefaultKeyPath(serverID string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("identity: resolve home directory: %w", err)
	}
	return filepath.Join(home, ".dream", serverID, "identity.key"), nil
}

func SaveKeyPair(path string, kp KeyPair) (err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("identity: create key directory: %w", err)
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, KeyFilePerm)
	if err != nil {
		return fmt.Errorf("identity: open key file: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("identity: close key file: %w", cerr)
		}
	}()
	if err := f.Chmod(KeyFilePerm); err != nil {
		return fmt.Errorf("identity: chmod key file: %w", err)
	}

	if _, err := f.Write(kp.PrivateKey); err != nil {
		return fmt.Errorf("identity: write key file: %w", err)
	}

	return nil
}

func LoadKeyPair(path string) (KeyPair, error) {
	info, err := os.Stat(path)
	if err != nil {
		return KeyPair{}, fmt.Errorf("identity: stat key file: %w", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return KeyPair{}, fmt.Errorf("identity: key file %s has overly permissive mode %04o (want 0600)", path, info.Mode().Perm())
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return KeyPair{}, fmt.Errorf("identity: read key file: %w", err)
	}
	if len(raw) != ed25519.PrivateKeySize {
		return KeyPair{}, fmt.Errorf("identity: key file %s has unexpected size %d (want %d)", path, len(raw), ed25519.PrivateKeySize)
	}

	priv := ed25519.PrivateKey(raw)
	pub, ok := priv.Public().(ed25519.PublicKey)
	if !ok {
		return KeyPair{}, fmt.Errorf("identity: unexpected public key type derived from private key")
	}
	return KeyPair{PrivateKey: priv, PublicKey: pub}, nil
}

func LoadOrGenerateKeyPair(path string) (KeyPair, error) {
	if _, err := os.Stat(path); err == nil {
		return LoadKeyPair(path)
	} else if !os.IsNotExist(err) {
		return KeyPair{}, fmt.Errorf("identity: stat key file: %w", err)
	}

	kp, err := GenerateKeyPair()
	if err != nil {
		return KeyPair{}, err
	}
	if err := SaveKeyPair(path, kp); err != nil {
		return KeyPair{}, err
	}
	return kp, nil
}

func (kp KeyPair) SignNonce(nonce []byte) []byte {
	return ed25519.Sign(kp.PrivateKey, nonce)
}

func VerifyNonceSignature(pub ed25519.PublicKey, nonce, signature []byte) bool {
	return ed25519.Verify(pub, nonce, signature)
}
