---
created_date: "2026-11-12"
document_status: final
closed_date: "2026-11-12"
---

# Research: 20261111093353-8ff3589f-domain-move

## Alternatives considered and rejected

### Option A: Teach the server two public addresses (sign-in, metadata and board links per request Host)

**Rejected**: the spec's Technical Approach picks one canonical address, the Constraints forbid demo.gs becoming a second sign-in origin, and the design doc says "the request's `Host` is never trusted" for addresses (`TCG Review Playmat Design and Build Plan.md:305`). `cmd/tcgreplay/hosted.go:52-76` already derives everything from one `TCGREPLAY_PUBLIC_URL`.

### Option B: Leave the live Cloudflare redirect forwarding `/mcp` too (status quo on 2026-11-12)

**Rejected**: `curl https://tcgreplay.demo.gs/mcp` gets `301 → https://tcgreplay.app/mcp` from Cloudflare. A cross-host redirect drops the `Authorization` header (knowledge `learnings/publishing-to-claude.md`, "Network pitfalls"), and tcgreplay.app's protected-resource metadata names `resource: https://tcgreplay.app/mcp`, which no longer equals the URL the user entered (knowledge `learnings/claude-ai-connector-oauth.md`: resource must equal the MCP URL exactly). The player sees a generic connector failure, failing the "moved message" criterion.

### Option C: Answer the old `/mcp` with a 401 or an OAuth error carrying a description

**Rejected**: a 401 sends claude.ai into discovery and sign-in at the old address (knowledge `learnings/claude-ai-connector-oauth.md`: `401 invalid_client` makes claude.ai register again and re-run sign-in), which is exactly the confusing re-sign-in the spec warns about, and the error text is not shown to the player.

### Option D: A single `moved` tool at the old `/mcp`

**Rejected**: in favour of mirroring the real tool list: ChatGPT keeps the tool list it saw when the connector was made until a Refresh (knowledge `learnings/publishing-to-chatgpt.md`: "retest after every Refresh"; changed tools keep the old definition), so an existing connector calls the real tool names. Mirroring means whatever tool the assistant calls, it gets the message.

### Option E: A Cloudflare Worker at `tcgreplay.demo.gs/mcp` answering the MCP protocol

**Rejected**: it is still code, but outside the repo and its tests, hand-rolling Streamable HTTP and JSON-RPC that the Go server already gets from go-sdk. The spec's preference for the hosting/Cloudflare layer is about removability by configuration, which the server answer also has (it only fires for hosts named in configuration, and stops when the alias and DNS go).

### Option F: Treat every Host other than the public one as "moved"

**Rejected**: tests serve on `127.0.0.1:<port>` with a different configured public URL (`cmd/tcgreplay/hosted_test.go:646-669`), App Platform's health check and default `*.ondigitalocean.app` name would be caught, and local `make docker-run` would redirect. An explicit list of old hosts is safer and documents itself.

### Option G: App Platform ingress redirect rules for the page redirects

**Rejected**: Not needed: the page redirects already work at Cloudflare (verified live), which is the spec's preferred layer, and App Platform's authority matching on ingress rules is not verified for this app spec. The server keeps a redirect for old-host page requests only as a guard so demo.gs can never serve sign-in at the origin if the Cloudflare rule drifts.

### Option H: Hard-code `https://tcgreplay.app` in `site/src/lib/facts.ts` as today

**Rejected**: in favour of reading `TCGREPLAY_PUBLIC_URL` from `.do/app.yaml`, the file `facts.ts` already reads the limits from (`site/src/lib/facts.ts:13`, "this file stays the one place they are set" in `.do/app.yaml` header). This makes the website's address and the server's address one setting.

## Chosen approach — evidence

- Live state, checked 2026-11-12 with curl and Cloudflare DoH:
  - `tcgreplay.app` and `demo.gs` both use Cloudflare nameservers (`sandy`/`terin.ns.cloudflare.com`); both resolve to Cloudflare proxy IPs (records proxied).
  - `https://tcgreplay.app/` is 200 with `x-do-app-origin` (App Platform behind Cloudflare); edge certificate `CN=tcgreplay.app`, issuer Google Trust Services WE1, valid to 2027-02-09.
  - `https://tcgreplay.app/mcp` is 401 with `resource_metadata="https://tcgreplay.app/.well-known/oauth-protected-resource/mcp"`; the metadata names `resource: https://tcgreplay.app/mcp`, issuer `https://tcgreplay.app`. So `TCGREPLAY_PUBLIC_URL=https://tcgreplay.app` is already deployed.
  - `http://tcgreplay.app/docs/connect/` → 301 to https. `www.tcgreplay.app` does not resolve.
  - `https://tcgreplay.demo.gs/<any path>` (including `/mcp`, `/board?x=1`, `/docs/connect/`) → Cloudflare 301 to the same path and query on tcgreplay.app; `http://tcgreplay.demo.gs/docs/` → https on demo.gs, then tcgreplay.app.
  - The live website still names `https://tcgreplay.demo.gs/mcp` on `/docs/connect/` (site source not yet changed).
- `cmd/tcgreplay/hosted.go:52-76` — `hostedPublicURL` validates one scheme+host public URL; `newHosted` (`:295-328`) feeds it to the registry, hostauth, board origin and `BoardURL`. Switching the address is configuration only.
- `cmd/tcgreplay/hosted.go:183-209` — `hostedHandler` is the single mux; wrapping it is the natural seam for an old-host guard.
- `internal/mcpserver/server.go:43-63` — `MCP()` builds a `*mcp.Server` with all tools and the guide prompts (`internal/mcpserver/guides.go:47`); a second instance with receiving middleware that intercepts `initialize`, `tools/call` and `prompts/get` gives a mirrored "moved" server with no new tool definitions. `internal/hostauth/limit.go:95` shows the receiving-middleware pattern already used.
- `.do/app.yaml` (working tree, uncommitted maintainer edit): `TCGREPLAY_PUBLIC_URL: https://tcgreplay.app`, domains `tcgreplay.app` PRIMARY and `tcgreplay.demo.gs` ALIAS, with comments. `.github/workflows/deploy.yml:36` only runs `doctl apps create-deployment`, so app-spec edits reach App Platform only through `doctl apps update --spec .do/app.yaml` (header of `.do/app.yaml`).
- `site/src/lib/facts.ts:13-22` already parses `.do/app.yaml` for the limits; `site/e2e/facts.spec.js` tests that parsing with a fixture.

## Files examined

- `cmd/tcgreplay/hosted.go:48-76` — public URL parsing and validation; `:183-209` hosted mux; `:295-328` everything derives from `public`; `:347-394` runHosted.
- `cmd/tcgreplay/main.go:60,163` — `-public-url` flag defaulting to `$TCGREPLAY_PUBLIC_URL`; new flags follow this shape.
- `cmd/tcgreplay/hosted_test.go:65,165,646-669,740` — test helpers `startHosted`/`startHostedWith(t, public, mod)`, `mcpPost`, `TestHostedBoardLinks`.
- `cmd/tcgreplay/hosted_routes_test.go:28` — `TestHostedRoutes`, the path table test to extend for old hosts.
- `internal/mcpserver/server.go:17-63,86-92` — Server fields, `MCP()`, instructions.
- `internal/mcpserver/guides.go:47` — guides are registered as prompts too.
- `internal/hostauth/hostauth.go:141-146` — `Routes`: Google callback, `/healthz`, OAuth, board routes.
- `internal/hostauth/oauth.go:433-444` — `RequireToken` builds the resource-metadata URL from the public address.
- `internal/hostauth/limit.go:95` — receiving middleware pattern.
- `internal/webui/server.go:147-161` — `localOnly` Host check (desktop only), a model for host normalisation (strip port, trim brackets).
- `.do/app.yaml` — env, ingress, domains; uncommitted maintainer edits already make tcgreplay.app primary and demo.gs an alias.
- `.github/workflows/deploy.yml` — deploys without applying the app spec.
- `site/astro.config.mjs:7` — `site: 'https://tcgreplay.demo.gs'` (not read by any page; no `Astro.site` use).
- `site/src/lib/facts.ts:15-22` — hard-coded `publicURL`, `mcpURL`, `boardURL`.
- `site/src/content/docs/connect.md:15,30,47`, `sign-in.md:15`, `site/src/pages/docs/limits.astro:16` — hard-coded old address in prose.
- `site/src/pages/index.astro:50`, `site/src/pages/docs/[...slug].astro:17`, `site/src/pages/privacy.astro:19` — already use `site.*` facts.
- `site/e2e/connect.spec.js:45,52,60,69,81`, `site/e2e/site.spec.js:255` — assert the old address; `site/e2e/facts.spec.js:15` fixture uses `${APP_URL}`; `site/e2e/site.spec.js:81` reads the built dist (pattern for a whole-site scan).
- `README.md:16-20` — old address links.
- `TCG Review Playmat Design and Build Plan.md:305,307` — design doc names the old address, says Host is never trusted, and has the consent-screen operator step naming demo.gs.
- `Makefile:76-95`, `Dockerfile:5` — public URL docs for local runs; no old address.

## External references

- MCP authorization (RFC 9728 protected resource metadata, `resource` must equal the entered URL) — via knowledge `learnings/claude-ai-connector-oauth.md` and `learnings/publishing-to-claude.md`; why the old `/mcp` cannot just redirect.
- OpenAI Apps SDK auth and testing notes — via knowledge `learnings/publishing-to-chatgpt.md`; tool lists persist until Refresh, so mirror tool names.
- Cloudflare Redirect Rules / Single Redirects (dashboard: Rules > Redirect Rules) — the layer already forwarding demo.gs; the rule needs `/mcp` excluded.
- DigitalOcean App Platform domains (Settings > Domains; `doctl apps update --spec`) — how the alias and env reach the platform.

## Prior plans / specs consulted

- Epic `20261111093348-e1b5c2b4-public-launch` — domain move has no dependencies; `chatgpt-submission-prep` and `launch-docs` depend on it; `observability` runs in parallel and may touch `cmd/tcgreplay/hosted.go` and `.do/app.yaml`.
- Plan list shows `20261017123240-21493a95-google-login`, `20261017105035-e3f64c1b-web-mcp-docker`, `20261106110254-f08e142d-project-website` as the history behind hosted mode, sign-in and the site; not needed to re-derive current behaviour (read from code).

## Open assumptions

- App Platform passes the original `Host` (`tcgreplay.demo.gs`) through to the container for requests on an alias domain proxied by Cloudflare. If the post-deploy check shows the moved answer does not fire at `https://tcgreplay.demo.gs/mcp`, STOP and ask (fallback candidates: `X-Forwarded-Host`, or a Cloudflare origin rule).
- claude.ai and ChatGPT, with a connector already on `https://tcgreplay.demo.gs/mcp`, accept an unauthenticated 200 MCP session there and show a tool result's error text to the player. Checked by a person after deploy; if either shows only a generic failure, STOP and ask.
- The Google OAuth client already lists `https://tcgreplay.app/auth/google/callback` (sign-in metadata at tcgreplay.app is live). The console steps still state and check it.
- The maintainer's uncommitted `.do/app.yaml` edits reflect the intended app spec; implementation in a worktree will not see them, so the plan restates their content.

## Drafting assumptions

### Chosen direction: one public address, Cloudflare page redirects, playmat answers the old /mcp (architecture)
- **Decision**: `TCGREPLAY_PUBLIC_URL=https://tcgreplay.app` stays the only working address; the website reads it from `.do/app.yaml`; Cloudflare forwards old-address pages except `/mcp`; the playmat, for hosts in `TCGREPLAY_MOVED_FROM`, serves an unauthenticated mirrored MCP server at `/mcp` whose every tool call, prompt and instructions say "moved, reconnect at https://tcgreplay.app/mcp", and permanently redirects every other path.
- **Key design decisions**: explicit old-host list rather than "any other Host"; mirror the real tool list rather than one `moved` tool (ChatGPT keeps cached tool names); no auth on the moved server so no sign-in starts at the old address; the moved text never echoes the request host; startup refuses a moved host equal to the public host.
- **Rationale**: the only answer that both surfaces a message in the assistant and keeps the old address removable by configuration alone.
- **Rejected**: two-address server, Cloudflare Worker, 401, single moved tool, catch-all hosts (see research.md alternatives).

### Server-side page redirect for moved hosts is a guard, not a second redirect layer (architecture)
- **Decision**: keep a server redirect for non-`/mcp` paths on moved hosts although Cloudflare does the page redirects.
- **Rationale**: routing old-address `/mcp` to the origin means the App Platform alias is attached; without the guard, any drift in the Cloudflare rule would serve sign-in and the board at demo.gs, breaking the "no second sign-in origin" constraint. The spec's "fall back to the server only where the platform can't express it" is met: Cloudflare cannot make the origin refuse the alias.
- **Rejected**: Cloudflare-only page redirects.

### Conventions: none apply (architecture)
- **Decision**: no conventions selected; the guide text/packaging convention is dropped as its files are untouched.
- **Rationale**: the moved message lives in `internal/mcpserver`, not a guide.
- **Rejected**: listing it anyway.

### Moved answer for the old /mcp is served by the playmat (discovery)
- **Decision**: Cloudflare keeps forwarding old-address pages, but excludes `/mcp`; the playmat answers `/mcp` on hosts listed in a new `TCGREPLAY_MOVED_FROM` setting with an unauthenticated MCP session that mirrors the real tools and returns a "moved, reconnect at https://tcgreplay.app/mcp" error from every tool call, prompt and the instructions.
- **Rationale**: a redirect drops the token and fails the resource match (generic failure); a 401 triggers a re-sign-in at the old host. An explicit host list keeps it removable by configuration.
- **Rejected**: Cloudflare Worker (out-of-repo protocol code), 401 with description, single `moved` tool, catch-all on any non-public Host.

### Server also redirects old-host page requests (discovery)
- **Decision**: for a moved host, every path other than `/mcp` gets a permanent redirect (301 for GET/HEAD, 308 otherwise) to the same path and query on the public URL.
- **Rationale**: guards the constraint that the old address never becomes a sign-in origin, even if the Cloudflare rule is changed while the App Platform alias remains. Cloudflare remains the primary redirect layer.
- **Rejected**: relying on Cloudflare alone.

### Treat the live console work as already partly done (discovery)
- **Decision**: the console steps are written as a runbook with a check per step, marked as steps to confirm where the live probe shows them done (tcgreplay.app DNS, certificate, primary domain, public URL), plus the new steps (Cloudflare rule excludes `/mcp`, `doctl apps update`, Google consent screen URLs, removing the old Google callback).
- **Rationale**: live probes on 2026-11-12 show tcgreplay.app serving with the new public URL and demo.gs forwarding at Cloudflare.
- **Rejected**: planning the console work from scratch.

### Runbook lives in docs/domain-move.md (components)
- **Decision**: the maintainer's console steps are a new repo doc, `docs/domain-move.md`, linked from the `.do/app.yaml` header and the design doc.
- **Rationale**: the spec requires written, ordered steps; `docs/` exists (holds images today) and is not part of the public website; the app.yaml header is too small for a runbook.
- **Rejected**: a section in the design doc (already very dense), a page on the public website (maintainer-only content).

### Move note only on the connecting page (components)
- **Decision**: the website's single allowed mention of the old address is a marked note on `/docs/connect` (and nowhere in the README).
- **Rationale**: that is where a player with a broken connector looks; one marked element makes the "no stray old address" check mechanical.
- **Rejected**: a home-page banner (the launch-docs spec owns wider docs changes), a README note.

### The human task confirms rather than redoes live console work (tasks)
- **Decision**: one `human` task follows the runbook end to end; where live probes show a step already done (tcgreplay.app DNS, certificate, public URL, page redirects), the person confirms it with the step's check.
- **Rationale**: the maintainer has already moved the live address; the remaining console work is the `/mcp` exclusion in the Cloudflare rule, `doctl apps update` for the moved setting, consent-screen URLs and removing the old Google callback.
- **Rejected**: separate human tasks per console (more tasks for one sitting of console work).

### app.yaml edits restated in the first task (tasks)
- **Decision**: the first agent task writes the public URL, domains and comments into `.do/app.yaml` itself (or confirms them if already committed), and the human task asks the maintainer to commit or discard the uncommitted main-tree edit before implementation.
- **Rationale**: implementation runs in a worktree that does not carry uncommitted edits; leaving it implicit would lose or conflict with them.
- **Rejected**: assuming the edit is committed.

### Site task titles avoid punctuation (tasks)
- **Decision**: the docs task is titled "Site pages and README name the new address" so its context anchor is unambiguous.
- **Rationale**: anchors are derived from titles; a comma and a dot make the slug ambiguous.
- **Rejected**: keeping "tcgreplay.app" in the title.

## Rehydration cues

- `spektacular spec file read 20261111093353-8ff3589f-domain-move`
- `spektacular knowledge read` for `learnings/claude-ai-connector-oauth.md`, `learnings/publishing-to-claude.md` (Network pitfalls), `learnings/publishing-to-chatgpt.md` (auth, testing), `gotchas/go-sdk-sessions-and-auth.md`, `gotchas/astro-site-build.md`.
- Re-read `cmd/tcgreplay/hosted.go`, `internal/mcpserver/server.go`, `.do/app.yaml`, `site/src/lib/facts.ts`, `site/e2e/connect.spec.js`.
- Live probes: `curl -sI https://tcgreplay.demo.gs/mcp`, `curl -s https://tcgreplay.app/.well-known/oauth-protected-resource/mcp`.
