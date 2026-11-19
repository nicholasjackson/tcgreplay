---
created_date: "2026-11-12"
document_status: final
closed_date: "2026-11-12"
---

# Plan: 20261112091756-c5b6d1fd-launch-docs

<!-- Metadata -->
<!-- Created: 2026-11-12T09:47:33Z -->
<!-- Commit: 6ecc1aa -->
<!-- Branch: main -->
<!-- Repository: tcgreplay (/home/nicj/code/github.com/nicholasjackson/tcgreplay) -->

## Overview

The docs catch up with the public launch. ChatGPT users get connection steps that match ChatGPT's settings at launch and name the plans that allow them, with the Plugin Directory route written in and switched on once OpenAI approves the listing. Every player gets one place that explains how to refresh their assistant's connection after an update, a "What's new" page that says what changed in each release and whether a refresh is needed, and a docs index at the docs' own address, so new or changed tools never leave a player confused.

## Conventions

No project conventions apply to this feature. The only always-applied convention (guide text and packaging: no "Claude" or "Nic " in `internal/guides`, never exclude `internal/guides` from a build context) covers files this plan does not change: all the work is in `site/`, and the docs may name Claude, claude.ai and Claude Code freely.

## Architecture & Design Decisions

All of this work is website content and its build-time checks, in the single registered repo `tcgreplay` (root `/home/nicj/code/github.com/nicholasjackson/tcgreplay`), under `site/`. No Go code changes: the site handler already answers `/docs/` with `docs/index.html` when that file exists (`internal/site/site.go`, `resolve`), so a new docs index page is enough. The plan builds on the domain-move plan, which lands first: the website already takes its address from `site/src/lib/facts.ts` (`site.mcpURL` is `https://tcgreplay.app/mcp`), the connecting page carries one `data-moved-note` element, and a whole-site scan fails the suite on `demo.gs` outside that element or on any `www.tcgreplay.app`. Nothing in this plan names the old address; the "What's new" launch entry points to the connecting page's move note instead.

**The connecting page becomes an Astro page, so the Plugin Directory route can be switched on with one line.** `site/src/content/docs/connect.md` is replaced by `site/src/pages/docs/connect.astro`, which keeps the same URL, title, address block, `h2` sections (`claude.ai`, `ChatGPT`, `Claude Code`, the desktop app, Next), prompts and the domain-move note, and prints every address from `site.*` facts rather than spelling it out. A small site module, `site/src/lib/listing.ts`, holds one flag, `chatgptDirectory.listed`, set to `false`. When it is `false`, the ChatGPT section gives only the custom connection steps, which follow OpenAI's current flow (Plugins, **+**, **Add custom MCP server**, the playmat's address with OAuth, accept the warning, **Create as a plugin**, install, connect with Google, pick it in a chat) and name the ChatGPT plans that allow a custom connection. When it is `true`, the section starts with finding "TCG Replay" in the Plugin Directory and connecting it, and the custom steps follow as the alternative. The directory steps are written now, inside the page, so turning them on after OpenAI approves is a one-line change the maintainer makes. A suite check makes the "not yet" state safe: while the flag is `false`, no built page may contain "Plugin Directory". Markdown cannot branch, which is why the page moves to Astro; a remark plugin, a hidden block or a pasted-in-later note were weighed and rejected (`research.md#alternatives-considered-and-rejected`).

**One refresh section, one dated list.** The connecting page gains a `Refresh after an update` section (`id="refresh"`) with a short paragraph on when a refresh is needed (a new or changed tool, guide or instructions) and an `h3` per assistant: claude.ai (remove the connector and add it again, then start a new chat), ChatGPT (open the plugin in Plugins and choose **Refresh**, then a new chat; once the directory route is on, a directory install updates on its own) and Claude Code (`/mcp`, choose tcgreplay, **Reconnect**, or restart). Each assistant's existing "if it can't reach the playmat" line gains "if a tool seems missing after an update, refresh the connection", linking to `#refresh`. "What's new" entries live in one YAML list, `site/src/data/whats-new.yaml`, newest first, each with a `date`, a `title`, a `refresh` flag and a list of `changes`. A loader, `site/src/lib/whats-new.ts`, follows the facts pattern (`site/src/lib/facts.ts`): it parses the file with the `yaml` package the site already uses, sorts entries newest first, and throws a named error that stops the build on a missing or malformed date, title, flag or change list. Keeping the data in the project rather than in Markdown files or GitHub releases means adding an entry is a few lines in one file, and the build checks the shape. The page `site/src/pages/docs/whats-new.astro` renders each entry with its date and a plain "Refresh needed" or "No refresh needed" line. "Refresh needed" links to `/docs/connect#refresh`. Its first entry is the public launch.

**Navigation and the docs index share one list.** The sidebar's list of docs pages moves out of `site/src/layouts/Docs.astro` into a helper, `site/src/lib/docs-pages.ts`, which returns each page's link, title, description and order. The sidebar renders it, preceded by a link to the docs index, and gains "What's new" and "Refreshing the connection" (`/docs/connect#refresh`). A new `site/src/pages/docs/index.astro` renders the same list with each page's one-line description, so `/docs` and `/docs/` show a docs index instead of the 404 page, and the index can never fall out of step with the sidebar. The header navigation in `site/src/layouts/Base.astro` is left alone: the spec asks for docs navigation, and the privacy-terms spec edits the shared layout's links.

**Keeping the list going is a convention, checked by a person.** The spec's second success metric (every player-visible release adds an entry) is upheld by a written convention, added to the knowledge base during implementation: a change players can see ships with an entry, flagged `refresh: true` when it adds, removes or changes a tool, a tool's description, a guide or the server's instructions. An automated guard that compares tool lists across releases was rejected, because most player-visible changes are not tool changes (`research.md#alternatives-considered-and-rejected`). The steps for each assistant come from OpenAI's and Anthropic's current documentation and the knowledge base. Which ChatGPT plans allow a custom connection, and claude.ai's refresh, are not settled in official sources, so a person checks every set of steps in the real assistants before launch.

## Component Breakdown

- **Connecting page (changed: Markdown to an Astro page).** Owns how a player connects each assistant and how they refresh it. It keeps its address, title, address block, per-assistant sections, check-it-works prompts and the domain-move note, which gains an anchor so other pages can link to it. The ChatGPT section gives the current custom connection steps and the plans that allow them, and starts with the Plugin Directory steps only when the listing flag says TCG Replay is listed. It gains the one refresh section, and each assistant's troubleshooting line links to it. It reads every address from the website facts and the listing state from the listing flag.
- **Listing flag (new).** Owns one fact: whether TCG Replay is listed in ChatGPT's Plugin Directory, and the name to search for. It starts as "not listed". The connecting page, the refresh section and the whole-site scan all read it, so switching it after OpenAI approves turns the directory route on everywhere at once. It is separate from the website facts, which describe what the playmat's own deployment sets, because the listing is a website decision.
- **What's new list (new).** Owns the dated record of player-visible changes: one data file with an entry per release (date, title, refresh needed or not, the changes), kept newest first. Adding an entry is a few lines in this one file.
- **What's new loader (new).** Owns reading and checking that list while the site builds, in the same style as the website facts: it sorts entries newest first and stops the build with a message naming the entry and field when a date, title, refresh flag or change list is missing or malformed. Only the What's new page uses it.
- **What's new page (new).** Renders the list: each entry's date, title, changes and whether a refresh is needed, with "Refresh needed" linking to the connecting page's refresh section. Its first entry is the public launch, which points players on the old connection to the move note.
- **Docs page list (new, extracted from the docs layout).** Owns the one list of docs pages (link, title, one-line description, order), from the Markdown docs plus the hand-written pages. The sidebar and the docs index both render it, so they cannot drift. It gains What's new, the connecting page (now hand-written) and a "Refreshing the connection" link to the refresh section.
- **Docs layout (changed).** Its sidebar renders the docs page list, with a link to the docs index at the top. Otherwise unchanged.
- **Docs index (new).** The page served at the docs address. It lists every docs page from the docs page list with its one-line description, so the docs address shows an index instead of the 404 page. The website's file handler already serves it at both `/docs` and `/docs/`; it needs no change.
- **Website browser suite (changed).** The connecting-page tests follow the new page: custom ChatGPT steps and named plans, the refresh section with all three assistants, troubleshooting links to it, and directory steps shown or absent according to the listing flag. New tests cover the What's new loader and page, the sidebar and index links, and `/docs/` answering the index. The domain-move whole-site scan gains one rule: while the listing flag says "not listed", no built page mentions the Plugin Directory.
- **What's new convention (new knowledge entry).** Owns the rule that every player-visible change ships with a What's new entry, flagged as needing a refresh when it adds, removes or changes a tool, a tool's description, a guide or the server's instructions. Written to the knowledge base so future plans and implementations apply it.
- **Header navigation (unchanged).** The site-wide header keeps its links; the docs sidebar and index are the docs navigation the spec asks for.

## Data Structures & Interfaces

No persisted data, wire format, Go type or playmat interface changes. The plan adds one data file and three small build-time contracts inside the website.

**What's new list (data file).** A YAML list, kept newest first, one entry per release that changes what players see:

```yaml
- date: 2026-11-20          # the day it shipped, YYYY-MM-DD
  title: TCG Replay is public
  refresh: true             # do players need to refresh their assistant's connection?
  changes:                  # one short line per player-visible change; inline links allowed
    - "The playmat has its own address, tcgreplay.app. ..."
```

**What's new loader.**

```ts
export type WhatsNewEntry = { date: string; title: string; refresh: boolean; changes: string[] };
export function loadWhatsNew(yamlText: string, source?: string): WhatsNewEntry[]  // newest first; throws WhatsNewError
export const whatsNew: WhatsNewEntry[]                                            // the project's list, loaded at build
```

It is the only reader of the data file. The What's new page renders `whatsNew`; tests call `loadWhatsNew` on fixtures.

**Listing flag.**

```ts
export const chatgptDirectory = { listed: false, name: 'TCG Replay' } as const;
```

The connecting page and the browser suite read it. Switching on the directory route is changing `listed` to `true`.

**Docs page list.**

```ts
export type DocsPage = { href: string; title: string; description?: string; order: number };
export async function docsPages(): Promise<DocsPage[]>   // lowest order first, then by title
```

The docs layout's sidebar and the docs index render it.

**Page anchors other pages link to.** `/docs/connect#refresh` is the refresh section, linked from What's new entries, the sidebar and each assistant's troubleshooting line. `/docs/connect#moved` is the domain-move note, linked from the launch entry. Both are stable `id`s on the connecting page.

## Implementation Detail

**Existing patterns followed.**
- The What's new loader copies the website facts' shape: a pure function that takes the file's text and returns checked values or throws an error naming the problem, plus a module-level value loaded once from a path relative to the site project (never from the module's own URL, which points into Astro's prerender bundle). The browser suite tests it from Node with fixtures, as it already tests the facts.
- New docs pages are hand-written Astro pages in the docs layout, like the limits page and the coaching-workflows index, and join the sidebar through the same list of hand-written pages.
- Example prompts on the connecting page use the existing prompt component, which renders the same copy block the Markdown prompt fence produced, so the browser suite's prompt checks keep working.
- Every address the connecting page prints comes from the website facts, so the domain-move scan and the facts stay the single source of the address.

**Changed shape: the connecting page leaves the Markdown docs.** The busiest docs page becomes a hand-written page. A reader opening it sees plain markup in the same order as the page they know: the address block, one section per assistant, the desktop app, the refresh section and "Next". The only logic is one condition on the listing flag around the directory steps. The page keeps its address, its sidebar position and its `h2` headings, so links and tests that name sections still work. The Markdown docs collection keeps its other pages unchanged; its "show the address" option stays but no page uses it once this page leaves.

**New pattern: content switched by a site flag.** Until now no page changed with a setting. The listing flag is a typed constant in its own small module, not an environment variable, so the switch is a reviewed one-line commit and the built site is the same wherever it is built. The browser suite reads the same flag and asserts the matching state: directory steps first when listed, and no "Plugin Directory" anywhere when not. Its tests stay valid on both sides of the switch.

**New pattern: release notes as checked data.** "What's new" is the project's first changelog. Entries are data, not prose pages: a developer shipping a player-visible change adds a few lines to one file, and the build tells them if a field is missing. Entries say what changed for players, in the docs' plain voice, not what changed in the code. Whether an entry needs a refresh is a required yes or no, so nobody has to guess. The rule for when to add one, and when to mark it as needing a refresh, goes into the knowledge base as a convention.

**Shared docs list.** The sidebar's inline list moves into a helper that both the sidebar and the new docs index render. A developer adding a hand-written docs page adds it in one place and it appears in both.

## Dependencies

- Design documents this plan was built on: none. The spec carries no design references.
- **Upstream spec `20261111093353-8ff3589f-domain-move` (must land first).** Provides the website's address from the deployment definition (`site.mcpURL` = `https://tcgreplay.app/mcp`), the connecting page's single `data-moved-note` move note, and the whole-site scan of built pages. This plan converts the connecting page that plan edits, so its tasks must be merged before this plan's connecting-page task starts; the move note is carried across unchanged apart from gaining an anchor, and the scan gains one rule.
- **Sibling spec `20261112090820-07d70cfb-chatgpt-submission-prep`.** Owns the plugin package, the listing name "TCG Replay" and the submission to OpenAI. No ordering dependency for drafting; the directory route can only be switched on after OpenAI approves the listing that spec submits. Its tool-label changes are player-visible and belong in the launch entry if they ship by launch.
- **Sibling spec `20261112083238-29247b36-privacy-terms-disclaimer`.** Edits the shared page layout's links (privacy, terms, help). This plan stays out of the shared layout to avoid a conflict. Its terms and privacy pages and the board's About panel (opened from the ☰ menu) are player-visible and belong in the launch entry; that spec's own What's new criterion was accepted as handed to this plan's launch entry.
- **Website toolchain: Astro 7, Playwright, `yaml` (already in the site project).** Astro builds the new pages; `yaml` parses the What's new list; Playwright runs the browser suite and the loader tests. No new packages.
- **Go site handler (internal `site` package), unchanged.** Already serves `docs/index.html` for `/docs` and `/docs/`.
- **OpenAI's current "Add custom MCP server" flow and help articles (external).** The source for the ChatGPT custom steps, the plans named and ChatGPT's Refresh. Not stable; a person checks the steps against them and a real account at the launch release.
- **Anthropic's connector documentation and the knowledge entries `learnings/publishing-to-chatgpt.md` and `learnings/publishing-to-claude.md`.** Sources for the claude.ai and Claude Code refresh steps; claude.ai's is undocumented, so a person confirms it.
- **OpenAI's approval of the Plugin Directory listing (external, gates the last task).** The directory route is switched on only after approval, when the maintainer chooses.

## Testing Approach

All automated tests are in the website's existing Playwright suite, which runs against the built site served through the Go site handler, as the hosted playmat serves it. No Go tests change: the Go site handler is untouched, and its existing tests keep covering file resolution and the 404 page.

**Loader tests (Node, no browser), in the style of the existing facts tests.**
- The project's real What's new list loads, its entries come out newest first, and its first entry is the public launch, with `refresh` set.
- A fixture with entries out of order comes back newest first.
- A fixture missing a date, with a date that is not a real `YYYY-MM-DD` day, missing a title, missing or non-yes/no `refresh`, or with an empty change list fails with a message naming the entry and the field.

**Browser tests (most coverage: the connecting page and navigation, where players land).**
- **Connecting page.** The existing per-assistant checks keep passing on the Astro page: each of claude.ai, ChatGPT and Claude Code has numbered steps using `site.mcpURL`, a "Check it works" block with a prompt, and what to expect; the address and Claude Code command copy exactly. The ChatGPT section names the plans that allow a custom connection and includes the "Create as a plugin" step. Each assistant's troubleshooting text links to `#refresh`.
- **Directory switch.** The test reads the listing flag. While not listed, the ChatGPT section's first numbered steps are the custom connection steps, and no built page contains "Plugin Directory" (added to the domain-move whole-site scan). When listed, the ChatGPT section's first numbered list finds "TCG Replay" in the Plugin Directory and a later list gives the custom steps. Only the current state is exercised on each build; the other is exercised the day the flag changes.
- **Refresh section.** `/docs/connect#refresh` exists and has a sub-section for each of claude.ai, ChatGPT and Claude Code, each with at least one step.
- **What's new page.** It loads, shows dated entries newest first (the dates on the page are in descending order), its first entry is the launch, every entry says "Refresh needed" or "No refresh needed", and every "Refresh needed" links to `/docs/connect#refresh`.
- **Navigation.** The docs sidebar on every docs page links to the docs index, What's new and `/docs/connect#refresh`. The docs index lists every sidebar page. `/docs/` and `/docs` both answer 200 with the index, not the 404 page.
- **Whole site.** The existing crawl reaches the new pages, so they pass the shared layout, no-sideways-scroll, no-cookie and no-third-party checks, and every link on them resolves. The domain-move scan keeps the old address out of the new pages.

**Deliberate gaps.**
- No automated test of the steps inside claude.ai, ChatGPT or Claude Code: their settings screens are external and change without notice. They are checked by hand below.
- The listed state of the directory route is not built in CI before approval; its test runs once the flag is switched.

**Spec acceptance criteria that need the real assistants.** Each is **Manual — captured in the implementation test plan**:
- **Custom steps work.** At the launch release, following the ChatGPT custom connection steps from an account on a plan the page names connects TCG Replay and its tools appear in a chat; the plan list on the page matches OpenAI's help page at that date.
- **Refresh steps work.** After a release that adds or changes a tool, following the refresh steps in each of claude.ai, ChatGPT and Claude Code makes the new or changed tool appear.
- **Directory steps after approval.** Once the listing is approved and the flag switched, following the ChatGPT section's first steps finds "TCG Replay" in the Plugin Directory and connects it, and the custom steps still follow.

**Success metrics.**
- **No GitHub issue in the first month asks a ChatGPT-connection or missing-tools question the docs already answer.** **Manual — captured in the implementation test plan.** The maintainer reviews the month's GitHub issues against the connecting page.
- **Every release after launch that changes what players see adds a What's new entry.** **Manual — captured in the implementation test plan.** The maintainer compares the deploys to main since launch with the What's new list. The convention entry and the build check on entry shape support it, but no test can tell whether a change was player-visible.

**Manual reviews.**
- **Docs wording review.** **Manual — captured in the implementation test plan.** The maintainer reads the connecting page, the refresh section, the What's new page and the launch entry, and confirms they read clearly and the launch entry lists what actually shipped.

## Milestones & Tasks

### Milestone 1: The docs address opens a docs index

**What changes**: Visiting the docs' own address, `/docs/` or `/docs`, shows an index of every docs page with a one-line description of each, instead of "Page not found". The docs sidebar gains a link to that index at the top. The sidebar and the index are built from one list, so a page added to the docs appears in both.

**Validation point**: The website builds and its browser suite passes, including new checks that `/docs/` and `/docs` answer with the index, that the index lists every sidebar page, and that the sidebar links to the index.

#### - [x] Task: Docs index and shared docs page list
**Id:** e8d473af-a98a-48b1-9c9e-dffb1df1de33
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

The docs sidebar's list of pages moves into one shared list, which the sidebar and a new docs index page both render. The index is served at the docs' own address, so `/docs/` and `/docs` show every docs page with its one-line description instead of "Page not found". The sidebar gains a link to the index at the top.

*Technical detail:* [context.md#task-docs-index-and-shared-docs-page-list](./context.md#task-docs-index-and-shared-docs-page-list)

**Acceptance criteria**:
- [x] Visiting `/docs/` or `/docs` on the website shows a page listing the docs, not the "Page not found" page.
- [x] The index lists every page the docs sidebar lists, each with its title linking to the page and a one-line description where the page has one.
- [x] Every docs page's sidebar links to the docs index, and still lists the same pages in the same order as before.

### Milestone 2: ChatGPT users can connect at launch, and every player can refresh their connection after an update

**What changes**: The connecting page's ChatGPT section gives the steps that match ChatGPT's settings at launch (add a custom MCP server from Plugins, create it as a plugin, install and connect it) and says which ChatGPT plans allow it. The page gains one "Refresh after an update" section with steps for claude.ai, ChatGPT and Claude Code, so a player who can't see a new or changed tool knows what to do, and each assistant's troubleshooting line points there. The docs sidebar links straight to the refresh steps. The Plugin Directory steps are written into the page but stay switched off, and no page mentions the Plugin Directory until they are switched on.

**Validation point**: The website builds and its browser suite passes: the ChatGPT section names its plans and starts with the custom steps, the refresh section covers all three assistants, the troubleshooting lines and sidebar link to it, and no built page mentions the Plugin Directory.

#### - [x] Task: Connecting page with current ChatGPT steps and refresh steps
**Id:** 54cc9ee6-8c80-45da-9594-2d9ff755ccea
**Repo:** tcgreplay
**Depends on:**
- e8d473af-a98a-48b1-9c9e-dffb1df1de33 — Docs index and shared docs page list
**Execution:** agent

The connecting page becomes a hand-written page at the same address, keeping everything it shows today, including the note for players on the old address. Its ChatGPT section is rewritten to ChatGPT's current flow for adding a custom MCP server as a plugin, and says which ChatGPT plans allow it. A new "Refresh after an update" section gives steps for claude.ai, ChatGPT and Claude Code, each assistant's troubleshooting line points to it, and the docs sidebar links to it. The Plugin Directory steps are written into the page behind a site flag that starts switched off, and the suite checks that no page mentions the Plugin Directory while it is off. This task starts only once the domain-move work has been merged.

*Technical detail:* [context.md#task-connecting-page-with-current-chatgpt-steps-and-refresh-steps](./context.md#task-connecting-page-with-current-chatgpt-steps-and-refresh-steps)

**Acceptance criteria**:
- [x] The connecting page keeps its address, title, address block, claude.ai and Claude Code steps, check-it-works prompts and its one note for players on the old address, and every address it shows is `https://tcgreplay.app` or derived from it.
- [x] The ChatGPT section gives numbered steps for adding the playmat as a custom MCP server and creating it as a plugin, then installing and connecting it, and names the ChatGPT plans that allow a custom connection.
- [x] The page has one refresh section with steps for claude.ai, ChatGPT and Claude Code, and each assistant's troubleshooting text links to it.
- [x] The docs sidebar and the docs index link to the refresh section.
- [x] Plugin Directory steps for ChatGPT exist in the page but are not shown, and no built page mentions the Plugin Directory while the listing flag is off; turning the flag on puts them first in the ChatGPT section, with the custom steps after.

### Milestone 3: Players can see what changed in each update

**What changes**: A new "What's new" page lists player-visible changes, newest first, each dated and saying whether players need to refresh their assistant's connection, with a link to the refresh steps when they do. Its first entry covers the public launch, including the new address and where to reconnect. It is linked from the docs sidebar and the docs index. Adding an entry for a later release is a few lines in one file, and the build stops if an entry is missing its date, title, refresh answer or changes. The rule for adding entries is written down for future work.

**Validation point**: The website builds and its browser suite passes, including the loader checks and a What's new page whose entries are dated, newest first, start with the launch and link "Refresh needed" to the refresh steps.

#### - [x] Task: What's new page and launch entry
**Id:** 244d82ad-183d-4a8b-a9ff-2deee9b45c80
**Repo:** tcgreplay
**Depends on:**
- e8d473af-a98a-48b1-9c9e-dffb1df1de33 — Docs index and shared docs page list
- 54cc9ee6-8c80-45da-9594-2d9ff755ccea — Connecting page with current ChatGPT steps and refresh steps
**Execution:** agent

A single dated list of player-visible changes is added to the website project, read and checked while the site builds, and shown on a new "What's new" page, newest first. Each entry says whether players need to refresh their assistant's connection, and links to the refresh steps when they do. The first entry covers the public launch, including the move to tcgreplay.app with a link to the connecting page's note. The page joins the docs sidebar and index, and the rule for adding an entry with each player-visible release is written into the project's knowledge base.

*Technical detail:* [context.md#task-whats-new-page-and-launch-entry](./context.md#task-whats-new-page-and-launch-entry)

**Acceptance criteria**:
- [x] The website has a "What's new" page, reachable from the docs sidebar and the docs index, listing dated entries newest first.
- [x] Every entry says whether a refresh is needed, and every entry that needs one links to the refresh steps on the connecting page.
- [x] The first entry covers the public launch, links to the connecting page's note for players on the old address, does not name the old address and does not say TCG Replay is in the Plugin Directory.
- [x] The first entry lists the privacy-terms-disclaimer spec's player-visible changes: the terms page, the updated privacy page and the board's About panel.
- [x] An entry missing its date, title, refresh answer or changes, or with an invalid date, stops the site build with a message naming the entry and what is wrong.
- [x] The knowledge base has a convention saying every player-visible change ships with a What's new entry, and when to mark it as needing a refresh.

### Milestone 4: ChatGPT users find TCG Replay in the Plugin Directory

**What changes**: Once OpenAI has approved the listing and the maintainer chooses to, the ChatGPT section starts with finding "TCG Replay" in the Plugin Directory and connecting it, with the custom connection steps kept below as the alternative, and the refresh section tells directory users their plugin updates on its own. This milestone waits on OpenAI's approval.

**Validation point**: After the switch, the website builds and its browser suite passes in the listed state: the ChatGPT section's first steps use the Plugin Directory and the custom steps follow.

#### - [x] Task: Switch on the Plugin Directory route
**Id:** c3c0c268-bdf7-4650-bcf8-c80327d7380c
**Repo:** tcgreplay
**Depends on:**
- 54cc9ee6-8c80-45da-9594-2d9ff755ccea — Connecting page with current ChatGPT steps and refresh steps
- 244d82ad-183d-4a8b-a9ff-2deee9b45c80 — What's new page and launch entry
**Execution:** human — waits on OpenAI approving the Plugin Directory listing (an approval outside the repo) and on the maintainer choosing when to switch, and needs a ChatGPT account to confirm the live directory wording

After OpenAI approves the listing, the maintainer checks the drafted directory steps against the live Plugin Directory, adjusts their wording if needed, switches the listing flag on and adds a What's new entry saying TCG Replay can now be found in the directory. The change deploys like any other.

*Technical detail:* [context.md#task-switch-on-the-plugin-directory-route](./context.md#task-switch-on-the-plugin-directory-route)

**Acceptance criteria**:
- [ ] The ChatGPT section starts with finding "TCG Replay" in the Plugin Directory and connecting it, and the custom connection steps follow as the alternative.
- [ ] The refresh section tells players who installed from the directory that their plugin updates on its own.
- [ ] A What's new entry announces the directory listing.

## Open Questions

- **Which ChatGPT plans actually allow a write-capable custom MCP server at launch?** OpenAI's official help names Business, Enterprise and Edu; third-party guides add Plus and Pro, and one says Pro is limited to read and fetch. This depends on OpenAI's live rollout and can only be settled with a real account at the launch release (the manual check "Custom steps work"). If a plan the page names cannot connect, or a plan it leaves out can, correct the plans paragraph on the connecting page and its test to match what was observed. If no individual plan can connect at all, STOP and ask the user, because that changes who the launch can reach.
- **Does removing and re-adding the connector refresh claude.ai's tool list, and does Reconnect in `/mcp` refresh Claude Code's?** Anthropic documents neither for claude.ai; it depends on the assistants' live behaviour after a release that changes a tool (the manual check "Refresh steps work"). If a step does not make the new tool appear, replace it with the step that does and update the page. If no step works in an assistant, STOP and ask the user how the docs should handle it.

## Out of Scope

- **Changing the addresses in the docs.** The move to tcgreplay.app, the connecting page's move note and the old-address scan belong to the domain-move spec (`20261111093353-8ff3589f-domain-move`); this plan builds on them.
- **The privacy and terms pages, and help links on every page.** The privacy, terms and disclaimer spec (`20261112083238-29247b36-privacy-terms-disclaimer`). This plan also leaves the site-wide header and footer alone for that reason.
- **The ChatGPT listing text, plugin package and submission.** The ChatGPT submission prep spec (`20261112090820-07d70cfb-chatgpt-submission-prep`). This plan only writes the docs' directory steps and switches them on after approval.
- **Listing TCG Replay in Claude's connector directory.** Not planned (spec Non-Goals); the claude.ai steps stay as a custom connector.
- **Rewriting the claude.ai and Claude Code connection steps.** Only the ChatGPT steps are brought up to date; the other two change only to link to the refresh steps (and their addresses, by the domain-move plan).
- **An automated check that each release adds a What's new entry.** The rule is a written convention, checked by the maintainer (a success metric in the test plan), because no test can tell whether a change was visible to players.
- **A "What's new" feed (RSS or similar) or in-assistant announcements of updates.** Not asked for; the page and its links are the whole surface.
- **Entries for releases before launch.** The list starts with the public launch, as the spec asks.

## Changelog

### 2026-11-19 — Task: Docs index and shared docs page list

**What was done**: The docs sidebar's list of pages moved from the docs layout into a shared helper, `docsPages()`, which also carries each page's one-line description. A new docs index page at `/docs` (and `/docs/`) renders that same list with descriptions, and the sidebar now starts with an "All docs" link to it.

**Deviations**: None.

**Files changed**:
- `site/src/lib/docs-pages.ts`
- `site/src/layouts/Docs.astro`
- `site/src/pages/docs/index.astro`
- `site/e2e/docs-index.spec.js`

**Discoveries**: `site/e2e/docs-index.spec.js` holds a hand-maintained list of the sidebar's pages in order; later tasks that add docs pages (Refreshing the connection, What's new) must add them to that list too.

### 2026-11-19 — Task: Connecting page with current ChatGPT steps and refresh steps

**What was done**: The connecting page moved from Markdown to a hand-written Astro page at the same address, keeping its address block, claude.ai, Claude Code and desktop sections, check prompts and the move note (now with `id="moved"`), with every new address printed from `site.mcpURL`. The ChatGPT section now names the plans (Business, Enterprise or Edu) and gives OpenAI's current custom-server flow through "Create as a plugin" and Install; the Plugin Directory steps sit behind `chatgptDirectory.listed` (off) in a new `site/src/lib/listing.ts`. A new "Refresh after an update" section (`#refresh`) covers claude.ai, ChatGPT and Claude Code, each assistant's troubleshooting line links to it, and the sidebar and docs index gained "Refreshing the connection".

**Deviations**: The refresh section's sentence linking to What's new (`/docs/whats-new`) was left out of this task, because that page does not exist until the next task and the site crawl fails on a link to a missing page; the What's new task adds it. The plan list also lives in `listing.ts` as `chatgptPlans`, which the page and test both read (the context offered this as an option). The ChatGPT troubleshooting line now says to open tcgreplay in Plugins and connect it again, matching the new flow.

**Files changed**:
- `site/src/pages/docs/connect.astro` (new)
- `site/src/content/docs/connect.md` (deleted)
- `site/src/lib/listing.ts` (new)
- `site/src/lib/docs-pages.ts`
- `site/src/content.config.ts`
- `site/e2e/connect.spec.js`
- `site/e2e/site.spec.js`
- `site/e2e/docs-index.spec.js`

**Discoveries**: The listed state was exercised once by flipping the flag locally (connect and docs-index suites pass), then restored. Astro text in `.astro` pages keeps straight apostrophes, while the Markdown docs got smart quotes; tests matching apostrophes should accept both.

### 2026-11-19 — Task: What's new page and launch entry

**What was done**: Added the What's new list `site/src/data/whats-new.yaml` and a loader, `site/src/lib/whats-new.ts`, built like the facts loader. It sorts entries newest first and stops the build with a `WhatsNewError` naming the entry and field when a date, title, refresh flag or change list is missing or malformed. `changeHTML` escapes each change line and turns only `/` and `https://` links into links. The new `/docs/whats-new` page shows each entry's date in words, its title, a "Refresh needed" line (linking `/docs/connect#refresh`) or "No refresh needed", and its changes. It is in the docs sidebar and index at order 85, and the refresh section links to it. The launch entry, dated 2026-11-19, covers:
- the new address, linking the `#moved` note;
- the ChatGPT steps;
- What's new, the refresh steps and the docs index;
- clearer tool names and the new account tool (chatgpt-submission-prep);
- the terms and privacy pages with footer links, and the board's About panel (privacy-terms-disclaimer).

**Deviations**: The refresh section's What's new link, held back from the previous task, was added here. Launch-entry tests read the oldest entry (`whatsNew.at(-1)`), not `whatsNew[0]`, so they stay valid once later entries are added above it. Hand-written pages were reflowed so no inline tag starts a new source line, because Astro dropped the space before it (see Discoveries). The launch date is the day this task ran; the maintainer corrects it in the wording review if launch day differs.

**Files changed**:
- `site/src/data/whats-new.yaml` (new)
- `site/src/lib/whats-new.ts` (new)
- `site/src/pages/docs/whats-new.astro` (new)
- `site/src/lib/docs-pages.ts`
- `site/src/pages/docs/connect.astro`
- `site/e2e/whats-new.spec.js` (new)
- `site/e2e/docs-index.spec.js`

**Discoveries**:
- In `.astro` pages, a line break between text and an inline element (`<a>`, `<strong>`, `<code>`) is dropped rather than collapsed to a space, so "needed,\n<a>" builds as "needed,<a>". Fixed on the new pages. `site/src/pages/docs/example.astro` still has one instance, outside this plan's scope. Written to knowledge as `gotchas/astro-inline-whitespace.md`.
- The `yaml` package's default schema keeps unquoted dates as strings.
- Knowledge entries in the always-applied `conventions` category cannot carry tags; the CLI reports them as unreachable.

### 2026-11-19 — Task: Switch on the Plugin Directory route

**What was done**: No code changes. This is a person's task (`Execution: human`): it waits on OpenAI approving the Plugin Directory listing (submitted by chatgpt-submission-prep) and on the maintainer choosing when to switch. The directory steps and the refresh-section note are already written into `site/src/pages/docs/connect.astro`, behind `chatgptDirectory.listed`. When the time comes, the maintainer:
1. checks those steps against the live directory and adjusts their wording;
2. sets `listed: true` in `site/src/lib/listing.ts`;
3. adds a What's new entry announcing the listing (`refresh: false`);
4. runs `make site-test`, which switches to asserting the listed state, and merges.

The steps are in the implementation test plan.

**Deviations**: Skipped, deferred to the maintainer: "deferred to the maintainer: live setup after deploy (waits on OpenAI approving the Plugin Directory listing)". The goal check ran three times; each time the reviewer judged all three criteria not met, because the flag is off and must stay off until approval. The skip was recorded through `implement goal decide`. The three acceptance criteria are left unticked:
- the ChatGPT section starts with the directory steps;
- the refresh section has the directory note;
- a What's new entry announces the listing.

The listed state's rendering was exercised once during the connecting-page task by flipping the flag locally, and the connect and docs-index suites passed.

**Files changed**:
- None

**Discoveries**: None.
