DONE summary for 20261017112555-30c99ab1-memory-store-record (kept for Step 6 of plan-epic)

Approach: every game (imported or new_game) lives only in the store's in-memory mode, desktop AND hosted. Store file mode (Create/Open) removed; manager List/Load/Resume, games/*.jsonl, <id>.import.json, current.txt, last-deck.txt removed; files on disk left alone, never read. Each imported game carries a record on Game: versioned JSON (log, player name, deck, opponent list as passed, seed as decimal string, SHA-256 check). Record returned by import_log, set_opponent_deck, and new get_record tool. Restore by passing record as import_log's log. Check covers values not JSON layout; on fail, undo curly quotes, NBSP, CRLF and accept only if check matches, else refuse as damaged. Verified all 10 corpus logs re-import identically (temp test, removed). Rejected: wiped temp dir, leaving file mode unused, events in record, separate restore tool.

Milestones (all agent; no human tasks):
M1 Every imported game comes with a record that brings it back exactly:
- Define the game record format
- Build and restore games from records (round trip over corpus)
- Give the assistant the record through the tools
- Tell the player about the record when importing (import skill)
M2 The playmat keeps nothing between sessions:
- Keep games in memory only (store, game, mcpserver, webui, cmd; removes list_games/load_game and every Resume; set_opponent_deck drops id/new_id, adds discard_branches)
- Remove stored games from board page (Svelte)
- Point drills at the game open now (three drill skills + opponent-deck; skills test against removed tools)
- Describe memory-only games in design doc

Out of scope: drilling across many past games; records for new_game games; keeping branches/notes/findings across sessions; card cache to memory; per-user games; Notion & skill packaging; migrating/deleting old game files; moving assistant.txt; backward compat for removed tools/params.

Drafting assumptions: record = five raw-game fields + version marker + check; seed decimal string; file mode deleted not left unused; restore via import_log log text detected by leading { and tcgreplay_record marker, record + deck/opp_deck/me refused; repairs accepted only when check matches; record keeps opp list as passed; set_opponent_deck current game only, refuses game with branches unless discard_branches:true; records first then storage removal; in M1 only, disk-loaded games have no record; restoring sets session's last deck; assistant.txt stays on disk, nothing-written test excludes it and process log; board landing-page task has no deps.

Project-wide rules:
- Relies on web-mcp-docker hosted mode, BoardURL/board switch, two-transport MCP tests. board key stays on new_game, import_log, set_opponent_deck.
- DIFFERS from web-mcp-docker: runHosted m.Resume() removed; load_game board link and hosted-instructions mention removed; container no longer writes game files under $HOME.
- Decides: no game storage in any mode; downstream must not add Resume/load_game back.
- Decides: record lives on Game; last deck in Manager memory; per-user-games would make these per account.
- Decides: record format {tcgreplay_record:1, me, seed(string), deck, opp_deck, log, check:"sha256:…"}.
- Otherwise same: hosted/desktop switch unchanged (no hosted-only behaviour added); make test; no hand-edited changelog; deployment untouched.

Manual checks:
- Success metric: first 10 real games, every saved record restores with no differences.
- Success metric: owner saves records to Notion without hand-editing.
- After import, restarting desktop daemon and hosted container leaves no game open.
- Real install: existing game files unchanged byte for byte; no new game/import files.
- Exact restore by hand: reveal view at start/middle/end matches after restart + record import.
- Paste round trip via Notion code block restores same game.
- Damaged paste through real note app restores identically or refused with damaged message.
- Drills: with game open each runs on it; with none, asks for import; none mentions listing/loading games.
- With no game open, board landing page shows no stored-game buttons.

Discoveries:
- Importer deterministic given log, deck text, opp list, owner name, seed; only randomness one seeded PCG; card-db lookups only fill Def. Tags: importer, determinism, seed, record.
- skills/skills_test.go rejects any skill containing "Claude" or "Nic ". Tags: skills, testing, conventions.
