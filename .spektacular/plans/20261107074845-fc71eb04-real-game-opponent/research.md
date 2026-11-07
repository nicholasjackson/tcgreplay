---
created_date: "2026-11-07"
document_status: final
closed_date: "2026-11-07"
---

# Research: 20261107074845-fc71eb04-real-game-opponent

## Alternatives considered and rejected

- **Option B: the wait loop applies the effect after it sees the card.** Rejected: a separate commit after the card (`presence.go:583-695` evaluates outside the game lock). The player's next move can land between the card and its effect, the effect applies only while a wait call happens to be running, and undo would treat the two separately (`reads.go:610-644` groups by action id and adjacency).
- **Option C: the board's dispatch applies it after `play_trainer`.** Rejected: also two commits (`internal/webui/actions.go:232-233` returns after the verb), and the card-text logic would sit in the web layer, away from the game's transaction and card access.

- **Apply the opponent's part from a game listener or from presence after the event lands.** Listeners run with `g.mu` held and cannot call verbs (knowledge `gotchas/game-listeners-under-lock.md`; `internal/game/presence.go:220-241`). A goroutine would make a second, racy commit, and the board would briefly show the card played but the effect missing. Rejected in favour of applying inside the opener's own transaction.
- **Apply with a second `runLocked` call joined cross-seat (the `EndTurnWith` pattern, `internal/game/verbs.go:251-276`).** This works, but makes two commits under one lock, so a failure in the second leaves the card played without its effect. Rejected for one transaction (`tx.mulligan`, `verbs.go:94-108`, already does reveal, move the hand to the deck, shuffle, then draw in one tx).
- **Stamp applied events with the player's actor (`human`).** A plain `t.emit` inherits the tx's actor (`game.go:209-233`). In review mode the human may also move the other seat's cards by hand in the same action, so applied events could not be told apart from manual ones. Rejected: applied events carry `ActorSystem` (already used for engine events: setup `verbs.go:39`, turn start `verbs.go:259`; undo allows it, `reads.go:627-631`).
- **A maintained list of card names (Judge, Iono, ...).** Excluded by the spec's Technical Approach ("by the wording of the effect, not by a maintained list of card names").
- **Stop the wait for any card whose text contains "opponent".** Too noisy. Many attacks and Items only name the opponent's Pokémon as a target or a damage modifier ("do 30 more damage to your opponent's Active Pokémon", Premium Power Pro `me01-124`; "Switch in 1 of your opponent's Benched Pokémon", Boss's Orders `me01-114`; "Put 6 damage counters on your opponent's Benched Pokémon", Dragapult ex `me02.5-160`). Those need nothing from the opponent and break the "ordinary turn, zero stops" metric. Rejected for "the opponent (or each player) is the subject of an action".
- **Defer opponent effects to the end of the turn.** Rejected during the spec (working context): resolution out of order changes the state and the log, and the hand size shows wrong meanwhile.
- **A separate hold mechanism beside the acting lock.** The spec prefers narrowing the existing lock (Technical Approach). Rejected for one presence hold with two strengths: the full lock (the assistant's own turn) and a decision hold (dependent actions only).
- **End the hold as soon as the assistant promotes (Active no longer empty).** The AC says the held actions succeed "once the assistant has promoted and handed the turn back", and the hand-back is a single, already-instrumented event (the next `wait_for_turn` call, `presence.go:554`). Rejected for releasing on the hand-back or on the lock-release timer.
- **Let the player promote the assistant's Pokémon with `switch` from the board.** The board refuses a foreign seat in play mode (`internal/webui/actions.go:129-139`). `move {iid, to: "p2:active"}` is already accepted: `Move` is cross-seat, forces the owner's seat and emits a swap into the empty Active (`verbs.go:716-771`). The board gains a menu entry that sends `move`; no new server verb.

## Chosen approach — evidence

- Opener verbs and the hook point: `PlayTrainer` (`verbs.go:428-454`), `UseAbility` (`verbs.go:458-483`), `Attack` (`verbs.go:551-582`) each run one tx through `g.run` and set `t.effect` to the effect text; `runLocked` stores it as `g.openText` (`game.go:396-400`). The opponent's part can be added at the end of the same `fn`, with the card text already in hand.
- `tx` helpers to reuse: `t.emit(seat, typ, payload)` stamps `Seat` from its argument (`game.go:209-233`); `t.shuffle(seat)` records the resulting order (`game.go:325-329`); `t.draw(seat, n)` warns on an empty deck and honours draws-like-main (`game.go:243-264`). Avoid `t.move` for the other seat's cards: it stamps the tx's own seat (`game.go:235-241`).
- Deck bottom: `state.Loc{Zone: Deck, Pos: &state.Bottom}` (`internal/state/types.go:86-89`).
- Card text is read without blocking: `g.card` returns the cached definition or nil and warms it in the background (`game.go:484-511`); definitions are prefetched at new_game and import (`manager.go:225-229`, `verbs.go:819-838`).
- Prize count: `Side.PrizesLeft()` (`internal/state/state.go:51-59`). Knockouts in the opponent's last turn: the scan pattern in `prizesOwedLocked` (`verbs.go:288-341`): EvMove events of `kind:"knockout"` actions whose `From` is a stack of the player's seat, with `e.Turn == st.Turn-1`.
- Undo by action: `Game.Undo` walks back runs of consecutive events with one action id (`reads.go:610-644`), so applied events inside the card's action undo with it. The board's timeline undo steps back to an event (`StepBackTo`, `reads.go:650-674`), which removes every later event too.
- Hidden information: moves into a hand give only the owner a Known bit; shuffles and deck moves clear them (`internal/state/apply.go:244-315`); `Project` hides unknown hand cards (`internal/state/view.go:127-234`); log lines hide them (`reads.go:17-22, 34-117`). **Gap:** `runLocked` builds `Result.Delta` unredacted (`game.go:423`) and the board's result sends it as is (`internal/webui/actions.go:103-108`). The MCP side already redacts with `DeltaFor` (`internal/mcpserver/server.go:980-993`).
- Wait evaluation lives in one place: `Manager.Wait` (`presence.go:493-696`), with the per-opener report at `presence.go:641-668` and outcome effects on the lock at `presence.go:558-577`. `waitSession` keeps what was already reported (`presence.go:163-166`).
- Rendering lives in one place: `waitForTurn` (`internal/mcpserver/wait.go:59-133`), already carrying `action`, `keep_waiting`, numbered steps and the hand-back line (uncommitted on main; knowledge `gotchas/handoff-in-tool-results.md`).
- The board's lock is checked once: `if s.M.Locked() && !lookVerbs[req.Verb]` (`internal/webui/actions.go:144-146`); refusals reach the player as an error toast (`web/src/lib/ws.js:88-90`). Status reaches the board through the hub's `opponent` field (`internal/webui/hub.go:148-151`), shown as a badge (`web/src/App.svelte:73-78, 178-183`); `oppActing()` drives the disabled controls (`web/src/lib/store.svelte.js:185-197`).
- Ending the turn with an empty Active and a Bench is already refused by the game (`verbs.go:233-246`); the board never sends `force`.
- The response window `Game.responder` gates the assistant's seat during the player's turn (`game.go:83-87, 458-482`; cross-seat rule `game.go:375-377`; knowledge `gotchas/turn-lock-cross-seat-joins.md`). Promotion is `anyTurn` (`verbs.go:536-547`).
- Real card wording (TCGdex, fetched 2026-11-07):
  - Judge `sv01-176`: "Each player shuffles their hand into their deck and draws 4 cards."
  - Special Red Card `me04-082`: "You can use this card only if your opponent has 3 or fewer Prize cards remaining. / Your opponent shuffles their hand and puts it on the bottom of their deck. If they put any cards on the bottom of their deck in this way, they draw 3 cards."
  - Iono `sv02-185`: "Each player shuffles their hand and puts it on the bottom of their deck. If either player put any cards on the bottom of their deck in this way, each player draws a card for each of their remaining Prize cards."
  - Unfair Stamp `sv06-165`: "You can use this card only if any of your Pokémon were Knocked Out during your opponent's last turn. / Each player shuffles their hand into their deck. Then, you draw 5 cards, and your opponent draws 2 cards."
  - Same family, not in the minimum set: Red Card `xy1-124` ("Your opponent shuffles his or her hand into his or her deck and draws 4 cards."), Reset Stamp `sm11-253` ("... draws a card for each of their remaining Prize cards."), Roxanne `swsh12.5gg-GG66`, Marnie `swshp-SWSH121`.
  - Opponent-as-subject cards that are not the family: Eri `sv05-210` ("Your opponent reveals their hand, and you discard up to 2 Item cards you find there."), Mega Absol ex attack (`me01-086`).
- Test fixtures: `cardstest.FakeDB` is a map tests add to (`internal/cards/cardstest/cardstest.go`; its Iono `tst-4` carries an older, shorter wording); `respondGame` swaps card defs to Judge and Unfair Stamp (`internal/game/presence_respond_test.go:14-53`); presence helpers `startedManager`, `startWait`, `recv`, `waitNow` (`internal/game/presence_test.go:16-89`); lock helpers `wantPresence`, `eventually` (`internal/game/presence_lock_test.go:12-46`); board helpers `startPlay`, `waitBG`, `waitOutcome`, `send` (`internal/webui/lock_test.go:22-121`, `internal/webui/webui_test.go:48-105`); MCP helpers `waitGameBench`, `waitCall` (`internal/mcpserver/wait_test.go:46-91`).

## Files examined

- internal/game/presence.go:42-50 — wait outcome kinds; :105 `openerKinds`; :131-144 acting lock fields; :151-170 `waitSession` (reported, koReported); :224-241 listener ends a session on mode/checkout/rewind/undo; :253-285 lock set/clear/self-release; :304-314 `opponentLocked` priority; :347-379 `waitView` collects human openers; :385-412 `usedCard` text per kind; :438-485 `Locked`, `Opponent`, `AssistantMoved`, `AssistantDone`; :493-696 `Wait`, outcome→lock at :558-577, mid-turn report at :641-668.
- internal/mcpserver/wait.go:15-44 — `waitResult` and `usedCard` JSON; :96-131 `next` texts per outcome (respond "Check needed…", promote "Action needed…"); :137-155 `waitLog` view per mode.
- internal/game/game.go:43-60 — `Update` and change kinds; :83-87 responder; :196-264 `tx`, `emit`, `move`, `draw`; :325-329 `shuffle`; :332-349 `actionSpec`; :364-434 `runLocked` (joins, cross-seat rule, unredacted delta at :423, game-over warning); :458-482 `checkTurn`, `setResponder`; :484-511 `card`/`warm`.
- internal/game/verbs.go:39 — setup uses ActorSystem; :94-108 `tx.mulligan` model; :222-276 `EndTurnWith` (empty Active refusal :233-246, two-runLocked merge :251-276); :288-341 `prizesOwedLocked` knockout scan; :344-351 Draw; :428-454 PlayTrainer; :458-483 UseAbility; :536-547 Switch (promote anyTurn); :551-582 Attack; :624-644 KnockOut; :690-694 Shuffle; :716-771 Move (cross-seat, owner seat, swap into empty Active).
- internal/game/reads.go:17-22, 34-117 — `describe` and hidden-card rendering; :120-158 `LogLine`; :165-195 `DeltaFor`; :197-228 `Log`; :426-490 `Warnings`/`turnWarnings`; :610-644 `Undo`; :650-674 `StepBackTo`; :705-725 `checkoutLocked` clears open action and responder.
- internal/state/events.go:11-16 — actor constants; :117-121 `ShufflePayload` order.
- internal/state/state.go:27, 51-59 — prizes, `PrizesLeft`; :129-154 `Winner` (no Pokémon in play loses).
- internal/state/types.go:86-89 — `Top`/`Bottom`; :136-142 seat-prefixed locations.
- internal/state/apply.go:244-315 — Known bits on moves and shuffles.
- internal/state/view.go:127-234 — `Project` hides hands.
- internal/cards/cards.go:27-40, 72-148 — `Card` (Category, TrainerType, Effect, Abilities, Attacks, `Text()`); :499-525 `Cached`.
- internal/cards/cardstest/cardstest.go:22 — test Iono `tst-4`.
- internal/webui/actions.go:14-66 — request/args/response; :86-108 dispatch, unredacted delta; :112-120 `lookVerbs`, `errOpponentActing`; :122-146 seat handling and the one lock check; :148-299 every board verb and its target arguments; :284-296 end_turn `waiting`.
- internal/webui/hub.go:68-75, 139-151, 245-260 — subscribe, state message with `opponent`, read loop.
- internal/webui/server.go:32-33, 41, 67 — `Human` seat, `Limit`, `OnPresence` wiring.
- internal/usergames/board.go, mcp.go — no lock checks of their own; `ResolveMCPQuiet` for the wait.
- internal/mcpserver/server.go:40 — `actor = ActorClaude`; :67-76 instructions; :197-223 `addQuiet`/`addWith` assistant-move notes; :258-279 `verb()` own-turn lock; :726-733 end_turn `AssistantDone`; :747 wait_for_turn description; :926-932 `handOffHint`; :980-993 `DeltaFor` redaction.
- internal/guides/play-game.md:25-39 — the loop; :32 "A `respond` or `promote` outcome asks your seat to act now: Iono's draws, a promotion…" (to rewrite).
- internal/guides/replay-drill.md:22-28; internal/guides/review-game.md:76, :86 — "answering any card that asks your side to act" (to reword).
- internal/guides/guides_test.go:225-241 — pinned rule anchors (none cover the lines that change).
- internal/mcpserver/wait_test.go:367-448 — instructions anchors; `TestWaitForTurnRespondAndPromote` pins respond/promote texts and actions.
- internal/game/presence_respond_test.go:98-288 — promote, Judge/Unfair Stamp respond, response window, assistant join tests (change).
- internal/game/presence_lock_test.go:56-440 — lock per outcome, release, status, hook, game changes (respond/promote cases change).
- internal/webui/lock_test.go:18-431 — `actingRefusal`, `lockByCard` (Ultra Ball expects respond), refused/allowed verbs.
- web/src/App.svelte:73-78, 178-183 — opponent badge texts; web/src/lib/store.svelte.js:17-19, 185-197 — `oppActing`, `canAct`; web/src/lib/ws.js:51, 80-104 — opponent field, refusal toasts; web/src/lib/actions.js:11-14, 226-329, 454-467 — `seatArgs`, `stackMenu` (promote only on own stacks), endTurn toast; web/src/components/Strip.svelte:64-69, Card.svelte:65, Stack.svelte:37 — controls disabled while acting.
- site/src/content/docs/workflows/play-game.md:10, replay-drill.md:12 — the only playing docs; describe no wait behaviour today. site/src/lib/workflows.ts — the site reads only guide frontmatter.
- internal/webui/webtest/cmd/boardtest/main.go:56-62 — browser harness has only `/test/reset`; no way to drive a wait.

## External references

- TCGdex API `https://api.tcgdex.net/v2/en/cards/<id>` — current English wording of Judge, Special Red Card, Iono, Unfair Stamp, Red Card, Reset Stamp, Roxanne, Marnie, Eri; the wording the matcher must accept (Iono and Special Red Card carry an "If … put any cards on the bottom … in this way" clause that the older test fixture lacks).

## Prior plans / specs consulted

- Plan `20261026104139-4e1bb7fa-wait-for-turn` — origin of presence, the acting lock, the respond/promote outcomes, the fixed evaluation order and the "two choke points" rule (board dispatch, tool registration). It deliberately excluded a playmat-side card list; this spec reverses that for the no-choice family only, by the user's decision.
- Spec `20261107074845-fc71eb04-real-game-opponent` — this plan's source.

## Open assumptions

- Every card the player can play is in the card cache by the time it is played (prefetched at new_game/import). If not, the card's text is unknown at play time: nothing is applied, and the wait falls back to asking the assistant. Verify in tests that this fallback stops the wait.
- Card text is always English. The card client's base is `https://api.tcgdex.net/v2/en` (`internal/cards/cards.go:169, 205`), which only tests override, and cards are identified by TCGdex id whatever their printed language. A print with no English entry has no text and falls back to asking. A Trainer with empty text is treated as unreadable (asks the assistant), not as asking nothing.
- TCGdex wording for the family stays within the grammar in this plan (subject "each player" / "your opponent" / "they"; "shuffles their hand into their deck" or "shuffles their hand and puts it on the bottom of their deck"; counts as digits, "a card", or "a card for each of their remaining Prize cards"; "his or her" normalised to "their"). A card outside it falls back to asking.
- Iono's "If either player put any cards…" is judged at play time from the opponent's hand and the player's hand after the card left it.
- Knockouts recorded by `move` rather than `knock_out`, and imported games' knockouts (kind "log"), are not seen by the Unfair Stamp condition check; the check then warns. Accepted: the warning is advisory.
- The board's undo (timeline step back) can split the card from its applied events if the player steps back to a point between them. Undoing the card itself removes both.
- An undo or rewind still ends the assistant's wait (existing behaviour, `presence.go:236-237`); this plan does not change it.


## Drafting assumptions

### Chosen direction: engine-side effect reader, in-transaction application, decision hold (architecture)
- **Decision**: Option A. A pure effect-text reader in the game package sorts opener texts into none, apply, choice and unknown. The opener verbs apply the family's opponent part inside the card's own transaction (system actor, opponent's seat) when the human plays it against the assistant's side. The wait replaces per-card `respond` and `promote` with one `decide` outcome (`promote` or `answer_card`) and reports applied effects on every result. The presence's acting flag becomes a full or decision hold, and the board refuses only dependent actions under a decision hold.
- **Key design decisions**: atomic one-commit application (no gap between the card and its effect, undo by action covers both); whole-text grammar, so a card applies only when every sentence is understood; the existing release timer and status machinery are reused for the decision hold; the board's single lock check stays the only enforcement point; and the board's result delta is redacted for hidden information.
- **Rationale**: Each concern lands at its existing single owner (card text, transaction, wait evaluation, board dispatch), with evidence in research.md.
- **Rejected**: Option B, the wait loop applying the effect after it sees the card (a separate commit that can interleave with the player's next move; applies only while a wait call happens to run; splits undo). Option C, the board's dispatch applying it after `play_trainer` (also two commits; the card-text logic would sit in the web layer). Both are effort Medium against A's Medium-High, but both break the "one action" requirement.

### One `decide` outcome replaces `respond` and `promote` (architecture)
- **Decision**: The wait reports decisions as outcome `decide`, with `action` `promote` (with `choices`, the Bench slots) or `answer_card` (with `cards`, each marked `choose` or `read`). Promotion is reported first; the cards still unreported come together in the next stop.
- **Rationale**: The spec asks for "a stop that names the decision". One outcome with an action field builds on the `action` field just added, and guides and tests change anyway. Batching the cards keeps stops to at most one per decision.
- **Rejected**: Keeping `respond`, whose meaning changes and invites the old "check every card" reading. One stop per card (more stops).

### Review-mode practice applies effects only while a wait has handed that seat over (architecture)
- **Decision**: In play mode the family applies whenever the human plays it. In review mode it applies only when the human acts on the seat a live turn wait has handed to the player. The presence tells the game which seat that is, and clears it when the session ends, the mode changes or the board moves.
- **Rationale**: The spec scopes the behaviour to "wherever the assistant waits on the player's turn". Exploring a line by hand in review mode, with no assistant waiting, should not change the other side by itself.
- **Rejected**: Applying in review mode always (surprising when a player recreates a line by hand). Applying only during a live wait call (it would miss cards played between polls).

### Dependent actions under a decision hold (architecture)
- **Decision**: These are held: attack; end_turn; checkout and rewind; any write whose target or destination is on the opponent's seat or that moves one of its cards; and play_trainer or use_ability whose card text names the opponent at all. Everything else goes through. The same set applies to every decision (promotion or a card answer).
- **Rationale**: This covers the AC (attack and end turn held; Energy, evolving and own-side Items allowed) and the spec's gust example. It stops the player from pre-empting the assistant's choice ("real decisions stay with the assistant") or moving the board under it. It is decidable from the request alone, plus a card-text check.
- **Rejected**: A per-decision dependency graph (precise but complex for little gain while a decision takes seconds). Holding nothing for card answers (the player's attack could act on a state the assistant is about to change).

### The decision hold ends on hand-back or release, not on the promotion itself (architecture)
- **Decision**: The hold lifts when the assistant calls wait_for_turn again, or when the existing lock-release timer fires. In the second case the board shows the opponent as stopped and tells the player they can make its move themselves.
- **Rationale**: The AC says held actions succeed "once the assistant has promoted and handed the turn back". The hand-back is already instrumented, and the release reuses the board's existing time (2 min).
- **Rejected**: Releasing as soon as the Active is filled (a second release path; the assistant may have more to do in the same decision).

### Conventions selected (architecture)
- **Decision**: Both loaded conventions apply: no "Claude"/"Nic " in guides, and never exclude `internal/guides` from the build. None dropped.
- **Rationale**: The plan edits three guides; it adds no packaging change.
- **Rejected**: None.

### "Involves the opponent" means the opponent acts (discovery)
- **Decision**: A card counts as involving the opponent, and so can stop the wait, only when the opponent (or "each player") is the subject of an action in its text, such as "your opponent shuffles", "your opponent reveals" or "each player switches". A text that only names the opponent's Pokémon as a target or a damage modifier ("your opponent's Active Pokémon", "1 of your opponent's Benched Pokémon") does not stop the wait: the player resolves it on the board.
- **Rationale**: Many attacks and Items name the opponent's Pokémon only for damage or a gust. Stopping for each would break "a turn of ordinary cards produces no stops" and the AC that an attack that only deals damage never stops. The spec's AC allows a gust to cost "at most one" stop, and zero is within that.
- **Rejected**: Any mention of "opponent" (too noisy). Stopping for gusts too (allowed by the AC, but costs the player time for nothing).

### Applied events carry the system actor in the card's own transaction (discovery)
- **Decision**: The opponent's part is emitted inside the opener's own transaction, stamped `ActorSystem` and the opponent's seat, so it commits atomically with the card and is recognisable as applied.
- **Rationale**: One commit means no window where the card is played without its effect. The system actor tells applied events apart from the human's own cross-seat moves in review mode, and undo already allows it.
- **Rejected**: A second joined `runLocked` (two commits); a listener or goroutine (cannot call verbs under the lock; racy); the human actor (indistinguishable from manual moves).

### Unchecked card conditions apply without a warning (discovery)
- **Decision**: The playmat checks two "You can use this card only if …" conditions: any of your Pokémon Knocked Out during your opponent's last turn (Unfair Stamp), and your opponent having N or fewer Prize cards remaining (Special Red Card, Roxanne). When either is unmet it applies the effect and warns. Any other condition sentence is accepted unchecked, with no warning.
- **Rationale**: The spec makes conditions the player's responsibility and asks only for a warning when one is unmet. A condition the playmat cannot judge cannot be called unmet.
- **Rejected**: Falling back to asking the assistant for unknown conditions (the condition is not the opponent's decision); warning "condition not checked" on every such card (noise).

### Undo still ends the wait (discovery)
- **Decision**: Leave the existing behaviour where an undo or rewind ends the assistant's wait (`presence.go:236-237`).
- **Rationale**: The spec's undo requirement is about the board's state, not about the wait continuing, and changing session survival across undo widens scope.
- **Rejected**: Keeping the session alive across an undo that stays within the turn (a separate change).

### No browser test for the decision hold (testing_approach)
- **Decision**: Cover the hold with the board server's Go websocket tests, and check the board's new label, notice and promote entry by hand. No Playwright test.
- **Rationale**: The browser harness exposes only a reset route and cannot start a wait or set the presence. A test-only route would be new test infrastructure for three small UI touches.
- **Rejected**: Adding a harness route to drive the presence (cost out of proportion).

### Ten tasks across three milestones, with interim test adjustments (tasks)
- **Decision**: Milestone 1 keeps the old respond outcome for cards that are not applied, so its lock tests switch from Judge to an unrecognised-opponent card. Milestone 2 replaces respond/promote with decide and temporarily maps the old texts until the renderer task rewrites them.
- **Rationale**: Each milestone then builds and passes on its own, as the milestones step requires.
- **Rejected**: One large task rewriting presence, renderer and tests at once (not independently deliverable, harder to verify).

### Unknown cards count as naming the opponent for the hold (tasks)
- **Decision**: Under a decision hold, a Trainer or Ability whose card definition is unknown is held, like one that names the opponent.
- **Rationale**: This is the safe side. An unknown card may touch the opponent, the hold lasts only seconds, and cards are normally cached.
- **Rejected**: Allowing unknown cards (could pre-empt the assistant's decision).

## Rehydration cues

- `spektacular spec file read 20261107074845-fc71eb04-real-game-opponent`
- `spektacular knowledge read` entries: `gotchas/handoff-in-tool-results.md`, `gotchas/turn-lock-cross-seat-joins.md`, `gotchas/game-listeners-under-lock.md`, `gotchas/chatgpt-tool-discovery.md`, `conventions/skills-text-and-packaging.md` (tier repo, store tcgreplay).
- Re-read: internal/game/presence.go (whole), internal/mcpserver/wait.go (whole), internal/game/game.go:196-434, internal/game/verbs.go:222-341 and :428-582, internal/webui/actions.go:86-146, internal/game/presence_respond_test.go:1-100.
- `spektacular plan file read 20261026104139-4e1bb7fa-wait-for-turn plan` for the presence design's history.
