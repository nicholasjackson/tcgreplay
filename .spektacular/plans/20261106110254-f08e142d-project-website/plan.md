---
created_date: "2026-11-06"
document_status: final
closed_date: "2026-11-06"
---

# Plan: 20261106110254-f08e142d-project-website

<!-- Metadata -->
<!-- Created: 2026-11-06T12:58:00Z -->
<!-- Commit: 6469e5d -->
<!-- Branch: main -->
<!-- Repository: github.com/nicholasjackson/tcgreplay -->

## Overview

A public website for tcgreplay, served by the hosted playmat at https://tcgreplay.demo.gs. Players open a shared link and see the playmat in action in a short animation, then follow plain how-to pages to connect it to Claude or ChatGPT, import a TCG Live game, review it, drill and play. It replaces a developer-facing README as the only explanation, so the tool can be shared. The board moves to `/board` to make room for the home page, and the site carries the privacy page and home page Google's consent screen needs.

## Conventions

- **Guide text and packaging: never write "Claude" or "Nic " in an `internal/guides` guide; never exclude `internal/guides` from a build context** — the site reads the guides' frontmatter at build time and the Dockerfile gains a `site` stage and new `.dockerignore` lines, so the packaging half binds directly. The site never edits guides, so the text rule holds untouched (site pages, which are not guides, may name Claude and ChatGPT).

## Architecture & Design Decisions

The website is a static Astro site in a new top-level `site/` package (its own `package.json`, apart from the board's `web/`). It builds to plain HTML, CSS and one small script into `internal/site/dist/app`. A new Go package, `internal/site`, embeds that directory with `//go:embed all:dist`, the same way `internal/webui` embeds the board (internal/webui/server.go:26-27). A checked-in placeholder page keeps the Go build working when the site hasn't been built, so desktop builds and Go tests don't need Node. The hosted playmat serves the site from the binary at `https://tcgreplay.demo.gs`, as the user decided on 2026-11-06 instead of GitHub Pages:
- the home page at `/`
- the docs under `/docs/…`
- the privacy page at `/privacy`

A merge to main publishes it through the deploy that already exists. App Platform builds the Dockerfile, and a new `site` stage builds the site next to the existing `web` stage. No other CI is needed, and each merge goes live in one build (well inside the 15 minutes).

The hosted router in `cmd/tcgreplay/hosted.go` gains two public parts and moves the board page:
- **Board page:** it moves to `/board`, and as the user accepted, board links in past chats stop working. Sign-in is still required there, and the page goes through the same per-account front door (`reg.Board()`), with the path rewritten to `/` before the board server sees it. So the board server, its page-load and `?board=` checks (internal/usergames/board.go:78), and desktop mode (board still at `127.0.0.1:7777/`, no site) are unchanged. `/board/` redirects to `/board`.
- **Board support paths:** they stay at the root behind the session, because the built board asks for them by absolute path (web/src/lib/ws.js:13, store.svelte.js:144, session.js:12,31): `/assets/`, `/ws` and `/api/`.
- **Every other path** goes to the site handler, which needs no session and sets no cookie. It serves an unknown path with the site's own 404 page. Astro's build assets live in `/_astro/`, so they never collide with the board's `/assets/`. `/mcp`, `/auth/*`, the OAuth metadata, `/register`, `/authorize`, `/token`, `/healthz` and `/admin` are registered as today.
- **Places that write `/` and mean the board** now say `/board`:
  - the board link becomes `/board?board=<handle>` (internal/usergames/usergames.go:338)
  - the default sign-in return path (internal/hostauth/board.go:23)
  - the signed-out page's sign-in-again link (board.go:143)
  - "Open your own board" (internal/usergames/board.go:124)
  - both "Back to the board" links (internal/cacheadmin/page.go:47, internal/hostauth/admin.go:47)

Nothing else about sign-in, limits, tools or board behaviour changes.

The site reads facts from the playmat's own sources at build time instead of retyping them, so it can't drift:
- **Workflows:** the build reads each guide's `name` and `description` from `internal/guides/*.md` frontmatter, in the order in internal/guides/guides.go:17. Each guide must have a hand-written workflow page with example prompts under `site/src/content/docs/workflows/<name>.md`. The build fails if a guide has no page, or if a page names a guide that doesn't exist.
- **Limits:** the build reads requests per minute, users served at once and idle time from `.do/app.yaml` (`TCGREPLAY_RATE_LIMIT`, `TCGREPLAY_MAX_USERS`, `TCGREPLAY_IDLE_TIMEOUT`). It fails if any is missing.
- **Example game:**
  - The log is a committed copy of `testdata/logs/2026-10-11_gatorusoju_mewex-bronzong-lopunny.txt` with "GatoruSoju" replaced by "Rival".
  - The decklist is read from `testdata/decks/mega-lucario-ex-v12.txt` at build time.
  - A Go test in `internal/game` proves the committed log equals the original with that one substitution, line for line, and imports with the decklist with no problems and the same result (modelled on `TestImportCorpus`, internal/game/import_test.go:18-48).

Each page is written once in Markdown, and the README shrinks to an introduction plus links to `/` and `/docs/connect`.

The home animation is a scripted HTML/CSS scene in the board's own style. It imports the colour tokens from web/src/app.css and copies the board's no-art card look: the text card, the card back and the energy token (web/src/components/Card.svelte:73-88). It never uses card artwork or Pokémon logos. It loops through five beats:
1. a battle log arriving
2. the board filling
3. a coaching message
4. a rewind to an earlier turn
5. a different line played

It runs from the page's own files, so it works with the playmat unreachable. Under `prefers-reduced-motion: reduce` the script doesn't start, and CSS shows the scene's final frame as a still with the same caption.

Copy controls are one shared script with a button per prompt, address and example block, using `navigator.clipboard.writeText` on the block's exact text. The site uses no web fonts, analytics or third-party URLs. Card images in the docs are never needed.

This direction beats the alternatives because it puts the site exactly where the user asked, on the domain Google's consent screen already trusts. It reuses the board's embed-and-deploy pattern rather than adding a host, a DNS record or a CI system. The rejected options are recorded with evidence in research.md#alternatives-considered-and-rejected:
- GitHub Pages, and the other root layouts the user declined
- serving the site from disk instead of embedding it
- VitePress, Eleventy, Hugo and a hand-rolled generator
- a video of the real board, or mounting its components

The accepted cost is that every docs merge redeploys the playmat and drops in-memory games, as every push to main already does. The user accepted this for now.

The project's guide conventions bind the build (knowledge conventions/skills-text-and-packaging.md):
- **Packaging:** the Dockerfile's new `site` stage needs `internal/guides`, `.do/app.yaml` and `testdata/decks` in the build context. `.dockerignore` gains `site/node_modules` and `internal/site/dist/app` and never excludes `internal/guides`.
- **Guide text:** the site reads the guides but never edits them, so their "no 'Claude' or 'Nic '" rule is untouched. The site's own pages may name Claude and ChatGPT freely, since they describe connecting each.

## Component Breakdown

- **Website source (new).** The Astro project: a shared page layout with header, footer, the unofficial fan project notice and the TCGdex credit on every page, and Markdown pages:
  - the home page
  - connecting from claude.ai, ChatGPT and Claude Code, plus the desktop app
  - signing in and opening the board
  - importing a game
  - one page per coaching workflow
  - rewinding
  - the example game
  - the hosted service's limits
  - privacy
  - a short developer pointer

  It owns all user-facing wording. It depends on the build-time facts loader for anything the playmat defines.
- **Build-time facts loader (new, part of the website build).** Reads the playmat's own sources while the site builds:
  - the coaching guides' names, one-line descriptions and order
  - the hosted limits from the App Platform definition
  - the example decklist from the test corpus

  It makes them available to pages. It fails the build when a guide has no workflow page, a page names a guide that doesn't exist, or a limit is missing, so the site can't drift from the playmat.
- **Home animation (new, part of the website).** A self-contained scripted scene of the playmat in the board's colours and no-art card style. It loops through the five beats (log arrives, board fills, coaching message, rewind, different line), and shows its last frame still under reduced motion. It loads nothing from the playmat or any other host.
- **Copy controls (new, part of the website).** One small shared script that gives every example prompt, the playmat's address and the example game a copy button that puts exactly that block's text on the clipboard.
- **Example game (new content).** The corpus game with the opponent's name replaced by a placeholder, committed beside the site. A Go test guards it: it checks the file is the original with only that substitution, and that it imports with its decklist cleanly and with the same result.
- **Site package (new Go package).** Embeds the built website in the binary, with a placeholder page when it hasn't been built. It serves it as plain static files: correct content types, long caching for hashed build assets, no caching for pages, the site's own page for unknown paths, and no cookies. It mirrors how the board UI is embedded.
- **Hosted router (changed).** Sends the board page at `/board` to the existing per-account board front door, rewritten so the board sees its usual root. It keeps the board's support paths (assets, live connection, card lookups) behind sign-in where they are, and gives every other path to the site package without sign-in. Sign-in, MCP, OAuth, health and admin routes are unchanged.
- **Board link and sign-in return paths (changed).** The board link in tool results, the default place sign-in returns to, and the links on the signed-out, "not your game", "not an admin" and admin pages now point at `/board` instead of `/`.
- **Container build (changed).** Gains a stage that builds the website, alongside the board's stage, and copies it into the site package before the Go build. The build context ignores the site's dependencies and build output and still includes the guides, the App Platform definition and the test decklist.
- **Developer tooling (changed).** Make targets to build, preview and test the site, run from the existing test target. The website's browser tests reuse the board's Playwright setup and conventions (Chromium-only profiles).
- **README and design notes (changed).** The README becomes a brief introduction that links to the site's home and connecting pages instead of carrying the instructions. The design and build notes record the new routes (site at `/`, board at `/board`) and the site's build.
- **Operator setup (human).** Entering the home page and privacy page addresses on Google's OAuth consent screen once the site is live.

## Data Structures & Interfaces

**Go: the site package.** One exported constructor returns the embedded site as an `http.Handler`. The hosted router mounts it at every path it doesn't route elsewhere.

```go
package site

// Handler serves the embedded website: pages, build assets and the site's
// 404 page. It never sets cookies and needs no sign-in. Built returns false
// when only the placeholder is embedded (the site was not built).
func Handler() http.Handler
func Built() bool
```

**Go: hosted routes (a contract, not a type).** The path table the hosted router guarantees:

| Path | Served by | Sign-in |
|---|---|---|
| `/mcp` | MCP (unchanged) | access token |
| `/auth/*`, OAuth metadata, `/register`, `/authorize`, `/token`, `/healthz` | sign-in service (unchanged) | none |
| `/admin`, `/admin/*` | cache admin (unchanged) | admin |
| `/board` (`/board/` redirects to it) | board front door, path rewritten to `/` | session |
| `/assets/*`, `/ws`, `/api/*` | board front door (unchanged paths) | session |
| everything else (`/`, `/docs/…`, `/privacy`, `/_astro/*`, `/logo.png`, …) | site package | none |

**Board link format (changed).** `<public>/board?board=<handle>` replaces `<public>/?board=<handle>`. The handle itself is unchanged.

**Website build: facts available to pages.** Produced by the build-time loader. Pages read these values instead of hard-coding them.

```ts
type Workflow = { name: string; description: string; order: number }  // from internal/guides/*.md frontmatter + guide order
type Limits   = { requestsPerMinute: number; maxUsers: number; idleTimeout: string /* e.g. "60 minutes" */ }  // from .do/app.yaml
type Example  = { log: string /* anonymised battle log */; deck: string /* TCG Live export */ }
const site    = { publicURL: "https://tcgreplay.demo.gs", mcpURL: "https://tcgreplay.demo.gs/mcp", repoURL: "https://github.com/nicholasjackson/tcgreplay" }
```

**Website content: a workflow page.** A Markdown file per guide, whose frontmatter names the guide it documents. The build pairs the pages with the guides one to one.

```yaml
---
guide: review-game          # must equal a guide's name
title: Review a game
prompts:                    # at least one; each gets a copy button
  - "Review this game with me. Start with the turn where I lost tempo."
---
```

**Example game files.** `site/src/content/example/battle-log.txt` is the corpus log with "GatoruSoju" → "Rival". The decklist is read from `testdata/decks/mega-lucario-ex-v12.txt` and not copied.

## Implementation Detail

**A second embedded web app, following the board's pattern.** The board is a Node-built front end compiled into a Go package and embedded with a placeholder fallback. The website repeats that exactly: Node builds it into a Go package's `dist`, and Go embeds it with a placeholder when it hasn't been built. A developer who knows how the board ships knows how the site ships. Its Make targets, Dockerfile stage and `.dockerignore` lines sit beside the board's equivalents and read the same way. Desktop builds embed only the placeholder unless the site is built, and desktop mode never serves it.

**The hosted router becomes an explicit path table.** Today it is "sign-in routes, `/mcp`, `/admin`, and everything else is the board". It becomes:
- sign-in, `/mcp` and `/admin` as before
- the board page at `/board`
- the board's support paths
- everything else is the public site

The board keeps seeing itself at the root: a small path-rewriting wrapper hands it `/` for `/board`. So the board server, the per-account front door and desktop mode don't change. The only other playmat edits are the handful of places that spell the board's address. Those move to `/board`, and existing tests that pin `/` or `/?board=` are updated to the new address, not loosened.

**Content is Markdown. Facts come from the playmat.** Pages are Markdown with a little frontmatter, under one shared layout that carries the header, footer, fan notice and credit. Anything the playmat defines is read at build time and checked, never retyped:
- the workflows and their order (from the guides)
- the hosted limits (from the App Platform definition)
- the example decklist (from the test corpus)

A guide added to the playmat without a page, or a page for a guide that's gone, breaks the site's build. That is the new safeguard keeping docs and product in step. Each page's example prompts sit in frontmatter or fenced blocks and are rendered with a copy button. Pages never assemble prompts in script.

**The animation is a small scripted scene, not a dependency.** One Astro component holds the scene's markup in the board's tokens and no-art card style. A short script steps through the five beats by toggling classes on a timer, and CSS transitions do the motion. With reduced motion the script exits early, and the CSS shows the final frame as a still. The site has no front-end framework, animation library, web font or external asset.

**The example game is guarded from both sides.** Its committed log is plain text the site shows and copies. A Go test sits beside the corpus import test and checks two things: that it is the corpus game with only the opponent's name changed, and that it imports cleanly with the same result. Editing it by hand without keeping it importable fails the Go tests.

**Testing follows the board's browser-suite conventions.** The site gets its own Playwright project in the site package, with Chromium-only profiles: a 1366×768 laptop and a 375-pixel phone. It runs against a built site served the way production serves it, through the Go site handler, so the routes and headers under test are the real ones. Go tests cover the router table, the board link format, and the site package's caching and cookie behaviour.

## Dependencies

- **Design documents this plan was built on: none.** The spec carries no design references.
- **Spec 20261106110254-f08e142d-project-website** — the source of scope. It was amended on 2026-11-06 with the user's hosting decisions (site at tcgreplay.demo.gs, board at `/board`, redeploys accepted).
- **Astro (current release, 7.x) — new dev dependency of the site package only.** Builds Markdown pages to static HTML and bundles the one site script. Not used at runtime and not in the board's package. The site needs no front-end framework integration.
- **A YAML parser (e.g. `yaml`) — new dev dependency of the site package.** Reads the limits from the App Platform definition at build time.
- **Playwright (already used by the board, 1.64.x).** The site's browser tests reuse it with Chromium-only profiles (knowledge gotchas/playwright-touch-tests.md); no new browser installs.
- **Node 22 (already in the Dockerfile's board stage).** The site stage uses the same image.
- **Go standard library `embed`, `net/http`, `io/fs`** — the site package. No new Go modules.
- **The playmat's guides (internal), the App Platform definition, and the test corpus decklist and game log.** Read, not changed. The App Platform definition remains the single place the limits are set.
- **The hosted router, the per-account registry's board link, and the sign-in service's return paths and pages (internal).** Changed, as described above, to move the board to `/board`.
- **The board's colour tokens (internal).** Imported read-only by the site so the animation matches the board's palette.
- **Google Cloud OAuth consent screen (external service).** After launch the operator enters `https://tcgreplay.demo.gs/` as the home page and `https://tcgreplay.demo.gs/privacy` as the privacy policy (a human task). `demo.gs` is assumed to be an authorized domain already.
- **DigitalOcean App Platform (external service, unchanged).** Its push-to-main deploy publishes the site. Every merge redeploys the playmat; the user accepted this.
- **Prior plans that must land first: none.** The TCGdex Spaces cache epic is complete. The read-only tool annotations, the ChatGPT name mapping and the CDN scope doc fixes are uncommitted in the working tree; they are independent of this plan and may land in either order.

## Testing Approach

**Three layers, each slotting into an existing convention.**

- **Go unit and integration tests**, beside the existing hosted, usergames and hostauth tests and run by the Go test suite:
  - **Hosted routing:** the route table. Signed out, `/`, a docs page, `/privacy` and a site asset are served without a redirect or a `Set-Cookie`. `/board` sends a signed-out browser to sign-in that returns to `/board`, and serves a signed-in account's board. `/board/` redirects to `/board`. `/assets/`, `/ws` and `/api/` still need a session. `/mcp`, `/auth/*`, `/healthz` and `/admin` behave exactly as before. An unknown path gets the site's 404 page with status 404.
  - **Board link:** tool results carry `<public>/board?board=<handle>`. The existing tests that pinned `/?board=` and `next=/` are updated to the new address. The "not your game", signed-out and admin pages link to `/board`.
  - **Site package:** the placeholder is served when the site isn't built. Pages are sent with no-cache headers and hashed build assets with long-lived ones. No response sets a cookie.
  - **Example game:** the site's example log equals the corpus game with only "GatoruSoju" → "Rival". It contains no "GatoruSoju". It imports with the corpus decklist with zero problems and zero invariant warnings, and gives the same winner and prize counts as the original (from `testdata/index.json`).
- **Build-time checks in the site build**, which fail the site build and so the container build:
  - every guide has exactly one workflow page, and every workflow page names a real guide
  - all three limits are present in the App Platform definition and parse
  - every workflow page has at least one example prompt
- **Browser tests (Playwright, Chromium-only profiles as the board's suite)** against the built site served through the Go site handler:
  - Laptop 1366×768: the pitch, the animation and "Get started" are within the first viewport, and "Get started" opens the connecting page.
  - The animation runs with every request to the playmat's API and board paths blocked. Its beats appear in order (log, board, coaching, rewind, new line) and it starts over.
  - With `prefers-reduced-motion: reduce` emulated, a still frame is shown and no element animates: no running CSS animations or transitions, and the scene's frame doesn't change over a few seconds.
  - Every page shows the fan notice naming Nintendo, The Pokémon Company, Creatures and Game Freak, and the TCGdex credit. Pages are crawled from the site's own links.
  - Phone 375 px: no page scrolls sideways (document and main checked, per the board's phone gotcha), and code, prompt and address blocks wrap or scroll inside their own box.
  - Every copy button puts exactly its block's text on the clipboard, with clipboard permissions granted.
  - In a fresh context, loading every page sets no cookies and makes no request to any host but the site's own.
  - Every workflow the playmat defines has a section with a prompt on the site. The limits shown match the App Platform definition. The privacy page exists at `/privacy`. The developer section links to the repository.
  - The home page's total transfer size is under 1.5 MB.

**Most coverage goes to the router and the build-time checks.** They are the two places where a mistake breaks the live playmat (a board that won't load) or lets the docs drift from the product. Page wording is covered by presence checks, not snapshots, so editing copy doesn't churn tests.

**Deliberate gaps:**
- No visual-regression snapshots of the animation. Its order and looping are asserted structurally instead.
- No WebKit runs, per the board's gotcha.
- The README change is checked by reading it during the goal check, not by a test.

**Success metrics:**
- *New player connects and imports the example game in under 10 minutes, unaided, on two of three tries with different people* — **Manual — captured in the implementation test plan.**
- *Home page shows its largest content in under 2.5 s in a Lighthouse mobile audit (Slow 4G), with total download under 1.5 MB* — the download half is a **behavioural test** (the browser test above sums the home page's transfer size). The largest-content time is **Manual — captured in the implementation test plan** (a Lighthouse run against the deployed site).
- *Every page scores at least 90 for accessibility and performance in a Lighthouse mobile audit* — **Manual — captured in the implementation test plan.**

**Manual reviews before acceptance:**
- **Manual — captured in the implementation test plan:** each assistant's connecting steps (claude.ai, ChatGPT, Claude Code) followed by someone new to tcgreplay, ending with the assistant confirming it reaches the playmat.
- **Manual — captured in the implementation test plan:** the desktop steps on a machine with the listed prerequisites, ending at the board on `127.0.0.1:7777`.
- **Manual — captured in the implementation test plan:** importing the site's example game from a real assistant per the import page, reporting no problems.
- **Manual — captured in the implementation test plan:** after deploy, the live site shows a merged content change within 15 minutes, `/board` loads a signed-in board, and a board link from a new tool result opens it.
- **Manual — captured in the implementation test plan:** a read-through of the privacy and limits pages against the playmat's behaviour on the day of publishing.
- **Manual — captured in the implementation test plan:** the import page's directions for copying the battle log and decklist checked against TCG Live's current menus.
- **Manual — captured in the implementation test plan:** the owner's look-and-feel review of the home page animation (board style, no card art or logos).

## Milestones & Tasks

### Milestone 1: tcgreplay.demo.gs opens on a home page that gets people connected, and the board moves to /board
**What changes**: Someone opening `https://tcgreplay.demo.gs` sees a public home page instead of being sent straight to Google sign-in. The page says in one sentence what tcgreplay is, shows a still of the playmat in the board's style, and has a "Get started" link to step-by-step connecting instructions for claude.ai, ChatGPT, Claude Code and the Claude desktop app, with what to expect from the Google sign-in and how to open the board link. Every page carries the unofficial fan project notice and the TCGdex credit, and a privacy page at `/privacy` says what the playmat collects, how long it keeps it and what it never collects. The board itself now lives at `/board`. Board links in new assistant replies point there, signing in returns there, and the admin and "not your game" pages link there. Board links in past chats stop working, as the user accepted. Connectors at `/mcp` are untouched. The site ships inside the playmat and goes live with each normal deploy.

**Validation point**: Go tests show public site paths serve without sign-in or cookies and the board needs sign-in at `/board`, with the new link format in tool results. The site's browser tests pass for the home page, privacy page, connecting pages, the address copy button, the notice on every page, phone width, and no cookies or third-party requests. After a deploy, the live address shows the home page and a fresh board link opens the board.

#### - [ ] Task: Embed and serve the website from a Go site package
**Id:** 603b3ddb-f67a-41c7-8abe-63bb41727ef2
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Add a small Go package that carries the built website inside the playmat binary, the same way the board UI is carried. It serves pages, build assets and the site's own "page not found" page as plain static files, and it falls back to a placeholder page when the site hasn't been built. Pages are never cached by browsers, hashed build assets are cached for a long time, and no response sets a cookie.

*Technical detail:* [context.md#task-embed-and-serve-the-website-from-a-go-site-package](./context.md#task-embed-and-serve-the-website-from-a-go-site-package)

**Acceptance criteria**:
- [ ] The playmat binary builds and its tests pass on a machine that has never built the website, serving a placeholder page.
- [ ] When the website has been built, its pages, assets and 404 page are served from the binary with the right content types.
- [ ] Pages are sent with headers that stop browsers caching them, hashed build assets with long-lived caching, and no response sets a cookie.
- [ ] An unknown address gets the site's own 404 page with a 404 status.

#### - [ ] Task: Move the board to /board and serve the site on every other path
**Id:** 5f2c6513-b004-460f-ab3d-fc25e2ef9b41
**Repo:** tcgreplay
**Depends on:**
- 603b3ddb-f67a-41c7-8abe-63bb41727ef2 — Embed and serve the website from a Go site package
**Execution:** agent

Change the hosted playmat's routing so the website is public at every path it doesn't otherwise use, and the board page lives at `/board` behind sign-in. The board's own support addresses (its files, live connection and card lookups) stay where they are behind sign-in, so the board itself doesn't change. Board links in tool results, the place sign-in returns to, and the links on the signed-out, "not your game", "not an admin" and admin pages all point at `/board`. MCP, sign-in, health and admin routes are untouched, and desktop mode is unchanged.

*Technical detail:* [context.md#task-move-the-board-to-board-and-serve-the-site-on-every-other-path](./context.md#task-move-the-board-to-board-and-serve-the-site-on-every-other-path)

**Acceptance criteria**:
- [ ] A signed-out visitor to `/`, a docs address or `/privacy` gets the website with no redirect to Google and no cookie set.
- [ ] A signed-out visitor to `/board` is sent to Google sign-in and comes back to `/board`; a signed-in visitor gets their own board there, and `/board/` redirects to `/board`.
- [ ] Board links returned by `import_log`, `new_game` and `set_opponent_deck` have the form `https://tcgreplay.demo.gs/board?board=<handle>`, and opening one in the signed-in browser shows that account's board.
- [ ] The board's files, live connection and card lookups still need sign-in, and the board works at `/board` as it did at `/`.
- [ ] `/mcp`, sign-in, OAuth, health and admin addresses behave exactly as before, and the desktop playmat still serves its board at `http://127.0.0.1:7777/` with no website.
- [ ] The signed-out, "not your game", "not an admin" and admin pages link to `/board`.

#### - [ ] Task: Scaffold the website build, layout and test harness
**Id:** b7c5ba5f-02c1-46c8-a7ad-2b9ed9550592
**Repo:** tcgreplay
**Depends on:**
- 603b3ddb-f67a-41c7-8abe-63bb41727ef2 — Embed and serve the website from a Go site package
**Execution:** agent

Create the website project. It holds Astro, a shared page layout and the shared site script, and builds into the Go site package. The layout carries the logo, navigation, the unofficial fan project notice naming Nintendo, The Pokémon Company, Creatures and Game Freak, and the TCGdex credit on every page. A shared copy-button script and a build-time loader for the playmat's address and service limits are part of the scaffold. Make targets build and test the site, the container build gains a stage that builds it, and a browser test suite runs against the built site served through the Go handler.

*Technical detail:* [context.md#task-scaffold-the-website-build-layout-and-test-harness](./context.md#task-scaffold-the-website-build-layout-and-test-harness)

**Acceptance criteria**:
- [ ] Building the site puts plain HTML pages into the Go site package, and the container build produces a playmat image that serves them.
- [ ] Every page shows the logo, the site navigation, the unofficial fan project notice naming all four companies, and the TCGdex credit.
- [ ] The site build fails with a clear message if any of the three service limits is missing from the App Platform definition.
- [ ] Any block marked as copyable gets a copy button that puts exactly that block's text on the clipboard.
- [ ] The browser suite runs from the project's test target against the built site, and checks every page for the notice, sideways scrolling at 375 pixels, cookies, and requests to other hosts.
- [ ] The site loads no web fonts, analytics or resources from any other host.

#### - [ ] Task: Write the home page and the privacy page
**Id:** b6dd327b-6407-4ad9-9fbf-71c92c547cbc
**Repo:** tcgreplay
**Depends on:**
- b7c5ba5f-02c1-46c8-a7ad-2b9ed9550592 — Scaffold the website build, layout and test harness
**Execution:** agent

Write the home page: a one-sentence description of tcgreplay, a still scene of the playmat drawn in the board's own colours and no-art card style, and a "Get started" link to the connecting instructions, all visible without scrolling on a laptop. Write the privacy page at `/privacy`, stating:
- what the hosted playmat collects at sign-in
- what it keeps while someone plays
- how long each is kept
- which outside services are involved
- what it never collects

The privacy page uses the facts the playmat implements today.

*Technical detail:* [context.md#task-write-the-home-page-and-the-privacy-page](./context.md#task-write-the-home-page-and-the-privacy-page)

**Acceptance criteria**:
- [ ] On a 1366×768 window the home page shows the one-sentence description, the playmat scene and a "Get started" link without scrolling.
- [ ] The scene shows a playmat in the board's style with plain stand-in cards, and no official card artwork or Pokémon logos.
- [ ] The privacy page at `/privacy` states what is collected at sign-in (Google account id, email and its verified flag), what is kept while playing and that it is held in memory only, how long each is kept, the outside services involved, and what is never collected.
- [ ] The privacy page's idle time comes from the App Platform definition, not typed in.

#### - [ ] Task: Write the connecting, desktop and sign-in pages
**Id:** 11845bab-16a7-4f15-9d70-ac5dbe866883
**Repo:** tcgreplay
**Depends on:**
- b7c5ba5f-02c1-46c8-a7ad-2b9ed9550592 — Scaffold the website build, layout and test harness
**Execution:** agent

Write the connecting instructions, the page the home page's "Get started" link opens. It covers:
- **claude.ai, ChatGPT and Claude Code:** step-by-step instructions for each, ending with how to check the assistant can reach the playmat.
- **Desktop:** installing the playmat and connecting the Claude desktop app, with prerequisites and the board's address there.
- **Sign-in and the board link:** what the Google sign-in looks like the first time, and how to open the board link from the assistant's reply.

The playmat's address has a copy button.

*Technical detail:* [context.md#task-write-the-connecting-desktop-and-sign-in-pages](./context.md#task-write-the-connecting-desktop-and-sign-in-pages)

**Acceptance criteria**:
- [ ] The home page's "Get started" link opens the connecting instructions.
- [ ] There are separate step-by-step instructions for claude.ai, ChatGPT and Claude Code, and each ends by telling the user how to check the connection works.
- [ ] The desktop instructions list the prerequisites, the install steps, the Claude desktop app setting and the board's address on the desktop.
- [ ] The sign-in and board-link text is reachable from the connecting instructions and describes the Google prompt and opening the board link (at `/board`).
- [ ] The playmat's MCP address has a copy button that copies exactly `https://tcgreplay.demo.gs/mcp`.

### Milestone 2: The how-to guide: import, review, drill and play
**What changes**: The site becomes the full user guide.
- **Importing:** how to import a TCG Live battle log with its decklist, and what a complete import looks like.
- **Workflows:** a page for every coaching workflow the playmat offers, each with example prompts, plus rewinding to try another line.
- **Example game:** a ready-made, anonymised example game with its decklist.
- **Limits:** the hosted service's limits, with the desktop option for when it is full.
- **Developers:** a short pointer to the repository.

Every prompt and the example game have one-click copy buttons. Workflow names and service limits come from the playmat itself, so the site can't drift. The README shrinks to an introduction that links here.

**Validation point**: The site build fails if a workflow page or limit is missing. The Go test proves the example game imports cleanly. Browser tests confirm copy buttons, workflow coverage, limits matching the App Platform settings, and no sideways scrolling on a phone. The README links to the home and connecting pages.

#### - [ ] Task: Write a page for every coaching workflow, read from the playmat's guides
**Id:** 0d86023b-e1a4-4cce-a547-5aad2d1b2dad
**Repo:** tcgreplay
**Depends on:**
- b7c5ba5f-02c1-46c8-a7ad-2b9ed9550592 — Scaffold the website build, layout and test harness
**Execution:** agent

Make the site read the playmat's coaching guides at build time and require a matching page for each, so a workflow added or removed in the playmat breaks the site's build until the docs follow. Write those pages, each in plain language with at least one copyable example prompt:
- play a game
- import a game
- the opponent's likely deck
- review a game
- replay drill
- before you move
- numbers drill

Also write a page on rewinding an imported game to a chosen turn and trying another line.

*Technical detail:* [context.md#task-write-a-page-for-every-coaching-workflow-read-from-the-playmats-guides](./context.md#task-write-a-page-for-every-coaching-workflow-read-from-the-playmats-guides)

**Acceptance criteria**:
- [ ] Every coaching workflow the playmat offers has a page on the site with at least one copyable example prompt, listed in the playmat's own order.
- [ ] The site build fails if a workflow has no page, a page names a workflow the playmat doesn't have, or a page has no example prompt.
- [ ] A page explains how to rewind an imported game to a chosen turn and play a different line, with an example prompt.
- [ ] The workflow descriptions shown come from the playmat's guides, not retyped.

#### - [ ] Task: Publish the anonymised example game and the import page
**Id:** cb1c4655-a05b-4285-a115-655e38a7b3df
**Repo:** tcgreplay
**Depends on:**
- b7c5ba5f-02c1-46c8-a7ad-2b9ed9550592 — Scaffold the website build, layout and test harness
**Execution:** agent

Add the example game: the corpus game from 11 October 2026 with the opponent's player name replaced by "Rival", and its Mega Lucario ex decklist, each with a copy button. A Go test guards it: the file must be the original with only that name changed, and it must import with its decklist cleanly and with the original result. Write the import page. It covers:
- where to copy the battle log and the decklist in TCG Live
- what to paste
- what a complete import looks like
- what to do if lines are missing, such as a chat app collapsing a long paste

*Technical detail:* [context.md#task-publish-the-anonymised-example-game-and-the-import-page](./context.md#task-publish-the-anonymised-example-game-and-the-import-page)

**Acceptance criteria**:
- [ ] The example page shows the complete battle log from setup to the result and its decklist, each with a copy button that copies exactly that text.
- [ ] The example log contains no real opponent's name, and a test fails if it differs from the original game in anything but that name.
- [ ] A test imports the example log with its decklist and finds no problems and the same winner and prize counts as the original game.
- [ ] The import page explains where to copy the log and the decklist in TCG Live, what a complete import looks like, and what to do if lines are missing.

#### - [ ] Task: Write the hosted limits and developer pages
**Id:** 8fdc8d6b-84a1-4306-8bb1-9e3116500e38
**Repo:** tcgreplay
**Depends on:**
- b7c5ba5f-02c1-46c8-a7ad-2b9ed9550592 — Scaffold the website build, layout and test harness
**Execution:** agent

Write the page on the hosted playmat's limits. It covers:
- what the playmat keeps, and that games are dropped after the idle time and on every update
- how to keep a game's record and import it again
- how many players it serves at once and the requests per minute
- what the "busy" and "too many requests" messages mean
- running it on the desktop when the hosted playmat is full

The numbers come from the App Platform definition. Write a short developer section that links to the source repository for running it locally and contributing, without copying the repository's developer notes.

*Technical detail:* [context.md#task-write-the-hosted-limits-and-developer-pages](./context.md#task-write-the-hosted-limits-and-developer-pages)

**Acceptance criteria**:
- [ ] The limits page's idle time, players served at once and requests per minute match the App Platform definition, and change when it does.
- [ ] The limits page says what the hosted playmat keeps, that games are lost when idle or updated, that a record can be kept and imported again, and points to the desktop option when the playmat is full.
- [ ] The developer section links to the source repository and contains no build or test instructions beyond that pointer.

#### - [ ] Task: Point the README at the site and record the new routes
**Id:** 4fcd4d8b-f215-48f0-926e-535fe36c7d1c
**Repo:** tcgreplay
**Depends on:**
- 5f2c6513-b004-460f-ab3d-fc25e2ef9b41 — Move the board to /board and serve the site on every other path
- 11845bab-16a7-4f15-9d70-ac5dbe866883 — Write the connecting, desktop and sign-in pages
- 0d86023b-e1a4-4cce-a547-5aad2d1b2dad — Write a page for every coaching workflow, read from the playmat's guides
**Execution:** agent

Shorten the README to a brief introduction with the logo, linking to the site's home page and connecting instructions in place of the step-by-step user instructions and example prompts, and keeping its developer notes. Update the design and build notes and the deploy definition's comments to record the new routes (site at `/`, board at `/board`), how the site is built and shipped, and that a docs merge redeploys the playmat.

*Technical detail:* [context.md#task-point-the-readme-at-the-site-and-record-the-new-routes](./context.md#task-point-the-readme-at-the-site-and-record-the-new-routes)

**Acceptance criteria**:
- [ ] The README links to the site's home page and its connecting instructions, and no longer contains the step-by-step connecting instructions or the example prompts.
- [ ] The README still carries a brief description, the logo and the developer notes.
- [ ] The design notes and the deploy definition's comments describe the site at `/`, the board at `/board`, how the site is built, and that each push to main redeploys the playmat.

### Milestone 3: The home page shows the playmat in action, and Google's consent screen points at the site
**What changes**: The home page's still is replaced by a short looping animation in the board's own look, with no card artwork or logos. A battle log arrives, the board fills with the game, the assistant coaches, the game rewinds to a decision and a different line is played. It plays on its own, needs nothing from the playmat, and shows a still to visitors who ask for reduced motion. Once the site is live, the operator enters its home and privacy addresses on Google's OAuth consent screen.

**Validation point**: Browser tests show the animation's beats in order and looping with the playmat blocked, a still with no motion under reduced motion, and a home page under 1.5 MB. The owner's look-and-feel review and the Lighthouse checks are recorded in the implementation test plan. The consent screen lists the site's home and privacy addresses.

#### - [ ] Task: Animate the home page's playmat scene
**Id:** 8eda3982-e121-4fdf-abb3-f81dc1235958
**Repo:** tcgreplay
**Depends on:**
- b6dd327b-6407-4ad9-9fbf-71c92c547cbc — Write the home page and the privacy page
**Execution:** agent

Turn the home page's still scene into a short looping animation. A battle log arrives, the board fills with the game, an assistant's coaching message appears, the game rewinds to an earlier turn, and a different line is played. It plays on its own from the page's own files, needs nothing from the playmat, and keeps the board's look with no card artwork or logos. For visitors who ask for reduced motion, it stays as the still, and nothing on the page moves.

*Technical detail:* [context.md#task-animate-the-home-pages-playmat-scene](./context.md#task-animate-the-home-pages-playmat-scene)

**Acceptance criteria**:
- [ ] With the playmat's addresses blocked, the home page animation plays without being touched, shows in order a battle log arriving, the board filling, a coaching message, a rewind to an earlier turn and a different line, then starts again.
- [ ] With the reduced-motion setting on, the home page shows the still scene and nothing on the page moves.
- [ ] The home page's total download, animation included, is under 1.5 MB.
- [ ] The animation uses the board's colours and plain stand-in cards, with no official card artwork or Pokémon logos.

#### - [ ] Task: Enter the site's addresses on Google's OAuth consent screen
**Id:** 797471ef-70b2-493f-9f4c-536d0a045c85
**Repo:** tcgreplay
**Depends on:**
- 5f2c6513-b004-460f-ab3d-fc25e2ef9b41 — Move the board to /board and serve the site on every other path
- b6dd327b-6407-4ad9-9fbf-71c92c547cbc — Write the home page and the privacy page
**Execution:** human — needs the Google Cloud console for the playmat's OAuth project and a deployed site

Once the site is deployed, open the playmat's OAuth consent screen in the Google Cloud console. Set the application home page to `https://tcgreplay.demo.gs/` and the privacy policy to `https://tcgreplay.demo.gs/privacy`, confirming `demo.gs` is listed as an authorized domain. This lets Google accept the app's public pages without a new domain.

*Technical detail:* [context.md#task-enter-the-sites-addresses-on-googles-oauth-consent-screen](./context.md#task-enter-the-sites-addresses-on-googles-oauth-consent-screen)

**Acceptance criteria**:
- [ ] The consent screen's home page is `https://tcgreplay.demo.gs/` and its privacy policy is `https://tcgreplay.demo.gs/privacy`, both loading publicly.
- [ ] `demo.gs` is an authorized domain on the consent screen, and Google sign-in to the playmat still works afterwards.

## Open Questions

- **Does the anonymised example game import cleanly with "Rival" in place of "GatoruSoju"?** It depends on the importer's name detection, which can only be proven by running it. The implementer runs the example-game test first. If it reports problems that the original game doesn't, STOP and ask the user before changing the placeholder or the importer: the importer is out of scope.
- **Does Astro's `glob` loader accept a `base` outside the site project (`../internal/guides`) in the pinned release?** It depends on the release installed at implementation time. If it doesn't, read the guides with a plain `fs` read and frontmatter parse in `site/src/lib/workflows.ts` instead. Same contract, no need to ask.

## Out of Scope

- **Changes to the playmat beyond serving the site and moving the board to `/board`.** Its sign-in, limits, tools and board behaviour stay as they are (spec Non-Goals). The OAuth refusal text that still mentions only Claude's callback (`redirect_uri must be Claude's callback or a loopback address`) is a separate small fix, reported to the user and not tracked yet.
- **Keeping docs merges from restarting the playmat.** Every push to main redeploys it. The user accepted this for now and declined both a separate site repo and deploying from GitHub Actions. There is no follow-up yet.
- **Redirecting old board links (`/?board=…`) to `/board`.** The user chose home at `/` always and accepted that links in past chats break.
- **A separate domain or host for the site, including GitHub Pages** (spec Non-Goals, as amended).
- **Accounts, comments, a blog, newsletters, search, versioned docs and translations** (spec Non-Goals).
- **An API reference or contribution guide.** The developer section only points to the repository (spec Non-Goals).
- **Replacing the project logo.** The site uses the current logo until the user decides whether it suits a public site (spec Non-Goals).
- **A "Help" or "Docs" link inside the board's menu.** Useful, but it is a board change the spec doesn't ask for.
- **Serving the website in desktop mode.** The desktop playmat keeps its board at the root, with no website.
- **Google brand verification.** Entering the home page and privacy addresses is in scope. Submitting the app for brand verification is optional and left to the operator.
