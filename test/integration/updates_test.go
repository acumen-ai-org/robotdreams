//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/acumen-ai-org/robotdreams/internal/server"
	"github.com/acumen-ai-org/robotdreams/pkg/messaging"
	"github.com/acumen-ai-org/robotdreams/pkg/updates"
)

func announceUpdate(ctx context.Context, admin *adminActor, kind, version, severity string) (updates.Announcement, int, error) {
	body := map[string]string{"kind": kind, "version": version}
	if severity != "" {
		body["severity"] = severity
	}
	var out struct {
		Announcement updates.Announcement `json:"announcement"`
		Recipients   int                  `json:"recipients"`
		Delivered    int                  `json:"delivered"`
	}
	if err := authedJSON(ctx, admin.httpClientOf(), admin.base(), admin.authToken(),
		"POST", "/api/updates", body, &out); err != nil {
		return updates.Announcement{}, 0, err
	}
	return out.Announcement, out.Delivered, nil
}

func reportUpdate(ctx context.Context, w *worker, kind, announcementID, status, currentVersion, detail string) error {
	body := map[string]string{"kind": kind, "status": status}
	for k, v := range map[string]string{
		"announcement_id": announcementID,
		"current_version": currentVersion,
		"detail":          detail,
	} {
		if v != "" {
			body[k] = v
		}
	}
	return authedJSON(ctx, w.hc, w.baseURL, w.token(), "POST", "/api/updates/reports", body, nil)
}

func fetchRollout(ctx context.Context, admin *adminActor, kind string) (updates.Rollout, error) {
	var out updates.Rollout
	err := authedJSON(ctx, admin.httpClientOf(), admin.base(), admin.authToken(),
		"GET", "/api/updates/rollout?kind="+urlQueryEscape(kind), nil, &out)
	return out, err
}

func TestUpdateAnnouncementReachesEveryNodeAndRollsUp(t *testing.T) {
	ctx := context.Background()
	ts := startServer(t, server.Config{})
	admin := newAdminActor(t, ts)

	lead := connectWorker(ctx, t, ts, "lead", "lead", "", nil)
	a := connectWorker(ctx, t, ts, "node-a", "worker", "lead", nil)
	b := connectWorker(ctx, t, ts, "node-b", "worker", "lead", nil)

	ann, delivered, err := announceUpdate(ctx, admin, updates.KindCLI, "0.4.2", updates.SeverityRecommended)
	if err != nil {
		t.Fatalf("announce: %v", err)
	}
	if delivered != 3 {
		t.Fatalf("delivered = %d, want 3", delivered)
	}

	var bodies []string
	for _, w := range []*worker{lead, a, b} {
		envs, err := tail(ctx, w, w.id)
		if err != nil {
			t.Fatalf("tail %s: %v", w.id, err)
		}
		var found int
		for _, env := range envs {
			if env.Subject != updates.SubjectUpdateAvailable {
				continue
			}
			found++
			if env.Type != string(messaging.TypeStatusUpdate) {
				t.Fatalf("%s: announcement type = %q, want %q — a new message type would break every existing consumer",
					w.id, env.Type, messaging.TypeStatusUpdate)
			}
			if env.From != server.ControlWorkerID {
				t.Fatalf("%s: announcement from = %q, want %q", w.id, env.From, server.ControlWorkerID)
			}
			bodies = append(bodies, string(env.Body))
		}
		if found != 1 {
			t.Fatalf("%s received %d announcements, want exactly 1", w.id, found)
		}
	}
	for i := 1; i < len(bodies); i++ {
		if bodies[i] != bodies[0] {
			t.Fatalf("nodes received different announcement bodies:\n%s\n%s", bodies[0], bodies[i])
		}
	}

	var got updates.Announcement
	if err := json.Unmarshal([]byte(bodies[0]), &got); err != nil {
		t.Fatalf("unmarshal announcement: %v", err)
	}
	if got.ID != ann.ID || got.Version != "0.4.2" {
		t.Fatalf("announcement body = %+v, want id %s version 0.4.2", got, ann.ID)
	}

	if err := reportUpdate(ctx, lead, updates.KindCLI, ann.ID, updates.StatusApplied, "0.4.2", ""); err != nil {
		t.Fatalf("lead report: %v", err)
	}
	if err := reportUpdate(ctx, a, updates.KindCLI, ann.ID, updates.StatusApplied, "0.4.2", ""); err != nil {
		t.Fatalf("node-a report: %v", err)
	}
	if err := reportUpdate(ctx, b, updates.KindCLI, ann.ID, updates.StatusFailed, "0.4.1", "npm EACCES"); err != nil {
		t.Fatalf("node-b report: %v", err)
	}

	rollout, err := fetchRollout(ctx, admin, updates.KindCLI)
	if err != nil {
		t.Fatalf("rollout: %v", err)
	}
	if rollout.Counts[updates.StatusApplied] != 2 {
		t.Fatalf("applied = %d, want 2 (counts %+v)", rollout.Counts[updates.StatusApplied], rollout.Counts)
	}
	if rollout.Counts[updates.StatusFailed] != 1 {
		t.Fatalf("failed = %d, want 1 (counts %+v)", rollout.Counts[updates.StatusFailed], rollout.Counts)
	}
	if rollout.Counts[updates.StatusUnknown] != 0 {
		t.Fatalf("unknown = %d, want 0 — every node reported", rollout.Counts[updates.StatusUnknown])
	}

	for _, n := range rollout.Nodes {
		if n.WorkerStatus == "" {
			t.Fatalf("node %s carries no connectivity status", n.WorkerID)
		}
		if n.WorkerID == "node-b" && n.Detail != "npm EACCES" {
			t.Fatalf("node-b detail = %q, want the reported failure reason", n.Detail)
		}
	}
}

func TestUpdateAnnouncementWaitsForASilentNode(t *testing.T) {
	ctx := context.Background()
	ts := startServer(t, server.Config{})
	admin := newAdminActor(t, ts)

	talker := connectWorker(ctx, t, ts, "talker", "worker", "", nil)
	silent := connectWorker(ctx, t, ts, "silent", "worker", "", nil)

	ann, _, err := announceUpdate(ctx, admin, updates.KindCLI, "0.4.2", "")
	if err != nil {
		t.Fatalf("announce: %v", err)
	}
	if err := reportUpdate(ctx, talker, updates.KindCLI, ann.ID, updates.StatusApplied, "0.4.2", ""); err != nil {
		t.Fatalf("report: %v", err)
	}

	rollout, err := fetchRollout(ctx, admin, updates.KindCLI)
	if err != nil {
		t.Fatalf("rollout: %v", err)
	}
	if rollout.Counts[updates.StatusUnknown] != 1 {
		t.Fatalf("unknown = %d, want 1 — a node that said nothing must still be visible", rollout.Counts[updates.StatusUnknown])
	}

	envs, err := tail(ctx, silent, silent.id)
	if err != nil {
		t.Fatalf("tail: %v", err)
	}
	var found bool
	for _, env := range envs {
		if env.Subject == updates.SubjectUpdateAvailable {
			found = true
		}
	}
	if !found {
		t.Fatal("the announcement was not waiting for a node that never subscribed")
	}
}

func TestUpdateContractIsGenericAcrossKinds(t *testing.T) {
	ctx := context.Background()
	ts := startServer(t, server.Config{})
	admin := newAdminActor(t, ts)
	w := connectWorker(ctx, t, ts, "node-a", "worker", "", nil)

	const version = "2026-09-01-g1a2b3c"
	ann, delivered, err := announceUpdate(ctx, admin, "acme/prompt-pack", version, updates.SeverityRequired)
	if err != nil {
		t.Fatalf("announce: %v", err)
	}
	if delivered != 1 {
		t.Fatalf("delivered = %d, want 1", delivered)
	}
	if ann.Version != version {
		t.Fatalf("announced version = %q, want it stored verbatim as %q", ann.Version, version)
	}

	if err := reportUpdate(ctx, w, "acme/prompt-pack", ann.ID, updates.StatusDeclined, "2026-08-01-gdeadbee", "pinned by policy"); err != nil {
		t.Fatalf("report: %v", err)
	}

	rollout, err := fetchRollout(ctx, admin, "acme/prompt-pack")
	if err != nil {
		t.Fatalf("rollout: %v", err)
	}
	if rollout.Counts[updates.StatusDeclined] != 1 {
		t.Fatalf("declined = %d, want 1 — declining is a legitimate, recorded outcome", rollout.Counts[updates.StatusDeclined])
	}
	if rollout.Nodes[0].CurrentVersion != "2026-08-01-gdeadbee" {
		t.Fatalf("current version = %q, want the date-shaped version verbatim", rollout.Nodes[0].CurrentVersion)
	}
}

func TestAnnounceIsAdminOnly(t *testing.T) {
	ctx := context.Background()
	ts := startServer(t, server.Config{})
	w := connectWorker(ctx, t, ts, "node-a", "worker", "", nil)

	err := authedJSON(ctx, w.hc, w.baseURL, w.token(), "POST", "/api/updates",
		map[string]string{"kind": updates.KindCLI, "version": "0.4.2"}, nil)
	if err == nil {
		t.Fatal("a worker token announced an update, want a refusal")
	}
	var apiE *apiErr
	if !errors.As(err, &apiE) || apiE.Status != 403 {
		t.Fatalf("err = %v, want HTTP 403", err)
	}
}
