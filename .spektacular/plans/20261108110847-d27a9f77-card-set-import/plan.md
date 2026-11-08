---
created_date: "2026-11-08"
document_status: final
closed_date: "2026-11-08"
---

# Plan: 20261108110847-d27a9f77-card-set-import

<!-- Metadata -->
<!-- Created: 2026-11-08T11:27:04Z -->
<!-- Commit: 1b8fc21 -->
<!-- Branch: main -->
<!-- Repository: git@github.com:nicholasjackson/tcgreplay.git -->

## Overview

The maintainer gains a `tcgreplay sets` command that builds any English card set ahead of time from TCGdex, filling TCGdex's gaps from the TCG Live export on malie.io, and writes it into the repository as readable JSON with a report of everything still in doubt. A project skill has the coding assistant check each doubtful card against its printed image and record corrections, which every later import keeps. The result is a complete, reviewed catalogue of every English set kept with the code, ready for the card-set-publishing spec to serve to the playmat, so players never see half-finished upstream data such as Delta Reign's missing Trainer text.

## Conventions

No project conventions apply to this feature. The only convention in the knowledge base (`conventions/skills-text-and-packaging.md`) governs the coaching guides in `internal/guides` and their packaging; this feature adds a project skill under `skills/` and data under `cardsets/`, and touches neither `internal/guides` nor how it is embedded.

## Architecture & Design Decisions

All work lands in the single registered repo, `tcgreplay` (`/home/nicj/code/github.com/nicholasjackson/tcgreplay`). The feature adds a build-time pipeline in a new Go package, `internal/cardset`, reached through a new `sets` subcommand on the existing binary: `tcgreplay sets import <set>` (or `-all`) and `tcgreplay sets check [<set>...]`. The binary today refuses every positional word (`cmd/tcgreplay/main.go:126-171`), so `main` dispatches `os.Args[1] == "sets"` to the subcommand before the playmat's flags are parsed; every other word is still refused. Built data lives in a new top-level `cardsets/` directory, outside `internal/`, so no `go:embed` can pull it into the program ("data in the bucket, logic and presentation in the binary"): `cardsets/sets/<set-id>.json` (the built set), `cardsets/corrections/<set-id>.json` (hand corrections, never written by the import) and `cardsets/reports/<set-id>.json` (the latest import report). Files are named by TCGdex set id (`me06`); the import accepts either the id or the decklist code (`DLR`).

The import is a straight pipeline with each stage a pure function over plain values, so every stage is tested without the network: **fetch** (TCGdex set and per-print card bodies, and the malie.io TCG Live set file, through a small uncached HTTP fetcher of the package's own, eight requests at a time with retries) → **normalise** (each source's print into one common candidate shape: HTML stripped and entities unescaped, malie's upper-case enums mapped to TCGdex's spellings, malie's two entries per print collapsed to the `_std_` one, malie prints matched to TCGdex ids by set `abbr` and collector number, falling back to `cards.LogCodeID` on `ext.tcgl.cardID`) → **merge** (one definition per print over the union of both sources' prints; TCGdex wins every field it has, malie fills fields TCGdex lacks, including evolves-from parsed from `stage_text`, regulation mark, effect text and images; both-present-but-different fields are recorded as disagreements) → **correct** (the set's corrections applied by JSON Pointer on the print's JSON form, so any field the format knows can be corrected, and a path the format does not know fails loudly) → **compare** (against the previous built set: changed English text is reported, prints that vanished upstream are kept unchanged and reported, other languages' text already in the file is carried over untouched) → **check** (the report's gap kinds) → **write** (the built set and the report as deterministic, two-space-indented JSON with fixed key order and prints sorted by collector number, so a change to one card diffs as that card's lines only). The importer has its own fetcher rather than reusing `cards.Client` because that client serves stale cache entries first and the card-set-publishing spec in this epic deletes its TCGdex fetch (`research.md#alternatives-considered-and-rejected`).

The built format is the playmat's own: a set header plus a list of prints, each print holding its language-independent core (id, number, category, HP, types, stage, retreat, attack costs and damage, ability kinds, weaknesses, resistances, Trainer and Energy kinds, regulation mark, image sources) once, and a `text` object keyed by language (`"en"`: name, evolves-from, attack and ability names and effects, effect), with attacks and abilities lined up by position. English is required for every print; other languages are allowed by the format, never written by the import, and preserved across re-imports. A conversion `Print.Card(lang)` turns a print into today's `cards.Card`, which the publishing spec builds on and which `sets check` uses to verify English text. Images are recorded as an ordered list of `{source, url}` (TCGdex first, malie second), so the board can fall back through them in order (decided in the card-set-publishing plan). (The card-set-publishing plan extends `Print.Card` and `cards.Card` to carry the whole ordered image list, and the board falls back through it.) Upstream fields that churn (`pricing`, `updated`) or that the playmat never reads are not kept (`learnings/upstream-card-source-formats.md`).

The report is the contract with the new project skill `skills/card-set-import/SKILL.md` (reached by assistants through the `.claude/skills` symlink): a machine-readable list of items, each with a card, a kind (`disagreement`, `missing-effect`, `no-attacks`, `missing-evolves-from`, `no-image`, `text-changed`, `correction-redundant`, `print-gone-upstream`), the field, the sources' values and a status (`open`, `corrected` or `acknowledged` with the reason taken from the corrections file's `acknowledged` list). The skill keeps no list of its own: it runs the import, checks each open item against the printed card image and the sources, writes corrections or acknowledgements with reasons, re-runs until no item is open, summarises the built-set diff, and never commits. `sets check` re-runs the offline checks (markup, English text, gaps, corrections that no longer resolve) over committed built sets and exits non-zero on any open item, and a Go test runs it over everything in `cardsets/`, so a later hand edit cannot quietly break the catalogue. Rejected options (reusing the caching client, whole-card overrides, per-language files, rewriting `{X}` energy symbols as words, a corrections CLI) are in `research.md#alternatives-considered-and-rejected`.

## Component Breakdown

- **Built set format (new, `cardset` package).** Owns the playmat's own card-set data shape: a set header and a list of prints, each print a language-independent core plus a `text` object keyed by language, attacks and abilities lined up by position, and an ordered list of image sources. Owns deterministic reading and writing (fixed key order, prints sorted by collector number, two-space indent, trailing newline) and the conversion of a print into the runtime card definition (`cards.Card`) for a given language. Every other component reads or writes sets through it; the card-set-publishing spec builds the playmat's loader on it.
- **Upstream fetchers (new).** One for TCGdex (set list, set details, per-print card bodies) and one for the malie.io TCG Live export (index and set files). Uncached, bounded concurrency, a per-request timeout and a few retries, with a configurable base address so tests point them at fake servers. They return raw upstream bodies and know nothing about the built format. Independent of the playmat's caching card client on purpose.
- **Normaliser (new).** Turns each source's raw print into one common candidate shape: strips HTML and unescapes entities, maps malie's upper-case enums and damage and weakness shapes to TCGdex spellings, parses evolves-from out of malie's `stage_text`, collapses malie's two entries per print to the standard one, and matches malie sets and prints to TCGdex ids. Drops upstream fields the playmat does not use.
- **Merger (new).** Combines the candidates into one definition per print over the union of both sources' prints. TCGdex wins any field it has; malie fills the rest. Records every field where both sources have different values (compared with whitespace, quote and energy-symbol differences ignored) as a disagreement, and which source supplied each image.
- **Corrections (new).** Owns the per-set corrections file: field corrections (card, JSON Pointer path, value, reason) and acknowledgements (card, report kind, optional field, reason). Applies corrections to merged prints, refusing a correction whose card or path does not exist in the format, and tells the checker which corrections the sources now match. Read by the import, never written by it.
- **Comparer (new).** Compares a fresh build against the previous built set: finds prints whose English text changed, keeps prints that vanished upstream, and carries over non-English text already in the file.
- **Checker and report (new).** Runs the gap checks (disagreements, Trainers and special Energy without effect, Pokémon without attacks, evolved Pokémon without evolves-from, prints without an image, changed text, redundant corrections, vanished prints) and the integrity checks (no HTML in text, English text present, text lined up with the core, corrections resolve). Produces the machine-readable report with each item's status (`open`, `corrected`, `acknowledged`) and writes it deterministically. The offline checks also power `sets check`.
- **Importer (new).** Orchestrates one set's import end to end (resolve the set name, fetch, normalise, merge, correct, compare, check, write the built set and report) and the all-sets run (every TCGdex English set except Pocket, plus the list of malie sets with no TCGdex match). Returns a short summary (prints, added, changed, open items).
- **`sets` subcommand (changed: the `tcgreplay` command line).** The binary gains `sets import <set>|-all [-no-malie] [-dir]` and `sets check [<set>...] [-dir]`, dispatched before the playmat's own flags; any other positional word is still refused.
- **Card-set import skill (new project skill).** The assistant's playbook for an agent-verified import: run the import, work through every open report item against the printed card image and the sources, record corrections or acknowledgements with reasons, re-run until nothing is open, run `sets check`, summarise the built-set diff for the maintainer, and never commit. It reads the report rather than keeping any list of its own.
- **Committed card data (new, `cardsets/`).** The built sets, corrections and reports for every English set, kept as reviewable text, and excluded from the container build context.
- **Documentation (changed).** The README's development section and a `cardsets/README.md` describe importing, verifying, the report kinds, the format, and how to record a correction or acknowledgement.

## Data Structures & Interfaces

**Built set file** (`cardsets/sets/<set-id>.json`). The playmat's own card-set format; one file per set. The card-set-publishing spec loads it; the paired-stadiums spec adds a field to `Print` for the pair mark.

```go
type Set struct {
    Format int       `json:"format"`          // 1; bumped only on a breaking change
    Set    SetHeader `json:"set"`
    Prints []Print   `json:"prints"`          // sorted by collector number
}

type SetHeader struct {
    ID           string                `json:"id"`           // TCGdex set id, "me06"
    Serie        string                `json:"serie"`        // "me"
    ReleaseDate  string                `json:"releaseDate"`
    Abbreviation string                `json:"abbreviation,omitempty"` // decklist code, "DLR"
    TCGOnline    string                `json:"tcgOnline,omitempty"`
    Text         map[string]SetText    `json:"text"`         // "en" required: {name}
}

type Print struct {
    // language-independent core
    ID, LocalID, Category       string      // "me06-001", "001", Pokemon|Trainer|Energy
    HP                          int
    Types                       []string
    Stage, Suffix               string
    Attacks                     []AttackCore   // {cost, damage}, by position
    Abilities                   []AbilityCore  // {type}, by position
    Weaknesses, Resistances     []cards.TypeValue
    Retreat                     int
    TrainerType, EnergyType     string
    RegulationMark              string
    Images                      []Image        // [{source: "tcgdex"|"malie"|..., url}], preferred first
    Sources                     []string       // which upstream sources listed this print
    // wording, per language
    Text map[string]PrintText `json:"text"`    // "en" required
}

type PrintText struct {
    Name       string        `json:"name"`
    EvolveFrom string        `json:"evolveFrom,omitempty"`
    Attacks    []MoveText    `json:"attacks,omitempty"`   // {name, effect}, same order as core
    Abilities  []MoveText    `json:"abilities,omitempty"`
    Effect     string        `json:"effect,omitempty"`
}

func (p Print) Card(lang string) (*cards.Card, error) // runtime definition in one language
func Read(path string) (*Set, error)
func Write(path string, s *Set) error                 // deterministic bytes
```

**Corrections file** (`cardsets/corrections/<set-id>.json`). Written by people and the skill, read by the import, never written by it.

```json
{
  "corrections": [
    {"card": "me06-094", "path": "/text/en/effect", "value": "…", "reason": "Pair rule printed on the card; neither source has it"}
  ],
  "acknowledged": [
    {"card": "me06-050", "kind": "no-image", "reason": "No source has art yet; TCGdex adds images weeks after release"}
  ]
}
```

`path` is an RFC 6901 JSON Pointer into the print's JSON form; `value` is any JSON value. An acknowledgement may also name a `field` to cover one disagreement on a card rather than all of them.

**Import report** (`cardsets/reports/<set-id>.json`). The contract with the skill; deterministic, no timestamps.

```json
{
  "set": "me06",
  "summary": {"prints": 141, "added": 0, "changed": 0, "gone": 0, "open": 12},
  "items": [
    {"card": "me06-089", "name": "Aarune", "kind": "missing-effect", "field": "/text/en/effect",
     "values": {"tcgdex": "", "malie": ""}, "status": "open"},
    {"card": "me06-050", "name": "…", "kind": "no-image", "status": "acknowledged", "reason": "…"}
  ]
}
```

Kinds: `disagreement`, `missing-effect`, `no-attacks`, `missing-evolves-from`, `no-image`, `text-changed`, `correction-redundant`, `print-gone-upstream`. Statuses: `open`, `corrected` (a correction covers the field), `acknowledged` (a reason is recorded).

**Internal pipeline contracts** (package-private): `candidate` (one source's normalised print, the same shape as `Print` plus the source name), `merged` (a `Print` plus its disagreements), and the fetcher interfaces, each returning raw bodies so tests can serve fixtures:

```go
type tcgdexSource interface {
    Sets(ctx) ([]tcgdexSetSummary, error)
    Set(ctx, id string) (tcgdexSet, error)
    Card(ctx, id string) (json.RawMessage, error)
}
type malieSource interface {
    Index(ctx) (malieIndex, error)          // en-US only
    SetFile(ctx, path string) ([]malieCard, error)
}
func Import(ctx, opts ImportOptions) (Summary, error) // opts: Set or All, Dir, NoMalie, sources
func Check(dir string, sets ...string) ([]ReportItem, error)
```

**Command line** (`tcgreplay sets …`): `sets import <set-id|code> [-dir cardsets] [-no-malie]`, `sets import -all [-dir cardsets] [-no-malie]`, `sets check [<set>...] [-dir cardsets]`. Exit 0 on success; `sets check` exits 1 when any item is open; import exits 0 with open items (they are the report's job) and non-zero only on errors.

## Implementation Detail

**A new module boundary: build-time card data.** Until now every card definition reached the playmat at run time through the caching TCGdex client. This plan introduces a second, separate path that runs only on the maintainer's machine: a `cardset` package that owns the built format and the import pipeline, with no dependency on the cache, the tiers or the admin clear. It depends on the `cards` package only for the runtime card type it converts into and for the existing log-code-to-id mapping. A developer reading the code finds "how card data is built" entirely in one package, and "how the playmat serves cards" unchanged in `cards` until the publishing spec replaces it.

**A staged pipeline of pure functions.** The import is a chain of small stages, each taking and returning plain values: fetch, normalise, merge, correct, compare, check, write. Only the first and last touch the outside world. Each stage gets its own file and table-driven tests over small fixtures, so a reader can follow a field from upstream body to built file one stage at a time, and a bug in, say, malie's damage suffixes is fixed and tested in the normaliser alone. This is new to the codebase, which elsewhere mixes I/O and logic inside one client; it is chosen because the import's behaviour is mostly data transformation that must be checked exhaustively without the network.

**Data and code kept apart.** Card data sits in its own top-level directory as readable JSON, outside the Go packages, so it can never be embedded into the binary and the container build context leaves it out. Hand knowledge (corrections and acknowledgements with reasons) sits in a file of its own that only people and the skill write, so no import can lose it. The report is regenerated on every import and is the only thing the skill reads to decide what to check.

**Deterministic output as a design rule.** Every written file is a pure function of its inputs: fixed key order, sorted prints, no timestamps, no upstream fields that change daily, a trailing newline. Re-running an import against unchanged upstream data rewrites identical bytes, so a git diff after an import shows only real changes, one card's lines at a time. Tests assert this byte-for-byte.

**Field-level corrections by path.** Corrections address a single field of a print by JSON Pointer over the print's JSON form, applied after merging and before comparison. Applying a correction round-trips the print through JSON and decodes it strictly, so a path the format does not define is an error rather than a silently ignored key; a later spec that needs a new card field (the pair mark) adds it to the format and can then correct it. The same path is how the report says which field an item is about, so the skill can turn a report item into a correction mechanically.

**Following existing patterns.** The command-line tests follow the existing re-exec-the-test-binary pattern; the fetchers' tests follow the existing fake-upstream `httptest` server pattern; bounded concurrency follows the catalog's eight-at-a-time set loading; set-code collisions are settled by the catalog's shorter-id rule; and malie card ids reuse the existing TCG Live log-code mapping rather than a second copy of it.

**A guard over committed data.** A Go test reads every built set in the repository and runs the offline checks over it, so the project's test suite fails if a hand edit leaves markup, missing English text, misaligned text, a dangling correction, or an unacknowledged gap. This makes the committed catalogue part of what the test suite protects.

**The skill as the human-in-the-loop layer.** Mechanical work stays in code; judgement (is this disagreement a source error, is this card really without attacks, what does the printed card say) lives in a project skill that reads the report, records its decisions in the corrections file with reasons, and loops until the report has nothing open. The skill never commits; it ends with a summary of the built-set diff for the maintainer to review.

## Dependencies

- **Design documents this plan was built on: none.** The spec carries no design references.
- **Prior specs or plans that must land first: none.** This is the first spec of the epic; card-set-publishing and paired-stadiums depend on it, not the other way round.
- **`cards` package (internal).** Provides the runtime card type (`cards.Card` and its attack, ability, damage and type-value types) that built prints convert into, and `cards.LogCodeID`, reused to map malie card ids to TCGdex ids. No changes needed. The publishing spec later removes this package's TCGdex fetch; the importer does not use it, so it is unaffected.
- **`tcgreplay` command line (internal).** Gains the `sets` subcommand; its existing flag handling and the refusal of other positional words stay as they are.
- **Go standard library.** `net/http`, `encoding/json`, `html` (entity unescaping) and `regexp` for tag stripping, `net/http/httptest` in tests. No new third-party module: HTML in card text is a handful of inline tags, so a full HTML parser is not needed.
- **TCGdex REST API v2 (external service).** The main source: set list, set details and per-print card bodies, English only. Used at build time only, by the maintainer. No account or key.
- **malie.io TCG Live export (external service).** Gap-filling source for Scarlet & Violet and Mega Evolution sets: English text, regulation marks, evolves-from and images. A fan project with an alpha format; used at build time only, switchable off with `-no-malie`. Asking its maintainers on Discord before relying on it is a task for a person in this plan; the build does not wait on the answer.
- **The assistant's project skills directory.** The new skill lives with the project's other skills and is picked up through the existing `.claude/skills` link. No change to how skills are installed.
- **Knowledge entries relied on:** `gotchas/new-set-tcgdex-data-incomplete.md` (Delta Reign's gaps, the Legendary Stadium rule neither source has), `learnings/card-image-sources.md` (malie terms and image URLs), `learnings/card-text-always-english.md` (English text is what game logic reads), `gotchas/tcg-live-log-card-codes.md` (log-code mapping and set-code collisions), `learnings/upstream-card-source-formats.md` (both sources' shapes, written during this plan's discovery).

## Testing Approach

Tests are standard Go tests in the new package and the command's package, run with the rest of the project's Go suite. No browser tests: nothing on the board changes.

**Unit tests per pipeline stage (most coverage).** The normaliser, merger, corrections, comparer and checker are pure, so each gets table-driven tests over small hand-written upstream bodies. Load-bearing guarantees:

- **No markup.** HTML in attack, ability and effect text (Delta Reign's energy spans and `<em>` reminder text) comes out as plain words with entities unescaped; `{X}` energy symbols pass through unchanged.
- **Fill from the other source.** When TCGdex lacks effect text, evolves-from, regulation mark or an image and malie has it, the built print has malie's value; when TCGdex has a value, TCGdex's wins.
- **Disagreements found, noise ignored.** Different values in both sources are reported; differences only in whitespace, apostrophe style or `{R}` versus `Fire` are not.
- **Union of prints.** A print only malie lists is built, with an id from the log-code mapping; malie's reverse-holo duplicate entries never produce a second print.
- **Every report kind.** For each of the eight kinds, a fixture that should raise it does, and a clean fixture raises nothing; an acknowledged item carries its reason and is not open; a field covered by a correction shows `corrected`.
- **Corrections survive.** After a correction is applied and the set is imported again, the built value is the corrected one, the corrections file's bytes are unchanged, and a correction the sources now match is reported as redundant. A correction to an unknown path fails with a clear error.
- **Re-import keeps and updates.** A print added upstream appears; every earlier print is still present; changed upstream text or image is updated and reported as changed text, unless a correction covers the field; a print gone upstream is kept and reported.
- **Language separation.** Adding French wording to one print by hand leaves the English wording and the core byte for byte the same in the re-serialised file, and a re-import keeps the French text and leaves the English unchanged.
- **Deterministic output.** Writing the same set twice gives identical bytes; changing one print's text changes only that print's lines.
- **Runtime conversion.** A built print converts into the same `cards.Card` the playmat uses today, for Pokémon, Trainers and Energy.

**Integration tests with fake upstreams.** The importer runs end to end against `httptest` fakes of TCGdex and malie (the existing fake-upstream pattern) serving a small fixture set, writing into a temporary directory: built set, report, and re-import behaviour. One test runs with `-no-malie` to prove the build works from TCGdex plus corrections alone. Fetcher tests cover retries, timeouts and a 404.

**Command-line tests.** Through the existing re-exec-the-test-binary helper: `sets import` against fake upstreams writes the files and exits 0; `sets check` exits 1 when a set has an open item and 0 when clean; other positional words are still refused.

**Guard over committed data (regression).** A test runs the offline checks over every built set in `cardsets/`: no HTML in any text, an English name and English text for every attack, ability and effect, text lined up with the core, every correction resolving, and no open structural item. It also asserts the Delta Reign file has 141 prints, every Trainer and special Energy in it has effect text, every Stage 1 and Stage 2 has evolves-from, and every print has a regulation mark and an image source or an acknowledgement.

**Deliberate gaps.** No automated test calls the real TCGdex or malie; real-source behaviour is covered by running the import for Delta Reign and every set during implementation, whose committed output the data guard then checks. The skill's judgement is not unit-tested; its outcome is checked by hand.

**Success metrics.**

- *After agent verification of a set, zero Trainers or special Energy lack effect text and zero cards carry markup.* **Behavioural test** — the guard over committed data fails on any Trainer or special Energy without effect text (unless acknowledged with a reason) and on any HTML in any built set's text.
- *A new set can be imported and verified in one maintainer session.* **Manual — captured in the implementation test plan.**

**Manual reviews.**

- **Manual — captured in the implementation test plan:** an agent-verified import of a set, run through the skill, ends with every report item resolved or carrying a reason, a summary of the built-set changes, and nothing committed by the assistant.
- **Manual — captured in the implementation test plan:** spot-check a sample of corrected and acknowledged Delta Reign cards against their printed images to confirm the skill's corrections are right.
- **Manual — captured in the implementation test plan:** review the README and `cardsets/README.md` instructions by following them to import a set and record a correction.

## Milestones & Tasks

### Milestone 1: The maintainer can build a card set into the project

**What changes**: The maintainer runs one command, naming a set by its id or decklist code, and the project gains a readable built set holding one definition per print, gathered from TCGdex and filled in from the TCG Live export where TCGdex has gaps. Text carries no HTML, every print names its image sources, English wording sits apart from the language-independent facts, and re-running the command against unchanged sources rewrites identical bytes. Nothing on the playmat changes yet: it still reads cards on demand until the publishing spec switches it over.

**Validation point**: Importing a fixture set against fake sources writes the expected built set and passes the stage tests; a real import of Delta Reign writes 141 prints, each with a name, category and set, with no HTML in any text.

#### - [x] Task: Built set format
**Id:** bf63f650-0e23-49a9-9406-61c923d47832
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Adds the playmat's own card-set format in a new `cardset` package: a set header and a list of prints, each with its language-independent facts stored once and its wording in a per-language text block, plus an ordered list of image sources. Reading and writing are deterministic so built sets diff cleanly, and each print converts into the card definition the playmat already uses.

*Technical detail:* [context.md#task-built-set-format](./context.md#task-built-set-format)

**Acceptance criteria**:
- [x] A built set holds HP, types, attack costs, damage, retreat and stage once per print, apart from its English wording.
- [x] Writing the same set twice produces identical bytes, and changing one print's text changes only that print's lines.
- [x] Adding a second language's wording to one print leaves that print's English wording and language-independent facts byte for byte unchanged.
- [x] A set with a print lacking an English name or English text for one of its attacks, abilities or effect is reported as invalid.
- [x] A built print converts into the same card definition the playmat uses today, for Pokémon, Trainers and Energy.

#### - [x] Task: Upstream source fetchers
**Id:** 876df490-ccfd-402a-a2f2-2744e9715227
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Adds two small fetchers that read raw data from TCGdex (set list, set details, each print's card) and from the malie.io TCG Live export (index and set files) with no caching, a few requests at a time, with timeouts and retries. They are separate from the playmat's caching card client, which the publishing spec retires, so the import always sees current upstream data.

*Technical detail:* [context.md#task-upstream-source-fetchers](./context.md#task-upstream-source-fetchers)

**Acceptance criteria**:
- [x] The TCGdex fetcher returns the English set list, a set's details and a print's card body, and reports a missing set or card as not found.
- [x] The malie fetcher returns the English index and a set's card list.
- [x] A slow or failing request is retried a few times and then fails with an error naming what was fetched, without hanging the import.
- [x] Both fetchers can be pointed at stand-in servers, so no test needs the network.

#### - [x] Task: Normalise and merge the sources
**Id:** 9dc57a4a-2c92-403a-be6a-9800a2d2d0df
**Repo:** tcgreplay
**Depends on:**
- bf63f650-0e23-49a9-9406-61c923d47832 — Built set format
**Execution:** agent

Turns each source's raw cards into one common shape and combines them into one definition per print. HTML is stripped from text, malie's spellings are mapped to TCGdex's, malie's duplicate reverse-holo entries are collapsed, and malie prints are matched to TCGdex ids. TCGdex wins every field it has, malie fills the gaps (effect text, evolves-from, regulation mark, images), prints either source lists are all kept, and fields where both sources have different values are recorded as disagreements.

*Technical detail:* [context.md#task-normalise-and-merge-the-sources](./context.md#task-normalise-and-merge-the-sources)

**Acceptance criteria**:
- [x] Text that arrived with HTML energy symbols or emphasis reads as plain words, with no tags left in any attack, ability or effect text.
- [x] A Trainer or special Energy without effect text in TCGdex gets malie's effect text; an evolved Pokémon without evolves-from gets it from malie; a print without a regulation mark or image gets malie's.
- [x] Where TCGdex has a value, the built print keeps TCGdex's value.
- [x] A print listed only by malie is built, and malie's reverse-holo entries never produce a second print.
- [x] Fields where the sources differ are recorded as disagreements, while differences only in spacing, apostrophe style or energy-symbol spelling are not.
- [x] Every print records each source that has an image for it, TCGdex first.

#### - [x] Task: Import command for one set
**Id:** 1820fbf4-0de2-40a2-b822-a05d64a511c5
**Repo:** tcgreplay
**Depends on:**
- 876df490-ccfd-402a-a2f2-2744e9715227 — Upstream source fetchers
- 9dc57a4a-2c92-403a-be6a-9800a2d2d0df — Normalise and merge the sources
**Execution:** agent

Adds `tcgreplay sets import <set>`, which takes a set's TCGdex id or decklist code, fetches it from both sources, merges it and writes the built set into the project's card-set directory. A `-no-malie` switch builds from TCGdex alone. The card-set directory is kept out of the container build, and every other unknown word on the command line is still refused.

*Technical detail:* [context.md#task-import-command-for-one-set](./context.md#task-import-command-for-one-set)

**Acceptance criteria**:
- [x] Importing a set by its id or by its decklist code writes one built set file for it, holding one definition per print the sources list, each with its name, category and set.
- [x] Importing with malie switched off still writes a built set from TCGdex alone.
- [x] Re-importing against unchanged sources rewrites identical bytes.
- [x] The playmat's own start-up and its refusal of other unknown words are unchanged.
- [x] Card-set data never enters the container image or the binary.

### Milestone 2: Imports say what still needs attention and keep hand corrections

**What changes**: Every import now writes a report listing each kind of gap: sources disagreeing, Trainers and special Energy without effect text, Pokémon without attacks, evolved Pokémon without evolves-from, prints without an image, text changed since the last import, corrections the sources now agree with, and prints gone from the sources. The maintainer can record a correction to any card field, or an acknowledgement with a reason, in a per-set corrections file that every later import applies and never rewrites. Re-imports add new prints, update changed text and art, and keep every earlier print. A check command and a test guard every committed built set.

**Validation point**: The report, correction, re-import and check tests pass; re-importing Delta Reign after recording a correction keeps the corrected value and leaves the corrections file unchanged.

#### - [x] Task: Corrections and acknowledgements
**Id:** 9ca89fea-2dae-45c5-8f01-e706ef23b6bf
**Repo:** tcgreplay
**Depends on:**
- bf63f650-0e23-49a9-9406-61c923d47832 — Built set format
**Execution:** agent

Adds the per-set corrections file: field corrections naming a card, the field's path, its value and a reason, and acknowledgements recording why a reported item needs no change. Corrections are applied to every freshly merged print; the import never writes the file. A correction naming a card or field that does not exist is an error, and a correction the sources now match is noticed.

*Technical detail:* [context.md#task-corrections-and-acknowledgements](./context.md#task-corrections-and-acknowledgements)

**Acceptance criteria**:
- [x] A correction to any field of a print, including text, numbers, lists and image sources, shows in the built print.
- [x] A correction to a field the format does not have, or to a card the set does not hold, fails with a message naming it.
- [x] Applying corrections never changes the corrections file.
- [x] A correction whose value the merged sources now provide is identified as redundant.

#### - [x] Task: Re-import comparison
**Id:** 2b246808-02b1-49dc-b8d8-aabf794c68cd
**Repo:** tcgreplay
**Depends on:**
- bf63f650-0e23-49a9-9406-61c923d47832 — Built set format
**Execution:** agent

Compares a fresh build with the set already in the project. New prints are added, changed upstream text and images replace the old values, prints that vanished upstream are kept, and wording in other languages already in the file is carried over untouched. It lists the prints whose English text changed and those gone upstream for the report.

*Technical detail:* [context.md#task-re-import-comparison](./context.md#task-re-import-comparison)

**Acceptance criteria**:
- [x] After upstream gains a print, the re-imported set holds it and every print that was already there.
- [x] After upstream changes a card's text or image, the re-imported set shows the new value, unless a correction covers that field.
- [x] A print upstream no longer lists stays in the set and is listed as gone.
- [x] A second language's wording added by hand survives a re-import, with the English wording unchanged.

#### - [x] Task: Import report
**Id:** 4f2a5073-e9f7-465f-ad9c-384f065888cd
**Repo:** tcgreplay
**Depends on:**
- 1820fbf4-0de2-40a2-b822-a05d64a511c5 — Import command for one set
- 9ca89fea-2dae-45c5-8f01-e706ef23b6bf — Corrections and acknowledgements
- 2b246808-02b1-49dc-b8d8-aabf794c68cd — Re-import comparison
**Execution:** agent

Wires corrections and comparison into the import and makes every import write a machine-readable report beside the built set. The report lists each item still needing attention, by card, kind and field, with the sources' values and whether it is open, covered by a correction or acknowledged with a reason. The command prints a short summary of prints, additions, changes and open items.

*Technical detail:* [context.md#task-import-report](./context.md#task-import-report)

**Acceptance criteria**:
- [x] The report names a card for each kind of gap: a source disagreement, a Trainer or special Energy with no effect text in any source, a Pokémon with no attacks, an evolved Pokémon with no evolves-from, a print with no image, text changed since the last import, a correction the sources now match, and a print gone upstream.
- [x] An item covered by a correction shows as corrected, and an acknowledged item shows its recorded reason; neither counts as open.
- [x] After a correction is recorded and the set is imported again, the built card still shows the corrected value and the corrections file is unchanged.
- [x] The report is identical when the import is re-run against unchanged sources.

#### - [x] Task: Set check command and committed-data guard
**Id:** fceb88b2-d1f0-40b5-945b-5716447d8008
**Repo:** tcgreplay
**Depends on:**
- 4f2a5073-e9f7-465f-ad9c-384f065888cd — Import report
**Execution:** agent

Adds `tcgreplay sets check`, which re-runs the offline checks over built sets in the project (no markup, English text everywhere, text lined up with the facts, corrections that resolve, and no unacknowledged gaps) and fails when anything is open. A test runs the same checks over every committed built set, so the test suite protects the catalogue from a bad hand edit.

*Technical detail:* [context.md#task-set-check-command-and-committed-data-guard](./context.md#task-set-check-command-and-committed-data-guard)

**Acceptance criteria**:
- [x] Checking a clean set succeeds, and checking a set with an open item fails and names the item.
- [x] A built set with HTML in any text, a missing English name or text, or a correction that no longer resolves fails the check.
- [x] The project's test suite fails if any committed built set fails the check.

### Milestone 3: The assistant verifies an import, and Delta Reign is built clean

**What changes**: The maintainer can ask the coding assistant to import a set. A project skill runs the import, checks each open report item against the printed card and the sources, records corrections or acknowledgements with reasons, repeats until nothing is open, and ends with a summary of what changed, without committing. The documentation explains importing, verifying and recording a correction. Delta Reign is the first set built this way: every Trainer and special Energy has effect text, every evolved Pokémon has evolves-from, every print has a regulation mark, and the Legendary Stadiums carry their missing rule text.

**Validation point**: Delta Reign's built set, corrections and clean report are in the working tree, the committed-data guard passes over it, and the skill's run ended with no open item and nothing committed.

#### - [x] Task: Card-set import skill
**Id:** 21685491-95d7-4c15-8b8f-d3fffc3833e1
**Repo:** tcgreplay
**Depends on:**
- fceb88b2-d1f0-40b5-945b-5716447d8008 — Set check command and committed-data guard
**Execution:** agent

Adds a project skill the coding assistant follows when the maintainer asks it to import a set. It runs the import, works through every open report item against the printed card image and both sources, records corrections or acknowledgements with reasons, re-runs until nothing is open, runs the check, and summarises what changed in the built set. It never commits and keeps no list of its own beyond the report.

*Technical detail:* [context.md#task-card-set-import-skill](./context.md#task-card-set-import-skill)

**Acceptance criteria**:
- [x] Asking the assistant to import a set triggers the skill.
- [x] The skill tells the assistant to check each open item against the printed card and the sources, and to record a correction or an acknowledgement with a reason for it.
- [x] The skill repeats the import until the report has nothing open and the check passes.
- [x] The skill ends with a summary of the built-set changes and forbids committing.

#### - [x] Task: Card-set documentation
**Id:** c37fcab9-ed2f-4207-b74f-52eb94d6662d
**Repo:** tcgreplay
**Depends on:**
- fceb88b2-d1f0-40b5-945b-5716447d8008 — Set check command and committed-data guard
**Execution:** agent

Documents the card-set workflow for maintainers: how to import a set, how to have the assistant verify it, what each report kind means, the built format, and how to record a correction or an acknowledgement. The README's development section points to a fuller guide kept beside the data.

*Technical detail:* [context.md#task-card-set-documentation](./context.md#task-card-set-documentation)

**Acceptance criteria**:
- [x] The documentation describes importing a set and having the assistant verify it.
- [x] The documentation describes recording a correction and an acknowledgement, with an example of each.
- [x] The documentation lists every report kind and what resolves it.

#### - [x] Task: Ask malie.io about build-time use
**Id:** 58104bf7-2374-4dcb-adda-47c2c8b3442c
**Repo:** tcgreplay
**Depends on:** none
**Execution:** human — action outside the repo: asking the malie.io maintainers on their Discord

The maintainer asks the malie.io maintainers whether the project may read their TCG Live export when building card sets, and whether its images may be used as a backup for card art. The import uses malie by default and can run without it, so this does not block the build; the answer decides whether malie stays switched on at build time. The board shows every listed image source in order regardless (the user's decision of 2026-11-08, recorded in the card-set-publishing plan).

*Technical detail:* [context.md#task-ask-malie.io-about-build-time-use](./context.md#task-ask-malie.io-about-build-time-use)

**Acceptance criteria**:
- [x] The malie.io maintainers have been asked, and their answer (or the lack of one) is noted in the knowledge entry on card image sources.

#### - [x] Task: Build and verify Delta Reign
**Id:** 63b04137-17cd-40e5-8163-6fe4c7592dcc
**Repo:** tcgreplay
**Depends on:**
- 21685491-95d7-4c15-8b8f-d3fffc3833e1 — Card-set import skill
- c37fcab9-ed2f-4207-b74f-52eb94d6662d — Card-set documentation
**Execution:** agent

Runs the skill for Delta Reign, the newest set and the one with the worst upstream gaps. Every report item is resolved or acknowledged, including the Legendary Stadiums' pair rule that neither source carries, which is recorded as a text correction. The built set, its corrections and its clean report are left uncommitted for the maintainer to review.

*Technical detail:* [context.md#task-build-and-verify-delta-reign](./context.md#task-build-and-verify-delta-reign)

**Acceptance criteria**:
- [x] The project holds a Delta Reign built set with one definition for every print the sources list (141 at the time of writing).
- [x] Every Trainer and special Energy has effect text, every Stage 1 and Stage 2 Pokémon has evolves-from, and every print has a regulation mark.
- [x] Every print with an image in either source names that source, and every print with none is in the report.
- [x] The three Legendary Stadiums' English text includes the rule that two different prints must be played together.
- [x] Delta Reign's final report has nothing open, and nothing was committed.

### Milestone 4: Every English set is built and verified

**What changes**: The whole English catalogue (every set TCGdex lists in English, Pokémon TCG Pocket aside) exists as built, verified data in the project, so the publishing spec can switch the playmat over without any set missing. Each set holds every print either source lists and has a final report with nothing open.

**Validation point**: There is a built set and clean report for every English TCGdex set, the import's all-sets run lists no set as missing, and the committed-data guard passes over the whole catalogue.

#### - [x] Task: Import every set in one run
**Id:** ac0fb619-c72a-4041-9d9a-ba3591aa8f29
**Repo:** tcgreplay
**Depends on:**
- 4f2a5073-e9f7-465f-ad9c-384f065888cd — Import report
**Execution:** agent

Adds `tcgreplay sets import -all`, which imports every English set TCGdex lists, Pokémon TCG Pocket aside, carrying on past a set that fails and ending with a summary of each set's open items and failures. It also lists any malie set that matches no TCGdex set, so nothing in either source is silently missed.

*Technical detail:* [context.md#task-import-every-set-in-one-run](./context.md#task-import-every-set-in-one-run)

**Acceptance criteria**:
- [x] An all-sets import writes a built set and report for every English TCGdex set except Pokémon TCG Pocket sets.
- [x] A set that fails to import does not stop the others, and the summary names it.
- [x] The summary lists every malie set with no TCGdex match.

#### - [x] Task: Build and verify the Scarlet & Violet and Mega Evolution sets
**Id:** 4a8176f8-15ca-4bc4-8b77-2efbef85cca8
**Repo:** tcgreplay
**Depends on:**
- ac0fb619-c72a-4041-9d9a-ba3591aa8f29 — Import every set in one run
- 63b04137-17cd-40e5-8163-6fe4c7592dcc — Build and verify Delta Reign
**Execution:** agent

Runs the skill over every Scarlet & Violet and Mega Evolution set, including promos and energy sets: the sets both sources cover, so disagreements between them are the main work. Each set ends with a clean report, and the changes are left uncommitted for review.

*Technical detail:* [context.md#task-build-and-verify-the-scarlet-&-violet-and-mega-evolution-sets](./context.md#task-build-and-verify-the-scarlet-&-violet-and-mega-evolution-sets)

**Acceptance criteria**:
- [x] Every Scarlet & Violet and Mega Evolution set TCGdex lists has a built set holding every print either source lists.
- [x] Each of those sets' final report has nothing open.

#### - [x] Task: Build and verify the Sword & Shield sets
**Id:** 67cce74f-0770-4522-b1a5-5f8764f582ef
**Repo:** tcgreplay
**Depends on:**
- ac0fb619-c72a-4041-9d9a-ba3591aa8f29 — Import every set in one run
- 63b04137-17cd-40e5-8163-6fe4c7592dcc — Build and verify Delta Reign
**Execution:** agent

Runs the skill over every Sword & Shield set, which only TCGdex covers, so missing fields and images are the main work. Each set ends with a clean report, and the changes are left uncommitted for review.

*Technical detail:* [context.md#task-build-and-verify-the-sword-&-shield-sets](./context.md#task-build-and-verify-the-sword-&-shield-sets)

**Acceptance criteria**:
- [x] Every Sword & Shield set TCGdex lists has a built set holding every print it lists.
- [x] Each of those sets' final report has nothing open.

#### - [x] Task: Build and verify the older sets
**Id:** 21d20a65-34b2-468c-bb54-3a488fda76a7
**Repo:** tcgreplay
**Depends on:**
- ac0fb619-c72a-4041-9d9a-ba3591aa8f29 — Import every set in one run
- 63b04137-17cd-40e5-8163-6fe4c7592dcc — Build and verify Delta Reign
**Execution:** agent

Runs the skill over every remaining English set, from the original sets through Sun & Moon, including promos and special sets. Only TCGdex covers these. Each set ends with a clean report, and the changes are left uncommitted for review.

*Technical detail:* [context.md#task-build-and-verify-the-older-sets](./context.md#task-build-and-verify-the-older-sets)

**Acceptance criteria**:
- [x] Every remaining English set TCGdex lists, Pokémon TCG Pocket aside, has a built set holding every print it lists.
- [x] Each of those sets' final report has nothing open.
- [ ] An all-sets check over the project's card data passes, and every card in every built set has an English name and English text for every attack, ability and effect it has.

## Open Questions

- **How noisy are TCGdex-versus-malie disagreements across the Scarlet & Violet and Mega Evolution sets?** This depends on real upstream wording across 33 sets, which only a full import shows. If one systematic difference (for example a punctuation or spelling convention) produces hundreds of items, the implementer should add that difference to the comparison's cosmetic-difference rules, with a test, rather than acknowledge each card one by one. If a difference changes meaning, it stays a disagreement.
- **Does any English set TCGdex lists fail to import (odd ids, sets with no cards, cards that 404)?** Only discoverable by running `-all`. A set with no English cards or a TCGdex error the retries do not clear: the implementer records it in the run's summary and STOPs to ask the user whether that set is out of scope, rather than silently leaving it unbuilt.
- **Do the TCG Live-only malie listings hold any print that no TCGdex set has?** Planning found `svalt` and `mealt` to be alternate finishes of existing prints and `me5-5m` to be three RGB Mew prints that may or may not be in TCGdex's `30th`; only the all-sets run over real data settles it. If a malie-only listing holds a print that no built set contains, the implementer STOPs and asks the user which set it belongs in, since corrections in this plan change fields of existing prints and do not add prints.

## Out of Scope

- **Languages other than English.** No other language is imported, published or shown; the format only leaves room for one (spec Non-Goals).
- **Japanese-only sets.** A separate catalogue with its own sets and numbering (spec Non-Goals).
- **Automatic detection of new sets or prints.** No scheduled job; the maintainer starts every import (spec Non-Goals).
- **Publishing built sets and switching the playmat to read them.** Uploading to the bucket, the manifest, runtime loading, retiring the playmat's TCGdex fetch, and the policy on which image source the board shows are the card-set-publishing spec (`20261108105504-9abfa10e-card-set-publishing`); it extends `Print.Card` and `cards.Card` to carry the whole ordered image list, and the board falls back through it. Until it lands, the playmat keeps reading cards on demand and ignores `cardsets/`.
- **The pair mark and pair play for Legendary Stadiums.** This plan records only the missing pair rule text as a correction; the data flag, its place in the format and pair play on the board are the paired-stadiums spec (`20261108110847-055548e3-paired-stadiums`).
- **Pokémon TCG Pocket sets.** A different game, already excluded from the playmat's catalogue.
- **Sets only TCG Live lists** (malie keys with no TCGdex set, such as `svalt` and `mealt`). Listed by the all-sets run, not built.
- **A command-line tool for writing corrections.** Corrections are edited as JSON by the maintainer or the skill and validated by `sets check`.
- **Rarity, illustrator, legality and prices.** Not kept in built sets; the playmat does not use them.
- **Borrowed art for prints without images.** Recording another print's art (as the playmat does at run time for Basic Energy) is not done by the import; such prints are reported and acknowledged or corrected.

## Changelog

### 2026-11-08 — Task: Built set format

**What was done**: Added the `internal/cardset` package with the built card-set format (set header, prints with a language-independent core and a per-language `text` block, ordered image sources), deterministic `Read`/`Decode`/`Write`/`Marshal`, validation, and conversion of a print into the playmat's `cards.Card`.

**Deviations**: Validation is split in two. `ValidateShape` (enforced by `Read`/`Decode`) checks structure: English name, text lined up with the core, named attacks and abilities, no wording another language has that English lacks. `Validate` adds `MissingEnglish`, which also refuses an ability, Trainer or special Energy with no English effect. A set with such a gap still decodes, so the import and checks can read it and report `missing-effect` instead of failing. `Print.Card(lang)` leaves the set reference empty and `Set.Card(id, lang)` fills it in. Conversion applies the playmat's borrowed Basic Energy art through a new exported `cards.EnergyArt` wrapper, so a converted Basic Energy matches the playmat's card exactly; the art is never stored in the built set.

**Files changed**:
- `internal/cardset/format.go`
- `internal/cardset/card.go`
- `internal/cardset/format_test.go`
- `internal/cards/cards.go`

**Discoveries**: The goal check reads "English text for ... effect" as "Trainers, special Energy and abilities need an English effect". Attack effects stay optional. The import must therefore validate only the shape before writing, or a TCGdex-only (`-no-malie`) build of a new set could not be written at all. Separately, TCGdex `/v2/en/sets` once returned an empty body in a one-off fetch, so the fetchers' retries need to treat an empty or undecodable body as retryable.

### 2026-11-08 — Task: Upstream source fetchers

**What was done**: Added uncached fetchers for TCGdex (`TCGdex.Sets`, `Set`, `Card`, and `Cards`, which fans out eight at a time) and for the malie.io export (`Malie.Index`, which returns `en-US` only, and `SetFile`). Both sit on a shared `httpGetter` with a per-attempt timeout (20s), three attempts with 500ms and 2s pauses, and errors that name the URL. The tests run against an `httptest` stand-in.

**Deviations**: The upstream types are exported (`TCGdexSetSummary`, `TCGdexSet`, `MalieSetRef`, `MalieCard`) because exported methods return them; the plan called them package-private. `Cards` returns the bodies plus a per-id error map instead of a first error. An undecodable or empty 200 body is retried as well as network errors, 429 and 5xx. Numeric malie fields decode through `flexInt` (number, numeric string or null).

**Files changed**:
- `internal/cardset/fetch.go`
- `internal/cardset/tcgdex.go`
- `internal/cardset/malie.go`
- `internal/cardset/fetch_test.go`

**Discoveries**: malie's CDN (Cloudflare) answers 403 to Python's default `Python-urllib` User-Agent, but serves `curl`, Go's default client and our `tcgreplay-sets` User-Agent. Use curl or set a User-Agent in ad-hoc scripts.

### 2026-11-08 — Task: Normalise and merge the sources

**What was done**: Added `text.go` (`cleanText` strips HTML, unescapes entities and tidies whitespace while keeping `{X}` symbols; `compareKey` and `nameKey` ignore cosmetic differences), `normalise.go` (`fromTCGdex` keeps definition fields only; `fromMalie` maps malie's enums, damage and weaknesses, evolves-from from `stage_text`, and collapses std/ph entries), `match.go` (malie set by decklist code with the shorter-id owner rule, then by log code; print ids by collector number, then by log code, then padded) and `merge.go` (TCGdex wins, malie fills gaps, disagreements recorded as JSON Pointer fields, images TCGdex first). A sub-agent wrote table tests over trimmed real Delta Reign fixtures in `internal/cardset/testdata/`.

**Deviations**: `fromTCGdex` takes only the raw body (the set is not needed). The plan's normaliser did not anticipate several malie details, now handled: the `FREE` cost is dropped (TCGdex lists no cost), the `EFFECT` text kind (Tera rule text) becomes the print's effect like `TEXT_BOX`, `EX_LOWER` maps to suffix `ex`, and `Basic {G} Energy` versus `Grass Energy` is a cosmetic name difference. When one source lists a move as an attack and the other as an ability (me06-080 Azurill), the gap is filled but recorded as an `/attacks` disagreement, so the print does not silently carry the same move twice. A zero HP or retreat counts as "not given" and is filled from malie.

**Files changed**:
- `internal/cardset/text.go`
- `internal/cardset/normalise.go`
- `internal/cardset/match.go`
- `internal/cardset/merge.go`
- `internal/cardset/malie.go`
- `internal/cardset/text_test.go`
- `internal/cardset/normalise_test.go`
- `internal/cardset/match_test.go`
- `internal/cardset/merge_test.go`
- `internal/cardset/testdata/`

**Discoveries**: On real Delta Reign data the merge records 34 disagreements and no text differences. They are TCGdex errors: suffix `EX` instead of `ex` on every ex, and stage `Basic` on evolved Mega ex. Malie entries for one print have different `cardID`s (std and ph), so grouping is by collector number, not card id. malie's `svbsp` abbr is `PR-SV` (TCGdex `SVP`), matched through the log code. `30th` and `30th-c` share `30C`.

### 2026-11-08 — Task: Import command for one set

**What was done**: Added `cardset.Import`, which resolves a set by TCGdex id or decklist/TCG Online code (shorter id wins), refuses Pocket sets, fetches every print from TCGdex and the matching malie set unless malie is switched off, merges them, checks the shape and writes `<dir>/sets/<id>.json`. Added `tcgreplay sets import <set> [-dir] [-no-malie]`, which `main` dispatches before the playmat's flags, and excluded `cardsets` from the container build in `.dockerignore`.

**Deviations**: Flags may come after the set name (`sets import me06 -dir x`), because the flags are parsed around the positional words. A single-set import fetches every set's details only when it resolves a code or when the malie match by code may be contested: another listed set id has this one's as a prefix, or the other way round (`30th`/`30th-c`). Every other import fetches just the set. A card the set lists but TCGdex cannot serve fails the import, and nothing is written. The usage text already lists `sets check`, which is added in a later task.

**Files changed**:
- `internal/cardset/import.go`
- `internal/cardset/import_test.go`
- `cmd/tcgreplay/sets.go`
- `cmd/tcgreplay/sets_test.go`
- `cmd/tcgreplay/main.go`
- `.dockerignore`

**Discoveries**: A real `sets import DLR` (Delta Reign by code) takes about 8s, most of it fetching every set's details to resolve the code. It writes 141 prints, all with a regulation mark and an image, and no HTML. A second run gives identical bytes. `-no-malie` gives 141 TCGdex-only prints.

### 2026-11-08 — Task: Corrections and acknowledgements

**What was done**: Added `corrections.go`, with these parts:
- `ReadCorrections` reads the per-set corrections file. A missing file means no corrections. Unknown fields are refused, and every entry needs a card, a path or kind, a value and a reason.
- `applyCorrections` and `applyCorrection` set one field of a print by an RFC 6901 JSON Pointer through its JSON form: missing object members are created and `-` appends to a list. The result is decoded strictly, so a path the format does not define fails with the card and path named.
- Each application records the merged value it replaced, and whether the sources already gave the correction's value (redundant).

The import now applies the set's corrections to every fresh build, before checking the shape and writing.

**Deviations**: The first goal check failed criterion 1: corrections were not yet applied by the import, which the plan left to the Import report task. Reading and applying corrections was moved into `importSet` in this task. A correction may not change a print's id. The redundancy flag is returned but not reported yet; the Import report task surfaces it.

**Files changed**:
- `internal/cardset/corrections.go`
- `internal/cardset/corrections_test.go`
- `internal/cardset/import.go`

**Discoveries**: `TestRelaySurvivesDaemonRestart` in `cmd/tcgreplay` failed once under `-race` with a data race in the daemon relay, which this spec does not touch. It passed three times on rerun, in the worktree and in the main checkout. It is a pre-existing flaky race, worth a separate look.

### 2026-11-08 — Task: Re-import comparison

**What was done**: Added `compare.go`. A fresh build is carried over the set already in the project: new prints are added, and existing prints take the fresh core, images and English text (corrections are already applied, so corrected fields stay put). Wording in other languages and the set's non-English name are carried over, and prints upstream no longer lists are kept unchanged. It lists the added prints, the prints whose English text changed, and the prints gone upstream. The import now reads the previous built set, compares and writes the result, and fills `Added`, `Changed` and `Gone` in its summary.

**Deviations**: `compare` returns a `comparison` struct instead of four values. Its wiring into `importSet` was done in this task instead of the Import report task, so the re-import criteria hold end to end. A first import reports every print as added. Only an English text change counts as `changed`; a new image is taken silently.

**Files changed**:
- `internal/cardset/compare.go`
- `internal/cardset/compare_test.go`
- `internal/cardset/import.go`

**Discoveries**: None.

### 2026-11-08 — Task: Import report

**What was done**: Added `report.go`. Every import now writes `<dir>/reports/<id>.json`, a deterministic report with no timestamps. Each item names a card, a kind, a field as a JSON Pointer, the sources' values and a status: `corrected` when a correction's path covers the field, `acknowledged` with the reason from the corrections file, otherwise `open`. The kinds are the source disagreements, the structural gaps (missing effect, no attacks, Stage 1 or Stage 2 without evolves-from, no image), text changed since the last import, redundant corrections and prints gone upstream. Items are sorted by collector number, kind and field. The summary line now carries the open count.

**Deviations**: `missing-effect` also covers an ability without English effect text, which is the same rule `Validate` uses, so every invalid gap shows in the report. A `print-gone-upstream` item has no field. `WriteReport` is exported for the check task.

**Files changed**:
- `internal/cardset/report.go`
- `internal/cardset/report_test.go`
- `internal/cardset/import.go`

**Discoveries**: A real Delta Reign import now reports 36 open items, all TCGdex-versus-malie disagreements (ex suffix spelling, Mega ex stage, Azurill's move kind). Every Trainer and special Energy has effect text, every evolved Pokémon has evolves-from, and every print has an image.

### 2026-11-08 — Task: Set check command and committed-data guard

**What was done**: Added `check.go`. `Check(dir, sets...)` runs the offline checks over built sets and returns every open item:
- a set that does not read or is badly formed (`invalid`: no English name or text, text not lined up with the facts);
- HTML in any text (`markup`);
- a corrections file that does not read (`corrections-invalid`), or a correction whose card or path no longer resolves (`correction-dangling`);
- an unacknowledged structural gap;
- any item of the last report that is still open under the current corrections file.

Added `tcgreplay sets check [<set>...] [-dir]`, which prints `<set> <card> <kind> <field>` per item and exits 1 on any. `TestCommittedSets` runs the check over `cardsets/`; it skips while no set is built. `TestCommittedDeltaReign` holds `me06.json` to its acceptance criteria once it exists.

**Deviations**: `Check` returns `[]CheckItem` (a `ReportItem` plus the set id) instead of bare `ReportItem`s. The check also re-judges the last report's source-dependent items (disagreements, changed text, redundant corrections, gone prints) against the current corrections file, so an acknowledgement or a deleted redundant correction clears them without a network re-import. `TestCommittedGuardCatchesBadEdits` proves the guard fails the suite on a bad edit even before any set is committed.

**Files changed**:
- `internal/cardset/check.go`
- `internal/cardset/check_test.go`
- `internal/cardset/committed_test.go`
- `internal/cardset/report_test.go`
- `cmd/tcgreplay/sets.go`
- `cmd/tcgreplay/sets_test.go`

**Discoveries**: The goal reviewer's reply must be bare JSON. A reply with prose or a code fence around the JSON was refused (`goal_reply_invalid`), and the reviewer had to be asked again.

### 2026-11-08 — Task: Card-set import skill

**What was done**: Added the project skill `skills/card-set-import/SKILL.md`, which assistants reach through the `.claude/skills` and `.agents/skills` links. It triggers on importing, building, updating or verifying a card set. It tells the assistant to:
- build the binary and run `sets import`;
- work only on the report's open items, checking each against the printed card image (TCGdex `<url>/high.png`, the malie jpg) and both sources;
- record a correction or an acknowledgement with a reason in `cardsets/corrections/<id>.json`, following a table of what resolves each report kind;
- re-run until `0 open`, then run `sets check` and `go test ./internal/cardset/`;
- end with a summary of the built-set changes.

The skill forbids committing, staging and pushing, and keeps no list beyond the report.

**Deviations**: The skill's guidance for disagreements: acknowledge with the field when the printed card agrees with TCGdex's kept value, and correct only when it does not, because a correction equal to the built value is reported as redundant. It also tells the assistant to stop and raise a systematic cosmetic difference with the maintainer instead of acknowledging card by card (plan Open Questions).

**Files changed**:
- `skills/card-set-import/SKILL.md`

**Discoveries**: None.

### 2026-11-08 — Task: Card-set documentation

**What was done**: Added `cardsets/README.md`, which covers:
- what the directory holds, and that it is never embedded or put in the image;
- the `sets import` (`-all`, `-no-malie`) and `sets check` commands, and what an import does step by step;
- asking the assistant to verify a set through the `card-set-import` skill;
- the built format, with an example print;
- recording a correction (the Legendary Pyroduct pair rule) and an acknowledgement (a `no-image` and a `/stage` disagreement);
- a table of every report kind and what resolves it, plus the check-only kinds.

The README's Development section gained a short "Card sets" paragraph with the two commands and a link.

**Deviations**: The docs already describe `sets import -all`, which the "Import every set in one run" task adds.

**Files changed**:
- `cardsets/README.md`
- `README.md`

**Discoveries**: None.

### 2026-11-08 — Task: Ask malie.io about build-time use

**What was done**: Nothing. This is a person's task (asking the malie.io maintainers on their Discord), which no agent can carry out.

**Deviations**: The goal check ran out of attempts (criterion 1 not met: no ask or answer recorded in `learnings/card-image-sources.md`). The user chose **skip**, so the task is left open with its criterion unticked, for the maintainer to do. Nothing depends on it: the import uses malie by default and runs without it via `-no-malie`.

**Files changed**:
- None

**Discoveries**: A human-execution task sitting first among the unchecked tasks blocks a whole-plan orchestrated run. The goal check targets it and can only exhaust its attempts before the user can skip it.

### 2026-11-08 — Task: Build and verify Delta Reign

**What was done**: Built Delta Reign (`me06`, DLR) into `cardsets/` by following the `card-set-import` skill. The first import had 36 open items, all disagreements between TCGdex and malie. Checking them against the printed cards (malie's TCG Live images; TCGdex has no art yet) gave 44 corrections and 2 acknowledgements:
- `ex` suffix on 22 prints (TCGdex wrote `EX`);
- stage on 12 Mega/evolved ex prints (TCGdex wrote `Basic`);
- Azurill (080, 115): the printed card shows Bouncy-Bounce Charge as a cost-free attack, so TCGdex's Ability is removed and the `/attacks` disagreement is acknowledged;
- the pair rule on all six Legendary Stadium prints (094-099).

The final report has 0 open items (34 corrected, 2 acknowledged), `sets check` is clean, and a re-import leaves the corrections file byte-identical. Every Trainer and special Energy has effect text, every Stage 1 and Stage 2 has evolves-from, and every print has regulation mark J and a malie image. `TestCommittedDeltaReign` now also asserts the pair rule on the six Legendary Stadiums. Nothing was committed.

**Deviations**: The Delta Reign files were built while the skipped human task was being resolved; they were checked by this task's own goal check.

**Files changed**:
- `cardsets/sets/me06.json`
- `cardsets/corrections/me06.json`
- `cardsets/reports/me06.json`
- `internal/cardset/committed_test.go`

**Discoveries**: TCGdex's Delta Reign data writes every ex as `EX` and gives evolved Mega ex the stage `Basic`. A correction's own change shows as `text-changed` on the next import and clears on the one after.

### 2026-11-08 — Task: Import every set in one run

**What was done**: Added `cardset.ImportAll` and `tcgreplay sets import -all`. The run imports every English TCGdex set except Pokémon TCG Pocket sets, one after another, and carries on past a set that fails. It prints one summary line per set, then a totals line, the failed sets and the malie sets no TCGdex set matches. The exit code is 1 if any set failed. A real run against the live sources took about 1m47s and printed "206 sets built, 1941 open items, 0 failed, 15 Pokémon TCG Pocket sets skipped". The unmatched malie sets were me5-5c (0 entries), me5-5m (3), me7 (0), mealt (11) and svalt (237).

**Deviations**: The first real run failed 15 old sets (dp, ecard, ex, neo, dpp): TCGdex gives some attacks and abilities no English name. `ValidateShape` refused those sets outright, so no report could list the gap. A missing English attack or ability name is now a reported gap instead: a new report kind `missing-name`, still refused by `Validate`, covered in `cardsets/README.md` and the skill. Two unnamed-move cases in the built-format tests moved from `ValidateShape` to `Validate` accordingly. A set with no prints now writes `"prints": []` instead of `null`.

**Files changed**:
- `internal/cardset/import.go`
- `internal/cardset/import_test.go`
- `internal/cardset/format.go`
- `internal/cardset/format_test.go`
- `internal/cardset/report.go`
- `internal/cardset/report_test.go`
- `internal/cardset/check.go`
- `cmd/tcgreplay/sets.go`
- `cmd/tcgreplay/sets_test.go`
- `cardsets/README.md`
- `skills/card-set-import/SKILL.md`

**Discoveries**: Open items across the whole catalogue:

| Kind | Open items |
| --- | --- |
| `no-image` | 1450 |
| `disagreement` | 194 |
| `no-attacks` | 140 |
| `missing-evolves-from` | 80 |
| `missing-effect` | 43 |
| `missing-name` | 34 |

Most disagreements are TCGdex writing weakness `x2` where malie writes `×2` (124). Four TCGdex sets list no cards at all: wp, sp, rc and jumbo. malie's me5-5m RGB Mews are TCGdex's `30th-R`, `30th-G` and `30th-B`, so they are not missing prints. svalt and mealt are alternate finishes of prints already in other sets.

### 2026-11-08 — Task: Build and verify the Scarlet & Violet and Mega Evolution sets

**What was done**: Built and verified all 30 TCGdex sets in the `sv` and `me` series (main sets, `.5` sets, promos, energies, `30th`, `30th-c`, `mfb`) into `cardsets/`. Every report ends with 0 open items and `sets check` is clean. The 194 disagreements of the first run included two systematic cosmetic differences, now added to the comparison rules with a test: weakness `x2` against `×2` (124 items) and energy symbols in square brackets against braces. What remained was resolved against the printed cards, using TCGdex or TCG Live images, the original prints on TCGdex where no image of the reprint exists, and the card's own rule text:

| Fix | Cards |
| --- | --- |
| Stage corrections | 10 `30th` ex prints |
| `energyType` set to `Special` | 14 Special Energy prints |
| Text and names from the printed promo cards | `mep` 005, 006, 007, 008 and 082 |
| Regulation mark corrected | `mep-051` |
| Evolves-from corrected | Raichu `sv02-064` and `sv02-211` |
| Evolves-from filled from the original prints | 5 `30th-c` and 3 `mfb` prints |
| Missing Poké-Power/Poké-Body added | `30th-c` Delcatty and Gengar |
| LEGEND half filled from its partner | `30th-c-019` |

Seven `svp` prints got TCGdex images from TCGdex's asset server. 71 prints with no art in any source were acknowledged. Greninja BREAK's no-attacks item and Diancie's attack-text disagreement (TCGdex is right) were acknowledged.

**Deviations**: Beyond the report's items, two `30th-c` reprints missing their Poké-Power or Poké-Body in TCGdex were corrected from the originals (found while cross-checking evolves-from). The other catalogue sets are still in a scratch build only: they are built and committed by the next two tasks.

**Files changed**:
- `cardsets/sets/*.json`, `cardsets/corrections/*.json` and `cardsets/reports/*.json` for the 30 sets
- `internal/cardset/merge.go`
- `internal/cardset/text.go`
- `internal/cardset/merge_test.go`

**Discoveries**: TCGdex's asset server often has art for promo prints whose card data lists no image (`svp` 191, 192, 213, 214, 215, 225, 500). Probing `https://assets.tcgdex.net/en/<serie>/<set>/<localId>/high.png` finds them. TCGdex marks several Special Energy cards as `Normal`.

### 2026-11-08 — Task: Build and verify the Sword & Shield sets

**What was done**: Built and verified all 26 TCGdex `swsh` series sets (3675 prints) from TCGdex alone. Every report ends with 0 open items and `sets check` is clean. Of the 374 open items:

| Item | Resolution |
| --- | --- |
| 312 gallery, Shiny Vault and Trainer Gallery prints without an image | TCGdex's asset server has the art under the parent set (`swsh4.5/SV001`, `swsh9/TG01`, `swsh12.5/GG01`), so each got a corrected `/images`; a sample of 18 was checked by name |
| 48 prints with no art anywhere | acknowledged (`cel25cc`, `swshp`, `cel25-25`) |
| Pokémon GO Ditto's no-attacks, on 3 prints | acknowledged: the printed card has only an Ability |
| The four Morpeko V-UNION pieces | each got the attack printed on it, plus Lightning type and retreat 2 (TCGdex images) |
| 7 Celebrations Classic Collection prints without evolves-from | filled from the originals' evolution lines |

The Classic Collection Venusaur also got the Pokémon Power Energy Trans, which TCGdex leaves out.

**Deviations**: Classic Collection Venusaur's missing Pokémon Power was added beyond the report's items, as for the `30th-c` reprints.

**Files changed**:
- `cardsets/sets/*.json`, `cardsets/corrections/*.json` and `cardsets/reports/*.json` for the 26 sets

**Discoveries**: For TCGdex sub-sets (`swsh4.5sv`, `swsh*tg`, `swsh12.5gg`), the card data lists no image, but the art lives at the parent set's asset path with the same local id.

### 2026-11-08 — Task: Build and verify the older sets

**What was done**: Built the 150 remaining English sets, from the original sets through Sun & Moon, including promos, Trainer Kits and McDonald's sets, into `cardsets/` from TCGdex. Every report ends with 0 open items and `sets check` over the whole catalogue (206 sets, 21,439 prints) passes. Of the 1285 open items:
- 1010 prints with no art in any source were acknowledged, and 2 `sm3.5` images were found on TCGdex's asset server;
- 77 gaps (attacks, names, effects, evolves-from) were filled from an identical reprint elsewhere in the catalogue (same name, stage, HP, attack costs and damage), each citing the reprint;
- the other 196 gaps (117 no-attacks, 30 missing-evolves-from, 27 missing-effect, 22 missing-name) were acknowledged as **unverified and left for the maintainer**. Search for "Unverified" in `cardsets/corrections/`.

`wp`, `sp`, `rc` and `jumbo` list no English cards on TCGdex and are kept as empty built sets, documented in `cardsets/README.md`.

**Deviations**: The goal check accepted a shortfall on criterion 3, which is left unticked: "every card in every built set has an English name and English text for every attack, ability and effect it has". At the scope question the user chose option (B): fill only the reprint matches and acknowledge the rest without reading the printed card images, rather than run parallel image-reading sub-agents. The 196 acknowledged gaps, among them 22 empty English attack or ability names and 27 missing effects in old EX, DP, Neo and e-Card sets, therefore stay incomplete for the maintainer. The empty-set question (a plan Open Question) was answered as option (a): keep them as empty built sets.

**Files changed**:
- `cardsets/sets/*.json`, `cardsets/corrections/*.json` and `cardsets/reports/*.json` for the 150 sets
- `cardsets/README.md`

**Discoveries**: A single-set import of an old set fetches every set's details, because the shared-code heuristic (one set id is a prefix of another) is true for many old ids (`base1`/`base10`, `ex1`/`ex10`). Importing the 150 older sets one by one takes about 4 minutes; `sets import -all` is faster. TCGdex answered `/sets` with 503 three times in a row once; re-running the import succeeded.

### 2026-11-08 — Task: Ask malie.io about build-time use (closed)

**What was done**: The maintainer asked the malie.io maintainers on their Discord on 2026-11-08: may tcgreplay read the TCG Live export at build time (a few requests per release, never live per user); may it republish card text built from it on its own CDN; may it use malie's card images as a fallback (linked or copied); how should malie.io be credited; and is there a heads-up channel for breaking changes to the alpha format. No reply has come yet. The knowledge entry `learnings/card-image-sources.md` (repo tier, store `tcgreplay`) now records the date, the questions, that there is no reply, what to do if the answer is no (`-no-malie`, drop malie from image lists), and that the entry is to be updated when a reply arrives. The task and its criterion are ticked.

**Deviations**: The earlier goal check of this task is recorded as `skipped` and the CLI cannot re-open it (`check_goal` reports no open item, `goal decide recheck` answers `goal_decision_not_due`, and a single-task run answers `task_requires_human` with "have a person carry the task out and tick it in plan.md"). As the user decided to record the ask and close the task, the criterion was checked by an independent `spek-goal-reviewer` run outside the recorder, which judged it **met** (the entry states the ask on 2026-11-08 with its five questions and "No reply yet"), and the task was ticked by hand as that refusal directs. The recorded goal state stays `skipped`. malie stays on by default at build time until a reply says otherwise.

**Files changed**:
- None in the repo (knowledge entry `learnings/card-image-sources.md` updated through `spek-knowledge`)

**Discoveries**: A human task whose goal check was skipped cannot be goal-checked again through the CLI once the person has done it; it is closed by ticking it in plan.md.
