### Milestone 1: tcgreplay.demo.gs opens on a home page, and the board moves to /board

#### - [ ] Task: Embed and serve the website from a Go site package
**Id:** 603b3ddb-f67a-41c7-8abe-63bb41727ef2
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Add a small Go package that carries the built website inside the playmat binary, the same way the board UI is carried. It serves pages, build assets and the site's own "page not found" page as plain static files, and it falls back to a placeholder page when the site hasn't been built. Pages are never cached by browsers, hashed build assets are cached for a long time, and no response sets a cookie.

*Technical detail:* [context.md#task-embed-and-serve-the-website-from-a-go-site-package](./context.md#task-embed-and-serve-the-website-from-a-go-site-package)

**Acceptance criteria**:
- [ ] The playmat binary builds and its tests pass on a machine that has never built the website, serving a placeholder page.
- [ ] When the website has been built, its pages, assets and 404 page are served from the binary with the right content types.
- [ ] Pages are sent with headers that stop browsers caching them, hashed build assets with long-lived caching, and no response sets a cookie.
- [ ] An unknown address gets the site's own 404 page with a 404 status.

#### - [ ] Task: Move the board to /board and serve the site on every other path
**Id:** 5f2c6513-b004-460f-ab3d-fc25e2ef9b41
**Repo:** tcgreplay
**Depends on:**
- 603b3ddb-f67a-41c7-8abe-63bb41727ef2 — Embed and serve the website from a Go site package
**Execution:** agent

Change the hosted playmat's routing so the website is public at every path it doesn't otherwise use, and the board page lives at `/board` behind sign-in. The board's own support addresses (its files, live connection and card lookups) stay where they are behind sign-in, so the board itself doesn't change. Board links in tool results, the place sign-in returns to, and the links on the signed-out, "not your game", "not an admin" and admin pages all point at `/board`. MCP, sign-in, health and admin routes are untouched, and desktop mode is unchanged.

*Technical detail:* [context.md#task-move-the-board-to-board-and-serve-the-site-on-every-other-path](./context.md#task-move-the-board-to-board-and-serve-the-site-on-every-other-path)

**Acceptance criteria**:
- [ ] A signed-out visitor to `/`, a docs address or `/privacy` gets the website with no redirect to Google and no cookie set.
- [ ] A signed-out visitor to `/board` is sent to Google sign-in and comes back to `/board`; a signed-in visitor gets their own board there, and `/board/` redirects to `/board`.
- [ ] Board links returned by `import_log`, `new_game` and `set_opponent_deck` have the form `https://tcgreplay.demo.gs/board?board=<handle>`, and opening one in the signed-in browser shows that account's board.
- [ ] The board's files, live connection and card lookups still need sign-in, and the board works at `/board` as it did at `/`.
- [ ] `/mcp`, sign-in, OAuth, health and admin addresses behave exactly as before, and the desktop playmat still serves its board at `http://127.0.0.1:7777/` with no website.
- [ ] The signed-out, "not your game", "not an admin" and admin pages link to `/board`.

#### - [ ] Task: Scaffold the website build, layout and test harness
**Id:** b7c5ba5f-02c1-46c8-a7ad-2b9ed9550592
**Repo:** tcgreplay
**Depends on:**
- 603b3ddb-f67a-41c7-8abe-63bb41727ef2 — Embed and serve the website from a Go site package
**Execution:** agent

Create the website project. It holds Astro, a shared page layout and the shared site script, and builds into the Go site package. The layout carries the logo, navigation, the unofficial fan project notice naming Nintendo, The Pokémon Company, Creatures and Game Freak, and the TCGdex credit on every page. A shared copy-button script and a build-time loader for the playmat's address and service limits are part of the scaffold. Make targets build and test the site, the container build gains a stage that builds it, and a browser test suite runs against the built site served through the Go handler.

*Technical detail:* [context.md#task-scaffold-the-website-build-layout-and-test-harness](./context.md#task-scaffold-the-website-build-layout-and-test-harness)

**Acceptance criteria**:
- [ ] Building the site puts plain HTML pages into the Go site package, and the container build produces a playmat image that serves them.
- [ ] Every page shows the logo, the site navigation, the unofficial fan project notice naming all four companies, and the TCGdex credit.
- [ ] The site build fails with a clear message if any of the three service limits is missing from the App Platform definition.
- [ ] Any block marked as copyable gets a copy button that puts exactly that block's text on the clipboard.
- [ ] The browser suite runs from the project's test target against the built site, and checks every page for the notice, sideways scrolling at 375 pixels, cookies, and requests to other hosts.
- [ ] The site loads no web fonts, analytics or resources from any other host.

#### - [ ] Task: Write the home page and the privacy page
**Id:** b6dd327b-6407-4ad9-9fbf-71c92c547cbc
**Repo:** tcgreplay
**Depends on:**
- b7c5ba5f-02c1-46c8-a7ad-2b9ed9550592 — Scaffold the website build, layout and test harness
**Execution:** agent

Write the home page: a one-sentence description of tcgreplay, a still scene of the playmat drawn in the board's own colours and no-art card style, and a "Get started" link to the connecting instructions, all visible without scrolling on a laptop. Write the privacy page at `/privacy`, stating:
- what the hosted playmat collects at sign-in
- what it keeps while someone plays
- how long each is kept
- which outside services are involved
- what it never collects

The privacy page uses the facts the playmat implements today.

*Technical detail:* [context.md#task-write-the-home-page-and-the-privacy-page](./context.md#task-write-the-home-page-and-the-privacy-page)

**Acceptance criteria**:
- [ ] On a 1366×768 window the home page shows the one-sentence description, the playmat scene and a "Get started" link without scrolling.
- [ ] The scene shows a playmat in the board's style with plain stand-in cards, and no official card artwork or Pokémon logos.
- [ ] The privacy page at `/privacy` states what is collected at sign-in (Google account id, email and its verified flag), what is kept while playing and that it is held in memory only, how long each is kept, the outside services involved, and what is never collected.
- [ ] The privacy page's idle time comes from the App Platform definition, not typed in.

#### - [ ] Task: Write the connecting, desktop and sign-in pages
**Id:** 11845bab-16a7-4f15-9d70-ac5dbe866883
**Repo:** tcgreplay
**Depends on:**
- b7c5ba5f-02c1-46c8-a7ad-2b9ed9550592 — Scaffold the website build, layout and test harness
**Execution:** agent

Write the connecting instructions, the page the home page's "Get started" link opens. It covers:
- **claude.ai, ChatGPT and Claude Code:** step-by-step instructions for each, ending with how to check the assistant can reach the playmat.
- **Desktop:** installing the playmat and connecting the Claude desktop app, with prerequisites and the board's address there.
- **Sign-in and the board link:** what the Google sign-in looks like the first time, and how to open the board link from the assistant's reply.

The playmat's address has a copy button.

*Technical detail:* [context.md#task-write-the-connecting-desktop-and-sign-in-pages](./context.md#task-write-the-connecting-desktop-and-sign-in-pages)

**Acceptance criteria**:
- [ ] The home page's "Get started" link opens the connecting instructions.
- [ ] There are separate step-by-step instructions for claude.ai, ChatGPT and Claude Code, and each ends by telling the user how to check the connection works.
- [ ] The desktop instructions list the prerequisites, the install steps, the Claude desktop app setting and the board's address on the desktop.
- [ ] The sign-in and board-link text is reachable from the connecting instructions and describes the Google prompt and opening the board link (at `/board`).
- [ ] The playmat's MCP address has a copy button that copies exactly `https://tcgreplay.demo.gs/mcp`.

### Milestone 2: The how-to guide: connect, import, review, drill and play

#### - [ ] Task: Write a page for every coaching workflow, read from the playmat's guides
**Id:** 0d86023b-e1a4-4cce-a547-5aad2d1b2dad
**Repo:** tcgreplay
**Depends on:**
- b7c5ba5f-02c1-46c8-a7ad-2b9ed9550592 — Scaffold the website build, layout and test harness
**Execution:** agent

Make the site read the playmat's coaching guides at build time and require a matching page for each, so a workflow added or removed in the playmat breaks the site's build until the docs follow. Write those pages, each in plain language with at least one copyable example prompt:
- play a game
- import a game
- the opponent's likely deck
- review a game
- replay drill
- before you move
- numbers drill

Also write a page on rewinding an imported game to a chosen turn and trying another line.

*Technical detail:* [context.md#task-write-a-page-for-every-coaching-workflow-read-from-the-playmats-guides](./context.md#task-write-a-page-for-every-coaching-workflow-read-from-the-playmats-guides)

**Acceptance criteria**:
- [ ] Every coaching workflow the playmat offers has a page on the site with at least one copyable example prompt, listed in the playmat's own order.
- [ ] The site build fails if a workflow has no page, a page names a workflow the playmat doesn't have, or a page has no example prompt.
- [ ] A page explains how to rewind an imported game to a chosen turn and play a different line, with an example prompt.
- [ ] The workflow descriptions shown come from the playmat's guides, not retyped.

#### - [ ] Task: Publish the anonymised example game and the import page
**Id:** cb1c4655-a05b-4285-a115-655e38a7b3df
**Repo:** tcgreplay
**Depends on:**
- b7c5ba5f-02c1-46c8-a7ad-2b9ed9550592 — Scaffold the website build, layout and test harness
**Execution:** agent

Add the example game: the corpus game from 11 October 2026 with the opponent's player name replaced by "Rival", and its Mega Lucario ex decklist, each with a copy button. A Go test guards it: the file must be the original with only that name changed, and it must import with its decklist cleanly and with the original result. Write the import page. It covers:
- where to copy the battle log and the decklist in TCG Live
- what to paste
- what a complete import looks like
- what to do if lines are missing, such as a chat app collapsing a long paste

*Technical detail:* [context.md#task-publish-the-anonymised-example-game-and-the-import-page](./context.md#task-publish-the-anonymised-example-game-and-the-import-page)

**Acceptance criteria**:
- [ ] The example page shows the complete battle log from setup to the result and its decklist, each with a copy button that copies exactly that text.
- [ ] The example log contains no real opponent's name, and a test fails if it differs from the original game in anything but that name.
- [ ] A test imports the example log with its decklist and finds no problems and the same winner and prize counts as the original game.
- [ ] The import page explains where to copy the log and the decklist in TCG Live, what a complete import looks like, and what to do if lines are missing.

#### - [ ] Task: Write the hosted limits and developer pages
**Id:** 8fdc8d6b-84a1-4306-8bb1-9e3116500e38
**Repo:** tcgreplay
**Depends on:**
- b7c5ba5f-02c1-46c8-a7ad-2b9ed9550592 — Scaffold the website build, layout and test harness
**Execution:** agent

Write the page on the hosted playmat's limits. It covers:
- what the playmat keeps, and that games are dropped after the idle time and on every update
- how to keep a game's record and import it again
- how many players it serves at once and the requests per minute
- what the "busy" and "too many requests" messages mean
- running it on the desktop when the hosted playmat is full

The numbers come from the App Platform definition. Write a short developer section that links to the source repository for running it locally and contributing, without copying the repository's developer notes.

*Technical detail:* [context.md#task-write-the-hosted-limits-and-developer-pages](./context.md#task-write-the-hosted-limits-and-developer-pages)

**Acceptance criteria**:
- [ ] The limits page's idle time, players served at once and requests per minute match the App Platform definition, and change when it does.
- [ ] The limits page says what the hosted playmat keeps, that games are lost when idle or updated, that a record can be kept and imported again, and points to the desktop option when the playmat is full.
- [ ] The developer section links to the source repository and contains no build or test instructions beyond that pointer.

#### - [ ] Task: Point the README at the site and record the new routes
**Id:** 4fcd4d8b-f215-48f0-926e-535fe36c7d1c
**Repo:** tcgreplay
**Depends on:**
- 5f2c6513-b004-460f-ab3d-fc25e2ef9b41 — Move the board to /board and serve the site on every other path
- 11845bab-16a7-4f15-9d70-ac5dbe866883 — Write the connecting, desktop and sign-in pages
- 0d86023b-e1a4-4cce-a547-5aad2d1b2dad — Write a page for every coaching workflow, read from the playmat's guides
**Execution:** agent

Shorten the README to a brief introduction with the logo, linking to the site's home page and connecting instructions in place of the step-by-step user instructions and example prompts, and keeping its developer notes. Update the design and build notes and the deploy definition's comments to record the new routes (site at `/`, board at `/board`), how the site is built and shipped, and that a docs merge redeploys the playmat.

*Technical detail:* [context.md#task-point-the-readme-at-the-site-and-record-the-new-routes](./context.md#task-point-the-readme-at-the-site-and-record-the-new-routes)

**Acceptance criteria**:
- [ ] The README links to the site's home page and its connecting instructions, and no longer contains the step-by-step connecting instructions or the example prompts.
- [ ] The README still carries a brief description, the logo and the developer notes.
- [ ] The design notes and the deploy definition's comments describe the site at `/`, the board at `/board`, how the site is built, and that each push to main redeploys the playmat.

### Milestone 3: The home page shows the playmat in action, and Google's consent screen points at the site

#### - [ ] Task: Animate the home page's playmat scene
**Id:** 8eda3982-e121-4fdf-abb3-f81dc1235958
**Repo:** tcgreplay
**Depends on:**
- b6dd327b-6407-4ad9-9fbf-71c92c547cbc — Write the home page and the privacy page
**Execution:** agent

Turn the home page's still scene into a short looping animation. A battle log arrives, the board fills with the game, an assistant's coaching message appears, the game rewinds to an earlier turn, and a different line is played. It plays on its own from the page's own files, needs nothing from the playmat, and keeps the board's look with no card artwork or logos. For visitors who ask for reduced motion, it stays as the still, and nothing on the page moves.

*Technical detail:* [context.md#task-animate-the-home-pages-playmat-scene](./context.md#task-animate-the-home-pages-playmat-scene)

**Acceptance criteria**:
- [ ] With the playmat's addresses blocked, the home page animation plays without being touched, shows in order a battle log arriving, the board filling, a coaching message, a rewind to an earlier turn and a different line, then starts again.
- [ ] With the reduced-motion setting on, the home page shows the still scene and nothing on the page moves.
- [ ] The home page's total download, animation included, is under 1.5 MB.
- [ ] The animation uses the board's colours and plain stand-in cards, with no official card artwork or Pokémon logos.

#### - [ ] Task: Enter the site's addresses on Google's OAuth consent screen
**Id:** 797471ef-70b2-493f-9f4c-536d0a045c85
**Repo:** tcgreplay
**Depends on:**
- 5f2c6513-b004-460f-ab3d-fc25e2ef9b41 — Move the board to /board and serve the site on every other path
- b6dd327b-6407-4ad9-9fbf-71c92c547cbc — Write the home page and the privacy page
**Execution:** human — needs the Google Cloud console for the playmat's OAuth project and a deployed site

Once the site is deployed, open the playmat's OAuth consent screen in the Google Cloud console. Set the application home page to `https://tcgreplay.demo.gs/` and the privacy policy to `https://tcgreplay.demo.gs/privacy`, confirming `demo.gs` is listed as an authorized domain. This lets Google accept the app's public pages without a new domain.

*Technical detail:* [context.md#task-enter-the-sites-addresses-on-googles-oauth-consent-screen](./context.md#task-enter-the-sites-addresses-on-googles-oauth-consent-screen)

**Acceptance criteria**:
- [ ] The consent screen's home page is `https://tcgreplay.demo.gs/` and its privacy policy is `https://tcgreplay.demo.gs/privacy`, both loading publicly.
- [ ] `demo.gs` is an authorized domain on the consent screen, and Google sign-in to the playmat still works afterwards.
