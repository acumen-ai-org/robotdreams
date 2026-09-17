package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/acumen-ai-org/robotdreams/internal/server"
	"github.com/acumen-ai-org/robotdreams/pkg/updates"
)

func TestUpdateRequestFor(t *testing.T) {
	body, err := json.Marshal(updates.Announcement{
		ID: "a1", Kind: updates.KindCLI, Version: "0.4.2",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	otherKind, err := json.Marshal(updates.Announcement{
		ID: "a2", Kind: "acme/prompt-pack", Version: "3",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	tests := []struct {
		name string
		env  envelopeView
		want bool
	}{
		{
			name: "announcement for this tool",
			env:  envelopeView{ID: "m1", From: server.ControlWorkerID, Subject: updates.SubjectUpdateAvailable, Body: body},
			want: true,
		},
		{
			name: "another kind is not this CLI's business",
			env:  envelopeView{ID: "m2", From: server.ControlWorkerID, Subject: updates.SubjectUpdateAvailable, Body: otherKind},
			want: false,
		},
		{
			name: "not from the control plane",
			env:  envelopeView{ID: "m3", From: "some-worker", Subject: updates.SubjectUpdateAvailable, Body: body},
			want: false,
		},
		{
			name: "wrong subject",
			env:  envelopeView{ID: "m4", From: server.ControlWorkerID, Subject: "worker reassigned", Body: body},
			want: false,
		},
		{
			name: "unparseable body",
			env:  envelopeView{ID: "m5", From: server.ControlWorkerID, Subject: updates.SubjectUpdateAvailable, Body: []byte("{oops")},
			want: false,
		},
		{
			name: "no version",
			env:  envelopeView{ID: "m6", From: server.ControlWorkerID, Subject: updates.SubjectUpdateAvailable, Body: []byte(`{"kind":"robotdreams/cli"}`)},
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, ok := updateRequestFor(tc.env)
			if ok != tc.want {
				t.Fatalf("updateRequestFor ok = %v, want %v", ok, tc.want)
			}
			if ok {
				if req.Version != "0.4.2" || req.AnnouncementID != "a1" {
					t.Fatalf("req = %+v", req)
				}
				if req.MessageID != tc.env.ID {
					t.Fatalf("message ID = %q, want %q", req.MessageID, tc.env.ID)
				}
			}
		})
	}
}

func TestPrintUpdateContractCoversEveryStatus(t *testing.T) {
	var sb strings.Builder
	printUpdateContract(&sb)
	got := sb.String()

	for _, s := range []string{
		updates.StatusCurrent, updates.StatusAcknowledged, updates.StatusInProgress,
		updates.StatusApplied, updates.StatusDeclined, updates.StatusFailed,
	} {
		if !strings.Contains(got, s) {
			t.Errorf("the contract text never mentions status %q", s)
		}
	}
	for _, s := range []string{
		updates.KindCLI, updates.SubjectUpdateAvailable, server.ControlWorkerID,
		"announcement_id", "min_version", "severity",
	} {
		if !strings.Contains(got, s) {
			t.Errorf("the contract text never mentions %q", s)
		}
	}
	if !strings.Contains(got, "NOT enforced") {
		t.Error("the contract text does not say the procedure is unenforced")
	}
}

func TestPrintRollout(t *testing.T) {
	var sb strings.Builder
	printRollout(&sb, updates.Rollout{
		Kind: updates.KindCLI,
		Announcement: &updates.Announcement{
			ID: "a1", Kind: updates.KindCLI, Version: "0.4.2",
			AnnouncedAt: time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC),
		},
		Counts: map[string]int{updates.StatusApplied: 2, updates.StatusUnknown: 1},
		Nodes: []updates.RolloutNode{
			{WorkerID: "w1", WorkerStatus: "connected", CurrentVersion: "0.4.2", Status: updates.StatusApplied},
			{WorkerID: "w2", WorkerStatus: "connected", CurrentVersion: "0.4.2", Status: updates.StatusApplied},
			{WorkerID: "w3", WorkerStatus: "disconnected", Status: updates.StatusUnknown},
		},
	})
	got := sb.String()

	for _, want := range []string{"a1", "0.4.2", "applied 2", "unknown 1", "w3", "disconnected"} {
		if !strings.Contains(got, want) {
			t.Errorf("rollout output missing %q:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "CONNECTIVITY") {
		t.Errorf("rollout output has no connectivity column:\n%s", got)
	}
}

func TestPrintRolloutWithNothingAnnounced(t *testing.T) {
	var sb strings.Builder
	printRollout(&sb, updates.Rollout{Kind: "acme/pack", Counts: map[string]int{}})
	if !strings.Contains(sb.String(), "nothing announced yet") {
		t.Fatalf("unexpected output: %s", sb.String())
	}
}

func TestEncodeQuery(t *testing.T) {
	if got := encodeQuery(nil); got != "" {
		t.Fatalf("empty query = %q, want the empty string", got)
	}
}
