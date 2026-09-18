package reporting

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func planDef() *Definition {
	return &Definition{
		Version:    DefinitionVersion,
		Kind:       DefinitionKind,
		Name:       "task-board",
		Categories: []string{"roadmap"},
		Scope: ScopeSpec{
			Attach:      []string{"site"},
			Aggregation: AggregationSpec{Status: "worst", Items: "merge"},
		},
		Data: DataContract{
			KPIs:   []KPISpec{{Name: "committed", Unit: "count"}},
			Series: []SeriesSpec{{Name: "remaining", Unit: "count"}},
		},
		Facets: Facets{
			Plan: &PlanFacet{
				States:      []string{"backlog", "doing", "review", "done"},
				Lanes:       []string{"platform", "growth"},
				Horizons:    []string{"now", "next", "later"},
				Commitments: []string{"committed", "planned", "candidate"},
				Levels:      []string{"activity", "task"},
				WIPLimits:   map[string]int{"doing": 3},
				SizeUnit:    "points",
				Baseline:    &PlanBaseline{Name: "Committed", Ref: "kpi/committed"},
			},
		},
	}
}

func item(id, state string) Item { return Item{ID: id, Title: id, State: state} }

func validateDef(d *Definition) error {
	problems := d.validateCore()
	if len(problems) == 0 {
		problems = append(problems, d.validateRefs()...)
	}
	if len(problems) == 0 {
		return nil
	}
	return errors.Join(problems...)
}

func TestPlanFacetNamesLast(t *testing.T) {
	d := planDef()
	got := d.FacetNames()
	if len(got) != 1 || got[0] != "plan" {
		t.Fatalf("FacetNames = %v, want [plan]", got)
	}

	d.Facets.Summary = &SummaryFacet{KPIs: []string{"committed"}}
	d.Facets.Timeline = &TimelineFacet{Events: []string{}}
	if got := d.FacetNames(); got[len(got)-1] != "plan" {
		t.Fatalf("FacetNames = %v, want plan last", got)
	}
}

func TestPlanFacetOrdering(t *testing.T) {
	f := planDef().Facets.Plan
	if got := f.StateRank("doing"); got != 1 {
		t.Errorf("StateRank(doing) = %d, want 1", got)
	}
	if got := f.StateRank("nope"); got != -1 {
		t.Errorf("StateRank(nope) = %d, want -1", got)
	}
	if !f.Terminal("done") || f.Terminal("review") {
		t.Error("Terminal should be true only for the last declared state")
	}
	var nilFacet *PlanFacet
	if nilFacet.StateRank("x") != -1 || nilFacet.Terminal("x") {
		t.Error("nil facet helpers must not panic and must report nothing")
	}
}

func TestValidateDefinitionPlanProblems(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Definition)
		want string
	}{
		{"no states", func(d *Definition) { d.Facets.Plan.States = nil }, "facets.plan.states: empty"},
		{"duplicate state", func(d *Definition) { d.Facets.Plan.States = []string{"a", "a"} }, "duplicate"},
		{"unknown level", func(d *Definition) { d.Facets.Plan.Levels = []string{"epic"} }, "unknown level"},
		{"wip limit on undeclared state", func(d *Definition) { d.Facets.Plan.WIPLimits = map[string]int{"nope": 2} }, "no such state"},
		{"baseline without name", func(d *Definition) { d.Facets.Plan.Baseline.Name = "" }, "baseline.name"},
		{"baseline to a table", func(d *Definition) { d.Facets.Plan.Baseline.Ref = "table/x" }, "cannot be a baseline"},
		{"baseline to a ghost kpi", func(d *Definition) { d.Facets.Plan.Baseline.Ref = "kpi/ghost" }, "no such kpi"},
		{"averaging a board", func(d *Definition) { d.Scope.Aggregation.Items = "avg" }, "does not apply to items"},
		{"items policy without a plan", func(d *Definition) { d.Facets.Plan = nil }, "without a facets.plan"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := planDef()
			tc.mut(d)
			err := validateDef(d)
			if err == nil {
				t.Fatalf("want a problem containing %q, got none", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q in error, got: %v", tc.want, err)
			}
		})
	}
}

func TestPanelItemsRefNeedsPlanFacet(t *testing.T) {
	d := planDef()
	d.Facets.Detail = &DetailFacet{Panels: []PanelSpec{{Title: "Board", Primitive: "board", Data: "items", Section: "next"}}}
	if err := validateDef(d); err != nil {
		t.Fatalf("items panel with a plan facet should validate: %v", err)
	}

	d.Facets.Plan = nil
	d.Scope.Aggregation.Items = ""
	err := validateDef(d)
	if err == nil || !strings.Contains(err.Error(), "declares no plan facet") {
		t.Fatalf("want a no-plan-facet problem, got: %v", err)
	}
}

func TestNextSectionIsStrategic(t *testing.T) {
	p := PanelSpec{Section: "next"}
	got := p.Stances()
	if len(got) != 1 || got[0] != "strategic" {
		t.Fatalf("next section stances = %v, want [strategic]", got)
	}
}

func TestValidateInstanceItems(t *testing.T) {
	def := planDef()
	base := func() *Instance {
		return &Instance{Definition: "task-board", Scope: "a/b", ProducedAt: time.Now()}
	}

	t.Run("valid", func(t *testing.T) {
		in := base()
		due := time.Now().Add(48 * time.Hour)
		in.Items = []Item{
			{
				ID: "1", Title: "Ship it", State: "doing", Lane: "platform", Horizon: "now",
				Commitment: "committed", Level: "task", Due: &due, Size: 3,
			},
			{ID: "2", Title: "Then this", State: "backlog", BlockedBy: []string{"1"}},
		}
		if err := ValidateInstance(def, in); err != nil {
			t.Fatalf("valid items rejected: %v", err)
		}
	})

	cases := []struct {
		name  string
		items []Item
		want  string
	}{
		{"empty id", []Item{{Title: "t", State: "doing"}}, "empty id"},
		{"duplicate id", []Item{item("1", "doing"), item("1", "done")}, "duplicate item id"},
		{"empty title", []Item{{ID: "1", State: "doing"}}, "empty title"},
		{"empty state", []Item{{ID: "1", Title: "t"}}, "empty state"},
		{"undeclared state", []Item{item("1", "shipping")}, `state "shipping" not one of`},
		{"undeclared lane", []Item{{ID: "1", Title: "t", State: "doing", Lane: "nope"}}, `lane "nope" not one of`},
		{"undeclared horizon", []Item{{ID: "1", Title: "t", State: "doing", Horizon: "someday"}}, "horizon"},
		{"level outside the vocabulary", []Item{{ID: "1", Title: "t", State: "doing", Level: "epic"}}, "not one of goal"},
		{"level this plan does not publish", []Item{{ID: "1", Title: "t", State: "doing", Level: "goal"}}, "not published by this plan"},
		{"negative size", []Item{{ID: "1", Title: "t", State: "doing", Size: -1}}, "negative size"},
		{"self block", []Item{{ID: "1", Title: "t", State: "doing", BlockedBy: []string{"1"}}}, "blocks itself"},
		{"dangling edge", []Item{{ID: "1", Title: "t", State: "doing", BlockedBy: []string{"9"}}}, "cross-instance dependencies are not modelled"},
		{"two-item cycle", []Item{
			{ID: "1", Title: "t", State: "doing", BlockedBy: []string{"2"}},
			{ID: "2", Title: "t", State: "doing", BlockedBy: []string{"1"}},
		}, "dependency cycle"},
		{"three-item cycle", []Item{
			{ID: "1", Title: "t", State: "doing", BlockedBy: []string{"2"}},
			{ID: "2", Title: "t", State: "doing", BlockedBy: []string{"3"}},
			{ID: "3", Title: "t", State: "doing", BlockedBy: []string{"1"}},
		}, "dependency cycle"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := base()
			in.Items = tc.items
			err := ValidateInstance(def, in)
			if err == nil {
				t.Fatalf("want a problem containing %q, got none", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got: %v", tc.want, err)
			}
		})
	}

	t.Run("items with no plan facet", func(t *testing.T) {
		bare := planDef()
		bare.Facets.Plan = nil
		bare.Scope.Aggregation.Items = ""
		in := base()
		in.Items = []Item{item("1", "doing")}
		err := ValidateInstance(bare, in)
		if err == nil || !strings.Contains(err.Error(), "declares no plan facet") {
			t.Fatalf("want a no-plan-facet problem, got: %v", err)
		}
	})

	t.Run("every fault is reported", func(t *testing.T) {
		in := base()
		in.Items = []Item{
			{ID: "", Title: "", State: "nope", Size: -4},
			{ID: "x", Title: "t", State: "doing", Lane: "ghost"},
		}
		err := ValidateInstance(def, in)
		for _, want := range []string{"empty id", "empty title", "not one of", "negative size", "lane"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("missing %q in joined error: %v", want, err)
			}
		}
	})
}

func planInstance(scope string, at time.Time, items ...Item) *Instance {
	return &Instance{Definition: "task-board", Scope: scope, ProducedAt: at, Items: items}
}

func TestAggregatePlanNilWithoutFacet(t *testing.T) {
	d := planDef()
	d.Facets.Plan = nil
	d.Scope.Aggregation.Items = ""
	if got := AggregatePlan(d, "a", nil); got != nil {
		t.Fatalf("want nil for a definition with no plan facet, got %+v", got)
	}
}

func TestAggregatePlanMergeStampsAndOrders(t *testing.T) {
	def := planDef()
	t0 := time.Unix(1700000000, 0).UTC()
	view := AggregatePlan(def, "acme", []*Instance{
		planInstance("acme/one", t0, item("a", "doing"), item("b", "done")),
		planInstance("acme/two", t0.Add(time.Minute), item("c", "backlog")),
	})

	if view.Total != 3 || view.Instances != 2 {
		t.Fatalf("Total=%d Instances=%d, want 3 and 2", view.Total, view.Instances)
	}

	wantCols := []string{"backlog", "doing", "review", "done"}
	if len(view.States) != len(wantCols) {
		t.Fatalf("got %d columns, want %d", len(view.States), len(wantCols))
	}
	for i, w := range wantCols {
		if view.States[i].State != w {
			t.Fatalf("column %d = %q, want %q", i, view.States[i].State, w)
		}
	}
	for _, it := range view.Items {
		if it.Scope == "" || it.Definition != "task-board" {
			t.Fatalf("item %q not origin-stamped: %+v", it.ID, it)
		}
	}

	if view.Items[len(view.Items)-1].ID != "b" {
		t.Fatalf("terminal item should sort last, got order %v", ids(view.Items))
	}

	if col := column(view, "doing"); col.Limit != 6 {
		t.Fatalf("doing limit = %d, want 6 (3 x 2 contributors)", col.Limit)
	}
}

func TestAggregatePlanSupersedes(t *testing.T) {
	def := planDef()
	t0 := time.Unix(1700000000, 0).UTC()
	view := AggregatePlan(def, "acme", []*Instance{
		planInstance("acme/one", t0, item("old", "doing")),
		planInstance("acme/one", t0.Add(time.Hour), item("new", "doing")),
	})
	if view.Total != 1 || view.Items[0].ID != "new" {
		t.Fatalf("a newer instance must supersede the older one, got %v", ids(view.Items))
	}
}

func TestAggregatePlanCountsSurviveBounding(t *testing.T) {
	def := planDef()
	def.Scope.Aggregation.Items = "sample(1)"
	t0 := time.Unix(1700000000, 0).UTC()

	var items []Item
	for _, id := range []string{"a", "b", "c"} {
		items = append(items, item(id, "doing"))
	}
	items = append(items, Item{ID: "blocked", Title: "blocked", State: "doing", BlockedBy: []string{"a"}})
	view := AggregatePlan(def, "acme", []*Instance{planInstance("acme/one", t0, items...)})

	col := column(view, "doing")
	if col.Items != 4 {
		t.Fatalf("column count = %d, want 4 — bounding must never move a total", col.Items)
	}
	if col.Shown != 1 || len(view.Items) != 1 {
		t.Fatalf("sample(1) should keep one card per column, got shown=%d len=%d", col.Shown, len(view.Items))
	}
	if !view.Truncated {
		t.Error("Truncated should be set when cards were dropped")
	}

	if view.Items[0].ID != "blocked" {
		t.Fatalf("kept %q, want the blocked card", view.Items[0].ID)
	}
	if view.Blocked != 1 {
		t.Fatalf("Blocked = %d, want 1", view.Blocked)
	}
}

func TestAggregatePlanCountPolicyDropsEveryCard(t *testing.T) {
	def := planDef()
	def.Scope.Aggregation.Items = "count"
	t0 := time.Unix(1700000000, 0).UTC()
	view := AggregatePlan(def, "acme", []*Instance{
		planInstance("acme/one", t0, item("a", "doing"), item("b", "doing")),
	})
	if len(view.Items) != 0 {
		t.Fatalf("count must carry no cards, got %d", len(view.Items))
	}
	if column(view, "doing").Items != 2 || view.Total != 2 {
		t.Fatal("count must still report exact totals")
	}
}

func TestAggregatePlanTopPolicy(t *testing.T) {
	def := planDef()
	def.Scope.Aggregation.Items = "top(2)"
	t0 := time.Unix(1700000000, 0).UTC()
	view := AggregatePlan(def, "acme", []*Instance{
		planInstance("acme/one", t0, item("a", "doing"), item("b", "backlog"), item("c", "review")),
	})
	if len(view.Items) != 2 {
		t.Fatalf("top(2) kept %d cards, want 2", len(view.Items))
	}
}

func TestAggregatePlanCollidingIDsAcrossScopes(t *testing.T) {
	def := planDef()
	t0 := time.Unix(1700000000, 0).UTC()
	view := AggregatePlan(def, "acme", []*Instance{
		planInstance("acme/one", t0, item("1", "doing")),
		planInstance("acme/two", t0, item("1", "doing")),
	})
	if view.Total != 2 {
		t.Fatalf("two teams publishing id %q must both survive, got %d", "1", view.Total)
	}
	if view.Items[0].Scope == view.Items[1].Scope {
		t.Fatal("colliding ids must stay distinguishable by scope")
	}
}

func TestAggregatePlanIsDeterministic(t *testing.T) {
	def := planDef()
	t0 := time.Unix(1700000000, 0).UTC()
	a := planInstance("acme/one", t0, item("a", "doing"), item("b", "done"))
	b := planInstance("acme/two", t0.Add(time.Minute), item("c", "backlog"), item("d", "review"))

	first, err := json.Marshal(AggregatePlan(def, "acme", []*Instance{a, b}))
	if err != nil {
		t.Fatal(err)
	}
	second, err := json.Marshal(AggregatePlan(def, "acme", []*Instance{b, a}))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("input order changed the view:\n%s\n%s", first, second)
	}
}

func column(v *PlanView, state string) PlanColumn {
	for _, c := range v.States {
		if c.State == state {
			return c
		}
	}
	return PlanColumn{}
}

func ids(items []Item) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.ID)
	}
	return out
}
