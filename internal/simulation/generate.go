package simulation

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"time"

	"github.com/acumen-ai-org/robotdreams/pkg/reporting"
)

type EventBatch struct {
	Definition string            `json:"definition"`
	Scope      string            `json:"scope"`
	Events     []reporting.Event `json:"events"`
}

type Submission struct {
	Producer string              `json:"producer"`
	Instance *reporting.Instance `json:"instance,omitempty"`
	Events   *EventBatch         `json:"events,omitempty"`
}

func (s Submission) At() time.Time {
	if s.Instance != nil {
		return s.Instance.ProducedAt
	}
	if s.Events != nil && len(s.Events.Events) > 0 {
		return s.Events.Events[0].T
	}
	return time.Time{}
}

type TeamBuilder func(g *Generator, teamIdx int, tm Team, t time.Time) Submission

type ScopeBuilder func(g *Generator, scope, producer string, t time.Time) Submission

var (
	teamBuilders  = map[string]TeamBuilder{}
	scopeBuilders = map[string]ScopeBuilder{}
	cadences      = map[string]time.Duration{}
)

func registerTeam(def string, every time.Duration, b TeamBuilder) {
	teamBuilders[def] = b
	cadences[def] = every
}

var lifecycleDefinitions = map[string]bool{"incident": true}

func registerScope(def string, every time.Duration, b ScopeBuilder) {
	scopeBuilders[def] = b
	cadences[def] = every
}

const (
	everyPulse   = 30 * time.Minute
	hourly       = time.Hour
	everyShift   = 4 * time.Hour
	everyDay     = 24 * time.Hour
	everyWeek    = 7 * 24 * time.Hour
	everyMonth   = 30 * 24 * time.Hour
	everyQuarter = 91 * 24 * time.Hour
)

func init() {
	registerTeam("performance", hourly, buildPerformance)
	registerTeam("logs", hourly, buildLogs)
	registerTeam("delivery", 3*time.Hour, buildDelivery)
	registerTeam("quality", 3*time.Hour, buildQuality)
	registerTeam("activity", everyPulse, buildTeamActivity)
	registerTeam("cost", everyShift, buildCost)

	registerScope("activity", everyPulse, buildScopeActivity)
	registerScope("roadmap", 12*time.Hour, buildRoadmap)
	registerScope("decisions", 2*time.Hour, buildDecisions)
}

type incident struct {
	teamIdx   int
	service   string
	cause     string
	summary   string
	opened    time.Time
	mitigated time.Time
	resolved  time.Time
}

type Generator struct {
	co    *Company
	rng   *rand.Rand
	llm   *FakeLLM
	now   time.Time
	start time.Time
	seed  int64

	incidents []incident

	warnTeam, critTeam int
}

func NewGenerator(co *Company, seed int64, now time.Time, history time.Duration) *Generator {
	now = now.UTC().Truncate(time.Second)
	svc := co.ServiceTeams()
	g := &Generator{
		co:       co,
		seed:     seed,
		rng:      rand.New(rand.NewSource(seed)),
		llm:      NewFakeLLM(seed+1, co.Vocab),
		now:      now,
		start:    now.Add(-history),
		warnTeam: svc[0],
		critTeam: svc[1],
	}
	g.incidents = g.makeIncidents()
	return g
}

func (g *Generator) between(lo, hi float64) float64 { return lo + g.rng.Float64()*(hi-lo) }

func (g *Generator) minutes(lo, hi float64) time.Duration {
	return time.Duration(g.between(lo, hi) * float64(time.Minute))
}

func (g *Generator) intBetween(lo, hi int) int {
	if hi <= lo {
		return lo
	}
	return lo + g.rng.Intn(hi-lo+1)
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

func pct(v float64) string { return fmt.Sprintf("%.0f%%", v) }

func money(v float64) string { return fmt.Sprintf("%.0f", v) }

const (
	consumerPeakHour    = 20.0
	consumerFloor       = 0.65
	consumerSwing       = 0.35
	consumerWeekendLift = 1.15
	officePeakHour      = 13.0
	officeFloor         = 0.04
	officeWeekendFactor = 0.12
	workingDayStartHour = 8
	workingDayEndHour   = 19
)

func localTime(t time.Time, tm Team) time.Time {
	return t.Add(time.Duration(regionOffsetHours[tm.Region] * float64(time.Hour)))
}

func isWeekend(t time.Time) bool {
	return t.Weekday() == time.Saturday || t.Weekday() == time.Sunday
}

func (g *Generator) season(t time.Time, tm Team, amp float64) float64 {
	local := localTime(t, tm)
	hour := float64(local.Hour()) + float64(local.Minute())/60
	weekend := isWeekend(local)

	var shape float64
	if consumerFacing(tm.Archetype) {
		shape = consumerFloor + consumerSwing*math.Cos(2*math.Pi*(hour-consumerPeakHour)/24)
		if weekend {
			shape *= consumerWeekendLift
		}
	} else {
		shape = math.Max(officeFloor, math.Cos(2*math.Pi*(hour-officePeakHour)/24)*0.5+0.5)
		shape = shape * shape * shape
		if weekend {
			shape *= officeWeekendFactor
		}
	}
	return 1 - amp + amp*2*shape
}

func consumerFacing(a Archetype) bool {
	switch a {
	case ArchService, ArchData, ArchSupport:
		return true
	}
	return false
}

func businessHour(t time.Time, tm Team) bool {
	local := localTime(t, tm)
	if isWeekend(local) {
		return false
	}
	return local.Hour() >= workingDayStartHour && local.Hour() < workingDayEndHour
}

const (
	minIncidentsPerDivision = 2
	maxIncidentsPerDivision = 5
	incidentOpenMargin      = 30 * time.Minute
	forcedOpenIncidentAge   = 25 * time.Minute
)

func (g *Generator) makeIncidents() []incident {
	var out []incident
	forced := false
	for _, division := range g.co.Divisions() {
		var idxs []int
		for _, i := range g.co.TeamsIn(division) {
			if g.co.Teams[i].Archetype == ArchService || g.co.Teams[i].Archetype == ArchData {
				idxs = append(idxs, i)
			}
		}
		if len(idxs) == 0 {
			continue
		}
		n := minIncidentsPerDivision + g.rng.Intn(maxIncidentsPerDivision-minIncidentsPerDivision+1)
		for i := 0; i < n; i++ {
			ti := idxs[g.rng.Intn(len(idxs))]
			inc := incident{
				teamIdx: ti,
				service: g.llm.Service(g.co.Teams[ti].Archetype),
				cause:   g.llm.Cause(g.co.Teams[ti].Archetype),
			}
			inc.summary = g.llm.IncidentSummary(inc.service, inc.cause)
			window := g.now.Sub(g.start) - incidentOpenMargin
			if window < time.Minute {
				window = time.Minute
			}
			inc.opened = g.start.Add(time.Duration(g.rng.Int63n(int64(window))))
			inc.mitigated = inc.opened.Add(g.minutes(10, 40))
			inc.resolved = inc.mitigated.Add(g.minutes(20, 80))
			if inc.mitigated.After(g.now) {
				inc.mitigated = time.Time{}
			}
			if inc.resolved.After(g.now) {
				inc.resolved = time.Time{}
			}
			if !forced && i == n-1 {
				forced = true
				for _, cand := range idxs {
					if cand != g.warnTeam && cand != g.critTeam {
						inc.teamIdx = cand
						break
					}
				}
				inc.opened = g.now.Add(-forcedOpenIncidentAge)
				inc.mitigated = time.Time{}
				inc.resolved = time.Time{}
			}
			out = append(out, inc)
		}
	}
	return out
}

func (g *Generator) incidentAt(ti int, t time.Time) *incident {
	for i := range g.incidents {
		inc := &g.incidents[i]
		if inc.teamIdx != ti {
			continue
		}
		end := inc.resolved
		if end.IsZero() {
			end = g.now
		}
		if !t.Before(inc.opened) && !t.After(end) {
			return inc
		}
	}
	return nil
}

func (g *Generator) History() []Submission {
	var subs []Submission

	for ti, tm := range g.co.Teams {
		for _, def := range tm.ReportDefinitions() {
			build, ok := teamBuilders[def]
			if !ok {
				continue
			}
			for _, t := range g.ticks(def, tm, true) {
				subs = append(subs, build(g, ti, tm, t))
			}
		}
	}

	for _, d := range g.co.Departments() {
		for _, def := range DepartmentReports {
			if build, ok := scopeBuilders[def]; ok {
				for _, t := range g.ticks(def, Team{}, false) {
					subs = append(subs, build(g, d.Scope(), d.LeadID(), t))
				}
			}
		}
	}
	for _, division := range g.co.Divisions() {
		for _, def := range DivisionReports {
			if build, ok := scopeBuilders[def]; ok {
				for _, t := range g.ticks(def, Team{}, false) {
					subs = append(subs, build(g, g.co.DivisionScope(division), g.co.DivisionVPID(division), t))
				}
			}
		}
	}
	for _, def := range g.co.RootDefinitions() {
		if build, ok := scopeBuilders[def]; ok {
			for _, t := range g.ticks(def, Team{}, false) {
				subs = append(subs, build(g, g.co.Name, g.co.RootID, t))
			}
		}
	}

	for i := range g.incidents {
		subs = append(subs, g.incidentLifecycle(&g.incidents[i])...)
	}

	sort.SliceStable(subs, func(i, j int) bool { return subs[i].At().Before(subs[j].At()) })
	return subs
}

const maxWorkingHourWalkback = 96

func (g *Generator) ticks(def string, tm Team, teamLevel bool) []time.Time {
	every, ok := cadences[def]
	if !ok || every <= 0 {
		every = hourly
	}

	var out []time.Time
	if every < everyDay {
		for t := g.start; t.Before(g.now); t = t.Add(every) {
			out = append(out, t)
		}
		return out
	}

	for t := g.now.Add(-time.Minute); t.After(g.start); t = t.Add(-every) {
		at := t
		if teamLevel && !businessHour(at, tm) {
			for i := 0; i < maxWorkingHourWalkback && !businessHour(at, tm); i++ {
				at = at.Add(-time.Hour)
			}
			if !businessHour(at, tm) {
				continue
			}
		}
		if at.Before(g.start) {
			break
		}
		if len(out) > 0 && out[len(out)-1].Equal(at) {
			continue
		}
		out = append(out, at)
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	if len(out) == 0 {
		out = append(out, g.now.Add(-time.Minute))
	}
	return out
}

func (g *Generator) series(t time.Time, n int, step time.Duration, base, jitter float64) []reporting.SeriesPoint {
	pts := make([]reporting.SeriesPoint, n)
	for i := 0; i < n; i++ {
		pt := t.Add(-time.Duration(n-1-i) * step)
		pts[i] = reporting.SeriesPoint{T: pt, V: round1(base * (1 + g.between(-jitter, jitter)))}
	}
	return pts
}

func bridge(openLabel, closeLabel string, closing float64, moves [][2]interface{}) reporting.Table {
	rows := make([][]string, 0, len(moves)+2)
	sum := 0.0
	for _, m := range moves {
		sum += m[1].(float64)
	}
	rows = append(rows, []string{openLabel, money(closing - sum), "total"})
	for _, m := range moves {
		rows = append(rows, []string{m[0].(string), money(m[1].(float64)), "delta"})
	}
	rows = append(rows, []string{closeLabel, money(closing), "total"})
	return reporting.Table{Columns: []string{"step", "amount", "kind"}, Rows: rows}
}

func funnel(stages [][2]interface{}) reporting.Table {
	rows := make([][]string, 0, len(stages))
	for _, s := range stages {
		rows = append(rows, []string{s[0].(string), fmt.Sprintf("%.0f", s[1].(float64))})
	}
	return reporting.Table{Columns: []string{"stage", "count"}, Rows: rows}
}

func byUnit(valueColumn string, tm Team, value float64) reporting.Table {
	return reporting.Table{
		Columns: []string{"unit", valueColumn},
		Rows:    [][]string{{tm.Name, money(value)}},
	}
}

func (tm Team) instance(def string, t time.Time, kpis, targets map[string]float64) *reporting.Instance {
	return &reporting.Instance{
		Definition: def,
		Scope:      tm.Scope(),
		Producer:   tm.ProducerID(),
		ProducedAt: t,
		KPIs:       kpis,
		Targets:    targets,
	}
}

func (tm Team) submit(in *reporting.Instance) Submission {
	return Submission{Producer: tm.ProducerID(), Instance: in}
}
