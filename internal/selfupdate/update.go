package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"
)

const lockStaleAfter = 15 * time.Minute

type Options struct {
	CurrentVersion string
	DevVersion     string
	BuildChannel   string
	TargetVersion  string
	AllowDowngrade bool
	CheckOnly      bool
	Releases       *GitHub
}

type Result struct {
	Install        Install
	CurrentVersion string
	LatestVersion  string
	Updated        bool
	AlreadyCurrent bool
	Refused        bool
}

var ErrRefused = errors.New("selfupdate: this binary is managed by another installer")

func Run(ctx context.Context, opts Options) (Result, error) {
	inst, err := Detect(opts.CurrentVersion, opts.DevVersion, opts.BuildChannel)
	if err != nil {
		return Result{}, err
	}
	res := Result{Install: inst, CurrentVersion: TrimVersionPrefix(opts.CurrentVersion)}

	releases := opts.Releases
	if releases == nil {
		releases = NewGitHub()
	}

	target := TrimVersionPrefix(opts.TargetVersion)
	if target == "" {
		latest, err := releases.Latest(ctx)
		if err != nil {
			return res, err
		}
		target = latest
	}
	res.LatestVersion = target

	if cmp, err := Compare(target, res.CurrentVersion); err == nil {
		switch {
		case cmp == 0:
			res.AlreadyCurrent = true
			return res, nil
		case cmp < 0 && !opts.AllowDowngrade:
			return res, fmt.Errorf("selfupdate: %s is older than the installed %s; pass --allow-downgrade to install it anyway",
				target, res.CurrentVersion)
		}
	} else if opts.TargetVersion == "" && res.CurrentVersion == target {
		res.AlreadyCurrent = true
		return res, nil
	}

	if opts.CheckOnly {
		return res, nil
	}

	if !inst.SelfManage {
		res.Install = inst.withTargetVersion(target)
		res.Refused = true
		return res, fmt.Errorf("%w (%s): %s", ErrRefused, inst.Method, inst.Reason)
	}
	if !SupportsInPlace() {
		res.Refused = true
		return res, fmt.Errorf("%w on %s: %s", ErrUnsupportedPlatform, runtime.GOOS, inst.ExePath)
	}

	unlock, err := lock(inst.ExePath)
	if err != nil {
		return res, err
	}
	defer unlock()

	if err := download(ctx, releases, inst, target); err != nil {
		return res, err
	}
	res.Updated = true
	return res, nil
}

func download(ctx context.Context, releases *GitHub, inst Install, target string) error {
	tgt, err := TargetForHost(target)
	if err != nil {
		return err
	}

	stageDir := StageDir(inst.ExePath)
	staged, err := os.CreateTemp(stageDir, ".dream-new-*")
	if err != nil {
		return fmt.Errorf("selfupdate: stage a download in %s: %w", stageDir, err)
	}
	stagedPath := staged.Name()
	defer func() {
		_ = staged.Close()
		_ = os.Remove(stagedPath)
	}()

	archivePath := stagedPath + ".archive"
	archive, err := os.Create(archivePath)
	if err != nil {
		return fmt.Errorf("selfupdate: stage an archive: %w", err)
	}
	defer os.Remove(archivePath)

	sum, err := releases.DownloadAsset(ctx, target, tgt.ArchiveName, archive)
	closeErr := archive.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return fmt.Errorf("selfupdate: finish writing the archive: %w", closeErr)
	}

	checksums, err := releases.Checksums(ctx, target)
	if err != nil {
		return err
	}
	want, err := ChecksumFor(checksums, tgt.ArchiveName)
	if err != nil {
		return err
	}
	if err := VerifySHA256(sum, want); err != nil {
		return err
	}

	binaryPath := stagedPath + ".bin"
	defer os.Remove(binaryPath)
	if err := ExtractBinary(archivePath, tgt.BinaryName, binaryPath); err != nil {
		return err
	}

	return Applier{Install: inst, WantVersion: target}.Apply(ctx, binaryPath)
}

func lock(exePath string) (func(), error) {
	path := filepath.Join(filepath.Dir(exePath), ".dream-update.lock")

	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if fi, statErr := os.Stat(path); statErr == nil && time.Since(fi.ModTime()) > lockStaleAfter {
			_ = os.Remove(path)
			f, err = os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		}
		if err != nil {
			return nil, fmt.Errorf("selfupdate: another update is in progress (%s); "+
				"remove it if no update is running", path)
		}
	}
	_, _ = f.WriteString(strconv.Itoa(os.Getpid()))
	_ = f.Close()

	return func() { _ = os.Remove(path) }, nil
}

func Hops() int {
	n, err := strconv.Atoi(os.Getenv(HopsEnv))
	if err != nil || n < 0 {
		return 0
	}
	return n
}
