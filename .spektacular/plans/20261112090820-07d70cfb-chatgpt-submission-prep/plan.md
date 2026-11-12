---
created_date: "2026-11-12"
document_status: final
closed_date: "2026-11-12"
---

# Plan: 20261112090820-07d70cfb-chatgpt-submission-prep

<!-- Metadata -->
<!-- Created: 2026-11-12T10:16:32Z -->
<!-- Commit: 6ecc1aa -->
<!-- Branch: main -->
<!-- Repository: tcgreplay (/home/nicj/code/github.com/nicholasjackson/tcgreplay) -->

## Overview

TCG Replay is being submitted to ChatGPT's Plugin Directory so ChatGPT users can find it and connect without setting up a custom connection. This plan makes the playmat's connection meet OpenAI's review rules: every tool titled, described and explicitly labelled, instructions that stand alone in their first 512 characters, waits inside the time limit, and a profile tool that identifies the signed-in player. It also adds what the submission needs: domain verification, a reviewer account that always gets in, a reproducible plugin package carrying the shared fan-project notice and a trademark-free icon, and the test cases, release notes and demo script, while Claude's connections keep working unchanged.

## Conventions

- **Never write "Claude" or "Nic " in an `internal/guides` guide; never exclude `internal/guides` from a build context** — the description pass points tools at the guides but changes no guide text, and the new `plugin/` folder and builder leave the Dockerfile and `.dockerignore` untouched, so the guides stay in the build. If an implementer finds a guide must change (for example a guide naming a tool by a description phrase), the guide test's word check applies.

## Architecture & Design Decisions

All of the work lands in the one registered repo, `tcgreplay` (root `/home/nicj/code/github.com/nicholasjackson/tcgreplay`), in four parts: the playmat's MCP tool server (`internal/mcpserver`), the hosted playmat's sign-in, user cap and routes (`internal/hostauth`, `internal/usergames`, `cmd/tcgreplay/hosted.go`, `.do/app.yaml`), a new plugin package builder (`internal/plugin`, `cmd/tcgreplay-plugin`, and a `plugin` target in the Makefile), and a new `plugin/` folder holding the listing source, the icon and the review material. Claude's connections are untouched: sign-in stays the existing Google flow with Dynamic Client Registration, which already accepts ChatGPT's callbacks, and every change to the tool list is additive or descriptive, so claude.ai and Claude Code see the same tools, arguments and results as before.

**Tool metadata is fixed where every tool is registered, and guarded by tests.** Every tool but `guide` is registered through one helper (`addWith`), and `guide` sits beside it, so each tool's title and its "when to use it, when not to" wording are added there, and the missing parameter descriptions are added to the shared argument types. The go-sdk v1.6.0 type cannot put `readOnlyHint: false` on the wire, and read-only tools carry no destructive flag. So the tool server gets its own `tools/list` receiving middleware. It hands back the SDK's own result wrapped in a type whose JSON encoding writes all three labels on every tool, as explicit `true` or `false`, from the existing read-only and game-replacing sets. It works on the desktop's in-memory transport and on hosted Streamable HTTP alike, and the mirrored "moved" server the domain move builds from the same definition inherits it. The labels keep their present meaning: read-only for the reads, destructive only for the three tools that replace the player's game (`new_game`, `import_log`, `set_opponent_deck`), and closed-world everywhere. A test reads the raw `tools/list` JSON, which the Go client cannot do because it drops an omitted `false`. It fails if any tool lacks a label, a title, a "Use this when" and a "Do not use" sentence, or a description on any parameter, so a new tool cannot slip through. Another test reports the total size of the tool list and fails above a ceiling set from the measurement after the description pass (26,149 bytes today). In hosted mode each tool also declares `securitySchemes: [{type: oauth2}]` in its `_meta`, as OpenAI asks.

**What an assistant reads first, and how long it waits.** The server instructions are reordered so their first 512 bytes stand alone. In complete sentences, they say what TCG Replay is, that cards are referred to by their instance IDs (iids) and never by name, and that coaching workflows have guides read through the `guide` tool. The rest follows unchanged, including the per-guide list. Waiting already fits ChatGPT's limit: each `wait_for_turn` call blocks at most the wait cap, 15 s by default, and start-up refuses any cap over 50 s. Every `still_waiting` result already says to call again. The plan adds a test pinning both, rather than changing behaviour.

**ChatGPT's identity, domain check and reviewer.** A new hosted-only tool, `get_account`, is OpenAI's profile tool. It is read-only and takes no arguments, and it is marked `_meta["openai/profile"]: true` with an output schema that allows only `id`. It returns the signed-in player's Google account ID (`sub`), which never changes for that account. It is registered beside `guide`, outside the per-game binding, so asking who is signed in never takes a place or counts as activity. It is new rather than a rename because `get_profile` is already the deck-profile tool, which the guides use. The privacy page gains one sentence saying the assistant is given this ID to tell accounts apart. A new open route, `GET /.well-known/openai-apps-challenge`, returns exactly the token in `TCGREPLAY_OPENAI_CHALLENGE` as plain text, with no sign-in, or 404 when unset. A new setting, `TCGREPLAY_REVIEWER`, names the reviewer's Google email. The per-account registry admits that account even when every place is held, and gives it a place outside the count, so the limit stays exactly as it is for everyone else. The match is the admin list's: the email compared without regard to case, and only when Google marked it verified. For tool calls to match too, the verified flag is sealed into access and refresh tokens and returned with the account. A token issued before the change reads as unverified, so the reviewer reconnects once. Both settings are declared in `.do/app.yaml` as secrets with no value, so the maintainer sets them in the App Platform console without a release. Neither the token nor the email is ever logged. The start-up log only says whether each is set, following the observability rule that no log line carries an email.

**The plugin package is built from sources the project already owns.** The Makefile's new `plugin` target runs a small Go builder that writes `bin/tcgreplay-plugin.zip` containing `plugin.json`, `mcp.json` and the icon. It reads:
- the public address from `TCGREPLAY_PUBLIC_URL` in `.do/app.yaml` (the one address; the builder fails unless it is a bare https origin), giving the MCP URL `<address>/mcp` and the website, `/privacy` and `/terms` links;
- the fan-project notice, card-data credit and support link (GitHub issues) from `internal/notice/notice.json`, appended to the long description so the listing cannot drift from the website;
- the hand-written listing source in `plugin/listing.json`: name `tcgreplay`, display name "TCG Replay", subtitle, description, category, starter prompts, version, and the five positive and three negative test cases;
- the release notes from `plugin/release-notes.md`.

The builder checks OpenAI's limits and fails with a message naming the problem: name format, 30-character display name and subtitle, 4,000-character description, at most three starter prompts of 128 characters, exactly five positive and three negative cases with their required fields, https links, and a square PNG icon of 48 to 4096 px under 5 MiB. It also checks that no secret-shaped field such as a client secret or test credentials goes in. `mcp.json` names one streamable-http server with OAuth through dynamic registration. The listing gets a new icon without a Poké Ball, drawn as SVG and rendered to PNG, because the current icon uses a Pokémon trademark. The demo video script and the test cases use the anonymised example game the website already publishes at `/docs/example`, so no copy of the battle log is made. Nothing about the listing goes on the website: while TCG Replay is unlisted, no built page may say "Plugin Directory". Other options were weighed and rejected with evidence in `research.md#alternatives-considered-and-rejected`: upgrading or forking go-sdk, rewriting HTTP bodies, reusing `get_profile`, an email-derived profile ID, matching the reviewer by `sub`, a Node packager, and a hard-coded address.

## Component Breakdown

- **Tool metadata (changed, in the MCP tool server).**
  - Owns each tool's title, its "use this when / do not use" description and its behaviour labels.
  - The existing read-only and game-replacing sets stay the one source of the labels.
  - The single registration helper and the guide tool's registration attach the title, the labels and, in hosted mode, the security scheme to every tool.
  - The shared argument types gain the missing parameter descriptions.
- **Explicit label encoder (new, in the MCP tool server).**
  - A receiving middleware on `tools/list`. It returns the SDK's result wrapped so that its JSON names `readOnlyHint`, `destructiveHint` and `openWorldHint` on every tool, with explicit values.
  - The tool server installs it itself, so the desktop, the hosted server and the domain move's mirrored "moved" server all send the same labels.
  - It is new because the SDK's own type cannot express `readOnlyHint: false`.
- **Server instructions (changed, in the MCP tool server).**
  - The opening is rewritten so its first 512 bytes stand alone: what TCG Replay is, the iid rule, and where the guides are.
  - The rest follows as today, including the guide list built from the guides and the hosted board-link line.
  - The domain move's moved answer still replaces the whole text on old addresses.
- **Account profile tool (new, in the MCP tool server; hosted only).**
  - `get_account` answers who is signed in with the Google account ID, as OpenAI's profile tool.
  - It gets the account from a hook the hosted wiring supplies, reading the request's token per call. With no hook (the desktop) it is not offered.
  - Like `guide`, it needs no game, takes no place and is not activity.
- **Turn wait (unchanged in behaviour).** It already returns within the wait cap with a "call again" result. Hosted start-up already bounds the cap at 50 s. A test pins both against ChatGPT's 60-second limit.
- **Sign-in service (changed).** Seals Google's verified-email flag into the access and refresh tokens it issues and returns it with the account on every MCP request, so the reviewer can be recognised on tool calls as well as at sign-in. Everything else about sign-in is unchanged.
- **Per-account registry and user cap (changed).**
  - Gains the reviewer's email as a setting.
  - At sign-in and at every action, a verified match is admitted even when every place is held. Its place is not counted against the cap, so everyone else's limit is unchanged.
  - The registry still refuses every other account at the cap with the existing busy message.
- **Hosted settings and routes (changed).**
  - Reads `TCGREPLAY_OPENAI_CHALLENGE` and `TCGREPLAY_REVIEWER` and hands the reviewer to the registry.
  - Gives the tool server the account hook.
  - Serves the domain-verification token at its well-known path with no sign-in.
  - The start-up log says whether each setting is set, never its value.
- **App Platform definition (changed).** Declares both new settings as secrets with no value, with comments saying where the maintainer sets them and that edits to the file reach the platform only when applied by hand.
- **Privacy page (changed, one sentence).** Says the assistant is given the player's Google account ID so it can tell accounts apart.
- **Plugin package builder (new: a Go package and a small command, run by the Makefile's `plugin` target).**
  - Reads the public address from the App Platform definition and the notice, credit and support link from the shared notice file. Reads the listing source, release notes and icon from the `plugin/` folder.
  - Checks them against OpenAI's limits and writes the plugin ZIP (`plugin.json`, `mcp.json`, icon).
  - It is new because nothing in the project produces a plugin package. It is a separate command so the playmat binary does not grow submission tooling.
- **Listing source and review material (new `plugin/` folder).**
  - Holds the hand-written listing: name, display name, subtitle, description body, category, starter prompts, version, test cases.
  - Holds the release notes, the demo video script, a submission checklist for the maintainer, and the new icon with its SVG source.
  - It points at the website's example game rather than copying it. It is outside every build context and is never published on the website.
- **What's new list (changed, only if present when this lands).** If the launch-docs work has created the What's new list, the description and label changes are added to its launch entry with a refresh needed. Otherwise that work's launch entry covers them, as its plan says.

## Data Structures & Interfaces

No persisted data or game format changes. The plan changes the MCP tool list's wire shape (additively), adds one tool, two settings, one HTTP route, one token claim, and two file formats for the plugin package.

**Tool definition on the wire** (each entry of `tools/list`): every tool now carries a `title`, all three labels as explicit booleans, and, in hosted mode, a security scheme. Existing fields (`name`, `description`, `inputSchema`) keep their names and meaning; every `inputSchema` property gains a `description`.

```json
{
  "name": "get_state",
  "title": "Read the board",
  "description": "Use this when … Do not use this to …",
  "inputSchema": { "type": "object", "properties": { "view": { "type": "string", "description": "…" } } },
  "annotations": { "readOnlyHint": true, "destructiveHint": false, "openWorldHint": false, "idempotentHint": true },
  "_meta": { "securitySchemes": [ { "type": "oauth2" } ] }
}
```

**Explicit label encoder**: a receiving middleware and the result wrapper it returns for `tools/list`.

```go
func explicitLabels() mcp.Middleware            // wraps tools/list only; passes every other method through
type labelledTools struct{ *mcp.ListToolsResult } // MarshalJSON writes readOnlyHint, destructiveHint, openWorldHint on every tool
```

**Account profile tool** `get_account` (hosted only): no arguments; marked as OpenAI's profile source; result in `structuredContent` with a JSON text copy.

```json
{
  "name": "get_account", "title": "Who is signed in",
  "inputSchema": { "type": "object", "properties": {}, "additionalProperties": false },
  "outputSchema": { "type": "object", "properties": { "id": { "type": "string", "minLength": 1, "pattern": "\\S" } }, "required": ["id"], "additionalProperties": false },
  "annotations": { "readOnlyHint": true, "destructiveHint": false, "openWorldHint": false },
  "_meta": { "openai/profile": true, "securitySchemes": [ { "type": "oauth2" } ] }
}
// result: { "id": "<Google sub>" }
```

```go
// mcpserver.Server gains the hook the hosted wiring supplies; nil means the tool is not offered.
Account func(ctx context.Context, extra *mcp.RequestExtra) (id string, err error)
```

**Signed-in account on MCP requests**: access and refresh tokens seal Google's verified flag (`v`, the claim the board cookie already carries), and `hostauth.AccountFromToken` returns it as `Account.EmailVerified`. A token without the claim opens as unverified.

**Settings** (environment only, declared `type: SECRET` with no value in `.do/app.yaml`):
- `TCGREPLAY_OPENAI_CHALLENGE`: the domain-verification token from OpenAI's dashboard. Empty means the route answers 404.
- `TCGREPLAY_REVIEWER`: the reviewer's Google account email. Empty means no reviewer.
- `usergames.Config` gains `Reviewer string`. It is matched without regard to case, only when the account's email is verified.

**Domain-verification route**: `GET /.well-known/openai-apps-challenge`. With no sign-in it returns `200`, `Content-Type: text/plain; charset=utf-8`, and a body that is exactly the token with no newline. With no token set it returns `404`.

**Listing source** (`plugin/listing.json`, hand-written): everything in the listing that is not derived from elsewhere.

```json
{
  "name": "tcgreplay", "version": "1.0.0",
  "displayName": "TCG Replay", "shortDescription": "Replay and review TCG games",
  "description": "…body; the builder appends the shared notice and credit…",
  "developerName": "…", "category": "…", "capabilities": ["…"],
  "defaultPrompt": ["…", "…", "…"],
  "review": { "test_cases": {
    "positive": [ { "description": "…", "prompt": "…", "tools_triggered": "import_log", "expected_behavior": "…" } ],
    "negative": [ { "description": "…", "prompt": "…" } ] } }
}
```

**Plugin package** (`bin/tcgreplay-plugin.zip`): `plugin.json` (agent-plugins schema 1.0.0; `extensions["com.openai"]` with `interface` (display name, subtitle, long description with the notice and credit, website, privacy, terms and support links, icon and logo), `review.test_cases`, `publication.release_notes`), `mcp.json` (one server, `tcgreplay`, `type: streamable-http`, `url: <public address>/mcp`, `auth: {type: oauth, client: {mode: dcr}}`), and `assets/icon.png`. Builder entry points:

```go
type Sources struct { AppSpec, Notice, Listing, ReleaseNotes string; Icon []byte } // file contents
func Build(src Sources) (zip []byte, err error) // err names the first rule broken
```

## Implementation Detail

**One place per tool, checked by one test.** Today a tool's name, description and labels are spread across the registration call and two name sets. After this work, registering a tool means giving its name, title and description, with the "Use this when …" and "Do not use this …" sentences, at the same call. The labels still come from the read-only and game-replacing sets. A new metadata test reads the raw tool list a client receives and names every tool missing a title, a label, either sentence or a parameter description. A developer adding a tool sees that failure before review, and the error tells them exactly what is missing. Descriptions follow the knowledge base's guidance: open with "Use this when", name the input that triggers the tool, keep the action-steering sentences the tools already carry (import rather than read the log, wait in the same reply), and tell similar tools apart, such as `get_state` and `get_log`, `move` and the specific verbs, `rewind` and `undo`. A description change reaches ChatGPT only after the player refreshes the connection, which the release notes say.

**A new pattern: shaping the wire with a receiving middleware.** The playmat already wraps MCP methods with receiving middleware: the rate limiter, the observability tool-call recorder, and the domain move's moved answer. This plan adds the first one owned by the tool server itself. It touches only `tools/list` and changes only how the result is written, not which tools exist. Because the tool server installs it, every server built from the definition gets it without the hosted wiring knowing: the desktop, the hosted server and the mirrored moved server. A reader sees a short wrapper type with its own JSON method beside the label sets, and a comment naming the SDK limitation it works around, so it can be deleted if a later SDK can express a false read-only flag.

**Instructions written for the reader who stops early.** The instructions constant is split into a short opening paragraph and the existing rules. The opening is plain sentences that make sense alone. The rules and the per-guide list follow unchanged, and the length check counts bytes, the stricter measure for "Pokémon". The existing instruction tests (the turn loop, the guide list, the hosted board line) keep passing. One new test checks the first 512 bytes for the three required statements, each ending within the limit.

**Hosted-only additions follow the existing settings and routes pattern.**
- The two settings are read like `TCGREPLAY_ADMINS`: environment only, never a flag, so they never show in a process listing. Start-up describes them as "set" or "not set" without their values, as `adminText` does for admins.
- The challenge route is registered beside the sign-in service's open routes, before the website catch-all, so it never needs a session and never falls through to the website's 404 page when set.
- The reviewer check sits inside the registry's existing place decision, the one point both sign-in and every action pass through. The busy refusal and its message are unchanged for everyone else.
- Sealing the verified flag into tokens reuses the claim the board cookie already carries. Tokens issued earlier keep working and simply read as unverified.

**The profile tool reuses the guide tool's shape.** Like `guide`, it is registered directly rather than through the per-game binding, so it never takes a place, never counts as activity and is never refused as busy. Its account comes from a hook the hosted wiring passes in, which reads the request's token on every call, following the per-request rule in `gotchas/go-sdk-sessions-and-auth.md`. The tool server never imports the sign-in package. With no hook, as on the desktop, the tool is not registered, so the desktop's tool list is unchanged.

**A new kind of artefact: a reproducible submission package.**
- The builder is a pure function from file contents to ZIP bytes plus a thin command that reads the files and writes `bin/`, the build output folder that stays out of git and the image. Tests call the pure function with the real sources and with broken copies.
- It reads `.do/app.yaml` with the same env-entry line scan the hosted Go tests use, moved somewhere both can share, so the Go side has one reader of that file.
- The listing's derived parts (address, links, notice, credit, support) are never typed into the listing source. A maintainer editing the notice or the address changes the next package automatically.
- The review material is plain Markdown and JSON in `plugin/`. The demo video script walks the positive test cases in order, so recording the video and running the test cases are the same rehearsal.

**What stays as it is.** No game, board or guide behaviour changes, and no result shapes change. Tool names and arguments are unchanged, so Claude's connections and the guides' word checks are unaffected. From the first published version on, tool names, parameters and results must stay backwards compatible, because OpenAI holds changed tools for re-review. The submission checklist says so for future releases.

## Dependencies

- **Design documents this plan was built on: none.** The spec carries no design references.
- **Privacy, terms and disclaimer plan (`20261112083238-29247b36-privacy-terms-disclaimer`), must land first.**
  - It creates `internal/notice/notice.json`, which the package builder reads for the notice, the credit and the support link (the GitHub issues page).
  - It also publishes `/privacy` and `/terms`, which the listing links to.
  - The builder fails if the notice file is missing. The privacy page's one new sentence goes into the page that plan writes.
- **Domain move plan (`20261111093353-8ff3589f-domain-move`), must land first.**
  - It sets `TCGREPLAY_PUBLIC_URL=https://tcgreplay.app` in `.do/app.yaml`, the address the builder reads for the MCP URL and the links. A published plugin cannot change its address later.
  - Its mirrored moved server is built from the same tool server definition, so it inherits the label encoder and titles unchanged.
  - Its whole-site scan (no `demo.gs` outside the moved note, no `www`) must keep passing after the privacy page edit.
- **Observability plan (`20261112090330-4ac2ea19-observability`), should land first.**
  - Its rule that no log line carries an email applies to every line this plan adds. The reviewer's email and the challenge token are never logged.
  - If it has not landed, the existing email-bearing lines stay as they are and this plan adds none. Nothing here changes its recorder or its tool-call middleware.
  - `TCGREPLAY_MAX_USERS`, which it sets, is the limit the reviewer bypasses.
- **Launch docs plan (`20261112091756-c5b6d1fd-launch-docs`), no ordering dependency.**
  - No built page may say "Plugin Directory" while its listing flag is off, so nothing in this plan goes on the website except the privacy sentence.
  - If its What's new list exists when this lands, this plan adds its changes to the launch entry. Otherwise its launch entry covers them.
  - Its Plugin Directory route is switched on only after OpenAI approves the listing this plan prepares.
- **`github.com/modelcontextprotocol/go-sdk` v1.6.0 (existing).** Provides receiving middleware, `ListToolsResult`, `Tool.Title`, `Tool.Meta`, `OutputSchema` and `structuredContent`. Its `ToolAnnotations.ReadOnlyHint` cannot express false, which the label encoder works around. No upgrade.
- **Go standard library (`archive/zip`, `image/png`, `encoding/json`).** Used by the package builder. No new module.
- **Internal packages `mcpserver`, `hostauth`, `usergames` and the hosted command (existing).** Each changes as the Component Breakdown describes. None needs to change before this work starts.
- **The website's example game (existing).** The anonymised battle log and decklist at `/docs/example` are the sample the test cases and video script use. They are used unchanged.
- **`rsvg-convert` (developer tool, present locally).** Renders the new icon's SVG to PNG once. The PNG is committed, so builds do not need it.
- **OpenAI's plugin submission (external).**
  - The package format, limits, test case fields, profile tool rules and domain challenge follow developers.openai.com/plugins (read 2026-11-12).
  - Identity verification, the reviewer's Google account (without two-step verification), recording the video and pressing submit are the maintainer's. They are spec non-goals.
- **DigitalOcean App Platform (external).** The two new secrets are set in its console. Edits to `.do/app.yaml` reach the platform only when the maintainer applies them by hand.
- **Knowledge entries relied on:**
  - `learnings/publishing-to-chatgpt.md`: OpenAI's rules and the checklist.
  - `gotchas/chatgpt-tool-discovery.md`: descriptions over instructions; refresh after a description change.
  - `gotchas/handoff-in-tool-results.md`: hand-offs stay in tool results.
  - `gotchas/go-sdk-sessions-and-auth.md`: per-request account lookup.
  - `gotchas/flaky-guide-instructions-test.md`: a known flake in the instructions test.
  - `conventions/skills-text-and-packaging.md`: guide text and packaging.

## Testing Approach

The tests extend the project's existing Go suites in their own style. The MCP tool server's tests run every check over both the in-memory and the Streamable HTTP transports. The hosted command's tests start a real hosted server against a fake Google and connect real MCP clients. One small website browser test covers the privacy sentence. No new framework is added. Most coverage goes to the tool server, because the review's automated checks read its tool list, and to the package builder, because a package that fails OpenAI's validation stops the submission.

**Tool metadata (unit, both transports).** The load-bearing guard reads the raw `tools/list` JSON, not the Go client's view, which hides an omitted `false`. For every tool it asserts:
- `readOnlyHint`, `destructiveHint` and `openWorldHint` are present as booleans, matching the read-only and game-replacing sets: read-only for the reads, destructive only for `new_game`, `import_log` and `set_opponent_deck`, and never open-world;
- there is a non-empty `title`;
- the description contains a "Use this when" sentence and a "Do not use" sentence;
- every input property has a description.

The existing label test keeps its check that every name in the sets is a real tool. A size test reports the `tools/list` byte count and fails above the recorded ceiling. In hosted configuration, every tool carries the oauth2 security scheme. On the desktop it carries none, and the desktop's tool names are exactly as before, the guard against breaking Claude.

**Instructions opening (unit).** The first 512 bytes of the instructions name TCG Replay as a Pokémon TCG playmat, give the rule to reference cards by iid, and point to the `guide` tool, each as a complete sentence ending inside the limit. The existing instruction tests (turn loop, guide list, hosted board line) still pass.

**Waits under the limit (unit and hosted).**
- A wait for a turn that is not over returns `still_waiting`, with `keep_waiting` true and a `next` telling the assistant to call `wait_for_turn` again, within the configured cap.
- Hosted start-up's default cap and its largest accepted cap are both under 60 seconds, and a larger cap is refused.

**Identity (hosted integration).**
- A connector signed in through the fake Google lists `get_account` with the profile marker, read-only labels, no input properties and an output schema requiring only `id`.
- Calling it returns `structuredContent` `{id: <the fake account's sub>}`.
- A second connection for the same account returns the same id; another account gets a different id.
- At a full cap, the call still answers without taking a place.
- The desktop server does not offer the tool.

**Domain challenge (hosted integration).** With the token set, `GET /.well-known/openai-apps-challenge` without a cookie or token returns 200, `text/plain`, and a body equal to the token byte for byte. With it unset, the route returns 404. The start-up log says whether it is set and never prints the token.

**Reviewer past the limit (hosted integration).** At a cap of 1, held by another account:
- the reviewer's connector signs in, imports the example battle log and makes a move;
- the reviewer's board sign-in succeeds;
- a third, non-reviewer account is still refused with the busy page;
- an account whose email matches the reviewer's but is unverified is refused;
- the reviewer's place does not count against the cap.

Tokens issued after the change carry the verified flag, and a token without it reads as unverified. The start-up log never names the reviewer, and no new log line carries an email.

**Plugin package (unit).**
- Built from the real sources, the ZIP holds exactly `plugin.json`, `mcp.json` and the icon.
- `plugin.json` has the display name "TCG Replay", the long description ending with the shared notice and credit word for word from the notice file, and website, privacy, terms and support links built from the App Platform address and the notice file's help link.
- It has exactly five positive cases, each with description, prompt, tools triggered and expected behaviour, and three negative cases, each with description and prompt.
- `mcp.json` names `<public address>/mcp`, streamable-http, with OAuth through dynamic registration.
- Broken sources are each refused with a message naming the rule: a display name over 30 characters, a description over 4,000, four starter prompts, four positive cases, an http link, a non-square or tiny icon, a missing notice file, and a field such as `test_credentials` or `clientSecret`.
- A cross-check fails if the listing source spells out an address, the notice or the support link rather than leaving them to the builder.

**Review material (unit).**
- The video script and the test cases reference the example game's page.
- `internal/game`'s existing guard already proves the example battle log imports.
- The listing's name and icon carry no Pokémon trademark text. The icon itself is a manual review below.

**Privacy page (website browser suite).** The assistant paragraph says the assistant is given the player's Google account ID, and the domain move's whole-site scan and the "Plugin Directory" rule still pass.

**Deliberate gaps:**
- No automated test drives ChatGPT, OpenAI's upload form or MCP Inspector, because all three are external.
- No test reads the deployed site.
- Claude's unchanged behaviour is guarded by the existing suites passing unchanged, plus the desktop tool-name check, and confirmed by hand below.

**Success metrics:**
- "The submission passes OpenAI's automated checks and reaches human review": **Manual — captured in the implementation test plan.** The builder's validation and the metadata guard cover what can be checked locally.
- "If the listing is rejected, the reason is not missing metadata, missing material or a broken connection": **Manual — captured in the implementation test plan.** It is judged from OpenAI's review email.
- "TCG Replay is listed in ChatGPT's Plugin Directory": **Manual — captured in the implementation test plan.** It depends on OpenAI's decision.

**Manual reviews and checks:**
- Package accepted: the built ZIP uploads to OpenAI's submission form without validation errors. It shows "TCG Replay", the icon, the description with the fan-project notice, and website, privacy, terms and support links that open. **Manual — captured in the implementation test plan.**
- Domain verified: after the maintainer sets the token and deploys, `https://tcgreplay.app/.well-known/openai-apps-challenge` returns exactly the token in a private window, and OpenAI's dashboard accepts it. **Manual — captured in the implementation test plan.**
- MCP Inspector: connect to `https://tcgreplay.app/mcp`, complete Google sign-in through its Auth settings, list tools (labels, titles, profile marker visible), and call `get_account`, `import_log` with the example game and `get_state`. **Manual — captured in the implementation test plan.**
- End to end in ChatGPT: in a new chat, connect TCG Replay, sign in with Google, import the example battle log and decklist, open the board link, and review and replay the game without errors. Each of the five positive cases produces its expected behaviour, and each of the three negative cases is declined or handled as written. **Manual — captured in the implementation test plan.**
- Reviewer account: signed in with the configured reviewer account while the deployed playmat is at its limit (or with the limit lowered for the check), it gets in and plays, and a non-reviewer account is turned away. **Manual — captured in the implementation test plan.**
- Existing assistants unaffected: claude.ai and Claude Code connect, import the example battle log and play exactly as before. **Manual — captured in the implementation test plan.**
- Icon and listing review: the maintainer approves the new icon and the listing text: no Pokémon trademark in name or icon, no targeting of under-13s, and no comparisons or pricing. **Manual — captured in the implementation test plan.**
- Tool list size: the measured `tools/list` size after the description pass is recorded in the release notes, and the tools still appear in ChatGPT after a refresh, with no size error. **Manual — captured in the implementation test plan.**

## Milestones & Tasks

### Milestone 1: Every tool tells an assistant what it does, when to use it and whether it changes anything

**What changes**: Every one of the playmat's tools now comes with a readable title, a description that says when to use it and when not to, and a description of each of its inputs. Each tool also states plainly whether it only reads, whether it can destroy or replace the player's game, and whether it reaches outside the playmat. ChatGPT no longer asks the player to confirm every look at the board, and OpenAI's review finds no unlabelled tool. The playmat's instructions open with a short paragraph that stands alone: what TCG Replay is, that cards are named by their iids, and where the coaching guides are. An assistant that reads only the start still knows the essentials. Waiting for the player's turn is shown to return well inside ChatGPT's one-minute limit. Claude sees the same tools and arguments as before, only better described. Players already connected in ChatGPT see the new descriptions after refreshing the connection.

**Validation point**: The Go suites pass. The metadata guard finds a title, three explicit labels, both description sentences and every parameter description on all tools, and the size test passes at its recorded ceiling. The instructions-opening test passes, and the wait tests show a call returning under the cap with a "call again" result.

#### - [ ] Task: Explicit labels on every tool
**Id:** 25c14f80-304d-42f9-b352-359b925c0f5e
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Makes every tool in the list the playmat sends an assistant state all three labels with an explicit true or false: whether it only reads, whether it can destroy or replace the player's game, and whether it reaches outside the playmat. The MCP library leaves out a false "only reads" label and gives read-only tools no "destructive" label, so the playmat writes the tool list's labels itself. In the hosted playmat, every tool also says it needs the player's sign-in, as OpenAI asks.

*Technical detail:* [context.md#task-explicit-labels-on-every-tool](./context.md#task-explicit-labels-on-every-tool)

**Acceptance criteria**:
- [ ] The tool list an assistant receives shows the read-only, destructive and open-world labels on every tool, each with an explicit true or false, on the desktop and on the hosted playmat.
- [ ] The reads are marked read-only, only starting a new game, importing a game and rebuilding it with an opponent list are marked destructive, and no tool is marked as reaching outside the playmat.
- [ ] In the hosted playmat every tool declares that it needs the player's sign-in; on the desktop none does.
- [ ] Tool names, arguments and results are unchanged, so Claude connects and plays as before.

#### - [ ] Task: Titles and descriptions for every tool
**Id:** 331b8154-0bf1-4408-ae2b-17fd6f374740
**Repo:** tcgreplay
**Depends on:**
- 25c14f80-304d-42f9-b352-359b925c0f5e — Explicit labels on every tool
**Execution:** agent

Gives every tool a readable title, a description that opens with when to use it and says when not to, and a description of every input. The descriptions keep the instructions the tools already carry, such as importing a pasted battle log rather than reading it, and waiting in the same reply. They also tell similar tools apart. A test fails whenever a tool lacks a label, a title, either sentence or an input description, so new tools cannot slip through. Another test reports the size of the whole tool list and stops it growing past a recorded ceiling.

*Technical detail:* [context.md#task-titles-and-descriptions-for-every-tool](./context.md#task-titles-and-descriptions-for-every-tool)

**Acceptance criteria**:
- [ ] Every tool in the list has a title, a description containing when to use it and when not to, and a description for every input.
- [ ] A tool added later without a title, a label, either sentence or an input description makes the test suite fail, naming the tool and what is missing.
- [ ] The size of the tool list after the change is measured, recorded and held under a ceiling by a test.
- [ ] The existing guidance in descriptions (import rather than read the log, ask for a missing decklist, call the wait in the same reply, the guide triggers) is still there.

#### - [ ] Task: Instructions that stand alone and waits inside the limit
**Id:** 2c3dc8ab-2f02-4002-a853-580559f0d631
**Repo:** tcgreplay
**Depends on:**
- 331b8154-0bf1-4408-ae2b-17fd6f374740 — Titles and descriptions for every tool
**Execution:** agent

Rewrites the opening of the playmat's instructions to assistants so that its first 512 characters, on their own, say what TCG Replay is, that cards are referred to by their instance IDs, and where the coaching guides are. The rest of the instructions stays as it is. It also pins with tests what the playmat already does: a wait for the other player's turn always returns well within ChatGPT's one-minute limit and tells the assistant to wait again if the turn is not over.

*Technical detail:* [context.md#task-instructions-that-stand-alone-and-waits-inside-the-limit](./context.md#task-instructions-that-stand-alone-and-waits-inside-the-limit)

**Acceptance criteria**:
- [ ] The first 512 characters of the playmat's instructions name what the playmat is, the rule to reference cards by their instance IDs, and where to find the coaching guides, as complete sentences.
- [ ] The turn loop, the guide list and the hosted board-link line are still in the instructions.
- [ ] A wait for a turn that is not over returns in under 60 seconds with a result telling the assistant to call it again.
- [ ] The playmat refuses to start with a wait limit that could exceed ChatGPT's one-minute limit.

### Milestone 2: ChatGPT can identify the player, OpenAI can verify the domain, and the reviewer always gets in

**What changes**: Connected from ChatGPT, the playmat can tell ChatGPT who is signed in, with an identifier that stays the same for that player on every connection, the way OpenAI's rules ask. The privacy page says so. tcgreplay.app can answer OpenAI's domain verification with the token the maintainer gets from OpenAI, set in the hosting console without a new release. One configured reviewer account can sign in and play even when the playmat is serving its maximum number of players, while everyone else is still turned away at the limit. None of this changes anything for players on Claude.

**Validation point**: The hosted Go tests pass:
- `get_account` returns the same id on two connections;
- the challenge path returns exactly the configured token without signing in;
- at a full limit the reviewer signs in, imports the example game and plays, while another account gets the busy page.

The maintainer has set both values in the hosting console, and the live challenge path returns the token.

#### - [ ] Task: Tell ChatGPT who is signed in
**Id:** 85e6a6bd-8ac8-4567-8e39-06636494c9d5
**Repo:** tcgreplay
**Depends on:**
- 25c14f80-304d-42f9-b352-359b925c0f5e — Explicit labels on every tool
**Execution:** agent

Adds a read-only tool to the hosted playmat, `get_account`, marked as OpenAI's profile tool, which answers who is signed in with the player's Google account ID. That ID stays the same for the player on every connection. It needs no game, never takes one of the playmat's places and is not offered on the desktop. The privacy page gains a sentence saying the assistant is given this ID so it can tell accounts apart.

*Technical detail:* [context.md#task-tell-chatgpt-who-is-signed-in](./context.md#task-tell-chatgpt-who-is-signed-in)

**Acceptance criteria**:
- [ ] Connected to the hosted playmat, asking who is signed in returns the player's identifier in the form OpenAI's profile rules require, and the same player gets the same identifier on a second connection.
- [ ] Two different players get different identifiers, and the answer comes even when the playmat is full.
- [ ] The desktop playmat's tools are unchanged.
- [ ] The privacy page says the assistant is given the player's Google account ID to tell accounts apart.

#### - [ ] Task: Answer OpenAI's domain verification
**Id:** 1e610604-b888-471d-8128-8071a5b724b0
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Serves OpenAI's domain-verification token at the address OpenAI checks, as plain text and without signing in, taking the token from a new hosting setting so it can be set without a release. With no token set, the address answers "not found". The hosting definition declares the setting as a secret with no value, and the start-up log says only whether it is set.

*Technical detail:* [context.md#task-answer-openais-domain-verification](./context.md#task-answer-openais-domain-verification)

**Acceptance criteria**:
- [ ] With the maintainer's token configured, `/.well-known/openai-apps-challenge` returns exactly that token as plain text, without signing in.
- [ ] With no token configured, the address answers "not found", and nothing else on the playmat changes.
- [ ] The token never appears in the playmat's logs or in the repository.

#### - [ ] Task: Let the reviewer in past the player limit
**Id:** d2d1dcc6-fbb3-4803-b774-0eb565951b1f
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Adds a hosting setting naming the one reviewer's Google account. That account can sign in and play even when the playmat is serving its maximum number of players, while every other account is still turned away at the limit. The reviewer is recognised only when Google has verified the email, as admins are, both at sign-in and on every tool call. Sign-ins handed to assistants therefore now carry that verified mark. The reviewer's place is not counted against the limit, and the reviewer's address is never logged or kept in the repository.

*Technical detail:* [context.md#task-let-the-reviewer-in-past-the-player-limit](./context.md#task-let-the-reviewer-in-past-the-player-limit)

**Acceptance criteria**:
- [ ] With the player limit reached by other accounts, the reviewer account still signs in, imports the example game and plays, while another non-reviewer account is still turned away.
- [ ] An account with the reviewer's address that Google has not verified is treated like any other account.
- [ ] The reviewer's place does not reduce the places available to other players.
- [ ] With no reviewer configured, the limit works exactly as before, and the reviewer's address never appears in the logs or the repository.

#### - [ ] Task: Set the domain token and reviewer on the hosted playmat
**Id:** 2c4138f0-bd43-4434-839a-1375d156661b
**Repo:** tcgreplay
**Depends on:**
- 1e610604-b888-471d-8128-8071a5b724b0 — Answer OpenAI's domain verification
- d2d1dcc6-fbb3-4803-b774-0eb565951b1f — Let the reviewer in past the player limit
**Execution:** human — needs the OpenAI Platform dashboard (to get the domain token) and the DigitalOcean App Platform console (to set secrets and apply the app spec), which an agent cannot access

Once the code is deployed, the maintainer copies the domain-verification token from OpenAI's dashboard and the reviewer's Google email into the hosting console as the two new secrets. They apply the updated hosting definition by hand, since its edits never reach the platform otherwise. Then they confirm the live address returns the token.

*Technical detail:* [context.md#task-set-the-domain-token-and-reviewer-on-the-hosted-playmat](./context.md#task-set-the-domain-token-and-reviewer-on-the-hosted-playmat)

**Acceptance criteria**:
- [ ] The hosted playmat's settings hold the domain token and the reviewer's email, set in the console and not in the repository.
- [ ] `https://tcgreplay.app/.well-known/openai-apps-challenge` returns exactly the token, and OpenAI's dashboard accepts the domain.

### Milestone 3: The plugin package and the review material are ready to submit

**What changes**: One command builds the plugin package OpenAI's submission form accepts. It is named "TCG Replay", connects to the playmat's address, and carries a new icon with no Pokémon trademark. Its description ends with the same fan-project notice and card-data credit the website shows, and it links to the website, privacy, terms and support pages. The project holds the review material: five positive and three negative test cases, release notes, a script for the demo video, and a submission checklist. All of it uses the example game already published on the website, so the reviewer has a battle log and decklist to paste. Submitting is then the maintainer's job: verifying their identity, uploading, recording the video and pressing submit.

**Validation point**: The package builder's tests pass on the real sources and refuse each broken copy with a message naming the rule. The built package holds the expected name, links, notice, test cases and icon. The manual checks in the test plan (upload validation, MCP Inspector, the end-to-end ChatGPT run) are ready to run against the deployed playmat.

#### - [ ] Task: A listing icon without Pokémon trademarks
**Id:** c00d0a9f-069e-4abb-8b4b-8e3672c04e73
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Draws a new square icon for the listing, in the project's colours, showing fanned cards and a replay mark with no Poké Ball or other Pokémon trademark. It is kept as an editable vector source and as the PNG the package carries. The current icon shows a Poké Ball, which OpenAI's rules and the spec rule out for the listing. The website's own icon is left alone.

*Technical detail:* [context.md#task-a-listing-icon-without-pokemon-trademarks](./context.md#task-a-listing-icon-without-pokemon-trademarks)

**Acceptance criteria**:
- [ ] The project holds a square listing icon, at least 512 pixels across and well under 5 MB, with its editable source beside it.
- [ ] The icon shows no Poké Ball, Pokémon name, logo or character.
- [ ] The website and board icons are unchanged.

#### - [ ] Task: Build the plugin package
**Id:** b00b3652-908e-48ee-9efa-eadf069ceac9
**Repo:** tcgreplay
**Depends on:**
- c00d0a9f-069e-4abb-8b4b-8e3672c04e73 — A listing icon without Pokémon trademarks
**Execution:** agent

Adds one command that builds the plugin package OpenAI's submission form accepts. Its contents:
- the name "TCG Replay" and the playmat's connection address;
- the new icon;
- a description ending with the same fan-project notice and card-data credit the website shows;
- website, privacy, terms and support links.

The address, links, notice and support come from the files the project already keeps them in, so they never drift. The hand-written parts (name, subtitle, description, starter prompts) live in a listing source in the project. The builder refuses, naming the problem, anything OpenAI's form would reject.

*Technical detail:* [context.md#task-build-the-plugin-package](./context.md#task-build-the-plugin-package)

**Acceptance criteria**:
- [ ] One command builds the plugin package, and building twice from the same sources gives the same contents.
- [ ] The package shows the name "TCG Replay", connects to `https://tcgreplay.app/mcp` with the playmat's sign-in, and carries the icon, the description with the fan-project notice and credit, and the website, privacy, terms and support links.
- [ ] The notice, credit, support link and address are taken from the project's existing sources, not typed into the listing.
- [ ] A listing that breaks one of OpenAI's limits (lengths, number of prompts or test cases, link form, icon size or shape, secrets) is refused with a message naming the rule.

#### - [ ] Task: Write the review material
**Id:** 7745b70b-54b0-4f23-b667-05e4c9ab416b
**Repo:** tcgreplay
**Depends on:**
- b00b3652-908e-48ee-9efa-eadf069ceac9 — Build the plugin package
- 331b8154-0bf1-4408-ae2b-17fd6f374740 — Titles and descriptions for every tool
- 85e6a6bd-8ac8-4567-8e39-06636494c9d5 — Tell ChatGPT who is signed in
**Execution:** agent

Writes what OpenAI's review asks for, all into the project:
- five positive and three negative test cases, each with its prompt and expected behaviour, carried in the package;
- release notes for the first version;
- a demo video script that walks the positive cases in order;
- a submission checklist for the maintainer.

The test cases and script use the example game the website already publishes, so the reviewer has a battle log and decklist to paste. If the website's What's new list exists by then, its launch entry gains these tool changes with a refresh needed.

*Technical detail:* [context.md#task-write-the-review-material](./context.md#task-write-the-review-material)

**Acceptance criteria**:
- [ ] The project contains five positive and three negative test cases (each with the prompt and the expected behaviour), release notes, the video script and a pointer to the sample battle log and decklist they use.
- [ ] The positive cases together cover connecting, signing in with Google, importing the sample game, opening the board link, reviewing and replaying.
- [ ] The negative cases cover requests the playmat should decline or hand back, and say what should happen.
- [ ] The built package carries the test cases and release notes, and the release notes tell players already connected to refresh their connection.
- [ ] No website page mentions the Plugin Directory.

## Open Questions

- **Does go-sdk v1.6.0 send a wrapped `tools/list` result exactly as its own JSON encoding writes it, on every transport?**
  - It depends on the SDK's internal handling of a middleware's returned result, which only the first test run shows.
  - If the raw tool list still lacks `readOnlyHint: false`, or the SDK errors on the wrapped type, the implementer of "Explicit labels on every tool" must STOP and ask the user before falling back to rewriting the HTTP response, which would not cover the desktop's in-memory transport.
- **How large is the tool list after the description pass, and does ChatGPT accept it?**
  - It depends on the wording written in "Titles and descriptions for every tool". The baseline is 26,149 bytes, and a community report of a 5,000-token limit is unverified.
  - The implementer measures the list and sets the ceiling.
  - If the list grows past about 40,000 bytes, or the manual ChatGPT check shows a tool-list size error, STOP and ask the user whether to shorten descriptions or accept it, rather than cutting the required "when not to use" sentences.
- **Does OpenAI's submission form accept the package as built?** In particular: the `Games` category, the capability labels, the subtitle wording, and the same PNG used as both `composerIcon` and `logo`.
  - Only uploading to the dashboard shows this, which happens in the manual "Package accepted" check.
  - Fix each reported field in the listing source or the builder and rebuild.
  - If OpenAI rejects something the spec fixes, STOP and ask the user: the name "TCG Replay", the address, or the notice wording.
- **Does the OpenAI dashboard's MCP connection register with a redirect address the sign-in service already accepts?** It accepts ChatGPT's fixed and per-connector callbacks today.
  - Only the live connection step in the dashboard shows this.
  - If registration is refused, report the redirect address the dashboard used and STOP and ask the user before adding it to the accepted list.

## Out of Scope

- **Bundling the coaching workflows as skills in the plugin.** A later version (spec non-goal). The package carries only the MCP server and the icon.
- **Interactive widgets shown inside ChatGPT.** Spec non-goal. The board stays a link the player opens.
- **A licence for the Pokémon card content.** Spec non-goal. The rejection risk over Pokémon content is accepted.
- **Raising the player limit.** Spec non-goal. The observability plan (`20261112090330-4ac2ea19-observability`) measures and sets `TCGREPLAY_MAX_USERS`. This plan only lets the one reviewer past it.
- **The privacy and terms pages and the Plugin Directory connection docs.** They belong to the privacy, terms and disclaimer plan (`20261112083238-29247b36-privacy-terms-disclaimer`) and the launch docs plan (`20261112091756-c5b6d1fd-launch-docs`). This plan adds one sentence to the privacy page and nothing else to the website. Switching the docs' Plugin Directory route on after approval is that plan's task.
- **The maintainer's own steps.** OpenAI identity verification, creating the reviewer's Google account without two-step verification, recording the video, uploading and pressing submit, and publishing after approval (spec non-goal). The submission checklist lists them, and only setting the two secrets is a plan task.
- **Replacing the website and board icon.** They still show a Poké Ball. Only the listing gets the new trademark-free icon, because the spec's constraint names the listing. Changing the site's branding would need its own decision.
- **Renaming the existing `get_profile` deck-profile tool** to free the name OpenAI's examples use. The profile tool is `get_account` instead.
- **Output schemas for the game tools.** Their results stay JSON text as today. Only `get_account` gets an output schema, which OpenAI's profile rules require.
- **A guard that compares tool lists across releases** for backwards compatibility after publication. The submission checklist states the rule. An automated snapshot can follow once a version is published.
- **CIMD client registration.** OpenAI prefers it, but the sign-in service's Dynamic Client Registration already works with ChatGPT, and the package declares that.
- **A country allow-list for the listing.** It is left to OpenAI's default availability.
