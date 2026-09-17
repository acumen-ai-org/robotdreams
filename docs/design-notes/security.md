# Security — design notes

## Tokens

- `TokenSource` is only "fetch a token"; proof of possession (signing the server nonce) belongs to the implementation, so tests can use a deterministic fake.
- `Claims.WorkerID` duplicates `RegisteredClaims.Subject` as an explicit alias, so consumers need not know the JWT subject convention.
- `Claims.DelegatedBy` names the parent that minted a child token; a child has no keypair and no org-chart entry, only scopes bounded by its parent's.
- `Claims.HasScope` matches literally; wildcard scopes (`storage:read:path/*`) are interpreted server-side until a client needs more.
- `Token` carries `ExpiresAt` and `IssuedAt` beside `Raw`, so the client manages lifecycle without re-parsing the JWT.

## Worker client

- `WorkerClient.Token` returns an `atomic.Pointer` snapshot, so a reader never sees a half-installed token during refresh.
- `WorkerClient.Start` is idempotent; a failed initial fetch resets `started` so the caller can retry.
- `refreshLoop` waits `MinRefreshInterval` after a failed refresh and retries indefinitely rather than giving up or spinning; `onRefreshError` is the test/observability hook.
- `nextRefreshDelay` measures `RefreshFraction` from `Token.IssuedAt`, not install time, so a slow fetch does not shorten the margin.
- `Clock` exists so TTL and expiry logic is testable with a fake clock; `RealClock` is the nil default in `NewWorkerClient`.
- The concurrent-reader test paces readers (`readerPauseSoRefreshGoroutineGetsCPU`) because a busy spin starves the refresh goroutine on small CI runners.
