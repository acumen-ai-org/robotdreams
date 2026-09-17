package reporting

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// sparklineCap is the maximum number of merged sparkline points kept in a
// SummaryView; only the most recent points survive.
const sparklineCap = 200

// KPIValue is one aggregated KPI in a SummaryView.
//
// Target and Variance are present only when the KPI has a target to be
// judged against — from the instances' own Instance.Targets, falling
// back to the definition's KPISpec.Target. Variance is signed, actual
// minus target, and Direction says which sign is bad. A KPI with no
// target carries all three as zero values and renders as a plain number,
// exactly as every KPI did before targets existed.
type KPIValue struct {
	Name      string   `json:"name"`
	Value     float64  `json:"value"`
	Unit      string   `json:"unit"`
	Policy    string   `json:"policy"`
	Target    *float64 `json:"target,omitempty"`
	Variance  *float64 `json:"variance,omitempty"`
	Direction string   `json:"direction,omitempty"`
}

// SummaryView is one definition's summary facet rolled up to one target
// scope from the instances at or below it.
type SummaryView struct {
	Definition  string   `json:"definition"`
	Description string   `json:"description"`
	Categories  []string `json:"categories"`
	// Stances is the footing(s) this report can be read from; empty means
	// any. Carried to the client so a tile can say which it answers.
	Stances  []string `json:"stances,omitempty"`
	Facets   []string `json:"facets"`
	Scope    string   `json:"scope"`
	Status   string   `json:"status"`
	Headline string   `json:"headline"`
	// HeadlinePartial marks a headline that is ONE contributor's sentence
	// standing in for several — the honest signal for the case where a
	// report's prose cannot be rolled up. A client should say so rather
	// than presenting it as the whole picture.
	HeadlinePartial bool          `json:"headline_partial,omitempty"`
	KPIs            []KPIValue    `json:"kpis"`
	Sparkline       []SeriesPoint `json:"sparkline,omitempty"`
	Instances       int           `json:"instances"` // contributing instance count
	Scopes          []string      `json:"scopes"`    // distinct contributing scope paths
	// Contributors names each instance this view was computed from: where
	// it came from, and WHO wrote it. The producer is recorded at ingest
	// (the authenticated worker) and was until now stored and never read
	// back, which left a reader unable to ask the obvious question about
	// an aggregate — which node actually said this. Scopes is kept beside
	// it unchanged: it is the same information minus the producer, and
	// clients already read it.
	Contributors   []ContributorView `json:"contributors,omitempty"`
	LastProducedAt time.Time         `json:"last_produced_at"`
	// Folded is how many stored instances the time stage folded into the
	// contributors above when the view was read over a period: three
	// reports from one site in one week become one contributor, and this
	// says three. Zero, and omitted, for a view read without a period —
	// where Instances already counts every instance that contributed.
	Folded int `json:"folded,omitempty"`
}

// ContributorView is one instance behind an aggregate: the scope it was
// written at, the worker that wrote it, and when. Producer is empty for
// an instance submitted before producers were recorded, or by a caller
// that set neither the field nor an authenticated identity.
type ContributorView struct {
	Scope      string    `json:"scope"`
	Producer   string    `json:"producer,omitempty"`
	ProducedAt time.Time `json:"produced_at"`
}

// Contributors filters instances to the definition's contributing set at or
// below scope — the NEWEST instance per exact scope path, later submissions
// winning ties — ordered by produced_at ascending.
//
// Exported because three readers must agree on it: the summary tile, the
// detail page's panels, and a plan roll-up. A view that resolved panels over
// a different set than the summary would show data from an instance the
// summary has already superseded.
//
// This is ContributorsIn without a period: the "now" reading.
func Contributors(def *Definition, scope string, instances []*Instance) []*Instance {
	return ContributorsIn(def, scope, instances, nil)
}

// ContributorsIn is the contributing set at or below scope read over a
// period — the time stage of the roll-up, which runs BEFORE the scope
// stage Aggregate applies.
//
// With a nil period it is exactly Contributors: the newest instance per
// exact scope path, whenever it was produced. With a period, every
// instance whose produced_at falls in it (half-open, Period.Contains) is
// taken, and each scope path's instances are folded into ONE synthetic
// instance by the definition's scope.aggregation.time policies:
//
//   - status: worst by default (a week with one critical day was a
//     critical week), or latest;
//   - headline: the producers' own prose cannot be recomputed, only
//     chosen between — latest by default, top(n) joins the n most
//     recent, synthesize degrades to latest. Where no instance in the
//     period carries prose the fold leaves the headline empty, so the
//     summary template is later rendered against the FOLDED numbers and
//     the sentence agrees with the KPI row beside it;
//   - kpis: per KPI, latest by default; a definition declares sum for
//     its counters. A target folds with the same policy as its metric,
//     as it does across scopes. An instance that carries no value for a
//     KPI contributes nothing to it;
//   - series and events are merged in time order; tables and plan items
//     are snapshots and come from the newest instance.
//
// The synthetic instance carries the newest instance's producer and
// produced_at, so a contributor entry still says who last wrote at that
// path and when. The result is ordered like Contributors', produced_at
// ascending, so everything downstream reads it the same way.
func ContributorsIn(def *Definition, scope string, instances []*Instance, period *Period) []*Instance {
	if period == nil {
		return latestPerPath(def, scope, instances)
	}

	byPath := map[string][]*Instance{}
	var order []string
	for _, in := range inPeriod(def, scope, instances, period) {
		if _, seen := byPath[in.Scope]; !seen {
			order = append(order, in.Scope)
		}
		byPath[in.Scope] = append(byPath[in.Scope], in)
	}

	contrib := make([]*Instance, 0, len(byPath))
	for _, path := range order {
		group := byPath[path]
		// Oldest first; a later submission wins a tie, as in Contributors.
		sort.SliceStable(group, func(i, j int) bool { return group[i].ProducedAt.Before(group[j].ProducedAt) })
		contrib = append(contrib, foldOverTime(def, group))
	}
	sort.SliceStable(contrib, func(i, j int) bool {
		if !contrib[i].ProducedAt.Equal(contrib[j].ProducedAt) {
			return contrib[i].ProducedAt.Before(contrib[j].ProducedAt)
		}
		return contrib[i].Scope < contrib[j].Scope
	})
	return contrib
}

// latestPerPath is the no-period contributing set: newest per exact
// scope path, later submissions winning ties, produced_at ascending.
func latestPerPath(def *Definition, scope string, instances []*Instance) []*Instance {
	latest := map[string]*Instance{}
	for _, in := range instances {
		if in == nil {
			continue
		}
		if in.Definition != "" && in.Definition != def.Name {
			continue
		}
		if !ScopeWithin(scope, in.Scope) {
			continue
		}
		if cur, ok := latest[in.Scope]; !ok || !in.ProducedAt.Before(cur.ProducedAt) {
			latest[in.Scope] = in
		}
	}

	contrib := make([]*Instance, 0, len(latest))
	for _, in := range latest {
		contrib = append(contrib, in)
	}
	sort.SliceStable(contrib, func(i, j int) bool {
		if !contrib[i].ProducedAt.Equal(contrib[j].ProducedAt) {
			return contrib[i].ProducedAt.Before(contrib[j].ProducedAt)
		}
		return contrib[i].Scope < contrib[j].Scope
	})
	return contrib
}

// inPeriod is every instance for the definition at or below scope whose
// produced_at falls inside the period, in input order.
func inPeriod(def *Definition, scope string, instances []*Instance, period *Period) []*Instance {
	var out []*Instance
	for _, in := range instances {
		if in == nil {
			continue
		}
		if in.Definition != "" && in.Definition != def.Name {
			continue
		}
		if !ScopeWithin(scope, in.Scope) {
			continue
		}
		if !period.Contains(in.ProducedAt) {
			continue
		}
		out = append(out, in)
	}
	return out
}

// foldOverTime folds one scope path's instances, oldest first, into one
// synthetic instance per the definition's time policies; see
// ContributorsIn for the rules. A single instance folds to itself.
func foldOverTime(def *Definition, group []*Instance) *Instance {
	if len(group) == 1 {
		return group[0]
	}
	newest := group[len(group)-1]
	out := &Instance{
		Definition: newest.Definition,
		Scope:      newest.Scope,
		Producer:   newest.Producer,
		ProducedAt: newest.ProducedAt,
		Tables:     newest.Tables,
		Items:      newest.Items,
	}

	// Status: explicit on the synthetic instance, because the status
	// rule applied later to a SUMMED kpi would judge a week's total
	// against a threshold written for a day.
	if def.Scope.Aggregation.TimeStatusPolicy().Name == "latest" {
		out.Status = instanceStatus(def, newest)
	} else {
		worst := StatusOK
		for _, in := range group {
			if s := instanceStatus(def, in); statusRank(s) > statusRank(worst) {
				worst = s
			}
		}
		out.Status = worst
	}

	// Headline: only prose folds; a template is rendered downstream.
	precomputed := false
	for _, in := range group {
		if in.Headline != "" {
			precomputed = true
			break
		}
	}
	if precomputed {
		n := 1
		if p := def.Scope.Aggregation.TimeHeadlinePolicy(); p.Name == "top" {
			n = p.N
		}
		var lines []string
		for i := len(group) - 1; i >= 0 && len(lines) < n; i-- {
			if h := instanceHeadline(def, group[i]); h != "" {
				lines = append(lines, h)
			}
		}
		out.Headline = strings.Join(lines, "; ")
	}

	// KPIs and targets, per KPI, per the declared time policy.
	for _, spec := range def.Data.KPIs {
		policy := def.Scope.Aggregation.TimeKPIPolicy(spec.Name)
		var vals, targets []float64
		for _, in := range group {
			if v, ok := in.KPIs[spec.Name]; ok {
				vals = append(vals, v)
			}
			if t, ok := in.Targets[spec.Name]; ok {
				targets = append(targets, t)
			}
		}
		if len(vals) > 0 || policy.Name == "count" {
			if out.KPIs == nil {
				out.KPIs = map[string]float64{}
			}
			out.KPIs[spec.Name] = applyKPIPolicy(policy, vals)
		}
		if len(targets) > 0 {
			if out.Targets == nil {
				out.Targets = map[string]float64{}
			}
			out.Targets[spec.Name] = applyKPIPolicy(policy, targets)
		}
	}

	// Series and events: merged, time ascending.
	for _, in := range group {
		for name, pts := range in.Series {
			if out.Series == nil {
				out.Series = map[string][]SeriesPoint{}
			}
			out.Series[name] = append(out.Series[name], pts...)
		}
		out.Events = append(out.Events, in.Events...)
	}
	for name := range out.Series {
		pts := out.Series[name]
		sort.SliceStable(pts, func(i, j int) bool { return pts[i].T.Before(pts[j].T) })
	}
	sort.SliceStable(out.Events, func(i, j int) bool { return out.Events[i].T.Before(out.Events[j].T) })
	return out
}

// Aggregate rolls the given instances up to one SummaryView at the target
// scope, per the definition's aggregation spec.
//
// Instances are filtered to those for this definition at or below the
// target scope, and only the LATEST instance per exact scope path
// contributes: a newer instance supersedes older ones from the same scope
// for the same definition.
//
// Status is computed per instance (its Status override if set, otherwise
// the summary facet's status rule applied to that instance's KPI), then
// combined across instances with the declared status policy — worst by
// default, latest supported; anything else falls back to worst.
//
// Headline: where the definition declares a template and no contributor
// carries precomputed prose, the template is rendered against the
// AGGREGATED KPI values, so the sentence and the KPI row on the same tile
// always describe the same numbers.
//
// Where they do carry prose — a producer's own words, which cannot be
// recomputed at a higher scope — the declared policy applies: latest and
// top(n) as named (most recent first, joined with "; "), and synthesize
// DEGRADES to top(1), because this library has no model behind it. Per
// contracts/aggregations.yaml that degradation must be explicit, never
// silent: HeadlinePartial says when one contributor's sentence is
// standing in for many, so a client can label it rather than pass it off
// as the whole picture.
//
// KPI values follow the per-KPI declared policy
// (sum/avg/min/max/p50/p95/latest/count; default latest). The KPI list is
// the summary facet's declared KPIs when present, otherwise every KPI in
// the data contract. Percentiles use the nearest-rank method.
//
// The sparkline merges the declared series' points from all contributing
// instances, sorted by time, capped to the most recent 200 points.
//
// Aggregate is AggregateIn without a period.
func Aggregate(def *Definition, scope string, instances []*Instance) *SummaryView {
	return AggregateIn(def, scope, instances, nil)
}

// AggregateIn is Aggregate read over a period: the contributing set is
// ContributorsIn's — each scope path's instances in the period folded
// into one by the time policies — and the scope roll-up then applies to
// it unchanged. A nil period is Aggregate exactly.
func AggregateIn(def *Definition, scope string, instances []*Instance, period *Period) *SummaryView {
	view := &SummaryView{
		Definition:  def.Name,
		Description: def.Description,
		Categories:  append([]string(nil), def.Categories...),
		Stances:     append([]string(nil), def.Stances...),
		Facets:      def.FacetNames(),
		Scope:       scope,
		KPIs:        []KPIValue{},
		Scopes:      []string{},
	}

	contrib := ContributorsIn(def, scope, instances, period)
	if period != nil {
		view.Folded = len(inPeriod(def, scope, instances, period))
	}

	view.Instances = len(contrib)
	for _, in := range contrib {
		view.Scopes = append(view.Scopes, in.Scope)
		view.Contributors = append(view.Contributors, ContributorView{
			Scope:      in.Scope,
			Producer:   in.Producer,
			ProducedAt: in.ProducedAt,
		})
	}
	sort.Strings(view.Scopes)
	// Contributors keeps Contributors() own ordering — produced_at
	// ascending — because "who wrote this, and when" reads as a history.
	// Scopes stays sorted by name, as it always was.
	if len(contrib) == 0 {
		return view
	}
	view.LastProducedAt = contrib[len(contrib)-1].ProducedAt

	view.Status = aggregateStatus(def, contrib)
	// KPIs first: the headline is rendered against them, so that the
	// sentence and the numbers on a tile cannot disagree.
	view.KPIs = aggregateKPIs(def, contrib)
	view.Headline, view.HeadlinePartial = aggregateHeadline(def, contrib, view.KPIs)
	view.Sparkline = aggregateSparkline(def, contrib)
	return view
}

// instanceStatus is one instance's status: its override if set, else the
// summary status rule applied to that instance's KPI, else ok.
func instanceStatus(def *Definition, in *Instance) string {
	if in.Status != "" {
		return in.Status
	}
	s := def.Facets.Summary
	if s == nil || s.Status == nil {
		return StatusOK
	}
	v, ok := in.KPIs[s.Status.From]
	if !ok {
		return StatusOK
	}
	worse := func(threshold float64) bool {
		if s.Status.Direction == "below" {
			return v <= threshold
		}
		return v >= threshold
	}
	switch {
	case worse(s.Status.CriticalAt):
		return StatusCritical
	case worse(s.Status.WarnAt):
		return StatusWarn
	default:
		return StatusOK
	}
}

func statusRank(s string) int {
	switch s {
	case StatusCritical:
		return 2
	case StatusWarn:
		return 1
	default:
		return 0
	}
}

// aggregateStatus combines per-instance statuses with the declared status
// policy: worst by default, latest supported, anything else worst.
func aggregateStatus(def *Definition, contrib []*Instance) string {
	policy := Policy{Name: "worst"}
	if p, err := ParsePolicy(def.Scope.Aggregation.Status); err == nil {
		policy = p
	}
	if policy.Name == "latest" {
		return instanceStatus(def, contrib[len(contrib)-1])
	}
	worst := StatusOK
	for _, in := range contrib {
		if s := instanceStatus(def, in); statusRank(s) > statusRank(worst) {
			worst = s
		}
	}
	return worst
}

// aggregateHeadline combines per-instance headlines with the declared
// headline policy; see Aggregate for the synthesize degradation rule.
//
// The rule that matters most here: when the definition declares a
// headline TEMPLATE, it is rendered against the aggregated KPI values
// rather than against one contributor's. Anything else puts a sentence
// and a KPI row on the same card that disagree — a workflow tile reading
// "1 blocked" beside a runs_blocked of 18, because the sentence came
// from one squad and the number from nineteen. That is not a degradation
// a reader can detect, which makes it the worst kind.
//
// Rendering a declared template against rolled-up numbers is not model
// synthesis; it is the same substitution used for a single instance,
// applied to the values the summary is actually reporting. Genuine
// synthesis — prose a model writes — still degrades to top(1), and now
// says so through Synthesized.
func aggregateHeadline(def *Definition, contrib []*Instance, kpis []KPIValue) (string, bool) {
	// A precomputed headline is prose from the producer: it cannot be
	// recomputed at a higher scope, only chosen between.
	precomputed := false
	for _, in := range contrib {
		if in.Headline != "" {
			precomputed = true
			break
		}
	}

	// Only synthesize. `latest` and `top(n)` are explicit requests for
	// contributors' own sentences, and honouring the request is the
	// whole point of having a policy vocabulary; `synthesize` is a
	// request for a sentence about the WHOLE, which is exactly what a
	// template rendered from the roll-up is.
	policy, perr := ParsePolicy(def.Scope.Aggregation.Headline)
	wantsSynthesis := perr == nil && policy.Name == "synthesize"

	if wantsSynthesis && !precomputed && len(contrib) > 1 {
		if s := def.Facets.Summary; s != nil && s.Headline != "" {
			vals := make(map[string]float64, len(kpis))
			for _, k := range kpis {
				vals[k.Name] = k.Value
			}
			// Placeholders naming a KPI outside the summary shortlist are
			// aggregated on demand, so a template is never rendered half
			// from the roll-up and half from nowhere.
			for _, spec := range def.Data.KPIs {
				if _, ok := vals[spec.Name]; ok {
					continue
				}
				if !strings.Contains(s.Headline, "{{"+spec.Name+"}}") {
					continue
				}
				policy, err := ParsePolicy(def.Scope.Aggregation.KPIs[spec.Name])
				if err != nil {
					policy = Policy{Name: "latest"}
				}
				var xs []float64
				for _, in := range contrib {
					if v, ok := in.KPIs[spec.Name]; ok {
						xs = append(xs, v)
					}
				}
				vals[spec.Name] = applyKPIPolicy(policy, xs)
			}
			return renderHeadline(s.Headline, vals), false
		}
	}

	n := 1
	if perr == nil {
		switch policy.Name {
		case "top":
			n = policy.N
		case "synthesize", "latest":
			n = 1 // synthesize degrades to top(1), explicitly
		}
	}
	var lines []string
	for i := len(contrib) - 1; i >= 0 && len(lines) < n; i-- {
		if h := instanceHeadline(def, contrib[i]); h != "" {
			lines = append(lines, h)
		}
	}
	// Degraded when one contributor's sentence is standing in for many.
	return strings.Join(lines, "; "), len(contrib) > n
}

// renderHeadline substitutes {{kpi}} placeholders from a value map,
// leaving unknown ones intact so a template bug reads as a template bug.
func renderHeadline(tmpl string, vals map[string]float64) string {
	return headlinePlaceholderRE.ReplaceAllStringFunc(tmpl, func(m string) string {
		name := headlinePlaceholderRE.FindStringSubmatch(m)[1]
		if v, ok := vals[name]; ok {
			return FormatKPI(v)
		}
		return m
	})
}

// instanceHeadline is one instance's headline: its precomputed headline if
// set, else the summary facet's template rendered against its KPIs.
func instanceHeadline(def *Definition, in *Instance) string {
	if in.Headline != "" {
		return in.Headline
	}
	s := def.Facets.Summary
	if s == nil || s.Headline == "" {
		return ""
	}
	return renderHeadline(s.Headline, in.KPIs)
}

// aggregateKPIs computes each declared KPI per its declared policy.
func aggregateKPIs(def *Definition, contrib []*Instance) []KPIValue {
	names := []string{}
	if s := def.Facets.Summary; s != nil && len(s.KPIs) > 0 {
		names = s.KPIs
	} else {
		for _, k := range def.Data.KPIs {
			names = append(names, k.Name)
		}
	}
	units := map[string]string{}
	specs := map[string]KPISpec{}
	for _, k := range def.Data.KPIs {
		units[k.Name] = k.Unit
		specs[k.Name] = k
	}

	out := make([]KPIValue, 0, len(names))
	for _, name := range names {
		policyStr := def.Scope.Aggregation.KPIs[name]
		if policyStr == "" {
			policyStr = "latest"
		}
		policy, err := ParsePolicy(policyStr)
		if err != nil {
			policy, policyStr = Policy{Name: "latest"}, "latest"
		}

		// Values in contribution order, which is time ascending.
		// Targets are collected alongside, one per contributing
		// instance, falling back to the definition default so a scope
		// that declares no target of its own still inherits one.
		spec := specs[name]
		var vals, targets []float64
		for _, in := range contrib {
			if v, ok := in.KPIs[name]; ok {
				vals = append(vals, v)
			}
			if t, ok := in.Targets[name]; ok {
				targets = append(targets, t)
			} else if spec.Target != nil {
				targets = append(targets, *spec.Target)
			}
		}

		kv := KPIValue{
			Name:   name,
			Value:  applyKPIPolicy(policy, vals),
			Unit:   units[name],
			Policy: policyStr,
		}
		// A target rolls up with the SAME policy as its metric: summed
		// KPIs sum their targets, averaged ones average them. Any other
		// choice makes the variance meaningless at every level above a
		// leaf — a sum compared against one squad's target is noise.
		if len(targets) > 0 {
			target := applyKPIPolicy(policy, targets)
			variance := kv.Value - target
			kv.Target, kv.Variance, kv.Direction = &target, &variance, spec.Direction
		}
		out = append(out, kv)
	}
	return out
}

// applyKPIPolicy reduces time-ordered values with one KPI policy; policies
// that make no sense for scalars fall back to latest.
func applyKPIPolicy(p Policy, vals []float64) float64 {
	if p.Name == "count" {
		return float64(len(vals))
	}
	if len(vals) == 0 {
		return 0
	}
	switch p.Name {
	case "sum":
		var t float64
		for _, v := range vals {
			t += v
		}
		return t
	case "avg":
		var t float64
		for _, v := range vals {
			t += v
		}
		return t / float64(len(vals))
	case "min":
		m := vals[0]
		for _, v := range vals[1:] {
			if v < m {
				m = v
			}
		}
		return m
	case "max":
		m := vals[0]
		for _, v := range vals[1:] {
			if v > m {
				m = v
			}
		}
		return m
	case "p50":
		return percentile(vals, 50)
	case "p95":
		return percentile(vals, 95)
	default: // latest, and any policy that does not apply to scalars
		return vals[len(vals)-1]
	}
}

// percentile is the nearest-rank percentile of the values.
func percentile(vals []float64, p float64) float64 {
	sorted := append([]float64(nil), vals...)
	sort.Float64s(sorted)
	rank := int(math.Ceil(p / 100 * float64(len(sorted))))
	if rank < 1 {
		rank = 1
	}
	return sorted[rank-1]
}

// aggregateSparkline merges the summary sparkline series' points across
// instances, time ascending, capped to the most recent points.
func aggregateSparkline(def *Definition, contrib []*Instance) []SeriesPoint {
	s := def.Facets.Summary
	if s == nil || s.Sparkline == "" {
		return nil
	}
	var pts []SeriesPoint
	for _, in := range contrib {
		pts = append(pts, in.Series[s.Sparkline]...)
	}
	sort.SliceStable(pts, func(i, j int) bool { return pts[i].T.Before(pts[j].T) })
	if len(pts) > sparklineCap {
		pts = pts[len(pts)-sparklineCap:]
	}
	return pts
}

// MergeEvents merges event batches into one stream sorted by time
// ascending. Events are expected to arrive already tagged with their
// origin Scope and Definition (the collector tags them when it draws them
// from an instance); the tags are preserved through the merge.
//
// If limit > 0 and the merged stream exceeds it, only the most recent limit
// events are kept — but critical-severity events are never dropped before
// non-critical ones: criticals only start falling off (oldest first) once
// the stream is criticals-only.
func MergeEvents(limit int, batches ...[]Event) []Event {
	var out []Event
	for _, b := range batches {
		out = append(out, b...)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].T.Before(out[j].T) })
	if limit <= 0 || len(out) <= limit {
		return out
	}
	var crit, rest []Event
	for _, e := range out {
		if e.Severity == SeverityCritical {
			crit = append(crit, e)
		} else {
			rest = append(rest, e)
		}
	}
	if len(crit) >= limit {
		return crit[len(crit)-limit:]
	}
	rest = rest[len(rest)-(limit-len(crit)):]
	merged := append(crit, rest...)
	sort.SliceStable(merged, func(i, j int) bool { return merged[i].T.Before(merged[j].T) })
	return merged
}

// FormatKPI formats a KPI value for headlines and badges: at most two
// decimals with trailing zeros (and a bare decimal point) trimmed, so 14.0
// renders as "14" and 32.50 as "32.5".
func FormatKPI(v float64) string {
	s := strconv.FormatFloat(v, 'f', 2, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" || s == "-" || s == "-0" {
		return "0"
	}
	return s
}

// planItemCap backstops an unbounded `merge`. A board that would render half
// a million cards is not a view, and the alternative to a cap here is every
// consumer inventing its own. Cards are dropped in keep-order, so what goes
// first is finished work and distant work.
const planItemCap = 500

// PlanColumn is one declared state rolled up.
//
// Items and Size are EXACT across every contributor whatever the items
// policy; Shown is how many cards survived the bound. Bounding the cards must
// never move a count — a rolled-up board that under-reported its own totals
// would be a lie about how much work there is.
type PlanColumn struct {
	State string  `json:"state"`
	Items int     `json:"items"`
	Size  float64 `json:"size,omitempty"`
	Shown int     `json:"shown"`
	// Limit is the declared WIP ceiling for this state, summed across
	// contributors: six squads each allowed four in progress means the
	// realm is allowed twenty-four. Zero means no limit was declared.
	Limit int `json:"limit,omitempty"`
}

// PlanBucket is one declared lane, horizon or commitment rolled up.
//
// An item carrying no value for that axis is counted in no bucket, so a
// bucket total may be less than PlanView.Total. That is deliberate: an
// "(other)" bucket would put a value in the vocabulary the definition never
// declared, which is exactly what these contracts exist to prevent.
type PlanBucket struct {
	Name  string  `json:"name"`
	Items int     `json:"items"`
	Size  float64 `json:"size,omitempty"`
}

// PlanView is one definition's plan facet rolled up to one scope.
//
// Everything in it comes from three things and nothing else: the plan
// facet's declared vocabularies, the declared items policy, and the
// instances beneath the scope. That is the scope-contract test applied to a
// board — no consumer needs to know what "in review" means at any of the
// teams below it, and nothing in this file matches an item's title, state,
// lane or level against a literal.
//
// Note what is absent: no overdue count. This package has no clock, and
// giving it one to compare Due against would be a new dependency for a
// derivation any client can do with the Due field it already receives.
type PlanView struct {
	Definition string `json:"definition"`
	Scope      string `json:"scope"`
	Policy     string `json:"policy"`

	States      []PlanColumn `json:"states"`
	Lanes       []PlanBucket `json:"lanes,omitempty"`
	Horizons    []PlanBucket `json:"horizons,omitempty"`
	Commitments []PlanBucket `json:"commitments,omitempty"`

	Items []Item `json:"items"`
	// Total and Blocked count every contributed item, before any bounding.
	Total     int  `json:"total"`
	Blocked   int  `json:"blocked"`
	Truncated bool `json:"truncated,omitempty"`

	SizeUnit string        `json:"size_unit,omitempty"`
	Baseline *PlanBaseline `json:"baseline,omitempty"`

	Instances      int       `json:"instances"`
	Scopes         []string  `json:"scopes"`
	LastProducedAt time.Time `json:"last_produced_at"`
}

// AggregatePlan rolls the given instances up to one PlanView at the target
// scope, per the definition's scope.aggregation.items policy. Returns nil
// when the definition exposes no plan facet.
//
// Contributor selection is Contributors', identical to the summary's, so a
// board never shows cards from an instance the tile above it has dropped.
//
// The four legal policies, by altitude:
//
//	merge      every card, ordered, capped at planItemCap   site / realm
//	sample(n)  at most n per declared column                 realm / world
//	top(n)     at most n overall                             world
//	count      no cards at all, columns only                 universe
//
// Anything else is rejected at definition-validation time; a hand-built
// Definition that slips through degrades to merge and says so in Policy.
//
// AggregatePlan is AggregatePlanIn without a period.
func AggregatePlan(def *Definition, scope string, instances []*Instance) *PlanView {
	return AggregatePlanIn(def, scope, instances, nil)
}

// AggregatePlanIn is AggregatePlan read over a period. A plan is a
// snapshot, so the time stage contributes each path's NEWEST board in
// the period — there is no arithmetic to do on a card over a week either.
func AggregatePlanIn(def *Definition, scope string, instances []*Instance, period *Period) *PlanView {
	facet := def.Facets.Plan
	if facet == nil {
		return nil
	}

	view := &PlanView{
		Definition: def.Name,
		Scope:      scope,
		Items:      []Item{},
		States:     make([]PlanColumn, 0, len(facet.States)),
		Scopes:     []string{},
		SizeUnit:   facet.SizeUnit,
		Baseline:   facet.Baseline,
	}

	policy := Policy{Name: "merge"}
	if p, err := ParsePolicy(def.Scope.Aggregation.Items); err == nil && contains(ItemPolicies, p.Name) {
		policy = p
	}
	view.Policy = policy.Name
	if policy.N > 0 {
		view.Policy = fmt.Sprintf("%s(%d)", policy.Name, policy.N)
	}

	contrib := ContributorsIn(def, scope, instances, period)
	view.Instances = len(contrib)
	for _, in := range contrib {
		view.Scopes = append(view.Scopes, in.Scope)
	}
	sort.Strings(view.Scopes)

	// Collect and stamp. Origin tags are overwritten unconditionally, the
	// same posture events take: an item's identity is (Scope, ID) at read
	// time, so two teams may safely publish the same id.
	var all []Item
	for _, in := range contrib {
		for _, it := range in.Items {
			it.Scope = in.Scope
			it.Definition = def.Name
			all = append(all, it)
		}
	}
	if len(contrib) > 0 {
		view.LastProducedAt = contrib[len(contrib)-1].ProducedAt
	}

	// Counts BEFORE any bound, always.
	view.Total = len(all)
	byState := map[string]*PlanColumn{}
	for i := range facet.States {
		st := facet.States[i]
		// Limits multiply out across contributors: each team's ceiling is
		// its own, so a realm's allowance is the sum of the allowances
		// beneath it. One team over its limit still shows at the realm as
		// a column over the realm's total, which is the honest reading —
		// the aggregate is over-committed by exactly that much.
		col := PlanColumn{State: st, Limit: facet.WIPLimits[st] * len(contrib)}
		view.States = append(view.States, col)
	}
	for i := range view.States {
		byState[view.States[i].State] = &view.States[i]
	}
	lanes := newBuckets(facet.Lanes)
	horizons := newBuckets(facet.Horizons)
	commitments := newBuckets(facet.Commitments)
	for _, it := range all {
		if it.Blocked() {
			view.Blocked++
		}
		if col := byState[it.State]; col != nil {
			col.Items++
			col.Size += it.Size
		}
		tallyBucket(lanes, it.Lane, it.Size)
		tallyBucket(horizons, it.Horizon, it.Size)
		tallyBucket(commitments, it.Commitment, it.Size)
	}
	view.Lanes = bucketList(facet.Lanes, lanes)
	view.Horizons = bucketList(facet.Horizons, horizons)
	view.Commitments = bucketList(facet.Commitments, commitments)

	if policy.Name == "count" {
		return view
	}

	// Order once, worst-first within the flow, then bound.
	sort.SliceStable(all, func(i, j int) bool { return itemLess(facet, all[i], all[j]) })

	kept := all
	switch policy.Name {
	case "sample":
		if policy.N > 0 {
			var out []Item
			per := map[string]int{}
			for _, it := range all {
				if per[it.State] >= policy.N {
					continue
				}
				per[it.State]++
				out = append(out, it)
			}
			kept = out
		}
	case "top":
		if policy.N > 0 && len(all) > policy.N {
			kept = all[:policy.N]
		}
	default: // merge
		if len(all) > planItemCap {
			kept = all[:planItemCap]
		}
	}
	view.Truncated = len(kept) < len(all)
	if len(kept) > 0 {
		view.Items = kept
	}
	for _, it := range kept {
		if col := byState[it.State]; col != nil {
			col.Shown++
		}
	}
	return view
}

func newBuckets(names []string) map[string]*PlanBucket {
	m := make(map[string]*PlanBucket, len(names))
	for _, n := range names {
		m[n] = &PlanBucket{Name: n}
	}
	return m
}

func tallyBucket(m map[string]*PlanBucket, key string, size float64) {
	if key == "" {
		return
	}
	if b := m[key]; b != nil {
		b.Items++
		b.Size += size
	}
}

func bucketList(names []string, m map[string]*PlanBucket) []PlanBucket {
	if len(names) == 0 {
		return nil
	}
	out := make([]PlanBucket, 0, len(names))
	for _, n := range names {
		out = append(out, *m[n])
	}
	return out
}

// itemLess is the keep-order from contracts/aggregations.yaml: what a
// bounded board keeps, derived only from what the plan DECLARES.
//
//  1. terminal-state items last  (a done card is a record, not a plan)
//  2. blocked before unblocked   (the cards a reader must act on)
//  3. strongest commitment first, in declared order
//  4. soonest due first; no due date sorts last
//  5. earliest declared state, then origin scope, then id
//
// The last three keys make the order total, so two consumers bounding the
// same board drop exactly the same cards.
func itemLess(facet *PlanFacet, a, b Item) bool {
	if at, bt := facet.Terminal(a.State), facet.Terminal(b.State); at != bt {
		return bt
	}
	if a.Blocked() != b.Blocked() {
		return a.Blocked()
	}
	if ac, bc := commitmentRank(facet, a.Commitment), commitmentRank(facet, b.Commitment); ac != bc {
		return ac < bc
	}
	switch {
	case a.Due != nil && b.Due == nil:
		return true
	case a.Due == nil && b.Due != nil:
		return false
	case a.Due != nil && b.Due != nil && !a.Due.Equal(*b.Due):
		return a.Due.Before(*b.Due)
	}
	if ar, br := facet.StateRank(a.State), facet.StateRank(b.State); ar != br {
		return ar < br
	}
	if a.Scope != b.Scope {
		return a.Scope < b.Scope
	}
	return a.ID < b.ID
}

// commitmentRank is an item's index in the declared commitments, which are
// ordered strongest-first. An undeclared or absent commitment sorts after
// every declared one.
func commitmentRank(facet *PlanFacet, c string) int {
	for i, v := range facet.Commitments {
		if v == c {
			return i
		}
	}
	return len(facet.Commitments) + 1
}

// MergeItems merges item batches into one board-ordered slice, tagging
// nothing: callers stamp origin before merging, as AggregatePlan does.
func MergeItems(facet *PlanFacet, limit int, batches ...[]Item) []Item {
	var out []Item
	for _, b := range batches {
		out = append(out, b...)
	}
	sort.SliceStable(out, func(i, j int) bool { return itemLess(facet, out[i], out[j]) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}
