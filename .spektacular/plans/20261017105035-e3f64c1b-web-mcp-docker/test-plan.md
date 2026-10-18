---
created_date: "2026-10-17"
document_status: final
closed_date: "2026-10-17"
---

# Test plan: 20261017105035-e3f64c1b-web-mcp-docker

These are the manual checks for the hosted playmat. Automated tests already cover tool parity over Streamable HTTP, the board links, the public origin, the board pings, and the in-process latency of a Claude move reaching the board (`cmd/tcgreplay/hosted_test.go`, `internal/mcpserver/server_test.go` and `internal/webui/webui_test.go`, all run by `make test`). Everything below needs real infrastructure, accounts or devices.

**Prerequisite for every deployed check (D):** this branch is merged to `main`, and the owner has done the deferred setup task "Create the App Platform app and its domain" (steps 1–4 in the plan's changelog). In the checks below, `<domain>` is the owner's real domain from `.do/app.yaml`.

**Local checks (L)** need only Docker or podman on a machine with internet access, run from the repo root.

Who runs them: the owner. When: local checks before merging, deployed checks right after the first deploy, and the go/no-go before any further work on the epic.

---

## Success metrics

### M1. End-to-end review from a phone (D)
- **What:** the owner imports and reviews a real game from claude.ai on a phone, with no desktop involved.
- **How:**
  1. On the phone, open a claude.ai conversation with the tcgreplay connector enabled (see R1).
  2. Paste a real TCG Live battle log and ask Claude to import it with a likely opponent deck. Claude calls `import_log` with `opp_deck`, or `import_log` followed by `set_opponent_deck`.
  3. Open the `board` link from the result in the phone's browser.
  4. Ask Claude to go through a turn, making moves, rewinding and annotating.
- **Pass:** every step completes on the phone alone. The board link opens the imported game, and Claude's analysis moves show on the board.

### M2. A 30–60 minute session with no hosting failures (D)
- **What:** no reconnects or failed tool calls caused by hosting during a 30–60 minute review.
- **How:**
  1. Run M1 as a full review lasting at least 30 minutes, including idle gaps of 5 minutes or more.
  2. Note every tool error or connector reconnect message in claude.ai.
  3. Afterwards, check the App Platform runtime logs (`doctl apps logs <app-id> --type run`, or the control panel) for errors and restarts.
- **Pass:** zero failed tool calls and zero connector reconnects attributable to hosting. A hosting failure looks like a timeout, a 5xx, "session not found" or a container restart. Game-rule errors returned by tools don't count.

### M3. A Claude move reaches the open board within 2 seconds (D, behind Cloudflare)
- **What:** the deployed half of the 2 s metric. The in-process half is `TestHostedBoardFollowsClaude`.
- **How:**
  1. With a game imported, open `https://<domain>/` on a second device or tab.
  2. Ask Claude in claude.ai to make a single `move`.
  3. Measure from the moment the tool result appears in claude.ai to the moment the card moves on the board, with a stopwatch or a screen recording. Repeat 5 times.
- **Pass:** every one of the 5 runs is ≤ 2 s, with no page reload.

### M4. Deploying is only a push to `main` (D)
- **How:** make a visible but harmless change on `main`, such as a comment or a log line, and `git push`. Watch the App Platform dashboard without touching it.
- **Pass:** a new deployment starts on its own, finishes, and `https://<domain>/` and `/mcp` keep working. No manual DigitalOcean step is needed.

### M5. Go/no-go for the epic (D)
- **What:** a clear answer to whether claude.ai can drive the hosted board.
- **How:** run R1, R2 and R3.
- **Pass:** record the outcome in the epic.
  - **Go** if claude.ai connects, lists the tools and drives the board.
  - **No-go** otherwise, with the exact blocker: the error text claude.ai shows, whether it asked for OAuth, and which call failed. If claude.ai demands OAuth, stop and raise it with the user. That would pull google-login forward.

---

## Manual reviews

### R1. Connector connects (D)
- **Where:** claude.ai → Settings → Connectors → Add custom connector. Use URL `https://<domain>/mcp` with no authentication.
- **Look for:** the connector saves and shows as connected, and its tool list matches the desktop's. The desktop list is the 44 tools `tcgreplay` registers, from `ping`, `new_game`, `import_log` and `set_opponent_deck` through `rewind`, `checkout` and `annotate`. Compare against `tools/list` from `make dev` or the Claude Code `tcgreplay-dev` server.
- **Pass:** connection succeeds and the tool names match one for one.

### R2. Import and play through the connector (D)
- **Where:** a claude.ai conversation with the connector.
- **Do:**
  1. Ask Claude to import a real log with an opponent deck.
  2. Ask it to move a card, `rewind` to an earlier event, make a branch (`checkout` with a new branch name), and `annotate` an event.
  3. After each step, ask Claude to read the state back (`get_state`, `get_log`, `list_branches`).
- **Pass:** every call returns success, and each read-back shows the change.

### R3. The link opens the right board (D)
- **Where:** the `board` link in the result of `import_log`, `new_game`, `load_game` or `set_opponent_deck` (it is `https://<domain>/`).
- **Do:** open it on a phone, a different device from the one running claude.ai.
- **Pass:** the board shows the game the tool just opened, with the game id and players matching the result.

### R4. The board stays connected (D, behind Cloudflare)
- **Do:**
  1. Open `https://<domain>/` and leave the tab idle and in front for 10 minutes.
  2. Then ask Claude to make one move.
- **Pass:** the move appears on the board without a reload. The board pings every 30 s, so Cloudflare's ~100 s idle limit should not cut the connection. If it drops anyway, try the fallbacks the plan names: JSON responses for `/mcp` and a shorter ping interval.

### R5. The public address setting changes the links (L, or D)
- **Local:**
  1. Run `make docker-build`, then `make docker-run PUBLIC_URL=https://one.example`.
  2. Over `/mcp`, call `new_game`, or `import_log` with a log and deck, and note the `board` value.
  3. Stop the container and run `make docker-run PUBLIC_URL=https://two.example` with no rebuild. Repeat step 2.
- **Deployed:** change `TCGREPLAY_PUBLIC_URL` in the app's settings and let it redeploy. App Platform does a fresh build, but no code change is involved.
- **Pass:** the links are `https://one.example/`, then `https://two.example/`.

### R6. One-command run (L)
- **Do:**
  1. From a clean checkout, run `make docker-build`.
  2. Run `make docker-run`.
  3. Open http://localhost:8080/.
- **Pass:**
  - The build succeeds with one command.
  - The container starts with one command and logs `hosted: listening on [::]:8080, public http://localhost:8080`.
  - The page is the real board UI, not the placeholder page ("The playmat app has not been built into this binary.").

### R7. Push deploys from `main` only (D)
- **Do:** push a commit to a branch other than `main`, then push one to `main`.
- **Pass:** the branch push triggers no deployment, and the `main` push does.

### R8. Card data out of the box (L, then D)
- **Do:** in a freshly started container or a fresh deploy, import a log over `/mcp` or claude.ai. Call `card_text` with `{"card": "<iid from get_state>"}`. Open the board and zoom that card.
- **Pass:** `card_text` returns the card's real text from TCGdex, and the board shows the card's image, loaded from `assets.tcgdex.net`.

### R9. Desktop regression (L)
- **Do:**
  1. Run `make install`, restart Claude Desktop, and import a log through it.
  2. In this repo, use Claude Code's `tcgreplay-dev` server after `make dev` and import a log.
  3. Open the desktop board at http://127.0.0.1:7777/.
- **Pass:**
  - Both clients connect and import as before.
  - Results carry no `board` key.
  - The board loads on loopback and is still refused for a non-loopback Host.
