**Three layers, each slotting into an existing convention.**

- **Go unit and integration tests**, beside the existing hosted, usergames and hostauth tests and run by the Go test suite:
  - **Hosted routing:** the route table. Signed out, `/`, a docs page, `/privacy` and a site asset are served without a redirect or a `Set-Cookie`. `/board` sends a signed-out browser to sign-in that returns to `/board`, and serves a signed-in account's board. `/board/` redirects to `/board`. `/assets/`, `/ws` and `/api/` still need a session. `/mcp`, `/auth/*`, `/healthz` and `/admin` behave exactly as before. An unknown path gets the site's 404 page with status 404.
  - **Board link:** tool results carry `<public>/board?board=<handle>`. The existing tests that pinned `/?board=` and `next=/` are updated to the new address. The "not your game", signed-out and admin pages link to `/board`.
  - **Site package:** the placeholder is served when the site isn't built. Pages are sent with no-cache headers and hashed build assets with long-lived ones. No response sets a cookie.
  - **Example game:** the site's example log equals the corpus game with only "GatoruSoju" → "Rival". It contains no "GatoruSoju". It imports with the corpus decklist with zero problems and zero invariant warnings, and gives the same winner and prize counts as the original (from `testdata/index.json`).
- **Build-time checks in the site build**, which fail the site build and so the container build:
  - every guide has exactly one workflow page, and every workflow page names a real guide
  - all three limits are present in the App Platform definition and parse
  - every workflow page has at least one example prompt
- **Browser tests (Playwright, Chromium-only profiles as the board's suite)** against the built site served through the Go site handler:
  - Laptop 1366×768: the pitch, the animation and "Get started" are within the first viewport, and "Get started" opens the connecting page.
  - The animation runs with every request to the playmat's API and board paths blocked. Its beats appear in order (log, board, coaching, rewind, new line) and it starts over.
  - With `prefers-reduced-motion: reduce` emulated, a still frame is shown and no element animates: no running CSS animations or transitions, and the scene's frame doesn't change over a few seconds.
  - Every page shows the fan notice naming Nintendo, The Pokémon Company, Creatures and Game Freak, and the TCGdex credit. Pages are crawled from the site's own links.
  - Phone 375 px: no page scrolls sideways (document and main checked, per the board's phone gotcha), and code, prompt and address blocks wrap or scroll inside their own box.
  - Every copy button puts exactly its block's text on the clipboard, with clipboard permissions granted.
  - In a fresh context, loading every page sets no cookies and makes no request to any host but the site's own.
  - Every workflow the playmat defines has a section with a prompt on the site. The limits shown match the App Platform definition. The privacy page exists at `/privacy`. The developer section links to the repository.
  - The home page's total transfer size is under 1.5 MB.

**Most coverage goes to the router and the build-time checks.** They are the two places where a mistake breaks the live playmat (a board that won't load) or lets the docs drift from the product. Page wording is covered by presence checks, not snapshots, so editing copy doesn't churn tests.

**Deliberate gaps:**
- No visual-regression snapshots of the animation. Its order and looping are asserted structurally instead.
- No WebKit runs, per the board's gotcha.
- The README change is checked by reading it during the goal check, not by a test.

**Success metrics:**
- *New player connects and imports the example game in under 10 minutes, unaided, on two of three tries with different people* — **Manual — captured in the implementation test plan.**
- *Home page shows its largest content in under 2.5 s in a Lighthouse mobile audit (Slow 4G), with total download under 1.5 MB* — the download half is a **behavioural test** (the browser test above sums the home page's transfer size). The largest-content time is **Manual — captured in the implementation test plan** (a Lighthouse run against the deployed site).
- *Every page scores at least 90 for accessibility and performance in a Lighthouse mobile audit* — **Manual — captured in the implementation test plan.**

**Manual reviews before acceptance:**
- **Manual — captured in the implementation test plan:** each assistant's connecting steps (claude.ai, ChatGPT, Claude Code) followed by someone new to tcgreplay, ending with the assistant confirming it reaches the playmat.
- **Manual — captured in the implementation test plan:** the desktop steps on a machine with the listed prerequisites, ending at the board on `127.0.0.1:7777`.
- **Manual — captured in the implementation test plan:** importing the site's example game from a real assistant per the import page, reporting no problems.
- **Manual — captured in the implementation test plan:** after deploy, the live site shows a merged content change within 15 minutes, `/board` loads a signed-in board, and a board link from a new tool result opens it.
- **Manual — captured in the implementation test plan:** a read-through of the privacy and limits pages against the playmat's behaviour on the day of publishing.
- **Manual — captured in the implementation test plan:** the import page's directions for copying the battle log and decklist checked against TCG Live's current menus.
- **Manual — captured in the implementation test plan:** the owner's look-and-feel review of the home page animation (board style, no card art or logos).
