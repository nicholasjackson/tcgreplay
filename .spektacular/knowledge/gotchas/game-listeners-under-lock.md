---
tags: [game, listeners, concurrency, memory]
---

# Game listeners run under the game lock and are never removed

`game.Game.Subscribe` (`internal/game/game.go`) appends a callback. Two consequences:

- **Listeners run while the game's mutex is held.** `notify` and `checkoutLocked` call every listener from inside a locked method. A listener that calls back into the game (`State`, `Head`, any verb) deadlocks. Listeners only record what changed and signal another goroutine, as the board hub's `watch` does (`internal/webui/hub.go`).
- **There is no unsubscribe.** A listener stays on the game until the game itself is dropped. Subscribe once per game when it becomes current (the hub subscribes from the manager's `OnLoad` hook), never once per request or per tool call. Per-call subscriptions pile up for the life of the game.
