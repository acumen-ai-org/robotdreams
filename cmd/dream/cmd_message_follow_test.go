package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/acumen-ai-org/robotdreams/pkg/updates"
)

const followTimeout = 10 * time.Second

func startFollow(ctx context.Context, opts messageTailOptions) (out, errOut *syncBuffer, done <-chan error) {
	out, errOut = &syncBuffer{}, &syncBuffer{}
	ch := make(chan error, 1)
	go func() { ch <- runMessageFollow(ctx, opts, out, errOut) }()
	return out, errOut, ch
}

func TestMessageFollowStreamsBacklogAndLive(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	addr, _ := startTestServer(t)
	ctx := context.Background()
	connectPair(t, addr)

	send := func(subject string) envelopeView {
		t.Helper()
		env, err := runMessageSend(ctx, messageSendOptions{Server: addr, AsWorker: "leaf", Type: "escalation", Subject: subject})
		if err != nil {
			t.Fatalf("send %q: %v", subject, err)
		}
		return env
	}
	send("disk full")

	followCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	out, errOut, done := startFollow(followCtx, messageTailOptions{Server: addr, AsWorker: "lead"})

	waitForOutput(t, out, "disk full", followTimeout)
	send("build broken")
	waitForOutput(t, out, "build broken", followTimeout)

	cancel()
	if err := waitForErr(t, done, followTimeout); err != nil {
		t.Fatalf("follow returned %v after cancel, want nil", err)
	}
	if errOut.String() != "" {
		t.Fatalf("unexpected stderr: %q", errOut.String())
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], "disk full") || !strings.Contains(lines[1], "build broken") {
		t.Fatalf("stream lines = %q", lines)
	}
	for _, l := range lines {
		if !strings.Contains(l, "escalation") || !strings.Contains(l, "from leaf") {
			t.Errorf("human line %q lacks the type or sender", l)
		}
	}
}

func TestMessageFollowJSONSkipsAckedMessages(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	addr, _ := startTestServer(t)
	ctx := context.Background()
	connectPair(t, addr)

	acked, err := runMessageSend(ctx, messageSendOptions{Server: addr, AsWorker: "leaf", Type: "escalation", Subject: "handled already"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if err := runMessageAck(ctx, messageAckOptions{Server: addr, AsWorker: "lead", MessageID: acked.ID, Action: "handled"}); err != nil {
		t.Fatalf("ack: %v", err)
	}
	pending, err := runMessageSend(ctx, messageSendOptions{Server: addr, AsWorker: "leaf", Type: "escalation", Subject: "still open"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	followCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	out, _, done := startFollow(followCtx, messageTailOptions{Server: addr, AsWorker: "lead", JSON: true})

	waitForOutput(t, out, pending.ID, followTimeout)
	cancel()
	if err := waitForErr(t, done, followTimeout); err != nil {
		t.Fatalf("follow returned %v, want nil", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want exactly the unacked message:\n%s", len(lines), out.String())
	}
	var env envelopeView
	if err := json.Unmarshal([]byte(lines[0]), &env); err != nil {
		t.Fatalf("--json line is not JSON: %v (%q)", err, lines[0])
	}
	if env.ID != pending.ID || env.Subject != "still open" || env.From != "leaf" || env.To != "lead" {
		t.Fatalf("envelope = %+v", env)
	}
	if strings.Contains(out.String(), acked.ID) {
		t.Fatalf("an acked message was replayed:\n%s", out.String())
	}
}

func TestMessageFollowWithoutIdentityFails(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	err := runMessageFollow(context.Background(), messageTailOptions{Since: "garbage"}, &syncBuffer{}, &syncBuffer{})
	if err == nil || !strings.Contains(err.Error(), "no connected server found") {
		t.Fatalf("got %v, want the no-connected-server error", err)
	}
}

func TestMessageFollowAnnouncementWithSelfUpdateOff(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	addr, srv := startTestServer(t)
	ctx := context.Background()
	connectPair(t, addr)

	followCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	out, errOut, done := startFollow(followCtx, messageTailOptions{Server: addr, AsWorker: "lead", JSON: true, SelfUpdate: false})

	if _, _, err := srv.AnnounceUpdate(ctx, updates.Announcement{Kind: updates.KindCLI, Version: "9.9.9"}, "ops"); err != nil {
		t.Fatalf("AnnounceUpdate: %v", err)
	}
	waitForOutput(t, errOut, "an update to robotdreams/cli 9.9.9 is available; run `dream self-update`", followTimeout)
	waitForOutput(t, out, updates.SubjectUpdateAvailable, followTimeout)

	if _, err := runMessageSend(ctx, messageSendOptions{Server: addr, AsWorker: "leaf", Type: "escalation", Subject: "after the announcement"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	waitForOutput(t, out, "after the announcement", followTimeout)

	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if !json.Valid([]byte(line)) {
			t.Errorf("non-JSON line on stdout under --json: %q", line)
		}
	}

	cancel()
	if err := waitForErr(t, done, followTimeout); err != nil {
		t.Fatalf("follow returned %v, want nil", err)
	}
}

func TestMessageFollowAnnouncementWithSelfUpdateOn(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ROBOTDREAMS_INSTALL_METHOD", "")
	addr, srv := startTestServer(t)
	ctx := context.Background()
	if _, err := runWorkerConnect(ctx, workerConnectOptions{Server: addr, WorkerID: "node-1", Role: "worker"}); err != nil {
		t.Fatalf("connect: %v", err)
	}

	followCtx, cancel := context.WithTimeout(ctx, followTimeout)
	defer cancel()
	out, errOut, done := startFollow(followCtx, messageTailOptions{Server: addr, AsWorker: "node-1", SelfUpdate: true})

	ann, _, err := srv.AnnounceUpdate(ctx, updates.Announcement{Kind: updates.KindCLI, Version: "9.9.9"}, "ops")
	if err != nil {
		t.Fatalf("AnnounceUpdate: %v", err)
	}

	if err := waitForErr(t, done, followTimeout); err != nil {
		t.Fatalf("follow returned %v, want nil after a refused update", err)
	}
	if followCtx.Err() != nil {
		t.Fatal("the follower only returned because the context timed out")
	}

	if !strings.Contains(out.String(), updates.SubjectUpdateAvailable) {
		t.Fatalf("announcement envelope not on stdout:\n%s", out.String())
	}
	for _, want := range []string{
		"applying robotdreams/cli 9.9.9 (announced by the control plane)",
		"not applying it:",
		"still following; this node keeps working on the version it has.",
	} {
		if !strings.Contains(errOut.String(), want) {
			t.Errorf("stderr missing %q:\n%s", want, errOut.String())
		}
	}
	if strings.Contains(out.String(), "applying") {
		t.Fatalf("update narration leaked onto stdout:\n%s", out.String())
	}

	rollout, err := srv.UpdateRollout(ctx, updates.KindCLI, ann.ID)
	if err != nil {
		t.Fatalf("UpdateRollout: %v", err)
	}
	if len(rollout.Nodes) != 1 {
		t.Fatalf("rollout nodes = %+v", rollout.Nodes)
	}
	node := rollout.Nodes[0]
	if node.WorkerID != "node-1" || node.Status != updates.StatusDeclined || node.TargetVersion != "9.9.9" {
		t.Fatalf("rollout node = %+v, want node-1 declined 9.9.9", node)
	}
	if !strings.Contains(node.Detail, "managed by ") {
		t.Fatalf("decline detail = %q, want the install method", node.Detail)
	}
	if rollout.Counts[updates.StatusDeclined] != 1 {
		t.Fatalf("counts = %+v", rollout.Counts)
	}
}
