# Perspective: Updates (a base contract, not an update runtime)

> **This is a contract the control plane carries, not a capability it
> performs.** The *announcement* — that a version of some kind exists —
> is a universal, non-negotiable fact about running a fleet, so it
> belongs to the control plane ([[control-plane.md]]) and rides the
> messaging primitive ([[core.md]]) like everything else. The *act of
> updating* is runtime-specific, so it belongs to whatever plugs in.
> Robot Dreams ships the message and the record of what nodes said
> back. It ships no updater, no supervisor and no drain machinery, and
> it never interprets a version string.

A node connects and, while connected, can receive messages. Until now
there was no way to tell it that new code exists — not for the `dream`
tool it is already running, and not for anything a deployment needs to
push out to its own nodes.

Two needs, and the temptation is to treat them as two features:

1. **Robot Dreams updating itself.** Anything that ran `dream worker
   connect` has the `dream` tool.
2. **Anything else, in an implemented universe.** A prompt pack, model
   weights, a sidecar, a policy bundle — whatever a particular
   deployment's node runtime is made of.

They are one mechanism. The message says *what kind* of thing has a new
version; the node decides whether that kind is any of its business.

## The scope test, applied honestly

[[core.md]]'s test asks whether a feature belongs to a swappable
primitive, to the fixed operational surface, or to neither — in which
case it belongs to whatever plugs into Robot Dreams rather than to
Robot Dreams.

Updates split cleanly across that line, and the split is the whole
design:

- **The announcement is control plane.** Every deployment needs the
  same shape of "something changed, here is what and where" reaching
  every node, and needs to see which nodes acted on it. That is the
  same kind of universal operational fact as "a worker attached" or
  "a credential was revoked."
- **Applying an update is neither.** It is agent- and
  runtime-specific, and by [[core.md]]'s rule that means it does not
  belong in Robot Dreams at all. A node might be a Kubernetes pod that
  updates by being replaced, a laptop running `npm install`, a process
  that swaps a file and re-execs, or a person reading a message. Robot
  Dreams is **100% agnostic about what a node's runtime is**, and the
  fastest way to stop being agnostic would be to ship the updater.

So the procedure — *stop taking new work, let work in flight finish,
apply, restart, resume* — is written down as **guidance and nothing
else**. No API enforces it. No library implements it. A node may
deviate, or ignore an announcement entirely, and that is a legitimate
outcome the contract has a word for.

The one exception proves the rule: Robot Dreams *does* implement
updating the `dream` binary, because that is the one runtime it
actually owns. It is a consumer of the contract, not a privileged part
of it — it uses the same message every other kind uses.

## Broadcast, because the server announces and the node decides

Every connected node receives every announcement and filters on the
kind itself. There is no selector, no targeting, no canary staging.

This costs something real: an operator cannot roll out to one Realm
first and the rest after. It is still the right shape, because the
alternative makes the control plane an orchestrator that decides what
each node should be running — which is precisely the judgment the
spine refuses to take from the node. "It is up to that node" is not a
simplification here; it is the position.

Delivery is durable, so this is not a broadcast into the void: a node
that was offline when something was announced finds it waiting on
reconnect. That is the messaging primitive doing its job, not a
special case for updates.

## Bidirectional, because a rollout nobody can see is not a rollout

The announcement goes out; nodes report back what they are running and
how an update went. Both directions are generic across kinds, which is
what makes the contract worth having rather than a one-off for the CLI:
a deployment announcing its own kind gets the same fleet-wide view of
who is on what, with no change to Robot Dreams.

This follows [[core.md]]'s "visible, not opaque" and [[security.md]]'s
posture that a fleet mutation is never silent. It also gives the
honest answer to the question an operator actually asks — *is the fleet
on the new version?* — including its most useful part: which nodes have
not answered, and whether that is because they are down or because
they are ignoring it.

The report is not a promise. Robot Dreams records what a node says
about its own runtime and does not verify it: it never checks that a
version is plausible, or that a node claiming to have applied an update
really did. Auditing a node's reality against its claims is the
[[environment.md]] drift problem, not this one.

## Vocabulary

**Kind.** What is being updated, namespaced as `<namespace>/<name>`.
`robotdreams/cli` is reserved and is the only kind Robot Dreams
defines; the `robotdreams/` namespace is closed so a deployment cannot
squat it and collide with a later release. Everything else belongs to
the deployment.

**Version.** An opaque string. Robot Dreams never parses, orders or
compares it, because it cannot know whether a given kind is versioned
by semver, by date, or by a git SHA. Any feature that would require
comparing two versions server-side is out of scope by construction.

**Severity.** `optional`, `recommended`, `required` — advisory, and
interpreted by the node. This is deliberately where "update now versus
update at next idle" lives: it is a node-side policy decision, not
Robot Dreams policy.

## Build scope for the reference implementation

**Shipping now:** the announcement message and its broadcast, the
report-back and the per-node update record, the rollout view, the
`dream updates` command group, the agent-facing contract in
`dream updates contract` and the node onboarding brief, and
`dream self-update` for the `robotdreams/cli` kind.

**Deferred, not dropped:**

- **Signed artifacts.** The self-update path verifies a SHA-256 from
  the release's `checksums.txt`, which catches corruption and a bad
  mirror but is not a signature — it travels the same channel as the
  artifact it describes. Signing (and verifying against a key that did
  not arrive alongside the artifact) is the obvious next step.
- **Continuous version reporting.** A node reports its version at
  connect and whenever it chooses. A node that never reports again
  shows a stale version indefinitely.
- **Mission Control surfacing.** Rollout state is reachable from the
  CLI and the API; the dashboard does not show it yet.
- **In-place self-update on Windows.** Refused with instructions
  rather than half-supported; see `docs/updates.md` for why.

## Open questions

- **Should a rollout ever be targetable?** Broadcast is the honest
  default, but a large fleet may genuinely want a canary. The question
  is whether targeting can exist without the control plane starting to
  decide what each node should run.
- **Does the control plane ever get to disbelieve a node?** Today a
  node's report is taken at face value. [[environment.md]]'s drift
  concept suggests a world where declared and observed state are
  compared — and updates would be one more thing to compare.
- **Where does a deployment publish its artifacts?** `source` is a
  free-form locator that Robot Dreams never fetches. A deployment
  could put a book in the library ([[core.md]]) and point at it, which
  would make the storage primitive the natural artifact channel — but
  nothing requires that today, and requiring it would be an opinion
  about runtimes.
- **Is the key-rotation push the same shape?** [[security.md]] defers
  "the server pushes a re-key, the worker accepts it," which is
  structurally this contract with different content. Whether they
  should converge, or stay separate because one is a security
  primitive and the other is advisory, is unresolved.

See also: [[core.md]] for the scope test this layer has to keep
passing and for the node/worker vocabulary, [[control-plane.md]] for
the operational surface it extends and the "never a silent mutation"
precedent set by reassignment, [[security.md]] for the deferred
server-driven re-keying this resembles, [[environment.md]] for the
drift question a node's self-reported version eventually runs into.
