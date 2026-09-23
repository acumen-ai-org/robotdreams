# Changelog

Versioned sections are written by [release-please](https://github.com/googleapis/release-please)
from the Conventional Commits merged to `main`; the project adheres to
[Semantic Versioning](https://semver.org/spec/v2.0.0.html) once it reaches
1.0. Pre-1.0, only the latest release is supported (see
[SECURITY.md](SECURITY.md)).

## [0.1.5](https://github.com/acumen-ai-org/robotdreams/compare/v0.1.4...v0.1.5) (2026-09-23)


### Bug Fixes

* **release:** release-please builds the tag it creates ([0429ffa](https://github.com/acumen-ai-org/robotdreams/commit/0429ffa483bd645d9acdc22d541b3198ecfa6daf))

## [0.1.4](https://github.com/acumen-ai-org/robotdreams/compare/v0.1.3...v0.1.4) (2026-09-22)


### Features

* **cli:** delete a worker and revoke its identity ([59841ac](https://github.com/acumen-ai-org/robotdreams/commit/59841acdc34a534f03af5506dcae35432f4c3515))
* **cli:** edit a worker's role after it has connected ([c6f4679](https://github.com/acumen-ai-org/robotdreams/commit/c6f46792220ea8b175ecb642c00613b9e6313f10))

## [0.1.3](https://github.com/acumen-ai-org/robotdreams/compare/v0.1.2...v0.1.3) (2026-09-18)


### Bug Fixes

* **release:** re-runs replace assets; the npm manifest tracks the tag ([08ed004](https://github.com/acumen-ai-org/robotdreams/commit/08ed004b035078a1c3a4eef3c1940a699b967671))

## [0.1.2](https://github.com/acumen-ai-org/robotdreams/compare/v0.1.1...v0.1.2) (2026-09-18)


### Bug Fixes

* **cli:** build on Windows — process groups behind build tags ([ae2fdd0](https://github.com/acumen-ai-org/robotdreams/commit/ae2fdd001401e08cbc56c4e876dd08a84fd8b929)), closes [#6](https://github.com/acumen-ai-org/robotdreams/issues/6)

## [0.1.1](https://github.com/acumen-ai-org/robotdreams/compare/v0.1.0...v0.1.1) (2026-09-18)


### Bug Fixes

* **release:** goreleaser before hook as one shell command ([7ab6617](https://github.com/acumen-ai-org/robotdreams/commit/7ab6617b19832bc4bd89ba51ba7a351e728316ac))

## 0.1.0 (2026-09-18)

The initial public release. Everything below the next heading is what it
contains; the commit history was squashed in under prose titles, which is
why release-please lists only the bootstrap commit here.

### Miscellaneous Chores

* bootstrap the first release ([0c07dd0](https://github.com/acumen-ai-org/robotdreams/commit/0c07dd0a20b7a09189f49b8c94a71ef3a3b0ea70))

## What 0.1.0 contains

Initial public release of Robot Dreams: two primitives (messaging, storage)
and a control plane, plus the Mission Control dashboard.

### Added

- Storage and messaging abstractions, with pluggable backends: embedded
  SQLite, webhook, and Temporal for messaging; local filesystem and
  S3-compatible for storage. Each backend ships a conformance test suite
  (`pkg/messaging/testsuite`, `pkg/storage/testsuite`) so new backends can
  verify they satisfy the shared contract.
- The security/identity core: signed, capability-scoped tokens
  (`pkg/security`, `internal/identity`).
- The Reporting layer: contracts, a report-definition/instance library, and
  the control-plane API surface for it.
- Scheduling: workers register a cron-cadence schedule and the control plane
  sends the message.
- The AgentEnvironment layer and node/organizational-hierarchy vision docs.
- The `dream` CLI, control-plane server, and org chart; `dream simulate` for
  a throwaway control plane; `npm`/`npx` distribution.
- Mission Control: a React dashboard for the control plane, packaged as an
  npm workspace library with embedding seams and a gated publish, including
  Explore, a detail panel, and scoped CSS/routes.
- Node updates, apps, and onboarding templates.

### Changed

- Delegated child workers get a lightweight join for ephemeral workers, and
  the vocabulary gained a delegate role.
- Container image and token-TTL flags for the control plane.

### Security

- Message scopes are now enforced by the API; a report's producer is bound
  to the caller rather than caller-supplied.
- Storage writes are attributed to their author; the advisory-only Category
  field was dropped in favor of that attribution.
- Server shutdown now releases open streams before the backends close,
  avoiding a shutdown race with in-flight messages.
