package simulation

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/acumen-ai-org/robotdreams/pkg/reporting"
)

const (
	latencyBudgetFactor       = 1.6
	warnErrorLines            = 8
	criticalErrorLines        = 40
	leadTimeTargetMinutes     = 60
	slipWithinSprintDays      = 5
	decisionWaitTargetMinutes = 120
	headcountPerCostUnit      = 30
	dailyBudgetPerCostUnit    = 420
	atRiskRoadmapColumn       = 2
)

func buildPerformance(g *Generator, ti int, tm Team, t time.Time) Submission {
	base := 120 + float64(ti)*4
	p95 := base * g.season(t, tm, 0.25) * (1 + g.between(0, 0.3))
	throughput := (900 + float64(ti)*90) * g.season(t, tm, 0.4) * (1 + g.between(-0.1, 0.1))
	errRate := g.between(0.05, 1.2)
	capacity := g.between(40, 82)

	if inc := g.incidentAt(ti, t); inc != nil {
		p95 *= g.between(4, 12)
		errRate += g.between(5, 20)
		capacity = math.Min(99, capacity+g.between(5, 15))
	}

	var events []reporting.Event
	hourAgo := t.Add(-time.Hour)
	for i := range g.incidents {
		ic := &g.incidents[i]
		if ic.teamIdx != ti {
			continue
		}
		if ic.opened.After(hourAgo) && !ic.opened.After(t) {
			events = append(events, reporting.Event{
				T: ic.opened, Type: "perf.degraded", Severity: reporting.SeverityWarn,
				Label: "latency degraded: " + ic.summary,
			})
		}
		if !ic.resolved.IsZero() && ic.resolved.After(hourAgo) && !ic.resolved.After(t) {
			events = append(events, reporting.Event{
				T: ic.resolved, Type: "perf.recovered", Severity: reporting.SeverityInfo,
				Label: ic.service + " latency back to baseline",
			})
		}
	}

	in := tm.instance("performance", t,
		map[string]float64{
			"p95_latency_ms":     round1(p95),
			"throughput_per_min": round1(throughput),
			"error_rate_pct":     round1(errRate),
			"capacity_used_pct":  round1(capacity),
		},
		map[string]float64{
			"p95_latency_ms":    round1(base * latencyBudgetFactor),
			"error_rate_pct":    1,
			"capacity_used_pct": 75,
		})
	in.Series = map[string][]reporting.SeriesPoint{
		"latency":    g.series(t, 6, 10*time.Minute, p95, 0.15),
		"throughput": g.series(t, 6, 10*time.Minute, throughput, 0.1),
	}
	in.Events = events
	in.Tables = map[string]reporting.Table{
		"latency_by_unit": byUnit("p95_latency_ms", tm, round1(p95)),
	}
	return tm.submit(in)
}

func buildLogs(g *Generator, ti int, tm Team, t time.Time) Submission {
	last := !t.Add(hourly).Before(g.now)
	logLines := (2000 + g.between(0, 9000)) * g.season(t, tm, 0.35)
	warnLines := g.between(4, 45)
	errLines := 0.0
	if g.incidentAt(ti, t) != nil {
		errLines = math.Round(g.between(30, 150))
	} else if g.rng.Float64() < 0.08 {
		errLines = math.Round(g.between(1, 5))
	}
	if last {
		switch ti {
		case g.warnTeam:
			errLines = warnErrorLines
		case g.critTeam:
			errLines = criticalErrorLines
		}
	}

	var events []reporting.Event
	var rows [][]string
	if errLines > 0 {
		n := 1 + g.rng.Intn(2)
		for i := 0; i < n; i++ {
			comp := g.llm.Component(tm.Archetype)
			msg := g.llm.LogError(comp, g.llm.Cause(tm.Archetype))
			et := t.Add(-g.minutes(1, 55))
			events = append(events, reporting.Event{
				T: et, Type: "log.error", Severity: reporting.SeverityCritical, Label: msg,
			})
			rows = append(rows, []string{et.Format("15:04:05"), comp, "ERROR", msg})
		}
	}
	if g.rng.Float64() < 0.3 {
		comp := g.llm.Component(tm.Archetype)
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(1, 55)), Type: "log.warn", Severity: reporting.SeverityWarn,
			Label: comp + ": slow response from dependency",
		})
	}

	in := tm.instance("logs", t,
		map[string]float64{
			"log_lines":   math.Round(logLines),
			"error_lines": errLines,
			"warn_lines":  math.Round(warnLines),
		}, nil)
	in.Series = map[string][]reporting.SeriesPoint{
		"log_volume": g.series(t, 6, 10*time.Minute, logLines/60, 0.25),
	}
	in.Events = events
	in.Tables = map[string]reporting.Table{
		"recent_errors":  {Columns: []string{"when", "component", "level", "message"}, Rows: rows},
		"errors_by_unit": byUnit("error_lines", tm, errLines),
	}
	return tm.submit(in)
}

func buildDelivery(g *Generator, ti int, tm Team, t time.Time) Submission {
	deploys := 1 + g.rng.Intn(5)
	failed := 0
	if g.rng.Float64() < 0.15 {
		failed = 1
	}
	leadTime := math.Round(g.between(12, 85))

	var events []reporting.Event
	var rows [][]string
	shown := deploys
	if shown > 3 {
		shown = 3
	}
	for i := 0; i < shown; i++ {
		service, version := g.llm.Service(tm.Archetype), g.llm.Version()
		dur := g.minutes(2, 9)
		end := t.Add(-g.minutes(5, 170))
		start := end.Add(-dur)
		status := "ok"
		events = append(events,
			reporting.Event{
				T: start, Type: "deploy.started", Severity: reporting.SeverityInfo,
				Label: "deploying " + service + " " + version,
			},
			reporting.Event{
				T: end, Type: "deploy.finished", Severity: reporting.SeverityInfo,
				Label: g.llm.DeployLabel(service, version),
			},
		)
		if failed > 0 && i == 0 {
			status = "rolled back"
			events = append(events, reporting.Event{
				T: end.Add(g.minutes(1, 5)), Type: "deploy.rolled_back", Severity: reporting.SeverityWarn,
				Label: g.llm.RollbackLabel(service, version),
			})
		}
		rows = append(rows, []string{
			end.Format("15:04"), service, version, status,
			fmt.Sprintf("%dm%02ds", int(dur.Minutes()), int(dur.Seconds())%60),
		})
	}
	if g.rng.Float64() < 0.3 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(1, 60)), Type: "release.tagged", Severity: reporting.SeverityInfo,
			Label: "tagged " + g.llm.Service(tm.Archetype) + " " + g.llm.Version(),
		})
	}

	in := tm.instance("delivery", t,
		map[string]float64{
			"deploys":           float64(deploys),
			"failed_deploys":    float64(failed),
			"lead_time_minutes": leadTime,
		},
		map[string]float64{"lead_time_minutes": leadTimeTargetMinutes, "failed_deploys": 0})
	in.Series = map[string][]reporting.SeriesPoint{
		"deploy_frequency": g.series(t, 6, 30*time.Minute, float64(deploys)/3, 0.5),
	}
	in.Events = events
	in.Tables = map[string]reporting.Table{
		"recent_deploys":  {Columns: []string{"when", "service", "version", "status", "duration"}, Rows: rows},
		"deploys_by_unit": byUnit("deploys", tm, float64(deploys)),
	}
	return tm.submit(in)
}

func buildTeamActivity(g *Generator, ti int, tm Team, t time.Time) Submission {
	load := g.season(t, tm, 0.85)
	nodes := len(tm.Roles)
	inFlight := int(math.Round(float64(g.rng.Intn(4)) * load))
	finished := int(math.Round(float64(g.rng.Intn(6)) * load))

	in := tm.instance("activity", t,
		map[string]float64{
			"active_nodes":    float64(nodes),
			"tasks_in_flight": float64(inFlight),
			"tasks_finished":  float64(finished),
		}, nil)
	in.Events = g.activityEvents(t, tm.Archetype)
	return tm.submit(in)
}

func buildScopeActivity(g *Generator, scope, producer string, t time.Time) Submission {
	return Submission{
		Producer: producer,
		Instance: &reporting.Instance{
			Definition: "activity",
			Scope:      scope,
			Producer:   producer,
			ProducedAt: t,
			KPIs: map[string]float64{
				"active_nodes":    5,
				"tasks_in_flight": float64(g.rng.Intn(8)),
				"tasks_finished":  float64(g.rng.Intn(12)),
			},
			Events: g.activityEvents(t, ArchStrategy),
		},
	}
}

func (g *Generator) activityEvents(t time.Time, a Archetype) []reporting.Event {
	var events []reporting.Event
	n := 1 + g.rng.Intn(2)
	for i := 0; i < n; i++ {
		label := g.llm.TaskLabel(a)
		st := t.Add(-g.minutes(1, 28))
		events = append(events, reporting.Event{
			T: st, Type: "task.started", Severity: reporting.SeverityInfo, Label: "started " + label,
		})
		if g.rng.Float64() < 0.7 {
			events = append(events, reporting.Event{
				T: st.Add(g.minutes(1, 10)), Type: "task.finished", Severity: reporting.SeverityInfo,
				Label: "finished " + label,
			})
		}
	}
	if g.rng.Float64() < 0.08 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(1, 25)), Type: "task.blocked", Severity: reporting.SeverityWarn,
			Label: "blocked on " + g.llm.Cause(a),
		})
	}
	return events
}

func buildRoadmap(g *Generator, scope, producer string, t time.Time) Submission {
	hit := 1 + g.rng.Intn(5)
	missed := 0
	if r := g.rng.Float64(); r < 0.1 {
		missed = 2
	} else if r < 0.35 {
		missed = 1
	}
	slip := math.Round(g.between(0, 2.9))
	if missed > 0 {
		slip = math.Round(g.between(3, 16))
	}

	phaseStart := t.Add(-g.minutes(300, 660))
	events := []reporting.Event{
		{
			T: phaseStart, Type: "phase.started", Severity: reporting.SeverityInfo,
			Label: "phase started: " + g.llm.MilestoneName(),
		},
		{
			T: t.Add(-g.minutes(30, 240)), Type: "milestone.reached", Severity: reporting.SeverityInfo,
			Label: "reached: " + g.llm.MilestoneName(),
		},
	}
	if g.rng.Float64() < 0.5 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(10, 120)), Type: "phase.finished", Severity: reporting.SeverityInfo,
			Label: "phase wrapped up",
		})
	}
	if missed > 0 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(20, 200)), Type: "milestone.missed", Severity: reporting.SeverityWarn,
			Label: "missed: " + g.llm.MilestoneName(),
		})
	}
	if g.rng.Float64() < 0.2 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(5, 100)), Type: "plan.rescheduled", Severity: reporting.SeverityWarn,
			Label: fmt.Sprintf("replanned around %s (+%dd)", g.llm.Cause(ArchStrategy), int(slip)),
		})
	}

	items := g.roadmapItems(scope, t, missed)
	var rows [][]string
	for _, it := range items {
		if it.State == "done" || it.Due == nil {
			continue
		}
		rows = append(rows, []string{
			it.Due.Format("2006-01-02"), it.Title, it.Owner,
			fmt.Sprintf("%d%%", confidenceOf(it, t)),
		})
	}

	return Submission{
		Producer: producer,
		Instance: &reporting.Instance{
			Definition: "roadmap",
			Scope:      scope,
			Producer:   producer,
			ProducedAt: t,
			KPIs: map[string]float64{
				"milestones_hit":    float64(hit),
				"milestones_missed": float64(missed),
				"slip_days":         slip,
			},
			Targets: map[string]float64{"milestones_missed": 0, "slip_days": slipWithinSprintDays},
			Events:  events,
			Items:   items,
			Tables: map[string]reporting.Table{
				"upcoming": {Columns: []string{"due", "milestone", "owner", "confidence"}, Rows: rows},
			},
		},
	}
}

func (g *Generator) roadmapItems(scope string, t time.Time, missed int) []reporting.Item {
	r := g.planRNG("roadmap", scope)
	llm := g.planLLM("roadmap", scope)
	states := []string{"proposed", "committed", "in_progress", "done"}
	horizons := []string{"now", "next", "later"}

	n := 5 + r.Intn(4)
	items := make([]reporting.Item, 0, n)
	for i := 0; i < n; i++ {
		born := g.start.Add(-time.Duration(r.Float64()*float64(60*24*time.Hour)) + time.Hour)
		pace := 0.3 + r.Float64()*0.8
		pos := (t.Sub(born).Hours() / 24 / 50) * pace
		idx := int(math.Floor(pos * float64(len(states)-1)))
		if idx < 0 {
			idx = 0
		}
		if idx > len(states)-1 {
			idx = len(states) - 1
		}
		if missed > 0 && i < missed && idx > 0 && idx < len(states)-1 {
			idx = atRiskRoadmapColumn
		}
		it := reporting.Item{
			ID:      fmt.Sprintf("RM-%03d", i+1),
			Title:   llm.MilestoneName(),
			State:   states[idx],
			Owner:   llm.Owner(),
			Level:   []string{"workstream", "activity"}[i%2],
			Horizon: horizons[minInt(len(horizons)-1, idx)],
			Size:    float64(5 + r.Intn(30)),
		}
		switch idx {
		case atRiskRoadmapColumn:
			it.Commitment = "planned"
		case 0:
			it.Commitment = "candidate"
		default:
			it.Commitment = "committed"
		}
		if it.State != "done" {
			due := t.Add(roadmapDueOffset(r, idx))
			it.Due = &due
		}
		items = append(items, it)
	}
	return items
}

func roadmapDueOffset(r *rand.Rand, idx int) time.Duration {
	ahead := time.Duration(3+r.Intn(40)) * 24 * time.Hour
	if idx == atRiskRoadmapColumn {
		return -time.Duration(1+r.Intn(9)) * 24 * time.Hour
	}
	return ahead
}

func confidenceOf(it reporting.Item, now time.Time) int {
	switch {
	case it.Due != nil && it.Due.Before(now):
		return 40
	case it.State == "in_progress":
		return 80
	default:
		return 60
	}
}

func buildDecisions(g *Generator, scope, producer string, t time.Time) Submission {
	open := g.rng.Intn(6)
	awaiting := 0
	if open > 0 {
		awaiting = g.rng.Intn(open + 1)
	}
	decided := 1 + g.rng.Intn(9)
	wait := math.Round(g.between(4, 220))

	var events []reporting.Event
	var rows [][]string
	for i := 0; i < awaiting; i++ {
		asked := t.Add(-g.minutes(10, 400))
		subject := g.llm.DecisionSubject()
		events = append(events, reporting.Event{
			T: asked, Type: "decision.requested", Severity: reporting.SeverityInfo,
			Label: "input needed on " + subject,
		})
		if g.rng.Float64() < 0.45 {
			events = append(events, reporting.Event{
				T: asked.Add(g.minutes(1, 20)), Type: "escalation.raised", Severity: reporting.SeverityWarn,
				Label: "escalated: " + subject + " is blocking downstream work",
			})
		}
		rows = append(rows, []string{
			asked.Format("15:04"), producer,
			"approve " + subject, fmt.Sprintf("%d task(s)", 1+g.rng.Intn(4)),
		})
	}
	for i := 0; i < decided && i < 3; i++ {
		made := t.Add(-g.minutes(5, 300))
		events = append(events, reporting.Event{
			T: made, Type: "decision.made", Severity: reporting.SeverityInfo,
			Label: "approved " + g.llm.DecisionSubject(),
		})
	}

	raised := float64(decided + open)
	return Submission{
		Producer: producer,
		Instance: &reporting.Instance{
			Definition: "decisions",
			Scope:      scope,
			Producer:   producer,
			ProducedAt: t,
			KPIs: map[string]float64{
				"open_decisions":      float64(open),
				"awaiting_human":      float64(awaiting),
				"decided_24h":         float64(decided),
				"median_wait_minutes": wait,
			},
			Targets: map[string]float64{"median_wait_minutes": decisionWaitTargetMinutes, "awaiting_human": 0},
			Series: map[string][]reporting.SeriesPoint{
				"open_decisions_trend": g.series(t, 8, time.Hour, float64(open)+1, 0.6),
			},
			Events: events,
			Tables: map[string]reporting.Table{
				"waiting": {Columns: []string{"since", "asked_by", "question", "blocking"}, Rows: rows},
				"decision_stages": funnel([][2]interface{}{
					{"raised", raised},
					{"reached a human", raised - float64(awaiting)*0.4},
					{"decided", float64(decided)},
				}),
			},
		},
	}
}

func buildCost(g *Generator, ti int, tm Team, t time.Time) Submission {
	scale := float64(tm.Headcount) / headcountPerCostUnit
	tokens := math.Round(g.between(40_000, 900_000) * scale)
	compute := math.Round(g.between(20, 400) * scale)
	spend := math.Round(tokens/1000*0.6+compute*0.11) + 1
	budget := math.Round(scale * dailyBudgetPerCostUnit)

	var events []reporting.Event
	if spend > budget*0.8 {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(2, 90)), Type: "budget.threshold", Severity: reporting.SeverityWarn,
			Label: "daily spend passed 80% of budget",
		})
	}
	if spend > budget {
		events = append(events, reporting.Event{
			T: t.Add(-g.minutes(1, 40)), Type: "budget.exceeded", Severity: reporting.SeverityCritical,
			Label: "daily budget exceeded for " + tm.Name,
		})
	}

	rows := [][]string{}
	for _, id := range tm.WorkerIDs() {
		share := g.between(0.25, 0.75)
		rows = append(rows, []string{id, money(tokens * share), money(compute * share), money(spend * share)})
	}

	in := tm.instance("cost", t,
		map[string]float64{
			"tokens":          tokens,
			"compute_minutes": compute,
			"spend_usd":       spend,
			"budget_used_pct": round1(spend / budget * 100),
		},
		map[string]float64{"spend_usd": budget, "budget_used_pct": 100})
	in.Series = map[string][]reporting.SeriesPoint{
		"spend_rate": g.series(t, 8, 30*time.Minute, spend/6, 0.5),
	}
	in.Events = events
	in.Tables = map[string]reporting.Table{
		"top_spenders": {Columns: []string{"node", "tokens", "compute_minutes", "spend_usd"}, Rows: rows},
		"spend_bridge": bridge("yesterday", "today", spend, [][2]interface{}{
			{"token volume", math.Round(g.between(-90, 160) * scale)},
			{"compute", math.Round(g.between(-40, 70) * scale)},
			{"cache hits", math.Round(g.between(-70, 10) * scale)},
		}),
		"spend_by_unit": byUnit("spend_usd", tm, spend),
	}
	return tm.submit(in)
}

func buildQuality(g *Generator, ti int, tm Team, t time.Time) Submission {
	reviews := 2 + g.rng.Intn(14)
	rework := g.rng.Intn(reviews/2 + 1)
	accepted := reviews - rework
	rate := math.Round(float64(accepted) / float64(reviews) * 100)

	var events []reporting.Event
	var rows [][]string
	shown := reviews
	if shown > 4 {
		shown = 4
	}
	ids := tm.WorkerIDs()
	for i := 0; i < shown; i++ {
		when := t.Add(-g.minutes(5, 175))
		artifact := g.llm.Service(tm.Archetype)
		outcome := "accepted"
		typ, sev := "review.passed", reporting.SeverityInfo
		if i < rework {
			outcome, typ, sev = "sent back", "review.failed", reporting.SeverityWarn
		}
		events = append(events, reporting.Event{
			T: when, Type: typ, Severity: sev, Label: g.llm.ReviewLabel(artifact, outcome),
		})
		if typ == "review.failed" {
			events = append(events, reporting.Event{
				T: when.Add(g.minutes(1, 15)), Type: "rework.started", Severity: reporting.SeverityWarn,
				Label: "rework started on " + artifact,
			})
		}
		rows = append(rows, []string{when.Format("15:04"), artifact, ids[i%len(ids)], outcome})
	}

	in := tm.instance("quality", t,
		map[string]float64{
			"reviews":         float64(reviews),
			"accepted":        float64(accepted),
			"rework":          float64(rework),
			"acceptance_rate": rate,
		},
		map[string]float64{"acceptance_rate": 85})
	in.Series = map[string][]reporting.SeriesPoint{
		"acceptance_trend": g.series(t, 8, time.Hour, rate, 0.12),
	}
	in.Events = events
	in.Tables = map[string]reporting.Table{
		"recent_reviews": {Columns: []string{"when", "artifact", "reviewer", "outcome"}, Rows: rows},
		"review_stages": funnel([][2]interface{}{
			{"submitted", float64(reviews)},
			{"reviewed", float64(reviews)},
			{"accepted", float64(accepted)},
		}),
		"acceptance_by_unit": byUnit("acceptance_rate", tm, rate),
	}
	return tm.submit(in)
}

func (g *Generator) incidentLifecycle(inc *incident) []Submission {
	subs := []Submission{g.incidentInstance(inc, "opened", inc.opened)}
	if !inc.mitigated.IsZero() {
		subs = append(subs, g.incidentInstance(inc, "mitigated", inc.mitigated))
	}
	if !inc.resolved.IsZero() {
		subs = append(subs, g.incidentInstance(inc, "resolved", inc.resolved))
	}
	return subs
}

func (g *Generator) incidentInstance(inc *incident, stage string, t time.Time) Submission {
	tm := g.co.Teams[inc.teamIdx]
	open, resolved := 1.0, 0.0
	mttr := math.Round(g.between(25, 90))
	errBase := g.between(8, 30)

	var ev reporting.Event
	rows := [][]string{{inc.opened.Format("15:04"), "critical", tm.Scope(), inc.summary, g.llm.Owner()}}
	switch stage {
	case "opened":
		ev = reporting.Event{
			T: t, Type: "incident.opened", Severity: reporting.SeverityCritical,
			Label: inc.summary,
		}
	case "mitigated":
		ev = reporting.Event{
			T: t, Type: "incident.mitigated", Severity: reporting.SeverityWarn,
			Label: "mitigated: " + inc.summary,
		}
		errBase /= 3
	case "resolved":
		ev = reporting.Event{
			T: t, Type: "incident.resolved", Severity: reporting.SeverityInfo,
			Label: "resolved: " + inc.summary,
		}
		open, resolved = 0, 1
		mttr = math.Round(t.Sub(inc.opened).Minutes())
		errBase = g.between(0.1, 1)
		rows = nil
	}

	in := tm.instance("incident", t,
		map[string]float64{
			"open_incidents":     open,
			"resolved_incidents": resolved,
			"mttr_minutes":       mttr,
		},
		map[string]float64{"mttr_minutes": 60, "open_incidents": 0})
	in.Series = map[string][]reporting.SeriesPoint{
		"error_rate": g.series(t, 6, 5*time.Minute, errBase, 0.4),
	}
	in.Events = []reporting.Event{ev}
	in.Tables = map[string]reporting.Table{
		"open_list":         {Columns: []string{"opened", "severity", "scope", "summary", "owner"}, Rows: rows},
		"incidents_by_unit": byUnit("open_incidents", tm, open),
	}
	return tm.submit(in)
}

func (g *Generator) pulseBatch(def string, tm Team, t time.Time) EventBatch {
	var events []reporting.Event
	switch def {
	case "logs":
		typ, sev := "log.warn", reporting.SeverityWarn
		label := g.llm.Component(tm.Archetype) + ": retrying flaky dependency"
		if g.rng.Float64() < 0.25 {
			typ, sev = "log.error", reporting.SeverityCritical
			label = g.llm.LogError(g.llm.Component(tm.Archetype), g.llm.Cause(tm.Archetype))
		}
		events = append(events, reporting.Event{T: t, Type: typ, Severity: sev, Label: label})
	case "delivery":
		service, version := g.llm.Service(tm.Archetype), g.llm.Version()
		events = append(events,
			reporting.Event{
				T: t.Add(-2 * time.Second), Type: "deploy.started",
				Severity: reporting.SeverityInfo, Label: "deploying " + service + " " + version,
			},
			reporting.Event{
				T: t, Type: "deploy.finished",
				Severity: reporting.SeverityInfo, Label: g.llm.DeployLabel(service, version),
			},
		)
	case "performance":
		events = append(events, reporting.Event{
			T: t, Type: "perf.recovered",
			Severity: reporting.SeverityInfo, Label: g.llm.Service(tm.Archetype) + " latency back to baseline",
		})
	default:
		label := g.llm.TaskLabel(tm.Archetype)
		typ, prefix := "task.started", "started "
		if g.rng.Float64() < 0.5 {
			typ, prefix = "task.finished", "finished "
		}
		events = append(events, reporting.Event{
			T: t, Type: typ,
			Severity: reporting.SeverityInfo, Label: prefix + label,
		})
	}
	return EventBatch{Definition: def, Scope: tm.Scope(), Events: events}
}

func (g *Generator) newLiveIncident(t time.Time) *incident {
	svc := g.co.ServiceTeams()
	ti := svc[g.rng.Intn(len(svc))]
	inc := &incident{
		teamIdx: ti,
		service: g.llm.Service(g.co.Teams[ti].Archetype),
		cause:   g.llm.Cause(g.co.Teams[ti].Archetype),
		opened:  t,
	}
	inc.summary = g.llm.IncidentSummary(inc.service, inc.cause)
	return inc
}
