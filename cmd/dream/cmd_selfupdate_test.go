package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/acumen-ai-org/robotdreams/internal/selfupdate"
	"github.com/acumen-ai-org/robotdreams/internal/server"
	"github.com/acumen-ai-org/robotdreams/pkg/updates"
)

func TestDeclineReason(t *testing.T) {
	cases := []struct {
		name string
		res  selfupdate.Result
		want string
	}{
		{
			name: "with a fix",
			res:  selfupdate.Result{Install: selfupdate.Install{Method: selfupdate.MethodNPM, Fix: "npm install -g robotdreams@latest"}},
			want: "managed by npm; run: npm install -g robotdreams@latest",
		},
		{
			name: "without a fix",
			res:  selfupdate.Result{Install: selfupdate.Install{Method: selfupdate.MethodUnknown}},
			want: "managed by unknown",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := declineReason(tc.res); got != tc.want {
				t.Fatalf("declineReason = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSelfUpdateRefusesAfterReexec(t *testing.T) {
	t.Setenv(selfupdate.HopsEnv, "2")

	out, err := runDreamCmd(t, "self-update")
	if err == nil {
		t.Fatalf("self-update after a re-exec should refuse; output:\n%s", out)
	}
	if !strings.Contains(err.Error(), "already re-exec'd 2 time(s)") {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(out, "current  ") {
		t.Fatalf("the guard must fire before the update runs; output:\n%s", out)
	}
}

func TestSelfUpdateRefusesUnmanagedBinary(t *testing.T) {
	t.Setenv(selfupdate.HopsEnv, "")
	t.Setenv(selfupdate.InstallHintEnv, "")

	out, err := runDreamCmd(t, "self-update", "--version", "9.9.9")
	if err == nil {
		t.Fatalf("a test binary must refuse to self-update; output:\n%s", out)
	}
	for _, want := range []string{"current  " + version, "target   9.9.9", "refusing to replace a ", "-managed binary", "run:  "} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}

	t.Setenv(selfupdate.InstallHintEnv, string(selfupdate.MethodNPM))
	out, err = runDreamCmd(t, "self-update", "--version", "9.9.9")
	if err == nil || !strings.Contains(out, "refusing to replace a npm-managed binary") || !strings.Contains(out, "npm") {
		t.Fatalf("npm-hinted refusal: err = %v, output:\n%s", err, out)
	}

	out, err = runDreamCmd(t, "self-update", "--version", version)
	if err != nil || !strings.Contains(out, "dream is already up to date.") {
		t.Fatalf("same version: err = %v, output:\n%s", err, out)
	}
	out, err = runDreamCmd(t, "self-update", "--check", "--version", version)
	if err != nil || !strings.Contains(out, "dream is already up to date.") {
		t.Fatalf("--check same version: err = %v, output:\n%s", err, out)
	}

	out, err = runDreamCmd(t, "self-update", "--version", "0.0.1")
	if err == nil || !strings.Contains(err.Error(), "older than the installed") {
		t.Fatalf("downgrade: err = %v, output:\n%s", err, out)
	}

	out, err = runDreamCmd(t, "self-update", "--check", "--version", "9.9.9")
	if err != nil || !strings.Contains(out, "an update is available. run `dream self-update` to install it.") {
		t.Fatalf("--check newer: err = %v, output:\n%s", err, out)
	}
}

func selfUpdateNode(t *testing.T) (addr string, srv *server.Server) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv(selfupdate.HopsEnv, "")
	t.Setenv(selfupdate.InstallHintEnv, "")
	a, s := startTestServer(t)
	if _, err := runWorkerConnect(context.Background(), workerConnectOptions{Server: a, WorkerID: "node-1", Role: "worker"}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	return a, s
}

func TestApplyAnnouncedUpdateOutcomes(t *testing.T) {
	cases := []struct {
		name       string
		version    string
		wantStatus string
		wantOut    []string
		wantDetail string
	}{
		{
			name: "refused: not a release build", version: "9.9.9",
			wantStatus: updates.StatusDeclined,
			wantOut:    []string{"applying robotdreams/cli 9.9.9", "not applying it:", "run:  ", "still following"},
			wantDetail: "managed by ",
		},
		{
			name: "failed: a downgrade", version: "0.0.1",
			wantStatus: updates.StatusFailed,
			wantOut:    []string{"applying robotdreams/cli 0.0.1", "update failed:", "older than the installed", "still following"},
			wantDetail: "older than the installed",
		},
		{
			name: "already current", version: version,
			wantStatus: updates.StatusApplied,
			wantOut:    []string{"applying robotdreams/cli " + version, "already on that version; nothing to do."},
			wantDetail: "already running this version",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, srv := selfUpdateNode(t)
			ctx := context.Background()

			ann, _, err := srv.AnnounceUpdate(ctx, updates.Announcement{Kind: updates.KindCLI, Version: tc.version}, "ops")
			if err != nil {
				t.Fatalf("AnnounceUpdate: %v", err)
			}
			req := &updateRequest{Kind: updates.KindCLI, Version: tc.version, AnnouncementID: ann.ID}

			var out strings.Builder
			if err := applyAnnouncedUpdate(ctx, &out, req); err != nil {
				t.Fatalf("applyAnnouncedUpdate: %v", err)
			}
			for _, want := range tc.wantOut {
				if !strings.Contains(out.String(), want) {
					t.Errorf("narration missing %q:\n%s", want, out.String())
				}
			}

			rollout, err := srv.UpdateRollout(ctx, updates.KindCLI, ann.ID)
			if err != nil {
				t.Fatalf("UpdateRollout: %v", err)
			}
			if len(rollout.Nodes) != 1 {
				t.Fatalf("rollout nodes = %+v", rollout.Nodes)
			}
			node := rollout.Nodes[0]
			if node.WorkerID != "node-1" || node.Status != tc.wantStatus {
				t.Fatalf("node = %+v, want status %s", node, tc.wantStatus)
			}
			if !strings.Contains(node.Detail, tc.wantDetail) {
				t.Errorf("detail = %q, want it to contain %q", node.Detail, tc.wantDetail)
			}
			if tc.wantStatus == updates.StatusApplied && node.CurrentVersion != version {
				t.Errorf("applied report should carry the running version, got %q", node.CurrentVersion)
			}
		})
	}
}

func TestReportUpdateProgressAndAck(t *testing.T) {
	addr, srv := selfUpdateNode(t)
	ctx := context.Background()

	ann, _, err := srv.AnnounceUpdate(ctx, updates.Announcement{Kind: updates.KindCLI, Version: "9.9.9"}, "ops")
	if err != nil {
		t.Fatalf("AnnounceUpdate: %v", err)
	}

	envs, err := runMessageTail(ctx, messageTailOptions{Server: addr, AsWorker: "node-1"})
	if err != nil {
		t.Fatalf("tail: %v", err)
	}
	var msgID string
	for _, env := range envs {
		if req, ok := updateRequestFor(env); ok && req.AnnouncementID == ann.ID {
			msgID = env.ID
		}
	}
	if msgID == "" {
		t.Fatalf("announcement not in the inbox: %+v", envs)
	}
	req := &updateRequest{Kind: updates.KindCLI, Version: "9.9.9", AnnouncementID: ann.ID, MessageID: msgID}

	reportUpdateProgress(ctx, req, updates.StatusInProgress, "", "draining")
	rollout, err := srv.UpdateRollout(ctx, updates.KindCLI, ann.ID)
	if err != nil {
		t.Fatalf("UpdateRollout: %v", err)
	}
	if len(rollout.Nodes) != 1 || rollout.Nodes[0].Status != updates.StatusInProgress ||
		rollout.Nodes[0].TargetVersion != "9.9.9" || rollout.Nodes[0].Detail != "draining" ||
		rollout.Nodes[0].AnnouncementID != ann.ID {
		t.Fatalf("rollout after report = %+v", rollout.Nodes)
	}

	followCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	out, _, done := startFollow(followCtx, messageTailOptions{Server: addr, AsWorker: "node-1", JSON: true})
	waitForOutput(t, out, msgID, followTimeout)
	cancel()
	_ = waitForErr(t, done, followTimeout)

	ackUpdateMessage(ctx, req)

	followCtx2, cancel2 := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel2()
	out2, _, done2 := startFollow(followCtx2, messageTailOptions{Server: addr, AsWorker: "node-1", JSON: true})
	_ = waitForErr(t, done2, followTimeout)
	if strings.Contains(out2.String(), msgID) {
		t.Fatalf("the acked announcement was replayed:\n%s", out2.String())
	}

	ackUpdateMessage(ctx, &updateRequest{Kind: updates.KindCLI, Version: "9.9.9"})

	t.Setenv(envDreamURL, "127.0.0.1:1")
	reportUpdateProgress(ctx, req, updates.StatusFailed, "", "unreachable")
	ackUpdateMessage(ctx, req)
}
