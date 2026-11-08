// Dragging cards on the board: by finger in the touch profiles and by
// mouse on the desktop, through the one pointer layer. Each drop expects
// the same hand-written timeline line in every profile, so a touch drop
// and a mouse drop are held to the same result.
import { test, expect, act, drag, press, timelineLines, isTouch, pile, pileLabel, isPhone, touchStart } from './fixtures.js';

const hand = (page) => page.getByRole('list', { name: 'p1 hand' });
const region = (page, name) => page.getByRole('region', { name, exact: true });
// pile finds p1's deck, discard or Lost Zone by its label ("Discard 1").
const label = pileLabel;
// titled finds a card by its full title, "Name (iid)".
const titled = (scope, title) => scope.locator(`.card[title="${title}"]`);

// expectNewLines waits for the timeline to grow by exactly these lines.
async function expectNewLines(page, before, lines) {
  await expect.poll(async () => (await timelineLines(page)).slice(before.length)).toEqual(lines);
}

// expectUnchanged checks, after a moment for any verb to land, that the
// timeline and p1's hand are as they were.
async function expectUnchanged(page, before, inHand = '8 in hand') {
  await page.waitForTimeout(300);
  expect(await timelineLines(page)).toEqual(before);
  await expect(hand(page).locator('.count')).toHaveText(inHand);
}

test('dragging a Pokémon from the hand to an empty bench slot benches it', async ({ page }, testInfo) => {
  const before = await timelineLines(page);
  await drag(page, titled(hand(page), 'Riolu (p1-2)'), region(page, 'p1 bench 3'), testInfo);
  await expect(titled(region(page, 'p1 bench 3'), 'Riolu (p1-2)')).toBeVisible();
  await expect(hand(page).locator('.count')).toHaveText('7 in hand');
  await expectNewLines(page, before, ['p1 moved p1-2 Riolu p1:hand → p1:bench:2']);
});

// Each drop: what is dragged, where to, the lines it adds and the board
// it leaves.
const drops = [
  {
    name: 'an Energy onto the Active Spot attaches it',
    from: (page) => titled(hand(page), 'Fighting Energy (p1-18)'),
    to: (page) => region(page, 'p1 Active'),
    lines: ['p1 moved p1-18 Fighting Energy p1:hand → p1:active as energy'],
    check: (page) => expect(titled(region(page, 'p1 Active'), 'Fighting Energy (p1-18)')).toBeVisible(),
  },
  {
    name: 'a Tool onto a bench slot attaches it',
    from: (page) => titled(hand(page), 'Band (p1-9)'),
    to: (page) => region(page, 'p1 bench 2'),
    lines: ['p1 moved p1-9 Band p1:hand → p1:bench:1 as tool'],
    check: (page) => expect(titled(region(page, 'p1 bench 2'), 'Band (p1-9)')).toBeVisible(),
  },
  {
    name: 'a Pokémon onto a Pokémon in play evolves it',
    from: (page) => titled(hand(page), 'Lucario (p1-3)'),
    to: (page) => region(page, 'p1 bench 1'),
    lines: ['p1 moved p1-3 Lucario p1:hand → p1:bench:0'],
    check: async (page) => {
      await expect(titled(region(page, 'p1 bench 1'), 'Lucario (p1-3)')).toBeVisible();
      await expect(region(page, 'p1 bench 1').locator('.chain')).toHaveText('2');
    },
  },
  {
    name: 'a card onto the discard pile discards it',
    from: (page) => titled(hand(page), 'Iono (p1-7)'),
    to: (page) => pile(page, 'Discard'),
    lines: ['p1 moved p1-7 Iono p1:hand → p1:discard'],
    check: (page) => expect(label(page, 'Discard')).toHaveText('Discard 2'),
  },
  {
    name: 'a card onto the deck puts it on top',
    from: (page) => titled(hand(page), 'Ultra Ball (p1-13)'),
    to: (page) => pile(page, 'Deck'),
    lines: ['p1 moved p1-13 Ultra Ball p1:hand → p1:deck:top'],
    check: (page) => expect(label(page, 'Deck')).toHaveText('Deck 41'),
  },
  {
    name: 'a card onto the Lost Zone sends it there',
    from: (page) => titled(hand(page), 'Fighting Energy (p1-18)'),
    to: (page) => pile(page, 'Lost'),
    lines: ['p1 moved p1-18 Fighting Energy p1:hand → p1:lost'],
    check: (page) => expect(label(page, 'Lost')).toHaveText('Lost 1'),
  },
  {
    name: 'a Stadium onto the Stadium plays it',
    from: (page) => titled(hand(page), 'Deep Zone (p1-12)'),
    to: (page) => page.getByRole('region', { name: 'Stadium' }),
    lines: ['p1 moved p1-11 Arena stadium → p1:discard', 'p1 moved p1-12 Deep Zone p1:hand → stadium'],
    check: (page) => expect(titled(page.getByRole('region', { name: 'Stadium' }), 'Deep Zone (p1-12)')).toBeVisible(),
  },
  {
    name: 'a Trainer onto the play area plays it',
    from: (page) => titled(hand(page), 'Iono (p1-7)'),
    to: (page) => page.getByRole('region', { name: 'Play area' }),
    lines: ['p1 moved p1-7 Iono p1:hand → p1:discard'],
    // Playing opens a compound action, which a plain discard does not.
    check: async (page) => {
      await expect(label(page, 'Discard')).toHaveText('Discard 2');
      await expect(page.getByRole('button', { name: /^End action/ })).toBeVisible();
    },
  },
  {
    name: 'a card from the discard pile onto the hand returns it',
    // On a phone the discard is a counter: the card comes from its view,
    // where a finger moving up or down scrolls the list, so it is lifted
    // sideways.
    from: async (page, testInfo) => {
      if (!isPhone(testInfo)) return titled(pile(page, 'Discard'), 'Fighting Energy (p1-16)');
      await hand(page).scrollIntoViewIfNeeded();
      await pile(page, 'Discard').tap();
      return titled(page.locator('.modal'), 'Fighting Energy (p1-16)');
    },
    sideways: true,
    to: (page) => hand(page),
    lines: ['p1 moved p1-16 Fighting Energy p1:discard → p1:hand'],
    check: async (page) => {
      await expect(hand(page).locator('.count')).toHaveText('9 in hand');
      await expect(label(page, 'Discard')).toHaveText('Discard 0');
    },
  },
];

for (const d of drops) {
  test(`dropping ${d.name}`, async ({ page }, testInfo) => {
    const before = await timelineLines(page);
    const from = await d.from(page, testInfo);
    if (d.sideways && isPhone(testInfo)) {
      const a = await from.boundingBox();
      const f = await touchStart(page, from);
      await f.move({ x: a.x + a.width / 2 + 30, y: a.y + a.height / 2 }, { steps: 4 });
      await f.move(d.to(page));
      await f.end();
    } else {
      await drag(page, from, d.to(page), testInfo);
    }
    await expectNewLines(page, before, d.lines);
    await d.check(page);
  });
}

test('dragging a benched Pokémon onto the Active Spot switches them', async ({ page }, testInfo) => {
  const before = await timelineLines(page);
  await drag(page, region(page, 'p1 bench 1').locator('.pile'), region(page, 'p1 Active'), testInfo);
  await expectNewLines(page, before, ['p1 swapped p1:bench:0 (Riolu) with p1:active (Makuhita)']);
  await expect(titled(region(page, 'p1 Active'), 'Riolu (p1-1)')).toBeVisible();
  await expect(titled(region(page, 'p1 bench 1'), 'Makuhita (p1-5)')).toBeVisible();
});

test('while dragging, every drop place is outlined and the one under the pointer lit', async ({ page }, testInfo) => {
  const before = await timelineLines(page);
  const outlines = () => page.evaluate(() => [...document.querySelectorAll('[data-drop]')].map((el) => [el.dataset.drop, getComputedStyle(el).outlineStyle]));
  expect((await outlines()).filter(([, style]) => style !== 'none')).toEqual([]);

  const riolu = titled(hand(page), 'Riolu (p1-2)');
  const h = await press(page, riolu, testInfo);
  const box = await riolu.boundingBox();
  await h.move({ x: box.x + box.width / 2, y: box.y - 30 }, { steps: 4 });
  await h.move(region(page, 'p1 bench 3'));

  await expect(page.locator('body')).toHaveClass(/\bdragging\b/);
  const zones = await outlines();
  expect(zones.map(([spec]) => spec)).toEqual(
    expect.arrayContaining(['p1:active', 'p1:bench:0', 'p1:bench:2', 'discard', 'deck', 'lost', 'stadium', 'play', 'hand']),
  );
  expect(zones.filter(([, style]) => style === 'none')).toEqual([]);
  await expect(page.locator('.drop-lit')).toHaveCount(1);
  await expect(region(page, 'p1 bench 3')).toHaveClass(/\bdrop-lit\b/);
  // The ghost is a copy of the dragged card.
  const ghost = page.locator('.card.drag-ghost[title="Riolu (p1-2)"]');
  await expect(ghost).toBeInViewport();
  // It follows the pointer, aimed at the centre of bench 3: centred on a
  // mouse, raised up to a card's height above a finger.
  // (It moves once a frame, so wait for it to catch up.)
  const slot = await region(page, 'p1 bench 3').boundingBox();
  const [sx, sy] = [slot.x + slot.width / 2, slot.y + slot.height / 2];
  await expect.poll(async () => { const g = await ghost.boundingBox(); return Math.abs(g.x + g.width / 2 - sx); }).toBeLessThan(4);
  const g = await ghost.boundingBox();
  expect(g.y + g.height / 2).toBeLessThanOrEqual(sy + 4);
  expect(g.y + g.height / 2).toBeGreaterThanOrEqual(sy - g.height);

  // The light follows the pointer to the next zone.
  await h.move(region(page, 'p1 Active'));
  await expect(region(page, 'p1 Active')).toHaveClass(/\bdrop-lit\b/);
  await expect(page.locator('.drop-lit')).toHaveCount(1);

  // Released over no drop place, nothing happens.
  // The felt at the screen's left edge, level with p1's Active, belongs
  // to no drop place.
  const row = await region(page, 'p1 Active').boundingBox();
  await h.move({ x: 4, y: row.y + row.height / 2 });
  await expect(page.locator('.drop-lit')).toHaveCount(0);
  await h.end();
  await expect(page.locator('body')).not.toHaveClass(/\bdragging\b/);
  await expect(page.locator('.drag-ghost')).toHaveCount(0);
  await expectUnchanged(page, before);
  await expect(titled(hand(page), 'Riolu (p1-2)')).toBeVisible();
});

// overflowHand adds Energy from the deck until the phone's hand is wider
// than the screen.
async function overflowHand(page) {
  for (let i = 22; i <= 35; i++) await act(page, 'move', { iid: `p1-${i}`, to: 'hand' });
  await expect(hand(page).locator('.count')).toHaveText('22 in hand');
  const [scrollWidth, clientWidth] = await hand(page).evaluate((h) => [h.scrollWidth, h.clientWidth]);
  expect(scrollWidth).toBeGreaterThan(clientWidth);
}

test('on a phone, a sideways swipe scrolls an overflowing hand and moves no card', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== 'phone', 'the hand scrolls on phone-sized screens');
  await overflowHand(page);
  const before = await timelineLines(page);
  const scrollLeft = () => hand(page).evaluate((h) => h.scrollLeft);
  expect(await scrollLeft()).toBe(0);

  const lucario = titled(hand(page), 'Lucario (p1-3)');
  await lucario.scrollIntoViewIfNeeded();
  const box = await lucario.boundingBox();
  const at = { x: box.x + box.width / 2, y: box.y + box.height / 2 };
  const f = await press(page, at, testInfo);
  await f.move({ x: at.x - 150, y: at.y + 2 }, { steps: 10 });
  await expect(page.locator('body')).not.toHaveClass(/\bdragging\b/);
  await expect(page.locator('.drag-ghost')).toHaveCount(0);
  await f.end();

  expect(await scrollLeft()).toBeGreaterThan(0);
  await expectUnchanged(page, before, '22 in hand');
  await expect(lucario).toBeAttached();
});

test('on a phone, an upward drag from an overflowing hand lifts the card', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== 'phone', 'the hand scrolls on phone-sized screens');
  await overflowHand(page);
  const before = await timelineLines(page);
  await drag(page, titled(hand(page), 'Riolu (p1-2)'), region(page, 'p1 bench 3'), testInfo);
  await expectNewLines(page, before, ['p1 moved p1-2 Riolu p1:hand → p1:bench:2']);
  await expect(titled(region(page, 'p1 bench 3'), 'Riolu (p1-2)')).toBeVisible();
  await expect(hand(page).locator('.count')).toHaveText('21 in hand');
});

test('on the desktop, the play area shows only while a card is dragged', async ({ page }, testInfo) => {
  test.skip(isTouch(testInfo), 'mouse dragging on the desktop');
  const play = page.getByRole('region', { name: 'Play area' });
  await expect(play).toHaveCount(0);
  const iono = titled(hand(page), 'Iono (p1-7)');
  const m = await press(page, iono, testInfo);
  const box = await iono.boundingBox();
  await m.move({ x: box.x + box.width / 2, y: box.y - 30 }, { steps: 4 });
  await expect(play).toBeVisible();
  await m.move(play);
  await expect(play).toHaveClass(/\bdrop-lit\b/);
  await m.end();
  await expect(play).toHaveCount(0);
  await expect(label(page, 'Discard')).toHaveText('Discard 2');
});

test('on the desktop, a hand card enlarged by hovering still drops where the pointer is', async ({ page }, testInfo) => {
  test.skip(isTouch(testInfo), 'mouse hover on the desktop');
  const before = await timelineLines(page);
  const band = titled(hand(page), 'Band (p1-9)');
  // Resting on a hand card enlarges it, as a player does before picking it up.
  await band.hover();
  await page.waitForTimeout(600);
  await drag(page, band, region(page, 'p1 bench 2'), testInfo);
  await expectNewLines(page, before, ['p1 moved p1-9 Band p1:hand → p1:bench:1 as tool']);
});

test('a press that barely moves is a tap, not a drag', async ({ page }, testInfo) => {
  const before = await timelineLines(page);
  const riolu = titled(hand(page), 'Riolu (p1-2)');
  await riolu.scrollIntoViewIfNeeded();
  const box = await riolu.boundingBox();
  const at = { x: box.x + box.width / 2, y: box.y + box.height / 2 };
  const h = await press(page, at, testInfo);
  await h.move({ x: at.x + 3, y: at.y - 3 }, { steps: 3 });
  await expect(page.locator('body')).not.toHaveClass(/\bdragging\b/);
  await h.end();
  // A tap on a card zooms it.
  await expect(page.getByRole('dialog', { name: 'Riolu' })).toBeVisible();
  await expectUnchanged(page, before);
});

// The two prints of Twin Peak, a made-up Stadium played as a pair, start
// in p1's deck (webtest.TwinPeak).
const twinPeak = ['p1-59', 'p1-60'];

// standingWarnings is the board's standing warnings: the header's ⚠ on a
// mat, the list behind ☰ on a phone.
async function standingWarnings(page, testInfo) {
  if (!isPhone(testInfo)) {
    const warn = page.locator('header .warn');
    return (await warn.count()) ? (await warn.getAttribute('title')).split('\n') : [];
  }
  await page.getByRole('button', { name: 'Menu' }).tap();
  const dropdown = page.locator('.dropdown');
  await expect(dropdown).toBeVisible();
  const lines = (await dropdown.locator('.warning').allInnerTexts()).map((w) => w.replace(/^⚠\s*/, ''));
  await page.locator('.more .backdrop').tap({ position: { x: 5, y: 200 } });
  await expect(dropdown).toHaveCount(0);
  return lines;
}

test('dragging the two halves of a pair-played Stadium onto the Stadium plays them as one', async ({ page }, testInfo) => {
  // The hand fans its newest card over the one before, so the second
  // print moved to the hand is dragged first.
  const [b, a] = twinPeak;
  for (const iid of twinPeak) await act(page, 'move', { iid, to: 'hand' });
  const stadium = page.getByRole('region', { name: 'Stadium' });
  await expect(titled(hand(page), `Twin Peak (${b})`)).toHaveCount(1);
  let before = await timelineLines(page);

  // The first half replaces the Arena and warns that it needs its partner.
  await drag(page, titled(hand(page), `Twin Peak (${a})`), stadium, testInfo);
  await expectNewLines(page, before, ['p1 moved p1-11 Arena stadium → p1:discard', `p1 moved ${a} Twin Peak p1:hand → stadium`]);
  await expect(titled(stadium, `Twin Peak (${a})`)).toBeVisible();
  await expect.poll(async () => (await standingWarnings(page, testInfo)).join('\n')).toContain(`Twin Peak (${a}) is played as a pair`);

  // The second half dropped on it completes the pair: both show, and the
  // warning is gone.
  before = await timelineLines(page);
  await drag(page, titled(hand(page), `Twin Peak (${b})`), stadium, testInfo);
  await expectNewLines(page, before, [`p1 moved ${b} Twin Peak p1:hand → stadium as pair`]);
  await expect(titled(stadium, `Twin Peak (${a})`)).toBeVisible();
  await expect(titled(stadium, `Twin Peak (${b})`)).toBeVisible();
  await expect.poll(async () => (await standingWarnings(page, testInfo)).join('\n')).not.toContain('is played as a pair');
});

test('each half of a Stadium pair opens in the zoom', async ({ page }) => {
  const [a, b] = twinPeak;
  for (const iid of twinPeak) await act(page, 'move', { iid, to: 'hand' });
  await act(page, 'play_trainer', { iid: a, pair_iid: b });
  const stadium = page.getByRole('region', { name: 'Stadium' });
  for (const iid of twinPeak) {
    const half = titled(stadium, `Twin Peak (${iid})`);
    await expect(half).toBeVisible();
    // Click where the half shows: the first half's left edge (the second
    // half lies over the rest of it), the second half's middle.
    const box = await half.boundingBox();
    const x = iid === a ? box.x + 8 : box.x + box.width / 2;
    await page.mouse.click(x, box.y + box.height / 2);
    await expect(page.locator('.zoom .meta').first(), `zoom of ${iid}`).toContainText(iid);
    await page.keyboard.press('Escape');
    await page.locator('.backdrop').first().click({ position: { x: 5, y: 5 } }).catch(() => {});
    await expect(page.locator('.zoom')).toHaveCount(0);
  }
});
