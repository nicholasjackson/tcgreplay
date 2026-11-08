---
name: card-set-import
description: Import and verify a card set into the project's card data (cardsets/). Use when the maintainer asks to import a set, build or rebuild card set data, update a set's cards (for example after new promos or art), re-import a set, verify imported cards, or work through a card-set import report. Takes a TCGdex set id ("me06") or a decklist code ("DLR"), or every set.
---

# Card-set import

The maintainer builds card sets ahead of time into `cardsets/`. The code does
the mechanical work: `tcgreplay sets import` gathers a set from TCGdex and the
malie.io TCG Live export, merges them, applies the hand corrections and writes
a report of everything still in doubt. Your job is the judgement the code
cannot make: check each doubtful card against its printed card, record a
correction or an acknowledgement with a reason, and repeat until nothing is
left open.

The report is your only to-do list. Keep no list of your own.

## Rules

- **Never commit.** Do not run `git commit`, `git add`, `git push` or anything
  that stages or records changes. Leave everything in the working tree for the
  maintainer to review.
- **Never edit `cardsets/sets/` or `cardsets/reports/` by hand.** The import
  writes them. Every decision you make goes in `cardsets/corrections/<id>.json`.
- **Every correction and acknowledgement carries a reason** that says what you
  checked, for example "Printed card (TCGdex and malie images) reads ..." or
  "Both images show no attack; this is a Fossil played as a Pokémon".
- **The printed card is the authority**, not either source. When the image is
  unreadable or missing, say so in the reason and prefer the source that
  matches the card's other prints.
- **English text is required** for every name, attack, ability and effect.
  Energy symbols written as `{R}`, `{W}` and so on are acceptable text; HTML is
  not (the import strips it).
- **malie.io** is a fan project used at build time only. If the maintainer says
  to build without it, pass `-no-malie` to every import.

## Steps

### 1. Build and import

From the repository root:

```sh
go build -o bin/tcgreplay ./cmd/tcgreplay
bin/tcgreplay sets import <set>          # a set id (me06) or decklist code (DLR)
```

The command prints one line such as
`me06: 141 prints, 0 added, 2 changed, 0 gone, 12 open`. The set's id is the
first word; use it for the file names below. For every set, run
`bin/tcgreplay sets import -all` once, then work set by set from step 2.

### 2. Read the report

Open `cardsets/reports/<id>.json`. Work only on items whose `status` is
`open`. Each item names the `card` (print id), its `name`, the `kind`, the
`field` (a JSON Pointer into the print, such as `/text/en/effect` or `/hp`)
and, for disagreements, each source's `values`.

### 3. Look at the printed card

For each open item, open the print in `cardsets/sets/<id>.json` and look at its
image sources (`images`):

- TCGdex: append `/high.png` to the URL (for example
  `https://assets.tcgdex.net/en/me/me06/001/high.png`).
- malie: the `.jpg` URL as recorded.

Download the image (for example with `curl -sL -o <scratch>/card.png <url>`)
and read it. Compare the printed card with the field and with both sources'
values. When a print has no image, use another print of the same card in the
same or another set, and say so in the reason.

### 4. Record the decision

Edit `cardsets/corrections/<id>.json` (create it if it does not exist):

```json
{
  "corrections": [
    {"card": "me06-096", "path": "/text/en/effect", "value": "…", "reason": "…"}
  ],
  "acknowledged": [
    {"card": "me06-050", "kind": "no-image", "reason": "…"}
  ]
}
```

What resolves each kind:

| Kind | Resolve with |
| --- | --- |
| `disagreement` | The built set keeps TCGdex's value. If the printed card agrees with it, an acknowledgement with the item's `field`; if not, a correction at the `field` with the printed value. (A correction equal to the built value is reported as redundant.) |
| `missing-name` | A correction at the `field` with the printed attack or ability name. |
| `missing-effect` | A correction at the `field` with the printed effect text. |
| `no-attacks` | An acknowledgement when the card really has no attacks (for example a Fossil or a Pokémon with only an ability), else corrections adding the attacks (`/attacks` and `/text/en/attacks`). |
| `missing-evolves-from` | A correction at `/text/en/evolveFrom`. |
| `no-image` | An acknowledgement when no source has art (TCGdex adds images weeks after release; a re-import picks them up), or a correction to `/images`. |
| `text-changed` | Check the new English text against the printed card. If it is right, nothing to record: the item clears on the next import. If not, correct the field. |
| `correction-redundant` | Delete that correction from the file: the sources now give the same value. |
| `print-gone-upstream` | An acknowledgement saying why the print stays (or ask the maintainer whether it should). |

A correction's `path` may point anywhere in the print's JSON, and its `value`
is any JSON value (text, number, list or object). A path the format does not
define makes the import fail with a message naming it. An acknowledgement may
name a `field` to cover one disagreement on a card rather than all of them.

When one cosmetic difference between the sources (a spelling or punctuation
convention) produces many disagreements across a set, stop and tell the
maintainer instead of acknowledging each card: it belongs in the import's
comparison rules.

### 5. Re-run until nothing is open

```sh
bin/tcgreplay sets import <set>
```

Repeat steps 2 to 5 until the summary line says `0 open`.

### 6. Check

```sh
bin/tcgreplay sets check <id>
go test ./internal/cardset/
```

Both must pass. `sets check` lists any open item and exits 1.

### 7. Summarise for the maintainer

End with a short summary for review:

- `git diff --stat cardsets/` and the set's final summary line;
- the notable changes in the built set (new prints, changed text or art);
- each correction and acknowledgement you recorded, one line each with its
  reason;
- anything you could not settle and left for the maintainer.

Do not commit. The maintainer reviews and commits the changes.
