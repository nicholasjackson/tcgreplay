---
tags: [cards, images, tcgdex, webui, cardsets, malie, webp]
---

# Card image sources: TCGdex gaps and backups

TCGdex, the main upstream source of the card-set import, can return a print's
card data with no `image` field. A built print then has only the images other
sources give (the import adds malie.io's), and a print with none shows the
board's text stand-in (name, HP, stage) from `web/src/components/Card.svelte`,
or, for a non-Energy card, borrows the image list of the same Pokémon's
preferred print (`webui` `borrowedArt`). Rules and damage still work, since
they come from the card data. The board tries a print's images in the order its
built set lists them (`architecture/published-card-sets.md`). Seen on Mega Evolution Black Star promos, e.g.
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

## Hosted image copies cover every source, resized to WebP

In hosted mode (`internal/cards/images.go`, `ImageCache`) every card image is
copied to the bucket on first use and served from its CDN afterwards; the first
viewer gets the source address while the copy is made. An image never viewed is
never copied.

- **TCGdex** images are copied byte for byte (TCGdex already serves `low.webp`
  and `high.webp`) to `images/<lang>/<serie>/<set>/<number>/`.
- **malie.io** images (single full-size JPEGs, about 273 KB each) are fetched
  once, resized to `low.webp` (245 px wide) and `high.webp` (600 px), WebP
  quality 75, and stored under `images/malie/<path below tcgl/cards, without
  the extension>/` (a `FileSource`, `cards.MalieImages`). A 600 px card comes
  to about 60 KB, the size of TCGdex's own `high.webp`. Every copy is WebP, so
  TCGdex's transparent corners survive and the board needs no second format.
- `/api/card` lists a malie image's copy (a base address, with its content
  version) before the original file, so the board falls back to the file when
  the copy fails.
- The encoder is `github.com/gen2brain/webp`: libwebp transpiled to Go, no cgo,
  so the static `CGO_ENABLED=0` build still works. It adds about 4 MB to the
  stripped binary.
- **Gotcha:** an admin clear scoped to one set (`cards.Scope.Image`) matches
  TCGdex paths only, so it leaves malie copies in place; only clearing
  everything removes them.
- Desktop has no bucket, so it always loads images from the source.

## Asking malie.io for permission (status: asked, no reply yet)

The maintainer asked the malie.io maintainers on their Discord on 2026-11-08.
The questions were:

1. May tcgreplay read the TCG Live export at build time (`tcgreplay sets
   import`, a few requests per release, never live per user)?
2. May it republish card text built from the export on tcgreplay's own CDN
   (the published card sets)?
3. May it use malie's card images as a fallback for art TCGdex lacks, either
   linked to malie's CDN or copied?
4. How should malie.io be credited?
5. Is there a heads-up channel for breaking changes to the alpha export
   format?

**No reply yet** (as of 2026-11-08). Until one comes, the import keeps reading
malie by default, and every print lists its malie image after TCGdex's. If the
answer is no, import with `-no-malie` from then on and drop malie from the
image lists; if it asks for credit or a change channel, act on that. **Update
this section when a reply arrives**, with its date and what it allows.


