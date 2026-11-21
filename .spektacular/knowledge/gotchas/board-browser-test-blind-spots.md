---
tags: [board, webui, playwright, testing, notes]
---

# Board browser tests: notes have no browser path, act() moves are held, and short class names collide

Two things in `web/e2e` that are not obvious until a test fails:

**A note (annotation) can't be added from the browser.** The board refuses the `annotate` verb ("annotations come from the assistant"), and the board test server (`internal/webui/webtest`) has no endpoint for notes. So `act(page, 'annotate', …)` fails. To show a note in a browser test or a review screenshot, rewrite the state messages instead: `page.routeWebSocket(/\/ws$/, …)` forwards to the server and adds `annotations` to a log line in each state message on its way to the page, then `page.reload()`.

**Tests use some short class names as page-level locators.** Several specs call `page.locator('.body')` and mean the app's `.body` grid in `App.svelte` (the board beside or above the timeline). A component that adds its own `.body` element, even scoped by Svelte, makes those locators match twice and fail in strict mode. Before giving an element a generic class (`.body`, `.board`, `.timeline`, `.line`, `.text`), grep `web/e2e` for a page-wide locator of it. The Studio timeline renamed a line's inner span to `.words` for this reason. `timelineLines` in `fixtures.js` reads `.line .text`, so those two names are part of the test contract.

**Moves made with `act()` arrive as another client's.** `act(page, …)` in `fixtures.js` drives the board over its own websocket, so the page treats those moves as someone else's. It holds them ("N new moves") or autoplays them instead of showing the new position. After a run of `act()` calls, `page.reload()` to see the latest position live. Set `replaySpeed` in localStorage first if a test needs time to check a replay step. Also, `end_turn` needs an explicit `seat` once it is p2's turn: without one, it defaults to the page's seat and is refused.

