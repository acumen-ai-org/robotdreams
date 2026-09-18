package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acumen-ai-org/robotdreams/internal/server"
)

func TestEnvURLSuppliesServerForConnect(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	addr, _ := startTestServerAt(t, filepath.Join(home, ".dream", "_server"))
	t.Setenv(envDreamURL, addr)

	res, err := runWorkerConnect(context.Background(), workerConnectOptions{
		WorkerID: "env-url-worker", Role: "tester",
	})
	if err != nil {
		t.Fatalf("connect via DREAM_URL: %v", err)
	}
	if res.WorkerID != "env-url-worker" {
		t.Fatalf("connected as %+v", res)
	}
}

const unroutableAddr = "127.0.0.1:1"

func TestServerFlagBeatsEnvURL(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	addr, _ := startTestServerAt(t, filepath.Join(home, ".dream", "_server"))
	t.Setenv(envDreamURL, unroutableAddr)

	res, err := runWorkerConnect(context.Background(), workerConnectOptions{
		Server: addr, WorkerID: "flag-beats-env", Role: "tester",
	})
	if err != nil {
		t.Fatalf("connect with explicit --server should ignore DREAM_URL: %v", err)
	}
	if res.WorkerID != "flag-beats-env" {
		t.Fatalf("connected as %+v", res)
	}
}

func TestConnectWithoutServerOrEnvFails(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	_, err := runWorkerConnect(context.Background(), workerConnectOptions{WorkerID: "nowhere"})
	if err == nil {
		t.Fatal("connect with no --server and no DREAM_URL should fail")
	}
	if !strings.Contains(err.Error(), envDreamURL) {
		t.Fatalf("error %q should mention %s", err, envDreamURL)
	}
}

func TestEnvURLBeatsLocalConfigScan(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	addr, _ := startTestServerAt(t, filepath.Join(home, ".dream", "_server"))

	if _, err := runWorkerConnect(context.Background(), workerConnectOptions{
		Server: addr, WorkerID: "scan-worker", Role: "tester",
	}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := saveLocalConfig("other-server_9999", localConfig{
		ServerAddr: "127.0.0.1:9999", WorkerID: "other",
	}); err != nil {
		t.Fatalf("saveLocalConfig: %v", err)
	}

	if _, err := resolveTarget("", "", ""); err == nil {
		t.Fatal("scan with two configs and no DREAM_URL should be ambiguous")
	}

	t.Setenv(envDreamURL, addr)
	target, err := resolveTarget("", "", "")
	if err != nil {
		t.Fatalf("resolveTarget with DREAM_URL: %v", err)
	}
	if target.workerID != "scan-worker" || target.baseURL != baseURLFromAddr(addr) {
		t.Fatalf("resolved %q at %q, want scan-worker at %q", target.workerID, target.baseURL, baseURLFromAddr(addr))
	}
}

func TestEnvTokenAuthorizesConnect(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	addr, srv := startTestServerAt(t, t.TempDir())
	t.Setenv(envDreamURL, addr)
	t.Setenv(envDreamToken, srv.EnrollmentToken())

	res, err := runWorkerConnect(context.Background(), workerConnectOptions{
		WorkerID: "env-token-worker", Role: "tester",
	})
	if err != nil {
		t.Fatalf("connect via DREAM_URL+DREAM_TOKEN only: %v", err)
	}
	if res.WorkerID != "env-token-worker" {
		t.Fatalf("connected as %+v", res)
	}
}

func TestAdminTokenFlagBeatsEnvToken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	addr, srv := startTestServerAt(t, t.TempDir())
	t.Setenv(envDreamToken, "garbage-env-token")

	res, err := runWorkerConnect(context.Background(), workerConnectOptions{
		Server: addr, WorkerID: "flag-token-worker", Role: "tester",
		AdminToken: srv.EnrollmentToken(),
	})
	if err != nil {
		t.Fatalf("connect with explicit --admin-token should ignore DREAM_TOKEN: %v", err)
	}
	if res.WorkerID != "flag-token-worker" {
		t.Fatalf("connected as %+v", res)
	}
}

func TestEnvTokenBeatsLocalDiscovery(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	addr, _ := startTestServerAt(t, filepath.Join(home, ".dream", "_server"))
	t.Setenv(envDreamToken, "garbage-env-token")

	_, err := runWorkerConnect(context.Background(), workerConnectOptions{
		Server: addr, WorkerID: "env-over-discovery", Role: "tester",
	})
	if err == nil {
		t.Fatal("a garbage DREAM_TOKEN must not fall through to local discovery")
	}
	var apiErr *apiError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("got %v, want an HTTP 401 *apiError", err)
	}
}

func TestNodeAliasResolves(t *testing.T) {
	root := newRootCmd()

	worker := findCommand(t, root, "worker")
	hasAlias := false
	for _, a := range worker.Aliases {
		if a == "node" {
			hasAlias = true
		}
	}
	if !hasAlias {
		t.Fatalf("worker command aliases %v do not include %q", worker.Aliases, "node")
	}

	for _, sub := range []string{"connect", "list", "reassign", "onboard"} {
		cmd, _, err := root.Find([]string{"node", sub})
		if err != nil {
			t.Fatalf("resolve `dream node %s`: %v", sub, err)
		}
		want := findCommand(t, root, "worker", sub)
		if cmd != want {
			t.Errorf("`dream node %s` resolved to %q, want the worker %s command", sub, cmd.CommandPath(), sub)
		}
	}
}

func TestNodeAliasEndToEnd(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	addr, _ := startTestServerAt(t, filepath.Join(home, ".dream", "_server"))
	t.Setenv(envDreamURL, addr)

	run := func(args ...string) string {
		t.Helper()
		root := newRootCmd()
		var buf bytes.Buffer
		root.SetOut(&buf)
		root.SetErr(&buf)
		root.SetArgs(args)
		if err := root.Execute(); err != nil {
			t.Fatalf("dream %s: %v\n%s", strings.Join(args, " "), err, buf.String())
		}
		return buf.String()
	}

	out := run("node", "connect", "--worker-id", "alias-node", "--role", "tester")
	if !strings.Contains(out, "connected alias-node") {
		t.Fatalf("node connect output: %q", out)
	}
	out = run("node", "list")
	if !strings.Contains(out, "alias-node") {
		t.Fatalf("node list output missing alias-node: %q", out)
	}
}

func TestSuggestedServerURL(t *testing.T) {
	cases := map[string]string{
		"127.0.0.1:7420": "http://127.0.0.1:7420",
		":7420":          "http://<this-host>:7420",
		"[::]:7420":      "http://<this-host>:7420",
		"0.0.0.0:7420":   "http://<this-host>:7420",
	}
	for in, want := range cases {
		if got := suggestedServerURL(in); got != want {
			t.Errorf("suggestedServerURL(%q) = %q, want %q", in, got, want)
		}
	}
}

var punchlineMustContain = []string{
	"Connect an agent from anywhere:",
	"any machine, any cloud",
	"DREAM_URL=",
	"DREAM_TOKEN",
	"dream node onboard",
	"npx robotdreams node onboard",
	"That command tells your agent everything it needs to know.",
}

func TestServerGuideCommand(t *testing.T) {
	dataDir := t.TempDir()
	tokenPath := filepath.Join(dataDir, "enrollment.token")
	if err := os.WriteFile(tokenPath, []byte("guide-test-token\n"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}

	root := newRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"server", "guide", "--data-dir", dataDir, "--server", "127.0.0.1:7420", "--show-token"})
	if err := root.Execute(); err != nil {
		t.Fatalf("server guide: %v", err)
	}

	out := buf.String()
	want := append([]string{
		"server setup guide",
		dataDir,
		"enrollment.token",
		"guide-test-token",
		"--messaging",
		"--storage",
		"--open-enrollment",
		"--no-dashboard",
		"http://127.0.0.1:7420/dashboard/",
		"DREAM_URL",
		"DREAM_TOKEN",
		"Precedence: explicit flag > environment variable > local auto-discovery",
	}, punchlineMustContain...)
	for _, s := range want {
		if !strings.Contains(out, s) {
			t.Errorf("guide output missing %q", s)
		}
	}
}

func TestStartupBannerEndsWithPunchline(t *testing.T) {
	dataDir := t.TempDir()
	srv, err := server.New(server.Config{DataDir: dataDir})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	defer srv.Close()

	var buf bytes.Buffer
	printStartupBanner(&buf, srv, dataDir, "127.0.0.1:7420", true, false)

	out := buf.String()
	for _, s := range append([]string{
		filepath.Join(dataDir, "enrollment.token"),
		"http://127.0.0.1:7420/dashboard/",
		"Full setup guide: dream server guide",
	}, punchlineMustContain...) {
		if !strings.Contains(out, s) {
			t.Errorf("startup banner missing %q", s)
		}
	}
	tail := out[strings.LastIndex(out, "That command tells your agent everything it needs to know."):]
	if !strings.Contains(tail, "Press Ctrl-C to stop.") {
		t.Errorf("punchline is not at the end of the banner:\n%s", out)
	}
}

func TestPunchlineOpenEnrollment(t *testing.T) {
	var buf bytes.Buffer
	printAgentPunchline(&buf, "http://127.0.0.1:7420", "/tmp/x/enrollment.token", true)
	out := buf.String()
	if !strings.Contains(out, "DREAM_TOKEN is not needed") {
		t.Errorf("open-enrollment punchline should say no token is needed:\n%s", out)
	}
	if strings.Contains(out, "/tmp/x/enrollment.token") {
		t.Errorf("open-enrollment punchline should not point at the token file:\n%s", out)
	}
}
