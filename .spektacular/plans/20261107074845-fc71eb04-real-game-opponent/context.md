---
created_date: "2026-11-07"
document_status: final
closed_date: "2026-11-07"
---

# Context: 20261107074845-fc71eb04-real-game-opponent

## Current State Analysis

- **Wait.** `Manager.Wait` (`internal/game/presence.go:493-696`) reports `respond` for every Trainer, Ability or attack the human used (`waitView` collects them, `presence.go:347-379`; the mid-turn report is at `:641-668`), and `promote` once per vacancy of the assistant's Active. Both open the response window (`Game.responder`) and set the acting lock (`:558-577`).
- **Renderer.** `waitForTurn` (`internal/mcpserver/wait.go:59-133`) tells the assistant to read every card ("Check needed: …") and says the board is locked for the player until the hand-back. Uncommitted on main: the 15 s cap, `wait`/`waited`, `keep_waiting`, `action` (`check_cards`/`promote`) and numbered steps.
- **Lock.** The presence's `acting` bool (`presence.go:131-139`) makes the board refuse every game write, `end_action`, `checkout` and `rewind` (`internal/webui/actions.go:144-146`, "the opponent is acting; wait for them to finish") until the next wait call, `AssistantDone`, or a 2-minute self-release that marks the opponent "stopped" (`presence.go:275-285`).
- **Opponent effects.** Nothing applies them automatically. The assistant does its side of Judge or Iono with cross-seat draw, shuffle and move verbs inside the response window (`internal/game/game.go:364-381`).
- **Card text.** Card text is available synchronously from the cache (`game.go:484-511`); openers record it as `t.effect` (`internal/game/verbs.go:428-582`).
- **Undo and hidden information.** Undo walks back whole actions (`internal/game/reads.go:610-644`). Hidden information holds in snapshots and logs, but the board's write result carries an unredacted delta (`game.go:423`, `actions.go:103-108`).
- **Board.** It shows "Opponent is waiting / acting / stopped" (`web/src/App.svelte:73-78`), disables drags and end turn while acting (`web/src/lib/store.svelte.js:185-197`), and offers "Promote to Active" only on the player's own Bench (`web/src/lib/actions.js:272-276`).
- **Docs.** Guides tell the assistant to answer every card (`internal/guides/play-game.md:32`, `replay-drill.md:23`, `review-game.md:76`). The website describes no wait behaviour (`site/src/content/docs/workflows/play-game.md:10`).

## Per-Task Technical Notes

All paths are in the `tcgreplay` repo (`/home/nicj/code/github.com/nicholasjackson/tcgreplay`), the only registered repo. Line numbers are from the tree at planning time, which includes the uncommitted wait changes on main; re-locate by symbol if they have drifted.

Requirement → repo and files:
- **No-choice effects apply themselves:** tcgreplay, `internal/game/effects.go` (new), `internal/game/verbs.go`, `internal/game/game.go`.
- **Card conditions stay the player's responsibility:** tcgreplay, `internal/game/effects.go`, `internal/game/verbs.go`.
- **The assistant learns what happened to its side:** tcgreplay, `internal/game/presence.go`, `internal/mcpserver/wait.go`.
- **Undo reverses an applied effect:** tcgreplay, `internal/game/verbs.go`; no change needed in `internal/game/reads.go`.
- **Non-opponent cards never stop; the wait stops when the assistant must decide; a decision stop says what to decide; unrecognised opponent cards still ask:** tcgreplay, `internal/game/presence.go`, `internal/mcpserver/wait.go`.
- **The player keeps playing; a hold never lasts forever; held actions say why:** tcgreplay, `internal/game/presence.go`, `internal/webui/actions.go`, `internal/webui/hub.go`, `web/src/*`.
- **The turn-end summary covers the whole turn:** tcgreplay, `internal/game/presence.go`, `internal/mcpserver/wait.go`.
- **Works in play and in practice:** tcgreplay, `internal/game/game.go`, `internal/game/presence.go`.
- **Applied effects respect hidden information:** tcgreplay, `internal/webui/actions.go`, plus the existing `internal/state/apply.go` rules.
- **Guides and docs:** tcgreplay, `internal/guides/*.md`, `internal/mcpserver/server.go`, `site/src/content/docs/workflows/play-game.md`.

### Task: Effect reader for what a card asks of the opponent

**File changes**
- `internal/game/effects.go` (new): `OppKind`, `OppEffect`, `Condition`, `ReadEffect(text string) OppEffect`, `NamesOpponent(text string) bool`, as in plan.md § Data Structures.
  - **Normalise:** lower-case, collapse whitespace and newlines, read "his or her" as "their", strip a trailing period per sentence. Split into sentences on `. ` and blank lines.
  - **Sentence shapes for apply (all required to match):**
    - (a) condition: `you can use this card only if <cond>`. `<cond>` is either `any of your pokémon were knocked out during your opponent's last turn` → `ko_last_turn`, or `your opponent has <N> or fewer prize cards remaining` → `opp_prizes_at_most`. Anything else → `unrecognised` (still apply).
    - (b) disposal: `(each player|your opponent) shuffles their hand into their deck` → deck, or `(each player|your opponent) shuffles their hand and puts it on the bottom of their deck` → bottom. Either may end with `and draws <count>`, applying to the same subject.
    - (c) draws: optional `then, ` prefix; optional `if (either player|they|your opponent) put any cards on the bottom of their deck in this way, ` (→ `either_put` / `opponent_put`); then one of `each player draws <count>`, `your opponent draws <count>`, `they draw <count>`, `you draw <count>, and your opponent draws <count>`, or `you draw <count>` (the player's own; not applied).
    - `<count>`: `<digits> cards?`, `a card` (1), or `a card for each of their remaining prize cards` (PerPrize).
    - The disposal subject must include the opponent, and the opponent must end with a draw (from (b) or (c)).
  - **Choice:** a sentence whose subject is `your opponent`, `each player` or `both players` (not followed by `'s`) and that contains `choose`, `chooses`, `of their choice`, `switches their active` or `switch their active` → `OppChoice`.
  - **Unknown:** any other sentence with the opponent or each player as subject, or empty text passed as "unknown card". Use a distinct entry point, or treat `text == ""` as none and let callers pass unknown separately. Prefer `ReadEffect("")` → `OppNone`, and have callers handle an unknown card (nil definition) as unknown themselves.
  - **None:** otherwise. Possessive `your opponent's …` alone is none. The reader is English-only: card data always comes from TCGdex `/v2/en` (`internal/cards/cards.go:205`), so callers never pass another language.
  - `NamesOpponent`: true if the normalised text contains `opponent` or `each player` or `both players`.
  - Keep the patterns in one table with a comment quoting the card each was written for.
- `internal/game/effects_test.go` (new): table-driven over the real wording recorded in research.md § Chosen approach. That covers Judge `sv01-176`, Special Red Card `me04-082`, Iono `sv02-185`, Unfair Stamp `sv06-165`, Red Card `xy1-124`, Reset Stamp `sm11-253`, Roxanne `swsh12.5gg-GG66`, Marnie `swshp-SWSH121`, Eri `sv05-210`, and the Boss's Orders, Premium Power Pro, Dragapult ex and Mega Absol ex texts from the local cache. Add an Escape Rope-style text ("Each player switches their Active Pokémon with 1 of their Benched Pokémon."), the old fixture Iono `tst-4`, Ultra Ball, empty text, and Judge plus an extra sentence.

**Complexity**: Medium
**Token estimate**: ~25k tokens
**Agent strategy**: Single agent, sequential: write the table of cases first, then the reader until they pass.

### Task: Apply family effects to the opponent inside the card

**File changes**
- `internal/game/game.go:83-87`: beside `responder`, add `handed state.Seat` (the seat a turn wait has handed to the player).
  - `internal/game/game.go:478-482`: add `setHanded(seat)`, modelled on `setResponder`.
  - Reset `handed` wherever `responder` is reset: `SetMode` (`internal/game/game.go:173`), `checkoutLocked` (`internal/game/reads.go:718`). Do not reset it in `EndTurnWith`: the presence clears it when the session ends.
- `internal/game/game.go:196-207`: no new field is needed. The helper switches `t.actor` to `state.ActorSystem` for its emits and restores it afterwards.
- `internal/game/verbs.go` (new helper, near `tx.mulligan` at :94-108): `func (t *tx) applyOpponent(opp state.Seat, eff OppEffect, playerHandAfter int) error`:
  1. Snapshot `hand := slices.Clone(t.st.Sides[opp].Hand)`.
  2. **Deck:** emit `EvMove` for each card, hand → deck, with `t.emit(opp, …)` (not `t.move`, which stamps the tx's seat, `game.go:235-241`), then `t.shuffle(opp)`.
  3. **Bottom:** permute `hand` with `t.g.rng`, then emit each card hand → `deck` with `Pos: &state.Bottom` (`internal/state/types.go:86-89`).
  4. Draw count: `eff.Draw`, or `t.st.Sides[opp].PrizesLeft()` when `PerPrize`. Skip the draw when `DrawIf == "opponent_put"` and `len(hand)==0`, or when `DrawIf == "either_put"` and `len(hand)==0 && playerHandAfter==0`.
  5. `t.draw(opp, n)` (`game.go:243-264`).
- `internal/game/verbs.go` (new helper): `func (g *Game) autoOpponent(seat state.Seat, actor state.Actor) bool`, called with `g.mu` held. It returns `actor == state.ActorHuman && (g.mode == ModePlay || g.handedLocked() == seat)`.
- `internal/game/verbs.go:428-454` `PlayTrainer`, `:458-483` `UseAbility`, `:551-582` `Attack`: after the card's own moves inside `fn`, if `t.effect != nil && g.autoOpponent(seat, actor)`, call `ReadEffect(*t.effect)`. If the kind is `OppApply`:
  - check the condition, adding `t.warn` "<card>'s condition is not met (…); its effect was applied anyway";
  - call `t.applyOpponent(seat.Other(), eff, len(t.st.Sides[seat].Hand))`.
  - In `PlayTrainer`, restructure the Stadium early `return` so the hook also runs for Items and Supporters. Stadiums never match the family.
- `internal/game/verbs.go:288-341` `prizesOwedLocked`: factor the knockout scan into `func (g *Game) knockedOutDuring(st *state.State, victim state.Seat, turn int) bool`. Use it for `ko_last_turn` with `victim = seat` and `turn = st.Turn-1`. `opp_prizes_at_most` uses `st.Sides[seat.Other()].PrizesLeft() <= N`.
- `internal/game/presence.go` (or a new `internal/game/applied.go`): `func (g *Game) appliedSince(cursor int, seat state.Seat) []AppliedEffect`, with `g.mu` held. It walks `g.eventsSince(cursor)`, groups `ActorSystem` events whose action is an opener of `seat` with `Actor == ActorHuman`, and builds `What`, for example "p2's hand of 6 cards was shuffled into p2's deck; p2 drew 4 cards" or "… was put on the bottom of p2's deck …". It counts hand→deck moves and draws, and takes the label from `g.log.Action(id).Label`.
- `internal/game/applied_test.go` (new), using `respondGame` (`internal/game/presence_respond_test.go:22-53`): extend the FakeDB with Special Red Card, real-wording Iono and Unfair Stamp. Assert:
  - the counts, hand destinations and system actor;
  - the condition warnings;
  - undo via `g.Undo(1)` restores both hands and decks (compare `State()` before and after);
  - `Project` for `state.P1` hides p2's hand, and `g.Log(since, state.P1, …)` shows "(hidden)" for the draws;
  - an assistant-actor Judge does nothing to p1;
  - review mode applies only after `setHanded`.

**Complexity**: High
**Token estimate**: ~50k tokens
**Agent strategy**: Parallel analysis, sequential integration. Read `runLocked`, the openers and `prizesOwedLocked` first, then implement the helper, the eligibility check, the hook and the applied read in order, testing each.

### Task: Hide opponent cards in board move results

**File changes**
- `internal/webui/actions.go:103-108`: replace `res.Delta = gr.Delta` with `g.DeltaFor(gr, viewer)` (`internal/game/reads.go:165-195`), where `viewer` is `s.Human` unless the client's reveal view is on in review mode (`c.reveal`, `internal/webui/actions.go:154`). In that case keep the unredacted delta. Model on `Server.redact` (`internal/mcpserver/server.go:980-993`). The game handle is resolved in `do`, so return it to `dispatch` or redact inside `do` before returning `gr`.
- `internal/webui/webui_test.go`: add a test. In play mode, a board write that moves p2 cards into p2's hand returns a delta with "(hidden)" and no p2 card names. With reveal on in review mode, the names appear. The applied Judge from the previous task is the natural trigger once it lands. Until then, use a review-mode `move` for p2 with reveal off.

**Complexity**: Low
**Token estimate**: ~10k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Tell the assistant about applied effects in the wait

**File changes**
- `internal/game/presence.go:493-556` `Wait`: after creating a fresh session, call `g.setHanded(req.Seat)`. Clear it with `setHanded("")` wherever the session is dropped: turn ended (:629-638), ended (:592-599), game over (:609-617), and the game-closed path (:500-513). The game's own resets cover mode and branch changes.
- `internal/game/presence.go:151-170` `waitSession`: add `appliedReported map[string]bool`.
- `internal/game/presence.go:326-379` `waitView`: add `applied []AppliedEffect` from `g.appliedSince(cursor, seat)`, and `appliedIDs`. When collecting `used`, skip openers that have applied events.
- `internal/game/presence.go:71-94` `WaitOutcome`: add `Applied []AppliedEffect`. In the loop (:583-695), compute the unreported applied effects each pass, mark them reported, and set `out.Applied` on every return path: still_waiting, respond/promote, check_in. Turn ended gets all of the turn's applied effects (`appliedSince` from the turn-start cursor), and ended or game over gets the unreported ones. An applied effect alone does not end the loop.
- `internal/mcpserver/wait.go:15-44`: add `Applied []appliedEffect` to `waitResult` with JSON `applied,omitempty`, and an `appliedEffect{IID, Name, Card, What}` struct. Fill it in `waitForTurn` (:85-95). On turn ended, append " The playmat applied its side of: <names>; see applied." to `next` when non-empty.
- `internal/game/presence_respond_test.go:164-201` `TestWaitRespondJudgeAndUnfairStamp`: rewrite. The Judge applies itself, the wait does not stop, and the next still_waiting result lists the applied effect; Unfair Stamp the same. Drop `shuffleHandAndDraw` (:85-96) if it is now unused.
- `internal/game/presence_lock_test.go:56-90` `TestOutcomesSetActingLock` and `:128-160` `TestWaitingAgainReleasesLock`: their "respond" cases played a Judge. Switch them to an unrecognised opponent card (an Eri-style text in the FakeDB) so they still reach the respond outcome until the next milestone.
- `internal/webui/lock_test.go:119-133` `lockByCard`: it plays an Ultra Ball and expects respond. Leave it until the next task changes it, since Ultra Ball still responds in this milestone.
- `internal/mcpserver/wait_test.go`: add a case. The player's Judge, played through the game as the human, shows in the next result's `applied`, and the `log` names Judge.
- A review-mode test: hand p1 over with a wait in review mode, play Judge as the human for p1, and assert p2's hand changed and was reported.

**Complexity**: Medium
**Token estimate**: ~30k tokens
**Agent strategy**: Single agent, sequential: presence first, then the renderer, then the tests.

### Task: Wait stops only for decisions

**File changes**
- `internal/game/presence.go:42-50`: replace `WaitRespond` and `WaitPromote` with `WaitDecide = "decide"`, and add `DecidePromote = "promote"` and `DecideAnswerCard = "answer_card"`. Add `Action`, `Choices []BenchChoice` and `Need` on `UsedCard` (:96-102), as in plan.md § Data Structures.
- `internal/game/presence.go:105` `openerKinds`: keep the kinds. Classification now happens per card.
- `internal/game/presence.go:347-379` `waitView`: for each unreported human opener of `seat`, classify:
  - no card definition (text unavailable), or a Trainer whose `Effect` is empty (no English text in TCGdex) → need `read`. An attack with empty text stays none, since damage-only attacks have no text;
  - `ReadEffect(text)`: `OppNone` → skip; `OppApply` with applied events → skip (reported as applied); `OppApply` without applied events → `read`; `OppChoice` → `choose`; `OppUnknown` → `read`.

  Also add `bench []BenchChoice`, the other seat's Bench stacks with slot, top iid and name, read from `g.st.Sides[seat.Other()].Bench`.
- `internal/game/presence.go:641-668`: promotion first. If `v.activeEmpty && !s.koReported && len(v.bench) > 0`, return `WaitDecide` with `Action = DecidePromote` and `Choices = v.bench`. An empty Active with no Bench is the game-over path, already handled above at :609-617. Otherwise, if any classified cards are unreported, return `WaitDecide` with `Action = DecideAnswerCard` and `Cards`. Keep marking reported ids for every human opener, including skipped ones, so none is re-examined. Keep `setResponder(s.seat.Other())` for both actions.
- `internal/game/presence.go:558-577`: the deferred lock switch uses `WaitDecide` in place of `WaitRespond, WaitPromote` (behaviour unchanged until the hold task).
- `internal/mcpserver/wait.go:100-116`: temporarily map `WaitDecide` to the existing two texts by `Action`, so the build passes. The next task rewrites them.
- Tests:
  - `internal/game/presence_respond_test.go:98-160` `TestWaitPromote`: expect `WaitDecide` with promote and the Bench choices, and expect no card report after the promotion for a damage-only attack.
  - `:203-250` `TestResponseWindowOnlyWhileOpen`: Ultra Ball no longer stops. Use an unknown-opponent card to open the window.
  - `:253-288` `TestAssistantJoinNeedsResponseWindow`: use a card that still stops, such as the old fixture Iono `tst-4` without applied events played by the human in review mode, or an Eri-style card. Keep the cross-join assertions.
  - `internal/game/presence_lock_test.go:56-90`: the promote case's `Kind` becomes `WaitDecide`.
  - `internal/webui/lock_test.go:119-133` `lockByCard`: switch to a knockout of p2's Active with a p2 Bench, expecting `game.WaitDecide`. Give `startPlay` (:22) a p2 Bench, modelled on `waitGameBench` in `internal/mcpserver/wait_test.go:53`.
  - New tests:
    - a scripted ordinary turn (Ultra Ball, attach, evolve, damage-only attack) gives only still_waiting until `EndTurn`;
    - a choice card gives one answer_card with need `choose`;
    - an Eri-style card gives `read`;
    - an uncached family card gives `read`; simulate by setting the iid's def to one missing from the FakeDB at play time, then adding it before the wait evaluates;
    - a last-Pokémon knockout gives ended `game_over`;
    - a mixed turn (ordinary cards, Judge, one unknown card, a knockout) gives exactly two decide outcomes (success metric).

**Complexity**: High
**Token estimate**: ~45k tokens
**Agent strategy**: Parallel analysis, sequential integration: the presence change first, then update the existing tests file by file, then add the new tests.

### Task: Decision results tell the assistant exactly what to do

**File changes**
- `internal/mcpserver/wait.go:15-44`: `waitResult` gains `Decision string` (`decision,omitempty`), `Choices []benchChoice` (`choices,omitempty`; `{slot, iid, name}`) and the per-card `need` on `usedCard`. The `Action` comment lists `promote | answer_card`.
- `internal/mcpserver/wait.go:96-131`: replace the respond and promote cases with `case game.WaitDecide`, branching on `o.Action`.
  - **promote:** `Decision` = "Your Active Pokémon (<seat>) was Knocked Out: choose a Benched Pokémon to promote." `next` = "Decision needed for your seat <mine>. 1. Promote: call switch with seat <mine> and to_slot set to one of the choices (slots <list>). 2. Do nothing else: do not take prizes, end the turn or play cards, it is still the player's turn. 3. Hand the turn back: call wait_for_turn with seat <seat> and since <n>, in this same reply. The player keeps playing meanwhile. Post nothing in chat."
  - **answer_card:** `Decision` names the cards. `next` = "Decision needed for your seat <mine>: the player used the cards below, which involve your side. 1. For each card: if need is choose, make the choice it leaves you with your seat's verbs (seat <mine>); if need is read, read its text and do exactly what it asks your side to do, or nothing if it asks nothing. 2. Do nothing else… 3. Hand the turn back: call wait_for_turn with seat <seat> and since <n>, in this same reply. Post nothing in chat."
  - Remove "the board is locked for them" and "That unlocks the board". Keep the still_waiting, check-in and ended texts byte-identical.
- `internal/mcpserver/server.go:747`: append to the `wait_for_turn` description: "Cards like Judge or Iono happen on your side by themselves and are listed in applied; outcome decide means your seat must act now (promote or answer a card), then call this again."
- `internal/mcpserver/server.go:67-76` instructions: add one line after :76. "During the player's turn the playmat applies shuffle-hand-and-draw cards (Judge, Iono and the like) to your side itself; wait_for_turn stops only when your seat has a decision (a promotion or a card it cannot apply), and its next says what to do." Keep :72's own-card sentence and the pinned loop lines.
- `internal/mcpserver/wait_test.go:390-448` `TestWaitForTurnRespondAndPromote`: rename it to `TestWaitForTurnDecisions`, and pin:
  - the promote result's `outcome:"decide"`, `action:"promote"`, `choices` equal to the Bench, `next` containing "Decision needed for your seat p2", "call switch with seat p2 and to_slot", "call wait_for_turn with seat p1 and since %d, in this same reply", and not containing "locked";
  - an answer_card result, from an Eri-style card in the FakeDB, with `cards[0].need == "read"`;
  - `keep_waiting` true on both.

  Add a check that no wait `next` text contains "every card" or "Check needed". `TestInstructionsTeachTheLoop` (:367-388) still passes, plus a pin for the new instructions line.

**Complexity**: Medium
**Token estimate**: ~20k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Guides and website describe applied effects and decision stops

**File changes**
- `internal/guides/play-game.md:32`: replace with "- The playmat carries out your side of shuffle-hand-and-draw cards the player uses (Judge, Iono and the like) by itself; the wait lists them in `applied`. A `decide` outcome means your seat must act now: promote after a Knock Out (pick from its `choices`), or answer a card it lists. Decide it on `get_state` view `seat` only, as above, then call `wait_for_turn` in the same reply."
- `internal/guides/replay-drill.md:23`: replace "answering any card that asks your side to act" with "acting on any `decide` outcome (a promotion or a card the playmat could not apply) and handing the turn back in the same reply". Keep the pinned sentences at :22 and :23's tail exactly.
- `internal/guides/review-game.md:76`: the same rewording of "answering any card that asks your side to act". Keep the pinned anchors at guides_test :233-236.
- `internal/guides/guides_test.go:225-241` `TestGuidesKeepTheirRules`: add anchors:
  - play-game: "A `decide` outcome means your seat must act now";
  - replay-drill and review-game: "acting on any `decide` outcome".

  Add a test that no guide contains "answering any card" or "respond` or `promote".
- `site/src/content/docs/workflows/play-game.md:10`: add a short paragraph after it. "Your opponent reacts as a real one would. When you play a card like Judge or Iono, its hand is shuffled away and it draws by itself, with no need to tell it. It only stops to think when it has a real choice, such as promoting after a Knock Out, and you can keep playing anything that doesn't depend on that choice."
- No guide may contain "Claude" or "Nic " (convention; enforced by `internal/guides/guides_test.go`).

**Complexity**: Low
**Token estimate**: ~12k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Decision hold in the presence

**File changes**
- `internal/game/presence.go:131-144`: replace `acting bool` with `hold Hold` (`HoldNone`, `HoldFull`, `HoldDecision`), and add `decision string` (the pending or lapsed decision's action). `stopped`, `lastMove`, `lockRelease` and `releaseTimer` are unchanged.
- `internal/game/presence.go:253-271`: `startActingLocked(release)` becomes `startHoldLocked(h Hold, decision string, release time.Duration)`. `clearActingLocked` becomes `clearHoldLocked`, which also clears `decision`.
  - `releaseFired` (:275-285): sets `hold = HoldNone` and `stopped = true`, and keeps `decision` when the lapsed hold was a decision (the board's notice).
- `internal/game/presence.go:304-314` `opponentLocked`: `HoldFull` gives "acting", `HoldDecision` gives "deciding", then "stopped" and "waiting" as today.
- `internal/game/presence.go:440-455`: replace `Locked()` with `Hold() Hold`. Add `Decision() string`. Keep `Opponent()`.
  - `AssistantMoved` (:461-472): refresh the clock under either hold. An own-turn write sets `HoldFull`.
  - `AssistantDone` (:476-485): clears any hold.
- `internal/game/presence.go:558-577`: `WaitTurnEnded` gives `startHoldLocked(HoldFull, "", …)`, and `WaitDecide` gives `startHoldLocked(HoldDecision, out.Action, …)`.
- `internal/game/presence.go:554`, `:198`, `:214`, `:227-228`: call `clearHoldLocked` (behaviour unchanged).
- Update every caller of `Locked()`: `internal/webui/actions.go:144`, which the next task rewrites; here, make it compile with `s.M.Hold() == game.HoldFull`. Grep the tests for `Locked()`: `internal/game/presence_lock_test.go:24` `wantPresence`, `internal/mcpserver/lock_test.go:16` `wantLock`, `internal/webui/lock_test.go`.
- `internal/game/presence_lock_test.go`: `TestOutcomesSetActingLock` (:56-90) expects `HoldDecision` and "deciding" for decide, and `HoldFull` and "acting" for turn ended. Add a test that a decision hold lapses to "stopped" with `Decision() == "promote"`, and that the next wait clears it. `TestGameChangesClearLock` (:361) covers decision holds too. `TestOnPresenceHook` (:284) fires on the deciding status.

**Complexity**: Medium
**Token estimate**: ~25k tokens
**Agent strategy**: Single agent, sequential: rename and widen, fix callers, then the tests.

### Task: Board holds only actions that depend on the decision

**File changes**
- `internal/webui/actions.go:112-120`: keep `lookVerbs` and `errOpponentActing` (full hold). Add a decision-hold refusal built from `s.M.Decision()`:
  - promote: "Waiting for your opponent to promote a new Active Pokémon; try again when they have."
  - answer_card: "Waiting for your opponent to answer your card; try again when they have."
- `internal/webui/actions.go:144-146`: replace the check with `if err := s.held(g, seat, req, a); err != nil { return nil, nil, err }`. `held` behaves as follows:
  - `HoldFull`: refuse all except `lookVerbs` (as today).
  - `HoldDecision`: refuse `attack`, `end_turn`, `checkout` and `rewind`.
  - Refuse any verb whose `Target` or `To` parses (`state.ParseLoc(spec, seat)`) to `seat.Other()`: evolve, attach, knock_out, set_condition, place_damage, move.
  - Refuse `move` whose `IID` is owned by `seat.Other()`, `shuffle` or `add_effect` with `Whose` set to the other seat, and `play_trainer` / `use_ability` when `g.OpenerNamesOpponent(seat, verb, iid, name)` is true.
  - Allow everything else, including `end_action`.

  This must run after the game is resolved; today the check sits before `g` is used, so reorder.
- `internal/game/presence.go` or `internal/game/effects.go`: `func (g *Game) OpenerNamesOpponent(seat state.Seat, verb string, iid state.IID, ability string) bool`.
  - It takes `g.mu` and gets the card with `g.card(g.st, iid)`, or, for `use_ability`, the Pokémon at the given slot and the named Ability.
  - It returns `NamesOpponent(text)`, and true when the card is unknown (safe).
- `internal/webui/hub.go:147-151`: set `msg["decision"] = h.s.M.Decision()` when it is not empty, next to `opponent`. Update the comment.
- `internal/webui/lock_test.go`: give `startPlay` (:22) a p2 Bench. Add `TestDecisionHoldAllowsIndependentActions`:
  - knock out p2's Active from the board under `waitBG` (:65), then `waitOutcome(t, done, game.WaitDecide)`;
  - attach Energy, evolve and an own-side Item (Ultra Ball "tst-3") succeed;
  - attack, end_turn, a Boss's Orders-text Trainer and `move` of a p2 bench card to "p2:active" are refused with the promote message;
  - after `g.Switch(P2, ActorClaude, 0)` and a new wait, attack and end_turn succeed.

  Add `TestDecisionHoldLapses`, with a short `LockRelease`: the pushed state shows `opponent:"stopped"` and `decision:"promote"`, then the board's `move` of a p2 bench card to "p2:active" succeeds, and attack succeeds. Keep `TestLockRefusesGameActions` (:141) for the full hold, driven by the assistant's own turn (`AssistantMoved(true)`) instead of a card. Update `actingRefusal` uses accordingly. `TestOpponentStatusPushed` (:289) expects "deciding" after a decide.

**Complexity**: Medium
**Token estimate**: ~35k tokens
**Agent strategy**: Single agent, sequential: the guard function and game helper, then the hub field, then the tests.

### Task: Board shows the opponent deciding and lets the player promote for it

**File changes**
- `web/src/lib/store.svelte.js:17-19`: update the comment to list 'deciding'. Add `app.decision`. `oppActing()` (:185-189) stays `=== 'acting'`, so controls stay enabled while deciding.
- `web/src/lib/ws.js:51`: set `app.decision = msg.decision || ''`. When the previous `app.opponent` was 'deciding' and the new one is 'stopped', `toast("Your opponent hasn't answered. Carry on, and make their move yourself if you need to (for example, promote one of their Benched Pokémon).", 'info')`.
- `web/src/App.svelte:73-78`: add `deciding: 'Opponent is deciding'`. Add a `.opp-status.deciding` style next to `.acting` (:410-411).
- `web/src/lib/actions.js:226-329` `stackMenu`: for a stack on the opponent's Bench (`!mine`, a bench location) while the opponent's Active is empty in the current board, add 'Promote to Active', which calls `act('move', { iid: <top iid>, to: `${opp}:active` })`. Place it near the existing 'Promote to Active' entry (:272-276). Use the store's board state for the emptiness check.
- `web/e2e`: no new Playwright test (see the testing approach's deliberate gap). Run the existing suite to confirm nothing regressed.

**Complexity**: Low
**Token estimate**: ~15k tokens
**Agent strategy**: Single agent, sequential execution.

## Testing Strategy

All tests are Go tests in the existing suite, using each package's helpers (game: `startedManager`, `respondGame`, `startWait`, `recv`, `waitNow`, `wantPresence`, `eventually`, with the `cardstest.FakeDB` and a seeded PCG; board: `setup`, `send`, `startPlay`, `waitBG`, `waitOutcome`; MCP: `forEachTransport`, `waitGameBench`, `waitCall`). Per task:

- **Effect reader:** table-driven unit tests over real TCGdex wording: apply with counts, destinations, draw conditions and use conditions; choice; unknown; none; family plus an extra sentence gives unknown; `NamesOpponent`.
- **Apply family effects:** game integration tests for the counts (4, 3, prizes, 2), bottom placement, the system actor on applied events, condition warnings, undo restoring both sides, the player's view and log hiding the drawn cards, the assistant's own Judge applying nothing, and review mode only when handed over.
- **Hide opponent cards:** a board websocket test that a play-mode result hides the opponent's moved cards and that the review reveal view keeps them.
- **Applied effects in the wait:** presence tests that Judge does not stop the wait and is listed once on the next result (including still_waiting), that turn ended lists the turn's applied effects and its log names the card, and that a review practice turn applies and reports. The MCP renderer shows `applied`.
- **Decisions:** presence tests that an ordinary turn yields only still_waiting, a knockout yields one promote with exact Bench choices, a last-Pokémon knockout yields game over, choice and unknown cards yield `choose` and `read`, an uncached family card yields `read`, and a mixed turn yields exactly two decide outcomes.
- **Decision results:** MCP contract tests for the fields and numbered `next` texts (seat, choices, hand-back call, no "locked"), plus instructions and description pins, and no "every card" / "Check needed" text.
- **Guides and website:** guide rule anchors for the new wording, a test that no guide says to answer every card, the existing "no Claude / Nic" check, and a site build.
- **Decision hold:** presence tests that decide sets a decision hold ("deciding") while turn ended sets a full hold ("acting"), that the next wait releases it, that a lapse goes to "stopped" with the decision remembered, that game changes clear it, and that the hook fires.
- **Board holds:** board tests that under a pending promotion Energy, evolve and an own-side Item succeed while attack, end turn, gust, a move onto the opponent and checkout/rewind are refused with the promote message; that both succeed after the hand-back; that after a lapse the player can promote for the opponent and attack; and that the full hold is unchanged on the assistant's own turn.
- **Board app:** no automated test (no harness route to drive a wait). Run the existing Playwright suite for regressions. Manual checks are listed in plan.md § Testing Approach.

Success metrics 1–3 are behavioural tests in the decisions and board-hold tasks. Metric 4 (a ChatGPT played-out turn) and the board visuals are manual, captured in the implementation test plan.

## Project References

- Spec: `20261107074845-fc71eb04-real-game-opponent`.
- Prior plan: `20261026104139-4e1bb7fa-wait-for-turn` (presence, wait tool, lock, status).
- Knowledge (repo tier, store `tcgreplay`): `gotchas/handoff-in-tool-results.md`, `gotchas/turn-lock-cross-seat-joins.md`, `gotchas/game-listeners-under-lock.md`, `gotchas/chatgpt-tool-discovery.md`, `gotchas/board-delta-unredacted.md` (written during this plan), `conventions/skills-text-and-packaging.md`.
- Design documents: none.
- Repo root: `tcgreplay` at `/home/nicj/code/github.com/nicholasjackson/tcgreplay`.
- Card wording: TCGdex `https://api.tcgdex.net/v2/en/cards/<id>` (ids in research.md).

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

## Migration Notes

- No data migration. Game records saved before this change need not keep working (spec non-goal), and wait outcomes are never stored.
- The wire contract of `wait_for_turn` changes: `respond` and `promote` become `decide`. Guides, instructions and the tool description change in the same milestone, so assistants learn the new outcome from them and from each result's `next`.
- The uncommitted wait changes on main must be kept; this plan builds on them.

## Performance Considerations

- The effect reader runs on short texts, once per opener and once per reported card. Its cost is negligible.
- `appliedSince` and card classification walk the events since the turn's start on each wait evaluation, which is the same order of work `waitView` already does.
- The board's delta redaction replays the current branch (`DeltaFor`) once per player write, as the MCP side already does per assistant write. That is acceptable at playmat log sizes, which are hundreds of events.
