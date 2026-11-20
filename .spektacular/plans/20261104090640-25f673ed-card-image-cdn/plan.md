---
created_date: "2026-11-04"
document_status: final
closed_date: "2026-11-04"
---

# Plan: 20261104090640-25f673ed-card-image-cdn

<!-- Metadata -->
<!-- Created: 2026-11-04T12:09:45Z -->
<!-- Commit: 8c26660570c61a3e063c43882de3fae7a6168aeb -->
<!-- Branch: main -->
<!-- Repository: git@github.com:nicholasjackson/tcgreplay.git -->

## Overview

The hosted playmat starts keeping its own copy of every card image the board shows, in the DigitalOcean Spaces bucket the shared card cache already uses, and the board loads those copies from the bucket's CDN instead of from TCGdex. An image without a copy still shows at once from TCGdex and is copied in the background, so the next page load uses the CDN. If the CDN cannot be reached, the board falls back to TCGdex. Copies are exact and come only from TCGdex's own art. Without the shared store (desktop, dev, offline, or hosted with no settings) images load from TCGdex exactly as today. Hosted players get images from the playmat's own delivery address, and the card source is asked for each image only once.

## Conventions

- **Guide text and packaging (`conventions/skills-text-and-packaging.md`)** — applies in its packaging half: this plan edits the `Dockerfile` comment and the hosted deploy files, and must not exclude `internal/guides` from the build context. The guide-wording half does not bear on this work, since no guide text changes.

## Architecture & Design Decisions

All the work lands in the one registered repo, `tcgreplay`: a new image cache in `internal/cards`, one more call and a public-address helper in `internal/spaces` (with the fake learning both), a field on the board server and its card endpoint in `internal/webui`, the hosted wiring in `cmd/tcgreplay`, three board components in `web/src`, a browser test in `web/e2e`, and the hosted deploy files and docs. It builds on the two upstream plans in this epic, which must land first. From the card cache freshness plan (`20261104090640-51bd1f36-card-cache-freshness`) it takes the rule that a card's `image` can appear later, when an imageless card is refreshed. From the shared card cache plan (`20261104082020-109dceb1-tcgdex-spaces-cache`) it takes the `internal/spaces` client (`Config`, `FromEnv`, `New`, `Get`, `Put` with `PutOptions`, `ErrNotFound`, `*Error`), the `spacestest` fake, the four `TCGREPLAY_SPACES_*` settings (environment only, hosted only), the private `tcgdex/` prefix, the breaker rules (3-second bound per Spaces request, the shared tier skipped for 1 minute after a failure, one log line when an outage starts and one when it ends) and the hosted helper that builds the card database from the environment. None of those shapes change. The `Store`, `Entry`, `ErrNotCached` contract and the freshness rules are not touched: images never pass through the card store.

**The playmat decides each image's address; the board never guesses.** The board reads art from one field today, the TCGdex base URL in `/api/card`'s `image`, and appends `/low.webp` (card) or `/high.webp` (zoom and preview). A new **image cache** in `internal/cards` owns the answer to "where should the board load this image from?". It keeps an in-memory set of the images that have a public copy in Spaces. That set is filled at start-up by listing the `images/` prefix in the bucket, and grows with every copy the playmat makes. `/api/card` asks it for each card's image. For an image with a copy, the answer is the CDN address of that copy, and the TCGdex address goes in a new `image_fallback` field. For an image without one, the answer is the TCGdex address, exactly as today, and the image is queued for copying in the background. The answer is a map lookup and a non-blocking queue push, so the card endpoint never waits on Spaces, and nothing under the game lock is involved. Because the board only ever asks the CDN for objects that exist, it never meets a "missing" answer the CDN could cache. A copy made during a page's life is used from the next page load, matching the board's once-per-page card info (knowledge `gotchas/board-card-info-per-page-load.md`). In the board, each of the three image spots tries `image`, switches once to `image_fallback` if that fails to load, and only then shows today's text stand-in or Energy token. This beats a board that tries the CDN first for everything, which would send a failing request for every uncopied image and risk the CDN caching that failure past the 60-second window. It also beats proxying images through the playmat, which the spec's "serve through the Spaces CDN" rules out, and checking Spaces on every card request, which adds latency where the board waits. The evidence is in `research.md#alternatives-considered-and-rejected`.

**Copies are exact, public, and only of TCGdex art.** A copy is made only for an image address under `https://assets.tcgdex.net/` (spec constraint: the card source's own images, no other source). That covers the Crown Zenith Energy art the card database already borrows. Anything else is passed through unchanged and never copied. One image is two objects, `low.webp` and `high.webp`, the two files the board shows. They are stored byte for byte (spec Non-Goal: no resizing or recompressing) under `images/<TCGdex asset path>/`, for example `images/en/swsh/swsh12.5/157/low.webp`. Each is written with `Content-Type: image/webp`, `Cache-Control: public, max-age=604800` and public-read access, while the bucket and its `tcgdex/` card data stay private. An image counts as copied only when both objects exist. The CDN address is worked out from the existing settings, `https://<bucket>.<region>.cdn.digitaloceanspaces.com`, so no new setting is added. For an endpoint that is not DigitalOcean's (the test fake), it is the path-style public address `<endpoint>/<bucket>`. The background copier runs two copies at a time. Each copy fetches both files from TCGdex, bounded at 10 seconds, and puts each to Spaces, bounded at 3 seconds. It follows the shared-cache plan's breaker: a Spaces failure skips image copies for a minute and logs once at the start and once at the end. A queued copy skipped while the breaker is open, or dropped from a full queue, is simply queued again the next time a board asks for that card. An image TCGdex fails to serve is not tried again for 5 minutes, matching the freshness plan's `RetryAfter`. Copies are never expired or overwritten. A card whose art arrives later has a new address, so it gets its own new copy the first time a board shows it.

**Switched on only where the shared store is.** Hosted start-up builds the image cache inside the same helper that builds the tiered card store, from the same Spaces client, only when the four settings are complete and the playmat is online. It starts the listing and the copier in the background, tied to the server's context, and gives the cache to every account's board server. With no settings, partial settings, or `-offline`, no image cache exists. `/api/card` then answers exactly as today, with no fallback field and nothing copied. Desktop, daemon, dev and the board test server never set it. Hourly, the cache logs how many card answers pointed at the CDN and how many at TCGdex, and how many copies it made. That gives the operator a running measure for the 95 % success metric without parsing CDN logs. Clearing copies stays with the later `admin-cache-clearing` spec. It is kept easy: all images sit under one prefix with the TCGdex set in the path, the Spaces client gains the listing call that clearing also needs, and the in-memory set is keyed by object path, so a "forget this prefix" method is one small addition. Purging the CDN edge needs a DigitalOcean API token and is left to that spec.

Packaging is unchanged: the Docker build context and `.dockerignore` are not touched, so `internal/guides` stays in the build (convention `conventions/skills-text-and-packaging.md`).

## Component Breakdown

- **Spaces client (changed, from the shared card cache plan).** It gains two card-agnostic pieces. The first is listing: one page of keys under a prefix at a time (S3 ListObjectsV2), on the same signer, so the caller can bound each page with the shared-store timeout. The later admin-clearing spec reuses it to clear by prefix. The second is the bucket's public address: the CDN address for a DigitalOcean endpoint, worked out from the endpoint's region and the bucket, and the path-style object address for any other endpoint. Everything else (settings, `Get`, `Put` with content type, cache-control and public-read, typed errors) is used as that plan leaves it.
- **Fake Spaces (changed, from the shared card cache plan).** It learns the two things this spec's tests need. It answers listing requests with prefix and paging, with a knob for a small page size so paging is exercised. It also serves objects stored with public-read access to unsigned requests, as Spaces and its CDN do, while refusing unsigned reads of private objects. Tests can then load an image the way a browser does, and prove that card data stays private.
- **Image cache (new, inside the card database package).** It owns where each card image is loaded from on the hosted board, and the copies that make the CDN answer possible. It holds the in-memory set of images that have a public copy. That set is filled by a start-up listing of the image prefix and by its own completed copies. It answers "address and fallback for this TCGdex image" without any network call. For an image with no copy, it queues one. Its background copier fetches the two board sizes from TCGdex and puts them to Spaces as public, exact copies under the image prefix. It applies the shared-store breaker and timeouts, remembers images TCGdex failed to serve for 5 minutes, and copies only TCGdex's own art. It also keeps the hourly count of CDN versus TCGdex answers and copies made. It depends on the Spaces client and plain HTTP to TCGdex. It knows nothing about the card store, the freshness policy or games.
- **Shared-store breaker (reused, from the shared card cache plan).** It is the same rule the tiered store applies: each Spaces request is bounded at 3 seconds, the shared store is skipped for 1 minute after a failure, and a log line is written when an outage starts and when it ends. The image cache uses its own instance with an image label. If the tiered store's breaker is written inline, it is first factored into a small type inside the card package, with its behaviour unchanged, so there is one definition of the rule.
- **Board card endpoint (changed).** `/api/card` gains an optional image source. When one is set, the card's `image` is the address the image source gives, and a new `image_fallback` field carries the TCGdex address whenever the two differ. When none is set (desktop, daemon, dev, the board test server, hosted without the shared store), the response is byte-for-byte as today. Everything else about the endpoint (no-cache, error handling, other fields) is unchanged.
- **Board image display (changed, front end).** The three places the board draws card art (the card on the mat, the zoomed card, the preview in the choice dialog) share one rule: load `image`, switch once to `image_fallback` if that fails to load, and only then fall back to today's text stand-in, or the Energy token for an Energy card. The once-per-page card info fetch is unchanged.
- **Hosted wiring (changed, from the shared card cache plan).** The helper that builds the hosted card database from the environment also builds the image cache, from the same Spaces client, when the settings are complete and the playmat is online. It starts the cache's listing and copier in the background on the server's context, and hands the cache to every account's board server. It logs the public image address once at start-up. With no settings, partial settings or offline mode, there is no image cache and boards answer as today. Desktop, daemon and dev builds are untouched.
- **Hosted deployment and docs (changed).** The App Platform definition's comments, the container image comment and the design document's hosted section and operator setup describe that card images are copied to the bucket and served from its CDN once the bucket's CDN is enabled. No new setting is added. The card cache knowledge entry stops saying images are never cached by the playmat.
- **Unchanged.** The card store, tiered store, freshness policy and `DB` interface, the game, the importer, the MCP tools (which carry no image addresses) and `cardstest.FakeDB` are unchanged.

## Data Structures & Interfaces

**Unchanged contracts this plan relies on.** From the shared card cache plan: `spaces.Config`, `spaces.FromEnv`, `spaces.New`, `(*spaces.Client).Get` and `Put`, `spaces.PutOptions{ContentType, CacheControl, Public}`, `spaces.ErrNotFound`, `*spaces.Error`, the `spacestest.Server` fake and its knobs (Fail, Deny, Hang, Close, request counts), the four `TCGREPLAY_SPACES_*` settings, and the private `tcgdex/` prefix. From the freshness plan: `cards.Store`, `cards.Entry`, `cards.ErrNotCached`, `cards.RetryAfter` and the `DB` interface. None changes shape. `cardstest.FakeDB` and every MCP tool schema are unchanged.

**`spaces.Client.List` and `spaces.Config.PublicBase` (new, `internal/spaces`).** These are one listing call on the existing signer and the bucket's public address. Both are card-agnostic.

```go
// ListPage is one page of keys under a prefix (S3 ListObjectsV2).
type ListPage struct {
    Keys []string // full object keys, in key order
    Next string   // continuation token; "" on the last page
}

// List returns the page of keys under prefix that follows token
// ("" for the first page).
func (c *Client) List(ctx context.Context, prefix, token string) (ListPage, error)

// PublicBase is where public-read objects are served:
// https://<bucket>.<region>.cdn.digitaloceanspaces.com for a DigitalOcean
// endpoint, otherwise <endpoint>/<bucket> (path style). No trailing slash.
func (c Config) PublicBase() string
```

**`spacestest.Server` additions (test support).** It answers ListObjectsV2 with prefix, continuation token and max-keys, and has a `PageSize(n)` knob. It serves unsigned GETs of objects stored public-read, and refuses unsigned GETs of private or missing objects with 403 `AccessDenied`, as Spaces does. `Public(key) bool` reports an object's access.

**`cards.ImageCache` (new, exported from `cards`).** This is the hosted board's image source and the copier behind it.

```go
type ImageCache struct {
    Spaces *spaces.Client
    Public string           // public base of the bucket (Config.PublicBase)
    Source string           // copyable image prefix; default "https://assets.tcgdex.net"
    HTTP   *http.Client     // fetches from Source
    Now    func() time.Time // default time.Now; tests inject a clock
    // unexported: known set, pending set, failed times, queue, breaker, counters
}

func NewImageCache(c *spaces.Client, public string) *ImageCache

// Run lists the copies already in the bucket, then copies queued images,
// and logs the hourly counts, until ctx is done.
func (ic *ImageCache) Run(ctx context.Context)

// Image returns where the board should load src from, and the fallback
// when that differs from src. It never blocks or touches the network;
// an image without a copy is queued for copying.
func (ic *ImageCache) Image(src string) (image, fallback string)

const (
    ImagePrefix    = "images/"
    ImageCacheControl = "public, max-age=604800"
)
var ImageFiles = []string{"low.webp", "high.webp"}
```

**Image object layout in the bucket (serialization boundary).** An image whose TCGdex base address is `https://assets.tcgdex.net/<path>` is copied to `images/<path>/low.webp` and `images/<path>/high.webp`, for example `images/en/swsh/swsh12.5/157/low.webp`. Each object holds the TCGdex bytes unchanged, with `Content-Type: image/webp`, `Cache-Control: public, max-age=604800` and `x-amz-acl: public-read`. The board loads it at `<public base>/images/<path>/low.webp`. The `images/` prefix is reserved for card images, and `tcgdex/` stays private card data. An image counts as copied only when both objects exist.

**`webui.ImageSource` and `Server.Images` (new, `internal/webui`).** This is the optional hook the card endpoint asks for each image. `*cards.ImageCache` satisfies it, and nil keeps today's behaviour.

```go
type ImageSource interface {
    Image(src string) (image, fallback string)
}

type Server struct {
    // ... existing fields ...
    Images ImageSource // hosted with the shared store only; nil elsewhere
}
```

**`/api/card` response (changed, board protocol).** `CardInfo` gains one optional field. `image` keeps its meaning, a base address to which the board appends `/low.webp` or `/high.webp`, but may now be the CDN address.

```json
{"id": "swsh12.5-157", "image": "https://<bucket>.<region>.cdn.digitaloceanspaces.com/images/en/swsh/swsh12.5/157",
 "image_fallback": "https://assets.tcgdex.net/en/swsh/swsh12.5/157", "...": "..."}
```

`image_fallback` is omitted whenever `image` is already the TCGdex address, which is always the case without an image source. Older boards ignore the field and still load `image`.

## Implementation Detail

**A non-blocking front, a background back.** The image cache has two halves with a strict boundary between them. The front, the method `/api/card` calls, only reads and writes in-memory maps under a short mutex. It reads the known set, the pending set and the failed-image times, pushes to a buffered channel without blocking, and bumps two counters. It never does I/O, so it is safe on any request path. The back is the goroutines `Run` starts: one start-up listing, then a small fixed pool of copy workers draining the channel, and an hourly ticker for the count log. A developer reading the cache sees the whole image story in one file: the address mapping, the known set, the queue, the copy of one image, and the breaker calls. The card store and freshness code are not touched, so a reader of a card lookup never meets images.

**Start-up listing before copying.** `Run` first lists the image prefix page by page, each page bounded by the shared-store timeout and gated by the breaker. It records an image as known once both of its files are listed. Workers start only when the listing ends, whether it succeeded or failed, and each worker re-checks the known set before copying. A board opened during the listing therefore gets TCGdex addresses for a few seconds and queues those images, but an image already in the bucket is not uploaded again. If the listing fails (Spaces down at start-up), the cache carries on with an empty set. Any image then viewed is copied again over its existing objects, which is harmless because a put of the same bytes is idempotent, and is cheaper than retrying the listing.

**Copying one image.** A worker builds the two TCGdex file addresses and the two object keys from the image's base address, a pure mapping that is tested on its own. It fetches each file from TCGdex with a 10-second bound, accepting only a 200 with a non-empty body. It then puts each to Spaces with the image content type, cache-control and public-read, each put bounded at 3 seconds and gated by the breaker. Only when both puts succeed is the image added to the known set. A TCGdex failure records the image as failed, so the front does not queue it again for 5 minutes. A Spaces failure trips the breaker and is not recorded as failed, so the image is queued again by the next view after the breaker window. Copies are never deleted or overwritten by this code. A borrowed Energy picture is copied once under its Crown Zenith path, shared by every Energy that borrows it.

**Address mapping is the only TCGdex-specific rule.** An address is copyable only when it starts with the configured source prefix plus `/`, and its remaining path is a clean relative path: no empty, `.` or `..` segment, no query and no fragment. Anything else is answered unchanged with no fallback and never queued, which enforces "the card source's own images only" in one place. The source prefix is a field, so tests point it at an `httptest` image host.

**Board fallback as one shared helper.** The three image spots in the board each hold a small per-card state that moves from the primary address, to the fallback, to "broken". That state lives in one helper in the board's store module, which gives the `src` for a size, and one error handler, so the three components change by a few lines each and behave identically. The Energy token and text stand-in keep their current conditions, now reached only after both addresses fail. The once-per-page card info cache is not touched.

**Patterns followed, not introduced.** These are carried over from the upstream plans and the codebase: the optional hosted-only hook on `webui.Server` (like `Public` and `Limit`), set only in hosted wiring; the injected `Now` clock; the shared-store breaker and its edge-triggered logs; package-internal tests in `internal/cards` with `httptest` fakes counting requests per path; the shared `spacestest` fake; `captureLog` and `t.Setenv` in hosted tests; and Playwright tests against `boardtest`, using route interception for the card endpoint and image hosts. One pattern is new to the board suite: intercepting the board's own `/api/card` responses to inject image fields, because the fake card pool has no art. No third-party modules are added, and `go.mod` and `package.json` are unchanged.

## Dependencies

- **Design documents this plan was built on: none.** The spec references no design documents.
- **Upstream plan `20261104082020-109dceb1-tcgdex-spaces-cache` — must be implemented and merged before this plan starts.** It provides the `internal/spaces` client (`Config`, `FromEnv`, `New`, `Get`, `Put`, `PutOptions`, `ErrNotFound`, `*Error`) and its signer, the `spacestest` fake, the four `TCGREPLAY_SPACES_*` settings, the private `tcgdex/` prefix, the shared-store breaker and its constants, the hosted helper that builds the card database from the environment, and the human task that creates the bucket and access key. This plan adds listing and the public address to the client, list and public reads to the fake, and the image cache to the hosted helper. It changes none of the existing shapes. It may factor the tiered store's breaker into a reusable type, without changing its behaviour.
- **Upstream plan `20261104090640-51bd1f36-card-cache-freshness` — must land first (it is the shared cache plan's own prerequisite).** It provides the rule that an imageless card is refreshed after 24 hours, which is how late artwork reaches `/api/card` and gets its own copy, and `RetryAfter` (5 minutes), reused for images TCGdex fails to serve. No change.
- **Downstream spec `20261104090640-a5a74fab-admin-cache-clearing`.** It will reuse `spaces.Client.List` added here, clear images by the `images/` prefix (one set by `images/<lang>/<series>/<set>/`), and need a small "forget this prefix" method on the image cache plus a CDN purge. This plan keeps those easy and does not build them.
- **DigitalOcean Spaces and its CDN (external service).** The bucket holds the public image copies, and its CDN serves them to browsers. It needs the bucket's CDN enabled by an operator, and an access key allowed to list and to put public-read objects. No real bucket or CDN is available to the automated tests, which use the fake. Real behaviour is covered by manual checks.
- **TCGdex assets (`https://assets.tcgdex.net`, external service).** It is the only image source, both for copies and as the board's fallback. It is used as the board uses it today (`low.webp`, `high.webp`). No change.
- **DigitalOcean App Platform (existing deployment).** No new settings. Comments only change in its definition.
- **Go standard library only.** It provides `net/http`, `encoding/xml` (listing responses), `net/url`, `path`, `sync`, `time` and `log`, with `net/http/httptest` in tests. `go.mod` is unchanged.
- **`internal/cards` (changed).** It gains the image cache, and possibly the breaker factored out of the tiered store. The `Store` contract, freshness policy and `DB` interface are unchanged.
- **`internal/spaces`, `internal/spaces/spacestest` (changed).** Listing, public address, and the fake's list and public reads.
- **`internal/webui` (changed).** The optional image source and the `image_fallback` field on `/api/card`.
- **`cmd/tcgreplay` (changed).** The hosted helper builds and runs the image cache. Desktop constructors are unchanged.
- **`web/` (changed).** Three components and the store module gain the fallback, and one Playwright spec is added. `boardtest`, `webtest` and `cardstest` are unchanged. `package.json` is unchanged.
- **`.do/app.yaml`, `Dockerfile`, `TCG Review Playmat Design and Build Plan.md` (changed, comments and docs only).** The Docker build context is not changed, so `internal/guides` stays in it.
- **Knowledge entries relied on:** `gotchas/board-card-info-per-page-load.md`, which makes "later views" mean later page loads and keeps the card info cache unchanged; `learnings/card-image-sources.md`, whose other image sources stay unused by constraint; `gotchas/tcgdex-cache-never-expires.md`, whose closing paragraph ("Card images are not cached by the playmat at all") this plan updates; `gotchas/game-listeners-under-lock.md`, honoured because the image source never does I/O; and `conventions/skills-text-and-packaging.md` (packaging half).

## Testing Approach

**Kinds of tests.** There are four layers, all run under the race detector like the upstream plans' tests, plus one browser test:

- **Spaces client and fake unit tests.** Listing is tested against the fake: keys under a prefix only, in order, across several pages with a small page size, and an empty prefix. Refused credentials and server errors come back as typed errors. The public address is a pure function, tested for a DigitalOcean endpoint and for a path-style one. The fake's new behaviour is tested too: an unsigned read of a public-read object succeeds, and an unsigned read of a private or missing object is refused.
- **Image cache tests in the card package.** These are package-internal and drive a real `ImageCache` against the fake Spaces and an `httptest` image host that serves `low.webp` and `high.webp`, counts requests per path, and can fail or be slow. The clock is injected. Background work is awaited through an unexported settle helper, never by sleeping.
- **Board endpoint tests in the board server package.** These use the existing `httptest` style with `cardstest.FakeDB` holding a card that has an image. They run once with a stub image source and once with none.
- **Hosted wiring tests.** These set the shared-store environment against the fake and check whether every account's board gets an image source, and what the start-up log says, using the existing `captureLog` and `hostedConfig` helpers.
- **One Playwright browser test.** It runs against the existing board test server in all three device profiles. It intercepts the board's own `/api/card` answers to give a card a CDN `image` and a TCGdex `image_fallback`, and intercepts both hosts. It checks which address the board ends up showing.

**Where coverage concentrates.** The image cache gets the most coverage, because the spec's behaviour lives there. Its tests assert outcomes and request counts on the fakes, never internal state:

- **Missing image filled in.** The first `Image` call for an uncopied TCGdex address returns that address with no fallback. Once background work settles, the fake Spaces holds both files at `images/<path>/low.webp` and `…/high.webp`, byte for byte as served, public-read, with `image/webp` and the cache-control header. The next `Image` call returns `<public base>/images/<path>` with the TCGdex address as fallback. An unsigned read of `<public base>/images/<path>/low.webp` from the fake returns the image, the way a browser through the CDN would. The test asserts the whole cycle finishes well inside the spec's 60 seconds.
- **Images with a copy make no source requests.** After a start-up listing that finds both files, `Image` returns the CDN address at once, and the image host records zero requests. An image with only one of its two files listed is treated as uncopied.
- **Listing.** Copies already in the bucket, spread across several listing pages, are all known after `Run` starts. An image queued during the listing is not uploaded again if the listing finds it.
- **Only TCGdex art.** An address on another host, or with `..`, a query or an empty path, is returned unchanged, with no fallback and no copy. An empty address stays empty and is not queued. A card whose image appears later, a new address, gets its own copy.
- **Outage and failure.** With the fake Spaces closed, hanging, failing or refusing credentials, `Image` keeps answering instantly with TCGdex addresses, at most one Spaces attempt is made per breaker window, a hang delays no `Image` call, and the outage is logged once when it starts and once when it ends. An image the source fails on (404, 500 or timeout) is fetched at most once per 5 minutes, and is copied once the clock passes that. A full queue drops without blocking.
- **Copies kept.** No code path deletes or overwrites a known copy: a second view of a copied image makes no Spaces or source request.
- **Counts.** The hourly log line reports CDN answers, TCGdex answers and copies made, and is not written for an hour in which nothing happened.

Board endpoint tests assert that, with no image source, the `/api/card` body is identical to today's: same `image`, no `image_fallback`. With a stub, `image` and `image_fallback` carry the stub's answer, and a card without an image has neither. Hosted wiring tests assert four cases. With complete settings and online, the boards get an image source, and the start-up log names the public image address. With no settings, partial settings or `-offline`, there is no image source, no request is made to the fake Spaces or image host for images, and no warning beyond the shared-cache plan's own is logged. The browser test asserts three cases. When the CDN address loads, the card's `<img>` uses it and no request goes to the TCGdex host. When the CDN host fails, the card on the mat, the zoom and the dialog preview all show the TCGdex image. When both fail, today's text stand-in or Energy token appears.

**Where tests follow existing conventions.** Card-package tests stay package-internal, with `httptest` fakes counting per path and an injected clock, following the freshness plan's style. Spaces tests extend the shared fake rather than adding handlers. Board endpoint tests follow the existing `/api/card` test. Hosted tests reuse `t.Setenv`, `captureLog` and `hostedConfig`, and the existing check that a clean start logs no "warning". The browser test follows `web/e2e` fixtures: reset per test, one worker. Every existing Go and Playwright test must pass unchanged, which guards "Images load as today without the shared store" for desktop and the board test server.

**Deliberate gaps.** No automated test reaches real DigitalOcean Spaces, its CDN, or `assets.tcgdex.net`. No bucket, CDN or credentials are available, and the real card source is never called in tests. Real-service behaviour is left to the manual checks below: the CDN address format, public-read in a private bucket, listing permission for the key, and upload latency within 3 seconds. The board's fallback is tested in Chromium only, as the whole browser suite is. Real iOS Safari stays a manual check, as it already is for the suite.

**Success metrics.**
- "Over a week of hosted use, card images load from the shared store's delivery address for at least 95% of image views": **Manual — captured in the implementation test plan.** It depends on a week of real use on the real CDN. The hourly count log (CDN versus TCGdex answers) supports it, and its counting is covered by a behavioural test.

**Manual reviews.**
- **Manual — captured in the implementation test plan.** Real CDN end to end: with the bucket's CDN enabled, open a fresh game on the hosted board. Images first load from `assets.tcgdex.net`. Within 60 seconds the bucket holds `images/…/low.webp` and `high.webp` as public objects, and after a reload every card image loads from `<bucket>.<region>.cdn.digitaloceanspaces.com`, with no requests to `assets.tcgdex.net`.
- **Manual — captured in the implementation test plan.** Card data stays private: an anonymous request for a `tcgdex/` object, through the origin and through the CDN, is refused, while an `images/` object loads.
- **Manual — captured in the implementation test plan.** Broken CDN fallback on the real deployment: with the CDN address blocked in the browser (for example by devtools request blocking), every card image still shows, loaded from `assets.tcgdex.net`.
- **Manual — captured in the implementation test plan.** No shared store: a hosted start with no `TCGREPLAY_SPACES_*` variables, and a desktop build, load every image from `assets.tcgdex.net`, and the bucket gains no `images/` objects.
- **Manual — captured in the implementation test plan.** Restart keeps copies: after a redeploy, the start-up listing finds the existing copies, and the first board load serves them from the CDN without re-uploading.
- **Manual — captured in the implementation test plan.** Real iOS Safari shows CDN images, and falls back to TCGdex when the CDN is blocked.

## Milestones & Tasks

### Milestone 1: The board can show a card image from a second address

**What changes**: Nothing a player sees changes yet, but the board and its card endpoint learn to carry two addresses for a card image: the one to load, and a fallback to use if that fails. The board tries the first, then the fallback, and only then shows the text stand-in or Energy token it shows today. This happens on the card on the mat, the zoomed card and the dialog preview alike. No image source is plugged in yet, so every board, desktop and hosted, keeps loading images from the card source exactly as today, and the card endpoint's answer is unchanged. It lands first because it is the board half of the "broken copy falls back" behaviour, and it can be proven in the browser suite on its own.

**Validation point**: The browser test shows a card image loaded from its first address, the fallback used when the first address fails, and the stand-in when both fail, in all three device profiles. Card endpoint tests show the response is unchanged with no image source, and carries both fields with one. The existing Go and browser suites pass unchanged.

#### - [x] Task: Carry an image fallback on the card endpoint
**Id:** e52e31a8-9a3f-49f2-9001-313ea09e2c9d
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Give the board server an optional image source, which the card endpoint asks where each card's image should load from. When the answer differs from the card source's address, the endpoint also returns that address as a fallback. Without an image source the endpoint answers exactly as today, so desktop, dev, hosted and the board test server are unaffected until hosted wiring plugs one in.

*Technical detail:* [context.md#task-carry-an-image-fallback-on-the-card-endpoint](./context.md#task-carry-an-image-fallback-on-the-card-endpoint)

**Acceptance criteria**:
- [x] With no image source, a card's answer from the card endpoint is the same as before this change: the same image address and no fallback field.
- [x] With an image source that gives a different address, the answer's image is that address and its fallback is the card source's address.
- [x] A card without an image has neither an image nor a fallback in its answer, and the image source is not asked about it.
- [x] The card endpoint still answers without waiting on the network for the image decision.

#### - [x] Task: Fall back to the card source when a board image fails
**Id:** a00bab29-1a24-4124-b3bc-00b521066320
**Repo:** tcgreplay
**Depends on:**
- e52e31a8-9a3f-49f2-9001-313ea09e2c9d — Carry an image fallback on the card endpoint
**Execution:** agent

Make the board's three image spots (the card on the mat, the zoomed card and the preview in the choice dialog) follow one rule. Load the image address, switch once to the fallback address if that fails, and only then show today's text stand-in or Energy token. A browser test proves it in every device profile by giving a card both addresses and making each host succeed or fail.

*Technical detail:* [context.md#task-fall-back-to-the-card-source-when-a-board-image-fails](./context.md#task-fall-back-to-the-card-source-when-a-board-image-fails)

**Acceptance criteria**:
- [x] When a card's image address loads, the board shows it and makes no request to the fallback address.
- [x] When the image address fails, the card on the mat, the zoomed card and the dialog preview each show the image from the fallback address.
- [x] When both addresses fail, the card shows today's text stand-in, or the Energy token for an Energy card.
- [x] A card with no fallback behaves exactly as today, and the existing browser tests pass unchanged.

### Milestone 2: The playmat can keep public copies of card images in the shared store

**What changes**: Nothing is switched on yet in the running playmat. The playmat gains an image cache that knows which card images have a public copy in DigitalOcean Spaces. It tells the board to use the copy's CDN address when there is one, and the card source's address otherwise. Each image without a copy is copied in the background, byte for byte and only from the card source's own art, so the next page load uses the copy. The Spaces client learns to list the bucket and to work out its public CDN address, and the stand-in Spaces used by tests learns both too. Shared-store outages are absorbed with the same timeout and one-minute skip as card data, and logged once.

**Validation point**: Tests with the stand-in Spaces and a fake image host show the whole cycle. A first view gets the source address, and both image files land in the stand-in as public, unchanged copies well within 60 seconds. Later views get the CDN address with no source requests. Copies already in the bucket are found by the start-up listing, non-TCGdex addresses are never copied, and every outage mode leaves answers instant and is logged once.

#### - [x] Task: List the bucket and work out its public address
**Id:** a344ea2e-51bf-452f-ac15-17bf0c6277dc
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Extend the shared Spaces client with two card-agnostic pieces: listing the keys under a prefix, one page at a time, and the bucket's public address (its DigitalOcean CDN address, or the plain object address for other endpoints). Teach the stand-in Spaces to answer listings with paging, and to serve public objects to unsigned requests while refusing private ones, as the real service does. This task needs the shared card cache plan to have landed, since it extends that plan's client and stand-in.

*Technical detail:* [context.md#task-list-the-bucket-and-work-out-its-public-address](./context.md#task-list-the-bucket-and-work-out-its-public-address)

**Acceptance criteria**:
- [x] Listing a prefix returns every key under it and none outside it, in key order, across as many pages as the stand-in splits it into.
- [x] A listing refused for its credentials, or failed by the server, is reported with its status and S3 error code.
- [x] The public address for a DigitalOcean endpoint such as `https://lon1.digitaloceanspaces.com` with bucket `b` is `https://b.lon1.cdn.digitaloceanspaces.com`, and for any other endpoint it is the endpoint followed by the bucket.
- [x] The stand-in serves an object written as public to a request with no signature, and refuses an unsigned request for a private or missing object.

#### - [x] Task: Copy card images to the shared store in the background
**Id:** d8aade03-4a48-4fc6-9b32-c9ed93102224
**Repo:** tcgreplay
**Depends on:**
- a344ea2e-51bf-452f-ac15-17bf0c6277dc — List the bucket and work out its public address
**Execution:** agent

Add the image cache to the card database package. Asked about a card image, it answers at once from memory: the CDN address of the image's copy when one exists, otherwise the card source's address, queuing the image for copying. A small background copier fetches the two image sizes the board uses from the card source and stores them unchanged as public objects under the image prefix. It uses the same timeout, one-minute skip and once-only logging as the shared card cache. Only the card source's own art is ever copied.

*Technical detail:* [context.md#task-copy-card-images-to-the-shared-store-in-the-background](./context.md#task-copy-card-images-to-the-shared-store-in-the-background)

**Acceptance criteria**:
- [x] The first request for an image without a copy returns the card source's address with no fallback. Shortly afterwards, both of its sizes exist in the shared store as public, byte-identical copies with the image content type and cache-control, well within 60 seconds.
- [x] Once copied, the image's address is its CDN address with the card source as fallback, and later requests make no request to the card source.
- [x] An address that is not the card source's own art, or is malformed, is returned unchanged and never copied. An empty address stays empty.
- [x] With the shared store unreachable, hanging, failing or refusing credentials, answers stay instant and use the card source's address. At most one shared-store attempt is made per minute, and the outage is logged once when it starts and once when it ends.
- [x] An image the card source fails to serve is not fetched again for 5 minutes, and is copied on a request after that.
- [x] A copied image is never deleted or rewritten by the cache, and a card whose art appears later gets its own new copy.

#### - [x] Task: Find existing copies at start-up and count image answers
**Id:** dc3ff181-4082-480b-9659-2308f38e304f
**Repo:** tcgreplay
**Depends on:**
- d8aade03-4a48-4fc6-9b32-c9ed93102224 — Copy card images to the shared store in the background
**Execution:** agent

Make the image cache list the image prefix when it starts, so images copied before a restart are served from the CDN straight away and are not uploaded again. Copying waits for the listing to finish, and carries on with an empty list if the shared store cannot be listed. Once an hour, the cache logs how many card answers pointed at the CDN and how many at the card source, and how many copies it made, as the running measure for the success metric.

*Technical detail:* [context.md#task-find-existing-copies-at-start-up-and-count-image-answers](./context.md#task-find-existing-copies-at-start-up-and-count-image-answers)

**Acceptance criteria**:
- [x] After start-up, every image whose two sizes are already in the bucket, across several listing pages, is answered with its CDN address and makes no request to the card source.
- [x] An image with only one of its two sizes in the bucket is treated as not copied, and is copied when next asked for.
- [x] An image asked for while the listing is still running is not uploaded again if the listing finds it.
- [x] If the listing fails, the cache still answers and copies, and the failure is logged once.
- [x] Each hour with activity produces one log line with the counts of CDN answers, card source answers and copies made. An hour with none produces no line.

### Milestone 3: The hosted board loads card images from the playmat's own CDN

**What changes**: On the hosted playmat with the shared store configured, card images now come from the bucket's CDN. An image without a copy still shows at once from the card source and is copied in the background, so it loads from the CDN on the next page load. If the CDN cannot be reached, images load from the card source as before. Without the shared store, offline, and on desktop and dev builds, images load from the card source exactly as today, and nothing is copied. The hosted docs and deploy comments explain this, an operator enables the bucket's CDN, and the hourly log line shows how many card images were served from the CDN versus the card source.

**Validation point**: Hosted wiring tests show the image cache switched on only with complete settings while online, and the public image address logged. After the operator enables the CDN, the manual checks in the implementation test plan confirm the real CDN end to end: copies appearing within 60 seconds, CDN loads after a reload, fallback with the CDN blocked, card data still private, and the week-long 95 % measure.

#### - [x] Task: Switch on image copies in hosted mode
**Id:** 9bb9d5f0-aead-4d7a-a25d-af1eef2907c7
**Repo:** tcgreplay
**Depends on:**
- e52e31a8-9a3f-49f2-9001-313ea09e2c9d — Carry an image fallback on the card endpoint
- dc3ff181-4082-480b-9659-2308f38e304f — Find existing copies at start-up and count image answers
**Execution:** agent

In hosted start-up, build the image cache from the same Spaces client as the shared card cache, only when the four settings are complete and the playmat is online. Start it in the background, and give it to every account's board. Log the public image address once at start-up. With no settings, partial settings or offline mode there is no image cache, and boards load images from the card source as today. Desktop, daemon and dev builds are untouched.

*Technical detail:* [context.md#task-switch-on-image-copies-in-hosted-mode](./context.md#task-switch-on-image-copies-in-hosted-mode)

**Acceptance criteria**:
- [x] With all four settings pointing at a reachable store and the playmat online, a board's card answer for an uncopied image gives the card source's address, and the image is then copied to the shared store. A later answer gives the CDN address.
- [x] The start-up log names the public address card images are served from.
- [x] With no settings, partial settings or offline mode, card answers are as today, no image is copied, and no new warning is logged.
- [x] The health check answers while the start-up listing is still running.
- [x] Desktop and dev builds behave exactly as before, whatever shared-store settings are in their environment.

#### - [x] Task: Document CDN card images for operators
**Id:** 79912466-672b-48d5-8ce7-d9a7a30a79fe
**Repo:** tcgreplay
**Depends on:**
- 9bb9d5f0-aead-4d7a-a25d-af1eef2907c7 — Switch on image copies in hosted mode
**Execution:** agent

Describe the new behaviour where operators look. The App Platform definition's comments, the container image comment and the design document's hosted section say that, with the shared store set, card images are copied to the bucket under `images/` and served from its CDN, falling back to the card source. The operator setup gains the step of enabling the bucket's CDN. No setting is added, and the build context is not changed.

*Technical detail:* [context.md#task-document-cdn-card-images-for-operators](./context.md#task-document-cdn-card-images-for-operators)

**Acceptance criteria**:
- [x] The App Platform definition's comments say card images are served from the bucket's CDN when the shared store is set, and from the card source otherwise.
- [x] The design document's hosted section describes the image copies (prefix, public access, the card source as the only source, kept until cleared), the hourly count line, and the fallback.
- [x] The operator setup says how to enable the bucket's CDN, and that the access key must be able to list the bucket and write public objects.
- [x] The Docker build context is unchanged, so the guides are still built in.

#### - [x] Task: Update the card image knowledge entries
**Id:** 8d8941d1-6d35-4c40-bb56-dceb0eae032c
**Repo:** tcgreplay
**Depends on:**
- 9bb9d5f0-aead-4d7a-a25d-af1eef2907c7 — Switch on image copies in hosted mode
**Execution:** agent

The card cache knowledge entry still says the playmat never caches images, and the board card info entry does not mention image addresses. Update them through the knowledge workflow. They should say where hosted image copies live, when the board gets a CDN address versus the card source's, that a copy made during a page's life is used from the next page load, and that copies are kept until an admin clears them.

*Technical detail:* [context.md#task-update-the-card-image-knowledge-entries](./context.md#task-update-the-card-image-knowledge-entries)

**Acceptance criteria**:
- [x] A reader of the card cache entry learns that hosted mode with the shared store copies card images to `images/` in the bucket and serves them from its CDN, and that desktop and dev builds still load images from the card source.
- [x] A reader of the board card info entry learns that the image address is fixed for the page's life, so a new copy is used from the next page load, and that the board falls back to the card source if the CDN fails.
- [x] The entries say that image copies are kept until cleared, and that clearing one also needs the playmat restarted (or the later admin clearing) so it stops pointing at the copy.

#### - [x] Task: Enable the CDN on the Spaces bucket
**Id:** db22391d-851d-499e-8713-44aec042aff3
**Repo:** tcgreplay
**Depends on:**
- 79912466-672b-48d5-8ce7-d9a7a30a79fe — Document CDN card images for operators
**Execution:** human — needs the DigitalOcean account's control panel to enable the Spaces CDN and check the access key's permissions

Enable the CDN on the Spaces bucket the shared card cache uses, and confirm its access key can list the bucket and write public objects. Then redeploy. Until the CDN is enabled, copies are still made, but the board's CDN addresses fail and it falls back to the card source, so players see no difference.

*Technical detail:* [context.md#task-enable-the-cdn-on-the-spaces-bucket](./context.md#task-enable-the-cdn-on-the-spaces-bucket)

**Acceptance criteria**:
- [x] The bucket's CDN is enabled, and its address matches the public image address in the playmat's start-up log.
- [x] After a redeploy, the start-up log shows no shared-store warning or failure for card images.
- [x] An image copied by the playmat loads from the CDN address in a browser with no sign-in.

## Open Questions

- **Do the upstream plans' implementations leave the seams this plan assumes?** This plan expects four things: a hosted helper (`hostedCardDB` or similar) that builds one `*spaces.Client` when the settings are complete and the playmat is online; a tiered-store breaker that can be factored into a reusable type with its log text and timing unchanged; a signer that can sign a request with a query string; and a fake Spaces whose handler can take bucket-level requests. This depends on how the shared card cache plan's tasks were actually implemented, which only shows once it has landed. *What to do:* adapt names to what landed, keeping the behaviour this plan specifies. If adding image copies would mean changing the `Store`/`Entry`/`ErrNotCached` contract, the four settings, or the breaker's behaviour for card data, STOP and ask the user.
- **Does real DigitalOcean Spaces accept this access key for listing and public-read writes, and serve the copies at the derived CDN address?** The plan assumes a bucket-scoped read/write key may list the bucket and PUT with `x-amz-acl: public-read`, that public objects in a private bucket are served by the CDN at `https://<bucket>.<region>.cdn.digitaloceanspaces.com/<key>`, and that a high-quality image uploads within the 3-second bound from App Platform. Only the real service shows this, during the operator's CDN task and the manual checks. *What to do:* if the key cannot set public-read or list, STOP and ask the user whether to require a wider key (docs only) or change how copies are made public. If the CDN address differs from the derived one, STOP and ask whether to add a `TCGREPLAY_SPACES_CDN_URL` setting. If uploads routinely exceed 3 seconds, STOP and ask whether image puts may use a longer bound than card data.

No other questions are left for implementation time. Every other decision is settled in the sections above or recorded as a drafting assumption.

## Out of Scope

- **Resizing, recompressing or otherwise changing card images** (spec Non-Goal). Copies are the TCGdex bytes, unchanged.
- **Any image source other than TCGdex** (spec constraint). The backups in knowledge `learnings/card-image-sources.md` (malie.io, pokemontcg.io, Limitless) stay unused, and cards TCGdex has no art for keep their text stand-in, as today.
- **Clearing image copies, and purging the CDN edge** — `20261104090640-a5a74fab-admin-cache-clearing`. This plan keeps it easy (one `images/` prefix with the set in the path, `spaces.Client.List`, a known set keyed by path) but adds no delete, forget or purge. Until then, removing a copy means deleting its objects by hand and restarting the playmat.
- **Desktop, daemon and dev builds using the CDN.** Only hosted mode with the shared store copies images, and every other build loads them from TCGdex as today.
- **Pushing new image addresses to boards already open.** A copy made while a page is open is used from the next page load (knowledge `gotchas/board-card-info-per-page-load.md`). Changing the board's update protocol is not part of this work.
- **A custom CDN domain, or a configurable image prefix, cache lifetime or copier limits.** The CDN address is derived from the existing settings, and the prefix, `Cache-Control` and copier limits are package constants.
- **Copying images the board has not shown.** There is no bulk seeding of a set's or the whole catalog's art. The first view of each image after a fresh bucket still loads it from TCGdex.
- **Other image formats or sizes** (PNG, JPG, other qualities). Only the `low.webp` and `high.webp` files the board uses are copied.
- **Retrying the start-up listing, or queuing copies across a shared-store outage.** A failed listing leaves an empty known set, so images are copied again as viewed. Skipped copies are queued again on the next view.
- **Exact per-view image metrics.** The hourly log counts card answers (one per card per page load), not individual image loads in the browser. The week-long 95 % figure is checked by hand against it and CDN usage.
- **The fan-site-tolerance risk** (spec Technical Approach). Mirroring art to a public CDN carries the same caveat as TCGdex itself, and nothing in this plan changes that.

## Changelog

### 2026-11-04 — Task: Carry an image fallback on the card endpoint

**What was done**: `webui.Server` gained an optional `Images ImageSource` hook (an interface with `Image(src) (image, fallback string)`, documented as never blocking or touching the network). `/api/card` asks it for a card's image when set and the card has one, and returns the new `image_fallback` field only when the answer differs from the TCGdex address. With no hook the answer is unchanged.

**Deviations**: Implementation started before the dependency `20261104082020-109dceb1-tcgdex-spaces-cache` was implemented: it was in progress (9/10 tasks complete; its code is merged, only its human bucket-creation task is open). The orchestrator approved continuing past it.

**Files changed**:
- `internal/webui/server.go`
- `internal/webui/webui_test.go`

**Discoveries**: The `webui` tests use the plain `testing` package (no testify). To check that a JSON key is absent, decode the answer into a map.

### 2026-11-04 — Task: Fall back to the card source when a board image fails

**What was done**: The board's store module gained `imageSrc(info, size, stage)` and `nextImageStage(info, stage)`. With them, the card on the mat, the zoom and the dialog preview each load `image`, then `image_fallback` once if that fails, and only then show the text stand-in or Energy token. The preview gained an `onerror` it lacked. The new `web/e2e/images.spec.js` intercepts `/api/card` to add a CDN image and a TCGdex fallback, and routes both hosts. It covers CDN success, CDN failure (card, zoom, and the desktop preview) and both failing, in all three profiles.

**Deviations**: This run started before the dependency `20261104082020-109dceb1-tcgdex-spaces-cache` was implemented: it was in progress (9/10 tasks complete). Card.svelte never reset its old `broken` flag when the card shown changed. The new `stage` resets on `card.def` through an `$effect`, as Zoom already did. The zoom in the spec is opened with a mouse click on the benched Riolu in every profile, not through the deck list: a click zooms in touch profiles too, and it is simpler.

**Files changed**:
- `web/src/lib/store.svelte.js`
- `web/src/components/Card.svelte`
- `web/src/components/Zoom.svelte`
- `web/src/components/Modal.svelte`
- `web/e2e/images.spec.js`

**Discoveries**: The `web/e2e/fixtures.js` page fixture loads the board before the test body runs. A test that adds `page.route` interceptions for card info must reload the page so the info is fetched again through them. The store's `requested` set lives in the module, so a reload clears it. In the full suite, `harness.spec.js` "a browser dialog fails the test" shows as an expected failure (it is marked `test.fail()`), not as a real one.

### 2026-11-04 — Task: List the bucket and work out its public address

**What was done**: `spaces.Client` gained `List(ctx, prefix, token) (ListPage, error)`, a signed ListObjectsV2 request at the bucket level that sets `Next` only on a truncated page. `do` was split so object and bucket requests share one signed `send`. `spaces.Config` gained `PublicBase()`: the bucket's CDN address for a `<region>.digitaloceanspaces.com` endpoint, otherwise `<endpoint>/<bucket>`. The `spacestest` fake now answers listings with prefix, continuation token, `max-keys` and a `PageSize(n)` knob, counted as `Requests("LIST", prefix)`. It serves unsigned GETs of public-read objects, refuses private or missing ones with 403 AccessDenied (deny does not apply; fail, hang and Close do), and reports `Public(key)`.

**Deviations**: This run started before the dependency `20261104082020-109dceb1-tcgdex-spaces-cache` was implemented: it was in progress (9/10 tasks complete). The signer already built a canonical query string, so it needed no change and no new signer test. A listing with an awkward prefix (spaces, `+`, `=`) is tested against the fake's real verifier instead.

**Files changed**:
- `internal/spaces/client.go`
- `internal/spaces/config.go`
- `internal/spaces/client_test.go`
- `internal/spaces/config_test.go`
- `internal/spaces/spacestest/spacestest.go`
- `internal/spaces/spacestest/spacestest_test.go`

**Discoveries**: `url.Values.Encode` writes a space as `+`. The signer's `canonicalQuery` parses the raw query and re-encodes it with `%20`, and the verifier does the same with the raw query it receives, so client and fake agree. `PublicBase` treats an endpoint with a port, or with more than one label before `.digitaloceanspaces.com`, as not DigitalOcean's.

### 2026-11-04 — Task: Copy card images to the shared store in the background

**What was done**: The tiered store's inline outage rule moved into an unexported `breaker` type (`internal/cards/breaker.go`). `tieredStore` embeds it, so its log text, its timing and `tiered_test.go` are unchanged. The new `cards.ImageCache` (`internal/cards/images.go`) does two things. `Image(src)` answers from memory alone: the CDN address with the source as fallback for a known copy, otherwise the source address, queuing the image without blocking (a queue of 256). `Run` drives two workers that fetch `low.webp` and `high.webp` from TCGdex, bounded at 10 s each, and put them as public `image/webp` objects under `images/<path>/` with `Cache-Control: public, max-age=604800`. Workers follow an image-labelled breaker, and an image TCGdex fails on waits `RetryAfter`.

**Deviations**:
- This run started before the dependency `20261104082020-109dceb1-tcgdex-spaces-cache` was implemented: it was in progress (9/10 tasks complete).
- The goal check took three attempts. Its reviewer read criteria 2 and 6 as holding across restarts, and criterion 4 as holding with both workers busy. To meet them:
  - The start-up listing (`loadKnown`, planned for the next task) was brought into this task.
  - Every shared-store request (listing pages and puts) is serialised across workers by a mutex, with the breaker checked under it, so an outage costs one attempt per window.
  - The cache never writes over an object already in the bucket, which departs from the plan's "a failed listing means images are copied again over their objects". The start-up listing records half copies (`partial`). When that listing did not complete, each image's own prefix is listed before it is copied (`present`). Only the files the bucket lacks are fetched and put, and each file that is put is remembered, so a copy that fails part way never rewrites it.
- The next task is left with the hourly count log and the fuller listing tests.
- The image path helper is `imageRel`, because `relPath` already exists in the package.

**Files changed**:
- `internal/cards/breaker.go`
- `internal/cards/tiered.go`
- `internal/cards/images.go`
- `internal/cards/images_test.go`

**Discoveries**: An embedded struct keeps tests that assign `store.now` and `store.logf` compiling, but the composite literal in the constructor has to name the embedded type. With more than one worker, a breaker alone does not bound attempts to one per window: both workers can pass `skip()` before either fails. Serialising the shared requests behind one mutex, with `skip()` checked under it, does bound them.

### 2026-11-04 — Task: Find existing copies at start-up and count image answers

**What was done**: The start-up listing itself landed in the previous task, at the goal check's request. This task added the hourly count. `Run` now ticks every `imageReportEvery` (an hour, a package variable so tests can shorten it) and calls `report()`. It logs `card images: last hour N from the shared store, M from the card source (P% shared), K copied` and resets the counters, and logs nothing in an hour with no activity. New tests cover the listing across pages (5 whole copies over 6 pages), an image asked for during a held listing and not uploaded again, a failed listing that still answers, logs once and copies after recovery, the report's counts and idle silence, and Run's ticker.

**Deviations**: This run started before the dependency `20261104082020-109dceb1-tcgdex-spaces-cache` was implemented: it was in progress (9/10 tasks complete). The listing (`loadKnown`) was implemented in the previous task (see its entry), so this task's code change is only the report. A failed listing no longer leads to rewriting copies: each image's own prefix is listed before it is copied.

**Files changed**:
- `internal/cards/images.go`
- `internal/cards/images_test.go`

**Discoveries**: The counts are card answers, one per card per page load (the board fetches card info once per page), not browser image loads. A CDN failure that falls back to TCGdex in the browser still counts as a CDN answer.

### 2026-11-04 — Task: Switch on image copies in hosted mode

**What was done**: `hostedCardDB` now also returns the image cache. It builds one from the same Spaces client, only in the branch that builds the tiered store (four settings, online, client built), and logs `card images: copied from the card source to the shared store, served from <PublicBase>/images/`. `runHosted` starts `images.Run` on the server context. A new `hostedBoard` helper, used by `newHosted`'s `NewBoard`, sets `ui.Images` only when the cache is not nil. `config` gained the test-only `imageBase`.

**Deviations**: This run started before the dependency `20261104082020-109dceb1-tcgdex-spaces-cache` was implemented: it was in progress (9/10 tasks complete). Board construction was factored into `hostedBoard(m, human, public, limit, images)` so tests can check a hosted board's card answers without signing in. The tests build that board directly over `hostedCardDB`'s outputs; they do not go through the registry.

**Files changed**:
- `cmd/tcgreplay/main.go`
- `cmd/tcgreplay/hosted.go`
- `cmd/tcgreplay/hosted_test.go`
- `cmd/tcgreplay/hosted_cache_test.go`

**Discoveries**: Assigning a nil `*cards.ImageCache` to the `webui.ImageSource` interface gives a non-nil interface, and the card endpoint would then call through a nil pointer. `hostedBoard` guards against it.

### 2026-11-04 — Task: Document CDN card images for operators

**What was done**: The `.do/app.yaml` header says that, with the `TCGREPLAY_SPACES_*` settings, card images are copied to `images/` (public) and served from the bucket's CDN with a TCGdex fallback, and that without them images come from TCGdex as before. A comment beside the four entries says they also switch on CDN images. The `Dockerfile` settings comment says the same. The design document's hosted-mode paragraph describes the copies (path, public-read, a 7-day cache lifetime, TCGdex as the only source, kept until cleared, never written over), the CDN address and `image_fallback`, the board's fallback order, the start-up listing and the hourly count line. A new operator setup step 7 covers enabling the bucket CDN, the access key's list and public-write rights, the expected log lines, and how to remove a copy. Step 6 no longer says the CDN is unneeded.

**Deviations**: This run started before the dependency `20261104082020-109dceb1-tcgdex-spaces-cache` was implemented: it was in progress (9/10 tasks complete). The CDN setup went into a new operator step 7 rather than being folded into step 6, so the card-cache step reads as before.

**Files changed**:
- `.do/app.yaml`
- `Dockerfile`
- `TCG Review Playmat Design and Build Plan.md`

**Discoveries**: None.

### 2026-11-04 — Task: Update the card image knowledge entries

**What was done**: Two repo-tier entries in store `tcgreplay` were updated through `spektacular knowledge write`.
- `gotchas/tcgdex-cache-never-expires.md`: the old "card images are not cached by the playmat" paragraph was replaced with a **Card images** paragraph. It covers which boards load from TCGdex, the hosted copies under `images/<TCGdex path>/{low,high}.webp` (public, 7-day cache lifetime, never deleted or written over), the CDN `image` and the TCGdex `image_fallback`, that only `assets.tcgdex.net` art is copied, the start-up listing, and the need to restart (until admin clearing exists) after deleting image objects by hand. Tag `cdn` added.
- `gotchas/board-card-info-per-page-load.md`: a consequence was added. The image address is fixed for the page, so a new copy is used from the next load, and each image spot tries `image`, then `image_fallback`, then the stand-in. Tags `cdn` and `spaces` added.

**Deviations**: This run started before the dependency `20261104082020-109dceb1-tcgdex-spaces-cache` was implemented: it was in progress (9/10 tasks complete).

**Files changed**:
- `.spektacular/knowledge/gotchas/tcgdex-cache-never-expires.md` (via the knowledge CLI)
- `.spektacular/knowledge/gotchas/board-card-info-per-page-load.md` (via the knowledge CLI)

**Discoveries**: The goal reviewer cannot read the knowledge store directly. Staging the entries' CLI read-back under the goal-check folder gives it something to judge.

### 2026-11-04 — Task: Enable the CDN on the Spaces bucket

**What was done**: Nothing. This is a human task in the DigitalOcean control panel: enable the bucket's CDN, confirm the access key can list the bucket and write public-read objects, and redeploy. The task went through the goal check, which judged all three criteria not met after three attempts, because none can be shown from the repository. It was then skipped with the reason "human task left for the operator (needs the DigitalOcean control panel); orchestrator reports it to the user".

**Deviations**: Skipped. The heading and all three criteria are left unticked and are **outstanding for the operator**: the CDN enabled with its address matching the start-up log's public image address; no shared-store warning or failure for card images after a redeploy; a copied image loading from the CDN address in a browser with no sign-in. It must follow the shared cache plan's human task (create the bucket and set its secrets), which is also still open. This run started before the dependency `20261104082020-109dceb1-tcgdex-spaces-cache` was implemented: it was in progress (9/10 tasks complete).

**Files changed**: None.

**Discoveries**: Until the operator enables the CDN, hosted boards with the shared store get CDN addresses that fail and fall back to TCGdex, so players see no difference. Copies are still made.

### 2026-11-06 — Task: Enable the CDN on the Spaces bucket

**What was done**: The operator enabled the CDN on the `tcgreplay` bucket and redeployed. The 10:14:00 start-up log reads `card images: copied from the card source to the shared store, served from https://tcgreplay.ams3.cdn.digitaloceanspaces.com/images/`, which is the bucket's CDN address, then `card images: 0 copies found in the shared store` with no shared-store warning or failure. After a board load the board served `https://tcgreplay.ams3.cdn.digitaloceanspaces.com/images/en/sv/sv03.5/016/high.webp?v=0e5d62d29792`, and signed-out requests for the copies of `sv03.5/016`, `sv02/172` and `sv01/191` (`low.webp` and `high.webp`) returned 200 `image/webp` with `Cache-Control: public, max-age=604800`. All three criteria are met and ticked.

**Deviations**: None. A signed-out request for an image not yet copied gets 403 (the private bucket hides missing objects), not 404.

**Files changed**: None.

**Discoveries**: None.
