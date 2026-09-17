package simulation

import (
	"strings"
	"testing"

	"github.com/acumen-ai-org/robotdreams/pkg/reporting"
)

func forEachUniverse(t *testing.T, fn func(t *testing.T, co *Company)) {
	t.Helper()
	for _, name := range UniverseNames() {
		co, err := LoadCompany(name)
		if err != nil {
			t.Fatalf("load universe %q: %v", name, err)
		}
		t.Run(name, func(t *testing.T) { fn(t, co) })
	}
}

func TestCompanyFixture(t *testing.T) {
	forEachUniverse(t, testCompanyFixture)
}

func testCompanyFixture(t *testing.T, co *Company) {
	specs := co.Workers()

	byID := map[string]WorkerSpec{}
	for _, w := range specs {
		if _, dup := byID[w.ID]; dup {
			t.Errorf("duplicate worker ID %q", w.ID)
		}
		byID[w.ID] = w
	}

	members := 0
	for _, tm := range co.Teams {
		members += len(tm.Roles)
	}
	wantWorkers := 1 + len(co.Divisions()) + len(co.Departments()) + members
	if len(specs) != wantWorkers {
		t.Errorf("Workers: got %d workers, want %d", len(specs), wantWorkers)
	}

	for _, tm := range co.Teams {
		if d := reporting.ScopeDepth(tm.Scope()); d != 4 {
			t.Errorf("team scope %q: depth %d, want 4", tm.Scope(), d)
		}
		if !strings.HasPrefix(tm.Scope(), co.Name+"/") {
			t.Errorf("team scope %q: not under universe %q", tm.Scope(), co.Name)
		}
		if _, ok := byID[tm.ProducerID()]; !ok {
			t.Errorf("team %s: producer %q not in the worker set", tm.Name, tm.ProducerID())
		}
		if len(tm.Roles) == 0 {
			t.Errorf("team %s: no roles", tm.Name)
		}
		if tm.Headcount < len(tm.Roles) {
			t.Errorf("team %s: headcount %d is below its %d nodes", tm.Name, tm.Headcount, len(tm.Roles))
		}
		if co.Scale == ScaleEnterprise && tm.Headcount <= len(tm.Roles) {
			t.Errorf("team %s: headcount %d is not larger than its %d nodes", tm.Name, tm.Headcount, len(tm.Roles))
		}
		if len(tm.ReportDefinitions()) == 0 {
			t.Errorf("team %s: archetype %q maps to no report definitions", tm.Name, tm.Archetype)
		}
	}
	for _, d := range co.Departments() {
		if got := reporting.ScopeDepth(d.Scope()); got != 3 {
			t.Errorf("department scope %q: depth %d, want 3", d.Scope(), got)
		}
	}

	contributorRoles := map[string]bool{
		"orchestrator": true, "builder": true, "researcher": true, "writer": true,
		"reviewer": true, "ops": true, "librarian": true, "messenger": true, "guardian": true,
		"delegate": true,
	}

	for _, w := range specs {
		if reporting.SplitScope(w.Scope) == nil {
			t.Errorf("worker %s: empty scope path", w.ID)
		}
		if w.ID == co.RootID {
			if w.ReportsTo != "" {
				t.Errorf("root %s: reports to %q, want nothing", w.ID, w.ReportsTo)
			}
			continue
		}
		switch w.Role {
		case "vp":
			if w.ReportsTo != co.RootID {
				t.Errorf("vp %s: reports to %q, want %q", w.ID, w.ReportsTo, co.RootID)
			}
		case "lead":
			vp, ok := byID[w.ReportsTo]
			if !ok || vp.Role != "vp" {
				t.Errorf("lead %s: reports to %q, want a vp", w.ID, w.ReportsTo)
			}
		default:
			if !contributorRoles[w.Role] {
				t.Errorf("worker %s: unexpected role %q", w.ID, w.Role)
				continue
			}
			lead, ok := byID[w.ReportsTo]
			if !ok {
				t.Errorf("contributor %s: lead %q does not exist", w.ID, w.ReportsTo)
				continue
			}
			if lead.Role != "lead" {
				t.Errorf("contributor %s: reports to %s with role %q, want lead", w.ID, lead.ID, lead.Role)
			}
		}
	}

	if got, want := co.ScopeCount(), len(co.Teams)+len(co.Departments())+len(co.Divisions())+1; got != want {
		t.Errorf("ScopeCount: got %d, want %d", got, want)
	}
}

func TestOnlyServiceTeamsReportEngineering(t *testing.T) {
	forEachUniverse(t, func(t *testing.T, co *Company) {
		engineering := []string{"performance", "logs", "delivery", "incident"}
		for _, tm := range co.Teams {
			isService := tm.Archetype == ArchService || tm.Archetype == ArchData
			for _, def := range engineering {
				if tm.Produces(def) && !isService {
					t.Errorf("team %s (%s) produces %q but runs no software", tm.Name, tm.Archetype, def)
				}
			}
		}
	})
}

func TestEveryTeamHasAKnownArchetype(t *testing.T) {
	forEachUniverse(t, func(t *testing.T, co *Company) {
		for _, tm := range co.Teams {
			if tm.Reports == nil && archetypeReports[tm.Archetype] == nil {
				t.Errorf("team %s: unknown archetype %q", tm.Name, tm.Archetype)
			}
		}
	})
}

func TestEveryArchetypeIsUsedSomewhere(t *testing.T) {
	used := map[Archetype]bool{}
	for _, name := range UniverseNames() {
		co, err := LoadCompany(name)
		if err != nil {
			t.Fatalf("load universe %q: %v", name, err)
		}
		for _, tm := range co.Teams {
			used[tm.Archetype] = true
		}
	}
	for a := range archetypeReports {
		if !used[a] {
			t.Errorf("archetype %q is declared but no team in any universe has it", a)
		}
	}
}

func TestHeadcountMatchesUniverseScale(t *testing.T) {
	forEachUniverse(t, func(t *testing.T, co *Company) {
		if got := co.Headcount(); got < co.MinFTE || got > co.MaxFTE {
			t.Errorf("headcount: got %d, want %d..%d", got, co.MinFTE, co.MaxFTE)
		}
	})
}

func TestEveryUniverseHasServiceTeams(t *testing.T) {
	forEachUniverse(t, func(t *testing.T, co *Company) {
		if n := len(co.ServiceTeams()); n < 2 {
			t.Errorf("service/data teams: got %d, want at least 2", n)
		}
	})
}

func TestUniverseNameIsOneScopeSegment(t *testing.T) {
	forEachUniverse(t, func(t *testing.T, co *Company) {
		if co.Name == "" || strings.ContainsAny(co.Name, "/ ") || strings.ToLower(co.Name) != co.Name {
			t.Errorf("universe name %q: want one lowercase path segment", co.Name)
		}
		if d := reporting.ScopeDepth(co.Name); d != 1 {
			t.Errorf("universe name %q: scope depth %d, want 1", co.Name, d)
		}
		if co.RootID == "" || co.RootRole == "" {
			t.Errorf("universe %q: root id/role must be set", co.Name)
		}
	})
}
