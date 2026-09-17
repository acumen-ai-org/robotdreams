package updates

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/acumen-ai-org/robotdreams/pkg/messaging"
)

func TestValidateKind(t *testing.T) {
	tests := []struct {
		name    string
		kind    string
		wantErr bool
	}{
		{"reserved cli kind", KindCLI, false},
		{"deployment kind", "acme/prompt-pack", false},
		{"shortest legal", "a/b", false},
		{"dots and underscores", "acme.co/model_weights-v2", false},
		{"digits", "acme2/pack9", false},

		{"empty", "", true},
		{"no separator", "cli", true},
		{"two separators", "a/b/c", true},
		{"empty namespace", "/x", true},
		{"empty name", "x/", true},
		{"uppercase namespace", "Acme/pack", true},
		{"uppercase name", "acme/Pack", true},
		{"trailing space", "acme/pack ", true},
		{"squats reserved namespace", "robotdreams/anything-else", true},
		{"reserved namespace bare", "robotdreams/", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateKind(tc.kind)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ValidateKind(%q) = nil, want an error", tc.kind)
				}
				if !errors.Is(err, ErrInvalidKind) {
					t.Fatalf("ValidateKind(%q) error %v, want ErrInvalidKind", tc.kind, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateKind(%q) = %v, want nil", tc.kind, err)
			}
		})
	}
}

func TestValidateStatus(t *testing.T) {
	tests := []struct {
		status  string
		wantErr bool
	}{
		{StatusCurrent, false},
		{StatusAcknowledged, false},
		{StatusInProgress, false},
		{StatusApplied, false},
		{StatusDeclined, false},
		{StatusFailed, false},

		// Synthesized by the control plane; a node cannot assert its own
		// silence.
		{StatusUnknown, true},
		{"", true},
		{"APPLIED", true},
		{"in-progress", true},
		{"done", true},
	}
	for _, tc := range tests {
		t.Run(tc.status, func(t *testing.T) {
			err := ValidateStatus(tc.status)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ValidateStatus(%q) = nil, want an error", tc.status)
				}
				if !errors.Is(err, ErrInvalidStatus) {
					t.Fatalf("ValidateStatus(%q) error %v, want ErrInvalidStatus", tc.status, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateStatus(%q) = %v, want nil", tc.status, err)
			}
		})
	}
}

func TestValidateSeverity(t *testing.T) {
	tests := []struct {
		severity string
		wantErr  bool
	}{
		{"", false}, // unstated is legal
		{SeverityOptional, false},
		{SeverityRecommended, false},
		{SeverityRequired, false},
		{"urgent", true},
		{"Required", true},
	}
	for _, tc := range tests {
		t.Run("severity="+tc.severity, func(t *testing.T) {
			err := ValidateSeverity(tc.severity)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ValidateSeverity(%q) = nil, want an error", tc.severity)
				}
				if !errors.Is(err, ErrInvalidSeverity) {
					t.Fatalf("ValidateSeverity(%q) error %v, want ErrInvalidSeverity", tc.severity, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateSeverity(%q) = %v, want nil", tc.severity, err)
			}
		})
	}
}

// TestAnnouncementWireFormat asserts the LITERAL JSON, field name by field
// name. This is a wire contract that node runtimes in other languages
// parse, so a struct-tag typo has to fail loudly here; a marshal/unmarshal
// round-trip would happily agree with itself and catch nothing.
func TestAnnouncementWireFormat(t *testing.T) {
	ann := Announcement{
		ID:          "4b0f2c9d1e7a4f38b6c2d5e1a9078f34",
		Kind:        KindCLI,
		Version:     "0.4.2",
		Source:      "https://example.test/releases/tag/v0.4.2",
		MinVersion:  "0.3.0",
		Severity:    SeverityRecommended,
		Notes:       "Fixes token refresh under clock skew.",
		AnnouncedBy: "ops-console",
		AnnouncedAt: time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC),
	}

	got, err := json.Marshal(ann)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"announcement_id":"4b0f2c9d1e7a4f38b6c2d5e1a9078f34",` +
		`"kind":"robotdreams/cli",` +
		`"version":"0.4.2",` +
		`"source":"https://example.test/releases/tag/v0.4.2",` +
		`"min_version":"0.3.0",` +
		`"severity":"recommended",` +
		`"notes":"Fixes token refresh under clock skew.",` +
		`"announced_by":"ops-console",` +
		`"announced_at":"2026-09-01T10:00:00Z"}`
	if string(got) != want {
		t.Fatalf("announcement JSON drifted from the contract\n got: %s\nwant: %s", got, want)
	}
}

// TestAnnouncementOmitsOptionalFields pins which fields disappear when
// unset: a node reading the contract must not have to distinguish "absent"
// from "empty" for the required ones.
func TestAnnouncementOmitsOptionalFields(t *testing.T) {
	got, err := json.Marshal(Announcement{
		ID:          "id1",
		Kind:        "acme/pack",
		Version:     "3",
		AnnouncedAt: time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"announcement_id":"id1","kind":"acme/pack","version":"3",` +
		`"announced_by":"","announced_at":"2026-09-01T10:00:00Z"}`
	if string(got) != want {
		t.Fatalf("announcement JSON drifted\n got: %s\nwant: %s", got, want)
	}
}

func TestReportWireFormat(t *testing.T) {
	got, err := json.Marshal(Report{
		Kind:           "acme/prompt-pack",
		AnnouncementID: "4b0f2c9d",
		Status:         StatusInProgress,
		CurrentVersion: "3",
		TargetVersion:  "4",
		Detail:         "draining; 2 tasks in flight",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"kind":"acme/prompt-pack","announcement_id":"4b0f2c9d",` +
		`"status":"in_progress","current_version":"3","target_version":"4",` +
		`"detail":"draining; 2 tasks in flight"}`
	if string(got) != want {
		t.Fatalf("report JSON drifted from the contract\n got: %s\nwant: %s", got, want)
	}

	// A bare version declaration carries no announcement.
	got, err = json.Marshal(Report{Kind: KindCLI, Status: StatusCurrent, CurrentVersion: "0.4.2"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want = `{"kind":"robotdreams/cli","status":"current","current_version":"0.4.2"}`
	if string(got) != want {
		t.Fatalf("bare current report JSON drifted\n got: %s\nwant: %s", got, want)
	}
}

// TestOpaqueVersionStrings is the executable form of "Robot Dreams never
// interprets a version": a deployment may version by semver, by date, by
// git describe, or by a bare integer, and every one must survive
// byte-identically.
func TestOpaqueVersionStrings(t *testing.T) {
	versions := []string{
		"0.4.2",
		"v4",
		"2026-09-01-g1a2b3c",
		"20260901.1",
		"1.0.0-rc.1+build.7",
		"sha256:9f86d081",
		"4",
	}
	for _, v := range versions {
		t.Run(v, func(t *testing.T) {
			b, err := json.Marshal(Announcement{ID: "i", Kind: "acme/pack", Version: v})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var back Announcement
			if err := json.Unmarshal(b, &back); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if back.Version != v {
				t.Fatalf("version %q round-tripped as %q", v, back.Version)
			}
		})
	}
}

func TestParseAnnouncement(t *testing.T) {
	body, err := json.Marshal(Announcement{ID: "i1", Kind: KindCLI, Version: "0.4.2"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	tests := []struct {
		name string
		env  messaging.Envelope
		want bool
	}{
		{
			name: "happy path",
			env:  messaging.Envelope{Type: messaging.TypeStatusUpdate, Subject: SubjectUpdateAvailable, Body: body},
			want: true,
		},
		{
			name: "wrong message type",
			env:  messaging.Envelope{Type: messaging.TypeEscalation, Subject: SubjectUpdateAvailable, Body: body},
			want: false,
		},
		{
			name: "wrong subject",
			env:  messaging.Envelope{Type: messaging.TypeStatusUpdate, Subject: "worker reassigned", Body: body},
			want: false,
		},
		{
			name: "malformed body",
			env:  messaging.Envelope{Type: messaging.TypeStatusUpdate, Subject: SubjectUpdateAvailable, Body: []byte("{oops")},
			want: false,
		},
		{
			name: "body missing kind",
			env:  messaging.Envelope{Type: messaging.TypeStatusUpdate, Subject: SubjectUpdateAvailable, Body: []byte(`{"version":"1"}`)},
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ann, ok := ParseAnnouncement(tc.env)
			if ok != tc.want {
				t.Fatalf("ParseAnnouncement ok = %v, want %v", ok, tc.want)
			}
			if ok && ann.Kind != KindCLI {
				t.Fatalf("parsed kind = %q, want %q", ann.Kind, KindCLI)
			}
		})
	}
}
