---
created_date: "2026-10-19"
document_status: final
closed_date: "2026-10-20"
---

# Test Plan: 20261019170829-bfb31628-turn-numbering-by-round

Server surfaces (board view, log lines and text, branches, errors, end-turn prompt, import report, skill text) are covered by the Go test suite (`make test`). The items below have no automated harness and are checked by hand.

## Setup (shared by all procedures)

1. From the repo root, build and run the dev daemon: `make dev`. This runs `make web` first (needs `npm ci` in `web/` once), then serves the playmat at http://127.0.0.1:7778/ with its own data dir (`.dev`).
2. In Claude Code in the same repo, the `tcgreplay-dev` MCP server (from `.mcp.json`) talks to that daemon.
3. Get a game with at least five turns each: import a bundled log, e.g. ask the assistant to import `testdata/logs/2026-10-11_haagensen_ns-zoroark-rematch.txt` (5 turns each), and open the board link it returns. Switch on **Review mode**.

## M1 — Playmat header shows the combined turn (manual review)

- **What**: the Strip header names the combined turn and the player.
- **How**: with the board at the head of the game (the second player's 5th turn), read the header in the middle of the mat. Then use the timeline to rewind to the second player's half of Turn 3 and read it again.
- **Pass**: it reads `Turn 5 · <name>'s turn` (or `Turn 5 · Your turn` when that seat is yours) at the head, and `Turn 3 · …` after the rewind. It never shows a number above 5 (e.g. never `Turn 6` or `Turn 10`).
- **Who / when**: the developer, before merging/release.

## M2 — Timeline groups both players under one turn (manual review)

- **What**: one section per combined turn, with a sub-heading per player's half.
- **How**: scroll the Timeline panel from top to bottom.
- **Pass**: headings read `Setup`, then `Turn 1` … `Turn 5` and nothing higher. Each Turn section holds two sub-headings, in playing order (the first player's first), labelled `Your turn` or `<name>'s turn`, each with a coloured left border matching the seat. Setup has no sub-heading. Turn headings stay sticky while scrolling.
- **Who / when**: the developer, before merging/release.

## M3 — Rewind and play from either half (manual review)

- **What**: per-player rewind and play from the timeline.
- **How**: in `Turn 3`, click the second player's sub-heading (↺ appears on hover). Note the branch picker and the board. Then click the first player's sub-heading in `Turn 3`. Then click ▶ next to each sub-heading in turn.
- **Pass**: the first rewind lands on branch `turn-3-p2` (seat of the second player) with the board at the start of that player's third turn, opening draw in hand. The second lands on `turn-3-p1` at the start of the first player's third turn. The two branch names differ. ▶ on a sub-heading starts playback at the first line of that player's half; ▶ on the `Turn 3` heading starts at the first player's half.
- **Who / when**: the developer, before merging/release.

## M4 — Playback bar and effect chips show the combined turn and player (manual review)

- **What**: the playback position and lingering-effect chips name the combined turn and the player.
- **How**: (a) start playback from the second player's half of `Turn 3` and step with ▶| through it; read the position text in the playback bar. (b) Ask the assistant to call `add_effect` with `text: "Items locked"`, `until_turn: 6`, `until_seat: "p2"` (or `p1`), and read the effect chip in the middle of the mat.
- **Pass**: (a) the bar shows `<i> / <n> · T3 · <second player's name>` while stepping through that half (and `Setup` for setup steps). (b) the chip reads `Items locked · <seat> · until end of T6 · <that seat's name>`.
- **Who / when**: the developer, before merging/release.

## M5 — No per-player turn count on any playmat surface (success metric)

- **What**: success metric "No surface a player or the assistant reads shows a per-player turn count", for the UI surfaces not covered by tests.
- **How**: while doing M1–M4, also look at the action labels in the timeline (`Turn N:` labels on turn-start lines) and the turn start/end lines (`turn N start (pX)`).
- **Pass**: across header, timeline headings and lines, playback bar and effect chips, no turn number exceeds the combined count (5 for the bundled game), and every turn mention names a player or seat. Branches and notes made before this change are exempt.
- **Who / when**: the developer, before merging/release.

## M6 — The assistant names turns by combined number in a review (success metric)

- **What**: success metric "the assistant names turns by the combined number without a '(your Nth turn)' translation, and the player does not need to correct a turn reference".
- **How**: in a fresh Claude Code session in this repo (so it loads the updated `skills/tcg-review-game`), with the imported game open, ask for a review of the game and go through at least two decision points.
- **Pass**: every turn the assistant names is in the form "turn N, your turn" / "turn N, <opponent>'s turn", matching the timeline headings; it never adds "(your Nth turn)" or uses a per-player number; the branches it creates are named `review-t<N>-<seat>`; and you never need to correct a turn reference. Any correction is a fail.
- **Who / when**: the player (repo owner), after merge, in the first real review session. Skill copies installed outside the repo must be re-uploaded from `skills/` first if that's where the session loads them from.
