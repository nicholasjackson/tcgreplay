---
created_date: "2026-11-08"
document_status: final
closed_date: "2026-11-08"
---

# Research: 20261108105504-9abfa10e-card-set-publishing

## Alternatives considered and rejected

- **Keep `cards.Client` and swap its TCGdex source for the bucket.** Rejected. The client is built around fetch-on-miss, refresh ages, stale-while-revalidate, a tiered store, single-flight, failure memory, clear generations and "cleared" markers (`internal/cards/cards.go:168-214`, `fresh.go`, `tiered.go`, `flight.go`, `clear.go:74-115`). The spec's Technical Approach retires every one of these. A published set file is complete and immutable, so the right shape is a loaded, immutable snapshot, not a cache.
- **One global catalogue that every game reads (swap in place).** Rejected: the spec requires "a game in progress keeps the card definitions it started with until it ends". Games look definitions up on every read through `g.db.Cached(def)` (`internal/game/game.go:502-512, 529-543`), so swapping one global catalogue would change an open game's cards mid-game. Chosen: immutable snapshots, and each game pins the snapshot current when it is created, imported or reopened (`game.Load` already takes a `cards.DB`, `game.go:115`).
- **Mutable set files at fixed names (`cardsets/sets/me06.json`) with a CDN purge per publish.** Rejected by the spec's Technical Approach ("content-addressed, immutable set files ... plus a small manifest"). A fixed name would need a purge for every changed set and would not keep earlier versions retrievable.
- **Hash in a query string (`me06.json?v=<hash>`) on one object.** Rejected: the object would be overwritten, so earlier versions would not stay retrievable byte for byte (acceptance criterion "Publish uploads only what changed"), and whether the Spaces CDN keys its cache on query strings is unverified.
- **Read published sets with signed Spaces requests (`spaces.Client.Get`).** Rejected for the playmat: desktop builds never hold Spaces credentials (`cmd/tcgreplay/hosted.go:305-355` is hosted-only; knowledge `gotchas/tcgdex-cache-never-expires.md`). Published objects are public-read and the playmat fetches them anonymously over HTTPS from the CDN, the same way the board already loads image copies. Only the publisher signs.
- **Embed a fallback catalogue in the binary.** Rejected by the spec's Constraint: "The released program carries no card definitions, not even as a fallback."
- **Publish uncompressed JSON with `Content-Encoding: gzip` metadata.** Rejected: `spaces.PutOptions` has no Content-Encoding (`internal/spaces/client.go:25-31`) and transparent decoding depends on the client's Accept-Encoding. Chosen: explicit `.json.gz` objects the playmat decompresses itself; the hash is over the uncompressed built-set bytes, so it is the same as hashing the committed file.
- **Copy malie.io images into the bucket like TCGdex images.** Rejected by the spec's Non-Goal ("Changing how images are fetched or stored"). The image cache copies only `https://assets.tcgdex.net/` paths as two webp files (`internal/cards/images.go:41,120-161,415-478`); a malie image is one jpg and would need the copy, versioning, scope and purge logic reworked. Chosen: a malie image is shown straight from its source address; the acceptance criterion only requires a CDN copy "where the hosted image copy exists".
- **One chosen image per card** (`Print.Card` putting only the first source the board can show into `cards.Card.Image`, and `/api/card` sending that one address). Rejected: the user wants the board to fall back across sources, so an image missing or broken at the first source (a TCGdex path not yet filled in, a malie address that moved) shows the next listed source instead of a placeholder. Chosen: `cards.Card` and `/api/card` carry the print's whole ordered image list, and the board tries each source in turn, showing the placeholder only when all fail.
- **Separate `image_low`/`image_high` fields on `/api/card`.** Considered; rejected for a smaller change: one boolean per entry of the `images` list, `image_file`, tells the board that `image` is a complete file address to load for both sizes instead of a base path to which it appends `/low.webp` or `/high.webp` (`web/src/lib/store.svelte.js:168-177`).
- **Leave App Platform's deploy-on-push as it is.** Rejected. Every push to `main` redeploys the hosted playmat and drops every in-memory game (`.do/app.yaml:5-6, 20-23`; design doc `TCG Review Playmat Design and Build Plan.md:304`). With publishing on merge to `main`, a card-data-only merge would also redeploy and drop every game, against "Games in progress are not disturbed". Chosen: deploys move to a GitHub Action that ignores `cardsets/**`-only pushes (`deploy_on_push: false`, `doctl apps create-deployment`). See Open assumptions.
- **Delete the legacy disk cache files on start.** Rejected: the spec says the playmat "ignores any left in local caches". The local copy lives under its own `cardsets/` subdirectory of the cache dir, and nothing reads `sets.json`, `sets/` or `cards/` any more.
- **Do the bucket clean-up by hand in the control panel.** Rejected for a small subcommand (`tcgreplay sets cleanup-legacy`, dry run by default) built on the existing `spaces.List`/`Delete` (`internal/spaces/client.go:168,255`) and `listAll` pattern (`internal/cards/spacesstore.go:100`). It is repeatable and testable against `spacestest`.
- **A new package boundary inside `internal/cardset` for the runtime loader.** Rejected. The card-set-import plan keeps "how card data is built" in `internal/cardset`. Chosen: a new `internal/catalog` package for "how the playmat loads and serves published card data". It imports `cardset` (format, `Print.Card`) and `cards` (runtime types, `DB`). `cards` cannot import `cardset` because `cardset` imports `cards`.

## Chosen approach — evidence

- `cards.DB` (`internal/cards/cards.go:152-165`: `Card`, `Cached`, `Resolve`, `ByName`) is the only card interface the rest of the program uses: `game.Manager.DB` (`manager.go:26,48`), `game.Game.db` (`game.go:68,115`), `deck.List.Resolve` (`deck/deck.go:134`), `game.BuildCards` (`manager.go:204`), `webui` (`server.go:118,122,197`), `mcpserver` `card_text` (`server.go:596`). An immutable snapshot implementing `cards.DB` drops into all of them.
- Games store only def ids (`state.Card{IID, Def, Name, ...}`, `internal/state/types.go:39-48`). Nothing is persisted (`internal/store/store.go:1-4`, `manager.go:21-23`). A record is text (`game/record.go:33-50`) and a reopen re-resolves every def (`game/import.go:59-164`). So pinning means "the snapshot passed to `Load`, also used for resolution in `New`/`Import`".
- Paths that bypass `g.db` today, and must use the game's pinned snapshot: `card_text` (`mcpserver/server.go:583-601`, `s.m(ctx).DB.Card`), `/api/card` and `borrowedArt` (`webui/server.go:107-127,189-225`, `s.M.DB`), `ParseDeck` / `BuildCards` / Import closures (`game/import.go:89-125,168-185`, `manager.go:204-241`).
- `openText` holds pointers into a cached card's text (`verbs.go:435,482,578`); immutable snapshots keep those pointers valid.
- The current set-code index, by-name ordering and set-code collision rule are pure functions over set data (`internal/cards/catalog.go:180-241`), plus `setAliases`/`codeAliases` (`cards.go:573-590`), `findNumber` (`cards.go:711-726`), `nameKey`/`SameName` (`cards.go:728-735`), `ByName` move matching (`cards.go:743-795`) and Pocket skip (`catalog.go:211-213`). They move to the snapshot unchanged in behaviour, fed from built sets instead of TCGdex set lists. Existing `catalog_test.go` and `byname_test.go` cases carry over.
- `energyArt` (`cards.go:529-546`) is runtime behaviour (the import does not borrow art), so the snapshot applies it on conversion, as the client does on load today.
- Publishing tools exist: `spaces.Client.Put` with `ContentType`, `CacheControl`, `Public` (`internal/spaces/client.go:25-31,99-109`), `List` with ETags (`:153-168`), `Delete` (`:255`), `spaces.FromEnv` (`config.go:63`), `PublicBase()` for the CDN host (`config.go:47-55`), `spaces.CDN.Purge(ctx, paths)` (`cdn.go:73`, token scopes `cdn:read` + `cdn:delete`, knowledge `gotchas/cdn-purge-needs-cdn-delete.md`). `spacestest.Server` fakes the bucket including anonymous reads of public objects (`spacestest.go:196`) and `spacestest.CDN` records purges (`cdn.go:89`).
- Background loop pattern: `select` on `ctx.Done()`/ticker (`internal/cards/images.go:227-245`, `internal/usergames/usergames.go:225-235`); outage logging once per outage through `breaker` (`internal/cards/breaker.go:13-52`); `log.Printf` with a lower-case subsystem prefix, injectable `logf` for tests.
- Admin page: inline `html/template` with no JavaScript (`internal/cacheadmin/page.go:15-51`), `POST /admin/clear` (`handler.go:17-57`), one background job at a time (`cacheadmin.go:123-178`). A clear today always covers card data and images together; the spec keeps only the image half and adds a "check now" action.
- `/api/card` returns `image` (a base path), `image_fallback`, `image_version` (`webui/server.go:161-225`); the board appends `/low.webp` or `/high.webp` (`web/src/lib/store.svelte.js:168-177`; callers `Card.svelte:82`, `Zoom.svelte:19`, `Modal.svelte:246`). `ImageCache.Image` passes any non-TCGdex address through unchanged (`images.go:185-191`), so a malie address reaches the board untouched and an image whose source changed is shown from the new source as soon as a game uses the new snapshot.
- Run modes: hosted builds one shared client (`cmd/tcgreplay/hosted.go:305-355`) used by every account's manager (`hosted.go:248-254`), the admin service (`:147-154`) and `warmCatalog` (`:358-372`); desktop/daemon/in-process use `newCardDB` (`main.go:224-236`, `daemon.go:162`). Flags `-cache`, `-offline` (`main.go:129-152`); test hooks `cardBase`/`imageBase` (`main.go:65-68`).
- Deployment: App Platform from `.do/app.yaml`, `deploy_on_push: true` on `main`; no `.github/` directory, no CI (`.do/app.yaml:28-35`). Desktop users build from source with `make install` (`site/src/content/docs/desktop.md`).
- The card-set-import plan's format: `cardset.Read`, `Print.Card(lang)`, `Set.Card(id, lang)`, images as ordered `{source,url}`, built files deterministic, `cardsets/` never embedded and excluded by `.dockerignore`, `sets` subcommand dispatched before the playmat's flags, `sets check` returning open items.

## Files examined

- `tcgreplay:internal/cards/cards.go:66-150` — `Card`, `Text()`; runtime type kept.
- `tcgreplay:internal/cards/cards.go:152-165` — `cards.DB`, the interface the snapshot implements.
- `tcgreplay:internal/cards/cards.go:168-546` — `Client`, fetch, `Card`, `Cached`, `energyArt`; all retired except `energyArt` (moves).
- `tcgreplay:internal/cards/cards.go:573-795` — aliases, `LogCodeID` (kept, used by importer and cardset), `Resolve`, `findNumber`, `SameName`, `ByName`.
- `tcgreplay:internal/cards/catalog.go:18-241` — catalog snapshot and pure `indexes`; logic moves to `internal/catalog`.
- `tcgreplay:internal/cards/{fresh,tiered,spacesstore,store,flight,warm}.go` — retired.
- `tcgreplay:internal/cards/clear.go`, `scope.go` — card-data clear retired; `Scope.Image`, `TierReport` kept for images; `SetID` moves to the snapshot.
- `tcgreplay:internal/cards/images.go:21-692` — image cache kept unchanged; passes non-TCGdex addresses through.
- `tcgreplay:internal/cards/breaker.go` — kept (image cache) and reused by the loader for outage logging.
- `tcgreplay:internal/cards/cardstest/cardstest.go` — in-memory `FakeDB`; stays the test pool.
- `tcgreplay:internal/game/game.go:68,115,502-543` — per-game `db`, `card`, `warm`, `hp`.
- `tcgreplay:internal/game/manager.go:21-50,172-183,204-261` — manager, `setCurrent`, `BuildCards`, `New`.
- `tcgreplay:internal/game/import.go:59-217` — Import, DefOf/DefOfCode, ParseDeck, SetOpponentDeck.
- `tcgreplay:internal/game/verbs.go:828-845` — `PrefetchCards` (retired with warming).
- `tcgreplay:internal/game/clear_test.go:96,147` — open game survives a clear; nearest existing test to pinning.
- `tcgreplay:internal/mcpserver/server.go:476-503,583-601` — prefetch calls; `card_text`.
- `tcgreplay:internal/webui/server.go:42-59,107-127,161-225` — `ImageSource`, `borrowedArt`, `CardInfo`, `serveCard`.
- `tcgreplay:web/src/lib/store.svelte.js:149-183` — `cardInfo`, `imageSrc`, `nextImageStage`.
- `tcgreplay:web/e2e/images.spec.js` — board image tests to extend.
- `tcgreplay:internal/webui/webtest/webtest.go:77` — board test server uses `cardstest`.
- `tcgreplay:internal/cacheadmin/{cacheadmin,handler,page}.go` — admin service, routes, page.
- `tcgreplay:internal/spaces/{client,config,cdn}.go`, `spacestest/` — bucket client and fakes.
- `tcgreplay:cmd/tcgreplay/main.go:42-236` — flags, config, mode switch, `newCardDB`, positional refusal.
- `tcgreplay:cmd/tcgreplay/hosted.go:147-425` — hosted wiring, Spaces, admin, warm.
- `tcgreplay:cmd/tcgreplay/daemon.go:105-220` — daemon server and board.
- `tcgreplay:cmd/tcgreplay/{hosted_cache_test,hosted_admin_test,hosted_test}.go` — fake-TCGdex cmd tests to rewrite.
- `tcgreplay:.do/app.yaml` — deploy on push to main; Spaces settings comments.
- `tcgreplay:Dockerfile:1-14`, `Makefile:62-93` — settings comments; dev daemon.
- `tcgreplay:TCG Review Playmat Design and Build Plan.md:27,300-340` — operational docs for card cache, admin, settings, operator steps.
- `tcgreplay:README.md:22-49` — development docs.
- `tcgreplay:site/src/content/docs/desktop.md` — desktop install; no card-data notes yet.

## External references

- DigitalOcean Spaces CDN purge API (`DELETE /v2/cdn/endpoints/{id}/cache`, `files` list, wildcards) — the publisher purges the manifest path after each publish.
- DigitalOcean App Platform app spec (`github.deploy_on_push`) and `doctl apps create-deployment` — moving deploys to a path-filtered GitHub Action.
- GitHub Actions `on.push.paths` / `paths-ignore` — run the publish only for `cardsets/**` changes, and the deploy for everything else.
- RFC 9111 `Cache-Control: immutable` / `max-age` — set files cached for a year, the manifest for a minute.

## Prior plans / specs consulted

- Plan `20261108110847-d27a9f77-card-set-import` (plan, context, research): the built format, `Print.Card(lang)`, the image list `{source,url}` with publishing owning the image policy, `cardsets/` outside `internal/`, `.dockerignore`, the `sets` subcommand, `sets check`, deterministic files. This plan publishes those files as they are and builds the runtime catalogue on `cardset.Read` and `Print.Card`.
- Spec `20261108110847-055548e3-paired-stadiums`: adds a pair flag to `Print` and pair play; it needs the runtime conversion to carry new fields through `Print.Card` and the loader to stay format-generic.
- Epic `20261108105504-9abfa10e-card-set-publishing`: no planning summary yet.

## Open assumptions

- **The bucket's CDN address for the default published-sets URL is not in the repository** (`TCGREPLAY_SPACES_BUCKET` is a secret in `.do/app.yaml`). The maintainer supplies it (human task); the implementer must not guess a bucket name.
- **DigitalOcean's CDN honours an object's own `Cache-Control: max-age` over the endpoint's edge TTL.** If it does not, the manifest is fresh only after the publisher's CDN purge, which the GitHub Action always runs with the CDN token. If neither holds, STOP and ask.
- **App Platform cannot skip a deploy by path natively.** If it can, use that instead of moving deploys to a GitHub Action, and say so.
- **The whole English catalogue, gzipped, is a few MB and fits the 0.5 GB hosted instance as one snapshot plus a few pinned older versions.** Snapshots share unchanged sets' converted cards, so a pinned older version costs only its changed sets and indexes. If memory use measured during implementation exceeds ~150 MB per snapshot, STOP and ask.
- **The card-set-import plan's `Print.Card` and `Set` API land as that plan states.** If they differ, adapt to what landed; if a needed field is missing, STOP and ask.
- **Showing malie.io art from malie's own address does not wait on the malie.io maintainers' answer** to the card-set-import plan's person task (the user's decision). Which art shows is data the maintainer controls through each print's image-list order, via imports and corrections; no code depends on the answer.

## Drafting assumptions

### Chosen direction: published manifest + immutable snapshots pinned per game (architecture)
- **Decision**: Option A. A `sets publish` command and GitHub Action write content-addressed gzipped set files and a manifest to the existing bucket; a new `internal/catalog` package loads them into immutable `cards.DB` snapshots (start, every 5 minutes, admin "check now"), keeps a local copy, and each game pins the snapshot current at its creation, import or reopen. `cards.Client` and every on-demand path are deleted.
- **Key design decisions**: anonymous CDN reads (no credentials on desktop); snapshots share unchanged sets so pinned old versions are cheap; one snapshot captured per New/Import so resolution and play never mix versions; publish refuses while `sets check` has open items; cards carry each print's whole ordered image list and the board falls back through it, malie shown direct; deploys skip `cardsets/**`-only pushes; start-up waits up to 30 s for the first load.
- **Rejected**: Option B, adapt `cards.Client` to read published files behind its cache tiers (keeps all the machinery the spec retires, and its in-place updates change open games); Option C, copy each game's definitions into the game at start with one global live catalogue (changes game state, records and every lookup path for no gain over pinning).

### Conventions selection (architecture)
- **Decision**: No conventions apply; `conventions/skills-text-and-packaging.md` considered and dropped.
- **Rationale**: It governs `internal/guides` text and embedding, which this feature does not touch.
- **Rejected**: listing it anyway.

### Per-game snapshot pinning through `cards.DB` (discovery)
- **Decision**: The runtime catalogue is a series of immutable snapshots implementing `cards.DB`; a game pins the snapshot current when it is created, imported or reopened, and `card_text` and `/api/card` read through the current game's snapshot.
- **Rationale**: Games already take a `cards.DB` in `game.Load` and store only def ids, so pinning needs no change to game state; it is the smallest way to meet "open games keep their cards".
- **Rejected**: one global catalogue swapped in place (changes open games' cards); copying definitions into each game (larger change to state and records).

### Playmat reads published sets anonymously over the CDN (discovery)
- **Decision**: Published objects are public-read; the playmat fetches the manifest and set files by plain HTTPS from a configured base address (default: the project's bucket CDN, override `-cardsets-url` / `TCGREPLAY_CARDSETS_URL`). Only the publisher holds Spaces credentials.
- **Rationale**: Desktop builds never hold Spaces credentials, and the board already loads public image copies from the same CDN.
- **Rejected**: signed `spaces.Client.Get` reads (needs credentials on every desktop).

### Gzipped, content-addressed set objects (discovery)
- **Decision**: Each published set is `cardsets/sets/<id>/<sha256 of the built file, 16 hex>.json.gz`, public-read, `Cache-Control: public, max-age=31536000, immutable`, never overwritten or deleted. The manifest is `cardsets/manifest.json`, `Cache-Control: public, max-age=60`, purged on the CDN after each publish when a CDN token is set.
- **Rationale**: Matches the spec's publish layout; earlier versions stay retrievable byte for byte; gzip keeps the full catalogue to a few MB without relying on Content-Encoding metadata the Spaces client cannot set.
- **Rejected**: fixed names with per-set purges; hash in a query string; Content-Encoding metadata.

### Images: the whole ordered list, with board fallback (discovery)
- **Decision**: `cards.Card` and `/api/card` carry the print's whole ordered image list. Each entry is a TCGdex base path (the board appends `/low.webp` or `/high.webp`, and the image cache copies it to the CDN as today) or a complete file address used as is (a malie.io jpg, flagged `image_file`). The board tries each source in order, moves to the next when an image is missing or fails to load, and shows the placeholder only when all fail. The hosted image cache keeps copying TCGdex images only.
- **Rationale**: The spec requires images "from the image source each built print names", Delta Reign has only malie images, and its Non-Goal rules out changing how images are fetched or stored. The user chose fallback across sources, so one missing or broken image never hides art another source has. Which art shows first is data (the list's order, set by imports and corrections), not code, and showing malie art is not gated on the malie.io maintainers' answer.
- **Rejected**: one chosen image per card (no fallback across sources; the user wants the board to fall back); copying malie images to the bucket (Non-Goal); TCGdex-only images (Delta Reign would show none).

### Deploys skip card-data-only pushes (discovery)
- **Decision**: Move hosted deploys from App Platform's deploy-on-push to a GitHub Action that runs `doctl apps create-deployment` on pushes to `main` that change anything outside `cardsets/**`; set `deploy_on_push: false`.
- **Rationale**: Every push to `main` redeploys and drops every in-memory game today. Publishing on merge would otherwise drop all games for a data-only change, against "Games in progress are not disturbed" and "New data must not need a redeploy".
- **Rejected**: leaving deploy-on-push (data merges drop games); keeping card data on another branch (the spec wants what is served to match the reviewed `main`).

### Bucket clean-up as a subcommand (discovery)
- **Decision**: `tcgreplay sets cleanup-legacy` lists and, with `-delete`, removes every object under `tcgdex/` in the bucket, using the `TCGREPLAY_SPACES_*` settings; a person runs it once after the switch is live.
- **Rationale**: Repeatable, testable against the fake bucket, and needs no control-panel steps.
- **Rejected**: manual deletion in the control panel (untestable, easy to get the prefix wrong).

### Legacy local cache ignored, not deleted (discovery)
- **Decision**: The local copy of published sets lives under `<cache dir>/cardsets/`; old `sets.json`, `sets/`, `cards/` and `set-index*.json` files are left alone and never read.
- **Rationale**: The spec says the playmat "ignores any left in local caches"; deleting user files is not asked for.
- **Rejected**: deleting the legacy files on start.

### Check interval default 5 minutes, bounded 10 s to 10 min (tasks)
- **Decision**: The loader checks the manifest every 5 minutes by default; `-cardsets-interval` accepts 10 s to 10 min.
- **Rationale**: The acceptance criteria require pickup within 10 minutes; 5 minutes leaves room for the manifest's 60 s cache and a slow check, and the upper bound stops a setting from breaking the criterion.
- **Rejected**: 1 minute (needless traffic from every desktop); 10 minutes (no margin).

### Publish refuses on open check items or missing published sets (tasks)
- **Decision**: `sets publish` runs the set check first and refuses to publish while anything is open, and refuses when a set in the published manifest is missing from the repository.
- **Rationale**: Players must never see half-finished data; a partial checkout must never unpublish a set.
- **Rejected**: publishing regardless (could serve an unverified set); dropping missing sets from the manifest (silent unpublishing). Removing a set on purpose is out of scope.

### Start-up waits up to 30 seconds for the first load (tasks)
- **Decision**: Every mode loads the local copy, then waits up to 30 s for the online check before serving.
- **Rationale**: A game started before the first load would pin an empty catalogue for its whole life; hosted starts with an empty disk on every deploy.
- **Rejected**: serving at once (early games show every card as unknown); blocking until loaded (a CDN outage would stop the playmat starting).

### The first full publish is a person's task that gates the switch (tasks)
- **Decision**: "Publish every built set" is a human task, and "Playmat reads published sets in every mode" depends on it; it must be done before that task is merged to `main`.
- **Rationale**: The spec requires every built set published before on-demand card data is retired, and merging to `main` deploys the switch.
- **Rejected**: relying on the publish workflow after merge (the deploy and the publish would race, and new games could pin a partial catalogue).

### Real-data regression over the test corpus (tasks)
- **Decision**: A test publishes the committed `cardsets/` locally, loads it and imports every corpus battle log, asserting full card text.
- **Rationale**: It turns "existing records still work" into a behavioural test over real data, and acts as a proxy for the "zero unknown cards" metric.
- **Rejected**: relying on manual checks only.

## Rehydration cues

- `spektacular spec file read 20261108105504-9abfa10e-card-set-publishing`
- `spektacular plan file read 20261108110847-d27a9f77-card-set-import plan` (and `context`)
- Knowledge (repo tier, store `tcgreplay`): `gotchas/tcgdex-cache-never-expires.md`, `gotchas/board-card-info-per-page-load.md`, `gotchas/cdn-purge-needs-cdn-delete.md`, `gotchas/spaces-endpoint-is-region-not-bucket.md`, `gotchas/new-set-tcgdex-data-incomplete.md`, `learnings/card-image-sources.md`, `gotchas/tcg-live-log-card-codes.md`, `learnings/card-text-always-english.md`, `architecture/importer-determinism.md`, `gotchas/game-listeners-under-lock.md`.
- Re-read: `internal/cards/cards.go:152-165`, `internal/cards/catalog.go:180-241`, `internal/game/game.go:502-543`, `internal/game/import.go:59-185`, `internal/webui/server.go:107-225`, `cmd/tcgreplay/hosted.go:305-425`, `internal/cacheadmin/`, `internal/spaces/`.
