---
created_date: "2026-10-20"
document_status: final
closed_date: "2026-10-20"
---

# Plan: 20261020081616-39b3e658-guide-tool

<!-- Metadata -->
<!-- Created: 2026-10-20T09:09:35Z -->
<!-- Commit: dd624cb -->
<!-- Branch: main -->
<!-- Repository: git@github.com:nicholasjackson/tcgreplay.git -->

## Overview

The playmat will serve its six coaching workflows (import a game, build the opponent's likely deck, review a game, replay drills, the pre-move checklist and numbers drills) as guides compiled into the binary. Any connected assistant can reach them through one `guide` tool, through server prompts in clients that show them, and through a short list in the instructions sent on connect. Today the workflows are skills that each player has to install or upload by hand, and those copies go stale when the playmat changes. With guides, every player gets the current coaching on every client, hosted or desktop, just by connecting. The skills and the `install` command are removed.

## Conventions

- **Skill text and packaging: never write "Claude" or "Nic " in coaching text; never exclude the embedded coaching text from the build context** — the coaching text moves from `skills/tcg-*` to `internal/guides/*.md`. The word ban carries over as a guides test, `internal/guides` must stay in the Docker build context, and the entry itself is updated (with the user's confirmation, through `spek-knowledge`) to name the guides now that `skills/tcg-*` no longer exists.

## Architecture & Design Decisions

The six coaching workflows move from `skills/tcg-*/SKILL.md` to markdown files in a new package, `internal/guides`, compiled into the binary with `//go:embed *.md`, the same mechanism `skills/skills.go` uses today. Each file is named after its guide (`review-game.md`, …) and starts with a two-field frontmatter: `name` (matching the file name) and `description`, the guide's one-line "use when". The package parses them once and offers `List()` (name and description, in a fixed order) and `Get(name)` (the body without frontmatter, or an error listing the valid names). The text is carried over from the skills with only the wording changes needed: skill names become guide names (`tcg-review-game` → `review-game`), "this skill" becomes "this guide", and each skill's long description is cut down to a short single-line "use when". Nothing is fetched at runtime, so a guide always matches the tools of the build it ships in.

`internal/mcpserver` is the single place the guides reach an assistant, which is why hosted and desktop serve identical guides: `Server.MCP()` builds the one `mcp.Server` that the desktop in-process and daemon modes (`newServer`) and hosted mode (`hosted.go:196`) all use, and the desktop shim relays JSON-RPC unchanged, so it passes prompt requests through as well. It gains three things. (1) One `guide` tool with an optional `name` argument: no name returns the list, and a name returns that guide's text. It is registered with `mcp.AddTool` directly rather than through `add`, because `add` binds every call to the caller's game, and in hosted mode that takes one of the limited places and can refuse with "busy". Reading static text should never cost a place or fail at the cap, and sign-in and the rate limit still apply at the HTTP layer. An unknown name is a tool error naming the six valid guides. (2) One MCP prompt per guide, with the guide's name and description, whose single user message is exactly the text `guide` returns. (3) A short block appended to the server instructions, generated from `List()` so it cannot drift: one line saying to read a guide with `guide` before starting that workflow, then one line per guide, `<name>: <use when>`. The guide list is the source for all three, so a seventh guide would appear everywhere by adding one file.

The skills delivery goes away entirely. `skills/skills.go`, `skills/skills_test.go` and `skills/tcg-*` are deleted. `skills/` keeps only Spektacular's `spek-*` skills, so the `.claude/skills` and `.agents/skills` links keep working for this repo's own tooling. `cmd/tcgreplay/install.go` and its test are deleted. `main.go` loses the `install` dispatch and the doc lines about it, and it now refuses any leftover command-line argument with "unknown command". Without that refusal, `tcgreplay install` would fall through to starting the shim instead of failing.

The text checks move with the text. `internal/guides` tests the format (name matches the file, a single-line description of at most 200 characters), the bans on "Claude", "Nic " and "skill", the removed tools, and turn numbering (the `turnNumberingProblems` checker and its self-test move over unchanged), plus that every backtick-quoted guide-like name refers to a guide that exists. The check that guides name only tools the playmat has needs the real tool list, so it lives in `internal/mcpserver`: it lists the server's tools and their input properties over an in-memory session and fails on any snake_case word in a guide that is neither of those nor the record marker `tcgreplay_record`. This keeps the guides package free of an import cycle and the tool list never hand-maintained. Following the convention *Skill text and packaging*, the "Claude"/"Nic " ban carries over to the guides, and `internal/guides` is never excluded from the build context. Serving the guides as MCP resources, keeping the text as Go constants, and splitting the tool into list and read tools were considered and rejected; see `research.md#alternatives-considered-and-rejected`.

## Component Breakdown

- **Guides (new)** — owns the six coaching workflows as text compiled into the binary, one document per guide with a name and a one-line "use when", and the only API for reaching them: list the guides in a fixed order, and read one by name (an unknown name is an error that lists the valid names). It knows nothing about MCP. Its own tests enforce the text conventions that need no server: the format, the word bans ("Claude", "Nic ", "skill"), the removed tools, turn naming and seat-qualified branch names, and that cross-references name guides that exist. It replaces the coaching half of today's skills package.
- **MCP server (changed)** — the single adapter between the guides and every connected assistant, in every mode. It registers the `guide` tool outside the per-account game binding, publishes one prompt per guide with the same text, and appends a guide list, generated from the Guides component, to the instructions it sends on connect. Its tests also own the one guide check that needs the server: every tool-like name in a guide is a real tool or tool argument.
- **Command entry point (changed)** — loses the install command and its help and doc text, and refuses any command-line argument it does not understand, so a removed command fails loudly instead of starting the server.
- **Install command (removed)** — the skill copier and client registration for Claude, Codex and Gemini is deleted with its tests. Players register the playmat with their client's own tools.
- **Skills package (shrunk)** — loses its Go embedding, its tests and the six coaching skills. What remains is the folder of Spektacular's own skills, still linked from this repo's `.claude/skills` and `.agents/skills`.
- **Build context and project docs (changed)** — the Docker ignore rules and their comment stop describing embedded skills and exclude the remaining skills folder whole, without excluding the guides. The living design notes and the *Skill text and packaging* knowledge convention describe guides instead of skills and the install command.

## Data Structures & Interfaces

**Guide** (Guides component) — one coaching workflow. `Name` is the identifier assistants use (`review-game`); `Description` is the single-line "use when"; `Text` is the full guide body as returned to the assistant (the file without its frontmatter).

```go
type Guide struct {
    Name        string // e.g. "review-game"
    Description string // one line: when to use it
    Text        string // full body, no frontmatter
}

func List() []Guide                 // all guides, fixed order
func Get(name string) (Guide, error) // unknown name: error listing the valid names
```

The fixed order is the workflow order players meet: `import-game`, `opponent-deck`, `review-game`, `replay-drill`, `before-you-move`, `numbers-drill`. `List` and `Get` read from one parse of the embedded files, so the tool, prompts and instructions all see the same values.

**Guide file format** (embedded source) — markdown with a two-field frontmatter, the same shape the skills used, minus everything skill-specific:

```markdown
---
name: review-game
description: Use when the player asks to review, go through, analyse or coach a game on the playmat.
---

# Review a game
…
```

**`guide` tool** (MCP contract) — input `{ "name"?: string }`.
- Without `name`: result text is a JSON array of `{ "name", "description" }`, one per guide, in the fixed order.
- With a known `name`: result text is the guide's `Text`, as plain text.
- With an unknown `name`: a tool error, `unknown guide "tcg-review"; guides are import-game, opponent-deck, review-game, replay-drill, before-you-move, numbers-drill`.

**Guide prompts** (MCP contract) — one prompt per guide: prompt `name` = guide name, `description` = guide description, no arguments. `prompts/get` returns one `user` message whose text content is exactly the guide's `Text`, so choosing a prompt gives the same text as calling `guide`.

**Instructions guide block** (connect-time contract) — appended to the existing instructions in both desktop and hosted mode (before the hosted board-link line), generated from `List()`:

```
- Coaching workflows have guides. Before starting one, read its guide with the guide tool (pass its name):
  - import-game: <description>
  - …one line per guide…
```

No other types change. The removed install command's types (`installer`, `installTarget`) go with it.

## Implementation Detail

**Patterns followed.** The guides reuse the embed-and-parse pattern the skills used: markdown files with frontmatter next to a tiny Go file that embeds them. Their tests are the skills' tests ported almost line for line, so the turn-numbering checker and its self-test move without change in behaviour. Server-side, the `guide` tool, prompts and instructions sit beside the existing registration code in the MCP server and use its existing result helper. Server tests use the existing transport matrix (in-memory and Streamable HTTP), so every guide behaviour is checked over both connection kinds.

**New pattern: tools that need no game.** Until now every tool went through the one registration helper that binds each call to the caller's game. `guide` is the first tool registered beside that helper rather than through it, because it reads only compiled-in text. The code says so in a short comment where it is registered, so the next static tool (if any) follows the same route instead of taking a hosted place.

**New pattern: instructions generated from data.** The connect-time instructions stop being a single constant: the guide block is built from the guide list, so adding or renaming a guide changes the tool, the prompts and the instructions together. The existing constant stays for the playmat basics. Tests that compared the instructions with that constant compare with the full generated text instead.

**Code shape after the change.** The coaching text lives under `internal/` next to the server that serves it, and the top-level skills folder is Spektacular's only. The command entry point loses its subcommand dispatch altogether: the binary has one way to run, selected by flags, and anything else on the command line is an error. A developer looking for "where does the assistant learn the workflows" finds one package and one block of registration code. Nothing writes to a player's machine anymore.

**Text migration is mechanical.** Each skill file is moved and renamed to its guide name, its frontmatter is cut to the short "use when" line, and the body is edited only where it names a skill or calls itself one. Moving the files with git keeps their history traceable from skill to guide.

## Dependencies

- **Design documents this plan was built on: none.** The spec carries no design references.
- **`github.com/modelcontextprotocol/go-sdk` v1.6.0** (existing) — provides tool registration, server prompts (`AddPrompt`, `GetPromptResult`) and the instructions sent on connect. No version change is needed: prompts are already supported.
- **Go `embed`** (standard library, existing) — compiles the guide text into the binary, as it does the skills today.
- **MCP server package** (internal, existing) — gains the `guide` tool, prompts and the instructions block; it is the only consumer of the new guides package.
- **Hosted per-account registry** (internal, existing, unchanged) — `guide` deliberately does not go through its game binding; nothing in it changes.
- **Desktop stdio shim** (internal, existing, unchanged) — relays JSON-RPC verbatim, so prompts reach the daemon with no change.
- **Spektacular skills in `skills/spek-*`** (existing, unchanged) — must keep working through the `.claude/skills` and `.agents/skills` links; this plan only removes the `tcg-*` folders and the Go files beside them.
- **Knowledge convention *Skill text and packaging*** (repo knowledge) — updated to describe guides; the update goes through `spek-knowledge` with the user's confirmation.
- **Prior plan `20261019170829-bfb31628-turn-numbering-by-round`** (landed) — introduced the turn-numbering text checks this plan moves to the guides; nothing further must land first.
- No new external services or libraries.

## Testing Approach

**Kinds of tests.** Unit tests on the guides package (parsing, ordering, lookup, text conventions); contract tests on the MCP server over both existing transports (in-memory and Streamable HTTP) for the `guide` tool, prompts and instructions; an integration test that compares a signed-in hosted session with a desktop session; and a process-level test of the command line using the existing re-exec helper pattern. All of them run with the existing test suite, so the conventions are enforced before release.

**Where coverage concentrates.** The text checks carry the most weight, because the guides are the product here and they are prose that drifts. The ported checks keep today's guarantees: the format, no "Claude" or "Nic ", no removed tools, turns named by the combined number, and branch names that carry the seat. The checker's own self-test is kept so the regexes stay honest. New checks guarantee: no guide says "skill"; every backtick-quoted guide-shaped name a guide mentions is a guide that exists; each description is a single short line; and every snake_case identifier in a guide is a real tool, a real tool argument, or the record marker. That last check reads the live server's tool list, so a tool rename that leaves a guide stale fails the build.

**Load-bearing assertions.**
- Listing returns exactly `import-game`, `opponent-deck`, `review-game`, `replay-drill`, `before-you-move` and `numbers-drill` in that order, each with a non-empty one-line description.
- Reading `review-game` returns its full text: it contains how decision points are found and the `Finding (` recording format. Reading `tcg-review` fails with an error that names all six guides.
- Each guide keeps the anchor rules of the skill it came from. For example, review-game still contains "Ask before judging", "Put the board where you are talking" and the timeline findings format, and the import, drill and before-you-move guides keep theirs. This is a regression guard against a dropped step. It does not replace reading them side by side.
- The instructions on connect, desktop and hosted alike, contain every guide name with its description and the "read its guide before starting" line, and the hosted version still carries the board-link line.
- `prompts/list` offers six prompts named after the guides, and `prompts/get` for each returns exactly the text `guide` returns for it.
- A signed-in hosted session and a desktop session return identical guide lists, guide texts and prompts.
- `guide` works for a signed-in hosted account even when every place is taken (it does not bind to a game).
- `tcgreplay install` (and any other stray argument) exits non-zero with "unknown command", and `tcgreplay -h` mentions neither "install" nor "skill".

**Success metrics.**
- *A fresh claude.ai conversation with only the playmat connected reads the matching guides and runs import and review without being pointed at them* — **Manual — captured in the implementation test plan.** It depends on a live model's behaviour in claude.ai. The automated instructions and tool tests guarantee the preconditions (the guides are announced on connect and readable).
- *A playmat update that changes a workflow reaches every client on its next connection, with zero re-installs or re-uploads* — **Behavioural test** for the mechanism: the tests assert that what the tool, prompts and instructions serve is exactly the guide text embedded in the build, and the install command and every skill-copying path are gone, so there is nothing left to re-install. Confirming it end to end on the deployed hosted playmat after a release is **Manual — captured in the implementation test plan.**

**Manual reviews.**
- **Manual — captured in the implementation test plan:** a side-by-side read of each old skill and its guide confirms no rule or step was dropped (acceptance criterion "Same workflows as before").
- **Manual — captured in the implementation test plan:** in Claude Code connected to the playmat, the six guides appear as prompts, and choosing the review prompt gives the review guide's text.

**Deliberate gaps.** No end-to-end test drives a real claude.ai or Claude Code client: the MCP contract tests cover the protocol, and the client behaviour is checked by hand as above. Tests for the deleted install command are deleted with it rather than rewritten.

## Milestones & Tasks

### Milestone 1: Assistants learn the coaching workflows from the playmat, not from skills

**What changes**: Any assistant connected to the playmat, hosted or desktop, is told on connect that six guides exist, with one line each on when to use them, and can list them and read any one in full with a single `guide` tool. Nothing needs to be installed or uploaded. The guides carry the same workflows the skills did, renamed (`import-game`, `opponent-deck`, `review-game`, `replay-drill`, `before-you-move`, `numbers-drill`) and pointing at each other by guide name. At the same time the old delivery disappears: the six tcg skills are no longer in the binary or the repo, and the `install` command is gone. Running it now fails as an unknown command, and the help no longer mentions install or skills. Spektacular's own skills keep working in this repo. The text conventions that guarded the skills now guard the guides, along with the new checks (no "skill", cross-references resolve, only real tools named).

**Validation point**: The full test suite passes, including the ported and new guide text checks. A connected test session lists six guides, reads `review-game`, gets an error naming the six guides for `tcg-review`, and sees all six in the instructions. `tcgreplay install` exits with "unknown command".

#### - [ ] Task: Remove the install command
**Id:** 816f2137-df9c-42f1-9aaf-e4bae0afc0cd
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Delete the `install` subcommand, which copied the coaching skills into a client's folder and registered the server, along with its tests. The binary then has a single way to run, chosen by flags. It must reject any leftover word on the command line, because otherwise `tcgreplay install` would quietly start the server instead of failing. The command's doc comment and help stop mentioning install and skills.

*Technical detail:* [context.md#task-remove-the-install-command](./context.md#task-remove-the-install-command)

**Acceptance criteria**:
- [ ] Running the playmat with `install`, or any other unknown word, fails with an "unknown command" message and a non-zero exit, and starts nothing.
- [ ] The playmat's help output mentions neither install nor skills.
- [ ] No code path in the binary copies skill files or edits a client's configuration.

#### - [ ] Task: Turn the coaching skills into guides
**Id:** 037320c2-789b-47ff-8482-efac6dff36e3
**Repo:** tcgreplay
**Depends on:**
- 816f2137-df9c-42f1-9aaf-e4bae0afc0cd — Remove the install command
**Execution:** agent

Move the six coaching skills into a new guides package compiled into the binary, one file per guide, named `import-game`, `opponent-deck`, `review-game`, `replay-drill`, `before-you-move` and `numbers-drill`. Each keeps its body text, with only the wording changes needed: skill names become guide names ("the `review-game` guide"), "this skill" becomes "this guide", and a short single-line "use when" replaces the long skill description. The package lists the guides in a fixed order and returns one by name, or an error naming the valid guides. The skills' text checks move with the text, and new checks are added for no "skill", cross-references that resolve, and short one-line descriptions. The old skills package's Go files go too, leaving the skills folder holding only Spektacular's own skills.

*Technical detail:* [context.md#task-turn-the-coaching-skills-into-guides](./context.md#task-turn-the-coaching-skills-into-guides)

**Acceptance criteria**:
- [ ] There are exactly six guides, with the six agreed names, each with a one-line "use when" and the full workflow text of the skill it came from.
- [ ] Asking the guides package for an unknown name gives an error that lists all six valid names.
- [ ] Every guide that sends the assistant to another workflow names it by a guide name that exists, and no guide mentions a skill.
- [ ] No guide contains "Claude" or "Nic ", tells the assistant to translate turn numbers, or gives a turn branch name without the seat.
- [ ] The repo no longer holds any tcg skill, and Spektacular's skills are still reachable through this repo's `.claude/skills` and `.agents/skills` links.

#### - [ ] Task: Serve the guides through a guide tool and the connect instructions
**Id:** 1bc3d818-e3fd-4ee2-912b-36feab56e842
**Repo:** tcgreplay
**Depends on:**
- 037320c2-789b-47ff-8482-efac6dff36e3 — Turn the coaching skills into guides
**Execution:** agent

Add one `guide` tool to the MCP server: with no name it lists the guides with their "use when" lines, and with a name it returns that guide's full text. It is registered beside the per-account game binding, not through it, so reading a guide never takes a hosted place or fails because the playmat is busy. The instructions every assistant receives on connect gain a short block, generated from the guide list, naming each guide with its "use when" and saying to read the guide before starting that workflow. A server-side test also checks that every tool or argument name a guide mentions really exists on the playmat.

*Technical detail:* [context.md#task-serve-the-guides-through-a-guide-tool-and-the-connect-instructions](./context.md#task-serve-the-guides-through-a-guide-tool-and-the-connect-instructions)

**Acceptance criteria**:
- [ ] A freshly connected assistant that calls `guide` with no name gets six entries, each with a name and a one-line "use when", in the agreed order.
- [ ] Calling `guide` for `review-game` returns its full text, including how to find decision points and how to record findings; calling it for `tcg-review` returns an error that lists the six valid names.
- [ ] The instructions a new connection receives, on desktop and hosted, list all six guides with their "use when" lines and tell the assistant to read a guide before starting that workflow, while staying about one line per guide.
- [ ] A guide that names a tool or argument the playmat does not have fails the test suite.

### Milestone 2: Guides can be picked directly as prompts, identically on hosted and desktop

**What changes**: In clients that offer a server's starting instructions as commands, such as Claude Code, each of the six guides appears as a prompt. Choosing one gives exactly the text the `guide` tool returns. The hosted and desktop playmats are proven to serve the same guides, texts and prompts, and a hosted account can read guides even when every place on the playmat is taken.

**Validation point**: The prompt contract tests pass over both transports, and the hosted-vs-desktop comparison test passes. In Claude Code connected to the dev playmat, the six prompts are listed and the review prompt's text matches the review guide (manual check in the implementation test plan).

#### - [ ] Task: Publish each guide as a server prompt
**Id:** 7c8c9968-624f-4ac1-bf68-6787f81f813d
**Repo:** tcgreplay
**Depends on:**
- 1bc3d818-e3fd-4ee2-912b-36feab56e842 — Serve the guides through a guide tool and the connect instructions
**Execution:** agent

Register one MCP prompt per guide, named after the guide and described by its "use when" line, so clients that offer server prompts as commands, such as Claude Code, can start a workflow directly. Choosing a prompt returns a single message holding exactly the text the `guide` tool returns for that guide. The tool remains the main path; prompts are an extra for clients that support them.

*Technical detail:* [context.md#task-publish-each-guide-as-a-server-prompt](./context.md#task-publish-each-guide-as-a-server-prompt)

**Acceptance criteria**:
- [ ] A connected client listing prompts sees six, named after the guides, each described by its "use when" line.
- [ ] Choosing any prompt gives exactly the same text as reading that guide with the `guide` tool.
- [ ] Clients that ignore prompts behave as before: tools and instructions are unchanged by the prompts' presence.

#### - [ ] Task: Prove hosted and desktop serve the same guides
**Id:** e1cf543a-876a-4c6b-8885-c416ef9604d2
**Repo:** tcgreplay
**Depends on:**
- 7c8c9968-624f-4ac1-bf68-6787f81f813d — Publish each guide as a server prompt
**Execution:** agent

Add tests that connect to a signed-in hosted playmat and to a desktop playmat from the same build, and compare the guide list, every guide's text, the prompts and the guide block of the instructions. Extend the existing "freed place is told busy" scenario so that an account refused game access because the playmat is full can still read guides.

*Technical detail:* [context.md#task-prove-hosted-and-desktop-serve-the-same-guides](./context.md#task-prove-hosted-and-desktop-serve-the-same-guides)

**Acceptance criteria**:
- [ ] Listing the guides, reading each one and listing and choosing each prompt give identical results on the hosted and the desktop playmat.
- [ ] The hosted instructions carry the same guide block as the desktop instructions, plus only the hosted board-link line.
- [ ] A signed-in hosted account whose game calls are refused as busy can still list and read guides.

### Milestone 3: Project docs and packaging describe guides instead of skills

**What changes**: This is an internal cleanup with no change in behaviour. The Docker build rules, the living design notes and the project's *Skill text and packaging* convention stop describing embedded skills and the install command, and describe the guides instead. It is worth its own milestone because the convention is binding for future work, and leaving it naming a folder that no longer exists would mislead the next plan.

**Validation point**: The Docker image still builds, with the guides compiled in. The design notes and the updated knowledge convention name guides and no longer mention `tcgreplay install` or tcg skills.

#### - [ ] Task: Update the build context and design notes for guides
**Id:** 0ef9f2ba-7cde-4625-8020-a338d2aefdb7
**Repo:** tcgreplay
**Depends on:**
- 037320c2-789b-47ff-8482-efac6dff36e3 — Turn the coaching skills into guides
**Execution:** agent

The Docker ignore rules say embedded skills must stay in the image build. Now that only Spektacular's skills remain in that folder, the whole folder can be excluded, as long as the guides stay in, and the comment should say guides. The living design notes describe the skills and `tcgreplay install`; rewrite that paragraph to describe the guides, the `guide` tool, the prompts and the connect instructions, and say that clients register the playmat with their own tools.

*Technical detail:* [context.md#task-update-the-build-context-and-design-notes-for-guides](./context.md#task-update-the-build-context-and-design-notes-for-guides)

**Acceptance criteria**:
- [ ] The container image still builds with the guides compiled in, and no longer receives the skills folder.
- [ ] The design notes describe the guides and how they reach an assistant, and no longer mention `tcgreplay install` or tcg skills.

#### - [ ] Task: Update the skill text and packaging convention for guides
**Id:** a253ae02-d25c-4cc5-b449-d281a76eea2a
**Repo:** tcgreplay
**Depends on:**
- 037320c2-789b-47ff-8482-efac6dff36e3 — Turn the coaching skills into guides
**Execution:** agent

The project convention *Skill text and packaging* tells future work never to write "Claude" or "Nic " in tcg skills and never to exclude them from the build context. Both rules still hold, but for the guides, which now live elsewhere. Propose the rewritten entry to the user through the knowledge workflow and write it once they confirm, so the binding convention names the files that exist.

*Technical detail:* [context.md#task-update-the-skill-text-and-packaging-convention-for-guides](./context.md#task-update-the-skill-text-and-packaging-convention-for-guides)

**Acceptance criteria**:
- [ ] The convention names the guides and the test that enforces it, and no longer refers to tcg skills or their embedding.
- [ ] The user confirmed the new wording before it was written.

## Open Questions

- **Does the tool-name check flag a snake_case word in a guide that is neither a tool nor a tool argument nor `tcgreplay_record`?** Today's skill text has none (every snake_case word was checked during discovery), but the check reads the live tool schemas, and only running it shows whether any input property is named differently on the wire than in the text. *Depends on:* the input-schema property names go-sdk derives from the `json` tags. *When hit:* if the word is a real playmat term (a result field such as a report key), add it to the check's small allowlist with a comment saying what it is. If it names something the playmat does not have, fix the guide text. If fixing it would change a workflow's substance, STOP and ask the user.

## Out of Scope

- **Rewriting, merging or splitting the six workflows** (spec Non-Goal). The text moves with wording changes only.
- **Showing guides to players on the playmat**, for example a help page (spec Non-Goal). Guides are for the connected assistant.
- **Pointing the assistant at the next guide from inside tool results**, for example the import result suggesting a review (spec Non-Goal).
- **Removing skill copies already on players' machines or uploaded to claude.ai** (spec constraint). Players delete them by hand, and nothing in the playmat touches them.
- **Spektacular's own `spek-*` skills and the `.claude/skills` / `.agents/skills` links.** They are untouched and keep working.
- **Other mentions of "Claude" in code comments, flag help (`-seat` "seat Claude plays") and the actor name.** They are not guide text, and the spec's naming convention covers guides only.
- **The Makefile's `install` target.** It installs the binary itself, is not the removed subcommand, and stays.
- **Serving guides as MCP resources.** The spec asks for a tool plus prompts; resources were considered and rejected (see research).

