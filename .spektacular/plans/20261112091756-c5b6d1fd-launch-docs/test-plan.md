---
created_date: "2026-11-19"
document_status: final
closed_date: "2026-11-19"
---

# Test plan: 20261112091756-c5b6d1fd-launch-docs

These are the manual checks for the launch docs: the spec's acceptance criteria that depend on the real assistants, the two success metrics, the docs wording review, and the deferred Plugin Directory switch. The automated checks run with `make site-test` (site/e2e/connect.spec.js, docs-index.spec.js, whats-new.spec.js, site.spec.js) and are not repeated here.

All page addresses below are on the deployed website, `https://tcgreplay.app`. Run every check after this change has merged to `main` and deployed (every push to `main` deploys).

## 1. Custom ChatGPT steps work (acceptance criterion "Custom steps work")

- **What to measure**: following the ChatGPT custom connection steps on `https://tcgreplay.app/docs/connect#chatgpt` connects TCG Replay, and its tools are available in a chat. The plans the page names match OpenAI's help page on the launch day.
- **How**:
  1. Open OpenAI's help article "Developer mode and full MCP apps in ChatGPT" (https://help.openai.com/en/articles/12584461) and note which plans it says can add a custom MCP server with write actions.
  2. Compare that list with the plans paragraph at the top of the page's ChatGPT section. The page says "Business, Enterprise or Edu" (the text comes from `chatgptPlans` in `site/src/lib/listing.ts`).
  3. Use an account on one of the named plans: ChatGPT on the web, with workspace admin or owner rights on Business. Follow the six numbered custom steps exactly as written:
     - Plugins, then **+**, then **Add custom MCP server**;
     - name `tcgreplay`, with Server URL `https://tcgreplay.app/mcp`;
     - **OAuth**;
     - **I understand and want to continue**, then **Create as a plugin**;
     - **Install**, then the Google sign-in;
     - type `@` in a new chat.
  4. In a new chat, paste the page's check prompt, "What can the tcgreplay playmat do?".
- **Expected result**:
  - Every step's button and field names match what ChatGPT shows.
  - The plugin installs, and Google sign-in returns to ChatGPT.
  - ChatGPT lists the playmat's coaching workflows.
  - The plan list matches OpenAI's article.
- **If it fails**:
  - If a named plan cannot connect, or a plan the page leaves out can, edit `chatgptPlans` in `site/src/lib/listing.ts` and the expectation in `site/e2e/connect.spec.js` ("names the plans…" test) to match what you saw.
  - If a step's wording differs, edit the custom `<ol>` in `site/src/pages/docs/connect.astro`.
  - If no individual plan can connect at all, stop and decide how the launch should present ChatGPT (plan Open Questions).
- **Who / when**: the maintainer, on launch day, with a real ChatGPT account.

## 2. Refresh steps work (acceptance criterion "Refresh steps")

- **What to measure**: after a release that adds or changes a tool, following the refresh steps at `https://tcgreplay.app/docs/connect#refresh` makes the new or changed tool appear in each of claude.ai, ChatGPT and Claude Code.
- **How**:
  1. Before the release, connect all three assistants using the page's steps, and note the tool list each one shows (for example, ask "List the tcgreplay tools you can use.").
  2. Deploy a release that adds a tool or changes a tool's description.
  3. In each assistant, check whether the change shows without a refresh. Then follow its refresh steps as written:
     - claude.ai: Settings → Connectors, remove tcgreplay, add it again with `https://tcgreplay.app/mcp`, connect, start a new chat.
     - ChatGPT: Plugins, then tcgreplay, then **Refresh**, then a new chat.
     - Claude Code: `/mcp`, then tcgreplay, then **Reconnect**, re-authenticating if asked; if the tool is still missing, quit and restart.
  4. Ask the same question again.
- **Expected result**: in all three assistants, the new or changed tool appears after the refresh steps.
- **If it fails**: if a step does not bring the tool in, find the step that does and update that assistant's `<h3>` list in `site/src/pages/docs/connect.astro`. If nothing works in an assistant, stop and decide how the docs should handle it (plan Open Questions).
- **Who / when**: the maintainer, at the first release after launch that changes a tool, or earlier on a test deployment with a deliberately changed tool description.

## 3. Directory steps after approval (deferred task "Switch on the Plugin Directory route", acceptance criterion "Directory steps after approval")

The task was skipped in this run, deferred to the maintainer: live setup after deploy. It waits on OpenAI approving the listing submitted by chatgpt-submission-prep.

- **What to do, once OpenAI approves and the plugin is published**:
  1. In ChatGPT, open **Plugins**, search for "TCG Replay" and connect it. Note the exact names of the search box, the listing and the add/connect button.
  2. In `site/src/pages/docs/connect.astro`, adjust the directory steps inside both `{chatgptDirectory.listed && …}` blocks (the ChatGPT section and the refresh section's note) to match what ChatGPT showed.
  3. In `site/src/lib/listing.ts`, set `listed: true`.
  4. Add a What's new entry at the top of `site/src/data/whats-new.yaml`, dated the switch day, with:
     - `title: TCG Replay is in ChatGPT's Plugin Directory`
     - `refresh: false`
     - the change line "TCG Replay is in ChatGPT's Plugin Directory: see [Get started](/docs/connect)."
  5. Run `make site-test`. In the listed state the connecting-page tests assert that the directory steps come first and the custom steps follow, and the whole-site scan's "no Plugin Directory" rule switches off. Then merge so it deploys.
- **Expected result**:
  - `make site-test` passes.
  - On the live page, the ChatGPT section's first numbered steps find "TCG Replay" in the Plugin Directory and connect it, and following them in ChatGPT connects TCG Replay.
  - The custom steps follow under "Or add it yourself as a custom connection".
  - The refresh section says directory installs update on their own.
  - The What's new page shows the new entry first.
- **Who / when**: the maintainer, after OpenAI's approval email, when they choose to switch.

## 4. Success metric: no GitHub issue asks a question the docs already answer

- **What to measure**: in the first month after launch, the number of GitHub issues asking a ChatGPT-connection or missing-tools question that the docs already answer. Target: 0.
- **How**: one month after launch day, review every issue opened since launch at https://github.com/nicholasjackson/tcgreplay/issues (also filter with `is:issue created:>=<launch-date>`). For each issue about connecting ChatGPT, plans, or tools that seem missing after an update, check whether `https://tcgreplay.app/docs/connect` (the ChatGPT section or `#refresh`) or `https://tcgreplay.app/docs/whats-new` already answered it on the date it was opened.
- **Expected result**: no such issue was already answered by the docs. For any that was, record it and improve the page so the answer is easier to find (wording, or a link from the troubleshooting lines).
- **Who / when**: the maintainer, one month after launch.

## 5. Success metric: every player-visible release adds a What's new entry

- **What to measure**: every deploy to `main` since launch that changed what players see has an entry in `site/src/data/whats-new.yaml`. Target: 100%.
- **How**: list the merges since launch with `git log --merges --since=<launch-date> --oneline main`, and the changes outside `cardsets/` with `git log --since=<launch-date> --stat main -- . ':!cardsets'`. Decide which were player-visible: website pages, the board, tools, guides, the playmat's instructions, or limits. Check each against the entries on `https://tcgreplay.app/docs/whats-new`, and check that changes to tools, tool descriptions, guides or instructions are marked "Refresh needed". The convention in the knowledge base (`conventions/whats-new-entries.md`) says what counts.
- **Expected result**: every player-visible release has an entry, and its refresh answer is right.
- **Who / when**: the maintainer, monthly, and at each release review.

## 6. Manual review: docs wording

- **What to look at**:
  - `https://tcgreplay.app/docs/connect`: the ChatGPT section, the refresh section and the troubleshooting lines.
  - `https://tcgreplay.app/docs/whats-new`: the launch entry.
  - `https://tcgreplay.app/docs`: the docs index and the sidebar.
- **What to look for**:
  - The pages read clearly, in the docs' plain voice.
  - The launch entry lists what actually shipped at launch: the new address and move note, the ChatGPT steps, What's new with the refresh steps and docs index, the clearer tool names with the new account tool, the terms and privacy pages with footer links, and the board's About panel. Remove any line for something that did not ship, and add any player-visible launch change that is missing.
  - The launch entry's date (`2026-11-19`, the day this change was built) is the real launch day. If not, correct `date:` in `site/src/data/whats-new.yaml`.
  - No visible text has words run together, such as "needed,refresh" (Astro drops the space before an inline element that starts a new source line; see knowledge `gotchas/astro-inline-whitespace.md`).
- **Passing**: the maintainer is happy with the wording, and the launch entry and its date are accurate.
- **Who / when**: the maintainer, before launch.
