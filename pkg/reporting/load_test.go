package reporting

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeLibrary(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", name, err)
		}
	}
	return dir
}

const baseDefinition = `version: v1alpha1
kind: ReportDefinition
name: base
description: base description
categories: [performance]
modalities: [glance]
scope:
  attach: [site]
  aggregation:
    status: worst
    kpis:
      latency_ms: p95
data:
  kpis:
    - { name: latency_ms, unit: ms }
  series:
    - { name: latency, unit: ms }
facets:
  summary:
    kpis: [latency_ms]
    sparkline: latency
`

func TestLoadDefinitionFile(t *testing.T) {
	dir := writeLibrary(t, map[string]string{"base.yaml": baseDefinition})

	d, err := LoadDefinitionFile(filepath.Join(dir, "base.yaml"))
	if err != nil {
		t.Fatalf("LoadDefinitionFile: %v", err)
	}
	if d.Name != "base" {
		t.Errorf("Name = %q, want base", d.Name)
	}

	t.Run("missing file", func(t *testing.T) {
		_, err := LoadDefinitionFile(filepath.Join(dir, "nope.yaml"))
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("error = %v, want os.ErrNotExist", err)
		}
	})

	t.Run("invalid contents keep path in message", func(t *testing.T) {
		path := filepath.Join(dir, "bad.yaml")
		if err := os.WriteFile(path, []byte("version: v9\nkind: ReportDefinition\nname: x\n"), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		_, err := LoadDefinitionFile(path)
		if !errors.Is(err, ErrInvalidDefinition) {
			t.Fatalf("error = %v, want ErrInvalidDefinition", err)
		}
		if !strings.Contains(err.Error(), "bad.yaml") {
			t.Errorf("error %q should name the file", err.Error())
		}
	})
}

func TestLoadLibraryDirExtends(t *testing.T) {
	dir := writeLibrary(t, map[string]string{
		"base.yaml": baseDefinition,
		"child.yaml": `version: v1alpha1
kind: ReportDefinition
name: child
extends: base
description: child override
scope:
  attach: [realm]
  aggregation:
    status: worst
    kpis:
      latency_ms: max
`,
	})

	defs, err := LoadLibraryDir(dir)
	if err != nil {
		t.Fatalf("LoadLibraryDir: %v", err)
	}
	if len(defs) != 2 {
		t.Fatalf("got %d definitions, want 2", len(defs))
	}
	// Filename order: base.yaml then child.yaml.
	base, child := defs[0], defs[1]
	if base.Name != "base" || child.Name != "child" {
		t.Fatalf("names = %q, %q; want base, child", base.Name, child.Name)
	}

	// Non-zero child fields override wholesale.
	if child.Description != "child override" {
		t.Errorf("child.Description = %q, want the override", child.Description)
	}
	if len(child.Scope.Attach) != 1 || child.Scope.Attach[0] != "realm" {
		t.Errorf("child.Scope.Attach = %v, want [realm]", child.Scope.Attach)
	}
	if got := child.Scope.Aggregation.KPIs["latency_ms"]; got != "max" {
		t.Errorf("child kpi policy = %q, want max", got)
	}

	// Zero child fields inherit from the parent.
	if len(child.Categories) != 1 || child.Categories[0] != "performance" {
		t.Errorf("child.Categories = %v, want inherited [performance]", child.Categories)
	}
	if len(child.Data.KPIs) != 1 || child.Data.KPIs[0].Name != "latency_ms" {
		t.Errorf("child.Data.KPIs = %+v, want inherited latency_ms", child.Data.KPIs)
	}
	if child.Facets.Summary == nil || child.Facets.Summary.Sparkline != "latency" {
		t.Errorf("child.Facets.Summary = %+v, want inherited", child.Facets.Summary)
	}
	// Inherited structures are deep copies, never aliases of the parent.
	if child.Facets.Summary == base.Facets.Summary {
		t.Error("child summary facet aliases the parent's")
	}
}

func TestLoadLibraryDirExtendsChain(t *testing.T) {
	dir := writeLibrary(t, map[string]string{
		"a-base.yaml": baseDefinition,
		"b-mid.yaml": `version: v1alpha1
kind: ReportDefinition
name: mid
extends: base
description: mid override
`,
		"c-leaf.yaml": `version: v1alpha1
kind: ReportDefinition
name: leaf
extends: mid
modalities: [delta]
`,
	})

	defs, err := LoadLibraryDir(dir)
	if err != nil {
		t.Fatalf("LoadLibraryDir: %v", err)
	}
	leaf := defs[2]
	if leaf.Name != "leaf" {
		t.Fatalf("defs[2].Name = %q, want leaf", leaf.Name)
	}
	if leaf.Description != "mid override" {
		t.Errorf("leaf.Description = %q, want inherited mid override", leaf.Description)
	}
	if len(leaf.Modalities) != 1 || leaf.Modalities[0] != "delta" {
		t.Errorf("leaf.Modalities = %v, want [delta]", leaf.Modalities)
	}
	if len(leaf.Data.KPIs) != 1 {
		t.Errorf("leaf.Data.KPIs = %+v, want inherited from base", leaf.Data.KPIs)
	}
}

func TestLoadLibraryDirErrors(t *testing.T) {
	tests := []struct {
		name         string
		files        map[string]string
		wantContains []string
	}{
		{
			name: "extends cycle",
			files: map[string]string{
				"a.yaml": "version: v1alpha1\nkind: ReportDefinition\nname: a\nextends: b\n",
				"b.yaml": "version: v1alpha1\nkind: ReportDefinition\nname: b\nextends: a\n",
			},
			wantContains: []string{"extends cycle"},
		},
		{
			name: "extends itself",
			files: map[string]string{
				"a.yaml": "version: v1alpha1\nkind: ReportDefinition\nname: a\nextends: a\n",
			},
			wantContains: []string{"extends itself"},
		},
		{
			name: "missing extends target",
			files: map[string]string{
				"a.yaml": "version: v1alpha1\nkind: ReportDefinition\nname: a\nextends: ghost\n",
			},
			wantContains: []string{`extends "ghost"`, "no such definition"},
		},
		{
			name: "duplicate definition name",
			files: map[string]string{
				"a.yaml": "version: v1alpha1\nkind: ReportDefinition\nname: same\n",
				"b.yaml": "version: v1alpha1\nkind: ReportDefinition\nname: same\n",
			},
			wantContains: []string{"duplicate definition name", `"same"`},
		},
		{
			name: "cross-file drilldown does not resolve",
			files: map[string]string{
				"a.yaml": baseDefinition + `  detail:
    panels:
      - { title: Latency, primitive: timeseries, data: series/latency }
    drilldown: [ghost-report]
`,
			},
			wantContains: []string{"facets.detail.drilldown[0]", `"ghost-report"`},
		},
		{
			name: "child references data the parent does not declare",
			files: map[string]string{
				"base.yaml": baseDefinition,
				"child.yaml": `version: v1alpha1
kind: ReportDefinition
name: child
extends: base
facets:
  summary:
    kpis: [no_such_kpi]
`,
			},
			wantContains: []string{"child.yaml", `"no_such_kpi"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeLibrary(t, tt.files)
			defs, err := LoadLibraryDir(dir)
			if err == nil {
				t.Fatalf("LoadLibraryDir succeeded, want error (got %d defs)", len(defs))
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

func TestLoadLibraryDirMissingDir(t *testing.T) {
	_, err := LoadLibraryDir(filepath.Join(t.TempDir(), "nope"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("error = %v, want os.ErrNotExist", err)
	}
}

// TestLoadShippedLibrary guards the checked-in report library that the
// server registers at startup: every definition must load and
// cross-validate with no error.
func TestLoadShippedLibrary(t *testing.T) {
	dir := filepath.Join("..", "..", "reporting", "library")
	defs, err := LoadLibraryDir(dir)
	if err != nil {
		t.Fatalf("LoadLibraryDir(%s): %v", dir, err)
	}
	// The nine generic per-category templates must always be present.
	// The library also ships the department reports a real company
	// produces (a month-end close, a hiring funnel, a support queue,
	// and so on), and those are expected to grow — so this asserts the
	// floor and the ordering contract, not an exact count. A count
	// assertion here would only ever be a chore to update.
	want := []string{"activity", "cost", "decisions", "delivery", "incident", "logs", "performance", "quality", "roadmap"}
	byName := map[string]bool{}
	for _, d := range defs {
		byName[d.Name] = true
	}
	for _, name := range want {
		if !byName[name] {
			t.Errorf("shipped library is missing the %q template", name)
		}
	}
	if len(defs) < len(want) {
		t.Fatalf("got %d definitions, want at least %d", len(defs), len(want))
	}

	// Every shipped definition declares its stances. The SCHEMA makes
	// the field optional — required would reject a perfectly good
	// third-party definition, and "no opinion" is a real answer — but
	// the shipped library is the reference implementation and is held to
	// a higher bar: a report here that has not said how it should be
	// judged has not finished being written.
	for _, d := range defs {
		if len(d.Stances) == 0 {
			t.Errorf("definition %q declares no stances", d.Name)
		}
		for _, st := range d.Stances {
			if !contains(Stances, st) {
				t.Errorf("definition %q: unknown stance %q", d.Name, st)
			}
		}
	}

	// LoadLibraryDir promises filename order; the definition name is
	// the filename stem throughout the library, so the loaded set must
	// come back sorted by name.
	for i := 1; i < len(defs); i++ {
		if defs[i-1].Name > defs[i].Name {
			t.Errorf("defs[%d] %q sorts after defs[%d] %q; want filename order",
				i-1, defs[i-1].Name, i, defs[i].Name)
		}
	}

	// Spot-check delivery, the fully-faceted template.
	r := NewRegistry(defs)
	delivery, ok := r.Get("delivery")
	if !ok {
		t.Fatal("delivery not found")
	}
	wantFacets := []string{"summary", "detail", "timeline", "pulse", "media", "conversation"}
	if got := delivery.FacetNames(); strings.Join(got, ",") != strings.Join(wantFacets, ",") {
		t.Errorf("delivery facets = %v, want %v", got, wantFacets)
	}
	agg := delivery.Scope.Aggregation
	if agg.Status != "worst" || agg.Headline != "synthesize" || agg.Timeline != "merge" || agg.Pulse != "sample(50)" {
		t.Errorf("delivery aggregation = %+v", agg)
	}
	if got := agg.KPIs["lead_time_minutes"]; got != "p50" {
		t.Errorf("delivery lead_time_minutes policy = %q, want p50", got)
	}
	if len(delivery.Scope.Attach) != 2 || delivery.Scope.Attach[0] != "site" || delivery.Scope.Attach[1] != "realm" {
		t.Errorf("delivery attach = %v, want [site realm]", delivery.Scope.Attach)
	}
	if delivery.Facets.Summary.Sparkline != "deploy_frequency" {
		t.Errorf("delivery sparkline = %q", delivery.Facets.Summary.Sparkline)
	}
	dd := delivery.Facets.Detail.Drilldown
	if len(dd) != 2 || dd[0] != "incident" || dd[1] != "logs" {
		t.Errorf("delivery drilldown = %v, want [incident logs]", dd)
	}
}

// A child that declares only a time fold still counts as overriding the
// scope block (wholesale, like every other field), and an inherited time
// fold is a deep copy.
func TestLoadLibraryDirExtendsTimeAggregation(t *testing.T) {
	dir := writeLibrary(t, map[string]string{
		"base.yaml": strings.Replace(baseDefinition, "  aggregation:\n", "  aggregation:\n    time:\n      kpis: { latency_ms: max }\n", 1),
		"child.yaml": `version: v1alpha1
kind: ReportDefinition
name: child
extends: base
`,
		"other.yaml": `version: v1alpha1
kind: ReportDefinition
name: other
extends: base
scope:
  aggregation:
    time:
      kpis: { latency_ms: p95 }
`,
	})
	defs, err := LoadLibraryDir(dir)
	if err != nil {
		t.Fatalf("LoadLibraryDir: %v", err)
	}
	byName := map[string]*Definition{}
	for _, d := range defs {
		byName[d.Name] = d
	}
	base, child, other := byName["base"], byName["child"], byName["other"]
	if child.Scope.Aggregation.Time == nil || child.Scope.Aggregation.TimeKPIPolicy("latency_ms").Name != "max" {
		t.Errorf("child did not inherit the time fold: %+v", child.Scope.Aggregation.Time)
	}
	if child.Scope.Aggregation.Time == base.Scope.Aggregation.Time {
		t.Error("child time spec aliases the parent's")
	}
	if other.Scope.Aggregation.TimeKPIPolicy("latency_ms").Name != "p95" || len(other.Scope.Attach) != 0 {
		t.Errorf("other's scope block should replace the parent's wholesale: %+v", other.Scope)
	}
}
