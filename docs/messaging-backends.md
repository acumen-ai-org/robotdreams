# Messaging backends

`pkg/messaging.MessagingBackend` is a deliberately dumb single-hop
transport: `Emit(ctx, Envelope)`, `Subscribe(ctx, workerID)`, `Ack(ctx,
Ack)`, `Tail(ctx, filter)`, `Health(ctx)`, `Close()`. It does not do
org-graph routing (escalation hops, status rollups) — that lives in
`internal/orgchart`/`internal/server`. An `Envelope` carries a `Type`
(`status_update`, `completed_work`, `escalation`, or
`request_for_input`), `From`, `To` (a resolved single hop), `Body`, and
optionally a `StoragePtr` (backend/path/revision) rather than the payload
itself — used by convention for `completed_work`.

A fifth kind, `scheduled`, is sent by the control plane itself rather
than by a worker: when a schedule ([scheduling.md](scheduling.md)) comes
due, the server emits one envelope from the schedule's owner to its
recipient, carrying the registered subject and body and the schedule id
as `CausationID`. It routes to exactly `To` with no parent hop, and
`POST /api/messages` refuses the kind so a worker cannot forge one.
Backends need nothing special for it — it is one more envelope through
`Emit`.

Select a backend with `--messaging <uri>` on `dream server init`, or leave
it unset for the zero-config default. The chosen URI is persisted in the
data directory, so a later flagless run keeps using it.

## Embedded (SQLite) — `queue://sqlite/<path>` — default

The zero-config default. A pure-Go SQLite database (via
`modernc.org/sqlite`, chosen so the binary cross-compiles without cgo)
under the control-plane data directory. No external dependency.

```
queue://sqlite/var/lib/dream/messages.db
```

`dream server init` with no `--messaging` flag uses
`queue://sqlite/<data-dir>/messages.db` automatically. Delivery is
poll-based: `Subscribe` checks for new messages every 300ms. This trades
sub-second delivery latency for implementation simplicity — correctness
(no missed or duplicated messages) matters more than immediacy for this
backend.

### Postgres variant — not implemented

`queue://postgres/...` is registered as a recognized URI but returns:

> `messaging/embedded: queue://postgres backend is not yet implemented`

This is a documented stub, not a working backend — the embedded package's
design leaves room for a Postgres-persisted variant but the reference
build only ships SQLite.

## Webhook — `webhook://`

Delivers envelopes via plain HTTP POST callbacks — "no broker required."
A bare `webhook://` URI selects the backend with default retry/backoff
settings; it does **not** encode any worker's endpoint.

**Callback registration** is a separate, code-level step: a caller (the
control-plane server process embedding this backend) calls
`Backend.RegisterCallback(workerID, url)` for each worker it wants to
reach, or supplies a `CallbackResolver` (`WithCallbackResolver` /
`WithDefaultCallbackURL`) that resolves worker IDs to URLs dynamically.
There is no CLI flag for this — it's a construction-time option for
whatever process wires up the webhook backend.

**Delivery semantics**: `Emit` makes one synchronous POST attempt
immediately (JSON-encoded `Envelope`, `Content-Type: application/json`).
On failure (non-2xx, network error, or timeout) the envelope goes to an
in-memory outbox that retries with exponential backoff up to a
configurable attempt cap — **at-least-once** delivery. `Emit` itself never
blocks on retries and never silently drops a message after one failure.
The outbox is in-memory only: pending/retrying deliveries do **not**
survive a process restart.

Because this backend is push-based, `Subscribe`/`Tail` are backed by a
small in-memory, per-worker ring buffer (200 entries) of envelopes that
were *successfully delivered* over HTTP — not a separate log. `Subscribe`
only sees deliveries from the point it starts (no history replay); use
`Tail` for history. If no callback can be resolved for a recipient at
`Emit` time, the call still succeeds (queued in the outbox) so a callback
registered later can still pick up the pending delivery within the retry
window.

**Auth**: the outbound POST carries no built-in authentication of its own
— securing the callback endpoint (e.g. requiring a bearer token, an
allowlisted source) is the receiving worker's responsibility.

**SSRF note**: `RegisterCallback` is a Go-level construction option today,
not exposed over HTTP — nothing in this build lets a worker register its
own callback URL remotely, so there's no live SSRF path yet. If a future
phase adds an HTTP endpoint for worker-supplied callback registration,
that endpoint must validate the URL (deny private/link-local/cloud
metadata ranges, restrict scheme) before storing it — `attemptDeliver`
itself does no such filtering.

## Temporal — `temporal://<host:port>/<namespace>`

A genuine Temporal-backed backend, using Temporal Workflows and Signals as
the message log ("Temporal signals / workflow execution history as the
message log").

```
temporal://localhost:7233/default
temporal://temporal.internal:7233/robotdreams?taskqueue=my-queue
```

Host:port selects the Temporal server address; the URI path (leading
slash stripped) selects the namespace, defaulting to `default` if empty.
An optional `?taskqueue=` query parameter overrides the task queue name.

**Running it for real** requires a real Temporal server reachable at that
host:port (self-hosted, Temporal Cloud, or `temporal server start-dev`
for local development — see the [Temporal
docs](https://docs.temporal.io/) for setup). This backend does not embed
or manage a Temporal server itself; it only connects to one and starts a
`worker.Worker` polling the configured task queue.

**Mailbox workflow design**: one long-running Workflow Execution exists
per worker ID (its "mailbox"), with a deterministic Workflow ID derived
from the worker ID.

- `Emit` uses `SignalWithStartWorkflow`: starts the mailbox workflow if it
  doesn't exist, or delivers a `"deliver"` Signal to the running one.
- `Ack` sends an `"ack"` Signal, removing the matching envelope from the
  workflow's pending buffer.
- `Tail` issues a `"pending"` Query directly (read-only, doesn't mutate
  state).
- `Subscribe` has no native external-push hook in Temporal, so it polls
  the same `"pending"` Query every 300ms, diffing against envelope IDs
  already delivered to that subscription so nothing is redelivered within
  one subscription's lifetime.

An ever-growing workflow history is a recognized Temporal anti-pattern;
the mailbox workflow continues-as-new after a fixed number of processed
signals, carrying forward only still-unacked envelopes.

`Ack` queries the recipient's mailbox for the ID before signalling, so an
unknown ID — or one addressed to another worker — returns
`messaging.ErrNotFound` like the other backends. Because the mailbox keeps
no record of acked envelopes, a second ack of the same ID also reads as
unknown; embedded and webhook stay idempotent there.
