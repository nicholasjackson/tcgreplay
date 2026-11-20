---
created_date: "2026-11-12"
document_status: final
closed_date: "2026-11-12"
---

# Plan: 20261111093353-8ff3589f-domain-move

<!-- Metadata -->
<!-- Created: 2026-11-12T09:35:28Z -->
<!-- Commit: 6ecc1aa -->
<!-- Branch: main -->
<!-- Repository: tcgreplay (/home/nicj/code/github.com/nicholasjackson/tcgreplay) -->

## Overview

The hosted playmat moves from its borrowed address, tcgreplay.demo.gs, to its own domain, tcgreplay.app, which becomes the one address the website, board, assistant connection and Google sign-in use and show. It comes first in the public launch because the ChatGPT listing, docs and legal pages all name the address, and a published plugin cannot change it later. Players on the old address are not stranded: its pages forward to tcgreplay.app, and an assistant still connected there is told, in the conversation, that the playmat has moved and where to reconnect.

## Conventions

No project conventions apply to this feature. The only always-applied convention (guide text and packaging: no "Claude" or "Nic " in `internal/guides`, never exclude `internal/guides` from a build context) covers files this plan does not change. The moved message lives in `internal/mcpserver`, not in a guide.

## Architecture & Design Decisions

The playmat keeps **one public address**, `TCGREPLAY_PUBLIC_URL`, and that address becomes `https://tcgreplay.app`. Everything that names the playmat while it runs already derives from that single setting (`cmd/tcgreplay/hosted.go:52-76` validates it; `newHosted` at `:295-328` feeds it to the sign-in issuer, the protected-resource metadata, the board's allowed origin and every board link), so the server gains no notion of a second working address and the old one can never become a second sign-in origin. The website reads the same setting: `site/src/lib/facts.ts` stops hard-coding the address and reads `TCGREPLAY_PUBLIC_URL` from `.do/app.yaml`, the file it already reads the hosted limits from, and fails the build if the value is missing, is not a bare `https` scheme and host, or does not match the app's `PRIMARY` domain. Docs prose and tests that spell the address out are updated to tcgreplay.app, and the only place the website may still name `tcgreplay.demo.gs` is one marked note on the connecting page that explains the move. All of this lands in the single registered repo, `tcgreplay` (root `/home/nicj/code/github.com/nicholasjackson/tcgreplay`).

**Old pages are forwarded at Cloudflare; the old assistant connection is answered by the playmat.** Live probes on 2026-11-12 show a Cloudflare redirect on the demo.gs zone already sending every path on `tcgreplay.demo.gs`, `/mcp` included, to the same path and query on tcgreplay.app. That is the spec's preferred layer for page redirects and stays. But a redirect cannot carry a connector across: the client drops its `Authorization` header on a cross-host redirect, and tcgreplay.app's metadata names `resource: https://tcgreplay.app/mcp`, which no longer matches the URL the player entered, so the player sees only a generic failure. A `401` would push the assistant into a confusing re-sign-in at the old address. So the Cloudflare rule is narrowed to leave `/mcp` alone, the request reaches the playmat through App Platform's `tcgreplay.demo.gs` alias domain, and the playmat gives it a deliberate answer. That answer is the one part the platform cannot express.

**The moved answer.** A new setting, `TCGREPLAY_MOVED_FROM` (a comma-separated list of old host names; flag `-moved-from`), names the hosts that have moved. A small wrapper around `hostedHandler` matches the request's host, ignoring case and port, against that list. Requests for any other host pass through untouched, so tests, health checks and local runs are unaffected. On a moved host, `/mcp` is served by a second, **unauthenticated** MCP server built from the same `mcpserver.Server` definition, so it lists exactly the real tools and guide prompts. A receiving middleware (the same pattern as `hostauth.MCPMiddleware`, `internal/hostauth/limit.go:95`) answers every `tools/call` with an error result (`isError: true`), answers every `prompts/get`, and replaces the `initialize` instructions, all with the text "The tcgreplay playmat has moved to https://tcgreplay.app. Remove this connector and reconnect at https://tcgreplay.app/mcp." The text is built only from the configured public URL and never echoes the request's host. Mirroring the tool list matters because ChatGPT keeps the tool list it saw when the connector was made, so whatever tool an existing connector calls, the player gets the message. With no auth demanded, no sign-in is ever started at the old address. Every other path on a moved host gets a permanent redirect (301 for GET and HEAD, 308 otherwise) to the same path and query on the public URL. This is a guard, not the primary redirect: it keeps the origin from serving sign-in or the board at the old address if the Cloudflare rule ever changes while the alias remains. Startup refuses a moved host equal to the public host, which would otherwise loop.

**Retiring the old address is configuration only.** Removing the Cloudflare redirect rule, the `tcgreplay.demo.gs` DNS record and the App Platform alias (and, optionally, `TCGREPLAY_MOVED_FROM`) stops all old-address traffic without a build. The wrapper does nothing for hosts it is not given. The maintainer's ordered console runbook (DNS, Cloudflare rule, App Platform domains and app spec, Google OAuth client and consent screen) is written into the repo's docs with a visible check for each step. Its first step confirms the Google client's new callback before anything else switches. Other options weighed: per-request two-address sign-in, a Cloudflare Worker speaking MCP, a `401` or a single `moved` tool, treating every unknown host as moved, and keeping the address hard-coded in `facts.ts`. Each is rejected with evidence in `research.md#alternatives-considered-and-rejected`.

## Component Breakdown

- **Hosted configuration (changed).** Owns the playmat's settings at start-up. It gains the old-host list (`-moved-from` / `TCGREPLAY_MOVED_FROM`): it parses the list into bare, lower-cased host names, rejects malformed entries and any entry equal to the public address's host, and logs the moved hosts in the start-up line. The public address setting itself is unchanged; only its deployed value moves to tcgreplay.app.
- **Moved-host guard (new).** A thin HTTP wrapper in front of the hosted router. It owns one decision: is this request for a moved host? For moved hosts it sends `/mcp` to the moved MCP server and permanently redirects every other path, with its query, to the public address. Every other request goes to the hosted router unchanged. It exists because the hosted router knows only one address, by design, and must not learn a second one.
- **Moved MCP answer (new, in the MCP server package).** A receiving middleware for an MCP server. Given the new connection address, it answers every tool call with an error result, every prompt request with a prompt carrying the message, and every `initialize` with replacement instructions, all saying the playmat has moved and where to reconnect. The hosted wiring applies it to a second instance of the existing MCP server definition, with no sign-in and no game resolver, so the tool and prompt lists mirror the real server's without a second copy of their definitions. It never reaches a game.
- **Hosted router (unchanged).** Still serves the website, board, sign-in, admin and `/mcp` for the public address only. The guard wraps it rather than changing its routes.
- **Website facts (changed).** Owns the addresses the website prints. It now reads the public address from the App Platform definition, checks that the value is a bare https URL whose host is the app's primary domain, and fails the build otherwise. It derives the connection and board addresses from that value as before.
- **Website content (changed).** The docs pages, the limits page and the README stop naming the old address. The connecting page gains one short, marked note for players whose connector still points at the old address, which is the only allowed mention of it.
- **Website browser suite (changed).** Its address assertions follow the facts rather than a literal. It gains a whole-site check over the built pages and the README: the old address appears only inside the marked move note, and `www.tcgreplay.app` never appears.
- **App Platform definition (changed).** Records the public address as a literal `https://tcgreplay.app`, tcgreplay.app as the `PRIMARY` domain, `tcgreplay.demo.gs` as an `ALIAS`, and `TCGREPLAY_MOVED_FROM=tcgreplay.demo.gs`. The comments say how to retire the old address by configuration.
- **Maintainer's domain runbook (new doc) and the design doc (changed).** The runbook is the ordered console steps for Google OAuth, Cloudflare DNS and redirect rule, App Platform domains and app spec, and retirement, each with a visible check. The design doc's hosted-mode and website sections name tcgreplay.app, describe the moved-host behaviour, and update the consent-screen operator step.

## Data Structures & Interfaces

No persisted data, wire format or stored type changes. The plan adds one setting, two Go seams and one website fact, all small contracts between components.

**Setting: the moved hosts.** `-moved-from` flag, defaulting to `TCGREPLAY_MOVED_FROM`: a comma-separated list of bare host names, such as `tcgreplay.demo.gs`. Empty means no host has moved, and behaviour is exactly as today. Parsed once at start-up into a set of lower-cased host names:

```go
// in package main (cmd/tcgreplay)
type movedHosts map[string]bool            // lower-cased host, no port
func parseMovedHosts(raw string, public *url.URL) (movedHosts, error)
```

An entry with a scheme, path, port or `@`, or one equal to the public URL's host, is an error, and hosted mode does not start.

**Moved-host guard.**

```go
// in package main
func movedGuard(hosts movedHosts, public *url.URL, moved http.Handler, next http.Handler) http.Handler
```

`moved` is the moved MCP server's Streamable HTTP handler, served at `/mcp` on moved hosts. `next` is `hostedHandler`'s mux. When `hosts` is empty, `movedGuard` returns `next` itself.

**Moved MCP answer.**

```go
// in package mcpserver
func MovedText(mcpURL string) string            // the one message, naming mcpURL
func Moved(mcpURL string) mcp.Middleware         // receiving middleware
```

`Moved` intercepts three methods and passes every other method (`tools/list`, `prompts/list`, `ping`, notifications) to the real handler:

- `initialize`: the real result with `Instructions` replaced by `MovedText`.
- `tools/call`: `&mcp.CallToolResult{IsError: true, Content: [text MovedText]}`, without calling the tool.
- `prompts/get`: `&mcp.GetPromptResult{Description: "The playmat has moved", Messages: [user text MovedText]}`.

**Website fact.** `site.publicURL` in `site/src/lib/facts.ts` keeps its shape (`publicURL`, `mcpURL`, `boardURL`, `repoURL`). Its value now comes from a new exported loader:

```ts
export function loadPublicURL(yamlText: string, source = '.do/app.yaml'): string
// TCGREPLAY_PUBLIC_URL of the playmat service: https, host only, no trailing
// slash, equal to the domain listed with type PRIMARY. Throws FactError otherwise.
```

**Marked move note.** The connecting page's note about the old address carries `data-moved-note` on its wrapping element. The whole-site check allows `demo.gs` only inside elements with that attribute.

## Implementation Detail

**Existing patterns followed.**
- The new setting follows the existing hosted settings: a flag whose default is its `TCGREPLAY_*` variable, validated once at start-up, a refusal to start on a bad value (as `hostedPublicURL` does for the public address), and a mention in the one start-up log line.
- The moved answer is a receiving middleware, the same shape the sign-in layer already uses to wrap every MCP method.
- The website keeps treating `.do/app.yaml` as the single source of the facts it prints. The public address joins the limits there, read and checked the same way, with the same fail-the-build error style.
- Browser tests keep reading facts rather than repeating literals.

**New pattern: a host-scoped front door.** Until now the hosted handler has never looked at the request's host, and the design doc says the host is never trusted. The guard is the first code that reads it. It does not trust the host for anything that names the playmat. It only uses the host to choose between "serve normally" and "tell this caller the playmat has moved", and every address it emits comes from the configured public URL. A reader sees one small wrapper applied in `runHosted`, outside the existing router, with the existing routes untouched. That separation is what keeps "one address" true inside the router. The design doc's statement is amended to say that the request host is never used to build an address; it is only compared against configured hosts (this moved-host guard, and the visitor-analytics public-host check that plan adds). Decided at epic planning.

**New pattern: a mirrored, inert MCP server.** The moved server is not a hand-written list of tools. It is a second instance of the same server definition, with the moved middleware in front and no sign-in, so its tool and prompt lists cannot drift from the real ones. Because the middleware answers every call before any handler runs, the instance needs no game manager or resolver and never touches game state. A developer adding a tool later gets the moved behaviour for it automatically.

**Website.** The address leaves the source. Pages already printing `site.*` facts are unchanged. Markdown pages that spell the address out are edited by hand, because Markdown cannot read facts. The browser suite then pins them: the connecting tests compare against `site.mcpURL` and `site.boardURL`, and a new whole-site scan walks every built HTML file and the README. The scan fails on any `demo.gs` outside a `data-moved-note` element and on any `www.tcgreplay.app`. So a future page that hard-codes an old or `www` address fails the suite rather than shipping.

**Operations.** The App Platform definition becomes the reviewed record of the domains and the moved-host setting. The runbook says an app-spec edit reaches the platform only when the maintainer applies the app spec by hand, because the deploy workflow never applies the spec. The runbook is ordered so that sign-in cannot break mid-move: the Google callback first, then DNS and certificate, then the app spec, then the Cloudflare rule, then the checks. Retirement is the same steps in reverse, with no release.

## Dependencies

- Design documents this plan was built on: none. The spec carries no design references.
- **`github.com/modelcontextprotocol/go-sdk` v1.6.0.** Provides `mcp.Middleware`, `InitializeResult.Instructions`, `CallToolResult` and `GetPromptResult`, which the moved answer is built from, and `NewStreamableHTTPHandler` for serving it. No upgrade needed.
- **Internal `mcpserver` package.** Gains the moved middleware. Its existing server definition is reused unchanged for the mirrored instance.
- **Internal `hostauth` package and the hosted router.** Unchanged. The guard sits in front of them.
- **Website toolchain (Astro, Playwright, `yaml`).** Already used by the site build and browser suite. No new packages.
- **Cloudflare (DNS for `tcgreplay.app` and `demo.gs`, redirect rule on the `demo.gs` zone).** External service. Its configuration changes by hand: the existing redirect rule must exclude `/mcp`. Covered by the runbook task for a person.
- **DigitalOcean App Platform.** External service. The app spec, applied by hand with DigitalOcean's CLI, must carry the alias domain and `TCGREPLAY_MOVED_FROM` before the old `/mcp` can be answered. Covered by the runbook task for a person.
- **Google Cloud OAuth client and consent screen.** External service. It needs the tcgreplay.app callback, which the live sign-in suggests is already present, and the consent screen's home page, privacy URL and authorised domain moved to tcgreplay.app. Covered by the runbook task for a person.
- **The maintainer's uncommitted `.do/app.yaml` edits in the main working tree.** These already make tcgreplay.app primary and demo.gs an alias. The plan restates their content because an implementation worktree will not see them. The maintainer should commit or discard them before implementation, so the worktree's change does not conflict on merge.
- **Upstream specs.** None. Domain move is first in the public-launch epic. `chatgpt-submission-prep` and `launch-docs` depend on it landing first.
- **Parallel spec `observability`.** No ordering dependency, but it may also edit the hosted start-up code and `.do/app.yaml`. The epic's order step should sequence the two if their plans touch the same files.

## Testing Approach

**Go unit and integration tests (most coverage: the moved answer and the guard).** These follow the existing hosted tests, which start a real hosted server on a test listener with a fake Google and connect real MCP clients.
- **The moved middleware**, tested in the MCP server package over an in-memory transport. `initialize` carries the moved instructions. `tools/list` and `prompts/list` match the real server's names exactly. Every tool call, including one that would normally need a game, returns `isError` with text naming the new connection address. `prompts/get` returns the message. No game manager is touched.
- **The guard**, tested end to end through the hosted server by setting the request host.
  - On a moved host, `/mcp` with no token opens an MCP session (no `401`, no `WWW-Authenticate`), and calling any tool returns the moved error naming `https://<public host>/mcp`.
  - A stale bearer token on that request is ignored, not refused.
  - On a moved host, `GET /docs/connect/` and `GET /board?x=1` get `301` to the same path and query on the public URL. A `POST` gets `308`. `/authorize`, `/auth/login` and `/.well-known/oauth-protected-resource/mcp` redirect too, so no sign-in or metadata is served for the old host.
  - Host matching ignores case and port.
  - Requests for the public host and for unlisted hosts are byte-for-byte what they are today. The existing route-table and board-link tests keep passing unchanged, which is the regression guard for "one address".
- **Settings.** Parsing accepts a list with blanks and mixed case, and rejects a scheme, path or port, or the public host itself. Hosted start-up refuses to run with a bad value, as it already does for a bad public address. The start-up log names the moved hosts.

**Website build and browser suite (Playwright, through the Go site handler).**
- The facts tests gain the public address. The real `.do/app.yaml` yields `https://tcgreplay.app`. A definition missing the value, using `${APP_URL}`, with a path or trailing slash, or whose `PRIMARY` domain differs fails with a message naming the problem.
- The connecting-page tests assert against `site.mcpURL` and `site.boardURL` rather than literals. The copy button and the Claude Code command copy `https://tcgreplay.app/mcp`.
- A new whole-site scan reads every built HTML file and the README:
  - `demo.gs` appears only inside a `data-moved-note` element.
  - `www.tcgreplay.app` appears nowhere.
  - The connecting page's move note names `https://tcgreplay.app/mcp`.

  This guards the spec's "no stray old address" and "no www" acceptance criteria automatically.

**Deliberate gaps.**
- No automated test of Cloudflare, App Platform, DNS, certificates or Google: these are external console state, checked by the runbook's per-step checks and the manual checks below.
- No automated test of what claude.ai or ChatGPT render: that needs the real assistants.

**Acceptance criteria checked only against the live deployment.** Each is **Manual — captured in the implementation test plan**:
- **Everything answers at the new address.** `/`, `/docs/` and `/board` at `https://tcgreplay.app`, and the published sign-in and connection metadata name only tcgreplay.app.
- **Valid HTTPS, plain HTTP redirected, no www.** The browser accepts the certificate, `http://tcgreplay.app/docs/connect/` ends at https, and `www.tcgreplay.app` does not resolve.
- **Old pages redirect permanently, keeping path and query.** Covers `https://tcgreplay.demo.gs/docs/connect/` and `/board?x=1`, and `http://tcgreplay.demo.gs/docs/`.
- **Full connect-and-play at the new address.** From each of claude.ai, ChatGPT and Claude Code: Google sign-in completes, a simple tool call succeeds, and the board link starts with `https://tcgreplay.app/` and opens the board signed in, with no error.
- **Old assistant connection shows the moved message.** In claude.ai and ChatGPT, with a connector still on `https://tcgreplay.demo.gs/mcp`, the conversation or the connector's error names `https://tcgreplay.app/mcp`.
- **The old address signs no one in.** A sign-in started through `tcgreplay.demo.gs` (board link or connector) never completes there and ends at tcgreplay.app.
- **Retiring the old address needs no release.** The rehearsal is reading the runbook's retirement section and confirming every step is configuration only. The live check, with demo.gs removed from Cloudflare and App Platform and no new build, is that tcgreplay.app still serves website, board and assistant.
- **Console steps are written and checkable.** Each runbook step has a command or page and its expected result.

**Success metrics.**
- **The ChatGPT plugin submission names tcgreplay.app with no later address change.** **Manual — captured in the implementation test plan.** It is confirmed when the chatgpt-submission-prep spec submits.
- **No player asks the maintainer for help reconnecting in the first two weeks.** **Manual — captured in the implementation test plan.** The maintainer watches the support channels for two weeks.
- **No server error at tcgreplay.app caused by the address change in the first week.** **Manual — captured in the implementation test plan.** The maintainer reviews the App Platform request and runtime logs for sign-in, connection and board-origin errors.

**Manual reviews.**
- **Runbook review.** **Manual — captured in the implementation test plan.** The maintainer reads the runbook against the live consoles and confirms the step order and each check's expected output.
- **Moved-message wording.** **Manual — captured in the implementation test plan.** The maintainer confirms the text reads well as it appears in claude.ai and ChatGPT.

## Milestones & Tasks

### Milestone 1: The website, docs and README name tcgreplay.app everywhere

**What changes**: Every page of the website, the README and the project's design notes give tcgreplay.app as the playmat's address. That includes the connection address players copy, the Claude Code command, the example board link and the limits page. The only place the old address still appears is a short note on the connecting page telling players with an old connector what to do. The website now takes the address from the same deployment setting the server uses, so the two can no longer disagree, and its build fails if that setting is missing or does not match the app's primary domain. The suite catches any page that names the old address or a `www` name.

**Validation point**: The website builds, and its browser suite passes. That includes the new whole-site scan, which finds `demo.gs` only in the move note and `www.tcgreplay.app` nowhere, and copy buttons that give `https://tcgreplay.app/mcp`.

#### - [x] Task: Website reads the public address from the deployment definition
**Id:** 336cf3bc-b8e0-49c8-b5cc-8eb72c4e28bb
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

The App Platform definition is updated to record the playmat's one public address as `https://tcgreplay.app`, with tcgreplay.app as the primary domain and the old address as an alias. The website stops hard-coding its address and reads that same value while it builds, so the site and the server always name the same address. The build fails with a clear message if the value is missing, is not a bare https address, or does not match the primary domain.

*Technical detail:* [context.md#task-website-reads-the-public-address-from-the-deployment-definition](./context.md#task-website-reads-the-public-address-from-the-deployment-definition)

**Acceptance criteria**:
- [x] The deployment definition names `https://tcgreplay.app` as the public address and tcgreplay.app as the primary domain, with `tcgreplay.demo.gs` only as an alias, and its comments say the two must match.
- [x] Every address the website prints from its facts (home page copy block, the connecting page's address block, the privacy page) is `https://tcgreplay.app` or derived from it.
- [x] A deployment definition with the public address missing, left as a platform placeholder, carrying a path or trailing slash, or differing from the primary domain stops the site build with a message naming the problem.

#### - [x] Task: Site pages and README name the new address
**Id:** 92eaa296-5177-4b63-b8a6-5dee0d1a81be
**Repo:** tcgreplay
**Depends on:**
- 336cf3bc-b8e0-49c8-b5cc-8eb72c4e28bb — Website reads the public address from the deployment definition
**Execution:** agent

The docs pages, the limits page and the README that spell out the old address are changed to tcgreplay.app. The connecting page gains one short marked note for players whose connector still uses the old address, telling them to remove it and reconnect at the new one. The browser suite follows the facts instead of literal addresses. A new whole-site check fails the build if any page or the README names the old address outside that note, or names a `www` address.

*Technical detail:* [context.md#task-site-pages-and-readme-name-the-new-address](./context.md#task-site-pages-and-readme-name-the-new-address)

**Acceptance criteria**:
- [x] The connecting instructions for claude.ai, ChatGPT and Claude Code, the sign-in page's example board link, the limits page and the README all name tcgreplay.app.
- [x] The connecting page has exactly one note about the old address, and it tells the player to reconnect at `https://tcgreplay.app/mcp`.
- [x] The built website and the README mention `demo.gs` only inside that note and never mention `www.tcgreplay.app`, and the browser suite fails if either rule is broken.
- [x] The copy buttons for the playmat's address and the Claude Code command copy the tcgreplay.app versions exactly.

### Milestone 2: An assistant still connected to the old address is told where the playmat moved

**What changes**: The playmat recognises requests for an old address named in its settings. An assistant connection there no longer fails with a bare error or a confusing sign-in: whatever the assistant tries, it gets a clear message that the playmat has moved, with `https://tcgreplay.app/mcp` as the address to reconnect at. Every other request for the old address is sent permanently to the same page on tcgreplay.app, so the old address can never sign anyone in. Nothing changes for tcgreplay.app itself. The deployment definition records tcgreplay.app as the primary domain, the old address as an alias, and the old address as moved.

**Validation point**: The Go tests pass. On a moved host, `/mcp` opens without sign-in, and every tool call returns the moved message. Pages on a moved host get permanent redirects with path and query kept. Requests for the public address behave exactly as before.

#### - [x] Task: Moved answer for MCP connections
**Id:** 9f103488-9914-4a9d-8c31-ec9f1b48e3a1
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

The MCP server package gains a "moved" answer that can be put in front of any instance of the playmat's MCP server. With it in place, the server still lists its usual tools and guides, but every tool call comes back as an error explaining that the playmat has moved and naming the new connection address. Guide requests and the server's instructions carry the same message. Mirroring the real tool list means an assistant that cached the old tools still reaches the message, whichever tool it calls.

*Technical detail:* [context.md#task-moved-answer-for-mcp-connections](./context.md#task-moved-answer-for-mcp-connections)

**Acceptance criteria**:
- [x] A server with the moved answer lists exactly the same tools and guides as the normal server.
- [x] Calling any of its tools returns an error result whose text says the playmat has moved and names the new connection address, and no game is created or changed.
- [x] Asking it for a guide, or reading its instructions on connecting, gives the same moved message.

#### - [x] Task: Moved-host setting and guard in the hosted playmat
**Id:** bdcae4a4-1b3a-4a9e-8ed6-9b6d06825e56
**Repo:** tcgreplay
**Depends on:**
- 9f103488-9914-4a9d-8c31-ec9f1b48e3a1 — Moved answer for MCP connections
- 336cf3bc-b8e0-49c8-b5cc-8eb72c4e28bb — Website reads the public address from the deployment definition
**Execution:** agent

The hosted playmat gains a setting that lists old host names. For a request on one of those hosts, the assistant connection is answered without sign-in by the moved MCP server, and every other path is permanently redirected to the same path and query on the public address. Requests for the public address and any other host are untouched, and the playmat refuses to start if the list is malformed or names its own public host. The deployment definition sets the list to the old address, with comments on how to retire it.

*Technical detail:* [context.md#task-moved-host-setting-and-guard-in-the-hosted-playmat](./context.md#task-moved-host-setting-and-guard-in-the-hosted-playmat)

**Acceptance criteria**:
- [x] An assistant connecting at `/mcp` on a moved host gets a working connection with no sign-in prompt, and any tool it calls answers with the moved message naming `https://tcgreplay.app/mcp`, even if it still sends an old token.
- [x] Pages, the board, sign-in and the sign-in metadata on a moved host all redirect permanently to the same path and query on the public address, so nothing signs in at the old host.
- [x] Requests for the public address behave exactly as before, including board links that start with the public address.
- [x] The playmat refuses to start with a moved host that is malformed or equal to its public host, and its start-up log lists the moved hosts.
- [x] The deployment definition lists `tcgreplay.demo.gs` as moved and explains that removing the alias and DNS retires it without a release.

### Milestone 3: The move is live, with a checked runbook the maintainer can follow and later reverse

**What changes**: The maintainer has written, ordered console steps for the Google sign-in client and consent screen, Cloudflare DNS and the redirect rule, and the App Platform domains and app spec. Each step comes with a command or page and what it should show. The runbook also covers retiring the old address later without a release. Following it puts the earlier milestones live: tcgreplay.app serves everything over HTTPS with no `www` name, old pages forward, and an assistant on the old connection sees the moved message.

**Validation point**: The runbook's checks all show their expected results against the live deployment. A request to `https://tcgreplay.demo.gs/mcp` gets an MCP response rather than a redirect, and old pages return `301` to tcgreplay.app.

#### - [x] Task: Domain runbook and design notes
**Id:** 28b63d48-377f-404b-bbbe-291b32af0e24
**Repo:** tcgreplay
**Depends on:**
- 92eaa296-5177-4b63-b8a6-5dee0d1a81be — Site pages and README name the new address
- bdcae4a4-1b3a-4a9e-8ed6-9b6d06825e56 — Moved-host setting and guard in the hosted playmat
**Execution:** agent

A maintainer runbook is written with the ordered console steps for the move, each with a check that shows whether it worked. The steps are the Google sign-in client and consent screen first, then Cloudflare DNS and certificate, the App Platform domains and app spec, and the Cloudflare redirect rule narrowed to leave the assistant connection alone. A retirement section explains how to remove the old address later by configuration alone. The project's design notes are updated to name tcgreplay.app and to describe the moved-host behaviour.

*Technical detail:* [context.md#task-domain-runbook-and-design-notes](./context.md#task-domain-runbook-and-design-notes)

**Acceptance criteria**:
- [x] The runbook lists the Google, Cloudflare and App Platform steps in an order that never breaks sign-in mid-move, and every step names a command or page and what it should show.
- [x] The runbook says whether each DNS record is proxied and how to confirm the certificate for tcgreplay.app was issued.
- [x] The runbook's retirement section removes the old address with configuration changes only, and says how to check tcgreplay.app still works afterwards.
- [x] The design notes name tcgreplay.app as the address, describe the moved-host answer, and no longer give the old address except when explaining the move.

#### - [x] Task: Carry out the console steps and put the move live
**Id:** c2f0b060-ea41-4d5f-9a99-3ab40202b7f7
**Repo:** tcgreplay
**Depends on:**
- 28b63d48-377f-404b-bbbe-291b32af0e24 — Domain runbook and design notes
**Execution:** human — needs access to the Google Cloud, Cloudflare and DigitalOcean consoles and the production app, and acts outside the repo (DNS, deploy)

The maintainer merges the work so it deploys, then follows the runbook. That means confirming the Google client and consent screen, the tcgreplay.app DNS and certificate, and the domains, then applying the app spec so the moved-host setting reaches the platform. Last, the Cloudflare redirect rule on the old zone is narrowed so the assistant connection reaches the playmat. Each step's check is run as it goes.

*Technical detail:* [context.md#task-carry-out-the-console-steps-and-put-the-move-live](./context.md#task-carry-out-the-console-steps-and-put-the-move-live)

**Acceptance criteria**:
- [ ] Every runbook step has been carried out or confirmed, and its check showed the expected result.
- [ ] The live playmat runs with tcgreplay.app as its public address and the old address listed as moved.
- [ ] The old address's assistant connection reaches the playmat rather than being redirected, while its pages still forward to tcgreplay.app.

## Open Questions

Two questions can only be answered once the moved answer runs on the live platform:

- **Does App Platform pass the original `Host` (`tcgreplay.demo.gs`) through to the container for an alias domain behind Cloudflare's proxy?** This depends on App Platform's ingress behaviour, which can only be observed live, in the runbook's step 6 `initialize` check. If the old `/mcp` returns the normal `401`, or anything other than the moved instructions, STOP and ask the user before changing code. Candidate fixes are matching `X-Forwarded-Host`, or a Cloudflare origin rule that sets a header.
- **Do claude.ai and ChatGPT show the moved tool-error text to the player when a connector still points at the old address?** This depends on each assistant's handling of an existing OAuth connector that now gets an unauthenticated session. ChatGPT may first try to refresh its token against the old token endpoint, which redirects. It can only be seen in the real assistants (the manual check "Old assistant connection shows the moved message"). If either assistant shows only a generic failure, STOP and ask the user. The fallback options differ in user-visible behaviour, so they are the user's call.

## Out of Scope

- **Keeping existing assistant connections or board sign-ins alive across the move.** Players sign in and connect again at tcgreplay.app; the old address only tells them so (spec Non-Goals).
- **Visitor analytics, observability, the privacy and terms pages and the Pokémon disclaimer.** These are the `visitor-analytics`, `observability` and `privacy-terms-disclaimer` specs in the public-launch epic.
- **The ChatGPT plugin files, domain verification (`/.well-known/openai-apps-challenge`) and the Plugin Directory flow in the docs.** These belong to the `chatgpt-submission-prep` and `launch-docs` specs. Docs change here only where they name the address, plus the one move note.
- **A `www.tcgreplay.app` name or redirect.** It is deliberately not set up (spec requirement), so the runbook adds no record for it.
- **Multi-address sign-in.** The old address never gets its own issuer, metadata or Google callback, which the maintainer rejected (spec Constraints).
- **Moving hosting or DNS providers.** The playmat stays on its App Platform app and both zones stay at Cloudflare (spec Constraints).
- **Removing the old address.** This plan leaves `tcgreplay.demo.gs` forwarding. Retiring it is a later configuration change by the maintainer, following the runbook's retirement section, with no follow-up spec.
- **Automated tests of Cloudflare, App Platform, Google or the assistants' rendering.** These are external, and are checked by the runbook and the manual checks in the test plan.

## Changelog


### 2026-11-12 — Task: Website reads the public address from the deployment definition

**What was done**: `site/src/lib/facts.ts` no longer hard-codes the playmat's address: a new exported `loadPublicURL` reads `TCGREPLAY_PUBLIC_URL` from the playmat service in `.do/app.yaml` and requires a bare `https://` scheme-and-host value equal to the `PRIMARY` domain, failing the build with a `site facts:` message naming the problem otherwise; `site.*` (publicURL, mcpURL, boardURL) is derived from it. `astro.config.mjs` now says `https://tcgreplay.app` with a comment tying it to `TCGREPLAY_PUBLIC_URL`. The facts tests cover the real definition and fixtures for missing, placeholder, path, trailing slash, query, fragment, non-URL, http, mismatched and absent PRIMARY values.

**Deviations**: `.do/app.yaml` needed no change: the maintainer's edit (public URL `https://tcgreplay.app`, tcgreplay.app PRIMARY, tcgreplay.demo.gs ALIAS, with the comments) was already committed on main before the worktree was made, so the task only confirmed it. The env lookup shared by `loadLimits` was factored into a private `readSpec` helper so both loaders use it. The home-page copy test in `site/e2e/site.spec.js` was switched to `site.mcpURL` now to keep that suite green; `site/e2e/connect.spec.js` still asserts the old address and fails until the next task rewrites it with the docs.

**Files changed**:
- `site/src/lib/facts.ts`
- `site/astro.config.mjs`
- `site/e2e/facts.spec.js`
- `site/e2e/site.spec.js`

**Discoveries**: A fresh worktree has no `site/node_modules`; run `npm ci` in the worktree's `site/` before building. Uppercase hosts in `TCGREPLAY_PUBLIC_URL` are rejected (the WHATWG URL lower-cases the host, so the raw value no longer equals scheme+host), which is consistent with the PRIMARY-domain match.

### 2026-11-12 — Task: Site pages and README name the new address

**What was done**: The connecting page's claude.ai, ChatGPT and Claude Code steps, the sign-in page's example board link and the README now name tcgreplay.app, and the limits page prints the address from `site.publicURL`. The connecting page gained one `<aside class="note" data-moved-note>` telling players with an old `tcgreplay.demo.gs` connector to remove it and add `https://tcgreplay.app/mcp`, with a small `.note` style. The browser suite now compares addresses against the `site.*` facts, checks the single move note, and has a whole-site scan of the built HTML and the README that fails on `demo.gs` outside the note, on any `www.tcgreplay.app`, or on more or fewer than one note.

**Deviations**: The scan also asserts that exactly one move note exists across the site, a small addition beyond the plan's text. The note is written as inline HTML (`<p>`, `<code>`) inside the Markdown, since Markdown inside a raw HTML block is not rendered.

**Files changed**:
- `site/src/content/docs/connect.md`
- `site/src/content/docs/sign-in.md`
- `site/src/pages/docs/limits.astro`
- `site/src/styles/site.css`
- `README.md`
- `site/e2e/connect.spec.js`
- `site/e2e/site.spec.js`

**Discoveries**: None beyond the plan.

### 2026-11-12 — Task: Moved answer for MCP connections

**What was done**: New `internal/mcpserver/moved.go` adds `MovedText(mcpURL)` and the `Moved(mcpURL)` receiving middleware: on a server wrapped with it, `initialize` carries the moved message as its instructions, every `tools/call` returns it as an error result without running the tool, and every `prompts/get` returns it as the prompt; listings and other methods pass through, so the tool and guide lists mirror the real server's. `moved_test.go` checks, over both the in-memory and Streamable HTTP transports, that a wrapped server with no game manager lists exactly the real tools and prompts and answers every tool, prompt and the instructions with the moved text.

**Deviations**: `internal/mcpserver/server.go`'s `InitializedHandler` now also returns early when `M` is nil. Without it, a server built with no manager and no resolver (the moved instance) panicked on `notifications/initialized`, because that handler labels the manager with the client's name. The plan assumed the instance needed no manager; this one-line guard makes that true.

**Files changed**:
- `internal/mcpserver/moved.go`
- `internal/mcpserver/moved_test.go`
- `internal/mcpserver/server.go`

**Discoveries**: go-sdk runs `ServerOptions.InitializedHandler` synchronously inside the `notifications/initialized` method handler, so receiving middleware sees that notification and anything the handler touches must be valid even on a server that never runs a tool.

### 2026-11-12 — Task: Moved-host setting and guard in the hosted playmat

**What was done**: Hosted mode gained `-moved-from` / `TCGREPLAY_MOVED_FROM`, a comma-separated list of old host names, parsed at start-up (`parseMovedHosts`: blanks skipped, case ignored, a scheme, path, port, user, space or the public host refused, so start-up stops) and named in the start-up log line ("moved from …" or "no moved hosts"). New `cmd/tcgreplay/moved.go` adds `movedGuard`, wrapped around `hostedHandler` in `runHosted`: for a moved host (matched ignoring case and port), `/mcp` goes to `movedMCP`, an unauthenticated Streamable HTTP endpoint serving a second instance of the playmat's MCP server with `mcpserver.Moved` in front, and every other path redirects permanently (301 GET/HEAD, 308 otherwise) to the same path and query on the public URL. Other hosts reach the router unchanged. `.do/app.yaml` sets `TCGREPLAY_MOVED_FROM: tcgreplay.demo.gs` with comments on retiring it by configuration, and the Dockerfile's settings comment names it.

**Deviations**: The moved MCP handler is built in `runHosted` (by `movedMCP(c, public)`) rather than returned from `newHosted`, so `newHosted`'s callers are unchanged. The test helper `serveHosted` (in `cmd/tcgreplay/cardsets_test.go`, not `hosted_test.go`) now routes through `movedGuard` too, so every hosted test exercises the real wiring. A new `TestRunHostedMovedFrom` covers the guard wired through `runHosted` itself. The domains comment in `.do/app.yaml` now mentions the moved-host setting. `.do/app.yaml` points at `docs/domain-move.md`, which the next task writes.

**Files changed**:
- `cmd/tcgreplay/moved.go`
- `cmd/tcgreplay/main.go`
- `cmd/tcgreplay/hosted.go`
- `cmd/tcgreplay/hosted_test.go`
- `cmd/tcgreplay/hosted_moved_test.go`
- `cmd/tcgreplay/cardsets_test.go`
- `.do/app.yaml`
- `Dockerfile`

**Discoveries**: None beyond the plan; `internal/webui/server.go`'s `localOnly` was only a model for host normalisation and was not changed.

### 2026-11-12 — Task: Domain runbook and design notes

**What was done**: New `docs/domain-move.md` is the maintainer's runbook for the address, nine ordered steps each with a **Do** and a **Check** (a command or page and its expected result): Google OAuth client, consent screen, attach tcgreplay.app in App Platform, Cloudflare DNS and certificate for tcgreplay.app (proxied, Full (strict), no `www`), the proxied demo.gs record, `doctl apps update` with the app spec, the narrowed Cloudflare redirect rule (excluding `/mcp`) with the moved-answer `initialize` curl, end-to-end checks in the assistants, and removing the old Google callback. A retirement section removes the old address by configuration only, with checks that tcgreplay.app still works. The `.do/app.yaml` header points at the runbook. The design doc's hosted-mode and website paragraphs now name tcgreplay.app, describe the moved-host guard and answer, carry the amended Host statement (the request host is never used to build an address, only compared against configured hosts), and update the consent-screen operator step and the site's address fact and old-address scan.

**Deviations**: The step order differs from the plan's list. The first goal check found that DNS checks placed before the app spec could not show their results, because the domain was not yet attached. The runbook now attaches tcgreplay.app in App Platform (Settings > Domains) first, without changing the public address, then sets up DNS and the certificate, then applies the app spec. This means sign-in moves only once the new address already serves. The runbook therefore has nine steps, not seven: the human task's "steps 1 to 6" now means the whole runbook, steps 1 to 9. The retirement section notes that committing the `.do/app.yaml` edit to `main` also runs the deploy workflow, which redeploys the same code.

**Files changed**:
- `docs/domain-move.md`
- `.do/app.yaml`
- `TCG Review Playmat Design and Build Plan.md`

**Discoveries**: Every push to `main` outside `cardsets/` redeploys, including an `.do/app.yaml`-only edit, so "configuration only" changes still cause a restart when committed. `doctl apps update` is what actually applies them.

### 2026-11-12 — Task: Carry out the console steps and put the move live

**What was done**: This is a human task, so no code changed. The user confirmed through the epic coordinator: "The domain is already live." Live probes on 2026-11-12 agree for the new address. `https://tcgreplay.app/` returns 200 from App Platform behind Cloudflare. Its protected-resource metadata names `https://tcgreplay.app/mcp` with issuer `https://tcgreplay.app`. `http://tcgreplay.app/docs/connect/` redirects to https. `https://tcgreplay.demo.gs/board?x=1` redirects with a 301 to the same path and query on tcgreplay.app.

**Deviations**: The goal check ran three times and judged all three criteria not met, so none are ticked. The user accepted the shortfall (`accepted_shortfall`, recorded via `implement goal decide`) rather than have the epic wait. The criteria cannot hold until this branch is merged and deployed, because `https://tcgreplay.demo.gs/mcp` is still redirected (301 to `https://tcgreplay.app/mcp`) and the moved answer is not deployed yet. These runbook items are left for the maintainer after merge, following `docs/domain-move.md`, and are captured as manual steps in the test plan:
- step 6: apply the app spec with `doctl apps update <app-id> --spec .do/app.yaml`, which sets `TCGREPLAY_MOVED_FROM=tcgreplay.demo.gs` and the domains;
- step 7: narrow the demo.gs Cloudflare redirect rule so it leaves `/mcp` alone, and run its `initialize` check;
- step 2: set the consent-screen URLs to tcgreplay.app;
- step 8: the end-to-end checks in claude.ai, ChatGPT and Claude Code, including an old connector;
- step 9: remove the old Google callback.

**Files changed**:
- none

**Discoveries**: None.
