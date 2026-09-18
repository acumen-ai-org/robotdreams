# Storage — design notes

## Interface and registry

- Concurrency control is optimistic: `PutOptions.IfMatchRevision` replaces a lease or lock service; callers retry against the latest revision on `ErrRevisionMismatch`.
- A failed `IfMatchRevision` check leaves the stored object unchanged.
- `Revisions` order is implementation-defined and history may be bounded; each backend documents its retention.
- `ObjectMeta.UpdatedBy` is stored verbatim from `PutOptions.UpdatedBy`; a backend never infers it from the process user or connection credentials.
- `storage.Register` panics on a duplicate scheme so two backends claiming one scheme fail at init instead of one silently shadowing the other.
- `pkg/storage` never imports a backend; each backend registers itself in `init()`, so a blank import is what makes a scheme available to `storage.New`.
- `testsuite.RunConformance` calls `factory` once per subtest so no subtest observes another's state.

## localfs

- `resolvePath` returns the canonical relative path and every metadata operation keys on it, because `victim.txt` and `./victim.txt` are one file and keying on the raw string would let one spelling bypass the other's `IfMatchRevision` (`TestPathSpellingsShareRevisionHistory`).
- `resolvePath` rejects the `.meta` name outright because a write there could forge a revision record's `UpdatedBy` or hide content from `List` (`TestReservedMetaPathRejected`).
- `isUnderRoot` re-checks the joined path after cleaning as a second guard against escaping root.
- The `file` dispatcher accepts the path from `u.Path`, `u.Opaque` or `u.Host` so tolerant forms like `file://relative/path` work.
- `Get` reads the whole file under the mutex so content and metadata come from the same revision; `Close` is a no-op because nothing outlives a call.
- `statLocked` prefers the latest revision record (which carries `UpdatedBy`/`UpdatedAt`) and falls back to mtime plus a content hash for files created out of band.
- Revision records are numbered `%08d.json` files under `.meta/<sha256(path)>/`, appended and pruned to `maxRevisionHistory` on each Put, so `Revisions` is O(maxRevisionHistory).
- Old revision records with JSON keys for fields that no longer exist still decode, since encoding/json ignores unknown fields; no migration is needed.
- `Health` creates and removes a probe file to prove root is writable, not merely present.

## s3compat

- `defaultRegion` is `us-east-1` because the SDK requires a non-empty region even when an explicit endpoint ignores it.
- `access-key`/`secret-key` URI query parameters are a dev-only static-credential fallback for local MinIO; otherwise `LoadDefaultConfig`'s credential chain applies.
- `versioningEnabled` is read once in the constructor and affects only `Revisions` and the sidecar bookkeeping.
- `resolveKey` also rejects empty segments, unlike localfs which cleans them, because S3 keys are literal strings.
- `pathFromKey` strips the configured prefix so callers never see bucket-level keys.
- Sidecars live at `<prefix>/.meta/<sha256(key)>.json` so two backends on different prefixes of one bucket never collide; `isUnderMetaDir` matches by segment so `.metadata` is not skipped.
- A sidecar is one JSON array rewritten unconditionally on each Put; a lost race degrades only `Revisions`, never the CAS, which rests on `If-Match`.
- When versioning is enabled `Put` skips the sidecar, since `ListObjectVersions` gives exact history.
- `Delete` HEADs first because S3 `DeleteObject` succeeds on a missing key; the sidecar delete is best-effort.
- `isNotFound` and `isPreconditionFailed` match typed SDK errors, API error codes and raw HTTP status, because providers disagree on which shape they return.
- `UpdatedAt` travels as user metadata from the backend clock rather than server `LastModified`, matching localfs's use of its injected clock; extra keys written by older versions are ignored on read.
- `listObjectVersions` fills `UpdatedBy` only on the latest version, from the HeadObject already made, because one HEAD per version is not worth it for a history listing.
- `Health` is `HeadBucket`, a permission and reachability check that transfers no data; `Close` is a no-op because the SDK client pools its own connections.
