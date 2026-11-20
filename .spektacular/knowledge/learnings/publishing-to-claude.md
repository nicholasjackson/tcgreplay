---
tags: [claude-ai, mcp, connector, oauth, hosting, publishing]
---

# Bringing a remote MCP server to Claude

*Research date: 7 November 2026. Every factual claim has a source (listed at the end). Anything not verified from an official Anthropic or MCP source is marked **Unverified**. Written with TCG Replay as the example, and drafted so it can later be published.*

## Summary

Anthropic does not have "plugins" in the old ChatGPT sense. The parts that matter for a hosted MCP server like TCG Replay are:

- **Custom connectors.** Any user adds your server by its URL. There is no review.
- **The Connectors Directory.** Anthropic's public catalogue. Anyone on a paid plan can submit to it.
- **MCP Apps.** Interactive UIs that render inside the chat.
- **Plugins.** A plugin bundles connectors with skills. Plugins are listed in the same directory, and Claude Code also has its own plugin marketplaces.
- **Desktop extensions (.mcpb).** Still installable locally, but the directory no longer accepts them.

All Claude surfaces run on one connector infrastructure, so a server that works as a custom connector also works when listed. For TCG Replay, three issues stand out:

1. claude.ai currently drops the server `instructions` field. Claude Code does not.
2. claude.ai and Desktop cut off every tool call at 240 seconds. That matters for `wait_for_turn`.
3. Directory listing means meeting the annotation, OAuth, privacy, test-account and intellectual-property rules. The IP rules matter a lot for a Pokémon TCG product.

## 1. Product surfaces and how they relate

**Connectors.** A connector links Claude to an outside service through an MCP server. The Customize page in claude.ai and the desktop app holds connectors, skills and plugins together. [S1, S2]

**Where connectors work.** [S1]
- claude.ai on the web: remote connectors, including MCP Apps.
- Claude desktop app: remote connectors, plus local connectors installed as desktop extensions.
- Claude mobile apps: remote connectors.
- Claude Code: connectors added in claude.ai are available when you sign in with your Claude account, in terminal and cloud sessions.
- Cowork: connectors and plugins.

**Custom connectors versus directory connectors.** Both use the same runtime, transport, authentication and tool-calling code. [S3]
- Directory connectors are reviewed. Users can find them by browsing and search, and Claude can suggest them in chat ("Suggested Connectors").
- Directory connectors can use Anthropic-held OAuth client credentials.
- Directory connectors can allowlist external-link destinations.
- Custom connectors appear with the label **Custom**.
- Directory listings carry either a **Verified** or a **Community** label. [S4]
- Publishing to the open MCP Registry does not make a server appear in Claude. [S3]

**Plugins.** An installable package that can hold skills, MCP connectors, commands and agents. [S2]
- In chat, a plugin's skills load, and its remote MCP servers are listed on the plugin's Connectors tab.
- Agents and hooks are ignored in chat but load in Cowork and Claude Code. Local MCP servers are ignored in chat. [S5]
- A plugin installed on a claude.ai account syncs to Claude Code as `<name>@synced`. [S5, S6]

**MCP Apps.** Interactive UI that your server renders inside a Claude conversation. [S7] Listed as supported in claude.ai on the web [S1]; the design guidelines also cover mobile, where apps render in a native WebView. [S8] Announced on 26 January 2026 (date from search snippets of Anthropic's blog; full post not fetched). [S9]

**Desktop extensions (MCPB, formerly DXT).** An `.mcpb` file is a zip holding a local stdio MCP server and a `manifest.json`, installed in Claude Desktop. [S10] The directory has deprecated desktop extension listings and no longer accepts MCPB submissions; to distribute a local server through the directory, put it inside a plugin. [S10, S11]

**Claude Code marketplaces.** [S13]
- `claude-plugins-official` is added to Claude Code automatically.
- `claude-community` (`anthropics/claude-plugins-community`) holds third-party plugins their authors submitted. Users add it by hand.
- Anyone can run their own marketplace with a `.claude-plugin/marketplace.json` file. [S14]
- The official marketplace does not take submissions through the directory portal. [S14]

**Plans.**

| Surface | Plans | Source |
|---|---|---|
| Custom connectors (remote, by URL) | Free (one connector only), Pro, Max, Ultra, Team, Enterprise | S15, S16 |
| Browse plugins from the directory | Pro, Max, Ultra, Team, Enterprise | S5 |
| Submit to the directory (connector or plugin) | Pro, Max, Ultra, Team, Enterprise; not Free | S11 |
| Desktop extensions | Claude desktop app signed in to claude.ai; organisations can turn them off | S16 |

An older search summary said submitting needed a Team or Enterprise organisation. The current "Publish to the directory" page says any paid plan can submit, and on Team and Enterprise an Owner submits. [S11] The current page is treated as authoritative.

## 2. How a user adds a custom remote connector

**Individual plans (Free, Pro, Max, Ultra).** [S16] Customize > Connectors > **Add custom connector**, enter the HTTPS URL, optionally OAuth details, then **Add**.

**Team and Enterprise.** [S16, S15] An Owner adds it under Organization settings > Connectors (Add > Custom > Web); on Enterprise a custom role with "Manage access to Libraries" can too. Each member then connects with their own account.

**Authentication options in the add dialog.** [S16] **Sign in now**, **Sign in when needed** or **No sign-in**. The OAuth client can be **Use Claude's published identity** (CIMD, recommended), **Register automatically** (DCR) or **Use your own OAuth client**. Authentication settings cannot be changed later: remove the connector and add it again.

**Install links.** A link can prefill the add dialog; the user still confirms. [S3]

```
https://claude.ai/customize/connectors?modal=add-custom-connector&connectorName=NAME&connectorUrl=ENCODED_URL
```

**Desktop and mobile.**
- Remote connectors belong to the account, so they appear on desktop and mobile once added. [S2] They must be added on web or desktop before they appear on mobile. [S8]
- Every connection comes from Anthropic's cloud, including from mobile, so the server must be reachable from the public internet. [S15]
- Servers in `claude_desktop_config.json` are a separate, local mechanism, not available in claude.ai or Cowork. [S15]

**Using a connector in chat.** [S1] Turn it on per conversation from + > Connectors. Approve calls with **Allow once** or **Always allow**. Per-tool permissions are **Always allow**, **Needs approval** or **Blocked**.

**Admin controls.** Owners can disable tool calls that render MCP Apps. [S15] Organisation `blocked` settings apply in chat, desktop and Claude Code. [S17]

**Claude Code.** `claude mcp add --transport http <name> <url>`, then sign in through `/mcp`. [S17] claude.ai connectors also appear automatically when signed in with a claude.ai subscription.

**Tool-list caching and refresh.**
- **Claude Code** handles `notifications/tools/list_changed` in interactive sessions without a reconnect, and keeps the previous list if a refresh fails. [S17]
- **claude.ai: no official documentation found.** **Unverified**: a GitHub issue and third-party guides report that claude.ai caches the tool list, and that removing and re-adding the connector refreshes it. [S18, S19]
- For directory listings, tool changes take effect when deployed, but the tool names shown on the listing need a reviewed listing edit. [S20]

## 3. Authentication

**Supported types.** [S21]

| Type | Availability |
|---|---|
| OAuth 2.0 with Dynamic Client Registration (RFC 7591) | Supported by default |
| OAuth 2.0 with Client ID Metadata Document (CIMD) | Supported by default |
| Anthropic-held client credentials | Directory listings only |
| Custom URL or credentials at connection time | By arrangement with Anthropic |
| Static request headers | Beta, limited set of organisations |
| No authentication | Supported by default |

- Machine-to-machine `client_credentials` grants are not supported. [S21]
- "Lazy authentication" leaves some tools open and asks for sign-in only when a protected tool is called. [S21]

**Spec versions.** Claude follows the MCP authorisation specs dated 2025-03-26, 2025-06-18 and 2025-11-25. [S22]

**Discovery requirements.** [S21, S23]
- Answer unauthenticated requests with **401** and `WWW-Authenticate: Bearer resource_metadata="…"`. Claude ignores this header on a 200.
- The protected resource metadata (RFC 9728) `resource` must exactly match the URL the user enters, path included.
- Claude uses only the **first** entry in `authorization_servers`.
- The authorisation server must serve RFC 8414 or OpenID Connect discovery metadata, and be reachable from Anthropic's egress range **160.79.104.0/21**.

**Callback URLs.** [S21] Hosted apps (web, Desktop, mobile, Cowork) use `https://claude.ai/api/mcp/auth_callback`. Claude Code uses a loopback redirect on a random port: match `http://localhost/callback` and `http://127.0.0.1/callback` ignoring the port.

**PKCE, scopes and tokens.** [S21, S23]
- Claude always sends PKCE with S256; advertise `code_challenge_methods_supported: ["S256"]`.
- Claude sends the RFC 8707 `resource` parameter; check the token audience against the canonical server URL.
- `offline_access` is requested when the metadata lists it.
- The token endpoint must accept `application/x-www-form-urlencoded`, return `invalid_grant` for a dead refresh token, and rotate refresh tokens for public clients.

**Time limits.** [S21] Discovery, registration and token endpoints get **10 seconds**; refresh requests get **30 seconds**.

**DCR versus CIMD at scale.** DCR registers a new client on every fresh connection. For high directory traffic Anthropic recommends CIMD or Anthropic-held credentials. [S21]

**Network pitfalls.** [S23] Connectors are IPv4-only (public A record needed). Private or carrier-grade NAT addresses are rejected. A redirect to another host drops the `Authorization` header, a common cause of "works in Inspector, fails in claude.ai". WAF or CDN blocks on Anthropic's range break connections.

**No-auth servers** are allowed; anyone with the URL can use them. [S21, S16] **Directory rule:** remote servers that need authentication must use secure OAuth 2.0 with certificates from recognised authorities. [S24, policy 5D]

## 4. Official publication

### 4.1 Connectors Directory

Submit at **claude.ai/directory/manage**: Submit new > MCP connector. [S25]

**Pre-submission checklist.** [S25]
- Remote and reachable over HTTPS; OAuth works with Claude's client.
- Every tool has a `title` and the right `readOnlyHint` or `destructiveHint`.
- Tested as a custom connector.
- Listing materials: documentation URL, privacy policy URL, support contact, icon (MCP Apps also need carousel screenshots).
- Test credentials for a fully populated account.

**Portal steps.** [S25]
1. **Connection.** The URL. Per-customer URLs need the Verified label.
2. **Tools.** Synced automatically; tools missing titles or annotations are flagged.
3. **Listing.** Name ≤100 characters, one-liner ≤200, description ≤2,000, one to five categories. The slug is permanent once published.
4. **Use cases.**
5. **Company.**
6. **Authentication.**
7. **Data handling.** Whether the API is your own, proxied from a partner with permission, or a third party's you don't control; whether it handles health data or sponsored content.
8. **Test & launch.** Confirm every tool ran through MCP Inspector or as a custom connector.
9. **Compliance.** Seven acknowledgements: directory guidelines, first-party API usage, financial transactions, AI media generation, prompt injection, conversation data collection and public documentation.
10. **Review and submit.**

**Review outcome.**
- Each submission is scanned automatically and listed as **Community** by default. [S25, S4]
- Anthropic may escalate a highly useful listing to **Verified** review, where a person tests each tool. You cannot apply for Verified. [S26, S4]
- Review times are not fixed; escalations go to `mcp-review@anthropic.com`. [S25, S11]
- Published listings get a health badge and usage metrics; delisting is by email. [S27, S20]

**Review criteria and common rejections.** [S26]
- **Read and write must be split.** A catch-all tool for both is rejected.
- **Tool names ≤64 characters.** Descriptions narrow and accurate.
- **No prompt-injection patterns.** Descriptions are rejected if they tell Claude to call tools the user didn't request, interfere with Claude calling other tools, direct Claude to pull behavioural instructions from external sources, contain hidden or encoded instructions, or tell Claude to behave in ways unrelated to the tool, override system instructions or promote products.
- **Functional quality.** Succeed with valid input; actionable errors; reasonably sized responses; no extra conversation data; never query Claude's memory, chat history or files.
- **API ownership.** Call your own APIs or ones you legitimately proxy; the server domain should match your service.
- **Unsupported uses.** Moving money or crypto; AI image, video or audio generation.

**Software Directory Policy (key items).** [S24]
- 1D privacy and data handling; 1E no extra conversation data, "even for logging purposes"; 1F no infringing others' intellectual property; 1G no user profiles built from conversation content.
- 3A privacy policy (collection, use, retention); 3B verified support and security contacts; 3C documentation; 3D test account with sample data; 3E at least three working example prompts; 3F you own or control any API endpoint, domain, UI or external resource the server connects to or renders; 3G/3H maintain the server and notify Anthropic of material changes.
- 5A–5F graceful errors, token frugality, names ≤64 characters, OAuth 2.0, annotations, Streamable HTTP.

**Directory terms.** [S28] You warrant all rights needed to distribute the software; provide and follow a privacy policy (and any applicable third-party ones); have a way to receive vulnerability reports; indemnify Anthropic. Anthropic may remove a listing "for any reason".

**Branding and trademarks.**
- *Anthropic's marks.* A listing grants no right to use Anthropic's name or marks, and you may not imply partnership or endorsement without written approval. [S28] The trademark guidelines (effective 1 August 2024) require prior approval for any use of the marks. [S29] **Unverified**: no examples are given for nominative phrases such as "works with Claude".
- *Third-party IP (matters for Pokémon).* Only general rules: policy 1F bans infringement [S24]; the terms require "all necessary rights" [S28]; policy 3F requires you to own or control what you render [S24]; for plugins, names that "match a known brand that isn't yours" are held for review [S12]. **Not found**: a connector-specific rule on third-party brand names in listings.
- *Pokémon's own terms* (not Anthropic's). The Pokémon legal page names "Pokémon" as a Nintendo trademark, and its fan-art terms grant no rights beyond "personal, noncommercial home use". [S30] No public licensing route for third-party apps was found. **Unverified**: how The Pokémon Company treats free fan tools. Take legal advice before publishing officially; nothing here is legal advice.

**External-link allowlist (optional).** With `ui/open-link`, list HTTPS origins you own so users skip the confirmation prompt; entries for domains you don't own are removed. [S25]

### 4.2 Plugins in the directory and in marketplaces

**Directory plugin submission.** [S31, S12, S11]
- Submit from a GitHub repository in the same portal (**Plugin bundle**). The repository must be public before the plugin goes live.
- Requirements: `.claude-plugin/plugin.json` with a kebab-case name; README of at least 40 words; a licence; pinned package launchers; no secrets; remote MCP servers declared with `type` `http`, `sse` or `ws` and an absolute `https://` URL.
- Every version is validated and security-scanned automatically; a person reviews a new listing before it goes live. Updates come from the tracked branch without resubmitting.
- Naming: reserved words (`claude`, `anthropic`, `official`, `plugin`, `mcp`, `test`) cannot be the whole name; names presenting the plugin as official are blocked; names matching a known brand are held for review.
- If you also run the remote server the plugin points at, submit the server as a connector too, pair the listings and point both at the same URL. [S11]

**Your own Claude Code marketplace.** [S14, S6] Add `.claude-plugin/marketplace.json` to a git repository; there is no review. Users run `claude plugin marketplace add owner/repo`, then `claude plugin install name@marketplace`. Auto-update is off by default for third-party marketplaces.

**Community marketplace.** `claude-community` holds third-party plugins "that their authors submitted to Anthropic". [S13] **Unverified**: exactly how a directory submission reaches it.

## 5. Technical specifics

**Tool annotations.**
- The MCP spec defines `title`, `readOnlyHint` (default false), `destructiveHint` (default true), `idempotentHint` (default false) and `openWorldHint` (default true). All are hints that clients must not trust from untrusted servers. Display-name precedence is `title`, then `annotations.title`, then `name`. [S32]
- Anthropic requires a `title` and the applicable hint on every directory tool. [S26, S24]
- In Claude, read-only tools "can run without per-call confirmation, and destructive tools always prompt". [S26]

**Two conflicts in the annotation guidance.**
1. Anthropic's checklist says `destructiveHint: true` for tools that "modify or delete data" [S26]; the MCP spec says `destructiveHint: false` means "only additive updates" [S32]. An additive game move fits the spec's `false` but could be read as needing `true` under Anthropic's wording.
2. The review page says destructive tools "always prompt" [S26]; the user docs say any tool can be set to **Always allow** [S1]. How these interact is not documented.

**Claude Code annotation behaviour.** [S17] `readOnlyHint` lets calls run in parallel; `readOnlyHint` plus `idempotentHint` allow one retry after a dropped connection; `_meta["anthropic/requiresUserInteraction"]` forces a prompt on every call.

**Descriptions.** [S26, S24, S33] Say what the tool does and when to call it, matching behaviour. The spec treats the description as a "hint" to the model. Report tool failures in the result with `isError: true` and actionable text, not as protocol errors.

**Limits.**

| Limit | claude.ai and Desktop | Claude Code | Source |
|---|---|---|---|
| Tool result size | ~150,000 characters | 25,000 tokens by default (`MAX_MCP_OUTPUT_TOKENS`); warns above 10,000. Text over 50,000 characters is saved to a file unless the tool sets `_meta["anthropic/maxResultSizeChars"]` (ceiling 500,000). | S22, S17 |
| Tool call time | **240 s per call** | `MCP_TOOL_TIMEOUT` (about 28 h if unset). Idle abort after 5 min with no response or progress on HTTP; calls move to the background after 2 min. | S22, S17 |
| Tool name | 64 characters (directory rule) | Full `mcp__server__tool` name ≤128 characters | S24, S17 |
| Tool count | No documented limit found | No fixed per-server cap; tool search defers definitions | S17 |
| Instructions and description length | Not documented | 4,096 characters for instructions and upfront tool descriptions; 16,384 for descriptions loaded through tool search | S17 |

**Server instructions.**
- The MCP spec says clients "MAY" add `instructions` to the system prompt. [S32] Claude Code uses them, especially to decide when to search for a server's tools. [S17]
- **claude.ai and Desktop currently drop them.** Issue #93 in Anthropic's `claude-ai-mcp` tracker is open, labelled `triaged` and `backlog`, with commenters confirming the behaviour as late as 31 October 2026. [S34, S35] This is a bug tracker, not documentation.

**Supported MCP features.** [S22] Tools, prompts and resources; text and image results; text and binary resources; Streamable HTTP; legacy SSE (being deprecated). Not supported: resource subscriptions, sampling, "advanced or draft capabilities".

**MCP Apps.** [S7, S8, S25]
- Built with the MCP Apps SDK (`registerAppTool`, `registerAppResource`) using `ui://` resources.
- For Claude, `_meta.ui.domain` is the first 32 hex characters of the SHA-256 of the server URL, followed by `.claudemcpcontent.com`.
- External origins are blocked unless declared in `_meta.ui.csp`. Display modes: inline, fullscreen, pip.
- On mobile, inline height is capped and vertical scrolling belongs to the conversation; use fullscreen for scrolling views.
- Directory listings for MCP Apps need 3–5 PNG screenshots at least 1,000 px wide.
- **Unverified**: whether go-sdk v1.6.0 has MCP Apps helpers. The official SDK is TypeScript; a Go server would emit the metadata itself.

## 6. Testing

- **MCP Inspector** [S36, S37]: web UI, CLI or TUI (`npx @modelcontextprotocol/inspector`, `--cli` or `--tui`); needs Node 22.19.0 or newer. Remote example: `npx @modelcontextprotocol/inspector --cli https://host/mcp --transport http --method tools/list`.
- **In Claude** [S37]: there is no staging environment. Add the server as a custom connector, check it shows **Connected**, that every tool appears under Tool permissions, and that a real call works. Use a tunnel for a local server.
- **Client detection** [S37]: `clientInfo.name` varies (`claude-ai`, `Anthropic`, `claude-code`) and is unauthenticated. Do not gate behaviour on it.
- **Troubleshooting** [S23]: public DNS, reachability, no cross-host redirect, no WAF block, discovery metadata, cross-host authorisation server. Report failures with the `ofid_` reference ID at `github.com/anthropics/claude-ai-mcp/issues`.
- **Official checklists**: connector pre-submission [S26] and submission [S25]; plugin pre-submission [S12] and `claude plugin validate --strict` [S14].

## 7. Practical implications for TCG Replay

These rest on TCG Replay's design as described to the researcher; check them against the code.

1. **`wait_for_turn` and the 240-second limit.** A long poll must return well before 240 s on claude.ai and Desktop. TCG Replay's 15 s default cap (50 s maximum) is already well inside it. In Claude Code, idle calls abort after 5 minutes without progress and move to the background after 2 minutes. [S22, S17]
2. **Server instructions are invisible on claude.ai.** Rules in the instructions (iids, locations, compound actions, the wait loop) reach the model only in Claude Code. [S34, S17] Keep the essentials in tool descriptions, the `guide` tool and tool results (see [[handoff-in-tool-results]]). A plugin whose skills carry the coaching guides is another route, because skills load in chat, Cowork and Claude Code. [S5]
3. **Prompt-injection review risk.** Review rejects descriptions that tell Claude how to behave beyond the tool's function, or direct it to pull behavioural instructions from external sources. [S26, S24] Some TCG Replay wording carries this risk: "Never ask the player to say 'your turn'", "do what its result's next says", and workflow guides fetched from a tool. **Unclear**: whether instructions served by the same server count as "external". Keep wording descriptive, and ask `mcp-review@anthropic.com` if in doubt.
4. **Annotations across ~45 tools.** Read-only tools (`get_state`, `get_log`, `card_text`, `guide`, `ping`) get `readOnlyHint: true`. Tools that discard state (`undo`, `rewind`, `new_game`, `checkout`) fit `destructiveHint: true`. Ordinary moves are additive: mark them per the conflict above and expect possible prompts. `openWorldHint: false` (a playmat is a closed world). Every tool gets a `title`.
5. **OAuth with Google login.** Claude must reach an authorisation server that offers DCR or CIMD, accepts the claude.ai callback and loopback redirects, and answers within 10 seconds. [S21] If TCG Replay brokers Google sign-in through its own authorisation server, check that server against section 3. Prefer CIMD for directory scale.
6. **Result size.** Keep `get_state` and `import_log` well under ~150,000 characters, and frugal in tokens. [S22, S24]
7. **MCP App opportunity.** The web board could become a fullscreen MCP App: declare the board's origin as an allowed link, and any card-image CDN in the CSP. Policy 3F requires owning or controlling what you render. [S8, S25, S24]
8. **Pokémon IP and data sources.** Lead with "TCG Replay", not "Pokémon". Add an "unofficial, not affiliated with Nintendo, The Pokémon Company or Anthropic" statement in the description and docs. In Data handling, disclose that card text and images come from a third-party API (TCGdex); the API-ownership rule may apply. [S26, S25] **Unverified**: whether Anthropic will list a Pokémon fan tool. Take legal advice.
9. **Reviewer access.** Reviewers need a fully populated account. With Google-only login, that means a dedicated test Google account with games already imported, plus clear steps. [S25, S37]

## Checklist for TCG Replay

**Hosting and transport**
- [ ] Public IPv4 A record; no cross-host redirect; 160.79.104.0/21 not blocked by a WAF; Streamable HTTP.

**Authentication**
- [ ] 401 with `resource_metadata` for unauthenticated requests; metadata `resource` exactly equals the URL users enter; own issuer listed first.
- [ ] DCR or CIMD offered; S256 PKCE; form-encoded token endpoint returning `invalid_grant`; refresh tokens rotated; token endpoints under 10 s.
- [ ] `https://claude.ai/api/mcp/auth_callback` and port-agnostic loopback redirects accepted.

**Tools**
- [ ] Every tool has a `title`, the right `readOnlyHint` or `destructiveHint`, and `openWorldHint: false`; names ≤64 characters; failures return `isError: true` with actionable text.

**Limits**
- [ ] `wait_for_turn` returns in under 240 s; results well under 150,000 characters.

**Guidance**
- [ ] Critical rules live in tool descriptions, `guide` and tool results, not only in `instructions`; behavioural wording checked against the prompt-injection criteria.

**Testing**
- [ ] Every tool tested in MCP Inspector (CLI) and as a custom connector on web, desktop, mobile and Claude Code.

**Listing materials**
- [ ] Public documentation page; privacy policy (collection, use, storage, sharing, retention, contact); support and security contact; icon; at least three example prompts; populated reviewer account.

**IP review**
- [ ] Non-Pokémon product name and an unaffiliated disclaimer; card data and image sources disclosed; legal advice taken.

**Optional**
- [ ] Fullscreen board MCP App (`ui.domain`, CSP, link allowlist, 3–5 screenshots).
- [ ] Plugin bundling the guides as skills (non-brand name, README, licence, pinned dependencies), submitted with the connector and paired.
- [ ] Before the directory: share the custom-connector install link from the project website.

## Sources (all accessed 2026-11-07)

- S1 Get started with connectors: https://claude.com/docs/connectors/getting-started.md
- S2 Connectors, skills, and plugins: https://claude.com/docs/extend/overview.md
- S3 Directory connectors vs custom connectors: https://claude.com/docs/connectors/building/directory-vs-custom.md
- S4 Connector verification: https://claude.com/docs/connectors/verification.md
- S5 Plugin feature support across platforms: https://claude.com/docs/plugins/platform-support.md
- S6 Install and manage plugins (Claude Code): https://code.claude.com/docs/en/discover-plugins.md
- S7 Get started with MCP Apps: https://claude.com/docs/connectors/building/mcp-apps/getting-started.md
- S8 MCP Apps design guidelines: https://claude.com/docs/connectors/building/mcp-apps/design-guidelines.md
- S9 Interactive tools in Claude (blog, search snippet only): https://claude.com/blog/interactive-tools-in-claude
- S10 Build a desktop extension with MCPB: https://claude.com/docs/connectors/building/mcpb.md
- S11 Publish to the directory: https://claude.com/docs/directory/publish.md
- S12 Plugin pre-submission checklist: https://claude.com/docs/plugins/pre-submission-checklist.md
- S13 Anthropic's marketplaces: https://code.claude.com/docs/en/plugins/anthropic-marketplaces.md
- S14 Publish and distribute a plugin: https://code.claude.com/docs/en/plugins/publish.md
- S15 Get started with custom connectors using remote MCP: https://support.claude.com/en/articles/11175166-get-started-with-custom-connectors-using-remote-mcp
- S16 Add a connector that isn't in the directory: https://claude.com/docs/connectors/custom/add-unlisted.md
- S17 Connect Claude Code to tools via MCP: https://code.claude.com/docs/en/mcp.md
- S18 (secondary) claude-ai-mcp issue #137: https://github.com/anthropics/claude-ai-mcp/issues/137
- S19 (secondary) Titan Network Claude setup: https://titanconnect.titannetwork.com/docs/claude-ai; Wheelhouse: https://help.usewheelhouse.com/en/articles/15908885-refreshing-your-claude-connector-to-get-new-tools
- S20 Manage your listing after publishing: https://claude.com/docs/connectors/building/after-publishing.md
- S21 Authentication for connectors: https://claude.com/docs/connectors/building/authentication.md
- S22 Build an MCP server for Claude: https://claude.com/docs/connectors/building/index.md
- S23 Troubleshoot your connector: https://claude.com/docs/connectors/building/troubleshooting.md
- S24 Anthropic Software Directory Policy: https://support.claude.com/en/articles/13145358-anthropic-software-directory-policy
- S25 Submit a connector to the directory: https://claude.com/docs/connectors/building/submission.md
- S26 Connector pre-submission checklist (review criteria): https://claude.com/docs/connectors/building/review-criteria.md
- S27 Manage your directory listing: https://claude.com/docs/connectors/building/managing-your-listing.md
- S28 Anthropic Software Directory Terms (16 March 2026): https://support.claude.com/en/articles/13145338-anthropic-software-directory-terms
- S29 Anthropic Trademark Guidelines: https://www.anthropic.com/legal/trademark-guidelines
- S30 Pokémon legal page: https://www.pokemon.com/us/legal
- S31 Submit your plugin: https://claude.com/docs/plugins/submit.md
- S32 MCP schema 2025-11-25: https://modelcontextprotocol.io/specification/2025-11-25/schema
- S33 MCP Tools spec 2025-11-25: https://modelcontextprotocol.io/specification/2025-11-25/server/tools
- S34 claude-ai-mcp issue #93 (server instructions not passed): https://github.com/anthropics/claude-ai-mcp/issues/93
- S35 claude-ai-mcp issue #131 (duplicate of #93): https://github.com/anthropics/claude-ai-mcp/issues/131
- S36 MCP Inspector: https://modelcontextprotocol.io/docs/tools/inspector
- S37 Test your connector: https://claude.com/docs/connectors/building/testing.md

Related: [[chatgpt-tool-discovery]], [[handoff-in-tool-results]], [[publishing-to-chatgpt]], [[publishing-to-gemini]].
