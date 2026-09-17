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

// Event is one timeline/pulse event.
type Event struct {
	T          time.Time         `json:"t"`
	Type       string            `json:"type"`
	Severity   string            `json:"severity"`
	Label      string            `json:"label"`
	Scope      string            `json:"scope,omitempty"`
	Definition string            `json:"definition,omitempty"`
	Attrs      map[string]string `json:"attrs,omitempty"`
}

// Item is one reported piece of intended work: a card, a roadmap row, a checklist line.
type Item struct {
	ID    string `json:"id"`
	Title string `json:"title"`

	State string `json:"state"`
	Lane  string `json:"lane,omitempty"`
	Owner string `json:"owner,omitempty"`

	Due *time.Time `json:"due,omitempty"`

	Size float64 `json:"size,omitempty"`

	BlockedBy  []string `json:"blocked_by,omitempty"`
	Horizon    string   `json:"horizon,omitempty"`
	Commitment string   `json:"commitment,omitempty"`
	Level      string   `json:"level,omitempty"`
	Scope      string   `json:"scope,omitempty"`
	Definition string   `json:"definition,omitempty"`
}

// Blocked reports whether anything is holding this item up.
func (it Item) Blocked() bool { return len(it.BlockedBy) > 0 }

// Instance is the wire payload a node submits: one report's data for one scope at one instant.
type Instance struct {
	Definition string    `json:"definition"`
	Scope      string    `json:"scope"`
	Producer   string    `json:"producer,omitempty"`
	ProducedAt time.Time `json:"produced_at"`
	Status     string    `json:"status,omitempty"`
	Headline   string    `json:"headline,omitempty"`

	KPIs map[string]float64 `json:"kpis,omitempty"`

	Targets map[string]float64       `json:"targets,omitempty"`
	Series  map[string][]SeriesPoint `json:"series,omitempty"`
	Tables  map[string]Table         `json:"tables,omitempty"`
	Events  []Event                  `json:"events,omitempty"`

	Items []Item `json:"items,omitempty"`
}

// ErrInvalidInstance wraps every problem found when an instance fails validation.
var ErrInvalidInstance = errors.New("reporting: invalid report instance")

// ValidateInstance checks an instance exhaustively against its definition's data contract and plan facet.
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

	requireDeclared := func(i int, field, val string, vocab []string) {
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
		requireDeclared(i, "lane", it.Lane, facet.Lanes)
		requireDeclared(i, "horizon", it.Horizon, facet.Horizons)
		requireDeclared(i, "commitment", it.Commitment, facet.Commitments)
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
			return cycleClosingAt(path, id)
		}
		state[id] = visiting
		path = append(path, id)
		for _, dep := range deps[id] {
			if _, known := deps[dep]; !known {
				continue
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

func cycleClosingAt(path []string, id string) string {
	for i, p := range path {
		if p == id {
			return strings.Join(append(append([]string(nil), path[i:]...), id), " -> ")
		}
	}
	return id
}

func sortedFloatKeys(m map[string]float64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
