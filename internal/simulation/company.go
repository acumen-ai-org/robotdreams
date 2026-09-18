package simulation

import (
	"fmt"
	"strings"
)

const CEOID = "ceo"

type Archetype string

const (
	ArchService    Archetype = "service"
	ArchData       Archetype = "data"
	ArchResearch   Archetype = "research"
	ArchSecurity   Archetype = "security"
	ArchFinance    Archetype = "finance"
	ArchAccounting Archetype = "accounting"
	ArchRevenue    Archetype = "revenue"
	ArchSales      Archetype = "sales"
	ArchPeople     Archetype = "people"
	ArchPeopleOps  Archetype = "people-ops"
	ArchLegal      Archetype = "legal"
	ArchCompliance Archetype = "compliance"
	ArchMarketing  Archetype = "marketing"
	ArchComms      Archetype = "comms"
	ArchSupport    Archetype = "support"
	ArchContent    Archetype = "content"
	ArchLicensing  Archetype = "licensing"
	ArchIT         Archetype = "it"
	ArchStrategy   Archetype = "strategy"
)

var universalReports = []string{"activity", "cost", "workforce"}

var archetypeReports = map[Archetype][]string{
	ArchService:    {"performance", "logs", "delivery", "incident", "quality", "task-board", "backlog"},
	ArchData:       {"data-pipeline", "performance", "incident", "task-board", "backlog"},
	ArchResearch:   {"ml-experiment", "quality", "task-board"},
	ArchSecurity:   {"security-posture", "task-board", "backlog"},
	ArchFinance:    {"budget-variance"},
	ArchAccounting: {"financial-close", "budget-variance"},
	ArchRevenue:    {"subscriber-growth"},
	ArchSales:      {"sales-pipeline"},
	ArchPeople:     {"hiring", "backlog"},
	ArchPeopleOps:  {"todo"},
	ArchLegal:      {"legal-matters", "backlog"},
	ArchCompliance: {"compliance", "legal-matters", "task-board", "backlog"},
	ArchMarketing:  {"campaign", "task-board"},
	ArchComms:      {"brand-reach", "todo"},
	ArchSupport:    {"support-queue", "task-board"},
	ArchContent:    {"editorial", "catalog", "task-board"},
	ArchLicensing:  {"licensing", "catalog", "backlog"},
	ArchIT:         {"it-service", "task-board"},
	ArchStrategy:   {"portfolio"},
}

var DepartmentReports = []string{"activity", "roadmap", "decisions", "plan"}

var DivisionReports = []string{"portfolio", "plan", "strategy"}

var UniverseReports = []string{"company-scorecard", "strategy"}

type Region string

const (
	RegionNordics Region = "nordics"
	RegionEMEA    Region = "emea"
	RegionNorthAm Region = "northam"
	RegionLatAm   Region = "latam"
	RegionAPAC    Region = "apac"
)

var regionOffsetHours = map[Region]float64{
	RegionNordics: 2,
	RegionEMEA:    1,
	RegionNorthAm: -4,
	RegionLatAm:   -3,
	RegionAPAC:    8,
}

type Team struct {
	Division   string
	Department string
	Name       string
	Archetype  Archetype
	Region     Region
	Roles      []string
	Headcount  int
	Reports    []string
	Variant    string

	universe     string
	extraReports []string
	idBase       string
}

var (
	rolesService  = []string{"builder", "builder", "reviewer", "ops"}
	rolesSmallSvc = []string{"builder", "builder", "ops"}
	rolesResearch = []string{"researcher", "researcher", "reviewer"}
	rolesData     = []string{"ops", "librarian", "builder"}
	rolesSecurity = []string{"guardian", "guardian", "reviewer"}
	rolesAnalyst  = []string{"researcher", "reviewer"}
	rolesControl  = []string{"reviewer", "reviewer", "guardian"}
	rolesCounsel  = []string{"guardian", "reviewer"}
	rolesOutreach = []string{"messenger", "messenger", "writer"}
	rolesCreative = []string{"writer", "writer", "reviewer"}
	rolesCare     = []string{"messenger", "messenger", "messenger", "reviewer"}
	rolesCurate   = []string{"writer", "librarian"}
	rolesRights   = []string{"librarian", "guardian"}
	rolesITOps    = []string{"ops", "ops", "messenger"}
	rolesStrategy = []string{"orchestrator", "researcher"}
)

func (t Team) ReportDefinitions() []string {
	base := t.Reports
	if base == nil {
		base = archetypeReports[t.Archetype]
	}
	return uniqueInOrder(base, t.extraReports, universalReports)
}

func uniqueInOrder(lists ...[]string) []string {
	n := 0
	for _, l := range lists {
		n += len(l)
	}
	out := make([]string, 0, n)
	seen := make(map[string]bool, n)
	for _, l := range lists {
		for _, name := range l {
			if seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

func (t Team) Produces(def string) bool {
	for _, d := range t.ReportDefinitions() {
		if d == def {
			return true
		}
	}
	return false
}

func (t Team) Scope() string {
	return t.universe + "/" + t.Division + "/" + t.Department + "/" + t.Name
}

func (t Team) DepartmentScope() string {
	return t.universe + "/" + t.Division + "/" + t.Department
}

func (t Team) DivisionScope() string { return t.universe + "/" + t.Division }

func compactName(name string) string {
	name = strings.TrimSuffix(name, "-squad")
	if i := strings.Index(name, "-"); i > 0 {
		return name[:i]
	}
	return name
}

func compactTeamID(t Team) string {
	return t.Division + "-" + compactName(t.Department) + "-" + compactName(t.Name)
}

func compactDepartmentID(division, department string) string {
	return division + "-" + compactName(department)
}

func twoDigits(n int) string {
	return fmt.Sprintf("%02d", n)
}

func (t Team) WorkerIDs() []string {
	base := t.idBase
	if base == "" {
		base = compactTeamID(t) + "-"
	}
	out := make([]string, len(t.Roles))
	for i := range t.Roles {
		out[i] = base + twoDigits(i+1)
	}
	return out
}

func (t Team) ProducerID() string { return t.WorkerIDs()[0] }

type Department struct {
	Division string
	Name     string

	universe string
	leadID   string
}

func (d Department) Scope() string { return d.universe + "/" + d.Division + "/" + d.Name }

func (d Department) LeadID() string {
	if d.leadID != "" {
		return d.leadID
	}
	return compactDepartmentID(d.Division, d.Name) + "-lead"
}

type WorkerSpec struct {
	ID        string
	Role      string
	ReportsTo string
	Scope     string
}
