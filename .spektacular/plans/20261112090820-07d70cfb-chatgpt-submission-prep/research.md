---
created_date: "2026-11-12"
document_status: final
closed_date: "2026-11-12"
---

# Research: 20261112090820-07d70cfb-chatgpt-submission-prep

## Alternatives considered and rejected

- **Explicit hints by upgrading or forking go-sdk.** go-sdk v1.6.0 declares `ToolAnnotations.ReadOnlyHint bool` with `omitempty` (`$GOMODCACHE/github.com/modelcontextprotocol/go-sdk@v1.6.0/mcp/protocol.go:1381`), so `readOnlyHint:false` never reaches the wire, and read-only tools get no `destructiveHint` (`internal/mcpserver/annotations.go:37-42`). Only v1.6.0 is in the module cache; no release with a pointer field was confirmed, and a `replace` to a fork adds a maintenance burden for one field. Rejected.
- **Rewriting the HTTP response body of `/mcp`.** Would work for Streamable HTTP but not the in-memory/stdio transports the desktop uses, and has to parse SSE frames. Rejected in favour of a receiving middleware that returns a result type with its own JSON encoding (works on every transport: `internal/jsonrpc2/messages.go:118` `NewResponse` marshals the returned `Result` with `encoding/json`).
- **Using the existing `get_profile` as OpenAI's profile tool.** `get_profile` is already the deck-profile tool (`internal/mcpserver/server.go` registration "What is known and assumed about a seat's list"), named in guides and read-only annotations; reusing or renaming it breaks Claude users and guides. OpenAI's own example names its profile tool `get_profile` (developers.openai.com/apps-sdk/build/auth), which collides. Rejected; a new tool `get_account` carries the `openai/profile` marker.
- **Profile id from the observability player ID or the email.** The observability plan's player ID is an HMAC of the email (`20261112090330-4ac2ea19-observability` plan, Pseudonymous IDs); an email can change, and OpenAI requires an id that never changes and says email is not identity. Rejected; the id is Google's `sub` (`hostauth.Account.ID`, `internal/hostauth/hostauth.go:69-71`), as `learnings/publishing-to-chatgpt.md` section 3 says.
- **Matching the reviewer by Google `sub`.** The maintainer knows the reviewer's email, not its `sub`, and nothing in the playmat shows a `sub`. Rejected for the verified-email match the admin list already uses (`internal/hostauth/admin.go:12-14`).
- **Reviewer matched by email alone on tool calls.** Access tokens carry no verified flag today (`internal/hostauth/store.go:404`, `oauth.go:470-472`), so an account with an unverified copy of the reviewer's address could skip the cap. Rejected; the verified flag is sealed into access and refresh tokens and returned by `AccountFromToken`.
- **Reviewer counted inside the cap.** Would shrink everyone else's places by one while the reviewer plays. Rejected; the reviewer holds a place outside the count.
- **Plugin package built by a Node script beside the site.** Node 26 could import `site/src/lib/facts.ts` and `notice.ts`, but has no ZIP writer without a new dependency or the `zip` CLI, and its tests would live in the Playwright suite. Rejected for a Go builder (`archive/zip`, `image/png`, `encoding/json` from the standard library) tested with `go test`, reading `.do/app.yaml` with the same env-entry line scan Go tests already use (`cmd/tcgreplay/hosted_test.go:1223-1246`).
- **Hard-coding `https://tcgreplay.app` in the listing source.** Contradicts the epic rule that `TCGREPLAY_PUBLIC_URL` in `.do/app.yaml` is the single public address. Rejected; the builder reads it.
- **Copying the sample battle log into the review material.** The anonymised example game already exists (`site/src/content/example/battle-log.txt`, decklist `testdata/decks/mega-lucario-ex-v12.txt`, published at `/docs/example`, guarded by `internal/game` `TestSiteExampleGame`). Rejected; the review material points at it.
- **Keeping the current icon.** `docs/icon.png`, `docs/logo.png` and `site/public/icon.png` show a Poké Ball, a Pokémon trademark, which the spec's constraint forbids on the listing. Rejected; a new icon is drawn for the listing.
- **Review material on the website.** While unlisted no built page may contain "Plugin Directory" (launch-docs plan). Rejected; the material lives in a `plugin/` folder in the repo.

## Chosen approach — evidence

- Annotation helper and current sets: `internal/mcpserver/annotations.go:10-43`; test `annotations_test.go:17-55` (uses the Go client, which cannot see an omitted `false`).
- Every tool goes through `addWith` (`server.go:211`) except `guide` (`guides.go:30`): one place to set `Title` and `_meta`.
- Middleware hooks: `srv.AddReceivingMiddleware(a.MCPMiddleware())` in `cmd/tcgreplay/hosted.go` `newHosted`; `Result` interface has an unexported method (`mcp/shared.go:553-561`) but embedding `*mcp.ListToolsResult` satisfies it.
- Current tool list measured: 45 tools, 26,149 bytes of `tools/list` JSON, 102 parameters of which 21 have no description, no tool has a title; instructions 3,381 characters with the guide pointer at the end (`server.go:70-81`, `guides.go:68-76`).
- Wait cap already bounded: default 15 s, `maxWaitCap = 50 * time.Second` refused at start-up (`cmd/tcgreplay/main.go:104-116,169-170`); `still_waiting` result says to call again (`wait_test.go:239-268`).
- Account from token: `hostauth.AccountFromToken` (`oauth.go:466-473`); per-request lookup rule in `gotchas/go-sdk-sessions-and-auth.md`.
- User cap: `usergames.Registry.Use`/`Admit`/`full` (`internal/usergames/usergames.go:94-150`) applied at sign-in and at each action.
- Hosted routes: `hostedHandler` (`cmd/tcgreplay/hosted.go`) registers auth routes, `/mcp`, `/admin`, board, then `/` website; settings read in `hostedAuth` (`TCGREPLAY_ADMINS` pattern) and start-up log describes admins by count only (`adminText`).
- app.yaml env parsing in Go tests: `appEnvFields` (`cmd/tcgreplay/hosted_test.go:1223`); secrets declared without value (`TestAppSpecHealthCheckAndSecrets`).
- DCR with ChatGPT callbacks already accepted (`internal/hostauth/oauth.go:36-48,139-151`), so `mcp.json` uses `auth.type: oauth`, `client.mode: dcr`.
- `rsvg-convert` is available locally to render an SVG icon to PNG.

## Files examined

- tcgreplay:internal/mcpserver/annotations.go:1-43 — read-only and replacing sets; hints built per tool.
- tcgreplay:internal/mcpserver/annotations_test.go:17 — current hint test through the Go client.
- tcgreplay:internal/mcpserver/server.go:44-81 — `MCP()`, instructions text; :196-225 `add`/`addWith`; :260-470 argument types with `jsonschema` tags; :489 `wait_for_turn` description; :620 `get_profile` (deck profile).
- tcgreplay:internal/mcpserver/guides.go:28-77 — guide tool, prompts, guide block in instructions.
- tcgreplay:internal/mcpserver/server_test.go:24-93 — `transports`, `connectServer`, `forEachTransport`, `call` helpers.
- tcgreplay:internal/mcpserver/guides_test.go:105-166,299-321 — instruction and "Use this when" tests.
- tcgreplay:internal/mcpserver/wait.go:14-40 — wait result fields `keep_waiting`, `next`.
- tcgreplay:internal/mcpserver/wait_test.go:239-268 — still_waiting under a short cap.
- tcgreplay:cmd/tcgreplay/main.go:96-170 — wait-cap bounds, `config`.
- tcgreplay:cmd/tcgreplay/hosted.go — settings, `hostedAuth`, `adminText`, `hostedHandler`, `newHosted`.
- tcgreplay:cmd/tcgreplay/hosted_cap_test.go:1-60 — cap test pattern with `hostauthtest` fake Google.
- tcgreplay:cmd/tcgreplay/hosted_test.go:1188-1246 — app.yaml line scan and secret checks.
- tcgreplay:internal/hostauth/hostauth.go:25-80 — `Config`, `Places`, `Account`.
- tcgreplay:internal/hostauth/admin.go:10-16 — verified-email admin match.
- tcgreplay:internal/hostauth/seal.go:92-110 — `grantClaims` with `Verified`.
- tcgreplay:internal/hostauth/store.go:400-408 — access/refresh tokens sealed without `Verified`.
- tcgreplay:internal/hostauth/oauth.go:36-51,139-151,446-473 — callbacks, DCR, `verify`, `AccountFromToken`.
- tcgreplay:internal/hostauth/google.go:75-100,213-232 — sign-in refusal and `signIn` cap check.
- tcgreplay:internal/usergames/usergames.go:94-150 — `Use`, `Admit`, `full`.
- tcgreplay:internal/usergames/mcp.go:20-49 — `ResolveMCP`, `ResolveMCPQuiet`.
- tcgreplay:internal/game/reads.go:274-280 — reads append an audit record (not a game change).
- tcgreplay:site/src/lib/facts.ts:1-60 — facts loader, app.yaml path rule.
- tcgreplay:site/src/lib/example.ts:1-30 — example battle log and deck sources.
- tcgreplay:site/src/pages/privacy.astro:28-34,109-110 — keeps Google account ID; assistant sees what the playmat tells it.
- tcgreplay:.do/app.yaml — public URL, secrets declared without values, comments on applying by hand.
- tcgreplay:Makefile — targets; no plugin target yet.
- tcgreplay:docs/icon.png, docs/logo.png — current icon shows a Poké Ball.
- tcgreplay:.dockerignore — `skills`, `.spektacular` excluded; a `plugin/` folder need not reach the image.

## External references

- OpenAI, Authentication (developers.openai.com/apps-sdk/build/auth, read 2026-11-12) — profile tool: read-only, empty input, `_meta["openai/profile"]: true`, `outputSchema` with required non-blank `id`, optional `name`/`email`/`nickname`, `additionalProperties:false`, result in `structuredContent` plus a JSON text copy; `securitySchemes` per tool, placed in `_meta` in the TypeScript examples.
- OpenAI, Package your plugin (developers.openai.com/plugins/build/plugins.md, read 2026-11-12) — `plugin.json` (`$schema` agent-plugins 1.0.0, `name`, `version`, `description`, `extensions["com.openai"].interface` with `displayName`, `shortDescription`, `longDescription`, `developerName`, `category`, `capabilities`, `websiteURL`, `privacyPolicyURL`, `termsOfServiceURL`, `defaultPrompt`, `brandColor`, `composerIcon`, `logo`); `mcp.json` with `mcpServers.<name>.type: streamable-http`, `url`, `extensions["com.openai"].auth` (`oauth`, client mode `dcr`).
- OpenAI, Upload and submit your plugin (developers.openai.com/plugins/deploy/submission.md, read 2026-11-12) — package name lowercase with single hyphens; `apps`/`.app.json` and hooks blocked; primary icon required, square ≥48 px, ≤5 MiB, ≤4096 px; support (`interface.supportURL`), privacy, terms, website HTTPS URLs required, ≤1024 chars; display name and subtitle ≤30, long description ≤4000, ≤3 starter prompts ≤128; `review.test_cases.positive` (exactly 5: `description`, `prompt`, `tools_triggered`, `expected_behavior`) and `negative` (exactly 3: `description`, `prompt`) and `review.demo_recording_url` and `publication.release_notes`/`countries` live in `plugin.json` under `extensions["com.openai"]`; reviewer credentials only in the dashboard; domain challenge is the exact token as plain text at `/.well-known/openai-apps-challenge`, paths ignored; submission flow order.
- `learnings/publishing-to-chatgpt.md` sources S5-S10, S29-S32 — hint semantics, 512-character instructions, unverified 5,000-token tool-list report, 60 s Codex timeout.

## Prior plans / specs consulted

- `20261112083238-29247b36-privacy-terms-disclaimer` (plan, final) — `internal/notice/notice.json` holds notice, credit and help (GitHub issues); `/privacy` and `/terms` live; the listing reads the notice file; privacy page structure and `data-*` test pattern.
- `20261111093353-8ff3589f-domain-move` (plan, final) — `TCGREPLAY_PUBLIC_URL=https://tcgreplay.app` is the one address; request Host never trusted for an address; moved MCP server mirrors the real tool list from the same `mcpserver.Server` definition with no resolver; whole-site scan for `demo.gs`/`www`.
- `20261112090330-4ac2ea19-observability` (plan, final) — no log line carries an email; player ID is an HMAC of the email (`TCGREPLAY_PLAYER_ID_KEY`); tool-call middleware added after the rate-limit middleware; `TCGREPLAY_MAX_USERS` set from a measurement.
- `20261112091756-c5b6d1fd-launch-docs` (plan, final) — `site/src/lib/listing.ts` `chatgptDirectory.listed=false`; no built page may contain "Plugin Directory" while unlisted; What's new entries in `site/src/data/whats-new.yaml` with `refresh` flag; tool-label changes belong in the launch entry if they ship by launch.

## Open assumptions

- A receiving middleware on `tools/list` may return a type embedding `*mcp.ListToolsResult` with its own `MarshalJSON`, and the SDK encodes it unchanged on every transport. If the SDK rejects or re-wraps it, STOP and ask before falling back to an HTTP-level rewrite.
- OpenAI's dashboard connection uses one of the ChatGPT callbacks hostauth already accepts. If the submission portal's connect step is refused at registration, the redirect it used must be reported before adding it.
- The `category` value "Games" and capability labels are accepted by OpenAI's validator; the upload check confirms.
- Cloudflare does not cache or alter `/.well-known/openai-apps-challenge`.
- The observability plan has landed (no email in logs); new log lines follow it.

## Drafting assumptions

### Destructive means destroying or replacing the game (discovery)
- **Decision**: `destructiveHint` is true only for `new_game`, `import_log` and `set_opponent_deck`; `rewind`, `undo`, `checkout`, `knock_out` and other moves stay false.
- **Rationale**: The spec's requirement defines the label as "can destroy or replace a player's game"; moves append events and nothing is deleted.
- **Rejected**: Marking `rewind` destructive as the knowledge checklist tentatively suggests: rewind deletes nothing.

### Reads that write an audit record stay read-only (discovery)
- **Decision**: `get_state`, `get_log`, `search_deck`, `look_top`, `wait_for_turn` and the other reads stay `readOnlyHint: true`.
- **Rationale**: They change nothing about the player's game; the internal read audit and the wait's presence are bookkeeping, matching the spec's "only reads".
- **Rejected**: Marking them as writes: every look at the board would need ChatGPT confirmation.

### Profile tool named get_account, id is Google's sub (discovery)
- **Decision**: New hosted-only tool `get_account` with `_meta["openai/profile"]: true`, returning only `id` (Google `sub`).
- **Rationale**: `get_profile` is taken by the deck-profile tool; OpenAI requires a never-changing id and says email is not identity; returning only the id keeps data to the minimum.
- **Rejected**: Renaming `get_profile`; the observability player ID (derived from the email); adding the email for display.

### Reviewer matched by verified email, outside the cap (discovery)
- **Decision**: One setting `TCGREPLAY_REVIEWER` (a Google email, declared SECRET in app.yaml); matched case-insensitively only when Google marked it verified; the reviewer takes a place outside the count.
- **Rationale**: The maintainer knows the email; the admin list already matches this way; an outside place keeps the limit unchanged for everyone else.
- **Rejected**: Matching by `sub`; counting the reviewer inside the cap.

### Package builder in Go (discovery)
- **Decision**: `internal/plugin` plus a small `cmd/tcgreplay-plugin` main and a `make plugin` target.
- **Rationale**: Standard library ZIP/PNG/JSON, tested by `go test`, no new dependency.
- **Rejected**: A Node script (needs a ZIP dependency or the `zip` CLI).

### Chosen direction (architecture)
- **Decision**: Fix metadata at the single registration helper plus a `tools/list` middleware that encodes explicit labels; guard with raw-JSON tests and a size ceiling; reorder instructions; pin the existing wait bound with a test; add `get_account`, the challenge route and the reviewer bypass as configuration; build the package with a Go builder from app.yaml, notice.json and a `plugin/` listing source; draw a new trademark-free icon.
- **Rationale**: Every piece reuses an existing seam (registration helper, receiving middleware, hosted settings, admin-style email match, app.yaml env scan), works on every transport, and keeps the epic's single-source rules.
- **Rejected**: See research.md alternatives (SDK fork, HTTP rewrite, renaming get_profile, Node packager, hard-coded address).

### Size ceiling set after the pass (architecture)
- **Decision**: The size test's ceiling is the measured `tools/list` size after the description pass plus 10%, written in the test with the measurement date; the figure goes in the release notes.
- **Rationale**: The spec asks to measure; no official limit exists (the 5,000-token report is unverified), and a ceiling stops silent creep.
- **Rejected**: No ceiling (creep goes unseen); a 5,000-token ceiling (already exceeded today at about 26 KB, and unverified).

### securitySchemes declared in hosted mode (architecture)
- **Decision**: Each tool carries `_meta.securitySchemes: [{"type":"oauth2"}]` in hosted mode only.
- **Rationale**: OpenAI asks for it per tool; every hosted `/mcp` request needs a token; the desktop and the moved server need none.
- **Rejected**: Omitting it (server default inheritance is allowed but OpenAI recommends per tool); `noauth` for `guide`/`card_text` (the HTTP layer refuses anonymous requests anyway).

### Privacy page gains one sentence (architecture)
- **Decision**: The privacy page's assistant paragraph says the assistant is told the player's Google account ID so it can tell accounts apart.
- **Rationale**: OpenAI rejects plugins whose responses carry data the privacy policy does not cover.
- **Rejected**: Leaving it to the generic "what the playmat tells it" sentence.

### Conventions selected (architecture)
- **Decision**: Only the guide text and packaging convention is cited, and only as a constraint to preserve.
- **Rationale**: It is the only always-applied convention; no guide text or build context changes.
- **Rejected**: None dropped.

### Builder is a separate command (components)
- **Decision**: `cmd/tcgreplay-plugin`, not a subcommand of the playmat binary.
- **Rationale**: Submission tooling has no place in the shipped server or desktop binary.
- **Rejected**: `tcgreplay plugin build` (the `sets` subcommand precedent is for running playmats, not packaging).

### What's new entry only if the list exists (components)
- **Decision**: Add the change to the What's new launch entry (refresh needed) only if `site/src/data/whats-new.yaml` exists when this lands; otherwise launch-docs covers it.
- **Rationale**: The launch-docs plan says tool-label changes belong in its launch entry; this spec does not depend on launch-docs.
- **Rejected**: Making launch-docs a dependency of this plan.

### Package name, version and subtitle (data_structures)
- **Decision**: Package `name` "tcgreplay" (the custom connection name), `version` "1.0.0" kept in the listing source and bumped per submission, subtitle drafted to fit 30 characters.
- **Rationale**: OpenAI requires lowercase with single hyphens; the custom connection is already named `tcgreplay`; a version in the source makes each ZIP reproducible.
- **Rejected**: "tcg-replay" (differs from the connection name); taking the version from the binary's build stamp (the listing version is not the server's).

### Countries left out (data_structures)
- **Decision**: `publication.countries` is omitted, leaving availability to OpenAI's default targeting.
- **Rationale**: The spec sets no regional limit, and OpenAI says omitting the field keeps existing targeting.
- **Rejected**: An explicit allow-list (no requirement for one).

### Byte count for the 512-character limit (implementation_detail)
- **Decision**: The instructions-opening test counts the first 512 bytes, not runes.
- **Rationale**: OpenAI says characters without defining them; bytes are never more lenient than characters, and "Pokémon" is multi-byte.
- **Rejected**: Counting runes (could pass while a byte-based reader truncates).

### Share the app.yaml env scanner (implementation_detail)
- **Decision**: Move the hosted tests' env-entry line scanner into a small shared helper the builder and the tests both use.
- **Rationale**: One Go reader of `.do/app.yaml`; no YAML dependency added.
- **Rejected**: Adding a YAML module; a second hand-written scanner.

### Address placeholder in the listing source (tasks)
- **Decision**: The listing source writes `{public}` where a test case needs the playmat's address; the builder substitutes the App Platform address and refuses any literal URL in the source.
- **Rationale**: Keeps the single-address rule while letting test cases point the reviewer at the example page.
- **Rejected**: Literal URLs in test cases (drift from app.yaml); no URL (the reviewer could not find the sample log).

### Test cases use the website's example game (tasks)
- **Decision**: The reviewer's sample battle log and decklist are the anonymised example at `/docs/example`; positive cases cover import, board, review, replay and restore; negative cases cover a missing decklist, an unrelated request and a web lookup.
- **Rationale**: The example is already anonymised, published and guarded by a test; the cases together walk the spec's end-to-end journey.
- **Rejected**: A new sample log (a second copy to keep anonymised and working).

### Category "Games" and developer name (tasks)
- **Decision**: `category: "Games"`, `developerName: "the tcgreplay project"` (matching the terms page's operator).
- **Rationale**: Closest fit; OpenAI's accepted category values are not documented, and the upload check will report a bad one.
- **Rejected**: "Education" (coaching fits, but games is the primary use).

### Website icon unchanged (tasks)
- **Decision**: Only the listing gets the new icon; the website and board keep the Poké Ball icon.
- **Rationale**: The spec's constraint names the listing's icon; changing the site's branding is outside the spec.
- **Rejected**: Replacing every icon (scope growth; flagged in the summary for the maintainer).

## Rehydration cues

- `spektacular spec file read 20261112090820-07d70cfb-chatgpt-submission-prep`
- `spektacular plan file read <dep> plan` for the four epic plans above.
- `spektacular knowledge read` on `learnings/publishing-to-chatgpt.md`, `gotchas/chatgpt-tool-discovery.md`, `gotchas/handoff-in-tool-results.md`, `gotchas/go-sdk-sessions-and-auth.md`, `gotchas/flaky-guide-instructions-test.md`.
- Re-read `internal/mcpserver/{annotations,server,guides}.go`, `cmd/tcgreplay/hosted.go`, `internal/usergames/usergames.go`, `internal/hostauth/{oauth,store,seal}.go`.
- Re-measure the tool list with a throwaway program that lists tools over an in-memory transport and prints `len(json)`.
