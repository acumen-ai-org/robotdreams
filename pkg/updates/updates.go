// Package updates defines the Robot Dreams update BASE CONTRACT: the JSON
// shape of an update announcement the control plane broadcasts, and the
// JSON shape of what a node says back about it.
//
// This package is DATA AND VALIDATION ONLY, deliberately and permanently.
// It contains no downloader, no supervisor, no work-drain machinery and no
// handler registry, and none may be added. docs/vision/core.md's rule is
// that Robot Dreams is "100% agnostic about what a node's runtime is";
// shipping any part of the apply-an-update procedure as code would make
// Robot Dreams an agent runtime, which is the one thing it is not. The
// procedure ("stop taking new work, let in-flight work finish, apply,
// restart, resume") is documented guidance in docs/updates.md and in
// `dream node onboard` — the node executes it, however it likes, or not.
//
// A node does not have to import this package at all: the contract is
// plain JSON on a plain messaging.TypeStatusUpdate envelope, and a bash
// node with jq is a first-class consumer.
package updates

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/acumen-ai-org/robotdreams/pkg/messaging"
)

// SubjectUpdateAvailable is the Envelope.Subject of an update
// announcement. It is the node's cheap pre-filter: a node that does not
// care about updates can skip the body parse on subject alone.
const SubjectUpdateAvailable = "update available"

// KindCLI is the one reserved kind: the `dream` binary itself, the only
// thing Robot Dreams ships and can therefore speak about with authority.
// Every other kind belongs to a deployment (e.g. "acme/prompt-pack") and
// Robot Dreams neither defines nor interprets it.
const KindCLI = "robotdreams/cli"

// KindNamespace is the reserved namespace. ValidateKind rejects any kind
// under "robotdreams/" other than the ones this package defines, so a
// deployment cannot squat the vendor namespace and have its meaning drift
// from a future release's.
const KindNamespace = "robotdreams"

// Severity levels an announcement may carry. Severity is what turns
// "update now" versus "update at next idle" into a NODE-side policy
// decision rather than a Robot Dreams policy: the control plane records
// the level and never acts on it.
const (
	SeverityOptional    = "optional"
	SeverityRecommended = "recommended"
	SeverityRequired    = "required"
)

// Report statuses. These are the full vocabulary a node may use.
const (
	// StatusCurrent is not about any announcement: it is a node simply
	// declaring what version of a kind it is running. AnnouncementID is
	// empty. This is what makes version reporting and update reporting
	// ONE mechanism instead of two — see docs/updates.md.
	StatusCurrent = "current"

	// StatusAcknowledged means the node has seen the announcement and
	// considers it applicable, but has not started.
	StatusAcknowledged = "acknowledged"

	// StatusInProgress means the node is applying the update now.
	StatusInProgress = "in_progress"

	// StatusApplied means the node finished; CurrentVersion should now
	// equal the announced version.
	StatusApplied = "applied"

	// StatusDeclined means the node saw the announcement and is
	// deliberately not applying it. Detail says why. This is a
	// legitimate answer, not a failure.
	StatusDeclined = "declined"

	// StatusFailed means the node tried and it did not work. Detail says
	// why.
	StatusFailed = "failed"

	// StatusUnknown is NEVER reported by a node — ValidateStatus rejects
	// it on the wire. The server synthesizes it in a rollout view for a
	// connected node that has said nothing at all.
	StatusUnknown = "unknown"
)

// ErrInvalidKind is returned by ValidateKind for a malformed or reserved
// kind.
var ErrInvalidKind = errors.New("updates: invalid kind")

// ErrInvalidStatus is returned by ValidateStatus for an unknown status.
var ErrInvalidStatus = errors.New("updates: invalid status")

// ErrInvalidSeverity is returned by ValidateSeverity for an unknown
// severity.
var ErrInvalidSeverity = errors.New("updates: invalid severity")

// Announcement is what the control plane broadcasts: "a version of this
// kind is available." It is carried as the Body of an ordinary
// messaging.TypeStatusUpdate envelope from the control worker, with
// Subject SubjectUpdateAvailable.
//
// Robot Dreams never downloads Source, never compares Version against
// anything, and never decides which nodes an announcement is "for". Every
// connected node receives every announcement and filters on Kind itself.
type Announcement struct {
	// ID identifies this announcement. It is what a Report joins back
	// to, and it is server-assigned — never supplied by a caller. The
	// same version can legitimately be re-announced (a node was offline,
	// a rollout was restarted) and the two rollouts must be separately
	// countable, which is why Version cannot serve as this key.
	ID string `json:"announcement_id"`

	// Kind names what is being updated, as "<namespace>/<name>". This is
	// the entire routing mechanism: the fan-out is unconditional and the
	// node decides whether the kind applies to it. KindCLI is reserved.
	Kind string `json:"kind"`

	// Version is the available version, as an OPAQUE string. Robot
	// Dreams never parses, orders or compares it — it does not know
	// whether a deployment versions by semver, by date, or by git SHA.
	Version string `json:"version"`

	// Source optionally locates the artifact: a release URL, an OCI
	// image reference, an npm tag, a storage path. It is free-form and
	// Robot Dreams never fetches it; it exists so an announcement is
	// actionable without an out-of-band channel.
	Source string `json:"source,omitempty"`

	// MinVersion optionally advises that nodes below this version should
	// update. It is advisory and interpreted ONLY by the node, which
	// knows its own kind's versioning scheme. No server code path reads
	// this field, deliberately — see the package doc comment.
	MinVersion string `json:"min_version,omitempty"`

	// Severity is optional/recommended/required, or empty. Advisory:
	// the node decides what to do about it.
	Severity string `json:"severity,omitempty"`

	// Notes is free text for a human or an agent — typically the
	// deployment's own drain-and-restart guidance for this kind. It is
	// the escape hatch that keeps this schema from growing a field for
	// every deployment's procedure.
	Notes string `json:"notes,omitempty"`

	// AnnouncedBy identifies who announced it, and AnnouncedAt when.
	// Both are server-stamped, mirroring reassigned_by on the
	// reassignment control message.
	AnnouncedBy string    `json:"announced_by"`
	AnnouncedAt time.Time `json:"announced_at"`
}

// Report is what a node says back about one kind — either its current
// version (StatusCurrent, no AnnouncementID) or its progress against a
// specific announcement.
//
// Nothing here is kind-specific: CurrentVersion and TargetVersion are
// opaque strings, so a deployment announcing "acme/prompt-pack" gets
// exactly the same rollout tracking as robotdreams/cli with no server
// changes at all.
type Report struct {
	// Kind is the kind this report is about.
	Kind string `json:"kind"`

	// AnnouncementID names the announcement being reported against.
	// Empty for a bare StatusCurrent version declaration.
	AnnouncementID string `json:"announcement_id,omitempty"`

	// Status is one of the Status* constants except StatusUnknown,
	// which the server synthesizes and never accepts.
	Status string `json:"status"`

	// CurrentVersion is the version of Kind the node is running now.
	CurrentVersion string `json:"current_version,omitempty"`

	// TargetVersion is the version the node is moving toward, when that
	// differs from CurrentVersion.
	TargetVersion string `json:"target_version,omitempty"`

	// Detail is free text explaining the status — why an update was
	// declined, how a failure failed, what is still in flight.
	Detail string `json:"detail,omitempty"`
}

// Pending is one announcement a node has not yet resolved, paired with
// what that node last said about the kind. "Resolved" means the node
// reported StatusApplied or StatusDeclined against this exact
// announcement; everything else — silence, acknowledged, in_progress, a
// failure it may retry — is still pending.
//
// This is a catch-up convenience, not an authority: the announcement is
// already sitting in the node's durable inbox, and a node is free never to
// ask for this.
type Pending struct {
	Announcement Announcement `json:"announcement"`

	// CurrentVersion is what the node last reported for this kind, if
	// anything. Empty when it has never said.
	CurrentVersion string `json:"current_version,omitempty"`

	// Status is the node's last reported status FOR THIS announcement, or
	// StatusUnknown if it has said nothing about this one.
	Status string `json:"status"`
}

// RolloutNode is one node's line in a rollout view.
type RolloutNode struct {
	WorkerID string `json:"worker_id"`

	// WorkerStatus is the node's orgchart connectivity status. It is not
	// decoration: because announcements are delivered durably to nodes
	// that are currently disconnected, this is what lets an operator
	// tell "not answering because it is down" from "not answering
	// because it is ignoring me."
	WorkerStatus string `json:"worker_status"`

	Kind           string    `json:"kind"`
	CurrentVersion string    `json:"current_version,omitempty"`
	AnnouncementID string    `json:"announcement_id,omitempty"`
	TargetVersion  string    `json:"target_version,omitempty"`
	Status         string    `json:"status"`
	Detail         string    `json:"detail,omitempty"`
	ReportedAt     time.Time `json:"reported_at,omitempty"`
}

// Rollout is the fleet-wide view of one kind, optionally narrowed to one
// announcement. Counts includes StatusUnknown for nodes that have said
// nothing.
type Rollout struct {
	Kind         string         `json:"kind"`
	Announcement *Announcement  `json:"announcement,omitempty"`
	Counts       map[string]int `json:"counts"`
	Nodes        []RolloutNode  `json:"nodes"`
}

// ValidateKind reports whether kind is a well-formed namespaced kind:
// exactly one "/", both halves non-empty, and characters limited to
// [a-z0-9._-] on each side. Lowercase is required rather than merely
// conventional, so "Acme/Pack" and "acme/pack" cannot become two kinds
// that look like one in a rollout table.
//
// The "robotdreams/" namespace is reserved: any kind under it other than
// those this package defines is rejected, so a deployment cannot squat
// the vendor namespace and then collide with a future release.
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

// validKindPart enforces the [a-z0-9._-] character class on one half of a
// kind.
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

// ValidateStatus reports whether s is a status a node may report.
// StatusUnknown is rejected: the server synthesizes it for a node that
// has said nothing, so a node claiming it would be asserting its own
// silence.
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

// ValidateSeverity reports whether s is a known severity. An empty
// severity is valid and means "unstated".
func ValidateSeverity(s string) error {
	switch s {
	case "", SeverityOptional, SeverityRecommended, SeverityRequired:
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrInvalidSeverity, s)
	}
}

// ParseAnnouncement extracts an Announcement from env, returning
// ok=false if env is not an announcement at all — wrong message type,
// wrong subject, or a body that does not parse.
//
// It is a convenience for Go nodes. The contract is the JSON, not this
// function: a node in any language matches on the same two fields and
// decodes the same body.
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
