# Perspective: Securing Messaging & Storage Across the Internet

> **This is the fixed trust layer, not a swappable primitive.** See
> [[core.md]]. Messaging and storage backends are pluggable; whatever
> backend a deployment plugs in, the identity and key model below is
> how a worker proves it's allowed to use it — the backend still owns
> its own transport and storage mechanics, but doesn't get to redefine
> how identity works.
>
> **Status**: the worker keypair, JWT mint/validate, JWKS, and
> revocation-store core of this design is being implemented now in the
> Go reference build (see "Build scope" below for exactly what's in vs.
> deferred for v0.1).

Workers, [[architecture.md]]'s remote and Kubernetes kinds alike, may
reach messaging and storage backends over the open internet, not just a
trusted cluster network. This is about what stops that from being a free
pass to anything the backend holds.

## One identity, not one shared secret

`dream worker connect` ([[control-plane.md]]) issues the worker a single
asymmetric keypair as its identity — not a password, not a static API
key shared across workers. That keypair is the "central key" the worker
holds, but it is never sent over the wire on every request. Instead:

- The keypair signs requests for short-lived **capability-scoped
  tokens** (JWT-style), minted by `dream server`.
- Each token is scoped narrowly — `message:send`, `message:receive`,
  `storage:read:path/*`, `storage:write:path/*` — not a blanket "this
  worker can do anything."
- Both the messaging backend and the storage backend validate tokens
  independently against `dream server`'s public key (JWKS-style). Neither
  backend has to trust or share secrets with the other — which is what
  keeps "pluggable" honest end to end: swapping the storage backend
  doesn't mean touching how messaging does auth, or vice versa.

This is the same shape as SPIFFE/SPIRE workload identity or Kubernetes
service-account tokens: one long-lived identity, many short-lived derived
credentials minted from it.

## Transport

- TLS in transit, always, for both messaging and storage traffic.
- mTLS where the endpoint itself should authenticate the worker at the
  connection layer, not just via the token riding inside it — belt and
  suspenders, since a token can be scoped but a connection either is or
  isn't from a holder of the private key.

## Storage content, specifically

Storage is the higher-value target: content itself crosses the wire and
often lands in a third-party-hosted backend (S3, managed Postgres, a
cloud Filestore). The path/metadata layer (who can read/write which
paths, via the workspace service in [[architecture.md]]) is covered by
the scoped-token model above, but the payload itself is worth going
further on — envelope-encrypting content client-side (age/ECIES-style,
keyed to the recipient worker's public key) before it ever reaches the
storage backend, so the backend only ever sees ciphertext. This still
matches core.md's "visible, not opaque" principle — an authorized human
or worker can decrypt; the storage backend operator cannot.

## Key lifecycle: rotation and recycling

A single identity opening both messaging and storage is high blast-radius
if it leaks. Rotation has to be **server-initiated, not client-initiated**
— a worker that's already compromised can't be trusted to rotate itself
honestly, and a legitimate worker shouldn't need to remember to.

1. **Short-lived tokens, auto-expiring.** The actual working credentials
   (the scoped JWTs above) carry a short TTL — minutes to an hour, not
   days. `dream server` re-mints them on a fixed cadence automatically,
   transparent to the agent logic (a background refresh in the client
   library, not something a worker's task code has to handle). This does
   most of the risk-reduction work by itself: a leaked token is only
   useful for a narrow window even if nobody notices the leak.
2. **Server-side revocation.** TTL expiry alone isn't enough for "we know
   this is compromised right now" — `dream server` needs a revoke path
   (`dream server revoke <worker-id>`) that immediately invalidates all
   outstanding tokens for an identity, independent of when they'd
   otherwise expire. This is the fast-response complement to the
   slow-but-automatic expiry in (1).
3. **Slower long-lived-key rotation.** The keypair itself (the thing
   issued at `dream worker connect`) should also rotate, but on a much
   slower, server-driven cadence (e.g. weekly) — the server pushes a
   re-key, the worker accepts it. A worker that goes unreachable for
   re-keying is itself a signal worth surfacing to Mission Control, not
   silently ignored.
4. **Scope narrowing on rotation.** Each re-mint is a natural checkpoint
   to shrink a token's scope to what the worker has actually used
   recently, rather than reflexively re-granting the same scope forever —
   self-tightening least-privilege instead of a one-time grant that only
   ever widens.
5. **Anomaly-triggered forced rotation.** Since Mission Control already
   watches live activity ([[control-plane.md]]), a token used in an
   unexpected pattern (new path, new channel, sudden volume) can trigger
   an immediate forced re-mint and a flag to a human, rather than waiting
   for the normal cadence.
6. **Per-token audit binding.** Every action a token authorizes should be
   attributable to the exact token / rotation-epoch that authorized it,
   so a post-hoc investigation can tell precisely which actions came from
   compromised material versus before/after — consistent with core.md's
   "visible, not opaque."

## Build scope for the reference implementation

The key-lifecycle list above is the full design; the Go build starts
narrower and grows into it. **Shipping now**: item 1 (short-lived
auto-refreshing tokens, transparent background refresh in the client
library) and item 2 (server-side revocation via `dream server revoke`).
Item 6 (per-token audit binding) is cheap enough to include alongside
them — every minted token gets a unique `jti`, logged on mint and
checked on revocation. **Deferred, not dropped**: item 3 (slow
keypair re-keying), item 4 (scope-narrowing on rotation), item 5
(anomaly-triggered forced rotation), and client-side payload
envelope-encryption for storage content. The interfaces are shaped to
leave room for these (a `Rotator` seam, an `Encryptor` hook point on
the storage client) so adding them later doesn't require a redesign —
but they are genuinely not implemented in the first build, and this doc
shouldn't be read as claiming otherwise.

## Open questions — resolved for the reference implementation

These were open when this doc was written. The Go build resolves them
as follows; noted as build decisions, not as the only legitimate
answers.

- **Where does the revocation list live?** Resolved: first-class,
  server-side state in the control plane's own datastore (the same
  SQLite/Postgres store the server already needs for other control-plane
  state), not routed through the swappable storage backend. Backends
  never own trust data — they only validate signatures/JWKS and,
  optionally, cache a revocation-status check. This directly answers the
  "compromised storage backend hiding its own revocation" concern: the
  storage backend was never the place revocation lived in the first
  place.
- **Opaque vs. self-describing token scope?** Resolved as a hybrid, not
  a pick-one: tokens are self-describing signed JWTs (JWKS-validated,
  offline-verifiable — the low-coupling, low-latency property of the
  "self-describing" option), **plus** backends perform a short-TTL
  (60-second) cached revocation-status check against the server. This
  bounds "how long can a revoked token still work" to the cache window
  instead of the full token TTL, without requiring a synchronous
  callback to the server on every single request.
- **Grace-period overlap for forced rotation** — still genuinely open;
  not addressed by the current build since forced/scheduled rotation
  (items 3 and 5 above) isn't implemented yet. Worth resolving before
  those items are built, not before.

See also: [[core.md]] for why messaging and storage are the only two
primitives Robot Dreams owns, [[control-plane.md]] for how a worker
attaches in the first place, [[architecture.md]] for what a connected
worker does once it has valid credentials.
