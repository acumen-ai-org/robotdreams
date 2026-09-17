// Package reporting is the contracts library for the reporting subsystem:
// it parses YAML ReportDefinitions, validates them against the shared
// vocabulary (reporting/contracts/*.yaml baked in as Go constants), models
// the report instances nodes submit, and aggregates instances across
// organizational scopes.
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

// DefinitionVersion and DefinitionKind are the only accepted values for a
// ReportDefinition document's version and kind fields.
const (
	DefinitionVersion = "v1alpha1"
	DefinitionKind    = "ReportDefinition"
)

// Statuses, ordered worst-last: aggregation's "worst" policy picks the
// highest-ranked status present.
const (
	StatusOK       = "ok"
	StatusWarn     = "warn"
	StatusCritical = "critical"
)

// Event severities.
const (
	SeverityInfo     = "info"
	SeverityWarn     = "warn"
	SeverityCritical = "critical"
)

// Vocabulary sets, mirroring reporting/contracts/*.yaml. Deployments may
// extend the YAML contracts, but this library validates against the
// standard vocabulary only.
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
	// PanelSections mirrors contracts/facets.yaml: the reading order a
	// detail page follows — what happened, why, what needs attention,
	// and the numbers behind it. A panel need not name one; those render
	// in declaration order ahead of the sectioned ones.
	PanelSections = []string{"overview", "performance", "drivers", "exceptions", "next", "detail"}
	// Directions is the shared "which way is bad" vocabulary, used by
	// both a summary status rule and a KPI target.
	Directions = []string{"above", "below"}
	// Modalities mirrors contracts/modalities.yaml.
	Modalities = []string{"narrative", "glance", "delta", "spatial", "storyboard", "board", "conversational"}
	// Stances mirrors contracts/stances.yaml: the footing a report is
	// read from — what its numbers are measured against. A threshold
	// (operational), a plan (strategic), or nothing at all, because the
	// record is the answer (diagnostic).
	Stances = []string{"operational", "strategic", "diagnostic"}
	// SectionStances inverts contracts/stances.yaml's per-stance section
	// lists: which stances each panel section serves.
	//
	// This is what makes the axis work across the whole library without a
	// per-panel declaration. The reading order in facets.yaml was already
	// a stance projection — "actual against target" is strategic by
	// definition, "items requiring a decision" is operational, "the
	// searchable matrix underneath" is diagnostic — so a panel that named
	// a section has already said most of it.
	SectionStances = map[string][]string{
		"overview":    {"operational", "strategic", "diagnostic"},
		"performance": {"strategic"},
		"drivers":     {"strategic", "operational"},
		"exceptions":  {"operational"},
		// `next` is the one section about the future, and steering is the
		// only footing from which a plan is a judgement rather than a list.
		"next":   {"strategic"},
		"detail": {"diagnostic"},
	}
	// MediaArchetypes mirrors contracts/media.yaml.
	MediaArchetypes = []string{"anchor", "podcast", "screencast", "recap"}
	// WorkLevels mirrors docs/vision/hierarchy.md's levels, used here as a
	// flat LABEL vocabulary for Item.Level. Naming the levels is not
	// modelling them: nothing in this library relates a "task" to the
	// "activity" above it, and no field could — hierarchy.md resolved that
	// the Go build represents no work hierarchy, and this does not reopen
	// it.
	WorkLevels = []string{"goal", "initiative", "workstream", "activity", "task", "subtask"}
	// ItemPolicies are the aggregation policies legal for a plan's items.
	// Every one of them either merges or counts; none of them averages,
	// because there is no arithmetic to do on a card.
	ItemPolicies = []string{"merge", "sample", "top", "count"}
	// kpiPolicies are the policies that reduce a list of scalars; the
	// ones a KPI may declare for its time fold.
	kpiPolicies = []string{"sum", "avg", "min", "max", "p50", "p95", "latest", "count"}
	// PolicyNames mirrors contracts/aggregations.yaml; sample and top take
	// a positive integer parameter, e.g. "sample(50)".
	PolicyNames = []string{
		"worst", "sum", "avg", "min", "max", "p50", "p95",
		"latest", "count", "merge", "sample", "top", "synthesize",
	}
)

// groundingKinds are the data kinds a conversation facet may expose.
var groundingKinds = []string{"kpis", "series", "events", "tables"}

// ErrInvalidDefinition is returned by ParseDefinition, LoadDefinitionFile
// and LoadLibraryDir when a definition fails validation. The returned error
// wraps the individual problems (joined with errors.Join), so every issue
// is reported at once rather than only the first.
var ErrInvalidDefinition = errors.New("reporting: invalid report definition")

// Definition is a parsed ReportDefinition: what a report produces (its data
// contract) and which facets it exposes, plus where in the organizational
// scope tree it attaches and how instances roll up.
type Definition struct {
	Version     string   `yaml:"version" json:"version"`
	Kind        string   `yaml:"kind" json:"kind"`
	Name        string   `yaml:"name" json:"name"`
	Description string   `yaml:"description" json:"description"`
	Extends     string   `yaml:"extends,omitempty" json:"extends,omitempty"`
	Categories  []string `yaml:"categories" json:"categories"`
	Modalities  []string `yaml:"modalities" json:"modalities"`
	// Stances the report can be read from. Optional: empty means every
	// stance, which is the honest answer for a report that does not care
	// how it is judged.
	Stances []string     `yaml:"stances,omitempty" json:"stances,omitempty"`
	Scope   ScopeSpec    `yaml:"scope" json:"scope"`
	Data    DataContract `yaml:"data" json:"data"`
	Facets  Facets       `yaml:"facets" json:"facets"`
}

// ScopeSpec declares which scope levels a definition attaches to and how
// its instances aggregate upward.
type ScopeSpec struct {
	Attach      []string        `yaml:"attach" json:"attach"`
	Aggregation AggregationSpec `yaml:"aggregation" json:"aggregation"`
}

// AggregationSpec names the aggregation policy per rolled-up field. Values
// are policy strings as accepted by ParsePolicy.
type AggregationSpec struct {
	Status   string            `yaml:"status" json:"status"`
	Headline string            `yaml:"headline" json:"headline"`
	KPIs     map[string]string `yaml:"kpis" json:"kpis"`
	Timeline string            `yaml:"timeline" json:"timeline"`
	Pulse    string            `yaml:"pulse" json:"pulse"`
	// Items is how N boards below a scope become one board at it. Legal
	// values are ItemPolicies; see contracts/aggregations.yaml for what
	// each does to a plan and for the keep-order a bounded board drops by.
	Items string `yaml:"items" json:"items"`
	// Time is how ONE scope path's instances fold over a period before
	// the scope roll-up above applies — the stage a reader asks for with
	// `period=week`. Absent, every field defaults to `latest`, which is
	// today's newest-per-path rule and therefore changes nothing for a
	// definition that never declares it.
	Time *TimeAggregationSpec `yaml:"time,omitempty" json:"time,omitempty"`
}

// TimeAggregationSpec names the policy per field for folding one scope
// path's instances across a period into one synthetic instance. It is
// a separate spec from the scope policies because the two questions
// differ: a counter like tokens_in SUMS over a week but also sums across
// sites, whereas a gauge like active_nodes sums across sites but should
// read `latest` (or `max`) over a week — adding Monday's node count to
// Tuesday's is not a number anyone asked for.
//
// Defaults: Status `worst` (a week with one critical day was a critical
// week), Headline `latest` (prose cannot be recomputed, only chosen; the
// most recent sentence stands for the period), KPIs `latest` per KPI.
// Values are policy strings as accepted by ParsePolicy; status accepts
// worst|latest, headline latest|top(n)|synthesize, kpis the scalar
// policies.
type TimeAggregationSpec struct {
	Status   string            `yaml:"status,omitempty" json:"status,omitempty"`
	Headline string            `yaml:"headline,omitempty" json:"headline,omitempty"`
	KPIs     map[string]string `yaml:"kpis,omitempty" json:"kpis,omitempty"`
}

// TimeStatusPolicy is the effective time-fold status policy: the declared
// one when it parses, else worst.
func (a AggregationSpec) TimeStatusPolicy() Policy {
	if a.Time != nil {
		if p, err := ParsePolicy(a.Time.Status); err == nil {
			return p
		}
	}
	return Policy{Name: "worst"}
}

// TimeHeadlinePolicy is the effective time-fold headline policy: the
// declared one when it parses, else latest.
func (a AggregationSpec) TimeHeadlinePolicy() Policy {
	if a.Time != nil {
		if p, err := ParsePolicy(a.Time.Headline); err == nil {
			return p
		}
	}
	return Policy{Name: "latest"}
}

// TimeKPIPolicy is the effective time-fold policy for one KPI: the
// declared one when it parses, else latest.
func (a AggregationSpec) TimeKPIPolicy(name string) Policy {
	if a.Time != nil {
		if p, err := ParsePolicy(a.Time.KPIs[name]); err == nil {
			return p
		}
	}
	return Policy{Name: "latest"}
}

// DataContract declares the data a report instance may carry. Facet
// references resolve against these names.
type DataContract struct {
	KPIs   []KPISpec    `yaml:"kpis" json:"kpis"`
	Series []SeriesSpec `yaml:"series" json:"series"`
	Events []EventSpec  `yaml:"events" json:"events"`
	Tables []TableSpec  `yaml:"tables" json:"tables"`
}

// KPISpec declares one named scalar metric.
//
// Target and Direction are what turn the metric into an answer rather
// than a number: contracts/primitives.yaml requires a kpi_card to show
// "the target and the variance between them", and without a declared
// target there is nothing to compare against. Both are optional — a
// metric that has no meaningful target (log lines, tokens) simply omits
// them and renders exactly as it did before.
//
// Target here is the definition-level default; an instance may override
// it per scope via Instance.Targets, because one squad's latency budget
// is not another's. Direction says which side of the target is bad, with
// the same meaning as StatusRule.Direction: "above" means higher is
// worse, "below" means lower is worse.
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

// Facets is the set of capability interfaces a definition exposes. A nil
// facet is simply not exposed.
type Facets struct {
	Summary      *SummaryFacet      `yaml:"summary" json:"summary,omitempty"`
	Timeline     *TimelineFacet     `yaml:"timeline" json:"timeline,omitempty"`
	Detail       *DetailFacet       `yaml:"detail" json:"detail,omitempty"`
	Plan         *PlanFacet         `yaml:"plan" json:"plan,omitempty"`
	Pulse        *PulseFacet        `yaml:"pulse" json:"pulse,omitempty"`
	Media        *MediaFacet        `yaml:"media" json:"media,omitempty"`
	Conversation *ConversationFacet `yaml:"conversation" json:"conversation,omitempty"`
}

// SummaryFacet answers "how are you doing?": a status rule, up to a few
// KPIs, an optional sparkline series and a one-line headline template.
type SummaryFacet struct {
	Status    *StatusRule `yaml:"status" json:"status,omitempty"`
	KPIs      []string    `yaml:"kpis" json:"kpis"`
	Sparkline string      `yaml:"sparkline" json:"sparkline,omitempty"`
	Headline  string      `yaml:"headline" json:"headline,omitempty"`
}

// StatusRule derives a status from one KPI against thresholds. Direction
// "above" means higher is worse; "below" means lower is worse.
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

// DetailFacet answers "show me everything": ordered panels plus typed
// drilldown links to other report definitions.
type DetailFacet struct {
	Panels    []PanelSpec `yaml:"panels" json:"panels"`
	Drilldown []string    `yaml:"drilldown" json:"drilldown,omitempty"`
}

// PanelSpec binds a visual primitive to a data reference of the form
// "series/<name>", "table/<name>", "spans/<span-name>", "kpi/<name>" or the
// literal "events".
type PanelSpec struct {
	Title     string `yaml:"title" json:"title"`
	Primitive string `yaml:"primitive" json:"primitive"`
	Data      string `yaml:"data" json:"data"`
	// Section places the panel in the detail page's reading order (see
	// PanelSections). Optional: a definition that names no sections
	// renders as a flat ordered list, exactly as before.
	Section string `yaml:"section,omitempty" json:"section,omitempty"`
	// Stance overrides the stance affinity its section implies. Needed
	// rarely and on purpose: an at-risk strategic bet legitimately sits
	// in `exceptions`, which is otherwise an operational section. Do not
	// declare a stance the section already implies.
	Stance string `yaml:"stance,omitempty" json:"stance,omitempty"`
}

// Stances returns the stances this panel serves: its own if it declared
// one, its section's affinity otherwise, and every stance if it declared
// neither.
func (p PanelSpec) Stances() []string {
	if p.Stance != "" {
		return []string{p.Stance}
	}
	if st, ok := SectionStances[p.Section]; ok {
		return append([]string(nil), st...)
	}
	return append([]string(nil), Stances...)
}

// PlanFacet answers "what are we going to do?": the declared board a
// producer publishes items onto.
//
// States is the spine and the only required field. THE DECLARED ORDER IS
// THE SEMANTICS — left to right is progress, the last state is terminal —
// and that is what lets the control plane roll a board up (order the
// columns, count them, drop finished work first when bounding) without
// knowing what any state means. A board whose columns are a set rather
// than a sequence could only be aggregated by the report that wrote it,
// which is the leak the scope contract exists to prevent.
type PlanFacet struct {
	States []string `yaml:"states" json:"states"`
	Lanes  []string `yaml:"lanes,omitempty" json:"lanes,omitempty"`
	// Horizons are declared nearest-first — now/next/later, or the
	// quarters in order. Same rule as States.
	Horizons []string `yaml:"horizons,omitempty" json:"horizons,omitempty"`
	// Commitments are declared STRONGEST-FIRST. The order is load-bearing:
	// it is the ranking a bounded universe-level board drops cards by.
	Commitments []string `yaml:"commitments,omitempty" json:"commitments,omitempty"`
	// Levels bounds which of WorkLevels this board publishes. Declaring it
	// is how a delivery board says "we publish tasks, not goals", and how
	// a portfolio says the opposite.
	Levels []string `yaml:"levels,omitempty" json:"levels,omitempty"`
	// WIPLimits caps occupancy per state. A board that cannot state its
	// limit cannot show that it is exceeded, which is most of what a board
	// is read for. A state with no entry has no limit — which is not the
	// same as a limit of zero.
	WIPLimits map[string]int `yaml:"wip_limits,omitempty" json:"wip_limits,omitempty"`
	// SizeUnit says what Item.Size counts: points, days, eur. Declared
	// once here rather than per item, the same convention as a KPI's unit.
	SizeUnit string        `yaml:"size_unit,omitempty" json:"size_unit,omitempty"`
	Baseline *PlanBaseline `yaml:"baseline,omitempty" json:"baseline,omitempty"`
}

// PlanBaseline names what the plan is measured against, which is the whole
// content of the strategic stance ("measured against a plan") seen from the
// other side. Ref uses the panel data grammar, restricted to kpi/<name> or
// series/<name>: the committed number, or the committed curve a burndown is
// drawn against.
//
// It points into the data contract rather than carrying a value, because a
// baseline is per-scope — one squad's commitment is not another's — and the
// data contract is already where per-scope values live. Two fields rather
// than one because a bare string is either an unresolvable label, useless to
// a chart, or a ref with no display name, which a chart cannot label.
type PlanBaseline struct {
	Name string `yaml:"name" json:"name"`
	Ref  string `yaml:"ref" json:"ref"`
}

// StateRank returns the declared index of a state, or -1 when the state is
// not one this plan declares. Progress order, and the only thing about two
// different plans that can be compared.
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

// Terminal reports whether state is this plan's last declared state — done,
// by declaration order rather than by a magic name, so a board whose final
// column is "shipped" or "archived" needs no special case.
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

// facetOrder is the canonical facet presentation order. `plan` is appended
// rather than slotted next to `timeline` (its forward-looking twin) on
// purpose: this order is presentational, and appending leaves the prefix
// every existing consumer and test already reads byte-identical.
var facetOrder = []string{"summary", "detail", "timeline", "pulse", "media", "conversation", "plan"}

// StanceNames returns the stances this report can be read from, in
// canonical order — every stance when the definition declares none,
// because a report that does not say is readable from any footing.
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

// ServesStance reports whether this report can be read from a stance. A
// definition declaring none serves all of them — note this is the
// OPPOSITE of the category rule, where an absent category excludes the
// report from that filter. A category is a claim about subject matter
// and its absence means "not about that"; a stance is a claim about how
// the numbers may be judged, and its absence means "no opinion".
func (d *Definition) ServesStance(stance string) bool {
	if stance == "" || len(d.Stances) == 0 {
		return true
	}
	return contains(d.Stances, stance)
}

// FacetNames returns which facets the definition exposes, in canonical
// order (summary, detail, timeline, pulse, media, conversation, plan).
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
//
// Validation is exhaustive rather than fail-fast: every problem found is
// collected and returned together in a single error wrapping
// ErrInvalidDefinition, with indexed messages (facets.detail.panels[2]: ...)
// so an operator can find the offending line in one pass.
//
// A definition that declares extends is only structurally validated here:
// its facet references may point at data inherited from the parent, so
// reference resolution is deferred to LoadLibraryDir, which resolves the
// extends chain first.
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

// validateCore checks everything that does not depend on the (possibly
// inherited) data contract: exact version/kind, the name pattern, and that
// every vocabulary-typed value is in its vocabulary.
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

	// Aggregation policy strings must parse.
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
			if p, err := ParsePolicy(tm.KPIs[name]); err == nil && !contains(kpiPolicies, p.Name) {
				problems = append(problems, fmt.Errorf("%s: policy %q does not apply to kpis, want one of %s", field, p.Name, strings.Join(kpiPolicies, "|")))
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

	// The plan facet's vocabularies. Each is an ORDERED list, so a repeat
	// is not a harmless duplicate — it makes the order ambiguous, and the
	// order is the entire basis on which a board can be rolled up.
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

	// Data contract: names present and unique, severities in vocabulary.
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

	// Facet-local vocabulary checks.
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

// validateRefs checks every facet reference against the data contract. For
// a definition using extends this runs only after inheritance has been
// resolved, so inherited data participates.
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

	// A baseline points into the data contract rather than carrying a
	// value, so it resolves like any other ref — but only against the two
	// kinds a plan can be measured against: the committed number, or the
	// committed curve.
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

// validatePanelData resolves one panel data reference against the data
// contract and the timeline facet's declared spans.
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

// sortedIntKeys returns a map's keys in sorted order, so validation problems
// over wip_limits are reported in a stable sequence.
func sortedIntKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// validateRateLimit checks the "N/min" producer-side bound format.
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

// headlinePlaceholders extracts the {{name}} placeholders in a headline
// template, in order of appearance.
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
