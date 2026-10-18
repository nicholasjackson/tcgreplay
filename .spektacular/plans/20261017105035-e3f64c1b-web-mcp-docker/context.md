---
created_date: "2026-10-17"
document_status: final
closed_date: "2026-10-17"
---

# Context: 20261017105035-e3f64c1b-web-mcp-docker

## Current State Analysis

- **Run modes** (`cmd/tcgreplay/main.go:54-97`): the default is a stdio shim (`cmd/tcgreplay/shim.go`) relaying to a shared daemon (`cmd/tcgreplay/daemon.go:240-334`) that serves MCP sessions over a Unix socket, all sharing one `game.Manager` and one `*mcp.Server` (`newServer`, `main.go:100-106`); `-no-daemon` serves MCP over stdio in-process (`main.go:108-118`). There is no HTTP MCP transport today.
- **Board** (`internal/webui`): an embedded Svelte app (`server.go:22-26`) plus `/ws` (live state push and verbs, `hub.go:306-365`) and `/api/card` (`server.go:123-146`), served on `-ui-addr` (default `127.0.0.1:7777`). It is locked to loopback: `localOnly` rejects non-loopback `Host` (`server.go:87-102`) and the websocket accepts only loopback origins (`hub.go:307-309`), pinned by `TestRejectsForeignHostAndOrigin` (`webui_test.go:254-295`). The hub has no keep-alive; the client reconnects with backoff and reloads full state on connect (`web/src/lib/ws.js:17-23`, `hub.go:345-347`).
- **MCP tools** (`internal/mcpserver/server.go`): 44 tools registered on one server; game-opening tools return maps with `game`, `head`, `mode` etc. (`:314-366`) and no link. The client name sets the assistant label (`:39-43`); `claude-ai` already maps to "Claude" (`server_test.go:177-186`).
- **Card data** (`internal/cards/cards.go`): fetched from `https://api.tcgdex.net/v2/en` on demand and cached under `os.UserCacheDir()/tcgreplay`; images are TCGdex CDN URLs the browser loads directly.
- **Build** (`Makefile`): `make build` runs the Vite build into `internal/webui/dist/app`, then `go build`. No Dockerfile, `.dockerignore` or deployment config exist.
- **Tests**: Go `testing` + `httptest`; MCP tests use in-memory transports (`server_test.go:17-32`); live TCGdex tests are gated by `TCGREPLAY_LIVE=1` (`internal/deck/live_test.go:13-16`).

## Per-Task Technical Notes

Requirement-to-repo resolution: every requirement lands in the single registered repo `tcgreplay` (root `/home/nicj/code/github.com/nicholasjackson/tcgreplay`).

| Requirement | Task(s) | Files |
|---|---|---|
| Remote connector, One address | Serve the playmat over HTTP in a hosted mode; Create the App Platform app and its domain | `cmd/tcgreplay/main.go`, `cmd/tcgreplay/hosted.go` |
| Full tool parity | Serve the playmat over HTTP in a hosted mode; Run the MCP tool tests over the hosted transport | `internal/mcpserver/server_test.go`, `cmd/tcgreplay/hosted_test.go` |
| Board link, Configurable public address | Add board links to game-opening tool results; Wire the public address through hosted mode | `internal/mcpserver/server.go`, `cmd/tcgreplay/hosted.go` |
| Live board, Board interaction | Let the board serve a public address and stay connected; Wire the public address through hosted mode | `internal/webui/server.go`, `internal/webui/hub.go` |
| Container deployment, Card data when hosted | Build the hosted playmat as a container image | `Dockerfile`, `.dockerignore`, `Makefile` |
| Push to deploy | Define the App Platform app in the repo; Create the App Platform app and its domain | `.do/app.yaml` |
| Desktop unaffected | all agent tasks (zero-value switches); regression tests | `internal/webui/webui_test.go`, `internal/mcpserver/server_test.go` |

### Task: Serve the playmat over HTTP in a hosted mode

**File changes**:
- `cmd/tcgreplay/main.go:36-39` — add `publicURL, listen string` to `config`. `config.args()` (`:42-44`) is for spawning the daemon and stays unchanged.
- `cmd/tcgreplay/main.go:55-65` — add flags: `-hosted` (bool, "serve MCP over HTTP at /mcp and the playmat on one address, for a container"), `-listen` (default `""`, resolved by `hostedListen`), `-public-url` (default `os.Getenv("TCGREPLAY_PUBLIC_URL")`).
- `cmd/tcgreplay/main.go:72-78` — when `-hosted`, skip the data-dir log file and keep logging on stderr (containers collect stderr).
- `cmd/tcgreplay/main.go:86-93` — add `case *hosted: err = runHosted(ctx, c)` as the first case.
- `cmd/tcgreplay/main.go:1-11` — leave the package comment for the Document task.
- `cmd/tcgreplay/hosted.go` (new) —
  - `hostedListen(flag string) string`: flag if set; else `":"+$PORT` if `PORT` set; else `":8080"`.
  - `hostedPublicURL(raw, listen string) (*url.URL, error)`: empty → `http://localhost:<port from listen>` and `log.Printf` a warning; else `url.Parse`, require scheme `http`/`https` and a host, reject a path other than `""` or `"/"`, query or fragment; normalise to no trailing slash.
  - `hostedHandler(srv *mcp.Server, ui *webui.Server) http.Handler`: `mux := http.NewServeMux()`; `mux.Handle("/mcp", mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, &mcp.StreamableHTTPOptions{SessionTimeout: 4 * time.Hour, DisableLocalhostProtection: true}))`; `mux.Handle("/", ui.Handler())`.
  - `runHosted(ctx, c)`: `m, srv := newServer(c)` (`main.go:100-106`); `m.Resume()` with the same log-on-error as `runInProcess` (`main.go:111-113`); `ui := webui.New(m, state.Seat(c.seat).Other())`; `ui.StartHub(ctx)` (`internal/webui/server.go:73-74`); `http.Server{Handler: hostedHandler(srv, ui), ReadHeaderTimeout: 10 * time.Second}` (no WriteTimeout: SSE and websockets are long-lived); `net.Listen("tcp", listen)`; shutdown on `ctx.Done()` with a 5 s timeout, like `webui.Server.Run` (`internal/webui/server.go:48-61`); log `tcgreplay <version> hosted: listening on <addr>, public <url>, data <dir>`. Public URL is parsed here but only passed on in the Wire task.
- `cmd/tcgreplay/hosted_test.go` (new) — table tests for `hostedListen` (`t.Setenv("PORT", ...)`) and `hostedPublicURL` (fallback, valid https host, trailing slash normalised, rejects `example.com`, `https://x/y`, `ftp://x`); `TestHostedHandlerServesMCPAndBoard`: `game.NewManager(t.TempDir(), nil)`, `(&mcpserver.Server{M: m, Version: "test"}).MCP()`, `webui.New(m, state.P1)` + `StartHub`, `httptest.NewServer(hostedHandler(...))`; connect `mcp.NewClient(...).Connect(ctx, &mcp.StreamableClientTransport{Endpoint: ts.URL + "/mcp"}, nil)`, call `ping`, `ListTools`; compare names and `InputSchema` JSON against an in-memory session (`mcp.NewInMemoryTransports`, as `internal/mcpserver/server_test.go:17-32`); `GET ts.URL + "/"` with `Host: localhost` returns the index (board guards are still loopback until the Wire task).

**Complexity**: Medium
**Token estimate**: ~35k tokens
**Agent strategy**: Single agent, sequential execution (flags, then `hosted.go`, then tests).

### Task: Run the MCP tool tests over the hosted transport

**File changes**:
- `internal/mcpserver/server_test.go:17-32` — replace `connect(t)` with `transports` (a slice of `{name string; dial func(t, srv *mcp.Server) mcp.Transport}`): `inmemory` (today's `mcp.NewInMemoryTransports`, connecting the server side) and `http` (`httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil))`, returning `&mcp.StreamableClientTransport{Endpoint: ts.URL}`, `t.Cleanup(ts.Close)`); `connect(t, tr)` builds the manager and server as today and dials through `tr`. Add `forEachTransport(t, func(t *testing.T, cs *mcp.ClientSession, m *game.Manager))` running `t.Run(tr.name, ...)`.
- `internal/mcpserver/server_test.go:49-122` — `TestMCPRoundTrip` body runs under `forEachTransport`. The `m.Assistant() != "Codex"` wait (`:55-60`) must still pass over HTTP (stateful sessions keep the client info).
- `internal/mcpserver/server_test.go:124-175` — `TestImportLogTool` body runs under `forEachTransport`.
- `internal/mcpserver/server_test.go:177-186` — `TestAssistantName` is pure; unchanged.
- No production code changes.

**Complexity**: Low
**Token estimate**: ~15k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Build the hosted playmat as a container image

**File changes**:
- `Dockerfile` (new, repo root) — three stages:
  1. `FROM node:22 AS web`; `WORKDIR /src/web`; copy `web/package.json`, `web/package-lock.json`; `npm ci`; copy `web/`; `npm run build` (Vite writes to `/src/internal/webui/dist/app`, per `web/vite.config.js:13-16`).
  2. `FROM golang:1.27 AS build` (matches `go.mod:3`, Go 1.27.2); `WORKDIR /src`; copy `go.mod`, `go.sum`; `go mod download`; copy the repo; `COPY --from=web /src/internal/webui/dist/app internal/webui/dist/app`; `ARG VERSION=dev`; `CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o /out/tcgreplay ./cmd/tcgreplay` (`main.version` is `cmd/tcgreplay/main.go:34`; `syscall.Flock` in `daemon.go` is fine on linux with CGO off).
  3. `FROM gcr.io/distroless/static-debian12:nonroot`; copy the binary to `/usr/local/bin/tcgreplay`; `EXPOSE 8080`; `ENTRYPOINT ["/usr/local/bin/tcgreplay", "-hosted"]`. Data and cache default to `/home/nonroot/.local/share/tcgreplay` and `/home/nonroot/.cache/tcgreplay` via `game.DefaultDataDir` (`internal/game/manager.go:69-78`) and `cards.DefaultCacheDir` (`internal/cards/cards.go:189-194`), writable by the nonroot user. CA certificates come with the distroless base, for `https://api.tcgdex.net` (`internal/cards/cards.go:181`).
- `.dockerignore` (new) — `.git`, `bin`, `.dev`, `web/node_modules`, `internal/webui/dist/app`, `.spektacular`, `skills/spek-*`. Must **not** exclude `skills/tcg-*` (embedded by `skills/skills.go:10-11`) or `internal/webui/dist/placeholder`.
- `Makefile:5` — add `docker-build docker-run` to `.PHONY`; add `IMAGE ?= tcgreplay`, `PUBLIC_URL ?= http://localhost:8080`; `docker-build: ; docker build -t $(IMAGE) .`; `docker-run: ; docker run --rm -p 8080:8080 -e TCGREPLAY_PUBLIC_URL=$(PUBLIC_URL) $(IMAGE)`, each with a one-line comment in the file's existing style.
- Verify locally: `make docker-build`, `make docker-run`, `curl -s localhost:8080/` returns the built app's `index.html` (not the placeholder), `curl` an MCP `initialize` POST to `/mcp`.

**Complexity**: Low
**Token estimate**: ~15k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Define the App Platform app in the repo

**File changes**:
- `.do/app.yaml` (new) — App Platform app spec:
  ```yaml
  name: tcgreplay
  region: lon
  services:
    - name: playmat
      github:
        repo: nicholasjackson/tcgreplay
        branch: main
        deploy_on_push: true
      dockerfile_path: Dockerfile
      source_dir: /
      http_port: 8080
      instance_count: 1
      instance_size_slug: apps-s-1vcpu-0.5gb
      health_check:
        http_path: /
      envs:
        - key: TCGREPLAY_PUBLIC_URL
          value: ${APP_URL}
          scope: RUN_TIME
  domains:
    - domain: playmat.example.com # replace with the owner's domain before creating the app
      type: PRIMARY
  ```
  `${APP_URL}` is App Platform's bindable variable for the app's live URL (the primary domain once set), so the public address is never hard-coded twice. `instance_count: 1` honours the single-instance constraint. A leading comment says: create once with `doctl apps create --spec .do/app.yaml`, apply later spec edits with `doctl apps update <app-id> --spec .do/app.yaml`; code deploys need only a push to `main`.
- Validate with `doctl apps spec validate .do/app.yaml` if `doctl` is available (it is not installed on the planning machine); otherwise check field names against the App Platform app spec reference.

**Complexity**: Low
**Token estimate**: ~8k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Create the App Platform app and its domain

**What the person does**:
1. Replace the placeholder domain in `.do/app.yaml` with the owner's real domain (e.g. a subdomain of the owner's Cloudflare-managed zone) and push to `main`.
2. In DigitalOcean, authorise App Platform's GitHub app for `nicholasjackson/tcgreplay`, then create the app from the spec (`doctl apps create --spec .do/app.yaml`, or the control panel's "create from app spec").
3. In Cloudflare, add a `CNAME` for the domain to the app's `*.ondigitalocean.app` hostname. Keep it DNS-only until App Platform shows the domain as active with its certificate, then switch it to proxied, with SSL/TLS mode "Full (strict)".
4. Leave `TCGREPLAY_PUBLIC_URL` as `${APP_URL}` (it resolves to the primary domain).

**How it is checked**: the App Platform dashboard shows a successful deploy from `main` with auto-deploy on; `https://<domain>/` returns the board page; a POST of an MCP `initialize` request to `https://<domain>/mcp` returns a result. The claude.ai connector check and the go/no-go are manual items in the test plan, not part of this task.

### Task: Let the board serve a public address and stay connected

**File changes**:
- `internal/webui/server.go:29-34` — add `Public *url.URL` to `Server` with a doc comment (hosted mode: public address; nil means desktop, loopback-only).
- `internal/webui/server.go:65-71` — `Handler()`: wrap with `localOnly` only when `s.Public == nil`; return `mux` otherwise.
- `internal/webui/hub.go:306-309` — build origin patterns from `[]string{"localhost:*", "127.0.0.1:*", "[::1]:*"}` plus `s.Public.Host` when set (`coder/websocket` matches patterns against the Origin's host).
- `internal/webui/hub.go:163-165` — add `var pingInterval = 30 * time.Second` (a package var so tests can shorten it).
- `internal/webui/hub.go:328-343` — in the writer goroutine add a `time.NewTicker(pingInterval)` case: `pctx, c := context.WithTimeout(ctx, 10*time.Second); err := conn.Ping(pctx)`; on error `cancel()` and return, as a failed write does. `conn.Ping` needs the read loop running (`hub.go:349-364`), which it is.
- `internal/webui/webui_test.go:254-295` — keep `TestRejectsForeignHostAndOrigin` unchanged (desktop guard). Add `TestPublicAddress`: `setup` variant with `s.Public` = `https://playmat.example`; `GET /` with `Host: playmat.example` → 200; websocket dial with `Origin: https://playmat.example` succeeds and receives a `state` message; `Origin: https://evil.example` fails. Add `TestPing`: set `pingInterval` to 50 ms (restore in cleanup), dial, read in a goroutine (coder/websocket answers pings during reads) and assert the connection stays open past several intervals; then a client that never reads is closed by the server within a few intervals (dial with a raw `net.Conn` or stop reading and assert server-side client count drops).

**Complexity**: Medium
**Token estimate**: ~25k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Add board links to game-opening tool results

**File changes**:
- `internal/mcpserver/server.go:18-25` — add `BoardURL string` to `Server` (doc: hosted mode's board address; empty on the desktop).
- `internal/mcpserver/server.go:37-38` — instructions: `instructions` when `BoardURL == ""`; otherwise `instructions + "\n- Results of import_log, new_game, load_game and set_opponent_deck carry a board link: give it to the player so they can watch and act on the live board."`.
- `internal/mcpserver/server.go:323` (`new_game`), `:335` (`import_log`), `:354` (`set_opponent_deck`), `:365` (`load_game`) — build the result map into a variable and add `"board": s.BoardURL` when non-empty, via a small helper `s.withBoard(map[string]any) map[string]any`.
- `internal/mcpserver/server_test.go` — `TestBoardLinks`: `connect` with a server whose `BoardURL` is `https://playmat.example/`; `new_game` and `import_log` results (and `load_game` of the imported id, and `set_opponent_deck` on it with a 60-card list, e.g. `testdata/decks/mega-lucario-ex-v12.txt`) each have `"board": "https://playmat.example/"`; `InitializeResult.Instructions` contains "board link". `TestNoBoardLinksOnDesktop`: default server → no `"board"` key in those results, instructions equal the `instructions` constant.

**Complexity**: Low
**Token estimate**: ~15k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Wire the public address through hosted mode

**File changes**:
- `cmd/tcgreplay/hosted.go` (`runHosted`) — set `ui.Public = publicURL` before serving, and build the MCP server with `BoardURL: publicURL.String() + "/"`. Since `newServer` (`cmd/tcgreplay/main.go:100-106`) constructs `mcpserver.Server` inline, give it an optional board URL (e.g. `newServer(c config, boardURL string)`, desktop callers pass `""`) rather than duplicating the construction.
- `cmd/tcgreplay/hosted.go` — `hostedHandler` is unchanged; tests build it the same way runHosted does via a shared `newHosted(c config, public *url.URL) (*game.Manager, *mcp.Server, *webui.Server)` helper so tests and production wire identically.
- `cmd/tcgreplay/daemon.go:297`, `cmd/tcgreplay/main.go:109` — pass `""` for the board URL (desktop unchanged).
- `cmd/tcgreplay/hosted_test.go` — with public `https://playmat.example`:
  - `TestHostedBoardLinks`: `import_log` (testdata log + deck) and `new_game` over `/mcp` return `board` starting with `https://playmat.example/`; a second handler built with `https://other.example` returns links on that host (setting drives links, no rebuild).
  - `TestHostedBoardFollowsClaude`: open the board websocket at `ts.URL + "/ws"` with `Origin: https://playmat.example` and `Host` left as the test server's; read the initial `state`; call `move` over MCP (review mode or Claude's seat, as `internal/mcpserver/server_test.go:92-98` does); assert a `state` message whose snapshot reflects the move arrives within 2 s.
  - `TestHostedBoardMoveReachesClaude`: send a `move` verb on the websocket (shape as `internal/webui/webui_test.go:125-138` `send`); then `get_state` over MCP shows the card at the new location.
  - Request `GET /` with `Host: playmat.example` → 200.

**Complexity**: Medium
**Token estimate**: ~30k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Document the hosted mode

**File changes**:
- `TCG Review Playmat Design and Build Plan.md:299` — extend the **Runtime** paragraph: `tcgreplay -hosted` serves MCP (Streamable HTTP) at `/mcp` and the board at `/` on one port (`-listen`, default `:$PORT` or `:8080`); `-public-url` / `TCGREPLAY_PUBLIC_URL` sets the address used for board links and the board's allowed origin; `make docker-build` / `make docker-run`; `.do/app.yaml` deploys on push to `main` to App Platform; add the connector in claude.ai as `https://<domain>/mcp`. State the trial limits: no login, one shared game for everyone, games and card cache lost on every deploy or restart.
- `cmd/tcgreplay/main.go:1-11` — package comment: add one sentence on `-hosted`.

**Complexity**: Low
**Token estimate**: ~6k tokens
**Agent strategy**: Single agent, sequential execution.

## Testing Strategy

Tests use Go's standard `testing` with `httptest`, run by `make test` (`go vet ./...` + `go test ./...`). No new test needs the network.

- **Serve the playmat over HTTP in a hosted mode** — unit tests for `hostedListen` and `hostedPublicURL` (env and fallback resolution, validation errors); `TestHostedHandlerServesMCPAndBoard` proves `/mcp` and the board share one server and that the HTTP tool list and schemas equal the in-memory ones.
- **Run the MCP tool tests over the hosted transport** — `TestMCPRoundTrip` and `TestImportLogTool` run as `inmemory` and `http` subtests; the assistant-name wait proves stateful sessions keep client info over HTTP.
- **Build the hosted playmat as a container image** — no Go tests; verified by building and running the image locally (`make docker-build`, `make docker-run`), loading `/` and POSTing an MCP `initialize` to `/mcp`.
- **Define the App Platform app in the repo** — spec validation with `doctl apps spec validate` where available; otherwise checked against the app spec reference.
- **Create the App Platform app and its domain** (human) — checked by a successful deploy and HTTPS responses from `/` and `/mcp` on the domain.
- **Let the board serve a public address and stay connected** — existing `TestRejectsForeignHostAndOrigin` unchanged (desktop guard); new `TestPublicAddress` (public host accepted, public origin accepted, foreign origin refused); `TestPing` with a shortened `pingInterval` (connection kept alive while reading; unresponsive client dropped).
- **Add board links to game-opening tool results** — `TestBoardLinks` (all four tools carry `board`, instructions mention it) and `TestNoBoardLinksOnDesktop` (no `board` key, instructions unchanged).
- **Wire the public address through hosted mode** — `TestHostedBoardLinks` (links start with the configured address; a different address yields different links), `TestHostedBoardFollowsClaude` (MCP `move` reaches an open board websocket within 2 s — the automated half of the 2-second metric), `TestHostedBoardMoveReachesClaude` (websocket `move` visible in MCP `get_state`).
- **Document the hosted mode** — no tests.

Manual items (success metrics and reviews needing claude.ai, a phone, Cloudflare and App Platform) are listed in plan.md § Testing Approach, each flagged **Manual — captured in the implementation test plan**; the implement workflow turns them into the test plan.

## Project References

- **Repo**: `tcgreplay`, root `/home/nicj/code/github.com/nicholasjackson/tcgreplay` (the only registered repo; every task lands here).
- **Spec**: `20261017105035-e3f64c1b-web-mcp-docker` (epic `20261017105033-91f90842-hosted-playmat`, first spec; memory-store-record and google-login depend on it).
- **Design documents**: none referenced by the spec.
- **Knowledge entries**: none (the `tcgreplay` knowledge store is empty).
- **Project design doc** (in-repo, not a Spektacular design source): `TCG Review Playmat Design and Build Plan.md` — its Runtime paragraph (`:299`) is updated by the Document task.
- **Library**: `github.com/modelcontextprotocol/go-sdk` v1.6.0 — `mcp/streamable.go` (`NewStreamableHTTPHandler`, `StreamableHTTPOptions`, `StreamableClientTransport`).

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

All nine tasks are Low or Medium and run as a single agent each. "Serve the playmat over HTTP in a hosted mode", "Run the MCP tool tests over the hosted transport", "Let the board serve a public address and stay connected" and "Add board links to game-opening tool results" have no dependencies on each other and can run in parallel; the container, app definition, wiring and documentation tasks follow their dependencies.

## Migration Notes

No data migration: game files and the card cache keep their formats, and the desktop modes keep their flags, defaults and data directories. Deploying is additive: the owner creates the App Platform app once (human task), after which a push to `main` deploys. A deploy discards the running container's games and card cache (accepted by the spec); re-import the game after a deploy.

## Performance Considerations

- The spec's latency target (a Claude move reaches the open board within 2 s) is met by the existing hub, which pushes on every game update; the hosted path adds only HTTP and proxy hops. An automated test asserts the 2 s bound in-process; the deployed bound is a manual check.
- Websocket pings every 30 s per open board are negligible; MCP sessions idle out after 4 h to bound memory.
- A cold card cache makes the first import slower (TCGdex fetches, prefetched per game); every deploy starts cold. Tool calls must still finish well under Cloudflare's ~100 s limit; if an import with a cold cache approaches it, see Open Questions.
- One small instance (`apps-s-1vcpu-0.5gb`) is ample for a single-user trial.
