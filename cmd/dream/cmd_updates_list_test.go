package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/acumen-ai-org/robotdreams/pkg/updates"
)

func TestUpdatesListRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	addr, srv := startTestServer(t)
	ctx := context.Background()

	if _, err := runWorkerConnect(ctx, workerConnectOptions{Server: addr, WorkerID: "node-1", Role: "worker"}); err != nil {
		t.Fatalf("connect: %v", err)
	}

	out := mustRunDreamCmd(t, "updates", "list", "--server", addr, "--worker-id", "node-1")
	if strings.TrimSpace(out) != "no announcements" {
		t.Fatalf("empty list = %q", out)
	}

	adminTok, err := srv.MintAdminToken("ops", time.Hour)
	if err != nil {
		t.Fatalf("MintAdminToken: %v", err)
	}
	var ids []string
	for _, a := range []struct{ kind, version, severity string }{
		{"acme/prompt-pack", "4", updates.SeverityRecommended},
		{"acme/prompt-pack", "5", updates.SeverityRequired},
		{"acme/skill-pack", "1.2.0", ""},
	} {
		res, err := runUpdatesAnnounce(ctx, updatesAnnounceOptions{
			updatesAdminOptions: updatesAdminOptions{Addr: addr, AdminToken: adminTok.Raw},
			Kind:                a.kind, Version: a.version, Severity: a.severity,
		})
		if err != nil {
			t.Fatalf("announce %s %s: %v", a.kind, a.version, err)
		}
		ids = append(ids, res.Announcement.ID)
	}

	anns, err := runUpdatesList(ctx, updatesListOptions{updatesClientOptions: updatesClientOptions{Server: addr, AsWorker: "node-1"}})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(anns) != 3 {
		t.Fatalf("got %d announcements, want 3: %+v", len(anns), anns)
	}
	if anns[0].ID != ids[2] || anns[2].ID != ids[0] {
		t.Fatalf("not newest first: %v vs announced %v", []string{anns[0].ID, anns[1].ID, anns[2].ID}, ids)
	}

	anns, err = runUpdatesList(ctx, updatesListOptions{
		updatesClientOptions: updatesClientOptions{Server: addr, AsWorker: "node-1"}, Kind: "acme/prompt-pack",
	})
	if err != nil {
		t.Fatalf("list --kind: %v", err)
	}
	if len(anns) != 2 || anns[0].Version != "5" || anns[1].Version != "4" {
		t.Fatalf("--kind acme/prompt-pack = %+v", anns)
	}
	anns, err = runUpdatesList(ctx, updatesListOptions{
		updatesClientOptions: updatesClientOptions{Server: addr, AsWorker: "node-1"}, Limit: 1,
	})
	if err != nil {
		t.Fatalf("list --limit: %v", err)
	}
	if len(anns) != 1 || anns[0].Kind != "acme/skill-pack" {
		t.Fatalf("--limit 1 = %+v", anns)
	}

	out = mustRunDreamCmd(t, "updates", "list", "--server", addr, "--worker-id", "node-1")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 4 {
		t.Fatalf("table has %d lines, want header + 3:\n%s", len(lines), out)
	}
	for _, col := range []string{"KIND", "VERSION", "SEVERITY", "ANNOUNCED", "ID"} {
		if !strings.Contains(lines[0], col) {
			t.Errorf("header %q lacks %q", lines[0], col)
		}
	}
	for _, want := range []string{"acme/skill-pack", "1.2.0", "acme/prompt-pack", "required", "recommended"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(lines[1], "1.2.0") || !strings.Contains(lines[1], "  -  ") {
		t.Errorf("row without severity should carry a dash: %q", lines[1])
	}
	for _, id := range ids {
		if !strings.Contains(out, id) {
			t.Errorf("table lacks announcement id %s", id)
		}
	}
}

func TestUpdatesListWithoutIdentityFails(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, err := runUpdatesList(context.Background(), updatesListOptions{})
	if err == nil || !strings.Contains(err.Error(), "no connected server found") {
		t.Fatalf("got %v, want the no-connected-server error", err)
	}
}

func TestUpdateRequestError(t *testing.T) {
	req := &updateRequest{Kind: updates.KindCLI, Version: "0.4.2", AnnouncementID: "a1", MessageID: "m1"}
	if got := req.Error(); got != "update announced: robotdreams/cli 0.4.2" {
		t.Fatalf("Error() = %q", got)
	}
	var back *updateRequest
	if !errors.As(fmt.Errorf("stream: %w", req), &back) || back != req {
		t.Fatal("errors.As did not recover the *updateRequest through a wrapping")
	}
}
