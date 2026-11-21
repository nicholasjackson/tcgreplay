// Checks the board with visitor counting on, served by the second board
// test server (playwright.config.js), which adds Cloudflare Web
// Analytics' beacon to the board page the way hosted mode does with a
// token: the page carries exactly one beacon with single-page-app
// tracking off; moving around the board (card zooms, branch switches, the
// touch action sheet, which pushes a history entry) loads no second one,
// so a board session is one page view; the board plays normally with the
// beacon blocked; and nothing beyond the board's own settings is stored.
//
// No test here sends anything to Cloudflare. Every browser context routes
// every request for another host before the board loads: the beacon
// script to a stub (or an abort, for blocked analytics), and everything
// else, Cloudflare's reporting address included, to an abort. The second
// server has its own game, so the shared game the other specs use is
// untouched; these tests reset this server's game instead.
import { test as base, expect } from '@playwright/test';
import { act, isTouch, isPhone } from './fixtures.js';

// The analytics server and its token, as playwright.config.js starts it.
const server = 'http://127.0.0.1:7789';
const token = '0123456789abcdef0123456789abcdef';
const beacon = 'https://static.cloudflareinsights.com/beacon.min.js';
// The stub counts how many times the beacon script ran on the page.
const stub = 'window.__beacon = (window.__beacon || 0) + 1;';
// The board's own settings (web/src/lib/playback.svelte.js and
// web/src/lib/store.svelte.js): the only keys it may store.
const settings = ['replaySpeed', 'autoplay', 'panelAt', 'prizeReminders', 'actionKey'];

// test starts every test from this server's fixed position, with every
// other-host request routed first. blocked aborts the beacon instead of
// stubbing it. The page fixture records Cloudflare requests (cloudflare),
// page errors (errors) and browser dialogs, which fail the test.
const test = base.extend({
  blocked: [false, { option: true }],
  cloudflare: async ({}, use) => use([]),
  errors: async ({}, use) => use([]),
  page: async ({ page, request, blocked, cloudflare, errors }, use) => {
    const reset = await request.post('/test/reset');
    expect(reset.ok(), 'reset the analytics board server').toBeTruthy();
    const host = new URL(server).host;
    await page.context().route(
      (url) => url.host !== host,
      (route) => {
        const url = route.request().url();
        if (new URL(url).host.endsWith('cloudflareinsights.com')) cloudflare.push(url);
        if (url === beacon && !blocked) {
          return route.fulfill({ status: 200, contentType: 'text/javascript', body: stub });
        }
        // Anything else elsewhere (card images from their sources, say)
        // is not part of these tests.
        return route.abort('blockedbyclient');
      },
    );
    const dialogs = [];
    page.on('dialog', (d) => {
      dialogs.push(`${d.type()}: ${d.message()}`);
      d.dismiss().catch(() => {});
    });
    page.on('pageerror', (e) => errors.push(e.message));
    await page.goto('/');
    await expect(page.locator('header .dot.on')).toBeVisible();
    await use(page);
    expect(dialogs, 'no browser dialog opens').toEqual([]);
  },
});

test.use({ baseURL: server });

const region = (page, name) => page.getByRole('region', { name, exact: true });
const riolu = (page) => region(page, 'p1 bench 1').locator('.card[title="Riolu (p1-1)"]');
const active = (page) => region(page, 'p1 Active').locator('.card[title="Makuhita (p1-5)"]');
const menu = (page) => page.getByRole('button', { name: 'Menu' });

// zoom opens and closes the zoom of the benched Riolu: a click zooms a
// card in every profile.
async function zoom(page) {
  const box = await riolu(page).boundingBox();
  await page.mouse.click(box.x + box.width / 2, box.y + box.height / 2);
  await expect(page.locator('.zoom')).toBeVisible();
  await page.keyboard.press('Escape');
  await page.locator('.backdrop').first().click({ position: { x: 5, y: 5 } }).catch(() => {});
  await expect(page.locator('.zoom')).toHaveCount(0);
}

// eventIds is the board's log, as event ids, oldest first.
async function eventIds(page) {
  return page.evaluate(
    () =>
      new Promise((resolve) => {
        const ws = new WebSocket(`ws://${location.host}/ws`);
        ws.onmessage = (ev) => {
          const msg = JSON.parse(ev.data);
          if (msg.type !== 'state' || !msg.snapshot) return;
          ws.close();
          resolve((msg.snapshot.log || []).map((l) => l.id));
        };
      }),
  );
}

// branchPicker is the branch combobox: in the header on a mat, behind ☰
// on a phone (opened here).
async function branchPicker(page, testInfo) {
  if (isPhone(testInfo)) await menu(page).tap();
  return page.getByRole('combobox', { name: 'branch' });
}

// switchBranches rewinds onto a branch of its own, then picks main again
// in the branch picker, as a player switching lines does.
async function switchBranches(page, testInfo) {
  const ids = await eventIds(page);
  expect(ids.length).toBeGreaterThan(1);
  await act(page, 'rewind', { event: ids[0], name: 'other-line' });
  let picker = await branchPicker(page, testInfo);
  await expect(picker.locator('option')).toHaveCount(2);
  await expect(picker).toHaveValue('other-line');
  await picker.selectOption('main');
  if (isPhone(testInfo)) await page.keyboard.press('Escape');
  picker = await branchPicker(page, testInfo);
  await expect(picker).toHaveValue('main');
  if (isPhone(testInfo)) await page.keyboard.press('Escape');
}

// actionSheet opens a card's action sheet by touch, which pushes a history
// entry, and closes it with its ✕ and then with Back.
async function actionSheet(page) {
  const sheet = page.locator('.sheet');
  await active(page).tap();
  await expect(sheet).toBeVisible();
  await sheet.getByRole('button', { name: 'Close' }).first().tap();
  await expect(sheet).toHaveCount(0);
  await active(page).tap();
  await expect(sheet).toBeVisible();
  await page.goBack();
  await expect(sheet).toHaveCount(0);
}

test('the board page carries exactly one beacon, with single-page-app tracking off and only the token', async ({ page, cloudflare }) => {
  const tags = page.locator(`script[src="${beacon}"]`);
  await expect(tags).toHaveCount(1);
  expect(await page.locator('script[src*="cloudflareinsights"]').count()).toBe(1);
  expect(JSON.parse(await tags.getAttribute('data-cf-beacon'))).toEqual({ token, spa: false });
  expect(await tags.getAttribute('defer')).not.toBeNull();
  await expect.poll(() => page.evaluate(() => window.__beacon)).toBe(1);
  expect(cloudflare, 'Cloudflare requests').toEqual([beacon]);
});

test('moving around the board loads no second beacon, and stores only its own settings', async ({ page, context, cloudflare, errors }, testInfo) => {
  await expect.poll(() => page.evaluate(() => window.__beacon)).toBe(1);
  const url = page.url();

  await zoom(page);
  await switchBranches(page, testInfo);
  // The touch action sheet pushes a history entry each time it opens (the
  // desktop's popover does not).
  if (isTouch(testInfo)) await actionSheet(page);
  await zoom(page);

  await expect(page.locator('header .dot.on')).toBeVisible();
  expect(page.url(), 'still the same page').toBe(url);
  expect(await page.evaluate(() => window.__beacon), 'the beacon ran once').toBe(1);
  expect(await page.locator(`script[src="${beacon}"]`).count()).toBe(1);
  expect(cloudflare, 'Cloudflare requests: the one stubbed beacon load').toEqual([beacon]);

  const stored = await page.evaluate(() => ({
    local: Object.keys(localStorage),
    session: sessionStorage.length,
    cookie: document.cookie,
  }));
  for (const key of stored.local) expect(settings, `localStorage key ${key}`).toContain(key);
  expect(stored.session, 'sessionStorage entries').toBe(0);
  expect(stored.cookie, 'document.cookie').toBe('');
  expect(await context.cookies(), 'cookies').toEqual([]);
  expect(errors, 'page errors').toEqual([]);
});

test.describe('with the beacon blocked', () => {
  test.use({ blocked: true });

  test('the board shows the game, opens a card zoom and accepts a move, with no error', async ({ page, cloudflare, errors }) => {
    await expect(riolu(page)).toBeVisible();
    await expect(active(page)).toBeVisible();
    expect(await page.evaluate(() => window.__beacon), 'the beacon never ran').toBeUndefined();

    await zoom(page);

    await act(page, 'move', { iid: 'p1-2', to: 'bench:2' });
    await expect(region(page, 'p1 bench 3').locator('.card[title="Riolu (p1-2)"]')).toBeVisible();

    await expect(page.locator('.toast.error'), 'no error toast').toHaveCount(0);
    await expect(page.getByRole('alert'), 'no error shown').toHaveCount(0);
    expect(errors, 'page errors').toEqual([]);
    // The blocked beacon is the only Cloudflare address asked, and it never
    // left the browser.
    expect(cloudflare).toEqual([beacon]);
  });
});
