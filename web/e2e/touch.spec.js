// Touch-friendly controls: fingertip-sized targets, no hover-only effects
// on a touch screen (hover kept for the mouse), the timeline's buttons
// shown without a hover, and cards a long touch cannot select.
import { test, expect, isTouch, pile, isPhone } from './fixtures.js';

const hand = (page) => page.getByRole('list', { name: 'p1 hand' });
const region = (page, name) => page.getByRole('region', { name, exact: true });
const titled = (scope, title) => scope.locator(`.card[title="${title}"]`);
const sheet = (page) => page.locator('.sheet');

// tooSmall lists every control and card on screen smaller than 44×44 CSS
// px: buttons, anything acting as one, selects, text inputs, a checkbox's
// label (its target) and cards.
function tooSmall(page) {
  return page.evaluate(() => {
    const sel = 'button, [role="button"], select, input:not([type="checkbox"]), label:has(> input[type="checkbox"]), .card';
    return [...document.querySelectorAll(sel)]
      .filter((el) => {
        const r = el.getBoundingClientRect();
        const style = getComputedStyle(el);
        return r.width > 0 && r.height > 0 && style.visibility !== 'hidden' && el.checkVisibility({ opacityProperty: true });
      })
      .map((el) => {
        const r = el.getBoundingClientRect();
        return { el: el.title || el.getAttribute('aria-label') || el.textContent.trim().slice(0, 30) || el.className, w: Math.round(r.width * 10) / 10, h: Math.round(r.height * 10) / 10 };
      })
      .filter((t) => t.w < 43.5 || t.h < 43.5);
  });
}

test('every control and card on the board is at least 44×44 on a touch screen', async ({ page }, testInfo) => {
  test.skip(!isTouch(testInfo), 'touch screens');
  expect(await tooSmall(page)).toEqual([]);
});

test('every control in the action sheet is at least 44×44 on a touch screen', async ({ page }, testInfo) => {
  test.skip(!isTouch(testInfo), 'touch screens');
  await titled(region(page, 'p1 Active'), 'Makuhita (p1-5)').tap();
  await expect(sheet(page)).toBeVisible();
  expect(await tooSmall(page)).toEqual([]);
});

test('every control in a pile view is at least 44×44 on a touch screen', async ({ page }, testInfo) => {
  test.skip(!isTouch(testInfo), 'touch screens');
  await pile(page, 'Discard').tap();
  await expect(page.locator('.modal')).toBeVisible();
  expect(await tooSmall(page)).toEqual([]);
});

// scaleOf is a card's current scale from its computed transform.
function scaleOf(card) {
  return card.evaluate((el) => {
    const t = getComputedStyle(el).transform;
    return t === 'none' ? 1 : new DOMMatrix(t).a;
  });
}

test('a touched card does not enlarge or reveal the quick damage buttons', async ({ page }, testInfo) => {
  test.skip(!isTouch(testInfo), 'touch screens');
  const riolu = titled(region(page, 'p1 bench 1'), 'Riolu (p1-1)');
  await riolu.tap();
  await page.keyboard.press('Escape');
  await expect(sheet(page)).toHaveCount(0);
  await page.waitForTimeout(400);
  expect(await scaleOf(riolu)).toBe(1);
  await expect(region(page, 'p1 bench 1').locator('.controls')).toBeHidden();
});

test('with a mouse, hover enlarges a card and shows the quick damage buttons', async ({ page }, testInfo) => {
  test.skip(isTouch(testInfo), 'mouse hover');
  const riolu = titled(region(page, 'p1 bench 1'), 'Riolu (p1-1)');
  await riolu.hover();
  await expect.poll(() => scaleOf(riolu)).toBeGreaterThan(1);
  await expect(region(page, 'p1 bench 1').locator('.controls')).toHaveCSS('opacity', '1');
  await expect(region(page, 'p1 bench 1').locator('.controls').getByRole('button', { name: '+10' })).toBeVisible();
});

test('the timeline shows its rewind and play buttons on a touch screen, and they work', async ({ page }, testInfo) => {
  test.skip(!isTouch(testInfo), 'touch screens');
  // A phone keeps the timeline behind ☰.
  if (isPhone(testInfo)) {
    await page.getByRole('button', { name: 'Menu' }).tap();
    await page.getByRole('menuitem', { name: 'Show timeline' }).tap();
  }
  const line = page.locator('.line').filter({ hasText: 'Move Riolu' }).first();
  const rewind = line.getByRole('button', { name: '↺' });
  const play = line.getByRole('button', { name: '▶' });
  await line.scrollIntoViewIfNeeded();
  await expect(rewind).toHaveCSS('opacity', '1');
  await expect(play).toBeVisible();
  // The turn heading's rewind and play show too.
  await expect(page.locator('h3 .rw, h4 .rw').first()).toHaveCSS('opacity', '1');

  const id = Number((await line.locator('.id').innerText()).slice(1));
  await rewind.tap();
  // The board rewinds to just after that line, on a branch of its own.
  await expect.poll(async () => Number((await page.locator('.line .id').last().innerText()).slice(1))).toBe(id);
  // The branch picker gains the new branch (behind ☰ on a phone, under
  // the timeline sheet).
  if (isPhone(testInfo)) {
    await page.getByRole('button', { name: 'Close the timeline' }).tap();
    await page.getByRole('button', { name: 'Menu' }).tap();
  }
  await expect(page.getByRole('combobox', { name: 'branch' }).locator('option')).toHaveCount(2);
  if (isPhone(testInfo)) {
    await page.getByRole('menuitem', { name: 'Show timeline' }).tap();
  }

  await page.locator('.line').filter({ hasText: 'Move Riolu' }).first().getByRole('button', { name: '▶' }).tap();
  await expect(page.locator('.line.now')).toHaveCount(1);
});

test('tapping a card in a pile or search view shows it large with its actions', async ({ page }, testInfo) => {
  test.skip(!isTouch(testInfo), 'touch screens');
  await pile(page, 'Discard').tap();
  await titled(page.locator('.modal'), 'Fighting Energy (p1-16)').tap();
  await expect(titled(sheet(page), 'Fighting Energy (p1-16)')).toBeVisible();
  // The mouse-only preview beside the view does not stick after a tap.
  await expect(page.locator('img.preview')).toHaveCount(0);
  await sheet(page).getByRole('button', { name: 'Close' }).tap();
  await page.locator('.modal').getByRole('button', { name: 'Close' }).tap();

  await pile(page, 'Deck').tap();
  await sheet(page).getByRole('button', { name: 'Search…', exact: true }).tap();
  const first = page.locator('.modal .row .card').first();
  const title = await first.getAttribute('title');
  await first.tap();
  await expect(titled(sheet(page), title)).toBeVisible();
});

test('tapping a card in the deck list shows it enlarged with its text', async ({ page }, testInfo) => {
  test.skip(!isTouch(testInfo), 'touch screens');
  await pile(page, 'Deck').tap();
  await sheet(page).getByRole('button', { name: 'Deck list…', exact: true }).tap();
  await page.locator('.modal .card[title^="Lucario ("]').tap();
  await expect(page.locator('.zoom')).toBeVisible();
  await expect(page.locator('.zoom pre')).toContainText('Lucario (tst-2)');
});

test('a long touch on a card selects nothing and opens no callout', async ({ page }) => {
  const card = titled(hand(page), 'Riolu (p1-2)');
  expect(await card.evaluate((el) => getComputedStyle(el).userSelect)).toBe('none');
  // Chromium drops -webkit-touch-callout (it is iOS only), so check the
  // built stylesheet carries it for cards.
  const css = await page.evaluate(async () => {
    const hrefs = [...document.querySelectorAll('link[rel="stylesheet"]')].map((l) => l.href);
    return (await Promise.all(hrefs.map((h) => fetch(h).then((r) => r.text())))).join('\n');
  });
  expect(css).toMatch(/\.card\.svelte-[a-z0-9]+\{[^}]*-webkit-touch-callout:none/);
});

test('on a touch screen the pile and deck views close with a round ✕, full screen on a phone', async ({ page }, testInfo) => {
  test.skip(!isTouch(testInfo), 'touch screens');
  const modal = page.locator('.modal');
  for (const open of [() => pile(page, 'Discard').tap(), () => openSearch(page)]) {
    await open();
    await expect(modal).toBeVisible();
    // One close, the round ✕: no Close or Done text button.
    await expect(modal.getByRole('button', { name: /^(Close|Done)$/ })).toHaveCount(1);
    await expect(modal.getByRole('button', { name: 'Close' })).toHaveText('✕');
    if (isPhone(testInfo)) expect(await modal.boundingBox()).toEqual({ x: 0, y: 0, width: 390, height: 844 });
    await modal.getByRole('button', { name: 'Close' }).tap();
    await expect(modal).toHaveCount(0);
  }
});

test('with a mouse the pile view keeps its Close button', async ({ page }, testInfo) => {
  test.skip(isTouch(testInfo), 'mouse');
  await pile(page, 'Discard').click();
  await expect(page.locator('.modal').getByRole('button', { name: 'Close' })).toHaveText('Close');
});

// openSearch opens the deck search from the deck's actions.
async function openSearch(page) {
  await pile(page, 'Deck').tap();
  await sheet(page).getByRole('button', { name: 'Search…', exact: true }).tap();
}
