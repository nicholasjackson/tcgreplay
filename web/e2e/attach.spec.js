// Attached cards on the board: a Basic Energy is a dot only; a Tool or a
// Special Energy is tucked behind its Pokémon, a strip of it showing above
// the card (at most two), the Bench row kept level; the dots count the
// Energy each card provides; every attached card is listed last in the
// Pokémon's menu, each row opening the card's view with its actions.
import { test, expect, act, drag, isTouch, isPhone, pile, timelineLines, tucked, attachedTitles } from './fixtures.js';

const region = (page, name) => page.getByRole('region', { name, exact: true });
const sheet = (page) => page.locator('.sheet');
const box = async (l) => {
  const r = await l.boundingBox();
  return { ...r, right: r.x + r.width, bottom: r.y + r.height };
};

// attach moves cards onto p1's Pokémon ([iid, to, as] each) and reloads,
// so the board shows the position live (act() moves arrive as another
// client's and are held).
async function attach(page, moves) {
  for (const [iid, to, as] of moves) await act(page, 'move', { iid, to, as });
  await page.reload();
  await expect(page.locator('header .dot.on')).toBeVisible();
}

// openMenu taps (touch) or right-clicks (mouse) a Pokémon in a region.
async function openMenu(page, name, testInfo) {
  const card = region(page, name).locator('.main > .card');
  if (isTouch(testInfo)) await card.tap();
  else await card.click({ button: 'right' });
  await expect(sheet(page)).toBeVisible();
}

async function press(target, testInfo) {
  if (isTouch(testInfo)) await target.tap();
  else await target.click();
}

test('a Basic Energy draws no card on the board, only its dot', async ({ page }) => {
  const active = region(page, 'p1 Active');
  await expect(active.getByLabel('1 Energy')).toBeVisible();
  await expect(active.locator('.card[title="Fighting Energy (p1-15)"]')).toHaveCount(0);
  await expect(active.locator('.tuck')).toHaveCount(0);
});

test('a Tool and a Special Energy are tucked behind the Pokémon, their strips above its card and within its width', async ({ page }, testInfo) => {
  test.skip(isPhone(testInfo), 'the phone draws no tucked cards');
  await attach(page, [['p1-9', 'bench:0', 'tool'], ['p1-58', 'bench:0', 'energy']]);
  const slot = region(page, 'p1 bench 1');
  await expect(slot.locator('.tuck')).toHaveCount(2);
  const card = await box(slot.locator('.main > .card'));
  const band = await box(tucked(page, 'p1 bench 1', 'Band (p1-9)'));
  const dce = await box(tucked(page, 'p1 bench 1', 'Double Colorless Energy (p1-58)'));
  // The Tool's strip ends at the card's top edge, 0.42 of its width tall;
  // the Special Energy's band a further 0.2 above it.
  expect(band.bottom).toBeCloseTo(card.y, 0);
  expect(band.height).toBeCloseTo(card.width * 0.42, 0);
  expect(dce.bottom).toBeCloseTo(band.y, 0);
  expect(dce.height).toBeCloseTo(card.width * 0.2, 0);
  for (const s of [band, dce]) {
    expect(s.x).toBeGreaterThanOrEqual(card.x - 0.5);
    expect(s.right).toBeLessThanOrEqual(card.right + 0.5);
  }
  // The tucked cards themselves are behind the Pokémon: a point in the
  // card's middle is the Pokémon's.
  const hit = await page.evaluate(({ x, y }) => document.elementFromPoint(x, y)?.closest('.card')?.title, { x: card.x + card.width / 2, y: card.y + card.height / 2 });
  expect(hit).toBe('Riolu (p1-1)');
});

test('at most two cards are tucked; the menu lists them all', async ({ page }, testInfo) => {
  test.skip(isPhone(testInfo), 'the phone draws no tucked cards');
  await attach(page, [['p1-9', 'bench:0', 'tool'], ['p1-10', 'bench:0', 'tool'], ['p1-58', 'bench:0', 'energy']]);
  await expect(region(page, 'p1 bench 1').locator('.tuck')).toHaveCount(2);
  await openMenu(page, 'p1 bench 1', testInfo);
  await expect(sheet(page).locator('section[aria-label="Attached"]').getByRole('button')).toHaveCount(3);
});

test('the Pokémon on one Bench row stay level when one has a Tool', async ({ page }, testInfo) => {
  test.skip(isPhone(testInfo), 'the phone draws no tucked cards');
  const tops = async () => {
    const out = [];
    for (const name of ['p1 bench 1', 'p1 bench 2']) out.push((await box(region(page, name).locator('.main > .card'))).y);
    out.push((await box(region(page, 'p1 bench 3'))).y);
    return out;
  };
  await attach(page, [['p1-9', 'bench:0', 'tool']]);
  await expect(region(page, 'p1 bench 1').locator('.tuck')).toHaveCount(1);
  const [withTool, without, empty] = await tops();
  expect(without).toBeCloseTo(withTool, 0);
  expect(empty).toBeCloseTo(withTool, 0);
  // Room is left above the row: the strip is inside the Bench zone.
  const zone = await box(page.locator('[aria-label="p1 side"] .bench'));
  const strip = await box(tucked(page, 'p1 bench 1', 'Band (p1-9)'));
  expect(strip.y).toBeGreaterThanOrEqual(zone.y - 0.5);
});

test('the Energy dots count what each card provides: a Double Colorless Energy gives two, ringed', async ({ page }, testInfo) => {
  // The Active has a Fighting Energy; a Double Colorless Energy joins it.
  await attach(page, [['p1-58', 'active', 'energy'], ['p1-18', 'bench:1', 'energy']]);
  const active = region(page, 'p1 Active');
  await expect(active.getByLabel('3 Energy')).toBeVisible();
  await expect(active.locator('.dots i.special')).toHaveCount(2);
  await expect(active.locator('.dots i:not(.special)')).toHaveCount(1);
  // A Special Energy's dot is ringed; a Basic one's is not.
  expect(await active.locator('.dots i.special').first().evaluate((i) => getComputedStyle(i).outlineStyle)).toBe('solid');
  expect(await active.locator('.dots i:not(.special)').first().evaluate((i) => getComputedStyle(i).outlineStyle)).toBe('none');
  // The panel's count says the same.
  if (!isPhone(testInfo)) await expect(active.locator('.count')).toHaveText('3 Energy');
  // A benched Pokémon with one Basic Energy: one plain dot.
  await expect(region(page, 'p1 bench 2').getByLabel('1 Energy')).toBeVisible();
  await expect(region(page, 'p1 bench 2').locator('.dots i.special')).toHaveCount(0);
});

test('the Active panel names its Tool', async ({ page }, testInfo) => {
  test.skip(isPhone(testInfo), 'the phone panel has no Tool tag');
  await attach(page, [['p1-9', 'active', 'tool']]);
  await expect(region(page, 'p1 Active').locator('.readout .tool')).toHaveText('Band');
  await expect(region(page, 'p1 Active').locator('.tuck')).toHaveCount(1);
});

test('with a mouse, a strip opens its card\'s view with its actions, and a right-click its menu', async ({ page }, testInfo) => {
  test.skip(isTouch(testInfo), 'the mouse');
  await attach(page, [['p1-9', 'bench:0', 'tool']]);
  await tucked(page, 'p1 bench 1', 'Band (p1-9)').click();
  const view = page.getByRole('dialog', { name: 'Band' });
  await expect(view).toHaveClass(/\bzoom\b/);
  await expect(view).toContainText('Tool on Bench 1 (Riolu)');
  await expect(view.getByRole('group', { name: 'Attached' }).getByRole('button')).toHaveText(['Move to', 'To hand', 'Discard']);
  await view.getByRole('button', { name: 'Close' }).click();
  await expect(view).toHaveCount(0);
  await tucked(page, 'p1 bench 1', 'Band (p1-9)').click({ button: 'right' });
  await expect(page.getByRole('dialog', { name: 'Band' })).toHaveClass(/\bsheet\b/);
  await expect(sheet(page).locator('section[aria-label="Move"]').getByRole('button')).toHaveText([/Move to/, 'To hand', 'Discard']);
});

test('with a mouse, a tucked card drags off the board', async ({ page }, testInfo) => {
  test.skip(isTouch(testInfo), 'the mouse');
  await attach(page, [['p1-9', 'bench:0', 'tool']]);
  const before = await timelineLines(page);
  await drag(page, tucked(page, 'p1 bench 1', 'Band (p1-9)'), pile(page, 'Discard'), testInfo);
  await expect.poll(async () => (await timelineLines(page)).slice(before.length)).toEqual(['p1 moved p1-9 Band p1:bench:0 → p1:discard']);
});

test('on a touch screen, a tap on a strip opens the Pokémon\'s sheet', async ({ page }, testInfo) => {
  test.skip(!isTouch(testInfo) || isPhone(testInfo), 'touch screens that draw tucked cards');
  await attach(page, [['p1-9', 'bench:0', 'tool']]);
  const strip = await box(tucked(page, 'p1 bench 1', 'Band (p1-9)'));
  await page.touchscreen.tap(strip.x + strip.width / 2, strip.y + strip.height / 2);
  await expect(page.getByRole('dialog', { name: 'Riolu' })).toHaveClass(/\bsheet\b/);
  await expect(sheet(page).locator('section[aria-label="Attached"]').getByRole('button')).toHaveText([/^Band\s*Tool/]);
});

test('the Attached rows show each card\'s picture and open its view, whose actions move it', async ({ page }, testInfo) => {
  await attach(page, [['p1-9', 'active', 'tool'], ['p1-58', 'active', 'energy']]);
  await openMenu(page, 'p1 Active', testInfo);
  const groups = await sheet(page).locator('section[role="group"]').evaluateAll((ss) => ss.map((s) => s.getAttribute('aria-label')));
  expect(groups.at(-1)).toBe('Attached');
  const rows = sheet(page).locator('section[aria-label="Attached"]').getByRole('button');
  await expect(rows).toHaveText([/^Band\s*Tool/, /^Fighting Energy\s*Energy/, /^Double Colorless Energy\s*Special Energy/]);
  await expect(rows.locator('.thumb')).toHaveCount(3);
  for (const r of await rows.all()) expect((await r.boundingBox()).height).toBeGreaterThanOrEqual(isTouch(testInfo) ? 44 : 40);
  await press(rows.nth(2), testInfo);
  await expect(sheet(page)).toHaveCount(0);
  const view = page.getByRole('dialog', { name: 'Double Colorless Energy' });
  await expect(view).toHaveClass(/\bzoom\b/);
  await expect(view).toContainText('Special Energy on Active (Makuhita)');
  await press(view.getByRole('button', { name: 'To hand' }), testInfo);
  await expect(view).toHaveCount(0);
  await expect.poll(() => attachedTitles(page, 'p1 Active')).toEqual(['Band (p1-9)', 'Fighting Energy (p1-15)']);
});
