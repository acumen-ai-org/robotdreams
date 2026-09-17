package simulation

import (
	"fmt"
	"math"
	"time"

	"github.com/acumen-ai-org/robotdreams/pkg/reporting"
)

const (
	hiringFillTargetDays        = 48.0
	hiringFillTargetDaysNorthAm = 42.0

	legalTurnaroundTargetDays           = 7.0
	legalTurnaroundTargetDaysRegulatory = 10.0

	supportTier1FirstResponseTargetMinutes = 30.0
	supportTier1ResolutionTargetHours      = 8.0
	supportTier2FirstResponseTargetMinutes = 120.0
	supportTier2ResolutionTargetHours      = 48.0

	itHelpdeskMTTRTargetHours = 8.0
	itSystemsMTTRTargetHours  = 12.0
)

func init() {
	registerTeam("hiring", everyWeek, buildHiring)
	registerTeam("workforce", everyWeek, buildWorkforce)
	registerTeam("legal-matters", everyWeek, buildLegalMatters)
	registerTeam("compliance", everyWeek, buildCompliance)
	registerTeam("support-queue", hourly, buildSupportQueue)
	registerTeam("it-service", everyDay, buildITService)
}

var hiringFunctions = []string{
	"engineering", "product", "design", "data science",
	"sales", "marketing", "customer support", "finance",
}

var hiringLevels = []string{"associate", "senior", "staff", "principal", "director"}

var workforceExitReasons = []string{
	"career growth elsewhere", "compensation", "manager or team fit",
	"relocation", "role redundancy", "performance",
}

var workforceRisks = []string{"high", "medium", "watch"}

var legalMatterTypes = []string{"commercial", "IP", "employment", "disputes", "regulatory"}

var (
	legalTypeMixCommercial = []float64{0.46, 0.22, 0.14, 0.08, 0.10}
	legalTypeMixRegulatory = []float64{0.14, 0.10, 0.12, 0.16, 0.48}
)

var legalCounterparties = []string{
	"a major label", "an indie aggregator", "a device partner",
	"a payments provider", "a cloud vendor", "a podcast network",
	"an audiobook publisher", "a telco bundling partner",
}

var complianceRegulations = []string{
	"GDPR", "DSA", "DMA", "CCPA", "SOX", "local telecoms", "local tax",
}

var complianceStatuses = []string{"on track", "at risk", "overdue"}

var complianceControls = []string{
	"access recertification", "data retention enforcement",
	"processor due diligence", "consent record integrity",
	"change approval evidence", "vendor DPA coverage",
	"transfer impact assessment", "revenue recognition cut-off",
}

var supportQueues = []string{
	"billing", "playback", "account recovery",
	"family plan", "refunds", "app crashes",
}

var supportHourBuckets = []string{"00-04", "04-08", "08-12", "12-16", "16-20", "20-24"}

var supportLanguages = []string{"en", "sv", "de", "es", "pt-BR", "ja", "hi"}

var supportHourShape = []float64{0.45, 0.30, 0.95, 1.15, 1.35, 1.10}

var (
	itHelpdeskServices = []string{
		"laptops and peripherals", "accounts and access", "VPN and network",
		"meeting rooms", "mobile devices", "printing",
	}
	itSystemsServices = []string{
		"identity provider", "HR system", "finance ERP",
		"collaboration suite", "device management", "ticketing platform",
	}
)

var (
	itHelpdeskApps = []string{"remote support", "asset tracking", "knowledge base"}
	itSystemsApps  = []string{
		"collaboration suite", "design tooling", "BI platform",
		"CRM", "project tracking", "e-signature",
	}
)

func gridTable(rows, cols []string, cell func(r, c int) float64) reporting.Table {
	out := make([][]string, 0, len(rows)*len(cols))
	for r, rowName := range rows {
		for c, colName := range cols {
			out = append(out, []string{rowName, colName, money(cell(r, c))})
		}
	}
	return reporting.Table{Columns: []string{"row", "column", "value"}, Rows: out}
}

func splitByWeight(total float64, weights []float64) []float64 {
	sum := 0.0
	for _, w := range weights {
		sum += w
	}
	out := make([]float64, len(weights))
	assigned := 0.0
	for i := 1; i < len(weights); i++ {
		out[i] = math.Round(total * weights[i] / sum)
		assigned += out[i]
	}
	out[0] = math.Max(0, total-assigned)
	return out
}

func evenWeights(n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = 1 / float64(i+2)
	}
	return out
}

func buildHiring(g *Generator, ti int, tm Team, t time.Time) Submission {
	recruiters := float64(tm.Headcount)
	openReqs := math.Round(recruiters * g.between(1.2, 2.2))
	week := g.season(t, tm, 0.15)

	offers := math.Max(1, math.Round(openReqs*g.between(0.04, 0.09)*week))
	accept := round1(g.between(84, 92))
	hires := math.Round(offers * accept / 100)
	pipeline := math.Round(openReqs * g.between(18, 34))
	timeToFill := math.Round(g.between(38, 58))

	onsite := math.Round(offers * g.between(2.6, 3.4))
	screened := math.Round(onsite * g.between(3.5, 5))
	applied := math.Round(screened * g.between(5, 8))

	byFunction := splitByWeight(openReqs, evenWeights(len(hiringFunctions)))
	funcRows := make([][]string, 0, len(hiringFunctions))
	for i, fn := range hiringFunctions {
		funcRows = append(funcRows, []string{
			fn, money(byFunction[i]),
			money(math.Round(g.between(12, 96))),
		})
	}

	aging := g.intBetween(2, 4)
	agingRows := make([][]string, 0, aging)
	for i := 0; i < aging; i++ {
		fn := hiringFunctions[g.rng.Intn(len(hiringFunctions))]
		agingRows = append(agingRows, []string{
			fmt.Sprintf("REQ-%d", 4100+g.rng.Intn(900)),
			fn, g.llm.Owner(),
			money(math.Round(g.between(timeToFill+8, timeToFill+70))),
			[]string{"screening", "onsite loop", "offer approval"}[g.rng.Intn(3)],
		})
	}

	offerRows := make([][]string, 0, 4)
	for i := 0; i < g.intBetween(2, 4); i++ {
		status := "accepted"
		if g.rng.Float64() > accept/100 {
			status = "declined"
		}
		offerRows = append(offerRows, []string{
			t.Add(-g.minutes(60, 5*24*60)).Format("2006-01-02"),
			g.llm.Service(tm.Archetype),
			hiringFunctions[g.rng.Intn(len(hiringFunctions))],
			hiringLevels[g.rng.Intn(len(hiringLevels))],
			status,
		})
	}

	var events []reporting.Event
	for i := 0; i < g.intBetween(1, 3); i++ {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(60, 4*24*60)), Type: "req.opened", Severity: reporting.SeverityInfo,
			Label: "opened a requisition for " + hiringFunctions[g.rng.Intn(len(hiringFunctions))],
		})
	}
	shown := int(math.Min(offers, 3))
	for i := 0; i < shown; i++ {
		when := t.Add(-g.minutes(30, 3*24*60))
		events = append(events, reporting.Event{
			T: when, Type: "offer.extended", Severity: reporting.SeverityInfo,
			Label: "offer out on " + g.llm.Service(tm.Archetype),
		})
		if float64(i) < hires {
			events = append(events, reporting.Event{
				T: when.Add(g.minutes(60, 900)), Type: "offer.accepted", Severity: reporting.SeverityInfo,
				Label: "offer signed: " + hiringLevels[g.rng.Intn(len(hiringLevels))] + " in " +
					hiringFunctions[g.rng.Intn(len(hiringFunctions))],
			})
		} else {
			events = append(events, reporting.Event{
				T: when.Add(g.minutes(60, 900)), Type: "offer.declined", Severity: reporting.SeverityWarn,
				Label: "offer declined: " + g.llm.Cause(tm.Archetype),
			})
		}
	}
	if len(agingRows) > 0 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(30, 2*24*60)), Type: "req.aged", Severity: reporting.SeverityWarn,
			Label: fmt.Sprintf("%d requisitions past time-to-fill target", len(agingRows)),
		})
	}

	fillTarget := hiringFillTargetDays
	if tm.Region == RegionNorthAm {
		fillTarget = hiringFillTargetDaysNorthAm
	}

	in := tm.instance("hiring", t,
		map[string]float64{
			"open_reqs":           openReqs,
			"offers_out":          offers,
			"offer_accept_pct":    accept,
			"time_to_fill_days":   timeToFill,
			"hires":               hires,
			"pipeline_candidates": pipeline,
		},
		map[string]float64{"offer_accept_pct": 88, "time_to_fill_days": fillTarget})
	in.Series = map[string][]reporting.SeriesPoint{
		"hires_trend":     g.series(t, 8, everyWeek, hires, 0.35),
		"pipeline_growth": g.series(t, 8, everyWeek, pipeline, 0.12),
	}
	in.Events = events
	in.Tables = map[string]reporting.Table{
		"hiring_funnel": funnel([][2]interface{}{
			{"applied", applied},
			{"screened", screened},
			{"onsite loop", onsite},
			{"offer", offers},
			{"hired", hires},
		}),
		"open_reqs_by_function": {
			Columns: []string{"function", "open_reqs", "days_open"}, Rows: funcRows,
		},
		"aging_reqs": {
			Columns: []string{"req", "function", "hiring_manager", "days_open", "stage"}, Rows: agingRows,
		},
		"recent_offers": {
			Columns: []string{"when", "role", "function", "level", "status"}, Rows: offerRows,
		},
		"hires_by_unit": byUnit("hires", tm, hires),
	}
	return tm.submit(in)
}

func buildWorkforce(g *Generator, ti int, tm Team, t time.Time) Submission {
	hc := float64(tm.Headcount)
	regretted := round1(g.between(6, 10))
	enps := math.Round(g.between(18, 42))
	absence := round1(g.between(2.4, 4.6))
	mobility := round1(g.between(6, 14))

	plan := hc + math.Round(hc*g.between(-0.03, 0.06))
	vsPlan := hc - plan

	voluntary := math.Round(hc * regretted / 100 * g.between(1.4, 1.9))
	involuntary := math.Round(hc * g.between(0.01, 0.03))
	transfers := math.Round(hc * g.between(0.03, 0.07))
	joiners := math.Round(hc * g.between(0.10, 0.18))
	leavers := voluntary + involuntary

	reasonCounts := splitByWeight(leavers, evenWeights(len(workforceExitReasons)))
	reasonRows := make([][]string, 0, len(workforceExitReasons))
	for i, reason := range workforceExitReasons {
		reasonRows = append(reasonRows, []string{reason, money(reasonCounts[i])})
	}

	watchRows := make([][]string, 0, 3)
	for i := 0; i < g.intBetween(1, 3); i++ {
		watchRows = append(watchRows, []string{
			tm.Roles[g.rng.Intn(len(tm.Roles))],
			tm.Name,
			workforceRisks[g.rng.Intn(len(workforceRisks))],
			"flagged after " + g.llm.Cause(tm.Archetype),
			g.llm.Owner(),
		})
	}

	var events []reporting.Event
	if joiners > 0 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(60, 5*24*60)), Type: "joiner.started", Severity: reporting.SeverityInfo,
			Label: "new starter onboarded into " + tm.Name,
		})
	}
	if g.rng.Float64() < 0.55 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(60, 6*24*60)), Type: "leaver.resigned", Severity: reporting.SeverityWarn,
			Label: "resignation: " + workforceExitReasons[g.rng.Intn(len(workforceExitReasons))],
		})
	}
	if g.rng.Float64() < 0.25 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(120, 6*24*60)), Type: "survey.closed", Severity: reporting.SeverityInfo,
			Label: fmt.Sprintf("engagement survey closed, eNPS %.0f", enps),
		})
	}
	if regretted > 9.5 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(60, 3*24*60)), Type: "attrition.spike", Severity: reporting.SeverityCritical,
			Label: fmt.Sprintf("regretted attrition at %.1f%% in %s", regretted, tm.Name),
		})
	}

	in := tm.instance("workforce", t,
		map[string]float64{
			"headcount":               hc,
			"headcount_vs_plan":       vsPlan,
			"regretted_attrition_pct": regretted,
			"enps":                    enps,
			"absence_pct":             absence,
			"internal_mobility_pct":   mobility,
		},
		map[string]float64{"regretted_attrition_pct": 8, "enps": 25})
	in.Series = map[string][]reporting.SeriesPoint{
		"headcount_trend": g.series(t, 8, everyWeek, hc, 0.03),
		"attrition_trend": g.series(t, 8, everyWeek, regretted, 0.18),
	}
	in.Events = events
	in.Tables = map[string]reporting.Table{
		"headcount_bridge": bridge("12 months ago", "today", hc, [][2]interface{}{
			{"hires", joiners},
			{"transfers in", transfers},
			{"voluntary exits", -voluntary},
			{"involuntary exits", -involuntary},
		}),
		"attrition_by_reason": {Columns: []string{"reason", "leavers"}, Rows: reasonRows},
		"retention_watchlist": {
			Columns: []string{"role", "unit", "risk", "note", "owner"}, Rows: watchRows,
		},
		"headcount_detail": {
			Columns: []string{"unit", "headcount", "plan", "joiners_12m", "leavers_12m", "attrition_pct"},
			Rows: [][]string{{
				tm.Name, money(hc), money(plan), money(joiners), money(leavers),
				pct(leavers / hc * 100),
			}},
		},
		"headcount_by_unit": byUnit("headcount", tm, hc),
	}
	return tm.submit(in)
}

func buildLegalMatters(g *Generator, ti int, tm Team, t time.Time) Submission {
	counsel := float64(tm.Headcount)
	week := g.season(t, tm, 0.15)

	inFlight := math.Round(counsel * g.between(2.5, 4.5))
	open := math.Round(counsel * g.between(1.5, 3.0))
	closed := math.Max(1, math.Round(counsel*g.between(0.4, 0.9)*week))
	turnaround := round1(g.between(4.5, 9.5))
	outsideEUR := math.Round(counsel * g.between(400, 1500))

	mix := legalTypeMixCommercial
	if tm.Archetype == ArchCompliance {
		mix = legalTypeMixRegulatory
	}
	openByType := splitByWeight(open, mix)
	closedByType := splitByWeight(closed, mix)
	typeRows := make([][]string, 0, len(legalMatterTypes))
	for i, typ := range legalMatterTypes {
		typeRows = append(typeRows, []string{
			typ, money(openByType[i]), money(closedByType[i]),
			fmt.Sprintf("%.1f", round1(turnaround*g.between(0.6, 1.8))),
		})
	}

	intake := inFlight + closed
	triaged := math.Round(intake * g.between(0.88, 0.96))
	drafted := math.Round(triaged * g.between(0.72, 0.86))
	negotiating := math.Round(drafted * g.between(0.60, 0.78))
	signed := math.Min(closed, negotiating)

	waiting := g.intBetween(1, 4)
	waitRows := make([][]string, 0, waiting)
	for i := 0; i < waiting; i++ {
		waitRows = append(waitRows, []string{
			t.Add(-g.minutes(240, 12*24*60)).Format("2006-01-02"),
			g.llm.Service(tm.Archetype),
			legalCounterparties[g.rng.Intn(len(legalCounterparties))],
			"business sign-off on " + g.llm.DecisionSubject(),
			g.llm.Owner(),
		})
	}

	openRows := make([][]string, 0, 5)
	for i := 0; i < g.intBetween(3, 5); i++ {
		openRows = append(openRows, []string{
			t.Add(-g.minutes(240, 40*24*60)).Format("2006-01-02"),
			legalMatterTypes[g.rng.Intn(len(legalMatterTypes))],
			g.llm.Service(tm.Archetype),
			legalCounterparties[g.rng.Intn(len(legalCounterparties))],
			g.llm.Owner(),
			money(math.Round(g.between(2, 90))),
		})
	}

	var events []reporting.Event
	for i := 0; i < g.intBetween(1, 3); i++ {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(120, 6*24*60)), Type: "matter.opened", Severity: reporting.SeverityInfo,
			Label: "opened " + g.llm.Service(tm.Archetype),
		})
	}
	for i := 0; i < int(math.Min(closed, 3)); i++ {
		when := t.Add(-g.minutes(60, 5*24*60))
		events = append(events, reporting.Event{
			T: when, Type: "matter.closed", Severity: reporting.SeverityInfo,
			Label: "closed " + g.llm.Service(tm.Archetype),
		})
		if g.rng.Float64() < 0.5 {
			events = append(events, reporting.Event{
				T: when.Add(g.minutes(5, 200)), Type: "contract.signed", Severity: reporting.SeverityInfo,
				Label: "signed with " + legalCounterparties[g.rng.Intn(len(legalCounterparties))],
			})
		}
	}
	for range waitRows {
		if g.rng.Float64() < 0.6 {
			events = append(events, reporting.Event{
				T: t.Add(-g.minutes(60, 4*24*60)), Type: "escalation.raised", Severity: reporting.SeverityWarn,
				Label: "waiting on the business: " + g.llm.DecisionSubject(),
			})
		}
	}
	if g.rng.Float64() < 0.12 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(60, 3*24*60)), Type: "deadline.missed", Severity: reporting.SeverityCritical,
			Label: "filing deadline missed: " + g.llm.Cause(tm.Archetype),
		})
	}

	in := tm.instance("legal-matters", t,
		map[string]float64{
			"contracts_in_flight":    inFlight,
			"median_turnaround_days": turnaround,
			"matters_open":           open,
			"matters_closed":         closed,
			"outside_counsel_eur":    outsideEUR,
		},
		map[string]float64{"median_turnaround_days": legalTurnaroundTargetDays})
	if tm.Archetype == ArchCompliance {
		in.Targets["median_turnaround_days"] = legalTurnaroundTargetDaysRegulatory
	}
	in.Series = map[string][]reporting.SeriesPoint{
		"turnaround_trend": g.series(t, 8, everyWeek, turnaround, 0.2),
		"in_flight_trend":  g.series(t, 8, everyWeek, inFlight, 0.15),
	}
	in.Events = events
	in.Tables = map[string]reporting.Table{
		"matter_stages": funnel([][2]interface{}{
			{"intake", intake},
			{"triaged", triaged},
			{"drafted", drafted},
			{"in negotiation", negotiating},
			{"signed", signed},
		}),
		"matters_by_type": {
			Columns: []string{"type", "open", "closed", "median_days"}, Rows: typeRows,
		},
		"awaiting_business_decision": {
			Columns: []string{"since", "matter", "counterparty", "question", "owner"}, Rows: waitRows,
		},
		"open_matters": {
			Columns: []string{"opened", "type", "matter", "counterparty", "owner", "days_open"}, Rows: openRows,
		},
		"matters_by_unit": byUnit("matters_open", tm, open),
	}
	return tm.submit(in)
}

func buildCompliance(g *Generator, ti int, tm Team, t time.Time) Submission {
	staff := float64(tm.Headcount)
	privacy := tm.Name == "privacy-office"

	var backlog, slaPct, tested, passed, findings, due30 float64
	if privacy {
		backlog = math.Round(staff * g.between(14, 34))
		slaPct = round1(g.between(94, 99))
		tested = math.Round(staff * g.between(1.0, 2.4))
		passed = round1(g.between(90, 98))
		findings = math.Round(staff * g.between(0.08, 0.25))
		due30 = float64(g.intBetween(2, 7))
	} else {
		backlog = math.Round(staff * g.between(1.5, 5))
		slaPct = round1(g.between(96, 99.6))
		tested = math.Round(staff * g.between(2.5, 4.5))
		passed = round1(g.between(88, 97))
		findings = math.Round(staff * g.between(0.10, 0.30))
		due30 = float64(g.intBetween(5, 14))
	}

	obligations := make([][3]float64, len(complianceRegulations))
	for i := range complianceRegulations {
		total := math.Round(g.between(6, 34))
		atRisk := math.Round(total * g.between(0.05, 0.22))
		overdue := 0.0
		if g.rng.Float64() < 0.3 {
			overdue = math.Round(g.between(1, 3))
		}
		obligations[i] = [3]float64{math.Max(0, total-atRisk-overdue), atRisk, overdue}
	}

	received := math.Round(backlog * g.between(1.3, 1.9))
	verified := math.Round(received * g.between(0.80, 0.92))
	gathered := math.Round(verified * g.between(0.85, 0.95))
	responded := math.Round(gathered * g.between(0.88, 0.97))
	closed := math.Round(responded * slaPct / 100)

	findingRows := make([][]string, 0, 4)
	for i := 0; i < int(math.Min(findings, 4)); i++ {
		sev := []string{"low", "medium", "high"}[g.rng.Intn(3)]
		findingRows = append(findingRows, []string{
			t.Add(-g.minutes(240, 45*24*60)).Format("2006-01-02"),
			complianceRegulations[g.rng.Intn(len(complianceRegulations))],
			g.llm.Cause(tm.Archetype),
			sev,
			g.llm.Owner(),
			t.Add(g.minutes(3*24*60, 60*24*60)).Format("2006-01-02"),
		})
	}

	testRows := make([][]string, 0, 5)
	for i := 0; i < g.intBetween(3, 5); i++ {
		outcome := "passed"
		if g.rng.Float64() > passed/100 {
			outcome = "failed"
		}
		testRows = append(testRows, []string{
			t.Add(-g.minutes(240, 25*24*60)).Format("2006-01-02"),
			complianceControls[g.rng.Intn(len(complianceControls))],
			complianceRegulations[g.rng.Intn(len(complianceRegulations))],
			g.llm.Owner(),
			outcome,
		})
	}

	var events []reporting.Event
	events = append(events,
		reporting.Event{
			T: t.Add(-g.minutes(60, 5*24*60)), Type: "dsar.received",
			Severity: reporting.SeverityInfo, Label: "subject access request received",
		},
		reporting.Event{
			T: t.Add(-g.minutes(60, 5*24*60)), Type: "dsar.closed",
			Severity: reporting.SeverityInfo, Label: "subject access request answered inside the deadline",
		},
	)
	if passed < 95 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(120, 5*24*60)), Type: "control.failed", Severity: reporting.SeverityWarn,
			Label: "control test failed: " + complianceControls[g.rng.Intn(len(complianceControls))],
		})
	}
	if findings > 0 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(120, 6*24*60)), Type: "finding.raised", Severity: reporting.SeverityWarn,
			Label: "audit finding: " + g.llm.Cause(tm.Archetype),
		})
	}
	if due30 > 4 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(60, 3*24*60)), Type: "deadline.approaching", Severity: reporting.SeverityWarn,
			Label: fmt.Sprintf("%.0f regulatory deadlines inside 30 days", due30),
		})
	}
	if g.rng.Float64() < 0.07 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(30, 2*24*60)), Type: "breach.reportable", Severity: reporting.SeverityCritical,
			Label: "reportable incident: 72-hour notification clock started",
		})
	}

	in := tm.instance("compliance", t,
		map[string]float64{
			"dsar_backlog":        backlog,
			"dsar_sla_pct":        slaPct,
			"controls_tested":     tested,
			"controls_passed_pct": passed,
			"audit_findings_open": findings,
			"deadlines_due_30d":   due30,
		},
		map[string]float64{"dsar_sla_pct": 100, "controls_passed_pct": 95})
	in.Series = map[string][]reporting.SeriesPoint{
		"dsar_backlog_trend": g.series(t, 8, everyWeek, backlog, 0.2),
		"control_pass_trend": g.series(t, 8, everyWeek, passed, 0.05),
	}
	in.Events = events
	in.Tables = map[string]reporting.Table{
		"regulation_status": gridTable(complianceRegulations, complianceStatuses,
			func(r, c int) float64 { return obligations[r][c] }),
		"dsar_stages": funnel([][2]interface{}{
			{"received", received},
			{"identity verified", verified},
			{"data gathered", gathered},
			{"responded", responded},
			{"closed inside deadline", closed},
		}),
		"findings_open": {
			Columns: []string{"raised", "regulation", "finding", "severity", "owner", "due"}, Rows: findingRows,
		},
		"control_tests": {
			Columns: []string{"tested", "control", "regulation", "owner", "outcome"}, Rows: testRows,
		},
		"controls_by_unit": byUnit("controls_passed_pct", tm, passed),
	}
	return tm.submit(in)
}

func buildSupportQueue(g *Generator, ti int, tm Team, t time.Time) Submission {
	agents := float64(tm.Headcount)
	load := g.season(t, tm, 0.55)
	tier1 := tm.Name == "tier1-care"

	var perAgent, frt, resolution, csat, deflection, escalationRate, backlogPerAgent, contactRate float64
	if tier1 {
		perAgent = g.between(3.2, 5.4)
		frt = math.Round(g.between(8, 38))
		resolution = round1(g.between(3, 9))
		csat = round1(g.between(4.3, 4.7))
		deflection = round1(g.between(28, 45))
		escalationRate = g.between(0.04, 0.07)
		backlogPerAgent = g.between(2, 9)
		contactRate = round1(g.between(0.9, 2.4))
	} else {
		perAgent = g.between(0.7, 1.1)
		frt = math.Round(g.between(40, 95))
		resolution = round1(g.between(18, 40))
		csat = round1(g.between(4.0, 4.4))
		deflection = round1(g.between(2, 8))
		escalationRate = g.between(0.01, 0.03)
		backlogPerAgent = g.between(6, 18)
		contactRate = round1(g.between(0.1, 0.4))
	}

	received := math.Round(agents * perAgent * load)
	resolved := math.Round(received * g.between(0.88, 1.06))
	backlog := math.Round(agents * backlogPerAgent)

	contacts := math.Round(received / (1 - deflection/100))
	pastSelfServe := math.Round(contacts * (1 - deflection/100))
	handled := math.Round(pastSelfServe * g.between(0.90, 0.98))
	escalated := math.Round(handled * escalationRate)

	queueWeights := splitByWeight(received, evenWeights(len(supportQueues)))

	queueRows := make([][]string, 0, len(supportQueues))
	for i, q := range supportQueues {
		qReceived := queueWeights[i]
		queueRows = append(queueRows, []string{
			q,
			money(qReceived),
			money(math.Round(qReceived * resolved / math.Max(1, received))),
			money(math.Round(backlog * qReceived / math.Max(1, received))),
			money(math.Round(frt * g.between(0.7, 1.4))),
			fmt.Sprintf("%.1f", round1(csat*g.between(0.95, 1.04))),
		})
	}

	aging := g.intBetween(2, 5)
	agingRows := make([][]string, 0, aging)
	for i := 0; i < aging; i++ {
		agingRows = append(agingRows, []string{
			t.Add(-g.minutes(frt, frt*8)).Format("15:04"),
			supportQueues[g.rng.Intn(len(supportQueues))],
			supportLanguages[g.rng.Intn(len(supportLanguages))],
			fmt.Sprintf("%.0fm", math.Round(g.between(frt, frt*6))),
			[]string{"P1", "P2", "P3"}[g.rng.Intn(3)],
			g.llm.Owner(),
		})
	}

	var events []reporting.Event
	if frt > 60 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(2, 55)), Type: "sla.breached", Severity: reporting.SeverityWarn,
			Label: fmt.Sprintf("first response at %.0fm on the %s queue",
				frt, supportQueues[g.rng.Intn(len(supportQueues))]),
		})
	}
	if resolved < received {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(2, 55)), Type: "queue.backlog_grew", Severity: reporting.SeverityWarn,
			Label: fmt.Sprintf("backlog grew by %.0f contacts", received-resolved),
		})
	} else if g.rng.Float64() < 0.3 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(2, 55)), Type: "queue.cleared", Severity: reporting.SeverityInfo,
			Label: "queue drained below target on " + supportQueues[g.rng.Intn(len(supportQueues))],
		})
	}
	if escalated > 0 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(1, 50)), Type: "ticket.escalated", Severity: reporting.SeverityWarn,
			Label: "escalated: " + g.llm.Cause(tm.Archetype),
		})
	}
	if load > 1.35 && g.rng.Float64() < 0.2 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(1, 40)), Type: "surge.detected", Severity: reporting.SeverityCritical,
			Label: "contact surge: " + g.llm.Cause(tm.Archetype),
		})
	}

	frtTarget, resTarget := supportTier1FirstResponseTargetMinutes, supportTier1ResolutionTargetHours
	if !tier1 {
		frtTarget, resTarget = supportTier2FirstResponseTargetMinutes, supportTier2ResolutionTargetHours
	}

	in := tm.instance("support-queue", t,
		map[string]float64{
			"tickets_received":       received,
			"tickets_resolved":       resolved,
			"backlog":                backlog,
			"first_response_minutes": frt,
			"resolution_hours":       resolution,
			"csat":                   csat,
			"contact_rate_per_1k":    contactRate,
			"deflection_pct":         deflection,
		},
		map[string]float64{
			"first_response_minutes": frtTarget,
			"resolution_hours":       resTarget,
			"csat":                   4.5,
		})
	in.Series = map[string][]reporting.SeriesPoint{
		"ticket_rate": g.series(t, 8, 10*time.Minute, received/6, 0.25),
		"csat_trend":  g.series(t, 8, time.Hour, csat, 0.04),
	}
	in.Events = events
	in.Tables = map[string]reporting.Table{
		"queue_by_hour": gridTable(supportQueues, supportHourBuckets, func(r, c int) float64 {
			return math.Round(queueWeights[r] * supportHourShape[c])
		}),
		"contact_stages": funnel([][2]interface{}{
			{"contacts", contacts},
			{"not deflected by self-serve", pastSelfServe},
			{"handled by an agent", handled},
			{"escalated", escalated},
		}),
		"aging_tickets": {
			Columns: []string{"opened", "queue", "language", "waiting", "priority", "owner"}, Rows: agingRows,
		},
		"queue_detail": {
			Columns: []string{"queue", "received", "resolved", "backlog", "first_response_minutes", "csat"},
			Rows:    queueRows,
		},
		"backlog_by_unit": byUnit("backlog", tm, backlog),
	}
	return tm.submit(in)
}

func buildITService(g *Generator, ti int, tm Team, t time.Time) Submission {
	staff := float64(tm.Headcount)
	day := g.season(t, tm, 0.3)
	helpdesk := tm.Name == "helpdesk"

	var tickets, mttr, compliance, licensed, used, uptime float64
	var services, apps []string
	if helpdesk {
		tickets = math.Round(staff * g.between(1.2, 2.4) * day)
		mttr = round1(g.between(3, 9))
		compliance = round1(g.between(91, 97))
		licensed = math.Round(staff * g.between(1.1, 1.6))
		used = math.Round(licensed * g.between(0.78, 0.95))
		uptime = round1(g.between(99.2, 99.8))
		services, apps = itHelpdeskServices, itHelpdeskApps
	} else {
		tickets = math.Round(staff * g.between(0.3, 0.8) * day)
		mttr = round1(g.between(6, 18))
		compliance = round1(g.between(94, 99))
		licensed = math.Round(float64(g.co.Headcount()) * g.between(2.4, 3.4))
		used = math.Round(licensed * g.between(0.72, 0.88))
		uptime = round1(g.between(99.0, 99.9))
		services, apps = itSystemsServices, itSystemsApps
	}

	ticketWeights := splitByWeight(tickets, evenWeights(len(services)))
	serviceRows := make([][]string, 0, len(services))
	healthRows := make([][]string, 0, len(services))
	for i, svc := range services {
		serviceRows = append(serviceRows, []string{
			svc, money(ticketWeights[i]),
			fmt.Sprintf("%.1f", round1(mttr*g.between(0.5, 1.7))),
		})
		healthRows = append(healthRows, []string{
			svc,
			fmt.Sprintf("%.2f", round1(uptime*g.between(0.998, 1.001))),
			money(math.Round(g.between(0, 3))),
			pct(compliance * g.between(0.97, 1.02)),
		})
	}

	licenceWeights := splitByWeight(licensed, evenWeights(len(apps)))
	usedWeights := splitByWeight(used, evenWeights(len(apps)))
	gapRows := make([][]string, 0, len(apps))
	for i, app := range apps {
		idle := math.Max(0, licenceWeights[i]-usedWeights[i])
		perSeat := g.between(90, 420)
		gapRows = append(gapRows, []string{
			app, money(licenceWeights[i]), money(usedWeights[i]),
			money(idle), money(idle * perSeat),
		})
	}

	var events []reporting.Event
	events = append(events, reporting.Event{
		T: t.Add(-g.minutes(30, 20*60)), Type: "ticket.raised", Severity: reporting.SeverityInfo,
		Label: "raised against " + services[g.rng.Intn(len(services))],
	})
	if g.rng.Float64() < 0.35 {
		start := t.Add(-g.minutes(120, 18*60))
		events = append(events,
			reporting.Event{
				T: start, Type: "service.degraded", Severity: reporting.SeverityWarn,
				Label: services[g.rng.Intn(len(services))] + " degraded: " + g.llm.Cause(tm.Archetype),
			},
			reporting.Event{
				T:    start.Add(time.Duration(mttr * float64(time.Hour))),
				Type: "service.restored", Severity: reporting.SeverityInfo, Label: "service restored",
			},
		)
	}
	if compliance < 95 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(60, 18*60)), Type: "patch.overdue", Severity: reporting.SeverityWarn,
			Label: fmt.Sprintf("%.0f%% of the fleet compliant, patch SLA at risk", compliance),
		})
	}
	if used/licensed > 0.97 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(30, 16*60)), Type: "licence.exhausted", Severity: reporting.SeverityCritical,
			Label: "licence pool exhausted on " + apps[g.rng.Intn(len(apps))],
		})
	}

	mttrTarget := itHelpdeskMTTRTargetHours
	if !helpdesk {
		mttrTarget = itSystemsMTTRTargetHours
	}

	in := tm.instance("it-service", t,
		map[string]float64{
			"tickets":               tickets,
			"mttr_hours":            mttr,
			"device_compliance_pct": compliance,
			"seats_licensed":        licensed,
			"seats_used":            used,
			"internal_uptime_pct":   uptime,
		},
		map[string]float64{
			"mttr_hours":            mttrTarget,
			"device_compliance_pct": 95,
			"internal_uptime_pct":   99.5,
		})
	in.Series = map[string][]reporting.SeriesPoint{
		"ticket_rate":  g.series(t, 8, time.Hour, tickets/10, 0.3),
		"uptime_trend": g.series(t, 8, everyDay, uptime, 0.002),
	}
	in.Events = events
	in.Tables = map[string]reporting.Table{
		"tickets_by_service": {
			Columns: []string{"service", "tickets", "mttr_hours"}, Rows: serviceRows,
		},
		"licence_gap": {
			Columns: []string{"application", "licensed", "used", "idle_90d", "annual_eur"}, Rows: gapRows,
		},
		"service_health": {
			Columns: []string{"service", "uptime_pct", "incidents", "compliance_pct"}, Rows: healthRows,
		},
		"tickets_by_unit": byUnit("tickets", tm, tickets),
	}
	return tm.submit(in)
}
