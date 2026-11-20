- **Website source (new).** The Astro project: a shared page layout with header, footer, the unofficial fan project notice and the TCGdex credit on every page, and Markdown pages:
  - the home page
  - connecting from claude.ai, ChatGPT and Claude Code, plus the desktop app
  - signing in and opening the board
  - importing a game
  - one page per coaching workflow
  - rewinding
  - the example game
  - the hosted service's limits
  - privacy
  - a short developer pointer

  It owns all user-facing wording. It depends on the build-time facts loader for anything the playmat defines.
- **Build-time facts loader (new, part of the website build).** Reads the playmat's own sources while the site builds:
  - the coaching guides' names, one-line descriptions and order
  - the hosted limits from the App Platform definition
  - the example decklist from the test corpus

  It makes them available to pages. It fails the build when a guide has no workflow page, a page names a guide that doesn't exist, or a limit is missing, so the site can't drift from the playmat.
- **Home animation (new, part of the website).** A self-contained scripted scene of the playmat in the board's colours and no-art card style. It loops through the five beats (log arrives, board fills, coaching message, rewind, different line), and shows its last frame still under reduced motion. It loads nothing from the playmat or any other host.
- **Copy controls (new, part of the website).** One small shared script that gives every example prompt, the playmat's address and the example game a copy button that puts exactly that block's text on the clipboard.
- **Example game (new content).** The corpus game with the opponent's name replaced by a placeholder, committed beside the site. A Go test guards it: it checks the file is the original with only that substitution, and that it imports with its decklist cleanly and with the same result.
- **Site package (new Go package).** Embeds the built website in the binary, with a placeholder page when it hasn't been built. It serves it as plain static files: correct content types, long caching for hashed build assets, no caching for pages, the site's own page for unknown paths, and no cookies. It mirrors how the board UI is embedded.
- **Hosted router (changed).** Sends the board page at `/board` to the existing per-account board front door, rewritten so the board sees its usual root. It keeps the board's support paths (assets, live connection, card lookups) behind sign-in where they are, and gives every other path to the site package without sign-in. Sign-in, MCP, OAuth, health and admin routes are unchanged.
- **Board link and sign-in return paths (changed).** The board link in tool results, the default place sign-in returns to, and the links on the signed-out, "not your game", "not an admin" and admin pages now point at `/board` instead of `/`.
- **Container build (changed).** Gains a stage that builds the website, alongside the board's stage, and copies it into the site package before the Go build. The build context ignores the site's dependencies and build output and still includes the guides, the App Platform definition and the test decklist.
- **Developer tooling (changed).** Make targets to build, preview and test the site, run from the existing test target. The website's browser tests reuse the board's Playwright setup and conventions (Chromium-only profiles).
- **README and design notes (changed).** The README becomes a brief introduction that links to the site's home and connecting pages instead of carrying the instructions. The design and build notes record the new routes (site at `/`, board at `/board`) and the site's build.
- **Operator setup (human).** Entering the home page and privacy page addresses on Google's OAuth consent screen once the site is live.
