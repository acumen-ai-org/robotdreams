package selfupdate

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDetectResolvesTheRunningExecutable(t *testing.T) {
	t.Setenv(InstallHintEnv, "")
	inst, err := Detect("0.1.0", testDevVersion, "release")
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	if inst.ExePath != exe {
		t.Fatalf("ExePath = %q, want the resolved executable %q", inst.ExePath, exe)
	}
	if inst.Reason == "" {
		t.Fatal("Detect gave no reason")
	}

	if inst.Method != MethodRelease && inst.Method != MethodNPM {
		t.Fatalf("Method = %q for a release-stamped binary", inst.Method)
	}
	if !inst.SelfManage && inst.Fix == "" {
		t.Fatal("refused to self-manage without a fix")
	}

	t.Setenv(InstallHintEnv, string(MethodNPM))
	inst, err = Detect("0.1.0", testDevVersion, "release")
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if inst.Method != MethodNPM || inst.SelfManage || inst.ExePath != exe {
		t.Fatalf("hinted Detect = %+v", inst)
	}

	t.Setenv(InstallHintEnv, "")
	t.Setenv("GOBIN", "")
	t.Setenv("GOPATH", "")
	t.Setenv("HOME", t.TempDir())
	inst, err = Detect(testDevVersion, testDevVersion, "")
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if inst.Method != MethodSource || inst.SelfManage {
		t.Fatalf("unstamped Detect = %+v", inst)
	}
	if !strings.HasPrefix(inst.Reason, "a local build") {
		t.Fatalf("Reason = %q", inst.Reason)
	}
}

func TestHostEnvironmentReadsTheProcessEnvironment(t *testing.T) {
	t.Setenv("GOBIN", "/opt/gobin")
	t.Setenv("GOPATH", "/opt/gopath")
	t.Setenv("HOME", "/home/tester")
	t.Setenv(InstallHintEnv, "npm")
	env := HostEnvironment()
	want := Environment{GOOS: runtime.GOOS, GOBIN: "/opt/gobin", GOPATH: "/opt/gopath", Home: "/home/tester", InstallHint: "npm"}
	if env != want {
		t.Fatalf("HostEnvironment() = %+v, want %+v", env, want)
	}

	t.Setenv("GOBIN", "")
	t.Setenv(InstallHintEnv, "")
	env = HostEnvironment()
	if env.GOBIN != "" || env.InstallHint != "" {
		t.Fatalf("unset variables read back as %+v", env)
	}
}

func TestShortRevision(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"abc", "abc"},
		{"0123456789ab", "0123456789ab"},
		{"0123456789abcdef0123456789abcdef01234567", "0123456789ab"},
	}
	for _, tc := range tests {
		if got := shortRevision(tc.in); got != tc.want {
			t.Errorf("shortRevision(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestTargetForHostMatchesTheRunningPlatform(t *testing.T) {
	want, err := TargetFor("0.4.2", runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skipf("host %s/%s is not in the release matrix: %v", runtime.GOOS, runtime.GOARCH, err)
	}
	got, err := TargetForHost("v0.4.2")
	if err != nil {
		t.Fatalf("TargetForHost: %v", err)
	}
	if got != want {
		t.Fatalf("TargetForHost = %+v, want %+v", got, want)
	}
	if !strings.Contains(got.ArchiveName, runtime.GOOS+"_"+runtime.GOARCH) {
		t.Fatalf("ArchiveName %q does not name the host", got.ArchiveName)
	}
	if _, err := TargetForHost(""); err == nil {
		t.Fatal("TargetForHost accepted an empty version")
	}
}

func TestNewGitHubReadsTheTokenFromTheEnvironment(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	g := NewGitHub()
	if g.Owner != DefaultOwner || g.Repo != DefaultRepo || g.Token != "" || g.HC != nil || g.APIBase != "" || g.DownloadBase != "" {
		t.Fatalf("NewGitHub() = %+v, want the default anonymous channel", g)
	}
	if g.apiBase() != "https://api.github.com" || g.downloadBase() != "https://github.com" {
		t.Fatalf("bases = %q / %q", g.apiBase(), g.downloadBase())
	}

	t.Setenv("GH_TOKEN", "gh-secret")
	if g := NewGitHub(); g.Token != "gh-secret" {
		t.Fatalf("GH_TOKEN not read: %q", g.Token)
	}

	t.Setenv("GITHUB_TOKEN", "github-secret")
	if g := NewGitHub(); g.Token != "github-secret" {
		t.Fatalf("GITHUB_TOKEN did not take precedence: %q", g.Token)
	}
}
