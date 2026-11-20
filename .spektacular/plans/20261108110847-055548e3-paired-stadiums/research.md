---
created_date: "2026-11-08"
document_status: final
closed_date: "2026-11-08"
---

# Research: 20261108110847-055548e3-paired-stadiums

## Alternatives considered and rejected

### Two separate fields for the spot (`State.Stadium IID` plus `State.StadiumPair IID`)

Keeps the 12 existing references compiling, but every one of them still needs review, and removing the first half needs a "promote the second" rule. A slice of at most two (`State.Stadium []IID`) makes `Find`, `remove`, `Locations` and `Check` plain loops (`internal/state/state.go:246`, `apply.go:110`, `invariants.go:43`). State JSON is never persisted (records hold events, `internal/game/record.go`), so the type change is safe. Rejected.

### The state decides pairing from card data

`internal/state/types.go:1-3` says state "knows nothing about card text or rules". Rejected: the verb, which can read the card, says so in the event (`MovePayload.As = "pair"`, like the stack roles `pokemon`/`energy`/`tool` at `types.go:189-193`).

### A pair as two move events, with the log collapsed when rendered

It would need collapsing in `describe`, `UISnapshot.Log`, `Game.Log`, `DeltaFor` and frames (`internal/game/reads.go:34-123`, `ui.go:41-117`), and the fold would pass through a lone-half state mid-action. Rejected for one event that moves both halves (`MovePayload.Pair`), so the log, frames and the fold see the pair as one.

### A new event type for the pair

Rejected. An optional `pair` field on the existing move event is additive, and old records fold unchanged.

### The importer decides pairs from card data

It would break `architecture/importer-determinism.md` ("card-database lookups only fill a card's `Def`, never where a card goes"). Rejected: the importer recognises a pair from the log's own shape.

### Guessing the TCG Live pair log line now

No real log exists (`testdata/logs/` has 11 logs, none with Legendary or Delta Reign cards, none with card codes). Both plausible shapes import wrongly today (importer research): two consecutive lines are taken as a replacement, and one line naming both is bound as a single card name, silently for the opponent. Rejected per the spec: build the importer against a real log, or record the deferral.

### A new board action, or a "Play as pair" menu item, for the second half

Not needed. The board already sends `play_trainer` when a hand Stadium is dropped on the spot (`web/src/lib/actions.js:105-110`). Completing a lone half is the verb's job, so dragging the second half works with no front-end action change. Rejected as unnecessary surface.

### A pair-rule line generated from the mark in `card_text`

`cards.Card.Text` (`internal/cards/cards.go:102-149`) could append a line such as "Played as a pair: 2 different prints of … are played together as 1 Stadium." whenever the mark is set. It duplicates the printed rule the data already carries (the card-set-import plan's Delta Reign corrections add it to each Legendary Stadium's effect text), so the assistant and the board zoom would show the rule twice, and code would state what a card reads. Rejected (user decision, 2026-11-08): the rule comes only from the card's effect text in the data, so card text matches the printed card. A future pair-played card is covered by the card-set guide, which says a marked print must also carry its printed rule, by correction if the sources lack it.

### Single-half warning only as a one-off toast from the verb

The acceptance criterion needs it shown "while the first half is alone and not once the pair is complete". Rejected for a standing warning built from state, next to `benchWarnings` (`reads.go:498`), which also reaches `UISnapshot.Warnings` and `get_warnings`.

## Chosen approach — evidence

- `internal/state/apply.go:180-183`: a move into an occupied spot is refused today. The pair role is an exception that applies only when the spot holds exactly one card.
- `internal/game/verbs.go:428-457` `PlayTrainer`: the single place where a Stadium replaces the old one. The importer has its own copy (`internal/importer/handlers.go:92-106`).
- `internal/game/game.go:339-444`: actions group events. `Undo` steps back over whole actions (`reads.go:610-648`), so a pair moved inside one action (or one event) undoes as one.
- `internal/game/reads.go:433-489` `turnWarnings` counts Stadium `trainer` actions ("played N Stadiums this turn"). A completion action must not count as a second Stadium.
- `internal/game/verbs.go:725-753` `Move`: the escape hatch, with `moveStack` (`:757-781`) as the model for moving many cards as one ("a Pokémon in play moves with its attachments").
- `web/src/lib/actions.js:105-110`: dropping a hand Stadium on the spot calls `play_trainer`. Other drops call `move` to `stadium`.
- `web/src/components/Strip.svelte:29,41-47,103-118,191-192`: the spot renders one `<Card>` and is sized for one `lg` card on desktop, `sm` on phone.
- `internal/webui/actions.go:200-205`: the lock check covers `a.IID` and `a.IIDs`. `:348-349` is the board's `play_trainer`.
- `internal/mcpserver/server.go:345-348` `IIDArg`, `:651-655` `play_trainer`, `:583-601` `card_text` (returns `c.Text()`), `:67-77` instructions. `reveal` (`:432-435`) and `retreat` (`:376`) are the models for extra iid arguments.
- `internal/mcpserver/guides_test.go` `TestGuidesNameRealTools`: any snake_case word in a guide must be a real tool or schema property, so `pair_iid` must exist in `play_trainer`'s schema before a guide names it.
- `internal/guides/guides_test.go`: guide text may not contain "Claude", "Nic ", or anything with "skill" or "tcg-" (knowledge `conventions/skills-text-and-packaging.md`).
- Card-set-import plan: `cardset.Print`, strict decode, corrections by JSON Pointer applied through a strict round trip ("a later spec that needs a new card field (the pair mark) adds it to the format and can then correct it"). `Print.Card(lang)` is the only conversion into `cards.Card`.
- Card-set-publishing plan: the loader converts only through `Print.Card`, so a new field needs no loader change. `NewManager(dir, CardSource)`, `Manager.CardDB()`, `Game.DB()`. `card_text` reads through the open game's snapshot.
- Knowledge `gotchas/new-set-tcgdex-data-incomplete.md`: the Legendary Stadium rule text ("You cannot play this card by itself. You must combine 2 different Legendary Pyroduct from your hand to play as 1 Stadium card."), and that neither source has it.

## Files examined

- `tcgreplay:internal/state/types.go:1-3,52-81,189-193` — state is rule-free plain data. Zones. Stack roles in `As`.
- `tcgreplay:internal/state/state.go:84,246` — `Stadium IID`. `Find`.
- `tcgreplay:internal/state/apply.go:96-130,132-260` — `remove`, `applyMove`, the occupied-spot refusal, knowledge bits per move.
- `tcgreplay:internal/state/events.go:93-104` — `MovePayload` (`IID, From, To, Face, As`).
- `tcgreplay:internal/state/invariants.go:12-95` — `Locations` and `Check` (each card exactly once).
- `tcgreplay:internal/state/view.go:14-24,60-75,185-188` — `CardView`. `BoardView.Stadium *CardView`.
- `tcgreplay:internal/game/verbs.go:428-457,725-781` — `PlayTrainer`, `Move`, `moveStack`.
- `tcgreplay:internal/game/game.go:203-248,339-444` — `tx`, `emit`, `move`, `actionSpec`, `runLocked` (warnings assembled at `:413-439`).
- `tcgreplay:internal/game/reads.go:34-123,426-530,610-648` — `describe`, `turnWarnings`, `benchWarnings`, `Undo`.
- `tcgreplay:internal/game/ui.go:41-146` — `UISnapshot` (`Warnings`, `Log`, `Frames`) and `touched`.
- `tcgreplay:internal/importer/handlers.go:92-106,494-511` — Stadium play and replacement. `lastStadium`. A Stadium discarded by an attack.
- `tcgreplay:internal/importer/replay.go:66,117,154-176,179-316` — binding, `move`/`emit` straight to state, `lastStadium`.
- `tcgreplay:internal/importer/importer.go:484-502` — handler order. Unrecognised lines become Problems.
- `tcgreplay:internal/importer/importer_test.go:61-139,343-371,645-697` — `TestCorpus`, `TestAttackDiscardsStadium`, the synthetic-line pattern.
- `tcgreplay:testdata/logs/*.txt`, `testdata/index.json` — the 11-log corpus and its oracle. No pair, no codes.
- `tcgreplay:internal/mcpserver/server.go:67-90,345-348,425-435,583-601,651-655,795-812` — instructions, `IIDArg`, `move`, `reveal`, `card_text`, `play_trainer`.
- `tcgreplay:internal/cards/cards.go:72-149` — `Card` and `Text()`.
- `tcgreplay:internal/cards/cardstest/cardstest.go:17-29` — FakeDB pool. Stadiums `tst-5` Arena and `tst-8` Deep Zone.
- `tcgreplay:internal/webui/actions.go:21-68,195-222,340-395` — action args, lock check, `play_trainer`, `move`.
- `tcgreplay:internal/webui/server.go:161-225` — `CardInfo` and `/api/card` (`trainer_type`, no `effect`).
- `tcgreplay:internal/webui/webtest/webtest.go:23-146`, `webtest_test.go:24-25,88-89` — board test seed. Stadium `p1-11`.
- `tcgreplay:web/src/components/Strip.svelte`, `web/src/lib/actions.js:40-50,91-114,185-200`, `web/src/components/Timeline.svelte:109-128`, `Zoom.svelte`, `web/src/lib/store.svelte.js:143-157` — Stadium spot, drops, one timeline line per event, zoom, card info.
- `tcgreplay:web/e2e/drag.spec.js:37-138`, `smoke.spec.js:45`, `phone.spec.js:101-131`, `rules.spec.js:66-69`, `fixtures.js` — table-driven drops, phone seeding, warnings and toasts.
- `tcgreplay:internal/guides/play-game.md`, `internal/guides/guides_test.go`, `internal/mcpserver/guides_test.go` — where the assistant's play instructions live and the rules on their text.
- `tcgreplay:TCG Review Playmat Design and Build Plan.md:34-60,109-140,146-186` — event schema and zones table, MCP tool contract, Playmat UI.
- `tcgreplay:site/src/content/docs/workflows/play-game.md` — the player-facing page on playing on the board.

## External references

- Pokémon TCG Delta Reign Legendary Stadium card text (confirmed by the user from the English card, recorded in knowledge `gotchas/new-set-tcgdex-data-incomplete.md`). It is the rule the pair mark and rule text encode.
- RFC 6901 JSON Pointer. The corrections path that sets `/playedAsPair`.

## Prior plans / specs consulted

- `20261108110847-d27a9f77-card-set-import` (plan, context): built format, `Print`, strict `Read` and correction round trip, `Print.Card`, the Delta Reign build task that records the pair rule text as a correction, and `cardsets/README.md`. This plan adds `playedAsPair` to `Print` and sets it by correction.
- `20261108105504-9abfa10e-card-set-publishing` (plan, context): loader converts via `Print.Card` with strict `cardset` decode. A set that fails to decode leaves the current snapshot. `game.NewManager(dir, CardSource)`, `Manager.CardDB()`, `Game.DB()`, `Manager.DB` removed. Publishing runs on merge to `main` through a workflow.
- Spec `20261108110847-055548e3-paired-stadiums`: requirements, acceptance criteria, the data-not-names constraint, the deferral rule for the import requirement.

## Open assumptions

- The card-set-import plan's correction apply lets a correction set a known `omitempty` field that is absent from the merged print (its context says the final pointer segment "may create an object member"). If it refuses, the implementer fixes that in `cardset` and records it; if that needs a change in direction, STOP and ask.
- The publishing plan's loader decodes built sets strictly, so a playmat built before `playedAsPair` cannot decode a set carrying it and keeps its previous copy of that set until it is updated. The built format stays `format: 1`, because the field is optional and additive. If the loader instead drops the whole catalogue on one bad set, STOP and ask.
- Delta Reign has exactly six Legendary Stadium prints: three names, two prints each. The implementer finds them by name in `cardsets/sets/me06.json` and does not hard-code numbers in program code.
- A real TCG Live log that plays a Stadium pair may or may not exist when the importer task is reached. If none exists, the implementer STOPs and asks the user to supply one or to confirm deferral, and never guesses the format.
- Both halves of a pair have the same owner in a legal game. The state and verbs still send each half to its own owner's pile.

## Drafting assumptions

### Chosen direction: pair-aware Stadium spot driven by a card-data mark (architecture)
- **Decision**: Option A. Add a `playedAsPair` mark to `cardset.Print` and `cards.Card`, set by corrections. Make the state's Stadium spot a list of at most two cards, and extend the move event with `pair` (the other half moves in the same event) and the role `as: "pair"` (join a lone half). Verbs (`PlayTrainer` with an optional pair, `Move`) decide from the mark. MCP `play_trainer` gets `pair_iid`. The board draws both halves. The importer recognises pairs from the log alone.
- **Key design decisions**: one event per pair move, so the log, frames and undo see one Stadium. The state stays rule-free, with the verb deciding and the event recording. The board view keeps `stadium` and nests the second half as `stadium.pair`. Completing a lone half joins the open play action. Lone-half and identical-print warnings are standing, built from state. The same-name warning is per action and applies to all Stadiums. `card_text` is unchanged: the pair rule comes only from the card's effect text in the data.
- **Rejected**: Option B, a second state field `StadiumPair` with two move events collapsed when the log is rendered (touches every log path, and the fold passes through a lone half mid-action). Option C, keeping one Stadium card and tracking the second half as an effect or marker in another zone (the board and every reader would have to infer the pair, and the invariant that each card is in one place would be bent).

### Pair rule text comes from data only (architecture)
- **Decision**: `cards.Card.Text()` is not changed and adds no line from the mark. The rule the assistant and the board zoom show is the card's own (corrected) effect text. The mark drives behaviour only (verbs, undo, board, `play_trainer` `pair_iid`, warnings). The committed-data test checks every Delta Reign Legendary Stadium print carries both the mark and the rule text, and the card-set guide tells maintainers a marked print must carry its printed rule.
- **Rationale**: Card text should match the printed card, and the data, not code, states what a card reads. A generated line would repeat the rule the Delta Reign corrections already add. Accepted by the user on 2026-11-08.
- **Rejected**: A rule line generated from the mark (shows the rule twice). Matching rule text in code to check the mark (code reading card text).

### Conventions selection (architecture)
- **Decision**: Apply `conventions/skills-text-and-packaging.md`.
- **Rationale**: The plan adds pair-play text to the `play-game` guide in `internal/guides`.
- **Rejected**: None dropped. It is the only convention.

### Same-name warning applies to every Stadium (architecture)
- **Decision**: The warning that a Stadium of the same name is already in play fires for any Stadium, not only pair-played ones.
- **Rationale**: It is the general rule of the game. Limiting it to pairs would need a special case, and it never blocks.
- **Rejected**: Pair-only warning. That is an arbitrary narrowing, and the spec lists it under pair warnings only because pairs are where it came up.

### Two halves of different names or without the mark still play (architecture)
- **Decision**: `play_trainer` with a `pair_iid` whose card has a different name, or is not marked pair-played, still places both cards with a warning. A `pair_iid` given with a non-Stadium is an error.
- **Rationale**: The playmat warns and never blocks on rules. Pairing a non-Stadium is a malformed request, which matches the Non-Goal that only Stadium pairs are supported.
- **Rejected**: Refusing mismatched pairs, which would make the playmat a rules engine.

### A completed pair joins the first half's open action (architecture)
- **Decision**: When the second half completes a lone half while the first half's play action is still open, it joins that action. Otherwise it is its own action.
- **Rationale**: Undo then removes the whole pair play in one step in the usual drag flow. Once the action has closed, the completion is a separate play, as the board shows it.
- **Rejected**: Always a separate action, which needs two undos for one pair. Merging closed actions, which would rewrite the log.

### Importer pair handling waits for a real log (discovery)
- **Decision**: The importer's pair handling is built only against a real TCG Live log that plays a pair. A person supplies the log. If none is available, the implementer stops and the user confirms the deferral, which is then recorded.
- **Rationale**: The spec makes the requirement conditional on a real log and names the format as a known risk. Today both plausible shapes import wrongly, one of them silently.
- **Rejected**: Guessing the line shape now. A wrong guess would hide a mis-import behind a passing test.

### Importer recognises pairs from the log, not card data (discovery)
- **Decision**: Whatever the real shape is, the importer recognises a pair from the log's own lines (for example, one line naming two cards, or a same-name play by the same player onto a lone half) and never from the card's pair mark.
- **Rationale**: Knowledge `architecture/importer-determinism.md` says card lookups only fill `Def` and never decide where a card goes. The spec's Technical Approach says the importer handles "the flag generally". The user settled this tension during planning (2026-11-08) by choosing log-text recognition (option A), so imports stay deterministic from the log, decklist and seed alone. Every other surface reads the mark.
- **Rejected**: Reading `playedAsPair` in the importer through the pinned snapshot, with the knowledge entry amended (option B, declined by the user).

### PlayTrainerWith next to PlayTrainer (tasks)
- **Decision**: Keep `PlayTrainer(seat, actor, iid)` and add `PlayTrainerWith(seat, actor, iid, pair)`, the way `EndTurn` and `EndTurnWith` work.
- **Rationale**: About twenty test call sites and two production callers use `PlayTrainer`. Only the MCP tool and the board action need the pair argument.
- **Rejected**: Changing the signature, which means mass churn in tests for no behaviour change.

### Board test seed keeps Arena as the default Stadium (tasks)
- **Decision**: Add the made-up Twin Peak prints to p1's hand in the board test seed, and leave Arena as the seeded Stadium.
- **Rationale**: Existing smoke, drag and phone tests rely on Arena in the spot. Pair tests drag or seed the pair themselves.
- **Rejected**: Seeding a pair by default, which would rewrite many existing e2e expectations.

### How the import deferral is recorded (tasks)
- **Decision**: With no real log, the implementer STOPs and asks. On the user's confirmation it records the deferral in the implementation changelog and test plan and proposes a follow-up spec. It makes no speculative importer change.
- **Rationale**: The spec says "deferred to a follow-up and recorded as deferred" and names no mechanism. The changelog and test plan are what the implement workflow produces.
- **Rejected**: A knowledge entry for the deferral (it is a work status, not durable knowledge), or leaving it unrecorded.

### Fan the pair on desktop, side by side on phone (tasks)
- **Decision**: Desktop and tablet show the second half slightly offset and fanned in a widened spot. Phone shows the two `sm` cards side by side.
- **Rationale**: Both halves stay visible and zoomable without a new component. The phone already uses small cards beside the Active.
- **Rejected**: A stacked view with a count badge, which hides the second half, and the spec asks for both halves visible.

## Rehydration cues

- `spektacular spec file read 20261108110847-055548e3-paired-stadiums`
- `spektacular plan file read 20261108110847-d27a9f77-card-set-import plan` (and `context`). Read §Data Structures and the Corrections and Built set format tasks.
- `spektacular plan file read 20261108105504-9abfa10e-card-set-publishing plan` (and `context`). Read §Data Structures (CardSource, CardDB) and the loader task.
- Re-read `internal/state/apply.go`, `internal/game/verbs.go:428-457,725-781`, `internal/game/reads.go:426-530`, `internal/importer/handlers.go:92-106,494-511`, `web/src/components/Strip.svelte`, `web/src/lib/actions.js:91-114`.
- `spektacular knowledge read` for `gotchas/new-set-tcgdex-data-incomplete.md`, `architecture/importer-determinism.md`, `architecture/games-read-cards-through-their-db.md`, `gotchas/playwright-touch-tests.md`, `conventions/skills-text-and-packaging.md`.
