---
created_date: "2026-11-13"
document_status: final
closed_date: "2026-11-13"
---

# Test plan: 20261111132045-1d1d32f6-visitor-analytics

Manual checks for visitor counting on the live playmat. The automated suites already cover the code:
- the Go tests in `internal/webanalytics`, `internal/site` and `cmd/tcgreplay` (`hosted_analytics_test.go`);
- the site suite's `site/e2e/analytics.spec.js`;
- the board suite's `web/e2e/analytics.spec.js`.

None of them loads Cloudflare's real beacon or reads the dashboard. Everything below needs the deployed playmat at `https://tcgreplay.app`, the Cloudflare dashboard (Web Analytics) or DigitalOcean (App Platform and Insights).

**Who / when:** the maintainer, after this branch is merged and deployed. Part A switches counting on; it is the plan's task "Switch visitor counting on for tcgreplay.app", deferred to the maintainer: live switch-on after deploy, per the user. Parts B and C follow once counting is on, and Part C's metrics a week after that.

Two things to know first:
- Cloudflare's `__cf_bm` cookie comes from the hosting edge, not from analytics (knowledge `gotchas/live-site-cf-bm-cookie.md`). Do not count it as an analytics cookie.
- What Cloudflare Web Analytics keeps, and for how long, is in knowledge `learnings/cloudflare-web-analytics-data.md`.

## Part A: switch counting on (deferred task, do in order)

### A1. Privacy page first
- **What:** the privacy page describing visitor counting is live before any token is set. This is the spec's "Privacy page first" criterion.
- **How:** open `https://tcgreplay.app/privacy` in a private window. Find the visitor-counting section: it names Cloudflare Web Analytics and says Cloudflare keeps the counts for "six months". In the page source, that figure sits in an element with `data-retention="visits"`.
- **Pass:** the section is there. If it is not, stop: counting must not start. The playmat also refuses to count without it.

### A2. The code is deployed, and counting is off
- **How:** DigitalOcean → the app → Insights / Runtime Logs for the `playmat` component. Find the start-up lines of the current deployment.
- **Pass:** a line reads exactly `visitor analytics: off (no TCGREPLAY_WEB_ANALYTICS_TOKEN)`.

### A3. Create the Cloudflare Web Analytics site
- **How:** Cloudflare dashboard → Web Analytics → Add a site, and enter `tcgreplay.app`.
  - If the zone is proxied and Cloudflare offers automatic setup, choose **Enable with manual snippet installation**. Otherwise make sure automatic injection is off.
  - Copy the token: the `token` value inside the snippet's `data-cf-beacon`.
- **Pass:** the site exists, set up for the manual snippet, with automatic injection off. The token is 16 to 64 letters and digits, typically 32 hex characters. The playmat refuses any other token with a warning.

### A4. Set the token on the playmat
- **How:** App Platform → the app → Settings → `playmat` → Environment Variables. Set `TCGREPLAY_WEB_ANALYTICS_TOKEN` to the token and save; this restarts the playmat. If the variable is not declared on the platform yet, first run `doctl apps update <app-id> --spec .do/app.yaml`, which declares it without a value.
- **Pass:** the deployment restarts without errors.

### A5. Counting is on, with one beacon
- **How:**
  1. Read the start-up log again.
  2. Run `curl -s https://tcgreplay.app/ | grep -o 'static.cloudflareinsights.com/beacon.min.js[^>]*'`, or view the page source.
  3. Sign in, open `https://tcgreplay.app/board` and view its page source.
- **Pass:**
  - The log reads exactly `visitor analytics: on for tcgreplay.app`, and no log line contains the token.
  - Each page source holds exactly one `<script defer src="https://static.cloudflareinsights.com/beacon.min.js" data-cf-beacon='{"token":"…","spa":false}'>` tag.
- **If the log says on but there is no beacon:** App Platform may not pass `tcgreplay.app` in the request's `Host` (plan Open Questions). Stop and raise it; the candidate fix, also matching `X-Forwarded-Host`, changes which requests count.
- **If there are two beacon tags:** Cloudflare's automatic injection is on. Switch it off in the Cloudflare dashboard.

**To switch counting off later:** clear `TCGREPLAY_WEB_ANALYTICS_TOKEN` in the control panel; a restart is enough. The log then reads `visitor analytics: off (no TCGREPLAY_WEB_ANALYTICS_TOKEN)`.

## Part B: manual checks once counting is on

### B1. Website page views appear (spec acceptance: "Website page views appear")
- **How:** in a private window, visit `https://tcgreplay.app/` and `https://tcgreplay.app/docs/connect`. Wait up to 30 minutes, then open Cloudflare dashboard → Web Analytics → tcgreplay.app, last 30 minutes or last hour.
- **Pass:** both `/` and `/docs/connect` appear in the page views or top pages.

### B2. One board session, one board visit (spec acceptance)
- **How:**
  1. Sign in and open `https://tcgreplay.app/board`.
  2. Open several card zooms and switch branches in the branch picker. On a phone, also open and close a card's action sheet.
  3. Do not reload.
  4. After up to 30 minutes, filter the dashboard by path `/board`.
- **Pass:** that session shows as one page view of `/board`, not one per zoom, branch switch or action sheet. Read board sessions as `/board` page views: Cloudflare counts a "visit" only when the referrer is another site, so a player who came from the home page makes a page view but no new visit.

### B3. Totals, top pages and breakdowns (spec acceptance)
- **How:** in the dashboard, choose the last 7 days.
- **Pass:** visit and page-view totals, a top-pages list, and referrer, country and device-type breakdowns are all shown.

### B4. No browser storage, no identity, on the live site (spec acceptance)
- **How:** open devtools on `https://tcgreplay.app/`, a docs page and the signed-in `/board`, with counting on.
  - Application → Cookies, Local Storage and Session Storage.
  - Network → filter `cloudflareinsights`, and open the beacon's request to `cloudflareinsights.com/cdn-cgi/rum`, or `/cdn-cgi/rum` on a proxied zone.
- **Pass:**
  - Nothing in cookies or storage comes from analytics. `__cf_bm` is the edge's, not analytics. On `/board`, local storage holds only the board's own settings: `replaySpeed`, `autoplay`, `panelAt`, `prizeReminders`.
  - No analytics request contains the signed-in email address, the account, a `?board=` handle, game or card data.
- **If anything from `cloudflareinsights.com` sets a cookie or writes storage:** stop and raise it. It contradicts the spec's cookieless constraint (plan Open Questions).

### B5. Blocked analytics change nothing, live (spec acceptance)
- **How:** turn on a content blocker that blocks `static.cloudflareinsights.com`, for example uBlock Origin. Load the home page, a docs page and a copy button, then sign in and use the board: a zoom, a move.
- **Pass:** everything loads and works, and no error is shown to the visitor.

### B6. No banner (spec acceptance)
- **How:** visit the home page, a docs page, the privacy page and the board, in a fresh private window.
- **Pass:** no consent banner or analytics prompt appears on any page.

### B7. Local and test runs are silent (spec acceptance)
- **How:** after running the playmat locally (`make dev`, or `make docker-run`, which deliberately does not pass the token) and the test suites (`make test`), check the dashboard's hostnames or top pages for the last day.
- **Pass:** no `localhost`, `127.0.0.1`, `*.ondigitalocean.app` or other-host traffic. Only `tcgreplay.app` appears.

## Part C: success metrics (a week after switch-on)

### C1. Visits and board sessions answerable from the dashboard alone
- **Metric:** a week after launch, the maintainer can answer "how many visits did the site get, and how many board sessions, this week?" from the dashboard alone.
- **How:** in Cloudflare Web Analytics → tcgreplay.app, choose the last 7 days. Read the visits total, then filter by path `/board` and read its page-view count.
- **Pass:** both numbers come from the dashboard, with no other source needed.

### C2. Only tcgreplay.app traffic
- **Metric:** the dashboard shows only tcgreplay.app traffic.
- **How:** in the same 7-day view, look at the hostname breakdown, or the full URLs in the top pages.
- **Pass:** every entry is on `tcgreplay.app`. There is nothing from `tcgreplay.demo.gs`, which the domain move redirects, nothing from the App Platform default address and nothing from local hosts.
