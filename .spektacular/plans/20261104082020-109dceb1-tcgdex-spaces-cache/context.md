---
created_date: "2026-11-04"
document_status: final
closed_date: "2026-11-04"
---

# Context: 20261104082020-109dceb1-tcgdex-spaces-cache

## Current State Analysis

All work is in the one registered repo, `tcgreplay` (root `/home/nicj/code/github.com/nicholasjackson/tcgreplay`). This plan sits on top of the card cache freshness plan (`20261104090640-51bd1f36-card-cache-freshness`), which is planned but not yet implemented; line numbers below are today's code, and the notes say where that plan rewrites it.

- `internal/cards/cards.go:167-198` — `Client{Base, CacheDir, HTTP, Offline}` plus private maps; `NewClient(dir)` sets a 20 s HTTP client. The freshness plan adds `Store Store` (default `NewDiskStore(dir)`), `Timeout`, `Now`, and unexported `store()`/`now()` helpers.
- `internal/cards/cards.go:200-241` — `safeName` and `getJSON` (cache forever, atomic write). The freshness plan replaces `getJSON` with `loadEntry` (store, then single-flight fetch, then `Put`) and adds `internal/cards/store.go` (`Entry`, `ErrNotCached`, `Store`, disk store with the `{"fetched","data"}` envelope and legacy-file fallback).
- `internal/cards/cards.go:243-287` — `Card` and `Cached`; `Cached` has no context and is called under the game lock (`internal/game/game.go:500-515`). The freshness plan makes `Cached` read through `store().Get` only and makes `Game.card` warm misses in the background through `Card`.
- `internal/game/manager.go:204-235` — `BuildCards` resolves deck codes and loads each card through `DB.Card` before the game lock is taken: the path that will read Spaces after a restart.
- `cmd/tcgreplay/main.go:224-229` — `newCardDB(c)`, the only construction of `cards.Client`, used by desktop (`newServer`, line 217) and hosted (`newHosted`, `cmd/tcgreplay/hosted.go:172-173`).
- `cmd/tcgreplay/hosted.go:110-130` — env-only secrets (`TCGREPLAY_GOOGLE_CLIENT_SECRET`, `TCGREPLAY_SIGNING_KEY`); `cmd/tcgreplay/hosted.go:207-247` — `runHosted` builds everything, starts `reg.Run`, logs one start-up line, serves.
- `cmd/tcgreplay/hosted_test.go:55-62,260-266,384-418,452-540` — `captureLog`, `hostedConfig` (always `offline: true`), the `runHosted` lifecycle test, and the subprocess `main` tests (`-offline`, no "warning" allowed in a clean start).
- `.do/app.yaml` — one instance, no volume; env entries with `type: SECRET` and no value for console-set secrets; header comment says the card cache is lost on every deploy. `Dockerfile` (distroless static, CA certs present), `Makefile:49-63` (`docker-run` env pass-through), `TCG Review Playmat Design and Build Plan.md:302-327` (hosted mode, settings table, operator setup).
- No S3 or Spaces code exists; `go.mod` has three direct dependencies (websocket, MCP go-sdk, oauth2).

## Per-Task Technical Notes

### Task: Sign requests and read settings for Spaces

**File changes**:
- `internal/spaces/doc.go` (new) — package doc: a minimal client for DigitalOcean Spaces over its S3-compatible API (path-style, SigV4), standard library only, card-agnostic. It is shared by the card cache, and later by image copies and admin clearing.
- `internal/spaces/sign.go` (new) — unexported `sign(req *http.Request, payloadHash, accessKey, secretKey, region string, now time.Time)`. It sets `X-Amz-Date` (`20060102T150405Z`, UTC) and `X-Amz-Content-Sha256` (hex SHA-256 of the body; `e3b0c442…b855` for empty), and builds the canonical request: the method; the canonical URI (each path segment URI-encoded per RFC 3986, unreserved `A-Za-z0-9-._~` kept, `/` kept, no double encoding, which is the S3 rule); the canonical query (keys sorted, encoded); canonical headers (`host` from `req.URL.Host`, plus every `x-amz-*`, `content-type`, `content-md5`, `cache-control` and `range` header present, lower-cased, values trimmed, sorted); signed headers; and the payload hash. String to sign: `AWS4-HMAC-SHA256\n<amzdate>\n<yyyymmdd>/<region>/s3/aws4_request\n<hex sha256(canonical)>`. The signing key is the HMAC chain `"AWS4"+secret → date → region → "s3" → "aws4_request"`. It sets `Authorization: AWS4-HMAC-SHA256 Credential=<key>/<scope>, SignedHeaders=<list>, Signature=<hex>`. Also exported `VerifyRequest(r *http.Request, payload []byte, accessKey, secretKey, region string) error`, documented as for test doubles. It parses `Authorization`, returns an `*Error{Status:403, Code:"InvalidAccessKeyId"}` when the key differs, recomputes the signature from the request's own `X-Amz-Date` and signed headers, and returns `*Error{Status:403, Code:"SignatureDoesNotMatch"}` on a mismatch or a payload-hash mismatch.
- `internal/spaces/config.go` (new) — `Config{Endpoint, Region, Bucket, AccessKey, SecretKey string; HTTP *http.Client}`. Env names are constants (`EnvEndpoint = "TCGREPLAY_SPACES_ENDPOINT"`, `EnvBucket`, `EnvKey = "TCGREPLAY_SPACES_KEY"`, `EnvSecret = "TCGREPLAY_SPACES_SECRET"`). `FromEnv(getenv func(string) string) (Config, bool, error)` trims values and treats empty as unset. With none set it returns `set=false, err=nil`. With some set it returns `set=true` and an error listing the unset names (`"TCGREPLAY_SPACES_SECRET is not set"`, joined). With all set it runs `cfg.validate()`. `validate` parses the endpoint with `url.Parse`; it requires scheme `http` or `https`, a non-empty host, and no path other than `/`, no query, fragment or user info, with error text naming the setting and the endpoint value, never the key or secret. It drops a trailing slash, and derives `Region` when empty: if the hostname (port stripped) ends in `.digitaloceanspaces.com`, the first label; otherwise `us-east-1`. Also add `func (c Config) String() string` returning `<endpoint>/<bucket>` for logs.
- `internal/spaces/sign_test.go` (new, package `spaces`) — the AWS "GET Object" worked example from the S3 SigV4 documentation ("Signature Calculations for the Authorization Header: Transferring Payload in a Single Chunk"): `GET https://examplebucket.s3.amazonaws.com/test.txt`, header `Range: bytes=0-9`, empty payload, time `20130524T000000Z`, region `us-east-1`, access key `AKIAIOSFODNN7EXAMPLE`, secret `wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY`. Assert the `Authorization` header equals the document's exactly: signed headers `host;range;x-amz-content-sha256;x-amz-date`, and the signature copied from the document (at planning time `f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41`). Also cover: a key with spaces and unicode encodes per segment, with `/` kept; `VerifyRequest` accepts what `sign` produced, and rejects a changed header, a changed body and a wrong key, each with the right code.
- `internal/spaces/config_test.go` (new) — table tests on `FromEnv` with a map-backed `getenv`. Cases: none set; all set, deriving region `ams3` from `https://ams3.digitaloceanspaces.com` and `us-east-1` from `http://127.0.0.1:9000`; each single variable missing (the error names it, and `strings.Contains(err.Error(), secret)` and `…(key)` are false); bad endpoints (`ams3.digitaloceanspaces.com` without a scheme, `ftp://x`, `https://x/path`, `https://x?q=1`); and whitespace-only values treated as unset.

**Complexity**: Medium
**Token estimate**: ~25k tokens
**Agent strategy**: Single agent, sequential execution (signer and its vector test first, then settings).

### Task: Add the Spaces client and its stand-in

**File changes**:
- `internal/spaces/client.go` (new) — `New(cfg Config) (*Client, error)` validates (same rules as `FromEnv`) and defaults `HTTP` to `&http.Client{Timeout: 30 * time.Second}` (an outer bound only; callers bound each call with their context). `var ErrNotFound = errors.New("spaces: no such key")`. `type Error struct{ Status int; Code, Message string }` with `Error()` = `"spaces: <status> <code>: <message>"`. `PutOptions{ContentType, CacheControl string; Public bool}`. `objectURL(key)` = `<endpoint>/<bucket>/<encoded key>`. `Get(ctx, key)`: GET, signed with the empty-payload hash. A 200 returns the body (read through `io.LimitReader` at 32 MB; a longer body is an error). A 404 returns `ErrNotFound`, whatever the body. Anything else returns `*Error` parsed from the S3 XML body (`<Error><Code/><Message/></Error>` via `encoding/xml`; when the body does not parse, `Code` is `http.StatusText`). `Put(ctx, key, body, o)`: PUT with `Content-Type`, `Cache-Control` if set, `x-amz-acl: public-read` when `o.Public`, `Content-Length`, signed with the body's SHA-256; 200 is success, anything else `*Error`. Transport errors are returned wrapped with the operation and key (`spaces: get tcgdex/sets.json: …`). Accessors `Bucket()` and `Endpoint()` for logs and later specs.
- `internal/spaces/spacestest/spacestest.go` (new) — package doc mirroring `internal/cards/cardstest/cardstest.go:1-3`. `type Server struct` wrapping `httptest.NewServer`, with fields under a mutex: `objects map[string]object{body []byte; header http.Header}`, `counts map[string]int` keyed `METHOD key`, `fail`, `deny` flags, and a `hang chan struct{}`. `New(t testing.TB) *Server` registers `t.Cleanup(s.Close)` and generates a bucket `test-bucket`, key `TESTKEY`, secret `testsecret`, region `us-east-1`. `Config() spaces.Config` points at `s.URL`. The handler splits `/<bucket>/<key>` (404 `NoSuchBucket` for another bucket), counts the request, then applies: `hang` (block until released or the request context ends), `fail` (500 `InternalError`), `deny` (403 `InvalidAccessKeyId`), and `spaces.VerifyRequest` with the real secret (403 from its error), before GET (200 or 404 `NoSuchKey`) or PUT (store body plus `Content-Type`, `Cache-Control`, `X-Amz-Acl`; 200). Other methods get 405 `MethodNotAllowed`. Error bodies are S3 XML. Methods: `Object(key) ([]byte, bool)`, `Header(key) http.Header`, `SetObject(key, body)`, `Requests(method, key) int`, `Total() int`, `Reset()` (counts only), `SetFail(bool)`, `SetDeny(bool)`, `Hang() (release func())`, and `Close()` (idempotent, after which the endpoint is unreachable).
- `internal/spaces/client_test.go` (new, package `spaces_test`, since `spacestest` imports `spaces`) — cases: a round trip with `PutOptions{ContentType:"application/json", CacheControl:"max-age=60", Public:true}` reads back the same bytes, and `Header` shows all three; a missing key gives `errors.Is(err, spaces.ErrNotFound)`; `SetDeny(true)` gives `errors.As(err, &e)` with `e.Status == 403` and `e.Code == "InvalidAccessKeyId"`; a client built with the wrong secret gets `SignatureDoesNotMatch`; `SetFail(true)` gives status 500; after `Close()` the error is non-nil and not `*Error`; with `Hang()` and a 100 ms context the call returns within 1 s with a context error; and keys with `/`, `.` and spaces round-trip.

**Complexity**: Medium
**Token estimate**: ~35k tokens
**Agent strategy**: 2 parallel agents: one on the client, one on the stand-in, both against the signer from the previous task; integrate with the client tests.

### Task: Add the Spaces card store

**File changes**:
- `internal/cards/store.go` (from the freshness plan's task "Add the cache store with fetch times") — if the envelope encoding and the key-to-path mapping are inline in the disk store, factor them into unexported helpers without changing behaviour or file paths: `keyPath(key string) (string, error)` → `sets.json`, `sets/<safeName(id)>.json`, `cards/<safeName(id)>.json` (an unknown prefix is an error); `encodeEnvelope(Entry) ([]byte, error)` (RFC 3339 nano, UTC); `decodeEnvelope([]byte) (Entry, bool)` (true only when both `fetched` and `data` are present). The disk store keeps its legacy-file fallback; the Spaces store does not use it.
- `internal/cards/spacesstore.go` (new) — `const spacesPrefix = "tcgdex/"`. `type spacesStore struct{ c *spaces.Client }`, with `NewSpacesStore(c *spaces.Client) Store`. `Get`: `p, err := keyPath(key)`, then `b, err := s.c.Get(ctx, spacesPrefix+p)`. `errors.Is(err, spaces.ErrNotFound)` gives `fmt.Errorf("%s: %w", key, ErrNotCached)`. Any other error is returned wrapped. If `decodeEnvelope` fails, return `fmt.Errorf("%s: shared copy is not an envelope: %w", key, ErrNotCached)`, so it is refetched and overwritten. `Put`: encode, then `s.c.Put(ctx, spacesPrefix+p, b, spaces.PutOptions{ContentType: "application/json"})` (private, no cache-control). No timeout here; the tiered store bounds calls.
- `internal/cards/spacesstore_test.go` (new, package `cards`) — uses `spacestest.New(t)`. Cases: a round trip with a `Fetched` at nanosecond precision compares `time.Equal` and body bytes; object keys are exactly `tcgdex/sets.json`, `tcgdex/sets/sv03.5.json` and `tcgdex/cards/sv03.5-001.json`; for the same `Entry`, the object bytes equal the file `NewDiskStore(dir)` writes; a missing key gives `errors.Is(err, ErrNotCached)`; an object seeded with a bare TCGdex body (no envelope) gives `ErrNotCached`; `SetDeny(true)` gives an error that is not `ErrNotCached`; and the object's `Content-Type` is `application/json` with no `X-Amz-Acl`.

**Complexity**: Low
**Token estimate**: ~20k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Add the tiered store with a failure breaker

**File changes**:
- `internal/cards/tiered.go` (new) — `const SharedTimeout = 3 * time.Second; const SharedRetryAfter = time.Minute`. `type tieredStore struct{ local, shared Store; timeout time.Duration; retry time.Duration; now func() time.Time; logf func(string, ...any); mu sync.Mutex; down bool; until time.Time }`. `NewTieredStore(local, shared Store) Store` sets `timeout: SharedTimeout`, `retry: SharedRetryAfter`, `now: time.Now`, `logf: log.Printf` (tests override the unexported fields). `Local() Store` returns `local`. `skip()` is true under `mu` when `down` and `now()` is before `until`. `failed(op, key, err)`: under `mu`, if not already `down`, log `card cache: shared store unavailable (%s %s: %v); using the local cache and the card source, retrying in %s`; then set `down=true` and `until=now()+retry` (re-arming the window on a failed probe, without logging again). `ok()`: under `mu`, if `down`, log `card cache: shared store reachable again` and clear it. `Get(ctx, key)`: local `Get`; on success return it. On any local error, treat it as a miss. If `skip()`, return `ErrNotCached`. Otherwise call `shared.Get` with `context.WithTimeout(ctx, timeout)`: on `ErrNotCached` call `ok()` and return `ErrNotCached`; on another error call `failed` and return `fmt.Errorf("%s: %w", key, ErrNotCached)`; on success call `ok()`, then `local.Put(ctx, key, e)` (error ignored, as the disk store's write failures are today), and return `e` unchanged (same `Fetched`). `Put(ctx, key, e)`: `lerr := local.Put(...)`; if not `skip()`, call `shared.Put` with the timeout, then `failed` or `ok`. Return `lerr` (shared errors are never returned). A doc comment states the contract: no expiry, `Fetched` preserved, shared failures absorbed.
- `internal/cards/tiered_test.go` (new) — builds `NewTieredStore(NewDiskStore(t.TempDir()), NewSpacesStore(spaces client on spacestest))`, type-asserted to `*tieredStore` to inject a fake `now`, `timeout = 200ms` and a `logf` that appends to a slice. Cases: a shared-only entry is returned with an equal `Fetched` and is then readable from the local store with an equal `Fetched`; a local hit makes `Total()==0`; `Put` puts the entry in both; for each failure (`Close`, `SetDeny`, `SetFail`, `Hang`), `Get` returns `ErrNotCached` (a `Hang` within ~200 ms plus slack, well under 1 s) and `Put` returns nil with the entry on disk; exactly one "unavailable" log line follows; a second `Get` and `Put` inside the window add 0 requests; after `now` is advanced 61 s, the next `Get` makes 1 request; with the store healthy again, one "reachable again" line is logged and later requests are not logged; a failed probe re-arms with no second "unavailable" line; and `Local()` returns the local store.

**Complexity**: Medium
**Token estimate**: ~30k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Keep game-lock and offline reads on local disk

**File changes**:
- `internal/cards/cards.go` — next to the freshness plan's `store()` helper (from its task "Route card lookups through the store", `Client` fields at `internal/cards/cards.go:167-190` today), add `localStore() Store`: if `c.store()` implements the unexported `interface{ Local() Store }`, return its `Local()`, otherwise `c.store()`. Comment it: `Cached` runs under the game lock, and offline mode must not touch the network, so both read only the local tier.
- `internal/cards/cards.go` (`Cached`, today `internal/cards/cards.go:266-287`, rewritten by the freshness plan to use `store().Get`) — use `localStore().Get`.
- `internal/cards/cards.go` (`loadEntry`, which replaces `getJSON` at `internal/cards/cards.go:204-241` in the freshness plan) — when `c.Offline`, read through `localStore()` (and never fetch, as already planned). Online, keep `store()`, so the tiered store is used. The catalog build and the background refresh go through `loadEntry`/`store()`, so they inherit this with no change.
- `internal/cards/tiered_client_test.go` (new) — uses the freshness plan's `fakeTCGdex`/`newTestClient` helpers, with `c.Store = NewTieredStore(NewDiskStore(c.CacheDir), NewSpacesStore(...))`. Cases: a card seeded only in the fake Spaces gives `Cached(id) == nil` and `Total()==0`; then `Card(ctx,id)` returns it, with fake TCGdex `hits("/cards/<id>")==0`, and the file `cards/<id>.json` exists under `CacheDir`, after which `Cached(id)` is non-nil; an offline client with an empty disk and the fake Spaces holding `sets`, `sets/tst` and `cards/tst-1` fails `Card`, `Resolve("TST","1")` and `ByName("Riolu")` as not cached/offline, with `Total()==0` on the fake Spaces; and a client with no tiered store behaves as before (`byname_test.go` and the freshness plan's tests pass unchanged).

**Complexity**: Low
**Token estimate**: ~15k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Prove a restart keeps the card cache

**File changes**:
- `internal/cards/restart_test.go` (new, package `cards`) — the spec's acceptance criteria at client level, using the freshness plan's `fakeTCGdex` (per-path `hits`, `close`, fake clock, `waitBackground`) and `spacestest`. Helper `restartPair(t)` returns a fake TCGdex seeded with a set list (`tst`, `sve`), set details and cards; one shared `spacestest.Server`; and a constructor for a client on a fresh `t.TempDir()` with the tiered store over that server. Cases:
  - **Warm restart:** client A runs `Resolve` for each code in a small deck, `Card` for each id and `ByName` for one name. Client B (new dir) does the same after `f.reset()`. `hits` is 0 for `/sets`, `/sets/tst`, `/sets/sve` and every `/cards/<id>`, and the ids and decoded cards are equal.
  - **Fills local:** card X is seeded only in Spaces (an envelope with a recent `fetched`). `Card(X)` makes 0 TCGdex hits, and `cards/X.json` then exists in the dir with the same `fetched`. Card Y is in neither: 1 TCGdex hit, then it exists in the dir and as `tcgdex/cards/Y.json`.
  - **Stale stays stale:** Spaces holds an imageless card envelope fetched 25 h before the fake clock's now (and the fake TCGdex now has an image). A new client's `Card` returns the imageless copy at once. The disk copy carries the old `fetched`. After `waitBackground`, there is 1 TCGdex hit, and both the Spaces object and the disk file hold the new `fetched` and the image. Likewise a set seeded 91 days old is refreshed once in the background after `Resolve`.
  - **Outage:** for each of `Close`, `SetDeny`, `SetFail` and `Hang` (with the tiered store's `timeout` lowered to 200 ms), a new client resolves the deck, loads cards and runs `ByName` with no error, from the fake TCGdex. Captured log output contains one "shared store unavailable" line.
  - **Offline:** an offline client with the tiered store and a disk seeded with the deck's entries resolves everything, with `Total()==0` on the fake Spaces.
- `internal/game/spacescache_test.go` (new, package `game`) — a game-level warm restart with the real `cards.Client`. An inline `httptest` fake TCGdex (a map of path to JSON body, per-path hit counter) serves `/sets` (`tst` with abbreviation `TST`, and `sve`), `/sets/tst` (cards 1-6), `/sets/sve` (`sve-006`) and `/cards/<id>` for the cards in `testDeck` (`internal/game/game_test.go:18-31`). It reuses the fixture bodies from the freshness plan's `internal/game/cardsource_test.go` if that file exposes helpers; otherwise it defines them inline. Client A has dir A, `Base` set to the fake, and `Store` set to `cards.NewTieredStore(cards.NewDiskStore(dirA), cards.NewSpacesStore(spacesClient))`. `BuildCards(ctx, clientA, {p1: testDeck, p2: testDeck})` (`internal/game/manager.go:204-235`) runs, then the counters reset. Client B (dir B, empty, same Spaces) runs `BuildCards` again. The two `[]state.Card` results are `reflect.DeepEqual`, the warnings are equal, and fake TCGdex hits are 0.

**Complexity**: Medium
**Token estimate**: ~40k tokens
**Agent strategy**: 2 parallel agents: one on the card-package acceptance tests, one on the game-level test; both run under the race detector at the end.

### Task: Use the shared store in hosted mode

**File changes**:
- `internal/cards/warm.go` (new) — `func (c *Client) Warm(ctx context.Context) error { _, err := c.ensureCatalog(ctx); return err }`. `ensureCatalog` comes from the freshness plan's task "Build the card catalog outside the database lock". Its doc comment says that it builds the set list and every set's card list through the normal lookup path, that it is for start-up, and that it is not part of `DB`.
- `internal/cards/warm_test.go` (new) — with the fake TCGdex and the tiered store, `Warm` loads `sets` and each set: `hits("/sets/<id>")==1` each, and `tcgdex/sets.json` plus the `tcgdex/sets/<id>.json` objects exist in the fake Spaces. A second `Warm` makes 0 hits.
- `cmd/tcgreplay/main.go:40-62` — `config` gains `cardBase string // if set, the TCGdex base URL (tests only)`, beside the existing test-only `now`.
- `cmd/tcgreplay/hosted.go` — new `hostedCardDB(c config) (db *cards.Client, shared bool)`. It does `db = newCardDB(c)` (`cmd/tcgreplay/main.go:225-229`, unchanged), sets `db.Base = c.cardBase` when it is non-empty, and calls `cfg, set, err := spaces.FromEnv(os.Getenv)`. When `!set`, it logs `card cache: local only, in <cacheDir> (no TCGREPLAY_SPACES_* settings)`. When `err != nil`, it logs `warning: card cache: <err>; using the local cache only`. When `c.offline`, it logs `card cache: offline, shared store <cfg> not used`. Otherwise it calls `spaces.New(cfg)` (an error is a warning, as above), sets `db.Store = cards.NewTieredStore(db.Store, cards.NewSpacesStore(sc))`, logs `card cache: <cacheDir> backed by shared store <cfg>`, and sets `shared = true`. Also new `warmCatalog(ctx, db)`: it records the start time, calls `db.Warm` under `context.WithTimeout(ctx, 2*time.Minute)`, and logs `card cache: catalog warmed in <d>` or `card cache: catalog warm failed after <d>: <err>`.
- `cmd/tcgreplay/hosted.go:172-175` — `newHosted` takes the card database as a parameter (`db *cards.Client`) instead of calling `newCardDB(c)` itself.
- `cmd/tcgreplay/hosted.go:207-229` — `runHosted` calls `db, shared := hostedCardDB(c)` before `newHosted(c, db, public, …)`, and after `go reg.Run(ctx)` runs `if shared { go warmCatalog(ctx, db) }`. The start-up log line at line 229 is unchanged; the card cache line is logged by `hostedCardDB`.
- `cmd/tcgreplay/hosted_test.go:662` — pass `newCardDB(c)` to `newHosted`.
- `cmd/tcgreplay/hosted_cache_test.go` (new) — a `clearSpacesEnv(t)` helper sets all four variables to `""` with `t.Setenv`. A small inline fake TCGdex (or `httptest` serving `/cards/x`, `/sets`, `/sets/tst`, with an optional gate on `/sets/tst`). Cases, calling `hostedCardDB` with `offline: false` and `cardBase` set to the fake:
  - no settings: `shared == false`, `Card(ctx,"x")` writes `cards/x.json` under the cache dir, and the log contains "local only" but neither "warning" nor "error";
  - all four set from `spacestest.Server.Config()`: `shared == true`, after `Card` the fake Spaces holds `tcgdex/cards/x.json`, and the log names the bucket;
  - each single variable missing, and a bad endpoint: `shared == false`, the log contains "warning" and the missing name, does not contain the secret value, and the card is still served;
  - `SetDeny(true)`: `shared == true`, `Card` succeeds, and the log contains "shared store unavailable";
  - `offline: true` with all four set: `shared == false`, `Card` and `Cached` on a seeded disk make `Total()==0`, and the log says "not used".
  Plus a `runHosted` test built like `TestRunHostedServesAndShutsDown` (`cmd/tcgreplay/hosted_test.go:384-418`) with `offline: false`, `cardBase` set to the gated fake and all four set: `/healthz` answers while the gate is held; after the gate is released, the fake Spaces eventually holds `tcgdex/sets.json` and the log contains "catalog warmed" (poll with a deadline).
  Desktop paths: a test that `newServer` (`cmd/tcgreplay/main.go:217-222`) with all four set and `offline: false` builds a client whose `Store` is not tiered (no `Local()` method), proving desktop never reads the settings.
- `cmd/tcgreplay/hosted_test.go:499-540` (`TestMainHostedFollowsEnvAndStopsOnSIGTERM`) — unchanged; it must still pass, since its helper runs `-offline` with no Spaces settings and the "local only" line holds no "warning".

**Complexity**: Medium
**Token estimate**: ~40k tokens
**Agent strategy**: 2 parallel agents: one on `Warm` and its test in `internal/cards`, one on the `cmd/tcgreplay` wiring and tests; integrate, then run the full suite.

### Task: Declare and document the shared store settings

**File changes**:
- `.do/app.yaml:7-11` — header comment: games and sign-ins are still lost on a deploy, and the card cache is kept in Spaces when the `TCGREPLAY_SPACES_*` settings are set (otherwise it is lost too).
- `.do/app.yaml:31-68` (`envs`) — add four entries after `TCGREPLAY_IDLE_TIMEOUT`, each `scope: RUN_TIME`, `type: SECRET`, no `value` (set in the console, like `TCGREPLAY_GOOGLE_CLIENT_ID`): `TCGREPLAY_SPACES_ENDPOINT`, `TCGREPLAY_SPACES_BUCKET`, `TCGREPLAY_SPACES_KEY` and `TCGREPLAY_SPACES_SECRET`. A comment explains the endpoint (`https://<region>.digitaloceanspaces.com`), the private bucket, a key limited to that bucket with read/write, that card data goes under `tcgdex/`, and that leaving all four unset keeps a local-only cache. It notes that the endpoint and bucket are not secret, but are declared value-less so they are set in the console with the key and secret.
- `Dockerfile:1-6` — header comment's settings list gains the four `TCGREPLAY_SPACES_*` variables (optional: keep the card cache across deploys). No build step or context change; `.dockerignore` is untouched.
- `Makefile:49-63` — the `docker-run` comment mentions the optional shared-store variables, and the command passes `-e TCGREPLAY_SPACES_ENDPOINT -e TCGREPLAY_SPACES_BUCKET -e TCGREPLAY_SPACES_KEY -e TCGREPLAY_SPACES_SECRET`.
- `TCG Review Playmat Design and Build Plan.md:302` — "Trial limits" sentence: every account's game is lost on a deploy or restart. The card cache is too, unless the shared store is configured, in which case it is kept in Spaces and only local disk is a per-run copy.
- `TCG Review Playmat Design and Build Plan.md:311-318` — the settings table gains four rows, all "environment only". `TCGREPLAY_SPACES_ENDPOINT`, `TCGREPLAY_SPACES_BUCKET`, `TCGREPLAY_SPACES_KEY` and `TCGREPLAY_SPACES_SECRET` each default to "none: card cache kept on local disk only (lost on restart); some but not all set → warning, local only".
- `TCG Review Playmat Design and Build Plan.md:322-327` — the one-time operator setup gains a step: create a Spaces bucket (private, file listing restricted, in a region near the app), create a Spaces access key limited to that bucket with read/write, and set the four variables encrypted on the component. Note that `doctl apps update` does not carry secret values, and that clearing the cached card data means deleting the bucket's `tcgdex/` objects.

**Complexity**: Low
**Token estimate**: ~12k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Update the card cache knowledge entry

**File changes**:
- `gotchas/tcgdex-cache-never-expires.md` (knowledge entry; repo tier, store `tcgreplay`; as rewritten, and possibly retitled, by the freshness plan's task "Update the card cache knowledge entries"; written only through the `spek-knowledge` skill, never by path) — add a "Hosted shared tier" part covering these points. With `TCGREPLAY_SPACES_*` set, hosted mode keeps every entry in a Spaces bucket under `tcgdex/`, at the disk path and in the same envelope. Local disk is a per-run copy filled from Spaces with the original `fetched`, so ages survive restarts. `Cached` (under the game lock) and offline mode never read Spaces, so a Spaces-only card shows by name until the background warm loads it. A Spaces failure is logged once and the tier is skipped for 1 minute, with nothing queued, so entries fetched meanwhile reach Spaces only on their next fetch. Forcing fresh data on hosted means deleting the `tcgdex/` objects as well as restarting, until admin cache clearing exists. Desktop and dev never use Spaces. Update the entry's tags (add `spaces`, `hosted`).

**Complexity**: Low
**Token estimate**: ~8k tokens
**Agent strategy**: Single agent, sequential execution, through the `spek-knowledge` skill.

### Task: Create the Spaces bucket and set its secrets

**What the person does** (in the DigitalOcean control panel; no repo files change):
1. Create a Spaces bucket for the hosted playmat in a region close to the app (App Platform region `lon` in `.do/app.yaml`; choose the nearest region offering Spaces). Keep file listing restricted (private). The CDN is not needed for this spec.
2. Create a Spaces access key with access limited to that bucket, read/write (DigitalOcean "Limited access" keys), and note the key id and secret.
3. On the `playmat` component of the App Platform app, set `TCGREPLAY_SPACES_ENDPOINT` (`https://<region>.digitaloceanspaces.com`), `TCGREPLAY_SPACES_BUCKET`, `TCGREPLAY_SPACES_KEY` and `TCGREPLAY_SPACES_SECRET`, all encrypted, then save to redeploy.

**How the result is checked**: The runtime log of the new deployment shows `card cache: … backed by shared store https://<region>.digitaloceanspaces.com/<bucket>`, then `card cache: catalog warmed in …`, with no `warning: card cache` line and no "shared store unavailable" line. The bucket then holds `tcgdex/sets.json` and `tcgdex/sets/*.json` objects. The behavioural checks on real Spaces are the manual items in the implementation test plan.

**Complexity**: Low
**Token estimate**: n/a (human task)


## Testing Strategy

Go tests throughout, run with `go test ./...` and under `-race` (the freshness plan's concurrency is exercised through the new tier). `make test` also runs `go vet` and the web e2e suite, which this plan does not change. No test uses real Spaces or real TCGdex. Per task:

- **Sign requests and read settings for Spaces** — the AWS published SigV4 GET Object example reproduced byte for byte; per-segment URI encoding; `VerifyRequest` accepting signed requests and rejecting altered ones with the right code; `FromEnv` for none, complete, each missing variable, bad endpoints and whitespace, never echoing secrets.
- **Add the Spaces client and its stand-in** — external-package tests against `spacestest`: round trip with put options recorded; 404 → `ErrNotFound`; 403 `InvalidAccessKeyId` and `SignatureDoesNotMatch`; 500; closed endpoint; hang bounded by the caller's context; awkward keys.
- **Add the Spaces card store** — envelope round trip with exact `Fetched`; object names under `tcgdex/`; bytes identical to the disk store's file; missing and non-envelope objects → `ErrNotCached`; refusals are errors, not misses; content type set, no public ACL.
- **Add the tiered store with a failure breaker** — copy-down keeping `Fetched`; local hits make no shared requests; write-through; each failure mode absorbed with writes still on disk; hang bounded by the store's timeout; one-minute skip window on an injected clock; probe and recovery; edge-triggered logging.
- **Keep game-lock and offline reads on local disk** — `Cached` never touches Spaces; offline `Card`/`Resolve`/`ByName` make zero Spaces requests; online `Card` of a Spaces-only card fills disk without TCGdex; existing tests unchanged.
- **Prove a restart keeps the card cache** — the spec's acceptance criteria at client level (warm restart with zero TCGdex hits and equal answers; fills local; new card in both; stale stays stale for a card and a set; every outage mode; offline) and at game level (`BuildCards` after a restart gives an identical card list with zero TCGdex hits). This is also the behavioural test for the first success metric.
- **Use the shared store in hosted mode** — `Warm` loads the catalog once; `hostedCardDB` for none, complete, each partial, bad endpoint, wrong credentials and offline, asserting the store kind (by behaviour) and the log lines; `runHosted` answers `/healthz` while a gated warm runs, then logs "catalog warmed"; the desktop `newServer` never builds the tiered store; existing hosted tests pass unchanged.
- **Declare and document the shared store settings** — no tests; checked by reading the files, and by the Docker image still building (guides embedded).
- **Update the card cache knowledge entry** — no tests; checked by reading the entry.
- **Create the Spaces bucket and set its secrets** — checked by the deployment's start-up log.

Manual items for the implementation test plan:
- Success metric: the first import after a restart (other than the first deploy) finishes within 2 seconds of a warm-cache import of the same game. **Manual — captured in the implementation test plan.**
- Real Spaces end to end: import, redeploy, import again with no card-data requests to TCGdex; `tcgdex/` objects in envelope format; start-up log names the shared store. **Manual — captured in the implementation test plan.**
- Real Spaces failure modes: wrong secret and unreachable endpoint each still start, import and show cards, with the failure logged once; a missing key on the real bucket is a 404 miss, not a breaker trip. **Manual — captured in the implementation test plan.**
- No settings: hosted start logs one "local only" line with no warning; desktop unchanged. **Manual — captured in the implementation test plan.**

## Project References

- Spec: `20261104082020-109dceb1-tcgdex-spaces-cache` (read with `spektacular spec file read`).
- Epic: `20261104082020-109dceb1-tcgdex-spaces-cache`.
- Upstream plan: `20261104090640-51bd1f36-card-cache-freshness` (must land first; read with `spektacular plan file read … plan` and `… context`).
- Downstream specs: `20261104090640-25f673ed-card-image-cdn`, `20261104090640-a5a74fab-admin-cache-clearing` (reuse `internal/spaces` and `spacestest`).
- Design documents: none referenced by the spec.
- Knowledge (repo tier, store `tcgreplay`): `gotchas/tcgdex-cache-never-expires.md`, `gotchas/game-listeners-under-lock.md`, `architecture/importer-determinism.md`, `conventions/skills-text-and-packaging.md`.
- Repo root: `tcgreplay` → `/home/nicj/code/github.com/nicholasjackson/tcgreplay` (Go server in `internal/`, `cmd/tcgreplay`; board in `web/`; deploy in `.do/app.yaml`, `Dockerfile`).

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

Per task: signer and settings ~25k (Medium); client and stand-in ~35k (Medium); Spaces card store ~20k (Low); tiered store ~30k (Medium); local-only reads ~15k (Low); restart acceptance tests ~40k (Medium); hosted wiring and warm ~40k (Medium); docs and deploy ~12k (Low); knowledge entry ~8k (Low); bucket and secrets: human.

## Migration Notes

- No data migration. The bucket starts empty; the first deploy with the settings fetches every set from TCGdex once (on the start-up warm) and fills it. Later deploys read from it.
- Objects use the same envelope as disk, so there is no legacy format in the bucket. A non-envelope object (for example one uploaded by hand) is treated as not cached and overwritten on the next fetch.
- Turning the feature off is removing the four variables; the hosted playmat goes back to a per-run local cache. The bucket can then be deleted. Turning it back on reuses whatever the bucket still holds, with its original fetch times, so stale entries are refreshed as usual.
- Desktop caches are untouched.
- Operator steps (bucket, key, four variables) are the human task "Create the Spaces bucket and set its secrets"; `doctl apps update` does not carry secret values, so they are set in the console.

## Performance Considerations

- A warm hosted restart replaces ~200 TCGdex set fetches (and every card fetch) with Spaces GETs in the same cloud, 8 at a time through the freshness plan's catalog build. The start-up warm moves that work ahead of the first import; the 2-second success metric is checked manually because it depends on real Spaces latency.
- Each TCGdex fetch now costs one extra synchronous PUT to Spaces (bounded at 3 s). Fetches are already the slow path, and the breaker removes the cost during an outage.
- During a Spaces outage the cost is at most one bounded attempt (3 s worst case for a hang) per minute across the whole process, not per lookup; all other lookups go straight to disk and TCGdex.
- `Cached` and offline mode never pay any shared-store cost; board reads under the game lock are unchanged.
- Spaces requests use the default HTTP transport (connection reuse, HTTP keep-alive); no extra goroutines are started apart from the one start-up warm.
