---
tags: [gemini, mcp, connector, oauth, hosting, publishing]
---

# Bringing a remote MCP server to Google Gemini

*Research date: 7 November 2026. Every claim cites an official Google source unless marked **Unverified** (secondary sources or search snippets only). Written with TCG Replay as the example, and drafted so it can later be published.*

## Summary

"Gemini" is now several products, and each treats third-party MCP servers differently.

- **Consumer Gemini app (gemini.google.com).** Supports custom remote MCP servers as **custom apps**, added by URL. Only some users can add them: in the US, aged 18 or over, personal account, Keep Activity on, English.
- **Public listing in the Gemini app** goes through the **Connected Apps Platform**, launched as a preview on 28 October 2026. Developers apply to join, and every app is reviewed.
- **Gemini CLI** still supports MCP servers and extensions, with a self-service extensions gallery. But from 18 June 2026 Google **replaced it with Antigravity CLI** for individual accounts. Antigravity keeps MCP support and turns extensions into "plugins".
- **Gemini Enterprise** (formerly Agentspace). Admins add a custom MCP server as a data store; it can render interactive MCP Apps.
- **Gems** do not connect to MCP servers, and Google is replacing them with Skills.

For TCG Replay: US users can add it to the consumer Gemini app today as a custom app; users outside the US cannot. A public listing needs acceptance into a gated preview that currently lists no games category and flags servers with dozens of tools.

## 1. The surfaces and how they relate

### 1.1 Consumer Gemini app: Connected Apps and custom apps

- "Gemini Extensions" has given way to "Connected Apps" in Google's release notes: entries up to March 2025 say "extensions", later ones "connected apps". [R1]
- In October 2026 a "customise" hub brought skills and connected apps together, with "more than 40" external services. Most connected apps are English-only in the US, some in other English-speaking regions, Brazil and India. It is rolling out to all tiers, including Workspace. [R2, R1]
- **Custom apps** add any MCP server by URL. Requirements [R3]: aged 18 or over and in the US; a personal Google Account (work or school "not supported yet"); Keep Activity on; English only; works in the Gemini web and mobile apps, added only on the web.
- **Plan requirement: Unverified.** The help page (fetched 2026-11-07) names no plan. Search snippets of Google's help pages tie custom apps to **Gemini Spark** ("Connect & manage custom apps for Gemini Spark in the Gemini web app"; answer/13695044: custom apps "only available to users with Gemini Spark access and within Spark tasks"), but the live pages did not show this text. Secondary sources say Spark needs Google AI Ultra, or Pro in some countries, and disagree with each other. [R4, X2, X3]
- **Gems.** Release notes for 30 September 2026 say "Skills will soon replace Gems", with existing Gems converted automatically. [R1] No official page says Gems or Skills can call MCP servers. **Unverified** (Zapier): Gems cannot use connected apps. [X1]

### 1.2 Connected Apps Platform: publishing to the Gemini app and Search

- Announced 28 October 2026 as one way to bring apps into the Gemini app and Search. [R5]
- Two integration paths: a cloud-hosted MCP server, or Android MCP through AppFunctions. Apps are registered in the **Google Connected Console**. [R5, R6]
- Access opens in phases ("more developers, markets, and Google products" over coming months), by interest form. [R5, R6]
- **Unverified** (TechCrunch via search): the preview is for US developers on a waitlist. Google's post names no regions. [X4]

### 1.3 Gemini CLI, now Antigravity CLI

- Announced 19 May 2026: from 18 June 2026 Gemini CLI stopped serving Google AI Pro and Ultra subscribers and free Gemini Code Assist for individuals. It stays for organisations with Gemini Code Assist Standard or Enterprise licences and for paid Gemini or Gemini Enterprise Agent Platform API keys. [R20]
- Agent Skills, Hooks, Subagents and Extensions carry over to Antigravity; extensions become "Antigravity plugins". Google warns of no "1:1 feature parity right out of the gate". [R20]
- The GitHub repository stays active (Apache 2.0), "maintained for enterprise Gemini Code Assist workflows on Google Cloud". [R28]

### 1.4 Gemini Enterprise (formerly Agentspace)

- Admins add a **custom MCP server data store** inside a Gemini Enterprise app. [R33]
- Business edition has a team-admin version, marked **Pre-GA**. [R34]
- Gemini Enterprise renders **MCP Apps** (interactive UI) for custom MCP servers. [R37]
- It also accepts **A2A agents**, registered in the console or REST API. [R36] (search summary)
- The editions page does not say which editions include custom MCP. A third-party guide says Business does not, but Google's Business help page documents that it does; trust Google's page. [R35, R34, X6]

### 1.5 Developer surfaces

- **Gemini API (Interactions API)** accepts an `mcp_server` tool pointing at a remote server. [R39]
- **Agent Development Kit (ADK)** connects through `McpToolset`. [R40]
- **Google's managed MCP servers**: about 75 Google Cloud servers, plus Workspace (Gmail, Drive, Docs and others), Google Home and Maps. None relate to games. [R38]

## 2. How a user adds a custom remote MCP server

### Consumer Gemini app (custom app) [R3]

1. On a computer, open gemini.google.com and go to **Settings > Connected Apps**.
2. Under "Custom apps", add a custom app and enter the MCP server URL.
3. If the server does not support Dynamic Client Registration, expand "Advanced features" and enter client credentials.
4. Click **Next** and follow the prompts.
5. In a chat, type "@" and pick the app.

It then also appears in the mobile app. Users can switch it off, disconnect it (revokes access, keeps it listed) or remove it.

**Tool refresh.** The help page does not say (**Unverified**). In Developer Mode, **Sync schema** re-fetches `tools/list` without clearing the chat. [R15]

### Gemini CLI (for users who still have it) [R25, R23, R24]

- `gemini mcp add --transport http tcgreplay https://example/mcp` (`--scope` defaults to `project`; `-H` adds a header; `--timeout`, `--include-tools`, `--exclude-tools` available).
- Or `settings.json`: `httpUrl` for Streamable HTTP, `url` for SSE.
- Or an extension whose `gemini-extension.json` has an `mcpServers` map, installed with `gemini extensions install <github-url>` (optionally `--ref`).
- **Refresh:** tools are discovered at startup; `/mcp reload` "reloads all MCP servers and re-discovers their available tools". [R27]

### Antigravity CLI and app [R29, R31, R30]

- Config files: global `~/.gemini/config/mcp_config.json`; workspace `<workspace>/.agents/mcp_config.json`; plugin `<plugin-root>/mcp_config.json`.
- Remote servers use `serverUrl` (or `url`). `httpUrl` and legacy HTTP+SSE are **not supported** in config files.
- Plugins install with `agy plugin install https://github.com/<owner>/<repo>` or a local path; `agy plugin import gemini` converts Gemini CLI extensions.
- **Refresh:** in the CLI, `/mcp` and reload; in the app, Restart.

### Gemini Enterprise [R33, R34]

- **Admin console:** Connected data stores > create > **Custom MCP Server** > **Add MCP server** > set up authentication > create, then wait for Creating → Active. All tools import as actions, **off by default**; use "Reload custom actions", then "Enable actions".
- **Business edition:** a team admin uses Settings & help > Manage team > Connected apps > **Add MCP Server**. The connection starts disabled until the admin enables it.
- **Refresh:** "Reload custom actions" lists tools again and re-authenticates.

### Developer paths

- **Interactions API:** a tool with `type: "mcp_server"`, `name`, `url`, optional `headers` and `allowed_tools`; server names must not contain `-`. [R39]
- **ADK:** `McpToolset` with `StreamableHTTPConnectionParams` (url, headers, timeout); `tool_filter` limits tools. [R40]

**`notifications/tools/list_changed`:** no Google doc describes support on any surface (**Unverified** either way).

## 3. Authentication

| Surface | What is documented |
|---|---|
| Consumer custom apps | Servers supporting Dynamic Client Registration (DCR) connect automatically; others need credentials under "Advanced features". [R3] |
| Connected Apps Platform | OAuth 2.1 authorisation code with PKCE (S256) recommended. Metadata discovered at `/.well-known/oauth-authorization-server`, then `/.well-known/openid-configuration`; must return HTTP 200. The OAuth guide says a `registration_endpoint` (DCR) is "currently" needed unless a client ID and secret are entered by hand. Redirect URIs on `googleusercontent.com` must be allowlisted. Refresh tokens "strongly recommended"; for Persistent Linking, do not rotate refresh tokens. Revocation endpoint optional. [R8] |
| Connected Console | No auth, machine-to-machine (client credentials), or user-delegated (Google Account Linking). For user-delegated auth the console tries Client ID Metadata Documents (CIMD) first, then DCR. **The docs conflict:** the OAuth guide says `registration_endpoint` is still required even with CIMD. User-delegated apps must give reviewers a seeded test account with two-factor authentication off. [R18, R8] |
| Gemini CLI | On a 401 it discovers OAuth and uses DCR with a localhost callback. Static `headers`; `authProviderType: google_credentials` (ADC); `service_account_impersonation` for OIDC ID tokens; `/mcp auth <server>` starts sign-in. [R25] |
| Antigravity | OAuth 2.0 with PKCE. Client credentials resolve as manual `oauth.clientId`, then CIMD, then DCR. Redirect URI `https://antigravity.google/oauth-callback`. Also `headers` and `google_credentials`. [R29] |
| Gemini Enterprise | No auth or OAuth 2.0, with authorisation URL, token URL, client ID, secret and scopes entered by hand, optional PKCE. Redirect URI `https://vertexaisearch.cloud.google.com/oauth-redirect`. For default `*.run.app` URLs Google also sends a Google-signed ID token in `X-Serverless-Authorization`. Request `offline_access` (or equivalent) for refresh tokens. [R33, R34] |
| Interactions API / ADK | Static headers only in the Interactions API; ADK supports Bearer, Basic, API key and OAuth2. [R39, R40] |

## 4. Official publication

### 4.1 Connected Apps Platform (Gemini app and Search)

**Process** [R18, R17]:
1. Apply through the interest form.
2. An owner or admin accepts the Connected Apps Terms of Service.
3. Create the app and choose a **category, which cannot be changed later**.
4. App details: icon 512 × 512 JPEG or PNG, ≤1 MB; one-sentence description under 30 words; at least three capabilities; at least three example prompts; Terms of Service and privacy policy URLs.
5. Verify the domain with a token at `/.well-known/google-connected-apps` (HTTP 200, no redirect).
6. Configure authentication.
7. Sync tools.
8. Set availability (countries, languages, products) and per-tool confirmation.
9. Add testers, who must be **@gmail.com** accounts.
10. Send for review, then publish.

**Versions.** Tools are snapshotted and locked at publish; changes need a new version, and Google recommends a new endpoint path per version. No review timeline is published. [R18]

**Categories.** Music, podcasts and audiobooks; content creation; grocery and food delivery; real estate; local services and reservations; events and ticketing; productivity; business and CRM; small business finance. There is **no games, hobby or education vertical**; more are "coming soon". [R12]

**Quality bar.** The app must improve with sign-in, or enable live context and completed actions; "generic signed-out information" does not qualify. Account linking through OAuth 2.1 should be live or on the "very near-term roadmap". [R11]

**Policy points relevant here** [R19]:
- All callable operations declared statically at registration: "Dynamic discovery functions and generic runtime dispatchers are not allowed."
- Sensitive, irreversible tools need a destructive annotation or console confirmation.
- Only operations supported by the registered vertical.
- No relaying to other autonomous AI agents; no third-party ads.
- No hidden instructions and no rules for how other apps' tools should be used.
- No impersonation or misleading claims about relationships.
- "Wrapper apps ... that mainly pass through another platform's APIs" are prohibited.
- Data minimisation: never request the user's prompts or conversation history; privacy policy and data-deletion help required.
- **IP:** apps that infringe "trademark, copyright, patent, and trade secret rights" are prohibited, as is facilitating unauthorised generation of copyrighted material.
- No brand-ownership requirement for submitters and no game-specific rules beyond age-gating real-money gaming.

### 4.2 Gemini CLI extensions gallery [R23, R24]

- No manual submission: the gallery indexes public repositories automatically.
- Requirements: public GitHub repository; `gemini-cli-extension` topic; `gemini-extension.json` at the repository (or release archive) root; a tagged release.
- Crawled daily, listed "if it passes validation". Validation rules, review, content policy and delisting rules are not documented.
- Its audience is shrinking because of the Antigravity switch.

### 4.3 Antigravity Marketplace [R32, R31]

- Plugins use a `plugin.json` manifest (`name`, `displayName`, `description`, `version`, `logo` ≥128 px, `suggestedPrompts`, `author`, …) and can bundle `mcp_config.json` plus always-on rules in `rules/AGENTS.md` or `rules/GEMINI.md`.
- To list: pass `agy plugin validate --strict`, then submit through a partner interest form.
- No review criteria, content policy or IP rules are published.

### 4.4 Gemini Enterprise

No public directory for custom MCP servers; each customer's admin adds them. A2A agents can be listed through Google Cloud Marketplace (search result, not read in full).

### 4.5 Branding and third-party IP

**Google marks.** Google's co-branding guidance allows plain-text phrases such as "works with", "is compatible with" or "integrates with" a Google product; forbids implying endorsement, affiliation or partnership; says to use product icons only when needed and so they do not look like Google's own; asks for a trademark legal line in a footer if Google trademarks are used. [R41] **Unverified**: an older page (now redirecting) said not to use Google product names in your own product name.

**Pokémon marks.** No Google document mentions Pokémon; only the general IP and impersonation rules apply. **Unverified**: no current official Pokémon Company policy on third-party tools was found. In practice: keep "Pokémon" out of the app name and icon; describe the relationship accurately ("an unofficial playmat for the Pokémon TCG"); add a "not affiliated with or endorsed by" disclaimer; take legal advice before submitting to a reviewed store.

## 5. Technical specifics

### Transports

| Surface | Streamable HTTP | SSE | stdio |
|---|---|---|---|
| Connected Apps Platform | Required. Spec 2025-03-26 up to 2025-11-25; newer not supported. One endpoint; GET may return 405. [R7] | Not supported | n/a |
| Consumer custom apps | Must "follow the standard MCP specifications"; transport not stated (**Unverified**). [R3] | Unverified | n/a |
| Gemini Enterprise | Only option [R33] | Not supported | n/a |
| Interactions API | Only option [R39] | Not supported | n/a |
| Gemini CLI | Yes (`httpUrl`) | Yes (`url`) | Yes [R25] |
| Antigravity config | Yes (`serverUrl`) | Not in config | Yes [R29] |
| ADK | Yes | Yes | Yes [R40] |

### Tool count

- **Connected Apps:** no hard limit, but "servers with dozens of tools are flagged in review". Expose only tools the model needs; keep schemas flat. [R7]
- **Gemini Enterprise:** at most 100 actions enabled at once. [R33]
- **Antigravity:** lazy-loads tools by default. [R29]
- **Gemini CLI:** names become `mcp_{serverName}_{toolName}`; invalid characters replaced; names over 63 characters truncated; avoid underscores in server names. [R25]

### Descriptions and annotations (Connected Apps) [R7]

- Directive descriptions; enums; long numeric IDs as strings.
- Missing annotations are treated as `readOnlyHint: false` and `destructiveHint: true`; `destructiveHint` and `openWorldHint` trigger a confirmation.
- `structuredContent` is "not yet supported": return JSON as text.
- Use the documented error codes (for example `-32500` DEADLINE_EXCEEDED).

Gemini Enterprise asks for confirmation on every action unless `readOnlyHint` is true. [R33] The consumer app asks for confirmation before write actions. [R3]

### Timeouts

| Surface | Documented behaviour |
|---|---|
| Gemini CLI | 600,000 ms default [R25] |
| Antigravity | 300 s to connect and list tools; 180 s per tool call; `timeoutSeconds` overrides both [R29] |
| Gemini Enterprise | No number; recommends pagination and asynchronous start-and-poll [R33] |
| Connected Apps | No number; "latency guardrails" and DEADLINE_EXCEEDED [R7] |
| Google Cloud MCP Apps blog | Tool responses under 2 s, with the widget polling [R37] |
| Consumer custom apps | Not documented (**Unverified**) |

### Server instructions and context files

- **Connected Apps:** instructions go into the system prompt as a concise, high-level hint about *what* the server does, not how to use its tools; snapshotted at registration. [R7]
- **Gemini CLI:** appends server instructions to the system instructions. [R25] **GEMINI.md** is loaded at global, workspace and just-in-time levels and sent with every prompt; `context.fileName` can include AGENTS.md; extensions can set `contextFileName`. [R26, R23]
- **Antigravity:** reads GEMINI.md and AGENTS.md; plugins use `rules/AGENTS.md`. [R30, R32]
- **Consumer app:** how it uses server instructions is not documented (**Unverified**).

### Interactive UI

- **Connected Apps:** MCP Apps in **beta**; "detected UI resources may be ignored". [R18] Types `text/html`, `text/javascript`, `text/css`, `text/html;profile=mcp-app`; 15 MB per resource, 100 MB total; CSP limits apply. [R7] Inline cards or fullscreen; no modals; no internal scrolling; external links need an explicit click. [R9]
- **Gemini Enterprise:** renders MCP Apps (SEP-1865) in a sandboxed iframe that can expand to a side panel or fullscreen. [R37]
- **Consumer custom apps:** whether UI renders for unregistered custom apps is not documented (**Unverified**).

### Other MCP features (Gemini CLI) [R25]

Prompts become slash commands; resources can be referenced with `@server://...`; images in results are passed to the model.

## 6. Testing

- **MCP Inspector** (`npx @modelcontextprotocol/inspector`) for local checks. [R13]
- **Gemini Apps Developer Mode** at gemini.google.com/devmode (or Settings > Gemini Apps Developer Mode): **+ Create new** with a public HTTPS URL and optional OAuth credentials; **Event Log** (calls, status, latency, payload size); **Tool Schemas** with safety badges and **Sync schema**; **Environment** (simulated date, time, language); **Authentication** (DCR or manual). Eligibility is not documented; it probably matches the custom-apps limits (**Unverified**). [R14, R15]
- **Connected Console preview:** @gmail.com testers; run all example prompts, the auth flow, confirmations and the UI. Changes can take up to 10 minutes to appear. [R18]
- **Gemini CLI:** `gemini extensions link <path>` for local development, then `/mcp list`, `/mcp desc`, `/mcp schema`. [R23, R27]
- **Antigravity:** `agy plugin validate --strict` and `/mcp list`. [R32]
- **Gemini Enterprise:** Reload custom actions, enable them, then test in the Default Assistant. [R33]

## 7. Practical implications for TCG Replay

- **Consumer Gemini app today.** US users with personal accounts can add `https://<host>/mcp` as a custom app, possibly only on a Spark-eligible paid plan (**Unverified**). TCG Replay's OAuth server must support DCR or offer pre-issued client credentials, and its authorisation-server metadata must return HTTP 200.
- **Not possible today in the consumer Gemini app:**
  - users outside the US, under 18, on work or school accounts, or using a language other than English cannot add TCG Replay;
  - there is no self-service public listing; listing needs acceptance into the gated Connected Apps preview;
  - the preview has no games or hobby category;
  - Gems cannot connect to it, and Gems are being retired.
- **Tool count.** About 45 tools counts as "dozens", which Connected Apps review flags. Hide health-check helpers such as `ping` from the model. The `guide` tool returns text and is not a dispatcher, but it sits close to the ban on "dynamic discovery functions", so make its purpose plain.
- **`wait_for_turn`.** TCG Replay's 15 s default cap (50 s maximum) is well under the shortest documented per-call default (Antigravity's 180 s) and already returns a "call again" result. The consumer app's timeout is unknown.
- **Annotations.** Mark read tools `readOnlyHint: true`. Without annotations Gemini treats every tool as destructive and asks for confirmation every time; on Gemini Enterprise every non-read-only tool asks regardless.
- **Results.** Put the full payload in text content; Connected Apps does not yet support `structuredContent`.
- **Server instructions.** TCG Replay's instructions are procedural. For Connected Apps, cut them to a short description of what the server does and move procedures into tool descriptions, guides and tool results (see [[handoff-in-tool-results]]). Keep fuller instructions for CLI-style clients.

## Checklist for TCG Replay

1. Serve Streamable HTTP at one stable, versioned path (for example `/mcp/v1`); declare protocol version 2025-11-25 or earlier.
2. OAuth: publish `/.well-known/oauth-authorization-server` (HTTP 200); PKCE S256; DCR (`registration_endpoint`), ideally CIMD too; issue refresh tokens without rotating them; add a revocation endpoint.
3. Allowlist redirect URIs as needed: Gemini's `googleusercontent.com` URIs; `https://antigravity.google/oauth-callback`; `https://vertexaisearch.cloud.google.com/oauth-redirect`; localhost for Gemini CLI.
4. Explicit `readOnlyHint`, `destructiveHint` and `openWorldHint` on every tool.
5. Remove developer-only tools from the model-facing list; consider merging tools to get below "dozens".
6. Return JSON as text content; use the documented error codes; keep `wait_for_turn` well under 180 s.
7. Shorten the server instructions to a high-level summary for Connected Apps.
8. Test with MCP Inspector, then in Gemini Developer Mode (gemini.google.com/devmode) from a US personal account.
9. User docs for "Add a custom app" (US-only), Gemini CLI (`gemini mcp add --transport http tcgreplay <url>`) and Antigravity (`serverUrl` in `mcp_config.json`).
10. Optional: a Gemini CLI extension or Antigravity plugin repository, with server name `tcgreplay` (no underscore or hyphen).
11. Before applying to the Connected Apps preview: Terms of Service and privacy policy with data-deletion help; 512 px icon; domain-verification file; reviewer test account with two-factor authentication off (check how this works with Google login); choose a category (permanent).
12. IP: keep "Pokémon" and "Gemini" out of the app name and icon; say "works with Gemini"; add an "unofficial, not affiliated with The Pokémon Company or Google" line; take legal advice on card text and images.

## Sources (all accessed 2026-11-07)

**Official (Google)**
- R1 Gemini release notes: https://gemini.google/release-notes/
- R2 Workspace Updates, *An easier way to customize how Gemini works for you*: https://workspaceupdates.googleblog.com/2026/10/an-easier-way-to-customize-how-gemini-works-for-you.html
- R3 Gemini Help, custom apps: https://support.google.com/gemini/answer/17209137?hl=en&co=GENIE.Platform%3DDesktop (also https://support.google.com/gemini/answer/17209137?hl=en-IN&ref_topic=15157994)
- R4 Gemini Help: https://support.google.com/gemini/answer/13695044?hl=en
- R5 Google blog, *Connected apps in one place*: https://blog.google/innovation-and-ai/technology/developers-tools/connected-apps-one-place/
- R6 Connected Apps: https://developers.google.com/connected-apps
- R7 Server optimisation: https://developers.google.com/connected-apps/mcp/server-optimization
- R8 OAuth: https://developers.google.com/connected-apps/mcp/oauth
- R9 Guidelines: https://developers.google.com/connected-apps/mcp/guidelines
- R11 Quality: https://developers.google.com/connected-apps/mcp/quality
- R12 Vertical journeys: https://developers.google.com/connected-apps/mcp/vertical-journeys
- R13 MCP overview: https://developers.google.com/connected-apps/mcp
- R14 Developer Mode: https://developers.google.com/connected-apps/mcp/dev-mode
- R15 Developer Mode usage: https://developers.google.com/connected-apps/mcp/dev-mode/usage
- R16 Connected Console: https://developers.google.com/connected-apps/gpc
- R17 Prerequisites: https://developers.google.com/connected-apps/gpc/prerequisites
- R18 Perform the steps: https://developers.google.com/connected-apps/gpc/perform-the-steps
- R19 Connected Apps policy: https://developers.google.com/connected-apps/connected-apps-policy
- R20 Google Developers Blog, *Transitioning Gemini CLI to Antigravity CLI*: https://developers.googleblog.com/an-important-update-transitioning-gemini-cli-to-antigravity-cli/
- R22 Gemini CLI extensions: https://geminicli.com/docs/extensions/ and https://geminicli.com/extensions/about/
- R23 Extensions reference: https://geminicli.com/docs/extensions/reference/
- R24 Releasing extensions: https://geminicli.com/docs/extensions/releasing/
- R25 MCP servers in Gemini CLI: https://geminicli.com/docs/tools/mcp-server/
- R26 GEMINI.md: https://geminicli.com/docs/cli/gemini-md/
- R27 Commands: https://geminicli.com/docs/reference/commands/
- R28 Gemini CLI repository: https://github.com/google-gemini/gemini-cli
- R29 Antigravity MCP: https://antigravity.google/docs/mcp/
- R30 Gemini CLI migration: https://antigravity.google/docs/cli/gcli-migration
- R31 Antigravity Marketplace: https://antigravity.google/docs/marketplace/
- R32 Build plugins: https://antigravity.google/docs/build-plugins/
- R33 Gemini Enterprise, custom MCP server: https://docs.cloud.google.com/gemini/enterprise/docs/connectors/custom-mcp-server/set-up-custom-mcp-server
- R34 Gemini Business help: https://support.google.com/g/answer/17106276?hl=en
- R35 Gemini Enterprise editions: https://docs.cloud.google.com/gemini/enterprise/docs/editions
- R36 Register an A2A agent (search summary only): https://docs.cloud.google.com/gemini/enterprise/docs/register-and-manage-an-a2a-agent
- R37 Google Cloud blog, *Dev's guide to MCP Apps in Gemini*: https://cloud.google.com/blog/topics/developers-practitioners/devs-guide-to-mcp-apps-in-gemini
- R38 Google-managed MCP servers: https://docs.cloud.google.com/mcp/supported-products
- R39 Function calling (Interactions API): https://aistudio.google.com/docs/function-calling
- R40 ADK MCP tools: https://adk.dev/tools-custom/mcp-tools/
- R41 Google product co-branding: https://partnermarketinghub.withgoogle.com/brands/google/use-cases/product-co-branding/

**Secondary (gap-filling only; marked Unverified in the text)**
- X1 Zapier, Gemini connected apps: https://zapier.com/blog/gemini-connected-apps/
- X2 TechRepublic, Gemini Spark integrations (403; snippet only): https://www.techrepublic.com/article/news-gemini-spark-app-integrations/
- X3 9to5Google, Gemini Spark eligibility: https://9to5google.com/2026/05/29/gemini-spark-ultra-us/
- X4 9to5Google, Connected Apps platform: https://9to5google.com/2026/10/28/gemini-google-connected-apps-platform/ (and TechCrunch via search)
- X5 IT Brief, MCP Apps in Gemini Enterprise: https://itbrief.co.uk/story/google-adds-interactive-mcp-apps-to-gemini-enterprise
- X6 Insightech, Gemini MCP setup: https://support.insightech.com/article/z488bdf1co-insightech-mcp-gemini-setup

Related: [[publishing-to-chatgpt]], [[publishing-to-claude]], [[handoff-in-tool-results]].
