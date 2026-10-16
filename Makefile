# Build the playmat UI into the Go package, then the binary.
BIN := bin/tcgreplay

.PHONY: build install web test dev clean

build: web
	go build -o $(BIN) ./cmd/tcgreplay

# install puts the binary where the Claude desktop app runs it from.
# Quit the desktop app first; it runs the installed copy.
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

# dev runs Vite with hot reload, proxying /ws and /api to a running binary
# on 127.0.0.1:7777.
dev: web/node_modules
	cd web && npm run dev

clean:
	rm -rf bin internal/webui/dist/app
