---
created_date: "2026-10-17"
document_status: final
closed_date: "2026-10-17"
---

# Test plan: 20261017112555-30c99ab1-memory-store-record

Manual procedures for the success metrics and manual reviews the plan's Testing Approach left to people. Automated coverage (record format, corpus round trip, nothing written, restart forgets, tools removed, board state message) lives in `go test ./...` (`make test`).

Common setup:
- **Desktop (dev):** from the repo root, `make dev` builds `bin/tcgreplay` and runs the dev daemon with data in `.dev/` and the board at http://127.0.0.1:7778/. Claude Code in the repo reaches it as the `tcgreplay-dev` MCP server. For the installed desktop app, `make install` and restart the app; its data dir is `~/.local/share/tcgreplay`.
- **Hosted:** `make docker-build`, then `make docker-run` (board at http://localhost:8080/, MCP at http://localhost:8080/mcp), or the deployed app at https://tcgreplay.demo.gs after this branch reaches `main`.
- A real TCG Live battle log and the player's decklist (the `tcg-import-game` skill drives the import).

## Success metrics

### SM1: Every saved record restores with no differences (first 10 real games)
- **What to measure:** over the owner's first 10 real games after delivery, the fraction of saved records that restore with no differences. Threshold: 10 of 10.
- **How:** after each game, import it (`import_log`), copy the `record` from the result (or `get_record`) into the owner's note as usual. Before restoring, note the reveal view at three points: call `rewind` to the first event, the middle of the timeline and the last event (event ids from `get_log`), and at each call `get_state` with `view: "reveal"`, saving the output. Later (at least after one playmat restart), paste the record back (`import_log` with `log` = the record, no other arguments), repeat the same three `rewind` + `get_state view=reveal` calls and diff each against the saved output, ignoring only the `game` id field.
- **Expected result:** all three diffs empty for each of the 10 games; no "damaged" refusal.
- **Who / when:** the owner, over the first 10 games played after release.

### SM2: Records saved to Notion without hand-editing
- **What to measure:** whether the owner can save a record to Notion and restore it without editing it by hand. Threshold: no hand edits in any of the first 10 games.
- **How:** paste each record into a Notion code block straight from the assistant's reply; later copy the code block and paste it back to the assistant for restore.
- **Expected result:** every restore succeeds with no manual change to the text; any "damaged" refusal counts as a failure and its pasted text is kept for analysis (see the plan's open question on note-app substitutions).
- **Who / when:** the owner, alongside SM1.

## Manual reviews

### MR1: Restart forgets, desktop and hosted
- **Where:** desktop dev daemon (`make dev`) and the hosted container (`make docker-run`).
- **Look for:** import a game, check the board shows it; stop and restart the daemon (Ctrl-C and `make dev` again) and, separately, stop and start the container. Open the board and call `get_state`.
- **Pass:** after each restart the board shows "No game open" and `get_state` errors with "no game open: import a battle log or a saved game record with import_log, or start one with new_game".

### MR2: Nothing written on a real install
- **Where:** the installed desktop app's data dir `~/.local/share/tcgreplay` (with old `games/*.jsonl`, `games/*.import.json`, `current.txt`, `last-deck.txt` from an earlier build).
- **Look for:** before the session, record checksums: `find ~/.local/share/tcgreplay -type f ! -name tcgreplay.log ! -name assistant.txt -exec sha256sum {} + | sort > /tmp/before.txt`. Run a real session: import, moves, a rewind branch, a note, `set_opponent_deck`, `new_game`. Repeat the command into `/tmp/after.txt` and `diff /tmp/before.txt /tmp/after.txt`.
- **Pass:** empty diff (no new, changed or removed game or import files; the old files are still there, byte-for-byte).

### MR3: Exact restore by hand
- **Where:** desktop dev daemon or hosted, with a real game.
- **Look for:** the SM1 procedure for one game, done by hand, with a restart of the playmat between saving and restoring.
- **Pass:** the reveal view (hands, prizes, deck order, discard) matches at the start, middle and end.

### MR4: Paste round trip through Notion
- **Where:** a Notion page with a code block.
- **Look for:** paste a record into the code block, copy it back out, and give it to the assistant to restore.
- **Pass:** the import succeeds and restores the same game (compare `get_state view=reveal` at the last event with the original, ignoring `game`).

### MR5: Damaged paste through a real note app
- **Where:** a note app that curls quotes (e.g. a Notion text block, not a code block, or Apple Notes with smart quotes on).
- **Look for:** paste a record as plain text so quotes curl, copy it back and restore it.
- **Pass:** either the identical game is restored, or the import is refused with "this game record was damaged on the way (its check does not match), so it was not restored..."; never a different game.

### MR6: Drills on the open game
- **Where:** an assistant with the bundled skills installed (`tcgreplay install -client claude|codex|gemini`) connected to the playmat.
- **Look for:** with a reviewed game open (findings recorded by `tcg-review-game`), run `tcg-replay-drill`, `tcg-numbers-drill` and `tcg-before-you-move`; then, after a restart with no game open, ask for each again.
- **Pass:** with a game open each drill runs on it; with none open each asks the player to import a battle log or saved record and stops; none errors or mentions listing or loading games.

### MR7: Board landing page
- **Where:** http://127.0.0.1:7778/ (dev) or http://localhost:8080/ (hosted) with no game open.
- **Look for:** the landing text and any buttons.
- **Pass:** it reads "No game open" and "Ask <assistant> to import a battle log or a saved game record, or to start a new game.", with no stored-game buttons; after an import the board shows and follows the game.
