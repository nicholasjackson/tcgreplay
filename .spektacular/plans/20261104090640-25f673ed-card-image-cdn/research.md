---
created_date: "2026-11-04"
document_status: final
closed_date: "2026-11-04"
---

# Research: 20261104090640-25f673ed-card-image-cdn

## Alternatives considered and rejected

- **Board tries the CDN first for every image, server unaware of what is copied.** The board would build the CDN URL itself from the TCGdex URL and fall back on `onerror`, asking the server to copy on a miss. Rejected: every image without a copy costs a failed request to the CDN, and the DigitalOcean CDN may cache that 403/404 at the edge for its TTL. A copy made afterwards could then stay invisible for up to the TTL, which breaks "Later views load it from the shared store" and the 60-second acceptance window. It also puts the bucket layout into the board. Evidence: `web/src/components/Card.svelte:76-77` already uses `onerror` only to switch to the text stand-in.
- **Proxy images through the playmat (`/api/image/...`) with the playmat reading Spaces.** Rejected: the spec's Technical Approach says to serve images through the Spaces CDN, and it puts every image byte through the single 0.5 GB App Platform instance (`.do/app.yaml` `instance_size_slug: apps-s-1vcpu-0.5gb`).
- **Check each image in Spaces (HEAD) while answering `/api/card`.** Rejected: it adds up to the 3-second shared-store bound to a card answer the board waits on (`internal/webui/server.go:136-160`, with a 15 s context), and repeats per card per process. A start-up listing plus an in-memory set answers without any network call.
- **Persist which images are copied inside the card data entries (`tcgdex/`).** Rejected: the `Entry.Data` contract is the raw TCGdex body, unchanged between tiers (freshness plan, Data Structures). Stamping it would change that project-wide contract.
- **Re-encode or resize images, or use another source when TCGdex has no art (malie.io, Limitless).** Rejected by the spec: Non-Goal forbids changing images, and the constraint forbids any source other than the card source's own images. Knowledge `learnings/card-image-sources.md` lists those sources; they stay unused.
- **A new `TCGREPLAY_SPACES_CDN_URL` setting.** Rejected for now: the CDN address of a DigitalOcean bucket follows from the endpoint and bucket (`https://<bucket>.<region>.cdn.digitaloceanspaces.com`), and the project rule fixes the four settings. A custom CDN domain is out of scope.
- **Adopting `minio-go`/AWS SDK for listing.** Rejected for the same reasons as the shared-cache plan: one more signed call (ListObjectsV2) on the existing signer is enough.

## Chosen approach — evidence

- The board reads card art from one field: `CardInfo.Image`, the TCGdex base URL, to which it appends `/low.webp` (`web/src/components/Card.svelte:77`) or `/high.webp` (`web/src/components/Zoom.svelte:18`, `web/src/components/Modal.svelte:239`). Changing what the server puts in that field, plus a fallback field, moves the board to the CDN with three small component edits.
- `/api/card` is the only producer of `CardInfo` (`internal/webui/server.go:136-160`, route at `:80`). It runs outside the game lock, so an in-memory lookup and a non-blocking queue push there respect "nothing under the game lock calls the network".
- `webui.Server` already carries optional hosted-only fields (`Public`, `Limit`, `internal/webui/server.go:29-43`) set only in `newHosted` (`cmd/tcgreplay/hosted.go:183-188`). A nil `Images` field keeps desktop, daemon, dev and the board test server exactly as today.
- Every image URL the server hands out is under `https://assets.tcgdex.net/` (`internal/cards/cards.go:77`, `305` for borrowed Energy art). Restricting copies to that prefix enforces the "card source's own images only" constraint.
- The board fetches card info once per page load (knowledge `gotchas/board-card-info-per-page-load.md`, `web/src/lib/store.svelte.js:137-152`), so the URL chosen at first card-info fetch holds for the page; a copy made meanwhile is used from the next page load, which is what "Later views" means.
- The shared-cache plan's Spaces client already has `Put` with `PutOptions{ContentType, CacheControl, Public}`, typed errors and the fake; its breaker rule (3 s, 1 minute, edge-triggered logs) is reused for image copies.
- Playwright can intercept `/api/card` and external hosts (`page.route`), so the board's fallback can be tested in the existing suite (`web/playwright.config.js`, `web/e2e/*.spec.js`) without changing `boardtest` or `cardstest`.

## Files examined

- `tcgreplay:internal/webui/server.go:29-43` — `Server` optional hosted fields (`Public`, `Limit`).
- `tcgreplay:internal/webui/server.go:77-84` — routes; `/api/card` registered once.
- `tcgreplay:internal/webui/server.go:117-160` — `CardInfo` (Image documented as TCGdex base) and `serveCard` (no-cache, 15 s ctx).
- `tcgreplay:internal/webui/webui_test.go:295-302` — existing `/api/card` test style (httptest, string contains).
- `tcgreplay:internal/cards/cards.go:77` — `Card.Image` field; `:289-308` — `energyArt` borrows TCGdex Crown Zenith art.
- `tcgreplay:internal/cards/cardstest/*.go:17-46` — `FakeDB` is a map; tests can add a card with `Image`.
- `tcgreplay:cmd/tcgreplay/hosted.go:172-205` — `newHosted` builds one card DB and a board per account (`NewBoard`).
- `tcgreplay:cmd/tcgreplay/hosted.go:207-246` — `runHosted`, server ctx and start-up log line.
- `tcgreplay:cmd/tcgreplay/main.go:224-229` — `newCardDB` (desktop path, unchanged).
- `tcgreplay:cmd/tcgreplay/hosted_test.go:55,260` — `captureLog`, `hostedConfig` helpers; "no warning" checks.
- `tcgreplay:web/src/components/Card.svelte:74-77` — Energy token and `<img>` with `broken` flag.
- `tcgreplay:web/src/components/Zoom.svelte:1-20` — zoom image, `broken` reset per card.
- `tcgreplay:web/src/components/Modal.svelte:110,238-239` — preview image, no `onerror` today.
- `tcgreplay:web/src/lib/store.svelte.js:137-152` — `cardInfo` once-per-page fetch.
- `tcgreplay:web/playwright.config.js` — e2e suite runs `boardtest` on 127.0.0.1:7790, three device profiles.
- `tcgreplay:internal/webui/webtest/cmd/boardtest/main.go` — board test server, fake card pool without images.
- `tcgreplay:.do/app.yaml` — env declarations; comments say the card cache is lost on deploy (upstream plan fixes that).
- `tcgreplay:Dockerfile:1-6`, `tcgreplay:Makefile:49-63` — settings comments, docker-run passes env.
- `tcgreplay:TCG Review Playmat Design and Build Plan.md:302-325` — hosted settings table and operator setup.

## External references

- AWS S3 ListObjectsV2 API (`GET /<bucket>?list-type=2&prefix=&continuation-token=&max-keys=`) — the listing call added to the Spaces client; DigitalOcean Spaces implements it.
- DigitalOcean Spaces CDN docs — CDN endpoint is `https://<bucket>.<region>.cdn.digitaloceanspaces.com`; enabled per bucket in the control panel with an edge TTL (max 1 week); serves only objects readable publicly (per-object `x-amz-acl: public-read` works in a private bucket).
- DigitalOcean Spaces per-object ACLs — `x-amz-acl: public-read` on PUT makes one object public while the bucket and its listing stay private.
- TCGdex assets — `https://assets.tcgdex.net/<lang>/<series>/<set>/<number>/<quality>.<ext>`, quality `low`/`high`.

## Prior plans / specs consulted

- `20261104090640-51bd1f36-card-cache-freshness` (plan) — `Store`/`Entry`/`ErrNotCached`, freshness above the store, `RetryAfter = 5m`, a card's `Image` can appear later via a 24 h refresh of imageless cards; injected clock pattern.
- `20261104082020-109dceb1-tcgdex-spaces-cache` (plan) — `internal/spaces` client (`Config`, `FromEnv`, `New`, `Get`, `Put`, `PutOptions`, `ErrNotFound`, `*Error`), `spacestest` fake (counts, Fail/Deny/Hang/Close), `tcgdex/` prefix private, breaker rules, hosted helper building the card DB from env, offline never builds the shared tier, human task that creates the private bucket and key.
- Spec `20261104090640-25f673ed-card-image-cdn` — requirements, constraints, acceptance criteria, 95 % success metric.

## Open assumptions

- The shared-cache plan's hosted helper and breaker land roughly as planned; this plan extends that helper and reuses the breaker logic (factored to an unexported type if it is inline).
- The access key the operator created for the shared-cache plan may PUT with `x-amz-acl: public-read` and list the bucket. If real Spaces refuses either, STOP and ask.
- DigitalOcean's CDN address follows `https://<bucket>.<region>.cdn.digitaloceanspaces.com` for endpoints `https://<region>.digitaloceanspaces.com`.
- High-quality TCGdex webp images upload to Spaces from App Platform within the 3-second bound; checked manually.
- The board reaches the CDN host directly; no Content-Security-Policy restricts `img-src` today (none found in `internal/`, `web/`).

## Drafting assumptions

### Chosen direction: server-side image cache with a board fallback (architecture)
- **Decision**: A playmat-side image cache in `internal/cards` keeps an in-memory set of copied images (start-up listing of `images/` plus its own copies). `/api/card` answers with the CDN address for copied images and `image_fallback` = TCGdex address; otherwise the TCGdex address and a queued background copy. The board tries `image`, then `image_fallback`, then the stand-in.
- **Key design decisions**: the board never requests an uncopied object from the CDN (no negative caching); `/api/card` never waits on Spaces; copies are byte-exact `low.webp` and `high.webp` under `images/<asset path>/`, public-read, `image/webp`, `Cache-Control: public, max-age=604800`; only `https://assets.tcgdex.net/` addresses are copied; the cache exists only when the shared store is on and online.
- **Rejected**: board-first CDN with fallback; proxying through the playmat; per-request HEAD; stamping copy state into card data entries.

### Server decides each image's address from an in-memory set of copies (discovery)
- **Decision**: The playmat answers `/api/card` with the CDN address only for images it knows are copied (start-up listing of the image prefix plus copies it made), otherwise the TCGdex address, and queues a background copy. The board never asks the CDN for an image without a copy.
- **Rationale**: Avoids CDN negative caching and failed requests, keeps `/api/card` free of network calls, and keeps the bucket layout out of the board.
- **Rejected**: board-first CDN with fallback (negative caching risk); per-request HEAD (latency); proxying through the playmat (contradicts "serve through the Spaces CDN").

### CDN address derived, no new setting (discovery)
- **Decision**: The public image address is derived from the endpoint and bucket (`https://<bucket>.<region>.cdn.digitaloceanspaces.com`), falling back to the path-style origin `<endpoint>/<bucket>` for non-DigitalOcean endpoints such as the test fake.
- **Rationale**: The project fixes the four `TCGREPLAY_SPACES_*` settings; the CDN host follows from them.
- **Rejected**: a fifth `TCGREPLAY_SPACES_CDN_URL` setting (only needed for a custom CDN domain, out of scope).

### Copy both board sizes as one image (architecture)
- **Decision**: Each image is copied as `low.webp` and `high.webp`; it counts as copied only when both exist.
- **Rationale**: The board shows exactly those two files (`Card.svelte` low, `Zoom.svelte` and `Modal.svelte` high); one address field serves both.
- **Rejected**: copying only `low.webp` (zoom would still hit TCGdex, failing "every card image"); copying PNG/JPG variants (unused).

### Browser cache lifetime of copies: 7 days (architecture)
- **Decision**: `Cache-Control: public, max-age=604800` on every image copy.
- **Rationale**: TCGdex art at a given address does not change; a week matches the CDN's longest edge TTL, and keeps a later admin clear from being masked for longer than that.
- **Rejected**: one year/immutable (an admin-cleared image would linger in browsers for a year); no header (browsers revalidate every view, raising CDN requests).

### Image cache lives in `internal/cards` with its own breaker instance (architecture)
- **Decision**: New file in `internal/cards`; it reuses the shared-cache plan's breaker logic (factored to an unexported type if inline) as a separate instance labelled for images.
- **Rationale**: The cache is TCGdex-specific (asset host, path shape) and the breaker logic already lives there; a separate instance keeps the card store's tiered store unchanged.
- **Rejected**: a new package (would duplicate the breaker); sharing one breaker instance with the tiered store (needs the tiered store to expose internals; an outage then logs two pairs of lines, accepted).

### Copier limits: 2 at a time, 10 s per TCGdex file, queue of 256 (architecture)
- **Decision**: Two concurrent copies; each TCGdex file fetch bounded at 10 s; each Spaces put at the project's 3 s; a 256-entry queue that drops when full; images TCGdex fails on are retried after 5 minutes.
- **Rationale**: Keeps the 0.5 GB instance and TCGdex load small while meeting the 60-second fill window; dropped or skipped copies are re-queued on the next view.
- **Rejected**: unbounded goroutines per image; persistent queue (copies are a cache).

### Hourly count log for the success metric (architecture)
- **Decision**: The image cache logs, once an hour when anything happened, how many card answers pointed at the CDN versus TCGdex and how many copies were made.
- **Rationale**: Gives the operator a measure for the 95 % metric from the app's own log.
- **Rejected**: no measure (metric unmeasurable without CDN analytics); a metrics endpoint (no metrics stack exists).

## Rehydration cues

- `spektacular spec file read 20261104090640-25f673ed-card-image-cdn`
- `spektacular plan file read 20261104082020-109dceb1-tcgdex-spaces-cache plan` (and `context`)
- `spektacular plan file read 20261104090640-51bd1f36-card-cache-freshness plan`
- `spektacular knowledge read` for `gotchas/board-card-info-per-page-load.md`, `learnings/card-image-sources.md`, `gotchas/tcgdex-cache-never-expires.md`
- Re-read `internal/webui/server.go:117-160`, `cmd/tcgreplay/hosted.go:172-205`, `web/src/components/{Card,Zoom,Modal}.svelte`, `web/src/lib/store.svelte.js:137-152`.
