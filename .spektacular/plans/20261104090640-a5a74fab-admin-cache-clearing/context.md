---
created_date: "2026-11-04"
document_status: final
closed_date: "2026-11-04"
---

# Context: 20261104090640-a5a74fab-admin-cache-clearing

## Current State Analysis

Today, before the three upstream plans land, the playmat has no shared store, no image copies and no admin role. `internal/cards/cards.go:167-287` caches TCGdex responses on disk forever, and in an unexpiring `mem` map. `internal/hostauth` signs users in with Google: `Account{ID, Email}` (`internal/hostauth/hostauth.go:64-69`), with sessions sealed into a cookie (`internal/hostauth/seal.go:94-105`) and the board behind `RequireSession` (`internal/hostauth/board.go:56-95`). Google sends `email_verified`, as does the fake at `internal/hostauth/hostauthtest/google.go:152`, but it is not read (`internal/hostauth/google.go:132-139`). Hosted routing is `hostedHandler` (`cmd/tcgreplay/hosted.go:151-161`): `/` goes to the per-account board, and there is no `/admin` route. `/api/card` returns `CardInfo` with the TCGdex `image` (`internal/webui/server.go:117-160`).

This plan starts from the state the three upstream plans leave, in order: `20261104090640-51bd1f36-card-cache-freshness`, then `20261104082020-109dceb1-tcgdex-spaces-cache`, then `20261104090640-25f673ed-card-image-cdn`. By then:

- `internal/cards` has `Store`/`Entry`/`ErrNotCached`, a disk store, an immutable catalog, a refresh tracker, single-flight, `mem` entries with fetch times, a Spaces store under `tcgdex/`, a tiered store with `Local()` and a breaker, and an `ImageCache` with `known` keyed by relative path.
- `internal/spaces` has `Get`, `Put`, `List` (`ListPage{Keys, Next}`) and `PublicBase`, with `spacestest` as its fake.
- `webui.ImageSource`, `image_fallback` and the board's `imageSrc` helper exist.
- `hostedCardDB` builds the tiered store and image cache from `TCGREPLAY_SPACES_*`.
- The upstream knowledge entry says that, until this spec, forcing fresh hosted data means deleting `tcgdex/` objects by hand.

Requirement → repo and files (all in repo `tcgreplay`):
- Admins can clear the whole cache: `internal/cards/{scope,store,spacesstore,tiered,clear,images}.go`, `internal/spaces/{client,cdn}.go`, `internal/cacheadmin/*`, `cmd/tcgreplay/hosted.go`.
- Admins can clear one set (by code or id): `internal/cards/clear.go` (`SetID`), `internal/cards/scope.go`, `internal/cacheadmin/handler.go`.
- Only admins can clear: `internal/hostauth/{hostauth,google,seal,store,admin}.go`, `cmd/tcgreplay/hosted.go`.
- Clearing does not interrupt play: `internal/cards/cards.go` and `fresh.go` (cleared mark, generation fence), `internal/game/clear_test.go`.
- Cleared images refreshed within 1 hour: `internal/cards/images.go` (versions, `Clear`), `internal/spaces/cdn.go`, `internal/webui/server.go`, `web/src/lib/store.svelte.js`.
- Settings as App Platform environment variables: `.do/app.yaml`, `Dockerfile`, `Makefile`, `TCG Review Playmat Design and Build Plan.md`.

## Per-Task Technical Notes

### Task: Delete objects in bulk and report their checksums

**File changes** (`internal/spaces` and `spacestest` are created by `20261104082020-109dceb1-tcgdex-spaces-cache`, and `List` by `20261104090640-25f673ed-card-image-cdn`; adapt names to what landed):
- `internal/spaces/client.go` (`ListPage` and `List`, from card-image-cdn) — add `ETags map[string]string` to `ListPage`. While decoding `<Contents>`, read `<ETag>`, strip the surrounding quotes and store `ETags[key]`. `Keys` and `Next` are unchanged. Document that for a single-part, non-SSE-C upload the ETag is the hex MD5 of the bytes.
- `internal/spaces/client.go` — add `func (c *Client) Delete(ctx context.Context, keys []string) error`. Return nil for no keys. Split into chunks of 1,000. For each chunk, build `<Delete><Quiet>true</Quiet><Object><Key>k</Key></Object>…</Delete>` with `encoding/xml` (escaping keys), and send a signed `POST <endpoint>/<bucket>?delete=` with headers `Content-MD5: base64(md5(body))` and `Content-Type: application/xml`, using the payload hash in the signature. Sign the canonical query `delete=` (an empty value), using the signer's query support added for `List`. A 200 is decoded from `<DeleteResult><Error><Key/><Code/></Error>…</DeleteResult>`; any `<Error>` entries are collected into `*DeleteError{Failed: key→code}`, carried across chunks and returned at the end. A non-200 is `*Error`, as for `Get`. Transport errors are wrapped `spaces: delete %d keys: …`. Stop at the first non-200 chunk and return its error.
- `internal/spaces/client.go` — `type DeleteError struct{ Failed map[string]string }` with `Error()` = `spaces: %d keys not deleted (first: <key> <code>)`.
- `internal/spaces/spacestest/spacestest.go` — record each object's MD5 (hex) at `Put`/`SetObject` time, and emit `<ETag>"<md5>"</ETag>` in list responses. Handle `POST /<bucket>?delete`: verify the signature (as for every request), require `Content-MD5` to match the body (400 `InvalidDigest` otherwise), parse the XML, remove each key that exists (missing keys succeed silently), and count requests under method `POST`, key `?delete`. Add a knob `DenyDelete(keys ...string)` that answers those keys with `<Error><Code>AccessDenied</Code>`. The existing Fail, Deny, Hang and Close knobs apply. Add `Keys(prefix string) []string` for tests to read what is left.
- `internal/spaces/client_test.go` — delete tests: three keys of five removed; 2,500 keys removed with exactly 3 POSTs; a missing key gives nil; `Deny` gives `*Error` with 403 and the code; `DenyDelete("a")` gives `*DeleteError` naming `a` while the other keys are gone; a mismatched Content-MD5 (via a test hook on the fake that corrupts it) is refused. Listing tests: `ETags[key]` equals `hex(md5(body))`, and a rewritten object lists a new ETag; the existing list tests still pass.

**Complexity**: Medium
**Token estimate**: ~30k
**Agent strategy**: Single agent, sequential: fake first (ETag and delete), then the client, then the tests.

### Task: Purge the bucket CDN through the DigitalOcean API

**File changes:**
- `internal/spaces/cdn.go` (new) — `type CDN struct{ Token, Origin, API string; HTTP *http.Client }`. `NewCDN(cfg Config, token string) *CDN` sets `Origin` to `<bucket>.<region>.digitaloceanspaces.com`, built from `cfg.Bucket` and the endpoint host (the origin host is the bucket name followed by the endpoint host, so a non-DigitalOcean endpoint gives `<bucket>.<host>`), `API: "https://api.digitalocean.com"` and `HTTP: &http.Client{Timeout: 10 * time.Second}`. `var ErrNoCDN = errors.New("spaces: no CDN endpoint for this bucket")`. `Purge(ctx, paths)`: return nil for no paths. Call `endpointID(ctx)`: `GET {API}/v2/cdn/endpoints?per_page=200&page=N` with `Authorization: Bearer <token>`, decoding `{"endpoints":[{"id","origin"}],"links":{"pages":{"next"}}}`. Follow pages until one has `origin == Origin` (compare case-insensitively), otherwise return `ErrNoCDN`. Then `DELETE {API}/v2/cdn/endpoints/{id}/cache` with JSON `{"files": paths}`, where a 204 or 200 succeeds. Any other status is `*CDNError{Status, ID, Message}`, decoded from DigitalOcean's `{"id","message"}` error body. Error strings never include the token. Purge sends at most 50 paths per request, and only for long lists.
- `internal/spaces/spacestest/cdn.go` (new) — `type CDN struct{ *httptest.Server … }`. `NewCDN(t, origin, token string) *CDN` serves `GET /v2/cdn/endpoints` (one endpoint `{id:"cdn-1", origin}`, plus a decoy endpoint with another origin) and `DELETE /v2/cdn/endpoints/cdn-1/cache`, and answers 404 for an unknown id. A wrong or missing bearer token gives 401 `{"id":"unauthorized"}`. Knobs: `SetFail(bool)` (500), `SetOrigin(string)`. `Purges() [][]string` returns the recorded files. `URL` is the base to put in `CDN.API`.
- `internal/spaces/cdn_test.go` (new) — the purge reaches `cdn-1` with exactly the paths; a decoy-only listing (`SetOrigin("other")`) gives `ErrNoCDN` and no purge is recorded; a wrong token gives `*CDNError` with 401 whose `Error()` does not contain the token; `SetFail` gives a 500 error; a closed server gives a transport error; `NewCDN` derives the origin `b.lon1.digitaloceanspaces.com` from `https://lon1.digitaloceanspaces.com` and bucket `b`.

**Complexity**: Low
**Token estimate**: ~20k
**Agent strategy**: Single agent, sequential.

### Task: Clear the card stores

**File changes** (the stores are created by the freshness and shared card cache plans):
- `internal/cards/scope.go` (new) — `type Scope struct{ Set string }`; `func (s Scope) All() bool`. `Key(key string) bool`: true for every key when `All()`. Otherwise true for `key == "sets/"+s.Set`, and for `strings.HasPrefix(key, "cards/"+s.Set+"-")`. `Image(rel string) bool`: split `rel` on `/`; true when `All()`, or when there are at least 3 segments and `segments[2] == s.Set`. Also `TierReport{Tier string; Removed int; Err error}`, and the unexported optional interface `clearer interface{ Clear(ctx context.Context, s Scope) []TierReport }`. Add a doc comment giving the TCGdex id convention (`<set id>-<local id>`) and the image path layout.
- `internal/cards/scope_test.go` (new) — a table test covering: all; `me01` matching `sets/me01` and `cards/me01-001` and not `sets/me01.5`, `cards/me01.5-001` or `sets`; `sv03` against `sv03.5`; `swsh12` against `swsh12tg` and `swsh12.5`; images `en/me/me01/001` matching and `en/swsh/swsh12.5/157` not matching `me01`; malformed paths.
- `internal/cards/store.go` (disk store, from the freshness plan) — add `Clear(ctx, s) []TierReport` on the disk store, reporting `Tier: "disk"`. For `All()`, remove `<dir>/sets.json`, `<dir>/sets/`, `<dir>/cards/` and the retired `<dir>/set-index-v2.json` if present, counting the `.json` files removed, and nothing else under the cache directory. For a set, remove `<dir>/sets/<safeName(set)>.json` and every `<dir>/cards/<safeName(set)>-*.json` (read the directory and match by prefix, not by glob, so ids containing glob metacharacters are safe). A missing file or directory is not an error. Other errors are reported in `Err` after removing what it can.
- `internal/cards/spacesstore.go` (from the shared card cache plan) — add `Clear(ctx, s)` reporting `Tier: "shared"`. For `All()`, list `tcgdex/`; for a set, list `tcgdex/cards/<safeName(set)>-` and add `tcgdex/sets/<safeName(set)>.json`. Collect keys across pages (each `List` bounded at `clearTimeout = 30 * time.Second`), then `Delete` them (bounded by the same constant per 1,000-key batch: use a context of `clearTimeout * (1 + len/1000)`). Count `Removed` as the keys sent minus `*DeleteError` failures. Any error goes in `Err`. The breaker is not consulted: this store has none, and the tiered store calls it directly.
- `internal/cards/tiered.go` (from the shared card cache plan) — add `Clear(ctx, s)`. If `shared` implements `clearer`, call it first, whether or not `skip()` holds (a comment explains that clears are admin actions and must report Spaces failures rather than skip). Then do the same for `local`, and return the reports concatenated. On shared success, call `ok()` so a recovered store clears the breaker. On failure, do not call `failed()`, so an admin's failed clear does not trip lookups. A comment explains the shared-then-local order: a concurrent `Get` that misses local after the local clear must not find an old shared entry and copy it down.
- `internal/cards/clear_store_test.go` (new) — over `t.TempDir()` disk stores, `spacestest`, and the tiered store composed of both: seed entries for `sets`, `sets/me01`, `sets/me01.5`, `cards/me01-001`, `cards/me01-002`, `cards/me01.5-001`, `cards/sv1-001`. A clear of `me01` removes exactly `sets/me01`, `cards/me01-001` and `cards/me01-002` from both tiers (`Get` gives `ErrNotCached`, and the fake's `Keys("tcgdex/")` agrees), with reports of disk 3 and shared 3. A clear of everything leaves no files under the store-owned paths (an unrelated file in the cache directory survives) and no `tcgdex/` objects. With `Deny` on the fake, the shared report has `Err` and the disk is still cleared. With the breaker open (trip it via a failing Get, then restore the fake), a clear still sends the delete. A store without `Clear` (a plain in-memory test store) is not affected.

**Complexity**: Medium
**Token estimate**: ~35k
**Agent strategy**: 2 parallel agents: (a) scope and disk store with their tests; (b) Spaces store and tiered store with their tests once (a)'s `scope.go` exists. Then integrate sequentially.

### Task: Clear the card database and name sets

**File changes** (the freshness plan creates `fresh.go` and `catalog.go` and reshapes `cards.go`; adapt names to what landed):
- `internal/cards/cards.go` (`Client`, `mem` of `cardEntry{card, fetched}` per the freshness plan) — add `cleared bool` to `cardEntry`, and `gen uint64` to `Client` (guarded by `mu`). Comment: a clear increments `gen`, and loads remember the `gen` they started under.
- `internal/cards/cards.go` (`Card`, after the freshness plan's lookup flow) — on a memory hit with `cleared` set and the client online, take the miss path (store, then single-flight fetch), keeping the old `*Card` aside. If the load fails (any error, including the card source being down or the failed-card memory being set), return the old card with a nil error. Offline, return the old card. On success, replace the entry (`cleared` false), subject to the generation rule below.
- `internal/cards/cards.go` (`Cached`) — on a cleared hit, return the card and start the existing background warm or refresh for `cards/<id>` (`maybeRefresh`). Never block.
- `internal/cards/fresh.go` (load and refresh helpers) — capture `gen` under `mu` when a load or refresh starts. When applying its result to `mem`, or swapping a catalog built by it, under `mu`, skip the memory or catalog update if `c.gen` has moved on (the caller still gets its result). Store puts are unaffected.
- `internal/cards/clear.go` (new) — `func (c *Client) Clear(ctx context.Context, s Scope) []TierReport`. (1) If `c.Store` (the full store, not the local accessor) implements `clearer`, append its reports. Otherwise append `{Tier: "disk", Err: errors.New("store cannot be cleared")}`. (2) Under `c.mu`, with no I/O: walk `mem`, setting `cleared = true` on entries where `s.Key("cards/"+id)`, and count them. Set the catalog pointer to nil. Delete tracker entries (background refresh times, miss times, failed cards) whose key matches `s.Key`; for `All()` reset all three maps, and also drop miss keys for set codes (the tracker's miss keys for codes are not card keys; on any clear, drop every miss entry, since misses are cheap). Increment `gen`. Append `{Tier: "memory", Removed: n}`. (3) Wait for nothing: background work started before is fenced by `gen`. Log `card cache: cleared <scope>: <reports>`.
- `internal/cards/clear.go` — `var ErrUnknownSet = fmt.Errorf("no such set: %w", ErrNotFound)`. `func (c *Client) SetID(ctx context.Context, name string) (id, setName string, err error)`: trim the name; empty gives `ErrUnknownSet`. Use `ensureCatalog(ctx)` (building it if needed, network allowed: this is never called under a game lock). If any catalog set id equals `strings.ToLower(name)`, return it with its `Name` from `setDetail`. Otherwise normalise as `Resolve` does (upper case, `codeAliases`, `setAliases`) and look up the catalog's code index. On a miss, use the freshness plan's miss path (refresh the set list and any new sets, throttled once per 5 minutes per distinct miss) and look again. Still missing gives `fmt.Errorf("%q: %w", name, ErrUnknownSet)`. Factor the normalisation out of `Resolve` into a shared helper rather than duplicating it.
- `internal/cards/clear_test.go` (new) — use the freshness plan's fake TCGdex and fake clock, with the tiered store over `t.TempDir()` and `spacestest`. Cases:
  - `SetID("MEG")`, `SetID("me01")` and `SetID("ME01")` all give `me01` with its name. `SetID("XYZ")` gives `ErrUnknownSet`, and the fake TCGdex records at most one set-list refresh for it.
  - After loading `me01-001`, `sv1-001` and resolving codes, `Clear(Scope{})` makes the fake Spaces empty under `tcgdex/` and the cache directory empty of card files. The next `Card("me01-001")` makes exactly one `/cards/me01-001` request and returns the source's updated body (change it in the fake before the clear). `Resolve("MEG","1")` rebuilds from the source.
  - After `Clear(Scope{Set:"me01"})`, `Card("me01-001")` makes one request and `Card("sv1-001")` makes none.
  - `Cached("me01-001")` straight after a clear returns the old card with zero new requests. After settle, it returns the updated card.
  - With the fake TCGdex closed after a clear, `Card` returns the old card with a nil error.
  - Generation fence: hold a set fetch open with the fake's gate, run `Clear`, release the gate, and the catalog is not the pre-clear one: the next `Resolve` triggers a rebuild.
  - A card in the failed-card memory, in scope, is requested again straight after a clear.
  - Reports: memory count, disk count and shared count are present and correct.

**Complexity**: High
**Token estimate**: ~55k
**Agent strategy**: Parallel analysis of the landed freshness code (lookup flow, catalog, tracker), then sequential integration: `SetID` first, then `Clear` and the cleared-entry rule, then the generation fence, then the tests under `-race`.

### Task: Prove an open game survives a clear

**File changes:**
- `internal/game/clear_test.go` (new) — build a fake TCGdex (reuse the helper style of `internal/game/spacescache_test.go` from the shared card cache plan) serving the cards of two small decklists, with images. Build a real `cards.Client` with the tiered store over `t.TempDir()` and `spacestest`. Create a game through `game.NewManager(dir, db)` and the existing new-game or import path (as `manager_test.go` does), so the cards are prefetched. Snapshot `View()` (names, HP, text and image via the card lookups the board uses). Call `db.Clear(ctx, cards.Scope{})`. Then: (1) `View()` returns within 1 s and every card still has its definition (non-nil `Cached` for every def, HP unchanged); (2) a play (for example `draw` or `bench`, via the existing verbs) succeeds; (3) after the client's settle helper (or polling `Cached` for an updated field the fake changed before the clear, with a 5 s deadline), the fake TCGdex shows one fresh request per card used and `Cached` returns the updated copy. A second case closes the fake TCGdex before the clear: `View` and the play still work, and `db.Card` for a game card returns the old copy with no error.

**Complexity**: Low
**Token estimate**: ~20k
**Agent strategy**: Single agent, sequential.

### Task: Version image copies by their content

**File changes** (`internal/cards/images.go` is created by card-image-cdn):
- `internal/cards/images.go` — change `known map[string]bool` to `known map[string]string` (rel → version). Add `func imageVersion(lowETag, highETag string) string`: the hex SHA-256 of `lowETag + "\n" + highETag`, first 12 characters. In `copyOne`, compute `hex(md5(body))` for each file as it is put, and on success set `known[rel] = imageVersion(md5Low, md5High)`. In `loadKnown` (start-up listing), collect `rel → {file → ETag}` from `page.ETags`, and mark known only when both `ImageFiles` are present **with** non-empty ETags, at `imageVersion(etagLow, etagHigh)`. Keep `ImageFiles` order fixed (low, high).
- `internal/cards/images.go` — change `Image(src) (image, fallback string)` to `Image(src) (image, fallback, version string)`. A known image gives `(public+"/"+ImagePrefix+rel, src, known[rel])`; every other path gives `version == ""`. The front still does only map reads and a non-blocking queue push under `mu`.
- `internal/cards/images_test.go` (from card-image-cdn) — update existing calls to three results. New cases: after a copy, the version equals `imageVersion(md5(low), md5(high))`; a re-copy of the same bytes (forget the copy via a test helper, then view again) gives the same version; changed bytes on the image host give a different version; a new `ImageCache` over the same fake (a restart), after its listing, gives the same version as before; a listing where one file has no ETag (a fake hook) treats the image as uncopied; `Image` still returns within 50 ms with the fake Spaces hanging.

**Complexity**: Low
**Token estimate**: ~20k
**Agent strategy**: Single agent, sequential.

### Task: Send image versions to the board

**File changes:**
- `internal/webui/server.go` (`ImageSource` and `CardInfo`, as card-image-cdn leaves them, near `:117-160`) — `ImageSource.Image` returns `(image, fallback, version string)`. `CardInfo` gains `ImageVersion string \`json:"image_version,omitempty"\`` after `ImageFallback`. In `serveCard`, take the version from the image source, and set it only when `img != c.Image` (a CDN answer). Update the comments: the board appends `?v=<image_version>` to the primary address with a size.
- `internal/webui/webui_test.go` (`TestCardImageSource` from card-image-cdn) — update the stub to three results. Add a case where the stub returns a version, so the JSON carries `image_version`. With no source, or a stub returning `(src, "", "")`, the key is absent.
- `web/src/lib/store.svelte.js` (`imageSrc` from card-image-cdn, next to `cardInfo` around `:137-152`) — for stage 0, return `${info.image}/${size}` plus `?v=${encodeURIComponent(info.image_version)}` when `info.image_version` is set. Stage 1 (fallback) is unchanged. No other component changes: `Card.svelte`, `Zoom.svelte` and `Modal.svelte` already use the helper.
- `web/e2e/images.spec.js` (from card-image-cdn) — add a case: the route-intercepted `/api/card` answer adds `image_version: 'abc123def456'`. The recorded CDN requests are `…/low.webp?v=abc123def456` for the card on the mat (and `high.webp?v=…` in the zoom). When the CDN aborts, the fallback request has no `?v=`. Run in all three profiles.
- `internal/webui/dist` — rebuilt by the normal build, as card-image-cdn does.

**Complexity**: Low
**Token estimate**: ~15k
**Agent strategy**: Single agent, sequential: Go side, then the helper, then build and run the Playwright case.

### Task: Clear image copies and purge the CDN

**File changes:**
- `internal/cards/images.go` — add the field `CDN Purger` (an interface with `Purge(ctx, paths []string) error`; `*spaces.CDN` satisfies it), and the unexported `clearing []Scope` under `mu`. Add `func (ic *ImageCache) inClearing(rel string) bool`. In `Image`, when the image is copyable and `inClearing(rel)`, count a source answer and return `(src, "", "")` without queuing. In the worker loop, skip (and unmark pending) any `rel` in clearing, and re-check it again just before marking known in `copyOne`, discarding a copy that finished during a clear (its objects are deleted by the clear's listing pass, or overwritten by the next copy).
- `internal/cards/images.go` — `func (ic *ImageCache) Clear(ctx context.Context, s Scope) []TierReport`:
  1. Under `mu`, append `s` to `clearing`. Delete `known`, `pending` and `failed` entries where `s.Image(rel)`.
  2. List `ImagePrefix` page by page (each page bounded at `clearTimeout`, ignoring the breaker), collecting keys `images/<rel>/<file>` where `s.Image(rel)`, and the distinct purge paths: for `All()` the single path `images/*`; for a set, `images/<lang>/<serie>/<set>/*` for each distinct first three segments seen.
  3. `Spaces.Delete(keys)` gives `{Tier: "images", Removed, Err}`.
  4. If `CDN != nil` and there were paths (or `All()`), call `CDN.Purge(ctx10s, paths)`, giving `{Tier: "cdn", Removed: len(paths), Err}`. With no CDN, `{Tier: "cdn", Err: ErrCDNNotConfigured}`, where `var ErrCDNNotConfigured = errors.New("CDN purge not configured")` is a sentinel the page shows as "not purged".
  5. Under `mu`, remove `s` from `clearing` (in a `defer`, so a panic or early return never leaves a scope stuck).
  Log `card images: cleared <scope>: <n> objects, CDN <purged|not purged|error>`.
- `internal/cards/images_clear_test.go` (new) — copies of `en/me/me01/001`, `en/me/me01/002`, `en/sv/sv1/1` and `en/swsh/swsh12.5/157` in the fake (via the copier or seeded objects with a listing). `Clear(Scope{Set:"me01"})` leaves only the sv1 and swsh12.5 objects, the CDN fake records `[["images/en/me/me01/*"]]`, and `Image` for me01 gives the source address with no version while sv1 keeps its CDN address. `Clear(Scope{})` leaves no `images/` objects and the purge is `["images/*"]`. During a clear held open (gate the fake's list with `Hang`), `Image` for a me01 image queues nothing, and after the clear the next `Image` queues a copy that lands with the new bytes and a new version. Without `CDN`, the report has `ErrCDNNotConfigured`. With the CDN fake failing, the `cdn` report has `Err` while the objects are still gone. With the fake Spaces denying deletes, the `images` report has `Err` and me01 is still answered from the source.

**Complexity**: Medium
**Token estimate**: ~35k
**Agent strategy**: Single agent, sequential: clearing guard in the front and workers, then `Clear`, then the tests under `-race`.

### Task: Name admins and guard the admin page

**File changes:**
- `internal/hostauth/hostauth.go:25-50` (`Config`) — add `Admins []string` with a comment: Google account emails allowed through `RequireAdmin`, compared case-insensitively and only for verified emails, empty meaning nobody. In `New` (`:83-121`), normalise into an unexported `admins map[string]bool` (trimmed, lower-cased, empties dropped).
- `internal/hostauth/hostauth.go:64-69` (`Account`) — add `EmailVerified bool` with a comment.
- `internal/hostauth/google.go:132-139` (`idClaims`) — add `EmailVerified json.RawMessage \`json:"email_verified"\``, and accept both `true` and `"true"` (Google has sent both forms). In `checkIDToken` (`:144-177`), set `Account.EmailVerified`.
- `internal/hostauth/seal.go:94-105` (`grantClaims`) — add `Verified bool \`json:"v,omitempty"\``. `account()` sets `EmailVerified: g.Verified`. `internal/hostauth/store.go:177-187` (`addSession`) seals `Verified: acct.EmailVerified`. Connector tokens (`issueTokens`, `:399-411`) may carry it too, for consistency, but nothing reads it there.
- `internal/hostauth/admin.go` (new) — `func (a *Auth) IsAdmin(acct Account) bool { return acct.EmailVerified && a.admins[strings.ToLower(strings.TrimSpace(acct.Email))] }`. `func (a *Auth) RequireAdmin(next http.Handler) http.Handler`, mirroring `RequireSession` (`internal/hostauth/board.go:56-95`):
  - With no session, a GET or HEAD page navigation (not a subresource) redirects 302 to `/auth/login?next=<RequestURI>` with `no-store`. Anything else gets 401 `sign in required`, logged as `hostauth: refused %s %s: not signed in`.
  - With a session but `!IsAdmin`, render 403 with `renderPage` (Title "Not an admin", Message "This page is for tcgreplay admins. You are signed in as <email>." plus, when `!EmailVerified`, "If you are an admin, sign out and sign in again." and a link to `/`). Log `hostauth: refused admin %s %s for %s`.
  - With a method other than GET or HEAD and `!a.sameSite(r)` (`board.go:110-118`), return 403 `admin changes must come from the playmat`.
  - Otherwise, `next.ServeHTTP` with `withAccount`. No rate limit is applied, since admin requests are rare and the page does not poll beyond its 2 s refresh while a job runs.
- `internal/hostauth/admin_test.go` (new) — use `hostauthtest.NewGoogle` with `Config.Admins: []string{"Admin@Example.com"}` and a trivial `next` that records calls. Cases: the admin signed in as `admin@example.com` gets GET 200 and POST (with `Origin: <public>`) 200; signed out, GET with `Sec-Fetch-Mode: navigate` gives 302 to `/auth/login?next=%2Fadmin` and POST gives 401; `SetAccount` to `other@example.com` gives 403 and `next` is not called; `Claims` setting `email_verified: false` gives 403 for the admin's email; `email_verified: "true"` (a string) passes; a cookie sealed without `v` (seal a `grantClaims` directly in an internal test) gives 403 but still passes `RequireSession`; a POST with `Origin: https://evil.example`, or no Origin or Referer, gives 403; `Admins: nil` refuses the admin. Existing `board_test.go`, `oauth_test.go` and `google_test.go` pass unchanged.

**Complexity**: Medium
**Token estimate**: ~30k
**Agent strategy**: Single agent, sequential: claims and seal, then the guard, then the tests.

### Task: Add the cache admin page and clear job

**File changes:**
- `internal/cacheadmin/cacheadmin.go` (new) — the package doc says this is the hosted playmat's cache admin page, served only behind `hostauth.RequireAdmin`. `Cards` and `Images` interfaces as in plan.md. `Service{Cards; Images; Now; Timeout time.Duration (default 10 * time.Minute); mu; job *Job; ctx context.Context}`. `var ErrBusy = errors.New("a clear is already running")`. `Start(ctx, by, name string, all bool) (Job, error)`: under `mu`, return `ErrBusy` if `job != nil && job.Running`. If `!all`, call `Cards.SetID(ctx, name)` outside `mu` (it may fetch). On error return it, so the caller maps `cards.ErrUnknownSet`. Re-check busy under `mu`, then create `Job{Scope, SetName, By, Started: Now(), Running: true}` and start a goroutine on `context.WithTimeout(s.ctx-or-Background, Timeout)` (a server-lifetime context set by `Run(ctx)`, so the job is not tied to the HTTP request). The goroutine calls `Cards.Clear`, then, if `Images != nil`, `Images.Clear`, and appends the reports. Under `mu`, it sets `Ended` and `Running = false`. It logs `cache admin: %s started clearing %s` and `cache admin: cleared %s in %s: %s` (each tier's count or error). `Status()` returns a copy of the job. `func (s *Service) Run(ctx context.Context)` stores the context and blocks until done. Add an unexported `wait()` for tests.
- `internal/cacheadmin/page.go` (new) — `html/template` page in the hostauth style (system font, max-width 32rem, light and dark). It shows: "Signed in as <email>"; "Clear everything" (`<form method=post action=/admin/clear><input type=hidden name=all value=1><button>`); "Clear one set" (`<form method=post action=/admin/clear><input name=set placeholder="MEG or me01" required><button>`); the job section ("Clearing <scope> for <admin>, started <time>…" with `<meta http-equiv="refresh" content="2">` while running, or "Last clear: <scope> by <admin> at <time>, took <d>" and a list of tiers: memory/disk/shared card entries removed, image copies removed, "CDN purged" / "CDN not purged (no token)" / "CDN purge failed: <err>"); and any message (an unknown set, busy). The scope shows as "everything" or "<set name> (<id>)". `Cache-Control: no-store`.
- `internal/cacheadmin/handler.go` (new) — `Handler()` is an `http.ServeMux` with `GET /admin`, which renders the page with `hostauth.AccountFrom` email and `Status()`, and `POST /admin/clear`, which runs `ParseForm`, takes `all := r.PostForm.Get("all") == "1"` and `set := r.PostForm.Get("set")`. With neither, or an empty set, it renders 400 "Name a set by its code (such as MEG) or id (such as me01)". `cards.ErrUnknownSet` renders 400 `No set is named "<name>". Nothing was cleared.`. `ErrBusy` renders 409. Other errors render 502 with the message. Success answers 303 to `/admin`.
- `internal/cacheadmin/cacheadmin_test.go` (new) — fake `Cards` (records `Clear` scopes; `SetID` knows `MEG`→`me01`, `me01`, `ME01`; can block on a channel) and fake `Images`. Wrap the handler with `hostauth.WithAccount` in a tiny test middleware. Cases: GET shows both forms and "Signed in as"; POST all gives 303, and after `wait()` the fake recorded `Scope{}` for cards and images and the page lists both reports; POST `set=MEG` and `set=me01` record `Scope{Set:"me01"}`; POST `set=XYZ` gives 400 with the message and no `Clear` call; POST with nothing gives 400; a second POST while the fake blocks gives 409; a `Cards` report with `Err` shows "failed" on the page; `Images == nil` works; while running, the page has the meta refresh, and after completion it does not. A second test uses the real `cards.Client` over the fake TCGdex, the tiered store over `spacestest`, and a real `ImageCache` with the CDN fake: POST `set=MEG` then `wait()`, and the fake Spaces has no `tcgdex/sets/me01.json`, no `tcgdex/cards/me01-*` and no `images/*/*/me01/*`, other sets' objects remain, and the CDN fake recorded the purge.

**Complexity**: Medium
**Token estimate**: ~40k
**Agent strategy**: 2 parallel agents: (a) service and job with the fake-based tests; (b) page template and handler. Then integrate and add the real-pieces test sequentially.

### Task: Serve the admin page in hosted mode

**File changes** (`hostedCardDB` comes from the shared card cache plan, extended by card-image-cdn to return `images`; adapt names to what landed):
- `cmd/tcgreplay/hosted.go` (`hostedCardDB`) — in the branch that builds the shared store, after the image cache is built: if `tok := os.Getenv("TCGREPLAY_SPACES_CDN_TOKEN"); tok != ""`, set `images.CDN = spaces.NewCDN(cfg, tok)`, with `CDN.API = c.cdnAPI` when that test-only config field is non-empty, and log `card images: CDN purge on clear: on`. Otherwise log `card images: CDN purge on clear: off (no TCGREPLAY_SPACES_CDN_TOKEN)`. In the branches without a shared store, a non-empty token logs `card cache: TCGREPLAY_SPACES_CDN_TOKEN ignored without the shared store`, which is not a warning.
- `cmd/tcgreplay/hosted.go:112-126` (`hostedAuth`) — read `TCGREPLAY_ADMINS` and split on commas into `hostauth.Config.Admins`. Add `adminText(n int) string` for the start-up log: `no cache admins (TCGREPLAY_ADMINS)` or `%d cache admins`.
- `cmd/tcgreplay/hosted.go:151-161` (`hostedHandler`) — take a `*cacheadmin.Service` and mount `mux.Handle("/admin", a.RequireAdmin(svc.Handler()))` and `mux.Handle("/admin/", a.RequireAdmin(svc.Handler()))` before the `/` board route.
- `cmd/tcgreplay/hosted.go:172-205` (`newHosted`) and `:207-247` (`runHosted`) — build `svc := &cacheadmin.Service{Cards: db}`, and set `svc.Images = images` only when `images != nil` (a nil `*ImageCache` in the interface would be non-nil). Run `go svc.Run(ctx)`, pass `svc` to `hostedHandler`, and add `adminText(len(admins))` to the start-up log line at `:229`.
- `cmd/tcgreplay/main.go` (`config`) — add the test-only field `cdnAPI string` beside the upstream plans' `cardBase` and `imageBase`, with no flag.
- `cmd/tcgreplay/hosted_admin_test.go` (new) — use `hostedServer` (`hosted_test.go:570-640`) extended to accept a card DB built by `hostedCardDB` with the fakes. `t.Setenv` the four `TCGREPLAY_SPACES_*` variables against `spacestest`, `TCGREPLAY_SPACES_CDN_TOKEN` against `spacestest.CDN`, and `TCGREPLAY_ADMINS=admin@example.com`, with `c.cardBase` and `c.imageBase` pointing at the fake TCGdex and image host. Cases:
  - The admin (`SetAccount`) gets the cookie, `GET /admin` gives 200, and `POST /admin/clear all=1` with `Origin: testPublic` gives 303. Poll `GET /admin` until it no longer shows "Clearing", then the fake has no `tcgdex/` or `images/` keys and the CDN fake recorded `images/*`. A fresh import through `import_log` or `new_game` via MCP shows cards (non-empty card text through `card_text`).
  - `set=MEG` and `set=me01` each remove only me01's objects.
  - `other@example.com` gets 403 on GET and POST, and signed out, GET gives 302 to sign-in and POST gives 401. In each case, `Keys("")` on the fake is unchanged and it recorded zero delete POSTs.
  - Open game: the admin opens a game via MCP, clears all, and `GET /api/state` (or the manager's `View`) still shows the cards, and a tool call such as `draw` succeeds.
  - No Spaces settings: `/admin` still clears memory and disk, and the page shows no image or CDN tiers. A token without Spaces logs "ignored". With Spaces but no token, the page shows "CDN not purged".
  - `captureLog`: the start-up names `1 cache admins` (never the email) and contains no "warning". The desktop `newServer` mounts no `/admin` (a GET gives the board or 404, never the admin page).

**Complexity**: Medium
**Token estimate**: ~40k
**Agent strategy**: Single agent, sequential: wiring, then the tests, reusing the existing hosted helpers.

### Task: Declare and document the admin and CDN settings

**File changes:**
- `.do/app.yaml` (`envs`) — add `TCGREPLAY_ADMINS` (`scope: RUN_TIME`, no value in the file, with a comment saying it is a comma-separated list of the Google account emails allowed to use `/admin` to clear the card cache, set in the console, and that unset means nobody). Add `TCGREPLAY_SPACES_CDN_TOKEN` (`scope: RUN_TIME`, `type: SECRET`, with a comment saying it is a DigitalOcean API token with CDN read and update access, used to purge the bucket's CDN when an admin clears image copies, and that unset means clears do not purge). Update the header comment to mention the admin page.
- `Dockerfile:1-6` — the header comment lists `TCGREPLAY_ADMINS` and `TCGREPLAY_SPACES_CDN_TOKEN` with a few words each.
- `Makefile:49-63` (`docker-run`) — add `-e TCGREPLAY_ADMINS -e TCGREPLAY_SPACES_CDN_TOKEN` and mention them in the target's comment.
- `TCG Review Playmat Design and Build Plan.md:302-330` (hosted section, settings table and operator setup, as the upstream plans leave them) — add a paragraph on the admin page (`/admin`, Google sign-in, clearing everything or one set by code or id across memory, disk, Spaces card data and image copies, the CDN purge, versioned image addresses, open games keep playing). Add two table rows (`TCGREPLAY_ADMINS`, environment only, default nobody; `TCGREPLAY_SPACES_CDN_TOKEN`, environment only, default no purge). Add operator steps: create a DigitalOcean API token with custom scopes CDN read and update, set it encrypted, set `TCGREPLAY_ADMINS`, and make sure the Spaces key can list and delete. Replace any "delete the `tcgdex/` objects by hand to force fresh data" sentence with the admin clear.
- Do not touch `.dockerignore` or the build context.

**Complexity**: Low
**Token estimate**: ~12k
**Agent strategy**: Single agent, sequential.

### Task: Update the card cache knowledge entries

**File changes** (knowledge entries, repo tier, store `tcgreplay`, written only through the `spek-knowledge` skill, never by path):
- `gotchas/tcgdex-cache-never-expires.md` (as rewritten and possibly retitled by the upstream plans) — replace the "until admin cache clearing exists, delete `tcgdex/` by hand" guidance. On the hosted playmat an admin clears from `/admin`, everything or one set by decklist code or TCGdex id, and the clear covers memory, local disk, Spaces `tcgdex/` and `images/` copies, and purges the CDN when `TCGREPLAY_SPACES_CDN_TOKEN` is set. A set is `sets/<id>` plus `cards/<id>-*`, and images with `<id>` as the third path segment. Open games keep their cards: `Cached` returns cleared copies until fresh ones arrive, while `Card` refetches at once. Desktop still clears by deleting the cache directory.
- `gotchas/board-card-info-per-page-load.md` — add that hosted CDN image addresses carry `?v=<content version>` (from the copies' ETags), so changed art gets a new URL despite the 7-day `Cache-Control`, and that after an admin clear an open board shows fresh card data and art only after a reload.

**Complexity**: Low
**Token estimate**: ~8k
**Agent strategy**: Single agent, sequential, through the `spek-knowledge` skill.

### Task: Name the admins and create the CDN purge token

What the person does:
1. In the DigitalOcean control panel, create a personal access token with custom scopes limited to CDN endpoints, read and update (`cdn:read`, `cdn:update`), with an expiry that suits the operator.
2. Check that the Spaces access key created for the shared card cache (`20261104082020-109dceb1-tcgdex-spaces-cache` human task) can list and delete objects in the bucket. A full-access key for the bucket can. A read/write-limited key must allow delete.
3. In App Platform → the playmat component → Environment Variables, set `TCGREPLAY_ADMINS` to the admins' Google account emails (comma-separated), and `TCGREPLAY_SPACES_CDN_TOKEN` to the token, encrypted. `doctl apps update` does not carry secret values.
4. Redeploy.

How it is checked: the start-up log shows `<n> cache admins` and `CDN purge on clear: on`, with no warning, and an admin can open `/admin` after signing in. The manual checks in the implementation test plan then exercise real clears and purges.

## Testing Strategy

Per task:
- **Delete objects in bulk and report their checksums** (`internal/spaces/client_test.go`):
  - Deletes remove exactly the keys given, and 2,500 keys go out as 3 POSTs.
  - Missing keys are not an error. A denied request returns `*Error`, and refused keys come back named in `*DeleteError`.
  - The fake checks Content-MD5.
  - ETags equal the MD5 and change when an object is rewritten. Existing list tests are unchanged.
- **Purge the bucket CDN through the DigitalOcean API** (`internal/spaces/cdn_test.go`):
  - The purge paths and the token reach `cdn-1`.
  - A listing with only a decoy endpoint gives `ErrNoCDN`.
  - A 401 does not put the token in the error text.
  - A 500 and an unreachable API are reported.
  - The origin is derived correctly.
- **Clear the card stores**:
  - `internal/cards/scope_test.go`: near-miss ids and image path segments.
  - `internal/cards/clear_store_test.go`: disk, Spaces and tiered stores remove exactly what is in scope, with correct counts, and unrelated files are kept. A shared-store failure is reported with the local tier still cleared, and a clear is still sent while the breaker is open.
- **Clear the card database and name sets** (`internal/cards/clear_test.go`, all under `-race`):
  - `SetID` resolves by code, by id, and in any case. An unknown name is refused, with at most one set-list refresh.
  - A clear-all then makes one fresh request and returns the updated data.
  - A set clear leaves other sets alone.
  - `Cached` returns the old copy, then the fresh one after settle.
  - With the source down, the old copy is returned.
  - The generation fence holds on a gated catalog build, and the failed-card memory is reset.
- **Prove an open game survives a clear** (`internal/game/clear_test.go`):
  - After a clear, `View` answers within 1 s with every definition, and a play succeeds.
  - Fresh fetches follow after settle.
  - The same holds with the source down.
- **Version image copies by their content** (`internal/cards/images_test.go`):
  - The version formula is applied. The same bytes give the same version, and changed bytes give a new one.
  - Versions are stable across a restart, via the listing's ETags.
  - Half copies and copies without ETags count as uncopied.
  - `Image` does not block.
- **Send image versions to the board**:
  - `internal/webui/webui_test.go`: `image_version` is present only with a versioned CDN answer.
  - `web/e2e/images.spec.js`, in three profiles: CDN requests carry `?v=`, and fallback requests do not.
- **Clear image copies and purge the CDN** (`internal/cards/images_clear_test.go`):
  - Set-scope and full-scope deletions remove the right copies, and the right paths are purged.
  - No copy is made during a clear. The next copy afterwards gets a new version.
  - Reports are correct with no CDN, with a failing CDN, and with deletes denied.
- **Name admins and guard the admin page** (`internal/hostauth/admin_test.go`, with the existing hostauth tests unchanged):
  - An admin passes, matched case-insensitively.
  - Signed out, a page request redirects to sign-in and a direct request gets 401.
  - A non-admin gets 403, and so does an unverified email, in both the bool and string forms.
  - A cookie from before the change gets 403 on the admin page but still works on the board.
  - A cross-site POST and a POST with no origin get 403.
  - With no admins configured, nobody passes.
- **Add the cache admin page and clear job** (`internal/cacheadmin/cacheadmin_test.go`):
  - The page shows its content. The all, code and id scopes each clear the right scope.
  - An unknown or empty set gets 400 and clears nothing. A clear while one is running gets 409.
  - Failures are displayed. The meta refresh appears only while a clear runs, and a nil image cache works.
  - With the real pieces over the fakes, a set clear removes the right objects.
- **Serve the admin page in hosted mode** (`cmd/tcgreplay/hosted_admin_test.go`):
  - End to end, an admin clears everything and clears single sets, with fake Spaces, CDN and TCGdex.
  - Refused requests leave the bucket unchanged.
  - An open game survives a clear.
  - The variants without Spaces and without a token behave as specified.
  - The start-up log gives only the admin count, with no warning.
  - Desktop builds have no admin page.
- **Declare and document the admin and CDN settings** and **Update the card cache knowledge entries** — no automated tests (docs, config and knowledge). The existing `internal/guides` packaging still builds.
- **Name the admins and create the CDN purge token** — human. It is checked through the start-up log and the manual checks.

Manual checks (each **Manual — captured in the implementation test plan**):
- Real admin sign-in, and refusal of a non-admin.
- A real clear-all.
- A real set clear by code and by id, and refusal of an unknown set.
- A real CDN purge, and image refresh within 1 hour in a browser that saw the old art.
- A real open game surviving a clear.
- Real Spaces behaviour for DeleteObjects, ETags and `?v=`.
- The success metric, timed on the real deployment.

## Project References

- Spec: `20261104090640-a5a74fab-admin-cache-clearing` (epic `20261104082020-109dceb1-tcgdex-spaces-cache`).
- Upstream plans: `20261104090640-51bd1f36-card-cache-freshness`, `20261104082020-109dceb1-tcgdex-spaces-cache`, `20261104090640-25f673ed-card-image-cdn` (read with `spektacular plan file read <name> plan|context`).
- Design documents: the spec references none.
- Knowledge (repo tier, store `tcgreplay`): `conventions/skills-text-and-packaging.md`, `gotchas/board-card-info-per-page-load.md`, `gotchas/tcgdex-cache-never-expires.md`, `gotchas/game-listeners-under-lock.md`.
- Repo root: `tcgreplay` at `/home/nicj/code/github.com/nicholasjackson/tcgreplay` (from `spektacular repo list`).

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

Per-task estimates:
- Delete objects in bulk and report their checksums: ~30k (Medium).
- Purge the bucket's CDN: ~20k (Low).
- Clear the card stores: ~35k (Medium).
- Clear the card database and name sets: ~55k (High).
- Prove an open game survives a clear: ~20k (Low).
- Version image copies by their content: ~20k (Low).
- Send image versions to the board: ~15k (Low).
- Clear image copies and purge the CDN: ~35k (Medium).
- Name admins and guard the admin page: ~30k (Medium).
- Add the cache admin page and clear job: ~40k (Medium).
- Serve the admin page in hosted mode: ~40k (Medium).
- Declare and document the settings: ~12k (Low).
- Update the knowledge entries: ~8k (Low).
- Name the admins and create the CDN purge token: human task, no agent.

## Migration Notes

- **Sign-ins from before this change** keep working for the board and MCP. For the admin page they count as unverified, so an admin signs out and in once. The refusal page says so.
- **Image addresses change once.** Every CDN image address gains `?v=<version>`, so each browser fetches each copied image once more. No objects are rewritten.
- **New settings.** `TCGREPLAY_ADMINS` defaults to nobody and `TCGREPLAY_SPACES_CDN_TOKEN` to no purge. Deploying without them changes nothing, except that `/admin` refuses everyone.
- **No data migration.** The bucket layout, the envelope and the disk layout are unchanged.

## Performance Considerations

- A full clear lists `tcgdex/` and `images/` at 1,000 keys a page and deletes at 1,000 keys a request. For about 20,000 card objects plus a few thousand image copies, that is roughly 50 requests, taking from seconds to tens of seconds. It runs as a background job that never blocks lookups or games.
- After a clear-all, the first deck resolution rebuilds the catalog from the card source, fetching every set's card list 8 at a time as on a cold start. Card lookups do not wait for this. After a set clear, only that set's card list is fetched again.
- The cleared mark is one boolean per in-memory card. The memory sweep is one pass under `Client.mu` with no I/O, taking microseconds for thousands of cards.
- `Image` gains one string per known copy and a scan of the clearing list, which is usually empty. It still does not block.
- Clears bypass the lookup breaker and use a 30 s bound per Spaces request. A Spaces outage therefore costs the admin a reported error, and never delays a lookup.
