# Perspective: Control Plane (CLI + Mission Control)

> **This is the fixed operational layer, not a swappable primitive.**
> See [[core.md]]. Messaging and storage are pluggable per deployment;
> this isn't — every deployment needs exactly this shape of
> bootstrapping and observing, whichever backends it plugs in
> underneath. The CLI and Mission Control don't do anything an agent
> couldn't also do by talking to the messaging and storage systems
> directly — they exist to make that tractable for a human.

The spine ([[core.md]]) says: a messaging system and a storage system,
pluggable. Something still has to (a) stand up a concrete instance of
those two systems with real backends wired in, (b) let a worker attach
to that instance, and (c) let a human see what's actually happening.
That's the control plane — three pieces:

## 1. `dream server` — register the server

A CLI command that stands up (or points at) a coordination point:

```
dream server init
  --messaging temporal://...      # or: queue://, webhook://, ...
  --storage git+filestore://...   # or: postgres://, s3://, ...
```

"The server" here isn't a monolith — it's the registered *configuration*
of which messaging backend and which storage backend this deployment is
plugged into. Different teams, different environments, even different
Factories could each register their own server pointing at different
backends. This is the literal, operational expression of "pluggable":
the config an `dream server init` writes is the thing that says which
concrete implementation ([[architecture.md]], [[memory.md]]) is live for
this deployment.

## 2. Mission Control — the UI for the same thing

Everything `dream server` can configure from the CLI, Mission Control can
configure visually — plus it's the live dashboard: what workers are
connected, what's moving through the messaging system right now, what
state changed in storage, who's reporting to whom (see the org chart
below). Mission Control doesn't have privileged access the CLI lacks —
it's a client of the same messaging/storage primitives everything else
uses, which is also what keeps it honest: if Mission Control can show
it, so could any other client, because there's no side channel.

Two jobs, one surface:
- **Configuration** — messaging backend, storage backend, connected
  workers, reporting topology. The same knobs as `dream server`.
- **Dashboard** — a live overview: agent activity, Workflow-equivalent
  progress, storage state, who reports to whom and what's flowing along
  those lines right now.

## 3. `dream worker connect` — attach a worker

A CLI command a worker (wherever it runs, whatever it is — see
[[core.md]]'s "doesn't care what an agent is") runs to join a registered
server:

```
dream worker connect
  --server <server-id-or-url>
  --reports-to <worker-id | team | server>
  --role <label>
```

Connecting is deliberately the same shape regardless of whether the
worker is a Kubernetes pod, a laptop, or a CI runner — see the "remote
workflow lifecycle" in [[architecture.md]] for what a connected worker
actually does once attached (acquire → work → sync → release). What's
new here is `--reports-to`.

A worker's place in the org chart is static once set (see "Open
questions" below), with one explicit escape hatch:

```
dream worker reassign <worker-id> --reports-to <new-parent-id>
```

Callable by the worker's current parent, the worker itself, or Mission
Control as a human-in-the-loop action — never a silent mutation. A
reassignment is itself emitted as a control message along the org
graph, so it's visible in the same activity stream as everything else,
consistent with core.md's "visible, not opaque."

## Connectivity is a graph, not a pool

The obvious default is: every worker connects flat to one server, and
that's the whole topology. That's not sufficient on its own. Workers
need to be able to form a real **organization chart** — how a given
worker reports, and to whom:

- A worker can report directly to the server (the flat case).
- A worker can report to *another worker* (a lead agent, a
  reviewer, a coordinator) — which itself reports further up.
- Reporting can fan out (one worker's output feeds several others) or
  fan in (several workers report to one aggregator/reviewer) — this is
  a graph, not strictly a tree, though a tree is the common case.
- "Report" is a messaging-primitive concept: status, completed work,
  escalations, and requests-for-input all flow along report-to edges,
  not just into an undifferentiated shared channel.

A vocabulary note, and a reconciliation. At the vision level these
entities are **nodes** — see [[core.md]] for the term; "worker" is the
implementation-level name for the same thing, and this doc keeps
"worker" because it describes the concrete CLI. (The CLI itself now
accepts both registers: `dream node ...` is an alias for `dream worker
...`, so agent-facing instructions can speak the vision vocabulary.) The consequence that
matters carries over either way: **the org chart is edges between
workers/nodes, not between containers.** A worker's direct boss is
just another node, and that boss may live at any higher level of the
optional organizational hierarchy ([[organization.md]]) — a node down
in "Site X-001" reporting straight to a boss-node up in "World X" is a
well-formed edge, not a special case. The reference implementation
enforces a single parent per worker and one-hop escalation along that
edge; both remain true and are fully compatible with level-skipping,
because "level" is an organizational reading laid over the graph, not
something the edge itself knows about. There is no rule — in the model
or in the build — that a reporting edge must connect adjacent levels.

This matters because "just messaging and storage" still has to answer
*who gets told what*. A flat broadcast model doesn't scale past a
handful of workers, and it doesn't match how real engineering
organizations actually coordinate — leads triage before escalating,
reviewers see aggregated work not raw firehose, and a manager-equivalent
agent needs to see rollups, not every leaf event. The org chart is how
the messaging primitive stays usable as the number of workers grows,
without becoming a second, hidden opinion about how work must be
structured (any topology is legal — flat, deep, fan-in, fan-out — the
system just needs to represent and route on it).

## Open questions — resolved for the reference implementation

These were open when this doc was written. The Go build resolves them
as follows; noted as build decisions, not as the only legitimate
answers, since a different deployment could reasonably choose otherwise.

- **Is `--reports-to` static or mutable at runtime?** Resolved: static
  at connect time, with an explicit, audited escape hatch —
  `dream worker reassign <worker-id> --reports-to <new-parent>` — a
  first-class control-plane operation (itself emitted as a control
  message, not a silent mutation), callable by the worker's *current*
  parent, the worker itself, or Mission Control as a human-in-the-loop
  action. Static-by-default keeps routing tables simple and avoids
  mid-flight message misdelivery; reassignment is a rare, deliberate,
  logged event rather than something that needs to be cheap or frequent.
- **Does the storage primitive persist the org chart, or is it
  first-class?** Resolved: first-class control-plane state, owned by
  the server's own datastore — not routed through the swappable storage
  backend. Routing correctness and the security model's trust decisions
  (see [[security.md]]) depend on the org chart being available even if
  the storage backend is degraded or misconfigured; it's exposed
  read-only to storage/messaging backends via the server, not stored as
  "just another blob."
- **How does Mission Control degrade for a deep org chart?** Resolved:
  roll up by depth — full render to depth 3 from the viewed root, with
  deeper subtrees collapsing into an expandable "N reports, M active"
  summary node computed from aggregated status rather than raw leaf
  data, plus a search box that jumps directly to any node with its
  ancestor chain expanded. Worth flagging: this is in some tension with
  the visual design system's general "avoid dense dashboards" guidance
  — the rollup is precisely what keeps a deep chart from becoming dense
  by default, but it's a real design trade-off, not a free resolution.

See also: [[core.md]] for why messaging and storage are the only two
primitives Robot Dreams owns (and for the node/worker vocabulary),
[[organization.md]] for the optional Universe → Worlds → Realms →
Sites hierarchy the org chart can be read against,
[[architecture.md]] for what a connected worker does with its lease
once attached, [[security.md]] for the identity and key-lifecycle
model behind `dream worker connect`'s credentials.
