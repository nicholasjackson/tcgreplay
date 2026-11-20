---
created_date: "2026-11-08"
document_status: draft
project: tcgreplay
spec: 20261108105504-9abfa10e-card-set-publishing
plan: 20261108105504-9abfa10e-card-set-publishing
---


# Card set publishing

Card data now comes from the project's own published card sets, not from TCGdex while you play. New sets, promos, art and text fixes reach every playmat within minutes of being merged, with no new release and no restart. Games already open keep the cards they started with. The desktop playmat keeps a local copy, so after its first run it plays offline. Card images now try each source a print lists in turn, so art TCGdex lacks (for example Delta Reign's) shows on the board.

> Derived from project tcgreplay, spec/plan 20261108105504-9abfa10e-card-set-publishing. See the project-level record for the full feature.

## What changed in this repo

**Publishing**

- New `tcgreplay sets publish [-dry-run] [-to <dir>]` command, backed by the publisher in `internal/cardset` (`manifest.go`, `publish.go`, `target.go`).
- It uploads only changed sets, as content-addressed gzip files under `cardsets/sets/`, plus a short-cached `cardsets/manifest.json` that it purges from the CDN.

**Workflows**

- New `.github/workflows/publish-cardsets.yml`: check and publish on pushes to `main` that change `cardsets/`.
- New `.github/workflows/deploy.yml`: deploy on pushes that change anything else.
- `.do/app.yaml` now sets `deploy_on_push: false`.

**Loading** (new `internal/catalog`)

- An immutable catalogue snapshot answers every card lookup.
- A loader checks the published sets every 5 minutes, or on demand, and keeps a verified local copy.
- The new settings are `-cardsets-url` / `TCGREPLAY_CARDSETS_URL` (default `https://tcgreplay.ams3.cdn.digitaloceanspaces.com/cardsets/`) and `-cardsets-interval` / `TCGREPLAY_CARDSETS_INTERVAL` (default 5m, allowed 10s to 10m).

**Games**

- `game.Manager` pins one snapshot per game.
- `card_text`, `/api/card` and the board read through the open game's snapshot.

**Images**

- `cards.Card` carries the ordered image list, and `/api/card` sends it as `images`.
- The board falls back from one source to the next.

**Admin page**

- Shows the loaded catalogue and offers "Check for published card sets now".
- Clears images only.

**Removed**

- The TCGdex card client and its cache, from `internal/cards`.
- A test now fails if the TCGdex API host appears anywhere but the import's fetcher.

**Other**

- New `tcgreplay sets cleanup-legacy [-delete]`, which removes the retired `tcgdex/` objects from the bucket. It has been run, and the bucket is clean.
- `TCGREPLAY_SPACES_ENDPOINT` now refuses the bucket's own host and CDN addresses.
- Documentation updated: `cardsets/README.md` (Publishing), `README.md`, the design doc, the `Dockerfile`, the `Makefile`, and the website's desktop and privacy pages.

## Why

The playmat used to fetch card definitions from TCGdex on demand and cache them. As a result, players could see half-finished upstream data, and every card fix needed the cache cleared. Publishing reviewed card sets from this repository makes the repository the single source of card data. It also keeps the program free of card data, and stops a card-data merge from redeploying the hosted playmat and dropping open games.
