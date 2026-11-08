---
tags: [importer, battle-log, stadium, testing]
---

# The battle-log format for a Stadium pair is assumed, not verified

No real TCG Live battle log in which a player plays a Legendary Stadium pair
(Delta Reign's Legendary Pyroduct, Summit or Trench) had been seen when the
importer learned to replay pairs (2026-11-08). The line shapes it recognises
are guesses built from the single-Stadium line
`<player> played <card> to the Stadium spot.`:

- two consecutive Stadium-spot lines by the same player for the same name,
  the second onto that player's lone card, join as a pair (`as: "pair"`);
- one line naming the Stadium twice, `<name> and <name>`, plays both halves;
- when the pair is replaced, either one `- <player> discarded <name>.`
  subline per half or one `- <player> discarded 2 cards.` bullet list;
- two plays whose card codes name the same print are not a pair (warning).

The only fixture is made up:
`testdata/logs/synthetic_2026-11-08_ledjo23_legendary-summit-pair.txt`,
marked `"synthetic": true` in `testdata/index.json`.

**When the first real log with a pair turns up, check it against these
shapes before trusting a pair import.** Trace it with
`TRACE=<log> go test -run TestTrace ./internal/importer`, fix
`playStadium` in `internal/importer/handlers.go` to match what TCG Live
really prints (from the log's lines only, never the card's `playedAsPair`
mark: see `architecture/importer-determinism.md`), and add the real log to
the corpus in place of, or beside, the synthetic one. A pair that imports as
two separate Stadium plays, or a "no <name> for <player> in hand" problem on
a Stadium line, is the sign the guess was wrong.
