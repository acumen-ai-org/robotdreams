//go:build windows

package identity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadKeyPairAcceptsWindowsReportedMode(t *testing.T) {
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
	if perm := info.Mode().Perm(); perm&0o077 == 0 {
		t.Fatalf("key file mode = %04o; expected Windows to report group/other bits", perm)
	}

	if _, err := LoadKeyPair(path); err != nil {
		t.Fatalf("LoadKeyPair: %v", err)
	}
}
