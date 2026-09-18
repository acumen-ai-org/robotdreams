package simulation

import (
	"bytes"
	"encoding/json"
	"sort"
	"testing"
	"time"

	"github.com/acumen-ai-org/robotdreams/pkg/reporting"
)

func mustCompany(t *testing.T, name string) *Company {
	t.Helper()
	co, err := LoadCompany(name)
	if err != nil {
		t.Fatalf("load universe %q: %v", name, err)
	}
	return co
}

var fixedNow = time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)

func marshalSubs(t *testing.T, subs []Submission) []byte {
	t.Helper()
	b, err := json.Marshal(subs)
	if err != nil {
		t.Fatalf("marshal submissions: %v", err)
	}
	return b
}

func TestGeneratorDeterminism(t *testing.T) {
	a := marshalSubs(t, NewGenerator(mustCompany(t, "spookify"), 1, fixedNow, 6*time.Hour).History())
	b := marshalSubs(t, NewGenerator(mustCompany(t, "spookify"), 1, fixedNow, 6*time.Hour).History())
	if !bytes.Equal(a, b) {
		t.Fatal("same seed produced different histories")
	}

	c := marshalSubs(t, NewGenerator(mustCompany(t, "spookify"), 2, fixedNow, 6*time.Hour).History())
	if bytes.Equal(a, c) {
		t.Fatal("different seeds produced identical histories")
	}
}

func TestGeneratorContractValidity(t *testing.T) {
	defs, err := reporting.LoadLibraryDir("../../reporting/library")
	if err != nil {
		t.Fatalf("load library: %v", err)
	}
	reg := reporting.NewRegistry(defs)

	co := mustCompany(t, "spookify")
	subs := NewGenerator(co, 7, fixedNow, 8*time.Hour).History()
	if len(subs) == 0 {
		t.Fatal("generator produced no submissions")
	}

	seenDefs := map[string]bool{}
	for i, sub := range subs {
		switch {
		case sub.Instance != nil:
			inst := sub.Instance
			def, ok := reg.Get(inst.Definition)
			if !ok {
				t.Fatalf("subs[%d]: unknown definition %q", i, inst.Definition)
			}
			if err := reporting.ValidateInstance(def, inst); err != nil {
				t.Errorf("subs[%d] (%s at %s): %v", i, inst.Definition, inst.Scope, err)
			}
			seenDefs[inst.Definition] = true
		case sub.Events != nil:
			def, ok := reg.Get(sub.Events.Definition)
			if !ok {
				t.Fatalf("subs[%d]: unknown definition %q", i, sub.Events.Definition)
			}
			declared := map[string]bool{}
			for _, e := range def.Data.Events {
				declared[e.Type] = true
			}
			for j, e := range sub.Events.Events {
				if !declared[e.Type] {
					t.Errorf("subs[%d].events[%d]: type %q not in %s's contract", i, j, e.Type, def.Name)
				}
			}
		default:
			t.Errorf("subs[%d]: neither instance nor events", i)
		}
		if sub.Producer == "" {
			t.Errorf("subs[%d]: empty producer", i)
		}
	}

	for _, name := range expectedDefinitions(co) {
		if !seenDefs[name] {
			t.Errorf("history contains no %q instances", name)
		}
	}

	g := NewGenerator(mustCompany(t, "spookify"), 7, fixedNow, 8*time.Hour)
	for _, def := range []string{"activity", "logs", "delivery", "performance"} {
		batch := g.pulseBatch(def, g.co.Teams[g.co.ServiceTeams()[0]], fixedNow)
		d, _ := reg.Get(def)
		declared := map[string]bool{}
		for _, e := range d.Data.Events {
			declared[e.Type] = true
		}
		for j, e := range batch.Events {
			if !declared[e.Type] {
				t.Errorf("pulseBatch(%s).events[%d]: type %q not declared", def, j, e.Type)
			}
		}
	}
	inc := g.newLiveIncident(fixedNow)
	for _, stage := range []string{"opened", "mitigated", "resolved"} {
		sub := g.incidentInstance(inc, stage, fixedNow)
		d, _ := reg.Get("incident")
		if err := reporting.ValidateInstance(d, sub.Instance); err != nil {
			t.Errorf("live incident %s: %v", stage, err)
		}
	}
}

func TestGeneratorChronologicalOrder(t *testing.T) {
	subs := NewGenerator(mustCompany(t, "spookify"), 1, fixedNow, 4*time.Hour).History()
	for i := 1; i < len(subs); i++ {
		if subs[i].At().Before(subs[i-1].At()) {
			t.Fatalf("subs[%d] (%s) is before subs[%d] (%s)", i, subs[i].At(), i-1, subs[i-1].At())
		}
	}
}

func TestGeneratorMixedStatuses(t *testing.T) {
	subs := NewGenerator(mustCompany(t, "spookify"), 1, fixedNow, 6*time.Hour).History()

	latestLogs := map[string]*reporting.Instance{}
	openIncidents := 0.0
	latestIncident := map[string]*reporting.Instance{}
	for _, sub := range subs {
		if sub.Instance == nil {
			continue
		}
		switch sub.Instance.Definition {
		case "logs":
			latestLogs[sub.Instance.Scope] = sub.Instance
		case "incident":
			latestIncident[sub.Instance.Scope] = sub.Instance
		}
	}
	for _, inst := range latestIncident {
		openIncidents += inst.KPIs["open_incidents"]
	}

	g := NewGenerator(mustCompany(t, "spookify"), 1, fixedNow, 6*time.Hour)
	warn := latestLogs[g.co.Teams[g.warnTeam].Scope()]
	crit := latestLogs[g.co.Teams[g.critTeam].Scope()]
	if warn == nil || crit == nil {
		t.Fatal("missing final logs instances for the designated teams")
	}
	if v := warn.KPIs["error_lines"]; v < 1 || v >= 25 {
		t.Errorf("warn team error_lines = %v, want in [1, 25)", v)
	}
	if v := crit.KPIs["error_lines"]; v < 25 {
		t.Errorf("critical team error_lines = %v, want >= 25", v)
	}
	if openIncidents < 1 {
		t.Error("no incident is open at boot; want at least one")
	}
}

func expectedDefinitions(co *Company) []string {
	seen := map[string]bool{}
	var out []string
	add := func(names ...string) {
		for _, n := range names {
			if !seen[n] {
				seen[n] = true
				out = append(out, n)
			}
		}
	}
	for _, tm := range co.Teams {
		add(tm.ReportDefinitions()...)
	}
	add(DepartmentReports...)
	add(DivisionReports...)
	add(UniverseReports...)
	sort.Strings(out)
	return out
}

func TestBuildersMatchLibrary(t *testing.T) {
	defs, err := reporting.LoadLibraryDir("../../reporting/library")
	if err != nil {
		t.Fatalf("load library: %v", err)
	}
	inLibrary := map[string]bool{}
	for _, d := range defs {
		inLibrary[d.Name] = true
	}

	produced := map[string]bool{}
	for _, universe := range UniverseNames() {
		for _, def := range expectedDefinitions(mustCompany(t, universe)) {
			produced[def] = true
		}
	}
	names := make([]string, 0, len(produced))
	for name := range produced {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		if !inLibrary[name] {
			t.Errorf("definition %q is produced by the company but has no YAML in reporting/library", name)
		}
		if lifecycleDefinitions[name] {
			continue
		}
		_, team := teamBuilders[name]
		_, scope := scopeBuilders[name]
		if !team && !scope {
			t.Errorf("definition %q is produced by the company but has no generator builder", name)
		}
		if _, ok := cadences[name]; !ok {
			t.Errorf("definition %q has no registered cadence", name)
		}
	}

	for name := range teamBuilders {
		if !produced[name] {
			t.Errorf("team builder %q is registered but no team produces it", name)
		}
	}
	for name := range scopeBuilders {
		if !produced[name] {
			t.Errorf("scope builder %q is registered but no scope produces it", name)
		}
	}
}

func TestTargetsAreDeclared(t *testing.T) {
	defs, err := reporting.LoadLibraryDir("../../reporting/library")
	if err != nil {
		t.Fatalf("load library: %v", err)
	}
	reg := reporting.NewRegistry(defs)

	withTargets := 0
	for _, sub := range NewGenerator(mustCompany(t, "spookify"), 3, fixedNow, 8*time.Hour).History() {
		if sub.Instance == nil || len(sub.Instance.Targets) == 0 {
			continue
		}
		withTargets++
		def, ok := reg.Get(sub.Instance.Definition)
		if !ok {
			continue
		}
		declared := map[string]bool{}
		for _, k := range def.Data.KPIs {
			declared[k.Name] = true
		}
		for name := range sub.Instance.Targets {
			if !declared[name] {
				t.Errorf("%s: target %q is not a declared KPI", def.Name, name)
			}
		}
	}
	if withTargets == 0 {
		t.Error("no generated instance carries a target; nothing can answer \"are we on target?\"")
	}
}

func TestBusinessTeamsProduceBusinessReports(t *testing.T) {
	co := mustCompany(t, "spookify")
	byScope := map[string]map[string]bool{}
	for _, sub := range NewGenerator(co, 1, fixedNow, 48*time.Hour).History() {
		if sub.Instance == nil {
			continue
		}
		if byScope[sub.Instance.Scope] == nil {
			byScope[sub.Instance.Scope] = map[string]bool{}
		}
		byScope[sub.Instance.Scope][sub.Instance.Definition] = true
	}

	engineering := []string{"performance", "logs", "delivery", "incident"}
	for _, tm := range co.Teams {
		if tm.Archetype == ArchService || tm.Archetype == ArchData {
			continue
		}
		got := byScope[tm.Scope()]
		if len(got) == 0 {
			t.Errorf("team %s produced no instances at all", tm.Scope())
			continue
		}
		for _, def := range engineering {
			if got[def] {
				t.Errorf("team %s (%s) produced a %q report but runs no software", tm.Name, tm.Archetype, def)
			}
		}
	}
}
