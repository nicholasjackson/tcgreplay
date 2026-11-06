### Chosen direction: Astro site embedded in the playmat binary (architecture)
- **Decision**: build `site/` with Astro into `internal/site/dist/app`, embed it in a new `internal/site` package, and serve it from the hosted router. The board page moves to `/board` (rewritten to `/` for the board server). `/assets/`, `/ws` and `/api/` stay behind the session. Every other path is the public site. Facts (workflows, limits, example deck) are read from the playmat's sources at build time. Publishing is the existing App Platform deploy, with a new Dockerfile `site` stage.
- **Rationale**: it is what the user chose (everything at tcgreplay.demo.gs; home at `/`, board at `/board`). It mirrors the board's proven embed pattern (internal/webui/server.go:26-27), needs no new host, CI or DNS, and keeps desktop mode and the board server unchanged.
- **Rejected**: copying the site into the image and serving it from a disk path (a new flag and runtime path in a distroless image, and untestable in `go test`); rewriting the board's JS to a `/board` base (touches every absolute path in web/src and desktop mode for no gain); a separate site process or container (App Platform would need a second component and routing).

### Board support paths stay at the root (architecture)
- **Decision**: only the board's page moves to `/board`. `/assets/`, `/ws` and `/api/` stay at the root behind sign-in.
- **Rationale**: the built board asks for them by absolute path (ws.js:13, store.svelte.js:144, Vite base `/`). Moving them would mean rebuilding the board with a base path and changing desktop mode too.
- **Rejected**: Vite `base: '/board/'` and prefixed `/board/ws` and `/board/api` (larger change, and it splits desktop and hosted builds).

### Conventions applied (architecture)
- **Decision**: apply conventions/skills-text-and-packaging.md (the only convention in the store).
- **Rationale**: the site build reads `internal/guides`, and the Dockerfile and `.dockerignore` change.
- **Rejected**: none dropped.

### Site generator: Astro (discovery)
- **Decision**: Astro, in its own `site/` package, building to plain HTML embedded in the playmat binary.
- **Rationale**: Markdown to HTML with zero client JS by default, one bundled script for the animation and copy controls, and content loaders that read the playmat's own guides and settings at build time. Node is already in the toolchain.
- **Rejected**: VitePress (ships a Vue SPA runtime and a foreign theme); Eleventy (needs a separate bundling step for the scripts); Hugo (a second build tool); a hand-rolled script (layouts, hashing and the Markdown pipeline to write and test).

### Home animation: scripted HTML/CSS scene in the board's no-art style (discovery)
- **Decision**: a hand-built scene using the board's tokens and its text-card, card-back and energy-token look, driven by a short script; with reduced motion, its final frame is shown still.
- **Rationale**: no official art (constraint), works offline, tiny download, and it reads as the real board.
- **Rejected**: a video of the real board (shows card art and is heavy); mounting the board's components (coupled to its live store and WebSocket).

### Example opponent placeholder name "Rival" (discovery)
- **Decision**: replace "GatoruSoju" with "Rival" throughout the example log; the owner stays "NicJackson" (the user's own name; the spec anonymises only the opponent).
- **Rationale**: a single word with no spaces parses like a TCG Live screen name, and it doesn't clash with the literal "Opponent" that TCG Live writes in result lines. A Go test proves the import is clean.
- **Rejected**: "Opponent" (TCG Live already prints it in "Opponent took all of their Prize cards"); "Player 2" (has a space).

### Connecting pages ship in milestone 1 (tasks)
- **Decision**: the connecting, desktop and sign-in pages are in milestone 1 with the home page, not milestone 2.
- **Rationale**: the home page's "Get started" link must open the connecting instructions. Shipping milestone 1 without them would put a dead link live, and they are what makes the site shareable.
- **Rejected**: a stub connect page in milestone 1 (a deployable broken experience).

### TCG Live menu names left general (tasks)
- **Decision**: the import page describes where to copy the log and decklist in general terms, and the exact menu names are a manual check.
- **Rationale**: the menu wording can't be verified from the code, and a wrong menu name misleads new players.
- **Rejected**: guessing exact menu paths.
