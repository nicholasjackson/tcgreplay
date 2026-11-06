Decided by the spaces plan:
- An optional `Local() Store` on tiered stores, found by type assertion; the
  Store interface does not grow.
- Bucket layout: card data under `tcgdex/` at the disk relative paths
  (tcgdex/sets.json, tcgdex/sets/<safe id>.json, tcgdex/cards/<safe id>.json),
  private, application/json. Images must use a different prefix.
- `internal/spaces` is the shared client: Config, FromEnv, New, Get, Put with
  PutOptions (content-type, cache-control, public-read), ErrNotFound, typed
  *Error, VerifyRequest. Later specs add Delete and List on the same signer.
- `internal/spaces/spacestest` is the shared fake S3.
- Settings: TCGREPLAY_SPACES_ENDPOINT, _BUCKET, _KEY, _SECRET; environment
  only, hosted only. None set: local-only with one log line. Partial: a
  warning, then local-only.
- Breaker: 3 s timeout, 1 minute skip, edge-triggered logs.
- HUMAN task: create the Spaces bucket and set its secrets in App Platform.
- Manual checks: the 2 s first-import metric; real Spaces end to end
  (import, redeploy, import); real failures (wrong secret, unreachable);
  hosted start with no settings logs a single local-only line.
- Open questions: the freshness seams; real Spaces 403 vs 404 for missing keys.
