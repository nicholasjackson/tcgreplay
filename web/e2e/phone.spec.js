// The compact phone board: no sideways page scroll, counters for each
// player's piles and prizes, a bench row reaching all 8 slots, the header's
// controls behind ☰, and a notice when the phone is held sideways. Tablets
// and desktops keep the mat.
import { test, expect, act, isPhone, pile } from './fixtures.js';

const region = (page, name) => page.getByRole('region', { name, exact: true });
const counters = (page, seat) => page.getByRole('group', { name: `${seat} counters` });
const chip = (page, seat, name) => counters(page, seat).getByRole('button', { name: new RegExp(`^${name} `) });
const menu = (page) => page.getByRole('button', { name: 'Menu' });

test('a phone in portrait has no sideways scroll, and its cards are large enough to read and tap', async ({ page }, testInfo) => {
  test.skip(!isPhone(testInfo), 'the phone board');
  // Neither the page nor the board inside it scrolls sideways.
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  expect(await page.locator('main').evaluate((m) => m.scrollWidth - m.clientWidth)).toBe(0);
  for (const title of ['Makuhita (p1-5)', 'Riolu (p1-1)', 'Makuhita (p2-5)', 'Riolu (p1-2)', 'Band (p1-9)']) {
    const box = await page.locator(`.card[title="${title}"]`).first().boundingBox();
    expect(box.width, title).toBeGreaterThanOrEqual(60);
  }
});

test('each player has counters for the deck, discard, Lost Zone and prizes', async ({ page }, testInfo) => {
  test.skip(!isPhone(testInfo), 'the phone board');
  for (const seat of ['p1', 'p2']) {
    await expect(chip(page, seat, 'Deck')).toHaveText(seat === 'p1' ? 'Deck 40' : 'Deck 46');
    await expect(chip(page, seat, 'Discard')).toHaveText(seat === 'p1' ? 'Discard 1' : 'Discard 0');
    await expect(chip(page, seat, 'Lost')).toHaveText('Lost 0');
    await expect(chip(page, seat, 'Prizes')).toHaveText('Prizes 6');
  }
  // The opponent's hand is a count in their counters.
  await expect(counters(page, 'p2').getByLabel('Hand 6')).toBeVisible();
  await expect(page.getByRole('list', { name: 'p2 hand' })).toHaveCount(0);
});

test('a counter opens its pile, the deck\'s actions or the prizes', async ({ page }, testInfo) => {
  test.skip(!isPhone(testInfo), 'the phone board');
  const modal = page.locator('.modal');
  const closeModal = () => modal.getByRole('button', { name: 'Close' }).tap();

  await chip(page, 'p1', 'Deck').tap();
  await expect(page.getByRole('dialog', { name: 'Your deck' })).toBeVisible();
  await page.getByRole('button', { name: 'Close' }).tap();

  await chip(page, 'p1', 'Discard').tap();
  await expect(modal.locator('h2')).toHaveText('p1 discard · 1');
  await closeModal();

  await chip(page, 'p2', 'Lost').tap();
  await expect(modal.locator('h2')).toHaveText('p2 Lost Zone · 0');
  await closeModal();

  // The prize view takes a prize.
  await chip(page, 'p1', 'Prizes').tap();
  await expect(modal.locator('h2')).toHaveText('p1 prizes · 6 left');
  await modal.getByRole('button', { name: 'Take' }).first().tap();
  await expect(modal.locator('h2')).toHaveText('p1 prizes · 5 left');
  await closeModal();
  await expect(chip(page, 'p1', 'Prizes')).toHaveText('Prizes 5');
});

test('a full bench of 8 can be reached by scrolling the bench row', async ({ page }, testInfo) => {
  test.skip(!isPhone(testInfo), 'the phone board');
  // Area Zero Underdepths' rule (Deep Zone here) and eight benched cards.
  await act(page, 'move', { iid: 'p1-11', to: 'discard' });
  await act(page, 'move', { iid: 'p1-12', to: 'stadium' });
  const extra = [['p1-2', 2], ['p1-3', 3], ['p1-4', 4], ['p1-19', 5], ['p1-20', 6], ['p1-21', 7]];
  for (const [iid, slot] of extra) await act(page, 'move', { iid, to: `bench:${slot}`, as: 'pokemon' });
  const last = region(page, 'p1 bench 8');
  await expect(last.locator('.card')).toHaveCount(1);

  const row = page.locator('.side.phone:not(.mirrored) .bench');
  await row.scrollIntoViewIfNeeded();
  const [scrollWidth, clientWidth] = await row.evaluate((r) => [r.scrollWidth, r.clientWidth]);
  expect(scrollWidth).toBeGreaterThan(clientWidth);
  // The row hints at the slots off screen.
  await expect(page.locator('.side.phone:not(.mirrored) .more')).toHaveText(/^\+\d+ ›$/);

  // A finger swiping the row brings the eighth slot into view.
  const box = await row.boundingBox();
  const cdp = await page.context().newCDPSession(page);
  const touch = (type, x) => cdp.send('Input.dispatchTouchEvent', { type, touchPoints: type === 'touchEnd' ? [] : [{ x, y: box.y + box.height / 2, id: 1 }] });
  await touch('touchStart', box.x + box.width - 20);
  for (let i = 1; i <= 10; i++) await touch('touchMove', box.x + box.width - 20 - i * 40);
  await touch('touchEnd');
  await expect.poll(() => row.evaluate((r) => r.scrollLeft)).toBeGreaterThan(0);
  await row.evaluate((r) => r.scrollTo({ left: r.scrollWidth }));
  const slot = await last.boundingBox();
  expect(slot.x).toBeGreaterThanOrEqual(0);
  expect(slot.x + slot.width).toBeLessThanOrEqual(390);
});

test('the header keeps the dot, warnings and ☰, and ☰ reaches every other control', async ({ page }, testInfo) => {
  test.skip(!isPhone(testInfo), 'the phone board');
  const header = page.locator('header');
  await expect(header.locator('.dot')).toBeVisible();
  await expect(menu(page)).toBeVisible();
  await expect(header.locator('label.branch')).toHaveCount(0);

  await menu(page).tap();
  await expect(page.locator('.dropdown')).toContainText('board-test');
  await expect(page.getByRole('combobox', { name: 'branch' })).toHaveValue('main');
  // Reveal all shows both hands.
  await page.getByLabel('Reveal all').tap();
  await expect(page.locator('.reveal-banner')).toBeVisible();
  await page.getByLabel('Reveal all').tap();
  await expect(page.locator('.reveal-banner')).toHaveCount(0);
  // Review mode switches to play.
  await page.getByLabel('Review mode').tap();
  await expect(page.getByLabel('Review mode')).not.toBeChecked();
  await expect(page.getByLabel('Reveal all')).toHaveCount(0);
  await page.getByLabel('Review mode').tap();
  await expect(page.getByLabel('Review mode')).toBeChecked();

  // The deck list.
  await page.getByRole('menuitem', { name: 'Deck list' }).tap();
  await expect(page.locator('.modal h2')).toContainText('Your deck');
  await page.locator('.modal').getByRole('button', { name: 'Done' }).tap();

  // The timeline opens full screen and closes.
  await menu(page).tap();
  await page.getByRole('menuitem', { name: 'Show timeline' }).tap();
  const timeline = page.locator('aside.sheet');
  expect(await timeline.boundingBox()).toEqual({ x: 0, y: 0, width: 390, height: 844 });
  await expect(timeline.locator('.line').first()).toBeVisible();
  await page.getByRole('button', { name: 'Close the timeline' }).tap();
  await expect(timeline).toHaveCount(0);
});

test('held sideways, a phone asks to be turned upright and loses nothing', async ({ page }, testInfo) => {
  test.skip(!isPhone(testInfo), 'the phone board');
  await act(page, 'move', { iid: 'p1-2', to: 'bench:2' });
  await expect(region(page, 'p1 bench 3').locator('.card[title="Riolu (p1-2)"]')).toBeVisible();

  await page.setViewportSize({ width: 844, height: 390 });
  await expect(page.getByRole('status').filter({ hasText: 'Turn your phone upright' })).toBeVisible();
  await expect(page.locator('main')).toHaveCount(0);

  await page.setViewportSize({ width: 390, height: 844 });
  await expect(page.getByRole('status').filter({ hasText: 'Turn your phone upright' })).toHaveCount(0);
  await expect(region(page, 'p1 bench 3').locator('.card[title="Riolu (p1-2)"]')).toBeVisible();
  await expect(chip(page, 'p1', 'Deck')).toBeVisible();
});

test('tablets and desktops keep the mat: piles and prizes beside it, no counters', async ({ page }, testInfo) => {
  test.skip(isPhone(testInfo), 'the mat');
  for (const seat of ['p1', 'p2']) {
    await expect(counters(page, seat)).toHaveCount(0);
    const side = region(page, `${seat} side`);
    await expect(side.locator('.piles')).toBeVisible();
    await expect(side.locator('.prizes .prize')).toHaveCount(6);
  }
  await expect(pile(page, 'Deck')).toBeVisible();
  await expect(page.getByRole('list', { name: 'p2 hand' })).toBeVisible();
  await expect(page.locator('aside')).toBeVisible();
  await expect(page.locator('aside')).not.toHaveClass(/\bsheet\b/);
});

test('tablets and desktops use none of the phone board', async ({ page }, testInfo) => {
  test.skip(isPhone(testInfo), 'the mat');
  // No phone layout anywhere: the mat's hand fan, middle strip and sides.
  await expect(page.locator('.app')).not.toHaveClass(/\bphone\b/);
  await expect(page.locator('.hand.phone, .strip.phone, .side.phone')).toHaveCount(0);
  await expect(page.locator('.seat-name').filter({ hasText: 'Alice · p1' })).toBeVisible();
  await expect(page.locator('.seat-name').filter({ hasText: 'Bob · p2' })).toBeVisible();

  // The header keeps its controls, and ☰ its timeline choices.
  const header = page.locator('header');
  await expect(header.getByText('tcgreplay', { exact: true })).toBeVisible();
  await expect(header.locator('label.branch select')).toHaveValue('main');
  await expect(header.getByLabel('Review mode')).toBeChecked();
  await expect(header.getByLabel('Reveal all')).toBeVisible();
  await page.getByRole('button', { name: 'Menu' }).click();
  await expect(page.getByRole('menuitem', { name: 'Hide timeline' })).toBeVisible();
  await expect(page.getByRole('menuitem', { name: /^Move timeline/ })).toBeVisible();
  await page.keyboard.press('Escape');

  // The timeline sits beside the board, and the cards keep the mat's
  // size: --cw from the root rule (timeline beside, width less 360px).
  const [aside, main] = [await page.locator('aside').boundingBox(), await page.locator('main').boundingBox()];
  expect(aside.x).toBeGreaterThanOrEqual(main.x + main.width - 1);
  // A benched card is one --cw wide (at least 44px on a touch screen).
  const width = await page.evaluate(() => {
    const probe = document.createElement('div');
    probe.style.width = 'clamp(48px, min((100vh - 115px) / 11.1, (100vw - 360px) / 10), 240px)';
    document.body.append(probe);
    const cw = probe.getBoundingClientRect().width;
    probe.remove();
    const card = document.querySelector('[aria-label="p1 bench 1"] .card').getBoundingClientRect().width;
    return { card: Math.round(card), cw: Math.round(Math.max(cw, matchMedia('(pointer: coarse)').matches ? 44 : 0)) };
  });
  expect(width.card).toBe(width.cw);

  // A tablet in a short window is not a phone held sideways.
  await page.setViewportSize({ width: 1024, height: 560 });
  await expect(page.getByRole('status').filter({ hasText: 'Turn your phone upright' })).toHaveCount(0);
  await expect(page.locator('main')).toBeVisible();
});
