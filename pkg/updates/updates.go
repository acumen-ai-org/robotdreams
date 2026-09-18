// Package updates defines the update contract: the JSON an announcement carries and the JSON a node reports back; data and validation only.
package updates

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/acumen-ai-org/robotdreams/pkg/messaging"
)

// SubjectUpdateAvailable is the Envelope.Subject of an update announcement.
const SubjectUpdateAvailable = "update available"

// KindCLI is the one reserved kind: the dream binary itself.
const KindCLI = "robotdreams/cli"

// KindNamespace is the reserved namespace; ValidateKind rejects any kind under it other than KindCLI.
const KindNamespace = "robotdreams"

// Severity levels an announcement may carry; the control plane records the level and never acts on it.
const (
	SeverityOptional    = "optional"
	SeverityRecommended = "recommended"
	SeverityRequired    = "required"
)

// Report statuses: the full vocabulary a node may use, plus StatusUnknown which only the server synthesizes.
const (
	// StatusCurrent is a node declaring the version of a kind it runs, with no AnnouncementID.
	StatusCurrent = "current"

	// StatusAcknowledged means the node has seen the announcement and considers it applicable, but has not started.
	StatusAcknowledged = "acknowledged"

	// StatusInProgress means the node is applying the update now.
	StatusInProgress = "in_progress"

	// StatusApplied means the node finished and CurrentVersion should now equal the announced version.
	StatusApplied = "applied"

	// StatusDeclined means the node saw the announcement and is deliberately not applying it; Detail says why.
	StatusDeclined = "declined"

	// StatusFailed means the node tried and it did not work; Detail says why.
	StatusFailed = "failed"

	// StatusUnknown is synthesized by the server for a node that has said nothing; ValidateStatus rejects it on the wire.
	StatusUnknown = "unknown"
)

// ErrInvalidKind is returned by ValidateKind for a malformed or reserved kind.
var ErrInvalidKind = errors.New("updates: invalid kind")

// ErrInvalidStatus is returned by ValidateStatus for an unknown status.
var ErrInvalidStatus = errors.New("updates: invalid status")

// ErrInvalidSeverity is returned by ValidateSeverity for an unknown severity.
var ErrInvalidSeverity = errors.New("updates: invalid severity")

// Announcement is what the control plane broadcasts as the Body of a messaging.TypeStatusUpdate envelope with Subject SubjectUpdateAvailable.
type Announcement struct {
	ID          string    `json:"announcement_id"`
	Kind        string    `json:"kind"`
	Version     string    `json:"version"`
	Source      string    `json:"source,omitempty"`
	MinVersion  string    `json:"min_version,omitempty"`
	Severity    string    `json:"severity,omitempty"`
	Notes       string    `json:"notes,omitempty"`
	AnnouncedBy string    `json:"announced_by"`
	AnnouncedAt time.Time `json:"announced_at"`
}

// Report is what a node says back about one kind: its current version, or its progress against one announcement.
type Report struct {
	Kind           string `json:"kind"`
	AnnouncementID string `json:"announcement_id,omitempty"`
	Status         string `json:"status"`
	CurrentVersion string `json:"current_version,omitempty"`
	TargetVersion  string `json:"target_version,omitempty"`
	Detail         string `json:"detail,omitempty"`
}

// Pending is one announcement a node has not reported StatusApplied or StatusDeclined against, with what it last said about the kind.
type Pending struct {
	Announcement   Announcement `json:"announcement"`
	CurrentVersion string       `json:"current_version,omitempty"`
	Status         string       `json:"status"`
}

// RolloutNode is one node's line in a rollout view.
type RolloutNode struct {
	WorkerID       string    `json:"worker_id"`
	WorkerStatus   string    `json:"worker_status"`
	Kind           string    `json:"kind"`
	CurrentVersion string    `json:"current_version,omitempty"`
	AnnouncementID string    `json:"announcement_id,omitempty"`
	TargetVersion  string    `json:"target_version,omitempty"`
	Status         string    `json:"status"`
	Detail         string    `json:"detail,omitempty"`
	ReportedAt     time.Time `json:"reported_at,omitempty"`
}

// Rollout is the fleet-wide view of one kind, optionally narrowed to one announcement; Counts includes StatusUnknown.
type Rollout struct {
	Kind         string         `json:"kind"`
	Announcement *Announcement  `json:"announcement,omitempty"`
	Counts       map[string]int `json:"counts"`
	Nodes        []RolloutNode  `json:"nodes"`
}

// ValidateKind reports whether kind is "<namespace>/<name>" in [a-z0-9._-] and not squatting the reserved namespace.
func ValidateKind(kind string) error {
	if kind == "" {
		return fmt.Errorf("%w: empty", ErrInvalidKind)
	}
	ns, name, found := strings.Cut(kind, "/")
	if !found {
		return fmt.Errorf("%w: %q has no %q separator, want <namespace>/<name>", ErrInvalidKind, kind, "/")
	}
	if ns == "" || name == "" {
		return fmt.Errorf("%w: %q has an empty namespace or name", ErrInvalidKind, kind)
	}
	if strings.Contains(name, "/") {
		return fmt.Errorf("%w: %q has more than one %q separator", ErrInvalidKind, kind, "/")
	}
	if err := validKindPart(ns, kind); err != nil {
		return err
	}
	if err := validKindPart(name, kind); err != nil {
		return err
	}
	if ns == KindNamespace && kind != KindCLI {
		return fmt.Errorf("%w: %q is under the reserved %q namespace (only %q is defined)",
			ErrInvalidKind, kind, KindNamespace+"/", KindCLI)
	}
	return nil
}

func validKindPart(part, kind string) error {
	for _, r := range part {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '.' || r == '_' || r == '-':
		default:
			return fmt.Errorf("%w: %q contains %q; allowed characters are a-z, 0-9, and ._-",
				ErrInvalidKind, kind, string(r))
		}
	}
	return nil
}

// ValidateStatus reports whether s is a status a node may report; StatusUnknown is not.
func ValidateStatus(s string) error {
	switch s {
	case StatusCurrent, StatusAcknowledged, StatusInProgress,
		StatusApplied, StatusDeclined, StatusFailed:
		return nil
	case StatusUnknown:
		return fmt.Errorf("%w: %q is synthesized by the control plane and cannot be reported", ErrInvalidStatus, s)
	case "":
		return fmt.Errorf("%w: empty", ErrInvalidStatus)
	default:
		return fmt.Errorf("%w: %q", ErrInvalidStatus, s)
	}
}

// ValidateSeverity reports whether s is a known severity or empty.
func ValidateSeverity(s string) error {
	switch s {
	case "", SeverityOptional, SeverityRecommended, SeverityRequired:
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrInvalidSeverity, s)
	}
}

// ParseAnnouncement extracts an Announcement from env, or returns false if env is not an announcement.
func ParseAnnouncement(env messaging.Envelope) (Announcement, bool) {
	if env.Type != messaging.TypeStatusUpdate || env.Subject != SubjectUpdateAvailable {
		return Announcement{}, false
	}
	var ann Announcement
	if err := json.Unmarshal(env.Body, &ann); err != nil {
		return Announcement{}, false
	}
	if ann.Kind == "" || ann.Version == "" {
		return Announcement{}, false
	}
	return ann, true
}
