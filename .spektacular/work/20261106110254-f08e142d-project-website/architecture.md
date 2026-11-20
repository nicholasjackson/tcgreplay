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
