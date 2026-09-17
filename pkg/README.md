# pkg

Public, importable Go packages for Robot Dreams. Each package is a small
abstraction with pluggable backends selected at runtime by URI scheme;
`internal/` holds the control-plane server and CLI that wire these together
and are not meant to be imported by other projects.

## Packages

### `messaging`

Defines the messaging abstraction workers use to exchange envelopes (status
updates, completed-work reports, escalations, requests for input) without
coupling to a transport. `MessagingBackend` is a dumb transport — it delivers
an `Envelope` to a single, already-resolved recipient; org-graph routing
("escalate to my manager") is out of scope and lives above this package.
Backends register themselves with the package registry (`messaging.Register`)
under a URI scheme, so callers select one via `messaging.New(uri)`.

Backends:

- `messaging/embedded` — zero-config, embedded SQLite (`queue://sqlite/...`),
  using the pure-Go `modernc.org/sqlite` driver so the binary cross-compiles
  without cgo.
- `messaging/webhook` — delivers via HTTP callbacks with no broker required;
  one synchronous delivery attempt, then an in-memory outbox with
  exponential-backoff retries on failure.
- `messaging/temporal` — backed by Temporal Workflows and Signals, one
  long-running workflow ("mailbox") per worker ID.
- `messaging/testsuite` — the backend-agnostic conformance suite; every
  backend's tests should call `testsuite.RunConformance` against it.

### `storage`

Defines the storage abstraction workers use to read and write objects
without coupling to a backend. Concurrency is optimistic: a caller that must
avoid clobbering a concurrent writer passes `PutOptions.IfMatchRevision`
rather than acquiring a lease.

Backends:

- `storage/localfs` — local filesystem; the reference/development backend.
- `storage/s3compat` — any S3-compatible object store (AWS S3, MinIO,
  Cloudflare R2, ...) via the AWS SDK for Go v2 with an explicit endpoint
  override. Uses the object's ETag uniformly as `Revision`.
- `storage/testsuite` — the backend-agnostic conformance suite for
  `storage.StorageBackend`.

### `security`

The worker-facing half of Robot Dreams' identity model: signed,
capability-scoped tokens (`Token`, `Claims`) and a `TokenSource` client that
keeps a worker's token fresh in the background. The server-side counterpart
(minting, JWKS, revocation) lives in `internal/identity`. See
[`docs/vision/security.md`](../docs/vision/security.md) for the full design.

### `reporting`

The contracts library for the reporting subsystem: parses YAML
`ReportDefinition`s, validates them against the shared vocabulary
(`reporting/contracts/*.yaml`, baked in as Go constants), models the report
instances nodes submit, and aggregates instances across organizational
scopes.

### `scheduling`

Defines a `Schedule` — a standing instruction to the control plane to send
one message to one worker on a cron cadence — and the `Store` a control
plane keeps them in. A schedule is not a job: the control plane only knows
who/when/what envelope, never what the message means or what the recipient
does with it.

### `updates`

The update BASE CONTRACT: the JSON shape of an update announcement the
control plane broadcasts, and of a node's response to it. Deliberately data
and validation only — no downloader, supervisor, or handler registry, since
Robot Dreams is agnostic about a node's runtime; the apply procedure is
documented guidance (`docs/updates.md`), not code.

## Implementing a backend

To add a backend for `messaging` or `storage`:

1. Implement the interface (`messaging.MessagingBackend` or
   `storage.StorageBackend`) in a new package under `pkg/messaging/` or
   `pkg/storage/`.
2. Register a constructor for your URI scheme, typically from the new
   package's `New` (see `messaging.Register` / the storage equivalent), so a
   blank import of the package makes the scheme available via
   `messaging.New` / `storage.New`. Don't import the new backend from the
   abstraction package itself — that would couple the registry to a
   concrete backend.
3. Run the shared conformance suite against it in your own test, following
   the pattern in `pkg/messaging/embedded/embedded_test.go`:

   ```go
   func TestConformance(t *testing.T) {
       testsuite.RunConformance(t, func() messaging.MessagingBackend {
           backend, err := New(/* fresh, isolated instance per call */)
           if err != nil {
               t.Fatalf("New: %v", err)
           }
           return backend
       })
   }
   ```

   The factory must return a fresh instance each time it's called — the
   suite runs multiple subtests and expects them to be isolated. See
   `pkg/messaging/webhook/webhook_test.go` for a backend that needs more
   setup (an HTTP test server) around the same pattern.
