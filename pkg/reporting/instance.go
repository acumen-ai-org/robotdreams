package reporting

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// SeriesPoint is one sample in a time series.
type SeriesPoint struct {
	T time.Time `json:"t"`
	V float64   `json:"v"`
}

// Table is a rendered table payload: column headers plus rows of cells.
type Table struct {
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
}

// Event is one timeline/pulse event. Scope and Definition are origin tags
// filled in when events from many instances are merged into one stream.
type Event struct {
	T          time.Time         `json:"t"`
	Type       string            `json:"type"`
	Severity   string            `json:"severity"`
	Label      string            `json:"label"`
	Scope      string            `json:"scope,omitempty"`
	Definition string            `json:"definition,omitempty"`
	Attrs      map[string]string `json:"attrs,omitempty"`
}

// Item is one piece of intended work: a card on a board, a row on a
// roadmap, a line on a checklist. Items are REPORTED, never authored — a
// node publishes the plan it holds, the control plane stores and rolls it
// up, and nothing here follows an item from one instance to the next.
//
// Four absences are the design, not gaps in it:
//
//   - No parent link. That field is where a work hierarchy would sneak
//     back in. Level is a flat vocabulary LABEL (see WorkLevels), so
//     calling an item a "goal" says how to read it; it does not create a
//     goal, nor anything the goal contains.
//   - No identity across instances. ID exists so BlockedBy has something
//     to point at within the same payload. Keeping ids stable between
//     publications is the producer's business and nothing reads it as a
//     key; a card present at T and absent at T+1 is simply not planned
//     any more, and no transition is derived from that.
//   - No status. A card is not a service. Blocked-ness and Due are the
//     two exception signals, and both are already here.
//   - No created-at. History is the timeline facet's job. A plan facet
//     that grew timestamps would be a task store.
//
// See docs/vision/hierarchy.md, which resolved that the Go build models
// no work hierarchy: reporting a board is how that stays true.
//
// Scope and Definition are origin tags, filled in when items from many
// instances are merged into one board, exactly as they are on Event. Two
// teams may legitimately publish the same ID, so an item — and every
// BlockedBy edge — resolves within one Scope.
type Item struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// State is one of the plan facet's declared states. The declared
	// ORDER is the semantics: left to right is progress, and the last
	// state is terminal.
	State string `json:"state"`
	Lane  string `json:"lane,omitempty"`
	Owner string `json:"owner,omitempty"`
	// Due is a pointer because committed-but-unscheduled work is the most
	// common real plan, and a zero time renders as year 1.
	Due *time.Time `json:"due,omitempty"`
	// Size is whatever the plan counts work in; the unit is declared once
	// on the facet as size_unit, never per item.
	Size float64 `json:"size,omitempty"`
	// BlockedBy names other items in THIS instance. After a roll-up an
	// edge may no longer resolve, because the item it points at belongs
	// to a sibling scope or was dropped by a bound — that is a fact about
	// the plan (the blocking work is somebody else's), not an error at
	// the reader, so it is never rewritten.
	BlockedBy  []string `json:"blocked_by,omitempty"`
	Horizon    string   `json:"horizon,omitempty"`
	Commitment string   `json:"commitment,omitempty"`
	Level      string   `json:"level,omitempty"`
	Scope      string   `json:"scope,omitempty"`
	Definition string   `json:"definition,omitempty"`
}

// Blocked reports whether anything is holding this item up.
func (it Item) Blocked() bool { return len(it.BlockedBy) > 0 }

// Instance is the JSON wire payload a node submits: one report's data for
// one scope at one point in time. Payload names must match the
// definition's data contract; see ValidateInstance.
type Instance struct {
	Definition string    `json:"definition"`
	Scope      string    `json:"scope"`
	Producer   string    `json:"producer,omitempty"`
	ProducedAt time.Time `json:"produced_at"`
	Status     string    `json:"status,omitempty"`   // optional override
	Headline   string    `json:"headline,omitempty"` // optional precomputed

	KPIs map[string]float64 `json:"kpis,omitempty"`
	// Targets carries this scope's target for any KPI, overriding the
	// definition-level KPISpec.Target. Per-scope because a target is a
	// local commitment: the Nordics' churn target is not India's, and a
	// mature squad's latency budget is not a new one's. Keys validate
	// against the same data contract as KPIs.
	Targets map[string]float64       `json:"targets,omitempty"`
	Series  map[string][]SeriesPoint `json:"series,omitempty"`
	Tables  map[string]Table         `json:"tables,omitempty"`
	Events  []Event                  `json:"events,omitempty"`
	// Items is the plan: intended work, one card each, legal only where
	// the definition exposes a plan facet. A whole-plan snapshot —
	// submitting an instance replaces the plan for that (definition,
	// scope), the same way KPIs replace KPIs. A producer that changes its
	// plan republishes it; there is no endpoint that moves a card.
	Items []Item `json:"items,omitempty"`
}

// ErrInvalidInstance is returned by ValidateInstance when an instance
// fails validation. The returned error wraps the individual problems
// (joined with errors.Join), so every issue is reported at once.
var ErrInvalidInstance = errors.New("reporting: invalid report instance")

// ValidateInstance checks an instance against its definition's data
// contract. Validation is exhaustive rather than fail-fast: missing
// scope/definition/produced_at, a definition name that does not match, a
// bad status override, and every unknown kpi/series/table name, unknown
// event type or bad event severity are all collected and returned together
// in a single error wrapping ErrInvalidInstance.
func ValidateInstance(def *Definition, inst *Instance) error {
	if def == nil || inst == nil {
		return fmt.Errorf("%w: nil definition or instance", ErrInvalidInstance)
	}

	var problems []error

	if inst.Definition == "" {
		problems = append(problems, errors.New("definition: empty"))
	} else if inst.Definition != def.Name {
		problems = append(problems, fmt.Errorf("definition: %q does not match definition %q", inst.Definition, def.Name))
	}
	if inst.Scope == "" {
		problems = append(problems, errors.New("scope: empty"))
	}
	if inst.ProducedAt.IsZero() {
		problems = append(problems, errors.New("produced_at: missing"))
	}
	if inst.Status != "" && !contains(Statuses, inst.Status) {
		problems = append(problems, fmt.Errorf("status: %q not one of ok|warn|critical", inst.Status))
	}

	kpis := map[string]bool{}
	for _, k := range def.Data.KPIs {
		kpis[k.Name] = true
	}
	series := map[string]bool{}
	for _, s := range def.Data.Series {
		series[s.Name] = true
	}
	tables := map[string]bool{}
	for _, t := range def.Data.Tables {
		tables[t.Name] = true
	}
	events := map[string]bool{}
	for _, e := range def.Data.Events {
		events[e.Type] = true
	}

	for _, name := range sortedFloatKeys(inst.KPIs) {
		if !kpis[name] {
			problems = append(problems, fmt.Errorf("kpis[%s]: no such kpi in data contract", name))
		}
	}
	for _, name := range sortedFloatKeys(inst.Targets) {
		if !kpis[name] {
			problems = append(problems, fmt.Errorf("targets[%s]: no such kpi in data contract", name))
		}
	}
	seriesNames := make([]string, 0, len(inst.Series))
	for name := range inst.Series {
		seriesNames = append(seriesNames, name)
	}
	sort.Strings(seriesNames)
	for _, name := range seriesNames {
		if !series[name] {
			problems = append(problems, fmt.Errorf("series[%s]: no such series in data contract", name))
		}
	}
	tableNames := make([]string, 0, len(inst.Tables))
	for name := range inst.Tables {
		tableNames = append(tableNames, name)
	}
	sort.Strings(tableNames)
	for _, name := range tableNames {
		if !tables[name] {
			problems = append(problems, fmt.Errorf("tables[%s]: no such table in data contract", name))
		}
	}
	for i, e := range inst.Events {
		if !events[e.Type] {
			problems = append(problems, fmt.Errorf("events[%d]: no such event type %q in data contract", i, e.Type))
		}
		if !contains(Severities, e.Severity) {
			problems = append(problems, fmt.Errorf("events[%d]: severity %q not one of info|warn|critical", i, e.Severity))
		}
	}

	problems = append(problems, validateItems(def, inst)...)

	if len(problems) > 0 {
		return fmt.Errorf("%w: %w", ErrInvalidInstance, errors.Join(problems...))
	}
	return nil
}

// validateItems checks a plan payload against the definition's plan facet.
// Exhaustive like the rest of ValidateInstance: every bad card is reported,
// not just the first.
//
// The load-bearing rule is that every State must be one the facet declared.
// It is what guarantees a rolled-up board's columns are exhaustive — an
// undeclared state would otherwise vanish into no column at all, and a card
// silently missing from a plan is the worst failure this layer has.
func validateItems(def *Definition, inst *Instance) []error {
	if len(inst.Items) == 0 {
		return nil
	}
	facet := def.Facets.Plan
	if facet == nil {
		return []error{fmt.Errorf("items: definition %q declares no plan facet", def.Name)}
	}

	var problems []error
	ids := make(map[string]bool, len(inst.Items))
	for _, it := range inst.Items {
		if it.ID != "" {
			ids[it.ID] = true
		}
	}

	// inVocab reports a value that is not in a declared list, distinguishing
	// "not one of these" from "this plan declares none at all" — the second
	// is a different mistake and deserves to say so.
	inVocab := func(i int, field, val string, vocab []string) {
		if val == "" {
			return
		}
		if len(vocab) == 0 {
			problems = append(problems, fmt.Errorf("items[%d]: %s %q but facets.plan declares no %s", i, field, val, field+"s"))
			return
		}
		if !contains(vocab, val) {
			problems = append(problems, fmt.Errorf("items[%d]: %s %q not one of %s", i, field, val, strings.Join(vocab, "|")))
		}
	}

	seenID := map[string]bool{}
	for i, it := range inst.Items {
		switch {
		case it.ID == "":
			problems = append(problems, fmt.Errorf("items[%d]: empty id", i))
		case seenID[it.ID]:
			problems = append(problems, fmt.Errorf("items[%d]: duplicate item id %q", i, it.ID))
		}
		seenID[it.ID] = true

		if it.Title == "" {
			problems = append(problems, fmt.Errorf("items[%d]: empty title", i))
		}
		if it.State == "" {
			problems = append(problems, fmt.Errorf("items[%d]: empty state", i))
		} else if facet.StateRank(it.State) < 0 {
			problems = append(problems, fmt.Errorf("items[%d]: state %q not one of %s", i, it.State, strings.Join(facet.States, "|")))
		}
		inVocab(i, "lane", it.Lane, facet.Lanes)
		inVocab(i, "horizon", it.Horizon, facet.Horizons)
		inVocab(i, "commitment", it.Commitment, facet.Commitments)
		if it.Level != "" {
			if !contains(WorkLevels, it.Level) {
				problems = append(problems, fmt.Errorf("items[%d]: level %q not one of %s", i, it.Level, strings.Join(WorkLevels, "|")))
			} else if len(facet.Levels) > 0 && !contains(facet.Levels, it.Level) {
				problems = append(problems, fmt.Errorf("items[%d]: level %q not published by this plan (%s)", i, it.Level, strings.Join(facet.Levels, "|")))
			}
		}
		if it.Size < 0 {
			problems = append(problems, fmt.Errorf("items[%d]: negative size", i))
		}
		for j, dep := range it.BlockedBy {
			switch {
			case dep == it.ID && dep != "":
				problems = append(problems, fmt.Errorf("items[%d].blocked_by[%d]: item blocks itself", i, j))
			case !ids[dep]:
				problems = append(problems, fmt.Errorf("items[%d].blocked_by[%d]: no item %q in this instance — cross-instance dependencies are not modelled", i, j, dep))
			}
		}
	}

	if cycle := findItemCycle(inst.Items); cycle != "" {
		problems = append(problems, fmt.Errorf("items: dependency cycle %s", cycle))
	}
	return problems
}

// findItemCycle returns a human-readable cycle through blocked_by, or "".
// A cycle cannot be drawn by the dag primitive and cannot be scheduled by
// anyone, so it is always a producer bug rather than a fact about the work.
func findItemCycle(items []Item) string {
	deps := make(map[string][]string, len(items))
	order := make([]string, 0, len(items))
	for _, it := range items {
		if it.ID == "" {
			continue
		}
		if _, seen := deps[it.ID]; !seen {
			order = append(order, it.ID)
		}
		deps[it.ID] = append(deps[it.ID], it.BlockedBy...)
	}

	const (
		visiting = 1
		done     = 2
	)
	state := map[string]int{}
	var path []string

	var walk func(id string) string
	walk = func(id string) string {
		if state[id] == done {
			return ""
		}
		if state[id] == visiting {
			// Trim the path back to where this id first appears, so the
			// message names the cycle rather than the route to it.
			for i, p := range path {
				if p == id {
					return strings.Join(append(append([]string(nil), path[i:]...), id), " -> ")
				}
			}
			return id
		}
		state[id] = visiting
		path = append(path, id)
		for _, dep := range deps[id] {
			if _, known := deps[dep]; !known {
				continue // dangling; already reported as its own problem
			}
			if c := walk(dep); c != "" {
				return c
			}
		}
		path = path[:len(path)-1]
		state[id] = done
		return ""
	}

	for _, id := range order {
		if c := walk(id); c != "" {
			return c
		}
	}
	return ""
}

func sortedFloatKeys(m map[string]float64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
