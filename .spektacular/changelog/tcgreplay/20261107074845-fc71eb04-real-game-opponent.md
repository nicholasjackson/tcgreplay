---
created_date: "2026-11-07"
document_status: draft
project: tcgreplay
spec: 20261107074845-fc71eb04-real-game-opponent
plan: 20261107074845-fc71eb04-real-game-opponent
---

# The assistant reacts like a real opponent during your turn

When you play your turn on the board against the assistant, cards like Judge, Iono, Special Red Card and Unfair Stamp now shuffle away the assistant's hand and make it draw by themselves, as part of your card, with no need to tell it anything. The assistant only stops to think when it has a real decision, such as which Pokémon to promote after a Knock Out, and while it decides you keep playing anything that doesn't depend on that decision. If it doesn't answer in time, the board tells you and lets you make its move yourself.

> Derived from project tcgreplay, spec/plan 20261107074845-fc71eb04-real-game-opponent. See the project-level record for the full feature.

**ChatGPT players:** after updating, open Plugins → TCG Replay → **Refresh**, then start a new chat, so ChatGPT picks up the new `wait_for_turn` behaviour.

## What changed in this repo

- **Effect reader** (`internal/game/effects.go`): reads an English effect text and says what it asks of the opponent: nothing, an effect the playmat applies itself (the shuffle-hand-and-draw family, with its hand destination, draw count, bottom-of-deck draw condition and use condition), a choice, or something it cannot judge. A text is applied only when every sentence is understood. `NamesOpponent` is the looser check the board's hold uses.
- **Applied effects** (`internal/game/applied.go`, `verbs.go`, `game.go`, `reads.go`): the human's Trainers, Abilities and attacks finish their own transaction with the opponent's part of a family effect, under the system actor and the opponent's seat, in play mode and for the seat a turn wait has handed over in review mode. Unmet Unfair Stamp and Special Red Card conditions warn. `appliedSince` describes what was applied.
- **Turn wait** (`internal/game/presence.go`): one `decide` outcome (`promote` with Bench `choices`, or `answer_card` with cards marked `choose` or `read`) replaces `respond` and `promote`; ordinary cards never stop the wait; every outcome carries the applied effects not yet reported; the hold on the board has a full and a decision strength, with the deciding seat and decision readable, and a lapsed decision kept until the next wait.
- **`wait_for_turn`** (`internal/mcpserver/wait.go`, `server.go`): renders `applied`, `decision`, `choices` and `need`; decision results give numbered steps ending with the hand-back call; the tool description and standing instructions describe applied effects and decision stops.
- **Board server** (`internal/webui/actions.go`, `hub.go`): one `held` check refuses everything during the assistant's own turn, and only decision-dependent actions while it decides, with a message naming what it waits for; move results are redacted for the player's view so the opponent's drawn cards never reach the browser; the state message carries the `deciding` status and the `decision`.
- **Board app** (`web/src/`): an "Opponent is deciding" badge with controls left enabled, a one-time notice when a decision lapses, and "Promote to Active" on the opponent's Benched Pokémon while its Active Spot is empty.
- **Guides and website** (`internal/guides/play-game.md`, `replay-drill.md`, `review-game.md`, `site/src/content/docs/workflows/play-game.md`): describe applied effects and decision stops; guide checks pin the wording and forbid "answer every card".
- **Also carried in:** the earlier uncommitted `wait_for_turn` improvements (15-second cap, wait counters, `keep_waiting`, `action`, numbered steps) in `cmd/tcgreplay/main.go`, `timing_test.go`, `presence.go`, `server.go`, `wait.go` and `wait_test.go`.
- **Tests:** new and updated tests across `internal/game`, `internal/mcpserver`, `internal/webui` and `internal/guides`; the board test client's websocket read limit was raised in the shared helper.

## Why

Players practising against the assistant had to wait for it, or instruct it, during their own turn: it was stopped for every card, did its side of Judge or Iono by hand, and locked the board meanwhile. This repo holds the playmat, the turn wait, the MCP tools and the board, so all of it changed here to make the assistant's side react on its own and interrupt only for real decisions.
