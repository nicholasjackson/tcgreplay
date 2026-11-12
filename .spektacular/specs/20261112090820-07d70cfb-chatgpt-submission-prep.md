---
created_date: "2026-11-12"
document_status: final
closed_date: "2026-11-12"
epic: 20261111093348-e1b5c2b4-public-launch
---

# Feature: 20261112090820-07d70cfb-chatgpt-submission-prep

## Overview

TCG Replay is submitted to ChatGPT's Plugin Directory as "TCG Replay", so ChatGPT users can find it and connect without setting up a custom connection. That means making the playmat's connection meet OpenAI's review rules (every tool clearly described and labelled for whether it changes anything) and preparing what the submission asks for: domain verification, the plugin package, a reviewer account that always gets in, test cases, release notes and a demo video. The listing may still be rejected over the Pokémon card content; that risk is accepted.

## Requirements

- [ ] **Every tool is labelled**
  Every tool explicitly states whether it only reads, whether it can destroy or replace a player's game, and whether it reaches outside the playmat, with each of the three stated as yes or no, never left out.
- [ ] **Every tool is described for an assistant**
  Every tool has a readable title, a description that says when to use it and when not to, and a description of each of its parameters.
- [ ] **The instructions survive being cut short**
  The opening of the playmat's instructions to assistants says, on its own, what the playmat is, that cards are referred to by their instance IDs, and where the coaching guides are, so an assistant that reads only the opening (see Constraints) still has them.
- [ ] **Waiting returns in time**
  Waiting for the other player's turn always returns within ChatGPT's time limit for a tool call (see Constraints), and tells the assistant to wait again if the turn isn't over.
- [ ] **ChatGPT can tell who is signed in**
  ChatGPT can ask the playmat who the signed-in player is and gets an identifier that stays the same for that player across connections, provided the way OpenAI's plugin rules require (see Constraints).
- [ ] **The domain is verified**
  tcgreplay.app answers OpenAI's domain verification with the token the maintainer gets from OpenAI.
- [ ] **The plugin package can be built**
  The project produces the plugin package OpenAI accepts, with the name and address set in Constraints, and carrying the icon, a description that includes the fan-project notice, and the website, privacy, terms and support links.
- [ ] **The reviewer always gets in**
  The reviewer's Google account can sign in and play even when the playmat is serving its maximum number of players.
- [ ] **The review material is ready**
  Five positive and three negative test cases, release notes and a script for the demo video are written in the project, using a sample battle log the reviewer can paste.
- [ ] **It works end to end in ChatGPT**
  In a new ChatGPT chat, a player can connect TCG Replay, sign in with Google, import the sample battle log, open the board link, and review and replay the game; the positive test cases cover this whole journey.

## Constraints

- The listing is named "TCG Replay", and neither the name nor the icon uses a Pokémon trademark (the maintainer's decision and OpenAI's IP rules).
- The plugin connects to `https://tcgreplay.app/mcp`; a published plugin's address can't change afterwards (OpenAI).
- After publication, tool names, parameters and results stay backwards compatible, because OpenAI holds changes for re-review.
- OpenAI's rules as researched on 2026-11-07: a tool call must return within 60 seconds; an assistant may read only the first 512 characters of a server's instructions; identity is given through a profile tool marked as such; domain verification is served at `/.well-known/openai-apps-challenge` (OpenAI's own path name); every tool states its read-only, destructive and open-world hints.
- Sign-in stays the playmat's existing Google sign-in and assistant connection flow, and assistants that connect today (claude.ai, Claude Code) must keep working unchanged.
- Only the one configured reviewer account bypasses the player limit; the limit stays in force for everyone else.
- The reviewer's Google account has no two-step verification, as OpenAI's review requires.
- Submission needs the privacy, terms and support pages live at tcgreplay.app (the privacy, terms and disclaimer spec) and the playmat served at tcgreplay.app (the domain move).

## Acceptance Criteria

- [ ] **Existing assistants unaffected**
  Claude (claude.ai and Claude Code) connects, imports the sample battle log and plays exactly as before the change.
- [ ] **Labels on every tool**
  The tool list the playmat sends to an assistant shows the read-only, destructive and open-world labels on every tool, each with an explicit true or false value.
- [ ] **Descriptions on every tool**
  Every tool in that list has a title, a description containing when to use it and when not to, and a description for every parameter.
- [ ] **Instructions opening**
  The first 512 characters of the playmat's instructions name what the playmat is, the rule to reference cards by their instance IDs, and where to find the coaching guides, as complete sentences.
- [ ] **Waits under the limit**
  A call to wait for a turn that isn't over returns in under 60 seconds with a result telling the assistant to call it again.
- [ ] **Identity**
  Connected from ChatGPT, asking who is signed in returns the player's identifier in the form OpenAI's rules require, and the same player gets the same identifier on a second connection.
- [ ] **Domain challenge**
  With the maintainer's token configured, `https://tcgreplay.app/.well-known/openai-apps-challenge` returns exactly that token, without signing in.
- [ ] **Package accepted**
  The plugin package built from the project uploads to OpenAI's submission form without validation errors and shows the name "TCG Replay", the icon, the description with the fan-project notice, and working website, privacy, terms and support links.
- [ ] **Reviewer past the limit**
  With the player limit reached by other accounts, the reviewer account still signs in, imports the sample game and plays, while another non-reviewer account is still turned away.
- [ ] **Review material**
  The project contains the five positive and three negative test cases (each with the prompt and the expected behaviour), release notes, the video script and the sample battle log they use.
- [ ] **End to end in ChatGPT**
  In a new ChatGPT chat, a player connects TCG Replay, signs in with Google, imports the sample battle log, opens the board link, and reviews and replays the game without errors; each positive test case produces its expected behaviour and each negative case is declined or handled as written.

## Technical Approach

- Expect to set every label explicitly: the Go MCP SDK drops a false read-only flag, and the read-only tools send no destructive flag today.
- Prefer a test that fails when any tool lacks a label, a title or parameter descriptions, so new tools can't slip through.
- Prefer supplying the domain-verification token and the reviewer account by configuration, so neither needs a release.
- Measure the total size of the tool definitions after the description pass, since OpenAI recommends keeping it small and long descriptions on ~45 tools add up.
- Test the connection with MCP Inspector, including sign-in, before submitting.
- Prefer building the plugin package with a project script, so it is reproducible for each release.

## Success Metrics

- The submission passes OpenAI's automated checks and reaches human review.
- If the listing is rejected, the reason is not missing metadata, missing material or a broken connection.
- TCG Replay is listed in ChatGPT's Plugin Directory.

## Non-Goals

- Bundling the coaching workflows as skills: a later version.
- Interactive widgets shown inside ChatGPT.
- A licence for the Pokémon card content.
- Raising the player limit: the observability spec measures and sets it.
- The privacy and terms pages (the privacy, terms and disclaimer spec) and the Plugin Directory connection docs (the docs spec of the public-launch epic).
- Maintainer-only steps: OpenAI identity verification, creating the reviewer's Google account, recording the video and pressing submit.

