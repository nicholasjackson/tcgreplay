---
tags: [chatgpt, mcp, connector, oauth, hosting, publishing]
---

# Bringing a remote MCP server to ChatGPT

*Research date: 7 November 2026. Every claim cites a source (listed at the end). Claims marked **Unverified** come only from secondary sources, or from official pages seen only as search snippets. Written with TCG Replay as the example, and drafted so it can later be published.*

## Summary

ChatGPT now calls its extension system **plugins**. This is not the 2023 plugins beta, which was retired in April 2024. A plugin is a package that can hold **skills**, **one MCP server** and an optional **UI**. ChatGPT and Codex share one Plugin Directory. [S1][S2]

There are two ways to bring an MCP server to ChatGPT:

- **Private use.** A user with the right plan or workspace permission adds a custom MCP server from **Plugins → + → Add custom MCP server**, then selects **Create as a plugin**. [S3][S4]
- **Public listing.** Package the plugin as a ZIP, verify your identity and domain, pass review, then publish to the Plugin Directory. [S5][S6]

The server must speak Streamable HTTP, use OAuth 2.1 with PKCE when it holds per-user data, and give every tool explicit `readOnlyHint`, `destructiveHint` and `openWorldHint` values. [S7][S8][S9]

For TCG Replay the hardest parts of a public listing are not technical:

- **Pokémon intellectual property.** OpenAI requires that you own or have permission to use any IP. [S9]
- **Audience.** Pokémon appeals to children, but plugins may not explicitly target under-13s. [S9]
- **The reviewer test account.** Google login must give reviewers an account that works without MFA. [S6][S5]

## 1. Product names and how they relate

| Period | Name | What it was |
|---|---|---|
| 2023 to April 2024 | ChatGPT plugins (beta) | OpenAPI-based plugins. New plugin chats were blocked on 19 March 2024 and existing ones ended on 9 April 2024; users were pointed to GPTs. [S20] (community forum; official help article not found) |
| Nov 2023 onwards | GPTs and GPT Actions | Custom GPTs called APIs through "actions". GPT Actions do not carry over to plugins and must be rebuilt as an app or a custom MCP server. [S12] |
| 2025 | Connectors | ChatGPT connected to outside data. Developer mode added full MCP client support, including write tools, as a beta in September 2025. [S21] (secondary) |
| 6 Oct 2025 | Apps in ChatGPT and the Apps SDK | Interactive apps built on MCP, at launch for logged-in Free, Go, Plus and Pro users **outside the EEA, Switzerland and the UK**; Business, Enterprise and Edu preview on 13 Nov 2025. [S13] (snippet) |
| 17 Dec 2025 | Connectors renamed "apps"; submissions open | Developers could submit apps for an in-ChatGPT directory. [S14][S15] (snippets) |
| 9 Jul 2026 | Plugins and the Plugin Directory | The app directory became a Plugin Directory shared by ChatGPT and Codex. A plugin bundles apps (MCP connections), skills and app templates. Existing connections kept working. [S1][S16] (help article as snippet) |
| 11 Dec 2026 (announced) | Custom GPTs retire | Being replaced by plugins. The date appears only in secondary sources. [S22] **Unverified date**; OpenAI's migration page gives none. [S12] |

Today's developer docs live under **developers.openai.com/plugins** and say "plugins" throughout; "Apps SDK" survives only in older URLs and the llms.txt index. [S2][S17] "App" still means the MCP-backed connection inside a plugin. [S12][S16]

**Where plugins work.** ChatGPT Chat and Work on web, desktop and mobile; Codex in the ChatGPT desktop app and the CLI. [S1]

**Plans and regions.** Official sources are incomplete and partly contradictory:
- A help article (snippet) says custom MCP connections with full MCP support are for **Business and Enterprise/Edu** on web, with **Pro** read/fetch only, and says nothing about Plus. [S18]
- An older, now removed developer-mode guide (snippet) listed Plus, Pro, Business, Enterprise and Edu on web. [S19]
- For workspaces, the permission "Create plugins with MCPs" ("Use developer mode to create custom app connections") is **off by default**. [S10][S11]
- Regions: the October 2025 launch excluded the EEA, Switzerland and the UK [S13], and release notes (snippet) said connectors on Plus and Pro were not available there. [S23] **Unverified**: current UK/EEA availability; no official source found.
- Projects with EU data residency cannot submit MCP plugins. [S6]

## 2. How a user adds a custom remote MCP server

Official steps [S3][S4]:
1. Open ChatGPT on the **web** and go to **Plugins** (chatgpt.com/plugins).
2. Select **+**, then **Add custom MCP server**.
3. Enter a name and optional description.
4. Enter the server URL, including `/mcp`. For a private server choose **Tunnel** (Secure MCP Tunnel).
5. Choose authentication: **OAuth**, **No authentication**, or **OAuth or no authentication** (mixed).
6. Accept the risk warning, then select **Create as a plugin**.
7. **Install** the plugin; in a new chat type `@` and pick it.

A help article (snippet) says developer mode no longer has to be turned on first [S18]; workspace docs still call the capability "developer mode". [S11]

**Protocols.** SSE and streaming HTTP [S3]. Public submission needs Streamable HTTP on a stable public HTTPS endpoint. [S7]

**Refreshing tools.**
- Each app has a settings page where users switch individual tools on or off.
- **Refresh** pulls in new tools, new descriptions **and new server instructions**. [S3]
- After changing names, descriptions, schemas, annotations, auth or UI resources: deploy, press **Refresh** on the connection in ChatGPT Plugins, confirm the metadata changed, then retest **in a new conversation**. [S4]
- OpenAI documents no automatic refresh for custom connections; ChatGPT keeps the list it last fetched (see [[chatgpt-tool-discovery]]). A community request asks for a refresh API. [S24]
- **Published plugins** differ: OpenAI scans the server daily and you can trigger **Rescan**, so users need not refresh (section 4). [S5][S6]

**Confirmations.** ChatGPT respects `readOnlyHint`; **tools without it are treated as write actions**, which need user confirmation by default. A user can remember approve or deny for a tool within one conversation; a new conversation asks again. [S3]

## 3. Authentication

Source: the official authentication page [S8] unless stated otherwise.

- **When needed.** Authenticate if the server exposes customer-specific data or write actions. Anonymous read-only servers can skip OAuth.
- **Standard.** The MCP authorization spec: OAuth 2.1 with PKCE. `code_challenge_methods_supported` **must include `S256`**.
- **Protected resource metadata (RFC 9728).** Serve `/.well-known/oauth-protected-resource` with `resource`, `authorization_servers` and optionally `scopes_supported`. Unauthenticated requests get `401` with `WWW-Authenticate: Bearer resource_metadata=..., scope=...`.
- **Authorization server metadata.** Serve `/.well-known/oauth-authorization-server` or `/.well-known/openid-configuration`. `issuer` must exactly match the value in `authorization_servers`.
- **Client registration**, in OpenAI's order of preference:
  1. **CIMD.** `client_id` is `https://chatgpt.com/oauth/client.json`, with no registration step. Set `client_id_metadata_document_supported: true`. Auth methods `none` and `private_key_jwt`.
  2. **DCR.** ChatGPT calls `registration_endpoint` once per connection, so many connections create many clients.
  3. A **predefined client**.
- **`resource` parameter.** Sent on authorise and token requests. Put it in the token's `aud` and reject tokens with the wrong audience.
- **Redirect URIs.** With RFC 9207 issuer identification, `https://chatgpt.com/connector_platform_oauth_redirect`; otherwise `https://chatgpt.com/connector/oauth/{callback_id}`. Copy the exact URI from the management page.
- **RFC 9207.** Optional; if advertised, return `iss` in **every** authorisation response.
- **Not supported:** client credentials, service accounts, API keys and custom headers. The portal supports no auth, OAuth and mixed. [S5] The package manifest lists `api_key` as an auth type [S25] but the submission portal says API keys are not supported [S5].
- **Per-tool `securitySchemes`.** Declare `noauth` and/or `oauth2` on **every tool**. In mixed mode, `initialize` and `tools/list` run without auth. [S3] To show the account-linking UI, a tool returns an error result carrying `_meta["mcp/www_authenticate"]`.
- **Multiple accounts.** Provide a read-only, argument-free profile tool marked `_meta["openai/profile"]: true` with an `outputSchema`. Its `id` must be stable and immutable: for Google, the `sub` claim, not the email address.
- **Enterprise domain restrictions.** Need `openid` and `email` scopes and a UserInfo endpoint returning `email_verified: true`.

With no auth every tool runs anonymously; with OAuth, linking starts at the first tool call; with mixed auth, anonymous use works at once and linking unlocks more. [S3][S8] For review, authenticated plugins need a demo account that works without MFA. [S6]

## 4. Official publication

**Before you submit** [S5][S6][S9]:
- Complete **individual or business verification** in the OpenAI Platform dashboard; publishing under an unverified name is rejected.
- Be an organisation owner, or a member with **Apps Management Write** (`api.apps.write`).
- Use a stable, public HTTPS endpoint (no tunnels or local endpoints).
- Prove domain ownership with a challenge token served as plain text at `/.well-known/openai-apps-challenge`.
- Use a project with global (not EU) data residency.

**Package** [S25][S26]: a ZIP with `plugin.json` (agent-plugins schema 1.0.0) and `mcp.json` (`type: streamable-http`, `url`, `extensions["com.openai"].auth`), optionally `skills/<name>/SKILL.md`. No secrets in the ZIP. **One MCP server per plugin.**

| Item | Limit |
|---|---|
| Display name | 30 characters |
| Short description | 30 characters |
| Long description | 4,000 characters |
| Starter prompts | 3, at 128 characters each |
| Images | square; 48×48 to 4096×4096 px; up to 5 MiB |
| ZIP | 100 MB |

**Review material** [S5][S6]: 5 positive and 3 negative test cases; a video walkthrough URL; release notes; reviewer credentials; logo, company URL and privacy policy URL; country availability (`publication.countries` is an allow-list).

**Process** [S5][S6]:
1. The portal scans your tools, importing names, descriptions, schemas, annotations, `securitySchemes`, `_meta`, UI CSP and server `instructions`.
2. Submit for review. Only one review can be active at a time.
3. Feedback arrives by email; appeal by replying. Do not ask for expedited review; no timeline is given.
4. After approval, you choose when to publish.
5. A plugin appears on main directory pages only if OpenAI picks it for "enhanced distribution". You cannot ask for this.

**Common rejections** [S6]: reviewers cannot connect; test cases fail; responses contain data the privacy policy does not cover; annotations do not match behaviour.

**After publication** [S5][S6]:
- OpenAI scans daily; you can trigger **Rescan**.
- New tools stay unavailable until approved. A changed tool keeps its old approved definition until the update passes, so **keep the server compatible with the live schema**. A deleted tool is removed once a scan sees it gone.
- Listing metadata or skill changes need a new ZIP version and another review.
- Changing the origin (scheme, host or port) needs a new plugin.
- UI resources may be cached for up to an hour.

**Policies** (plugin guidelines [S9]; usage policies [S27]):
- **Purpose and quality.** Complete and stable; demos and trials are rejected.
- **Audience.** Suitable for general audiences including 13–17; **may not explicitly target children under 13**. Mature (18+) experiences are not supported yet.
- **IP and branding.** "Only use intellectual property that you own or have permission to use." No copycat designs or impersonation; do not imply OpenAI made or endorses the plugin; avoid generic single-word names; do not add "MCP", "MCP Server" or "Plugin" to the name; no competitor comparisons or pricing in descriptions.
- **Third-party services.** "Do not scrape external websites, relay queries, or integrate with third-party APIs without proper authorization." OpenAI cannot approve plugins that "primarily function as unofficial connectors to third-party services".
- **Usage policies** ban attempts to infringe others' IP. [S27] (snippet)
- **Trademarks.** No specific trademark section in the guidelines [S9]; OpenAI's brand page restricts using "GPT" and OpenAI model names in product names. [S28] (snippet)
- **Privacy.** A privacy policy covering data categories, purposes, recipients, retention and user controls; minimum data; no restricted data (card numbers, health data, government IDs, credentials); no covert tracking.
- **Monetisation.** Commerce limited to physical goods; no digital goods, subscriptions, credits or upsells inside ChatGPT (an informational plan page link is allowed, checkout is not); users may use features of a subscription they already pay for; **no ads**.
- **Support.** Accurate support contact details.

## 5. Technical specifics

**Tools** [S7][S9][S29][S30]: a stable, action-oriented `name`; a human-readable `title`; a description; an explicit `inputSchema`; an `outputSchema` when returning structured data; accurate annotations. Prefer focused tools over multi-mode ones, do not hide operations behind a generic executor, and make each tool work on its own.

**Annotations** must all be explicit booleans [S9][S6]:
- `readOnlyHint`: true only if the tool can change nothing. Writing logs or running jobs count as state changes.
- `destructiveHint`: true for deletion, overwriting, cancellation or irreversible sends. "Being undoable doesn't justify `false`."
- `openWorldHint`: true for the public internet or open-ended parties; a bounded private account can use false.

**Descriptions** [S29][S30]: start with "Use this when…"; state when not to use the tool; tell it apart from similar tools; document every parameter with examples and allowed values; do not steer the model towards or away from other plugins. ChatGPT decides when to call a tool "based on the metadata you provide". [S29]

**Server instructions.** Use them for cross-tool guidance. **Only the first 512 characters are guaranteed to be available during tool selection** [S10], so they should stand alone [S3][S7]. Do not repeat tool descriptions in them. [S7] They are imported at scan time and refreshed by Refresh. [S6][S3]

**Limits.**
- No official limit on tool count or description length was found. [S6][S7][S29]
- **Unverified** (community): an error saying all tool definitions together must be **under 5,000 tokens**. [S31]

**Timeouts.**
- ChatGPT web: **no official tool-call timeout published**. **Unverified** community reports range from about 1 to 5 minutes. [S32]
- Codex: `tool_timeout_sec` defaults to 60 s, `startup_timeout_sec` to 10 s. [S10]
- UIs feel sluggish when tool calls take more than a few hundred milliseconds; proxies and CDNs that buffer responses break streaming. [S33]

**Results** [S7][S9]: `structuredContent` is concise data the model can reuse; `content` is text for the model; `_meta` is hidden from the model. Leave out session, trace and request IDs, timestamps and logging metadata unless needed. Results must make sense without the UI.

**UI (MCP Apps standard)** [S34][S35]:
- Serve the component as a `ui://` resource with MIME type `text/html;profile=mcp-app`, linked from the tool with `_meta.ui.resourceUri` (`_meta["openai/outputTemplate"]` is a legacy alias that still works). The URI is a cache key; publish a new URI for breaking changes.
- The bridge is JSON-RPC over `postMessage` (`ui/initialize`, `tools/call`, `ui/message`, …); `window.openai` is only for extras such as widget state.
- Declare a CSP with `_meta.ui.csp` (`connectDomains`, `resourceDomains`, `frameDomains`); nested frames are blocked by default.
- Embedded pages must come from the MCP server's own registrable domain. Iframes need a written justification and can slow review. [S9]
- Display modes: inline card, carousel, fullscreen and **picture-in-picture**, which OpenAI names as a good fit for games.
- Use system fonts, do not show your own logo, meet WCAG AA.

**SDKs.** The docs name only the TypeScript and Python MCP SDKs [S7]. The Go SDK can serve the same protocol fields; how go-sdk v1.6.0 exposes `_meta` and annotations was not checked.

## 6. Testing

- **MCP Inspector.** List and call tools; try edge cases, missing identifiers and empty results [S4]; step through OAuth with its Auth settings. [S8]
- **API Playground.** Add the MCP server under Tools and inspect request and response logs. [S4]
- **ChatGPT custom MCP server.** Install the plugin, then test direct, indirect, follow-up, write-action and unsupported requests in a new chat; record the tool chosen, its arguments and errors; retest after every Refresh. [S4]
- **Golden prompt set.** Direct, indirect and negative prompts with expected behaviour; track precision and recall; change one metadata field at a time; fix precision on negative prompts first. [S29]
- **Official pre-submission checklist** [S4]: skills activate correctly; similar phrasing behaves consistently; starter prompts work; the UI renders without console errors and restores state; test on desktop and mobile [S9]; test cases pass on the supported ChatGPT and Codex surfaces. [S6]

## 7. Practical implications for TCG Replay

1. **The tool list is stale until refreshed** (documented for custom connections [S3][S4]). Release notes for ChatGPT users should say: "after an update, open Plugins → TCG Replay → Refresh, then start a new chat." A published plugin avoids this through daily scans, but tool changes are then held for approval. [S5]
2. **Descriptions matter more than instructions.** Make the first 512 characters of the server instructions stand alone. [S10][S3] Put critical sequencing in tool descriptions too ("after end_turn, call wait_for_turn", "read the guide before coaching"), and in tool results (see [[handoff-in-tool-results]]). Pointing at your own tools is allowed; the rule covers other *plugins*. [S9]
3. **About 45 tools.** No official cap. If the community-reported 5,000-token total is real it would be tight [S31] (**Unverified**). Measure the total `tools/list` size.
4. **Confirmation friction.** About 35 tools change state, and each needs confirmation by default unless remembered for the conversation. [S3] Expect friction in play mode; accurate `readOnlyHint` on read tools removes it for those.
5. **`wait_for_turn` long-polling.** ChatGPT's timeout is undocumented [S32]. TCG Replay's 15 s default cap (50 s maximum) already keeps polls under 60 s and returns a "call again" result.
6. **Pokémon IP is the main barrier to a public listing.** OpenAI requires owning or having permission for any IP [S9], and its usage policies ban infringement. [S27] The name, card text and card images belong to their rights holders. **Unverified**: The Pokémon Company's current fan-content or licensing terms; secondary reports describe past enforcement over card images. [S36] Take legal advice or seek permission before submitting, and make sure name and branding do not suggest an official Pokémon or OpenAI product.
7. **Age.** The listing must not explicitly target under-13s. [S9]
8. **Third-party data.** Card text and images come from TCGdex; confirm its terms authorise this use. [S9]
9. **Google login and review.** Reviewers need a demo account with sample games and no MFA. [S6][S5] **Unverified**: Google sign-in may trigger verification challenges for a shared reviewer account; consider a dedicated reviewer login path.
10. **UI.** The web board could become an MCP Apps resource in picture-in-picture mode, served from TCG Replay's own domain with a tight CSP. Embedding the existing board as an iframe needs a justification. [S34][S9][S35]
11. **Plans and region.** Official sources conflict on Plus and Pro, and UK/EEA availability is unconfirmed. [S18][S19][S13] Check before telling players which plan they need.

## Checklist for TCG Replay

**Protocol and auth**
- [ ] Streamable HTTP at a stable public HTTPS `/mcp` URL; no response buffering at the proxy. [S7][S33]
- [ ] `/.well-known/oauth-protected-resource` served; 401 responses carry a `WWW-Authenticate` challenge. [S8]
- [ ] Authorization server metadata with an exact `issuer` match, `S256`, and CIMD (preferred) or DCR. [S8]
- [ ] Tokens bound to `resource`/`aud`; signature, `exp` and scopes checked on every call. [S8]
- [ ] `securitySchemes` on every tool; consider `noauth` for `guide` and `card_text`. [S8]
- [ ] `get_profile` argument-free, read-only, `_meta["openai/profile"]: true`, with an `outputSchema` and a stable `id` (Google `sub`). [S8]

**Tool metadata**
- [ ] Explicit `readOnlyHint`, `destructiveHint` and `openWorldHint` on every tool. Read-only: `get_state`, `get_log`, `card_text`, `guide`, `ping`, `list_branches` and similar. `wait_for_turn` writes presence state, so check whether it counts as read-only. [S6][S9]
- [ ] `destructiveHint: true` for tools that overwrite or discard a game (`new_game`, `import_log` if it replaces a game, `rewind`, `set_opponent_deck`); probably `openWorldHint: false` throughout. [S9]
- [ ] Every tool has a `title`, a "Use this when…" description that excludes wrong cases, documented parameters and an `outputSchema` where structured. [S29][S30]
- [ ] First 512 characters of the server instructions stand alone; key sequencing rules also in tool descriptions. [S10]
- [ ] Total tool-definition size measured. [S31]
- [ ] `wait_for_turn` returns well within 60 seconds and says to call again. [S10][S32]
- [ ] Internal IDs, timestamps and trace data removed from results unless needed. [S9]

**Testing**
- [ ] MCP Inspector, including the OAuth flow. [S4][S8]
- [ ] Golden prompt set (direct, indirect, negative); retest in a new chat after each Refresh. [S29][S4]
- [ ] "How to Refresh" instructions with each release. [S3]

**For a public listing**
- [ ] Legal clearance or permission for Pokémon IP; a name and branding that imply no official product. [S9][S27]
- [ ] General-audience listing, not aimed at under-13s. [S9]
- [ ] Privacy policy covering Google identity data, game logs, retention and deletion. [S9]
- [ ] Individual or business verification; global-residency project; domain challenge at `/.well-known/openai-apps-challenge`. [S5][S6]
- [ ] Reviewer account with sample games and no MFA. [S6]
- [ ] 5 positive and 3 negative test cases, a video and release notes. [S5]
- [ ] `plugin.json` and `mcp.json` ZIP within the limits. [S25][S26]
- [ ] No ads and no upsell of paid tiers. [S9]
- [ ] After publication, tool schemas kept backwards compatible, because changes are held for approval. [S5][S6]
- [ ] Optional: `guide` workflows bundled as skills (scanned in review). [S27b]

## Sources (all accessed 2026-11-07)

"(snippet)" marks pages that refused automated fetching (HTTP 403 or 404); their content was read from search-engine snippets of the official page.

- [S1] OpenAI, *Plugins* (ChatGPT docs): https://learn.chatgpt.com/docs/plugins.md
- [S2] OpenAI, *Plugins documentation*: https://developers.openai.com/apps-sdk (now serves the Plugins docs)
- [S3] OpenAI, *Add custom MCP server*: https://developers.openai.com/api/docs/guides/custom-mcp-server
- [S4] OpenAI, *Connect and test your plugin*: https://developers.openai.com/plugins/deploy/connect-chatgpt.md
- [S5] OpenAI, *Upload and submit your plugin*: https://developers.openai.com/plugins/deploy/submission.md
- [S6] OpenAI, *Remote MCP server review requirements*: https://developers.openai.com/plugins/deploy/app-review.md
- [S7] OpenAI, *Build an MCP server*: https://developers.openai.com/plugins/build/mcp-server.md
- [S8] OpenAI, *Authentication*: https://developers.openai.com/apps-sdk/build/auth
- [S9] OpenAI, *Plugin guidelines*: https://developers.openai.com/plugins/plugin-guidelines.md
- [S10] OpenAI, *Model Context Protocol* (ChatGPT docs): https://learn.chatgpt.com/docs/extend/mcp.md
- [S11] OpenAI, *Plugin controls*: https://learn.chatgpt.com/docs/enterprise/apps-and-connectors.md
- [S12] OpenAI, *Moving your custom GPT workflows to plugins*: https://learn.chatgpt.com/docs/migrate-custom-gpts.md
- [S13] OpenAI, *Introducing apps in ChatGPT* (snippet): https://openai.com/index/introducing-apps-in-chatgpt/
- [S14] OpenAI, *Developers can now submit apps to ChatGPT* (snippet): https://openai.com/index/developers-can-now-submit-apps-to-chatgpt/
- [S15] OpenAI Help Center, *Apps in ChatGPT* (snippet): https://help.openai.com/en/articles/11487775-apps-in-chatgpt
- [S16] OpenAI Help Center, *Plugins in ChatGPT and Codex* (snippet): https://help.openai.com/en/articles/20001256-plugins-in-chatgpt-and-codex
- [S17] OpenAI developer docs index: https://developers.openai.com/llms.txt
- [S18] OpenAI Help Center, *Connecting custom MCP servers in ChatGPT* (snippet): https://help.openai.com/en/articles/12584461-developer-mode-and-mcp-apps-in-chatgpt
- [S19] OpenAI, *ChatGPT Developer mode* (former guide, now 404; snippet): https://developers.openai.com/api/docs/guides/developer-mode
- [S20] OpenAI Developer Community, *Plugin Store and New Chats With Plugins – Closed March 19 2024* (community): https://community.openai.com/t/plugin-store-and-new-chats-with-plugins-closed-march-19-2024/689877
- [S21] The Register, *Full MCP support in beta ChatGPT*, 15 Sep 2025 (secondary): https://theregister.com/2025/09/15/full_mcp_support_in_beta_chatgpt
- [S22] Developers Digest, *Custom GPTs Retire Dec 11, 2026* (secondary): https://www.developersdigest.tech/blog/custom-gpt-retirement-migrate-to-plugins
- [S23] OpenAI Help Center, *ChatGPT release notes* (snippet): https://help.openai.com/en/articles/6825453-chatgpt-release-notes
- [S24] OpenAI Developer Community, *Feature Request: Public API for Refreshing a Developer MCP App's Actions* (community): https://community.openai.com/t/feature-request-public-api-for-refreshing-a-developer-mcp-apps-actions-in-chatgpt/1395374
- [S25] OpenAI, *Package your plugin*: https://developers.openai.com/plugins/build/plugins.md
- [S26] OpenAI, *Plugin submission errors*: https://developers.openai.com/plugins/deploy/submission-errors.md
- [S27] OpenAI, *Usage policies* (snippet): https://openai.com/policies/usage-policies/
- [S27b] OpenAI, *Plugin architecture*: https://developers.openai.com/plugins/concepts/plugins.md
- [S28] OpenAI, *Brand guidelines* (snippet): https://openai.com/brand/
- [S29] OpenAI, *Optimize Metadata*: https://developers.openai.com/plugins/guides/optimize-metadata.md
- [S30] OpenAI, *Define tools*: https://developers.openai.com/plugins/plan/tools.md
- [S31] OpenAI Developer Community, *Token limit when connecting ChatGPT with external MCP* (community): https://community.openai.com/t/token-limit-when-connecting-chatgpt-with-external-mcp/1371022
- [S32] OpenAI Developer Community, *How to configure long MCP tool call times for ChatGPT app* (community): https://community.openai.com/t/how-to-configure-long-mcp-tool-call-times-for-chatgpt-app/1379834
- [S33] OpenAI, *Troubleshooting*: https://developers.openai.com/plugins/deploy/troubleshooting.md
- [S34] OpenAI, *Add UI to your MCP server*: https://developers.openai.com/plugins/build/chatgpt-ui.md
- [S35] OpenAI, *UI guidelines*: https://developers.openai.com/plugins/concepts/ui-guidelines.md
- [S36] Techdirt, *Pokemon Vs. Pokellector In Trademark/Copyright Dispute* (secondary, 2014): https://www.techdirt.com/2014/02/12/

Related: [[chatgpt-tool-discovery]], [[handoff-in-tool-results]], [[publishing-to-claude]], [[publishing-to-gemini]].
