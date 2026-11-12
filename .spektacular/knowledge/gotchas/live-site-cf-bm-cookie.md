---
tags: [hosting, cloudflare, cookies, privacy, site, analytics]
---

# The live site carries a `__cf_bm` cookie the playmat never sets

- **What you see.** A request to `https://tcgreplay.app/` comes back with `Set-Cookie: __cf_bm=…; HttpOnly; SameSite=None; Partitioned; Secure; Path=/; Domain=tcgreplay.app` (expires after 30 minutes), alongside `server: cloudflare`, `cf-ray` and `x-do-app-origin` headers. Observed on 2026-11-12 with `curl -sS -D - -o /dev/null https://tcgreplay.app/`.
- **Who sets it.** Cloudflare's bot management, at the Cloudflare edge in front of App Platform. The headers do not say whether that is the maintainer's own proxied zone (the design doc's operator steps mention Cloudflare bot protection on the domain) or App Platform's own Cloudflare-based edge. Either way it is not the playmat: its Go handlers set no cookie on website pages (`internal/site/site.go`). It is not Cloudflare Web Analytics either: the page carried no analytics beacon when it was observed.
- **Why it matters.**
  - The site and board browser suites run against local Go servers and never see this cookie, so a green "sets no cookie" test says nothing about the live response.
  - A devtools check on the live site ("no cookie comes from analytics", "the site sets no cookies") must tell `__cf_bm` apart from anything the playmat or analytics sets. It belongs to the hosting edge.
  - A privacy statement that the website "never sets a cookie" is true of the playmat but not of the response a visitor's browser receives. Wording about cookies should say whose they are.
