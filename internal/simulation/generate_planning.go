package simulation

import (
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"
	"sort"
	"time"

	"github.com/acumen-ai-org/robotdreams/pkg/reporting"
)

func init() {
	registerScope("plan", 12*time.Hour, buildPlan)
	registerTeam("task-board", everyShift, buildTaskBoard)
	registerScope("strategy", everyWeek, buildStrategy)
	registerTeam("backlog", everyDay, buildBacklog)
	registerTeam("todo", everyDay, buildTodo)
}

func keyedSeed(key string) int64 {
	h := fnv.New64a()
	fmt.Fprint(h, key)
	return int64(h.Sum64() & 0x7fffffffffffffff)
}

func (g *Generator) planRNG(def, scope string) *rand.Rand {
	return rand.New(rand.NewSource(keyedSeed(fmt.Sprintf("%d/%s/%s", g.seed, def, scope))))
}

type planSource struct{ r *rand.Rand }

func (g *Generator) planSource(def, scope string, t time.Time) *planSource {
	seed := keyedSeed(fmt.Sprintf("%d/%s/%s/%d", g.seed, def, scope, t.Unix()))
	return &planSource{r: rand.New(rand.NewSource(seed))}
}

func (p *planSource) intn(n int) int { return p.r.Intn(n) }

func (g *Generator) planLLM(def, scope string) *FakeLLM {
	return NewFakeLLM(keyedSeed(fmt.Sprintf("llm/%d/%s/%s", g.seed, def, scope)), g.co.Vocab)
}

func (g *Generator) tickLLM(def, scope string, t time.Time) *FakeLLM {
	return NewFakeLLM(keyedSeed(fmt.Sprintf("llm/%d/%s/%s/%d", g.seed, def, scope, t.Unix())), g.co.Vocab)
}

func (p *planSource) between(lo, hi float64) float64 { return lo + p.r.Float64()*(hi-lo) }

func (p *planSource) minutes(lo, hi float64) time.Duration {
	return time.Duration(p.between(lo, hi)) * time.Minute
}

const (
	doneLingers       = 0.6
	bornInWindowShare = 0.25
)

type planCard struct {
	id               string
	title            string
	lane             string
	owner            string
	size             float64
	born             time.Time
	pace             float64
	sticksBeforeDone bool
	blocksInFlight   bool
	horizon          int
}

func (g *Generator) planDeck(def, scope string, n int, lanes []string, arch Archetype, spreadDays float64) []planCard {
	r := g.planRNG(def, scope)
	llm := g.planLLM(def, scope)
	cards := make([]planCard, 0, n)
	for i := 0; i < n; i++ {
		lane := ""
		if len(lanes) > 0 {
			lane = lanes[r.Intn(len(lanes))]
		}
		age := time.Duration((r.Float64()*(1+bornInWindowShare) - bornInWindowShare) * spreadDays * float64(24*time.Hour))
		cards = append(cards, planCard{
			id:               fmt.Sprintf("%s-%03d", shortDef(def), i+1),
			title:            llm.ItemTitle(arch),
			lane:             lane,
			owner:            llm.Owner(),
			size:             float64(1 + r.Intn(8)),
			born:             g.start.Add(-age),
			pace:             0.25 + r.Float64()*0.75,
			sticksBeforeDone: r.Float64() < 0.22,
			blocksInFlight:   r.Float64() < 0.18,
			horizon:          r.Intn(3),
		})
	}
	return cards
}

func shortDef(def string) string {
	switch def {
	case "task-board":
		return "TB"
	case "backlog":
		return "BL"
	case "strategy":
		return "OKR"
	case "todo":
		return "TD"
	default:
		return "PLN"
	}
}

func (g *Generator) planItems(cards []planCard, states []string, t time.Time, flowDays float64) []reporting.Item {
	if len(states) == 0 {
		return nil
	}
	last := len(states) - 1
	items := make([]reporting.Item, 0, len(cards))
	blocked := make([]bool, 0, len(cards))
	for _, c := range cards {
		pos, idx := c.flowPositionAt(t, flowDays, last)
		if agedOffBoard(idx, pos, last) {
			continue
		}
		if t.Before(c.born) {
			continue
		}
		it := reporting.Item{
			ID:    c.id,
			Title: c.title,
			State: states[idx],
			Lane:  c.lane,
			Owner: c.owner,
			Size:  c.size,
		}
		blocked = append(blocked, c.blocksInFlight && idx > 0 && idx < last)
		items = append(items, it)
	}
	blockOnPreviousSurvivor(items, blocked)
	return items
}

func (c planCard) flowPositionAt(t time.Time, flowDays float64, last int) (pos float64, idx int) {
	days := t.Sub(c.born).Hours() / 24
	pos = (days / flowDays) * c.pace
	idx = int(math.Floor(pos * float64(last)))
	if idx < 0 {
		idx = 0
	}
	if idx > last {
		idx = last
	}
	if c.sticksBeforeDone && idx >= last {
		idx = last - 1
		if idx < 0 {
			idx = 0
		}
	}
	return pos, idx
}

func agedOffBoard(idx int, pos float64, last int) bool {
	return idx >= last && pos > 1+doneLingers/float64(last)
}

func blockOnPreviousSurvivor(items []reporting.Item, blocked []bool) {
	for i := range items {
		if blocked[i] && i > 0 {
			items[i].BlockedBy = []string{items[i-1].ID}
		}
	}
}

func countBy(items []reporting.Item, state string) int {
	n := 0
	for _, it := range items {
		if it.State == state {
			n++
		}
	}
	return n
}

func countBlocked(items []reporting.Item) int {
	n := 0
	for _, it := range items {
		if len(it.BlockedBy) > 0 {
			n++
		}
	}
	return n
}

var (
	taskBoardStates = []string{"proposed", "committed", "in_progress", "review", "done"}
	taskBoardLanes  = []string{"feature", "defect", "chore"}
)

const overWIPEveryNthTeam = 7

func buildTaskBoard(g *Generator, ti int, tm Team, t time.Time) Submission {
	src := g.planSource("task-board", tm.Scope(), t)
	cards := g.planDeck("task-board", tm.Scope(), 22+ti%8, taskBoardLanes, tm.Archetype, 16)
	items := g.planItems(cards, taskBoardStates, t, 7)

	overWIP := ti%overWIPEveryNthTeam == 0
	if overWIP {
		pullEvenDeckCardsIntoProgress(items, cards)
	}

	for i := range items {
		items[i].Commitment = "committed"
		items[i].Level = "task"
		if items[i].State == "proposed" {
			items[i].Commitment = "planned"
		}
		if d := t.Add(time.Duration(6+i%20) * 24 * time.Hour); items[i].State != "done" {
			items[i].Due = &d
		}
	}

	inFlight := countBy(items, "committed") + countBy(items, "in_progress") + countBy(items, "review")
	blocked := countBlocked(items)
	done := countBy(items, "done")
	cycle := math.Round((4+src.between(0, 4)+float64(blocked))*10) / 10

	var events []reporting.Event
	for i, it := range items {
		if len(it.BlockedBy) > 0 && i < 3 {
			events = append(events, reporting.Event{
				T: t.Add(-src.minutes(20, 200)), Type: "item.blocked", Severity: reporting.SeverityWarn,
				Label: "blocked: " + it.Title,
			})
		}
	}
	if done > 0 {
		events = append(events, reporting.Event{
			T: t.Add(-src.minutes(10, 180)), Type: "item.delivered", Severity: reporting.SeverityInfo,
			Label: "delivered: " + items[0].Title,
		})
	}
	if overWIP {
		events = append(events, reporting.Event{
			T: t.Add(-src.minutes(5, 90)), Type: "wip.exceeded", Severity: reporting.SeverityWarn,
			Label: "work in progress over the limit in in_progress",
		})
	}

	var blockedRows [][]string
	for _, it := range items {
		if len(it.BlockedBy) > 0 && len(blockedRows) < 5 {
			blockedRows = append(blockedRows, []string{
				it.Title, it.Owner, fmt.Sprintf("%d", 1+src.intn(9)), it.BlockedBy[0],
			})
		}
	}
	byOwner := map[string][2]int{}
	for _, it := range items {
		e := byOwner[it.Owner]
		if it.State == "done" {
			e[1]++
		} else {
			e[0]++
		}
		byOwner[it.Owner] = e
	}
	owners := make([]string, 0, len(byOwner))
	for o := range byOwner {
		owners = append(owners, o)
	}
	sort.Strings(owners)
	var ownerRows [][]string
	for _, o := range owners {
		ownerRows = append(ownerRows, []string{o, fmt.Sprintf("%d", byOwner[o][0]), fmt.Sprintf("%d", byOwner[o][1])})
	}

	in := tm.instance("task-board", t,
		map[string]float64{
			"items_in_flight": float64(inFlight),
			"items_blocked":   float64(blocked),
			"items_done":      float64(done),
			"cycle_time_days": cycle,
		},
		map[string]float64{"items_blocked": 0, "cycle_time_days": 5},
	)
	in.Items = items
	in.Events = events
	in.Series = map[string][]reporting.SeriesPoint{
		"remaining_trend": g.burndown(t, len(items)-done),
		"done_trend":      g.countSeries(t, done),
	}
	in.Tables = map[string]reporting.Table{
		"blocked":  {Columns: []string{"item", "owner", "blocked_days", "waiting_on"}, Rows: blockedRows},
		"by_owner": {Columns: []string{"owner", "in_flight", "done"}, Rows: ownerRows},
	}
	return tm.submit(in)
}

func pullEvenDeckCardsIntoProgress(items []reporting.Item, cards []planCard) {
	ordinal := make(map[string]int, len(cards))
	for i, c := range cards {
		ordinal[c.id] = i
	}
	for i := range items {
		if items[i].State == "committed" && ordinal[items[i].ID]%2 == 0 {
			items[i].State = "in_progress"
		}
	}
}

func (g *Generator) burndown(t time.Time, remaining int) []reporting.SeriesPoint {
	pts := make([]reporting.SeriesPoint, 0, 14)
	for i := 13; i >= 0; i-- {
		at := t.Add(-time.Duration(i) * 12 * time.Hour)
		v := float64(remaining) + float64(i)*1.4
		pts = append(pts, reporting.SeriesPoint{T: at, V: math.Round(v)})
	}
	return pts
}

func (g *Generator) countSeries(t time.Time, latest int) []reporting.SeriesPoint {
	pts := make([]reporting.SeriesPoint, 0, 14)
	for i := 13; i >= 0; i-- {
		at := t.Add(-time.Duration(i) * 12 * time.Hour)
		pts = append(pts, reporting.SeriesPoint{T: at, V: math.Max(0, float64(latest)-float64(i)*0.4)})
	}
	return pts
}

var (
	planStates   = []string{"proposed", "committed", "in_progress", "done"}
	planLanes    = []string{"product", "platform", "operations", "compliance"}
	planHorizons = []string{"now", "next", "later"}
)

func buildPlan(g *Generator, scope, producer string, t time.Time) Submission {
	src := g.planSource("plan", scope, t)
	llm := g.tickLLM("plan", scope, t)
	cards := g.planDeck("plan", scope, 16, planLanes, ArchStrategy, 70)
	items := g.planItems(cards, planStates, t, 45)

	slipped := 0
	for i := range items {
		c := cards[i]
		h := c.horizon
		if slipsAt(c, t) && items[i].State != "done" {
			h++
			slipped++
		}
		if h >= len(planHorizons) {
			h = len(planHorizons) - 1
		}
		items[i].Horizon = planHorizons[h]
		items[i].Level = []string{"initiative", "workstream", "activity"}[i%3]
		switch items[i].State {
		case "proposed":
			items[i].Commitment = "candidate"
		case "committed":
			items[i].Commitment = "committed"
		default:
			items[i].Commitment = "planned"
		}
		if items[i].State != "done" {
			d := t.Add(time.Duration(10+i*5) * 24 * time.Hour)
			items[i].Due = &d
		}
	}

	committed := len(items) - countBy(items, "done")
	effort := 0.0
	for _, it := range items {
		effort += it.Size
	}
	confidence := math.Round(math.Max(35, 92-float64(slipped)*7))

	events := []reporting.Event{
		{
			T: t.Add(-src.minutes(60, 600)), Type: "plan.committed", Severity: reporting.SeverityInfo,
			Label: "committed: " + items[0].Title,
		},
	}
	if slipped > 0 {
		events = append(events, reporting.Event{
			T: t.Add(-src.minutes(20, 300)), Type: "plan.rescheduled", Severity: reporting.SeverityWarn,
			Label: fmt.Sprintf("%d item(s) moved out a horizon around %s", slipped, llm.Cause(ArchStrategy)),
		})
	}
	if n := countBy(items, "done"); n > 0 {
		events = append(events, reporting.Event{
			T: t.Add(-src.minutes(10, 240)), Type: "item.delivered", Severity: reporting.SeverityInfo,
			Label: "delivered: " + items[len(items)-1].Title,
		})
	}
	if b := countBlocked(items); b > 0 {
		events = append(events, reporting.Event{
			T: t.Add(-src.minutes(5, 180)), Type: "item.blocked", Severity: reporting.SeverityWarn,
			Label: "blocked: " + items[0].Title,
		})
	}

	byHorizon := map[string][2]float64{}
	for _, it := range items {
		e := byHorizon[it.Horizon]
		e[0]++
		e[1] += it.Size
		byHorizon[it.Horizon] = e
	}
	var horizonRows [][]string
	for _, h := range planHorizons {
		e := byHorizon[h]
		horizonRows = append(horizonRows, []string{h, fmt.Sprintf("%.0f", e[0]), fmt.Sprintf("%.0f", e[1])})
	}
	var slipRows [][]string
	for i, it := range items {
		if slipsAt(cards[i], t) && it.State != "done" && len(slipRows) < 5 {
			was := t.Add(time.Duration(4+i) * 24 * time.Hour)
			slipRows = append(slipRows, []string{
				it.Title, it.Owner, was.Format("2006-01-02"), it.Due.Format("2006-01-02"),
				llm.Cause(ArchStrategy),
			})
		}
	}

	return Submission{
		Producer: producer,
		Instance: &reporting.Instance{
			Definition: "plan",
			Scope:      scope,
			Producer:   producer,
			ProducedAt: t,
			KPIs: map[string]float64{
				"items_committed":  float64(committed),
				"items_slipped":    float64(slipped),
				"effort_committed": effort,
				"confidence_pct":   confidence,
			},
			Targets: map[string]float64{"items_slipped": 3, "confidence_pct": 70},
			Items:   items,
			Events:  events,
			Series: map[string][]reporting.SeriesPoint{
				"committed_trend": g.countSeries(t, committed),
			},
			Tables: map[string]reporting.Table{
				"by_horizon": {Columns: []string{"horizon", "items", "effort_days"}, Rows: horizonRows},
				"slipped":    {Columns: []string{"item", "owner", "was_due", "now_due", "reason"}, Rows: slipRows},
			},
		},
	}
}

const (
	holdsDatePace = 0.55
	slipsAfter    = 21 * 24 * time.Hour
)

func slipsAt(c planCard, t time.Time) bool {
	if c.pace > holdsDatePace {
		return false
	}
	return t.Sub(c.born) > slipsAfter
}

var (
	strategyStates   = []string{"proposed", "committed", "in_progress", "done"}
	strategyHorizons = []string{"this_quarter", "next_quarter", "this_year", "next_year"}
)

func buildStrategy(g *Generator, scope, producer string, t time.Time) Submission {
	src := g.planSource("strategy", scope, t)
	llm := g.tickLLM("strategy", scope, t)
	deck := g.planLLM("strategy", scope)
	r := g.planRNG("strategy", scope)
	n := 4 + r.Intn(3)
	items := make([]reporting.Item, 0, n)
	var objRows, offRows [][]string

	offTrack := 0
	attainSum := 0.0
	for i := 0; i < n; i++ {
		obj := deck.Objective(ArchStrategy)
		att := math.Round(35 + r.Float64()*60)
		attainSum += att
		state := strategyStates[minInt(3, int((att/100)*4))]
		horizon := strategyHorizons[r.Intn(len(strategyHorizons))]
		owner := deck.Owner()
		commitment := "committed"
		if state == "proposed" {
			commitment = "candidate"
		}
		items = append(items, reporting.Item{
			ID:         fmt.Sprintf("OKR-%03d", i+1),
			Title:      obj,
			State:      state,
			Owner:      owner,
			Horizon:    horizon,
			Commitment: commitment,
			Level:      "goal",
		})
		objRows = append(objRows, []string{
			obj, owner, horizon,
			fmt.Sprintf("%.0f", att), fmt.Sprintf("%d%%", 50+10*r.Intn(5)),
		})
		if att < 60 {
			offTrack++
			offRows = append(offRows, []string{
				llm.KeyResult(), obj, owner,
				fmt.Sprintf("%.0f pts", 70-att), llm.DecisionSubject(),
			})
		}
	}
	attainment := math.Round(attainSum / float64(n))
	onTrack := n - offTrack

	events := []reporting.Event{
		{
			T: t.Add(-src.minutes(240, 2400)), Type: "objective.set", Severity: reporting.SeverityInfo,
			Label: "set: " + items[0].Title,
		},
	}
	if offTrack > 0 {
		events = append(events, reporting.Event{
			T: t.Add(-src.minutes(60, 900)), Type: "key_result.off_track", Severity: reporting.SeverityWarn,
			Label: "off track: " + llm.KeyResult(),
		})
	}
	if countBy(items, "done") > 0 {
		events = append(events, reporting.Event{
			T: t.Add(-src.minutes(30, 600)), Type: "objective.met", Severity: reporting.SeverityInfo,
			Label: "met: " + items[len(items)-1].Title,
		})
	}

	return Submission{
		Producer: producer,
		Instance: &reporting.Instance{
			Definition: "strategy",
			Scope:      scope,
			Producer:   producer,
			ProducedAt: t,
			KPIs: map[string]float64{
				"objectives_active":     float64(n),
				"key_results_on_track":  float64(onTrack),
				"key_results_off_track": float64(offTrack),
				"attainment_pct":        attainment,
			},
			Targets: map[string]float64{"key_results_off_track": 2, "attainment_pct": 70},
			Items:   items,
			Events:  events,
			Series: map[string][]reporting.SeriesPoint{
				"attainment_trend": g.countSeries(t, int(attainment)),
			},
			Tables: map[string]reporting.Table{
				"objectives": {Columns: []string{"objective", "owner", "horizon", "attainment_pct", "confidence"}, Rows: objRows},
				"off_track":  {Columns: []string{"key_result", "objective", "owner", "gap", "ask"}, Rows: offRows},
			},
		},
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var (
	backlogStates = []string{"proposed", "triaged", "committed"}
	backlogLanes  = []string{"request", "defect", "question", "risk"}
)

func buildBacklog(g *Generator, ti int, tm Team, t time.Time) Submission {
	src := g.planSource("backlog", tm.Scope(), t)
	cards := g.planDeck("backlog", tm.Scope(), 26+ti%11, backlogLanes, tm.Archetype, 80)
	items := g.planItems(cards, backlogStates, t, 55)

	oldest := 0.0
	for i := range items {
		items[i].Level = "task"
		items[i].Commitment = "candidate"
		if items[i].State == "committed" {
			items[i].Commitment = "committed"
		}
		if items[i].State != "committed" {
			if d := t.Sub(cards[i].born).Hours() / 24; d > oldest {
				oldest = d
			}
		}
	}
	oldest = math.Round(oldest)

	waiting := len(items) - countBy(items, "committed")
	arrived := 2 + src.intn(7)
	triaged := 1 + src.intn(6)

	events := []reporting.Event{
		{
			T: t.Add(-src.minutes(30, 600)), Type: "item.arrived", Severity: reporting.SeverityInfo,
			Label: "arrived: " + items[0].Title,
		},
		{
			T: t.Add(-src.minutes(20, 400)), Type: "item.triaged", Severity: reporting.SeverityInfo,
			Label: "triaged: " + items[len(items)-1].Title,
		},
	}
	if oldest > 30 {
		events = append(events, reporting.Event{
			T: t.Add(-src.minutes(10, 200)), Type: "item.aged_out", Severity: reporting.SeverityWarn,
			Label: fmt.Sprintf("waiting %d days without triage", int(oldest)),
		})
	}

	buckets := []struct {
		label string
		max   float64
	}{{"0-7d", 7}, {"8-30d", 30}, {"31-90d", 90}, {"90d+", math.MaxFloat64}}
	counts := make([]int, len(buckets))
	for i, it := range items {
		if it.State == "committed" {
			continue
		}
		age := t.Sub(cards[i].born).Hours() / 24
		for b, bk := range buckets {
			if age <= bk.max {
				counts[b]++
				break
			}
		}
	}
	var bucketRows [][]string
	for i, bk := range buckets {
		bucketRows = append(bucketRows, []string{bk.label, fmt.Sprintf("%d", counts[i])})
	}

	type aged struct {
		it   reporting.Item
		days float64
	}
	var ageList []aged
	for i, it := range items {
		if it.State != "committed" {
			ageList = append(ageList, aged{it, t.Sub(cards[i].born).Hours() / 24})
		}
	}
	sort.SliceStable(ageList, func(i, j int) bool { return ageList[i].days > ageList[j].days })
	var oldestRows [][]string
	for _, a := range ageList {
		if len(oldestRows) >= 5 {
			break
		}
		oldestRows = append(oldestRows, []string{a.it.Title, a.it.Owner, fmt.Sprintf("%.0f", a.days), a.it.Lane})
	}

	in := tm.instance("backlog", t,
		map[string]float64{
			"items_waiting": float64(waiting),
			"items_arrived": float64(arrived),
			"items_triaged": float64(triaged),
			"oldest_days":   oldest,
		},
		map[string]float64{"oldest_days": 30},
	)
	in.Items = items
	in.Events = events
	in.Series = map[string][]reporting.SeriesPoint{"waiting_trend": g.countSeries(t, waiting)}
	in.Tables = map[string]reporting.Table{
		"age_buckets": {Columns: []string{"age", "items"}, Rows: bucketRows},
		"oldest":      {Columns: []string{"item", "requested_by", "waiting_days", "lane"}, Rows: oldestRows},
	}
	return tm.submit(in)
}

func buildTodo(g *Generator, ti int, tm Team, t time.Time) Submission {
	src := g.planSource("todo", tm.Scope(), t)
	cards := g.planDeck("todo", tm.Scope(), 9+ti%5, nil, tm.Archetype, 14)
	items := g.planItems(cards, []string{"committed", "done"}, t, 9)
	for i := range items {
		items[i].Level = "task"
		items[i].BlockedBy = nil
	}

	done := countBy(items, "done")
	open := len(items) - done

	events := []reporting.Event{
		{
			T: t.Add(-src.minutes(30, 700)), Type: "item.added", Severity: reporting.SeverityInfo,
			Label: "added: " + items[0].Title,
		},
	}
	if done > 0 {
		events = append(events, reporting.Event{
			T: t.Add(-src.minutes(10, 300)), Type: "item.done", Severity: reporting.SeverityInfo,
			Label: "done: " + items[len(items)-1].Title,
		})
	}

	in := tm.instance("todo", t,
		map[string]float64{"items_open": float64(open), "items_done": float64(done)}, nil)
	in.Items = items
	in.Events = events
	return tm.submit(in)
}
