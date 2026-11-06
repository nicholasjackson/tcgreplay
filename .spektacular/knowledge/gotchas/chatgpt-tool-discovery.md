---
tags: [mcp, chatgpt, connector, guides]
---

# ChatGPT picks tools from their descriptions, not the server instructions

With TCG Replay switched on in a chat (ChatGPT listed all 45 tools when asked), an attached TCG Live battle log plus "Let's look at this game" did not trigger an import. ChatGPT read the log itself and coached from the text. The server instructions said to use the import-game guide when a log is pasted, but at the time the `import_log` description only said what the tool does, and the `guide` description named no guides.

ChatGPT decides whether to use an app from each tool's name and description. Treat the MCP server instructions as a bonus that Claude reads, not something every client relies on.

- A tool that should fire on a user action opens its description with "Use this when …", names what the input looks like, and adds "even if they do not mention TCG Replay". `import_log` does this, and also says to import rather than read the log yourself.
- The `guide` tool's description is built from the guide list (`guideToolDescription` in `internal/mcpserver/guides.go`), so every guide's "Use when …" trigger is in the tool list too and cannot drift from the guides.
- A long paste reaches ChatGPT as an attached file ("Pasted text.txt"), so say "pastes or attaches".

Related: [[handoff-in-tool-results]]. Instructions that must be followed at a moment of decision belong in tool results.
