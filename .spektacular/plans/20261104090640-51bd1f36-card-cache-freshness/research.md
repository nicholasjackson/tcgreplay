---
created_date: "2026-11-04"
document_status: final
closed_date: "2026-11-04"
---

# Research: 20261104090640-51bd1f36-card-cache-freshness

## Alternatives considered and rejected

- **Fetch time from file modification time only (no recorded fetch time).** Set `os.Chtimes` to the fetch time on write and read age from mtime; keeps the raw TCGdex body format so an older release can still read the files. Rejected: the spec's Technical Approach says to record each entry's fetch time so it survives "later copying between cache tiers", the spec's "Existing caches carried over" requirement treats mtime only as the fallback for entries "with no recorded fetch time", and the next spec (`tcgdex-spaces-cache`, Technical Approach) says a file's modification time alone does not survive copying to the shared store.
- **Sidecar metadata files (`cards/<id>.json.meta`).** Rejected: two files per entry can be copied or deleted apart, and a Spaces tier would need two objects per key. An envelope keeps the body and its fetch time together.
- **Merging a `_fetched` field into the TCGdex object.** Downgrade-safe for objects, but the set list is a JSON array, so it needs a second format anyway. Rejected for one uniform envelope.
- **Periodic background sweep of the whole cache on a timer.** Rejected: the spec asks for stale-while-revalidate (refresh when a lookup touches stale data); a timer would contact TCGdex with no player present, and keeps goroutines alive the desktop build does not need.
- **Keep persisting `set-index-v2.json` with its own expiry.** Rejected: it is derived from the set list and set card lists, so a separately aged copy can disagree with its inputs (the exact bug in `gotchas/tcgdex-cache-never-expires.md`: rebuilding the index refetched only `sets.json`). The set index and the by-name index are both rebuilt in memory from the set entries (pure CPU once the set files are read).
- **Keep holding `Client.mu` across `buildIndex` / `buildNameIndex` and only add expiry.** Rejected: `Resolve` (`internal/cards/cards.go:475-476`) and `prints` (`cards.go:558-559`) hold `c.mu` while fetching every set, and `Cached` (`cards.go:268`) takes the same lock, so every board read (`Game.View` → `g.hp` → `Cached`, `internal/game/reads.go:277-289`, `game.go:500-515`) stalls for the whole cold build. This is the "No stall during a cold build" failure.
- **`golang.org/x/sync/singleflight`.** Viable, but adds a module for ~30 lines; a small in-package flight group (with waiters selecting on their own context) is used instead. Not a hard rejection.
- **Keep `Game.card` blocking on `db.Card` for up to 5 s under the game lock.** Rejected: `Game.card` (`internal/game/game.go:481-497`) is called with `g.mu` held from reads and verbs (`reads.go:303,463,500`, `verbs.go:85,160,375,404,433,473,505,566,596,730`, `presence.go:381`), so one slow or failing card freezes every other board read and play on that game, and a failing card was retried on every read.

## Chosen approach — evidence

- `internal/cards/cards.go:204-241` — `getJSON` returns any cached file forever; the single point where a freshness check and a store abstraction slot in.
- `internal/cards/cards.go:243-287` — `Card` and `Cached` keep `mem map[string]*Card` with no fetch time; `Cached` reads the disk file on a mem miss and never touches the network (contract in the `DB` interface, `cards.go:155-157`).
- `internal/cards/cards.go:400-467` — `buildIndex` fetches the set list and every set (8 at a time) to map set codes; once-per-process refresh flag `indexed` (`cards.go:486-493`) is the only refresh today.
- `internal/cards/cards.go:589-659` — `buildNameIndex` reads every set and orders prints (art first, newest release, lowest number). The ordering (`better`, `cards.go:601-609`) must not change (spec Non-Goal: matching and print preference unchanged).
- `internal/cards/cards.go:297-309` — `energyArt` gives Basic Energy borrowed art after load; it runs on every decode, so it applies to refreshed cards too.
- `internal/cards/byname_test.go:73-110` — fixture pattern for a cache directory written by the test (`sets.json`, `sets/<id>.json`, `cards/<id>.json`) with `Offline = true`; new tests extend this with an `httptest` fake TCGdex and an injected clock.
- `internal/game/game.go:481-497` — `Game.card`: `Cached` first, then `db.Card` with a 5 s timeout, under `g.mu`.
- `internal/game/verbs.go:818-837` — `PrefetchCards` loads every def outside the game lock (called from `internal/mcpserver/server.go:470,485,497` after import/new game); `BuildCards` (`internal/game/manager.go:204-235`) also calls `db.Card` for every list entry before the game exists. So by the time a game's lock is taken, its cards are normally in memory, and a non-blocking `Game.card` loses nothing in practice.
- `internal/game/import.go:88-103` — the importer's `DefOf` calls `Card`/`ByName` outside any game lock; only the chosen definition depends on the cache, never placement (spec Constraint).
- `internal/webui/server.go:138-165` — `/api/card` calls `db.Card` with a 15 s budget and `Cache-Control: no-cache`; `web/src/lib/store.svelte.js:136-152` memoises card info per page load (`requested` set), so refreshed art shows on the board's next load.
- `cmd/tcgreplay/main.go:224-229` — `newCardDB` is the only construction site; `-offline` sets `Client.Offline`.
- Knowledge `gotchas/tcgdex-cache-never-expires.md` — documents the layout (`sets.json`, `sets/<id>.json`, `set-index-v2.json`, `cards/<id>.json`) and asks for expiry of the set list, set files and imageless cards.
- Next spec `20261104082020-109dceb1-tcgdex-spaces-cache` — adds memory → local disk → Spaces → source tiers and requires the fetch time to travel with each entry; a `Store` interface (Get/Put by key, carrying the fetch time) is the seam.

## Files examined

- `tcgreplay:internal/cards/cards.go:167-190` — `Client` fields (`Base`, `CacheDir`, `HTTP` 20 s timeout, `Offline`, `mu`, `mem`, `setIndex`, `indexed`, `byName`).
- `tcgreplay:internal/cards/cards.go:200-202` — `safeName` sanitises ids into file names.
- `tcgreplay:internal/cards/cards.go:204-241` — `getJSON`: cache-forever read, 404 → `ErrNotFound`, atomic write via `.tmp` + rename.
- `tcgreplay:internal/cards/cards.go:394-398` — `set` loads `sets/<id>.json`.
- `tcgreplay:internal/cards/cards.go:469-513` — `Resolve`: aliases, index, once-per-process rebuild, number match (exact or numeric).
- `tcgreplay:internal/cards/cards.go:532-575` — `ByName` / `prints` (with "basic " prefix fallback).
- `tcgreplay:internal/cards/byname_test.go` — offline fixture tests; `TestByNameCached` runs against the real local cache and skips without one.
- `tcgreplay:internal/cards/cardstest/cardstest.go` — `FakeDB` implements `DB`; used by game and webui tests. Interface stays the same, so it needs no change.
- `tcgreplay:internal/game/game.go:481-516` — `Game.card` and `Game.hp` (hp uses `Cached` only).
- `tcgreplay:internal/game/reads.go:277-289` — `View` holds `g.mu` and projects with `g.hp`.
- `tcgreplay:internal/game/verbs.go:718-740` — `move` infers attach kind from the card's category via `t.g.card`.
- `tcgreplay:internal/game/presence.go:375-385` — `usedCard` reads card text under `g.mu`.
- `tcgreplay:internal/game/manager.go:204-235` — `BuildCards` resolves lists and loads each card before the game exists.
- `tcgreplay:internal/deck/deck.go:134-170` — `List.Resolve` calls `db.Resolve`, then `BasicEnergyDef`, then `ByName` for set-less entries.
- `tcgreplay:internal/game/import.go:88-103` — importer `DefOf`.
- `tcgreplay:internal/game/game_test.go:33-50` — `newTestGame` with `cardstest.New()`.
- `tcgreplay:internal/webui/server.go:138-165` — `/api/card`.
- `tcgreplay:internal/mcpserver/server.go:575-595` — `card_text` tool calls `DB.Card`.
- `tcgreplay:web/src/lib/store.svelte.js:136-152` — board fetches `/api/card` once per def per page load.
- `tcgreplay:cmd/tcgreplay/main.go:123-125,224-229` — `-cache`, `-offline`, `newCardDB`.
- `tcgreplay:Makefile:29-33` — `make test` runs `go vet ./...`, `go test ./...`, then the web e2e suite.
- `tcgreplay:go.mod` — Go 1.27.2; no `golang.org/x/sync`.

## External references

- RFC 5861 (HTTP Cache-Control extensions for stale content: `stale-while-revalidate`, `stale-if-error`) — the semantics the spec's Technical Approach names; used for the serve-stale-then-refresh and keep-stale-on-error rules.
- TCGdex REST API v2 (`https://api.tcgdex.net/v2/en/sets`, `/sets/<id>`, `/cards/<id>`) — the set list carries only ids and names, so set codes need every set's detail; a card's `image` field is absent until TCGdex adds art.

## Prior plans / specs consulted

- Spec `20261104082020-109dceb1-tcgdex-spaces-cache` (next in this epic) — memory → disk → Spaces → source tiers; fetch time stored with each entry; offline never contacts the shared store; importer determinism constraint repeated. Drove the `Store` interface shape (context-taking Get/Put carrying `Fetched`).
- Pre-spec discussion notes `.spektacular/work/card-cache/notes.md` (2026-10-26) — agreed direction: per-entry fetch time, lifetime by kind, stale served while refreshing, a small `Store` interface (get/put by key with fetch time). Lifetimes there (a day / weeks / hours) were superseded by the spec's 30 days / 90 days / 24 hours.
- No prior plan touches `internal/cards`.

## Open assumptions

- TCGdex responses never carry a top-level `fetched` key alongside `data`, so the envelope `{"fetched":…,"data":…}` cannot be mistaken for a raw body. (Cards and set details are objects keyed by `id`; the set list is an array.)
- A Basic Energy print whose art is borrowed by `energyArt` counts as "cached with its artwork" (judged on the card as served), so it is not refetched daily.
- Every request to TCGdex can finish within 5 s on a normal connection (set details are at most a few hundred KB); the 5 s bound is applied to every source request, not only refreshes.
- Reading all set files from local disk to build the in-memory indexes (~200 files) takes well under a second, so dropping the persisted `set-index-v2.json` does not slow warm starts noticeably.
- Games live in memory only (`Manager` doc, `internal/game/manager.go:20-22`), so a game's cards were loaded by `BuildCards`/`PrefetchCards` in the same process before its lock is taken.

## Drafting assumptions

### Chosen direction: freshness policy in the Client above a pluggable Store (architecture)
- **Decision**: Three layers in `internal/cards`: a `Store` interface (Get/Put by key, each entry carrying its fetch time; disk implementation with an envelope format and an mtime fallback for legacy files); a freshness policy in `cards.Client` (per-kind refresh ages, stale-while-revalidate, stale-if-error, per-miss and per-failure 5-minute memory, 5-second source timeout, single-flight per key); and an immutable catalog snapshot (set list, set entries, set-code and by-name indexes) built off-lock and swapped under `Client.mu`. `Game.card` stops blocking on the network.
- **Key design decisions**: freshness decided above the store, so a Spaces tier slots in as another `Store` with no policy of its own; `Client.mu` never held across I/O; derived indexes never persisted; the `DB` interface unchanged so game, webui, mcpserver and `cardstest` need no API change.
- **Rejected**: Expiry inside the store (the policy needs card-level knowledge and would be copied per tier); a minimal mtime-TTL patch of `getJSON` under the current locks (keeps the cold-build stall, no fetch time across tiers). See research.md alternatives.

### Conventions selected (architecture)
- **Decision**: Only `conventions/skills-text-and-packaging.md` is cited, for its packaging rule (do not exclude `internal/guides` from build contexts). No other conventions exist in the store.
- **Rationale**: The plan edits Go code and tests only; no guide text or packaging changes.
- **Rejected**: Listing it as fully applicable (its guide-wording half does not bear on this work).

### Fetch time recorded in an envelope, not file mtime (discovery)
- **Decision**: New cache entries are written as `{"fetched":"<RFC 3339>","data":<raw TCGdex body>}`; a file without the envelope is a legacy entry whose fetch time is its modification time.
- **Rationale**: The spec asks for the fetch time to survive copying between tiers and treats mtime only as the fallback for entries with no recorded time; the next spec says mtime does not survive copying to Spaces.
- **Rejected**: mtime only (lost on copy); sidecar meta files (body and time can separate); merging a field into the TCGdex object (the set list is an array).

### Downgrade after upgrading is not supported (discovery)
- **Decision**: A previous release opening a cache the new release has rewritten would misread envelope files; no downgrade path is built.
- **Rationale**: The spec requires only forward compatibility ("Existing caches carried over"); the cache can always be deleted and rebuilt from TCGdex.
- **Rejected**: A downgrade-safe format (needs two formats, see above).

### Set index no longer persisted (discovery)
- **Decision**: `set-index-v2.json` is neither read nor written; the set-code index and the by-name index are rebuilt in memory from the set list and set entries. An old `set-index-v2.json` is left on disk untouched.
- **Rationale**: Derived data with its own age can disagree with its inputs, the root cause recorded in `gotchas/tcgdex-cache-never-expires.md`. Rebuilding from local files is cheap.
- **Rejected**: Persisting the index with its own expiry.

### "Cached with artwork" judged on the card as served (discovery)
- **Decision**: A card counts as having artwork if its `Image` is set after `energyArt`, so Basic Energy prints with borrowed art are not refetched every day.
- **Rationale**: Their art is deliberately borrowed and identical; a daily refetch of every Energy print would add requests for no visible change.
- **Rejected**: Judging on TCGdex's raw `image` field.

### Every source request bounded at 5 seconds (discovery)
- **Decision**: Each request to TCGdex runs under a 5-second timeout (a `Client` field, shortened in tests), for cold fetches as well as refreshes. A timeout counts as a failure: stale data is kept, and an uncached card is remembered as failed for 5 minutes.
- **Rationale**: The spec's "does not answer" and "takes longer than 5 seconds" need one concrete bound; TCGdex responses are small.
- **Rejected**: Keeping the 20-second client timeout for cold fetches (a dead source would hold an import for 20 s per card).

### In-package single-flight instead of x/sync (discovery)
- **Decision**: A small flight group in `internal/cards` dedupes fetches and index builds by key; waiters select on their own context while the fetch runs on a detached, time-bounded context.
- **Rationale**: Avoids a new module for ~30 lines and lets one caller's cancellation not fail the others.
- **Rejected**: `golang.org/x/sync/singleflight` (viable, but its waiters cannot leave early on their own context).

### Game.card never blocks on the network (discovery)
- **Decision**: `Game.card`, always called with the game lock held, uses only `Cached`; on a miss it starts a background `db.Card` fetch and returns nil (the card is treated as unknown for that call).
- **Rationale**: The spec's Technical Approach prefers network calls outside the game lock, and board reads must not wait on other cards' fetches. A game's cards are already loaded by `BuildCards`/`PrefetchCards` before its lock is taken, so this changes nothing in normal play; advisory warnings and the move verb's attach-kind guess may skip a card whose fetch failed.
- **Rejected**: Keeping a 5 s blocking fetch under the lock.

### Refreshed art reaches the board on its next load (discovery)
- **Decision**: No board (Svelte) change; the board fetches `/api/card` once per card per page load, so late art appears on the next load of the board after the refresh finished.
- **Rationale**: The acceptance criterion says the art appears "on a view after the first lookup's refresh has finished"; a fresh load of the board is such a view, and the spec's scope is the server cache.
- **Rejected**: Pushing card updates to open boards (new board protocol, out of scope).

### Refresh ages as constants, not settings (data_structures)
- **Decision**: The refresh ages and the 5-minute retry window are package constants; only the per-request timeout and the clock are `Client` fields (for tests).
- **Rationale**: The spec fixes the values; no requirement asks for them to be configurable, and adding flags or env vars widens the surface.
- **Rejected**: CLI flags or environment variables for each age.

### Store interface exported with a context (data_structures)
- **Decision**: `Store`, `Entry`, `ErrNotCached` and `NewDiskStore` are exported from `internal/cards`, and `Get`/`Put` take a `context.Context`.
- **Rationale**: The next spec builds a Spaces tier (a network store) and a tiered store, possibly in its own file or package; a context lets a network tier honour timeouts.
- **Rejected**: An unexported interface without a context (the next spec would have to change its signature).

### Source timeout shortened in tests (testing_approach)
- **Decision**: The 5-second bound is asserted through the `Client.Timeout` field set to a few hundred milliseconds against a fake that sleeps longer; production keeps 5 s.
- **Rationale**: Keeps the suite fast while exercising the same code path.
- **Rejected**: Real 5-second sleeps in tests.

### Board-level "source down" covered by a manager test, not an imported battle log (testing_approach)
- **Decision**: The end-to-end "stale served when source down" test builds a game through the manager from decklists with a real `cards.Client`, rather than importing a full battle log.
- **Rationale**: The import path uses the same `Resolve`/`ByName`/`Card` calls; a full log would need fixtures for every card it names.
- **Rejected**: A full battle-log import fixture.

### Interim unknown-code refresh kept until the miss policy lands (tasks)
- **Decision**: The catalog task keeps today's single forced set-list refresh for an unknown code; the set-data task replaces it with the per-miss 5-minute rule.
- **Rationale**: Each milestone stays independently deliverable with no regression in between.
- **Rejected**: Dropping the forced refresh in the catalog task (a new set would stop resolving until milestone 3).

### Game.card change is independent of the cards rewrite (tasks)
- **Decision**: "Keep card fetches out of the game lock" depends on no other task and can run in parallel with milestone 1.
- **Rationale**: It touches only `internal/game` and works against the unchanged `DB` interface.
- **Rejected**: Sequencing it after the cards tasks (needless serialisation).

### Knowledge updates are an agent task in this plan (tasks)
- **Decision**: Updating `gotchas/tcgdex-cache-never-expires.md` and `gotchas/tcg-live-log-card-codes.md` is a plan task, done through `spek-knowledge` after the set-data task.
- **Rationale**: Both entries become wrong once the change lands, and the planning session must not rewrite them before the code exists.
- **Rejected**: Updating them now during planning (they would describe code that does not exist yet).

### By-name misses do not force a refresh (out_of_scope)
- **Decision**: Only unknown set codes and numbers missing from a set refresh before answering; an unknown name does not.
- **Rationale**: The spec's "Misses checked against fresh data" names only those two misses; the name index follows set changes through background refreshes.
- **Rejected**: Refreshing the set list on every unknown name (opponent cards with misspelled or log-only names would each trigger a fetch).

## Rehydration cues

- `spektacular spec file read 20261104090640-51bd1f36-card-cache-freshness` and `spektacular spec file read 20261104082020-109dceb1-tcgdex-spaces-cache`.
- `spektacular knowledge read --data '{"tier":"repo","name":"tcgreplay","path":"gotchas/tcgdex-cache-never-expires.md"}'` and `gotchas/tcg-live-log-card-codes.md`, `gotchas/game-listeners-under-lock.md`.
- Re-read `internal/cards/cards.go` (whole file), `internal/game/game.go:481-516`, `internal/game/verbs.go:818-837`, `internal/game/reads.go:277-289`, `internal/webui/server.go:138-165`, `web/src/lib/store.svelte.js:136-152`.
- `grep -rn --include='*.go' -E '\.(Cached|Card|Resolve|ByName)\(' internal cmd` to list every call site of the card database.
