// The compact phone board: no sideways page scroll, counters for each
// player's piles and prizes, a bench row reaching all 8 slots, the header's
// controls behind ☰, and a notice when the phone is held sideways. Tablets
// and desktops keep the mat.
import { test, expect, act, isPhone, isWide, pile } from './fixtures.js';

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
    expect(box.width, title).toBeGreaterThanOrEqual(48);
  }
});

test('the phone board fits the screen with the hand along the bottom, no scrolling', async ({ page }, testInfo) => {
  test.skip(!isPhone(testInfo), 'the phone board');
  // 390×664 is an iPhone 13 in Safari, its toolbars showing.
  for (const size of [{ width: 390, height: 844 }, { width: 390, height: 664 }, { width: 375, height: 812 }, { width: 375, height: 667 }]) {
    await page.setViewportSize(size);
    const fit = () =>
      page.evaluate(() => {
        const hand = document.querySelector('[aria-label="p1 hand"]').getBoundingClientRect();
        const main = document.querySelector('main');
        return { page: document.documentElement.scrollHeight - innerHeight, board: main.scrollHeight - main.clientHeight, handOver: Math.round(hand.bottom) - innerHeight };
      });
    // The cards follow the board area's measured height, a frame after
    // the resize.
    const at = `${size.width}×${size.height}`;
    await expect.poll(async () => (await fit()).page, `page scroll at ${at}`).toBeLessThanOrEqual(0);
    await expect.poll(async () => (await fit()).handOver, `hand on screen at ${at}`).toBeLessThanOrEqual(0);
    await expect.poll(async () => (await fit()).board, `board scroll at ${at}`).toBeLessThanOrEqual(0);
  }
  // Each counter bar is one row.
  for (const seat of ['p1', 'p2']) {
    const chips = await page.getByRole('group', { name: `${seat} counters` }).getByRole('button').evaluateAll((bs) => bs.map((b) => Math.round(b.getBoundingClientRect().top)));
    expect(new Set(chips).size, `${seat} counters on one row`).toBe(1);
  }
});

// overflow sums the phone board's rows, flattening the halves (laid out
// as display: contents, gotcha board-mat-display-contents), with the least
// gap between them and the one above the first, less the board's height.
async function overflow(page) {
  return page.locator('main').evaluate((m) => {
    const flat = (els) => els.flatMap((c) => (getComputedStyle(c).display === 'contents' ? flat([...c.children]) : [c]));
    const rows = flat([...m.children]).filter((c) => getComputedStyle(c).position !== 'absolute');
    const cs = getComputedStyle(m);
    const gaps = (parseFloat(cs.rowGap) || 0) * rows.length;
    return rows.reduce((h, c) => h + c.offsetHeight, gaps + parseFloat(cs.paddingTop) + parseFloat(cs.paddingBottom)) - m.clientHeight;
  });
}

test('a busy phone board still fits: a two-row strip and Pokémon with several attachments', async ({ page }, testInfo) => {
  test.skip(!isPhone(testInfo), 'the phone board');
  await page.setViewportSize({ width: 390, height: 664 });
  // An open action adds End action to the strip; Tools and Energy pile up.
  await act(page, 'play_trainer', { iid: 'p1-7' });
  for (const [iid, as] of [['p1-9', 'tool'], ['p1-17', 'energy'], ['p1-18', 'energy']]) await act(page, 'move', { iid, to: 'bench:1', as });
  await expect(page.getByRole('button', { name: 'End action', exact: true })).toBeVisible();
  // The phone draws the Energy as dots under the card, no attachment hangs
  // below it.
  await expect(region(page, 'p1 bench 2').getByLabel('2 Energy')).toBeVisible();
  await expect(region(page, 'p1 bench 2').locator('.tuck')).toHaveCount(0);
  await expect.poll(() => overflow(page)).toBeLessThanOrEqual(0);
  // Both players' counters are on screen, above the hand.
  const hand = await page.getByRole('list', { name: 'p1 hand' }).boundingBox();
  for (const seat of ['p1', 'p2']) {
    const bar = await page.getByRole('group', { name: `${seat} counters` }).boundingBox();
    expect(bar.y, `${seat} counters on screen`).toBeGreaterThanOrEqual(0);
    expect(bar.y + bar.height, `${seat} counters above the hand`).toBeLessThanOrEqual(hand.y + 1);
  }
});

test('the phone board follows the PhoneBoard design: even rows 12px in from the edges, 56px Bench cards with their empty spots', async ({ page }, testInfo) => {
  test.skip(!isPhone(testInfo), 'the phone board');
  const b = async (l) => {
    const r = await l.boundingBox();
    return { ...r, right: r.x + r.width, bottom: r.y + r.height };
  };
  const header = await b(page.locator('header'));
  const top = await b(counters(page, 'p2'));
  const p2Bench = await b(page.locator('.side.phone.mirrored .bench'));
  const p2Active = await b(region(page, 'p2 Active'));
  const strip = await b(page.locator('.strip'));
  const p1Active = await b(region(page, 'p1 Active'));
  const p1Bench = await b(page.locator('.side.phone:not(.mirrored) .bench'));
  const bottom = await b(counters(page, 'p1'));
  const hand = await b(page.getByRole('list', { name: 'p1 hand' }));
  // Top to bottom: the opponent's counters, Bench and Active, the strip,
  // your Active, Bench and counters, then your hand 10px off the foot.
  const rows = [header, top, p2Bench, p2Active, strip, p1Active, p1Bench, bottom, hand];
  const gaps = rows.slice(1).map((r, i) => r.y - rows[i].bottom);
  // The gap under the header matches the one under your counters, and
  // every gap between the rows is the same (the design's 13px).
  for (const [i, g] of gaps.entries()) expect(g, `gap ${i}`).toBeCloseTo(gaps[0], 0);
  expect(gaps[0]).toBeGreaterThanOrEqual(12);
  expect(844 - hand.bottom).toBeCloseTo(10, 0);
  // Each row is 12px in from the screen's edges (a counter bar's chips).
  const chips = async (seat) => {
    const all = counters(page, seat).getByRole('button');
    const [first, last] = [await b(all.first()), await b(all.last())];
    return { x: first.x, right: last.right };
  };
  for (const [i, r] of [await chips('p2'), p2Bench, p2Active, strip, p1Active, p1Bench, await chips('p1')].entries()) {
    expect(r.x, `row ${i} left`).toBeCloseTo(12, 0);
    expect(390 - r.right, `row ${i} right`).toBeCloseTo(12, 0);
  }
  // 56px Bench cards, and each empty spot shown as a box of a card's size.
  expect((await b(region(page, 'p1 bench 1').locator('.card'))).width).toBeCloseTo(56, 0);
  const empty = await b(region(page, 'p1 bench 5'));
  expect(empty.width).toBeCloseTo(56, 0);
  expect(empty.height).toBeCloseTo(78, 0);
  // The Active cards: yours 70px, the opponent's 60px; no "N Energy" or
  // "HP" words, and no attached card hanging below.
  expect((await b(region(page, 'p1 Active').locator('.main .card'))).width).toBeCloseTo(70, 0);
  expect((await b(region(page, 'p2 Active').locator('.main .card'))).width).toBeCloseTo(60, 0);
  await expect(region(page, 'p1 Active').locator('.readout')).not.toContainText('Energy');
  await expect(region(page, 'p1 Active').locator('.readout')).not.toContainText('HP');
  await expect(region(page, 'p1 Active').getByLabel('1 Energy')).toBeVisible();
  await expect(region(page, 'p1 Active').locator('.tuck')).toHaveCount(0);
  // The strip: the Stadium and its name, a 44px coin and End turn; no Draw.
  await expect(page.locator('.strip .stadium-name')).toHaveText(/Stadium\s*Arena/);
  const coin = await b(page.locator('.strip').getByRole('button', { name: 'Flip coin' }));
  expect(coin.width).toBeCloseTo(44, 0);
  expect(coin.height).toBeCloseTo(44, 0);
  await expect(page.locator('.strip').getByRole('button', { name: 'End turn' })).toBeVisible();
  await expect(page.locator('.strip').getByRole('button', { name: 'Draw', exact: true })).toHaveCount(0);
});

test('on a phone the Deck counter draws, as the strip has no Draw', async ({ page }, testInfo) => {
  test.skip(!isPhone(testInfo), 'the phone board');
  await expect(chip(page, 'p1', 'Hand')).toHaveText('Hand 8');
  await chip(page, 'p1', 'Deck').tap();
  await page.getByRole('dialog', { name: 'Your deck' }).getByRole('button', { name: 'Draw 1' }).tap();
  await expect(chip(page, 'p1', 'Hand')).toHaveText('Hand 9');
  await expect(chip(page, 'p1', 'Deck')).toHaveText('Deck 39');
});

test('a Pokémon\'s attachments are listed last in its phone sheet, each opening its card\'s view with its actions', async ({ page }, testInfo) => {
  test.skip(!isPhone(testInfo), 'the phone board');
  await act(page, 'move', { iid: 'p1-9', to: 'active', as: 'tool' });
  await page.locator('.card[title="Makuhita (p1-5)"]').tap();
  const sheet = page.getByRole('dialog', { name: 'Makuhita' });
  const attached = sheet.getByRole('group', { name: 'Attached' });
  await expect(attached.getByRole('button')).toHaveText([/^Band\s*Tool/, /^Fighting Energy\s*Energy/]);
  await expect(attached.locator('.thumb')).toHaveCount(2);
  // The Energy's view replaces the sheet and offers its moves.
  await attached.getByRole('button', { name: /^Fighting Energy/ }).tap();
  const energy = page.getByRole('dialog', { name: 'Fighting Energy' });
  await expect(energy).toHaveClass(/\bzoom\b/);
  await expect(page.locator('.sheet')).toHaveCount(0);
  await expect(energy).toContainText('Energy on Active (Makuhita)');
  await energy.getByRole('button', { name: 'To hand' }).tap();
  await expect(energy).toHaveCount(0);
  // 8 in hand, less the Band, plus the Energy.
  await expect(chip(page, 'p1', 'Hand')).toHaveText('Hand 8');
  await expect(region(page, 'p1 Active').getByLabel('1 Energy')).toHaveCount(0);
});

test('the phone header\'s warnings button opens the warnings list, as on the desktop', async ({ page }, testInfo) => {
  test.skip(!isPhone(testInfo), 'the phone board');
  await expect(page.locator('header .warn')).toHaveCount(0);
  // Half of a Stadium played as a pair, played alone, warns.
  await act(page, 'move', { iid: 'p1-59', to: 'hand' });
  await act(page, 'play_trainer', { iid: 'p1-59' });
  const warn = page.locator('header').getByRole('button', { name: /^\d+ warnings?$/ });
  await expect(warn).toBeVisible();
  const box = await warn.boundingBox();
  expect(box.height).toBeGreaterThanOrEqual(44);
  expect(box.width).toBeGreaterThanOrEqual(44);
  await warn.tap();
  const list = page.getByRole('dialog', { name: 'Warnings' });
  await expect(list).toBeVisible();
  await expect(list.locator('.warning').first()).toContainText('is played as a pair');
  const lb = await list.boundingBox();
  expect(lb.x).toBeGreaterThanOrEqual(0);
  expect(lb.x + lb.width).toBeLessThanOrEqual(390);
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
  await modal.getByRole('button', { name: 'Take this prize into your hand', exact: true }).first().tap();
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
  await page.locator('.modal').getByRole('button', { name: 'Close' }).tap();

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
  test.skip(isWide(testInfo), 'the landscape mat (wide.spec.js)');
  for (const seat of ['p1', 'p2']) {
    await expect(counters(page, seat)).toHaveCount(0);
    const side = region(page, `${seat} side`);
    await expect(side.locator('.piles')).toBeVisible();
    await expect(side.locator('.prizes .prize')).toHaveCount(6);
  }
  await expect(pile(page, 'Deck')).toBeVisible();
  // The opponent's hand is the Hand chip in their player block, opening
  // the hand view; it is not drawn on the mat.
  await expect(page.getByRole('list', { name: 'p2 hand' })).toHaveCount(0);
  await expect(region(page, 'p2 side').getByRole('button', { name: 'Hand 6' })).toBeVisible();
  await expect(page.locator('aside')).toBeVisible();
  await expect(page.locator('aside')).not.toHaveClass(/\bsheet\b/);
});

// handCount measures a hand's "N in hand" against the hand's last card and
// a benched card's width (one card width).
async function handCount(page, seat) {
  return page.evaluate((seat) => {
    const hand = document.querySelector(`[aria-label="${seat} hand"]`);
    const slots = hand.querySelectorAll('.slot');
    const last = slots[slots.length - 1].getBoundingClientRect();
    const count = hand.querySelector('.count');
    const box = count.getBoundingClientRect();
    const bench = document.querySelector('[aria-label="p1 bench 1"] .card, [aria-label="p1 bench 0"] .card');
    return {
      text: count.textContent,
      position: getComputedStyle(count).position,
      gap: box.left - last.right,
      cw: bench ? bench.getBoundingClientRect().width : null,
      handRight: hand.getBoundingClientRect().right,
      countRight: box.right,
    };
  }, seat);
}

test('on the mat each "N in hand" sits right beside its hand', async ({ page }, testInfo) => {
  test.skip(isPhone(testInfo), 'the mat');
  for (const size of [null, { width: 1920, height: 1080 }]) {
    if (size) await page.setViewportSize(size);
    // Today's mat draws your hand only; the landscape mat both.
    const landscape = (await page.locator('.app').getAttribute('data-wide')) === 'true';
    for (const seat of landscape ? ['p1', 'p2'] : ['p1']) {
      const m = await handCount(page, seat);
      expect(m.text, seat).toMatch(/^\d+ in hand$/);
      // Within one card width of the hand's nearest (last) card, never
      // overlapping it.
      expect(m.gap, `${seat} gap`).toBeGreaterThanOrEqual(-1);
      expect(m.gap, `${seat} gap`).toBeLessThanOrEqual(m.cw);
    }
  }
});

test('on a phone the hand count stays in the strip\'s corner', async ({ page }, testInfo) => {
  test.skip(!isPhone(testInfo), 'the phone board');
  const m = await handCount(page, 'p1');
  expect(m.text).toMatch(/^\d+ in hand$/);
  expect(m.position).toBe('absolute');
  expect(m.handRight - m.countRight).toBeCloseTo(12, 0);
});

test('tablets and desktops use none of the phone board', async ({ page }, testInfo) => {
  test.skip(isPhone(testInfo), 'the mat');
  // No phone layout anywhere: the mat's hand fan, middle strip and sides.
  await expect(page.locator('.app')).not.toHaveClass(/\bphone\b/);
  await expect(page.locator('.hand.phone, .strip.phone, .side.phone')).toHaveCount(0);
  // Each player block names its player: "you" and your name; the
  // opponent's name and status. The landscape mat names each by seat.
  if (isWide(testInfo)) {
    await expect(page.locator('.seat-name').filter({ hasText: 'Alice · p1' })).toBeVisible();
    await expect(page.locator('.seat-name').filter({ hasText: 'Bob · p2' })).toBeVisible();
  } else {
    await expect(region(page, 'p1 side').locator('.seat-name')).toHaveText(/^you\s*Alice$/);
    await expect(region(page, 'p2 side').locator('.seat-name')).toHaveText(/^Bob\s*waiting$/);
  }

  // The header keeps its controls, and ☰ its timeline choices.
  const header = page.locator('header');
  await expect(header.getByText('TCG Replay', { exact: true })).toBeVisible();
  await expect(header.locator('label.branch select')).toHaveValue('main');
  await expect(header.getByLabel('Review mode')).toBeChecked();
  await expect(header.getByLabel('Reveal all')).toBeVisible();
  await page.getByRole('button', { name: 'Menu' }).click();
  await expect(page.getByRole('menuitem', { name: 'Hide timeline' })).toBeVisible();
  // Beside the board the timeline's control row has no room for the dock
  // button (the Studio design), so moving the timeline is in ☰.
  await expect(page.getByRole('menuitem', { name: 'Move timeline below the board' })).toBeVisible();
  await page.keyboard.press('Escape');

  // The timeline sits beside the board, and the cards keep the mat's
  // size: --cw from the root rule (timeline beside: the height, and the
  // width the widest row needs, --cw-w). The landscape mat sizes its
  // cards from the board area instead.
  const [aside, main] = [await page.locator('aside').boundingBox(), await page.locator('main').boundingBox()];
  expect(aside.x).toBeGreaterThanOrEqual(main.x + main.width - 1);
  if (isWide(testInfo)) return;
  // A benched card is a little under one --cw wide, 0.88 of it, as the
  // Studio design draws the Bench (at least 44px on a touch screen).
  const width = await page.evaluate(() => {
    const probe = document.createElement('div');
    probe.style.width = 'var(--cw)';
    document.querySelector('.app').append(probe);
    const cw = probe.getBoundingClientRect().width;
    probe.remove();
    const card = document.querySelector('[aria-label="p1 bench 1"] .card').getBoundingClientRect().width;
    return { card: Math.round(card), cw: Math.round(Math.max(cw * 0.88, matchMedia('(pointer: coarse)').matches ? 44 : 0)) };
  });
  expect(width.card).toBe(width.cw);

  // A tablet in a short window is not a phone held sideways.
  await page.setViewportSize({ width: 1024, height: 560 });
  await expect(page.getByRole('status').filter({ hasText: 'Turn your phone upright' })).toHaveCount(0);
  await expect(page.locator('main')).toBeVisible();
});

test('☰ moves the timeline below the board, and the docked timeline\'s dock button moves it back', async ({ page }, testInfo) => {
  test.skip(isPhone(testInfo), 'the phone timeline is a sheet');
  const aside = page.locator('aside');
  await page.getByRole('button', { name: 'Menu' }).click();
  await page.getByRole('menuitem', { name: 'Move timeline below the board' }).click();
  await expect(page.locator('.body')).toHaveClass(/\bbottom\b/);
  let [a, m] = [await aside.boundingBox(), await page.locator('main').boundingBox()];
  expect(a.y).toBeGreaterThanOrEqual(m.y + m.height - 1);
  await aside.getByRole('button', { name: 'Move timeline beside the board' }).click();
  await expect(page.locator('.body')).not.toHaveClass(/\bbottom\b/);
  [a, m] = [await aside.boundingBox(), await page.locator('main').boundingBox()];
  expect(a.x).toBeGreaterThanOrEqual(m.x + m.width - 1);
});

test('the last timeline entry has no rewind button: rewinding there would change nothing', async ({ page }, testInfo) => {
  test.skip(isPhone(testInfo), 'the phone timeline is a sheet');
  const lines = page.locator('aside .line');
  // A tablet's lines show their buttons when tapped (the Studio design has
  // no per-line buttons on a touch screen's timeline beside the board).
  const show = async (l) => {
    if (testInfo.project.name === 'tablet' && (await l.locator('.row').getAttribute('aria-expanded')) !== 'true') await l.locator('.row').tap();
  };
  await show(lines.last());
  await expect(lines.last().getByRole('button', { name: /^Rewind to just after #/ })).toHaveCount(0);
  await expect(lines.last().getByTitle('Play from here')).toHaveCount(1);
  await show(lines.nth(-2));
  await expect(lines.nth(-2).getByRole('button', { name: /^Rewind to just after #/ })).toHaveCount(1);
  // A new event moves the missing button to the new last entry.
  await act(page, 'move', { iid: 'p1-2', to: 'discard' });
  await expect(lines.last()).toContainText('p1-2 Riolu p1:hand → p1:discard');
  await show(lines.last());
  await expect(lines.last().getByRole('button', { name: /^Rewind to just after #/ })).toHaveCount(0);
  await show(lines.nth(-2));
  await expect(lines.nth(-2).getByRole('button', { name: /^Rewind to just after #/ })).toHaveCount(1);
});

test('a Stadium pair shows both halves side by side, on screen and clear of the Active', async ({ page }, testInfo) => {
  test.skip(!isPhone(testInfo), 'the phone board');
  const pair = ['p1-59', 'p1-60']; // the two prints of Twin Peak (webtest.TwinPeak)
  for (const iid of pair) await act(page, 'move', { iid, to: 'hand' });
  await act(page, 'play_trainer', { iid: pair[0], pair_iid: pair[1] });
  const stadium = page.getByRole('region', { name: 'Stadium' });
  const active = await region(page, 'p1 Active').locator('.card').first().boundingBox();
  for (const iid of pair) {
    const half = stadium.locator(`.card[title="Twin Peak (${iid})"]`);
    await expect(half).toBeInViewport();
    const box = await half.boundingBox();
    const apart = box.x + box.width <= active.x || active.x + active.width <= box.x || box.y + box.height <= active.y || active.y + active.height <= box.y;
    expect(apart, `${iid} overlaps the Active`).toBe(true);
  }
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
});
