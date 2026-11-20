---
created_date: "2026-11-04"
document_status: final
closed_date: "2026-11-04"
---

# Context: 20261104090640-51bd1f36-card-cache-freshness

## Current State Analysis

All work is in the one registered repo, `tcgreplay` (root `/home/nicj/code/github.com/nicholasjackson/tcgreplay`).

- `internal/cards/cards.go:204-241` — `getJSON` serves any cached file forever: no fetch time, no expiry, no refresh. Every lookup (`Card`, `set`, the set list) goes through it.
- `internal/cards/cards.go:167-190` — `Client` holds `mem map[string]*Card` (no fetch time), `setIndex`, `indexed` (a once-per-process rebuild flag) and `byName`, all under one `mu`.
- `internal/cards/cards.go:469-513` — `Resolve` holds `c.mu` for its whole body, including `buildIndex` (`cards.go:400-467`), which fetches the set list and every set (8 at a time) on a cold cache and persists `set-index-v2.json`. An unknown code triggers one forced set-list refetch per process; set files are never refetched.
- `internal/cards/cards.go:556-575` and `589-659` — `prints`/`buildNameIndex` hold `c.mu` while reading every set; the by-name index is built once per process and never rebuilt.
- `internal/cards/cards.go:266-287` — `Cached` takes the same `c.mu`, so during a cold build every `Cached` call waits behind the build.
- `internal/game/game.go:481-497` — `Game.card`, called with `g.mu` held from reads and verbs, falls back to `db.Card` with a 5 s timeout, so a slow or failing card stalls the game's lock on every call (no memory of failures).
- `internal/game/reads.go:277-289` — `View` holds `g.mu` and projects with `g.hp`, which calls `Cached` (`game.go:500-515`): the path by which a cold catalog build stalls board reads.
- `internal/game/verbs.go:818-837`, `internal/game/manager.go:204-235` — `PrefetchCards` and `BuildCards` load card definitions outside the game lock before play.
- `internal/webui/server.go:138-165` — `/api/card` calls `db.Card` (15 s budget), served `no-cache`; `web/src/lib/store.svelte.js:136-152` fetches each def once per page load.
- `cmd/tcgreplay/main.go:224-229` — the only construction of the client (`NewClient`, `Offline` from `-offline`).
- Tests: `internal/cards/byname_test.go` (offline fixtures in `t.TempDir()`; one test against the real local cache that skips without one); `internal/cards/cardstest/cardstest.go` (`FakeDB` used by game and webui tests); no HTTP-level tests of the client exist yet.

## Per-Task Technical Notes

### Task: Add the cache store with fetch times

**File changes**:
- `internal/cards/store.go` (new) — `Entry{Data []byte; Fetched time.Time}`, `ErrNotCached`, `Store` interface (`Get(ctx, key) (Entry, error)`, `Put(ctx, key, Entry) error`), `NewDiskStore(dir) Store`. Keys are `sets`, `sets/<id>`, `cards/<id>` with raw ids; the disk store maps a key to `<dir>/<prefix>/<safeName(id)>.json` (and `sets` → `<dir>/sets.json`), reusing `safeName` from `internal/cards/cards.go:200-202` so paths match today's files exactly. `Put` writes the envelope `{"fetched":"<RFC 3339 nano, UTC>","data":<raw body>}` with the same `.tmp` + `os.Rename` atomic write as `internal/cards/cards.go:234-239`, creating parent directories. `Get` reads the file; `os.ErrNotExist` → `ErrNotCached`. Decode: if the first non-space byte is `{`, try `struct{Fetched *time.Time "json:\"fetched\""; Data json.RawMessage "json:\"data\""}`; when both are present it is an envelope. Anything else (arrays such as the old `sets.json`, bare TCGdex objects) is legacy: `Data` = whole file, `Fetched` = `os.Stat(...).ModTime()`. The disk store ignores `ctx`.
- `internal/cards/store_test.go` (new) — round trip keeps body and `Fetched` (compare with `time.Equal` at nanosecond precision); a legacy array file and a legacy object file with `os.Chtimes` set to a known time read back with that time; a missing key returns `errors.Is(err, ErrNotCached)`; a key with `/` or `..` in the id stays inside the directory; the paths written are `sets.json`, `sets/<id>.json`, `cards/<id>.json`.

**Complexity**: Low
**Token estimate**: ~20k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Route card lookups through the store

**File changes**:
- `internal/cards/cards.go:167-190` — `Client` gains exported `Store Store`, `Timeout time.Duration`, `Now func() time.Time`; `NewClient` sets `Store: NewDiskStore(dir)`, `Timeout: 5 * time.Second`, `Now: time.Now`. Add unexported helpers `store()`/`now()` that fall back to these defaults when a field is nil/zero. `mem` becomes `map[string]cardEntry` where `cardEntry{card *Card; fetched time.Time}`.
- `internal/cards/cards.go:204-241` — replace `getJSON` with `loadEntry(ctx, key) (Entry, error)`: `store().Get`; on `ErrNotCached`, return `ErrNotFound` when `Offline` (same message shape as today), else `fetch(ctx, key)` then `store().Put` (ignore a Put error, as today ignores write failures). `fetch` builds the URL from the key (`sets` → `/sets`, `sets/<id>` → `/sets/`+`url.PathEscape(id)`, `cards/<id>` → `/cards/`+`url.PathEscape(id)`), keeps the 404 → `ErrNotFound`, non-200 → error and 16 MB limit, rejects bodies that are not valid JSON, and stamps `Fetched: c.now()`.
- `internal/cards/cards.go:243-287` — `Card` and `Cached` decode `Entry.Data` into `Card`, apply `energyArt`, store `cardEntry` with `Fetched`. `Cached` uses only `store().Get` (never `fetch`).
- `internal/cards/cards.go:394-398` — `set` loads `sets/<id>` through `loadEntry` and also returns the entry's `Fetched` (needed by the catalog later).
- `internal/cards/cards.go:400-467` — `buildIndex`: the forced refetch (`os.Remove(sets.json)` at line 419) becomes a direct `fetch` + `Put` of `sets`; reads go through `loadEntry`. `set-index-v2.json` handling stays for now (retired in "Build the card catalog outside the database lock").
- `internal/cards/cards.go:589-595` — `buildNameIndex` reads `sets` through `loadEntry`.
- `internal/cards/fake_tcgdex_test.go` (new) — `fakeTCGdex` test helper: `httptest.Server` serving `/sets`, `/sets/{id}`, `/cards/{id}` from mutable maps under a mutex; per-path hit counter (`hits(path) int`); knobs: `delay time.Duration`, `fail bool` (500), `gate chan struct{}` that set-detail handlers block on until closed, `close()` to make it unreachable; `newTestClient(t, f)` returns a `Client` on `t.TempDir()` with `Base: f.URL`, a controllable clock (`*fakeClock` with `Advance`), and `Timeout` of ~300 ms. Helpers to seed a cache directory with envelope or legacy files at a chosen age (`seedEnvelope(dir, key, body, fetched)`, `seedLegacy(dir, key, body, mtime)`).
- `internal/cards/store_client_test.go` (new) — an online `Card` fetch writes an envelope whose `fetched` equals the injected clock; an offline client over a mixed legacy/envelope cache resolves `Card`, `Resolve` and `ByName` as the legacy cache did; `internal/cards/byname_test.go` passes unchanged.

**Complexity**: Medium
**Token estimate**: ~45k tokens
**Agent strategy**: 2 parallel agents: one on the `cards.go` rewiring, one on the fake TCGdex helper and its tests; integrate sequentially.

### Task: Deduplicate fetches and remember failed cards

**File changes**:
- `internal/cards/flight.go` (new) — `flightGroup{mu sync.Mutex; calls map[string]*flightCall}`; `Do(ctx, key, fn func(context.Context) (Entry, error)) (Entry, error)`: the first caller starts `fn` in a goroutine on `context.WithTimeout(context.Background(), timeout)` (detached from callers); every caller selects on the call's `done` channel or its own `ctx.Done()`; the call is removed from the map when `fn` returns. Generic enough to also run the catalog build (a second method or a generic `doAny` returning `any`).
- `internal/cards/fresh.go` (new) — `RetryAfter = 5 * time.Minute`; `refreshTracker{failed map[string]time.Time}` (more maps are added in later tasks), with `recent(m, key, now) bool` and `mark(m, key, now)`, guarded by `Client.mu`.
- `internal/cards/cards.go:204-241` (`loadEntry`/`fetch` from the previous task) — every source fetch goes through `c.flight.Do(ctx, key, …)` bounded by `c.Timeout`.
- `internal/cards/cards.go:243-264` (`Card`) — before fetching an uncached card: if `tracker.failed[id]` is within `RetryAfter`, return `fmt.Errorf("%s: recently failed: %w", id, ErrNotFound)` without any request; on any fetch failure (404, error, timeout) `mark` it; on success delete it. Not consulted when `Offline`.
- `internal/cards/cards.go:183-190` — `HTTP` keeps its 20 s client timeout as an outer bound; the 5 s per-request bound is the context.
- `internal/cards/flight_test.go` (new) — 10 goroutines `Card` the same uncached id against a fake with a 100 ms delay → `hits("/cards/x") == 1` and all get the card; a waiter whose ctx is cancelled returns early while the others still succeed; 404 card looked up twice → 1 hit, second call under 1 s; closed fake → second call under 1 s and no new attempt; fake slower than `Timeout` → error, no second attempt; after `clock.Advance(6 * time.Minute)` → retried (2 hits); offline client with nothing cached → 0 hits.

**Complexity**: Medium
**Token estimate**: ~35k tokens
**Agent strategy**: Single agent, sequential execution (flight group first, then wiring, then tests).

### Task: Build the card catalog outside the database lock

**File changes**:
- `internal/cards/catalog.go` (new) — `catalog{listFetched time.Time; setIDs []string; sets map[string]setEntry; setIndex map[string]string; byName map[string][]string}` with `setEntry{d *setDetail; raw []byte; fetched time.Time}`; never mutated after publication. `buildCatalog(ctx)` loads `sets` then each set via `loadEntry` with the 8-way semaphore from `internal/cards/cards.go:425-457`, skipping sets that fail (as `internal/cards/cards.go:620-623` does), entirely outside `c.mu`. `indexes(sets)` is pure: the set-code loop from `internal/cards/cards.go:443-454` (shorter id wins on shared codes) and the print ordering from `internal/cards/cards.go:596-657` (art, newest release, lowest number; `tcgp` series skipped), moved verbatim. `ensureCatalog(ctx) (*catalog, error)`: read `c.cat` under `c.mu`; if nil, run `buildCatalog` through the flight group under key `catalog`, then set `c.cat` under `c.mu`. Offline with no `sets` entry → the existing "set index not cached and offline" `ErrNotFound`.
- `internal/cards/cards.go:175-180` — remove `setIndex`, `indexed`, `byName`; add `cat *catalog`, `flight flightGroup`, `tracker refreshTracker`. Comment on `mu`: it guards in-memory maps and the catalog pointer only, and is never held across disk or network I/O.
- `internal/cards/cards.go:400-467` — delete `buildIndex` and all `set-index-v2.json` reads and writes (the file is left on disk untouched).
- `internal/cards/cards.go:469-513` — `Resolve` no longer holds `c.mu` for its whole body: aliases as today, then `ensureCatalog`, look up `setIndex`; keep today's one forced set-list refresh for an unknown code (refetch `sets`, load any new set ids, rebuild indexes, swap) until the miss policy replaces it in "Refresh set data and check misses against fresh data"; find the number in `cat.sets[setID].d.Cards` (same exact or numeric match); a set id absent from the catalog (e.g. from `setAliases`) is loaded through `loadEntry`.
- `internal/cards/cards.go:556-575` — `prints` reads `cat.byName` from `ensureCatalog`, keeping the `basic ` fallback.
- `internal/cards/cards.go:589-659` — delete `buildNameIndex` (body moved into `indexes`).
- `internal/cards/catalog_test.go` (new) — cold build with the fake's `gate` held: a goroutine calls `Resolve`; meanwhile `Card` and `Cached` for a pre-seeded card return within 1 s; then release the gate and `Resolve` succeeds. Three concurrent `Resolve` calls → each `/sets/<id>` hit once. Shared code (`30th` and `30th-c` both `30C`) resolves to `30th`. No `set-index-v2.json` is written. `TestByNameMoves` and `TestByNameCached` in `internal/cards/byname_test.go` pass unchanged.

**Complexity**: High
**Token estimate**: ~50k tokens
**Agent strategy**: Parallel analysis, sequential integration: one agent extracts `indexes` and its tests while another restructures `Resolve`/`prints` around `ensureCatalog`; merge, then run the race detector.

### Task: Keep card fetches out of the game lock

**File changes**:
- `internal/game/game.go:481-497` — `Game.card`: keep `g.db.Cached(c.Def)`; on nil, call `g.warm(c.Def)` and return nil. Drop the blocking `g.db.Card` with its 5 s timeout. `warm` starts at most one goroutine per def at a time (guard with a `sync.Map` field on `Game`, e.g. `warming`), which calls `g.db.Card` under a 5 s `context.WithTimeout(context.Background(), …)` and never touches `g.mu`.
- `internal/game/game.go:63-89` — add the `warming sync.Map` field with a comment.
- `internal/game/cardlock_test.go` (new) — a `slowDB` that embeds `cardstest.FakeDB` and, once a toggle is set after `newTestGame`-style setup, returns nil from `Cached` for one def and blocks `Card` for it until the test ends (or returns an error, for the failing variant), counting calls. With the toggle on: `g.View` for both seats, `SearchDeck`, and a verb that reaches `card` (e.g. `Move` to the bench) each return within 1 s; 20 repeated `View` calls each return within 1 s; with a `Card` that succeeds after a short delay, a later `View` reflects the card once the warm has finished (poll with a deadline, not a fixed sleep).

**Complexity**: Low
**Token estimate**: ~20k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Refresh stale cards in the background

**File changes**:
- `internal/cards/fresh.go` — add `SetListMaxAge = 30 * 24 * time.Hour`, `SetMaxAge = 90 * 24 * time.Hour`, `ImagelessCardMaxAge = 24 * time.Hour`; `cardStale(e cardEntry, now) bool` = `e.card.Image == "" && now.Sub(e.fetched) > ImagelessCardMaxAge` (judged after `energyArt`); tracker map `bg` for background refresh attempts; `c.maybeRefresh(key, apply func(Entry))`: return if `Offline` or `tracker.bg[key]` within `RetryAfter`; mark; `c.bgwg.Add(1)` and start a goroutine that fetches through the flight group, and on success calls `store().Put` and `apply`; on failure leaves everything as is. Unexported `waitBackground()` (`c.bgwg.Wait()`) for tests.
- `internal/cards/cards.go:243-287` — `Card` and `Cached`, on any hit (memory or store) whose entry is stale, call `maybeRefresh("cards/"+id, …)`, where `apply` decodes, runs `energyArt` and swaps the `mem` entry under `c.mu`; return the stale card at once.
- `internal/cards/cards.go:167-180` — add `bgwg sync.WaitGroup`.
- `internal/cards/fresh_test.go` (new) — card with image seeded 3 years old → `Card` makes 0 hits; imageless card 23 h old → 0 hits; imageless 25 h old, fake now has an image → first `Card` returns no image and is quick, after `waitBackground` `Card` returns the image and the store holds an envelope with the new `fetched`; the same via `Cached`; fake closed or slower than `Timeout` with a stale imageless card → served with no error, and a second lookup within 5 minutes makes no attempt; legacy file with mtime 1 h → 0 hits, legacy imageless with mtime 25 h → refreshed and rewritten as an envelope; offline stale imageless → 0 hits; a Basic Energy print whose art comes from `energyArt` is not refreshed.

**Complexity**: Medium
**Token estimate**: ~40k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Refresh set data and check misses against fresh data

**File changes**:
- `internal/cards/catalog.go` — `c.maybeSweep(cat)` called at the end of `ensureCatalog` when not `Offline`: if `cat.listFetched` is older than `SetListMaxAge` or any `setEntry.fetched` is older than `SetMaxAge`, start one sweep (flight key `sweep`, tracked by `bgwg`): refresh the list if stale and not recently tried (tracker `bg`), load the details of any new set ids, drop removed ids, refresh each stale set (tracker-gated, 8 at a time). If any body changed (`bytes.Equal` on raw) or the set ids changed, publish a new catalog with recomputed `indexes`; otherwise publish one with only the updated fetch times. Swap under `c.mu`. Failures keep the old entries (stale-if-error).
- `internal/cards/catalog.go` — `refreshSetList(ctx)` and `refreshSet(ctx, id)`: synchronous versions (bounded by `Timeout` per request through the flight group) used by misses; both rebuild and swap the catalog when something changed.
- `internal/cards/fresh.go` — tracker map `miss` keyed `code:<UPPER CODE>` and `num:<set id>:<number>`.
- `internal/cards/cards.go:469-513` — `Resolve`: an unknown code (after aliases) → if `miss["code:"+code]` not within `RetryAfter` and not `Offline`, mark it, `refreshSetList`, retry the lookup; a number not found in the set → same with `num:` key and `refreshSet(setID)`. Refresh errors fall through to the existing not-found errors. Remove the once-per-process forced refresh kept by the catalog task.
- `internal/cards/setfresh_test.go` (new) — set list seeded 31 days old lacking set `new` that the fake lists → `Resolve("NEW","1")` succeeds first time; same with the list 2 days old; set seeded 10 days old lacking card 200 that the fake has → resolves first time; same unknown code twice within 5 minutes → `hits("/sets") == 1`, after `clock.Advance(6 * time.Minute)` → 2; list 10 days and set 50 days old → resolving an existing card makes 0 hits; list 31 days old → `Resolve` of a known code returns at once, then after `waitBackground` exactly 1 list hit; set 91 days old → background refresh, 1 hit on `/sets/<id>`; `ByName("Newmon")` fails or picks the old print, then after a refresh adds a set with a newer "Newmon" print (triggered by the stale-list sweep), `ByName` returns the new print on the same client; fake closed with everything stale → `Resolve` and `ByName` succeed from stale data; offline with a stale catalog → 0 hits.

**Complexity**: High
**Token estimate**: ~60k tokens
**Agent strategy**: Parallel analysis, sequential integration: one agent on the sweep and rebuild, one on miss handling in `Resolve`; integrate, then one agent writes the acceptance tests and runs them under the race detector.

### Task: Prove the playmat plays on with the card source down

**File changes**:
- `internal/game/cardsource_test.go` (new) — seed a `t.TempDir()` cache with envelope entries all past their age (fetched over a year ago): `sets` listing `tst` and `sve`; `sets/tst` with official abbreviation `TST` and cards `tst-1`..`tst-6`; `sets/sve` with `sve-006`; card files for the deck in `internal/game/game_test.go:18-31` (`testDeck`), some with an image and some without. Build `cards.NewClient(dir)` with `Base` pointing at a closed `httptest` server and a short `Timeout`. `NewManager(t.TempDir(), client)` then `m.New` with `testDeck` for both seats: no error, only mulligan warnings; `g.View` for both seats each return within 1 s; `client.Card` for `tst-4` returns the stale text; a deck adding a card absent from the cache yields that card by name only, and 10 repeated `g.View` calls each return within 1 s with at most one attempt on the source.

**Complexity**: Low
**Token estimate**: ~25k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Update the card cache knowledge entries

**File changes**:
- `gotchas/tcgdex-cache-never-expires.md` (knowledge entry; repo tier, store `tcgreplay`; written only through the `spek-knowledge` skill, never by path) — rewrite as the current behaviour (consider retitling, e.g. "The TCGdex cache refreshes by kind"): refresh ages (set list 30 days, set card list 90 days, imageless card 24 hours, card with art never); stale served while refreshing, stale kept when TCGdex fails or takes over 5 s; misses refresh at most once per 5 minutes; failed cards remembered 5 minutes, in memory only; envelope format with `fetched`, legacy files aged by mtime; no persisted set index (an old `set-index-v2.json` is ignored); offline serves any age; the board picks up refreshed art on its next load; deleting the cache still forces everything fresh. Keep the image-hosting paragraph accurate.
- `gotchas/tcg-live-log-card-codes.md` (knowledge entry; repo tier, store `tcgreplay`; via `spek-knowledge`) — replace "Bump the index cache filename (now `set-index-v2.json`) whenever the way the index is built changes" and the earlier mention: the set-code index is rebuilt in memory from cached sets, so an index change takes effect on restart with nothing to bump.

**Complexity**: Low
**Token estimate**: ~10k tokens
**Agent strategy**: Single agent, sequential execution, through the `spek-knowledge` skill.

## Testing Strategy

Tests are Go tests, package-internal for `internal/cards`, run with `go test ./...` (and `-race`, given the concurrency). `make test` adds `go vet` and the web e2e suite, which this plan does not change. Per task:

- **Add the cache store with fetch times** — store contract tests: envelope round trip with exact fetch time, legacy files aged by mtime, `ErrNotCached`, safe paths, today's paths.
- **Route card lookups through the store** — introduces the `httptest` fake TCGdex (request counts per path, delay, failure, gate, close) and a fake clock. It asserts that fetched entries are stored with the clock's time, and that an offline legacy cache answers as before. The existing by-name tests stay green.
- **Deduplicate fetches and remember failed cards** — concurrency and failure tests: one request for N concurrent lookups; a cancelled waiter does not fail the others; 404, closed and slow sources are each tried once per 5 minutes and answered within 1 s in between; a retry happens after 5 minutes; offline makes no requests.
- **Build the card catalog outside the database lock** — a cold build held open by the fake's gate does not delay `Card`/`Cached` of a cached card beyond 1 s; concurrent `Resolve` calls fetch each set once; set-code collisions still pick the shorter id; no persisted index is written.
- **Keep card fetches out of the game lock** — game tests with a slow or failing `DB` wrapper around `cardstest.FakeDB`: `View`, `SearchDeck` and a verb each return within 1 s; repeated views stay fast; a background warm makes the card visible to later reads.
- **Refresh stale cards in the background** — age boundaries for imageless cards (23 h vs 25 h); art never refetched; stale served then refreshed (`Card` and `Cached`); stale-if-error for closed and slow sources, with the 5-minute retry window; legacy mtime ages; offline makes no requests; borrowed Energy art is not refreshed.
- **Refresh set data and check misses against fresh data** — the spec's set-level acceptance criteria: a new set on stale and recent lists; a new promo in a recent set; a repeated miss makes one list request (two after 5 minutes); fresh set data makes no requests; a stale list or set is refreshed in the background; the name index follows a new set; stale-if-error for `Resolve`/`ByName`; offline makes no requests.
- **Prove the playmat plays on with the card source down** — manager-level test with a real `cards.Client`, a stale fixture cache and a closed source: a game builds without errors, views stay under 1 s, stale card text is served, and an uncached card shows by name and is not retried.
- **Update the card cache knowledge entries** — no tests; checked by reading the updated entries.

Manual items for the implementation test plan:
- Success metric: 95% of hosted board reads within 200 ms over a week, none waiting more than 1 s on a card fetch. **Manual — captured in the implementation test plan.**
- Upgrade check against a real previous-release cache, offline and online. **Manual — captured in the implementation test plan.**
- Live TCGdex check: a new set on an aged list resolves on the first import, and aged imageless art appears on the board after a reload. **Manual — captured in the implementation test plan.**

## Project References

- Spec: `20261104090640-51bd1f36-card-cache-freshness` (read with `spektacular spec file read`).
- Epic: `20261104082020-109dceb1-tcgdex-spaces-cache`; next spec `20261104082020-109dceb1-tcgdex-spaces-cache` builds on the `Store` contract.
- Design documents: none referenced by the spec.
- Knowledge (repo tier, store `tcgreplay`): `gotchas/tcgdex-cache-never-expires.md`, `gotchas/tcg-live-log-card-codes.md`, `gotchas/game-listeners-under-lock.md`, `learnings/card-image-sources.md`, `conventions/skills-text-and-packaging.md`.
- Pre-spec notes: `.spektacular/work/card-cache/notes.md`.
- Repo root: `tcgreplay` → `/home/nicj/code/github.com/nicholasjackson/tcgreplay` (Go server in `internal/`, board in `web/`).

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

Per task: store ~20k (Low); route through store ~45k (Medium); single-flight and failed memory ~35k (Medium); catalog ~50k (High); game lock ~20k (Low); card refresh ~40k (Medium); set refresh and misses ~60k (High); end-to-end game test ~25k (Low); knowledge updates ~10k (Low).

## Migration Notes

- No migration step is run. Previous-release cache files (bare TCGdex JSON) are read as legacy entries, with their modification time as their fetch time. They are rewritten in the envelope format only when next refreshed, so a cache can hold both formats indefinitely.
- `set-index-v2.json` is no longer read or written; an existing one is left in place and ignored. Implementers must not delete it programmatically (harmless, and keeps the change read-only for old files).
- Downgrade is unsupported: an earlier release cannot read envelope files. Recovery is to delete the cache directory (`~/.cache/tcgreplay` by default, or the `-cache` directory).
- The hosted image has no cache volume, so it starts empty on each deploy and writes envelopes from the start.

## Performance Considerations

- `Client.mu` is held only to read or swap in-memory maps and the catalog pointer, never across disk or network I/O. That is what keeps `Cached` (and so every board read) fast during a cold build.
- The catalog rebuild after a refresh is pure CPU over ~200 sets and ~20,000 prints, done off-lock: on the order of tens of milliseconds, and only when a refresh changed something.
- A warm start no longer has a persisted set index, so the first `Resolve` or `ByName` reads every set file from disk (in parallel, 8 at a time), which today happens anyway for `ByName`. Expect well under a second on local disk.
- A fresh cache generates no background traffic: refreshes happen only when a lookup touches stale data. At most one sweep runs at a time, and each key is retried no more than once per 5 minutes.
- Each source request is bounded at 5 s, and single-flight caps concurrent duplicate work. The catalog build keeps the existing 8-way parallelism toward TCGdex.
- Success metric (95% of board reads within 200 ms hosted, none waiting more than 1 s on a fetch) is supported by `Game.card` never fetching under the game lock and `Cached` never waiting on the catalog.
