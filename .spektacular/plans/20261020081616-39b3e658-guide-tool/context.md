---
created_date: "2026-10-20"
document_status: final
closed_date: "2026-10-20"
---

# Context: 20261020081616-39b3e658-guide-tool

## Current State Analysis

- The six coaching workflows are Agent Skills at `skills/tcg-*/SKILL.md` (~4,100 words), embedded by `skills/skills.go:8-11` (`//go:embed tcg-*`) and used only by `cmd/tcgreplay/install.go`, which copies them into a client's skills folder and registers the server (`tcgreplay install -client claude|codex|gemini`).
- `skills/skills_test.go` enforces the skill format, bans "Claude"/"Nic ", bans removed tools and checks turn numbering. `skills/` also holds Spektacular's `spek-*` skills; `.claude/skills` and `.agents/skills` are symlinks to it.
- `cmd/tcgreplay/main.go:70-76` dispatches `install` before `flag.Parse()`; positional arguments are otherwise ignored.
- `internal/mcpserver/server.go:39-58` builds the one `mcp.Server` used by the desktop in-process and daemon modes (`cmd/tcgreplay/main.go` `newServer`) and hosted mode (`cmd/tcgreplay/hosted.go:196`). Instructions are a constant plus a hosted-only board-link line (`server.go:62-83`). Every tool goes through `add` (`server.go:171-185`), which binds the call to the caller's game; in hosted mode that takes a place (`internal/usergames/mcp.go:20-33`).
- The desktop shim (`cmd/tcgreplay/shim.go`) relays raw JSON-RPC to the daemon. The server registers no prompts today.
- `.dockerignore` keeps the tcg skills in the build context and excludes `skills/spek-*`. `TCG Review Playmat Design and Build Plan.md:298` documents the skills and `tcgreplay install`.

## Per-Task Technical Notes

### Task: Remove the install command

Requirement attribution: "The install command is removed" and "Skills are no longer shipped" (install half) → repo `tcgreplay`, `cmd/tcgreplay/`.

**File changes**:
- `cmd/tcgreplay/install.go:1-282` — delete the file (installer, targets, `copySkills`, `mergeJSON`, `codexTOML`, `claudeAdd`, `serverName`). It is the only importer of `github.com/nicholasjackson/tcgreplay/skills`.
- `cmd/tcgreplay/install_test.go:1-125` — delete. Its helpers `newInstaller` and `readFile` are used nowhere else (checked with grep).
- `cmd/tcgreplay/main.go:1-20` — package doc: drop the two lines about `tcgreplay install` (lines 4-5). The rest of the doc stays.
- `cmd/tcgreplay/main.go:70-76` — delete the `os.Args[1] == "install"` dispatch.
- `cmd/tcgreplay/main.go:96` (right after `flag.Parse()`) — if `flag.NArg() > 0`, print `unknown command "<first arg>"` to stderr followed by `flag.Usage()`, then `os.Exit(2)`. This has to happen before the log redirection and the mode switch, so nothing starts.
- `cmd/tcgreplay/main_test.go` (new) — add a generic re-exec helper modelled on `TestHostedMainHelper` (`cmd/tcgreplay/hosted_test.go:452-460`): `TestMainHelper` runs `main()` with `os.Args = ["tcgreplay"] + strings.Fields($HELPER_ARGS)` when `TCGREPLAY_MAIN_HELPER=1`. Tests: (1) `install` exits non-zero, and stderr contains `unknown command "install"`; (2) `install -client claude` behaves the same; (3) `-h` output (stderr) contains neither "install" nor "skill" (case-insensitive). Use a temp `-data` dir so a failed refusal could not touch the real data dir.

**Complexity**: Low
**Token estimate**: ~15k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Turn the coaching skills into guides

Requirement attribution: "The six coaching workflows are guides", "Guides refer to each other by guide name", "Guide text follows the existing conventions", "Skills are no longer shipped", and the "Conventions are enforced automatically" constraint → repo `tcgreplay`, `internal/guides/`, `skills/`.

**File changes**:
- `skills/tcg-import-game/SKILL.md` → `internal/guides/import-game.md` (use `git mv`). Frontmatter becomes `name: import-game` and `description: Use when the player pastes a TCG Live battle log or a saved game record, or asks to import, load or replay a game.` Body edits: line 14 "If another skill records results … This skill only adds the playmat game." → "If something else records results (a game ledger, a notes vault), let it do its part as usual. This guide only adds the playmat game."; line 26 "Follow the `tcg-opponent-deck` skill" → "Follow the `opponent-deck` guide"; line 56 "with the `tcg-review-game` skill" → "with the `review-game` guide".
- `skills/tcg-opponent-deck/SKILL.md` → `internal/guides/opponent-deck.md`. `description: Use right after importing a game, or when the opponent's unseen cards show as unknown, to build and apply the opponent's likely 60-card list.` Check the body for any skill wording (grep showed none outside the frontmatter).
- `skills/tcg-review-game/SKILL.md` → `internal/guides/review-game.md`. `description: Use when the player asks to review, go through, analyse or coach a game on the playmat, or accepts the offer of a review after importing.` Line 71: "Offer the `tcg-replay-drill` skill" → "Offer the `replay-drill` guide".
- `skills/tcg-replay-drill/SKILL.md` → `internal/guides/replay-drill.md`. `description: Use when the player asks for a drill, a puzzle, practice, or to replay their losses or mistakes from reviewed games.` Line 13: "Those come from `tcg-review-game`" → "Those come from the `review-game` guide"; line 15: "with `tcg-review-game`" → "with the `review-game` guide".
- `skills/tcg-before-you-move/SKILL.md` → `internal/guides/before-you-move.md`. `description: Use when the player asks to practise their pre-move checklist or turn routine, or to be quizzed on a position.` Line 8: "This skill practises asking them" → "This guide practises asking them".
- `skills/tcg-numbers-drill/SKILL.md` → `internal/guides/numbers-drill.md`. `description: Use when the player asks for a numbers drill, damage maths practice, or a quick quiz on the cards in the open game or the meta's threats.`
- Re-grep every moved file for `tcg-` and `skill` (case-insensitive) after the edits; there must be no hits outside the intentional text above.
- `internal/guides/guides.go` (new) — package doc: "Package guides holds the playmat's coaching workflows, served to connected assistants over MCP." `//go:embed *.md` into `files embed.FS`; `var order = []string{"import-game","opponent-deck","review-game","replay-drill","before-you-move","numbers-drill"}`; `type Guide struct{ Name, Description, Text string }`. Parse once (package-level `var all = mustLoad()`): split frontmatter with the same `---\n … \n---\n` cut that `skills/skills_test.go:25-29` uses, read `name` and `description`, and set `Text` to the body with leading newlines trimmed. Panic with the file name on a malformed file or a file missing from `order` (a build defect the package tests catch). `List() []Guide` returns a copy in `order`. `Get(name string) (Guide, error)` returns `fmt.Errorf("unknown guide %q; guides are %s", name, strings.Join(order, ", "))` for an unknown name.
- `internal/guides/guides_test.go` (new) — port from `skills/skills_test.go`: the format check (name equals the file stem, matches `^[a-z0-9]+(-[a-z0-9]+)*$`, ≤64 characters), the "Claude"/"Nic " ban, the removed-tools ban (`list_games`, `load_game`, `new_id`), `turnNumberingProblems` with `TestTurnNumberingCheckerCatchesOldText` unchanged, and the turn check across every guide (no `tcg-` prefix filter now). New tests: the description is non-empty, has no newline and is ≤200 characters; no guide contains "skill" (case-insensitive) or `tcg-`; every match of the regexp ``(?i)`([a-z0-9-]+)` guide`` names a guide in `order`; `List()` names exactly `order` and every embedded `.md` file is listed; `Get("review-game").Text` contains "decision points" and "Finding ("; `Get("tcg-review")` errors and the message contains all six names; and the anchor rules survive (`review-game` contains "Ask before judging" and "Put the board where you are talking"). Add one anchor phrase per other guide, taken from its current text during the move.
- `skills/skills.go:1-11` — delete.
- `skills/skills_test.go:1-151` — delete (its tests now live in `internal/guides/guides_test.go`).
- `skills/tcg-*` — gone after the `git mv`s; `skills/spek-*` untouched. The `.claude/skills` and `.agents/skills` symlinks still resolve to `skills/`.

**Complexity**: Medium
**Token estimate**: ~35k tokens
**Agent strategy**: 2 parallel agents for independent changes: one moves and edits the six text files, one writes `guides.go` and the ported tests. Then integrate sequentially and run `go test ./internal/guides/ ./cmd/...`.

### Task: Serve the guides through a guide tool and the connect instructions

Requirement attribution: "Assistants can list the guides", "Assistants can read a guide by name", "Connecting is enough to know the guides exist", "Short instructions" and the tool half of "Must work in every client" → repo `tcgreplay`, `internal/mcpserver/`.

**File changes**:
- `internal/mcpserver/guides.go` (new) — `type GuideArg struct{ Name string \`json:"name,omitempty" jsonschema:"guide to read; omit to list the guides"\` }`. `func (s *Server) registerGuides(srv *mcp.Server)` calls `mcp.AddTool(srv, &mcp.Tool{Name: "guide", Description: "List the playmat's coaching guides, or read one by name. Read a guide before starting its workflow."}, h)`, and h returns `out(list)` for an empty name (list = `[]{name, description}`) or `out(g.Text)` for a known name. An unknown name returns `&mcp.CallToolResult{IsError: true, Content: text(err.Error())}`. Add a short comment that the tool is registered outside `add` because it needs no game and must not take a hosted place (see `add`/`bind` at `internal/mcpserver/server.go:101-185`). Also add `func guideInstructions() string`, which builds "\n- Coaching workflows have guides. Before starting one, read its guide with the guide tool (pass its name):" followed by "\n  - <name>: <description>" for each guide in `guides.List()`.
- `internal/mcpserver/server.go:58` — call `s.registerGuides(srv)` after `s.register(srv)`.
- `internal/mcpserver/server.go:74-83` — `instructions()` returns `instructions + guideInstructions()` and appends `boardInstruction` when `BoardURL != ""`, so the guide block sits before the board line.
- `internal/mcpserver/server.go:1` — package doc "exposes the playmat to Claude" can stay (code comment, not guide text). Leave it.
- `internal/mcpserver/server_test.go:580` — `ins != instructions` → `ins != instructions+guideInstructions()`.
- `internal/mcpserver/guides_test.go` (new), using `forEachTransport` (`server_test.go:65-73`) and `call`:
  - `guide` with no args → JSON array of six `{name, description}` objects in `order`, each description non-empty and single-line.
  - `guide` `{name: "review-game"}` → text equals `guides.Get("review-game").Text` and contains "Finding (".
  - `guide` `{name: "tcg-review"}` → error result whose text contains all six names.
  - instructions contain the "read its guide" line and `"  - <name>: <description>"` for each guide; the whole guide block stays ≤ 7 lines.
  - Tool-name check: `cs.ListTools` → set of tool names ∪ every property key of every tool's input schema (decode `InputSchema` as `map[string]any` and read `properties`) ∪ `{"tcgreplay_record"}`. For each guide, every match of `\b[a-z]+(_[a-z]+)+\b` in `Text` must be in the set; report the guide and the word. Include a self-test showing the check flags `list_games` and accepts `import_log`.
  - `guide` works on a server with a `Resolve` that always errors (as in `TestResolvePerCall`, `server_test.go:589`), which proves it bypasses `bind`.

**Complexity**: Medium
**Token estimate**: ~30k tokens
**Agent strategy**: Single agent, sequential execution. The registration and the tests are tightly coupled.

### Task: Publish each guide as a server prompt

Requirement attribution: "Guides can be picked directly where clients allow it" and the "prompts are an extra" part of "Must work in every client" → repo `tcgreplay`, `internal/mcpserver/`.

**File changes**:
- `internal/mcpserver/guides.go` — in `registerGuides`, for each `g := range guides.List()`: `srv.AddPrompt(&mcp.Prompt{Name: g.Name, Description: g.Description}, func(context.Context, *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) { return &mcp.GetPromptResult{Description: g.Description, Messages: []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: g.Text}}}}, nil })`. go-sdk v1.6.0 API: `mcp/server.go:202`, `mcp/protocol.go:661,1012,1058`. Per-iteration `g` is safe (go.mod is Go 1.27.2).
- `internal/mcpserver/guides_test.go` — over `forEachTransport`: `cs.ListPrompts` returns exactly the six names in any order, each with the guide's description; for each, `cs.GetPrompt` returns one message, role `user`, whose `TextContent.Text` equals the `guide` tool's text for that name; `cs.InitializeResult().Capabilities.Prompts` is non-nil.
- Existing tool tests stay green unchanged, which shows the prompts add nothing to the tool list.

**Complexity**: Low
**Token estimate**: ~12k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Prove hosted and desktop serve the same guides

Requirement attribution: "Hosted and desktop serve the same guides" → repo `tcgreplay`, `cmd/tcgreplay/`.

**File changes**:
- `cmd/tcgreplay/guides_test.go` (new) — `TestHostedGuidesMatchDesktop`. Hosted session: `startHosted(t, "https://playmat.example").connect(t)` (`cmd/tcgreplay/hosted_test.go:584-668`). Desktop session: `newServer(config{dataDir: t.TempDir(), cacheDir: t.TempDir(), seat: "p2", offline: true}, "")` over in-memory transports, as in `TestDesktopServerHasNoBoardLinks` (`hosted_test.go:936-955`). Compare the `guide` list text, the `guide` text for every listed name, the `ListPrompts` names and descriptions, and the `GetPrompt` text for each. Instructions: the hosted ones must start with the desktop ones and differ only by the board-link line (assert that the extra suffix contains "board link" and no guide name).
- `cmd/tcgreplay/hosted_places_test.go:89-124` (`TestHostedFreedPlaceToldBusy`) — at the point where B's `get_state` is refused busy (around line 118-122), also call `guide` with no args and with `{name: "review-game"}` on B's session, and assert both succeed (not an error result).

**Complexity**: Low
**Token estimate**: ~15k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Update the build context and design notes for guides

Requirement attribution: the "Guides ship with the playmat" constraint (build context) and "Skills are no longer shipped" (docs) → repo `tcgreplay`, `.dockerignore` and the design notes.

**File changes**:
- `.dockerignore:1-2` — comment: "The guides and the UI placeholder are embedded, so they stay in."
- `.dockerignore:9` — `skills/spek-*` → `skills` (only Spektacular's skills remain there; `internal/guides` is not excluded).
- `TCG Review Playmat Design and Build Plan.md:298` — replace the "**Skills (any assistant).**" paragraph with a "**Guides (any assistant).**" paragraph. The coaching workflows are markdown in `internal/guides` (embedded): `import-game`, `opponent-deck`, `review-game` (keep the existing parenthetical summary), `replay-drill`, `before-you-move`, `numbers-drill`. They are written for any player and any assistant, with bare tool names. They reach every client through the `guide` tool (list, or read by name), one MCP prompt per guide, and a one-line-per-guide block in the server instructions; the text checks run in the guides' and the MCP server's tests. Clients register the playmat with their own tools (for example `claude mcp add`, or a claude.ai custom connector for hosted). Keep "The playmat labels the assistant's seat…". Drop "MCP prompts" from the "Later:" list, since this change ships them.
- Grep the design notes for any other `tcgreplay install` or `skills` mention and update it (the `make install` mentions refer to the Makefile target and stay).
- Verify with `make docker-build` (or `docker build .`) that the image builds.

**Complexity**: Low
**Token estimate**: ~10k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Update the skill text and packaging convention for guides

Requirement attribution: the knowledge convention `conventions/skills-text-and-packaging.md` (repo tier, store `tcgreplay`) → repo `tcgreplay` knowledge store.

**File changes**:
- Knowledge entry `conventions/skills-text-and-packaging.md` (tier `repo`, name `tcgreplay`) — update through the `spek-knowledge` skill, never by editing the file. Proposed content: title "Guide text and packaging"; rules: never write "Claude" or "Nic " in an `internal/guides` guide, because guides serve any assistant and any player and `internal/guides/guides_test.go` fails on either word; never exclude `internal/guides` from a build context (`.dockerignore` or other packaging), because `internal/guides/guides.go` embeds the guides with `//go:embed *.md` and the binary will not build without them. Tags: `[guides, testing, build, conventions]`. Whether to keep the path and only update the content, or move it to `conventions/guide-text-and-packaging.md` and delete the old entry, is the skill's propose-then-confirm decision with the user.
- Read the current entry first with `spektacular knowledge read --data '{"tier":"repo","name":"tcgreplay","path":"conventions/skills-text-and-packaging.md"}'`.

**Complexity**: Low
**Token estimate**: ~6k tokens
**Agent strategy**: Single agent, sequential execution; it stops for the user's confirmation inside `spek-knowledge`.

## Testing Strategy

All tests run in the existing `go test ./...` (`make test`). Per task:

- **Remove the install command** — process-level tests in a new `cmd/tcgreplay/main_test.go` re-exec the test binary to run `main()` with `install`, `install -client claude` and `-h`. The first two must exit non-zero with `unknown command`; `-h` must mention neither "install" nor "skill". The install tests are deleted with the code.
- **Turn the coaching skills into guides** — unit tests in `internal/guides/guides_test.go`: the ported format, word-ban, removed-tools and turn-numbering checks (with the checker self-test); new checks for no "skill"/`tcg-`, "the `<name>` guide" cross-references that resolve, single-line descriptions of at most 200 characters, `List()` order and coverage of every embedded file, `Get` text and the unknown-name error, and anchor phrases per guide (for example "Ask before judging" and "Put the board where you are talking" in review-game).
- **Serve the guides through a guide tool and the connect instructions** — contract tests in `internal/mcpserver/guides_test.go` over both transports: list, read, the unknown-name error, the instructions guide block, the tool-name check against the live tool list and input properties (with a self-test), and `guide` working when `Resolve` always fails. Update `TestNoBoardLinksOnDesktop` to expect `instructions + guideInstructions()`.
- **Publish each guide as a server prompt** — contract tests over both transports: six prompts with guide names and descriptions, and `GetPrompt` text equal to the `guide` text for each. The prompts capability is advertised.
- **Prove hosted and desktop serve the same guides** — integration test `TestHostedGuidesMatchDesktop` comparing a signed-in hosted session with a desktop `newServer` session; `TestHostedFreedPlaceToldBusy` is extended to show `guide` still works when game calls are refused busy.
- **Update the build context and design notes for guides** — verified by building the Docker image; no new tests.
- **Update the skill text and packaging convention for guides** — no tests; the user confirms the wording.

Success metrics and manual checks (carried into the implementation test plan):
- Fresh claude.ai conversation with only the playmat connected reads the guides and runs import and review unprompted — **Manual — captured in the implementation test plan.**
- A workflow change reaches every client on its next connection with zero re-installs — the mechanism is a behavioural test (served text equals the embedded guides; no install path remains). End-to-end confirmation on the deployed hosted playmat is **Manual — captured in the implementation test plan.**
- Side-by-side read of each old skill against its guide finds no dropped rule or step — **Manual — captured in the implementation test plan.**
- In Claude Code connected to the playmat, six guide prompts appear and the review prompt's text equals the review guide's — **Manual — captured in the implementation test plan.**

## Project References

- Spec: `20261020081616-39b3e658-guide-tool` (read with `spektacular spec file read`).
- Design documents: none referenced by the spec.
- Knowledge: `conventions/skills-text-and-packaging.md` (repo tier, store `tcgreplay`), `gotchas/go-sdk-sessions-and-auth.md`, `learnings/claude-ai-connector-oauth.md`.
- Repo root: `tcgreplay` at `/home/nicj/code/github.com/nicholasjackson/tcgreplay` (from `spektacular repo list`). Every task lands here.
- go-sdk v1.6.0 sources in the module cache (`mcp/server.go`, `mcp/prompt.go`, `mcp/protocol.go`).

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

Total for this plan is about 125k tokens across seven tasks. Only "Turn the coaching skills into guides" warrants parallel agents (text moves vs. Go code and tests).

## Migration Notes

- Use `git mv` for each `skills/tcg-<x>/SKILL.md` → `internal/guides/<x>.md`, so history follows the text.
- Players who installed skills with `tcgreplay install`, or uploaded them to claude.ai, keep those copies until they delete them by hand. The playmat never touches them. Stale copies may still trigger in a client, but the guides announced in the instructions are current. Mention this in release notes if any are written.
- Players who registered the server with `tcgreplay install` keep that registration; it still points at the binary and keeps working.

## Performance Considerations

- The instructions grow by about seven short lines (~1 KB), sent every turn by clients. Each description is held to one line of at most 200 characters to bound this.
- Guide text (~4,100 words in total) is parsed once at start-up from embedded files and sent only when asked for.
