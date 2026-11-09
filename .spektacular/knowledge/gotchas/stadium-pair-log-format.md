---
tags: [importer, battle-log, stadium, testing]
---

# How TCG Live logs a Stadium pair

A Legendary Stadium pair (Delta Reign's Legendary Pyroduct, Summit or
Trench) is not written as two Stadium plays. TCG Live writes the second half
**attached to the first, with no place**, then the first played to the
Stadium spot:

```
GameSpeer attached (me6_95) Legendary Pyroduct to (me6_94) Legendary Pyroduct.
GameSpeer played (me6_94) Legendary Pyroduct to the Stadium spot.
```

Seen in the first real log with a pair,
`testdata/logs/2026-11-09_gamespeer_mega-rayquaza-talonflame.txt` (in the
corpus). The importer recognises the shape from those lines alone, never from
the card's `playedAsPair` mark (`architecture/importer-determinism.md`):
`attachPairHalf` takes the attached half from the hand (with the line's first
card code) and holds it; the very next headline, the same player playing the
same name to the Stadium spot, plays both halves as one Stadium in one move
(`playStadium`, `internal/importer/handlers.go`). An "attached X to Y." line
with no place and two different names is a problem, not a pair.

The earlier guesses (two consecutive Stadium-spot lines; one line naming the
Stadium twice) were wrong and were removed on 2026-11-09.

**Still assumed:** how a pair being replaced or discarded is logged. The
importer accepts one `- <player> discarded <name>.` subline per half or one
`- <player> discarded 2 cards.` bullet list, tested only on
`testdata/logs/synthetic_2026-11-08_ledjo23_legendary-summit-pair.txt`
(marked `"synthetic": true`; its pair lines are in the real shape). When a
real log shows a pair replaced, trace it with
`TRACE=<log> go test -run TestTrace ./internal/importer`, compare, and add
it to the corpus.

Related quirk in the same log: Mega Rayquaza ex's Ruler's Roar logs
`- NicJackson shuffled their cards.` under the *opponent's* Ability (the
wrong player); it is replayed as a no-op.
