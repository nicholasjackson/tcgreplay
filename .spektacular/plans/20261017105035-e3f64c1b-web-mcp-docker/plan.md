---
created_date: "2026-10-17"
document_status: final
closed_date: "2026-10-17"
---

# Plan: 20261017105035-e3f64c1b-web-mcp-docker

<!-- Metadata -->
<!-- Created: 2026-10-17T14:52:19Z -->
<!-- Commit: 77e6a6a -->
<!-- Branch: main -->
<!-- Repository: git@github.com:nicholasjackson/tcgreplay.git -->

## Overview

Adds a hosted mode to the tcgreplay binary so the playmat can run as a single Docker container on DigitalOcean App Platform, behind the owner's HTTPS domain. Claude in claude.ai (web or mobile) can then connect to it as a custom connector, use every playmat tool, and hand the player a link to a live board that follows Claude's moves and takes the player's own, with no desktop involved. This is the epic's first, open, owner-only trial: it proves claude.ai can drive the hosted board before sign-in and per-user games arrive, and it leaves the desktop setup (Claude Code and Claude Desktop) exactly as it is.

## Conventions

No project conventions apply to this feature: the `tcgreplay` knowledge store holds no entries yet (the always-applied load for the repo returned nothing, and topic searches for mcp, http, docker, daemon, web, board, server, transport, deploy, cards and tcgdex found no entries). The plan follows the patterns already in the code instead: the repo's existing vet-and-test target, standard-library `testing` with `httptest`, and doc comments on every exported identifier.

## Architecture & Design Decisions

All work lands in the single registered repo, `tcgreplay` (`/home/nicj/code/github.com/nicholasjackson/tcgreplay`). The hosted playmat becomes a fourth run mode of the existing binary, `tcgreplay -hosted`, beside the shim (default), `-daemon` and `-no-daemon` (`cmd/tcgreplay/main.go:86-93`). Hosted mode builds the same game manager and MCP server the other modes use (`newServer`, `cmd/tcgreplay/main.go:100-106`) and serves everything from **one plain-HTTP listener** on the platform's port: `/mcp` is the go-sdk's Streamable HTTP handler (`mcp.NewStreamableHTTPHandler`, go-sdk v1.6.0), returning the one shared `*mcp.Server` for every session, exactly as the daemon shares one server across shim connections today (`cmd/tcgreplay/daemon.go:297-332`); every other path (`/`, `/ws`, `/api/card`) is the existing board handler (`internal/webui/server.go:65-71`). TLS stays outside the container (App Platform, with Cloudflare in front). The shim, daemon and in-process modes are not touched, so the desktop keeps its stdio and Unix-socket paths.

Hosted mode is configured by settings, not code: `-public-url` (default `$TCGREPLAY_PUBLIC_URL`) and `-listen` (default `:$PORT`, else `:8080`), with the data and cache directories defaulting under the container user's `$HOME` as on the desktop. The **public URL is the single source of truth for the address the playmat is reached at**: board links in tool results are built from it (never from the request's `Host`, which is untrusted behind Cloudflare), and the board's websocket origin allow-list adds its host. Later specs in the epic (google-login) reuse this same setting for sign-in addresses. The MCP server gains a `BoardURL` field; when it is set (hosted mode only), the results of `new_game`, `import_log`, `load_game` and `set_opponent_deck` carry a `board` link (the public URL's root, since there is one shared current game), and the server instructions gain a hosted-only line telling the assistant to hand that link to the player. With `BoardURL` empty (every desktop mode), results, tool list and instructions are byte-for-byte as today.

The board's desktop guards stay as they are by default: the loopback `Host` check (`localOnly`, `internal/webui/server.go:87-102`) and loopback websocket origins (`internal/webui/hub.go:307-309`). The board server gains an optional public URL; when set, the `Host` check is skipped (the hosted board is public by design in this trial and the host header is not trustworthy) and the public host joins the allowed origins, so other websites still cannot drive the board from a visitor's browser. The MCP handler likewise sets `DisableLocalhostProtection` in hosted mode. To survive Cloudflare's ~100 s idle cut-off, the hub's per-client writer sends a websocket ping every 30 s; the board client already reconnects with backoff and reloads full state on connect (`web/src/lib/ws.js:17-23`, `internal/webui/hub.go:345-347`), so a dropped connection heals without a page reload. MCP sessions are stateful (so the client-name handshake that labels the assistant keeps working, `internal/mcpserver/server.go:39-43`), with a generous idle timeout, and no server-initiated keep-alive pings (a ping claude.ai fails to answer would close the session).

Packaging is one multi-stage `Dockerfile` at the repo root: a Node stage runs the Vite build into `internal/webui/dist/app` (`web/vite.config.js:13-16`), a Go stage embeds it and builds a static binary, and a distroless non-root runtime image runs `tcgreplay -hosted`, with CA certificates for runtime TCGdex fetches (card images load in the browser straight from TCGdex's CDN). `.do/app.yaml` keeps the App Platform definition in the repo: one service, one instance, built from the Dockerfile with `deploy_on_push` on `main` only, `TCGREPLAY_PUBLIC_URL` set as an env var. This beats the alternatives (a separate binary, two listeners behind a proxy, the legacy SSE transport, stateless sessions, a CI-built registry image) on the spec's constraints and on effort; the evidence is in `research.md#alternatives-considered-and-rejected`. Because claude.ai's acceptance of an authless connector is the epic's go/no-go, the first milestone ships the smallest hosted server that claude.ai can connect to and proves it before the board, links and deployment polish follow.

## Component Breakdown

- **Hosted run mode (new, in the `tcgreplay` command)** — owns the `-hosted` mode: reads the hosted settings (public URL, listen address), builds the shared game manager and MCP server through the same constructor the desktop modes use, reopens the last game, and serves one plain-HTTP server until the process is told to stop, then shuts it down gracefully. It is the only new component; it composes the three below and leaves the shim, daemon and in-process modes untouched.
- **Hosted HTTP handler (new, alongside the hosted run mode)** — one router that mounts the MCP Streamable HTTP endpoint at `/mcp` (stateful sessions, idle timeout, localhost protection off) and hands every other path to the board server. It is a plain function of the MCP server and the board server (each already configured with the public URL), so tests can serve it from an in-memory HTTP server without starting a process.
- **MCP tool server (changed)** — gains an optional board URL. When set, the game-opening tools (`new_game`, `import_log`, `load_game`, `set_opponent_deck`) add a `board` link to their results and the server instructions gain one line asking the assistant to hand that link to the player. When unset, which is every desktop mode, its tool list, instructions and results are unchanged. The tool set itself is shared by every transport, which is what gives the hosted connector full tool parity.
- **Board server (changed)** — gains an optional public URL. Without it, the loopback `Host` guard and loopback-only websocket origins stay exactly as today. With it, the `Host` guard is skipped and the public URL's host is added to the allowed websocket origins. Its live-update hub sends a websocket ping to each connected board every 30 seconds so proxies with idle limits (Cloudflare) keep the connection open; the existing board client already reconnects and reloads state if a connection does drop.
- **Container image (new, `Dockerfile` + `.dockerignore`)** — builds the board UI, embeds it in a static binary, and ships it on a minimal non-root runtime with CA certificates, starting `tcgreplay -hosted` on port 8080. The game files and the TCGdex card cache live in the container's own filesystem, fetched at runtime.
- **App Platform app definition (new, `.do/app.yaml`)** — declares the single-instance service built from the repo's Dockerfile, deploying on every push to `main` only, with the HTTP port, health check, public-URL environment setting and custom domain. Created once by the owner; every later deploy is a push to `main`.
- **Build and docs (changed)** — the `Makefile` gains targets to build and run the container image locally in one command each, and the design document's runtime paragraph describes the hosted mode and its settings beside the desktop modes.

## Data Structures & Interfaces

No persisted data formats change: game files, the card cache and the board's websocket messages keep their shapes. The new and changed contracts are configuration and in-process interfaces, plus one additive field in four tool results.

**Hosted settings** — the operator-facing contract, as flags with environment defaults:

| Setting | Flag | Environment default | Fallback |
|---|---|---|---|
| Hosted mode | `-hosted` | — | off (desktop modes as today) |
| Public address | `-public-url` | `TCGREPLAY_PUBLIC_URL` | `http://localhost:<port>`, with a logged warning |
| Listen address | `-listen` | `:` + `PORT` | `:8080` |
| Data, cache, seat, offline | `-data`, `-cache`, `-seat`, `-offline` | — | as today (under the user's home) |

The public URL is the address users and claude.ai reach the playmat at (scheme and host, optionally a port; no path). It is the single value later specs reuse for sign-in addresses.

**Command config (changed)** — the binary's internal config gains the hosted settings beside the existing data, cache, seat, UI address and offline fields:

```go
type config struct {
    dataDir, cacheDir, seat, uiAddr string
    offline                         bool
    publicURL, listen               string // hosted mode only
}
```

**Hosted handler (new)** — composes the two existing servers on one router:

```go
// /mcp → Streamable HTTP for srv; everything else → ui.Handler().
func hostedHandler(srv *mcp.Server, ui *webui.Server) http.Handler
```

**MCP tool server (changed)** — one optional field:

```go
type Server struct {
    M          *game.Manager
    ClaudeSeat state.Seat
    HumanSeat  state.Seat
    Version    string
    BoardURL   string // hosted mode: link added to game-opening results; "" = desktop, unchanged
}
```

**Board server (changed)** — one optional field:

```go
type Server struct {
    M      *game.Manager
    Human  state.Seat
    Public *url.URL // hosted mode: skip the loopback Host guard, allow this host's websocket origin; nil = desktop, unchanged
}
```

**Tool results (additive)** — `new_game`, `import_log`, `load_game` and `set_opponent_deck` gain one key when `BoardURL` is set:

```json
{ "game": "2026-10-11-ledjo23", "board": "https://playmat.example.com/", "...": "existing fields unchanged" }
```

## Implementation Detail

**Patterns followed.** Hosted mode is one more case in the command's existing mode switch, built the way the daemon is: one game manager, one MCP server shared by every session, one board server watching the manager. A developer reading the command package sees four sibling run functions (shim, daemon, in-process, hosted), each a short composition of the same constructors; hosted mode's distinguishing code is only the HTTP wiring. The board and MCP servers stay transport-agnostic: hosted behaviour is switched on by an optional field on each (a public URL on the board, a board URL on the MCP server), with the zero value meaning "desktop, exactly as today". That keeps every hosted difference behind an explicit, testable switch rather than spread through conditionals on a global mode.

**New pattern: one router, two protocols.** The MCP endpoint and the board share one `net/http` router. The MCP SDK's Streamable HTTP handler is mounted as an ordinary handler at a fixed path, and the board's existing handler serves everything else, so the board's routes do not change and the MCP endpoint does not need to know the board exists. TLS, the public hostname and HTTPS termination stay outside the process.

**New pattern: one public-address value.** The configured public URL is parsed and validated once at start-up and passed down. It feeds both board links and the board's allowed websocket origin; no code reads the request's `Host` or forwarding headers to build an address. Later specs (sign-in redirect addresses) take the same value.

**Keep-alive.** The board hub's per-client writer gains a ticker that pings the browser well inside proxy idle limits; a failed ping ends that client exactly as a failed write does today, and the client's existing reconnect-and-reload loop takes over. MCP traffic relies on short request/response calls instead of server pings.

**Tests follow the existing shape and gain a transport dimension.** The MCP tool tests already connect a client to the server through a helper; that helper becomes parameterised over transports (in-memory, as today, and Streamable HTTP through an in-process HTTP test server), and each existing tool test runs as a subtest under both. That is how "the existing automated tool tests pass through the hosted connection" is met without duplicating tests. The hosted handler gets its own tests using the same in-process HTTP server, covering the single-address routing, tool-list parity, links and the board's websocket behind a public origin. The board's existing foreign-host and foreign-origin rejection test stays as the guard that desktop behaviour has not loosened.

**Packaging follows the existing build.** The container build repeats what the Makefile's build target does (Vite build into the Go package, then the Go build) in isolated stages, so there is still one way the UI gets embedded; the Makefile gains thin wrappers for building and running the image. Deployment is declarative, in a reviewed app definition in the repo, and has no CI scripts.

## Dependencies

- **Design documents this plan was built on: none.** The spec carries no design references.
- **Upstream specs and plans: none.** This is the first spec of epic `20261017105033-91f90842-hosted-playmat`; nothing must land before it. Downstream, `memory-store-record` and `google-login` build on it (google-login reuses the public-address setting introduced here).
- **`github.com/modelcontextprotocol/go-sdk` v1.6.0 (existing)** — provides the Streamable HTTP server handler for `/mcp` and the Streamable client transport used by the tests. No version change needed.
- **`github.com/coder/websocket` (existing, indirect via the go-sdk)** — the board's websocket; its ping support provides the keep-alive. May move from indirect to direct in `go.mod` once the board code calls it for pings; no version change.
- **Internal packages (changed)** — `internal/mcpserver` (board URL field), `internal/webui` (public URL field, ping), `cmd/tcgreplay` (hosted mode). `internal/game`, `internal/cards`, `internal/state` are used unchanged.
- **TCGdex (external service, existing)** — card definitions fetched at runtime from `api.tcgdex.net` by the container, card images loaded by the browser from `assets.tcgdex.net`. The container needs outbound HTTPS and CA certificates; no change to the client.
- **Container build images (new)** — `node:22` (board UI build), `golang:1.27` (binary, matching `go.mod`'s Go 1.27.2), `gcr.io/distroless/static-debian12:nonroot` (runtime).
- **DigitalOcean App Platform (external service, new)** — builds the Dockerfile from GitHub on push to `main` and runs one instance with HTTPS. Needs the owner's DigitalOcean account with GitHub access to `nicholasjackson/tcgreplay`, and the DigitalOcean CLI or control panel once, to create the app from `.do/app.yaml`.
- **Cloudflare (external service, new for this project)** — proxied DNS for the owner's domain in front of App Platform; its ~100 s idle limit shapes the websocket keep-alive.
- **claude.ai custom connectors (external service)** — must accept an authless Streamable HTTP server. Unverified; proven first (Milestone 1), and the epic's go/no-go.
- **Docker (developer tool)** — to build and run the image locally through two new Makefile targets.

## Testing Approach

Tests stay in Go's standard `testing` package with `httptest`, beside the code they cover, and run under the repo's existing vet-and-test Makefile target. Nothing new needs network access by default; the existing `TCGREPLAY_LIVE=1` gate remains the way to exercise TCGdex for real.

**Integration: every tool test over both transports (most coverage).** The existing MCP tool tests are parameterised over transports and run as subtests twice: in-memory (as today) and Streamable HTTP through an in-process HTTP server mounting the same handler hosted mode uses. This is the load-bearing guarantee for **Full tool parity** and **Every tool works hosted**: the same calls, inputs and assertions (game creation, import, moves, mode switches, profile, sampling) pass through the hosted connection. A parity test asserts the tool list seen over Streamable HTTP equals the in-memory one, name for name and schema for schema.

**Integration: the hosted handler.** Tests serve the hosted handler with a configured public URL and check that:
- `/mcp` speaks MCP and `/`, `/ws`, `/api/card` serve the board from the same server (**One address**);
- `import_log`, `new_game`, `load_game` and `set_opponent_deck` results carry a `board` link that starts with the configured public URL, and changing the configured URL changes the link (**Board link**, **Public address setting** at code level);
- a board websocket dialled with the public URL's origin is accepted, while a foreign origin is still refused;
- **latency: a `move` made through the MCP connection reaches an open board websocket as a state update within 2 seconds** — the automated half of the "board move within 2 s" metric;
- a card moved through the board's websocket appears in `get_state` read through the MCP connection (**User moves reach Claude**).

**Unit: board server and MCP server switches.** With no public URL the board still rejects a foreign `Host` and a foreign websocket origin (the existing rejection test, unchanged); with a public URL the foreign `Host` is accepted. With no board URL the game-opening tool results have no `board` key and the server instructions are unchanged (**Desktop unaffected**). The hub sends pings on its interval (driven by a short test interval) and a client that stops answering is dropped.

**Unit: hosted settings.** Flag and environment resolution: `PORT` and `TCGREPLAY_PUBLIC_URL` defaults, the `:8080` and `http://localhost:<port>` fallbacks, and rejection of a public URL with no scheme or host or with a path.

**Regression.** The full existing suite (shim relay, installer, importer, game, board) passes unchanged, which with the switch tests covers the automated half of **Desktop regression**.

**Deliberate gaps.** No automated test drives Docker, App Platform, Cloudflare or claude.ai: those need real infrastructure and accounts, and the in-process HTTP tests already exercise the same handler the container runs. The container is checked by building and running it by hand.

**Success metrics:**
- Owner imports and reviews a real game end to end from claude.ai on a phone (import, opponent deck, board link, analysis on the board) — **Manual — captured in the implementation test plan.**
- The connector survives a 30–60 minute review session with no hosting-caused reconnects or failed calls — **Manual — captured in the implementation test plan.**
- A board move by Claude appears in the open tab within 2 seconds — **Behavioural test** (MCP move → board websocket update within 2 s through the hosted handler), plus a **Manual — captured in the implementation test plan** check on the deployed app behind Cloudflare.
- Deploying takes only a push to `main`, with no manual DigitalOcean steps — **Manual — captured in the implementation test plan.**
- The trial gives a clear go/no-go answer for the epic — **Manual — captured in the implementation test plan** (record whether claude.ai drives the hosted board, or exactly what blocked it).

**Manual reviews:**
- **Connector connects**: adding the public `/mcp` address as a claude.ai custom connector succeeds and its tool list matches the desktop's — **Manual — captured in the implementation test plan.**
- **Import and play through the connector** in claude.ai: import a real log with an opponent deck, move, rewind, branch, annotate, each read back — **Manual — captured in the implementation test plan.**
- **Link opens the right board** on another device (phone) — **Manual — captured in the implementation test plan.**
- **Board stays connected** after 10 idle minutes behind Cloudflare and shows Claude's next move without a reload — **Manual — captured in the implementation test plan.**
- **Public address setting** changed on the running deployment (or a local container) and the container restarted, with no rebuild, changes the links — **Manual — captured in the implementation test plan.**
- **One-command run**: the image builds with one command and starts with one command plus settings, and the board loads — **Manual — captured in the implementation test plan.**
- **Push deploys** from `main` only (a push to another branch does not deploy) — **Manual — captured in the implementation test plan.**
- **Card data out of the box** in a fresh container: `card_text` returns text and the board shows the card image — **Manual — captured in the implementation test plan.**
- **Desktop regression**: Claude Code and Claude Desktop launch the playmat locally, connect and import a log as before — **Manual — captured in the implementation test plan.**

## Milestones & Tasks

### Milestone 1: Claude in claude.ai can drive the hosted playmat

**What changes**: The playmat can run as a container on the web. The owner builds and starts it with one command each, and every push to `main` deploys it to DigitalOcean App Platform behind the owner's HTTPS domain with no manual steps. Adding its address as a custom connector in claude.ai lists the same tools as the desktop, and Claude can import a game and play it through them, with card text fetched on demand. This settles the epic's go/no-go question first: either claude.ai drives the hosted playmat, or we know exactly what blocks it. The desktop setup (Claude Code and Claude Desktop) is unchanged.

**Validation point**: The full automated suite passes, with every MCP tool test also passing over the hosted (Streamable HTTP) connection; the image builds and runs locally; after a push to `main` the app deploys; claude.ai connects to `https://<domain>/mcp`, lists the tools, and imports a real log; the go/no-go outcome is recorded.

#### - [x] Task: Serve the playmat over HTTP in a hosted mode
**Id:** 4f92c498-4101-4238-b5ef-ff6641470e1a
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Adds `tcgreplay -hosted`, a new run mode of the existing binary that keeps the game manager and MCP server in-process and serves them over one plain-HTTP listener: MCP at `/mcp` through the official library's Streamable HTTP transport, and the board on every other path. The listen address and public address come from flags with environment defaults (`PORT`, `TCGREPLAY_PUBLIC_URL`), so a container can be configured without code changes. The shim, daemon and in-process modes are left exactly as they are.

*Technical detail:* [context.md#task-serve-the-playmat-over-http-in-a-hosted-mode](./context.md#task-serve-the-playmat-over-http-in-a-hosted-mode)

**Acceptance criteria**:
- [x] Starting the binary in hosted mode serves an MCP endpoint at `/mcp` and the board's routes from the same address and port.
- [x] An MCP client connecting over HTTP sees the same tool list, with the same input schemas, as one connecting the desktop way.
- [x] The listen port follows `PORT` when set and is 8080 otherwise; the public address follows `TCGREPLAY_PUBLIC_URL` when set and falls back to `http://localhost:<port>` with a logged warning otherwise; a public address without a scheme or host, or with a path, stops start-up with a clear error.
- [x] Hosted mode logs to standard error and shuts down cleanly when the container is stopped.
- [x] Running the binary without `-hosted` behaves as before, and the existing tests pass.

#### - [x] Task: Run the MCP tool tests over the hosted transport
**Id:** c0bf5d4d-b38e-4db0-87ba-27161cf3f276
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Makes every existing MCP tool test run twice: once over the in-memory connection, as today, and once over Streamable HTTP through an in-process web server. This is the automated proof that every playmat tool works the same through the hosted connector as on the desktop. No test is weakened or duplicated by hand; the transport becomes a parameter of the shared connection helper.

*Technical detail:* [context.md#task-run-the-mcp-tool-tests-over-the-hosted-transport](./context.md#task-run-the-mcp-tool-tests-over-the-hosted-transport)

**Acceptance criteria**:
- [x] Each existing MCP tool test runs as a subtest for both the in-memory and the Streamable HTTP connection, and both pass.
- [x] The assistant name is still taken from the client's name over the HTTP connection.
- [x] A failure over one transport is reported with that transport's name.

#### - [x] Task: Build the hosted playmat as a container image
**Id:** 7363b5d3-a237-4d53-82f5-615569108a43
**Repo:** tcgreplay
**Depends on:**
- 4f92c498-4101-4238-b5ef-ff6641470e1a — Serve the playmat over HTTP in a hosted mode
**Execution:** agent

Adds a multi-stage `Dockerfile` that builds the board UI, embeds it into a static binary, and runs `tcgreplay -hosted` on a minimal non-root image with CA certificates, so card data is fetched from TCGdex at runtime with no setup. A `.dockerignore` keeps local build output and dependencies out of the build, and two Makefile targets build and run the image with one command each.

*Technical detail:* [context.md#task-build-the-hosted-playmat-as-a-container-image](./context.md#task-build-the-hosted-playmat-as-a-container-image)

**Acceptance criteria**:
- [x] The image builds from a clean checkout with one command and contains the built board UI, not the placeholder page.
- [x] The container starts with one command plus settings, listens on port 8080 by default, and the board page loads at its address.
- [x] The container runs as a non-root user and can write its game files and card cache.
- [x] In a freshly started container with network access, the card text tool returns text for a card in an imported game.

#### - [x] Task: Define the App Platform app in the repo
**Id:** ec3826cf-87be-4c14-9a8f-b530b773a190
**Repo:** tcgreplay
**Depends on:**
- 7363b5d3-a237-4d53-82f5-615569108a43 — Build the hosted playmat as a container image
**Execution:** agent

Adds the DigitalOcean App Platform app definition to the repo, so the deployment is reproducible and reviewed with the code. It declares one service, one instance, built from the repo's Dockerfile and deployed on every push to `main` (and no other branch), serving plain HTTP on port 8080 with the public address set from the app's own URL.

*Technical detail:* [context.md#task-define-the-app-platform-app-in-the-repo](./context.md#task-define-the-app-platform-app-in-the-repo)

**Acceptance criteria**:
- [x] The app definition builds from the repo's Dockerfile, deploys on push to `main` only, and runs exactly one instance.
- [x] The service's HTTP port, health check and public-address setting match what the container expects.
- [x] The owner's custom domain is declared in the definition, with a clearly marked placeholder for the owner to fill in.
- [ ] The definition passes App Platform's spec validation.

#### - [x] Task: Create the App Platform app and its domain
**Id:** e7a98d25-c8ac-46f2-9001-88ea4c9cf159
**Repo:** tcgreplay
**Depends on:**
- ec3826cf-87be-4c14-9a8f-b530b773a190 — Define the App Platform app in the repo
**Execution:** human — needs the owner's DigitalOcean account, GitHub authorisation and Cloudflare DNS for the domain

The owner fills in the real domain in the app definition, creates the app on App Platform from it, authorises App Platform's GitHub access to the repo, and points the domain at the app through Cloudflare. After this one-time setup, deploying is a push to `main`.

*Technical detail:* [context.md#task-create-the-app-platform-app-and-its-domain](./context.md#task-create-the-app-platform-app-and-its-domain)

**Acceptance criteria**:
- [x] The app exists on App Platform, built from `main` with deploy on push enabled.
- [x] The owner's domain serves the app over HTTPS through Cloudflare, and `https://<domain>/mcp` answers MCP requests.

### Milestone 2: A board link opens a live board that follows Claude

**What changes**: Whenever Claude imports, creates, loads or rebuilds a game on the hosted playmat, its result carries a link to the board at the configured public address, and Claude hands it to the player. Opening it in any browser, including a phone, shows the live board on the same address as the connector: Claude's moves appear within two seconds without a reload, the board stays connected through long idle spells behind Cloudflare, and moves the player makes on the board are what Claude reads back. Changing the public-address setting and restarting changes the links with no rebuild. The desktop board and tool results are unchanged, and the design document describes the hosted mode.

**Validation point**: The hosted-handler tests pass (links start with the configured address, a Claude move reaches an open board within 2 seconds, a board move is visible to Claude, the public origin is accepted while foreign ones are still refused, desktop results carry no link), and on the deployed app the board link opens on a phone and follows Claude's moves.

#### - [x] Task: Let the board serve a public address and stay connected
**Id:** 971e60a2-6107-4180-b60f-91aa2003f971
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Gives the board server an optional public address. Without one, it keeps today's loopback-only host and origin checks exactly. With one, it accepts requests for any host (the hosted board is public in this trial) and accepts live connections from pages on the public address, while still refusing pages from other sites. The live connection to each open board is also pinged every 30 seconds so proxies such as Cloudflare do not drop it while idle.

*Technical detail:* [context.md#task-let-the-board-serve-a-public-address-and-stay-connected](./context.md#task-let-the-board-serve-a-public-address-and-stay-connected)

**Acceptance criteria**:
- [x] With no public address, a request with a foreign host and a live connection from a foreign origin are refused, as today.
- [x] With a public address, the board loads for a request whose host is not loopback, and a live connection from the public address's origin is accepted while one from another site is refused.
- [x] An open board receives a ping at the configured interval, and a board that stops responding is disconnected.

#### - [x] Task: Add board links to game-opening tool results
**Id:** 918423d4-37e8-4435-b12c-9cbea30fa6b6
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Gives the MCP tool server an optional board address. When it is set, the results of importing a log, starting a new game, loading a game and rebuilding a game with an opponent deck include a `board` link, and the server's instructions ask the assistant to give that link to the player. When it is not set, as on the desktop, tool results, the tool list and the instructions are exactly as before.

*Technical detail:* [context.md#task-add-board-links-to-game-opening-tool-results](./context.md#task-add-board-links-to-game-opening-tool-results)

**Acceptance criteria**:
- [x] With a board address set, the import, new game, load game and set opponent deck results each contain a `board` link equal to that address.
- [x] With a board address set, the server instructions tell the assistant to share the board link.
- [x] Without a board address, no result contains a `board` key and the instructions are unchanged.

#### - [x] Task: Wire the public address through hosted mode
**Id:** 2db4d50d-55f1-4b34-b3e9-acafee5c81f6
**Repo:** tcgreplay
**Depends on:**
- 4f92c498-4101-4238-b5ef-ff6641470e1a — Serve the playmat over HTTP in a hosted mode
- 971e60a2-6107-4180-b60f-91aa2003f971 — Let the board serve a public address and stay connected
- 918423d4-37e8-4435-b12c-9cbea30fa6b6 — Add board links to game-opening tool results
**Execution:** agent

Passes the configured public address to both the board server and the MCP tool server in hosted mode, so board links and the board's allowed origin come from the one setting and never from the incoming request. End-to-end tests through the hosted handler prove the board follows Claude within two seconds and that moves made on the board reach Claude.

*Technical detail:* [context.md#task-wire-the-public-address-through-hosted-mode](./context.md#task-wire-the-public-address-through-hosted-mode)

**Acceptance criteria**:
- [x] In hosted mode, board links in tool results start with the configured public address, and changing the setting changes the links with no rebuild.
- [x] A move Claude makes through the hosted connection reaches an open board as a live update within 2 seconds.
- [x] A card moved from the board shows in the state Claude reads back through the hosted connection.
- [x] A board opened from the public address connects live, and the desktop modes still pass no public address and show no links.

#### - [x] Task: Document the hosted mode
**Id:** 83ee551e-61ae-4c47-8e8c-25bfb5fc7e9a
**Repo:** tcgreplay
**Depends on:**
- 2db4d50d-55f1-4b34-b3e9-acafee5c81f6 — Wire the public address through hosted mode
- ec3826cf-87be-4c14-9a8f-b530b773a190 — Define the App Platform app in the repo
**Execution:** agent

Describes the hosted mode beside the desktop modes in the project's design document: how to run it, its settings, the single address for connector and board, how it deploys, and the trial's limits (no login, one shared game, games lost on deploy). The command's own package comment names the new mode.

*Technical detail:* [context.md#task-document-the-hosted-mode](./context.md#task-document-the-hosted-mode)

**Acceptance criteria**:
- [x] The design document's runtime description covers hosted mode, its settings, the `/mcp` connector address, local Docker use and push-to-deploy.
- [x] The trial's limits are stated where an operator will read them.

## Open Questions

- **Does claude.ai accept an authless Streamable HTTP connector at `https://<domain>/mcp`?** Depends on claude.ai's live connector requirements, which can only be exercised against a deployed server (end of Milestone 1). If claude.ai refuses to connect, insists on OAuth, or cannot list or call the tools, STOP and report exactly what blocks it to the user: this is the epic's go/no-go, and an OAuth requirement would pull google-login's work forward, which is the user's decision, not the implementer's.
- **Do Cloudflare and App Platform pass the `/mcp` streamed (SSE) responses and the board websocket through unbuffered and within their idle limits?** Depends on the proxies' real behaviour, only observable on the deployed app. If tool calls hang or are cut off, or the board drops despite the 30-second ping, first try JSON responses for `/mcp` (`StreamableHTTPOptions.JSONResponse`) and a shorter ping interval; if neither fixes it, STOP and ask the user before changing the hosting arrangement (e.g. bypassing the Cloudflare proxy).

## Out of Scope

- **Login and access control** — the hosted playmat is open for this short trial. Tracked in the google-login spec (`20261017123240-21493a95-google-login`), which reuses this plan's public-address setting.
- **In-memory-only storage, a tool that returns the game record to Claude, and exact replay of an import from a saved record** — tracked in the memory-store-record spec (`20261017112555-30c99ab1-memory-store-record`). This plan keeps writing game files to the container's own filesystem.
- **Separate games per user** — everyone who reaches the server shares one current game, as on the desktop. Tracked in the per-user-games spec (`20261017143118-b05aa6b1-per-user-games`), which also decides how board links map to a user's game.
- **Keeping games or the card cache across restarts or deploys** — every deploy replaces the container; a game in progress must be re-imported. No volumes are attached.
- **Packaging the tcg-* skills for claude.ai** — a later spec; this plan only proves the tools work through the connector.
- **Notion integration** — not part of the hosted playmat.
- **A board inside the Claude chat (MCP Apps) or chat inside the board page** — the board opens in its own browser tab.
- **More than one instance, autoscaling or TLS inside the container** — single instance, plain HTTP; HTTPS is App Platform's and Cloudflare's job.
- **A CI pipeline or container registry** — App Platform builds the Dockerfile itself on push to `main`; no GitHub Actions or DOCR.
- **Server-initiated MCP keep-alive pings and stream resumption (event store)** — not added; tool calls are short requests. Revisited only if the open question on proxy behaviour shows a need.
- **Changes to the board client** — the existing client already uses `wss` over HTTPS and reconnects with state reload; no Svelte changes are planned.

## Changelog

### 2026-10-17 — Task: Serve the playmat over HTTP in a hosted mode

**What was done**: Added `tcgreplay -hosted`, which builds the shared game manager and MCP server through `newServer`, reopens the last game, and serves MCP over Streamable HTTP at `/mcp` (stateful sessions, 4 h idle timeout, localhost protection off) and the board on every other path from one plain-HTTP listener. `-listen` defaults to `:$PORT` else `:8080`; `-public-url` defaults to `$TCGREPLAY_PUBLIC_URL`, falls back to `http://localhost:<port>` with a warning, and is validated (http/https, host, no path/query/fragment/userinfo). Hosted mode logs only to stderr and waits for `http.Server.Shutdown` (5 s) before returning on SIGTERM.

**Deviations**: `hostedPublicURL` also rejects userinfo in the URL (not named in the plan). `runHosted` waits for Shutdown to finish before returning, so in-flight requests drain (raised by the test author). Tests go further than the plan's list: `main()` is exercised in a child process for env-driven config, stderr-only logging and SIGTERM exit.

**Files changed**:
- `cmd/tcgreplay/main.go`
- `cmd/tcgreplay/hosted.go`
- `cmd/tcgreplay/hosted_test.go`

**Discoveries**: Plan line numbers for `internal/webui/hub.go` and `cmd/tcgreplay/daemon.go` are stale (files are shorter than the plan assumed); symbols are all present. The public URL is parsed in `runHosted` but not yet passed to the board or MCP server (Wire task).

### 2026-10-17 — Task: Run the MCP tool tests over the hosted transport

**What was done**: The MCP test helper `connect` now takes a transport from a `transports` table (`inmemory` and `http`, the latter a Streamable HTTP handler on an `httptest.Server`), and `forEachTransport` runs each tool test as a named subtest per transport. `TestMCPRoundTrip` and `TestImportLogTool` run under both; a small `TestTransportsNamed` pins the transport names.

**Deviations**: None.

**Files changed**:
- `internal/mcpserver/server_test.go`

**Discoveries**: The assistant-name handshake (`InitializedHandler`) works over stateful Streamable HTTP with the SDK's default handler options, so the existing `Codex` wait passes over HTTP unchanged.

### 2026-10-17 — Task: Build the hosted playmat as a container image

**What was done**: Added a three-stage `Dockerfile` (Node 22 Vite build into `internal/webui/dist/app`, Go 1.27 static build with the UI embedded, `gcr.io/distroless/static-debian12:nonroot` runtime running `tcgreplay -hosted` on port 8080), a `.dockerignore`, and `make docker-build` / `make docker-run` targets (`IMAGE`, `PUBLIC_URL` overridable). Verified by building from a clean worktree, serving the built board (not the placeholder) as uid 65532, and importing a log and fetching `card_text` from TCGdex over `/mcp` in a fresh container.

**Deviations**: `.dockerignore` also excludes `.claude` and `.agents` (tracked symlinks to skills, not needed for the build). No Go tests, as planned; the test step was a no-op.

**Files changed**:
- `Dockerfile`
- `.dockerignore`
- `Makefile`

**Discoveries**: `docker` on the dev machine is podman-compatible (images are tagged `localhost/<name>`). The `card_text` tool's argument is `card`, not `iid`.

### 2026-10-17 — Task: Define the App Platform app in the repo

**What was done**: Added `.do/app.yaml`: app `tcgreplay` in region `lon`, one service `playmat` built from the root `Dockerfile`, GitHub `nicholasjackson/tcgreplay` branch `main` with `deploy_on_push: true`, `http_port: 8080`, `instance_count: 1`, `apps-s-1vcpu-0.5gb`, health check on `/`, `TCGREPLAY_PUBLIC_URL=${APP_URL}` at run time, an ingress rule routing `/` to the service, and a PRIMARY domain `playmat.example.com` marked as a placeholder. Header comments give the `doctl apps create/update` commands and the trial limits.

**Deviations**: Added an explicit `ingress` rule (`/` → `playmat`), not in the plan's sketch, so the routing of `/mcp`, `/ws` and the board is declared rather than defaulted. The acceptance criterion "passes App Platform's spec validation" is left unchecked: `doctl` is not installed (and `doctl apps spec validate` needs the owner's API token), so the file was checked only against the app spec reference (fields, `lon` region, `apps-s-1vcpu-0.5gb` slug, `${APP_URL}` bindable variable all documented). The human task's `doctl apps create --spec .do/app.yaml` performs the real validation.

**Files changed**:
- `.do/app.yaml`

**Discoveries**: `${APP_URL}` resolves to the app's primary domain with scheme (e.g. `https://my-domain.com`), which `hostedPublicURL` accepts as is.

### 2026-10-17 — Task: Let the board serve a public address and stay connected

**What was done**: `webui.Server` gained `Public *url.URL`. With it nil (desktop) the handler keeps the loopback `localOnly` guard and loopback-only websocket origins; with it set, the Host guard is skipped and `Public.Host` joins the allowed origins, so other sites are still refused. Each board's writer goroutine now pings the client every `pingInterval` (30 s) and drops it when a ping is not answered within `pingTimeout` (10 s). Tests: `TestPublicAddress`, and `TestPing` using a raw websocket client to observe ping frames and play a board that never answers; `TestRejectsForeignHostAndOrigin` is unchanged.

**Deviations**: Added a `pingTimeout` package variable beside `pingInterval` (the plan hard-coded 10 s) so the unresponsive-board test runs in under a second instead of over 10 s. The webui test `setup` now delegates to a new `setupServer(t, public)`; behaviour unchanged.

**Files changed**:
- `internal/webui/server.go`
- `internal/webui/hub.go`
- `internal/webui/webui_test.go`

**Discoveries**: coder/websocket answers pings only while a `Read` is in progress and exposes no ping callback, so observing server pings in a test needs a raw client. A ping blocks the per-client writer for up to `pingTimeout`, delaying queued state pushes for that client only.

### 2026-10-17 — Task: Add board links to game-opening tool results

**What was done**: `mcpserver.Server` gained `BoardURL`. When set, `new_game`, `import_log`, `set_opponent_deck` and `load_game` results carry `"board": <BoardURL>` (via `withBoard`), and the server instructions gain one line asking the assistant to give the player the board link (via `instructions()`). With it empty, results, tool descriptions and the `instructions` constant are unchanged. Tests `TestBoardLinks` and `TestNoBoardLinksOnDesktop` run over both transports; the test helper `connect` now delegates to `connectServer(t, tr, s)`.

**Deviations**: None.

**Files changed**:
- `internal/mcpserver/server.go`
- `internal/mcpserver/server_test.go`

**Discoveries**: None.

### 2026-10-17 — Task: Wire the public address through hosted mode

**What was done**: `newServer` takes a board URL (desktop callers in `main.go` and `daemon.go` pass `""`), and a new `newHosted(c, public)` builds the manager, the MCP server with `BoardURL = <public>/` and the board with `Public = public`; `runHosted` uses it, so links and the board's allowed origin both come from the one setting. Hosted tests now build through the same helper and cover links per configured address, Claude's `move` reaching an open board (measured about 1 ms in-process, asserted under 2 s), a board `move` visible in Claude's `get_state`, the public Host/origin accepted and a foreign origin refused, and the desktop server returning no `board` key.

**Deviations**: None. Verified end to end with one binary run twice under different `TCGREPLAY_PUBLIC_URL` values: the `new_game` link followed the setting with no rebuild.

**Files changed**:
- `cmd/tcgreplay/main.go`
- `cmd/tcgreplay/daemon.go`
- `cmd/tcgreplay/hosted.go`
- `cmd/tcgreplay/hosted_test.go`

**Discoveries**: During setup in review mode, `move` needs an explicit `seat` (both over MCP and on the board).

### 2026-10-17 — Task: Document the hosted mode

**What was done**: Added a **Hosted mode (trial).** paragraph after the Runtime paragraph of `TCG Review Playmat Design and Build Plan.md`: the single port with `/mcp` and the board, `-listen` / `-public-url` and their defaults, that the public URL alone drives board links and the board's origin, stderr logging, the 30 s board ping, `make docker-build` / `make docker-run`, push-to-deploy from `main` via `.do/app.yaml` (with the placeholder domain to replace), the claude.ai connector address `https://<domain>/mcp`, and the trial limits. The `cmd/tcgreplay` package comment names `-hosted`.

**Deviations**: The trial limits are also in the `.do/app.yaml` header comment (written in the App Platform task), where an operator deploying will read them.

**Files changed**:
- `TCG Review Playmat Design and Build Plan.md`
- `cmd/tcgreplay/main.go`

**Discoveries**: None.

### 2026-10-17 — Task: Create the App Platform app and its domain (deferred)

**What was done**: Nothing in code. This is the owner's task (human execution), and by the user's decision during the orchestrated run it is **deferred until this spec's branch is merged to `main`**. App Platform builds from `main`, so the app cannot be created before the Dockerfile, `.do/app.yaml` and `-hosted` are there. The task and its acceptance criteria stay unchecked, and so does the "passes App Platform's spec validation" criterion of "Define the App Platform app in the repo", which `doctl apps create` checks in step 2 below.

**Owner's steps after the merge**:
1. In `.do/app.yaml`, replace the placeholder domain `playmat.example.com` with the real domain, and push to `main`.
2. In DigitalOcean, authorise App Platform's GitHub app for `nicholasjackson/tcgreplay`, then create the app with `doctl apps create --spec .do/app.yaml` (or "create from app spec" in the control panel). This validates the spec.
3. In Cloudflare, add a CNAME from the domain to the app's `*.ondigitalocean.app` hostname. Keep it DNS-only until App Platform shows the domain as active with its certificate, then switch it to proxied with SSL/TLS mode "Full (strict)".
4. Check: the dashboard shows a deploy from `main` with auto-deploy on, `https://<domain>/` returns the board, and a POST of an MCP `initialize` to `https://<domain>/mcp` returns a result. Then run the claude.ai connector go/no-go check in the test plan.

**Deviations**: Deferred by the user's decision (option (a)) rather than done in the run.

**Files changed**:
- None

**Discoveries**: A human deployment task that needs the merged code cannot be done inside a worktree run; it belongs after the merge.

### 2026-10-17 — Task: Create the App Platform app and its domain (done after merge)

**What was done**: The owner created the app in the DigitalOcean control panel (not from `.do/app.yaml`) and set up Cloudflare for `tcgreplay.demo.gs`. Pushes to `main` redeploy. The orchestrator pushed the merged `main` (`77e6a6a..d298169`), then set the domain in `.do/app.yaml` to `tcgreplay.demo.gs` and pushed it (`6ad437f`). Checked: `GET https://tcgreplay.demo.gs/` returns 200, and a POST of an MCP `initialize` to `https://tcgreplay.demo.gs/mcp` returns a result with the playmat's instructions.

**Deviations**: The app was created by hand in the console, not from `.do/app.yaml`, so the "passes App Platform's spec validation" criterion of "Define the App Platform app in the repo" stays unchecked.

**Files changed**:
- `.do/app.yaml` (domain)

**Discoveries**: None.
