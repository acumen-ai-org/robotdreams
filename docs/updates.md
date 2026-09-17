# Updates

Implementation-level guide to the update contract: the message the
control plane broadcasts, what a node says back, the HTTP surface, and
`dream self-update`. Design rationale lives in
[vision/updates.md](vision/updates.md).

**What Robot Dreams does here is narrow, on purpose.** It broadcasts
"a version of *this kind* is available" to every node and records what
each node says back. It does **not**:

- download, install, unpack or run anything for a node;
- restart, pause, drain or supervise a node;
- compare two version strings, or decide whether an update applies;
- choose which nodes an announcement is for;
- enforce any part of the update procedure.

All of that is the node's, because Robot Dreams is agnostic about what
a node's runtime is. The one thing it does implement is updating the
`dream` binary itself — the one runtime it owns — and that is an
ordinary consumer of the same contract.

## Pieces

- **`pkg/updates`** — the contract: the announcement and report JSON
  shapes plus their validation. Pure schema, no transport, no HTTP.
  Data and validation only, permanently; the package doc line says so
  and means it.
- **`internal/server/updates.go`** — the broadcast, the report
  recording, and the rollout join.
- **`internal/server/store/updates.go`** — the `update_announcements`
  log and the per-`(worker, kind)` `worker_update_state` row.
- **`internal/server/api/updates.go`** — the five HTTP routes.
- **`internal/selfupdate`** — `dream` replacing its own binary. Not a
  general update runtime, and not what a node uses for other kinds.
- **`cmd/dream/cmd_updates.go`, `cmd_selfupdate.go`** — the CLI.

## The announcement

Broadcast to every registered node as an **ordinary
`status_update` message** from `server`, with subject
`update available`:

```json
{
  "announcement_id": "4b0f2c9d1e7a4f38b6c2d5e1a9078f34",
  "kind": "robotdreams/cli",
  "version": "0.4.2",
  "source": "https://github.com/acumen-ai-org/robotdreams/releases/tag/v0.4.2",
  "min_version": "0.3.0",
  "severity": "recommended",
  "notes": "Finish in-flight work, then update and restart.",
  "announced_by": "ops-console",
  "announced_at": "2026-09-01T10:00:00Z"
}
```

It is deliberately **not** a fifth message type. A new entry in the
`status_update` / `completed_work` / `escalation` /
`request_for_input` taxonomy would force every existing consumer — both
messaging backends' conformance suites, the SSE hub, the dashboard,
every node's inbox loop — to learn it for no gain. This follows the
same reasoning as the reassignment notice in `Server.ReassignWorker`.

| Field | Meaning |
| --- | --- |
| `announcement_id` | Server-assigned. Quote it when reporting back — it is what joins a report to a rollout. The same version may legitimately be re-announced, so `version` cannot serve as that key. |
| `kind` | `<namespace>/<name>`, lowercase, `[a-z0-9._-]` each side. The whole routing mechanism: nodes filter on it. |
| `version` | **Opaque.** Never parsed, ordered or compared by Robot Dreams. |
| `source` | Free-form locator. Never fetched by Robot Dreams. |
| `min_version` | Advisory. No server code path reads it. |
| `severity` | `optional` / `recommended` / `required`, or absent. Advisory. |
| `notes` | Free text — where a deployment puts its own guidance. |

`robotdreams/cli` is the reserved kind for the `dream` binary. The
whole `robotdreams/` namespace is closed: announcing
`robotdreams/anything-else` is rejected, so a deployment cannot squat
it and later collide with a real one.

**Deliberately absent:** no recipient selector (delivery is a
broadcast), no staging or canary fields (the control plane announces,
it does not orchestrate), no deadline (one the server cannot enforce
would be a lie), and no checksum (that belongs to whatever `source`
points at).

## The report

Posted to `POST /api/updates/reports`, always about the authenticated
caller — there is no `worker_id` field, and supplying one is a 400.

```json
{
  "kind": "acme/prompt-pack",
  "announcement_id": "4b0f2c9d1e7a4f38b6c2d5e1a9078f34",
  "status": "in_progress",
  "current_version": "3",
  "target_version": "4",
  "detail": "draining; 2 tasks in flight"
}
```

| Status | Meaning |
| --- | --- |
| `current` | Just declaring a version. No announcement involved; `announcement_id` is empty. |
| `acknowledged` | Seen it, it applies, not started. |
| `in_progress` | Applying now. |
| `applied` | Done. `current_version` should now be the announced version. |
| `declined` | Seen it, deliberately not applying. A legitimate answer, recorded as such. |
| `failed` | Tried, did not work. |

`unknown` is **synthesized by the server** for a node that has said
nothing, and is rejected on the wire — a node cannot assert its own
silence.

Only `applied` and `declined` settle an announcement. `acknowledged`,
`in_progress` and `failed` leave it pending, because none of them says
the matter is closed.

Nothing here is kind-specific. `current_version` and `target_version`
are opaque strings, so a deployment announcing `acme/prompt-pack` gets
identical rollout tracking to `robotdreams/cli` with no server changes.

### Why the report is HTTP and not a message

This is the one asymmetry in the design, and it is deliberate:

1. `Server.EmitFromWorker` routes a `status_update` **always to the
   sender's parent**, ignoring any `to` the client supplies. A rollout
   report sent as a message would go to the wrong party.
2. `server` is not a registered worker and has no inbox to reply into.
3. Rollout status is queryable control-plane state, not a stream —
   the same shape as report instances, which are also POSTed over HTTP
   rather than sent through the substrate.

So the announcement goes **out** over messaging (durable, survives an
offline node) and the report comes **back** over HTTP.

## HTTP surface

| Route | Auth | Notes |
| --- | --- | --- |
| `POST /api/updates` | **admin** | Announce. 202 with `{announcement, recipients, delivered}`. |
| `GET /api/updates` | any worker | Recent announcements, newest first. |
| `GET /api/updates/pending` | own, or admin for another | Announcements this node has not settled. |
| `POST /api/updates/reports` | any worker | Always about the caller. |
| `GET /api/updates/rollout` | **admin** | Fleet-wide state for one kind. |

Announcing is admin-gated because it puts a message in every node's
inbox — the same blast radius as revocation. Reading announcements is
**not** gated beyond authentication: they were broadcast into every
node's inbox anyway, so a stricter pull endpoint would protect nothing
while breaking a node's catch-up path after downtime. The rollout view
*is* admin-gated, because it is cross-worker visibility into the whole
org chart.

## The rollout view

`UpdateRollout` enumerates the org graph and left-joins the stored
state, rather than listing stored state and stopping. A node that
received an announcement and said nothing appears as `unknown`.

That is the point of the view. The operator's real question is "is the
fleet on 0.4.2?", and its most useful part is which nodes have not
answered — exactly the rows a query over the state table alone would
omit. Every row also carries the node's connectivity, so "not
answering because it is down" is distinguishable from "not answering
because it is ignoring me."

```
$ dream updates rollout --kind robotdreams/cli
kind robotdreams/cli, announcement 4b0f2c9d… (0.4.2, announced 2026-09-01T10:00:00Z)
  applied 2  failed 1  unknown 1

NODE    CONNECTIVITY  MY VERSION  UPDATE STATUS  REPORTED              DETAIL
web-01  connected     0.4.2       applied        2026-09-01T10:04:11Z  -
web-02  connected     0.4.2       applied        2026-09-01T10:04:44Z  -
web-03  connected     0.4.1       failed         2026-09-01T10:03:02Z  npm EACCES
web-04  disconnected  0.4.1       unknown        -                     -
```

## Version reporting

A node declares versions through the **same** report endpoint, with
`status: "current"` and no announcement. One mechanism rather than two:
"what version are you on" and "how is your update going" are the same
statement about the same `(node, kind)` pair, and two endpoints would
mean two places to look for "what is web-02 running" that disagree the
first time one is missed.

`dream worker connect` declares versions automatically as a
**follow-up call** after registration — always `robotdreams/cli`, plus
any `--version-of kind=version` pairs:

```sh
dream worker connect --version-of acme/prompt-pack=3
```

It is a separate call rather than a field on the connect request
because `decodeJSON` sets `DisallowUnknownFields`: adding `versions` to
the connect body would make every newer CLI fail with a 400 against
every older server, on the very first call it makes. A follow-up call
is skew-safe in both directions — an older server simply 404s it — and
a failure is warned about rather than failing the connect, because a
node that cannot report its version is still a working node.

**Known gap:** a node that never reports again shows a stale version
indefinitely. The tempting fix is to ride along on `POST /api/token`,
which every live node already calls every few minutes — but that is an
unauthenticated route whose body is signed-nonce credential material,
and adding a mutable-state side effect to credential issuance deserves
its own security review rather than a ride-along.

## Storage

Two tables in `control.db`, times as unix nanoseconds:

- **`update_announcements`** — the announcement log, pruned per kind to
  the most recent `UpdateAnnouncementRetention` (50). The control-plane
  database holds a bounded recent window; anything a deployment must
  keep forever belongs in the storage primitive.
- **`worker_update_state`** — one row per `(worker_id, kind)`, so a
  node's current version is a single-row lookup that needs no
  announcement to exist.

The upsert's `CASE WHEN` clauses are load-bearing: a bare `current`
report carries no announcement, and assigning it unconditionally would
blank the `announcement_id` of a rollout the node is midway through,
silently dropping it out of its own rollout view.

This state is deliberately **not** `workers.metadata_json`.
`UpsertWorker` replaces that column wholesale, so anything living there
is destroyed by the next write that does not carry it forward — a
data-loss bug, not a style preference. It also has to be queryable
(`GROUP BY kind, current_version`), and it is seven fields per kind
rather than a flat string map.

## Node-side procedure — recommended, not enforced

Nothing checks any of this. `dream updates contract` prints it for an
agent to read:

1. Report `acknowledged`.
2. Stop accepting **new** work.
3. Let work already in flight finish; report `in_progress` if that
   will take a while.
4. Apply the update — `dream self-update` for `robotdreams/cli`,
   whatever applying means in your runtime for anything else.
5. Restart if the update needs it.
6. Resume, and report `applied` with the new version.

If it fails, report `failed` with `--detail` and carry on working on
the old version. If you are deliberately not applying it, report
`declined`. Both are legitimate.

**Ack the announcement.** An unacknowledged message is redelivered on
every reconnect, so a node that never acks re-sees the same
announcement forever. `dream message ack <id>` settles it; the
follower's own self-update path acks automatically.

## `dream self-update`

Updates the `dream` binary, and only that. It is a separate top-level
command from `dream updates` (which is about the fleet contract),
because naming them `update` and `updates` would differ by one letter
while meaning entirely different things.

**Install method decides what is allowed.** A binary a package manager
owns is never overwritten — writing behind npm's back leaves its
records claiming something that is no longer true:

```
$ dream self-update
  current  0.2.1   (installed via npm)
  latest   0.3.0

  refusing to replace a npm-managed binary.
  run:  npm install -g robotdreams@0.3.0
```

Detection resolves the executable, then classifies it as `release`,
`npm`, `go-install`, `source` or `unknown`. **Only `release` may be
replaced in place, and every ambiguity refuses** and prints a command
to run instead. A refusal exits non-zero, so a script cannot mistake
"I did not update" for "I updated". `dream version --verbose` shows
what was detected.

The in-place path, when allowed:

1. Download the release archive, hashing while writing.
2. Verify against the release's `checksums.txt`.
3. Extract the binary, staged **in the target's own directory** —
   `os.Rename` across filesystems fails with `EXDEV`, and `/tmp` is
   very often a separate mount.
4. **Run the staged binary's `dream version`** and require the version
   we asked for. This catches a truncated download, a wrong-architecture
   asset, and a release that served the wrong tag — while nothing live
   has been touched.
5. Rename the running binary aside, then the new one into place. Two
   renames rather than one, so a rollback exists; if the second fails,
   the original is restored, and if even that fails the error names
   both absolute paths.
6. `syscall.Exec` to re-exec, preserving the PID — which matters under
   systemd `Restart=always` or any supervisor tracking the process it
   started.

> **The checksum is not a signature.** `checksums.txt` travels the same
> channel as the archive it describes, so it defends against a
> corrupted or truncated download and a bad mirror — not against a
> compromised release or a stolen release token. Signing the checksums
> and verifying against a key that did not arrive alongside them is the
> natural next step; it is not done today.

**Windows is not supported in place**, deliberately. The rename dance
would work, but there is no `exec(2)`: restarting would mean a new PID,
orphaned from whatever service manager launched the old process, plus a
console and stdio handoff. That is a materially different reliability
story, and the node use case is Linux and macOS. Windows gets the
verified artifact details and an instruction to replace the file by
hand.

### Follower-driven self-update

`dream message tail --follow --self-update` lets an announced
`robotdreams/cli` update be applied automatically: the follower stops
reading, lets in-flight handling finish, reports `in_progress`,
applies, reports `applied`, acks the announcement, and re-execs.

**It is off by default, and the default matters.** A message from the
network causing this binary to replace itself is a real capability, so
it is opt-in via the flag or `DREAM_SELF_UPDATE=1`. With it off the
follower prints one line telling you to run `dream self-update` and
keeps following. If the install method refuses self-management, the
follower reports `declined`, says what to run, and **keeps following** —
a node that cannot update itself is still a working node, and killing
it because the control plane mentioned a version would turn an advisory
message into an outage.

A `ROBOTDREAMS_UPDATE_HOPS` guard refuses a second consecutive
self-update, so a control plane announcing a version the binary does not
report after the swap cannot drive an endless restart loop.

## Limits worth knowing

- **The fan-out is synchronous**, one `Emit` per registered node inside
  the HTTP handler, against an embedded SQLite queue capped at a single
  connection. At a few hundred nodes this holds the writer for the
  whole broadcast. A per-recipient failure does not abort the rest —
  the response reports `delivered` against `recipients` so a partial
  broadcast is visible rather than silent — but if fleets get large
  this should become "persist, return 202, fan out in a goroutine".
- **Announcements go to disconnected nodes too.** That is correct:
  delivery is durable and a reconnecting node should find its mail.
  The rollout view carries connectivity so the wait is legible.
- **The archive naming contract now has three copies** —
  `.goreleaser.yaml` (the source of truth), `npm/scripts/install.js`,
  and `internal/selfupdate/release.go`. `TestGoreleaserContractUnchanged`
  pins the Go copy against the YAML and CI runs the JS self-test, but
  nothing forces the two consumers to agree with *each other*.

## Commands

```sh
# admin
dream updates announce --kind robotdreams/cli --version 0.4.2 \
    --source https://github.com/acumen-ai-org/robotdreams/releases/tag/v0.4.2 \
    --severity recommended
dream updates rollout --kind robotdreams/cli

# node
dream updates pending
dream updates report --kind robotdreams/cli --status applied --announcement-id <id>
dream updates list
dream updates contract          # the contract, written for an agent to read
dream message ack <message-id>

# this binary
dream self-update [--check] [--version X] [--allow-downgrade] [--no-restart]
dream version --verbose
```

Admin credentials for `announce` and `rollout` resolve as
`--admin-token`, then `$DREAM_TOKEN`, then a token minted locally from
`--data-dir` (which requires filesystem access to the server's state,
and is therefore already equivalent to admin control — see
[security-model.md](security-model.md)).
