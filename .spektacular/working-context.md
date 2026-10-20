# Working context: guide-tool spec

## Problem
The coaching workflows (tcg-import-game, tcg-opponent-deck, tcg-review-game, tcg-replay-drill, tcg-before-you-move, tcg-numbers-drill) only reach an assistant if they are installed as Agent Skills: `tcgreplay install` copies them from the binary (`skills.FS`, `//go:embed tcg-*`) into the client's skills dir (Claude `.claude/skills`, Codex `.agents/skills`, Gemini `.gemini/skills`). claude.ai users must upload them by hand, and installed copies go stale (e.g. the turn-numbering change needed a re-upload). The user asked: "how can connecting an llm to tcg replay teach it how to use it without needing to install skills?" — and then: "I really like the idea of the guide tool", "I would like to get rid of the skills so let's see if we can build a guide tool".

## Direction discussed (assistant's proposal, user liked the guide tool)
- A `guide` MCP tool: lists the guides and returns one by name, served from the text already embedded in the binary. Works in every MCP client because every client supports tools.
- One line in the server instructions saying guides exist and when to read them.
- Next-step hints in tool results (e.g. import_log result points at the review guide).
- Possibly the same text as MCP prompts (clients like Claude Code surface them as commands) — offered as optional.
- Server instructions are injected every turn, so they should stay short; the full guide text is fetched on demand.

## Facts
- Skills total ~4,100 words across six SKILL.md files; skills cross-reference each other by name ("offer to review it first with tcg-review-game").
- `skills/skills_test.go` enforces format and that tcg skills never mention "Claude" or "Nic ".
- `.claude/skills` and `.agents/skills` in this repo are symlinks to `skills/` (also holds spek-* skills for Spektacular, which are out of scope).
- Live hosted instance: https://tcgreplay.demo.gs (MCP at /mcp, Google OAuth); also desktop daemon via `tcgreplay install`.

## Interview answers (2026-10-20)
- Spec name: guide-tool; no epic.
- Delete skills/tcg-* outright; guides in a new home/format. Remove `tcgreplay install` entirely ("get rid of install too, I can manually delete").
- Discovery: server-instructions list + MCP prompts. No result hints. Assistant-only audience. Same six workflows, same content.
- User (2026-10-20): "just create the spec and accept the reviewers comments" — draft remaining sections without asking, apply reviewer findings.
- Spec stored after reviewer fixes (guide names: import-game, opponent-deck, review-game, replay-drill, before-you-move, numbers-drill).
