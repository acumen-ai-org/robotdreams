package reporting

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)

// aggregateInstances builds four in-scope instances (two from the same
// scope, so latest-wins is exercised) plus two that must be filtered out.
func aggregateInstances() []*Instance {
	return []*Instance{
		{ // superseded by the newer instance from the same scope
			Definition: "delivery",
			Scope:      "acme/music/platform/a",
			ProducedAt: t0,
			Headline:   "stale",
			KPIs:       map[string]float64{"deploys": 100, "failed_deploys": 9},
		},
		{
			Definition: "delivery",
			Scope:      "acme/music/platform/a",
			ProducedAt: t0.Add(3 * time.Hour),
			KPIs:       map[string]float64{"deploys": 4, "failed_deploys": 0, "lead_time_minutes": 30},
			Series: map[string][]SeriesPoint{
				"deploy_frequency": {{T: t0.Add(1 * time.Hour), V: 1}, {T: t0.Add(3 * time.Hour), V: 3}},
			},
		},
		{
			Definition: "delivery",
			Scope:      "acme/music/platform/b",
			ProducedAt: t0.Add(1 * time.Hour),
			Headline:   "b shipped",
			KPIs:       map[string]float64{"deploys": 2, "failed_deploys": 1, "lead_time_minutes": 10},
			Series: map[string][]SeriesPoint{
				"deploy_frequency": {{T: t0.Add(2 * time.Hour), V: 2}},
			},
		},
		{
			Definition: "delivery",
			Scope:      "acme/music/search/c",
			ProducedAt: t0.Add(2 * time.Hour),
			Headline:   "search shipped",
			KPIs:       map[string]float64{"deploys": 6, "failed_deploys": 0, "lead_time_minutes": 50},
		},
		{ // outside the target scope
			Definition: "delivery",
			Scope:      "acme/video/x",
			ProducedAt: t0,
			KPIs:       map[string]float64{"deploys": 999},
		},
		{ // different definition
			Definition: "incident",
			Scope:      "acme/music/platform/a",
			ProducedAt: t0,
			KPIs:       map[string]float64{"deploys": 999},
		},
	}
}

func TestAggregate(t *testing.T) {
	def := testDef()
	view := Aggregate(def, "acme/music", aggregateInstances())

	if view.Definition != "delivery" || view.Scope != "acme/music" {
		t.Errorf("Definition/Scope = %q/%q", view.Definition, view.Scope)
	}
	if view.Instances != 3 {
		t.Errorf("Instances = %d, want 3 (latest per scope, filtered)", view.Instances)
	}
	wantScopes := []string{"acme/music/platform/a", "acme/music/platform/b", "acme/music/search/c"}
	if strings.Join(view.Scopes, ",") != strings.Join(wantScopes, ",") {
		t.Errorf("Scopes = %v, want %v", view.Scopes, wantScopes)
	}
	if !view.LastProducedAt.Equal(t0.Add(3 * time.Hour)) {
		t.Errorf("LastProducedAt = %v, want %v", view.LastProducedAt, t0.Add(3*time.Hour))
	}
	if got := strings.Join(view.Facets, ","); got != "summary,timeline,pulse" {
		t.Errorf("Facets = %v", view.Facets)
	}

	// Status: instance b trips the warn threshold (failed_deploys >= 1),
	// the others are ok; worst wins.
	if view.Status != StatusWarn {
		t.Errorf("Status = %q, want warn", view.Status)
	}

	// Headline: synthesize degrades to top(1) — the most recent instance —
	// and that instance has no precomputed headline, so the summary
	// template is rendered against its kpis.
	if view.Headline != "4 deploys, 0 failed" {
		t.Errorf("Headline = %q, want rendered template", view.Headline)
	}

	// KPIs in summary-facet order, aggregated per declared policy.
	if len(view.KPIs) != 3 {
		t.Fatalf("KPIs = %+v, want 3 entries", view.KPIs)
	}
	checks := []struct {
		name   string
		value  float64
		unit   string
		policy string
	}{
		{"deploys", 12, "count", "sum"},             // 4+2+6, stale 100 superseded
		{"failed_deploys", 1, "count", "sum"},       // 0+1+0
		{"lead_time_minutes", 30, "minutes", "p50"}, // median of 10, 30, 50
	}
	for i, want := range checks {
		got := view.KPIs[i]
		if got.Name != want.name || got.Value != want.value || got.Unit != want.unit || got.Policy != want.policy {
			t.Errorf("KPIs[%d] = %+v, want %+v", i, got, want)
		}
	}

	// Sparkline: merged across instances, time ascending.
	if len(view.Sparkline) != 3 {
		t.Fatalf("Sparkline = %+v, want 3 points", view.Sparkline)
	}
	for i, wantV := range []float64{1, 2, 3} {
		if view.Sparkline[i].V != wantV {
			t.Errorf("Sparkline[%d].V = %v, want %v (time-sorted merge)", i, view.Sparkline[i].V, wantV)
		}
	}
}

func TestAggregateHeadlineTopN(t *testing.T) {
	def := testDef()
	def.Scope.Aggregation.Headline = "top(2)"
	view := Aggregate(def, "acme/music", aggregateInstances())
	// Most recent first: the rendered template for instance a, then the
	// precomputed headline of instance c.
	want := "4 deploys, 0 failed; search shipped"
	if view.Headline != want {
		t.Errorf("Headline = %q, want %q", view.Headline, want)
	}
}

func TestAggregateStatusOverrideAndCritical(t *testing.T) {
	def := testDef()
	insts := aggregateInstances()

	t.Run("critical from status rule", func(t *testing.T) {
		insts[3].KPIs["failed_deploys"] = 5 // >= critical_at 3
		defer func() { insts[3].KPIs["failed_deploys"] = 0 }()
		if got := Aggregate(def, "acme/music", insts).Status; got != StatusCritical {
			t.Errorf("Status = %q, want critical", got)
		}
	})

	t.Run("explicit override wins", func(t *testing.T) {
		insts[3].Status = StatusCritical
		defer func() { insts[3].Status = "" }()
		if got := Aggregate(def, "acme/music", insts).Status; got != StatusCritical {
			t.Errorf("Status = %q, want critical from override", got)
		}
	})

	t.Run("latest policy", func(t *testing.T) {
		def2 := testDef()
		def2.Scope.Aggregation.Status = "latest"
		// The most recent instance (a, t0+3h) is ok even though b is warn.
		if got := Aggregate(def2, "acme/music", insts).Status; got != StatusOK {
			t.Errorf("Status = %q, want ok under latest policy", got)
		}
	})
}

func TestAggregateEmpty(t *testing.T) {
	def := testDef()
	view := Aggregate(def, "acme/music", nil)
	if view.Instances != 0 || len(view.Scopes) != 0 {
		t.Errorf("Instances/Scopes = %d/%v, want empty", view.Instances, view.Scopes)
	}
	if view.Status != "" || view.Headline != "" {
		t.Errorf("Status/Headline = %q/%q, want empty", view.Status, view.Headline)
	}
	if !view.LastProducedAt.IsZero() {
		t.Errorf("LastProducedAt = %v, want zero", view.LastProducedAt)
	}
}

func TestMergeEvents(t *testing.T) {
	ev := func(offset time.Duration, sev string) Event {
		return Event{T: t0.Add(offset), Type: "e", Severity: sev, Scope: "acme/a"}
	}
	batchA := []Event{ev(1*time.Hour, SeverityCritical), ev(3*time.Hour, SeverityCritical), ev(5*time.Hour, SeverityWarn)}
	batchB := []Event{ev(2*time.Hour, SeverityInfo), ev(4*time.Hour, SeverityInfo), ev(6*time.Hour, SeverityInfo)}

	t.Run("no cap merges sorted", func(t *testing.T) {
		got := MergeEvents(0, batchA, batchB)
		if len(got) != 6 {
			t.Fatalf("len = %d, want 6", len(got))
		}
		for i := 1; i < len(got); i++ {
			if got[i].T.Before(got[i-1].T) {
				t.Fatalf("events not sorted ascending: %v", got)
			}
		}
		if got[0].Scope != "acme/a" {
			t.Errorf("origin tags not preserved: %+v", got[0])
		}
	})

	t.Run("cap keeps criticals over recency", func(t *testing.T) {
		got := MergeEvents(4, batchA, batchB)
		if len(got) != 4 {
			t.Fatalf("len = %d, want 4", len(got))
		}
		// Both criticals (t+1h, t+3h) survive even though they are the
		// oldest; the rest of the budget goes to the most recent events.
		wantOffsets := []time.Duration{1 * time.Hour, 3 * time.Hour, 5 * time.Hour, 6 * time.Hour}
		for i, off := range wantOffsets {
			if !got[i].T.Equal(t0.Add(off)) {
				t.Errorf("got[%d].T = %v, want t0+%v", i, got[i].T, off)
			}
		}
	})

	t.Run("cap below critical count keeps most recent criticals", func(t *testing.T) {
		got := MergeEvents(1, batchA, batchB)
		if len(got) != 1 || got[0].Severity != SeverityCritical || !got[0].T.Equal(t0.Add(3*time.Hour)) {
			t.Errorf("got = %+v, want the most recent critical only", got)
		}
	})
}

func TestFormatKPI(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{14, "14"},
		{32.5, "32.5"},
		{0, "0"},
		{33.333333, "33.33"},
		{1234.567, "1234.57"},
		{-2.5, "-2.5"},
		{-0.001, "0"},
		{0.5, "0.5"},
	}
	for _, tt := range tests {
		if got := FormatKPI(tt.in); got != tt.want {
			t.Errorf("FormatKPI(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// f is a pointer-to-float helper for target literals.
func f(v float64) *float64 { return &v }

// targetDef is a two-KPI definition where one KPI sums and the other
// averages, which is the case that matters: a target must roll up the
// same way its metric does or the variance is meaningless above a leaf.
func targetDef() *Definition {
	return &Definition{
		Version: DefinitionVersion, Kind: DefinitionKind, Name: "close",
		Categories: []string{"delivery"},
		Scope: ScopeSpec{
			Attach:      []string{"site"},
			Aggregation: AggregationSpec{KPIs: map[string]string{"entries": "sum", "days_to_close": "avg"}},
		},
		Data: DataContract{KPIs: []KPISpec{
			{Name: "entries", Unit: "count"},
			{Name: "days_to_close", Unit: "days", Target: f(6), Direction: "above"},
		}},
	}
}

// TestAggregateTargets: an instance target overrides the definition
// default, a missing one falls back to it, and both roll up with the
// KPI's own policy.
func TestAggregateTargets(t *testing.T) {
	insts := []*Instance{
		{
			Definition: "close", Scope: "acme/finance/accounting/a", ProducedAt: t0,
			KPIs:    map[string]float64{"entries": 10, "days_to_close": 8},
			Targets: map[string]float64{"entries": 4, "days_to_close": 5},
		},
		{
			// No targets of its own: inherits the definition's 6 days,
			// and contributes no target at all for entries.
			Definition: "close", Scope: "acme/finance/accounting/b", ProducedAt: t0,
			KPIs: map[string]float64{"entries": 6, "days_to_close": 4},
		},
	}

	view := Aggregate(targetDef(), "acme/finance/accounting", insts)
	got := map[string]KPIValue{}
	for _, k := range view.KPIs {
		got[k.Name] = k
	}

	// entries sums (10+6=16); only one instance declared a target, so
	// the summed target is that one's 4.
	entries := got["entries"]
	if entries.Value != 16 {
		t.Errorf("entries value = %v, want 16", entries.Value)
	}
	if entries.Target == nil || *entries.Target != 4 {
		t.Fatalf("entries target = %v, want 4", entries.Target)
	}
	if *entries.Variance != 12 {
		t.Errorf("entries variance = %v, want 12", *entries.Variance)
	}

	// days_to_close averages ((8+4)/2=6); targets average too, the
	// instance's 5 with the definition default 6 -> 5.5.
	days := got["days_to_close"]
	if days.Value != 6 {
		t.Errorf("days value = %v, want 6", days.Value)
	}
	if days.Target == nil || *days.Target != 5.5 {
		t.Fatalf("days target = %v, want 5.5", days.Target)
	}
	if *days.Variance != 0.5 {
		t.Errorf("days variance = %v, want 0.5", *days.Variance)
	}
	if days.Direction != "above" {
		t.Errorf("days direction = %q, want above", days.Direction)
	}
}

// TestAggregateNoTargetStaysBare: a KPI nobody set a target for must
// come back exactly as it did before targets existed — nil, not zero,
// so a consumer can tell "no target" from "target of zero".
func TestAggregateNoTargetStaysBare(t *testing.T) {
	def := targetDef()
	def.Data.KPIs[1].Target, def.Data.KPIs[1].Direction = nil, ""

	view := Aggregate(def, "acme/finance/accounting", []*Instance{{
		Definition: "close", Scope: "acme/finance/accounting/a", ProducedAt: t0,
		KPIs: map[string]float64{"entries": 3, "days_to_close": 7},
	}})
	for _, k := range view.KPIs {
		if k.Target != nil || k.Variance != nil {
			t.Errorf("kpi %s: got target %v variance %v, want both nil", k.Name, k.Target, k.Variance)
		}
	}
}

// TestHeadlineMatchesAggregatedKPIs is the guard for a failure that is
// invisible from the outside: a tile whose sentence and whose numbers
// come from different places. Before this, a summary at a scope with
// nineteen contributors rendered its template against ONE of them, so a
// report could read "1 blocked" beside a blocked count of 18.
func TestHeadlineMatchesAggregatedKPIs(t *testing.T) {
	def := &Definition{
		Version: DefinitionVersion, Kind: DefinitionKind, Name: "work",
		Categories: []string{"activity"},
		Scope: ScopeSpec{
			Attach: []string{"site"},
			Aggregation: AggregationSpec{
				Status:   "worst",
				Headline: "synthesize",
				KPIs:     map[string]string{"blocked": "sum", "active": "sum"},
			},
		},
		Data: DataContract{KPIs: []KPISpec{
			{Name: "blocked", Unit: "count"},
			{Name: "active", Unit: "count"},
		}},
		Facets: Facets{Summary: &SummaryFacet{
			KPIs:     []string{"blocked", "active"},
			Headline: "{{active}} active, {{blocked}} blocked",
		}},
	}

	var instances []*Instance
	for i, scope := range []string{"acme/a", "acme/b", "acme/c"} {
		instances = append(instances, &Instance{
			Definition: "work", Scope: scope,
			ProducedAt: t0.Add(time.Duration(i) * time.Minute),
			KPIs:       map[string]float64{"blocked": float64(i + 1), "active": 10},
		})
	}

	view := Aggregate(def, "acme", instances)
	byName := map[string]KPIValue{}
	for _, k := range view.KPIs {
		byName[k.Name] = k
	}
	if got := byName["blocked"].Value; got != 6 {
		t.Fatalf("blocked = %v, want 6", got)
	}
	// The sentence must carry the same 6 and 30 the KPI row does.
	if want := "30 active, 6 blocked"; view.Headline != want {
		t.Errorf("headline = %q, want %q", view.Headline, want)
	}
	if view.HeadlinePartial {
		t.Error("HeadlinePartial: a template rendered from the roll-up is not partial")
	}

	// Prose from a producer cannot be recomputed, so it degrades to one
	// contributor's — and must SAY it is standing in for the others.
	for _, in := range instances {
		in.Headline = "hand-written for " + in.Scope
	}
	prose := Aggregate(def, "acme", instances)
	if !prose.HeadlinePartial {
		t.Error("HeadlinePartial: precomputed prose across 3 scopes must be marked partial")
	}
	if prose.Headline != "hand-written for acme/c" {
		t.Errorf("degraded headline = %q, want the most recent contributor's", prose.Headline)
	}

	// One contributor is the whole picture, so nothing is partial.
	one := Aggregate(def, "acme/a", instances[:1])
	if one.HeadlinePartial {
		t.Error("HeadlinePartial: a single contributor is not partial")
	}
}

// The aggregate says which instances it was computed from, and who wrote
// each one. The producer is recorded at ingest and was stored but never
// read back; without it a reader of a rolled-up number cannot ask which
// node actually said it.
func TestAggregateContributors(t *testing.T) {
	def := testDef()
	instances := []*Instance{
		{ // superseded by the newer instance from the same scope
			Definition: "delivery",
			Scope:      "acme/music/platform/a",
			Producer:   "old-a",
			ProducedAt: t0,
			KPIs:       map[string]float64{"deploys": 1},
		},
		{
			Definition: "delivery",
			Scope:      "acme/music/platform/a",
			Producer:   "platform-a-lead",
			ProducedAt: t0.Add(2 * time.Hour),
			KPIs:       map[string]float64{"deploys": 4},
		},
		{
			Definition: "delivery",
			Scope:      "acme/music/search/c",
			ProducedAt: t0.Add(1 * time.Hour), // no producer: legal, and must not be invented
			KPIs:       map[string]float64{"deploys": 6},
		},
	}

	view := Aggregate(def, "acme/music", instances)

	if len(view.Contributors) != 2 {
		t.Fatalf("Contributors = %+v, want 2 (newest per scope)", view.Contributors)
	}
	// Contributors() orders by produced_at ascending, and that order is kept.
	if view.Contributors[0].Scope != "acme/music/search/c" || view.Contributors[0].Producer != "" {
		t.Errorf("Contributors[0] = %+v, want the search scope with no producer", view.Contributors[0])
	}
	if view.Contributors[1].Scope != "acme/music/platform/a" || view.Contributors[1].Producer != "platform-a-lead" {
		t.Errorf("Contributors[1] = %+v, want platform/a written by platform-a-lead", view.Contributors[1])
	}
	if !view.Contributors[1].ProducedAt.Equal(t0.Add(2 * time.Hour)) {
		t.Errorf("Contributors[1].ProducedAt = %v, want the newer instance's time", view.Contributors[1].ProducedAt)
	}
	// The superseded instance's producer must not appear at all.
	for _, c := range view.Contributors {
		if c.Producer == "old-a" {
			t.Errorf("superseded instance contributed: %+v", c)
		}
	}
	// Scopes keeps its own contract: sorted by name, unchanged.
	if strings.Join(view.Scopes, ",") != "acme/music/platform/a,acme/music/search/c" {
		t.Errorf("Scopes = %v, want name-sorted", view.Scopes)
	}
}

// activityDef is a definition in the shape of reporting/library/activity.yaml:
// counters that sum over time, gauges that do not, a status rule on a
// gauge, and a headline template.
func activityDef() *Definition {
	return &Definition{
		Version: DefinitionVersion, Kind: DefinitionKind, Name: "activity",
		Categories: []string{"activity"},
		Scope: ScopeSpec{
			Attach: []string{"site", "realm"},
			Aggregation: AggregationSpec{
				Status: "worst", Headline: "latest",
				KPIs: map[string]string{"active_nodes": "sum", "tokens_in": "sum", "tokens_out": "sum"},
				Time: &TimeAggregationSpec{KPIs: map[string]string{"tokens_in": "sum", "tokens_out": "sum"}},
			},
		},
		Data: DataContract{
			KPIs: []KPISpec{
				{Name: "active_nodes", Unit: "count"},
				{Name: "tokens_in", Unit: "count"},
				{Name: "tokens_out", Unit: "count"},
			},
			Series: []SeriesSpec{{Name: "load", Unit: "per-hour"}},
		},
		Facets: Facets{
			Summary: &SummaryFacet{
				Status:    &StatusRule{From: "active_nodes", WarnAt: 5, CriticalAt: 10, Direction: "above"},
				KPIs:      []string{"active_nodes", "tokens_in", "tokens_out"},
				Sparkline: "load",
				Headline:  "{{active_nodes}} nodes, {{tokens_in}} tokens in",
			},
		},
	}
}

// day is 2026-09-08 (a Tuesday) in UTC; the instances below sit in it,
// the day before, and the day after.
var day = PeriodAt(PeriodDay, time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC), time.UTC)

func timeFoldInstances() []*Instance {
	return []*Instance{
		{ // the day before: excluded from the day, included in the week
			Definition: "activity", Scope: "acme/music/a", Producer: "a-1",
			ProducedAt: day.Start.Add(-2 * time.Hour), Headline: "yesterday at a",
			KPIs: map[string]float64{"active_nodes": 9, "tokens_in": 1000, "tokens_out": 100},
		},
		{
			Definition: "activity", Scope: "acme/music/a", Producer: "a-1",
			ProducedAt: day.Start.Add(1 * time.Hour), Headline: "morning at a",
			KPIs:   map[string]float64{"active_nodes": 2, "tokens_in": 10, "tokens_out": 1},
			Series: map[string][]SeriesPoint{"load": {{T: day.Start.Add(1 * time.Hour), V: 1}}},
		},
		{ // no headline of its own; carries no tokens_out
			Definition: "activity", Scope: "acme/music/a", Producer: "a-2",
			ProducedAt: day.Start.Add(5 * time.Hour),
			KPIs:       map[string]float64{"active_nodes": 7, "tokens_in": 20},
			Series:     map[string][]SeriesPoint{"load": {{T: day.Start.Add(5 * time.Hour), V: 5}}},
		},
		{
			Definition: "activity", Scope: "acme/music/a", Producer: "a-1",
			ProducedAt: day.Start.Add(9 * time.Hour), Headline: "evening at a",
			KPIs:   map[string]float64{"active_nodes": 3, "tokens_in": 30, "tokens_out": 3},
			Series: map[string][]SeriesPoint{"load": {{T: day.Start.Add(9 * time.Hour), V: 9}}},
		},
		{ // a sibling site, one instance in the day
			Definition: "activity", Scope: "acme/music/b", Producer: "b-1",
			ProducedAt: day.Start.Add(3 * time.Hour), Headline: "b today",
			KPIs: map[string]float64{"active_nodes": 4, "tokens_in": 100, "tokens_out": 10},
		},
		{ // the day after: excluded from the day
			Definition: "activity", Scope: "acme/music/b", Producer: "b-1",
			ProducedAt: day.End, Headline: "b tomorrow",
			KPIs: map[string]float64{"active_nodes": 1, "tokens_in": 5000, "tokens_out": 500},
		},
	}
}

func TestContributorsInFoldsOnePathOverADay(t *testing.T) {
	def := activityDef()
	contrib := ContributorsIn(def, "acme/music/a", timeFoldInstances(), &day)
	if len(contrib) != 1 {
		t.Fatalf("contributors = %d, want the three instances in the day folded to one", len(contrib))
	}
	got := contrib[0]
	if got.Scope != "acme/music/a" || got.Producer != "a-1" || !got.ProducedAt.Equal(day.Start.Add(9*time.Hour)) {
		t.Errorf("synthetic identity = %s/%s@%v, want the newest instance's", got.Scope, got.Producer, got.ProducedAt)
	}
	// Counters sum over the day; yesterday's 1000 is not in it.
	if got.KPIs["tokens_in"] != 60 {
		t.Errorf("tokens_in = %v, want 10+20+30", got.KPIs["tokens_in"])
	}
	// An instance carrying no value contributes nothing to a sum.
	if got.KPIs["tokens_out"] != 4 {
		t.Errorf("tokens_out = %v, want 1+3 (the middle instance carried none)", got.KPIs["tokens_out"])
	}
	// A gauge keeps the default, latest.
	if got.KPIs["active_nodes"] != 3 {
		t.Errorf("active_nodes = %v, want 3 (latest)", got.KPIs["active_nodes"])
	}
	// Headline: the latest prose — the newest instance with any.
	if got.Headline != "evening at a" {
		t.Errorf("headline = %q, want the latest", got.Headline)
	}
	// Status: worst over the day — the 7-node instance tripped warn even
	// though the newest reads ok.
	if got.Status != StatusWarn {
		t.Errorf("status = %q, want warn (worst over the day)", got.Status)
	}
	// Sparkline points merged in time order.
	if pts := got.Series["load"]; len(pts) != 3 || pts[0].V != 1 || pts[2].V != 9 {
		t.Errorf("series = %+v, want the three points merged in time order", pts)
	}
}

func TestAggregateInRollsUpChildScopesAfterTheFold(t *testing.T) {
	def := activityDef()
	view := AggregateIn(def, "acme/music", timeFoldInstances(), &day)

	if view.Instances != 2 || strings.Join(view.Scopes, ",") != "acme/music/a,acme/music/b" {
		t.Fatalf("contributing set = %d %v, want one folded contributor per site", view.Instances, view.Scopes)
	}
	if view.Folded != 4 {
		t.Errorf("Folded = %d, want 4 (three at a, one at b)", view.Folded)
	}
	// Scope stage: sum across the two sites of each site's day.
	if got := kpiByName(t, view, "tokens_in"); got != 160 {
		t.Errorf("tokens_in = %v, want 60 (a's day) + 100 (b's day)", got)
	}
	if got := kpiByName(t, view, "active_nodes"); got != 7 {
		t.Errorf("active_nodes = %v, want 3 (a latest) + 4 (b)", got)
	}
	// Worst across sites of worst across the day.
	if view.Status != StatusWarn {
		t.Errorf("status = %q, want warn", view.Status)
	}
	// Headline policy latest across scopes: the newest contributor is a's
	// fold, carrying the latest prose in a's day.
	if view.Headline != "evening at a" {
		t.Errorf("headline = %q", view.Headline)
	}
	if view.Contributors[1].Producer != "a-1" || view.Contributors[0].Scope != "acme/music/b" {
		t.Errorf("contributors = %+v, want b then a's fold", view.Contributors)
	}
	if len(view.Sparkline) != 3 {
		t.Errorf("sparkline = %+v, want a's three points", view.Sparkline)
	}

	// A week holds every instance, including yesterday's and tomorrow's
	// (the day after is still inside the ISO week that Tuesday sits in).
	week := PeriodAt(PeriodWeek, day.Start, time.UTC)
	wv := AggregateIn(def, "acme/music", timeFoldInstances(), &week)
	if wv.Folded != 6 || kpiByName(t, wv, "tokens_in") != 6160 {
		t.Errorf("week: folded %d, tokens_in %v; want 6 and 1060+5100", wv.Folded, kpiByName(t, wv, "tokens_in"))
	}
	// Status: yesterday's nine nodes were warn (>= 5, < 10); the week's a
	// fold is worst, so warn.
	if wv.Status != StatusWarn {
		t.Errorf("week status = %q", wv.Status)
	}
}

// Without prose the headline template renders against the FOLDED numbers,
// so the sentence and the KPI row on the same tile agree.
func TestAggregateInTemplateRendersFoldedKPIs(t *testing.T) {
	def := activityDef()
	var instances []*Instance
	for _, in := range timeFoldInstances() {
		in.Headline = ""
		instances = append(instances, in)
	}
	view := AggregateIn(def, "acme/music/a", instances, &day)
	if view.Headline != "3 nodes, 60 tokens in" {
		t.Errorf("headline = %q, want the template over the folded kpis", view.Headline)
	}
	if view.HeadlinePartial {
		t.Errorf("one folded contributor is the whole picture, not partial")
	}
}

// The time policies are honoured: status latest, headline top(2), a KPI
// max; and a target folds with its metric's policy.
func TestContributorsInDeclaredPolicies(t *testing.T) {
	def := activityDef()
	def.Scope.Aggregation.Time = &TimeAggregationSpec{
		Status: "latest", Headline: "top(2)",
		KPIs: map[string]string{"tokens_in": "sum", "active_nodes": "max"},
	}
	instances := timeFoldInstances()
	instances[1].Targets = map[string]float64{"tokens_in": 15}
	instances[3].Targets = map[string]float64{"tokens_in": 25}
	contrib := ContributorsIn(def, "acme/music/a", instances, &day)
	got := contrib[0]
	if got.Status != StatusOK {
		t.Errorf("status = %q, want ok (latest instance reads 3 nodes)", got.Status)
	}
	// top(2) most-recent-first; the middle instance has no prose of its
	// own and contributes its rendered template, as it does in the scope
	// stage.
	if got.Headline != "evening at a; 7 nodes, 20 tokens in" {
		t.Errorf("headline = %q, want top(2) most-recent-first", got.Headline)
	}
	if got.KPIs["active_nodes"] != 7 {
		t.Errorf("active_nodes = %v, want max 7", got.KPIs["active_nodes"])
	}
	if got.Targets["tokens_in"] != 40 {
		t.Errorf("tokens_in target = %v, want 15+25 (a target folds like its metric)", got.Targets["tokens_in"])
	}
}

// A single instance in the period is contributed as itself, untouched.
func TestContributorsInSingleInstanceIsItself(t *testing.T) {
	def := activityDef()
	instances := timeFoldInstances()
	contrib := ContributorsIn(def, "acme/music/b", instances, &day)
	if len(contrib) != 1 || contrib[0] != instances[4] {
		t.Errorf("a lone instance should be contributed as the same pointer")
	}
}

// Without a period ContributorsIn IS Contributors, and AggregateIn IS
// Aggregate: byte-identical JSON on the existing fixtures.
func TestNoPeriodIsToday(t *testing.T) {
	def := testDef()
	instances := aggregateInstances()

	a := Contributors(def, "acme/music", instances)
	b := ContributorsIn(def, "acme/music", instances, nil)
	if len(a) != len(b) {
		t.Fatalf("contributor counts differ: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Errorf("contributor %d differs", i)
		}
	}

	want, err := json.Marshal(Aggregate(def, "acme/music", instances))
	if err != nil {
		t.Fatal(err)
	}
	got, err := json.Marshal(AggregateIn(def, "acme/music", instances, nil))
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != string(got) {
		t.Errorf("AggregateIn(nil) differs from Aggregate:\n%s\n%s", want, got)
	}
	if strings.Contains(string(got), `"folded"`) {
		t.Errorf("no period must not report a fold: %s", got)
	}

	// And on the activity fixtures, where a period WOULD change things.
	def = activityDef()
	instances = timeFoldInstances()
	want, _ = json.Marshal(Aggregate(def, "acme/music", instances))
	got, _ = json.Marshal(AggregateIn(def, "acme/music", instances, nil))
	if string(want) != string(got) {
		t.Errorf("AggregateIn(nil) differs from Aggregate on activity:\n%s\n%s", want, got)
	}
	if v := AggregateIn(def, "acme/music", instances, nil); kpiByName(t, v, "tokens_in") != 5030 {
		t.Errorf("no period: tokens_in = %v, want newest per path 30 + 5000", kpiByName(t, v, "tokens_in"))
	}
}

// A period with nothing in it is an empty view, not an error.
func TestAggregateInEmptyPeriod(t *testing.T) {
	def := activityDef()
	empty := PeriodAt(PeriodDay, day.Start.AddDate(0, 0, 30), time.UTC)
	view := AggregateIn(def, "acme/music", timeFoldInstances(), &empty)
	if view.Instances != 0 || view.Folded != 0 || len(view.KPIs) != 0 {
		t.Errorf("empty period view = %+v", view)
	}
}

func kpiByName(t *testing.T, view *SummaryView, name string) float64 {
	t.Helper()
	for _, k := range view.KPIs {
		if k.Name == name {
			return k.Value
		}
	}
	t.Fatalf("no kpi %q in %+v", name, view.KPIs)
	return 0
}
