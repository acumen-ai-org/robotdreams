# Robot Dreams

[![CI](https://github.com/acumen-ai-org/robotdreams/actions/workflows/ci.yml/badge.svg)](https://github.com/acumen-ai-org/robotdreams/actions/workflows/ci.yml)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/acumen-ai-org/robotdreams/badge)](https://scorecard.dev/viewer/?uri=github.com/acumen-ai-org/robotdreams)
[![License: Apache-2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

**Status:** pre-release. There is no tagged release yet; the npm
package and prebuilt binaries described below become available with
the first tagged release. Until then, build from source.

Robot Dreams is a lightweight control plane for fleets of autonomous
software agents ("workers"), built around two primitives — messaging and
storage — plus a thin, user-added control plane for org-chart routing,
escalation, and oversight. See [`docs/vision/core.md`](docs/vision/core.md) for the
full product vision and design philosophy.

This repository holds the Go implementation: the `dream` CLI, a
control-plane server, pluggable messaging and storage backends, and a
security/identity layer built on short-lived, capability-scoped tokens.

## Get started

1. **Install the `dream` CLI:**

   ```sh
   npm install -g robotdreams        # prebuilt binary, no Go needed
   # or, from source:
   go install github.com/acumen-ai-org/robotdreams/cmd/dream@latest
   ```

2. **Start a control plane** (zero config — embedded queue, local
   storage, dashboard included):

   ```sh
   dream server init
   ```

   The startup output is a setup guide that ends with exactly what to do
   next (re-print it anytime with `dream server guide`).

3. **Connect an agent — from anywhere.** For agents on other machines
   the server has to listen beyond loopback: `dream server init --addr
   :7420` plus `--tls-cert`/`--tls-key`, or `--insecure` behind a
   TLS-terminating proxy (the default `--addr` is `127.0.0.1:7420`, this
   machine only). Then, on the agent's runtime — any machine, any
   cloud — set two variables:

   ```sh
   export DREAM_URL=<the server's reachable URL>
   export DREAM_TOKEN=<the enrollment token from the guide>
   ```

   …and then just tell your agent to create its node with:

   ```sh
   dream node onboard
   ```

   That command tells your agent everything it needs to know: how to
   connect its node, send and receive messages, and put and get its
   work in the library (storage).

The full walkthrough is in [docs/setup-guide.md](docs/setup-guide.md);
a detailed local tour is in the [Quickstart](#quickstart-zero-config)
below.

## Prerequisites

- **Go 1.25 or newer.** `go.mod` requires it — both `modernc.org/sqlite`
  and the Temporal SDK need a current Go toolchain. Check with `go version`.
- **Node 22+** to build a binary that includes Mission Control (the
  dashboard). Its Vite build output is not committed; `make build`
  produces it (`internal/dashboard/web/dist/`) and embeds it. Without Node,
  `go build` still succeeds and the binary serves a placeholder page at
  `/dashboard` explaining how to build the real one
  ([docs/dashboard.md](docs/dashboard.md)).

## Build

```sh
make build      # builds Mission Control, then -> bin/dream with it embedded
make go-build   # Go only (no Node): bin/dream with the dashboard placeholder
# or, equivalently to the second:
go build -o bin/dream ./cmd/dream
```

`make test` runs the unit test suite (`make test-race` with the race
detector, as CI does); `make lint` runs golangci-lint plus the UI's
eslint/prettier checks; `make test-integration` runs the build-tagged
integration tests (some require Docker); `make ci` runs everything CI
runs — see [CONTRIBUTING.md](CONTRIBUTING.md).

### Install via npm (no Go toolchain)

```sh
npm install -g robotdreams   # installs the `dream` command
# or one-off:
npx robotdreams -- --help
```

The npm package downloads a prebuilt `dream` binary for your platform
(Linux/macOS/Windows, x64/arm64) from the matching GitHub Release and
verifies its checksum. It needs a tagged release to exist (see the
status line at the top); `go install
github.com/acumen-ai-org/robotdreams/cmd/dream@latest` is the
alternative that needs only Go (that binary carries the dashboard
placeholder, not Mission Control — `go install` cannot run the UI
build).

## Quickstart (zero-config)

Everything below works on a clean checkout with no external services —
the default messaging backend is an embedded SQLite queue and the default
storage backend is the local filesystem, both created under
`~/.dream/_server`.

1. **Start a control plane.** This blocks in the foreground and also
   serves Mission Control (the dashboard) at `/dashboard`:

   ```sh
   bin/dream server init
   ```

   Run it in a second terminal, or background it, before continuing.

2. **Connect two workers**, forming a simple two-level org chart:

   ```sh
   bin/dream worker connect --server 127.0.0.1:7420 --worker-id root --role lead
   bin/dream worker connect --server 127.0.0.1:7420 --worker-id leaf1 --role ic --reports-to root
   ```

   Each `connect` generates an Ed25519 identity under `~/.dream/<server-id>/`
   and records local config so later commands can default `--server` /
   `--worker-id`. Registration also requires an admin/enrollment
   credential (see [docs/security-model.md](docs/security-model.md)); on
   this same-machine, default-data-dir, loopback setup it's auto-discovered
   with no extra flag. A remote worker, or a local server on a custom
   `--data-dir`, needs `--admin-token` passed explicitly.

3. **Send a message** from the leaf and **tail** it as the root:

   ```sh
   bin/dream message send --worker-id leaf1 --server 127.0.0.1:7420 \
     --type status_update --subject "hello" --body '{"progress":"50%"}'

   bin/dream message tail --worker-id root --server 127.0.0.1:7420
   ```

4. **Put and get an object** in the storage backend:

   ```sh
   bin/dream storage put demo/hello.txt ./some-local-file.txt \
     --worker-id leaf1 --server 127.0.0.1:7420
   bin/dream storage ls --worker-id leaf1 --server 127.0.0.1:7420
   ```

5. **Open the dashboard**: visit `http://127.0.0.1:7420/dashboard/` in a
   browser. See [docs/dashboard.md](docs/dashboard.md) for the token-paste
   auth flow.

Run `dream --help`, or `--help` on any subcommand, for the full flag
reference — or see [docs/quickstart.md](docs/quickstart.md) for every
subcommand documented with examples.

## Further documentation

- [docs/quickstart.md](docs/quickstart.md) — every CLI subcommand, in detail.
- [docs/setup-guide.md](docs/setup-guide.md) — server setup guide: what init
  creates, the enrollment token, and connecting agents from anywhere with
  `DREAM_URL`/`DREAM_TOKEN` + `dream node onboard`.
- [docs/messaging-backends.md](docs/messaging-backends.md) — embedded, webhook,
  and Temporal messaging backends.
- [docs/storage-backends.md](docs/storage-backends.md) — local filesystem and
  S3-compatible storage backends.
- [docs/scheduling.md](docs/scheduling.md) — schedules: a worker registers
  one, the control plane sends the message on a cron cadence.
- [docs/security-model.md](docs/security-model.md) — the identity/token model,
  and its current scope and limitations.
- [docs/apps.md](docs/apps.md) — how a node advertises something a human
  can open, and how Mission Control offers it.
- [docs/templates.md](docs/templates.md) — onboarding templates: concrete
  recipes for one kind of node, printed by `dream node onboard --template`.
  They are documents, not machinery — printing one installs nothing.
- [docs/updates.md](docs/updates.md) — the update contract: how the control
  plane announces that a new version of something is available, what a node
  says back, and how `dream` updates itself.
- [docs/dashboard.md](docs/dashboard.md) — running and using Mission Control.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Licensed under
[Apache-2.0](LICENSE).

## About

Robot Dreams is an open-source project by
[Acumen AI](https://acumen-ai.org) — learn more about the project and the
vision behind it at
[acumen-ai.org/robotdreams](https://acumen-ai.org/robotdreams).
