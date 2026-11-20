---
created_date: "2026-11-04"
document_status: final
closed_date: "2026-11-04"
---

# Research: 20261104090640-a5a74fab-admin-cache-clearing

## Alternatives considered and rejected

- **Delete cached card data from memory outright on a clear.** After the freshness plan, `Game.card` (called under the game lock) reads only `Cached` and, on a miss, starts a background fetch and treats the card as unknown for that call (freshness plan, Component Breakdown "Game card lookup"). Dropping memory entries would make every open game's cards unknown until each warm completes: board reads (`View` → `g.hp` → `Cached`, `internal/game/game.go:500-515`) would show 0 HP and missing text in between. That breaks the "Open game survives a clear" criterion. Rejected in favour of marking memory entries cleared: `Cached` keeps answering the old copy and warms, while `Card` treats a cleared entry as a miss.
- **Mark cleared entries stale and let stale-while-revalidate refresh them.** The first `Card` after a clear would still return the old copy, and the board's once-per-page card info (knowledge `gotchas/board-card-info-per-page-load.md`) would keep showing it for that page load. That falls short of "every lookup is answered from fresh data" and the "within a minute" success metric. It would also need a "stale whatever its art" exception in the policy, because cards with art never expire. Rejected.
- **Version-free image copies, relying only on a CDN purge.** Copies carry `Cache-Control: public, max-age=604800` (card-image-cdn plan, Architecture), so a browser that loaded a copy keeps it for up to 7 days without asking again. Purging the CDN edge does not reach that browser cache. After a clear the image is copied again to the **same** key (`images/<TCGdex asset path>/{low,high}.webp`, a fixed layout), so the board's URL is the same and the browser shows the old art. That fails "Cleared images refreshed … within 1 hour". Rejected in favour of a content version on the board's CDN URL (`?v=<hash of both objects' ETags>`), together with the purge the spec asks for.
- **Putting a version or clear generation in the object key** (for example `images/v2/…`). It busts every cache cleanly, but it changes the fixed image layout that card-image-cdn and the epic's rules settle. Rejected.
- **Persisting a clear generation in a marker object** (for example a private `admin/image-versions.json`). It works across restarts, but adds a third bucket prefix and new state to keep consistent. The bucket already records what changed: ListObjectsV2 returns each object's ETag, which is the MD5 of the bytes for a single-part PUT. Rejected in favour of ETag-derived versions (an additive `ETags` field on card-image-cdn's `ListPage`).
- **Version from the copy time, or from listing `LastModified`.** The copier's clock and S3's `LastModified` differ, so every restart would change every URL and make every browser fetch every image again. ETag/MD5 is the same before and after a restart, and the same for an identical re-copy. Rejected.
- **No CDN purge (the spec's Technical Approach asks for one).** Recorded spec decision: "purge the image delivery network's cache for cleared images". With versioned URLs the purge is belt and braces if the CDN keys its cache on the query string, and essential if it does not (unknown, so it is a manual check). Kept.
- **Single-object S3 DeleteObject per key.** A full clear could cover tens of thousands of `tcgdex/cards/*` objects. One request per key is minutes of work. S3 DeleteObjects takes 1,000 keys per request (POST `?delete`, XML body, `Content-MD5` required). Chosen instead, as `Delete(ctx, keys)` batching internally.
- **Running the clear inside the POST request.** A full clear lists and deletes thousands of objects. App Platform and Cloudflare sit in front, with request time limits this plan cannot see, and a timed-out request would hide the result. Rejected in favour of one background clear job at a time, with the page showing its progress and result (meta refresh while running).
- **Admin check inside the board server or the new admin package.** Sign-in, sessions, the same-site check (`(*Auth).sameSite`, `internal/hostauth/board.go:110-118`, unexported) and the sign-in redirects all live in `hostauth`. Rejected in favour of `(*hostauth.Auth).RequireAdmin`, beside `RequireSession`.
- **Naming admins by Google subject id.** That is stable, but nobody knows their `sub`, and the spec says admins are named by their Google accounts. Emails are chosen, matched case-insensitively, and only when Google marked the email verified (`email_verified`, already sent by `hostauthtest` at `internal/hostauth/hostauthtest/google.go:152` but not read today).
- **Admin page in the Svelte board.** It would need a new board route, a build change and Playwright coverage for a three-control form. Rejected in favour of a server-rendered page in the style of the hostauth pages (`internal/hostauth/pages.go`).
- **Clearing a set's images by computing `images/<lang>/<serie>/<set>/` from the set detail.** That needs the set's detail (series id) and the language. Rejected in favour of one rule used for both the in-memory copy list and the bucket listing: an image belongs to set S when the third segment of its TCGdex path (`<lang>/<serie>/<set>/<number>`) is S.

## Chosen approach — evidence

- Card ids are `<set id>-<local id>` (`LogCodeID` builds `me03-121`, `sv10.5b-079`, `mep-033`, `internal/cards/cards.go:358-392`; set detail cards carry `id` and `localId`, `cards.go:326-332`). So one set's cards are the keys `cards/<set id>-…`, and other sets sharing a prefix (`sv03` vs `sv03.5`, `swsh12` vs `swsh12tg`) are not matched, because the next character after the set id must be `-`.
- Disk paths come from `safeName` (`cards.go:200-202`: replaces `/`, `\`, `..` with `_`). Because it is applied per character group, `safeName(set)+"-"` is a prefix of `safeName(card id)` for every card of the set, on disk and under `tcgdex/` in Spaces.
- Set codes resolve through `codeAliases`, `setAliases` and the catalog's code→id index from `abbreviation.official` and `tcgOnline` (`cards.go:334-349`, `443`). Set ids are lowercase TCGdex ids. The catalog's set list (freshness plan) is the authority for "names a set".
- The freshness plan keeps `Client.mu` only for in-memory maps, never across I/O, and replaces the catalog whole. So a clear can mark memory and drop the catalog under the lock, and do store deletions outside it.
- The shared-cache plan's tiered store has `Local()`, found by an unexported optional interface. The same pattern (an optional `Clear` interface found by type assertion) keeps `Store` unchanged.
- The image cache (card-image-cdn) keeps `known map[string]bool` keyed by relative path (`en/swsh/swsh12.5/157`). Turning it into `map[string]string` (path → version) and adding a scope filter is a small, local change.
- Board image URLs are built in one helper (`imageSrc` in `web/src/lib/store.svelte.js`, card-image-cdn context). Adding `?v=<image_version>` to the primary address is a one-line change used by all three image spots.
- `RequireSession` (`internal/hostauth/board.go:56-95`) already separates page navigations (redirect to sign-in) from data and non-GET requests (401). `sameSite` already guards sign-out against cross-site POSTs. `RequireAdmin` reuses both.
- The session cookie seals `grantClaims{ID, Client, Sub, Email, Exp}` (`internal/hostauth/seal.go:94-105`). An added `omitempty` field opens false on old cookies, so they keep working for the board and fail the admin check until the person signs in again.
- `hostauthtest.FakeGoogle.Claims` (`google.go:40-43`) lets tests mark an email unverified.

## Files examined

- tcgreplay:internal/hostauth/hostauth.go:25-161 — `Config`, `Account{ID, Email}`, `Routes`, `WithAccount`/`AccountFrom`. No admin notion today.
- tcgreplay:internal/hostauth/board.go:56-95 — `RequireSession` semantics (redirect for pages, 401 for live, data, non-GET and subresources). `:110-118` `sameSite`.
- tcgreplay:internal/hostauth/google.go:130-185 — `idClaims` lacks `email_verified`. `checkIDToken` builds `Account`.
- tcgreplay:internal/hostauth/seal.go:92-105 — `grantClaims` sealed into the board cookie.
- tcgreplay:internal/hostauth/store.go:176-213 — sessions reopen from the sealed cookie after a restart.
- tcgreplay:internal/hostauth/pages.go — the one server-rendered page template (light/dark, `no-store`).
- tcgreplay:internal/hostauth/hostauthtest/google.go:23-165 — fake Google, `SetAccount`, `Claims` hook, `email_verified: true`.
- tcgreplay:cmd/tcgreplay/hosted.go:149-247 — `hostedHandler` mux (`/` → `RequireSession(reg.Board())`), `newHosted`, `runHosted` start-up log.
- tcgreplay:cmd/tcgreplay/hosted_test.go:55-62, 260-266, 570-640 — `captureLog`, `hostedConfig`, `hostedServer`, `cookie`, `getWith`/`sendWith` helpers.
- tcgreplay:internal/cards/cards.go:167-287 — `Client`, `mem`, `Card`, `Cached` today (rewritten by the freshness plan).
- tcgreplay:internal/cards/cards.go:200-202 — `safeName`.
- tcgreplay:internal/cards/cards.go:289-349 — `energyArt` (borrowed swsh12.5 art), `setDetail` (with `serie.id`, `abbreviation.official`, `tcgOnline`), aliases.
- tcgreplay:internal/game/game.go:482-515 — `Game.card` and `hp` read `Cached` under the game lock.
- tcgreplay:internal/webui/server.go:117-160 — `CardInfo` and `serveCard` (`no-cache`).
- tcgreplay:.do/app.yaml — env declarations and comment style; one instance, no volume.
- tcgreplay:Makefile:51-62, Dockerfile:5 — where hosted env vars are passed through and listed.

## External references

- AWS S3 API, DeleteObjects (POST `/<bucket>?delete`, up to 1,000 keys, `Content-MD5` required, per-key `<Error>` in the result) — the batch delete the Spaces client gains. DigitalOcean Spaces documents S3 compatibility for it.
- AWS S3 API, ListObjectsV2 `<Contents><ETag>` — the ETag is the hex MD5 of the body for single-part, non-SSE-C uploads, quoted. Basis for content versions.
- DigitalOcean API v2, CDN Endpoints: `GET /v2/cdn/endpoints` (each with `id`, `origin` such as `<bucket>.<region>.digitaloceanspaces.com`, `endpoint`), and `DELETE /v2/cdn/endpoints/{id}/cache` with `{"files": [...]}`, where `*` is a wildcard (for example `images/en/me/me01/*`, or `*` for everything). Bearer token auth. A custom-scoped token needs CDN read and update.
- RFC 9111 §4.2 (HTTP caching freshness) — a response with `max-age=604800` is served from the browser cache without revalidation for 7 days. That is why purging the CDN alone cannot refresh a viewer's browser.

## Prior plans / specs consulted

- `20261104090640-51bd1f36-card-cache-freshness` (plan + context) — `Store`/`Entry`/`ErrNotCached`, keys, envelope, `mem` as `cardEntry{card, fetched}`, the immutable catalog with `ensureCatalog`, refresh tracker (misses, failed cards, background refreshes, 5 min), single-flight, `Game.card` reading only `Cached`, `Client.mu` never held across I/O, background wait helper for tests.
- `20261104082020-109dceb1-tcgdex-spaces-cache` (plan + context) — `internal/spaces` (`Config`, `FromEnv`, `New`, `Get`, `Put`, `PutOptions`, `ErrNotFound`, `*Error`, SigV4 signer), `spacestest` (knobs Fail/Deny/Hang/Close, request counts), `NewSpacesStore` (`tcgdex/` + disk relative path), `NewTieredStore` with `Local()`, breaker (3 s, 1 min, edge-triggered logs), `hostedCardDB(c) (db, shared)`, `c.cardBase`, the four `TCGREPLAY_SPACES_*` settings, and the knowledge entry note "forcing fresh hosted data means deleting `tcgdex/` by hand until admin cache clearing exists".
- `20261104090640-25f673ed-card-image-cdn` (plan + context) — `List(ctx, prefix, token) → ListPage{Keys, Next}`, `Config.PublicBase`, fake list and public reads, `cards.ImageCache` (`known` keyed by relative path, `pending`, `failed`, queue, two workers, `settle()`), `ImagePrefix = "images/"`, `ImageFiles`, 7-day `Cache-Control`, `webui.ImageSource.Image(src) (image, fallback)`, `CardInfo.ImageFallback`, the board helper `imageSrc`/`nextImageStage`, `hostedCardDB` returning `images`, `c.imageBase`. It leaves "forget this prefix" and the CDN purge to this spec.
- `20261017123240-21493a95-google-login` (an implement workflow note exists in `.spektacular/workflows/`, not read) — the origin of `hostauth`. The current code was read directly instead.

## Open assumptions

- The three upstream plans land with the names their plans give (`hostedCardDB`, `ImageCache.known`, `tieredStore`, `ListPage`, `imageSrc`, `c.cardBase`, `c.imageBase`, the freshness plan's `cardEntry`, catalog and tracker). If a seam differs, adapt names. If clearing would need the `Store` interface itself to change, STOP and ask.
- Every TCGdex card id of set S starts with `S-`. If a real set breaks this, its odd cards would survive a set clear (the full clear still removes them).
- Every TCGdex image path is `<lang>/<serie>/<set id>/<local id>`, with the set id as the third segment.
- DigitalOcean Spaces accepts DeleteObjects with our SigV4 signature and `Content-MD5`, returns per-object ETags in listings, and ignores an unknown `v` query parameter on public GETs through origin and CDN.
- The DigitalOcean CDN endpoint for the bucket has `origin` equal to `<bucket>.<region>.digitaloceanspaces.com`, and a purge accepts `images/…/*` wildcards.
- Google's ID token carries `email_verified` for the accounts admins use.

## Drafting assumptions

### Chosen direction: clear through each owning layer, version image URLs, admin job page (architecture)
- **Decision**: Clear every tier through the layer that owns it: optional `Clear` on the disk, Spaces and tiered stores; `cards.Client.Clear` marks memory entries cleared and drops the catalog and tracker state; `ImageCache.Clear` forgets copies, deletes them and purges the CDN. Image CDN addresses carry an ETag-derived `?v=` version. Admins are gated by `hostauth.RequireAdmin` with `TCGREPLAY_ADMINS`. A `cacheadmin` page starts one background clear job at a time.
- **Rationale**: Every rule the upstream plans fix is kept, with only additive extensions. Open games keep their cards, real lookups get fresh data at once, and browsers and the CDN cannot keep serving old art.
- **Rejected**: (B) a synchronous clear with memory deleted outright and purge only, which is simpler but blanks open games and leaves browser caches stale; (C) clearing only the shared store and restarting the app through the DigitalOcean API, which would drop every open game (they live in memory) and break "Clearing does not interrupt play".

### Conventions selected (architecture)
- **Decision**: Only `conventions/skills-text-and-packaging.md` (packaging half) applies.
- **Rationale**: The plan edits the deploy files and the Dockerfile comment. No guide text changes.
- **Rejected**: none. It is the only convention loaded.

### Content-versioned image URLs on top of the CDN purge (discovery)
- **Decision**: The board's CDN address for a copy carries `?v=<version>`, where the version is a short hash of the two objects' ETags (MD5). Listings report ETags through an additive `ETags` field on card-image-cdn's `ListPage`, and the copier computes the MD5 itself. The CDN purge the spec asks for is kept as well.
- **Rationale**: Copies carry a 7-day browser `Cache-Control` and are copied again to the same key after a clear, so a purge alone leaves old art in viewers' browsers for up to a week. That would fail "Cleared images refreshed within 1 hour".
- **Rejected**: purge only (browser caches stay stale); a version in the object key (breaks the fixed `images/` layout); a persisted clear-generation object (extra state and a new prefix); copy-time or LastModified versions (change on every restart).

### Cleared memory entries stay visible to Cached (discovery)
- **Decision**: A clear marks in-memory card entries as cleared rather than deleting them. `Card` treats a cleared entry as a miss and fetches it, falling back to the old copy if the card source fails. `Cached` still returns the old copy and starts the background warm.
- **Rationale**: Open games read only `Cached` under their lock (freshness plan). Deleting entries would blank their cards until each warm finished. Treating the entry as a miss keeps "every lookup answered from fresh data" for real lookups.
- **Rejected**: deleting memory (open games flicker to unknown cards); stale-while-revalidate (the first lookup after a clear returns old data).

### Set membership by id prefix and image path segment (discovery)
- **Decision**: Set S's card data is `sets/S` plus every `cards/S-…` key. Its images are every copy whose TCGdex path has S as its third segment.
- **Rationale**: TCGdex card ids are `<set id>-<local id>`, and image paths are `<lang>/<serie>/<set>/<number>`. One rule works for memory, disk and the bucket without fetching anything.
- **Rejected**: deriving keys from the set's cached card list (misses cards cached without their set); computing the image directory from set detail (needs series and language).

### Background clear job with a status page (discovery)
- **Decision**: A clear runs as one background job at a time. The POST validates the request and starts the job, then redirects to the page, which shows progress and the result.
- **Rationale**: A full clear lists and deletes thousands of objects, and proxy request limits in front of App Platform could cut a synchronous request off.
- **Rejected**: a synchronous clear in the request.

### Admins named by verified Google email (discovery)
- **Decision**: `TCGREPLAY_ADMINS` holds a comma-separated list of Google account emails, matched case-insensitively. A session is an admin only when Google marked its email verified. The flag is sealed into the session cookie, and older cookies count as unverified until the person signs in again.
- **Rationale**: The spec names admins by Google account. An email is what an operator knows, and the verified flag stops an unverified address from claiming an admin's email.
- **Rejected**: Google subject ids (nobody knows theirs).

### Clear bypasses the lookup breaker (components)
- **Decision**: Admin clears call Spaces directly, with their own longer bounds (30 s per list page or delete batch, 10 minutes for the whole job), whether or not the breaker is open, and report errors to the admin.
- **Rationale**: The breaker exists so lookups never fail because of Spaces. An admin action needs to report failure, not skip silently.
- **Rejected**: routing clears through the breaker, which would hide a failed clear behind "skipped".

### Purge token named in the SPACES family, endpoint discovered (components)
- **Decision**: One new secret, `TCGREPLAY_SPACES_CDN_TOKEN`. The CDN endpoint id is found by listing CDN endpoints and matching the origin `<bucket>.<region>.digitaloceanspaces.com`.
- **Rationale**: This keeps the epic's rule that shared-store settings are `TCGREPLAY_SPACES_*`, environment only and hosted only, and adds a single setting.
- **Rejected**: a separate `TCGREPLAY_SPACES_CDN_ID` setting (one more thing to copy by hand); a non-SPACES variable name.

### Open boards show cleared data after a reload (tasks)
- **Decision**: "The board shows" cleared card data and art means the next page load. Boards already open keep the card info they loaded until reloaded.
- **Rationale**: Knowledge `gotchas/board-card-info-per-page-load.md`. Pushing card updates to open boards needs a change to the update protocol, which the card image plan also left out.
- **Rejected**: pushing updates to open boards after a clear.

### Borrowed Energy art belongs to its own set (tasks)
- **Decision**: Basic Energy cards that borrow Crown Zenith art keep that art under `swsh12.5`. Clearing another set does not clear it, and clearing `swsh12.5` does.
- **Rationale**: The copy lives under the Crown Zenith path and is shared by every Energy that borrows it.
- **Rejected**: also clearing borrowed art when clearing the set whose Energy borrows it, which would mean tracking borrowers.

### Admins count against the user cap (tasks)
- **Decision**: Admins sign in like any user and take a place under `TCGREPLAY_MAX_USERS`. No bypass is added.
- **Rationale**: The spec requires the existing sign-in, and the non-goals exclude other admin capabilities.
- **Rejected**: letting admins bypass the cap, which is extra scope.

### The card source's own caching is out of scope (tasks)
- **Decision**: Until a cleared image is copied again, the board loads it from `assets.tcgdex.net`, which may have its own browser caching. "As the card source currently has it" means as TCGdex serves it.
- **Rationale**: TCGdex's caching is outside the playmat's control.
- **Rejected**: none.

### Clear scope reported per tier; partial failure allowed (tasks)
- **Decision**: A failed shared-store or CDN step does not undo the memory and disk clears. The page reports each tier, and a repeat clear is safe.
- **Rationale**: Clearing is idempotent, and leaving tiers half-cleared is harmless because lower tiers are re-read or refetched.
- **Rejected**: all-or-nothing clears, which S3 cannot provide anyway.

## Rehydration cues

- `spektacular plan file read 20261104090640-51bd1f36-card-cache-freshness plan` (and `context`): store, catalog, tracker, memory shapes.
- `spektacular plan file read 20261104082020-109dceb1-tcgdex-spaces-cache plan` (and `context`): Spaces client, tiered store, hosted helper.
- `spektacular plan file read 20261104090640-25f673ed-card-image-cdn plan` (and `context`): image cache, `List`, board helper.
- `spektacular spec file read 20261104090640-a5a74fab-admin-cache-clearing`.
- Re-read `internal/hostauth/board.go`, `internal/hostauth/seal.go`, `internal/hostauth/google.go:130-185` and `cmd/tcgreplay/hosted.go:149-205` before touching auth or wiring.
- Knowledge: `gotchas/board-card-info-per-page-load.md`, `gotchas/tcgdex-cache-never-expires.md` (as rewritten upstream), `gotchas/game-listeners-under-lock.md`.
