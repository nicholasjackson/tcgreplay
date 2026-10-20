---
created_date: "2026-10-20"
document_status: final
closed_date: "2026-10-20"
---

# Test plan: 20261020081616-39b3e658-guide-tool

The automated suite (`make test`) covers the guide list, reading guides, the unknown-name error, the instructions guide block, prompts, hosted-vs-desktop equality, guides while busy, the text conventions, the only-real-tools check and the removed `install` command. The items below need a person or a live client.

## 1. Success metric: a fresh claude.ai conversation finds and follows the guides unprompted

- **What to measure**: with only the playmat connected and no tcg skills uploaded, asking to import and review a game leads the assistant to read the `import-game` guide (and `opponent-deck`, which it points to), then the `review-game` guide, and to run the workflows, without the player naming a guide or a tool. Pass: both `guide` calls appear in the conversation's tool calls before the workflow steps run.
- **How**:
  1. Deploy the build to the hosted playmat. In claude.ai, open Settings → Connectors and make sure the tcgreplay custom connector is the only playmat source. Remove any uploaded tcg skills (the `tcg-import-game` … `tcg-numbers-drill` skills from before this change).
  2. Start a new conversation. Paste a battle log, for example `testdata/logs/2026-10-11_ledjo23_alakazam-donk.txt`, with the message "Import this game and review it with me".
  3. Expand the tool calls. Expect `guide` with `{"name":"import-game"}` before `import_log`, and `guide` with `{"name":"review-game"}` before the review starts. The review should ask the player's reasoning before judging.
- **Expected result**: both guides are read unprompted, and the import and review follow them (opponent list built, board link shared, review asks first). Fail if the assistant skips the guide and improvises, or asks the player which workflow to use.
- **Who / when**: the maintainer, after the first hosted deploy of this change.

## 2. Success metric: a workflow change reaches every client on its next connection, with no re-install

- **What to measure**: a change to a guide's text in a new release is what a client gets on its next connection, with nothing installed or uploaded. The automated tests already show that the served text is exactly the embedded guide and that no install path remains. This checks it end to end on the deployed playmat.
- **How**:
  1. Note a distinctive sentence from the deployed `review-game` guide: in a claude.ai conversation with the connector, ask "call the guide tool for review-game and quote its first paragraph".
  2. Deploy a release whose `internal/guides/review-game.md` differs, even by one word in that paragraph. On a test deploy, a deliberate temporary edit is enough; revert it afterwards.
  3. Start a new claude.ai conversation, with nothing re-uploaded and the connector unchanged, and ask again. Also connect Claude Code to the desktop build of the same release (`make install`, restart) and run `/mcp`, then call `guide` for `review-game`.
- **Expected result**: both clients return the new text on their first call after the release. Zero manual steps on the client side.
- **Who / when**: the maintainer, on the first release after this change that touches a guide.

## 3. Manual review: side-by-side read, no rule or step dropped (acceptance criterion "Same workflows as before")

- **What to look at**: each old skill against its guide. The old text is at the base commit of this change; the new text is in `internal/guides/`.
- **How**: for each pair, run `git diff -M <base> -- skills/tcg-<x>/SKILL.md internal/guides/<x>.md` (or `git show <base>:skills/tcg-<x>/SKILL.md` beside `internal/guides/<x>.md`) for `import-game`, `opponent-deck`, `review-game`, `replay-drill`, `before-you-move` and `numbers-drill`. `<base>` is the commit this branch started from (`git merge-base HEAD main` before merging).
- **What to look for**: the body differs only by the frontmatter (a short one-line `description` replaced the long skill description) and these wording changes: skill names replaced by "the `<name>` guide", "This skill" replaced by "This guide", and "If another skill records results" replaced by "If something else records results". Every heading, rule and numbered step from the skill is still present. Anchors such as "Ask before judging", "Put the board where you are talking" and the `Finding (` recording format are also checked by `internal/guides/guides_test.go`.
- **Pass**: no rule, step or example is missing or changed in meaning.
- **Who / when**: the reviewer of this change, before merging.

## 4. Manual review: guides appear as prompts in Claude Code

- **What to look at**: Claude Code connected to the dev playmat (`tcgreplay-dev` in `.mcp.json`) built from this branch.
- **How**: `make build`, then start Claude Code in the repo and approve the `tcgreplay-dev` server. Type `/` and look for the server's prompts. Claude Code lists MCP prompts as slash commands named after the server and prompt, such as `/mcp__tcgreplay-dev__review-game`. Choose the review prompt.
- **What to look for**: six prompts named `import-game`, `opponent-deck`, `review-game`, `replay-drill`, `before-you-move` and `numbers-drill`, each described by its one-line "use when". The review prompt inserts the review guide's full text, the same text the `guide` tool returns for `review-game`.
- **Pass**: all six are listed, and the review prompt's text matches the guide's.
- **Who / when**: the maintainer, before merging or right after.
