---
created_date: "2026-11-12"
document_status: final
closed_date: "2026-11-12"
---

# Research: 20261112091756-c5b6d1fd-launch-docs

## Alternatives considered and rejected

**Switching on the ChatGPT Plugin Directory route**
- *Keep `connect.md` as Markdown and paste drafted directory steps in later from a maintainer note.* Rejected: Markdown cannot branch (`site/src/content.config.ts:15-24`, `site/src/pages/docs/[...slug].astro:12-18` render the file verbatim), so the drafted steps would sit outside the site, untested, and drift; the switch would be a hand edit rather than the "small follow-up change" the spec's Technical Approach asks for.
- *A remark plugin in `site/astro.config.mjs` that strips a comment-marked block from `connect.md` unless a flag is set.* Rejected: new custom build machinery for one block; the site has no remark plugins today (`site/astro.config.mjs:6-13`).
- *Render the directory steps hidden (`hidden` attribute or CSS) until approval.* Rejected: the text would still be in the built HTML, breaking the acceptance criterion "before approval, no page says TCG Replay is in the directory".

**Where "What's new" entries live**
- *One Markdown file per entry in an Astro content collection.* Rejected: the spec's Technical Approach prefers one dated list; per-file entries spread a release's note across a new file each time.
- *Astro's `file()` content loader over a YAML file.* Viable, but rejected in favour of the existing facts pattern (`site/src/lib/facts.ts:43-90`): a plain TS loader that throws a named error is directly unit-testable from the Playwright suite in Node (`site/e2e/facts.spec.js:7`), which a content-collection loader is not.
- *A hand-written Markdown "What's new" page.* Rejected: nothing would check that each entry has a date and a refresh flag, or that entries needing a refresh link to the steps.
- *`CHANGELOG.md` or GitHub releases.* Rejected: not on the website, and the project has no tags or release step; every push to `main` deploys (`.github/workflows/deploy.yml:1-20`).

**Refresh steps**
- *A separate "Refreshing the connection" docs page.* Rejected: the spec's Technical Approach prefers one refresh section in the connection docs that entries and troubleshooting text link to.

**Docs index at `/docs/`**
- *Redirect `/docs/` to `/docs/connect`.* Rejected: the acceptance criterion asks for a page listing the docs.

**Enforcing "every player-visible release adds an entry"**
- *An automated guard (e.g. a Go golden file of tool names and descriptions that fails until a new entry is added).* Rejected: crosses Go and the site build, and most player-visible changes are not tool changes, so it would both miss changes and nag on internal ones. A written convention plus the manual success-metric check is proportionate.

## Chosen approach — evidence

- `site/src/content/docs/connect.md:1-70` — the one connecting page, sections `## claude.ai`, `## ChatGPT`, `## Claude Code`; current ChatGPT steps already use "Plugins → Add → Add custom MCP server" but miss the risk warning, "Create as a plugin", install and the plan names.
- `site/src/pages/docs/[...slug].astro:17` — `showAddress` is used only by `connect.md`; an Astro page can render the same `CopyBlock` itself.
- `site/src/layouts/Docs.astro:12-20` — the docs sidebar: Markdown docs by `order` plus an `extra` list of Astro pages; new pages join `extra`. The list is computed inline, so a shared helper is needed to reuse it for a `/docs/` index.
- `site/src/layouts/Base.astro:21-28` — header nav; left unchanged (the privacy-terms spec edits the shared layout's links; the spec asks only for docs navigation).
- `internal/site/site.go` `resolve` — `/docs/` cleans to `docs`, then tries `docs/index.html`; so adding `site/src/pages/docs/index.astro` (built as `docs/index.html` under `build.format: 'directory'`, `site/astro.config.mjs:9-10`) makes `/docs/` and `/docs` answer 200 with no Go change. Today it falls to `404.html`.
- `cmd/tcgreplay/hosted.go:208` — `/` (so `/docs/…`) goes to `site.Handler()`; no other route claims `/docs`.
- `site/src/lib/facts.ts:43-90` and `site/e2e/facts.spec.js` — the pattern for a build-time data loader with a named error, tested from Playwright in Node.
- `site/e2e/site.spec.js:14-80` — crawl from `/` checks every linked page exists and passes layout checks; new pages are covered once linked.
- `site/e2e/connect.spec.js:14-45` — groups `.prose` children by `h2`; requires h2 sections named exactly `claude.ai`, `ChatGPT`, `Claude Code`, each with an `ol`, a "Check it works" block, a prompt and "should list". A refresh `h2` with `h3` per assistant does not disturb it.
- `site/src/scripts/copy.ts:11-29` and `site/src/components/Prompt.astro` — prompts render the same in Markdown (```prompt fence) and Astro (`<Prompt>`), and the e2e accepts either.
- Knowledge `learnings/publishing-to-chatgpt.md` §2 — official add steps (chatgpt.com/plugins → + → Add custom MCP server → name → Server URL → OAuth → accept risk warning → Create as a plugin → Install → `@` in a new chat); **Refresh** on the plugin's page pulls new tools, descriptions and instructions; published plugins are rescanned daily, so directory users need not refresh. §11: plans conflict (Business/Enterprise/Edu official; Pro read/fetch only; Plus unclear). Workspace permission "Create plugins with MCPs" is off by default.
- Knowledge `learnings/publishing-to-claude.md` — Claude Code handles `tools/list_changed` and `/mcp` reconnects; claude.ai refresh is undocumented, reported fix is removing and re-adding the connector.
- Domain-move plan (`20261111093353-8ff3589f-domain-move`) — `site/src/lib/facts.ts` will read `TCGREPLAY_PUBLIC_URL`; `connect.md` gains one `data-moved-note` element; a whole-site scan fails on `demo.gs` outside that element or any `www.tcgreplay.app`.

## Files examined

- `tcgreplay:site/src/content/docs/connect.md:1-70` — connecting page; ChatGPT section at "## ChatGPT".
- `tcgreplay:site/src/content.config.ts:15-24` — docs collection schema (`title`, `description`, `order`, `showAddress`).
- `tcgreplay:site/src/layouts/Docs.astro:12-20` — sidebar items (`extra` Astro pages + collection, sorted by order).
- `tcgreplay:site/src/layouts/Base.astro:21-28` — header nav list.
- `tcgreplay:site/src/pages/docs/[...slug].astro:1-19` — Markdown docs renderer; injects the address CopyBlock.
- `tcgreplay:site/src/pages/docs/limits.astro` — model of a hand-written Astro docs page using `Docs` layout and facts.
- `tcgreplay:site/src/pages/docs/workflows/index.astro` — model of a docs listing page.
- `tcgreplay:site/src/pages/404.astro` — the page `/docs/` serves today.
- `tcgreplay:site/src/lib/facts.ts:1-90` — facts loader pattern, `FactError`, `process.cwd()` paths.
- `tcgreplay:site/astro.config.mjs:6-13` — `trailingSlash: 'never'`, `build.format: 'directory'`, no remark plugins.
- `tcgreplay:site/playwright.config.js` — suite runs against the built site via `go run ./internal/site/sitetest/cmd/serve`.
- `tcgreplay:site/e2e/connect.spec.js:14-45` — connecting-page structure assertions.
- `tcgreplay:site/e2e/site.spec.js:14-80` — crawl, nav link list, every-linked-page-exists.
- `tcgreplay:site/e2e/facts.spec.js:1-60` — Node-side loader tests in Playwright.
- `tcgreplay:site/src/scripts/copy.ts:11-29` — prompt fence handling.
- `tcgreplay:internal/site/site.go` (`Handler`, `resolve`, `notFound`) — `/docs/` resolution and 404 behaviour.
- `tcgreplay:cmd/tcgreplay/hosted.go:191-208` — route table; `/` to the site handler.
- `tcgreplay:.github/workflows/deploy.yml:1-40` — every push to main deploys; no release tags.
- `tcgreplay:Makefile:34-53` — `make site`, `make site-test`, `make test`.

## External references

- OpenAI, *Add custom MCP server* (https://developers.openai.com/api/docs/guides/custom-mcp-server.md) — current ChatGPT add steps ("Add custom MCP server", "I understand and want to continue", "Create as a plugin", install) and the app details page's Refresh that pulls new tools, descriptions and server instructions. Names no plans.
- OpenAI Help Center, *Developer mode and full MCP apps in ChatGPT* (https://help.openai.com/en/articles/12584461) — full MCP support (write actions) for Business, Enterprise and Edu on web; on Business only admins/owners create custom MCP plugins; developer mode need not be turned on first. Plus/Pro availability not stated.

## Prior plans / specs consulted

- `20261111093353-8ff3589f-domain-move` plan and context — builds the address facts and the `data-moved-note` + whole-site scan this plan must respect; edits `connect.md` in its task "Site pages and README name the new address" (92eaa296), which this plan's conversion of the connecting page must carry over.
- `20261112083238-29247b36-privacy-terms-disclaimer` spec — adds privacy/terms/help links to every page (shared layout); this plan stays out of `Base.astro`.
- `20261112090820-07d70cfb-chatgpt-submission-prep` spec — owns the plugin package, listing name "TCG Replay" and submission; its Non-Goals hand the Plugin Directory connection docs to this spec.

## Open assumptions

- ChatGPT's plan eligibility for custom MCP servers is drafted from OpenAI's help article (Business, Enterprise, Edu; on Business an admin or owner adds it) and must be confirmed against the live help page and a real account at the launch release; if Plus or Pro can connect a write-capable custom server, they are added.
- claude.ai's refresh is drafted as removing and re-adding the connector (undocumented by Anthropic); confirmed in the real assistant at launch.
- Claude Code's refresh is drafted as `/mcp` → tcgreplay → Reconnect (or restarting Claude Code).
- The directory route's exact wording (search name "TCG Replay", the button to add it) cannot be confirmed until the listing is approved; it is checked when the switch is turned on.
- Directory-installed plugins are rescanned daily by OpenAI, so the refresh section tells directory users they need not refresh (only shown once the directory route is on).
- The domain-move plan lands first, so `site.mcpURL` is `https://tcgreplay.app/mcp` and the connecting page carries its `data-moved-note`.

## Drafting assumptions

### Chosen direction: Astro connecting page with a listing flag, YAML What's new list, shared docs list (architecture)
- **Decision**: Replace `connect.md` with `site/src/pages/docs/connect.astro` gated by `site/src/lib/listing.ts` (`chatgptDirectory.listed = false`); "What's new" as `site/src/data/whats-new.yaml` read by a facts-style loader `site/src/lib/whats-new.ts` and rendered at `/docs/whats-new`; one `#refresh` section on the connecting page; sidebar list moved to `site/src/lib/docs-pages.ts` and reused by a new `/docs/` index. Key trade-offs: the connecting page leaves the Markdown collection (more markup to maintain) so the directory route can be drafted, tested and switched on by one line; entries are build-validated data rather than prose; the header nav is untouched to avoid colliding with privacy-terms.
- **Rationale**: Meets every requirement with no Go change; follows the existing facts pattern and Astro page patterns (`limits.astro`, `workflows/index.astro`); keeps "before approval, no page says it is listed" checkable.
- **Rejected**: Markdown + pasted-in-later directory steps; a remark plugin; hidden HTML; per-entry Markdown collection; Astro `file()` loader; CHANGELOG/GitHub releases; a redirect for `/docs/` (see research.md alternatives).

### ChatGPT plans named in the docs (discovery)
- **Decision**: Draft the plans from OpenAI's help article (Business, Enterprise and Edu on the web; on Business, a workspace admin or owner creates the plugin), and confirm or widen the list against the live help page and a real account at the launch release.
- **Rationale**: Official sources conflict on Plus and Pro (knowledge `learnings/publishing-to-chatgpt.md` §11); the spec needs plans named and the acceptance check is live.
- **Rejected**: Naming Plus and Pro from third-party guides (unverified; Pro reported read/fetch only, and the playmat has write tools).

### Refresh steps per assistant (discovery)
- **Decision**: claude.ai: remove and re-add the connector, then a new chat. ChatGPT: Plugins → TCG Replay → Refresh, then a new chat (custom connection); directory-installed plugins update on their own. Claude Code: `/mcp` → tcgreplay → Reconnect, or restart.
- **Rationale**: From knowledge entries and OpenAI's docs; claude.ai's is undocumented, so a person confirms all three at launch.
- **Rejected**: Waiting for official claude.ai documentation (none exists).

### Conventions selected (architecture)
- **Decision**: No conventions apply; the guide-text convention covers `internal/guides`, which this plan does not touch.
- **Rationale**: All work is under `site/`.
- **Rejected**: Listing the guide convention as applicable (it is not).

### Header navigation left unchanged (architecture)
- **Decision**: "What's new", the refresh steps and the docs index are reached from the docs sidebar and the index; `Base.astro`'s header nav is not edited.
- **Rationale**: The spec asks for docs navigation; the privacy-terms spec edits the shared layout, so staying out of it avoids merge conflicts.
- **Rejected**: Adding "What's new" to the header nav.

### The launch entry does not name the old address (architecture)
- **Decision**: The "What's new" launch entry says the playmat has its own address and links to the connecting page's move note, without naming the old host.
- **Rationale**: The domain-move whole-site scan allows the old address only inside its one move note.
- **Rejected**: Naming the old host in the entry inside another `data-moved-note` element (would add a second note, against the domain-move plan's "exactly one note").

### Live assistant checks are manual checks, not tasks (testing_approach)
- **Decision**: Checking the ChatGPT custom steps, plans and the three refresh flows in the real assistants is a manual check in the test plan; the only task for a person is switching on the directory route after approval.
- **Rationale**: The workflow sends reviews and by-hand checks to the test plan; switching the flag is an action with a timing only the maintainer can choose.
- **Rejected**: A human "verify the steps" task (never ticked).

### Directory switch is a human task (tasks)
- **Decision**: Switching on the Plugin Directory route is one `human` task depending on the connecting-page and What's new tasks, waiting on OpenAI's approval; an agent may make the edit once the person supplies the live wording.
- **Rationale**: Its timing depends on an approval outside the repo and the maintainer's choice (spec Constraints), and the directory wording can only be checked in a live ChatGPT account.
- **Rejected**: Leaving it out of the plan (the requirement would have no task); an agent task (an agent cannot know approval has happened).

### Launch entry content and date (tasks)
- **Decision**: The launch entry is dated the day the change merges and lists the new address (linking the move note), the new ChatGPT steps, the refresh steps and What's new, plus any sibling launch changes already on `main`; the maintainer corrects the date in the wording review if launch day differs.
- **Rationale**: The launch date and which sibling specs land first are unknown at planning time.
- **Rejected**: A placeholder date (would ship wrong); waiting for every sibling spec (blocks this plan on others).

### Custom connection name stays `tcgreplay` (tasks)
- **Decision**: The custom connection steps keep naming the connection `tcgreplay`; the directory steps use the listing name "TCG Replay".
- **Rationale**: Matches the current docs and the claude.ai/Claude Code steps; renaming the brand in the docs is not in this spec.
- **Rejected**: Renaming every mention to "TCG Replay".

### What's new sits at sidebar order 85, refresh link at 15 (tasks)
- **Decision**: Sidebar order: Get started 10, Refreshing the connection 15, ..., Limits 80, What's new 85, For developers 90; "All docs" link first.
- **Rationale**: Refresh belongs beside connecting; What's new near the reference pages.
- **Rejected**: Putting What's new first (pushes the getting-started path down).

## Rehydration cues

- `spektacular spec file read 20261112091756-c5b6d1fd-launch-docs`
- `spektacular plan file read 20261111093353-8ff3589f-domain-move plan` (and `context`)
- `spektacular knowledge read --data '{"tier":"repo","name":"tcgreplay","path":"learnings/publishing-to-chatgpt.md"}'` and `learnings/publishing-to-claude.md`, `gotchas/astro-site-build.md`
- Re-read `site/src/content/docs/connect.md`, `site/src/layouts/Docs.astro`, `site/src/lib/facts.ts`, `site/e2e/connect.spec.js`, `site/e2e/site.spec.js`, `internal/site/site.go`.
