---
tags: [cards, tcgdex, cache, images, hosting, spaces, cdn, admin]
---

# The TCGdex cache refreshes by kind

`internal/cards` caches every TCGdex response through a `cards.Store`
(`internal/cards/store.go`; the disk store under `cards.DefaultCacheDir()`,
`~/.cache/tcgreplay`, or `-cache`). Keys and files: `sets` → `sets.json` (the
set list), `sets/<id>` → `sets/<id>.json` (each set's card list), `cards/<id>`
→ `cards/<id>.json` (each print). (This file's name is historical: until late
2026 the cache never expired.)

**Refresh ages** (`internal/cards/fresh.go`, constants, not settings):

- set list: 30 days (`SetListMaxAge`)
- a set's card list: 90 days (`SetMaxAge`)
- a card without artwork: 24 hours (`ImagelessCardMaxAge`), judged on the card
  as served, so Basic Energy with borrowed art (`energyArt`) counts as having art
- a card with artwork: never

**How it behaves.**

- Stale data is served at once and refreshed in the background
  (stale-while-revalidate). The next lookup after the refresh sees the new data;
  an open board shows it on its next load (see
  `gotchas/board-card-info-per-page-load.md`).
- A refresh that errors or takes longer than 5 s (`Client.Timeout`) keeps the
  stale copy and is not retried for 5 minutes (`RetryAfter`).
- A miss checks fresh data before answering: an unknown set code refetches the
  set list, a number missing from a set refetches that set, each distinct miss
  at most once per 5 minutes. An unknown *name* does not force a refresh.
- A card TCGdex fails to return (404, error, timeout) is remembered as failed
  for 5 minutes: lookups answer `recently failed` at once with no request.
- Simultaneous requests for one key share one request (an in-package
  single-flight). This retry/failure memory is in memory only, so a restart may
  ask TCGdex once more.
- Offline (`-offline`) never contacts TCGdex and serves every cached entry
  whatever its age.
- `Game.card` (called under the game lock) never fetches: an uncached card is
  unknown for that call and is warmed in the background.

**Fetch times.** New files are an envelope,
`{"fetched":"<RFC 3339>","data":<TCGdex body>}`, so the fetch time travels with
the entry (it must survive copying between cache tiers). A file without the
envelope, written by an earlier release, is read with its modification time as
its fetch time, and is rewritten as an envelope only when it is next refreshed.
A release from before this change cannot read envelope files: downgrading needs
the cache deleted.

**No persisted index.** The set-code index and the by-name index are rebuilt in
memory from the set list and set files (`internal/cards/catalog.go`), and
rebuilt again whenever a refresh changes the list or a set. An old
`set-index-v2.json` (or `set-index.json`) left on disk is ignored; nothing
deletes it.

**Hosted shared tier (DigitalOcean Spaces).** With all four
`TCGREPLAY_SPACES_ENDPOINT`, `_BUCKET`, `_KEY` and `_SECRET` set, hosted mode
(only; desktop, daemon and dev never read them) wraps the disk store in a
tiered store (`cards.NewTieredStore`, `internal/cards/tiered.go`) over a Spaces
store (`internal/cards/spacesstore.go`, client in `internal/spaces`). Every
entry is kept in the bucket under `tcgdex/` at the disk path, in the same
envelope bytes: `tcgdex/sets.json`, `tcgdex/sets/<id>.json`,
`tcgdex/cards/<id>.json`. The hosted cache directory is then only a per-run
copy: a disk miss is read from Spaces and copied to disk with its original
`fetched`, so an entry's age (and staleness) survives a restart; every fetch or
refresh is written to disk and then Spaces. None set: local only, one log line.
Some set or invalid: a warning and local only. At start-up the catalog (set
list and every set's card list) is warmed in the background.

- `Cached` (under the game lock) and offline mode read only the local tier
  (`Client.localStore`), never Spaces: a card held only in Spaces shows by name
  until the background warm's `Card` lookup copies it to disk.
- A Spaces failure (unreachable, 403, 5xx, a request over 3 s) never reaches a
  lookup: it is logged once ("shared store unavailable"), Spaces is skipped for
  1 minute (`SharedRetryAfter`) and the next request probes it; recovery is
  logged once. Writes made while it is skipped are not queued: they reach Spaces
  on the entry's next fetch or refresh. A 404 is a normal miss; if a real key
  ever answered 403 for missing objects, every miss would trip the breaker.

**To force fresh data on the hosted playmat**, an admin clears the cache from
`/admin` (accounts in `TCGREPLAY_ADMINS`, verified Google email; behind
`hostauth.RequireAdmin`; `internal/cacheadmin`). This replaces deleting bucket
objects by hand, which no longer works on its own (memory and the image cache's
in-memory copies would keep pointing at the old data). A clear covers
**everything** or **one set**, named by decklist code (`MEG`) or TCGdex id
(`me01`, any case; `cards.Client.SetID`, which refuses a name matching no set
after one set-list refresh). Scope (`cards.Scope`, `internal/cards/scope.go`):
set S is `sets/S` plus every `cards/S-…` key (the `-` keeps `sv03` from
matching `sv03.5`), and images whose TCGdex path has S as its third segment
(borrowed Crown Zenith Energy art belongs to `swsh12.5`). It reaches every
tier, each through its owner: memory, the disk store, Spaces `tcgdex/`
(tiered store clears shared first, then local, outside the breaker), the
`images/` copies, and a CDN purge when `TCGREPLAY_SPACES_CDN_TOKEN` is set. It
runs as one background job and reports per tier; a failed tier can be cleared
again (clears are idempotent).

Open games keep their cards: a clear **marks** memory entries cleared rather
than dropping them. `Cached` (under the game lock) still returns a cleared copy
and warms it in the background; `Card`, `Resolve` and `ByName` treat it as a
miss and fetch from TCGdex at once, keeping the old copy only if TCGdex fails.
A clear also drops the catalog and the in-scope retry/miss throttles, and moves
a clear generation on, so a fetch or catalog build begun before the clear is
not kept in memory. Desktop has no admin page: delete the cache directory (or
the affected files) and restart. Without the shared store the hosted image has
no volume for `/home/nonroot`, so its cache starts empty on each redeploy.

**Card images.** Desktop, daemon, dev and hosted-without-the-shared-store
boards load images straight from `assets.tcgdex.net` (`<image>/low.webp` on the
mat, `/high.webp` in the zoom and preview); the playmat never fetches them.
Hosted mode with the shared store copies each image the board shows
(`cards.ImageCache`, `internal/cards/images.go`) byte for byte to
`images/<TCGdex path>/low.webp` and `high.webp` in the same bucket, public-read
with `Cache-Control: public, max-age=604800`, and keeps them until an admin
clears them: only `ImageCache.Clear` deletes image objects, and the cache never
writes over one already in the bucket. `/api/card` then answers a copied image with its CDN
address (`<bucket>.<region>.cdn.digitaloceanspaces.com/images/...`) as `image`
and the TCGdex address as `image_fallback`, and an image without a copy with
the TCGdex address, queuing a background copy. Only addresses under
`https://assets.tcgdex.net/` are copied. The playmat learns which copies exist
by listing `images/` at start-up and remembers them in memory, each with a
content version (a hash of the two objects' ETags, the MD5 of their bytes),
sent as `image_version` and put in the board's CDN address as `?v=`, so changed
art gets a new URL past the week-long browser and CDN caches (see
`gotchas/board-card-info-per-page-load.md`). Deleting image objects by hand
leaves the cache pointing boards at them until a restart; use the admin clear,
which stops pointing at them at once, deletes them, purges the CDN and lets the
next view copy the current art. `/api/card` is served `no-cache`, so the board
revalidates it every load.

