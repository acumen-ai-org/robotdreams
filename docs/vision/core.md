# Core: What We Want to Achieve

*(Synthesized from [[architecture.md]], [[memory.md]], [[hierarchy.md]],
[[organization.md]], [[prior-art.md]], [[control-plane.md]], and
[[security.md]]. This is the source for the website's Vision/Idea
page.)*

## The one-sentence version

Robot Dreams is two primitives you can swap — messaging and storage —
held together by two layers you can't: a control plane to run them, and
security to trust them.

This supersedes the earlier "two primitives, nothing else" framing.
That framing was true the day it was written, but it stopped being the
honest description of the project the moment [[control-plane.md]] and
[[security.md]] were added as real, permanent parts of it — not
experiments, not later footnotes. Presenting them as equal peers to
Messaging and Storage in the site's navigation while the prose insisted
"nothing else" was a contradiction worth resolving rather than
papering over. The distinction that actually matters isn't "two things
exist vs. four things exist" — it's **which of the four are
swappable** and which aren't. See below.

**Status: in active development.** This is no longer just a design —
a Go reference implementation is being built in the open at
github.com/acumen-ai-org/robotdreams, Apache-2.0 licensed. What follows is
still the authoritative design; where the reference build makes a
concrete, scoped choice that's narrower than the design (e.g. which
storage backends ship first, which open questions below got resolved
for v0.1), that's called out explicitly in the relevant doc rather than
silently assumed — the build is expected to grow into the full design
over time, not redefine it.

## The spine, not the stack

The earlier framing of this doc leaned hard on specific technology —
Temporal, GKE, Filestore, PostgreSQL, git. That's not wrong, but it's
not the point either: it's **one reference implementation of the
spine**, not the definition of Robot Dreams itself.

The actual bet is narrower and more radical: Robot Dreams doesn't care
what an agent *is* (a Claude session, a Nous Hermes agent, a bash script
with a model behind it, a human, eventually), doesn't care *where* it
runs (a Kubernetes pod, a laptop, a CI runner, a phone), and doesn't
care *how* work gets defined (a Temporal Workflow, a cron job, a plain
function call). All of that is pluggable — swappable per deployment,
per team, per agent. What Robot Dreams actually provides is four
things, in two different registers:

**Swappable — the primitives:**

1. **A messaging system** — however agents (and the things coordinating
   them) talk to each other. Could be Temporal signals, could be a
   queue, could be webhooks, could be something else entirely.
2. **A storage system** — however agents read and write shared state.
   Could be git + a leased shared filesystem (see [[architecture.md]]
   and [[memory.md]] for that specific design), could be a database,
   could be object storage. Pluggable the same way.

**Fixed — the operational surface:**

3. **A control plane** ([[control-plane.md]]) — how a concrete server
   gets registered, how a worker attaches, how a human observes any of
   it. Not a client-facing choice; every deployment needs exactly this
   shape of bootstrapping, whichever backends it plugs in underneath.
4. **A security model** ([[security.md]]) — how a worker proves it's
   allowed to use the wire at all. Not swappable per deployment either;
   the identity/token/rotation shape is the same regardless of which
   messaging or storage backend sits behind it.

The test for "does this belong in Robot Dreams" hasn't changed — it's
just two-tiered now. Something backend-specific and swappable belongs in
the messaging or storage primitive. Something universal and
non-negotiable about *running* or *trusting* the primitives belongs in
the control plane or security layer. Something agent- or
work-definition-specific belongs in neither — it belongs in whatever
plugs into Robot Dreams, not in Robot Dreams itself.

Everything in [[architecture.md]] (Temporal, GKE+Filestore, the
workspace lease/lock/revision service) and everything in [[memory.md]]
(git vs. Filestore/Postgres, the memory-tier mapping) is a **concrete,
opinionated instance** of these two primitives — worth building and
worth being good at, because "pluggable" still has to plug into
*something* real to be useful on day one. But it should never harden
into "the only way." The test for any future feature: does it belong in
the messaging primitive, does it belong in the storage primitive, or is
it actually agent/work-definition-specific — in which case it doesn't
belong in Robot Dreams at all, it belongs in whatever plugs into it.

## The spine embodies, it doesn't stand beside

A refinement to the spine metaphor, recorded here because earlier
drafts (and early website visuals) got it subtly wrong: the spine is
not a separate object that agents plug into — not a box drawn next to
the hierarchy with wires running over to it. Messaging and storage are
the enduring foundation of the ecosystem precisely because they
*permeate* it: **the core embodies the ecosystem, alive at every
level**. Every message between two nodes, every book deposited in or
taken from the library (both terms defined just below), at whatever
level of however much organizational structure a deployment chooses
([[organization.md]]), *is* the spine in operation. Any framing —
prose or picture — that reads as "your agents, and beside them, the
spine" should be sharpened until the ecosystem of nodes is the thing
being shown and the spine is what's visibly alive inside it. This
updates, rather than contradicts, the "spine, not the stack" section
above: the spine is still thin and pluggable; it just doesn't stand
anywhere. It's embodied.

## Nodes, the library, and the substrate

Three vocabulary decisions, made once here so the rest of the vision
docs can lean on them.

**Node.** A *node* is the entity that connects itself to the system
and does the communicating. Robot Dreams is 100% agnostic about what a
node's runtime is — a VM, a Kubernetes pod, a coding-agent session, a
plain script, a person. Small standard contracts govern how a node
interacts with the system; every implementation is free to extend and
adapt its own beyond them. Nodes can exist at *any* level of the
organizational hierarchy ([[organization.md]]), and the reporting
structure is defined **node-to-node** — edges between nodes, never
between containers — which is why a reporting edge may skip levels: a
node working down in one Site can report directly to a boss-node up in
a World. One mapping to state plainly, once: what the vision calls a
**node**, the Go reference implementation and the implementation-level
docs call a **worker** (`dream worker connect`, the worker registry,
worker credentials). Same entity, two registers — "node" is the
vision-level term, and the code and the top-level docs keep "worker";
nothing is being renamed.

**The library.** The canonical way to describe the storage primitive:
a central library that nodes reach into. One node deposits a book
(put); another — maybe days later, maybe at a different level of the
hierarchy — takes it out (get). This maps exactly to the existing
`completed_work` flow: messages carry storage pointers, never payloads.
The pointer is the catalog card; the book stays in the library. That
rule already holds throughout the docs and stays.

**The substrate.** The messaging engine is not a hub with a wire to
every message — that picture puts a box in the middle that doesn't
exist. It is the *substrate*: the medium that enables messages to flow
between nodes. Describe and depict the nodes and the messages moving
between them; the substrate is what makes the flow possible, not
another participant in it.

## Why this is one idea, not two

Communication and shared state are one problem, not two, because
neither is useful without the other:

- A message with nowhere to point (no shared state to reference, hand
  off, or build on) is just a chat log — ephemeral, and useless to the
  next agent that wasn't in the conversation.
- Shared state with no way to signal "this changed, here's what it
  means, here's what's next" is just a filesystem nobody's watching —
  agents would have to poll and guess.

Robot Dreams' bet is that the combination — one coordination layer, one
state layer, deliberately kept this thin — is what actually lets agents
work "anywhere and be anything" without every deployment reinventing
both from scratch.

## What "doing it right" looks like, concretely

1. **No lost work on crash.** Whatever executes the work dies mid-step
   — the messaging layer and the storage layer's revision/lease model
   mean nothing silently clobbers or loses state while it's down. (The
   Temporal + workspace-lease design in [[architecture.md]] is how this
   looks in the reference implementation — not the only way it could.)
2. **No lost context between agents.** One agent's work ends, another's
   begins — on a different machine, a different model, days later — and
   what it needs to continue is a deliberate, structured handoff written
   to shared state, not a raw transcript it has to re-triage. See
   [[memory.md]] for the concrete storage-tier design this implies.
3. **A defensible line between "durable" and "disposable."** The
   storage primitive has to make a real, opinionated choice about what
   persists long-term (reviewable, diffable, trusted later) versus
   what's working state — not leave it to accident. [[memory.md]]
   works this out for the git/Filestore instance specifically.
4. **Visible, not opaque.** Every message and every state change is
   something a human can watch happen — because the whole point of a
   thin, pluggable spine is that nothing important is hidden inside a
   vendor-specific black box.
5. **Actually pluggable, not pluggable-in-theory.** If swapping the
   messaging backend or the storage backend requires touching anything
   about how agents are defined or where they run, the abstraction has
   leaked and the spine has quietly become a stack.

## Where this sits relative to what exists

- Against **ticket queues**: a ticket queue conflates "how do I know
  this needs doing" (messaging) with "where does its state live"
  (storage) with "what tool executes it" (everything Robot Dreams
  deliberately stays out of). Robot Dreams keeps the first two thin and
  pluggable and refuses to own the third at all.
- Against **Hermes AI** ([[prior-art.md]]): Hermes bundles a specific
  agent runtime with a specific (flat-file) memory approach — it *is*
  an agent, with memory built in. Robot Dreams isn't an agent and isn't
  a memory product; it's the layer underneath, that a Hermes-like
  agent (or any other) could plug into for messaging and shared state
  without adopting its runtime.
- Against **Graphiti / Mem0** ([[prior-art.md]]): both are opinionated,
  vertically-integrated memory products — they decide the storage model
  *for* you. Robot Dreams' storage primitive doesn't compete with them
  so much as it's the layer they'd sit behind, if a deployment wanted
  vector/graph memory instead of the git/Filestore reference design.

## Bootstrapping and observing the spine ([[control-plane.md]])

The two primitives don't stand themselves up. `dream server` (CLI) and
Mission Control (UI) are two faces of the same job: register which
messaging backend and which storage backend a given deployment is
actually plugged into, let workers attach (`dream worker connect`), and
give a human a live view of what's happening. Neither tool has
privileged access the primitives themselves don't expose — they're
clients, which is what keeps "pluggable" honest end to end, not just at
the primitive layer.

One thing the control plane has to get right that the primitives alone
don't decide: connectivity is a graph, not a flat pool. Workers —
nodes, in the vocabulary above — can report to the server directly, or
to another node, forming a real organization chart — leads, reviewers,
aggregators — because a flat broadcast to everyone stops scaling almost
immediately and doesn't match how engineering orgs actually coordinate.
The edges of that chart run node-to-node, and may skip levels of any
organizational structure laid over them — see [[organization.md]] and
the reconciliation note in [[control-plane.md]].

Connecting a worker also means handing it credentials that reach both
primitives over the open internet, not just a trusted cluster network —
see [[security.md]] for the identity, key-rotation, and revocation model
that keeps a single worker credential from being a single point of
catastrophic failure.

## How this measures up against what exists today ([[prior-art.md]])

Looking at OpenClaw and Hermes Agent (Nous Research) as concrete
systems, not just prior art: neither actually separates messaging from
storage from execution as independent, swappable primitives — both are
single-agent products where all three are fused, and neither has a real
answer for concurrent multi-worker access to shared state. Both
independently converge on markdown/file-based memory, which is
validation of the storage bet in [[memory.md]]. See [[prior-art.md]] for
the full architecture research on both, and
[[implementation-alternatives.md]] for why the reference build is
shaped the way it is.

## The open scope question ([[hierarchy.md]])

Even "just messaging and storage" has a scope edge to defend: does
Robot Dreams represent anything about the Goal → Initiative → Workstream
→ Task hierarchy at all, or is that entirely something that plugs in on
top (a Workflow definition is just another thing that flows through the
storage/messaging primitives, indistinguishable in kind from any other
agent-authored state)? Given the "spine, not stack" framing above, the
honest answer leans toward the latter — but it's still worth stating
explicitly rather than letting Factories/Workflows quietly imply
otherwise.

Distinct from that work-item question is the **organizational**
hierarchy — Universe → Worlds → Realms → Sites ([[organization.md]]):
an optional way to organize nodes and chain reporting up and down,
not a way to break work into pieces. The two hierarchies answer
different questions and shouldn't be conflated; both docs
cross-reference each other for exactly that reason.
