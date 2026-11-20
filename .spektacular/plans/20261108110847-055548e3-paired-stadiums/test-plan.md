---
created_date: "2026-11-08"
document_status: final
closed_date: "2026-11-08"
---

# Test plan: 20261108110847-055548e3-paired-stadiums

These checks are manual. Everything else is covered by the automated suites:
- `go test ./...`
- the board's Playwright suite (`make web && cd web && npm run test:e2e`), whose drag, zoom and phone pair tests run on desktop, tablet and phone;
- `make site-test`.

Run the checks after the merge to `main`, once the "Publish card sets" workflow has published the rebuilt `me06` and the hosted playmat has redeployed.

## 1. Success metric: Delta Reign's Legendary Stadiums play as pairs in a real game, by hand and by the assistant, with no workarounds

- **What:** a real game in which a Legendary Stadium pair is played by the player on the board and, in another turn or game, by the assistant. Nothing needs fixing by hand: no `move` corrections, no stray cards.
- **How:**
  1. On the hosted playmat (or a desktop playmat built from `main`), start a game with `new_game`. Give the player a decklist holding `1 Legendary Summit DLR 96` and `1 Legendary Summit DLR 97` (any Legendary pair works), and give the assistant's deck a pair too.
  2. **Player:** drag one Legendary Summit from the hand onto the Stadium spot. The header warning (⚠ on a mat; the ☰ badge and list on a phone) reads `Legendary Summit (pN-…) is played as a pair: play a second, different Legendary Summit with it`. Drag the second print onto it. The timeline shows `… → stadium as pair`, both halves show in the spot, and the warning is gone. Undo once: both halves return to the hand.
  3. **Assistant:** in a turn where it holds both prints, check that it calls `play_trainer` with `iid` and `pair_iid`. Its log line reads `pN moved pN-x Legendary Summit + pN-y Legendary Summit pN:hand → stadium`.
  4. Play a different Stadium over the pair. Both halves land in their owner's discard in one timeline line, and undo returns both to the spot.
- **Pass:** every step behaves as described, with no hand correction.
- **Who / when:** the maintainer, after the merge, on the hosted playmat and on desktop.

## 2. Manual review: hosted Delta Reign data reads with the pair rule

- **Where:** the hosted playmat after publish; `/admin` shows the loaded catalogue version.
- **What:** open the board zoom on Legendary Pyroduct, Summit and Trench (both prints of each, `me06-094` to `me06-099`), and have the assistant run `card_text` on each.
- **Pass:** each text starts "You cannot play this card by itself. You must combine 2 different Legendary <name> from your hand to play as 1 Stadium card." and has no duplicated rule line. Each of the three plays as a pair, as in check 1, step 2.

## 3. Manual follow-up: verify the importer against the first real TCG Live log with a Legendary Stadium pair

- **Why:** no real log exists. The importer's pair handling was built against a made-up log: `testdata/logs/synthetic_2026-11-08_ledjo23_legendary-summit-pair.txt`, marked `"synthetic": true` in `testdata/index.json`. The line shapes are assumed (knowledge `gotchas/stadium-pair-log-format-unverified.md`). This item replaces the spec's "battle log with a pair imports" acceptance criterion, which stays unticked until it is done.
- **How:** when a real game with a Legendary Stadium pair is played, export its battle log from TCG Live and run `TRACE=<log> go test -run TestTrace ./internal/importer`. Then import it on the playmat with `import_log` and the owner's decklist.
- **Pass:** the import reports no problems, and the board shows the pair in the Stadium spot (or both halves in the discard where the log replaces it). If TCG Live prints the pair differently, fix `playStadium` in `internal/importer/handlers.go` from the log's lines only, then add the real log and its oracle entry to the corpus.
- **Who / when:** the maintainer, as soon as such a log exists.

## 4. Manual review: the pair on a real phone and on desktop

- **Where:** the board on a real iPhone (Safari) and on a desktop browser, with a pair in the Stadium spot (check 1, step 2).
- **What:** both halves are visible, not clipped, and clear of the Active Spot and the turn controls. Each half opens its own zoom: tap on a phone, click on desktop. Hovering one half on desktop lifts it above the other. While the lone-half warning toast shows, the second half can still be dragged from the hand (toasts no longer take touches).
- **Pass:** all of the above.
- **Who / when:** the maintainer, after the merge.

## 5. Manual review: follow the documentation

- **Where:** the site page "Play a game" (`/docs/workflows/play-game`, the "Stadiums played as a pair" paragraph), `cardsets/README.md` (the `playedAsPair` field, the correction example, and "A new field in the format reaches older playmats as an error"), and the design notes (`TCG Review Playmat Design and Build Plan.md`).
- **What:** play a pair on the board and ask the assistant to play one by following only the docs. Check that the card-set guide is enough to mark a new pair-played card.
- **Pass:** the steps work as written, with no step missing.
- **Who / when:** the maintainer, after the merge.

## 6. Manual check: ChatGPT sees `pair_iid`

- **What:** ChatGPT caches tool descriptions (knowledge `gotchas/chatgpt-tool-discovery.md`).
- **How:** in ChatGPT's app settings, refresh the tcgreplay connector's tools, then check that `play_trainer` lists `pair_iid`.
- **Pass:** `pair_iid` is present, and ChatGPT can play a pair with it.
- **Who / when:** the maintainer, after the hosted redeploy.

## 7. Rollout watch: older desktop playmats after the `me06` publish

- **What:** playmats built before this release cannot decode the rebuilt `me06`. They keep their catalogue but take no further card-set updates until upgraded.
- **How:** on a desktop playmat still running the previous release, watch its log after the publish for `card sets: check failed (…); keeping version …`, check that games still open with the old cards, then upgrade it and check that it picks up the new catalogue.
- **Pass:** the old playmat keeps working on its old catalogue, and the upgraded one loads the new catalogue, including the Delta Reign pair marks.
- **Who / when:** the maintainer, after the publish.
