### Milestone 1: tcgreplay.demo.gs opens on a home page that gets people connected, and the board moves to /board

**What changes**: Someone opening `https://tcgreplay.demo.gs` sees a public home page instead of being sent straight to Google sign-in. The page says in one sentence what tcgreplay is, shows a still of the playmat in the board's style, and has a "Get started" link to step-by-step connecting instructions for claude.ai, ChatGPT, Claude Code and the Claude desktop app, with what to expect from the Google sign-in and how to open the board link. Every page carries the unofficial fan project notice and the TCGdex credit, and a privacy page at `/privacy` says what the playmat collects, how long it keeps it and what it never collects. The board itself now lives at `/board`. Board links in new assistant replies point there, signing in returns there, and the admin and "not your game" pages link there. Board links in past chats stop working, as the user accepted. Connectors at `/mcp` are untouched. The site ships inside the playmat and goes live with each normal deploy.

**Validation point**: Go tests show public site paths serve without sign-in or cookies and the board needs sign-in at `/board`, with the new link format in tool results. The site's browser tests pass for the home page, privacy page, connecting pages, the address copy button, the notice on every page, phone width, and no cookies or third-party requests. After a deploy, the live address shows the home page and a fresh board link opens the board.

### Milestone 2: The how-to guide: import, review, drill and play

**What changes**: The site becomes the full user guide.
- **Importing:** how to import a TCG Live battle log with its decklist, and what a complete import looks like.
- **Workflows:** a page for every coaching workflow the playmat offers, each with example prompts, plus rewinding to try another line.
- **Example game:** a ready-made, anonymised example game with its decklist.
- **Limits:** the hosted service's limits, with the desktop option for when it is full.
- **Developers:** a short pointer to the repository.

Every prompt and the example game have one-click copy buttons. Workflow names and service limits come from the playmat itself, so the site can't drift. The README shrinks to an introduction that links here.

**Validation point**: The site build fails if a workflow page or limit is missing. The Go test proves the example game imports cleanly. Browser tests confirm copy buttons, workflow coverage, limits matching the App Platform settings, and no sideways scrolling on a phone. The README links to the home and connecting pages.

### Milestone 3: The home page shows the playmat in action, and Google's consent screen points at the site

**What changes**: The home page's still is replaced by a short looping animation in the board's own look, with no card artwork or logos. A battle log arrives, the board fills with the game, the assistant coaches, the game rewinds to a decision and a different line is played. It plays on its own, needs nothing from the playmat, and shows a still to visitors who ask for reduced motion. Once the site is live, the operator enters its home and privacy addresses on Google's OAuth consent screen.

**Validation point**: Browser tests show the animation's beats in order and looping with the playmat blocked, a still with no motion under reduced motion, and a home page under 1.5 MB. The owner's look-and-feel review and the Lighthouse checks are recorded in the implementation test plan. The consent screen lists the site's home and privacy addresses.
