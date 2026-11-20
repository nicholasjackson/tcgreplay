---
created_date: "2026-11-04"
document_status: final
closed_date: "2026-11-04"
---

# Admin cache clearing for the hosted playmat

## What was built

Admins of the hosted playmat get a small page at `/admin`, behind the existing Google sign-in, from which they clear the card cache: everything, or one set named by its decklist code (such as `MEG`) or its TCGdex id (such as `me01`, any case). A name that matches no set, even after the set list is refreshed, is refused and nothing is cleared.

- **Who may use it.** `TCGREPLAY_ADMINS` lists the admins' Google account emails (any case). The sign-in now reads Google's `email_verified` claim and keeps it in the board cookie; only a verified email on the list passes the new admin guard. A signed-out page request goes through Google sign-in, a signed-out direct request gets 401, a non-admin or a sign-in from before this release gets a 403 "Not an admin" page, and a change not sent from the playmat's own pages gets 403. Board, connector and MCP sign-in are unchanged.
- **What a clear reaches.** Every tier the hosted playmat keeps, each through the layer that owns it: memory, the local disk cache, the DigitalOcean Spaces card data under `tcgdex/` (`sets/<id>` and every `cards/<id>-…`), the public image copies under `images/` (for a set, those whose TCGdex path has the set id as its third segment), and, with `TCGREPLAY_SPACES_CDN_TOKEN` set, a purge of the bucket's CDN through the DigitalOcean API (`images/*` or `images/<lang>/<serie>/<set>/*`; the CDN endpoint is found by the bucket's origin). Bucket objects are deleted 1,000 at a time with S3's batch delete. A clear calls Spaces directly with its own longer bounds, outside the lookup breaker, so a failure is reported rather than skipped.
- **Open games keep playing.** A clear marks in-memory cards as cleared instead of dropping them: the cached-only lookup that games use under their lock keeps returning the old copy and fetches the fresh one in the background, while card, deck-code and name lookups fetch from TCGdex at once (keeping the old copy only if TCGdex is down). A clear generation stops a fetch or catalog build begun before a clear from putting old data back.
- **Images reach browsers.** Every CDN image address now carries a content version, `?v=<12 hex>`, taken from the copy's two objects' ETags (the MD5 of their bytes). It is the same across restarts and changes exactly when the bytes do, so art copied again after a clear gets an address no browser or CDN edge has cached, despite the week-long `Cache-Control`. `/api/card` sends it as `image_version`; the board adds it to the CDN address only, never to the TCGdex fallback.
- **The job and the page.** A clear runs as one background job at a time (a second gets "already running"), bounded at 10 minutes; the page refreshes itself while it runs and then shows what each tier removed, whether the CDN was purged and any error. Every clear is logged at start and end with the admin and the scope. Without the shared store the page clears memory and disk only.
- **Deployment and docs.** `.do/app.yaml` declares `TCGREPLAY_ADMINS` and `TCGREPLAY_SPACES_CDN_TOKEN` (encrypted), `make docker-run` passes both through, the `Dockerfile` header lists them, and the design document describes the admin page, the settings and how to create a CDN-only DigitalOcean API token. The start-up log names the number of admins (never who) and whether CDN purging is on. Desktop, daemon and dev builds serve no admin page.
- Two knowledge entries were updated: the TCGdex cache entry now describes the admin clear instead of deleting objects by hand, and the board card info entry explains the versioned image addresses and that an open board shows cleared data after a reload.

## Why it matters

A correction at the card source (fixed card text, new or corrected art) can now reach the hosted board within a minute, without a redeploy and without deleting bucket objects by hand, while open games keep working as the cache refills.

## Deviations from the plan

- **Started past unmet dependencies.** Implementation started before these dependencies were implemented, as the orchestrator approved: `20261104082020-109dceb1-tcgdex-spaces-cache` was in progress (9/10 tasks complete) and `20261104090640-25f673ed-card-image-cdn` in progress (8/9 tasks complete). All their code was merged; only their human operator tasks (create the bucket, enable the CDN) were open.
- **Human task skipped, outstanding for the operator.** "Name the admins and create the CDN purge token" was sent through the goal check and skipped: it needs the DigitalOcean control panel and App Platform settings. Until it is done nobody can open `/admin` and clears do not purge the CDN. Its steps and checks are in the test plan.
- **Never-overwrite rule kept.** The card image cache never writes over an object already in the bucket and remembers half copies. Versions were fitted to it: a half copy is versioned from the listed ETag of the file already there plus the MD5 of the file put, and an image with a file whose ETag is unknown stays uncopied. A clear whose listing or delete fails marks its scope so the cache asks the bucket before copying again and never writes over what the clear left behind.
- **Clear waits for copies in flight** rather than discarding them afterwards, so no copy can land between the clear's listing and its delete.
- **Whole-request delete failures name every key not deleted** (a `*DeleteError` wrapping the S3 error), not only keys refused one by one; asked for by the goal check.
- **Small additions:** a shared `normCode` set-code helper; `loadCard`/`refreshCard` factored out of the lookups; `hostauth.Auth.AdminCount` for the start-up log; an optional `Logf` on the admin service; `hostedHandler` takes the admin service as a fourth argument; `webui.ImageSource.Image` gained its third result one task early to keep the build green.
- **Tests:** existing assertions were changed only where the new fields forced it (`google_test.go` expects `EmailVerified`, the webui stub's signature); new tests sit in new files.
- **Pre-existing flakes seen:** one `go test ./...` run hung `cmd/tcgreplay` for 600 s, and `internal/webui` `TestWebsocketLoadAndAct` failed once under `-race` ("message too big"); both passed on every rerun.
