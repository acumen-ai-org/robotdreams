package reporting

import (
	"strings"
	"testing"
	"time"
)

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Skipf("no tz database for %s: %v", name, err)
	}
	return loc
}

func TestParsePeriodKind(t *testing.T) {
	for raw, want := range map[string]PeriodKind{
		"day": PeriodDay, " Week ": PeriodWeek, "MONTH": PeriodMonth, "quarter": PeriodQuarter, "year": PeriodYear,
	} {
		got, err := ParsePeriodKind(raw)
		if err != nil || got != want {
			t.Errorf("ParsePeriodKind(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
	for _, raw := range []string{"", "bogus", "weeks", "hour"} {
		if _, err := ParsePeriodKind(raw); err == nil {
			t.Errorf("ParsePeriodKind(%q) accepted", raw)
		} else if !strings.Contains(err.Error(), "day|week|month|quarter|year") {
			t.Errorf("ParsePeriodKind(%q) error does not list the kinds: %v", raw, err)
		}
	}
}

// TestPeriodAtGoldens pins the boundaries for every kind at a few
// instants, in UTC. Sunday 2026-09-13 is the interesting weekday: a
// Sunday-based week would start it on itself, an ISO week puts it at the
// end of the week that started Monday the 7th.
func TestPeriodAtGoldens(t *testing.T) {
	cases := []struct {
		kind       PeriodKind
		at         string
		start, end string
	}{
		{PeriodDay, "2026-09-13T23:59:59Z", "2026-09-13T00:00:00Z", "2026-09-14T00:00:00Z"},
		{PeriodDay, "2026-09-13T00:00:00Z", "2026-09-13T00:00:00Z", "2026-09-14T00:00:00Z"},
		{PeriodWeek, "2026-09-13T12:00:00Z", "2026-09-07T00:00:00Z", "2026-09-14T00:00:00Z"}, // Sunday
		{PeriodWeek, "2026-09-14T00:00:00Z", "2026-09-14T00:00:00Z", "2026-09-21T00:00:00Z"}, // Monday
		{PeriodWeek, "2026-09-16T09:30:00Z", "2026-09-14T00:00:00Z", "2026-09-21T00:00:00Z"}, // Wednesday
		{PeriodWeek, "2026-01-01T00:00:00Z", "2025-12-29T00:00:00Z", "2026-01-05T00:00:00Z"}, // week across a year end
		{PeriodMonth, "2026-02-14T00:00:00Z", "2026-02-01T00:00:00Z", "2026-03-01T00:00:00Z"},
		{PeriodMonth, "2024-02-29T00:00:00Z", "2024-02-01T00:00:00Z", "2024-03-01T00:00:00Z"}, // leap day
		{PeriodMonth, "2026-12-31T23:00:00Z", "2026-12-01T00:00:00Z", "2027-01-01T00:00:00Z"},
		{PeriodQuarter, "2026-01-15T00:00:00Z", "2026-01-01T00:00:00Z", "2026-04-01T00:00:00Z"},
		{PeriodQuarter, "2026-03-31T23:59:59Z", "2026-01-01T00:00:00Z", "2026-04-01T00:00:00Z"},
		{PeriodQuarter, "2026-04-01T00:00:00Z", "2026-04-01T00:00:00Z", "2026-07-01T00:00:00Z"},
		{PeriodQuarter, "2026-09-13T00:00:00Z", "2026-07-01T00:00:00Z", "2026-10-01T00:00:00Z"},
		{PeriodQuarter, "2026-11-30T00:00:00Z", "2026-10-01T00:00:00Z", "2027-01-01T00:00:00Z"},
		{PeriodYear, "2026-09-13T00:00:00Z", "2026-01-01T00:00:00Z", "2027-01-01T00:00:00Z"},
		{PeriodYear, "2026-12-31T23:59:59Z", "2026-01-01T00:00:00Z", "2027-01-01T00:00:00Z"},
	}
	for _, c := range cases {
		at, _ := time.Parse(time.RFC3339, c.at)
		got := PeriodAt(c.kind, at, time.UTC)
		if got.Kind != c.kind {
			t.Errorf("PeriodAt(%s, %s).Kind = %q", c.kind, c.at, got.Kind)
		}
		if s := got.Start.Format(time.RFC3339); s != c.start {
			t.Errorf("PeriodAt(%s, %s).Start = %s, want %s", c.kind, c.at, s, c.start)
		}
		if e := got.End.Format(time.RFC3339); e != c.end {
			t.Errorf("PeriodAt(%s, %s).End = %s, want %s", c.kind, c.at, e, c.end)
		}
		if !got.Contains(at) {
			t.Errorf("PeriodAt(%s, %s) does not contain its own instant", c.kind, c.at)
		}
	}
}

// A nil location is UTC.
func TestPeriodAtNilLocation(t *testing.T) {
	at := time.Date(2026, 9, 13, 12, 0, 0, 0, time.FixedZone("x", 3600))
	got := PeriodAt(PeriodDay, at, nil)
	if got.Start.Location() != time.UTC || !got.Start.Equal(time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("nil loc: Start = %v, want 2026-09-13T00:00:00Z", got.Start)
	}
}

// Boundaries are wall-clock boundaries in the location, so the same
// instant lands in different days on either side of a date line — and
// a day in Stockholm starts at 22:00 UTC the evening before.
func TestPeriodAtNonUTCLocation(t *testing.T) {
	stockholm := mustLoc(t, "Europe/Stockholm")
	// 23:30 in Stockholm on Sunday the 13th is 21:30Z; UTC still says
	// Sunday, but Stockholm's Monday-start week must not have begun.
	at := time.Date(2026, 9, 13, 21, 30, 0, 0, time.UTC)

	day := PeriodAt(PeriodDay, at, stockholm)
	if want := time.Date(2026, 9, 13, 0, 0, 0, 0, stockholm); !day.Start.Equal(want) {
		t.Errorf("day.Start = %v, want %v", day.Start, want)
	}
	if want := time.Date(2026, 9, 13, 22, 0, 0, 0, time.UTC); !day.End.Equal(want) {
		t.Errorf("day.End = %v, want %v (Stockholm midnight, CEST)", day.End, want)
	}

	// Ten minutes past Stockholm midnight is still Sunday in UTC but
	// Monday the 14th in Stockholm: a new day AND a new ISO week.
	later := time.Date(2026, 9, 13, 22, 10, 0, 0, time.UTC)
	if PeriodAt(PeriodDay, later, time.UTC).Start.Day() != 13 {
		t.Errorf("UTC day should still be the 13th")
	}
	if got := PeriodAt(PeriodDay, later, stockholm).Start; got.Day() != 14 {
		t.Errorf("Stockholm day should be the 14th, got %v", got)
	}
	week := PeriodAt(PeriodWeek, later, stockholm)
	if want := time.Date(2026, 9, 14, 0, 0, 0, 0, stockholm); !week.Start.Equal(want) {
		t.Errorf("Stockholm week.Start = %v, want %v", week.Start, want)
	}
	if week.Start.Weekday() != time.Monday {
		t.Errorf("week starts on %v, want Monday", week.Start.Weekday())
	}

	// A fixed offset, which is what an RFC 3339 `at` carries.
	plus2 := time.FixedZone("+02:00", 2*3600)
	if got := PeriodAt(PeriodMonth, later, plus2); got.Start.Month() != time.September || got.Start.Day() != 1 || got.Start.Location() != plus2 {
		t.Errorf("fixed-offset month.Start = %v", got.Start)
	}
}

// A day across a daylight-saving change is 23 or 25 hours long, and the
// boundaries are still local midnights.
func TestPeriodAtDSTCrossingDay(t *testing.T) {
	stockholm := mustLoc(t, "Europe/Stockholm")

	// 2026-03-29: clocks go forward at 02:00 CET -> 03:00 CEST.
	spring := PeriodAt(PeriodDay, time.Date(2026, 3, 29, 12, 0, 0, 0, stockholm), stockholm)
	if d := spring.End.Sub(spring.Start); d != 23*time.Hour {
		t.Errorf("spring-forward day is %v long, want 23h", d)
	}
	if h, m, s := spring.Start.Clock(); h != 0 || m != 0 || s != 0 {
		t.Errorf("spring.Start is not local midnight: %v", spring.Start)
	}
	if h, m, s := spring.End.Clock(); h != 0 || m != 0 || s != 0 {
		t.Errorf("spring.End is not local midnight: %v", spring.End)
	}

	// 2026-10-25: clocks go back at 03:00 CEST -> 02:00 CET.
	fall := PeriodAt(PeriodDay, time.Date(2026, 10, 25, 12, 0, 0, 0, stockholm), stockholm)
	if d := fall.End.Sub(fall.Start); d != 25*time.Hour {
		t.Errorf("fall-back day is %v long, want 25h", d)
	}

	// The week and month containing the change are still bounded at
	// local midnights on the right dates, and the periods chain.
	week := PeriodAt(PeriodWeek, time.Date(2026, 3, 29, 12, 0, 0, 0, stockholm), stockholm)
	if week.Start.Day() != 23 || week.End.Day() != 30 || week.Start.Weekday() != time.Monday {
		t.Errorf("DST week = %v", week)
	}
	if d := week.End.Sub(week.Start); d != 7*24*time.Hour-time.Hour {
		t.Errorf("DST week is %v long, want 167h", d)
	}
	if !week.Next().Start.Equal(week.End) || !week.Previous().End.Equal(week.Start) {
		t.Errorf("Previous/Next do not chain across DST: %v / %v / %v", week.Previous(), week, week.Next())
	}
}

func TestPeriodPreviousNextContains(t *testing.T) {
	at := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	for _, kind := range PeriodKinds {
		p := PeriodAt(kind, at, time.UTC)
		prev, next := p.Previous(), p.Next()
		if prev.Kind != kind || next.Kind != kind {
			t.Errorf("%s: kinds drift: %s / %s", kind, prev.Kind, next.Kind)
		}
		if !prev.End.Equal(p.Start) || !next.Start.Equal(p.End) {
			t.Errorf("%s: periods do not chain: %v | %v | %v", kind, prev, p, next)
		}
		if prev.Contains(p.Start) || !p.Contains(p.Start) {
			t.Errorf("%s: Start belongs to the period, not the previous one", kind)
		}
		if p.Contains(p.End) || !next.Contains(p.End) {
			t.Errorf("%s: End belongs to the next period, not this one", kind)
		}
		if p.Contains(p.End.Add(-time.Nanosecond)) != true {
			t.Errorf("%s: the last nanosecond is inside", kind)
		}
		// Stepping back and forward lands on the same period.
		if back := next.Previous(); !back.Start.Equal(p.Start) || !back.End.Equal(p.End) {
			t.Errorf("%s: Next().Previous() = %v, want %v", kind, back, p)
		}
	}

	// Quarter and year steps cross year ends.
	q1 := PeriodAt(PeriodQuarter, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), time.UTC)
	if prev := q1.Previous(); prev.Start.Year() != 2025 || prev.Start.Month() != time.October {
		t.Errorf("Q1.Previous() = %v, want Q4 2025", prev)
	}
	y := PeriodAt(PeriodYear, at, time.UTC)
	if next := y.Next(); next.Start.Year() != 2027 || next.End.Year() != 2028 {
		t.Errorf("year.Next() = %v", next)
	}
}

func TestPeriodString(t *testing.T) {
	p := PeriodAt(PeriodWeek, time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC), time.UTC)
	if got, want := p.String(), "week 2026-09-07T00:00:00Z..2026-09-14T00:00:00Z"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
