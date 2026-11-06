**A second embedded web app, following the board's pattern.** The board is a Node-built front end compiled into a Go package and embedded with a placeholder fallback. The website repeats that exactly: Node builds it into a Go package's `dist`, and Go embeds it with a placeholder when it hasn't been built. A developer who knows how the board ships knows how the site ships. Its Make targets, Dockerfile stage and `.dockerignore` lines sit beside the board's equivalents and read the same way. Desktop builds embed only the placeholder unless the site is built, and desktop mode never serves it.

**The hosted router becomes an explicit path table.** Today it is "sign-in routes, `/mcp`, `/admin`, and everything else is the board". It becomes:
- sign-in, `/mcp` and `/admin` as before
- the board page at `/board`
- the board's support paths
- everything else is the public site

The board keeps seeing itself at the root: a small path-rewriting wrapper hands it `/` for `/board`. So the board server, the per-account front door and desktop mode don't change. The only other playmat edits are the handful of places that spell the board's address. Those move to `/board`, and existing tests that pin `/` or `/?board=` are updated to the new address, not loosened.

**Content is Markdown. Facts come from the playmat.** Pages are Markdown with a little frontmatter, under one shared layout that carries the header, footer, fan notice and credit. Anything the playmat defines is read at build time and checked, never retyped:
- the workflows and their order (from the guides)
- the hosted limits (from the App Platform definition)
- the example decklist (from the test corpus)

A guide added to the playmat without a page, or a page for a guide that's gone, breaks the site's build. That is the new safeguard keeping docs and product in step. Each page's example prompts sit in frontmatter or fenced blocks and are rendered with a copy button. Pages never assemble prompts in script.

**The animation is a small scripted scene, not a dependency.** One Astro component holds the scene's markup in the board's tokens and no-art card style. A short script steps through the five beats by toggling classes on a timer, and CSS transitions do the motion. With reduced motion the script exits early, and the CSS shows the final frame as a still. The site has no front-end framework, animation library, web font or external asset.

**The example game is guarded from both sides.** Its committed log is plain text the site shows and copies. A Go test sits beside the corpus import test and checks two things: that it is the corpus game with only the opponent's name changed, and that it imports cleanly with the same result. Editing it by hand without keeping it importable fails the Go tests.

**Testing follows the board's browser-suite conventions.** The site gets its own Playwright project in the site package, with Chromium-only profiles: a 1366×768 laptop and a 375-pixel phone. It runs against a built site served the way production serves it, through the Go site handler, so the routes and headers under test are the real ones. Go tests cover the router table, the board link format, and the site package's caching and cookie behaviour.
