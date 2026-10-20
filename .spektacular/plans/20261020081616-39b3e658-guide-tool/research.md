---
created_date: "2026-10-20"
document_status: final
closed_date: "2026-10-20"
---

# Research: 20261020081616-39b3e658-guide-tool



## Alternatives considered and rejected

### Option: Keep the guides as `skills/tcg-*/SKILL.md` and serve them from there

**Rejected**: the spec removes the skills outright, and `skills/` is the symlink target of `.claude/skills` and `.agents/skills` (`ls -la .claude .agents`), so any `SKILL.md` left there is still offered to this repo's own Claude Code / Codex as a skill — the opposite of "no skills left".

### Option: Top-level `guides/` package (mirroring `skills/skills.go`)

**Rejected**: in favour of `internal/guides/`: only `internal/mcpserver` consumes the text, the skills sat at top level only because client symlinks pointed at them (`skills/skills.go:1-3`), and `internal/` keeps the API private to the binary.

### Option: Register `guide` through `add()` like every other tool

**Rejected**: `add` (`internal/mcpserver/server.go:171-185`) runs `bind`, which in hosted mode calls `Registry.ResolveMCP` → `r.Use(acct)` (`internal/usergames/mcp.go:20-33`), which takes a place and can refuse with "tcgreplay is busy" at the user cap. Reading static text needs no game, so `guide` is registered with `mcp.AddTool` directly and works even when places are full. The HTTP-layer sign-in and rate limit still apply.

### Option: Two tools (`list_guides`, `read_guide`)

**Rejected**: the spec's Technical Approach asks for one tool for listing and reading; one tool with an optional `name` keeps the tool list short.

### Option: Use the skills' full descriptions as the "use when" line

**Rejected**: they are 300–400 characters each (`skills/tcg-*/SKILL.md:3`); six of them in the instructions would be ~2,000 characters sent every turn, against the "Short instructions" constraint. Each guide gets a short, single-line "use when" taken from the "Use when …" sentence of its skill description.

### Option: Next-step hints in tool results (e.g. `import_log` suggesting review-game)

**Rejected**: explicit Non-Goal in the spec.

### Option: Check "names a tool the playmat doesn't have" against a hard-coded list

**Rejected**: it would drift. The check builds the real server and lists its tools and their input-schema properties over an in-memory MCP session (pattern in `internal/mcpserver/server_test.go:47-62`).


### Option: Serve the guides as MCP resources

**Rejected**: client support for resources varies even more than for prompts, and the spec's constraint "Must work in every client" makes a tool the primary path (Technical Approach: "Offer one tool for listing and reading guides; also publish the same text as server prompts").

### Option: Keep the guide text as Go string constants in the MCP server

**Rejected**: ~4,100 words of prose in Go literals is hard to edit and review, loses the history the files carry from `skills/tcg-*`, and breaks the existing pattern of markdown plus `//go:embed` (`skills/skills.go:8-11`).

## Chosen approach — evidence

- `skills/skills.go:8-11` — the text is already compiled in with `//go:embed tcg-*`; the same mechanism (`//go:embed *.md`) serves guides with no runtime fetch ("Guides ship with the playmat").
- `internal/mcpserver/server.go:39-58` — `Server.MCP()` builds the one `mcp.Server` used by every mode: desktop in-process and daemon via `newServer` (`cmd/tcgreplay/main.go:150-155`) and hosted (`cmd/tcgreplay/hosted.go:196`). Registering guides, prompts and the instructions there gives hosted and desktop identical guides by construction.
- `cmd/tcgreplay/shim.go:26-56` — the desktop shim relays raw newline-delimited JSON-RPC to the daemon, so `prompts/list` and `prompts/get` reach the daemon unchanged.
- go-sdk v1.6.0: `(*Server).AddPrompt(*Prompt, PromptHandler)` (`mcp/server.go:202`), `PromptHandler func(context.Context, *GetPromptRequest) (*GetPromptResult, error)` (`mcp/prompt.go:12`), `GetPromptResult`/`PromptMessage` (`mcp/protocol.go:661,1058`). Adding a prompt makes the server advertise the prompts capability.
- `internal/mcpserver/server.go:62-83` — instructions are a const plus a hosted-only board line; the guides block is appended to both, so the existing `ins != instructions` test (`server_test.go:580`) and the hosted board-line tests keep their meaning once updated to include the guides block.
- `skills/skills_test.go` — existing checks to move: format (name/description), "Claude"/"Nic " ban, removed tools (`list_games`, `load_game`, `new_id`), turn-numbering checker (`turnNumberingProblems` + its self-test).
- Snake_case identifiers in today's skill text are all tools (`import_log`, `card_text`, `sample_hidden`, `get_profile`, `get_log`, `get_record`, `get_state`, `list_branches`, `set_opponent_deck`), tool arguments (`opp_deck`, `discard_branches`) or the record marker `tcgreplay_record` (`internal/game/record.go:20`) — so "every snake_case word is a tool, a tool argument, or `tcgreplay_record`" is a workable automatic check.
- `cmd/tcgreplay/main.go:69-75` — `install` is dispatched before `flag.Parse`. Removing it alone would make `tcgreplay install` silently start the shim, because positional arguments are ignored after `flag.Parse()`; the "unknown command" criterion needs an explicit refusal of leftover arguments.
- `cmd/tcgreplay/hosted_test.go:584-673` — `startHosted(t, public).connect(t)` gives a signed-in hosted MCP session; `TestDesktopServerHasNoBoardLinks` (`hosted_test.go:936-955`) shows a desktop session built through `newServer`. Together they allow a hosted-vs-desktop comparison test.

## Files examined

- `skills/skills.go:1-11` — embeds `tcg-*` into `skills.FS`; only importer is `cmd/tcgreplay/install.go:16`.
- `skills/skills_test.go:1-151` — the text checks to move (format, word bans, removed tools, turn numbering).
- `skills/tcg-*/SKILL.md` — six skills, ~4,100 words; cross-references at `tcg-import-game:26,56`, `tcg-review-game:71`, `tcg-replay-drill:13,15`; "skill" in prose at `tcg-import-game:14` ("If another skill records results … This skill only adds") and `tcg-before-you-move:8` ("This skill practises").
- `skills/spek-*` — Spektacular's skills; must stay (constraint).
- `.claude/skills`, `.agents/skills` — symlinks to `../skills`; must keep working.
- `cmd/tcgreplay/install.go:1-282` — the install command (copies skills, registers server for claude/codex/gemini); delete.
- `cmd/tcgreplay/install_test.go:1-125` — its tests; helpers `newInstaller`, `readFile` used nowhere else; delete.
- `cmd/tcgreplay/main.go:1-20` — package doc mentions install and skills; `:69-75` install dispatch; `:77` `-seat` help says "seat Claude plays" (out of scope: not guide text).
- `internal/mcpserver/server.go:39-83` — `MCP()`, `instructions` const, `boardInstruction`; `:171-185` `add` binds every tool; `:401` `register`, tool list.
- `internal/mcpserver/server_test.go:20-80` — transports (in-memory, HTTP), `connect`, `forEachTransport`, `call` helpers; `:560-585` instruction tests.
- `internal/usergames/mcp.go:20-33` — `ResolveMCP` takes a place via `r.Use`.
- `cmd/tcgreplay/hosted.go:196` — hosted server built from `mcpserver.Server`.
- `cmd/tcgreplay/shim.go:26-56` — raw JSON-RPC relay.
- `cmd/tcgreplay/hosted_test.go:584-700,936-955` — hosted/desktop session helpers.
- `.dockerignore:1-11` — comment says tcg-* skills are embedded; excludes `skills/spek-*`.
- `Makefile:15-18` — `make install` is `go install` of the binary (not the install subcommand); unaffected.
- `TCG Review Playmat Design and Build Plan.md:298` — living design notes describe skills and `tcgreplay install`; needs updating to guides.
- `.mcp.json` — dev server registration; unaffected.

## External references

- go-sdk v1.6.0 source in the module cache (`mcp/server.go:202`, `mcp/prompt.go`, `mcp/protocol.go`) — the prompt API used for the prompts requirement.
- MCP specification, prompts (`prompts/list`, `prompts/get`) — prompts are user-selected starting instructions; client support varies, which is why the tool is the primary path.

## Prior plans / specs consulted

- Spec `20261020081616-39b3e658-guide-tool` — the source of truth for this plan.
- Spec/plan `20261019170829-bfb31628-turn-numbering-by-round` — introduced the turn-numbering checks in `skills/skills_test.go` (known from the test itself; plan not re-read).
- Knowledge `conventions/skills-text-and-packaging.md` — bans "Claude"/"Nic " in tcg skills and forbids excluding them from the build context; both rules carry over to guides, and the entry must be updated to say guides once the skills are gone.
- Knowledge `gotchas/go-sdk-sessions-and-auth.md` — sessions are bound to the token; state is looked up per request (relevant to why `guide` must not go through `bind`).

## Open assumptions

- Claude Code lists server prompts from a stdio server relayed by the shim and from the hosted HTTP server (expected from the MCP spec; verified only by the manual check).
- The six guides' bodies need only the wording changes listed in the tasks (skill → guide names, "skill" → "guide"); no other text references skills or client-specific tools.
- No client relies on the server not advertising the prompts capability.

## Drafting assumptions

### Chosen direction: embedded guides package served by mcpserver (architecture)
- **Decision**: `internal/guides` embeds `*.md` with name/description frontmatter; `internal/mcpserver` adds one `guide` tool (not bound to a game), one prompt per guide, and an instructions block generated from the guide list. Skills, `skills.go`, `skills_test.go` and the install command are deleted; `main.go` refuses unknown arguments.
- **Rationale**: One source drives tool, prompts and instructions; the single `mcp.Server` used by every mode makes hosted and desktop identical by construction; embed matches today's mechanism and the "ships with the playmat" constraint. Effort: Low–Medium.
- **Rejected**: Guides as MCP resources (patchy client support; spec asks for a tool); guide text as Go string constants (long prose is hard to edit and diff); separate list/read tools (spec asks for one tool).

### Tool-name check lives in mcpserver tests (architecture)
- **Decision**: The "names only tools the playmat has" check runs in `internal/mcpserver` against the live tool list and input properties, allowing also `tcgreplay_record`.
- **Rationale**: Needs the real tool list; putting it in `internal/guides` would require an import cycle or a hand-kept list.
- **Rejected**: A hard-coded tool list in the guides test.

### Conventions selected (architecture)
- **Decision**: Apply *Skill text and packaging* (word ban, never exclude from build context), retargeted at guides; plan updates the entry via spek-knowledge with user confirmation.
- **Rationale**: It is the only convention, and it governs exactly the text this change moves.
- **Rejected**: None dropped.

### guide tool bypasses per-account binding (discovery)
- **Decision**: Register `guide` with `mcp.AddTool` directly, not through `add`/`bind`.
- **Rationale**: Guides are static text; in hosted mode `bind` takes a place and can refuse at the user cap. Sign-in and rate limiting still apply at the HTTP layer.
- **Rejected**: Going through `add` (a guide read would occupy a place and fail when the playmat is busy).

### Short "use when" lines instead of full skill descriptions (discovery)
- **Decision**: Each guide carries a single-line `description` derived from its skill's "Use when …" sentence, checked to be one line and short.
- **Rationale**: Instructions are sent every turn; six 300–400 character descriptions would be ~2,000 characters.
- **Rejected**: Reusing the full skill descriptions verbatim.

### "skill" wording in guide prose rewritten (discovery)
- **Decision**: Rewrite "This skill …" to "This guide …" and "If another skill records results" to "If something else records results", and ban the word "skill" in guides by test.
- **Rationale**: Acceptance criterion "No guide mentions a skill"; the change is wording only.
- **Rejected**: Leaving generic uses of "skill" in place.

### guide tool result shapes (data_structures)
- **Decision**: List as a JSON array of {name, description}; a guide as plain text; unknown name as a tool error listing the valid names.
- **Rationale**: Matches the existing `out()` helper (strings as text, everything else as indented JSON) and the spec's error requirement.
- **Rejected**: Wrapping the guide text in JSON (escapes the markdown for no gain).

### Guide block placement in instructions (data_structures)
- **Decision**: Guide block goes after the existing bullets and before the hosted-only board-link line.
- **Rationale**: Keeps the desktop and hosted instructions identical up to the board line, so existing hosted tests that look for the board line still hold.
- **Rejected**: Putting the guides first (pushes the playmat basics down in every turn's context).

### Skills removal lands with the guides, not after (milestones)
- **Decision**: Milestone 1 moves the skill text to guides and removes the skills and the install command together.
- **Rationale**: The install command embeds the same files; moving them (to keep history) breaks the embed, so the swap is atomic. A copy-then-delete split would duplicate ~4,100 words and lose the move history.
- **Rejected**: Guides first with skills kept until a later milestone.

### Cross-reference phrasing "the `<name>` guide" (tasks)
- **Decision**: Guides refer to each other as "the `<name>` guide", and the test resolves every such phrase; `tcg-` is banned outright.
- **Rationale**: Backticked hyphenated words also include finding categories (`prize-trade`), so only the "guide" phrasing reliably marks a cross-reference.
- **Rejected**: Treating every backticked hyphenated word as a guide reference.

### Knowledge-convention update is an agent task with confirmation (tasks)
- **Decision**: Updating the convention is an `agent` task done through `spek-knowledge`'s propose-then-confirm flow.
- **Rationale**: The work is in-repo and needs no secrets; the user's confirmation happens inside the skill.
- **Rejected**: A `human` task (none of the human criteria apply).

## Rehydration cues

- `spektacular spec file read 20261020081616-39b3e658-guide-tool`
- `spektacular knowledge read --data '{"tier":"repo","name":"tcgreplay","path":"conventions/skills-text-and-packaging.md"}'`
- Re-read `internal/mcpserver/server.go:39-185`, `skills/skills_test.go`, `cmd/tcgreplay/main.go:60-80`, `cmd/tcgreplay/install.go`.
- `grep -n 'tcg-\|skill' skills/tcg-*/SKILL.md` for every cross-reference to rewrite.
