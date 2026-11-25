// The card-list views' icon actions (the PileActions design): every row of
// the discard, the Lost Zone, the deck search and the top N shows all six
// fixed cells, an action the card can't take disabled with the reason; a
// key under the header names the six, and "Hide key" is remembered in the
// browser. The discard is a list of rows in the pile's own order.
import { test, expect, act, pile, isTouch, isPhone, isWide, timelineLines } from './fixtures.js';

const press = (locator, testInfo) => (isTouch(testInfo) ? locator.tap() : locator.click());
const view = (page) => page.locator('.modal');
const rows = (page) => view(page).getByRole('listitem');
// cellsOf is a row's six buttons by its card's name.
const cellsOf = (page, name) => view(page).getByRole('group', { name, exact: true }).first().getByRole('button');
// cells reads each button's name, whether it is disabled and its tooltip.
const cells = (locator) => locator.evaluateAll((bs) => bs.map((b) => ({ name: b.getAttribute('aria-label'), disabled: b.disabled, title: b.title })));

const SIX_PILE = ['Put in hand', 'Move to the Bench', 'Attach to a Pokémon', 'Put on top of the deck', 'Discard', 'Put in the Lost Zone'];
const SIX_DECK = ['Put in hand', 'Move to the Bench', 'Attach to a Pokémon', 'Put on the bottom of the deck', 'Discard', 'Put in the Lost Zone'];

async function openSearch(page, testInfo) {
  await press(pile(page, 'Deck'), testInfo);
  await press(page.locator('.sheet').getByRole('button', { name: 'Search…', exact: true }), testInfo);
  await expect(rows(page).first()).toBeVisible();
}
async function openDiscard(page, testInfo) {
  await press(pile(page, 'Discard'), testInfo);
  await expect(rows(page).first()).toBeVisible();
}
// deckCard finds a card in p1's deck by name, from the deck search.
async function deckCards(page) {
  return (await act(page, 'search_deck', { whose: 'p1', filter: '' })).data;
}
// discardSome moves the named deck cards to the discard in this order, so
// the last one ends on top, and reloads to show the result live.
async function discardSome(page, names) {
  const deck = await deckCards(page);
  const moved = [];
  for (const n of names) {
    const c = deck.find((d) => d.name === n && !moved.includes(d.iid));
    moved.push(c.iid);
    await act(page, 'move', { iid: c.iid, to: 'discard' });
  }
  await page.reload();
  await expect(page.locator('header .dot.on')).toBeVisible();
}

test('every row of the deck search and the discard shows all six cells', async ({ page }, testInfo) => {
  test.skip(isWide(testInfo), 'the views are the same on the landscape mat');
  await discardSome(page, ['Iono', 'Lucario', 'Band']);
  await openDiscard(page, testInfo);
  const n = await rows(page).count();
  expect(n).toBe(4);
  for (let i = 0; i < n; i++) {
    const got = await cells(rows(page).nth(i).getByRole('group').getByRole('button'));
    expect(got.map((c) => c.name)).toEqual(SIX_PILE);
  }
  await page.keyboard.press('Escape');

  await openSearch(page, testInfo);
  const m = await rows(page).count();
  expect(m).toBeGreaterThan(5);
  for (let i = 0; i < m; i++) {
    const got = await cells(rows(page).nth(i).getByRole('group').getByRole('button'));
    expect(got.map((c) => c.name)).toEqual(SIX_DECK);
  }
});

test('closing a deck search after taking a card shuffles the deck, unless it was shuffled', async ({ page }, testInfo) => {
  test.skip(isWide(testInfo), 'the views are the same on the landscape mat');
  const shuffles = async () => (await timelineLines(page)).filter((l) => /shuffle/i.test(l)).length;
  const before = await shuffles();
  // Looking without taking anything leaves the deck alone.
  await openSearch(page, testInfo);
  await page.keyboard.press('Escape');
  await expect(view(page)).toHaveCount(0);
  expect(await shuffles()).toBe(before);
  // Taking a card and closing shuffles once.
  await openSearch(page, testInfo);
  await press(rows(page).first().getByRole('group').getByRole('button').first(), testInfo);
  await page.keyboard.press('Escape');
  await expect.poll(shuffles).toBe(before + 1);
  // Taking a card and pressing Shuffle does not shuffle again on close.
  await openSearch(page, testInfo);
  await press(rows(page).first().getByRole('group').getByRole('button').first(), testInfo);
  await press(view(page).getByRole('button', { name: 'Shuffle' }), testInfo);
  await expect.poll(shuffles).toBe(before + 2);
  await page.keyboard.press('Escape');
  await expect(view(page)).toHaveCount(0);
  await page.waitForTimeout(300);
  expect(await shuffles()).toBe(before + 2);
});

test('an action a card cannot take is a disabled cell that says why', async ({ page }, testInfo) => {
  test.skip(isWide(testInfo), 'the views are the same on the landscape mat');
  // The deck search: no Attach from the deck; Bench only for a Pokémon.
  await openSearch(page, testInfo);
  expect(await cells(cellsOf(page, 'Iono'))).toEqual([
    { name: 'Put in hand', disabled: false, title: 'Put in hand' },
    { name: 'Move to the Bench', disabled: true, title: 'Only Pokémon go on the Bench' },
    { name: 'Attach to a Pokémon', disabled: true, title: 'Not from the deck' },
    { name: 'Put on the bottom of the deck', disabled: false, title: 'Put on the bottom of the deck' },
    { name: 'Discard', disabled: false, title: 'Discard' },
    { name: 'Put in the Lost Zone', disabled: false, title: 'Put in the Lost Zone' },
  ]);
  const lucario = await cells(cellsOf(page, 'Lucario'));
  expect(lucario[1]).toEqual({ name: 'Move to the Bench', disabled: false, title: 'Move to the Bench' });
  // A disabled cell does nothing when pressed.
  await expect(cellsOf(page, 'Iono').nth(1)).toBeDisabled();
  await page.keyboard.press('Escape');

  // The discard: never Discard; Attach only for an Energy or a Tool.
  await discardSome(page, ['Ultra Ball', 'Band']);
  await openDiscard(page, testInfo);
  const energy = await cells(cellsOf(page, 'Fighting Energy'));
  expect(energy.filter((c) => c.disabled)).toEqual([
    { name: 'Move to the Bench', disabled: true, title: 'Only Pokémon go on the Bench' },
    { name: 'Discard', disabled: true, title: 'Already in the discard' },
  ]);
  const ball = await cells(cellsOf(page, 'Ultra Ball'));
  expect(ball[2]).toEqual({ name: 'Attach to a Pokémon', disabled: true, title: 'Only Energy and Tools attach' });
  const band = await cells(cellsOf(page, 'Band'));
  expect(band[2]).toEqual({ name: 'Attach to a Pokémon', disabled: false, title: 'Attach to a Pokémon' });
  // A cell still does what its button did: Lost Zone moves the Ultra Ball.
  await press(cellsOf(page, 'Ultra Ball').nth(5), testInfo);
  await expect(view(page).getByRole('group', { name: 'Ultra Ball', exact: true })).toHaveCount(0);
  await page.keyboard.press('Escape');

  // The Lost Zone swaps them: Discard is offered, Lost Zone is not.
  await press(pile(page, 'Lost'), testInfo);
  const lost = await cells(cellsOf(page, 'Ultra Ball'));
  expect(lost.filter((c) => c.disabled).map((c) => c.title)).toEqual(['Only Pokémon go on the Bench', 'Only Energy and Tools attach', 'Already in the Lost Zone']);
  expect(lost[4]).toEqual({ name: 'Discard', disabled: false, title: 'Discard' });
});

test('the key under the header names all six cells for the view', async ({ page }, testInfo) => {
  test.skip(isWide(testInfo), 'the views are the same on the landscape mat');
  const key = page.getByRole('note', { name: 'Key to the action buttons' });
  await openDiscard(page, testInfo);
  await expect(key.locator('.k')).toHaveText(['Hand', 'Bench', 'Attach', 'Deck top', 'Discard', 'Lost Zone']);
  await expect(key.locator('.k svg')).toHaveCount(6);
  await page.keyboard.press('Escape');
  await openSearch(page, testInfo);
  await expect(key.locator('.k')).toHaveText(['Hand', 'Bench', 'Attach', 'Deck bottom', 'Discard', 'Lost Zone']);
  // The key sits under the header, above the rows.
  const k = await key.boundingBox();
  expect(k.y).toBeGreaterThan((await view(page).locator('header').boundingBox()).y);
  expect(k.y).toBeLessThan((await rows(page).first().boundingBox()).y);
});

test('"Hide key" hides the key in every view and is remembered after a reload', async ({ page }, testInfo) => {
  test.skip(isWide(testInfo), 'the views are the same on the landscape mat');
  const key = page.getByRole('note', { name: 'Key to the action buttons' });
  await openDiscard(page, testInfo);
  await press(key.getByRole('button', { name: 'Hide key' }), testInfo);
  await expect(key).toHaveCount(0);
  const show = view(page).getByRole('button', { name: 'Show the key' });
  await expect(show).toHaveAttribute('aria-pressed', 'false');
  expect(await page.evaluate(() => localStorage.getItem('actionKey'))).toBe('off');

  await page.reload();
  await expect(page.locator('header .dot.on')).toBeVisible();
  await openSearch(page, testInfo);
  await expect(key).toHaveCount(0);
  await press(view(page).getByRole('button', { name: 'Show the key' }), testInfo);
  await expect(key).toBeVisible();
  expect(await page.evaluate(() => localStorage.getItem('actionKey'))).toBe('on');
});

test('on a phone the cells are a 2×3 grid of 44px buttons', async ({ page }, testInfo) => {
  test.skip(!isPhone(testInfo), 'the phone');
  await openSearch(page, testInfo);
  const boxes = await cellsOf(page, 'Lucario').evaluateAll((bs) => bs.map((b) => b.getBoundingClientRect().toJSON()));
  for (const b of boxes) expect([b.width, b.height]).toEqual([44, 44]);
  const xs = [...new Set(boxes.map((b) => Math.round(b.x)))];
  const ys = [...new Set(boxes.map((b) => Math.round(b.y)))];
  expect([xs.length, ys.length]).toEqual([3, 2]);
  // Row 1: Hand, Bench, Attach; row 2: Deck, Discard, Lost Zone, in the same columns.
  for (let i = 0; i < 3; i++) {
    expect(Math.round(boxes[i].y)).toBe(ys[0]);
    expect(Math.round(boxes[i + 3].y)).toBe(ys[1]);
    expect(Math.round(boxes[i].x)).toBe(Math.round(boxes[i + 3].x));
  }
  // The key is a grid of the same three columns.
  const key = await page.locator('.key .k').evaluateAll((ks) => ks.map((k) => k.getBoundingClientRect().toJSON()));
  expect(new Set(key.map((k) => Math.round(k.y))).size).toBe(2);
  // Nothing in the view scrolls sideways.
  expect(await page.locator('.plist').evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(true);
});

test('with a mouse on a desktop the cells are one row of six 36px buttons', async ({ page }, testInfo) => {
  test.skip(isTouch(testInfo) || isWide(testInfo), 'the desktop');
  await openDiscard(page, testInfo);
  const boxes = await cellsOf(page, 'Fighting Energy').evaluateAll((bs) => bs.map((b) => b.getBoundingClientRect().toJSON()));
  for (const b of boxes) expect([b.width, b.height]).toEqual([36, 36]);
  expect(new Set(boxes.map((b) => Math.round(b.y))).size).toBe(1);
  const gaps = boxes.slice(1).map((b, i) => Math.round(b.x - boxes[i].x - boxes[i].width));
  // Hand, Bench, Attach, then a wider gap, then Deck top, Discard, Lost Zone.
  expect(gaps).toEqual([4, 4, 16, 4, 4]);
  // The key is one line.
  const key = await page.locator('.key .k').evaluateAll((ks) => ks.map((k) => k.getBoundingClientRect().toJSON()));
  expect(new Set(key.map((k) => Math.round(k.y))).size).toBe(1);
});

test('the discard is a list of rows in the pile\'s own order, top card first and numbered', async ({ page }, testInfo) => {
  test.skip(isWide(testInfo), 'the views are the same on the landscape mat');
  // Fighting Energy is already in the discard; these go on top of it.
  await discardSome(page, ['Iono', 'Ultra Ball', 'Lucario']);
  await openDiscard(page, testInfo);
  await expect(view(page).locator('h2')).toHaveText('p1 discard · 4');
  await expect(view(page).locator('.note')).toHaveText('Top of the discard first (1 is the card on top). Cards keep the order they were discarded in.');
  await expect(rows(page).locator('.nm')).toHaveText(['Lucario', 'Ultra Ball', 'Iono', 'Fighting Energy']);
  await expect(rows(page).locator('.cat')).toHaveText([/^1 · Stage1/, '2 · Item', '3 · Supporter', '4 · Basic Energy']);
  // Each row is the card, then its name over its category, then the cells.
  const row = rows(page).first();
  const [card, name, group] = await Promise.all([row.locator('.card').boundingBox(), row.locator('.nm').boundingBox(), row.getByRole('group').boundingBox()]);
  expect(card.x).toBeLessThan(name.x);
  expect(name.x + name.width).toBeLessThanOrEqual(group.x);
  // Tapping the card's name opens its moves in words, as the card does.
  await press(row.locator('.ptxt'), testInfo);
  await expect(page.locator('.sheet')).toHaveAttribute('aria-label', 'Lucario');
});
