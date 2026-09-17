# Messaging — design notes

## Interface and registry

- `MessagingBackend` is a single-hop transport: `Envelope.To` is already resolved; org-graph routing lives in `internal/orgchart` and `internal/server`, one Envelope per recipient.
- Backends store `MessageType` without interpreting it; the convention that `TypeCompletedWork` carries `StoragePtr` is the caller's to enforce.
- `Ack` returns the same `ErrNotFound` for "no such message" and "not addressed to ack.ByWorker", so a worker cannot probe other workers' mail through Ack.
- `Close` ends every live `Subscribe` without waiting for the subscriber's ctx, because a poller would otherwise keep ticking against a closed store for the ctx's lifetime.
- `messaging.Register` panics on a duplicate scheme so two backends claiming one scheme fail at init instead of one silently shadowing the other.
- `pkg/messaging` never imports a backend; each backend registers itself in `init()`, so a blank import is what makes a scheme available to `messaging.New`.
- `testsuite.RunConformance` calls `factory` once per subtest so no subtest observes another's state.
- In `embedded` and `temporal`, only the polling goroutine sends on or closes the Subscribe channel, so the close can never race a send.

## Embedded (SQLite)

- `modernc.org/sqlite` is used over `mattn/go-sqlite3` so the binary cross-compiles without cgo.
- `openStore` sets `db.SetMaxOpenConns(1)` because SQLite allows one writer; serialising at database/sql level avoids `SQLITE_BUSY` under concurrent goroutines.
- `pollSince` uses `rowid` as the delivery cursor because it orders rows by insertion even when `created_at` values collide.
- `Subscribe` skips a tick on a transient poll error rather than ending the subscription.
- Every new `Subscribe` starts from rowid zero and re-reads the backlog; the `acked_at IS NULL` filter alone keeps a reconnecting listener from redoing handled work (`TestAckedMessagesAreNotRedeliveredToANewSubscription`).
- That redelivery test lives in `embedded`, not the conformance suite, because the interface does not yet say every backend must guarantee it.
- `sqliteStore.ack` reports an already-acked message as found, so Ack is idempotent, while a missing or foreign one reports not found.
- `Tail` returns most recent first, since callers want a worker's latest activity.
- `Close` closes `closed` before the database handle; a poller mid-query sees one error and exits on its next select, so the order is a courtesy, not a requirement.
- `dispatch` returns a clear error for `queue://postgres` rather than silently falling back to SQLite.

## Webhook

- `Emit` makes one synchronous POST so the healthy path has no poll-interval latency; a failure counts as attempt 1 and goes to the outbox.
- `RetryPolicy.delayFor(n)` is the delay before attempt n+1, since attempt 1 already happened in `Emit`.
- Outbox retries are scheduled against the injected `security.Clock` and driven by `Backend.Tick`, so tests assert backoff without sleeping.
- `defaultPollInterval` governs only how promptly a due retry is noticed, never the retry delay itself.
- The outbox is in-memory; pending retries do not survive a restart. A SQLite-backed outbox is a possible later improvement.
- `enqueueFailed` is only reached after a first attempt, so `Emit` succeeds when no callback resolves; a callback registered later picks the delivery up within the retry window.
- `maxRetainedTerminal` bounds delivered/failed items kept for `Inspect` and `Health`, evicting oldest first; pending items are never evicted because they are the work queue.
- `retainTerminalLocked` deletes an evicted ID only if its item is still terminal, because a re-Emit of the same ID replaces the entry with a fresh pending one.
- `subscription` serialises `deliver` and `close` under its own mutex because fan-out and teardown run on different goroutines and a send on a closed channel panics (`TestSubscribe_UnsubscribeRacingDeliveryNeverPanics`).
- `subscription.deliver` gives up the send once the subscriber's ctx ends or the backend closes, so a reader that stopped draining never wedges the sender.
- `onDelivered` snapshots the subscriber list before sending so a slow subscriber never blocks other workers' Subscribe or teardown.
- `defaultSubscriberBuffer` lets delivery bookkeeping continue while a reader is briefly slow.
- `Ack` checks the recipient's ring (`defaultRingBufferSize`) rather than a separate delivered-ID set, so Ack memory is bounded exactly like Tail; an envelope evicted from the ring can no longer be acked.
- `Ack` is idempotent; no acked/unacked state is tracked because delivery already happened over HTTP.
- `Health` fails on any retry-exhausted outbox item and after `Close`; pending retries are not unhealthy.
- `Subscribe` sees only deliveries after it starts; `Tail` serves history from the same ring.
- `dispatch` for a bare `webhook://` returns a Backend with no callbacks; callers type-assert to `*Backend` and call `RegisterCallback`.

## Temporal

- `WithMailboxPrefix` exists so the conformance suite, whose factory runs per subtest against one shared dev-server namespace, gets non-overlapping mailboxes; production leaves it unset.
- `mailboxWorkflow` uses only `workflow.*` APIs, never `time` or the injected clock, per Temporal determinism; `WithClock` only defaults `CreatedAt` in `Emit`.
- The Backend owns a `worker.Worker` because `SignalWithStartWorkflow` only enqueues; without a worker polling `TaskQueue` no mailbox would ever run.
- `Subscribe` polls `QueryPending` because Temporal has no external push hook; its first tick sees envelopes emitted before Subscribe, so the promise is no missed and no duplicate delivery within one subscription, not after-subscription-only.
- `forgetIDsNoLongerPending` prunes the subscription's delivered set to IDs still pending each tick, bounding it to the mailbox cap at the cost of not deduplicating a re-Emit that reuses an acked ID.
- `Ack` queries pending first and returns `ErrNotFound` when the mailbox lacks the ID, because a Signal is fire-and-forget; a second Ack of the same ID is therefore `ErrNotFound`, unlike embedded and webhook.
- Two concurrent Acks of one ID both pass the check and both signal; `removeEnvelope` makes the second a no-op.
- `Tail` returns an empty result, not an error, when no mailbox workflow exists yet.
- `isNotFound` matches the error text rather than `errors.As` on `serviceerror.NotFound`, avoiding a dependency on the wrapped type that changes across SDK versions.
- `maxBufferedEnvelopes` drops the oldest unacked envelope from the queryable buffer so a stuck consumer cannot grow the mailbox; the signal is still in history and `Emit` still succeeds.
- `continueAsNewAfterSignals` is a fixed signal count, not a history byte size; `absorbAlreadyQueuedSignals` drains buffered signals first so the rollover drops none.
- `workflowStartToCloseTimeout` is a defensive ten-year bound; continue-as-new resets it before it matters.
- `Health` uses `client.CheckHealth`, which errors unless the server reports SERVING.
- Tests use `testsuite.StartDevServer` (a real Temporal dev server) rather than `TestWorkflowEnvironment`, because the conformance suite needs a real client; no container variant since the dev server is already real.
- The goroutine leak test allows `goroutineSlackAfterClose` because gRPC and SDK pollers unwind briefly after Close.
