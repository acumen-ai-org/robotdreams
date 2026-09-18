package simulation

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/acumen-ai-org/robotdreams/pkg/reporting"
)

func init() { registerTeam("workflow", everyShift, buildWorkflow) }

type WorkflowTemplate struct {
	ID             string
	Name           string
	DoneWhen       string
	Owner          string
	Steps          []WorkflowStep
	TypicalMinutes float64
	BlockedStep    string
	BlockedOn      string
	BlockedFor     time.Duration
}

type WorkflowStep struct {
	ID        string
	Name      string
	DependsOn []string
	Owner     string
}

func (g *Generator) workflowTemplates(tm Team) []WorkflowTemplate {
	if g.co.Workflows != nil {
		if tpls, ok := g.co.Workflows[tm.Scope()]; ok && len(tpls) > 0 {
			return tpls
		}
	}
	return genericWorkflows(tm)
}

func genericWorkflows(tm Team) []WorkflowTemplate {
	return []WorkflowTemplate{
		{
			ID: "intake", Name: "Intake and triage",
			DoneWhen: "the request is routed or closed", Owner: tm.Name,
			TypicalMinutes: 180,
			Steps: []WorkflowStep{
				{ID: "receive", Name: "Receive request"},
				{ID: "assess", Name: "Assess and categorise", DependsOn: []string{"receive"}},
				{ID: "route", Name: "Route to an owner", DependsOn: []string{"assess"}},
				{ID: "confirm", Name: "Confirm with the requester", DependsOn: []string{"route"}},
			},
		},
		{
			ID: "review-cycle", Name: "Review cycle",
			DoneWhen: "the work is accepted or sent back with reasons", Owner: tm.Name,
			TypicalMinutes: 420,
			Steps: []WorkflowStep{
				{ID: "prepare", Name: "Prepare the work"},
				{ID: "review", Name: "Review", DependsOn: []string{"prepare"}},
				{ID: "revise", Name: "Revise", DependsOn: []string{"review"}},
				{ID: "accept", Name: "Accept", DependsOn: []string{"revise"}},
			},
		},
	}
}

func buildWorkflow(g *Generator, ti int, tm Team, t time.Time) Submission {
	tpls := g.workflowTemplates(tm)
	day := g.season(t, tm, 0.45)
	load := math.Max(1, float64(tm.Headcount)/3)

	started := math.Round(g.between(0.6, 2.2) * load * day)
	carried := math.Round(load * g.between(0.4, 1.4))
	instantiated := started + carried

	begun := math.Round(instantiated * g.between(0.88, 1.0))
	halfway := math.Round(begun * g.between(0.72, 0.94))
	finished := math.Round(halfway * g.between(0.75, 0.96))
	completed := math.Round(finished * g.between(0.88, 1.0))

	var blockedRows [][]string
	blocked := 0.0
	for _, tpl := range tpls {
		if tpl.BlockedStep == "" {
			continue
		}
		blocked++
		since := tpl.BlockedFor
		if since == 0 {
			since = g.minutes(90, 2400)
		}
		blockedRows = append(blockedRows, []string{
			humanSince(since), tpl.Name, stepName(tpl, tpl.BlockedStep), tpl.BlockedOn, tm.Name,
		})
	}
	for extra := g.rng.Intn(3); extra > 0; extra-- {
		tpl := tpls[g.rng.Intn(len(tpls))]
		step := tpl.Steps[g.rng.Intn(len(tpl.Steps))]
		blocked++
		blockedRows = append(blockedRows, []string{
			humanSince(g.minutes(45, 1800)), tpl.Name, step.Name, g.llm.Owner(), tm.Name,
		})
	}

	active := math.Max(0, instantiated-completed)
	stepRate := math.Round(g.between(88, 99.4)*10) / 10
	medianRun := math.Round(tpls[0].TypicalMinutes * g.between(0.8, 1.3))

	tpl := tpls[g.rng.Intn(len(tpls))]
	stepItems := walkSteps(tpl)
	stepRows := stepRowsOf(stepItems)

	tplRows := make([][]string, 0, len(tpls))
	for _, x := range tpls {
		runs := math.Round(started / float64(len(tpls)) * g.between(0.7, 1.3))
		tplRows = append(tplRows, []string{
			x.Name,
			fmt.Sprintf("%.0f", runs),
			fmt.Sprintf("%.0f", math.Round(x.TypicalMinutes*g.between(0.85, 1.2))),
			fmt.Sprintf("%.1f", g.between(78, 99)),
		})
	}

	in := tm.instance("workflow", t,
		map[string]float64{
			"runs_started":         started,
			"runs_completed":       completed,
			"runs_active":          active,
			"runs_blocked":         blocked,
			"median_run_minutes":   medianRun,
			"step_completion_rate": stepRate,
		},
		map[string]float64{
			"runs_blocked":         0,
			"median_run_minutes":   240,
			"step_completion_rate": 95,
		})
	in.Series = map[string][]reporting.SeriesPoint{
		"runs_completed_trend": g.series(t, 8, 4*time.Hour, completed+1, 0.5),
		"blocked_runs_trend":   g.series(t, 8, 4*time.Hour, blocked+0.5, 0.6),
	}
	in.Items = stepItems
	in.Events = g.workflowEvents(t, tpls, int(completed), blockedRows)
	in.Tables = map[string]reporting.Table{
		"step_stages": funnel([][2]interface{}{
			{"instantiated", instantiated},
			{"started", begun},
			{"past halfway", halfway},
			{"all steps done", finished},
			{"done_when met", completed},
		}),
		"blocked_runs": {
			Columns: []string{"since", "run", "step", "waiting_on", "capability"},
			Rows:    blockedRows,
		},
		"steps":        {Columns: []string{"step", "status", "depends_on", "owner", "duration"}, Rows: stepRows},
		"templates":    {Columns: []string{"template", "runs", "median_minutes", "completion_rate"}, Rows: tplRows},
		"runs_by_unit": byUnit("runs", tm, started),
	}
	return tm.submit(in)
}

func (g *Generator) workflowEvents(t time.Time, tpls []WorkflowTemplate, completed int, blockedRows [][]string) []reporting.Event {
	var out []reporting.Event
	show := completed
	if show > 3 {
		show = 3
	}
	for i := 0; i < show; i++ {
		tpl := tpls[i%len(tpls)]
		dur := time.Duration(tpl.TypicalMinutes) * time.Minute
		created := t.Add(-dur - g.minutes(5, 90))
		out = append(out,
			reporting.Event{
				T: created, Type: "workflow.created", Severity: reporting.SeverityInfo,
				Label: tpl.Name + " started — " + tpl.DoneWhen,
			},
		)
		for si, step := range tpl.Steps {
			at := created.Add(time.Duration(float64(si+1) / float64(len(tpl.Steps)+1) * float64(dur)))
			out = append(out,
				reporting.Event{T: at, Type: "step.started", Severity: reporting.SeverityInfo, Label: step.Name},
				reporting.Event{T: at.Add(g.minutes(3, 40)), Type: "step.completed", Severity: reporting.SeverityInfo, Label: step.Name + " done"},
			)
		}
		out = append(out, reporting.Event{
			T: created.Add(dur), Type: "workflow.completed", Severity: reporting.SeverityInfo,
			Label: tpl.Name + " reached done_when",
		})
	}
	for _, row := range blockedRows {
		out = append(out, reporting.Event{
			T: t.Add(-g.minutes(10, 600)), Type: "step.blocked", Severity: reporting.SeverityWarn,
			Label: row[1] + " blocked at " + row[2] + ", waiting on " + row[3],
		})
	}
	return out
}

func walkSteps(tpl WorkflowTemplate) []reporting.Item {
	blockedAt := tpl.BlockedStep
	stalled := stalledBehind(tpl, blockedAt)
	per := tpl.TypicalMinutes / float64(len(tpl.Steps))
	items := make([]reporting.Item, 0, len(tpl.Steps))
	for _, s := range tpl.Steps {
		state := "done"
		switch {
		case s.ID == blockedAt:
			state = "in_progress"
		case stalled[s.ID]:
			state = "committed"
		}
		owner := s.Owner
		if owner == "" {
			owner = tpl.Owner
		}
		it := reporting.Item{
			ID:         s.ID,
			Title:      s.Name,
			State:      state,
			Owner:      owner,
			Level:      "task",
			Commitment: "committed",
			BlockedBy:  append([]string(nil), s.DependsOn...),
		}
		if state == "done" {
			it.Size = per
		}
		items = append(items, it)
	}
	return items
}

func stalledBehind(tpl WorkflowTemplate, blockedAt string) map[string]bool {
	stalled := map[string]bool{}
	if blockedAt == "" {
		return stalled
	}
	stalled[blockedAt] = true
	for again := true; again; {
		again = false
		for _, s := range tpl.Steps {
			if stalled[s.ID] {
				continue
			}
			for _, dep := range s.DependsOn {
				if stalled[dep] {
					stalled[s.ID] = true
					again = true
				}
			}
		}
	}
	return stalled
}

func stepRowsOf(items []reporting.Item) [][]string {
	rows := make([][]string, 0, len(items))
	for _, it := range items {
		dur := "—"
		if it.Size > 0 {
			dur = fmt.Sprintf("%.0fm", it.Size)
		}
		rows = append(rows, []string{it.Title, it.State, strings.Join(it.BlockedBy, ", "), it.Owner, dur})
	}
	return rows
}

func stepName(tpl WorkflowTemplate, id string) string {
	for _, s := range tpl.Steps {
		if s.ID == id {
			return s.Name
		}
	}
	return id
}

func humanSince(d time.Duration) string {
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
