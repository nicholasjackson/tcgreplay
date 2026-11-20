---
tags: [webui, game, play-mode, security]
---

# The board's write results carry an unredacted delta

`runLocked` builds every write's `Result.Delta` with no viewer: `describe(&e, before, g.st, "")` (`internal/game/game.go`, in `runLocked`). Each line names every card it moves, hidden or not.

- **The MCP side redacts it.** `Server.redact` (`internal/mcpserver/server.go`) rewrites the delta with `g.DeltaFor(r, s.ClaudeSeat)` in play mode, so the assistant never sees the player's hidden cards.
- **The board does not.** `webui.Server.dispatch` (`internal/webui/actions.go`) copies `gr.Delta` into the websocket result as is. The board UI does not display `delta`, but the card names still reach the browser.
- Today this leaks nothing, because a human write on the board moves only the human's own cards (in play mode the board refuses a foreign seat). Any human write that also moves the opponent's hidden cards would send their names to the browser: for example, the playmat applying the opponent's draws from the player's Judge.
- `Game.Update.Delta` is also unredacted. The board hub uses only `u.Events` and renders redacted snapshots, so it is safe today; a listener that forwards `Delta` would not be.

When a board write can touch the other seat's hidden cards, redact its delta with `g.DeltaFor(res, viewer)` for the board's viewer, as the MCP side does.
