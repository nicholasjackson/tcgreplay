---
created_date: "2026-11-08"
document_status: final
closed_date: "2026-11-08"
---

# Plan: 20261108110847-055548e3-paired-stadiums

<!-- Metadata -->
<!-- Created: 2026-11-08T12:04:41Z -->
<!-- Commit: 1b8fc21 -->
<!-- Branch: main -->
<!-- Repository: git@github.com:nicholasjackson/tcgreplay.git -->

## Overview

Delta Reign's Legendary Stadiums are two different prints of one name played together as one Stadium, and today the playmat cannot hold, play, move or import them that way. This plan adds a "played as a pair" mark to the project's own card data and makes the playmat honour it generally. The Stadium spot holds a pair as one Stadium. Playing, replacing, discarding and undoing move both halves together, with advisory warnings. The assistant reads the pair rule and plays both halves in one action, and the board shows both halves and completes a pair by dragging. Delta Reign's three Legendary Stadiums are marked through corrections, battle logs with a pair import once a real log is available, and the documentation describes pair play. Players and the assistant can then play these cards in real games with no workarounds, and any future pair-played card needs only data.

## Conventions

- **Guide text and packaging (`conventions/skills-text-and-packaging.md`): never write "Claude" or "Nic " in an `internal/guides` guide, and never exclude `internal/guides` from a build context** — this feature adds pair-play instructions to the `play-game` guide, so the new text must avoid both words. `internal/guides/guides_test.go` enforces this.

## Architecture & Design Decisions

All work lands in the single registered repo, `tcgreplay` (`/home/nicj/code/github.com/nicholasjackson/tcgreplay`), across five layers: card data, game state, game verbs, the assistant's tools and the board, plus the importer and the docs. **Card data says which cards play as a pair.** The card-set-import plan's built format gains one optional, language-independent field on each print, `playedAsPair` (`cardset.Print.PlayedAsPair`, JSON `"playedAsPair": true`), and `Print.Card` copies it into a new `cards.Card.PlayedAsPair`. Because the publishing plan's loader converts prints only through `Print.Card`, the mark reaches every playmat with no loader change. Delta Reign's six Legendary Stadium prints (Pyroduct, Summit and Trench, two prints each) get the mark through the import plan's corrections file (`{"card": …, "path": "/playedAsPair", "value": true, "reason": …}`), beside the rule-text correction that plan already records. No program code names a card: removing the correction, or the field, makes the card an ordinary Stadium again (spec Constraint, "data in the bucket, logic and presentation in the binary").

**The Stadium spot holds up to two cards, and the state stays free of rules.** `state.State.Stadium` becomes `[]IID`, holding at most two cards, first half first. The move event gains two additive, optional parts. `pair` names the other half, which moves with the card in the same event, so the pair is played, discarded or replaced in one event and the fold never passes through a lone-half state mid-action. The role `as: "pair"` lets a single card join a lone card already in the spot, which is how a pair is completed by dragging. The state accepts a second card in the spot only by one of these two routes, and still refuses a plain move into an occupied spot. It never reads card text (`internal/state/types.go:1-3`): the verb, which can read the card, decides and says so in the event. Old records hold neither part and fold exactly as before. The board view keeps `stadium` as one object and adds the second half inside it (`stadium.pair`), so every consumer sees one Stadium made of two cards, and existing readers of `stadium.iid` and `stadium.name` keep working. The game log, frames and undo all work per event and per action, so one event per pair move gives one log line and one undo step with no change to the log or undo code.

**Verbs apply the mark generally.** `Game.PlayTrainer` gains a form that takes an optional second card (`PlayTrainerWith`, after the existing `EndTurn`/`EndTurnWith`). Given two halves, it plays them together in one event. Given one half of a pair-played Stadium while the spot holds a lone half of the same name and owner, it completes the pair (`as: "pair"`), joining the first half's play action when that is still open, so the whole pair play undoes as one. Otherwise it replaces whatever is in the spot, sending a pair to its owner's discard in one event. `Game.Move` (the escape hatch the board uses for drops that are not plays, and for discarding the Stadium) moves both halves together whenever either half leaves the spot, the way `moveStack` moves a Pokémon with its attachments (`internal/game/verbs.go:757-781`). It also completes a lone half on the same terms as `PlayTrainer`. Warnings never block (the playmat is not a rules engine, spec Non-Goals). A lone half of a pair-played card, and a pair made of two identical prints, are standing warnings built from the state beside `benchWarnings`. They show on the board header, in `get_warnings` and after every action for as long as the spot holds them, and disappear once the pair is complete or replaced. A Stadium played while one of the same name is in play is a one-off warning from the verb, applied to every Stadium because it is the general rule. Completing a pair is not counted as a second Stadium played that turn.

**Every surface reuses that one path.** MCP `play_trainer` gains an optional `pair_iid`, and the board's `play_trainer` action accepts the same. Dropping the second half from the hand onto a lone half already calls `play_trainer` (`web/src/lib/actions.js:105-110`), so completing a pair by dragging needs no new front-end action. The board's Stadium spot (`web/src/components/Strip.svelte`) draws both halves, slightly fanned, at desktop and phone sizes, each zoomable on its own. `cards.Card.Text()`, which `card_text` returns and the board zoom shows, is unchanged and adds nothing from the mark: the pair rule they show is the card's own printed rule in its effect text in the data (recorded by correction where the sources lack it), so card text matches the printed card and the data, not code, states what a card reads. The card-set guide tells maintainers that a print marked as played as a pair must also carry its printed pair rule. The server instructions and the `play-game` guide explain `pair_iid`. The TCG Live importer recognises a pair from the log's own lines and never from the mark, because card lookups must not decide where a card goes (knowledge `architecture/importer-determinism.md`). Its exact line shape is built against a real battle log that a person supplies. If none exists when the work is built, the importer requirement is deferred and recorded, as the spec allows. Rejected options are in `research.md#alternatives-considered-and-rejected`: a second Stadium field, the state reading card data, two events collapsed in rendering, a new event type, importer decisions from card data, guessing the log format, a new board action, a rule line generated from the mark, and a toast-only lone-half warning.

## Component Breakdown

- **Built set format and runtime card (changed: `cardset` print and `cards.Card`).** Each print's language-independent facts gain the pair mark, and the runtime card definition carries it. The conversion from a print to a runtime card copies it. The card's plain-text rendering, which the assistant reads, is unchanged: a pair-played card's rule is part of its effect text in the data. Corrections can set the mark like any other field. The publishing loader is unchanged, because it converts prints only through this conversion.
- **Delta Reign corrections (changed: committed card data).** The Delta Reign corrections file marks each Legendary Stadium print as pair-played, with a reason, beside the existing rule-text corrections. The rebuilt Delta Reign set carries the mark, and publishing on merge takes it to every playmat.
- **Game state: the Stadium spot (changed: `state`).** The spot holds up to two cards as one Stadium. The move event can carry a second card that moves with the first, and a role that lets one card join a lone card already in the spot. Locating, removing, invariant checks and the board projection all treat the spot as a list of up to two cards. The projection shows one Stadium with its second half nested inside it. The state still reads no card data: it accepts a pair only when the event says so.
- **Game verbs and warnings (changed: `game`).** Playing a Trainer accepts an optional second half and decides, from the cards' pair mark, name and owner, whether to play a pair together, complete a lone half, or replace what is in the spot. Replacing a pair sends both halves to the discard as one. The escape-hatch move takes both halves when either leaves the spot. Standing warnings flag a lone half of a pair-played Stadium and a pair of identical prints. Each play warns when a Stadium of the same name is already in play, and completing a pair does not count as a second Stadium that turn. The log, frames and undo need no change of their own, because each pair move is one event inside one action.
- **Assistant tools and instructions (changed: MCP server and the `play-game` guide).** The play-Trainer tool takes an optional second half. Card text includes the pair rule because the card's data carries it, with no change to `card_text`. The server instructions and the guide the assistant follows when playing explain how to play a pair and how to complete one.
- **Board (changed: board server actions and the web board).** The board's play-Trainer action passes an optional second half through to the same verb. The Stadium spot draws both halves as one Stadium, each zoomable, at desktop and phone sizes. Dropping the second half from the hand onto a lone half completes the pair through the existing play-Trainer drop. The board test seed and the browser tests cover a pair.
- **TCG Live importer (changed: `importer`).** Replays a Stadium pair, and the replacement of a pair, from the battle log's own lines into the Stadium spot as one event, and recognises both halves' discard lines when a pair is replaced. It never reads the pair mark. Its line shape comes from a real battle log, added to the test corpus. Until one is available this component changes only as far as the new Stadium spot requires.
- **Real pair battle log (new input, from a person).** A TCG Live battle log in which a player plays a Legendary Stadium pair, supplied by the user. It is what the importer change is built and tested against, or its absence is recorded as the deferral.
- **Documentation (changed).** The design-and-build notes (event schema, MCP tool contract, playmat UI), the player-facing page on playing a game on the board, and the card-set guide (the pair mark and its correction, and that a marked print must also carry its printed pair rule in its effect text) describe pair-played Stadiums.

## Data Structures & Interfaces

**Pair mark in card data** (changed: `cardset.Print`, the card-set-import plan's built format, and `cards.Card`). The mark is one optional, language-independent fact per print. It is absent (false) for every card that is not played as a pair, so existing built sets do not change by a byte. The format stays `format: 1`, because the field is optional and additive.

```go
// cardset.Print: language-independent core (excerpt)
PlayedAsPair bool `json:"playedAsPair,omitempty"` // two different prints of this name are played together as one card

// cards.Card (excerpt)
PlayedAsPair bool `json:"playedAsPair,omitempty"` // copied by Print.Card
```

Set by correction (`cardsets/corrections/me06.json`), one entry per Legendary Stadium print:

```json
{"card": "me06-096", "path": "/playedAsPair", "value": true, "reason": "Legendary Stadium: printed rule says two different prints are played together as 1 Stadium"}
```

**Card text** (unchanged: `cards.Card.Text()`, returned by `card_text` and shown in the board's zoom). The mark adds no line. The pair rule a card shows is its own printed rule in its effect text in the data, for Delta Reign the rule-text corrections the card-set-import work records (for example "You cannot play this card by itself. You must combine 2 different Legendary Pyroduct from your hand to play as 1 Stadium card."). A print marked as played as a pair must carry that rule in its effect text, recorded by correction if the sources lack it. The committed-data test checks this for Delta Reign, and no code matches text.

**Stadium spot** (changed: `state.State`). The spot is a list of at most two cards, first half first. The state is not persisted (records hold events), so the type change touches no stored data.

```go
Stadium []IID `json:"stadium,omitempty"` // was IID; at most 2
```

**Move event** (changed: `state.MovePayload`). Two additive, optional parts. A record without them folds exactly as before.

```go
type MovePayload struct {
    IID  IID
    From *Loc
    To   Loc
    Face string
    As   string // pokemon | energy | tool | pair (new: join the lone card in the Stadium spot)
    Pair IID    `json:"pair,omitempty"` // new: the other half, moved with IID in this event to the same zone, each to its owner's side
}
const AsPair = "pair"
```

The rules the fold enforces are these. A move into an empty spot places one card, or two when `pair` is set. A move with `as: "pair"` into a spot holding exactly one card makes it two. Any other move into an occupied spot is refused, as today. A `pair` card must exist and be in the same zone as `IID`, or in the Stadium spot with it.

**Board view** (changed: `state.BoardView.Stadium`). The view is still one object, so existing readers of `stadium.iid`, `stadium.name` and `stadium.def` keep working. The second half is nested in it.

```go
type StadiumView struct {
    CardView                      // the first half, as today
    Pair *CardView `json:"pair,omitempty"` // the second half of a pair
}
Stadium *StadiumView `json:"stadium,omitempty"`
```

```json
"stadium": {"iid": "p1-11", "name": "Legendary Summit", "def": "me06-096",
            "pair": {"iid": "p1-12", "name": "Legendary Summit", "def": "me06-097"}}
```

**Game verb** (changed: `game.Game`). This follows the existing `EndTurn`/`EndTurnWith` pattern: `PlayTrainer` keeps its signature, so its many callers and tests are untouched, and the MCP server and the board's actions call the new form with the optional second half.

```go
func (g *Game) PlayTrainer(seat state.Seat, actor state.Actor, iid state.IID) (*Result, error) // = PlayTrainerWith(…, iid, "")
func (g *Game) PlayTrainerWith(seat state.Seat, actor state.Actor, iid, pair state.IID) (*Result, error)
```

`pair == ""` keeps today's behaviour, plus completion of a lone half. `pair` set on a non-Stadium is an error.

**Assistant tool** (changed: MCP `play_trainer` input):

```go
type PlayTrainerArg struct {
    SeatArg
    IID  string `json:"iid"`
    Pair string `json:"pair_iid,omitempty" jsonschema:"For a Stadium played as a pair: the second, different print to play together with iid"`
}
```

**Board action** (changed: `webui` action args). `play_trainer` accepts the same `pair_iid`, and the action lock check covers it:

```go
Pair string `json:"pair_iid,omitempty"`
```

**Log line for a pair move** (changed: `game.describe`). A pair is one event and one line:

```
p1 moved p1-11 Legendary Summit + p1-12 Legendary Summit p1:hand → stadium
p1 moved p1-12 Legendary Summit p1:hand → stadium as pair
```

**Warnings** (new text, advisory only):
- standing, while the spot holds it: `Legendary Summit (p1-11) is played as a pair: play a second, different Legendary Summit with it`
- standing: `the Stadium pair Legendary Summit is two copies of the same print (me06-096): a pair needs two different prints`
- per play: `a Stadium named Legendary Summit is already in play`
- per play, when the two halves are not two prints of one pair-played Stadium: `Arena and Deep Zone are not two halves of one pair-played Stadium`

No other new types. The importer's internal replay state changes (the replaced Stadium becomes a list), but that is private to the package.

## Implementation Detail

**A data mark read by general code.** The feature introduces the first card behaviour the playmat takes from a flag in card data rather than from parsing card text or recognising a name. A developer looking for "which cards are paired" finds the answer only in the built sets and their corrections. The code holds the general behaviour ("a card marked this way is played as two different prints together") and nothing else. Tests use made-up pair-played Stadiums in the in-memory card pool, so the rule is proven independent of Delta Reign. A test over the committed data checks that Delta Reign's real Legendary Stadiums carry the mark and the rule text.

**The verb decides, the event records, the state obeys.** This follows the existing split. The state package folds plain events and knows nothing of cards. Verbs in the game package read card definitions and emit events. The pair is expressed the same way stack roles already are: the event says what the card is doing (`as: "pair"`, or a `pair` card moving with it), and the fold checks only that the board allows it. Replaying a game therefore never needs card data to place a Stadium, and a game imported or rebuilt from its record lands the pair exactly where it was.

**One event per pair move.** Moving a pair is one event rather than two moves. Today one event moves one card, except a swap, which moves two stacks. A pair move is the second exception. It is chosen so that every per-event consumer (the timeline, frames and their highlight, deltas, the assistant's log, undo by action, rewind to an event) sees the pair as one Stadium without changes of their own, and so that no intermediate board ever shows a lone half or an occupied-spot error in the middle of a replacement. Reviewers should check every place that renders, highlights or redacts a move event and confirm that it names both halves.

**Escape hatch moves the whole thing.** The general "move any card anywhere" verb already moves a Pokémon in play with its attachments when its top card moves. A Stadium pair follows the same idea: moving either half out of the spot takes the other with it. So discarding a pair from the board, returning it to the hand, or any hand correction keeps the pair together in one action, and undo returns both.

**Standing warnings built from state.** The lone-half and identical-print warnings sit beside the existing bench-size warning. They are recomputed from the current board on every action and snapshot, rather than raised once by a verb. That is how they appear while the condition holds and vanish once it is fixed, which is the board behaviour the spec asks for. The same-name warning is a one-off from the play, because the condition it describes is gone as soon as the play replaces the old Stadium.

**Surfaces stay thin.** The MCP tool, the board action and the board's drop handler all funnel into the one play-Trainer verb, so pair rules are written once. The front end gains rendering only (two cards in the spot), and no new decision logic: completing a pair by dragging is the same drop it already sends. The importer is the only second writer of Stadium events, because it replays logs straight into the state. It keeps its own log-driven logic and recognises pairs from the log text, in line with the rule that card lookups never decide where an imported card goes.

**Card-set data follows the import plan's workflow.** The mark is added to the format as an ordinary optional field. The Delta Reign set is rebuilt by re-running the import with the new correction, never by hand-editing the built file. The committed-data guard then protects it like every other field.

## Dependencies

- **Design documents this plan was built on: none.** The spec carries no design references.
- **Spec `20261108110847-d27a9f77-card-set-import` and its plan (must land first).** It provides the built format (`cardset.Print`, strict `Read`, deterministic `Write`, `format: 1`), `Print.Card(lang)`, corrections by JSON Pointer applied through a strict round trip, the import report, `sets check` with the committed-data guard, `cardsets/README.md`, and the verified Delta Reign set whose corrections already add the Legendary Stadiums' rule text. This plan changes one thing in it: `Print` gains the optional `playedAsPair` field, and `Print.Card` copies it. The pair-mark task depends on that plan's "Built set format" task. The Delta Reign task depends on its "Build and verify Delta Reign" task.
- **Spec `20261108105504-9abfa10e-card-set-publishing` and its plan (must land first).** It provides the catalogue loader (conversion only through `Print.Card`, so no loader change), `game.NewManager(dir, CardSource)`, `Manager.CardDB()` and `Game.DB()` (`Manager.DB` is removed, so `card_text` reads through the open game's snapshot), and the publish-on-merge workflow that takes the rebuilt Delta Reign set to the hosted playmat. It needs no change from this plan. Every task here assumes the switch to published sets has landed, because the hosted check of Delta Reign's Legendary Stadiums is only possible after it.
- **`state` package (internal, changed).** The Stadium spot becomes a list of up to two cards, and the move event gains `pair` and the `pair` role. Additive, so old records fold unchanged.
- **`game` package (internal, changed).** `PlayTrainer` takes an optional second half, `Move` keeps a pair together, `describe` and `touched` name both halves, and new warnings are added. The action, log and undo machinery are used as they are.
- **`cards` package (internal, changed).** `Card` gains `PlayedAsPair`, and `Text()` is unchanged. `cardstest.FakeDB` gains pair-played Stadium fixtures.
- **`mcpserver` package (internal, changed).** `play_trainer` gains `pair_iid`, and the server instructions gain one line about pairs.
- **`internal/guides` (internal, changed).** The `play-game` guide explains pair play, subject to the guide-text convention and the guide tests (`pair_iid` must be a real schema property).
- **`webui` package and the web board (`web/`, changed).** The `play_trainer` action passes `pair_iid`, the Stadium spot renders two halves, and the board test seed and Playwright tests gain a pair. No new front-end dependency.
- **`importer` package (internal, changed).** It changes for the new Stadium spot, and for pair lines once a real log exists.
- **A real TCG Live battle log that plays a Legendary Stadium pair (information the user holds).** The user has been asked for one. The importer requirement is built against it, or deferred and recorded if none exists when the work is built. Supplying it is a task for a person.
- **Go standard library and existing test tooling (Go tests, Playwright with Chromium).** No new modules.
- **Knowledge entries relied on:** `gotchas/new-set-tcgdex-data-incomplete.md` (the Legendary Stadium rule text and the one-card spot), `architecture/importer-determinism.md` (the importer must not decide placement from card data), `architecture/games-read-cards-through-their-db.md` (card facts come through the game's database, which the verbs use to read the mark), `gotchas/playwright-touch-tests.md` (phone layout, touch drags through CDP), `gotchas/tcg-live-log-card-codes.md` (two prints of one name in a log line, and how codes bind), and `conventions/skills-text-and-packaging.md` (guide text).

## Testing Approach

Tests are standard Go tests beside the code they change, plus the board's existing Playwright suite (desktop, tablet and phone projects, Chromium only, per knowledge `gotchas/playwright-touch-tests.md`). All run as part of the project's usual test suite. Game, MCP and board tests use the in-memory card pool, which gains two pair-played test Stadiums: two prints of one made-up name, both with the mark. Two copies of one print cover the identical-print case. Nothing in the program or the game tests depends on Delta Reign's names.

**Most coverage: the state fold and the play verb**, because every surface goes through them.

- **State.** A pair placed in one event occupies the spot as two cards, and each card is still in exactly one place (the invariant check passes). Joining with the `pair` role works only when the spot holds exactly one card. A plain move into an occupied spot is still refused. A pair moved out in one event lands each half on its owner's side. The projection shows one Stadium with the second half nested. Records without the new parts fold exactly as before, which the existing corpus and record round-trip tests already check.
- **Pair rule comes from data (acceptance criterion).** With the mark on the test Stadium, two prints play as a pair and the lone-half warning shows for one. With the mark removed from the same card in a fresh pool, the same requests behave as ordinary Stadiums (the second replaces the first), with no code change.
- **Play.** Two different prints given together occupy the spot as one Stadium in one event and one action. A second half played onto a lone half completes the pair, joining the first half's still-open action. Undo after either returns both halves to the hand in one step.
- **Replace and discard as one.** Playing a different Stadium over a pair sends both halves to their owner's discard. Undo returns both to the spot. The log shows one line for the pair's move. Moving either half of a pair from the spot to the discard moves both in one action, and undo returns both.
- **Warnings, never blocking.** Each of the three spec cases (one half alone, two identical prints as a pair, a pair played while a pair of the same name is in play) produces its warning, and the action is still carried out. The lone-half and identical-print warnings appear in the board snapshot's standing warnings while the condition holds, and are gone once the pair is complete or replaced. Completing a pair does not raise the "played 2 Stadiums this turn" warning.

**Assistant surface.** An unmarked card's text is unchanged, and a marked card's text is just its data text: the mark adds no line, so its pair rule is the one in its effect text. `play_trainer` with `pair_iid` plays both halves in one action through the MCP round trip. The existing guide tests keep the new guide text within the rules: real tool and argument names only, and none of the forbidden words.

**Board, end to end.** In the board test harness, a seeded pair shows both halves in the Stadium spot at desktop and phone widths. Dragging the first half from the hand onto the spot shows the lone-half warning, and dragging the second half onto it completes the pair, after which the warning is gone and both halves show. This follows the table-driven drop tests and the phone-only seeding pattern already in the suite.

**Card data.** A unit test shows a print with the mark converts into a runtime card with it, and that a correction setting `/playedAsPair` lands in the built print. A test over the committed Delta Reign data asserts that every Legendary Stadium print is marked and that its English text carries the rule. It is a test over data, so naming the cards there does not break the constraint, which binds program code.

**Importer.** Once a real log exists, it joins the test corpus and its oracle. The corpus test then guarantees it imports with no problems and leaves the pair in the Stadium spot (or replaced as one, if the log replaces it). Corpus imports across seeds keep the importer deterministic. Without a real log, no speculative importer test is written, and the deferral is recorded instead.

**Deliberate gaps.** The hosted playmat's view of the real Delta Reign data and a full game played with a pair are manual checks below. The importer's handling of a pair line has no automated test until a real log exists.

**Success metrics.**

- *Delta Reign's Legendary Stadiums can be played as pairs, both by hand on the board and by the assistant, in a real game, with no hand workarounds.* **Manual — captured in the implementation test plan.** The automated proxies are the board drag tests, the MCP `play_trainer` pair test and the committed-data test on Delta Reign's marks.

**Manual reviews.**

- **Manual — captured in the implementation test plan:** on the hosted playmat after the rebuilt Delta Reign set is published, Legendary Pyroduct, Summit and Trench each read with the pair rule in card text and the board zoom, and each plays as a pair.
- **Manual — captured in the implementation test plan:** importing a real TCG Live battle log in which a player plays a Legendary Stadium pair succeeds and shows the pair in the Stadium spot, or the deferral is recorded if no such log was available.
- **Manual — captured in the implementation test plan:** a look at the Stadium pair on a real phone and on desktop, checking both halves are visible, zoomable and not clipped, beyond what the emulated Chromium phone shows.
- **Manual — captured in the implementation test plan:** review the documentation by following it to play a pair on the board and to have the assistant play one.
- **Manual — captured in the implementation test plan:** ChatGPT players must refresh the app's tools before checking `play_trainer`'s new `pair_iid`, because ChatGPT caches tool descriptions (knowledge `gotchas/chatgpt-tool-discovery.md`).

## Milestones & Tasks

### Milestone 1: The playmat plays pair-played Stadiums

**What changes**: Card data can mark a Stadium as played as a pair, and the playmat honours the mark for any card that carries it. The Stadium spot can hold two different prints of such a Stadium as one Stadium. Playing a different Stadium over a pair, or discarding the pair, moves both halves together, and one undo puts both back. The log shows the pair's move as one line. The playmat warns, without stopping the play, when one half is played alone, when two identical prints are paired, and when a Stadium of the same name is already in play. Nothing visible changes for any real card yet, because no card carries the mark.

**Validation point**: The state and verb tests pass with made-up pair-played Stadiums: a pair is played, completed, replaced, discarded and undone as one, and each warning shows without blocking. Removing the mark from the test card makes it an ordinary Stadium. The existing corpus, record and board tests still pass.

#### - [ ] Task: Pair mark in card data
**Id:** ba448b36-faa3-4065-b50d-62412c2bec0f
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Adds an optional "played as a pair" mark to each print in the built card-set format and to the playmat's runtime card, and carries it from one to the other. The mark adds nothing to a card's plain text: a pair-played card states its rule in its own effect text in the data, which the assistant reads and the board's zoom shows. The test card pool gains made-up pair-played Stadiums for the later tasks. This needs the card-set-import plan's built format to have landed.

*Technical detail:* [context.md#task-pair-mark-in-card-data](./context.md#task-pair-mark-in-card-data)

**Acceptance criteria**:
- [ ] A print marked as played as a pair becomes a runtime card with the mark, and an unmarked print becomes one without it.
- [ ] A correction can set the mark on a print, and the import keeps it on every re-import.
- [ ] A marked card's text is its data text alone, with no line generated from the mark, and an unmarked card's text is unchanged.
- [ ] Existing built sets are unchanged, byte for byte, by the new field.

#### - [ ] Task: Stadium spot holds a pair
**Id:** 4053fd2f-5557-402a-82f2-637258dc41d2
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Lets the game state's Stadium spot hold two cards as one Stadium. A move can carry the other half with it, so a pair is played, discarded or replaced in a single step, and a single card can be marked as joining a lone card already in the spot. The board view shows one Stadium with its second half inside it, and the log describes a pair's move in one line. The state still reads no card data, and games recorded before this change replay exactly as before.

*Technical detail:* [context.md#task-stadium-spot-holds-a-pair](./context.md#task-stadium-spot-holds-a-pair)

**Acceptance criteria**:
- [ ] Two cards moved into an empty Stadium spot together occupy it as one Stadium, and every card is still in exactly one place.
- [ ] A card can join a spot holding exactly one card as its pair, and a plain move into an occupied spot is still refused.
- [ ] A pair moved out of the spot in one step puts each half on its owner's side.
- [ ] The board view shows the Stadium with its second half nested in it, and existing readers of the Stadium still see the first half.
- [ ] Every existing battle log, game record and saved game still replays to the same board.

#### - [ ] Task: Play, replace and discard Stadium pairs
**Id:** dcf69aa9-ae33-4b60-b152-19246998167e
**Repo:** tcgreplay
**Depends on:**
- ba448b36-faa3-4065-b50d-62412c2bec0f — Pair mark in card data
- 4053fd2f-5557-402a-82f2-637258dc41d2 — Stadium spot holds a pair
**Execution:** agent

Teaches the playmat's play and move actions the pair rule, read from the card's mark. Two halves can be played together. A second half played onto a lone half completes the pair, as part of the first half's play while that is still open. A Stadium played over a pair sends both halves to the discard, and moving either half out of the spot takes both. Advisory warnings flag a lone half, a pair of identical prints, and a Stadium played while one of the same name is in play, and none of them stops the action.

*Technical detail:* [context.md#task-play,-replace-and-discard-stadium-pairs](./context.md#task-play,-replace-and-discard-stadium-pairs)

**Acceptance criteria**:
- [ ] Playing two different prints of a pair-played Stadium together puts both in the Stadium spot as one Stadium, in one action.
- [ ] Playing one half alone, and then the other, completes the pair, and one undo removes both when the second joined the first's play.
- [ ] Playing a different Stadium over a pair puts both halves in their owner's discard pile. Undoing it returns both to the Stadium spot, and the log shows one line for the pair's move.
- [ ] Discarding a pair from the Stadium spot moves both halves in one logged action, and undoing it returns both.
- [ ] A lone half, a pair of two identical prints, and a Stadium played while one of the same name is in play each show a warning while the action is still carried out. The lone-half and identical-print warnings last only while the spot holds them.
- [ ] Removing the mark from the test Stadium makes it behave as an ordinary Stadium, with no program change.
- [ ] Completing a pair is not reported as playing a second Stadium that turn.

### Milestone 2: Players and the assistant can play a pair

**What changes**: The assistant reads the pair rule in a pair-played card's text, can play both halves in one play action, and its playing instructions explain how. On the board, the Stadium spot shows a pair as one Stadium with both halves visible on desktop and phone. A player plays a pair by dragging the first half onto the spot, where a warning says it needs its partner, and then dragging the second half onto it, which completes the pair and clears the warning.

**Validation point**: The MCP round-trip test plays a pair with `play_trainer`. The board tests show both halves in the spot at desktop and phone widths and complete a pair by dragging, with the lone-half warning shown and then gone.

#### - [ ] Task: Assistant plays Stadium pairs
**Id:** 8bcd43bb-ce27-43e9-8a8a-5a0c35885e50
**Repo:** tcgreplay
**Depends on:**
- dcf69aa9-ae33-4b60-b152-19246998167e — Play, replace and discard Stadium pairs
**Execution:** agent

Gives the assistant's play-Trainer tool an optional second card, so the assistant plays both halves of a pair in one play action. The card text the assistant reads includes the pair rule, from the card's own effect text. The server instructions and the guide the assistant follows when playing explain how to play a pair, and how to complete one when a lone half is already down.

*Technical detail:* [context.md#task-assistant-plays-stadium-pairs](./context.md#task-assistant-plays-stadium-pairs)

**Acceptance criteria**:
- [ ] The assistant can play both halves of a pair-played Stadium in one play action.
- [ ] The card text the assistant reads for a pair-played Stadium includes the pair rule, taken from the card's data.
- [ ] The assistant's playing instructions explain how to play a pair.
- [ ] Playing a single Trainer with the tool works exactly as before.

#### - [ ] Task: Board shows and plays Stadium pairs
**Id:** db242fa7-8da4-45b4-854d-2be909c1ecab
**Repo:** tcgreplay
**Depends on:**
- dcf69aa9-ae33-4b60-b152-19246998167e — Play, replace and discard Stadium pairs
**Execution:** agent

Shows a Stadium pair on the board as one Stadium with both halves visible, at desktop and phone sizes, each half zoomable on its own. A player completes a pair by dragging the second half from the hand onto a lone first half, through the drop the board already uses for playing a Stadium. The board's play action also accepts a second card for parity with the assistant. The board test setup and browser tests gain a pair.

*Technical detail:* [context.md#task-board-shows-and-plays-stadium-pairs](./context.md#task-board-shows-and-plays-stadium-pairs)

**Acceptance criteria**:
- [ ] On desktop and phone widths, the board shows both halves of a Stadium pair in the Stadium spot, and either can be opened in the zoom.
- [ ] Dragging the second half onto a lone first half completes the pair.
- [ ] The single-half warning shows while the first half is alone and is gone once the pair is complete.
- [ ] A single Stadium still shows and plays as before, including replacing one Stadium with another by dragging.

### Milestone 3: Delta Reign's Legendary Stadiums play as pairs everywhere

**What changes**: Delta Reign's Legendary Pyroduct, Summit and Trench carry the pair mark and the pair rule in their published data, so on the hosted and desktop playmats they read with the rule and play as pairs. TCG Live battle logs in which a player plays one of these pairs import with the pair in the Stadium spot, provided a real log was available when the work was built; otherwise that is recorded as deferred to a follow-up. The project documentation describes playing a pair on the board and by the assistant.

**Validation point**: The committed-data test shows every Delta Reign Legendary Stadium print marked and carrying the rule text, and the set check passes. A real pair log imports cleanly as part of the corpus, or its deferral is recorded. The documentation covers pair play. On the hosted playmat, the three Legendary Stadiums read with the rule and play as pairs.

#### - [ ] Task: Mark Delta Reign's Legendary Stadiums
**Id:** ef13bcef-35f2-46e9-955c-d4333ef954e9
**Repo:** tcgreplay
**Depends on:**
- ba448b36-faa3-4065-b50d-62412c2bec0f — Pair mark in card data
**Execution:** agent

Records in Delta Reign's corrections that each print of Legendary Pyroduct, Summit and Trench is played as a pair, with a reason, beside the rule-text corrections the card-set-import work already made. It then rebuilds the set through the import, never by hand-editing the built file. A test over the committed data checks every one of these prints carries the mark and the rule text. Publishing on merge then takes the change to every playmat. This needs the card-set-import plan's verified Delta Reign set to have landed.

*Technical detail:* [context.md#task-mark-delta-reign's-legendary-stadiums](./context.md#task-mark-delta-reign's-legendary-stadiums)

**Acceptance criteria**:
- [ ] Every Delta Reign print of Legendary Pyroduct, Summit and Trench is marked as played as a pair in the built set, and its English text includes the pair rule.
- [ ] The marks come from recorded corrections with reasons, and survive a re-import.
- [ ] Delta Reign's report has nothing open, and the set check passes over every committed set.
- [ ] No other Delta Reign card changes.

#### - [ ] Task: Supply a real Stadium pair battle log
**Id:** 1ac769ec-378c-4977-86d6-b4dfbf31b8bc
**Repo:** tcgreplay
**Depends on:** none
**Execution:** human — needs a TCG Live battle log from a real game, which only a player can export from TCG Live

The user exports a TCG Live battle log from a game in which a player plays a Legendary Stadium pair (ideally one where a pair is also replaced or discarded), with the matching decklist, and hands it over for the test corpus. The importer task is built against it. If no such game has been played by the time the importer task is reached, the user says so and the importer requirement is deferred.

*Technical detail:* [context.md#task-supply-a-real-stadium-pair-battle-log](./context.md#task-supply-a-real-stadium-pair-battle-log)

**Acceptance criteria**:
- [ ] A real TCG Live battle log that plays a Legendary Stadium pair, and its decklist, are available to the implementer, or the user has confirmed none is available yet.

#### - [ ] Task: Import battle logs with a Stadium pair
**Id:** 34004346-c1aa-404f-92d0-7875ce7f8773
**Repo:** tcgreplay
**Depends on:**
- 4053fd2f-5557-402a-82f2-637258dc41d2 — Stadium spot holds a pair
- 1ac769ec-378c-4977-86d6-b4dfbf31b8bc — Supply a real Stadium pair battle log
**Execution:** agent

Makes the TCG Live log importer replay a Stadium pair into the Stadium spot as one Stadium, and a replaced pair into its owner's discard, recognising both from the log's own lines rather than from card data. The real log joins the test corpus. If no real log was available, nothing is guessed: the deferral is recorded for a follow-up instead.

*Technical detail:* [context.md#task-import-battle-logs-with-a-stadium-pair](./context.md#task-import-battle-logs-with-a-stadium-pair)

**Acceptance criteria**:
- [ ] Importing the real battle log succeeds with no problems reported, and the board then shows the pair in the Stadium spot (or both halves in the discard where the log replaces it).
- [ ] Every existing battle log in the test corpus imports exactly as before, for every seed.
- [ ] If no real log was available, the requirement is recorded as deferred to a follow-up, and the importer still imports every existing log.

#### - [ ] Task: Pair play documentation
**Id:** e53f2613-04cd-468b-96c9-5169e90a2592
**Repo:** tcgreplay
**Depends on:**
- 8bcd43bb-ce27-43e9-8a8a-5a0c35885e50 — Assistant plays Stadium pairs
- db242fa7-8da4-45b4-854d-2be909c1ecab — Board shows and plays Stadium pairs
- ef13bcef-35f2-46e9-955c-d4333ef954e9 — Mark Delta Reign's Legendary Stadiums
**Execution:** agent

Documents pair-played Stadiums: for players, how to play and complete a pair on the board and what the warnings mean; for developers, the pair mark, the Stadium spot's two cards, the move event's pair parts and the assistant tool's new argument; for maintainers, how a card is marked as pair-played by correction, and that a marked print must also carry its printed pair rule in its effect text.

*Technical detail:* [context.md#task-pair-play-documentation](./context.md#task-pair-play-documentation)

**Acceptance criteria**:
- [ ] The player-facing documentation describes playing a pair-played Stadium on the board, including completing a lone half.
- [ ] The design documentation describes how the assistant plays a pair and how the game records it.
- [ ] The card-set guide describes marking a card as played as a pair, with an example correction, and says a marked print must also carry its printed pair rule in its effect text, recorded by correction if the sources lack it.

## Open Questions

- **What does a TCG Live battle log print when a player plays a Legendary Stadium pair, and when a pair is replaced?** This depends on a real log, which only the user can supply, and no log in the corpus has one. The importer task reads the real shape first and recognises it from the log text alone. If no real log exists when that task is reached, STOP and ask the user to supply one or to confirm deferral, and record the deferral. If the shape can only be recognised by reading card data, STOP and ask the user, because that would contradict knowledge `architecture/importer-determinism.md`.
- **Do the card-set-import plan's `Print`, `Print.Card` and correction apply land as that plan states?** If names differ, adapt to what landed. If a correction cannot set an absent optional field such as `/playedAsPair`, fix it in the corrections code with a test. If the format has no place for a new optional core field without bumping `format`, STOP and ask the user.
- **Does the publishing plan's loader keep its current copy of a set it cannot decode, rather than dropping the catalogue?** A playmat built before `playedAsPair` decodes built sets strictly, so after the rebuilt Delta Reign set is published, such a playmat will fail to decode `me06`. The publishing plan says a failed or unknown set leaves the current snapshot in place, so old playmats keep their previous Delta Reign until updated. Confirm this against the landed loader while implementing the Delta Reign task. If an older playmat would lose its whole catalogue or Delta Reign entirely, STOP and ask the user (options: ship the code release before the data, or make the runtime decoder tolerate unknown fields).

## Out of Scope

- **Enforcing Stadium effects** (spec Non-Goals). The playmat does not apply Legendary Summit's prize reduction, Pyroduct's removal of Abilities or Trench's double healing. Players and the assistant apply them as with any other card. Pair warnings are advisory, and no pair rule blocks a move.
- **Pair play for card types other than Stadiums** (spec Non-Goals). The mark is general data, but only Stadium pairs are supported by the state, the verbs, the assistant's tools, the board and the importer. A `pair_iid` given with a non-Stadium is refused.
- **Importing a pair without a real battle log.** If no real TCG Live log that plays a pair exists when the importer task is reached, that requirement is deferred to a follow-up spec and recorded, as the spec allows. No log format is guessed.
- **Building and publishing card sets.** Importing, verifying, the corrections file, the report and the built format belong to the card-set-import spec (`20261108110847-d27a9f77-card-set-import`). Publishing and the playmat's loader belong to the card-set-publishing spec (`20261108105504-9abfa10e-card-set-publishing`). This plan adds one field to the format and two kinds of corrections, and relies on publish-on-merge to deliver them.
- **Marking pair-played cards in sets other than Delta Reign.** No other pair-played card exists today. A future one is marked by a correction in its own set, with no program change.
- **A dedicated "play as pair" control on the board.** Pairs are played on the board by dragging the two halves in turn. A menu item or multi-select for playing both at once is not added.
- **Fixing the board's 8th bench-slot hint for Stadiums.** The board reads an `effect` field that `/api/card` never sends (found during discovery). It is unrelated to pairs and is left for a separate fix.
- **Rarity, alternate prints and other card fields.** Only the pair mark is added to the format.
