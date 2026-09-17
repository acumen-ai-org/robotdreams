//go:build !windows

package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeRelease struct {
	t                *testing.T
	version          string
	target           Target
	archive          []byte
	mu               sync.Mutex
	assetHits        int
	checksumOverride string
	assetStatus      int
	latestStatus     int
	omitChecksum     bool
	srv              *httptest.Server
}

func newFakeRelease(t *testing.T, version, binaryVersion string) *fakeRelease {
	t.Helper()
	tgt, err := TargetForHost(version)
	if err != nil {
		t.Skipf("no release target for this host: %v", err)
	}
	f := &fakeRelease{t: t, version: version, target: tgt}
	f.archive = tarGzScript(t, tgt.BinaryName, "#!/bin/sh\necho 'dream version "+binaryVersion+"'\n")
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeRelease) github() *GitHub {
	return &GitHub{Owner: "acme", Repo: "proj", APIBase: f.srv.URL, DownloadBase: f.srv.URL}
}

func (f *fakeRelease) checksums() string {
	sum := sha256.Sum256(f.archive)
	digest := hex.EncodeToString(sum[:])
	if f.checksumOverride != "" {
		digest = f.checksumOverride
	}
	lines := "0000000000000000000000000000000000000000000000000000000000000000  dream_" + f.version + "_other_arch.tar.gz\n"
	if !f.omitChecksum {
		lines += digest + "  " + f.target.ArchiveName + "\n"
	}
	return lines
}

func (f *fakeRelease) hits() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.assetHits
}

func (f *fakeRelease) serve(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/repos/acme/proj/releases/latest":
		if f.latestStatus != 0 {
			w.WriteHeader(f.latestStatus)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": "v" + f.version})
	case "/acme/proj/releases/download/v" + f.version + "/" + f.target.ArchiveName:
		f.mu.Lock()
		f.assetHits++
		f.mu.Unlock()
		if f.assetStatus != 0 {
			w.WriteHeader(f.assetStatus)
			return
		}
		_, _ = w.Write(f.archive)
	case "/acme/proj/releases/download/v" + f.version + "/" + checksumsAsset:
		_, _ = w.Write([]byte(f.checksums()))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func tarGzScript(t *testing.T, name, script string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(script)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(script)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func installedAt(t *testing.T, version string) Install {
	t.Helper()
	requireShell(t)
	dir := t.TempDir()
	exe := filepath.Join(dir, "dream")
	writeFakeBinary(t, exe, version, 0)
	return Install{Method: MethodRelease, ExePath: exe, SelfManage: true, Reason: "a release build in " + dir}
}

func runVersion(t *testing.T, exe string) string {
	t.Helper()
	out, err := exec.Command(exe, "version").CombinedOutput()
	if err != nil {
		t.Fatalf("%s version: %v: %s", exe, err, out)
	}
	return strings.TrimSpace(string(out))
}

func dirEntries(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestDownloadInstallsAVerifiedRelease(t *testing.T) {
	rel := newFakeRelease(t, "0.5.0", "0.5.0")
	inst := installedAt(t, "0.4.0")

	if err := download(context.Background(), rel.github(), inst, "0.5.0"); err != nil {
		t.Fatalf("download: %v", err)
	}
	if got := runVersion(t, inst.ExePath); !strings.Contains(got, "0.5.0") {
		t.Fatalf("installed binary reports %q, want 0.5.0", got)
	}

	if got := dirEntries(t, filepath.Dir(inst.ExePath)); strings.Join(got, ",") != "dream,dream.old" {
		t.Fatalf("directory after install: %v, want [dream dream.old]", got)
	}
	if got := runVersion(t, inst.ExePath+backupSuffix); !strings.Contains(got, "0.4.0") {
		t.Fatalf("backup reports %q, want the old 0.4.0", got)
	}
	if rel.hits() != 1 {
		t.Fatalf("archive downloaded %d times, want 1", rel.hits())
	}
}

func TestDownloadRefusesAChecksumMismatch(t *testing.T) {
	rel := newFakeRelease(t, "0.5.0", "0.5.0")
	rel.checksumOverride = strings.Repeat("ab", 32)
	inst := installedAt(t, "0.4.0")

	err := download(context.Background(), rel.github(), inst, "0.5.0")
	if err == nil || !strings.Contains(err.Error(), "SHA-256 mismatch") {
		t.Fatalf("err = %v, want a SHA-256 mismatch", err)
	}
	if got := runVersion(t, inst.ExePath); !strings.Contains(got, "0.4.0") {
		t.Fatalf("binary was replaced despite the mismatch: %q", got)
	}
	if got := dirEntries(t, filepath.Dir(inst.ExePath)); strings.Join(got, ",") != "dream" {
		t.Fatalf("staging files left behind: %v", got)
	}
}

func TestDownloadRefusesAMissingChecksumEntry(t *testing.T) {
	rel := newFakeRelease(t, "0.5.0", "0.5.0")
	rel.omitChecksum = true
	inst := installedAt(t, "0.4.0")

	err := download(context.Background(), rel.github(), inst, "0.5.0")
	if err == nil || !strings.Contains(err.Error(), "no checksum for") {
		t.Fatalf("err = %v, want a missing-checksum error", err)
	}
	if got := dirEntries(t, filepath.Dir(inst.ExePath)); strings.Join(got, ",") != "dream" {
		t.Fatalf("staging files left behind: %v", got)
	}
}

func TestDownloadPropagatesAMissingAsset(t *testing.T) {
	rel := newFakeRelease(t, "0.5.0", "0.5.0")
	rel.assetStatus = http.StatusNotFound
	inst := installedAt(t, "0.4.0")

	err := download(context.Background(), rel.github(), inst, "0.5.0")
	if err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("err = %v, want HTTP 404", err)
	}
	if got := runVersion(t, inst.ExePath); !strings.Contains(got, "0.4.0") {
		t.Fatalf("binary changed: %q", got)
	}
	if got := dirEntries(t, filepath.Dir(inst.ExePath)); strings.Join(got, ",") != "dream" {
		t.Fatalf("staging files left behind: %v", got)
	}
}

func TestDownloadRefusesAWrongVersionBinary(t *testing.T) {
	rel := newFakeRelease(t, "0.5.0", "0.4.9")
	inst := installedAt(t, "0.4.0")

	err := download(context.Background(), rel.github(), inst, "0.5.0")
	if err == nil || !strings.Contains(err.Error(), "want version 0.5.0") {
		t.Fatalf("err = %v, want a version-mismatch refusal", err)
	}
	if got := dirEntries(t, filepath.Dir(inst.ExePath)); strings.Join(got, ",") != "dream" {
		t.Fatalf("directory after refusal: %v, want only the untouched binary", got)
	}
}

func TestDownloadRejectsAnEmptyVersion(t *testing.T) {
	inst := installedAt(t, "0.4.0")
	err := download(context.Background(), &GitHub{}, inst, "")
	if err == nil || !strings.Contains(err.Error(), "empty version") {
		t.Fatalf("err = %v, want an empty-version error", err)
	}
}

func TestDownloadFailsWhenTheStageDirIsMissing(t *testing.T) {
	rel := newFakeRelease(t, "0.5.0", "0.5.0")
	inst := Install{Method: MethodRelease, ExePath: filepath.Join(t.TempDir(), "nope", "dream"), SelfManage: true}
	err := download(context.Background(), rel.github(), inst, "0.5.0")
	if err == nil || !strings.Contains(err.Error(), "stage a download") {
		t.Fatalf("err = %v, want a staging error", err)
	}
	if rel.hits() != 0 {
		t.Fatal("downloaded before staging was possible")
	}
}

func TestLock(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "dream")
	lockPath := filepath.Join(dir, ".dream-update.lock")

	unlock, err := lock(exe)
	if err != nil {
		t.Fatalf("lock: %v", err)
	}
	data, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("lock file missing: %v", err)
	}
	if string(data) != fmt.Sprint(os.Getpid()) {
		t.Fatalf("lock holds %q, want this pid", data)
	}

	if _, err := lock(exe); err == nil || !strings.Contains(err.Error(), lockPath) {
		t.Fatalf("second lock: err = %v, want a refusal naming %s", err, lockPath)
	}

	unlock()
	if _, err := os.Stat(lockPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lock file after unlock: %v", err)
	}

	unlock2, err := lock(exe)
	if err != nil {
		t.Fatalf("relock: %v", err)
	}
	unlock2()
	unlock2()
}

func TestLockTakesOverAStaleLock(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "dream")
	lockPath := filepath.Join(dir, ".dream-update.lock")

	if err := os.WriteFile(lockPath, []byte("99999"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-lockStaleAfter - time.Minute)
	if err := os.Chtimes(lockPath, old, old); err != nil {
		t.Fatal(err)
	}

	unlock, err := lock(exe)
	if err != nil {
		t.Fatalf("stale lock was honored: %v", err)
	}
	defer unlock()
	data, _ := os.ReadFile(lockPath)
	if string(data) != fmt.Sprint(os.Getpid()) {
		t.Fatalf("lock holds %q after takeover, want this pid", data)
	}

	unlock()
	if err := os.WriteFile(lockPath, []byte("99999"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := lock(exe); err == nil {
		t.Fatal("a fresh foreign lock was taken over")
	}
}

func TestLockFailsWithoutADirectory(t *testing.T) {
	if _, err := lock(filepath.Join(t.TempDir(), "missing", "dream")); err == nil {
		t.Fatal("lock succeeded in a nonexistent directory")
	}
}

func TestHops(t *testing.T) {
	tests := []struct {
		env  string
		want int
	}{
		{"", 0},
		{"0", 0},
		{"3", 3},
		{"-1", 0},
		{"x", 0},
		{"1.5", 0},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%q", tc.env), func(t *testing.T) {
			t.Setenv(HopsEnv, tc.env)
			if got := Hops(); got != tc.want {
				t.Fatalf("Hops() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestSupportsInPlaceOnUnix(t *testing.T) {
	if !SupportsInPlace() {
		t.Fatalf("SupportsInPlace() = false on %s", runtime.GOOS)
	}
}

func hostInstall(t *testing.T) Install {
	t.Helper()
	inst, err := Detect("0.1.0", testDevVersion, "release")
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	return inst
}

func TestRunCheckOnlyReportsWithoutChanging(t *testing.T) {
	rel := newFakeRelease(t, "9.9.9", "9.9.9")
	res, err := Run(context.Background(), Options{
		CurrentVersion: "v0.1.0", DevVersion: testDevVersion, BuildChannel: "release",
		CheckOnly: true, Releases: rel.github(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.CurrentVersion != "0.1.0" || res.LatestVersion != "9.9.9" {
		t.Fatalf("versions = %q -> %q", res.CurrentVersion, res.LatestVersion)
	}
	if res.Updated || res.AlreadyCurrent || res.Refused {
		t.Fatalf("flags = %+v", res)
	}
	want := hostInstall(t)
	if res.Install.Method != want.Method || res.Install.ExePath != want.ExePath {
		t.Fatalf("Install = %+v, want Detect's %+v", res.Install, want)
	}
	if rel.hits() != 0 {
		t.Fatal("check-only downloaded the archive")
	}
}

func TestRunAlreadyCurrent(t *testing.T) {
	rel := newFakeRelease(t, "0.1.0", "0.1.0")
	g := rel.github()

	res, err := Run(context.Background(), Options{CurrentVersion: "0.1.0", DevVersion: testDevVersion, BuildChannel: "release", Releases: g})
	if err != nil || !res.AlreadyCurrent || res.Updated {
		t.Fatalf("latest == current: res = %+v, err = %v", res, err)
	}

	res, err = Run(context.Background(), Options{CurrentVersion: "0.1.0", TargetVersion: "v0.1.0", DevVersion: testDevVersion, BuildChannel: "release", Releases: g})
	if err != nil || !res.AlreadyCurrent || res.LatestVersion != "0.1.0" {
		t.Fatalf("pinned == current: res = %+v, err = %v", res, err)
	}

	rel2 := newFakeRelease(t, "abc-dev", "abc-dev")
	res, err = Run(context.Background(), Options{CurrentVersion: "abc-dev", DevVersion: testDevVersion, BuildChannel: "", Releases: rel2.github()})
	if err != nil || !res.AlreadyCurrent {
		t.Fatalf("non-semver latest == current: res = %+v, err = %v", res, err)
	}
	if rel.hits()+rel2.hits() != 0 {
		t.Fatal("an already-current run downloaded something")
	}
}

func TestRunRefusesADowngradeUnlessAllowed(t *testing.T) {
	rel := newFakeRelease(t, "0.0.1", "0.0.1")
	opts := Options{CurrentVersion: "0.1.0", DevVersion: testDevVersion, BuildChannel: "release", Releases: rel.github()}

	res, err := Run(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "--allow-downgrade") {
		t.Fatalf("downgrade: err = %v, want a refusal naming the flag", err)
	}
	if res.LatestVersion != "0.0.1" || res.Updated {
		t.Fatalf("res = %+v", res)
	}

	opts.AllowDowngrade = true
	opts.CheckOnly = true
	res, err = Run(context.Background(), opts)
	if err != nil || res.AlreadyCurrent || res.Updated {
		t.Fatalf("allowed downgrade (check): res = %+v, err = %v", res, err)
	}
}

func TestRunPropagatesALatestLookupFailure(t *testing.T) {
	rel := newFakeRelease(t, "9.9.9", "9.9.9")
	rel.latestStatus = http.StatusNotFound
	res, err := Run(context.Background(), Options{CurrentVersion: "0.1.0", DevVersion: testDevVersion, BuildChannel: "release", Releases: rel.github()})
	if err == nil || !strings.Contains(err.Error(), "no latest release") {
		t.Fatalf("err = %v, want the 404 hint", err)
	}

	if res.Install.ExePath == "" || res.CurrentVersion != "0.1.0" {
		t.Fatalf("res = %+v", res)
	}
}

func TestRunRefusesAnNPMInstall(t *testing.T) {
	t.Setenv(InstallHintEnv, string(MethodNPM))
	rel := newFakeRelease(t, "9.9.9", "9.9.9")
	res, err := Run(context.Background(), Options{CurrentVersion: "0.1.0", DevVersion: testDevVersion, BuildChannel: "release", Releases: rel.github()})
	if !errors.Is(err, ErrRefused) {
		t.Fatalf("err = %v, want ErrRefused", err)
	}
	if !res.Refused || res.Updated {
		t.Fatalf("res = %+v", res)
	}
	if res.Install.Method != MethodNPM || res.Install.Fix != "npm install -g "+DefaultRepo+"@9.9.9" {
		t.Fatalf("Install = %+v, want an npm fix naming the target", res.Install)
	}
	if !strings.Contains(err.Error(), "npm launcher declared it") {
		t.Fatalf("err = %v, want it to carry the reason", err)
	}
	if rel.hits() != 0 {
		t.Fatal("a refused run downloaded the archive")
	}

	res, err = Run(context.Background(), Options{CurrentVersion: "0.1.0", DevVersion: testDevVersion, BuildChannel: "release", CheckOnly: true, Releases: rel.github()})
	if err != nil || res.Refused {
		t.Fatalf("check-only on npm: res = %+v, err = %v", res, err)
	}
}

func TestRunRefusesAGoInstall(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	t.Setenv(InstallHintEnv, "")
	t.Setenv("GOBIN", filepath.Dir(exe))
	rel := newFakeRelease(t, "9.9.9", "9.9.9")

	res, err := Run(context.Background(), Options{CurrentVersion: testDevVersion, DevVersion: testDevVersion, BuildChannel: "", Releases: rel.github()})
	if !errors.Is(err, ErrRefused) {
		t.Fatalf("err = %v, want ErrRefused", err)
	}
	if res.Install.Method != MethodGoInstall || !res.Refused {
		t.Fatalf("res = %+v", res)
	}
	if want := "go install github.com/acumen-ai-org/robotdreams/cmd/dream@v9.9.9"; res.Install.Fix != want {
		t.Fatalf("Fix = %q, want %q", res.Install.Fix, want)
	}
}

func TestRunRefusesASourceBuild(t *testing.T) {
	t.Setenv(InstallHintEnv, "")
	t.Setenv("GOBIN", "")
	t.Setenv("GOPATH", "")
	t.Setenv("HOME", t.TempDir())
	rel := newFakeRelease(t, "9.9.9", "9.9.9")
	res, err := Run(context.Background(), Options{CurrentVersion: testDevVersion, DevVersion: testDevVersion, Releases: rel.github()})
	if !errors.Is(err, ErrRefused) || res.Install.Method != MethodSource {
		t.Fatalf("res = %+v, err = %v; want a refused source build", res, err)
	}
	if res.Install.Fix != "git pull && make build" {
		t.Fatalf("Fix = %q", res.Install.Fix)
	}
}

func requireSelfManagingHost(t *testing.T) Install {
	t.Helper()
	t.Setenv(InstallHintEnv, "")
	inst := hostInstall(t)
	if !inst.SelfManage {
		t.Skipf("test binary is not self-managing: %s", inst.Reason)
	}
	return inst
}

func TestRunRefusesWhileAnotherUpdateHoldsTheLock(t *testing.T) {
	inst := requireSelfManagingHost(t)
	unlock, err := lock(inst.ExePath)
	if err != nil {
		t.Fatalf("lock: %v", err)
	}
	defer unlock()

	rel := newFakeRelease(t, "9.9.9", "9.9.9")
	res, err := Run(context.Background(), Options{CurrentVersion: "0.1.0", DevVersion: testDevVersion, BuildChannel: "release", Releases: rel.github()})
	if err == nil || !strings.Contains(err.Error(), "another update is in progress") {
		t.Fatalf("err = %v, want the lock refusal", err)
	}
	if res.Updated || res.Refused {
		t.Fatalf("res = %+v", res)
	}
	if rel.hits() != 0 {
		t.Fatal("downloaded while locked out")
	}

	if _, err := os.Stat(filepath.Join(filepath.Dir(inst.ExePath), ".dream-update.lock")); err != nil {
		t.Fatalf("lock file: %v", err)
	}
}

func TestRunStopsAtAChecksumMismatch(t *testing.T) {
	inst := requireSelfManagingHost(t)
	before, err := os.ReadFile(inst.ExePath)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(inst.ExePath)
	entriesBefore := dirEntries(t, dir)

	rel := newFakeRelease(t, "9.9.9", "9.9.9")
	rel.checksumOverride = strings.Repeat("cd", 32)
	res, err := Run(context.Background(), Options{CurrentVersion: "0.1.0", DevVersion: testDevVersion, BuildChannel: "release", Releases: rel.github()})
	if err == nil || !strings.Contains(err.Error(), "SHA-256 mismatch") {
		t.Fatalf("err = %v, want a SHA-256 mismatch", err)
	}
	if res.Updated || !res.Install.SelfManage || res.LatestVersion != "9.9.9" {
		t.Fatalf("res = %+v", res)
	}
	if rel.hits() != 1 {
		t.Fatalf("archive downloaded %d times, want 1", rel.hits())
	}

	after, err := os.ReadFile(inst.ExePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("the running binary was modified")
	}
	if got := dirEntries(t, dir); strings.Join(got, ",") != strings.Join(entriesBefore, ",") {
		t.Fatalf("directory changed: before %v, after %v", entriesBefore, got)
	}
}

func TestRunDefaultsToTheRealChannel(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res, err := Run(ctx, Options{CurrentVersion: "0.1.0", DevVersion: testDevVersion, BuildChannel: "release"})
	if err == nil || !strings.Contains(err.Error(), "query latest release") {
		t.Fatalf("err = %v, want a latest-release failure", err)
	}
	if res.Install.ExePath == "" {
		t.Fatal("Detect did not run")
	}
}
