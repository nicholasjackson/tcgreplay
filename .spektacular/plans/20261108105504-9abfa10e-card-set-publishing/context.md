---
created_date: "2026-11-08"
document_status: final
closed_date: "2026-11-08"
---

# Context: 20261108105504-9abfa10e-card-set-publishing

## Current State Analysis

- One repo, `tcgreplay` (`/home/nicj/code/github.com/nicholasjackson/tcgreplay`). Every task lands there.
- Card data today is a read-through cache over TCGdex: `cards.Client` (`internal/cards/cards.go:168-214`) with stale-while-revalidate refresh ages (`fresh.go:11-24`), a disk store (`store.go`), a hosted Spaces tier under `tcgdex/` (`spacesstore.go`, `tiered.go`), single-flight (`flight.go`), failure memory, clear generations and "cleared" markers (`clear.go:74-115`), and a pure catalogue index (`catalog.go:180-241`). Hosted mode warms the catalogue at start (`cmd/tcgreplay/hosted.go:358-372`). Knowledge `gotchas/tcgdex-cache-never-expires.md` describes it in full.
- The rest of the program uses the `cards.DB` interface (`cards.go:152-165`). Games hold only def ids (`internal/state/types.go:39-48`) and look every card up through `g.db` (`internal/game/game.go:502-543`), set once in `game.Load` (`game.go:115`). `game.Manager.DB` is shared by `card_text` (`internal/mcpserver/server.go:583-601`), `/api/card` and `borrowedArt` (`internal/webui/server.go:107-225`), deck parsing and import resolution (`internal/game/import.go:59-185`, `manager.go:204-261`). Nothing about a game is persisted; a record is text and a reopen re-resolves (`game/record.go:33-50`).
- The hosted image cache (`internal/cards/images.go`) copies TCGdex images (`low.webp`, `high.webp`) to `images/` in the bucket, versions them by ETag, and passes any other address through unchanged. The board appends `/low.webp` or `/high.webp` to `image` (`web/src/lib/store.svelte.js:168-177`).
- `/admin` (`internal/cacheadmin/`) clears card data and images together, everything or one set.
- Deploys: App Platform redeploys on every push to `main` (`.do/app.yaml:28-35`) and drops every in-memory game. There is no `.github/` directory.
- The card-set-import plan (must land first) adds `internal/cardset` (format, `Print.Card`, `Check`), `tcgreplay sets import|check`, and `cardsets/{sets,corrections,reports}/` for every English set, outside `internal/` and excluded from the image build.

## Per-Task Technical Notes

### Task: Card set publisher

Requirements carried: Publish sets; A publish reaches every viewer (manifest freshness). Repo: `tcgreplay`. Builds on the card-set-import plan's `internal/cardset` (`format.go` `Set`/`Read`, `check.go` `Check(dir, sets...)`); must land after them.

- `internal/cardset/manifest.go` (new): `Manifest{Format int; Sets map[string]ManifestSet}` and `ManifestSet{Path, SHA256 string; Prints, Format int}`. Constant `ManifestFormat = 1`. `ManifestKey = "manifest.json"`, `SetKey(id, sum) = "sets/<id>/<sum[:16]>.json.gz"`. `EncodeManifest` produces deterministic bytes (two-space indent, sorted keys through `map` marshalling, trailing newline). `DecodeManifest` rejects an unknown top-level `format` with a clear error. The loader reads this file too, so it is the shared contract.
- `internal/cardset/publish.go` (new): `PublishTarget` (`Get`, `Put(ctx, key, body, contentType, cacheControl)`, `List`), `PublishOptions{Dir, Prefix ("cardsets/"), Target, Purge, DryRun, Logf}`, `PublishResult`, `Publish(ctx, opts)`. Steps:
  1. `Check(opts.Dir)`. Any open item returns an error listing them, with no upload.
  2. Read every `<Dir>/sets/*.json` as raw bytes plus `Read` for `Format` and print count. `sum := sha256(raw)` (hash the committed bytes exactly; the built files are deterministic per the card-set-import plan).
  3. `Get(prefix+"manifest.json")`. A not-found means an empty manifest. Any set in the remote manifest missing locally returns an error naming the set ids, with no upload.
  4. For each set whose `SHA256` differs (or is absent), gzip `raw` with `gzip.NewWriterLevel(BestCompression)` and a zero header ModTime, so the output is deterministic. `Put(prefix+SetKey, gz, "application/gzip", "public, max-age=31536000, immutable")`. Skip the Put when `List(prefix+"sets/<id>/")` already holds the key, which happens after an interrupted publish.
  5. Build the new manifest. If its bytes equal the remote bytes, stop (`ManifestChanged=false`). Otherwise `Put(prefix+"manifest.json", bytes, "application/json", "public, max-age=60")`, then `Purge(ctx, []string{prefix+"manifest.json"})` if set, else log `card sets: no CDN token; the manifest may be cached up to the CDN's TTL`. A purge failure is a returned error after the manifest write, so the maintainer sees it.
  6. `DryRun` performs steps 1-3 and reports what would be uploaded.
  - Never `Delete`, and never `Put` an existing set key.
- `internal/cardset/target.go` (new): `SpacesTarget{C *spaces.Client}` maps to `spaces.Client.Get/Put(PutOptions{ContentType, CacheControl, Public: true})/List` (`internal/spaces/client.go:74,99,168`). It maps `spaces.ErrNotFound` to `ErrNotFound` and pages `List` like `listAll` (`internal/cards/spacesstore.go:100-116`). `DirTarget{Dir string}` writes files under `Dir/<key>` (mkdir, write to a temp file then rename) and ignores cache headers. It is used by tests, by `-to`, and by the loader's local-directory base in development.
- `internal/cardset/publish_test.go` (new): uses `spacestest.New(t)` (`internal/spaces/spacestest/spacestest.go:72`) and `spacestest.NewCDN` (`cdn.go:32`).
  - First publish puts every set plus the manifest. `Header(key)` shows the content type and cache control, and `Public(key)` is true.
  - Change one built set, publish again: one new set key plus the manifest are put (`Requests("PUT", …)`), and the old key's bytes are unchanged (`Object`).
  - An unchanged publish makes no PUT.
  - An open check item and a missing published set each return an error with zero PUTs.
  - The purge is recorded (`Purges()`). With no purge, a warning is logged.
  - The gzipped object decompresses to the committed bytes.
  - `DirTarget` publish then decode is equal.
  - Fixtures: two small built sets written with the card-set-import plan's `Write` in a temp `cardsets/`.

Complexity: Medium. Token estimate: ~30k. Agent strategy: single agent, sequential.

### Task: Sets publish command

Requirements carried: Publish sets. Repo: `tcgreplay`.

- `cmd/tcgreplay/sets.go` (from the card-set-import plan, dispatch on `os.Args[1] == "sets"` before `flag.Parse`, `cmd/tcgreplay/main.go:126-171`): add the `publish` verb. It has its own `flag.FlagSet` with `-dir` (default `cardsets`), `-to <directory>` and `-dry-run`.
  - Without `-to`: `spaces.FromEnv(os.Getenv)` (`internal/spaces/config.go:63`). Not set: exit 2 with "set TCGREPLAY_SPACES_ENDPOINT, _BUCKET, _KEY and _SECRET, or use -to". Partially set: print its joined error and exit 2.
  - `TCGREPLAY_SPACES_CDN_TOKEN` set: `Purge` uses `spaces.NewCDN(cfg, token).Purge` (`internal/spaces/cdn.go:56,73`).
  - Prints `uploaded <id> (<prints> prints)` and `unchanged: N sets`, plus `manifest updated` or `manifest unchanged`.
  - Exits 1 on a publish error (open items, missing sets, upload or purge failure).
- `cmd/tcgreplay/main.go` usage text: list `sets publish` with the other `sets` verbs.
- `cmd/tcgreplay/sets_test.go` (from the card-set-import plan; extend). Uses the re-exec helper `runMain` (`cmd/tcgreplay/main_test.go:17-52`):
  - `sets publish -to <tmp>` writes the manifest and set files and exits 0.
  - With env pointing at `spacestest` (`Config()` gives the endpoint): publishes, then a second run reports unchanged.
  - Partial env: exits 2 with the message.
  - `-dry-run` writes nothing.
  - `TestUnknownCommandRefused` still passes.

Complexity: Low. Token estimate: ~15k. Agent strategy: single agent, sequential.

### Task: Publish on merge workflow

Requirements carried: Publish sets (who publishes); Published changes picked up while running (time from merge). Repo: `tcgreplay`.

- `.github/workflows/publish-cardsets.yml` (new):
  - `on: push: branches: [main], paths: ['cardsets/**']` plus `workflow_dispatch`.
  - `concurrency: publish-cardsets` with `cancel-in-progress: false`, so publishes never overlap.
  - Job on `ubuntu-latest`: `actions/checkout@v4`, `actions/setup-go@v5` with `go-version-file: go.mod`, then `go run ./cmd/tcgreplay sets check`, then `go run ./cmd/tcgreplay sets publish`.
  - `env` from secrets: `TCGREPLAY_SPACES_ENDPOINT`, `TCGREPLAY_SPACES_BUCKET`, `TCGREPLAY_SPACES_KEY`, `TCGREPLAY_SPACES_SECRET`, `TCGREPLAY_SPACES_CDN_TOKEN`.
  - `permissions: contents: read`.
  - A header comment explains: what it publishes, that the playmat picks the change up within its check interval (5 minutes) or at once from `/admin`, and which secrets it needs.
- `go run` needs no Node: the board and site placeholders are checked in, so Go builds without them (design doc `TCG Review Playmat Design and Build Plan.md:304`).
- No automated test. Its first real run is a manual check in the test plan.

Complexity: Low. Token estimate: ~8k. Agent strategy: single agent, sequential.

### Task: Deploy workflow that skips card data

Requirements carried: Constraint "New data must not need a redeploy"; Overview "Games in progress are not disturbed". Repo: `tcgreplay`.

- `.github/workflows/deploy.yml` (new):
  - `on: push: branches: [main], paths-ignore: ['cardsets/**']` plus `workflow_dispatch`.
  - `concurrency: deploy`.
  - Job: `digitalocean/action-doctl@v2` with `token: ${{ secrets.DIGITALOCEAN_ACCESS_TOKEN }}`, then `doctl apps create-deployment ${{ secrets.TCGREPLAY_APP_ID }} --wait`.
  - A header comment says why it exists: a card-data-only push must not restart the playmat and drop games.
- `.do/app.yaml:28-35`: set `deploy_on_push: false`. Rewrite the header comment at `:5-6` ("Deploying code needs only a push to main") and at `:20-23` to say deploys come from `.github/workflows/deploy.yml` on pushes to `main` that change anything outside `cardsets/`, and that applying `doctl apps update --spec` is still how spec edits are applied.
- Open assumption check: if App Platform's app spec supports a native path filter for `deploy_on_push`, use it instead, leave the workflow out, and record the substitution.
- No automated test (manual check in the test plan).

Complexity: Low. Token estimate: ~8k. Agent strategy: single agent, sequential.

### Task: Set up publishing secrets and the published sets address

Requirements carried: Publish sets; Desktop works offline (default address). Repo: `tcgreplay`. Human task.

What the maintainer does:
- In GitHub → Settings → Secrets and variables → Actions, add:
  - `TCGREPLAY_SPACES_ENDPOINT` (`https://<region>.digitaloceanspaces.com`, the region address, not the bucket's "Origin Endpoint"; knowledge `gotchas/spaces-endpoint-is-region-not-bucket.md`).
  - `TCGREPLAY_SPACES_BUCKET`, `TCGREPLAY_SPACES_KEY`, `TCGREPLAY_SPACES_SECRET`. A key that can write public-read objects; the hosted key already can.
  - `TCGREPLAY_SPACES_CDN_TOKEN`, with scopes `cdn:read` + `cdn:delete` (knowledge `gotchas/cdn-purge-needs-cdn-delete.md`).
  - `DIGITALOCEAN_ACCESS_TOKEN` (can create App Platform deployments) and `TCGREPLAY_APP_ID` (`doctl apps list`).
- Give the bucket's CDN address, `https://<bucket>.<region>.cdn.digitaloceanspaces.com`, to whoever implements "Playmat reads published sets in every mode". It becomes the default `-cardsets-url` (`…/cardsets/`). Record it in this plan's working notes or the task conversation, not in a secret.
- Apply the app spec change (`doctl apps update <app-id> --spec .do/app.yaml`) once the deploy workflow is merged, then start the deploy workflow by hand once.

How it is checked: the hand-started deploy succeeds, and the publish workflow's hand start reaches the publish step without a credentials error.

### Task: Publish every built set

Requirements carried: Every built set published before the switch. Repo: `tcgreplay`. Human task. Precondition: the card-set-import plan's final milestone (every English set built and verified, `sets check` clean over `cardsets/`) has landed on `main`.

What the maintainer does: start `publish-cardsets` by hand (Actions → Run workflow), or locally run `go run ./cmd/tcgreplay sets publish` with the four `TCGREPLAY_SPACES_*` values and `TCGREPLAY_SPACES_CDN_TOKEN` exported.

How it is checked: `curl -s https://<bucket>.<region>.cdn.digitaloceanspaces.com/cardsets/manifest.json` lists the same set ids as `ls cardsets/sets/`. A second run reports every set unchanged and `manifest unchanged`.

Must be done before "Playmat reads published sets in every mode" is merged to `main`, because merging deploys the switch.

### Task: Catalogue snapshot

Requirements carried: Playmat reads card data only from published storage (lookups); Open games keep their cards (immutability, sharing). Repo: `tcgreplay`.

- `internal/catalog/snapshot.go` (new):
  - `Snapshot{version string; sets map[string]*setData; cards map[string]*cards.Card; setIndex map[string]string; byName map[string][]string}`. `setData{id, hash string; header cardset.SetHeader; cards []*cards.Card}`.
  - `NewSnapshot(version string, sets []*setData) *Snapshot` builds the `cards` map and indexes.
  - `convertSet(s *cardset.Set, hash string) (*setData, error)` calls `s.Card(id, "en")` / `Print.Card("en")` for each print, then `energyArt` on the result.
  - A set that fails conversion is logged and left out. The rest of the snapshot builds.
- Index logic moved from `internal/cards/catalog.go:180-241` (`indexes`: codes from `Abbreviation` and `TCGOnline` with the shorter-id collision rule; Pocket `serie == "tcgp"` skipped for names; preference: art, then newer `ReleaseDate`, then lower number, then id). It now reads `SetHeader.Abbreviation`, `TCGOnline`, `Serie`, `ReleaseDate` and the converted cards' `Image`.
- Moved verbatim from `internal/cards/cards.go`:
  - `codeAliases`/`setAliases`/`normCode` (`:573-590,701-709`), as package-level vars in `catalog`.
  - `findNumber` (`:711-726`), over `setData.cards`' `LocalID`.
  - `nameKey` usage and the `basic ` prefix fallback (`:728-784`).
  - `hasMoves` (`:786-795`).
  - `basicEnergyArt`/`energyArt` (`:529-546`). With the image list from "Image source list and board fallback", it copies the borrowed print's whole `Images` as well as `Image`.
  - `cards.SameName` stays in `cards`, and `catalog` calls it.
- Methods:
  - `Card` returns `ErrNotFound` wrapping the id when the id is missing.
  - `Cached` is a map read.
  - `Resolve`: alias, then `setIndex`, then `findNumber`. There are no refreshes. Errors wrap `cards.ErrNotFound` with today's messages (`cards.go:684-697`).
  - `ByName`: as `cards.go:743-766`, without network.
  - `SetID(name)`: as `internal/cards/clear.go:21-60`, minus the refresh, with errors wrapping `cards.ErrUnknownSet` (kept in `cards`).
  - `Version()`, `Sets() []SetInfo{ID, Name, Hash, Prints}`.
  - A nil or empty snapshot answers not found everywhere.
- `Next(prev *Snapshot, version string, changed map[string]*setData, removed []string) *Snapshot` reuses `prev.sets` entries not in `changed`/`removed` (structural sharing).
- `internal/catalog/snapshot_test.go` (new): port the cases of `internal/cards/catalog_test.go` and `internal/cards/byname_test.go` to built-set fixtures:
  - collision `30th`/`30th-c` and `cel25`/`cel25cc`, aliases (`30TH`, `M30`), Pocket skipped, art-first preference, moves pick (the Chi-Yu case), `basic ` prefix;
  - `Resolve` with leading zeros and lettered numbers;
  - `energyArt` on a Normal Energy without image;
  - `Next` shares unchanged `*setData` pointers (pointer equality), and the old snapshot still returns old text;
  - empty snapshot.
  - Fixtures: small `cardset.Set` values built in Go.

Complexity: Medium. Token estimate: ~35k. Agent strategy: single agent, sequential (port tests alongside).

### Task: Image source list and board fallback

Requirements carried: Card images still shown; A publish reaches every viewer (changed image source). Repo: `tcgreplay`.

- `internal/cards/cards.go`:
  - Add `type ImageRef struct{ URL string \`json:"url"\`; File bool \`json:"file,omitempty"\` }` and `Card.Images []ImageRef`, the print's whole ordered list.
  - Keep `Card.Image`, set to `Images[0].URL` (empty when there is none), for readers that only ask whether a card has art (the by-name art-first preference, the Basic Energy and board borrowed-art checks) and for test pools that set only `Image`.
  - Add `func (c *Card) ImageList() []ImageRef`: `Images` when set, else one entry made from `Image` when that is set (`File` from `ImageIsFile`), else nil.
  - Add `func ImageIsFile(url string) bool`, true when the last path segment has a file extension (`.jpg`, `.jpeg`, `.png`, `.webp`). TCGdex base paths end in the card number, so they have none.
- `internal/cardset/card.go` (from the card-set-import plan): `Print.Card` fills `Images` from every `Images[i]` in the print's order. A `source == "tcgdex"` URL starting with `https://assets.tcgdex.net/` with no file extension is a base path (`File: false`). An `https://` URL for which `ImageIsFile` is true (for example a malie `https://cdn.malie.io/….jpg`) is a file entry (`File: true`). Anything else is left out. `Image` is the first entry's URL. Update the doc comment that says publishing decides the policy, and that plan's conversion test for the new rule. Add cases: malie-only (one file entry); tcgdex then malie (both kept, tcgdex first as listed); malie listed first by correction (malie first); an unknown source or unclassifiable URL left out; no images (empty list, empty `Image`).
- Borrowed art: `energyArt` (moved into `internal/catalog` by "Catalogue snapshot"; still in `internal/cards` if this task lands first) and `borrowedArt` (`internal/webui/server.go:107-127`) borrow the other print's whole `ImageList()`, not one address, and set `Image` to its first entry. A print counts as having art when its list is non-empty.
- `internal/webui/server.go:161-188` `CardInfo`: replace `Image`, `ImageFallback` and `ImageVersion` with `Images []CardImage \`json:"images"\``, where `CardImage{Image string \`json:"image"\`; File bool \`json:"image_file,omitempty"\`; Fallback string \`json:"image_fallback,omitempty"\`; Version string \`json:"image_version,omitempty"\`}`.
- `internal/webui/server.go:202-214` `serveCard`: take `c.ImageList()`, else `borrowedArt`. For each base-path entry, call `s.Images.Image(url)` as today for the CDN copy, version and TCGdex fallback. A file entry is sent as is with `File: true` and no version or fallback. `s.Images.Image` already passes non-TCGdex addresses through unchanged (`internal/cards/images.go:185-191`), so no image-cache change is needed.
- `internal/webui/server.go:49-59`: update the `ImageSource` doc comment: it is called once per base-path entry, and file entries never reach the copy logic.
- `web/src/lib/store.svelte.js:149-183` `cardInfo`/`imageSrc`/`nextImageStage`: build the card's ordered candidate addresses from `info.images`. A base entry gives `image + "/" + size + ".webp"` (with `?v=` when `image_version` is set), then `image_fallback + "/" + size + ".webp"` when a fallback is sent. A file entry gives `image` as is for every size. An entry with an empty `image` is skipped. Stage N is the Nth candidate: on a load error the board moves to the next, and past the last it shows the placeholder (the text stand-in). Update the comment block at `:159-166`. Check that the callers (`Card.svelte:82`, `Zoom.svelte:19`, `Modal.svelte:246`) still advance the stage on `error` and reset it per card.
- `internal/cards/cardstest/cardstest.go`: add board-harness cards, e.g.:
  - `tst-9`: one file entry, `https://cdn.malie.io/file/malie-io/tcgl/cards/jpg/en/tst/tst_en_009_std.jpg`;
  - `tst-10`: a TCGdex base path the spec routes to 404, then a malie file entry;
  - `tst-11`: a malie file entry the spec routes to 404, then a TCGdex base path;
  - `tst-12`: two sources that both 404.
  Check `internal/webui/webtest/webtest.go:77` serves them.
- `web/e2e/images.spec.js`: route the URLs to stub images or 404s as the existing image tests do. `tst-9` requests exactly the jpg URL on the mat, in the zoom and in the modal. `tst-10` tries the base path's copy and TCGdex original, then shows the jpg. `tst-11` tries the jpg, then loads the base path with `/low.webp` (mat) and `/high.webp` (zoom). `tst-12` tries every source in order, then shows the placeholder. Existing base-path cases are unchanged.
- `internal/webui/webui_test.go` (or the existing card-endpoint tests): `/api/card` for `tst-10` returns both entries in order, the first with the fake `ImageSource`'s copy, version and fallback and no `image_file`, the second with `image_file: true` and no fallback or version. A card with no art and nothing to borrow returns an empty `images`. A Basic Energy without art returns the borrowed print's whole list.

Complexity: Medium. Token estimate: ~25k. Agent strategy: 2 parallel agents (Go side: cards, cardset, webui; front end: store and e2e), then integrate.

### Task: Catalogue loader with local copy

Requirements carried: Published changes picked up while running; Desktop works offline after first use; On-demand card data retired (legacy local cache ignored). Repo: `tcgreplay`.

- `internal/catalog/loader.go` (new): `Loader{Base, CacheDir string; Offline bool; Interval time.Duration; HTTP *http.Client; Logf func(string, ...any)}`.
  - `cur atomic.Pointer[Snapshot]`, `checkMu sync.Mutex` (serialises checks), `status` guarded by a mutex.
  - Defaults: Interval 5m, HTTP timeout 30s, and up to 8 set downloads at a time (as `internal/cards/catalog.go:124-160` loads sets).
- `Base` is an `https://…/` URL or a local directory path (no scheme). The fetcher reads `<Base>/manifest.json` and `<Base>/<path>`: by `http.Get` when the base is a URL, by `os.ReadFile` when it is a directory.
- Local copy at `<CacheDir>/cardsets/`, mirroring the layout. The loader reads and writes only that subdirectory, so legacy `sets.json`, `sets/`, `cards/` and `set-index*.json` (knowledge `gotchas/tcgdex-cache-never-expires.md`) are never read.
- `Start(ctx, wait)`:
  1. Load the local manifest and its set files (verify each sha256). If complete, swap in that snapshot.
  2. Unless `Offline`, run `Check` bounded by `wait`.
  3. Log `card sets: loaded N sets (version <12 hex>) from <local copy|base>`, or `card sets: none loaded; cards will show as unknown`.
- `Check(ctx)`, under `checkMu`:
  - Fetch the manifest and `DecodeManifest` (`internal/cardset/manifest.go`). Compute version = sha256 of its bytes. If it equals the current version, record the check and return.
  - Otherwise diff against the current snapshot's per-set hashes. For each changed set: take the local copy if present and verified, else download, gunzip (max 64 MiB uncompressed), verify sha256 against the manifest entry, `cardset` decode. Skip a set with an unknown per-set `format` (logged); it keeps its previous version if any.
  - Write each new object to the local copy (temp file then rename), then build `Next(...)`, then write the local manifest (temp then rename), then `cur.Store`, then remove local set files the new manifest does not name.
  - Any failure before the swap leaves `cur` unchanged and returns the error.
  - Log `card sets: picked up version <v>: changed <ids>, removed <ids>`.
- Outages: `breaker`-style logging (`internal/cards/breaker.go:13-52`, log once down, once up). Copy or import the pattern; it stays in `cards` for the image cache.
- `Run(ctx)`: ticker every `Interval` calls `Check` and logs errors through the breaker. `Offline` means `Run` returns at once.
- `Status()` returns `{Version, Sets, Prints int; LastCheck, LastChange time.Time; LastError string; Base string; Offline bool}`. `Snapshot() cards.DB` returns `Current()`.
- `internal/catalog/loader_test.go` (new), using an `httptest.Server` serving a `DirTarget`-published layout (publish with the task "Card set publisher"'s code into a temp dir and serve it with `http.FileServer`):
  - First load. Publish a change and `Check`: only the changed set is requested (count requests per path). Interval pickup with `Interval: 20ms`.
  - Corrupt object (bytes changed): error, and the old snapshot is kept. Unknown manifest format: error, kept. Server down: error, kept, one log line for repeated failures, one recovery line.
  - Restart offline from the same CacheDir: same definitions. Interrupted local write (manifest absent, set files present): falls back to the previous complete manifest or empty, never a partial snapshot.
  - Legacy `cards/x.json` in CacheDir is never read (place a conflicting definition there and assert it is not served). No server and no copy: empty snapshot.
  - Concurrent `Check` calls never interleave (race detector).

Complexity: High. Token estimate: ~45k. Agent strategy: parallel analysis, sequential integration (fetch and local copy first, then check and swap, then run and status).

### Task: Games pin their catalogue snapshot

Requirements carried: Open games keep their cards. Repo: `tcgreplay`.

- `internal/game/manager.go:24-50`: replace the exported `DB cards.DB` with `src CardSource`, where `type CardSource interface{ Snapshot() cards.DB }`. `NewManager(dir string, src CardSource)`. Add `func (m *Manager) snapshot() cards.DB`, which returns nil when `src` is nil, keeping today's nil-DB behaviour that many tests rely on (`import_test`, `record_roundtrip_test`, `mcpserver/server_test.go:57`). Add `func (m *Manager) CardDB() cards.DB`, which returns the current game's `db` when a game is open, else `m.snapshot()`.
- `internal/game/manager.go:204-261` `BuildCards`/`New`: capture `db := m.snapshot()` once and pass it to `BuildCards(ctx, db, …)` and `Load(req.ID, l, db, …)`.
- `internal/game/import.go:59-164` `Import`: capture `db := m.snapshot()` at the top. Use it in the DefOf/DefOfCode closures (`:88-125`), in `ParseDeck` (make an internal `parseDeck(ctx, db, text, who)` used by both; `:168-185`) and in `Load` (`:154`). `SetOpponentDeck` (`:193-217`) re-runs Import and so takes a fresh snapshot, which is the "reopened" case.
- `internal/game/game.go`: add `func (g *Game) DB() cards.DB`. Remove `warm` (`:516-526`) and the `warming` field (`:96`). `card` (`:502-512`) becomes `g.db.Cached(c.Def)` with no warm.
- `internal/game/verbs.go:828-845`: remove `PrefetchCards`. In `internal/mcpserver/server.go:476,491,503`, remove the calls.
- `internal/mcpserver/server.go:583-601` `card_text`: use `s.m(ctx).CardDB()` instead of `.DB` (nil check kept).
- `internal/webui/server.go:107-127` `borrowedArt` and `:189-225` `serveCard`: use `s.M.CardDB()`.
- `internal/cards/cardstest/cardstest.go`: add `func (f FakeDB) Snapshot() cards.DB { return f }`. Update every `game.NewManager(dir, db)` call site in tests and in `internal/webui/webtest/webtest.go:77`, `internal/usergames` tests and `cmd/tcgreplay` (production wiring changes in the task "Playmat reads published sets in every mode"; until then `main.go:224-236`/`hosted.go:248-254` wrap the existing `*cards.Client` with a one-line adapter `clientSource{db}` whose `Snapshot()` returns it).
- Tests:
  - `internal/game/pin_test.go` (new), with a `swapSource` holding two `FakeDB` versions where `tst-8` (Deep Zone) has different `Effect` text and `tst-2` a different HP:
    - New game, swap, then `card_text`-equivalent `m.CardDB().Card` and `Game.card` return the old text, `hp` the old HP, and the bench warning reads the old Stadium text.
    - New game after the swap: new text.
    - Import from a record after the swap: new text.
    - An import during which the source swaps mid-way (a source that swaps on its first `Snapshot()` call's later use) resolves all defs against one version.
  - Adjust `internal/game/cardlock_test.go` (`slowDB`) and `clear_test.go`: remove assertions about background warming; `clear_test.go` is deleted in "Remove the on-demand card client".
  - `internal/webui` test: `/api/card` for a def in an open game returns the pinned version after a swap.
  - `internal/mcpserver` test: `card_text` returns the pinned version.

Complexity: Medium. Token estimate: ~35k. Agent strategy: single agent, sequential (signature change ripples through tests).

### Task: Admin page checks for published sets

Requirements carried: Published changes picked up while running (admin trigger); On-demand card data retired (admin no longer clears card data). Repo: `tcgreplay`.

- `internal/cacheadmin/cacheadmin.go:20-25`: replace `Cards` with `Catalog interface{ SetID(name string) (id, setName string, err error); Check(ctx context.Context) (catalog.CheckResult, error); Status() catalog.Status }`. `Service{Catalog, Images, …}`.
- `Start` (`:123-178`): resolve the set through `Catalog.SetID`, then call only `Images.Clear`. Log text becomes `cache admin: <by> started clearing images for <label>`.
- Add `CheckNow(ctx, by) (catalog.CheckResult, error)`. It runs synchronously on the request with `DefaultTimeout` capped at 2 minutes, because a check is quick. Log `cache admin: <by> checked published card sets: <result>`. Remember the last result for the page.
- `internal/cacheadmin/handler.go:17-57`: add `POST /admin/check`, which redirects to `/admin` with a flash message (changed sets, or "already up to date", or the error with 502). `/admin/clear` is unchanged in form but clears images only.
- `internal/cacheadmin/page.go:15-51`:
  - Retitle the forms to "Clear images: everything" and "Clear images for one set".
  - Add a "Card sets" section: version (12 hex), set count, prints, base address, last check, last change, last error, and a "Check for published card sets now" button.
  - `tierNames` (`:67-72`) keeps `images` and `cdn` only.
- `internal/cacheadmin/cacheadmin_test.go`: rework `ClearEverything`/`ClearOneSet`/`UnknownOrEmpty`/`OneClearAtATime`/`FailedTier`/`JobOutlivesRequest` against fake `Catalog` + `Images`. Add `TestCheckNow` (a fake catalog's Check is called and its result shown) and `TestPageOffersNoCardDataClear` (the page has no card-definition clear wording; it has the image forms and the check button). `TestRealPiecesClearOneSet` (`:313`) is rewritten with a real `catalog.Snapshot` and `ImageCache` over `spacestest`.
- `cmd/tcgreplay/hosted.go:147-154` `hostedAdmin`: pass the loader as `Catalog` (wired fully in "Playmat reads published sets in every mode"). The admin tests in `cmd/tcgreplay/hosted_admin_test.go:212-436` move with that task.

Complexity: Medium. Token estimate: ~25k. Agent strategy: single agent, sequential.

### Task: Playmat reads published sets in every mode

Requirements carried: Playmat reads card data only from published storage; Published changes picked up while running; Desktop works offline after first use; Every built set published before the switch (merge only after "Publish every built set"); No card data in the program. Repo: `tcgreplay`.

- `cmd/tcgreplay/main.go:42-87` config: add `cardsetsURL string` and `cardsetsInterval time.Duration`. Remove `cardBase` (`:65-68`) once no test uses it, in the next task. `config.args()` (`:76-87`) passes `-cardsets-url` and `-cardsets-interval` on to a spawned daemon.
- `main.go:129-152` flags:
  - `-cardsets-url` (default `$TCGREPLAY_CARDSETS_URL`, else `defaultCardsetsURL`).
  - `-cardsets-interval` (default `$TCGREPLAY_CARDSETS_INTERVAL`, else 5m; reject values under 10s or over 10m).
  - `-cache` help becomes "directory for the local copy of published card sets".
  - `-offline` help becomes "use only the local copy of published card sets".
- `main.go`: `const defaultCardsetsURL = "https://<bucket>.<region>.cdn.digitaloceanspaces.com/cardsets/"`, using the address the maintainer gave in "Set up publishing secrets and the published sets address". If it was not given, STOP and ask; never guess a bucket name.
- `main.go:224-236`: replace `newCardDB` with `newCatalog(c) *catalog.Loader`. `newServer` passes it to `game.NewManager`. `runInProcess` (`:238`) and `runDaemon` (`cmd/tcgreplay/daemon.go:105-162`) call `loader.Start(ctx, 30*time.Second)` before serving, then `go loader.Run(ctx)`.
- `cmd/tcgreplay/hosted.go:305-355` `hostedCardDB` becomes `hostedImages(c)`. It keeps the Spaces config and `NewImageCache`/CDN wiring for images (`:316-350`), and drops `NewTieredStore`/`NewSpacesStore` (`:337`) and the "card cache: … backed by shared store" log.
- `hosted.go:358-372,399-401` `warmCatalog`: removed.
- `runHosted` (`:376-425`): builds the loader, `Start(ctx, 30s)` before listening, `go loader.Run(ctx)`. `newHosted` (`:248-254`) passes the loader to each account's `game.NewManager`. `hostedAdmin(loader, images)`.
- `cmd/tcgreplay/hosted_cache_test.go`, `hosted_admin_test.go` (`:44-47,103` `adminSource`, `cardBase`): rewrite against a published layout served by `httptest` (publish fixtures with `DirTarget`). Cover:
  - a full hosted game (import log, play, `card_text`), with definitions equal to the fixture sets;
  - the fixture TCGdex-like server (kept only as a tripwire) receives zero requests;
  - with no URL reachable and an empty cache: `card_text` reports the card unknown;
  - restart with the same `-cache` and the server down: full text;
  - admin check picks up a newly published change at once;
  - `TestDesktopServesNoAdminPage` (`:436`) unchanged.
- `cmd/tcgreplay/hosted_test.go:662,938,1527`, `guides_test.go`, `timing_test.go`: replace `offline: true` plus empty cache with a `cardsetsURL` pointing at an empty temp dir (same "no card data" behaviour).
- `internal/catalog/corpus_test.go` (new), the regression over real data: `cardset.Publish` the repository's `../../cardsets` into a temp dir with `DirTarget`, `Loader{Base: tmp, Offline: false}.Start`, then for every log in `../../testdata/logs/*.txt` (index in `testdata/index.json`) run `game.Manager.Import` with that loader as source. Every `state.Card` with a `Def` resolves through `g.DB().Card` to a card with a name, and every Trainer and special Energy has non-empty `Effect`. Also import the `testdata/decks` deck with `ParseDeck`. Skip with a log line when `cardsets/sets` is absent. (Place the test in `cmd/tcgreplay` or `internal/game` if importing `game` from `catalog` tests creates a cycle; `catalog` must not import `game` in non-test code.)
- `Makefile:62-63` `dev`: add `-cardsets-url` only if the dev daemon should use a local publish (`make dev-cardsets` publishing to `.dev/cardsets-published` is optional, not required).

Complexity: High. Token estimate: ~50k. Agent strategy: parallel analysis, sequential integration (desktop and daemon wiring, hosted wiring, test rewrites, corpus test).

### Task: Remove the on-demand card client

Requirements carried: On-demand card data retired; No card data in the program; Playmat makes no upstream card requests. Repo: `tcgreplay`.

- Delete from `internal/cards`:
  - `catalog.go`, `fresh.go`, `flight.go`, `warm.go`, `store.go`, `spacesstore.go`, `tiered.go`, `clear.go`. Keep `ErrUnknownSet`: move it into `scope.go` or `cards.go`.
  - From `cards.go`: `Client`, `NewClient`, `cardEntry`, store/fetch helpers (`:168-546` except the types), `DefaultCacheDir` (move to `catalog`, or keep, since `main.go:130` uses it), `setSummary`/`setDetail`, `codeAliases`/`setAliases`/`normCode`/`findNumber`/`ByName`/`prints`/`hasMoves` (already moved in "Catalogue snapshot"), and `energyArt` (moved).
  - Keep: the types (`:27-150`, including `ImageRef`), `DB`, `ErrNotFound`, `logCodePrefix`/`logCodeRe`/`LogCodeID` (`:588-635`), `nameKey`/`SameName`, `ImageList`, `ImageIsFile`.
  - Update the package doc (`cards.go:1-2`): card types, lookup interface and image cache; definitions come from published sets (`internal/catalog`).
- `internal/cards/scope.go`: keep `Scope{Set}`, `All`, `String`, `Image`, `TierReport`, `clearer` (if used by images), `clearTimeout`. Remove `Key`.
- `internal/cards/images.go`: unchanged. Verify `breaker.go` is still used by it.
- Delete tests:
  - `internal/cards/{catalog_test,byname_test (ported),clear_test,clear_store_test,fake_tcgdex_test,flight_test,fresh_test,restart_test,setfresh_test,spacesstore_test,store_test,store_client_test,tiered_test,tiered_client_test,warm_test,scope_test (keep Image cases)}.go`;
  - `internal/game/{cardsource_test,spacescache_test,clear_test}.go`;
  - `internal/deck/live_test.go`, which uses `cards.NewClient` against the real TCGdex; rewrite it against a `catalog.Snapshot` fixture or delete it.
  - Keep `images_test.go`, `images_clear_test.go`, `images_version_test.go`, `logcode_test.go`.
- `cmd/tcgreplay/main.go`: remove `cardBase`; keep `imageBase` for the image cache source in tests.
- `internal/catalog/nofetch_test.go` (new): walk the repository's non-test `.go` files (`../../cmd`, `../../internal`) and fail if any contains `api.tcgdex.net`, except the files on a small allow-list: `internal/cardset/tcgdex.go`, the card-set-import plan's maintainer-side TCGdex fetcher used by `tcgreplay sets import` (add any further file that fetcher is split into by name, never a directory or package wildcard). The failure message names the file and says only the import fetcher may name the TCGdex API. The playmat's runtime packages (`internal/catalog`, `internal/cards`, `internal/game`, `internal/webui`, `internal/mcpserver`) and the playmat paths of `cmd/` must never name it. `assets.tcgdex.net` (images) stays allowed everywhere. Include a test case that feeds the scanner a fake tree, so a hit outside the allow-list is shown to fail and a hit in the allowed file to pass.
- Run `go vet ./...` and `go test ./...`. Fix leftovers (`game.Manager` users, `cacheadmin` imports).

Complexity: Medium. Token estimate: ~30k. Agent strategy: single agent, sequential (deletion, then compile fixes).

### Task: Legacy card data clean-up command

Requirements carried: On-demand card data retired (maintainer clean-up). Repo: `tcgreplay`.

- `cmd/tcgreplay/sets.go`: add the `cleanup-legacy` verb with `-delete`. It uses `spaces.FromEnv` (same messages as `publish`). It lists `tcgdex/` by paging `List` (`internal/spaces/client.go:168`) and prints the count and the first 20 keys. With `-delete` it calls `Delete` in batches (`client.go:255`; `*DeleteError` lists failed keys, printed with exit 1). The prefix is a constant `"tcgdex/"`. Never list or delete any other prefix.
- `cmd/tcgreplay/sets_test.go`: with `spacestest` holding `tcgdex/sets.json`, `tcgdex/cards/x.json`, `images/en/sv/sv01/1/low.webp` and `cardsets/manifest.json`:
  - without `-delete`, it prints count 2 and deletes nothing;
  - with `-delete`, only the two `tcgdex/` keys are gone;
  - with `DenyDelete` on one key, it reports it and exits 1.

Complexity: Low. Token estimate: ~12k. Agent strategy: single agent, sequential.

### Task: Publishing documentation

Requirements carried: Documentation for publishing. Repo: `tcgreplay`.

- `cardsets/README.md` (from the card-set-import plan): add a "Publishing" section. Cover:
  - `tcgreplay sets publish` (by hand, `-to`, `-dry-run`) and the workflow on merge;
  - the layout (manifest plus content-addressed `.json.gz`), that old versions stay, and rollback (publish an older commit);
  - pickup: hosted and desktop check every 5 minutes, admins can press "Check now" on `/admin`, and an open game keeps its cards until it ends;
  - the secrets the workflow needs.
- `README.md:22-49` "Development": one paragraph on publishing and how the playmat loads card sets, linking `cardsets/README.md`. Rewrite `:42-47` (the Spaces bucket now holds published card sets and image copies; there is no card cache).
- `TCG Review Playmat Design and Build Plan.md`:
  - `:27` "Card data" bullet: published sets, loader, pinning.
  - `:300` Runtime: `-cache` holds the local copy, `-offline` uses it.
  - `:302` Hosted mode: remove the `tcgdex/` cache description and keep the image copies.
  - `:306` admin page: image clears plus check now.
  - `:315-329` settings table: add `-cardsets-url`/`TCGREPLAY_CARDSETS_URL` and `-cardsets-interval`/`TCGREPLAY_CARDSETS_INTERVAL`, and reword the `TCGREPLAY_SPACES_*` and `TCGREPLAY_ADMINS` rows.
  - `:331-340` operator steps 6-8: the bucket is for images and published sets; publishing secrets; the deploy workflow; the one-off `sets cleanup-legacy`.
- `.do/app.yaml:11-18,84-119` comments: the Spaces settings serve image copies; card data is published separately and read from the CDN; `TCGREPLAY_ADMINS` can clear images and check for published sets.
- `Dockerfile:1-14` header comment: the same changes to the settings list. Add `TCGREPLAY_CARDSETS_URL` and `TCGREPLAY_CARDSETS_INTERVAL`.
- `Makefile:70-93` comments and `docker-run`: pass `TCGREPLAY_CARDSETS_URL`/`_INTERVAL` through if set.
- `site/src/content/docs/desktop.md`: a short note that the first run needs an internet connection to download card data, after which the playmat works offline and picks up new card data within minutes when online.

Complexity: Low. Token estimate: ~20k. Agent strategy: single agent, sequential.

### Task: Rewrite card cache knowledge entries

Requirements carried: Documentation for publishing (project knowledge). Repo: `tcgreplay`. Done through the `spek-knowledge` skill only. Never edit store files directly.

- Replace `gotchas/tcgdex-cache-never-expires.md` (delete it through `spektacular knowledge delete`, or rewrite it in place under a new title) with an architecture entry on published card sets. Cover:
  - the layout and manifest, the loader (start, interval, admin check, local copy under `<cache>/cardsets/`, legacy files ignored);
  - snapshot pinning per game, and `card_text` and `/api/card` reading through the game;
  - the image cache (unchanged details carried over from the old entry: `images/` copies, versions, fallback, admin image clear, CDN purge);
  - the clean-up command.
- Update:
  - `gotchas/board-card-info-per-page-load.md`: the "artwork TCGdex added later" and "after an admin clears the card cache" bullets become "after a publish is picked up; an open game keeps its cards".
  - `gotchas/new-set-tcgdex-data-incomplete.md`: "The freeze" section no longer applies at run time; the playmat shows only published data.
  - `learnings/card-text-always-english.md`: the catalogue reads `text.en` of published sets, not `NewClient`'s base.
  - `gotchas/tcg-live-log-card-codes.md`: the set-code index is now built from published sets in `internal/catalog`.
  - `gotchas/spaces-endpoint-is-region-not-bucket.md`: drop the `card cache: … backed by shared store` log line, which no longer exists.
  - Any other hit from `spektacular knowledge search "card cache"` / `"tcgdex"` describing on-demand fetching as current.

Complexity: Low. Token estimate: ~15k. Agent strategy: single agent, sequential.

### Task: Remove legacy card data from the bucket

Requirements carried: On-demand card data retired (one-off clean-up). Repo: `tcgreplay`. Human task. Precondition: the switch ("Playmat reads published sets in every mode" and "Remove the on-demand card client") is deployed and the hosted playmat logs `card sets: loaded …`.

What the maintainer does: with the four `TCGREPLAY_SPACES_*` values exported, run `go run ./cmd/tcgreplay sets cleanup-legacy` and check the listed keys are all under `tcgdex/`. Then run it again with `-delete`.

How it is checked: a further run without `-delete` reports 0 objects. The board still shows CDN image copies, and `cardsets/manifest.json` is still in the bucket.

## Testing Strategy

- **Card set publisher**: `internal/cardset/publish_test.go` against `spacestest` and `spacestest.CDN`. Covers first publish (headers, public-read), one changed set (one new object, old bytes intact), unchanged (no PUT), refusal on open items or missing sets, purge or warning, determinism, gzip round trip, `DirTarget` round trip.
- **Sets publish command**: `cmd/tcgreplay/sets_test.go` via `runMain`. Covers `-to`, env against `spacestest`, partial env, `-dry-run`, unknown words still refused.
- **Publish on merge workflow** / **Deploy workflow that skips card data**: no automated tests. First real runs are manual checks in the test plan.
- **Catalogue snapshot**: `internal/catalog/snapshot_test.go`, porting `internal/cards/catalog_test.go` and `byname_test.go` cases. Covers collisions, aliases, Pocket, preference, moves, numbers, Energy art, structural sharing, empty snapshot.
- **Image source list and board fallback**: the `cardset` conversion test cases (order kept, both entry kinds, unclassifiable entries left out), the `/api/card` `images` list test in `internal/webui` (per-entry fields, borrowed list), and `web/e2e/images.spec.js` (full-file image on mat, zoom, modal; fallback from a failed base path to a file and from a failed file to a base path; all sources failing shows the placeholder; base-path cases unchanged).
- **Catalogue loader with local copy**: `internal/catalog/loader_test.go` with `httptest` serving a `DirTarget` publish. Covers first load, changed-set-only fetch, interval pickup, corrupt object, unknown format, outage logging once, offline restart, interrupted local write, legacy files ignored, empty, concurrent checks under `-race`.
- **Games pin their catalogue snapshot**: `internal/game/pin_test.go` (old text, HP and warnings after a swap; new game, import and reopen see new; one version per import), plus `internal/webui` and `internal/mcpserver` pinned-read tests.
- **Admin page checks for published sets**: `internal/cacheadmin/cacheadmin_test.go` (check now, page has no card-data clear, image clears for all or one set, busy, failed tier, real pieces with a snapshot and `spacestest`).
- **Playmat reads published sets in every mode**: `cmd/tcgreplay/hosted_cache_test.go` and `hosted_admin_test.go` rewritten against a published layout (full game, zero tripwire requests, no storage means unknown cards, offline restart, admin check). `internal/catalog/corpus_test.go` (or under `cmd/tcgreplay`/`internal/game` if a cycle forces it) imports every corpus log against the committed catalogue.
- **Remove the on-demand card client**: `internal/catalog/nofetch_test.go` source scan (TCGdex API host allowed only in `internal/cardset/tcgdex.go`); the whole suite passes with old tests deleted.
- **Legacy card data clean-up command**: `cmd/tcgreplay/sets_test.go` against `spacestest` (list only, delete only `tcgdex/`, `DenyDelete` failure).
- **Publishing documentation** / **Rewrite card cache knowledge entries**: reviewed by hand (manual item: follow the publishing docs).
- Success metrics (median merge-to-live under one hour; zero unknown-card reports) and the manual reviews in plan.md § Testing Approach are all **Manual — captured in the implementation test plan**.

## Project References

- Repo `tcgreplay`, root `/home/nicj/code/github.com/nicholasjackson/tcgreplay` (the only registered repo).
- Spec `20261108105504-9abfa10e-card-set-publishing`; epic `20261108105504-9abfa10e-card-set-publishing`. Depends on `20261108110847-d27a9f77-card-set-import` (plan read: plan, context, research). Depended on by `20261108110847-055548e3-paired-stadiums`.
- Design documents: none referenced by the spec.
- Knowledge entries (repo tier, store `tcgreplay`): `gotchas/tcgdex-cache-never-expires.md`, `gotchas/board-card-info-per-page-load.md`, `gotchas/cdn-purge-needs-cdn-delete.md`, `gotchas/spaces-endpoint-is-region-not-bucket.md`, `gotchas/new-set-tcgdex-data-incomplete.md`, `learnings/card-image-sources.md`, `gotchas/tcg-live-log-card-codes.md`, `learnings/card-text-always-english.md`, `architecture/importer-determinism.md`, `gotchas/game-listeners-under-lock.md`, and `conventions/skills-text-and-packaging.md` (considered, not applicable).
- Requirement → files:
  - Publish sets → `internal/cardset/{manifest,publish,target}.go`, `cmd/tcgreplay/sets.go`, `.github/workflows/publish-cardsets.yml`.
  - Playmat reads only published storage → `internal/catalog/{snapshot,loader}.go`, `cmd/tcgreplay/{main,hosted,daemon}.go`.
  - Published changes picked up → `internal/catalog/loader.go`, `internal/cacheadmin/*`.
  - Open games keep their cards → `internal/game/{manager,import,game,verbs}.go`, `internal/mcpserver/server.go`, `internal/webui/server.go`.
  - Desktop offline → `internal/catalog/loader.go` (local copy).
  - Card images → `internal/cardset/card.go`, `internal/cards/cards.go` (`ImageRef`, `Card.Images`, `ImageList`, `ImageIsFile`), `internal/catalog/snapshot.go` (`energyArt`), `internal/webui/server.go`, `web/src/lib/store.svelte.js`.
  - A publish reaches every viewer → `internal/cardset/publish.go` (content addressing, manifest purge).
  - On-demand retired → deletions in `internal/cards`, `internal/cacheadmin`, `cmd/tcgreplay/hosted.go`, and `sets cleanup-legacy`.
  - Every built set published first → the human task "Publish every built set" gating "Playmat reads published sets in every mode".
  - Documentation → `cardsets/README.md`, `README.md`, `TCG Review Playmat Design and Build Plan.md`, `.do/app.yaml`, `Dockerfile`, `Makefile`, `site/src/content/docs/desktop.md`, knowledge entries.

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

Two tasks are High ("Catalogue loader with local copy", "Playmat reads published sets in every mode"): analyse in parallel, integrate sequentially. "Remove the on-demand card client" is mostly deletion followed by compile fixes; one agent. Milestone 1's agent tasks and "Catalogue snapshot", "Image source list and board fallback" and "Games pin their catalogue snapshot" have no dependencies between them and can run in parallel.

## Migration Notes

- **Order matters.**
  1. Merge milestone 1 (publisher, command, workflows).
  2. The maintainer adds the secrets, applies `.do/app.yaml` (`deploy_on_push: false`) and starts the deploy workflow once.
  3. The card-set-import plan's catalogue lands on `main`, and its first push publishes automatically, or the maintainer publishes by hand ("Publish every built set").
  4. Only then merge "Playmat reads published sets in every mode", which deploys the switch.
  5. Then merge milestone 3.
  6. Then run `sets cleanup-legacy -delete`.
- **Hosted data.** The bucket's `tcgdex/` card cache is left untouched by the switch and removed by the one-off clean-up. `images/` copies are kept as they are. `cardsets/` is new.
- **Desktop data.** `~/.cache/tcgreplay/` keeps any old `sets.json`, `sets/`, `cards/` and `set-index*.json`, now ignored. The local copy goes in `~/.cache/tcgreplay/cardsets/`. A first run after upgrading needs network access once.
- **Downgrade.** A release from before the switch still runs: it reads its own old cache files, which this plan never deletes on disk. Only the bucket's `tcgdex/` copy is gone after the clean-up.
- **Rollback of data.** Publish an older commit's `cardsets/`. Its objects still exist, so only the manifest changes.
- **Interim adapter.** Between "Games pin their catalogue snapshot" and "Playmat reads published sets in every mode", production wiring wraps the old `*cards.Client` in a one-line `CardSource` adapter, so the tree builds and runs at every task boundary.

## Performance Considerations

- **Catalogue size.** About 20,000 prints in the full English catalogue. Gzipped set files total a few MB, downloaded once per hosted deploy (the hosted disk starts empty) and once per desktop install, then only changed sets.
- **Memory.** One snapshot holds every converted card plus two indexes; expect tens of MB. Unchanged sets are shared between snapshots, so an older snapshot pinned by an open game costs only its changed sets and the indexes. Hosted is capped at `TCGREPLAY_MAX_USERS` (10) open games on a 0.5 GB instance. Measure it during implementation (Open Questions).
- **Start-up.** The local copy loads in well under a second for the full catalogue (gunzip and decode). Hosted's first online load fetches every set, 8 at a time, and start-up waits up to 30 s for it before serving.
- **Lookups.** Map reads with no locks on the read path (atomic pointer load, then an immutable map). This is faster than today's cache, which takes a mutex per lookup and may touch disk.
- **Network.** One manifest request every 5 minutes per playmat, a few KB served from the CDN. Set downloads happen only after a publish.
- **Publishing.** One manifest read, uploads for changed sets only, one manifest write and one purge. A full first publish uploads every set once.
