---
created_date: "2026-11-06"
document_status: final
closed_date: "2026-11-06"
---

# Test plan: 20261106110254-f08e142d-project-website

Manual checks for the project website, for what the automated suites can't cover. The automated suites are `go test ./...` and `make site-test`, which builds the site and runs the Playwright suite in `site/e2e` through the Go site handler. Everything below runs against the **deployed** site at https://tcgreplay.demo.gs, after this branch is merged to `main` and App Platform has redeployed, unless a step says otherwise.

The site's pages, all of which the checks below refer to:

- `/`
- `/privacy`
- `/docs/connect`, `/docs/sign-in`, `/docs/desktop`, `/docs/import`
- `/docs/workflows` and the seven workflow pages: `/docs/workflows/play-game`, `import-game`, `opponent-deck`, `review-game`, `replay-drill`, `before-you-move`, `numbers-drill`
- `/docs/rewind`, `/docs/example`, `/docs/limits`, `/docs/developers`
- the 404 page, e.g. `/no-such-page`

## Success metrics (manual)

### M1. A new player connects and imports the example game in under 10 minutes, unaided

- **What to measure:** time from opening https://tcgreplay.demo.gs to the assistant reporting an import of the example game with no problems. The target is under 10 minutes, unaided, on at least two of three tries with different people.
- **How:**
  1. Recruit three people who have never used tcgreplay. Each has a Google account and uses one of claude.ai, ChatGPT or Claude Code; cover at least two different assistants across the three.
  2. Give each person only the link https://tcgreplay.demo.gs and the task: "Connect this to your assistant and import the example game." Don't help them.
  3. Start a timer when the page opens. Stop it when the assistant reports the import: the game, the result (Rival wins, 6 prizes to 4) and no problems.
  4. Note where each person hesitated, and any page they couldn't follow.
- **Expected result:** at least two of the three finish in under 10:00 with no help. Record each time and assistant.
- **Who / when:** the owner, after the first deploy with the site, before sharing the link widely.

### M2. The home page's largest content shows in under 2.5 s on Lighthouse mobile (Slow 4G)

- **What to measure:** Largest Contentful Paint (LCP) on the home page, with Lighthouse's mobile emulation and its default simulated Slow 4G throttling. The target is under 2.5 s. (The download half of this metric, under 1.5 MB, is covered by `site/e2e/animation.spec.js`.)
- **How:** in Chrome, open https://tcgreplay.demo.gs/ in an Incognito window, then DevTools → Lighthouse → Mode: Navigation, Device: Mobile, Categories: Performance → Analyze page load. Or from a terminal:
  ```sh
  npx lighthouse https://tcgreplay.demo.gs/ --form-factor=mobile --only-categories=performance --output=html --output-path=./lighthouse-home.html
  ```
  Run it three times and take the median LCP.
- **Expected result:** median LCP under 2.5 s, and the report's total byte weight under 1.5 MB (expected about 60 KB).
- **Who / when:** the owner, after each deploy that changes the home page or the scene.

### M3. Every page scores at least 90 for accessibility and performance on Lighthouse mobile

- **What to measure:** the Lighthouse Accessibility and Performance scores, mobile emulation, for every page listed at the top. The target is at least 90 for both on every page.
- **How:** for each page:
  ```sh
  npx lighthouse https://tcgreplay.demo.gs<path> --form-factor=mobile --only-categories=performance,accessibility --output=json --quiet --output-path=./lh<slug>.json
  ```
  Or use DevTools → Lighthouse (Mobile; Performance and Accessibility) page by page. Record both scores per page in a table.
- **Expected result:** every page at least 90 for Accessibility and at least 90 for Performance. Fix and recheck any page below either.
- **Who / when:** the owner, after the first deploy with the site, and again after large content changes.

## Manual reviews

### R1. Each assistant's connecting steps lead to a working connection

- **Where:** https://tcgreplay.demo.gs/docs/connect, the claude.ai, ChatGPT and Claude Code sections, with `/docs/sign-in` as linked from them.
- **What to do:** someone new to tcgreplay follows each section in turn, exactly as written, in that assistant: claude.ai Settings → Connectors → Add custom connector; ChatGPT's custom MCP server settings (the page says **Plugins → Add → Add custom MCP server**); Claude Code `claude mcp add --transport http tcgreplay https://tcgreplay.demo.gs/mcp`, then `/mcp` → Authenticate. Then they ask the "Check it works" prompt: `What can the tcgreplay playmat do?`.
- **What to look for:** every menu name and button on the page matches what the assistant shows today; the Google account chooser appears as `/docs/sign-in` describes; the assistant then lists the playmat's coaching workflows.
- **Passing:** all three end with the assistant listing the workflows. Correct any menu name that differs, especially ChatGPT's, whose path came from the old README.

### R2. The desktop steps end at the board on 127.0.0.1:7777

- **Where:** https://tcgreplay.demo.gs/docs/desktop.
- **What to do:** on a Linux or macOS machine with Go 1.27+, Node.js 22+, git and the Claude desktop app, follow the page exactly:
  1. `git clone`, then `make install`;
  2. find the binary path with `echo "$(go env GOPATH)/bin/tcgreplay"`;
  3. add the `mcpServers` block to `claude_desktop_config.json` (Settings → Developer → Edit Config) and restart the app;
  4. ask `What can the tcgreplay playmat do?`, then start a game, e.g. "Let's play a game. Here's my deck: …".
- **What to look for:** each step works as written, and the Settings → Developer → Edit Config path matches the current app.
- **Passing:** the board opens at http://127.0.0.1:7777 showing the game.

### R3. Importing the site's example game from a real assistant

- **Where:** https://tcgreplay.demo.gs/docs/import and https://tcgreplay.demo.gs/docs/example.
- **What to do:** in a connected assistant, copy the decklist and the battle log with the page's Copy buttons, paste both with the suggested prompt "Here's a game from TCG Live and my decklist. Import it and review it with me.", and open the board link from the reply.
- **What to look for:** the paste arrives whole (no "lines hidden" truncation; if there is, the import page's advice works), and the assistant reports the game with no problems.
- **Passing:** the import reports no problems, NicJackson vs Rival, Rival wins 6–4; the board link opens at `/board?board=…` and shows the game.

### R4. After deploy: content changes go live within 15 minutes, and the board works at /board

- **Where:** the live site and a connected assistant.
- **What to do:**
  1. Merge a small visible content change, e.g. a word on `/docs/developers`, to `main`, note the time, and reload the page until it shows.
  2. Signed out, open https://tcgreplay.demo.gs/board: it should send you through Google sign-in and back to `/board`.
  3. In a connected assistant, run `new_game` or an import, and open the `board` link from its result.
- **What to look for:** the change appears without any manual step. Pages are sent `no-cache`, so a plain reload is enough. `/board` loads the signed-in board, and the new link has the form `https://tcgreplay.demo.gs/board?board=<handle>`.
- **Passing:** the change is live within 15 minutes of the merge, and the board loads and follows the game at `/board`.

### R5. The privacy and limits pages match the playmat on the day of publishing

- **Where:** https://tcgreplay.demo.gs/privacy and https://tcgreplay.demo.gs/docs/limits, against `internal/hostauth` (google.go, store.go, oauth.go, limit.go, pages.go), `internal/usergames/usergames.go`, `internal/game/manager.go` and `.do/app.yaml` on `main`.
- **What to look for:**
  - sign-in keeps only the Google account ID, email and verified flag (scopes `openid`, `email`);
  - the two cookies `tcgreplay_session` (30 days) and `tcgreplay_login_…` (10 minutes);
  - games held in memory only and dropped after the idle time, on restart and when the last sign-in ends;
  - the lifetimes: board sign-in 30 days, connector refresh 30 days, access 1 hour;
  - the logged events, and the outside services;
  - the limits page's idle time, user cap and rate limit equal the values in `.do/app.yaml`, which today are 60 minutes, 10 and 300;
  - the quoted "busy" and "too many requests" messages match the code.
- **Passing:** every statement holds against the code and settings on that day, or the page is corrected before publishing.

### R6. The import page's TCG Live directions match TCG Live's current menus

- **Where:** https://tcgreplay.demo.gs/docs/import, section "Where to copy them in TCG Live".
- **What to do:** in the current Pokémon TCG Live client, find where a finished game's battle log is copied (the page says: match history → the game's log) and where a deck is exported as text (the page says: the deck editor's export).
- **What to look for:** the general directions lead to the right place. Exact menu names are deliberately not given; add them if the client's wording is stable.
- **Passing:** a player following the page finds both, and the copied log starts with `Setup` and the deck ends with `Total Cards: 60`.

### R7. The owner's look-and-feel review of the home page animation

- **Where:** https://tcgreplay.demo.gs/ in a desktop browser (dark and light system themes) and on a phone; the scene is `site/src/components/Playmat.astro`.
- **What to look for:**
  - the loop reads as the real board: the battle log arrives, the board fills, the coach's message appears, the rewind sweep with Turn 7 ticking back to Turn 3, then the second attacker played with its log line;
  - the scene uses the board's felt, seat colours and no-art cards;
  - there is no card artwork and no Pokémon logo;
  - with the OS's reduced-motion setting on, it shows the still last frame and nothing moves;
  - the pace (about 15 s a loop) feels right.
- **Passing:** the owner signs off on the look, or lists changes.

## Operator step (skipped implementation task)

### O1. Enter the site's addresses on Google's OAuth consent screen

The implement run skipped this task because it needs the Google Cloud console and a deployed site.

- **Where:** Google Cloud console → the playmat's OAuth project → Google Auth Platform → Branding (OAuth consent screen).
- **What to do:**
  1. Set **Application home page** to `https://tcgreplay.demo.gs/` and **Application privacy policy link** to `https://tcgreplay.demo.gs/privacy`.
  2. Confirm `demo.gs` is under **Authorized domains**, adding it if it's missing, and save.
- **What to look for:** both addresses open publicly in a signed-out browser, and the console shows no domain warning.
- **Passing:** after saving, sign out at `/board`, open a fresh board link, and complete a Google sign-in; then tick the task in the plan.
