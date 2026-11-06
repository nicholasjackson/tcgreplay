---
created_date: "2026-11-04"
document_status: final
closed_date: "2026-11-04"
---

# Plan: 20261104090640-a5a74fab-admin-cache-clearing

<!-- Metadata -->
<!-- Created: 2026-11-04T12:29:05Z -->
<!-- Commit: 8c26660570c61a3e063c43882de3fae7a6168aeb -->
<!-- Branch: main -->
<!-- Repository: git@github.com:nicholasjackson/tcgreplay.git -->

## Overview

Admins of the hosted playmat get a small page, behind the existing Google sign-in, from which they can clear the card cache, either all of it or one set named by its decklist code (such as MEG) or its TCGdex id (such as me01). A clear reaches every cache tier the hosted playmat keeps: memory, local disk, the DigitalOcean Spaces card data and the public image copies, and it purges the CDN. Card image addresses now carry a content version, so corrected art also reaches viewers' browsers. Open games keep showing and playing their cards throughout, while the cleared data is fetched fresh from TCGdex, so a correction at the card source reaches the board within a minute, without a redeploy.

## Conventions

- **Guide text and packaging (`conventions/skills-text-and-packaging.md`)** — applies in its packaging half: this plan edits the `Dockerfile` comment, the `Makefile` container target and the App Platform definition, and must not exclude `internal/guides` from the build context, because the binary embeds the guides. The guide-wording half does not bear on this work, since no guide text changes.

## Architecture & Design Decisions

All the work lands in the one registered repo, `tcgreplay`. It adds a new `internal/cacheadmin` package (the admin page and the clear job), an admin check in `internal/hostauth`, clearing in `internal/cards` (stores, client and image cache), a batch delete and a CDN purge client in `internal/spaces` (with the fake learning both), one optional field on `/api/card` in `internal/webui` and one line in the board's image helper in `web/src`. It also wires the pieces into hosted mode in `cmd/tcgreplay` and updates the hosted deploy files and docs. It builds on the three upstream plans in this epic, which must land first, and keeps every rule they fix. From the freshness plan it takes the `Store`/`Entry`/`ErrNotCached` contract, the freshness rules above the store, the in-memory catalog with no persisted set index, and the rule that `Client.mu` is never held across I/O. From the shared card cache plan it takes the tiered store with `Local()`, `internal/spaces` with its signer, the `spacestest` fake, the private `tcgdex/` prefix at the disk relative paths, the `TCGREPLAY_SPACES_*` settings (environment only, hosted only) and the hosted helper that builds the card database. From the card image plan it takes `List` with `ListPage{Keys, Next}`, `cards.ImageCache` with its in-memory copies keyed by path, the public `images/<TCGdex asset path>/{low,high}.webp` layout with its 7-day `Cache-Control`, and the board's image fallback. None of those shapes is redesigned. Three are extended additively: `ListPage` gains `ETags`, `ImageCache` gains a version per copy and a `Clear`, and `webui.ImageSource.Image` also returns that version.

**Clearing reaches every tier through the layers that already own them.** A clear names its scope: everything, or one set id. Each layer gets its own way to forget what is in scope, and the `Store` interface does not grow. As with `Local()`, the disk store, the Spaces store and the tiered store implement an optional `Clear` found by type assertion. Set S is the key `sets/S` plus every `cards/S-…` key. TCGdex card ids are `<set id>-<local id>`, so the `-` keeps `sv03` from matching `sv03.5`. On disk and in Spaces the same rule applies to the safe file names under the cache directory and under `tcgdex/`. The tiered store clears the shared tier first and the local tier second, so a lookup during the clear cannot copy an old shared entry onto a freshly cleared disk. The clear calls Spaces directly, with its own bounds, and bypasses the breaker, so the admin sees a Spaces failure instead of a silent skip. Above the store, `cards.Client.Clear` drops the catalog snapshot, forgets the refresh tracker's throttles for keys in scope, and **marks** in-memory card entries in scope as cleared instead of deleting them. That choice protects open games. They read cards only through `Cached` under their lock (project rule: nothing under the game lock calls the network), and deleting entries would blank their cards until each background warm finished. A cleared entry is still returned by `Cached`, which starts the existing background warm. `Card` treats it as a miss and fetches it from the card source, or falls back to the old copy if the source fails. Real lookups (`Card`, `Resolve`, `ByName`, so the board's card endpoint, imports and MCP tools) are answered from fresh data straight after a clear, while games already on the board keep their cards and pick up the fresh copies as they land. A catalog build or memory fill that began before a clear is not kept, so a slow fetch cannot put pre-clear data back. A set is named by its TCGdex id (case-insensitive) or by its decklist code, through the same aliases and catalog code index `Resolve` uses. A name that matches neither is refused before anything is touched.

**Images are cleared, purged and re-addressed so that browsers fetch them again.** `ImageCache.Clear` first stops answering CDN addresses for copies in scope and holds off re-copying them. It then lists `images/`, deletes the objects in scope (a copy is in set S when the third segment of its TCGdex path `<lang>/<serie>/<set>/<number>` is S), and purges those paths from the bucket's CDN through the DigitalOcean API. Finally it lets copying resume, so the next view copies the current TCGdex art again. The purge alone would not meet "the board shows current art within 1 hour". Copies carry a 7-day browser `Cache-Control` and are copied back to the same key, so any viewer who saw the old copy would keep it for up to a week. So the board's CDN address now carries a content version, `?v=<short hash of both objects' ETags>`. The copier computes it from the bytes it uploads, an ETag being the MD5 of a single-part upload. The start-up listing reads it back from the ETags the listing reports, so it survives restarts and changes only when the bytes change. Changed art therefore gets a new URL that no browser or CDN edge has cached, and identical art keeps its URL and its caches. The purge stays as the spec asks: it is what refreshes the edge if the CDN ignores query strings. It needs a DigitalOcean API token, `TCGREPLAY_SPACES_CDN_TOKEN`, an encrypted secret in the same environment-only family. The CDN endpoint is found by matching the bucket's origin. Without the token, clears still work, and the page and log say the CDN was not purged.

**Admins sign in as everyone does, and the clear runs as a background job.** `TCGREPLAY_ADMINS` lists admins' Google account emails. `hostauth` gains `RequireAdmin`, beside `RequireSession`. A signed-out page request is sent through the existing Google sign-in. A signed-out direct request gets 401. A signed-in account not on the list, or whose email Google did not mark verified, gets 403. A POST not sent from the playmat's own pages (judged by the same Origin check sign-out uses) gets 403. The verified flag is read from Google's ID token and sealed into the session cookie. `internal/cacheadmin` serves `GET /admin`, a small server-rendered page in the style of the sign-in pages, and `POST /admin/clear`. The POST resolves the set name first, so an unknown set is refused with a message and nothing is cleared. It then starts one clear job at a time and redirects to the page, which refreshes itself while the job runs and then shows what each tier removed, the purge result and any error. A full clear deletes thousands of objects with S3's batch delete (`Delete`, added to the Spaces client on the same signer, 1,000 keys a request). That can outlast a proxy's request limit, which is why the work runs in the background. The admin page works with or without the shared store: without it there is only memory and disk to clear, and images come straight from TCGdex. Desktop, daemon and dev builds never serve it.

This beats deleting memory outright (open games lose their cards mid-play), stale-while-revalidate after a clear (the first lookup still answers old data), a purge without versioned URLs (viewers' browsers keep old art for a week), a version in the object key (it breaks the fixed image layout), and a synchronous clear inside the request. The evidence and the rejected options are in `research.md#alternatives-considered-and-rejected`. Packaging is unchanged: the Docker build context and `.dockerignore` are not touched, so `internal/guides` stays in the build (convention `conventions/skills-text-and-packaging.md`).

## Component Breakdown

- **Spaces client (changed, from the shared card cache and card image plans).** It gains a batch delete: it removes a list of keys, 1,000 a request with S3's DeleteObjects, on the existing signer, and reports which keys failed. Its listing now also reports each object's ETag beside its key, as an additive field, so callers can tell when an object's bytes changed. Everything else (settings, `Get`, `Put`, `List`, public address, typed errors) is used as those plans leave it. It stays card-agnostic.
- **CDN purge client (new, inside the Spaces package).** It owns the one DigitalOcean API call this spec needs: purging paths (with `*` wildcards) from the bucket's CDN cache. It finds the bucket's CDN endpoint by matching the endpoint's origin to the bucket's origin host, then sends the purge with a bearer token. It reports "no CDN endpoint for this bucket" and refused tokens as distinct errors. It is built only when the CDN token setting is present and the shared store is on. It knows nothing about cards.
- **Fake Spaces and fake DigitalOcean API (changed and new, test support).** The existing fake answers batch deletes (removing objects, counting requests, and failing or denying like any other call) and puts ETags (MD5 of the stored bytes) in listings. A new fake of the DigitalOcean CDN API, in the same test package, lists one endpoint for the fake bucket's origin, records every purge with its paths, checks the bearer token and can fail. Every automated test of deletion and purging uses them, since no real bucket, CDN or token is available.
- **Card stores (changed, from the freshness and shared card cache plans).** The disk store, the Spaces store and the tiered store each learn to clear a scope (everything, or one set: `sets/S` and every `cards/S-…` key) and to report how many entries they removed. The disk store removes only the files it owns under the cache directory. The Spaces store lists `tcgdex/` (or the set's card prefix) and batch-deletes. The tiered store clears its shared tier and then its local tier, calling Spaces directly with longer bounds and outside the breaker, and reports both counts and any shared-tier error. The `Store` interface itself is unchanged: clearing is an optional interface the client finds by type assertion, like `Local()`.
- **Card database client (changed).** `cards.Client` gains two methods outside the `DB` interface. **Set naming** turns a name into a TCGdex set id: an exact id (any case) from the catalog's set list, or a decklist code through the same aliases and code index deck resolution uses. It refuses a name that matches neither, after the freshness plan's one-per-5-minutes refresh for misses, so a brand-new set is found. **Clear** clears a scope across every tier the client holds. It clears the store through its optional interface, then, under the lock and with no I/O, marks in-memory card entries in scope as cleared, drops the catalog snapshot, forgets the refresh tracker's throttles for keys in scope, and moves the clear generation on. Lookups learn one rule: `Card` treats a cleared memory entry as a miss and fetches it, falling back to the old copy if the card source fails, while `Cached` still returns it and starts the background warm. A catalog build or memory fill that began under an earlier generation is not kept. The freshness rules, single-flight and tiering are otherwise untouched.
- **Image cache (changed, from the card image plan).** Its in-memory copies keyed by path now also hold each copy's content version, a short hash of the two objects' ETags. The copier takes it from the MD5 of the bytes it uploads, and the start-up listing from the ETags reported. Its answer for a copied image carries that version. It gains **Clear** for a scope. Clear stops answering CDN addresses for copies in scope at once and holds off queuing or copying them. It lists `images/`, batch-deletes the objects in scope (a copy belongs to set S when the third segment of its TCGdex path is S), and purges those paths from the CDN when a purge client is set. It then lets copying resume, so the next view copies the current TCGdex art again. It reports what it removed and whether the purge ran.
- **Board card endpoint and board images (changed).** The image source the card endpoint asks also gives the copy's version. When there is one, `/api/card` returns it as an optional `image_version` field. The board's one image helper appends `?v=<version>` to the primary (CDN) address only, never to the TCGdex fallback. Without an image source, or for an image without a copy, the response and the board are byte-for-byte as before.
- **Admin check (changed, in the sign-in service).** The sign-in service learns the admin list (Google account emails, matched case-insensitively) and whether Google marked each signed-in email verified, a flag it now reads from the ID token and seals into the session cookie. Its new admin guard sits beside the board's session guard. A signed-out page request is sent through Google sign-in. A signed-out direct request gets 401. A signed-in non-admin, an unverified email or a session from before this change gets 403 with a short page. A state-changing request not sent from the playmat's own pages gets 403. An admin request passes through carrying the account. Board, MCP and connector sign-in are otherwise unchanged.
- **Cache admin page and clear job (new package).** It owns the admin page and the clear job, behind the admin guard. The page shows the signed-in admin, a "Clear everything" action, a "Clear one set" field taking a set code or set id, and the state of the current or last clear. That state covers the scope, who started it, when, what each tier removed, the CDN purge result and any error, and the page refreshes itself while a clear runs. A clear request names the set first, so an unknown name is refused with a message and nothing changes. It then starts the job, unless one is already running, in which case it says so. The job runs the card database's clear and then the image cache's clear, bounded overall, and logs one line when it starts and one when it ends. It depends on the card database client and, when present, the image cache, through narrow interfaces, so it is tested with fakes and with the real pieces.
- **Hosted wiring (changed).** Hosted start-up reads the admin list and the CDN token from the environment. It builds the CDN purge client only when the token is set and the shared store is on, and gives it to the image cache. It builds the cache admin service from the hosted card database and image cache, and mounts the admin page behind the admin guard. The start-up log states how many admins are named and whether CDN purging is on. A token without the shared store is logged as ignored. Desktop, daemon and dev builds are untouched.
- **Hosted deployment and docs (changed).** The App Platform definition declares the admin list and the CDN token (encrypted). The container run target passes them through, and the image comment lists them. The design document's hosted settings table and operator setup describe the admin page and how to create a DigitalOcean API token with CDN read and update access. The card cache and board card info knowledge entries describe clearing and versioned image addresses.
- **Unchanged.** The `Store` and `DB` interfaces, the freshness policy, the breaker for lookups, the game, the importer, the MCP tools, `cardstest.FakeDB`, the board's update protocol and its once-per-page card info.

## Data Structures & Interfaces

**Unchanged contracts this plan relies on.** From the freshness plan: `cards.Store`, `cards.Entry`, `cards.ErrNotCached`, the keys `sets`, `sets/<id>` and `cards/<id>`, the envelope, `RetryAfter`, and the `DB` interface with `cardstest.FakeDB`. From the shared card cache plan: `spaces.Config`, `FromEnv`, `New`, `Get`, `Put`, `PutOptions`, `ErrNotFound`, `*spaces.Error`, `spacestest.Server` and its knobs, `NewSpacesStore` (objects under `tcgdex/`), `NewTieredStore` with `Local()`, `SharedTimeout` and `SharedRetryAfter`, and the four `TCGREPLAY_SPACES_*` settings. From the card image plan: `List`, `Config.PublicBase`, `ImagePrefix`, `ImageFiles`, `ImageCacheControl`, and the `images/<path>/{low,high}.webp` layout. No MCP tool schema changes, and the board's update protocol does not change.

**`spaces.ListPage` (extended) and `spaces.Client.Delete` (new).** Listing also reports ETags. Delete removes keys in batches.

```go
type ListPage struct {
    Keys  []string
    Next  string
    ETags map[string]string // key -> ETag, unquoted (hex MD5 for single-part uploads)
}

// Delete removes keys, 1,000 per DeleteObjects request. Missing keys are not
// errors. A *DeleteError lists the keys S3 refused; other failures are *Error.
func (c *Client) Delete(ctx context.Context, keys []string) error

type DeleteError struct{ Failed map[string]string } // key -> S3 error code
```

**`spaces.CDN` (new).** This is the DigitalOcean CDN purge client for the bucket.

```go
type CDN struct {
    Token  string       // DigitalOcean API token (TCGREPLAY_SPACES_CDN_TOKEN)
    Origin string       // "<bucket>.<region>.digitaloceanspaces.com", from Config
    API    string       // default "https://api.digitalocean.com"; tests point at the fake
    HTTP   *http.Client
}

func NewCDN(cfg Config, token string) *CDN
// Purge finds the endpoint whose origin is Origin and purges paths
// (no leading slash; "*" wildcards). ErrNoCDN when no endpoint matches.
func (c *CDN) Purge(ctx context.Context, paths []string) error
var ErrNoCDN = errors.New("spaces: no CDN endpoint for this bucket")
```

**`spacestest` additions (test support).** `Server` answers `POST /<bucket>?delete` and puts `<ETag>` in listings. The new `spacestest.CDN`, built with `NewCDN(t, origin, token)`, serves `GET /v2/cdn/endpoints` and `DELETE /v2/cdn/endpoints/{id}/cache`. It records `Purges() [][]string`, checks the bearer token (403 otherwise), and has a `Fail` knob.

**Clear scope and reports (new, `cards`).** One scope type is shared by every layer that clears.

```go
// Scope is what a clear covers: everything (Set == "") or one TCGdex set id.
type Scope struct{ Set string }

func (s Scope) Key(key string) bool    // sets/S, or cards/S-…; every key when Set == ""
func (s Scope) Image(rel string) bool  // third segment of <lang>/<serie>/<set>/<n> == S

// TierReport is what one tier removed, and its error if it could not finish.
type TierReport struct {
    Tier    string // "memory", "disk", "shared", "images", "cdn"
    Removed int
    Err     error
}

// Optional store interface, found by type assertion (Store does not change).
type clearer interface {
    Clear(ctx context.Context, s Scope) []TierReport
}
```

**`cards.Client` (new methods, not part of `DB`).**

```go
var ErrUnknownSet = fmt.Errorf("no such set: %w", ErrNotFound)

// SetID names a set by TCGdex id (any case) or decklist code (aliases applied).
func (c *Client) SetID(ctx context.Context, name string) (id, setName string, err error)

// Clear clears s from the store tiers, then memory, catalog and refresh state.
func (c *Client) Clear(ctx context.Context, s Scope) []TierReport
```

The in-memory card entry gains a `cleared` mark, and the client gains a clear generation. Both are unexported, and the next section describes their rules.

**`cards.ImageCache` (extended).** Copies keyed by path now carry a version, and the cache can clear and purge.

```go
type ImageCache struct {
    // ... fields from the card image plan ...
    CDN Purger // optional; nil means no purge
    // unexported: known map[path]version (was map[path]bool), clearing []Scope
}

type Purger interface{ Purge(ctx context.Context, paths []string) error } // *spaces.CDN

// Image now also returns the copy's version ("" without a copy).
func (ic *ImageCache) Image(src string) (image, fallback, version string)

// Clear forgets, deletes and purges the copies in s; reports "images" and "cdn".
func (ic *ImageCache) Clear(ctx context.Context, s Scope) []TierReport
```

A version is the first 12 hex characters of SHA-256 over `low.webp`'s ETag, a newline, and `high.webp`'s ETag.

**`webui.ImageSource` and `/api/card` (changed, board protocol).** The interface's one method returns the version too. `CardInfo` gains one optional field, and the board appends `?v=<image_version>` to `image` + `/low.webp` or `/high.webp`, never to `image_fallback`.

```go
type ImageSource interface {
    Image(src string) (image, fallback, version string)
}
// CardInfo: ImageVersion string `json:"image_version,omitempty"`
```

**`hostauth` (changed).**

```go
type Config struct {
    // ... existing fields ...
    Admins []string // Google account emails; case-insensitive; empty = no admins
}
type Account struct {
    ID, Email     string
    EmailVerified bool // Google's email_verified; sealed in the board cookie
}
func (a *Auth) IsAdmin(acct Account) bool
// RequireAdmin: signed-out page -> Google sign-in; signed-out other -> 401;
// non-admin or unverified -> 403; non-GET not from the public origin -> 403.
func (a *Auth) RequireAdmin(next http.Handler) http.Handler
```

The board cookie's sealed claims gain `"v": true` when the email is verified, omitted otherwise. A cookie sealed before this change opens as unverified.

**`cacheadmin` (new package).**

```go
type Cards interface {
    SetID(ctx context.Context, name string) (id, setName string, err error)
    Clear(ctx context.Context, s cards.Scope) []cards.TierReport
}
type Images interface {
    Clear(ctx context.Context, s cards.Scope) []cards.TierReport
}

type Service struct {
    Cards  Cards
    Images Images           // nil without the shared store
    Now    func() time.Time
    // unexported: mu, current/last Job
}

type Job struct {
    Scope            cards.Scope
    SetName, By      string // set's display name; admin email
    Started, Ended   time.Time
    Reports          []cards.TierReport
    Running          bool
}

func (s *Service) Start(ctx context.Context, by, name string, all bool) (Job, error) // ErrUnknownSet, ErrBusy
func (s *Service) Status() (current Job, ok bool)
func (s *Service) Handler() http.Handler // GET /admin, POST /admin/clear
```

**HTTP surface (new, hosted only).** `GET /admin` returns an HTML page. `POST /admin/clear` takes a form (`all=1`, or `set=<code or id>`). It answers 303 to `/admin` on start, 400 with the page and a message for an unknown or empty set, and 409 when a clear is already running. All of it sits behind `RequireAdmin`. Responses are `Cache-Control: no-store`.

**Hosted settings (new; environment only, hosted only).**

| Variable | Meaning |
|---|---|
| `TCGREPLAY_ADMINS` | Comma-separated Google account emails allowed to use `/admin`. Unset means nobody. |
| `TCGREPLAY_SPACES_CDN_TOKEN` | DigitalOcean API token with CDN read and update access (encrypted secret). Unset means clears do not purge the CDN. Ignored without the four shared-store settings. |

## Implementation Detail

**One scope type, cleared layer by layer.** The plan's one new cross-cutting idea is the clear scope: everything, or one TCGdex set id. It answers "is this key in scope" for card data and "is this image path in scope" for copies. Each layer that holds card data asks it and forgets what matches: memory, the catalog, the refresh tracker, the disk store, the Spaces store and the image cache. Nothing reaches across layers. The client never learns how the disk lays out files, and the image cache never learns about card keys. The scope is defined and tested once, including the near-miss set ids (`sv03` against `sv03.5`, `swsh12` against `swsh12tg`), so the rule cannot drift between layers. Clearing reports, rather than returns, its errors. Each layer gives a small per-tier report (what it removed and any failure), so a partial failure in Spaces still leaves memory and disk cleared, and the page shows exactly which tier needs another try. Clearing is idempotent, so trying again is always safe.

**Optional capabilities by type assertion, as with `Local()`.** The shared card cache plan found the tiered store's local tier through an unexported optional interface rather than widening `Store`. Clearing follows the same pattern, so `Store`, `Entry` and `ErrNotCached` keep their exact shape, and test stores that do not clear keep compiling. A developer reading a store sees `Get`, `Put` and, next to them, `Clear`. The tiered store's `Clear` is the only place the shared-before-local ordering lives, with a comment saying why: a concurrent lookup must not copy an old shared entry onto a cleared disk. Clears call Spaces directly with their own bounds and ignore the breaker, because the breaker protects lookups, not admin actions.

**"Cleared" is a third memory state, next to fresh and stale.** The freshness plan gives each in-memory card an entry with its fetch time, and the policy above the store decides fresh or stale. This plan adds one mark: cleared. Its rule is short and lives where the lookup decides what to do with a memory hit. For `Card` a cleared entry is a miss: go to the store, then the card source, and keep the old copy only if the source fails. For `Cached` it is a hit that starts the background warm. A clear generation counter, read when a load starts and checked before its result is kept, stops a fetch or catalog build that began before the clear from putting old data back. `Client.mu` is still held only to read or swap maps. The clear does its store I/O first and takes the lock only for the in-memory sweep, following the freshness plan's rule.

**Content versions for public copies.** The image cache's copies keyed by path change from "copied" to "copied, at this version". A version comes from the objects' ETags, so the bucket itself is the record and nothing new is persisted. The copier hashes what it uploads, and the start-up listing reads the ETags back. A copy whose two ETags are not both known counts as uncopied, as a half copy already does. The image cache's front stays non-blocking: `Image` is still a map read under a short mutex. During a clear, scopes being cleared are held in a small list that both the front (no CDN answer, no queuing) and the workers (skip) check, so a fresh copy can never be made and then deleted by the clear still running. The board change is one line in the shared image helper from the card image plan. The three image spots inherit it.

**A small admin surface in the hosted style.** The admin guard follows `RequireSession` almost line for line: same session lookup, same page-versus-request split, and the same Origin check sign-out uses for state-changing requests. Admin refusals use the existing sign-in page template, so they look like every other sign-in page. The cache admin package is a new, hosted-only module boundary. It holds one service (job state under a mutex, one job at a time) and one handler with two routes. It renders with `html/template` in the same small, light/dark style, with no JavaScript. The page refreshes itself with `<meta http-equiv="refresh">` only while a job runs, and the "Clear everything" button sits in its own form, apart from the set form. The service depends on the card database and image cache through two narrow interfaces, so its tests use fakes and the hosted tests use the real pieces.

**Patterns followed, not introduced.** These come from the upstream plans and the codebase: env-only secrets read in hosted wiring and summarised in the start-up log; `t.Setenv`, `captureLog` and `hostedConfig` in hosted tests; the fake Google with its `Claims` hook; package-internal card tests with the fake TCGdex, the fake clock and the background-settle helper; the shared `spacestest` fake, now with a CDN API fake beside it; S3 calls on the existing SigV4 signer with typed errors; and the Playwright route interception the card image plan introduced for image fields. No third-party modules are added, and `go.mod` and `package.json` are unchanged.

## Dependencies

- **Design documents this plan was built on: none.** The spec references no design documents.
- **Upstream plan `20261104090640-25f673ed-card-image-cdn` — must be implemented and merged before this plan starts.** It provides `spaces.Client.List` and `ListPage`, `Config.PublicBase`, the fake's listing and public reads, `cards.ImageCache` (copies keyed by path, queue, workers, breaker, settle helper), the `images/` layout and 7-day `Cache-Control`, `webui.ImageSource` and `image_fallback`, the board's image helper, and `hostedCardDB` returning the image cache. This plan extends `ListPage` with `ETags`, the copies with a version, `ImageSource.Image` with a third result, and the board helper with `?v=`. All of these are additive. It also adds `ImageCache.Clear`, the "forget this prefix" and purge that plan left to this spec.
- **Upstream plan `20261104082020-109dceb1-tcgdex-spaces-cache` — must land first (card-image-cdn's own prerequisite).** It provides `internal/spaces` (`Config`, `FromEnv`, `New`, `Get`, `Put`, the SigV4 signer, typed errors), `spacestest`, `NewSpacesStore` under `tcgdex/`, `NewTieredStore` with `Local()` and the breaker, the four `TCGREPLAY_SPACES_*` settings, `hostedCardDB`, and the bucket and key the operator created. This plan adds `Delete` and the CDN purge client on the same package, and `Clear` on the two stores it introduced.
- **Upstream plan `20261104090640-51bd1f36-card-cache-freshness` — must land first (the shared cache plan's own prerequisite).** It provides `Store`/`Entry`/`ErrNotCached`, the disk store, `Client` with `mem` entries carrying fetch times, the immutable catalog, the refresh tracker, single-flight, the miss refresh used by set naming, `Game.card` reading only `Cached`, and the fake TCGdex, fake clock and settle helper in the card tests. This plan adds the cleared mark, the clear generation and `Client.Clear`/`SetID`, and changes none of those shapes.
- **DigitalOcean Spaces (external service), through its S3-compatible API.** Clears use ListObjectsV2 (with ETags) and DeleteObjects. This needs the existing access key to be allowed to list and delete. No real bucket is available to automated tests, which use the fake.
- **DigitalOcean API v2, CDN endpoints (external service).** It lists endpoints and purges cache paths. It needs a DigitalOcean API token with CDN read and update access, created by an operator (human task). Without it, clears skip the purge and say so. Automated tests use the fake.
- **Google sign-in (existing, through `hostauth`).** The ID token's `email_verified` claim is now read. No change to the OAuth client or its redirect URI.
- **DigitalOcean App Platform (existing deployment).** It supplies two new environment variables, `TCGREPLAY_ADMINS` (plain) and `TCGREPLAY_SPACES_CDN_TOKEN` (encrypted). Instance count, size and the lack of a volume are unchanged.
- **TCGdex REST API v2 and assets (external services).** They remain the only card and image source. After a clear they are asked again for what was cleared. No change.
- **Go standard library only.** It provides `net/http`, `crypto/md5` (Content-MD5 and copy versions), `crypto/sha256`, `encoding/xml`, `encoding/json`, `html/template`, `sync` and `time`, with `net/http/httptest` in tests. `go.mod` is unchanged.
- **`internal/spaces`, `internal/spaces/spacestest` (changed).** Delete, ETags in listings, the CDN purge client, and their fakes.
- **`internal/cards` (changed).** Scope, store `Clear`s, `Client.Clear` and `SetID`, the cleared memory rule, and `ImageCache` versions and `Clear`. The `Store` and `DB` interfaces are unchanged.
- **`internal/hostauth`, `internal/hostauth/hostauthtest` (changed).** The admin list, `EmailVerified`, `RequireAdmin`, and the sealed verified flag. The fake already sends `email_verified`.
- **`internal/cacheadmin` (new).** The admin page and the clear job.
- **`internal/webui` and `web/` (changed).** `image_version` on `/api/card`, and the `?v=` in the board image helper, with one Playwright case. `package.json` is unchanged.
- **`internal/game` (unchanged code).** A new test proves an open game survives a clear.
- **`cmd/tcgreplay` (changed).** Hosted wiring of the admin list, the CDN purge client and the admin page.
- **`.do/app.yaml`, `Dockerfile`, `Makefile`, `TCG Review Playmat Design and Build Plan.md` (changed, docs and config only).** The Docker build context is not changed, so `internal/guides` stays in it.
- **Knowledge entries relied on:** `gotchas/game-listeners-under-lock.md`, honoured because clears never touch a game and `Cached` stays local and non-blocking; `gotchas/board-card-info-per-page-load.md`, which makes "the board shows it" mean the next page load and is updated with versioned image addresses; `gotchas/tcgdex-cache-never-expires.md` (as rewritten upstream), whose "delete the `tcgdex/` objects by hand until admin cache clearing exists" note this plan replaces; and `conventions/skills-text-and-packaging.md` (packaging half).

## Testing Approach

**Kinds of tests.** There are five layers of Go tests, all run under the race detector like the upstream plans' tests, plus one browser case:

- **Spaces client and fake unit tests.** Batch delete against the fake: keys gone, other keys kept, more than 1,000 keys split into several requests, a `Content-MD5` the fake checks, missing keys not an error, refused credentials and per-key refusals reported. Listings carry each object's ETag (MD5 of its bytes). The CDN purge client against the fake DigitalOcean API: the endpoint is found by origin, the purge carries exactly the given paths and the bearer token, a bucket with no endpoint gives "no CDN", and a refused token or a failing API is reported.
- **Card store and client tests in the card package.** These are package-internal and drive a real client with the tiered store over a disk store and the fake Spaces, against the freshness plan's fake TCGdex (request counts per path) with an injected clock and the settle helper. The scope rule gets a table test of its own, including near-miss set ids.
- **Image cache tests in the card package.** A real `ImageCache` runs against the fake Spaces, the fake DigitalOcean API and the card image plan's fake image host, awaited with the settle helper.
- **Sign-in and admin tests.** `hostauth` tests use the fake Google and its `Claims` hook. `cacheadmin` tests run the handler with fake card and image clearers, and once with the real pieces over the fakes.
- **Game-level and hosted wiring tests.** A game-level test opens a game through the real client and clears under it. Hosted tests start hosted mode with the fake Google, fake Spaces, fake TCGdex and fake DigitalOcean API, using the existing `hostedConfig`, `captureLog`, `t.Setenv` and board-cookie helpers.
- **One Playwright case**, added to the card image plan's image spec. It checks that a card answer with `image_version` makes the board request the CDN address with `?v=` and the fallback without it.

**Where coverage concentrates.** Most coverage goes to the clear itself (stores, client, image cache) and to the admin gate, because that is where the spec's behaviour and its risks lie. The load-bearing assertions, each tied to an acceptance criterion:

- **Admin clears everything.** After a clear-all, the fake Spaces holds no `tcgdex/` or `images/` objects, the cache directory holds no card files, and the next card lookup makes one request to the card source and returns its current data. A deck resolves again through a catalog rebuilt from the card source, and an import through the real client shows its cards, with no restart.
- **Admin clears one set by code / by id.** Clearing by code (`MEG`) and by id (`me01`, and `ME01`) removes the same objects: `sets/me01` and every `cards/me01-…` in both tiers, and that set's image copies. Afterwards a lookup of a card in that set makes one card-source request, while a lookup of a cached card in another set makes none. Near-miss sets (`me01` against `me01.5`, `sv03` against `sv03.5`) are untouched.
- **Unknown set refused.** An unknown code or id is answered with a message on the page (400), and the fake Spaces records no list or delete request and holds the same objects. The cache directory and memory are unchanged.
- **Cleared images refreshed.** After a set clear, `Image` for that set's copies answers the TCGdex address at once, with no version. The copies are deleted, and the purge records the set's `images/…/*` path. While the clear runs, no copy in scope is made. The next view copies the current bytes again. When the source's bytes changed, the new answer's version differs from the old one, so the board URL changes. When they are identical, it is the same. A restart (a new cache listing the same bucket) gives the same version as before the restart. Without a purge client the clear still completes and reports the CDN as not purged.
- **Non-admins refused.** Signed out, `GET /admin` redirects to Google sign-in and `POST /admin/clear` gets 401. Signed in as an account not on the list, or with `email_verified` false, or with a cookie sealed before the change, both get 403. A POST with a foreign or missing Origin gets 403, even from an admin. In every refused case the fake Spaces records no list or delete request and holds the same objects.
- **Open game survives a clear.** A game is built through the real client and the fakes, and everything is cleared. Board reads keep returning every card's definition (HP, text, image) without waiting on the network. A play on the game succeeds. Afterwards each card's next `Card` lookup fetches it from the card source, and once the background warm settles, `Cached` returns the fresh copy. With the card source down during the clear, the game still shows its cards and `Card` returns the old copies with no error.
- **Clears and lookups do not race.** A fetch or catalog build that began before a clear is not kept in memory afterwards, and a clear during a lookup storm leaves no old shared entry copied onto disk (tiered order). This runs under `-race`.
- **Job behaviour.** A second clear while one runs gets 409. The page shows running, then the per-tier counts, the purge result and any error. A Spaces failure mid-clear leaves memory and disk cleared and reports the shared tier's error. Clearing again then completes.

**Where tests follow existing conventions.** Card-package tests stay package-internal with the fake TCGdex, fake clock and settle helper. Spaces tests extend the shared fake, and the CDN fake lives beside it. `hostauth` tests follow `board_test.go`'s style with the fake Google. Hosted tests reuse `hostedConfig`, `captureLog`, the board cookie helper and the existing check that a clean start logs no "warning". The browser case reuses the card image plan's route interception. Every existing Go and Playwright test must pass unchanged, which guards desktop and dev builds and the board's existing image behaviour.

**Deliberate gaps.** No automated test reaches real DigitalOcean Spaces, its CDN or the DigitalOcean API, since no bucket, CDN or token is available. Their acceptance of DeleteObjects and our signature, the ETag format in real listings, the CDN's handling of the `?v=` query and of wildcard purges, and real latency are manual checks. Real Google sign-in is also a manual check. There is no browser test of the admin page, because it is server-rendered HTML with two forms, covered by handler tests.

**Success metrics.**
- "An admin can make a corrected card or set show on the board within a minute, without a redeploy": **behavioural test** for the playmat's part. A clear of a set against the fakes completes well inside a minute, and the next card lookup (the one the board's next page load makes) returns the card source's new data, with no restart. The same on the real deployment, timed from pressing "Clear" to a reloaded board showing the correction: **Manual — captured in the implementation test plan.**

**Manual reviews.**
- **Manual — captured in the implementation test plan.** Real admin sign-in: an account named in `TCGREPLAY_ADMINS` reaches `/admin` on the hosted playmat through Google sign-in. Another Google account gets the refusal page, and a signed-out direct POST is refused.
- **Manual — captured in the implementation test plan.** Real clear-all: after clearing everything, the bucket holds no `tcgdex/` or `images/` objects, the log shows the clear, and the next import shows its cards correctly with no restart.
- **Manual — captured in the implementation test plan.** Real set clear by code and by id: only that set's `tcgdex/` and `images/` objects are removed, and an unknown code is refused with the bucket unchanged.
- **Manual — captured in the implementation test plan.** Real CDN purge and image refresh: with the token set, a set clear logs a successful purge of `images/…/<set>/*`. After it, a browser that had shown the set's images before shows each image as TCGdex currently has it within 1 hour, loaded from the CDN with the new `?v=` once re-copied.
- **Manual — captured in the implementation test plan.** Real open game survives: with a game open on the hosted board, clear everything. The board keeps showing the game with card text and art, and playing on it works.
- **Manual — captured in the implementation test plan.** Real service details: Spaces accepts DeleteObjects with our signature and `Content-MD5`, listing ETags equal the MD5 of uploaded copies, and the CDN serves `…/low.webp?v=<version>`.

## Milestones & Tasks

### Milestone 1: The playmat can delete from the shared store and purge its CDN

**What changes**: Nothing a player or admin sees changes yet. This milestone is internal. The playmat's Spaces client learns to delete objects in bulk and to report each object's checksum in a listing, and a small client for the DigitalOcean API learns to purge paths from the bucket's CDN. The stand-ins used by tests learn both. It earns its own milestone because both later milestones depend on it, it needs no card knowledge, and landing it alone keeps their reviews focused on behaviour rather than wire formats.

**Validation point**: Against the stand-in Spaces, a bulk delete removes exactly the keys asked for, across more than one request when there are over 1,000, and reports refusals. Listings carry each object's checksum. Against the stand-in DigitalOcean API, a purge finds the bucket's CDN endpoint and sends exactly the requested paths with the token, and a missing endpoint or refused token is reported. The existing suites pass unchanged.

#### - [x] Task: Delete objects in bulk and report their checksums
**Id:** d28975cd-5c4c-4a5f-a7c5-9b0f728eea81
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Teach the shared Spaces client to delete many objects at once, up to 1,000 per request, as S3's batch delete allows, so a full cache clear takes a handful of requests rather than thousands. Listings also report each object's checksum (its ETag) beside its key, which the image versions later rely on. The stand-in Spaces learns both. This task needs the card image plan to have landed, since it extends that plan's listing.

*Technical detail:* [context.md#task-delete-objects-in-bulk-and-report-their-checksums](./context.md#task-delete-objects-in-bulk-and-report-their-checksums)

**Acceptance criteria**:
- [x] Deleting a list of keys removes exactly those objects from the stand-in and leaves every other object in place.
- [x] Deleting more than 1,000 keys is split into several requests, and every key is removed.
- [x] Deleting a key that does not exist is not an error.
- [x] Refused credentials, a server error, or keys the store refuses individually are reported, naming the refused keys.
- [x] A listing reports, for each key, the checksum of its bytes, and an object rewritten with different bytes is listed with a different checksum.
- [x] Existing listing callers see the same keys and paging as before.

#### - [x] Task: Purge the bucket CDN through the DigitalOcean API
**Id:** a1a61615-9c94-4ab3-a7a2-18aad9fbe17e
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Add a small client that tells DigitalOcean's CDN to drop its cached copies of given paths in the bucket, using a DigitalOcean API token. It finds the bucket's CDN endpoint by matching the bucket's address, so no endpoint id needs configuring. A stand-in DigitalOcean API, beside the stand-in Spaces, records each purge so tests can check what was purged. This task needs the shared card cache plan to have landed, since it lives in that plan's Spaces package.

*Technical detail:* [context.md#task-purge-the-bucket-cdn-through-the-digitalocean-api](./context.md#task-purge-the-bucket-cdn-through-the-digitalocean-api)

**Acceptance criteria**:
- [x] A purge sends exactly the requested paths, wildcards included, to the CDN endpoint whose origin is the bucket, with the token.
- [x] When the account has no CDN endpoint for the bucket, the purge reports that distinctly and purges nothing.
- [x] A refused token, or a failing or unreachable API, is reported as an error that never contains the token.
- [x] The stand-in records every purge's paths, and refuses requests without the right token.

### Milestone 2: The card database can be cleared, all of it or one set, without interrupting play

**What changes**: The card database can forget cached card data in every tier it holds: memory, the local disk and the shared store. It can forget everything, or one set named by its decklist code or its card source id. After a clear, every card, deck code and name lookup gets current data from the card source, while other sets stay cached. Games already open keep showing and playing their cards, which are refetched in the background. A set name that matches no set is refused, and nothing is cleared. Nothing calls this yet, so the running playmat behaves exactly as before.

**Validation point**: Tests with the stand-in card source and stand-in Spaces show a clear-all leaving no card data in either tier and the next lookup fetching fresh data. A set clear by code or by id removes only that set's data, and near-miss set ids are untouched. An unknown name is refused with nothing touched. An open game keeps its cards through a clear and picks up the fresh copies.

#### - [x] Task: Clear the card stores
**Id:** b96d1764-0345-478d-b6b2-c6e90ee34dcf
**Repo:** tcgreplay
**Depends on:**
- d28975cd-5c4c-4a5f-a7c5-9b0f728eea81 — Delete objects in bulk and report their checksums
**Execution:** agent

Define what a clear covers, everything or one set, once, and teach each card store to forget what it covers. The local disk store removes its files. The Spaces store lists and deletes its objects under the card data prefix. The tiered store clears the shared tier and then the local one, so a lookup during the clear cannot copy old shared data back onto disk. Each store reports how much it removed and any failure. The stores' existing read and write contract does not change.

*Technical detail:* [context.md#task-clear-the-card-stores](./context.md#task-clear-the-card-stores)

**Acceptance criteria**:
- [x] Clearing everything leaves no card data files in the cache directory and no objects under the card data prefix in the stand-in Spaces.
- [x] Clearing one set removes its card list and every one of its cards from disk and from the stand-in, and keeps the set list and every other set's data.
- [x] Sets whose ids share a beginning with the cleared set (for example `sv03` and `sv03.5`) are untouched.
- [x] Each store reports how many entries it removed. A shared-store failure is reported while the local tier is still cleared.
- [x] A clear reaches the shared store even when lookups are currently skipping it after a failure.
- [x] Stores that cannot clear, and every existing caller of the stores, keep working unchanged.

#### - [x] Task: Clear the card database and name sets
**Id:** e4d686b1-4a39-45bc-8f47-582936141ba6
**Repo:** tcgreplay
**Depends on:**
- b96d1764-0345-478d-b6b2-c6e90ee34dcf — Clear the card stores
**Execution:** agent

Give the card database a clear that reaches every tier it holds, and a way to name a set by its decklist code or its card source id. After a clear, card, deck code and name lookups fetch current data from the card source, and other sets stay cached. The cached-only lookup that games use under their lock keeps returning the old copy until the fresh one arrives in the background, so open games never lose their cards. Unknown names are refused before anything is touched.

*Technical detail:* [context.md#task-clear-the-card-database-and-name-sets](./context.md#task-clear-the-card-database-and-name-sets)

**Acceptance criteria**:
- [x] A set named by its decklist code (such as `MEG`), by its id (such as `me01`) or by its id in another case names the same set. A name that matches no set is refused, even after the set list has been refreshed.
- [x] After a clear of everything, the next card lookup makes one request to the card source and returns its current data, and deck codes resolve again through freshly fetched set data.
- [x] After a clear of one set, a lookup of a card in that set makes one request to the card source, while a lookup of a cached card in another set makes none.
- [x] After a clear, the cached-only lookup still returns a cleared card's previous copy without any network request, and returns the fresh copy once the background fetch completes.
- [x] If the card source is down after a clear, card lookups return the previous copy with no error.
- [x] A fetch or set-data build that started before a clear does not put pre-clear data back into memory.
- [x] A recent failed or missed lookup inside the cleared scope does not stop the next lookup from asking the card source.

#### - [x] Task: Prove an open game survives a clear
**Id:** 6933d1f1-b6f3-41ae-8a4f-80de777b8d07
**Repo:** tcgreplay
**Depends on:**
- e4d686b1-4a39-45bc-8f47-582936141ba6 — Clear the card database and name sets
**Execution:** agent

Add a game-level test that opens a game through the real card database, with the stand-in card source and stand-in Spaces, clears everything under it, and keeps playing. It proves the spec's "open game survives a clear" behaviour where games actually read their cards, and guards the rule that nothing under the game lock waits on the network.

*Technical detail:* [context.md#task-prove-an-open-game-survives-a-clear](./context.md#task-prove-an-open-game-survives-a-clear)

**Acceptance criteria**:
- [x] After everything is cleared, board reads of the open game still show every card's definition (name, HP, text, image), each read returning within a second.
- [x] A play on the game after the clear succeeds and behaves as before the clear.
- [x] Once background work settles, the game's cards have been fetched again from the card source.
- [x] With the card source down during and after the clear, the game still shows and plays its cards with no error.

### Milestone 3: Cleared card images come back as current art, past every cache

**What changes**: Card image copies in the shared store can now be cleared for one set or all sets, with the bucket's CDN purged for those paths when a DigitalOcean API token is configured. The board's CDN image addresses now carry a short content version, so whenever a copy's bytes change, viewers' browsers and the CDN fetch it again instead of showing an old copy they kept. Images whose bytes have not changed keep their address and their caches. Until a cleared image is copied again, the board shows it straight from the card source. Nothing calls the clear yet, but the versioned addresses are live on the hosted board as soon as this lands.

**Validation point**: Tests show a set's copies deleted while other sets' copies stay, the purge sent for the set's paths, no copy made in scope during the clear, and a re-copy of changed art getting a new version while identical art keeps its version, also across a restart. The board requests the CDN address with the version, and the card source fallback without it.

#### - [x] Task: Version image copies by their content
**Id:** 0d9a0015-f4e5-46f6-8a08-ccb29e4f2c9b
**Repo:** tcgreplay
**Depends on:**
- d28975cd-5c4c-4a5f-a7c5-9b0f728eea81 — Delete objects in bulk and report their checksums
**Execution:** agent

Make the image cache remember a short content version for every copy, taken from the checksums of its two image files. It gets the version from the bytes it uploads, and from the listing after a restart. Its answer for a copied image carries the version, so the board's address for a copy changes exactly when the copy's bytes change. Viewers' browsers and the CDN then fetch changed art instead of showing an old copy they kept, while unchanged art keeps its address and caches.

*Technical detail:* [context.md#task-version-image-copies-by-their-content](./context.md#task-version-image-copies-by-their-content)

**Acceptance criteria**:
- [x] A copied image is answered with its CDN address, its card source fallback, and a version. An image without a copy has no version.
- [x] Copying the same bytes again gives the same version. Copying different bytes gives a different one.
- [x] After a restart, an image found in the bucket gets the same version it had before the restart.
- [x] An image whose two files' checksums are not both known counts as not copied, as a half copy does.
- [x] Answers still never wait on the network.

#### - [x] Task: Send image versions to the board
**Id:** 2326484e-c15d-45ea-af26-2ae311e5a032
**Repo:** tcgreplay
**Depends on:**
- 0d9a0015-f4e5-46f6-8a08-ccb29e4f2c9b — Version image copies by their content
**Execution:** agent

Carry each copy's version from the image cache through the board's card endpoint, as one optional field, and have the board add it to the CDN address it loads. The card source fallback address is never changed. Without an image source, or for images without a copy, the card endpoint and the board behave exactly as before.

*Technical detail:* [context.md#task-send-image-versions-to-the-board](./context.md#task-send-image-versions-to-the-board)

**Acceptance criteria**:
- [x] With an image source that gives a version, the card endpoint's answer carries that version beside the image and fallback addresses.
- [x] Without an image source, or for an image without a version, the answer is the same as before this change.
- [x] The board requests the CDN address with the version for the card on the mat, the zoom and the dialog preview, and requests the fallback without it.
- [x] Every existing board and browser test passes unchanged.

#### - [x] Task: Clear image copies and purge the CDN
**Id:** 0bbbf9d5-59ea-4852-aa44-b3246d038beb
**Repo:** tcgreplay
**Depends on:**
- 0d9a0015-f4e5-46f6-8a08-ccb29e4f2c9b — Version image copies by their content
- a1a61615-9c94-4ab3-a7a2-18aad9fbe17e — Purge the bucket CDN through the DigitalOcean API
- b96d1764-0345-478d-b6b2-c6e90ee34dcf — Clear the card stores
**Execution:** agent

Give the image cache a clear for one set or everything. It stops pointing the board at copies in scope at once and holds off copying them while it works. It deletes their objects from the bucket, purges their paths from the CDN when a purge client is configured, and then lets copying resume, so the next view copies the card source's current art. It reports what it removed and whether the CDN was purged.

*Technical detail:* [context.md#task-clear-image-copies-and-purge-the-cdn](./context.md#task-clear-image-copies-and-purge-the-cdn)

**Acceptance criteria**:
- [x] Clearing one set deletes every copy of that set's images from the stand-in, and keeps every other set's copies.
- [x] Clearing everything deletes every image copy.
- [x] From the moment a clear starts, images in scope are answered with the card source's address. No copy in scope is made until the clear has finished, and the next view after it copies the current art again.
- [x] With a purge client, the stand-in DigitalOcean API records a purge of the cleared set's image paths, or of all image paths for a full clear. Without one, the clear completes and reports that the CDN was not purged.
- [x] A failed delete or purge is reported, and the copies in scope are still no longer pointed at.

### Milestone 4: Admins can clear the hosted playmat's card cache from a page

**What changes**: On the hosted playmat, accounts named in the new admin setting can open a small admin page after the usual Google sign-in. From it they can clear the whole card cache, or one set by its code or id, and watch the clear run and report what each cache removed. Anyone else, signed in or not, is refused on the page and on any direct request. Games carry on throughout. The deploy definition, container target and hosted docs describe the admin setting and the CDN token. An operator names the admins and creates the token, and the manual checks confirm the real deployment.

**Validation point**: Hosted tests show an admin reaching the page and clearing everything or one set, with the stand-in shared store and CDN showing the result. Non-admins, unverified emails, signed-out requests and cross-site posts are refused with nothing cleared. An open game survives a clear. After the operator's setup, the manual checks in the implementation test plan confirm real sign-in, real clears and purges, and the one-minute and one-hour outcomes.

#### - [x] Task: Name admins and guard the admin page
**Id:** 62859d76-6566-4410-9748-bc7f55360283
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Teach the hosted sign-in service which Google accounts are admins, and add a guard for admin pages beside the board's session guard. An account is an admin only when its email is on the configured list and Google confirmed that email. Signed-out visitors are sent through the usual Google sign-in, signed-out direct requests and non-admins are refused, and admin changes must come from the playmat's own pages. Board, connector and MCP sign-in behave exactly as before.

*Technical detail:* [context.md#task-name-admins-and-guard-the-admin-page](./context.md#task-name-admins-and-guard-the-admin-page)

**Acceptance criteria**:
- [x] An account whose confirmed email is on the admin list, in any letter case, passes the guard.
- [x] A signed-out request for an admin page is sent through Google sign-in and comes back to it. A signed-out direct or form request is refused as not signed in.
- [x] A signed-in account not on the list, one whose email Google did not confirm, or a sign-in from before this change is refused with a page saying so, and the request does not reach the admin page.
- [x] A request that changes something, sent from another site or with no origin, is refused even for an admin.
- [x] With no admins configured, everyone is refused.
- [x] Existing board sessions keep working for the board after this change.

#### - [x] Task: Add the cache admin page and clear job
**Id:** 931c0a49-f2e4-4fb9-bdf9-0edc5792fa7e
**Repo:** tcgreplay
**Depends on:**
- e4d686b1-4a39-45bc-8f47-582936141ba6 — Clear the card database and name sets
- 0bbbf9d5-59ea-4852-aa44-b3246d038beb — Clear image copies and purge the CDN
**Execution:** agent

Add the small admin page and the job behind it. The page offers "Clear everything" and "Clear one set" (by code or id), and shows the current or last clear: what it covered, who started it, what each cache removed, whether the CDN was purged, and any error. It refreshes itself while a clear runs. A clear request names its set first, so an unknown name is reported and nothing changes. One clear runs at a time, in the background, so a long clear is never cut off by a request time limit.

*Technical detail:* [context.md#task-add-the-cache-admin-page-and-clear-job](./context.md#task-add-the-cache-admin-page-and-clear-job)

**Acceptance criteria**:
- [x] The page shows the signed-in admin, both clear actions and the state of the last clear.
- [x] "Clear everything" clears the card database and, when present, the image copies, and the page then shows each tier's result.
- [x] "Clear one set" with a set code or a set id clears that set. An unknown or empty name is reported on the page, and nothing is cleared.
- [x] A clear asked for while another runs is refused with a message saying one is running.
- [x] A clear that fails part-way shows which tier failed and why. Clearing again is allowed.
- [x] Every clear is logged when it starts and when it ends, with who started it and its scope.

#### - [x] Task: Serve the admin page in hosted mode
**Id:** 418ea4cb-bc17-45a7-9e94-8891acc2432c
**Repo:** tcgreplay
**Depends on:**
- 62859d76-6566-4410-9748-bc7f55360283 — Name admins and guard the admin page
- 931c0a49-f2e4-4fb9-bdf9-0edc5792fa7e — Add the cache admin page and clear job
**Execution:** agent

Wire it all together in hosted start-up only. It reads the admin list and the CDN token from the environment, builds the CDN purge client when the token and the shared store are both present, and gives the image cache and the card database to the admin page. It then serves the page behind the admin guard. The start-up log says how many admins are named and whether CDN purging is on. Desktop, daemon and dev builds are untouched.

*Technical detail:* [context.md#task-serve-the-admin-page-in-hosted-mode](./context.md#task-serve-the-admin-page-in-hosted-mode)

**Acceptance criteria**:
- [x] An account named in the admin setting can sign in, open the admin page and clear everything. Afterwards the stand-in Spaces holds no card data or image objects, and the next import shows its cards.
- [x] Clearing one set by code, and by id, removes only that set's data and images, and the stand-in DigitalOcean API records the purge when the token is set.
- [x] An account not in the setting, and a signed-out browser, are refused on the page and on a direct clear request, and the stand-in Spaces still holds the same objects.
- [x] With a game open on the board, clearing everything leaves the game showing and playable.
- [x] Without the shared store, the admin page still clears memory and disk. Without the token, clears say the CDN was not purged. A token without the shared store is logged as ignored.
- [x] The start-up log names the number of admins (never their emails or the token), and a clean start logs no warning.
- [x] Desktop and dev builds serve no admin page, whatever is in their environment.

#### - [x] Task: Declare and document the admin and CDN settings
**Id:** 5cbe1f40-8024-4f6b-96ba-ca14b02c686c
**Repo:** tcgreplay
**Depends on:**
- 418ea4cb-bc17-45a7-9e94-8891acc2432c — Serve the admin page in hosted mode
**Execution:** agent

Make the two new settings discoverable where an operator looks. The App Platform definition declares the admin list and the CDN token (encrypted), the container run target passes them through, and the image comment lists them. The hosted design document gains the admin page, the settings, and the steps for creating a DigitalOcean API token with CDN access.

*Technical detail:* [context.md#task-declare-and-document-the-admin-and-cdn-settings](./context.md#task-declare-and-document-the-admin-and-cdn-settings)

**Acceptance criteria**:
- [x] The App Platform definition declares the admin list and, as an encrypted secret, the CDN token, each explained in a comment.
- [x] Running the container locally passes both settings through when they are exported.
- [x] The hosted settings table and operator setup describe the admin page, each setting and its default, and how to create the token with CDN read and update access.
- [x] No document still says hosted card data can only be refreshed by deleting objects by hand. The build context is unchanged, so the guides are still built in.

#### - [x] Task: Update the card cache knowledge entries
**Id:** a55ed943-0356-44b5-aa9c-483c222ec1fe
**Repo:** tcgreplay
**Depends on:**
- 418ea4cb-bc17-45a7-9e94-8891acc2432c — Serve the admin page in hosted mode
- 2326484e-c15d-45ea-af26-2ae311e5a032 — Send image versions to the board
**Execution:** agent

Update the card cache and board card info knowledge entries through the knowledge workflow, so the next reader knows how to force fresh hosted data and why image addresses carry a version. They describe the admin clear (what it covers, set naming, open games keeping their cards) and the versioned image addresses with the CDN purge. They also note that an open board still needs a reload to show cleared data.

*Technical detail:* [context.md#task-update-the-card-cache-knowledge-entries](./context.md#task-update-the-card-cache-knowledge-entries)

**Acceptance criteria**:
- [x] A reader of the card cache entry learns that admins clear the hosted cache from the admin page, everything or one set by code or id, across memory, disk, the shared store and image copies, and that this replaces deleting objects by hand.
- [x] The entry explains that games already open keep their cards through a clear, and that cleared data is fetched again from the card source.
- [x] A reader of the board card info entry learns that copied images carry a content version in their address, so changed art reaches browsers despite the long cache lifetime, and that an open board shows cleared data after a reload.

#### - [ ] Task: Name the admins and create the CDN purge token
**Id:** a533b58b-cd2a-483c-8038-e8a3291c8a3d
**Repo:** tcgreplay
**Depends on:**
- 5cbe1f40-8024-4f6b-96ba-ca14b02c686c — Declare and document the admin and CDN settings
**Execution:** human — needs the DigitalOcean control panel to create an API token and to set production environment variables and encrypted secrets in App Platform

Create a DigitalOcean API token limited to reading and updating CDN endpoints, and make sure the Spaces access key can list and delete objects in the bucket. Then set the admin list and the token (encrypted) on the hosted playmat's App Platform component, and redeploy. Until this is done, nobody can open the admin page, and clears would not purge the CDN.

*Technical detail:* [context.md#task-name-the-admins-and-create-the-cdn-purge-token](./context.md#task-name-the-admins-and-create-the-cdn-purge-token)

**Acceptance criteria**:
- [ ] The hosted playmat's App Platform component has the admin list set to the admins' Google account emails, and the CDN token set as an encrypted value.
- [ ] After a redeploy, the start-up log reports the expected number of admins and CDN purging on, with no warning.
- [ ] The Spaces access key in use can list and delete objects in the bucket.

## Open Questions

- **Do the upstream plans' implementations leave the seams this plan assumes?** This plan expects five things: the freshness plan's in-memory card entries, catalog pointer, refresh tracker and load or refresh helpers, so that a cleared mark and a generation fence can be added where results are applied; the tiered store, Spaces store and disk store as separate types that can each gain an optional `Clear`; `ImageCache` with `known` keyed by relative path, a worker loop and a start-up listing; `List` decoding `<Contents>` so ETags can be read; and `hostedCardDB` returning the card database and image cache. This depends on how those plans' tasks were actually implemented, which only shows once they land. *What to do:* adapt names to what landed, keeping the behaviour this plan specifies. If clearing would need the `Store`/`Entry`/`ErrNotCached` contract to change, or `Cached` to do I/O, STOP and ask the user.
- **Does real DigitalOcean Spaces accept DeleteObjects as signed here, and report MD5 ETags for the copies?** The batch delete needs Spaces to accept our SigV4 signature with `Content-MD5`, and the image versions assume a single-part upload's listed ETag equals the MD5 of its bytes. Only the real service shows this, during the manual checks. *What to do:* if DeleteObjects is refused for a reason other than permissions, STOP and ask whether to fall back to one DeleteObject per key, which is slower but simpler. If listed ETags are not MD5, nothing goes stale (versions still change whenever the bytes change), but every restart would change every image URL once. STOP and ask whether to accept that or to version by a HEAD of each object after upload.
- **Does the Spaces CDN serve `?v=` addresses and accept wildcard purges for this bucket?** The plan assumes the CDN passes or ignores the `v` query when fetching from the bucket, that the bucket's CDN endpoint lists the origin `<bucket>.<region>.digitaloceanspaces.com`, and that `images/…/*` purges are accepted. Only the real CDN and API show this. *What to do:* if `?v=` addresses fail on the CDN, STOP and ask the user, because the browser-cache refresh depends on it. If the endpoint's origin differs, STOP and ask whether to add a `TCGREPLAY_SPACES_CDN_ID` setting instead of discovery.

No other questions are left for implementation time. Every other decision is settled in the sections above or recorded as a drafting assumption.

## Out of Scope

- **Clearing a single card on its own** (spec Non-Goal). An admin clears the card's whole set instead.
- **Any other admin capability** (spec Non-Goal): managing users, games or settings. The admin page offers only the two clear actions and their status, and adds no MCP tool.
- **Pushing cleared data to boards already open.** An open board shows the fresh card data and art on its next page load (knowledge `gotchas/board-card-info-per-page-load.md`). Changing the board's update protocol is not part of this work.
- **Admin clearing on desktop, daemon or dev builds.** They have no sign-in. Their cache is cleared, as today, by deleting the cache directory.
- **Clearing the card source's own caches.** Images loaded from `assets.tcgdex.net` before a copy exists follow TCGdex's caching, which the playmat cannot purge.
- **Clearing borrowed Energy art with the set that borrows it.** Borrowed Crown Zenith art is cleared with `swsh12.5`.
- **Letting admins bypass the user cap, or a separate admin sign-in.** Admins use the existing Google sign-in and count as users.
- **Scheduled or automatic clears, a clear history beyond the last job, or clear audit storage.** The page shows the current or last clear, the log records every clear, and job state is in memory only.
- **Re-warming the cache after a clear.** Cleared data is fetched again on demand by the next lookup or view, and nothing is preloaded.
- **Changing the image object layout, the 7-day `Cache-Control`, the breaker for lookups, or the shared-store settings** fixed by the upstream plans. This plan only extends them additively: listing ETags, copy versions, the board's `?v=`, and the one new `TCGREPLAY_SPACES_CDN_TOKEN` setting.
- **A configurable CDN endpoint id, custom CDN domains, or other CDN providers.** The endpoint is found by the bucket's origin on DigitalOcean.

## Changelog

### 2026-11-04 — Task: Delete objects in bulk and report their checksums

**What was done**: The Spaces client gained `Delete(ctx, keys)`, which removes keys 1,000 at a time with S3 DeleteObjects (signed POST `?delete=` with `Content-MD5`), and `ListPage` gained an additive `ETags` map (unquoted, the hex MD5 of single-part uploads). The `spacestest` fake answers batch deletes, lists ETags, and has new knobs `DenyDelete`, `OmitETag`, `CorruptMD5` and readers `Keys(prefix)` and `ETag(key)`.

**Deviations**: Implementation started before its dependencies were implemented, as the orchestrator approved: `20261104082020-109dceb1-tcgdex-spaces-cache` was in progress (9/10 tasks complete) and `20261104090640-25f673ed-card-image-cdn` in progress (8/9 tasks complete); in both only the human operator task was open, and all their code was merged. `*DeleteError` also carries `Err` (with `Unwrap`), and a whole-request failure (refused credentials, server error, bad digest, transport) is reported as a `*DeleteError` naming every key not deleted with the request's code ("RequestFailed" for transport errors) rather than as a bare `*Error`; `errors.As(err, *spaces.Error)` still finds the S3 error. The goal check asked for this on its first attempt. Tests are in a new `internal/spaces/delete_test.go` rather than `client_test.go`.

**Files changed**:
- `internal/spaces/client.go`
- `internal/spaces/spacestest/spacestest.go`
- `internal/spaces/delete_test.go`

**Discoveries**: The signer already signs `content-md5`, and `canonicalQuery("delete=")` gives the `delete=` S3 expects, so no signer change was needed. Delete requests are counted in the fake as method `POST`, key `?delete`.

### 2026-11-04 — Task: Purge the bucket CDN through the DigitalOcean API

**What was done**: Added `spaces.CDN` (`NewCDN(cfg, token)`, `Purge(ctx, paths)`), which finds the bucket's CDN endpoint by matching its origin (`<bucket>.<endpoint host>`, case-insensitive) across the paged `GET /v2/cdn/endpoints`, then sends `DELETE /v2/cdn/endpoints/{id}/cache` with `{"files": [...]}`, 50 paths a request. `ErrNoCDN` reports a missing endpoint; `*CDNError` carries DigitalOcean's status, id and message; the token is scrubbed from every error. Added `spacestest.CDN` (`NewCDN(t, origin, token)`, `Purges`, `SetFail`, `SetOrigin`, endpoint id `CDNEndpointID` = `cdn-1`, plus a decoy endpoint).

**Deviations**: Implementation started before its dependencies were implemented, as the orchestrator approved: `20261104082020-109dceb1-tcgdex-spaces-cache` was in progress (9/10 tasks complete) and `20261104090640-25f673ed-card-image-cdn` in progress (8/9 tasks complete); only their human operator tasks were open. The fake answers a wrong token with 401 (as the context file says), where plan.md's interface note said 403.

**Files changed**:
- `internal/spaces/cdn.go`
- `internal/spaces/spacestest/cdn.go`
- `internal/spaces/cdn_test.go`

**Discoveries**: None.

### 2026-11-04 — Task: Clear the card stores

**What was done**: Added `cards.Scope` (everything, or one TCGdex set id: `sets/S` plus `cards/S-…`, images whose third path segment is S), `TierReport`, the optional `clearer` interface and `clearTimeout` (30 s) in `internal/cards/scope.go`. The disk store clears the files it owns (`sets.json`, `sets/`, `cards/`, the retired `set-index-v2.json`), the Spaces store lists and batch-deletes under `tcgdex/` (or the set's two prefixes), and the tiered store clears shared then local, bypassing the breaker, lifting it on success and never tripping it on failure.

**Deviations**: Implementation started before its dependencies were implemented, as the orchestrator approved: `20261104082020-109dceb1-tcgdex-spaces-cache` was in progress (9/10 tasks complete) and `20261104090640-25f673ed-card-image-cdn` in progress (8/9 tasks complete); only their human operator tasks were open. The Spaces store lists `tcgdex/sets/<id>.json` as a prefix instead of always sending it to the delete, so its removed count stays exact. `Scope` also has `String()` ("everything" / "set <id>") for logs.

**Files changed**:
- `internal/cards/scope.go`
- `internal/cards/scope_test.go`
- `internal/cards/store.go`
- `internal/cards/spacesstore.go`
- `internal/cards/tiered.go`
- `internal/cards/clear_store_test.go`

**Discoveries**: One full `go test ./...` run timed out in `cmd/tcgreplay` after 600 s with only idle webui hub goroutines left; two reruns passed (the package in 1.9 s). Treated as a pre-existing flake, unrelated to this task.

### 2026-11-04 — Task: Clear the card database and name sets

**What was done**: `cards.Client` gained `SetID` (TCGdex id in any case, or a decklist code through `codeAliases`, `setAliases` and the catalog code index, with one set-list refresh per name per `RetryAfter`; `ErrUnknownSet` otherwise) and `Clear` (store clear through the optional `clearer`, then under `c.mu` with no I/O: in-scope memory entries marked `cleared`, catalog dropped, in-scope tracker throttles and all miss throttles forgotten, clear generation `gen` moved on). `Card` treats a cleared entry as a miss and falls back to the old copy if the load fails or the client is offline; `Cached` returns it and warms it in the background. Loads remember the generation they began under: `remember`, `publish` and `update` keep nothing begun before a clear, and single-flight keys carry the generation so a lookup after a clear never joins a fetch begun before it.

**Deviations**: Implementation started before its dependencies were implemented, as the orchestrator approved: `20261104082020-109dceb1-tcgdex-spaces-cache` was in progress (9/10 tasks complete) and `20261104090640-25f673ed-card-image-cdn` in progress (8/9 tasks complete); only their human operator tasks were open. The codebase research for this High-complexity task was done in the main context (the code was already read during the drift check) rather than by parallel sub-agents; the tests were written by a sub-agent as required. Set codes are normalised by a new shared `normCode` helper used by `Resolve` and `SetID`. Card loading was factored into `loadCard`, and the stale-refresh apply into `refreshCard`/`warmCleared`.

**Files changed**:
- `internal/cards/cards.go`
- `internal/cards/fresh.go`
- `internal/cards/catalog.go`
- `internal/cards/clear.go`
- `internal/cards/clear_test.go`

**Discoveries**: A source fetch already in flight when a clear runs still writes its result to the store (disk or shared) after the clear; only memory and the catalog are fenced. That data was fetched from the card source, so it is current, and the plan leaves store puts unaffected.

### 2026-11-04 — Task: Prove an open game survives a clear

**What was done**: Added `internal/game/clear_test.go`: a game is opened through the real card database (tiered store over a temp directory and `spacestest`, the existing `liveSource` card source), everything is cleared under it, and the test checks that every card's definition (name, HP, text, image) is still shown with each read under a second, that a play (placing a Basic as Active) succeeds, and that the board's reads warm every cleared card back from the card source exactly once, bringing a correction made at the source. A second case closes the card source before the clear: the game still shows and plays its cards, and `Card` returns the old copies with no error.

**Deviations**: Implementation started before its dependencies were implemented, as the orchestrator approved: `20261104082020-109dceb1-tcgdex-spaces-cache` was in progress (9/10 tasks complete) and `20261104090640-25f673ed-card-image-cdn` in progress (8/9 tasks complete); only their human operator tasks were open. The game is built with `Manager.New` from decklists rather than an import; the play is `PlaceActive` during setup.

**Files changed**:
- `internal/game/clear_test.go`

**Discoveries**: None.

### 2026-11-04 — Task: Version image copies by their content

**What was done**: `ImageCache` keeps a content version per copy (`known` is now path → version): the first 12 hex characters of SHA-256 over low.webp's ETag, a newline and high.webp's ETag. The copier takes each ETag as the MD5 of the bytes it puts, and the start-up listing (and the per-image listing used when the start-up listing failed) reads them from `ListPage.ETags`. `Image` returns `(image, fallback, version)`; only a copy has a version.

**Deviations**: Implementation started before its dependencies were implemented, as the orchestrator approved: `20261104082020-109dceb1-tcgdex-spaces-cache` was in progress (9/10 tasks complete) and `20261104090640-25f673ed-card-image-cdn` in progress (8/9 tasks complete); only their human operator tasks were open. Adapted to card-image-cdn's never-overwrite rule: `partial` is now path → file → ETag, `present` returns ETags, a half copy is versioned from the listed ETag of the file already there plus the MD5 of the file put, and an image with a file whose ETag is unknown stays uncopied (served from the source) rather than being written over. To keep the build green, `webui.ImageSource.Image` gained its third result here (the server discards it until the next task) and the webui test stub was updated. New tests are in `internal/cards/images_version_test.go`.

**Files changed**:
- `internal/cards/images.go`
- `internal/cards/images_test.go`
- `internal/cards/images_version_test.go`
- `internal/webui/server.go`
- `internal/webui/webui_test.go`

**Discoveries**: `internal/webui` `TestWebsocketLoadAndAct` failed once under `-race` with "websocket: message too big: read limited at 32769 bytes" and passed on every rerun: a pre-existing flake.

### 2026-11-04 — Task: Send image versions to the board

**What was done**: `/api/card` carries an optional `image_version` (`CardInfo.ImageVersion`) when the image source answers a copy with a version; a version is dropped whenever the answer is the source address itself. The board's `imageSrc` helper appends `?v=<image_version>` to the primary (CDN) address only, never to the TCGdex fallback, so the card on the mat, the zoom and the dialog preview all request versioned copies. New Go test `TestCardImageVersion` and two Playwright cases in `web/e2e/images.spec.js` (versioned requests for the mat, zoom and preview; a failed versioned copy falls back without `?v=`).

**Deviations**: Implementation started before its dependencies were implemented, as the orchestrator approved: `20261104082020-109dceb1-tcgdex-spaces-cache` was in progress (9/10 tasks complete) and `20261104090640-25f673ed-card-image-cdn` in progress (8/9 tasks complete); only their human operator tasks were open. The goal check's first attempt judged "existing tests pass unchanged" not met because the first version added assertions inside the existing `TestCardImageSource`; they were moved into a new `TestCardImageVersion` with its own stub, so the only change to existing test code is the `stubImages.Image` signature the interface change forces. `serveImages` in the Playwright spec gained an optional `version` argument (inert when unset).

**Files changed**:
- `internal/webui/server.go`
- `internal/webui/webui_test.go`
- `web/src/lib/store.svelte.js`
- `web/e2e/images.spec.js`

**Discoveries**: The board build writes to `internal/webui/dist/app`, which `internal/webui/.gitignore` ignores, so a rebuild leaves the tree clean.

### 2026-11-04 — Task: Clear image copies and purge the CDN

**What was done**: `ImageCache` gained `CDN Purger` (satisfied by `*spaces.CDN`), `ErrCDNNotConfigured` and `Clear(ctx, Scope)`, which reports the "images" tier (objects deleted) and the "cdn" tier (paths purged, or why not). From its start, images in scope are answered from the source and neither queued nor copied; it waits for any in-scope copy already under way, forgets the scope's copies, lists `images/` and batch-deletes the objects in scope (every object under `images/` for a full clear) with `clearTimeout` bounds and no breaker, then purges `images/*` or `images/<lang>/<serie>/<set>/*` per directory found. Copying resumes when it ends, so the next view copies the current art at a new version.

**Deviations**: Implementation started before its dependencies were implemented, as the orchestrator approved: `20261104082020-109dceb1-tcgdex-spaces-cache` was in progress (9/10 tasks complete) and `20261104090640-25f673ed-card-image-cdn` in progress (8/9 tasks complete); only their human operator tasks were open. Instead of discarding a copy that finished during a clear, `Clear` waits for in-scope copies under way (a `copying` set) before listing, so no copy can land between the listing and the delete. To keep card-image-cdn's never-overwrite rule, a clear whose listing or delete fails marks its scope "unsure": before copying an image in it again the cache lists that image's own prefix rather than trusting memory, and never writes over objects the clear left behind (it may then point at those surviving objects again until a later clear succeeds, which lifts the mark). A set with no copies sends no purge.

**Files changed**:
- `internal/cards/images.go`
- `internal/cards/images_clear_test.go`

**Discoveries**: A clear that runs while the start-up listing is still in progress could, in principle, have the listing mark known a copy the clear then deletes; the board then falls back to TCGdex for that image until a restart. Clears are admin actions long after start-up, so this is left as is.

### 2026-11-04 — Task: Name admins and guard the admin page

**What was done**: `hostauth.Config` gained `Admins` (Google account emails, trimmed and lower-cased; blanks dropped), `Account` gained `EmailVerified`, read from the ID token's `email_verified` (both `true` and `"true"`) and sealed into the board cookie as `"v"` (omitted when false, so older cookies open unverified). New `admin.go`: `IsAdmin`, `AdminCount` (for the start-up log) and `RequireAdmin`, which sends a signed-out page request through Google sign-in, answers other signed-out requests 401, answers a non-admin, an unverified email or a pre-change sign-in with a 403 "Not an admin" page (telling an unverified admin to sign in again), and refuses a non-GET/HEAD request that fails the sign-out Origin check with 403.

**Deviations**: Implementation started before its dependencies were implemented, as the orchestrator approved: `20261104082020-109dceb1-tcgdex-spaces-cache` was in progress (9/10 tasks complete) and `20261104090640-25f673ed-card-image-cdn` in progress (8/9 tasks complete); only their human operator tasks were open. Two existing assertions in `google_test.go` compared a session's `Account` to one built without the new field (one by converting `hostauthtest.Account`); they now expect `EmailVerified: true`, which the fake Google sends. Connector tokens do not carry the flag (nothing reads it there). Added `AdminCount` for the hosted start-up log.

**Files changed**:
- `internal/hostauth/hostauth.go`
- `internal/hostauth/google.go`
- `internal/hostauth/seal.go`
- `internal/hostauth/store.go`
- `internal/hostauth/admin.go`
- `internal/hostauth/admin_test.go`
- `internal/hostauth/google_test.go`

**Discoveries**: None.

### 2026-11-04 — Task: Add the cache admin page and clear job

**What was done**: New package `internal/cacheadmin`: `Service` (narrow `Cards` and `Images` interfaces, one background job at a time on the server's lifetime from `Run`, bounded by `DefaultTimeout` of 10 minutes, `ErrBusy`, `Status`, a start and an end log line naming the admin and the scope), `Job` (`Label`, `Failed`), a server-rendered `html/template` page in the sign-in pages' style (no script; a 2-second meta refresh only while a clear runs; "Clear everything" and "Clear one set" forms; the current or last clear with each tier's line, including "CDN not purged (no TCGREPLAY_SPACES_CDN_TOKEN)"), and `Handler` serving `GET /admin` and `POST /admin/clear` (303 on start, 400 for an empty or unknown set, 409 while busy, 502 otherwise; all `no-store`).

**Deviations**: Implementation started before its dependencies were implemented, as the orchestrator approved: `20261104082020-109dceb1-tcgdex-spaces-cache` was in progress (9/10 tasks complete) and `20261104090640-25f673ed-card-image-cdn` in progress (8/9 tasks complete); only their human operator tasks were open. `Service` gained an optional `Logf` (tests capture the log lines). Written in one pass rather than by two parallel agents. The real-pieces test uses a small stand-in TCGdex of its own (the card package's fake is internal to that package).

**Files changed**:
- `internal/cacheadmin/cacheadmin.go`
- `internal/cacheadmin/page.go`
- `internal/cacheadmin/handler.go`
- `internal/cacheadmin/cacheadmin_test.go`

**Discoveries**: None.

### 2026-11-04 — Task: Serve the admin page in hosted mode

**What was done**: Hosted start-up reads `TCGREPLAY_ADMINS` into the sign-in service's admin list and `TCGREPLAY_SPACES_CDN_TOKEN` in `hostedCardDB`: with the shared store on, a token gives the image cache a `spaces.CDN` purge client (logged "CDN purge on clear: on", or "off (no TCGREPLAY_SPACES_CDN_TOKEN)"); without the shared store a token is logged as ignored (not a warning). New `hostedAdmin` builds the `cacheadmin.Service` over the card database and, when present, the image cache; `runHosted` runs it and `hostedHandler` mounts `/admin` and `/admin/` behind `RequireAdmin`. The start-up line ends with `adminText` ("N cache admins", never who). A test-only `config.cdnAPI` points the purge client at the stand-in API. New `cmd/tcgreplay/hosted_admin_test.go` covers clear-all with an open game and a fresh import, set clears by code and id with the purge, refusals of a non-admin and a signed-out browser with the bucket unchanged, the no-shared-store and no-token variants, the start-up count with no warning, and the desktop board serving no admin page.

**Deviations**: Implementation started before its dependencies were implemented, as the orchestrator approved: `20261104082020-109dceb1-tcgdex-spaces-cache` was in progress (9/10 tasks complete) and `20261104090640-25f673ed-card-image-cdn` in progress (8/9 tasks complete); only their human operator tasks were open. `hostedHandler` takes the admin service as a fourth argument, so the shared test helper `startHostedWith` in `hosted_test.go` passes `hostedAdmin(db, nil)`. The admin count comes from a new `hostauth.Auth.AdminCount` (deduplicated), and the hosted admin tests use their own `startHostedAdmin` harness over `hostedCardDB` rather than extending `startHostedWith`.

**Files changed**:
- `cmd/tcgreplay/hosted.go`
- `cmd/tcgreplay/main.go`
- `cmd/tcgreplay/hosted_test.go`
- `cmd/tcgreplay/hosted_admin_test.go`

**Discoveries**: None.

### 2026-11-04 — Task: Declare and document the admin and CDN settings

**What was done**: `.do/app.yaml` declares `TCGREPLAY_ADMINS` (run time, value set in the console) and `TCGREPLAY_SPACES_CDN_TOKEN` (run time, `type: SECRET`), each with a comment, and its header mentions `/admin`. The `Dockerfile` header lists both, and `make docker-run` passes both through. The design document's hosted section gains the versioned image addresses and a "Card cache admin page" paragraph; its settings table gains both rows with their defaults; operator steps 6 and 7 now point to the admin clear instead of deleting objects by hand, and a new step 8 says how to name admins, create a DigitalOcean API token with only CDN read and update scopes, and check the Spaces key can list and delete.

**Deviations**: Implementation started before its dependencies were implemented, as the orchestrator approved: `20261104082020-109dceb1-tcgdex-spaces-cache` was in progress (9/10 tasks complete) and `20261104090640-25f673ed-card-image-cdn` in progress (8/9 tasks complete); only their human operator tasks were open. The `ImageCache` doc comment in `internal/cards/images.go` still said copies are never deleted; it now says only an admin's clear deletes them. `.dockerignore` and the build context are untouched.

**Files changed**:
- `.do/app.yaml`
- `Dockerfile`
- `Makefile`
- `TCG Review Playmat Design and Build Plan.md`
- `internal/cards/images.go`

**Discoveries**: None.

### 2026-11-04 — Task: Update the card cache knowledge entries

**What was done**: Through `spek-knowledge` (repo tier, store `tcgreplay`), `gotchas/tcgdex-cache-never-expires.md` now says hosted card data is refreshed by an admin clear from `/admin` (everything, or one set by code or id; scope rule; every tier including the image copies and the CDN purge), that this replaces deleting objects by hand, that open games keep their cards (cleared entries are marked, `Cached` keeps answering and warms them, `Card`/`Resolve`/`ByName` refetch at once) and that image copies carry ETag-based versions. `gotchas/board-card-info-per-page-load.md` now explains `image_version` and the `?v=` CDN address that gets changed art past the week-long caches, and that an open board shows cleared data after a reload. Both gained the `admin` tag.

**Deviations**: Implementation started before its dependencies were implemented, as the orchestrator approved: `20261104082020-109dceb1-tcgdex-spaces-cache` was in progress (9/10 tasks complete) and `20261104090640-25f673ed-card-image-cdn` in progress (8/9 tasks complete); only their human operator tasks were open. The entries live in the project's knowledge store, not the worktree, so the goal check read copies exported with `spektacular knowledge read`.

**Files changed**:
- knowledge (repo tier, store `tcgreplay`): `gotchas/tcgdex-cache-never-expires.md`
- knowledge (repo tier, store `tcgreplay`): `gotchas/board-card-info-per-page-load.md`

**Discoveries**: None.

### 2026-11-04 — Task: Name the admins and create the CDN purge token

**What was done**: Nothing in code: this is a human operator task. It was sent through the goal check (three attempts, every criterion judged not met because the App Platform settings, the redeploy log and the Spaces key's permissions cannot be seen from here) and then skipped.

**Deviations**: Skipped, left unticked with all three acceptance criteria open: human task left for the operator (needs the DigitalOcean control panel and App Platform settings); the orchestrator reports it to the user. Still outstanding for the operator: set `TCGREPLAY_ADMINS` to the admins' Google account emails and `TCGREPLAY_SPACES_CDN_TOKEN` (encrypted) to a DigitalOcean API token with only CDN read and update scopes on the App Platform playmat component, confirm the Spaces access key can list and delete objects in the bucket, redeploy, and check the start-up log shows `<n> cache admins` and `card images: CDN purge on clear: on` with no warning. Implementation started before its dependencies were implemented, as the orchestrator approved: `20261104082020-109dceb1-tcgdex-spaces-cache` was in progress (9/10 tasks complete) and `20261104090640-25f673ed-card-image-cdn` in progress (8/9 tasks complete); only their human operator tasks were open.

**Files changed**:
- None.

**Discoveries**: A human task can only be skipped after its goal check is exhausted: `goal decide` refuses while the check is retrying, so it takes all three attempts.
