---
tags: [analytics, cloudflare, hosting, site, webui, privacy]
---

# Visitor counting is added at serve time, by hosted mode only

The website (Astro, `site/`) and the board (Vite, `web/`) share no page shell, and neither build carries Cloudflare Web Analytics. The beacon is added to the HTML on its way out by `internal/webanalytics`, which hosted mode applies in `hostedHandler` to exactly two routes: the website mount (`/`) and the `GET /board` page route, outside `RequireSession`.

- **Counting is on only when all of these hold.** `webanalytics.Setup` decides once at start-up and logs the status, never the token:
  - `TCGREPLAY_WEB_ANALYTICS_TOKEN` is set;
  - the token is 16 to 64 letters and digits;
  - the embedded privacy page carries `data-retention="visits"` (`site.DescribesVisitorCounting`);
  - the request's host is the public URL's host, ignoring case and port.
- **Only successful `GET` HTML pages are changed.** Status 200 with `text/html`. Redirects (a signed-out `/board`), the 404 page, the 403 "Not your game" page, 503 refusals, `HEAD`, range responses and assets pass through byte for byte. No route special-casing is needed to keep them out of the counts.
- **The tag turns single-page-app tracking off** (`"spa":false`). This is manual-snippet only. The board's touch action sheet pushes history entries, which SPA mode would count as extra page views.
- **Do not add an analytics script to `Base.astro` or `web/index.html`.** It would count local builds and test runs, need a rebuild to switch off, and bypass the privacy-page gate.
- **Tests opt in and never call out.** The site and board test servers (`internal/site/sitetest/cmd/serve`, `internal/webui/webtest/cmd/boardtest`) add the beacon only with `-analytics-token`. Each browser suite runs a second server for that (site on 127.0.0.1:7792, board on 127.0.0.1:7789). The `analytics.spec.js` files route every other-host request to a stub or an abort before loading any page.
- **The privacy hook in Go tests.** Hosted Go tests swap `privacyDescribesCounting` (`cmd/tcgreplay/hosted.go`), because they embed whatever website happens to be built.
- **Content security policy.** No CSP is sent today. A future one must allow `script-src https://static.cloudflareinsights.com` and `connect-src https://cloudflareinsights.com`.
