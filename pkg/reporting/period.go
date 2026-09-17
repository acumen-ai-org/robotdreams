package reporting

import (
	"fmt"
	"strings"
	"time"
)

// PeriodKind is a calendar unit a report can be read over.
type PeriodKind string

// The period kinds, shortest first.
const (
	PeriodDay     PeriodKind = "day"
	PeriodWeek    PeriodKind = "week"
	PeriodMonth   PeriodKind = "month"
	PeriodQuarter PeriodKind = "quarter"
	PeriodYear    PeriodKind = "year"
)

// PeriodKinds lists every accepted PeriodKind, shortest first.
var PeriodKinds = []PeriodKind{PeriodDay, PeriodWeek, PeriodMonth, PeriodQuarter, PeriodYear}

// ParsePeriodKind parses a period kind name, case-insensitively and
// ignoring surrounding whitespace.
func ParsePeriodKind(s string) (PeriodKind, error) {
	k := PeriodKind(strings.ToLower(strings.TrimSpace(s)))
	for _, known := range PeriodKinds {
		if k == known {
			return k, nil
		}
	}
	names := make([]string, len(PeriodKinds))
	for i, known := range PeriodKinds {
		names[i] = string(known)
	}
	return "", fmt.Errorf("unknown period %q, want one of %s", s, strings.Join(names, "|"))
}

// Period is one calendar period: the half-open interval [Start, End) of
// wall-clock time in the location Start and End carry.
//
// Boundaries are calendar boundaries in that location, not fixed
// durations: a day that crosses a daylight-saving change is 23 or 25
// hours long, and a month is however many days it has. Contains is the
// one question a reader asks of it, and a produced_at instant either
// falls inside the interval or does not, whatever zone it was written in.
type Period struct {
	Kind  PeriodKind `json:"kind"`
	Start time.Time  `json:"start"`
	End   time.Time  `json:"end"`
}

// PeriodAt is the period of the given kind that contains the instant
// `at`, with boundaries taken in loc (nil means UTC).
//
// A week is an ISO week: it starts on Monday. A quarter starts on the
// first of January, April, July or October.
func PeriodAt(kind PeriodKind, at time.Time, loc *time.Location) Period {
	if loc == nil {
		loc = time.UTC
	}
	t := at.In(loc)
	y, m, d := t.Date()
	var start, end time.Time
	switch kind {
	case PeriodWeek:
		// Monday-based: Sunday is 6 days after Monday, not one before.
		offset := (int(t.Weekday()) + 6) % 7
		start = time.Date(y, m, d-offset, 0, 0, 0, 0, loc)
		end = time.Date(y, m, d-offset+7, 0, 0, 0, 0, loc)
	case PeriodMonth:
		start = time.Date(y, m, 1, 0, 0, 0, 0, loc)
		end = time.Date(y, m+1, 1, 0, 0, 0, 0, loc)
	case PeriodQuarter:
		qm := time.Month((int(m)-1)/3*3 + 1)
		start = time.Date(y, qm, 1, 0, 0, 0, 0, loc)
		end = time.Date(y, qm+3, 1, 0, 0, 0, 0, loc)
	case PeriodYear:
		start = time.Date(y, time.January, 1, 0, 0, 0, 0, loc)
		end = time.Date(y+1, time.January, 1, 0, 0, 0, 0, loc)
	default: // day, and anything unrecognised — a caller validates first
		kind = PeriodDay
		start = time.Date(y, m, d, 0, 0, 0, 0, loc)
		end = time.Date(y, m, d+1, 0, 0, 0, 0, loc)
	}
	return Period{Kind: kind, Start: start, End: end}
}

// Contains reports whether t falls inside the period: Start <= t < End.
func (p Period) Contains(t time.Time) bool {
	return !t.Before(p.Start) && t.Before(p.End)
}

// Previous is the period of the same kind immediately before this one.
func (p Period) Previous() Period {
	// An instant just before Start lies in the previous period, whatever
	// its length; asking PeriodAt for it keeps calendar arithmetic in one
	// place.
	return PeriodAt(p.Kind, p.Start.Add(-time.Nanosecond), p.Start.Location())
}

// Next is the period of the same kind immediately after this one.
func (p Period) Next() Period {
	return PeriodAt(p.Kind, p.End, p.Start.Location())
}

// String renders the period as "<kind> <start>..<end>" in RFC 3339.
func (p Period) String() string {
	return fmt.Sprintf("%s %s..%s", p.Kind, p.Start.Format(time.RFC3339), p.End.Format(time.RFC3339))
}
