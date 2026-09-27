//go:build !windows

package identity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveKeyPairWritesOwnerOnlyMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.key")
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
}

func TestLoadKeyPairRejectsLoosePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.key")
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
