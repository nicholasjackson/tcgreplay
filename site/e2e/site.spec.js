// Checks every page of the built website, served by the Go site handler:
// the shared layout (logo, navigation, and the footer's fan notice and
// card-data credit from the shared notice file, with its Privacy, Terms,
// help and source links), no sideways scrolling on a 375px phone, no
// cookies, nothing fetched from any other host (no web fonts, analytics or
// third-party resources) in a run without the visitor-counting token, and
// copy buttons that copy exactly their block's text. The pages with
// counting on are checked by analytics.spec.js.
//
// The pages are found by crawling same-origin links from /. Every page the
// site links to must exist: a link to a missing page fails the suite.
import { readFileSync, existsSync, readdirSync } from 'node:fs';
import { test, expect } from '@playwright/test';
import { site } from '../src/lib/facts.ts';
import { crawl } from './crawl.js';

const companies = ['Nintendo', 'The Pokémon Company', 'Creatures', 'Game Freak'];

// The shared wording, read straight from its file (not through the site's
// loader), so a footer that drifts from it fails.
const shared = JSON.parse(readFileSync(new URL('../../internal/notice/notice.json', import.meta.url), 'utf8'));

const navLinks = [
  { name: 'Home', href: '/' },
  { name: 'Get started', href: '/docs/connect' },
  { name: 'Guides', href: '/docs/workflows' },
  { name: 'Example', href: '/docs/example' },
  { name: 'Limits', href: '/docs/limits' },
  { name: 'Privacy', href: '/privacy' },
];

let pages;
test.beforeAll(async ({ browser }, testInfo) => {
  pages = await crawl(browser, testInfo.project.use.baseURL);
});

test('the crawl reaches the home page, every nav link and the terms', async () => {
  const paths = pages.map((p) => p.path);
  expect(paths[0]).toBe('/');
  expect(pages[0].status).toBe(200);
  for (const { href } of navLinks) expect(paths).toContain(href);
  expect(paths).toContain('/terms');
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

test('every page shows the logo, the site navigation, the shared fan notice and credit, and the footer links', async ({ page }) => {
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
    await expect(notice, `${path}: notice is the shared wording`).toHaveText(shared.notice);
    await expect(notice).toContainText('unofficial fan project');
    for (const c of companies) await expect(notice, `${path}: notice names ${c}`).toContainText(c);

    const credit = page.locator('footer .credit');
    await expect(credit, `${path}: card-data credit`).toBeVisible();
    await expect(credit, `${path}: credit is the shared wording`).toHaveText(
      `${shared.credit.lead} ${shared.credit.sources.map((s) => s.name).join(' and ')}.`,
    );
    for (const { name, url } of shared.credit.sources) {
      await expect(credit.getByRole('link', { name }), `${path}: credit links ${name}`).toHaveAttribute('href', url);
    }

    const links = page.locator('footer').getByRole('navigation', { name: 'Footer' });
    await expect(links, `${path}: footer links`).toBeVisible();
    await expect(links.getByRole('link', { name: 'Privacy', exact: true }), `${path}: footer Privacy`).toHaveAttribute('href', '/privacy');
    await expect(links.getByRole('link', { name: 'Terms', exact: true }), `${path}: footer Terms`).toHaveAttribute('href', '/terms');
    const help = links.getByRole('link', { name: /Get help/ });
    await expect(help, `${path}: footer help`).toHaveAttribute('href', shared.help.url);
    await expect(help).toHaveText('Get help');
    await expect(links.getByRole('link', { name: 'Source on GitHub' }), `${path}: footer source`).toHaveAttribute('href', site.repoURL);
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

test('without the analytics token, a fresh visit to every page sets no cookie, carries no beacon and asks no other host for anything', async ({ context, page, baseURL }) => {
  const host = new URL(baseURL).host;
  const requests = [];
  context.on('request', (r) => requests.push({ url: r.url(), type: r.resourceType() }));
  expect(await context.cookies()).toEqual([]);
  for (const { path } of pages) {
    await page.goto(path);
    await page.waitForLoadState('networkidle');
    expect(await context.cookies(), `${path}: cookies`).toEqual([]);
    expect(await page.evaluate(() => document.cookie), `${path}: document.cookie`).toBe('');
    expect(await page.content(), `${path}: no visitor-counting beacon`).not.toContain('cloudflareinsights');
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
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(site.mcpURL);
});

// The playmat moved from tcgreplay.demo.gs to its own domain. The old
// address may appear only inside the connecting page's marked move note
// (an element with data-moved-note), and the playmat has no www name.
test('no page or the README names the old address outside the move note, or a www address', () => {
  const dist = new URL('../../internal/site/dist/app/', import.meta.url);
  const files = readdirSync(dist, { recursive: true })
    .filter((f) => String(f).endsWith('.html'))
    .map((f) => ({ name: `internal/site/dist/app/${f}`, url: new URL(String(f), dist) }));
  expect(files.length).toBeGreaterThan(5);
  files.push({ name: 'README.md', url: new URL('../../README.md', import.meta.url) });
  const note = /<aside[^>]*data-moved-note[^>]*>[\s\S]*?<\/aside>/g;
  const stray = [];
  const www = [];
  let notes = 0;
  for (const { name, url } of files) {
    const text = readFileSync(url, 'utf8');
    notes += (text.match(note) || []).length;
    if (text.replace(note, '').includes('demo.gs')) stray.push(name);
    if (text.includes('www.tcgreplay.app')) www.push(name);
  }
  expect(stray, 'files naming demo.gs outside the move note').toEqual([]);
  expect(www, 'files naming www.tcgreplay.app').toEqual([]);
  expect(notes, 'move notes across the site').toBe(1);
});
