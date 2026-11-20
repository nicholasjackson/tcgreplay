---
created_date: "2026-11-12"
document_status: final
closed_date: "2026-11-12"
---

# Context: 20261112090820-07d70cfb-chatgpt-submission-prep

## Current State Analysis

All work is in the one registered repo, `tcgreplay` (root `/home/nicj/code/github.com/nicholasjackson/tcgreplay`). Measured on 2026-11-12 at commit 6ecc1aa:

- **Tool list.** 45 tools; the desktop `tools/list` JSON is 26,149 bytes. No tool has a `title`. 21 of 102 input properties have no description.
  - Read-only tools send `readOnlyHint:true` with no `destructiveHint`. Write tools send `destructiveHint` and `openWorldHint` but never `readOnlyHint:false`, because go-sdk v1.6.0's `ToolAnnotations.ReadOnlyHint` is a `bool` with `omitempty` (`$GOMODCACHE/github.com/modelcontextprotocol/go-sdk@v1.6.0/mcp/protocol.go:1357-1384`).
  - Labels come from `internal/mcpserver/annotations.go:10-43`. Every tool except `guide` is registered through `addWith` (`internal/mcpserver/server.go:210-225`); `guide` at `internal/mcpserver/guides.go:30`.
- **Instructions.** 3,381 characters (`internal/mcpserver/server.go:67-91` plus `guides.go:68-76`). The guide pointer comes at the very end, far past 512 characters.
- **Turn wait.** Capped by `game.Timing.WaitCap`: default 15 s from `TCGREPLAY_WAIT_CAP`; start-up refuses a cap over `maxWaitCap = 50s` (`cmd/tcgreplay/main.go:104-116,169-170`). A `still_waiting` result says to call again and carries `keep_waiting` (`internal/mcpserver/wait.go:14-40`).
- **Profile.** There is no profile tool. `get_profile` (`internal/mcpserver/server.go:879`) is the deck-profile tool.
- **Domain challenge.** There is no `/.well-known/openai-apps-challenge` route. `hostedHandler` (`cmd/tcgreplay/hosted.go:188-215`) sends unknown paths to the website.
- **User cap.** `usergames.Registry.Use`/`Admit`/`full` (`internal/usergames/usergames.go:94-150`) refuses any new account when `MaxUsers` places are held, at sign-in and at each action. There is no reviewer exception.
  - Access and refresh tokens do not seal the verified flag (`internal/hostauth/store.go:400-408`). `AccountFromToken` returns no `EmailVerified` (`internal/hostauth/oauth.go:466-473`).
- **Plugin package and listing.** There is no package builder and no `plugin/` folder. The only icon (`docs/icon.png`, `docs/logo.png`, `site/public/icon.png`) shows a Poké Ball.
- **Sample battle log.** It already exists, anonymised: `site/src/content/example/battle-log.txt`, with the decklist `testdata/decks/mega-lucario-ex-v12.txt`, published at `/docs/example` and guarded by `internal/game/site_example_test.go:26`.
- **Sign-in.** Dynamic Client Registration already accepts ChatGPT's callbacks (`internal/hostauth/oauth.go:36-48,139-151`).

Requirement to repo and files:
- Every tool is labelled → `tcgreplay`: `internal/mcpserver/annotations.go`, `server.go`, `guides.go`.
- Every tool is described → `tcgreplay`: `internal/mcpserver/server.go`, `guides.go`, tests.
- Instructions survive being cut short → `tcgreplay`: `internal/mcpserver/server.go`, `guides_test.go`.
- Waiting returns in time → `tcgreplay`: `internal/mcpserver/wait_test.go`, `cmd/tcgreplay` tests.
- ChatGPT can tell who is signed in → `tcgreplay`: `internal/mcpserver/account.go`, `internal/usergames/mcp.go`, `cmd/tcgreplay/hosted.go`, `site/src/pages/privacy.astro`.
- The domain is verified → `tcgreplay`: `cmd/tcgreplay/hosted.go`, `cmd/tcgreplay/main.go`, `.do/app.yaml`, plus a human task.
- The plugin package can be built → `tcgreplay`: `internal/appspec`, `internal/plugin`, `cmd/tcgreplay-plugin`, `plugin/`, `Makefile`.
- The reviewer always gets in → `tcgreplay`: `internal/hostauth/store.go`, `oauth.go`, `internal/usergames/usergames.go`, `cmd/tcgreplay`, `.do/app.yaml`, plus a human task.
- The review material is ready → `tcgreplay`: `plugin/listing.json`, `plugin/release-notes.md`, `plugin/demo-script.md`, `plugin/SUBMITTING.md`.
- It works end to end in ChatGPT → covered by the positive test cases plus manual checks.

## Per-Task Technical Notes

### Task: Explicit labels on every tool

**Requirement → repo/files:** "Every tool is labelled" → `tcgreplay`, `internal/mcpserver`.

**File changes:**
- `internal/mcpserver/annotations.go:1-43` — keep `readOnlyTools` and `replacingTools` as the one source. Add `explicitLabels() mcp.Middleware`. It intercepts only `method == "tools/list"`, calls `next`, and returns `labelledTools{res.(*mcp.ListToolsResult)}`. That type embeds `*mcp.ListToolsResult` (embedding satisfies the unexported `isResult`, `mcp/shared.go:553`) and has `MarshalJSON`, which marshals the embedded result to a map. For each tool it sets `annotations.readOnlyHint`, `annotations.destructiveHint` and `annotations.openWorldHint` explicitly (read-only tools: `destructiveHint:false`; writes: `readOnlyHint:false`), from the sets by tool name. Keep the comment naming the go-sdk v1.6.0 limitation (`ReadOnlyHint bool` with `omitempty`, `mcp/protocol.go:1381`). Pass every other method through untouched.
- `internal/mcpserver/server.go:43-62` (`MCP()`) — after `s.register(srv)` and `s.registerGuides(srv)`, call `srv.AddReceivingMiddleware(explicitLabels())`. The tool server owns it, so `newHosted` and the domain move's mirrored moved server inherit it.
- `internal/mcpserver/server.go:210-225` (`addWith`) and `internal/mcpserver/guides.go:30` — build the `mcp.Tool` through one helper, e.g. `s.tool(name, title, desc)`, which sets `Annotations: annotations(name)` and, when `s.Resolve != nil` (hosted), `Meta: mcp.Meta{"securitySchemes": []map[string]any{{"type": "oauth2"}}}`. The title argument is filled by the next task. This task can pass `""` and add the parameter, or add titles directly.
- `internal/mcpserver/annotations_test.go:17-55` — keep the "every set name is a real tool" check. Add a raw-JSON test over both transports. Use `mcp.NewInMemoryTransports()` with a raw `Connection` (`jsonrpc` package from `github.com/modelcontextprotocol/go-sdk/jsonrpc`), or POST `initialize`, `notifications/initialized` and `tools/list` to an `httptest` Streamable handler and read the JSON or SSE body. Assert that every tool's `annotations` object has the three keys as JSON booleans with the expected values. Add a hosted-configured case (`Server{Resolve: …}` fake) asserting `_meta.securitySchemes` on every tool, and a desktop case asserting none.

**Complexity:** Medium
**Token estimate:** ~40k tokens
**Agent strategy:** Single agent, sequential. Write the raw-JSON test first (it fails today on `readOnlyHint`), then the middleware.
**Note:** If the SDK refuses or re-encodes the wrapped result (the test sees the plain struct encoding), STOP and ask before switching to an HTTP-level rewrite (research.md open assumptions).

### Task: Titles and descriptions for every tool

**Requirement → repo/files:** "Every tool is described for an assistant" → `tcgreplay`, `internal/mcpserver`.

**File changes:**
- `internal/mcpserver/server.go:470-930` (`register`) — for each of the 44 tools registered here, add a title (short, human, e.g. "Read the board", "Import a TCG Live game") and rewrite the description to open with "Use this when …" and contain a "Do not use this …" sentence. Keep the existing behavioural text:
  - `import_log` (`:487`): every phrase `TestToolsSayWhenToUseThem` checks, `guides_test.go:299-321`;
  - `wait_for_turn` (`:748`): the same-reply rule and the outcomes `TestWaitToolDescribesDecisions` checks, `wait_test.go:743`;
  - `end_turn`, `place_damage`, `play_trainer` pair rule (`pair_test.go`), `set_opponent_deck` branch rule.
  - Tell apart `get_state`/`get_log`, `move`/specific verbs, `rewind`/`undo`/`checkout`, `switch`/`retreat`, `search_deck`/`look_top`, `get_profile`/`sample_hidden`.
- `internal/mcpserver/guides.go:30,64-70` — title for `guide` ("Read a coaching guide"). `guideToolDescription` gains a "Do not use …" sentence and keeps every guide's trigger (checked by `TestToolsSayWhenToUseThem`).
- `internal/mcpserver/server.go:260-466` (argument structs) — add `jsonschema` descriptions to the 21 undescribed properties found on 2026-11-12:
  - `add_effect.text`, `annotate.event_id`, `annotate.text`;
  - `iid` on `attach`, `bench`, `move`, `place_active` and `play_trainer`;
  - `attack.name`, `checkout.branch`, `coin_flip.n`, `look_top.n`, `look_top.whose`;
  - `new_game.name_p1`, `new_game.name_p2`, `place_damage.target`, `remove_effect.id`, `reveal.iids`;
  - `set_condition.clear`, `set_condition.target`, `shuffle.whose`.

  Mostly the shared `IIDArg`, `TargetArg` and `Empty`-adjacent types. Do not rename or retype any argument.
- `internal/mcpserver/annotations_test.go` (or a new `metadata_test.go`) — the guard, reusing the raw-JSON reader from the previous task. For every tool:
  - a non-empty `title`;
  - description contains "Use this when" and "Do not use";
  - every `inputSchema.properties.*.description` is non-empty;
  - the three labels are present.

  Failures name the tool and the gap. Add a size test: `len(raw tools/list result JSON)` in the hosted configuration, `t.Logf` the figure, and fail above `toolListCeiling`. Set the ceiling to the post-pass measurement plus 10%, with the date in a comment. Baseline before the pass: 45 tools, 26,149 bytes (desktop, in-memory).
- `internal/mcpserver/server_test.go:417-460` (`TestToolList`) — add a check that the desktop tool names equal the pre-change list of 45 (the Claude-unaffected guard).
- `internal/guides/*.md` — no change expected. If a guide quotes a description phrase, keep the phrase rather than editing the guide (convention: no "Claude" or "Nic " in guides).

**Complexity:** High (45 tools of careful wording)
**Token estimate:** ~90k tokens
**Agent strategy:** Parallel analysis, sequential integration. One agent drafts titles and descriptions for the game-flow verbs, another for reads, branches and records. A single integrator applies them to `server.go` in one pass, adds the parameter descriptions and the guard, then measures and sets the ceiling.

### Task: Instructions that stand alone and waits inside the limit

**Requirement → repo/files:** "The instructions survive being cut short", "Waiting returns in time" → `tcgreplay`, `internal/mcpserver`, `cmd/tcgreplay`.

**File changes:**
- `internal/mcpserver/server.go:67-79` (`instructions` constant) — prepend an opening paragraph, for example: "TCG Replay is a virtual Pokémon TCG playmat for replaying, reviewing and playing games; it is a playmat, not a rules engine, so you police legality. Refer to every card by its instance ID (iid, such as "p2-12"), never by name; get_state lists them. Coaching workflows such as importing, reviewing and drills have guides: read the matching guide with the guide tool before you start, or call guide with no name to list them." The whole paragraph must fit in 512 bytes. Keep the existing bullet rules after it. The first line's "Virtual Pokémon TCG playmat…" sentence may be merged into the opening.
- `internal/mcpserver/server.go:85-91` (`instructions()`) — unchanged order: opening and rules, then `guideInstructions()`, then the board line.
- `internal/mcpserver/guides_test.go:105-166` — new test `TestInstructionsOpening`, over desktop and hosted. It takes `ins[:512]` in bytes and checks:
  - it contains "TCG Replay" and "playmat", the iid rule ("instance ID" and "never by name"), and "guide";
  - the sentence carrying each ends with "." before byte 512.

  `TestGuideInstructions` and `TestInstructionsTeachTheLoop` (`wait_test.go:386`) must still pass. A lone `TempDir RemoveAll` failure under `-race` is the known flake (`gotchas/flaky-guide-instructions-test.md`).
- `internal/mcpserver/wait_test.go:239-268` — extend `TestWaitForTurnStillWaiting`, or add a test, asserting `keep_waiting == true` and that `next` names `wait_for_turn`.
- `cmd/tcgreplay/main.go:104-116,169-170` — no behaviour change. Add a test in `cmd/tcgreplay/timing_test.go` or `main_test.go`, if not already present, asserting the default `TCGREPLAY_WAIT_CAP` is 15 s, `maxWaitCap` is under 60 s, and a 61 s cap is refused.

**Complexity:** Low
**Token estimate:** ~20k tokens
**Agent strategy:** Single agent, sequential execution.

### Task: Tell ChatGPT who is signed in

**Requirement → repo/files:** "ChatGPT can tell who is signed in" → `tcgreplay`, `internal/mcpserver`, `cmd/tcgreplay/hosted.go`, `site/src/pages/privacy.astro`.

**File changes:**
- `internal/mcpserver/server.go:18-37` (`Server`) — add `Account func(ctx context.Context, extra *mcp.RequestExtra) (string, error)`; nil means not offered.
- `internal/mcpserver/account.go` (new) — `registerAccount(srv)`, called from `MCP()` when `s.Account != nil`. It uses `mcp.AddTool` with an `Out` type `accountInfo{ID string \`json:"id"\`}`. Set `Tool.OutputSchema` explicitly: object, `id` string `minLength:1` `pattern:"\\S"`, `required:["id"]`, `additionalProperties:false`; the input schema is an empty object with `additionalProperties:false`. Title "Who is signed in". Description: "Use this when you need to know which TCG Replay account is signed in, for example to tell connected accounts apart. Do not use this to read a game or the player's details; it returns only an opaque account ID." `Meta: {"openai/profile": true, "securitySchemes": [{"type":"oauth2"}]}`. Add `get_account` to `readOnlyTools` (`annotations.go:10`). The handler calls `s.Account(ctx, req.Extra)`: on error it returns an `IsError` result with the message; on success it returns `structuredContent` `{id}` and a JSON text copy. It is not routed through `addWith`, so no place, no activity and no busy refusal, like `guide` (`guides.go:24-30`).
- `internal/usergames/mcp.go:20-49` — add `func (r *Registry) AccountID(_ context.Context, extra *mcp.RequestExtra) (string, error)`. It reads `hostauth.AccountFromToken(extra.TokenInfo)` per request and returns `acct.ID` (Google `sub`) or `errSignIn`, touching no entry.
- `cmd/tcgreplay/hosted.go:247-275` (`newHosted`) — pass `Account: reg.AccountID` to `mcpserver.Server`.
- `cmd/tcgreplay/hosted_test.go` (or new `hosted_account_test.go`) — sign in through `hostauthtest` and check:
  - `get_account` is listed with `_meta["openai/profile"] == true` and the output schema;
  - the call returns `structuredContent.id == "sub-1"`;
  - a second `h.connect(t)` gives the same id; `SetAccount(capSecond)` gives `sub-2`;
  - at `maxUsers=1` with another account holding the place, the call still answers.
- `internal/mcpserver/server_test.go` — the desktop `Server{}` does not list `get_account`.
- `site/src/pages/privacy.astro:108-111` — extend the "Your AI assistant" item: the playmat tells the assistant your Google account ID (never your email) so it can tell your accounts apart. Do not add `demo.gs`, `www` or "Plugin Directory".
- `site/e2e/privacy.spec.js` — assert the sentence is present in the outside-services section.

**Complexity:** Medium
**Token estimate:** ~35k tokens
**Agent strategy:** Single agent, sequential execution: tool and hook, then hosted test, then privacy page and its test.

### Task: Answer OpenAI's domain verification

**Requirement → repo/files:** "The domain is verified" → `tcgreplay`, `cmd/tcgreplay/hosted.go`, `.do/app.yaml`.

**File changes:**
- `cmd/tcgreplay/main.go:49-84` (`config`) — add `openAIChallenge string`, read in the hosted path from `os.Getenv("TCGREPLAY_OPENAI_CHALLENGE")`, trimmed. Environment only, no flag, so it never shows in a process listing.
- `cmd/tcgreplay/hosted.go:188-215` (`hostedHandler`) — take the token, through a new parameter or a small options struct. Register `mux.HandleFunc("GET /.well-known/openai-apps-challenge", …)` after `a.Routes(mux)` and before `mux.Handle("/", site.Handler())`. With a token it writes 200, `Content-Type: text/plain; charset=utf-8`, `Cache-Control: no-store` and exactly the token, with no newline. With none it returns `http.NotFound`. Update the handler's doc comment list of routes.
- `cmd/tcgreplay/hosted.go:382` (start-up log) — add "OpenAI domain token set" or "no OpenAI domain token (TCGREPLAY_OPENAI_CHALLENGE)", never the value (pattern: `adminText`, `:137`).
- `.do/app.yaml` (envs, after `TCGREPLAY_SIGNING_KEY`) — add `- key: TCGREPLAY_OPENAI_CHALLENGE`, `scope: RUN_TIME`, `type: SECRET`, no value. Comment: the token from OpenAI's plugin dashboard, set in the console, served at `/.well-known/openai-apps-challenge`; edits to this file apply only with `doctl apps update`.
- `cmd/tcgreplay/hosted_routes_test.go` — new test:
  - with the token in config, a plain GET (no cookie, no bearer) returns 200, `text/plain` and the exact body, and sets no cookie;
  - with an empty token it returns 404, not the website page;
  - the captured log (`captureLog`) never contains the token.
- `cmd/tcgreplay/hosted_test.go:1190-1221` (`TestAppSpecHealthCheckAndSecrets`) — add `TCGREPLAY_OPENAI_CHALLENGE` to the keys that must be SECRET with no value.

**Complexity:** Low
**Token estimate:** ~15k tokens
**Agent strategy:** Single agent, sequential execution.

### Task: Let the reviewer in past the player limit

**Requirement → repo/files:** "The reviewer always gets in" → `tcgreplay`, `internal/hostauth`, `internal/usergames`, `cmd/tcgreplay`, `.do/app.yaml`.

**File changes:**
- `internal/hostauth/store.go:400-408` (`issueTokens`) — include `Verified: acct.EmailVerified` in both the access and refresh `grantClaims`. Check that the refresh exchange path carries the account's verified flag forward (`refreshToken.account`).
- `internal/hostauth/oauth.go:446-473` — in `verify`, add `"verified": t.account.EmailVerified` to `TokenInfo.Extra`. In `AccountFromToken`, read it into `Account.EmailVerified` (absent means false). Check that the access-token record (`accessToken`) opens the sealed `Verified` claim (`seal.go:108`).
- `internal/usergames/usergames.go:25-42` (`Config`) — add `Reviewer string` (lower-cased, trimmed at construction). Add `func (r *Registry) isReviewer(acct hostauth.Account) bool { return r.reviewer != "" && acct.EmailVerified && strings.EqualFold(strings.TrimSpace(acct.Email), r.reviewer) }`.
- `internal/usergames/usergames.go:94-150` — in `Use`, skip the `r.full()` refusal when `isReviewer(acct)`. Record reviewer-ness on the entry (`e.reviewer = true`, refreshed from `acct` each `Use`). In `full()`, count only entries with `e.holds && !e.reviewer`. `Admit` and the busy message stay unchanged for everyone else. Do not log the email. If the observability plan has landed, use its recorder (player ID only); otherwise add no new log line.
- `internal/usergames/places_test.go` — unit tests:
  - at `MaxUsers=1`, held by A, the reviewer (verified) is admitted and C is refused;
  - the reviewer with `EmailVerified=false` is refused;
  - after the reviewer is admitted, A leaving lets C in (the reviewer is not counted);
  - an empty `Reviewer` behaves as today.
- `internal/hostauth/oauth_test.go` (or `seal_test.go`) — an access token issued for a verified account yields `AccountFromToken(...).EmailVerified == true`; a hand-sealed token without `v` yields false.
- `cmd/tcgreplay/main.go:49-84,160-170` — `config.reviewer` from `os.Getenv("TCGREPLAY_REVIEWER")` only, no flag.
- `cmd/tcgreplay/hosted.go:247-275` (`newHosted`) — pass `Reviewer: c.reviewer` into `usergames.Config`.
- `cmd/tcgreplay/hosted.go:382` — start-up log says "reviewer account set" or "no reviewer account (TCGREPLAY_REVIEWER)", never the address.
- `cmd/tcgreplay/hosted_cap_test.go` — new `TestHostedReviewerPastCap`, at `maxUsers=1`:
  - account one connects;
  - `SetAccount({ID:"sub-r", Email:"Reviewer@Example.com"})` with `c.reviewer="reviewer@example.com"`: the connector signs in, `import_log` with the website example (`site/src/content/example/battle-log.txt` and `testdata/decks/mega-lucario-ex-v12.txt`, via `readTestdata`) succeeds, and a move (e.g. `rewind` or `get_state` and a `draw` in review) succeeds;
  - the reviewer's board sign-in returns 200;
  - `SetAccount(capSecond)` still gets the busy page;
  - with `FakeGoogle.Claims` setting `email_verified=false` for the reviewer's address, the busy page;
  - the log contains neither address.
- `.do/app.yaml` — add `- key: TCGREPLAY_REVIEWER`, `scope: RUN_TIME`, `type: SECRET`, no value. Comment: the one Google account OpenAI's reviewer signs in with (no two-step verification); it may play even when `TCGREPLAY_MAX_USERS` is reached; set in the console.
- `cmd/tcgreplay/hosted_test.go:1208` — add `TCGREPLAY_REVIEWER` to the secret keys checked.
- `Makefile:95-103` (`docker-run`) — pass `-e TCGREPLAY_REVIEWER -e TCGREPLAY_OPENAI_CHALLENGE`, and mention both in the comment.

**Complexity:** Medium
**Token estimate:** ~45k tokens
**Agent strategy:** Medium: two agents in parallel, (a) the sign-in service token flag with its tests and (b) the registry reviewer logic with its unit tests. Then one agent wires the settings, app spec, Makefile and the hosted integration test.

### Task: Set the domain token and reviewer on the hosted playmat

**Requirement → repo/files:** "The domain is verified", "The reviewer always gets in" (deployment) → `tcgreplay` (`.do/app.yaml` applied by hand; values only in the console).

**What the person does:**
1. After both agent tasks are merged and deployed, open OpenAI's plugin dashboard, start the plugin's MCP connection step, and copy the domain-verification token.
2. In DigitalOcean App Platform (Settings > playmat > Environment Variables), set `TCGREPLAY_OPENAI_CHALLENGE` to the token and `TCGREPLAY_REVIEWER` to the reviewer's Google email, both encrypted. The reviewer account itself (with no two-step verification) is created by the maintainer, a spec non-goal.
3. Apply the updated app spec by hand (`doctl apps update <app-id> --spec .do/app.yaml`), since the deploy workflow never applies it, and let the playmat redeploy. Players' games are dropped on restart, as with any deploy.
4. Check the start-up log in Insights: "OpenAI domain token set" and "reviewer account set" appear, and neither value does.

**How it is checked:** `https://tcgreplay.app/.well-known/openai-apps-challenge` in a private window returns exactly the token, and OpenAI's dashboard marks the domain verified.

### Task: A listing icon without Pokémon trademarks

**Requirement → repo/files:** "The plugin package can be built" (icon), constraint "neither the name nor the icon uses a Pokémon trademark" → `tcgreplay`, `plugin/assets/`.

**File changes:**
- `plugin/assets/icon.svg` (new) — square 512×512 viewBox. Navy rounded-square background and cyan accents as in `docs/logo.png`. Three fanned plain card outlines with a circular replay arrow (or rewind chevrons) on the front card. No Poké Ball, no "Pokémon" text, no characters. Optionally the "TCG Replay" wordmark; OpenAI shows the display name beside the icon, so a mark-only icon reads better at 48 px.
- `plugin/assets/icon.png` (new) — rendered once with `rsvg-convert -w 512 -h 512 plugin/assets/icon.svg -o plugin/assets/icon.png`. Committed; well under 5 MiB.
- `docs/icon.png`, `docs/logo.png`, `site/public/icon.png` — unchanged (out of scope).

**Complexity:** Low
**Token estimate:** ~10k tokens
**Agent strategy:** Single agent, sequential execution. Render it, then view the PNG to confirm it reads at small sizes (e.g. also render 48 px to a scratch file).

### Task: Build the plugin package

**Requirement → repo/files:** "The plugin package can be built" → `tcgreplay`, `internal/plugin` (new), `cmd/tcgreplay-plugin` (new), `internal/appspec` (new, shared), `plugin/listing.json` (new), `Makefile`.

**File changes:**
- `internal/appspec/appspec.go` (new) — `EnvFields(yamlText, key string) map[string]string`, the env-entry line scan moved from `cmd/tcgreplay/hosted_test.go:1223-1246` (`appEnvFields`). Also `PublicURL(yamlText) (*url.URL, error)`, which requires `TCGREPLAY_PUBLIC_URL` to be a bare `https://host` origin. Unit tests with the real `.do/app.yaml` and broken copies.
- `cmd/tcgreplay/hosted_test.go:1191-1246` — use `appspec.EnvFields` instead of the local helper.
- `internal/plugin/plugin.go` (new) — `Sources`, `Build(src Sources) ([]byte, error)`, `Validate`. Composes:
  - `plugin.json`: `$schema https://agent-plugins.org/schemas/1.0.0/plugin.schema.json`, plus `name`, `version`, `description` (the subtitle), `homepage` (the public URL) and `repository` (the GitHub repo URL derived from the notice's help URL minus `/issues`);
  - `extensions["com.openai"].interface`: `displayName`, `shortDescription`, `longDescription` (listing description + "\n\n" + `notice.notice` + "\n\n" + credit lead and sources rendered "TCGdex (https://tcgdex.dev) and malie.io (https://malie.io)"), `developerName`, `category`, `capabilities`, `websiteURL=<public>`, `privacyPolicyURL=<public>/privacy`, `termsOfServiceURL=<public>/terms`, `supportURL=notice.help.url`, `defaultPrompt`, `brandColor`, `composerIcon:"./assets/icon.png"`, `logo:"./assets/icon.png"`;
  - `extensions["com.openai"].review.test_cases` from the listing, and `publication.release_notes` from the release notes text, trimmed;
  - `mcp.json`: `{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{"tcgreplay":{"type":"streamable-http","url":"<public>/mcp","extensions":{"com.openai":{"auth":{"type":"oauth","client":{"mode":"dcr"}}}}}}}`;
  - a ZIP with a fixed file order and fixed modification times, so it is reproducible: `plugin.json`, `mcp.json`, `assets/icon.png`.

  `Validate` refuses, naming the rule:
  - a name not matching `^[a-z0-9]+(-[a-z0-9]+)*$`;
  - a displayName or shortDescription over 30 characters, or a longDescription over 4000;
  - more than 3 defaultPrompt entries or any over 128 characters;
  - positive cases not exactly 5, or any missing description, prompt, tools_triggered or expected_behavior;
  - negative cases not exactly 3, or any missing description or prompt;
  - any URL not https or over 1024 characters;
  - an icon that is not a PNG, not square, outside 48 to 4096 px, or over 5 MiB (`image.DecodeConfig`);
  - forbidden keys anywhere in the listing source (`clientSecret`, `test_credentials`, `reviewer_instructions`, `apps`, `hooks`);
  - a listing source containing `http://` or `https://` URLs (other than the optional `review.demo_recording_url`, which names the externally hosted video), the notice text, or the support URL (these must come from the builder). Where a test case or the demo needs the playmat's address, the listing writes `{public}` (e.g. `{public}/docs/example`) and the builder replaces it with the App Platform address before validating; an unreplaced `{` placeholder is refused.
- `internal/plugin/plugin_test.go` (new) — builds from the real files (`../../.do/app.yaml`, `../../internal/notice/notice.json`, `../../plugin/listing.json`, `../../plugin/release-notes.md`, `../../plugin/assets/icon.png`). It checks:
  - the ZIP entries are exactly the three files;
  - displayName is "TCG Replay"; longDescription ends with the notice and credit; the four links are `https://tcgreplay.app`, `/privacy`, `/terms` and the GitHub issues URL;
  - `mcp.json` has url `https://tcgreplay.app/mcp`, streamable-http, oauth dcr;
  - two builds are byte-identical;
  - table-driven broken copies, one per rule.

  Until the review-material task adds the real test cases, the listing's `review` block holds placeholders that pass the count check. That task replaces them.
- `cmd/tcgreplay-plugin/main.go` (new) — flags `-root` (default `.`) and `-out` (default `bin/tcgreplay-plugin.zip`). Reads the files, calls `plugin.Build`, writes the ZIP, prints the path and size.
- `plugin/listing.json` (new) — `name:"tcgreplay"`, `version:"1.0.0"`, `displayName:"TCG Replay"`, `shortDescription` (≤30, e.g. "Replay and review TCG games"), `description` (plain prose: what TCG Replay does, import a TCG Live battle log and decklist, review and replay on a live board, drills; for players 13 and over; no comparisons, pricing or "Plugin"/"MCP" in the name). Also `developerName:"the tcgreplay project"`, `category:"Games"`, `capabilities`, `brandColor` from the site palette, 3 `defaultPrompt` entries (e.g. "Import my TCG Live battle log and review it with me", "Give me a numbers drill on my last game", "Play a practice game against me on the playmat"), and the `review` placeholders.
- `Makefile:1-10,108-110` — add `plugin` to `.PHONY` and a target `plugin: ; go run ./cmd/tcgreplay-plugin -out bin/tcgreplay-plugin.zip`, with a comment that it needs `internal/notice/notice.json` (privacy-terms work) and that the ZIP goes in OpenAI's plugin dashboard.

**Complexity:** Medium
**Token estimate:** ~50k tokens
**Agent strategy:** Medium: two parallel agents, (a) `internal/appspec` with the hosted test refactor and (b) `internal/plugin` with its tests against a fixture public URL. Then one agent adds the command, the listing source and the Makefile target, and runs the real build.

### Task: Write the review material

**Requirement → repo/files:** "The review material is ready", "It works end to end in ChatGPT" (cases cover the journey) → `tcgreplay`, `plugin/`, optionally `site/src/data/whats-new.yaml`.

**File changes:**
- `plugin/listing.json` (`review.test_cases`) — replace the placeholders. Positive cases (each with `description`, `prompt`, `tools_triggered`, `expected_behavior`), together covering connect, Google sign-in, import, board link, review and replay:
  1. Import the example game. The prompt pastes the decklist and battle log from `{public}/docs/example`, which the builder fills in as `https://tcgreplay.app/docs/example` (the prompt says to copy both from that page). Triggers `guide`, `import_log`. Expected: imports, gives the board link, summarises the game.
  2. Open the board and step through. Triggers `get_state`, `get_log`. Expected: describes the position at a named turn; the board link opens signed in.
  3. Review a turn. Triggers `guide` (review-game), `get_log`, `card_text`, `annotate`. Expected: coaching notes appear on the board timeline.
  4. Replay from a turn. Triggers `rewind`, `wait_for_turn`. Expected: a branch is created, the player plays on the board, and the assistant waits in the same reply.
  5. Restore from the record. Triggers `get_record`, `import_log`. Expected: the identical game is rebuilt.

  Negative cases (`description`, `prompt`):
  1. A battle log without a decklist: asks for the decklist and imports nothing.
  2. A request unrelated to card games: TCG Replay is not used.
  3. A request to look up live tournament results or prices on the web: declined, since the playmat reaches nothing outside.

  Keep prompts free of trademark claims; card names in the log are the reviewer's paste.
- `plugin/release-notes.md` (new) — first version, 1.0.0:
  - what the plugin does;
  - that every tool now has a title, labels and when-to-use descriptions, and that sign-in is with Google;
  - the measured tool list size;
  - "players already connected through a custom connection should open Plugins → TCG Replay → Refresh, then start a new chat".

  Must not say "Plugin Directory" if it is ever published on the website (it is not).
- `plugin/demo-script.md` (new) — a shot list for the video:
  - a new ChatGPT chat;
  - connect TCG Replay, sign in with Google;
  - positive cases 1 to 5 in order, with the exact prompts, then one negative case.

  It names the example page as the source of the log and decklist, and states the reviewer account must have no two-step verification.
- `plugin/SUBMITTING.md` (new) — the maintainer's checklist:
  1. Identity verification on the OpenAI Platform.
  2. `make plugin`.
  3. Upload the ZIP; fix any validation errors.
  4. The MCP step: set the domain token (the human task), connect, sign in, wait for the tool scan.
  5. Review details: the reviewer credentials and login URL `https://tcgreplay.app`.
  6. Record the video from `demo-script.md` and add its URL as `review.demo_recording_url` in `plugin/listing.json` (then rebuild), or in the dashboard if allowed.
  7. Run MCP Inspector first.
  8. Submit.
  9. After approval, publish and tell the launch-docs owner to switch `chatgptDirectory.listed`.

  It also says that after publication tool names, parameters and results must stay backwards compatible, and that listing changes need a new version and review.
- `internal/plugin/plugin_test.go` — the real-source test now also checks that each positive prompt or description mentions the example page or a step of the journey, and that the five cases' `tools_triggered` include `import_log`, `get_state`, `rewind` and `wait_for_turn`. Also that `demo-script.md` exists and names `https://tcgreplay.app/docs/example` (or the path built from app.yaml).
- `site/src/data/whats-new.yaml` — only if it exists when this task runs: add to the launch entry's `changes` "Every tool now has a title and clearer descriptions" and "ChatGPT can tell which account is signed in". Set `refresh: true`, and do not mention the Plugin Directory. If the file does not exist, skip: the launch-docs plan's launch entry covers it.

**Complexity:** Medium
**Token estimate:** ~35k tokens
**Agent strategy:** Single agent, sequential execution. It reads the final tool descriptions and the example game first so prompts and expected behaviour match the real tools.

## Testing Strategy

Per task:
- **Explicit labels on every tool.** Raw `tools/list` JSON read over the in-memory and Streamable HTTP transports. All three labels are present as booleans and match `readOnlyTools`/`replacingTools`. `_meta.securitySchemes` appears on every tool in a hosted-configured server and on none on the desktop. The existing "set names are real tools" check is kept.
- **Titles and descriptions for every tool.** The metadata guard (title, "Use this when", "Do not use", every property described, labels present) names each failing tool. A size test logs the byte count and enforces the ceiling. A desktop tool-name list check confirms the 45 names are unchanged. The existing description tests (`TestToolsSayWhenToUseThem`, `TestWaitToolDescribesDecisions`, pair tests) still pass.
- **Instructions that stand alone and waits inside the limit.** `TestInstructionsOpening` checks the first 512 bytes for the three statements and whole sentences. `still_waiting` has `keep_waiting:true` and a `next` naming `wait_for_turn`. The default and maximum caps are under 60 s and a larger cap is refused. Existing instruction tests pass; a lone `-race` TempDir failure is the known flake.
- **Tell ChatGPT who is signed in.** In a hosted test with the fake Google: `get_account` is listed with the profile marker, read-only labels and the output schema. Its id is the `sub`, the same on reconnect and different for another account, and it answers at a full cap. The desktop does not list it. The website privacy test checks the new sentence.
- **Answer OpenAI's domain verification.** A hosted route test checks the exact body, `text/plain`, no cookie, no sign-in, and 404 when unset. The log never contains the token. The app spec test checks the key is SECRET with no value.
- **Let the reviewer in past the player limit.** Registry unit tests: the reviewer is admitted at the cap and is not counted; an unverified match is refused; an empty setting behaves as today. A sign-in service test checks the token carries the verified flag and an old token reads as unverified. A hosted integration test at cap 1: the reviewer connects, imports the website example and plays, and gets a board sign-in; another account and an unverified match get the busy page; no address appears in the log. The app spec test checks the key is SECRET.
- **A listing icon without Pokémon trademarks.** The builder's icon validation (square PNG, 48 to 4096 px, under 5 MiB) runs on the real file. The look is a manual review.
- **Build the plugin package.** Unit tests on the real sources check the ZIP entries, name, links, notice and credit, `mcp.json`, and that two builds are byte-identical. Table-driven broken copies, one per rule. `internal/appspec` is tested on the real and broken app specs.
- **Write the review material.** The real-source build has exactly 5 positive and 3 negative cases with required fields, `{public}` is filled, the tools triggered include `import_log`, `get_state`, `rewind` and `wait_for_turn`, and the demo script exists naming the example page.
- **Set the domain token and reviewer on the hosted playmat (human).** Checked by the live challenge URL and OpenAI's dashboard.

Manual items (success metrics and reviews) are listed in plan.md's Testing Approach and go into the implementation test plan: package accepted, domain verified, MCP Inspector, end to end in ChatGPT, reviewer account live, existing assistants unaffected, icon and listing review, and tool list size accepted. The three success metrics (passes automated checks and reaches human review; any rejection not for missing metadata, material or connection; listed in the directory) are also manual.

## Project References

- Spec: `20261112090820-07d70cfb-chatgpt-submission-prep` (epic `20261111093348-e1b5c2b4-public-launch`).
- Plans:
  - `20261112083238-29247b36-privacy-terms-disclaimer`: `internal/notice/notice.json`, `/privacy`, `/terms`.
  - `20261111093353-8ff3589f-domain-move`: `TCGREPLAY_PUBLIC_URL`, mirrored moved server, whole-site scan.
  - `20261112090330-4ac2ea19-observability`: no email in logs, `TCGREPLAY_MAX_USERS`.
  - `20261112091756-c5b6d1fd-launch-docs`: listing flag, no "Plugin Directory" while unlisted, What's new.
- Designs: none.
- Knowledge (repo tier, store `tcgreplay`):
  - `conventions/skills-text-and-packaging.md`;
  - `learnings/publishing-to-chatgpt.md`;
  - `gotchas/chatgpt-tool-discovery.md`;
  - `gotchas/handoff-in-tool-results.md`;
  - `gotchas/go-sdk-sessions-and-auth.md`;
  - `gotchas/flaky-guide-instructions-test.md`;
  - `learnings/claude-ai-connector-oauth.md`.
- Repo root: `/home/nicj/code/github.com/nicholasjackson/tcgreplay`.

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

The description pass (High, ~90k) is the largest task. Split the drafting across two agents by tool group and integrate in one pass to avoid conflicting edits in `internal/mcpserver/server.go`. The instructions task depends on it for the same reason.

## Migration Notes

- **Access and refresh tokens.** Tokens issued before the reviewer change carry no verified flag and read as unverified. Only the reviewer is affected: they reconnect once to be recognised on tool calls. Board cookies already carry the flag.
- **New secrets.** `TCGREPLAY_OPENAI_CHALLENGE` and `TCGREPLAY_REVIEWER` are declared in `.do/app.yaml` with no value. The maintainer sets them in the App Platform console and applies the app spec by hand (`doctl apps update`); the deploy workflow never applies it.
- **ChatGPT custom connections.** Players already connected see the new titles, descriptions and the `get_account` tool only after refreshing the connection (Plugins → TCG Replay → Refresh, then a new chat).
- **After publication.** Tool names, parameters and results must stay backwards compatible, because OpenAI holds changed tools for re-review. Listing changes need a new package version and review.

## Performance Considerations

- The label encoder re-encodes the `tools/list` result once per call. The list is about 30 KB and is fetched only at connect and on refresh, so the cost is negligible.
- The tool list grows with the description pass; the size test holds it under a ceiling set from the measurement. Waits stay at the 15 s default, well inside 60 s.
- The reviewer's place is outside the cap, so peak memory can exceed the measured limit by one player's game. The observability plan sets `TCGREPLAY_MAX_USERS` with headroom.
