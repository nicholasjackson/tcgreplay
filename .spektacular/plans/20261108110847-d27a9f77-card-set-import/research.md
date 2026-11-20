---
created_date: "2026-11-08"
document_status: final
closed_date: "2026-11-08"
---

# Research: 20261108110847-d27a9f77-card-set-import

## Alternatives considered and rejected

- **Build the importer on `cards.Client` (the playmat's caching TCGdex client).** Rejected. The client serves stale entries first and refreshes in the background (`internal/cards/cards.go:257-282`, `fresh.go`; knowledge `gotchas/tcgdex-cache-never-expires.md`), applies borrowed Basic Energy art to the card itself (`cards.go:520-534`), and the card-set-publishing spec in this epic retires the playmat's TCGdex fetch, refresh ages, tiers and failure memory. An importer leaning on that code would read stale data and would be deleted from under itself. The importer gets its own small uncached fetcher in a new package.
- **Corrections as a whole-card override (replace a card's definition).** Rejected. The spec requires a re-import to pick up upstream changes "unless a recorded correction covers that field", and to report "corrections that the sources now agree with". Both need field-level corrections. Chosen: one correction = card id + JSON Pointer (RFC 6901) path + value + reason.
- **Separate files per language (`<set>.core.json`, `<set>.en.json`).** Rejected: the spec's Technical Approach fixes "one file per set ... holding a language-independent core plus per-language text". Chosen: one file per set, each print carrying its core fields and a `text` object keyed by language.
- **Turn `{R}`-style energy symbols into words in all text.** Rejected. TCGdex has written older sets' text with `{L}`-style symbols for years (`sv01-170` Electric Generator: "attach up to 2 Basic {L} Energy cards"), malie writes the same notation, decklists and tests use it (`internal/deck/deck.go:10,122`, `internal/game/game.go:595`), and the playmat's one text regex accepts both (`internal/game/game.go:596`). Only HTML (Delta Reign's `<span class="energy-symbol Fire" title="Fire">Fire</span>`, `<em>…</em>`) is markup; spans become their inner word, so Delta Reign's text reads as plain words as the acceptance criterion asks. Comparison between sources treats `{R}` and `Fire` as equal.
- **Commit-free report kept only on stdout.** Rejected: the verification skill must consume a machine-readable report, and the final "nothing without a recorded reason" state is worth keeping in review. Chosen: the report is written beside the built set, deterministic (no timestamps), and tracked in git.
- **A CLI command to record corrections.** Rejected as unnecessary: the corrections file is small readable JSON the assistant and maintainer edit directly; `sets check` validates it (card exists, path resolves, value decodes).
- **Keeping malie entirely out of the build until its maintainers agree.** Rejected as the default: the spec prefers malie "only at build time", which is what the import is. A `-no-malie` flag makes a TCGdex-plus-corrections build possible, and asking on its Discord is a human task.

## Chosen approach — evidence

- CLI entry point: `cmd/tcgreplay/main.go:126-171` parses flags then refuses any positional word (`flag.NArg() > 0`); a `sets` subcommand must be dispatched on `os.Args[1]` before `flag.Parse`. CLI tests re-run the test binary through `runMain` (`cmd/tcgreplay/main_test.go:17-52`), and `TestUnknownCommandRefused` (`main_test.go:54+`) must keep passing for other words.
- TCGdex shapes (fetched 2026-11-08): `/v2/en/sets` lists 221 sets (`{id,name,cardCount}`), `/v2/en/sets/me06` gives `serie`, `releaseDate`, `abbreviation.official`, `legal` and `cards[{id,localId,name,image?}]` (141 cards); `/v2/en/cards/me06-001` adds `illustrator`, `rarity`, `variants`, `legal`, `updated`, `thirdParty` and a daily-changing `pricing` block. Only definition fields are kept, so re-imports do not churn.
- Delta Reign on TCGdex (2026-11-08): 0 images, 0 effects, 0 evolveFrom, 0 regulation marks of 141; 19 cards with HTML (`<em>(Don't apply Weakness and Resistance for Benched Pokémon.)</em>`, energy spans). Matches knowledge `gotchas/new-set-tcgdex-data-incomplete.md`.
- malie.io export: `https://cdn.malie.io/file/malie-io/tcgl/export/index.json` → `en-US` → 33 TCG Live sets (SV and ME eras only), each `{path,name,num,hash,abbr}`; `me6` → `v0.1.9.13/me6.en-US.json`, a list of 228 entries = 141 prints × (std + `ph` reverse-holo) entries, told apart by `ext.tcgl.longFormID` containing `_std_`. Fields: `card_type` (POKEMON/TRAINER/ENERGY), `subtype` (SUPPORTER/ITEM/STADIUM/TOOL/SPECIAL), `stage` (BASIC/STAGE1/STAGE2), `stage_text` ("Evolves from Surskit"), `regulation_mark`, `hp`, `types` (upper case), `weakness{types,operator,amount}`, `resistance`, `retreat`, `text[{kind: ATTACK|ABILITY|TEXT_BOX|RULE_BOX|REMINDER, name, text, cost, damage{amount,suffix}}]`, `collector_number{numeric}`, `ext.tcgl.cardID` (`me6_1`), `images.tcgl.jpg.front`.
- malie → TCGdex id: `cards.LogCodeID` (`internal/cards/cards.go:640-680`) already maps TCG Live card ids (`me6_1` → `me06-001`, `mebsp_33` → `mep-033`, `zsv10-5_79` → `sv10.5b-079`); set matching should prefer TCGdex `abbreviation.official` == malie `abbr` (shorter id wins on collisions, as `catalog.go:197-209` does), then the LogCodeID set part; numbers match by numeric value like `findNumber` (`cards.go:726-740`).
- Pokémon TCG Pocket sets (`serie.id == "tcgp"`) are already treated as "a different game" by the catalog (`internal/cards/catalog.go:211-213`).
- Runtime card type the built format must convert to: `cards.Card` (`internal/cards/cards.go:66-89`) with `Ability`, `Attack`, `Damage`, `TypeValue`, `SetRef`; `Card.Text()` (`cards.go:94-142`) renders card text for the assistant.
- Concurrency precedent: the catalog loads sets eight at a time (`catalog.go:120+`, `loadSets`).
- Test fakes: `internal/cards/fake_tcgdex_test.go:15-80` serves bodies by path from an `httptest.Server`; the importer's tests follow the same pattern for TCGdex and malie.
- Project skills live in `skills/<name>/SKILL.md`; `.claude/skills` is a symlink to `skills/`. `.dockerignore` already excludes `skills`, `.spektacular`, `.claude`.
- Developer docs live in `README.md` "Development"; `site/` is the user-facing website (its `developers.md` only links to GitHub).

## Files examined

- `cmd/tcgreplay/main.go:126-171` — flag parsing; positional words refused; place to dispatch `sets`.
- `cmd/tcgreplay/main_test.go:17-52` — `TestMainHelper`/`runMain` subprocess pattern for CLI tests.
- `internal/cards/cards.go:66-142` — `Card` and its text rendering; the runtime type built sets convert to.
- `internal/cards/cards.go:520-534` — `energyArt`: borrowed Basic Energy art is runtime behaviour, not upstream data.
- `internal/cards/cards.go:556-640` — `setDetail` shape, `codeAliases`, `setAliases`, `logCodePrefix`.
- `internal/cards/cards.go:640-680` — `LogCodeID`, reusable for malie card ids.
- `internal/cards/cards.go:726-740` — `findNumber` number matching.
- `internal/cards/catalog.go:197-213` — set-code collision rule; Pocket skipped.
- `internal/cards/fake_tcgdex_test.go:15-80` — fake upstream server pattern.
- `internal/cards/cardstest/cardstest.go` — in-memory card pool for tests.
- `internal/game/game.go:593-608` — `costRe` accepts `{F}` and words.
- `internal/deck/deck.go:10,122` — decklists use `Basic {F} Energy` names.
- `.dockerignore` — build context exclusions.
- `Makefile` — `make test` runs `go vet`, `go test ./...`, then browser suites.
- `README.md` — developer documentation home.
- `.gitignore` — `bin`, `.dev`, web test output only.

## External references

- TCGdex REST API v2 (`https://api.tcgdex.net/v2/en/...`) — main source; shapes recorded above.
- malie.io TCG Live export (`https://cdn.malie.io/file/malie-io/tcgl/export/index.json`) — gap-filling source for SV and ME sets; fan project, format marked alpha.
- RFC 6901 JSON Pointer — the path syntax corrections use, so a correction names exactly one field.

## Prior plans / specs consulted

- Epic `20261108105504-9abfa10e-card-set-publishing` and its specs: card-set-publishing publishes one file per set and makes the playmat read only that data, retiring the TCGdex fetch (so the importer must not depend on it); paired-stadiums records the pair flag and pair rule text through this spec's corrections (so corrections must reach any card field, and the format must grow a field for the flag).
- Knowledge: `gotchas/new-set-tcgdex-data-incomplete.md`, `learnings/card-image-sources.md`, `learnings/card-text-always-english.md`, `gotchas/tcg-live-log-card-codes.md`, `gotchas/tcgdex-cache-never-expires.md`.

## Open assumptions

- TCGdex's English set list (minus Pocket) is the catalogue of "every English set"; malie keys with no TCGdex counterpart are reported by `-all` but not built on their own: `svalt` and `mealt` are TCG Live alternate finishes of prints in other sets (their `longFormID` names the original print), `me7` and `me5-5c` are empty, and `me5-5m` (three RGB Mew prints, abbr `30C-M`) may or may not be in TCGdex's `30th`.
- Malie's `_std_` entry is the print's canonical entry; the `ph` reverse-holo entry carries the same text.
- TCGdex image URLs are stable (`https://assets.tcgdex.net/en/<serie>/<set>/<localId>`), and malie jpg URLs are stable per set key and number.
- Importing every set (~200 sets, ~20,000 TCGdex card requests) is acceptable load on TCGdex at eight requests at a time, run once by the maintainer.
- The built catalogue (~20,000 prints) fits comfortably in the repository as pretty JSON (tens of MB).

## Drafting assumptions

### Chosen direction: a build-time pipeline in `internal/cardset` behind `tcgreplay sets` (architecture)
- **Decision**: New package `internal/cardset` (own uncached fetchers for TCGdex and malie; normalise → merge → correct → compare → check → write) behind `tcgreplay sets import <set>|-all` and `tcgreplay sets check`; data in top-level `cardsets/{sets,corrections,reports}/<set-id>.json`; a project skill `skills/card-set-import/SKILL.md` drives verification from the report. Key trade-offs: field-level corrections by JSON Pointer (so any known field is correctable and unknown paths fail loudly); one file per set with a language-keyed `text` object; images as an ordered source list; deterministic JSON for clean diffs; `sets check` plus a Go test guard committed data.
- **Rationale**: Matches the spec's Technical Approach (CLI on the existing program, corrections per set, one readable JSON file per set, report consumed by a skill) and keeps the importer independent of the playmat's TCGdex client, which the publishing spec retires.
- **Rejected**: A separate binary or script (spec says a command on the existing program); building on `cards.Client` (stale-first cache, being retired); a single-pass importer without separable stages (untestable without the network).

### Conventions: none apply (architecture)
- **Decision**: No convention cited; `conventions/skills-text-and-packaging.md` dropped.
- **Rationale**: It governs `internal/guides` and its embedding, which this feature does not touch.
- **Rejected**: Applying its "no Claude" wording rule to the new project skill (the rule is scoped to guides by its own test).

### Corrections file name and data directory (architecture)
- **Decision**: Top-level `cardsets/` with `sets/`, `corrections/`, `reports/`, files named by TCGdex set id.
- **Rationale**: Outside `internal/` so nothing embeds it; TCGdex ids are unique, whereas decklist codes collide (`30C`, `CEL`).
- **Rejected**: `testdata/` or `internal/...` (embedding risk, wrong meaning); naming by decklist code (collisions).
### Pokémon TCG Pocket sets are not part of "every English set" (discovery)
- **Decision**: The catalogue is TCGdex's English set list minus sets whose serie is `tcgp`.
- **Rationale**: Pocket is a different game whose sets are not Pokémon TCG sets; the playmat's catalog already skips it as "a different game" (`internal/cards/catalog.go:211-213`) and no TCG Live log or decklist can name its cards. This reads the spec's "every English set" as every English Pokémon TCG set; flagged for the review.
- **Rejected**: Importing Pocket sets too (dozens of sets of cards the playmat cannot use).

### TCG Live-only malie listings are not built as sets of their own (discovery)
- **Decision**: Malie keys with no matching TCGdex set are listed by `sets import -all` with their entry counts, not built as sets. Checked 2026-11-08: `svalt` (237) and `mealt` (11) are TCG Live alternate finishes of prints that already belong to other sets (their `longFormID` names the original print, e.g. `Sprigatito_sv1_13_alt_…`), so they add no prints; `me7` and `me5-5c` are empty; `me5-5m` (abbr `30C-M`, three RGB Mew prints) may belong in TCGdex's `30th`, which the implementer checks (Open Questions).
- **Rationale**: TCGdex is the main source and defines set ids and numbering; the alternate listings are the same cards in another finish, so "every print either source lists" is met by the original prints.
- **Rejected**: Building `svalt`/`mealt` as sets (duplicate cards under ids no decklist uses).

### Energy symbols `{R}` stay; only HTML is markup (discovery)
- **Decision**: HTML tags are stripped (energy spans become their inner word, entities unescaped); `{X}` energy symbols are kept as the sources write them. Source comparison treats `{R}` and `Fire` as equal.
- **Rationale**: TCGdex's older sets and malie both use `{X}`, decklists and the playmat's text regex use it, and no game logic depends on either form; the acceptance criterion names HTML tags and the energy-symbol HTML specifically; and knowledge `gotchas/new-set-tcgdex-data-incomplete.md` describes older sets, which carry `{X}`, as having no markup.
- **Rejected**: Rewriting every `{X}` into a word across the whole catalogue (changes years of upstream wording for no requirement).

### malie is used by default at build time, with an opt-out (discovery)
- **Decision**: The import reads malie unless `-no-malie` is given; asking malie's maintainers on Discord is a human task.
- **Rationale**: The spec prefers malie "only at build time", which the import is, and says the build must still run from TCGdex plus corrections alone.
- **Rejected**: Waiting for the Discord answer before using malie at all (Delta Reign cannot be completed without it today).

### Every image source is recorded, in preference order (discovery)
- **Decision**: Each print records a list of image sources (`tcgdex` first, then `malie`), not a single chosen image.
- **Rationale**: The requirement is to name where the image comes from; the card-set-publishing plan carries the whole ordered list to the board, which falls back through it; showing malie art does not wait on the Discord answer (user's decision, 2026-11-08).
- **Rejected**: Recording only one image (would force this spec to decide the malie-art policy).

### Prints that vanish upstream are kept and reported (discovery)
- **Decision**: A print in the previous built set that neither source lists any more is kept unchanged and reported as `print-gone-upstream`.
- **Rationale**: The acceptance criterion says every print already there is still present after a re-import; a vanished print still deserves a look.
- **Rejected**: Dropping it, or keeping it silently.

### Import reports are tracked in git (discovery)
- **Decision**: Each import writes `cardsets/reports/<set>.json`, deterministic (no timestamps), tracked in git.
- **Rationale**: The skill consumes it; the final clean report is evidence for review and shows in diffs when upstream changes.
- **Rejected**: Reports only on stdout or gitignored (no record of the final state).

### Catalogue build split into three agent tasks by era (tasks)
- **Decision**: After Delta Reign, the full catalogue is built in three agent tasks: Scarlet & Violet plus Mega Evolution (both sources), Sword & Shield (TCGdex only), and everything older.
- **Rationale**: The work differs by era (disagreements where both sources cover a set, missing fields and art where only TCGdex does), and each task touches only its own sets' files, so they can run in parallel.
- **Rejected**: One task for every set (too large to verify or review); one task per set (~200 tasks).

### Legendary Stadium rule recorded as effect text here; the pair mark is left to paired-stadiums (tasks)
- **Decision**: The Delta Reign task records the pair rule as a correction prefixing each Legendary Stadium's English effect; it adds no pair-mark field to the format.
- **Rationale**: The spec puts missing rule text in the corrections file; the paired-stadiums spec owns the data flag and its meaning on the board, and corrections let it add the field later.
- **Rejected**: Adding a pair field now (would decide the paired-stadiums spec's data shape for it).

### Missing evolves-from checked for Stage 1 and Stage 2 only (tasks)
- **Decision**: The `missing-evolves-from` report kind covers `Stage1` and `Stage2` prints only.
- **Rationale**: The spec and its acceptance criterion name Stage 1 and Stage 2; other evolving stages (VMAX, BREAK, LEVEL-UP) use different evolve rules in TCGdex data.
- **Rejected**: Checking every non-Basic stage (would flood old sets with items that need no change).

### `.dockerignore` excludes `cardsets/` in this spec (tasks)
- **Decision**: The import task adds `cardsets` to `.dockerignore`.
- **Rationale**: The program must carry no card data, and the catalogue is tens of MB the image build does not need.
- **Rejected**: Leaving it to the publishing spec (the data lands in this spec, so the exclusion should too).

### The malie Discord question is a person's task that blocks nothing (tasks)
- **Decision**: A `human` task with no dependants asks malie's maintainers and records the answer in `learnings/card-image-sources.md`.
- **Rationale**: Contacting a third party is outside the repo; the spec says the build can run without malie, so no agent task waits on it.
- **Rejected**: Making the Delta Reign or catalogue tasks depend on it (would stall the epic on an outside reply).

## Rehydration cues

- `spektacular spec file read 20261108110847-d27a9f77-card-set-import`
- `spektacular epic read 20261108105504-9abfa10e-card-set-publishing`
- `spektacular knowledge read` the five entries listed under Prior plans / specs.
- Re-read `internal/cards/cards.go:66-142,556-740`, `cmd/tcgreplay/main.go:126-171`, `cmd/tcgreplay/main_test.go:17-52`.
- `curl https://api.tcgdex.net/v2/en/sets/me06` and `curl https://cdn.malie.io/file/malie-io/tcgl/export/index.json` to see live shapes.
