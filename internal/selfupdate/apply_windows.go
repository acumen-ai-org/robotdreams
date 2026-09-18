//go:build windows

package selfupdate

func Restart(exePath string, argv []string, hops int) error {
	return ErrUnsupportedPlatform
}

func SupportsInPlace() bool { return false }
