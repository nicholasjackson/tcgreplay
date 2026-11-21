// Checks every page of the built website with visitor counting on, served
// by the second test server (playwright.config.js), which adds Cloudflare
// Web Analytics' beacon the way hosted mode does with a token: each page
// carries exactly one beacon, with single-page-app tracking off and
// nothing but the token; nothing else is fetched from another host but
// Google Fonts (served locally, fixtures.js);
// nothing is stored in the browser; and a visitor who blocks the beacon
// sees and can do exactly the same as before.
//
// No test here sends anything to Cloudflare. Every browser context routes
// every request for another host before it loads a page: the beacon
// script to a stub (or an abort, for blocked analytics), the Google Fonts
// hosts on to the local fonts, and everything else, Cloudflare's
// reporting address included, to an abort.
import { test, expect, fontHosts } from './fixtures.js';
import { crawl } from './crawl.js';

// The analytics server and its token, as playwright.config.js starts it.
const base = 'http://127.0.0.1:7792';
const token = '0123456789abcdef0123456789abcdef';
const beacon = 'https://static.cloudflareinsights.com/beacon.min.js';
// The stub counts how many times the beacon script ran on the page.
const stub = 'window.__beacon = (window.__beacon || 0) + 1;';

test.use({ baseURL: base });

// shield routes every request for another host: the beacon to the stub,
// or to an abort when blocked, Google Fonts on to the fixture's local
// fonts, and anything else to an abort, recording what it stopped.
async function shield(context, { blocked = false } = {}) {
  const host = new URL(base).host;
  const stopped = [];
  await context.route(
    (url) => url.host !== host,
    (route) => {
      const url = route.request().url();
      if (fontHosts.includes(new URL(url).host)) return route.fallback();
      if (url === beacon && !blocked) {
        return route.fulfill({ status: 200, contentType: 'text/javascript', body: stub });
      }
      stopped.push(url);
      return route.abort('blockedbyclient');
    },
  );
  return stopped;
}

let pages;
test.beforeAll(async ({ browser }) => {
  pages = await crawl(browser, base, (context) => shield(context, { blocked: true }));
  expect(pages.length).toBeGreaterThan(1);
  expect(pages.filter((p) => p.status !== 200)).toEqual([]);
});

test('every page carries exactly one beacon, with single-page-app tracking off and only the token', async ({ context, page }) => {
  await shield(context);
  for (const { path } of pages) {
    await page.goto(path);
    const tags = page.locator(`script[src="${beacon}"]`);
    await expect(tags, `${path}: one beacon tag`).toHaveCount(1);
    expect(await page.locator('script[src*="cloudflareinsights"]').count(), `${path}: no other Cloudflare script`).toBe(1);
    const config = JSON.parse(await tags.getAttribute('data-cf-beacon'));
    expect(config, `${path}: beacon settings`).toEqual({ token, spa: false });
    expect(await tags.getAttribute('defer'), `${path}: deferred`).not.toBeNull();
    await expect.poll(() => page.evaluate(() => window.__beacon), `${path}: the beacon ran once`).toBe(1);
  }
});

test("the site's own 404 page is not counted", async ({ request }) => {
  const resp = await request.get('/no-such-page');
  expect(resp.status()).toBe(404);
  expect(await resp.text()).not.toContain('cloudflareinsights');
});

test('the only other addresses asked are the stubbed beacon script and Google Fonts, and nothing is stored', async ({ context, page }) => {
  const host = new URL(base).host;
  const stopped = await shield(context);
  const requests = [];
  context.on('request', (r) => requests.push(r.url()));
  expect(await context.cookies()).toEqual([]);
  for (const { path } of pages) {
    await page.goto(path);
    await page.waitForLoadState('networkidle');
    expect(await context.cookies(), `${path}: cookies`).toEqual([]);
    const stored = await page.evaluate(() => ({
      cookie: document.cookie,
      local: localStorage.length,
      session: sessionStorage.length,
    }));
    expect(stored, `${path}: browser storage`).toEqual({ cookie: '', local: 0, session: 0 });
  }
  const elsewhere = requests.filter((url) => {
    const u = new URL(url);
    if (u.protocol === 'data:' || u.protocol === 'blob:') return false;
    return u.host !== host;
  });
  expect(elsewhere.length, 'the beacon was loaded').toBeGreaterThan(0);
  // The beacon's address carries nothing: no query, no page data. Google
  // Fonts is the only other host asked.
  expect(elsewhere, 'the beacon was asked for').toContain(beacon);
  const others = [...new Set(elsewhere.filter((url) => url !== beacon).map((url) => new URL(url).host))];
  expect(others.filter((h) => !fontHosts.includes(h)), 'requests to other hosts').toEqual([]);
  expect(stopped, 'requests stopped on their way to another host').toEqual([]);
});

test('with the beacon blocked, every page loads with its content and navigation, raises no error, and copy buttons work', async ({ context, page }) => {
  await shield(context, { blocked: true });
  await context.grantPermissions(['clipboard-read', 'clipboard-write'], { origin: base });
  const errors = [];
  page.on('pageerror', (e) => errors.push(`${page.url()}: ${e.message}`));
  let copied = 0;
  for (const { path } of pages) {
    const resp = await page.goto(path);
    expect(resp.status(), `${path}: status`).toBe(200);
    await expect(page.locator('h1').first(), `${path}: heading`).toBeVisible();
    await expect(page.locator('header img[src="/logo.png"]'), `${path}: logo`).toBeVisible();
    await expect(page.getByRole('navigation', { name: 'Site' }), `${path}: site nav`).toBeAttached();
    await expect(page.locator('footer .fan-notice'), `${path}: footer`).toBeVisible();
    await expect(page.locator('footer').getByRole('navigation', { name: 'Footer' }), `${path}: footer links`).toBeVisible();
    expect(await page.evaluate(() => window.__beacon), `${path}: the beacon never ran`).toBeUndefined();
    await expect(page.getByRole('alert'), `${path}: no error shown`).toHaveCount(0);

    const blocks = page.locator('[data-copy]');
    const n = await blocks.count();
    for (let i = 0; i < n; i++) {
      const block = blocks.nth(i);
      const want = await block.evaluate((el) => el.dataset.copyText ?? el.textContent);
      const button = block.locator('xpath=..').locator('button.copy');
      await page.evaluate(() => navigator.clipboard.writeText(''));
      await button.click();
      await expect(button).toHaveText('Copied');
      expect(await page.evaluate(() => navigator.clipboard.readText()), `${path}: block ${i}`).toBe(want);
      copied++;
    }
  }
  expect(copied, 'copy buttons checked').toBeGreaterThan(0);
  expect(errors, 'page errors').toEqual([]);
});
