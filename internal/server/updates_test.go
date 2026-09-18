package server

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/acumen-ai-org/robotdreams/pkg/messaging"
	"github.com/acumen-ai-org/robotdreams/pkg/updates"
)

func inboxOf(t *testing.T, s *Server, workerID string) []messaging.Envelope {
	t.Helper()
	envs, err := s.Messaging().Tail(context.Background(), messaging.TailFilter{WorkerID: workerID})
	if err != nil {
		t.Fatalf("Tail(%q): %v", workerID, err)
	}
	return envs
}

func announcementsIn(envs []messaging.Envelope) []messaging.Envelope {
	var out []messaging.Envelope
	for _, e := range envs {
		if e.Subject == SubjectUpdateAvailable {
			out = append(out, e)
		}
	}
	return out
}

func TestAnnounceUpdateBroadcastsToEveryWorker(t *testing.T) {
	s := newTestServer(t, nil)
	ctx := context.Background()

	connect(t, s, "w1", "")
	connect(t, s, "w2", "w1")
	connect(t, s, "w3", "w1")

	ann, delivered, err := s.AnnounceUpdate(ctx, updates.Announcement{
		Kind:    updates.KindCLI,
		Version: "0.4.2",
		Source:  "https://example.test/v0.4.2",
	}, "ops")
	if err != nil {
		t.Fatalf("AnnounceUpdate: %v", err)
	}
	if delivered != 3 {
		t.Fatalf("delivered = %d, want 3", delivered)
	}

	var bodies [][]byte
	for _, id := range []string{"w1", "w2", "w3"} {
		got := announcementsIn(inboxOf(t, s, id))
		if len(got) != 1 {
			t.Fatalf("%s received %d announcements, want exactly 1", id, len(got))
		}
		env := got[0]

		if env.Type != messaging.TypeStatusUpdate {
			t.Fatalf("%s: envelope type = %q, want %q — the announcement must not introduce a new message type",
				id, env.Type, messaging.TypeStatusUpdate)
		}
		if env.From != ControlWorkerID {
			t.Fatalf("%s: From = %q, want %q", id, env.From, ControlWorkerID)
		}
		if env.To != id {
			t.Fatalf("%s: To = %q, want %q", id, env.To, id)
		}
		bodies = append(bodies, env.Body)
	}

	for i := 1; i < len(bodies); i++ {
		if string(bodies[i]) != string(bodies[0]) {
			t.Fatalf("announcement bodies differ between recipients:\n%s\n%s", bodies[0], bodies[i])
		}
	}

	var got updates.Announcement
	if err := json.Unmarshal(bodies[0], &got); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if got.ID != ann.ID || got.Version != "0.4.2" || got.Kind != updates.KindCLI {
		t.Fatalf("announced body = %+v, want it to match the returned announcement %+v", got, ann)
	}
}

func TestAnnounceUpdateDoesNotRouteToParent(t *testing.T) {
	s := newTestServer(t, nil)
	ctx := context.Background()

	connect(t, s, "lead", "")
	connect(t, s, "leaf", "lead")

	if _, _, err := s.AnnounceUpdate(ctx, updates.Announcement{
		Kind: updates.KindCLI, Version: "0.4.2",
	}, "ops"); err != nil {
		t.Fatalf("AnnounceUpdate: %v", err)
	}

	if n := len(announcementsIn(inboxOf(t, s, "leaf"))); n != 1 {
		t.Fatalf("leaf received %d announcements, want 1 — the broadcast was routed to a parent instead of the node", n)
	}
	if n := len(announcementsIn(inboxOf(t, s, "lead"))); n != 1 {
		t.Fatalf("lead received %d announcements, want exactly 1 (its own, not also its report's)", n)
	}
}

func TestAnnounceUpdateStampsServerOwnedFields(t *testing.T) {
	now := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	clock := newFakeClock(now)
	s := newTestServer(t, func(c *Config) { c.Clock = clock })
	connect(t, s, "w1", "")

	ann, _, err := s.AnnounceUpdate(context.Background(), updates.Announcement{
		Kind:    updates.KindCLI,
		Version: "0.4.2",

		ID:          "caller-supplied",
		AnnouncedBy: "impersonated",
		AnnouncedAt: now.Add(-99 * time.Hour),
	}, "ops")
	if err != nil {
		t.Fatalf("AnnounceUpdate: %v", err)
	}

	if ann.ID == "" || ann.ID == "caller-supplied" {
		t.Fatalf("ID = %q, want a server-assigned value", ann.ID)
	}
	if !ann.AnnouncedAt.Equal(now) {
		t.Fatalf("AnnouncedAt = %v, want the server clock's %v", ann.AnnouncedAt, now)
	}
	if ann.AnnouncedBy != "ops" {
		t.Fatalf("AnnouncedBy = %q, want the authenticated announcer %q", ann.AnnouncedBy, "ops")
	}
}

func TestAnnounceUpdateDefaultsAnnouncedByToControl(t *testing.T) {
	s := newTestServer(t, nil)
	connect(t, s, "w1", "")

	ann, _, err := s.AnnounceUpdate(context.Background(), updates.Announcement{
		Kind: updates.KindCLI, Version: "0.4.2",
	}, "")
	if err != nil {
		t.Fatalf("AnnounceUpdate: %v", err)
	}
	if ann.AnnouncedBy != ControlWorkerID {
		t.Fatalf("AnnouncedBy = %q, want %q", ann.AnnouncedBy, ControlWorkerID)
	}
}

func TestAnnounceUpdateRejectsInvalid(t *testing.T) {
	s := newTestServer(t, nil)
	connect(t, s, "w1", "")
	ctx := context.Background()

	tests := []struct {
		name string
		ann  updates.Announcement
	}{
		{"empty kind", updates.Announcement{Version: "1"}},
		{"malformed kind", updates.Announcement{Kind: "nope", Version: "1"}},
		{"reserved namespace", updates.Announcement{Kind: "robotdreams/other", Version: "1"}},
		{"empty version", updates.Announcement{Kind: updates.KindCLI}},
		{"unknown severity", updates.Announcement{Kind: updates.KindCLI, Version: "1", Severity: "urgent"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := s.AnnounceUpdate(ctx, tc.ann, "ops")
			if !errors.Is(err, ErrInvalidUpdate) {
				t.Fatalf("err = %v, want ErrInvalidUpdate", err)
			}
		})
	}

	got, err := s.LatestUpdateAnnouncements(ctx, "", 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("rejected announcements were still recorded: %+v", got)
	}
}

func TestRecordUpdateStateRejectsUnknownWorker(t *testing.T) {
	s := newTestServer(t, nil)
	err := s.RecordUpdateState(context.Background(), "ghost", updates.Report{
		Kind: updates.KindCLI, Status: updates.StatusCurrent, CurrentVersion: "0.4.2",
	})
	if err == nil {
		t.Fatal("recording state for an unregistered worker succeeded, want an error")
	}
}

func TestRecordUpdateStateRejectsInvalidReport(t *testing.T) {
	s := newTestServer(t, nil)
	connect(t, s, "w1", "")
	ctx := context.Background()

	tests := []struct {
		name string
		rep  updates.Report
	}{
		{"bad kind", updates.Report{Kind: "nope", Status: updates.StatusCurrent}},
		{"empty status", updates.Report{Kind: updates.KindCLI}},
		{"unknown status", updates.Report{Kind: updates.KindCLI, Status: "done"}},
		{"synthesized status", updates.Report{Kind: updates.KindCLI, Status: updates.StatusUnknown}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := s.RecordUpdateState(ctx, "w1", tc.rep); !errors.Is(err, ErrInvalidUpdate) {
				t.Fatalf("err = %v, want ErrInvalidUpdate", err)
			}
		})
	}
}

func TestRecordUpdateStateDoesNotSecondGuessTheNode(t *testing.T) {
	s := newTestServer(t, nil)
	connect(t, s, "w1", "")
	ctx := context.Background()

	if err := s.RecordUpdateState(ctx, "w1", updates.Report{
		Kind: "acme/never-announced", Status: updates.StatusCurrent, CurrentVersion: "7",
	}); err != nil {
		t.Fatalf("RecordUpdateState: %v", err)
	}

	if err := s.RecordUpdateState(ctx, "w1", updates.Report{
		Kind: updates.KindCLI, AnnouncementID: "no-such-announcement",
		Status: updates.StatusApplied, CurrentVersion: "0.4.2",
	}); err != nil {
		t.Fatalf("RecordUpdateState with unknown announcement: %v", err)
	}
}

func TestUpdateRolloutCountsNeverReportedAsUnknown(t *testing.T) {
	s := newTestServer(t, nil)
	ctx := context.Background()
	connect(t, s, "w1", "")
	connect(t, s, "w2", "w1")
	connect(t, s, "w3", "w1")

	ann, _, err := s.AnnounceUpdate(ctx, updates.Announcement{Kind: updates.KindCLI, Version: "0.4.2"}, "ops")
	if err != nil {
		t.Fatalf("AnnounceUpdate: %v", err)
	}

	if err := s.RecordUpdateState(ctx, "w1", updates.Report{
		Kind: updates.KindCLI, AnnouncementID: ann.ID,
		Status: updates.StatusApplied, CurrentVersion: "0.4.2",
	}); err != nil {
		t.Fatalf("report: %v", err)
	}

	rollout, err := s.UpdateRollout(ctx, updates.KindCLI, ann.ID)
	if err != nil {
		t.Fatalf("UpdateRollout: %v", err)
	}
	if rollout.Counts[updates.StatusApplied] != 1 {
		t.Fatalf("applied count = %d, want 1", rollout.Counts[updates.StatusApplied])
	}
	if rollout.Counts[updates.StatusUnknown] != 2 {
		t.Fatalf("unknown count = %d, want 2 — silent nodes must be visible", rollout.Counts[updates.StatusUnknown])
	}
	if len(rollout.Nodes) != 3 {
		t.Fatalf("rollout lists %d nodes, want all 3", len(rollout.Nodes))
	}
	if rollout.Announcement == nil || rollout.Announcement.ID != ann.ID {
		t.Fatalf("rollout announcement = %+v, want %s", rollout.Announcement, ann.ID)
	}
}

func TestUpdateRolloutIgnoresOtherAnnouncementsButKeepsVersion(t *testing.T) {
	s := newTestServer(t, nil)
	ctx := context.Background()
	connect(t, s, "w1", "")

	first, _, err := s.AnnounceUpdate(ctx, updates.Announcement{Kind: updates.KindCLI, Version: "0.4.1"}, "ops")
	if err != nil {
		t.Fatalf("first announce: %v", err)
	}
	if err := s.RecordUpdateState(ctx, "w1", updates.Report{
		Kind: updates.KindCLI, AnnouncementID: first.ID,
		Status: updates.StatusApplied, CurrentVersion: "0.4.1",
	}); err != nil {
		t.Fatalf("report: %v", err)
	}

	second, _, err := s.AnnounceUpdate(ctx, updates.Announcement{Kind: updates.KindCLI, Version: "0.4.2"}, "ops")
	if err != nil {
		t.Fatalf("second announce: %v", err)
	}

	rollout, err := s.UpdateRollout(ctx, updates.KindCLI, second.ID)
	if err != nil {
		t.Fatalf("UpdateRollout: %v", err)
	}
	if len(rollout.Nodes) != 1 {
		t.Fatalf("got %d nodes, want 1", len(rollout.Nodes))
	}
	n := rollout.Nodes[0]
	if n.Status != updates.StatusUnknown {
		t.Fatalf("status = %q, want unknown — the node answered about a different announcement", n.Status)
	}
	if n.CurrentVersion != "0.4.1" {
		t.Fatalf("current version = %q, want 0.4.1 carried through despite the unknown status", n.CurrentVersion)
	}
}

func TestUpdateRolloutCarriesWorkerStatus(t *testing.T) {
	s := newTestServer(t, nil)
	ctx := context.Background()
	connect(t, s, "w1", "")

	if _, _, err := s.AnnounceUpdate(ctx, updates.Announcement{Kind: updates.KindCLI, Version: "0.4.2"}, "ops"); err != nil {
		t.Fatalf("announce: %v", err)
	}
	rollout, err := s.UpdateRollout(ctx, updates.KindCLI, "")
	if err != nil {
		t.Fatalf("UpdateRollout: %v", err)
	}
	if rollout.Nodes[0].WorkerStatus == "" {
		t.Fatal("worker status is empty; an operator cannot tell a down node from a silent one")
	}
}

func TestPendingUpdatesFor(t *testing.T) {
	s := newTestServer(t, nil)
	ctx := context.Background()
	connect(t, s, "w1", "")

	ann, _, err := s.AnnounceUpdate(ctx, updates.Announcement{Kind: updates.KindCLI, Version: "0.4.2"}, "ops")
	if err != nil {
		t.Fatalf("announce: %v", err)
	}

	pending, err := s.PendingUpdatesFor(ctx, "w1")
	if err != nil {
		t.Fatalf("PendingUpdatesFor: %v", err)
	}
	if len(pending) != 1 || pending[0].Announcement.ID != ann.ID {
		t.Fatalf("pending = %+v, want the one announcement", pending)
	}
	if pending[0].Status != updates.StatusUnknown {
		t.Fatalf("status = %q, want unknown before the node has said anything", pending[0].Status)
	}

	if err := s.RecordUpdateState(ctx, "w1", updates.Report{
		Kind: updates.KindCLI, AnnouncementID: ann.ID, Status: updates.StatusAcknowledged,
	}); err != nil {
		t.Fatalf("acknowledge: %v", err)
	}
	pending, err = s.PendingUpdatesFor(ctx, "w1")
	if err != nil {
		t.Fatalf("PendingUpdatesFor: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("acknowledged announcement dropped out of pending; want it still listed")
	}
	if pending[0].Status != updates.StatusAcknowledged {
		t.Fatalf("status = %q, want acknowledged", pending[0].Status)
	}

	if err := s.RecordUpdateState(ctx, "w1", updates.Report{
		Kind: updates.KindCLI, AnnouncementID: ann.ID,
		Status: updates.StatusApplied, CurrentVersion: "0.4.2",
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	pending, err = s.PendingUpdatesFor(ctx, "w1")
	if err != nil {
		t.Fatalf("PendingUpdatesFor: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending = %+v, want empty after applying", pending)
	}
}

func TestPendingUpdatesForKeepsOnlyTheLatestPerKind(t *testing.T) {
	clock := newFakeClock(time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC))
	s := newTestServer(t, func(c *Config) { c.Clock = clock })
	ctx := context.Background()
	connect(t, s, "w1", "")

	for _, v := range []string{"0.4.0", "0.4.1", "0.4.2"} {
		if _, _, err := s.AnnounceUpdate(ctx, updates.Announcement{Kind: updates.KindCLI, Version: v}, "ops"); err != nil {
			t.Fatalf("announce %s: %v", v, err)
		}
		clock.Advance(time.Hour)
	}
	if _, _, err := s.AnnounceUpdate(ctx, updates.Announcement{Kind: "acme/pack", Version: "9"}, "ops"); err != nil {
		t.Fatalf("announce other kind: %v", err)
	}

	pending, err := s.PendingUpdatesFor(ctx, "w1")
	if err != nil {
		t.Fatalf("PendingUpdatesFor: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("got %d pending, want one per kind", len(pending))
	}
	byKind := map[string]string{}
	for _, p := range pending {
		byKind[p.Announcement.Kind] = p.Announcement.Version
	}
	if byKind[updates.KindCLI] != "0.4.2" {
		t.Fatalf("cli pending version = %q, want the latest 0.4.2", byKind[updates.KindCLI])
	}
	if byKind["acme/pack"] != "9" {
		t.Fatalf("acme pending version = %q, want 9", byKind["acme/pack"])
	}
}

type failingEmitBackend struct {
	messaging.MessagingBackend
	failFor string
}

func (b failingEmitBackend) Emit(ctx context.Context, env messaging.Envelope) error {
	if env.To == b.failFor {
		return errors.New("simulated transport failure")
	}
	return b.MessagingBackend.Emit(ctx, env)
}

func TestAnnounceUpdateContinuesPastEmitFailure(t *testing.T) {
	s := newTestServer(t, nil)
	ctx := context.Background()
	connect(t, s, "w1", "")
	connect(t, s, "w2", "w1")
	connect(t, s, "w3", "w1")

	s.messaging = failingEmitBackend{MessagingBackend: s.messaging, failFor: "w2"}

	ann, delivered, err := s.AnnounceUpdate(ctx, updates.Announcement{
		Kind: updates.KindCLI, Version: "0.4.2",
	}, "ops")
	if err == nil {
		t.Fatal("expected the per-recipient failure to be reported")
	}
	if delivered != 2 {
		t.Fatalf("delivered = %d, want 2 of 3", delivered)
	}

	if n := len(announcementsIn(inboxOf(t, s, "w3"))); n != 1 {
		t.Fatalf("w3 received %d announcements, want 1 — the fan-out aborted early", n)
	}

	got, err := s.LatestUpdateAnnouncements(ctx, updates.KindCLI, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].ID != ann.ID {
		t.Fatalf("announcement not recorded despite a partial fan-out: %+v", got)
	}
}

func TestUpdateRolloutWithoutAnnouncementIDScoresAgainstTheLatest(t *testing.T) {
	clock := newFakeClock(time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC))
	s := newTestServer(t, func(c *Config) { c.Clock = clock })
	ctx := context.Background()
	connect(t, s, "w1", "")

	first, _, err := s.AnnounceUpdate(ctx, updates.Announcement{Kind: updates.KindCLI, Version: "0.4.1"}, "ops")
	if err != nil {
		t.Fatalf("first announce: %v", err)
	}
	if err := s.RecordUpdateState(ctx, "w1", updates.Report{
		Kind: updates.KindCLI, AnnouncementID: first.ID,
		Status: updates.StatusApplied, CurrentVersion: "0.4.1",
	}); err != nil {
		t.Fatalf("report: %v", err)
	}

	clock.Advance(time.Hour)
	second, _, err := s.AnnounceUpdate(ctx, updates.Announcement{Kind: updates.KindCLI, Version: "0.4.2"}, "ops")
	if err != nil {
		t.Fatalf("second announce: %v", err)
	}

	rollout, err := s.UpdateRollout(ctx, updates.KindCLI, "")
	if err != nil {
		t.Fatalf("UpdateRollout: %v", err)
	}
	if rollout.Announcement == nil || rollout.Announcement.ID != second.ID {
		t.Fatalf("headline announcement = %+v, want the latest %s", rollout.Announcement, second.ID)
	}
	if got := rollout.Nodes[0].Status; got != updates.StatusUnknown {
		t.Fatalf("node status = %q under a headline naming %s, want %q — "+
			"the node applied %s and has said nothing about the newer one",
			got, second.ID, updates.StatusUnknown, first.ID)
	}
	if rollout.Counts[updates.StatusApplied] != 0 {
		t.Fatalf("applied = %d, want 0 — that count would be a false all-clear",
			rollout.Counts[updates.StatusApplied])
	}

	if rollout.Nodes[0].CurrentVersion != "0.4.1" {
		t.Fatalf("current version = %q, want 0.4.1 carried through", rollout.Nodes[0].CurrentVersion)
	}
}
