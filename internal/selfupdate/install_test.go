package selfupdate

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const testDevVersion = "0.1.0-dev"

func TestClassify(t *testing.T) {
	env := Environment{
		GOOS:   "linux",
		GOBIN:  "/home/dev/gobin",
		GOPATH: "/home/dev/go",
		Home:   "/home/dev",
	}

	tests := []struct {
		name         string
		exe          string
		version      string
		buildChannel string
		env          Environment
		want         Method
	}{
		{
			name:    "npm dist path",
			exe:     "/proj/node_modules/robotdreams/dist/dream",
			version: "0.4.2", buildChannel: "release", env: env,
			want: MethodNPM,
		},
		{
			name:    "pnpm nested layout",
			exe:     "/proj/node_modules/.pnpm/robotdreams@0.4.2/node_modules/robotdreams/dist/dream",
			version: "0.4.2", buildChannel: "release", env: env,
			want: MethodNPM,
		},
		{
			name:    "npm wins even for a dev build",
			exe:     "/proj/node_modules/robotdreams/dist/dream",
			version: testDevVersion, buildChannel: "", env: env,
			want: MethodNPM,
		},
		{
			name:    "explicit npm hint",
			exe:     "/opt/somewhere/dream",
			version: "0.4.2", buildChannel: "release",
			env:  Environment{GOOS: "linux", InstallHint: string(MethodNPM)},
			want: MethodNPM,
		},
		{
			name:    "GOBIN",
			exe:     "/home/dev/gobin/dream",
			version: testDevVersion, buildChannel: "", env: env,
			want: MethodGoInstall,
		},
		{
			name:    "GOPATH bin",
			exe:     "/home/dev/go/bin/dream",
			version: testDevVersion, buildChannel: "",
			env:  Environment{GOOS: "linux", GOPATH: "/home/dev/go", Home: "/home/dev"},
			want: MethodGoInstall,
		},
		{
			name:    "default HOME/go/bin",
			exe:     "/home/dev/go/bin/dream",
			version: testDevVersion, buildChannel: "",
			env:  Environment{GOOS: "linux", Home: "/home/dev"},
			want: MethodGoInstall,
		},
		{
			name:    "local make build",
			exe:     "/src/robotdreams/bin/dream",
			version: testDevVersion, buildChannel: "", env: env,
			want: MethodSource,
		},
		{
			name:    "go run temp dir",
			exe:     "/tmp/go-build123/b001/exe/dream",
			version: testDevVersion, buildChannel: "", env: env,
			want: MethodSource,
		},
		{
			name:    "release stamp but no channel marker is still source",
			exe:     "/usr/local/bin/dream",
			version: "0.4.2", buildChannel: "", env: env,
			want: MethodSource,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := classify(tc.exe, tc.version, testDevVersion, tc.buildChannel, tc.env, nil, false)
			if got.Method != tc.want {
				t.Fatalf("method = %q, want %q (reason: %s)", got.Method, tc.want, got.Reason)
			}
			if got.Method != MethodRelease && got.SelfManage {
				t.Fatalf("%q self-manages; only a release build may", got.Method)
			}
			if !got.SelfManage && got.Fix == "" {
				t.Fatalf("%q refuses to self-manage but offers no fix command", got.Method)
			}
		})
	}
}

func TestClassifyReleaseSelfManages(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "dream")

	got := classify(exe, "0.4.2", testDevVersion, "release", Environment{GOOS: runtime.GOOS}, nil, false)
	if got.Method != MethodRelease {
		t.Fatalf("method = %q, want %q", got.Method, MethodRelease)
	}
	if !got.SelfManage {
		t.Fatalf("a release build in a writable dir does not self-manage: %s", got.Reason)
	}
	if got.Fix != "" {
		t.Fatalf("a self-managing install should not carry a fix command, got %q", got.Fix)
	}
}

func TestClassifyRefusesWhenPathUnresolved(t *testing.T) {
	dir := t.TempDir()
	got := classify(filepath.Join(dir, "dream"), "0.4.2", testDevVersion, "release",
		Environment{GOOS: runtime.GOOS}, nil, true)
	if got.Method != MethodUnknown || got.SelfManage {
		t.Fatalf("got %q self-manage=%v, want unknown and no self-management", got.Method, got.SelfManage)
	}
}

func TestClassifyRefusesUnwritableReleaseDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory mode bits do not gate writes the same way on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root can write a read-only directory, so this cannot be exercised as root")
	}

	dir := filepath.Join(t.TempDir(), "ro")
	if err := os.Mkdir(dir, 0o500); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	got := classify(filepath.Join(dir, "dream"), "0.4.2", testDevVersion, "release",
		Environment{GOOS: runtime.GOOS}, nil, false)
	if got.SelfManage {
		t.Fatal("classified an unwritable directory as self-managing")
	}
	if got.Fix == "" {
		t.Fatal("no fix offered for an unwritable install directory")
	}
}

func TestWritableDir(t *testing.T) {
	if !writableDir(t.TempDir()) {
		t.Fatal("a fresh temp dir reported unwritable")
	}
	if writableDir(filepath.Join(t.TempDir(), "does-not-exist")) {
		t.Fatal("a nonexistent dir reported writable")
	}
}

func TestIsNPMPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/proj/node_modules/robotdreams/dist/dream", true},
		{"/proj/node_modules/.pnpm/robotdreams@0.4.2/node_modules/robotdreams/dist/dream", true},
		{"/proj/node_modules/.bin/dream", true},
		{"/usr/local/bin/dream", false},
		{"/home/dev/go/bin/dream", false},
		{"/src/node_modules_backup/dream", false},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			if got := isNPMPath(tc.path); got != tc.want {
				t.Fatalf("isNPMPath(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestFixNamesTheTargetVersion(t *testing.T) {
	npm := classify("/proj/node_modules/robotdreams/dist/dream", "0.2.1", testDevVersion, "release",
		Environment{GOOS: "linux"}, nil, false)
	if strings.Contains(npm.Fix, "0.2.1") {
		t.Fatalf("the pre-resolution fix names the INSTALLED version: %q", npm.Fix)
	}

	withTarget := npm.withTargetVersion("0.3.0")
	if withTarget.Fix != "npm install -g robotdreams@0.3.0" {
		t.Fatalf("fix = %q, want it to name the target 0.3.0", withTarget.Fix)
	}

	gi := classify("/home/dev/go/bin/dream", testDevVersion, testDevVersion, "",
		Environment{GOOS: "linux", Home: "/home/dev"}, nil, false)
	if got := gi.withTargetVersion("0.3.0").Fix; got != "go install github.com/acumen-ai-org/robotdreams/cmd/dream@v0.3.0" {
		t.Fatalf("go-install fix = %q, want it to name v0.3.0", got)
	}

	src := classify("/src/bin/dream", testDevVersion, testDevVersion, "", Environment{GOOS: "linux"}, nil, false)
	if src.withTargetVersion("0.3.0").Fix != src.Fix {
		t.Fatalf("a source build's fix was rewritten: %q", src.withTargetVersion("0.3.0").Fix)
	}
}
