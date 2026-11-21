// The board's About panel: reached from ☰ at every layout, it shows the
// fan-project notice, the card-data credit and a help link, worded exactly
// as the shared notice file (internal/notice/notice.json) the website also
// uses. Nothing shows until it is asked for, it closes like the board's
// other views, and it keeps nothing in the browser.
import { readFileSync } from 'node:fs';
import { test, expect, isTouch, isPhone } from './fixtures.js';

// The shared wording, read straight from its file, not through the board's import.
const shared = JSON.parse(readFileSync(new URL('../../internal/notice/notice.json', import.meta.url), 'utf8'));
const companies = ['Nintendo', 'The Pokémon Company', 'Creatures', 'Game Freak'];
const settings = ['actionKey', 'autoplay', 'panelAt', 'prizeReminders', 'replaySpeed'];

const menu = (page) => page.getByRole('button', { name: 'Menu' });

// press taps on a touch profile and clicks with the mouse otherwise.
async function press(locator, testInfo) {
  if (isTouch(testInfo)) await locator.tap();
  else await locator.click();
}

// boxes records where the board's cards and controls are: the menu button,
// the player's hand and every card on the board. The pointer is moved to
// the corner first, so a card lifted under it is not counted as moved, and
// running transitions (the hand fanning out, a lifted card settling) are
// let finish.
async function boxes(page) {
  await page.mouse.move(1, 1);
  await page.evaluate(() =>
    Promise.all(
      document
        .getAnimations()
        .filter((a) => a.effect?.getComputedTiming().endTime !== Infinity)
        .map((a) => a.finished.catch(() => {})),
    ),
  );
  const menuBox = await menu(page).boundingBox();
  const hand = await page.locator('[aria-label="p1 hand"]').boundingBox();
  const cards = await page.locator('main .card').evaluateAll((cs) =>
    cs.map((c) => {
      const r = c.getBoundingClientRect();
      return [c.getAttribute('title'), Math.round(r.x), Math.round(r.y), Math.round(r.width), Math.round(r.height)];
    }),
  );
  return { menuBox, hand, cards };
}

async function checkAbout(page, testInfo) {
  // The board settles first: its card size follows the measured board area
  // a frame or two after a load or a resize, so two readings must agree.
  let before = await boxes(page);
  await expect
    .poll(async () => {
      const now = await boxes(page);
      const same = JSON.stringify(now) === JSON.stringify(before);
      before = now;
      return same;
    })
    .toBe(true);
  expect(before.cards.length).toBeGreaterThan(0);
  await expect(page.locator('.modal')).toHaveCount(0);

  // Open it from ☰.
  await press(menu(page), testInfo);
  await press(page.getByRole('menuitem', { name: 'About tcgreplay' }), testInfo);
  const panel = page.getByRole('dialog');
  await expect(panel).toBeVisible();
  await expect(panel.getByRole('heading', { name: 'About tcgreplay' })).toBeVisible();
  await expect(page.locator('.dropdown')).toHaveCount(0);

  // The shared wording and links.
  await expect(panel.locator('.fan-notice')).toHaveText(shared.notice);
  for (const c of companies) await expect(panel.locator('.fan-notice'), `names ${c}`).toContainText(c);
  await expect(panel.locator('.credit')).toHaveText(`${shared.credit.lead} ${shared.credit.sources.map((s) => s.name).join(' and ')}.`);
  for (const { name, url } of shared.credit.sources) {
    await expect(panel.getByRole('link', { name }), `credit links ${name}`).toHaveAttribute('href', url);
  }
  expect(shared.credit.sources.map((s) => s.name)).toEqual(['TCGdex', 'malie.io']);
  await expect(panel.getByRole('link', { name: 'Get help' })).toHaveAttribute('href', shared.help.url);
  expect(new URL(shared.help.url).host).toBe('github.com');
  expect(new URL(shared.help.url).pathname.endsWith('/issues')).toBe(true);

  // The text wraps inside the panel: nothing runs off the screen.
  const width = page.viewportSize().width;
  const panelBox = await panel.boundingBox();
  expect(panelBox.x).toBeGreaterThanOrEqual(0);
  expect(panelBox.x + panelBox.width).toBeLessThanOrEqual(width);
  expect(await panel.evaluate((p) => p.scrollWidth - p.clientWidth)).toBeLessThanOrEqual(0);

  // It closes with its close button (✕ on a touch screen, Close with a mouse) ...
  await press(panel.getByRole('button', { name: 'Close' }), testInfo);
  await expect(page.locator('.modal')).toHaveCount(0);
  // ... and, reopened, with Escape.
  await press(menu(page), testInfo);
  await press(page.getByRole('menuitem', { name: 'About tcgreplay' }), testInfo);
  await expect(panel).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.locator('.modal')).toHaveCount(0);

  // The board is as it was, and nothing new is kept in the browser.
  // (Polled, while a card lifted under the pointer settles back.)
  await expect.poll(() => boxes(page)).toEqual(before);
  const keys = await page.evaluate(() => Object.keys(localStorage));
  for (const k of keys) expect(settings, `localStorage key ${k}`).toContain(k);
  expect(await page.evaluate(() => sessionStorage.length)).toBe(0);
  expect(await page.evaluate(() => document.cookie)).toBe('');
}

test('About tcgreplay opens from ☰ with the shared notice, credit and help link, and closes leaving the board as it was', async ({ page }, testInfo) => {
  await checkAbout(page, testInfo);
  if (isPhone(testInfo)) {
    // The phone board still fits the screen with nothing scrolling.
    const fit = await page.evaluate(() => {
      const main = document.querySelector('main');
      return { page: document.documentElement.scrollHeight - innerHeight, board: main.scrollHeight - main.clientHeight, sideways: document.documentElement.scrollWidth - innerWidth };
    });
    expect(fit.page).toBeLessThanOrEqual(0);
    expect(fit.board).toBeLessThanOrEqual(0);
    expect(fit.sideways).toBeLessThanOrEqual(0);
  }
});

test('on an 820px tablet, About tcgreplay opens from ☰ and closes leaving the board as it was', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== 'tablet', 'the 820px tablet width runs on the tablet profile');
  await page.setViewportSize({ width: 820, height: 1180 });
  await page.reload();
  await expect(page.locator('header .dot.on')).toBeVisible();
  await expect(page.locator('.app')).not.toHaveClass(/\bphone\b/);
  await checkAbout(page, testInfo);
});
