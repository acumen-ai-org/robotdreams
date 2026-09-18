package simulation

import (
	"fmt"
	"sort"
	"strings"
)

type Scale int

const (
	ScaleTeam Scale = iota
	ScaleEnterprise
)

type UniverseDef struct {
	Name             string
	Title            string
	Tagline          string
	RootID           string
	RootRole         string
	Scale            Scale
	MinFTE, MaxFTE   int
	Teams            []Team
	ExtraTeamReports []string
	RootReports      []string
	Scorecard        ScorecardProfile
	Vocab            *Vocabulary
	Workflows        map[string][]WorkflowTemplate
}

type Company struct {
	UniverseDef
	divisions   []string
	departments []Department
	serviceIdx  []int
	headcount   int
}

var universes = map[string]UniverseDef{}

func registerUniverse(d UniverseDef) {
	if _, dup := universes[d.Name]; dup {
		panic("simulation: duplicate universe " + d.Name)
	}
	universes[d.Name] = d
}

func UniverseNames() []string {
	out := make([]string, 0, len(universes))
	for name := range universes {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func HasUniverse(name string) bool {
	_, ok := universes[name]
	return ok
}

func LoadCompany(name string) (*Company, error) {
	def, ok := universes[name]
	if !ok {
		return nil, fmt.Errorf("unknown universe %q; available: %s", name, strings.Join(UniverseNames(), ", "))
	}
	return newCompany(def), nil
}

func countCompactIDs(teams []Team) (teamIDs, departmentIDs map[string]int) {
	teamIDs = map[string]int{}
	departmentIDs = map[string]int{}
	seenDepartment := map[string]bool{}
	for _, t := range teams {
		teamIDs[compactTeamID(t)]++
		if key := t.Division + "/" + t.Department; !seenDepartment[key] {
			seenDepartment[key] = true
			departmentIDs[compactDepartmentID(t.Division, t.Department)]++
		}
	}
	return teamIDs, departmentIDs
}

func newCompany(def UniverseDef) *Company {
	co := &Company{UniverseDef: def}
	co.Teams = make([]Team, len(def.Teams))
	copy(co.Teams, def.Teams)

	teamIDCount, departmentIDCount := countCompactIDs(def.Teams)

	seenDiv := map[string]bool{}
	seenDep := map[string]bool{}
	for i := range co.Teams {
		t := &co.Teams[i]
		t.universe = def.Name
		t.extraReports = def.ExtraTeamReports
		compact := compactTeamID(*t)
		if teamIDCount[compact] > 1 {
			compact = t.Division + "-" + t.Department + "-" + t.Name
		}
		t.idBase = compact + "-"
		co.headcount += t.Headcount
		if t.Archetype == ArchService || t.Archetype == ArchData {
			co.serviceIdx = append(co.serviceIdx, i)
		}
		if !seenDiv[t.Division] {
			seenDiv[t.Division] = true
			co.divisions = append(co.divisions, t.Division)
		}
		key := t.Division + "/" + t.Department
		if !seenDep[key] {
			seenDep[key] = true
			compactDep := compactDepartmentID(t.Division, t.Department)
			if departmentIDCount[compactDep] > 1 {
				compactDep = t.Division + "-" + t.Department
			}
			co.departments = append(co.departments, Department{
				universe: def.Name,
				Division: t.Division,
				Name:     t.Department,
				leadID:   compactDep + "-lead",
			})
		}
	}
	return co
}

func (c *Company) RootDefinitions() []string {
	if len(c.RootReports) > 0 {
		return c.RootReports
	}
	return UniverseReports
}

func (c *Company) Divisions() []string { return c.divisions }

func (c *Company) Departments() []Department { return c.departments }

func (c *Company) DivisionScope(division string) string { return c.Name + "/" + division }

func (c *Company) DivisionVPID(division string) string { return division + "-vp" }

func (c *Company) TeamsIn(division string) []int {
	var out []int
	for i, t := range c.Teams {
		if t.Division == division {
			out = append(out, i)
		}
	}
	return out
}

func (c *Company) ServiceTeams() []int { return c.serviceIdx }

func (c *Company) Headcount() int { return c.headcount }

func (c *Company) ScopeCount() int {
	return len(c.Teams) + len(c.departments) + len(c.divisions) + 1
}

func (c *Company) Workers() []WorkerSpec {
	out := []WorkerSpec{{ID: c.RootID, Role: c.RootRole, ReportsTo: "", Scope: c.Name}}
	for _, d := range c.divisions {
		out = append(out, WorkerSpec{ID: c.DivisionVPID(d), Role: "vp", ReportsTo: c.RootID, Scope: c.DivisionScope(d)})
	}
	for _, d := range c.departments {
		out = append(out, WorkerSpec{ID: d.LeadID(), Role: "lead", ReportsTo: c.DivisionVPID(d.Division), Scope: d.Scope()})
	}
	leadOf := make(map[string]string, len(c.departments))
	for _, d := range c.departments {
		leadOf[d.Division+"/"+d.Name] = d.LeadID()
	}
	for _, t := range c.Teams {
		lead := leadOf[t.Division+"/"+t.Department]
		ids := t.WorkerIDs()
		for i, id := range ids {
			out = append(out, WorkerSpec{ID: id, Role: t.Roles[i], ReportsTo: lead, Scope: t.Scope()})
		}
	}
	return out
}

type ScorecardProfile struct {
	UnitLabels     [2]string
	Users          float64
	PayingUnits    float64
	RevenuePerUnit float64
	GrossMarginPct float64
	UserTarget     float64
	PayingTarget   float64
	RevenueShare   map[string]float64
}
