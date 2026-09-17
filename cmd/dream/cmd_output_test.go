package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/acumen-ai-org/robotdreams/pkg/updates"
)

func TestMessageTailAndAckCommandOutput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	addr, _ := startTestServer(t)
	ctx := context.Background()
	connectPair(t, addr)

	out := mustRunDreamCmd(t, "message", "tail", "--server", addr, "--worker-id", "lead")
	if strings.TrimSpace(out) != "no messages" {
		t.Fatalf("empty tail = %q", out)
	}
	out = mustRunDreamCmd(t, "message", "tail", "--server", addr, "--worker-id", "lead", "--json")
	if out != "" {
		t.Fatalf("empty --json tail must print nothing, got %q", out)
	}

	var ids []string
	for _, subject := range []string{"first", "second"} {
		env, err := runMessageSend(ctx, messageSendOptions{Server: addr, AsWorker: "leaf", Type: "escalation", Subject: subject, Body: `{"n":1}`})
		if err != nil {
			t.Fatalf("send: %v", err)
		}
		ids = append(ids, env.ID)
	}

	out = mustRunDreamCmd(t, "message", "tail", "--server", addr, "--worker-id", "lead")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 || !strings.Contains(out, "first") || !strings.Contains(out, "second") {
		t.Fatalf("tail lines = %q", lines)
	}
	for _, l := range lines {
		if !strings.Contains(l, `{"n":1}`) || !strings.Contains(l, "escalation") || !strings.Contains(l, "from leaf") {
			t.Fatalf("human line lacks body, type or sender: %q", l)
		}
	}

	out = mustRunDreamCmd(t, "message", "tail", "--server", addr, "--worker-id", "lead", "--json", "--limit", "1")
	lines = strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 {
		t.Fatalf("--limit 1 printed %d lines:\n%s", len(lines), out)
	}
	var env envelopeView
	if err := json.Unmarshal([]byte(lines[0]), &env); err != nil {
		t.Fatalf("--json line: %v (%q)", err, lines[0])
	}
	if env.ID != ids[0] && env.ID != ids[1] {
		t.Fatalf("--json id %q is neither sent message", env.ID)
	}

	if _, err := runDreamCmd(t, "message", "tail", "--server", "127.0.0.1:1", "--worker-id", "lead", "--since", "yesterday"); err == nil || !strings.Contains(err.Error(), "RFC3339") {
		t.Fatalf("bad --since: %v", err)
	}

	out = mustRunDreamCmd(t, "message", "ack", ids[0], "--server", addr, "--worker-id", "lead", "--action", "handled")
	if strings.TrimSpace(out) != "acked "+ids[0] {
		t.Fatalf("ack output = %q", out)
	}
	if _, err := runDreamCmd(t, "message", "ack", "no-such-message", "--server", addr, "--worker-id", "lead"); err == nil {
		t.Fatal("acking an unknown message through cobra should fail")
	}
}

func TestStoragePutAndWorkerReassignCommandOutput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	addr, _ := startTestServer(t)
	connectPair(t, addr)

	file := filepath.Join(t.TempDir(), "report.txt")
	if err := os.WriteFile(file, []byte("hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := mustRunDreamCmd(t, "storage", "put", "workers/leaf/report.txt", file, "--server", addr, "--worker-id", "leaf")
	if !strings.HasPrefix(out, "wrote workers/leaf/report.txt (6 bytes, revision ") {
		t.Fatalf("put output = %q", out)
	}
	rev := strings.TrimSuffix(strings.TrimSpace(out[strings.LastIndex(out, "revision ")+len("revision "):]), ")")

	root := newRootCmd()
	var buf strings.Builder
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetIn(strings.NewReader("hello again\n"))
	root.SetArgs([]string{"storage", "put", "workers/leaf/report.txt", "-", "--server", addr, "--worker-id", "leaf", "--if-match-revision", rev})
	if err := root.Execute(); err != nil {
		t.Fatalf("conditional put from stdin: %v\n%s", err, buf.String())
	}
	if !strings.Contains(buf.String(), "(12 bytes, revision ") || strings.Contains(buf.String(), rev) {
		t.Fatalf("second put output = %q", buf.String())
	}
	if _, err := runDreamCmd(t, "storage", "put", "workers/leaf/report.txt", file, "--server", addr, "--worker-id", "leaf", "--if-match-revision", rev); err == nil || !strings.Contains(err.Error(), "HTTP 409") {
		t.Fatalf("stale conditional put: %v", err)
	}
	if _, err := runDreamCmd(t, "storage", "put", "x", filepath.Join(t.TempDir(), "nope"), "--server", "127.0.0.1:1", "--worker-id", "leaf"); err == nil || !strings.Contains(err.Error(), "read ") {
		t.Fatalf("missing file: %v", err)
	}

	out = mustRunDreamCmd(t, "worker", "reassign", "leaf", "--server", addr, "--worker-id", "leaf", "--reports-to", "")
	if strings.TrimSpace(out) != "leaf now reports to (root)" {
		t.Fatalf("reassign to root output = %q", out)
	}
	out = mustRunDreamCmd(t, "worker", "reassign", "leaf", "--server", addr, "--worker-id", "leaf", "--reports-to", "lead")
	if strings.TrimSpace(out) != "leaf now reports to lead" {
		t.Fatalf("reassign output = %q", out)
	}
}

func TestUpdatesPendingRolloutContractCommandOutput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	addr, srv := startTestServer(t)
	ctx := context.Background()
	if _, err := runWorkerConnect(ctx, workerConnectOptions{Server: addr, WorkerID: "node-1", Role: "worker", Versions: map[string]string{"acme/prompt-pack": "3"}}); err != nil {
		t.Fatalf("connect: %v", err)
	}

	out := mustRunDreamCmd(t, "updates", "pending", "--server", addr, "--worker-id", "node-1")
	if strings.TrimSpace(out) != "nothing pending" {
		t.Fatalf("empty pending = %q", out)
	}

	adminTok, err := srv.MintAdminToken("ops", time.Hour)
	if err != nil {
		t.Fatalf("MintAdminToken: %v", err)
	}
	for _, a := range []struct{ kind, version string }{{"acme/prompt-pack", "4"}, {"acme/skill-pack", "2"}} {
		out = mustRunDreamCmd(t, "updates", "announce", "--server", addr, "--admin-token", adminTok.Raw, "--kind", a.kind, "--version", a.version)
		if !strings.Contains(out, a.kind) || !strings.Contains(out, a.version) {
			t.Fatalf("announce output = %q", out)
		}
	}

	out = mustRunDreamCmd(t, "updates", "pending", "--server", addr, "--worker-id", "node-1")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 || !strings.Contains(lines[0], "KIND") || !strings.Contains(lines[0], "MY STATUS") {
		t.Fatalf("pending table:\n%s", out)
	}
	if !strings.Contains(out, "acme/prompt-pack") || !strings.Contains(out, "acme/skill-pack") {
		t.Fatalf("pending table lacks a kind:\n%s", out)
	}
	for _, l := range lines[1:] {
		if strings.HasPrefix(l, "acme/prompt-pack") && !strings.Contains(l, "  3  ") {
			t.Errorf("prompt-pack row lacks the declared version 3: %q", l)
		}
	}
	out = mustRunDreamCmd(t, "updates", "pending", "--server", addr, "--worker-id", "node-1", "--quiet")
	if strings.TrimSpace(out) != "acme/prompt-pack\nacme/skill-pack" && strings.TrimSpace(out) != "acme/skill-pack\nacme/prompt-pack" {
		t.Fatalf("--quiet = %q", out)
	}
	out = mustRunDreamCmd(t, "updates", "pending", "--server", addr, "--worker-id", "node-1", "--quiet", "--kind", "acme/skill-pack")
	if strings.TrimSpace(out) != "acme/skill-pack" {
		t.Fatalf("--quiet --kind = %q", out)
	}
	if _, err := runDreamCmd(t, "updates", "pending", "--server", addr, "--worker-id", "node-1", "--worker", "someone-else"); err == nil {
		t.Fatal("reading another node's pending without admin should fail")
	}

	anns, err := runUpdatesList(ctx, updatesListOptions{updatesClientOptions: updatesClientOptions{Server: addr, AsWorker: "node-1"}, Kind: "acme/prompt-pack"})
	if err != nil || len(anns) != 1 {
		t.Fatalf("list prompt-pack announcements: %v, %+v", err, anns)
	}
	out = mustRunDreamCmd(t, "updates", "report", "--server", addr, "--worker-id", "node-1", "--kind", "acme/prompt-pack",
		"--status", updates.StatusApplied, "--current-version", "4", "--announcement-id", anns[0].ID)
	if strings.TrimSpace(out) != "reported acme/prompt-pack applied" {
		t.Fatalf("report output = %q", out)
	}
	out = mustRunDreamCmd(t, "updates", "rollout", "--server", addr, "--admin-token", adminTok.Raw, "--kind", "acme/prompt-pack")
	for _, want := range []string{"acme/prompt-pack", "node-1", updates.StatusApplied, "4"} {
		if !strings.Contains(out, want) {
			t.Errorf("rollout missing %q:\n%s", want, out)
		}
	}
	t.Setenv(envDreamToken, adminTok.Raw)
	out2 := mustRunDreamCmd(t, "updates", "rollout", "--server", addr, "--kind", "acme/prompt-pack")
	if out2 != out {
		t.Fatalf("rollout via DREAM_TOKEN differs:\n%s\nvs\n%s", out2, out)
	}
	t.Setenv(envDreamToken, "garbage")
	if _, err := runDreamCmd(t, "updates", "rollout", "--server", addr, "--kind", "acme/prompt-pack"); err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("garbage DREAM_TOKEN: %v", err)
	}

	out = mustRunDreamCmd(t, "updates", "contract")
	var sb strings.Builder
	printUpdateContract(&sb)
	if out != sb.String() || !strings.Contains(out, updates.KindCLI) {
		t.Fatalf("contract command output differs from printUpdateContract")
	}
}
