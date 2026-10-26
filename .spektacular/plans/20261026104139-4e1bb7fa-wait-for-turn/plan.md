---
created_date: "2026-10-26"
document_status: draft
---

# Plan: 20261026104139-4e1bb7fa-wait-for-turn

<!-- Metadata -->
<!-- Created: 2026-10-26T11:34:29Z -->
<!-- Commit: 136ea40 -->
<!-- Branch: main -->
<!-- Repository: git@github.com:nicholasjackson/tcgreplay.git -->

## Overview

The assistant waits while the player takes a turn on the playmat and picks up its own part by itself: its next turn, a promotion after a knockout, or its side of a card such as Judge. Today the player has to type "your turn" in chat every time. A new long-polling `wait_for_turn` tool, backed by a per-game presence that also locks the board while the assistant acts and shows the player what the opponent is doing, removes that step in play mode, replay drills and played-out lines, on both the desktop and the hosted playmat. Players practising against the assistant get a match that flows like a real one, with a simple "Still playing?" check if they step away.

## Conventions

- **Never write "Claude" or "Nic " in an `internal/guides` guide; guides serve any assistant and player** — the replay-drill and review-game guides gain the waiting step, and `internal/guides/guides_test.go` fails on either word.
- **Never exclude `internal/guides` from a build context; the guides are embedded with `//go:embed *.md`** — this feature edits guide files but adds no packaging change; keep `.dockerignore` untouched so the hosted image still builds.

## Architecture & Design Decisions

All work lands in the single registered repo, `tcgreplay` (`/home/nicj/code/github.com/nicholasjackson/tcgreplay`). The feature adds one long-polling MCP tool, `wait_for_turn`, backed by a small **presence** object that each game manager owns (`internal/game/presence.go`). The presence tracks three things for the manager's current game: the assistant's **wait session** (which seat was handed to the player, the cursor of what has been reported so far, when the five minutes started, and why the wait ended if it did), the assistant's **acting lock** (set while the assistant responds or plays its own turn, refreshed by each of its tool calls, released when it waits again, ends its turn, or goes two minutes without a move), and the **player's last board action**. The presence lives on the manager because the manager is already the one thing the MCP tools and the board both hold. In hosted mode there is one manager per account (`internal/usergames/usergames.go:64-81`), so presence is per account with no new mapping. The presence subscribes to each game once, when that game becomes current (`Manager.setCurrent`, `internal/game/manager.go:168-178`), and is told directly when the manager replaces or expires its game. Its listener only records the change and wakes waiters, because game listeners run with the game lock held (knowledge entry `gotchas/game-listeners-under-lock.md`).

A wait call is a long poll capped at about 30 seconds (a setting). It looks at the board since its cursor and returns as soon as there is something to report:
- **turn ended:** the handed-over seat ended its turn. The result carries the hand-off text the board already builds, factored into one shared function, plus the player's moves as log lines.
- **respond:** the player used a card, Ability or attack. The result carries the card's iid, name and text, and the assistant decides from that text whether its side has anything to do.
- **promote:** the assistant's Active Spot is empty.
- **check in:** five minutes without a board action from the player.
- **ended:** the mode changed, the game was replaced, closed or expired, or the board moved to another branch (checkout or rewind).
- **still waiting:** the cap was reached.

Each result also says in a `next` line exactly what to do, so the loop teaches itself. Waiters block on a channel that the presence closes and replaces on every change, never on a per-call game listener. Abandoned or cancelled calls therefore leave nothing behind: a new call supersedes the one still in flight, and the session is a single value that is replaced, not added to. This answers the spec's named risk and the memory success metric. To report why a wait ended, `game.Update` gains a change kind (`mode`, `checkout`, `rewind`, `undo`), and `SetMode` starts notifying listeners (`internal/game/game.go:143-151`, `reads.go:576-592`).

The **lock** is enforced in the one place board actions enter (`webui.Server.do`, `internal/webui/actions.go:107-272`). While the assistant is acting, every game write, `end_action`, `checkout` and `rewind` is refused with "Opponent is acting". Mode switches, the reveal toggle, replays and reads stay open, so the player can look at the board and has an escape hatch. The same entry point records the player's board activity for the five-minute check. The assistant's side gets the matching hooks in the one place its tools enter (`add`/`bind`, `internal/mcpserver/server.go:110-188`): each game tool call refreshes the acting clock, and in play mode the assistant's own writes on its own turn mark it acting, so its whole turn locks the board even when no wait came first. While a respond outcome is open, play mode's turn lock also lets the assistant's seat make its response during the player's turn, even if the player's compound action has already closed. The board learns of every presence change through the existing hub (`internal/webui/hub.go:88-172`): its state message gains an `opponent` field (`waiting`, `acting`, `stopped`). The Svelte board shows that field next to the opponent's name, disables its controls while the opponent acts, and stops copying the "paste this prompt" text at end of turn when the assistant is waiting.

**Hosted idle expiry** is preserved by giving the wait its own quiet manager lookup. The MCP server gains an optional `ResolveQuiet` hook, and hosted mode wires it to `Registry.Get`, which finds the account's manager without recording activity (`usergames.go:159-166`). Every other tool keeps `Resolve`/`Use`. An abandoned game therefore expires on schedule, and the expiry reaches the wait through the manager's `Expire` hook. The three timings (per-call cap 30 s, check-in 5 min, lock release 2 min) are manager settings with these defaults. They are exposed as flags and environment variables like `-idle-timeout` (`cmd/tcgreplay/main.go:84-100`), passed through to the daemon, and the cap is refused above 50 s so no call can approach the one-minute limit. The assistant learns the loop from one standing-instruction line, the tool description and each result's `next` line. The replay-drill and review-game guides gain the waiting step wherever they hand the player a turn.

This beats the alternatives because it puts all waiting state behind one owner that already sees every game change, and needs no unsubscribe machinery. It keeps the two entry points (board dispatch and tool registration) as the only places that enforce or refresh anything. It also reuses the hand-off text, the hub and the no-activity lookup that already exist instead of adding parallel paths. Stateless cursor-only waits would miss changes made between calls, per-call subscriptions would leak, and a playmat-side list of cards that need a response is excluded by the spec. See `research.md#alternatives-considered-and-rejected` for the evidence.

## Component Breakdown

- **Presence (new, in the game package)**: the one new component, owned by each game manager and justified by needing a single owner that sees every game change and is shared by the board and the tools. It owns:
  - the assistant's wait session for the current game: the handed-over seat, the cursor of what has been reported, when the five-minute clock started, whether a check-in is outstanding, and the reason the wait ended;
  - the acting lock: whether the assistant is acting, when it last moved, whether it has been marked as stopped responding, and the open response window that lets the assistant's seat act during the other seat's turn;
  - the time of the player's last board action.

  It answers "what should this wait report now?", "is the board locked?" and "what is the opponent doing?" (`waiting`, `acting`, `stopped` or nothing). It wakes waiting calls through a broadcast that it replaces on every change, never through a listener per call. It tells subscribers, chiefly the board hub, whenever its status changes, including changes driven by its own timers (the waiting display's grace period and the two-minute lock release). It subscribes once to each game the manager makes current, and the manager tells it directly when a game is replaced or expires.
- **Game manager (changed)**: owns one presence and the three timing settings (per-call cap, check-in delay, lock-release delay), with the spec's defaults. It hands every newly current game to the presence and reports replacements and expiry to it. It offers the long-poll wait used by the tool, and the small calls the board and tools make on the presence: note a board action, check the lock, note an assistant move, end the assistant's acting. It also gains a hook for presence changes beside its existing load and clear hooks.
- **Game (changed)**:
  - its change notices now say what kind of change happened: new events, a mode switch, a branch checkout, a rewind or an undo. A mode switch now notifies at all.
  - it gives the presence what it needs to judge a wait: the events on the current branch after a cursor, and the action each one belongs to.
  - it builds the one hand-off text ("Your turn in tcgreplay … Call get_log since N and play turn T (seat's turn)") that the board and the wait share.
  - its turn lock honours the presence's response window.
- **MCP tool server (changed)**:
  - gains the `wait_for_turn` tool. It resolves the seat to wait on (the player's seat in play mode; the named or current seat in review mode), runs the manager's wait, and renders the outcome as JSON with the log lines for the right view and a `next` instruction.
  - its single tool-registration path notes each game tool call as an assistant move, and in play mode marks the assistant acting when it writes on its own turn. Ending its turn ends its acting.
  - gains an optional quiet resolver that the wait uses in place of the normal one, so a wait never counts as activity.
  - its standing instructions gain one line on the play loop.
- **Board server (changed)**:
  - its single dispatch path records every request as the player's activity, and refuses game writes, closing an action, checkout and rewind while the presence says the assistant is acting.
  - its hub pushes a fresh state whenever the presence changes, and the state message carries the opponent status.
  - its end-of-turn result says when the assistant is waiting, so the board does not ask the player to paste a prompt.
- **Board app (changed, Svelte)**: shows "Opponent is waiting", "Opponent is acting" or "Opponent stopped responding" beside the opponent's name, disables its own controls while the opponent acts, and skips the clipboard prompt and its toast when the assistant is waiting.
- **Per-account registry (changed, hosted only)**: offers the quiet resolver for the MCP server, finding the caller's manager without recording activity or taking a place, so a waiting assistant never keeps a game alive. Its idle sweep and expiry are unchanged and reach a wait through the manager's expiry.
- **Run modes and settings (changed)**: read the three timing settings as flags and environment variables, refuse a per-call cap above 50 seconds, pass the settings to the daemon the shim starts, give them to every manager (desktop and per account), and wire the quiet resolver in hosted mode.
- **Coaching guides (changed)**: replay-drill and review-game gain the waiting step wherever they hand the player a turn. No new guide.

## Data Structures & Interfaces

**`wait_for_turn` tool (new MCP contract).** The assistant calls it after handing the player a turn, and again after every non-final outcome.

```text
input  { seat?: "p1"|"p2",   // review mode: the seat handed to the player (default: the seat whose turn it is);
                              // play mode: must be the player's seat if given
         since?: int }        // omit to start a fresh wait; pass the previous result's `since` to continue
output { outcome: "turn_ended" | "respond" | "promote" | "check_in" | "ended" | "still_waiting",
         seat: string,        // the seat being waited on
         since: int,          // cursor to pass on the next call
         turn?, turn_seat?,   // turn_ended: the new turn number and whose turn it is
         prompt?,             // turn_ended: the shared hand-off text
         log?: string,        // log lines since the previous cursor, in the same view get_log would use
         cards?: [{ iid, name, card, kind, text }],   // respond: each card, Ability or attack the player used
         reason?: "mode_changed" | "game_replaced" | "game_closed" | "game_expired" | "branch_switched" | "rewound",  // ended
         next: string }       // what to do now, in one or two sentences
```

**`game.Update` (changed).** The game's change notice gains a change kind, so listeners can tell new events from a moved board.

```go
type Update struct {
    Head   int
    Branch string
    Events []int
    Delta  []string
    Change string // "" (new events) | "mode" | "checkout" | "rewind" | "undo"
}
```

**`game.Timing` (new).** The three timing settings a manager carries. A zero value means the spec's defaults (30 s cap, 5 min check-in, 2 min lock release).

```go
type Timing struct {
    WaitCap      time.Duration
    CheckIn      time.Duration
    LockRelease  time.Duration
}
```

**`game.WaitRequest` / `game.WaitOutcome` (new).** The game-level contract behind the tool. The MCP layer turns an outcome into the JSON above, rendering log lines for the right view.

```go
type WaitRequest struct {
    Seat  state.Seat // handed-over seat
    Since *int       // nil starts a fresh session
}
type WaitOutcome struct {
    Kind     string        // turn_ended | respond | promote | check_in | ended | still_waiting
    Seat     state.Seat
    Since    int           // cursor the log covers from
    Next     int           // cursor for the next call
    Turn     int           // turn_ended
    TurnSeat state.Seat
    Prompt   string        // turn_ended
    Cards    []UsedCard    // respond
    Reason   string        // ended
}
type UsedCard struct { IID state.IID; Name, Def, Kind, Text string }
```

**Manager presence surface (new methods).** These are the calls other components make. The presence type itself stays unexported.

```go
func (m *Manager) Wait(ctx context.Context, req WaitRequest) (WaitOutcome, error)
func (m *Manager) BoardActivity()                 // board: the player did something
func (m *Manager) Locked() bool                   // board: refuse game actions?
func (m *Manager) Opponent() string               // board: "waiting" | "acting" | "stopped" | ""
func (m *Manager) AssistantMoved(write bool)      // tools: refresh the acting clock (write on own turn in play mode marks acting)
func (m *Manager) AssistantDone()                 // tools: the assistant ended its turn; release the lock
func (m *Manager) OnPresence(f func())            // board hub: presence status changed
var  Manager.Timing                               // settings, set by the run modes
```

**`game.Game` (new methods).** `HandOff(seat state.Seat) string` builds the shared hand-off text for the turn that just started. An unexported events-since-cursor read serves the presence.

**`mcpserver.Server.ResolveQuiet` (new optional hook).** It has the same signature as `Resolve`: `func(ctx, *mcp.RequestExtra) (*game.Manager, string, error)`. The wait tool uses it when it is set, and falls back to `Resolve`, then to `M`. Hosted mode sets it to `Registry.ResolveMCPQuiet`, which uses `Registry.Get` (no activity, no place).

**Board state message (changed wire format).** The hub's `"state"` message gains `"opponent": "waiting" | "acting" | "stopped"`, omitted when there is nothing to show. The board's `end_turn` result data gains `"waiting": true` when the assistant is waiting, and the prompt is still included for clients that are not waiting.

**Settings (new).** `-wait-cap` / `TCGREPLAY_WAIT_CAP` (default `30s`, more than 0 and at most `50s`), `-check-in` / `TCGREPLAY_CHECK_IN` (default `5m`), `-lock-release` / `TCGREPLAY_LOCK_RELEASE` (default `2m`). All are Go durations, apply in every run mode, and are passed to the spawned daemon.

## Implementation Detail

**A new pattern: one presence per manager, woken by a replaceable broadcast.** The presence keeps a channel that is closed and swapped for a fresh one whenever anything it watches changes: a game event, a moved board, a mode switch, a game replaced or expired, board activity, or an assistant move. A waiting call reads the current channel under the presence's own lock, releases the lock, and selects on that channel, its context, the per-call cap and the next check-in deadline. When it wakes, it re-evaluates from scratch. Evaluation is a pure function of the presence's session and the game's current events, so a wake-up never carries data and missed wake-ups cannot lose information. This is new to the codebase. It is chosen over per-call game listeners because those cannot be removed (knowledge entry `gotchas/game-listeners-under-lock.md`). Readers will find all waiting logic in one file, with the evaluation (what to report) kept separate from the blocking (when to look).

**Fixed evaluation order.** On each look, the wait checks in this order:
1. The session has ended (game replaced or expired, mode changed, branch switched or rewound) → `ended` with the reason.
2. A turn-end event for the handed-over seat after the cursor → `turn_ended`.
3. The assistant's Active Spot is empty, and this has not been reported yet for this vacancy → `promote`.
4. Openers (card, Ability, attack) by the handed-over seat's human actor after the cursor → `respond`, all of them at once.
5. The five-minute clock has run out → `check_in`.
6. The cap has been reached → `still_waiting`.

Play mode refuses a fresh wait while it is the assistant's own turn and nothing is pending. A new call supersedes one still in flight: the older call returns `still_waiting` straight away. Timers are only ever the per-call ones (stopped on return) and the presence's two `AfterFunc` timers (display grace and lock release), which are reset, never stacked.

**Game notices say what changed.** Every code path in the game that notifies listeners now sets the change kind, and a mode switch notifies like a checkout does. This is a small widening of an existing pattern: the board hub keeps ignoring the kind and simply refreshes. A side benefit is that a mode switch made by the assistant now refreshes open boards.

**Two choke points stay the only enforcement sites.** The board's dispatch gains one guard before game writes (and before closing an action, checkout and rewind) and one activity note per request. The MCP tool-registration helper gains one "assistant moved" note per game tool call. The new tool is registered through that same helper, apart from its quiet resolution. No verb implementation learns about locks. The one exception is the game's turn check, which consults the presence's response window so the assistant's play-mode response can happen during the player's turn. It is kept to a single extra condition in the existing turn check.

**Shared hand-off text.** The "Your turn in tcgreplay …" message moves from the board's end-turn handler into the game, so the board's prompt and the wait's `turn_ended` result cannot drift apart. The board keeps returning it (plus a `waiting` flag) for clients that are not waiting.

**Quiet resolution follows the existing hook pattern.** The MCP server already takes an optional `Resolve` hook that is nil on the desktop. `ResolveQuiet` sits beside it with the same signature and the same fallback, so the desktop path is untouched and hosted mode supplies both from the registry.

**Settings follow the `-idle-timeout` pattern.** A flag with an environment default, validated at start-up, carried in the run config, passed to the daemon through the config's argument list, and set on each manager when it is built. Desktop managers and every per-account manager get the same values.

**Board app.** One derived opponent status in the shared store, fed by the state message. One small badge beside the opponent's seat name. `canAct()` returns false while the opponent is acting, which disables the board's own controls through the gate they already use. The end-turn handler skips the clipboard and its toast when the result says the assistant is waiting. This follows the existing "server is authoritative, UI renders" rule: the server refuses locked actions whatever the UI shows.

**Guides and instructions.** One line is added to the standing instructions. The replay-drill and review-game guides each gain a short waiting step at the point where they hand the player a turn, and new rule anchors are pinned by the existing "guides keep their rules" test.

## Dependencies

- **Design documents this plan was built on:** none. The spec carries no design references.
- **`github.com/modelcontextprotocol/go-sdk` (v1.6.0, existing)**: tool registration, request contexts that end when a client cancels or disconnects, and the in-memory and Streamable HTTP transports the tests use. No version change.
- **`github.com/coder/websocket` (existing)**: the board's live connection; unchanged.
- **Svelte 5 / Vite board app (existing)**: changed to show the opponent status and to respect the lock. No new packages.
- **Game package (internal, changed)**: gains the presence, timing settings, change kinds on notices, the shared hand-off text and the response window in the turn lock.
- **Board server package (internal, changed)**: gains the lock guard, the activity note, the opponent status in state messages and the `waiting` flag on end turn.
- **MCP server package (internal, changed)**: gains `wait_for_turn`, the quiet resolver hook, the assistant-move notes and one instruction line.
- **Per-account registry package (internal, changed, hosted only)**: gains a quiet resolver built on its existing no-activity lookup.
- **Coaching guides package (internal, changed)**: two guide texts change; the embedded six-guide set is unchanged.
- **Run modes (`cmd/tcgreplay`, changed)**: gain three timing settings and the hosted wiring.
- **Prior plans relied on, all already landed**: per-user-games (per-account managers, idle expiry, the activity and no-activity lookups), memory-store-record (in-memory manager that replaces and expires games), turn-numbering-by-round (turn numbers and seat naming used in the hand-off text and results), guide-tool (guides served over MCP, guide tests that pin rule anchors and real tool names). Nothing must land before this plan starts.
- **External services**: none new. TCGdex card text is already cached locally and supplies the `text` of reported cards.

## Testing Approach

**Test types.** All automated tests are Go tests in the project's existing Go test suite, which is vetted as part of the project's test target. They follow each package's current conventions:
- the game package's table-free scenario tests on an in-memory manager;
- the MCP server's `forEachTransport` tests, so every tool assertion runs over both the in-memory (desktop) and Streamable HTTP (hosted) transports;
- the board server's websocket tests against an `httptest` server;
- the registry's fake-clock idle tests.

Timing-dependent behaviour is tested by giving the manager millisecond-scale `Timing` values (and, where the registry is involved, its fake clock), so no test sleeps for real minutes. The board app has no test suite, so its changes are covered by manual checks.

**Most coverage: the presence and the wait (game package).** This is new logic with the most branches, and every other component only calls it. The load-bearing assertions:
- a wait on the handed-over seat returns `turn_ended` with the new turn, the seat whose turn it is, the hand-off text and a cursor covering exactly the player's moves. It returns promptly (well under a second) after the end-turn event, never at the cap.
- in review mode, a turn ending for the other seat does not end a wait on the handed-over seat.
- knocking out the assistant's Active by attack, by Ability or by a plain knock-out returns `promote` once per vacancy. After the assistant promotes, a continuing wait goes back to waiting.
- every card, Ability or attack the player uses is reported as `respond` with its name, id and card text, including Judge and Unfair Stamp. A card with nothing for the assistant to do is still reported once, and the next continuing call goes back to waiting without reporting it again.
- with no board activity, the wait returns `check_in` once the check-in time passes. Board activity at intervals shorter than the check-in time never triggers it. The next call after a check-in starts a fresh clock, and a turn the player ended while away is reported as `turn_ended` on that call.
- a mode switch, a new game, an expiry, a branch checkout and a rewind (both the tool's rewind and the board's jump) each end a wait within the cap, with the matching reason, whether the change happens during a call or between two calls.
- each call returns `still_waiting` no later than the cap. A second call supersedes a call still in flight. A cancelled context returns at once.
- the lock: a `respond`, `promote` or `turn_ended` outcome locks; a new wait or the assistant's end turn unlocks; the lock releases itself and reports `stopped` after the lock-release time with no assistant move, and stays locked while the assistant keeps moving more often than that.
- the response window lets the assistant's seat draw and shuffle during the player's turn in play mode even after the player's action closed, and only while a respond outcome is open.
- change notices carry their kind, and a mode switch now notifies.

**MCP tool server.** The tool's contract over both transports:
- the result shape and `next` line for each outcome;
- seat defaults and refusals (play mode only waits on the player's seat; a fresh wait on the assistant's own turn with nothing pending is refused);
- log lines in the same view `get_log` would use;
- the standing instructions name `wait_for_turn` and "Still playing?" and never ask the player to say "your turn" or "done";
- game tool calls refresh the acting clock, and a wait uses the quiet resolver when one is set;
- the tool appears in the tool list.

The existing guide tests (rule anchors, real tool names, banned words) are extended so the replay-drill and review-game guides must keep their new waiting step.

**Board server.**
- while the presence is acting, every game write, close-action, checkout and rewind from the board is refused with the opponent-acting message, while set-mode, reveal, replay and reads still work. Once the assistant finishes, the same action succeeds in the same turn.
- in review mode with no wait session, nothing is ever refused and no opponent status is shown.
- the state message carries `waiting`, `acting` and `stopped`, and is pushed when the presence changes.
- `end_turn` reports `waiting: true` when the assistant is waiting.
- each board request counts as the player's activity.

**Hosted.**
- a wait resolved through the registry does not move the account's last activity.
- a game left with only a waiting assistant expires after the same idle time as one with no assistant.
- that expiry ends the wait with `game_expired`.
- settings validation rejects a cap above 50 s or below zero, and the daemon receives the configured values.

**Deliberate gaps.** Whether a real assistant follows the loop, judges card text correctly and phrases the check-in exactly cannot be asserted in Go. The tests guarantee the playmat side of each of those acceptance criteria (the right outcome, text and `next` line reach the assistant, and its responses are allowed). The assistant's own behaviour is a manual check.

**Success metrics.**
- *A full play-mode game with zero "your turn"/"done" chat messages*: **Manual — captured in the implementation test plan**. The playmat side (each turn end produces a `turn_ended` outcome without any chat) is a behavioural test.
- *A replay drill session where the player never says they finished a drill turn*: **Manual — captured in the implementation test plan**. The playmat side (a review-mode wait on the handed-over seat ends on that seat's turn end) is a behavioural test.
- *The assistant picks up its turn within a few seconds*: **Behavioural test**. A wait returns `turn_ended` within a fraction of a second of the end-turn event, not at the cap. The end-to-end delay including the assistant's own response time is **Manual — captured in the implementation test plan**.
- *Memory and open connections do not grow with the number of waits*: **Behavioural test**. Many waits are run, some timed out, some cancelled, some superseded, some ended by game changes. The test asserts that the number of game listeners, presence timers and goroutines returns to its baseline. A long real session including a "Still playing?" check-in is **Manual — captured in the implementation test plan**.

**Manual reviews.**
- **Manual — captured in the implementation test plan**: the board's "Opponent is waiting", "Opponent is acting" and "Opponent stopped responding" badges read clearly, appear and clear at the right moments, and controls are disabled while the opponent acts.
- **Manual — captured in the implementation test plan**: in a fresh session, the assistant follows the loop for a play-mode game and a replay drill (waits after handing over, responds to Judge and Unfair Stamp, promotes after a knockout, asks exactly "Still playing?" after five idle minutes, and never asks for "your turn" or "done").
- **Manual — captured in the implementation test plan**: on the hosted server, a waiting assistant behaves the same through the claude.ai connector, and an abandoned game still expires.

## Milestones & Tasks

### Milestone 1: Turns pass without "your turn"

**What changes**: In a play-mode game, the player no longer types "your turn" in chat. After the assistant ends its turn it waits, and when the player ends their turn on the board the assistant picks up straight away. It knows what the player did and starts its own turn. If the player does nothing on the board for five minutes, the assistant stops and asks "Still playing?". When they answer, it carries on waiting, or takes its turn if they ended it while away. Switching mode, opening or losing the game, switching branch or rewinding ends the wait with the reason, so the assistant never hangs. On the hosted server a waiting assistant does not keep an abandoned game alive. The three timings can be adjusted when the playmat is started.

**Validation point**: the game and MCP tests for hand-off, check-in, game-change endings, cap and supersession pass over both transports. The hosted idle test shows a game with only a waiting assistant still expires on time, and the waits-don't-leak test shows listeners, timers and goroutines back at baseline. By hand, a short play-mode game against the assistant passes several turns with no chat from the player.

#### - [x] Task: Game notices say what changed
**Id:** 5637335f-d478-4bfb-82d4-d9bdeb5b4511
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

The game's change notices start saying what kind of change happened: new events, a mode switch, a branch checkout, a rewind or an undo. A mode switch now sends a notice at all. The "Your turn in tcgreplay …" hand-off text moves out of the board's end-turn handler into the game, so the board and the coming wait share one wording. The game also gains a read of the events on the current branch after a given point, for the wait to judge what happened.

*Technical detail:* [context.md#task-game-notices-say-what-changed](./context.md#task-game-notices-say-what-changed)

**Acceptance criteria**:
- [x] Switching a game between play and review mode notifies its listeners, and the notice says it was a mode switch; switching to the mode it is already in sends nothing.
- [x] Checking out a branch, rewinding (by the tool or the board's jump) and undoing each send a notice naming that kind of change; new events send a notice with no change kind, as today.
- [x] Ending a turn on the board in play mode returns the same prompt text as before, now built by the game.
- [x] Open boards still refresh on every kind of notice, including a mode switch made by the assistant.

#### - [x] Task: The presence and the turn wait
**Id:** 87047afe-af4e-4f0a-9074-2cf6c15410ba
**Repo:** tcgreplay
**Depends on:**
- 5637335f-d478-4bfb-82d4-d9bdeb5b4511 — Game notices say what changed
**Execution:** agent

Each game manager gains a presence that watches its current game and holds the assistant's wait session, together with the three timing settings and their defaults. The manager offers a long-poll wait that returns when the handed-over seat ends its turn, when the player has been idle for the check-in time, when the game changes under the wait, or when the per-call cap is reached. The board can report player activity to it. Waits never register anything that outlives them, so many waits leave memory where it was.

*Technical detail:* [context.md#task-the-presence-and-the-turn-wait](./context.md#task-the-presence-and-the-turn-wait)

**Acceptance criteria**:
- [x] A wait on the player's seat ends within a fraction of a second of that seat ending its turn, reporting the new turn, whose turn it is, the hand-off text and the point the player's moves start from.
- [x] In review mode, a turn ending for the other seat does not end a wait on the handed-over seat.
- [x] With no player activity for the check-in time, the wait reports a check-in. Activity more often than that never triggers one. The next call after a check-in starts a fresh clock, or reports the turn as ended if the player ended it meanwhile.
- [x] A mode switch, a new or replaced game, an expired game, a branch checkout and a rewind each end the wait with the matching reason, whether they happen during a call or between calls.
- [x] No call lasts longer than the cap. A second call takes over from one still in flight, and a cancelled call returns at once.
- [x] After many waits that time out, get cancelled, get superseded or end on game changes, the game's listeners, the presence's timers and the running goroutines are back where they started.

#### - [x] Task: The wait_for_turn tool
**Id:** c7463a67-1bee-4a40-989b-f9bc864d7b83
**Repo:** tcgreplay
**Depends on:**
- 87047afe-af4e-4f0a-9074-2cf6c15410ba — The presence and the turn wait
**Execution:** agent

The assistant gets a `wait_for_turn` tool that runs the manager's wait and returns its outcome with the player's moves as log lines and a plain `next` instruction. The MCP server gains an optional quiet resolver for the tool, so a wait can find its game without counting as activity. The standing instructions gain one line on the play loop: wait after handing over a turn, act on what the wait says, never ask for "your turn" or "done", and ask "Still playing?" on a check-in.

*Technical detail:* [context.md#task-the-wait_for_turn-tool](./context.md#task-the-wait_for_turn-tool)

**Acceptance criteria**:
- [x] Over both the desktop and hosted connections, the tool reports each outcome with its seat, the next cursor, the log lines in the view `get_log` would use, and a `next` line saying what to do.
- [x] In play mode the tool waits only on the player's seat, and refuses a fresh wait during the assistant's own turn when nothing is pending; in review mode it waits on the named seat or the seat whose turn it is.
- [x] The tool finds its game through the quiet resolver when one is set, and through the normal lookup otherwise.
- [x] The standing instructions tell the assistant to wait after handing over a turn, to ask exactly "Still playing?" on a check-in, and never to ask the player to say "your turn" or "done".
- [x] The tool appears in the tool list.

#### - [x] Task: Waiting does not keep a hosted game alive
**Id:** c51ac894-dc16-4165-8342-cfd2cd167773
**Repo:** tcgreplay
**Depends on:**
- c7463a67-1bee-4a40-989b-f9bc864d7b83 — The wait_for_turn tool
**Execution:** agent

On the hosted server, the per-account registry gains a quiet lookup that finds the caller's game without recording activity or taking a place, and hosted mode wires it into the MCP server for the wait. A game whose only traffic is a waiting assistant then expires after the usual idle time, and the expiry ends the wait.

*Technical detail:* [context.md#task-waiting-does-not-keep-a-hosted-game-alive](./context.md#task-waiting-does-not-keep-a-hosted-game-alive)

**Acceptance criteria**:
- [x] A hosted wait does not move its account's last-activity time or take a place.
- [x] A hosted game left with only a waiting assistant expires after the same idle time as a game with no assistant, and the waiting assistant is told the game expired.
- [x] Every other tool still counts as activity, as today.

#### - [x] Task: Adjustable wait timings
**Id:** 1b311f39-a9bf-4a9e-8ce3-76a029b75717
**Repo:** tcgreplay
**Depends on:**
- 87047afe-af4e-4f0a-9074-2cf6c15410ba — The presence and the turn wait
**Execution:** agent

The playmat gains three settings, the per-call cap, the check-in time and the lock-release time, as flags with environment-variable defaults like the existing idle timeout. Every run mode reads them, passes them to the daemon the shim starts, and gives them to every game manager. A cap above 50 seconds is refused at start-up, so no call can approach the one-minute limit.

*Technical detail:* [context.md#task-adjustable-wait-timings](./context.md#task-adjustable-wait-timings)

**Acceptance criteria**:
- [x] With no settings given, managers use a 30-second cap, a five-minute check-in and a two-minute lock release.
- [x] Each setting can be changed by flag or environment variable, and the flag wins over the environment.
- [x] A cap of zero, below zero or above 50 seconds, or a negative check-in or lock-release time, stops the playmat from starting with a clear message.
- [x] The daemon started by the shim, the in-process server and every hosted account's manager all use the configured values.

### Milestone 2: The assistant answers the player's cards mid-turn

**What changes**: During the player's turn, the assistant reacts by itself. When the player knocks out its Active Pokémon, by attack, Ability or any other effect, the assistant promotes a Benched Pokémon. When the player plays a card or uses an effect, the assistant reads the card text and does its side's part if there is one, for example shuffling its hand away and drawing 4 for Judge or 2 for Unfair Stamp. Then it goes back to waiting while the player carries on with the same turn. Cards that ask nothing of the assistant's side get no move.

**Validation point**: the game and MCP tests for promote and respond outcomes pass, including the response window that lets the assistant's draws and shuffle through in play mode. By hand, playing Judge and knocking out the assistant's Active in a play-mode game produce the right moves with no chat.

#### - [x] Task: Mid-turn promote and respond outcomes
**Id:** 62a6e8cf-32fc-4491-944f-c37a06e1935e
**Repo:** tcgreplay
**Depends on:**
- c7463a67-1bee-4a40-989b-f9bc864d7b83 — The wait_for_turn tool
**Execution:** agent

The wait learns to stop mid-turn. It reports a promotion when the assistant's Active Spot has been emptied, and reports each card, Ability or attack the player uses, with its card text. The assistant then decides whether its side must do anything. While such a response is open, play mode's turn lock lets the assistant's seat make its part of the effect during the player's turn. The tool's results tell the assistant to do its part, or nothing, and then wait again.

*Technical detail:* [context.md#task-mid-turn-promote-and-respond-outcomes](./context.md#task-mid-turn-promote-and-respond-outcomes)

**Acceptance criteria**:
- [x] When the player knocks out the assistant's Active by attack, Ability or a plain knock-out, the wait reports a promotion once. After the assistant promotes, a continuing wait goes back to waiting.
- [x] When the player plays Judge or Unfair Stamp, the wait reports the card with its name, id and text, and the assistant can then shuffle its hand into its deck and draw during the player's turn in play mode.
- [x] A card that asks nothing of the assistant's side is reported once, and a continuing wait does not report it again.
- [x] The assistant's seat can act during the player's turn in play mode only while a response is open; at any other time the turn lock refuses it as today.
- [x] Each promote and respond result's `next` line tells the assistant what to do, and to wait again afterwards.

### Milestone 3: The board shows what the opponent is doing, and locks while it acts

**What changes**: The board shows "Opponent is waiting" while the assistant waits for the player. While the assistant is taking its turn or answering a card mid-turn, it shows "Opponent is acting", and the player's game actions are refused, though the player can still look at the board, replay moves and switch mode. The board unlocks as soon as the assistant finishes. If the assistant goes quiet for two minutes it unlocks by itself and says the opponent stopped responding. Ending a turn no longer asks the player to paste a prompt when the assistant is already waiting. Outside a handed-over turn, review mode never locks and never shows these messages.

**Validation point**: the board-server tests for refusal, allowed controls, the opponent field and the end-turn `waiting` flag pass, along with the lock-release tests. A manual board review confirms the badges and disabled controls.

#### - [x] Task: The acting lock and opponent status on the board server
**Id:** 60639400-0c9d-471f-98da-098108b561f6
**Repo:** tcgreplay
**Depends on:**
- 62a6e8cf-32fc-4491-944f-c37a06e1935e — Mid-turn promote and respond outcomes
**Execution:** agent

The presence gains the acting lock. It is set when a wait hands the assistant something to do, or when the assistant writes on its own turn in play mode. Each assistant tool call refreshes it. It clears when the assistant waits again or ends its turn, and releases itself after two minutes without a move, showing the opponent stopped responding. The board server refuses the player's game actions while locked, records every board request as activity, pushes the opponent status (waiting, acting, stopped) to open boards, and marks its end-turn result when the assistant is already waiting.

*Technical detail:* [context.md#task-the-acting-lock-and-opponent-status-on-the-board-server](./context.md#task-the-acting-lock-and-opponent-status-on-the-board-server)

**Acceptance criteria**:
- [x] While the assistant answers a card, promotes or plays its own turn, the board refuses game actions, closing an action, checkout and rewind with an "opponent is acting" message, but still allows mode switching, the reveal toggle, replays and reads.
- [x] Once the assistant waits again or ends its turn, the player's actions work again in the same turn.
- [x] If the assistant makes no move for the lock-release time, the board unlocks by itself and shows the opponent stopped responding. An assistant that keeps moving more often than that stays locked until it finishes.
- [x] Open boards are told "waiting" while the assistant waits, and the status disappears within 60 seconds of the assistant stopping, including straight after a check-in.
- [x] In review mode with no handed-over turn, the board never refuses an action and never shows an opponent status.
- [x] Ending a turn on the board reports that the assistant is waiting when it is.

#### - [x] Task: Board shows the opponent status
**Id:** b49a0200-5e27-49f9-96f2-20fb436c1f82
**Repo:** tcgreplay
**Depends on:**
- 60639400-0c9d-471f-98da-098108b561f6 — The acting lock and opponent status on the board server
**Execution:** agent

The board app shows "Opponent is waiting", "Opponent is acting" or "Opponent stopped responding" beside the opponent's name, from the status the server pushes. It disables its own game controls while the opponent acts. At end of turn it skips copying the paste-this-prompt text, and its toast, when the assistant is already waiting.

*Technical detail:* [context.md#task-board-shows-the-opponent-status](./context.md#task-board-shows-the-opponent-status)

**Acceptance criteria**:
- [x] The opponent's name on the board carries the matching message while the assistant waits, acts or has stopped responding, and nothing otherwise.
- [x] The board's game controls are disabled while the opponent is acting and come back when it finishes.
- [x] Ending a turn while the assistant is waiting shows no "paste this prompt" message; with no assistant waiting, the prompt is copied as before.
- [x] The board app builds.

### Milestone 4: Drills and played-out lines flow without "done"

**What changes**: In review mode, when the assistant sets the player a replay drill or plays a line out with the player taking one seat, it waits on the seat it handed over. It moves on, comparing the drill line or playing the other seat's reply, as soon as the player ends that seat's turn on the board. The player never has to say "done". The coaching guides teach this step.

**Validation point**: the review-mode wait tests (only the handed-over seat ends the wait) and the extended guide tests pass. By hand, one replay drill and one played-out line in a fresh session complete with no "done" message.

#### - [x] Task: Guides teach the waiting step
**Id:** c9c3f1ef-7944-4f9e-a602-60f2bde2b69e
**Repo:** tcgreplay
**Depends on:**
- c7463a67-1bee-4a40-989b-f9bc864d7b83 — The wait_for_turn tool
- 62a6e8cf-32fc-4491-944f-c37a06e1935e — Mid-turn promote and respond outcomes
**Execution:** agent

The replay-drill and review-game guides gain a waiting step wherever they hand the player a turn: wait on that seat with `wait_for_turn` instead of asking the player to say they are done, answer any mid-turn outcome, and carry on when the turn ends. The guide tests pin the new step so it cannot be dropped.

*Technical detail:* [context.md#task-guides-teach-the-waiting-step](./context.md#task-guides-teach-the-waiting-step)

**Acceptance criteria**:
- [x] When the replay-drill guide asks the player to play the drill turn on the board, it tells the assistant to wait on that seat and move on to comparing when the turn ends, never asking for "done".
- [x] When the review-game guide lets the player play a turn themselves or play a line out against the assistant, it tells the assistant to wait on the player's seat and play the other seat's reply when the turn ends.
- [x] The guides still pass every existing guide rule (no "Claude" or "Nic ", real tool names only, turns named by round and seat), and the tests fail if the waiting step is removed.

## Open Questions

- **Does every MCP client in use tolerate a ~30-second tool call?** This depends on the real tool-call timeouts of Claude Code and the claude.ai connector, which can only be seen by exercising them. If a client times out before the cap, lower the default cap (the setting exists for this) and record the new default. If any client times out under about 10 seconds, STOP and ask the user, because the long-poll shape would need rethinking.
- **Does the go-sdk cancel the handler context promptly when a desktop shim or hosted client disconnects mid-wait?** This depends on go-sdk v1.6.0's behaviour over the shim's socket relay and over Streamable HTTP, which will show up in the supersede and cancellation tests. If it does not cancel promptly, the per-call cap still bounds each call and the design stands. Note it in the implementation and carry on, unless the "Opponent is waiting" badge cannot be made to clear within 60 seconds; in that case, STOP and ask.

## Out of Scope

- **No turn timer against the player** (spec Non-Goals). The five-minute check only pauses the assistant and asks "Still playing?". It never ends the player's turn, forfeits the game or moves for the player.
- **No rules engine.** The wait reports what happened on the board; the playmat does not decide whether a card needs a response, whether a move is legal, or which Pokémon to promote. The assistant judges all of that from card text.
- **No new `play-game` guide.** The loop is taught by the standing instructions, the tool's description and each result's `next` line, plus the replay-drill and review-game guides. A dedicated guide can follow if real sessions show the loop needs more room.
- **No push channel or server-initiated notifications to the assistant.** Waiting is a tool call the assistant makes, as the spec requires.
- **No finer-grained lock rules.** While the assistant acts, the board refuses all game actions rather than allowing some overlapping moves (spec Technical Approach).
- **No support for several assistants waiting on one game at once.** One wait session per game manager; a second caller's wait supersedes the first.
- **No automated tests for the board app.** The project has no frontend test suite; the badges and disabled controls are checked by hand (see Testing Approach).

## Changelog

### 2026-10-26 — Task: Game notices say what changed

**What was done**: `game.Update` gained a `Change` kind (`mode`, `checkout`, `rewind`, `undo`; empty for new events). `SetMode` now notifies listeners on a real switch and stays silent on a no-op switch, and `checkoutLocked` carries the kind from `Checkout`, `Rewind`, `JumpTo` (rewind) and `Undo`. The "Your turn in tcgreplay …" hand-off moved into `Game.HandOff(since)`, which the board's end-turn now calls, and the game gained the unexported `eventsSince` and `actionOf` reads for the coming wait.

**Deviations**: `HandOff` takes the `since` cursor (as context.md says) rather than a seat (as plan.md's Data Structures says): both callers already hold the cursor, and the turn's seat comes from the state.

**Files changed**:
- `internal/game/game.go`
- `internal/game/reads.go`
- `internal/game/ui.go`
- `internal/webui/actions.go`
- `internal/webui/hub.go`
- `internal/game/game_test.go`
- `internal/webui/webui_test.go`

**Discoveries**: `SetMode`'s notice is sent with `g.mu` held, like every other game notice, so presence listeners in the next task must only record and signal. A mode switch made by the assistant now refreshes open boards through the hub (it did not before). `eventsSince` walks parents from the head back to the cursor and reports false when the cursor is not on the current branch, which the wait can use to spot a moved board.

### 2026-10-26 — Task: The presence and the turn wait

**What was done**: Added `internal/game/presence.go`: a per-manager presence (made lazily on first use) that subscribes once to each game the manager makes current, holds a single wait session, and wakes waiting calls through a channel it closes and replaces on every change. `Manager.Wait` long-polls up to `Timing.WaitCap` and reports `turn_ended` (with turn, seat, the shared hand-off prompt and cursors), `check_in` after `Timing.CheckIn` without board activity, `ended` with a reason (mode_changed, game_replaced, game_expired, game_closed, branch_switched, rewound) or `still_waiting`; a newer call supersedes an older one. The manager gained `Timing`, `BoardActivity`, `OnPresence`, and calls into the presence from `setCurrent` and `Expire`.

**Deviations**: The 20 s "waiting" display grace timer listed for this task is left to Task 7, the only place that reads it (`Opponent()`). A fresh wait's starting point was made more precise than the plan text: when it is not the handed-over seat's turn, review mode waits for that seat's next turn (cursor = head), and play mode accepts the wait only when the player's turn ended with no non-system event since (the hand-off is still pending), otherwise it refuses with "it is X's turn: end_turn first, then wait". A continuing call with no live session starts one at its `since`; a cursor that is no longer on the current branch ends the wait as `rewound`. A cancelled call returns `still_waiting` with `ctx.Err()`.

**Files changed**:
- `internal/game/presence.go`
- `internal/game/manager.go`
- `internal/game/presence_test.go`

**Discoveries**: `BoardActivity` has no caller until Task 7 wires the board dispatch to it, so until then the check-in clock runs from the start of the session only. `presence.mu` must never be held while calling into a `Game` (game listeners hold `g.mu` and take `p.mu`); `Wait` therefore reads the game through one `waitView` call between lock sections and re-evaluates from scratch after each wake. `Manager.Expire` is called by the hosted registry with its own lock held, so the presence's hooks must stay non-blocking.

### 2026-10-26 — Task: The wait_for_turn tool

**What was done**: Added the `wait_for_turn` MCP tool (`internal/mcpserver/wait.go`). It resolves the seat (play mode: the player's seat only; review mode: the named seat or the seat whose turn it is), runs `Manager.Wait`, and returns `{outcome, seat, since, turn, turn_seat, prompt, log, reason, next}` with the player's moves rendered in the view `get_log` uses and a `next` line for each outcome. The server gained an optional `ResolveQuiet` hook (with `bindWith`/`addQuiet`/`addWith`), which the wait uses in place of `Resolve`, and the standing instructions gained the play-loop line.

**Deviations**: The tool's handler and result type live in a new file, `internal/mcpserver/wait.go`, with its tests in `internal/mcpserver/wait_test.go`, rather than inside `server.go`/`server_test.go`. Every outcome carries the log, `still_waiting` included; an `ended` outcome carries it only while the cursor is still on the board's current branch (a branch switch or rewind leaves no moves to read), using the new `Game.OnCurrentBranch`. The goal check's first attempt flagged the missing log on `ended`; the second attempt passed.

**Files changed**:
- `internal/mcpserver/server.go`
- `internal/mcpserver/wait.go`
- `internal/mcpserver/wait_test.go`
- `internal/mcpserver/server_test.go`
- `internal/game/reads.go`

**Discoveries**: `TestGuideInstructions` and `TestGuidesNameRealTools` check every snake_case word in the instructions against the real tool list, so the new instruction line can only name tools that exist (`wait_for_turn`, `end_turn`). A `still_waiting` result repeats the player's moves since the cursor on each poll, because the cursor only advances on `turn_ended` (and, from Task 6, on `respond`).

### 2026-10-26 — Task: Waiting does not keep a hosted game alive

**What was done**: The per-account registry gained `ResolveMCPQuiet`, which makes the same sign-in checks as `ResolveMCP` but finds the account's manager and board link through `Registry.Get`, so it records no activity and takes no place. Hosted mode now passes it to the MCP server as `ResolveQuiet`, so `wait_for_turn` calls never keep an abandoned game alive, and the idle expiry reaches the waiting call through the manager's `Expire` and ends it with `game_expired`.

**Deviations**: The optional end-to-end hosted test over HTTP (`cmd/tcgreplay/hosted_idle_test.go`) was not added; the registry-level tests cover both the quiet lookup and a wait ended by the idle sweep.

**Files changed**:
- `internal/usergames/mcp.go`
- `cmd/tcgreplay/hosted.go`
- `internal/usergames/mcp_test.go`
- `internal/usergames/idle_test.go`

**Discoveries**: `Registry.Get` also creates an entry (without a place) for an account that has none, and runs the idle expiry, so a quiet lookup at exactly the idle time keeps the game and one second later expires it, the same boundary as a board's live connection.

### 2026-10-26 — Task: Adjustable wait timings

**What was done**: The binary gained `-wait-cap`, `-check-in` and `-lock-release` (with `TCGREPLAY_WAIT_CAP`, `TCGREPLAY_CHECK_IN` and `TCGREPLAY_LOCK_RELEASE` as their defaults, else 30s, 5m and 2m). A flag overrides its variable, even an invalid one. `validTiming` refuses a cap of 0, below 0 or above 50s and a negative check-in or lock release, and main exits with status 2 and the message before anything starts. The settings are passed to the daemon through `config.args()` and set on the manager built by `newServer` (daemon and in-process) and on every hosted account's manager.

**Deviations**: Validation runs in `main` for every run mode, straight after the flags are parsed (exit 2 with the message on stderr), rather than per mode. A zero check-in or lock release is turned into its default in `main`, so the daemon receives explicit values. A new start-up log fragment, `turn waits up to … a call, check-in after …, lock release after …`, is added to the hosted, daemon and in-process start-up lines, so the effective values can be seen (and are asserted by the end-to-end test). The tests live in a new `cmd/tcgreplay/timing_test.go` rather than in `main_test.go`/`hosted_test.go`. `.do/app.yaml` is unchanged (the defaults apply).

**Files changed**:
- `cmd/tcgreplay/main.go`
- `cmd/tcgreplay/hosted.go`
- `cmd/tcgreplay/daemon.go`
- `cmd/tcgreplay/timing_test.go`

**Discoveries**: `envDuration`'s error message ends "0 to never drop", which is specific to the idle timeout; the wait settings use a thin `envWait` wrapper with their own hint. `config.args()` leaves unset (zero) timings out, so code and tests that build a `config` literal without timings still spawn a daemon that falls back to its defaults.

### 2026-10-26 — Task: Mid-turn promote and respond outcomes

**What was done**: The wait now stops mid-turn. It reports `promote` once per vacancy of the assistant's Active Spot, and `respond` with each card, Ability or attack the handed-over seat's human player used (iid, action name, card id, kind and the text of what was used), de-duplicated by action id. Either outcome opens a response window (`Game.responder`) that lets the assistant's seat act during the player's turn in play mode. The window closes at the start of the next wait call, when the session ends, on end turn, on a mode switch and on a branch move. `wait_for_turn` renders `cards` and a `next` line for each outcome.

**Deviations**: `respond` and `promote` keep the cursor at the turn's start rather than moving it to the head (as context.md suggested): reported cards are de-duplicated by action id instead, so the final `turn_ended` hand-off and its "get_log since N" still cover the player's whole turn. A card's `name` is the action label ("Judge", "Riolu: Punch"), which names the Ability or attack used. The goal check's first attempt flagged that the assistant could still slip into the player's turn by joining an action the player left open (the existing cross-seat join). The user decided the assistant plays as the opponent, so in play mode an assistant (`claude` actor) move that joins the other seat's open action now also needs the response window, while the player's (`human`) side of the assistant's cards joins as before. The second attempt passed.

**Files changed**:
- `internal/game/presence.go`
- `internal/game/game.go`
- `internal/game/verbs.go`
- `internal/game/reads.go`
- `internal/mcpserver/wait.go`
- `internal/game/presence_respond_test.go`
- `internal/mcpserver/wait_test.go`

**Discoveries**: Cross-seat verbs (draw, move, shuffle, switch) skip the turn lock whenever they join the other seat's open action, so any "who may act when" rule has to cover that join path as well as `checkTurn`. A fresh wait (no `since`) re-reads the whole turn and so re-reports cards already answered, so the loop must always continue with `since`.

### 2026-10-26 — Task: The acting lock and opponent status on the board server

**What was done**: The presence gained the acting lock and the board's opponent status. `turn_ended`, `respond` and `promote` outcomes lock the board, and so does an assistant write on its own turn in play mode. Every assistant tool call refreshes the lock. A new wait call, the assistant's `end_turn`, and any game change (mode, branch, rewind, undo, new game, expiry) release it. With no move for `LockRelease`, a single reset-not-stacked timer releases it and marks it `stopped`. `Manager.Locked`, `Opponent`, `AssistantMoved(ownTurn)` and `AssistantDone` expose this. The board's dispatch notes every request as player activity and refuses everything but reads, replays, the reveal toggle and set_mode while locked, with "the opponent is acting; wait for them to finish". The hub's state message carries `opponent` and is pushed on every presence change. The board's `end_turn` adds `waiting: true` when the assistant was waiting.

**Deviations**: The lock guard is one check before the board's first switch, with an allow-list (`lookVerbs`), rather than separate guards on `end_action`/`checkout`/`rewind` and the write switch. The 20 s "waiting" grace (deferred from Task 2) lands here as the package variable `waitingGrace`, with its own reset-not-stacked timer that pushes the change when it lapses. `AssistantMoved(ownTurn)` takes the play-mode own-turn decision from the MCP layer (`verb` checks mode and that it is `ClaudeSeat`'s turn after the write), rather than the presence working it out. A plain tool call never re-locks after a self-release; only an own-turn write does. The board's `waiting` flag is read before `EndTurn`, because a waiting assistant flips to acting the moment the turn ends. Tests are in new files `internal/game/presence_lock_test.go`, `internal/webui/lock_test.go` and `internal/mcpserver/lock_test.go`.

**Files changed**:
- `internal/game/presence.go`
- `internal/webui/actions.go`
- `internal/webui/hub.go`
- `internal/webui/server.go`
- `internal/mcpserver/server.go`
- `internal/game/presence_lock_test.go`
- `internal/webui/lock_test.go`
- `internal/mcpserver/lock_test.go`

**Discoveries**: `set_mode` is the player's escape hatch: it is allowed while locked and, as a game change, releases the lock. A board `end_action` with nothing open pushes no state message, so tests that need a fresh state after it must read `Opponent()` directly. The real 20 s grace can only be bounded in tests outside the game package (the variable is unexported).

### 2026-10-26 — Task: Board shows the opponent status

**What was done**: The board app keeps the server's `opponent` status in its store and shows "Opponent is waiting", "Opponent is acting" or "Opponent stopped responding" as a badge beside the opponent's seat name. While the opponent acts, `canAct()` is false (which disables Draw), the End action and End turn buttons are disabled, and cards and Pokémon stacks cannot be dragged. Ending a turn while the assistant is waiting shows a plain "Turn ended." toast instead of copying the paste-this-prompt text; otherwise the prompt is copied as before.

**Deviations**: A small `oppActing()` helper was added to the store for the buttons that do not go through `canAct()`. Setup-phase buttons (Mulligan, who goes first) are not gated, since the assistant never acts during setup; the server refuses locked actions regardless. Verified with `make build` after `npm ci` inside the worktree; there is no front-end test suite, so the badges and disabled controls stay a manual check.

**Files changed**:
- `web/src/lib/store.svelte.js`
- `web/src/lib/ws.js`
- `web/src/lib/actions.js`
- `web/src/App.svelte`
- `web/src/components/Strip.svelte`
- `web/src/components/Card.svelte`
- `web/src/components/Stack.svelte`

**Discoveries**: A fresh worktree has no `web/node_modules`; `npm ci` in `web/` is needed before `make build`. The build output (`internal/webui/dist/app/`) and `bin/` are git-ignored.

### 2026-10-26 — Task: Guides teach the waiting step

**What was done**: The replay-drill guide now tells the assistant, when the player plays the drill turn on the board, to call `wait_for_turn` on their seat, follow each result's `next`, and go straight to Compare when the turn ends, never asking them to say they are done. Compare reads their line from the result's log. The review-game guide's step 2.4 waits on the player's seat instead of reading `get_log` afterwards. "Test the alternative" gains the played-out-line loop: play the other seat's turns, and after each `end_turn` wait on the player's seat and reply when their turn ends. `TestGuidesKeepTheirRules` pins each new step by its own wording.

**Deviations**: The goal check's first attempt found the anchor too loose (a bare `wait_for_turn` also appeared elsewhere in each guide, so removing a step would not fail the test). The anchors now quote each step's own sentence, and removing either guide's step was confirmed to fail the test. The second attempt passed.

**Files changed**:
- `internal/guides/replay-drill.md`
- `internal/guides/review-game.md`
- `internal/guides/guides_test.go`

**Discoveries**: An anchor in `TestGuidesKeepTheirRules` must be a phrase unique to the step it protects; a tool name repeated across a guide does not pin anything.
