//go:build !windows

package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFakeBinary(t *testing.T, path, version string, exitCode int) {
	t.Helper()
	script := "#!/bin/sh\n"
	if exitCode != 0 {
		script += "echo 'boom' >&2\nexit " + itoa(exitCode) + "\n"
	} else {
		script += "echo 'dream version " + version + "'\n"
	}
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func requireShell(t *testing.T) {
	t.Helper()
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("/bin/sh unavailable; the smoke test needs a runnable script")
	}
}

func TestApplyReplacesAtomically(t *testing.T) {
	requireShell(t)
	dir := t.TempDir()
	exe := filepath.Join(dir, "dream")
	staged := filepath.Join(dir, ".dream-new")

	writeFakeBinary(t, exe, "0.4.1", 0)
	writeFakeBinary(t, staged, "0.4.2", 0)

	a := Applier{
		Install:     Install{Method: MethodRelease, ExePath: exe, SelfManage: true},
		WantVersion: "0.4.2",
	}
	if err := a.Apply(context.Background(), staged); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatalf("read replaced binary: %v", err)
	}
	if !strings.Contains(string(got), "0.4.2") {
		t.Fatalf("the running path does not hold the new binary: %s", got)
	}

	backup, err := os.ReadFile(exe + backupSuffix)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if !strings.Contains(string(backup), "0.4.1") {
		t.Fatalf("backup does not hold the old binary: %s", backup)
	}

	CleanupBackup(exe)
	if _, err := os.Stat(exe + backupSuffix); !os.IsNotExist(err) {
		t.Fatal("CleanupBackup left the backup behind")
	}
}

func TestApplyRollsBackWhenInstallFails(t *testing.T) {
	requireShell(t)
	dir := t.TempDir()
	exe := filepath.Join(dir, "dream")
	staged := filepath.Join(dir, ".dream-new")

	writeFakeBinary(t, exe, "0.4.1", 0)
	writeFakeBinary(t, staged, "0.4.2", 0)
	original, err := os.ReadFile(exe)
	if err != nil {
		t.Fatalf("read original: %v", err)
	}

	calls := 0
	orig := renameFn
	renameFn = func(oldpath, newpath string) error {
		calls++
		if calls == 2 {
			return errors.New("simulated rename failure")
		}
		return orig(oldpath, newpath)
	}
	t.Cleanup(func() { renameFn = orig })

	a := Applier{
		Install:     Install{Method: MethodRelease, ExePath: exe, SelfManage: true},
		WantVersion: "0.4.2",
	}
	err = a.Apply(context.Background(), staged)
	if err == nil {
		t.Fatal("Apply succeeded despite a failing rename")
	}
	if !strings.Contains(err.Error(), "restored") {
		t.Fatalf("error does not say the original was restored: %v", err)
	}

	restored, rErr := os.ReadFile(exe)
	if rErr != nil {
		t.Fatalf("the original binary is missing after rollback: %v", rErr)
	}
	if string(restored) != string(original) {
		t.Fatalf("restored binary differs from the original:\n got %s\nwant %s", restored, original)
	}
}

func TestApplyRejectsBadBinary(t *testing.T) {
	requireShell(t)

	tests := []struct {
		name        string
		stagedVer   string
		exitCode    int
		wantVersion string
	}{
		{"binary does not run", "0.4.2", 1, "0.4.2"},
		{"binary reports the wrong version", "0.3.9", 0, "0.4.2"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			exe := filepath.Join(dir, "dream")
			staged := filepath.Join(dir, ".dream-new")

			writeFakeBinary(t, exe, "0.4.1", 0)
			writeFakeBinary(t, staged, tc.stagedVer, tc.exitCode)
			original, err := os.ReadFile(exe)
			if err != nil {
				t.Fatalf("read original: %v", err)
			}

			a := Applier{
				Install:     Install{Method: MethodRelease, ExePath: exe, SelfManage: true},
				WantVersion: tc.wantVersion,
			}
			if err := a.Apply(context.Background(), staged); err == nil {
				t.Fatal("Apply succeeded with a bad staged binary")
			}

			after, err := os.ReadFile(exe)
			if err != nil {
				t.Fatalf("the running binary is gone: %v", err)
			}
			if string(after) != string(original) {
				t.Fatal("the running binary was modified despite a failed smoke test")
			}
			if _, err := os.Stat(exe + backupSuffix); err == nil {
				t.Fatal("a backup was created before the smoke test passed")
			}
		})
	}
}

func TestApplyPreservesPermissionBits(t *testing.T) {
	requireShell(t)
	dir := t.TempDir()
	exe := filepath.Join(dir, "dream")
	staged := filepath.Join(dir, ".dream-new")

	writeFakeBinary(t, exe, "0.4.1", 0)
	writeFakeBinary(t, staged, "0.4.2", 0)
	if err := os.Chmod(exe, 0o750); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	a := Applier{
		Install:     Install{Method: MethodRelease, ExePath: exe, SelfManage: true},
		WantVersion: "0.4.2",
	}
	if err := a.Apply(context.Background(), staged); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	fi, err := os.Stat(exe)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if fi.Mode().Perm() != 0o750 {
		t.Fatalf("permissions = %v, want the original 0750", fi.Mode().Perm())
	}
}

func TestStageDirIsTargetDir(t *testing.T) {
	if got, want := StageDir("/usr/local/bin/dream"), "/usr/local/bin"; got != want {
		t.Fatalf("StageDir = %q, want %q", got, want)
	}
}
