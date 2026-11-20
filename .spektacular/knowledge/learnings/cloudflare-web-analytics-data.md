---
tags: [cloudflare, analytics, privacy, retention]
---

# What Cloudflare Web Analytics records and keeps, per its own docs

*Checked 13 November 2026, for the privacy page's visitor-counting section (`site/src/pages/privacy.astro`).*

- **What it records.**
  - Its dimensions page (developers.cloudflare.com/web-analytics/data-metrics/dimensions/) lists country, host, path, referrer (host and path), device type, browser, operating system, site and navigation type.
  - Its data-origin page (.../data-metrics/data-origin-and-collection/) adds page-load timings (Navigation Timing) and Core Web Vitals.
  - It logs no query strings (FAQ).
  - So a privacy statement listing only "page, referrer, country and device" undersells it. Name browser, operating system and page-load time too.
- **How long.** The FAQ (.../web-analytics/faq/) says:
  - unsampled beacon data is kept for 7 days, then aggregated down to about 10%;
  - data is viewable for the previous six months.

  The limits page gives no retention figure. The privacy page states six months ("full detail for the first 7 days, then a sample of about a tenth"), in `data-retention="visits"`. Nothing in the repo records that figure, so no test checks it: it is a manual check against the FAQ.
- **Cookies.** The docs pages say nothing about cookies or browser storage. The "no cookies or localStorage, no fingerprinting" claim comes from Cloudflare's launch blog post (blog.cloudflare.com/free-privacy-first-analytics-for-a-better-web/). Cite the blog when a statement needs a source.
- **Do not confuse it with `__cf_bm`.** That cookie comes from Cloudflare's bot management at the edge, not from Web Analytics (see `gotchas/live-site-cf-bm-cookie.md`).

Why it matters: every privacy-page statement about visitor counting must be checkable against these pages. Recheck them when the visitor-analytics work switches counting on, and whenever the page's retention or field list changes.
