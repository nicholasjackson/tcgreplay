---
created_date: "2026-11-07"
document_status: final
closed_date: "2026-11-07"
---

# Plan: 20261107074845-fc71eb04-real-game-opponent

<!-- Metadata -->
<!-- Created: 2026-11-07T08:37:33Z -->
<!-- Commit: ea083d7 -->
<!-- Branch: main -->
<!-- Repository: git@github.com:nicholasjackson/tcgreplay.git -->

## Overview

When the player takes their turn on the board while the assistant plays the other side, the assistant's side now reacts the way a real opponent would. Cards like Judge, Special Red Card, Iono and Unfair Stamp shuffle away the assistant's hand and draw for it at once, as part of the player's card. The wait for the player's turn stops only when the assistant has a real decision: promoting after a knockout, or a card it must choose for or read. While it decides, the player keeps playing everything that doesn't depend on that decision. Players practising against the assistant no longer have to wait for it, or tell it what to do, during their own turn.

## Conventions

- **Never write "Claude" or "Nic " in an `internal/guides` guide; guides serve any assistant and any player** — the play-game, replay-drill and review-game guides are reworded to describe applied effects and decision stops, and `internal/guides/guides_test.go` fails on either word.
- **Never exclude `internal/guides` from a build context; the guides are embedded with `//go:embed *.md`** — this feature edits guide files only and adds no packaging change; `.dockerignore` and the Dockerfile stay untouched so the binary still builds.

## Architecture & Design Decisions

All work lands in the single registered repo, `tcgreplay` (`/home/nicj/code/github.com/nicholasjackson/tcgreplay`), and builds on the turn wait's presence (`internal/game/presence.go`) and its renderer (`internal/mcpserver/wait.go`), including the uncommitted 15-second cap, `action`, `keep_waiting` and numbered steps already on main. The feature has three parts.

1. **Reading a card's effect on the opponent.** A small, pure reader in the game package (`internal/game/effects.go`, new) reads an effect text and sorts it into one of four kinds:
   - **none**: the opponent is never the subject of an action. Naming the opponent's Pokémon as a target ("your opponent's Active Pokémon", a gust, a damage bonus) still counts as none, because the player resolves it on the board.
   - **apply**: the whole text is the shuffle-your-hand-and-draw family, parsed sentence by sentence:
     - an optional "You can use this card only if …" condition;
     - a disposal: "each player / your opponent shuffles their hand into their deck" or "… shuffles their hand and puts it on the bottom of their deck";
     - the draws, with an optional "Then," and an optional "If either player / they put any cards on the bottom … in this way,". A draw count is a number, "a card", or "a card for each of their remaining Prize cards". "his or her" reads as "their".
   - **choice**: the opponent or each player is the subject and chooses something, for example "chooses", "switches their Active Pokémon" or "of their choice".
   - **unknown**: the opponent is the subject of anything else, or the text is unavailable.

   Any sentence the family grammar does not accept makes a text unknown rather than apply, so the playmat applies only what it fully understands. This follows the spec's "by the wording, not a list of names". It answers the named risk by checking each card's own condition and count, never a family-wide default. Card text always comes from TCGdex's English data, whatever language the card was printed in, because the playmat identifies cards by their TCGdex id. So the reader handles English only. A card with no English text (a print that exists only in another language, or a Trainer whose text TCGdex lacks) counts as unknown and goes to the assistant.

2. **Applying the family inside the player's own card.** `PlayTrainer`, `UseAbility` and `Attack` (`internal/game/verbs.go:428-582`) already hold the effect text in their transaction. When the actor is the human and the opponent is the assistant's side (always in play mode; in review mode only while a turn wait has handed that seat to the player, which the presence tells the game the same way it sets the response window today), they finish the same transaction with the opponent's part:
   - the opponent's hand moves to its deck (shuffled in) or to the bottom (in a random order);
   - then the opponent draws the parsed count (the opponent's remaining prizes for Iono).

   These events are emitted on the opponent's seat with the system actor, so the card and its effect commit atomically as one action. Undo by action (`reads.go:610-644`) removes both, and the turn's log and summary list them under the card's label. Draws go through the existing deck and knowledge rules, so the assistant's new hand stays hidden. A checkable condition that is not met (Unfair Stamp's knockout during the opponent's last turn, using the knockout scan of `prizesOwedLocked`; Special Red Card's "3 or fewer Prize cards remaining") still applies, and adds a warning to the result, which the board shows as a toast. The player's own side of the card is untouched: they resolve it on the board as today. The board's write result is redacted for the player's view with `DeltaFor`, as the MCP side already is (knowledge `gotchas/board-delta-unredacted.md`), so the drawn cards' names never reach the browser.

3. **Decision stops and a narrowed hold.** The wait's per-card `respond` and its separate `promote` become one `decide` outcome with `action` `promote` or `answer_card`:
   - **promote**: the assistant's Active is empty and it has Benched Pokémon. The result lists the exact Bench slots it may promote from. A knockout that leaves no Pokémon still ends the game first, as today.
   - **answer_card**: the player used a card whose kind is choice or unknown, or an apply card that was not applied because its text was not cached at play time. Each card says whether it leaves a choice or needs reading.

   Cards of kind none, and applied cards, never stop the wait. Every result carries an `applied` list of effects not yet reported (card, iid and what happened, for example "p2's hand of 6 was shuffled into p2's deck; p2 drew 4"); a turn-ended result lists all of the turn's. The `next` text names the decision and the seat, gives numbered steps ending with the hand-back call, and stops saying the board is locked. The instruction lives in the tool result (knowledge `gotchas/handoff-in-tool-results.md`, `gotchas/chatgpt-tool-discovery.md`).

   The presence's single `acting` flag becomes a hold with two strengths:
   - **full**: the assistant's own turn, as today.
   - **decision**: set by a `decide` outcome, released when the assistant waits again, or by the existing lock-release timer, which then marks the opponent `stopped`.

   The board's one lock check (`internal/webui/actions.go:144`) refuses everything under a full hold. Under a decision hold it refuses only dependent actions: attacking; ending the turn; checkout and rewind; any write aimed at the opponent's side (a target or destination on its seat, or one of its cards); and a Trainer or Ability whose text names the opponent at all. The refusal says what the board is waiting for ("Waiting for your opponent to promote a new Active Pokémon; try again when they have"). The board shows a new `deciding` status. When a decision hold lifts by itself, it tells the player they can carry on and make the opponent's move themselves. A "Promote to Active" entry on the opponent's Benched Pokémon (shown while its Active is empty) sends the existing `move` verb. The game turn lock and the response window (knowledge `gotchas/turn-lock-cross-seat-joins.md`) are unchanged: a decision still opens the window for the assistant's seat.

This beats the alternatives because it puts each concern at its existing single owner:
- the effect reader beside the card text, in the game;
- the application inside the one transaction that already plays the card;
- what to report in the wait's evaluation;
- the hold in the board's one dispatch check.

Applying from the wait or from the board's dispatch instead would mean a second commit after the card, which can interleave with the player's next move and splits undo. A plain "mentions opponent" filter would stop on most attacks. A separate hold mechanism would duplicate the presence's timer, status and release paths. Guides (`play-game`, `replay-drill`, `review-game`), the server instructions, the `wait_for_turn` description and the website's play-game page change to describe applied effects and decision stops, following the guide-text convention below. See `research.md#alternatives-considered-and-rejected` for the evidence.

## Component Breakdown

- **Effect reader (new, in the game package).** A pure function from an effect text to what it asks of the opponent: none, apply, choice or unknown. For apply it also returns the parsed effect:
  - where the opponent's hand goes (shuffled into the deck, or onto the bottom);
  - how many cards the opponent draws (a fixed number, or one per remaining Prize card);
  - whether the draw depends on cards having been put on the bottom (by the opponent alone, or by either player);
  - which recognised use condition, if any, the card carries.

  It also answers the narrower question the board needs: does a text name the opponent at all. It owns no state and knows no card names. It is justified as new because both the game (to apply) and the wait and board (to classify and hold) need the same reading, and it must be testable on real card wording alone.
- **Game (changed).**
  - Its three opener verbs (play a Trainer, use an Ability, attack) finish their transaction with the opponent's part of an apply effect when the human plays it against the assistant's side. They emit it on the opponent's seat as system events within the card's action.
  - It checks the two recognisable use conditions, Unfair Stamp's knockout during the opponent's last turn and the opponent's remaining Prize cards, and adds a warning when one is unmet.
  - It holds which seat a turn wait has handed to the player, set and cleared by the presence the same way as the response window, and cleared with it on a mode switch or board move. This decides whether review-mode practice applies effects.
  - It reports, for the wait, the effects applied after a cursor (card, iid and what happened to the opponent's side) and the opponent's Bench slots for a promotion. It offers a card-text check for the board's hold.
  - Undo, the log, knowledge bits and redaction are unchanged, and cover applied events because the events go through the ordinary paths.
- **Presence and turn wait (changed).**
  - It replaces the per-card respond and the separate promote with one decide outcome. Promotion is judged first, from the assistant's empty Active with a Bench. Then come the cards the player used that need the opponent: choice and unknown cards, and apply cards that were not applied.
  - It carries the applied effects not yet reported on every outcome, and all of the turn's on turn ended.
  - It replaces its single acting flag with a hold of two strengths. A full hold is the assistant's own turn, set as today. A decision hold is set by a decide outcome and released by the next wait call or by the existing release timer.
  - It reports the board status `deciding` under a decision hold, and remembers that a decision hold lifted by itself so the board can tell the player.
  - It tells the game the handed seat when a session starts, and clears it when the session ends.
- **Turn wait renderer (changed, MCP server).**
  - It renders decide outcomes: `action` promote with the Bench choices, or answer_card with each card marked to choose or to read.
  - It renders the `applied` list on every result.
  - Its `next` texts name the decision and the assistant's seat, give numbered steps ending with the hand-back call, and no longer say the board is locked for the player.
  - The tool description and the standing instructions describe applied effects and decision stops.
- **Board server (changed).**
  - Its one dispatch check asks the presence for the hold. Under a full hold it refuses game actions as today. Under a decision hold it refuses only dependent actions: attack, end turn, checkout, rewind, writes aimed at the opponent's side, and Trainers or Abilities that name the opponent. The refusal says what the board is waiting for.
  - Its write results redact the delta for the player's view.
  - Its state message carries the new status, and whether a decision hold lapsed.
- **Board app (changed, Svelte).**
  - It shows "Opponent is deciding" while a decision is pending, keeping the player's controls enabled.
  - It shows a one-time notice when a decision hold lapses, telling the player they can carry on and make the opponent's move.
  - It offers "Promote to Active" on the opponent's Benched Pokémon while the opponent's Active Spot is empty, using the existing move action.
  - Held actions surface through the existing refusal toast.
- **Coaching guides and server instructions (changed).** play-game, replay-drill and review-game stop telling the assistant to answer every card. They describe instead that the playmat applies the shuffle-and-draw family itself, and that a decide outcome asks for a promotion or a card answer. Each guide keeps its pinned rule anchors.
- **Website playing docs (changed).** The play-game workflow page gains a short description: the opponent's side of cards like Judge happens on its own, and the assistant is only asked when it has a real decision.

## Data Structures & Interfaces

**Effect reading (new, game package).** `ReadEffect` is the effect reader's contract, a pure function from effect text to what it asks of the opponent. The family's parsed shape is returned only for `OppApply`. `NamesOpponent` is the looser check the board's hold uses.

```go
type OppKind int // OppNone | OppApply | OppChoice | OppUnknown

type OppEffect struct {
    Kind      OppKind
    HandTo    string    // "deck" (shuffled in) | "bottom" (OppApply only)
    Draw      int       // fixed number of cards the opponent draws
    PerPrize  bool      // draw one card per remaining Prize card instead
    DrawIf    string    // "" | "opponent_put" | "either_put": draw only if cards went to the bottom
    Condition Condition // the card's "You can use this card only if …", when recognised
}

type Condition struct {
    Kind string // "" | "ko_last_turn" | "opp_prizes_at_most" | "unrecognised"
    N    int    // for opp_prizes_at_most
}

func ReadEffect(text string) OppEffect
func NamesOpponent(text string) bool
```

**Applied effects and the handed seat (new, game).** `AppliedEffect` describes one effect the playmat applied to the other side, for the wait and its renderer. `What` is a short sentence in seat names, such as "p2's hand of 6 cards was shuffled into p2's deck; p2 drew 4 cards". The handed seat is set by the presence and read inside the opener verbs.

```go
type AppliedEffect struct {
    IID    state.IID // the card the player used
    Name   string    // its action label ("Judge")
    Def    string
    Action string    // the action id, so a wait reports each once
    What   string
}

func (g *Game) setHanded(seat state.Seat)          // "" clears; reset with the responder
func (g *Game) OpenerNamesOpponent(seat state.Seat, verb string, iid state.IID, ability string) bool // board's hold check
```

**Wait outcome (changed, game).** `WaitRespond` and `WaitPromote` are replaced by `WaitDecide`. The outcome gains the decision's action, the Bench choices for a promotion, a need on each reported card, and the applied effects.

```go
const WaitDecide = "decide"
const (
    DecidePromote    = "promote"
    DecideAnswerCard = "answer_card"
)

type WaitOutcome struct {
    // … existing: Kind, Seat, Since, Next, Turn, TurnSeat, Prompt, Reason, Polls, Waited
    Action  string          // decide: DecidePromote | DecideAnswerCard
    Choices []BenchChoice   // promote: the Bench slots the assistant's seat may promote from
    Cards   []UsedCard      // answer_card: the cards to answer
    Applied []AppliedEffect // every outcome: effects not yet reported (turn_ended: the whole turn's)
}

type BenchChoice struct {
    Slot int
    IID  state.IID
    Name string
}

type UsedCard struct {
    IID                   state.IID
    Name, Def, Kind, Text string
    Need                  string // "choose" (the card leaves the opponent a choice) | "read" (read it and do what it asks)
}
```

**Presence hold (changed, manager surface).** `Hold` replaces `Locked` as the board's question. `Opponent` gains `deciding`. `Decision` names the pending or lapsed decision for the board's message and notice.

```go
type Hold string // HoldNone "" | HoldFull "full" | HoldDecision "decision"

func (m *Manager) Hold() Hold
func (m *Manager) Opponent() string   // "waiting" | "acting" | "deciding" | "stopped" | ""
func (m *Manager) Decision() string   // DecidePromote | DecideAnswerCard while deciding or after it lapsed, else ""
```

**`wait_for_turn` result (changed MCP contract).** The tool's input is unchanged. The output keeps every existing field.

```text
output { outcome: "turn_ended" | "decide" | "check_in" | "ended" | "still_waiting",   // "respond" and "promote" are gone
         action?: "promote" | "answer_card",                        // decide only
         decision?: string,                                         // decide: one sentence naming the decision and the assistant's seat
         choices?: [{ slot, iid, name }],                           // promote: Bench slots it may promote from
         cards?: [{ iid, name, card, kind, text, need: "choose" | "read" }],   // answer_card
         applied?: [{ iid, name, card, what }],                     // any outcome: effects the playmat applied to the assistant's side
         seat, since, turn?, turn_seat?, prompt?, log?, reason?, keep_waiting, wait, waited, next }
```

**Board wire format (changed).**
- The hub's `state` message: `opponent` gains `"deciding"`, and a new `decision` (`"promote"` | `"answer_card"`) is present while deciding and after a decision hold lapsed.
- A held action's `result` has `ok: false` with an error naming what the board waits for.
- Write results carry a delta redacted for the player.
- Board requests are unchanged: the opponent's promotion from the board uses the existing `move` verb.

## Implementation Detail

**A new pattern: a whole-text effect grammar.** The effect reader is the first place where the playmat interprets card text beyond the single-word `joinIf` check it uses for joins today. It is deliberately small and strict:
- It normalises the text: whitespace, "his or her" read as "their", and number words.
- It splits the text into sentences, then matches each sentence against a short list of recognised shapes: a condition, a hand disposal, draws (with "Then," and the "If … put any cards on the bottom … in this way," lead-ins), and the player's own draw.
- A text is apply only if every sentence matches and the opponent is in the disposal. Otherwise it falls through to choice, unknown or none.

The rules live in one table of patterns that a reader can scan, with each pattern next to the real card text it was written for. The tests are table-driven over real TCGdex wording, both the minimum set and near misses (Eri, Boss's Orders, damage-bonus attacks, Escape Rope-style switches). The reader has no dependencies on the game state, so it is unit-tested alone.

**Applying the effect follows the existing compound-transaction pattern.** The opponent's part is a small transaction helper modelled on the mulligan helper: it moves the hand, shuffles (or orders it for the bottom), then draws. It is called at the end of the opener verbs' own transaction functions, not from a listener or a second commit. The helper emits on the opponent's seat under the system actor by temporarily switching the transaction's actor, which is a contained widening of the transaction type: one transaction, two actors. A developer reading an opener verb sees one extra call ("apply the opponent's part, if any") after the card's own moves. The eligibility rule sits in one place: a human actor, and the assistant's side by mode and the handed seat. The condition checks reuse the knockout scan the prize-owed check already does, factored out so both share it.

**The wait keeps its fixed evaluation order with one changed step.** The order stays: ended, turn ended, game over, then mid-turn. The mid-turn step now asks two questions in turn:
1. Promotion pending (the vacancy not yet reported, and a Bench exists)?
2. Any unreported cards that need the opponent?

It also gathers the applied effects not yet reported, which ride on whatever outcome this call returns. That includes `still_waiting`, so the assistant learns of them within one wait cap without the wait stopping. The session remembers reported applied actions, as it already remembers reported cards. Card classification happens at report time from the cached text. So a card that was unknown at play time but is cached by the time it is reported still lands correctly, as `read` if it should have applied but did not.

**The hold widens the existing acting lock rather than adding a lock.** The boolean becomes a two-valued hold. The timer, the stopped flag, the release path, the clearing on game changes and the board status all stay as they are, and gain only the decision case and a remembered "lapsed decision". The board's single guard becomes a small function: given the hold and the request, is this request allowed? It keeps a table of dependent verbs, plus checks on the request's target and destination seats and, for openers, the card text through the game. The guard stays the only enforcement site, so no game verb learns about holds. The existing turn lock and response window are untouched.

**Rendering stays in one function.** The renderer's switch replaces its respond and promote cases with one decide case that branches on the action. Each branch's `next` text is a numbered list ending in the exact hand-back call, matching the style just introduced. The applied list and the choices are plain JSON fields. The turn-ended `next` text is unchanged apart from mentioning applied effects when there are any.

**Board app changes follow "server is authoritative, UI renders".** One new status label, one notice on the `deciding` → `stopped` transition, and one menu entry. Nothing in the client decides what is held: held actions come back as refusals and show through the existing error toast.

**Tests follow each package's existing helpers.**
- The game's respond and lock tests are rewritten around decide outcomes, using the existing respond-game fixture with the real card wording.
- The board's lock tests gain a game with an opponent Bench and a promotion hold.
- The MCP wait test pins the new `next` texts and fields.
- The guide rule anchors gain lines for the new decision wording.

## Dependencies

- **Design documents this plan was built on:** none. The spec carries no design references.
- **Uncommitted wait improvements on main (must be present before this plan starts).** The 15-second wait cap, the `wait`/`waited` counters, `keep_waiting`, the `action` field and the numbered respond/promote steps sit uncommitted in the working tree (presence, wait renderer, its tests, run-mode flag default). This plan rewrites the same code and builds on those fields. The implementer must start from a tree that contains them, committed or not, and must not discard them.
- **Turn wait presence (internal, changed).** The wait session, acting lock, release timer, board status and response window come from the wait-for-turn plan, already landed. They change into decide outcomes and a two-strength hold.
- **Game package (internal, changed).** Opener verbs, transactions, the action log, undo by action, knowledge bits and log redaction. Gains the effect reader, the opponent's part of apply effects, the handed seat and the applied-effects read.
- **Card catalogue (internal, unchanged).** Supplies each card's effect, Ability and attack text from the local cache without blocking. The family is recognised from this text, so a card missing from the cache falls back to asking the assistant.
- **MCP server package (internal, changed).** The `wait_for_turn` renderer, its tool description and the standing instructions.
- **Board server package (internal, changed).** The one dispatch guard, delta redaction for the player, and the state message's status and decision.
- **Board app, Svelte 5 / Vite (existing, changed).** A status label, a lapse notice and a promote entry for the opponent's Bench. No new packages.
- **Coaching guides package (internal, changed).** Three guide texts change; the embedded guide set is unchanged.
- **Website, Astro (existing, changed).** One workflow page's text. It is built into the embedded site as today, with no build change.
- **`github.com/modelcontextprotocol/go-sdk` v1.6.0 (existing).** Tool registration and test transports; no version change.
- **TCGdex (external, existing).** The source of the card wording the reader is written against. No new calls are added at runtime.
- **Prior plans relied on, all landed:** wait-for-turn (presence, wait tool, board lock and status), turn-numbering-by-round (per-player turns used for "your opponent's last turn"), guide-tool (guide rule-anchor tests), project-website (the site's workflow pages). Nothing else must land first.

## Testing Approach

**Test types.** All automated tests are Go tests in the project's existing suite. They follow each package's current helpers and conventions: in-package tests with the fake card catalogue, a seeded random source, the started-manager and respond-game fixtures for the game, websocket helpers for the board server, and in-memory transports for the MCP server.

- **Unit (effect reader): the heaviest coverage.** It is the riskiest new logic, and pure. The tests are table-driven over real TCGdex wording:
  - the minimum set (Judge, Special Red Card, Iono, Unfair Stamp) and the rest of the family (Red Card, Reset Stamp, Roxanne, Marnie) each read as apply, with the right disposal, count, per-prize flag, bottom-condition and use condition;
  - choice texts (each player switches their Active, the opponent chooses) read as choice;
  - opponent-as-subject texts outside the family (Eri, Mega Absol ex's attack) read as unknown;
  - possessive-only texts (Boss's Orders, damage bonuses, damage-counter placement, Ultra Ball, empty text) read as none;
  - a family text with an extra unknown sentence reads as unknown, not apply;
  - `NamesOpponent` is true for possessive mentions.
- **Integration (game): application.** Through the game's verbs, with the human on one seat and the assistant's seat holding a known hand:
  - Judge leaves the assistant with 4 cards from a shuffled deck.
  - Special Red Card leaves 3, with its old hand on the bottom of its deck.
  - Iono leaves one card per remaining prize, with its old hand at the bottom.
  - Unfair Stamp leaves 2.
  - Unfair Stamp without a knockout in the opponent's last turn still applies and returns the condition warning; the same holds for Special Red Card above 3 prizes.
  - The applied events sit in the card's action under the system actor.
  - Undo of the card restores the assistant's hand, deck and the player's hand exactly.
  - The assistant's drawn cards are hidden in the player's view of the state, the log and the board's write result.
  - The assistant's own Judge (assistant actor) applies nothing to the player's side.
  - In review mode it applies only while the seat is handed over.
- **Integration (presence and wait): outcomes and the hold.**
  - A turn of Ultra Ball, Energy, evolving and a damage-only attack yields only `still_waiting` until the turn ends.
  - Judge yields no stop, and the next result lists it in `applied`; turn ended lists it too.
  - A knockout with a Bench yields one `decide`/`promote` with exactly the Bench slots.
  - A knockout of the last Pokémon ends with game over and no promotion.
  - A choice card and an unknown card each yield one `decide`/`answer_card` naming the card and its need.
  - A family card whose text was uncached at play time yields `answer_card` with need `read`.
  - Decide sets a decision hold, never a full one. The next wait call releases it, and the release timer lapses it to stopped with the decision remembered.
  - The assistant's own turn still sets a full hold.
  - Review-mode practice behaves the same.
- **Integration (board server): the narrowed guard.** With a promotion pending:
  - attaching Energy, evolving and an own-side Item succeed;
  - attack, end turn, checkout, rewind, a gust Trainer and a move onto the opponent's Active are refused with the waiting message;
  - after the assistant promotes and waits again, attack and end turn succeed;
  - after the release time, the status is stopped with the decision, and the player can move the opponent's Benched Pokémon into its Active and attack.

  The full hold's existing refusal tests stay for the assistant's own turn. The pushed status includes `deciding`.
- **Contract (MCP renderer).** The decide results carry the right `action`, `decision`, `choices`, `cards` with `need`, `applied`, `keep_waiting` true, and numbered `next` texts. Those texts name the seat, end with the exact hand-back call, and no longer say the board is locked. The instructions keep their pinned loop lines.
- **Regression (guides).** Guide rule anchors pin the new decision wording in play-game, replay-drill and review-game. A check asserts that no guide or wait text tells the assistant to check every card or that the wait stops for every card.

**Load-bearing guarantees.** The playmat applies exactly the opponent's part of the family and nothing else. A card it does not fully understand always reaches the assistant. The assistant is told about every applied effect. Undo restores the board. Hidden cards stay hidden. The player is held only on dependent actions, and never longer than the release time.

**Success metrics.**
- *A player turn of non-opponent cards or no-choice effects never blocks the player's next legal move (zero seconds of waiting).* **Behavioural test.** A scripted board turn of Ultra Ball, Energy, evolving, a damage-only attack and Judge sends each action straight after the last. Every one is accepted with no hold in place, and the wait reports only `still_waiting` until the turn ends.
- *When the assistant must decide, only dependent actions are blocked, and only for as long as it takes.* **Behavioural test.** Under a pending promotion the board accepts the independent actions and refuses the dependent ones. The refused ones succeed as soon as the assistant hands the turn back, with no further delay, and the hold lapses at the release time if it never does.
- *The wait stops at most once per decision the assistant's side has to make, plus at most one check per card the playmat cannot apply; a turn of ordinary cards produces no stops.* **Behavioural test.** Over a scripted turn mixing ordinary cards, Judge, one unknown opponent card and a knockout, the wait returns exactly two decide outcomes (one promote, one answer_card), and an ordinary turn returns none.
- *In a played-out turn against ChatGPT with Judge and a knockout, the player never has to tell the assistant to act or keep waiting.* **Manual — captured in the implementation test plan.**

**Manual reviews.**
- **Manual — captured in the implementation test plan:** the board's new "Opponent is deciding" label, the lapse notice and the "Promote to Active" entry on the opponent's Bench look right and read clearly on desktop and phone.
- **Manual — captured in the implementation test plan:** a played-out turn against Claude, matching the ChatGPT run, confirms the decide `next` steps are followed and the hand-back happens in the same reply.

**Deliberate gaps.** No Playwright test for the hold. The browser harness has no way to start a wait or set the presence, and adding a test-only route for it is out of proportion: the server guard and status are covered by the board server's Go tests, and the visuals by the manual review above. The website copy change has no test beyond the site build.

## Milestones & Tasks

### Milestone 1: Judge and its family happen on the opponent's side by themselves

**What changes**: When the player plays Judge, Special Red Card, Iono, Unfair Stamp or another card worded the same way on their turn against the assistant, the assistant's hand goes into its deck (or to the bottom) and it draws the card's count at once, as part of the player's card, with no one asked. If the card's own condition is not met (Unfair Stamp with no knockout last turn, Special Red Card above 3 prizes), the effect still happens and the board warns the player. The assistant is told what happened to its side in its next wait result and sees it in the log. Undoing the card puts both hands and decks back. The assistant's new cards stay face down for the player. Practice turns in review mode behave the same while the assistant is waiting on them. The wait no longer asks the assistant to do these cards itself; other cards still stop it as before.

**Validation point**: With the assistant waiting on a played turn, playing Judge leaves it holding 4 cards with no wait stop. The next wait result lists the applied effect. Undo restores the prior hands. The board shows the assistant's new hand only as face-down cards. The game and wait tests for the family, conditions, undo and hidden cards pass.



#### - [x] Task: Effect reader for what a card asks of the opponent
**Id:** b1228e9b-048f-4cab-a969-2a59e623431f
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Add a small, self-contained reader that takes a card's effect text and says what it asks of the opponent: nothing, an effect the playmat can apply itself, a choice for the opponent, or something it cannot judge. For the shuffle-your-hand-and-draw family it also returns:
- where the hand goes;
- how many cards the opponent draws;
- whether the draw depends on cards having gone to the bottom;
- any use condition it recognises.

It accepts a card as applicable only when it understands every sentence, so anything unusual falls back to asking the assistant. A looser check, "does this text name the opponent at all", serves the board's hold later.

*Technical detail:* [context.md#task-effect-reader-for-what-a-card-asks-of-the-opponent](./context.md#task-effect-reader-for-what-a-card-asks-of-the-opponent)

**Acceptance criteria**:
- [x] Judge, Special Red Card, Iono and Unfair Stamp, in their real current wording, read as applicable, each with its own count (4; 3; one per remaining prize; 2) and its own hand destination (deck, or bottom for Special Red Card and Iono).
- [x] Red Card, Reset Stamp, Roxanne and Marnie also read as applicable with their own counts.
- [x] Unfair Stamp's knockout condition and Special Red Card's "3 or fewer Prize cards" condition are recognised.
- [x] A card making each player switch their Active Pokémon reads as a choice. Eri and Mega Absol ex's hand-reveal attack read as unrecognised.
- [x] Boss's Orders, damage-bonus cards, damage-counter attacks, Ultra Ball and empty text read as asking nothing of the opponent.
- [x] A family card with one extra sentence it does not understand reads as unrecognised, never applicable. A "You can use this card only if …" condition is not such a sentence: a condition the reader cannot check is the player's responsibility, so the card still applies with no warning.

#### - [x] Task: Apply family effects to the opponent inside the card
**Id:** fb832ac4-11a0-4186-81ea-41a8679dca62
**Repo:** tcgreplay
**Depends on:**
- b1228e9b-048f-4cab-a969-2a59e623431f — Effect reader for what a card asks of the opponent
**Execution:** agent

When the human plays a Trainer, uses an Ability or attacks with an applicable effect against the assistant's side, the game finishes the same move with the opponent's part. The assistant's hand goes into its deck (shuffled) or onto the bottom, and it draws the card's count, recorded as part of the player's card. This applies always in play mode, and in review mode only for the seat a turn wait has handed to the player. An unmet use condition still applies and adds a warning. The game can also list, for the wait, the effects it applied after a point in the log.

*Technical detail:* [context.md#task-apply-family-effects-to-the-opponent-inside-the-card](./context.md#task-apply-family-effects-to-the-opponent-inside-the-card)

**Acceptance criteria**:
- [x] With the assistant holding 6 cards, the player's Judge leaves it with 4 cards drawn from a shuffled deck containing its old hand, in the same move as the Judge.
- [x] Special Red Card leaves the assistant 3 cards with its old hand at the bottom of its deck; Iono leaves as many cards as its remaining prizes with its old hand at the bottom; Unfair Stamp leaves 2.
- [x] Unfair Stamp with no knockout of the player's Pokémon in the opponent's last turn, and Special Red Card while the opponent has more than 3 prizes, still apply and return a warning that the card's condition was not met.
- [x] Undoing the card restores the assistant's hand and deck and the player's hand exactly.
- [x] The assistant's own Judge changes nothing on the player's side, and a review-mode Judge applies nothing unless that seat has been handed over by a turn wait.
- [x] In play mode the player's view of the state and of the log never names the cards the assistant drew.

#### - [x] Task: Hide opponent cards in board move results
**Id:** 8edc34a3-dc1e-4703-93cb-d36d78bd821d
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

The board's reply to each of the player's moves includes a list of what changed, and today that list names every card, including hidden ones. Now that a player's card can make the opponent draw, the list is redacted for the player's own view, as the assistant's results already are, so the opponent's new cards never reach the browser.

*Technical detail:* [context.md#task-hide-opponent-cards-in-board-move-results](./context.md#task-hide-opponent-cards-in-board-move-results)

**Acceptance criteria**:
- [x] In play mode, the board's result for a move that draws cards for the opponent shows those cards as hidden, never by name.
- [x] With the reveal view on in review mode, the result still names every card, as the board itself does.

#### - [x] Task: Tell the assistant about applied effects in the wait
**Id:** 5cf49385-b917-4e48-b98a-81a2a9190663
**Repo:** tcgreplay
**Depends on:**
- fb832ac4-11a0-4186-81ea-41a8679dca62 — Apply family effects to the opponent inside the card
**Execution:** agent

The turn wait tells the game which seat it has handed to the player, so practice turns in review mode apply effects too. Every wait result lists the effects applied to the assistant's side that it has not been told about yet, and the turn-ended result lists the whole turn's. A card whose effect was applied no longer stops the wait asking the assistant to do it again. Other cards still stop it as before until the next milestone.

*Technical detail:* [context.md#task-tell-the-assistant-about-applied-effects-in-the-wait](./context.md#task-tell-the-assistant-about-applied-effects-in-the-wait)

**Acceptance criteria**:
- [x] With the assistant waiting, the player's Judge does not stop the wait, and the next wait result (even a still-waiting one) lists Judge and what happened to the assistant's hand. Later results do not repeat it.
- [x] The turn-ended result lists every effect applied that turn, and its log lines include them under the card's name.
- [x] In a review-mode practice turn handed over by a wait, the player's Judge applies to the other seat and is reported the same way.


### Milestone 2: The wait stops only when the assistant has a real decision

**What changes**: The assistant is no longer stopped for every card, Ability or attack. A turn of ordinary cards (Ultra Ball, Energy, evolving, a damage-only attack) runs without a single stop. The wait stops only:
- to promote after a knockout, listing the exact Bench slots it may choose;
- for a card that leaves the opponent a choice;
- for a card that involves the opponent in a way the playmat cannot apply.

Each stop names the decision and the assistant's seat, and gives numbered steps ending with how to hand the turn back. The guides, the server instructions, the wait tool's description and the website's play-game page describe applied effects and decision stops, and none says the wait stops for every card.

**Validation point**: An ordinary turn yields only still-waiting results until it ends. A knockout yields one promote decision with the Bench slots. A choice card and an unrecognised opponent card each yield one decision naming the card. The wait renderer, guide anchor and "no stop for every card" tests pass.



#### - [x] Task: Wait stops only for decisions
**Id:** d77cddb9-f4e1-4513-b354-de0446b62fad
**Repo:** tcgreplay
**Depends on:**
- b1228e9b-048f-4cab-a969-2a59e623431f — Effect reader for what a card asks of the opponent
- 5cf49385-b917-4e48-b98a-81a2a9190663 — Tell the assistant about applied effects in the wait
**Execution:** agent

The wait's per-card stop and its separate promotion stop become one decision stop. It reports a promotion when the assistant's Active Spot is empty and it has Benched Pokémon, with the exact Bench slots it may choose. Otherwise it reports the cards the player used that need the opponent, marked "choose" or "read": cards leaving a choice, cards it cannot judge, and family cards that could not be applied. Cards asking nothing of the opponent never stop the wait.

*Technical detail:* [context.md#task-wait-stops-only-for-decisions](./context.md#task-wait-stops-only-for-decisions)

**Acceptance criteria**:
- [x] A turn of Ultra Ball, attaching Energy, evolving and a damage-only attack gives only still-waiting results until the turn ends.
- [x] Knocking out the assistant's Active with Pokémon on its Bench gives one promotion decision listing exactly those Bench slots. Knocking out its last Pokémon ends the game with no promotion asked for.
- [x] A card making each player switch their Active gives one decision naming the card as a choice. An unrecognised card naming the opponent as actor gives one decision asking the assistant to read it.
- [x] A family card whose text was not available when played gives one decision asking the assistant to read it.
- [x] A Trainer with no text in the card data (for example, a card with no English entry) gives one decision asking the assistant to read it, never a silent pass.

#### - [x] Task: Decision results tell the assistant exactly what to do
**Id:** 7171a850-b29b-49d0-946f-4f0bf5f1e1ef
**Repo:** tcgreplay
**Depends on:**
- d77cddb9-f4e1-4513-b354-de0446b62fad — Wait stops only for decisions
**Execution:** agent

The wait tool's results present the new decision stop: which decision it is, the assistant's seat, the valid choices, the cards and what each needs, and the applied effects. Each result gives numbered steps ending with the exact call that hands the turn back. The tool's description and the standing instructions explain that shuffle-and-draw cards happen on their own and that the wait stops only for decisions. No wait text says the board is locked for the player or that every card must be checked.

*Technical detail:* [context.md#task-decision-results-tell-the-assistant-exactly-what-to-do](./context.md#task-decision-results-tell-the-assistant-exactly-what-to-do)

**Acceptance criteria**:
- [x] A promotion result names the decision and the assistant's seat, lists the Bench slots, and ends its steps with the hand-back call carrying the right seat and cursor.
- [x] A card result names each card and whether it leaves a choice or needs reading, with steps to act on the assistant's seat and then hand the turn back.
- [x] Every result carries the applied effects not yet reported, and still says to keep waiting until the turn ends.
- [x] The tool description and standing instructions describe applied effects and decision stops, and keep their existing loop lines.

#### - [x] Task: Guides and website describe applied effects and decision stops
**Id:** b6967fe9-829c-4c4f-8a8d-e0313c71fe41
**Repo:** tcgreplay
**Depends on:**
- 7171a850-b29b-49d0-946f-4f0bf5f1e1ef — Decision results tell the assistant exactly what to do
**Execution:** agent

The play-game, replay-drill and review-game guides stop telling the assistant to answer every card. They say instead that the playmat applies the shuffle-and-draw family itself and that a decision stop asks for a promotion or a card answer, to be done and handed back in the same reply. The website's play-game page gains a short description of the opponent reacting on its own. Guide rule checks pin the new wording.

*Technical detail:* [context.md#task-guides-and-website-describe-applied-effects-and-decision-stops](./context.md#task-guides-and-website-describe-applied-effects-and-decision-stops)

**Acceptance criteria**:
- [x] No guide, wait instruction or website page says the wait stops for every card or that the assistant must answer each card.
- [x] The three guides and the website's play-game page describe applied effects and decision stops.
- [x] The guides still pass their rule checks, including never naming a specific assistant or player.


### Milestone 3: The player keeps playing while the assistant decides

**What changes**: While the assistant decides, the board no longer locks the player out. They can attach Energy, evolve and play Items on their own side straight away. Only actions that depend on the decision are held: attacking, ending the turn, a gust, anything aimed at the opponent's side, or moving the board back. A held action is refused with a message saying what the board is waiting for (for example, the opponent promoting), and it works as soon as the assistant hands the turn back. The board shows "Opponent is deciding". If the assistant does not answer within the board's release time, the hold lifts and the board tells the player. The player can then carry on, including promoting one of the opponent's Benched Pokémon themselves from a new menu entry. The assistant's own turn still locks the board as it does today.

**Validation point**: With a promotion pending, attaching Energy, evolving and an own-side Item succeed, while attack and end turn are refused with the waiting message. Both succeed after the assistant promotes and waits again. After the release time, the board reports the opponent stopped and the player can promote for it and attack. The board server's hold tests pass.


#### - [x] Task: Decision hold in the presence
**Id:** ec60f31b-b200-4ab0-899c-46462d57a681
**Repo:** tcgreplay
**Depends on:**
- d77cddb9-f4e1-4513-b354-de0446b62fad — Wait stops only for decisions
**Execution:** agent

The assistant's single "acting" lock becomes a hold of two strengths:
- a full hold for the assistant's own turn, unchanged;
- a decision hold while it answers a decision stop.

A decision hold ends when the assistant waits again, or lapses after the board's existing release time. The opponent then shows as stopped, and the presence remembers which decision lapsed. The board can read the hold, a new "deciding" status and the pending or lapsed decision.

*Technical detail:* [context.md#task-decision-hold-in-the-presence](./context.md#task-decision-hold-in-the-presence)

**Acceptance criteria**:
- [x] A decision stop sets a decision hold with the status "deciding", never a full hold. The assistant's own turn still sets a full hold with "acting".
- [x] The assistant's next wait call ends a decision hold.
- [x] A decision hold the assistant leaves for the release time lapses to "stopped", with the lapsed decision still readable until the next wait or game change.
- [x] Mode switches, branch moves, undo, a new game and expiry still clear any hold.

#### - [x] Task: Board holds only actions that depend on the decision
**Id:** edf61c8e-f2a0-4f59-9fcb-94b75fa95c99
**Repo:** tcgreplay
**Depends on:**
- ec60f31b-b200-4ab0-899c-46462d57a681 — Decision hold in the presence
- b1228e9b-048f-4cab-a969-2a59e623431f — Effect reader for what a card asks of the opponent
**Execution:** agent

The board's one lock check refuses everything under a full hold, as today. Under a decision hold it refuses only the actions that depend on the decision:
- attacking and ending the turn;
- moving the board back (checkout and rewind);
- anything aimed at the opponent's side;
- a Trainer or Ability whose text names the opponent.

The refusal says what the board is waiting for. The board's state message carries the new status and the decision, so the board can show them.

*Technical detail:* [context.md#task-board-holds-only-actions-that-depend-on-the-decision](./context.md#task-board-holds-only-actions-that-depend-on-the-decision)

**Acceptance criteria**:
- [x] While a promotion is pending, attaching Energy, evolving and an Item that touches only the player's side take effect at once.
- [x] While a promotion is pending, attacking, ending the turn, a gust Trainer and moving a card onto the opponent's side are refused with a message that the board is waiting for the opponent to promote.
- [x] The same attack and end turn succeed once the assistant has promoted and handed the turn back.
- [x] After the release time with no answer, the board shows the opponent as stopped, and the player can move one of the opponent's Benched Pokémon into its empty Active Spot and attack.
- [x] During the assistant's own turn, the board still refuses the player's game actions as before.

#### - [x] Task: Board shows the opponent deciding and lets the player promote for it
**Id:** 7610c453-767a-4ebd-a34e-3192db9551f8
**Repo:** tcgreplay
**Depends on:**
- edf61c8e-f2a0-4f59-9fcb-94b75fa95c99 — Board holds only actions that depend on the decision
**Execution:** agent

The board shows "Opponent is deciding" beside the opponent's name while a decision is pending, and keeps the player's controls enabled. When a decision hold lapses, it shows a one-time notice that the opponent has not answered and the player can carry on and make its move. The opponent's Benched Pokémon gain a "Promote to Active" menu entry while the opponent's Active Spot is empty.

*Technical detail:* [context.md#task-board-shows-the-opponent-deciding-and-lets-the-player-promote-for-it](./context.md#task-board-shows-the-opponent-deciding-and-lets-the-player-promote-for-it)

**Acceptance criteria**:
- [x] While the opponent decides, the board shows "Opponent is deciding", and the player's cards, menus and buttons stay usable.
- [x] A held action shows the server's waiting message as a notice.
- [x] When a decision hold lapses, the player sees a one-time notice saying they can carry on and make the opponent's move.
- [x] With the opponent's Active Spot empty, each of its Benched Pokémon offers "Promote to Active", which moves it into the Active Spot when the board allows it.

## Open Questions

- **Does the new draw event under the system actor disturb anything that counts draws per turn?** It depends on code paths that treat system draws specially: draws-like-main branch pinning counts draws by turn, and the turn-start draw is also a system draw. That only shows when exercised on a "draws like main" branch. If applied draws shift which card a later main-pinned draw takes, or trip a turn warning, STOP and ask the user before changing the pinning rules.

No other open questions: everything else was resolved during planning and recorded as an assumption.

## Out of Scope

- **General rules enforcement or new legality checks** beyond today's warnings (spec non-goal). The only new warning is the unmet-condition one on applied cards.
- **A step where the player tells the opponent what to do** (spec non-goal; rejected during the spec).
- **A setting to switch the automatic responses off** (spec non-goal).
- **Keeping game records saved before this change working** (spec non-goal). No migration of records or wait outcomes.
- **Request ids or protection against a decision being resolved twice** (spec non-goal).
- **Applying effects that leave the opponent a choice, or any effect outside the shuffle-hand-and-draw family.** Those stop the wait and stay with the assistant (spec constraint "real decisions stay with the assistant").
- **Card text in other languages.** The playmat always reads card data in English from TCGdex, whatever language the card is printed in, and the reader understands English wording only. A card with no English text falls back to asking the assistant. Supporting other languages would need per-language patterns.
- **Applying the player's side of the assistant's cards.** When the assistant plays Judge on its own turn, the player still does their part on the board, as today.
- **Keeping the wait alive across an undo or rewind.** An undo or rewind still ends the assistant's wait, as today. A later change can let a session survive an undo that stays within the turn.
- **A browser (Playwright) test of the decision hold.** The harness cannot drive a wait; the hold is covered by Go tests and a manual check.
- **Per-decision dependency precision.** Every decision holds the same set of dependent actions. A finer dependency graph per decision is left for later if the holds prove too broad in practice.

## Changelog

### 2026-11-07 — Task: Effect reader for what a card asks of the opponent

**What was done**: Added `internal/game/effects.go`, a pure reader that sorts an effect text into none, apply, choice or unknown. For the shuffle-hand-and-draw family it returns the hand destination, the opponent's draw (fixed or per prize), the bottom-of-deck draw condition and the use condition. A strict sentence grammar, kept as one commented pattern table, accepts a text as apply only when every sentence matches. `NamesOpponent` is the looser check for the board's hold. Table-driven tests cover the real TCGdex wording of the family, choice, unknown and none cards, and near misses.

**Deviations**:
- Before this task, the uncommitted wait changes on main were copied into the worktree as a patch for six files (`cmd/tcgreplay/main.go`, `cmd/tcgreplay/timing_test.go`, `internal/game/presence.go`, `internal/mcpserver/server.go`, `internal/mcpserver/wait.go`, `internal/mcpserver/wait_test.go`), as the user chose. Main still holds its own uncommitted copy, which must be committed or discarded before the final merge.
- The goal check first failed criterion 6, reading an uncheckable use condition as an "extra sentence". At the user's decision the plan's behaviour stands (such a card still applies, with no warning), and criterion 6 was clarified in plan.md to say so.

**Files changed**:
- `internal/game/effects.go`
- `internal/game/effects_test.go`

**Discoveries**:
- TCGdex separates a card's use condition from its effect inconsistently: "\n\n" for Special Red Card and Unfair Stamp, a single "\n" for Roxanne. The reader collapses all whitespace before splitting on sentence ends.
- `ReadEffect` returns `Condition` for any text that opens with "You can use this card only if …", including non-family cards such as Ultra Ball (`CondUnrecognised`). Callers should read `Condition` only when `Kind == OppApply`.
- `opponentIsSubject` skips possessives ("your opponent's") and comparisons ("than your opponent"). Any other non-possessive mention counts as the opponent acting, so a passive rule such as Deep Zone's "Each player can have 8 Pokémon on their Bench" reads as unknown and will cost one stop.

### 2026-11-07 — Task: Apply family effects to the opponent inside the card

**What was done**: When the human plays a Trainer, uses an Ability or attacks with a shuffle-hand-and-draw effect against the assistant's side, the opener's own transaction now finishes with the opponent's part: its hand goes into its deck (shuffled) or onto the bottom (random order), then it draws the card's count, all on the opponent's seat under the system actor inside the card's action. This happens always in play mode, and in review mode only for the seat a turn wait has handed to the player. An unmet use condition (Unfair Stamp's knockout, Special Red Card's prize count) still applies and adds a warning. `appliedSince` lists the applied effects after a cursor for the wait.

**Deviations**:
- Moved forward from task 4: `Manager.Wait` now hands the seat over (`setHanded(req.Seat)`, set before the call is registered) and clears it on ended, game over and turn ended. The goal check required a review-mode handover "by a turn wait", which needed this wiring. Task 4 now only adds the applied reporting.
- `TestOpeningTurnWarnings` (`internal/game/game_test.go`) played the fixture Iono `tst-4` as the human in play mode, which now rewrites p2's hand. Its p1 Supporters were swapped to a draw-only test def so it still checks only turn warnings. That file was outside the task's listed scope.

**Files changed**:
- `internal/game/applied.go` (new)
- `internal/game/applied_test.go` (new)
- `internal/game/game.go`
- `internal/game/reads.go`
- `internal/game/verbs.go`
- `internal/game/presence.go`
- `internal/game/game_test.go`

**Discoveries**:
- `tx.emit` always stamps the transaction's actor and action, and `t.move` stamps the transaction's seat. Opponent events are emitted with `t.emit(opp, …)` under a temporarily swapped `t.actor`.
- `nameOf` takes `g.mu`, so calling it inside a transaction function deadlocks; use `t.st.Name`.
- Test fixtures that swap a card's Def/Name on `g.st` revert on undo or checkout, because both refold the state from the log. Tests that undo must re-apply the swap.
- The cardstest fixture Iono (`tst-4`) is a family card, so any human play-mode use of it now changes p2's hand. Watch for this in later tests.
- If the assistant stops polling mid-turn, the handed seat stays set until a mode switch, a branch move or the next wait ends it, so review-mode cards keep applying until then.

### 2026-11-07 — Task: Hide opponent cards in board move results

**What was done**: The board's reply to each player request now carries a delta redacted for the human seat with `DeltaFor`, as the MCP side already does for the assistant, so cards hidden from the player (the opponent's hand and the draws an applied card makes it take) never reach the browser by name. With the reveal view on in review mode the delta stays unredacted, matching the board. If redaction fails the delta is dropped rather than leaked.

**Deviations**: None. `do` hands the resolved game to `dispatch` through an unexported `game` field on the response rather than a new return value. During verification one `go test -race ./internal/webui/` run reported FAIL without a captured test name; 14 further race runs (including `-count=8`) all passed, so it was treated as a pre-existing intermittent flake.

**Files changed**:
- `internal/webui/actions.go`
- `internal/webui/webui_test.go`

**Discoveries**:
- The board websocket's state pushes in review mode exceed coder/websocket's default 32 KiB client read limit; tests that read them must call `SetReadLimit`.
- `go test -race ./internal/webui/` can fail intermittently; a single failure was not reproducible in 14 reruns.

### 2026-11-07 — Task: Tell the assistant about applied effects in the wait

**What was done**: Every wait outcome now carries `Applied`, the effects the playmat applied to the assistant's side that no earlier result reported (the session remembers them by action id). They ride on whatever the call returns, `still_waiting` included, and never end the wait. A turn-ended outcome lists the whole turn's. Cards whose effect was applied are left out of the cards reported for the assistant to answer, so Judge and its family no longer stop the wait. `wait_for_turn` renders an `applied` list (`iid`, `name`, `card`, `what`), and its turn-ended `next` adds "The playmat applied its side of: <names>; see applied."

**Deviations**:
- The handed-seat wiring planned here (`setHanded` in `Manager.Wait`) was already done in task 2, so this task added only the reporting.
- `TestResponseWindowOnlyWhileOpen` and `TestAssistantJoinNeedsResponseWindow` also used the fixture Iono to open the response window; it now applies itself, so they switched to Eri (`sv05-210`, real wording) via a new `eriCard` helper, as the lock tests did. The unused `shuffleHandAndDraw` helper was removed.
- `internal/game/applied_test.go`'s `TestAppliedReviewHandedByWait` (from task 2) was updated to expect the review-mode Judge reported in `applied` rather than a respond outcome; it is the review-mode coverage for criterion 3. That file was outside this task's listed scope.

**Files changed**:
- `internal/game/presence.go`
- `internal/mcpserver/wait.go`
- `internal/game/presence_respond_test.go`
- `internal/game/presence_lock_test.go`
- `internal/game/applied_test.go`
- `internal/mcpserver/wait_test.go`

**Discoveries**:
- MCP server tests run on a manager with no card source (`game.NewManager(dir, nil)`), so no card text is known and nothing applies there. A test that needs applied effects through MCP builds its own manager on a `cardstest.FakeDB` (see `TestWaitForTurnReportsAppliedJudge`).
- During a turn the session cursor stays at the turn's start (every mid-turn outcome returns `Next = cursor`), so `appliedSince(cursor)` on turn end covers the whole turn without a separate turn-start lookup.
- `LogLine.Seat` is a string, not a `state.Seat`.

### 2026-11-07 — Task: Wait stops only for decisions

**What was done**: The wait's per-card `respond` and separate `promote` outcomes became one `decide` outcome with `Action` `promote` (with `Choices`, the assistant seat's Bench slots) or `answer_card` (with `Cards`, each marked `choose` or `read`). Promotion is judged first, once per vacancy and only when a Bench exists (an empty Active with no Bench is game over). Each unreported human opener is classified by `cardNeed`: an uncached card or a Trainer with no text needs reading; otherwise the effect reader decides (asks nothing → never stops; a choice → `choose`; anything else, including a family card that was not applied → `read`). Every opener is still marked reported, so none is re-examined. The renderer temporarily maps `decide` onto the old promote and check texts, with `action` `promote` and `answer_card`.

**Deviations**:
- The board test client's websocket read limit (32 KiB by default) was raised in the shared `dial` helper (`internal/webui/webui_test.go`, outside the task's listed scope). Some state pushes exceeded it, which failed `TestLockReleasesAfterFinish` at random once `startPlay` gave p2 a Bench, and very likely caused the unexplained race-run failure in task 3. The per-test override from task 3 was removed.
- The webui `lockByCard` helper became `lockByKnockOut`, and the unused `trainerInHand` helper was removed.

**Files changed**:
- `internal/game/presence.go`
- `internal/mcpserver/wait.go`
- `internal/game/presence_respond_test.go`
- `internal/game/presence_lock_test.go`
- `internal/webui/lock_test.go`
- `internal/webui/webui_test.go`
- `internal/mcpserver/wait_test.go`

**Discoveries**:
- MCP tests built on `connect` have no card source, so every card there is unknown and needs reading: cards still stop the wait in those tests.
- A card definition missing from the source is not remembered as missing: `g.card` asks the source's cache on every call, so a def cached later is judged at report time. The background `warm` fetch reads the test FakeDB map concurrently, so tests must not write the map while a warm is in flight (wait for `g.warming` to clear).

### 2026-11-07 — Task: Decision results tell the assistant exactly what to do

**What was done**: `wait_for_turn` renders `decide` outcomes with a one-sentence `decision` naming the assistant's seat, `choices` (Bench slot, iid, name) for a promotion, and `need` (`choose` or `read`) on each card. Each decide `next` gives numbered steps: act on the assistant's seat, do nothing else, then hand the turn back with the exact `wait_for_turn` call for the player's seat and cursor, "and keep calling it until the turn ends". It says the player keeps playing meanwhile, never that the board is locked. Applied effects are named in the decide, still-waiting, turn-ended and ended texts, and still-waiting and check-in now also say to keep waiting until the turn ends. The `wait_for_turn` description and the standing instructions describe applied effects and decision stops, keeping their loop lines.

**Deviations**:
- The goal check (attempt 1) found that a wait ending early (mode switch, game closed or expired, branch switch, rewind) dropped applied effects not yet reported. The fix went into `internal/game/presence.go` and `internal/game/applied.go` (outside the listed scope): the ended path now reports them via `appliedAfter` and a shared `unreportedLocked`, with `TestEndedWaitCarriesApplied` in `internal/game/presence_respond_test.go`.
- The plan asked to keep the still-waiting and check-in texts byte-identical. The goal check (attempt 2) required every result to say to keep waiting until the turn ends, so both texts gained that clause and their pins in `wait_test.go` were updated.
- Took three goal-check attempts.

**Files changed**:
- `internal/mcpserver/wait.go`
- `internal/mcpserver/server.go`
- `internal/mcpserver/wait_test.go`
- `internal/game/presence.go`
- `internal/game/applied.go`
- `internal/game/presence_respond_test.go`

**Discoveries**:
- `TestGuideInstructions` (`internal/mcpserver`) fails intermittently under `-race` with "TempDir RemoveAll cleanup: directory not empty" (8 of 30 runs on the main checkout without this work). Something keeps writing into the manager's temp directory after the test ends. Pre-existing and unrelated; worth its own fix.
- MCP tests that need card text (applied effects, real decide cards) use the new `cardGame` helper in `wait_test.go`: a server on a `cardstest.FakeDB` manager with Judge (`tst-9`) and Eri (`tst-10`) in a TST deck.

### 2026-11-07 — Task: Guides and website describe applied effects and decision stops

**What was done**: The play-game guide's respond/promote line now says the playmat carries out the assistant's side of shuffle-hand-and-draw cards itself (listed in `applied`) and that a `decide` outcome asks the seat to promote from its `choices` or answer a listed card, then hand the turn back in the same reply. The replay-drill and review-game guides replace "answering any card that asks your side to act" with acting on any `decide` outcome, and each gained a sentence that applied cards are listed in `applied` and must not be done again. The website's play-game page gained a "Your opponent reacts as a real one would" paragraph. Guide rule anchors pin the new wording, and `TestNoGuideSaysAnswerEveryCard` fails if any guide says to answer every card or names the old outcomes.

**Deviations**: The first goal check found replay-drill and review-game described decision stops but not applied effects; both gained the applied sentence (replay-drill's as its own bullet, so its pinned rule line stays intact). Two goal-check attempts.

**Files changed**:
- `internal/guides/play-game.md`
- `internal/guides/replay-drill.md`
- `internal/guides/review-game.md`
- `internal/guides/guides_test.go`
- `site/src/content/docs/workflows/play-game.md`

**Discoveries**:
- The website builds into `internal/site/dist/app`, which is git-ignored; a fresh worktree needs `make site` (runs `npm ci` in `site/`) before the build works.
- Guide rule anchors in `guides_test.go` are exact substrings, so inserting text inside an anchored sentence breaks them; add new guidance as its own sentence or bullet.

### 2026-11-07 — Task: Decision hold in the presence

**What was done**: The presence's `acting` flag became a `Hold` of two strengths: `HoldFull` for the assistant's own turn (status "acting", set by a turn-ended outcome or an own-turn move) and `HoldDecision` for a decide outcome (status "deciding", remembering the decision's action). The next wait call clears any hold. The existing release timer lapses either hold to "stopped"; a lapsed decision hold keeps its decision readable through `Manager.Decision()` until the next wait or a game change. `Manager.Locked()` was replaced by `Hold()` and `Decision()`. Mode switches, branch moves, undo, a new game and expiry clear any hold and decision, as before.

**Deviations**: The board's guard (`internal/webui/actions.go`) refuses game actions under either hold (`Hold() != HoldNone`), so the board behaves exactly as before until the next task narrows the decision hold. The plan suggested `Hold() == HoldFull` here, which would have left the board refusing nothing under a decision hold in the meantime.

**Files changed**:
- `internal/game/presence.go`
- `internal/webui/actions.go`
- `internal/game/presence_lock_test.go`
- `internal/mcpserver/lock_test.go`
- `internal/webui/lock_test.go`

**Discoveries**:
- Every new wait call clears the hold before it does anything else, so "the assistant hands the turn back" and "the decision hold ends" are the same event; a promotion alone does not lift it.

### 2026-11-07 — Task: Board holds only actions that depend on the decision

**What was done**: The board's one lock check became `held`, run once the game and acting seat are resolved. Under a full hold (the assistant's own turn) it refuses every request but looking and switching mode, as before. Under a decision hold it refuses only requests that depend on the decision: attack, end turn, checkout and rewind; acting as the deciding seat; a target or destination on its side; one of its cards; a shuffle or effect on it; and a Trainer or Ability whose text names the opponent (`Game.OpenerNamesOpponent`, which counts an unknown card as naming it). The refusal says what the board waits for ("Waiting for your opponent to promote a new Active Pokémon; try again when they have." or "…to answer your card…"). The hub's state message carries `decision` while deciding and after a lapse. The presence now also records the deciding seat (`Manager.Decider`), so review-mode practice holds the right side.

**Deviations**:
- Added `Manager.Decider()` (the deciding seat), not in the plan, so the guard knows which side is deciding when the player practises the other seat in review mode.
- `OpenerNamesOpponent` takes `(verb, iid, ability)`, with the board resolving an Ability's Pokémon to its top card, rather than the plan's `(seat, verb, iid, ability)`.
- `TestOpponentStatusPushed` (unchanged) failed once in about 23 runs with a 5 s timeout waiting for the "acting" push; 15 isolated and 5 whole-package reruns passed here and on the main checkout. Treated as a rare timing flake.

**Files changed**:
- `internal/webui/actions.go`
- `internal/webui/hub.go`
- `internal/game/presence.go`
- `internal/game/effects.go`
- `internal/webui/lock_test.go`

**Discoveries**:
- Ending the turn after a knockout also requires taking the prize first (the game refuses `end_turn` while prizes are owed), so board tests that knock out and then end the turn must `take_prize` in between.
- Promotion of the opponent's Pokémon from the board works with the existing `move` verb to `p2:active`; no new verb is needed once the hold has lifted.

### 2026-11-07 — Task: Board shows the opponent deciding and lets the player promote for it

**What was done**: The board shows an "Opponent is deciding" badge (dashed accent) beside the opponent's name while a decision is pending; controls stay enabled because only "acting" disables them. The store tracks `app.decision` from the state message. When the status goes from "deciding" to "stopped", the board shows a one-time notice that the opponent hasn't answered and the player can carry on and make its move. While the opponent's Active Spot is empty, each of its Benched Pokémon offers "Promote to Active", which sends the existing `move` verb to `<seat>:active`; the server holds it while the opponent is still deciding, and that refusal shows through the existing error toast.

**Deviations**: None. No new Playwright test, as the plan's testing approach records (the browser harness cannot start a turn wait); the existing suite passed (245 passed, 61 skipped).

**Files changed**:
- `web/src/App.svelte`
- `web/src/lib/actions.js`
- `web/src/lib/store.svelte.js`
- `web/src/lib/ws.js`

**Discoveries**:
- A fresh worktree has no `web/node_modules`; `npm ci` in `web/` is needed before `npm run build` or the Playwright suite (which builds the Go board test server itself).
- Only a decision hold lapses through "deciding", so the deciding → stopped transition is a reliable trigger for the lapse notice; a full hold lapses from "acting" and shows no notice.

