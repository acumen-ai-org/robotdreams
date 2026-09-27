//go:build !windows

package identity

import (
	"fmt"
	"os"
)

func checkKeyFileMode(path string, info os.FileInfo) error {
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("identity: key file %s has overly permissive mode %04o (want 0600)", path, info.Mode().Perm())
	}
	return nil
}
