# Changelog

## 0.1.0 (2026-09-18)


### Miscellaneous Chores

* bootstrap the first release ([0c07dd0](https://github.com/acumen-ai-org/robotdreams/commit/0c07dd0a20b7a09189f49b8c94a71ef3a3b0ea70))

## Changelog

All notable changes to this project are documented in this file.

Versioned sections are written by [release-please](https://github.com/googleapis/release-please)
from the Conventional Commits merged to `main`; the project adheres to
[Semantic Versioning](https://semver.org/spec/v2.0.0.html) once it reaches
1.0. Pre-1.0, only the latest release is supported (see
[SECURITY.md](SECURITY.md)).

## Before the first release

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
