# Perspective: Reporting (technical report capability contracts)

> **Reports are books with a standard binding.** A report instance is
> ordinary spine traffic — deposited in the library ([[core.md]]),
> announced by a pointer message. What this layer adds is the *binding*:
> a small set of capability contracts a report declares, so that a
> hundred different reports from a hundred different nodes can be
> aggregated into one dashboard, one timeline, one pulse — without any
> consumer knowing what any individual report is about. The contract
> definitions and the report registry are fixed control-plane surface
> ([[control-plane.md]]); report *types* are YAML documents anyone can
> extend, shipped as a template library (`reporting/`).

A management or orchestration role — human or agent — does not read a
hundred reports. It looks at one surface and drills down. That only
works if every report answers the same small set of questions in the
same machine-readable way: *how are you doing* (a summary tile), *when
did things happen* (a timeline), *what is happening right now* (a
pulse), *show me everything* (a detail page), *and say it in a form I
can consume where I am* (a modality). The reporting layer is those
questions, made into contracts.

## Two entities, one distinction

- **ReportDefinition** — a YAML document declaring a report *type*: which
  facets it exposes, which categories it covers, what its data contract
  contains, how it aggregates across scopes. Definitions live in the
  control plane's report registry and ship as extendable templates in
  `reporting/library/`.
- **Report instance** — a data payload conforming to a definition's data
  contract, produced by a node at a moment, at a scope. Instances are
  books: they live in the library, referenced by pointer. The registry
  indexes them; it does not store them.

The consequence: dashboards, timelines, and media generators are written
once, against definitions — never against individual reports.

## The facets

A definition exposes any subset of seven **facets** — the capability
contracts. Formal shapes in `reporting/contracts/facets.yaml`.

1. **`summary`** — how the report appears *inside a larger dashboard*:
   name, categories, a status light, a handful of KPIs (value, delta,
   trend sparkline), one headline sentence. The minimal legal summary is
   a named box you can click into; everything richer is opt-in. This is
   the contract that makes the 100-reports-one-dashboard aggregation
   possible.
2. **`detail`** — the report's *own* dashboard: panels binding named
   data to visual primitives, plus **drilldown** — typed links to
   sub-reports, so detail pages chain into hierarchies of reports.
3. **`timeline`** — the report's temporal contract: events, milestones,
   and spans (start/end bounds), each timestamped, severity-tagged, and
   filterable. In the simplest form this is one event — the report's own
   creation. Consumers can merge any number of timelines because the
   event shape is shared.
4. **`pulse`** — current activity: a live, rate-bounded stream of what
   is happening *right now*. Designed to surface at every dashboard
   level — a site's pulse is its nodes' activity; a universe's pulse is
   the merged, sampled stream of everything below.
5. **`media`** — synthesized media generation: script archetypes (news
   anchor, co-hosted podcast, screencast walkthrough, incident recap),
   pacing/tone bounds, and a storyboard contract binding data widgets to
   script timestamps. See `reporting/contracts/media.yaml`.
6. **`conversation`** — on-demand Q&A: which questions this report can
   ground, and which data it exposes as context, so an agent (or a
   virtual-meeting persona panel) can answer "where are today's
   bottlenecks?" with the report as ground truth.
7. **`plan`** — what the producer intends to do: an ordered set of flow
   **states**, optional lanes, horizons, commitment grades and
   work-in-progress limits, and the **items** on it. The declared order
   of the states *is* the semantics — left to right is progress, the
   last state is terminal — which is what lets a realm roll six teams'
   boards into one without knowing what "in review" means at any of
   them. The degenerate plan is a checklist: two states and no flow.

### Plans are reported, never authored

The first six facets are retrospective or present-tense. `plan` is the
only one that says what is *intended*, and it exists because the
`strategic` stance was defined as "measured against a plan" while
nothing modelled the plan: the comparison it names was answered by a
scalar target and by `roadmap`'s `upcoming` table, which was the only
future-dated data in the build and was opaque cells no renderer
understood.

Adding it does **not** reopen [[hierarchy.md]]'s resolution that the Go
build models no work hierarchy. Five guards keep that line, and they are
structural rather than a promise:

- **No item store.** Items ride inside the instance payload the control
  store already persists opaquely. No new table, column, index or
  migration.
- **No mutation API.** `POST /api/reports/instances` remains the only
  entry point, and it is whole-instance replace. A producer that changes
  its plan republishes it; nothing moves a card.
- **No identity across instances.** An `id` exists so `blocked_by` has
  something to point at inside the same payload. Nothing correlates one
  between two instances, and no transition is derived from a card's
  appearing or disappearing. Keeping ids stable between publications is
  the *producer's* business.
- **No parent link.** That field is where a work hierarchy would return.
  `level` is a flat label from hierarchy.md's vocabulary — calling an
  item a "goal" says how to read it; it does not create a goal, nor
  anything the goal contains.
- **No cross-definition board.** There is no endpoint merging two
  definitions' plans, because deciding whether one's `in review` is the
  other's `verifying` is report-specific knowledge, which the scope
  contract forbids.

A deployment running Jira or Linear has a node that emits a `task-board`
instance, exactly as its finance node emits a `budget-variance`. Robot
Dreams reports the plan; it does not keep it.

Facets are **data-first**: each contract defines the data shape;
presentation is a *hint*, chosen from a shared vocabulary. A dashboard
may re-render a report's KPI series as a sparkline, a gauge, or a plain
number — the contract survives, because the contract is the data.

## Categories

Standard concepts for filtering and scoping, shared across every
surface (`reporting/contracts/categories.yaml`):

`logs` · `performance` · `roadmap` · `delivery` · `failures` ·
`activity` · `decisions` · `cost` · `quality`

The last three were added once the fleet model was real: `decisions`
(escalations, approvals, work blocked on a human — "what needs me?",
which is not a failure), `cost` (tokens, compute, spend — a different
owner from `performance`, and it sums rather than averages), and
`quality` (review outcomes, acceptance, rework — neither "it shipped"
nor "it broke").

A definition declares which categories it covers; every dashboard,
timeline, and pulse view filters on the same list. The set is
extendable per deployment — but a report that invents a category
forfeits being found by the standard filters, which is the point of
having standards.

## Visual primitives and modalities

Two shared vocabularies, versioned as contracts rather than baked into
any renderer:

- **Visual primitives** (`reporting/contracts/primitives.yaml`) — the
  render widgets a `detail` panel or `summary` tile may hint at, in five
  families: *temporal* (gantt, burndown, timeseries, event ticker),
  *topological* (DAG, heatmap/treemap, topology map), *aggregate* (KPI
  card, status matrix, gauge), *planning* (board, roadmap lanes,
  checklist), *diagnostic* (flame graph, waterfall, data grid, log
  buffer, diff viewer). The DAG is the only primitive naming two data
  kinds, and the reason is worth recording: `graph` never had a producer
  at all, which is why `workflow.yaml` carried an apology for rendering
  `depends_on` as a table column. A plan item's `blocked_by` edges are
  that graph.
- **Consumption modalities** (`reporting/contracts/modalities.yaml`) —
  the human-facing delivery modes a facet can serve: *narrative* (TL;DR
  banners, plain-language briefings), *glance* (traffic-light badges,
  ambient notifications), *delta* (diff-first views, trend arrows),
  *spatial* (dependency webs, drill-down trees), *storyboard* (replay
  scrubbing, linear event feeds), *board* (committed work in flow order,
  scanned for pile-up and blockage rather than read for values),
  *conversational* (Q&A, audio debriefs).

A definition declares which modalities each facet supports. A consumer
picks the modality that fits its context; the report doesn't get a
vote about where it is read.

## The scope contract

Every capability must know how it behaves across the organizational
hierarchy ([[organization.md]]): Universe → World → Realm → Site. Two
pieces:

- **Scope path.** Every report instance carries the path it attaches
  to — `acme/streaming/platform-tribe/deploy-squad` — with as much or
  as little depth as the deployment uses. Consistent with
  [[organization.md]], levels are a *reading*, not containers the code
  enforces: the path is a label on the instance, and a lone node
  publishing at a bare root is a complete, valid deployment.
- **Aggregation policy, per facet, per field.** The definition declares
  how N instances below a scope become one view at that scope: status
  rolls up `worst`; a KPI declares `sum`, `avg`, `max`, `p50`/`p95`,
  `latest`, or `count`; timelines and pulses `merge` (with `sample` and
  `top(n)` bounds so a universe view doesn't drown); headlines
  `synthesize` (regenerate a summary over summaries) or `top(n)`.
  Policies are named in `reporting/contracts/aggregations.yaml`. Plans
  `merge`, `sample(n)` per column, `top(n)`, or collapse to `count` —
  and whichever is chosen, the column totals stay exact: bounding the
  cards must never move a total, or a rolled-up board becomes a lie
  about how much work there is.
- **Time is the same contract, one stage earlier.** A reader who asks
  for a week rather than now gets each scope path's instances in that
  week folded into one by the definition's `time` policies — counters
  `sum`, gauges stay `latest`, status `worst` — and only then the
  scope roll-up above. "Now" is the degenerate case: no period, newest
  per path. Nothing about a week is report-specific either; it is one
  more declared policy, and the same test applies.

The test of the contract: viewing any scope must require only the
declared policies and the instances beneath it — never knowledge of
what any report means. If a dashboard needs report-specific code to
roll something up, the scope contract has leaked.

## Mission Control: the Outcomes section

Mission Control ([[control-plane.md]]) gains an Outcomes section built
entirely against the contracts:

- **Scope bar** — breadcrumb navigation across Universe → World → Realm
  → Site; every view below it is aggregated to the selected scope.
- **Aggregate dashboard** — the summary tiles of every definition
  active at that scope, filterable by category and by facet.
- **Report page** — one definition at one scope: its detail panels,
  its timeline, its pulse, its drilldowns into sub-reports.
- **Pulse rail** — the merged live stream at the current scope, always
  visible, always one level of the same contract.

Nothing in the section knows any report by name. That is the
acceptance test for the whole layer.

## Testing: the simulated company

The layer is testable only with a believable population of reports, so
the reference build includes a simulation fixture: a fictionalized
real-world org (a fictional streaming company — Universe: the company;
Worlds: divisions; Realms: departments; Sites: teams) populated by a
**mock LLM** — a deterministic, seeded generator that emits plausible
report instances with template-generated narrative text, no network,
no real model.

The fixture is deliberately mostly *not* engineering, because real
companies are not: ten of its thirteen divisions are business
functions, and each team emits only what its own work produces — a
month-end close and a budget bridge from Finance, a queue and a CSAT
from Support, matter turnaround from Legal, deploys and latency only
from the teams that actually run software. That constraint is the
point of the fixture rather than a detail of it. A reporting layer
that only ever sees engineering telemetry is not being tested; it is
being flattered. `dream dashboard --simulate` stands the whole thing up for
review.

## The scope test, applied

Same two-tier test as [[core.md]] and [[environment.md]]. Report
instances are ordinary library content — the spine already carries
them; nothing new is claimed there. The contract vocabulary, the
registry, and the aggregation rules are control-plane surface: they are
exactly the universal, backend-agnostic agreements that must hold for
*observing* a deployment to scale past a handful of nodes — the same
justification as the org chart. What a report is *about*, and what
produces it, stays out entirely: any node, any workload, any tool can
emit a conforming instance. If defining a new report type ever requires
touching the dashboard, a renderer, or Robot Dreams itself, the
abstraction has leaked.

## Open questions

- **Registry vs. library boundary.** Definitions are control-plane
  state (like the org chart); instances are library content. Where do
  *indexes* over instances (by scope, by time, by category) live —
  control plane, or a storage-backend capability with a fallback scan?
- **Pulse transport.** Live streams over the messaging primitive
  directly, or a control-plane fan-in that tails messaging and serves
  consumers? The former is purer; the latter is what keeps 100
  dashboards from becoming 100 messaging subscribers.
- **Synthesis is a capability, not a given.** `synthesize` aggregation
  and the `media`/`conversation` facets need a model behind them.
  Deployments without one must degrade to `top(n)`/templates — the
  contracts must make that degradation explicit, not silent.
- **Retention.** Timelines and pulses imply history; the storage
  primitive's durable-vs-disposable line ([[memory.md]]) needs a stated
  mapping for report instances (pulse: disposable; instances:
  durable; synthesized media: regenerable cache).
- **Item identity is deliberately absent.** The first deployment that
  needs to follow one card across two instances — to derive a burndown
  by differencing, say, rather than having the producer publish one — is
  the deployment asking for option 2 in [[hierarchy.md]]. That is a
  scope decision about what the product owns, not a field.

See also: [[core.md]] for the library and the scope test,
[[control-plane.md]] for Mission Control, [[organization.md]] for the
hierarchy scope paths are read against, [[environment.md]] for the
sibling pattern of contracts + template library.
