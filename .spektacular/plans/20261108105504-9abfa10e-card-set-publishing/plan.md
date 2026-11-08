---
created_date: "2026-11-08"
document_status: final
closed_date: "2026-11-08"
---

# Plan: 20261108105504-9abfa10e-card-set-publishing

<!-- Metadata -->
<!-- Created: 2026-11-08T11:48:24Z -->
<!-- Commit: 1b8fc21 -->
<!-- Branch: main -->
<!-- Repository: git@github.com:nicholasjackson/tcgreplay.git -->

## Overview

Built card sets are published from the repository to the playmat's existing Spaces bucket and CDN, as unchangeable compressed set files plus a small manifest, by a `tcgreplay sets publish` command and a GitHub Action on every merge that changes card data. Every playmat, hosted and desktop, then loads its card definitions only from those published sets, picks up a publish within minutes without a restart or at once from the admin page, lets games already open keep the cards they started with, and keeps a local copy so desktop plays offline. The on-demand TCGdex card cache is retired, so players never see half-finished upstream data, and new sets, promos, art and text fixes reach players with no release of the playmat.

## Conventions

No project conventions apply to this feature. The only convention in the knowledge base (`conventions/skills-text-and-packaging.md`) governs the coaching guides in `internal/guides` and how they are embedded and packaged; this feature changes neither the guides nor their embedding, and its `.dockerignore` and Dockerfile edits leave `internal/guides` in the build context.

## Architecture & Design Decisions

All work lands in the single registered repo, `tcgreplay` (`/home/nicj/code/github.com/nicholasjackson/tcgreplay`). The feature has two halves joined by a published layout in the existing DigitalOcean Spaces bucket. **Publishing** is a new `tcgreplay sets publish` command (beside the card-set-import plan's `sets import` and `sets check`) and a GitHub Action that runs it on every push to `main` that changes `cardsets/**`. It refuses to publish while `sets check` finds an open item. It uploads each built set as an immutable, content-addressed, gzipped object, `cardsets/sets/<set-id>/<16-hex sha256 of the built file>.json.gz`: public-read, cached for a year, never overwritten or deleted, so every earlier version stays retrievable byte for byte. It then rewrites one small manifest, `cardsets/manifest.json`, which names each set's current object, its hash and its print count. The manifest is cached for a minute and purged from the CDN after each publish. A set whose hash matches the published manifest is not uploaded, and a publish with nothing changed writes nothing. **Loading** is a new `internal/catalog` package. It fetches the manifest anonymously over HTTPS from a configured base address (by default the project bucket's CDN; `-cardsets-url` or `TCGREPLAY_CARDSETS_URL` overrides it). It fetches only the sets whose hash changed, verifies each one against its hash, converts the prints with the card-set-import plan's `Print.Card("en")`, and swaps in a new immutable snapshot. It checks at start, every 5 minutes (`-cardsets-interval`, comfortably inside the 10-minute acceptance limit) and when an admin presses "Check now" on `/admin`. In every mode it keeps a content-addressed copy of what it loaded under `<cache dir>/cardsets/`, with the manifest written last. That copy is what desktop plays from when it cannot reach the CDN, and `-offline` uses it alone. Desktop needs no Spaces credentials, because only the publisher signs requests ("data in the bucket; logic and presentation in the program").

**Open games keep their cards through snapshot pinning, not copying.** A snapshot implements the existing `cards.DB` interface (`Card`, `Cached`, `Resolve`, `ByName`). It holds every card already converted, the set-code index, the by-name index and set-name lookup, all built by the same pure rules the current catalog uses: set-code collisions keep the shorter id, Pocket sets are skipped, by-name order prefers art then newer release then lower number, and `setAliases`/`codeAliases` and Basic Energy borrowed art behave as today (a borrowing print takes the other print's whole image list). Unchanged sets' converted cards are shared between snapshots, so an older snapshot pinned by an open game costs only the sets that changed. Games already look every definition up through the `cards.DB` handed to `game.Load` and store only def ids, so the game manager now takes the live catalogue and captures `Current()` once at the start of `New`, `Import` and `SetOpponentDeck`. It uses that one snapshot both to resolve the deck and log and as the game's DB, so an import can never mix versions. `card_text` and the board's `/api/card` (with `borrowedArt`) read through the current game's snapshot, falling back to the current one when no game is open. A game started, imported or reopened after a pickup gets the new definitions. An open board sees nothing change until its game is replaced, matching how the board already treats card data per page load (knowledge `gotchas/board-card-info-per-page-load.md`). A playmat with no reachable storage and no local copy has an empty snapshot, so every card shows as unknown. Hosted and desktop wait up to 30 seconds at start for the first load before serving, so an early game does not pin an empty catalogue.

**Retiring on-demand card data is mostly deletion.** `cards.Client` goes, along with its TCGdex fetch, refresh ages, disk, Spaces and tiered stores, single-flight, failure memory, clear generations, `Warm` and the hosted catalog warm, the game's background `warm` and `PrefetchCards`, and the card-data half of the admin clear. `internal/cards` keeps the runtime card types, `cards.DB`, `LogCodeID` and `SameName` (used by the importer and `cardset`), and the image cache with its CDN copies, versions, breaker, image scope and image clear, unchanged. `/admin` offers "Clear images" (everything or one set, named through the current snapshot) and "Check for published card sets now", and shows the loaded catalogue's state. Legacy cache files on disk are never read. A one-off `tcgreplay sets cleanup-legacy` command lists, and with `-delete` removes, the bucket's `tcgdex/` objects. The spec's ordering is enforced by task dependencies: every built set is published (a task for the maintainer, who holds the bucket keys) before the milestone that switches the playmat over and deletes the old path.

**Images and deploys.** `Print.Card` carries the print's whole ordered image list onto `cards.Card` (`Images`), not one chosen image. Each entry is marked as either a TCGdex base path, to which the board appends `/low.webp` or `/high.webp` and which the image cache copies to the CDN as today, or a complete file address used as is for both sizes, such as a malie.io jpg. `/api/card` sends the list in the same order, and the board tries each source in turn, moving to the next when an image is missing or fails to load. It shows the placeholder only when every source fails. A full-file address passes through the image cache untouched, which only ever copies `assets.tcgdex.net` paths. TCGdex images keep their CDN copies, `?v=` versions and TCGdex fallback, so how images are fetched and stored is unchanged (spec Non-Goal). Which art shows is data the maintainer controls through the list's order, via imports and corrections, not code. Showing malie art does not wait on the malie.io maintainers' answer to the card-set-import plan's person task. Because an image is looked up by its source address, an image whose source changed in a publish is shown from the new source in the next game. Finally, App Platform redeploys on every push to `main` and drops every in-memory game, which would make each data-only merge disturb open games. Deploys therefore move to a GitHub Action that skips pushes touching only `cardsets/**` (`deploy_on_push: false`, `doctl apps create-deployment`). Rejected options (adapting `cards.Client`, one global catalogue swapped in place, fixed-name set files with per-set purges, signed reads, an embedded fallback, one chosen image per card, copying malie art, keeping deploy-on-push) are in `research.md#alternatives-considered-and-rejected`.

## Component Breakdown

- **Published layout and manifest (new).** Owns the shape of what sits in the bucket: content-addressed, gzipped set objects that are never changed once written, and one small manifest naming each set's current object, its hash and its print count. It is the contract between the publisher and the playmat's loader, and it carries a format number so a later breaking change is detectable. The publisher writes it and the loader reads it. Neither depends on the other's code.
- **Publisher (new, build-time side of `cardset`).** Reads the committed built sets through the card-set-import plan's format, refuses to publish while the set check finds anything open, compares each set's hash with the published manifest, uploads only changed sets, rewrites the manifest when anything changed, and purges the manifest from the CDN. It writes to the bucket through the existing Spaces client, or to a local directory for development and tests. It never deletes or overwrites a set object, and it refuses to drop a published set that is missing from the repository.
- **`sets publish` and `sets cleanup-legacy` subcommands (changed: the `tcgreplay` command line).** `publish` runs the publisher with the bucket settings the hosted playmat already uses, plus the CDN token. `cleanup-legacy` lists the on-demand card cache left in the bucket and, when asked, deletes it. Both sit beside the card-set-import plan's `sets import` and `sets check`.
- **Publish and deploy automation (new: GitHub Actions).** One workflow runs the set check and the publisher on each push to `main` that changes card-set data, and can also be started by hand. A second workflow deploys the hosted playmat on pushes to `main` that change anything else, replacing App Platform's deploy-on-push, so a data-only merge never restarts the playmat.
- **Catalogue snapshot (new, `catalog` package).** An immutable, fully converted catalogue that answers the existing card-lookup interface (a card by id, a cached card, a set code and number, a name with moves) and resolves set names for the admin page. It is built from loaded sets by the same pure index rules the playmat uses today. Converted sets are shared between snapshots, so pinning an older snapshot costs only its changed sets.
- **Catalogue loader (new, `catalog` package).** Holds the current snapshot, loads it at start from the local copy and then online, re-checks the manifest on an interval and on demand, fetches and verifies only changed sets, swaps snapshots atomically, keeps the local copy content-addressed with the manifest written last, logs each pickup and each outage once, and reports its state (version, sets, last check, last error) for the admin page.
- **Game manager and game (changed).** The manager takes the loader rather than a fixed card database. It captures one snapshot at the start of each new game, import and opponent-deck re-import, and uses it both to resolve the decklists and log and as the game's card database. It exposes the open game's snapshot for card text and the board. The game's background warming and card prefetching go away, because every card in a snapshot is already in memory.
- **Card text tool and board card endpoint (changed: MCP server and board server).** Both read definitions through the open game's snapshot, so an open game's card text and art never change under it. The board endpoint sends the card's whole ordered image list, each entry marked as a base path (with its CDN copy, version and TCGdex fallback, as today) or a complete file address. Borrowed art for prints without images, which takes the other print's whole list, reads through the same snapshot.
- **Board image loading (changed: web front end).** Tries the card's image sources in order. A base path gets the size file appended, as today (the CDN copy, then the TCGdex original), and a complete file address is used as is. When an image is missing or fails to load it moves to the next source, and it shows the placeholder only when every source fails.
- **Card runtime package (changed, `cards`).** Shrinks to the runtime card types, the lookup interface, the TCG Live log-code mapping, name matching and the image cache. A card gains its ordered image list, each entry marked as a TCGdex base path or a complete file address. The TCGdex client, its stores, tiers, refresh and failure tracking, catalogue warming and card-data clearing are removed. The image cache, its CDN copies and versions, and its clear are unchanged.
- **Cache admin service and page (changed).** Drops card-data clearing. It keeps clearing images (everything, or one set named through the current snapshot), adds "Check for published card sets now", and shows the loaded catalogue's state and the last check.
- **Playmat start-up and settings (changed: run modes).** Every mode builds the loader with the published-sets address, check interval, cache directory and offline switch, waits a bounded time for the first load, and starts its background checks. Hosted mode keeps the Spaces settings for image copies only. The old shared card-cache wiring and catalogue warm are removed.
- **Documentation and project knowledge (changed).** The card-set guide, README, design-and-build notes, hosted settings comments and desktop page describe publishing, how quickly playmats pick a publish up, the offline copy, the admin page and the clean-up. Knowledge entries describing the retired card cache are rewritten to describe published card sets.

## Data Structures & Interfaces

**Published layout** (in the existing Spaces bucket; every object public-read). Paths in the manifest are relative to the manifest's own directory, so the same layout works from the CDN, the bucket's origin address or a local directory.

```
cardsets/manifest.json                         Cache-Control: public, max-age=60   (purged after each publish)
cardsets/sets/<set-id>/<hash>.json.gz          Cache-Control: public, max-age=31536000, immutable
                                               <hash> = first 16 hex of sha256(built set file bytes)
```

**Manifest** (`cardsets/manifest.json`). It is the contract between the publisher and the loader. It is deterministic (keys sorted, no timestamps), so a publish with nothing changed produces identical bytes and uploads nothing.

```json
{
  "format": 1,
  "sets": {
    "me06": {"path": "sets/me06/3f9a…c2.json.gz", "sha256": "<64 hex>", "prints": 141, "format": 1},
    "sv01": {"path": "sets/sv01/…json.gz",        "sha256": "…",        "prints": 258, "format": 1}
  }
}
```

`format` at the top is the manifest's own format. The per-set `format` is the built set's `format` (from the card-set-import plan's `Set.Format`), so a loader can skip a set written in a format it does not know rather than misread it.

**Publisher** (`cardset` package, build-time side):

```go
type PublishTarget interface {            // the bucket, or a directory for development and tests
    Get(ctx, key string) ([]byte, error)  // ErrNotFound when absent
    Put(ctx, key string, body []byte, contentType, cacheControl string) error
    List(ctx, prefix string) ([]string, error)
}
type PublishOptions struct {
    Dir    string            // the cardsets/ directory
    Target PublishTarget
    Purge  func(ctx, paths []string) error // CDN purge of the manifest; nil = none (warned)
    DryRun bool
}
type PublishResult struct{ Uploaded, Unchanged []string; ManifestChanged bool }
func Publish(ctx, opts PublishOptions) (PublishResult, error)
```

**Catalogue** (new `catalog` package, runtime side):

```go
// Snapshot is an immutable, fully converted catalogue. It implements cards.DB.
type Snapshot struct{ /* sets, cards by id, set-code index, by-name index */ }
func (s *Snapshot) Card(ctx, id string) (*cards.Card, error)
func (s *Snapshot) Cached(id string) *cards.Card
func (s *Snapshot) Resolve(ctx, setCode, number string) (string, error)
func (s *Snapshot) ByName(ctx, name string, moves ...string) (string, error)
func (s *Snapshot) SetID(name string) (id, setName string, err error) // admin page: id, decklist code, any case
func (s *Snapshot) Version() string                                   // sha256 of the manifest it came from; "" = empty

// Loader holds the current snapshot and keeps it up to date.
type Loader struct {
    Base     string        // published-sets address (https://…/cardsets/ or a directory)
    CacheDir string        // local copy lives in <CacheDir>/cardsets/
    Offline  bool          // never go online; local copy only
    Interval time.Duration // default 5m
    HTTP     *http.Client
}
func (l *Loader) Current() *Snapshot             // never nil; empty before the first load
func (l *Loader) Snapshot() cards.DB             // Current(), as the game manager's CardSource
func (l *Loader) Start(ctx, wait time.Duration)   // first load (local copy, then online) bounded by wait
func (l *Loader) Run(ctx)                         // interval checks until ctx ends
func (l *Loader) Check(ctx) (CheckResult, error)  // one check now (admin trigger)
func (l *Loader) Status() Status                  // version, set count, last check, last change, last error

type CheckResult struct{ Version string; Changed, Removed []string }
```

**Snapshot source for games** (changed, `game` package). The manager's card database becomes a source of snapshots. An in-memory test pool keeps working unchanged because it is its own snapshot.

```go
type CardSource interface{ Snapshot() cards.DB } // *catalog.Loader satisfies it (returns Current())
func NewManager(dir string, src CardSource) *Manager
func (m *Manager) CardDB() cards.DB  // the open game's snapshot, else src.Snapshot()
func (g *Game) DB() cards.DB         // the snapshot the game was loaded with
```

`cardstest.FakeDB` gains `Snapshot() cards.DB { return f }`, so tests pass it where a `CardSource` is wanted.

**Card image list** (changed, `cards`). A card carries every image source of its print, in the order the board tries them.

```go
type ImageRef struct {
    URL  string `json:"url"`
    File bool   `json:"file,omitempty"` // true: a complete file used for every size (e.g. a malie.io jpg); false: a TCGdex base path
}
// cards.Card gains:
//   Images []ImageRef // the print's whole ordered list; empty = no art
// and keeps Image, set to Images[0].URL, for readers that only ask whether a card has art.
func (c *Card) ImageList() []ImageRef // Images, or one entry made from Image when only Image is set (test pools)
```

`Print.Card` fills `Images` from the print's `[{source, url}]` list in order. A `tcgdex` URL under `https://assets.tcgdex.net/` becomes a base path. An address whose last path segment is an image file (`.jpg`, `.jpeg`, `.png`, `.webp`) becomes a file entry. Anything else is left out.

**Board card endpoint** (changed, `/api/card`). The single image fields become an ordered list. Every other field is unchanged.

```json
{
  "images": [
    { "image": "https://<cdn>/images/en/me/me06/1", "image_fallback": "https://assets.tcgdex.net/en/me/me06/1", "image_version": "a1b2" },
    { "image": "https://cdn.malie.io/…/me6_en_001_std.jpg", "image_file": true }
  ]
}
```

`images` lists every source in the order the board tries them, and is empty for a card with no art (after borrowed art). For an entry without `image_file`, `image` is a base path (the CDN copy where one exists) to which the board appends `/low.webp` or `/high.webp`, with `image_version` and `image_fallback` as today. For an entry with `image_file: true`, `image` is a complete file address used as is for every size, with no version or fallback. The list replaces the top-level `image`, `image_fallback` and `image_version`, whose only reader is the board's image code.

**Cache admin** (changed). The `cacheadmin.Cards` interface (set lookup plus card-data clear) is replaced:

```go
type Catalog interface {
    SetID(name string) (id, setName string, err error)
    Check(ctx) (catalog.CheckResult, error)
    Status() catalog.Status
}
// routes: GET /admin, POST /admin/clear (images: all=1 | set=<code or id>), POST /admin/check
```

**Command line** (`tcgreplay sets …`, beside `import` and `check`):

- `sets publish [-dir cardsets] [-to <directory>] [-dry-run]`. It publishes to the bucket named by `TCGREPLAY_SPACES_ENDPOINT/_BUCKET/_KEY/_SECRET`, purging with `TCGREPLAY_SPACES_CDN_TOKEN` when set, or to a directory with `-to`. It exits 1 when the set check has open items or a published set is missing from the repository.
- `sets cleanup-legacy [-delete]`. It lists the bucket's `tcgdex/` objects, and deletes them with `-delete`.
- Playmat flags (all modes): `-cardsets-url` (env `TCGREPLAY_CARDSETS_URL`; default: the project's published-sets CDN address), `-cardsets-interval` (env `TCGREPLAY_CARDSETS_INTERVAL`, default `5m`). `-cache` now holds the local copy of published sets. `-offline` uses only that copy.

**Local copy** (`<cache dir>/cardsets/`). It mirrors the published layout: `manifest.json` (written last, by rename) and `sets/<id>/<hash>.json.gz`. Objects the local manifest no longer names are removed after a successful swap. Legacy `sets.json`, `sets/`, `cards/` and `set-index*.json` in the cache dir are never read.

**Removed interfaces.** `cards.Client`, `cards.Store`/`Entry`, `NewDiskStore`, `NewSpacesStore`, `NewTieredStore`, `Client.Warm`, `Client.Clear`, `Client.SetID`, the refresh constants and `game.(*Game).PrefetchCards` are removed. `cards.Scope` keeps only its image matching, for the image clear.

## Implementation Detail

**A cache becomes a catalogue.** Today the playmat's card data is a read-through cache: every lookup can fetch, refresh, fail, be remembered as failed, or be marked cleared, and much of the `cards` package exists to make that safe under concurrency (single-flight, clear generations, background refreshes, tier breakers). This plan replaces it with a different pattern: a complete catalogue loaded as a whole, held as an immutable value, and replaced as a whole. A lookup is a map read that cannot block, fail transiently or change under its caller. A developer reading card lookups afterwards finds no network code on that path. Everything that can go wrong (unreachable storage, a corrupt download, an unknown format) happens in the loader, before a snapshot is built, and leaves the current snapshot in place.

**A new module boundary: `catalog` between `cardset` and `cards`.** The dependency direction is `catalog` → `cardset` (built format, `Print.Card`) → `cards` (runtime types). `cards` becomes a small leaf package of runtime types, the lookup interface, log-code and name helpers, and the image cache. `cardset` keeps everything that runs on the maintainer's machine (import, check, now publish). `catalog` is the only package that knows how the playmat obtains card data. The index rules (set codes, collisions, name preference, aliases, Basic Energy art) move from `cards` into `catalog` with their existing tests, so their behaviour is carried over rather than rewritten.

**Snapshot pinning instead of shared mutable state.** Each game is handed one snapshot when it is created, imported or reopened, and keeps it for its life. This follows the existing shape: a game already receives its card database once, at load, and never re-reads it. The new rule is that the manager captures the snapshot once per operation and uses the same one for resolving decklists and logs and for the game itself. Callers outside the game that show card data (card text, the board's card endpoint) ask the manager for the open game's database rather than reaching for a global. Reviewers should watch for any path that still reads "the latest" catalogue while a game is open. Removing the global card database from the manager's public surface makes such a path fail to compile.

**Structural sharing keeps pinning cheap.** A snapshot is a map of per-set converted cards plus indexes. A new snapshot reuses the per-set values of every set whose hash did not change, so at most a handful of versions coexist cheaply even with several games pinned to older ones. Older snapshots are freed by the garbage collector when the last game holding them ends. Nothing needs reference counting.

**Content addressing end to end.** The same hash names a set in three places: the committed built file (hashed), the bucket object (named by it) and the local copy (named by it). Publishing is "upload what is not there yet, then move the manifest". Loading is "download what is not here yet, verify it, then move the local manifest". Both are idempotent and safe to interrupt, because the manifest is always written last and only ever points at complete objects. Rollback is publishing an older commit's sets: their objects still exist, so only the manifest changes.

**One loop pattern for checks.** The loader's interval check, its admin-triggered check and its first load share one code path. It follows the codebase's existing background-loop shape (a ticker loop on the server's lifetime context), its outage handling (log once when storage becomes unreachable, once when it recovers) and its logging style (`log.Printf` with a short lower-case subsystem prefix, an injectable logger in tests). Overlapping checks are serialised, so an admin press during an interval check waits for it rather than racing it.

**Images stay a separate concern.** Image copying, versioning and clearing are untouched. The only image changes are at the edges: a card carries its print's whole ordered list of sources (the order decided by data), each marked as a base path or a complete file address, and the board falls back through that list. A developer changing image behaviour later still works in the image cache alone.

**Deletion as a first-class task.** Roughly two thirds of the `cards` package and its tests go, along with the fake-TCGdex servers in other packages' tests and the hosted catalogue warm. Tests that exercised the old cache are deleted, not ported. The behaviours worth keeping are re-expressed against the new catalogue: index rules, open games keep their cards, admin clear of images, restart with a local copy. The retirement milestone comes last, so the playmat already reads published sets before any old path is removed.

**Automation lives in the repository.** The project gains its first GitHub Actions workflows. The publish workflow runs the same `tcgreplay sets` commands a maintainer runs by hand, so there is one publishing code path, and the deploy workflow reproduces App Platform's deploy-on-push minus data-only pushes. Secrets (bucket keys, CDN token, DigitalOcean deploy token) live in the repository's GitHub settings, never in files.

## Dependencies

- **Design documents this plan was built on: none.** The spec carries no design references.
- **Spec `20261108110847-d27a9f77-card-set-import` and its plan (must land first).** Provides the built format (`cardset.Set`, `Print`, `Read`, deterministic files, `format: 1`), `Print.Card(lang)` and `Set.Card(id, lang)` conversion into `cards.Card`, `cardset.Check` and `tcgreplay sets check`, the `sets` subcommand dispatch, `cardsets/` outside `internal/` and excluded by `.dockerignore`, and the built, verified catalogue of every English set. This plan extends one thing in it: `Print.Card` and `cards.Card` carry the print's whole ordered image list (TCGdex base paths and complete file addresses such as malie jpgs), and the board falls back through it, where that plan puts only the first TCGdex image into `cards.Card.Image` and leaves the wider policy to this spec. Every task here depends on that plan's format task, the publish tasks on its check command, and the full publish on its last catalogue milestone.
- **Spec `20261108110847-055548e3-paired-stadiums` (depends on this plan).** Adds a pair flag to `Print` and to `cards.Card`. The loader converts prints only through `Print.Card`, so a new field flows to the playmat with no loader change. Nothing here blocks it beyond landing first.
- **`cards` package (internal, changed heavily).** Keeps `Card` and its parts (`Card` gains the ordered image list), `DB`, `LogCodeID`, `SameName`, `ImageCache`, image `Scope` and `TierReport`, and `breaker`. Loses the TCGdex client and all its stores, tiers, refresh, warm and clear code.
- **`game` package (internal, changed).** The manager takes a snapshot source, and each game pins one snapshot. Warming and prefetching are removed.
- **`mcpserver` and `webui` packages (internal, changed).** `card_text`, `/api/card` and `borrowedArt` read through the open game's snapshot. `/api/card` sends the ordered `images` list in place of its single image fields.
- **`cacheadmin` package (internal, changed).** Card-data clear is replaced by a catalogue check and status. Image clear is kept.
- **`spaces` package (internal, unchanged).** Supplies `Put` with content type, cache control and public-read, `Get`, `List`, `Delete`, `FromEnv`, `PublicBase` and `CDN.Purge`, which the publisher and the clean-up use as they are. `spacestest` fakes the bucket (including anonymous reads of public objects) and the CDN for tests.
- **Board front end (`web/`, changed).** Image loading tries each source in the card's list in order, moving to the next on a missing or failed image. The Playwright image tests are extended.
- **Go standard library.** `compress/gzip`, `crypto/sha256`, `net/http`, `sync/atomic`, `encoding/json`. No new third-party Go module.
- **DigitalOcean Spaces bucket and CDN (external service, existing).** Holds the published layout beside the existing `images/` copies. It needs a key that can write public-read objects (the hosted key already can) and the CDN enabled (already done for images). No new hosting service, per the spec's Constraint.
- **DigitalOcean CDN purge API (external service, existing).** Purges the manifest after each publish. It needs a token with `cdn:read` and `cdn:delete` (knowledge `gotchas/cdn-purge-needs-cdn-delete.md`).
- **DigitalOcean App Platform and `doctl` (external, changed use).** Deploys move from `deploy_on_push` to `doctl apps create-deployment` in a GitHub Action. It needs a DigitalOcean API token able to create deployments for the app.
- **GitHub Actions (external, new to the project).** Runs the publish and deploy workflows. It needs repository secrets: the four `TCGREPLAY_SPACES_*` values, `TCGREPLAY_SPACES_CDN_TOKEN`, a DigitalOcean deploy token and the app id. Adding them is a task for a person.
- **The bucket's public CDN address (information the maintainer holds).** It becomes the default published-sets address compiled into the program. The bucket name is a secret setting today, so the maintainer supplies it. This is a task for a person, and it must happen before the playmat switches over.
- **Knowledge entries relied on:** `gotchas/tcgdex-cache-never-expires.md` (the cache, tiers, admin clear and image cache being retired or kept), `gotchas/board-card-info-per-page-load.md` (per-page-load card data, image versions), `gotchas/cdn-purge-needs-cdn-delete.md`, `gotchas/spaces-endpoint-is-region-not-bucket.md` (the CDN address derivation), `gotchas/new-set-tcgdex-data-incomplete.md`, `learnings/card-image-sources.md` (malie image addresses and terms), `gotchas/tcg-live-log-card-codes.md` (set-code collisions, aliases, `LogCodeID`), `learnings/card-text-always-english.md` (the runtime uses English text), `architecture/importer-determinism.md` (lookups fill only `Def`, so pinning cannot change where cards go), `gotchas/game-listeners-under-lock.md`. Entries describing the retired cache are rewritten by a task in this plan.

## Testing Approach

Tests are standard Go tests beside the code they cover, plus the board's existing Playwright suite for the image change, all run by the project's usual `make test`. Fakes follow existing patterns: the in-memory card pool for game, MCP and board tests; the fake Spaces bucket and fake CDN for publishing and clean-up; `httptest` servers (or a local published directory) for the loader; and the re-exec-the-test-binary helper for command-line and hosted-mode tests. No test calls the real bucket, CDN, TCGdex or malie.

**Most coverage: the loader and the snapshot**, because every card the playmat shows now passes through them.

- **Index parity.** The snapshot answers set code plus number, name plus moves, and id lookups with the same results the current catalogue gives. The existing set-code collision, alias, Pocket and name-preference cases are carried over, and Basic Energy without art still borrows Crown Zenith art (its whole image list).
- **Only changed sets fetched.** After one set's hash changes in the manifest, a check downloads that set alone, and the new snapshot shares every other set with the old one.
- **Atomic swap and failure safety.** A corrupt object (hash mismatch), an unknown format, an unreachable server or a half-written local copy leaves the current snapshot in place and is logged once. Overlapping checks never interleave.
- **Local copy.** After a load, a loader started offline (or against a dead server) from the same cache directory serves the same definitions. Legacy cache files in that directory are never read. With no copy and no storage, the snapshot is empty and cards are unknown.
- **Interval and trigger.** With a short interval, a published change is picked up with no restart. A direct check picks it up at once.

**Game pinning (regression for "open games keep their cards").** A game created, imported, or created by an opponent-deck re-import before a swap keeps its old text, HP and image through card text, the board's card endpoint, warnings and attack lookups. A game started or reopened from its record after the swap sees the new text. An import never mixes two versions.

**Publishing.** Against the fake bucket: a first publish uploads every set and the manifest with the right content type, cache headers and public-read. Changing one set and publishing again uploads that set's new object and the manifest only, and the old object is still there byte for byte. Publishing unchanged sets uploads nothing. Publishing refuses on an open check item and on a published set missing from the repository. The manifest is purged from the fake CDN when a token is set, and the publish warns when none is. The output is deterministic (same sets, same manifest bytes). A publish to a local directory followed by a load from it gives a snapshot equal to converting the built files directly, which proves the two halves agree on the layout.

**Retirement.** The admin page offers image clearing and "Check now", and no longer offers card-data clearing. "Check now" runs a check and shows the outcome. The clean-up command lists `tcgdex/` objects, deletes them only with `-delete`, and touches nothing else (`images/`, `cardsets/`). A source-scan test asserts that the TCGdex API host (`api.tcgdex.net`) is named only in `internal/cardset`'s TCGdex fetcher file (`internal/cardset/tcgdex.go`, the maintainer-side fetcher behind `tcgreplay sets import`), and fails if any other non-test Go file names it. The playmat's runtime packages (`internal/catalog`, `internal/cards`, `internal/game`, the board and MCP servers, and the playmat paths of `cmd/`) therefore never name it, so no card-data request from the playmat can reach it.

**Image list.** `Print.Card` keeps the print's order, marks each entry as a base path or a complete file address, and leaves out entries it cannot classify. `/api/card` sends the list in that order, with the CDN copy, version and fallback on base-path entries only. A Basic Energy print without art borrows the other print's whole list.

**Board images (end to end).** In the board test harness, a card whose image is a complete file address loads that address unchanged for the mat and the zoom. A base-path card still loads `/low.webp` and `/high.webp`, with the CDN version and the fallback as today. Fallback order: a card whose first source is a base path that 404s (copy and TCGdex original) loads its next source, a complete file address, and a card whose first source is a file address that 404s loads its next source, a base path, with the size suffix. A card whose every source fails shows the placeholder, after trying each source in order.

**Command line and hosted mode.** `sets publish` and `sets cleanup-legacy` run through the re-exec helper. A hosted-mode playmat pointed at a fake published layout serves a full game (import a log, play, read card text) with definitions equal to the published sets and no other card-data requests. A playmat with no storage and no copy shows cards as unknown.

**Existing records (regression over real data).** A test publishes the committed `cardsets/` to a temporary directory, loads it, and imports every battle log and game record in the project's test corpus. Every card in every game resolves to a definition with English text. These logs predate Delta Reign, so this covers "records from before the switch, from sets other than Delta Reign".

**Deliberate gaps.** The GitHub Actions workflows are not unit-tested: they run the same commands the tests cover, and are checked by hand on their first runs. Real CDN caching behaviour, real App Platform deploy skipping and real bucket clean-up can only be seen on the real infrastructure, so they are manual checks below.

**Success metrics.**

- *The median time from a set change being merged to being live on the hosted playmat is under one hour.* **Manual — captured in the implementation test plan** (measured across the first real publishes: merge time against the playmat's pickup log line).
- *Zero player reports of cards showing as unknown or without text after the switch.* **Manual — captured in the implementation test plan** (post-release observation). The existing-records regression test and the committed-data guard are its automated proxy.

**Manual reviews.**

- **Manual — captured in the implementation test plan:** on the real hosted playmat, publish a changed card and confirm that a new game shows it within 10 minutes with no restart, and at once after "Check now" on `/admin`, while a game opened before the publish keeps the old text.
- **Manual — captured in the implementation test plan:** after a real publish, a fresh request through the CDN for the changed set's current version (manifest, then its object) returns the new version, and a card whose image source changed shows the new image.
- **Manual — captured in the implementation test plan:** on the hosted board, a Delta Reign card with an image source shows its image, and a TCGdex image with a hosted copy loads from the CDN address.
- **Manual — captured in the implementation test plan:** on the hosted board, a card whose first listed image source is missing (404) shows the next listed source, and a card whose every source fails shows the placeholder.
- **Manual — captured in the implementation test plan:** a desktop playmat that has loaded sets once, restarted with the network off, plays a game with full card text, and an online desktop playmat picks a publish up within 10 minutes.
- **Manual — captured in the implementation test plan:** a merge that changes only `cardsets/` publishes and does not redeploy the hosted playmat, and a code merge still deploys it.
- **Manual — captured in the implementation test plan:** after `sets cleanup-legacy -delete` on the real bucket, no `tcgdex/` objects remain and image copies are intact.
- **Manual — captured in the implementation test plan:** follow the publishing documentation to publish a set by hand, and check that it states how quickly playmats pick a publish up.

## Milestones & Tasks

### Milestone 1: Built card sets are published to online storage

**What changes**: The maintainer can publish the project's built card sets to the playmat's existing online storage with one command, and every merge to `main` that changes card data publishes automatically. Only sets that changed are uploaded, earlier versions stay retrievable, and the storage's current list of sets reaches viewers quickly after each publish. A merge that changes only card data no longer redeploys the hosted playmat, so open games are not dropped. Every built set is published once, ready for the switch. Players see no difference yet: the playmat still reads cards on demand.

**Validation point**: Publishing tests pass against the fake bucket. A real publish of the whole catalogue leaves a manifest naming every built set in the bucket. A second publish with nothing changed uploads nothing.

#### - [x] Task: Card set publisher
**Id:** f26662e6-4d47-4d0b-aba4-bc7912d4c0bd
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Adds the publishing logic beside the card-set import. It reads the committed built sets and names each one by a hash of its contents. It uploads only the sets the published manifest does not already list with that hash, as compressed files that are never changed once written, then writes a new manifest naming each set's current file and purges that manifest from the CDN. It refuses to publish while the set check finds open items, or when a set already published is missing from the repository. It can publish to the bucket or to a local directory, for development and tests. It needs the card-set-import plan's built format and set check to have landed.

*Technical detail:* [context.md#task-card-set-publisher](./context.md#task-card-set-publisher)

**Acceptance criteria**:
- [x] A first publish uploads every built set and a manifest naming each one, all publicly readable, with long-lived caching on set files and short-lived caching on the manifest.
- [x] After one set changes, a publish uploads only that set and the manifest, and the set's earlier file is still retrievable unchanged.
- [x] Publishing unchanged sets uploads nothing.
- [x] Publishing stops without uploading when the set check reports an open item, or when a published set is missing from the repository.
- [x] The manifest is purged from the CDN after a publish when a CDN token is available, and the publish warns when it is not.

#### - [x] Task: Sets publish command
**Id:** 6d338ee2-fbf2-40b2-b80d-d38f7174e368
**Repo:** tcgreplay
**Depends on:**
- f26662e6-4d47-4d0b-aba4-bc7912d4c0bd — Card set publisher
**Execution:** agent

Adds `tcgreplay sets publish`, which publishes the built sets to the bucket named by the hosted playmat's existing storage settings, purging the CDN with its existing CDN token, or to a local directory. It prints what it uploaded and what was unchanged. A dry run shows what would be uploaded without writing anything.

*Technical detail:* [context.md#task-sets-publish-command](./context.md#task-sets-publish-command)

**Acceptance criteria**:
- [x] Running the command with the storage settings publishes changed sets and reports which were uploaded and which were unchanged.
- [x] Running it with missing or partial storage settings fails with a message naming what is missing, and uploads nothing.
- [x] A dry run reports the sets that would be uploaded and writes nothing.
- [x] The playmat's own start-up and its refusal of unknown words are unchanged.

#### - [x] Task: Publish on merge workflow
**Id:** 0e1b48c4-41dc-4364-8308-7f0b66ee7fa7
**Repo:** tcgreplay
**Depends on:**
- 6d338ee2-fbf2-40b2-b80d-d38f7174e368 — Sets publish command
**Execution:** agent

Adds the project's first GitHub Actions workflow. On every push to `main` that changes card-set data, it runs the set check and then publishes, using storage credentials kept as repository secrets. It can also be started by hand. Publishing therefore always matches the reviewed repository.

*Technical detail:* [context.md#task-publish-on-merge-workflow](./context.md#task-publish-on-merge-workflow)

**Acceptance criteria**:
- [x] A push to `main` that changes card-set data starts the workflow, and other pushes do not.
- [x] The workflow can be started by hand.
- [x] The workflow checks the sets before publishing and fails without publishing if the check fails.
- [x] No credential appears in the repository: the workflow reads them from repository secrets.

#### - [x] Task: Deploy workflow that skips card data
**Id:** 101b9a9a-9891-40f8-8351-3f376043a009
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Today every push to `main` redeploys the hosted playmat and drops every open game, so a card-data-only merge would disturb players. This task moves deploys into a GitHub Actions workflow that deploys on pushes to `main` that change anything other than card-set data, and switches off the hosting platform's own deploy-on-push.

*Technical detail:* [context.md#task-deploy-workflow-that-skips-card-data](./context.md#task-deploy-workflow-that-skips-card-data)

**Acceptance criteria**:
- [x] A push to `main` that changes code or documentation starts a deploy, and a push that changes only card-set data does not.
- [x] The hosting definition no longer deploys on every push by itself.
- [x] A deploy can also be started by hand.
- [x] The deploy credential is read from a repository secret, never from a file.

#### - [ ] Task: Set up publishing secrets and the published sets address
**Id:** 1e0644d4-e712-47f3-9ec8-46bd7687f0dd
**Repo:** tcgreplay
**Depends on:**
- 0e1b48c4-41dc-4364-8308-7f0b66ee7fa7 — Publish on merge workflow
- 101b9a9a-9891-40f8-8351-3f376043a009 — Deploy workflow that skips card data
**Execution:** human — needs the bucket's access keys, a DigitalOcean API token and admin access to the GitHub repository's settings

The maintainer adds the storage keys, the CDN purge token, a deploy token and the app's id as repository secrets, so both workflows can run. They also give the bucket's public CDN address, which becomes the playmat's default address for published sets. The bucket name is a secret today, so nobody else can supply it.

*Technical detail:* [context.md#task-set-up-publishing-secrets-and-the-published-sets-address](./context.md#task-set-up-publishing-secrets-and-the-published-sets-address)

**Acceptance criteria**:
- [ ] The repository holds every secret the publish and deploy workflows read.
- [ ] The bucket's public CDN address is recorded for the playmat's default.
- [ ] A hand-started deploy succeeds.

#### - [x] Task: Publish every built set
**Id:** e0a9670e-59f3-4b74-b6ed-2963c6b81332
**Repo:** tcgreplay
**Depends on:**
- 6d338ee2-fbf2-40b2-b80d-d38f7174e368 — Sets publish command
- 1e0644d4-e712-47f3-9ec8-46bd7687f0dd — Set up publishing secrets and the published sets address
**Execution:** human — writes to the production bucket, which needs the bucket's access keys

Once the card-set-import plan has built and verified every English set, the maintainer publishes the whole catalogue, by starting the publish workflow by hand or running the command with the keys. This must happen before the playmat switches to published sets, so saved games, records and logs from before the switch keep full card definitions.

*Technical detail:* [context.md#task-publish-every-built-set](./context.md#task-publish-every-built-set)

**Acceptance criteria**:
- [x] The published manifest names every built set in the repository.
- [x] Publishing again straight after uploads nothing.

### Milestone 2: The playmat plays from published card sets

**What changes**: Every playmat, hosted and desktop, gets its card definitions from the published sets and never from the upstream card sources. It picks up a publish within minutes without a restart, or at once when an admin presses "Check now" on the admin page, which now also shows the loaded catalogue. Games already open keep the cards they started with, and new, imported or reopened games use the latest. Desktop keeps a copy of what it loaded and plays offline from it. Card images show from the sources each print names, in the order it lists them, falling back to the next source when one is missing or fails to load, including art TCGdex lacks, such as Delta Reign's. The admin page no longer offers to clear card data. It still clears images.

**Validation point**: Loader, pinning, image and admin tests pass. A hosted-mode playmat pointed at a published layout plays a full game with definitions equal to the published sets. Every corpus battle log imports with full card text from the committed catalogue.

#### - [x] Task: Catalogue snapshot
**Id:** 9065da89-fc46-4e58-8e8d-e200f180bfe5
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Adds the playmat's in-memory card catalogue, built from loaded card sets and never changed once built. It answers every lookup the playmat makes today: a card by id, a set code and collector number, a name with the moves seen, and a set name for the admin page. It follows the same rules as today for set-code clashes, aliases, skipping Pocket sets, preferring prints, and borrowed Basic Energy art. A new snapshot reuses the unchanged sets of the previous one.

*Technical detail:* [context.md#task-catalogue-snapshot](./context.md#task-catalogue-snapshot)

**Acceptance criteria**:
- [x] Looking up cards by set code and number, by name with moves, and by id gives the same results as today's catalogue for the same set data.
- [x] Sets sharing a code resolve to the main set, and Pocket sets never answer a name lookup.
- [x] A Basic Energy print without art shows the borrowed art it shows today.
- [x] A snapshot built after one set changed shares every other set with the previous snapshot, and the previous snapshot still answers with the old data.
- [x] An empty snapshot answers every lookup as not found.

#### - [x] Task: Image source list and board fallback
**Id:** c82fe60b-91bb-4ef8-a5a9-1a2210db3350
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Carries each print's whole ordered list of image sources onto the card definition and the board's card endpoint, instead of one chosen image. Each entry is marked as a TCGdex image path, to which the board adds the size suffix, or a complete file address such as a malie.io card image, which the board loads as is for every size. The board tries each source in order and moves to the next when an image is missing or fails to load, showing the text stand-in only when every source fails. Which art shows first is data the maintainer controls through the list's order. TCGdex images keep their hosted copies, versions and fallback exactly as today, and the image cache still copies only TCGdex images.

*Technical detail:* [context.md#task-image-source-list-and-board-fallback](./context.md#task-image-source-list-and-board-fallback)

**Acceptance criteria**:
- [x] A print whose only image is a malie.io image shows that image on the mat, in the zoom and in the preview.
- [x] A print with a TCGdex image shows it as today, from the hosted copy where one exists, with the TCGdex fallback.
- [x] When a print's first listed image is missing or fails to load, the board shows the next listed source, whether that is a TCGdex image or a complete file address.
- [x] A print whose every listed source fails, or which lists none, shows its text stand-in.
- [x] A listed image from a source the board cannot show is left out of the card's list.
- [x] Reordering a print's images changes which source the board tries first.
- [x] A Basic Energy print without art borrows the other print's whole image list.

#### - [x] Task: Catalogue loader with local copy
**Id:** 756dccd1-aad4-49cd-944e-07d674413ce1
**Repo:** tcgreplay
**Depends on:**
- f26662e6-4d47-4d0b-aba4-bc7912d4c0bd — Card set publisher
- 9065da89-fc46-4e58-8e8d-e200f180bfe5 — Catalogue snapshot
**Execution:** agent

Adds the part of the playmat that keeps its catalogue current. At start it loads its local copy, then the published manifest. It downloads and verifies only sets whose hash changed, and swaps in a new snapshot in one step. It checks again on a regular interval and whenever it is asked to. It keeps a copy of everything it loaded on disk, written so an interrupted update never leaves a broken copy, and uses that copy alone when offline or when storage cannot be reached. Outages are logged once, and the current snapshot stays in place.

*Technical detail:* [context.md#task-catalogue-loader-with-local-copy](./context.md#task-catalogue-loader-with-local-copy)

**Acceptance criteria**:
- [x] After a set is published with a change, the loader picks it up on its next interval check, or at once when asked, with no restart.
- [x] A check downloads only the sets whose published version changed.
- [x] A loader that loaded once, started again with no network, serves the same definitions from its local copy.
- [x] A corrupt download, an unknown format or unreachable storage leaves the current catalogue unchanged and is reported.
- [x] Old card files left in the cache directory by the on-demand approach are never used.
- [x] With no reachable storage and no local copy, the catalogue is empty.

#### - [x] Task: Games pin their catalogue snapshot
**Id:** c22f8109-e88b-419a-9fb1-71062e260ffc
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Makes each game keep the catalogue it started with. The game manager takes a source of snapshots, and captures one when a game is created, imported or re-imported with an opponent deck. It uses that snapshot for resolving the decklists and log and for the whole game. Card text and the board's card details, including borrowed art, read through the open game's snapshot. Background card warming and prefetching are removed, because every card is already in memory.

*Technical detail:* [context.md#task-games-pin-their-catalogue-snapshot](./context.md#task-games-pin-their-catalogue-snapshot)

**Acceptance criteria**:
- [x] A game started before a newer catalogue arrives keeps showing its original card text, HP and image in card text, on the board and in warnings until it ends.
- [x] A game started, imported or reopened from its record after the newer catalogue arrives shows the new text.
- [x] An import resolves every card against one catalogue, even if a newer one arrives during it.
- [x] With no game open, card text and the board's card details use the current catalogue.

#### - [x] Task: Admin page checks for published sets
**Id:** f82814b9-1498-4871-abff-ebd2e0c87dd8
**Repo:** tcgreplay
**Depends on:**
- 756dccd1-aad4-49cd-944e-07d674413ce1 — Catalogue loader with local copy
**Execution:** agent

Changes the hosted admin page. A "Check for published card sets now" button runs a catalogue check at once and shows what changed. The page shows the loaded catalogue's version, its number of sets and the last check. Clearing card data is no longer offered. Clearing images, for everything or one set, stays, with set names resolved through the current catalogue.

*Technical detail:* [context.md#task-admin-page-checks-for-published-sets](./context.md#task-admin-page-checks-for-published-sets)

**Acceptance criteria**:
- [x] An admin can ask for a check from the admin page, and a set published since the last check is picked up at once.
- [x] The page shows the loaded catalogue's version, set count, last check time and any last error.
- [x] The page no longer offers to clear card definitions, and still offers to clear images for everything or one set.
- [x] Non-admins still cannot reach the page.

#### - [x] Task: Playmat reads published sets in every mode
**Id:** 663730e9-51e0-41d3-9783-95cf5525cfbf
**Repo:** tcgreplay
**Depends on:**
- e0a9670e-59f3-4b74-b6ed-2963c6b81332 — Publish every built set
- c82fe60b-91bb-4ef8-a5a9-1a2210db3350 — Image source list and board fallback
- 756dccd1-aad4-49cd-944e-07d674413ce1 — Catalogue loader with local copy
- c22f8109-e88b-419a-9fb1-71062e260ffc — Games pin their catalogue snapshot
- f82814b9-1498-4871-abff-ebd2e0c87dd8 — Admin page checks for published sets
**Execution:** agent

Switches every run mode (hosted, desktop daemon, in-process, dev) to the catalogue loader. The default address is the published sets on the project's CDN, with settings to point elsewhere, change the check interval or stay offline. Each mode waits briefly at start for the first load, then keeps checking in the background. Hosted mode keeps its storage settings for image copies only, and stops building the old shared card cache and warming it.

*Technical detail:* [context.md#task-playmat-reads-published-sets-in-every-mode](./context.md#task-playmat-reads-published-sets-in-every-mode)

**Acceptance criteria**:
- [x] A playmat in any mode plays a full game (import a log, play turns, read card text) with definitions equal to the published sets, making no other card-data requests.
- [x] A playmat started with no access to published storage and no local copy shows cards as unknown.
- [x] A desktop playmat that loaded sets once, restarted offline, plays with full card text.
- [x] Every battle log and game record in the project's test corpus imports with full card text for every card from the committed catalogue.
- [x] Hosted image copies and the image fallback work as before.

### Milestone 3: The on-demand card cache is gone

**What changes**: An internal clean-up with a visible end. The code that fetched, refreshed, stored and cleared card data on demand is deleted, so it can never serve half-finished upstream data again. The maintainer gets a one-off command to remove the old card data left in online storage. The documentation and the project's knowledge describe publishing, how quickly playmats pick up a publish, the offline copy and the admin page. It is its own milestone because removing the old path only once the new one is live keeps every step safe to ship.

**Validation point**: The full test suite passes with the old card client removed. No program code names the upstream card API except the maintainer's card-set import fetcher. After the clean-up command runs against the real bucket, no old card data remains. The documentation describes publishing and pickup times.

#### - [x] Task: Remove the on-demand card client
**Id:** 1811be10-d6c8-4a22-91ca-794fb3282713
**Repo:** tcgreplay
**Depends on:**
- 663730e9-51e0-41d3-9783-95cf5525cfbf — Playmat reads published sets in every mode
**Execution:** agent

Deletes the TCGdex card client and everything that only served it: fetching, refresh ages, the disk, shared and tiered stores, request sharing, failure memory, catalogue warming and card-data clearing, with their tests and the fake TCGdex servers other packages' tests used. The card package keeps the card types, the lookup interface, log-code and name helpers, and the image cache. A test guards that no program code names the upstream card API again, except the maintainer-side TCGdex fetcher the card-set import uses.

*Technical detail:* [context.md#task-remove-the-on-demand-card-client](./context.md#task-remove-the-on-demand-card-client)

**Acceptance criteria**:
- [x] The program no longer contains any code that fetches, refreshes, stores or clears card definitions from upstream sources.
- [x] A test fails if any program code other than the card-set import's TCGdex fetcher names the upstream card API, including any of the playmat's runtime packages.
- [x] Image copying, versions, the image fallback and image clearing behave as before.
- [x] The full test suite passes.

#### - [x] Task: Legacy card data clean-up command
**Id:** 5c84b8f7-2882-4ef0-b8e2-008092faf9ec
**Repo:** tcgreplay
**Depends on:**
- 6d338ee2-fbf2-40b2-b80d-d38f7174e368 — Sets publish command
**Execution:** agent

Adds `tcgreplay sets cleanup-legacy`, a one-off maintainer command that lists the card data the on-demand approach left in online storage and deletes it only when explicitly asked. It never touches image copies or published sets.

*Technical detail:* [context.md#task-legacy-card-data-clean-up-command](./context.md#task-legacy-card-data-clean-up-command)

**Acceptance criteria**:
- [x] Without the delete switch, the command lists the old card data and its count and deletes nothing.
- [x] With the delete switch, it removes all old card data and reports any it could not remove.
- [x] Image copies and published sets are never listed or removed.

#### - [x] Task: Publishing documentation
**Id:** dac434a8-5cbd-46d6-a9cb-58b7a7d3cd1a
**Repo:** tcgreplay
**Depends on:**
- 0e1b48c4-41dc-4364-8308-7f0b66ee7fa7 — Publish on merge workflow
- 101b9a9a-9891-40f8-8351-3f376043a009 — Deploy workflow that skips card data
- 1811be10-d6c8-4a22-91ca-794fb3282713 — Remove the on-demand card client
- 5c84b8f7-2882-4ef0-b8e2-008092faf9ec — Legacy card data clean-up command
**Execution:** agent

Documents publishing for maintainers and operators: publishing a set by hand and on merge, what the published layout is, how quickly hosted and desktop playmats pick up a publish (interval and admin check), the offline copy, the admin page's check and image clearing, the new settings and secrets, the deploy workflow, and the clean-up. Descriptions of the old card cache are removed or rewritten.

*Technical detail:* [context.md#task-publishing-documentation](./context.md#task-publishing-documentation)

**Acceptance criteria**:
- [x] The documentation describes publishing a set by hand and on merge.
- [x] The documentation states how quickly hosted and desktop playmats pick up a publish, and how an admin forces a check.
- [x] The documentation describes the offline copy, and that a first desktop run needs network access.
- [x] No documentation still describes fetching or clearing card definitions on demand.

#### - [x] Task: Rewrite card cache knowledge entries
**Id:** 2eb79741-e736-4cd5-bbd0-257f81caf59c
**Repo:** tcgreplay
**Depends on:**
- 1811be10-d6c8-4a22-91ca-794fb3282713 — Remove the on-demand card client
**Execution:** agent

Brings the project's knowledge base in line with the new card data path, through the knowledge skill. The entry on the TCGdex cache is replaced by one on published card sets: layout, manifest, loader, pinning, local copy, admin check and image copies. Entries that mention the old cache, its refresh rules or its clear are updated.

*Technical detail:* [context.md#task-rewrite-card-cache-knowledge-entries](./context.md#task-rewrite-card-cache-knowledge-entries)

**Acceptance criteria**:
- [x] The knowledge base describes how card sets are published and loaded, and how open games keep their cards.
- [x] No knowledge entry still describes fetching, refreshing or clearing card definitions on demand as current behaviour.

#### - [ ] Task: Remove legacy card data from the bucket
**Id:** 0cd658d7-82c2-448b-8897-4e5730f65919
**Repo:** tcgreplay
**Depends on:**
- 1811be10-d6c8-4a22-91ca-794fb3282713 — Remove the on-demand card client
- 5c84b8f7-2882-4ef0-b8e2-008092faf9ec — Legacy card data clean-up command
**Execution:** human — deletes objects in the production bucket, which needs the bucket's access keys, after the switch is deployed

Once the playmat reading published sets is deployed and working, the maintainer runs the clean-up command against the real bucket: first to list, then to delete the old card data.

*Technical detail:* [context.md#task-remove-legacy-card-data-from-the-bucket](./context.md#task-remove-legacy-card-data-from-the-bucket)

**Acceptance criteria**:
- [ ] The bucket holds no card data from the on-demand approach.
- [ ] Image copies and published sets are still in the bucket.

## Open Questions

- **Does DigitalOcean's CDN honour an object's own `Cache-Control: max-age=60` over the endpoint's edge TTL (a week for images)?** This can only be seen on the real CDN. The publisher purges the manifest after every publish whenever a CDN token is set, and the workflow always has one, so the design does not rely on the answer. If a real publish shows the CDN still serving an old manifest after the purge has succeeded, STOP and ask the user. The fallback is to have the playmat read the manifest from the bucket's origin address instead of the CDN.
- **Can App Platform skip a deploy by path natively?** The deploy-workflow task checks the current app-spec reference while implementing. If a native path filter exists, use it instead of the workflow and record the substitution. If neither works (for example, the deploy token cannot create deployments), STOP and ask the user.
- **How much memory does a full snapshot take on the 0.5 GB hosted instance?** This depends on the real catalogue's size, which only exists once the card-set-import plan has built every set. Measure it while implementing the loader against the committed `cardsets/`. If one snapshot exceeds about 150 MB, STOP and ask the user before going on (options: a larger instance, or keeping attack and ability text compact).
- **Does the card-set-import plan's `Print.Card` and `Set` API land as that plan states?** If names or shapes differ, adapt to what landed. If a field this plan needs is missing (set header abbreviation, release date or serie, or the image list), STOP and ask the user rather than extending the format silently.

## Out of Scope

- **Changing how images are fetched or stored** (spec Non-Goal). The hosted image cache still copies only TCGdex images to the bucket. malie.io images are shown from malie's own address and are not copied, versioned or purged. Copying them would be a later change to the image cache.
- **Pushing card changes to boards already open.** An open game keeps its cards, and a board shows new data on its next game or page load, as today (knowledge `gotchas/board-card-info-per-page-load.md`).
- **Building and verifying card sets, the built format, corrections and the import report.** These belong to the card-set-import spec (`20261108110847-d27a9f77-card-set-import`). This plan only publishes what it builds, and extends `Print.Card` to carry the print's whole ordered image list.
- **Pair play for Legendary Stadiums and the pair flag in card data.** These belong to the paired-stadiums spec (`20261108110847-055548e3-paired-stadiums`), which builds on this plan's loader.
- **Unpublishing or removing a set on purpose.** Publishing refuses to drop a published set that is missing from the repository. Deliberately retiring a set is not designed here.
- **Languages other than English at run time.** The loader converts the English text only. Other languages in built sets are published untouched but never shown.
- **Release binaries or packaging for desktop.** Desktop users still build from source. Only the run-time behaviour changes: it reads published sets and keeps a local copy.
- **Moving hosted games out of memory.** A code deploy still drops every in-memory game. This plan only stops card-data merges from deploying.
- **Deleting legacy card-cache files from players' disks.** The playmat ignores them. Only the bucket's old card data is removed, by the one-off clean-up.
- **Metrics dashboards or alerting on publishes.** Pickup is visible in the playmat's log and on the admin page. The success metrics are measured by hand.

## Changelog

### 2026-11-08 — Task: Card set publisher

**What was done**: Added the publishing half of `internal/cardset`: the shared manifest contract (`manifest.go`: `Manifest`, `ManifestSet`, `SetKey`, deterministic `EncodeManifest`, `DecodeManifest` refusing unknown formats and bad entries), `Publish` (`publish.go`: check first, hash each committed built file, refuse on open items or published sets missing locally, upload only changed sets as deterministic gzip under `sets/<id>/<16 hex>.json.gz`, rewrite the manifest only when its bytes change, then purge it or warn), and two targets (`target.go`: `SpacesTarget` public-read over `spaces.Client`, `DirTarget` for a local directory).

**Deviations**: `PublishResult` also carries the published `Manifest`, so the command can print print counts. `PublishOptions.Prefix` is empty for a directory and `BucketPrefix` ("cardsets/") for the bucket, so `-to <dir>` and a loader base directory both hold `manifest.json` at their root. Publishing with no built sets at all is refused. The existing `cardset.ErrNotFound` is reused for a target's missing object (its doc comment was widened in `fetch.go`).

**Files changed**:
- `internal/cardset/manifest.go`
- `internal/cardset/publish.go`
- `internal/cardset/target.go`
- `internal/cardset/publish_test.go`
- `internal/cardset/fetch.go`

**Discoveries**: `spacestest.Server.Reset` forgets request counts but keeps objects, so "no PUT" can be asserted per key after a reset. The publish warning about a missing CDN token is only logged when the manifest is actually rewritten (an unchanged publish has nothing to purge).

### 2026-11-08 — Task: Sets publish command

**What was done**: Added `tcgreplay sets publish [-dir cardsets] [-to <directory>] [-dry-run]`. Without `-to` it publishes to the bucket the `TCGREPLAY_SPACES_*` settings name, under `cardsets/`, purging the manifest from the CDN when `TCGREPLAY_SPACES_CDN_TOKEN` is set. It prints `uploaded <id> (<n> prints)`, `unchanged: N sets` and the manifest's state, exits 2 on missing or partial settings and 1 on any publish error.

**Deviations**: Added a test-only `-cdn-api` flag (like `sets import`'s `-tcgdex-base`) so the purge can be pointed at the fake CDN from the re-exec tests. The usage text is the `sets` subcommand's own usage (`setsUsage` in `sets.go`) plus the package doc in `main.go`; the playmat's `-h` lists only playmat flags, as before. `sets publish` refuses positional set names.

**Files changed**:
- `cmd/tcgreplay/sets.go`
- `cmd/tcgreplay/main.go`
- `cmd/tcgreplay/sets_test.go`

**Discoveries**: `cmd/tcgreplay/hosted_cache_test.go` already defines a `setSpacesEnv` helper; the publish tests use `setPublishEnv` (which also sets the CDN token) to avoid the clash. `runMain` subprocesses inherit `t.Setenv` values, so the bucket settings are given through the environment.

### 2026-11-08 — Task: Publish on merge workflow

**What was done**: Added the project's first GitHub Actions workflow, `.github/workflows/publish-cardsets.yml`. It runs on pushes to `main` that touch `cardsets/**` and by hand (`workflow_dispatch`), never overlaps another publish, runs `go run ./cmd/tcgreplay sets check` and then `sets publish` with the four `TCGREPLAY_SPACES_*` values and `TCGREPLAY_SPACES_CDN_TOKEN` taken from repository secrets. Its header comment explains what it publishes, how quickly playmats pick a publish up and which secrets it needs.

**Deviations**: None.

**Files changed**:
- `.github/workflows/publish-cardsets.yml`

**Discoveries**: `go run ./cmd/tcgreplay` builds without Node (the board and site placeholders are checked in); `sets check` over the committed catalogue is clean, and a dry-run publish of it reports every set as new. actionlint is not installed locally, so the workflow was checked by YAML parse only; its first real run is a manual check.

### 2026-11-08 — Task: Deploy workflow that skips card data

**What was done**: Added `.github/workflows/deploy.yml`, which runs `doctl apps create-deployment "$APP_ID" --wait` (via `digitalocean/action-doctl@v2`) on pushes to `main` that change anything outside `cardsets/**`, and by hand. Set `deploy_on_push: false` in `.do/app.yaml` and rewrote its header comments to say where deploys now come from and that a card-data-only push never restarts the playmat.

**Deviations**: The app id secret is passed to `doctl` through an `env` value rather than interpolated into the shell line. The open assumption (a native App Platform path filter) was not taken: the app spec's `github` source has only `repo`, `branch` and `deploy_on_push`, with no path filter, so the workflow is used as planned.

**Files changed**:
- `.github/workflows/deploy.yml`
- `.do/app.yaml`

**Discoveries**: The website build parses `.do/app.yaml` (`site/src/lib/facts.ts`) for the playmat's limits, so edits there must keep it valid YAML with the playmat service's `envs` intact. The new spec only takes effect once the maintainer runs `doctl apps update <app-id> --spec .do/app.yaml` (human task "Set up publishing secrets and the published sets address").

### 2026-11-08 — Task: Catalogue snapshot

**What was done**: Added `internal/catalog` with `Snapshot`, an immutable card database implementing `cards.DB` (`Card`, `Cached`, `Resolve`, `ByName`) plus `SetID`, `Version`, `Sets` and `Prints`, built from converted built sets (`convertSet`, `NewSnapshot`) by the same index rules as `internal/cards`' catalogue (shorter id wins a shared code, Pocket skipped for names, art then newer release then lower number, `codeAliases`/`setAliases`, the `basic ` prefix, move matching). `Next` builds the following snapshot reusing every unchanged set's `*setData`. A nil or `Empty` snapshot answers not found everywhere.

**Deviations**:
- Ran as a single-task run (orchestrator answer A) because the plan's next tasks in order are human tasks; the human tasks 1e0644d4 and e0a9670e stay unticked.
- `energyArt` was not moved into `catalog`: the card-set-import work already has `Print.Card` call `cards.EnergyArt`, and `cardset` cannot import `catalog`, so the borrowed Basic Energy art stays in `cards` and reaches the snapshot through `Print.Card`.
- `nameKey` is exported from `cards` as `cards.NameKey` rather than copied; `codeAliases`, `setAliases`, `normCode` and `findNumber` are copied into `catalog` (the `cards` copies go with the client in "Remove the on-demand card client").
- First goal check found by-name ranking counted borrowed Energy art as art; fixed by recording per print whether its image is its own (`setData.art`), with a regression test.

**Files changed**:
- `internal/catalog/snapshot.go`
- `internal/catalog/snapshot_test.go`
- `internal/cards/cards.go`

**Discoveries**: `Print.Card` already applies borrowed Basic Energy art, so anything ranking or filtering on `Card.Image` after conversion must tell borrowed art from the print's own image list.

### 2026-11-08 — Task: Image source list and board fallback

**What was done**: `cards.Card` now carries the print's whole ordered image list (`Images []ImageRef{URL, File}`, with `Image` kept as the first entry, `ImageList()` and `ImageIsFile()`). `Print.Card` fills it in the print's order, keeping TCGdex base paths under `https://assets.tcgdex.net/` and https image files (malie.io jpgs) and leaving out anything else. `/api/card` sends an ordered `images` list (`image`, `image_file`, `image_fallback`, `image_version`) in place of the single image fields; base-path entries still go through the image cache (copy, version, TCGdex fallback), file entries go as is. The board builds the candidate addresses from that list and moves to the next on every load error, showing the text stand-in or Energy token after the last. Borrowed art (the board's same-name borrowing and the snapshot's Basic Energy borrowing) takes the other print's whole list.

**Deviations**:
- Single-task run (orchestrator answer A).
- The e2e cases rewrite `/api/card` responses for the board harness's Riolu and Energy (as the existing image tests do) instead of adding `tst-9`..`tst-12` to `cardstest`, because the harness position only shows the cards already in play.
- Basic Energy's whole-list borrowing is done in `catalog.build` (a copied card takes the Crown Zenith print's `ImageList()` when the snapshot holds it), since `cards.EnergyArt` cannot see other prints; `EnergyArt` itself now fills `Images` with the borrowed base path.
- Hosted and board tests that read the old single image fields now read the first entry of `images`.

**Files changed**:
- `internal/cards/cards.go`
- `internal/cards/imageref_test.go`
- `internal/cardset/card.go`
- `internal/cardset/format_test.go`
- `internal/catalog/snapshot.go`
- `internal/catalog/snapshot_test.go`
- `internal/webui/server.go`
- `internal/webui/webui_test.go`
- `internal/webui/borrowed_art_test.go`
- `cmd/tcgreplay/hosted_cache_test.go`
- `web/src/lib/store.svelte.js`
- `web/src/components/Card.svelte`
- `web/src/components/Zoom.svelte`
- `web/src/components/Modal.svelte`
- `web/e2e/images.spec.js`

**Discoveries**: The worktree needs `cd web && npm ci` before the Playwright suite runs (no `node_modules` in a fresh worktree); the browsers in `~/.cache/ms-playwright` are shared. The board's image stage is now an index into the candidate list, so every size has the same number of candidates and one stage counter serves the mat, the zoom and the preview.

### 2026-11-08 — Task: Catalogue loader with local copy

**What was done**: Added `catalog.Loader` (`internal/catalog/loader.go`). `Start(ctx, wait)` loads the local copy under `<CacheDir>/cardsets/` (every set verified against its sha256; a copy with any set missing or damaged is not used at all), then, unless `Offline`, runs a check bounded by `wait`. `Check` fetches `manifest.json` anonymously from `Base` (an http(s) address or a directory), returns early when its sha256 matches the current version, downloads only sets whose hash changed (8 at a time, reusing a verified local object when present), gunzips (64 MiB cap), verifies, converts, writes the objects to the local copy, builds `Next`, writes the local manifest and a version file, swaps atomically and prunes objects no longer named. Checks are serialised; a failure leaves the snapshot in place, is kept in `Status().LastError` and logged once per outage, with one recovery line. `Run` checks every `Interval` (default 5 minutes). `Status`, `Snapshot()` (for the game manager) and `SetID` serve the admin page.

**Deviations**:
- Single-task run (orchestrator answer A).
- The outage logging is a small down/up flag in the loader rather than a copy of `cards.breaker` (no retry window is needed: checks are already spaced by the interval).
- The local copy's manifest is rebuilt from the snapshot's own sets (same bytes as the publisher's in the normal case), plus a `version` file holding the published version and the sha256 of the local manifest it belongs to, so a restart reports the published version and a version left by an interrupted write is ignored.
- A set whose own manifest entry has an unknown format is skipped with a log line and keeps its previous version, rather than failing the check.
- Open question on memory answered: the full committed catalogue (206 sets, 21,439 prints) takes about 21 MB of heap as one snapshot and loads in about 70 ms (180 ms from the local copy), well under the 150 MB stop line.

**Files changed**:
- `internal/catalog/loader.go`
- `internal/catalog/loader_test.go`

**Discoveries**: Snapshot memory for the whole English catalogue is about 21 MB, so several pinned versions fit the 0.5 GB hosted instance comfortably.

### 2026-11-08 — Task: Games pin their catalogue snapshot

**What was done**: `game.Manager` now takes a `CardSource` (`Snapshot() cards.DB`) instead of a fixed `DB`. `New`, `Import` (and so `SetOpponentDeck` and reopening from a record) capture one snapshot at the start and use it for resolving the decklists and log and as the game's database (`Load`). `Game.DB()` exposes the pinned snapshot and `Manager.CardDB()` gives the open game's snapshot, or the current one with no game open; `card_text`, `/api/card` and the board's borrowed art read through it. The game's background card warming and `PrefetchCards` (and their MCP calls) are removed. `cardstest.FakeDB` is its own source, and `game.Static{DB}` wraps a fixed database.

**Deviations**:
- Single-task run (orchestrator answer A).
- The interim production wiring uses `game.Static{DB: client}` (in `cmd/tcgreplay/main.go` and `hosted.go`) rather than a cmd-local `clientSource` adapter. Until "Playmat reads published sets in every mode" lands, the old client's cards a game never fetched stay unknown to it, since warming is gone (interim only; never merged on its own).
- Test call sites passing a wrapper type that embeds `FakeDB` (`artFirstDB`, `slowDB`) wrap it in `game.Static`, because the promoted `Snapshot()` would return the bare pool and skip the wrapper's overrides.
- `cardlock_test.go` now asserts games never fetch a card (the warming tests became `TestMissingCardStaysUnknown`); a `presence_respond_test.go` wait on the removed warming was dropped.
- `TestGuideInstructions/hosted/inmemory` fails intermittently under `-race` (about 1 in 15, "TempDir RemoveAll cleanup: directory not empty"). It does not touch card data and was left alone.

**Files changed**:
- `internal/game/manager.go`
- `internal/game/import.go`
- `internal/game/game.go`
- `internal/game/verbs.go`
- `internal/game/pin_test.go`
- `internal/game/cardlock_test.go`
- `internal/game/presence_respond_test.go`
- `internal/game/applied_test.go`
- `internal/game/game_test.go`
- `internal/game/cardsource_test.go`
- `internal/game/clear_test.go`
- `internal/mcpserver/server.go`
- `internal/mcpserver/pin_test.go`
- `internal/webui/server.go`
- `internal/webui/pin_test.go`
- `internal/webui/borrowed_art_test.go`
- `internal/cards/cardstest/cardstest.go`
- `cmd/tcgreplay/main.go`
- `cmd/tcgreplay/hosted.go`
- `cmd/tcgreplay/hosted_cache_test.go`

**Discoveries**: A test type that embeds `cardstest.FakeDB` inherits `Snapshot()` returning the bare pool, silently bypassing its own `Card`/`Cached`/`ByName` overrides; wrap such types in `game.Static` when passing them to `game.NewManager`.

### 2026-11-08 — Task: Admin page checks for published sets

**What was done**: `cacheadmin` now works over a `Catalog` (`SetID`, `Check`, `Status`; a `*catalog.Loader` satisfies it) instead of the card client. `POST /admin/check` runs `CheckNow` on the request (bounded by 2 minutes), logs it, remembers the outcome and redirects to the page, or shows the error with 502. The page ("Card sets and images") shows the loaded catalogue's version (12 hex), set and card counts, published address, last check, last change and last error, with a "Check for published card sets now" button and the last admin check's outcome. Clearing card definitions is gone: the two forms are "Clear images: everything" and "Clear images for one set" (set names resolved through the catalogue), shown only when the image cache exists; without it a clear is refused (409).

**Deviations**:
- Single-task run (orchestrator answer A).
- Until "Playmat reads published sets in every mode" wires the loader, `cmd/tcgreplay/hosted.go` passes a small `clientCatalog` adapter over the old client (set names only; its check reports that published sets are not loaded yet).
- The hosted admin tests in `cmd/tcgreplay` were adjusted now (image-only clears, a refused check for non-admins, a 409 without the shared store) rather than moved wholly to the switch-over task, so the suite passes at this task boundary. `cmd/tcgreplay/hosted_test.go` changed one call to the new `hostedAdmin` signature.

**Files changed**:
- `internal/cacheadmin/cacheadmin.go`
- `internal/cacheadmin/handler.go`
- `internal/cacheadmin/page.go`
- `internal/cacheadmin/cacheadmin_test.go`
- `cmd/tcgreplay/hosted.go`
- `cmd/tcgreplay/hosted_admin_test.go`
- `cmd/tcgreplay/hosted_test.go`

**Discoveries**: None.

### 2026-11-08 — Task: Legacy card data clean-up command

**What was done**: Added `tcgreplay sets cleanup-legacy [-delete]`. With the `TCGREPLAY_SPACES_*` settings it lists every object under the constant prefix `tcgdex/` (the retired on-demand card cache), prints the count and the first 20 keys, and deletes nothing unless `-delete` is given; with it, it deletes them in batches and reports each key the bucket refused (exit 1). It never lists or deletes any other prefix (image copies, published card sets). The bucket-settings lookup is shared with `sets publish` (`bucketFromEnv`).

**Deviations**: Single-task run (orchestrator answer A). Until the switch-over is deployed the hosted playmat still writes card data under `tcgdex/`, so running the command with `-delete` before then would only empty a cache that refills; the human task "Remove legacy card data from the bucket" already requires the switch to be deployed first.

**Files changed**:
- `cmd/tcgreplay/sets.go`
- `cmd/tcgreplay/sets_test.go`

**Discoveries**: None.

### 2026-11-08 — Task: Publish every built set

**What was done**: Done by the maintainer (a person's task), reported through the orchestrator: `go run ./cmd/tcgreplay sets publish` from this branch's worktree with the bucket keys exported uploaded every built set and wrote the manifest ("manifest updated in bucket tcgreplay"); a second run printed "unchanged: 206 sets" and "manifest unchanged". `https://tcgreplay.ams3.cdn.digitaloceanspaces.com/cardsets/manifest.json` answers 200 `application/json`, format 1, listing every set. Ticked on the maintainer's report; no agent wrote to the bucket.

**Deviations**: Published by hand from the worktree rather than by the publish workflow, which exists only once this branch is merged. The maintainer's first attempt failed with `list published set 2011bw: spaces: 404 NoSuchKey` because `TCGREPLAY_SPACES_ENDPOINT` named the bucket's own host (`https://tcgreplay.ams3.digitaloceanspaces.com`), and the dry run before it wrongly reported every set as never published (the manifest GET 404'd the same way). Settings validation is tightened in "Playmat reads published sets in every mode".

**Files changed**:
- none (bucket only)

**Discoveries**: The published-sets CDN address is `https://tcgreplay.ams3.cdn.digitaloceanspaces.com/cardsets/` (bucket `tcgreplay`, region `ams3`).

### 2026-11-08 — Task: Playmat reads published sets in every mode

**What was done**: Every run mode now builds a `catalog.Loader` from the config and passes it to the game manager: `-cardsets-url` (default `$TCGREPLAY_CARDSETS_URL`, else `https://tcgreplay.ams3.cdn.digitaloceanspaces.com/cardsets/`, the address the maintainer gave), `-cardsets-interval` (default `$TCGREPLAY_CARDSETS_INTERVAL`, else 5m; 10s..10m enforced, exit 2 otherwise), `-cache` (the local copy) and `-offline` (local copy only); `config.args()` carries the new flags to a spawned daemon. In-process and hosted modes wait up to 30s at start for the published sets before serving; hosted mode now keeps the Spaces settings for image copies only (`hostedImages`), and its admin page runs over the loader. The old shared card cache wiring, `newCardDB`, `hostedCardDB`, `warmCatalog`, the interim `clientCatalog` adapter and the `cardBase` test hook are gone. `spaces.FromEnv` now refuses a `TCGREPLAY_SPACES_ENDPOINT` whose host is the bucket's own (`<bucket>.<region>.digitaloceanspaces.com`) or any `*.cdn.digitaloceanspaces.com` address, naming the setting and the expected `https://<region>.digitaloceanspaces.com`.

**Deviations**:
- Single-task run, after the maintainer completed "Publish every built set" and gave the CDN address (orchestrator answer A).
- The daemon serves from its local copy at once and runs its first online check in the background (`Loader.Start` with wait 0), because the shim is already connected to the socket and its handshake replay has a 10s deadline; the in-process and hosted modes wait the planned 30s.
- Added the endpoint validation fix the orchestrator asked for: the maintainer's first real publish failed with `list published set 2011bw: spaces: 404 NoSuchKey` (and its dry run wrongly reported every set as never published) because the endpoint named the bucket's own host; tests in `internal/spaces/config_test.go`, `cmd/tcgreplay/hosted_cache_test.go` and `cmd/tcgreplay/cardsets_test.go`.
- The corpus regression lives in `internal/game/corpus_test.go` (package `game_test`), as the plan allowed to avoid an import cycle.
- `cmd/tcgreplay/shim_test.go`: fixed a data race already in `TestRelaySurvivesDaemonRestart` (it failed under `-race`).
- No `make dev-cardsets` target was added (the plan marked it optional).

**Files changed**:
- `cmd/tcgreplay/main.go`
- `cmd/tcgreplay/daemon.go`
- `cmd/tcgreplay/hosted.go`
- `cmd/tcgreplay/cardsets_test.go`
- `cmd/tcgreplay/hosted_cache_test.go`
- `cmd/tcgreplay/hosted_admin_test.go`
- `cmd/tcgreplay/hosted_test.go`
- `cmd/tcgreplay/guides_test.go`
- `cmd/tcgreplay/timing_test.go`
- `cmd/tcgreplay/shim_test.go`
- `internal/catalog/loader.go`
- `internal/game/corpus_test.go`
- `internal/spaces/config.go`
- `internal/spaces/config_test.go`

**Discoveries**: Every corpus battle log imports with every resolved card named and every Trainer and special Energy carrying effect text from the committed catalogue. A path-style Spaces client given the bucket's own host as its endpoint 404s every key, which a publish reads as "nothing published yet"; the settings now refuse that endpoint up front.

### 2026-11-08 — Task: Remove the on-demand card client

**What was done**: Deleted the TCGdex card client and everything only it used from `internal/cards` (`catalog.go`, `fresh.go`, `flight.go`, `warm.go`, `store.go`, `spacesstore.go`, `tiered.go`, `clear.go`, the `Client` half of `cards.go`) with their tests, plus `internal/game/{cardsource,spacescache,clear}_test.go` and `internal/deck/live_test.go`. `internal/cards` now holds the card types, `cards.DB`, `ErrNotFound`/`ErrUnknownSet`, the image list helpers, borrowed Basic Energy art, `LogCodeID`, `NameKey`/`SameName`, the image scope and the image cache (unchanged). The image cache's timing constants and `listAll` moved into `scope.go`; `DefaultCacheDir` moved to `internal/catalog`. Added `internal/catalog/nofetch_test.go`, which fails if any non-test Go file under `cmd/` or `internal/` other than `internal/cardset/tcgdex.go` names `api.tcgdex.net`, with a scanner test over a fake tree. Comments and the `card_text` tool description no longer mention a TCGdex cache.

**Deviations**:
- Single-task run.
- `energyArt` stays in `cards` (it is what `Print.Card` applies).
- The test helpers the image tests shared with deleted files (`waitFor`, `fakeClock`, `testEpoch`, `logBuffer`, `outageModes`) moved to `internal/cards/helpers_test.go`.
- The first goal check judged criterion 1 not met because `tcgreplay sets cleanup-legacy` (task 5c84b8f7) deletes the retired cache's objects in the bucket. The retry, with no code change, judged it met: the clean-up is the planned one-off maintainer command, not on-demand card data.

**Files changed**:
- `internal/cards/cards.go`
- `internal/cards/scope.go`
- `internal/cards/scope_test.go`
- `internal/cards/logcode_test.go`
- `internal/cards/images.go`
- `internal/cards/helpers_test.go`
- `internal/cards/{catalog,fresh,flight,warm,store,spacesstore,tiered,clear}.go` (deleted) and their tests (deleted)
- `internal/game/{cardsource,spacescache,clear}_test.go` (deleted)
- `internal/deck/live_test.go` (deleted)
- `internal/catalog/loader.go`
- `internal/catalog/nofetch_test.go`
- `internal/cardset/tcgdex.go`
- `internal/mcpserver/server.go`
- `internal/spaces/doc.go`
- `cmd/tcgreplay/main.go`
- `cmd/tcgreplay/hosted.go`

**Discoveries**: None.

### 2026-11-08 — Task: Publishing documentation

**What was done**: Documented publishing and how playmats load card data:
- `cardsets/README.md` gained a "Publishing" section: on merge and by hand (`sets publish`, `-dry-run`, `-to`), the published layout and rollback, pickup times (5-minute checks, the admin's "Check now", open games keep their cards), the offline copy and first-run network need, `-cardsets-url`, the workflow secrets, and `sets cleanup-legacy`. It also explains how a print's image list order decides the art the board tries first.
- `README.md` describes how sets are published and loaded, and the deploy workflow.
- The design doc's card data, runtime, hosted, admin page, settings table and operator steps are rewritten for published sets, image copies only in the bucket, the deploy workflow and the clean-up; it also documents the new `-cardsets-url` and `-cardsets-interval` settings.
- `.do/app.yaml`, `Dockerfile` and `Makefile` comments are updated, and `docker-run` passes `TCGREPLAY_CARDSETS_URL` and `TCGREPLAY_CARDSETS_INTERVAL` through.
- The desktop page explains the first-run download, the offline copy and pickup within minutes.

**Deviations**: Single-task run. The first goal check found the website's privacy page still saying the desktop playmat fetches card data from TCGdex and the hosted one "looks cards up there". It was reworded (card data is the project's published copy; images load from the playmat's store, TCGdex or malie.io), and the footer credit names malie.io too. The design doc's remaining "TCGdex text" mentions were made neutral. Accepted on the retry.

**Files changed**:
- `cardsets/README.md`
- `README.md`
- `TCG Review Playmat Design and Build Plan.md`
- `.do/app.yaml`
- `Dockerfile`
- `Makefile`
- `site/src/content/docs/desktop.md`
- `site/src/pages/privacy.astro`
- `site/src/layouts/Base.astro`

**Discoveries**: The website's privacy page (`site/src/pages/privacy.astro`) states where the playmat gets card data, so it must change whenever card data's source does; its browser test only checks the outside services are named.

### 2026-11-08 — Task: Rewrite card cache knowledge entries

**What was done**: Through the knowledge CLI (repo tier, store `tcgreplay`):
- **Deleted** `gotchas/tcgdex-cache-never-expires.md`.
- **Added** `architecture/published-card-sets.md`, which covers:
  - the published layout and manifest, and publishing;
  - the loader (start, interval, admin check, local copy, legacy files ignored, 30 s start wait, the daemon's background check);
  - snapshot pinning per game and the admin page;
  - the image copies (carried over from the old entry);
  - the clean-up command.
- **Rewrote, so none describes the on-demand cache as current:**
  - `gotchas/board-card-info-per-page-load.md`: open games keep their cards; the image candidate list; the admin's image clear.
  - `gotchas/new-set-tcgdex-data-incomplete.md`: "the freeze" is history; gaps now surface in import reports.
  - `learnings/card-text-always-english.md`: English comes from `Print.Card("en")`.
  - `gotchas/tcg-live-log-card-codes.md`: the set-code index lives in `internal/catalog`; borrowed art is a whole list.
  - `gotchas/spaces-endpoint-is-region-not-bucket.md`: such endpoints are now refused; adds the publish failure the maintainer hit.
  - `gotchas/cdn-purge-needs-cdn-delete.md`: who purges.
  - `learnings/card-image-sources.md`: TCGdex is the import's source, not the playmat's database.
- `architecture/games-read-cards-through-their-db.md` was already rewritten during "Games pin their catalogue snapshot".

**Deviations**: Single-task run. No code changed. The first goal check found `learnings/card-image-sources.md` still calling TCGdex "the playmat's card database"; it was reworded (the plan's "any other hit" clause), and the retry was accepted. The reviewer was given a read-only export of the entries (through `spektacular knowledge read`) rather than the store files.

**Files changed**:
- none in the code (knowledge store only)

**Discoveries**: None.
