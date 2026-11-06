---
created_date: "2026-11-04"
document_status: final
closed_date: "2026-11-04"
---

# Plan: 20261104090640-51bd1f36-card-cache-freshness

<!-- Metadata -->
<!-- Created: 2026-11-04T11:40:03Z -->
<!-- Commit: 8c26660570c61a3e063c43882de3fae7a6168aeb -->
<!-- Branch: main -->
<!-- Repository: git@github.com:nicholasjackson/tcgreplay.git -->

## Overview

The playmat's card database starts refreshing its cached TCGdex data on its own: the set list after 30 days, each set's card list after 90 days, and a card without artwork after 24 hours, while cards with artwork are kept. Stale data is served at once and refreshed in the background. The last good copy is kept when TCGdex is down or slow. A deck code naming a set or promo the cache has not seen resolves on the first import, and card lookups never freeze the board, while offline mode keeps working from whatever is cached. Desktop and hosted players get new sets, promos and late artwork without anyone clearing the cache. The work also lays down the storage seam the next spec in this epic uses to add a DigitalOcean Spaces tier.

## Conventions

- **Guide text and packaging (`conventions/skills-text-and-packaging.md`)** — applies only in its packaging half: nothing in this plan may exclude `internal/guides` from a build context, and no guide text changes. The guide-wording rule does not bear on this work, since no guide is edited.

## Architecture & Design Decisions

All the work lands in the one registered repo, `tcgreplay`: the card database in `internal/cards` and one function in `internal/game`. The card database is split into three layers. At the bottom is a small **`Store`** interface: get and put a raw response by key (`sets`, `sets/<id>`, `cards/<id>`), each value carrying the time it was fetched (`Entry{Data, Fetched}`). Its one implementation for now, the disk store, keeps today's file layout under the cache directory. New files hold an envelope, `{"fetched": <RFC 3339 time>, "data": <TCGdex body>}`. A file without the envelope, written by an earlier release, is read with its modification time as its fetch time, so existing caches keep working and age from when they were written. The next spec in this epic adds a DigitalOcean Spaces tier by putting a tiered `Store` (disk, then Spaces) in the same slot. The fetch time travels inside the entry, so it survives copying between tiers. Freshness is decided above the store, so the new tier gets the policy without changes.

The middle layer is the **freshness policy** in `cards.Client`. Each kind of entry has a refresh age: the set list 30 days, a set's card list 90 days, a card without artwork 24 hours, and a card with artwork none. Lookups follow stale-while-revalidate with stale-if-error (RFC 5861). A hit on stale data returns the stale copy at once and starts one background refresh for that key. A refresh that errors or takes longer than 5 seconds keeps the stale copy and is not retried for 5 minutes. A miss (an unknown set code, or a number missing from a set) refreshes that data before answering, at most once per distinct miss every 5 minutes. A card the source fails to return (404, error or timeout) is remembered as failed for 5 minutes, so repeated reads return at once with no data. In offline mode none of this contacts TCGdex: every cached entry is served whatever its age. Every fetch and index build goes through an in-package single-flight group keyed by entry, so simultaneous requests make one request to TCGdex. The fetch runs on a detached context bounded at 5 seconds, so one caller giving up does not fail the others.

The top layer is an immutable **catalog snapshot**: the set list, every set's card list with its fetch time, and the two indexes derived from them (set code to set id, and name to prints, keeping today's print preference unchanged). It is built off-lock under single-flight, and swapped in under `Client.mu`, which is now held only to read or swap maps and never across network or disk I/O. Whenever a refresh changes the set list or a set's card list, the snapshot is rebuilt in memory from its entries and swapped, so `Resolve` and `ByName` follow new sets and promos without a restart. The persisted `set-index-v2.json` is retired: derived data with an age of its own is what let the old index disagree with its inputs (knowledge `gotchas/tcgdex-cache-never-expires.md`). On the game side, `Game.card` is always called with the game lock held, and it stops calling the network. It reads `Cached` and, on a miss, starts a background fetch and treats the card as unknown for that call. Games load their cards before their lock is taken (`BuildCards`, `PrefetchCards`), so normal play is unchanged, and no board read or play waits on another card's fetch. The `DB` interface and `cardstest.FakeDB` are unchanged, and only which definition a card gets can depend on cache timing, never where it is placed (spec constraint).

This beats putting expiry inside the store, and beats patching `getJSON` with mtime ages under the existing locks. The expiry rules need card-level knowledge (does the card have art, is this a miss), and a Spaces tier would otherwise have to copy them. The patch would keep `Resolve` and `ByName` holding `Client.mu` through a cold build of every set (`internal/cards/cards.go:475-479`, `558-562`), which stalls every board read through `Cached`. The evidence and the rejected options are in `research.md#alternatives-considered-and-rejected`.

## Component Breakdown

- **Cache store (new).** It owns persistence of raw TCGdex responses by key, each entry carrying its fetch time. It knows nothing about expiry. The disk store keeps today's layout under the cache directory and writes atomically. It reads both the new envelope and the bare files earlier releases wrote, taking a bare file's modification time as its fetch time. The card database reads and writes only through this interface, so the next spec's Spaces tier (a tiered store: disk, then Spaces) replaces it without touching anything above.
- **Freshness policy (new, inside the card database).** It owns the refresh age of each kind of entry (set list, set card list, card without artwork, card with artwork), and decides whether a given entry is fresh, stale or never-expiring at a given time. It also holds the time windows for retries: 5 minutes for a repeated miss, 5 minutes for a failed fetch, and a 5-second bound on any source request. The clock is injectable, so tests can age entries.
- **Single-flight group (new, inside the card database).** It owns the deduplication of simultaneous work by key: one fetch per entry, one catalog build at a time. The work runs on a detached, time-bounded context, and each waiter may give up on its own context without cancelling the work for the others.
- **Refresh tracker (new, inside the card database).** It owns the short-lived memory of recent attempts: when each key was last refreshed in the background, when each distinct miss last forced a refresh, and which cards recently failed. It is the reason a down source or an unknown card costs one request per 5 minutes rather than one per read. It is in memory only, so a restart forgets it.
- **Catalog (new, replacing the set index and name index fields).** It is an immutable snapshot of the set list, every set's card list with its fetch time, and the two indexes derived from them: set code to set id, and name to prints with the existing print preference. It is built from the store, falling back to the source for missing entries, outside the database lock. It is swapped in whole under the lock, and rebuilt in memory whenever a refresh changes the set list or any set's card list. `Resolve` and `ByName` read the current snapshot. The persisted set index file is retired.
- **Card database client (changed).** `cards.Client` keeps its public `DB` interface (`Card`, `Cached`, `Resolve`, `ByName`) and its constructor and offline flag. It now composes the components above. Card lookups go through memory, then the store, then a single-flight fetch, with stale-while-revalidate and stale-if-error. `Cached` still never waits on the network, but may start a background refresh of a stale card. `Resolve` checks misses against fresh data. Its lock guards only in-memory maps. Offline mode short-circuits every source contact and every expiry.
- **Game card lookup (changed).** `Game.card`, called with the game lock held, reads only the card database's cached copy. On a miss it starts a background fetch and treats the card as unknown for that call, so no board read or play waits on the network. Prefetching (when a game is built or imported) is unchanged and still loads every card before the game's lock is taken.
- **Unchanged consumers.** The importer, deck resolution, the board's card endpoint, the MCP `card_text` tool, `cardstest.FakeDB` and the board front end all keep calling the same `DB` methods, and gain freshness without code changes.

## Data Structures & Interfaces

**`Entry` and `Store` (new, exported from `cards`).** These are the contract between the card database and any cache tier. A key names one TCGdex response: `sets`, `sets/<set id>` or `cards/<card id>`, with raw ids (each store makes them safe for its own medium). `Data` is the body exactly as TCGdex sent it. `Fetched` is when it was fetched from TCGdex, never when it was copied between tiers, so a tier that copies an entry must keep `Fetched` as it is.

```go
type Entry struct {
    Data    []byte    // raw TCGdex JSON body
    Fetched time.Time // when the card source returned it
}

var ErrNotCached = errors.New("not cached")

type Store interface {
    Get(ctx context.Context, key string) (Entry, error) // ErrNotCached when absent
    Put(ctx context.Context, key string, e Entry) error
}

func NewDiskStore(dir string) Store
```

**On-disk entry format (serialization boundary).** The disk store writes each entry as a JSON envelope at `<dir>/<key>.json`, the same paths as today:

```json
{"fetched": "2026-11-04T10:00:00Z", "data": <TCGdex body>}
```

A file that is not an envelope (no `fetched` and `data` keys, or a JSON array such as the old `sets.json`) is a legacy entry. Its whole content is `Data`, and its modification time is `Fetched`. Legacy files are rewritten as envelopes only when they are next refreshed. `set-index-v2.json` is no longer read or written.

**`Client` (changed, exported).** Its public shape stays compatible: `Base`, `CacheDir`, `HTTP` and `Offline` keep their meaning, `NewClient(dir)` still returns a ready client, and it still implements the unchanged `DB` interface. The new optional fields are listed below; each has a working default set by `NewClient`.

```go
type Client struct {
    Base, CacheDir string
    HTTP           *http.Client
    Offline        bool
    Store          Store            // default NewDiskStore(CacheDir)
    Timeout        time.Duration    // per source request; default 5s
    Now            func() time.Time // default time.Now; tests inject a clock
    // unexported: mu, mem (id -> card entry with fetch time), catalog
    // snapshot, refresh tracker, single-flight group, background wait group
}
```

**Refresh ages (constants, `cards`).** `SetListMaxAge = 30 × 24h`, `SetMaxAge = 90 × 24h` and `ImagelessCardMaxAge = 24h`. A card with artwork never expires. `RetryAfter = 5m` is shared by repeated misses, failed fetches and failed background refreshes. These are package constants, not settings.

**Internal contracts (unexported, `cards`).** `catalog` is an immutable snapshot of `{setList, sets map[setID]setEntry, setIndex map[code]setID, byName map[nameKey][]cardID}`. Every `setEntry` pairs a decoded `setDetail` with its `Fetched` time, and the catalog is replaced whole, never mutated. `flightGroup.Do(ctx, key, fn)` runs `fn` once per key at a time, and lets each waiter return early on its own `ctx`. `refreshTracker` holds three maps of last-attempt times (background refreshes, misses, failed cards), read and written under `Client.mu`.

**`DB` interface and `cardstest.FakeDB`: unchanged.** No caller-facing type, MCP tool schema, HTTP response or board protocol changes.

## Implementation Detail

**Package shape.** `internal/cards` stays one package. `cards.go` keeps the card types, the `DB` interface and the TCGdex-specific helpers (`LogCodeID`, aliases, `energyArt`, `nameKey`). The new pieces go in their own files, each with one job: the store and its envelope format, the freshness policy and refresh tracker, the single-flight group, and the catalog (snapshot, build, rebuild, miss handling). The one function that serves a cached file forever (`getJSON`) is replaced by a single "load entry" path that every kind of lookup shares: memory, then the store, then a single-flight fetch from TCGdex. A reader can find the whole expiry story in one place, rather than in per-method special cases like today's once-per-process index rebuild flag.

**Lookup flow (new pattern: stale-while-revalidate with stale-if-error).** Every lookup gets an entry and its fetch time, then asks the policy whether it is fresh. If the entry is fresh, or the client is offline, it is used as is. If it is stale, it is used as is, and a background refresh for its key starts. The refresh is skipped if one is already running (single-flight) or was attempted in the last 5 minutes (tracker). A background refresh fetches with the 5-second bound. On success it puts the new entry into the store and updates memory (for a card) or rebuilds the catalog (for the set list or a set). On failure it leaves everything as it was. A cold miss on a card checks the failed-card memory first, then fetches through the single-flight group; a failure is remembered for 5 minutes. A miss in the catalog (an unknown set code, or a number absent from a set) is different: it fetches synchronously, because its answer depends on the fresh data. If the set list changed, the details of any new set ids are fetched too, and then the lookup is answered from the rebuilt catalog. The tracker allows each distinct miss to do this once per 5 minutes.

**Catalog lifecycle.** The catalog is built on first use by `Resolve` or `ByName`, under single-flight, from the store, fetching only the entries missing from it. Fetches run eight at a time, as today, and a set that cannot be loaded is left out, as today. Each use of the catalog also checks it for staleness: if the set list or any set's card list is past its age, one background sweep refreshes the stale entries (at most one sweep at a time, each key throttled by the tracker). Any refresh whose body differs from the old one produces a new snapshot. The indexes are recomputed in memory from the snapshot's set entries, then swapped in under the lock. A refresh whose body is unchanged swaps in only the new fetch time. Index building moves out of `Client.mu` entirely: the lock now guards only reading and swapping maps, a rule the code states in a comment on the field. That removes the cold-build stall for `Cached` and `Card`, whose callers include every board read.

**Background work.** Refreshes run as goroutines on a detached context, tracked by a wait group. Tests use an unexported helper to wait for background work to settle, instead of sleeping. No timers or long-lived goroutines are added: nothing runs unless a lookup touches stale data, so a desktop session with a fresh cache makes no TCGdex requests at all.

**Game side.** `Game.card` keeps its signature and nil-means-unknown contract. It loses its blocking fetch and gains a fire-and-forget warm of a missing card. That follows the existing rule that work under the game lock must not call out (knowledge `gotchas/game-listeners-under-lock.md` applies the same rule to listeners).

**Patterns followed, not introduced.** The atomic write (temporary file, then rename), the 8-way fetch semaphore, the print-preference ordering and the "skip what cannot be loaded" rule are carried over unchanged. Tests follow the existing package-internal style of `byname_test.go`, which writes a cache directory in `t.TempDir()`. The new piece is an `httptest` fake TCGdex that counts requests per path, plus an injected clock, so request counts and entry ages can be asserted directly.

## Dependencies

- **Design documents this plan was built on: none.** The spec references no design documents.
- **Upstream specs and plans: none.** This is the first spec in the `tcgdex-spaces-cache` epic, with no dependencies, so nothing must land before it starts.
- **Downstream: `20261104082020-109dceb1-tcgdex-spaces-cache`** (next in the epic) builds on the `Store` and `Entry` contract introduced here. It adds a Spaces tier and a tiered store in the store slot. The contract must not change shape without updating that plan.
- **TCGdex REST API v2** (`https://api.tcgdex.net/v2/en`), an external service: it supplies the set list, set card lists and cards. It is used exactly as today (same endpoints, no auth), and needs no change.
- **Go standard library only** (`net/http`, `encoding/json`, `os`, `sync`, `time`, `context`; `net/http/httptest` in tests). No new modules; the single-flight group is written in-package rather than adding `golang.org/x/sync`.
- **`internal/cards` (changed):** the package this plan rewrites internally; its `DB` interface stays as it is.
- **`internal/cards/cardstest` (unchanged):** `FakeDB` keeps implementing `DB` for game and webui tests.
- **`internal/game` (changed):** only `Game.card` changes; the importer, `BuildCards` and `PrefetchCards` use the database as before.
- **`internal/deck`, `internal/webui`, `internal/mcpserver`, `cmd/tcgreplay`, `web/` (unchanged):** they call `DB` methods or construct the client through `NewClient`, and gain freshness with no code change.
- **Knowledge entries relied on:** `gotchas/tcgdex-cache-never-expires.md` (the cache layout and the failures this fixes), `gotchas/tcg-live-log-card-codes.md` (set-code collisions: the shorter id wins; an index-format change needs stale caches rebuilt) and `gotchas/game-listeners-under-lock.md` (no outbound work under the game lock). The first two are updated by this plan once the change lands.

## Testing Approach

**Kinds of tests.** Most of the coverage is package-internal Go unit and integration tests in `internal/cards`. They drive a real `cards.Client` against an `httptest` fake TCGdex, which serves configurable set lists, set card lists and cards, counts requests per path, and can be made slow, failing or closed. A test-controlled clock ages the entries. Cache directories are written in `t.TempDir()`, following the existing fixture style of the by-name tests. Background refreshes are awaited through an unexported wait helper, never by sleeping. A smaller set of tests in `internal/game` covers the game lock, using a deliberately slow or failing `DB` built on `cardstest.FakeDB`, plus one end-to-end manager test with a real `cards.Client` and a dead source. The existing tests stay green unchanged, including the offline by-name tests and `TestByNameCached`, which runs against a real local cache when one exists. The whole suite runs under the race detector, since this change is mostly about concurrency.

**Where coverage concentrates.** The heaviest coverage goes to the freshness policy and the catalog, where the spec's behaviour lives. Each acceptance criterion maps to at least one behavioural test asserting an outcome and a request count:

- **Refresh ages.** An entry just under its age makes no request. An entry just over its age is served stale and then refreshed: the set list at 30 days, a set's card list at 90 days, and an imageless card at 24 hours. A card with artwork aged years makes no request ("Art kept", "Set data kept between refreshes").
- **Stale-while-revalidate.** The first lookup of a stale entry returns the old data at once. After the refresh settles, the next lookup returns the new data ("Late art appears").
- **Misses against fresh data.** An unknown set code resolves on the first try against a stale list and against a recent list. A card missing from a recent set's card list resolves on the first try. The same unknown code looked up twice within 5 minutes makes exactly one set-list request, and a second request is allowed after the clock passes 5 minutes ("New set resolves…", "New promo resolves…", "Repeated miss not refetched").
- **Stale-if-error.** With the source closed, failing, or slower than the (test-shortened) timeout, and everything stale, `Resolve`, `ByName` and `Card` all answer from stale copies with no error ("Stale served when source down", at cache level).
- **Name index follows refreshes.** A by-name lookup that resolved to an old print resolves to the new set's print after a set-list refresh adds that set, on the same client ("Name lookup refreshed in place").
- **Legacy caches.** A bare-file cache with controlled modification times serves every entry offline. Online, it makes no requests for entries younger than their age, and refreshes those older, rewriting them as envelopes ("Old cache still works").
- **Concurrency.** N simultaneous `Card` calls for one uncached card make exactly one request ("One request for the same card"). During a cold catalog build held open by a blocking fake, `Card` and `Cached` for an already-cached card return within one second ("No stall during a cold build").
- **Failed lookups.** A card TCGdex reports unknown, or one whose source is unreachable, is requested at most once in 5 minutes. Repeated lookups return within one second with no data ("Failed lookups not retried at once").
- **Offline.** With an offline client and only stale data, everything is served and the fake records zero requests ("Offline untouched").
- **Store contract.** The disk store round-trips an entry with its exact fetch time, reads a legacy file with its modification time, and reports `ErrNotCached` for a missing key.

**Game-side assertions.** A board read (`View`) and a play on a game whose `DB.Card` blocks for one card both return within one second, without waiting for that fetch. Repeated board reads with a failing card stay under one second each. An end-to-end manager test builds a game (deck codes resolved, card text available) from a stale fixture cache with the source down, with no error and no warnings beyond those of a fresh cache. Together these cover the board half of "Stale served when source down" and "Failed lookups not retried at once".

**Determinism.** No test changes the importer's placement; the existing importer and record round-trip tests must pass unchanged, which guards the spec constraint that cache timing affects only which definition a card gets.

**Deliberate gaps.** There are no browser (e2e) tests: the board front end does not change, and refreshed art reaching the board is a server behaviour covered above. The real TCGdex API is never called from tests. The spec's 5-second source timeout is asserted with a shortened timeout, so the suite stays fast.

**Success metrics.**
- "Over a week of hosted use, 95% of board reads complete within 200 ms, and none waits on a card cache fetch for more than 1 second": **Manual — captured in the implementation test plan.** It needs a week of real hosted traffic. The one-second bounds on board reads during fetches and failures are also asserted by the behavioural tests above, but the hosted percentile itself can only be observed in production.

**Manual reviews.**
- **Manual — captured in the implementation test plan.** Upgrade check against a real cache from the previous release, offline and online: every card shows, and no requests go out for young entries.
- **Manual — captured in the implementation test plan.** Live check against real TCGdex: with an aged set list, a deck naming a recent set resolves on the first import, and an aged imageless card shows its art on the board after a reload once the refresh has finished.

## Milestones & Tasks

### Milestone 1: The card cache records when each entry was fetched

**What changes**: Nothing a player sees changes yet. This milestone is internal, and it is worth landing on its own because the rest of this spec and the next one (the Spaces tier) both build on it. Every card, set and set list the playmat caches now carries the time it was fetched from the card database. The cache sits behind a small storage interface that a second tier can join later. Caches written by the previous release keep working unchanged, offline and online, and take their file's age as their fetch time. Cards, deck codes and by-name lookups all behave exactly as before.

**Validation point**: The existing card, game and importer tests pass unchanged. New store tests show an entry round-trips with its exact fetch time, and a previous-release cache file is served with its modification time as its fetch time. An offline run against a copy of a real previous-release cache resolves every card it did before.

#### - [x] Task: Add the cache store with fetch times
**Id:** 319e4ac7-542c-40f9-8d3e-4f0af0454fd1
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Introduce the storage interface the card database will read and write through: get and put a raw card-source response by key, with the time it was fetched. Its first implementation keeps entries on local disk in today's layout. New entries are written in an envelope that records the fetch time, and files from earlier releases are read with their modification time standing in. This is the seam the next spec's Spaces tier plugs into.

*Technical detail:* [context.md#task-add-the-cache-store-with-fetch-times](./context.md#task-add-the-cache-store-with-fetch-times)

**Acceptance criteria**:
- [x] An entry put into the disk store comes back with the same body and exactly the same fetch time.
- [x] A cache file written by the previous release is read back as an entry whose body is the file's content and whose fetch time is the file's modification time.
- [x] Asking for a key that was never stored reports "not cached", distinct from any other error.
- [x] Entries land at the same paths as today's cache files, and writes are atomic, so a crash never leaves a half-written entry.

#### - [x] Task: Route card lookups through the store
**Id:** a7f5f7ae-709c-4b35-8e70-52f5c456e93c
**Repo:** tcgreplay
**Depends on:**
- 319e4ac7-542c-40f9-8d3e-4f0af0454fd1 — Add the cache store with fetch times
**Execution:** agent

Replace the card database's cache-forever file reads with one shared path for loading an entry: memory, then the store, then the card source. Every entry fetched is stored with its fetch time. The in-memory card cache remembers each card's fetch time too, ready for expiry. Behaviour stays as it is today. This task also adds the fake card source that later tests use.

*Technical detail:* [context.md#task-route-card-lookups-through-the-store](./context.md#task-route-card-lookups-through-the-store)

**Acceptance criteria**:
- [x] Cards, deck codes and by-name lookups give the same answers as before, and the existing card, deck, game and importer tests pass unchanged.
- [x] A card, set or set list fetched from the card source is saved with the time it was fetched.
- [x] A cache from the previous release, opened offline, serves every card, set and by-name lookup it served before.
- [x] Constructing the card database the way the app does today needs no new settings.

### Milestone 2: The board never waits on card data

**What changes**: Board reads, plays and lookups of cards already cached return at once, even while the playmat is building its card database from scratch or fetching other cards. A card the card database cannot supply (unknown, erroring or not answering within 5 seconds) is tried at most once every 5 minutes. Meanwhile it shows by name only, instantly, instead of stalling every board refresh. Several requests for the same card or the same set data make a single request to the card database.

**Validation point**: Tests show that during a cold build held open by a slow fake card source, reads of cached cards return within one second. Simultaneous requests for one uncached card make exactly one request. A failing card is requested at most once in 5 minutes, and repeated board reads holding it each return within one second.

#### - [x] Task: Deduplicate fetches and remember failed cards
**Id:** bd415d22-a274-4b78-83cc-456df13ddb11
**Repo:** tcgreplay
**Depends on:**
- a7f5f7ae-709c-4b35-8e70-52f5c456e93c — Route card lookups through the store
**Execution:** agent

Make simultaneous requests for the same card source data share one request. Bound every request to the card source at 5 seconds. Remember for 5 minutes any card the source failed to return (unknown, error, or no answer), so repeated lookups answer at once with no data instead of asking again. One caller giving up must not cancel the shared request for the others.

*Technical detail:* [context.md#task-deduplicate-fetches-and-remember-failed-cards](./context.md#task-deduplicate-fetches-and-remember-failed-cards)

**Acceptance criteria**:
- [x] Several simultaneous lookups of the same uncached card produce exactly one request to the card source, and all of them get the card.
- [x] A card the source reports as unknown, or cannot return because it is down or slower than 5 seconds, is requested at most once in 5 minutes. Lookups in between return within one second with no card.
- [x] Once 5 minutes have passed, the next lookup of a failed card asks the source again.
- [x] Offline mode still never contacts the card source.

#### - [x] Task: Build the card catalog outside the database lock
**Id:** 013ab016-d61d-4022-b6e2-8966b5614f21
**Repo:** tcgreplay
**Depends on:**
- bd415d22-a274-4b78-83cc-456df13ddb11 — Deduplicate fetches and remember failed cards
**Execution:** agent

Replace the separate set-code index and name index with one immutable catalog, holding the set list, every set's card list with its fetch time, and the two indexes. It is built once under single-flight, without holding the lock that card lookups use, and swapped in whole. That ends the stall where a cold build of every set blocked every board read. The persisted set index file is retired, since the indexes are rebuilt from the cached sets. Print preference and set-code collision rules stay exactly as they are.

*Technical detail:* [context.md#task-build-the-card-catalog-outside-the-database-lock](./context.md#task-build-the-card-catalog-outside-the-database-lock)

**Acceptance criteria**:
- [x] Starting with empty caches, while a first import is still fetching set data, a lookup of a card that is already cached returns within one second.
- [x] Several imports resolving deck codes at once fetch each set only once.
- [x] Deck codes and by-name lookups pick the same prints as before, including the shorter id for sets that share a code.
- [x] The playmat no longer reads or writes the persisted set index file, and an old one left on disk does no harm.

#### - [x] Task: Keep card fetches out of the game lock
**Id:** 739b3a00-fb90-4918-abf2-65f7d1b9548e
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

The game's own card lookup runs while the game is locked, and today it can wait up to 5 seconds on the card source, which freezes every other board read and play on that game. Make it use only cached card data, and start a background fetch for a missing card instead of waiting. A card that is not cached is treated as unknown for that moment, as it already is when a fetch fails.

*Technical detail:* [context.md#task-keep-card-fetches-out-of-the-game-lock](./context.md#task-keep-card-fetches-out-of-the-game-lock)

**Acceptance criteria**:
- [x] A board read or a play on a game holding a card whose fetch hangs returns within one second.
- [x] Repeated board reads of a game holding a card whose lookup fails each return within one second.
- [x] A card missing from the cache when it is first needed is fetched in the background, and later reads see it once that fetch finishes.
- [x] Games built from decklists and imported games behave as before, and the existing game tests pass unchanged.

### Milestone 3: Cached card data refreshes on its own

**What changes**: New sets, new promos and newly added artwork now appear without anyone clearing the cache. The set list refreshes after 30 days, each set's card list after 90 days, and a card without artwork after 24 hours. Cards with artwork are kept. A deck naming a set or promo the cache has not seen yet resolves on the first import, and by-name lookups follow those changes without a restart. A lookup that finds stale data gets it at once while the refresh runs in the background. If the card database is down or slow, the playmat keeps serving its last good copy with no error. Offline mode never contacts the card database and serves whatever it has, however old.

**Validation point**: Tests with an aged cache and a fake card source show each acceptance criterion: new set and new promo resolve first time; a repeated miss makes one request; late art appears once the refresh settles; art and fresh set data make no requests; stale data is served with the source down or slow; the name lookup follows a new set; offline makes no requests. The knowledge entries describing the cache are updated to the new behaviour.

#### - [x] Task: Refresh stale cards in the background
**Id:** 80112c38-1d01-417f-b5ac-fa258f8061a5
**Repo:** tcgreplay
**Depends on:**
- 013ab016-d61d-4022-b6e2-8966b5614f21 — Build the card catalog outside the database lock
**Execution:** agent

Add the freshness policy and apply it to cards. A card cached without artwork goes stale after 24 hours, and a card with artwork never does. A lookup of a stale card gets the cached copy at once and starts one background refresh. The refresh keeps the old copy if the source errors or takes longer than 5 seconds, and is not retried for 5 minutes. Offline mode never refreshes.

*Technical detail:* [context.md#task-refresh-stale-cards-in-the-background](./context.md#task-refresh-stale-cards-in-the-background)

**Acceptance criteria**:
- [x] A card cached without artwork more than 24 hours ago is returned at once from the cache. Once the refresh finishes, lookups show the artwork the card source now has, with no restart or cache clear.
- [x] A card cached with artwork, however long ago, causes no request to the card source.
- [x] With the card source down or slow, a stale card is still served with no error, and the source is asked again no sooner than 5 minutes later.
- [x] A previous-release card file younger than 24 hours causes no request; an older imageless one is refreshed and saved with its new fetch time.
- [x] In offline mode, stale cards are served and the card source is never contacted.

#### - [x] Task: Refresh set data and check misses against fresh data
**Id:** 98142a67-9383-4af7-b5b6-dd83ca083519
**Repo:** tcgreplay
**Depends on:**
- 80112c38-1d01-417f-b5ac-fa258f8061a5 — Refresh stale cards in the background
**Execution:** agent

Apply the freshness policy to set data. The set list goes stale after 30 days and each set's card list after 90 days. Stale set data is served at once and refreshed in the background, and the catalog is rebuilt when a refresh changes anything, so by-name lookups follow new sets without a restart. A deck code naming an unknown set, or a number missing from a set, refreshes that data before answering, at most once per distinct miss every 5 minutes. This replaces today's once-per-process index rebuild.

*Technical detail:* [context.md#task-refresh-set-data-and-check-misses-against-fresh-data](./context.md#task-refresh-set-data-and-check-misses-against-fresh-data)

**Acceptance criteria**:
- [x] A deck naming a set the cached set list lacks resolves on the first import, whether the cached list is older or younger than 30 days.
- [x] A deck naming a card that a recently cached set lacks, but the card source now has, resolves on the first import.
- [x] Looking up the same unknown set code twice within 5 minutes makes exactly one request for the set list.
- [x] With the set list younger than 30 days and a set younger than 90 days, looking up that set's cards makes no request to the card source.
- [x] After a refresh adds a new set while the playmat runs, a card known only by name resolves to that set's print without a restart.
- [x] With the card source down or slow and all set data stale, deck codes and by-name lookups still answer from the stale copies with no error.
- [x] In offline mode, stale set data is served and the card source is never contacted.

#### - [x] Task: Prove the playmat plays on with the card source down
**Id:** 6967f59a-3846-4e45-b250-8821f21100cb
**Repo:** tcgreplay
**Depends on:**
- 98142a67-9383-4af7-b5b6-dd83ca083519 — Refresh set data and check misses against fresh data
- 739b3a00-fb90-4918-abf2-65f7d1b9548e — Keep card fetches out of the game lock
**Execution:** agent

Add an end-to-end game test that wires the real card database to a stale cache and an unreachable card source. It builds a game from decklists, reads the board and asks for card text. This proves the cache-level behaviour holds where players see it: stale copies are served, nothing errors, and board reads stay fast.

*Technical detail:* [context.md#task-prove-the-playmat-plays-on-with-the-card-source-down](./context.md#task-prove-the-playmat-plays-on-with-the-card-source-down)

**Acceptance criteria**:
- [x] With every cached entry past its refresh age and the card source unreachable, a game builds from decklists with set codes, with no error and no warnings beyond those a fresh cache gives.
- [x] Card text and artwork come from the stale copies, and board reads each return within one second.
- [x] A card the stale cache lacks shows by name only, and repeated board reads do not retry it within 5 minutes.

#### - [x] Task: Update the card cache knowledge entries
**Id:** dc01362a-e359-4508-b44d-d88f6e905ebb
**Repo:** tcgreplay
**Depends on:**
- 98142a67-9383-4af7-b5b6-dd83ca083519 — Refresh set data and check misses against fresh data
**Execution:** agent

The knowledge base says the card cache never expires, and tells readers to bump the persisted set index's filename when the index changes. Both are wrong once this lands. Rewrite the never-expires gotcha to describe the new behaviour and its traps: per-kind refresh ages, the envelope format, the 5-minute miss and failure memory, offline serving stale data, and no persisted index. Then correct the set-index note in the card-codes gotcha.

*Technical detail:* [context.md#task-update-the-card-cache-knowledge-entries](./context.md#task-update-the-card-cache-knowledge-entries)

**Acceptance criteria**:
- [x] No knowledge entry still says the card cache never expires or tells readers to bump the set index filename.
- [x] A reader of the card cache entry learns each kind's refresh age, how fetch times are recorded, and how to force fresh data.

## Open Questions

- **Does any game entry point put cards into play without loading them first?** The known paths load every card before the game's lock is taken: building from decklists, importing, and the MCP server's prefetch after import and new game. Once `Game.card` stops fetching, a path that skips this would show those cards by name until the background warm finishes. Whether such a path exists (for example, a board-side action that introduces a new definition) only shows up when the change runs against the full test suite and the board's e2e suite. *What to do:* if one turns up, add a prefetch of the new definitions outside the game lock on that path. Do not restore a blocking fetch under the lock. Stop and ask the user only if the prefetch cannot be placed outside the lock.

No other questions are left for implementation time: every other decision is settled in the sections above or recorded as a drafting assumption.

## Out of Scope

- **How cards are matched, and which print is preferred**, stay as they are (spec Non-Goal). Print ordering and set-code collision rules move into the catalog without change.
- **The DigitalOcean Spaces tier** and any tiered store are left to the next spec in this epic, `20261104082020-109dceb1-tcgdex-spaces-cache`. This plan only provides the `Store` seam it plugs into.
- **Serving card images through the playmat or a CDN** is left to `20261104090640-25f673ed-card-image-cdn`. The board still loads images from TCGdex.
- **An admin page or CLI flag to clear or refresh the cache on demand** is left to `20261104090640-a5a74fab-admin-cache-clearing`. Until then, deleting the cache directory forces fresh data.
- **Pushing refreshed card data to boards already open.** A board shows refreshed artwork on its next load; the board's front end and its update protocol do not change.
- **Persisting the failed-card and repeated-miss memory across restarts.** It is kept in memory only, so a restart may ask the card source once more.
- **Downgrading to an earlier release after running this one.** Rewritten entries use the new envelope, which earlier releases cannot read. Deleting the cache directory recovers.
- **Configurable refresh ages** (flags or environment variables). The spec fixes them.
- **Refreshing on by-name misses.** Only the two misses the spec names refresh before answering: an unknown set code, and a number missing from a set. A name absent from the index is answered from the current catalog, which background refreshes keep current.
- **Other card sources or fallbacks** (malie.io, borrowed prints' art) are not part of this epic's freshness work.

## Changelog


### 2026-11-04 — Task: Add the cache store with fetch times

**What was done**: Added `internal/cards/store.go` with the exported `Entry`, `ErrNotCached`, `Store` interface and `NewDiskStore`. The disk store maps keys to today's file paths (via `safeName`), writes a `{"fetched":…,"data":…}` envelope atomically (temp file in the same directory, then rename), and reads files without the envelope as legacy entries aged by their modification time.

**Deviations**: The envelope is written by hand rather than marshalled, so the body is kept byte for byte (json.Marshal would compact a `RawMessage`). The temporary file is made with `os.CreateTemp` instead of a fixed `<file>.tmp`, so two concurrent puts of the same key cannot clobber each other's temp file. `Put` refuses a body that is not valid JSON. Nothing in `cards.go` is wired to the store yet (next task).

**Files changed**:
- `internal/cards/store.go`
- `internal/cards/store_test.go`

**Discoveries**: The project's Go tests use the standard `testing` package, not testify. `safeName("../x")` gives `__x` (both `..` and `/` are replaced). `internal/mcpserver` `TestGuideInstructions/desktop/inmemory` is flaky (TempDir cleanup "directory not empty"), unrelated to this work.

### 2026-11-04 — Task: Route card lookups through the store

**What was done**: Replaced `getJSON` with one load path, `loadEntry` (store, then card source) plus `fetchEntry` (card source, then store), keyed `sets`, `sets/<id>` and `cards/<id>`. `Card`, `Cached`, the set loads, the set index rebuild and the name index all go through it, and every fetched entry is stored with the client clock's time. `Client` gained `Store`, `Timeout` and `Now` (defaults set by `NewClient`, with nil fallbacks), and the in-memory card cache now keeps each card's fetch time. Added the `httptest` fake TCGdex, a fake clock and cache-seeding helpers for later tests.

**Deviations**: `loadEntry` takes an `out any` and decodes into it, storing a fetched body only once it decodes (as `getJSON` did), rather than returning only the raw entry. `set` keeps its two-value signature because `TestByNameCached` calls it; the fetch time comes from a new `setFetched`. `Client.Timeout` is set but not yet applied to requests: the next task wires it through the single-flight group.

**Files changed**:
- `internal/cards/cards.go`
- `internal/cards/fake_tcgdex_test.go`
- `internal/cards/store_client_test.go`

**Discoveries**: `TestByNameCached`, `TestLogCodesCached` and `TestResolve30th` run offline against the developer's real `~/.cache/tcgreplay` when it exists, so they exercise a genuine previous-release cache. Offline `Resolve` still needs the persisted `set-index-v2.json` until the catalog task replaces it. `cmd/tcgreplay` `TestHostedGuidesMatchDesktop` fails on main at 8c26660 (7 desktop guides, test expects 6), unrelated to this spec.

### 2026-11-04 — Task: Deduplicate fetches and remember failed cards

**What was done**: Added an in-package single-flight group (`internal/cards/flight.go`) whose work runs on a detached context bounded by `Client.Timeout` (5 s by default), so a caller giving up returns early without cancelling the shared request. Every source fetch now goes through it (`Client.shared`), which validates the body as its kind, stores it, and re-checks the store first for lookups so a late joiner does not refetch. Added the refresh tracker (`internal/cards/fresh.go`, `RetryAfter = 5m`) with a failed-card map: a card the source fails to return (404, error, timeout, unreachable) is remembered, and `Card` answers `recently failed: ErrNotFound` at once for 5 minutes without contacting the source. Offline never reaches the source or the tracker.

**Deviations**: The failure is recorded inside the shared fetch rather than by each caller, so a caller whose own context ends is never counted as a failure of the card, and the outcome is recorded once even when nobody is still waiting. `Card` checks the store before the failed-card memory, so a card present on disk is always served. A lookup that joins a request already in flight waits for it (up to the 5 s bound); only lookups after the failure answer at once.

**Files changed**:
- `internal/cards/flight.go`
- `internal/cards/fresh.go`
- `internal/cards/cards.go`
- `internal/cards/flight_test.go`

**Discoveries**: `golang.org/x/sync/singleflight` was not needed; the group is ~40 lines. A body that is valid JSON but does not decode as its kind is refused before it reaches the store (`validateEntry`), preserving the old `getJSON` guarantee now that decoding happens per caller.

### 2026-11-04 — Task: Build the card catalog outside the database lock

**What was done**: Added `internal/cards/catalog.go`: an immutable `catalog` (set list fetch time, set ids, every set's card list with raw body and fetch time, the set-code index and the by-name index), built by `ensureCatalog` under the single-flight key `catalog` with no lock held, and swapped in whole under `Client.mu`. `indexes` is a pure function carrying the old set-code collision rule (shorter id wins) and print ordering verbatim. `Resolve` and `prints` read the catalog; `buildIndex`, `buildNameIndex`, the `setIndex`/`indexed`/`byName` fields and every read or write of `set-index-v2.json` are gone. `Client.mu` now carries a comment that it guards only in-memory maps and the catalog pointer. The interim once-per-process forced set-list refresh for an unknown code is kept (`refreshSetList`, `forced` flag) until the miss policy replaces it.

**Deviations**: An alias code (`setAliases`) still resolves without building the catalog, as before, using an already built catalog's set when there is one. A failed catalog build is not cached, so the next lookup tries again. Tests were written by a sub-agent (High complexity), as the workflow requires.

**Files changed**:
- `internal/cards/catalog.go`
- `internal/cards/cards.go`
- `internal/cards/catalog_test.go`

**Discoveries**: Against the real previous-release cache (`~/.cache/tcgreplay`, 220 sets), the in-memory set-code index equals the persisted `set-index-v2.json` exactly (200 codes, none differ), so retiring the file changes no resolution. Offline `Resolve` now needs `sets.json` and the set files rather than the index file; a previous release always wrote `sets.json` before the index, so no real cache is affected.

### 2026-11-04 — Task: Keep card fetches out of the game lock

**What was done**: `Game.card`, always called with `g.mu` held, now reads only `DB.Cached`. On a miss it calls the new `Game.warm`, which starts one background `DB.Card` fetch per def at a time (deduplicated by a `warming sync.Map` field, 5 s timeout, never touching `g.mu`), and treats the card as unknown for that call. The blocking 5 s `db.Card` under the game lock is gone.

**Deviations**: The browser e2e suite was not run for this task: its board test server builds games over `cardstest.FakeDB` (`internal/webui/webtest/webtest.go:78`), whose `Cached` never misses, so the change cannot alter its behaviour. The plan's open question (a game path that puts cards into play without loading them first) did not show up: the full Go suite, including webui and mcpserver, passes unchanged.

**Files changed**:
- `internal/game/game.go`
- `internal/game/cardlock_test.go`

**Discoveries**: With the real `cards.Client`, `Cached` still reads the local disk store on a memory miss while `g.mu` is held. That is local I/O only, never the card source, and games warm their cards before the lock is taken (`BuildCards`, `PrefetchCards`).

### 2026-11-04 — Task: Refresh stale cards in the background

**What was done**: Added the refresh ages to `internal/cards/fresh.go` (`SetListMaxAge` 30 days, `SetMaxAge` 90 days, `ImagelessCardMaxAge` 24 hours), `cardStale` (no artwork as served, older than 24 hours), the tracker's `bg` map and `maybeRefresh`, which starts one background refresh per key per 5 minutes through the single-flight fetch, tracked by `Client.bgwg`, and does nothing offline. `Card` and `Cached` return a stale card at once from memory or the store and start its refresh; a successful refresh stores the new envelope and swaps the in-memory card. `waitBackground` lets tests wait for refreshes instead of sleeping.

**Deviations**: `remember` now keeps whichever copy was fetched later and returns it, so a lookup that read an older copy from disk cannot overwrite a card a background refresh has just swapped in.

**Files changed**:
- `internal/cards/fresh.go`
- `internal/cards/cards.go`
- `internal/cards/fresh_test.go`

**Discoveries**: `Cached` is called on every board read (through `Game.hp`), so the stale check there must stay cheap: it is a comparison plus, for a stale card only, one map lookup under `Client.mu`.

### 2026-11-04 — Task: Refresh set data and check misses against fresh data

**What was done**: The catalog now records when it first holds stale data (`staleAt`: set list at 30 days, oldest set at 90 days), and `ensureCatalog` starts one background `sweep` once that passes (one at a time, at most once per 5 minutes, never offline). The sweep refetches a stale list, loads newly listed sets, refetches stale sets (each key throttled by the tracker), and hands the results to `update`, which merges them into the current catalog keeping the newest copy of each entry and recomputes the indexes only when a body or the list changed, so `Resolve` and `ByName` follow new sets without a restart. `Resolve` checks misses against fresh data: an unknown code refreshes the set list (`refreshSetList`), a number missing from a set refreshes that set (`refreshSet`), each distinct miss at most once per 5 minutes (tracker `miss` map). Failed refreshes keep the stale copies. The once-per-process forced rebuild is gone.

**Deviations**: `update` merges by fetch time under a small writer mutex (`catMu`, held for CPU work only) rather than the sweep publishing its own snapshot, so a miss refresh and a sweep racing each other cannot drop each other's data. Sweep throttling uses one `sweep` key in the tracker as well as the per-key throttle. After a miss refreshes the set list, the list key is marked in the tracker so an overlapping sweep does not fetch it again. `missAllowed` records the miss before refreshing, so a miss whose refresh fails is not retried for 5 minutes either. The analyze step's research was not delegated again: the files were the ones this run had just written in the catalog task. Tests were written by a sub-agent (High complexity).

**Files changed**:
- `internal/cards/catalog.go`
- `internal/cards/fresh.go`
- `internal/cards/cards.go`
- `internal/cards/setfresh_test.go`

**Discoveries**: A newly listed set whose old copy is already in the store is loaded from the store rather than refetched; if that copy is past 90 days the next sweep refreshes it. `internal/mcpserver` `TestGuideInstructions/*/inmemory` is flaky (TempDir cleanup), independent of the card cache.

### 2026-11-04 — Task: Prove the playmat plays on with the card source down

**What was done**: Added `internal/game/cardsource_test.go`, an end-to-end manager test with a real `cards.Client` over a `t.TempDir()` cache whose envelopes are all 400 days old, against a card source that drops every connection (and, as a second case, answers slower than the timeout). The game builds from `testDeck` with set codes, with no error and exactly the warnings a fresh cache gives; Iono's text and Riolu's art come from the stale copies; board reads each return within one second. A deck holding a card listed in a cached set but not cached itself (Ghostmon, `tst-9`) keeps it by name only, and ten rounds of `View` and `SearchDeck` make no further request for it.

**Deviations**: The unreachable source is an `httptest` server that hijacks and closes each connection rather than a closed server, so requests per path can still be counted. A slow-source case was added alongside it, matching the spec's "answering only after more than 5 seconds". One run of `go test -race ./internal/game ./internal/cards` reported a failure in `internal/game` whose output was not captured; it did not reproduce in nine further runs (including `-count=4` of the package and `-count=40` of the new tests).

**Files changed**:
- `internal/game/cardsource_test.go`

**Discoveries**: None beyond the unreproduced failure above, which is worth watching for in CI.

### 2026-11-04 — Task: Update the card cache knowledge entries

**What was done**: Rewrote the repo-tier knowledge entry `gotchas/tcgdex-cache-never-expires.md` (store `tcgreplay`) as "The TCGdex cache refreshes by kind": per-kind refresh ages, stale-while-revalidate and stale-if-error with the 5 s bound, misses checked against fresh data once per 5 minutes, the 5-minute failed-card memory (in memory only), single-flight, offline serving any age, `Game.card` never fetching, the envelope format with legacy files aged by mtime, no persisted set index, and how to force fresh data. Corrected `gotchas/tcg-live-log-card-codes.md` so it no longer tells readers to bump `set-index-v2.json`: the set-code index is rebuilt in memory from cached sets on each start.

**Deviations**: The cache entry keeps its path (`tcgdex-cache-never-expires.md`) so existing references to it still resolve; only its title changed, and its body notes the name is historical.

**Files changed**:
- knowledge (repo tier, store `tcgreplay`): `gotchas/tcgdex-cache-never-expires.md`
- knowledge (repo tier, store `tcgreplay`): `gotchas/tcg-live-log-card-codes.md`

**Discoveries**: None.
