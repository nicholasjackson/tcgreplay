---
created_date: "2026-11-19"
document_status: final
closed_date: "2026-11-19"
---

# Test plan: 20261112090820-07d70cfb-chatgpt-submission-prep

Manual checks for what the automated suites cannot cover: OpenAI's upload form and review, the deployed playmat, ChatGPT itself, and the maintainer's judgement. The automated checks run with `make test`, and the package with `make plugin` (`internal/plugin`, `internal/appspec`, `internal/mcpserver` metadata guards, `cmd/tcgreplay` hosted tests).

Run the checks in this order: 0 (deploy setup, the deferred human task), then 1 to 8, then the three success metrics, which follow the submission.

## 0. Deploy setup (deferred human task: "Set the domain token and reviewer on the hosted playmat")

The user deferred this task to the maintainer, to do after deploy. Its two criteria are not ticked in the plan until this is done.

- **Who / when**: the maintainer, after this branch is merged and deployed, during step 4 of `plugin/SUBMITTING.md`.
- **How**:
  1. In OpenAI's plugin dashboard, start the plugin's MCP connection step and copy the domain-verification token.
  2. In DigitalOcean App Platform (Settings > playmat > Environment Variables), set `TCGREPLAY_OPENAI_CHALLENGE` to the token and `TCGREPLAY_REVIEWER` to the reviewer's Google email, both encrypted. The reviewer account must have no two-step verification.
  3. Apply the app spec by hand, since the deploy workflow never does: `doctl apps update <app-id> --spec .do/app.yaml`. Let the playmat redeploy. Players' games are dropped on restart, as with any deploy.
  4. In Insights, read the start-up line `tcgreplay <version> hosted: listening on ...`.
- **Expected result**: the start-up line contains `OpenAI domain token set` and `reviewer account set`, and contains neither the token nor the reviewer's address. `git grep` for the token and the email in the repository finds nothing.

## Manual reviews and checks

### 1. Domain verified
- **What**: OpenAI accepts the domain.
- **How**: in a private browser window, open `https://tcgreplay.app/.well-known/openai-apps-challenge`. Also run `curl -si https://tcgreplay.app/.well-known/openai-apps-challenge`. Then press verify in OpenAI's dashboard.
- **Expected result**: status 200, `Content-Type: text/plain; charset=utf-8`, `Cache-Control: no-store`, and a body that is exactly the token with no trailing newline (`curl -s ... | xxd | tail -1` shows no `0a`). No sign-in is asked for. The dashboard marks the domain verified. If it does not, check that Cloudflare is not caching or rewriting the path.
- **Who / when**: the maintainer, after step 0.

### 2. Package accepted
- **What**: the built ZIP passes OpenAI's upload validation.
- **How**: run `make plugin`, then upload `bin/tcgreplay-plugin.zip` in OpenAI's plugin dashboard.
- **Expected result**: no validation errors. The listing preview shows the name "TCG Replay", the subtitle "Replay and review TCG games", the new icon, and a long description ending with the fan-project notice and the TCGdex/malie.io credit. The website (`https://tcgreplay.app`), privacy (`/privacy`), terms (`/terms`) and support (GitHub issues) links each open. If a field is rejected, for example the `Games` category, the `Interactive` capability, `brandColor`, or the same PNG used as `composerIcon` and `logo`, fix it in `plugin/listing.json` or `internal/plugin/plugin.go` and rebuild. If OpenAI rejects the name, the address or the notice wording, stop and ask the user, since the spec fixes those.
- **Who / when**: the maintainer, at submission (SUBMITTING.md step 3).

### 3. MCP Inspector
- **What**: the deployed server's tool list and sign-in work in a neutral client.
- **How**: run `npx @modelcontextprotocol/inspector`. Connect to `https://tcgreplay.app/mcp` with Streamable HTTP, complete Google sign-in through its Auth settings, list the tools, then call `get_account`, `import_log` (the decklist and battle log from `https://tcgreplay.app/docs/example`, as `deck` and `log`) and `get_state`.
- **Expected result**: there are 46 tools. Each has a title and `readOnlyHint`, `destructiveHint` and `openWorldHint` as explicit true or false values, plus `_meta.securitySchemes: [{type: oauth2}]`. `get_account` carries `_meta["openai/profile"]: true` and an output schema with only a required `id`. Calling `get_account` returns `structuredContent` `{"id": "<your Google sub>"}`. `import_log` returns a summary and a board link. `get_state` returns the board.
- **Who / when**: the maintainer, before submitting (SUBMITTING.md step 7).

### 4. End to end in ChatGPT
- **What**: the whole journey and the eight test cases.
- **How**: in a new ChatGPT chat, connect TCG Replay and sign in with Google. Then run each case in `plugin/listing.json` (`review.test_cases`) in order, following `plugin/demo-script.md`.
- **Expected result**: each of the five positive cases produces its `expected_behavior`: import with summary and board link; the board link opens signed in and the position is described; coaching notes appear on the timeline; rewind makes a branch and the assistant waits in the same reply; the record restores the identical game. Each of the three negative cases goes as its description says: the decklist is asked for and nothing is imported; TCG Replay is not used for banana bread; it is not used for a tournament or price lookup. No errors appear.
- **Who / when**: the maintainer, before submitting and again while recording the demo video.

### 5. Reviewer account
- **What**: the configured reviewer gets in at the player limit, and others do not.
- **How**: lower `TCGREPLAY_MAX_USERS` to 1 in the console (a restart only), or wait until the live playmat is full. Sign in and play with a non-reviewer account so it holds the place. Then, from another browser or chat, connect with the reviewer account, import the example game and make a move, and open its board link. Then try a third, non-reviewer account. Restore `TCGREPLAY_MAX_USERS` afterwards.
- **Expected result**: the reviewer connects, imports, plays and opens the board. The third account gets the "tcgreplay is busy" page (503). The reviewer's address does not appear in the Insights logs. A reviewer connected before this deploy must reconnect once, because older tokens carry no verified flag.
- **Who / when**: the maintainer, after step 0 and before submitting.

### 6. Existing assistants unaffected
- **What**: Claude's connections work exactly as before.
- **How**: in claude.ai (custom connector to `https://tcgreplay.app/mcp`) and in Claude Code (`claude mcp add --transport http tcgreplay https://tcgreplay.app/mcp`), sign in, paste the example decklist and battle log, import, open the board and play a turn in review mode.
- **Expected result**: sign-in, import, the board link and moves behave as before, with no new prompts or errors. The tool names are the same 45 plus `get_account`.
- **Who / when**: the maintainer, after deploy.

### 7. Icon and listing review
- **What**: the listing is fit to submit.
- **How**: open `plugin/assets/icon.png` (and view it at 48 px, for example with `rsvg-convert -w 48 plugin/assets/icon.svg -o /tmp/icon48.png`). Read `plugin/listing.json`'s `displayName`, `shortDescription`, `description` and `defaultPrompt`, and `plugin/release-notes.md`.
- **Expected result**: there is no Poké Ball or other Pokémon trademark in the name or the icon; the icon reads clearly at 48 px. Nothing targets under-13s (the description says "For players aged 13 and over"), and there are no comparisons with other products and no pricing. The maintainer approves the icon and text, or edits `plugin/` and rebuilds. The website and board still use the old icon, which shows a Poké Ball; replacing it is out of scope and is the maintainer's decision.
- **Who / when**: the maintainer, before submitting.

### 8. Tool list size
- **What**: ChatGPT accepts the tool list size.
- **How**: the measured size is in `plugin/release-notes.md` (37,959 bytes across 46 hosted tools; `go test ./internal/mcpserver -run TestToolListSize -v` prints it, and the ceiling is 41,000). In ChatGPT, after connecting (or Plugins → TCG Replay → Refresh for an existing custom connection), start a new chat and ask what TCG Replay can do.
- **Expected result**: the tools appear, with no size or truncation error, and the new descriptions are used. If a size error appears, stop and ask the user whether to shorten descriptions; do not cut the "Do not use" sentences.
- **Who / when**: the maintainer, during check 4.

## Success metrics

### The submission passes OpenAI's automated checks and reaches human review
- **How**: submit the package (SUBMITTING.md step 8) after checks 1 to 8 pass, then watch the dashboard's submission status.
- **Expected result**: the status moves past automated checks to "in review", with no automated rejection.
- **Who / when**: the maintainer, at submission.

### If the listing is rejected, the reason is not missing metadata, missing material or a broken connection
- **How**: read OpenAI's review email or the dashboard's rejection notes.
- **Expected result**: no rejection cites a missing tool title, description or label, missing test cases, release notes, demo video or privacy, terms or support links, a failed domain verification, or a failed connection or sign-in. Any other reason, for example Pokémon content, is recorded and handled separately.
- **Who / when**: the maintainer, on OpenAI's decision.

### TCG Replay is listed in ChatGPT's Plugin Directory
- **How**: after approval, publish the listing from the dashboard (SUBMITTING.md step 9), then search for "TCG Replay" in ChatGPT's plugin directory from an account that has never connected it.
- **Expected result**: TCG Replay appears and connects with Google sign-in. Then tell the launch-docs owner to switch `chatgptDirectory.listed` on.
- **Who / when**: the maintainer, after approval.
