---
created_date: "2026-10-17"
document_status: final
closed_date: "2026-10-17"
---

# Research: 20261017105035-e3f64c1b-web-mcp-docker

## Alternatives considered and rejected

### Option A: A separate hosted binary

A new command (e.g. `cmd/tcgreplay-web`) for the hosted playmat.

**Rejected**: the spec's first constraint requires a mode of the existing `tcgreplay` binary; `cmd/tcgreplay/main.go:86-93` already switches between modes and a fourth case fits it.

### Option B: The daemon in the container, fronted by an HTTP relay or reverse proxy

Run `-daemon` (MCP on a Unix socket, board on its own TCP port, `cmd/tcgreplay/daemon.go:284-303`) and put an HTTP relay or a proxy (Caddy/nginx) in front to merge them on one port.

**Rejected**: two hops and an extra process for no benefit in a single-process container; App Platform exposes one `http_port`, and one `http.ServeMux` can mount `/mcp` beside the board's `/`, `/ws`, `/api/card` (`internal/webui/server.go:65-71`).

### Option C: The legacy SSE transport

`mcp.NewSSEHandler` (go-sdk `mcp/sse.go`).

**Rejected**: Streamable HTTP is the current MCP transport, claude.ai supports it, and the spec prefers the official library's web transport. SSE needs a long-lived GET stream for every response, fragile under Cloudflare's ~100 s idle limit.

### Option D: Stateless Streamable HTTP

`StreamableHTTPOptions.Stateless`.

**Rejected**: stateless sessions use default initialize params, so the `InitializedHandler` that names the assistant from the client info (`internal/mcpserver/server.go:39-43`) would never see claude.ai's client name. A single instance keeps sessions in memory without trouble.

### Option E: Server-side MCP keep-alive pings

`ServerOptions.KeepAlive` (go-sdk `mcp/server.go:78-81`).

**Rejected** for this trial: a ping the peer fails to answer closes the session, and a server→client request needs the client's standalone GET stream, which claude.ai is not guaranteed to hold open. Tool calls are short POSTs and do not hit the idle limit.

### Option F: Board links from the request's Host or X-Forwarded-Host

**Rejected** by the spec's Technical Approach: the host is not trustworthy behind Cloudflare. Links come from the configured public URL.

### Option G: Board links carrying the game id (`/?game=<id>`)

**Rejected**: there is one shared current game (`game.Manager.Current`, used by `internal/webui/hub.go:270`), so the board root always shows the game the tool just opened; the later per-user-games spec ties the board to the signed-in account's single game, not to an id in the link. An id the board ignores would mislead.

### Option H: GitHub Actions building and pushing to a registry, then deploying

**Rejected**: App Platform's GitHub integration with `deploy_on_push: true` builds the repo's Dockerfile on push to `main` with no pipeline or registry credentials, and the spec prefers the app definition in the repo (`.do/app.yaml`).

### Option I: Baking a card cache into the image

**Rejected**: the spec prefers runtime TCGdex fetches; `cards.Client` already fetches and caches on demand (`internal/cards/cards.go:179-240`). Images are loaded by the browser from `assets.tcgdex.net` (`internal/webui/server.go:118-119`), so the image only needs CA certificates.

### Option J: An Alpine runtime image

**Rejected** in favour of `gcr.io/distroless/static-debian12:nonroot`: static Go binary (`CGO_ENABLED=0`), CA certs included, writable `$HOME` (`/home/nonroot`) for the default data and cache dirs (`internal/game/manager.go:69-78`, `internal/cards/cards.go:189-194`), no shell to maintain.

## Chosen approach — evidence

- go-sdk v1.6.0 (`go.mod:5`) ships `mcp.NewStreamableHTTPHandler(getServer, *StreamableHTTPOptions)` (`$GOMODCACHE/github.com/modelcontextprotocol/go-sdk@v1.6.0/mcp/streamable.go:127-207`) with `SessionTimeout`, `JSONResponse`, `DisableLocalhostProtection`; the handler is a plain `http.Handler`, so it mounts on the board's mux.
- The handler's DNS-rebinding guard rejects non-loopback `Host` on a loopback listener (`streamable.go:250-255`); hosted mode sets `DisableLocalhostProtection` so a tunnel to localhost (or a local `docker run` behind a proxy) is not refused.
- One `*mcp.Server` already serves many sessions in the daemon (`cmd/tcgreplay/daemon.go:297-332`, `srv.Connect` per shim connection); `getServer` can return that same server for every HTTP session.
- `newServer` (`cmd/tcgreplay/main.go:100-106`) and `webui.New` (`internal/webui/server.go:37-45`) build the manager, MCP server and board; hosted mode reuses both.
- The board's guards are hard-wired to loopback: `localOnly` (`webui/server.go:87-102`) and websocket `OriginPatterns` (`webui/hub.go:307-309`). Hosted mode must let the configured public host through; `TestRejectsForeignHostAndOrigin` (`webui/webui_test.go:254`) pins the desktop behaviour and must stay green.
- The board client already builds its websocket URL from `location` and picks `wss` on HTTPS (`web/src/lib/ws.js:11-12`), and reconnects with backoff on close (`ws.js:17-23`), getting the full state on connect (`hub.go:345-347`). So the board needs no client change to work behind HTTPS; a server-side websocket ping keeps it under Cloudflare's idle limit.
- `coder/websocket` (`go.mod`) offers `Conn.Ping(ctx)`; the hub's per-client writer goroutine (`hub.go:328-343`) is where a ticker fits.
- Tool results for `new_game`, `import_log`, `load_game`, `set_opponent_deck` are built as maps in `internal/mcpserver/server.go:314-366`; adding a `board` key is local to those handlers.
- MCP tests use `connect(t)` with in-memory transports (`internal/mcpserver/server_test.go:17-32`); go-sdk's `mcp.StreamableClientTransport` lets the same tests run against an `httptest.Server`.
- The web build writes into `internal/webui/dist/app` for `go:embed` (`web/vite.config.js:13-16`, `webui/server.go:22-26`); a Node build stage followed by a Go build stage produces a self-contained binary.

## Files examined

- `tcgreplay:cmd/tcgreplay/main.go:36-97` — config struct, flags, mode switch (daemon / no-daemon / shim); logging to the data dir unless `-log-stderr`.
- `tcgreplay:cmd/tcgreplay/main.go:99-135` — `newServer`, `runInProcess`, `startUI`.
- `tcgreplay:cmd/tcgreplay/daemon.go:240-358` — daemon: lock, Unix socket, one MCP server for many sessions, `keepUI`.
- `tcgreplay:cmd/tcgreplay/shim.go:21-60` — stdio relay to daemon; untouched by this plan.
- `tcgreplay:internal/webui/server.go:28-102` — board server, mux routes, embedded assets, `localOnly` host guard.
- `tcgreplay:internal/webui/hub.go:306-365` — websocket accept with loopback origin patterns, writer goroutine, state on connect.
- `tcgreplay:internal/webui/webui_test.go:79-140,254-295` — `setup`, `dial`, foreign host/origin rejection test.
- `tcgreplay:internal/mcpserver/server.go:17-90,308-366` — server struct, `MCP()`, `add`, game-opening tool handlers.
- `tcgreplay:internal/mcpserver/server_test.go:17-186` — `connect`/`call` helpers, `TestMCPRoundTrip`, `TestImportLogTool`, `TestAssistantName` (already expects `claude-ai` → Claude).
- `tcgreplay:internal/cards/cards.go:165-240` — TCGdex client, `DefaultCacheDir` (`os.UserCacheDir`), runtime fetch + disk cache.
- `tcgreplay:internal/game/manager.go:69-78` — `DefaultDataDir` from `XDG_DATA_HOME` or `$HOME`.
- `tcgreplay:web/src/lib/ws.js:1-34` — websocket URL from `location`, reconnect with backoff.
- `tcgreplay:web/vite.config.js` — build output into the Go package.
- `tcgreplay:Makefile` — `build`, `web`, `test`, `dev` targets; no docker target yet.
- `tcgreplay:.gitignore`, `internal/webui/.gitignore`, `web/.gitignore` — `bin`, `.dev`, `dist/app/`, `node_modules/` ignored; no `.dockerignore`.
- `tcgreplay:TCG Review Playmat Design and Build Plan.md:299` — the "Runtime" paragraph describing the run modes; needs a hosted-mode sentence.
- `$GOMODCACHE/.../go-sdk@v1.6.0/mcp/streamable.go:127-255` — handler options and localhost protection.
- `$GOMODCACHE/.../go-sdk@v1.6.0/mcp/server.go:78-81` — `KeepAlive` semantics (closes session on failed ping).

## External references

- MCP specification, Streamable HTTP transport (2025-06-18 / 2025-11-25) — the transport claude.ai remote connectors speak; single endpoint for POST/GET, `Mcp-Session-Id`.
- Anthropic help centre, "Building custom connectors via remote MCP servers" — claude.ai custom connectors accept authless or OAuth remote servers over Streamable HTTP or SSE, reached from Anthropic's cloud, so the URL must be public HTTPS. Not verified against the live product; the first milestone proves it (spec risk).
- DigitalOcean App Platform app spec reference — `services[].github {repo, branch, deploy_on_push}`, `dockerfile_path`, `http_port`, `instance_count`, `envs`, `health_check`, `domains`. Apps are created from a spec with `doctl apps create --spec` and updated with `doctl apps update --spec`.
- Cloudflare: proxied HTTP connections, websockets included, are closed after ~100 s without data — why the board websocket needs a ping well under that.
- `gcr.io/distroless/static-debian12:nonroot` — static runtime with CA certs and a non-root `$HOME`.

## Prior plans / specs consulted

- No prior plans exist (`spektacular plan file list` is empty).
- Epic `20261017105033-91f90842-hosted-playmat` — this spec is first; memory-store-record and google-login depend on it, per-user-games on those.
- Spec `20261017123240-21493a95-google-login` (historical intent, read only to keep this plan compatible) — sign-in addresses reuse "the public address setting that the hosted mode already provides", so the setting this plan adds must be a single, reusable value on the hosted server.
- Spec `20261017143118-b05aa6b1-per-user-games` (same) — the board will show the signed-in account's one game, not a game named in the link; supports a plain root board link here.

## Open assumptions

- claude.ai accepts an authless Streamable HTTP connector at `https://<host>/mcp` and lists its tools. If it refuses (e.g. demands OAuth), STOP: this is the epic's go/no-go.
- App Platform passes the container's `http_port` traffic through with websocket upgrades intact, and sets `PORT` for the service (the binary also defaults to 8080, matching the app spec, so either way works).
- Cloudflare in front of App Platform (owner's domain, proxied) does not buffer SSE responses from `/mcp` in a way that breaks claude.ai; tool calls return in well under 100 s (import with a cold card cache is the slowest call).
- `golang:1.27` and `node:22` images exist for the build stages (`go.mod` requires Go 1.27.2; locally `go1.27.2`).
- The owner has a DigitalOcean account with the GitHub app authorised for `nicholasjackson/tcgreplay`, and controls DNS for the domain at Cloudflare.

## Drafting assumptions

### Chosen direction: in-process `-hosted` mode on one HTTP mux (architecture)
- **Decision**: Add `tcgreplay -hosted`, which runs the shared game manager and MCP server in-process and serves `/mcp` (go-sdk Streamable HTTP) and the board (`/`, `/ws`, `/api/card`) from one plain-HTTP listener; packaged by a multi-stage Dockerfile and deployed by `.do/app.yaml` with `deploy_on_push` on `main`.
- **Key design decisions**: the public URL setting is the only source for board links and the hosted websocket origin allow-list; board links appear only when the MCP server has a `BoardURL` (hosted), so desktop results are unchanged; board guards relax only when a public URL is configured; websocket pings every 30 s for Cloudflare; stateful MCP sessions without server pings; claude.ai connection proven in the first milestone.
- **Rationale**: Meets every constraint (mode of the binary, Docker, single instance, plain HTTP on the platform port, App Platform push-to-deploy) with the least new code, reusing `newServer` and the board handler.
- **Rejected**: daemon in the container fronted by an HTTP relay to its Unix socket (two hops, more moving parts, no benefit with one process); a reverse proxy combining two listeners (extra process, two ports inside the container); separate binary (violates constraint).

### Board link is the public root URL (discovery)
- **Decision**: Board links are the configured public URL's root (`<public-url>/`), not `/?game=<id>`.
- **Rationale**: One shared current game today; per-user-games later binds the board to the signed-in account's game, not to an id in the link.
- **Rejected**: `/?game=<id>` that the board ignores (misleading); making the board load the named game on open (a side effect on the shared game).

### Stateful Streamable HTTP, no server keep-alive pings (discovery)
- **Decision**: Use `mcp.NewStreamableHTTPHandler` with sessions (not stateless), default SSE responses, no `ServerOptions.KeepAlive`.
- **Rationale**: Stateful keeps the assistant-name handshake working; tool calls are short POSTs so Cloudflare's idle limit does not bite; server pings would close the session if claude.ai holds no GET stream.
- **Rejected**: Stateless mode (loses client info); server KeepAlive (can close sessions); legacy SSE transport.

### Hosted mode disables the SDK's localhost protection and the board's loopback guard (discovery)
- **Decision**: In hosted mode, the MCP handler sets `DisableLocalhostProtection`, the board skips `localOnly`, and websocket origins allow the public URL's host (plus loopback).
- **Rationale**: Hosted mode is public by design (no login in this spec); the guards exist to stop DNS rebinding against a desktop playmat. The board's origin check still blocks other sites from driving it.
- **Rejected**: Trusting the request Host for the allow-list (spec says it cannot be trusted).

### Hosted settings: flags with env-var defaults (architecture)
- **Decision**: `-hosted` mode flag; `-public-url` defaults to `$TCGREPLAY_PUBLIC_URL`; `-listen` defaults to `:$PORT`, else `:8080`. With no public URL, hosted mode falls back to `http://localhost:<port>` and logs a warning. Hosted mode always logs to stderr.
- **Rationale**: App Platform and `docker run -e` configure through env vars; flags keep parity with the other modes; the localhost fallback makes the one-command local run work without extra settings.
- **Rejected**: refusing to start without a public URL (makes the local smoke run need a setting that has an obvious default); env-only configuration (inconsistent with existing flags).

### Board link also on set_opponent_deck (architecture)
- **Decision**: `set_opponent_deck` results carry the board link too, besides import, new game and load game.
- **Rationale**: It rebuilds or creates the current game, which is "creating a game" in the spec's sense, and is the tool the import skill calls right after import.
- **Rejected**: Exactly the three tools named (would leave the rebuilt game without a link).

### Hosted-only server instruction line (architecture)
- **Decision**: In hosted mode the MCP server instructions gain one line telling the assistant to give the player the `board` link; tool descriptions stay unchanged.
- **Rationale**: Keeps the desktop tool list and instructions identical (Desktop unaffected), while making claude.ai surface the link.
- **Rejected**: Editing tool descriptions (changes the desktop tool list text).

### Websocket ping for every board client (architecture)
- **Decision**: The hub sends a websocket ping every 30 s to every client, desktop included.
- **Rationale**: One code path; a ping frame is invisible to the board and harmless on loopback; 30 s is well under Cloudflare's ~100 s idle limit.
- **Rejected**: Hosted-only pings (extra branching for no observable desktop difference); application-level heartbeat messages (needs client changes).

### MCP session idle timeout of 4 hours (architecture)
- **Decision**: `StreamableHTTPOptions.SessionTimeout = 4h`.
- **Rationale**: Bounds memory from abandoned sessions over the trial while comfortably covering a 30-60 minute review with idle gaps.
- **Rejected**: No timeout (unbounded sessions); short timeouts (risk mid-review reconnects, hurting the success metric).

### Conventions: none recorded (architecture)
- **Decision**: No project conventions selected.
- **Rationale**: The knowledge store is empty.
- **Rejected**: n/a.

### App Platform region, size and public URL binding (tasks)
- **Decision**: `region: lon`, `instance_size_slug: apps-s-1vcpu-0.5gb`, `TCGREPLAY_PUBLIC_URL: ${APP_URL}`, health check on `/`.
- **Rationale**: Owner is UK-based (nearest region); smallest size suits a single-user trial; `${APP_URL}` resolves to the primary domain so the address is declared once.
- **Rejected**: Hard-coding the URL in the env (two places to change); larger sizes (cost with no need).

### Owner's domain as a placeholder filled in by the human task (tasks)
- **Decision**: `.do/app.yaml` declares the domain with a marked placeholder; the human setup task replaces it before creating the app.
- **Rationale**: The domain is information only the owner holds and is not needed to write or test any code; keeping it in the spec keeps the deployment reproducible.
- **Rejected**: Asking now (blocks planning on a value only needed at deploy time); leaving domains out of the spec (a later `doctl apps update` would drop the console-added domain).

### Cloudflare DNS-only until the App Platform certificate is issued (tasks)
- **Decision**: The setup task adds the CNAME DNS-only, switches to proxied once App Platform marks the domain active, with SSL mode Full (strict).
- **Rationale**: App Platform's certificate issuance can fail behind the proxy; Full (strict) keeps HTTPS end to end.
- **Rejected**: Proxied from the start (certificate issuance risk).

### Tests for MCP parity live in mcpserver; hosted wiring tests in cmd (tasks)
- **Decision**: Transport-parameterised tool tests use the SDK handler directly in `internal/mcpserver`; hosted-handler, link, latency and board-move tests live in `cmd/tcgreplay`.
- **Rationale**: Keeps the parity task independent of the hosted mode task, so both can start in parallel; the cmd tests cover the real composed handler.
- **Rejected**: A new `internal/hosted` package (extra package for ~100 lines of wiring).

## Rehydration cues

- `spektacular spec file read 20261017105035-e3f64c1b-web-mcp-docker`
- `spektacular epic read 20261017105033-91f90842-hosted-playmat`
- Re-read `cmd/tcgreplay/main.go`, `internal/webui/server.go`, `internal/webui/hub.go`, `internal/mcpserver/server.go:300-370`, `internal/mcpserver/server_test.go:17-47`.
- `grep -n 'type StreamableHTTPOptions' -A60 $(go env GOMODCACHE)/github.com/modelcontextprotocol/go-sdk@v1.6.0/mcp/streamable.go`
- `make test` from the repo root runs `go vet` and `go test ./...`.
