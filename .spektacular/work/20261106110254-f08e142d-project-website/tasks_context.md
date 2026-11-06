### Requirement → repo and files

All work is in the one registered repo, `tcgreplay` (root `/home/nicj/code/github.com/nicholasjackson/tcgreplay`).
- **Home page at a glance; animation; reduced motion:** `site/src/pages/index.astro`, `site/src/components/Playmat.astro`. Tasks: home and privacy pages, animate.
- **Connecting; desktop; sign-in and board:** `site/src/content/docs/connect.md` and its siblings. Task: connecting pages.
- **Importing; example game:** `site/src/content/docs/import.md`, `site/src/pages/docs/example.astro`, `site/src/content/example/battle-log.txt`, `internal/game/site_example_test.go`. Task: example game.
- **Reviewing, rewinding and drills; playing; every workflow; prompts:** `site/src/content/docs/workflows/*.md`, `site/src/content/docs/rewind.md`, `site/src/lib/facts.ts`. Task: workflow pages.
- **Limits:** `site/src/content/docs/limits.md`, `site/src/lib/facts.ts`. Task: limits pages.
- **Privacy:** `site/src/pages/privacy.astro` (or `.md`). Task: home and privacy pages.
- **Fan notice; phones; copy; no tracking:** `site/src/layouts/Base.astro`, `site/src/scripts/copy.ts`. Task: scaffold.
- **Developer pointer:** `site/src/content/docs/developers.md`. Task: limits pages.
- **README:** `README.md`. Task: README.
- **Published automatically:** `Dockerfile`, `.dockerignore`, `internal/site/*`, `cmd/tcgreplay/hosted.go`. Tasks: site package, router, scaffold.

### Task: Embed and serve the website from a Go site package

**File changes**:
- `internal/site/site.go` (new) — the package doc, then:
  - `//go:embed all:dist` `var dist embed.FS`.
  - `assets()` returns `dist/app` if it holds `index.html`, else `dist/placeholder`, mirroring `internal/webui/server.go:106-115`.
  - `Built() bool`.
  - `Handler() http.Handler` serves `assets()`:
    - **Path resolution:** `/` → `index.html`. `/x` → `x/index.html` when it exists (Astro's directory format), `x.html` otherwise, or the file itself.
    - **Unknown paths:** `404.html` with status 404, or plain text if absent.
    - **Methods:** GET and HEAD only, 405 otherwise.
    - **Headers:** `Cache-Control: public, max-age=31536000, immutable` for paths under `/_astro/`, `no-cache` for HTML and everything else.
    - **Never** calls `SetCookie`.
- `internal/site/dist/placeholder/index.html` (new) — modelled on `internal/webui/dist/placeholder/index.html:1-5`: "The website has not been built into this binary. Run `make site`."
- `internal/site/dist/app/.gitkeep` — not needed. Instead add `internal/site/dist/app` to `.gitignore` (beside `web/test-results/`) so built output is never committed, the same way `internal/webui/dist/app` is handled. Check how that is ignored today first (it is listed in `.dockerignore:7`, and `make clean` removes it, `Makefile:62-63`).
- `internal/site/site_test.go` (new) — swap in an `fstest.MapFS` through an unexported `fsys` variable, and test:
  - the placeholder fallback
  - directory and `.html` resolution
  - 404 page and status
  - cache headers for `/_astro/` versus pages
  - no `Set-Cookie` header on any response
  - content types (`.html`, `.css`, `.js`, `.svg`, `.png`, `.txt`)
  - 405 on POST

**Complexity**: Low
**Token estimate**: ~15k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Move the board to /board and serve the site on every other path

**File changes**:
- `cmd/tcgreplay/hosted.go:172-192` — `hostedHandler`:
  - Keep `a.Routes(mux)`, `/mcp`, `/admin` and `/admin/`.
  - Replace `mux.Handle("/", a.RequireSession(reg.Board()))` with:
    - `board := a.RequireSession(reg.Board())`
    - `mux.Handle("GET /board", rewrite("/", board))`, where the rewrite copies the request with `URL.Path = "/"`, keeps `RawQuery` so `?board=` survives, and keeps `RequestURI` meaningful for `RequireSession`'s `next`. See the note below.
    - `mux.Handle("GET /board/", http.RedirectHandler("/board", http.StatusMovedPermanently))`. Preserve the query: write a small handler that redirects to `/board` + `?` + RawQuery.
    - `mux.Handle("/assets/", board)`, `mux.Handle("/ws", board)` and `mux.Handle("/api/", board)`.
    - `mux.Handle("/", site.Handler())`.
  - Update the doc comment at `hosted.go:172-178` to describe the table.
  - **Note on `next`:** `RequireSession` (`internal/hostauth/board.go:58-93`) redirects a signed-out page load to `/auth/login?next=`+`r.URL.RequestURI()` (board.go:69). Apply the rewrite *inside* RequireSession, i.e. `RequireSession(rewrite(reg.Board()))`, so the signed-out redirect carries `next=/board?board=…`. Then the board sees `/`, and its page-load check `internal/usergames/board.go:76-79` still matches.
- `internal/usergames/usergames.go:332-340` — `Link`: `Path: "/board"`. Update the comment at :333 to `<public>/board?board=<handle>`.
- `internal/hostauth/board.go:19-25` — the default `next` becomes `/board`. `board.go:143` — `Link: "/auth/login?next=/board"`.
- `internal/usergames/board.go:124` — `<a href="/board">Open your own board</a>`.
- `internal/cacheadmin/page.go:47` and `internal/hostauth/admin.go:47` — the "Back to the board" link becomes `/board`.
- `web/src/lib/session.js:31` — no change: it sends `location.pathname + location.search`, which is `/board?...` on the hosted board.
- Tests to update to the new address (not loosen):
  - `cmd/tcgreplay/hosted_users_test.go:267`
  - `cmd/tcgreplay/hosted_test.go:746-747`
  - `internal/usergames/mcp_test.go:33,89`
  - `internal/usergames/usergames_test.go:157`
  - `internal/hostauth/board_test.go:421` (`next=/board`)
  - `internal/usergames/board_test.go:278,299` test the per-account board server directly, so they stay at `/`.
  - `internal/mcpserver/server_test.go:614` uses fixed fake links, so it is unchanged.
- `cmd/tcgreplay/hosted_test.go` — new `TestHostedRoutes`, using the existing hosted harness:
  - Signed out, `GET /`, `/docs/connect` and `/privacy` return 200 from the site (the placeholder is fine in tests) with no `Location` and no `Set-Cookie`.
  - Signed out, `GET /board?board=x` redirects to `/auth/login?next=%2Fboard%3Fboard%3Dx`.
  - Signed in, `/board` serves the board's index.
  - `/board/?board=x` gives 301 to `/board?board=x`.
  - Signed out, `/assets/x.js`, `/ws` and `/api/card` are refused as today.
  - `/healthz`, `/mcp` (401 with `WWW-Authenticate`) and `/admin` are unchanged.
  - An unknown path gives 404 from the site.

**Complexity**: Medium
**Token estimate**: ~35k tokens
**Agent strategy**: 2 parallel agents: (1) the router and its new test in `cmd/tcgreplay`; (2) the link and return-path edits plus updating the pinned tests in `internal/usergames`, `internal/hostauth` and `internal/cacheadmin`. Integrate sequentially and run `go test ./...`.

### Task: Scaffold the website build, layout and test harness

**File changes**:
- `site/package.json` (new) — private, `type: module`.
  - Scripts: `dev` (astro dev), `build` (astro build), `test:e2e` (`node ../web/scripts/check-browsers.js && playwright test`, or a local copy of that script).
  - devDependencies: `astro` (pin the current 7.x exactly), `yaml`, `@playwright/test` at `1.64.1` to match `web/package.json`.
  - `site/package-lock.json` is committed.
- `site/astro.config.mjs` (new) — `site: 'https://tcgreplay.demo.gs'`, `outDir: '../internal/site/dist/app'`, `build.format: 'directory'`, `trailingSlash: 'never'`, no integrations.
- `site/src/lib/facts.ts` (new) — reads `../.do/app.yaml` with `yaml`, finds the `playmat` service's envs `TCGREPLAY_RATE_LIMIT`, `TCGREPLAY_MAX_USERS` and `TCGREPLAY_IDLE_TIMEOUT` (`.do/app.yaml:58-74`), and throws with a clear message if any is missing or unparsable.
  - Exports `limits = { requestsPerMinute, maxUsers, idleTimeout }`, rendering a Go duration the way `humanDuration` does (`internal/usergames/usergames.go:203-220`), e.g. "60 minutes".
  - Also exports `site = { publicURL, mcpURL, boardURL: publicURL + '/board', repoURL }`.
- `site/src/layouts/Base.astro` (new):
  - `<html lang="en">`, title, meta description and a favicon (from `docs/icon.png`, copied to `site/public/icon.png`).
  - Header with the logo (`docs/logo.png` → `site/public/logo.png`, resized if over ~60 KB) and the nav: Home, Get started, Guides, Example, Limits, Privacy.
  - `<main>`, and a footer carrying:
    - "tcgreplay is an unofficial fan project. It is not affiliated with, endorsed, sponsored or approved by Nintendo, The Pokémon Company, Creatures or Game Freak. Pokémon and its trademarks are theirs."
    - "Card data and images from TCGdex" (link https://tcgdex.dev)
    - a link to the source repository
  - Styles use the board's tokens: `@import` of `../../web/src/app.css`'s `:root` variables only. Prefer copying the token block into `site/src/styles/tokens.css` with a comment naming the source, because importing app.css brings the board's global element rules. System font stack, no web fonts.
  - Layout rules: `pre`, `code` and prompt blocks get `overflow-x: auto` and `white-space: pre-wrap` where prose, and `max-width: 100%`. The body has no fixed widths.
- `site/src/scripts/copy.ts` (new) — on load, for every `[data-copy]` element (or `pre` inside `.copyable`), insert a `<button type="button" class="copy">Copy</button>`. On click it calls `navigator.clipboard.writeText(el.dataset.copy ?? el.textContent.trimEnd())`, shows "Copied" for 2 s, and announces through an `aria-live` region. Imported once by Base.astro.
- `site/src/components/Prompt.astro` and `site/src/components/CopyBlock.astro` (new) — wrappers that render a copyable block.
- `site/src/pages/404.astro` (new) — "Page not found" with a link home.
- `site/src/pages/index.astro` (new, placeholder) — a minimal home with the layout (filled by the home-page task).
- `Makefile:4` — add `site site-dev site-test` to `.PHONY`. New targets:
  - `site: site/node_modules` → `cd site && npm run build`
  - `site/node_modules: site/package.json site/package-lock.json` → `cd site && npm ci && touch site/node_modules`
  - `site-dev` → `cd site && npm run dev`
  - `site-test: site` → `cd site && npm run test:e2e`
  - `test` (`Makefile:24-29`) also runs `$(MAKE) site-test`
  - `clean` also removes `internal/site/dist/app`
- `Dockerfile:15-21` — a new stage after `web`:
  - `FROM node:22 AS site`, `WORKDIR /src/site`, `COPY site/package.json site/package-lock.json ./`, `RUN npm ci`
  - `COPY site/ ./`, `COPY internal/guides /src/internal/guides`, `COPY .do/app.yaml /src/.do/app.yaml`, `COPY testdata/decks /src/testdata/decks`, `COPY web/src/app.css /src/web/src/app.css` (if the tokens are read from there), `COPY docs /src/docs` (if assets are read from there)
  - `RUN npm run build`
  - In the build stage, after line 29: `COPY --from=site /src/internal/site/dist/app internal/site/dist/app`
  - Update the header comment, lines 1-13.
- `.dockerignore` — add `site/node_modules`, `internal/site/dist/app`, `site/test-results`. Never `internal/guides` (knowledge conventions/skills-text-and-packaging.md).
- `.gitignore` — add `site/node_modules/`, `site/test-results/`, `site/playwright-report/`, `internal/site/dist/app/`.
- `internal/site/sitetest/cmd/serve/main.go` (new) — serves `site.Handler()` on `-addr` (default 127.0.0.1:7791) for the browser suite, like `internal/webui/webtest/cmd/boardtest`. It also answers `/board`, `/mcp`, `/ws` and `/api/*` with 503 so blocked-playmat tests are realistic.
- `site/playwright.config.js` (new) — modelled on `web/playwright.config.js:1-50`:
  - `webServer: go run ./internal/site/sitetest/cmd/serve -addr 127.0.0.1:7791` (cwd `..`)
  - Projects: `laptop` (Chromium, 1366×768) and `phone` (Chromium with touch emulation, 375×812). Chromium only (knowledge gotchas/playwright-touch-tests.md).
- `site/e2e/site.spec.js` (new) — `crawl()` collects every same-origin link from `/` (breadth-first, deduplicated). Then, for every page:
  - the footer notice contains Nintendo, The Pokémon Company, Creatures and Game Freak, and the TCGdex credit
  - at 375 px, `document.scrollingElement.scrollWidth <= innerWidth` and `main`'s scrollWidth fits
  - in a fresh context, `context.cookies()` is empty and every `request.url()` host equals the site's host
  - every copy button (with clipboard permissions granted) writes exactly its block's text, read back with `navigator.clipboard.readText()`

**Complexity**: High
**Token estimate**: ~60k tokens
**Agent strategy**: Parallel analysis, sequential integration:
1. Analyse the web toolchain and Dockerfile in parallel with drafting the Astro layout.
2. Then integrate in order: package and config → layout and scripts → Make and Docker → test server → browser suite.
3. Run `make site`, `go test ./internal/site/...` and `make site-test`, and finish with a `docker build`.

### Task: Write the home page and the privacy page

**File changes**:
- `site/src/pages/index.astro` — the hero:
  - an `<h1>` and the one-line pitch, e.g. "Paste a Pokémon TCG Live battle log, and your AI assistant rebuilds the game on a live board, coaches you through it, and rewinds to try better lines."
  - the `Playmat` component
  - a primary "Get started" link to `/docs/connect`
  - below the fold, three short "what it does" points drawn from the README's "What it does" (`README.md:19-33`)

  The hero is sized so all three hero items fit in 1366×768 minus the header.
- `site/src/components/Playmat.astro` (new) — the static scene markup, structured for the animation task to drive later:
  - the felt (`--felt`) with the two seats' colours (`--p1`, `--p2`)
  - an Active and Bench row per side built from no-art cards. Copy the visual rules for `.text-card` (name, HP, stage), `.back-art` and the energy `.token` from `web/src/components/Card.svelte:73-88` and its styles at `:96` onward, into the component's scoped style.
  - a battle-log panel and a coach bubble

  Use made-up card names that are clearly stand-ins, or generic labels ("Basic", "Stage 1", "Energy"), never official art. Add `role="img"` with an `aria-label` describing the scene, plus a visible caption.
- `site/src/pages/privacy.md` or `privacy.astro` (new), at `/privacy`. Facts, from research.md "Privacy facts as implemented":
  - **At sign-in:** Google sign-in asks only for `openid` and `email`. The playmat keeps the Google account id, the email address and whether Google verified it (`internal/hostauth/google.go:189`; `hostauth.go:134`). It never asks for or keeps a name, photo or password. Google's own token is discarded after sign-in.
  - **Session lifetimes:** sign-ins are kept in memory, encrypted. The board session lasts 30 days, connector refresh tokens 30 days (renewed on use), access tokens 1 hour (`internal/hostauth/store.go:13-17`, `oauth.go:25-27`).
  - **Two cookies:** `tcgreplay_session` (30 days) and `tcgreplay_login_…` (10 minutes, during sign-in). The board also stores four display preferences in the browser.
  - **While playing:** games, game records, coaching notes and the assumed opponent list are held in memory only, never on disk or in storage (`internal/game/manager.go:21-23`). They are dropped after `limits.idleTimeout` without activity, when your last sign-in ends, and on every update or restart.
  - **Logs:** the email appears on sign-in, sign-out and limit events. No IP addresses, battle logs or decks are logged. The hosting providers (DigitalOcean, Cloudflare) may keep their own request logs.
  - **Outside services:** Google (sign-in), TCGdex (card data, no personal data), card images loaded by your browser from the playmat's image CDN or TCGdex.
  - **Never:** no passwords, no name or photo, no Google access tokens, no analytics or advertising, no selling or sharing.
  - **Removing your data:** sign out, and disconnect the connector in your assistant. There is no stored account to delete.
  - **Contact:** the GitHub issues link.
  - **"Last updated" date.**
- `site/e2e/home.spec.js` (new) — at 1366×768, `h1`, the pitch, `[data-playmat]` and the "Get started" link all have a bounding box within the viewport. The link's `href` is `/docs/connect`. No `img` on the page has a `src` containing `tcgdex` or `pokemon`.
- `site/e2e/privacy.spec.js` (new) — `/privacy` returns 200, contains the sections (sign-in, while playing, how long, outside services, never collected), and its idle time equals the value derived from `.do/app.yaml` (read in the test with `yaml`).

**Complexity**: Medium
**Token estimate**: ~30k tokens
**Agent strategy**: 2 parallel agents: (1) the home page and Playmat component; (2) the privacy page from research.md's facts, re-checked against the cited code. Then add the browser specs sequentially.

### Task: Write the connecting, desktop and sign-in pages

**File changes**:
- `site/src/content/docs/connect.md` (new, `/docs/connect`). It opens with the address (CopyBlock, `site.mcpURL`), then:
  - **claude.ai:** Settings → Connectors → Add custom connector, name "tcgreplay", URL, then the Google sign-in, from `README.md:44-45`.
  - **ChatGPT:** Plugins → Add → Add custom MCP server, URL, authentication OAuth (`README.md:47-49`). Drop the README's "rough edges" caveat: the owner reports ChatGPT works well.
  - **Claude Code:** `claude mcp add --transport http tcgreplay https://tcgreplay.demo.gs/mcp` as a CopyBlock, then `/mcp` to sign in.
  - Each section ends with "Check it works": ask "What can the tcgreplay playmat do?". The assistant should list the playmat's workflows (from the server instructions, `internal/mcpserver/guides.go:55-64`), and if it can't reach the playmat, sign in again from the connector settings.
- `site/src/content/docs/sign-in.md` (new, `/docs/sign-in`), linked from each connect section:
  - The first tool use opens Google's account chooser directly, with no tcgreplay page in between (`internal/hostauth/oauth.go:258-298`). It asks only for your email.
  - After sign-in you return to the assistant.
  - Replies to import, new game and opponent deck carry a link to `https://tcgreplay.demo.gs/board?board=…`. Open it in the same browser and you're already signed in; in another browser, sign in with the same Google account (`internal/hostauth/google.go:210-214`).
  - "Not your game" means you're signed in as a different account.
  - If the playmat is busy, try later or use the desktop.
- `site/src/content/docs/desktop.md` (new, `/docs/desktop`). From `README.md:54-83` and research.md "Desktop":
  - **Prerequisites:** Go 1.27 or newer and Node 22; tested on Linux and macOS, Windows untested.
  - **Install:** `git clone`, then `make install`.
  - **Setting:** the `claude_desktop_config.json` block with the `go install` path (`$(go env GOPATH)/bin/tcgreplay`). Restart the Claude desktop app.
  - **Board:** http://127.0.0.1:7777. No sign-in, one game at a time.
  - Point to the repository for details.
- `site/src/pages/docs/[...slug].astro` and the content collection config `site/src/content.config.ts` (new, if not created by the workflow task) — render the docs Markdown with Base.astro and a docs sidebar.
- `site/e2e/connect.spec.js` (new) — the home "Get started" link lands on `/docs/connect`. The page has the headings claude.ai, ChatGPT and Claude Code, each followed by a "Check it works" step before the next heading. The address copy button copies exactly `https://tcgreplay.demo.gs/mcp`. `/docs/sign-in` is linked from `/docs/connect` and mentions Google and `/board`. `/docs/desktop` mentions `127.0.0.1:7777`.

**Complexity**: Medium
**Token estimate**: ~25k tokens
**Agent strategy**: Single agent, sequential execution. Writing carries more weight than code here; re-check each claim against the cited code.

### Task: Write a page for every coaching workflow, read from the playmat's guides

**File changes**:
- `site/src/content.config.ts` — collections:
  - `guides`: a `glob` loader with `base: '../internal/guides'` and pattern `*.md`, schema `{ name, description }` from the frontmatter.
  - `docs`: `site/src/content/docs/**/*.md`.
  - `workflows`: `site/src/content/docs/workflows/*.md`, schema `{ guide: string, title: string, prompts: string[] (min 1) }`.
- `site/src/lib/workflows.ts` (new):
  - `GUIDE_ORDER` is read from `internal/guides/guides.go:17` by a regex on `var order = []string{...}`, so the order isn't retyped. Throw if it isn't found.
  - `pairWorkflows()` throws a build error naming the gap if any guide has no workflow page, a page's `guide` is not a guide name, or names are duplicated. It returns the pages in guide order with each guide's `description`.
- `site/src/content/docs/workflows/{play-game,import-game,opponent-deck,review-game,replay-drill,before-you-move,numbers-drill}.md` (new). Each has a plain-language explanation and 2-4 prompts in frontmatter. Seed the prompts from `README.md:87-133`, reworded so none names a specific card the example game doesn't contain unless it is just an illustration.
  - `import-game` links to `/docs/import`.
  - `review-game` explains how the coach asks before judging and rates decisions apart from outcomes.
  - `play-game` explains that the assistant doesn't peek at your hidden cards.
- `site/src/content/docs/rewind.md` (new, `/docs/rewind`) — rewind to "your third turn", try a line on a branch, compare boards, sample the opponent's hidden hand several times. Prompts from `README.md:98-101`. Name turns the way the playmat does: "Turn N is each player's Nth turn".
- `site/src/pages/docs/workflows/index.astro` (new, `/docs/workflows`) — lists every workflow (title + guide description + link) in guide order, from `pairWorkflows()`.
- `site/src/pages/docs/workflows/[guide].astro` (new) — renders a workflow page with its prompts as Prompt components (copyable).
- `site/e2e/workflows.spec.js` (new) — reads `internal/guides/*.md` frontmatter names in the test. For each, `/docs/workflows/<name>` returns 200 with at least one `.prompt` with a copy button. `/docs/workflows` lists them in the order from `guides.go`. `/docs/rewind` exists with a prompt.

**Complexity**: Medium
**Token estimate**: ~35k tokens
**Agent strategy**: 2 parallel agents: (1) the collections, loader, pairing check and page templates; (2) the eight Markdown pages. Then integrate, add the spec, and prove the build fails by temporarily renaming one page.

### Task: Publish the anonymised example game and the import page

**File changes**:
- `site/src/content/example/battle-log.txt` (new) — generated once from `testdata/logs/2026-10-11_gatorusoju_mewex-bronzong-lopunny.txt` by replacing every `GatoruSoju` with `Rival` (98 lines affected, 266 lines total). Byte-identical otherwise, including curly apostrophes (testdata/README.md quirk 3).
- `internal/game/site_example_test.go` (new) — `TestSiteExampleGame`:
  - Read the original and `../../site/src/content/example/battle-log.txt`. Assert `strings.ReplaceAll(orig, "GatoruSoju", "Rival") == example` and that `!strings.Contains(example, "GatoruSoju")`.
  - Import the example with `testdata/decks/mega-lucario-ex-v12.txt` via `NewManager(t.TempDir(), nil).Import(ctx, ImportRequest{Log, Deck, Seed: 1, ID: "g-site-example"})`, as `internal/game/import_test.go:24-31` does.
  - Assert zero `Report.Problems` and no `State().Check()` warnings.
  - Assert the winner and prizes match `testdata/index.json`'s entry for that file (`expected.winner` with GatoruSoju → Rival, and `expected.prizes_taken`).
  - Skip the test if the site file is absent, matching the corpus test's skip (`import_test.go:20-22`).
- `site/src/pages/docs/example.astro` (new, `/docs/example`) — a short intro: the owner is NicJackson playing Mega Lucario ex against a Mew ex / Bronzong / Mega Lopunny ex deck, a close loss. Then:
  - a suggested prompt (Prompt): "Here's a game from TCG Live and my decklist. Import it and review it with me."
  - CopyBlock of the decklist, read at build time from `../testdata/decks/mega-lucario-ex-v12.txt`
  - CopyBlock of the battle log, imported with `?raw`, in a scrollable box with max-height
- `site/src/content/docs/import.md` (new, `/docs/import`):
  - **Where to copy:** in TCG Live, the battle log is copied from the match history's game log, and the deck from the deck editor's export. Word it from what the README says, keep it general, and flag in the test plan that the menu names must be checked against TCG Live.
  - **What to paste:** the log and the decklist together.
  - **A complete import:** the assistant reports the game, the turns and no problems.
  - **Missing lines:** some chat apps collapse a long paste (e.g. "141 lines hidden"). Paste the whole log again, or attach it as a file.
  - **Keeping the game:** ask for its record and keep it. Importing the record later restores the game (`internal/mcpserver/server.go:483,507`).
  - Link to `/docs/example`.
- `site/e2e/example.spec.js` (new):
  - The log copy button's clipboard text equals the file's contents, trimmed of a trailing newline, and contains "Rival" and not "GatoruSoju".
  - Its first line is "Setup", and its last line is the result line "Opponent took all of their Prize cards. Rival wins.".
  - The decklist copy equals `testdata/decks/mega-lucario-ex-v12.txt`.
  - `/docs/import` mentions a complete import and missing lines.

**Complexity**: Medium
**Token estimate**: ~25k tokens
**Agent strategy**: Single agent, sequential execution: generate the file, Go test first (run it), then pages and spec.

### Task: Write the hosted limits and developer pages

**File changes**:
- `site/src/content/docs/limits.md` (or `.astro` so it can read `limits` from `site/src/lib/facts.ts`), at `/docs/limits`. It states:
  - each Google account has one game at a time, held in memory
  - the game is dropped after `limits.idleTimeout` without activity, and on every update or restart of the playmat
  - **keep the record:** ask the assistant for the game's record (`get_record`) and keep it in a note. Importing it later restores the identical game (branches and notes aren't kept).
  - it serves `limits.maxUsers` signed-in players at once, and `limits.requestsPerMinute` requests a minute per account
  - the messages you may see, quoted from the code (research.md "Limits as users meet them"): "tcgreplay is busy…" and "too many requests…"
  - when it's full, run it on your desktop (link `/docs/desktop`)
- `site/src/content/docs/developers.md` (new, `/docs/developers`) — two or three sentences: the source, design notes and developer instructions are in the repository (link `site.repoURL`). Issues and contributions there. No commands.
- `site/e2e/limits.spec.js` (new) — reads `.do/app.yaml` in the test. `/docs/limits` contains the rate number, the user count and the idle time rendered as `humanDuration` does ("60 minutes"). It mentions the record and links to `/docs/desktop`. `/docs/developers` links to `https://github.com/nicholasjackson/tcgreplay` and contains no `make ` or `go test` text.

**Complexity**: Low
**Token estimate**: ~15k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Point the README at the site and record the new routes

**File changes**:
- `README.md:1-17` — keep the logo, title and a shortened intro (3-4 sentences).
  - Replace `README.md:19-133` ("What it does", "Using it", "Example prompts") with a short "Get started" paragraph: "See https://tcgreplay.demo.gs for what it does, and https://tcgreplay.demo.gs/docs/connect to connect it to Claude or ChatGPT." Add one line for the desktop docs link.
  - Keep "Development" (`README.md:135-152`). Add `make site`, `make site-dev` and `make site-test`, and a sentence that the website lives in `site/` and is served by the hosted playmat.
- `TCG Review Playmat Design and Build Plan.md` — in the hosted section (near the admin page paragraph around line 304, and the settings and operator steps at 326-338), add a paragraph covering:
  - **Routes:** the hosted playmat serves the website at `/` (public, static, embedded from `internal/site`), the board at `/board` (signed in; its assets, `/ws` and `/api/` stay at the root), and board links of the form `/board?board=<handle>`.
  - **Build:** the site is built from `site/` by the Dockerfile's `site` stage, and reads guides, limits and the example deck at build time.
  - **Redeploys:** each push to main redeploys and drops games.
  - **Operator step:** set the consent screen's home page and privacy policy to `/` and `/privacy`.
- `.do/app.yaml:1-18` — header comment: the playmat also serves the website at `/` and the board at `/board`. The limits below are also shown on the website, which reads them from this file at build time.
- `Makefile:52-56` comment above `docker-build` — mention that the image includes the website.

**Complexity**: Low
**Token estimate**: ~12k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Animate the home page's playmat scene

**File changes**:
- `site/src/components/Playmat.astro` — add `data-beat` states and a `<script>` (bundled, local, no dependencies):
  - If `matchMedia('(prefers-reduced-motion: reduce)').matches`, return immediately and leave the still (final) frame.
  - Otherwise step `data-beat` through `log` → `board` → `coach` → `rewind` → `line` on a timer (about 2.5-3.5 s each, roughly a 15 s loop), then reset and loop. Pause when the tab is hidden (`visibilitychange`) and when the scene is off-screen (IntersectionObserver).
  - CSS transitions (transform and opacity only) do the motion:
    - log lines typing into the log panel
    - cards sliding from the deck to the Active and Bench spots
    - the coach bubble fading in ("Turn 3: what if you bench a second attacker first?", a made-up stand-in)
    - a rewind sweep: the board dims, a turn counter ticks back, and cards return
    - a different card moving to a new spot with an accent outline (`--accent`)
  - Each beat sets `data-beat-seen` on the scene root, for the test.
  - The caption has `aria-live="off"`, and the whole scene stays `role="img"` with a stable label.
- Reduced-motion CSS: `@media (prefers-reduced-motion: reduce) { [data-playmat] * { animation: none !important; transition: none !important; } }`. The default (no-JS) state is the still final frame, so the still shows without JS too.
- `site/e2e/animation.spec.js` (new):
  - With `page.route` aborting `/board*`, `/mcp*`, `/ws*` and `/api/*`, record the scene's `data-beat` changes via a MutationObserver exposed to the test. Assert the order `log, board, coach, rewind, line`, and that `log` appears again (the loop) within about 20 s.
  - With `emulateMedia({ reducedMotion: 'reduce' })`, assert `data-beat` doesn't change over 4 s and that `document.getAnimations().length === 0`.
  - Sum `response.body()` sizes for every request on `/` and assert the total is under 1.5 MB.
  - Assert that no `img` `src` or CSS `url()` in the scene points at tcgdex or any other host.

**Complexity**: Medium
**Token estimate**: ~30k tokens
**Agent strategy**: Single agent, sequential execution: script and CSS first, check in a browser, then the spec. Keep the bundle tiny; no libraries.

### Task: Enter the site's addresses on Google's OAuth consent screen

**What the person does**:
1. After the site has deployed, in the Google Cloud console for the playmat's OAuth client open **Google Auth Platform → Branding** (the OAuth consent screen).
2. Set **Application home page** to `https://tcgreplay.demo.gs/` and **Application privacy policy link** to `https://tcgreplay.demo.gs/privacy`. Confirm `demo.gs` is under **Authorized domains**, adding it if missing. It already hosts the sign-in redirect at `https://tcgreplay.demo.gs/auth/google/callback`.
3. Save.
4. If Google offers brand verification, it may be submitted, but it isn't required for the `openid` and `email` scopes.

**How it is checked**:
- Both addresses open publicly in a signed-out browser.
- The console shows them saved with no domain warning.
- A fresh Google sign-in to the playmat (sign out at `/board`, open a board link) still completes.
