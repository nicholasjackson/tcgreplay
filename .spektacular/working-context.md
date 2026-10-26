# Working context: plan for 20261026104139-4e1bb7fa-wait-for-turn

## Decisions / user answers
- User chose spec 20261026104139-4e1bb7fa-wait-for-turn (only spec without a plan).

## Learnings carried from the spec session
- Repo root: /home/nicj/code/github.com/nicholasjackson/tcgreplay (single registered repo; verify via `spektacular repo list`).
- Board end_turn handler internal/webui/actions.go already builds a play-mode hand-off prompt.
- game.Game.Subscribe (internal/game/game.go) has no unsubscribe (risk: listener leak).
- Manager swaps games (OnLoad/OnClear/Expire); hosted mode has idle expiry; MCP session timeout 4h.
- Judge sv01-176: each player shuffles hand, draws 4. Unfair Stamp sv06-165: you draw 5, opponent draws 2.

## Discovery (done)
- research.md + assumptions.md written in .spektacular/work/<plan>/.
- Direction: per-Manager "presence" in internal/game (wait session, acting lock, board activity clock),
  subscribed once per game in setCurrent; broadcast channel wakes long-poll waits (no per-call listeners).
- Update gains a change kind (mode/checkout/rewind/undo); SetMode notifies.
- Hosted: new quiet resolve hook using Registry.Get so waits are not activity.
- Knowledge written: gotchas/game-listeners-under-lock.md (repo tier, tcgreplay).
- Architecture done: Option A (per-manager presence). Response window relaxes play-mode turn lock for assistant seat during a respond outcome. Timings flags+env, cap<=50s.
- Drafting done (tasks: 9, ids in tasks_plan.md). Next: assemble.
- Assembled + staged to .spektacular/tmp/<plan>/ (slug = spec name; plan store uses spec names, no NNNN slug).
- All 3 docs committed; work dir removed. Now in walkthrough awaiting user sign-off.
