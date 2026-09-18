package main

import (
	"bytes"
	"strings"
	"testing"
)

func runOnboard(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := newRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(append([]string{"node", "onboard"}, args...))
	err := root.Execute()
	return buf.String(), err
}

var onboardMustContain = []string{
	"node onboarding brief",
	"messaging",
	"storage",
	"library",
	"substrate",
	"dream node connect --worker-id <your-node-id> --role <your-role> --reports-to <parent-node-id>",
	"dream node list",
	"--type status_update",
	"--type completed_work",
	"--type escalation",
	"--type request_for_input",
	"--storage-path shared/<your-node-id>/report.md",
	"dream message tail --limit 20",
	"dream message tail --follow",
	"dream storage put <path> <local-file>",
	"dream storage get <path> <local-file>",
	"dream storage ls [prefix]",
	"workers/<your-node-id>/*",
	"shared/*",
	"put before send",
	"Token refresh is automatic",
}

func TestOnboardRequiresURL(t *testing.T) {
	t.Setenv(envDreamURL, "")

	out, err := runOnboard(t)
	if err == nil {
		t.Fatal("onboard with no DREAM_URL should fail")
	}
	if !strings.Contains(err.Error(), envDreamURL) {
		t.Fatalf("error %q should mention %s", err, envDreamURL)
	}
	if strings.Contains(out, "onboarding brief") {
		t.Fatalf("no brief should print on the error path:\n%s", out)
	}
}

func TestOnboardReachable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	addr, _ := startTestServer(t)
	t.Setenv(envDreamURL, addr)
	t.Setenv(envDreamToken, "some-token")

	out, err := runOnboard(t)
	if err != nil {
		t.Fatalf("onboard: %v\n%s", err, out)
	}
	for _, s := range append([]string{"REACHABLE (health check passed)"}, onboardMustContain...) {
		if !strings.Contains(out, s) {
			t.Errorf("onboard output missing %q", s)
		}
	}
	if strings.Contains(out, "NOT set") {
		t.Errorf("DREAM_TOKEN is set; the brief should not warn it is missing:\n%s", out)
	}
}

func TestOnboardUnreachable(t *testing.T) {
	t.Setenv(envDreamURL, "127.0.0.1:1")

	out, err := runOnboard(t)
	if err != nil {
		t.Fatalf("onboard against an unreachable server should still succeed: %v", err)
	}
	if !strings.Contains(out, "UNREACHABLE") {
		t.Errorf("output should warn the server is unreachable:\n%s", out)
	}
	for _, s := range onboardMustContain {
		if !strings.Contains(out, s) {
			t.Errorf("onboard output missing %q despite unreachable server", s)
		}
	}
	if !strings.Contains(out, "NOT set") {
		t.Errorf("brief should flag the missing DREAM_TOKEN:\n%s", out)
	}
}
