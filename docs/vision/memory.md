# Perspective: What Gets Stored (Agent Memory)

> **This is one reference implementation, not the definition.** The
> storage primitive in [[core.md]] just promises "agents can share
> state." Git + Filestore/Postgres is one opinionated way to build
> that; the design below should be read as a proof that the primitive
> is buildable and good, not as a constraint on what it has to be.
>
> **The Go reference build ships a simpler two-backend abstraction
> first** (local filesystem + S3-compatible, see [[architecture.md]]),
> not the full git-vs-Filestore/Postgres split described here. The
> four content categories below (short-term, long-term, outcome,
> process/handoff) are still the right conceptual model and are
> represented as advisory metadata on stored objects in the build; the
> git-specific tiering advice (map to the seven-systems taxonomy, git as
> procedural/episodic-summary memory) is design guidance for whoever
> builds or plugs in that richer backend later, not something v0.1
> implements.

## The core distinction

What lives on GKE + Filestore (and PostgreSQL, and git) is **not just**
Robot Dreams' own bookkeeping — Workflow definitions, task state, lease
metadata, "the contracts data." It is also, deliberately, **whatever
structure an agent itself wants to store**:

- Short-term data — scratch state for the current run, not meant to
  outlive it.
- Long-term data — meant to persist across runs, Workflows, or even
  Factories.
- Data about the **outcome** — the actual deliverable, result, artifact.
- Data about the **process** — how the agent got there: decisions made,
  approaches rejected, assumptions, partial attempts, why the final
  approach was chosen over alternatives.
- Data written specifically **for another agent to read later** — a
  deliberate handoff, not just a byproduct of the first agent's own
  bookkeeping. This is the mechanism by which one agent continues
  another's work without replaying the whole history.

This is the difference between a system that stores "what Robot Dreams
needs to orchestrate" and a system that gives agents a real place to
think in — one that survives past a single Workflow run and is legible
to the next agent that picks the thread back up.

## Where it lives

- **Git**, when the data should live long-term, be version-controlled,
  diffable, reviewable by a human, and survive independently of the
  Robot Dreams infrastructure itself. This is the default for anything
  that looks like a durable artifact: code, docs, decisions, specs.
- **Elsewhere** (Filestore-backed workspace / Postgres-backed metadata),
  when the data shouldn't live in git — because it's too ephemeral, too
  large/binary, too frequently mutated, or because there is no git
  repository in scope for this work at all.

The system needs to make this a real choice the agent (or the platform,
on the agent's behalf) makes deliberately — not an accident of "wherever
the last write happened to land."

## Research: how others frame agent memory

Full detail in [[prior-art.md]]. Summary and what it implies for Robot
Dreams' design specifically:

1. **Map storage tiers to the seven-systems taxonomy explicitly, don't
   blur them.** Filestore (shared FS) + Postgres workspace-lease data =
   working/session memory and durable workflow state (tiers 1, 2, 7 —
   see [[prior-art.md]]). Git = semantic + procedural memory: anything
   meant to be re-read, reasoned about, or trusted later (distilled
   facts, decisions, reusable playbooks). Ephemeral scratch data should
   not leak into git, and a Filestore JSON blob should not be treated
   as if it were a queryable semantic store — it isn't one.
2. **Avoid Mem0's split-store drift by design.** If a vector/graph
   index is ever added on top of git or Postgres content for search,
   the git commit (or Postgres row) must stay the single source of
   truth and the index must be a disposable, rebuildable cache — never
   let two stores independently hold "the fact." This is the direct
   lesson from Mem0's context-blindness failure.
3. **Use git as procedural + episodic-summary memory, not a raw
   episodic log.** Git's commit history is naturally suited to durable,
   human-reviewable, audit-trailed "how we did this" (procedural) and
   "what changed and why" (distilled episodic) — matching patterns like
   Git-Context-Controller and Letta Context Repositories, where
   commit/branch/merge operations directly improved agent task
   performance. Raw high-volume episodic data (tool calls, intermediate
   steps) belongs in Postgres/Filestore, not git — commits should be
   curated handoff artifacts, not exhaust.
4. **Design the cross-agent handoff as a structured document, not a
   transcript dump.** A workspace-lease-owning agent should write a
   bounded handoff artifact — result-so-far, reasoning, open
   constraints, pointers to relevant git commits/files — rather than
   exposing its full session history to the next agent. This is what
   keeps the next agent's context window from suffering the same
   top-K-crowding problem Mem0 exhibits.
5. **Reserve relational/graph structure for genuinely relational data.**
   Only add graph-like modeling where temporal fact evolution or entity
   relationships actually matter for reasoning (e.g. cross-workflow
   dependencies, agent/task lineage) — not as a default, given the cost
   and latency penalty shown in the Graphiti vs Mem0 research.

## Open questions

- Does an agent choose "this is long-term / for another agent" itself,
  or does the platform infer/enforce it (e.g. anything committed to git
  is long-term by definition; anything left in the workspace is
  short-term by default and gets swept)?
- What's the retrieval story for the *elsewhere* store — is it just
  "look in the workspace at a known path," or does it need something
  more like the memory systems in [[prior-art.md]] (semantic search,
  graph traversal) once volume grows past what a directory listing can
  usefully surface?
- How does "for another agent to read" interact with the lease/revision
  model in [[architecture.md]] — is a handoff note itself a file under
  revision control, with the same staleness risk as any other file?

See also: [[architecture.md]] for the infrastructure this lives on,
[[core.md]] for why this matters.
