# dream CLI — design notes

## process entry
- `newRootCmd` sets `SilenceUsage` so a runtime failure (server refused, no identity, network down) prints the error alone; cobra still prints usage for flag/argument errors, which are reported before `RunE`.
- `main` wires one `signal.NotifyContext` for every command so Ctrl-C cancels an in-flight HTTP call instead of killing the process mid-write; server, dashboard and simulate build their own and are unaffected.
- `dream version`'s plain line is byte-stable (`TestVersionCmd` asserts the exact string): scripts parse it and `dream self-update` runs it against a freshly downloaded binary as a smoke test.
- `devVersion` duplicates `version`'s compile-time default so `selfupdate.Detect` can tell a goreleaser-stamped build from a local one by comparing the two.
- `buildChannel` is an explicit ldflags marker (`.goreleaser.yaml`) rather than inferred from `version != devVersion`, because a wrong inference means a binary that overwrites itself when it may not.

## env
- Precedence everywhere is explicit flag > environment variable > local auto-discovery; `serverAddrOrEnv` implements the flag-beats-env half, the auto-discovery half lives in the worker/target resolution.
- `selfUpdateDefault` treats anything but a recognized truthy value as off: the safe reading of an ambiguous `DREAM_SELF_UPDATE` is "do not replace the binary".

## HTTP client
- `serverIDFromAddr` derives the `~/.dream/<server-id>/` directory from `--server` (scheme stripped, unsafe chars to `_`) because there is no server-minted server ID; the same address always maps to the same directory.
- The identity key is per server, not per worker: several workers on one machine against one control plane share one keypair (`identity.DefaultKeyPath(serverID)`). Known gap, deliberate for simplicity.
- `resolveTarget` with no selector accepts exactly one `config.json` under `~/.dream`; zero or several is an error rather than a guess, so a command never authenticates as the wrong worker.
- `fetchToken` stamps `IssuedAt` client-side at receipt because the server does not echo it; a zero `IssuedAt` makes `security.WorkerClient` treat every token as stale and busy-loop `POST /api/token`.
- `clientTarget.oneShotClient` runs one handshake and no refresh loop: commands that make one or two calls exit long before the token TTL matters; only `dream message tail --follow` uses `tokenSourceFor` with a `WorkerClient`.
- `defaultHTTPClient` carries `requestTimeout` (30s); the SSE path builds its own client without a timeout.
- `storagePointer` has no JSON tags on purpose: it mirrors `messaging.StoragePointer`, which serializes with Go field names.

## dream server
- `defaultServerDataDirName` is `_server` because worker state lives in `~/.dream/<host:port>/`; a leading underscore can never collide with a host-derived name, and everything stays under one root.
- `runServerInit` defers `srv.Close` first so it runs last: the scheduler wait and request drain registered after it still use the backends, and closing SQLite under a live handler is the failure this ordering rules out.
- `runServerInit` derives every request context from `reqCtx` (`http.Server.BaseContext`), not the listener's background context, so `drainThenCancelRequests` can end the SSE streams that would otherwise stall `Shutdown` for the full `shutdownGrace`.
- `drainThenCancelRequests` gives ordinary requests `shutdownDrain` undisturbed, then cancels `reqCtx` to release streams; `Shutdown` returns as soon as those handlers exit, bounded by `shutdownGrace`.
- `readHeaderTimeout` closes the Slowloris gap; `ReadTimeout`/`WriteTimeout` stay unset on purpose because `/api/events` and `/api/messages/subscribe` are long-lived by design. `idleTimeout` reclaims keep-alive connections and never touches a stream.
- The scheduler goroutine is awaited (`schedDone`), not just canceled, before `srv.Close`, since a fire in progress writes through the backends `Close` releases.
- `checkTransportSecurity` loads the TLS pair before serving so a typo fails with a clear message rather than inside `ServeTLS` after the banner printed.
- `mintLocalAdminToken` closes its short-lived `*server.Server` before returning so two processes' handles on the same SQLite files overlap for as little time as possible; `adminTokenTTL` only needs to survive one round trip.
- `runServerRotateEnrollment` refuses a directory without `server.ServerKeyFileName`: rotating into an empty directory would create an orphan token no server reads.

## dream dashboard
- `runDashboard` (standalone) exists for a control plane reachable only over the network; in-process `dream server init` mounts the same handler via `withDashboardMounted` with an empty apiBase so `app.js` calls `/api/*` relative to the serving origin and needs no cross-origin setup.
- `dashboard.New` takes an empty `apiBase` for the in-process mount (same origin) and a non-empty one only for standalone `dream dashboard --server`, where the dashboard's origin is not the control plane's.
- `dashboard.New` panics on a missing embedded subtree: `webFS` is compiled in, so its absence is a build-time programming error, not a runtime condition a caller can recover from.
- `dashboard.Mount` wraps `Handler` in `http.StripPrefix` so `cmd_server.go` never handles trailing-slash mechanics itself.

## dream worker
- `runWorkerConnect` defaults `--worker-id` to the hostname: stable across restarts (so reconnecting reuses the same identity) and unique enough per box; anything random would mint a new identity per run and litter the org chart.
- `declareApp` is a separate call after registration for the same reason `declareVersions` is (docs/updates.md): the connect body is decoded with `DisallowUnknownFields`, so a new key would break every newer CLI against every older server.
- `declareVersions`/`declareApp` failures warn on stderr and never fail the connect: a node that cannot advertise a version or app is still a working node.
- `discoverLocalEnrollmentToken` returns `""` (no error) for a missing or unreadable file, so the server's 401 tells the operator to pass `--admin-token` rather than the CLI failing an open-enrollment dev server locally.
- `runWorkerAppSet` validates the URL with `server.ValidateAppURL` before dialing, so a typo is reported next to the flag; `TestWorkerAppSetRejectsBadURLBeforeDialing` pins it.
- `dream worker edit` is a field-agnostic verb rather than a `set-role` command: role is the first editable field, not the last, and a second verb per field would make the surface grow without teaching an operator anything new.
- `dream worker delete` takes an admin token instead of resolving a local worker identity like its siblings, because the server requires the admin scope for it: a delete revokes, and no worker identity — not even the target's parent — can authorize that.
- `discoverLocalAdminToken` mints a token from the server's data dir (`mintLocalAdminToken`, as `dream server revoke` does) rather than reading `enrollment.token` the way `discoverLocalEnrollmentToken` does: an enrollment secret authorizes registration only, so it would be refused by an admin-scoped endpoint.
- `discoverLocalAdminToken` checks for `store.DBFileName` before minting, because `server.New` creates a data dir and signing key when none exists: without the check, a loopback address with no local control plane mints a token signed by an unrelated fresh key, and the operator reads `invalid token` instead of being told no credential was found.
- `runWorkerEdit` reads `Flags().Changed("role")` instead of testing for an empty string, because `--role ""` is a real instruction (clear the label) and must not be confused with not passing the flag at all.

## dream onboard
- `printOnboardBrief` is written for an AI agent as the reader; every command it prints must match this package's real flag set, and the same rule is enforced mechanically for template briefs by `TestTemplateBriefsOnlyPrintRealCommands` and `TestTemplateBriefFlagsExist`.
- `runWorkerOnboard` treats an unreachable server as a warning in the brief and a missing `DREAM_URL` as a hard error: the brief stays correct before connectivity exists, but cannot name a server it does not know.
- `checkServerHealth` is bounded by `onboardHealthTimeout` (5s) so an unreachable server degrades into the warning quickly instead of hanging the brief.

## template library
- `templates.List` panics on a malformed embedded template; `TestEveryTemplateLoads` turns that into a build failure here rather than a panic in a user's terminal.
- `templates.Get` lists the available names in its error so a typo answers itself (`TestGetUnknownNamesTheAlternatives`).
- `templates.load` rejects separators and `..` in a name because `--template` is user input even though it indexes a read-only embedded FS (`TestGetRejectsPathTraversal`).

## dream message
- `streamEvents` parses SSE framing inline (a `data:` prefix check per line) rather than with a library, because the server emits exactly one `data: <json>` line per event; other fields and malformed payloads are skipped.
- `envelopeHandler` returning an error is how `streamEvents` stops and how `updateRequest` breaks `runMessageFollow`'s reconnect loop; there is no side channel.
- `runMessageFollow` keeps an `inFlight` WaitGroup even though handling is synchronous, so the moment envelopes are dispatched to goroutines "let in-flight work finish" is already honored before an update.
- `runMessageFollow` dials with a timeout-free `streamingClient` because a streaming connection has no meaningful response deadline; token refresh runs via `security.WorkerClient` for reconnects.
- `runMessageFollow` reconnects after `followReconnectDelay` when the stream ends without cancellation (server restart, proxy timeout), reusing whatever token the refresh loop holds.
- `runMessageSend` sends `storagePointer` (httpclient.go) for `--storage-path` because it mirrors the server's untagged `messaging.StoragePointer` and so serializes with the Go field names the API expects; `--storage-revision` alone is rejected client-side.

## dream updates
- `updateRequest` implements `error` so the follower's update trigger travels out through `streamEvents` and ends the reconnect loop instead of needing a side channel.
- `updateRequestFor` matches exactly what any node runtime would: sender `server.ControlWorkerID`, subject `updates.SubjectUpdateAvailable`, body kind `robotdreams/cli`; other kinds are ignored by the CLI and left to the node.
- `updatesReportOptions` fills `--current-version` with the binary's own `version` for `robotdreams/cli`, the reason that kind is reserved.
- `printRollout` orders counts by `rolloutStatusOrder` (applied first, unknown last) rather than map order, so the headline reads the same on every run.

## dream schedule

## dream storage
- `storageLsPath` always emits the `prefix` query parameter, even empty, because the server branches on its presence (not value) to tell "list everything" from "fetch one object"; `TestStorageLsPathAlwaysCarriesPrefix` guards it.

## dream simulate
- `simulateOptions.listsUniverses` accepts both `--list-universes` and `--universe list`; the flag is the honest form, the in-band value is kept for compatibility and will collide with a universe actually named "list".
- `runSimulate` validates `--speed` and the universe name before any temp dir or listener exists, so a typo costs nothing and names the choices.
- `startUIDev` starts npm in its own process group (`Setpgid`) so stopping kills the whole npm -> vite tree, not just npm.

## dream guide
- `suggestedServerURLWithScheme` turns a wildcard bind host ("", "::", "0.0.0.0") into the `<this-host>` placeholder because such a host cannot be dialed from another machine; a copy-pasteable lie is worse than a placeholder.
- `serverGuideParams` is plain data (no `*server.Server`) so the same guide prints from a live `server init`, from `dream server guide`, and from tests.
- `runServerGuide` reads the enrollment token only under `--show-token` and best-effort: an unreadable file falls back to printing the path (the secret-handling rationale is in docs/security-model.md).

## output
- `newMessageTailCmd` prints nothing for an empty inbox under `--json` (and "no messages" otherwise) because a parser wants an empty stream, not prose to skip.
- The follower's "an update is available" hint and the self-update narration go to `errOut`, never stdout, so `--json` stdout stays one envelope per line.

## npm launcher
- `npm/scripts/install.js` has zero runtime dependencies (built-in fetch/crypto/fs plus the system `tar`, which is bsdtar on macOS and Windows 10+ and also reads zip); `bin/dream.js` calls it lazily when postinstall was skipped.
- `downloadAsset` uses the public download URL anonymously and the GitHub API (name -> asset id) with `GITHUB_TOKEN`/`GH_TOKEN`, the only route that serves private release assets; `internal/selfupdate.GitHub` mirrors it.
