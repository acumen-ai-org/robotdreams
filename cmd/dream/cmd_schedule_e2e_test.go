package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/acumen-ai-org/robotdreams/pkg/scheduling"
)

func TestScheduleListDeleteFireRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	addr, srv := startTestServer(t)
	ctx := context.Background()
	connectPair(t, addr)

	out := mustRunDreamCmd(t, "schedule", "list", "--server", addr, "--worker-id", "lead")
	if strings.TrimSpace(out) != "no schedules" {
		t.Fatalf("empty list = %q", out)
	}
	out = mustRunDreamCmd(t, "schedule", "list", "--server", addr, "--worker-id", "lead", "--json")
	if strings.TrimSpace(out) != "" {
		t.Fatalf("empty --json list should print nothing, got %q", out)
	}

	out = mustRunDreamCmd(t, "schedule", "create", "--server", addr, "--worker-id", "lead",
		"--id", "daily-standup", "--cron", "0 9 * * *", "--to", "leaf", "--subject", "standup")
	if !strings.Contains(out, "registered schedule daily-standup: 0 9 * * * from lead to leaf") {
		t.Fatalf("create output = %q", out)
	}

	out = mustRunDreamCmd(t, "schedule", "list", "--server", addr, "--worker-id", "lead")
	for _, want := range []string{"daily-standup", "0 9 * * *", "to leaf", "standup", "last never"} {
		if !strings.Contains(out, want) {
			t.Errorf("list output missing %q:\n%s", want, out)
		}
	}
	out = mustRunDreamCmd(t, "schedule", "list", "--server", addr, "--worker-id", "lead", "--json")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 {
		t.Fatalf("--json printed %d lines, want 1:\n%s", len(lines), out)
	}
	var got scheduleView
	if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
		t.Fatalf("--json line is not JSON: %v (%q)", err, lines[0])
	}
	if got.ID != "daily-standup" || got.Owner != "lead" || got.To != "leaf" || !got.Enabled {
		t.Fatalf("json schedule = %+v", got)
	}

	list, err := runScheduleList(ctx, scheduleListOptions{Server: addr, AsWorker: "lead", Worker: "leaf"})
	if err != nil {
		t.Fatalf("list --worker leaf: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("leaf sees %d schedules, want 1", len(list))
	}
	list, err = runScheduleList(ctx, scheduleListOptions{Server: addr, AsWorker: "lead", Worker: "nobody"})
	if err != nil {
		t.Fatalf("list --worker nobody: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("unrelated worker sees %d schedules, want 0", len(list))
	}

	before, err := srv.Schedules().Get(ctx, "daily-standup")
	if err != nil {
		t.Fatalf("server Get: %v", err)
	}
	out = mustRunDreamCmd(t, "schedule", "fire", "daily-standup", "--server", addr, "--worker-id", "lead")
	if !strings.Contains(out, "fired schedule daily-standup: sent scheduled ") ||
		!strings.Contains(out, "from lead to leaf; next still at "+formatTime(before.NextAt)) {
		t.Fatalf("fire output = %q", out)
	}
	after, err := srv.Schedules().Get(ctx, "daily-standup")
	if err != nil {
		t.Fatalf("server Get after fire: %v", err)
	}
	if after.LastAt == nil {
		t.Fatal("fire did not record last_at")
	}
	if !after.NextAt.Equal(before.NextAt) {
		t.Fatalf("a manual fire moved next_at from %s to %s", before.NextAt, after.NextAt)
	}
	msgs, err := runMessageTail(ctx, messageTailOptions{Server: addr, AsWorker: "leaf"})
	if err != nil {
		t.Fatalf("tail leaf: %v", err)
	}
	var found bool
	for _, m := range msgs {
		if m.Type == "scheduled" && m.From == "lead" && m.Subject == "standup" && m.CausationID == "daily-standup" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no scheduled message in the leaf's inbox: %+v", msgs)
	}
	out = mustRunDreamCmd(t, "schedule", "list", "--server", addr, "--worker-id", "lead")
	if strings.Contains(out, "last never") {
		t.Fatalf("list after fire still says never:\n%s", out)
	}

	out = mustRunDreamCmd(t, "schedule", "delete", "daily-standup", "--server", addr, "--worker-id", "lead")
	if strings.TrimSpace(out) != "deleted schedule daily-standup" {
		t.Fatalf("delete output = %q", out)
	}
	if _, err := srv.Schedules().Get(ctx, "daily-standup"); !errors.Is(err, scheduling.ErrNotFound) {
		t.Fatalf("schedule still on the server after delete: %v", err)
	}
	out = mustRunDreamCmd(t, "schedule", "list", "--server", addr, "--worker-id", "lead")
	if strings.TrimSpace(out) != "no schedules" {
		t.Fatalf("list after delete = %q", out)
	}
}

func TestScheduleDeleteFireAuthorization(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	addr, _ := startTestServer(t)
	ctx := context.Background()
	connectPair(t, addr)

	if _, err := runScheduleCreate(ctx, scheduleCreateOptions{
		Server: addr, AsWorker: "lead", ID: "owned-by-lead", Cron: "*/5 * * * *", To: "leaf",
	}, strings.NewReader("")); err != nil {
		t.Fatalf("create: %v", err)
	}

	wantStatus := func(t *testing.T, err error, status int) {
		t.Helper()
		var apiErr *apiError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != status {
			t.Fatalf("got %v, want HTTP %d", err, status)
		}
	}

	err := runScheduleDelete(ctx, scheduleIDOptions{Server: addr, AsWorker: "leaf", ID: "owned-by-lead"})
	wantStatus(t, err, http.StatusForbidden)
	_, err = runScheduleFire(ctx, scheduleIDOptions{Server: addr, AsWorker: "leaf", ID: "owned-by-lead"})
	wantStatus(t, err, http.StatusForbidden)

	err = runScheduleDelete(ctx, scheduleIDOptions{Server: addr, AsWorker: "lead", ID: "no-such"})
	wantStatus(t, err, http.StatusNotFound)
	_, err = runScheduleFire(ctx, scheduleIDOptions{Server: addr, AsWorker: "lead", ID: "no-such"})
	wantStatus(t, err, http.StatusNotFound)

	if err := runScheduleDelete(ctx, scheduleIDOptions{Server: "127.0.0.1:1"}); err == nil || !strings.Contains(err.Error(), "schedule id is required") {
		t.Fatalf("delete with no id: %v", err)
	}
	if _, err := runScheduleFire(ctx, scheduleIDOptions{Server: "127.0.0.1:1"}); err == nil || !strings.Contains(err.Error(), "schedule id is required") {
		t.Fatalf("fire with no id: %v", err)
	}

	if _, err := runDreamCmd(t, "schedule", "delete", "owned-by-lead", "--server", addr, "--worker-id", "leaf"); err == nil {
		t.Fatal("`dream schedule delete` by a non-owner should fail")
	}
}

func TestScheduleListWithoutIdentityFails(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, err := runScheduleList(context.Background(), scheduleListOptions{})
	if err == nil || !strings.Contains(err.Error(), "no connected server found") {
		t.Fatalf("got %v, want the no-connected-server error", err)
	}
}
