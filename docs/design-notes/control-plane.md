# Control plane — design notes

## identity

- `ValidateToken` selects the JWKS key by the token's `kid` and falls back to trying every key when `kid` is absent or unknown, so a stale header never rejects a valid token.
- `NewJWKS` publishes the previous key beside the current one so tokens minted just before a rotation still validate during the grace window.
- `Issuer` does not consult a `RevocationStore`; callers combine `Validate` with a revocation lookup so key handling and revocation policy stay separable.
- `SQLiteRevocationStore` lives in `internal/identity`, not `internal/server`, so CLI tooling and tests can persist revocations without the server package.
- `NewSQLiteRevocationStoreFromDB` never closes a caller-supplied `*sql.DB` (`ownsDB` false), so one control-plane SQLite handle is shared across registry, revocation and config.
- `sqliteSingleWriterConns` (identity) and `openFile`'s `SetMaxOpenConns(1)` (store) cap SQLite at one connection: SQLite has a single writer, so `database/sql` serializes goroutines instead of surfacing SQLITE_BUSY.
- `revocations.revoked_at` stores unix nanoseconds, matching `pkg/messaging/embedded`, so a shared control-plane datastore reads consistently across tables.
- `SQLiteRevocationStore.Revoke` upserts (a second revocation replaces reason and timestamp); `Unrevoke` stays off the `RevocationStore` interface because revocation is deliberate and audited.
- `newJTI` and `server.NewID` fall back to a timestamp id when `crypto/rand` fails rather than panicking mid-request; `api.randomBytes` panics instead because a challenge without entropy is unsafe.

## org chart

- `Graph` is a forest under an implicit root (`ReportsTo == ""`); edges point child to parent and stay acyclic by construction: `AddWorker` rejects unknown parents, `Reassign` rejects cycles via `hasAncestorLocked`.
- `EscalationTarget` returns exactly one hop; routing calls it again from each recipient, never `Ancestors`, so escalation cannot skip to a grandparent or root.
- `Graph` methods return `clone`d workers so callers cannot mutate graph state through the shared `PublicKey` slice or `Metadata` map.
- `ParseChartFile` returns workers in file order, not parent-before-child; `AddWorker`'s duplicate and dangling-parent checks are a second line of defense, so loaders retry `ErrParentNotFound` or sort by depth.
- `Rollup` aggregates one level deep (direct reports only); a dashboard calls it once per node so each number is attributable to a single hop.
- `RollupWithActivity` takes fresher status via an `ActivityInfo` map instead of reaching into a presence backend, keeping aggregation decoupled from messaging.

## server

- `backends.go` blank-imports the embedded queue and localfs backends in `internal/server`, not `cmd/dream`, so anyone constructing a `Server` directly gets working zero-config backends.
- `DefaultKeyID` is a fixed string so a restart on the same data directory publishes the same `kid` for the same on-disk key; key rotation is deferred.
- `EmitFromWorker` drops a `status_update` or `completed_work` at root (empty `To`, nil error) but fails `escalation` and `request_for_input` with `ErrNoEscalationTarget`: only the latter need an answer.
- `EmitFromWorker` honours an explicit `To` for `completed_work` and `request_for_input` only if it names a registered worker; `status_update` and `escalation` always go one hop up.
- `EmitFromWorker` checks only that a `StoragePtr` has a path, never that the object exists, so an emit costs no storage round-trip and may reference another deployment's backend.
- `ReassignWorker` reads the current parent before the move so the outgoing parent, not the incoming one, is the one whose consent counts; a committed move is not rolled back when the notice fails to emit.
- `loadGraph` re-attaches a worker whose stored parent is missing to root (`reattachOrphansToRoot`) and persists the repair, so no registry row silently vanishes after tampering with `control.db`.
- `Config.ReportsDir` is re-read on every startup, unlike `OrgChartFile`, because a report library is declarative configuration and a newer one on disk should win.
- `MintAdminToken` carries literal `storage:read:*`/`storage:write:*` beside `AdminScope` so code inspecting `Scopes` directly still sees that admin can touch everything.

## store

- `Store` owns one `*sql.DB` for workers, revocations, config, reports, updates, apps and schedules, so one file is the whole control plane; `Store.DB` exposes it for future tables and only `Store.Close` closes it.
- `openFile` runs WAL with `synchronous=NORMAL`: commits skip per-commit fsync; a power loss may drop the last commits, acceptable since long-term records live in the storage primitive.
- `server_config` pins `id = 1` with a CHECK constraint: one configuration per database file, so reading it is a single unambiguous row lookup.
- `OpenURI` returns `ErrUnsupportedBackend` for `postgres://` rather than falling back to SQLite, so a misconfigured deployment fails at startup instead of writing state elsewhere.
- `reportTimeLayout` stores report times as fixed-width RFC3339 TEXT so lexicographic order equals chronological order, which pruning ORDER BY and since-filters rely on (pinned by `TestReportTimeLayoutSortsLexicographically`); other tables keep unix-nanosecond INTEGER because nothing range-scans them in SQL.
- `ListReportInstances`/`ListReportEvents` apply `reporting.ScopeWithin` in Go, not as a SQL LIKE, so scope semantics cannot drift from the library contract.
- `report_instances.payload` is the whole `reporting.Instance` JSON: the instance is the durable record within retention, so the submitted document round-trips unchanged.
- `InsertReportEvents` stores the definition and scope arguments, not each event's own tags; `ListReportEvents` re-tags on the way out.
- `update_announcements.recipient_count` is the intended recipient count at announce time; actual deliveries are a fact about one API call and are returned, not stored.
- `DeleteWorker` also deletes `worker_update_state` and `worker_apps` rows: no table has a foreign key onto workers, so they would otherwise outlive the worker.

## API

- The `api` package authenticates, decodes, encodes and streams only; every routing and reassignment decision lives in `internal/server`, so a non-HTTP front end inherits identical behavior.
- `NewAPIRouter` applies `withAuth` per route rather than wrapping the mux, so each registration line shows whether it is public and no prefix exception list can drift.
- `handleChallenge` and `handleToken` never reveal whether a worker ID exists (a challenge is issued and consumed for unknown IDs too), so the unauthenticated endpoints are not an enumeration oracle.
- `challengeStore` keeps nonces in memory only, one per worker ID, replaced on reissue, and `consume` deletes the nonce on every lookup, verified or not, so a wrong signature cannot be retried against a live challenge.
- `verifyChallenge` requires the client to echo the server-issued nonce exactly; signing self-chosen material would prove possession but not freshness.
- `handleToken` reads the public key from the org chart, never from the request; a refresh that could supply its own key would let any keypair take over any worker ID.
- `revocationCache` lives in the validator (this front end), not in `identity.RevocationStore`, because each validator (a storage backend validating on its own) gets its own cache; `handleRevoke` busts the entry immediately.
- `handleRevoke` requires the admin scope and does not require the worker to exist in the org chart; revoking an ID that never completed connect is a legitimate pre-emptive action.
- `requireEnrollmentAuth` compares the enrollment secret with `constantTimeEqual`; length leaks, but the secret is fixed-length high-entropy so that is harmless.
- `decodeJSON` caps every body at `maxJSONBodyBytes` before auth runs (pinned by `TestDecodeJSONRejectsOversizedBody`), so an anonymous caller cannot exhaust memory via the unauthenticated connect endpoints.
- `handleListWorkers` returns a flat, ID-sorted list with `reports_to` edges; clients assemble the tree, keeping one unambiguous wire shape; `workerView` is a separate type from `orgchart.Worker` so JSON names stay stable.
- `reassignRequest` treats an empty `reports_to` as meaningful: it moves the worker to the root, so blank is not "missing".
- `handleHealth` is unauthenticated on purpose: a liveness probe needs no credential and the body says only up or down, never what or where.
- `eventHub` polls orgchart, storage and messaging once per `pollInterval` and fans out to every `/api/events` subscriber, so load scales with the org chart, not with viewers; the poll goroutine runs only while a subscriber exists.
- `eventHub.broadcast` drops an event for a subscriber whose buffer is full rather than blocking the hub on a stalled client.
- `pollMessagesLocked` rolls status_update messages into one `message_volume` event per (from,to) edge per tick and skips `SubjectUpdateAvailable` from `ControlWorkerID`, which `handleAnnounceUpdate` already broadcast once.
- Report and update writes broadcast to SSE synchronously in their handlers because the handler is the sole ingestion point; poll-diffing is only for systems with no push.
- `handleReportSummary`/`handleReportTimeline`: a definition declaring no stances passes every stance filter, while one declaring no categories passes none — a category claims subject, a stance claims judgement.
- `deriveSpans` closes an end event against the oldest open start from the same origin scope (`openByScope`, FIFO), so overlapping spans from different sites never swallow each other's ends; an unmatched start keeps a null end.
- `reportTimelineCap` equals `store.ReportEventRetention` so an uncapped single-scope report never truncates below what the store retains anyway.
- `boundEventsToPeriod` sets `Since` one nanosecond before the period start because `ReportEventFilter.Since` is strictly-after while the period start is inclusive.
- `handleReportInstancePost` stamps a missing `produced_at` before `ValidateInstance`, which rejects a zero value.
- `handleSubscribe` streams SSE rather than long-polling or WebSockets: the backend already yields a context-cancelled channel, no cursor protocol is needed, and the stream is one-way.
- `envelopeView.Delivered` is false when routing found no recipient (sender reports to root); `handleEmitMessage` answers 409 for `ErrNoEscalationTarget` because the request is valid and only the chart's shape makes it impossible.
- `handleGetObject` serves both get (`?path`) and list (`?prefix`) on one route because ServeMux matches on path, not query; the presence of `prefix`, even empty, selects listing.
- Conditional storage writes use query `if_match_revision`, not the `If-Match` header, because HTTP `If-Match` carries ETag semantics (quoting, weak compare, `*`) the store does not implement.
- `maxObjectUploadBytes` bounds a PUT body because both localfs and s3compat buffer the whole body in memory before writing.
- `getObject` returns raw bytes with `X-Path`/`X-Revision`/`X-Updated-*` headers so a client gets content and metadata in one round trip without base64 wrapping.

## delegation

- `childNameRe` is `workerIDRe` itself: a child's name obeys the registered-ID rule, and neither may contain `DelegationSeparator`.
- `MintDelegatedToken` enforces every delegation rule in `Server`, not the HTTP handler, so an in-process caller gets the same answer; `handleDelegate` sits behind `withAuth` like any worker action.

## scheduler

- `FireSchedule` never touches `NextAt` or `LastAt`; the caller decides whether a fire counts as an occurrence (`Tick`) or not (manual fire).
- `Tick` records `LastAt` only for a successful emit (`lastFiredAt`); a failed fire advances `NextAt` but is not a fire.
- `API.RunScheduler` starts the schedule loop from the api package because the SSE hub lives there and every fire is broadcast as `schedule_fired`.

## updates

- `AnnounceUpdate` writes the announcement row before the fan-out, so a node reporting back immediately can never reference an announcement the server has no record of.
- `AnnounceUpdate` and `UpdateRollout` sort `graph.List()` by ID (`workersByID`) because the graph's order is unspecified and the fan-out and counts must be deterministic.
- `UpdateRollout` without an announcement ID scores every node against the latest announcement, not whichever one each node last mentioned, to prevent a false all-clear.
- `rolloutNodeFor` carries `CurrentVersion` through even when a node's stored status is about another announcement, so the operator sees where the node is.
