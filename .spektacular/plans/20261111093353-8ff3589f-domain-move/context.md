---
created_date: "2026-11-12"
document_status: final
closed_date: "2026-11-12"
---

# Context: 20261111093353-8ff3589f-domain-move

## Current State Analysis

- **Live (probed 2026-11-12):** `https://tcgreplay.app` already serves the playmat through Cloudflare's proxy in front of App Platform. Its edge certificate is `CN=tcgreplay.app` (Google Trust Services), valid to 2027-02-09. Its OAuth metadata names `https://tcgreplay.app` as issuer and `https://tcgreplay.app/mcp` as resource, so `TCGREPLAY_PUBLIC_URL=https://tcgreplay.app` is deployed. `http://` redirects to https, and `www.tcgreplay.app` does not resolve. A Cloudflare redirect on the demo.gs zone sends every `tcgreplay.demo.gs` path, `/mcp` included, with a 301 to the same path and query on tcgreplay.app. The live website still shows `https://tcgreplay.demo.gs/mcp`.
- **Server:** `cmd/tcgreplay/hosted.go:52-76` (`hostedPublicURL`) is the one public address. `:188-215` (`hostedHandler`) is the single mux and never reads the request host. `:247-275` (`newHosted`) feeds the address to the registry, hostauth, board origin and `BoardURL`. `:348-394` (`runHosted`) builds the `http.Server` at `:384`. Flags are in `cmd/tcgreplay/main.go:60,163`.
- **MCP server:** `internal/mcpserver/server.go:43-63` builds the tools and the guide prompts (`internal/mcpserver/guides.go:47`). Receiving-middleware precedent: `internal/hostauth/limit.go:95`.
- **Deployment:** `.do/app.yaml` has uncommitted maintainer edits in the main tree: `TCGREPLAY_PUBLIC_URL: https://tcgreplay.app`, tcgreplay.app PRIMARY, tcgreplay.demo.gs ALIAS. `.github/workflows/deploy.yml` only runs `doctl apps create-deployment`, so app-spec edits need `doctl apps update --spec`.
- **Website:** `site/src/lib/facts.ts:15` hard-codes `https://tcgreplay.demo.gs`. `site/astro.config.mjs:7` has the same value. The old address also appears in `site/src/content/docs/connect.md:15,30,47`, `sign-in.md:15`, `site/src/pages/docs/limits.astro:16` and `README.md:16-20`, and is asserted in `site/e2e/connect.spec.js:45,52,60,69,81` and `site/e2e/site.spec.js:255`. The design doc `TCG Review Playmat Design and Build Plan.md:305,307` names it too.

## Per-Task Technical Notes

### Task: Website reads the public address from the deployment definition

Requirement → repo: "One canonical address" (website side) → `tcgreplay` (root `/home/nicj/code/github.com/nicholasjackson/tcgreplay`).

**File changes**
- `.do/app.yaml:58-67` — make the playmat service's env match the maintainer's uncommitted edit, which a worktree will not see. `TCGREPLAY_PUBLIC_URL` gets `value: https://tcgreplay.app`, not `${APP_URL}`. The comment reads: "The playmat's one public address: sign-in, the assistant connection, board links and the board's allowed origin all use it. Keep it the same as the primary domain under domains below." The Google client comment says its authorised redirect URIs include `TCGREPLAY_PUBLIC_URL` + `/auth/google/callback`. If the main tree's edit has already been committed when this runs, just confirm the content.
- `.do/app.yaml:141-143` (`domains:`) — `tcgreplay.app` `PRIMARY`, then `tcgreplay.demo.gs` `ALIAS`, with the comment block above `domains:` from the maintainer's edit. It explains that the primary domain is the public address and that the alias exists only to forward players and answer old connections, with no sign-in there.
- `site/src/lib/facts.ts:15-22` — drop the literal `publicURL`. Add exported `loadPublicURL(yamlText, source)`, which parses the YAML and finds the `playmat` service (reuse the lookup in `loadLimits`). It reads the `TCGREPLAY_PUBLIC_URL` env value and requires `https:` with a host and no path (other than none), query, fragment or trailing slash. It also requires a top-level `domains` entry with `type: PRIMARY` whose `domain` equals the URL's host. Otherwise it throws `FactError` naming the problem; a value containing `${` gets "is a platform placeholder; set the literal address". Build `site` from it: `publicURL`, `mcpURL`, `boardURL`, `repoURL` unchanged in shape. Read the file with the same `appSpecPath` and `readFileSync` the limits use.
- `site/astro.config.mjs:7` — `site: 'https://tcgreplay.app'`. Nothing reads `Astro.site` today, so a literal is fine. Add a comment that it must equal `TCGREPLAY_PUBLIC_URL`; the facts check guards the pages.
- `site/e2e/facts.spec.js:11-25` — the fixture gains `TCGREPLAY_PUBLIC_URL: https://tcgreplay.app` (replacing `${APP_URL}`) and a `domains:` block. New tests:
  - the real `.do/app.yaml` yields `https://tcgreplay.app`, and `site.mcpURL` is `https://tcgreplay.app/mcp`;
  - a missing value throws naming `TCGREPLAY_PUBLIC_URL`;
  - `${APP_URL}` throws "placeholder";
  - a path or trailing slash throws;
  - an `http:` URL throws;
  - a `PRIMARY` domain that differs throws naming both.
- `site/src/pages/privacy.astro:19`, `site/src/pages/index.astro:50`, `site/src/pages/docs/[...slug].astro:17` — no change; they already print `site.*`.

**Complexity**: Low
**Token estimate**: ~25k tokens
**Agent strategy**: Single agent, sequential. Run `make site-test` (site build plus Playwright), or at least `cd site && npm run build && npx playwright test e2e/facts.spec.js`. The connecting tests still assert the old literal until the next task, so expect those to fail here, or update the two literals in `site/e2e/site.spec.js:255` and `site/e2e/connect.spec.js` to `site.mcpURL` now if you want a green run. The next task owns the full rewrite.

### Task: Site pages and README name the new address

Requirements → repo: "One canonical address" (docs), "No stray old address", "Secure, with no www name" (docs side) → `tcgreplay`.

**File changes**
- `site/src/content/docs/connect.md:15,30,47` — replace `https://tcgreplay.demo.gs/mcp` with `https://tcgreplay.app/mcp` in the claude.ai step, the ChatGPT step and the `claude mcp add` command. After the intro paragraphs, before `## claude.ai`, add one raw-HTML note, `<aside class="note" data-moved-note>…</aside>`: "The playmat used to be at `tcgreplay.demo.gs`. If your connector still uses `https://tcgreplay.demo.gs/mcp`, remove it and add `https://tcgreplay.app/mcp` instead, then sign in again." Check that Astro's Markdown renders the inner Markdown, or write it as inline HTML with `<code>`. Style `.note` in the existing site CSS only if no note style exists. Keep it plain.
- `site/src/content/docs/sign-in.md:15` — example board link to `https://tcgreplay.app/board?board=…`.
- `site/src/pages/docs/limits.astro:16` — "The hosted playmat at tcgreplay.demo.gs" becomes `{site.publicURL.replace('https://', '')}` (import `site` from `../../lib/facts` if not already imported), so it follows the fact.
- `README.md:16-20` — the three links become `https://tcgreplay.app`, `https://tcgreplay.app/docs/connect` and `https://tcgreplay.app/docs/desktop`, and the link text `tcgreplay.app`.
- `site/e2e/connect.spec.js:45,52,60,69,81` — import `site` from `../src/lib/facts.ts`. Assert `s.text` contains `site.mcpURL`; the copy test expects `site.mcpURL`; the command test expects `` `claude mcp add --transport http tcgreplay ${site.mcpURL}` ``; the sign-in test expects `` `${site.boardURL}?board=` ``. Rename the test titles so they no longer quote the old address. Add a test that the connecting page has exactly one `[data-moved-note]` element and that it contains `site.mcpURL`.
- `site/e2e/site.spec.js:248-256` — the home copy test expects `site.mcpURL`. Add a test, "no page or the README names the old address outside the move note, or a www address". It walks `internal/site/dist/app/**/*.html` (the `dist` URL at `:81`, `readdirSync` with `recursive: true`) plus `../../README.md`. For each file it strips `<aside[^>]*data-moved-note[^>]*>[\s\S]*?</aside>`, then expects no `demo.gs`, and expects no `www.tcgreplay.app` in the unstripped text. Failures name the file.

**Complexity**: Low
**Token estimate**: ~30k tokens
**Agent strategy**: Single agent, sequential. Verify with `make site-test`.

### Task: Moved answer for MCP connections

Requirement → repo: "Old assistant connection explains the move" (server side) → `tcgreplay`.

**File changes**
- `internal/mcpserver/moved.go` (new):
  - `MovedText(mcpURL string) string` returns "The tcgreplay playmat has moved to a new address. This connection no longer works: remove it, then add the playmat again at " + mcpURL + " and sign in." Keep it free of any assistant's brand name, because it is shown in every assistant.
  - `Moved(mcpURL string) mcp.Middleware` returns `func(next mcp.MethodHandler) mcp.MethodHandler`. It switches on `method`:
    - `"initialize"`: call `next`, then set `res.(*mcp.InitializeResult).Instructions = MovedText(mcpURL)` (guard the type assertion) and return.
    - `"tools/call"`: return `&mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: MovedText(mcpURL)}}}, nil` without calling `next`.
    - `"prompts/get"`: return `&mcp.GetPromptResult{Description: "The playmat has moved", Messages: []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: MovedText(mcpURL)}}}}, nil`.
    - Default: `next`.
  
  Model the shape on `internal/hostauth/limit.go:95-120`. Doc comment: used for hosts that have moved (see cmd/tcgreplay's guard). The instance it wraps needs no game manager because no tool handler ever runs.
- `internal/mcpserver/moved_test.go` (new) — use the in-memory transport pattern from `internal/mcpserver/server_test.go:25-60`. Build `(&Server{Version: "test", BoardURL: "https://x/board"}).MCP()`, plus a real one for comparison, and add `Moved("https://tcgreplay.app/mcp")`. Assert:
  - `InitializeResult.Instructions` equals `MovedText(...)`;
  - the tool names from `ListTools` equal the unwrapped server's;
  - the prompt names from `ListPrompts` equal the unwrapped server's;
  - for every listed tool, `CallTool` with empty arguments returns `IsError` and text containing `https://tcgreplay.app/mcp`;
  - `GetPrompt` for each guide returns the message.
  
  A `Server` with nil `M` must not panic, which proves no handler ran.

**Complexity**: Low
**Token estimate**: ~25k tokens
**Agent strategy**: Single agent, sequential. Verify with `go test ./internal/mcpserver/`.

### Task: Moved-host setting and guard in the hosted playmat

Requirements → repo: "Old assistant connection explains the move", "Old pages forward" (origin guard), "Old address can be retired without a release", constraint "old address must not become a second sign-in origin" → `tcgreplay`.

**File changes**
- `cmd/tcgreplay/main.go:60` (config struct) — add `movedFrom string` next to `publicURL`, with a comment.
- `cmd/tcgreplay/main.go:163` — add `flag.StringVar(&c.movedFrom, "moved-from", os.Getenv("TCGREPLAY_MOVED_FROM"), "with -hosted, comma-separated old host names the playmat has moved from: their /mcp tells assistants the new address and every other path redirects to the public URL (default $TCGREPLAY_MOVED_FROM)")`.
- `cmd/tcgreplay/moved.go` (new):
  - `type movedHosts map[string]bool`.
  - `parseMovedHosts(raw string, public *url.URL) (movedHosts, error)` splits on commas, trims and lower-cases each entry, and skips blanks. It rejects an entry containing `/`, `:`, `@` or whitespace ("must be a host name only"), and an entry equal to `public.Hostname()` ("is the public address").
  - `movedGuard(hosts, public, moved, next http.Handler) http.Handler` returns `next` when `hosts` is empty. Otherwise it normalises `r.Host` as `internal/webui/server.go:151-155` does (strip port, lower-case). On a moved host:
    - `r.URL.Path == "/mcp"` goes to `moved`;
    - any other path redirects to `public.String() + r.URL.EscapedPath()`, plus `"?" + r.URL.RawQuery` when there is a query, with 301 for GET and HEAD and 308 otherwise.
  
    Other hosts go to `next`.
  - `movedText(hosts)` produces the start-up log fragment: "moved from a, b" or "no moved hosts".
- `cmd/tcgreplay/hosted.go:247-275` (`newHosted`) — also build the moved MCP handler: `movedSrv := (&mcpserver.Server{ClaudeSeat: state.Seat(c.seat), Version: version, BoardURL: public.String() + "/board"}).MCP()`, then `movedSrv.AddReceivingMiddleware(mcpserver.Moved(public.String() + "/mcp"))`. Wrap it with `mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return movedSrv }, &mcp.StreamableHTTPOptions{SessionTimeout: mcpSessionTimeout, DisableLocalhostProtection: true})`, with no `RequireToken`. Return it, or build it in `runHosted`; either is fine, but keep `newHosted`'s callers in the tests compiling.
- `cmd/tcgreplay/hosted.go:348-357` (`runHosted`) — after `hostedPublicURL`, call `parseMovedHosts(c.movedFrom, public)` and return its error (as for a bad public URL). Add the moved hosts to the start-up log line at `:382`. At `:384`, use `Handler: movedGuard(hosts, public, movedHandler, hostedHandler(srv, reg, a, admin))`.
- `cmd/tcgreplay/hosted_test.go` — add `TestParseMovedHosts` (table: blanks, mixed case, scheme, path, port, the public host). Add `TestRunHostedRejectsBadMovedFrom`, modelled on `TestRunHostedRejectsBadPublicURL` at `:423`. `hostedConfig` at `:261` gets no moved hosts by default.
- `cmd/tcgreplay/hosted_moved_test.go` (new) — use `startHostedWith(t, testPublic, func(c *config){ c.movedFrom = "old.example" })`. Check that `startHostedWith` (`:653`) builds the handler through the same path as `runHosted`; if it calls `hostedHandler` directly, route it through `movedGuard` too, so the test covers the real wiring. Send requests with `r.Host = "old.example"` (and `"OLD.example:443"`):
  - an MCP client over a Streamable transport whose HTTP client sets that Host connects without a token;
  - `ListTools` is non-empty;
  - `CallTool("get_state")` is `IsError` with `testPublic + "/mcp"`;
  - a POST to `/mcp` with `Authorization: Bearer junk` is not 401;
  - `GET /docs/connect/` → 301 to `testPublic + "/docs/connect/"`;
  - `GET /board?x=1` → 301 to `testPublic + "/board?x=1"`;
  - `POST /token` → 308;
  - `GET /authorize?...` → 301 to public;
  - `GET /.well-known/oauth-protected-resource/mcp` → 301;
  - `GET /healthz` on the old host → 301; that is fine, because App Platform's health check does not use the old host.
  
  For an unlisted host (default 127.0.0.1), `/mcp` without a token is still 401, as `checkRefused` at `:305` checks.
- `.do/app.yaml` (envs, after `TCGREPLAY_PUBLIC_URL`) — add `TCGREPLAY_MOVED_FROM`, `value: tcgreplay.demo.gs`, `scope: RUN_TIME`, with a comment: old host names the playmat has moved from; their `/mcp` tells assistants to reconnect at the public address and every other path redirects there. Retire an old address by removing its alias under `domains`, its DNS record and redirect rule at Cloudflare (see docs/domain-move.md), and then this entry. No release is needed. Add a header line saying app-spec edits reach the platform only via `doctl apps update` (already said; keep it).
- `Dockerfile:5` — add `TCGREPLAY_MOVED_FROM` to the settings comment.

**Complexity**: Medium
**Token estimate**: ~50k tokens
**Agent strategy**: Single agent, sequential: settings, then guard, then wiring, then tests, because the pieces are small and interlocked. Verify with `go vet ./... && go test ./cmd/tcgreplay/ ./internal/...`.

### Task: Domain runbook and design notes

Requirements → repo: "Maintainer has the console steps", "Secure, with no www name" (instructions), "Old address can be retired without a release" (instructions), "No stray old address" (design doc) → `tcgreplay`.

**File changes**
- `docs/domain-move.md` (new) — the maintainer runbook. Ordered steps, each with **Do** and **Check** (a command or page and the expected output):
  1. **Google OAuth client** (Google Cloud console > APIs & Services > Credentials > the web client). Authorised redirect URIs include `https://tcgreplay.app/auth/google/callback`. Remove `https://tcgreplay.demo.gs/auth/google/callback` once step 6 is checked. Authorised JavaScript origins, if set, are `https://tcgreplay.app`. Check: the URI is listed, and later, step 7's sign-in completes.
  2. **OAuth consent screen / Branding.** Home page `https://tcgreplay.app/`, privacy policy `https://tcgreplay.app/privacy`, and `tcgreplay.app` among the authorised domains. Check: the console shows them.
  3. **Cloudflare DNS for tcgreplay.app.** An apex record pointing at the app's default ingress (CNAME flattening to `<app>.ondigitalocean.app`, as App Platform's Domains page says). Say plainly whether it is proxied: live today it is proxied, with SSL mode Full (strict). There is no `www` record. Check: `curl -sI https://tcgreplay.app/` shows `x-do-app-origin`. `curl -s "https://cloudflare-dns.com/dns-query?name=www.tcgreplay.app&type=A" -H 'accept: application/dns-json'` has no Answer. Cloudflare's Edge Certificates page shows an active certificate covering `tcgreplay.app`. `curl -sv https://tcgreplay.app/ 2>&1 | grep subject` names `tcgreplay.app`. Note the risk: App Platform may not issue its own certificate while the record is proxied. If the App Platform Domains page shows the domain stuck in "Pending" or "Configuring", grey-cloud the record until it reads "Active", then proxy it again. "Always Use HTTPS" is on, so `curl -sI http://tcgreplay.app/docs/connect/` → 301 to https.
  4. **App Platform app spec.** `doctl apps update <app-id> --spec .do/app.yaml` applies the domains (tcgreplay.app PRIMARY, tcgreplay.demo.gs ALIAS), `TCGREPLAY_PUBLIC_URL` and `TCGREPLAY_MOVED_FROM`; the deploy workflow never applies the spec. Check: `doctl apps spec get <app-id>` shows them. Settings > Domains shows both domains "Active". The runtime log's start line shows `public https://tcgreplay.app` and `moved from tcgreplay.demo.gs`. `curl -s https://tcgreplay.app/.well-known/oauth-protected-resource/mcp` shows `"resource":"https://tcgreplay.app/mcp"`.
  5. **Cloudflare DNS for tcgreplay.demo.gs.** The record stays and is proxied, so the alias receives `/mcp`. Check: the record is listed and proxied.
  6. **Cloudflare redirect rule on the demo.gs zone** (Rules > Redirect Rules). Expression `(http.host eq "tcgreplay.demo.gs" and http.request.uri.path ne "/mcp")`, dynamic target `concat("https://tcgreplay.app", http.request.uri.path)`, status 301, preserve query string. Check:
     - `curl -sI "https://tcgreplay.demo.gs/board?x=1"` → `301` to `https://tcgreplay.app/board?x=1`;
     - `curl -sI https://tcgreplay.demo.gs/docs/connect/` → `301` to the same path on tcgreplay.app;
     - `curl -sIL http://tcgreplay.demo.gs/docs/` ends at `https://tcgreplay.app/docs/`;
     - `curl -s -X POST https://tcgreplay.demo.gs/mcp -H 'content-type: application/json' -H 'accept: application/json, text/event-stream' -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"curl","version":"0"}}}'` returns an `initialize` result whose `instructions` name `https://tcgreplay.app/mcp`, not a 301.
  7. **End-to-end.** Connect claude.ai, ChatGPT and Claude Code to `https://tcgreplay.app/mcp`, sign in, call a tool and open the board link. Check that an old connector shows the moved message.
  
  Retirement section: in Cloudflare, delete the demo.gs redirect rule and the `tcgreplay` record in the demo.gs zone. Remove the `tcgreplay.demo.gs` ALIAS (and `TCGREPLAY_MOVED_FROM`) from `.do/app.yaml` and run `doctl apps update`. No build or deploy is needed: `doctl apps update` with only domain and env changes restarts the service and needs no new build. Check: `curl -sI https://tcgreplay.app/` is 200, `/mcp` is 401 with tcgreplay.app metadata, and `/board` redirects to sign-in. Note that `doctl apps update` with an env change restarts the app (games dropped), as any restart does.
- `.do/app.yaml:1-35` header — add a line pointing at `docs/domain-move.md` for domain and DNS changes.
- `TCG Review Playmat Design and Build Plan.md:305`:
  - amend "the request's `Host` is never trusted" to say the request host is never used to build an address; it is only compared against configured hosts: the moved-host guard (`-moved-from` / `TCGREPLAY_MOVED_FROM`), which picks the moved answer or a redirect, and the visitor-analytics public-host check (added by that plan). Every address still comes from the public URL (decided at epic planning);
  - describe the moved `/mcp` answer and the redirects;
  - replace "with `TCGREPLAY_PUBLIC_URL` set to the app's own URL" with "set to `https://tcgreplay.app`, the primary domain";
  - reference `docs/domain-move.md`.
- `TCG Review Playmat Design and Build Plan.md:307` — website address `https://tcgreplay.app`. In the operator step, the consent screen's home page is `https://tcgreplay.app/`, privacy `https://tcgreplay.app/privacy`, with `tcgreplay.app` among the authorised domains. Mention the website's public address now comes from `.do/app.yaml`, and the whole-site check for the old address.

**Complexity**: Low
**Token estimate**: ~30k tokens
**Agent strategy**: Single agent, sequential. Verify with `grep -rn "demo\.gs"` over the repo outside `.spektacular/` and `internal/site/dist/`. Hits should remain only in the runbook, `.do/app.yaml` (alias and moved setting), the connecting page's note, tests that use it as a fixture, and the design doc's move explanation.

### Task: Carry out the console steps and put the move live

Requirement → repo: "Players can connect at the new address", "Secure, with no www name", "Old pages forward", "Old assistant connection explains the move" (live) → `tcgreplay` (deployment of this repo).

**What the person does**
1. Before implementation starts, commit or discard the uncommitted `.do/app.yaml` edit in the main tree, so the implementation's version merges cleanly.
2. Merge the implemented work to `main`; the "Deploy playmat" workflow deploys it.
3. Follow `docs/domain-move.md` steps 1 to 6 in order, running each step's check. Live probes on 2026-11-12 show steps 3 and 4's address (public URL, certificate, no `www`) and the page redirects already in place, so for those steps the person confirms rather than changes. The new work is:
   - `doctl apps update` to apply `TCGREPLAY_MOVED_FROM` and the domains;
   - narrowing the Cloudflare redirect rule to exclude `/mcp`;
   - the consent-screen URLs;
   - removing the old Google callback.

**How it is checked**: every runbook check shows its expected result, in particular:
- the `initialize` curl against `https://tcgreplay.demo.gs/mcp` returns the moved instructions;
- `curl -sI https://tcgreplay.demo.gs/docs/connect/` returns `301` to tcgreplay.app;
- the runtime log's start line shows `public https://tcgreplay.app` and the moved host.

If the moved answer does not appear at the old `/mcp`, App Platform may not be passing the original Host through. Stop and raise it, rather than changing code ad hoc.

## Testing Strategy

- **Website reads the public address from the deployment definition.** `site/e2e/facts.spec.js` covers `loadPublicURL` with the real definition and fixtures for missing, placeholder, path, trailing slash, http and mismatched-primary values.
- **Site pages and README name the new address.** In `site/e2e/connect.spec.js` and `site/e2e/site.spec.js`, addresses compare against `site.*` facts, there is exactly one `[data-moved-note]` naming `site.mcpURL`, and a whole-site and README scan checks for `demo.gs` outside the note and for any `www.tcgreplay.app`. Run with `make site-test`.
- **Moved answer for MCP connections.** `internal/mcpserver/moved_test.go` runs over in-memory transports. It checks the moved instructions, that tool and prompt names equal the real server's, that every tool call gives `isError` with the new address, that `prompts/get` gives the message, and that a nil manager causes no panic.
- **Moved-host setting and guard in the hosted playmat.**
  - `cmd/tcgreplay/hosted_test.go` tests parsing and the start-up refusal.
  - `cmd/tcgreplay/hosted_moved_test.go` runs end to end through the hosted server. On a moved host: no-token MCP session, moved tool error, stale token ignored, 301 and 308 redirects with path and query for pages, board, `/authorize`, `/token` and metadata, and host matching that ignores case and port.
  - On unlisted hosts, behaviour is unchanged: `/mcp` without a token is still 401, and the existing `TestHostedRoutes` and `TestHostedBoardLinks` pass untouched.
- **Domain runbook and design notes.** No automated test. A grep for `demo.gs` outside `.spektacular/` and the built site finds only intended mentions.
- **Carry out the console steps and put the move live.** These checks are manual and live: the runbook's per-step checks, then the manual acceptance checks and success metrics listed in plan.md's Testing Approach, all captured in the implementation test plan.

## Project References

- Spec: `20261111093353-8ff3589f-domain-move` (epic `20261111093348-e1b5c2b4-public-launch`).
- Design documents: none referenced by the spec.
- Knowledge (repo tier, store `tcgreplay`): `learnings/claude-ai-connector-oauth.md`, `learnings/publishing-to-claude.md` (network pitfalls: cross-host redirects drop `Authorization`), `learnings/publishing-to-chatgpt.md` (auth, tool lists persist until Refresh), `gotchas/go-sdk-sessions-and-auth.md` (`DisableLocalhostProtection`), `gotchas/astro-site-build.md` (`process.cwd()` paths, build before Go tests). Convention `conventions/skills-text-and-packaging.md` was loaded and judged not to apply.
- Repo root: `tcgreplay` → `/home/nicj/code/github.com/nicholasjackson/tcgreplay` (the only registered repo).

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

Tasks here are Low (site and docs, moved middleware, runbook) or Medium (the guard and its wiring), each about 25-50k tokens. Every task runs as a single agent: the Medium task is small and interlocked, so parallel agents would mostly coordinate.

## Migration Notes

- Commit or discard the maintainer's uncommitted `.do/app.yaml` edit in the main tree before implementation worktrees are made. The first task restates the same content.
- After merge: run `doctl apps update <app-id> --spec .do/app.yaml` (the deploy workflow never applies the spec), then narrow the Cloudflare redirect rule to exclude `/mcp`. Order and checks are in `docs/domain-move.md`.
- Existing assistant connections and board sign-ins on the old address are not carried over: players reconnect at tcgreplay.app (spec Non-Goal).
- An env change through `doctl apps update` restarts the app, dropping in-memory games, as any deploy does.

## Performance Considerations

- The guard adds one host comparison per request, and returns at once with no moved hosts configured.
- The moved MCP server is a second `*mcp.Server` instance built once at start-up. Its sessions hold no game state and time out like normal sessions (`mcpSessionTimeout`).
