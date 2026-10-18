---
tags: [importer, determinism, seed, record]
---

# The importer is deterministic

Given the battle log, the deck text, the opponent list, the owner's name and the seed, an import always builds the same game. Its only randomness is one seeded PCG, and card-database lookups only fill a card's `Def`, never where a card goes.

This is what lets a game record (those five inputs) rebuild a game exactly, and it is what makes running without game storage viable. Any change to the importer must keep it: no unseeded randomness, no ordering that depends on map iteration or lookup results.
