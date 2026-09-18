//go:build !windows

package selfupdate

import (
	"fmt"
	"os"
	"strconv"
	"syscall"
)

func Restart(exePath string, argv []string, hops int) error {
	env := append(os.Environ(), HopsEnv+"="+strconv.Itoa(hops+1))
	if err := syscall.Exec(exePath, argv, env); err != nil {
		return fmt.Errorf("selfupdate: re-exec %s: %w", exePath, err)
	}
	return nil
}

func SupportsInPlace() bool { return true }
