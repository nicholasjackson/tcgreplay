# Build the playmat UI into the Go package, then the binary.
BIN := bin/tcgreplay

.PHONY: build install web test dev dev-ui clean

# Dev mode runs this checkout's build beside the installed one, with its
# own data dir and UI port, so real games and the desktop app's daemon are
# untouched. Relative, so it matches .mcp.json (both run from the repo root).
DEV_DATA ?= .dev
DEV_UI ?= 127.0.0.1:7778

build: web
	go build -o $(BIN) ./cmd/tcgreplay

# install puts the binary where the Claude desktop app runs it from. Then
# restart the app: its next launch replaces the daemon from the old build.
install: web
	go install ./cmd/tcgreplay

web: web/node_modules
	cd web && npm run build

web/node_modules: web/package.json web/package-lock.json
	cd web && npm ci
	touch web/node_modules

test:
	go vet ./...
	go test ./...

# dev builds and runs the dev daemon in the foreground, logging to the
# terminal; the playmat is at http://$(DEV_UI)/. Claude Code reaches it
# through the tcgreplay-dev server in .mcp.json, whose shim uses the same
# flags. It replaces any dev daemon already running (e.g. one a shim
# started); after a rebuild, rerun it. Connected shims follow the daemon
# to its replacement, so the MCP server stays connected.
dev: build
	$(BIN) -daemon -replace -data $(DEV_DATA) -ui-addr $(DEV_UI) -log-stderr

# dev-ui serves the playmat with hot reload on Vite's port, proxying /ws and
# /api to the dev daemon (run make dev first).
dev-ui: web/node_modules
	cd web && TCGREPLAY_ADDR=$(DEV_UI) npm run dev

clean:
	rm -rf bin internal/webui/dist/app
