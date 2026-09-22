# Security model

This summarizes the identity/token design from
[`vision/security.md`](vision/security.md) as actually implemented in
this build, **and** states plainly what's simplified or not yet built.
Its job is to stop you from over-trusting the current state — read the
"Known limitations" section before relying on this in production.

## What's implemented

### Worker identity: Ed25519 keypair, not a shared secret

`dream worker connect` generates (or reuses) an Ed25519 keypair at
`~/.dream/<server-id>/identity.key`, written with mode `0600`
(`identity.KeyFilePerm`). `LoadKeyPair` refuses to load a key file whose
on-disk permissions are looser than `0600` rather than silently trusting
it. The private key never crosses the wire — the client proves possession
by signing a server-issued nonce (`SignNonce`/`VerifyNonceSignature`,
plain `ed25519.Sign`/`ed25519.Verify`), a classic challenge-response.

### Short-lived, capability-scoped JWTs

`internal/identity.Issuer.Mint` issues Ed25519-signed (`EdDSA`) JWTs via
`golang-jwt/jwt/v5`, with a `kid` header for JWKS key selection, an
expiry, and a unique `jti` per token (`newJTI`) for per-token audit
binding — this is docs/vision/security.md's key-lifecycle item 6, shipped
alongside items 1 and 2 below because it's cheap.

- Default token TTL is **15 minutes** (`server.DefaultTokenTTL`).
- Every connected worker's token carries `DefaultScopes =
  ["message:send", "message:receive"]` — enforced by `POST /api/messages`
  (send) and by `GET /api/messages` and `/api/messages/subscribe`
  (receive), with `admin` bypassing — plus **path-scoped storage
  capabilities**, computed per worker by `server.WorkerScopes`: read+write
  on its own `workers/<id>/*` prefix, and read+write on the shared
  `shared/*` namespace (`server.SharedStoragePrefix`). This replaces the
  uniform `storage:read:*`/`storage:write:*` placeholder a phase-11
  security-review pass flagged — see "Fixed in this pass" below for the
  full write-up. `admin` (`server.AdminScope`) still bypasses path
  scoping entirely and is still never carried by a connect-time token —
  only `Server.MintAdminToken`'s local bootstrap path grants it.
- `pkg/security.WorkerClient` refreshes transparently in the background:
  it schedules its next refresh at **70%** of the current token's TTL
  (`security.RefreshFraction = 0.7`), bounded below by a 10ms minimum
  interval (`MinRefreshInterval`) so a pathological zero/negative TTL
  can't spin a tight loop. Task/agent code never touches token logic
  directly.

### JWKS validation

`GET /api/.well-known/jwks.json` publishes the server's Ed25519 public
key so any backend — in-process or out-of-process — can validate tokens
independently, without a shared secret between backends.
`identity.ValidatePublicKey` is the primitive this builds on: it doesn't
require an `Issuer` at all, just the published public key(s).

### Server-side revocation

`dream server revoke <worker-id>` writes to a `RevocationStore`
(SQLite-backed in the server, `SQLiteRevocationStore`; an
`InMemoryRevocationStore` exists as a standalone/test seam). Revoking an
already-revoked worker is not an error. Validation combines signature/
expiry checks (`Issuer.Validate`) with a separate revocation lookup —
`identity.ErrTokenRevoked` is the sentinel both sides agree on.

### Challenge-response connect, HTTP API surface

`POST /api/workers/challenge` and `POST /api/workers` implement the
connect flow; `POST /api/token` re-mints a token given proof of key
possession. All worker-facing endpoints besides those three and the
health/JWKS endpoints require an authenticated bearer token
(`a.withAuth(...)` in `internal/server/api/router.go`). `POST
/api/workers` additionally requires an admin/enrollment credential — see
"Fixed in this pass" below.

`PATCH /api/workers/{id}` edits a registered worker — today its `role`
label — and `POST /api/workers/{id}/reassign` moves it under a new
parent. Both carry the same authorization rule beyond the bearer token:
the control plane accepts the call only from the worker itself, the
worker's current parent, or a caller holding the admin scope, and
answers 403 to anyone else. Relabelling a node is no more privileged
than moving one, so neither is admin-only.

### Delegated (ephemeral) child workers

A connected worker can mint a credential for a short-lived child of its
own — a subprocess it spawns to do one thing — without the child going
through connect at all: `POST /api/workers/delegate` (or `dream worker
delegate`), authenticated as the parent. The child gets a bearer token
and nothing else: no keypair, no org-chart entry, no refresh path. It is
the same trust model every request already uses (a short-lived JWT
signed by the control plane, validated against JWKS), applied at
join time for a child the parent vouches for. The rules, enforced in
`internal/server/delegate.go`:

- The child's ID is `<parent>~<child>`, and the token carries
  `delegated_by: <parent>`; `POST /api/workers` refuses any ID containing
  `~`, so a registered worker can never pose as a child or be shadowed
  by one. Every write the child makes is attributed to its own ID
  (`ObjectMeta.UpdatedBy`), which names the parent by construction.
- Scopes are a subset of the parent's: by default everything the parent
  holds except `admin`; explicitly, only scopes the parent holds outright
  or storage scopes narrower than one it holds (`storage:read:a/b/*`
  under `storage:read:a/*`). `admin` is never delegated, however it is
  asked for. A child minted without `message:send` or `message:receive`
  is refused at the message endpoints, so the narrowing holds.
- The token lasts at most `Config.DelegatedTokenTTL` (default five
  minutes), never past the parent's own token, and a child cannot
  delegate further. A child that needs more time asks its parent for a
  new token.
- Revoking the parent revokes every child at its next request, whatever
  its token has left (`withAuth` checks `delegated_by` against the
  revocation store as well as the subject).

What this deliberately does not do: give the child an identity of its
own. Nothing about the child survives its token, it appears in no
listing, and there is no per-child revocation — revoke the parent, or
wait. That is the trade for a join that costs one authenticated request.

## Fixed in this pass (phase-11 security-hardening follow-up)

The phase-11 security-review pass flagged two genuine gaps, both closed
here. This section describes the fix; the "Known limitations" section
below has been updated to drop these two items (they're no longer open)
and states plainly what's still simplified.

### Storage is now per-caller, per-path authorized

**Before:** `internal/server/api/storage.go`'s put/get/list/delete
handlers never checked `claims.Scopes` against the requested object path.
Combined with the old uniform `storage:read:*`/`storage:write:*`
placeholder scopes, any connected worker could read, overwrite, or delete
any object anywhere in the storage backend — `handleDeleteObject` checked
no scope whatsoever, the most acute part of the finding.

**After:** every worker's token now carries path-scoped storage
capabilities via `server.WorkerScopes(workerID)`:

- Read+write on its own `workers/<id>/*` prefix — the convention chosen
  because worker IDs are already the stable, org-chart-addressable key
  the rest of the system uses; no new identity scheme was introduced.
- Read+write on a shared, durable namespace, `shared/*`
  (`server.SharedStoragePrefix`).

All four storage HTTP handlers now call a shared `hasStorageScope(claims,
verb, path)` helper (`internal/server/api/auth.go`) before touching the
backend, verb being `"read"` for Get/List or `"write"` for Put/Delete. A
scope of the form `storage:<verb>:<prefix>*` grants access to every path
under `<prefix>`; List reuses the identical check against the requested
prefix itself, so listing a prefix broader than any granted prefix (e.g.
the empty "list everything" prefix) is refused for a non-admin caller
too. An `admin`-scoped token bypasses the check entirely, so
`Server.MintAdminToken`'s existing bootstrap path (`dream server revoke`,
Mission Control operator tokens) keeps unrestricted storage access.

**Design decision — why not org-chart-derived read grants for leads?**
`docs/vision/security.md`'s design and `messaging.html`'s model both rely on
`completed_work` carrying a storage pointer so a lead can retrieve a
report's output. Two designs were considered: (a) recompute a lead's
scopes to include each direct report's owned prefix, recalculated on
every reassignment, or (b) the shared namespace above, with no
org-chart-position-derived grants at all. (b) was chosen: it's simpler,
needs no scope recomputation when the org chart changes, and fully
satisfies the real handoff use case the e2e suite exercises — a report
writes its deliverable under `shared/...` and any recipient (its lead, a
peer it hands off to directly) can read it back. The tradeoff is that a
lead has no *automatic* read access to a report's private
`workers/<report-id>/*` scratch space; only what was explicitly placed
under `shared/`. This matches how `EmitFromWorker` already treats
`completed_work`/`request_for_input` recipients (an explicit registered
worker, or the parent by default) — nothing in the existing routing model
assumed implicit read access into a report's private storage either.

Because the object-path convention changed, `test/integration`'s mock
task harness (`e2e_local_test.go`'s `runSuccessPath`) now writes its
deliverable under `shared/work/<worker-id>/...` instead of a bare
`work/<worker-id>/...` path, so the lead's read-back of the
`completed_work` handoff still succeeds under the new scoping.

### Worker enrollment now requires an admin or enrollment credential

**Before:** `POST /api/workers/challenge` and `POST /api/workers` were
fully unauthenticated. The challenge-response only proved the caller held
the private key for a keypair it had just generated — never that it was
entitled to the worker ID it was claiming. A network-adjacent attacker
could race a legitimate worker to register a known ID first, and a
pre-emptively revoked worker ID could still complete registration
(`handleConnect` didn't consult the revocation store).

**After:** `POST /api/workers` requires an `Authorization: Bearer <...>`
header carrying either a valid **admin-scoped token** or the server's
**local enrollment secret**, checked by
`internal/server/api.(*API).requireEnrollmentAuth` before the challenge
is even verified. Proof of key possession remains necessary (the
signature check is unchanged and unweakened) but is no longer
sufficient. A worker ID that has already been revoked is now explicitly
rejected before registration completes, closing the squatting gap too —
checked via the same revocation cache `withAuth`/`handleToken` already
use, not a separate, weaker path.

**The local enrollment secret** (`Server.EnrollmentToken`) is a random
32-byte value, generated once on a control plane's first-ever startup and
persisted at `<data-dir>/enrollment.token` (mode `0600`, same discipline
as the signing key). `dream server init` prints its path in the startup
banner (the value only behind `dream server guide --show-token`), and
`dream server rotate-enrollment` replaces it — see "The enrollment token
can be rotated" below. This is the credential the zero-config quickstart needs to keep
working without a manual credential-copying step: `dream worker connect`
auto-discovers it (`discoverLocalEnrollmentToken` in
`cmd/dream/cmd_worker.go`) **only** when `--server` resolves to a
loopback address (`127.0.0.1`, `::1`, `localhost`) AND the server is
running at the CLI's default data directory (`~/.dream/_server`) — i.e.
only when this exact process already has filesystem access to read the
file directly. A remote worker, or a worker connecting to a local server
started with a custom `--data-dir`, must pass `--admin-token` explicitly
(accepting either an admin JWT or the raw enrollment secret). This is the
actual security property the fix buys: **a caller without filesystem
access to the server's data directory gets nothing from auto-discovery**
and cannot register a worker without an operator handing it a credential
out of band — closing the "reachable by anyone who can reach the API"
gap while keeping the local, single-machine quickstart flag-free.

**Configurable escape hatch:** `Config.OpenEnrollment` (the `dream server
init --open-enrollment` flag) disables this check entirely, reverting to
the pre-fix "proof of key possession is sufficient" behavior. This is an
explicit, off-by-default opt-out for local dev/test setups that have a
reason not to use the enrollment-token file (some of this repo's own CLI
e2e tests use it, since they're exercising unrelated command plumbing,
not the enrollment boundary itself) — **it must not be set on any
control plane reachable from an untrusted network**, since it reopens
exactly the gap this section closes.

### Message scopes are enforced; a report's producer is the caller

**Before:** `message:send` and `message:receive` were minted into every
token and narrowed for delegated children, but no message handler
checked them — only schedule registration did. A child delegated with
neither scope could still send and read. Separately,
`POST /api/reports/.../instances` defaulted `producer` only when the body
left it empty, so any worker could attribute a report instance to
another worker.

**After:** `handleEmitMessage` requires `message:send`, and
`handleSubscribe` / `handleTailMessages` require `message:receive`, with
`admin` bypassing — the same rule `schedules.go` already applied. The
check runs before decoding and routing, so a narrowed child gets 403
rather than a 400 from the org-chart lookup. A report instance's
`producer` is the authenticated caller unless the caller is admin, the
same way `from` on emit and `by_worker` on ack are owned by the server.

### Tokens must carry an expiry; worker IDs are hostname-shaped; the challenge store is bounded

**Before:** `identity.ValidatePublicKey` parsed with signature and
expiry checks, but `golang-jwt` only checks `exp` when the claim is
present — a signed token that simply omitted it would have validated
forever. Registration validated a worker ID only for "non-empty" and
"no `~`", so `/` was allowed: `server.WorkerScopes` grants worker `a`
the storage prefix `workers/a/*`, which also covers a worker registered
as `a/b` (and vice versa for `a/b/c`), letting two workers read and
write each other's objects. The unauthenticated
`POST /api/workers/challenge` endpoint accepted any ID length and kept
every pending nonce for two minutes with no cap on how many.

**After:** the parser runs with `jwt.WithExpirationRequired()`; every
token this build mints sets `exp`, so nothing legitimate changes, and a
token without one is refused (`TestValidateRejectsTokenWithoutExpiry`).
A registered worker ID must match `server.ValidateWorkerID` — start with
a letter or digit, then only letters, digits, `.`, `-` and `_`, at most
`server.MaxWorkerIDLen` (128) bytes — the same pattern as a delegated
child's name (`delegate.go`'s `childNameRe`), and everything a hostname
(the CLI's default `--worker-id`) can contain. `/`, whitespace, control
characters and `~` are all refused with a 400, both in `handleConnect`
and in `Server.ConnectWorker` for in-process callers. The challenge
endpoint refuses over-long IDs before storing anything, and the
challenge store caps outstanding nonces at `maxPendingChallenges`
(10,000), evicting the entries nearest expiry first — a flood can at
worst make a slow legitimate client ask for a fresh nonce, never grow
memory.

### The enrollment token can be rotated, and is not printed by default

**Before:** the enrollment secret was generated once, loaded into memory
at startup, and never changed: no rotation command, and a leak meant
regenerating the file *and* restarting the server. `dream server guide`
printed the value to stdout whenever the file was readable.

**After:** `dream server rotate-enrollment [--data-dir …]` replaces the
secret on disk (`server.RotateEnrollmentToken`, written via a temporary
file renamed into place, mode `0600`). The running server no longer
caches the value: `Server.EnrollmentToken` reads the file on every
enrollment attempt, so the old secret is refused from the moment the
file changes and no restart is needed; tokens already minted for
connected workers are unaffected. If the file is missing or empty, no
enrollment secret is accepted at all (an admin token still works) —
fail closed, never fall back to a value loaded at startup. The startup
banner and `dream server guide` print the token's **path**; the value
is shown only with `--show-token` (on `guide` and `rotate-enrollment`).
Every consumer already reads the file (`DREAM_TOKEN=$(cat <path>)`), so
nothing in the documented flows changes.

### TLS can be served directly; plaintext on a reachable interface is refused

**Before:** `dream server init` served plain HTTP only, defaulting to
`:7420` on every interface, with the documentation asking operators to
put a TLS proxy in front. Bearer tokens travel in every request, so a
default install on a LAN handed them to anyone on the path.

**After:** `--tls-cert <cert.pem> --tls-key <key.pem>` serve HTTPS
directly (`http.Server.ServeTLS` with `MinVersion: TLS 1.2`; both files
are checked at startup so a typo fails before the banner). `--addr` now
defaults to `127.0.0.1:7420`, so the zero-config quickstart is unchanged
and plaintext but reachable from this machine only. Any non-loopback
`--addr` (`:7420`, `0.0.0.0:…`, a LAN IP, a hostname other than
`localhost`) without TLS is refused with a message naming the three
ways out: serve TLS, pass `--insecure` (behind a TLS-terminating proxy
or on a network you trust — the hop to the proxy is plaintext by
design), or bind loopback. `listenAddrIsLoopback` in
`cmd/dream/cmd_server.go` is the classification: `localhost`,
`127.0.0.0/8` and `::1` count; an empty host, `0.0.0.0`, `::` and any
other name or address do not (a name that happens to resolve to
loopback today is not a security property of the address). `dream
simulate` has its own in-process server and is not affected.

## Known limitations — read before relying on this in production

These are **accepted, documented simplifications** for this build, not
oversights the security-review pass is expected to silently "fix" — each
one has an explicit comment in the code acknowledging it. The findings
the security-review passes flagged so far (storage authorization, worker
enrollment, message scopes, expiry-less tokens, worker-ID shape, the
challenge store, enrollment-token rotation, direct TLS) are fixed above,
not simplifications still open.

- **Delegated children are only as visible as their parent's token.**
  A child holds a bearer token with no key behind it; whoever has the
  token is the child. Keep delegated tokens short (they default to five
  minutes and are capped by the parent's own expiry), hand them to the
  child process directly rather than through anything shared, and
  revoke the parent if one leaks. There is no per-child revocation.
- **The org chart is fully visible to every connected worker, not just
  admins.** `GET /api/workers` and `GET /api/workers/{id}` require a
  valid token but perform no further authorization — any connected
  worker can list every worker's ID, role, `reports_to` edge, metadata,
  and Ed25519 public key (the last of which is meant to be public
  anyway). This is a narrower, lower-severity version of the storage gap
  above, and is consistent with the org chart being collaborative,
  shared routing state rather than a partitioned per-worker view — but
  if your `metadata` fields carry anything sensitive, know that every
  worker can read every other worker's metadata.

- **TLS is direct-or-nothing; there is no mTLS, ACME, or reload.**
  `--tls-cert`/`--tls-key` serve HTTPS from a static PEM pair; the
  certificate is loaded once at startup (restart to renew), client
  certificates are not requested (workers authenticate with bearer
  tokens, not mTLS — `docs/vision/security.md`'s optional transport-layer
  mTLS is still unbuilt), and there is no built-in ACME/Let's Encrypt.
  `--insecure` deliberately re-opens plaintext on a reachable interface
  for deployments behind a TLS-terminating proxy; the hop between proxy
  and server is then only as private as that network. The refusal keys
  on the *address*, not on what is actually reachable: a loopback bind
  plus a port-forward or an SSH tunnel is your responsibility.

- **The enrollment token is one shared secret, rotated by hand.**
  `dream server rotate-enrollment` replaces it and the old value dies at
  once, but there is still a single secret for every enrollment (no
  per-worker or single-use enrollment credentials, no expiry, no audit
  of which enrollment used it), and rotation is an operator action, not
  a schedule. Whoever can write the data directory can rotate it — the
  same filesystem-access trust boundary as `dream server revoke` below.
  A rotated-out secret is not remembered, so an enrollment attempt with
  it is indistinguishable in the logs from a random wrong token.

- **`dream server revoke`'s admin auth is a filesystem-access bootstrap,
  not a real RBAC model.** Nothing mints an admin token over HTTP —
  `Server.MintAdminToken` is a local-only path. The `revoke` command
  opens its own short-lived `*server.Server` against `--data-dir` purely
  to mint an admin token locally, then makes an ordinary authenticated
  HTTP call to the already-running server. This works, and is
  intentionally scoped as "safe", *because* whoever can do this already
  has filesystem access to the server's data directory — and could read
  the signing key directly regardless. It is not a substitute for a real
  administrator identity: a proper admin-identity and admin-token
  issuance flow is a documented gap, and must exist before this command
  is safe across a trust boundary. Don't expose `--data-dir` access to
  anyone you wouldn't also trust with full admin control.

- **The dashboard's auth is a token-paste flow, not a browser-side
  keypair.** Mission Control has no server-side state or privileged side
  channel of its own — it's a pure client of the same `/api/*` endpoints
  a worker or the CLI uses. The operator runs `dream worker connect` (or
  mints an admin token) elsewhere and pastes the resulting bearer token
  into the dashboard once; it's stored in the browser's `localStorage`
  and attached as `Authorization: Bearer <token>`. A token minted without
  the admin scope can view its own worker but nothing broader. This is a
  deliberate simplification for this phase, not a real browser-based
  worker identity/keypair flow.

- **Storage scoping is by worker ID and a shared namespace, not a
  general per-role/per-path capability policy.** `server.WorkerScopes`
  (see "Fixed in this pass" above) grants every worker the same *shape*
  of scope — its own `workers/<id>/*` prefix plus `shared/*` — rather
  than a richer, administrator-defined policy keyed by role or an
  arbitrary path hierarchy. There's still no way to grant, say, a
  `payments-lead` role read access to a `payments/*` prefix distinct from
  its own worker ID, or to narrow a specific worker's grant below the
  standard shape. That's a real per-role/per-path capability policy
  source, which `docs/vision/security.md`'s `storage:read:path/*` framing
  gestures at but does not fully specify — a documented gap, not a
  silent one.

- **The following `docs/vision/security.md` key-lifecycle items are explicitly
  NOT implemented** (deferred, not silently dropped — the code leaves
  interface seams for them, e.g. a `Rotator` seam and an `Encryptor` hook
  point mentioned in the vision doc, but nothing wired up yet):
  - Item 3: slow, server-driven long-lived keypair re-keying (e.g.
    weekly rotation of the identity keypair itself).
  - Item 4: scope-narrowing on rotation (each re-mint shrinking a token's
    scope to what's actually been used).
  - Item 5: anomaly-triggered forced rotation (Mission Control flagging
    unusual token usage and forcing a re-mint).
  - Client-side payload envelope-encryption for storage content (so the
    storage backend operator only ever sees ciphertext) — storage content
    is not encrypted client-side in this build; it is only as protected
    as the storage backend's own access controls and (if you add it)
    transport TLS.

If you need any of the above for your deployment, treat this as a
starting point to extend, not a finished security boundary.
