---
tags: [chatgpt, mcp, publishing, plugin, skills, testing]
---

# A plugin ZIP with mcp.json uploads to ChatGPT as "Desktop only"; without it, it has no tools

**The trap.** Uploading the plugin package as a personal plugin in ChatGPT (Customize > Plugins > Add, upload the ZIP) marks it **Desktop only** whenever the ZIP holds an `mcp.json`, even when that file names a remote server (`type: streamable-http`, an https URL, OAuth with dynamic client registration). The plugin page then offers only "Open in desktop app", the Personal list shows a laptop icon in place of "+", and Work mode's Plugins picker on the web never lists it. There is no desktop app for Linux, so on Linux it cannot be used at all.

**The second half of the trap.** Dropping `mcp.json` makes the upload installable on the web, but it is then a **skills-only** plugin: no MCP server, no tools, no sign-in. Verified 24 November 2026 by installing TCG Replay from the Personal tab as an end user: install asked for no Connect or Google sign-in, the plugin page lists "Skills 1" and no "Apps" section, its settings page shows no tools or permission level, and a starter prompt ("Play a practice game…") got "playmat controls aren't available in this chat". A question like "what can the playmat do?" still gets a plausible answer, from the skill text alone, so check "Worked for Ns" for tool calls before calling a test a pass.

**Why it surprises.** OpenAI's docs define desktop-only as plugins that run locally (a stdio MCP server, a local program, or `requires_local_executor`), and recommend putting the remote server in `mcp.json`. Nothing says a remote `mcp.json` triggers the badge. The likely reason is the `mcp_configuration_excluded` rule: MCP config in a ZIP counts as local MCP unless the organization is approved for it.

**Verified 22 November 2026** by uploading the same package three ways:
- full package (plugin.json, mcp.json, icon, skill): Desktop only;
- plus a skill `agents/openai.yaml` with `policy.products: [CHAT]` and a remote `dependencies.tools` entry: still Desktop only;
- without `mcp.json`: "Install plugin" on the web, skills only (see above). That upload also has to drop the plugin-level `review.test_cases`, or it is refused with "Plugin-level test_cases require exactly one MCP server".

**What `.app.json` is, and why it does not fix this** (OpenAI docs, read 24 November 2026: developers.openai.com/plugins/build/plugins and /plugins/deploy/submission).
- `.app.json` maps a name to an MCP server **already registered** with OpenAI (an `asdk_app_…` ID), referenced from `extensions.com.openai.apps` (or `apps` in a Codex manifest). Notion's plugin in github.com/openai/plugins does this, and its ChatGPT page shows an "Apps" section; that binding is where its tools and OAuth come from.
- It is for local-marketplace and workspace packages that reuse an existing connection. **ZIPs containing `apps` / `.app.json` (or lifecycle hooks) cannot be submitted**, which is why `internal/plugin` forbids the `apps` key. A public plugin declares the server in `mcp.json` and connects it in the portal's **With MCP** step.
- **The MCP server must be in the initial ZIP**: OpenAI does not support adding an MCP server to an existing skills-only plugin, so a dashboard draft created skills-only needs a new plugin to gain one.

**How to test before approval** (OpenAI's "Connect and test your plugin" page):
1. **The server:** Plugins > + > Add custom MCP server, URL `https://tcgreplay.app/mcp`, OAuth, Create as a plugin; install it, then `@` it in a new chat and run prompts that must call tools. This works on the web.
2. **The complete plugin** (skills plus server): add the package to a local marketplace and install it from the Plugins Directory. Local marketplaces are read by the ChatGPT desktop app, so this step cannot be done on Linux.
- No documented route lets an end user install an unpublished, submitted plugin with its MCP server on the web; the full end-user install exists only once the listing is published.
