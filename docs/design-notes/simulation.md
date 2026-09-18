# Simulation — design notes

## Generator and history

- Each `generate_*.go` registers its builders from its own `init()` via `registerTeam`/`registerScope`, so a department family adds reports without a shared edit point; `TestBuildersMatchLibrary` catches a definition with no builder.
- `lifecycleDefinitions` (incident) run a state machine instead of a cadence, so they have no builder or cadence entry and the consistency test consults this set rather than flagging a gap.
- `NewGenerator` forces two service teams' final logs instance to warn/critical (`warnTeam`, `critTeam`) and `makeIncidents` keeps one incident open on a third team, so boot shows a mixed status picture whatever the dice roll.
- `ticks` walks daily-or-slower cadences backwards from now, snaps team-level ticks to the previous local working hour, and collapses ticks landing on one hour, because latest-wins would silently drop one.
- `season` runs two curves: office archetypes follow local business hours and collapse at weekends; `consumerFacing` archetypes run around the clock, peak at `consumerPeakHour` and rise at weekends.
- `bridge` derives the opening total from the movements, so a waterfall always reconciles with its two ends.
- `byUnit` gives every team-level instance a one-row ranking table, so a roll-up concatenates rows into a ranked chart with no aggregation rule.

## Company shape and universe registry

- `universalReports` includes `workforce` because People Operations owns the definition, not the population; if only HR reported it the roll-up would describe HR's FTE, not the company's.
- `archetypeReports` never says one thing twice: `sales-pipeline` is Sales' board, the close checklist lives in `financial-close`, and Support and IT already report intake, so they get a board but no `backlog`.
- `ArchSecurity` reports `security-posture`, not `incident`, because `incident` is an availability report and a security team would show an empty tile forever.
- `Team.Variant` selects a builder's flavour (appsec vs detection) instead of equality checks against literal team names, which tied builders to one company.
- The spookify `helpdesk` row overrides `Reports` to add `workflow`: the one team in a streaming company running named, repeatable processes.
- `UniverseDef` is a value, not package globals, so two simulations in one process cannot race; `universes` is read only through the sorted `UniverseNames`, so registration order never leaks into output.
- `newCompany` derives divisions and departments from the team rows in fixture order, stamps `Team.universe` so `Team.Scope()` needs no company argument, and falls back from `compactTeamID` to the full name where two rows collide.
- `Scale` decides which invariants tests apply: `ScaleEnterprise` rows must have Headcount above their node count; `ScaleTeam` rows are one person each. `MinFTE`/`MaxFTE` is the band the fixture claims.
- `UniverseDef.ExtraTeamReports`, `RootReports` and `Workflows` let one company add a universal definition, replace the root scorecard, or name per-scope processes without touching the shared tables.
- `Company.Workers` returns root, then VPs, leads and team members, so a parent always exists before its children connect.

## Update seeding and wire client

- `declareFleetVersions` and `announceFleetUpdate` exist so the rollout view is not an empty state in a demo; everything goes over the real HTTP API as the workers themselves.
- `reportUpdateSpread` draws only from `Sim.rng` (Seed+2), never the generator's, and permutes the stable `Company.Workers()` order rather than a map, so the spread is deterministic and the golden hash is unaffected.
- `updateSpread` uses fractions, not counts, because fixture companies differ by an order of magnitude; the leftover stays silent (shown as unknown), and fleets above `minFleetForForcedOutcomes` always get one failure and one decline.
- `httpAPI`/`simWorker` reimplement the challenge/sign/connect handshake rather than importing `test/integration`, because that is a test package and the fixture is production code; `simWorker.token` refreshes within `tokenRefreshMargin` of expiry.

## Runtime (`run.go`)

- `Options.Universe`, `Seed` and `Now` are inputs rather than package state, so the contract "same inputs, same history" has no hidden global.
- `Sim.close` waits `closeDrain`, then cancels every request context (`BaseContext`), because SSE handlers block until their context ends and would hold `Shutdown` for all of `closeGrace`; the control plane closes only after handlers exit.
- `replayHistory` shards submissions across `replayShards` goroutines by scope path: order holds within a scope (what latest-wins and the timeline need) while scopes replay in parallel; serial replay took minutes.
- `submit` returns a validation 400 as a generator bug rather than tolerating it; `submitLive` logs and skips every other error so a demo keeps running.
- `RunLive` pulses only definitions the chosen team produces (`pickProducedDefinition`, falling back to activity) and refreshes a department report at `liveAboveTeamRefreshRate`, so above-team tiles keep moving too.

## Ping-pong demo apps (`pingpong.go`)

- `startPingPong` creates the only app records in the simulation, on the first two department leads, because an app record must point at a page that loads; `TestSimulationAppsAreAllReachable` keeps it that way.
- `pingMessageType` is request_for_input and `pongMessageType` completed_work because only those honor an explicit `to`; status_update and escalation route one hop up to the parent.
- `pingPongApp.listen` reopens the subscription after `reconnectDelay` on any stream end, minting a fresh token each time, and uses no client timeout because a quiet subscription is not a broken one.
- `countAndAck` acks only ping/pong subjects: other inbox messages belong to the rest of the simulation, and un-acked handled ones would be redelivered on every reconnect.

## Fake LLM (`fakellm.go`)

- `FakeLLM` pools are keyed by `Archetype`, so payroll never has a kafka-consumer and legal never canaries anything.
- `Vocabulary` lets a universe override word pools; a nil pool falls back to the shared spookify pools, so a company supplies only what makes it recognisably itself.
- `forArchetype` falls back to the `ArchService` pool so a new archetype never panics; a set that falls back is a gap to fill, not a design.
- `ItemTitle` is imperative (a card is an instruction) while `TaskLabel` is a gerund (an activity line is a report).

## Tests and the golden hash

- `TestGeneratedHistoryIsStable` pins a SHA-256 of the spookify history: one rng drives every builder in `History()` order, so any reordering rewrites everything while other tests still pass; re-pin only after reading the diff.
- Renaming vocabulary keeps every word pool the same length, or each later draw moves and the golden hash changes for a cosmetic edit.
- Most generator tests run against spookify, the largest fixture; invariants that must hold for every company iterate `UniverseNames`.
- `TestBuildersMatchLibrary` checks the union across universes in both directions: a definition only one company produces is not a dead builder, and a builder nobody produces is dead code.
- `TestCompanyFixture` requires headcount strictly above node count only at enterprise scale, where one row stands for a group of teams; in a small company one row is one person.

## Finance and scorecard

- `buildBudgetVariance` draws bridge movements first, derives the total from their sum, and spreads the residual across `financeCostCentres`, so the bridge and the cost-centre table show one delta.
- `portfolioNumbers` serves both `buildTeamPortfolio` (strategy team) and `buildPortfolio` (division VP), so a division's portfolio and its strategy team's are the same kind of thing.
- `portfolioItems` gives the first `atRisk` undelivered bets an already-passed `Due`, so the at-risk table and the board light from one fact.
- `buildCompanyScorecard` reads every figure from `g.co.Scorecard` so a second universe's root tile never claims another company's numbers; its two targets (`scorecardMarginTarget`, `scorecardSubsTarget`) are still spookify literals.
- `smallUniverseUsers` switches `scorecardRounding` to 1 for practice-sized universes, which count users in ones rather than hundred-thousands.

## Commercial

- `marketProfiles` apportions the company's reported MAU and premium totals across the market teams, exact by construction, so `spookify/markets` rolls up to the scorecard.
- Headcount does not drive `buildSubscriberGrowth` figures: subscribers follow the region; headcount drives only what a team can do (deals worked, releases written).
- `fallbackMarketProfile`/`fallbackSalesDesk`/`fallbackMarketingDesk`/`fallbackCommsDesk` size an unprofiled team off its headcount rather than emit a zero row that would silently deflate the roll-up.
- `buildSubscriberGrowth`, `buildSalesPipeline`, `buildCampaign` and every people/platform funnel build stage-by-stage from one anchor, so no stage exceeds the one it feeds.
- `desks` keeps brand-sales and self-serve-ads at opposite ends (six-figure long-cycle deals vs thousands of small ones); `marketingDesks` gives brand desks CAC five to ten times performance's, with targets on the instance.
- `negativeNewsCycleChance` moves mentions up and sentiment down together, so a comms report has bad days.

## People and platform

- `platformShare` gives each team its headcount fraction of a company-wide total (`catalogDailyDeliveries`, `licensingCompanyMGEUR`) so five teams' reports roll up to one company number.
- `splitByWeight` rounds every bucket but the first and gives the first the remainder, so a breakdown always sums back to its headline KPI.
- `gridTable` emits a long [row, column, value] table in declaration order because a heatmap's contract is a long table and order must be stable for the golden hash.
- `buildWorkforce` takes headcount from `tm.Headcount` and derives the bridge's opening from the movements, so bridge, scorecard and org chart land on one number.
- `buildDataPipeline` copies the reported lag onto the worst dataset before sorting, so the KPI tile and the lateness ranking cannot disagree; an open `incidentAt` worsens lag, failures and data quality.
- `buildSecurityPosture` builds the severity-by-age heatmap first and derives open/breached counts, exceptions and events from it; `detection_sources` is drawn only for the detection team.
- `buildSupportQueue` uses `g.season` at 0.55 amplitude so care follows the streaming curve; tier1-care and escalations differ in per-agent volume and response targets, not just scale.

## Planning decks and flow

- `strategy` registers at `everyWeek`, not `everyQuarter`, because a quarterly cadence puts one instance in the 48h window and never moves; `plan` registers at 12h to match `roadmap`, two readings of one commitment.
- `keyedSeed` hashes (seed, definition, scope) for `planRNG`/`planLLM`, so a card's title is identical at every tick; `tickLLM` adds `t.Unix()` so event prose differs per instance.
- `doneLingers` is a fraction of one column's width, not of the whole flow, so a two-state `todo` and a five-state `task-board` keep comparable done columns.
- `planDeck` takes `spreadDays` per definition (`backlog` wants an old tail, `task-board` births near the window); `bornInWindowShare` lets a quarter of births fall after `g.start` so work arrives instead of only draining.
- `blockOnPreviousSurvivor` picks the previous surviving card as blocker, so `blocked_by` always resolves inside the instance and edges only point backwards, making cycles impossible.
- Every `overWIPEveryNthTeam`th team is permanently over its WIP limit so the status rule fires from the first frame; `pullEvenDeckCardsIntoProgress` picks cards by deck ordinal, so the same cards stay pulled forward across ticks.
- `slipsAt` is a function of (card, t) with `holdsDatePace`/`slipsAfter`, not a die roll, so a slipped card stays slipped and the plan moves outward rather than flickering.
- `buildBacklog` measures `oldest_days` only on cards still queued, so it climbs rather than resets; `buildTodo` is the deliberate floor of the facet (two states, no lanes, horizons or edges).

## Workflow

- `WorkflowTemplate.BlockedFor` is a duration back from now, not a date, so a fixture copied from a real snapshot never reads as stale; `humanSince` renders it relatively for the same reason.
- `workflowTemplates` falls back to `genericWorkflows` for any scope a universe does not name, so every team has something real to run.
- `workflowEvents` pairs `workflow.created`/`workflow.completed` and `step.started`/`step.completed` so declared spans have ends; `step.blocked` is deliberately unpaired.
- `walkSteps` marks the blocked step `in_progress` with its `blocked_by` edges and keeps steps behind it (via `stalledBehind`) `committed`; every step is emitted so edges resolve.

## Engineering builders

- `buildPerformance` puts the latency budget (`latencyBudgetFactor`) on the instance rather than the definition because it is the team's own commitment.
- `buildLogs` forces `warnErrorLines`/`criticalErrorLines` on `warnTeam`/`critTeam`'s final instance.
- `buildTeamActivity` keeps counts small because activity's status rule warns at 50 in flight and roll-ups sum teams and departments.
- `roadmapItems` forces the first `missed` cards into `atRiskRoadmapColumn` with a past due date, so the late pile agrees with the `milestones_missed`/`slip_days` KPIs; risk is a missed date, not a column.
- `buildCost` sizes `dailyBudgetPerCostUnit` so a typical day lands near 70% of budget, with a few teams over 80% and the odd one past 100; status is judged against the team's own budget.
- `incidentLifecycle` omits stages past now, so an incident opened late in the window is still open at boot.
