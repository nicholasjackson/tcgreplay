---
tags: [importer, battle-log, cards]
---

# The battle log does not print every move a card's effect makes

TCG Live's log shows that an effect happened but can leave out cards it moved,
and the format changes over time. Wally's Compassion (me01-132) heals a Mega
Evolution Pokémon ex and puts all its Energy into the hand; older logs print
the heal and a "moved ... to their hand" line, newer ones print only:

```
NicJackson played Wally's Compassion.
- NicJackson's Mega Lucario ex healed 130 damage.
```

An importer that replays only printed lines leaves the Energy attached, and
the error surfaces much later (a knockout discarding Energy the Pokémon never
had), far from the cause.

What to do: model such effects in `internal/importer/implied.go`, keyed by the
card's log name and quoting the card text, run from the handler of the line
that shows the effect happened (heals run `afterHeal`). Make each one skip when
the log does print the move, so older and newer logs both replay correctly,
and test both forms. To find candidates, grep the card text in
`cardsets/sets/*.json` for the effect pattern; as of November 2026 Wally's
Compassion is the only card in the pool that heals and then returns attached
cards to the hand.
