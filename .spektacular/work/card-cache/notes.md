# Card cache and image sources: notes for a spec

Discussion of 2026-10-26. Not yet specced; the spec is next.

## Problem

The card layer (`internal/cards`) works for one player's games but does not
extend:

1. **It never refreshes.** `getJSON` serves any cached file forever: the set
   list, each set's card list, the set-code index and every card. New sets,
   images TCGdex adds later and text fixes are never seen. The local cache's
   `sets.json` dates from 2026-10-16. See knowledge
   `gotchas/tcgdex-cache-never-expires.md`.
2. **It is tied to TCGdex.** `cards.Client` is both the TCGdex client and the
   cache. Images bypass the playmat: the board loads
   `assets.tcgdex.net/<image>/low.webp` directly. Adding a source or a fallback
   means editing the client.
3. **It is tied to one local disk.** The hosted image has no volume, so the
   cache is lost on each redeploy and cannot be shared between instances.

Trigger: MEP 33 Mega Lucario ex (`mep-033`) has card data but no image on
TCGdex, so the board shows its text stand-in. See knowledge
`learnings/card-image-sources.md`.

## Direction agreed

- **No Postgres.** The cache is small (about 20,000 English prints, roughly
  100 MB of JSON), can be rebuilt from TCGdex, and is looked up by key only.
  Postgres would be unusable by the desktop build (a single binary the Claude
  app launches) and would make the cache the only stateful part of a system that
  keeps games in memory. Revisit only if the server comes to hold data it
  cannot rebuild (saved games, accounts, match history).
- **Separate sources from the cache.**
  - Card data: a chain of providers, TCGdex first.
  - Images: their own chain:
    1. the print's own TCGdex image;
    2. another TCGdex print with the same name and attacks;
    3. malie.io's TCG Live export, if malie agrees (ask on their Discord).
  - A new source is one more provider.
- **Freshness policy.** Each entry stores when it was fetched, with a lifetime
  by kind:
  - the set list: about a day;
  - card data: weeks;
  - "not found" and "no image": hours.

  Stale entries are served immediately and refreshed in the background.
- **Pluggable storage behind a small `Store` interface** (get and put by key,
  with the fetch time):
  - desktop and dev: local disk (embedded SQLite, with a pure-Go driver, only
    if per-file storage becomes awkward);
  - hosted: DigitalOcean Spaces (S3-compatible, CDN built in, about
    $5 a month for 250 GB and 1 TB of transfer; check current pricing).
    Card JSON is private and images are public.
- **Images served through the playmat.** `/api/image?def=…`:
  1. if the image is in Spaces, redirect to its CDN URL;
  2. otherwise run the image chain, upload the result as public, then redirect.

  Desktop does the same against the local disk.
- **Refresh on demand.** A CLI flag or admin endpoint refreshes a card or set
  without deleting files.

## Open questions for the spec

- Exact lifetimes for each kind of entry, and whether the background refresh
  is per request or a periodic sweep.
- Whether to turn on the Spaces CDN, and how to name and lay out the bucket
  (keys for card JSON vs images, image sizes: low and high).
- Spaces credentials and settings as environment variables, alongside
  `TCGREPLAY_GOOGLE_*`.
- Whether malie.io is in scope now or later, and how its set codes map
  (through each set's `abbr`, e.g. MEP to `mebsp`, MEG to `me1`).
- Moving the existing local cache to the new format (or just rebuilding it).
- How the borrowed-print image fallback is shown (it shows the other print's
  artwork).
- Whether the Energy art override (`energyArt`, `basicEnergyArt`) becomes an
  image provider.
