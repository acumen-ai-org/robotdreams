package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const smokeTestTimeout = 30 * time.Second

const HopsEnv = "ROBOTDREAMS_UPDATE_HOPS"

const backupSuffix = ".old"

var renameFn = os.Rename

type Applier struct {
	Install     Install
	WantVersion string
}

func (a Applier) Apply(ctx context.Context, stagedPath string) error {
	exe := a.Install.ExePath
	if exe == "" {
		return errors.New("selfupdate: no executable path to replace")
	}

	if err := a.smokeTest(ctx, stagedPath); err != nil {
		return err
	}

	if fi, err := os.Stat(exe); err == nil {
		if err := os.Chmod(stagedPath, fi.Mode().Perm()); err != nil {
			return fmt.Errorf("selfupdate: chmod staged binary: %w", err)
		}
	}

	backup := exe + backupSuffix
	_ = os.Remove(backup)
	if err := renameFn(exe, backup); err != nil {
		return fmt.Errorf("selfupdate: move the current binary aside (%s): %w\n"+
			"nothing was changed", exe, err)
	}

	if err := renameFn(stagedPath, exe); err != nil {
		if rbErr := renameFn(backup, exe); rbErr != nil {
			return fmt.Errorf("selfupdate: installing the new binary failed (%v) AND restoring the "+
				"original failed (%v).\nThe working binary is at %s and must be moved back to %s by hand",
				err, rbErr, backup, exe)
		}
		return fmt.Errorf("selfupdate: install the new binary: %w\nthe original was restored", err)
	}
	return nil
}

func (a Applier) smokeTest(ctx context.Context, stagedPath string) error {
	ctx, cancel := context.WithTimeout(ctx, smokeTestTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, stagedPath, "version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("selfupdate: the downloaded binary does not run (%w): %s\n"+
			"nothing was changed", err, strings.TrimSpace(string(out)))
	}
	if a.WantVersion != "" && !strings.Contains(string(out), TrimVersionPrefix(a.WantVersion)) {
		return fmt.Errorf("selfupdate: the downloaded binary reports %q, want version %s\n"+
			"nothing was changed", strings.TrimSpace(string(out)), TrimVersionPrefix(a.WantVersion))
	}
	return nil
}

func CleanupBackup(exePath string) {
	if exePath == "" {
		return
	}
	_ = os.Remove(exePath + backupSuffix)
}

func StageDir(exePath string) string { return filepath.Dir(exePath) }
