package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/acumen-ai-org/robotdreams/internal/server"
	"github.com/acumen-ai-org/robotdreams/pkg/messaging"
)

type scheduleEnv struct {
	*testEnv
	clock *fakeClock
	lead  testWorker
	leaf  testWorker
}

func newScheduleEnv(t *testing.T) *scheduleEnv {
	t.Helper()
	clock := newFakeClock(time.Date(2026, 9, 13, 9, 0, 30, 0, time.UTC))
	e := newTestEnv(t, func(c *server.Config) { c.Clock = clock })
	return &scheduleEnv{
		testEnv: e,
		clock:   clock,
		lead:    e.connect("lead-1", "lead", ""),
		leaf:    e.connect("leaf-1", "leaf", "lead-1"),
	}
}

func (e *scheduleEnv) put(token, id string, body map[string]any, wantStatus int) scheduleView {
	e.t.Helper()
	status, raw := e.do(http.MethodPut, "/api/schedules/"+id, token, body)
	if status != wantStatus {
		e.t.Fatalf("PUT /api/schedules/%s: status %d, want %d: %s", id, status, wantStatus, raw)
	}
	var out scheduleView
	if status == http.StatusOK {
		decodeInto(e.t, raw, &out)
	}
	return out
}

func (e *scheduleEnv) list(token, workerID string, wantStatus int) []scheduleView {
	e.t.Helper()
	path := "/api/schedules"
	if workerID != "" {
		path += "?worker_id=" + workerID
	}
	status, raw := e.do(http.MethodGet, path, token, nil)
	if status != wantStatus {
		e.t.Fatalf("GET %s: status %d, want %d: %s", path, status, wantStatus, raw)
	}
	var out struct {
		Schedules []scheduleView `json:"schedules"`
	}
	if status == http.StatusOK {
		decodeInto(e.t, raw, &out)
	}
	return out.Schedules
}

func (e *scheduleEnv) tail(w testWorker) []envelopeView {
	return e.tailAs(w.Token, w.ID)
}

func (e *scheduleEnv) tailAs(token, workerID string) []envelopeView {
	e.t.Helper()
	status, raw := e.do(http.MethodGet, "/api/messages?worker_id="+workerID, token, nil)
	if status != http.StatusOK {
		e.t.Fatalf("tail %s: status %d: %s", workerID, status, raw)
	}
	var out struct {
		Messages []envelopeView `json:"messages"`
	}
	decodeInto(e.t, raw, &out)
	return out.Messages
}

func TestScheduleUpsertTwiceIsOneSchedule(t *testing.T) {
	e := newScheduleEnv(t)

	first := e.put(e.lead.Token, "daily-ideas", map[string]any{
		"cron": "0 9 * * *", "subject": "collect ideas", "body": map[string]any{"job": "idea-collector"},
	}, http.StatusOK)
	if first.Owner != "lead-1" || first.To != "lead-1" {
		t.Fatalf("owner/to = %s/%s, want lead-1/lead-1 (to defaults to the owner)", first.Owner, first.To)
	}
	if !first.Enabled {
		t.Fatal("enabled should default to true")
	}
	wantNext := time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC)
	if !first.NextAt.Equal(wantNext) {
		t.Fatalf("next_at = %s, want %s", first.NextAt, wantNext)
	}
	if string(first.Body) != `{"job":"idea-collector"}` {
		t.Fatalf("body = %s", first.Body)
	}

	second := e.put(e.lead.Token, "daily-ideas", map[string]any{
		"cron": "0 10 * * *", "to": "leaf-1", "subject": "collect ideas",
	}, http.StatusOK)
	if second.Cron != "0 10 * * *" || second.To != "leaf-1" {
		t.Fatalf("second upsert did not replace the definition: %+v", second)
	}
	if want := time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC); !second.NextAt.Equal(want) {
		t.Fatalf("next_at after upsert = %s, want %s", second.NextAt, want)
	}
	if !second.CreatedAt.Equal(first.CreatedAt) {
		t.Fatalf("created_at changed on upsert: %s -> %s", first.CreatedAt, second.CreatedAt)
	}

	all := e.list(e.adminToken("ops"), "", http.StatusOK)
	if len(all) != 1 {
		t.Fatalf("admin list = %d schedules, want 1", len(all))
	}
}

func TestScheduleRejectsUnknownRecipientAndBadCron(t *testing.T) {
	e := newScheduleEnv(t)

	e.put(e.lead.Token, "s1", map[string]any{"cron": "0 9 * * *", "to": "nobody"}, http.StatusBadRequest)
	e.put(e.lead.Token, "s2", map[string]any{"cron": "every morning"}, http.StatusBadRequest)

	e.put(e.lead.Token, "s4", map[string]any{"cron": "0 9 * * *", "owner": "leaf-1"}, http.StatusForbidden)

	e.put(e.adminToken("ops"), "s5", map[string]any{"cron": "0 9 * * *", "owner": "ghost"}, http.StatusBadRequest)
	got := e.put(e.adminToken("ops"), "s6", map[string]any{"cron": "0 9 * * *", "owner": "leaf-1"}, http.StatusOK)
	if got.Owner != "leaf-1" || got.To != "leaf-1" {
		t.Fatalf("admin-registered owner/to = %s/%s", got.Owner, got.To)
	}

	e.put(e.adminToken("ops"), "s7", map[string]any{"cron": "0 9 * * *"}, http.StatusBadRequest)

	if status, raw := e.do(http.MethodPost, "/api/schedules", e.lead.Token, map[string]any{"cron": "0 9 * * *"}); status != http.StatusOK {
		t.Fatalf("POST /api/schedules: %d %s", status, raw)
	} else {
		var v scheduleView
		decodeInto(t, raw, &v)
		if v.ID == "" {
			t.Fatal("POST did not mint an id")
		}
	}
}

func TestScheduleRefusesDelegatedChild(t *testing.T) {
	e := newScheduleEnv(t)
	child := e.delegate(e.lead.Token, map[string]any{"child": "job-1"}, http.StatusCreated)

	e.put(child.Token, "s1", map[string]any{"cron": "0 9 * * *"}, http.StatusForbidden)
	if status, _ := e.do(http.MethodPost, "/api/schedules", child.Token, map[string]any{"cron": "0 9 * * *"}); status != http.StatusForbidden {
		t.Fatalf("POST as a delegated child: status %d, want 403", status)
	}
}

func TestScheduleFireDeliversOneScheduledEnvelope(t *testing.T) {
	e := newScheduleEnv(t)
	s := e.put(e.lead.Token, "nudge", map[string]any{
		"cron": "0 9 * * *", "to": "leaf-1", "subject": "collect ideas", "body": map[string]any{"job": "idea-collector"},
	}, http.StatusOK)

	if status, _ := e.do(http.MethodPost, "/api/schedules/nudge/fire", e.leaf.Token, nil); status != http.StatusForbidden {
		t.Fatalf("fire as recipient: status %d, want 403", status)
	}
	status, raw := e.do(http.MethodPost, "/api/schedules/nudge/fire", e.lead.Token, nil)
	if status != http.StatusAccepted {
		t.Fatalf("fire: status %d: %s", status, raw)
	}
	var resp struct {
		Fired   scheduleFiredView `json:"fired"`
		Message envelopeView      `json:"message"`
	}
	decodeInto(t, raw, &resp)
	if resp.Fired.ScheduleID != "nudge" || resp.Message.ID == "" || resp.Fired.MessageID != resp.Message.ID {
		t.Fatalf("fire response: %+v", resp)
	}

	inbox := e.tail(e.leaf)
	if len(inbox) != 1 {
		t.Fatalf("leaf-1 inbox has %d messages, want exactly 1: %+v", len(inbox), inbox)
	}
	got := inbox[0]
	if got.Type != string(messaging.TypeScheduled) || got.From != "lead-1" || got.To != "leaf-1" ||
		got.CausationID != "nudge" || got.Subject != "collect ideas" || string(got.Body) != `{"job":"idea-collector"}` {
		t.Fatalf("delivered envelope = %+v", got)
	}
	if len(e.tail(e.lead)) != 0 {
		t.Fatal("the owner received a copy; a scheduled message goes to exactly To")
	}

	status, raw = e.do(http.MethodGet, "/api/schedules/nudge", e.lead.Token, nil)
	if status != http.StatusOK {
		t.Fatalf("GET after fire: %d %s", status, raw)
	}
	var after scheduleView
	decodeInto(t, raw, &after)
	if !after.NextAt.Equal(s.NextAt) {
		t.Fatalf("next_at moved on a manual fire: %s -> %s", s.NextAt, after.NextAt)
	}
	if after.LastAt == nil || !after.LastAt.Equal(e.clock.Now()) {
		t.Fatalf("last_at = %v, want %s", after.LastAt, e.clock.Now())
	}
}

func TestSchedulerFiresOncePerDueMinute(t *testing.T) {
	e := newScheduleEnv(t)
	e.put(e.lead.Token, "every-minute", map[string]any{"cron": "* * * * *", "to": "leaf-1", "subject": "tick"}, http.StatusOK)

	var fired []server.ScheduleFired
	sched := server.NewScheduler(e.srv, e.clock, func(f server.ScheduleFired) { fired = append(fired, f) })
	ctx := context.Background()
	if err := sched.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	tick := func(want int) {
		t.Helper()
		n, err := sched.Tick(ctx)
		if err != nil {
			t.Fatalf("Tick: %v", err)
		}
		if n != want {
			t.Fatalf("Tick at %s fired %d, want %d", e.clock.Now().Format(time.RFC3339), n, want)
		}
	}

	tick(0)
	e.clock.Advance(30 * time.Second)
	tick(1)
	tick(0)
	e.clock.Advance(20 * time.Second)
	tick(0)
	e.clock.Advance(40 * time.Second)
	tick(1)

	if len(fired) != 2 {
		t.Fatalf("hook saw %d fires, want 2", len(fired))
	}
	if f := fired[1]; f.Envelope.From != "lead-1" || f.Envelope.To != "leaf-1" || f.Envelope.CausationID != "every-minute" ||
		!f.NextAt.Equal(time.Date(2026, 9, 13, 9, 3, 0, 0, time.UTC)) {
		t.Fatalf("second fire = %+v", f)
	}
	inbox := e.tail(e.leaf)
	if len(inbox) != 2 {
		t.Fatalf("leaf-1 inbox has %d messages, want 2", len(inbox))
	}

	got := e.list(e.leaf.Token, "", http.StatusOK)
	if len(got) != 1 || !got[0].NextAt.Equal(time.Date(2026, 9, 13, 9, 3, 0, 0, time.UTC)) || got[0].LastAt == nil ||
		!got[0].LastAt.Equal(time.Date(2026, 9, 13, 9, 2, 0, 0, time.UTC)) {
		t.Fatalf("schedule after two fires = %+v", got)
	}

	if status, raw := e.do(http.MethodDelete, "/api/schedules/every-minute", e.leaf.Token, nil); status != http.StatusForbidden {
		t.Fatalf("delete as recipient: status %d: %s", status, raw)
	}
	if status, raw := e.do(http.MethodDelete, "/api/schedules/every-minute", e.lead.Token, nil); status != http.StatusOK {
		t.Fatalf("delete: status %d: %s", status, raw)
	}
	e.clock.Advance(time.Minute)
	tick(0)
	if len(e.tail(e.leaf)) != 2 {
		t.Fatal("a deleted schedule fired")
	}
	if status, _ := e.do(http.MethodGet, "/api/schedules/every-minute", e.lead.Token, nil); status != http.StatusNotFound {
		t.Fatalf("GET after delete: status %d, want 404", status)
	}
}

func TestSchedulerSkipsFiresMissedWhileDown(t *testing.T) {
	e := newScheduleEnv(t)
	e.put(e.lead.Token, "hourly", map[string]any{"cron": "0 * * * *", "to": "leaf-1"}, http.StatusOK)

	e.clock.Advance(3 * time.Hour)
	sched := server.NewScheduler(e.srv, e.clock, nil)
	ctx := context.Background()
	if err := sched.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if n, _ := sched.Tick(ctx); n != 0 {
		t.Fatalf("Tick right after Start fired %d, want 0 (no catch-up)", n)
	}

	admin := e.adminToken("ops")
	if len(e.tailAs(admin, "leaf-1")) != 0 {
		t.Fatal("missed occurrences were replayed")
	}
	got := e.list(admin, "lead-1", http.StatusOK)
	if want := time.Date(2026, 9, 13, 13, 0, 0, 0, time.UTC); len(got) != 1 || !got[0].NextAt.Equal(want) {
		t.Fatalf("next_at after Start = %+v, want %s", got, want)
	}
	if got[0].LastAt != nil {
		t.Fatal("a skipped occurrence was recorded as a fire")
	}
}

func TestScheduleListFiltersByWorker(t *testing.T) {
	e := newScheduleEnv(t)
	e.put(e.lead.Token, "lead-own", map[string]any{"cron": "0 9 * * *"}, http.StatusOK)
	e.put(e.lead.Token, "lead-to-leaf", map[string]any{"cron": "0 9 * * *", "to": "leaf-1"}, http.StatusOK)
	e.put(e.leaf.Token, "leaf-own", map[string]any{"cron": "0 9 * * *"}, http.StatusOK)

	ids := func(list []scheduleView) []string {
		var out []string
		for _, s := range list {
			out = append(out, s.ID)
		}
		return out
	}
	equal := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}

	all := []string{"lead-own", "lead-to-leaf", "leaf-own"}
	if got := ids(e.list(e.lead.Token, "", http.StatusOK)); !equal(got, all) {
		t.Fatalf("unfiltered list sees %v, want everything", got)
	}
	if got := ids(e.list(e.lead.Token, "lead-1", http.StatusOK)); !equal(got, []string{"lead-own", "lead-to-leaf"}) {
		t.Fatalf("filtered by lead-1: %v", got)
	}
	if got := ids(e.list(e.leaf.Token, "leaf-1", http.StatusOK)); !equal(got, []string{"lead-to-leaf", "leaf-own"}) {
		t.Fatalf("filtered by leaf-1 (owned or received): %v", got)
	}

	if got := ids(e.list(e.leaf.Token, "lead-1", http.StatusOK)); !equal(got, []string{"lead-own", "lead-to-leaf"}) {
		t.Fatalf("leaf-1 filtering by lead-1: %v", got)
	}
	e.list("", "", http.StatusUnauthorized)

	child := e.delegate(e.lead.Token, map[string]any{"child": "proxy"}, http.StatusCreated)
	if got := ids(e.list(child.Token, "", http.StatusOK)); !equal(got, all) {
		t.Fatalf("delegated child sees %v, want everything", got)
	}
	if status, _ := e.do(http.MethodGet, "/api/schedules/lead-own", child.Token, nil); status != http.StatusOK {
		t.Fatalf("delegated child GET one: %d", status)
	}
	e.put(child.Token, "lead-own", map[string]any{"cron": "0 9 * * *"}, http.StatusForbidden)
	if status, _ := e.do(http.MethodDelete, "/api/schedules/lead-own", child.Token, nil); status != http.StatusForbidden {
		t.Fatalf("delegated child DELETE: %d, want 403", status)
	}

	if status, _ := e.do(http.MethodGet, "/api/schedules/lead-own", e.leaf.Token, nil); status != http.StatusOK {
		t.Fatalf("stranger GET: %d, want 200", status)
	}
	if status, _ := e.do(http.MethodDelete, "/api/schedules/lead-own", e.leaf.Token, nil); status != http.StatusForbidden {
		t.Fatalf("stranger DELETE: %d, want 403", status)
	}
}

func TestScheduleFiredIsBroadcastOnEvents(t *testing.T) {
	e := newScheduleEnv(t)
	e.put(e.lead.Token, "nudge", map[string]any{"cron": "0 9 * * *", "to": "leaf-1", "subject": "go"}, http.StatusOK)

	ch, unsubscribe := e.api.events.subscribe()
	defer unsubscribe()

	if status, raw := e.do(http.MethodPost, "/api/schedules/nudge/fire", e.adminToken("ops"), nil); status != http.StatusAccepted {
		t.Fatalf("fire as admin: %d %s", status, raw)
	}

	select {
	case ev := <-ch:
		if ev.Type != "schedule_fired" {
			t.Fatalf("event type = %q", ev.Type)
		}
		raw, _ := json.Marshal(ev.Data)
		var v scheduleFiredView
		decodeInto(t, raw, &v)
		if v.ScheduleID != "nudge" || v.To != "leaf-1" || v.MessageID == "" {
			t.Fatalf("event = %s", raw)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no schedule_fired event")
	}
}

func TestEmitRefusesScheduledType(t *testing.T) {
	e := newScheduleEnv(t)
	status, raw := e.do(http.MethodPost, "/api/messages", e.lead.Token, map[string]any{
		"type": "scheduled", "to": "leaf-1", "causation_id": "forged",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status %d, want 400: %s", status, raw)
	}
}
