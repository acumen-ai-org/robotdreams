# Server setup guide

This is the written form of the guide the CLI itself carries: `dream
server init` ends its startup banner with the punchline below, and
`dream server guide` prints the full version on demand. Its agent-facing
counterpart is [`dream node onboard`](#connect-an-agent-from-anywhere),
which tells an agent everything it needs to know.

## What `dream server init` creates

With no flags at all (the zero-config path), the first startup creates
`~/.dream/_server/` and everything in it:

| What                        | Where / role                                                        |
| --------------------------- | ------------------------------------------------------------------- |
| Control-plane DB (SQLite)   | Org chart, revocations, and the embedded message queue              |
| Signing key                 | Signs the short-lived worker capability tokens (JWTs)               |
| `enrollment.token` (`0600`) | The secret that authorizes new workers to enroll                    |
| `storage/`                  | The default local-filesystem storage backend                        |

Backend URIs passed once are persisted: a later flagless run against the
same `--data-dir` keeps using them.

## Server URLs

- **API**: `http://127.0.0.1:7420` by default (`--addr` defaults to
  `127.0.0.1:7420`: this machine only). `--addr :7420` binds all
  interfaces, and then needs `--tls-cert`/`--tls-key` (giving
  `https://<host>:7420`) or `--insecure` — see the flags below.
- **Dashboard**: `<the API URL>/dashboard/` — Mission Control, served
  by the same process unless `--no-dashboard` is passed.

## The enrollment token

Generated on first-ever startup, written to
`<data-dir>/enrollment.token` (mode `0600`), path printed in the startup
banner. Read it with:

```sh
cat ~/.dream/_server/enrollment.token
```

`dream worker connect` on the server's own machine auto-discovers it; a
worker anywhere else needs it handed over out of band — via the
`DREAM_TOKEN` environment variable or `--admin-token`. `dream server
guide` prints only the file's path; `dream server guide --show-token`
prints the value itself. See [security-model.md](security-model.md) for
the model behind it.

To replace the token — after it leaked, or on a schedule — run

```sh
dream server rotate-enrollment      # add --show-token to print the new value
```

The old value stops working immediately, with no restart (the server
re-reads the file on every enrollment attempt); workers that are already
connected are unaffected — only new registrations need the new token.

## Flags that matter (`dream server init`)

| Flag                | One line                                                                 |
| ------------------- | ------------------------------------------------------------------------ |
| `--addr`            | Listen address (default `127.0.0.1:7420`, this machine only; `:7420` = all interfaces, which then needs the TLS flags or `--insecure`) |
| `--tls-cert`        | PEM certificate file: serve HTTPS directly (TLS 1.2+); requires `--tls-key` |
| `--tls-key`         | PEM private key file for `--tls-cert`                                   |
| `--insecure`        | Allow plaintext HTTP on a non-loopback `--addr` (only behind a TLS-terminating proxy or on a trusted network) |
| `--data-dir`        | State directory (default `~/.dream/_server`)                             |
| `--messaging`       | Messaging backend URI (default: embedded SQLite queue in the data dir)   |
| `--storage`         | Storage backend URI (default: local files in the data dir)               |
| `--orgchart`        | YAML org chart bulk-loaded on a first-ever startup                       |
| `--no-dashboard`    | Do not serve Mission Control at `/dashboard`                             |
| `--open-enrollment` | Disable enrollment authorization (dev/test only; never on a network)     |

A server that must be reachable from other machines must serve TLS:
either directly, with `--tls-cert`/`--tls-key`, or behind a
TLS-terminating proxy with `--insecure` telling the server that the
plaintext hop is deliberate. `dream server init` refuses a non-loopback
`--addr` without one of the two ([security-model.md](security-model.md)).

## Pointing workers at a non-loopback URL

Workers on other machines need a URL that resolves to the server —
`https://<host-or-ip>:7420` when it serves TLS itself, or your TLS
proxy's `https://` URL. That URL
is what goes in `DREAM_URL` below; a loopback address only works for
workers on the server's own machine.

## Environment variables

Every `dream` command honors two environment variables, so an agent
runtime configured once needs no per-command flags:

- `DREAM_URL` — the control plane's base URL; used whenever `--server`
  is not passed.
- `DREAM_TOKEN` — an enrollment/admin token; used whenever
  `--admin-token` is not passed.

Precedence, everywhere both mechanisms apply: **explicit flag >
environment variable > local auto-discovery** (the enrollment-token file
above, and the `~/.dream/<server-id>/config.json` a previous `dream
worker connect` recorded).

## Connect an agent from anywhere

```text
───────────────────────────────────────────
Connect an agent from anywhere:
  1. On your runtime — any machine, any cloud — set two variables:
       DREAM_URL=<this server's reachable URL>
       DREAM_TOKEN=$(cat ~/.dream/_server/enrollment.token)
  2. Then just tell your agent to create its node with:
       dream node onboard
       (or: npx robotdreams node onboard)
     That command tells your agent everything it needs to know.
───────────────────────────────────────────
```

`dream node onboard` (a.k.a. `dream worker onboard` — `node` is the
vision-level word for the same entity, see
[vision/core.md](vision/core.md)) reads `DREAM_URL`/`DREAM_TOKEN`,
health-checks the server, and prints a complete onboarding brief written
for an AI agent as the reader: how to register its node, send every
message type, tail its inbox, and use storage — every command runnable
as printed, given just those two variables.
