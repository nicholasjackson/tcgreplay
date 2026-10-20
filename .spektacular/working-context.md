# Working context: plan for 20261020081616-39b3e658-guide-tool

## Decisions / answers
- User chose to plan the guide-tool spec (2026-10-20).

## Carried from spec session
- Guide names: import-game, opponent-deck, review-game, replay-drill, before-you-move, numbers-drill.
- Delete skills/tcg-* outright; remove `tcgreplay install` entirely; players delete installed copies by hand.
- Discovery via server instructions (one line per guide) + MCP prompts; no next-step hints in tool results.
- `.claude/skills` and `.agents/skills` in this repo are symlinks to `skills/`, which also holds spek-* skills (must keep working).
- `skills/skills_test.go` enforces format and "Claude"/"Nic " bans for tcg skills — move to guides.

## Learnings
- Discovery done: guides go in `internal/guides` (embed *.md); `guide` tool registered without `bind` (no hosted place needed); prompts via go-sdk AddPrompt in `internal/mcpserver`; shim relays raw JSON-RPC so prompts work on desktop.
- Removing install needs main.go to refuse leftover positional args, else `tcgreplay install` silently starts the shim.
- Knowledge convention `conventions/skills-text-and-packaging.md` must be updated to guides (needs user confirmation via spek-knowledge at implement time).
- `TCG Review Playmat Design and Build Plan.md:298` describes skills/install; update it.
- Drafting steps through out_of_scope done (2026-10-20); all section working files are in `.spektacular/work/20261020081616-39b3e658-guide-tool/`. 7 tasks across 3 milestones; task ids from `plan task-id`.
- Assembled and staged plan/context/research to .spektacular/tmp/20261020081616-39b3e658-guide-tool/.
- All three plan documents committed; working dir removed; now in walkthrough.
- User signed off the walkthrough (2026-10-20); no changes requested.
