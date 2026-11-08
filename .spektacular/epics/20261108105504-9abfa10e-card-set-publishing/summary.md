---
created_date: "2026-11-08"
---

# Planning summary: 20261108105504-9abfa10e-card-set-publishing

## Decisions
- **Card art sources** (card-set-import, card-set-publishing): the import plan put the first TCGdex image into the card; the publishing plan picked one showable source. Outcome: `Print.Card` and `cards.Card` carry the whole ordered image list, `/api/card` sends it, and the board falls back through it in order. Showing malie art does not wait on the malie.io maintainers' answer.
- **TCGdex host source-scan test** (card-set-publishing, card-set-import): the publishing plan's scan banned the host from every non-test Go file, but the importer must call it. Outcome: the host may appear only in `internal/cardset/tcgdex.go`, and every playmat runtime package must not name it.
- **Pair rule text** (card-set-import, paired-stadiums): the import plan corrected the printed rule into the effect text, and the paired-stadiums plan also generated a rule line from the mark, so the rule showed twice. Outcome: the rule comes from data only; `card_text` is unchanged, and a marked print must carry its printed rule.
- **Importer pair recognition** (paired-stadiums): the spec's "handle the flag generally" was weighed against `architecture/importer-determinism.md`. Outcome: the importer recognises pairs from log lines only and never reads the mark.

## Order added for shared files
None added.

## 20261108105504-9abfa10e-card-set-publishing
### Approach
- **Publishing:** `tcgreplay sets publish` and a GitHub Action, on pushes to `main` that touch `cardsets/**`, upload each changed set as an immutable, content-addressed `cardsets/sets/<id>/<16-hex sha256>.json.gz` (public-read, cached a year). They then rewrite a short-cache `cardsets/manifest.json` (60 s) and purge it on the CDN. Publishing refuses while `sets check` has open items, or when a set already published is missing from the repo.
- **Loading:** a new `internal/catalog` package loads immutable `cards.DB` snapshots anonymously over the CDN. It checks at start, every 5 minutes and on an admin "Check now", and keeps a content-addressed local copy. Any failure leaves the current snapshot in place.
- **Pinning:** each game is pinned to the snapshot current when it was created, imported or reopened. `game.NewManager` takes a `CardSource`.
- **Images:** `Print.Card` carries the print's whole ordered image list onto `cards.Card.Images`. Each entry is either a TCGdex base path or a full-file address. `/api/card` returns an `images` array, and the board tries each source in order, showing the placeholder only when all fail.
- **Deploys:** these move to a GitHub Action that skips `cardsets/**`-only pushes.
- **Retirement:** `cards.Client` and its cache tiers are deleted.

### Milestones and tasks
- M1, built card sets are published: Card set publisher; Sets publish command; Publish on merge workflow; Deploy workflow that skips card data; Set up publishing secrets and the published sets address (person); Publish every built set (person).
- M2, the playmat plays from published card sets: Catalogue snapshot; Image source list and board fallback; Catalogue loader with local copy; Games pin their catalogue snapshot; Admin page checks for published sets; Playmat reads published sets in every mode.
- M3, the on-demand card cache is gone: Remove the on-demand card client; Legacy card data clean-up command; Publishing documentation; Rewrite card cache knowledge entries; Remove legacy card data from the bucket (person).

### Tasks for a person
- Set up publishing secrets and the published sets address. This needs the bucket keys, a DigitalOcean token, GitHub admin and the CDN address.
- Publish every built set. This writes to the production bucket, gates the switch, and needs the import plan's full catalogue.
- Remove legacy card data from the bucket. This deletes production objects after the switch is deployed.

### Out of scope
Changing how images are fetched or stored (malie art is not copied); pushing changes to boards already open; building sets; pair play; deliberately unpublishing a set; non-English text at run time; desktop release binaries; moving hosted games out of memory; deleting legacy local cache files; dashboards and alerting.

### Drafting assumptions
- The default CDN address is compiled in; `-cardsets-url` / `TCGREPLAY_CARDSETS_URL` overrides it.
- `deploy_on_push: false`, with a deploy workflow instead, so data merges don't drop open games.
- Bucket clean-up is a dry-run-by-default subcommand.
- Legacy local cache files are ignored.
- The check interval is 5 minutes (10 s to 10 min).
- Start-up waits up to 30 s for the first load.
- A regression test publishes the committed `cardsets/` locally and imports every corpus log.
- Showing malie art does not wait on the malie.io maintainers' answer; art order is maintainer data.
- Open for implementation, each with a stop: whether DigitalOcean's CDN honours an object's `max-age`; whether App Platform can skip a deploy by path; snapshot memory on the 0.5 GB instance (stop above ~150 MB); whether the import plan's `Print.Card`/`Set` API landed as described.

### Project-wide rules
- `Print.Card` and `cards.Card` carry the whole ordered image list (`cards.ImageRef{URL, File}`, `Card.Images`, with `Image` kept as `Images[0].URL`). `/api/card` sends `images`.
- `game.NewManager` takes a `CardSource` (`Snapshot() cards.DB`), and `Manager.DB` is removed. Use `Manager.CardDB()` or `Game.DB()`.
- New card fields reach the playmat only through `Print.Card`.
- Published layout: `cardsets/manifest.json` (`format: 1`) plus `cardsets/sets/<id>/<hash>.json.gz`.
- `sets` gains `publish` and `cleanup-legacy`.
- Deploys come from `.github/workflows/deploy.yml`, skipping `cardsets/**`-only pushes; `publish-cardsets.yml` publishes.
- New settings: `-cardsets-url` and `-cardsets-interval`. `-cache` holds the local copy, and `-offline` uses only it.
- `/admin` offers image clears and "Check now"; card-data clearing is gone.
- The TCGdex API host may appear only in `internal/cardset/tcgdex.go`. A source-scan test fails on any other non-test Go file.

### Manual checks
- Success metric: the median time from merge to live on hosted is under one hour.
- Success metric: zero player reports of unknown cards or missing text after the switch.
- On hosted, a publish shows in a new game within 10 minutes, and at once after "Check now". An earlier game keeps its old text.
- After a real publish, the CDN returns the new manifest and set version.
- A card whose first image source 404s shows the next one; a card whose sources all fail shows the placeholder.
- A Delta Reign card with an image source shows its image on hosted, and TCGdex images load from the CDN.
- A desktop that loaded once plays offline with full text after a restart, and an online desktop picks up a publish within 10 minutes.
- A data-only merge publishes without redeploying, and a code merge still deploys.
- After `sets cleanup-legacy -delete`, no `tcgdex/` objects remain and image copies are intact.
- Following the publishing docs, a person can publish by hand.

## 20261108110847-d27a9f77-card-set-import
### Approach
A new `internal/cardset` package behind a `tcgreplay sets` subcommand (`sets import <set-id|code>`, `-all`, `-no-malie`; `sets check [set...]`). The import is a pipeline of pure stages: fetch → normalise → merge → correct → compare → check → write. It uses its own uncached TCGdex and malie.io fetchers rather than `cards.Client`. Built sets, per-set corrections and import reports live in a top-level `cardsets/` directory (`sets/`, `corrections/`, `reports/`, named by TCGdex set id), outside `internal/`, so they are never embedded. A project skill, `skills/card-set-import/SKILL.md`, has the assistant check every open report item against the printed card and record corrections or acknowledgements. A Go test runs the offline checks over all committed data.

### Milestones and tasks
- M1, the maintainer can build a set: Built set format; Upstream source fetchers; Normalise and merge the sources; Import command for one set.
- M2, reports and corrections: Corrections and acknowledgements; Re-import comparison; Import report; Set check command and committed-data guard.
- M3, assistant-verified import and a clean Delta Reign: Card-set import skill; Card-set documentation; Ask malie.io about build-time use (person); Build and verify Delta Reign.
- M4, every English set built: Import every set in one run; Build and verify the Scarlet & Violet and Mega Evolution sets; Build and verify the Sword & Shield sets; Build and verify the older sets.

### Tasks for a person
- Ask malie.io about build-time use, on its Discord. Nothing depends on it. The answer goes in `learnings/card-image-sources.md` and decides only whether malie stays switched on at build time.

### Out of scope
Other languages and Japanese-only sets; automatic detection of new sets; publishing, runtime loading and image display (card-set-publishing); the pair-mark field and pair play (paired-stadiums); Pokémon TCG Pocket sets; malie listings that only TCG Live has; a CLI for writing corrections; rarity, illustrator, legality and prices; borrowed art for prints without images.

### Drafting assumptions
- Corrections address single fields by JSON Pointer.
- "Every English set" means every English Pokémon TCG set, without Pocket.
- malie-only listings are listed but not built. `me5-5m` is checked during implementation, with a stop to ask the user.
- HTML is stripped; `{X}` energy symbols are kept; comparisons treat `{R}` and `Fire` as equal.
- malie is on by default.
- A print gone upstream is kept and reported.
- Reports are deterministic and tracked in git.
- After Delta Reign, the catalogue is built in three agent tasks by era.
- The Legendary Stadium pair rule is a correction to the effect text.
- `missing-evolves-from` covers Stage 1 and Stage 2 only.
- `.dockerignore` excludes `cardsets/`.

### Project-wide rules
- Built format: `cardsets/sets/<tcgdex-id>.json`, `format: 1`, a language-independent core per print plus `text` keyed by language (`en` required), and images as an ordered `{source, url}` list. `Print.Card(lang)` converts to `cards.Card`. card-set-publishing extends it to carry the whole image list.
- Corrections: `cardsets/corrections/<id>.json`, with `corrections[{card, path, value, reason}]` and `acknowledged[{card, kind, field?, reason}]`. A correction to a field the format lacks is an error.
- Report kinds: `disagreement`, `missing-effect`, `no-attacks`, `missing-evolves-from`, `no-image`, `text-changed`, `correction-redundant`, `print-gone-upstream`. Statuses: `open`, `corrected`, `acknowledged`.
- No card data in the program: `cardsets/` sits outside `internal/`, is never embedded, and is in `.dockerignore`.
- `sets` is dispatched on `os.Args[1]` before the playmat's flags.
- The skill never commits.

### Manual checks
- Success metric: a new set can be imported and verified in one maintainer session.
- An agent-verified import ends with every report item resolved or given a reason, a summary of the diff, and nothing committed.
- Spot-check corrected and acknowledged Delta Reign cards against their printed images.
- Following `README.md` and `cardsets/README.md`, a person can import a set and record a correction.

## 20261108110847-055548e3-paired-stadiums
### Approach
- **Data:** a `playedAsPair` mark is added to the built card format (an optional field; `format` stays `1`) and to `cards.Card`. `Print.Card` copies it. Delta Reign's six Legendary Stadium prints get the mark by correction.
- **State and events:** the Stadium spot becomes a list of up to two cards. The move event gains `pair` (the other half moves in the same event) and `as: "pair"` (a card joins a lone half), so the log, frames and undo see one Stadium. The state reads no card data: verbs read the mark and record the decision. `PlayTrainerWith(seat, actor, iid, pair)` is added beside `PlayTrainer`, and `Move` takes both halves when either leaves.
- **Warnings (never blocking):** a lone half; a pair of identical prints; a Stadium played while one of the same name is in play.
- **Surfaces:** MCP `play_trainer` gains `pair_iid`. `card_text` is unchanged: the pair rule comes from the card's own effect text in the data. The board draws both halves, and dragging the second half onto a lone one completes the pair.
- **Importer:** recognises pairs from log text only, never from the mark (user's decision).

### Milestones and tasks
- M1, the playmat plays pair-played Stadiums: Pair mark in card data; Stadium spot holds a pair; Play, replace and discard Stadium pairs.
- M2, players and the assistant can play a pair: Assistant plays Stadium pairs; Board shows and plays Stadium pairs.
- M3, Delta Reign's Legendary Stadiums play as pairs everywhere: Mark Delta Reign's Legendary Stadiums; Supply a real Stadium pair battle log (person); Import battle logs with a Stadium pair (deferred and recorded if no real log exists); Pair play documentation.
- Dependencies across plans: the pair-mark task needs the import plan's "Built set format"; the Delta Reign task needs "Build and verify Delta Reign"; everything assumes the publishing switch has landed.

### Tasks for a person
- Supply a real Stadium pair battle log, exported from a real TCG Live game.

### Out of scope
Enforcing Stadium effects; pair play for non-Stadiums (`pair_iid` is refused); importing a pair without a real log; card-set building and publishing; marking cards outside Delta Reign; a dedicated board control for pairs; the board's 8th-bench-slot hint bug; any other card field.

### Drafting assumptions
- The same-name warning applies to every Stadium.
- Halves with different names, or without the mark, still play, with a warning.
- A completed pair joins the first half's open action, so one undo removes both.
- `PlayTrainerWith` sits beside `PlayTrainer`.
- Board tests add made-up "Twin Peak" pair cards whose effect carries a made-up printed rule.
- A deferred import is recorded, with a follow-up spec proposed.
- Desktop fans the halves in a widened spot; phone shows them side by side.
- `conventions/skills-text-and-packaging.md` applies to the `play-game` guide text.

### Project-wide rules
- `playedAsPair` is an optional `Print` field and does not bump `format`.
- Card-data changes go through corrections and re-import only.
- The pair rule text comes only from data. A print marked `playedAsPair` must also carry its printed rule in its effect text, by correction if needed. Code never generates or matches rule text.
- The importer never uses card data to decide placement, flags included (`architecture/importer-determinism.md`).
- Data says what a card does, and code never names cards.
- `card_text` reads through `Manager.CardDB()`.
- An older playmat that cannot decode a rebuilt set keeps its current snapshot (the publishing loader's failure rule).
- Players using ChatGPT must refresh the app's tools to see `pair_iid`.

### Manual checks
- Success metric: Delta Reign's Legendary Stadiums are played as pairs, by hand and by the assistant, in a real game, with no workarounds.
- On hosted after publish, Legendary Pyroduct, Summit and Trench show the pair rule in card text and in the board zoom, and each plays as a pair.
- A real TCG Live pair log imports with the pair in the Stadium spot, or the deferral is recorded.
- On a phone and on desktop, both halves of a pair are visible, zoomable and not clipped.
- Following the docs, a person can play a pair and have the assistant play one.
- ChatGPT tools are refreshed before checking `pair_iid`.
