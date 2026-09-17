// Package reporting parses and validates ReportDefinitions, models report instances, and aggregates them across scopes.
package reporting

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefinitionVersion and DefinitionKind are the only accepted version and kind of a ReportDefinition document.
const (
	DefinitionVersion = "v1alpha1"
	DefinitionKind    = "ReportDefinition"
)

// StatusOK, StatusWarn and StatusCritical are the statuses, ordered worst-last.
const (
	StatusOK       = "ok"
	StatusWarn     = "warn"
	StatusCritical = "critical"
)

// SeverityInfo, SeverityWarn and SeverityCritical are the event severities, mildest first.
const (
	SeverityInfo     = "info"
	SeverityWarn     = "warn"
	SeverityCritical = "critical"
)

var (
	// Categories mirrors contracts/categories.yaml.
	Categories = []string{
		"logs", "performance", "roadmap", "delivery", "failures", "activity",
		"decisions", "cost", "quality",
	}
	// Severities are the accepted event severities, mildest first.
	Severities = []string{SeverityInfo, SeverityWarn, SeverityCritical}
	// Statuses are the accepted statuses, ordered — worst wins.
	Statuses = []string{StatusOK, StatusWarn, StatusCritical}
	// ScopeLevels are the advisory level names for scope path depth 1..4.
	ScopeLevels = []string{"universe", "world", "realm", "site"}
	// Primitives mirrors contracts/primitives.yaml.
	Primitives = []string{
		"gantt", "burndown", "timeseries", "small_multiples", "ticker",
		"dag", "decomposition_tree", "heatmap", "topology", "geomap",
		"kpi_card", "status_matrix", "gauge", "bar_ranked", "funnel", "waterfall", "stacked_bar",
		"board", "roadmap_lanes", "checklist",
		"flamegraph", "scatter", "datagrid", "logbuffer", "diff",
	}
	// PanelSections mirrors contracts/facets.yaml: the reading order of a detail page.
	PanelSections = []string{"overview", "performance", "drivers", "exceptions", "next", "detail"}
	// Directions is the shared "which way is bad" vocabulary of status rules and KPI targets.
	Directions = []string{"above", "below"}
	// Modalities mirrors contracts/modalities.yaml.
	Modalities = []string{"narrative", "glance", "delta", "spatial", "storyboard", "board", "conversational"}
	// Stances mirrors contracts/stances.yaml: the footing a report is read from.
	Stances = []string{"operational", "strategic", "diagnostic"}
	// SectionStances maps each panel section to the stances it serves, per contracts/stances.yaml.
	SectionStances = map[string][]string{
		"overview":    {"operational", "strategic", "diagnostic"},
		"performance": {"strategic"},
		"drivers":     {"strategic", "operational"},
		"exceptions":  {"operational"},
		"next":        {"strategic"},
		"detail":      {"diagnostic"},
	}
	// MediaArchetypes mirrors contracts/media.yaml.
	MediaArchetypes = []string{"anchor", "podcast", "screencast", "recap"}
	// WorkLevels is the flat label vocabulary for Item.Level, from docs/vision/hierarchy.md.
	WorkLevels = []string{"goal", "initiative", "workstream", "activity", "task", "subtask"}
	// ItemPolicies are the aggregation policies legal for a plan's items.
	ItemPolicies = []string{"merge", "sample", "top", "count"}

	timeKPIPolicies = []string{"sum", "avg", "min", "max", "p50", "p95", "latest", "count"}
	// PolicyNames mirrors contracts/aggregations.yaml; sample and top take a positive integer parameter.
	PolicyNames = []string{
		"worst", "sum", "avg", "min", "max", "p50", "p95",
		"latest", "count", "merge", "sample", "top", "synthesize",
	}
)

var groundingKinds = []string{"kpis", "series", "events", "tables"}

// ErrInvalidDefinition wraps every problem found when a definition fails validation.
var ErrInvalidDefinition = errors.New("reporting: invalid report definition")

// Definition is a parsed ReportDefinition: its data contract, facets, scope attachment and aggregation policies.
type Definition struct {
	Version     string   `yaml:"version" json:"version"`
	Kind        string   `yaml:"kind" json:"kind"`
	Name        string   `yaml:"name" json:"name"`
	Description string   `yaml:"description" json:"description"`
	Extends     string   `yaml:"extends,omitempty" json:"extends,omitempty"`
	Categories  []string `yaml:"categories" json:"categories"`
	Modalities  []string `yaml:"modalities" json:"modalities"`

	Stances []string     `yaml:"stances,omitempty" json:"stances,omitempty"`
	Scope   ScopeSpec    `yaml:"scope" json:"scope"`
	Data    DataContract `yaml:"data" json:"data"`
	Facets  Facets       `yaml:"facets" json:"facets"`
}

// ScopeSpec declares which scope levels a definition attaches to and how its instances aggregate upward.
type ScopeSpec struct {
	Attach      []string        `yaml:"attach" json:"attach"`
	Aggregation AggregationSpec `yaml:"aggregation" json:"aggregation"`
}

// AggregationSpec names the aggregation policy per rolled-up field.
type AggregationSpec struct {
	Status   string            `yaml:"status" json:"status"`
	Headline string            `yaml:"headline" json:"headline"`
	KPIs     map[string]string `yaml:"kpis" json:"kpis"`
	Timeline string            `yaml:"timeline" json:"timeline"`
	Pulse    string            `yaml:"pulse" json:"pulse"`

	Items string `yaml:"items" json:"items"`

	Time *TimeAggregationSpec `yaml:"time,omitempty" json:"time,omitempty"`
}

// TimeAggregationSpec names the policy per field for folding one scope path's instances over a period.
type TimeAggregationSpec struct {
	Status   string            `yaml:"status,omitempty" json:"status,omitempty"`
	Headline string            `yaml:"headline,omitempty" json:"headline,omitempty"`
	KPIs     map[string]string `yaml:"kpis,omitempty" json:"kpis,omitempty"`
}

// TimeStatusPolicy is the effective time-fold status policy.
func (a AggregationSpec) TimeStatusPolicy() Policy {
	if a.Time != nil {
		if p, err := ParsePolicy(a.Time.Status); err == nil {
			return p
		}
	}
	return Policy{Name: defaultStatusPolicy}
}

// TimeHeadlinePolicy is the effective time-fold headline policy.
func (a AggregationSpec) TimeHeadlinePolicy() Policy {
	if a.Time != nil {
		if p, err := ParsePolicy(a.Time.Headline); err == nil {
			return p
		}
	}
	return Policy{Name: defaultHeadlinePolicy}
}

// TimeKPIPolicy is the effective time-fold policy for one KPI.
func (a AggregationSpec) TimeKPIPolicy(name string) Policy {
	if a.Time != nil {
		if p, err := ParsePolicy(a.Time.KPIs[name]); err == nil {
			return p
		}
	}
	return Policy{Name: defaultKPIPolicy}
}

// DataContract declares the data a report instance may carry.
type DataContract struct {
	KPIs   []KPISpec    `yaml:"kpis" json:"kpis"`
	Series []SeriesSpec `yaml:"series" json:"series"`
	Events []EventSpec  `yaml:"events" json:"events"`
	Tables []TableSpec  `yaml:"tables" json:"tables"`
}

// KPISpec declares one named scalar metric.
type KPISpec struct {
	Name      string   `yaml:"name" json:"name"`
	Unit      string   `yaml:"unit" json:"unit"`
	Window    string   `yaml:"window" json:"window"`
	Target    *float64 `yaml:"target,omitempty" json:"target,omitempty"`
	Direction string   `yaml:"direction,omitempty" json:"direction,omitempty"`
}

// SeriesSpec declares one named time series.
type SeriesSpec struct {
	Name string `yaml:"name" json:"name"`
	Unit string `yaml:"unit" json:"unit"`
}

// EventSpec declares one event type a report emits, with its severity.
type EventSpec struct {
	Type     string `yaml:"type" json:"type"`
	Severity string `yaml:"severity" json:"severity"`
}

// TableSpec declares one named table and its columns.
type TableSpec struct {
	Name    string   `yaml:"name" json:"name"`
	Columns []string `yaml:"columns" json:"columns"`
}

// Facets is the set of capability interfaces a definition exposes.
type Facets struct {
	Summary      *SummaryFacet      `yaml:"summary" json:"summary,omitempty"`
	Timeline     *TimelineFacet     `yaml:"timeline" json:"timeline,omitempty"`
	Detail       *DetailFacet       `yaml:"detail" json:"detail,omitempty"`
	Plan         *PlanFacet         `yaml:"plan" json:"plan,omitempty"`
	Pulse        *PulseFacet        `yaml:"pulse" json:"pulse,omitempty"`
	Media        *MediaFacet        `yaml:"media" json:"media,omitempty"`
	Conversation *ConversationFacet `yaml:"conversation" json:"conversation,omitempty"`
}

// SummaryFacet answers "how are you doing?": a status rule, a few KPIs, a sparkline and a headline template.
type SummaryFacet struct {
	Status    *StatusRule `yaml:"status" json:"status,omitempty"`
	KPIs      []string    `yaml:"kpis" json:"kpis"`
	Sparkline string      `yaml:"sparkline" json:"sparkline,omitempty"`
	Headline  string      `yaml:"headline" json:"headline,omitempty"`
}

// StatusRule derives a status from one KPI against thresholds.
type StatusRule struct {
	From       string  `yaml:"from" json:"from"`
	WarnAt     float64 `yaml:"warn_at" json:"warn_at"`
	CriticalAt float64 `yaml:"critical_at" json:"critical_at"`
	Direction  string  `yaml:"direction" json:"direction"`
}

// TimelineFacet answers "when did things happen?".
type TimelineFacet struct {
	Events     []string   `yaml:"events" json:"events"`
	Spans      []SpanSpec `yaml:"spans" json:"spans,omitempty"`
	Milestones []string   `yaml:"milestones" json:"milestones,omitempty"`
}

// SpanSpec pairs a start and end event type into a named span.
type SpanSpec struct {
	Name  string `yaml:"name" json:"name"`
	Start string `yaml:"start" json:"start"`
	End   string `yaml:"end" json:"end"`
}

// DetailFacet answers "show me everything": ordered panels plus drilldown links to other definitions.
type DetailFacet struct {
	Panels    []PanelSpec `yaml:"panels" json:"panels"`
	Drilldown []string    `yaml:"drilldown" json:"drilldown,omitempty"`
}

// PanelSpec binds a visual primitive to a data reference such as "series/<name>", "kpi/<name>", "events" or "items".
type PanelSpec struct {
	Title     string `yaml:"title" json:"title"`
	Primitive string `yaml:"primitive" json:"primitive"`
	Data      string `yaml:"data" json:"data"`

	Section string `yaml:"section,omitempty" json:"section,omitempty"`

	Stance string `yaml:"stance,omitempty" json:"stance,omitempty"`
}

// Stances returns the stances this panel serves: its own, else its section's, else every stance.
func (p PanelSpec) Stances() []string {
	if p.Stance != "" {
		return []string{p.Stance}
	}
	if st, ok := SectionStances[p.Section]; ok {
		return append([]string(nil), st...)
	}
	return append([]string(nil), Stances...)
}

// PlanFacet answers "what are we going to do?": the board a producer publishes items onto, states in progress order.
type PlanFacet struct {
	States []string `yaml:"states" json:"states"`
	Lanes  []string `yaml:"lanes,omitempty" json:"lanes,omitempty"`

	Horizons []string `yaml:"horizons,omitempty" json:"horizons,omitempty"`

	Commitments []string `yaml:"commitments,omitempty" json:"commitments,omitempty"`

	Levels []string `yaml:"levels,omitempty" json:"levels,omitempty"`

	WIPLimits map[string]int `yaml:"wip_limits,omitempty" json:"wip_limits,omitempty"`

	SizeUnit string        `yaml:"size_unit,omitempty" json:"size_unit,omitempty"`
	Baseline *PlanBaseline `yaml:"baseline,omitempty" json:"baseline,omitempty"`
}

// PlanBaseline names what the plan is measured against: a kpi/<name> or series/<name> ref in the data contract.
type PlanBaseline struct {
	Name string `yaml:"name" json:"name"`
	Ref  string `yaml:"ref" json:"ref"`
}

// StateRank returns the declared index of a state, or -1 when the state is not one this plan declares.
func (p *PlanFacet) StateRank(state string) int {
	if p == nil {
		return -1
	}
	for i, s := range p.States {
		if s == state {
			return i
		}
	}
	return -1
}

// Terminal reports whether state is this plan's last declared state.
func (p *PlanFacet) Terminal(state string) bool {
	if p == nil || len(p.States) == 0 {
		return false
	}
	return state == p.States[len(p.States)-1]
}

// PulseFacet answers "what is happening right now?".
type PulseFacet struct {
	Events    []string `yaml:"events" json:"events"`
	RateLimit string   `yaml:"rate_limit" json:"rate_limit,omitempty"`
}

// MediaFacet declares which media script archetypes apply.
type MediaFacet struct {
	Archetypes []string `yaml:"archetypes" json:"archetypes"`
}

// ConversationFacet declares which data kinds ground on-demand Q&A.
type ConversationFacet struct {
	Grounding []string `yaml:"grounding" json:"grounding"`
	Prompts   []string `yaml:"prompts" json:"prompts,omitempty"`
}

var facetOrder = []string{"summary", "detail", "timeline", "pulse", "media", "conversation", "plan"}

// StanceNames returns the stances this report can be read from, in canonical order; every stance when it declares none.
func (d *Definition) StanceNames() []string {
	if len(d.Stances) == 0 {
		return append([]string(nil), Stances...)
	}
	out := make([]string, 0, len(d.Stances))
	for _, s := range Stances {
		if contains(d.Stances, s) {
			out = append(out, s)
		}
	}
	return out
}

// ServesStance reports whether this report can be read from a stance.
func (d *Definition) ServesStance(stance string) bool {
	if stance == "" || len(d.Stances) == 0 {
		return true
	}
	return contains(d.Stances, stance)
}

// FacetNames returns which facets the definition exposes, in canonical order.
func (d *Definition) FacetNames() []string {
	present := map[string]bool{
		"summary":      d.Facets.Summary != nil,
		"detail":       d.Facets.Detail != nil,
		"timeline":     d.Facets.Timeline != nil,
		"pulse":        d.Facets.Pulse != nil,
		"media":        d.Facets.Media != nil,
		"conversation": d.Facets.Conversation != nil,
		"plan":         d.Facets.Plan != nil,
	}
	names := make([]string, 0, len(facetOrder))
	for _, n := range facetOrder {
		if present[n] {
			names = append(names, n)
		}
	}
	return names
}

var (
	namePattern           = regexp.MustCompile(`^[a-z0-9-]+$`)
	headlinePlaceholderRE = regexp.MustCompile(`\{\{\s*([^{}]+?)\s*\}\}`)
)

// ParseDefinition parses and validates a YAML ReportDefinition document.
func ParseDefinition(data []byte) (*Definition, error) {
	var d Definition
	if err := yaml.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("reporting: parse definition: %w", err)
	}
	problems := d.validateCore()
	if d.Extends == "" {
		problems = append(problems, d.validateRefs()...)
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("%w: %w", ErrInvalidDefinition, errors.Join(problems...))
	}
	return &d, nil
}

func (d *Definition) validateCore() []error {
	var problems []error

	if d.Version != DefinitionVersion {
		problems = append(problems, fmt.Errorf("version: got %q, want %q", d.Version, DefinitionVersion))
	}
	if d.Kind != DefinitionKind {
		problems = append(problems, fmt.Errorf("kind: got %q, want %q", d.Kind, DefinitionKind))
	}
	if d.Name == "" {
		problems = append(problems, errors.New("name: empty"))
	} else if !namePattern.MatchString(d.Name) {
		problems = append(problems, fmt.Errorf("name: %q does not match [a-z0-9-]+", d.Name))
	}

	for i, c := range d.Categories {
		if !contains(Categories, c) {
			problems = append(problems, fmt.Errorf("categories[%d]: unknown category %q", i, c))
		}
	}
	for i, st := range d.Stances {
		if !contains(Stances, st) {
			problems = append(problems, fmt.Errorf("stances[%d]: unknown stance %q, want one of %s", i, st, strings.Join(Stances, "|")))
		}
	}
	for i, m := range d.Modalities {
		if !contains(Modalities, m) {
			problems = append(problems, fmt.Errorf("modalities[%d]: unknown modality %q", i, m))
		}
	}
	for i, l := range d.Scope.Attach {
		if !contains(ScopeLevels, l) {
			problems = append(problems, fmt.Errorf("scope.attach[%d]: unknown scope level %q", i, l))
		}
	}

	checkPolicy := func(field, s string) {
		if s == "" {
			return
		}
		if _, err := ParsePolicy(s); err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", field, err))
		}
	}
	checkPolicy("scope.aggregation.status", d.Scope.Aggregation.Status)
	checkPolicy("scope.aggregation.headline", d.Scope.Aggregation.Headline)
	checkPolicy("scope.aggregation.timeline", d.Scope.Aggregation.Timeline)
	checkPolicy("scope.aggregation.pulse", d.Scope.Aggregation.Pulse)
	for _, name := range sortedKeys(d.Scope.Aggregation.KPIs) {
		checkPolicy(fmt.Sprintf("scope.aggregation.kpis[%s]", name), d.Scope.Aggregation.KPIs[name])
	}
	if tm := d.Scope.Aggregation.Time; tm != nil {
		checkPolicy("scope.aggregation.time.status", tm.Status)
		if p, err := ParsePolicy(tm.Status); tm.Status != "" && err == nil && p.Name != "worst" && p.Name != "latest" {
			problems = append(problems, fmt.Errorf("scope.aggregation.time.status: policy %q does not apply to status, want worst|latest", p.Name))
		}
		checkPolicy("scope.aggregation.time.headline", tm.Headline)
		if p, err := ParsePolicy(tm.Headline); tm.Headline != "" && err == nil && p.Name != "latest" && p.Name != "top" && p.Name != "synthesize" {
			problems = append(problems, fmt.Errorf("scope.aggregation.time.headline: policy %q does not apply to headline, want latest|top(n)|synthesize", p.Name))
		}
		for _, name := range sortedKeys(tm.KPIs) {
			field := fmt.Sprintf("scope.aggregation.time.kpis[%s]", name)
			checkPolicy(field, tm.KPIs[name])
			if p, err := ParsePolicy(tm.KPIs[name]); err == nil && !contains(timeKPIPolicies, p.Name) {
				problems = append(problems, fmt.Errorf("%s: policy %q does not apply to kpis, want one of %s", field, p.Name, strings.Join(timeKPIPolicies, "|")))
			}
		}
	}
	if items := d.Scope.Aggregation.Items; items != "" {
		checkPolicy("scope.aggregation.items", items)
		if pol, err := ParsePolicy(items); err == nil && !contains(ItemPolicies, pol.Name) {
			problems = append(problems, fmt.Errorf("scope.aggregation.items: policy %q does not apply to items, want one of %s",
				pol.Name, strings.Join(ItemPolicies, "|")))
		}
		if d.Facets.Plan == nil {
			problems = append(problems, errors.New("scope.aggregation.items: declared without a facets.plan to roll up"))
		}
	}

	if pf := d.Facets.Plan; pf != nil {
		if len(pf.States) == 0 {
			problems = append(problems, errors.New("facets.plan.states: empty — a board with no columns declares no plan"))
		}
		checkVocab := func(field string, vals []string) {
			seen := map[string]bool{}
			for i, v := range vals {
				switch {
				case v == "":
					problems = append(problems, fmt.Errorf("%s[%d]: empty", field, i))
				case seen[v]:
					problems = append(problems, fmt.Errorf("%s[%d]: duplicate %q — the order would be ambiguous", field, i, v))
				}
				seen[v] = true
			}
		}
		checkVocab("facets.plan.states", pf.States)
		checkVocab("facets.plan.lanes", pf.Lanes)
		checkVocab("facets.plan.horizons", pf.Horizons)
		checkVocab("facets.plan.commitments", pf.Commitments)
		checkVocab("facets.plan.levels", pf.Levels)
		for i, l := range pf.Levels {
			if !contains(WorkLevels, l) {
				problems = append(problems, fmt.Errorf("facets.plan.levels[%d]: unknown level %q, want one of %s",
					i, l, strings.Join(WorkLevels, "|")))
			}
		}
		for _, st := range sortedIntKeys(pf.WIPLimits) {
			if !contains(pf.States, st) {
				problems = append(problems, fmt.Errorf("facets.plan.wip_limits[%s]: no such state in facets.plan.states", st))
			}
			if pf.WIPLimits[st] < 0 {
				problems = append(problems, fmt.Errorf("facets.plan.wip_limits[%s]: negative limit", st))
			}
		}
		if pf.Baseline != nil && pf.Baseline.Name == "" {
			problems = append(problems, errors.New("facets.plan.baseline.name: empty — a baseline with no name cannot be labelled"))
		}
	}

	seen := map[string]bool{}
	for i, k := range d.Data.KPIs {
		if k.Name == "" {
			problems = append(problems, fmt.Errorf("data.kpis[%d]: empty name", i))
		} else if seen[k.Name] {
			problems = append(problems, fmt.Errorf("data.kpis[%d]: duplicate kpi %q", i, k.Name))
		}
		if k.Direction != "" && !contains(Directions, k.Direction) {
			problems = append(problems, fmt.Errorf("data.kpis[%d]: unknown direction %q, want one of %s", i, k.Direction, strings.Join(Directions, "|")))
		}
		if k.Target != nil && k.Direction == "" {
			problems = append(problems, fmt.Errorf("data.kpis[%d]: target set without a direction — a target nobody can be on the wrong side of says nothing", i))
		}
		seen[k.Name] = true
	}
	seen = map[string]bool{}
	for i, s := range d.Data.Series {
		if s.Name == "" {
			problems = append(problems, fmt.Errorf("data.series[%d]: empty name", i))
		} else if seen[s.Name] {
			problems = append(problems, fmt.Errorf("data.series[%d]: duplicate series %q", i, s.Name))
		}
		seen[s.Name] = true
	}
	seen = map[string]bool{}
	for i, e := range d.Data.Events {
		if e.Type == "" {
			problems = append(problems, fmt.Errorf("data.events[%d]: empty type", i))
		} else if seen[e.Type] {
			problems = append(problems, fmt.Errorf("data.events[%d]: duplicate event type %q", i, e.Type))
		}
		seen[e.Type] = true
		if !contains(Severities, e.Severity) {
			problems = append(problems, fmt.Errorf("data.events[%d]: severity %q not one of %s", i, e.Severity, strings.Join(Severities, "|")))
		}
	}
	seen = map[string]bool{}
	for i, t := range d.Data.Tables {
		if t.Name == "" {
			problems = append(problems, fmt.Errorf("data.tables[%d]: empty name", i))
		} else if seen[t.Name] {
			problems = append(problems, fmt.Errorf("data.tables[%d]: duplicate table %q", i, t.Name))
		}
		seen[t.Name] = true
	}

	if s := d.Facets.Summary; s != nil && s.Status != nil {
		if dir := s.Status.Direction; !contains(Directions, dir) {
			problems = append(problems, fmt.Errorf("facets.summary.status.direction: %q not one of above|below", dir))
		}
	}
	if det := d.Facets.Detail; det != nil {
		for i, p := range det.Panels {
			if !contains(Primitives, p.Primitive) {
				problems = append(problems, fmt.Errorf("facets.detail.panels[%d]: unknown primitive %q", i, p.Primitive))
			}
			if p.Stance != "" && !contains(Stances, p.Stance) {
				problems = append(problems, fmt.Errorf("facets.detail.panels[%d]: unknown stance %q, want one of %s", i, p.Stance, strings.Join(Stances, "|")))
			}
			if p.Section != "" && !contains(PanelSections, p.Section) {
				problems = append(problems, fmt.Errorf("facets.detail.panels[%d]: unknown section %q, want one of %s", i, p.Section, strings.Join(PanelSections, "|")))
			}
		}
	}
	if p := d.Facets.Pulse; p != nil && p.RateLimit != "" {
		if err := validateRateLimit(p.RateLimit); err != nil {
			problems = append(problems, fmt.Errorf("facets.pulse.rate_limit: %w", err))
		}
	}
	if m := d.Facets.Media; m != nil {
		for i, a := range m.Archetypes {
			if !contains(MediaArchetypes, a) {
				problems = append(problems, fmt.Errorf("facets.media.archetypes[%d]: unknown archetype %q", i, a))
			}
		}
	}
	if c := d.Facets.Conversation; c != nil {
		for i, g := range c.Grounding {
			if !contains(groundingKinds, g) {
				problems = append(problems, fmt.Errorf("facets.conversation.grounding[%d]: unknown data kind %q (want one of %s)", i, g, strings.Join(groundingKinds, "|")))
			}
		}
	}

	return problems
}

func (d *Definition) validateRefs() []error {
	var problems []error

	kpis := map[string]bool{}
	for _, k := range d.Data.KPIs {
		kpis[k.Name] = true
	}
	series := map[string]bool{}
	for _, s := range d.Data.Series {
		series[s.Name] = true
	}
	events := map[string]bool{}
	for _, e := range d.Data.Events {
		events[e.Type] = true
	}
	tables := map[string]bool{}
	for _, t := range d.Data.Tables {
		tables[t.Name] = true
	}
	spans := map[string]bool{}
	if t := d.Facets.Timeline; t != nil {
		for _, sp := range t.Spans {
			spans[sp.Name] = true
		}
	}

	for _, name := range sortedKeys(d.Scope.Aggregation.KPIs) {
		if !kpis[name] {
			problems = append(problems, fmt.Errorf("scope.aggregation.kpis[%s]: no such kpi in data contract", name))
		}
	}
	if tm := d.Scope.Aggregation.Time; tm != nil {
		for _, name := range sortedKeys(tm.KPIs) {
			if !kpis[name] {
				problems = append(problems, fmt.Errorf("scope.aggregation.time.kpis[%s]: no such kpi in data contract", name))
			}
		}
	}

	if s := d.Facets.Summary; s != nil {
		for i, k := range s.KPIs {
			if !kpis[k] {
				problems = append(problems, fmt.Errorf("facets.summary.kpis[%d]: no such kpi %q in data contract", i, k))
			}
		}
		if s.Sparkline != "" && !series[s.Sparkline] {
			problems = append(problems, fmt.Errorf("facets.summary.sparkline: no such series %q in data contract", s.Sparkline))
		}
		if s.Status != nil && !kpis[s.Status.From] {
			problems = append(problems, fmt.Errorf("facets.summary.status.from: no such kpi %q in data contract", s.Status.From))
		}
		for _, ph := range headlinePlaceholders(s.Headline) {
			if !kpis[ph] {
				problems = append(problems, fmt.Errorf("facets.summary.headline: placeholder {{%s}} is not a kpi in the data contract", ph))
			}
		}
	}

	if t := d.Facets.Timeline; t != nil {
		for i, e := range t.Events {
			if !events[e] {
				problems = append(problems, fmt.Errorf("facets.timeline.events[%d]: no such event type %q in data contract", i, e))
			}
		}
		for i, sp := range t.Spans {
			if sp.Name == "" {
				problems = append(problems, fmt.Errorf("facets.timeline.spans[%d]: empty name", i))
			}
			if !events[sp.Start] {
				problems = append(problems, fmt.Errorf("facets.timeline.spans[%d]: start event %q not in data contract", i, sp.Start))
			}
			if !events[sp.End] {
				problems = append(problems, fmt.Errorf("facets.timeline.spans[%d]: end event %q not in data contract", i, sp.End))
			}
		}
		for i, m := range t.Milestones {
			if !events[m] {
				problems = append(problems, fmt.Errorf("facets.timeline.milestones[%d]: no such event type %q in data contract", i, m))
			}
		}
	}

	if det := d.Facets.Detail; det != nil {
		for i, p := range det.Panels {
			if err := validatePanelData(p.Data, kpis, series, tables, spans, len(d.Data.Events) > 0, d.Facets.Plan != nil); err != nil {
				problems = append(problems, fmt.Errorf("facets.detail.panels[%d]: %w", i, err))
			}
		}
	}

	if p := d.Facets.Pulse; p != nil {
		for i, e := range p.Events {
			if !events[e] {
				problems = append(problems, fmt.Errorf("facets.pulse.events[%d]: no such event type %q in data contract", i, e))
			}
		}
	}

	if pf := d.Facets.Plan; pf != nil && pf.Baseline != nil {
		kind, name, ok := strings.Cut(pf.Baseline.Ref, "/")
		switch {
		case !ok || name == "":
			problems = append(problems, fmt.Errorf("facets.plan.baseline.ref: bad ref %q (want kpi/<name> or series/<name>)", pf.Baseline.Ref))
		case kind == "kpi" && !kpis[name]:
			problems = append(problems, fmt.Errorf("facets.plan.baseline.ref: no such kpi %q in data contract", name))
		case kind == "series" && !series[name]:
			problems = append(problems, fmt.Errorf("facets.plan.baseline.ref: no such series %q in data contract", name))
		case kind != "kpi" && kind != "series":
			problems = append(problems, fmt.Errorf("facets.plan.baseline.ref: %q cannot be a baseline (want kpi/<name> or series/<name>)", pf.Baseline.Ref))
		}
	}

	return problems
}

func validatePanelData(ref string, kpis, series, tables, spans map[string]bool, hasEvents, hasPlan bool) error {
	const grammar = "want series/<name>, table/<name>, spans/<name>, kpi/<name>, events, or items"
	if ref == "events" {
		if !hasEvents {
			return errors.New(`data ref "events": data contract declares no events`)
		}
		return nil
	}
	if ref == "items" {
		if !hasPlan {
			return errors.New(`data ref "items": definition declares no plan facet`)
		}
		return nil
	}
	kind, name, ok := strings.Cut(ref, "/")
	if !ok || name == "" {
		return fmt.Errorf("bad data ref %q (%s)", ref, grammar)
	}
	switch kind {
	case "series":
		if !series[name] {
			return fmt.Errorf("data ref %q: no such series in data contract", ref)
		}
	case "table":
		if !tables[name] {
			return fmt.Errorf("data ref %q: no such table in data contract", ref)
		}
	case "spans":
		if !spans[name] {
			return fmt.Errorf("data ref %q: no such span declared in facets.timeline.spans", ref)
		}
	case "kpi":
		if !kpis[name] {
			return fmt.Errorf("data ref %q: no such kpi in data contract", ref)
		}
	default:
		return fmt.Errorf("bad data ref %q (%s)", ref, grammar)
	}
	return nil
}

func sortedIntKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func validateRateLimit(s string) error {
	count, unit, ok := strings.Cut(s, "/")
	if !ok {
		return fmt.Errorf("bad rate limit %q (want N/min)", s)
	}
	n, err := strconv.Atoi(strings.TrimSpace(count))
	if err != nil || n <= 0 {
		return fmt.Errorf("bad rate limit %q: count must be a positive integer", s)
	}
	if strings.TrimSpace(unit) != "min" {
		return fmt.Errorf("bad rate limit %q: unit must be min", s)
	}
	return nil
}

func headlinePlaceholders(tmpl string) []string {
	var names []string
	for _, m := range headlinePlaceholderRE.FindAllStringSubmatch(tmpl, -1) {
		names = append(names, m[1])
	}
	return names
}

func contains(set []string, v string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
