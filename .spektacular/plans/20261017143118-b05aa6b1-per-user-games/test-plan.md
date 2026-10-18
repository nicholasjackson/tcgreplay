---
created_date: "2026-10-18"
document_status: final
closed_date: "2026-10-18"
---

# Test plan: 20261017143118-b05aa6b1-per-user-games

Manual checks for per-account games on the hosted playmat (https://tcgreplay.demo.gs). The automated tests cover these behaviours with fake Google sign-in, an injected clock and in-process servers. What remains needs real Google accounts, real claude.ai connectors, the deployed app, wall-clock time or the desktop app.

**Who / when**: the owner, after the branch is merged to `main` and App Platform has deployed it (the deploy is green and `GET https://tcgreplay.demo.gs/healthz` answers `ok`). Run section 0 first. Sections 1–7 need two Google accounts, called **A** and **B** here, and a third (**C**) for section 5. Use separate browser profiles, so each profile holds one account's sign-in.

Settings live in the App Platform console under Apps → tcgreplay → Settings → App-Level Environment Variables. Saving a change there restarts the app with no rebuild, and every restart clears all games and sign-ins.

## 0. Deployed app settings (from the human task's open criteria)

- **What**: the app runs exactly one instance, and the idle setting takes effect from the console with no rebuild.
- **How**:
  1. In the console, open Settings → `playmat` component → Resources and check the container count is **1**, with no autoscaling.
  2. Open Runtime Logs and find the start-up line `tcgreplay <version> hosted: listening on …`.
  3. Change `TCGREPLAY_IDLE_TIMEOUT` from `60m` to `2m` (App-Level, Run time) and save. When the app has restarted, find the new start-up line.
- **Expected**:
  - Step 1 shows 1 container.
  - The first start-up line ends with `…, 10 users, idle games dropped after 1h0m0s`.
  - After the change it ends with `idle games dropped after 2m0s`, and the deploy did not rebuild the image.
- Leave the setting at `2m` for sections 4 and 5, then set it back to `60m` at the end (section 8).

## 1. Success metric and review: two accounts, real claude.ai, isolation

- **What**: two signed-in accounts use the server at the same time for a full session, and neither sees the other's game. Pass is 0 sightings of the other account's game.
- **How**:
  1. In profile A, add a claude.ai custom connector at `https://tcgreplay.demo.gs/mcp` and sign in with Google as A. Do the same in profile B as B. If a connector already exists from before sign-in, remove it and add it again.
  2. In A's conversation, paste a TCG Live battle log and ask Claude to import it (it calls `import_log`). In B's conversation, import a different log.
  3. Each opens the `board` link Claude returns, of the form `https://tcgreplay.demo.gs/?board=<handle>`.
  4. In A, have Claude make some moves (for example a `draw` or a `move`). In B, ask Claude for the state (`get_state`).
  5. Carry on for a full review session, about 30 minutes, with both accounts active.
- **Expected**:
  - Each conversation and board shows only its own game: the game id in the board header, and the players' names and cards.
  - A's moves never appear in B's `get_state` or on B's board, and the reverse.
  - The two board links have different `board=` handles.

## 2. Review: same account on two devices

- **What**: one account's conversations, tabs and devices share one game.
- **How**:
  1. Sign in as A on a laptop (claude.ai plus the board tab) and on a phone (claude.ai app or browser with the connector).
  2. From the phone conversation, ask Claude to make a move.
  3. Watch the laptop's board tab, then ask the laptop conversation for `get_state`.
  4. From the laptop, import a different log.
- **Expected**:
  - The move shows on the laptop's board tab within 2 seconds, and the laptop conversation's next read includes it.
  - After the laptop import, both conversations and both boards show the new game.

## 3. Review: owner-only link in a browser

- **What**: a board link opens only for its owner.
- **How**:
  1. Copy A's board link (`https://tcgreplay.demo.gs/?board=<A's handle>`).
  2. Open it in B's browser profile, signed in as B.
  3. Open it in a private window with no sign-in.
- **Expected**:
  - B sees a page titled **Not your game** (HTTP 403) saying "This board belongs to another account. You are signed in as <B's email>". It has an "Open your own board" link and a **Sign out** button, and no board, cards or game details.
  - The private window is sent to Google sign-in before anything is shown, and no cards appear.
  - Clicking "Open your own board" in B's profile shows B's own board.

## 4. Review: idle drop on the deployed app (idle time 2m)

- **What**: an idle game expires after 2 minutes, an open tab does not keep it, and activity does.
- **How** (with `TCGREPLAY_IDLE_TIMEOUT=2m` from section 0):
  1. As A, import a game through claude.ai and open its board. Keep the board tab open and do nothing in Claude or on the board for 3 minutes.
  2. Ask Claude for the board state.
  3. Ask Claude to import the saved game record it returned earlier (pass the record to `import_log`).
  4. Then make a move, and make one action every minute for 5 minutes: alternate a Claude tool call and a board move.
- **Expected**:
  - Within 30 seconds after the 2 minutes pass, the open board switches, without a reload, to "No game open" with the text "Your game expired after 2 minutes without activity and was cleared. Import its saved game record again (import_log with the record) to carry on." The open tab did not stop the expiry.
  - Claude's `get_state` returns that same message.
  - Runtime Logs show `usergames: game for <A's email> expired after 2 minutes idle`.
  - After the re-import the game is back, card for card.
  - After 5 minutes of activity every minute, the game is still there with every move intact.

## 5. Review: an expired game frees a place (user cap 1)

- **What**: with the server full, a new account can sign in once another account's game expires.
- **How**:
  1. Temporarily add or set `TCGREPLAY_MAX_USERS=1` (App-Level, Run time) with `TCGREPLAY_IDLE_TIMEOUT=2m`, and save. The app restarts.
  2. As A, sign in (board or connector) and import a game.
  3. As C, open `https://tcgreplay.demo.gs/` and sign in with Google.
  4. Wait 2 minutes with A idle, then sign in as C again and ask C's Claude to import a game.
  5. Now act as A, for example ask A's Claude for `get_state`.
- **Expected**:
  - Step 3 shows "tcgreplay is busy" (HTTP 503): "tcgreplay is serving as many signed-in users as it can (1) right now."
  - In step 4, C gets in and the import works.
  - In step 5, A is told "the server is busy: it already has the most signed-in users it serves at once (1); try again later."
  - Afterwards, set `TCGREPLAY_MAX_USERS` back to `10`, or delete it so the default of 10 applies.

## 6. Review: own name on the board

- **What**: each account's board labels the player's side with that account's own name.
- **How**:
  1. As A, ask Claude to start a game with `new_game` and no player names, and open the board.
  2. Then import a battle log.
  3. Repeat steps 1–2 as B.
- **Expected**:
  - For the `new_game` game, the player's side reads `<A's email before the @> · p1`, not "You".
  - For the imported game, it reads the log owner's name.
  - B's board shows B's name or B's import's name, never A's.

## 7. Review: desktop regression

- **What**: the desktop keeps one game, no sign-in and no board-link handle.
- **How**:
  1. Install this build (`make install`) and restart Claude Desktop (or use `make dev` with Claude Code's `tcgreplay-dev` server).
  2. Import a log and open http://127.0.0.1:7777/ (7778 for dev).
  3. Start a second Claude conversation and ask it for `get_state`.
  4. Start a `new_game` with no names.
- **Expected**:
  - No sign-in is asked for anywhere.
  - Tool results carry no `board` key, and the board header shows no account and no Sign out button.
  - Both conversations see the same one game.
  - The player's side of the `new_game` game reads "You".
  - Leaving the desktop idle for over an hour does not clear the game.

## 8. Success metric: memory over a week

- **What**: over a week with the idle drop on, memory stays within what the container is given. The instance is `apps-s-1vcpu-0.5gb`, so the limit is 512 MiB.
- **How**:
  1. Set `TCGREPLAY_IDLE_TIMEOUT` back to `60m`.
  2. For 7 days of normal use, check App Platform → Insights → Memory each day.
  3. Search Runtime Logs for `usergames:` lines: `usergames: N accounts, M holding places, K games` after each sweep that changed something, plus expiry and removal lines.
- **Expected**:
  - Peak memory stays below 512 MiB, with no out-of-memory restarts in the deployment history.
  - "holding places" never exceeds `TCGREPLAY_MAX_USERS`.
  - "accounts" falls back after quiet days, because accounts without a place are removed after 24 hours.
  - If memory nears the limit while places held stay within the cap, record the logged counts with the memory graph and ask whether to lower the user cap or the idle time, or to resize the instance. Do not add game storage or a second limit.
