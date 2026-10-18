DONE summary for 20261017105035-e3f64c1b-web-mcp-docker (kept for Step 6 of plan-epic)

Approach: new `tcgreplay -hosted` mode of existing binary; manager + MCP in-process; one plain-HTTP listener: /mcp via go-sdk v1.6.0 Streamable HTTP (stateful, 4h idle), /, /ws, /api/card from existing board handler. Single public URL setting (-public-url / TCGREPLAY_PUBLIC_URL) drives board links and allowed ws origin; Host header never used. Unset => desktop unchanged. Multi-stage Dockerfile node->go->distroless nonroot. .do/app.yaml deploys on push to main only, one instance, public URL from ${APP_URL}. Rejected: separate binary, daemon behind relay, legacy SSE, stateless sessions, server keep-alive pings, Host-header links, ?game=<id> links, CI pipeline+registry, baked card cache, Alpine runtime.

Milestone 1 (go/no-go: claude.ai drives hosted playmat):
- Serve the playmat over HTTP in a hosted mode
- Run MCP tool tests over hosted transport (in-memory and Streamable HTTP)
- Build hosted playmat as container image (Dockerfile, .dockerignore, make docker-build/docker-run)
- Define App Platform app in repo
- HUMAN TASK: Create App Platform app and domain (DO account, GitHub auth, Cloudflare DNS; placeholder domain; CNAME DNS-only first then proxied Full (strict))
Milestone 2 (board link opens live board following Claude):
- Board serves public address & stays connected (Public *url.URL on webui.Server; ws ping 30s)
- Board links on game-opening tool results (BoardURL on mcpserver.Server; `board` key on new_game, import_log, load_game, set_opponent_deck; one hosted-only instruction line)
- Wire public address through hosted mode (tests: 2s move latency, board move reaches Claude)
- Document hosted mode (Runtime paragraph of design doc)

Out of scope: login; in-memory storage & record tool; per-user games; persistence across deploys; skills packaging for claude.ai; Notion; MCP Apps / chat in board; multiple instances, TLS in container; CI/registry; MCP keep-alive & stream resumption; Svelte client changes.

Drafting assumptions: in-process -hosted mode on one mux; board link = public root URL; Streamable HTTP stateful, default SSE, no KeepAlive; hosted disables DisableLocalhostProtection check, skips loopback Host guard, allows public host ws origin; -listen defaults :$PORT else :8080; no public URL => http://localhost:<port> + warning; hosted logs to stderr; set_opponent_deck returns link; instruction line hosted-only, tool descriptions unchanged; ws ping to all clients incl desktop; MCP sessions 4h idle; no conventions (knowledge empty); App Platform region lon, apps-s-1vcpu-0.5gb, ${APP_URL}, health check /; domain placeholder; Cloudflare DNS-only then proxied Full (strict); parity tests in internal/mcpserver, wiring tests in cmd/tcgreplay.

Project-wide rules:
- Public address setting (decides): single -public-url / TCGREPLAY_PUBLIC_URL, scheme+host no path, only source for addresses; google-login must reuse it.
- Board link shape (decides): `board` key = public root URL, hosted only; per-user-games keep or deliberately change.
- Hosted vs desktop switch: optional fields webui.Server.Public, mcpserver.Server.BoardURL; zero value = desktop unchanged; later specs hang hosted-only behaviour on same switch.
- MCP endpoint path: /mcp on same host as board.
- Persistence: game files & card cache in container FS under $HOME; memory-store-record replaces this.
- Deployment: .do/app.yaml, App Platform builds Dockerfile on push to main, one instance; spec changes applied by owner with doctl.
- Tests: make test (go vet + go test); network tests behind TCGREPLAY_LIVE=1.
- Changelog: none.

Manual checks:
- Owner imports & reviews a real game end to end from claude.ai on a phone.
- 30-60 min review session, no reconnects/failed calls caused by hosting.
- Claude's move appears on deployed board behind Cloudflare within 2s (also automated in-process).
- Deploying needs only a push to main.
- Go/no-go outcome recorded.
- claude.ai accepts https://<domain>/mcp; tool list matches desktop.
- Import with opponent deck, then move, rewind, branch, annotate, each read back.
- Board link opens right board on phone.
- Idle 10 min behind Cloudflare, board still shows Claude's next move without reload.
- Changing public address + restart changes links, no rebuild.
- Image builds with one command, starts with one command + settings, board loads.
- Push to main deploys; other branch does not.
- Fresh container: card_text returns text, board shows card image.
- Desktop regression: Claude Code & Desktop launch locally, connect, import a log.

Open for implementation: does claude.ai accept authless Streamable HTTP connector (go/no-go; OAuth would pull google-login forward); Cloudflare/App Platform passing SSE & websocket (fallback JSON responses + shorter ping; hosting changes go to user).

Discoveries for knowledge base:
- Board locked to loopback in two places: localOnly Host check (internal/webui/server.go:87-102) and ws OriginPatterns (internal/webui/hub.go:307-309). Tags: webui, security, hosting.
- go-sdk v1.6.0 Streamable handler rejects loopback-listener requests with non-loopback Host unless DisableLocalhostProtection; server KeepAlive closes session on unanswered ping. Tags: mcp, go-sdk, transport.
- skills/skills.go embeds skills/tcg-*; .dockerignore must not exclude them. Tags: build, docker, skills.
