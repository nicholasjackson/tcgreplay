---
created_date: "2026-11-12"
document_status: final
closed_date: "2026-11-12"
---

# Context: 20261112091756-c5b6d1fd-launch-docs

## Current State Analysis

All work is in the single registered repo `tcgreplay` (root `/home/nicj/code/github.com/nicholasjackson/tcgreplay`), under `site/`.

- **Connecting page.** `site/src/content/docs/connect.md:1-70` is Markdown in the `docs` collection (`site/src/content.config.ts:15-24`), rendered by `site/src/pages/docs/[...slug].astro:1-19`, which adds the address `CopyBlock` because of `showAddress: true` (the only page using it). Sections: `## claude.ai`, `## ChatGPT`, `## Claude Code`, `## The Claude desktop app`, `## Next`. The ChatGPT steps say "Plugins → Add → Add custom MCP server", without the risk warning, "Create as a plugin", installing, or plans. There are no refresh steps. Prompts are ```prompt fences that `site/src/scripts/copy.ts:11-29` turns into copy blocks.
- **After domain-move lands** (precondition): `site/src/lib/facts.ts` reads `TCGREPLAY_PUBLIC_URL` from `.do/app.yaml` (`site.mcpURL` = `https://tcgreplay.app/mcp`); `connect.md` names tcgreplay.app and carries one `<aside class="note" data-moved-note>` after the intro; `site/e2e/connect.spec.js` compares against `site.*`; `site/e2e/site.spec.js` scans built HTML and the README for `demo.gs` outside that aside and for `www.tcgreplay.app`.
- **Docs navigation.** `site/src/layouts/Docs.astro:12-20` builds the sidebar (`<nav aria-label="Guide">`) inline from the collection plus `extra` hand-written pages (Coaching workflows 50, Example game 70, Limits 80); collection docs: Get started 10, Signing in 20, Desktop 30, Importing 40, Rewind 60, For developers 90. Header nav `site/src/layouts/Base.astro:21-28` (aria-label "Site").
- **Docs index.** None. `/docs/` resolves in `internal/site/site.go` `resolve` to `docs` → `docs/index.html` (missing) → `404.html` with status 404.
- **What's new.** Nothing exists; no CHANGELOG, no tags. `.github/workflows/deploy.yml` deploys every push to `main` (outside `cardsets/`), so each merge is a release.
- **Tests.** Playwright suite in `site/e2e/` against the built site through `go run ./internal/site/sitetest/cmd/serve` (`site/playwright.config.js`), laptop and phone projects. `site/e2e/facts.spec.js` tests TS loaders from Node. `make site-test` builds and runs it.

## Per-Task Technical Notes

### Task: Docs index and shared docs page list

All changes in repo `tcgreplay` (root `/home/nicj/code/github.com/nicholasjackson/tcgreplay`). Requirement covered: "The docs address opens the docs", and the navigation half of "Easy to find".

**File changes**
- `site/src/lib/docs-pages.ts` (new) — export `type DocsPage = { href; title; description?; order }` and `async function docsPages(): Promise<DocsPage[]>`. Move the `extra` array and the collection mapping from `site/src/layouts/Docs.astro:12-20` here, adding a `description` to each `extra` item (Coaching workflows: reuse the text of `site/src/pages/docs/workflows/index.astro`'s `description`; Example game: from `site/src/pages/docs/example.astro`'s `description`; Limits: from `site/src/pages/docs/limits.astro`'s `description`) and `description: d.data.description` for collection docs. Sort as today: `order` ascending, then `title.localeCompare`. Import `getCollection` from `astro:content` (allowed in a `.ts` module used by pages).
- `site/src/layouts/Docs.astro:12-20,26-32` — replace the inline list with `const items = await docsPages();`. Render, before the list, a first `<li><a href="/docs" aria-current={here === '/docs' ? 'page' : undefined}>All docs</a></li>`. The `here` comparison at `:21` already strips a trailing slash, so `/docs/` marks it current too.
- `site/src/pages/docs/index.astro` (new) — `Docs` layout, `title="Docs"`, `description="Every page of the tcgreplay docs: connecting your assistant, importing and reviewing games, limits and what's new."`. `<h1>Docs</h1>`, a one-line intro, then `<ul class="docs-index" data-docs-index>` with one `<li>` per `docsPages()` item: `<a href={href}>{title}</a>` and, when present, `<p>{description}</p>`. Style like `site/src/pages/docs/workflows/index.astro`'s `.workflows` list. With `build.format: 'directory'` (`site/astro.config.mjs:10`) this builds `internal/site/dist/app/docs/index.html`; `internal/site/site.go` `resolve` tries `docs/index.html` for both `/docs` and `/docs/`, so no Go change.
- `site/e2e/docs-index.spec.js` (new) — tests: (1) `request.get('/docs/')` and `request.get('/docs')` both return 200, `text/html; charset=utf-8`, and the body is not the built `404.html` (read it as `site.spec.js:94` does) and contains `data-docs-index`; (2) on `/docs/`, the `h1` is "Docs" and the hrefs in `[data-docs-index] a` equal the hrefs in the `Guide` navigation (`page.getByRole('navigation', { name: 'Guide' })`) minus its first `/docs` link, in the same order; (3) on `/docs/connect` the `Guide` navigation's first link is "All docs" with href `/docs`.

**Complexity**: Low
**Token estimate**: ~20k tokens
**Agent strategy**: Single agent, sequential. Build with `make site-test` (site build plus the browser suite); `site/e2e/site.spec.js`'s crawl picks the new page up and checks layout, no sideways scroll, cookies and third-party requests. `site/e2e/limits.spec.js:48-53,64-66` assert sidebar links by name and must keep passing.

### Task: Connecting page with current ChatGPT steps and refresh steps

All changes in repo `tcgreplay`. Requirements covered: "ChatGPT custom connection steps are current", "Directory route once listed" (drafted, switched off), "Refresh steps for each assistant", and "Easy to find" for the refresh steps. **Precondition:** the domain-move tasks `336cf3bc` and `92eaa296` are merged, so `site/src/lib/facts.ts` exports `site.mcpURL` = `https://tcgreplay.app/mcp`, `site/src/content/docs/connect.md` carries the `<aside class="note" data-moved-note>` note, and `site/e2e/site.spec.js` has the whole-site old-address scan. If they are not merged, stop.

**File changes**
- `site/src/lib/listing.ts` (new) — `export const chatgptDirectory = { listed: false, name: 'TCG Replay' } as const;` with a comment: set `listed` to `true` only after OpenAI has approved the Plugin Directory listing and the maintainer chooses to switch; the browser suite follows it.
- `site/src/content/docs/connect.md` (deleted) — content moves to the Astro page below. Carry over every sentence that is not rewritten here, including the domain-move note, verbatim.
- `site/src/pages/docs/connect.astro` (new) — `Docs` layout, title "Get started", the same description as `connect.md`'s front matter. Body, in order:
  1. `<h1>Get started</h1>` and `<CopyBlock text={site.mcpURL} label="The playmat's address" wrap class="address" />` (what `site/src/pages/docs/[...slug].astro:17` rendered for `showAddress`).
  2. The two intro paragraphs from `connect.md`, then the domain-move `<aside class="note" data-moved-note>`, now with `id="moved"`. Keep its text unchanged.
  3. `<h2>claude.ai</h2>` — steps as today, with the address printed from `{site.mcpURL}`. Use `<Prompt text="What can the tcgreplay playmat do?" />` for the check prompt (`site/src/components/Prompt.astro`). Troubleshooting paragraph as today, plus: "If a tool seems missing after an update, <a href="#refresh">refresh the connection</a>."
  4. `<h2>ChatGPT</h2>` — first a short plans paragraph: "Adding a custom MCP server needs ChatGPT on the web, on a Business, Enterprise or Edu plan. On Business, a workspace admin or owner adds it for the workspace." (from OpenAI's help article, see research.md; the manual launch check confirms or widens it). Then `{chatgptDirectory.listed && (<>…directory block…</>)}` holding: a paragraph "Find TCG Replay in ChatGPT's Plugin Directory:" and an `<ol>`: open **Plugins** in ChatGPT; search for "{chatgptDirectory.name}"; open it and choose **Add** (or **Connect**); sign in with Google when asked and allow it to share your email address; in a new chat, type `@` and pick TCG Replay. Then a paragraph "Or add it yourself as a custom connection:" (shown only when listed). Then the custom steps `<ol>`: 1. In ChatGPT on the web, open **Plugins** (chatgpt.com/plugins), select **+**, then **Add custom MCP server**. 2. Name it `tcgreplay` and, under **Connection**, paste the playmat's address, `{site.mcpURL}`, as the **Server URL**. 3. Leave authentication on **OAuth**. 4. Read the warning, choose **I understand and want to continue**, then **Create as a plugin**. 5. Find tcgreplay in your plugins and **Install** it. The first time it is used, Google asks you to choose an account and share your email address, and you come back to ChatGPT. 6. In a new chat, type `@` and pick tcgreplay. Then the check prompt and "ChatGPT should list the playmat's coaching workflows." and troubleshooting with the same refresh link.
  5. `<h2>Claude Code</h2>` — as today; command `claude mcp add --transport http tcgreplay ${site.mcpURL}` in a `<pre><code>` (keep a code block the copy script turns into a copy block, `site/src/scripts/copy.ts`; no trailing newline). Troubleshooting with the refresh link.
  6. `<h2>The Claude desktop app</h2>` — as today.
  7. `<h2 id="refresh">Refresh after an update</h2>` — paragraph: "When tcgreplay adds or changes a tool, your assistant may keep the list it saw when you connected, so a new tool seems missing or an old one behaves oddly. <a href="/docs/whats-new">What's new</a> says when an update needs a refresh." Then `<h3>claude.ai</h3>` `<ol>`: open **Settings → Connectors**; remove tcgreplay; add it again with `{site.mcpURL}` and connect; start a new chat. `<h3>ChatGPT</h3>` `<ol>`: open **Plugins** and choose tcgreplay; choose **Refresh** on its page, which fetches the new tools; start a new chat. Plus `{chatgptDirectory.listed && <p>If you added TCG Replay from the Plugin Directory, it updates on its own; you don't need to refresh it.</p>}`. `<h3>Claude Code</h3>` `<ol>`: type `/mcp`; choose tcgreplay and **Reconnect** (authenticate again if asked); if the tool still doesn't appear, quit and restart Claude Code.
  8. `<h2>Next</h2>` — as today.
  The custom ChatGPT steps and the whole page must contain the string "Plugin Directory" only inside the `listed` branches.
- `site/src/lib/docs-pages.ts` (from the previous task) — add `{ href: '/docs/connect', title: 'Get started', description: <connect.md description>, order: 10 }` to the hand-written pages (the collection no longer has `connect`), and `{ href: '/docs/connect#refresh', title: 'Refreshing the connection', description: 'Make your assistant see new or changed tools after an update.', order: 15 }`. `Docs.astro`'s `aria-current` compares `href` to the path, so the anchor item is never marked current.
- `site/src/content.config.ts:21-22` — leave `showAddress` in the schema (no page uses it now); update its comment to say so. Do not remove `site/src/pages/docs/[...slug].astro`'s handling.
- `site/e2e/connect.spec.js:14-50` — the per-assistant test keeps working if the Astro page keeps `.prose` children flat (`h2`, `p`, `ol`, copy blocks as direct children of `<article class="prose">` — avoid wrapping sections in `div`s, except that the `listed` fragment renders flat too). `Refresh after an update` is a new `h2` key; the test only looks up the three names. Extend it: the ChatGPT section text matches `/Business, Enterprise or Edu/` (or whatever plan list the page holds; assert against a `chatgptPlans` constant exported from `listing.ts` if you prefer one source) and contains "Create as a plugin"; each of the three sections has an `a[href="#refresh"]`. Keep the domain-move assertions (`site.mcpURL`, exactly one `[data-moved-note]`).
- `site/e2e/connect.spec.js` (new tests) — (1) `#refresh` exists, is an `h2`, and is followed by `h3` headings `claude.ai`, `ChatGPT`, `Claude Code`, each followed by an `ol` with at least two items; (2) directory switch: import `chatgptDirectory` from `../src/lib/listing.ts`; if `!listed`, the ChatGPT section's first `ol` contains "Add custom MCP server" and the page text has no "Plugin Directory"; if `listed`, the first `ol` contains `chatgptDirectory.name` and "Plugin Directory", and a later `ol` contains "Add custom MCP server"; (3) the `Guide` navigation has a "Refreshing the connection" link with href `/docs/connect#refresh`.
- `site/e2e/site.spec.js` (the domain-move whole-site scan test) — add: when `chatgptDirectory.listed` is false, no built HTML file under `internal/site/dist/app` contains `Plugin Directory` (case-insensitive); failures name the file.

**Complexity**: Medium
**Token estimate**: ~45k tokens
**Agent strategy**: Single agent, sequential: page first, then `docs-pages.ts`, then tests; run `make site-test`. Diff the rendered `/docs/connect` text before and after (ignoring the rewritten ChatGPT section and the new refresh section) to confirm nothing else was lost in the move from Markdown.

### Task: What's new page and launch entry

All changes in repo `tcgreplay`. Requirements covered: "A 'What's new' page", the linking half of "Refresh steps for each assistant", and the What's new half of "Easy to find"; also supports the success metric "every release adds an entry".

**File changes**
- `site/src/data/whats-new.yaml` (new) — a header comment: one entry per release that changes what players see, newest first; `date` is the day it shipped (`YYYY-MM-DD`); `refresh: true` when the release adds, removes or changes a tool, a tool's description, a guide or the playmat's instructions to assistants; `changes` are short player-facing lines, Markdown links allowed (rendered as inline HTML links, see loader). First entry: `date:` the day this change merges (the maintainer corrects it to the launch day in the manual wording review if they differ), `title: TCG Replay is public`, `refresh: true`, `changes:` (a) "The playmat has its own address, https://tcgreplay.app. If your assistant was connected before, remove the connection and add it again at the new address: see [the note on connecting](/docs/connect#moved)." (b) "New ChatGPT connection steps for adding the playmat as a plugin: see [Get started](/docs/connect)." (c) "This What's new page, and [steps to refresh your assistant's connection](/docs/connect#refresh) after an update." Add one line each for the sibling launch changes that are merged on `main` when this task runs (privacy and terms pages, the fan-project notice on the board, clearer tool labels for assistants); check `git log` and the site pages; leave out any that have not landed. Never name the old host and never mention the Plugin Directory.
- `site/src/lib/whats-new.ts` (new) — follows `site/src/lib/facts.ts:1-60`: `export const whatsNewPath = join(process.cwd(), 'src', 'data', 'whats-new.yaml')` (`process.cwd()` is `site/` during build and the suite; never `import.meta.url`, knowledge `gotchas/astro-site-build.md`). `class WhatsNewError extends Error` with prefix `what's new: `. `export function loadWhatsNew(yamlText: string, source = 'site/src/data/whats-new.yaml'): WhatsNewEntry[]`: parse with `yaml`'s `parse`; require a non-empty array; for entry *i* (named in errors as `entry <i+1> ("<title or date>")`): `date` a string matching `^\d{4}-\d{2}-\d{2}$` that round-trips through `new Date(date + 'T00:00:00Z').toISOString().slice(0,10)` (note: YAML parses unquoted dates as strings with the `yaml` package's default core schema — confirm, and accept a `Date` by formatting it if not); `title` a non-empty string; `refresh` strictly boolean; `changes` a non-empty array of non-empty strings. Return entries sorted by `date` descending, stable. `export const whatsNew = loadWhatsNew(readFileSync(whatsNewPath, 'utf8'))`. Also `export function changeHTML(s: string): string` that HTML-escapes the text and turns `[text](href)` into `<a href="href">text</a>` for hrefs starting with `/` or `https://` only.
- `site/src/pages/docs/whats-new.astro` (new) — `Docs` layout, title "What's new", description "What changed in each tcgreplay update, and whether you need to refresh your assistant's connection.". `<h1>What's new</h1>`, a one-line intro linking `/docs/connect#refresh`, then `<ol class="whats-new" data-whats-new>` with one `<li data-entry data-date={date}>` per entry: `<h2><time datetime={date}>{human date, e.g. 20 November 2026, formatted with Intl.DateTimeFormat('en-GB', { timeZone: 'UTC', dateStyle: 'long' })}</time> — {title}</h2>`, then `<p data-refresh>` with either `<strong>Refresh needed.</strong> <a href="/docs/connect#refresh">Refresh your assistant's connection</a> to see the changes.` or `No refresh needed.`, then `<ul>` of `<li set:html={changeHTML(c)} />`.
- `site/src/lib/docs-pages.ts` — add `{ href: '/docs/whats-new', title: "What's new", description: <page description>, order: 85 }`.
- `site/e2e/whats-new.spec.js` (new) — Node tests, modelled on `site/e2e/facts.spec.js`: the real file loads, entries are newest first, the first entry's title mentions the launch (match `/public/i`) and has `refresh: true`; fixtures out of order come back sorted; fixtures missing `date`, with `date: 2026-02-30`, missing `title`, with `refresh: "yes"`, with `changes: []` each throw naming the entry and the field; `changeHTML` escapes `<` and links only `/` and `https://` hrefs. Browser tests: `/docs/whats-new` answers 200; the `time[datetime]` values are in descending order; the number of `[data-entry]` equals `whatsNew.length`; the first entry's heading text equals `whatsNew[0].title` suffix; every `[data-refresh]` reads "Refresh needed" or "No refresh needed", and every one reading "Refresh needed" contains `a[href="/docs/connect#refresh"]`; the first entry contains `a[href="/docs/connect#moved"]`; the `Guide` navigation and `[data-docs-index]` both link to `/docs/whats-new`.
- Knowledge entry (write through the `spek-knowledge` skill, never by hand) — category `conventions`, repo tier, store `tcgreplay`, e.g. `conventions/whats-new-entries.md`, tags such as `site`, `docs`, `release`, `conventions`: "Every change that players can see (website pages, the board, tools, guides, the playmat's instructions, limits) ships with an entry in `site/src/data/whats-new.yaml` in the same change. Mark `refresh: true` when it adds, removes or renames a tool, changes a tool's description or parameters, or changes a guide or the server instructions; otherwise `false`. Never name an old address outside the connecting page's move note, and never say TCG Replay is in the Plugin Directory while `site/src/lib/listing.ts` says it is not." Being always-applied, it is loaded by every future plan.

**Complexity**: Medium
**Token estimate**: ~40k tokens
**Agent strategy**: Single agent, sequential: loader and its Node tests first, then the data file, the page, the sidebar entry, the browser tests, and the knowledge entry last. Run `make site-test`.

### Task: Switch on the Plugin Directory route

Repo `tcgreplay`. Requirement covered: "Directory route once listed". Human task: it waits on OpenAI approving the listing submitted by the `chatgpt-submission-prep` spec, and on the maintainer choosing when to switch.

**What the person does**
1. After OpenAI's approval email, and once the plugin is published, open ChatGPT's **Plugins**, search for "TCG Replay" and connect it, noting the exact names of the search box, listing and button.
2. In `site/src/pages/docs/connect.astro`, adjust the directory steps inside `{chatgptDirectory.listed && …}` to match what ChatGPT showed (the custom steps and the rest stay).
3. In `site/src/lib/listing.ts`, set `listed: true`.
4. Add a What's new entry to `site/src/data/whats-new.yaml` dated the switch day: "TCG Replay is in ChatGPT's Plugin Directory: see [Get started](/docs/connect)." with `refresh: false`.
5. Run `make site-test` (the connecting-page tests switch to asserting the listed state), then merge so it deploys. An agent may make steps 2-5 on request once the person gives it the live wording.

**How the result is checked**
- The browser suite passes in the listed state: the ChatGPT section's first steps name "TCG Replay" and the Plugin Directory, the custom steps follow, and the refresh section has the directory note.
- The manual check "Directory steps after approval" in the implementation test plan: following the live page's directory steps in ChatGPT connects TCG Replay.

## Testing Strategy

- **Docs index and shared docs page list.** New `site/e2e/docs-index.spec.js`: `/docs/` and `/docs` answer 200 with the index (not `404.html`); the index's links equal the sidebar's pages in order; the sidebar's first link is "All docs" to `/docs`. Existing `site/e2e/limits.spec.js:48-53,64-66` sidebar checks and the `site/e2e/site.spec.js` crawl keep passing.
- **Connecting page with current ChatGPT steps and refresh steps.** `site/e2e/connect.spec.js`: the per-assistant test still passes on the Astro page; the ChatGPT section names its plans and has "Create as a plugin"; each assistant section links `#refresh`; the refresh `h2` has `h3`s for claude.ai, ChatGPT and Claude Code, each with an `ol`; the directory test follows `chatgptDirectory.listed`; the sidebar links "Refreshing the connection". `site/e2e/site.spec.js` whole-site scan: no built page contains "Plugin Directory" while not listed.
- **What's new page and launch entry.** New `site/e2e/whats-new.spec.js`: Node tests of `loadWhatsNew` (real file, sorting, each malformed field named) and `changeHTML`; browser tests of the page (dates descending, entry count, refresh lines and links, launch entry links the move note) and its sidebar and index links.
- **Switch on the Plugin Directory route.** The same connecting-page tests assert the listed state once the flag is `true`.
- **Manual (implementation test plan):** custom ChatGPT steps work on a named plan at launch; refresh steps work in all three assistants after a tool change; directory steps work after approval; no GitHub issue in the first month asks a question the docs answer; every player-visible release since launch has an entry; docs wording review.

## Project References

- Spec: `20261112091756-c5b6d1fd-launch-docs` (epic `20261111093348-e1b5c2b4-public-launch`).
- Upstream plan: `20261111093353-8ff3589f-domain-move` (tasks `336cf3bc` website reads the public address, `92eaa296` site pages and README name the new address) — must be merged first.
- Sibling specs: `20261112090820-07d70cfb-chatgpt-submission-prep` (listing and approval), `20261112083238-29247b36-privacy-terms-disclaimer` (shared layout links).
- Knowledge: `learnings/publishing-to-chatgpt.md` (add flow, Refresh, plans, daily rescans), `learnings/publishing-to-claude.md` (claude.ai and Claude Code refresh), `gotchas/astro-site-build.md` (`process.cwd()` paths, build before serving), `gotchas/chatgpt-tool-discovery.md` (ChatGPT keeps its fetched tool list).
- Design documents: none.
- Repo root: `tcgreplay` at `/home/nicj/code/github.com/nicholasjackson/tcgreplay`.

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

All three agent tasks touch the same few site files and tests, so each runs as a single sequential agent, in dependency order (docs index, then connecting page, then What's new).

## Migration Notes

- `site/src/content/docs/connect.md` is deleted and replaced by `site/src/pages/docs/connect.astro` at the same URL; nothing links to the Markdown file by path. Any open change that edits `connect.md` (for example a late domain-move fix) must be re-applied to the Astro page.
- No data migration. The What's new list starts with the launch entry.

## Performance Considerations

None. All pages are static HTML built once; the What's new list is read once per build.
