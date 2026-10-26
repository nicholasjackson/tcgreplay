---
created_date: "2026-10-26"
document_status: final
closed_date: "2026-10-26"
---

# Test plan: 20261026104139-4e1bb7fa-wait-for-turn

Manual checks for the success metrics and reviews that the Go suite cannot cover. Everything else (outcomes, lock, timings, hosted quiet lookup, guides) is covered by `make test`.

## Setup (desktop)

1. Build: `make build` (run `npm ci` in `web/` first in a fresh checkout).
2. Use the repo's dev MCP server `tcgreplay-dev` (`.mcp.json`: `./bin/tcgreplay -data .dev -ui-addr 127.0.0.1:7778`) from a **fresh** Claude Code session in the repo root, and open the board at `http://127.0.0.1:7778/`.
3. To exercise the check-in quickly, run the daemon by hand with short timings: `./bin/tcgreplay -daemon -replace -data .dev -ui-addr 127.0.0.1:7778 -log-stderr -check-in 1m -lock-release 1m`. The daemon start-up line names the values in force: `turn waits up to 30s a call, check-in after 1m0s, lock release after 1m0s`.
4. Have two decklists ready, one with Judge (sv01-176) and Unfair Stamp (sv06-165).

## Success metrics

### M1. A full play-mode game with zero "your turn" or "done" messages
- **What:** count of chat messages from the player that say "your turn", "done" or similar, across a whole game. Threshold: **0**.
- **How:** in the fresh session, ask only "start a play-mode game with these two decks, I'm p1" (paste the decks). From then on, play every p1 turn on the board only. Type nothing in chat, except answering "Still playing?" if it is asked.
- **Pass:** the game reaches its end (prizes, deck-out or concession) with the assistant taking every p2 turn after the board's End turn, and no player chat message asking it to go. The board's End turn shows a "Turn ended." toast, never the "Prompt copied" toast.
- **Who / when:** developer, before merging the feature branch.

### M2. A replay-drill session with no "done" message
- **What:** chat messages telling the assistant a drill turn is finished. Threshold: **0**.
- **How:** import a game (`import_log`) and have it reviewed so it has findings (the `review-game` guide), then ask "give me a drill". Play two drill turns on the board, pressing End turn on the board each time.
- **Pass:** after each End turn the assistant moves on to comparing your line by itself (its next message is the comparison), quoting your moves; you never type that you are done.
- **Who / when:** developer, before merging.

### M3. The assistant picks up its turn within a few seconds
- **What:** time from pressing End turn on the board to the assistant's first move appearing on the board. Threshold: **≤ 5 s** typical, never tens of seconds.
- **How:** during M1, time five turn hand-overs with a stopwatch, from the click to the first p2 event in the board's timeline.
- **Pass:** every hand-over ≤ 10 s, median ≤ 5 s. The playmat side alone (wait returning) is under a second and is tested in `TestWaitTurnEndedPromptly`; the rest is the assistant's own response time.
- **Who / when:** developer, during M1.

### M4. Memory and connections do not grow over a long session
- **What:** the daemon's resident memory and open connections over a long session with many waits and a check-in. Threshold: **no steady growth** (RSS within ±20% of its value after the first 10 minutes; connection count flat).
- **How:** run the daemon from Setup step 3. Note its pid: `pgrep -f 'tcgreplay -daemon'`. Play a game for 30+ minutes, including stepping away until "Still playing?" is asked and then answering. Every 5 minutes record `ps -o rss= -p <pid>` and `ss -tnp | grep -c tcgreplay`.
- **Pass:** RSS and connection count level off; no step up per turn or per check-in. Automated cover for listeners, timers and goroutines: `TestWaitNoLeaks`.
- **Who / when:** developer, before release.

## Manual reviews

### R1. Board badges and disabled controls
- **Where:** the board at `http://127.0.0.1:7778/` during M1.
- **Look for:**
  - "Opponent is waiting" appears beside the opponent's name (top seat) while the assistant waits on your turn.
  - "Opponent is acting" (accent colour) appears while it plays its turn or answers your card.
  - "Opponent stopped responding" (pink) appears if you stop the assistant mid-turn (press Esc in Claude Code) and wait the lock-release time (1 min with Setup step 3).
- **Disabled while acting:** Draw, End action and End turn, and you cannot drag cards or Pokémon. Mode switch, the reveal toggle, the timeline replay and the deck list still work. A forced board action shows "the opponent is acting; wait for them to finish".
- **Clearing:** the waiting badge clears within 60 s of the assistant stopping (stop it with Esc mid-wait). It clears immediately after a "Still playing?" check-in.
- **Pass:** every badge reads clearly, appears and clears at the right moments, and controls come back the moment the assistant finishes.

### R2. The assistant follows the loop in a fresh session
- **Where:** a new Claude Code session with the `tcgreplay-dev` server, with no instructions beyond starting a play-mode game (M1) and a replay drill (M2).
- **Look for:**
  - It calls `wait_for_turn` after every `end_turn`, and never asks you to say "your turn" or "done".
  - When you play **Judge**, it shuffles its hand into its deck and draws 4 during your turn. When you play **Unfair Stamp**, it does the same and draws 2. Each time it then waits again, and you carry on with your turn.
  - When you play a card with nothing for it (e.g. Ultra Ball), it makes no move.
  - When you knock out its Active Pokémon by attack, by Ability or with a plain knock-out, it promotes a Benched Pokémon during your turn.
  - After 5 idle minutes (or the short `-check-in`), its next chat message is exactly "Still playing?". After you reply it carries on waiting. If you ended your turn before replying, it takes its turn straight after your reply.
  - In review mode, while it analyses or plays both sides of a line itself, it does not wait and the board shows no opponent status.
- **Pass:** all of the above with no prompting.

### R3. Hosted behaviour through the claude.ai connector
- **Where:** the hosted playmat (staging) with the claude.ai connector signed in.
- **Look for:**
  - The M1 loop works the same through the connector: each `wait_for_turn` call returns within about 30 s with no client timeout, and the board badge shows.
  - An abandoned game still expires. Start a play-mode game, end your turn so the assistant waits, then close the board and stop interacting. Leave it for the configured idle time (`TCGREPLAY_IDLE_TIMEOUT`, e.g. 2m on staging).
- **Pass:** the game expires on schedule (the next tool call or board load shows the expiry notice), and a wait in flight at that moment ends with "the game expired".
- **Open questions to confirm here:**
  - Whether either client times out before the 30 s cap. If so, lower `TCGREPLAY_WAIT_CAP` / `-wait-cap` and record the new default; under about 10 s, stop and rethink.
  - Whether closing the session mid-wait clears the waiting badge within 60 s.
- **Who / when:** developer, on staging before the hosted deploy.
