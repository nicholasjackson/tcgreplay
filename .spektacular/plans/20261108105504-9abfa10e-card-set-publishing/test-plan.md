---
created_date: "2026-11-08"
document_status: final
closed_date: "2026-11-08"
---

# Test Plan: 20261108105504-9abfa10e-card-set-publishing

These checks cover the spec's success metrics and the plan's manual reviews. None of them can be automated, because they need the real bucket, CDN, App Platform or players. Everything else is covered by `make test`.

**Where things are**

- Hosted playmat: `https://tcgreplay.demo.gs`. The admin page is `/admin`; its button "Check for published card sets now" posts to `/admin/check`.
- Published sets: `https://tcgreplay.ams3.cdn.digitaloceanspaces.com/cardsets/manifest.json`, with each set at `cardsets/sets/<id>/<16 hex>.json.gz`.
- Workflows: "Publish card sets" (`.github/workflows/publish-cardsets.yml`) runs on pushes to `main` that change `cardsets/**`. "Deploy playmat" (`.github/workflows/deploy.yml`) runs on pushes to `main` that change anything else. Both can be started by hand.
- Loader log lines (hosted: App Platform runtime logs; desktop: stderr):
  - `card sets: loaded N sets (version V) from …` at start.
  - `card sets: picked up version V: changed …, removed …` when a check picks up a publish.
  - `card sets: check failed (…); keeping version V` when a check fails.
- Pickup interval: `-cardsets-interval` / `TCGREPLAY_CARDSETS_INTERVAL`, 5 minutes by default (allowed range 10s to 10m).
- Desktop local copy: `<user cache dir>/tcgreplay/cardsets/`, for example `~/.cache/tcgreplay/cardsets/` on Linux. `-cache` overrides the location.

The current state is noted under each check.

---

## Success metrics

### SM1: The median time from a set change being merged to being live on the hosted playmat is under one hour

- **What to measure:** for each merged PR that changes `cardsets/`, the time from the merge commit to the hosted playmat's pickup log line. Take the median across the first five such merges.
- **How:**
  1. Note the merge time: `gh pr view <n> --json mergedAt`.
  2. Find the "Publish card sets" run for that merge (`gh run list --workflow publish-cardsets.yml`) and note when it completed.
  3. In the App Platform runtime logs, find the first `card sets: picked up version …` line after that time that names the changed set ids.
  4. Live time = pickup log time − `mergedAt`.
- **Expected result:** median under 60 minutes. With the defaults, expect under about 10 minutes: the workflow's run time plus up to 5 minutes until the next check.
- **Who / when:** the maintainer, over the first five card-data merges after the switch.

### SM2: Zero player reports of cards showing as unknown or without text after the switch

- **What to measure:** the count of player reports (GitHub issues, messages) of a card shown as unknown or with no text.
- **How:**
  1. Watch the issue tracker and player channels for 30 days from the switch's deploy (deployment `80f3ccd9`, 2026-11-08 18:42 UTC).
  2. For each report, check the playmat log for `card sets: none loaded` or `check failed`.
  3. Check whether the card's set is in the manifest.
- **Expected result:** 0 reports traced to card data missing from published sets.
- **Who / when:** the maintainer, for 30 days after the switch.

---

## Manual reviews

### MR1: A publish reaches the hosted playmat without a restart; open games keep their cards

- **Where:** hosted playmat, `/admin`, a board.
- **Steps:**
  1. Open a game on the hosted playmat and note a card's text (for example through `card_text`).
  2. Change that card's text in `cardsets/` on a branch and merge it. "Publish card sets" runs; "Deploy playmat" does not.
  3. Without pressing anything, wait up to 10 minutes, then start a new game with the card.
  4. Repeat with a second change, and this time press "Check for published card sets now" on `/admin` straight after the publish run completes. The page shows `Last check now by … : picked up version …`.
- **Pass:**
  - The new game shows the new text within 10 minutes with no restart, and at once after the admin check.
  - The game opened before the publish still shows the old text.

### MR2: A fresh CDN request returns the new version after a publish

- **Where:** CDN, board.
- **Steps:**
  1. After a real publish that changes a set, run `curl -s https://tcgreplay.ams3.cdn.digitaloceanspaces.com/cardsets/manifest.json`.
  2. Fetch the changed set's `key` from the manifest under the same base, and gunzip it.
  3. On the board, open a card whose image source changed.
- **Pass:**
  - Within a minute of the publish run, the manifest names the new key (the publisher purges the manifest from the CDN).
  - The object decodes to the new set.
  - The card shows the new image.
  - If the manifest is still old after the purge succeeded, follow the plan's Open Questions fallback: read the manifest from the bucket's origin.

### MR3: Delta Reign art and CDN image copies show on the hosted board

- **Where:** hosted board.
- **Steps:**
  1. Import or open a game containing a Delta Reign card with an image source.
  2. Open a game containing a common TCGdex-art card. In the browser's network tab, look at its image request.
- **Pass:**
  - The Delta Reign card shows its image.
  - The TCGdex card's image loads from `tcgreplay.ams3.cdn.digitaloceanspaces.com/images/…`.

### MR4: Image fallback on the hosted board

- **Where:** hosted board.
- **Steps:**
  1. Find, or arrange in a test set, a card whose first listed image source returns 404, and one whose every source fails.
  2. Open both on the board, and watch the network tab.
- **Pass:**
  - The first card shows its next listed source.
  - The second shows the placeholder after trying each source in order.

### MR5: Desktop plays offline, and picks up a publish when online

- **Where:** a desktop playmat built from `main` (`go run ./cmd/tcgreplay`).
- **Steps:**
  1. Run the playmat online once. The log shows `card sets: loaded N sets … from https://…`, and `~/.cache/tcgreplay/cardsets/manifest.json` exists.
  2. Turn the network off and restart. Play a game and read card text.
  3. Turn the network back on, leave the playmat running, and publish a change.
- **Pass:**
  - Offline, the log shows the sets loaded from the local copy, and every card has full text.
  - Online, `card sets: picked up version …` appears within 10 minutes of the publish.

### MR6: A card-data-only merge publishes without redeploying; a code merge deploys

- **Where:** GitHub Actions, App Platform deployments.
- **Steps:**
  1. Merge a PR that changes only `cardsets/`.
  2. Merge a PR that changes code or docs.
  3. Check `gh run list --limit 5` and the App Platform deployments list after each.
- **Pass:**
  - The first merge starts "Publish card sets" only, and no new App Platform deployment appears.
  - The second merge starts "Deploy playmat", which succeeds with a new ACTIVE deployment.
- **Status (2026-11-08):** partly seen.
  - Autodeploy is off in the DigitalOcean console, which is the same as `deploy_on_push: false`.
  - The 2026-11-08 push to `main` changed both code and `cardsets/`. It started both workflows: "Publish card sets" run 42510960904 succeeded, and "Deploy playmat" run 42510960863 succeeded on a hand re-run after the `TCGREPLAY_APP_ID` fix (deployment `80f3ccd9` ACTIVE).
  - No hand-started (`workflow_dispatch`) deploy has run yet.
  - A card-data-only merge has not happened yet. Confirm the first one.

### MR7: Legacy card data is gone from the bucket; image copies are intact

- **Where:** bucket `tcgreplay`, with the `TCGREPLAY_SPACES_*` settings exported.
- **Steps:**
  1. Run `go run ./cmd/tcgreplay sets cleanup-legacy` (lists only; it deletes nothing without `-delete`).
  2. Open a board and check that the CDN image copies still load.
  3. `curl` the manifest.
- **Pass:**
  - The listing reports 0 objects under `tcgdex/`.
  - Images load from `…/images/…`.
  - The manifest answers 200.
- **Status (2026-11-08):** done by the maintainer.
  - The listing found 401 objects.
  - `-delete` deleted 401.
  - A re-run listed 0.
  - The manifest answers 200 `application/json`.
  - Confirm on a board that the image copies still load.

### MR8: Follow the publishing documentation to publish a set by hand

- **Where:** `cardsets/README.md` § Publishing.
- **Steps:** follow only that section:
  1. Run `go run ./cmd/tcgreplay sets publish -dry-run` with the bucket settings exported, then without `-dry-run`. Alternatively, start "Publish card sets" from Actions → Run workflow.
  2. Note what the section says about how quickly playmats pick a publish up.
- **Pass:**
  - The steps work as written, and the output reports uploaded and unchanged sets.
  - The section states the 5-minute check, the admin's "Check for published card sets now", and that open games keep their cards.
