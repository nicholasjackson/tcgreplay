---
created_date: "2026-11-08"
document_status: final
closed_date: "2026-11-08"
---

# Test plan: 20261108110847-d27a9f77-card-set-import

Manual checks for the success metric and the reviews the plan's Testing Approach leaves to a person. The behavioural success metric (zero Trainers or special Energy without effect text, zero markup) is covered by `TestCommittedSets`, `TestCommittedDeltaReign` and `sets check`, and is not repeated here.

Setup for every procedure: work in the repository root, then build the binary.

```sh
go build -o bin/tcgreplay ./cmd/tcgreplay
```

## 1. Success metric: a new set can be imported and verified in one maintainer session

- **What to measure:** One maintainer session takes a set from nothing to a verified, checked state. The session asks the coding assistant to import the set, and it ends with a summary and nothing committed. Threshold: one session, with no hand edits to `cardsets/sets/` or `cardsets/reports/`.
- **How:**
  1. Pick a set and delete its files: `rm cardsets/{sets,reports,corrections}/sv06.5.json`. Shrouded Fable is small (99 prints).
  2. Ask the assistant: "import Shrouded Fable" (or "import SFA"). Confirm it loads the `card-set-import` skill.
  3. Let it run to the end without intervening.
  4. Afterwards run `bin/tcgreplay sets check sv06.5` and `go test ./internal/cardset/`.
- **Expected result:**
  - The session ends with the import summary line showing `0 open`, a `git diff --stat cardsets/` style summary, and a list of every correction and acknowledgement with its reason.
  - `sets check` prints `all card sets check out` and exits 0.
  - The tests pass.
  - `git log` shows no new commit and `git status` shows the changes uncommitted.
- **Who / when:** The maintainer, before relying on the skill for a new set release, and again when the next set comes out.

## 2. Review: an agent-verified import ends clean, summarised and uncommitted

- **What to look at:** The end of the session in procedure 1, plus `cardsets/reports/<id>.json` and `cardsets/corrections/<id>.json`.
- **What to look for:**
  - Every report item has `status` `corrected` or `acknowledged`, and every acknowledgement has a `reason` naming what was checked. Check with `grep -c '"status": "open"' cardsets/reports/sv06.5.json`; the expected count is 0.
  - The assistant's closing message summarises the built-set changes.
  - The assistant ran no `git add`, `git commit` or `git push`.
- **Pass:** All three hold.

## 3. Review: spot-check Delta Reign corrections and acknowledgements against the printed cards

- **What to look at:** `cardsets/corrections/me06.json` (44 corrections, 2 acknowledgements) and the printed card images. TCG Live images are at `https://cdn.malie.io/file/malie-io/tcgl/cards/jpg/en/me6/me6_en_<NNN>_std.jpg`; TCGdex images, once published, are at `https://assets.tcgdex.net/en/me/me06/<NNN>/high.png`.
- **What to look for:** Open at least these cards and confirm the built set matches the printed card:
  - `me06-011` Mega Golisopod ex: Stage 1, suffix `ex`.
  - `me06-088` Talonflame ex: Stage 2.
  - `me06-080` Azurill: one cost-free attack, Bouncy-Bounce Charge, and no Ability.
  - `me06-094` and `me06-096` Legendary Pyroduct and Summit: the effect starts with "You cannot play this card by itself. You must combine 2 different Legendary … from your hand to play as 1 Stadium card."
  - Two more corrections of your choice.

  Compare with `jq '.prints[] | select(.id=="me06-080")' cardsets/sets/me06.json`.
- **Pass:** Every checked card matches its printed image.

## 4. Review: follow the documentation to import a set and record a correction

- **What to look at:** `README.md` (Development → Card sets) and `cardsets/README.md`.
- **How:**
  1. Using only the docs, re-import a set: `bin/tcgreplay sets import DLR`.
  2. Record a test correction in `cardsets/corrections/me06.json`, for example change `/hp` of `me06-001` to 140 with a reason. Re-import and confirm that `cardsets/sets/me06.json` shows HP 140 and the corrections file is unchanged.
  3. Record an acknowledgement as the docs show. Run `bin/tcgreplay sets check`.
  4. Revert both test edits with `git checkout cardsets/`, or by hand if the files are uncommitted.
- **Pass:** The steps can be followed from the docs alone, the correction shows in the built set, the corrections file is left as written, and `sets check` behaves as the docs describe.

## 5. Review: the unverified gaps in the older sets (follow-up for the maintainer)

- **What to look at:** The 196 acknowledgements whose reason starts with "Unverified". Find them with `grep -l Unverified cardsets/corrections/*.json`; they are spread over 47 sets.
- **What to look for:** For each one, read the printed card and either replace the acknowledgement with a correction carrying the printed text, or keep it with an updated reason. There are 117 no-attacks, 30 missing-evolves-from, 27 missing-effect and 22 missing-name gaps. The user accepted leaving these for later (decision of 2026-11-08).
- **Pass:** Not a release gate. Track it as follow-up work until no "Unverified" acknowledgements remain.
