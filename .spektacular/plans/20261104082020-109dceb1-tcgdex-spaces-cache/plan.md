---
created_date: "2026-11-04"
document_status: final
closed_date: "2026-11-04"
---

# Plan: 20261104082020-109dceb1-tcgdex-spaces-cache

<!-- Metadata -->
<!-- Created: 2026-11-04T11:56:02Z -->
<!-- Commit: 8c26660570c61a3e063c43882de3fae7a6168aeb -->
<!-- Branch: main -->
<!-- Repository: git@github.com:nicholasjackson/tcgreplay.git -->

## Overview

The hosted playmat keeps its cached TCGdex card data (the set list, each set's card list and every card) in a DigitalOcean Spaces bucket as well as on local disk, so a deploy or restart no longer throws the cache away and the first import afterwards is as fast as any other. Spaces sits behind local disk as a second cache tier, entries keep the time they were first fetched so stale data stays stale across restarts, and any Spaces outage is logged and skipped so play never stops. Desktop and dev builds keep their local cache and need no new settings, and the small Spaces client added here is the one the later card-image and admin-clearing specs build on.

## Conventions

- **Guide text and packaging (`conventions/skills-text-and-packaging.md`)** — applies in its packaging half: this plan edits the `Dockerfile` and the hosted deploy files, and must not exclude `internal/guides` from the build context (the binary embeds the guides). The guide-wording half does not bear on this work, since no guide text changes.

## Architecture & Design Decisions

All the work lands in the one registered repo, `tcgreplay`: a new `internal/spaces` package, new files in `internal/cards`, the hosted wiring in `cmd/tcgreplay`, and the hosted deploy files and docs. It builds directly on the storage seam from the card cache freshness plan (`20261104090640-51bd1f36-card-cache-freshness`), which must land first. That plan routes every card-data read and write through `Client.Store`, a `Store` of raw TCGdex responses by key (`sets`, `sets/<id>`, `cards/<id>`), each an `Entry{Data, Fetched}`. It keeps all freshness rules above the store. This plan adds no freshness rules and changes neither the `Store` interface nor the `DB` interface. It puts a **tiered store** into that slot on the hosted instance: local disk first, then DigitalOcean Spaces. The memory cache above both stays as it is, and the card source (TCGdex) is still reached only by the client, when the store reports `ErrNotCached`. A read that misses on disk and hits Spaces copies the entry to disk with its `Fetched` time untouched, so an entry that was stale before a restart is still stale after it. A put (after a TCGdex fetch or background refresh) goes to disk and then to Spaces. Each Spaces object holds the same envelope bytes the disk store writes, `{"fetched": RFC 3339, "data": <body>}`, under `tcgdex/` at the disk store's relative path (`tcgdex/sets.json`, `tcgdex/sets/<id>.json`, `tcgdex/cards/<id>.json`). One encoder serves both tiers, and an object downloaded by hand is self-describing.

Two reads must never reach the network: `Cached`, which the game calls under its lock (project rule: nothing under the game lock calls the network), and every read in offline mode (spec: offline never contacts the shared store). Rather than widen the `Store` interface, the tiered store also offers `Local() Store`, and the client's store accessor uses that local tier, found by type assertion, for `Cached` and whenever `Offline` is set. A card that is only in Spaces is therefore `nil` from `Cached`. The game's existing background warm then loads it through `Card`, which reads Spaces and fills the disk. Hosted wiring also never builds the shared tier when `-offline` is given. Shared-store failures are absorbed inside the tiered store, never returned to a lookup. Each Spaces request is bounded at 3 seconds. A miss (404) is normal, while any other failure (unreachable, timeout, refused credentials, 5xx) is logged and trips a **breaker**: the shared tier is skipped for 1 minute, and the log records the outage once and the recovery once. A dead or misconfigured Spaces therefore costs at most one timeout per minute, and lookups fall through to disk and TCGdex exactly as without it.

The Spaces client is a small, general S3 client in `internal/spaces`. It signs requests itself with AWS Signature Version 4 on the standard library, uses path-style addressing (`<endpoint>/<bucket>/<key>`), and returns typed errors carrying S3's error code. For now it offers `Get` and `Put` (with content type, cache-control and public-read options). The later specs add delete, list and image copies on the same signer: `card-image-cdn` puts public image copies beside the card data, and `admin-cache-clearing` deletes by prefix. A companion `internal/spaces/spacestest` package provides an in-process fake of the S3 API. It counts requests, checks the signature with the same access key, and can fail, refuse credentials, hang or close. It is how every automated test exercises the shared tier, since no real bucket is available to the suite. The client knows nothing about cards; the card-specific adapter (key mapping, envelope) lives in `internal/cards`. Settings come only from the environment and only in hosted mode: `TCGREPLAY_SPACES_ENDPOINT`, `TCGREPLAY_SPACES_BUCKET`, `TCGREPLAY_SPACES_KEY` and `TCGREPLAY_SPACES_SECRET`, with the last two encrypted App Platform secrets. The signing region is the endpoint's first host label. With none set, hosted mode runs local-only and logs one plain line saying so. A partial or invalid set is logged as a warning and also runs local-only, because an optional cache must never stop play. Desktop, daemon and dev builds keep calling the unchanged `newCardDB` and never read these settings. When the shared store is on, hosted start-up warms the card catalog (set list and every set's card list) in the background, through a new `Client.Warm`. This is aimed at the success metric that the first import after a restart finishes within 2 seconds of a warm one.

This beats teaching `cards.Client` a second store field and a three-way branch. The freshness plan was shaped so that a tier drops into the store slot and inherits its policy with no changes. It also beats adopting `minio-go` or the AWS SDK, which add a dozen or more modules to a three-dependency module for two HTTP calls. And it beats copying the whole bucket to disk at start-up, which needs a listing and downloads cards no game uses. The evidence and the rejected options are in `research.md#alternatives-considered-and-rejected`. Packaging stays as it is: the Docker build context and `.dockerignore` are not changed, so `internal/guides` stays in the build (convention `conventions/skills-text-and-packaging.md`).

## Component Breakdown

- **Spaces client (new, general purpose).** It owns talking to DigitalOcean Spaces through its S3-compatible API: building path-style object URLs, signing each request with AWS Signature Version 4, and mapping responses to results. A missing object is a distinct "not found", and any other failure is a typed error carrying S3's status and error code (for example `AccessDenied`). It reads and writes whole objects by key, with options on a write for content type, cache-control and public-read access. It knows nothing about cards, envelopes or retries, so the later image and admin-clearing specs reuse it as is, and add delete and list on the same signer. It also owns parsing and validating its settings (endpoint, bucket, key, secret; region from the endpoint), and reports none, complete, or partial/invalid.
- **Fake Spaces (new, test support).** An in-process stand-in for the S3 API that tests in any package can start. It stores objects in memory and counts requests per method and key. It checks the access key and the signature, answering 403 like Spaces for wrong credentials, and can be switched to fail with 500s, hang, or close. Tests seed objects and read them back directly. It is the only way the automated suite exercises the shared tier, since no real bucket is available.
- **Spaces card store (new, inside the card database).** It is the card database's `Store` over the Spaces client. It maps each cache key to its object under the `tcgdex/` prefix, using the same relative path the disk store uses. It writes the same envelope the disk store writes (fetch time plus raw body) and reads it back, reporting `ErrNotCached` for a missing object or one that is not a valid envelope. It holds no expiry and no retry logic.
- **Tiered store (new, inside the card database).** It is a `Store` composed of a local store and a shared store. A read is served from local. On a local miss it is served from shared, and the entry is copied to local with its fetch time unchanged. A write goes to local and then to shared. It bounds each shared request at 3 seconds and logs shared failures. It owns the breaker that skips the shared tier for 1 minute after a failure, logging the outage once and the recovery once, so a lookup never fails because of the shared store. It exposes its local tier, so callers that must not reach the network can bypass the shared store.
- **Card database client (changed, small).** `cards.Client` keeps its `DB` interface, constructor and fields from the freshness plan. Two things change. Its store accessor returns the store's local tier, when it has one, for `Cached` and whenever the client is offline, so neither ever reaches the shared store. And it gains `Warm`, which builds the card catalog (set list and every set's card list) through the normal lookup path, for hosted start-up. The freshness policy, single-flight, refresh tracker and catalog are untouched, and they reach the shared tier only through the store they already use.
- **Hosted wiring (changed).** Hosted start-up reads the four shared-store settings from the environment. When they are complete and the playmat is online, it builds the card database with a tiered store (disk, then Spaces) and starts the catalog warm in the background. When none is set, it builds the card database as today and logs one plain line saying the cache is local only. When they are partial or invalid, it logs a warning naming the problem and runs local-only. In offline mode it logs that the shared store is ignored. The desktop, daemon and dev constructors are unchanged and never read these settings.
- **Hosted deployment and docs (changed).** The App Platform definition declares the four settings, with the key and secret as encrypted secrets, and its comments stop saying the card cache is lost on every deploy. The container run target passes them through, and the image comment lists them. The design document's hosted settings table and operator setup gain the shared store: creating the bucket and an access key, and setting the variables. The card cache knowledge entry describes the shared tier.
- **Unchanged consumers.** The game, importer, deck resolution, board card endpoint, MCP tools, `cardstest.FakeDB` and the board front end all keep calling the same `DB` methods. The game's background warm of a card missing from `Cached` is what pulls a Spaces-only card onto local disk during play.

## Data Structures & Interfaces

**Unchanged contracts this plan relies on.** `cards.Store`, `cards.Entry{Data []byte; Fetched time.Time}`, `cards.ErrNotCached`, the keys `sets`, `sets/<id>` and `cards/<id>` (raw ids), and the envelope `{"fetched": RFC 3339, "data": <body>}` all come from the card cache freshness plan, and this plan does not change their shape. The `cards.DB` interface and `cardstest.FakeDB` are unchanged too. No MCP tool schema, HTTP response or board protocol changes.

**`spaces.Config` and `spaces.Client` (new, `internal/spaces`).** These are the general S3 client and its settings. Both later specs (`card-image-cdn`, `admin-cache-clearing`) build on them.

```go
type Config struct {
    Endpoint  string       // e.g. https://ams3.digitaloceanspaces.com
    Region    string       // signing region; derived from Endpoint when empty
    Bucket    string
    AccessKey string
    SecretKey string
    HTTP      *http.Client // optional
}

// FromEnv reads TCGREPLAY_SPACES_{ENDPOINT,BUCKET,KEY,SECRET} via getenv.
// set=false when none is set; err names what is missing or invalid.
func FromEnv(getenv func(string) string) (cfg Config, set bool, err error)

func New(cfg Config) (*Client, error)

var ErrNotFound = errors.New("spaces: no such key")

type Error struct { Status int; Code, Message string } // S3 error response

type PutOptions struct {
    ContentType  string
    CacheControl string
    Public       bool // x-amz-acl: public-read (for later image copies)
}

func (c *Client) Get(ctx context.Context, key string) ([]byte, error) // ErrNotFound on 404
func (c *Client) Put(ctx context.Context, key string, body []byte, o PutOptions) error
```

**`spacestest.Server` (new, `internal/spaces/spacestest`).** This is the in-process fake of the S3 API that any package's tests can use. It is built on `httptest`. `Config()` returns a `spaces.Config` pointing at it. The rest of its surface, roughly:

```go
func New(t testing.TB) *Server            // closed at test cleanup
func (s *Server) Config() spaces.Config   // endpoint, bucket, matching keys
func (s *Server) Object(key string) ([]byte, bool)
func (s *Server) SetObject(key string, body []byte)
func (s *Server) Requests(method, key string) int // and Total()
// knobs: Fail (500), Deny (403 InvalidAccessKeyId), Hang (block until
// released), Close (unreachable)
```

**`cards.NewSpacesStore` and `cards.NewTieredStore` (new, exported from `cards`).** These are the shared-tier `Store` and the tier composition. Both satisfy the unchanged `Store` interface.

```go
func NewSpacesStore(c *spaces.Client) Store // objects under "tcgdex/"
func NewTieredStore(local, shared Store) Store

const (
    SharedTimeout    = 3 * time.Second // per shared-store request
    SharedRetryAfter = time.Minute     // shared tier skipped after a failure
)
```

The tiered store also has `Local() Store`. The client finds it with an unexported optional interface, `interface{ Local() Store }`, so the `Store` interface itself does not grow.

**Object layout in the bucket (serialization boundary).** Card data lives at `tcgdex/<disk relative path>`: `tcgdex/sets.json`, `tcgdex/sets/<safe id>.json` and `tcgdex/cards/<safe id>.json`. Each object holds the envelope bytes exactly as the disk store writes them (`Content-Type: application/json`, private). The `tcgdex/` prefix is reserved for card data. The later image spec uses its own prefix, and admin clearing selects card data, one set's card list or one set's cards by prefix.

**`cards.Client.Warm` (new method).** It builds the card catalog through the normal lookup path. It is used by hosted start-up and is not part of `DB`.

```go
func (c *Client) Warm(ctx context.Context) error
```

**Hosted settings (environment only, hosted mode only).**

| Variable | Meaning |
|---|---|
| `TCGREPLAY_SPACES_ENDPOINT` | Spaces endpoint URL, e.g. `https://ams3.digitaloceanspaces.com` |
| `TCGREPLAY_SPACES_BUCKET` | bucket name |
| `TCGREPLAY_SPACES_KEY` | access key id (encrypted secret) |
| `TCGREPLAY_SPACES_SECRET` | secret key (encrypted secret) |

None set means local cache only. All four valid means the shared store is on. Anything in between gives a warning and local only.

## Implementation Detail

**New module boundary: a general Spaces client.** `internal/spaces` is a new leaf package that depends only on the standard library and knows nothing about cards. It holds three small pieces: settings parsing, a Signature Version 4 signer, and the object calls. The signer is a pure function of the request, payload hash, credentials and time. That lets it be tested against AWS's published worked example, independently of any fake, and lets later specs add delete and list as one method each. Errors follow the card package's style: a sentinel for the normal miss (`ErrNotFound`), and a typed error, carrying the HTTP status and S3's error code, for everything else, so log lines say *why* (`InvalidAccessKeyId`, `SignatureDoesNotMatch`, `503`) without callers parsing XML. The client sets no timeouts or retries of its own beyond the HTTP client's outer bound. Bounding and failure policy belong to the caller, as they do for TCGdex in the card package.

**New test-support package, following `cardstest`.** `internal/spaces/spacestest` mirrors the existing `internal/cards/cardstest` pattern: an exported, importable test double, so `internal/cards`, `internal/game` and `cmd/tcgreplay` tests (and the later image and admin specs) share one fake rather than each writing an `httptest` handler. It verifies signatures with the real signer, so a signing bug that the fake would otherwise accept shows up as a 403 in every test that touches the shared tier.

**Composition, not branching, in the card package.** The shared tier arrives as two more `Store` implementations beside the disk store: one over Spaces, and one that composes a local and a shared store. Nothing above the store learns that tiers exist. The freshness policy, single-flight, refresh tracker and catalog are unchanged, apart from one rule in the client's store accessor. That rule is "use the local tier for `Cached` and offline", written once, with a comment saying why: `Cached` runs under the game lock, and offline must not touch the network. A developer reading a lookup still sees one path, memory → store → single-flight fetch. A developer reading the tiered store sees the whole tier story (copy-down with the fetch time kept, write-through, bounded shared calls, breaker, logging) in one short file. The Spaces store reuses the disk store's envelope encoder and key-to-path mapping, factored out where the freshness plan left them inline, so there is one definition of the bytes and the names in both tiers.

**Failure handling pattern (new, local to the tiered store).** It is a per-tier breaker, not per-key memory: any shared failure marks the shared tier down until a time 1 minute ahead, and reads and writes skip it until then. The first request after that time is the probe. Logging is edge-triggered: one line when the tier goes down (with the error) and one when a request succeeds again, so an outage produces two log lines rather than one per lookup. A put that is skipped while the tier is down is not queued. The entry reaches Spaces the next time it is fetched or refreshed, which is acceptable for a cache. The breaker's clock is injectable, following the freshness plan's `Now` pattern, so tests age it without sleeping.

**Hosted wiring stays in `cmd/tcgreplay`.** Hosted start-up gains one helper that builds the card database from the environment. It follows the existing hosted pattern: secrets come only from the environment, and settings are summarised in the start-up log. Unlike the Google credentials, which stop start-up, a bad cache setting warns and degrades, because play must not depend on the shared store. The desktop constructor is untouched, so desktop, daemon and dev builds cannot reach the new code. The catalog warm runs as a background goroutine tied to the server's context, and logs its duration, or the error if it fails. Nothing waits on it.

**Patterns followed, not introduced.** These are carried over unchanged: atomic local writes (through the disk store), env-only secrets with `t.Setenv` in tests, `httptest` fakes with per-path request counts, injected clocks, package-internal tests in `internal/cards`, and `captureLog`-style log assertions in `cmd/tcgreplay`. The only new module-level dependency is the new in-repo package itself. No third-party modules are added.

## Dependencies

- **Design documents this plan was built on: none.** The spec references no design documents.
- **Upstream plan `20261104090640-51bd1f36-card-cache-freshness` — must be implemented and merged before this plan starts.** It provides `Store`, `Entry`, `ErrNotCached`, the keys, the envelope, the disk store and its encoder, `Client.Store`, `Timeout` and `Now`, the single lookup path, the catalog with `ensureCatalog`, a `Cached` that reads only the store, offline mode that never fetches, and the `httptest` fake TCGdex plus fake clock in the card package's tests. This plan changes none of their shapes. It needs two small touches: the client's store accessor learns to use a local tier for `Cached` and offline, and the disk store's envelope encoder and key-to-path mapping are made reusable by the Spaces store.
- **Downstream specs `20261104090640-25f673ed-card-image-cdn` and `20261104090640-a5a74fab-admin-cache-clearing`.** Both reuse the Spaces client, its settings and the fake Spaces, adding image puts (public-read, content type) and delete/list. They rely on card data staying under the `tcgdex/` prefix with the key layout above.
- **DigitalOcean Spaces (external service), through its S3-compatible API.** It provides the shared store. It needs a bucket and an access key created by an operator. No real bucket is available to the automated tests, which use the fake; real-Spaces behaviour is covered by manual checks.
- **DigitalOcean App Platform (existing deployment).** It supplies the settings as environment variables, with the access key and secret as encrypted secrets. The app definition gains four variables; instance count, size and the lack of a volume are unchanged.
- **TCGdex REST API v2 (external service).** It remains the card source, used exactly as the freshness plan leaves it. This plan only reduces how often it is asked.
- **Go standard library only.** It provides `net/http`, `crypto/hmac`, `crypto/sha256`, `encoding/hex`, `encoding/xml` (S3 error bodies), `net/url`, `time` and `log`, with `net/http/httptest` in tests. No new third-party modules; `go.mod` is unchanged.
- **`internal/cards` (changed).** It gains the Spaces store, the tiered store and `Client.Warm`, plus the local-tier rule in the store accessor. Its `DB` interface is unchanged.
- **`internal/spaces` and `internal/spaces/spacestest` (new).** The general S3 client and its fake.
- **`cmd/tcgreplay` (changed).** Hosted start-up builds the card database from the environment. The desktop constructor and all desktop modes are unchanged.
- **`.do/app.yaml`, `Dockerfile`, `Makefile`, `TCG Review Playmat Design and Build Plan.md` (changed, docs and config only).** Settings declared and documented. The Docker build context is not changed, so `internal/guides` stays in it.
- **`internal/game`, `internal/importer`, `internal/deck`, `internal/webui`, `internal/mcpserver`, `web/`, `internal/cards/cardstest` (unchanged).** They keep calling `DB` methods. A new test in `internal/game` uses the real client with the fakes.
- **Knowledge entries relied on:** `gotchas/tcgdex-cache-never-expires.md`, which the freshness plan rewrites and this plan extends with the shared tier; `architecture/importer-determinism.md`, which says lookups only fill a card's definition and is guarded by the warm-restart test; `gotchas/game-listeners-under-lock.md`, which forbids outbound work under the game lock and is the reason `Cached` stays local; and `conventions/skills-text-and-packaging.md` (packaging half).

## Testing Approach

**Kinds of tests.** Most of the coverage is Go unit and integration tests, run under the race detector like the freshness plan's tests. There are four layers:

- **Signer and client unit tests** in the new Spaces package. The signer is checked against AWS's published Signature Version 4 worked example (the S3 "GET Object" example), byte for byte, so its correctness does not rest on our own fake agreeing with it. The client is tested against the fake: a round trip of bytes and headers, a 404 mapped to "not found", S3 error bodies mapped to typed errors with their code, and settings parsing (none, complete, each kind of partial, bad endpoint, region from the endpoint).
- **Store tests** in the card package. These cover the Spaces store over the fake (envelope round trip with an exact fetch time, object names under `tcgdex/`, missing and malformed objects reported as not cached) and the tiered store over a disk store and the fake (copy-down keeping the fetch time, write-through, every failure mode absorbed, breaker timing on an injected clock, edge-triggered logging).
- **Client-level acceptance tests** in the card package. These drive a real `cards.Client` with the tiered store against the freshness plan's fake TCGdex (request counts per path, injected clock) and the fake Spaces. They are where the spec's acceptance criteria are asserted.
- **One game-level test and hosted wiring tests.** The game-level test builds a game's cards twice through the real client, with an empty disk the second time, to prove the import result is identical and the card source idle. The hosted wiring tests set the environment variables against the fake and check which store the hosted card database gets and what the log says.

**Where coverage concentrates.** The tiered store and the client-level acceptance tests get the most attention, because that is where the spec's behaviour lives. They assert outcomes and request counts on both fakes, never internal state:

- **Warm restart.** A client fills the fake Spaces while resolving a deck's set codes and loading its cards. A second client on the same fake Spaces, with an empty cache directory, then resolves the same codes and cards. That makes zero card-source requests for the set list, the set card lists or the cards, and gives identical answers. At game level, building a game's cards from the same decklists gives an identical card list.
- **No shared store, no change.** A client built as today writes only under its cache directory. Hosted wiring with no settings logs no warning or error, and builds a plain disk-backed client.
- **Shared store fills local.** With an empty cache directory, a card held only in the fake Spaces is returned with zero card-source requests, and then exists in the cache directory. A card in neither is fetched once, and afterwards exists in both.
- **Fetch age kept / stale stays stale.** An entry seeded in the fake Spaces with an old fetch time is copied to disk with that exact time. A Spaces entry past its refresh age, read through a new client with an empty disk, is served stale. It is then refreshed from the card source once (the freshness plan's background refresh), and the refreshed entry, with its new fetch time, lands in both tiers.
- **Outage.** With the fake Spaces closed, hanging, failing with 500s, or refusing the credentials, lookups succeed from the card source with no error returned. Each mode costs at most one shared-store attempt per breaker window, a hanging store delays the first lookup by no more than the shared timeout, and the log records the failure once. Hosted wiring with wrong credentials still starts, and builds a working client.
- **Offline.** An offline client with the tiered store makes zero requests to the fake Spaces, even with an empty disk and the entry present in Spaces. Hosted wiring in offline mode never builds the shared tier.
- **Game lock stays local.** `Cached` for a card held only in the fake Spaces returns nothing and makes zero Spaces requests.
- **Warm.** `Client.Warm` loads the set list and every set's card list. Hosted start-up with the shared store on runs it in the background without delaying the health check.

**Where tests follow existing conventions.** Card-package tests stay package-internal and reuse the freshness plan's fake TCGdex, fake clock and seed helpers. The fake Spaces follows the `cardstest` pattern as an exported test package. Hosted tests use the existing `t.Setenv`, `captureLog` and `hostedConfig` helpers, and the existing check that a clean start logs no "warning". The importer, record round-trip and game tests must pass unchanged, which guards the importer-determinism rule.

**Deliberate gaps.** No automated test talks to real DigitalOcean Spaces, since no bucket or credentials are available, so behaviour that depends on the real service is left to manual checks: its 404-versus-403 answers for missing keys, its acceptance of our signatures and region, and its latency. There are no browser tests, because the board does not change. The real TCGdex is never called.

**Success metrics.**
- "After any redeploy other than the first, the first import makes zero requests to the card source for card data the shared store holds that is still fresh": **behavioural test**. The warm-restart tests (client level and game level) assert zero card-source requests for the set list, set card lists and cards, after a restart with an empty disk against a shared store that holds them fresh. The same thing on the real deployment is part of the manual real-Spaces check below.
- "After any restart other than the first deploy, the first import on the hosted playmat finishes within 2 seconds of a warm-cache import of the same game": **Manual — captured in the implementation test plan.** It depends on real Spaces latency from App Platform, and on the start-up catalog warm, which only a real deployment shows.

**Manual reviews.**
- **Manual — captured in the implementation test plan.** Real Spaces end to end: with a real bucket and key configured on the hosted playmat, import a game, redeploy, import it again. The card source gets no card-data requests, the bucket holds `tcgdex/` objects in the envelope format, and the start-up log names the shared store.
- **Manual — captured in the implementation test plan.** Real Spaces failure modes: with a wrong secret, and with an unreachable endpoint, the hosted playmat starts, imports and shows cards, and its log records the shared-store failure once, with no error shown to players. Also check that a lookup of a key missing from the real bucket is treated as a miss (404), not as a failure that trips the breaker.
- **Manual — captured in the implementation test plan.** No settings: a hosted start with no `TCGREPLAY_SPACES_*` variables logs one "local only" line with no warning, and a desktop build is unchanged.

## Milestones & Tasks

### Milestone 1: The playmat can read and write DigitalOcean Spaces

**What changes**: Nothing a player sees changes yet. This milestone is internal. The playmat gains a small client for DigitalOcean Spaces, and a stand-in Spaces that tests can run in-process. It earns its own milestone because three specs in this epic build on it: this one for card data, the card image copies, and admin cache clearing. Landing it alone keeps their reviews small, and proves request signing against AWS's published example before anything depends on it. No setting, log line or behaviour of the running playmat changes.

**Validation point**: The signer reproduces AWS's published Signature Version 4 example exactly. Objects round-trip through the client against the stand-in Spaces, a missing object is reported as missing, and refused credentials and server errors are reported with their S3 error code. Settings parsing tells none, complete and partial apart. The existing test suite passes unchanged.

#### - [x] Task: Sign requests and read settings for Spaces
**Id:** 74466fe9-64c8-418d-b653-4c2a71bd852e
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Start a new, card-agnostic Spaces package with its two pure pieces: request signing with AWS Signature Version 4, and the shared-store settings. Settings are the endpoint, bucket, access key and secret, read from the environment, with the signing region worked out from the endpoint. Signing is proven against AWS's own published example, so the client built on it next does not rest on our fake agreeing with itself. Settings parsing tells "none set" apart from "complete" and from "partial or invalid", and names what is wrong without ever echoing a secret.

*Technical detail:* [context.md#task-sign-requests-and-read-settings-for-spaces](./context.md#task-sign-requests-and-read-settings-for-spaces)

**Acceptance criteria**:
- [x] Signing AWS's published GET Object example request produces exactly the signature AWS documents.
- [x] With none of the four settings present, parsing reports the shared store as not configured, with no error.
- [x] With all four present and a valid endpoint, parsing returns them, with the signing region taken from the endpoint (for example `ams3` from `https://ams3.digitaloceanspaces.com`).
- [x] With only some settings present, or an endpoint that is not an http or https address with a host, parsing reports an error naming the missing or invalid setting. The error never contains the key or secret value.

#### - [x] Task: Add the Spaces client and its stand-in
**Id:** a6e2428c-2777-4bee-84a3-e416a57db7d6
**Repo:** tcgreplay
**Depends on:**
- 74466fe9-64c8-418d-b653-4c2a71bd852e — Sign requests and read settings for Spaces
**Execution:** agent

Add the client that reads and writes whole objects in a Spaces bucket, and an in-process stand-in for Spaces that any package's tests can start. The client reports a missing object distinctly from a failure, and reports failures with S3's status and error code, so logs say why. Writes can carry a content type, cache-control and public-read access, ready for the later image spec. The stand-in checks signatures with the real signer, counts requests, and can be made to fail, refuse credentials, hang or go away. That is how every later test exercises the shared store without a real bucket.

*Technical detail:* [context.md#task-add-the-spaces-client-and-its-stand-in](./context.md#task-add-the-spaces-client-and-its-stand-in)

**Acceptance criteria**:
- [x] An object written through the client reads back byte for byte, and the stand-in records the content type, cache-control and public-read setting the write asked for.
- [x] Reading a key that does not exist reports "not found", distinct from every other error.
- [x] With wrong credentials the stand-in refuses the request as Spaces does, and the client reports an error carrying the status 403 and S3's error code. A server error is reported with its status.
- [x] Requests signed by the client are accepted by the stand-in's signature check, and a request altered after signing is refused.
- [x] The stand-in can be switched to fail, refuse, hang or become unreachable, and reports how many requests it received for each key.

### Milestone 2: Cached card data can live in a shared store that outlasts restarts

**What changes**: The card database can now keep its cache in two places, local disk first and a shared store behind it. Card data fetched once survives the local disk being wiped: a fresh start with an empty disk finds it in the shared store, copies it back to disk, and never asks the card source again while it is fresh. It keeps the age it had when first fetched, so a restart never makes stale data look fresh. If the shared store is down, slow or refuses its credentials, lookups carry on from the local cache and the card source with no error, and the failure is logged once. Offline mode and the game's own card lookups never touch the shared store. Nothing is switched on yet in the running playmat: desktop, dev and hosted all behave exactly as before.

**Validation point**: Tests with the stand-in Spaces and the fake card source show each acceptance criterion at the card database. After a warm restart there are zero card-source requests and identical answers. A Spaces-only card is served and copied to disk, and a new card lands in both. A stale entry is still refreshed after a restart. Every outage mode is absorbed and logged. Offline makes zero Spaces requests. A game's cards built after a restart with an empty disk are identical to the first build.

#### - [x] Task: Add the Spaces card store
**Id:** 591bd0ba-960e-45ac-abac-1eef2da12ae5
**Repo:** tcgreplay
**Depends on:**
- a6e2428c-2777-4bee-84a3-e416a57db7d6 — Add the Spaces client and its stand-in
**Execution:** agent

Give the card database a store backed by Spaces. It keeps each cached card-source response as an object under the `tcgdex/` prefix, at the same relative path and in the same envelope (fetch time plus raw body) as the local disk cache. Disk and Spaces then hold identical bytes under matching names, and an entry's fetch time travels with it. This task needs the card cache freshness plan to have landed, since it builds on that plan's store contract and envelope.

*Technical detail:* [context.md#task-add-the-spaces-card-store](./context.md#task-add-the-spaces-card-store)

**Acceptance criteria**:
- [x] An entry put into the Spaces store comes back with the same body and exactly the same fetch time.
- [x] Entries land at `tcgdex/sets.json`, `tcgdex/sets/<id>.json` and `tcgdex/cards/<id>.json`, each holding the same bytes the disk cache writes for that entry.
- [x] A key with no object, or an object that is not a valid envelope, is reported as "not cached".
- [x] A refused or failed request is reported as an error, distinct from "not cached".

#### - [x] Task: Add the tiered store with a failure breaker
**Id:** 11456ca4-6890-4532-95fa-cc57d57ce8d7
**Repo:** tcgreplay
**Depends on:**
- 591bd0ba-960e-45ac-abac-1eef2da12ae5 — Add the Spaces card store
**Execution:** agent

Add a store that puts local disk in front of a shared store. A read is served from disk. On a disk miss it is served from the shared store, and copied to disk with its original fetch time. A write goes to both. Shared-store trouble never reaches a lookup: each shared request is bounded at 3 seconds. A failure is logged once and the shared store is skipped for a minute, and its recovery is logged once. So an outage costs at most one slow request a minute and two log lines.

*Technical detail:* [context.md#task-add-the-tiered-store-with-a-failure-breaker](./context.md#task-add-the-tiered-store-with-a-failure-breaker)

**Acceptance criteria**:
- [x] An entry only in the shared store is returned and then exists on disk with exactly the same fetch time. An entry on disk is returned without any shared-store request.
- [x] An entry written to the tiered store exists both on disk and in the shared store.
- [x] With the shared store unreachable, refusing credentials, failing, or hanging, reads fall back to "not cached" and writes still reach disk, with no error returned. A hanging store delays only the first request, and by no more than 3 seconds.
- [x] After a failure, no shared-store request is made for one minute. The next request after that tries again, and recovery is logged.
- [x] An outage is logged once when it starts and once when it ends, not once per lookup.

#### - [x] Task: Keep game-lock and offline reads on local disk
**Id:** 82ace641-7996-4ea0-8792-9fdf3127dece
**Repo:** tcgreplay
**Depends on:**
- 11456ca4-6890-4532-95fa-cc57d57ce8d7 — Add the tiered store with a failure breaker
**Execution:** agent

Make the card database read only its local tier in the two places that must never touch the network. The first is the cached-only lookup the game uses while it holds its lock. The second is every lookup in offline mode. A card held only in the shared store is then "not cached" to those reads, and the game's existing background warm fetches it through the normal lookup, which does reach the shared store and fills the disk.

*Technical detail:* [context.md#task-keep-game-lock-and-offline-reads-on-local-disk](./context.md#task-keep-game-lock-and-offline-reads-on-local-disk)

**Acceptance criteria**:
- [x] The cached-only card lookup makes no shared-store request, even for a card that is only in the shared store, and returns nothing for it.
- [x] In offline mode with a shared store attached, no lookup of any kind (card, deck code, by name) makes a request to the shared store.
- [x] An online lookup of a card held only in the shared store returns it with no request to the card source, and the card then exists in the local cache directory.
- [x] Without a shared store attached, every lookup behaves exactly as before, and the existing card, game and importer tests pass unchanged.

#### - [x] Task: Prove a restart keeps the card cache
**Id:** d302f56c-8933-4cae-bfef-0425ab6625dd
**Repo:** tcgreplay
**Depends on:**
- 82ace641-7996-4ea0-8792-9fdf3127dece — Keep game-lock and offline reads on local disk
**Execution:** agent

Add the acceptance tests that show the spec's behaviour end to end at the card database and at game level. They use the stand-in Spaces and the fake card source, and simulate a restart by starting a second card database on an empty cache directory against the same shared store. They cover the warm restart, filling the local cache, sharing newly fetched data, keeping fetch ages, every outage mode, and offline mode. The game-level test proves an import's cards come out identical after a restart, which guards the rule that cache contents never change where a card goes.

*Technical detail:* [context.md#task-prove-a-restart-keeps-the-card-cache](./context.md#task-prove-a-restart-keeps-the-card-cache)

**Acceptance criteria**:
- [x] After a restart with an empty local cache, resolving the same deck codes and loading the same cards makes no request to the card source for the set list, set card lists or cards, and gives identical answers.
- [x] A game's cards built from the same decklists after such a restart are identical to the first build.
- [x] With the local cache empty, a card only in the shared store is returned without the card source, and then exists locally. A card in neither is fetched once and then exists in both.
- [x] A shared-store entry past its refresh age, read after a restart, is still treated as stale and refreshed from the card source once. Its refreshed copy lands in both places.
- [x] With the shared store unreachable, refusing credentials, failing or hanging, deck codes, by-name lookups and card lookups all succeed from the card source with no error, and the failure is logged.
- [x] In offline mode with a shared store attached, the shared store receives no requests.

### Milestone 3: The hosted playmat keeps its card cache across deploys

**What changes**: With the shared store's four settings set in App Platform, the hosted playmat keeps its card cache in DigitalOcean Spaces. Imports right after a deploy or restart are as fast as any other, and the card source is asked only for data the shared store does not hold or that has gone stale. At start-up the playmat warms its card catalog from the shared store in the background, so the first import does not wait for it. Without the settings, the hosted playmat runs exactly as today and says so in one log line. With partial or wrong settings, it warns in its log and carries on with its local cache. Desktop and dev builds are unchanged and need no new settings. The deploy definition, the container run target and the hosted documentation describe the new settings, and an operator creates the bucket and key and sets them.

**Validation point**: Hosted wiring tests show that each kind of settings (none, complete, partial, wrong credentials, offline) produces the right card database and log lines, and that start-up does not wait for the warm. After the operator's setup, the manual checks in the implementation test plan confirm the behaviour on real Spaces: warm restart, outage, and the 2-second first import.

#### - [x] Task: Use the shared store in hosted mode
**Id:** c0450f8d-572c-492c-aa1d-888914fc9d2d
**Repo:** tcgreplay
**Depends on:**
- 82ace641-7996-4ea0-8792-9fdf3127dece — Keep game-lock and offline reads on local disk
**Execution:** agent

Wire the shared store into hosted start-up, and only there. When all four settings are present and the playmat is online, the hosted card database gets the tiered store, and a background warm of the card catalog starts so the first import does not wait for it. With no settings, hosted mode runs as today and logs one plain line saying the cache is local only. With partial or invalid settings it logs a warning and runs local-only. In offline mode it logs that the shared store is not used. Desktop, daemon and dev builds never read the settings.

*Technical detail:* [context.md#task-use-the-shared-store-in-hosted-mode](./context.md#task-use-the-shared-store-in-hosted-mode)

**Acceptance criteria**:
- [x] With all four settings pointing at a reachable store, card data the hosted playmat fetches is saved in the shared store as well as in its local cache directory.
- [x] With no settings, the hosted playmat starts, writes card data only to its local cache directory, and logs no warning or error about the shared store.
- [x] With partial or invalid settings, the hosted playmat starts, logs a warning naming the problem (never a secret), and caches locally only.
- [x] With wrong credentials, the hosted playmat starts and serves cards from the card source, and its log records the shared-store failure.
- [x] In offline mode with the settings present, the hosted playmat makes no request to the shared store.
- [x] With the shared store on, the health check answers while the catalog warm is still running, and the warm's result is logged when it finishes.
- [x] Desktop and dev builds start and behave exactly as before, whatever shared-store settings are in their environment.

#### - [x] Task: Declare and document the shared store settings
**Id:** b49eb3be-7dd2-487a-96ed-d3ada79a5a1c
**Repo:** tcgreplay
**Depends on:**
- c0450f8d-572c-492c-aa1d-888914fc9d2d — Use the shared store in hosted mode
**Execution:** agent

Make the new settings discoverable where an operator looks. The App Platform definition declares the four variables as encrypted values set in the console. Its comments, and the hosted design document's trial-limits note, stop saying the card cache is lost on every deploy. The container run target passes the variables through, and the image's header comment lists them. The design document's hosted settings table gains the four rows, and its operator setup gains the steps for creating the bucket and key.

*Technical detail:* [context.md#task-declare-and-document-the-shared-store-settings](./context.md#task-declare-and-document-the-shared-store-settings)

**Acceptance criteria**:
- [x] The App Platform definition declares all four shared-store variables for run time, with the access key and secret as encrypted secrets, and explains each in a comment.
- [x] Running the container locally passes the shared-store variables through when they are exported.
- [x] The hosted settings table and operator setup describe each variable, its default (local cache only), and how to create the bucket and access key.
- [x] No deploy file or document still says the hosted card cache is always lost on a deploy or restart. The build context is unchanged, so the guides are still built in.

#### - [x] Task: Update the card cache knowledge entry
**Id:** c9549726-3611-4275-a486-2bc9a5b0cf86
**Repo:** tcgreplay
**Depends on:**
- c0450f8d-572c-492c-aa1d-888914fc9d2d — Use the shared store in hosted mode
**Execution:** agent

The card cache knowledge entry, as the freshness plan leaves it, describes a local cache only. Extend it with the hosted shared tier: where entries live in the bucket, how they reach local disk with their fetch time kept, and which reads never touch it (cached-only and offline). Add how outages behave, and the trap that deleting the hosted cache directory no longer forces fresh data while the shared copies remain.

*Technical detail:* [context.md#task-update-the-card-cache-knowledge-entry](./context.md#task-update-the-card-cache-knowledge-entry)

**Acceptance criteria**:
- [x] A reader of the card cache entry learns that hosted mode can keep entries in Spaces under `tcgdex/`, which settings switch it on, and that local disk is a per-run copy.
- [x] The entry says that forcing fresh data on the hosted playmat means removing the `tcgdex/` objects as well as restarting, until admin cache clearing exists.
- [x] The entry says the cached-only lookup and offline mode never read the shared store, and that a shared-store outage is logged and skipped for a minute at a time.

#### - [x] Task: Create the Spaces bucket and set its secrets
**Id:** 1e7d6bb7-a950-4432-b4db-f374ae7baa94
**Repo:** tcgreplay
**Depends on:**
- b49eb3be-7dd2-487a-96ed-d3ada79a5a1c — Declare and document the shared store settings
**Execution:** human — needs the DigitalOcean account's control panel to create the bucket and an access key, and to set encrypted production secrets in App Platform

Create the private Spaces bucket the hosted playmat will use, and an access key allowed to read and write it. Then set the four shared-store variables on the App Platform component, the key and secret as encrypted values, and redeploy. Until this is done, the hosted playmat keeps running with its local cache only, exactly as today.

*Technical detail:* [context.md#task-create-the-spaces-bucket-and-set-its-secrets](./context.md#task-create-the-spaces-bucket-and-set-its-secrets)

**Acceptance criteria**:
- [x] A private Spaces bucket exists for the hosted playmat, with an access key that can read and write it.
- [x] The four shared-store variables are set on the hosted playmat's App Platform component, with the key and secret encrypted.
- [x] After a redeploy, the hosted playmat's start-up log names the shared store, and no shared-store warning appears.

## Open Questions

- **Does the freshness plan's implementation leave the seams this plan assumes?** This plan expects four things: `Client.Store` holds the store every lookup uses; `Cached` and the offline path read through one store accessor; the envelope encoder and key-to-path mapping can be factored out of the disk store; and a catalog-building function (`ensureCatalog`) exists. This depends on how that plan's tasks were actually implemented, which only shows once it has landed. *What to do:* adapt names to what landed, keeping the behaviour this plan specifies. If `Cached` or offline reads cannot be pointed at a local-only store without changing the `Store` interface, or if the catalog cannot be built on demand outside a lookup, STOP and ask the user rather than changing the shared contract.
- **Does real DigitalOcean Spaces answer a missing key with 404 for the access key the operator creates?** The tiered store treats 404 as a normal miss and 403 as a failure that trips the breaker. A limited key without list permission might get 403 for missing objects. That is only discoverable against the real service, during the operator's setup and the manual check. *What to do:* if real Spaces returns 403 `AccessDenied` for missing keys with a correctly scoped key, STOP and ask the user whether to require a key with list permission (documentation only) or to treat 403 on GET as a miss (which would hide refused credentials on reads).

No other questions are left for implementation time. Every other decision is settled in the sections above or recorded as a drafting assumption.

## Out of Scope

- **Desktop and dev builds using the shared store** (spec Non-Goal). Only hosted mode reads the `TCGREPLAY_SPACES_*` settings, and desktop, daemon and dev builds never build the shared tier.
- **Keeping games, game records or sign-ins in the shared store** (spec Non-Goal). Only card data (the set list, set card lists and cards) goes to Spaces.
- **Card images** — copying them to Spaces and serving them through the Spaces CDN is `20261104090640-25f673ed-card-image-cdn`. This plan leaves the Spaces client ready for it (public-read writes with content type and cache-control) and keeps card data under its own `tcgdex/` prefix.
- **Clearing the cache on demand** (all of it or one set, in every tier, including Spaces) is `20261104090640-a5a74fab-admin-cache-clearing`, which will add delete and list to the Spaces client. Until then, forcing fresh hosted data means deleting the bucket's `tcgdex/` objects by hand.
- **Changing how cards are matched or which print is preferred** (spec Non-Goal). Freshness rules, print preference and the catalog stay exactly as the card cache freshness plan leaves them.
- **Avoiding the first cold build.** The first deploy starts with an empty bucket, so it still fetches every set from the card source once (spec Technical Approach risk). No seeding step is added.
- **Copying the whole bucket to local disk at start-up.** Entries reach disk only when looked up. The start-up warm loads the catalog (set list and set card lists), not every card.
- **Queuing writes made while the shared store is down.** They reach Spaces the next time the entry is fetched or refreshed.
- **Persisting the breaker state, or retrying shared writes**, beyond the single next attempt after the one-minute window.
- **Configurable shared-store timeouts, retry windows or object prefix.** They are package constants.
- **Other S3 providers or several deployments sharing one bucket.** The client is S3-compatible and the settings accept any endpoint, but only DigitalOcean Spaces is supported and checked, and the `tcgdex/` prefix is fixed.
- **Purging or using the Spaces CDN.** Card data objects are private and read only through the API.

## Changelog


### 2026-11-04 — Task: Sign requests and read settings for Spaces

**What was done**: Added the card-agnostic `internal/spaces` package with its two pure pieces: an AWS Signature Version 4 signer (proven byte for byte against AWS's published GET Object example) plus `VerifyRequest` for test doubles, and `Config`/`FromEnv`, which reads the four `TCGREPLAY_SPACES_*` settings, tells none, complete and partial/invalid apart, and derives the signing region from the endpoint.

**Deviations**: The `Error` type (status, S3 code, message) was added now, in `internal/spaces/errors.go`, rather than in the client task, because `VerifyRequest` returns it. `VerifyRequest` also refuses a signable header (`x-amz-*`, content type, cache-control) that was not signed, as S3 does, so a header added after signing is caught. Settings errors name the setting only and never echo a value (not even the endpoint or bucket), after the goal check found that a mis-set variable could otherwise leak a credential into the log. Outside the plan: `cmd/tcgreplay/guides_test.go` `TestHostedGuidesMatchDesktop` expected 6 desktop guides but main now lists 7 (a `play-game` guide was added), so the expectation was changed to 7 to make `go test ./...` green.

**Files changed**:
- `internal/spaces/doc.go`
- `internal/spaces/sign.go`
- `internal/spaces/errors.go`
- `internal/spaces/config.go`
- `internal/spaces/sign_test.go`
- `internal/spaces/config_test.go`
- `cmd/tcgreplay/guides_test.go`

**Discoveries**: The signer computes the canonical URI from the decoded `URL.Path`, encoding each segment once; the client must send the same encoding on the wire (set `URL.RawPath`) so the server's view matches. On the server side the host comes from `r.Host`, not `r.URL.Host`.

### 2026-11-04 — Task: Add the Spaces client and its stand-in

**What was done**: Added `spaces.Client` (`New`, `Get`, `Put` with content type, cache-control and public-read options, `Bucket`, `Endpoint`), which maps a 404 to `ErrNotFound` and any other answer to an `*Error` carrying S3's status and code. Added `internal/spaces/spacestest`, an in-process S3 stand-in that verifies every request with the real signature check, counts requests per method and key, and can be switched to fail (500), refuse (403 `InvalidAccessKeyId`), hang or close.

**Deviations**: `spacestest` exports its fixed bucket and credentials as constants and adds a `Client(t)` convenience beside `Config()`. `Hang()` shares one gate between calls, and `Close()` releases any hang before shutting down, so a test cleanup never blocks on a held request. The stand-in checks the signature before the bucket name, as S3 answers unauthenticated requests first.

**Files changed**:
- `internal/spaces/client.go`
- `internal/spaces/spacestest/spacestest.go`
- `internal/spaces/client_test.go`

**Discoveries**: `httptest.Server.Close` waits for in-flight requests, so a stand-in that holds requests open must release them before closing. A 404 from `Get` wraps `ErrNotFound` with the key, so callers must use `errors.Is`, not `==`.

### 2026-11-04 — Task: Add the Spaces card store

**What was done**: Added `cards.NewSpacesStore`, a `Store` over the Spaces client that keeps each entry at `tcgdex/<disk relative path>` in the same envelope the disk store writes (`application/json`, private). A missing object or a non-envelope object is `ErrNotCached`; any other failure is returned as an error. The disk store's key-to-path mapping and envelope encode/decode were factored into shared helpers (`relPath`, `encodeEnvelope`, `decodeEnvelope`) with no change to its behaviour or file paths.

**Deviations**: The key mapping helper is `relPath(key) string` with no error for an unknown prefix (the plan sketched `keyPath(key) (string, error)`), because the disk store has always mapped any key and the two tiers must name entries identically. `encodeEnvelope` takes the key for its error messages.

**Files changed**:
- `internal/cards/store.go`
- `internal/cards/spacesstore.go`
- `internal/cards/spacesstore_test.go`

**Discoveries**: None.

### 2026-11-04 — Task: Add the tiered store with a failure breaker

**What was done**: Added `cards.NewTieredStore(local, shared)`: reads come from local, a local miss is served from shared and copied to local with its `Fetched` unchanged, writes go to local then shared. Each shared request is bounded by `SharedTimeout` (3 s); any shared failure is absorbed (reads become "not cached", only local write errors are returned), logged once, and trips a breaker that skips shared for `SharedRetryAfter` (1 minute), with the recovery logged once. `Local()` exposes the local tier.

**Deviations**: A shared request that ends because the caller's own context ended (not the store's timeout) neither trips nor clears the breaker, so a caller giving up is never logged as a Spaces outage. Not in the plan, added because the card client's fetch path hands the store a context bounded by its own source timeout.

**Files changed**:
- `internal/cards/tiered.go`
- `internal/cards/tiered_test.go`

**Discoveries**: The client's single-flight fetch (`shared` in `cards.go`) writes to the store on a context bounded by the card-source timeout (5 s), so a shared write after a slow fetch gets only what is left of it.

### 2026-11-04 — Task: Keep game-lock and offline reads on local disk

**What was done**: The card client gained `localStore()` (the store's `Local()` tier when it has one, found through an unexported optional interface) and `readStore()` (the whole store online, the local tier offline). `Cached` reads only the local tier; `Card` and `loadEntry` read through `readStore()`, so offline lookups of every kind never reach the shared store. Without a tiered store all three are the disk store, so behaviour is unchanged.

**Deviations**: `Card` reads the store directly as well as through `loadEntry`, so the offline rule was applied in both places (via `readStore()`). The single-flight recheck in `shared()` now reads only the local tier: another caller's write lands there first, and reading the shared tier again would cost a second Spaces GET for every cold miss.

**Files changed**:
- `internal/cards/cards.go`
- `internal/cards/tiered_client_test.go`

**Discoveries**: `go test -race ./...` can fail in `cmd/tcgreplay` `TestRelaySurvivesDaemonRestart` because of a data race in the test itself (`shim_test.go` reassigns `stop` in a goroutine and reads it at the end). It is pre-existing, unrelated to this plan, and passes without `-race` or when run alone; it was left as is.

### 2026-11-04 — Task: Prove a restart keeps the card cache

**What was done**: Added the spec's acceptance tests at the card database (`restart_test.go`: warm restart with zero card-source requests and identical answers, filling local and sharing new cards, stale card and stale set refreshed once after a restart into both tiers, every outage mode absorbed and logged once, offline making no shared requests) and at game level (`spacescache_test.go`: `BuildCards` after a restart with an empty cache directory gives an identical card list and warnings with zero card-source requests). No production code changed.

**Deviations**: The card-package tests reuse the freshness plan's `sv01`/`sv02` fixtures (`serveAll`) rather than new `tst`/`sve` ones. The game-level fake card source serves the bodies `seedCardCache` lays out, rather than a second inline copy of the pool.

**Files changed**:
- `internal/cards/restart_test.go`
- `internal/game/spacescache_test.go`

**Discoveries**: None.

### 2026-11-04 — Task: Use the shared store in hosted mode

**What was done**: Hosted start-up now builds its card database through `hostedCardDB`, which reads the four `TCGREPLAY_SPACES_*` settings: none set logs one "local only" line; partial or invalid settings log a one-line warning naming the setting (never a value) and run local-only; offline logs that the shared store is not used; all four set and online wrap the disk store in the tiered store over Spaces and log the bucket. With the shared store on, `runHosted` starts `warmCatalog` in the background (new `cards.Client.Warm`, bounded at 2 minutes), which logs how long the warm took or why it failed. `newHosted` takes the card database as a parameter; `config` gained the test-only `cardBase`. Desktop, daemon and dev builds still use the unchanged `newCardDB` and never read the settings.

**Deviations**: A multi-setting error (several missing) is joined onto one log line with "; ". The hosted tests log through a mutex-guarded writer (`captureSyncLog`) rather than `captureLog`, because the catalog warm and the breaker log from background goroutines while the test reads the log.

**Files changed**:
- `internal/cards/warm.go`
- `internal/cards/warm_test.go`
- `cmd/tcgreplay/main.go`
- `cmd/tcgreplay/hosted.go`
- `cmd/tcgreplay/hosted_test.go`
- `cmd/tcgreplay/hosted_cache_test.go`

**Discoveries**: `captureLog` in `cmd/tcgreplay/hosted_test.go` hands the standard logger a plain `bytes.Buffer`, so reading it while a background goroutine logs is a data race under `-race`; tests that poll the log during background work need a synchronised writer.

### 2026-11-04 — Task: Declare and document the shared store settings

**What was done**: `.do/app.yaml` declares the four `TCGREPLAY_SPACES_*` variables as run-time, value-less encrypted secrets with a comment on the endpoint, the private bucket, a bucket-limited read/write key, the `tcgdex/` prefix and the local-only default, and its header no longer says the card cache is always lost. The `Dockerfile` header lists the variables, `make docker-run` passes them through, and the design document's trial-limits note, settings table (four rows) and operator setup (new step 6: create the bucket and key, set the variables, what the start-up log shows, how to force fresh data) describe the shared store.

**Deviations**: None. The build context and `.dockerignore` are unchanged; `docker build` of the worktree succeeds.

**Files changed**:
- `.do/app.yaml`
- `Dockerfile`
- `Makefile`
- `TCG Review Playmat Design and Build Plan.md`

**Discoveries**: None.

### 2026-11-04 — Task: Update the card cache knowledge entry

**What was done**: Extended the repo-tier knowledge entry `gotchas/tcgdex-cache-never-expires.md` (store `tcgreplay`) with a "Hosted shared tier" section: the four settings that switch it on (hosted only), the `tcgdex/` object layout in the same envelope, local disk as a per-run copy filled with the original fetch time, `Cached` and offline reading only the local tier, the logged one-minute breaker with unqueued writes, the 404-versus-403 caveat, and that forcing fresh hosted data now means deleting the `tcgdex/` objects as well as restarting, until admin cache clearing exists. Added the `spaces` tag (kept the existing `hosting` tag rather than adding `hosted`).

**Deviations**: The plan suggested adding a `hosted` tag; the vocabulary already has `hosting` on this entry, so only `spaces` was added.

**Files changed**:
- knowledge entry (repo tier, store `tcgreplay`): `gotchas/tcgdex-cache-never-expires.md`, written through `spektacular knowledge write`

**Discoveries**: None.

### 2026-11-04 — Task: Create the Spaces bucket and set its secrets

**What was done**: Nothing in the repo. This is a human task (it needs the DigitalOcean control panel to create the bucket and access key and to set encrypted App Platform secrets), so the agent run left it for the operator. It went through the goal check, which judged all three criteria not met (no bucket, secrets or deployment log can be seen from the repo), and was then skipped at the orchestrator's instruction.

**Deviations**: Skipped, outstanding. All three acceptance criteria are left unticked: the bucket and bucket-limited read/write key, the four `TCGREPLAY_SPACES_*` variables set encrypted on the `playmat` component, and a redeploy whose start-up log names the shared store with no shared-store warning. Until it is done the hosted playmat runs with a local-only card cache, exactly as before. Its checks are in the manual test plan. Reason recorded: human task left for the operator (needs the DigitalOcean control panel); the orchestrator reports it to the user.

**Files changed**:
- None

**Discoveries**: A whole-plan implement run can only get past an open human task by sending it through the goal check until its attempts are exhausted and then recording a `skip` decision. `goal decide` is refused (`goal_decision_not_due`) before the check has stopped the run.

### 2026-11-06 — Task: Create the Spaces bucket and set its secrets

**What was done**: The operator created the private `tcgreplay` bucket in `ams3` with an access key, set the four `TCGREPLAY_SPACES_*` variables encrypted on the `playmat` component, and redeployed. The 10:14:00 start-up log reads `card cache: /home/nonroot/.cache/tcgreplay backed by shared store https://ams3.digitaloceanspaces.com/tcgreplay` with no shared-store warning, and `card cache: catalog warmed in 3.983s` follows. All three criteria are met and ticked.

**Deviations**: The first deploy set `TCGREPLAY_SPACES_ENDPOINT` to the bucket's origin address (`https://tcgreplay.ams3.digitaloceanspaces.com`, which the DigitalOcean panel labels "Origin Endpoint") rather than the region endpoint. `normalized()` accepts it, so start-up still logged `backed by shared store`; requests then went path-style under the bucket host, card data keys landed under `tcgreplay/`, and the image listing failed with `404 NoSuchKey`. Setting the region endpoint `https://ams3.digitaloceanspaces.com` fixed it.

**Files changed**:
- None

**Discoveries**: A bucket-origin endpoint passes settings validation and shows only as later shared-store failures. Accepting it (stripping a leading `<bucket>.` label) or refusing it at start-up would catch this; not done yet.
