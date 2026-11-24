---
tags: [testing, site, build]
---

# A fresh worktree has no built website, and one hosted test fails until it is built

The built website (`internal/site/dist/app`) is not tracked in git; only `internal/site/dist/placeholder` is. A new git worktree (such as an implement run's under `.spektacular/worktrees/`) therefore embeds no website, and `cmd/tcgreplay` `TestOpenAIChallengeNotSet` fails there. It compares the challenge path's 404 with the website's 404 page, and without a built site the two are the same.

**What to do.** Run `make site` (it runs `npm ci` and the Astro build inside the worktree's `site/`) before treating a `go test ./...` failure in a worktree as real. The board is likewise built by `make web`.
