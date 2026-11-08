---
tags: [cards, tcgdex, sets, cardsets]
---

# TCGdex card data has systematic errors; don't trust a field just because TCGdex has it

Importing every English set into `cardsets/` (2026-11-08) and checking the
report items against printed cards turned up these TCGdex errors. The import
lets TCGdex win every field it has, so each one has to be corrected in
`cardsets/corrections/<set>.json`, and the same kind will turn up in new sets:

- **New-set ex cards.** On Delta Reign (`me06`) every Pokémon ex has suffix
  `EX` (the old Pokémon-EX mechanic) instead of `ex`, and evolved Mega ex
  and ex cards have stage `Basic`. `30th` has the same wrong stage on 10
  prints.
- **Special Energy marked `Normal`.** 14 Scarlet & Violet and Mega
  Evolution Special Energy cards (Prism, Ignition, Team Rocket's, the
  `me03`-`me05` typed ones, Reversal) have `energyType: Normal`.
- **Move kind swapped.** Azurill `me06-080`/`115`: TCGdex lists the
  cost-free attack Bouncy-Bounce Charge as an Ability.
- **Old wording on reprinted promos.** `mep` promos carry pre-Mega
  Evolution text and attack names that the printed cards changed (Creepy
  Wind, Ram, the new Damp wording).
- **Small typos.** Regulation mark `j` (`mep-051`); Raichu evolving from
  `Pikachu ex` (`sv02-064`, `211`); weakness `x2` instead of `×2` in
  several sets (cosmetic: the import's comparison ignores it).
- **Reprints missing their Poké-Power/Body.** Classic Collection reprints
  (`30th-c` Delcatty and Gengar, `cel25cc` Venusaur) lack the ability the
  original print has, and have no evolves-from. LEGEND halves
  (`30th-c-019`) and V-UNION pieces (`swshp-SWSH215`-`218`) have no
  attacks.
- **Old sets with unnamed moves.** Some EX, DP, Neo and e-Card prints have
  attacks or abilities with no name or text (e.g. `neo4-113`, `dpp-DP03`).
  Trainer Kit (`tk-*`) Pokémon often have no attacks at all.
- **Sets with no cards.** `wp`, `sp`, `rc` and `jumbo` have a card count
  but an empty English card list, so they build as empty sets.

The malie.io TCG Live export was right in every Scarlet & Violet and Mega
Evolution case above. Where neither source covers a print, use an identical
reprint elsewhere in the catalogue, then the printed image.
