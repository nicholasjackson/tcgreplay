---
created_date: "2026-11-04"
---

# Planning summary: 20261104082020-109dceb1-tcgdex-spaces-cache

## Decisions
- **admin-cache-clearing extends card-image-cdn's contracts.** Plans involved: card-image-cdn, admin-cache-clearing. The admin plan adds `ETags` to `spaces.ListPage`, a version to `webui.ImageSource.Image` and `image_version` to `/api/card`. Outcome: accepted as additive extensions made by the later spec. card-image-cdn's plan is unchanged and is implemented first. Settled by the orchestrator under the user's "just do the whole thing".
- **Changelog handling.** Plans involved: all four. Every plan uses the default implement workflow's changelog. Outcome: no disagreement.

## Order added for shared files
None added.

## 20261104082020-109dceb1-tcgdex-spaces-cache
### Approach
On the hosted instance, a tiered `Store` (local disk, then DigitalOcean Spaces) goes into the freshness plan's store slot.
- **Reads:** a disk miss is served from Spaces and copied to disk with `Fetched` kept.
- **Writes:** go to disk, then to Spaces.
- **Failures:** a breaker (3 s per request, Spaces skipped for 1 minute) absorbs outages, and the log records each outage once.
- **No-network reads:** `Local()` keeps `Cached` (under the game lock) and offline mode on local disk.
- **S3 client:** a new standard-library client in `internal/spaces` (SigV4 signing, path-style addresses), with an in-process fake in `spacestest`.
- **Start-up warm:** `Client.Warm` builds the catalog in the background at hosted start-up.
### Milestones and tasks
- M1, the playmat can read and write Spaces: request signing and settings; the client and its fake.
- M2, card data outlasts restarts: the Spaces card store; the tiered store with its breaker; local-only reads under the game lock and offline; restart acceptance tests.
- M3, hosted keeps its cache across deploys: hosted wiring with `Warm`; settings and documentation; the knowledge entry; the bucket (human).
### Tasks a person must do
- Create the Spaces bucket and access key, and set the four `TCGREPLAY_SPACES_*` secrets in App Platform.
### Out of scope
- Desktop and dev builds using Spaces.
- Games, game records or sign-ins in Spaces.
- Card images and clearing (later specs).
- Avoiding the first cold build.
- Queuing writes made during an outage.
- The CDN.
### Drafting assumptions
- A standard-library S3 client rather than minio-go or the AWS SDK.
- Settings are read from the environment in hosted mode only; partial settings warn and run local-only.
- Writes to Spaces are synchronous.
- Card data uses the fixed prefix `tcgdex/`, private.
- `Warm` runs only when Spaces is configured and the playmat is online.
### Project-wide rules
- Decides optional `Local() Store` on tiered stores, found by type assertion.
- Decides the bucket layout: `tcgdex/` at the disk paths, private, `application/json`.
- Decides `internal/spaces` (Config, FromEnv, New, Get, Put with PutOptions, ErrNotFound, *Error) as the shared client, with `spacestest` as the shared fake.
- Decides the settings `TCGREPLAY_SPACES_ENDPOINT`, `_BUCKET`, `_KEY`, `_SECRET`, environment and hosted only.
- Decides the breaker: 3 s per request, 1 minute skip, edge-triggered logs.
### Manual checks
- The 2 s first-import-after-restart metric.
- Real Spaces end to end: import, redeploy, import again.
- Real failures: a wrong secret and an unreachable endpoint.
- A missing key on the real bucket is a 404, not a breaker trip.
- A hosted start with no settings logs one local-only line.

## 20261104090640-51bd1f36-card-cache-freshness
### Approach
`internal/cards` is split into three layers:
- **Store:** an exported `Store` interface (Get/Put of an `Entry{Data, Fetched}`) with a disk implementation. New files hold an envelope recording the fetch time; older files use their modification time.
- **Freshness, in `cards.Client`:**
  - Refresh ages: the set list 30 days, a set's card list 90 days, a card without art 24 hours, a card with art never.
  - Stale data is served at once and refreshed in the background, and the stale copy is kept if the refresh fails.
  - A miss (unknown set code, or a card number missing from a set) refreshes before answering, at most once every 5 minutes. Failed cards are remembered for 5 minutes.
  - Every TCGdex request has a 5 s limit, and simultaneous requests for the same data share one.
- **Catalog:** an in-memory snapshot of the set list, each set's cards and the two indexes, built outside the database lock and replaced whole.

`Game.card` no longer reaches the network while holding the game lock.
### Milestones and tasks
- M1, entries record their fetch time: add the store; route lookups through it.
- M2, the board never waits: share duplicate fetches and remember failures; build the catalog outside the lock; keep card fetches out of the game lock.
- M3, data refreshes on its own: background refresh of cards; set-data refresh and the miss rule; a game-level test with the card source down; update the knowledge entries.
### Tasks a person must do
None.
### Out of scope
- Card matching and print preference.
- The Spaces tier, image copies and admin clearing (later specs).
- Pushing refreshed data to boards already open.
- Remembering misses and failed cards across restarts.
- Configurable refresh ages.
### Drafting assumptions
- Fetch time is stored in a JSON envelope, with modification time as the fallback for old files.
- Downgrading after the upgrade is unsupported.
- `set-index-v2.json` is retired.
- Every TCGdex request has a 5 s limit.
- Duplicate requests are shared by an in-house helper.
- `Game.card` reads only the cached copy, starting a fetch in the background on a miss.
### Project-wide rules
- Decides the `Store` contract, the keys `sets`, `sets/<id>` and `cards/<id>`, and that `Fetched` is kept when entries are copied.
- Decides the on-disk envelope `{"fetched","data"}`.
- Freshness lives above the store; offline mode never contacts any source.
- No persisted set index.
- The `DB` interface is unchanged.
- Nothing under the game lock calls the network.
### Manual checks
- The 95%/200 ms board-read metric over a week of hosted use.
- An upgrade check against a real cache from the previous release.
- Live TCGdex: a new set resolves on the first import, and late art appears after a reload.

## 20261104090640-25f673ed-card-image-cdn
### Approach
The playmat decides each image's address.
- **Image cache:** a new `cards.ImageCache` keeps an in-memory list of images with a public copy under `images/` in Spaces, filled by listing the bucket at start-up.
- **Card endpoint:** `/api/card` returns the CDN address with the TCGdex address in `image_fallback`. An image not yet copied gets the TCGdex address and a background copy.
- **Board:** each image tries `image`, then `image_fallback`, then the text stand-in.
### Milestones and tasks
- M1, the board shows an image from a second address: the image fallback on the card endpoint; the board fallback with a Playwright test.
- M2, public copies: list the bucket and derive its public address; copy images in the background; find existing copies at start-up and count image answers.
- M3, hosted images from the CDN: hosted wiring; operator documentation; knowledge entries; enable the CDN (human).
### Tasks a person must do
- Enable the CDN on the Spaces bucket and check the access key can list and write public objects.
### Out of scope
- Resizing images, and any image source other than TCGdex.
- Clearing and purging (admin-cache-clearing).
- The CDN on desktop.
- Pushing new addresses to boards already open.
- A custom CDN domain.
- Bulk-copying images no board has shown.
### Drafting assumptions
- The CDN address is derived as `https://<bucket>.<region>.cdn.digitaloceanspaces.com`, with no new setting.
- Both `low.webp` and `high.webp` are copied, with a 7-day Cache-Control.
- The copier runs 2 at a time, with a queue of 256.
- An hourly log line counts CDN and TCGdex answers.
### Project-wide rules
- Decides `spaces.Client.List(ctx, prefix, token) -> ListPage{Keys, Next}` and `Config.PublicBase()`.
- Decides the image prefix `images/<asset path>/{low,high}.webp`, public-read, 7-day cache; `tcgdex/` stays private.
- The fake gains listing, unsigned public reads and `Public(key)`.
- `/api/card` gains `image_fallback`, and `webui.Server` gains `Images`.
### Manual checks
- The 95% CDN metric over a week.
- Real CDN end to end.
- `tcgdex/` objects stay private.
- Images fall back with the CDN blocked.
- No shared store: images load from TCGdex as today.
- A restart keeps the copies.
- Real iOS Safari.

## 20261104090640-a5a74fab-admin-cache-clearing
### Approach
Each layer clears itself for one scope (everything, or one TCGdex set id) through optional `Clear` interfaces, so `Store` does not change.
- **Card database:** `cards.Client.Clear` marks cards in memory as cleared rather than deleting them, so open games keep playing. `Cached` still returns the old copy while the new one is fetched; `Card` fetches now and falls back to the old copy.
- **Images:** `ImageCache.Clear` deletes the scope's copies and purges the CDN through the DigitalOcean API. Board image addresses carry `?v=<content hash>`, so browsers cannot keep old art past a clear.
- **Admin page:** `TCGREPLAY_ADMINS` names admins (verified Google emails), and `hostauth.RequireAdmin` guards a small server-rendered page in `internal/cacheadmin` that runs one clear at a time as a background job.
### Milestones and tasks
- M1, delete and purge: batch delete with checksums; the CDN purge client.
- M2, clear without interrupting play: clear the stores; clear the card database and name sets; prove an open game survives.
- M3, cleared images return as current art: version image copies; send versions to the board; clear copies and purge the CDN.
- M4, the admin page: admin guard; page and clear job; hosted wiring; settings and documentation; knowledge entries; admins and the CDN token (human).
### Tasks a person must do
- Name the admins in `TCGREPLAY_ADMINS`, and create a DigitalOcean API token with CDN access as `TCGREPLAY_SPACES_CDN_TOKEN`.
### Out of scope
- Clearing a single card, and other admin capabilities.
- Pushing cleared data to open boards.
- An admin page on desktop.
- Clearing TCGdex's own caches.
- Scheduled clears or an audit history.
- Re-warming after a clear.
### Drafting assumptions
- Image addresses carry `?v=` from the objects' ETags, on top of the purge.
- Cleared cards stay visible to `Cached`.
- One background clear at a time.
- Sign-ins from before this change count as unverified until the admin signs in again.
- Clears bypass the lookup breaker (30 s per request, 10 minutes per job).
- Partial failures are reported per tier, and clearing again is safe.
### Project-wide rules
- Adds `spaces.Client.Delete` (batch) and a `spaces.CDN` purge client.
- Extends `ListPage` with `ETags`.
- Extends `ImageCache` with versions and `Clear`; `webui.ImageSource.Image` gains a version, and `/api/card` gains `image_version`.
- Decides the settings `TCGREPLAY_SPACES_CDN_TOKEN` (secret) and `TCGREPLAY_ADMINS`.
- `hostauth.Account` gains `EmailVerified`.
### Manual checks
- Clear to corrected card in under a minute.
- Real admin sign-in and refusal for other accounts.
- A real clear of everything.
- Real set clears by code and by id, and an unknown code refused.
- A real CDN purge, with current art within 1 hour.
- An open game survives a real clear.
- Real Spaces batch delete, ETags and versioned CDN addresses.
