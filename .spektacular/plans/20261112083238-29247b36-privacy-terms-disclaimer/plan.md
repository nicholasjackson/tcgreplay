---
created_date: "2026-11-12"
document_status: final
closed_date: "2026-11-12"
---

# Plan: 20261112083238-29247b36-privacy-terms-disclaimer

<!-- Metadata -->
<!-- Created: 2026-11-12T09:56:13Z -->
<!-- Commit: 6ecc1aa -->
<!-- Branch: main -->
<!-- Repository: tcgreplay (/home/nicj/code/github.com/nicholasjackson/tcgreplay) -->

## Overview

Before TCG Replay goes public and is submitted to ChatGPT, it needs the pages a public service and the ChatGPT review expect. This plan publishes a terms of service page, brings the privacy page up to date for the coming visitor counting and diagnostics (and corrects what it says today that the playmat no longer does), and links both pages and the project's GitHub issues from every website page. It writes the unofficial fan-project notice and the card-data credit once, shared by the website footer, the terms, the board's new About panel and the ChatGPT listing, so players everywhere see that tcgreplay has no connection to Nintendo or The Pokémon Company and where its card data comes from.

## Conventions

- **Never write "Claude" or "Nic " in an `internal/guides` guide; never exclude `internal/guides` from a build context** — this work edits the Dockerfile's `web` and `site` stages (each gains a `COPY` of `internal/notice/notice.json`) and must leave the `site` stage's `COPY internal/guides` and `.dockerignore` untouched. No guide text changes.

## Architecture & Design Decisions

All of the work lands in the one registered repo, `tcgreplay` (root `/home/nicj/code/github.com/nicholasjackson/tcgreplay`), in three of its parts: the public website (`site/`, Astro), the board (`web/`, Svelte), and the container build (`Dockerfile`). No Go behaviour changes. The hosted server already serves every built website page at its own path, with no sign-in and no cookie (`internal/site/site.go`), so a new `site/src/pages/terms.astro` is live at `https://tcgreplay.app/terms` as soon as it is built. The only Go change is a test: the hosted route test gains `/terms` among the public pages it fetches signed out.

**One wording, in one file.** The fan-project notice, the card-data credit and the help address are written once, in a new data file, `internal/notice/notice.json`:
- `notice`: the footer's existing two sentences, unchanged in substance ("tcgreplay is an unofficial fan project. It is not affiliated with, endorsed, sponsored or approved by Nintendo, The Pokémon Company, Creatures or Game Freak. Pokémon and its trademarks are theirs.").
- `credit`: the sources of card data and images, TCGdex (`https://tcgdex.dev`) and malie.io (`https://malie.io`).
- `help`: the project's GitHub issues page, `https://github.com/nicholasjackson/tcgreplay/issues`, the only help channel (spec constraint).

The website reads it at build time through a small loader beside the existing facts (`site/src/lib/notice.ts`), with the same fail-the-build checks and the same `process.cwd()`-relative path rule the facts use (`gotchas/astro-site-build.md`). The board imports the same file through Vite. The Dockerfile's `web` and `site` stages each copy it in, as the `site` stage already copies `.do/app.yaml` and `internal/guides`. The ChatGPT listing work can read the same file, so the footer, the terms, the board and the listing cannot drift apart. This is the spec's Technical Approach (reuse the footer wording everywhere). It is chosen over keeping hand-made copies pinned by tests, and over serving the text from the Go server. The evidence is in `research.md#alternatives-considered-and-rejected`.

**The website.** Every page uses the shared layout, `Base.astro`, so the footer changes once for every page. It renders the notice and credit from the shared file, and gains a row of links: Privacy, Terms, Get help (the GitHub issues page) and Source on GitHub. The header navigation stays as it is.

The new terms page is modelled on a community tool's terms (Limitless), in plain words, with no governing-law clause. It has one section per item the spec lists:
- who runs the playmat ("the tcgreplay project");
- free and as is, with no guarantee it stays available;
- games can be lost at any time;
- the hosted limits;
- acceptable use;
- minimum age 13;
- changes to the terms and the service;
- trademarks and card images (the shared notice and credit);
- privacy (a link to the privacy page);
- help (GitHub issues);
- a last-updated date.

The figures it states (requests a minute, players served at once, idle time) come from the existing facts loader, which reads them from `.do/app.yaml`; the address comes from `site.publicURL`, which the domain move makes `https://tcgreplay.app`. Neither is typed in, and so the terms never name `demo.gs` or a `www` address, keeping the domain move's whole-site check green.

**The privacy page** keeps its structure and address (`/privacy`, which the Google consent screen and the ChatGPT submission point at). Three kinds of change:
- **Two new sections.**
  - *Visitor counting*: Cloudflare Web Analytics on the website and the board. It records the page, the referrer, the country and the device. It sets no cookies, stores nothing in the browser and is never linked to an account. It is kept for the period Cloudflare documents.
  - *Diagnostics*: DigitalOcean's request and performance metrics, and the playmat's own usage and error records. The records are keyed by a pseudonymous player ID, never an email. The section lists what they contain and what they never contain, and how long each is kept.
  - Both are drawn from `docs/observability.md`, the runbook the observability work writes, and from Cloudflare's documentation. The retention figures are typed in, inside elements marked `data-retention`. A browser test checks that the `logs` and `metrics` figures appear in the runbook's Retention section, so the two cannot quietly disagree. The `visits` figure is Cloudflare's, which the runbook does not record, so it is checked against Cloudflare's documentation in the manual privacy review (decided at epic planning).
- **Corrections from the re-check of every existing statement against the code** (`research.md`, audit):
  - Sign-ins are held in memory, and the copies given to the browser and the assistant are encrypted, so a sign-in survives an update while a game does not.
  - Disconnecting the assistant does not reach the playmat; its record runs out within 30 days of last use (`gotchas/disconnect-does-not-reach-playmat.md`).
  - The playmat fetches card images from TCGdex and malie.io itself and keeps copies in DigitalOcean Spaces; no player data goes with them.
  - The website now loads one thing from another site, Cloudflare's counting script.
  - Logs no longer carry email addresses.
  - "Analytics of any kind" leaves the never-collected list, replaced by "advertising or tracking data" and "anything that identifies you in visitor counts".
- **A new last-updated date**, set to the day the updated page goes live.

The page describes visitor counting before it is switched on, because the visitor-analytics spec requires the privacy page first.

**The board** gains an "About tcgreplay" item in its ☰ menu. The menu is present at every layout, in hosted and desktop modes. The item opens a panel in the board's existing modal system with the notice, the credit and a "Get help" link, all from the shared file. Nothing shows until the player asks, so no card or control is ever covered and the phone board's no-scroll fit is untouched. The panel closes like every other view: ✕, Escape or the backdrop. This follows the spec's preference for an on-demand menu or about panel over a permanent strip. The panel adds no browser storage, so the privacy page's "four display settings" stays true. It carries no privacy or terms links, because the desktop board serves no website for them to open.

## Component Breakdown

- **Shared notice (new).**
  - Owns the one wording of the fan-project notice, the card-data credit (TCGdex and malie.io, each with its address) and the help address (the project's GitHub issues page).
  - It is a plain data file with no code. The website build and the board build read it, and the ChatGPT listing work can read it too.
  - It is new because no existing place is reachable from both builds. The website's layout is an Astro component, which the board cannot import.
- **Website notice loader (new, beside the existing site facts).**
  - Reads the shared notice while the website builds and hands it to pages.
  - Stops the build with a message naming the problem if the file is missing, a field is empty, the notice does not name all four companies, or an address is not https.
  - It follows the facts loader's pattern, and like it, it is imported by the shared layout so every page's build checks it.
- **Site facts (unchanged).** Still the only source of the hosted limits (requests a minute, players at once, idle time) and of the public address, all read from the App Platform definition. The terms page and the privacy page read their figures from it. The domain move changes how it reads the address; this work only uses it.
- **Shared layout and footer (changed).**
  - The footer renders the notice and credit from the notice loader instead of its own text.
  - It gains links to Privacy, Terms, Get help and Source on GitHub, so every website page links to both policy pages and to help.
  - The header navigation is unchanged.
- **Terms page (new).**
  - A public page at `/terms`, served by the existing website handler with no sign-in.
  - States who runs the playmat, the service terms and the hosted limits (from site facts), the acceptable-use rules, the minimum age, and that the terms and service can change.
  - Has a trademarks and card images section (from the notice loader), links to the privacy page and to help, and shows a last-updated date.
- **Privacy page (changed).**
  - Gains visitor-counting and diagnostics sections. Their retention figures are marked so the browser suite can compare them with the observability runbook.
  - Every existing statement is corrected where the code differs.
  - The last-updated date moves to the day the updated page goes live.
  - It keeps its address and its idle time from site facts.
- **Observability runbook (read only).** Written by the observability work. It is the source of what diagnostics record, what they never record, and how long each is kept. This work never edits it. The browser suite reads its Retention section to check the privacy page.
- **Board menu and About panel (changed, in the board app).**
  - The ☰ menu gains an "About tcgreplay" item at every layout. It opens a panel in the board's existing modal system showing the shared notice, the credit links and a Get help link.
  - The panel closes like the other views, keeps no browser storage, and is the same in hosted and desktop modes.
- **Container build (changed).** The board and website build stages each copy the shared notice file in beside their sources, as the website stage already does for the App Platform definition and the guides.
- **Website browser suite, board browser suite and hosted route test (changed).**
  - The website suite checks the terms content, the new privacy sections and corrections, the footer links and shared wording on every page, and the retention figures against the runbook.
  - The board suite checks the About panel at phone, tablet and desktop widths.
  - The hosted route test fetches `/terms` signed out, with no redirect and no cookie.

## Data Structures & Interfaces

No persisted data, wire format or Go type changes. The plan adds one data file and two small build-time contracts.

**Shared notice file (`notice.json`)**: the one source of the notice, the credit and the help address. Both the website build and the board build read it, and the ChatGPT listing work can read it later.

```json
{
  "notice": "tcgreplay is an unofficial fan project. It is not affiliated with, endorsed, sponsored or approved by Nintendo, The Pokémon Company, Creatures or Game Freak. Pokémon and its trademarks are theirs.",
  "credit": {
    "lead": "Card data and images from",
    "sources": [
      { "name": "TCGdex", "url": "https://tcgdex.dev" },
      { "name": "malie.io", "url": "https://malie.io" }
    ]
  },
  "help": { "label": "GitHub issues", "url": "https://github.com/nicholasjackson/tcgreplay/issues" }
}
```

**Website notice loader**: a pure parse function plus the value it yields, alongside the site facts:

```ts
type Notice = {
  notice: string;                                   // names all four companies
  credit: { lead: string; sources: { name: string; url: string }[] };
  help: { label: string; url: string };             // https
};
function loadNotice(jsonText: string, source?: string): Notice; // throws a build-stopping error naming the problem
export const notice: Notice;                        // read once at build
```

**Marked values on the policy pages**: the page contract the browser suites read. They follow the existing `data-idle` and `data-limit` pattern.
- Privacy page:
  - `data-retention="logs"` holds the period usage and error records are kept;
  - `data-retention="metrics"` holds the period the hosting platform's metrics are kept;
  - `data-retention="visits"` holds Cloudflare's period for visitor counts.
- Terms page: `data-limit="rate"`, `data-limit="users"` and `data-limit="idle"`, as on the limits page.
- Both pages: a last-updated line marked `data-updated`.
- Each section is a `section` labelled by its heading, so tests find it as a named region, as the privacy suite does today.

**Board About panel**: one new modal kind in the board's existing modal state (`app.modal = { kind: 'about' }`). It needs no new store field and no browser storage.

## Implementation Detail

**A new cross-build source of truth.** Until now the website and the board have shared nothing at build time: the website reads the App Platform definition and the guides, and the board reads only its own sources. The shared notice file is the first data both builds read. It sits outside both build folders, and each build stage of the container copies it in. A developer changing the notice edits one file, and every surface that shows it changes on the next build.

The board side needs one small allowance. Vite's dev server refuses files outside the board's folder by default, so the board's Vite config allows that one file's folder. The production build reads it either way.

**Following the facts pattern for build-time data.** The website's notice loader copies the existing site facts loader:
- a pure parse function, testable on its own;
- a value computed once at import;
- paths taken from the working directory;
- an error that names the file and the problem, and stops the build.

Because the shared layout imports both, every page's build checks both. Browser tests keep their own independent readers of the source files, as the limits and privacy tests already do with the App Platform definition. A broken loader therefore shows up as a mismatch rather than agreeing with itself.

**Policy pages as code-backed documents.** The terms page follows the privacy page's existing pattern:
- figures come from facts;
- each section is a heading-labelled region;
- values a test pins carry a `data-*` marker;
- a comment at the top says which source each statement follows.

The privacy page's comment is extended to name the observability runbook and Cloudflare's documentation as sources, alongside the code. A developer who changes what the playmat collects will find the comment on the page, and a failing marked-value test, pointing them at the runbook.

**Retention: typed in, then cross-checked.** Retention is the one figure that cannot come from configuration, because it is DigitalOcean's and Cloudflare's. It is written on the privacy page and checked by a browser test against the runbook's Retention section. The test reads the runbook as text and looks for each marked figure inside the Retention section. It does not parse the runbook's structure, so the observability work can reword its prose freely as long as the figure stays.

**The board panel reuses the modal system.** The About panel is one more modal kind beside the pile, search, deck-list and prize views. It is opened from the ☰ menu like Deck list, closes the menu as it opens, and closes itself like the other views (✕, Escape, backdrop). On a phone the modal system already fills the screen, so the panel needs no layout of its own and cannot cover a card or control while closed. The menu item sits in the part of the menu present at every layout and in desktop mode, outside the hosted-only account block.

**Nothing new in Go.** The server needs no route, handler or setting: the website handler serves any built page publicly. The hosted route test only adds `/terms` to the public pages it checks, keeping the "stable public address without signing in" guarantee pinned in Go as well as in the browser suite.

## Dependencies

- **Design documents this plan was built on: none.** The spec carries no design references.
- **Observability plan (`20261112090330-4ac2ea19-observability`), must land first.** Its runbook, `docs/observability.md`, is the source of the privacy page's diagnostics section: what is recorded, what is never recorded, the pseudonymous player IDs, and the retention of logs and metrics. Its log lines must already carry no email address, or the page's new statement would be false.
  - The runbook's Retention figures come from a person's task in that plan. If the runbook has no figures when this work starts, the implementer STOPs and asks.
  - Nothing in that work changes here.
- **Domain move plan (`20261111093353-8ff3589f-domain-move`), should land first.** It makes `site.publicURL` read `https://tcgreplay.app` from the App Platform definition. It also adds the whole-site check that allows `demo.gs` only inside a `data-moved-note` element and never allows `www.tcgreplay.app`.
  - The new terms page and the edited footer name the address only through `site.publicURL`, so they pass that check in either order.
  - If this lands first, the pages show whatever address the facts give at the time.
- **Visitor-analytics spec (`20261111132045-1d1d32f6-visitor-analytics`), downstream.** It depends on this work: counting must not start before the privacy page describes it.
  - This plan describes Cloudflare Web Analytics as that spec sets it out: website and board, cookieless, page, referrer, country and device, never linked to an account.
  - When that work adds Cloudflare's script, it updates the website suite's "nothing fetched from other hosts" check, not this plan.
- **ChatGPT submission prep spec (`20261112090820-07d70cfb-chatgpt-submission-prep`), downstream.** It submits the `/privacy` and `/terms` addresses and the GitHub issues page as the support link. Its listing description can read the notice from the shared notice file.
- **Site facts loader (existing).** Provides the hosted limits and the public address from the App Platform definition. Used unchanged.
- **Board modal system and ☰ menu (existing, in the board app).** Gain one modal kind and one menu item.
- **Website handler (existing Go package).** Serves the new page publicly. Unchanged.
- **Astro 7, Vite, Svelte 5, Playwright and `yaml` (existing toolchain).** No new packages.
- **Cloudflare Web Analytics documentation (external).** The source for what the counting records and how long Cloudflare keeps it. The implementer confirms both there and cites them in the page's source comment.
- **Knowledge entries relied on:**
  - `gotchas/astro-site-build.md`: paths come from the working directory, and the site must be built before the Go handler can serve it.
  - `gotchas/disconnect-does-not-reach-playmat.md`, written during this plan: what disconnecting and signing out really end.
  - `gotchas/playwright-touch-tests.md`: the board suite's touch profiles run in Chromium.
  - `learnings/publishing-to-chatgpt.md`: what the review expects of the privacy policy and the support contact.
  - `conventions/skills-text-and-packaging.md`: guides packaging in the container build.

## Testing Approach

The tests extend the project's three existing suites in their own style. No new framework is added.
- **Website browser suite** (Playwright, against the built site served by the Go site handler, laptop and phone profiles). Most of the coverage goes here, because nearly every requirement is a statement on a page.
- **Board browser suite** (Playwright, against the board test server, phone, tablet and desktop profiles).
- **Go hosted route test.**

**Build-time loader tests (website suite, no browser page).** They follow the existing site-facts tests:
- The notice loader, given the real shared file, returns the notice naming Nintendo, The Pokémon Company, Creatures and Game Freak, the TCGdex and malie.io credit, and the GitHub issues address.
- Given a missing field, a notice missing a company, or a non-https address, it throws an error naming the problem, so a broken file stops the build.

**Website end-to-end tests** (the spec's acceptance criteria):
- **Terms page live and linked.** `/terms` answers 200 with no sign-in. The existing crawl reaches it from the footer. On every crawled page, the footer has a Privacy link to `/privacy`, a Terms link to `/terms` and a help link to the GitHub issues page.
- **Terms content.** The terms page:
  - names the tcgreplay project as running the playmat;
  - has a heading-labelled section for each required item;
  - states free and as is with no availability guarantee, games lost at any time, the disallowed uses (overloading or attacking, automated scraping, getting round the limits, unlawful use), minimum age 13, and that the terms and service may change or end;
  - links to the privacy page and to GitHub issues;
  - shows a last-updated date.
  - Its requests a minute, players at once and idle time equal the App Platform definition's, read by the test's own independent reader.
- **Trademarks section.** The terms' trademarks and card images section contains the shared notice text exactly, naming all four companies, and links TCGdex and malie.io.
- **Shared wording.** The footer notice on every page, and the terms' notice, equal the shared file's text, so a hand-edited copy fails.
- **Visitor counting on the privacy page.**
  - The section names Cloudflare Web Analytics and lists page, referrer, country and device.
  - It says it sets no cookies and is not linked to accounts.
  - "Analytics" no longer appears in the never-collected list. The existing assertion that it does is reversed.
- **Diagnostics on the privacy page.**
  - The section names DigitalOcean's metrics and the playmat's own usage and error records.
  - It lists their contents and what they never contain (email, name, Google tokens, battle logs, decklists, card names, IP addresses), and says how long each is kept.
  - The `logs` and `metrics` `data-retention` figures appear in the observability runbook's Retention section; the `visits` figure is checked by hand against Cloudflare's documentation.
- **Existing statements re-checked.** Assertions pin each corrected statement:
  - the sign-in record is in memory, and the copies handed out are encrypted;
  - disconnecting the assistant takes up to 30 days to reach the playmat;
  - the playmat fetches card images itself and keeps copies in DigitalOcean Spaces;
  - logs carry no email address;
  - the four browser settings.
  - They also check that the false statements are gone ("loads nothing from other sites", "writes your email address to its logs").
- **Privacy page dated.** The last-updated date is a real date in the page's format. That it equals the go-live day is a manual check.
- **Help links.** The footer, the privacy page and the terms page each link to the GitHub issues page.
- **Phone width.** The existing 375 px no-sideways-scroll check covers the terms page automatically through the crawl.

**Board end-to-end tests:**
- **Notice on the board.** At 390 px (phone), 820 px (tablet, set on the tablet profile) and 1440 px (desktop), "About tcgreplay" is reachable from ☰. The panel shows the shared notice text naming all four companies, the TCGdex and malie.io links and a help link to the GitHub issues page.
- **Covers nothing until asked.** Before the panel opens, the board's cards and controls are where they were. The phone fit check still passes. The panel closes with ✕ and with Escape, after which the board is unchanged.
- **No storage.** Opening the panel writes no browser storage key beyond the four existing settings.

**Go test:** the hosted route test fetches `/terms` signed out and gets the website's page with no redirect and no cookie, as it does for `/privacy`.

**Deliberate gaps:**
- No test reads the deployed site. Whether the pages are live at `tcgreplay.app` is a manual check after deploy.
- No test checks that Cloudflare's documented retention is what the page says. That is a manual review.
- The board's signed-in account block is not shown by the board test server, which is why the About item sits outside it.

**Success metric:**
- "The ChatGPT submission accepts the privacy, terms and support links, and its review raises no objection about missing or inaccurate policy pages": **Manual — captured in the implementation test plan.** The links' existence, public reachability and content are covered by the tests above. The review itself is OpenAI's, and it happens in the submission prep work.

**Manual reviews and checks:**
- Live at stable public addresses: after deploy, `https://tcgreplay.app/privacy`, `https://tcgreplay.app/terms` and the GitHub issues page open in a private window without signing in. **Manual — captured in the implementation test plan.**
- Privacy page accuracy review: a person reads the privacy page against `docs/observability.md` and Cloudflare Web Analytics' documentation, and confirms every recorded item is described, nothing undescribed is collected, and the retention periods match. **Manual — captured in the implementation test plan.**
- Last-updated dates: the privacy page's and the terms' last-updated dates are the day the updated pages went live. **Manual — captured in the implementation test plan.**
- Terms read-through: the maintainer reads the terms for tone and completeness against the spec's list. This is not a legal review, which is a non-goal. **Manual — captured in the implementation test plan.**
- Board on real devices: on a phone and a tablet, the About panel opens from ☰, is readable, and closes without covering or shifting cards or controls. **Manual — captured in the implementation test plan.**

## Milestones & Tasks

### Milestone 1: Terms of service are published, and every website page links to them, the privacy page and help

**What changes**: The website gains a terms of service page at `/terms`, open to anyone without signing in. It says who runs the playmat (the tcgreplay project), that it is free and provided as is, that games can be lost at any time, and what the hosted limits are, with the playmat's own figures. It also covers what is not allowed, the minimum age of 13, and that the terms and the service can change or end. It ends with a trademarks and card images section, where to get help, and a last-updated date. The footer of every website page shows the fan-project notice and the card-data credit, now taken from one shared wording the board and the ChatGPT listing will also use. It links to the privacy page, the terms and the project's GitHub issues for help.

**Validation point**: The website builds and its browser suite passes. The crawl reaches `/terms`, every page's footer carries the notice, the credit and the three links, and the terms content tests pass. The hosted route test serves `/terms` signed out with no cookie.

#### - [x] Task: Share one notice wording and show it in the website footer
**Id:** 1cd169b9-8886-4729-8233-d2e423427168
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Writes the fan-project notice, the card-data credit and the help address once, in a shared file that the website, the board and the ChatGPT listing can all read. The website build reads it, and stops with a clear message if it is broken. Every page's footer then shows the notice and credit from that file and links to the privacy page and to the project's GitHub issues for help. The container build copies the file in for both the website and the board.

*Technical detail:* [context.md#task-share-one-notice-wording-and-show-it-in-the-website-footer](./context.md#task-share-one-notice-wording-and-show-it-in-the-website-footer)

**Acceptance criteria**:
- [x] The notice (naming Nintendo, The Pokémon Company, Creatures and Game Freak), the TCGdex and malie.io credit and the GitHub issues address exist in exactly one file, and the footer of every website page shows them word for word.
- [x] A missing or broken shared file, a notice that drops one of the four companies, or a non-https address stops the website build with a message naming the problem.
- [x] Every website page's footer links to the privacy page and to the project's GitHub issues.
- [x] The container image still builds, with the guides still copied into the website build.

#### - [x] Task: Publish the terms of service page
**Id:** 958e954d-c12a-4b23-86c4-0870f30eebb4
**Repo:** tcgreplay
**Depends on:**
- 1cd169b9-8886-4729-8233-d2e423427168 — Share one notice wording and show it in the website footer
**Execution:** agent

Adds a terms of service page at `/terms`, modelled on what community tools publish, in plain words with no governing-law clause. It names the tcgreplay project as running the playmat and covers each point the spec lists. Its limits come from the playmat's own configuration, and its trademarks section uses the shared notice and credit. Every page's footer gains a Terms link, and the hosted server's route test confirms the page opens without signing in.

*Technical detail:* [context.md#task-publish-the-terms-of-service-page](./context.md#task-publish-the-terms-of-service-page)

**Acceptance criteria**:
- [x] The terms page opens at `/terms` without signing in, sets no cookie, and is linked from the footer of every website page.
- [x] It names the tcgreplay project as running the playmat and states that the playmat is free and provided as is, with no guarantee it stays available, and that games may be lost at any time.
- [x] It states the hosted limits (requests a minute, players served at once, idle games dropped after the idle time), with the same figures as the playmat's configuration.
- [x] It states the disallowed uses (overloading or attacking the service, automated scraping, getting round the limits, anything unlawful), the minimum age of 13, and that the terms and the service may change or end.
- [x] Its trademarks and card images section carries the shared non-affiliation notice naming Nintendo, The Pokémon Company, Creatures and Game Freak, and the TCGdex and malie.io credit.
- [x] It links to the privacy page and to the project's GitHub issues for help, and shows a last-updated date.

### Milestone 2: The privacy page tells the truth about visitor counting and diagnostics

**What changes**: The privacy page explains the cookieless visitor counting (Cloudflare Web Analytics): what it records, that it sets no cookies, that it is never linked to an account, and how long it is kept. It also explains the diagnostics: DigitalOcean's request and performance metrics, and the playmat's own usage and error records keyed by a pseudonymous player ID. It says what they contain, what they never contain, and how long each is kept. The promise that no analytics are collected is gone. Every older statement is brought into line with how the playmat works:
- how sign-ins are kept;
- what disconnecting an assistant does;
- where card images come from;
- that logs no longer hold email addresses.

The page shows the date it went live. Visitor counting can be switched on after this lands.

**Validation point**: The privacy suite passes with the new sections and the corrections. The logs and metrics retention figures on the page appear in the observability runbook's Retention section. The page shows its new last-updated date.

#### - [x] Task: Bring the privacy page up to date
**Id:** 8d984cd4-8af8-4541-9215-e91038a05c89
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Adds a visitor-counting section (Cloudflare Web Analytics) and a diagnostics section (DigitalOcean's metrics and the playmat's own usage and error records) to the privacy page. Both are drawn from the observability runbook and Cloudflare's documentation, with each retention period stated. It corrects every older statement the code no longer bears out, removes the promise that no analytics are collected, and sets the new last-updated date. The work needs the observability runbook, with its retention periods filled in, to exist first.

*Technical detail:* [context.md#task-bring-the-privacy-page-up-to-date](./context.md#task-bring-the-privacy-page-up-to-date)

**Acceptance criteria**:
- [x] The privacy page has a visitor-counting section naming Cloudflare Web Analytics, listing page, referrer, country and device, saying it sets no cookies and is never linked to an account, and saying how long Cloudflare keeps the counts.
- [x] It has a diagnostics section naming DigitalOcean's request and performance metrics and the playmat's own usage and error records, listing what they contain and what they never contain, and saying how long each is kept, matching the observability runbook.
- [x] It no longer says analytics are never collected, that the website loads nothing from other sites, or that logs hold email addresses.
- [x] Its cookie statement is true on the live site: the playmat sets no cookies of its own beyond the two sign-in cookies, Cloudflare (in front of the site) may set a short-lived bot-protection cookie (`__cf_bm`) that is not used for visitor counting and is not tied to an account, and the only thing loaded from another site is the visitor-counting script.
- [ ] If a What's new list (`site/src/data/whats-new.yaml`) exists when this lands, the launch entry gains this work's player-visible changes (the terms page and the board's About panel) rather than a new entry; otherwise the launch-docs launch entry covers them.
- [x] Its statements on sign-ins, disconnecting an assistant, card images and outside services match how the playmat works.
- [x] Its other statements (cookies, browser settings, games, retention of games and sign-ins, removing data) are unchanged where the code bears them out.
- [x] It shows a new last-updated date and still links to the project's GitHub issues for questions.

### Milestone 3: The board shows the fan-project notice, the card-data credit and a help link

**What changes**: On the board, the ☰ menu gains "About tcgreplay" at every screen size, on the hosted playmat and the desktop app alike. It opens a panel with the fan-project notice, the TCGdex and malie.io credit, and a link to the project's GitHub issues for help, worded exactly as on the website. Nothing appears until the player asks for it, so no card or control is ever covered, and the panel closes like the board's other views.

**Validation point**: The board's browser suite passes at phone (390 px), tablet (820 px) and desktop (1440 px) widths. The panel is reached from ☰, shows the shared wording and links, closes with ✕ and Escape, adds no browser storage, and leaves the phone board's fit unchanged.

#### - [x] Task: Add an About panel to the board
**Id:** 80cee16e-9860-43f5-b7b7-14cf144c5944
**Repo:** tcgreplay
**Depends on:**
- 1cd169b9-8886-4729-8233-d2e423427168 — Share one notice wording and show it in the website footer
**Execution:** agent

Adds "About tcgreplay" to the board's ☰ menu, at every screen size and in both the hosted and desktop board. It opens a panel showing the shared fan-project notice, the TCGdex and malie.io credit and a Get help link to the project's GitHub issues. Nothing shows until the player asks, so no card or control is covered. The panel closes like the board's other views, and it keeps nothing in the browser.

*Technical detail:* [context.md#task-add-an-about-panel-to-the-board](./context.md#task-add-an-about-panel-to-the-board)

**Acceptance criteria**:
- [x] At phone (390 px), tablet (820 px) and desktop (1440 px) widths, a player can open "About tcgreplay" from the board's ☰ menu.
- [x] The panel shows the same notice and credit wording as the website, with links to TCGdex, malie.io and the project's GitHub issues.
- [x] Before it is opened nothing covers or moves any card or control, and it closes with its close button and with Escape, leaving the board as it was.
- [x] Opening it stores nothing new in the browser.

## Open Questions

- **What retention periods the observability runbook records, and whether they are filled in when this work starts.** This depends on a person's task in the observability plan, which confirms with DigitalOcean how long Insights keeps logs and metrics.
  - If `docs/observability.md` is missing, or its Retention section has no figures, the implementer of "Bring the privacy page up to date" must STOP and ask the user rather than guess.
  - If the figures are stated as "until the preview ends" or are otherwise uncertain, the page states them as the runbook does, and the implementer reports the wording.
- **What Cloudflare Web Analytics documents about what it records and how long it keeps it.** Only Cloudflare's current documentation answers this; secondary sources say about six months.
  - If Cloudflare's documentation lists anything recorded beyond page, referrer, country and device (for example a visit duration or performance timings), the page lists it too, because nothing collected may go undescribed.
  - If it documents no retention period, the page says the counts are kept under Cloudflare's own policy, with a link.
  - No STOP is needed in either case, unless Cloudflare says Web Analytics sets a cookie or stores anything in the browser. That would contradict the visitor-analytics spec's cookieless constraint, so STOP and ask the user.

## Out of Scope

- **Building the visitor counting.** Adding Cloudflare's script, its token and the matching change to the website suite's "nothing fetched from other hosts" check all belong to the visitor-analytics spec (`20261111132045-1d1d32f6-visitor-analytics`). This plan only describes the counting on the privacy page, which that spec requires first.
- **Building the diagnostics.** The records, player IDs, alerts and runbook belong to the observability plan (`20261112090330-4ac2ea19-observability`). This plan reads the runbook and never edits it.
- **Writing the ChatGPT listing.** That is the ChatGPT submission prep spec (`20261112090820-07d70cfb-chatgpt-submission-prep`). This plan provides the `/privacy`, `/terms` and support addresses, and the shared notice file the listing can read.
- **A lawyer's review of the terms or the privacy page, and a governing law for the terms.** Both are non-goals of the spec.
- **Changing what the playmat does to make the old privacy wording true.** Examples: a token revocation endpoint so that disconnecting an assistant forgets the player at once, or stopping hosted mode writing the assistant's name to a shared file on disk. The page is corrected to match the playmat instead. Revocation would need its own spec (see `gotchas/disconnect-does-not-reach-playmat.md`).
- **Privacy and terms links on the board.** The board gets the notice, the credit and the help link the spec asks for. The desktop board serves no website, so links to `/privacy` and `/terms` would break there.
- **A Terms entry in the website's header navigation.** The footer of every page links to it, as the spec asks.
- **Moving addresses to tcgreplay.app in existing pages and docs.** Examples: the limits page, the sign-in doc's example board link, and the design notes' consent-screen step. All belong to the domain move plan (`20261111093353-8ff3589f-domain-move`).
- **Fixing the stale comment in `.do/app.yaml`** that says sign-ins are lost on every restart. It is a comment, not a published statement, and that file's edits belong to other plans in this epic.

## Changelog


### 2026-11-13 — Task: Share one notice wording and show it in the website footer

**What was done**: The fan-project notice, the TCGdex and malie.io credit and the GitHub issues address now live once, in `internal/notice/notice.json`. A new build-time loader (`site/src/lib/notice.ts`, modelled on the facts loader) reads it and stops the build with a `site notice:` message if the file is missing, not JSON, has an empty field, drops one of the four companies, or has a non-https address. The shared layout's footer renders the notice and credit from it and gains a footer navigation with Privacy, Get help (the issues page) and Source on GitHub. Both Dockerfile UI stages copy `internal/notice` in.

**Deviations**:
- The goal check's first attempt judged criterion 1 not met because `site/e2e/notice.spec.js` pinned the notice text literally, a second copy of the wording. The tests now compare the loader's output with the shared file parsed independently, and check only its rules (the four company names, the two source names, https addresses, a github.com `/issues` help page). Fixture URLs for the non-https cases use `example.org`.
- The help link's visible text is "Get help", with the accessible name "Get help on GitHub issues" built from `help.label`.
- The Dockerfile is not in the goal's scope list, but the plan's context assigns both Dockerfile edits to this task, and criterion 4 needs it.

**Files changed**:
- `internal/notice/notice.json` (new)
- `site/src/lib/notice.ts` (new)
- `site/src/layouts/Base.astro`
- `site/src/styles/site.css`
- `Dockerfile`
- `site/e2e/notice.spec.js` (new)
- `site/e2e/site.spec.js`

**Discoveries**:
- The worktree has no `node_modules`; `npm ci` in `site/` and `web/` is needed before building or testing.
- `docker` on this machine is podman; `docker build .` works and took a few minutes.
- The privacy page still builds its issues link from `site.repoURL + '/issues'`; the privacy task can switch it to `notice.help.url`.

### 2026-11-13 — Task: Publish the terms of service page

**What was done**: Added the terms of service at `/terms` (`site/src/pages/terms.astro`), modelled on community tools' terms, in plain words with no governing-law clause. It has one heading-labelled section each for who runs tcgreplay (the tcgreplay project), free and as is, your games, hosted limits (from the facts loader, with `data-limit` markers), acceptable use, age (13), trademarks and card images (the shared notice and credit), privacy, changes and help (GitHub issues, no support email), and a `data-updated` last-updated line. The footer gains a Terms link, and the hosted route test fetches `/terms` signed out.

**Deviations**:
- The last-updated date is set to the implementation day, 13 November 2026. The manual go-live check in the test plan confirms or adjusts it.
- `internal/site/site_test.go` was left unchanged (optional in the plan; it uses fixtures and needs no terms row).
- The trademarks section repeats the footer's four-line credit markup rather than sharing a component. The wording still comes only from the shared file.

**Files changed**:
- `site/src/pages/terms.astro` (new)
- `site/src/layouts/Base.astro`
- `site/e2e/terms.spec.js` (new)
- `site/e2e/site.spec.js`
- `cmd/tcgreplay/hosted_routes_test.go`

**Discoveries**:
- `TestHostedRoutes` compares the hosted response with the embedded site handler's own answer, so it passes whether or not the site was built first. Only a built site (`make site`) makes it actually exercise the terms page.
- `site/src/data/whats-new.yaml` does not exist on this branch, so no What's new entry was added (as the plan's epic-planning note directs).

### 2026-11-13 — Task: Bring the privacy page up to date

**What was done**: Rewrote the privacy page against the code, the observability runbook and Cloudflare's documentation. It gains a "Visitor counting" section (Cloudflare Web Analytics: page, referrer, country, device type, browser and operating system, page-load time; no cookies or browser storage; never linked to an account; kept six months) and a "Diagnostics" section (DigitalOcean's metrics and the playmat's own usage and error records by player ID, their contents and never-list, each kept 7 days), with `data-retention` markers. It corrects the statements on sign-ins (kept in memory, encrypted copies survive updates), disconnecting an assistant (its record runs out within 30 days), card images (fetched by the playmat into DigitalOcean Spaces) and logs (no email address). It also replaces the "no analytics" promise, names `__cf_bm` in the cookie sentence, takes the help link from the shared notice file and sets the last-updated date to 13 November 2026. `site/e2e/privacy.spec.js` covers each change, including a check that the `logs` and `metrics` figures appear in the runbook's Retention section.

**Deviations**:
- **Accepted shortfall (goal check exhausted, user chose `accept`):** criterion 5 is left unticked. `site/src/data/whats-new.yaml` does not exist, so this task changes no What's new list. The launch-docs spec's launch entry covers this work's player-visible changes, the terms page and the board's About panel (the coordinator is adding the About panel to the launch-docs plan).
- **Provisional retention:** the logs and metrics figures (7 days) are the runbook's provisional values, marked "to be confirmed by the maintainer". The user deferred confirming them, as the orchestrator directed. The test plan keeps a manual check to confirm them and update the runbook and the privacy page together.
- **The never-list follows the runbook exactly**, as the context says the runbook wins: email address or name, Google tokens or the playmat's own tokens and cookies, battle logs, decklists or card data. The plan's "card names" and "IP address" were dropped. The runbook does not list them, and redacted error messages can still name cards that are not in the player's own text. No Go code logs an IP address today, but the runbook does not promise it.
- **Wording on the visitor-counting script:** the intro says the only thing the website "may" load from another site is Cloudflare's counting script. The script is not live until the visitor-analytics spec lands. The visitor-counting section describes the counting before it is switched on, as the plan intends.
- **Cloudflare records more than the spec lists.** Cloudflare's documentation lists page-load timings and browser and operating system beyond page, referrer, country and device, so the page lists them (plan open question).
- **Card-image fallback:** a fallback image load from TCGdex or malie.io reaches them as any browser request does, and the page now says so.
- The usage list names every event in the runbook's catalogue (refused sign-ins, account removals, attempts to open another player's board, refused admin visits, admin actions).

**Files changed**:
- `site/src/pages/privacy.astro`
- `site/e2e/privacy.spec.js`

**Discoveries**:
- Cloudflare Web Analytics, docs read 13 November 2026:
  - It keeps unsampled data for 7 days, then about 10%, viewable for six months (FAQ).
  - Its dimensions are country, host, path, referrer, device type, browser, operating system and navigation type, and it collects page-load timings.
  - Its blog says it uses no cookies or localStorage. Its docs pages themselves say nothing about cookies.
- The goal reviewer reads "matching the runbook" strictly. A privacy statement beyond the runbook's lists (even a true one) is judged a mismatch.

### 2026-11-13 — Task: Add an About panel to the board

**What was done**: The board's ☰ menu gains "About tcgreplay" after the Autoplay toggle, outside every phone-only and hosted-only block, so it shows at every layout and in the desktop app. It opens a new `about` view in the board's modal system with the shared fan-project notice, the TCGdex and malie.io credit links and a "Get help" link to the GitHub issues. All of it is imported from `internal/notice/notice.json`. The view closes like the others (✕ on touch, Close with a mouse, Escape, backdrop) and keeps nothing in the browser. Vite's dev server is allowed to serve `../internal/notice`.

**Deviations**:
- `web/e2e/phone.spec.js` was left unchanged: its menu tests check particular items, not an exact list.
- To compare card and control boxes before and after, the About test moves the pointer to the corner and waits for running CSS transitions. Otherwise a hand card lifted under the pointer, or the hand still fanning out after load, reads as "moved".

**Files changed**:
- `web/vite.config.js`
- `web/src/App.svelte`
- `web/src/components/Modal.svelte`
- `web/e2e/about.spec.js` (new)

**Discoveries**:
- Board layout boxes taken with `getBoundingClientRect` are unstable right after load, because the hand fans out with a transition, and under the pointer, because cards lift on hover. A test comparing positions must settle both first: move the mouse away and await `document.getAnimations()` with a finite end time.
- In the board suite, the list reporter marks the harness's `test.fail()` cases with ✘ even though they count as passed.
