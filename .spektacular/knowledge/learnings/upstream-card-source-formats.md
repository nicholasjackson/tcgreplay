---
tags: [cards, tcgdex, malie, sets, images, cardsets]
---

# Upstream card source formats: TCGdex and the malie.io export

Checked 2026-11-08 while planning offline set import.

**TCGdex** (`https://api.tcgdex.net/v2/en`)

- `/sets` lists every English set as `{id, name, cardCount}` only (221 sets,
  Pocket included). Serie, release date and the decklist code
  (`abbreviation.official`) are only on `/sets/<id>`, along with
  `cards[{id, localId, name, image?}]`. Full definitions need one
  `/cards/<id>` request per print.
- A card body also carries `updated`, `pricing`, `thirdParty`, `variants`
  and `legal`. `pricing` and `updated` change daily: anything that stores or
  diffs card bodies must drop them, or every re-fetch looks like a change.
- Older sets write energy in text as `{L}`-style symbols ("attach up to 2
  Basic {L} Energy cards", `sv01-170`). New sets can arrive with HTML instead
  (see `gotchas/new-set-tcgdex-data-incomplete.md`), and a few write `[R]`.
- **Art often exists when the card data says none.** The asset server
  (`https://assets.tcgdex.net/en/<serie>/<set>/<localId>/high.png`, also
  `low.png`, `.webp`) has images for many prints whose card body has no
  `image`: some promos (`svp` 191, 192, 213-215, 225, 500), and every
  sub-set print, which lives under the **parent** set's path with the same
  local id: `swsh4.5sv-SV001` at `swsh/swsh4.5/SV001`, `swsh9tg-TG01` at
  `swsh/swsh9/TG01`, `swsh12.5gg-GG01` at `swsh/swsh12.5/GG01`. Probe the
  address before concluding a print has no art (checked 2026-11-08).
- `/sets` once answered with an empty body during a fetch; treat an empty or
  undecodable 200 as retryable.

**malie.io TCG Live export** (index
`https://cdn.malie.io/file/malie-io/tcgl/export/index.json`, `en-US`)

- Covers only TCG Live's sets: 33 English keys, Scarlet & Violet and Mega
  Evolution eras (`sv1` … `me6`, promos `svbsp`/`mebsp`, energies `sve`/`mee`),
  plus TCG Live-only or empty keys (`svalt`, `mealt`, `me7`, `me5-5c`,
  `me5-5m`) with no TCGdex set. Nothing older than Scarlet & Violet.
- Each set file is a list with **two entries per print**: the standard one
  (`ext.tcgl.longFormID` contains `_std_`) and a reverse-holo `_ph_` one with
  the same text. Delta Reign: 228 entries for 141 prints. De-duplicate by
  collector number, preferring `_std_`.
- `ext.tcgl.cardID` (`me6_1`) is the same code TCG Live logs use, so
  `cards.LogCodeID` turns it into the TCGdex id (`me06-001`). Codes that
  break the pattern (30th Celebration's `me5-5`, TCGdex `30th`) need matching
  by set `abbr` against TCGdex's `abbreviation.official` instead.
- Enums are upper case (`POKEMON`, `STAGE1`, `GRASS`, `SUPPORTER`);
  evolves-from is only in prose, `stage_text: "Evolves from Surskit"`.
  Damage is `{amount, suffix}` (`×`, `+`); weakness is
  `{types, operator, amount}`.
- `text[]` entries have a `kind`: `ATTACK`, `ABILITY`, `TEXT_BOX` (a Trainer's
  or special Energy's effect, which TCGdex calls `effect`), `RULE_BOX` (the ex
  rule) and `REMINDER` (the "only 1 Supporter" line). TCGdex's `effect` matches
  `TEXT_BOX` alone.
- Energy appears as `{W}` both in text and in names ("Bubbly {W} Energy",
  which TCGdex names "Bubbly Water Energy"), so comparing the two sources needs
  `{W}` and `Water` treated as equal. Basic Energy is "Basic {G} Energy" where
  TCGdex writes "Grass Energy".
- Other shapes met when importing every set: a cost-free attack has cost
  `["FREE"]` (TCGdex lists no cost); kind `EFFECT` is a Pokémon's own rule
  text (Tera); the `std` and `ph` entries of one print have **different**
  `cardID`s, so group by collector number; the promo key `svbsp` has abbr
  `PR-SV` (TCGdex `SVP`); `30th` and `30th-c` share the code `30C`, and
  `me5-5m`'s three RGB Mews are TCGdex's `30th-R`, `30th-G`, `30th-B`.
- **The CDN (Cloudflare) answers 403 to Python's default `Python-urllib`
  User-Agent**, while curl, Go's default client and `tcgreplay-sets/<v>`
  are served. Set a User-Agent in ad-hoc scripts.
