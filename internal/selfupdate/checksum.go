package selfupdate

import (
	"fmt"
	"strings"
)

func ChecksumFor(checksumsText, assetName string) (string, error) {
	for _, line := range strings.Split(checksumsText, "\n") {
		line = strings.TrimRight(line, "\r")
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		sum, name := fields[0], strings.TrimPrefix(fields[1], "*")
		if name != assetName {
			continue
		}
		if len(sum) != 64 {
			return "", fmt.Errorf("selfupdate: checksum for %q is %d hex characters, want 64", assetName, len(sum))
		}
		return strings.ToLower(sum), nil
	}
	return "", fmt.Errorf("selfupdate: no checksum for %q in checksums.txt", assetName)
}

func VerifySHA256(gotHex, wantHex string) error {
	if !strings.EqualFold(gotHex, wantHex) {
		return fmt.Errorf("selfupdate: SHA-256 mismatch: expected %s, got %s", wantHex, gotHex)
	}
	return nil
}
