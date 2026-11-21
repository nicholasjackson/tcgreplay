// The board's header: the mark, the word Live beside the connection dot,
// the Play/Review switch following the game's mode, and on a tablet or
// touch PC a header button that hides and shows the timeline, which the
// mouse layouts and the phone do not have.
import { test, expect, isTouch, isPhone } from './fixtures.js';
import { readFileSync } from 'node:fs';

// mark is the TCG Replay mark the header shows (the build may inline it
// as a data: URI rather than serve it as a file).
const mark = readFileSync(new URL('../../internal/design/mark.svg', import.meta.url), 'utf8');

const header = (page) => page.locator('header');
const timelineButton = (page) => header(page).getByRole('button', { name: /^(Hide|Show) timeline$/ });

// press taps (touch) or clicks (mouse) a locator.
async function press(locator, testInfo) {
  if (isTouch(testInfo)) await locator.tap();
  else await locator.click();
}

test('on a tablet the header button hides and shows the timeline', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== 'tablet', 'the header timeline button is for a tablet or touch PC');
  const aside = page.locator('aside');
  await expect(aside).toBeVisible();
  const hide = header(page).getByRole('button', { name: 'Hide timeline', exact: true });
  await expect(hide).toHaveAttribute('aria-pressed', 'true');

  await hide.tap();
  await expect(aside).toHaveCount(0);
  const show = header(page).getByRole('button', { name: 'Show timeline', exact: true });
  await expect(show).toHaveAttribute('aria-pressed', 'false');

  await show.tap();
  await expect(aside).toBeVisible();
  await expect(hide).toBeVisible();
});

test('on a tablet the header timeline button is at least a fingertip wide and tall', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== 'tablet', 'the header timeline button is for a tablet or touch PC');
  const box = await timelineButton(page).boundingBox();
  expect(box.width).toBeGreaterThanOrEqual(44);
  expect(box.height).toBeGreaterThanOrEqual(44);
});

test('the desktop and the phone have no header timeline button', async ({ page }, testInfo) => {
  test.skip(!['desktop', 'phone'].includes(testInfo.project.name), 'desktop and phone only');
  await expect(header(page).getByRole('button', { name: 'Menu' })).toBeVisible();
  await expect(timelineButton(page)).toHaveCount(0);
});

test('the header shows the mark and the word Live beside the connected dot', async ({ page }, testInfo) => {
  test.skip(isPhone(testInfo), 'a phone has no header bar, only the dot and ☰');
  const img = header(page).locator('.brand img');
  await expect(img).toBeVisible();
  expect(await img.evaluate((el) => el.complete && el.naturalWidth > 0), 'the mark loads').toBe(true);
  const shown = await img.evaluate(async (el) => (await fetch(el.src)).text());
  expect(shown, 'the image is internal/design/mark.svg').toBe(mark);

  const conn = header(page).locator('.conn');
  await expect(conn).toHaveText('Live');
  await expect(conn.locator('.dot.on')).toBeVisible();
});

test('the Play/Review switch shows the mode and follows a change of mode', async ({ page }, testInfo) => {
  test.skip(isPhone(testInfo), "a phone's mode is a checkbox in ☰ (phone.spec.js)");
  const mode = header(page).getByRole('radiogroup', { name: 'Mode' });
  const playRadio = mode.getByRole('radio', { name: 'Play mode' });
  const reviewRadio = mode.getByRole('radio', { name: 'Review mode' });
  // The board test server's game is in review mode.
  await expect(reviewRadio).toBeChecked();
  await expect(playRadio).not.toBeChecked();
  await expect(header(page).getByText('Reveal all')).toBeVisible();

  await press(mode.locator('label', { hasText: 'Play' }), testInfo);
  await expect(playRadio).toBeChecked();
  await expect(reviewRadio).not.toBeChecked();
  await expect(header(page).getByText('Reveal all')).toHaveCount(0);

  await press(mode.locator('label', { hasText: 'Review' }), testInfo);
  await expect(reviewRadio).toBeChecked();
  await expect(playRadio).not.toBeChecked();
  await expect(header(page).getByText('Reveal all')).toBeVisible();
  // The board is still there and connected.
  await expect(page.getByRole('region', { name: 'p1 side' })).toBeVisible();
  await expect(header(page).locator('.conn .dot.on')).toBeVisible();
});
