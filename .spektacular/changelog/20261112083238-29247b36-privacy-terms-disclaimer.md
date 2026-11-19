---
created_date: "2026-11-13"
document_status: final
closed_date: "2026-11-13"
---

# Privacy page, terms of service and the fan-project notice everywhere

## What was built

- **One shared notice.** The fan-project notice, the card-data credit and the help address are written once, in `internal/notice/notice.json`:
  - the notice says tcgreplay is an unofficial fan project, not affiliated with, endorsed, sponsored or approved by Nintendo, The Pokémon Company, Creatures or Game Freak;
  - the credit names TCGdex and malie.io;
  - the help address is the project's GitHub issues page.

  The website build reads it through a new loader (`site/src/lib/notice.ts`). The loader stops the build, with a message naming the problem, if the file is missing, not JSON, has an empty field, drops one of the four companies, or has a non-https address. Both container build stages copy the file in. The ChatGPT listing work can read the same file.
- **Website footer.** Every page's footer renders the notice and credit from the shared file and links to Privacy, Terms, Get help (GitHub issues) and Source on GitHub.
- **Terms of service** at `/terms`, open without signing in. It is modelled on community tools' terms, in plain words, with no governing-law clause. It covers:
  - who runs it (the tcgreplay project);
  - free and as is, with no availability guarantee;
  - games lost at any time;
  - the hosted limits, read from the App Platform definition;
  - acceptable use;
  - the minimum age of 13;
  - trademarks and card images (from the shared notice);
  - privacy, changes and help;
  - a last-updated date.

  The hosted route test checks `/terms` signed out with no cookie.
- **Privacy page brought up to date.**
  - **New "Visitor counting" section.** Cloudflare Web Analytics: page, referrer, country, device type, browser and operating system, page-load time. No cookies or browser storage, never linked to an account, kept six months.
  - **New "Diagnostics" section.** DigitalOcean's request and performance metrics, and the playmat's own usage and error records keyed by a pseudonymous player ID. It lists their contents and what they never contain, with each kept 7 days.
  - **Corrected older statements:**
    - sign-ins are kept in memory, and the encrypted copies survive updates;
    - disconnecting an assistant takes up to 30 days to reach the playmat;
    - the playmat fetches card images into DigitalOcean Spaces;
    - logs hold no email address.
  - **Cookies:** the cookie sentence names Cloudflare's `__cf_bm` bot-protection cookie.
  - **Analytics promise:** the "no analytics" promise is replaced.
  - **Date:** the page has a new last-updated date.
  - **Test against the runbook:** a browser test checks the logs and metrics retention against the observability runbook's Retention section.
- **Board About panel.**
  - The board's ☰ menu gains "About tcgreplay" at every layout, on the hosted board and in the desktop app.
  - It opens a panel with the shared notice, the credit links and a Get help link, worded exactly as on the website.
  - It covers nothing until it is opened, closes with ✕, Close or Escape, and stores nothing in the browser.

## Why it matters

TCG Replay is going public and being submitted to ChatGPT. A public service, and the ChatGPT review, expect three things:
- a privacy policy that stays truthful once visitor counting and diagnostics are added;
- terms of service;
- a clear place to ask for help.

Players meet the playmat on the website and on the board. In both places they now see that it is an unofficial fan project with no connection to Nintendo or The Pokémon Company, and where its card data comes from. The shared notice file keeps the footer, the terms, the board and the ChatGPT listing from drifting apart. The privacy page had to describe visitor counting before the visitor-analytics work could switch it on.

## Deviations from the plan

- **The "What's new" launch entry is left to launch-docs (accepted shortfall).** `site/src/data/whats-new.yaml` did not exist, so this work changed no What's new list. The privacy task's criterion on it was left unticked, and the user accepted that. The launch-docs spec's launch entry covers the terms page and the board's About panel.
- **The diagnostics retention is provisional.** The logs and metrics figures (7 days) are the observability runbook's provisional values, awaiting the maintainer's confirmation with DigitalOcean, which the user deferred. The test plan has a manual step to confirm them and update the runbook and the privacy page together.
- **The privacy page's "never contain" list follows the runbook exactly.** The plan's "card names" and "IP address" were dropped: the runbook does not list them, and redacted error messages can still name cards.
- **Visitor counting is described before it is switched on.** The privacy page says the website "may" load Cloudflare's counting script, because the script is not live until the visitor-analytics spec lands. The visitor-counting section lists more than the spec named (browser, operating system, page-load time), because Cloudflare's documentation says it records them.
- **The card-image wording now says** that a fallback image load from TCGdex or malie.io reaches them as any browser request does.
- **The website's shared-notice tests check rules, not a copy of the wording.** They read the shared file independently and check its rules (the four companies, the sources, https addresses), because a literal copy in a test would be a second copy of the wording.
- **The last-updated dates** on the privacy page and the terms are the implementation day, 13 November 2026. The test plan checks them against the go-live day.
- `internal/site/site_test.go` and `web/e2e/phone.spec.js` needed no change.
