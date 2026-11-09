# The hosted playmat: builds the board UI and the public website, embeds
# both in a static binary and runs `tcgreplay -hosted` on a minimal
# non-root image. MCP is served at /mcp, the board at /board and the
# website on every other path, on port 8080 (or $PORT).
# Settings: TCGREPLAY_PUBLIC_URL (the address users reach it at), PORT,
# TCGREPLAY_GOOGLE_CLIENT_ID and TCGREPLAY_GOOGLE_CLIENT_SECRET (required:
# users sign in with Google), TCGREPLAY_RATE_LIMIT, TCGREPLAY_MAX_USERS,
# TCGREPLAY_CARDSETS_URL (where card sets are published; default the
# project's CDN) and TCGREPLAY_CARDSETS_INTERVAL (how often to check for a
# publish; default 5m), and optionally TCGREPLAY_SPACES_ENDPOINT,
# TCGREPLAY_SPACES_BUCKET, TCGREPLAY_SPACES_KEY and TCGREPLAY_SPACES_SECRET
# (all four: copy card images to a DigitalOcean Spaces bucket and serve
# them from its CDN), TCGREPLAY_ADMINS (the Google accounts that may check
# for published card sets and clear image copies at /admin) and
# TCGREPLAY_SPACES_CDN_TOKEN (a DigitalOcean API token, so an image clear
# also purges the bucket's CDN). The image carries no card data: card sets
# are loaded from the published copy at start.

# Build the board UI into the Go package (see web/vite.config.js).
FROM node:22 AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Build the website into its Go package (see site/astro.config.mjs). The
# build reads the guides, the limits in .do/app.yaml and the example deck,
# so they are copied beside it. Astro needs Node 22.23.4 or newer.
FROM node:22.23.4 AS site
WORKDIR /src/site
COPY site/package.json site/package-lock.json ./
RUN npm ci
COPY site/ ./
COPY internal/guides /src/internal/guides
COPY .do/app.yaml /src/.do/app.yaml
COPY testdata/decks /src/testdata/decks
RUN npm run build

# Build a static binary with the UI and the website embedded.
FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/webui/dist/app internal/webui/dist/app
COPY --from=site /src/internal/site/dist/app internal/site/dist/app
ARG VERSION=dev
# nodynamic keeps the WebP encoder (github.com/gen2brain/webp) from loading
# a system libwebp through purego, which would make the binary dynamically
# linked even with CGO_ENABLED=0: distroless/static has no dynamic loader,
# so such a binary fails to start with "no such file or directory".
RUN CGO_ENABLED=0 go build -tags nodynamic -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/tcgreplay ./cmd/tcgreplay

# Run as the distroless nonroot user. CA certificates are included for
# loading the published card sets and copying card images over HTTPS; the
# local copy of the card sets goes under /home/nonroot.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/tcgreplay /usr/local/bin/tcgreplay
# Fail the build, not the deploy, if the binary cannot start here (for
# example because it came out dynamically linked).
RUN ["/usr/local/bin/tcgreplay", "-version"]
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/tcgreplay", "-hosted"]
