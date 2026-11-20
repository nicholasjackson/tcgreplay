// A card's actions in one grouped sheet: tapped open as the full-screen
// phone sheet or the tablet panel, right-clicked open as the desktop
// popover. Every container is held to the same hand-written menus.
// Attack items, Draw N… and Look at top N… still ask with a browser
// dialog, which fails the test, so they are listed but never pressed.
import { test, expect, isTouch, touchStart, pile } from './fixtures.js';

const hand = (page) => page.getByRole('list', { name: 'p1 hand' });
const region = (page, name) => page.getByRole('region', { name, exact: true });
const titled = (scope, title) => scope.locator(`.card[title="${title}"]`);
const sheet = (page) => page.locator('.sheet');
const group = (page, name) => sheet(page).locator(`section[role="group"][aria-label="${name}"]`);
const button = (page, name) => sheet(page).getByRole('button', { name, exact: true });

const active = (page) => titled(region(page, 'p1 Active'), 'Makuhita (p1-5)');

// openActions taps a card (touch profiles) or right-clicks it (desktop).
async function openActions(page, target, testInfo) {
  if (isTouch(testInfo)) await target.tap();
  else await target.click({ button: 'right' });
  await expect(sheet(page)).toBeVisible();
  await expect(sheet(page)).toHaveAttribute('data-mode', isTouch(testInfo) ? testInfo.project.name : 'popover');
}

// activate taps or clicks a control in the sheet.
async function activate(control, testInfo) {
  if (isTouch(testInfo)) await control.tap();
  else await control.click();
}

// menuOf reads the open sheet as [group, [controls]]: an item is its label
// (with " · detail" when it has one, and " ›" when it goes on to a target
// on the board), a stepper its buttons and input.
function menuOf(page) {
  return sheet(page).evaluate((el) =>
    [...el.querySelectorAll('section[role="group"]')].map((sec) => [
      sec.getAttribute('aria-label'),
      [...sec.querySelectorAll('button, input')].map((c) => {
        if (c.tagName === 'INPUT') return c.getAttribute('aria-label');
        const detail = c.querySelector('.detail');
        if (detail) return `${c.firstElementChild.textContent} · ${detail.textContent}`;
        return c.querySelector('.go') ? `${c.firstElementChild.textContent} ›` : c.textContent.trim();
      }),
    ]),
  );
}

// expectCardAndText checks the touch sheet shows the card and, asked,
// its text: behind "Card text ›" on the phone, the Text switch on a tablet.
async function expectCardAndText(page, title, text, testInfo) {
  if (testInfo.project.name === 'phone') {
    await expect(titled(sheet(page).locator('header'), title)).toBeVisible();
    const link = button(page, 'Card text ›');
    await expect(link).toHaveAttribute('aria-expanded', 'false');
    await link.tap();
    await expect(sheet(page).locator('pre.text')).toContainText(text);
    await expect(sheet(page).locator('section[role="group"]')).toHaveCount(0);
    await expect(button(page, 'Actions ›')).toHaveAttribute('aria-expanded', 'true');
  } else {
    await expect(titled(sheet(page).locator('.side'), title)).toBeVisible();
    await expect(button(page, 'Card')).toHaveAttribute('aria-pressed', 'true');
    await button(page, 'Text').tap();
    await expect(button(page, 'Text')).toHaveAttribute('aria-pressed', 'true');
    await expect(sheet(page).locator('.side pre.text')).toContainText(text);
    // The actions stay beside the text.
    await expect(sheet(page).locator('section[role="group"]').first()).toBeVisible();
  }
}

// Each card a tap opens: the card, the sheet's title and its text.
const taps = [
  { kind: 'a card in hand', target: (page) => titled(hand(page), 'Riolu (p1-2)'), title: 'Riolu (p1-2)', name: 'Riolu', text: 'Riolu (tst-1)' },
  { kind: 'a Pokémon in play', target: active, title: 'Makuhita (p1-5)', name: 'Makuhita', text: 'Makuhita (tst-7)' },
  {
    kind: 'an attached card',
    target: (page) => titled(region(page, 'p1 Active'), 'Fighting Energy (p1-15)'),
    title: 'Fighting Energy (p1-15)',
    name: 'Fighting Energy',
    text: 'Fighting Energy (sve-006)',
  },
];

for (const t of taps) {
  test(`tapping ${t.kind} opens its actions with the card and its text`, async ({ page }, testInfo) => {
    test.skip(!isTouch(testInfo), 'tapping is touch');
    await openActions(page, t.target(page), testInfo);
    await expect(page.getByRole('dialog', { name: t.name })).toHaveClass(/\bsheet\b/);
    await expect(page.locator('.zoom')).toHaveCount(0);
    await expectCardAndText(page, t.title, t.text, testInfo);
  });
}

test('tapping the deck opens its actions with the deck and its text', async ({ page }, testInfo) => {
  test.skip(!isTouch(testInfo), 'tapping is touch');
  await openActions(page, pile(page, 'Deck'), testInfo);
  await expect(page.getByRole('dialog', { name: 'Your deck' })).toHaveClass(/\bsheet\b/);
  await expect(group(page, 'Draw')).toBeVisible();
  // The deck shows as its back (titled by its iid, "deck"); its text says
  // how many cards are left.
  await expectCardAndText(page, 'deck', '40 cards left, face down', testInfo);
});

test('a tap whose click calls itself a mouse (iOS Safari) still opens the touch sheet', async ({ page }, testInfo) => {
  test.skip(!isTouch(testInfo), 'touch screens');
  await active(page).evaluate((el) => {
    const at = { bubbles: true, cancelable: true, composed: true, clientX: 10, clientY: 10 };
    el.dispatchEvent(new PointerEvent('pointerdown', { ...at, pointerType: 'touch', isPrimary: true }));
    el.dispatchEvent(new PointerEvent('pointerup', { ...at, pointerType: 'touch', isPrimary: true }));
    el.dispatchEvent(new PointerEvent('click', { ...at, pointerType: 'mouse' }));
  });
  await expect(sheet(page)).toHaveAttribute('data-mode', testInfo.project.name);
});

test('tapping a card in a pile view opens its moves with the card and its text', async ({ page }, testInfo) => {
  test.skip(!isTouch(testInfo), 'tapping is touch');
  await pile(page, 'Discard').tap();
  const view = page.locator('.modal');
  await titled(view, 'Fighting Energy (p1-16)').tap();
  await expect(sheet(page)).toHaveAttribute('aria-label', 'Fighting Energy');
  // The sheet mirrors the view's buttons for the card.
  expect(await menuOf(page)).toEqual([['Move', ['Hand', 'Attach', 'Deck', 'Lost']]]);
  await expect(view.locator('.acts button')).toHaveText(['Hand', 'Attach', 'Deck', 'Lost']);
  await expectCardAndText(page, 'Fighting Energy (p1-16)', 'Fighting Energy (sve-006)', testInfo);
});

test('on a phone, the sheet covers the screen and an overflowing group scrolls sideways', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== 'phone', 'the full-screen sheet is the phone');
  await openActions(page, active(page), testInfo);
  expect(await sheet(page).boundingBox()).toEqual({ x: 0, y: 0, width: 390, height: 844 });

  // The five condition toggles are wider than the screen.
  const row = group(page, 'Conditions').locator('.row');
  const [scrollWidth, clientWidth] = await row.evaluate((r) => [r.scrollWidth, r.clientWidth]);
  expect(scrollWidth).toBeGreaterThan(clientWidth);
  await expect(row).toHaveClass(/\bmore\b/);

  const box = await row.boundingBox();
  const at = { x: box.x + box.width - 40, y: box.y + box.height / 2 };
  const f = await touchStart(page, at);
  await f.move({ x: at.x - 200, y: at.y }, { steps: 10 });
  await f.end();
  await expect.poll(() => row.evaluate((r) => r.scrollLeft)).toBeGreaterThan(0);

  // At the far end nothing more is off screen and the fade goes.
  await row.evaluate((r) => r.scrollTo({ left: r.scrollWidth }));
  await expect(row).not.toHaveClass(/\bmore\b/);
  // A group that fits shows no fade.
  await expect(group(page, 'Attack').locator('.row')).not.toHaveClass(/\bmore\b/);
});

test('on a tablet, the card sits left of the actions and only the actions scroll', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== 'tablet', 'the side-by-side panel is the tablet');
  // The Active's actions fit a 768px-tall panel, so a shorter tablet
  // window makes them taller than the panel.
  await page.setViewportSize({ width: 1024, height: 560 });
  await openActions(page, active(page), testInfo);
  await expect(sheet(page)).toHaveAttribute('data-mode', 'tablet');

  const left = await sheet(page).locator('.side').boundingBox();
  const right = await sheet(page).locator('.main').boundingBox();
  expect(left.x + left.width).toBeLessThanOrEqual(right.x + 1);
  await expect(titled(sheet(page).locator('.side'), 'Makuhita (p1-5)')).toBeVisible();
  await expect(sheet(page).locator('.main section[role="group"]').first()).toBeVisible();

  const body = sheet(page).locator('.main .body');
  const [scrollHeight, clientHeight] = await body.evaluate((b) => [b.scrollHeight, b.clientHeight]);
  expect(scrollHeight).toBeGreaterThan(clientHeight);

  const fixed = async () => ({
    sheet: await sheet(page).boundingBox(),
    card: await titled(sheet(page).locator('.side'), 'Makuhita (p1-5)').boundingBox(),
    title: await sheet(page).locator('h3').boundingBox(),
  });
  const before = await fixed();
  const box = await body.boundingBox();
  const at = { x: box.x + box.width / 2, y: box.y + box.height - 40 };
  const f = await touchStart(page, at);
  await f.move({ x: at.x, y: at.y - 250 }, { steps: 10 });
  await f.end();
  await expect.poll(() => body.evaluate((b) => b.scrollTop)).toBeGreaterThan(0);
  expect(await fixed()).toEqual(before);
  expect(await page.evaluate(() => [scrollX, scrollY])).toEqual([0, 0]);
});

test('with a mouse, hovering enlarges a card, clicking shows it with its text and right-clicking opens the popover there', async ({ page }, testInfo) => {
  test.skip(isTouch(testInfo), 'mouse on the desktop');
  const riolu = titled(region(page, 'p1 bench 1'), 'Riolu (p1-1)');
  await riolu.hover();
  await page.waitForTimeout(600);
  const scale = await riolu.evaluate((el) => {
    const m = new DOMMatrix(getComputedStyle(el).transform);
    return Math.hypot(m.a, m.b);
  });
  expect(scale).toBeGreaterThan(1);

  await riolu.click();
  const zoom = page.locator('.zoom');
  await expect(zoom).toBeVisible();
  await expect(zoom).toContainText('Riolu (tst-1)');
  await expect(sheet(page)).toHaveCount(0);
  await page.mouse.click(5, 5);
  await expect(zoom).toHaveCount(0);

  const box = await riolu.boundingBox();
  const at = { x: Math.round(box.x + box.width / 2), y: Math.round(box.y + box.height / 2) };
  await page.mouse.click(at.x, at.y, { button: 'right' });
  await expect(sheet(page)).toHaveAttribute('data-mode', 'popover');
  await expect(sheet(page)).toHaveAttribute('aria-label', 'Riolu');
  const pop = await sheet(page).boundingBox();
  expect(pop.width).toBe(264);
  expect(Math.abs(pop.x - at.x)).toBeLessThanOrEqual(1);
  expect(pop.y).toBeLessThanOrEqual(at.y);
  expect(pop.y + pop.height).toBeGreaterThanOrEqual(at.y);
  // Grouped, and without the card's image.
  await expect(group(page, 'Ability')).toBeVisible();
  await expect(sheet(page).locator('.card')).toHaveCount(0);
});

test('on a touch screen with a mouse too, a tap opens the sheet, a click zooms and a right-click opens the popover', async ({ page }, testInfo) => {
  test.skip(!isTouch(testInfo), 'a touch screen');
  const riolu = titled(region(page, 'p1 bench 1'), 'Riolu (p1-1)');
  const box = await riolu.boundingBox();
  const at = { x: box.x + box.width / 2, y: box.y + box.height / 2 };

  await page.mouse.click(at.x, at.y);
  await expect(page.locator('.zoom')).toBeVisible();
  await expect(sheet(page)).toHaveCount(0);
  await page.mouse.click(5, 5);
  await expect(page.locator('.zoom')).toHaveCount(0);

  await riolu.tap();
  await expect(sheet(page)).toHaveAttribute('data-mode', testInfo.project.name);
  await expect(page.locator('.zoom')).toHaveCount(0);
  await button(page, 'Close').tap();
  await expect(sheet(page)).toHaveCount(0);

  await page.mouse.click(at.x, at.y, { button: 'right' });
  await expect(sheet(page)).toHaveAttribute('data-mode', 'popover');
});

test("an Active Pokémon's groups follow the design's order, with no Ability group when it has none", async ({ page }, testInfo) => {
  await openActions(page, active(page), testInfo);
  expect((await menuOf(page)).map(([label]) => label)).toEqual(['Attack', 'Move', 'Damage', 'Conditions', 'Other']);
  await expect(group(page, 'Ability')).toHaveCount(0);
  // One item per attack, its damage beside it.
  await expect(group(page, 'Attack').locator('.item')).toHaveText(['Slap 20', 'Body Slam 60']);
  await expect(group(page, 'Attack').locator('.detail')).toHaveText(['20', '60']);
  // Knock Out is the last action, marked dangerous.
  const last = sheet(page).locator('section[role="group"]').last().locator('button').last();
  await expect(last).toHaveText('Knock Out');
  await expect(last).toHaveClass(/\bdanger\b/);
});

test('a benched Pokémon with an Ability shows it before its moves', async ({ page }, testInfo) => {
  await openActions(page, titled(region(page, 'p1 bench 1'), 'Riolu (p1-1)'), testInfo);
  expect((await menuOf(page)).map(([label]) => label)).toEqual(['Ability', 'Move', 'Damage', 'Other']);
});

test('the common action comes first', async ({ page }, testInfo) => {
  await openActions(page, titled(hand(page), 'Riolu (p1-2)'), testInfo);
  const first = sheet(page).locator('section[role="group"]').first();
  await expect(first).toHaveAttribute('aria-label', 'Play');
  await expect(first.locator('.item').first()).toHaveText('Bench');
  await expect(sheet(page).locator('.item.primary')).toHaveText(['Bench']);
  await page.keyboard.press('Escape');
  await expect(sheet(page)).toHaveCount(0);

  await openActions(page, pile(page, 'Deck'), testInfo);
  await expect(sheet(page).locator('.item.primary')).toHaveText(['Draw 1']);
});

test('damage is raised by 10, 30 or 50, lowered by 10 or typed, and the board shows it', async ({ page }, testInfo) => {
  await openActions(page, active(page), testInfo);
  const total = sheet(page).getByRole('textbox', { name: 'Damage total' });
  const hpLeft = region(page, 'p1 Active').locator('.hpleft');
  await expect(total).toHaveValue('0');
  await expect(hpLeft).toHaveText('80/80');

  // Makuhita has 80 HP.
  const steps = [
    ['+10', '10', '70/80'],
    ['+30', '40', '40/80'],
    ['−10', '30', '50/80'],
    ['+50', '80', '0/80'],
  ];
  for (const [step, value, left] of steps) {
    await activate(group(page, 'Damage').getByRole('button', { name: step, exact: true }), testInfo);
    await expect(total).toHaveValue(value);
    await expect(hpLeft).toHaveText(left);
  }

  // A typed total lands on Enter or when the field loses focus.
  await total.fill('20');
  await total.press('Enter');
  await expect(hpLeft).toHaveText('60/80');
  await expect(total).toHaveValue('20');
  await total.fill('50');
  await total.blur();
  await expect(hpLeft).toHaveText('30/80');
  await expect(total).toHaveValue('50');
});

// Each Special Condition and how the board shows it on the Active.
const conditions = [
  ['Asleep', (page) => expect(active(page)).toHaveAttribute('style', /--rot:\s*-90deg/)],
  ['Burned', (page) => expect(region(page, 'p1 Active').locator('.marker[title="Burned"]')).toBeVisible()],
  ['Confused', (page) => expect(active(page)).toHaveAttribute('style', /--rot:\s*180deg/)],
  ['Paralyzed', (page) => expect(active(page)).toHaveAttribute('style', /--rot:\s*90deg/)],
  ['Poisoned', (page) => expect(region(page, 'p1 Active').locator('.marker[title="Poisoned"]')).toBeVisible()],
];

test('each Special Condition toggles on and off with one tap and shows its state', async ({ page }, testInfo) => {
  await openActions(page, active(page), testInfo);
  await expect(group(page, 'Conditions').getByRole('button')).toHaveText(conditions.map(([name]) => name));
  for (const [name, shown] of conditions) {
    // A toggle that is on reads "✓ Asleep" on touch screens.
    const toggle = group(page, 'Conditions').getByRole('button', { name: new RegExp(`^(✓ )?${name}$`) });
    await expect(toggle).toHaveAttribute('aria-pressed', 'false');
    await activate(toggle, testInfo);
    await expect(toggle).toHaveAttribute('aria-pressed', 'true');
    await shown(page);

    await activate(toggle, testInfo);
    await expect(toggle).toHaveAttribute('aria-pressed', 'false');
    await expect(region(page, 'p1 Active').locator('.marker')).toHaveCount(0);
    await expect(active(page)).toHaveAttribute('style', /--rot:\s*0deg/);
  }
});

// The menus every container offers, one list per kind of card. Today's
// menus, renamed: "To deck (top)/(bottom)" became "Deck top"/"Deck bottom";
// "+10/+30/+50/−10 damage" and "Set damage…" the Damage stepper; "Set/Clear
// <condition>" the Conditions toggles; "Return X to hand" and "Discard X
// (devolve)" became "Return to hand" and "Devolve"; "Switch into
// Active/Promote" is "Switch into Active" with a Pokémon in the Active
// Spot; "Zoom" went (a mouse click or the card's text replaces it). The
// items listed once per Pokémon ("Attach to Bench 1 (Riolu)", "Evolve …",
// "Retreat to …", "Switch with … (no cost)", "Move to …") became one item
// each, finished by tapping the Pokémon on the board.
const moveFromHand = ['Move', ['Discard', 'Deck top', 'Deck bottom', 'Lost Zone']];
const reveal = ['Other', ['Reveal to opponent']];
const damage = ['Damage', ['−10', 'Damage total', '+10', '+30', '+50']];
const devolve = ['Other', ['Return to hand', 'Devolve', 'Knock Out']];

const kinds = [
  {
    kind: 'a Pokémon in hand',
    open: (page) => titled(hand(page), 'Riolu (p1-2)'),
    menu: [['Play', ['Bench', 'Evolve ›']], moveFromHand, reveal],
  },
  {
    kind: 'an Energy in hand',
    open: (page) => titled(hand(page), 'Fighting Energy (p1-18)'),
    menu: [['Play', ['Attach ›']], moveFromHand, reveal],
  },
  {
    kind: 'a Tool in hand',
    open: (page) => titled(hand(page), 'Band (p1-9)'),
    menu: [['Play', ['Attach ›']], moveFromHand, reveal],
  },
  {
    kind: 'a Trainer in hand',
    open: (page) => titled(hand(page), 'Iono (p1-7)'),
    menu: [['Play', ['Play']], moveFromHand, reveal],
  },
  {
    kind: 'the Active Pokémon',
    open: active,
    menu: [
      ['Attack', ['Slap · 20', 'Body Slam · 60']],
      ['Move', ['Retreat ›', 'Switch, no cost ›']],
      damage,
      ['Conditions', ['Asleep', 'Burned', 'Confused', 'Paralyzed', 'Poisoned']],
      devolve,
    ],
  },
  {
    kind: 'a benched Pokémon',
    open: (page) => titled(region(page, 'p1 bench 1'), 'Riolu (p1-1)'),
    menu: [['Ability', ['Focus']], ['Move', ['Switch into Active']], damage, devolve],
  },
  {
    kind: 'an attached Energy',
    open: (page) => titled(region(page, 'p1 Active'), 'Fighting Energy (p1-15)'),
    menu: [['Move', ['Move to ›', 'To hand', 'Discard']]],
  },
  {
    kind: 'the deck',
    open: (page) => pile(page, 'Deck'),
    menu: [
      ['Draw', ['Draw 1', 'Draw N…']],
      ['Look', ['Look at top N…', 'Search…']],
      ['Other', ['Deck list…', 'Shuffle']],
    ],
  },
];

for (const k of kinds) {
  test(`${k.kind} offers the same actions by touch and by mouse`, async ({ page }, testInfo) => {
    await openActions(page, k.open(page), testInfo);
    expect(await menuOf(page)).toEqual(k.menu);
    await expect(sheet(page)).not.toContainText('Zoom');
  });
}

test('a card in a pile view offers the same moves by mouse', async ({ page }, testInfo) => {
  test.skip(isTouch(testInfo), 'the touch sheet is checked with the pile view tap');
  await pile(page, 'Discard').click();
  await titled(page.locator('.modal'), 'Fighting Energy (p1-16)').click({ button: 'right' });
  await expect(sheet(page)).toHaveAttribute('data-mode', 'popover');
  expect(await menuOf(page)).toEqual([['Move', ['Hand', 'Attach', 'Deck', 'Lost']]]);
});

test('the sheet closes with ✕, Escape and Back, and the tablet panel with its backdrop', async ({ page }, testInfo) => {
  test.skip(!isTouch(testInfo), 'the touch sheet');
  const open = () => openActions(page, active(page), testInfo);
  await open();
  await button(page, 'Close').tap();
  await expect(sheet(page)).toHaveCount(0);

  await open();
  await page.keyboard.press('Escape');
  await expect(sheet(page)).toHaveCount(0);

  await open();
  await page.goBack();
  await expect(sheet(page)).toHaveCount(0);
  await expect(page.locator('header .dot.on')).toBeVisible();

  if (testInfo.project.name === 'tablet') {
    await open();
    await page.touchscreen.tap(20, 20);
    await expect(sheet(page)).toHaveCount(0);
  }
});

test('on a phone, a swipe down on the header closes the sheet', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== 'phone', 'the phone sheet');
  await openActions(page, active(page), testInfo);
  const title = sheet(page).locator('h3');
  const f = await touchStart(page, title);
  const box = await title.boundingBox();
  await f.move({ x: box.x + box.width / 2, y: box.y + 150 }, { steps: 8 });
  await f.end();
  await expect(sheet(page)).toHaveCount(0);
});
