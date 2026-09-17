package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/acumen-ai-org/robotdreams/pkg/scheduling"
)

func TestScheduleRoundtripAndUpsertKeepsHistory(t *testing.T) {
	s := openTestStore(t).Schedules()
	ctx := context.Background()
	t0 := time.Date(2026, 9, 13, 9, 0, 0, 0, time.UTC)

	sc := scheduling.Schedule{
		ID: "s1", Owner: "lead-1", To: "lead-1", Cron: "0 9 * * *", Subject: "daily",
		Body: json.RawMessage(`{"k":1}`), Enabled: true, NextAt: t0, CreatedAt: t0,
	}
	if err := s.Upsert(ctx, sc); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	got, err := s.Get(ctx, "s1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Owner != "lead-1" || got.To != "lead-1" || got.Cron != "0 9 * * *" || got.Subject != "daily" ||
		string(got.Body) != `{"k":1}` || !got.Enabled || !got.NextAt.Equal(t0) || got.LastAt != nil || !got.CreatedAt.Equal(t0) {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}

	fired := t0.Add(time.Hour)
	if err := s.MarkFired(ctx, "s1", fired, fired.Add(24*time.Hour)); err != nil {
		t.Fatalf("MarkFired: %v", err)
	}

	sc.Cron = "0 10 * * *"
	sc.NextAt = t0.Add(25 * time.Hour)
	sc.CreatedAt = t0.Add(48 * time.Hour)
	if err := s.Upsert(ctx, sc); err != nil {
		t.Fatalf("Upsert again: %v", err)
	}
	got, err = s.Get(ctx, "s1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Cron != "0 10 * * *" || !got.NextAt.Equal(t0.Add(25*time.Hour)) {
		t.Fatalf("definition not replaced: %+v", got)
	}
	if !got.CreatedAt.Equal(t0) {
		t.Fatalf("created_at overwritten: %s", got.CreatedAt)
	}
	if got.LastAt == nil || !got.LastAt.Equal(fired) {
		t.Fatalf("last_at lost: %v", got.LastAt)
	}
	all, err := s.List(ctx, scheduling.Filter{})
	if err != nil || len(all) != 1 {
		t.Fatalf("List = %d schedules, err %v; want exactly one", len(all), err)
	}
}

func TestScheduleListFiltersAndDue(t *testing.T) {
	s := openTestStore(t).Schedules()
	ctx := context.Background()
	t0 := time.Date(2026, 9, 13, 9, 0, 0, 0, time.UTC)

	put := func(id, owner, to string, next time.Time, enabled bool) {
		t.Helper()
		if err := s.Upsert(ctx, scheduling.Schedule{
			ID: id, Owner: owner, To: to, Cron: "* * * * *", Enabled: enabled, NextAt: next, CreatedAt: t0,
		}); err != nil {
			t.Fatalf("Upsert %s: %v", id, err)
		}
	}
	put("a", "lead-1", "lead-1", t0, true)
	put("b", "lead-1", "leaf-1", t0.Add(time.Minute), true)
	put("c", "leaf-1", "leaf-1", t0.Add(-time.Minute), false)

	ids := func(list []scheduling.Schedule) []string {
		var out []string
		for _, sc := range list {
			out = append(out, sc.ID)
		}
		return out
	}
	cases := []struct {
		f    scheduling.Filter
		want []string
	}{
		{scheduling.Filter{Owner: "lead-1"}, []string{"a", "b"}},
		{scheduling.Filter{To: "leaf-1"}, []string{"b", "c"}},
		{scheduling.Filter{Worker: "leaf-1"}, []string{"b", "c"}},
		{scheduling.Filter{Worker: "lead-1"}, []string{"a", "b"}},
		{scheduling.Filter{Owner: "lead-1", To: "leaf-1"}, []string{"b"}},
	}
	for _, tc := range cases {
		got, err := s.List(ctx, tc.f)
		if err != nil {
			t.Fatalf("List(%+v): %v", tc.f, err)
		}
		if g := ids(got); len(g) != len(tc.want) || func() bool {
			for i := range g {
				if g[i] != tc.want[i] {
					return true
				}
			}
			return false
		}() {
			t.Errorf("List(%+v) = %v, want %v", tc.f, g, tc.want)
		}
	}

	due, err := s.ListDue(ctx, t0)
	if err != nil {
		t.Fatalf("ListDue: %v", err)
	}
	if g := ids(due); len(g) != 1 || g[0] != "a" {
		t.Fatalf("ListDue at t0 = %v, want [a] (b is not due yet, c is disabled)", g)
	}

	if err := s.Delete(ctx, "a"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := s.Delete(ctx, "a"); !errors.Is(err, scheduling.ErrNotFound) {
		t.Fatalf("second Delete = %v, want ErrNotFound", err)
	}
	if _, err := s.Get(ctx, "a"); !errors.Is(err, scheduling.ErrNotFound) {
		t.Fatalf("Get after delete = %v, want ErrNotFound", err)
	}
	if err := s.MarkFired(ctx, "a", t0, t0); !errors.Is(err, scheduling.ErrNotFound) {
		t.Fatalf("MarkFired after delete = %v, want ErrNotFound", err)
	}
}
