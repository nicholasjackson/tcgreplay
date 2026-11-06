---
tags: [mcp, wait-for-turn, guides, chatgpt]
---

# Assistants skip a guide's wait_for_turn hand-off; the tool result has to say it

Telling the assistant in a guide (and in the server instructions) to "ask for the player's play, then call `wait_for_turn` in the same reply" is not enough. ChatGPT rewound to a practice turn, asked "What's your play? You can play it on the gameboard", and ended its reply. This happened in more than one session, and each time the player had to tell it to wait. Guide text is read once and drifts out of focus, and the server instructions are easy to under-weight. A tool result is read at the exact moment of the decision.

So the hand-off now lives where the decision happens:

- `rewind` and `checkout` in review mode, when a turn is in progress, append a line to their result: call `wait_for_turn` with that seat in this same reply, and do not end the reply to wait (`handOffHint` in `internal/mcpserver/server.go`).
- `wait_for_turn`'s own tool description says to call it in the same reply and never end the reply to wait.
- Each `wait_for_turn` result already carries a `next` line that says what to do.

When adding any new flow that hands the player a turn, put the instruction in that tool's result, not only in a guide.

