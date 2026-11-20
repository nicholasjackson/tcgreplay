---
created_date: "2026-11-08"
document_status: final
closed_date: "2026-11-08"
---

# Context: 20261108110847-d27a9f77-card-set-import

## Current State Analysis

- The playmat gets every card definition at run time from TCGdex through the caching client in `internal/cards` (`internal/cards/cards.go:183-212` `NewClient`, `Base: https://api.tcgdex.net/v2/en`), with refresh ages, a Spaces tier and an admin clear (knowledge `gotchas/tcgdex-cache-never-expires.md`). There is no build-time card data and no `cardsets/` directory.
- `cards.Card` (`internal/cards/cards.go:66-89`) is the runtime definition every game, MCP and board path uses; `Card.Text()` (`cards.go:94-142`) renders it for the assistant.
- `cmd/tcgreplay/main.go:127-160` has no subcommands: positional words are refused at `main.go:156`.
- Project skills live in `skills/` (`.claude/skills` is a symlink to it); only Spektacular's `spek-*` skills exist today.
- Developer docs are in `README.md:22-49`; `.dockerignore` lists what stays out of the image.
- Upstream state (2026-11-08): TCGdex lists 221 English sets (Pocket included); Delta Reign (`me06`) has 141 prints with no images, effects, evolveFrom or regulation marks and HTML in 19 cards' text; malie's export covers 33 TCG Live sets (SV and ME) and had complete Delta Reign data.

## Per-Task Technical Notes

### Task: Built set format

Requirements carried: Room for other languages, Built sets are reviewable, Each print names its image source (shape only). Repo: `tcgreplay`.

- `internal/cardset/format.go` (new) — `Set`, `SetHeader`, `SetText{Name}`, `Print`, `AttackCore{Cost []string; Damage cards.Damage}`, `AbilityCore{Type}`, `Image{Source, URL}`, `PrintText`, `MoveText{Name, Effect}` as in plan § Data Structures. JSON tags in the order shown; `omitempty` on every optional field so absent facts add no lines. `Format` constant `1`. `Print.Sources []string` sorted.
- `internal/cardset/format.go` — `Read(path)`: `json.Decoder` with `DisallowUnknownFields`, then `Validate()`. `Write(path, s)`: sort prints by numeric `LocalID` then string (lettered numbers such as `TG05` after numbers, mirroring `internal/cards/catalog.go:214-217`), `json.MarshalIndent(s, "", "  ")` with `SetEscapeHTML(false)` (so `×`, `é`, `&` stay literal), trailing newline, write to a temp file in the same directory then rename. Maps (`Text`) marshal with sorted keys, so `en` precedes other languages deterministically.
- `internal/cardset/format.go` — `(s *Set) Validate() error`: each print has `Text["en"]` with non-empty `Name`; for every language present, `len(text.Attacks) == len(core.Attacks)` and same for abilities; English attack/ability names non-empty; set header `Text["en"].Name` non-empty; no duplicate print ids.
- `internal/cardset/card.go` (new) — `(p Print) Card(lang string) (*cards.Card, error)`: fill `cards.Card` (`internal/cards/cards.go:66-89`) from core plus `Text[lang]`; `Set: cards.SetRef{ID: set id, Name: set name}` (method takes the set header too, e.g. `(s *Set) Card(id, lang)` as a convenience); `Image` = first `Images[].URL` whose source is `tcgdex` (publishing spec decides wider policy; document this). The card-set-publishing plan later extends `Print.Card` and `cards.Card` to carry the whole ordered image list, and the board falls back through it; this task keeps the single TCGdex `Image`. Error if `lang` text missing.
- `internal/cardset/format_test.go` (new) — round trip; byte-identical double write; one-print text change diffs only that print's lines (compare line sets); adding `fr` text to one print leaves `en` and core bytes equal (marshal `Text["en"]` and the print with `Text` cleared before/after); `Validate` failures; `Card` conversion for a Pokémon, Trainer, Energy against hand-built `cards.Card` values (model on `internal/cards/cardstest/cardstest.go`).

Complexity: Medium. Token estimate: ~25k. Agent strategy: single agent, sequential.

### Task: Upstream source fetchers

Requirements carried: Import a set (sources). Repo: `tcgreplay`.

- `internal/cardset/fetch.go` (new) — `httpGetter{Client *http.Client; Timeout time.Duration; Retries int}` with `getJSON(ctx, url, out)`: per-request `context.WithTimeout` (default 20s), up to 3 attempts with backoff (500ms, 2s) on network errors, 429 and 5xx; 404 → `ErrNotFound` wrapped with the URL; body limit 32 MiB (malie set files are ~0.5 MB; ASC lists 613 entries). User-Agent `tcgreplay-sets/<version>`.
- `internal/cardset/tcgdex.go` (new) — `TCGdex{Base string; get httpGetter}` default `https://api.tcgdex.net/v2/en`; `Sets(ctx) ([]tcgdexSetSummary, error)` (`/sets`), `Set(ctx, id) (tcgdexSet, error)` (`/sets/<url.PathEscape(id)>`; shape like `setDetail` at `internal/cards/cards.go:556-574` plus `legal`), `Card(ctx, id) (json.RawMessage, error)` (`/cards/<id>`). `Cards(ctx, ids, workers=8)` fans out with a semaphore (pattern: `internal/cards/catalog.go:124` `loadSets`), returning a map and the first error per id.
- `internal/cardset/malie.go` (new) — `Malie{Base string}` default `https://cdn.malie.io/file/malie-io/tcgl/export`; `Index(ctx)` decodes `index.json` and returns `["en-US"]` as `map[string]malieSetRef{Path, Name, Num, Hash, Abbr}`; `SetFile(ctx, path)` returns `[]malieCard` (fields listed in research.md § Chosen approach — evidence and `learnings/upstream-card-source-formats.md`).
- `internal/cardset/fetch_test.go` (new) — fake servers modelled on `internal/cards/fake_tcgdex_test.go:15-80` (bodies by path, hit counts, fail/slow switches): success, 404 → `errors.Is(err, ErrNotFound)`, 500 twice then 200 succeeds with 3 hits, persistent 500 fails after 3, timeout honoured, malie index picks `en-US`.

Complexity: Low. Token estimate: ~15k. Agent strategy: single agent, sequential.

### Task: Normalise and merge the sources

Requirements carried: Import a set (combine per print), Clean up upstream text, Each print names its image source. Repo: `tcgreplay`.

- `internal/cardset/text.go` (new) — `cleanText(s string) string`: replace `<br>`/`<br/>` with newline, strip tags with `regexp.MustCompile("<[^>]+>")` (energy spans keep their inner word, e.g. `<span class="energy-symbol Fire" title="Fire">Fire</span>` → `Fire`), `html.UnescapeString`, normalise `\r\n`, collapse runs of spaces (keep single newlines; malie uses `"\n \n"` paragraph breaks → `"\n\n"`), trim. `{X}` symbols untouched. `compareKey(s)`: cleanText, then map `{G} {R} {W} {L} {P} {F} {D} {M} {C} {N} {Y}` to Grass/Fire/Water/Lightning/Psychic/Fighting/Darkness/Metal/Colorless/Dragon/Fairy, unify `’‘` → `'` and `“”` → `"`, collapse all whitespace, so cosmetic differences are not disagreements.
- `internal/cardset/normalise.go` (new) — `candidate` (a `Print` with `Text["en"]` plus `source string` and `setID`). `fromTCGdex(raw json.RawMessage, set tcgdexSet) (candidate, error)`: decode into a local struct mirroring TCGdex card fields (do not reuse `cards.Card`, which applies no cleaning and may change under the publishing spec); keep only definition fields (drop `pricing`, `updated`, `variants`, `legal`, `thirdParty`, `illustrator`, `rarity`); `cleanText` every name/effect; damage via `cards.Damage` decoding; `image` → `Images[{tcgdex, url}]` when present. No borrowed Basic Energy art (`internal/cards/cards.go:520-534` stays runtime behaviour).
- `internal/cardset/normalise.go` — `fromMalie(entries []malieCard, tcgdexSet, cardIDs) []candidate`: group by `collector_number.numeric`, prefer the entry whose `ext.tcgl.longFormID` contains `_std_`; map `card_type` POKEMON/TRAINER/ENERGY → Pokemon/Trainer/Energy; `subtype` SUPPORTER/ITEM/STADIUM/TOOL → Supporter/Item/Stadium/Tool, SPECIAL → EnergyType Special, ENERGY without subtype → Normal; `stage` BASIC/STAGE1/STAGE2 → Basic/Stage1/Stage2 (others title-cased; record unknown values verbatim); `types` title-case; `weakness{types,operator,amount}` → `TypeValue{Type, Value: operator+amount}` (`×2`), resistance `-30`; damage `{amount, suffix}` → `"50×"`/`"30+"`; `text` kinds: ATTACK → attack (name, cost, damage, text), ABILITY → ability (type "Ability"), TEXT_BOX → effect, RULE_BOX and REMINDER dropped; `stage_text` "Evolves from X" → evolveFrom `X`; `regulation_mark`; `images.tcgl.jpg.front` → `Images[{malie, url}]`. Names: cleanText only (malie's `Bubbly {W} Energy` vs TCGdex `Bubbly Water Energy` is a cosmetic difference, so TCGdex's name wins without a disagreement).
- `internal/cardset/match.go` (new) — `matchMalieSet(tcgdexSet, index) (malieSetRef, bool)`: first a malie set whose `abbr` equals the TCGdex set's `abbreviation.official` (case-insensitive; collisions settled by the shorter TCGdex id, as `internal/cards/catalog.go:197-209`), else a malie key whose `cards.LogCodeID(key+"_1")` set part equals the TCGdex id. Print id: TCGdex print with the same numeric `localId` (like `findNumber`, `internal/cards/cards.go:726-740`); otherwise `cards.LogCodeID(ext.tcgl.cardID)` if its set part is this set, else `<setID>-<NNN>` padded to the width of the set's existing localIds (default 3).
- `internal/cardset/merge.go` (new) — `merge(tcgdex, malie []candidate) []merged`: union by id; per field, TCGdex value if non-empty, else malie's; for scalar and list fields both non-empty and `compareKey` different → `disagreement{Field: JSON Pointer e.g. "/hp", "/text/en/attacks/1/effect", Values: {tcgdex, malie}}`. Attacks/abilities compared position by position when counts match; a count mismatch is one disagreement on `/attacks` (TCGdex's list kept). `Images`: TCGdex's then malie's. `Sources`: which sources listed the print.
- `internal/cardset/normalise_test.go`, `merge_test.go`, `text_test.go` (new) — table tests using trimmed real bodies saved under `internal/cardset/testdata/` (a Delta Reign Pokémon with an energy span, a Trainer with empty effect, a Stage 1 without evolveFrom, a special Energy, Masquerain's `stage_text`, a malie-only print, std+ph pair). Assertions listed in plan § Testing Approach.

Complexity: High. Token estimate: ~45k. Agent strategy: parallel analysis, sequential integration (one agent writes text.go + tests, one writes the malie mapping; merge integrates).

### Task: Import command for one set

Requirements carried: Import a set, Built data lives with the code base (constraint), Card data is data, not code (constraint). Repo: `tcgreplay`.

- `internal/cardset/import.go` (new) — `ImportOptions{Set string; Dir string (default "cardsets"); NoMalie bool; TCGdex *TCGdex; Malie *Malie; Log io.Writer}`; `Import(ctx, opts) (Summary, error)`: resolve `Set` (exact TCGdex id from `/sets`; else fetch all set details and match `abbreviation.official` or `tcgOnline` case-insensitively, shorter id wins; refuse Pocket `serie.id == "tcgp"`); fetch set details and all its cards; unless `NoMalie`, find the malie set and fetch it (a missing malie set is not an error); normalise, merge, build `Set` (header from TCGdex: id, serie id, releaseDate, abbreviation.official, tcgOnline, `Text["en"].Name`), `Validate`, `Write` to `<Dir>/sets/<id>.json`. `Summary{Set, Prints, Added, Changed, Gone, Open int}` (Added/Changed/Gone/Open filled by later tasks).
- `cmd/tcgreplay/sets.go` (new) — `runSets(args []string) int`: own `flag.FlagSet` per verb; `import` takes one positional set plus `-dir`, `-no-malie`, and hidden test flags `-tcgdex-base`, `-malie-base` (mirroring `config.cardBase` at `cmd/tcgreplay/main.go:64-66`); prints the summary to stdout; errors to stderr, exit 1. Unknown verb → usage, exit 2.
- `cmd/tcgreplay/main.go:127` — at the top of `main()`, before any `flag.*Var` call: `if len(os.Args) > 1 && os.Args[1] == "sets" { os.Exit(runSets(os.Args[2:])) }`. Update the comment at `main.go:154-155` ("The binary has no subcommands") to say `sets` is the one subcommand. Package doc comment (`main.go:1-15`) gains one line about `tcgreplay sets`.
- `cmd/tcgreplay/sets_test.go` (new) — via `runMain` (`cmd/tcgreplay/main_test.go:31-52`): fake TCGdex and malie `httptest` servers serving a 3-print fixture set; `sets import TST -dir <tmp> -tcgdex-base … -malie-base …` exits 0 and writes `<tmp>/sets/tst.json`; a second run gives identical bytes; `-no-malie` writes a set with TCGdex data only. `TestUnknownCommandRefused` (`main_test.go:57`) unchanged and passing.
- `internal/cardset/import_test.go` (new) — end to end against fakes into `t.TempDir()`.
- `.dockerignore` — add `cardsets` with a comment: card data is published from the repository, never built into the image.

Complexity: Medium. Token estimate: ~30k. Agent strategy: single agent, sequential.

### Task: Corrections and acknowledgements

Requirements carried: Corrections survive re-import. Repo: `tcgreplay`.

- `internal/cardset/corrections.go` (new) — `Corrections{Corrections []Correction; Acknowledged []Ack}`, `Correction{Card, Path string; Value json.RawMessage; Reason string}`, `Ack{Card, Kind, Field, Reason string}`. `ReadCorrections(path)`: missing file → empty; `DisallowUnknownFields`; every entry needs a non-empty `Reason`.
- `internal/cardset/corrections.go` — `apply(p Print, cs []Correction) (Print, []applied, error)`: marshal print to `map[string]any`, for each correction resolve the RFC 6901 pointer (`~0`, `~1` escapes; array indices; the final segment may create an object member or replace an array element; `-` appends), record the value found before replacement (the merged upstream value) to detect redundancy (`compareKey`-equal or JSON-equal → redundant), set the value, marshal, decode back into `Print` with `DisallowUnknownFields` so a path the format does not define errors as `card <id>: path <p>: unknown field`. Unknown card id → error.
- `internal/cardset/corrections_test.go` (new) — text, number, list and `/images` corrections; unknown path and unknown card errors; redundancy detection; the corrections file bytes are untouched by an import (hash before/after).

Complexity: Medium. Token estimate: ~20k. Agent strategy: single agent, sequential.

### Task: Re-import comparison

Requirements carried: Update an imported set. Repo: `tcgreplay`.

- `internal/cardset/compare.go` (new) — `compare(prev *Set, next *Set) (out *Set, changed []string, gone []string, added []string)`: `prev` nil → everything added. For each id in `prev` not in `next`, carry the previous print over unchanged and list it as gone. For each id in both, copy every non-`en` language block from `prev` into `next` (the import writes only English); list as changed when `Text["en"]` differs (JSON bytes). Images and core take `next`'s values (corrections already applied, so corrected fields do not churn). Header: `next`'s, with non-`en` header text carried over.
- `internal/cardset/compare_test.go` (new) — added print kept with all previous prints; changed text and image updated and listed; vanished print kept and listed; hand-added `fr` text survives with English unchanged.

Complexity: Low. Token estimate: ~12k. Agent strategy: single agent, sequential.

### Task: Import report

Requirements carried: Import report, Corrections survive re-import, Re-import picks up upstream changes. Repo: `tcgreplay`.

- `internal/cardset/report.go` (new) — `Report{Set string; Summary Summary; Items []ReportItem}`, `ReportItem{Card, Name, Kind, Field string; Values map[string]any; Status, Reason string}`; kinds and statuses as plan § Data Structures. `buildReport(merged, applied, changed, gone, final *Set, acks)`: disagreements; `missing-effect` (Trainer, or Energy with EnergyType Special, whose final `Text["en"].Effect` is empty); `no-attacks` (Pokémon with no attacks); `missing-evolves-from` (Stage `Stage1` or `Stage2` with empty English `EvolveFrom`, as the spec states; other evolving stages such as `VMAX` or `BREAK` are not checked, noted in a code comment); `no-image` (final `Images` empty); `text-changed`; `correction-redundant`; `print-gone-upstream`. Status: `corrected` when an applied correction's path covers the field (equal or prefix), `acknowledged` when an `Ack` matches card+kind (+field if set) with its reason, else `open`. Sort items by card id, kind, field. Deterministic JSON like `Write`.
- `internal/cardset/import.go` — wire in: read previous set and corrections, `apply` after merge, `compare`, `buildReport`, write `<Dir>/reports/<id>.json`, fill `Summary` (Added, Changed, Gone, Open); stdout line `me06: 141 prints, 0 added, 0 changed, 0 gone, 12 open`.
- `internal/cardset/report_test.go`, `import_test.go` — one fixture per kind raises it; clean fixture raises none; corrected and acknowledged statuses; correction survives re-import with the corrections file unchanged; report bytes identical on an unchanged re-run.

Complexity: Medium. Token estimate: ~30k. Agent strategy: single agent, sequential.

### Task: Set check command and committed-data guard

Requirements carried: Built sets are reviewable, English text everywhere (constraint), success metric 1. Repo: `tcgreplay`.

- `internal/cardset/check.go` (new) — `Check(dir string, sets ...string) ([]ReportItem, error)` (no sets → every `<dir>/sets/*.json`): `Read` + `Validate`; any `<` … `>` tag in any name, effect or attack/ability text → `markup` item; corrections file parses and every correction's card and path resolve on the built set (`apply` on a copy) → else `correction-dangling` item; structural gaps (`missing-effect`, `no-attacks`, `missing-evolves-from`, `no-image`) with acknowledgement status from the corrections file. Returns open items only. Offline: never touches the network.
- `cmd/tcgreplay/sets.go` — `check` verb: `-dir`, optional set ids; prints each open item `<set> <card> <kind> <field>`; exit 1 if any, 0 otherwise.
- `internal/cardset/committed_test.go` (new) — `TestCommittedSets`: `Check("../../cardsets")` returns no items (skip with a log line when `cardsets/sets` does not exist yet, so the guard is in place before Delta Reign lands); for `me06.json` when present: 141 prints, every Trainer and special Energy has effect text or an acknowledgement, every Stage1/Stage2 has evolveFrom, every print has a regulation mark, every print has an image source or a `no-image` acknowledgement.
- `cmd/tcgreplay/sets_test.go` — `sets check` exit codes on a clean and an open fixture directory.

Complexity: Medium. Token estimate: ~20k. Agent strategy: single agent, sequential.

### Task: Card-set import skill

Requirements carried: Agent-verified import, The assistant never commits (constraint), Verification is an agent's job, packaged as a project skill (constraint). Repo: `tcgreplay`.

- `skills/card-set-import/SKILL.md` (new; reached through the `.claude/skills` → `skills` symlink) — frontmatter `name: card-set-import`, `description:` triggering on "import a set", "build card set", "update <set> card data", "verify imported cards". Steps: (1) `go build -o bin/tcgreplay ./cmd/tcgreplay` and `bin/tcgreplay sets import <set>`; (2) read `cardsets/reports/<id>.json`, work only on `status: open` items; (3) for each, open the print's image URLs (TCGdex `<url>/high.webp`, malie jpg) and both sources' values, decide from the printed card; (4) record in `cardsets/corrections/<id>.json` a correction (card, JSON Pointer path from the item's `field`, value, reason citing the printed card) or an acknowledgement (card, kind, field, reason) — never edit `cardsets/sets/` or `cardsets/reports/` by hand; for `text-changed`, verify the new text against the card and either accept (no entry needed; it clears on the next run) or correct; for `correction-redundant`, remove the correction; (5) re-run the import until `open` is 0; (6) `bin/tcgreplay sets check <id>` passes and `go test ./internal/cardset/` passes; (7) summarise `git diff --stat cardsets/` and the notable card changes for the maintainer; (8) never `git commit`, `git add` or push. Notes: energy symbols `{R}` are acceptable text; English text is required; malie may be switched off with `-no-malie` if the maintainer says so; for `-all` runs, work set by set.
- No test file: the skill's outcome is a manual check (plan § Testing Approach).

Complexity: Low. Token estimate: ~10k. Agent strategy: single agent, sequential.

### Task: Card-set documentation

Requirements carried: Documentation for importing. Repo: `tcgreplay`.

- `cardsets/README.md` (new) — what the directory holds and that it is never built into the program; `tcgreplay sets import <set>` / `-all` / `-no-malie` / `sets check`; asking the assistant (the `card-set-import` skill); the built format (core vs `text.<lang>`, attacks by position, image sources); the corrections file with one correction and one acknowledgement example (e.g. the Legendary Pyroduct rule text, a `no-image` acknowledgement); every report kind and what resolves it; the energy-symbol rule (HTML stripped, `{R}` kept); malie's status (fan project, build time only).
- `README.md:22-49` — in "Development", a short "Card sets" paragraph and the two commands, linking `cardsets/README.md`.

Complexity: Low. Token estimate: ~8k. Agent strategy: single agent, sequential.

### Task: Ask malie.io about build-time use

A person's task (Execution: human — action outside the repo).

- What to do: on the malie.io Discord, ask whether tcgreplay may read the TCG Live export (`https://cdn.malie.io/file/malie-io/tcgl/export/index.json`) when building card sets offline, a few times a month, and whether malie's card images may be recorded and used as a backup for art TCGdex lacks.
- Record the answer (or "asked on <date>, no reply") in the knowledge entry `learnings/card-image-sources.md` through the `spek-knowledge` skill, and if the answer is no, import with `-no-malie` from then on.
- How it is checked: the knowledge entry states the outcome.

### Task: Build and verify Delta Reign

Requirements carried: Agent-verified import (first run), acceptance criteria for Delta Reign, Legendary Stadium rule text (handed to paired-stadiums). Repo: `tcgreplay`.

- `cardsets/sets/me06.json`, `cardsets/corrections/me06.json`, `cardsets/reports/me06.json` (new) — produced by following `skills/card-set-import/SKILL.md` for `me06` (decklist `DLR`).
- `cardsets/corrections/me06.json` — corrections prefixing `/text/en/effect` of the six Legendary Stadium prints (Pyroduct, Summit, Trench: two prints each, numbers from the built set, e.g. Summit 096) with "You cannot play this card by itself. You must combine 2 different Legendary <Name> from your hand to play as 1 Stadium card." (wording confirmed by the user, `gotchas/new-set-tcgdex-data-incomplete.md`), reason citing the printed card. The pair mark itself is added by the paired-stadiums spec, which extends the format and corrects it here.
- Expected report work: TCGdex has no images, effects, evolveFrom or regulation marks for Delta Reign as of 2026-11-08, so malie fills them; remaining items are disagreements and any print without art in either source.
- `internal/cardset/committed_test.go` passes with the file present (141 prints at the time of writing; if upstream has added prints since, update the count in the test to the sources' count and note it).
- Leave everything uncommitted.

Complexity: Medium. Token estimate: ~60k (reading report items and card images). Agent strategy: single agent, sequential.

### Task: Import every set in one run

Requirements carried: Every English set built (tooling). Repo: `tcgreplay`.

- `internal/cardset/import.go` — `ImportAll(ctx, opts) ([]Summary, []error, unmatched []string)`: TCGdex `/sets`, fetch every set's details (8 at a time), skip `serie.id == "tcgp"`, import each set in turn (sets sequential, cards 8 at a time inside a set), continue on error; `unmatched` = malie `en-US` keys with no TCGdex set by the matching rule in `internal/cardset/match.go`.
- `cmd/tcgreplay/sets.go` — `import -all` (mutually exclusive with a positional set); prints one summary line per set, then failures and unmatched malie sets; exit 1 if any set failed.
- `internal/cardset/import_test.go` — fakes with two sets, one Pocket and one malie-only key: both TCGdex sets written, Pocket skipped, one failing set does not stop the other, the malie-only key listed.

Complexity: Low. Token estimate: ~12k. Agent strategy: single agent, sequential.

### Task: Build and verify the Scarlet & Violet and Mega Evolution sets

Requirements carried: Every English set built (SV/ME part). Repo: `tcgreplay`.

- `cardsets/sets/<id>.json`, `cardsets/corrections/<id>.json`, `cardsets/reports/<id>.json` (new) for every TCGdex set with `serie.id` `sv` or `me` (main sets, `.5` sets, `svp`, `mep`, `sve`, `mee`, `sv10.5b`, `sv10.5w`, `30th` and its variants), via `bin/tcgreplay sets import -all` once and then the skill set by set.
- Disagreements with malie are expected to dominate; resolve by printed card. Basic Energy prints with no art: acknowledge `no-image` with a reason (the playmat borrows Crown Zenith art at run time) unless a source has art.
- `bin/tcgreplay sets check` passes for these sets; uncommitted.

Complexity: High. Token estimate: ~200k+ across sets. Agent strategy: parallel analysis, sequential integration (one agent per set or small group of sets, each touching only its own set's files; integration runs `sets check` over all).

### Task: Build and verify the Sword & Shield sets

Requirements carried: Every English set built (SWSH part). Repo: `tcgreplay`.

- `cardsets/sets/<id>.json`, `cardsets/corrections/<id>.json`, `cardsets/reports/<id>.json` (new) for every TCGdex set with `serie.id` `swsh` (including `swsh12.5` Crown Zenith and its Galarian Gallery, `swshp`), via the skill set by set (TCGdex only; malie has no SWSH sets).
- Expected items: missing images, occasional missing effect or evolveFrom, Pokémon without attacks to confirm by printed card.
- `bin/tcgreplay sets check` passes for these sets; uncommitted.

Complexity: High. Token estimate: ~150k+. Agent strategy: parallel analysis, sequential integration (one agent per group of sets, disjoint files).

### Task: Build and verify the older sets

Requirements carried: Every English set built (remainder), English text everywhere. Repo: `tcgreplay`.

- `cardsets/sets/<id>.json`, `cardsets/corrections/<id>.json`, `cardsets/reports/<id>.json` (new) for every remaining non-Pocket TCGdex English set (Sun & Moon, XY, Black & White, HeartGold SoulSilver, Platinum, Diamond & Pearl, EX, e-Card, Neo, Gym, original, promos, McDonald's and other special sets), via the skill set by set.
- Older sets have no regulation marks and may lack images; prints with no English text for an attack or effect are corrected from the printed card.
- Final: `bin/tcgreplay sets check` over all of `cardsets/` passes, `go test ./internal/cardset/` passes, and `bin/tcgreplay sets import -all` reports no missing TCGdex set. Uncommitted.

Complexity: High. Token estimate: ~250k+. Agent strategy: parallel analysis, sequential integration (one agent per series, disjoint files; final `sets check` over everything).

## Testing Strategy

Per task (all standard Go tests; no browser tests, since the board does not change):

- **Built set format** — round-trip, byte-identical double write, one-print-change diff locality, second-language byte stability, `Validate` failures, `Print.Card` conversion for Pokémon/Trainer/Energy (`internal/cardset/format_test.go`).
- **Upstream source fetchers** — fake `httptest` servers (pattern of `internal/cards/fake_tcgdex_test.go`): success, 404 → `ErrNotFound`, retry then success, retry exhaustion, timeout (`internal/cardset/fetch_test.go`).
- **Normalise and merge the sources** — table tests over trimmed real bodies in `internal/cardset/testdata/`: HTML stripped to words, `{X}` untouched, malie enum/damage/weakness mapping, `stage_text` parsing, std/ph collapse, malie-only print id, TCGdex precedence, malie fill, cosmetic differences not disagreements, image source order (`text_test.go`, `normalise_test.go`, `merge_test.go`).
- **Import command for one set** — end-to-end against fakes into a temp dir; `runMain` CLI tests for `sets import` by id and by code, `-no-malie`, identical bytes on re-run; `TestUnknownCommandRefused` still passes (`internal/cardset/import_test.go`, `cmd/tcgreplay/sets_test.go`).
- **Corrections and acknowledgements** — text/number/list/image corrections, unknown path and card errors, redundancy, corrections file bytes untouched (`corrections_test.go`).
- **Re-import comparison** — added, changed, gone prints; other-language text carried over (`compare_test.go`).
- **Import report** — one fixture per kind, corrected/acknowledged statuses, correction survives re-import, deterministic report bytes (`report_test.go`, `import_test.go`).
- **Set check command and committed-data guard** — `Check` over clean and broken fixture dirs; `sets check` exit codes; `TestCommittedSets` over `cardsets/` including the Delta Reign assertions (`check_test.go`, `committed_test.go`, `cmd/tcgreplay/sets_test.go`).
- **Card-set import skill, documentation, Ask malie.io** — no automated tests; manual checks in the implementation test plan.
- **Build and verify Delta Reign / the three catalogue tasks / Import every set in one run** — the committed-data guard and `sets check` over the produced files; `ImportAll` behaviour unit-tested against fakes.

Success metric "zero Trainers or special Energy without effect text and zero markup after verification" is a behavioural test (committed-data guard). Success metric "a new set imported and verified in one maintainer session" and the manual reviews (agent-verified import ends clean with nothing committed; spot-check corrected Delta Reign cards against printed images; follow the docs to import and correct) are Manual — captured in the implementation test plan.

## Project References

- Repo: `tcgreplay`, root `/home/nicj/code/github.com/nicholasjackson/tcgreplay` (the only registered repo; every task lands here).
- Spec: `20261108110847-d27a9f77-card-set-import`; epic `20261108105504-9abfa10e-card-set-publishing` (dependants: `20261108105504-9abfa10e-card-set-publishing`, `20261108110847-055548e3-paired-stadiums`).
- Design documents: none referenced by the spec.
- Knowledge entries (repo tier, store `tcgreplay`): `gotchas/new-set-tcgdex-data-incomplete.md`, `learnings/card-image-sources.md`, `learnings/card-text-always-english.md`, `gotchas/tcg-live-log-card-codes.md`, `gotchas/tcgdex-cache-never-expires.md`, `learnings/upstream-card-source-formats.md` (written during this plan's discovery), `conventions/skills-text-and-packaging.md` (considered, not applicable).
- Requirement → files: Import a set / Update / Clean up text / Image source → `internal/cardset/{fetch,tcgdex,malie,text,normalise,match,merge,compare,import}.go`, `cmd/tcgreplay/{main,sets}.go`; Import report / Corrections → `internal/cardset/{report,corrections,check}.go`; Room for other languages / Reviewable → `internal/cardset/format.go`, `cardsets/`; Agent-verified import → `skills/card-set-import/SKILL.md`; Every English set → `cardsets/sets/*.json` and friends; Documentation → `README.md`, `cardsets/README.md`.

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

The three catalogue tasks are far above the High tier: split them per set or per small group of sets, one agent each, every agent touching only its own sets' files under `cardsets/`, then integrate with one `sets check` over everything.

## Migration Notes

- No migration of existing data: the playmat's cache and behaviour are untouched by this plan. `cardsets/` is new.
- `.dockerignore` gains `cardsets`, so the image build context shrinks rather than grows.
- The built format starts at `format: 1`. A later spec that adds a field (the pair mark) adds an optional field without bumping the format; only a breaking change bumps it.

## Performance Considerations

- A single set import makes one TCGdex request per print (Delta Reign: 141) at eight at a time, plus two malie requests; seconds to a minute.
- An all-sets import makes about 20,000 TCGdex card requests plus ~200 set requests at eight at a time, run once by the maintainer; expect tens of minutes. Retries back off so a TCGdex hiccup does not hammer it.
- The committed catalogue is roughly 20,000 prints as pretty JSON, tens of MB in the repository. The committed-data test reads all of it on each `go test ./...`; reading and validating that much JSON takes a few seconds, acceptable for the suite. Publishing compresses it (card-set-publishing spec).
