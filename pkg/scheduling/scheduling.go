// Package scheduling defines a Schedule, a standing instruction to send one message to one worker on a cron cadence, and the Store that keeps them.
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
	ID        string
	Owner     string
	To        string
	Cron      string
	Subject   string
	Body      json.RawMessage
	Enabled   bool
	NextAt    time.Time
	LastAt    *time.Time
	CreatedAt time.Time
}

// Filter narrows a Store.List; every non-empty field narrows further, and Worker matches a schedule the worker owns or receives.
type Filter struct {
	Owner  string
	To     string
	Worker string
}

// Store is where a control plane keeps schedules; implementations must be safe for concurrent use.
type Store interface {
	// Upsert inserts s, or replaces every field of the schedule with the same ID except CreatedAt and LastAt.
	Upsert(ctx context.Context, s Schedule) error

	// Get returns the schedule with the given ID, or an error wrapping ErrNotFound.
	Get(ctx context.Context, id string) (Schedule, error)

	// List returns every schedule matching f, ordered by ID.
	List(ctx context.Context, f Filter) ([]Schedule, error)

	// Delete removes the schedule, or returns an error wrapping ErrNotFound.
	Delete(ctx context.Context, id string) error

	// ListDue returns every enabled schedule whose NextAt is at or before now, ordered by NextAt.
	ListDue(ctx context.Context, now time.Time) ([]Schedule, error)

	// MarkFired records that the schedule fired at lastAt and is next due at nextAt.
	MarkFired(ctx context.Context, id string, lastAt, nextAt time.Time) error
}

// ErrNotFound is returned by Get, Delete and MarkFired for an unknown ID.
var ErrNotFound = errors.New("scheduling: not found")

// ErrInvalid is wrapped by every validation failure from Validate and ParseCron.
var ErrInvalid = errors.New("scheduling: invalid schedule")

var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

// ParseCron parses a five-field cron expression into a function returning the next match strictly after its argument, in UTC, or the zero time if none.
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

// Validate checks ID, Owner, To, Cron and Body for shape; it does not check that Owner or To are registered.
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
