# Self-update — design notes

## Package scope

- `internal/selfupdate` lives under `internal/`, not `pkg/`, because it is CLI-private machinery with no interface third parties implement against.
- `GitHub` is a deliberate close port of `npm/scripts/install.js` (same URLs, redirect and auth behaviour), because divergence between the two install paths surfaces for only one of them.

## Install detection

- `classify` applies ordered rules, first match wins: npm (hint or path), then non-release build (`go install` dir vs source), then release, which self-manages only with a resolved path and a writable directory.
- `Install.Reason` distinguishes "the npm launcher declared it via `InstallHintEnv`" from "path looks like npm's layout", so a refusal names the evidence that applied.
- `isNPMPath` matches `node_modules/robotdreams/dist/` exactly, with `underNodeModules` as a weaker fallback; a hand-copied release binary under `node_modules` errs toward refusing, the safe direction. Yarn PnP is unsupported by `install.js` too.
- `classify` decides on `version == devVersion` plus the executable's directory, never on `debug.BuildInfo`: Go 1.24 stamps a VCS version on plain `go build` too, so `bi.Main.Version` does not separate `go install` from a local build. `reasonForSource` uses `vcs.revision` only to sharpen the message.
- `Detect` treats an `EvalSymlinks` failure as non-fatal but downgrades a release build to `MethodUnknown`, since Windows aliases and Developer-Mode symlinks can fail there.
- `writableDir` probes by creating and removing a temp file rather than reading mode bits, which lie under ACLs, root (writes a 0555 dir) and Windows.
- `npmFix`/`goInstallFix` take the target version, filled in by `Run` via `withTargetVersion`; `classify` only knows the installed version and must not tell the user to reinstall what they have.

## Run

- `Run` never re-execs; the caller decides (one-shot `dream self-update` exits, a follower drains first).
- `Run` treats `Compare` errors as best-effort: a dev build's version does not parse, and that must not block an explicit update.
- `lock` is an `O_EXCL` file next to the binary, taken over after `lockStaleAfter`; a real flock would be Unix-only and does not warrant build tags.
- `ErrRefused` is an error rather than a quiet no-op so a script cannot mistake "did not update" for "updated".

## Download and verify

- `GitHub.Latest` uses `/releases/latest`, which excludes drafts and prereleases, so a release candidate never triggers a fleet-wide self-update.
- `GitHub.DownloadAsset` hashes in the same pass as the write, capped at `maxArchiveBytes`, so the archive is never held in memory and cannot fill the disk before the checksum is checked.
- `githubTimeout` bounds API calls only; the asset download runs on the caller's context because a large archive on a slow link is not an error.
- Go's `http.Client` drops `Authorization` on a cross-host redirect exactly as Node's `fetch` does, so the hop to object storage matches `install.js`.
- `ExtractBinary` is stdlib-only (no `tar` subprocess), so the release binary keeps zero platform dependencies; `install.js` has no such choice.
- `safeEntryName` accepts only a regular file named exactly `binaryName` at the archive root; separators, `..`, symlinks, hardlinks and devices are rejected, not sanitized, because the archive has exactly one shape.
- `writeBinary` calls `Chmod(0o755)` explicitly because the `OpenFile` mode is masked by umask and a binary without its execute bit is useless.
- `ChecksumFor` parses sha256sum lines (`<hex><space or *><name>`); the asterisk is binary mode. Mirrors `checksumFor` in `install.js`.

## Apply

- `Applier.Apply` copies the original's permission bits onto the staged binary so a restrictive install (0750) is not widened to 0755 (`TestApplyPreservesPermissionBits`).
- `renameFn` indirects `os.Rename` so `TestApplyRollsBackWhenInstallFails` can make the second rename fail deterministically.
- `smokeTestTimeout` bounds the staged binary's `version` run; `WantVersion` guards against a release that served the wrong tag.
- `CleanupBackup` removes `backupSuffix` just before re-exec (the running process still holds its inode) and best-effort at startup, sweeping up after a crash between the two renames.
- `Restart` uses `syscall.Exec`, preserving PID, argv, cwd and fds, so a supervisor tracking the process it started is not orphaned.

## Version arithmetic

- `Compare` is ~60 hand-written lines rather than a dependency: it applies to exactly one kind, the dream binary. The control plane must not reach for it; `pkg/updates` treats versions as opaque strings.
- `parseVersion` requires exactly three numeric components so a date-shaped version like `2026-09-01-g1a2b3c` is rejected rather than read as major=2026 (`TestCompareRejectsNonSemver`).

## `dream self-update` command

- `self-update` is one command with `--check` rather than a group, because it has one action and one query; `server` and `worker` are groups because they have three or more verbs.
- `runSelfUpdate` returns an error on refusal so the process exits non-zero, and reports a failed re-exec as an error even though the new binary is installed, so a supervisor restarts it.
- `applyAnnouncedUpdate` reports `applied` and acks before `Restart`, because after `syscall.Exec` this process is gone and the new one does not know it was mid-rollout.
- `reportUpdateProgress` swallows failures: reporting is observability, and the node that cannot reach the control plane is the one most in need of the newer binary.
