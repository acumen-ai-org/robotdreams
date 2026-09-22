# Quickstart

Every `dream` CLI subcommand, with a short description and a real,
verified example. Flag names below were checked against the actual
built binary (`dream <command> --help`) — if they ever drift, that
`--help` output is the source of truth, not this file.

Build the CLI with `make build` (or `go build -o bin/dream ./cmd/dream`).
Alternatively, install via npm without a Go toolchain — `npm install -g
robotdreams` (installs the `dream` command) or `npx robotdreams --
--help` — noting that the npm channel activates with the first tagged
release once the repository is public; until then use `go install
github.com/acumen-ai-org/robotdreams/cmd/dream@latest` or build from
source.

All backend-specific defaults (embedded/SQLite messaging, localfs
storage) apply when you don't pass `--messaging` / `--storage`. See
[messaging-backends.md](messaging-backends.md) and
[storage-backends.md](storage-backends.md) for the other options.

Two conventions apply CLI-wide:

- **Environment variables.** `DREAM_URL` supplies the server address
  wherever `--server` is not passed, and `DREAM_TOKEN` supplies the
  enrollment/admin credential wherever `--admin-token` is not passed.
  Precedence: explicit flag > environment variable > local
  auto-discovery. See [setup-guide.md](setup-guide.md).
- **`node` is an alias for `worker`.** `dream node
  connect|list|edit|reassign|onboard` work identically to their `dream
  worker ...` forms — "node" is the vision-level term for the same
  entity ([vision/core.md](vision/core.md)); `worker` stays canonical.

## `dream server init`

Initialize a control-plane data directory and serve the HTTP API from it.
With no flags, this is the zero-config path: state goes to
`~/.dream/_server`, messaging uses the embedded SQLite queue, and storage
uses the local filesystem, both inside that directory. It blocks until
`SIGINT`/`SIGTERM`, then shuts down gracefully.

```sh
dream server init
dream server init --addr :7420 --tls-cert cert.pem --tls-key key.pem \
  --data-dir /var/lib/dream \
  --messaging queue://sqlite/var/lib/dream/messages.db \
  --storage file:///var/lib/dream/storage
```

Flags: `--addr` (default `127.0.0.1:7420`, this machine only; an address
other machines can reach needs `--tls-cert`/`--tls-key` — PEM files, TLS
1.2+ — or `--insecure` when a TLS-terminating proxy sits in front),
`--data-dir` (default
`~/.dream/_server`), `--messaging`, `--storage`, `--orgchart` (YAML org
chart to bulk-load on first-ever startup), `--reports` (report definition
library directory), `--no-dashboard` (don't serve Mission Control at
`/dashboard`), `--token-ttl` (lifetime of minted worker tokens, default
`15m`), `--delegated-token-ttl` (default and maximum lifetime of a
delegated child token, default `5m`; a child is still capped at its
parent's remaining lifetime), `--open-enrollment` (disable worker
enrollment authorization; dev/test only — see
[security-model.md](security-model.md)).

A container image is built from the repository's `Dockerfile`
(`docker build -t dream .`); it ships the report library at `/reports`
and defaults to `server init --data-dir /data --addr :7420 --insecure
--reports /reports` (plaintext inside the container network: put a TLS
proxy in front, or override the command with `--tls-cert`/`--tls-key`),
so `docker run -p 7420:7420 -v dream-data:/data dream` is a complete
control plane.

The enrollment token lives at `<data-dir>/enrollment.token`. `dream
server rotate-enrollment` replaces it — the old value stops working at
once, connected workers are unaffected — and `dream server guide
--show-token` prints the value (the guide shows only the path by
default).

Backend URIs passed here are persisted: a later flagless run against the
same `--data-dir` keeps using them.

On first startup this also generates an **enrollment token**, written to
`<data-dir>/enrollment.token` (mode `0600`) and printed in the startup
banner. `dream worker connect` on the same machine reads it automatically
(see below); a worker connecting from elsewhere needs it passed
explicitly (`DREAM_TOKEN` or `--admin-token`).

The startup banner ends with the "connect an agent from anywhere"
punchline: set `DREAM_URL` and `DREAM_TOKEN` on any runtime, then have
the agent run `dream node onboard` (below).

## `dream server guide`

Print the full server setup guide — the long form of init's banner: what
init creates and where, the URLs it serves, where the enrollment token
lives (and its value, when readable locally), the flags that matter, and
how to connect an agent from anywhere with just `DREAM_URL` and
`DREAM_TOKEN`. Works without a running server; see
[setup-guide.md](setup-guide.md) for the same content as a document.

```sh
dream server guide
dream server guide --data-dir /var/lib/dream --server dream.example.com:7420
```

Flags: `--data-dir` (default `~/.dream/_server`), `--server` (address
shown in the guide; default `$DREAM_URL`, else `127.0.0.1:7420`).

## `dream server revoke <worker-id>`

Revoke a worker's credentials on a running control plane. This is a
**local-admin bootstrap path**, not a full administrator identity model —
see [security-model.md](security-model.md) for what that means.

```sh
dream server revoke leaf1 --server 127.0.0.1:7420 --reason "key suspected leaked"
```

Flags: `--data-dir` (default `~/.dream/_server`, used to mint the local
bootstrap admin token), `--server` (default `127.0.0.1:7420`), `--reason`.

## `dream worker connect`

Register this worker with a control plane. Generates (or reuses) an
Ed25519 identity at `~/.dream/<server-id>/identity.key`, proves possession
via a signed-nonce challenge, registers the worker, and records
`~/.dream/<server-id>/config.json` so later commands default `--server`
and `--worker-id`.

Registration also requires an admin or enrollment credential (see
[security-model.md](security-model.md)) — proof of key possession alone
is no longer enough to claim a worker ID. For a **loopback** `--server`
address this is auto-discovered from the local server's default data
directory (`~/.dream/_server/enrollment.token`), so the zero-config
quickstart below needs no extra flag when the server and the worker run
on the same machine. Connecting to a remote server, or a local server
started with a custom `--data-dir`, needs `--admin-token` passed
explicitly.

```sh
dream worker connect --server 127.0.0.1:7420 --worker-id root --role lead
dream worker connect --server 127.0.0.1:7420 --worker-id leaf1 --role ic \
  --reports-to root --metadata team=infra,tier=1

# Remote server, or a local server on a custom --data-dir: pass the
# enrollment token (or an admin token) explicitly.
dream worker connect --server dream.example.com:7420 --worker-id leaf2 \
  --role ic --admin-token "$(cat /path/to/enrollment.token)"

# Or configure once via the environment — no --server/--admin-token flags:
export DREAM_URL=dream.example.com:7420
export DREAM_TOKEN="$(cat /path/to/enrollment.token)"
dream node connect --worker-id leaf3 --role ic --reports-to root
```

Flags: `--server` (default `$DREAM_URL`; required if that is unset),
`--worker-id` (default: hostname), `--role`, `--reports-to` (empty =
reports directly to root), `--metadata` (`key=value` pairs),
`--admin-token` (admin or enrollment credential; precedence: flag, then
`$DREAM_TOKEN`, then auto-discovery for a loopback `--server` against
the default data dir).

A node also declares what versions it runs, so a rollout view can show
it. The `dream` CLI's own version is declared automatically; add your
own kinds with `--version-of`:

```sh
dream worker connect --version-of acme/prompt-pack=3
```

This is a best-effort follow-up call: if it fails you get a warning, not
a failed connect. See [updates.md](updates.md).

## `dream worker delegate`

The lightweight join for an ephemeral child — a subprocess a worker
spawns to do one thing and exit. No keypair, no org-chart entry, no
refresh: the parent mints a short-lived token and hands it over.

```sh
# From the parent's identity; prints only the token, so it can go
# straight into the child's environment.
CHILD_TOKEN=$(dream worker delegate --child fetch-1 --ttl 2m)
```

The child is known as `<parent>~fetch-1`, calls the same `/api/*`
endpoints with `Authorization: Bearer $CHILD_TOKEN`, and every write it
makes is attributed to that ID. Its scopes are the parent's minus
`admin` (or a narrower set via repeated `--scope`); the token lasts at
most the server's delegated-token limit (five minutes by default), never
past the parent's own token, and cannot delegate further. Revoking the
parent revokes every child. `--json` prints the whole credential.
See docs/security-model.md for the rules.

## `dream worker app`

Declare what this node serves for a human to open — one URL and a short
description, shown on the node in Mission Control.

```sh
dream worker app set --url https://build.example.test --description "Build dashboard"
dream worker app clear
dream worker app list
```

`dream worker connect` also takes `--app-url` / `--app-description` as a
convenience, but only at a node's **first** registration: a second connect
for a live worker is refused with 409, so use `app set` to change it later.

The control plane stores the claim and never fetches the URL. It does
require `http` or `https`, because the dashboard turns this into something
an operator clicks. See [apps.md](apps.md).

## `dream node onboard`

(Equivalently `dream worker onboard`.) Print a self-contained onboarding
brief **written for an AI agent as the reader**: what Robot Dreams is, a
live reachability check of the control plane at `$DREAM_URL` (a warning,
not a failure, if unreachable), and the exact commands to register a
node, send each of the four message types, tail the inbox, and use
storage — every command runnable as printed, given `DREAM_URL` and
`DREAM_TOKEN` in the environment. Errors out only when neither
`$DREAM_URL` nor `--server` names a server.

```sh
DREAM_URL=127.0.0.1:7420 dream node onboard
```

Flags: `--server` (default `$DREAM_URL`).

### Templates

A **template** is a concrete recipe for one kind of node, printed instead of
the generic brief:

```sh
dream node onboard --list-templates
dream node onboard --template linux-onstart-claude
```

`linux-onstart-claude` sets up a Linux box that listens for messages on boot
and hands each one to an AI coding agent, which reports back over Robot
Dreams messaging.

Printing a template **installs nothing** — no file is written, no unit is
registered, nothing is started. It is a document you choose to act on. See
[templates.md](templates.md) for what that means and why, including the
security property a message-driven agent node has by design.

## `dream worker edit <worker-id>`

Change what a worker is on the org chart. Today the one editable field
is `--role`, the free-form label the control plane records and Mission
Control draws an icon from; a role is otherwise uninterpreted, so
changing it relabels the node, it does not move it or change what it may
do. A role could previously only be set at `dream worker connect`, and a
second connect is refused while the worker is registered — this is the
way to correct a mislabelled node. Authenticated as the local identity
selected by `--server`/`--server-id`/`--worker-id`; the control plane
only allows the call when the caller is the worker itself, the worker's
parent, or holds the admin scope — the same rule as `reassign`.

```sh
dream worker edit leaf1 --role reviewer
dream worker edit leaf1 --role ""   # clear the label (default ⬡ icon)
```

Prints `<worker-id> is now <role>`, with `-` for a cleared role. Passing
no `--role` at all is an error: there is nothing to edit.

Flags: `--role`, `--server`, `--server-id`, `--worker-id`.

## `dream worker reassign <worker-id>`

Move a worker under a new parent. Authenticated as the local identity
selected by `--server`/`--server-id`/`--worker-id`; the control plane
only allows the call when the caller is the worker itself, the worker's
*current* parent, or holds the admin scope.

```sh
dream worker reassign leaf1 --reports-to lead2
dream worker reassign leaf1 --reports-to ""   # move to root
```

Flags: `--reports-to`, `--server`, `--server-id`, `--worker-id`.

## `dream worker list`

List every worker in the org chart.

```sh
dream worker list --server 127.0.0.1:7420
```

Flags: `--server`, `--server-id`, `--worker-id`.

## `dream message send`

Send a message from the local worker identity. `--to` is honored only for
`completed_work` and `request_for_input`; `status_update` and `escalation`
always go exactly one hop, to the sender's parent, and any `--to` is
ignored server-side.

```sh
dream message send --type status_update --subject "50% done" \
  --body '{"progress":"50%"}'
dream message send --type escalation --subject "blocked" \
  --body '{"reason":"missing credentials"}'

# completed_work carries a storage POINTER, never the payload:
# put the deliverable into storage first, then send its path.
dream storage put shared/leaf1/report.md ./report.md
dream message send --type completed_work --subject "report ready" \
  --storage-path shared/leaf1/report.md
```

Flags: `--type` (required: `status_update` | `completed_work` |
`escalation` | `request_for_input`), `--subject`, `--body` (raw JSON),
`--storage-path` / `--storage-revision` (attach a storage pointer to the
message — put before send), `--to`, `--causation-id`, `--server`,
`--server-id`, `--worker-id`.

An `escalation` or `request_for_input` from a worker that already reports
to root fails with a conflict — there's nobody above it to answer.

## `dream message tail`

Print a worker's recent messages, optionally following live via SSE.

```sh
dream message tail --limit 20
dream message tail --follow
dream message tail --worker leaf1 --since 2026-08-19T00:00:00Z
```

```sh
dream message tail --follow --json
```

`--json` prints one JSON envelope per line (NDJSON) instead of the human
line. This is what a script should read: the human line does not carry the
message **id**, and without an id you cannot `dream message ack` what you
just handled — and an unacked message is redelivered on every reconnect. The
JSON is the API's own envelope shape, so a reader of this stream and a reader
of `GET /api/messages` see the same fields. Under `--json` an empty inbox
prints nothing at all, and the follower's own "an update is available" notice
goes to stderr, so stdout stays parseable.

Flags: `--follow`, `--json`, `--limit`, `--since` (RFC3339), `--worker`
(default: the calling identity — reading another worker's inbox requires the
admin scope), `--self-update`, `--server`, `--server-id`, `--worker-id`.

## `dream message ack <message-id>`

Acknowledge a message you have handled.

```sh
dream message ack 3f9a1c2e --action handled
```

This matters more than it looks: an unacknowledged message stays in your
inbox and is redelivered on every reconnect, so a node that never acks
sees the same messages forever. `--action` is free text (default
`read`), and the acknowledgment is always attributed to the calling
identity.

Flags: `--action`, `--server`, `--server-id`, `--worker-id`.

## `dream updates`

Announce, inspect and report on fleet updates. Robot Dreams ships an
update *contract*, not an update runtime: the control plane broadcasts
"a version of `<kind>` is available" and records what nodes say back. It
never installs, restarts or supervises anything, and never compares two
version strings. Full detail in [updates.md](updates.md).

```sh
# admin: broadcast to every node, which each filter on --kind themselves
dream updates announce --kind robotdreams/cli --version 0.4.2 \
    --source https://github.com/acumen-ai-org/robotdreams/releases/tag/v0.4.2 \
    --severity recommended

# admin: how is it going, fleet-wide
dream updates rollout --kind robotdreams/cli

# node: what is announced that I have not settled
dream updates pending

# node: say what happened
dream updates report --kind robotdreams/cli --status applied \
    --announcement-id 4b0f2c9d --current-version 0.4.2

# the contract itself, written for an agent to read
dream updates contract
```

`robotdreams/cli` is the one reserved kind. Your deployment defines its
own (`acme/prompt-pack`, say) and gets the same rollout tracking with no
server changes. Statuses are `current`, `acknowledged`, `in_progress`,
`applied`, `declined`, `failed`; only `applied` and `declined` settle an
announcement.

`announce` and `rollout` need an admin credential, resolved as
`--admin-token`, then `$DREAM_TOKEN`, then minted locally from
`--data-dir`.

## `dream self-update`

Update the `dream` binary itself — distinct from `dream updates`, which
is about the fleet.

```sh
dream self-update --check
dream self-update
```

What happens depends on how dream was installed. A release binary is
replaced in place: downloaded, checksum-verified, run once to confirm it
works, swapped atomically, then re-exec'd with the same PID. A binary
npm or `go install` owns is **never** overwritten — dream prints the
exact command to run and exits non-zero. So does an install it cannot
identify. `dream version --verbose` shows what it detected.

Flags: `--check`, `--version`, `--allow-downgrade`, `--no-restart`.

Note the checksum is not a signature: it travels the same channel as the
archive, so it catches corruption, not a compromised release.

## `dream storage put <path> <local-file>`

Upload a local file to an object path. Use `-` as `<local-file>` to read
from stdin.

```sh
dream storage put demo/hello.txt ./hello.txt
dream storage put demo/hello.txt ./hello.txt --if-match-revision <rev>
```

Flags: `--if-match-revision` (conditional write; fails with a conflict if
the object changed since that revision), `--server`, `--server-id`, `--worker-id`.

## `dream storage get <path> <local-file>`

Download an object. Use `-` as `<local-file>` to write bytes to stdout
(object metadata always goes to stderr, so stdout stays clean for piping).

```sh
dream storage get demo/hello.txt ./out.txt
dream storage get demo/hello.txt - > out.txt
```

Flags: `--server`, `--server-id`, `--worker-id`.

## `dream storage ls [prefix]`

List objects under a prefix (no prefix lists everything).

```sh
dream storage ls
dream storage ls demo/
```

Flags: `--server`, `--server-id`, `--worker-id`.

## `dream dashboard`

Serve Mission Control as a standalone process pointed at a remote control
plane. Only needed when the control plane's own dashboard was disabled
(`--no-dashboard`) or you want it served separately — otherwise just open
`http://<server>/dashboard/` directly.

```sh
dream dashboard --server 127.0.0.1:7420 --addr 127.0.0.1:7421
```

Flags: `--server` (default `127.0.0.1:7420`), `--addr` (default
`127.0.0.1:7421`). See [dashboard.md](dashboard.md) for the auth flow.
