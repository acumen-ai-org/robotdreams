package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/acumen-ai-org/robotdreams/pkg/updates"
)

var announcedAt = time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)

func testAnnouncement(id, kind, version string, at time.Time) updates.Announcement {
	return updates.Announcement{
		ID:          id,
		Kind:        kind,
		Version:     version,
		Source:      "https://example.test/" + version,
		Severity:    updates.SeverityRecommended,
		AnnouncedBy: "ops",
		AnnouncedAt: at,
	}
}

func TestInsertAndGetUpdateAnnouncement(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	want := testAnnouncement("a1", updates.KindCLI, "0.4.2", announcedAt)
	want.MinVersion = "0.3.0"
	want.Notes = "fixes clock skew"
	if err := s.InsertUpdateAnnouncement(ctx, want, 7); err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := s.GetUpdateAnnouncement(ctx, "a1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Kind != want.Kind || got.Version != want.Version || got.Source != want.Source ||
		got.MinVersion != want.MinVersion || got.Severity != want.Severity ||
		got.Notes != want.Notes || got.AnnouncedBy != want.AnnouncedBy {
		t.Fatalf("round-trip mismatch:\n got %+v\nwant %+v", got, want)
	}
	if !got.AnnouncedAt.Equal(want.AnnouncedAt) {
		t.Fatalf("announced_at = %v, want %v", got.AnnouncedAt, want.AnnouncedAt)
	}

	if _, err := s.GetUpdateAnnouncement(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get missing = %v, want ErrNotFound", err)
	}
}

func TestOpaqueVersionSurvivesStorage(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	for i, v := range []string{"0.4.2", "2026-09-01-g1a2b3c", "v4", "sha256:9f86d081"} {
		id := fmt.Sprintf("a%d", i)
		if err := s.InsertUpdateAnnouncement(ctx, testAnnouncement(id, "acme/pack", v, announcedAt), 1); err != nil {
			t.Fatalf("insert %q: %v", v, err)
		}
		got, err := s.GetUpdateAnnouncement(ctx, id)
		if err != nil {
			t.Fatalf("get %q: %v", v, err)
		}
		if got.Version != v {
			t.Fatalf("version %q came back as %q", v, got.Version)
		}
	}
}

func TestListUpdateAnnouncements(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	for i, spec := range []struct{ id, kind, version string }{
		{"a1", updates.KindCLI, "0.4.0"},
		{"a2", "acme/pack", "1"},
		{"a3", updates.KindCLI, "0.4.1"},
	} {
		at := announcedAt.Add(time.Duration(i) * time.Hour)
		if err := s.InsertUpdateAnnouncement(ctx, testAnnouncement(spec.id, spec.kind, spec.version, at), 3); err != nil {
			t.Fatalf("insert %s: %v", spec.id, err)
		}
	}

	all, err := s.ListUpdateAnnouncements(ctx, "", 0)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("list all returned %d, want 3", len(all))
	}
	if all[0].ID != "a3" || all[2].ID != "a1" {
		t.Fatalf("list order = %s,%s,%s, want a3,a2,a1", all[0].ID, all[1].ID, all[2].ID)
	}

	cli, err := s.ListUpdateAnnouncements(ctx, updates.KindCLI, 0)
	if err != nil {
		t.Fatalf("list by kind: %v", err)
	}
	if len(cli) != 2 {
		t.Fatalf("list by kind returned %d, want 2", len(cli))
	}

	limited, err := s.ListUpdateAnnouncements(ctx, "", 1)
	if err != nil {
		t.Fatalf("list limited: %v", err)
	}
	if len(limited) != 1 || limited[0].ID != "a3" {
		t.Fatalf("limited list = %+v, want just a3", limited)
	}
}

func TestUpdateAnnouncementRetention(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	if err := s.InsertUpdateAnnouncement(ctx, testAnnouncement("other", "acme/pack", "1", announcedAt), 1); err != nil {
		t.Fatalf("insert other kind: %v", err)
	}

	total := UpdateAnnouncementRetention + 5
	for i := 0; i < total; i++ {
		id := fmt.Sprintf("cli-%03d", i)
		at := announcedAt.Add(time.Duration(i) * time.Minute)
		if err := s.InsertUpdateAnnouncement(ctx, testAnnouncement(id, updates.KindCLI, "0.4."+id, at), 1); err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
	}

	cli, err := s.ListUpdateAnnouncements(ctx, updates.KindCLI, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(cli) != UpdateAnnouncementRetention {
		t.Fatalf("kept %d announcements, want %d", len(cli), UpdateAnnouncementRetention)
	}
	if cli[0].ID != fmt.Sprintf("cli-%03d", total-1) {
		t.Fatalf("newest kept is %q, want %q", cli[0].ID, fmt.Sprintf("cli-%03d", total-1))
	}
	for _, a := range cli {
		if a.ID == "cli-000" {
			t.Fatal("oldest announcement survived pruning")
		}
	}

	other, err := s.ListUpdateAnnouncements(ctx, "acme/pack", 0)
	if err != nil {
		t.Fatalf("list other: %v", err)
	}
	if len(other) != 1 {
		t.Fatalf("other kind has %d announcements, want 1 — pruning is not per kind", len(other))
	}
}

func TestUpsertWorkerUpdateStatePreservesAnnouncement(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	if err := s.UpsertWorkerUpdateState(ctx, WorkerUpdateState{
		WorkerID: "w1", Kind: updates.KindCLI,
		CurrentVersion: "0.4.1", AnnouncementID: "a1", TargetVersion: "0.4.2",
		Status: updates.StatusInProgress, ReportedAt: announcedAt,
	}); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	if err := s.UpsertWorkerUpdateState(ctx, WorkerUpdateState{
		WorkerID: "w1", Kind: updates.KindCLI,
		CurrentVersion: "0.4.2", Status: updates.StatusCurrent,
		ReportedAt: announcedAt.Add(time.Minute),
	}); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	got, err := s.ListWorkerUpdateState(ctx, WorkerUpdateFilter{WorkerID: "w1"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d rows, want 1", len(got))
	}
	st := got[0]
	if st.AnnouncementID != "a1" {
		t.Fatalf("announcement_id = %q, want it preserved as a1", st.AnnouncementID)
	}
	if st.TargetVersion != "0.4.2" {
		t.Fatalf("target_version = %q, want it preserved as 0.4.2", st.TargetVersion)
	}
	if st.CurrentVersion != "0.4.2" {
		t.Fatalf("current_version = %q, want the newly reported 0.4.2", st.CurrentVersion)
	}
	if st.Status != updates.StatusCurrent {
		t.Fatalf("status = %q, want it overwritten to %q", st.Status, updates.StatusCurrent)
	}
}

func TestUpsertWorkerUpdateStateIsPerKind(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	for _, spec := range []struct{ kind, version string }{
		{updates.KindCLI, "0.4.2"},
		{"acme/prompt-pack", "3"},
	} {
		if err := s.UpsertWorkerUpdateState(ctx, WorkerUpdateState{
			WorkerID: "w1", Kind: spec.kind, CurrentVersion: spec.version,
			Status: updates.StatusCurrent, ReportedAt: announcedAt,
		}); err != nil {
			t.Fatalf("upsert %s: %v", spec.kind, err)
		}
	}

	got, err := s.ListWorkerUpdateState(ctx, WorkerUpdateFilter{WorkerID: "w1"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2 — kinds are clobbering each other", len(got))
	}
	byKind := map[string]string{}
	for _, st := range got {
		byKind[st.Kind] = st.CurrentVersion
	}
	if byKind[updates.KindCLI] != "0.4.2" || byKind["acme/prompt-pack"] != "3" {
		t.Fatalf("per-kind versions wrong: %+v", byKind)
	}
}

func TestListWorkerUpdateStateFilters(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	seed := []WorkerUpdateState{
		{WorkerID: "w1", Kind: updates.KindCLI, AnnouncementID: "a1", Status: updates.StatusApplied},
		{WorkerID: "w1", Kind: "acme/pack", AnnouncementID: "a2", Status: updates.StatusApplied},
		{WorkerID: "w2", Kind: updates.KindCLI, AnnouncementID: "a1", Status: updates.StatusFailed},
		{WorkerID: "w3", Kind: updates.KindCLI, AnnouncementID: "a9", Status: updates.StatusApplied},
	}
	for _, st := range seed {
		st.ReportedAt = announcedAt
		if err := s.UpsertWorkerUpdateState(ctx, st); err != nil {
			t.Fatalf("seed %s/%s: %v", st.WorkerID, st.Kind, err)
		}
	}

	tests := []struct {
		name   string
		filter WorkerUpdateFilter
		want   int
	}{
		{"no filter", WorkerUpdateFilter{}, 4},
		{"by worker", WorkerUpdateFilter{WorkerID: "w1"}, 2},
		{"by kind", WorkerUpdateFilter{Kind: updates.KindCLI}, 3},
		{"by announcement", WorkerUpdateFilter{AnnouncementID: "a1"}, 2},
		{"kind and announcement", WorkerUpdateFilter{Kind: updates.KindCLI, AnnouncementID: "a1"}, 2},
		{"worker and kind", WorkerUpdateFilter{WorkerID: "w1", Kind: "acme/pack"}, 1},
		{"all three", WorkerUpdateFilter{WorkerID: "w2", Kind: updates.KindCLI, AnnouncementID: "a1"}, 1},
		{"no match", WorkerUpdateFilter{WorkerID: "nobody"}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := s.ListWorkerUpdateState(ctx, tc.filter)
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			if len(got) != tc.want {
				t.Fatalf("got %d rows, want %d", len(got), tc.want)
			}
		})
	}
}

func TestDeleteWorkerRemovesUpdateState(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	for _, id := range []string{"w1", "w2"} {
		if err := s.UpsertWorkerUpdateState(ctx, WorkerUpdateState{
			WorkerID: id, Kind: updates.KindCLI, CurrentVersion: "0.4.2",
			Status: updates.StatusCurrent, ReportedAt: announcedAt,
		}); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}

	if err := s.DeleteWorker(ctx, "w1"); err != nil {
		t.Fatalf("delete worker: %v", err)
	}

	got, err := s.ListWorkerUpdateState(ctx, WorkerUpdateFilter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].WorkerID != "w2" {
		t.Fatalf("after deleting w1, state rows are %+v; want only w2's", got)
	}
}
