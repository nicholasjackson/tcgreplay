# The hosted playmat: builds the board UI, embeds it in a static binary and
# runs `tcgreplay -hosted` on a minimal non-root image. MCP is served at
# /mcp and the board on every other path, on port 8080 (or $PORT).
# Settings: TCGREPLAY_PUBLIC_URL (the address users reach it at), PORT,
# TCGREPLAY_GOOGLE_CLIENT_ID and TCGREPLAY_GOOGLE_CLIENT_SECRET (required:
# users sign in with Google), TCGREPLAY_RATE_LIMIT, TCGREPLAY_MAX_USERS.

# Build the board UI into the Go package (see web/vite.config.js).
FROM node:22 AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Build a static binary with the UI embedded.
FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/webui/dist/app internal/webui/dist/app
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/tcgreplay ./cmd/tcgreplay

# Run as the distroless nonroot user. CA certificates are included for the
# runtime TCGdex fetches; games and the card cache go under /home/nonroot.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/tcgreplay /usr/local/bin/tcgreplay
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/tcgreplay", "-hosted"]
