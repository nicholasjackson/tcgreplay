---
created_date: "2026-11-04"
document_status: draft
project: tcgreplay
spec: 20261104090640-51bd1f36-card-cache-freshness
plan: 20261104090640-51bd1f36-card-cache-freshness
---

# Card cache freshness

The playmat now keeps its card data up to date by itself. New sets, new promos and artwork TCGdex adds later show up without clearing the cache: the set list refreshes after 30 days, each set's card list after 90 days, and a card without artwork after a day, while cards with artwork are kept. Stale data is shown at once and refreshed in the background, the last good copy is kept when TCGdex is down or slow, and board reads and plays never wait on a card download. Offline mode works as before, and caches from earlier releases keep working.

> Derived from project tcgreplay, spec/plan 20261104090640-51bd1f36-card-cache-freshness. See the project-level record for the full feature.

## What changed in this repo

- `internal/cards/store.go` (new): `Entry`, `ErrNotCached`, the `Store` interface and the disk store, writing atomic `{"fetched","data"}` envelopes at the existing paths and reading earlier releases' bare files with their modification time as fetch time.
- `internal/cards/flight.go` (new): single-flight group on a detached context bounded by `Client.Timeout` (5 s by default).
- `internal/cards/fresh.go` (new): refresh ages (`SetListMaxAge`, `SetMaxAge`, `ImagelessCardMaxAge`), `RetryAfter`, the in-memory tracker of failed cards, background refreshes and misses, and background card refresh.
- `internal/cards/catalog.go` (new): immutable catalog of the set list, set card lists and both indexes, built off-lock, swept in the background when stale, refreshed on misses, merged by fetch time.
- `internal/cards/cards.go`: `Client` gains `Store`, `Timeout` and `Now` (all defaulted by `NewClient`); every lookup goes through the store and the single-flight fetch; `Resolve` and `ByName` read the catalog; the persisted `set-index-v2.json` is no longer read or written.
- `internal/game/game.go`: `Game.card` never fetches under the game lock and warms missing cards in the background.
- Tests: `internal/cards/{store,store_client,flight,catalog,fresh,setfresh}_test.go` with a fake TCGdex (`fake_tcgdex_test.go`), `internal/game/cardlock_test.go` and `internal/game/cardsource_test.go`.

## Why

The card cache used to keep every TCGdex answer forever, so new sets failed to resolve and late artwork never appeared until someone deleted the cache, and a cold index build or a failing card could freeze every board read. This repo holds the card database and the game, so both changes land here.

## Upgrade notes

- Existing caches keep working; entries are rewritten in the new format only when refreshed. An old `set-index-v2.json` is ignored.
- Downgrading to an earlier release after running this one needs the cache directory deleted (earlier releases cannot read the new format).
