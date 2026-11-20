---
created_date: "2026-11-07"
document_status: final
closed_date: "2026-11-07"
---

# Test plan: 20261107074845-fc71eb04-real-game-opponent

Success metrics 1–3 are covered by automated behavioural tests (`TestOrdinaryTurnNeverStops`, `TestDecisionsPerTurn` and `TestPromotionDecision` in `internal/game/presence_respond_test.go`; `TestDecisionHoldAllowsIndependentActions` and `TestDecisionHoldLapses` in `internal/webui/lock_test.go`). The procedures below cover the one manual metric and the two manual reviews the plan lists.

## Setup used by every procedure

- **Hosted run (ChatGPT, claude.ai):** deploy this branch to the hosted playmat (staging if available) and connect the assistant to its MCP endpoint (`https://<host>/mcp`) with a signed-in Google account.
- **Local run (Claude Code, board review):** from the repo root, `make dev` builds and starts the dev daemon; the board is at `http://127.0.0.1:7778/`, and Claude Code reaches it through the `tcgreplay-dev` server in `.mcp.json`. To test the lapse notice without waiting the default 2 minutes, run the binary with a shorter release: `make build` then `./tcgreplay -daemon -replace -data <dev data dir> -ui-addr 127.0.0.1:7778 -log-stderr -lock-release 20s` (use the paths `make dev` prints).
- **Decks:** a player decklist containing Judge (for example `testdata/decks/mega-lucario-ex-v12.txt`, which has 2 Judge POR 76) and any opponent list with at least two Basic Pokémon, so the opponent has a Bench to promote from.
- **Refresh tools first (ChatGPT):** this release changes the `wait_for_turn` description and its outcomes, and ChatGPT keeps the tool list it last fetched. In ChatGPT open Plugins → TCG Replay → **Refresh**, confirm the `wait_for_turn` description mentions `applied` and outcome `decide`, then start a **new** chat.

## Metric 4 — a played-out turn against ChatGPT needs no prompting (manual)

- **What to measure:** in one player turn that includes Judge and a knockout, the number of times the player has to tell the assistant to act or to keep waiting. **Threshold: 0.**
- **How:**
  1. After refreshing tools (above), in a new ChatGPT chat say: "Let's play a game. Here's my deck: (paste the decklist). You play this one: (paste the opponent list)." Open the board link it gives.
  2. Play until it is your turn with the opponent holding a hand and at least one Benched Pokémon. Make sure Judge is in your hand (draw or search for it on the board if needed).
  3. On the board, play Judge. Do not type anything in the chat.
  4. Attack and knock out the opponent's Active Pokémon. Do not type anything.
  5. Take your prize, then end your turn on the board. Do not type anything.
  6. Watch the chat and the board throughout. Count every time you have to type something to make the assistant act, promote, answer a card or keep waiting.
- **Expected result:**
  - After Judge, the opponent's hand count on the board changes to 4 by itself, with no chat message.
  - The board briefly shows "Opponent is deciding" after the knockout, the opponent promotes a Benched Pokémon without being asked, and the badge returns to "Opponent is waiting".
  - After you end the turn, the assistant plays its turn and reports it.
  - **Prompts typed: 0.** Any prompt needed is a fail; record which step and what the assistant said.
- **Who / when:** the maintainer, on the hosted staging deploy, before release.

## Manual review — board label, lapse notice and "Promote to Active" (desktop and phone)

- **Where:** the board, on a desktop browser window and on a phone (or the browser's phone emulation at about 390 px wide), against the local dev daemon started with `-lock-release 20s`.
- **What to look at and what passes:**
  1. **"Opponent is deciding" label.** With the assistant waiting on your turn (Claude Code on `tcgreplay-dev`, play mode), knock out the opponent's Active Pokémon while it has a Bench. Pass: a dashed accent badge reading "Opponent is deciding" appears beside the opponent's name (on the phone, in the top seat name line), it is legible in light and dark themes, and your hand, cards, menus and End turn button stay usable while it shows. Try attaching an Energy: it works. Try ending the turn: a notice reads "Waiting for your opponent to promote a new Active Pokémon; try again when they have."
  2. **Lapse notice.** Repeat the knockout, then stop the assistant from answering (cancel its run in Claude Code). Pass: after about 20 s the badge changes to "Opponent stopped responding", and one notice appears once: "Your opponent hasn't answered. Carry on, and make their move yourself if you need to (for example, promote one of their Benched Pokémon)." It does not repeat on later state updates.
  3. **"Promote to Active".** While the opponent's Active Spot is empty, open the menu on one of its Benched Pokémon. Pass: the menu offers "Promote to Active"; choosing it after the lapse moves that Pokémon into the Active Spot; choosing it while the badge still shows "Opponent is deciding" shows the waiting notice instead and changes nothing. On the phone the entry is reachable and at least 44×44.
- **Who / when:** the maintainer, locally, before release.

## Manual review — a played-out turn against Claude

- **Where:** claude.ai (hosted connector) and Claude Code (`tcgreplay-dev`), each once.
- **Note:** claude.ai currently ignores MCP server instructions (see knowledge `learnings/publishing-to-claude.md`), so this check shows whether the tool description and each result's `next` are enough on their own.
- **What to look at:** play the same turn as Metric 4 (Judge, a knockout of a Pokémon with a Bench behind it, take a prize, end the turn) without typing anything during it. Read the assistant's tool calls (the tool-use panel in claude.ai, or the transcript in Claude Code).
- **What passes:**
  - After Judge, the `wait_for_turn` result lists Judge in `applied`, and the assistant does **not** shuffle or draw for its side again.
  - After the knockout, the result is `outcome: "decide"`, `action: "promote"` with `choices`; the assistant calls `switch` with seat p2 and a listed `to_slot`, then calls `wait_for_turn` again **in the same reply**, without posting in chat or asking the player anything.
  - It keeps calling `wait_for_turn` until `turn_ended`, then plays its turn.
  - Any extra chat message during the player's turn, any repeated shuffle/draw for Judge, or a hand-back in a later reply is a fail; note the client and the step.
- **Who / when:** the maintainer, before release.
