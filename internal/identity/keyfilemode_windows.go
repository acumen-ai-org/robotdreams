//go:build windows

package identity

import "os"

func checkKeyFileMode(string, os.FileInfo) error {
	return nil
}
