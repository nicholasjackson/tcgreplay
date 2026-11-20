---
created_date: "2026-11-04"
document_status: final
closed_date: "2026-11-04"
---

# Context: 20261104090640-25f673ed-card-image-cdn

## Current State Analysis

- **Board images today.** The board loads every card image straight from TCGdex. `/api/card` (`internal/webui/server.go:136-160`) returns `CardInfo.Image`, the TCGdex base URL (`internal/webui/server.go:132-133`), and the board appends `/low.webp` on the mat (`web/src/components/Card.svelte:76-77`) or `/high.webp` in the zoom (`web/src/components/Zoom.svelte:17-18`) and the dialog preview (`web/src/components/Modal.svelte:238-239`). On a load error the card and zoom switch to the text stand-in or Energy token (`Card.svelte:74`). The preview has no error handler. The playmat itself never fetches or caches images (knowledge `gotchas/tcgdex-cache-never-expires.md`).
- **Image addresses.** `cards.Card.Image` (`internal/cards/cards.go:77`) comes from TCGdex, or for an imageless Basic Energy from Crown Zenith art under `https://assets.tcgdex.net/en/swsh/swsh12.5/` (`internal/cards/cards.go:297-308`). After the freshness plan, an imageless card is refreshed after 24 hours, so an address can appear later.
- **Card info per page.** `cardInfo` (`web/src/lib/store.svelte.js:137-152`) fetches each def once per page load, `no-cache` (knowledge `gotchas/board-card-info-per-page-load.md`).
- **Hosted wiring.** `newHosted` (`cmd/tcgreplay/hosted.go:172-205`) builds one card database and a `webui.Server` per account, setting the hosted-only fields `Public` and `Limit` (`internal/webui/server.go:29-43`). After the shared card cache plan, a `hostedCardDB` helper builds the tiered store from the four `TCGREPLAY_SPACES_*` settings, from one `*spaces.Client`, only online.
- **Spaces client and fake (after the shared card cache plan).** `internal/spaces` offers `Get` and `Put` with `PutOptions{ContentType, CacheControl, Public}`, typed errors, and SigV4 signing. `internal/spaces/spacestest` verifies signatures, counts requests, records `X-Amz-Acl`, and has Fail, Deny, Hang and Close knobs. Neither lists nor serves unsigned reads yet. The tiered store's breaker (`internal/cards/tiered.go`) is written inline.
- **Board tests.** Playwright drives `boardtest` (`web/playwright.config.js`) over `cardstest.FakeDB`, whose cards have no images. Board endpoint tests live in `internal/webui/webui_test.go` (`/api/card` at `:295-302`).

## Per-Task Technical Notes

### Task: Carry an image fallback on the card endpoint

**File changes**:
- `internal/webui/server.go:29-43` — add `Images ImageSource` to `Server` beside `Public` and `Limit`, with a comment: hosted with the shared store only; nil (desktop, daemon, dev, boardtest) keeps the TCGdex address. Add `type ImageSource interface{ Image(src string) (image, fallback string) }` with a doc comment saying implementations must never block or touch the network (the endpoint answers on the board's critical path).
- `internal/webui/server.go:117-134` — `CardInfo` gains `ImageFallback string \`json:"image_fallback,omitempty"\`` after `Image`. Update the `Image` comment: the base address to load from (the CDN copy in hosted mode with the shared store, else TCGdex); append `/low.webp` or `/high.webp`. `ImageFallback`: the TCGdex base when `Image` differs from it.
- `internal/webui/server.go:136-160` — in `serveCard`, compute `img, fb := c.Image, ""`; when `s.Images != nil && c.Image != ""`, `img, fb = s.Images.Image(c.Image)`, and drop `fb` when it equals `img`. Put both in the `CardInfo` literal. Headers and error paths unchanged.
- `internal/webui/webui_test.go` (near the existing `/api/card` check at `:295-302`) — add a test `TestCardImageSource` using a `cardstest.FakeDB` with one card that has `Image: "https://assets.tcgdex.net/en/sv/sv1/1"` and one without. Cases: `Images == nil` → decoded body has that `image` and the raw JSON has no `image_fallback` key; a stub `ImageSource` returning `("https://cdn.example/images/en/sv/sv1/1", src)` → both fields set; the stub records its calls and is not called for the imageless card; a stub returning `(src, "")` → no fallback key. Decode into a map to check key absence.

**Complexity**: Low
**Token estimate**: ~12k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Fall back to the card source when a board image fails

**File changes**:
- `web/src/lib/store.svelte.js:137-152` (next to `cardInfo`) — add and export a small helper, for example `imageSrc(info, size, stage)` returning `${info.image}/${size}` for stage 0, `${info.image_fallback}/${size}` for stage 1 when a fallback exists, else `null`; and `nextImageStage(info, stage)` returning 1 when on stage 0 with a fallback, else 2 (broken). Document that `stage` resets per card. `cardInfo` and its `requested` set are unchanged (knowledge `gotchas/board-card-info-per-page-load.md`).
- `web/src/components/Card.svelte:74-77` — replace `broken` with `let stage = $state(0)` (reset when `card.def` changes, as `broken` is today if it is reset; otherwise add an `$effect` like `Zoom.svelte:7-10`). `<img src={imageSrc(info,'low.webp',stage)} … onerror={() => (stage = nextImageStage(info, stage))}>`. The Energy-token branch becomes `info?.category === 'Energy' && (!info.image || stage === 2)` and the image branch `info?.image && stage < 2`. The text stand-in is unchanged.
- `web/src/components/Zoom.svelte:6-18` — same with `high.webp`; the existing reset effect resets `stage` instead of `broken`.
- `web/src/components/Modal.svelte:110,238-239` — the preview `<img>` gains the same stage state (reset when `preview` changes) and an `onerror`, showing nothing once both fail (today it has no `onerror`).
- `web/e2e/images.spec.js` (new) — uses `./fixtures.js` (`test`, reset per test). It intercepts `**/api/card?def=*` with `page.route`, calls `route.fetch()`, and fulfils the JSON with `image: 'https://cdn.test.invalid/images/en/sv/sv1/1'` and `image_fallback: 'https://assets.tcgdex.net/en/sv/sv1/1'` added. It routes `https://cdn.test.invalid/**` and `https://assets.tcgdex.net/**` to either fulfil a tiny inline PNG (base64, `contentType: 'image/webp'` is fine; Chromium sniffs) or `route.abort()`, recording requested URLs. Cases: (1) CDN fulfils → a hand card's `img` `src` starts with the CDN host and no `assets.tcgdex.net` request was made; (2) CDN aborts → the card's `img` `src` becomes the `assets.tcgdex.net` address and the image is visible (`naturalWidth > 0`); the zoom (opened as `web/e2e/touch.spec.js:131-137` does: Deck pile → "Deck list…" → tap a card; click on desktop) shows the fallback `high.webp`; in the desktop profile only (the preview is mouse-only, `Modal.svelte:104-110`), opening the Discard pile modal and hovering `Fighting Energy (p1-16)` shows `img.preview` with the fallback `high.webp`; (3) both abort → a hand Pokémon shows `.text-card` and an Energy shows `.token`.
- `internal/webui/dist` — rebuilt by `make web` as part of the normal build; no hand edits.

**Complexity**: Medium
**Token estimate**: ~30k tokens
**Agent strategy**: Single agent, sequential: helper and components first, build the board, then the Playwright spec in all three profiles.

### Task: List the bucket and work out its public address

**File changes** (the files are created by the shared card cache plan's tasks "Sign requests and read settings for Spaces" and "Add the Spaces client and its stand-in"; adapt names to what landed):
- `internal/spaces/client.go` — add `type ListPage struct{ Keys []string; Next string }` and `func (c *Client) List(ctx context.Context, prefix, token string) (ListPage, error)`. It sends a signed `GET <endpoint>/<bucket>?list-type=2&prefix=<p>&continuation-token=<t>` (omit empty parameters; canonical query string sorted and URI-encoded as SigV4 requires, using the existing signer; empty-payload hash). A 200 is decoded with `encoding/xml` from `<ListBucketResult><Contents><Key/></Contents>…<IsTruncated/><NextContinuationToken/></ListBucketResult>`; `Next` is set only when `IsTruncated` is true. Anything else is `*Error` exactly like `Get`. Transport errors are wrapped `spaces: list <prefix>: …`. If the signer does not yet handle query strings, extend it (canonical query) and add a signer test with a query.
- `internal/spaces/config.go` — add `func (c Config) PublicBase() string`. Parse `Endpoint`; when the host is `<region>.digitaloceanspaces.com` return `https://<bucket>.<region>.cdn.digitaloceanspaces.com`; otherwise return `strings.TrimRight(Endpoint, "/") + "/" + Bucket`. No trailing slash.
- `internal/spaces/config_test.go` — table test for `PublicBase`: `https://lon1.digitaloceanspaces.com` + `b` → `https://b.lon1.cdn.digitaloceanspaces.com`; `https://ams3.digitaloceanspaces.com/` (trailing slash) → same shape; `http://127.0.0.1:9000` + `b` → `http://127.0.0.1:9000/b`.
- `internal/spaces/spacestest/spacestest.go` — (1) bucket-level `GET /<bucket>?list-type=2`: after the existing count/hang/fail/deny/signature checks, return sorted keys with the prefix, honouring `continuation-token` (the last key of the previous page, opaque to clients) and a page size (`max-keys` param, or the server's `PageSize(n)` knob, default 1000), as S3 XML with `IsTruncated` and `NextContinuationToken`. Count it as `Requests("LIST", prefix)`. (2) An unsigned request (no `Authorization` header) for `GET /<bucket>/<key>` is served when the object was stored with `X-Amz-Acl: public-read`; otherwise 403 `AccessDenied`. It is still counted and still subject to `fail`, `hang` and `Close`, but not to `deny` (credentials do not apply). (3) Add `Public(key string) bool`.
- `internal/spaces/client_test.go` — `List` cases against the fake: keys under `images/` only (seed some under `tcgdex/`), in order, with `PageSize(2)` across 3 pages via `Next`; an empty prefix result; `SetDeny(true)` → `*Error` with status 403 and code `InvalidAccessKeyId`; `SetFail(true)` → status 500.
- `internal/spaces/spacestest/spacestest_test.go` (new, or extend the existing fake test) — an object put with `Public: true` is readable with a plain `http.Get(cfg.Endpoint + "/" + bucket + "/" + key)`; an object put without it, and a missing key, give 403.

**Complexity**: Medium
**Token estimate**: ~25k tokens
**Agent strategy**: Single agent, sequential: signer query support if needed, then `List`, then the fake, then tests.

### Task: Copy card images to the shared store in the background

**File changes**:
- `internal/cards/breaker.go` (new; factored from the shared card cache plan's `internal/cards/tiered.go`, which has `down`, `until`, `skip()`, `failed(...)`, `ok()` inline) — `type breaker struct{ retry time.Duration; now func() time.Time; logf func(string, ...any); down bool; until time.Time; mu sync.Mutex; downMsg, upMsg string }` with `skip() bool`, `failed(err error)` (logs `downMsg` with the error and retry once per outage, re-arms the window on a failed probe without logging), `ok()` (logs `upMsg` once when coming back). Change `tieredStore` to hold a `breaker` with its existing messages; its log text and timing must stay identical, and `internal/cards/tiered_test.go` must pass unchanged. If `tiered.go` already has a reusable type, use it instead.
- `internal/cards/images.go` (new) — `ImageCache` as in plan.md Data Structures. `NewImageCache(c, public)` sets `Source: "https://assets.tcgdex.net"`, `HTTP: &http.Client{Timeout: 30 * time.Second}`, `Now: time.Now`, a `queue chan string` of 256, and a breaker with `SharedRetryAfter`, messages `card images: shared store unavailable (%v); using the card source, retrying in %s` / `card images: shared store reachable again`. Unexported: `mu sync.Mutex`, `known map[string]bool` (keyed by the image's relative path, e.g. `en/swsh/swsh12.5/157`, so a later "forget prefix" is a loop over keys), `pending map[string]bool`, `failed map[string]time.Time`, `ready chan struct{}` (closed when start-up listing ends; the next task adds the listing — until then `Run` closes it at once), counters `cdn, source, copied int`, `wg sync.WaitGroup` for tests.
  - `relPath(src) (string, bool)`: requires `strings.HasPrefix(src, Source+"/")`; the rest must have no `?` or `#`, and every `/`-separated segment must be non-empty and not `.` or `..`.
  - `Image(src)`: `""` → `("", "")`. Not copyable → `(src, "")`. Under `mu`: known → `cdn++`, return `(Public + "/" + ImagePrefix + rel, src)`. Else `source++`; if not pending and not failed within `RetryAfter` of `Now()`, mark pending and do a non-blocking send on `queue` (on a full queue, unmark pending). Return `(src, "")`.
  - `Run(ctx)`: closes `ready` (listing added later), starts 2 workers and returns when `ctx` is done. Each worker waits for `ready`, then loops on `queue`: skip if known; skip (unmark pending) if `breaker.skip()`; else `copyOne`.
  - `copyOne(ctx, rel)`: for each of `ImageFiles`, GET `Source + "/" + rel + "/" + file` with `context.WithTimeout(ctx, 10*time.Second)`; non-200 or empty body → `failed[rel] = Now()`, unmark pending, return. Then for each file `Spaces.Put(ctx3s, ImagePrefix+rel+"/"+file, body, spaces.PutOptions{ContentType: "image/webp", CacheControl: ImageCacheControl, Public: true})` with `context.WithTimeout(ctx, SharedTimeout)`; any error → `breaker.failed(err)`, unmark pending, return; success → `breaker.ok()`. After both puts: mark known, `copied++`, unmark pending.
  - Unexported `settle()` for tests: waits until the queue is empty and no copy is in flight.
- `internal/cards/images_test.go` (new, package `cards`) — `spacestest.New(t)`, an `httptest` image host serving `/en/sv/sv1/1/low.webp` and `/high.webp` (distinct byte bodies), per-path hit counts, and switches for 404/500/slow. `ic := NewImageCache(sc, cfg.PublicBase()); ic.Source = imageHost.URL`; fake clock in `ic.Now`; breaker `now` and `logf` overridden. Cases: first `Image` → `(src, "")`; after `settle`, the fake holds both keys, bytes equal, `Public(key)`, `Content-Type` `image/webp`, `Cache-Control` equal to `ImageCacheControl`; next `Image` → `(PublicBase+"/images/en/sv/sv1/1", src)`, and a plain `http.Get` of that + `/low.webp` returns the bytes; a third and fourth `Image` call add zero image-host hits; another host, `..`, `?x`, empty segment → unchanged, never queued (`Total()` unchanged); `""` → `""`; a second address (late art) gets its own copy; fake Spaces `Close`/`Hang`/`SetFail`/`SetDeny` → `Image` returns within 50 ms and gives the TCGdex address, at most one PUT attempt per breaker window (advance the clock by `SharedRetryAfter` to see the probe), exactly one "unavailable" and one "reachable again" line; image host 404 → one hit, a repeat `Image` within 5 minutes queues nothing, after `RetryAfter` it is copied; a full queue (block workers by not running `Run`, push 300 addresses) never blocks `Image`. Run with `-race`.

**Complexity**: High
**Token estimate**: ~45k tokens
**Agent strategy**: Parallel analysis, sequential integration: one agent factors the breaker and keeps `tiered_test.go` green, then the image cache and its tests are written on top in sequence.

### Task: Find existing copies at start-up and count image answers

**File changes**:
- `internal/cards/images.go` — `Run(ctx)` first runs `loadKnown(ctx)` in a goroutine and closes `ready` when it returns. `loadKnown`: loop `Spaces.List(ctxPage, ImagePrefix, token)` with `context.WithTimeout(ctx, SharedTimeout)` per page; skip entirely when `breaker.skip()`; on error `breaker.failed(err)` and stop (keep what was gathered); on success `breaker.ok()`. Collect `rel → set of files` for keys `images/<rel>/<file>` with `file` in `ImageFiles`; at the end, under `mu`, mark known every `rel` with all `ImageFiles` present. Log `card images: %d copies found in the shared store` once at the end (or the breaker's failure line). Workers re-check `known` before copying (already in place).
- `internal/cards/images.go` — add the hourly count log: `Run` starts a `time.Ticker` of `imageReportEvery = time.Hour` (unexported var so tests can shorten it, or call an unexported `report()` directly). `report()` swaps the counters to zero under `mu` and, when any is non-zero, logs `card images: last hour %d from the shared store, %d from the card source (%.0f%% shared), %d copied`.
- `internal/cards/images_test.go` — cases: seed the fake with both files for 5 images and one file for a sixth, `PageSize(2)`; after `Run` and waiting on `ready`, the 5 answer with CDN addresses with zero image-host hits and `Requests("LIST", "images/") == 3`; the sixth answers with TCGdex and is then copied; an `Image` call made before `ready` (hold the listing with `Hang`, then release) for an image the listing finds produces no PUT; with `SetFail(true)` during the listing, `ready` still closes, `Image` still answers, and one "unavailable" line is logged; `report()` after 3 CDN and 1 source answers logs one line with `3`, `1` and `75%`, and a second `report()` with no activity logs nothing.

**Complexity**: Medium
**Token estimate**: ~25k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Switch on image copies in hosted mode

**File changes** (the helper is created by the shared card cache plan's task "Use the shared store in hosted mode"; adapt names to what landed):
- `cmd/tcgreplay/main.go:40-62` — `config` gains `imageBase string // if set, the card image source prefix (tests only)`, beside the test-only `cardBase` the shared cache plan adds.
- `cmd/tcgreplay/hosted.go` (`hostedCardDB`) — return a third value `images *cards.ImageCache`, built only in the branch that builds the tiered store (settings complete, online, `spaces.New` succeeded), from the same `*spaces.Client`: `images = cards.NewImageCache(sc, cfg.PublicBase())`, `images.Source = c.imageBase` when non-empty. Log `card images: copied from the card source to the shared store, served from <PublicBase>/images/`. In every other branch `images` is nil and nothing new is logged.
- `cmd/tcgreplay/hosted.go:172-188` (`newHosted`) — take `images *cards.ImageCache` as a parameter. In `NewBoard`, set `ui.Images = images` only when `images != nil` (assigning a nil `*ImageCache` to the interface would make it non-nil).
- `cmd/tcgreplay/hosted.go:207-229` (`runHosted`) — `db, shared, images := hostedCardDB(c)`; pass `images` to `newHosted`; after `go reg.Run(ctx)`, `if images != nil { go images.Run(ctx) }`.
- `cmd/tcgreplay/hosted_test.go` (the call at `:662` that the shared cache plan changed to pass `newCardDB(c)`) — also pass `nil` images.
- `cmd/tcgreplay/hosted_cache_test.go` (from the shared cache plan) — add cases with an `httptest` image host as `imageBase` and a fake TCGdex card whose `image` is under it: all four settings from `spacestest.Server.Config()` and `offline: false` → `images != nil`; build a board through `newHosted`'s `NewBoard` (or call `images.Image` directly plus a `webui.Server` with `Images` set) and GET `/api/card` → TCGdex address; after settling, the fake Spaces holds `images/<rel>/low.webp` and `high.webp`; a second GET → CDN address with `image_fallback`; the log names the public base. No settings, a partial set, and `offline: true` → `images == nil`, `/api/card` has no `image_fallback`, the image host and fake Spaces record zero image requests, and the log has no new "warning". A `runHosted` lifecycle case with the fake hanging on LIST checks `/healthz` answers 200 before the hang is released.

**Complexity**: Medium
**Token estimate**: ~30k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Document CDN card images for operators

**File changes** (on top of the shared card cache plan's edits to the same files):
- `.do/app.yaml:7-11` — header comment: with the `TCGREPLAY_SPACES_*` settings set, card images are copied to the bucket under `images/` (public) and the board loads them from the bucket's CDN (enable it on the bucket), falling back to TCGdex; without them, images come from TCGdex as before. Beside the four `TCGREPLAY_SPACES_*` entries, add one comment line that the same settings switch on CDN images. No new env entry.
- `Dockerfile:1-6` — the settings comment notes that the `TCGREPLAY_SPACES_*` variables also switch on CDN card images. No build or context change; `.dockerignore` untouched.
- `TCG Review Playmat Design and Build Plan.md:302` — in the hosted mode paragraph (after the shared card cache sentence), add: card images are copied byte for byte from TCGdex (the only image source) to `images/<TCGdex path>/{low,high}.webp`, public-read with a 7-day cache lifetime, kept until an admin clears them; the board is given the CDN address (`https://<bucket>.<region>.cdn.digitaloceanspaces.com`) for copied images and TCGdex's otherwise, and falls back to TCGdex if the CDN fails; the start-up listing; the hourly `card images: last hour …` log line as the measure of CDN use.
- `TCG Review Playmat Design and Build Plan.md:322-327` — the operator setup step for the Spaces bucket (added by the shared cache plan) gains: enable the bucket's CDN (Spaces → bucket → Settings → CDN, edge TTL up to 1 week); the access key must be able to list the bucket and write objects with a public-read ACL; `tcgdex/` objects stay private.

**Complexity**: Low
**Token estimate**: ~10k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Update the card image knowledge entries

**File changes** (knowledge entries are written only through the `spek-knowledge` skill and `spektacular knowledge write`, never with file tools):
- `gotchas/tcgdex-cache-never-expires.md` (repo tier, store `tcgreplay`; as rewritten and possibly retitled by the freshness and shared cache plans) — replace the closing "Card images are not cached by the playmat at all…" paragraph: desktop and dev boards still load images from `assets.tcgdex.net`; hosted with the shared store copies each image the board shows to `images/<TCGdex path>/{low,high}.webp` (public, kept until cleared), and `/api/card` gives the CDN address once a copy is known, with `image_fallback` to TCGdex; the playmat learns existing copies by listing at start-up, so deleting image objects by hand needs a restart before the playmat stops pointing at them (or the admin clearing spec once it exists).
- `gotchas/board-card-info-per-page-load.md` (repo tier, store `tcgreplay`) — add a consequence: the image address in a card's info is fixed for the page, so an image copied while the page is open loads from the CDN only after a reload; the board tries `image`, then `image_fallback`, then the text stand-in.
- Add the `cdn` and `spaces` tags to both if the tag list allows (check `spektacular knowledge tags`).

**Complexity**: Low
**Token estimate**: ~8k tokens
**Agent strategy**: Single agent, sequential execution, through the `spek-knowledge` skill.

### Task: Enable the CDN on the Spaces bucket

**What the person does** (in the DigitalOcean control panel; no repo files change). This follows the shared card cache plan's human task "Create the Spaces bucket and set its secrets", which must be done first:
1. Open the Spaces bucket the hosted playmat uses, go to Settings → CDN, and enable it with an edge cache TTL (1 week is the longest and suits images that never change). Keep file listing restricted.
2. Check that the bucket's access key (from the shared cache task) is allowed to list the bucket and to write objects with a public-read ACL. For a DigitalOcean "Limited access" key, read/write on the bucket covers both; widen it if not.
3. Redeploy the app (or restart it) so it starts with the image cache.

**How the result is checked**: The start-up log shows `card images: … served from https://<bucket>.<region>.cdn.digitaloceanspaces.com/images/` and `card images: N copies found in the shared store`, with no `card images: shared store unavailable` line. After opening a board with a new game, the bucket gains `images/…/low.webp` and `high.webp` objects marked public. One of them loads at its CDN address in a private browser window, while a `tcgdex/` object at the same host is refused. The behavioural checks on the real CDN are the manual items in the implementation test plan.

**Complexity**: Low
**Token estimate**: n/a (human task)

## Testing Strategy

Go tests throughout, run with `go test ./...` and under `-race`, plus the Playwright suite (`make web`, then `npm run test:e2e` in `web/`). No test reaches real Spaces, its CDN, or TCGdex. Per task:

- **Carry an image fallback on the card endpoint** — `internal/webui/webui_test.go`: no image source gives a byte-identical answer (no `image_fallback` key); a stub source sets both fields; an imageless card never asks the stub.
- **Fall back to the card source when a board image fails** — `web/e2e/images.spec.js` in phone, tablet and desktop: CDN loads with no TCGdex request; CDN fails, so card, zoom and (desktop) preview show TCGdex; both fail, so stand-in or token. Every existing spec passes unchanged.
- **List the bucket and work out its public address** — `internal/spaces/client_test.go` (List paging, prefix isolation, 403/500 typed errors), `internal/spaces/config_test.go` (`PublicBase` table), the fake's test (unsigned public read allowed, private or missing refused).
- **Copy card images to the shared store in the background** — `internal/cards/images_test.go`: the fill-in cycle with byte-identical public copies and headers, the CDN address afterwards with zero source hits, non-TCGdex and malformed addresses never copied, every outage mode instant with one attempt per window and two log lines, a source failure retried after 5 minutes, a full queue never blocking. `internal/cards/tiered_test.go` passes unchanged after the breaker is factored out.
- **Find existing copies at start-up and count image answers** — `internal/cards/images_test.go`: copies across listing pages known at start, a half copy treated as missing, no re-upload of an image found by the listing, a failed listing logged once, the hourly line's counts, and silence when idle.
- **Switch on image copies in hosted mode** — `cmd/tcgreplay/hosted_cache_test.go`: complete settings online give a board image source and a copy, then the CDN address; no settings, partial settings and offline give no image source, no image requests and no new warning; `/healthz` answers while the listing hangs.
- **Document CDN card images for operators** and **Update the card image knowledge entries** — no tests; the existing build and guides tests guard packaging.
- **Enable the CDN on the Spaces bucket** — human; checked by the manual items in the implementation test plan (real CDN end to end, card data private, fallback with the CDN blocked, no settings, restart keeps copies, iOS Safari, and the week-long 95 % measure).

## Project References

- **Spec:** `20261104090640-25f673ed-card-image-cdn` (epic `20261104082020-109dceb1-tcgdex-spaces-cache`).
- **Upstream plans:** `20261104082020-109dceb1-tcgdex-spaces-cache` (Spaces client, fake, settings, breaker, hosted helper, bucket human task) and `20261104090640-51bd1f36-card-cache-freshness` (Store contract, late artwork via the 24-hour imageless refresh, `RetryAfter`).
- **Downstream spec:** `20261104090640-a5a74fab-admin-cache-clearing` (reuses `List`, clears `images/`, needs forget and CDN purge).
- **Design documents:** none referenced by the spec.
- **Knowledge entries:** `gotchas/board-card-info-per-page-load.md`, `learnings/card-image-sources.md`, `gotchas/tcgdex-cache-never-expires.md`, `gotchas/game-listeners-under-lock.md`, `conventions/skills-text-and-packaging.md` (repo tier, store `tcgreplay`).
- **Repo:** `tcgreplay`, root `/home/nicj/code/github.com/nicholasjackson/tcgreplay`.

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

Estimated total: about 185k tokens across eight agent tasks. The one High task (the image cache copier) factors the breaker first, keeping the tiered store's tests green, then builds the cache on top.

## Migration Notes

No data migration. The `/api/card` change is additive: `image_fallback` is a new optional field, and older boards still open from cache simply ignore it and load `image`. The first hosted start with this change finds no `images/` objects, so every image is copied once as boards show it. No setting changes, so an existing deployment switches on CDN images as soon as it runs this build with the shared store configured. Until the operator enables the bucket's CDN, the CDN addresses fail and the board falls back to TCGdex, so the order of deploy and CDN enabling does not matter to players.

## Performance Considerations

- `/api/card` gains a map lookup under a short mutex and a non-blocking channel send. No I/O is added to any request path or under the game lock.
- Copying runs at most two images at a time, each two files (typically tens to a few hundred KB) held in memory only for the copy, which suits the 0.5 GB instance. A burst larger than the 256-entry queue is dropped and re-queued on later views.
- Spaces failures cost at most one 3-second attempt per minute for images, in the background. Players never wait on them.
- The start-up listing reads 1,000 keys per request, about two keys per image, so 10,000 copied images take about 20 requests, in the background. The known set costs about 100 bytes per image in memory.
- Once an image is copied, TCGdex image traffic for it drops to zero for hosted players, apart from fallbacks. Browsers cache copies for 7 days.
