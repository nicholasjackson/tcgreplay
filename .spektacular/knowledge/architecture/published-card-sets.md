---
tags: [cards, catalog, cardsets, publishing, spaces, cdn, images, admin, hosting]
---

# Card data comes only from published card sets

The playmat carries no card definitions and never asks TCGdex or malie.io for
them while running. The maintainer builds every English set into `cardsets/`
(`tcgreplay sets import`, `internal/cardset`), and the built sets are published
to the project's DigitalOcean Spaces bucket; every playmat, hosted and desktop,
loads them from there. `internal/catalog/nofetch_test.go` fails if any program
code other than `internal/cardset/tcgdex.go` (the import's fetcher) names
`api.tcgdex.net`.

**Publishing** (`cardset.Publish`, `tcgreplay sets publish`, and the
`.github/workflows/publish-cardsets.yml` workflow on every push to `main` that
changes `cardsets/**`). Layout in the bucket, all public-read:

- `cardsets/sets/<id>/<first 16 hex of sha256(built file)>.json.gz`, cached for a
  year, never overwritten or deleted, so every old version stays retrievable.
- `cardsets/manifest.json` (`cardset.Manifest`: `format` 1, and per set its
  `path` relative to the manifest, full `sha256`, `prints` and set `format`),
  deterministic bytes, cached for a minute and purged from the CDN after each
  publish when `TCGREPLAY_SPACES_CDN_TOKEN` is set.

A publish refuses while `sets check` has an open item or when a set the
published manifest names is missing from `cardsets/sets/`; it uploads only sets
whose hash changed and writes nothing when nothing changed. Rollback is
publishing an older commit's `cardsets/`. Deploys skip `cardsets/**`-only
pushes (`.github/workflows/deploy.yml`, App Platform's `deploy_on_push` off),
so a data publish never restarts the playmat.

**Loading** (`catalog.Loader`, `internal/catalog/loader.go`). Base address
`-cardsets-url` / `TCGREPLAY_CARDSETS_URL` (default
`https://tcgreplay.ams3.cdn.digitaloceanspaces.com/cardsets/`; an https address
or a directory, such as `sets publish -to` output). It fetches the manifest
anonymously, downloads only sets whose hash changed (8 at a time), verifies each
against the manifest, converts prints with `cardset.Print.Card("en")` and swaps
in a new immutable `catalog.Snapshot` atomically. Checks run at start, every
`-cardsets-interval` (default 5m, 10s..10m) and on demand from `/admin`; they
are serialised, and a failure (unreachable, corrupt object, unknown format)
keeps the current snapshot and is logged once per outage. In-process and
hosted modes wait up to 30 s for the first check before serving; the desktop
daemon serves its local copy at once and checks in the background (the shim is
already waiting on its socket). With no copy and no reachable address the
snapshot is empty and every card is unknown.

**Local copy.** `<-cache>/cardsets/` (default `~/.cache/tcgreplay/cardsets/`)
mirrors the layout: objects named by hash, written via temp file and rename,
the manifest written after them, plus a `version` file holding the published
version and the sha256 of the local manifest it belongs to. A copy with any set
missing or damaged is not used at all. `-offline` uses only the copy. Files the
retired on-demand cache left in the cache directory (`sets.json`, `sets/`,
`cards/`, `set-index*.json`) are never read. The whole catalogue is about 21 MB
in memory.

**Snapshots and games.** A `Snapshot` implements `cards.DB` with the index
rules the playmat always had (shared set codes keep the shorter id, Pocket sets
skipped for names, art then newer release then lower number, `setAliases`,
borrowed Crown Zenith Basic Energy art with the borrowed print's whole image
list). `catalog.Next` reuses every unchanged set's converted cards, so older
snapshots pinned by open games cost little. Each game pins the snapshot current
when it is created, imported or reopened (see
`architecture/games-read-cards-through-their-db.md`): a publish never changes an
open game's cards.

**Admin page** (`/admin`, `internal/cacheadmin`, accounts in
`TCGREPLAY_ADMINS`). Shows the loaded catalogue (version, sets, cards, base,
last check, last change, last error), "Check for published card sets now"
(picks a publish up at once), and clears **image copies** only, everything or
one set by code or id (resolved through the current snapshot). Card data is
never cleared.

**Card images.** A card carries its print's whole ordered image list
(`cards.Card.Images`, TCGdex base paths and complete image files such as malie
jpgs); `/api/card` sends it as `images` and the board tries each in order. With
all four `TCGREPLAY_SPACES_*` settings (hosted only), `cards.ImageCache` copies
each TCGdex image the board shows to `images/<TCGdex path>/{low,high}.webp`,
public-read, `Cache-Control: public, max-age=604800`, never overwriting an
existing object; `/api/card` gives a copied entry its CDN address, the TCGdex
address as `image_fallback` and a content version (`image_version`, from the
objects' ETags) the board adds as `?v=`. Only `assets.tcgdex.net` addresses are
copied; other sources are loaded from their own address. The cache learns
existing copies by listing `images/` at start. Deleting image objects by hand
leaves the cache pointing at them until a restart: use the admin clear, which
deletes them, purges the CDN and lets the next view copy the current art.

**Clean-up.** The retired on-demand cache kept card data under `tcgdex/` in the
bucket; `tcgreplay sets cleanup-legacy` lists it and, with `-delete`, removes it,
never touching `images/` or `cardsets/`.
