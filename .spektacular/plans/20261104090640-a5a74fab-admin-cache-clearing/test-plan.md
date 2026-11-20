---
created_date: "2026-11-04"
document_status: final
closed_date: "2026-11-04"
---

# Test plan: 20261104090640-a5a74fab-admin-cache-clearing

Manual checks for the card cache admin page on the real hosted playmat. The automated suite (`go test ./...`, `cd web && npx playwright test`) covers the clear against stand-ins for TCGdex, Spaces, the DigitalOcean CDN API and Google; nothing below can run without the real services.

**Who / when:** the operator, on the deployed app (`https://tcgreplay.demo.gs`), after the merge deploys and after the human task "Name the admins and create the CDN purge token" is done.

## 0. Operator setup (the skipped human task)

This task was left for the operator. Do it first; every check below depends on it.

1. In the DigitalOcean control panel: API → Tokens → Generate New Token, custom scopes **CDN read and update only** (`cdn:read`, `cdn:update`), with an expiry that suits you.
2. Make sure the Spaces access key in `TCGREPLAY_SPACES_KEY` can list and delete objects in the bucket. A "Limited access" key with read and write on the bucket can. Check from a shell with the key: `s3cmd ls s3://<bucket>/images/` works, and `s3cmd put /etc/hostname s3://<bucket>/tcgdex/zz-probe.json && s3cmd del s3://<bucket>/tcgdex/zz-probe.json` succeeds.
3. App Platform → the `playmat` component → Environment Variables: set `TCGREPLAY_ADMINS` to the admins' Google account emails, comma-separated, and `TCGREPLAY_SPACES_CDN_TOKEN` to the token, **Encrypt** ticked. `doctl apps update` does not carry secret values.
4. Redeploy.

**Pass:** the runtime log's start-up line ends with `<n> cache admins` (n = the number of emails set; no email is printed), the log has `card images: CDN purge on clear: on`, and there is no line containing `warning`.

## 1. Real admin sign-in, and refusal of everyone else

1. In a browser signed out of the playmat, open `https://tcgreplay.demo.gs/admin`. **Pass:** it goes through Google sign-in and lands back on the "Card cache" page showing "Signed in as <your email>", with the "Clear everything" and "Clear one set" actions.
2. If you signed in to the board before this release, the page says "Not an admin … sign out and sign in again": sign out from the board and in again, then retry. **Pass:** the page now opens.
3. In a private window, sign in with a Google account not in `TCGREPLAY_ADMINS` and open `/admin`. **Pass:** a "Not an admin" page (HTTP 403) naming that account; the board still works for it.
4. Signed out: `curl -si -X POST https://tcgreplay.demo.gs/admin/clear -d all=1`. **Pass:** `401` and "sign in required"; the log shows `hostauth: refused POST /admin/clear: not signed in` and no `cache admin:` line.
5. **Pass for all of 3–4:** the bucket's object count under `tcgdex/` and `images/` is the same before and after (`s3cmd ls -r s3://<bucket>/ | wc -l`).

## 2. Real clear of everything

1. Note the counts: `s3cmd ls -r s3://<bucket>/tcgdex/ | wc -l` and `s3cmd ls -r s3://<bucket>/images/ | wc -l` (both above 0 on a used deployment).
2. On `/admin`, press "Clear everything". The page shows "Clearing now" and refreshes every 2 seconds until it shows "Last clear: everything by <you>" with one line per tier.
3. **Pass:**
   - both counts are now 0;
   - the page lists the card entries removed from the shared store, from local disk and in memory, the image copies removed and "CDN purged (1 paths)";
   - the log shows `cache admin: <you> started clearing everything` and `cache admin: cleared everything for <you> in …`, plus `card cache: cleared everything: …` and `card images: cleared everything: <n> objects, CDN purged`;
   - no restart happened (the app's uptime and its open games are unchanged).
4. In claude.ai, import a game (paste a TCG Live log with `import_log`). **Pass:** the board shows every card with its text and art, and the bucket refills under `tcgdex/` and `images/`.

## 3. Real set clear by code and by id, and an unknown set

1. Note which objects exist for one set you have played, for example Mega Evolution: `s3cmd ls s3://<bucket>/tcgdex/sets/me01.json`, `s3cmd ls s3://<bucket>/tcgdex/cards/ | grep /me01-` and `s3cmd ls -r s3://<bucket>/images/ | grep /me01/`, and the totals for everything else.
2. On `/admin`, clear one set with `MEG`. **Pass:** the page says "Last clear: Mega Evolution (me01) by <you>"; every me01 object listed in step 1 is gone; every other object is still there (totals for other sets unchanged, including `tcgdex/sets.json`); the log shows `card images: cleared set me01: <n> objects, CDN purged`.
3. Play or import something with me01 cards again so they refill, then clear with `me01` (and once with `ME01`). **Pass:** the same objects, and only those, are removed.
4. Clear one set with `XYZ`. **Pass:** the page answers (HTTP 400) `No set is named "XYZ". Nothing was cleared.`; the bucket listing is unchanged and there is no `cache admin: … started clearing` line.

## 4. Real CDN purge and image refresh within 1 hour (spec: "Cleared images refreshed")

1. In a browser, open a board showing a card of a set whose copies exist, and open devtools → Network. **Pass (baseline):** its image loads from `https://<bucket>.<region>.cdn.digitaloceanspaces.com/images/<path>/low.webp?v=<12 hex>`.
2. Note the `v=` value. On `/admin`, clear that set. **Pass:** the log shows the purge succeeded (`CDN purged`), and the DigitalOcean control panel's CDN endpoint for the bucket shows no error.
3. Reload the board. **Pass:** the image now loads from `https://assets.tcgdex.net/...` (no copy yet). Within a minute, reload again: it loads from the CDN address with a `v=` value. If TCGdex changed the art, the `v=` differs from step 2 and the picture is TCGdex's current one; if not, it is the same `v=` and the same picture.
4. The browser that saw the old art must show TCGdex's current art **within 1 hour** of the clear. **Pass:** after a reload within the hour, the image matches `https://assets.tcgdex.net/<path>/low.webp` opened directly.
5. `curl -sI 'https://<bucket>.<region>.cdn.digitaloceanspaces.com/images/<path>/low.webp?v=<v>'` **Pass:** `200`, `Content-Type: image/webp`, `Cache-Control: public, max-age=604800` (the CDN serves `?v=` addresses).

## 5. Real open game survives a clear (spec: "Open game survives a clear")

1. Open a game on the hosted board (import a log or start one with `new_game`) and leave the board tab open.
2. From another tab, clear everything on `/admin`.
3. **Pass:** the open board keeps showing the game with card text and art, without reloading; Claude's next tool calls (for example `get_state`, `card_text`, a move) work; a play on the board (drag a card) works. After a reload the board still shows every card.

## 6. Real Spaces and CDN behaviour this design assumes

1. **DeleteObjects with our signature and `Content-MD5`:** covered by check 2 (a full clear deletes thousands of objects in batches of 1,000). **Pass:** no `spaces: 4xx` error on the page or in the log. If Spaces refuses DeleteObjects for a reason other than permissions, stop and raise it: the fallback is one DeleteObject per key.
2. **Listing ETags equal the MD5 of uploaded copies:** pick a copied image; `s3cmd info s3://<bucket>/images/<path>/low.webp` shows an MD5 equal to `curl -s https://assets.tcgdex.net/<path>/low.webp | md5sum`. Restart the app: **Pass:** the board's `v=` for that image is unchanged after the restart.
3. **CDN endpoint discovery:** the purge in check 2 succeeded, so the CDN endpoint's origin is `<bucket>.<region>.digitaloceanspaces.com`. If the log says `spaces: no CDN endpoint for this bucket`, check the CDN is enabled on the bucket and raise it (an explicit endpoint id setting would be needed).

## 7. Success metric: a correction shows on the board within a minute, without a redeploy

1. Pick a card whose data or art on TCGdex differs from what the board shows (or one TCGdex has just corrected), open it on the board and note its text or art.
2. Start a timer and, on `/admin`, clear its set by code.
3. When the page shows "Last clear", reload the board.
4. **Pass:** the reloaded board shows the card as TCGdex currently has it, with the timer under **60 seconds** from pressing "Clear set" to the reloaded board showing the correction, and no redeploy or restart.
