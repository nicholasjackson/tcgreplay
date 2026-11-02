---
tags: [cards, images, tcgdex, webui]
---

# Card image sources: TCGdex gaps and backups

TCGdex, the playmat's card database, can return a print's card data with no
`image` field. When that happens the board draws its text stand-in (name, HP,
stage) from `web/src/components/Card.svelte`. Rules and damage still work, since
they come from the card data. Seen on Mega Evolution Black Star promos, e.g.
`mep-033` (MEP 33 Mega Lucario ex) and `mep-012`, while the `me01` prints of the
same card have images. A decklist naming several prints of one card brings each
print in exactly, so one copy can lack art while the others show it.

Backup sources, checked 2026-10-26:

- **malie.io TCG Live export.** Card data and images taken from the
  Pokémon TCG Live client, in 7 languages. Index:
  `https://cdn.malie.io/file/malie-io/tcgl/export/index.json`, keyed by locale
  (`en-US`), then by malie set id. Each set's `abbr` is the TCG Live decklist
  set code (MEP → `mebsp`, MEG → `me1`), so map through `abbr`, not TCGdex ids.
  Images:
  `https://cdn.malie.io/file/malie-io/tcgl/cards/jpg/en/<set>/<set>_en_<NNN>_std.jpg`
  (PNG and foil/etch textures also listed per card). It had MEP 33. It is the
  upstream of Limitless TCG's images (per Limitless's about page). Its format
  is marked alpha, and it is a fan project: ask on its Discord before relying
  on it from the hosted server.
- **pokemontcg.io.** A free API built for apps, but it had no image for the MEP
  promos (404) and intermittently returned empty bodies while being tested.
- **Limitless TCG.** It has the promos on its own CDN
  (`limitlesstcg.nyc3.cdn.digitaloceanspaces.com/tpci/<SET>/<SET>_<NNN>_R_EN.png`)
  but offers no public API, so using it means hotlinking.

No source is truly free to use: all card art belongs to The Pokémon Company,
and every one of these sites hosts it under fan-site tolerance.

A cheap first fallback needing no new source is to borrow the image of another
TCGdex print with the same name and attacks; it shows that print's artwork.
