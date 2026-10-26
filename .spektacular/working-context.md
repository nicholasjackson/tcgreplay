# Working context: wait-for-turn spec

## Problem / motivation
In play mode the player (human, on the board) and the agent (Claude, own seat) alternate turns.
Today, when the player ends their turn on the board, the player has to tell the agent in chat
("your turn") before it continues. The user wants the agent to wait until the player is finished
and then carry on automatically, "so that I do not have to say your turn so that we can simulate
playing a game". The user also wants to "ensure that the agent understands this process".

User's phrasing: "during my turn the agent will wait until I am finished before continuing with
it's turn. This could be implemented as a long running tool based on the rules of pokemon. i.e.
30s timeout etc. This would stop the lock running forever."

## Discussion so far (from the pre-spec exchange)
- Proposed: new MCP tool `wait_for_turn`, called by the agent straight after `end_turn` in play mode.
  Blocks until: the player ends their turn (returns the hand-off: turn, get_log since point, summary);
  the agent must act during the player's turn (promote a new Active after a knockout); or the game
  leaves play mode / is replaced / closed.
- Two time limits: a per-call cap (~30-60 s, under client/proxy timeouts; returns "still waiting",
  agent re-calls) and an overall limit after which the agent stops and checks in with the player.
- I flagged that official Pokémon rules have no 30 s turn rule; TCG Live has turn timers but I was
  unsure of exact values, so limits should be settings, not quoted rules.
- Agent understanding: play-mode line in server instructions; possibly a `play-game` guide.
- User accepted capturing as a spec; name `wait-for-turn`; not part of the hosted-playmat epic.

## Code facts learned
- Repo root: /home/nicj/code/github.com/nicholasjackson/tcgreplay (single registered repo).
- MCP tools: internal/mcpserver/server.go (`add` helper, `instructions` const, guideInstructions()).
- Guides: internal/guides/*.md served by the `guide` tool (internal/mcpserver/guides.go).
- Board end_turn handler internal/webui/actions.go:256 already builds a hand-off prompt in play mode:
  "Your turn in tcgreplay (game %s, branch %s). Call get_log since %d and play turn %d (%s's turn)."
  using g.TurnStartedAt(seat).
- game.Game.Subscribe (internal/game/game.go:128) has no unsubscribe.
- Manager can swap the current game (OnLoad/OnClear/Expire); hosted mode has idle expiry and
  MCP session timeout 4h; hosted http server has no WriteTimeout.
- ClaudeSeat defaults to p2; play mode restricts the agent to its seat.

## Interview answers (user)
- Overall limit 5 minutes. On time-out the agent just asks "Still playing?"; the user replies
  ("oh yes sorry") and waiting resumes. Purpose: lets the user walk away.
- If the user KOs the agent's Active mid-turn (maybe via an ability, not necessarily ending the
  turn), the agent detects it, promotes, and keeps waiting. Promotion is the only mid-turn action.
- Board indicator: user wants the board to show "Opponent is waiting" while the agent waits.
- Confirmed: 5 min counts from the player's last board action (inactivity), restarts on each action.
- Scope widened at the constraints step (user agreed to my recommendation): waiting also applies in
  review mode when the assistant hands the player a turn (replay drill, playing a line out). The
  assistant names the seat it waits on; branch switch/rewind end the wait. Overview, requirements
  and acceptance criteria working files were amended accordingly (no re-confirmation needed; the
  verification step reviews them).
- Constraints confirmed (3 bullets: desktop+hosted, existing tool connection only, playmat not rules engine).
- Technical approach confirmed. User said "just finish the spec, this looks decent": draft the
  remaining sections (success metrics, non-goals) and finish without per-section confirmation;
  only surface genuine issues.
- At the non-goals step the user added: playing a card like Judge or Unfair Stamp that requires the
  opponent to do something must trigger a move from the assistant. Requirements, AC, overview and
  technical approach amended; the "mid-turn actions beyond promotion" non-goal was dropped.
  Card text verified via card_text: Judge sv01-176 (each player shuffles hand, draws 4);
  Unfair Stamp sv06-165 (each shuffles hand; you draw 5, opponent draws 2).
- User chose: board is LOCKED whenever the assistant is acting (its own turn and mid-turn
  responses) — "simplest approach". I added a self-releasing lock limit so it cannot stick.
- Remaining non-goal (no turn timer against the player) kept; user said to finish the spec.
- Verification: fresh-eyes review returned 15 findings; user approved all proposed fixes, incl.
  lock release after 2 min of no assistant moves (default) and hosted idle expiry ends the wait
  (waiting does not keep a game alive). Per-call bound moved to Constraints.
- Split offered at split step; user declined: 'this is a spec with a singular purpose'.
