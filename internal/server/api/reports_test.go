package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/acumen-ai-org/robotdreams/internal/server"
	"github.com/acumen-ai-org/robotdreams/internal/server/store"
	"github.com/acumen-ai-org/robotdreams/pkg/reporting"
)

const deployDefYAML = `
version: v1alpha1
kind: ReportDefinition
name: deploy
description: Deployment health
categories: [delivery]
modalities: [glance]
stances: [operational]
scope:
  attach: [site]
  aggregation:
    status: worst
    headline: top(2)
    kpis:
      deploys: sum
      fail_rate: max
    timeline: sample(300)
    pulse: sample(100)
data:
  kpis:
    - name: deploys
      unit: count
      window: 24h
    - name: fail_rate
      unit: percent
      window: 24h
  series:
    - name: rate
      unit: per-hour
  events:
    - type: deploy_started
      severity: info
    - type: deploy_finished
      severity: info
    - type: deploy_failed
      severity: critical
  tables:
    - name: recent
      columns: [service, result]
facets:
  summary:
    status:
      from: fail_rate
      warn_at: 5
      critical_at: 10
      direction: above
    kpis: [deploys, fail_rate]
    sparkline: rate
    headline: "{{deploys}} deploys"
  timeline:
    events: [deploy_started, deploy_finished, deploy_failed]
    spans:
      - name: deploy
        start: deploy_started
        end: deploy_finished
  detail:
    panels:
      - title: Deploy rate
        primitive: timeseries
        data: series/rate
      - title: Recent deploys
        primitive: datagrid
        data: table/recent
      - title: Events
        primitive: logbuffer
        data: events
      - title: Deploys
        primitive: kpi_card
        data: kpi/deploys
      - title: Windows
        primitive: gantt
        data: spans/deploy
    drilldown: [errors]
  pulse:
    events: [deploy_failed]
`

const errorsDefYAML = `
version: v1alpha1
kind: ReportDefinition
name: errors
description: Error stream
categories: [failures]
modalities: [glance]
scope:
  attach: [site]
  aggregation:
    kpis:
      error_count: sum
    time:
      kpis:
        error_count: sum
data:
  kpis:
    - name: error_count
      unit: count
      window: 1h
  events:
    - type: error
      severity: warn
facets:
  summary:
    kpis: [error_count]
  pulse:
    events: [error]
`

func newReportsTestEnv(t *testing.T) *testEnv {
	t.Helper()
	dir := t.TempDir()
	for name, body := range map[string]string{
		"deploy.yaml": deployDefYAML,
		"errors.yaml": errorsDefYAML,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return newTestEnv(t, func(cfg *server.Config) { cfg.ReportsDir = dir })
}

func (e *testEnv) postInstance(token string, inst reporting.Instance) {
	e.t.Helper()
	status, raw := e.do(http.MethodPost, "/api/reports/instances", token, inst)
	if status != http.StatusCreated {
		e.t.Fatalf("POST instance: status %d, body %s", status, raw)
	}
}

func waitForSSE(t *testing.T, ch <-chan sseEvent, typ string) sseEvent {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev := <-ch:
			if ev.Type == typ {
				return ev
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %q SSE event", typ)
		}
	}
}

func TestReportDefinitionsListing(t *testing.T) {
	e := newReportsTestEnv(t)
	w := e.connect("w1", "node", "")

	status, raw := e.do(http.MethodGet, "/api/reports/definitions", w.Token, nil)
	if status != http.StatusOK {
		t.Fatalf("GET definitions: status %d, body %s", status, raw)
	}
	var resp struct {
		Definitions []*reporting.Definition `json:"definitions"`
	}
	decodeInto(t, raw, &resp)
	if len(resp.Definitions) != 2 {
		t.Fatalf("definitions = %d, want 2", len(resp.Definitions))
	}
	if resp.Definitions[0].Name != "deploy" || resp.Definitions[1].Name != "errors" {
		t.Fatalf("definitions not sorted by name: %s, %s", resp.Definitions[0].Name, resp.Definitions[1].Name)
	}
	if resp.Definitions[0].Facets.Detail == nil || len(resp.Definitions[0].Facets.Detail.Panels) != 5 {
		t.Fatalf("full definition JSON not served: %+v", resp.Definitions[0].Facets)
	}
}

func TestReportsAuthRequired(t *testing.T) {
	e := newReportsTestEnv(t)

	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/reports/definitions"},
		{http.MethodGet, "/api/reports/summary?scope=acme"},
		{http.MethodPost, "/api/reports/instances"},
		{http.MethodPost, "/api/reports/events"},
	} {
		status, _ := e.do(tc.method, tc.path, "", nil)
		if status != http.StatusUnauthorized {
			t.Fatalf("%s %s without token: status %d, want 401", tc.method, tc.path, status)
		}
	}
}

func TestReportInstancePost(t *testing.T) {
	e := newReportsTestEnv(t)
	w := e.connect("w1", "node", "")

	ch, unsubscribe := e.api.events.subscribe()
	defer unsubscribe()

	status, raw := e.do(http.MethodPost, "/api/reports/instances", w.Token, reporting.Instance{
		Definition: "deploy",
		Scope:      "acme/music/platform/squad",
		KPIs:       map[string]float64{"deploys": 3, "fail_rate": 1},
	})
	if status != http.StatusCreated {
		t.Fatalf("POST instance: status %d, body %s", status, raw)
	}
	var resp struct {
		Definition string    `json:"definition"`
		Scope      string    `json:"scope"`
		ProducedAt time.Time `json:"produced_at"`
	}
	decodeInto(t, raw, &resp)
	if resp.Definition != "deploy" || resp.Scope != "acme/music/platform/squad" {
		t.Fatalf("unexpected accept echo: %+v", resp)
	}
	if resp.ProducedAt.IsZero() {
		t.Fatal("produced_at was not stamped server-side")
	}

	ev := waitForSSE(t, ch, "report_instance")
	data, ok := ev.Data.(map[string]any)
	if !ok {
		t.Fatalf("report_instance data = %T, want map", ev.Data)
	}
	if data["definition"] != "deploy" || data["scope"] != "acme/music/platform/squad" {
		t.Fatalf("unexpected report_instance payload: %+v", data)
	}

	stored, err := e.srv.Store().ListReportInstances(t.Context(), "deploy", "")
	if err != nil {
		t.Fatalf("ListReportInstances: %v", err)
	}
	if len(stored) != 1 || stored[0].Producer != "w1" {
		t.Fatalf("stored instance producer = %+v, want producer w1", stored)
	}

	status, _ = e.do(http.MethodPost, "/api/reports/instances", w.Token, reporting.Instance{
		Definition: "nope", Scope: "acme",
	})
	if status != http.StatusNotFound {
		t.Fatalf("unknown definition: status %d, want 404", status)
	}

	status, raw = e.do(http.MethodPost, "/api/reports/instances", w.Token, reporting.Instance{
		Definition: "deploy",
		Scope:      "acme",
		Status:     "meh",
		KPIs:       map[string]float64{"bogus": 1, "deploys": 2},
	})
	if status != http.StatusBadRequest {
		t.Fatalf("invalid instance: status %d, body %s", status, raw)
	}
	var errResp struct {
		Error string `json:"error"`
	}
	decodeInto(t, raw, &errResp)
	for _, want := range []string{"kpis[bogus]", `status: "meh"`} {
		if !strings.Contains(errResp.Error, want) {
			t.Fatalf("validation error body missing %q: %s", want, errResp.Error)
		}
	}
}

func TestReportInstanceProducerIsTheCaller(t *testing.T) {
	e := newReportsTestEnv(t)
	w1 := e.connect("w1", "node", "")
	e.connect("w2", "node", "")

	post := func(token, producer string, at time.Time) {
		t.Helper()
		status, raw := e.do(http.MethodPost, "/api/reports/instances", token, reporting.Instance{
			Definition: "deploy",
			Scope:      "acme",
			Producer:   producer,
			ProducedAt: at,
			KPIs:       map[string]float64{"deploys": 1, "fail_rate": 0},
		})
		if status != http.StatusCreated {
			t.Fatalf("POST instance as %q: status %d, body %s", producer, status, raw)
		}
	}
	base := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	post(w1.Token, "w2", base)
	post(e.adminToken("ops"), "w2", base.Add(time.Minute))
	post(e.adminToken("ops"), "", base.Add(2*time.Minute))

	stored, err := e.srv.Store().ListReportInstances(t.Context(), "deploy", "acme")
	if err != nil {
		t.Fatalf("ListReportInstances: %v", err)
	}
	got := map[time.Time]string{}
	for _, in := range stored {
		got[in.ProducedAt] = in.Producer
	}
	want := map[time.Time]string{
		base:                      "w1",
		base.Add(time.Minute):     "w2",
		base.Add(2 * time.Minute): "ops",
	}
	for at, producer := range want {
		if got[at] != producer {
			t.Fatalf("instance at %s: producer %q, want %q (all: %v)", at, got[at], producer, got)
		}
	}
}

func TestReportScopesListing(t *testing.T) {
	e := newReportsTestEnv(t)
	w := e.connect("w1", "node", "")
	base := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

	e.postInstance(w.Token, reporting.Instance{
		Definition: "deploy", Scope: "acme", ProducedAt: base,
		KPIs: map[string]float64{"deploys": 1},
	})
	e.postInstance(w.Token, reporting.Instance{
		Definition: "deploy", Scope: "acme/music", ProducedAt: base.Add(time.Minute),
		KPIs: map[string]float64{"deploys": 1},
	})
	e.postInstance(w.Token, reporting.Instance{
		Definition: "errors", Scope: "acme/music", ProducedAt: base,
		KPIs: map[string]float64{"error_count": 2},
	})

	status, raw := e.do(http.MethodGet, "/api/reports/scopes", w.Token, nil)
	if status != http.StatusOK {
		t.Fatalf("GET scopes: status %d, body %s", status, raw)
	}
	var resp struct {
		Scopes []reportScopeView `json:"scopes"`
	}
	decodeInto(t, raw, &resp)
	want := []reportScopeView{
		{Path: "acme", Depth: 1, Level: "universe", Instances: 1},
		{Path: "acme/music", Depth: 2, Level: "world", Instances: 2},
	}
	if len(resp.Scopes) != len(want) {
		t.Fatalf("scopes = %+v, want %+v", resp.Scopes, want)
	}
	for i := range want {
		if resp.Scopes[i] != want[i] {
			t.Fatalf("scopes[%d] = %+v, want %+v", i, resp.Scopes[i], want[i])
		}
	}
}

func TestReportSummaryAggregation(t *testing.T) {
	e := newReportsTestEnv(t)
	w := e.connect("w1", "node", "")
	base := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

	e.postInstance(w.Token, reporting.Instance{
		Definition: "deploy", Scope: "acme/a", ProducedAt: base,
		KPIs: map[string]float64{"deploys": 10, "fail_rate": 1},
	})
	e.postInstance(w.Token, reporting.Instance{
		Definition: "deploy", Scope: "acme/a", ProducedAt: base.Add(time.Minute),
		KPIs: map[string]float64{"deploys": 3, "fail_rate": 1},
	})

	e.postInstance(w.Token, reporting.Instance{
		Definition: "deploy", Scope: "acme/b", ProducedAt: base.Add(2 * time.Minute),
		KPIs: map[string]float64{"deploys": 5, "fail_rate": 12},
	})

	e.postInstance(w.Token, reporting.Instance{
		Definition: "errors", Scope: "acme/a", ProducedAt: base,
		KPIs: map[string]float64{"error_count": 7},
	})

	status, raw := e.do(http.MethodGet, "/api/reports/summary?scope=acme", w.Token, nil)
	if status != http.StatusOK {
		t.Fatalf("GET summary: status %d, body %s", status, raw)
	}
	var resp struct {
		Scope string                   `json:"scope"`
		Tiles []*reporting.SummaryView `json:"tiles"`
	}
	decodeInto(t, raw, &resp)
	if resp.Scope != "acme" || len(resp.Tiles) != 2 {
		t.Fatalf("summary = scope %q, %d tiles, want acme, 2", resp.Scope, len(resp.Tiles))
	}

	deploy, errTile := resp.Tiles[0], resp.Tiles[1]
	if deploy.Definition != "deploy" || errTile.Definition != "errors" {
		t.Fatalf("tiles not sorted by definition: %s, %s", deploy.Definition, errTile.Definition)
	}

	if got := kpiValue(t, deploy.KPIs, "deploys"); got != 8 {
		t.Fatalf("deploys sum = %v, want 8 (latest per scope wins)", got)
	}
	if got := kpiValue(t, deploy.KPIs, "fail_rate"); got != 12 {
		t.Fatalf("fail_rate max = %v, want 12", got)
	}
	if deploy.Status != "critical" {
		t.Fatalf("deploy status = %q, want critical (worst across scopes)", deploy.Status)
	}
	if deploy.Instances != 2 || len(deploy.Scopes) != 2 {
		t.Fatalf("contributing set = %d instances, scopes %v, want 2/2", deploy.Instances, deploy.Scopes)
	}
	if deploy.Headline != "5 deploys; 3 deploys" {
		t.Fatalf("headline = %q, want top(2) most-recent-first", deploy.Headline)
	}
	if got := kpiValue(t, errTile.KPIs, "error_count"); got != 7 {
		t.Fatalf("error_count = %v, want 7", got)
	}

	for category, wantDef := range map[string]string{"delivery": "deploy", "failures": "errors"} {
		status, raw := e.do(http.MethodGet, "/api/reports/summary?scope=acme&category="+category, w.Token, nil)
		if status != http.StatusOK {
			t.Fatalf("GET summary (%s): status %d, body %s", category, status, raw)
		}
		var filtered struct {
			Tiles []*reporting.SummaryView `json:"tiles"`
		}
		decodeInto(t, raw, &filtered)
		if len(filtered.Tiles) != 1 || filtered.Tiles[0].Definition != wantDef {
			t.Fatalf("category=%s tiles = %+v, want just %s", category, filtered.Tiles, wantDef)
		}
	}

	for stance, want := range map[string][]string{
		"operational": {"deploy", "errors"},
		"strategic":   {"errors"},
		"diagnostic":  {"errors"},
	} {
		status, raw := e.do(http.MethodGet, "/api/reports/summary?scope=acme&stance="+stance, w.Token, nil)
		if status != http.StatusOK {
			t.Fatalf("GET summary (stance=%s): status %d, body %s", stance, status, raw)
		}
		var filtered struct {
			Tiles []*reporting.SummaryView `json:"tiles"`
		}
		decodeInto(t, raw, &filtered)
		var got []string
		for _, tile := range filtered.Tiles {
			got = append(got, tile.Definition)
		}
		sort.Strings(got)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("stance=%s tiles = %v, want %v", stance, got, want)
		}
	}

	status, raw = e.do(http.MethodGet, "/api/reports/summary?scope=elsewhere", w.Token, nil)
	if status != http.StatusOK {
		t.Fatalf("GET summary (empty scope): status %d", status)
	}
	var empty struct {
		Tiles []*reporting.SummaryView `json:"tiles"`
	}
	decodeInto(t, raw, &empty)
	if len(empty.Tiles) != 0 {
		t.Fatalf("tiles at empty scope = %+v, want none", empty.Tiles)
	}
}

func kpiValue(t *testing.T, kpis []reporting.KPIValue, name string) float64 {
	t.Helper()
	for _, k := range kpis {
		if k.Name == name {
			return k.Value
		}
	}
	t.Fatalf("kpi %q not present in %+v", name, kpis)
	return 0
}

type reportResponse struct {
	Definition *reporting.Definition  `json:"definition"`
	Summary    *reporting.SummaryView `json:"summary"`
	Timeline   struct {
		Events []reporting.Event `json:"events"`
		Spans  []reportSpanView  `json:"spans"`
	} `json:"timeline"`
	Panels     []reportPanelView `json:"panels"`
	Drilldowns []string          `json:"drilldowns"`
}

func TestReportEndpointPanels(t *testing.T) {
	e := newReportsTestEnv(t)
	w := e.connect("w1", "node", "")
	base := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

	e.postInstance(w.Token, reporting.Instance{
		Definition: "deploy", Scope: "acme/a", ProducedAt: base,
		KPIs: map[string]float64{"deploys": 3, "fail_rate": 1},
		Series: map[string][]reporting.SeriesPoint{
			"rate": {{T: base.Add(-2 * time.Minute), V: 1}, {T: base.Add(-time.Minute), V: 2}},
		},
		Tables: map[string]reporting.Table{
			"recent": {Columns: []string{"service", "result"}, Rows: [][]string{{"api", "ok"}}},
		},
		Events: []reporting.Event{
			{T: base.Add(-2 * time.Minute), Type: "deploy_started", Severity: "info", Label: "v1"},
			{T: base.Add(-time.Minute), Type: "deploy_finished", Severity: "info", Label: "v1 done"},
		},
	})
	e.postInstance(w.Token, reporting.Instance{
		Definition: "deploy", Scope: "acme/b", ProducedAt: base.Add(time.Minute),
		KPIs: map[string]float64{"deploys": 5, "fail_rate": 2},
		Series: map[string][]reporting.SeriesPoint{
			"rate": {{T: base, V: 3}},
		},
		Tables: map[string]reporting.Table{
			"recent": {Columns: []string{"service", "result"}, Rows: [][]string{{"web", "fail"}}},
		},
		Events: []reporting.Event{
			{T: base, Type: "deploy_started", Severity: "info", Label: "v2"},
		},
	})

	status, raw := e.do(http.MethodGet, "/api/reports/report?definition=deploy&scope=acme", w.Token, nil)
	if status != http.StatusOK {
		t.Fatalf("GET report: status %d, body %s", status, raw)
	}
	var resp reportResponse
	decodeInto(t, raw, &resp)

	if resp.Definition == nil || resp.Definition.Name != "deploy" {
		t.Fatalf("report definition = %+v, want deploy", resp.Definition)
	}
	if resp.Summary == nil || resp.Summary.Instances != 2 {
		t.Fatalf("report summary = %+v, want 2 contributing instances", resp.Summary)
	}
	if len(resp.Drilldowns) != 1 || resp.Drilldowns[0] != "errors" {
		t.Fatalf("drilldowns = %v, want [errors]", resp.Drilldowns)
	}

	if len(resp.Timeline.Events) != 3 {
		t.Fatalf("timeline events = %d, want 3", len(resp.Timeline.Events))
	}
	for i := 1; i < len(resp.Timeline.Events); i++ {
		if resp.Timeline.Events[i].T.Before(resp.Timeline.Events[i-1].T) {
			t.Fatalf("timeline events out of order at %d", i)
		}
	}
	if len(resp.Timeline.Spans) != 2 {
		t.Fatalf("timeline spans = %+v, want 2", resp.Timeline.Spans)
	}
	paired, inflight := resp.Timeline.Spans[0], resp.Timeline.Spans[1]
	if paired.Scope != "acme/a" || paired.End == nil || !paired.End.Equal(base.Add(-time.Minute)) || paired.Label != "v1" {
		t.Fatalf("paired span = %+v, want acme/a closed at start+1m labeled v1", paired)
	}
	if inflight.Scope != "acme/b" || inflight.End != nil || inflight.Label != "v2" {
		t.Fatalf("in-flight span = %+v, want acme/b with null end", inflight)
	}

	if len(resp.Panels) != 5 {
		t.Fatalf("panels = %d, want 5", len(resp.Panels))
	}

	series := resp.Panels[0]
	if series.DataKind != "series" || series.Primitive != "timeseries" {
		t.Fatalf("panel[0] = %+v, want series/timeseries", series)
	}
	if len(series.Series) != 3 || series.Series[0].V != 1 || series.Series[2].V != 3 {
		t.Fatalf("series panel points = %+v, want merged time-ascending [1 2 3]", series.Series)
	}

	table := resp.Panels[1]
	if table.DataKind != "table" || table.Table == nil {
		t.Fatalf("panel[1] = %+v, want table", table)
	}
	wantCols := []string{"service", "result", "scope"}
	if len(table.Table.Columns) != 3 {
		t.Fatalf("table columns = %v, want %v", table.Table.Columns, wantCols)
	}
	for i, c := range wantCols {
		if table.Table.Columns[i] != c {
			t.Fatalf("table columns = %v, want %v", table.Table.Columns, wantCols)
		}
	}
	if len(table.Table.Rows) != 2 ||
		strings.Join(table.Table.Rows[0], ",") != "api,ok,acme/a" ||
		strings.Join(table.Table.Rows[1], ",") != "web,fail,acme/b" {
		t.Fatalf("table rows = %+v, want concatenated with scope column", table.Table.Rows)
	}

	events := resp.Panels[2]
	if events.DataKind != "events" || len(events.Events) != 3 {
		t.Fatalf("panel[2] = kind %q with %d events, want events/3", events.DataKind, len(events.Events))
	}
	if events.Events[0].Scope != "acme/a" || events.Events[0].Definition != "deploy" {
		t.Fatalf("events not tagged with origin: %+v", events.Events[0])
	}

	kpi := resp.Panels[3]
	if kpi.DataKind != "kpi" || kpi.KPI == nil || kpi.KPI.Name != "deploys" || kpi.KPI.Value != 8 || kpi.KPI.Policy != "sum" {
		t.Fatalf("panel[3] = %+v, want kpi deploys=8 via sum", kpi.KPI)
	}

	spansPanel := resp.Panels[4]
	if spansPanel.DataKind != "spans" || len(spansPanel.Spans) != 2 {
		t.Fatalf("panel[4] = %+v, want the two deploy spans", spansPanel)
	}

	if status, _ := e.do(http.MethodGet, "/api/reports/report?definition=nope", w.Token, nil); status != http.StatusNotFound {
		t.Fatalf("unknown definition: status %d, want 404", status)
	}
	if status, _ := e.do(http.MethodGet, "/api/reports/report", w.Token, nil); status != http.StatusBadRequest {
		t.Fatalf("missing definition: status %d, want 400", status)
	}
}

func TestReportTimeline(t *testing.T) {
	e := newReportsTestEnv(t)
	w := e.connect("w1", "node", "")
	base := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

	ch, unsubscribe := e.api.events.subscribe()
	defer unsubscribe()

	status, raw := e.do(http.MethodPost, "/api/reports/events", w.Token, reportEventsRequest{
		Definition: "deploy",
		Scope:      "acme/a",
		Events:     []reporting.Event{{T: base, Type: "deploy_failed", Label: "boom"}},
	})
	if status != http.StatusCreated {
		t.Fatalf("POST events: status %d, body %s", status, raw)
	}
	status, raw = e.do(http.MethodPost, "/api/reports/events", w.Token, reportEventsRequest{
		Definition: "errors",
		Scope:      "acme/b",
		Events:     []reporting.Event{{T: base.Add(time.Minute), Type: "error", Severity: "warn", Label: "oops"}},
	})
	if status != http.StatusCreated {
		t.Fatalf("POST events: status %d, body %s", status, raw)
	}

	ev := waitForSSE(t, ch, "report_event")
	data, ok := ev.Data.(map[string]any)
	if !ok {
		t.Fatalf("report_event data = %T, want map", ev.Data)
	}
	if data["definition"] != "deploy" || data["type"] != "deploy_failed" || data["severity"] != "critical" || data["label"] != "boom" {
		t.Fatalf("unexpected report_event payload: %+v", data)
	}

	status, raw = e.do(http.MethodGet, "/api/reports/timeline?scope=acme", w.Token, nil)
	if status != http.StatusOK {
		t.Fatalf("GET timeline: status %d, body %s", status, raw)
	}
	var resp struct {
		Events []reporting.Event `json:"events"`
	}
	decodeInto(t, raw, &resp)
	if len(resp.Events) != 2 || resp.Events[0].Type != "deploy_failed" || resp.Events[1].Type != "error" {
		t.Fatalf("timeline events = %+v, want [deploy_failed error]", resp.Events)
	}
	if resp.Events[0].Severity != "critical" {
		t.Fatalf("deploy_failed severity = %q, want the contract's declared critical", resp.Events[0].Severity)
	}

	status, raw = e.do(http.MethodGet, "/api/reports/timeline?scope=acme&since="+base.Format(time.RFC3339), w.Token, nil)
	if status != http.StatusOK {
		t.Fatalf("GET timeline since: status %d, body %s", status, raw)
	}
	resp.Events = nil
	decodeInto(t, raw, &resp)
	if len(resp.Events) != 1 || resp.Events[0].Type != "error" {
		t.Fatalf("since-filtered events = %+v, want just the later error", resp.Events)
	}

	status, raw = e.do(http.MethodGet, "/api/reports/timeline?scope=acme&category=failures", w.Token, nil)
	if status != http.StatusOK {
		t.Fatalf("GET timeline category: status %d, body %s", status, raw)
	}
	resp.Events = nil
	decodeInto(t, raw, &resp)
	if len(resp.Events) != 1 || resp.Events[0].Definition != "errors" {
		t.Fatalf("category-filtered events = %+v, want just errors'", resp.Events)
	}

	if status, _ := e.do(http.MethodGet, "/api/reports/timeline?since=yesterday", w.Token, nil); status != http.StatusBadRequest {
		t.Fatalf("bad since: status %d, want 400", status)
	}
}

func TestReportEventsPostValidationAndRetention(t *testing.T) {
	e := newReportsTestEnv(t)
	w := e.connect("w1", "node", "")
	base := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)

	status, _ := e.do(http.MethodPost, "/api/reports/events", w.Token, reportEventsRequest{
		Definition: "nope", Scope: "acme", Events: []reporting.Event{{Type: "x"}},
	})
	if status != http.StatusNotFound {
		t.Fatalf("unknown definition: status %d, want 404", status)
	}

	status, raw := e.do(http.MethodPost, "/api/reports/events", w.Token, reportEventsRequest{
		Definition: "deploy",
		Scope:      "acme",
		Events: []reporting.Event{
			{Type: "not_declared"},
			{Type: "deploy_started", Severity: "loud"},
		},
	})
	if status != http.StatusBadRequest {
		t.Fatalf("invalid events: status %d, body %s", status, raw)
	}
	var errResp struct {
		Error string `json:"error"`
	}
	decodeInto(t, raw, &errResp)
	for _, want := range []string{`events[0]: no such event type "not_declared"`, `events[1]: severity "loud"`} {
		if !strings.Contains(errResp.Error, want) {
			t.Fatalf("error body missing %q: %s", want, errResp.Error)
		}
	}

	total := store.ReportEventRetention + 50
	batch := make([]reporting.Event, 0, total)
	for i := 0; i < total; i++ {
		batch = append(batch, reporting.Event{
			T:     base.Add(time.Duration(i) * time.Second),
			Type:  "deploy_started",
			Label: fmt.Sprintf("e%d", i),
		})
	}
	status, raw = e.do(http.MethodPost, "/api/reports/events", w.Token, reportEventsRequest{
		Definition: "deploy", Scope: "acme/a", Events: batch,
	})
	if status != http.StatusCreated {
		t.Fatalf("POST bulk events: status %d, body %s", status, raw)
	}
	var accepted struct {
		Accepted int `json:"accepted"`
	}
	decodeInto(t, raw, &accepted)
	if accepted.Accepted != total {
		t.Fatalf("accepted = %d, want %d", accepted.Accepted, total)
	}

	stored, err := e.srv.Store().ListReportEvents(t.Context(), store.ReportEventFilter{
		Definitions: []string{"deploy"}, ScopePrefix: "acme/a",
	})
	if err != nil {
		t.Fatalf("ListReportEvents: %v", err)
	}
	if len(stored) != store.ReportEventRetention {
		t.Fatalf("retained events = %d, want %d", len(stored), store.ReportEventRetention)
	}

	if stored[0].Label != fmt.Sprintf("e%d", total-store.ReportEventRetention) {
		t.Fatalf("oldest retained = %q, want e%d", stored[0].Label, total-store.ReportEventRetention)
	}
}

func TestReportsDirLoadError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte("version: nope\nkind: ReportDefinition\nname: bad\n"), 0o600); err != nil {
		t.Fatalf("write bad.yaml: %v", err)
	}

	_, err := server.New(server.Config{DataDir: t.TempDir(), ReportsDir: dir})
	if err == nil {
		t.Fatal("server.New with an invalid report library succeeded, want error")
	}
	if !strings.Contains(err.Error(), "bad.yaml") {
		t.Fatalf("load error does not name the offending file: %v", err)
	}
}

func TestReportsEmptyRegistry(t *testing.T) {
	e := newTestEnv(t, nil)
	w := e.connect("w1", "node", "")

	status, raw := e.do(http.MethodGet, "/api/reports/definitions", w.Token, nil)
	if status != http.StatusOK {
		t.Fatalf("GET definitions: status %d, body %s", status, raw)
	}
	var resp struct {
		Definitions []*reporting.Definition `json:"definitions"`
	}
	decodeInto(t, raw, &resp)
	if len(resp.Definitions) != 0 {
		t.Fatalf("definitions = %+v, want none", resp.Definitions)
	}

	status, _ = e.do(http.MethodPost, "/api/reports/instances", w.Token, reporting.Instance{
		Definition: "deploy", Scope: "acme",
	})
	if status != http.StatusNotFound {
		t.Fatalf("POST instance against empty registry: status %d, want 404", status)
	}
}

func TestReportPeriod(t *testing.T) {
	e := newReportsTestEnv(t)
	w := e.connect("w1", "node", "")

	day := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)

	e.postInstance(w.Token, reporting.Instance{
		Definition: "deploy", Scope: "acme/a", ProducedAt: day.Add(-time.Hour),
		KPIs: map[string]float64{"deploys": 100, "fail_rate": 1},
	})
	for i, n := range []float64{1, 2, 3} {
		e.postInstance(w.Token, reporting.Instance{
			Definition: "deploy", Scope: "acme/a", ProducedAt: day.Add(time.Duration(i+1) * time.Hour),
			KPIs: map[string]float64{"deploys": n, "fail_rate": 1},
		})
		e.postInstance(w.Token, reporting.Instance{
			Definition: "errors", Scope: "acme/a", ProducedAt: day.Add(time.Duration(i+1) * time.Hour),
			Headline: fmt.Sprintf("errors #%d", i+1),
			KPIs:     map[string]float64{"error_count": 10 * n},
		})
	}
	e.postInstance(w.Token, reporting.Instance{
		Definition: "deploy", Scope: "acme/b", ProducedAt: day.Add(5 * time.Hour),
		KPIs: map[string]float64{"deploys": 7, "fail_rate": 1},
	})

	for _, ev := range []reporting.Event{
		{T: day.Add(-time.Hour), Type: "deploy_failed", Label: "yesterday"},
		{T: day.Add(2 * time.Hour), Type: "deploy_failed", Label: "today"},
	} {
		status, raw := e.do(http.MethodPost, "/api/reports/events", w.Token, reportEventsRequest{
			Definition: "deploy", Scope: "acme/a", Events: []reporting.Event{ev},
		})
		if status != http.StatusCreated {
			t.Fatalf("POST events: status %d, body %s", status, raw)
		}
	}

	at := day.Add(12 * time.Hour).Format(time.RFC3339)

	_, beforeSummary := e.do(http.MethodGet, "/api/reports/summary?scope=acme", w.Token, nil)
	_, beforeReport := e.do(http.MethodGet, "/api/reports/report?definition=errors&scope=acme", w.Token, nil)
	_, beforeTimeline := e.do(http.MethodGet, "/api/reports/timeline?scope=acme", w.Token, nil)

	status, raw := e.do(http.MethodGet, "/api/reports/summary?scope=acme&period=day&at="+at, w.Token, nil)
	if status != http.StatusOK {
		t.Fatalf("GET summary period=day: status %d, body %s", status, raw)
	}
	var summary struct {
		Scope  string                   `json:"scope"`
		Period *reporting.Period        `json:"period"`
		Tiles  []*reporting.SummaryView `json:"tiles"`
	}
	decodeInto(t, raw, &summary)
	if summary.Period == nil || summary.Period.Kind != reporting.PeriodDay || !summary.Period.Start.Equal(day) || !summary.Period.End.Equal(day.AddDate(0, 0, 1)) {
		t.Fatalf("period echo = %+v, want day %v..%v", summary.Period, day, day.AddDate(0, 0, 1))
	}
	if len(summary.Tiles) != 2 {
		t.Fatalf("tiles = %d, want deploy and errors", len(summary.Tiles))
	}
	deploy, errs := summary.Tiles[0], summary.Tiles[1]

	if got := kpiValue(t, deploy.KPIs, "deploys"); got != 10 {
		t.Fatalf("deploys over the day = %v, want 3 + 7", got)
	}
	if deploy.Folded != 4 || deploy.Instances != 2 {
		t.Fatalf("deploy folded %d into %d, want 4 into 2", deploy.Folded, deploy.Instances)
	}

	if got := kpiValue(t, errs.KPIs, "error_count"); got != 60 {
		t.Fatalf("error_count over the day = %v, want 10+20+30", got)
	}
	if errs.Headline != "errors #3" || errs.Folded != 3 || errs.Instances != 1 {
		t.Fatalf("errors tile = headline %q, folded %d, instances %d", errs.Headline, errs.Folded, errs.Instances)
	}

	status, raw = e.do(http.MethodGet, "/api/reports/summary?scope=acme&period=week&at="+at, w.Token, nil)
	if status != http.StatusOK {
		t.Fatalf("GET summary period=week: status %d, body %s", status, raw)
	}
	summary.Tiles, summary.Period = nil, nil
	decodeInto(t, raw, &summary)
	wantStart, wantEnd := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC), time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	if summary.Period == nil || summary.Period.Kind != "week" || !summary.Period.Start.Equal(wantStart) || !summary.Period.End.Equal(wantEnd) {
		t.Fatalf("week echo = %+v, want %v..%v", summary.Period, wantStart, wantEnd)
	}
	if summary.Tiles[0].Folded != 5 {
		t.Fatalf("deploy folded over the week = %d, want 5", summary.Tiles[0].Folded)
	}

	status, raw = e.do(http.MethodGet, "/api/reports/summary?scope=acme&period=day&at=2026-09-08T00:30:00%2B02:00", w.Token, nil)
	if status != http.StatusOK {
		t.Fatalf("GET summary with offset: status %d, body %s", status, raw)
	}
	summary.Period = nil
	decodeInto(t, raw, &summary)
	if want := time.Date(2026, 9, 7, 22, 0, 0, 0, time.UTC); summary.Period == nil || !summary.Period.Start.Equal(want) {
		t.Fatalf("offset day start = %v, want %v (midnight at +02:00)", summary.Period.Start, want)
	}

	status, raw = e.do(http.MethodGet, "/api/reports/report?definition=deploy&scope=acme&period=day&at="+at, w.Token, nil)
	if status != http.StatusOK {
		t.Fatalf("GET report period=day: status %d, body %s", status, raw)
	}
	var report struct {
		Period   *reporting.Period     `json:"period"`
		Summary  reporting.SummaryView `json:"summary"`
		Timeline struct {
			Events []reporting.Event `json:"events"`
		} `json:"timeline"`
		Panels []reportPanelView `json:"panels"`
	}
	decodeInto(t, raw, &report)
	if report.Period == nil || report.Period.Kind != "day" {
		t.Fatalf("report period echo = %+v", report.Period)
	}
	if got := kpiValue(t, report.Summary.KPIs, "deploys"); got != 10 {
		t.Fatalf("report deploys over the day = %v, want 10", got)
	}
	if len(report.Timeline.Events) != 1 || report.Timeline.Events[0].Label != "today" {
		t.Fatalf("report timeline = %+v, want only today's event", report.Timeline.Events)
	}
	for _, p := range report.Panels {
		if p.DataKind == "kpi" && p.KPI != nil && p.KPI.Name == "deploys" && p.KPI.Value != 10 {
			t.Fatalf("kpi panel = %v, want the same folded 10 the summary shows", p.KPI.Value)
		}
	}

	status, raw = e.do(http.MethodGet, "/api/reports/timeline?scope=acme&period=day&at="+at, w.Token, nil)
	if status != http.StatusOK {
		t.Fatalf("GET timeline period=day: status %d, body %s", status, raw)
	}
	var timeline struct {
		Period *reporting.Period `json:"period"`
		Events []reporting.Event `json:"events"`
	}
	decodeInto(t, raw, &timeline)
	if timeline.Period == nil || len(timeline.Events) != 1 || timeline.Events[0].Label != "today" {
		t.Fatalf("timeline over the day = period %+v, events %+v", timeline.Period, timeline.Events)
	}

	status, raw = e.do(http.MethodGet, "/api/reports/timeline?scope=acme&period=day&at="+at+"&since="+day.Add(3*time.Hour).Format(time.RFC3339), w.Token, nil)
	if status != http.StatusOK {
		t.Fatalf("GET timeline period+since: status %d, body %s", status, raw)
	}
	timeline.Events = nil
	decodeInto(t, raw, &timeline)
	if len(timeline.Events) != 0 {
		t.Fatalf("since after today's event should exclude it: %+v", timeline.Events)
	}
	status, raw = e.do(http.MethodGet, "/api/reports/timeline?scope=acme&period=day&at="+at+"&since="+day.AddDate(0, 0, -7).Format(time.RFC3339), w.Token, nil)
	if status != http.StatusOK {
		t.Fatalf("GET timeline period+wide since: status %d, body %s", status, raw)
	}
	timeline.Events = nil
	decodeInto(t, raw, &timeline)
	if len(timeline.Events) != 1 {
		t.Fatalf("a since before the period must not widen it: %+v", timeline.Events)
	}

	for _, path := range []string{
		"/api/reports/summary?scope=acme&period=bogus",
		"/api/reports/report?definition=deploy&scope=acme&period=bogus",
		"/api/reports/timeline?scope=acme&period=bogus",
		"/api/reports/summary?scope=acme&period=week&at=tomorrow",
	} {
		if status, raw := e.do(http.MethodGet, path, w.Token, nil); status != http.StatusBadRequest {
			t.Fatalf("%s: status %d, want 400 (%s)", path, status, raw)
		}
	}

	for _, c := range []struct {
		path   string
		before []byte
	}{
		{"/api/reports/summary?scope=acme", beforeSummary},
		{"/api/reports/report?definition=errors&scope=acme", beforeReport},
		{"/api/reports/timeline?scope=acme", beforeTimeline},
	} {
		_, after := e.do(http.MethodGet, c.path, w.Token, nil)
		if string(after) != string(c.before) {
			t.Fatalf("%s changed without a period:\n%s\n%s", c.path, c.before, after)
		}
		if strings.Contains(string(after), `"period"`) || strings.Contains(string(after), `"folded"`) {
			t.Fatalf("%s carries a period or fold without one asked for: %s", c.path, after)
		}
	}

	var plain struct {
		Tiles []*reporting.SummaryView `json:"tiles"`
	}
	decodeInto(t, beforeSummary, &plain)
	if got := kpiValue(t, plain.Tiles[1].KPIs, "error_count"); got != 30 {
		t.Fatalf("no-period error_count = %v, want the newest instance's 30", got)
	}
}
