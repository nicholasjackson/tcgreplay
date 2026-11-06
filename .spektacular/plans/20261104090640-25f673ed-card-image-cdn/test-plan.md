---
created_date: "2026-11-04"
document_status: final
closed_date: "2026-11-04"
---

# Test plan: 20261104090640-25f673ed-card-image-cdn

These checks cover what the automated suites cannot reach: the real DigitalOcean Spaces bucket, its CDN, real browsers, and a week of real use. Go tests (`go test ./...`) and the Playwright suite (`cd web && npm ci && npm run build && npx playwright test`) cover everything else, against the `spacestest` stand-in and stub hosts.

## Prerequisites (operator, once)

All of the checks below need these done first:

1. The shared card cache's human task: a private Spaces bucket, a bucket-scoped read/write access key, and the four `TCGREPLAY_SPACES_*` settings set encrypted on the App Platform component. See the design document's operator setup, step 6.
2. This spec's human task, still open: in the bucket's Settings → CDN, enable the CDN with an edge cache TTL of 1 week, and keep file listing restricted. Confirm the access key can list the bucket and write objects with a public-read ACL. Then redeploy. See operator setup step 7.

Who and when: the operator, after the build that contains this change is deployed to the hosted App Platform app.

---

## M1. Start-up log after the CDN is enabled (the human task's checks)

- **What:** the human task's acceptance criteria.
- **How:**
  1. Redeploy, or restart the `playmat` component.
  2. Open the runtime logs: App Platform → app → Runtime Logs, or `doctl apps logs <app-id> playmat --type run`.
  3. Read the start-up lines.
- **Expected:**
  - `card images: copied from the card source to the shared store, served from https://<bucket>.<region>.cdn.digitaloceanspaces.com/images/`. The host must equal the CDN endpoint the control panel shows for the bucket.
  - `card images: N copies found in the shared store`, where N is 0 on the first deploy.
  - No `card images: shared store unavailable` line, and no `warning: card cache:` line.
- **Pass:** all three hold.

## M2. Real CDN end to end (spec "Missing image filled in", "Hosted images from the shared store")

- **How:**
  1. Sign in to the hosted board in a desktop browser with devtools → Network open, filtered on `webp`.
  2. Start a new game (or import a log) with cards not yet copied.
  3. On first view, the card images load from `assets.tcgdex.net`.
  4. Within 60 seconds, list the bucket: Spaces → bucket → `images/` in the control panel, or `s3cmd ls -r s3://<bucket>/images/` with the access key. Each shown card has `images/<lang>/<series>/<set>/<number>/low.webp` and `high.webp`. Check one object's permissions: it is public.
  5. Reload the board and open a card's zoom.
- **Expected:** after the reload, every card image request (`low.webp` on the mat, `high.webp` in the zoom) goes to `<bucket>.<region>.cdn.digitaloceanspaces.com`, and none goes to `assets.tcgdex.net`.
- **Pass:** the copies appear within 60 s, and after the reload there are zero `assets.tcgdex.net` image requests for copied cards.

## M3. Card data stays private, images are public (the human task's criterion 3)

- **How:**
  1. In a private window with no sign-in, open `https://<bucket>.<region>.cdn.digitaloceanspaces.com/images/<a copied path>/low.webp`.
  2. Open `https://<bucket>.<region>.cdn.digitaloceanspaces.com/tcgdex/sets.json`.
  3. Open `https://<region>.digitaloceanspaces.com/<bucket>/tcgdex/sets.json`, the origin rather than the CDN.
- **Expected:** step 1 shows the image (HTTP 200, `Content-Type: image/webp`, `Cache-Control: public, max-age=604800`). Steps 2 and 3 are refused (403 AccessDenied).
- **Pass:** exactly that.

## M4. Broken CDN falls back (spec "Broken copy falls back")

- **How:**
  1. On the hosted board, with copies present (after M2), open devtools → Network → Request blocking.
  2. Block `*.cdn.digitaloceanspaces.com` and reload.
  3. Look at the hand, the bench, a zoomed card, and (on desktop) the preview when hovering a card in the Discard view.
- **Expected:** every card image still shows, loaded from `assets.tcgdex.net`: one failed CDN request, then the TCGdex request. No card falls back to its text stand-in unless TCGdex has no art for it.
- **Pass:** every image shows.

## M5. No shared store, images as today (spec "No shared store, images as today")

- **How:**
  - Hosted: run the container with no `TCGREPLAY_SPACES_*` variables (`make docker-run` with them unset). Note the bucket's `images/` object count, open a board and play a few cards. Check the log, Network and the bucket again.
  - Desktop: run the desktop build (`tcgreplay` with `-ui`) with the four variables exported in the shell, and open the board.
- **Expected:**
  - The log has `card cache: local only` and no `card images:` line.
  - Every image request goes to `assets.tcgdex.net`.
  - `/api/card` answers carry no `image_fallback` (check a response in Network).
  - The bucket's `images/` object count is unchanged.
- **Pass:** all four hold, in both runs.

## M6. Restart keeps copies

- **How:** after M2, redeploy or restart the component. Then open a board with the same cards and watch the logs and Network.
- **Expected:**
  - The start-up log says `card images: N copies found in the shared store`, with N at least the number copied in M2.
  - On the first board load after the restart, copied cards load from the CDN straight away, with no `assets.tcgdex.net` request for them.
  - No new PUTs for those objects: their Last-Modified in the control panel is unchanged.
- **Pass:** all three hold.

## M7. Real iOS Safari

- **How:** on an iPhone (Safari), sign in to the hosted board, open a game with copied cards, and tap a card to zoom. Then block the CDN host and reload. On iOS the simplest block is a content-blocker app or a DNS block for `*.cdn.digitaloceanspaces.com`.
- **Expected:** images load from the CDN first, then from TCGdex when it is blocked. No text stand-ins appear for cards that have art.
- **Pass:** both behaviours are seen.

## M8. Success metric: at least 95 % of image views from the shared store over a week

- **What:** "Over a week of hosted use, card images load from the shared store's delivery address for at least 95% of image views."
- **How:**
  1. After M1 to M6 pass, leave the hosted playmat in normal use for 7 days, with no redeploy that clears the bucket.
  2. Collect the hourly lines `card images: last hour A from the shared store, B from the card source (P% shared), K copied` from the runtime logs, for example `doctl apps logs <app-id> playmat --type run | grep 'card images: last hour'`, or the log forwarding destination if one is set up.
  3. Sum A and B over the week. Share = ΣA / (ΣA + ΣB).
  4. Optionally cross-check against the bucket's CDN bandwidth in the Spaces → bucket → Insights view, against browser traffic to TCGdex if known.
- **Expected:** share ≥ 95 %.
- **Note:** the counts are card-info answers (one per card per page load), not browser image loads. A CDN failure that falls back in the browser still counts as a CDN answer, so M4 should pass before this measure is trusted.
- **Pass:** ΣA / (ΣA + ΣB) ≥ 0.95 over the 7 days.
- **Who and when:** the operator, one week after M1 to M6 pass.
