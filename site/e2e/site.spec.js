// Checks every page of the built website, served by the Go site handler:
// the shared layout (logo, navigation, fan notice, TCGdex credit), no
// sideways scrolling on a 375px phone, no cookies, nothing fetched from
// any other host (no web fonts, analytics or third-party resources), and
// copy buttons that copy exactly their block's text.
//
// The pages are found by crawling same-origin links from /. Every page the
// site links to must exist: a link to a missing page fails the suite.
import { readFileSync, existsSync } from 'node:fs';
import { test, expect } from '@playwright/test';

const companies = ['Nintendo', 'The Pokémon Company', 'Creatures', 'Game Freak'];

const navLinks = [
  { name: 'Home', href: '/' },
  { name: 'Get started', href: '/docs/connect' },
  { name: 'Guides', href: '/docs/workflows' },
  { name: 'Example', href: '/docs/example' },
  { name: 'Limits', href: '/docs/limits' },
  { name: 'Privacy', href: '/privacy' },
];

// pagePath is a same-origin URL's path, without query or fragment.
function pagePath(href, origin) {
  const u = new URL(href, origin);
  if (u.origin !== origin) return undefined;
  return u.pathname;
}

// crawl visits / and every same-origin link it reaches, breadth-first and
// deduplicated, returning each page's path and HTTP status.
async function crawl(browser, baseURL) {
  const origin = new URL(baseURL).origin;
  const context = await browser.newContext({ baseURL });
  const page = await context.newPage();
  const seen = new Set(['/']);
  const queue = ['/'];
  const pages = [];
  try {
    while (queue.length) {
      const path = queue.shift();
      const resp = await page.goto(path);
      expect(resp, `a response for ${path}`).not.toBeNull();
      expect([200, 404], `${path} answers a page`).toContain(resp.status());
      pages.push({ path, status: resp.status() });
      const hrefs = await page.locator('a[href]').evaluateAll((as) => as.map((a) => a.href));
      for (const href of hrefs) {
        const p = pagePath(href, origin);
        if (p !== undefined && !seen.has(p)) {
          seen.add(p);
          queue.push(p);
        }
      }
    }
  } finally {
    await context.close();
  }
  return pages;
}

let pages;
test.beforeAll(async ({ browser }, testInfo) => {
  pages = await crawl(browser, testInfo.project.use.baseURL);
});

test('the crawl reaches the home page and every nav link', async () => {
  const paths = pages.map((p) => p.path);
  expect(paths[0]).toBe('/');
  expect(pages[0].status).toBe(200);
  for (const { href } of navLinks) expect(paths).toContain(href);
  // Paths are unique.
  expect(new Set(paths).size).toBe(paths.length);
});

test('every page the site links to exists', async () => {
  const broken = pages.filter((p) => p.status !== 200).map((p) => `${p.path} (${p.status})`);
  expect(broken).toEqual([]);
});

test('the build writes plain HTML pages into the Go site package, which serves them', async ({ request }) => {
  const dist = new URL('../../internal/site/dist/app/', import.meta.url);
  for (const file of ['index.html', '404.html']) {
    expect(existsSync(new URL(file, dist)), `internal/site/dist/app/${file} exists`).toBe(true);
  }
  const index = readFileSync(new URL('index.html', dist), 'utf8');
  expect(index).toMatch(/^<!DOCTYPE html>/i);
  const resp = await request.get('/');
  expect(resp.status()).toBe(200);
  expect(resp.headers()['content-type']).toBe('text/html; charset=utf-8');
  expect(await resp.text()).toBe(index);

  const missing = await request.get('/no-such-page');
  expect(missing.status()).toBe(404);
  expect(await missing.text()).toBe(readFileSync(new URL('404.html', dist), 'utf8'));
});

test('the playmat addresses are not part of the website test server', async ({ request }) => {
  for (const path of ['/board', '/mcp', '/ws', '/api/card']) {
    expect((await request.get(path)).status(), path).toBe(503);
  }
});

test('every page shows the logo, the site navigation, the fan notice and the TCGdex credit', async ({ page }) => {
  for (const { path } of pages) {
    await page.goto(path);
    await expect(page.locator('header img[src="/logo.png"]'), `${path}: logo`).toBeVisible();

    const nav = page.getByRole('navigation', { name: 'Site' });
    await expect(nav, `${path}: site nav`).toBeAttached();
    for (const { name, href } of navLinks) {
      await expect(nav.getByRole('link', { name, exact: true }), `${path}: nav ${name}`).toHaveAttribute('href', href);
    }

    const notice = page.locator('footer .fan-notice');
    await expect(notice, `${path}: fan notice`).toBeVisible();
    await expect(notice).toContainText('unofficial fan project');
    for (const c of companies) await expect(notice, `${path}: notice names ${c}`).toContainText(c);

    const credit = page.locator('footer .credit');
    await expect(credit, `${path}: TCGdex credit`).toBeVisible();
    await expect(credit.getByRole('link', { name: 'TCGdex' })).toHaveAttribute('href', 'https://tcgdex.dev');
  }
});

test('no page scrolls sideways at 375px, and code blocks scroll or wrap in their own box', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== 'phone', 'the 375px check runs on the phone profile');
  for (const { path } of pages) {
    await page.goto(path);
    const m = await page.evaluate(() => {
      const se = document.scrollingElement;
      const main = document.querySelector('main');
      const blocks = [...document.querySelectorAll('pre, [data-copy]')].map((el) => {
        const cs = getComputedStyle(el);
        return {
          right: el.getBoundingClientRect().right,
          fits: el.scrollWidth <= el.clientWidth,
          scrolls: ['auto', 'scroll'].includes(cs.overflowX),
          wraps: ['pre-wrap', 'normal', 'pre-line', 'break-spaces'].includes(cs.whiteSpace),
        };
      });
      return {
        innerWidth,
        doc: se.scrollWidth,
        mainScroll: main.scrollWidth,
        mainClient: main.clientWidth,
        mainRight: main.getBoundingClientRect().right,
        blocks,
      };
    });
    expect(m.innerWidth, `${path}: viewport`).toBe(375);
    expect(m.doc, `${path}: document scrollWidth`).toBeLessThanOrEqual(m.innerWidth);
    expect(m.mainScroll, `${path}: main scrollWidth`).toBeLessThanOrEqual(m.mainClient);
    expect(m.mainRight, `${path}: main's right edge`).toBeLessThanOrEqual(m.innerWidth);
    for (const [i, b] of m.blocks.entries()) {
      expect(b.right, `${path}: block ${i} stays on screen`).toBeLessThanOrEqual(m.innerWidth);
      expect(b.fits || b.scrolls, `${path}: block ${i} fits or scrolls inside its box`).toBe(true);
      expect(b.scrolls || b.wraps, `${path}: block ${i} scrolls or wraps`).toBe(true);
    }
  }
});

test('a fresh visit to every page sets no cookie and asks no other host for anything', async ({ context, page, baseURL }) => {
  const host = new URL(baseURL).host;
  const requests = [];
  context.on('request', (r) => requests.push({ url: r.url(), type: r.resourceType() }));
  expect(await context.cookies()).toEqual([]);
  for (const { path } of pages) {
    await page.goto(path);
    await page.waitForLoadState('networkidle');
    expect(await context.cookies(), `${path}: cookies`).toEqual([]);
    expect(await page.evaluate(() => document.cookie), `${path}: document.cookie`).toBe('');
  }
  expect(requests.length).toBeGreaterThan(0);
  const elsewhere = requests.filter(({ url }) => {
    const u = new URL(url);
    if (u.protocol === 'data:' || u.protocol === 'blob:') return false;
    return u.host !== host;
  });
  expect(elsewhere, 'requests to other hosts').toEqual([]);
});

test('no page loads a web font or a script, stylesheet or image from another host', async ({ context, page, baseURL }) => {
  const origin = new URL(baseURL).origin;
  const fonts = [];
  context.on('request', (r) => {
    if (r.resourceType() === 'font') fonts.push(r.url());
  });
  for (const { path } of pages) {
    await page.goto(path);
    await page.waitForLoadState('networkidle');
    const found = await page.evaluate(() => {
      const fontFaces = [];
      for (const sheet of document.styleSheets) {
        let rules;
        try {
          rules = sheet.cssRules;
        } catch {
          fontFaces.push(`unreadable stylesheet ${sheet.href}`);
          continue;
        }
        for (const rule of rules) if (rule instanceof CSSFontFaceRule) fontFaces.push(rule.cssText);
      }
      const resources = [
        ...[...document.querySelectorAll('script[src]')].map((e) => e.src),
        ...[...document.querySelectorAll('link[href]:not([rel~="canonical"]):not([rel~="alternate"])')].map((e) => e.href),
        ...[...document.querySelectorAll('img[src], source[src], video[src], audio[src], iframe[src]')].map((e) => e.src),
        ...[...document.querySelectorAll('img[srcset], source[srcset]')].flatMap((e) =>
          e.srcset.split(',').map((s) => new URL(s.trim().split(/\s+/)[0], location.href).href),
        ),
      ];
      return { fontFaces, fontSet: document.fonts.size, resources };
    });
    expect(found.fontFaces, `${path}: @font-face rules`).toEqual([]);
    expect(found.fontSet, `${path}: document.fonts`).toBe(0);
    for (const r of found.resources) expect(new URL(r).origin, `${path}: ${r}`).toBe(origin);
  }
  expect(fonts, 'font requests').toEqual([]);
});

test('every copy button copies exactly its block text', async ({ context, page, baseURL }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'], { origin: new URL(baseURL).origin });
  let checked = 0;
  for (const { path } of pages) {
    await page.goto(path);
    const blocks = page.locator('[data-copy]');
    const buttons = page.locator('button.copy');
    const n = await blocks.count();
    await expect(buttons, `${path}: one copy button per copyable block`).toHaveCount(n);
    for (let i = 0; i < n; i++) {
      const block = blocks.nth(i);
      const want = await block.evaluate((el) => el.dataset.copyText ?? el.textContent);
      // Each button sits in its own block's box.
      const box = block.locator('xpath=..');
      const button = box.locator('button.copy');
      await expect(button).toHaveCount(1);
      await expect(buttons.nth(i)).toBeVisible();
      // Clear the clipboard so a stale value can't pass.
      await page.evaluate(() => navigator.clipboard.writeText(''));
      await button.click();
      await expect(button).toHaveText('Copied');
      expect(await page.evaluate(() => navigator.clipboard.readText()), `${path}: block ${i}`).toBe(want);
      checked++;
    }
  }
  expect(checked).toBeGreaterThan(0);
});

test('the home page copy button copies the playmat address exactly', async ({ context, page, baseURL }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'], { origin: new URL(baseURL).origin });
  await page.goto('/');
  const button = page.getByRole('button', { name: "Copy The playmat's address" });
  await expect(button).toBeVisible();
  await button.click();
  await expect(button).toHaveText('Copied');
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe('https://tcgreplay.demo.gs/mcp');
});
