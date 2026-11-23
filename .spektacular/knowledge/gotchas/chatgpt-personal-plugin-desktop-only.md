---
tags: [chatgpt, mcp, publishing, plugin, skills]
---

# A plugin ZIP with mcp.json uploads to ChatGPT as "Desktop only"

**The trap.** Uploading the plugin package as a personal plugin in ChatGPT (Customize > Plugins > Add, upload the ZIP) marks it **Desktop only** whenever the ZIP holds an `mcp.json`, even when that file names a remote server (`type: streamable-http`, an https URL, OAuth with dynamic client registration). The plugin page then offers only "Open in desktop app", the Personal list shows a laptop icon in place of "+", and Work mode's Plugins picker on the web never lists it. There is no desktop app for Linux, so on Linux it cannot be used at all.

**Why it surprises.** OpenAI's docs define desktop-only as plugins that run locally (a stdio MCP server, a local program, or `requires_local_executor`), and recommend putting the remote server in `mcp.json`. Nothing says a remote `mcp.json` triggers the badge. The likely reason is the `mcp_configuration_excluded` rule: MCP config in a ZIP counts as local MCP unless the organization is approved for it.

**Verified 22 November 2026** by uploading the same package three ways:
- full package (plugin.json, mcp.json, icon, skill): Desktop only;
- plus a skill `agents/openai.yaml` with `policy.products: [CHAT]` and a remote `dependencies.tools` entry: still Desktop only;
- without `mcp.json`: "Install plugin", usable on the web. That upload also has to drop the plugin-level `review.test_cases`, or it is refused with "Plugin-level test_cases require exactly one MCP server".

**What to do instead.**
- Submit through the plugin dashboard on platform.openai.com using the **With MCP** path, where the remote server is connected, domain-verified, signed in to and scanned. Do not judge the submission by a personal upload's badge.
- To try the server in a web chat before approval, use a custom connection to `https://tcgreplay.app/mcp` (developer mode), which is not desktop only. A skill-only personal upload can sit alongside it to try the skill.
