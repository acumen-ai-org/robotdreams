package reporting

import (
	"errors"
	"strings"
	"testing"
)

const validDefinition = `version: v1alpha1
kind: ReportDefinition
name: test-delivery
description: Deploys and releases over time.
categories: [delivery]
modalities: [glance, narrative]

scope:
  attach: [site, realm]
  aggregation:
    status: worst
    headline: synthesize
    kpis:
      deploys: sum
      failed_deploys: sum
      lead_time_minutes: p50
    timeline: merge
    pulse: sample(50)

data:
  kpis:
    - { name: deploys, unit: count, window: 24h }
    - { name: failed_deploys, unit: count, window: 24h }
    - { name: lead_time_minutes, unit: minutes, window: 24h }
  series:
    - { name: deploy_frequency, unit: per-hour }
  events:
    - { type: deploy.started, severity: info }
    - { type: deploy.finished, severity: info }
    - { type: deploy.rolled_back, severity: warn }
  tables:
    - name: recent_deploys
      columns: [when, service, version, status]

facets:
  summary:
    status: { from: failed_deploys, warn_at: 1, critical_at: 3, direction: above }
    kpis: [deploys, failed_deploys, lead_time_minutes]
    sparkline: deploy_frequency
    headline: "{{deploys}} deploys, {{failed_deploys}} failed"
  timeline:
    events: [deploy.started, deploy.finished, deploy.rolled_back]
    spans:
      - { name: deploy, start: deploy.started, end: deploy.finished }
    milestones: [deploy.finished]
  detail:
    panels:
      - { title: Deploy frequency, primitive: timeseries, data: series/deploy_frequency }
      - { title: Recent deploys, primitive: datagrid, data: table/recent_deploys }
      - { title: Deploy windows, primitive: gantt, data: spans/deploy }
      - { title: Live, primitive: ticker, data: events }
      - { title: Lead time, primitive: gauge, data: kpi/lead_time_minutes }
  pulse:
    events: [deploy.started, deploy.rolled_back]
    rate_limit: 30/min
  media:
    archetypes: [anchor, screencast]
  conversation:
    grounding: [kpis, events, tables]
    prompts:
      - "What shipped in the last 24 hours?"
`

func TestParseDefinitionValid(t *testing.T) {
	d, err := ParseDefinition([]byte(validDefinition))
	if err != nil {
		t.Fatalf("ParseDefinition: %v", err)
	}
	if d.Name != "test-delivery" {
		t.Errorf("Name = %q, want test-delivery", d.Name)
	}
	if d.Version != DefinitionVersion || d.Kind != DefinitionKind {
		t.Errorf("Version/Kind = %q/%q", d.Version, d.Kind)
	}
	if got := d.Scope.Aggregation.KPIs["lead_time_minutes"]; got != "p50" {
		t.Errorf("aggregation kpi policy = %q, want p50", got)
	}
	if len(d.Data.KPIs) != 3 || d.Data.KPIs[0].Name != "deploys" || d.Data.KPIs[0].Window != "24h" {
		t.Errorf("data kpis = %+v", d.Data.KPIs)
	}
	if len(d.Data.Tables) != 1 || len(d.Data.Tables[0].Columns) != 4 {
		t.Errorf("data tables = %+v", d.Data.Tables)
	}
	s := d.Facets.Summary
	if s == nil || s.Status == nil {
		t.Fatal("summary facet or status rule missing")
	}
	if s.Status.From != "failed_deploys" || s.Status.WarnAt != 1 || s.Status.CriticalAt != 3 || s.Status.Direction != "above" {
		t.Errorf("status rule = %+v", s.Status)
	}
	if s.Sparkline != "deploy_frequency" {
		t.Errorf("sparkline = %q", s.Sparkline)
	}
	if d.Facets.Pulse == nil || d.Facets.Pulse.RateLimit != "30/min" {
		t.Errorf("pulse facet = %+v", d.Facets.Pulse)
	}
}

func TestFacetNames(t *testing.T) {
	d, err := ParseDefinition([]byte(validDefinition))
	if err != nil {
		t.Fatalf("ParseDefinition: %v", err)
	}
	want := []string{"summary", "detail", "timeline", "pulse", "media", "conversation"}
	got := d.FacetNames()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("FacetNames = %v, want %v", got, want)
	}

	d.Facets.Media = nil
	d.Facets.Conversation = nil
	d.Facets.Pulse = nil
	want = []string{"summary", "detail", "timeline"}
	got = d.FacetNames()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("FacetNames = %v, want %v", got, want)
	}
}

const minimalHeader = `version: v1alpha1
kind: ReportDefinition
name: t
`

func TestParseDefinitionValidation(t *testing.T) {
	tests := []struct {
		name         string
		yaml         string
		wantContains []string
	}{
		{
			name:         "bad version",
			yaml:         "version: v2\nkind: ReportDefinition\nname: t\n",
			wantContains: []string{"version", `"v1alpha1"`},
		},
		{
			name:         "bad kind",
			yaml:         "version: v1alpha1\nkind: Report\nname: t\n",
			wantContains: []string{"kind", `"ReportDefinition"`},
		},
		{
			name:         "bad name pattern",
			yaml:         "version: v1alpha1\nkind: ReportDefinition\nname: Bad_Name\n",
			wantContains: []string{"name", "does not match"},
		},
		{
			name:         "unknown category",
			yaml:         minimalHeader + "categories: [delivery, sportsball]\n",
			wantContains: []string{"categories[1]", `"sportsball"`},
		},
		{
			name:         "unknown modality",
			yaml:         minimalHeader + "modalities: [telepathy]\n",
			wantContains: []string{"modalities[0]", `"telepathy"`},
		},
		{
			name:         "unknown scope level",
			yaml:         minimalHeader + "scope:\n  attach: [site, galaxy]\n",
			wantContains: []string{"scope.attach[1]", `"galaxy"`},
		},
		{
			name:         "bad aggregation policy",
			yaml:         minimalHeader + "scope:\n  aggregation:\n    status: bogus\n",
			wantContains: []string{"scope.aggregation.status", `"bogus"`},
		},
		{
			name:         "parameterized policy without parameter",
			yaml:         minimalHeader + "scope:\n  aggregation:\n    pulse: sample\n",
			wantContains: []string{"scope.aggregation.pulse", "requires a parameter"},
		},
		{
			name:         "aggregation kpi key not in data contract",
			yaml:         minimalHeader + "scope:\n  aggregation:\n    kpis: { ghost: sum }\n",
			wantContains: []string{"scope.aggregation.kpis[ghost]", "no such kpi"},
		},
		{
			name:         "time kpi key not in data contract",
			yaml:         minimalHeader + "scope:\n  aggregation:\n    time:\n      kpis: { ghost: sum }\n",
			wantContains: []string{"scope.aggregation.time.kpis[ghost]", "no such kpi"},
		},
		{
			name:         "time kpi policy not scalar",
			yaml:         minimalHeader + "data:\n  kpis:\n    - { name: n, unit: count }\nscope:\n  aggregation:\n    time:\n      kpis: { n: merge }\n",
			wantContains: []string{"scope.aggregation.time.kpis[n]", `"merge"`, "does not apply to kpis"},
		},
		{
			name:         "time status policy not a status policy",
			yaml:         minimalHeader + "scope:\n  aggregation:\n    time:\n      status: sum\n",
			wantContains: []string{"scope.aggregation.time.status", `"sum"`, "worst|latest"},
		},
		{
			name:         "time headline policy unknown",
			yaml:         minimalHeader + "scope:\n  aggregation:\n    time:\n      headline: bogus\n",
			wantContains: []string{"scope.aggregation.time.headline", `"bogus"`},
		},
		{
			name:         "bad event severity",
			yaml:         minimalHeader + "data:\n  events:\n    - { type: x.y, severity: fatal }\n",
			wantContains: []string{"data.events[0]", `"fatal"`},
		},
		{
			name: "unresolved sparkline",
			yaml: minimalHeader + `facets:
  summary:
    sparkline: ghost_series
`,
			wantContains: []string{"facets.summary.sparkline", `"ghost_series"`},
		},
		{
			name: "unresolved summary kpi",
			yaml: minimalHeader + `facets:
  summary:
    kpis: [ghost]
`,
			wantContains: []string{"facets.summary.kpis[0]", `"ghost"`},
		},
		{
			name: "unresolved status rule kpi",
			yaml: minimalHeader + `facets:
  summary:
    status: { from: ghost, warn_at: 1, critical_at: 2, direction: above }
`,
			wantContains: []string{"facets.summary.status.from", `"ghost"`},
		},
		{
			name: "bad status direction",
			yaml: minimalHeader + `data:
  kpis:
    - { name: x, unit: count }
facets:
  summary:
    status: { from: x, warn_at: 1, critical_at: 2, direction: sideways }
`,
			wantContains: []string{"facets.summary.status.direction", `"sideways"`},
		},
		{
			name: "headline placeholder unknown",
			yaml: minimalHeader + `data:
  kpis:
    - { name: x, unit: count }
facets:
  summary:
    headline: "{{x}} and {{ghost}}"
`,
			wantContains: []string{"facets.summary.headline", "{{ghost}}"},
		},
		{
			name: "bad span refs",
			yaml: minimalHeader + `data:
  events:
    - { type: a.start, severity: info }
facets:
  timeline:
    events: [a.start]
    spans:
      - { name: run, start: a.start, end: a.finish }
`,
			wantContains: []string{"facets.timeline.spans[0]", `end event "a.finish"`},
		},
		{
			name: "unresolved timeline event",
			yaml: minimalHeader + `facets:
  timeline:
    events: [ghost.event]
`,
			wantContains: []string{"facets.timeline.events[0]", `"ghost.event"`},
		},
		{
			name: "unresolved pulse event",
			yaml: minimalHeader + `facets:
  pulse:
    events: [ghost.event]
`,
			wantContains: []string{"facets.pulse.events[0]", `"ghost.event"`},
		},
		{
			name: "bad rate limit",
			yaml: minimalHeader + `facets:
  pulse:
    rate_limit: often
`,
			wantContains: []string{"facets.pulse.rate_limit", `"often"`},
		},
		{
			name: "bad panel data ref form",
			yaml: minimalHeader + `facets:
  detail:
    panels:
      - { title: X, primitive: datagrid, data: junk }
`,
			wantContains: []string{"facets.detail.panels[0]", `"junk"`},
		},
		{
			name: "unresolved panel series ref",
			yaml: minimalHeader + `facets:
  detail:
    panels:
      - { title: X, primitive: timeseries, data: series/ghost }
`,
			wantContains: []string{"facets.detail.panels[0]", `"series/ghost"`},
		},
		{
			name: "unresolved panel span ref",
			yaml: minimalHeader + `facets:
  detail:
    panels:
      - { title: X, primitive: gantt, data: spans/ghost }
`,
			wantContains: []string{"facets.detail.panels[0]", `"spans/ghost"`},
		},
		{
			name: "unknown primitive",
			yaml: minimalHeader + `data:
  series:
    - { name: s, unit: count }
facets:
  detail:
    panels:
      - { title: X, primitive: hologram, data: series/s }
`,
			wantContains: []string{"facets.detail.panels[0]", `"hologram"`},
		},
		{
			name: "unknown media archetype",
			yaml: minimalHeader + `facets:
  media:
    archetypes: [opera]
`,
			wantContains: []string{"facets.media.archetypes[0]", `"opera"`},
		},
		{
			name: "unknown conversation grounding",
			yaml: minimalHeader + `facets:
  conversation:
    grounding: [feelings]
`,
			wantContains: []string{"facets.conversation.grounding[0]", `"feelings"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDefinition([]byte(tt.yaml))
			if err == nil {
				t.Fatalf("ParseDefinition succeeded, want error (got %+v)", got)
			}
			if got != nil {
				t.Errorf("definition = %+v, want nil on error", got)
			}
			if !errors.Is(err, ErrInvalidDefinition) {
				t.Errorf("error = %v, want errors.Is(..., ErrInvalidDefinition)", err)
			}
			msg := err.Error()
			for _, want := range tt.wantContains {
				if !strings.Contains(msg, want) {
					t.Errorf("error %q does not contain %q", msg, want)
				}
			}
		})
	}
}

func TestParseDefinitionCollectsEveryProblem(t *testing.T) {
	const bad = `version: v9
kind: ReportDefinition
name: BAD NAME
categories: [nonsense]
scope:
  aggregation:
    status: bogus
data:
  kpis:
    - { name: x, unit: count }
facets:
  summary:
    kpis: [ghost]
    sparkline: nope
    headline: "{{missing}}"
`
	_, err := ParseDefinition([]byte(bad))
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("error = %v, want ErrInvalidDefinition", err)
	}
	msg := err.Error()
	for _, want := range []string{
		`"v9"`, "does not match", `"nonsense"`, `"bogus"`,
		`"ghost"`, `"nope"`, "{{missing}}",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
	if n := strings.Count(msg, "\n"); n < 6 {
		t.Errorf("expected at least 7 joined problems, got %d newlines in %q", n, msg)
	}
}

func TestParseDefinitionMalformedYAML(t *testing.T) {
	_, err := ParseDefinition([]byte("version: [ unclosed"))
	if err == nil {
		t.Fatal("expected error for malformed YAML")
	}
	if !strings.Contains(err.Error(), "reporting:") {
		t.Errorf("error %q should be package-prefixed", err.Error())
	}
}

func TestParseDefinitionExtendsDefersRefValidation(t *testing.T) {
	const child = `version: v1alpha1
kind: ReportDefinition
name: child
extends: base
facets:
  summary:
    kpis: [inherited_kpi]
`
	d, err := ParseDefinition([]byte(child))
	if err != nil {
		t.Fatalf("ParseDefinition(child with extends): %v", err)
	}
	if d.Extends != "base" {
		t.Errorf("Extends = %q, want base", d.Extends)
	}
}

func TestKPITargetValidation(t *testing.T) {
	tests := []struct {
		name    string
		kpi     string
		wantErr string
	}{
		{"target with direction", "{ name: days_to_close, unit: days, target: 6, direction: above }", ""},
		{"direction alone", "{ name: days_to_close, unit: days, direction: below }", ""},
		{"neither", "{ name: days_to_close, unit: days }", ""},
		{"unknown direction", "{ name: days_to_close, unit: days, direction: sideways }", "unknown direction"},
		{"target without direction", "{ name: days_to_close, unit: days, target: 6 }", "without a direction"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := "version: v1alpha1\nkind: ReportDefinition\nname: close\ncategories: [delivery]\ndata:\n  kpis:\n    - " + tc.kpi + "\n"
			_, err := ParseDefinition([]byte(src))
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tc.wantErr != "" && err == nil:
				t.Fatalf("want error containing %q, got nil", tc.wantErr)
			case tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr):
				t.Fatalf("error %v does not contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestTimeAggregationDefaults(t *testing.T) {
	var a AggregationSpec
	if a.TimeStatusPolicy().Name != "worst" || a.TimeHeadlinePolicy().Name != "latest" || a.TimeKPIPolicy("x").Name != "latest" {
		t.Errorf("undeclared time policies = %v/%v/%v, want worst/latest/latest",
			a.TimeStatusPolicy(), a.TimeHeadlinePolicy(), a.TimeKPIPolicy("x"))
	}
	a.Time = &TimeAggregationSpec{Status: "latest", Headline: "top(3)", KPIs: map[string]string{"x": "sum"}}
	if a.TimeStatusPolicy().Name != "latest" || a.TimeHeadlinePolicy().N != 3 || a.TimeKPIPolicy("x").Name != "sum" || a.TimeKPIPolicy("y").Name != "latest" {
		t.Errorf("declared time policies not honoured: %v/%v/%v/%v",
			a.TimeStatusPolicy(), a.TimeHeadlinePolicy(), a.TimeKPIPolicy("x"), a.TimeKPIPolicy("y"))
	}

	defs, err := LoadLibraryDir("../../reporting/library")
	if err != nil {
		t.Fatalf("LoadLibraryDir: %v", err)
	}
	byName := map[string]*Definition{}
	for _, d := range defs {
		byName[d.Name] = d
	}
	for def, kpis := range map[string][]string{
		"activity": {"tokens_in", "tokens_out", "cost_usd", "tasks_finished"},
		"cost":     {"tokens", "compute_minutes", "spend_usd"},
	} {
		d := byName[def]
		if d == nil {
			t.Fatalf("library has no %q", def)
		}
		for _, k := range kpis {
			if d.Scope.Aggregation.TimeKPIPolicy(k).Name != "sum" {
				t.Errorf("%s: time policy for %s = %v, want sum", def, k, d.Scope.Aggregation.TimeKPIPolicy(k))
			}
		}
	}
	if byName["activity"].Scope.Aggregation.TimeKPIPolicy("active_nodes").Name != "latest" {
		t.Errorf("activity: a gauge must not sum over time")
	}
}
