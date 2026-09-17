package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/acumen-ai-org/robotdreams/internal/server/store"
	"github.com/acumen-ai-org/robotdreams/pkg/reporting"
)

const (
	reportSeriesCap   = 200
	reportTimelineCap = store.ReportEventRetention
)

type reportScopeView struct {
	Path      string `json:"path"`
	Depth     int    `json:"depth"`
	Level     string `json:"level"`
	Instances int    `json:"instances"`
	Recent    *int   `json:"recent,omitempty"`
}

type reportSpanView struct {
	Name  string     `json:"name"`
	Start time.Time  `json:"start"`
	End   *time.Time `json:"end"`
	Label string     `json:"label"`
	Scope string     `json:"scope"`
}

type reportPanelView struct {
	Title     string                  `json:"title"`
	Primitive string                  `json:"primitive"`
	Section   string                  `json:"section,omitempty"`
	Stances   []string                `json:"stances,omitempty"`
	DataKind  string                  `json:"data_kind"`
	Series    []reporting.SeriesPoint `json:"series,omitempty"`
	Table     *reporting.Table        `json:"table,omitempty"`
	Events    []reporting.Event       `json:"events,omitempty"`
	Spans     []reportSpanView        `json:"spans,omitempty"`
	KPI       *reporting.KPIValue     `json:"kpi,omitempty"`
	Plan      *reporting.PlanView     `json:"plan,omitempty"`
}

type reportEventsRequest struct {
	Definition string            `json:"definition"`
	Scope      string            `json:"scope"`
	Events     []reporting.Event `json:"events"`
}

func normalizeScope(s string) string {
	return strings.Join(reporting.SplitScope(s), "/")
}

func parsePeriod(q url.Values) (*reporting.Period, error) {
	raw := strings.TrimSpace(q.Get("period"))
	if raw == "" {
		return nil, nil
	}
	kind, err := reporting.ParsePeriodKind(raw)
	if err != nil {
		return nil, fmt.Errorf("period: %w", err)
	}
	at := time.Now().UTC()
	if rawAt := strings.TrimSpace(q.Get("at")); rawAt != "" {
		t, err := time.Parse(time.RFC3339, rawAt)
		if err != nil {
			return nil, fmt.Errorf("at: %q is not an RFC3339 timestamp", rawAt)
		}
		at = t
	}
	p := reporting.PeriodAt(kind, at, at.Location())
	return &p, nil
}

func withPeriod(body map[string]any, period *reporting.Period) map[string]any {
	if period != nil {
		body["period"] = period
	}
	return body
}

func instanceWindow(period *reporting.Period) []store.ReportInstanceWindow {
	if period == nil {
		return nil
	}
	return []store.ReportInstanceWindow{{From: period.Start, Until: period.End}}
}

func (a *API) handleReportDefinitions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"definitions": a.srv.Reports().List()})
}

func (a *API) handleReportScopes(w http.ResponseWriter, r *http.Request) {
	var since time.Time
	if raw := strings.TrimSpace(r.URL.Query().Get("since")); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "since must be an RFC3339 timestamp")
			return
		}
		since = t
	}
	counts, err := a.srv.Store().ListReportScopes(r.Context(), since)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list report scopes")
		return
	}
	out := make([]reportScopeView, 0, len(counts))
	for _, c := range counts {
		depth := reporting.ScopeDepth(c.Path)
		v := reportScopeView{
			Path:      c.Path,
			Depth:     depth,
			Level:     reporting.LevelName(depth),
			Instances: c.Instances,
		}
		if !since.IsZero() {
			recent := c.Recent
			v.Recent = &recent
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"scopes": out})
}

func (a *API) handleReportSummary(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	scope := normalizeScope(q.Get("scope"))
	category := q.Get("category")
	stance := q.Get("stance")
	period, err := parsePeriod(q)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	tiles := make([]*reporting.SummaryView, 0)
	for _, def := range a.srv.Reports().List() {
		if category != "" && !containsString(def.Categories, category) {
			continue
		}

		if !def.ServesStance(stance) {
			continue
		}
		instances, err := a.srv.Store().ListReportInstances(r.Context(), def.Name, scope, instanceWindow(period)...)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not list report instances")
			return
		}
		view := reporting.AggregateIn(def, scope, instances, period)
		if view.Instances == 0 {
			continue
		}
		tiles = append(tiles, view)
	}
	writeJSON(w, http.StatusOK, withPeriod(map[string]any{"scope": scope, "tiles": tiles}, period))
}

func (a *API) handleReport(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	name := q.Get("definition")
	if name == "" {
		writeError(w, http.StatusBadRequest, "definition query parameter is required")
		return
	}
	def, ok := a.srv.Reports().Get(name)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("unknown report definition %q", name))
		return
	}
	scope := normalizeScope(q.Get("scope"))
	period, err := parsePeriod(q)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	instances, err := a.srv.Store().ListReportInstances(r.Context(), def.Name, scope, instanceWindow(period)...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list report instances")
		return
	}
	summary := reporting.AggregateIn(def, scope, instances, period)

	eventFilter := store.ReportEventFilter{
		Definitions: []string{def.Name},
		ScopePrefix: scope,
	}
	boundEventsToPeriod(&eventFilter, period)
	stored, err := a.srv.Store().ListReportEvents(r.Context(), eventFilter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list report events")
		return
	}
	merged := reporting.MergeEvents(reportEventCap(def), stored)
	if merged == nil {
		merged = []reporting.Event{}
	}

	var spanSpecs []reporting.SpanSpec
	if def.Facets.Timeline != nil {
		spanSpecs = def.Facets.Timeline.Spans
	}
	spans := deriveSpans(spanSpecs, merged)

	contrib := contributorsIn(def, scope, instances, period)
	plan := reporting.AggregatePlanIn(def, scope, instances, period)
	panels := resolvePanels(def, scope, contrib, merged, spans, plan)

	drilldowns := []string{}
	if def.Facets.Detail != nil {
		drilldowns = append(drilldowns, def.Facets.Detail.Drilldown...)
	}

	writeJSON(w, http.StatusOK, withPeriod(map[string]any{
		"definition": def,
		"summary":    summary,
		"timeline": map[string]any{
			"events": merged,
			"spans":  spans,
		},
		"plan":       plan,
		"panels":     panels,
		"drilldowns": drilldowns,
	}, period))
}

func boundEventsToPeriod(f *store.ReportEventFilter, period *reporting.Period) {
	if period == nil {
		return
	}
	if start := period.Start.Add(-time.Nanosecond); f.Since.IsZero() || f.Since.Before(start) {
		f.Since = start
	}
	if f.Until.IsZero() || period.End.Before(f.Until) {
		f.Until = period.End
	}
}

func (a *API) handleReportTimeline(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	scope := normalizeScope(q.Get("scope"))
	category := q.Get("category")
	stance := q.Get("stance")
	period, err := parsePeriod(q)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var since time.Time
	if raw := q.Get("since"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("since: %q is not an RFC3339 timestamp", raw))
			return
		}
		since = t
	}

	filter := store.ReportEventFilter{ScopePrefix: scope, Since: since}
	boundEventsToPeriod(&filter, period)
	if category != "" || stance != "" {
		for _, def := range a.srv.Reports().List() {
			if category != "" && !containsString(def.Categories, category) {
				continue
			}

			if !def.ServesStance(stance) {
				continue
			}
			filter.Definitions = append(filter.Definitions, def.Name)
		}
		if len(filter.Definitions) == 0 {
			writeJSON(w, http.StatusOK, withPeriod(map[string]any{"events": []reporting.Event{}}, period))
			return
		}
	}

	stored, err := a.srv.Store().ListReportEvents(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list report events")
		return
	}
	merged := reporting.MergeEvents(reportTimelineCap, stored)
	if merged == nil {
		merged = []reporting.Event{}
	}
	writeJSON(w, http.StatusOK, withPeriod(map[string]any{"events": merged}, period))
}

func (a *API) handleReportInstancePost(w http.ResponseWriter, r *http.Request) {
	claims, _ := ClaimsFromContext(r.Context())

	var inst reporting.Instance
	if !decodeJSON(w, r, &inst) {
		return
	}
	if inst.Definition == "" {
		writeError(w, http.StatusBadRequest, "definition is required")
		return
	}
	def, ok := a.srv.Reports().Get(inst.Definition)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("unknown report definition %q", inst.Definition))
		return
	}

	if inst.ProducedAt.IsZero() {
		inst.ProducedAt = a.clock.Now().UTC()
	}
	if inst.Producer == "" || !isAdmin(claims) {
		inst.Producer = claims.WorkerID
	}

	if err := reporting.ValidateInstance(def, &inst); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	inst.Scope = normalizeScope(inst.Scope)
	for i := range inst.Events {
		if inst.Events[i].T.IsZero() {
			inst.Events[i].T = inst.ProducedAt
		}
	}

	if err := a.srv.Store().InsertReportInstance(r.Context(), &inst); err != nil {
		writeError(w, http.StatusInternalServerError, "could not persist report instance")
		return
	}
	if err := a.srv.Store().InsertReportEvents(r.Context(), inst.Definition, inst.Scope, inst.Events); err != nil {
		writeError(w, http.StatusInternalServerError, "could not persist report instance events")
		return
	}

	a.events.broadcast(sseEvent{Type: "report_instance", Data: map[string]any{
		"definition":  inst.Definition,
		"scope":       inst.Scope,
		"produced_at": inst.ProducedAt,
	}})

	writeJSON(w, http.StatusCreated, map[string]any{
		"definition":  inst.Definition,
		"scope":       inst.Scope,
		"produced_at": inst.ProducedAt,
	})
}

func (a *API) handleReportEventsPost(w http.ResponseWriter, r *http.Request) {
	var req reportEventsRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Definition == "" {
		writeError(w, http.StatusBadRequest, "definition is required")
		return
	}
	def, ok := a.srv.Reports().Get(req.Definition)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("unknown report definition %q", req.Definition))
		return
	}
	scope := normalizeScope(req.Scope)
	if scope == "" {
		writeError(w, http.StatusBadRequest, "scope is required")
		return
	}

	declared := make(map[string]string, len(def.Data.Events))
	for _, e := range def.Data.Events {
		declared[e.Type] = e.Severity
	}

	now := a.clock.Now().UTC()
	var problems []error
	for i := range req.Events {
		e := &req.Events[i]
		sev, ok := declared[e.Type]
		if !ok {
			problems = append(problems, fmt.Errorf("events[%d]: no such event type %q in data contract", i, e.Type))
			continue
		}
		if e.Severity == "" {
			e.Severity = sev
		} else if !containsString(reporting.Severities, e.Severity) {
			problems = append(problems, fmt.Errorf("events[%d]: severity %q not one of %s", i, e.Severity, strings.Join(reporting.Severities, "|")))
		}
		if e.T.IsZero() {
			e.T = now
		}
	}
	if len(problems) > 0 {
		writeError(w, http.StatusBadRequest, errors.Join(problems...).Error())
		return
	}

	if err := a.srv.Store().InsertReportEvents(r.Context(), req.Definition, scope, req.Events); err != nil {
		writeError(w, http.StatusInternalServerError, "could not persist report events")
		return
	}

	for _, e := range req.Events {
		a.events.broadcast(sseEvent{Type: "report_event", Data: map[string]any{
			"definition": req.Definition,
			"scope":      scope,
			"t":          e.T,
			"type":       e.Type,
			"severity":   e.Severity,
			"label":      e.Label,
		}})
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"definition": req.Definition,
		"scope":      scope,
		"accepted":   len(req.Events),
	})
}

func reportEventCap(def *reporting.Definition) int {
	for _, raw := range []string{def.Scope.Aggregation.Timeline, def.Scope.Aggregation.Pulse} {
		if p, err := reporting.ParsePolicy(raw); err == nil && p.Name == "sample" {
			return p.N
		}
	}
	return reportTimelineCap
}

func contributorsIn(def *reporting.Definition, scope string, instances []*reporting.Instance, period *reporting.Period) []*reporting.Instance {
	return reporting.ContributorsIn(def, scope, instances, period)
}

func deriveSpans(specs []reporting.SpanSpec, events []reporting.Event) []reportSpanView {
	spans := []reportSpanView{}
	for _, spec := range specs {
		openByScope := map[string][]int{}
		for _, e := range events {
			switch e.Type {
			case spec.Start:
				spans = append(spans, reportSpanView{
					Name:  spec.Name,
					Start: e.T,
					Label: e.Label,
					Scope: e.Scope,
				})
				openByScope[e.Scope] = append(openByScope[e.Scope], len(spans)-1)
			case spec.End:
				queue := openByScope[e.Scope]
				if len(queue) == 0 {
					continue
				}
				idx := queue[0]
				openByScope[e.Scope] = queue[1:]
				end := e.T
				spans[idx].End = &end
			}
		}
	}
	sort.SliceStable(spans, func(i, j int) bool { return spans[i].Start.Before(spans[j].Start) })
	return spans
}

func resolvePanels(def *reporting.Definition, scope string, contrib []*reporting.Instance, merged []reporting.Event, spans []reportSpanView, plan *reporting.PlanView) []reportPanelView {
	panels := []reportPanelView{}
	if def.Facets.Detail == nil {
		return panels
	}

	var kpis []reporting.KPIValue
	kpisResolved := false

	for _, p := range def.Facets.Detail.Panels {
		view := reportPanelView{Title: p.Title, Primitive: p.Primitive, Section: p.Section, Stances: p.Stances()}
		kind, name, _ := strings.Cut(p.Data, "/")
		switch {
		case p.Data == "events":
			view.DataKind = "events"
			view.Events = merged
		case p.Data == "items":
			view.DataKind = "items"
			view.Plan = plan
		case kind == "series":
			view.DataKind = "series"
			view.Series = mergeSeries(name, contrib)
		case kind == "table":
			view.DataKind = "table"
			view.Table = concatTables(def, name, contrib)
		case kind == "spans":
			view.DataKind = "spans"
			named := []reportSpanView{}
			for _, s := range spans {
				if s.Name == name {
					named = append(named, s)
				}
			}
			view.Spans = named
		case kind == "kpi":
			view.DataKind = "kpi"
			if !kpisResolved {
				kpis = aggregateAllKPIs(def, scope, contrib)
				kpisResolved = true
			}
			for i := range kpis {
				if kpis[i].Name == name {
					kpi := kpis[i]
					view.KPI = &kpi
					break
				}
			}
		default:
			view.DataKind = kind
		}
		panels = append(panels, view)
	}
	return panels
}

func mergeSeries(name string, contrib []*reporting.Instance) []reporting.SeriesPoint {
	pts := []reporting.SeriesPoint{}
	for _, in := range contrib {
		pts = append(pts, in.Series[name]...)
	}
	sort.SliceStable(pts, func(i, j int) bool { return pts[i].T.Before(pts[j].T) })
	if len(pts) > reportSeriesCap {
		pts = pts[len(pts)-reportSeriesCap:]
	}
	return pts
}

func concatTables(def *reporting.Definition, name string, contrib []*reporting.Instance) *reporting.Table {
	var cols []string
	for _, ts := range def.Data.Tables {
		if ts.Name == name {
			cols = append([]string(nil), ts.Columns...)
			break
		}
	}
	rows := [][]string{}
	for _, in := range contrib {
		tbl, ok := in.Tables[name]
		if !ok {
			continue
		}
		if len(cols) == 0 {
			cols = append([]string(nil), tbl.Columns...)
		}
		for _, row := range tbl.Rows {
			rows = append(rows, append(append([]string(nil), row...), in.Scope))
		}
	}
	return &reporting.Table{Columns: append(cols, "scope"), Rows: rows}
}

func aggregateAllKPIs(def *reporting.Definition, scope string, contrib []*reporting.Instance) []reporting.KPIValue {
	d := *def
	if d.Facets.Summary != nil {
		s := *d.Facets.Summary
		s.KPIs = nil
		d.Facets.Summary = &s
	}
	return reporting.Aggregate(&d, scope, contrib).KPIs
}

func containsString(set []string, v string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}
