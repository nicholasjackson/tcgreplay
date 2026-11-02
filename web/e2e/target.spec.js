// Choosing a target on the board: an action that needs a destination
// closes the sheet, names itself in a banner, lights its candidates and
// waits for a tap (touch profiles) or a click (desktop) on one of them.
import { test, expect, act, isTouch, timelineLines } from './fixtures.js';

const hand = (page) => page.getByRole('list', { name: 'p1 hand' });
const region = (page, name) => page.getByRole('region', { name, exact: true });
const titled = (scope, title) => scope.locator(`.card[title="${title}"]`);
const sheet = (page) => page.locator('.sheet');
const banner = (page) => page.getByRole('region', { name: 'Choose a target' });
const item = (page, name) => sheet(page).getByRole('button', { name, exact: true });

// open taps a card (touch) or right-clicks it (desktop) for its actions.
async function open(page, target, testInfo) {
  if (isTouch(testInfo)) await target.tap();
  else await target.click({ button: 'right' });
  await expect(sheet(page)).toBeVisible();
}

// press taps (touch) or clicks (desktop).
async function press(target, testInfo) {
  if (isTouch(testInfo)) await target.tap();
  else await target.click();
}

// expectNewLines waits for the timeline to grow by exactly these lines.
async function expectNewLines(page, before, lines) {
  await expect.poll(async () => (await timelineLines(page)).slice(before.length)).toEqual(lines);
}

// expectUnchanged checks, after a moment for any verb to land, that the
// timeline is as it was and target mode has ended.
async function expectUnchanged(page, before) {
  await page.waitForTimeout(300);
  expect(await timelineLines(page)).toEqual(before);
  await expect(banner(page)).toHaveCount(0);
  await expect(page.locator('.card.lit')).toHaveCount(0);
}

const energy = (page) => titled(hand(page), 'Fighting Energy (p1-18)');
// p1's Pokémon in play: the Active Makuhita and the benched Riolu and Makuhita.
const own = ['Makuhita (p1-5)', 'Riolu (p1-1)', 'Makuhita (p1-6)'];
const lit = (page) => page.locator('.card.lit').evaluateAll((cs) => cs.map((c) => c.title).sort());

test('Attach for an Energy names it, lights every own Pokémon and attaches to the one tapped', async ({ page }, testInfo) => {
  const before = await timelineLines(page);
  await open(page, energy(page), testInfo);
  await press(item(page, 'Attach'), testInfo);

  await expect(sheet(page)).toHaveCount(0);
  await expect(banner(page)).toContainText('Attach Fighting Energy');
  await expect(banner(page)).toContainText('Tap one of your Pokémon');
  expect(await lit(page)).toEqual([...own].sort());
  await expect(energy(page)).toHaveClass(/\btarget-source\b/);

  await press(titled(region(page, 'p1 bench 1'), 'Riolu (p1-1)'), testInfo);
  await expect(titled(region(page, 'p1 bench 1'), 'Fighting Energy (p1-18)')).toBeVisible();
  await expectNewLines(page, before, ['p1 moved p1-18 Fighting Energy p1:hand → p1:bench:0 as energy']);
  await expect(banner(page)).toHaveCount(0);
  await expect(page.locator('.card.lit')).toHaveCount(0);
});

test('Cancel attaches nothing and adds nothing to the timeline', async ({ page }, testInfo) => {
  const before = await timelineLines(page);
  await open(page, energy(page), testInfo);
  await press(item(page, 'Attach'), testInfo);
  await press(banner(page).getByRole('button', { name: 'Cancel' }), testInfo);
  await expectUnchanged(page, before);
  await expect(energy(page)).toBeVisible();
});

test('a tap on anything but a candidate cancels and does nothing else', async ({ page }, testInfo) => {
  const before = await timelineLines(page);
  await open(page, energy(page), testInfo);
  await press(item(page, 'Attach'), testInfo);
  // The opponent's Active is not a candidate: the tap neither attaches to
  // it nor opens its actions.
  await press(titled(region(page, 'p2 Active'), 'Makuhita (p2-5)'), testInfo);
  await expectUnchanged(page, before);
  await expect(sheet(page)).toHaveCount(0);
  await expect(page.locator('.zoom')).toHaveCount(0);
});

test('Escape cancels', async ({ page }, testInfo) => {
  const before = await timelineLines(page);
  await open(page, energy(page), testInfo);
  await press(item(page, 'Attach'), testInfo);
  await page.keyboard.press('Escape');
  await expectUnchanged(page, before);
});

// Each destination action: the card it starts from, the item, the
// Pokémon it lights (today's menus listed the same ones), the one tapped
// and what the board shows after.
const actions = [
  {
    name: 'Evolve',
    from: (page) => titled(hand(page), 'Lucario (p1-3)'),
    item: 'Evolve',
    lit: own,
    tap: (page) => titled(region(page, 'p1 bench 1'), 'Riolu (p1-1)'),
    lines: ['p1 moved p1-3 Lucario p1:hand → p1:bench:0'],
    check: async (page) => {
      await expect(titled(region(page, 'p1 bench 1'), 'Lucario (p1-3)')).toBeVisible();
      await expect(region(page, 'p1 bench 1').locator('.chain')).toHaveText('2');
    },
  },
  {
    name: 'a Tool attach',
    from: (page) => titled(hand(page), 'Band (p1-9)'),
    item: 'Attach',
    lit: own,
    tap: (page) => titled(region(page, 'p1 Active'), 'Makuhita (p1-5)'),
    lines: ['p1 moved p1-9 Band p1:hand → p1:active as tool'],
    check: (page) => expect(titled(region(page, 'p1 Active'), 'Band (p1-9)')).toBeVisible(),
  },
  {
    name: 'Retreat',
    from: (page) => titled(region(page, 'p1 Active'), 'Makuhita (p1-5)'),
    item: 'Retreat',
    lit: ['Makuhita (p1-6)', 'Riolu (p1-1)'],
    tap: (page) => titled(region(page, 'p1 bench 2'), 'Makuhita (p1-6)'),
    lines: [
      // Makuhita's Retreat Cost of 2 discards the one Energy it has.
      'p1 moved p1-15 Fighting Energy p1:active → p1:discard',
      'p1 swapped p1:bench:1 (Makuhita) with p1:active (Makuhita)',
    ],
    check: (page) => expect(titled(region(page, 'p1 Active'), 'Makuhita (p1-6)')).toBeVisible(),
  },
  {
    name: 'Switch, no cost',
    from: (page) => titled(region(page, 'p1 Active'), 'Makuhita (p1-5)'),
    item: 'Switch, no cost',
    lit: ['Makuhita (p1-6)', 'Riolu (p1-1)'],
    tap: (page) => titled(region(page, 'p1 bench 1'), 'Riolu (p1-1)'),
    lines: ['p1 swapped p1:bench:0 (Riolu) with p1:active (Makuhita)'],
    check: (page) => expect(titled(region(page, 'p1 Active'), 'Riolu (p1-1)')).toBeVisible(),
  },
  {
    name: 'moving an attached Energy',
    from: (page) => titled(region(page, 'p1 Active'), 'Fighting Energy (p1-15)'),
    item: 'Move to',
    lit: own,
    tap: (page) => titled(region(page, 'p1 bench 2'), 'Makuhita (p1-6)'),
    lines: ['p1 moved p1-15 Fighting Energy p1:active → p1:bench:1 as energy'],
    check: (page) => expect(titled(region(page, 'p1 bench 2'), 'Fighting Energy (p1-15)')).toBeVisible(),
  },
];

for (const a of actions) {
  test(`${a.name} picks its destination on the board`, async ({ page }, testInfo) => {
    const before = await timelineLines(page);
    await open(page, a.from(page), testInfo);
    await press(item(page, a.item), testInfo);
    await expect(banner(page)).toBeVisible();
    expect(await lit(page)).toEqual([...a.lit].sort());
    await press(a.tap(page), testInfo);
    await a.check(page);
    await expectNewLines(page, before, a.lines);
    await expect(banner(page)).toHaveCount(0);
  });
}

test('with one benched Pokémon, Retreat names it and retreats at once', async ({ page }, testInfo) => {
  await act(page, 'move', { iid: 'p1-6', to: 'hand' });
  await expect(region(page, 'p1 bench 2').locator('.card')).toHaveCount(0);
  const before = await timelineLines(page);
  await open(page, titled(region(page, 'p1 Active'), 'Makuhita (p1-5)'), testInfo);
  await expect(item(page, 'Retreat')).toHaveCount(0);
  await press(item(page, 'Retreat to Riolu'), testInfo);
  await expect(banner(page)).toHaveCount(0);
  await expect(titled(region(page, 'p1 Active'), 'Riolu (p1-1)')).toBeVisible();
  await expectNewLines(page, before, [
    'p1 moved p1-15 Fighting Energy p1:active → p1:discard',
    'p1 swapped p1:bench:0 (Riolu) with p1:active (Makuhita)',
  ]);
});

test('with a full bench, attaching an Energy to a benched Pokémon takes three taps', async ({ page }, testInfo) => {
  test.skip(!isTouch(testInfo), 'counted in taps');
  for (const [iid, to] of [['p1-2', 'bench:2'], ['p1-3', 'bench:3'], ['p1-4', 'bench:4']]) await act(page, 'move', { iid, to });
  await expect(region(page, 'p1 bench 5').locator('.card')).toHaveCount(1);

  await energy(page).tap(); // 1: the Energy
  await item(page, 'Attach').tap(); // 2: Attach
  await titled(region(page, 'p1 bench 5'), 'Lucario (p1-4)').tap(); // 3: the Pokémon
  await expect(titled(region(page, 'p1 bench 5'), 'Fighting Energy (p1-18)')).toBeVisible();
});
