---
created_date: "2026-10-26"
document_status: draft
---

# Context: 20261026104139-4e1bb7fa-wait-for-turn

## Current State Analysis

- **Hand-off today**: ending a turn on the board in play mode returns a "Your turn in tcgreplay … Call get_log since N and play turn T" prompt, which the board copies to the clipboard for the player to paste into chat (`internal/webui/actions.go:256-264`, `web/src/lib/actions.js:350-360`). Nothing tells the assistant by itself.
- **Game change notices**: `game.Update{Head, Branch, Events, Delta}` (`internal/game/game.go:43-49`) is sent to listeners after writes (`game.go:404-412`) and branch checkouts (`internal/game/reads.go:576-592`, used by `Checkout`, `Rewind`, `JumpTo`, `Undo`). `SetMode` sends nothing (`game.go:143-151`). Listeners run under the game lock and cannot be removed (`game.go:128-133`; knowledge `gotchas/game-listeners-under-lock.md`).
- **Turn lock**: play mode lets a seat act only on its own turn (`game.go:417-434`); review mode never locks. Opponent-side effect verbs skip the lock only while joined to the other seat's open compound action (`game.go:336-348`); promotion into an empty Active is allowed any turn (`internal/game/verbs.go:380-391`).
- **Manager**: one current game, in memory, replaced by `setCurrent` (`internal/game/manager.go:168-178`) and dropped by `Expire` (`manager.go:119-128`), with `OnLoad`/`OnClear` hooks the board hub uses (`internal/webui/server.go:46-55`).
- **Entry points**: every board request goes through `webui.Server.dispatch`/`do` (`internal/webui/actions.go:83-272`); every game tool goes through `mcpserver.add`/`bind` (`internal/mcpserver/server.go:110-188`).
- **Hosted**: per-account managers (`internal/usergames/usergames.go:64-81`); tool calls resolve via `ResolveMCP` → `Registry.Use`, which records activity (`internal/usergames/mcp.go:20-33`); `Registry.Get` finds an entry without activity (`usergames.go:159-166`); the idle sweep runs every min(idle/4, 30 s) (`usergames.go:225-246`).
- **Settings**: flags with env defaults, validated at start-up and passed to the spawned daemon through `config.args()` (`cmd/tcgreplay/main.go:40-100`).
- **Guides**: six embedded guides, pinned by tests (`internal/guides/guides.go:17`, `internal/guides/guides_test.go`); replay-drill and review-game hand the player turns without any waiting step (`internal/guides/replay-drill.md:24`, `internal/guides/review-game.md:37`).

## Per-Task Technical Notes

Requirement-to-repo attribution: every requirement lands in the single registered repo `tcgreplay` (`/home/nicj/code/github.com/nicholasjackson/tcgreplay`). The per-task file lists below are the file attribution.
- Turn hand-off, inactivity check-in, resume after check-in, game changes ending the wait, waiting lasting as long as the turn, and waits tied to the handed-over seat: Tasks 1-3.
- Waiting does not keep a game alive: Task 4.
- Adjustable timings: Task 5.
- Mid-turn promotion, responding to card effects, the assistant judging what its side must do, and waiting resuming after a response: Task 6.
- Board locks while the opponent acts, the lock cannot stick, and the board shows "Opponent is waiting": Tasks 7-8.
- The assistant knows the play loop: Tasks 3 and 9.
- Waits only after handing over a turn: Tasks 3, 7 and 9.

### Task: Game notices say what changed

- `internal/game/game.go:43-49`: add `Change string \`json:"change,omitempty"\`` to `Update`, with constants `ChangeMode = "mode"`, `ChangeCheckout = "checkout"`, `ChangeRewind = "rewind"`, `ChangeUndo = "undo"`.
- `internal/game/game.go:143-151`: `SetMode` returns early when the mode is unchanged. Otherwise it sets the mode and notifies every listener with `Update{Head, Branch, Change: ChangeMode}` while still holding `g.mu`, as `checkoutLocked` does.
- `internal/game/reads.go:570-592`: `checkoutLocked(name, change string)` sets `u.Change`. Callers pass the kind: `Checkout` → `ChangeCheckout` (`reads.go:570-574`); `Rewind` (`reads.go:453-477`) and `JumpTo` (`reads.go:482-505`) → `ChangeRewind`; `Undo` (`reads.go:511-547`) → `ChangeUndo`; the failure-rollback path keeps no notify.
- `internal/game/ui.go:145-163` (beside `TurnStartedAt`): add `func (g *Game) HandOff(since int) string`, returning `fmt.Sprintf("Your turn in tcgreplay (game %s, branch %s). Call get_log since %d and play turn %d (%s's turn).", g.ID, branch, since, state.Round(st.Turn), st.Current)`. Take the lock once and read state and branch inside it.
- `internal/game/reads.go` (new unexported helper): `eventsSince(since int) ([]state.Event, bool)` returns the path from after `since` to the head on the current branch, with `false` when `since` is not on the current path. Also `actionOf(id string) *store.Action` via `g.log.Action`. Both are called with `g.mu` held. Add an exported locked wrapper only if Task 2 needs it from the manager.
- `internal/webui/actions.go:256-264`: replace the inline `fmt.Sprintf` with `g.HandOff(since)`. The data key stays `prompt`.
- `internal/webui/hub.go:68-73`: no logic change; the hub keeps appending `u.Events` (nil for change notices) and waking. Add a comment that change notices refresh too.
- Tests: `internal/game/game_test.go` adds a listener-recording test covering the mode switch (and the no-op switch), checkout, rewind, jump and undo kinds. `internal/webui/webui_test.go:276-306` (`TestEndTurnCopiesPrompt`) keeps passing unchanged. Add a webui test that an MCP-style `g.SetMode` pushes a fresh state to a connected board.

**Complexity**: Low
**Token estimate**: ~25k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: The presence and the turn wait

- `internal/game/presence.go` (new): unexported `presence` struct guarded by its own `sync.Mutex` (never held while calling into a `Game`, which takes `g.mu`). Fields:
  - `changed chan struct{}`: closed and replaced on every change by `bump()`.
  - `game *Game`.
  - `session *waitSession`.
  - `callGen int`: a new call bumps it; an older in-flight call returns `still_waiting`.
  - `inFlight bool`.
  - `lastBoard time.Time`.
  - `waitingUntil time.Time` and `graceTimer *time.Timer` (`AfterFunc`, reset never stacked): keeps the display "waiting" for a 20 s grace after a `still_waiting` return, so the status clears within cap + 20 s ≤ 50 s of the assistant stopping.
  - `onChange []func()`.
  - `waitSession{game *Game, branch string, seat state.Seat, cursor int, idleFrom time.Time, checkedIn bool, ended string, koReported bool, reported map[string]bool}`, where `reported` holds the action ids already reported. Only Task 6 fills `koReported` and `reported`; declare them here.
- `internal/game/presence.go`: `watch(g *Game)` is called from `Manager.setCurrent`. It subscribes once per game, and the listener (running under `g.mu`) only takes `p.mu`, records `Change`, and calls `bump()`. On `ChangeMode`, `ChangeCheckout`, `ChangeRewind` or `ChangeUndo` with a live session for that game, it sets `session.ended` to `mode_changed`, `branch_switched` (checkout) or `rewound` (rewind or undo). The same function also handles the game being replaced (`game_replaced`) and `cleared()` from `Expire` (`game_expired`). Follow knowledge entry `gotchas/game-listeners-under-lock.md`.
- `internal/game/presence.go`: `func (m *Manager) Wait(ctx, WaitRequest) (WaitOutcome, error)`:
  1. Under `p.mu`, start or continue the session. A nil `Since` starts a fresh session, with cursor = `g.TurnStartedAt(seat)` (called outside `p.mu`) and `idleFrom = now`. A `Since` continues: if `session.checkedIn`, reset `idleFrom = now` and clear `checkedIn`; set `cursor = *Since`. If a session has `ended`, return `ended` with that reason and drop it. If `m.Current()` errors, return `ended` with `game_expired` when it is `ErrExpired`, else `game_closed`.
  2. Bump `callGen` and set `inFlight`.
  3. Loop: `evaluate()`, then select on `changed`, `ctx.Done()`, the cap timer (`Timing.WaitCap`), the idle deadline timer (`max(lastBoard, idleFrom) + Timing.CheckIn`) and the supersede signal. Stop both timers on return.
  - `evaluate` order for this task: ended → turn-end `EvTurn{Phase:"end"}` with `Seat == session.seat` after the cursor (via `eventsSince`) → check-in deadline passed (set `checkedIn`) → nothing.
  - `turn_ended` fills `Turn` (`state.Round(st.Turn)`), `TurnSeat`, `Prompt = g.HandOff(cursor)`, `Since = cursor` and `Next = head`, then drops the session.
  - `still_waiting` returns `Next = cursor` unchanged.
  - A fresh wait in play mode where `st.Current != seat` and no turn-end is pending returns an error ("it is <seat>'s turn: end_turn first, then wait"). Play mode is read from `g.Mode()`, and the check runs after the turn-end scan.
- `internal/game/manager.go:21-41`: add `Timing Timing` (exported, zero means defaults via `func (t Timing) withDefaults() Timing`) and `presence *presence` (lazy init under `m.mu`, or in `NewManager`; tests construct `&Manager{}` too, so prefer lazy). Add `OnPresence(f func())` and `BoardActivity()`; the latter sets `lastBoard = now` and bumps.
- `internal/game/manager.go:168-178`: `setCurrent` calls `m.presenceRef().gameChanged(g)` (ends the old session with `game_replaced`, watches g). `manager.go:119-128`: `Expire` calls `presence.cleared()` (ends with `game_expired`).
- `internal/game/presence.go`: add the `WaitRequest`, `WaitOutcome` and `UsedCard` types and the outcome-kind constants, as in plan.md's Data Structures.
- Tests in `internal/game/presence_test.go` (new), using `Manager.Timing{WaitCap: 200*time.Millisecond, CheckIn: 300*time.Millisecond, LockRelease: …}` and the setup helpers in `internal/game/game_test.go`:
  - prompt turn-end (returns well under the cap);
  - review mode with the other seat's end not counted (start the session while it is the other seat's turn);
  - check-in, and no check-in under steady `BoardActivity`;
  - check-in then continue resets the clock, and the turn ended while away is reported;
  - each ending reason, during a call and between calls;
  - cap;
  - supersede;
  - ctx cancel;
  - leak test: 200 waits across those exits, then assert `len(g.listeners)` is unchanged (test in package `game`), `runtime.NumGoroutine()` returns to baseline (poll with a short deadline) and the presence has at most one grace timer.

**Complexity**: High
**Token estimate**: ~70k tokens
**Agent strategy**: Parallel analysis, sequential integration. One agent writes `presence.go` and the manager hooks; a second drafts `presence_test.go` against the Data Structures contract at the same time; then integrate and run sequentially.

### Task: The wait_for_turn tool

- `internal/mcpserver/server.go:17-34`: add `ResolveQuiet func(ctx context.Context, extra *mcp.RequestExtra) (*game.Manager, string, error)` with a doc comment ("used by wait_for_turn so waiting is not activity; nil falls back to Resolve").
- `internal/mcpserver/server.go:110-131`: factor `bind` into `bindWith(ctx, req, resolve)`. Add an `addQuiet` variant of `add` (`server.go:174-188`) that binds with `ResolveQuiet` when set, else `Resolve`.
- `internal/mcpserver/server.go` (new tool in `register`, after `end_action` at `server.go:666-676`): input `WaitArg{Seat string; Since *int}` with jsonschema descriptions. Behaviour:
  - Seat resolution: play mode → `s.HumanSeat`, and refuse any other seat; review mode → the given seat, else `g.State().Current`.
  - Call `s.m(ctx).Wait(ctx, game.WaitRequest{...})`.
  - Render: `{outcome, seat, since: Next, turn, turn_seat, prompt, log, cards, reason, next}`. `log` is the lines from `g.Log(outcome.Since, viewer, "wait_for_turn", actor)`, with the viewer as in `get_log` (`server.go:482-508`: seat view in play mode, reveal in review mode). Omit `log` for `still_waiting` when there are no lines.
  - `next` per outcome:
    - still_waiting: "Call wait_for_turn again with since N. Post nothing in chat."
    - turn_ended: the prompt, plus "Play your turn now."
    - check_in: "Ask the player exactly \"Still playing?\" and nothing else. When they reply, call wait_for_turn with since N."
    - ended: "Stop waiting: <reason in words>. Wait again only after you hand the player a turn."
  - Tool description: "After you hand the player a turn (end_turn in play mode; a drill or a line for them to play in review mode), call this and act on the result. Each call returns within about 30 seconds."
- `internal/mcpserver/server.go:61-68`: add one bullet to `instructions`: "- After handing the player a turn (your end_turn in play mode, or a drill or line for them to play in review mode), call wait_for_turn and do what its result's next says, calling it again until the turn ends. Never ask the player to say \"your turn\" or \"done\"; on a check-in ask only \"Still playing?\"."
- Tests in `internal/mcpserver/server_test.go`, all through `forEachTransport`:
  - `TestWaitForTurnHandOff`: set up a play-mode game with `game.Manager.Timing` short, end the assistant's turn, run the call in a goroutine, end the human turn via `m.Current()` → `g.EndTurn(human, state.ActorHuman)`, and assert outcome, turn, turn_seat, prompt, log and next.
  - `TestWaitForTurnSeatRules`.
  - `TestWaitForTurnStillWaiting` (cap).
  - `TestWaitForTurnCheckIn`.
  - `TestWaitUsesQuietResolver`: count calls to Resolve and ResolveQuiet.
  - `TestInstructionsTeachTheLoop`: names `wait_for_turn` and `Still playing?`, and does not ask for `your turn`.
  - Extend `TestToolList` (`server_test.go:408-448`) to require `wait_for_turn`.
  - Check `internal/mcpserver/guides_test.go:104-160` (`TestGuideInstructions`) still passes with the new bullet.

**Complexity**: Medium
**Token estimate**: ~45k tokens
**Agent strategy**: 2-3 parallel agents for independent changes: one for the tool and its rendering, one for the instruction line and tests, one for the quiet-resolver plumbing; integrate at the end.

### Task: Waiting does not keep a hosted game alive

- `internal/usergames/mcp.go:17-33`: add `func (r *Registry) ResolveMCPQuiet(_ context.Context, extra *mcp.RequestExtra) (*game.Manager, string, error)`. It does the same token checks, then `e := r.Get(acct)` (no activity, no place; `usergames.go:159-166`) and returns `e.M, e.Link(r.cfg.Public), nil`. Comment: "waiting is not activity, so an abandoned game still expires".
- `cmd/tcgreplay/hosted.go:196`: set `ResolveQuiet: reg.ResolveMCPQuiet` beside `Resolve: reg.ResolveMCP`.
- Tests:
  - `internal/usergames/mcp_test.go`: `TestResolveMCPQuietIsNotActivity`. Use, advance the fake clock, call ResolveMCPQuiet, and assert `LastActive` unchanged and the same manager.
  - `internal/usergames/idle_test.go`: `TestWaitingDoesNotKeepGame`. With an idle registry and a game open, start `m.Wait` in a goroutine with short Timing, advance the clock past idle, `Sweep()`, and assert the game expired and the wait returned `ended` with `game_expired`.
  - Optionally `cmd/tcgreplay/hosted_idle_test.go`: an end-to-end hosted wait over HTTP with the same assertion, following that file's helpers.

**Complexity**: Low
**Token estimate**: ~20k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Adjustable wait timings

- `cmd/tcgreplay/main.go:40-58`: add `timing game.Timing` and `timingErr error` to `config`.
- `cmd/tcgreplay/main.go:84-100`: three `envDuration` reads (`TCGREPLAY_WAIT_CAP` default 30s, `TCGREPLAY_CHECK_IN` 5m, `TCGREPLAY_LOCK_RELEASE` 2m) and `flag.DurationVar` for `-wait-cap`, `-check-in` and `-lock-release`. A flag clears its env error, extending the `flag.Visit` at `main.go:95-99`. Validate after parsing in one `validTiming` function: cap must be > 0 and ≤ 50s; the others must not be negative (0 on check-in or lock release means the default). An invalid value is fatal, with the setting name and limit in the message.
- `cmd/tcgreplay/main.go:60-63` (`config.args`): append `-wait-cap`, `-check-in` and `-lock-release` with their values so the daemon spawned by the shim (`cmd/tcgreplay/shim.go`) gets them.
- `cmd/tcgreplay/main.go:136-142` (`newServer`, used by in-process and the daemon at `cmd/tcgreplay/daemon.go:162`): set `m.Timing = c.timing`.
- `cmd/tcgreplay/hosted.go:178`: in the `NewManager` closure, set `Timing: c.timing` on each new manager.
- `.do/app.yaml`: no change needed (defaults apply). Optionally document the three env vars in a comment beside `TCGREPLAY_IDLE_TIMEOUT`.
- Tests:
  - `cmd/tcgreplay/main_test.go`: `config.args()` includes the three flags with values; validation rejects 0, -1s and 51s caps and negative others.
  - `cmd/tcgreplay/hosted_test.go`: a hosted account's manager carries the configured Timing (via the registry's `NewManager`).

**Complexity**: Low
**Token estimate**: ~20k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Mid-turn promote and respond outcomes

- `internal/game/presence.go` (`evaluate`): after the turn-end check, add:
  - **promote**: the assistant seat is the session seat's `Other()`. If `st.Sides[assistant].Active.Empty()` and `!session.koReported`, set `koReported` and return `promote`. Clear `koReported` when the Active is filled again.
  - **respond**: for each event after the cursor whose action (via `actionOf`) has `Seat == session.seat`, `Actor == state.ActorHuman` and `Kind` in {`trainer`, `ability`, `attack`} (`internal/game/verbs.go:272,312,402`), and whose id is not in `session.reported`, collect `UsedCard{IID: action.Source, Name: card name, Def, Kind, Text}`. Take the text from `g.card(st, iid)` (`internal/game/game.go:437-452`): `Effect` for trainers, the named ability or attack text from the action label otherwise. Fall back to the card's full `Text()` and an empty string when it is unknown.
  - Mark the collected ids reported and return `respond`, with `Next = head`.
- Response window: `internal/game/game.go:56-72` gains `responder state.Seat` (guarded by `g.mu`) with `setResponder(seat)`. `checkTurn` (`internal/game/game.go:417-434`) allows `seat == g.responder` while `g.st.TurnOpen`. The presence sets the responder to the assistant seat when it returns `respond` or `promote` (calling into the game outside `p.mu`), and clears it at the start of the next `Wait` call, on session end, and on the game's `EndTurn` (`internal/game/verbs.go:155-191`: set `g.responder = ""` there).
- `internal/mcpserver/server.go` (wait tool rendering, from Task 3):
  - render `cards` as `[{iid, name, card, kind, text}]`.
  - `next` for respond: "The player used the cards below. Read each card's text: if it asks your side (<seat>) to do something, do it now with your seat's verbs (pass seat <seat> in review mode), then call wait_for_turn with since N. If nothing, call it again straight away."
  - `next` for promote: "Your Active Pokémon was Knocked Out: promote with switch (to_slot) if you have a Benched Pokémon, then call wait_for_turn with since N."
- Tests:
  - `internal/game/presence_test.go`: KO by `Attack` + `KnockOut`, by `UseAbility` + `KnockOut`, and by plain `KnockOut` each return promote once, and continuing after `Switch` waits.
  - Judge (`sv01-176`) and Unfair Stamp (`sv06-165`) via a fake `cards.DB` (pattern in `internal/webui/webui_test.go:31-60`) are reported with text. Then, in play mode, the assistant's `Move` of its hand to the deck, `Shuffle` and `Draw` succeed during the human turn even after `EndAction`.
  - The same verbs are refused when no response is open.
  - A no-op trainer is reported once only.
  - `internal/mcpserver/server_test.go`: respond and promote outcomes render `cards` and `next`.

**Complexity**: Medium
**Token estimate**: ~45k tokens
**Agent strategy**: 2-3 parallel agents for independent changes: game-side evaluation and response window, MCP rendering, and tests; integrate sequentially.

### Task: The acting lock and opponent status on the board server

- `internal/game/presence.go`: add acting state `acting bool`, `stopped bool`, `lastMove time.Time` and `releaseTimer *time.Timer` (`AfterFunc(Timing.LockRelease)`, reset on each move, never stacked).
  - Set acting when `Wait` returns `turn_ended`, `respond` or `promote`.
  - `AssistantMoved(write bool)` refreshes `lastMove` and resets the timer while acting or stopped (a move while stopped sets acting again). In play mode, a write by the assistant seat while `st.Current` is the assistant seat sets acting.
  - `AssistantDone()` and the start of any `Wait` call clear acting and stopped.
  - The timer firing sets `stopped`, clears acting and notifies.
  - Game changes (replace, expire, mode, checkout, rewind) clear acting and stopped.
  - `Locked()` is `acting`.
  - `Opponent()` returns `acting`, else `stopped`, else `waiting` (`inFlight` or `now < waitingUntil`), else "".
  - Every change runs the `OnPresence` hooks outside `p.mu`.
- `internal/game/manager.go`: expose `Locked()`, `Opponent()`, `AssistantMoved(bool)` and `AssistantDone()`, delegating to the presence.
- `internal/mcpserver/server.go:174-188` (`add`): after a successful bind, call `s.m(ctx).AssistantMoved(false)`. In `verb` (`server.go:215-226`), after a successful write call `AssistantMoved(true)` (the presence checks mode and turn). In the `end_turn` tool (`server.go:663-665`), after success call `AssistantDone()`. The wait tool (quiet path) does not call `AssistantMoved`.
- `internal/webui/actions.go:83-105` (`dispatch`): call `s.M.BoardActivity()` for every request after the Limit check.
- `internal/webui/actions.go:107-180`: before the `end_action`, `checkout` and `rewind` cases, and before the second switch (`actions.go:182`), refuse when `s.M.Locked()` with the error "the opponent is acting; wait for them to finish". `set_mode`, `set_reveal`, `replay` and the reads stay above the guard.
- `internal/webui/actions.go:256-264`: when `s.M.Opponent() == "waiting"` (checked after `EndTurn`), add `"waiting": true` to `r.Data` and keep the prompt.
- `internal/webui/server.go:46-55` (`New`): `m.OnPresence(s.hub.wake)`.
- `internal/webui/hub.go:137-172` (`stateMessage`): add `msg["opponent"] = h.s.M.Opponent()` when it is non-empty.
- Tests:
  - `internal/webui/webui_test.go`:
    - `TestLockRefusesGameActions`: drive the presence by starting a wait and producing a respond outcome, or through a test hook on the manager; assert the refusal message for draw, end_action, checkout and rewind, and success for set_mode, set_reveal, replay and deck_list.
    - `TestLockReleasesAfterFinish`.
    - `TestOpponentStatusPushed`: waiting, acting, stopped.
    - `TestNoLockInReviewWithoutWait`.
    - `TestEndTurnReportsWaiting`.
    - `TestBoardActivityRecorded`.
  - `internal/game/presence_test.go`: release after LockRelease with no move, `stopped` reported; steady `AssistantMoved` keeps it locked; the waiting status clears within the grace after `still_waiting`, and at once after `check_in`.
  - `internal/mcpserver/server_test.go`: a tool call refreshes `lastMove`, and `end_turn` clears acting.

**Complexity**: High
**Token estimate**: ~60k tokens
**Agent strategy**: Parallel analysis, sequential integration. Analyse the presence lock state and the board guard in parallel, then integrate the presence first, the board server second and the MCP hooks last, running the full suite after each.

### Task: Board shows the opponent status

- `web/src/lib/store.svelte.js:4-60`: add `opponent: ''` to `app`. In `canAct()` (`store.svelte.js:103-108`), return false when `app.opponent === 'acting'`.
- `web/src/lib/ws.js:45-52` (`onState`): `app.opponent = msg.opponent || ''`.
- `web/src/App.svelte:101`: after the top seat name, render a badge when `app.opponent` is set: "Opponent is waiting", "Opponent is acting" or "Opponent stopped responding" (`class="opp-status {app.opponent}"`), with styles beside `.seat-name` (`App.svelte:238`) using the existing CSS variables.
- `web/src/lib/actions.js:350-360` (`endTurn`): return before copying when `data?.waiting` (optionally a short info toast "Turn ended"); otherwise behave as today.
- Check other action entry points that bypass `canAct()` (context menus in `web/src/lib/actions.js`, drag and drop in `web/src/components/Card.svelte` and `Stack.svelte`). The server refuses regardless, so disabling is best-effort; at minimum, gate the Strip buttons (`web/src/components/Strip.svelte:72-74`) and the hand drag.
- Verify with `make build` (runs `npm run build`).

**Complexity**: Low
**Token estimate**: ~20k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Guides teach the waiting step

- `internal/guides/replay-drill.md:24` (end of "2. Set it up without the answer"): after "Ask: \"It's your turn. What's your play?\"", add: "If they play it on the board, call `wait_for_turn` with their seat and do what each result says; when it reports the turn ended, go straight to Compare. Never ask them to say they are done." In "3. Compare" (`replay-drill.md:28`), "Read their line" uses the result's log or `get_log` since the rewind point.
- `internal/guides/review-game.md:37` (step 2.4): replace "let them and read their moves with `get_log` afterwards" with: "let them: call `wait_for_turn` with their seat, answer any card that asks your side to act, and read their moves from the result when the turn ends." Add a bullet to "3. Test the alternative" (`review-game.md:40-46`): "To play a line out with the player taking one seat, play the other seat's turns yourself and, after each `end_turn`, call `wait_for_turn` on the player's seat; reply when it reports their turn ended. Never ask them to say \"done\"." Name turns by round and seat, keep the six-guide set, and do not write "Claude" or "Nic ".
- `internal/guides/guides_test.go:225-248` (`TestGuidesKeepTheirRules`): add the anchors `"wait_for_turn"` to `replay-drill` and to `review-game`.
- `internal/mcpserver/guides_test.go:166-180` (`TestGuidesNameRealTools`) passes only once Task 3's tool exists, which is why this task depends on it.

**Complexity**: Low
**Token estimate**: ~12k tokens
**Agent strategy**: Single agent, sequential execution.

## Testing Strategy

Per task (all Go tests run by `make test`; timing tests use millisecond `game.Timing` values and the registry's fake clock):

- **Game notices say what changed**: game listener tests per change kind (mode, no-op mode, checkout, rewind, jump, undo); webui test that an assistant mode switch refreshes a board; `TestEndTurnCopiesPrompt` unchanged.
- **The presence and the turn wait**: new `internal/game/presence_test.go` covering prompt turn-end, other-seat turn ignored in review, check-in and steady activity, check-in then continue, each ending reason during and between calls, cap, supersede, cancel, and the leak test (listeners, goroutines, timers back to baseline). Covers success metrics "picks up within a few seconds" (playmat side) and "memory and connections do not grow".
- **The wait_for_turn tool**: `forEachTransport` tests for hand-off rendering, seat rules, still-waiting, check-in, quiet resolver use, the standing-instruction line, and the tool list.
- **Waiting does not keep a hosted game alive**: registry tests that the quiet resolve leaves last activity untouched and that a waited-on game still expires and ends the wait with `game_expired`.
- **Adjustable wait timings**: `config.args()` and validation tests; hosted managers carry the configured timing.
- **Mid-turn promote and respond outcomes**: promote once per vacancy for attack, ability and plain knock-outs; Judge and Unfair Stamp reported with text via a fake card DB; the response window allows the assistant's move/shuffle/draw during the player's turn only while open; no-op cards reported once; MCP rendering of `cards` and `next`.
- **The acting lock and opponent status on the board server**: webui tests for refused vs allowed verbs, unlock after finish, pushed opponent status, no lock in review without a wait, end-turn `waiting` flag, board activity recorded; presence tests for self-release, steady moves staying locked, and waiting status clearing within the grace and at once after a check-in.
- **Board shows the opponent status**: no frontend suite; `make build` must succeed; badges and disabled controls are a manual check.
- **Guides teach the waiting step**: new `wait_for_turn` anchors in `TestGuidesKeepTheirRules`; existing format, banned-word, real-tool and turn-naming tests keep passing.

Manual items (captured in the implementation test plan): a full play-mode game with no "your turn"/"done"; a replay-drill session with no "done"; end-to-end pick-up delay; a long session including "Still playing?"; the board badge review; the assistant following the loop in a fresh session; hosted behaviour through the claude.ai connector.

## Project References

- Spec: `20261026104139-4e1bb7fa-wait-for-turn` (no design references).
- Repo: `tcgreplay`, root `/home/nicj/code/github.com/nicholasjackson/tcgreplay`.
- Knowledge (repo tier, store `tcgreplay`): `conventions/skills-text-and-packaging.md`, `gotchas/game-listeners-under-lock.md`, `gotchas/go-sdk-sessions-and-auth.md`, `gotchas/board-loopback-locks.md`.
- Prior plans: `20261017143118-b05aa6b1-per-user-games` (activity rules, `Registry.Get`), `20261017112555-30c99ab1-memory-store-record`, `20261019170829-bfb31628-turn-numbering-by-round`, `20261020081616-39b3e658-guide-tool`.

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

The two High tasks (the presence and the turn wait; the acting lock) should run in their own sessions, re-reading `internal/game/game.go`, `internal/game/manager.go` and the new `internal/game/presence.go` first.

## Migration Notes

No data migration: games are in memory. The board `end_turn` result keeps its `prompt` key, so a board app built before this change still works. `game.Update` gains an optional `change` field; the hub ignores it. New settings have defaults, so existing launch commands and `.do/app.yaml` need no change.

## Performance Considerations

- A waiting assistant makes about two tool calls a minute; well within the hosted per-account rate limit (300/min default).
- Each wake-up re-scans events since the cursor on the current branch; a single turn is tens of events, so this is cheap. Do not re-fold state per wake.
- No per-call listeners, one broadcast channel per change, timers reset rather than created per call: memory and goroutines stay flat with the number of waits (covered by the leak test).
- The 30 s cap keeps every call well inside client and proxy timeouts; the cap is refused above 50 s.
