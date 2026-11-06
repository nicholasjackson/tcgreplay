---
created_date: "2026-11-04"
document_status: final
closed_date: "2026-11-04"
---

# Card cache freshness

## What was built

The playmat's TCGdex card database now refreshes its cached data on its own and never stalls the board while it does.

- **A cache store with fetch times.** Card data is read and written through a small `Store` interface (`internal/cards/store.go`). The disk store keeps the existing file layout (`sets.json`, `sets/<id>.json`, `cards/<id>.json`) and writes each entry atomically as an envelope, `{"fetched": <time>, "data": <TCGdex body>}`. Files written by earlier releases are read as-is, with their modification time as their fetch time. This is the seam the next spec in the epic uses to add a DigitalOcean Spaces tier.
- **Refresh by kind.** The set list goes stale after 30 days, a set's card list after 90 days, and a card without artwork after 24 hours; a card with artwork never does. Stale data is served at once and refreshed in the background; a refresh that fails or takes longer than 5 seconds keeps the old copy and is not retried for 5 minutes. Offline mode never contacts TCGdex and serves everything it has.
- **Misses checked against fresh data.** A deck code naming a set the cached list lacks, or a number missing from a cached set, refreshes that data before answering, at most once per distinct miss every 5 minutes, so new sets and promos resolve on the first import.
- **One request per thing, failures remembered.** Simultaneous requests for the same data share one TCGdex request (an in-package single-flight on a detached, 5-second-bounded context). A card TCGdex fails to return is remembered for 5 minutes, so repeated lookups answer at once.
- **An in-memory catalog built off the lock.** The set-code index and the by-name index are now one immutable catalog, built without holding the lock card lookups use and swapped in whole, rebuilt whenever a refresh changes the set list or a set. By-name lookups follow new sets without a restart. The persisted `set-index-v2.json` is retired (ignored if present). Print preference and set-code collision rules are unchanged; against a real previous-release cache the rebuilt set-code index matched the persisted one exactly.
- **No card fetches under the game lock.** `Game.card` uses only cached data and warms a missing card in the background, so a slow or failing card never freezes board reads or plays.
- **Tests.** An `httptest` fake TCGdex with request counts and an injected clock drive behavioural tests for every acceptance criterion in `internal/cards`, plus game-level tests for the lock and an end-to-end manager test with a stale cache and a dead source. Knowledge entries `gotchas/tcgdex-cache-never-expires.md` (now "The TCGdex cache refreshes by kind") and `gotchas/tcg-live-log-card-codes.md` were updated.

## Why it matters

New sets, new promos and newly added artwork now appear for desktop and hosted players without anyone clearing the cache, the last good copy is kept when TCGdex is down or slow, and card lookups no longer freeze the board or retry a failing card on every read. Offline use works as before.

## Deviations from the plan

- `loadEntry` decodes into the caller's value and a fetched body is stored only once it decodes as its kind, preserving the old guarantee that a malformed response never lands in the cache.
- Failed-card memory is recorded inside the shared fetch, so a caller giving up on its own context is never counted as a failure of the card.
- `remember` keeps whichever in-memory copy was fetched later, so a lookup reading an older disk copy cannot overwrite a freshly refreshed card.
- Catalog updates merge by fetch time under a small writer mutex held for CPU work only, so a miss refresh and a background sweep racing each other keep the newest data. A miss refresh of the set list also marks the list as freshly attempted so an overlapping sweep does not fetch it again. A miss whose refresh fails is not retried for 5 minutes either.
- An alias set code still resolves without building the catalog; a failed catalog build is not cached.
- The end-to-end test uses a source that drops every connection (so requests can be counted) and adds a slow-source case.
- The cache knowledge entry keeps its historical path; only its title and body changed.
- The browser e2e suite was not run: its board test server uses the in-memory fake card pool, whose cached lookups never miss, so the change cannot affect it.

## Known issues seen during implementation (not caused by this change)

- `cmd/tcgreplay` `TestHostedGuidesMatchDesktop` fails on `main` at 8c26660 (the desktop lists 7 guides, the test expects 6).
- `internal/mcpserver` `TestGuideInstructions/*/inmemory` is flaky (TempDir cleanup "directory not empty").
- One `go test -race ./internal/game ./internal/cards` run reported an `internal/game` failure whose output was not captured; it did not reproduce in nine further runs.
