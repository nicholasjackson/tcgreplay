// The main view never scrolls vertically, in any layout: the board takes
// exactly what is left under the header and any banners (Reveal all, game
// over), its cards sizing to that room, and the hand stays on screen.
import { test, expect, act, isPhone, isTouch } from './fixtures.js';

// sizes are the window sizes each profile checks, with the timeline where
// it starts; the desktop also checks the narrow board (timeline docked
// below) and a 1920 screen.
const sizes = {
  phone: [{ width: 390, height: 844 }, { width: 375, height: 667 }],
  tablet: [{ width: 1024, height: 768 }, { width: 1180, height: 820 }],
  desktop: [{ width: 1440, height: 900 }, { width: 1920, height: 1080 }, { width: 1440, height: 900, docked: true }],
  wide: [{ width: 2560, height: 1440 }, { width: 1920, height: 1080, hidden: true }],
};

// overflow measures how far the page and the board area run past the
// window, and where the hand's foot sits against the window's.
function overflow(page) {
  return page.evaluate(() => {
    const main = document.querySelector('main');
    const hand = document.querySelector('[aria-label="p1 hand"]').getBoundingClientRect();
    return {
      page: document.scrollingElement.scrollHeight - innerHeight,
      main: main.scrollHeight - main.clientHeight,
      hand: Math.ceil(hand.bottom) - innerHeight,
    };
  });
}

async function expectNoScroll(page, at) {
  // The cards follow the board area's measured height, a frame after any
  // change to it.
  await expect.poll(async () => (await overflow(page)).page, `page scrolls at ${at}`).toBeLessThanOrEqual(0);
  await expect.poll(async () => (await overflow(page)).main, `board scrolls at ${at}`).toBeLessThanOrEqual(1);
  await expect.poll(async () => (await overflow(page)).hand, `hand off screen at ${at}`).toBeLessThanOrEqual(0);
}

// setReveal turns Reveal all on or off: in the header, or in ☰ on a phone.
async function setReveal(page, on, testInfo) {
  const tap = (l) => (isTouch(testInfo) ? l.tap() : l.click());
  if (isPhone(testInfo)) await tap(page.getByRole('button', { name: 'Menu' }));
  const box = page.getByLabel('Reveal all');
  if ((await box.isChecked()) !== on) await tap(box);
  if (isPhone(testInfo)) await page.keyboard.press('Escape');
  await expect(page.locator('.reveal-banner')).toHaveCount(on ? 1 : 0);
}

async function layout(page, size) {
  await page.evaluate((docked) => localStorage.setItem('panelAt', docked ? 'bottom' : 'side'), !!size.docked);
  await page.setViewportSize({ width: size.width, height: size.height });
  await page.reload();
  await expect(page.locator('header .dot.on')).toBeVisible();
  if (size.hidden) {
    await page.getByRole('button', { name: 'Menu' }).click();
    await page.getByRole('menuitem', { name: 'Hide timeline' }).click();
    await expect(page.locator('aside')).toHaveCount(0);
  }
}

test('the window never scrolls, with or without the Reveal all and game over banners', async ({ page }, testInfo) => {
  for (const size of sizes[testInfo.project.name]) {
    const at = `${size.width}×${size.height}${size.docked ? ', timeline below' : ''}${size.hidden ? ', timeline hidden' : ''}`;
    await layout(page, size);
    await expectNoScroll(page, at);
    await setReveal(page, true, testInfo);
    await expectNoScroll(page, `${at}, Reveal all`);
    // The banner takes its own row: it covers none of the board.
    const [banner, main] = [await page.locator('.reveal-banner').boundingBox(), await page.locator('main').boundingBox()];
    expect(main.y, `banner over the board at ${at}`).toBeGreaterThanOrEqual(banner.y + banner.height - 1);
    await setReveal(page, false, testInfo);
    await expectNoScroll(page, `${at}, Reveal all off`);
  }
});

test('the game over banner, alone and with Reveal all, leaves the window unscrolled', async ({ page }, testInfo) => {
  // Taking the last prize ends the game.
  for (let i = 0; i < 6; i++) await act(page, 'take_prize', { seat: 'p1' });
  for (const size of sizes[testInfo.project.name]) {
    const at = `${size.width}×${size.height}${size.docked ? ', timeline below' : ''}${size.hidden ? ', timeline hidden' : ''}`;
    await layout(page, size);
    await expect(page.locator('.over-banner')).toBeVisible();
    await expectNoScroll(page, `${at}, game over`);
    await setReveal(page, true, testInfo);
    await expectNoScroll(page, `${at}, game over and Reveal all`);
    const [banner, main] = [await page.locator('.reveal-banner').boundingBox(), await page.locator('main').boundingBox()];
    expect(main.y, `banners over the board at ${at}`).toBeGreaterThanOrEqual(banner.y + banner.height - 1);
    await setReveal(page, false, testInfo);
  }
});
