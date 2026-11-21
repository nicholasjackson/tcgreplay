---
tags: [testing, playwright, design, workflow]
---

# While the maintainer is tweaking the design, iterate fast: targeted specs only

When the maintainer is reviewing the board live (on the dev server, often on an iPad or a second screen) and feeding back design tweaks or bugs one after another, speed matters more than the full suite. For each round: build (`cd web && npm run build`), run only the specs that cover what changed, rebuild the dev server, and show the result. Don't run the full board suite or re-record the screenshot baselines every round.

The full board suite, the site suite, `go test ./...` and a two-run baseline re-record happen once, at the end of the session, before anything is committed or handed off. The maintainer asked for this on 2026-11-21: waiting for the full suite on each design tweak was too slow.

Design system updates are batched the same way. While the maintainer is fixing the game, change the board first and record the round in the design system afterwards, in one pass, unless they ask to see a design first.
