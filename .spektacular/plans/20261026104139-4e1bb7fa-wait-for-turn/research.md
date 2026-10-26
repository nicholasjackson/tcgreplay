---
created_date: "2026-10-26"
document_status: draft
---

# Research: 20261026104139-4e1bb7fa-wait-for-turn

## Alternatives considered and rejected

- **Push from the playmat to the assistant (server-initiated MCP notification / sampling).** Rejected: the spec's Technical Approach prefers a blocking tool, and MCP clients (Claude Code, claude.ai connectors) do not turn server notifications into a new model turn. The only channel is a tool call the assistant makes (spec Constraints, "existing assistant tool connection").
- **One blocking call per player turn (no cap).** Rejected: violates the "<1 minute per call" constraint; hosted `/mcp` has no WriteTimeout (`cmd/tcgreplay/hosted.go:226`) but clients and proxies do. Long-poll with a ~30 s cap and a cursor instead.
- **Subscribing a listener per wait call with `Game.Subscribe`.** Rejected: `Subscribe` has no unsubscribe (`internal/game/game.go:128-133`), so every wait would leak a closure on the game: the spec's named risk and its memory success metric. Chosen instead: one presence object per manager, subscribed once per game when it becomes current (the hub's pattern, `internal/webui/hub.go:57-76`), waking waiters through a replace-on-change broadcast channel, so a wait registers nothing that outlives it.
- **Stateless waits (cursor only, no server-side session).** Rejected: game changes that happen between two calls (mode switch, branch switch, rewind, a new import, hosted expiry) would be invisible or misreported, and the five-minute clock must restart after "Still playing?" without board activity. A single per-manager wait session (bounded: one struct, replaced on each fresh wait) records the reason a wait ended as the change happens.
- **A playmat-side list of cards that need an opponent response (Judge, Unfair Stamp, Iono …).** Rejected by the spec (Technical Approach, "report each card or effect"; Requirement "Assistant judges what its side must do"). The wait reports every opener (trainer, ability, attack) the handed-over seat uses, with its card text, and the assistant decides.
- **Finer-grained overlap rules for the lock.** Rejected by the spec ("whole-board lock"). One lock refusing every board game action while the assistant acts.
- **Resolving waits through the hosted `Resolve` hook as-is.** Rejected: `ResolveMCP` calls `Registry.Use`, which records activity (`internal/usergames/mcp.go:20-33`, `usergames.go:99-114`), so a waiting assistant would keep an abandoned game alive, contradicting "Waiting does not keep a game alive". The existing no-activity lookup `Registry.Get` (`usergames.go:159-166`) is the right primitive.
- **A new `play-game` guide.** Rejected for now: guide tests pin exactly six guides (`internal/guides/guides_test.go:12,173-191`, `guides.go:17`), and the loop fits in one standing-instruction line plus self-describing wait results (each outcome says what to do next). The two guides that hand turns over (replay-drill, review-game) gain the waiting step.
- **Hard-coded timing constants.** Rejected: spec Technical Approach wants adjustable settings. Follow the `-idle-timeout`/`TCGREPLAY_IDLE_TIMEOUT` pattern (`cmd/tcgreplay/main.go:84-100`).

## Chosen approach — evidence

- Hand-off text already exists: board `end_turn` builds "Your turn in tcgreplay (game %s, branch %s). Call get_log since %d and play turn %d (%s's turn)." from `g.TurnStartedAt(seat)` (`internal/webui/actions.go:256-264`, `internal/game/ui.go:145-163`). Factor it into one game function used by the board and the wait result (spec: "build the hand-off on the your-turn message").
- Every board write funnels through `webui.Server.do` (`internal/webui/actions.go:107-272`): one place to refuse actions while locked and to record player activity. Non-game controls (`set_mode`, `set_reveal`, `replay`, reads) are handled in the first switch (`actions.go:127-180`), game writes in the second (`actions.go:182-267`).
- Every assistant game tool funnels through `add`/`bind` (`internal/mcpserver/server.go:174-188`, `110-131`): one place to refresh the "assistant last moved" clock.
- Manager already owns the current game and fires hooks on replace (`setCurrent`, `manager.go:168-178`, used by `New` and `import.go:136`) and on expiry (`Expire`, `manager.go:119-128`); in hosted mode there is one manager per account (`usergames.go:64-81`), so per-manager presence is automatically per-account.
- Game listeners run with the game lock held (`game.go:404-412`, `reads.go:576-592`): presence listeners must only record and signal, never call back into the game.
- Branch moves notify with an `Update` carrying no events (`checkoutLocked`, `reads.go:576-592`), used by `Checkout`, `Rewind`, `JumpTo` (board "rewind" verb, `actions.go:154-160`) and `Undo`. `SetMode` does not notify at all (`game.go:143-151`), nor does the board's `set_mode` (only a hub refresh, `actions.go:137-142`). Both need a change kind on `Update` so the wait can name the reason.
- Turn ends are `EvTurn` events with phase `end` and the ending seat (`verbs.go:155-191`); openers are actions of kind `trainer`, `ability`, `attack` (`verbs.go:272,312,402`) carrying `Source` (the card iid). Promotion is allowed any turn (`Switch` with empty Active is `anyTurn`, `verbs.go:380-391`). Opponent-side effect verbs (`draw`, `move`, `shuffle`, `switch`) are `crossSeat` and skip the turn lock only while joined to the open action of the other seat (`game.go:336-348`).
- Hub state message is built in one place (`hub.stateMessage`, `hub.go:137-172`) and can carry an `opponent` status; the hub already exposes `wake()` for non-event refreshes (`hub.go:88-93`).
- Board UI: `canAct()` gates buttons (`web/src/lib/store.svelte.js:103-108`); `endTurn` copies the prompt to the clipboard and toasts "paste it to <assistant>" (`web/src/lib/actions.js:350-360`), which must not happen when the assistant is waiting.
- Hosted idle sweep runs every min(idle/4, 30 s) (`usergames.go:225-246`) and `Expire` fires `OnClear` hooks, so an expiry reaches a waiting call well within 60 s.
- go-sdk handlers receive the request context; in-memory and Streamable HTTP transports are both exercised by `forEachTransport` (`internal/mcpserver/server_test.go:22-76`).

## Files examined

- `internal/game/game.go:43-49` — `Update{Head, Branch, Events, Delta}`; no change kind.
- `internal/game/game.go:128-133` — `Subscribe` appends, no unsubscribe.
- `internal/game/game.go:143-151` — `SetMode` changes mode silently.
- `internal/game/game.go:325-393` — `run`/`runLocked`: turn lock, crossSeat joins, notify.
- `internal/game/game.go:417-434` — `checkTurn`: review mode never locked; play mode only the current seat.
- `internal/game/manager.go:21-41,89-178` — Manager fields, `OnLoad`, `OnClear`, `Expire`, `Current`, `setCurrent`.
- `internal/game/verbs.go:155-191` — `EndTurn` emits end + next turn start + draw.
- `internal/game/verbs.go:272,312,380-391,402,511,537` — opener kinds, promote anyTurn, crossSeat verbs.
- `internal/game/reads.go:453-592` — `Rewind`, `JumpTo`, `Undo`, `Checkout`/`checkoutLocked` notify.
- `internal/game/ui.go:12-24,38-110,145-163` — `UISnapshot` fields; `TurnStartedAt`.
- `internal/state/events.go:8-15` — actors human/claude/system/import.
- `internal/webui/actions.go:83-272` — board dispatch, Limit hook, end_turn prompt.
- `internal/webui/hub.go:29-181` — hub watch/clear/wake/stateMessage.
- `internal/webui/server.go:29-55` — `Server{M, Human, Public, Limit}`, `New` wires OnLoad/OnClear.
- `internal/mcpserver/server.go:17-34` — `Server` fields incl. `Resolve`.
- `internal/mcpserver/server.go:61-80` — standing `instructions` const + guide block.
- `internal/mcpserver/server.go:110-188` — `bind`, `m`, `add`.
- `internal/mcpserver/server.go:194-226` — `acting` seat rules, `verb`.
- `internal/mcpserver/server.go:452-462,663-665,761-803` — set_mode, end_turn, rewind/undo/checkout tools.
- `internal/mcpserver/guides.go` — guide tool bypasses `bind`; `guideInstructions()`.
- `internal/usergames/usergames.go:93-196` — `Use` (activity, place), `Get` (no activity), `expireIfIdle`.
- `internal/usergames/mcp.go:17-33` — `ResolveMCP` uses `Use`.
- `internal/usergames/board.go:56-73` — `BoardLimit` counts board actions as activity.
- `cmd/tcgreplay/main.go:40-100,136-175` — flags/env pattern, `config.args()` passes flags to the daemon, `newServer`.
- `cmd/tcgreplay/hosted.go:150-200` — hosted MCP wiring with `Resolve: reg.ResolveMCP`, NewManager per account.
- `internal/guides/replay-drill.md:18-24` — drill hands the turn over ("play the turn on the board").
- `internal/guides/review-game.md:33-46` — "If the player wants to play the turn themselves on the board…", test-the-alternative section.
- `internal/guides/guides_test.go:35-266` — guide format tests, six-guide pin, `TestGuidesKeepTheirRules` anchors, banned words.
- `internal/mcpserver/guides_test.go:166-180` — guides may only name real tools (`wait_for_turn` must exist before guides name it).
- `internal/mcpserver/server_test.go:22-91,408-448` — transports, `connect`, `call`, `TestToolList`.
- `internal/webui/webui_test.go:83-156,276-306` — websocket test helpers, `TestEndTurnCopiesPrompt`.
- `internal/usergames/idle_test.go:15-130` — idle registry helpers with fake clock.
- `web/src/lib/store.svelte.js`, `web/src/lib/ws.js`, `web/src/lib/actions.js:350-360`, `web/src/App.svelte` — board state, act(), end-turn prompt copy, header. No frontend test suite exists (`web/package.json`).
- `Makefile` — `make test` = `go vet ./... && go test ./...`; `make build` builds the web app first.

## External references

- Model Context Protocol, tools and cancellation (modelcontextprotocol.io spec) — a tool result is the only way a server's information reaches the model; cancelled requests cancel the handler context.
- go-sdk v1.6.0 Streamable HTTP (`mcp/streamable.go`) — handler context ends with the HTTP request; see knowledge entry `gotchas/go-sdk-sessions-and-auth.md`.

## Prior plans / specs consulted

- Spec `20261026104139-4e1bb7fa-wait-for-turn` — source of truth; no design references (`design ref list` returned none).
- Plan `20261017143118-b05aa6b1-per-user-games` — activity is any tool call, board action or page load; live connections and `Registry.Get` are deliberately not activity; per-call resolution because `getServer` runs once per session. The wait must use the no-activity path.
- Knowledge `gotchas/go-sdk-sessions-and-auth.md` — per-session state goes stale; look state up per request (the wait resolves its manager per call too).
- Knowledge `gotchas/board-loopback-locks.md` — unrelated to this lock, but the board's live connection rules are untouched.
- Knowledge `conventions/skills-text-and-packaging.md` — guides never say "Claude" or "Nic "; guides stay embedded.

## Open assumptions

- The assistant's mid-turn responses to an effect (Judge, Unfair Stamp) join the player's still-open compound action, so play mode's turn lock lets them through. If the open action has been closed (player pressed End action first), the plan's "response window" lets the assistant's seat act during the other seat's turn while it is responding; if implementation finds that window unsafe or unneeded, STOP and ask.
- MCP clients in use (Claude Code, claude.ai connector) tolerate a tool call that takes ~30 s. If a client times out sooner, the cap setting is lowered, not the design.
- Board "activity" for the five-minute check counts every request the board sends (including reads and replays), not only game writes.
- `set_mode` and `set_reveal` stay allowed while locked (escape hatch); every game write, `end_action`, `checkout` and `rewind` is refused while the assistant acts.
- A hosted account's MCP calls after sign-out are refused by token checks before reaching the wait; the wait still reports "no game open" if it reaches a fresh entry.

## Drafting assumptions

### Chosen direction: per-manager presence + long-poll wait_for_turn (architecture)
- **Decision**: Option A. A presence object owned by each `game.Manager` holds the wait session, acting lock and board-activity clock, subscribed once per game. `wait_for_turn` long-polls on a replace-on-change channel, and the board and MCP entry points enforce and refresh the lock. Key decisions: (1) `game.Update` gains a change kind and `SetMode` notifies; (2) a server-side wait session (one value per manager) records why a wait ended, and a call without `since` starts a fresh session; (3) every opener by the handed-over seat is reported, and the assistant judges the card text; (4) hosted waits resolve through a new `ResolveQuiet` hook backed by `Registry.Get` (no activity); (5) a "response window" relaxes play mode's turn lock for the assistant's seat while it answers a respond outcome; (6) timings are manager settings exposed as flags and env vars, with the cap limited to 50 s or less.
- **Rationale**: the manager is the only object both the board and the MCP tools hold, already sees every game swap or expiry, and is per account in hosted mode. One owner gives bounded state and no listener leaks.
- **Rejected**: Option B, a separate `internal/turnwait` package wired into both servers (needs a per-account mapping and shared construction in two places; Medium-High). Option C, stateless cursor waits with per-call `Subscribe` plus a new unsubscribe (misses changes between calls and cannot restart the five minutes after a check-in; Medium).

### Conventions selected (architecture)
- **Decision**: apply both guide conventions (no "Claude"/"Nic " in guides; never exclude `internal/guides` from builds).
- **Rationale**: the plan edits two guides; the packaging rule guards the hosted image.
- **Rejected**: none dropped; the project has only these conventions.

### No new play-game guide (discovery)
- **Decision**: teach the loop with one standing-instruction line, the wait tool's description, and a `next` line in every wait result; add the waiting step to the replay-drill and review-game guides.
- **Rationale**: the spec makes a new guide conditional on the loop needing more than a line or two; self-describing results keep it short, and guide tests pin six guides.
- **Rejected**: a `play-game` guide (more surface, test churn, duplicate of the result text).

### Lock scope (discovery)
- **Decision**: while the assistant acts, the board refuses every game write plus `end_action`, `checkout` and `rewind`; `set_mode`, `set_reveal`, `replay` and reads stay allowed.
- **Rationale**: "the player can still look at the board"; switching mode must stay available as an escape hatch and already ends the wait.
- **Rejected**: locking everything (traps the player), locking only writes (lets branch moves pull the board from under the assistant).

### Board activity for the five-minute check counts every board request (discovery)
- **Decision**: any request the board sends counts as the player's activity.
- **Rationale**: "every action the player takes on the board"; a player studying the board with deck lists or replays is still playing.
- **Rejected**: counting only game writes (could fire "Still playing?" on a thinking player).

### Timing settings apply in every run mode (data_structures)
- **Decision**: `-wait-cap`, `-check-in` and `-lock-release` (with `TCGREPLAY_*` env vars) work on the desktop shim, daemon, in-process and hosted modes, and are passed to the spawned daemon.
- **Rationale**: the spec requires both run modes and wants the timings adjustable; the `-idle-timeout` pattern already exists.
- **Rejected**: hosted-only settings (the desktop is the main play-mode surface); constants (the spec asks for settings).

### Wait session restarts with a call that omits since (data_structures)
- **Decision**: omitting `since` starts a fresh session (cursor = just before the waited seat's latest turn started); passing it continues, and the next call after a check-in restarts the five minutes.
- **Rationale**: lets the assistant start a new hand-over cleanly even after it rewound or switched mode itself, while still reporting game changes that happen mid-loop.
- **Rejected**: an explicit `resume` flag (one more thing to get wrong); a purely stateless cursor (misses changes made between calls).

### Response window in the turn lock (tasks)
- **Decision**: while a respond/promote outcome is open, play mode's turn check lets the assistant's seat act during the player's turn. The window closes on the next wait call, on session end and on end turn.
- **Rationale**: the assistant's Judge/Unfair Stamp moves must succeed even when the player's compound action has already closed; the window is narrow and server-tracked.
- **Rejected**: relying only on crossSeat joins (fails if the player pressed End action); lifting the turn lock for the assistant entirely (loses play mode's protection).

### Waiting display grace of 20 s (tasks)
- **Decision**: after a still_waiting return the board keeps "Opponent is waiting" for 20 s so the next poll can renew it.
- **Rationale**: the worst case of a 30 s cap plus 20 s grace keeps clearing under the spec's 60 s.
- **Rejected**: clearing at once (the badge would flicker between polls); a grace tied to the cap (could exceed 60 s with a 50 s cap).

## Rehydration cues

- `spektacular spec file read 20261026104139-4e1bb7fa-wait-for-turn`
- `spektacular knowledge always-applied --tier repo --filter tcgreplay`
- Re-read: `internal/game/game.go`, `internal/game/manager.go`, `internal/game/reads.go:453-606`, `internal/webui/actions.go`, `internal/webui/hub.go`, `internal/mcpserver/server.go:17-226`, `internal/usergames/usergames.go:93-166`, `internal/usergames/mcp.go`, `cmd/tcgreplay/main.go`, `cmd/tcgreplay/hosted.go:150-200`, `internal/guides/replay-drill.md`, `internal/guides/review-game.md`, `web/src/lib/{store.svelte.js,ws.js,actions.js}`, `web/src/App.svelte`.
- `make test` to run vet + all Go tests.
