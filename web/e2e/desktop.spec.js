// The desktop and tablet board's chrome on any screen size: the playback
// controls centre on the board area, not the window, and on screens wider
// than 1920px the header and timeline text grow and the timeline widens.
// The board knows when its area is wide enough for the landscape mat.
import { test, expect, isPhone, isWide } from './fixtures.js';

// offCentre is how far the playback bar's and the caption's horizontal
// centres sit from the board area's (main's).
async function offCentre(page) {
  return page.evaluate(() => {
    const mid = (el) => {
      const r = el.getBoundingClientRect();
      return r.left + r.width / 2;
    };
    const main = mid(document.querySelector('main'));
    const caption = document.querySelector('.overlay .caption');
    return {
      bar: Math.abs(mid(document.querySelector('.overlay .bar')) - main),
      caption: caption ? Math.abs(mid(caption) - main) : null,
    };
  });
}

test('during a replay the playback bar and caption centre on the board, with the timeline beside, below or hidden', async ({ page }, testInfo) => {
  test.skip(isPhone(testInfo), 'the phone timeline is a sheet');
  const aside = page.locator('aside');
  await aside.locator('.line').first().getByTitle('Play from here').click();
  const bar = page.locator('.overlay .bar');
  await expect(bar).toBeVisible();
  // Pause so the replay holds still while it is measured.
  await bar.getByRole('button', { name: 'Pause' }).click();
  await bar.getByRole('button', { name: 'Next move' }).click();
  await expect(page.locator('.overlay .caption')).toBeVisible();

  const check = async (where) => {
    const off = await offCentre(page);
    expect(off.bar, `bar, timeline ${where}`).toBeLessThanOrEqual(2);
    if (off.caption !== null) expect(off.caption, `caption, timeline ${where}`).toBeLessThanOrEqual(2);
  };
  await check('beside');

  await aside.getByRole('button', { name: 'Move timeline below the board' }).click();
  await expect(page.locator('.body')).toHaveClass(/\bbottom\b/);
  await check('below');

  await aside.getByRole('button', { name: 'Move timeline beside the board' }).click();
  await page.getByRole('button', { name: 'Menu' }).click();
  await page.getByRole('menuitem', { name: 'Hide timeline' }).click();
  await expect(aside).toHaveCount(0);
  await check('hidden');

  // The controls still work: step back, then back to the live position.
  await bar.getByRole('button', { name: 'Previous move' }).click();
  await bar.getByRole('button', { name: /^Live/ }).click();
  await expect(bar).toHaveCount(0);
});

// textSizes reads the header's and the timeline's font sizes, in pixels.
async function textSizes(page) {
  return page.evaluate(() => {
    const px = (sel) => parseFloat(getComputedStyle(document.querySelector(sel)).fontSize);
    return { header: px('header'), timeline: px('aside .timeline'), line: px('aside .line .text'), heading: px('aside h2') };
  });
}

test('header and timeline text keep today\'s size up to 1920px wide and grow above it', async ({ page }, testInfo) => {
  test.skip(isPhone(testInfo), 'the phone timeline is a sheet');
  await page.setViewportSize({ width: 1440, height: 900 });
  expect(await textSizes(page)).toEqual({ header: 13, timeline: 12, line: 12, heading: 13 });
  await page.setViewportSize({ width: 1920, height: 1080 });
  const hd = await textSizes(page);
  expect(hd).toEqual({ header: 13, timeline: 12, line: 12, heading: 13 });
  await page.setViewportSize({ width: 3840, height: 2160 });
  const uhd = await textSizes(page);
  for (const k of Object.keys(hd)) expect(uhd[k], k).toBeGreaterThanOrEqual(hd[k] * 1.5);
});

test('on a very wide screen the timeline beside the board grows past 480px', async ({ page }, testInfo) => {
  test.skip(isPhone(testInfo), 'the phone timeline is a sheet');
  await page.setViewportSize({ width: 3840, height: 2160 });
  const [aside, main] = [await page.locator('aside').boundingBox(), await page.locator('main').boundingBox()];
  expect(aside.x).toBeGreaterThanOrEqual(main.x + main.width - 1);
  expect(aside.width).toBeGreaterThan(480);
  // Up to about 2667px wide it keeps today's width: 18vw, at most 480px.
  await page.setViewportSize({ width: 1920, height: 1080 });
  expect(Math.round((await page.locator('aside').boundingBox()).width)).toBe(Math.round(1920 * 0.18));
});

const wide = (page) => page.locator('.app').getAttribute('data-wide');

test('the board is wide when its area beside the timeline is 1600px or more, and follows resizing live', async ({ page }, testInfo) => {
  test.skip(isPhone(testInfo), 'the phone board');
  await page.setViewportSize({ width: 2560, height: 1440 });
  await expect(page.locator('.app')).toHaveAttribute('data-wide', 'true');
  await expect(page.locator('.app')).toHaveClass(/\blandscape\b/);
  expect((await page.locator('main').boundingBox()).width).toBeGreaterThanOrEqual(1600);

  // No reload: the same page follows the window down and back up.
  await page.evaluate(() => (window.__samePage = true));
  await page.setViewportSize({ width: 1440, height: 900 });
  await expect(page.locator('.app')).toHaveAttribute('data-wide', 'false');
  await expect(page.locator('.app')).not.toHaveClass(/\blandscape\b/);
  await page.setViewportSize({ width: 2560, height: 1440 });
  await expect(page.locator('.app')).toHaveAttribute('data-wide', 'true');

  // At 1800 wide the board area beside the timeline is under 1600px...
  await page.setViewportSize({ width: 1800, height: 1000 });
  await expect(page.locator('.app')).toHaveAttribute('data-wide', 'false');
  // ...but moving the timeline below gives the board the full width,
  await page.locator('aside').getByRole('button', { name: 'Move timeline below the board' }).click();
  await expect(page.locator('.app')).toHaveAttribute('data-wide', 'true');
  await page.locator('aside').getByRole('button', { name: 'Move timeline beside the board' }).click();
  await expect(page.locator('.app')).toHaveAttribute('data-wide', 'false');
  // ...and so does hiding it.
  await page.getByRole('button', { name: 'Menu' }).click();
  await page.getByRole('menuitem', { name: 'Hide timeline' }).click();
  await expect(page.locator('.app')).toHaveAttribute('data-wide', 'true');
  await page.getByRole('button', { name: 'Menu' }).click();
  await page.getByRole('menuitem', { name: 'Show timeline' }).click();
  await expect(page.locator('.app')).toHaveAttribute('data-wide', 'false');
  expect(await page.evaluate(() => window.__samePage)).toBe(true);
});

test('the wide profile starts wide; the other profiles start narrow', async ({ page }, testInfo) => {
  expect(await wide(page)).toBe(isWide(testInfo) ? 'true' : 'false');
});
