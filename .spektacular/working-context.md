# Working context: real-game-opponent (plan workflow)

Plan for spec `20261107074845-fc71eb04-real-game-opponent`. Single repo `tcgreplay` at /home/nicj/code/github.com/nicholasjackson/tcgreplay. No design references.

## From the spec workflow (still binding)

- User's goal: "play like a real game where possible. I don't want to have to tell everything." Telling the opponent via a board step was rejected (bad UX). Streaming events, server push, deferring effects to end of turn, request ids: all rejected.
- Applies in play mode and in review-mode practice; unrecognised opponent cards stop and ask; no off switch; saved records need not stay compatible.
- Hold covers ending the turn and lifts after the board's release time; unmet card conditions still apply with a warning; Special Red Card (not Red Card) in the minimum set.

## Plan learnings and decisions so far

- Uncommitted work on main (15 s wait cap, `action` field, numbered steps, `keep_waiting`, `wait`/`waited`) is the base this plan builds on.
- Real TCGdex wording differs from the test fixtures: Special Red Card puts the hand on the bottom and draws 3 only "if they put any cards on the bottom"; its condition is the opponent having 3 or fewer Prize cards. Iono's draw has "If either player put any cards on the bottom…". The cardstest Iono (`tst-4`) is an older wording.
- Applied effects: emitted inside the opener's own tx, stamped ActorSystem and the opponent's seat (one commit; undo by action covers them).
- "Involves the opponent" = the opponent / each player is the subject of an action; possessive-only mentions (gusts, damage modifiers) don't stop.
- Board result delta is unredacted (knowledge entry written: gotchas/board-delta-unredacted.md); must redact for hidden info AC.
- Board can already promote the opponent's Pokémon via `move {iid, to:"p2:active"}`; needs a menu entry only. End turn with empty Active is already refused by the game.
- Undo/rewind still end the wait (left unchanged; recorded assumption).
- Judgement calls are logged in .spektacular/work/<plan>/assumptions.md.
- Architecture chosen (Option A): effects.go reader (none/apply/choice/unknown); in-tx application on PlayTrainer/UseAbility/Attack for human vs assistant side (play mode always; review only while a wait has handed that seat over — presence sets a "handed" seat on Game); one `decide` outcome (action promote|answer_card) + `applied` list; presence hold full|decision; board refuses only dependent actions under a decision hold; board "Promote to Active" on opponent bench via `move`; site play-game doc gains a paragraph.
- Drafting steps done through out_of_scope (10 tasks, 3 milestones). Task titles avoid apostrophes so context.md anchors resolve. One open question (system-actor draws vs draws-like-main pinning).
- All three plan documents committed to the store (2026-11-07); working dir removed. Now in walkthrough: read docs back with `spektacular plan file read <name> <doc>`.
- Walkthrough: user raised other-language cards. Answer: card data is always English (TCGdex /v2/en, cards identified by TCGdex id). Applied: empty Trainer text → "read" (never silent pass); out-of-scope entry for other-language text; open assumption. Knowledge entry learnings/card-text-always-english.md written. User signed off on the plan (2026-11-07).
