package reporting

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

const sparklineCap = 200

// KPIValue is one aggregated KPI in a SummaryView.
type KPIValue struct {
	Name      string   `json:"name"`
	Value     float64  `json:"value"`
	Unit      string   `json:"unit"`
	Policy    string   `json:"policy"`
	Target    *float64 `json:"target,omitempty"`
	Variance  *float64 `json:"variance,omitempty"`
	Direction string   `json:"direction,omitempty"`
}

// SummaryView is one definition's summary facet rolled up to one scope.
type SummaryView struct {
	Definition  string   `json:"definition"`
	Description string   `json:"description"`
	Categories  []string `json:"categories"`

	Stances  []string `json:"stances,omitempty"`
	Facets   []string `json:"facets"`
	Scope    string   `json:"scope"`
	Status   string   `json:"status"`
	Headline string   `json:"headline"`

	HeadlinePartial bool          `json:"headline_partial,omitempty"`
	KPIs            []KPIValue    `json:"kpis"`
	Sparkline       []SeriesPoint `json:"sparkline,omitempty"`
	Instances       int           `json:"instances"`
	Scopes          []string      `json:"scopes"`

	Contributors   []ContributorView `json:"contributors,omitempty"`
	LastProducedAt time.Time         `json:"last_produced_at"`

	Folded int `json:"folded,omitempty"`
}

// ContributorView is one instance behind an aggregate: where, who and when.
type ContributorView struct {
	Scope      string    `json:"scope"`
	Producer   string    `json:"producer,omitempty"`
	ProducedAt time.Time `json:"produced_at"`
}

// Contributors is the newest instance per scope path at or below scope, produced_at ascending.
func Contributors(def *Definition, scope string, instances []*Instance) []*Instance {
	return ContributorsIn(def, scope, instances, nil)
}

// ContributorsIn is Contributors read over a period: each path's instances in it folded into one.
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

	if def.Scope.Aggregation.TimeStatusPolicy().Name == "latest" {
		out.Status = instanceStatus(def, newest)
	} else {
		out.Status = worstStatus(def, group)
	}

	if anyPrecomputedHeadline(group) {
		n := 1
		if p := def.Scope.Aggregation.TimeHeadlinePolicy(); p.Name == "top" {
			n = p.N
		}
		out.Headline = newestHeadlines(def, group, n)
	}

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

// Aggregate rolls the instances up to one SummaryView at scope per the definition's aggregation spec.
func Aggregate(def *Definition, scope string, instances []*Instance) *SummaryView {
	return AggregateIn(def, scope, instances, nil)
}

// AggregateIn is Aggregate over ContributorsIn's set; a nil period is Aggregate exactly.
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

	if len(contrib) == 0 {
		return view
	}
	view.LastProducedAt = contrib[len(contrib)-1].ProducedAt

	view.Status = aggregateStatus(def, contrib)

	view.KPIs = aggregateKPIs(def, contrib)
	view.Headline, view.HeadlinePartial = aggregateHeadline(def, contrib, view.KPIs)
	view.Sparkline = aggregateSparkline(def, contrib)
	return view
}

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

func aggregateStatus(def *Definition, contrib []*Instance) string {
	policy := Policy{Name: defaultStatusPolicy}
	if p, err := ParsePolicy(def.Scope.Aggregation.Status); err == nil {
		policy = p
	}
	if policy.Name == "latest" {
		return instanceStatus(def, contrib[len(contrib)-1])
	}
	return worstStatus(def, contrib)
}

func worstStatus(def *Definition, instances []*Instance) string {
	worst := StatusOK
	for _, in := range instances {
		if s := instanceStatus(def, in); statusRank(s) > statusRank(worst) {
			worst = s
		}
	}
	return worst
}

func anyPrecomputedHeadline(instances []*Instance) bool {
	for _, in := range instances {
		if in.Headline != "" {
			return true
		}
	}
	return false
}

func newestHeadlines(def *Definition, instances []*Instance, n int) string {
	var lines []string
	for i := len(instances) - 1; i >= 0 && len(lines) < n; i-- {
		if h := instanceHeadline(def, instances[i]); h != "" {
			lines = append(lines, h)
		}
	}
	return strings.Join(lines, "; ")
}

func aggregateHeadline(def *Definition, contrib []*Instance, kpis []KPIValue) (headline string, partial bool) {
	policy, perr := ParsePolicy(def.Scope.Aggregation.Headline)
	wantsSynthesis := perr == nil && policy.Name == "synthesize"

	if wantsSynthesis && !anyPrecomputedHeadline(contrib) && len(contrib) > 1 {
		if s := def.Facets.Summary; s != nil && s.Headline != "" {
			return renderHeadline(s.Headline, templateKPIValues(def, s.Headline, contrib, kpis)), false
		}
	}

	n := 1
	if perr == nil && policy.Name == "top" {
		n = policy.N
	}
	return newestHeadlines(def, contrib, n), len(contrib) > n
}

func templateKPIValues(def *Definition, tmpl string, contrib []*Instance, kpis []KPIValue) map[string]float64 {
	vals := make(map[string]float64, len(kpis))
	for _, k := range kpis {
		vals[k.Name] = k.Value
	}
	for _, spec := range def.Data.KPIs {
		if _, ok := vals[spec.Name]; ok {
			continue
		}
		if !strings.Contains(tmpl, "{{"+spec.Name+"}}") {
			continue
		}
		policy, err := ParsePolicy(def.Scope.Aggregation.KPIs[spec.Name])
		if err != nil {
			policy = Policy{Name: defaultKPIPolicy}
		}
		var xs []float64
		for _, in := range contrib {
			if v, ok := in.KPIs[spec.Name]; ok {
				xs = append(xs, v)
			}
		}
		vals[spec.Name] = applyKPIPolicy(policy, xs)
	}
	return vals
}

func renderHeadline(tmpl string, vals map[string]float64) string {
	return headlinePlaceholderRE.ReplaceAllStringFunc(tmpl, func(m string) string {
		name := headlinePlaceholderRE.FindStringSubmatch(m)[1]
		if v, ok := vals[name]; ok {
			return FormatKPI(v)
		}
		return m
	})
}

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
			policyStr = defaultKPIPolicy
		}
		policy, err := ParsePolicy(policyStr)
		if err != nil {
			policy, policyStr = Policy{Name: defaultKPIPolicy}, defaultKPIPolicy
		}

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

		if len(targets) > 0 {
			target := applyKPIPolicy(policy, targets)
			variance := kv.Value - target
			kv.Target, kv.Variance, kv.Direction = &target, &variance, spec.Direction
		}
		out = append(out, kv)
	}
	return out
}

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
	default:
		return vals[len(vals)-1]
	}
}

func percentile(vals []float64, p float64) float64 {
	sorted := append([]float64(nil), vals...)
	sort.Float64s(sorted)
	rank := int(math.Ceil(p / 100 * float64(len(sorted))))
	if rank < 1 {
		rank = 1
	}
	return sorted[rank-1]
}

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

// MergeEvents merges event batches into one stream sorted by time ascending.
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

// FormatKPI renders a KPI value with at most two decimals and no trailing zeros.
func FormatKPI(v float64) string {
	s := strconv.FormatFloat(v, 'f', 2, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" || s == "-" || s == "-0" {
		return "0"
	}
	return s
}

const planItemCap = 500

// PlanColumn is one declared state rolled up.
type PlanColumn struct {
	State string  `json:"state"`
	Items int     `json:"items"`
	Size  float64 `json:"size,omitempty"`
	Shown int     `json:"shown"`

	Limit int `json:"limit,omitempty"`
}

// PlanBucket is one declared lane, horizon or commitment rolled up.
type PlanBucket struct {
	Name  string  `json:"name"`
	Items int     `json:"items"`
	Size  float64 `json:"size,omitempty"`
}

// PlanView is one definition's plan facet rolled up to one scope.
type PlanView struct {
	Definition string `json:"definition"`
	Scope      string `json:"scope"`
	Policy     string `json:"policy"`

	States      []PlanColumn `json:"states"`
	Lanes       []PlanBucket `json:"lanes,omitempty"`
	Horizons    []PlanBucket `json:"horizons,omitempty"`
	Commitments []PlanBucket `json:"commitments,omitempty"`

	Items []Item `json:"items"`

	Total     int  `json:"total"`
	Blocked   int  `json:"blocked"`
	Truncated bool `json:"truncated,omitempty"`

	SizeUnit string        `json:"size_unit,omitempty"`
	Baseline *PlanBaseline `json:"baseline,omitempty"`

	Instances      int       `json:"instances"`
	Scopes         []string  `json:"scopes"`
	LastProducedAt time.Time `json:"last_produced_at"`
}

// AggregatePlan rolls the instances up to one PlanView at scope, or nil without a plan facet.
func AggregatePlan(def *Definition, scope string, instances []*Instance) *PlanView {
	return AggregatePlanIn(def, scope, instances, nil)
}

// AggregatePlanIn is AggregatePlan over ContributorsIn's set: each path's newest board in the period.
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

	policy := Policy{Name: defaultItemsPolicy}
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

	all := originStampedItems(def, contrib)
	if len(contrib) > 0 {
		view.LastProducedAt = contrib[len(contrib)-1].ProducedAt
	}

	view.Total = len(all)
	byState := map[string]*PlanColumn{}
	for i := range facet.States {
		st := facet.States[i]

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

	sort.SliceStable(all, func(i, j int) bool { return itemLess(facet, all[i], all[j]) })

	kept := boundItems(policy, all)
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

func originStampedItems(def *Definition, contrib []*Instance) []Item {
	var all []Item
	for _, in := range contrib {
		for _, it := range in.Items {
			it.Scope = in.Scope
			it.Definition = def.Name
			all = append(all, it)
		}
	}
	return all
}

func boundItems(policy Policy, ordered []Item) []Item {
	switch policy.Name {
	case "sample":
		if policy.N <= 0 {
			return ordered
		}
		var out []Item
		per := map[string]int{}
		for _, it := range ordered {
			if per[it.State] >= policy.N {
				continue
			}
			per[it.State]++
			out = append(out, it)
		}
		return out
	case "top":
		if policy.N > 0 && len(ordered) > policy.N {
			return ordered[:policy.N]
		}
		return ordered
	default:
		if len(ordered) > planItemCap {
			return ordered[:planItemCap]
		}
		return ordered
	}
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

func commitmentRank(facet *PlanFacet, c string) int {
	for i, v := range facet.Commitments {
		if v == c {
			return i
		}
	}
	return len(facet.Commitments) + 1
}

// MergeItems merges already origin-stamped item batches into one board-ordered slice, bounded by limit.
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
