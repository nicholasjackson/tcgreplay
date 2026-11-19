---
tags: [playwright, testing, site, webui]
---

# A spec aimed at a second test server must pass its address itself

Both browser suites can start more than one test server (`webServer` is an array in `site/playwright.config.js` and `web/playwright.config.js`; the analytics specs use a second one). A spec that switches with `test.use({ baseURL: 'http://127.0.0.1:<port>' })` changes the `page` and `request` fixtures. It does **not** change `testInfo.project.use.baseURL`, which still names the project's default server.

The site suite's `beforeAll` crawl (`site/e2e/crawl.js`) runs in its own browser context and is given a base URL. A spec for another server must therefore pass that server's address to `crawl` explicitly, not `testInfo.project.use.baseURL`. Any routes it needs, such as blocking Cloudflare, go in through `crawl`'s `prepare(context)` hook, because the crawl's context is not the test's.

On the board suite, the shared `fixtures.js` `page` fixture resets the default server's game with `POST /test/reset` and loads `/`. A spec for the second board server uses Playwright's base `test` with its own fixture that resets that server instead (see `web/e2e/analytics.spec.js`).
