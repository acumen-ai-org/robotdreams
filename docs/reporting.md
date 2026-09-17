# Reporting

Implementation-level guide to the reporting subsystem: the report
registry, the HTTP API, scope aggregation, and the simulation fixture.
Design rationale lives in `docs/vision/reporting.md`; the YAML
contracts and template library live in `reporting/`.

## Pieces

- **`pkg/reporting`** — contracts library: parses and validates
  `ReportDefinition` YAML, models report instances, aggregates
  instances across scopes. Pure library, no server dependencies.
- **Report registry** (in the control-plane server) — definitions
  loaded at startup from a library directory:
  `dream server --reports reporting/library/`. Loading follows the
  same pattern as `--orgchart`: exhaustive validation, all problems
  reported at once.
- **Instance store** — submitted instances and pulse events persisted
  in the server's control store (SQLite), pruned to a bounded window
  per definition + scope. Large artifacts belong in the storage
  primitive, referenced by pointer — the instance payload is the
  book's catalog card plus its summary data, not a dumping ground.
- **Mission Control → Outcomes section** — scope breadcrumb,
  category filters, aggregate tiles, per-report drill-down, live
  pulse rail. See `docs/dashboard.md`.
- **`dream simulate`** — boots an in-process server populated by a
  simulated company and a mock LLM, for demos and review.

## Scope paths

Every instance carries a slash-separated scope path, e.g.

```
spookify/music/playback/player-squad
└universe┘└world┘└realm─┘└───site────┘
```

Levels are advisory names for depth 1–4 (`universe`, `world`,
`realm`, `site`); the code operates on depth and prefix only. A view
at scope S aggregates all instances whose path is S or beneath S. The
empty scope is the root and sees everything.

## HTTP API

All routes require a bearer token (same auth model as the rest of the
API). Reads are available to any authenticated token; writes to any
connected worker.

| Route | Purpose |
| --- | --- |
| `GET /api/reports/definitions` | Registered definitions: `{"definitions": [Definition…]}` (full definition JSON, snake_case). |
| `GET /api/reports/scopes` | Distinct scope paths with instance counts: `{"scopes": [{"path": "...", "depth": 2, "level": "world", "instances": 5}…]}`, sorted by path. An optional `?since=<RFC3339>` adds `"recent"` to each entry — the instances produced after that instant, which is what a reader means by "reports this week"; `instances` is the whole retained history and only ever grows. Omitted entirely when no `since` is given, so a client can tell "none lately" from "not asked". Note that retention prunes per definition and scope, so a long window can be truncated. |
| `GET /api/reports/summary?scope=S&category=C&period=P&at=T` | Aggregate dashboard at scope S: `{"scope": S, "tiles": [SummaryView…]}` — one tile per definition with ≥1 contributing instance, optionally filtered to a category. Tiles sorted by definition name. Each tile carries `contributors`: one entry per instance behind the aggregate — `{scope, producer, produced_at}`, in produced_at order — so a reader of a rolled-up number can see which node actually wrote it. `producer` is the authenticated worker recorded at ingest, and is omitted for an instance submitted without one. The older `scopes` field is the same information minus the producer, name-sorted, and is unchanged. |
| `GET /api/reports/report?definition=D&scope=S&period=P&at=T` | One report at one scope: `{"definition": {...}, "summary": SummaryView, "timeline": {"events": [Event…], "spans": [{"name","start","end","label","scope"}…]}, "plan": PlanView\|null, "panels": [Panel…], "drilldowns": ["incident"…]}`. |
| `GET /api/reports/timeline?scope=S&category=C&since=RFC3339&period=P&at=T` | Merged, sampled events across definitions at scope S. `{"events": [Event…]}`. |
| `POST /api/reports/instances` | Submit an instance (JSON body = `pkg/reporting.Instance`). Validated against the definition's data contract; 400 with the full problem list otherwise. |
| `POST /api/reports/events` | Submit live pulse events without a full instance: `{"definition": D, "scope": S, "events": [Event…]}`. |

### Reading over a period

`summary`, `report` and `timeline` take the same two optional
parameters:

- `period=day|week|month|quarter|year` — read the view over that
  calendar period instead of "now". A week is an ISO week (Monday to
  Monday); a quarter starts in January, April, July or October.
- `at=<RFC3339>` — an instant inside the period; default now. The
  period is bounded at calendar boundaries in the **UTC offset `at`
  carries**: `at=2026-09-08T09:00:00+02:00&period=day` is the day that
  starts at midnight +02:00, a bare `Z` or no `at` at all means UTC.
  An RFC 3339 timestamp can only express a fixed offset, so a period
  that straddles a daylight-saving change is bounded in the offset the
  client sent, not the one it would have at the other end.

With a period the response gains one field, `"period": {"kind",
"start", "end"}` — the half-open interval `[start, end)` that was
actually read — and each summary tile gains `"folded"`, the number of
stored instances the time stage folded into its contributors. The
timeline is bounded to the period; an explicit `since` inside it still
wins, one before it does not widen it. An unknown `period` or a
malformed `at` is a 400. **Without `period` the responses are
byte-identical to what they were**: no echo, no `folded`, newest
instance per scope path.

`Panel` in the report response is the definition's panel binding plus
resolved data:
`{"title", "primitive", "data_kind": "series|table|events|spans|kpi|items",
"series": [SeriesPoint…] | "table": Table | "events": [Event…] |
"spans": […] | "kpi": KPIValue | "plan": PlanView}` — aggregated to the
requested scope (series merged, tables concatenated with a scope column
appended, events merged and sampled).

An `items` panel carries the whole `PlanView`, not just its cards: a
renderer is handed a panel and nothing else, and without the declared
column order it cannot draw a board at all. The same value appears once
at the top level, so a surface reading a whole plan does not have to go
looking through the panels for it.

`PlanView` is one definition's plan facet rolled up to one scope:
`{"definition", "scope", "policy", "states": [{state, items, shown,
size, limit}…], "lanes"/"horizons"/"commitments": [{name, items,
size}…], "items": [Item…], "total", "blocked", "truncated",
"size_unit", "baseline", "instances", "scopes"}`. Every declared state
appears whether or not it holds anything — an empty column is a fact.

### Live updates (SSE)

`GET /api/events` (admin token) gains two event types, both
backward-compatible with older dashboards:

- `report_instance` — `{"definition", "scope", "produced_at"}` on each
  accepted instance.
- `report_event` — one per accepted pulse event:
  `{"definition", "scope", "t", "type", "severity", "label"}`.

## Aggregation semantics

Two stages, **time within a scope path, then scope prefix** — and
"now" is the degenerate case of the first.

**Time.** Per definition and exact scope path, the instances whose
`produced_at` falls in the requested period (`?period=`, above) fold
into one synthetic instance by the definition's
`scope.aggregation.time` policies: `status` `worst` by default (a
week with one critical day was a critical week) or `latest`;
`headline` `latest` by default, `top(n)` joins the n most recent,
`synthesize` degrades to latest — prose cannot be recomputed, only
chosen between, and where no instance in the period carries any the
summary template is rendered against the folded numbers so the
sentence agrees with the KPI row beside it; `kpis` per KPI, `latest`
by default, `sum` for counters (`activity` and `cost` declare it for
theirs; a gauge like `active_nodes` stays `latest` — adding Monday's
node count to Tuesday's is not a number anyone asked for). A target
folds with its metric's policy. Series and events merge in time
order; tables and plan items are snapshots and come from the newest
instance. The synthetic instance carries the newest instance's
producer and `produced_at`. Without a period the stage is simply "the
newest instance per exact scope path wins" — superseded instances
drop out of view (history stays queryable via the timeline) — which
is exactly what it was before periods existed.

**Scope.** Roll-up across the contributing scope paths follows the
definition's `scope.aggregation` policies (see
`reporting/contracts/aggregations.yaml`), applied to the time stage's
output unchanged. Two deliberate degradations:

- `synthesize` (headline) degrades to `top(1)` — the reference server
  has no model behind it; degradation is explicit in the contract.
- Pulse/timeline sampling is severity-weighted: critical events are
  the last to be dropped.

Plans roll up per `scope.aggregation.items`, one of `merge` (every card,
capped), `sample(n)` (n per declared column), `top(n)` (n overall) or
`count` (columns only, no cards — the honest universe-level board).
Whichever applies, **`total`, `blocked` and every column count are
computed before any bound**: bounding the cards must never move a total.
What survives a bound is chosen by the keep-order in
`reporting/contracts/aggregations.yaml` — finished work goes first,
blocked work last — and every card is stamped with its origin scope, so
two teams publishing the same item id stay distinguishable.

There is deliberately **no** cross-definition plan endpoint to sit
alongside the cross-definition timeline. Merging two definitions' boards
would mean deciding whether one's `in review` is the other's
`verifying`, which is report-specific knowledge the scope contract
forbids. A client wanting several plans at one scope fetches several
reports and stacks them, each in its own vocabulary.

## Retention

The instance store keeps the most recent **50 instances** and **500
events** per (definition, scope), pruned on insert. Plan items need no
retention rule of their own: they ride inside the instance payload and
are pruned with it, which is the whole reason putting them there cost no
schema change. Pulse is
disposable signal; instances are the durable record within the
window; anything that must live longer belongs in the library (the
storage primitive) with a pointer.

Retention is count-based, and that bounds how far back a period is
honest: a scope path posting an `activity` instance per run keeps its
last 50 runs, so a `period=month` there sums what survived, not what
happened. The response says how many instances it folded (`folded`),
which is the reader's only signal that a window may have been cut
short. Raising the window, or keying it by age rather than count, is
a separate decision and is left open here.

## Simulation

```
dream simulate [--addr 127.0.0.1:0] [--seed 1] [--speed 1.0] [--ui-dev]
```

`--ui-dev` additionally starts the Vite dev UI wired to the simulation
(auto-connected, hot reload) — the dashboard-development loop in
[dashboard.md](dashboard.md).

Boots a throwaway server (temp data dir, embedded backends), loads
`reporting/library/`, mounts the dashboard, and populates a
fictional streaming company — universe `spookify`, whose 13
**divisions** (`music`, `podcasts`, `audiobooks`, `advertising`,
`platform`, `content`, `markets`, `finance`, `people`, `legal`,
`marketing`, `user-support`, `corporate`) sit at scope depth 2, their
**departments** at depth 3 and their **teams** at depth 4. (Those are
the fixture's own words for the levels; the scope contract's advisory
names for the same depths are world/realm/site — see
`pkg/reporting/scope.go`.) Simulated workers connect into the org chart,
and a **mock LLM** (deterministic, seeded, template-based text; no
network) generates 48h of plausible history plus a live stream of
instances and pulse events. The command prints the dashboard URL and
runs until interrupted. Same seed, same history.

Ten of those thirteen divisions are business functions, and **each
team produces only the reports its work actually generates**. That
routing lives in `internal/simulation/company.go`: every team declares
an `Archetype`, and the archetype maps to a report set. Finance
produces `financial-close` and `budget-variance`; Support produces
`support-queue`; Legal produces `legal-matters` and `compliance`;
`performance`, `logs`, `delivery` and `incident` are produced only by
the teams that run software. Teams also carry a `Region`, which puts
their reports on local working hours — corporate functions collapse at
the weekend while streaming traffic rises — and a `Headcount`, which
scales what they consume and report.

The five planning definitions (`plan`, `task-board`, `strategy`,
`backlog`, `todo`) are the fixture's hardest case, because a plan must
*persist*: a board whose cards are redrawn each tick cannot age, pile up
or slip, and reads as a shuffle rather than as work. So their items come
from a per-plan random source keyed to (seed, definition, scope) rather
than from the generator's shared sequence — only the item's STATE is a
function of time. Work arrives, moves at its own pace, sometimes sticks
in review, and ages off the board once finished. `roadmap`, `portfolio`
and `workflow` build their items first and project their existing tables
out of them, so a board and a table at the same scope cannot disagree.

Cadence is per definition, not global: activity every 30 minutes,
performance and logs hourly, cost every four hours, campaigns and
support daily, hiring and security weekly, the financial close and the
company scorecard monthly. A report whose cadence is longer than the
simulated window still gets a most-recent instance, so nothing shows
as an empty tile.
