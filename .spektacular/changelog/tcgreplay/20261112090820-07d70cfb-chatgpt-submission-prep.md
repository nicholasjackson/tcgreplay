---
created_date: "2026-11-19"
document_status: draft
project: tcgreplay
spec: 20261112090820-07d70cfb-chatgpt-submission-prep
plan: 20261112090820-07d70cfb-chatgpt-submission-prep
---

# ChatGPT submission prep

The playmat's tools now explain themselves to any assistant: each has a title, says when to use it and when not to, describes every input, and states plainly whether it only reads, whether it can replace your game, and that it reaches nothing outside the playmat. On the hosted playmat, ChatGPT can tell which account is signed in, OpenAI can verify the domain, and one configured reviewer account can play even when the playmat is full. The project can now build the plugin package and holds the review material for submitting TCG Replay to ChatGPT. Claude users see the same tools as before, only better described.

> Derived from project tcgreplay (local), spec/plan 20261112090820-07d70cfb-chatgpt-submission-prep. See the project-level record for the full feature.

## What changed in this repo

- **Tool metadata** (`internal/mcpserver`):
  - Every tool is built through one helper, `Server.tool(name, title, desc)`. Each has a title and a "Use this when … / Do not use …" description, and every argument field gained a description.
  - The `explicitLabels` `tools/list` middleware writes `readOnlyHint`, `destructiveHint` and `openWorldHint` as explicit booleans. In hosted mode each tool also carries `_meta.securitySchemes: [{type: oauth2}]`.
  - New guards: `TestToolMetadataComplete`, `TestToolAddedWithoutMetadataFails`, `TestToolListSize` (ceiling 41,000 bytes; hosted 37,959 bytes, 46 tools) and `TestDesktopToolNamesUnchanged`.
- **Instructions and waits**: the instructions open with `openingInstructions`, which stands alone in under 512 bytes. Tests pin the 15 s default wait (`defaultWaitCap`), the 50 s maximum, and the "call again" result.
- **Profile tool**: `get_account` (`internal/mcpserver/account.go`) is a hosted-only tool backed by `usergames.Registry.AccountID`. It returns the Google `sub` as `structuredContent` under an output schema that allows only `id`. The privacy page says the assistant is given the Google account ID.
- **Domain verification**: `GET /.well-known/openai-apps-challenge` serves `TCGREPLAY_OPENAI_CHALLENGE` exactly, as plain text, and returns 404 when it is unset.
- **Reviewer past the limit**:
  - `TCGREPLAY_REVIEWER` is passed to `usergames.Config.Reviewer`. A verified match skips the busy refusal and is not counted in the cap.
  - Access and refresh tokens seal the verified flag, and `hostauth.AccountFromToken` returns it as `EmailVerified`.
  - Both new settings are declared as secrets in `.do/app.yaml`, passed through by `make docker-run`, and described in the start-up log without their values.
- **Plugin package**: `make plugin` runs `cmd/tcgreplay-plugin`, which calls `internal/plugin.Build` and validates against OpenAI's limits. The new shared reader `internal/appspec` reads `.do/app.yaml` and replaces the hosted tests' own scanner.
- **Listing and review material** (`plugin/`): the listing source (`listing.json`, with the test cases), `release-notes.md`, `demo-script.md`, `SUBMITTING.md`, and the new icon (`assets/icon.svg`, `assets/icon.png`). The website and board icons are unchanged.

## Why

The hosted playmat is TCG Replay's connection for every assistant, so OpenAI's review rules apply to it directly: labelled and described tools, a profile tool, a verifiable domain and a reviewer who can always get in. The package and review material live here because they are built from this repo's own sources: the public address in the App Platform definition and the fan-project notice. That way the listing cannot drift from the website.

Still to do by the maintainer after deploy: set `TCGREPLAY_OPENAI_CHALLENGE` and `TCGREPLAY_REVIEWER` in the App Platform console, apply `.do/app.yaml` with `doctl apps update`, and confirm the live challenge path. See `plugin/SUBMITTING.md`.
