package simulation

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/acumen-ai-org/robotdreams/pkg/reporting"
)

func init() {
	registerTeam("financial-close", everyMonth, buildFinancialClose)
	registerTeam("budget-variance", everyMonth, buildBudgetVariance)
	registerTeam("portfolio", everyWeek, buildTeamPortfolio)
	registerScope("portfolio", everyWeek, buildPortfolio)
	registerScope("company-scorecard", everyMonth, buildCompanyScorecard)
}

var financeCloseTasks = []string{
	"bank reconciliations", "royalty accrual", "revenue cut-off testing",
	"payroll journal", "intercompany elimination", "deferred revenue roll-forward",
	"FX revaluation", "accrued marketing spend", "fixed asset depreciation",
	"lease schedule update", "VAT return preparation", "flux analysis commentary",
}

var financeLedgerAccounts = []string{
	"salaries and benefits", "cloud infrastructure", "content royalties",
	"marketing and promotion", "contractors and agencies", "software licences",
	"travel and events", "facilities and workplace",
}

var financeCostCentres = []string{
	"engineering", "content and licensing", "sales and marketing",
	"customer operations", "corporate functions", "infrastructure",
}

var financeInitiatives = []string{
	"audiobooks in five new markets", "in-app purchase migration",
	"AI host international rollout", "podcast ad marketplace",
	"family plan repricing", "carrier bundle partnerships",
	"video podcasts expansion", "creator monetisation tooling",
	"catalogue metadata overhaul", "cost-to-serve programme",
	"self-serve advertising in APAC", "lossless tier launch",
}

var financeVarianceDrivers = []string{
	"headcount", "cloud", "royalties", "marketing", "contractors", "FX",
}

const (
	closeHeadcountPerLedgerUnit = 25
	closeBenchmarkDaysLo        = 5
	closeBenchmarkDaysHi        = 8.4
	closeDaysTarget             = 6

	forecastAccuracyLo = 92
	forecastAccuracyHi = 97
	varianceSpreadBase = 0.035
	varianceSpreadHigh = 0.055
	varianceSpreadLow  = 0.02

	headcountPerInitiative = 25
	okrAttainmentTarget    = 70

	scorecardRounding     = 1e5
	smallUniverseUsers    = 10_000
	adSupportedShareLo    = 0.10
	adSupportedShareHi    = 0.13
	scorecardMarginTarget = 31.5
	scorecardSubsTarget   = 276e6
)

func roundCents(v float64) float64 { return math.Round(v*100) / 100 }

func (g *Generator) divisionHeadcount(division string) int {
	total := 0
	for _, tm := range g.co.Teams {
		if tm.Division == division {
			total += tm.Headcount
		}
	}
	return total
}

func scopeLeaf(scope string) string {
	if i := strings.LastIndex(scope, "/"); i >= 0 {
		return scope[i+1:]
	}
	return scope
}

func buildFinancialClose(g *Generator, ti int, tm Team, t time.Time) Submission {
	scale := float64(tm.Headcount) / closeHeadcountPerLedgerUnit

	daysToClose := round1(g.between(closeBenchmarkDaysLo, closeBenchmarkDaysHi))
	closeDay := float64(g.intBetween(1, int(daysToClose)))

	completePct := round1(closeDay / daysToClose * 100 * g.between(0.85, 1.05))
	if completePct > 100 {
		completePct = 100
	}
	unreconciled := math.Round(g.between(2, 26) * scale * (1.05 - completePct/100))
	adjusting := math.Round(g.between(3, 15) * scale)

	tasks := float64(60 + int(math.Round(g.between(20, 60)*scale)))
	reconciled := math.Round(tasks * g.between(0.72, 0.95))
	reviewed := math.Round(reconciled * g.between(0.70, 0.95))
	signedOff := math.Round(reviewed * g.between(0.60, 0.95))

	started := t.Add(-time.Duration(closeDay*24) * time.Hour)
	events := []reporting.Event{{
		T: started, Type: "close.started", Severity: reporting.SeverityInfo,
		Label: "close opened for the period",
	}}
	for i := 0; i < 1+g.rng.Intn(2); i++ {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(20, 400)), Type: "subledger.closed", Severity: reporting.SeverityInfo,
			Label: "closed " + g.llm.Service(ArchAccounting),
		})
	}
	if unreconciled > 6*scale {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(30, 600)), Type: "close.blocked", Severity: reporting.SeverityWarn,
			Label: "close blocked on " + g.llm.Cause(ArchAccounting),
		})
	}
	if adjusting > 10*scale {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(10, 300)), Type: "entry.adjusted", Severity: reporting.SeverityWarn,
			Label: fmt.Sprintf("%d adjusting entries posted after cut-off", int(adjusting)),
		})
	}
	if completePct >= 99 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(5, 90)), Type: "close.signed_off", Severity: reporting.SeverityInfo,
			Label: "close signed off by " + g.llm.Owner(),
		})
	}

	var openRows [][]string
	shown := int(math.Min(unreconciled, 5))
	for i := 0; i < shown; i++ {
		openRows = append(openRows, []string{
			financeLedgerAccounts[g.rng.Intn(len(financeLedgerAccounts))],
			g.llm.Owner(),
			fmt.Sprintf("%d", g.intBetween(1, int(closeDay)+2)),
			money(g.between(12_000, 900_000)),
			g.llm.Cause(ArchAccounting),
		})
	}

	var checkRows [][]string
	for i := 0; i < 6; i++ {
		task := financeCloseTasks[(ti+i*2)%len(financeCloseTasks)]
		dueDay := i + 1
		status := "done"
		if float64(dueDay) > closeDay {
			status = "not started"
		} else if float64(dueDay) == closeDay {
			status = "in progress"
		}
		checkRows = append(checkRows, []string{task, g.llm.Owner(), fmt.Sprintf("day %d", dueDay), status})
	}

	progress := make([]reporting.SeriesPoint, 0, 8)
	for i := 1; i <= 8; i++ {
		progress = append(progress, reporting.SeriesPoint{
			T: t.Add(-time.Duration(8-i) * 12 * time.Hour),
			V: round1(completePct * float64(i) / 8 * g.between(0.92, 1.02)),
		})
	}

	in := tm.instance("financial-close", t,
		map[string]float64{
			"close_day":             closeDay,
			"tasks_complete_pct":    completePct,
			"unreconciled_accounts": unreconciled,
			"adjusting_entries":     adjusting,
			"days_to_close":         daysToClose,
		},
		map[string]float64{
			"days_to_close":         closeDaysTarget,
			"tasks_complete_pct":    100,
			"unreconciled_accounts": 0,
		})
	in.Series = map[string][]reporting.SeriesPoint{"close_progress": progress}
	in.Events = events
	in.Tables = map[string]reporting.Table{
		"close_stages": funnel([][2]interface{}{
			{"sub-ledgers closed", tasks},
			{"reconciled", reconciled},
			{"reviewed", reviewed},
			{"signed off", signedOff},
		}),
		"open_items":         {Columns: []string{"account", "owner", "days_open", "amount_eur", "blocker"}, Rows: openRows},
		"close_checklist":    {Columns: []string{"task", "owner", "due_day", "status"}, Rows: checkRows},
		"close_days_by_unit": byUnit("days_to_close", tm, daysToClose),
	}
	return tm.submit(in)
}

func varianceDriverSpread(driver string) float64 {
	switch driver {
	case "headcount", "royalties":
		return varianceSpreadHigh
	case "FX", "contractors":
		return varianceSpreadLow
	}
	return varianceSpreadBase
}

func buildBudgetVariance(g *Generator, ti int, tm Team, t time.Time) Submission {
	plan := math.Round(float64(tm.Headcount)*g.between(11_500, 16_500)/100) * 100

	moves := make([][2]interface{}, 0, len(financeVarianceDrivers))
	delta := 0.0
	for _, driver := range financeVarianceDrivers {
		spread := varianceDriverSpread(driver)
		amount := math.Round(plan*g.between(-spread*0.7, spread)/100) * 100
		moves = append(moves, [2]interface{}{driver, amount})
		delta += amount
	}
	actual := plan + delta
	variancePct := round1(delta / plan * 100)
	forecast := math.Round(actual*g.between(0.99, 1.06)/100) * 100
	accuracy := round1(g.between(forecastAccuracyLo, forecastAccuracyHi))

	raw := make([]float64, len(financeCostCentres))
	rawSum := 0.0
	for i := range financeCostCentres {
		raw[i] = math.Round(plan*g.between(-0.02, 0.02)/100) * 100
		rawSum += raw[i]
	}
	residual := (delta - rawSum) / float64(len(financeCostCentres))
	centreRows := make([][]string, 0, len(financeCostCentres))
	for i, centre := range financeCostCentres {
		centreRows = append(centreRows, []string{centre, money(raw[i] + residual)})
	}

	planW := make([]float64, len(financeLedgerAccounts))
	actualW := make([]float64, len(financeLedgerAccounts))
	planSum, actualSum := 0.0, 0.0
	for i := range financeLedgerAccounts {
		planW[i] = g.between(0.3, 1.8)
		actualW[i] = planW[i] * g.between(0.88, 1.15)
		planSum += planW[i]
		actualSum += actualW[i]
	}
	lineRows := make([][]string, 0, len(financeLedgerAccounts))
	var overrunRows [][]string
	for i, account := range financeLedgerAccounts {
		lPlan := math.Round(plan * planW[i] / planSum)
		lActual := math.Round(actual * actualW[i] / actualSum)
		lVar := lActual - lPlan
		owner := g.llm.Owner()
		lineRows = append(lineRows, []string{
			account, money(lPlan), money(lActual), money(lVar),
			pct(lVar / math.Max(lPlan, 1) * 100), owner,
		})
		if lVar > 0 && len(overrunRows) < 4 {
			overrunRows = append(overrunRows, []string{
				account, owner, money(lPlan), money(lActual), money(lVar),
			})
		}
	}

	var events []reporting.Event
	if g.rng.Float64() < 0.45 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(60, 900)), Type: "forecast.revised", Severity: reporting.SeverityInfo,
			Label: "reforecast landed on " + g.llm.Service(ArchFinance),
		})
	}
	if g.rng.Float64() < 0.6 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(30, 700)), Type: "variance.explained", Severity: reporting.SeverityInfo,
			Label: "variance attributed to " + g.llm.Cause(ArchFinance),
		})
	}
	if g.rng.Float64() < 0.25 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(20, 600)), Type: "budget.reallocated", Severity: reporting.SeverityInfo,
			Label: "budget moved between cost centres after " + g.llm.Cause(ArchFinance),
		})
	}
	if variancePct > 5 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(10, 400)), Type: "budget.overrun", Severity: reporting.SeverityWarn,
			Label: fmt.Sprintf("%s is %.1f%% over plan", tm.Name, variancePct),
		})
	}
	if variancePct > 12 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(5, 120)), Type: "budget.breached", Severity: reporting.SeverityCritical,
			Label: "cost centre breached its annual envelope",
		})
	}

	in := tm.instance("budget-variance", t,
		map[string]float64{
			"plan_eur":              plan,
			"actual_eur":            actual,
			"forecast_eur":          forecast,
			"variance_pct":          variancePct,
			"forecast_accuracy_pct": accuracy,
		},
		map[string]float64{"variance_pct": 5, "forecast_accuracy_pct": 95})
	in.Series = map[string][]reporting.SeriesPoint{
		"burn_rate": g.series(t, 10, 3*24*time.Hour, actual/30, 0.18),
	}
	in.Events = events
	in.Tables = map[string]reporting.Table{
		"variance_bridge":         bridge("plan", "actual", actual, moves),
		"variance_by_cost_centre": {Columns: []string{"cost_centre", "variance_eur"}, Rows: centreRows},
		"overruns":                {Columns: []string{"account", "owner", "plan_eur", "actual_eur", "overrun_eur"}, Rows: overrunRows},
		"line_items":              {Columns: []string{"account", "plan_eur", "actual_eur", "variance_eur", "variance_pct", "owner"}, Rows: lineRows},
		"actual_by_unit":          byUnit("actual_eur", tm, actual),
	}
	return tm.submit(in)
}

type portfolioFigures struct {
	kpis    map[string]float64
	trend   []reporting.SeriesPoint
	events  []reporting.Event
	tables  map[string]reporting.Table
	targets map[string]float64
	items   []reporting.Item
}

func (g *Generator) portfolioNumbers(unit string, headcount int, t time.Time) portfolioFigures {
	active := float64(g.intBetween(4, 6+headcount/headcountPerInitiative))
	atRisk := float64(g.intBetween(0, int(active/3)+1))
	attainment := round1(g.between(52, 88))
	investment := math.Round(float64(headcount)*g.between(30_000, 55_000)/1000) * 1000

	items := g.portfolioItems(unit, t, int(active), int(atRisk))
	committed := float64(len(items))
	started := float64(countBy(items, "in_progress") + countBy(items, "done"))
	delivered := float64(countBy(items, "done"))
	atRisk = 0
	for _, it := range items {
		if it.Due != nil && it.Due.Before(t) {
			atRisk++
		}
	}
	active = started

	inits := g.llm.initiatives()
	offset := g.rng.Intn(len(inits))
	next := 0
	nextUnusedInitiative := func() string {
		name := inits[(offset+next)%len(inits)]
		next++
		return name
	}

	var initiativeRows [][]string
	for _, it := range items {
		if len(initiativeRows) >= 5 {
			break
		}
		initiativeRows = append(initiativeRows, []string{
			it.Title, it.Owner, it.State,
			fmt.Sprintf("%d%%", portfolioConfidence(it)),
			money(it.Size),
		})
	}

	var riskRows [][]string
	for _, it := range items {
		if it.Due == nil || !it.Due.Before(t) || len(riskRows) >= 4 {
			continue
		}
		riskRows = append(riskRows, []string{
			it.Title,
			it.Owner,
			g.llm.Cause(ArchStrategy),
			fmt.Sprintf("%d", int(t.Sub(*it.Due).Hours()/24)),
			"decision at the next portfolio review",
		})
	}

	events := []reporting.Event{{
		T: t.Add(-g.minutes(60, 3000)), Type: "initiative.started", Severity: reporting.SeverityInfo,
		Label: "started " + nextUnusedInitiative(),
	}}
	if delivered > 0 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(30, 2000)), Type: "initiative.delivered", Severity: reporting.SeverityInfo,
			Label: "delivered " + nextUnusedInitiative(),
		})
	}
	if atRisk > 0 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(20, 1500)), Type: "initiative.at_risk", Severity: reporting.SeverityWarn,
			Label: "at risk: " + g.llm.Cause(ArchStrategy),
		})
	}
	if g.rng.Float64() < 0.3 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(10, 1200)), Type: "investment.reallocated", Severity: reporting.SeverityInfo,
			Label: "investment moved onto " + nextUnusedInitiative(),
		})
	}
	if g.rng.Float64() < 0.15 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(5, 900)), Type: "initiative.stopped", Severity: reporting.SeverityWarn,
			Label: "stopped " + nextUnusedInitiative() +
				" after " + g.llm.Cause(ArchStrategy),
		})
	}

	return portfolioFigures{
		kpis: map[string]float64{
			"initiatives_active":  active,
			"okr_attainment_pct":  attainment,
			"initiatives_at_risk": atRisk,
			"investment_eur":      investment,
		},
		targets: map[string]float64{"okr_attainment_pct": okrAttainmentTarget},
		items:   items,
		trend:   g.series(t, 8, 7*24*time.Hour, attainment, 0.12),
		events:  events,
		tables: map[string]reporting.Table{
			"portfolio_stages": funnel([][2]interface{}{
				{"committed", committed},
				{"in progress", started},
				{"delivered", delivered},
			}),
			"at_risk":     {Columns: []string{"initiative", "owner", "risk", "days_at_risk", "ask"}, Rows: riskRows},
			"initiatives": {Columns: []string{"initiative", "owner", "stage", "confidence", "investment_eur"}, Rows: initiativeRows},
			"investment_by_unit": {
				Columns: []string{"unit", "investment_eur"},
				Rows:    [][]string{{unit, money(investment)}},
			},
		},
	}
}

func (g *Generator) portfolioItems(unit string, t time.Time, active, atRisk int) []reporting.Item {
	r := g.planRNG("portfolio", unit)
	inits := g.llm.initiatives()
	states := []string{"committed", "in_progress", "done"}
	quarters := []string{"this_quarter", "next_quarter", "h2", "next_year"}

	n := active + 2 + r.Intn(4)
	items := make([]reporting.Item, 0, n)
	for i := 0; i < n; i++ {
		born := g.start.Add(-time.Duration(r.Float64()*float64(200*24*time.Hour)) + time.Hour)
		pace := 0.3 + r.Float64()*0.9
		pos := (t.Sub(born).Hours() / 24 / 160) * pace
		idx := int(math.Floor(pos * float64(len(states)-1)))
		if idx < 0 {
			idx = 0
		}
		if idx > len(states)-1 {
			idx = len(states) - 1
		}
		it := reporting.Item{
			ID:      fmt.Sprintf("INV-%03d", i+1),
			Title:   inits[(i*7+r.Intn(3))%len(inits)],
			State:   states[idx],
			Owner:   g.llm.Owner(),
			Level:   []string{"goal", "initiative"}[i%2],
			Horizon: quarters[minInt(len(quarters)-1, idx)],
			Size:    math.Round(r.Float64()*4_000_000/1000) * 1000,
		}
		switch idx {
		case 0:
			it.Commitment = "provisional"
		case len(states) - 1:
			it.Commitment = "funded"
		default:
			it.Commitment = "funded"
		}
		if it.State != "done" {
			offset := time.Duration(20+r.Intn(200)) * 24 * time.Hour
			if i < atRisk {
				offset = -time.Duration(5+r.Intn(50)) * 24 * time.Hour
			}
			due := t.Add(offset)
			it.Due = &due
		}
		items = append(items, it)
	}
	return items
}

func portfolioConfidence(it reporting.Item) int {
	switch it.State {
	case "done":
		return 95
	case "in_progress":
		return 65
	default:
		return 40
	}
}

func buildTeamPortfolio(g *Generator, ti int, tm Team, t time.Time) Submission {
	f := g.portfolioNumbers(tm.Name, tm.Headcount, t)
	in := tm.instance("portfolio", t, f.kpis, f.targets)
	in.Series = map[string][]reporting.SeriesPoint{"okr_trend": f.trend}
	in.Events = f.events
	in.Tables = f.tables
	return tm.submit(in)
}

func buildPortfolio(g *Generator, scope, producer string, t time.Time) Submission {
	division := scopeLeaf(scope)
	f := g.portfolioNumbers(division, g.divisionHeadcount(division), t)
	return Submission{
		Producer: producer,
		Instance: &reporting.Instance{
			Definition: "portfolio",
			Scope:      scope,
			Producer:   producer,
			ProducedAt: t,
			KPIs:       f.kpis,
			Targets:    f.targets,
			Series:     map[string][]reporting.SeriesPoint{"okr_trend": f.trend},
			Events:     f.events,
			Items:      f.items,
			Tables:     f.tables,
		},
	}
}

func buildCompanyScorecard(g *Generator, scope, producer string, t time.Time) Submission {
	sc := g.co.Scorecard
	rounding := scorecardRounding
	if sc.Users < smallUniverseUsers {
		rounding = 1
	}
	mau := math.Round(sc.Users*g.between(0.985, 1.02)/rounding) * rounding
	premium := math.Round(sc.PayingUnits*g.between(0.985, 1.02)/rounding) * rounding
	arpu := roundCents(sc.RevenuePerUnit * g.between(0.97, 1.03))

	premiumRevenue := math.Round(premium * arpu)
	adShare := g.between(adSupportedShareLo, adSupportedShareHi)
	revenue := math.Round(premiumRevenue / (1 - adShare))
	grossMargin := round1(sc.GrossMarginPct * g.between(0.965, 1.035))
	opex := math.Round(revenue * g.between(0.18, 0.21))
	headcount := float64(g.co.Headcount())
	headcountPlan := math.Round(headcount * g.between(0.97, 1.04))

	moves := [][2]interface{}{
		{"subscriber growth", math.Round(revenue * g.between(0.004, 0.014))},
		{"ARPU movement", math.Round(revenue * g.between(-0.006, 0.008))},
		{"ad-supported", math.Round(revenue * g.between(-0.004, 0.010))},
		{"FX", math.Round(revenue * g.between(-0.008, 0.006))},
	}

	divisionRows := make([][]string, 0, len(sc.RevenueShare))
	for _, division := range g.co.Divisions() {
		share, ok := sc.RevenueShare[division]
		if !ok {
			continue
		}
		divisionRows = append(divisionRows, []string{division, money(revenue * share)})
	}

	trendWord := func(v float64) string {
		if v > 0.5 {
			return "up"
		}
		if v < -0.5 {
			return "down"
		}
		return "flat"
	}
	marginGap := round1(grossMargin - sc.GrossMarginPct)
	subsGap := premium - sc.PayingTarget

	revenuePlan := math.Round(sc.PayingTarget * sc.RevenuePerUnit / (1 - adShare))
	scorecardRows := [][]string{
		{sc.UnitLabels[0], money(mau), money(sc.UserTarget), money(mau - sc.UserTarget), trendWord(mau - sc.Users)},
		{sc.UnitLabels[1], money(premium), money(sc.PayingTarget), money(subsGap), trendWord(subsGap)},
	}
	if sc.RevenuePerUnit > 0 {
		scorecardRows = append(scorecardRows,
			[]string{
				"ARPU", fmt.Sprintf("%.2f", arpu), fmt.Sprintf("%.2f", sc.RevenuePerUnit),
				fmt.Sprintf("%.2f", arpu-sc.RevenuePerUnit), trendWord((arpu - sc.RevenuePerUnit) * 100),
			},
			[]string{"Revenue", money(revenue), money(revenuePlan), money(revenue - revenuePlan), trendWord(revenue - revenuePlan)},
		)
	}
	if sc.GrossMarginPct > 0 {
		scorecardRows = append(scorecardRows,
			[]string{
				"Gross margin", fmt.Sprintf("%.1f%%", grossMargin), fmt.Sprintf("%.1f%%", sc.GrossMarginPct),
				fmt.Sprintf("%.1f pts", marginGap), trendWord(marginGap),
			})
	}
	scorecardRows = append(scorecardRows,
		[]string{"Headcount", money(headcount), money(headcountPlan), money(headcount - headcountPlan), trendWord(headcount - headcountPlan)})

	var offTrack [][]string
	if marginGap < 0 && sc.GrossMarginPct > 0 {
		offTrack = append(offTrack, []string{"Gross margin", g.llm.Owner(), fmt.Sprintf("%.1f%%", grossMargin), fmt.Sprintf("%.1f%%", sc.GrossMarginPct), fmt.Sprintf("%.1f pts", marginGap)})
	}
	if subsGap < 0 {
		offTrack = append(offTrack, []string{sc.UnitLabels[1], g.llm.Owner(), money(premium), money(sc.PayingTarget), money(subsGap)})
	}

	events := []reporting.Event{{
		T: t.Add(-g.minutes(5, 120)), Type: "scorecard.published", Severity: reporting.SeverityInfo,
		Label: "monthly scorecard published to the board",
	}}
	if marginGap >= 0 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(10, 200)), Type: "target.met", Severity: reporting.SeverityInfo,
			Label: fmt.Sprintf("gross margin at %.1f%%, at or above plan", grossMargin),
		})
	} else {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(10, 200)), Type: "target.missed", Severity: reporting.SeverityWarn,
			Label: fmt.Sprintf("gross margin %.1f points below plan", -marginGap),
		})
	}
	if g.rng.Float64() < 0.2 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(20, 400)), Type: "guidance.revised", Severity: reporting.SeverityWarn,
			Label: "full-year guidance revised after " + g.llm.Cause(ArchFinance),
		})
	}

	return Submission{
		Producer: producer,
		Instance: &reporting.Instance{
			Definition: "company-scorecard",
			Scope:      scope,
			Producer:   producer,
			ProducedAt: t,
			KPIs: map[string]float64{
				"mau":              mau,
				"premium_subs":     premium,
				"arpu_eur":         arpu,
				"gross_margin_pct": grossMargin,
				"revenue_eur":      revenue,
				"opex_eur":         opex,
				"headcount":        headcount,
			},
			Targets: map[string]float64{"gross_margin_pct": scorecardMarginTarget, "premium_subs": scorecardSubsTarget},
			Series: map[string][]reporting.SeriesPoint{
				"mau_trend":     g.series(t, 12, 30*24*time.Hour, mau, 0.03),
				"revenue_trend": g.series(t, 12, 30*24*time.Hour, revenue, 0.05),
			},
			Events: events,
			Tables: map[string]reporting.Table{
				"revenue_bridge":      bridge("last month", "this month", revenue, moves),
				"revenue_by_division": {Columns: []string{"unit", "revenue_eur"}, Rows: divisionRows},
				"off_track":           {Columns: []string{"metric", "owner", "actual", "target", "gap"}, Rows: offTrack},
				"scorecard":           {Columns: []string{"metric", "actual", "target", "variance", "trend"}, Rows: scorecardRows},
			},
		},
	}
}
