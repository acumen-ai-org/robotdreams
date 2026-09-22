package server

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/acumen-ai-org/robotdreams/pkg/messaging"
	"github.com/acumen-ai-org/robotdreams/pkg/scheduling"
)

var schedStart = time.Date(2026, 9, 13, 9, 0, 30, 0, time.UTC)

type schedEnv struct {
	srv   *Server
	clock *fakeClock
	ctx   context.Context
}

func newSchedEnv(t *testing.T) *schedEnv {
	t.Helper()
	clock := newFakeClock(schedStart)
	s := newTestServer(t, func(c *Config) { c.Clock = clock })
	connect(t, s, "lead-1", "")
	connect(t, s, "leaf-1", "lead-1")
	return &schedEnv{srv: s, clock: clock, ctx: context.Background()}
}

func (e *schedEnv) put(t *testing.T, id, cron, owner, to string, enabled bool) scheduling.Schedule {
	t.Helper()
	next, err := scheduling.ParseCron(cron)
	if err != nil {
		t.Fatalf("ParseCron(%q): %v", cron, err)
	}
	s := scheduling.Schedule{
		ID: id, Owner: owner, To: to, Cron: cron, Subject: "tick " + id,
		Body: []byte(`{"n":1}`), Enabled: enabled, NextAt: next(e.clock.Now()), CreatedAt: e.clock.Now(),
	}
	if err := e.srv.Schedules().Upsert(e.ctx, s); err != nil {
		t.Fatalf("Upsert(%q): %v", id, err)
	}
	return s
}

func (e *schedEnv) get(t *testing.T, id string) scheduling.Schedule {
	t.Helper()
	s, err := e.srv.Schedules().Get(e.ctx, id)
	if err != nil {
		t.Fatalf("Get(%q): %v", id, err)
	}
	return s
}

func (e *schedEnv) inbox(t *testing.T, workerID string) []messaging.Envelope {
	t.Helper()
	msgs, err := e.srv.Messaging().Tail(e.ctx, messaging.TailFilter{WorkerID: workerID})
	if err != nil {
		t.Fatalf("Tail(%q): %v", workerID, err)
	}
	return msgs
}

func at(h, m int) time.Time { return time.Date(2026, 9, 13, h, m, 0, 0, time.UTC) }

func TestNextAfter(t *testing.T) {
	now := schedStart
	tests := []struct {
		cron string
		want time.Time
	}{
		{"* * * * *", at(9, 1)},
		{"0 * * * *", at(10, 0)},
		{"30 9 * * *", at(9, 30)},
		{"0 9 * * *", time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC)},
		{"@hourly", at(10, 0)},
		{"@daily", time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)},
		{"*/15 * * * *", at(9, 15)},

		{"0 0 * * 1", time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)},

		{"TZ=Europe/Stockholm 0 12 * * *", time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC)},
	}
	for _, tc := range tests {
		t.Run(tc.cron, func(t *testing.T) {
			got, err := nextAfter(scheduling.Schedule{ID: "x", Cron: tc.cron}, now)
			if err != nil {
				t.Fatalf("nextAfter: %v", err)
			}
			if !got.Equal(tc.want) {
				t.Fatalf("nextAfter(%q, %s) = %s, want %s", tc.cron, now, got, tc.want)
			}
			if got.Location() != time.UTC {
				t.Fatalf("nextAfter returned %s in %s, want UTC", got, got.Location())
			}
		})
	}

	got, err := nextAfter(scheduling.Schedule{Cron: "* * * * *"}, at(9, 1))
	if err != nil || !got.Equal(at(9, 2)) {
		t.Fatalf("nextAfter on the boundary = %s, %v; want %s", got, err, at(9, 2))
	}

	if _, err := nextAfter(scheduling.Schedule{Cron: "not a cron"}, now); !errors.Is(err, scheduling.ErrInvalid) {
		t.Fatalf("bad cron: err = %v, want ErrInvalid", err)
	}
	_, err = nextAfter(scheduling.Schedule{Cron: "0 0 30 2 *"}, now)
	if err == nil || !strings.Contains(err.Error(), "no next occurrence") {
		t.Fatalf("impossible date: err = %v, want a 'no next occurrence' error", err)
	}
}

func TestNewSchedulerDefaultsToServerClock(t *testing.T) {
	e := newSchedEnv(t)
	sc := NewScheduler(e.srv, nil, nil)
	if sc.clock != e.srv.Clock() {
		t.Fatal("nil clock did not default to the server's")
	}
	if sc.log == nil {
		t.Fatal("logger is nil")
	}
	sc.SetLogger(nil)
	if sc.log == nil {
		t.Fatal("SetLogger(nil) cleared the logger")
	}
	own := log.New(&bytes.Buffer{}, "", 0)
	sc.SetLogger(own)
	if sc.log != own {
		t.Fatal("SetLogger did not install the logger")
	}

	other := newFakeClock(schedStart.Add(time.Hour))
	if sc := NewScheduler(e.srv, other, nil); sc.clock != other {
		t.Fatal("an explicit clock was not kept")
	}
}

func TestTickFiresDueSchedulesAndAdvances(t *testing.T) {
	e := newSchedEnv(t)
	e.put(t, "minutely", "* * * * *", "lead-1", "leaf-1", true)
	e.put(t, "hourly", "0 * * * *", "lead-1", "leaf-1", true)
	e.put(t, "off", "* * * * *", "lead-1", "leaf-1", false)

	var fired []ScheduleFired
	sc := NewScheduler(e.srv, e.clock, func(f ScheduleFired) { fired = append(fired, f) })

	tick := func(want int) {
		t.Helper()
		n, err := sc.Tick(e.ctx)
		if err != nil {
			t.Fatalf("Tick at %s: %v", e.clock.Now(), err)
		}
		if n != want {
			t.Fatalf("Tick at %s fired %d, want %d", e.clock.Now().Format(time.RFC3339), n, want)
		}
	}

	tick(0)
	if len(fired) != 0 || len(e.inbox(t, "leaf-1")) != 0 {
		t.Fatal("something fired before its time")
	}

	e.clock.Advance(30 * time.Second)
	tick(1)
	tick(0)
	s := e.get(t, "minutely")
	if !s.NextAt.Equal(at(9, 2)) || s.LastAt == nil || !s.LastAt.Equal(at(9, 1)) {
		t.Fatalf("after first fire: next_at=%s last_at=%v", s.NextAt, s.LastAt)
	}

	e.clock.Advance(90 * time.Second)
	tick(1)
	s = e.get(t, "minutely")
	if !s.NextAt.Equal(at(9, 3)) || !s.LastAt.Equal(time.Date(2026, 9, 13, 9, 2, 30, 0, time.UTC)) {
		t.Fatalf("after second fire: next_at=%s last_at=%v", s.NextAt, s.LastAt)
	}

	if h := e.get(t, "hourly"); !h.NextAt.Equal(at(10, 0)) || h.LastAt != nil {
		t.Fatalf("hourly moved: %+v", h)
	}
	if off := e.get(t, "off"); !off.NextAt.Equal(at(9, 1)) || off.LastAt != nil {
		t.Fatalf("disabled schedule moved: %+v", off)
	}

	e.clock.Advance(57*time.Minute + 30*time.Second)
	tick(2)
	if h := e.get(t, "hourly"); !h.NextAt.Equal(at(11, 0)) || h.LastAt == nil || !h.LastAt.Equal(at(10, 0)) {
		t.Fatalf("hourly after fire: %+v", h)
	}
	if m := e.get(t, "minutely"); !m.NextAt.Equal(at(10, 1)) {
		t.Fatalf("minutely after 10:00 fire: next_at=%s", m.NextAt)
	}

	if len(fired) != 4 {
		t.Fatalf("hook saw %d fires, want 4", len(fired))
	}
	wantIDs := []string{"minutely", "minutely", "minutely", "hourly"}
	for i, f := range fired {
		if f.Schedule.ID != wantIDs[i] {
			t.Fatalf("fire %d was %q, want %q", i, f.Schedule.ID, wantIDs[i])
		}
		env := f.Envelope
		if env.Type != messaging.TypeScheduled || env.From != "lead-1" || env.To != "leaf-1" ||
			env.CausationID != f.Schedule.ID || env.Subject != "tick "+f.Schedule.ID || env.ID == "" {
			t.Fatalf("fire %d envelope = %+v", i, env)
		}
		if !f.FiredAt.Equal(env.CreatedAt) {
			t.Fatalf("fire %d: FiredAt %s != envelope CreatedAt %s", i, f.FiredAt, env.CreatedAt)
		}
	}
	if f := fired[3]; !f.FiredAt.Equal(at(10, 0)) || !f.NextAt.Equal(at(11, 0)) {
		t.Fatalf("hourly fire = %+v", f)
	}

	inbox := e.inbox(t, "leaf-1")
	if len(inbox) != 4 {
		t.Fatalf("leaf-1 inbox has %d messages, want 4", len(inbox))
	}
	if len(e.inbox(t, "lead-1")) != 0 {
		t.Fatal("the owner received its own scheduled message")
	}
}

func TestTickWithoutHook(t *testing.T) {
	e := newSchedEnv(t)
	e.put(t, "minutely", "* * * * *", "lead-1", "leaf-1", true)
	sc := NewScheduler(e.srv, e.clock, nil)
	e.clock.Advance(time.Minute)
	n, err := sc.Tick(e.ctx)
	if err != nil || n != 1 {
		t.Fatalf("Tick = %d, %v; want 1, nil", n, err)
	}
}

func TestTickAdvancesPastAFailedFire(t *testing.T) {
	e := newSchedEnv(t)

	e.put(t, "ghost", "* * * * *", "lead-1", "nobody", true)
	e.put(t, "ok", "* * * * *", "lead-1", "leaf-1", true)

	var logs bytes.Buffer
	var fired []ScheduleFired
	sc := NewScheduler(e.srv, e.clock, func(f ScheduleFired) { fired = append(fired, f) })
	sc.SetLogger(log.New(&logs, "", 0))

	e.clock.Advance(30 * time.Second)
	n, err := sc.Tick(e.ctx)
	if err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if n != 1 {
		t.Fatalf("Tick fired %d, want 1 (the ghost is not a fire)", n)
	}
	if len(fired) != 1 || fired[0].Schedule.ID != "ok" {
		t.Fatalf("hook saw %+v, want only the ok schedule", fired)
	}
	if !strings.Contains(logs.String(), "schedule ghost: fire:") {
		t.Fatalf("failed fire not logged: %q", logs.String())
	}
	g := e.get(t, "ghost")
	if !g.NextAt.Equal(at(9, 2)) {
		t.Fatalf("ghost not advanced: next_at=%s", g.NextAt)
	}
	if g.LastAt != nil {
		t.Fatalf("a failed fire was recorded as one: last_at=%v", g.LastAt)
	}

	logs.Reset()
	if n, _ := sc.Tick(e.ctx); n != 0 {
		t.Fatalf("second tick fired %d", n)
	}
	if logs.Len() != 0 {
		t.Fatalf("second tick logged: %q", logs.String())
	}

	e.clock.Advance(time.Minute)
	if n, _ := sc.Tick(e.ctx); n != 1 {
		t.Fatalf("third tick fired %d, want 1", n)
	}
	if g := e.get(t, "ghost"); g.LastAt != nil || !g.NextAt.Equal(at(9, 3)) {
		t.Fatalf("ghost after second failure: %+v", g)
	}
	if ok := e.get(t, "ok"); ok.LastAt == nil || !ok.LastAt.Equal(at(9, 2)) {
		t.Fatalf("ok after two fires: %+v", ok)
	}
}

func TestTickSkipsAScheduleWithNoNextOccurrence(t *testing.T) {
	e := newSchedEnv(t)

	s := scheduling.Schedule{
		ID: "never", Owner: "lead-1", To: "leaf-1", Cron: "0 0 30 2 *", Enabled: true,
		NextAt: schedStart.Add(-time.Hour), CreatedAt: schedStart,
	}
	if err := e.srv.Schedules().Upsert(e.ctx, s); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	var logs bytes.Buffer
	sc := NewScheduler(e.srv, e.clock, nil)
	sc.SetLogger(log.New(&logs, "", 0))
	n, err := sc.Tick(e.ctx)
	if err != nil || n != 0 {
		t.Fatalf("Tick = %d, %v; want 0, nil", n, err)
	}
	if !strings.Contains(logs.String(), "schedule never:") || !strings.Contains(logs.String(), "(skipped)") {
		t.Fatalf("skip not logged: %q", logs.String())
	}
	if len(e.inbox(t, "leaf-1")) != 0 {
		t.Fatal("the impossible schedule fired")
	}
}

func TestStartSkipsMissedOccurrences(t *testing.T) {
	e := newSchedEnv(t)
	e.put(t, "hourly", "0 * * * *", "lead-1", "leaf-1", true)
	e.put(t, "future", "0 14 * * *", "lead-1", "leaf-1", true)

	firedAt := at(8, 0)
	if err := e.srv.Schedules().MarkFired(e.ctx, "hourly", firedAt, at(9, 0)); err != nil {
		t.Fatalf("MarkFired: %v", err)
	}

	e.clock.Advance(3 * time.Hour)
	var logs bytes.Buffer
	starting := true
	sc := NewScheduler(e.srv, e.clock, func(ScheduleFired) {
		if starting {
			t.Error("Start fired a schedule")
		}
	})
	sc.SetLogger(log.New(&logs, "", 0))
	if err := sc.Start(e.ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	starting = false

	h := e.get(t, "hourly")
	if !h.NextAt.Equal(at(13, 0)) {
		t.Fatalf("hourly next_at = %s, want 13:00 (no catch-up)", h.NextAt)
	}
	if h.LastAt == nil || !h.LastAt.Equal(firedAt) {
		t.Fatalf("hourly last_at = %v, want the pre-outage %s preserved", h.LastAt, firedAt)
	}
	f := e.get(t, "future")
	if !f.NextAt.Equal(at(14, 0)) || f.LastAt != nil {
		t.Fatalf("future = %+v", f)
	}
	if !strings.Contains(logs.String(), "schedule hourly was due at 2026-09-13T09:00:00Z while the server was down; not fired, next at 2026-09-13T13:00:00Z") {
		t.Fatalf("skip log = %q", logs.String())
	}
	if strings.Contains(logs.String(), "future") {
		t.Fatalf("a schedule that was not due was logged: %q", logs.String())
	}
	if len(e.inbox(t, "leaf-1")) != 0 {
		t.Fatal("missed occurrences were replayed")
	}

	if n, _ := sc.Tick(e.ctx); n != 0 {
		t.Fatalf("Tick right after Start fired %d", n)
	}
	e.clock.Advance(time.Hour)
	if n, _ := sc.Tick(e.ctx); n != 1 {
		t.Fatalf("Tick at 13:00:30 fired %d, want 1", n)
	}
}

func TestStartLeavesAnImpossibleScheduleAlone(t *testing.T) {
	e := newSchedEnv(t)
	s := scheduling.Schedule{
		ID: "never", Owner: "lead-1", To: "leaf-1", Cron: "0 0 30 2 *", Enabled: true,
		NextAt: schedStart.Add(-time.Hour), CreatedAt: schedStart,
	}
	if err := e.srv.Schedules().Upsert(e.ctx, s); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	var logs bytes.Buffer
	sc := NewScheduler(e.srv, e.clock, nil)
	sc.SetLogger(log.New(&logs, "", 0))
	if err := sc.Start(e.ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := e.get(t, "never"); !got.NextAt.Equal(s.NextAt) {
		t.Fatalf("NextAt moved to %s", got.NextAt)
	}
	if !strings.Contains(logs.String(), "(left as is)") {
		t.Fatalf("log = %q", logs.String())
	}
}

func TestStartAndTickReportStoreErrors(t *testing.T) {
	e := newSchedEnv(t)
	ctx, cancel := context.WithCancel(e.ctx)
	cancel()
	sc := NewScheduler(e.srv, e.clock, nil)
	if err := sc.Start(ctx); err == nil {
		t.Fatal("Start on a canceled context succeeded")
	}
	if _, err := sc.Tick(ctx); err == nil {
		t.Fatal("Tick on a canceled context succeeded")
	}
}

func TestFireScheduleDoesNotTouchTheSchedule(t *testing.T) {
	e := newSchedEnv(t)
	s := e.put(t, "manual", "0 12 * * *", "lead-1", "leaf-1", true)

	env, err := e.srv.FireSchedule(e.ctx, s)
	if err != nil {
		t.Fatalf("FireSchedule: %v", err)
	}
	if env.Type != messaging.TypeScheduled || env.From != "lead-1" || env.To != "leaf-1" ||
		env.CausationID != "manual" || env.Subject != "tick manual" || string(env.Body) != `{"n":1}` {
		t.Fatalf("envelope = %+v", env)
	}
	if !env.CreatedAt.Equal(schedStart) {
		t.Fatalf("CreatedAt = %s, want the fake clock's %s", env.CreatedAt, schedStart)
	}
	after := e.get(t, "manual")
	if !after.NextAt.Equal(s.NextAt) || after.LastAt != nil {
		t.Fatalf("FireSchedule moved the schedule: %+v", after)
	}
	if len(e.inbox(t, "leaf-1")) != 1 {
		t.Fatal("no message delivered")
	}

	if _, err := e.srv.FireSchedule(e.ctx, scheduling.Schedule{ID: "x", Owner: "nobody", To: "leaf-1"}); err == nil {
		t.Fatal("fired for an unknown owner")
	}
	if _, err := e.srv.FireSchedule(e.ctx, scheduling.Schedule{ID: "x", Owner: "lead-1", To: "nobody"}); err == nil {
		t.Fatal("fired to an unknown recipient")
	}
}

type steppingClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *steppingClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(time.Minute)
	return c.now
}

func TestRunSchedulerTicksUntilCanceled(t *testing.T) {
	e := newSchedEnv(t)
	e.put(t, "minutely", "* * * * *", "lead-1", "leaf-1", true)

	clock := &steppingClock{now: schedStart.Add(-time.Minute)}

	ctx, cancel := context.WithCancel(e.ctx)
	defer cancel()
	fires := make(chan ScheduleFired, 64)
	done := make(chan struct{})
	go func() {
		defer close(done)
		RunScheduler(ctx, e.srv, clock, time.Millisecond, func(f ScheduleFired) {
			fires <- f
			cancel()
		})
	}()

	select {
	case f := <-fires:
		if f.Schedule.ID != "minutely" || f.Envelope.To != "leaf-1" {
			t.Fatalf("fire = %+v", f)
		}
		if !f.FiredAt.Equal(time.Date(2026, 9, 13, 9, 1, 30, 0, time.UTC)) || !f.NextAt.Equal(at(9, 2)) {
			t.Fatalf("first fire at %s next %s", f.FiredAt, f.NextAt)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run never fired")
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after cancel")
	}

	if s := e.get(t, "minutely"); s.LastAt == nil {
		t.Fatal("Start consumed the first occurrence instead of Tick firing it")
	}
}

func TestRunReturnsOnAlreadyCanceledContext(t *testing.T) {
	e := newSchedEnv(t)
	ctx, cancel := context.WithCancel(e.ctx)
	cancel()
	var logs bytes.Buffer
	sc := NewScheduler(e.srv, e.clock, func(ScheduleFired) { t.Error("fired") })
	sc.SetLogger(log.New(&logs, "", 0))
	done := make(chan struct{})
	go func() {
		defer close(done)
		sc.Run(ctx, 0)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return on a canceled context")
	}

	if !strings.Contains(logs.String(), "scheduler: start:") {
		t.Fatalf("log = %q", logs.String())
	}
}

func TestDeleteWorkerRemovesItsSchedules(t *testing.T) {
	e := newSchedEnv(t)
	e.put(t, "owned-by-leaf", "0 12 * * *", "leaf-1", "lead-1", true)
	e.put(t, "addressed-to-leaf", "0 12 * * *", "lead-1", "leaf-1", true)
	e.put(t, "unrelated", "0 12 * * *", "lead-1", "lead-1", true)

	if _, err := e.srv.DeleteWorker(e.ctx, "leaf-1", "gone", false); err != nil {
		t.Fatalf("DeleteWorker: %v", err)
	}

	left, err := e.srv.Schedules().List(e.ctx, scheduling.Filter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(left) != 1 || left[0].ID != "unrelated" {
		t.Fatalf("schedules after the delete = %+v, want only \"unrelated\"", left)
	}
}
