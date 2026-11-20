---
created_date: "2026-11-19"
document_status: final
closed_date: "2026-11-19"
---

# ChatGPT submission prep

## What was built

The hosted playmat now meets OpenAI's plugin review rules, and the project holds everything a ChatGPT Plugin Directory submission needs, built from sources it already keeps.

**Tools an assistant can understand.**
- Every tool has a readable title and a description that opens "Use this when …" and says when not to use it, pointing at the right alternative. Every input is described.
- Every tool states three labels, each as an explicit true or false: whether it only reads, whether it can replace the player's game, and that it never reaches outside the playmat. Only `new_game`, `import_log` and `set_opponent_deck` are destructive. The MCP SDK cannot send a false "read only" label, so a `tools/list` receiving middleware writes the labels itself. It works on the desktop's in-memory transport and on hosted Streamable HTTP alike.
- In hosted mode every tool declares the oauth2 security scheme.
- A raw-wire test fails if any tool lacks a title, a label, either description sentence or an input description. A size test holds the tool list under a 41,000-byte ceiling. The hosted list measures 37,959 bytes across 46 tools; the desktop list is 35,057 bytes across 45.
- The instructions open with a 482-byte paragraph that stands alone: what TCG Replay is, the rule to refer to cards by instance ID, and where the coaching guides are.
- Tests pin that a turn wait returns within its 15 s default (at most 50 s, under ChatGPT's 60 s limit), with a "call again" result when the turn isn't over.

**Identity, domain and reviewer.**
- A new hosted-only tool, `get_account`, is OpenAI's profile tool. It is read-only, takes no input, is marked `openai/profile`, and returns only the player's Google account ID, which is the same on every connection. It takes no place and answers even when the playmat is full. The privacy page says the assistant is given this ID, never the email, to tell accounts apart.
- `GET /.well-known/openai-apps-challenge` answers OpenAI's domain verification with exactly the token in `TCGREPLAY_OPENAI_CHALLENGE`, as plain text, with no sign-in. It returns 404 when the token is unset.
- `TCGREPLAY_REVIEWER` names one Google account. When Google has verified its email, it is admitted past the player limit at sign-in and on every tool call, and its place is not counted against the limit. To make tool calls recognisable, access and refresh tokens now carry the verified flag.
- Both settings are environment-only secrets declared without a value in `.do/app.yaml`. The start-up log says only whether each is set.

**The plugin package and review material.**
- `make plugin` builds `bin/tcgreplay-plugin.zip`, containing `plugin.json`, `mcp.json` and a new trademark-free icon. Two builds from the same sources are byte-identical. The builder takes the address from `TCGREPLAY_PUBLIC_URL` in `.do/app.yaml`, and the fan-project notice, card-data credit and support link from `internal/notice/notice.json`. It refuses, naming the rule, anything OpenAI's form would reject, and any address, notice text or secret typed into the listing source.
- `plugin/` holds the listing source, five positive and three negative test cases built on the website's example game, release notes (with the refresh step for players already connected), a demo video script and the maintainer's submission checklist.

## Why it matters

Submitting TCG Replay to ChatGPT's Plugin Directory lets ChatGPT users find the playmat and connect without setting up a custom connection. OpenAI's review rejects plugins with unlabelled or undescribed tools, a missing profile tool, an unverified domain, a reviewer who cannot get in, or missing test cases and release notes. This work removes each of those causes, so a rejection, if one comes, is about the content rather than the submission. Claude's connections are unchanged: every change to the tool list is descriptive or additive, and the desktop keeps exactly its 45 tools.

## Deviations from the plan

- **Deferred human task.** "Set the domain token and reviewer on the hosted playmat" was deferred by the user to the maintainer, to do after deploy. The goal check judged both criteria not met, the shortfall was accepted, and nothing is ticked. The steps are item 0 of the test plan and step 4 of `plugin/SUBMITTING.md`.
- **Label encoder writes declared labels.** The encoder writes the labels each tool declared, rather than filling them from the name sets for every tool, so the metadata guard can catch a tool registered without them. This was found on the goal check's first attempt in the descriptions task.
- **Extra tests.**
  - The desktop's 45-name guard was added in the labels task.
  - `TestHostedReviewerToolCallsPastCap` lets the reviewer's place idle out so that its tool call has to rely on the token's verified flag. Without that, the claim is never exercised.
  - The builder's command and the review material have their own tests (`internal/plugin/review_test.go`, `cmd/tcgreplay-plugin/main_test.go`).
- **Small substitutions.**
  - `appspec.EnvFields` returns values as written, as the old test helper did. Only `PublicURL` unquotes.
  - `hostedHandler` takes the challenge token as a new parameter.
  - The wait default became the named constant `defaultWaitCap`.
  - `brandColor` is the icon's navy, `#14306b`.
  - The release notes placeholder was created in the builder task.
- **No What's new entry.** There is no What's new list yet (`site/src/data/whats-new.yaml`), so the launch-docs work's launch entry covers these changes.
- **Size.** The tool list grew from 26,149 bytes (desktop, before) to 37,959 bytes (hosted, after, including `get_account`). That is below the plan's ~40,000-byte stop point, so no question was raised.
- **Sequencing.** Several tasks ran in one context rather than through the parallel agents the plan suggested.
