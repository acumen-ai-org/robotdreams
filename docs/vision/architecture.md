# Perspective: Infrastructure & Execution Architecture

> **This is one reference implementation, not the definition.** Robot
> Dreams' actual scope is just a messaging primitive and a storage
> primitive — see [[core.md]]. Everything below is one concrete,
> opinionated way to build the *storage* side (and the coordination
> that rides on top of it); a deployment is free to plug in something
> else entirely.
>
> **What the Go reference build actually ships first is narrower than
> this doc.** The initial storage backends are a simpler two-way blob
> abstraction — local filesystem and S3-compatible — with optimistic
> concurrency (a rejected write on stale revision) instead of the
> pessimistic acquire/lease model below, and no git integration. That's
> a deliberate, scoped choice for v0.1, not a reversal of this design:
> the git+Filestore+Postgres+lease-service architecture described here
> remains a legitimate, and likely eventual, pluggable storage backend
> — it's just not what ships first. See [[memory.md]] for the same
> caveat applied to the memory-tier mapping.

Raw shape of the system that runs a Workflow inside a Factory.

## Components

- **Temporal** — manages Workflows and task execution. Owns durability,
  retries, and the record of what happened, in what order.
- **GKE + Filestore** — a central shared filesystem all agents can see.
  This is the default working surface: agents read and write files in
  place, no local checkout required for the common case.
- **Kubernetes workers** — run inside the cluster and operate directly on
  the shared Filestore volume. Cheapest, fastest path — no
  download/upload round trip.
- **Remote workers** — run outside the cluster (a developer's machine, a
  CI runner, anything not co-located with Filestore) and occasionally
  need to check out a workspace locally to do their work, then sync back.
- **Workspace service** — the arbiter for concurrent access to shared
  state: leases, locks, revisions, and syncing between local checkouts
  and the shared filesystem.
- **PostgreSQL** — stores workspace and lease metadata (who holds what,
  since when, at what revision) — the source of truth the workspace
  service reads/writes, separate from the file content itself.

## Remote workflow lifecycle

For a worker that isn't co-located with Filestore:

```
acquire → download → work locally → sync → commit → release
```

- **acquire** — take a lease on the workspace (or the relevant slice of
  it) via the workspace service.
- **download** — pull the current revision to a local checkout.
- **work locally** — the agent does its work against local files.
- **sync** — reconcile local changes back toward the shared state.
- **commit** — make the result durable/visible (this is the moment the
  new revision exists for others to see).
- **release** — drop the lease so another worker can acquire it.

## File revisions

Every write produces a revision. Revisions exist specifically to prevent
a stale worker (one that checked out an old revision and worked on it for
a while) from blindly overwriting newer work that landed while it was
running. Conflict handling is a first-class concern of the workspace
service, not an afterthought bolted onto file syncing.

## Open questions / things to resolve

- Conflict resolution policy when two workers' syncs collide on
  overlapping paths (reject and force a re-sync? merge? last-writer-wins
  per-path?).
- Whether Kubernetes workers ever need the same lease/lock discipline as
  remote workers, or whether shared-filesystem co-location is assumed
  safe by construction (it isn't, if two K8s workers touch the same
  path concurrently).
- Relationship between a Temporal Workflow's execution history and the
  file revision history — are they the same timeline, or two logs that
  need to be correlated after the fact?

See also: [[memory.md]] for what actually gets stored on this
infrastructure, and [[core.md]] for why this architecture exists at all.
