package scheduling

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestParseCronStandardFiveFields(t *testing.T) {
	next, err := ParseCron("0 9 * * *")
	if err != nil {
		t.Fatalf("ParseCron: %v", err)
	}
	from := time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC)
	got := next(from)
	want := time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("next after %s = %s, want %s", from, got, want)
	}
	onTheBoundary := want
	if got2 := next(onTheBoundary); !got2.Equal(onTheBoundary.Add(24 * time.Hour)) {
		t.Fatalf("next on the boundary = %s, want the day after", got2)
	}
}

func TestParseCronHonoursTZPrefix(t *testing.T) {
	next, err := ParseCron("TZ=Europe/Stockholm 0 9 * * 1")
	if err != nil {
		t.Fatalf("ParseCron: %v", err)
	}
	sundayNoonUTC := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	got := next(sundayNoonUTC)
	stockholm, err := time.LoadLocation("Europe/Stockholm")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	want := time.Date(2026, 9, 14, 9, 0, 0, 0, stockholm).UTC()
	if !got.Equal(want) {
		t.Fatalf("next = %s, want %s", got, want)
	}
	if got.Location() != time.UTC {
		t.Fatalf("next returned in %s, want UTC", got.Location())
	}
}

func TestParseCronRejectsGarbage(t *testing.T) {
	for _, expr := range []string{"", "every day", "0 9 * *", "61 9 * * *", "0 0 0 0 0 0"} {
		if _, err := ParseCron(expr); !errors.Is(err, ErrInvalid) {
			t.Errorf("ParseCron(%q) = %v, want ErrInvalid", expr, err)
		}
	}
}

func TestValidate(t *testing.T) {
	ok := Schedule{ID: "s1", Owner: "a", To: "b", Cron: "*/5 * * * *", Body: json.RawMessage(`{"k":1}`)}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid schedule rejected: %v", err)
	}
	bad := []Schedule{
		{Owner: "a", To: "b", Cron: "* * * * *"},
		{ID: "has space", Owner: "a", To: "b", Cron: "* * * * *"},
		{ID: "s", To: "b", Cron: "* * * * *"},
		{ID: "s", Owner: "a", Cron: "* * * * *"},
		{ID: "s", Owner: "a", To: "b", Cron: "nope"},
		{ID: "s", Owner: "a", To: "b", Cron: "* * * * *", Body: json.RawMessage(`{`)},
	}
	for i, s := range bad {
		if err := s.Validate(); !errors.Is(err, ErrInvalid) {
			t.Errorf("case %d: %v, want ErrInvalid", i, err)
		}
	}
}
