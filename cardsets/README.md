# Card sets

This directory holds the playmat's card data, built ahead of time by the
maintainer: one file per English card set, the hand corrections that every
import keeps, and the report of the last import. It is data, not code: it is
never embedded in the program and never copied into the container image
(`.dockerignore` leaves it out). It reaches players by being published to the
playmat's Spaces bucket (see [Publishing](#publishing)); every playmat reads its
card definitions only from the published sets, never from TCGdex or malie.io.

```
cardsets/
  sets/<set-id>.json         built sets (written by the import)
  corrections/<set-id>.json  hand corrections and acknowledgements (written by people and the assistant)
  reports/<set-id>.json      the last import's report (written by the import)
```

Files are named by TCGdex set id (`me06`). Commands take the id or the
decklist code (`DLR`).

Four sets are built empty on purpose: TCGdex lists `wp` (W Promotional),
`sp` (Sample), `rc` (Radiant Collection) and `jumbo` (Jumbo cards) but gives
no English cards for them (checked 2026-11-08), and corrections cannot add
prints. A re-import fills them if TCGdex adds their cards.

Some gaps in the oldest sets are acknowledged as unverified: where TCGdex
lacks an attack, name, effect or evolves-from and no identical reprint gives
it, the acknowledgement says so and leaves the printed card for the
maintainer to check (search `cardsets/corrections/` for "Unverified").

## Importing a set

```sh
go build -o bin/tcgreplay ./cmd/tcgreplay
bin/tcgreplay sets import DLR            # one set, by decklist code or id
bin/tcgreplay sets import -all           # every English set (Pokémon TCG Pocket aside)
bin/tcgreplay sets import me06 -no-malie # TCGdex and corrections only
bin/tcgreplay sets check                 # check every built set; exits 1 on any open item
bin/tcgreplay sets check me06            # check one
```

An import:

1. fetches the set and every print from [TCGdex](https://tcgdex.dev) (the main
   source) and, unless `-no-malie` is given, the set from the
   [malie.io](https://malie.io) TCG Live export, which fills gaps such as a new
   set's missing effects, evolves-from, regulation marks and images;
2. merges them into one definition per print over both sources' prints. TCGdex
   wins every field it has, malie fills the rest, and fields where they differ
   (beyond spacing, quote style or `{R}` versus `Fire`) are reported;
3. strips HTML from all text (energy spans become their word); energy symbols
   such as `{R}` are text and stay;
4. applies `corrections/<set-id>.json`;
5. carries the result over the previous build: new prints are added, changed
   text and art replace the old, prints upstream no longer lists are kept,
   and wording in other languages is kept;
6. writes `sets/<set-id>.json` and `reports/<set-id>.json` and prints a summary
   such as `me06: 141 prints, 0 added, 2 changed, 0 gone, 3 open`.

Output is deterministic: re-importing unchanged sources rewrites identical
bytes, and a change to one card diffs as that card's lines.

malie.io is a fan project and is used at build time only. The import runs
without it (`-no-malie`) from TCGdex plus corrections.

## Having the assistant verify a set

Ask the coding assistant to import the set ("import Delta Reign", "update the
DLR card data"). Its `card-set-import` skill (`skills/card-set-import/`) runs
the import, checks every open report item against the printed card image and
both sources, records a correction or an acknowledgement with a reason for
each, re-runs until nothing is open, runs `sets check`, and ends with a summary
of the changes. It never commits: review the diff and commit it yourself.

## The built format

A built set is a header and a list of prints sorted by collector number. Each
print holds its language-independent facts once (category, HP, types, stage,
attack costs and damage, ability kinds, weaknesses, resistances, retreat,
Trainer and Energy kinds, regulation mark, image sources) and its wording in a
`text` object keyed by language. Attacks and abilities line up by position
between the facts and every language's text.

```json
{
  "id": "me06-003",
  "localId": "003",
  "category": "Pokemon",
  "hp": 110,
  "types": ["Grass"],
  "stage": "Stage1",
  "attacks": [{"cost": ["Colorless"], "damage": "30"}],
  "retreat": 1,
  "regulationMark": "J",
  "images": [{"source": "malie", "url": "https://cdn.malie.io/…/me6_en_003_std.jpg"}],
  "sources": ["malie", "tcgdex"],
  "text": {
    "en": {
      "name": "Masquerain",
      "evolveFrom": "Surskit",
      "attacks": [{"name": "Frightening Pattern", "effect": "…"}]
    }
  }
}
```

English text is required for every print: game logic reads it. Other
languages may be added by hand under `text`; the import writes English only
and keeps the rest. `images` lists every source that has art, TCGdex first. The board tries them
in that order and falls back to the next when one is missing or fails to load,
so reordering a print's images (with a correction) changes which art shows
first. A TCGdex image is a base path (the board adds `/low.webp` or
`/high.webp`, and the hosted playmat serves its copy from the CDN); any other
source must be an https address of a complete image file (`.jpg`, `.jpeg`,
`.png` or `.webp`), which the board loads as is. Anything else is left out.

## Recording a correction

A correction sets one field of one print. `path` is a
[JSON Pointer](https://www.rfc-editor.org/rfc/rfc6901) into the print as it
appears in the built set; `value` is any JSON value; `reason` says what was
checked. For example, the Legendary Stadiums' pair rule, which neither source
carries:

```json
{
  "corrections": [
    {
      "card": "me06-095",
      "path": "/text/en/effect",
      "value": "You cannot play this card by itself. You must combine 2 different Legendary Pyroduct from your hand to play as 1 Stadium card.\n\nEvolution Pokémon in play (both yours and your opponent's) have no Abilities.",
      "reason": "Pair rule printed on the English card; neither TCGdex nor malie has it"
    }
  ],
  "acknowledged": []
}
```

Every later import applies it and never rewrites the file. A path the format
does not define, or a card the set does not hold, makes the import and
`sets check` fail with a message naming it. When the sources later give the
same value, the report lists the correction as redundant: delete it.

## Recording an acknowledgement

An acknowledgement says why a report item needs no change. `field` is optional
and limits it to one disagreement on a card:

```json
{
  "corrections": [],
  "acknowledged": [
    {"card": "me06-050", "kind": "no-image", "reason": "No source has art yet; TCGdex adds images weeks after release"},
    {"card": "me06-011", "kind": "disagreement", "field": "/stage", "reason": "Printed card is a Stage 1; TCGdex's Basic is wrong"}
  ]
}
```

## Report kinds

Each report item names a card, a kind, the field (a JSON Pointer), the
sources' values for a disagreement, and a status: `open`, `corrected` (a
correction covers the field) or `acknowledged` (with its reason).

| Kind | Meaning | Resolved by |
| --- | --- | --- |
| `disagreement` | TCGdex and malie give a field different values; the built set keeps TCGdex's | an acknowledgement (TCGdex is right) or a correction (it is not) |
| `missing-name` | an attack or ability without an English name | a correction with the printed name |
| `missing-effect` | a Trainer, special Energy or ability without English effect text | a correction with the printed text |
| `no-attacks` | a Pokémon without attacks | an acknowledgement (it really has none) or corrections adding them |
| `missing-evolves-from` | a Stage 1 or Stage 2 without evolves-from | a correction at `/text/en/evolveFrom` |
| `no-image` | no source has an image for the print | an acknowledgement, or a correction to `/images`; a re-import picks up new art |
| `text-changed` | the English text changed since the last import | checking it against the card; it clears on the next import, or correct it |
| `correction-redundant` | the sources now give a correction's value | deleting the correction |
| `print-gone-upstream` | a print upstream no longer lists; it is kept | an acknowledgement |

`sets check` also reports `invalid` (a built set that does not read, or lacks
English names or text), `markup` (HTML in text), `corrections-invalid` and
`correction-dangling` (a correction whose card or path no longer resolves).
These are fixed in the files, not acknowledged. The Go test suite runs the same
check over everything here, so a bad edit fails `go test ./...`.

## Publishing

Players get card data from the published sets, so a change here reaches them
without a new release of the playmat. Publishing is automatic: every merge to
`main` that changes `cardsets/` runs the "Publish card sets" GitHub Actions
workflow (`.github/workflows/publish-cardsets.yml`), which runs `sets check` and
then `sets publish`, and fails without publishing if the check finds an open
item. The workflow can also be started by hand from the Actions tab. A merge
that changes only `cardsets/` never redeploys the hosted playmat (see
`.github/workflows/deploy.yml`), so games in progress carry on.

To publish by hand, export the bucket settings and run the command:

```sh
export TCGREPLAY_SPACES_ENDPOINT=https://ams3.digitaloceanspaces.com  # the region endpoint, not the bucket's own address
export TCGREPLAY_SPACES_BUCKET=… TCGREPLAY_SPACES_KEY=… TCGREPLAY_SPACES_SECRET=…
export TCGREPLAY_SPACES_CDN_TOKEN=…   # optional: purges the manifest from the CDN (scopes cdn:read and cdn:delete)
bin/tcgreplay sets publish -dry-run   # what would be uploaded; writes nothing
bin/tcgreplay sets publish            # uploads changed sets, then the manifest
bin/tcgreplay sets publish -to /tmp/published   # publish to a local directory instead (development)
```

`sets publish` refuses to publish while `sets check` has an open item, or when a
set already published is missing from `cardsets/sets/` (a set is never
unpublished by accident). It prints each set it uploaded and how many were
unchanged; a publish with nothing changed uploads nothing.

### The published layout

```
cardsets/manifest.json                    which object is each set's current version (cached for a minute, purged after each publish)
cardsets/sets/<set-id>/<hash>.json.gz     a built set, gzipped, named by the first 16 hex of the sha256 of its file (cached for a year, never changed)
```

Set objects are content-addressed and never overwritten or deleted, so every
earlier version of a set stays in the bucket byte for byte. To roll back,
publish an older commit's `cardsets/`: its objects are still there, so only the
manifest changes.

### How quickly playmats pick up a publish

- Hosted and desktop playmats check the manifest every 5 minutes
  (`-cardsets-interval` or `TCGREPLAY_CARDSETS_INTERVAL`, from 10s to 10m), so a
  publish is live within about 5 minutes with no restart.
- An admin can make the hosted playmat check at once with "Check for published
  card sets now" on `/admin`, which also shows the loaded catalogue's version,
  set count and last check.
- A game already open keeps the cards it started with until it ends; new,
  imported and reopened games use the latest.
- Each playmat keeps a copy of what it loaded under `<cache>/cardsets/` (the
  `-cache` directory, `~/.cache/tcgreplay` by default) and plays from it when it
  cannot reach the bucket, or always with `-offline`. A desktop playmat's first
  run needs network access to download the card data once.
- `-cardsets-url` (or `TCGREPLAY_CARDSETS_URL`) points a playmat at another
  published copy: an https address or a local directory such as the output of
  `sets publish -to`.

### Secrets the workflows need

In the GitHub repository's Settings → Secrets and variables → Actions:
`TCGREPLAY_SPACES_ENDPOINT` (the region endpoint,
`https://<region>.digitaloceanspaces.com`), `TCGREPLAY_SPACES_BUCKET`,
`TCGREPLAY_SPACES_KEY`, `TCGREPLAY_SPACES_SECRET` and
`TCGREPLAY_SPACES_CDN_TOKEN` for publishing; `DIGITALOCEAN_ACCESS_TOKEN` (able to
create App Platform deployments) and `TCGREPLAY_APP_ID` for the deploy workflow.

### Cleaning up the old card cache

Before card sets were published, the hosted playmat cached TCGdex card data in
the bucket under `tcgdex/`. Once the playmat reading published sets is
deployed, remove it once:

```sh
bin/tcgreplay sets cleanup-legacy           # lists the old objects and their count; deletes nothing
bin/tcgreplay sets cleanup-legacy -delete   # deletes them
```

It only ever touches `tcgdex/`, never image copies (`images/`) or published sets
(`cardsets/`).
