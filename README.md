<p align="center"><img src="docs/logo.png" alt="TCG Replay" width="256"></p>

# tcgreplay

A virtual Pokémon TCG playmat that an AI assistant drives over MCP. Paste a
Pokémon TCG Live battle log and the assistant rebuilds the game on a live board,
steps through your turns with you, rewinds to the decisions that mattered, plays
out better lines, and turns your mistakes into drills. You can also just play a
game against it.

It is a playmat, not a rules engine: the assistant runs the game and the board
shows it, live in your browser. It works with Claude and ChatGPT.

## Get started

See **[tcgreplay.demo.gs](https://tcgreplay.demo.gs)** for what it does, and
**[Get started](https://tcgreplay.demo.gs/docs/connect)** to connect it to
claude.ai, ChatGPT or Claude Code. To run it on your own computer with the
Claude desktop app, see
[running it on your desktop](https://tcgreplay.demo.gs/docs/desktop).

## Development

```sh
make build   # build the board UI and the binary into bin/tcgreplay
make test    # Go vet and tests, then the board's and the website's browser tests
make dev     # run a dev daemon beside the installed one (board on :7778)
make dev-ui  # board with hot reload, proxying to the dev daemon
make site      # build the website (site/) into internal/site
make site-dev  # website with live reload
make site-test # build the website and run its browser tests
```

The website lives in `site/` (Astro). It is built into `internal/site` and
served by the hosted playmat at `/`, with the board at `/board`. It reads the
coaching guides, the hosted limits in `.do/app.yaml` and the example deck at
build time, so a guide without a page under `site/src/content/docs/workflows/`
fails the build.

`.mcp.json` points Claude Code at the dev build as `tcgreplay-dev`.

### Card sets

Card data is built ahead of time into `cardsets/`, one readable JSON file per
English set with its hand corrections and import report, from TCGdex and the
malie.io TCG Live export:

```sh
bin/tcgreplay sets import DLR   # import or re-import a set (id or decklist code); -all for every set
bin/tcgreplay sets check        # check every built set; exits 1 on any open item
```

To have the assistant verify a set against the printed cards, ask it to import
the set: its `card-set-import` skill works through the report and records
corrections. See [cardsets/README.md](cardsets/README.md) for the format, the
report kinds and how to record a correction or an acknowledgement.

The hosted playmat runs the same binary with `-hosted` in a container
(`make docker-build`, `make docker-run`; deployed from `.do/app.yaml`). It adds
Google sign-in, per-account games, rate limits and, optionally, a DigitalOcean
Spaces bucket that keeps the card cache across deploys and serves card images
from its CDN. Its settings are documented in `.do/app.yaml` and in the
comments above `docker-run` in the `Makefile`.

Design notes are in [TCG Review Playmat Design and Build Plan.md](TCG%20Review%20Playmat%20Design%20and%20Build%20Plan.md).
