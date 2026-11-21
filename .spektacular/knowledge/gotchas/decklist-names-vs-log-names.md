---
tags: [importer, battle-log, decklist, cards]
---

# Key decklists by the log's card name, not the name as written

TCG Live's own decklist export and its battle log spell the same card
differently, so a list matched against a log by its written names fails:

- The export writes energy symbols, the log writes words: `Rocky {F} Energy`
  and `Basic {F} Energy` in the list are `Rocky Fighting Energy` and
  `Basic Fighting Energy` in the log.
- The card database names most Basic Energy without "Basic" (`sve-006` is
  `Fighting Energy`), but the log always says `Basic Fighting Energy`. So
  "use the resolved card's name" alone is not enough either.

Resolving the card (`deck.Resolve` uses set + number) succeeds either way,
which hides the bug: the importer keys every per-name map (the owner's
counts, prints and cards left to deal; the opponent's `oppLimit`, prints and
`infer.Reconcile` profile) by entry name, so a mismatch makes every Energy
"not in deck" and the replay fails from the opening hand. A real game did
exactly that with a list straight from the TCG Live deck editor.

What to do: name every list entry with `deck.LogName` before matching it to a
log, applied to the resolved card's name when a database is at hand
(`game.parseDeck` does this, feeding `import_log`, `set_opponent_deck`,
record rebuilds and `sample_hidden`) and to the written name otherwise
(`importer.Import` re-normalises its inputs, so direct callers and tests are
covered). Any new code that compares list names with log names, or with card
names bound from a log, must go through the same normalisation.

Related trap: with an assumed opponent list, a replay with problems used to
blame the opponent's list ("the assumed list conflicts with the log") even
when the problem was in the owner's list. The importer now falls back, and
warns, only when dropping the opponent list actually reduces the problems.
