---
tags: [importer, tcgdex, cards, battle-log]
---

# TCG Live logs prefix card names with codes; TCGdex set codes collide

**Log card codes.** Since about November 2026, TCG Live battle logs put a card
code before every card name: `(me3_121) Meowth ex`, `(me2_85_ph) Battle Cage`,
`(zsv10-5_79) Air Balloon`. Older logs and the test corpus have no codes. Every
importer handler matches on bare names, so the codes must be stripped before
parsing. `importer.Normalize` does this. If they are not stripped, nothing
matches: the owner's opening hand fails and every later line fails with it. A
new log shape that seems to break everything is more likely a format change
than a cache problem.

The codes still name the exact print of each copy, which is how alternate arts
show as themselves on the board. The importer keeps, for each card, the code
the line that named it gives for that name, when the line (with its bullet
lists) gives exactly one; a list naming two prints of the same card gives none,
and a later line naming it alone (drawing it, evolving into it) fills it in.
`Options.DefOfCode` turns that code into the card's def, for both players, and a
copy without a usable code keeps the decklist's print (or `DefOf`'s for the
opponent). Never key prints by card name alone: one deck often runs two prints
of a name (`2 MEG 188` and `1 MEP 33`), and a name-keyed map gives every copy
the same one. `importer.LogCodes(log, name)` lists a name's codes, and
`cards.LogCodeID` turns a code into a TCGdex id using these rules:
- the series number is padded to 2 digits for Scarlet & Violet and Mega
  Evolution (`me3` → `me03`, `sv5` → `sv05`) but not for older series
  (`swsh1` stays `swsh1`, as TCGdex spells it)
- a `-5` half set becomes `.5` (`me2-5` → `me02.5`)
- `zsv10-5` → `sv10.5b` (Black Bolt) and `rsv10-5` → `sv10.5w` (White Flare)
- `mebsp` → `mep`, and `mee` stays `mee`
- the card number is padded to 3 digits
- trailing parts such as `_ph` are dropped

The mapping is a best guess, and some prints are not on TCGdex at all
(`me5-5_66`). Always check that the TCGdex card's name matches before using the
id, and fall back otherwise; `game/import.go`'s `DefOf` and `DefOfCode` do
this. A print TCGdex has but without an image (new promos such as `mep-033`)
gets the art of the same name's preferred print with the same moves from the
board server (`webui` `borrowedArt`), keeping its own identity. The log may name a different print than the decklist (for
example a secret rare `sv10_226` against `DRI 176`), so match cards by name,
never by print number.

**Set-code collisions.** TCGdex gives some sets the same official
abbreviation: `30th` and `30th-c` are both `30C`, and `cel25` and `cel25cc` are
both `CEL`. The set-code index keeps the shorter id (the main set), so the result
no longer depends on fetch order. Set codes that exports spell their own way
(`30th`, `30TH`, `M30`) go straight to a set id through `setAliases` in
`internal/cards`. The deck parser accepts set codes that start with digits
(`30th`). The set-code index is not persisted: it is rebuilt in memory from the
cached set files on each start (`internal/cards/catalog.go`), so a change to how
it is built takes effect on restart with no cache file to bump or delete.
