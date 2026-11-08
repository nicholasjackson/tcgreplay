---
tags: [importer, determinism, seed, record]
---

# The importer is deterministic

Given the battle log, the deck text, the opponent list, the owner's name and the seed, an import always builds the same game. Its only randomness is one seeded PCG, and card-database lookups only fill a card's `Def`, never where a card goes.

This is what lets a game record (those five inputs) rebuild a game exactly, and it is what makes running without game storage viable. Any change to the importer must keep it: no unseeded randomness, no ordering that depends on map iteration or lookup results.

**This holds for card-data flags too.** A flag in card data that changes how a card is played, such as the `playedAsPair` mark on Legendary Stadiums, is read by the game verbs, the board and the assistant's tools, never by the importer. The importer recognises such play from the log's own lines: one line naming both halves, or a second same-name Stadium played onto a lone half by the same player. The user decided this on 2026-11-08 while planning the paired-stadiums spec, over reading the flag through the game's pinned catalogue snapshot.
