# Reporting — design notes

Covers `pkg/reporting`, the YAML under `reporting/`, `pkg/scheduling` and `pkg/updates`. The model and the aggregation semantics are in `docs/reporting.md` and `docs/vision/reporting.md`; these are the smaller decisions the code makes.

## Aggregation

- `foldOverTime` sets `Status` explicitly on the synthetic instance, because a status rule applied later to a summed KPI would judge a week's total against a threshold written for a day.
- `aggregateHeadline` renders the summary template against rolled-up KPIs only for `synthesize`; `latest` and `top(n)` are honoured as requests for contributors' own sentences.
- `SummaryView.HeadlinePartial` marks one contributor's sentence standing in for several, so a client can label the degradation instead of passing it off as the whole picture.
- `templateKPIValues` aggregates on demand any KPI a headline placeholder names outside the summary shortlist, so a template is never rendered half from the roll-up and half from nowhere.
- `Contributors` is exported so the summary tile, detail panels and plan roll-up resolve the same instance set; a panel over a different set would show superseded data.
- `KPIValue.Target` and `Variance` are pointers present only when a target exists, so a consumer can tell "no target" from "target of zero".
- `PlanColumn.Limit` is the declared WIP limit times the contributor count, so a realm's allowance is the sum of its teams'; `0` means undeclared, not zero.
- `PlanBucket` puts an item with no value for an axis in no bucket; an "(other)" bucket would add a value the definition never declared.
- `PlanView` carries no overdue count because the package has no clock; a client derives it from `Item.Due`.
- `itemLess` makes the keep-order total (state rank, origin scope, id last), so two consumers bounding the same board drop the same cards.
- `planItemCap` backstops an unbounded `merge`; without it every consumer would invent its own cap.
- `defaultStatusPolicy`, `defaultHeadlinePolicy`, `defaultKPIPolicy` and `defaultItemsPolicy` in policy.go are the single source of each policy default.

## Definitions and loading

- `ParseDefinition` skips reference validation for a definition with `extends`; `LoadLibraryDir` resolves the chain first, then runs `validateRefs`, because refs may point at inherited data.
- `mergeExtends` starts a child from a deep copy of its parent; any non-zero child field or facet slot overrides wholesale, never per element.
- `facetOrder` appends `plan` last rather than beside `timeline`, so the prefix existing consumers read stays byte-identical.
- `ServesStance` treats absent `Stances` as "every stance", the opposite of categories, because a stance is a claim about judgement and its absence is "no opinion".
- `SectionStances` derives a panel's stances from its section so the stance axis works library-wide; `PanelSpec.Stance` overrides only for the rare panel such as a strategic bet in `exceptions`.
- `PanelSections`: a panel naming no section renders in declaration order ahead of the sectioned ones.
- `PlanFacet.Commitments` are declared strongest-first and `Horizons` nearest-first; as with `States`, the declared order is what a bounded board drops cards by, so duplicates are rejected.
- `PlanBaseline.Ref` points into the data contract (`kpi/<name>` or `series/<name>`) rather than carrying a value, because a baseline is per scope.
- `KPISpec.Target` requires `Direction`, because a target nobody can be on the wrong side of says nothing.
- `NewRegistry` lets a later duplicate name win; only hand-built slices can contain duplicates, since `LoadLibraryDir` rejects them.

## Instances and periods

- `Item` carries no status and no created-at: blocked-ness and `Due` are the only exception signals, and history is the timeline facet's job.
- `Item.Due` is a pointer because committed-but-unscheduled work is the common case, and a zero time would render as year 1.
- `Item.BlockedBy` edges may not resolve after a roll-up (blocker in a sibling scope, or bounded away); that is a fact about the plan the reader never rewrites.
- `Instance.Targets` overrides `KPISpec.Target` per scope because a target is a local commitment; keys validate against the same data contract as KPIs.
- `validateItems` rejects any state the facet did not declare, so a rolled-up board's columns are exhaustive.
- `PeriodAt` treats an unknown kind as `day`; callers validate with `ParsePeriodKind` first (`TestPeriodAtUnknownKindIsDay` pins it), and `Period.Previous` reuses it so calendar arithmetic lives in one place.

## Contracts (`reporting/contracts`)

- Rationale lives in `description:`, `note:` and `guidance` keys, which the loader carries and a reader of the parsed YAML still sees; `#` lines are terse value hints for definition authors only.
- `stances.yaml`: a definition declares which stances it can serve and the reader picks one; declaring none means readable in every stance, since a raw feed has no footing.
- `stances.yaml` names which `panel_sections` serve each stance because the facets reading order was already a stance projection.
- `primitives.yaml` has no dependency primitive: `blocked_by` edges are a graph and `dag` already answers that question; burndown renders a series and sits with the temporal family.
- The `guidance` blocks in `primitives.yaml` and `stances.yaml` are house style, not validated: a definition cannot be rejected for choosing badly.
- `categories.yaml`: a report using a non-standard category is invisible to the standard filters; that trade-off is the point of a standard.
- `media.yaml`: a deployment with no generation model has no media facet; the contract makes the absence explicit rather than silent.
- `aggregations.yaml` `scope_levels` are advisory names for depth 1..4; the code operates on depth only.
- Waterfall tables carry a `kind` column, `total` at the two ends and `delta` between; no primitive schema declares it, so each library file keeps the one-line hint.

## Library (`reporting/library`)

- Every plan-bearing definition spells its states from one flow vocabulary (`proposed`, `triaged`, `committed`, `in_progress`, `review`, `done`), so boards aggregate across definitions at a wide scope.
- `todo.yaml` is the degenerate plan (two states, nothing else) kept as the facet's acceptance test: a `checklist` must render it with no special case.
- Stopped or cancelled work is never a column (`portfolio`, `roadmap`): it has left the plan and the timeline event says so, which keeps `done` terminal.
- `workflow.yaml` publishes steps as plan items with `depends_on` edges so the `dag` primitive has data to draw; the table stays as the searchable record.
- `task-board.yaml` WIP limits sit where a squad actually sits, and `proposed`/`done` are unbounded: an intake limit is a filing rule and a done limit is nothing.
- `brand-reach.yaml` covers press office and internal comms with one definition because both publish, carry and react; `extends` is the seam to split on.
- `security-posture.yaml` holds the findings/patching half and the detections half together so one page answers "are we exposed?" and "did anyone try?"; patch SLAs are 30/60/90 days by severity.
- `data-pipeline.yaml` reports freshness, SLA and data quality only; latency and error rate already come from `performance` and `incident`.
- `campaign.yaml`, `cost.yaml` and `performance.yaml` carry targets per instance, not in the definition, because brand and performance spend (or teams of different size) differ too much for one number; `budget_used_pct` drives cost status for the same reason.
- `company-scorecard.yaml` is the only definition produced at the universe, and rolls audience KPIs up with `max`: one user on music and podcasts is one MAU, so worlds overlap.
- `max` is also the honest roll-up for `licensing` territories cleared, `data-pipeline` lateness, `backlog` oldest age and `financial-close` close day: one four-hour-stale table is a four-hour problem an average hides.
- `portfolio.yaml` `at_risk` sums rather than takes a max because the VP's question is "how many of my bets are in trouble".
- Durations (`task-board` cycle time, `workflow` run time, `legal-matters` turnaround, `decisions` wait) roll up as `p50`, never `sum`.
- Status reads the health signal, not the volume: `brand-reach` on sentiment, `subscriber-growth` on churn, `sales-pipeline` on coverage, `catalog` on metadata quality.

## Scheduling (`pkg/scheduling`)

- `Schedule.ID` becomes the `CausationID` of every message it sends, so a recipient can tell which schedule a message came from.
- `Schedule.To` defaults to `Owner` because a worker scheduling a message to itself is the ordinary case; `Filter.Worker` matches schedules the worker owns or receives.
- `Store.Upsert` replaces every field except `CreatedAt` and `LastAt`, which record history the caller does not own.
- `ErrInvalid` wraps every failure from `Validate` and `ParseCron`, so a caller maps the family to one status code.
- `ParseCron` returns the next match strictly after its argument, normalised to UTC, and the zero time for an expression that never matches.
- `Schedule.Validate` checks shape only (IDs without `/` or whitespace, so they are safe in a URL path); the control plane owns whether `Owner` and `To` exist.

## Updates (`pkg/updates`)

- `SubjectUpdateAvailable` is the node's cheap pre-filter: a node that ignores updates skips the body parse on subject alone.
- The contract is plain JSON on a `messaging.TypeStatusUpdate` envelope; `ParseAnnouncement` is a Go convenience and a bash node with jq is a first-class consumer.
- `Announcement.ID` is server-assigned and separate from `Version` because the same version can be re-announced and each rollout must be counted separately.
- `Announcement.Notes` is the escape hatch that keeps the schema from growing a field per deployment's drain-and-restart procedure.
- `ValidateKind` requires lowercase rather than recommending it, so `Acme/Pack` and `acme/pack` cannot appear as two kinds in one rollout table.
- `Pending` is a catch-up convenience, not an authority (the announcement already sits in the durable inbox); only `StatusApplied` and `StatusDeclined` count as resolved.
- `RolloutNode.WorkerStatus` lets an operator tell "not answering because down" from "not answering because ignoring me".
- `TestAnnouncementLiteralWireFormat` asserts literal JSON field by field, because a round-trip would agree with itself and miss a struct-tag typo.
