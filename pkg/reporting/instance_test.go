package reporting

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func testDef() *Definition {
	return &Definition{
		Version:     DefinitionVersion,
		Kind:        DefinitionKind,
		Name:        "delivery",
		Description: "Deploys and releases over time.",
		Categories:  []string{"delivery"},
		Scope: ScopeSpec{
			Attach: []string{"site", "realm"},
			Aggregation: AggregationSpec{
				Status:   "worst",
				Headline: "synthesize",
				KPIs: map[string]string{
					"deploys":           "sum",
					"failed_deploys":    "sum",
					"lead_time_minutes": "p50",
				},
				Timeline: "merge",
				Pulse:    "sample(50)",
			},
		},
		Data: DataContract{
			KPIs: []KPISpec{
				{Name: "deploys", Unit: "count", Window: "24h"},
				{Name: "failed_deploys", Unit: "count", Window: "24h"},
				{Name: "lead_time_minutes", Unit: "minutes", Window: "24h"},
			},
			Series: []SeriesSpec{{Name: "deploy_frequency", Unit: "per-hour"}},
			Events: []EventSpec{
				{Type: "deploy.started", Severity: SeverityInfo},
				{Type: "deploy.rolled_back", Severity: SeverityWarn},
			},
			Tables: []TableSpec{{Name: "recent_deploys", Columns: []string{"when", "service"}}},
		},
		Facets: Facets{
			Summary: &SummaryFacet{
				Status:    &StatusRule{From: "failed_deploys", WarnAt: 1, CriticalAt: 3, Direction: "above"},
				KPIs:      []string{"deploys", "failed_deploys", "lead_time_minutes"},
				Sparkline: "deploy_frequency",
				Headline:  "{{deploys}} deploys, {{failed_deploys}} failed",
			},
			Timeline: &TimelineFacet{Events: []string{"deploy.started", "deploy.rolled_back"}},
			Pulse:    &PulseFacet{Events: []string{"deploy.rolled_back"}, RateLimit: "30/min"},
		},
	}
}

func TestValidateInstanceValid(t *testing.T) {
	def := testDef()
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	inst := &Instance{
		Definition: "delivery",
		Scope:      "acme/music/platform/deploy-squad",
		Producer:   "node-7",
		ProducedAt: now,
		Status:     StatusWarn,
		KPIs:       map[string]float64{"deploys": 4, "failed_deploys": 1},
		Series: map[string][]SeriesPoint{
			"deploy_frequency": {{T: now, V: 2}},
		},
		Tables: map[string]Table{
			"recent_deploys": {Columns: []string{"when", "service"}, Rows: [][]string{{"12:00", "api"}}},
		},
		Events: []Event{
			{T: now, Type: "deploy.rolled_back", Severity: SeverityWarn, Label: "api v2 rolled back"},
		},
	}
	if err := ValidateInstance(def, inst); err != nil {
		t.Fatalf("ValidateInstance: %v", err)
	}
}

func TestValidateInstanceCollectsEveryProblem(t *testing.T) {
	def := testDef()
	inst := &Instance{
		Definition: "",
		Scope:      "",
		Status:     "meh",
		KPIs:       map[string]float64{"ghost_kpi": 1},
		Series:     map[string][]SeriesPoint{"ghost_series": nil},
		Tables:     map[string]Table{"ghost_table": {}},
		Events: []Event{
			{Type: "ghost.event", Severity: "fatal"},
		},
	}
	err := ValidateInstance(def, inst)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrInvalidInstance) {
		t.Fatalf("error = %v, want errors.Is(..., ErrInvalidInstance)", err)
	}
	msg := err.Error()
	for _, want := range []string{
		"definition: empty",
		"scope: empty",
		"produced_at: missing",
		`status: "meh"`,
		"kpis[ghost_kpi]",
		"series[ghost_series]",
		"tables[ghost_table]",
		`events[0]: no such event type "ghost.event"`,
		`events[0]: severity "fatal"`,
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
}

func TestValidateInstanceDefinitionMismatch(t *testing.T) {
	def := testDef()
	inst := &Instance{
		Definition: "incident",
		Scope:      "acme",
		ProducedAt: time.Now(),
	}
	err := ValidateInstance(def, inst)
	if !errors.Is(err, ErrInvalidInstance) {
		t.Fatalf("error = %v, want ErrInvalidInstance", err)
	}
	if !strings.Contains(err.Error(), `"incident" does not match definition "delivery"`) {
		t.Errorf("error %q should report the mismatch", err.Error())
	}
}

func TestValidateInstanceNil(t *testing.T) {
	if err := ValidateInstance(nil, &Instance{}); !errors.Is(err, ErrInvalidInstance) {
		t.Errorf("ValidateInstance(nil def) = %v, want ErrInvalidInstance", err)
	}
	if err := ValidateInstance(testDef(), nil); !errors.Is(err, ErrInvalidInstance) {
		t.Errorf("ValidateInstance(nil inst) = %v, want ErrInvalidInstance", err)
	}
}
