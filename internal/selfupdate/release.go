package selfupdate

import (
	"fmt"
	"runtime"
)

const (
	projectName    = "dream"
	binaryName     = "dream"
	checksumsAsset = "checksums.txt"
	DefaultOwner   = "acumen-ai-org"
	DefaultRepo    = "robotdreams"
)

type Target struct {
	GOOS        string
	GOARCH      string
	ArchiveName string
	BinaryName  string
}

var ErrUnsupportedPlatform = fmt.Errorf("selfupdate: unsupported platform")

var (
	supportedGOOS   = []string{"linux", "darwin", "windows"}
	supportedGOARCH = []string{"amd64", "arm64"}
)

func TargetFor(version, goos, goarch string) (Target, error) {
	if version == "" {
		return Target{}, fmt.Errorf("selfupdate: empty version")
	}
	version = TrimVersionPrefix(version)

	if !contains(supportedGOOS, goos) || !contains(supportedGOARCH, goarch) {
		return Target{}, fmt.Errorf("%w: no prebuilt dream binary for %s/%s (supported: %v on %v)",
			ErrUnsupportedPlatform, goos, goarch, supportedGOOS, supportedGOARCH)
	}

	ext := ".tar.gz"
	bin := binaryName
	if goos == "windows" {
		ext = ".zip"
		bin = binaryName + ".exe"
	}

	return Target{
		GOOS:        goos,
		GOARCH:      goarch,
		ArchiveName: fmt.Sprintf("%s_%s_%s_%s%s", projectName, version, goos, goarch, ext),
		BinaryName:  bin,
	}, nil
}

func TargetForHost(version string) (Target, error) {
	return TargetFor(version, runtime.GOOS, runtime.GOARCH)
}

func TrimVersionPrefix(v string) string {
	if len(v) > 1 && v[0] == 'v' {
		return v[1:]
	}
	return v
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
