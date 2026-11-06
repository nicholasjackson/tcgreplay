---
title: Get started
description: Connect the hosted tcgreplay playmat to claude.ai, ChatGPT or Claude Code, and check it works.
order: 10
showAddress: true
---

tcgreplay runs as a connector (an MCP server) for the AI assistant you already use. Add the hosted playmat at the address above, sign in with Google the first time it is used, and the assistant can import your games, coach you and show the board.

You need a Google account to sign in. The hosted playmat serves a few players at once; if it is full, you can [run it on your own computer](/docs/desktop) instead.

## claude.ai

1. Open **Settings → Connectors** and choose **Add custom connector**.
2. Name it `tcgreplay` and paste the playmat's address, `https://tcgreplay.demo.gs/mcp`.
3. Choose **Add**, then **Connect**. Google asks you to choose an account: pick one and allow it to share your email address. You come back to claude.ai.
4. In a chat, make sure tcgreplay is switched on in the chat's tools menu.

**Check it works.** Start a new chat and ask:

```prompt
What can the tcgreplay playmat do?
```

Claude should list the playmat's coaching workflows, such as importing a game, reviewing it and drills. If it says it can't reach the playmat, open the connector in **Settings → Connectors** and connect it again. See [signing in and opening the board](/docs/sign-in) for what the sign-in looks like.

## ChatGPT

1. Open ChatGPT's settings and add a custom MCP server (**Plugins → Add → Add custom MCP server**).
2. Paste the playmat's address, `https://tcgreplay.demo.gs/mcp`, and leave authentication on **OAuth**.
3. Save, then connect. Google asks you to choose an account and share your email address, and you come back to ChatGPT.
4. In a chat, turn tcgreplay on from the chat's tools or plugins menu.

**Check it works.** Start a new chat and ask:

```prompt
What can the tcgreplay playmat do?
```

ChatGPT should list the playmat's coaching workflows. If it can't reach the playmat, connect the server again from settings. See [signing in and opening the board](/docs/sign-in).

## Claude Code

1. In a terminal, add the playmat:

   ```sh
   claude mcp add --transport http tcgreplay https://tcgreplay.demo.gs/mcp
   ```

2. Start Claude Code, type `/mcp`, choose **tcgreplay** and **Authenticate**. Your browser opens Google's account chooser; pick your account, then return to the terminal.

**Check it works.** Ask Claude Code:

```prompt
What can the tcgreplay playmat do?
```

It should list the playmat's coaching workflows. If it can't reach the playmat, run `/mcp` again and authenticate tcgreplay. See [signing in and opening the board](/docs/sign-in).

## The Claude desktop app

The desktop app can use the playmat running on your own computer, with no sign-in. See [running it on your desktop](/docs/desktop).

## Next

- [Signing in and opening the board](/docs/sign-in): what to expect from Google, and how to open the board link.
- [Coaching workflows](/docs/workflows): what to ask once you're connected.
