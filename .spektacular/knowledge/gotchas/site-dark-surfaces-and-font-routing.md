---
tags: [site, webui, css, theme, playwright, testing, fonts]
---

# Website: dark-in-every-theme surfaces, and font routing in its tests

**A surface that is dark in every theme takes `data-theme="dark"`.** The website's footer and code boxes stay dark even in the light theme. Rather than adding new tokens or borrowing mat tokens, they carry `data-theme="dark"` on their element and use the ordinary Studio names (`--panel`, `--ink`, `--muted`, `--line`). The generated `tokens.css` redefines those names under `[data-theme='dark']`, and custom properties inherit, so everything inside takes the dark values. Every footer and code-box colour in the Studio mockups is exactly a dark-theme token value. The board can do the same for any always-dark piece.

**Website specs import `test` from `site/e2e/fixtures.js`.** The site loads IBM Plex from Google Fonts, and the tests stay offline by routing `fonts.googleapis.com` and `fonts.gstatic.com` to `testdata/fonts` (`serveFonts(context)`, mirroring the board's `web/e2e/fixtures.js`).
- A spec importing `test` straight from `@playwright/test` makes real requests to Google, or fails offline.
- A context a spec creates itself (as `crawl.js` does) needs `serveFonts(context)` too.
- Playwright tries the most recently registered route first, so a catch-all `context.route('**', …)` added later (as the analytics shield is) must `route.fallback()` for the font hosts, or it swallows them.
