package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/acumen-ai-org/robotdreams/pkg/reporting"
)

func reportInstance(def, scope string, at time.Time) *reporting.Instance {
	return &reporting.Instance{
		Definition: def,
		Scope:      scope,
		Producer:   "worker-1",
		ProducedAt: at,
		KPIs:       map[string]float64{"n": 1},
	}
}

func TestReportInstanceRoundtripAndScopeFilter(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

	for i, scope := range []string{"acme/music", "acme/ads", "other"} {
		if err := s.InsertReportInstance(ctx, reportInstance("deploy", scope, base.Add(time.Duration(i)*time.Minute))); err != nil {
			t.Fatalf("InsertReportInstance: %v", err)
		}
	}
	if err := s.InsertReportInstance(ctx, reportInstance("errors", "acme/music", base)); err != nil {
		t.Fatalf("InsertReportInstance: %v", err)
	}

	got, err := s.ListReportInstances(ctx, "deploy", "acme")
	if err != nil {
		t.Fatalf("ListReportInstances: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("instances under acme for deploy = %d, want 2", len(got))
	}
	if got[0].Scope != "acme/music" || got[1].Scope != "acme/ads" {
		t.Fatalf("unexpected order/scopes: %q, %q", got[0].Scope, got[1].Scope)
	}
	if got[0].Producer != "worker-1" || got[0].KPIs["n"] != 1 {
		t.Fatalf("payload did not round-trip: %+v", got[0])
	}

	all, err := s.ListReportInstances(ctx, "", "")
	if err != nil {
		t.Fatalf("ListReportInstances(all): %v", err)
	}
	if len(all) != 4 {
		t.Fatalf("all instances = %d, want 4", len(all))
	}
}

func TestReportInstanceRetentionPrune(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)

	total := ReportInstanceRetention + 5
	for i := 0; i < total; i++ {
		if err := s.InsertReportInstance(ctx, reportInstance("deploy", "acme/music", base.Add(time.Duration(i)*time.Second))); err != nil {
			t.Fatalf("InsertReportInstance #%d: %v", i, err)
		}
	}
	if err := s.InsertReportInstance(ctx, reportInstance("deploy", "acme/ads", base)); err != nil {
		t.Fatalf("InsertReportInstance other scope: %v", err)
	}

	got, err := s.ListReportInstances(ctx, "deploy", "acme/music")
	if err != nil {
		t.Fatalf("ListReportInstances: %v", err)
	}
	if len(got) != ReportInstanceRetention {
		t.Fatalf("retained instances = %d, want %d", len(got), ReportInstanceRetention)
	}
	if want := base.Add(5 * time.Second); !got[0].ProducedAt.Equal(want) {
		t.Fatalf("oldest retained produced_at = %v, want %v", got[0].ProducedAt, want)
	}

	other, err := s.ListReportInstances(ctx, "deploy", "acme/ads")
	if err != nil {
		t.Fatalf("ListReportInstances other scope: %v", err)
	}
	if len(other) != 1 {
		t.Fatalf("other scope's bucket = %d instances, want 1", len(other))
	}
}

func TestReportEventsRoundtripFiltersAndPrune(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)

	evs := []reporting.Event{
		{T: base, Type: "deploy_started", Severity: "info", Label: "v1", Attrs: map[string]string{"svc": "api"}},
		{T: base.Add(time.Minute), Type: "deploy_failed", Severity: "critical", Label: "boom"},
	}
	if err := s.InsertReportEvents(ctx, "deploy", "acme/music", evs); err != nil {
		t.Fatalf("InsertReportEvents: %v", err)
	}
	if err := s.InsertReportEvents(ctx, "errors", "acme/ads", []reporting.Event{
		{T: base.Add(2 * time.Minute), Type: "error", Severity: "warn"},
	}); err != nil {
		t.Fatalf("InsertReportEvents(errors): %v", err)
	}

	got, err := s.ListReportEvents(ctx, ReportEventFilter{})
	if err != nil {
		t.Fatalf("ListReportEvents: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("events = %d, want 3", len(got))
	}
	if got[0].Definition != "deploy" || got[0].Scope != "acme/music" || got[0].Attrs["svc"] != "api" {
		t.Fatalf("event did not round-trip with origin tags: %+v", got[0])
	}
	if !got[0].T.Equal(base) || !got[2].T.Equal(base.Add(2*time.Minute)) {
		t.Fatalf("events out of time order: %v .. %v", got[0].T, got[2].T)
	}

	got, err = s.ListReportEvents(ctx, ReportEventFilter{Definitions: []string{"errors"}})
	if err != nil {
		t.Fatalf("ListReportEvents(errors): %v", err)
	}
	if len(got) != 1 || got[0].Type != "error" {
		t.Fatalf("definition filter: got %+v", got)
	}

	got, err = s.ListReportEvents(ctx, ReportEventFilter{ScopePrefix: "acme/music"})
	if err != nil {
		t.Fatalf("ListReportEvents(scope): %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("scope filter: %d events, want 2", len(got))
	}

	got, err = s.ListReportEvents(ctx, ReportEventFilter{Since: base})
	if err != nil {
		t.Fatalf("ListReportEvents(since): %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("since filter: %d events, want 2 (strictly after)", len(got))
	}

	batch := make([]reporting.Event, 0, ReportEventRetention+50)
	for i := 0; i < ReportEventRetention+50; i++ {
		batch = append(batch, reporting.Event{
			T:        base.Add(time.Duration(i) * time.Second),
			Type:     "deploy_started",
			Severity: "info",
			Label:    fmt.Sprintf("e%d", i),
		})
	}
	if err := s.InsertReportEvents(ctx, "deploy", "acme/music", batch); err != nil {
		t.Fatalf("InsertReportEvents(bulk): %v", err)
	}
	got, err = s.ListReportEvents(ctx, ReportEventFilter{Definitions: []string{"deploy"}, ScopePrefix: "acme/music"})
	if err != nil {
		t.Fatalf("ListReportEvents(after prune): %v", err)
	}
	if len(got) != ReportEventRetention {
		t.Fatalf("retained events = %d, want %d", len(got), ReportEventRetention)
	}
	other, err := s.ListReportEvents(ctx, ReportEventFilter{Definitions: []string{"errors"}})
	if err != nil {
		t.Fatalf("ListReportEvents(errors after prune): %v", err)
	}
	if len(other) != 1 {
		t.Fatalf("other bucket = %d events, want 1", len(other))
	}
}

func TestReportScopesCounts(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)

	for i, scope := range []string{"acme/music", "acme/music", "acme"} {
		if err := s.InsertReportInstance(ctx, reportInstance("deploy", scope, base.Add(time.Duration(i)*time.Second))); err != nil {
			t.Fatalf("InsertReportInstance: %v", err)
		}
	}

	got, err := s.ListReportScopes(ctx, time.Time{})
	if err != nil {
		t.Fatalf("ListReportScopes: %v", err)
	}
	want := []ReportScopeCount{{Path: "acme", Instances: 1}, {Path: "acme/music", Instances: 2}}
	if len(got) != len(want) {
		t.Fatalf("scopes = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("scopes[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestReportScopesSince(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)

	for _, in := range []struct {
		scope string
		at    time.Time
	}{
		{"acme/music", base},
		{"acme/music", base.Add(48 * time.Hour)},
		{"acme/music", base.Add(49 * time.Hour)},
		{"acme", base.Add(time.Second)},
	} {
		if err := s.InsertReportInstance(ctx, reportInstance("deploy", in.scope, in.at)); err != nil {
			t.Fatalf("InsertReportInstance: %v", err)
		}
	}

	got, err := s.ListReportScopes(ctx, base.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("ListReportScopes: %v", err)
	}
	want := []ReportScopeCount{
		{Path: "acme", Instances: 1, Recent: 0},
		{Path: "acme/music", Instances: 3, Recent: 2},
	}
	if len(got) != len(want) {
		t.Fatalf("scopes = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("scopes[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestReportInstanceWindow(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)

	for i := 0; i < 4; i++ {
		if err := s.InsertReportInstance(ctx, reportInstance("deploy", "acme/music", base.AddDate(0, 0, i))); err != nil {
			t.Fatalf("InsertReportInstance #%d: %v", i, err)
		}
	}

	got, err := s.ListReportInstances(ctx, "deploy", "acme", ReportInstanceWindow{From: base.AddDate(0, 0, 1), Until: base.AddDate(0, 0, 3)})
	if err != nil {
		t.Fatalf("ListReportInstances(window): %v", err)
	}
	if len(got) != 2 || !got[0].ProducedAt.Equal(base.AddDate(0, 0, 1)) || !got[1].ProducedAt.Equal(base.AddDate(0, 0, 2)) {
		t.Fatalf("windowed instances = %d (%v), want the two at +1d and +2d", len(got), got)
	}

	got, err = s.ListReportInstances(ctx, "deploy", "acme", ReportInstanceWindow{Until: base.AddDate(0, 0, 1)})
	if err != nil {
		t.Fatalf("ListReportInstances(until): %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("until-only = %d instances, want 1", len(got))
	}
	got, err = s.ListReportInstances(ctx, "deploy", "acme", ReportInstanceWindow{From: base.AddDate(0, 0, 3)})
	if err != nil {
		t.Fatalf("ListReportInstances(from): %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("from-only = %d instances, want 1", len(got))
	}

	got, err = s.ListReportInstances(ctx, "deploy", "acme", ReportInstanceWindow{})
	if err != nil {
		t.Fatalf("ListReportInstances(zero window): %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("zero window = %d instances, want 4", len(got))
	}
}

func TestReportEventsUntil(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)

	var evs []reporting.Event
	for i := 0; i < 4; i++ {
		evs = append(evs, reporting.Event{T: base.Add(time.Duration(i) * time.Hour), Type: "error", Severity: "warn", Label: fmt.Sprint(i)})
	}
	if err := s.InsertReportEvents(ctx, "errors", "acme/music", evs); err != nil {
		t.Fatalf("InsertReportEvents: %v", err)
	}

	got, err := s.ListReportEvents(ctx, ReportEventFilter{Since: base, Until: base.Add(3 * time.Hour)})
	if err != nil {
		t.Fatalf("ListReportEvents: %v", err)
	}
	if len(got) != 2 || got[0].Label != "1" || got[1].Label != "2" {
		t.Fatalf("bounded events = %+v, want labels 1 and 2", got)
	}
}

func TestReportTimeLayoutSortsLexicographically(t *testing.T) {
	base := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	times := []time.Time{
		base,
		base.Add(1 * time.Nanosecond),
		base.Add(100 * time.Millisecond),
		base.Add(1 * time.Second),
		base.Add(1 * time.Second).In(time.FixedZone("plus", 3600)),
		base.Add(24 * time.Hour),
	}
	for i := 1; i < len(times); i++ {
		prev, cur := formatReportTime(times[i-1]), formatReportTime(times[i])
		if len(prev) != len(reportTimeLayout) || len(cur) != len(reportTimeLayout) {
			t.Fatalf("formatReportTime is not fixed width: %q %q", prev, cur)
		}
		if times[i-1].Before(times[i]) && prev >= cur {
			t.Fatalf("lexicographic order disagrees with chronological: %q !< %q", prev, cur)
		}
		if times[i-1].Equal(times[i]) && prev != cur {
			t.Fatalf("equal instants encode differently: %q != %q", prev, cur)
		}
		got, err := parseReportTime(cur)
		if err != nil {
			t.Fatalf("parseReportTime(%q): %v", cur, err)
		}
		if !got.Equal(times[i]) {
			t.Fatalf("parseReportTime(%q) = %v, want %v", cur, got, times[i])
		}
	}
}
