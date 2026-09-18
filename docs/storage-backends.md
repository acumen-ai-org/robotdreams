# Storage backends

`pkg/storage.StorageBackend` provides `Get`, `Put` (with
`PutOptions.IfMatchRevision`/`UpdatedBy`), `Delete`, `List`, `Stat`,
`Revisions`, `Health`, `Close`. `ObjectMeta` carries an opaque `Revision`
identifier (two writes with different content must never produce the same
revision for the same path) and `UpdatedBy`, the identity of the writer.
A conditional write that fails its `IfMatchRevision`
check returns `storage.ErrRevisionMismatch` — this is how the system
avoids silent overwrites without a separate lease/lock service.

Select a backend with `--storage <uri>` on `dream server init`, or leave
it unset for the zero-config default. The chosen URI is persisted in the
data directory.

## Local filesystem — `file:///<path>` — default

The reference/development backend, and the zero-config default
(`file://<data-dir>/storage`). Object content mirrors each object's path
as a file under the root directory.

```
file:///var/lib/dream/storage
```

- Writes are atomic: content is written to a temp file in the same
  directory and renamed into place, so a crash mid-write never exposes
  partial content.
- Revision is the SHA-256 hex digest of the object's content.
- A single mutex guards the check-then-write critical section for every
  `Put`, so `IfMatchRevision` checks are genuinely TOCTOU-safe — this
  trades some write throughput for correctness, which is the right
  tradeoff for a single-process local backend.
- Path handling rejects any object path that is absolute or contains a
  `".."` segment outright (rather than silently clamping it), so `Get`,
  `Put`, `Delete`, `Stat`, and `Revisions` can never read or write outside
  the backend's root.
- Revision history is kept in a sibling `.meta/<sha256(path)>/` directory,
  bounded to the most recent 10 revisions per path (`maxRevisionHistory`)
  — older records are pruned on write, a deliberate bound against
  unlimited growth rather than full history retention.
- `UpdatedBy` is recorded on every write, from `PutOptions.UpdatedBy`.
  The control plane sets it from the authenticated worker on the request;
  a direct library caller passes it itself, and a backend never infers
  it.

## S3-compatible — `s3://<bucket>/<prefix>?endpoint=...&region=...&use-path-style=...`

Works against AWS S3, MinIO, Cloudflare R2, or any S3-compatible store,
via AWS SDK for Go v2 and an explicit endpoint override.

```
s3://my-bucket/robotdreams?endpoint=http://127.0.0.1:9000&region=us-east-1&use-path-style=true
```

Query parameters: `endpoint` (override, needed for MinIO/R2/non-AWS
endpoints), `region`, `use-path-style` (needed for most non-AWS
S3-compatible servers).

**Credentials**: for local development, access-key/secret-key can be
supplied directly (check the backend's connection options / environment
variables used in your deployment — e.g. `AWS_ACCESS_KEY_ID` /
`AWS_SECRET_ACCESS_KEY`). This dev-only fallback exists for convenience
against a local MinIO instance. **For production, use the standard AWS
credential chain** (IAM role, instance profile, `~/.aws/credentials`,
environment) instead of static keys embedded in configuration.

`UpdatedBy` and `UpdatedAt` travel as object user metadata (`rd-updated-by`,
`rd-updated-at`), so a `Stat` or `Get` (a HeadObject/GetObject) returns the
full record. `List` uses `ListObjectsV2`, which carries no user metadata:
listed entries therefore have no `UpdatedBy`, and their `UpdatedAt` is the
server's `LastModified`. `Stat` the path when you need the complete record.
The same applies per version to `Revisions` on a versioned bucket, except
for the current version, whose record is already at hand.

### Revision semantics (from the package's own doc comment)

This backend uses the object's **ETag** uniformly as `Revision`, for both
versioned and unversioned buckets — not `VersionId`, even though S3 offers
both. This is deliberate: S3's native conditional-write headers
(`If-Match`/`If-None-Match`) compare against ETag, not `VersionId`, and
every write here is a single-shot `PutObject` (never multipart), so ETag
is effectively a content hash — the same guarantee localfs gets from
SHA-256. Bucket versioning, when enabled, is still put to use for a more
accurate `Revisions()` (via `ListObjectVersions`) than the bounded
sidecar history this backend otherwise maintains by hand.

`Put` implements `IfMatchRevision` as: (1) `HeadObject` to read the live
ETag, (2) fail immediately with `ErrRevisionMismatch` if it doesn't match
the caller's expected revision, (3) otherwise issue `PutObject` with an
`If-Match` header set to the just-observed ETag.

**Step 3 is the load-bearing correctness measure, not step 1.** Classic
AWS S3 didn't support conditional PUT until 2024, and some
S3-compatible servers may still silently ignore an `If-Match` header they
don't recognize rather than reject the request. Where the endpoint honors
`If-Match` (recent AWS S3, recent MinIO, most actively maintained
S3-compatible stores), the check-then-write is genuinely atomic
server-side: a concurrent writer causes a 412 Precondition Failed, mapped
to `storage.ErrRevisionMismatch`. **Where the endpoint silently ignores
`If-Match`**, only the application-level check-then-write remains, and it
is **not** atomic — a real, documented TOCTOU window bounded by one
HEAD+PUT round trip, inherent to any lock-free multi-client store without
confirmed conditional-write support. This is a genuine gap versus
localfs's mutex-guarded atomicity, not specific to this implementation.
