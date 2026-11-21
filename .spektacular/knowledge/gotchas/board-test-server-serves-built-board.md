---
tags: [webui, playwright, testing, build, embed]
---

# The board's browser tests run the last built board, not the source

The board test server (`go run ./internal/webui/webtest/cmd/boardtest`, started by `web/playwright.config.js`) serves the board embedded from `internal/webui/dist/app`, which `npm run build` in `web/` writes and git ignores. Running `npx playwright test` directly does not rebuild it, so after a merge, a branch switch or an edit to `web/src`, the suite tests whatever board was last built. Symptoms are tests that fail for markup the source plainly has (for example a group or class added on another branch), or pass against code that no longer exists.

Build first: `make web` (or `npm run build` in `web/`), then run Playwright. `make test` already does this before the board suite. A fresh worktree also has no `node_modules`: run `npm ci` in `web/` (and `site/`) inside that worktree first.
