---
created_date: "2026-11-04"
document_status: final
closed_date: "2026-11-04"
---

# Research: 20261104082020-109dceb1-tcgdex-spaces-cache

## Alternatives considered and rejected

### Option: Shared-tier logic inside `cards.Client` (a second `Shared Store` field and a three-way branch in `loadEntry`)

**Rejected**: the dependency plan `20261104090640-51bd1f36-card-cache-freshness` puts every lookup through one `store()` slot and says the Spaces tier arrives as "a tiered `Store` (disk, then Spaces) in the same slot" so freshness rules stay above the store and untouched (dependency plan, Architecture: "The next spec in this epic adds a DigitalOcean Spaces tier by putting a tiered `Store` ... in the same slot"). Branching inside `Client` would spread tier rules across the freshness code. The one place `Client` must know about tiers (local-only reads for `Cached` and offline) is handled with an optional `Local() Store` method found by type assertion, which leaves the `Store` interface unchanged.

### Option: `github.com/minio/minio-go/v7` as the S3 client

**Rejected**: it brings a dozen transitive modules (klauspost/compress, google/uuid, go-humanize, rs/xid, minio/md5-simd, golang.org/x/net and x/crypto among them) into a module that has three direct dependencies (`go.mod`). This feature needs two object operations (GET, PUT); the later specs need DELETE and a prefix listing. The dependency plan made the same call for single-flight ("written in-package rather than adding `golang.org/x/sync`", dependency plan Dependencies).

### Option: `github.com/aws/aws-sdk-go-v2/service/s3`

**Rejected**: the same reason as `minio-go`, more strongly: the S3 service module plus `config`, `credentials`, `smithy-go` and the endpoint resolvers, for two calls, and DigitalOcean endpoint overrides that need custom resolver code.

### Option: Fetch time in S3 object metadata (`x-amz-meta-fetched`) with the raw TCGdex body as the object

**Rejected**: the project-wide rule fixes the entry encoding as the envelope `{"fetched": RFC 3339, "data": <body>}` (dependency plan, Data Structures "On-disk entry format"). Keeping the same bytes in both tiers means one encoder, no header parsing, and an object downloaded by hand is self-describing.

### Option: Copy the whole shared store to disk at start-up (list the bucket, download everything)

**Rejected**: as the main mechanism: it needs a bucket listing this spec otherwise does not need, it downloads cards no game uses (tens of thousands of objects over time), and it still races the first import. Read-through per key, plus warming the catalog (set list and set card lists, about 200 objects) in the background at hosted start-up, gives the same first-import speed for far less traffic.

### Option: Write-behind (asynchronous) puts to the shared store

**Rejected**: puts happen only after a TCGdex fetch, which already costs a network round trip, so one more synchronous PUT is cheap; synchronous puts keep tests deterministic (no flush helper), and a failure breaker (below) bounds the cost during an outage.

### Option: Spaces as the only tier on the hosted instance (no local disk)

**Rejected**: the spec's Technical Approach keeps local disk as "a per-run scratch copy", and `Cached` (called under the game lock, `internal/game/game.go:500-515`) must answer from local data without any network call (project rule: nothing under the game lock calls the network).

### Option: Fail hosted start-up on partial or invalid shared-store settings

**Rejected**: the spec requires that shared-store problems never stop play; a typo in an optional cache setting should degrade to local-only with a logged warning, not take the hosted playmat down. (Contrast: missing Google credentials stop start-up, `cmd/tcgreplay/hosted.go:117-121`, because running open is unsafe; running without the shared cache is merely slower.)

### Option: Virtual-hosted-style addressing (`<bucket>.<region>.digitaloceanspaces.com`)

**Rejected**: for this client: path-style (`<endpoint>/<bucket>/<key>`) needs no per-bucket DNS, works unchanged against an `httptest` fake or MinIO, and DigitalOcean Spaces accepts it. The later image spec's public CDN address is a separate, unsigned URL and does not depend on the API addressing style.

## Chosen approach — evidence

- `internal/cards/cards.go:167-190` — today's `Client` (`Base`, `CacheDir`, `HTTP`, `Offline`); the dependency plan adds `Store Store`, `Timeout`, `Now`, with `NewClient` defaulting `Store` to `NewDiskStore(dir)`. A tiered store drops into `Client.Store`.
- Dependency plan, context "Task: Add the cache store with fetch times" — `Store{Get, Put}`, `Entry{Data, Fetched}`, `ErrNotCached`, keys `sets`, `sets/<id>`, `cards/<id>`, disk paths via `safeName` (`internal/cards/cards.go:200-202`), envelope encode/decode in `internal/cards/store.go`. The Spaces store reuses the same encoder and the same key-to-relative-path mapping, so object names equal disk paths under a fixed prefix.
- Dependency plan, context "Task: Route card lookups through the store" — `loadEntry` = `store().Get`, then on `ErrNotCached` fetch + `store().Put`; `Cached` uses only `store().Get`. With a tiered store, `Cached` would reach Spaces on a disk miss, so `store()` needs a local-only variant for `Cached` and for offline mode.
- `internal/game/game.go:481-515` and dependency plan "Keep card fetches out of the game lock" — `Game.card` reads `Cached` under `g.mu` and warms misses in the background via `Card`; a local-only `Cached` keeps that path network-free, and the background warm reaches Spaces through `Card`.
- `internal/game/manager.go:204-235` — `BuildCards` resolves deck codes and loads every card through `DB.Card` outside the game lock: the path that reaches the shared tier on a restart.
- `cmd/tcgreplay/main.go:224-229` (`newCardDB`) and `cmd/tcgreplay/hosted.go:174-175` (`newHosted` calls `newCardDB(c)`) — the single hosted construction point; the desktop paths (`newServer`, `runInProcess`, daemon) also call `newCardDB`, so hosted-only wiring goes in `newHosted`.
- `cmd/tcgreplay/hosted.go:110-130` — secrets read with `os.Getenv` only (never flags), tests set them with `t.Setenv`/subprocess env (`cmd/tcgreplay/hosted_test.go:452-470`, `499-540`). Spaces credentials follow the same pattern.
- `.do/app.yaml:39-50` — encrypted secrets declared as `type: SECRET` with `scope: RUN_TIME`, values set in the console; comment at lines 9-11 says the card cache is lost on every deploy.
- `Makefile:60-63` — `docker-run` passes hosted env vars through with bare `-e NAME`.
- `TCG Review Playmat Design and Build Plan.md:302,311-325` — the hosted settings table and operator setup steps, which the docs task extends; its "Trial limits" sentence says the card cache is lost on every deploy.
- AWS Signature Version 4 for S3 (header-based auth) — documented algorithm plus worked examples ("GET Object" with `examplebucket`, `AKIAIOSFODNN7EXAMPLE`, 20130524) that give a published expected signature, so a stdlib signer can be checked against an authoritative vector rather than only against its own fake.

## Files examined

- `tcgreplay:internal/cards/cards.go:1-21` — imports: stdlib only; package doc says it caches on disk.
- `tcgreplay:internal/cards/cards.go:167-198` — `Client` fields, `NewClient` (20 s HTTP timeout), `DefaultCacheDir`.
- `tcgreplay:internal/cards/cards.go:200-241` — `safeName` (`/`, `\`, `..` → `_`), `getJSON` (cache forever, atomic `.tmp` + rename write).
- `tcgreplay:internal/cards/cards.go:243-287` — `Card` and `Cached`; `Cached` reads disk directly with no ctx.
- `tcgreplay:internal/cards/byname_test.go:1-50` — package-internal test style, `t.TempDir()` fixtures, one test against the real cache that skips without it.
- `tcgreplay:internal/cards/cardstest/cardstest.go:1-30` — `FakeDB` with set `TST` cards `tst-1`..`tst-8`, `sve-006`; the model for an exported test-double package (`spacestest`).
- `tcgreplay:internal/game/game_test.go:18-31` — `testDeck` (TST 1-6, SVE 6) used for game-level fixtures.
- `tcgreplay:internal/game/manager.go:185-235` — `rng`, `NewGameRequest`, `BuildCards` (deterministic IIDs in list order; defs from `Resolve`/`Card`).
- `tcgreplay:cmd/tcgreplay/main.go:40-62,121-150,220-229` — config struct, flag/env parsing, `newCardDB`.
- `tcgreplay:cmd/tcgreplay/hosted.go:68-130,166-247` — `envInt`/`envDuration`, `hostedAuth` (env-only secrets), `newHosted` (one shared card DB for all accounts), `runHosted` start-up log line.
- `tcgreplay:cmd/tcgreplay/hosted_test.go:380-560` — `captureLog`, `hostedConfig`, `runHosted` lifecycle test, subprocess helper `TestHostedMainHelper` (always passes `-offline`), log assertions (`strings.Contains(out, "warning")` must be false on a clean start).
- `tcgreplay:.do/app.yaml` — one instance, no volume, env vars and secrets; header comment says cache lost on deploy.
- `tcgreplay:Dockerfile` — distroless static image, CA certs included (needed for HTTPS to Spaces), comment lists settings.
- `tcgreplay:Makefile:49-63` — `docker-run` env pass-through.
- `tcgreplay:go.mod` — Go 1.27.2; direct deps websocket, MCP go-sdk, oauth2.
- `tcgreplay:TCG Review Playmat Design and Build Plan.md:302-325` — hosted mode description, settings table, operator setup.

## External references

- AWS, "Authenticating Requests: Using the Authorization Header (AWS Signature Version 4)" and "Signature Calculations for the Authorization Header: Transferring Payload in a Single Chunk" (S3 API reference) — canonical request, string to sign, signing key derivation, and the worked GET Object example used as the signer's test vector.
- AWS S3 API reference, GetObject / PutObject and the error response XML (`<Error><Code>…</Code><Message>…</Message></Error>`) — status codes (200, 404 `NoSuchKey`, 403 `AccessDenied`/`InvalidAccessKeyId`/`SignatureDoesNotMatch`) the client maps.
- DigitalOcean Spaces documentation, "Spaces S3 compatibility" and "How to use the S3 API with Spaces" — endpoint `https://<region>.digitaloceanspaces.com`, path-style and virtual-hosted addressing both supported, SigV4 signing, access keys (key id + secret) created per team with optional per-bucket limits.
- DigitalOcean App Platform documentation, "Use environment variables" — `type: SECRET` encrypts a value; `doctl apps update` does not carry secret values (already noted in the design doc's operator steps).
- RFC 5861 (stale-while-revalidate / stale-if-error) — the dependency plan's freshness model, which this tier inherits unchanged.

## Prior plans / specs consulted

- Plan `20261104090640-51bd1f36-card-cache-freshness` (planned, not implemented) — defines `Store`/`Entry`/`ErrNotCached`, key names, the envelope, `Client.Store`/`Timeout`/`Now`, `loadEntry`, catalog built from the store (8-way), local `Cached`, offline never contacting a source, background refresh `Put`s through the store, test helpers `fakeTCGdex`/`fakeClock`/`seedEnvelope` in `internal/cards` test files. This plan builds on all of it and must not redesign it.
- Spec `20261104090640-25f673ed-card-image-cdn` — will store image copies in the same Spaces bucket and serve them from the Spaces CDN; needs PUT with a content type, public-read ACL and cache headers. Drives `spaces.PutOptions` and a key layout that keeps card data under its own prefix.
- Spec `20261104090640-a5a74fab-admin-cache-clearing` — will clear all card data, or one set's card list, cards and images, from every tier; needs DELETE and prefix listing. Drives keeping card data under one prefix (`tcgdex/`) with set and card keys that a prefix can select (`tcgdex/sets/<id>.json`, `tcgdex/cards/<setid>-…`).
- Spec `20261017143118-b05aa6b1-per-user-games` (via the design doc) — hosted settings conventions: env-only secrets, start-up log line, settings table in the design doc.

## Open assumptions

- DigitalOcean Spaces answers a GET for a missing key with 404 `NoSuchKey` for the access key the operator creates. If a limited key without list permission gets 403 for missing keys, every cold miss would trip the failure breaker; the manual real-Spaces check verifies this. If it turns out wrong, the implement workflow must STOP and ask.
- Spaces accepts SigV4 with the region taken from the endpoint's first host label (`ams3` from `ams3.digitaloceanspaces.com`); DigitalOcean also documents `us-east-1` as accepted. Verified only by the manual check.
- The dependency plan lands as written: `Client.Store` exists and every read except `Cached` goes through `store()`; `Cached` reads through `store().Get`; `ensureCatalog(ctx)` exists and builds the catalog from the store. If its implementation names these differently, adapt names but keep the behaviour; if `Cached` or offline reads cannot be pointed at a local-only store, STOP and ask.
- The catalog's ~200 set objects read from Spaces at 8 at a time finish within the 2-second success metric when warmed at start-up; the background warm exists because a cold read on the first import alone may not. Measured only by the manual check.
- TCGdex ids never need more escaping than `safeName` plus SigV4 URI encoding gives (ids seen: `sv03.5-001`, `swsh12.5-GG01`, `mep-033`).

## Drafting assumptions

### Chosen direction: a tiered Store in the existing store slot (architecture)
- **Decision**: Put a tiered `Store` (local disk, then Spaces) into `Client.Store` on the hosted instance; it copies entries from Spaces to disk with `Fetched` unchanged, writes puts to both, and absorbs every Spaces failure behind a 3 s bound and a 1-minute breaker. An optional `Local() Store` method (found by type assertion) gives `Cached` and offline mode a local-only store, so the `Store` and `DB` interfaces stay unchanged. A general stdlib SigV4 S3 client lives in `internal/spaces` with an in-process fake in `internal/spaces/spacestest`; the card-specific adapter (prefix `tcgdex/`, envelope, `safeName` paths) lives in `internal/cards`. Hosted mode alone reads four env settings; partial settings warn and run local-only; `Client.Warm` builds the catalog at hosted start-up when the shared store is on.
- **Rationale**: The dependency plan was shaped for exactly this drop-in; freshness rules stay in one place; `Cached` and offline keep their no-network guarantees; the module stays nearly dependency-free; the S3 client stays reusable by `card-image-cdn` and `admin-cache-clearing`.
- **Rejected**: a second store field and tier branching inside `Client` (spreads tier logic through the freshness code); `minio-go` / AWS SDK (heavy dependencies); whole-bucket sync at start-up (needs listing, downloads unused cards); Spaces-only on hosted (breaks local `Cached`).

### Conventions selected (architecture)
- **Decision**: Keep only `conventions/skills-text-and-packaging.md`, packaging half (Dockerfile and deploy files are edited; the build context must keep `internal/guides`).
- **Rationale**: It is the only convention in the repo store, and the Dockerfile is touched.
- **Rejected**: applying its guide-wording half (no guide text changes).

### S3 client written in-package on the standard library (discovery)
- **Decision**: A small `internal/spaces` package signs requests with AWS Signature Version 4 itself (stdlib `crypto/hmac`, `crypto/sha256`, `net/http`) and offers Get and Put now; Delete and List are left for the later specs to add on the same signer.
- **Rationale**: Two operations are needed; the module keeps three direct dependencies; the dependency plan made the same stdlib-first call for single-flight. The signer is checked against AWS's published worked example, not only against its own fake.
- **Rejected**: `minio-go/v7` (a dozen transitive modules), `aws-sdk-go-v2` (larger still, custom endpoint resolution for Spaces).

### Shared-store settings read in hosted mode only (discovery)
- **Decision**: `TCGREPLAY_SPACES_ENDPOINT`, `TCGREPLAY_SPACES_BUCKET`, `TCGREPLAY_SPACES_KEY` and `TCGREPLAY_SPACES_SECRET` are read only by hosted mode, from the environment only (no flags). Desktop, daemon and dev builds never read them.
- **Rationale**: The spec's Non-Goals put desktop and dev use out of scope and require no new settings there; secrets follow the existing env-only rule for `TCGREPLAY_GOOGLE_CLIENT_SECRET`.
- **Rejected**: reading them in every mode (would let a desktop build talk to Spaces); flags (secrets would show in process listings).

### Partial or invalid settings degrade to local-only (discovery)
- **Decision**: If some but not all four settings are set, or the endpoint is not an http(s) URL with a host, hosted mode logs a warning naming the problem and runs with the local cache only. None set: one informational log line, no warning.
- **Rationale**: The spec says shared-store problems must never stop play; the cache is optional.
- **Rejected**: refusing to start (as for missing Google credentials), which would turn a cache typo into an outage.

### Failure breaker on the shared tier (discovery)
- **Decision**: After a shared-store request fails (transport error, timeout, 5xx, 403 or any status other than 200/404 on GET, 200 on PUT), the tiered store skips the shared tier for 1 minute (`SharedRetryAfter`), logging once when it goes down and once when it is back. Each shared request is bounded at 3 seconds (`SharedTimeout`).
- **Rationale**: Without it an unreachable or slow Spaces would add a timeout to every one of ~200 catalog reads and every put, making an import take minutes. One minute is short enough that a blip does not leave a whole cold build unshared.
- **Rejected**: no breaker (slow during outages); reusing the 5-minute `RetryAfter` (a single blip during a first cold build would leave most of it out of Spaces); per-key memory (an outage is per-tier, not per-key).

### Synchronous puts to the shared store (discovery)
- **Decision**: `Tiered.Put` writes local, then the shared store, in the caller's goroutine; a shared failure is logged and trips the breaker, never returned as a lookup error.
- **Rationale**: Puts follow a TCGdex fetch, so one more round trip is cheap; tests stay deterministic.
- **Rejected**: write-behind with a background queue (needs a flush helper for tests and can lose writes on shutdown).

### Card data under the `tcgdex/` prefix, path-style addressing (discovery)
- **Decision**: Objects are `tcgdex/sets.json`, `tcgdex/sets/<safeName(id)>.json`, `tcgdex/cards/<safeName(id)>.json` (the disk store's relative paths under a fixed prefix), holding the same envelope bytes as disk, `Content-Type: application/json`, private. Requests use path-style addressing (`<endpoint>/<bucket>/<key>`).
- **Rationale**: One encoder for both tiers; the prefix keeps card data apart from the later image copies and lets admin clearing select all card data, one set's list, or one set's cards by prefix. Path-style works with an `httptest` fake and needs no per-bucket DNS.
- **Rejected**: a configurable prefix (no requirement for one bucket serving several deployments); metadata-held fetch time (contradicts the envelope rule).

### Warm the catalog at hosted start-up when the shared store is on (discovery)
- **Decision**: Add `Client.Warm(ctx) error`, which builds the catalog (set list and every set's card list) through the normal lookup path; hosted mode runs it in the background at start-up only when the shared store is configured and the client is online, logging how long it took or why it failed.
- **Rationale**: The success metric asks the first import after a restart to finish within 2 seconds of a warm import; reading ~200 set objects from Spaces 8 at a time on the first import alone may not. The warm makes that likely without changing the dependency plan's catalog or its 8-way limit.
- **Rejected**: raising the catalog's parallelism (redesigns the dependency plan's catalog); syncing the whole bucket at start-up (needs listing, downloads unused cards).

### The catalog warm and its wiring live in the hosted task (tasks)
- **Decision**: `Client.Warm` is added in the task "Use the shared store in hosted mode", and `newHosted` takes the card database as a parameter so `runHosted` can start the warm on the same client; `config` gains a test-only `cardBase` so hosted tests point TCGdex at a fake.
- **Rationale**: The warm exists only for hosted start-up; passing the client in changes two call sites; `config` already carries a test-only `now`.
- **Rejected**: returning the client from `newHosted` (widens a four-value return used by tests); calling the real TCGdex from tests.

### All four App Platform variables declared value-less as secrets (tasks)
- **Decision**: `.do/app.yaml` declares the endpoint and bucket like the key and secret: `type: SECRET`, no value, set in the console.
- **Rationale**: Their values are not known to the repo (the operator picks the bucket and region), and a placeholder value would switch the shared store on against a bucket that does not exist; this matches how `TCGREPLAY_GOOGLE_CLIENT_ID` is declared.
- **Rejected**: plain variables with placeholder values (would log failures on every deploy until edited).

### Provisioning is a human task; real-Spaces behaviour is manual checks (tasks)
- **Decision**: One `human` task creates the bucket and access key and sets the four variables; it is checked by the start-up log. Behaviour on real Spaces (warm restart, outage, 404 vs 403, 2-second first import) is in the Testing Approach as manual items, not tasks.
- **Rationale**: Creating production credentials needs the DigitalOcean control panel; reviews and checks are never tasks.
- **Rejected**: leaving provisioning implicit (the feature would ship switched off with nobody owning the switch).

### Dependency on the freshness plan stated in prose, not as a task dependency (tasks)
- **Decision**: Task "Add the Spaces card store" says in its summary that the freshness plan must have landed; the first two tasks (Spaces client) do not depend on it and can start at once.
- **Rationale**: Task dependencies must name tasks in this plan; the cross-plan ordering is carried by the plan's Dependencies section and the epic's dependency order.
- **Rejected**: blocking the Spaces client tasks on the freshness plan (they touch none of its code).

## Rehydration cues

- `spektacular spec file read 20261104082020-109dceb1-tcgdex-spaces-cache` — the spec.
- `spektacular plan file read 20261104090640-51bd1f36-card-cache-freshness plan` and `... context` — the Store contract and lookup flow this plan sits on.
- `spektacular spec file read 20261104090640-25f673ed-card-image-cdn` and `20261104090640-a5a74fab-admin-cache-clearing` — what the Spaces client must stay reusable for.
- Read `internal/cards/cards.go:160-290`, `cmd/tcgreplay/main.go:220-229`, `cmd/tcgreplay/hosted.go:100-247`, `.do/app.yaml`, `cmd/tcgreplay/hosted_test.go:380-560`.
- `spektacular knowledge read` for repo-tier `gotchas/tcgdex-cache-never-expires.md`, `gotchas/game-listeners-under-lock.md`, `architecture/importer-determinism.md`.
