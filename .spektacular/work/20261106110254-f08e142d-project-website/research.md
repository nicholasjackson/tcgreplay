## Alternatives considered and rejected

- **GitHub Pages (`nicholasjackson.github.io/tcgreplay` or a demo.gs subdomain).** Rejected by the user on 2026-11-06 in favour of serving everything at tcgreplay.demo.gs. Google has also rejected github.io pages as a home page and privacy policy because their ownership can't be verified (discuss.google.dev thread, below).
- **Board stays at `/`, with the site under `/about/`; or the home page only for signed-out visitors at `/`.** Both offered; the user chose "home at `/`, board at `/board`", accepting that board links in past chats stop working.
- **Separate site repo, or deploying the playmat from GitHub Actions with a path filter so docs edits don't restart it.** Offered; the user chose to accept the restarts for now.
- **Record a video of the real board for the home animation.** Rejected: the real board shows TCGdex card artwork (web/src/components/Card.svelte:81-82), which is official Pokémon card art and is barred by the Constraints. Video also costs far more of the 1.5 MB home-page budget than a scripted scene.
- **Mount the board's own Svelte components in the site.** Rejected: `Card.svelte`, `Side.svelte` and the rest import the global `app` store and its card cache (web/src/components/Card.svelte:4, web/src/components/Side.svelte:4). That store is fed by the board's WebSocket (web/src/lib/ws.js:48) and pointer and drag machinery (web/src/lib/pointer.svelte.js). Feeding them a scripted game would couple the site to board internals that change often. The look is reused instead, through the board's tokens and its no-art card style.
- **VitePress.** Rejected: it ships a Vue single-page runtime and a docs theme that looks nothing like the board. It is more than "the lightest generator that fits".
- **Hugo (Go).** Rejected: it adds a second build tool that neither the repo nor its CI has. Node is already present for the board (web/package.json).
- **Hand-rolled Node script (marked + templates + Vite).** Rejected: layouts, asset hashing, base paths, a Markdown pipeline and per-page builds would all need writing and testing.
- **Eleventy.** A credible runner-up: zero client JS and good Markdown. Rejected because the animation and copy controls would need a separate bundling step. Astro bundles the one interactive island and ships nothing else.
- **Commit the anonymised example game as a hand-edited file only.** Rejected as the sole safeguard: a hand edit can drop a line unnoticed. A Go test imports the site's copy and compares it with the original corpus game.

## Chosen approach — evidence

- **Served by the playmat (user decision, 2026-11-06).** The hosted mux (cmd/tcgreplay/hosted.go:179-191) sends `/` to `a.RequireSession(reg.Board())`. The board's server (internal/webui/server.go:94-96) serves `GET /ws`, `GET /api/card` and the embedded files at `/`, and the built board's files are `index.html` plus `assets/` (internal/webui/dist/app). The board's JavaScript uses absolute `/ws`, `/api/card`, `/auth/session` and `/auth/login?next=<path>` (web/src/lib/ws.js:13, store.svelte.js:144, session.js:12,31). Vite base `/` gives `/assets/...` URLs. So the board page can move to `/board` while `/assets/`, `/ws` and `/api/` stay where they are behind the session. The site gets every other path, with its build assets in Astro's `/_astro/`, which doesn't collide. These hard-code `/` as the board and must change to `/board`: the board link `Path: "/"` (internal/usergames/usergames.go:338); the default `next` (internal/hostauth/board.go:23); the signed-out page link `/auth/login?next=/` (board.go:143); "Open your own board" (internal/usergames/board.go:124); "Back to the board" (internal/cacheadmin/page.go:47; internal/hostauth/admin.go:47); and the board page-load check `/` or `/index.html` (internal/usergames/board.go:78), which still sees `/` if `/board` is rewritten to `/` before the board handler. The site is embedded like the board: a `//go:embed all:dist` package with a checked-in placeholder so `go build` works without Node.
- **Astro static site in a new top-level `site/` directory**, with its own package.json so the board's build (web/, Makefile `web` target) is untouched. Astro (7.4.7 current; @astrojs/svelte 9.1.2) builds Markdown pages to plain HTML and ships JavaScript only for components marked as islands. Its content loader can read files outside the project, so the workflow list can be read straight from `internal/guides/*.md`.
- **The playmat's own sources hold the facts to quote:**
  - Workflow names and their one-line "use when" text are in each guide's frontmatter (internal/guides/*.md lines 1-4). The canonical order is `order` in internal/guides/guides.go:17: play-game, import-game, opponent-deck, review-game, replay-drill, before-you-move, numbers-drill.
  - Service limits are the App Platform settings: TCGREPLAY_RATE_LIMIT "300" requests per minute per account, TCGREPLAY_MAX_USERS "10" places, TCGREPLAY_IDLE_TIMEOUT "60m" (.do/app.yaml, the envs under the playmat service). Reading them from .do/app.yaml at build time keeps the site matched to what is deployed.
- **Board look without card art.** Tokens are in web/src/app.css:1-27: felt gradient `--felt`, `--bg` #17321f, `--accent` #e3b341, `--p1` #4fc3f7, `--p2` #ef5350, `--card-bg`, and a light variant. The card's no-art rendering is in web/src/components/Card.svelte:73-88: `.back-art`, the `.token` energy disc and the `.text-card` with name, HP and stage. The scene can import app.css's tokens and copy the small no-art card rules.
- **Example game.** testdata/logs/2026-10-11_gatorusoju_mewex-bronzong-lopunny.txt is 266 lines and names the opponent "GatoruSoju" on 98 of them. The owner is "NicJackson". The game ends "Opponent took all of their Prize cards. GatoruSoju wins." The decklist is testdata/decks/mega-lucario-ex-v12.txt. `TestImportCorpus` (internal/game/import_test.go:18-48) imports every corpus log with that deck and fails on any problem or invariant warning, which is the pattern for the example's own test. The importer is deterministic for a given log, deck, opponent list, owner name and seed (knowledge architecture/importer-determinism.md). `import_log` detects the owner's name from the log when `me` is omitted (internal/mcpserver/server.go:304).
- **Testing.** Playwright 1.64.1 is already a dev dependency (web/package.json). The board's suite uses Chromium-only profiles because WebKit doesn't run on the developer's Arch machine (knowledge gotchas/playwright-touch-tests.md; web/playwright.config.js). The site suite follows the same rule and adds a 375 px phone and a 1366×768 laptop profile.
- **Publishing.** Deploys are App Platform's `deploy_on_push` from main, building the Dockerfile (.do/app.yaml header). The Dockerfile's `web` stage (node:22) builds the board into the Go package before the Go build. A sibling `site` stage does the same for the site, so a merge publishes it with no new CI. The repo has no GitHub Actions (`.github/` absent).

- **Privacy facts as implemented (from a code read on 2026-11-06; the privacy page quotes these):**
  - Google scopes are only `openid` and `email` (internal/hostauth/hostauth.go:134). The playmat keeps the account id (Google `sub`), the email and the email-verified flag (internal/hostauth/google.go:189). It never requests or stores a name or picture. Google's access token is discarded after sign-in (google.go:120-128).
  - Sign-ins live in memory, and tokens and cookies are encrypted with AES-GCM (internal/hostauth/seal.go). Lifetimes: board session 30 days; connector refresh token 30 days, rotated; access token 1 h; sign-in in progress 10 min (store.go:13-17; oauth.go:25-27).
  - Exactly two cookies, both HttpOnly: `tcgreplay_login_<state>` (10 min, while signing in) and `tcgreplay_session` (30 days). The board keeps four UI preferences in localStorage: replaySpeed, autoplay, panelAt, prizeReminders.
  - Games, records, coaching notes and the assumed opponent list are held in memory only (internal/game/manager.go:21-23; internal/store/store.go:1-4). They are dropped after the idle timeout (60m), when the last sign-in ends, after 24 h without activity for an account holding no place, and on every deploy or restart.
  - Logs go to the platform's stderr collection. The email is logged on sign-in, sign-out, rate limiting, busy refusals, expiry and purge, and admin clears. Logs contain no IP addresses, user agents, battle logs or decks. DigitalOcean and Cloudflare may keep their own request logs.
  - Third parties: Google (sign-in), the TCGdex API (card data, no user data), card images loaded by the browser from the Spaces CDN or assets.tcgdex.net.
  - Limits as users meet them: "too many requests: this account is limited to 300 requests a minute; try again in Ns" (internal/hostauth/limit.go:17). When full: "tcgreplay is busy" page / "the server is busy: it already has the most signed-in users it serves at once (10)" (limit.go:21-28; pages.go:49-55). Expiry: "Your game expired after 60 minutes without activity and was cleared. Import its saved game record again (import_log with the record) to carry on." (internal/usergames/usergames.go:198-201).
- **Sign-in and board link as users meet them:** connecting goes straight to Google's account chooser, with no tcgreplay page in between (oauth.go:258-298). The callback also sets the board cookie, so opening the board link in the same browser needs no second sign-in; another browser signs in with Google (google.go:210-214). The board link `<public>/?board=<handle>` always shows the signed-in account's own game; another account's handle gets "Not your game" (internal/usergames/board.go:32-36). The board's menu shows the email and Sign out.
- **Desktop:** `make install` (Go 1.27 per go.mod, plus Node), add the binary path to `claude_desktop_config.json`, restart the Claude desktop app. The board is at http://127.0.0.1:7777, with no sign-in and one game at a time (cmd/tcgreplay/main.go:133). There is no packaged installer, and it is Unix-socket based, so Windows is unverified.
- **"What can you do" check:** `ping` returns `pong`. `guide` with no name lists the workflows. The server's instructions list every guide (internal/mcpserver/guides.go:55-64). A good first prompt is "What can the tcgreplay playmat do?"

## Files examined

- `.do/app.yaml` — the deploy model (push to main deploys; one instance; games in memory, lost on deploy) and the limits: RATE_LIMIT 300, MAX_USERS 10, IDLE_TIMEOUT 60m.
- `Dockerfile` — the image build copies the whole repo (`COPY . .`), so `site/node_modules` and `site/dist` must join `.dockerignore`.
- `.dockerignore` — current exclusions. `internal/guides` must never be excluded (knowledge conventions/skills-text-and-packaging.md).
- `Makefile` — targets build, install, web, test, dev, dev-ui, docker-*. `test` runs Go and then the board's e2e. A `site` target belongs beside them.
- `README.md` — today's user instructions to move to the site: hosted connect steps for claude.ai, Claude Code and ChatGPT; desktop install; example prompts; workflow list. Its ChatGPT caveat ("guides were written with Claude in mind") is out of date: the user reports ChatGPT works as well or better.
- `internal/guides/guides.go:17` — the workflow order. `internal/guides/*.md` — frontmatter `name`/`description`.
- `internal/mcpserver/server.go:79,301-306,483,507` — the board-link instruction, the import_log parameters (log, deck, opp_deck, me), and the record and get_record wording ("give it to the player to keep").
- `internal/game/import_test.go:18-48` — the corpus import test pattern.
- `testdata/README.md`, `testdata/logs/2026-10-11_gatorusoju_mewex-bronzong-lopunny.txt`, `testdata/decks/mega-lucario-ex-v12.txt` — the example game and deck.
- `web/src/app.css:1-45` — board tokens and the light theme.
- `web/src/components/Card.svelte:4,73-140` — no-art card rendering; coupling to the global store.
- `web/vite.config.js`, `web/playwright.config.js`, `web/package.json` — toolchain versions (Vite 8.3.1, Svelte 5.57.1, Playwright 1.64.1) and the test conventions.
- `docs/logo.png` (512 px, transparent corners), `docs/icon.png` (256 px) — the site's logo and favicon source.
- `TCG Review Playmat Design and Build Plan.md:331` — the consent screen is External, scopes openid and email, published "In production" without verification.

## External references

- discuss.google.dev, "OAuth verification stuck on 'Homepage requirements'" (https://discuss.google.dev/t/oauth-verification-stuck-on-homepage-requirements-firebase-web-app-domain-not-recognized/333105) — Google rejected a github.io home page and privacy policy as unverifiable third-party hosting. This is why the site uses a demo.gs subdomain.
- Google, OAuth client verification (https://developers.google.com/apps-script/guides/client-verification) — the home page and privacy policy must be on authorized domains the owner has verified.
- DigitalOcean App Platform app-spec reference (https://docs.digitalocean.com/products/app-platform/reference/app-spec/) — `deploy_on_push` is a plain boolean with no path filter, so any push to main redeploys the playmat.
- Astro docs (docs.astro.build) — static output, `site`/`base` config, the Svelte integration, and content collections with a glob loader whose base can be outside `src/`.

## Prior plans / specs consulted

- The spec 20261106110254-f08e142d-project-website — scope, constraints, the default host and the example game.
- The hosted-playmat epic and Google sign-in work, through the design doc's operator steps (line 331) and knowledge learnings/claude-ai-connector-oauth.md — claude.ai and Claude Code callbacks; sign-in via DCR with PKCE.

## Open assumptions

- The OAuth redirect refusal text still says "redirect_uri must be Claude's callback or a loopback address" (internal/hostauth/oauth.go:201), although ChatGPT is allowed too. This is a playmat wording fix, out of scope here (Non-Goals); report it to the user.
- README.md:39 says games drop "after an hour"; the service says "60 minutes". The site states the limit from .do/app.yaml.
- Desktop on Windows is unverified (Unix socket daemon). The site lists macOS and Linux as tested and says Windows is untested.

- `demo.gs` is on the Google project's authorized domains list (implied by the working redirect URI on tcgreplay.demo.gs). If it isn't, the operator adds it when entering the home page and privacy addresses.
- Any push to main redeploys the playmat (App Platform, `deploy_on_push`), so every docs merge restarts it and drops in-memory games. This is today's behaviour for every commit and is not changed here.
- The anonymised example (opponent name replaced with a placeholder) imports with the same outcome as the original. A test proves it before the page is published.

## Rehydration cues

- `spektacular spec file read 20261106110254-f08e142d-project-website` — the spec.
- `spektacular knowledge read` of repo/tcgreplay: conventions/skills-text-and-packaging.md, gotchas/playwright-touch-tests.md, architecture/importer-determinism.md, learnings/claude-ai-connector-oauth.md.
- Re-read: .do/app.yaml (limits and deploy), README.md (today's user docs), internal/guides/guides.go and *.md frontmatter, web/src/app.css and web/src/components/Card.svelte (look), internal/game/import_test.go (test pattern), internal/hostauth/{google,store,limit,pages,oauth}.go and internal/usergames/usergames.go (privacy and limit wording).
