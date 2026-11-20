// The landscape mat on a big monitor: both Active Pokémon side by side in
// the middle row between the Stadium and the turn controls, each player's
// prize fan, bench and piles in one mirrored row, the opponent's hand
// peeking from the top edge with their name in the corner, cards sized
// from the board area so it fits without scrolling, and every pile, prize,
// drop target, action sheet and turn control working as on today's mat.
// The board switches between the landscape mat, today's mat and the phone
// board as the window resizes. The checks shared with today's mat run on
// this profile too: hand counts beside their hands (phone.spec.js), and
// the switch as the timeline moves or hides, playback centring and text
// and timeline scaling (desktop.spec.js).
import { test, expect, act, mouseDrag, mouseStart, pile, pileLabel, isWide } from './fixtures.js';

const region = (page, name) => page.getByRole('region', { name, exact: true });
const side = (page, seat) => region(page, `${seat} side`);
const hand = (page) => page.getByRole('list', { name: 'p1 hand' });
const controls = (page) => page.locator('.strip > .controls');
const titled = (scope, title) => scope.locator(`.card[title="${title}"]`);

// box is a locator's bounding box with its right and bottom edges.
async function box(locator) {
  const b = await locator.boundingBox();
  return { ...b, right: b.x + b.width, bottom: b.y + b.height };
}

// overlapY reports whether two boxes share some of their height: they sit
// in one row.
const overlapY = (a, b) => a.y < b.bottom && b.y < a.bottom;

// fit measures how far the board area and the page overflow, both ways.
async function fit(page) {
  return page.evaluate(() => {
    const main = document.querySelector('main');
    const doc = document.documentElement;
    return {
      mainY: main.scrollHeight - main.clientHeight,
      mainX: main.scrollWidth - main.clientWidth,
      pageY: doc.scrollHeight - innerHeight,
      pageX: doc.scrollWidth - innerWidth,
    };
  });
}

test('both Actives share the middle row, labelled with whose they are, between the Stadium and the turn controls', async ({ page }, testInfo) => {
  test.skip(!isWide(testInfo), 'the landscape mat');
  const p1 = await box(region(page, 'p1 Active'));
  const p2 = await box(region(page, 'p2 Active'));
  expect(overlapY(p1, p2), 'the Actives share a row').toBe(true);
  // Their tops line up (an attached card can make one spot taller).
  expect(Math.abs(p1.y - p2.y), 'the Actives\' tops line up').toBeLessThanOrEqual(2);
  await expect(page.locator('.strip .actives .vs')).toHaveText('vs');

  const stadium = await box(page.getByRole('region', { name: 'Stadium' }));
  const turn = await box(controls(page));
  expect(stadium.right, 'the Stadium left of both Actives').toBeLessThanOrEqual(Math.min(p1.x, p2.x));
  expect(turn.x, 'the turn controls right of both Actives').toBeGreaterThanOrEqual(Math.max(p1.right, p2.right));
  expect(overlapY(stadium, p1), 'the Stadium in the middle row').toBe(true);
  expect(overlapY(turn, p1), 'the turn controls in the middle row').toBe(true);
  await expect(controls(page).getByRole('button', { name: 'Draw', exact: true })).toBeVisible();
  await expect(controls(page).getByRole('button', { name: 'End turn', exact: true })).toBeVisible();

  // Each Active is captioned with its player's name.
  const spot = (seat) => page.locator('.strip .spot').filter({ has: region(page, `${seat} Active`) });
  await expect(spot('p1').locator('figcaption')).toHaveText("Alice's Active");
  await expect(spot('p2').locator('figcaption')).toHaveText("Bob's Active");
  for (const seat of ['p1', 'p2']) await expect(spot(seat).locator('figcaption')).toBeInViewport();
});

test('each player\'s prizes, bench and piles share one row, mirrored for the opponent above the Actives', async ({ page }, testInfo) => {
  test.skip(!isWide(testInfo), 'the landscape mat');
  const actives = await box(page.locator('.strip .actives'));
  const rows = {};
  for (const seat of ['p1', 'p2']) {
    const s = side(page, seat);
    const [prizes, bench, piles] = [await box(s.locator('.prizes')), await box(s.locator('.bench')), await box(s.locator('.piles'))];
    expect(overlapY(prizes, bench), `${seat} prizes and bench in one row`).toBe(true);
    expect(overlapY(bench, piles), `${seat} bench and piles in one row`).toBe(true);
    expect(overlapY(prizes, piles), `${seat} prizes and piles in one row`).toBe(true);
    rows[seat] = { prizes, bench, piles };
  }
  // The opponent's row is above the Actives, yours below.
  for (const part of ['prizes', 'bench', 'piles']) {
    expect(rows.p2[part].bottom, `p2 ${part} above the Actives`).toBeLessThanOrEqual(actives.y);
    expect(rows.p1[part].y, `p1 ${part} below the Actives`).toBeGreaterThanOrEqual(actives.bottom);
  }
  // Your prizes, bench and piles run left to right; the opponent's the
  // other way, as across a table.
  const p1 = rows.p1;
  const p2 = rows.p2;
  expect(p1.prizes.right).toBeLessThanOrEqual(p1.bench.x);
  expect(p1.bench.right).toBeLessThanOrEqual(p1.piles.x);
  expect(p2.piles.right).toBeLessThanOrEqual(p2.bench.x);
  expect(p2.bench.right).toBeLessThanOrEqual(p2.prizes.x);
  // So the prizes sit at opposite ends of the mat, and the piles too.
  expect(p1.prizes.x).toBeLessThan(p2.prizes.x);
  expect(p2.piles.x).toBeLessThan(p1.piles.x);
});

// uncovered is the middle of the strip of a fanned prize that the next
// prize does not cover, relative to the prize, where a click lands on it.
async function uncovered(prizes, i) {
  const b = await box(prizes.nth(i));
  const right = i + 1 < (await prizes.count()) ? (await box(prizes.nth(i + 1))).x : b.right;
  return { x: (right - b.x) / 2, y: b.height / 2 };
}

// clickPrize clicks a fanned prize where it shows.
async function clickPrize(prizes, i) {
  await prizes.nth(i).click({ position: await uncovered(prizes, i) });
}

// expectFan checks a seat's prize fan shows `left` prizes, all on screen,
// in one row, each overlapping the next and each showing enough to click.
async function expectFan(page, seat, left) {
  const prizes = side(page, seat).locator('.prizes .prize');
  await expect(prizes).toHaveCount(left);
  const boxes = [];
  for (let i = 0; i < left; i++) {
    await expect(prizes.nth(i)).toBeInViewport({ ratio: 1 });
    boxes.push(await box(prizes.nth(i)));
  }
  for (let i = 1; i < left; i++) {
    expect(Math.abs(boxes[i].y - boxes[0].y), `${seat} prize ${i + 1} in the row`).toBeLessThanOrEqual(1);
    expect(boxes[i].x, `${seat} prize ${i + 1} overlaps the one before`).toBeLessThan(boxes[i - 1].right);
    expect(boxes[i].x, `${seat} prize ${i + 1} after the one before`).toBeGreaterThan(boxes[i - 1].x);
  }
  for (let i = 0; i < left; i++) {
    const at = await uncovered(prizes, i);
    const hit = await prizes.nth(i).evaluate((el, at) => {
      const r = el.getBoundingClientRect();
      return document.elementFromPoint(r.left + at.x, r.top + at.y)?.closest('.prize') === el;
    }, at);
    expect(hit, `${seat} prize ${i + 1} of ${left} can be clicked`).toBe(true);
  }
}

test('with 6, 3 and 1 prizes left every remaining prize shows in one overlapping row and can be taken', async ({ page }, testInfo) => {
  test.skip(!isWide(testInfo), 'the landscape mat');
  for (const seat of ['p1', 'p2']) {
    const args = seat === 'p1' ? {} : { seat: 'p2' };
    await expectFan(page, seat, 6);
    for (const slot of [0, 1, 2]) await act(page, 'take_prize', { slot, ...args });
    await expectFan(page, seat, 3);
    // A click on the middle one of three takes it (slot 4), in review mode
    // for either seat.
    const prizes = side(page, seat).locator('.prizes .prize');
    await clickPrize(prizes, 1);
    await expect(prizes).toHaveCount(2);
    await act(page, 'take_prize', { slot: 3, ...args });
    await expectFan(page, seat, 1);
    await expect(side(page, seat).locator('.prizes .count')).toHaveText('1');

    await clickPrize(prizes, 0);
    await expect(prizes).toHaveCount(0);
    await expect(side(page, seat).locator('.prizes .count')).toHaveText('0');
  }
  // Each prize went to its player's hand.
  await expect(hand(page).locator('.count')).toHaveText('14 in hand');
  await expect(page.getByRole('list', { name: 'p2 hand' }).locator('.count')).toHaveText('12 in hand');
});

test('a benched card is at least 1.6 times its width on today\'s mat at 2560×1440', async ({ page }, testInfo) => {
  test.skip(!isWide(testInfo), 'the landscape mat');
  // Today's --cw with the timeline beside the board (the root rule), at
  // this viewport: 119.4px at 2560×1440.
  const width = await page.evaluate(() => {
    const probe = document.createElement('div');
    probe.style.width = 'clamp(48px, min((100vh - 115px) / 11.1, (100vw - 360px) / 10), 240px)';
    document.body.append(probe);
    const today = probe.getBoundingClientRect().width;
    probe.remove();
    const card = document.querySelector('[aria-label="p1 bench 1"] .card').getBoundingClientRect().width;
    return { today, card };
  });
  expect(width.today).toBeCloseTo(119.4, 0);
  expect(width.card).toBeGreaterThanOrEqual(1.6 * width.today);
});

test('the board fits with no scroll bars at 2560×1440 and 3840×2160, with the timeline beside, below or hidden', async ({ page }, testInfo) => {
  test.skip(!isWide(testInfo), 'the landscape mat');
  const aside = page.locator('aside');
  const sizes = [{ width: 2560, height: 1440 }, { width: 3840, height: 2160 }];
  const check = async (where) => {
    for (const size of sizes) {
      await page.setViewportSize(size);
      const at = `${size.width}×${size.height}, timeline ${where}`;
      await expect(page.locator('.app'), at).toHaveAttribute('data-wide', 'true');
      const f = await fit(page);
      expect(f.mainY, `board scrolls down at ${at}`).toBeLessThanOrEqual(1);
      expect(f.mainX, `board scrolls across at ${at}`).toBeLessThanOrEqual(1);
      expect(f.pageY, `page scrolls down at ${at}`).toBeLessThanOrEqual(0);
      expect(f.pageX, `page scrolls across at ${at}`).toBeLessThanOrEqual(0);
    }
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
});

test('each pile opens on the landscape mat: the deck\'s actions, the discard and the Lost Zone', async ({ page }, testInfo) => {
  test.skip(!isWide(testInfo), 'the landscape mat');
  const modal = page.locator('.modal');
  for (const seat of ['p1', 'p2']) {
    await pile(page, 'Deck', seat).click();
    await expect(page.locator('.sheet')).toBeVisible();
    await expect(page.locator('.sheet section[role="group"][aria-label="Draw"]')).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(page.locator('.sheet')).toHaveCount(0);

    await pile(page, 'Discard', seat).click();
    await expect(modal.locator('h2')).toHaveText(seat === 'p1' ? 'p1 discard · 1' : 'p2 discard · 0');
    await modal.getByRole('button', { name: 'Close' }).click();
    await expect(modal).toHaveCount(0);

    await pile(page, 'Lost', seat).click();
    await expect(modal.locator('h2')).toHaveText(`${seat} Lost Zone · 0`);
    await modal.getByRole('button', { name: 'Close' }).click();
    await expect(modal).toHaveCount(0);
  }
});

test('a prize is taken by clicking it on the landscape mat', async ({ page }, testInfo) => {
  test.skip(!isWide(testInfo), 'the landscape mat');
  const prizes = side(page, 'p1').locator('.prizes .prize');
  await expect(prizes).toHaveCount(6);
  await clickPrize(prizes, 2);
  await expect(prizes).toHaveCount(5);
  await expect(side(page, 'p1').locator('.prizes .count')).toHaveText('5');
  await expect(hand(page).locator('.count')).toHaveText('9 in hand');
});

test('dragging a Pokémon from the hand to a free bench slot benches it on the landscape mat', async ({ page }, testInfo) => {
  test.skip(!isWide(testInfo), 'the landscape mat');
  await mouseDrag(page, titled(hand(page), 'Riolu (p1-2)'), region(page, 'p1 bench 3'));
  await expect(titled(region(page, 'p1 bench 3'), 'Riolu (p1-2)')).toBeVisible();
  await expect(hand(page).locator('.count')).toHaveText('7 in hand');
  // Dropping on a pile works too: the Energy goes to the discard.
  await mouseDrag(page, titled(hand(page), 'Fighting Energy (p1-18)'), pile(page, 'Discard'));
  await expect(pileLabel(page, 'Discard')).toHaveText('Discard 2');
});

test('a card\'s action sheet opens on the landscape mat', async ({ page }, testInfo) => {
  test.skip(!isWide(testInfo), 'the landscape mat');
  const sheet = page.locator('.sheet');
  for (const title of ['Makuhita (p1-5)', 'Makuhita (p2-5)', 'Riolu (p1-1)']) {
    await page.locator(`.card[title="${title}"]`).first().click({ button: 'right' });
    await expect(sheet, title).toBeVisible();
    await expect(sheet).toHaveAttribute('data-mode', 'popover');
    await expect(sheet.locator('section[role="group"]').first(), title).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(sheet).toHaveCount(0);
  }
});

test('Draw and End turn work on the landscape mat', async ({ page }, testInfo) => {
  test.skip(!isWide(testInfo), 'the landscape mat');
  const turn = controls(page).locator('.turn');
  await expect(turn).toHaveText('Turn 1 · Your turn');
  await controls(page).getByRole('button', { name: 'Draw', exact: true }).click();
  await expect(hand(page).locator('.count')).toHaveText('9 in hand');
  await expect(pileLabel(page, 'Deck')).toHaveText('Deck 39');

  await controls(page).getByRole('button', { name: 'End turn', exact: true }).click();
  await expect(turn).toHaveText("Turn 1 · Bob's turn");
});

test('the opponent\'s hand peeks from the top edge: every card back partly in view, still a list with its count', async ({ page }, testInfo) => {
  test.skip(!isWide(testInfo), 'the landscape mat');
  const main = await box(page.locator('main'));
  const list = page.getByRole('list', { name: 'p2 hand' });
  await expect(list.getByRole('listitem')).toHaveCount(6);
  const cards = list.locator('.card');
  await expect(cards).toHaveCount(6);
  for (let i = 0; i < 6; i++) {
    const c = await box(cards.nth(i));
    // It starts above the board's top edge and ends a little way into it.
    expect(c.y, `card ${i + 1}`).toBeLessThan(main.y);
    expect(c.bottom, `card ${i + 1}`).toBeGreaterThan(main.y + 8);
    expect(c.bottom - main.y, `card ${i + 1} shows under half`).toBeLessThan(c.height / 2);
    // The part in view is the part a player sees: nothing covers it.
    const hit = await page.evaluate(({ x, y }) => !!document.elementFromPoint(x, y)?.closest('[aria-label="p2 hand"]'), { x: c.x + c.width / 2, y: main.y + 4 });
    expect(hit, `card ${i + 1} on top`).toBe(true);
  }
  // The count reads as text beside the cards' visible ends.
  await expect(list.locator('.count')).toHaveText('6 in hand');
  const count = await box(list.locator('.count'));
  expect(count.y).toBeGreaterThanOrEqual(main.y);
  // The hand takes no row: the opponent's row starts near the top.
  const row = await box(side(page, 'p2'));
  expect(row.y - main.y).toBeLessThan(main.height * 0.12);
});

test('the opponent\'s name and status sit in the board\'s top-left corner, with no bar across it', async ({ page }, testInfo) => {
  test.skip(!isWide(testInfo), 'the landscape mat');
  const main = await box(page.locator('main'));
  const name = page.locator('main .seat-name.top');
  await expect(name).toContainText('Bob · p2');
  const b = await box(name);
  expect(b.x - main.x).toBeLessThan(120);
  expect(b.y - main.y).toBeLessThan(120);
  expect(b.width).toBeLessThan(main.width / 2);
  // It is clear of the peeking hand.
  const hand = await box(page.getByRole('list', { name: 'p2 hand' }).locator('.slot').first());
  expect(b.right).toBeLessThan(hand.x);
});

// clipped lists the board's parts, apart from the peeking hand, that run
// past the board area's edges.
async function clipped(page) {
  return page.evaluate(() => {
    const m = document.querySelector('main').getBoundingClientRect();
    const parts = document.querySelectorAll('.board .card, .board .slot, .board .pile, .board .label, .board .seat-name, .board .strip button, .board .turn, .board .count, .board figcaption');
    return [...parts]
      .filter((el) => !el.closest('.peek') && el.getClientRects().length)
      .map((el) => [el, el.getBoundingClientRect()])
      .filter(([, r]) => r.width > 1 && (r.left < m.left - 1 || r.top < m.top - 1 || r.right > m.right + 1 || r.bottom > m.bottom + 1))
      .map(([el]) => el.getAttribute('title') || el.className);
  });
}

test('apart from the peek, nothing on the board is cut off at 2560×1440 or 3840×2160, even with Energy under benched Pokémon', async ({ page }, testInfo) => {
  test.skip(!isWide(testInfo), 'the landscape mat');
  for (const size of [{ width: 2560, height: 1440 }, { width: 3840, height: 2160 }]) {
    await page.setViewportSize(size);
    await expect(page.locator('.app')).toHaveAttribute('data-wide', 'true');
    expect(await clipped(page), `${size.width}×${size.height}`).toEqual([]);
  }
  // A busy board: Energy and a Tool under benched Pokémon in both rows.
  for (const [iid, to, as] of [['p1-17', 'bench:0', 'energy'], ['p1-18', 'bench:1', 'energy'], ['p1-9', 'bench:1', 'tool'], ['p2-2', 'p2:bench:0', 'energy']]) {
    await act(page, 'move', { iid, to, as, ...(iid.startsWith('p2') ? { seat: 'p2' } : {}) });
  }
  await expect(region(page, 'p2 bench 1').locator('.att')).toHaveCount(1);
  for (const size of [{ width: 2560, height: 1440 }, { width: 3840, height: 2160 }]) {
    await page.setViewportSize(size);
    await expect.poll(() => clipped(page), { message: `busy ${size.width}×${size.height}` }).toEqual([]);
    await expect.poll(() => page.locator('main').evaluate((m) => m.scrollHeight - m.clientHeight)).toBeLessThanOrEqual(1);
  }
});

test('a dragged card\'s ghost keeps the card\'s size on the landscape mat, with the timeline below', async ({ page }, testInfo) => {
  test.skip(!isWide(testInfo), 'the landscape mat');
  // Below the board, the timeline makes the board area much shorter than
  // the window, so a size taken from the window would show.
  await page.locator('aside').getByRole('button', { name: 'Move timeline below the board' }).click();
  await expect(page.locator('.app')).toHaveAttribute('data-wide', 'true');
  const riolu = titled(hand(page), 'Riolu (p1-2)');
  // A card's corners are 5% of its width (Card.svelte), so they show its size.
  const radius = (el) => parseFloat(getComputedStyle(el).borderTopLeftRadius);
  const before = await riolu.evaluate(radius);
  const b = await box(riolu);
  const h = await mouseStart(page, riolu);
  await h.move({ x: b.x + b.width / 2, y: b.y + b.height / 2 - 30 }, { steps: 4 });
  await h.move(region(page, 'p1 bench 3'));
  const ghost = page.locator('.card.drag-ghost[title="Riolu (p1-2)"]');
  await expect(ghost).toBeInViewport();
  expect(await ghost.evaluate(radius)).toBeCloseTo(before, 1);
  await h.end();
});

// layoutShown names the board the page shows: the landscape mat, today's
// mat or the phone board.
async function layoutShown(page) {
  return page.evaluate(() => {
    if (document.querySelector('.side.wide')) return 'landscape';
    if (document.querySelector('.side.phone')) return 'phone';
    if (document.querySelector('.side .field')) return 'mat';
    return 'none';
  });
}

test('the board is the landscape mat at 2560×1440, today\'s mat at 1440×900 and the phone board at 390×844, switching live', async ({ page }, testInfo) => {
  test.skip(!isWide(testInfo), 'the landscape mat');
  await page.evaluate(() => (window.__samePage = true));
  expect(await layoutShown(page)).toBe('landscape');
  await page.setViewportSize({ width: 1440, height: 900 });
  await expect.poll(() => layoutShown(page)).toBe('mat');
  await expect(page.locator('.app')).toHaveAttribute('data-wide', 'false');
  await page.setViewportSize({ width: 390, height: 844 });
  await expect.poll(() => layoutShown(page)).toBe('phone');
  await page.setViewportSize({ width: 2560, height: 1440 });
  await expect.poll(() => layoutShown(page)).toBe('landscape');
  // Every Active, bench slot, pile and prize is back after the round trip.
  await expect(region(page, 'p1 Active')).toHaveCount(1);
  await expect(region(page, 'p2 Active')).toHaveCount(1);
  await expect(side(page, 'p1').locator('.prizes .prize')).toHaveCount(6);
  await expect(pile(page, 'Deck')).toBeVisible();
  expect(await page.evaluate(() => window.__samePage)).toBe(true);
});

test('on the landscape mat each "N in hand" sits within one card width of its hand', async ({ page }, testInfo) => {
  test.skip(!isWide(testInfo), 'the landscape mat');
  const cw = (await box(region(page, 'p1 bench 1').locator('.card'))).width;
  for (const seat of ['p1', 'p2']) {
    const list = page.getByRole('list', { name: `${seat} hand` });
    const count = await box(list.locator('.count'));
    // The nearest card is the last one; its box is measured unturned by
    // taking the slot's centre and the card's own width.
    const last = await box(list.locator('.slot').last());
    await expect(list.locator('.count')).toBeInViewport();
    expect(count.x - last.right, seat).toBeGreaterThanOrEqual(-1);
    expect(count.x - last.right, seat).toBeLessThanOrEqual(cw);
  }
});
