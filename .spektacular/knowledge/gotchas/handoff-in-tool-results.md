---
tags: [mcp, wait-for-turn, guides, chatgpt]
---

# Assistants skip a guide's wait_for_turn hand-off; the tool result has to say it

Telling the assistant in a guide (and in the server instructions) to "ask for the player's play, then call `wait_for_turn` in the same reply" is not enough. ChatGPT rewound to a practice turn, asked "What's your play? You can play it on the gameboard", and ended its reply. This happened in more than one session, and each time the player had to tell it to wait. Guide text is read once and drifts out of focus, and the server instructions are easy to under-weight. A tool result is read at the exact moment of the decision.

So the hand-off now lives where the decision happens:

- `rewind` and `checkout` in review mode, when a turn is in progress, append a line to their result: call `wait_for_turn` with that seat in this same reply, and do not end the reply to wait (`handOffHint` in `internal/mcpserver/server.go`).
- `wait_for_turn`'s own tool description says to call it in the same reply and never end the reply to wait.
- Each `wait_for_turn` result already carries a `next` line that says what to do.

**Never return the same result twice in a row.** ChatGPT stopped waiting after two `still_waiting` results in a row, although each said "Call wait_for_turn again… Post nothing in chat." Those results were identical, with the same `since` and the same text, which plausibly looks like a stuck loop to an agent. Each `still_waiting` now carries a rising `wait` count and the time `waited`, and says "The player is still taking their turn… This is not the end of the wait, however many times you see it." Every result also has `keep_waiting`, which is false only after `turn_ended` or `ended`. The default wait per call dropped from 30 s to 15 s (`-wait-cap`, max 50 s for HTTP timeouts), which the user preferred: shorter calls, polled more often.

When adding any new flow that hands the player a turn, put the instruction in that tool's result, not only in a guide.

