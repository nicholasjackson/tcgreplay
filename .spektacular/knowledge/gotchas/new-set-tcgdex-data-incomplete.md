---
tags: [cards, tcgdex, cardsets, images, sets]
---

# A brand-new set's TCGdex data starts incomplete

TCGdex adds a new set before its data is complete. On Delta Reign (`me06`,
decklist code `DLR`, TCG Live log code `me6_<n>`), two days after release
(checked 2026-11-08), the data had these gaps:

- **No images** on any of the 141 cards.
- **No `effect`** on any Trainer (24) or special Energy (3), so Supporters,
  Items, Tools and Stadiums have no text at all.
- **No `evolveFrom`** on any Stage 1 or Stage 2 card. The evolve warning in
  `game/verbs.go` is then silently skipped.
- **HTML in attack and ability text** on 19 cards, for example
  `<span class="energy-symbol Fire" title="Fire">Fire</span>`. Older sets
  have none. The board shows text as plain text, not HTML, so raw tags would
  appear on the board, in `card_text` and in the effect reader's input; the
  card-set import strips them and `sets check` refuses any left.
- **No `regulationMark`**, and `legal.standard` is `false`.

Set and card ids still resolve: `Resolve("DLR", n)`, `LogCodeID("me6_84")`
and `ByName` all reach the right `me06-…` card without any code change.

**Players never see these gaps any more.** The playmat reads card data only
from published card sets (`architecture/published-card-sets.md`), which the
maintainer builds from TCGdex and malie.io with corrections and publishes only
when `sets check` is clean. A new set's gaps show up in its import report
instead; re-import the set once TCGdex fills them, and publish. (Before card
sets were published, an on-demand TCGdex cache could freeze a card's empty text
once its image arrived.)

**Rule text neither source has.** The English Legendary Stadiums (Legendary
Pyroduct, Summit and Trench, two different prints of each) carry a rule that
neither TCGdex nor the malie.io export's `text` includes: "You cannot play this
card by itself. You must combine 2 different Legendary Pyroduct from your hand
to play as 1 Stadium card." (confirmed by the user from the English card,
2026-11-08). The two halves are one Stadium in play, so the single-card stadium
slot (`state.Stadium`, one IID) cannot hold it as it stands. Card data taken
straight from either source silently loses this rule.

**Where complete data exists on day one.** The malie.io TCG Live export
(`https://cdn.malie.io/file/malie-io/tcgl/export/index.json`, `en-US`, set key
`me6`, `abbr` `DLR`) had full English text, regulation mark `J` and images
for every Delta Reign card the day after TCG Live got the set. See
`learnings/card-image-sources.md` for its terms and URL patterns.

