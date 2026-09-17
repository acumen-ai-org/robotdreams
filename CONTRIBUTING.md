# Contributing to Robot Dreams

Thanks for your interest in contributing.

## Getting started

1. Install Go 1.25 or newer (required by `go.mod`, due to
   `modernc.org/sqlite` and the Temporal SDK).
2. Fork and clone the repository.
3. Install Node 22+ if you want Mission Control (the dashboard) in your
   binary; `make build` builds it and embeds it. Without Node,
   `make go-build` compiles the Go side alone, with a placeholder page
   in the dashboard's place.
4. Run `make build` to compile the `dream` binary, and `make test` to run
   the unit test suite.

## Development workflow

- `make fmt` — format everything: `gofmt`, `golangci-lint fmt`
  (gofumpt + goimports), and prettier for the UI.
- `make vet` — run `go vet`.
- `make lint` — `golangci-lint run` (config in `.golangci.yml`) plus
  the UI's `npm run lint` and `npm run format:check`.
- `make test` — run unit tests.
- `make test-integration` — run build-tagged integration tests
  (`-tags=integration ./test/...`). These spin up real dependencies via
  `testcontainers-go` (MinIO) and require a working Docker daemon. The
  Temporal-backed tests (`pkg/messaging/temporal`) instead use the SDK's
  `testsuite.StartDevServer`, which downloads and runs the real
  `temporal` CLI dev-server binary — no Docker or external Temporal
  deployment needed, but it does need network access on first run to
  fetch that binary.

- `make ui-build` — build Mission Control (Vite + React, Node 22+)
  into `internal/dashboard/web/dist/`. That output is gitignored, never
  committed; the only tracked file under `web/` is the placeholder page
  the binary serves when no build is embedded, and CI rejects anything
  else there. `make ui-clean` drops `dist/`. `make ui-dev` runs the Vite dev loop
  (see docs/dashboard.md). `internal/dashboard/ui/` is an npm workspace:
  the app under `ui/app/` and the library under
  `ui/packages/mission-control/` share one `npm install`.
- `make ui-check` — the UI gates CI runs: eslint, prettier, tsc, the
  library build, and `npm run smoke`, which builds a consumer against
  the library's compiled output and typechecks against its emitted
  `.d.ts` (something `make ui-build` alone does not exercise). The
  library (`@robotdreams/mission-control`) is published by the
  `publish-mission-control` workflow on a `mission-control-v*` tag or a
  manual dispatch, never by the release pipeline.
- `make ci` — everything CI runs, in CI's order.

Please run `make ci` (or at least `make fmt lint test`) before opening a
pull request.

## Pull requests

- Keep pull requests focused on a single change.
- Add or update tests for any behavioral change.
- Describe the "why" in the PR description, not just the "what".
- Be sure your branch is up to date with `main` before requesting review.

## Commit messages

Commits follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <subject in the imperative, lowercase, no period>

<body: why, wrapped at 72 columns — optional but welcome>
```

- Types: `feat`, `fix`, `perf`, `refactor`, `docs`, `test`, `build`, `ci`,
  `chore`, `revert`. A breaking change adds `!` after the type/scope and a
  `BREAKING CHANGE:` footer.
- Scope is the area touched, lowercase: `messaging`, `storage`, `server`,
  `api`, `cli`, `identity`, `orgchart`, `reporting`, `scheduling`,
  `updates`, `selfupdate`, `simulation`, `dashboard`, `ui`, `docs`, `deps`.
  Leave it out when a change has no natural area.
- Every commit in a pull request is checked by commitlint in CI
  (`commitlint.config.mjs`); `feat`/`fix` subjects become CHANGELOG lines,
  so write them for a reader of the changelog.

Releases are automated: release-please keeps a release pull request open
against `main`, bumps the version from the commit types, and tags on
merge. There is no sign-off or CLA requirement.

## Code style

- Follow standard Go conventions (`gofmt`, `go vet` and `golangci-lint`
  clean — `.golangci.yml` says which linters; gofumpt formatting and
  goimports grouping with the module path as the local prefix).
- Prefer small, well-named packages with clear interfaces, consistent with
  the pluggable-backend architecture described in `docs/vision/architecture.md`.

## Reporting issues

Use GitHub Issues for bugs and feature requests. For security
vulnerabilities, see [SECURITY.md](SECURITY.md) instead of opening a
public issue.

## Code of conduct

This project follows the [Contributor Covenant](CODE_OF_CONDUCT.md).
