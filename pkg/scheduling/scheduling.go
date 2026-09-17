// Package scheduling defines a Schedule — a standing instruction to the
// control plane to send one message to one worker on a cron cadence — and
// the Store a control plane keeps them in.
//
// A schedule is deliberately not a job: the control plane does not know
// what the message means or what the recipient does with it. It fires the
// message; the recipient's own inbox handling is where the work starts.
// This keeps the control plane's knowledge to "who, when, what envelope"
// and leaves every deployment-specific meaning of the envelope (a
// workflow id, a report to compose) to the worker that registered it.
package scheduling

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// Schedule is one standing instruction.
type Schedule struct {
	// ID is client-chosen (PUT) or minted by the control plane (POST). It
	// becomes the CausationID of every message the schedule sends, so a
	// recipient can tell which schedule a message came from.
	ID string

	// Owner is the registered worker the message is sent From. It is the
	// identity that registered the schedule (or, for an admin-registered
	// schedule, the identity named at registration).
	Owner string

	// To is the registered worker the message is delivered to. Defaults
	// to Owner at registration: a worker scheduling a message to itself is
	// the ordinary case (a recurring task in its own inbox).
	To string

	// Cron is a five-field cron expression, optionally prefixed with
	// "TZ=<location> " (or "CRON_TZ=") — see ParseCron.
	Cron string

	// Subject and Body are copied verbatim onto every envelope sent.
	Subject string
	Body    json.RawMessage

	// Enabled is false for a schedule kept on file but not firing.
	Enabled bool

	// NextAt is the next time the schedule is due, in UTC.
	NextAt time.Time

	// LastAt is when the schedule last fired (nil if never).
	LastAt *time.Time

	CreatedAt time.Time
}

// Filter narrows a Store.List. Every non-empty field narrows further (the
// conditions are AND-ed); Worker matches a schedule the worker either
// owns or receives, which is what "a worker's own schedules" means at the
// API.
type Filter struct {
	Owner  string
	To     string
	Worker string
}

// Store is where a control plane keeps schedules. Implementations must be
// safe for concurrent use.
type Store interface {
	// Upsert inserts s, or replaces every field of an existing schedule
	// with the same ID except CreatedAt and LastAt, which record history
	// the caller does not own.
	Upsert(ctx context.Context, s Schedule) error

	// Get returns the schedule with the given ID, or an error wrapping
	// ErrNotFound.
	Get(ctx context.Context, id string) (Schedule, error)

	// List returns every schedule matching f, ordered by ID.
	List(ctx context.Context, f Filter) ([]Schedule, error)

	// Delete removes the schedule; deleting an unknown ID returns an
	// error wrapping ErrNotFound.
	Delete(ctx context.Context, id string) error

	// ListDue returns every enabled schedule whose NextAt is at or before
	// now, ordered by NextAt.
	ListDue(ctx context.Context, now time.Time) ([]Schedule, error)

	// MarkFired records that the schedule fired at lastAt and is next due
	// at nextAt.
	MarkFired(ctx context.Context, id string, lastAt, nextAt time.Time) error
}

// ErrNotFound is returned by Get, Delete and MarkFired for an unknown ID.
var ErrNotFound = errors.New("scheduling: not found")

// ErrInvalid is wrapped by every validation failure from Validate and
// ParseCron, so a caller can map the whole family to one status code.
var ErrInvalid = errors.New("scheduling: invalid schedule")

// cronParser is the standard five-field parser (minute hour dom month
// dow) with the "TZ=" / "CRON_TZ=" prefix and the "@daily"-style
// descriptors that robfig/cron's ParseStandard accepts. Seconds are
// deliberately not a field: a control plane polls for due schedules on a
// coarse tick, and minute resolution is what a cron line means to the
// people writing them.
var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

// ParseCron parses a five-field cron expression and returns a Next
// function: the first time strictly after its argument that the
// expression matches, in the expression's location (UTC unless a
// "TZ=<location> " prefix names one), returned in UTC. An expression that
// never matches yields the zero time from Next.
func ParseCron(expr string) (func(time.Time) time.Time, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, fmt.Errorf("%w: empty cron expression", ErrInvalid)
	}
	sched, err := cronParser.Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("%w: cron %q: %v", ErrInvalid, expr, err)
	}
	return func(t time.Time) time.Time {
		next := sched.Next(t)
		if next.IsZero() {
			return next
		}
		return next.UTC()
	}, nil
}

// Validate checks the fields a Store or a sender needs: an ID, an owner,
// a recipient, a parseable cron, and a body that is either empty or valid
// JSON. It does not check that Owner or To exist anywhere — the control
// plane owns the org chart and does that itself.
func (s Schedule) Validate() error {
	switch {
	case strings.TrimSpace(s.ID) == "":
		return fmt.Errorf("%w: empty id", ErrInvalid)
	case strings.ContainsAny(s.ID, "/ \t\n"):
		return fmt.Errorf("%w: id %q must not contain slashes or whitespace", ErrInvalid, s.ID)
	case s.Owner == "":
		return fmt.Errorf("%w: empty owner", ErrInvalid)
	case s.To == "":
		return fmt.Errorf("%w: empty recipient", ErrInvalid)
	}
	if _, err := ParseCron(s.Cron); err != nil {
		return err
	}
	if len(s.Body) > 0 && !json.Valid(s.Body) {
		return fmt.Errorf("%w: body is not valid JSON", ErrInvalid)
	}
	return nil
}
