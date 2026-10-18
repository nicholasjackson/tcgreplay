# Working context: hosted-playmat epic / per-user-games spec

## Problem and motivation
- The player's workflow after a game: "I pass the log to claude, claude then estimates the opponents deck creates a data file with my deck the opponents deck and the moves. It then saves that in Notion. Then generally claude gives me some analysis on the game."
- "I do not always have access to the replay viewer" — today tcgreplay is desktop-only by design (design plan: "No remote hosting, auth or HTTPS"). Claude reaches it only over stdio MCP (`cmd/tcgreplay/main.go:117`, `mcp.StdioTransport`) and the UI is on 127.0.0.1. Away from the desktop both the viewer and the board tools are lost; losing the tools hurts analysis most.
- Goal: "examining the prospect of an online version".

## Decisions (epic-wide)
- Host tcgreplay itself, reachable from claude.ai (web/mobile) as a custom connector.
- Runs in a Docker container; user wants it **stateless**: "This would have to run in a docker container so would have no data storage. I would like it to be stateless". Interpreted and accepted as: nothing survives a restart; session state lives in memory.
- **Memory store**, not a temp dir: "I think a memory store would be better". `store.NewMemory` already exists (`internal/store/store.go:111`).
- Game record = transcript + my deck + opponent deck + seed (already `<id>.import.json` today). Deterministic rebuild from it is what makes stateless viable.
- **Notion optional**: "Notion is something I would use. it is not mandatory for others" — server returns the record via a tool; Claude saves it wherever the user keeps things.
- **Google auth**, only to protect against abuse: "Login would just protect against abuse. A simple google auth."
- Other people may use it → one game per signed-in account (later spec).
- **Link to the board**: "I would want to be able to get a link to open the board". Tools that open/create a game return a viewer URL.
- **Dropped**: in-chat board via MCP Apps — "Lets drop the view the board from claude, it is fine to alt tab". Also dropped API-backed chat in the viewer.
- **Dropped for now**: multiple instances / LB routing — "Don't worry about several instances for now. We can do that later if needed."

## Epic specs (agreed order)
1. web-mcp-docker — MCP over HTTP + Dockerfile; tools return board link; one user, one instance, no login. Proves claude.ai can drive the board.
2. memory-store-record — memory-only store, record-returning tool, seed accepted on import_log.
3. google-login — Google sign-in on viewer and on claude.ai's connector (claude.ai expects the MCP server to be an OAuth authorization server; likely a small OAuth proxy delegating to Google; fallback secret URL for /mcp).
4. per-user-games — each Google account gets its own game, name and seat.

## Known gaps noted
- set_opponent_deck with new_id writes the list to the new game's import.json but the original game's record is not updated; older games (carlos0oz, carlos0oz-art) have no import.json.
- import_log MCP tool does not expose Seed though ImportRequest has it.

## Repo facts
- Spektacular initialised this session; spek-* skills live in skills/ (via .claude/skills symlink) but only tcg-* are embedded in the binary (`skills/skills.go`).

## Interview answers (web-mcp-docker)
- Hosted mode = new mode of the same binary; desktop stdio/daemon unchanged.
- Trial access: "Fully open, short-lived" — no protection in this spec.
- Host: DigitalOcean App Platform (corrected from Droplet): "Not a digital ocean droplet but an application, you can deploy these automatically from a github push". Added push-to-deploy requirement + AC.
- TLS: "I have a domain in cloudflare I will setup forwarding" — container serves plain HTTP; no TLS/proxy shipped.
- Skills packaging for claude.ai: "No, later spec".
- Hosted board is interactive (user can drag cards from browser), not view-only — confirmed with requirements.
- Deploy branch: only `main` deploys on DO ("only main for DO").
- Review fixes applied: HTTPS story unified in Constraints (plain HTTP on platform port; App Platform + Cloudflare in front); AC added that automated tool tests pass over hosted connection; behaviour-level wording for container/push requirements.
- Spec web-mcp-docker written to store (20261017105035-e3f64c1b-web-mcp-docker).

## Process
- User asked to "finish all these specs" (memory-store-record, google-login, per-user-games). Sections are presented as one whole draft per spec for review rather than one at a time.
- web-mcp-docker is done (20261017105035-e3f64c1b-web-mcp-docker).
- memory-store-record done: record includes player name; damaged pastes restore correctly or are rejected; session = one run of the playmat.

## google-login interview answers (asked before spec new)
- Who may sign in: "Any Google account" (login only stops anonymous traffic).
- Connector auth: "Standard OAuth sign-in" — adding the connector in claude.ai opens Google sign-in.
- Desktop: "No login on desktop".
- google-login done; review fixes applied.

## per-user-games interview answers (asked before spec new)
- Same account from two conversations/devices: "One game per account".
- Idle games: "Yes, after an idle time" set by operator; user re-imports from record.
- Board links: "No, owner only" — others get "not your game".
- per-user-games done: activity = action via Claude, board move, or opening the board; an idle open tab does not keep a game.

## Plan-epic run (2026-10-17)
- Epic: 20261017105033-91f90842-hosted-playmat. Project root: /home/nicj/code/github.com/nicholasjackson/tcgreplay.
- Order: web-mcp-docker -> (memory-store-record, google-login in parallel) -> per-user-games.
- Started child: web-mcp-docker (start).
- User away swimming: "save up any questions until you are done unless they are real show stoppers". Children told to take recorded defaults; only a question that blocks the whole chain goes to the user.
- web-mcp-docker DONE; summary kept in .spektacular/work/plan-epic/web-mcp-docker.done.md.
- Started children: memory-store-record (start), google-login (start).
- memory-store-record DONE; summary in .spektacular/work/plan-epic/memory-store-record.done.md. Cross-plan note: it removes web-mcp-docker's Resume/load_game link (settle in Step 6).
- Started child: per-user-games (start). google-login shows done in status; its DONE hand-back still pending.
- google-login DONE; summary in .spektacular/work/plan-epic/google-login.done.md. Cross-plan note: health check / -> /healthz vs web-mcp-docker.
- per-user-games DONE; summary in .spektacular/work/plan-epic/per-user-games.done.md. All 4 planned. Now Step 6 (cross-plan decisions) — held for user's return.
- Summary sections staged in .spektacular/tmp/plan-epic/<short>.md (not yet written; waiting on decisions).
- Cross-plan decisions to put to user on return (proposal: later plan's rule stands, earlier plans unchanged since they land first):
  D1 saved games/Resume/load_game: web-mcp-docker uses them; memory-store-record removes them (desktop too, per spec).
  D2 board link: root URL (web-mcp-docker) -> /?board=<handle> (per-user-games).
  D3 health check: / (web-mcp-docker) -> /healthz (google-login).
  D4 user cap: signed-in accounts (google-login) -> places freed by idle expiry (per-user-games).
- User accepted D1-D4 as proposed (later plan's rule stands, no plan edits).
- Summary written (4 sections + decisions). epic order added: google-login depends on memory-store-record (shared webui/cmd/web files). Next: Step 7 review.
- Review: user confirmed 'Looks right'. Saving all 5 discoveries to knowledge via spek-knowledge.
- Knowledge: 5 entries written to repo/tcgreplay. Plan-epic run complete.

## Implement-epic run (2026-10-17)
- Epic: 20261017105033-91f90842-hosted-playmat. Project root: /home/nicj/code/github.com/nicholasjackson/tcgreplay.
- Order is a strict chain: web-mcp-docker -> memory-store-record -> google-login -> per-user-games.
- Dirty repo tcgreplay: only untracked AGENTS.md, CLAUDE.md, skills/spek-* (no plan code). Not asked to commit (user's global rule: never offer to commit); proceeding, they will not be in worktrees.
- Worktree created and child started: web-mcp-docker (start).
- web-mcp-docker QUESTION (human task "Create the App Platform app and its domain"): user chose (a) defer until after merge; task + spec-validation AC stay unchecked. Owner steps after merge: set real domain in .do/app.yaml, doctl apps create --spec .do/app.yaml, Cloudflare CNAME (DNS-only then proxied Full strict), verify / and /mcp initialize. Answer sent to child.
- web-mcp-docker DONE and merged; summary in .spektacular/work/implement-epic/web-mcp-docker.done.md.
- Run blocked: CLI keeps web-mcp-docker 'ready' while human task unticked. User on another laptop asked me to push; pushed main (77e6a6a..d298169) to origin. User setting up DO + Cloudflare; on their word, resume a child to tick the human task, then start memory-store-record.
- App live at https://tcgreplay.demo.gs (user set up in DO console, not from app.yaml; pushes to main redeploy). Verified / 200 and /mcp initialize. Committed+pushed .do/app.yaml domain -> tcgreplay.demo.gs (6ad437f) at user's request. Ticked human task in plan (spec-validation AC left unchecked). web-mcp-docker now implement=done.
- Worktree created and child started: memory-store-record (start). Children told never to push (main auto-deploys).
- memory-store-record DONE and merged; summary in .spektacular/work/implement-epic/memory-store-record.done.md. Not pushed.
- Worktree created and child started: google-login (start).
- User approved push: main pushed with memory-store-record.
- User (durable for this run): 'If you need to push any changes just go do it, no need to ask' — push main after each merge without asking.
- google-login QUESTION (human task "Create the Google sign-in client and deployment secrets"). Done via browser: new GCP project `tcgreplay` (jumppad's consent screen is Heimdall's, left untouched); consent screen External, In production; scopes openid+userinfo.email; home https://tcgreplay.demo.gs; privacy link https://github.com/nicholasjackson/tcgreplay (authorised domains demo.gs, github.com); Web client `tcgreplay` with redirects https://tcgreplay.demo.gs/auth/google/callback and http://localhost:8080/auth/google/callback.
- Cloudflare demo.gs (free plan): Bot Fight Mode already off, no custom/WAF rules -> no change needed.
- Waiting on user: set TCGREPLAY_GOOGLE_CLIENT_ID / _SECRET in App Platform console (I may not enter secrets). Then: tell child to tick task, wrap up, commit; merge; push; verify; user switches health check to /healthz.
- User confirmed Google env vars set in App Platform. Told google-login child to tick human task, wrap up, commit, hand back DONE.
- google-login DONE, merged and pushed; summary in .spektacular/work/implement-epic/google-login.done.md.
- Worktree created and child started: per-user-games (start).
- Live issue: TCGREPLAY_PUBLIC_URL was unset on console-created app -> issuer/redirect localhost:8080. Fixed by user in console; verified healthz ok, /mcp 401 w/ correct resource_metadata, register 201, redirect_uri https://tcgreplay.demo.gs/auth/google/callback. Pending: user browser sign-in, claude.ai connector, health check -> /healthz.
- Hotfix written on main (uncommitted): per-state login cookie + 401 for signed-out subresources; make test passes. Commit+push was blocked by the permission classifier (production deploy) — waiting on user.
