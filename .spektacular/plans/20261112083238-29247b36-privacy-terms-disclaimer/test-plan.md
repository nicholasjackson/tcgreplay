---
created_date: "2026-11-13"
document_status: final
closed_date: "2026-11-13"
---

# Test plan: 20261112083238-29247b36-privacy-terms-disclaimer

These checks cover what the automated suites cannot: the deployed site, people's judgement, and outside services' own documentation and reviews. The automated coverage is in `site/e2e/notice.spec.js`, `site/e2e/terms.spec.js`, `site/e2e/privacy.spec.js`, `site/e2e/site.spec.js`, `web/e2e/about.spec.js` and `cmd/tcgreplay/hosted_routes_test.go`.

## 1. Success metric: the ChatGPT submission accepts the privacy, terms and support links

- **What to measure:** OpenAI's review of the TCG Replay plugin listing accepts the policy and support links and raises no objection about missing or inaccurate policy pages.
- **How:**
  1. In the submission (the ChatGPT submission prep spec's work), give these three addresses:
     - privacy: `https://tcgreplay.app/privacy`;
     - terms: `https://tcgreplay.app/terms`;
     - support: `https://github.com/nicholasjackson/tcgreplay/issues`.
  2. The listing description can take its fan-project notice from `internal/notice/notice.json` (`notice`).
  3. Read the review's result and any reviewer comments.
- **Expected result:** the review passes with zero comments about the privacy policy, the terms or the support contact. Any such comment is a failure: fix the page, then resubmit.
- **Who / when:** the maintainer, when the ChatGPT submission is made (after this spec and visitor-analytics are live).

## 2. The pages are live at stable public addresses without signing in

- **What to check:** the three addresses open for a signed-out visitor.
- **How:** after the deploy that carries this work (the push to main), open a private browser window with no Google session and visit each address:
  - `https://tcgreplay.app/privacy`;
  - `https://tcgreplay.app/terms`;
  - `https://github.com/nicholasjackson/tcgreplay/issues`.

  Also run `curl -sS -o /dev/null -w '%{http_code} %{redirect_url}\n' https://tcgreplay.app/terms`, and the same for `/privacy`.
- **Expected result:** each page loads with HTTP 200 and no redirect to a sign-in. The terms page's heading is "Terms of service", and the privacy page shows the "Visitor counting" and "Diagnostics" sections. On every website page, the footer links Privacy, Terms, Get help and Source on GitHub.
- **Who / when:** the maintainer, straight after the deploy.

## 3. Privacy page accuracy review

- **What to review:** every statement on `https://tcgreplay.app/privacy` (source `site/src/pages/privacy.astro`) against its sources.
- **How:** read the page side by side with:
  - **The observability runbook, `docs/observability.md`:**
    - the "What is recorded" event list matches the page's usage and error records;
    - the "What is never recorded" list matches the page's never-list;
    - "Player IDs" matches the page's player-ID paragraph;
    - "Retention" matches the page's `data-retention="logs"` and `data-retention="metrics"` figures, both "7 days" today.
  - **Cloudflare Web Analytics' documentation** (the page's header comment lists the URLs; see also knowledge `learnings/cloudflare-web-analytics-data.md`):
    - the dimensions page lists what is recorded (country, path, referrer, device type, browser, operating system) and the data-origin page the page-load timings;
    - the FAQ gives the retention behind `data-retention="visits"`: "six months", full detail for 7 days, then about a tenth;
    - nothing in Cloudflare's documentation says Web Analytics sets a cookie or stores anything in the browser.
  - **The live response:** `curl -sS -D - -o /dev/null https://tcgreplay.app/` shows only Cloudflare's `__cf_bm` cookie, and no cookie from the playmat. After visitor-analytics is switched on, the browser's devtools (Network) show that the only request to another host is Cloudflare's counting script and its beacon.
- **Expected result:**
  - Every recorded item is described.
  - Nothing is collected that the page does not describe.
  - The three retention figures match their sources exactly.
  - Any mismatch is a failure: correct the page and redeploy.
- **Who / when:** the maintainer, before the ChatGPT submission, and again when visitor-analytics switches counting on.

## 4. Confirm the provisional diagnostics retention and update both documents together

- **What to check:** the logs and metrics retention figures are still provisional. `docs/observability.md`'s Retention section says "7 days, provisional ... to be confirmed by the maintainer", and the privacy page states the same 7 days.
- **How:**
  1. Confirm with DigitalOcean (the Insights console, its docs or support) how long Insights keeps logs and metrics.
  2. Replace the provisional figures in `docs/observability.md` with the confirmed periods, the date and the source.
  3. In the same change, update the `data-retention="logs"` and `data-retention="metrics"` text in `site/src/pages/privacy.astro`, and its last-updated date.
  4. Run `make site-test`.
- **Expected result:** both documents state the same confirmed figures. The `site/e2e/privacy.spec.js` test "its logs and metrics retention appear in the observability runbook's Retention section" passes. A page figure that the runbook does not contain fails that test.
- **Who / when:** the maintainer. The user has deferred this; do it before the ChatGPT submission if possible, and in any case as soon as DigitalOcean gives a figure.

## 5. Last-updated dates match the go-live day

- **What to check:** the privacy page and the terms each show "Last updated 13 November 2026." (the implementation day). That must be the day the updated pages went live.
- **How:** after the deploy, compare the date on `https://tcgreplay.app/privacy` and `https://tcgreplay.app/terms` with the deploy date (App Platform's deployment history). If they differ, set `updated` in `site/src/pages/privacy.astro` and `site/src/pages/terms.astro` to the go-live day (format `D Month YYYY`) and push.
- **Expected result:** both dates equal the go-live day.
- **Who / when:** the maintainer, on the day of the deploy.

## 6. Terms read-through

- **What to review:** `https://tcgreplay.app/terms` (source `site/src/pages/terms.astro`), for tone and completeness. This is not a legal review, which is a non-goal.
- **How:** read the page and tick each point the spec lists:
  - the tcgreplay project runs it;
  - free and as is, with no availability guarantee;
  - games can be lost at any time;
  - the hosted limits, with today's figures (300 requests a minute, 10 players at once, 60 minutes idle);
  - acceptable use: no overloading or attacking, no automated scraping, no getting round the limits, nothing unlawful;
  - minimum age 13;
  - the terms and the service can change or end;
  - the trademarks and card images section;
  - the privacy link;
  - help through GitHub issues;
  - the last-updated date.
- **Expected result:** every point is present, in plain words, with no governing-law clause, and the tone suits a free fan project. Any missing point, or wording the maintainer is unhappy with, is a failure: edit the page.
- **Who / when:** the maintainer, before the ChatGPT submission.

## 7. Board About panel on real devices

- **What to check:** the About panel on a real phone and a real tablet. The automated suite only emulates touch in Chromium, so this checks iOS Safari and Android Chrome.
- **How:** on a phone (portrait) and on a tablet:
  1. Sign in at `https://tcgreplay.app/board` with a game open.
  2. Tap ☰, then "About tcgreplay".
  3. Read the panel. It shows the fan-project notice, "Card data and images from TCGdex and malie.io." with both linked, and "Get help (GitHub issues)".
  4. Close it with ✕, open it again and close it by tapping the backdrop.
- **Expected result:**
  - The text is readable without zooming and wraps with no sideways scrolling.
  - The links open the right sites in a new tab.
  - Before and after the panel, no card or control is covered or has moved, and the phone board still fits the screen with nothing scrolling.
- **Who / when:** the maintainer, after the deploy.
