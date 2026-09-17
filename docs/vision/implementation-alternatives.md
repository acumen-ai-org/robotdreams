# Perspective: Explaining the Reference Build

> Companion to [[core.md]]. This doc is the source for the website's
> Explanation page — not a comparison anymore, just the reference build
> laid out in full: what it is, why it's shaped this way, and what the
> trade-offs actually are.
>
> Earlier drafts of this page compared the reference build against
> OpenClaw, Hermes Agent, and a roll-your-own baseline. That comparison
> framing has been dropped from the site — the page now focuses purely
> on explaining the one concept. The research is preserved: OpenClaw and
> Hermes Agent's architecture lives in full in [[prior-art.md]]; the
> roll-your-own rationale is folded into "why it's built this way" below,
> since it's the clearest way to explain what the discipline actually
> buys.

## Robot Dreams reference build

The full design is Temporal + GKE/Filestore + Postgres + git, as
sketched in [[architecture.md]] and [[memory.md]]. **The Go build in
active development at github.com/acumen-ai-org/robotdreams (Apache-2.0)
ships a narrower v0.1 first** — see the per-doc "what ships first"
callouts in [[architecture.md]], [[memory.md]], [[control-plane.md]],
and [[security.md]] for exactly where the two diverge and why.

- **Execution**: agnostic — any worker (K8s pod, laptop, CI runner)
  attaches via `dream worker connect` and executes whatever it's given.
- **Messaging**: three real, pluggable backends in the build — an
  embedded queue (zero-config default, SQLite/Postgres-backed),
  webhooks, and Temporal signals/workflow execution history.
- **Storage**: v0.1 ships a simpler two-backend blob abstraction — local
  filesystem (zero-config default) and S3-compatible — with optimistic-
  concurrency revisions; the full git + Filestore/Postgres + lease
  design remains a legitimate future pluggable backend, not yet built.
- **Control plane**: `dream server` (CLI) registers backends; Mission
  Control (UI) is the same config plus a live dashboard; `dream worker
  connect` attaches workers into a real organization chart, and `dream
  worker reassign` moves one explicitly and audibly.
- **Deployment**: distributed by design — any worker attaches the same
  way regardless of where it runs; messaging and storage backends are
  chosen independently per deployment.
- **Maturity**: in active development, not a shipped 1.0 — the design
  is settled and approved; the implementation is landing incrementally,
  phase by phase, each phase tested before the next starts. Check the
  repo for current status rather than trusting a snapshot here.
- **Strengths**: purpose-built for concurrent multi-worker access;
  explicit conflict handling (revisions) instead of an assumed absence
  of conflicts; every backend is a genuine, conformance-tested
  implementation behind one interface, not a sketch; the security model
  (asymmetric identity, scoped short-lived tokens, server-side
  revocation) is built in from the start.
- **Trade-offs**: early — not yet feature-complete, so still partly a
  bet rather than a full track record; Temporal remains a real
  operational dependency for teams that choose it (its own cluster to
  run and reason about); more moving parts than a single self-hosted
  process, by design.

## Why it's built this way

A queue and a database are cheap to stand up on their own — anyone
could wire messaging and storage together in an afternoon with no
framework at all. What the reference build actually buys, on top of
those cheap primitives, is the discipline around them:

- **Explicit conflict handling** ([[architecture.md]]'s lease/revision
  model) instead of an assumed absence of conflicts. Without it, every
  team reinvents this from scratch the first time two workers race on
  the same path — or worse, doesn't notice until data is silently lost.
- **Durable execution semantics** instead of hand-rolled retry logic.
  Without it, a crash mid-task means starting over, and "did this
  actually finish" becomes a question someone has to answer by hand.
- **Organization-chart routing** ([[control-plane.md]]) instead of every
  team inventing its own. Without it, messaging either becomes a flat
  broadcast that stops scaling almost immediately, or a bespoke routing
  scheme nobody else can reason about.

That discipline — not the pieces themselves — is the actual product.
And it's not a hypothetical failure mode: OpenClaw and Hermes Agent are
real, working systems where execution, messaging, and storage all fused
into one inseparable product, precisely because nothing forced the
separation to hold (see [[prior-art.md]] for the full architecture
research on both).

See also: [[core.md]] for why messaging + storage are the only two
things Robot Dreams commits to, [[prior-art.md]] for the OpenClaw/Hermes
architecture research and the memory-system research (Graphiti/Mem0/
seven-tier taxonomy) this pairs with.
