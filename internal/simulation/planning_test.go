package simulation

import (
	"sort"
	"testing"
	"time"

	"github.com/acumen-ai-org/robotdreams/pkg/reporting"
)

var planningDefs = map[string]bool{
	"plan": true, "task-board": true, "strategy": true, "backlog": true, "todo": true,
}

func TestPlanItemsAreDeclared(t *testing.T) {
	defs, err := reporting.LoadLibraryDir("../../reporting/library")
	if err != nil {
		t.Fatalf("load library: %v", err)
	}
	byName := map[string]*reporting.Definition{}
	for _, d := range defs {
		byName[d.Name] = d
	}

	g := mustGenerator(t)
	seen := map[string]bool{}
	for _, sub := range g.History() {
		if sub.Instance == nil || len(sub.Instance.Items) == 0 {
			continue
		}
		def := byName[sub.Instance.Definition]
		if def == nil {
			t.Fatalf("instance for unknown definition %q", sub.Instance.Definition)
		}
		seen[def.Name] = true
		if def.Facets.Plan == nil {
			t.Fatalf("%s emits items but declares no plan facet", def.Name)
		}
		if err := reporting.ValidateInstance(def, sub.Instance); err != nil {
			t.Fatalf("%s at %s: %v", def.Name, sub.Instance.Scope, err)
		}
	}

	for name := range planningDefs {
		if !seen[name] {
			t.Errorf("definition %q produced no items anywhere in the history", name)
		}
	}
}

func TestPlansAge(t *testing.T) {
	g := mustGenerator(t)

	type point struct {
		at      time.Time
		oldest  float64
		blocked int
		review  int
	}
	backlogs := map[string][]point{}
	boards := map[string][]point{}

	for _, sub := range g.History() {
		in := sub.Instance
		if in == nil {
			continue
		}
		switch in.Definition {
		case "backlog":
			backlogs[in.Scope] = append(backlogs[in.Scope], point{at: in.ProducedAt, oldest: in.KPIs["oldest_days"]})
		case "task-board":
			review := 0
			for _, it := range in.Items {
				if it.State == "review" {
					review++
				}
			}
			boards[in.Scope] = append(boards[in.Scope], point{
				at: in.ProducedAt, blocked: int(in.KPIs["items_blocked"]), review: review,
			})
		}
	}
	if len(backlogs) == 0 || len(boards) == 0 {
		t.Fatal("no planning history generated")
	}

	for scope, pts := range backlogs {
		sort.Slice(pts, func(i, j int) bool { return pts[i].at.Before(pts[j].at) })
		for i := 1; i < len(pts); i++ {
			if pts[i].oldest < pts[i-1].oldest-1 {
				t.Fatalf("%s: oldest_days went backwards, %.0f -> %.0f — cards are being reinvented, not aged",
					scope, pts[i-1].oldest, pts[i].oldest)
			}
		}
	}

	moved := false
	for _, pts := range boards {
		sort.Slice(pts, func(i, j int) bool { return pts[i].at.Before(pts[j].at) })
		if len(pts) < 2 {
			continue
		}
		if pts[len(pts)-1].review > pts[0].review {
			moved = true
			break
		}
	}
	if !moved {
		t.Error("no board's review column grew across the window: the boards are static")
	}
}

func TestPlanItemsAreStableAcrossTicks(t *testing.T) {
	g := mustGenerator(t)

	byScope := map[string][]*reporting.Instance{}
	for _, sub := range g.History() {
		if sub.Instance != nil && sub.Instance.Definition == "task-board" {
			byScope[sub.Instance.Scope] = append(byScope[sub.Instance.Scope], sub.Instance)
		}
	}

	rank := map[string]int{}
	for i, st := range taskBoardStates {
		rank[st] = i
	}

	checked, overlaps := 0, 0
	for scope, list := range byScope {
		if len(list) < 2 {
			continue
		}
		sort.Slice(list, func(i, j int) bool { return list[i].ProducedAt.Before(list[j].ProducedAt) })
		first, last := list[0], list[len(list)-1]
		was := map[string]reporting.Item{}
		for _, it := range first.Items {
			was[it.ID] = it
		}
		for _, now := range last.Items {
			prev, ok := was[now.ID]
			if !ok {
				continue
			}
			overlaps++
			if prev.Title != now.Title || prev.Lane != now.Lane || prev.Owner != now.Owner {
				t.Fatalf("%s: card %q changed identity between ticks: %+v then %+v", scope, now.ID, prev, now)
			}
			if rank[now.State] < rank[prev.State] {
				t.Fatalf("%s: card %q moved backwards, %s -> %s", scope, now.ID, prev.State, now.State)
			}
		}
		checked++
	}
	if checked == 0 || overlaps == 0 {
		t.Fatal("no board was observed twice with any card in common; the property is untested")
	}
}

func TestPlanningReportsAreRouted(t *testing.T) {
	producers := map[string]map[Archetype]bool{}
	for _, universe := range UniverseNames() {
		co := mustCompany(t, universe)
		for _, tm := range co.Teams {
			for _, def := range tm.ReportDefinitions() {
				if !planningDefs[def] {
					continue
				}
				if producers[def] == nil {
					producers[def] = map[Archetype]bool{}
				}
				producers[def][tm.Archetype] = true
			}
			has := map[string]bool{}
			for _, def := range tm.ReportDefinitions() {
				has[def] = true
			}
			if has["backlog"] && (has["support-queue"] || has["it-service"]) {
				t.Errorf("%s (%s) reports both backlog and its own intake report", tm.Scope(), tm.Archetype)
			}
			if has["task-board"] && has["sales-pipeline"] {
				t.Errorf("%s (%s) reports both task-board and sales-pipeline — the pipeline is the board", tm.Scope(), tm.Archetype)
			}
		}
	}

	for _, def := range []string{"task-board", "backlog", "todo"} {
		if n := len(producers[def]); n < 2 {
			t.Errorf("definition %q is produced by %d archetype(s); a definition only one kind of team emits is a fixture of one", def, n)
		}
	}
}

func mustGenerator(t *testing.T) *Generator {
	t.Helper()
	co := mustCompany(t, "spookify")
	return NewGenerator(co, 1, time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC), 48*time.Hour)
}
