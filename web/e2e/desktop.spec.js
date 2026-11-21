// The desktop and tablet board's chrome on any screen size: the playback
// controls head the timeline beside or below the board, and with it hidden
// they and the caption centre on the board area, not the window; on screens wider
// than 1920px the header and timeline text grow and the timeline widens.
// The board knows when its area is wide enough for the landscape mat.
import { test, expect, isPhone, isWide } from './fixtures.js';

// offCentre is how far the floating playback bar's and the caption's
// horizontal centres sit from the board area's (main's), null for one not
// shown.
async function offCentre(page) {
  return page.evaluate(() => {
    const mid = (el) => {
      const r = el.getBoundingClientRect();
      return r.left + r.width / 2;
    };
    const main = mid(document.querySelector('main'));
    const caption = document.querySelector('.overlay .caption');
    const bar = document.querySelector('.overlay .bar');
    return {
      bar: bar ? Math.abs(mid(bar) - main) : null,
      caption: caption ? Math.abs(mid(caption) - main) : null,
    };
  });
}

test('during a replay the playback controls head the timeline beside or below the board, and with it hidden float centred on the board, the caption centred throughout', async ({ page }, testInfo) => {
  test.skip(isPhone(testInfo), 'the phone timeline is a sheet');
  const aside = page.locator('aside');
  // A tablet's lines show their Play from here when tapped (the Studio
  // design has no per-line buttons on a touch screen's timeline).
  if (testInfo.project.name === 'tablet') await aside.locator('.line .row').first().click();
  await aside.locator('.line').first().getByTitle('Play from here').click();
  const controls = aside.getByRole('toolbar', { name: 'Playback' });
  // Pause so the replay holds still while it is measured.
  await controls.getByRole('button', { name: 'Pause' }).click();
  await controls.getByRole('button', { name: 'Next step' }).click();
  await expect(page.locator('.overlay .caption')).toBeVisible();

  const check = async (where) => {
    const off = await offCentre(page);
    if (where === 'hidden') expect(off.bar, `bar, timeline ${where}`).toBeLessThanOrEqual(2);
    else expect(off.bar, `no floating bar, timeline ${where}`).toBeNull();
    if (off.caption !== null) expect(off.caption, `caption, timeline ${where}`).toBeLessThanOrEqual(2);
  };
  await expect(controls).toBeVisible();
  await check('beside');

  // Beside the board, the dock button is in ☰ (the Studio design).
  await page.getByRole('button', { name: 'Menu' }).click();
  await page.getByRole('menuitem', { name: 'Move timeline below the board' }).click();
  await expect(page.locator('.body')).toHaveClass(/\bbottom\b/);
  await expect(controls).toBeVisible();
  await check('below');

  await aside.getByRole('button', { name: 'Move timeline beside the board' }).click();
  await page.getByRole('button', { name: 'Menu' }).click();
  await page.getByRole('menuitem', { name: 'Hide timeline' }).click();
  await expect(aside).toHaveCount(0);
  const bar = page.locator('.overlay .bar');
  await expect(bar).toBeVisible();
  await check('hidden');

  // The controls still work: step back, then back to the live position.
  await bar.getByRole('button', { name: 'Previous step' }).click();
  await bar.getByRole('button', { name: 'Go live' }).click();
  await expect(bar).toHaveCount(0);
});

// textSizes reads the header's and the timeline's font sizes, in pixels.
async function textSizes(page) {
  return page.evaluate(() => {
    const px = (sel) => parseFloat(getComputedStyle(document.querySelector(sel)).fontSize);
    return { header: px('header'), timeline: px('aside .timeline'), line: px('aside .line .text'), heading: px('aside .timeline h3') };
  });
}

test('header and timeline text keep today\'s size up to 1920px wide and grow only a little above it', async ({ page }, testInfo) => {
  test.skip(isPhone(testInfo), 'the phone timeline is a sheet');
  await page.setViewportSize({ width: 1440, height: 900 });
  // The Studio design: lines at 13px, each half's heading at 12px.
  expect(await textSizes(page)).toEqual({ header: 13, timeline: 13, line: 13, heading: 12 });
  await page.setViewportSize({ width: 1920, height: 1080 });
  const hd = await textSizes(page);
  expect(hd).toEqual({ header: 13, timeline: 13, line: 13, heading: 12 });
  await page.setViewportSize({ width: 3840, height: 2160 });
  // At 3840px they are 1.15 times the size, no more (13px lines at about
  // 15px): readable on a 4K panel without the lines turning huge.
  const uhd = await textSizes(page);
  for (const k of Object.keys(hd)) {
    expect(uhd[k], k).toBeGreaterThan(hd[k]);
    expect(uhd[k], k).toBeLessThanOrEqual(hd[k] * 1.15 + 0.05);
    expect(uhd[k], k).toBeCloseTo(hd[k] * 1.15, 0);
  }
  // Halfway, at 2880px, halfway there.
  await page.setViewportSize({ width: 2880, height: 1620 });
  const mid = await textSizes(page);
  expect(mid.line).toBeCloseTo(13 * 1.075, 0);
});

test('on a wide screen the timeline beside the board grows with it, to at most 520px', async ({ page }, testInfo) => {
  test.skip(isPhone(testInfo), 'the phone timeline is a sheet');
  await page.setViewportSize({ width: 3840, height: 2160 });
  const [aside, main] = [await page.locator('aside').boundingBox(), await page.locator('main').boundingBox()];
  expect(aside.x).toBeGreaterThanOrEqual(main.x + main.width - 1);
  // Capped (timeline-width-max): the board keeps the rest of a 4K screen.
  expect(Math.round(aside.width)).toBe(520);
  // Up to about 2111px wide it keeps the design's timeline width, 380px
  // (362px on a touch screen: the design's 310px plus a Play button), and
  // grows with the screen (18vw) above it.
  await page.setViewportSize({ width: 1920, height: 1080 });
  const base = testInfo.project.name === 'tablet' ? 362 : 380;
  expect(Math.round((await page.locator('aside').boundingBox()).width)).toBe(base);
  await page.setViewportSize({ width: 2560, height: 1440 });
  expect(Math.round((await page.locator('aside').boundingBox()).width)).toBe(Math.round(2560 * 0.18));
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
  // Beside the board, the dock button is in ☰ (the Studio design).
  await page.getByRole('button', { name: 'Menu' }).click();
  await page.getByRole('menuitem', { name: 'Move timeline below the board' }).click();
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
