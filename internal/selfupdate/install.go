package selfupdate

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
)

type Method string

const (
	MethodRelease   Method = "release"
	MethodNPM       Method = "npm"
	MethodGoInstall Method = "go-install"
	MethodSource    Method = "source"
	MethodUnknown   Method = "unknown"
)

const InstallHintEnv = "ROBOTDREAMS_INSTALL_METHOD"

type Install struct {
	Method     Method
	ExePath    string
	Reason     string
	SelfManage bool
	Fix        string
}

type Environment struct {
	GOOS        string
	GOBIN       string
	GOPATH      string
	Home        string
	InstallHint string
}

func HostEnvironment() Environment {
	return Environment{
		GOOS:        runtime.GOOS,
		GOBIN:       os.Getenv("GOBIN"),
		GOPATH:      os.Getenv("GOPATH"),
		Home:        os.Getenv("HOME"),
		InstallHint: os.Getenv(InstallHintEnv),
	}
}

func Detect(version, devVersion, buildChannel string) (Install, error) {
	exe, err := os.Executable()
	if err != nil {
		return Install{Method: MethodUnknown, Reason: "the running executable's path could not be determined"},
			fmt.Errorf("selfupdate: locate executable: %w", err)
	}

	resolved, rErr := filepath.EvalSymlinks(exe)
	if rErr == nil {
		exe = resolved
	}

	bi, _ := debug.ReadBuildInfo()
	inst := classify(exe, version, devVersion, buildChannel, HostEnvironment(), bi, rErr != nil)
	return inst, nil
}

func classify(exe, version, devVersion, buildChannel string, env Environment, bi *debug.BuildInfo, symlinkUnresolved bool) Install {
	dir := filepath.Dir(exe)

	if hinted := env.InstallHint == string(MethodNPM); hinted || isNPMPath(exe) {
		reason := "the binary lives in an npm package directory (node_modules/" + DefaultRepo + "/dist)"
		if hinted {
			reason = "the npm launcher declared it, via $" + InstallHintEnv
		}
		return Install{
			Method:  MethodNPM,
			ExePath: exe,
			Reason:  reason,
			Fix:     npmFix(""),
		}
	}

	if version == devVersion || buildChannel != "release" {
		if isGoBinDir(dir, env) {
			return Install{
				Method:  MethodGoInstall,
				ExePath: exe,
				Reason:  "the binary is in the Go install directory and carries no release version stamp",
				Fix:     goInstallFix(""),
			}
		}
		return Install{
			Method:  MethodSource,
			ExePath: exe,
			Reason:  reasonForSource(bi),
			Fix:     "git pull && make build",
		}
	}

	if symlinkUnresolved {
		return Install{
			Method:  MethodUnknown,
			ExePath: exe,
			Reason:  "the executable path could not be fully resolved, so its provenance is uncertain",
			Fix:     "replace " + exe + " manually",
		}
	}
	if !writableDir(dir) {
		return Install{
			Method:  MethodRelease,
			ExePath: exe,
			Reason:  "a release build in " + dir + ", which this user cannot write",
			Fix:     "re-run with sufficient permissions to write " + dir,
		}
	}
	return Install{
		Method:     MethodRelease,
		ExePath:    exe,
		Reason:     "a release build in " + dir,
		SelfManage: true,
	}
}

func isNPMPath(exe string) bool {
	dir := filepath.Dir(exe)
	if filepath.Base(dir) == "dist" &&
		filepath.Base(filepath.Dir(dir)) == DefaultRepo &&
		filepath.Base(filepath.Dir(filepath.Dir(dir))) == "node_modules" {
		return true
	}

	return underNodeModules(exe)
}

func underNodeModules(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == "node_modules" {
			return true
		}
	}
	return false
}

func isGoBinDir(dir string, env Environment) bool {
	for _, candidate := range []string{
		env.GOBIN,
		filepath.Join(env.GOPATH, "bin"),
		filepath.Join(env.Home, "go", "bin"),
	} {
		if candidate == "" || candidate == "bin" {
			continue
		}
		if filepath.Clean(candidate) == filepath.Clean(dir) {
			return true
		}
	}
	return false
}

func reasonForSource(bi *debug.BuildInfo) string {
	if bi != nil {
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" && s.Value != "" {
				return "a local build from revision " + shortRevision(s.Value)
			}
		}
	}
	return "a local build carrying no release version stamp"
}

func shortRevision(rev string) string {
	if len(rev) > 12 {
		return rev[:12]
	}
	return rev
}

func writableDir(dir string) bool {
	f, err := os.CreateTemp(dir, ".dream-writable-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

func npmFix(target string) string {
	if target == "" {
		return "npm install -g " + DefaultRepo + "@latest"
	}
	return "npm install -g " + DefaultRepo + "@" + TrimVersionPrefix(target)
}

func goInstallFix(target string) string {
	const mod = "github.com/acumen-ai-org/robotdreams/cmd/dream"
	if target == "" {
		return "go install " + mod + "@latest"
	}
	return "go install " + mod + "@v" + TrimVersionPrefix(target)
}

func (i Install) withTargetVersion(target string) Install {
	if target == "" {
		return i
	}
	switch i.Method {
	case MethodNPM:
		i.Fix = npmFix(target)
	case MethodGoInstall:
		i.Fix = goInstallFix(target)
	}
	return i
}
