// The menus in the Studio look: the popover a right-click opens, the
// panel a tap opens on a tablet or touch PC and the full-screen sheet on a
// phone, each painted in the Studio colours; the attack panel's Knock Out
// hint; Card text for a card in hand; and the target banner, the deck menu
// and the count panels restyled in the same family. Colours are checked
// against the light theme's Studio tokens (internal/design/tokens.json),
// written out here by hand.
import { test, expect, act, isTouch, isPhone, timelineLines, pile } from './fixtures.js';

const STUDIO = {
  panelRaised: '#FFFFFF', // panel-raised
  panelSunk: '#F6F7F9', // panel-sunk
  primary: '#13285C', // primary
  onPrimary: '#FFFFFF', // on-primary
  danger: '#B42318', // danger
  dangerBg: '#FEF3F2', // danger-bg
  dangerLine: '#F5C2BC', // danger-line
  scrim: '#0A0C108C', // scrim
  controlLine: '#D5DAE1', // control-line
  ink: '#14171C', // ink
};
const MONO = /^"IBM Plex Mono"/;

const hand = (page) => page.getByRole('list', { name: 'p1 hand' });
const region = (page, name) => page.getByRole('region', { name, exact: true });
const titled = (scope, title) => scope.locator(`.card[title="${title}"]`);
const sheet = (page) => page.locator('.sheet');
const group = (page, name) => sheet(page).locator(`section[role="group"][aria-label="${name}"]`);
const button = (page, name) => sheet(page).getByRole('button', { name, exact: true });
const banner = (page) => page.getByRole('region', { name: 'Choose a target' });
const active = (page) => titled(region(page, 'p1 Active'), 'Makuhita (p1-5)');
const ko = (page) => sheet(page).locator('p.ko');

// mode is the sheet a tap or right-click opens in this project.
const mode = (testInfo) => (isTouch(testInfo) ? testInfo.project.name : 'popover');

// open taps a card (touch) or right-clicks it (mouse) for its actions.
async function open(page, target, testInfo) {
  if (isTouch(testInfo)) await target.tap();
  else await target.click({ button: 'right' });
  await expect(sheet(page)).toBeVisible();
  await expect(sheet(page)).toHaveAttribute('data-mode', mode(testInfo));
}

// press taps (touch) or clicks (mouse).
async function press(target, testInfo) {
  if (isTouch(testInfo)) await target.tap();
  else await target.click();
}

async function expectNewLines(page, before, lines) {
  await expect.poll(async () => (await timelineLines(page)).slice(before.length)).toEqual(lines);
}

// style reads an element's computed style, its colours as upper-case hex
// (with the alpha byte when it is not opaque).
function style(locator) {
  return locator.evaluate((el) => {
    const s = getComputedStyle(el);
    const hex = (v) => {
      const m = /rgba?\((\d+),\s*(\d+),\s*(\d+)(?:,\s*([\d.]+))?/.exec(v || '');
      if (!m) return v;
      const bytes = m.slice(1, 4).map(Number);
      if (m[4] !== undefined && Number(m[4]) < 1) bytes.push(Math.round(Number(m[4]) * 255));
      return '#' + bytes.map((n) => n.toString(16).padStart(2, '0')).join('').toUpperCase();
    };
    return {
      background: hex(s.backgroundColor),
      color: hex(s.color),
      borderColor: hex(s.borderTopColor),
      borderWidth: s.borderTopWidth,
      borderStyle: s.borderTopStyle,
      borderLeftColor: hex(s.borderLeftColor),
      borderLeftWidth: s.borderLeftWidth,
      radius: s.borderTopLeftRadius,
      fontFamily: s.fontFamily,
      textTransform: s.textTransform,
    };
  });
}

// expectStudioSheet checks the open sheet's chrome for its mode: the
// popover's 264px raised panel with a mono summary, the tablet's 900×660
// panel with its sunken card column, Card/Text switch, 44px Close and
// scrim, or the phone's full-screen sheet. Every mode's group labels are
// uppercase.
async function expectStudioSheet(page, testInfo) {
  const s = await style(sheet(page));
  expect(s.background).toBe(STUDIO.panelRaised);
  expect(s.color).toBe(STUDIO.ink);
  for (const label of await sheet(page).locator('section[role="group"] > .label').all()) {
    expect((await style(label)).textTransform).toBe('uppercase');
  }
  const m = mode(testInfo);
  if (m === 'popover') {
    expect((await sheet(page).boundingBox()).width).toBe(264);
    expect(s.radius).toBe('10px'); // radius-m
    expect((await style(sheet(page).locator('.psum'))).fontFamily).toMatch(MONO);
    // No backdrop is painted behind the popover.
    await expect(page.locator('.backdrop.tablet')).toHaveCount(0);
    return;
  }
  const close = await button(page, 'Close').boundingBox();
  expect(close.width).toBeGreaterThanOrEqual(44);
  expect(close.height).toBeGreaterThanOrEqual(44);
  const c = await style(button(page, 'Close'));
  expect([c.borderWidth, c.borderStyle, c.borderColor]).toEqual(['1px', 'solid', STUDIO.controlLine]);
  if (m === 'phone') {
    expect(await sheet(page).boundingBox()).toEqual({ x: 0, y: 0, width: 390, height: 844 });
    return;
  }
  // The tablet panel, in a 1024×768 window.
  const box = await sheet(page).boundingBox();
  expect([box.width, box.height]).toEqual([900, 660]);
  expect(s.radius).toBe('16px'); // radius-xl
  expect((await style(page.locator('.backdrop.tablet'))).background).toBe(STUDIO.scrim);
  const side = sheet(page).locator('.side');
  expect((await side.boundingBox()).width).toBe(320);
  expect((await style(side)).background).toBe(STUDIO.panelSunk);
  const on = sheet(page).locator('.seg button[aria-pressed="true"]');
  await expect(on).toHaveText('Card');
  const seg = await style(on);
  expect([seg.background, seg.color]).toEqual([STUDIO.primary, STUDIO.onPrimary]);
  const off = await style(sheet(page).locator('.seg button[aria-pressed="false"]'));
  expect(off.background).toBe(STUDIO.panelRaised);
  // The switch follows the choice.
  await button(page, 'Text').tap();
  const text = await style(button(page, 'Text'));
  expect([text.background, text.color]).toEqual([STUDIO.primary, STUDIO.onPrimary]);
  await button(page, 'Card').tap();
}

// ---- Criterion 1: each container in the Studio look.

test('a card in hand opens its menu in the Studio look for the pointer used', async ({ page }, testInfo) => {
  await open(page, titled(hand(page), 'Riolu (p1-2)'), testInfo);
  await expectStudioSheet(page, testInfo);
  // The summary names where the card is, its kind and its HP.
  if (mode(testInfo) === 'popover') await expect(sheet(page).locator('.psum')).toHaveText('Hand · Basic Pokémon · 70 HP');
  else await expect(sheet(page).locator('header .sum')).toHaveText('Hand · Basic Pokémon · 70 HP');
  // Bench, the common action, is the filled primary button.
  const bench = await style(button(page, 'Bench'));
  expect([bench.background, bench.color]).toEqual([STUDIO.primary, STUDIO.onPrimary]);
});

test('your Active opens its menu in the Studio look, Knock Out last in the danger colour', async ({ page }, testInfo) => {
  await open(page, active(page), testInfo);
  await expectStudioSheet(page, testInfo);
  if (mode(testInfo) === 'popover') {
    await expect(sheet(page).locator('.psum')).toHaveText('Active · 80/80 HP · 1 Energy');
  } else {
    await expect(sheet(page).locator('header .sum')).toHaveText('Active · 80/80 HP · 1 Energy');
    expect((await style(sheet(page).locator('header .facts'))).fontFamily).toMatch(MONO);
  }
  // The attacks: cost dots before the name, the damage in mono.
  const attack = group(page, 'Attack');
  await expect(attack.locator('.item .lbl')).toHaveText(['Slap', 'Body Slam']);
  await expect(attack.locator('.item').nth(0).locator('.cost .dot')).toHaveCount(1);
  await expect(attack.locator('.item').nth(1).locator('.cost .dot')).toHaveCount(2);
  expect((await style(attack.locator('.detail').first())).fontFamily).toMatch(MONO);
  // A move that ends on the board shows the target ring.
  await expect(group(page, 'Move').locator('.item .go svg')).toHaveCount(2);

  const last = sheet(page).locator('section[role="group"]').last().locator('button').last();
  await expect(last).toHaveText('Knock Out');
  const k = await style(last);
  expect(k.color).toBe(STUDIO.danger);
  if (mode(testInfo) !== 'popover') {
    // On touch, Knock Out is an outlined button tinted for danger.
    expect([k.background, k.borderColor]).toEqual([STUDIO.dangerBg, STUDIO.dangerLine]);
  }
});

// ---- Criterion 2: the Knock Out hint.

// openAttack opens your Active's attack panel for the named attack.
async function openAttack(page, name, testInfo) {
  await open(page, active(page), testInfo);
  await press(group(page, 'Attack').getByRole('button', { name: new RegExp(`^${name}`) }), testInfo);
  await expect(sheet(page).getByRole('textbox', { name: 'Attack damage' })).toBeVisible();
}

// setBase types the stepper's base damage.
async function setBase(page, value) {
  const field = sheet(page).getByRole('textbox', { name: 'Attack damage' });
  await field.fill(String(value));
  await field.press('Enter');
  await expect(field).toHaveValue(String(value));
}

test('the Knock Out hint shows once the damage reaches the target\'s HP left, and an attack below it still lands', async ({ page }, testInfo) => {
  const before = await timelineLines(page);
  await openAttack(page, 'Body Slam', testInfo);
  // p2's Active Makuhita has 80 HP and no damage.
  await expect(sheet(page).locator('.target .where')).toHaveText("Bob's Active · 80 HP left");
  await expect(sheet(page).locator('.target').getByRole('button', { name: 'Change ›', exact: true })).toBeVisible();
  await expect(ko(page)).toHaveCount(0);
  await expect(button(page, 'Attack Makuhita for 60')).toBeEnabled();

  await press(button(page, '+10'), testInfo);
  await expect(button(page, 'Attack Makuhita for 70')).toBeVisible();
  await expect(ko(page)).toHaveCount(0);
  await press(button(page, '+10'), testInfo);
  await expect(ko(page)).toHaveText('80 is enough to Knock Out Makuhita.');
  await expect(button(page, 'Attack Makuhita for 80')).toBeEnabled();
  // More than enough says so too.
  await press(button(page, '+10'), testInfo);
  await expect(ko(page)).toHaveText('90 is enough to Knock Out Makuhita.');

  await press(button(page, '−10'), testInfo);
  await press(button(page, '−10'), testInfo);
  await expect(button(page, 'Attack Makuhita for 70')).toBeVisible();
  await expect(ko(page)).toHaveCount(0);
  await press(button(page, '−10'), testInfo);
  await expect(ko(page)).toHaveCount(0);

  await press(button(page, 'Attack Makuhita for 60'), testInfo);
  await expect(sheet(page)).toHaveCount(0);
  await expectNewLines(page, before, ['p1 used attack Body Slam from p1:active (Makuhita)', 'p1 set damage on p2:active (Makuhita): 0 → 60']);
});

test('Weakness and Resistance count toward the Knock Out hint and the damage sent', async ({ page }, testInfo) => {
  const before = await timelineLines(page);
  await openAttack(page, 'Body Slam', testInfo);
  const weakness = button(page, '×2 Weakness');
  const resistance = button(page, '−30 Resistance');
  await expect(weakness).toHaveAttribute('aria-pressed', 'false');
  await expect(resistance).toHaveAttribute('aria-pressed', 'false');

  await setBase(page, 40);
  await expect(ko(page)).toHaveCount(0);
  await press(weakness, testInfo);
  await expect(weakness).toHaveAttribute('aria-pressed', 'true');
  // The stepper keeps the base; the hint and the button use 40 × 2.
  await expect(sheet(page).getByRole('textbox', { name: 'Attack damage' })).toHaveValue('40');
  await expect(ko(page)).toHaveText('80 is enough to Knock Out Makuhita.');
  await expect(button(page, 'Attack Makuhita for 80')).toBeEnabled();
  // A toggle that is on is painted primary.
  const on = await style(weakness);
  expect([on.background, on.color]).toEqual([STUDIO.primary, STUDIO.onPrimary]);

  // Resistance comes off after Weakness: 40 × 2 − 30 = 50.
  await press(resistance, testInfo);
  await expect(resistance).toHaveAttribute('aria-pressed', 'true');
  await expect(button(page, 'Attack Makuhita for 50')).toBeVisible();
  await expect(ko(page)).toHaveCount(0);
  // 60 × 2 − 30 = 90.
  await setBase(page, 60);
  await expect(ko(page)).toHaveText('90 is enough to Knock Out Makuhita.');
  // Without Weakness, 60 − 30 = 30.
  await press(weakness, testInfo);
  await expect(button(page, 'Attack Makuhita for 30')).toBeVisible();
  await expect(ko(page)).toHaveCount(0);
  // Never below 0: 20 − 30.
  await setBase(page, 20);
  await expect(button(page, 'Attack Makuhita for 0')).toBeVisible();

  await setBase(page, 60);
  await press(weakness, testInfo);
  await press(button(page, 'Attack Makuhita for 90'), testInfo);
  await expectNewLines(page, before, ['p1 used attack Body Slam from p1:active (Makuhita)', 'p1 set damage on p2:active (Makuhita): 0 → 90']);
});

test('the Knock Out hint follows a changed target\'s HP left', async ({ page }, testInfo) => {
  await openAttack(page, 'Body Slam', testInfo);
  await press(button(page, '+10'), testInfo);
  // 70 does not Knock Out the Active's 80 HP.
  await expect(ko(page)).toHaveCount(0);

  await press(button(page, 'Change ›'), testInfo);
  await expect(banner(page)).toBeVisible();
  await press(titled(region(page, 'p2 bench 1'), 'Riolu (p2-1)'), testInfo);
  await expect(sheet(page)).toBeVisible();
  // The benched Riolu has 70 HP.
  await expect(sheet(page).locator('.target .where')).toHaveText("Bob's Bench 1 · 70 HP left");
  await expect(ko(page)).toHaveText('70 is enough to Knock Out Riolu.');
  await expect(button(page, 'Attack Riolu for 70')).toBeEnabled();
  await press(button(page, '−10'), testInfo);
  await expect(ko(page)).toHaveCount(0);
});

test('damage already on the target lowers the Knock Out threshold', async ({ page }, testInfo) => {
  await act(page, 'place_damage', { target: 'p2:active', amount: 30 });
  // The move arrives as another client's: reload to show it live.
  await page.reload();
  await expect(page.locator('header .dot.on')).toBeVisible();

  await openAttack(page, 'Body Slam', testInfo);
  await expect(sheet(page).locator('.target .where')).toHaveText("Bob's Active · 50 HP left");
  await expect(ko(page)).toHaveText('60 is enough to Knock Out Makuhita.');
  await press(button(page, '−10'), testInfo);
  await expect(ko(page)).toHaveText('50 is enough to Knock Out Makuhita.');
  await press(button(page, '−10'), testInfo);
  await expect(ko(page)).toHaveCount(0);
  await expect(button(page, 'Attack Makuhita for 40')).toBeEnabled();
});

// ---- Criterion 3: Card text for a card in hand.

const inHand = [
  { kind: 'a Pokémon', title: 'Riolu (p1-2)', name: 'Riolu', text: 'Riolu (tst-1)' },
  { kind: 'a Trainer', title: 'Iono (p1-7)', name: 'Iono', text: 'Each player shuffles their hand and puts it on the bottom of their deck.' },
  { kind: 'an Energy', title: 'Fighting Energy (p1-18)', name: 'Fighting Energy', text: 'Fighting Energy (sve-006)' },
];

for (const c of inHand) {
  test(`${c.kind} in hand offers Card text, which shows the card's text`, async ({ page }, testInfo) => {
    await open(page, titled(hand(page), c.title), testInfo);
    // Card text follows Reveal to opponent in Other, each with its icon.
    const other = group(page, 'Other');
    await expect(other.getByRole('button')).toHaveText(['Reveal to opponent', 'Card text']);
    await expect(other.locator('.item .ico svg')).toHaveCount(2);
    // On a phone the header's own "Card text ›" toggle is a different
    // control; the menu's item is named exactly "Card text".
    await press(other.getByRole('button', { name: 'Card text', exact: true }), testInfo);
    await expect(sheet(page)).toHaveCount(0);
    const zoom = page.locator('.zoom');
    await expect(zoom).toBeVisible();
    await expect(zoom).toHaveAttribute('aria-label', c.name);
    await expect(zoom.locator('h2')).toHaveText(c.name);
    await expect(zoom.locator('pre')).toContainText(c.text);
    // Nothing was played or moved.
    await expect(titled(hand(page), c.title)).toHaveCount(1);
  });
}

// ---- Criterion 4: target mode, the deck menu and the count panels.

// expectStudioBanner checks the target banner: raised, edged in primary,
// with an outlined 44px Cancel.
async function expectStudioBanner(page) {
  await expect(banner(page)).toBeVisible();
  await expect(banner(page)).toHaveClass(/\btarget-banner\b/);
  const b = await style(banner(page));
  expect([b.background, b.color]).toEqual([STUDIO.panelRaised, STUDIO.ink]);
  expect([b.borderLeftWidth, b.borderLeftColor]).toEqual(['4px', STUDIO.primary]);
  const cancel = banner(page).getByRole('button', { name: 'Cancel', exact: true });
  const c = await style(cancel);
  expect([c.borderWidth, c.borderStyle, c.borderColor, c.background]).toEqual(['1px', 'solid', STUDIO.controlLine, STUDIO.panelRaised]);
  expect((await cancel.boundingBox()).height).toBeGreaterThanOrEqual(44);
}

test('target mode shows the Studio banner, and Cancel still works', async ({ page }, testInfo) => {
  const before = await timelineLines(page);
  const energy = titled(hand(page), 'Fighting Energy (p1-18)');
  await open(page, energy, testInfo);
  await press(button(page, 'Attach'), testInfo);
  await expect(sheet(page)).toHaveCount(0);
  await expectStudioBanner(page);
  await expect(banner(page)).toContainText('Attach Fighting Energy');
  await press(banner(page).getByRole('button', { name: 'Cancel', exact: true }), testInfo);
  await expect(banner(page)).toHaveCount(0);
  await expect(page.locator('.card.lit')).toHaveCount(0);
  await expect(energy).toBeVisible();
  await page.waitForTimeout(300);
  expect(await timelineLines(page)).toEqual(before);

  // The attack panel's Change › uses the same banner; Cancel brings the
  // panel back with its target.
  await openAttack(page, 'Slap', testInfo);
  await press(button(page, 'Change ›'), testInfo);
  await expect(sheet(page)).toBeHidden();
  await expectStudioBanner(page);
  await press(banner(page).getByRole('button', { name: 'Cancel', exact: true }), testInfo);
  await expect(banner(page)).toHaveCount(0);
  await expect(sheet(page)).toBeVisible();
  await expect(button(page, 'Attack Makuhita for 20')).toBeVisible();
});

test('the deck menu has the Studio sheet styling, Draw 1 filled primary', async ({ page }, testInfo) => {
  await open(page, pile(page, 'Deck'), testInfo);
  await expect(sheet(page)).toHaveAttribute('aria-label', 'Your deck');
  await expectStudioSheet(page, testInfo);
  if (mode(testInfo) === 'popover') await expect(sheet(page).locator('.psum')).toHaveText('40 cards');
  await expect(sheet(page).locator('.item .lbl')).toHaveText(['Draw 1', 'Draw N…', 'Look at top N…', 'Search…', 'Deck list…', 'Shuffle']);
  const draw1 = button(page, 'Draw 1');
  await expect(draw1).toHaveClass(/\bprimary\b/);
  const d = await style(draw1);
  expect([d.background, d.color]).toEqual([STUDIO.primary, STUDIO.onPrimary]);
  // The other items are not filled.
  for (const name of ['Draw N…', 'Look at top N…', 'Search…', 'Deck list…', 'Shuffle']) {
    expect((await style(button(page, name))).background, name).not.toBe(STUDIO.primary);
  }
});

// expectStudioCount checks a count panel: its numbers in mono and its go
// button filled primary.
async function expectStudioCount(page, field, go) {
  const f = await style(sheet(page).getByRole('textbox', { name: field }));
  expect(f.fontFamily).toMatch(MONO);
  expect(f.background).toBe(STUDIO.panelSunk);
  expect((await style(button(page, '+1'))).fontFamily).toMatch(MONO);
  expect((await style(sheet(page).locator('.left .n'))).fontFamily).toMatch(MONO);
  await expect(sheet(page).locator('.left')).toHaveText('40 in the deck');
  const g = await style(button(page, go));
  expect([g.background, g.color]).toEqual([STUDIO.primary, STUDIO.onPrimary]);
}

test('Draw N… is a Studio count panel and still draws', async ({ page }, testInfo) => {
  await open(page, pile(page, 'Deck'), testInfo);
  await press(button(page, 'Draw N…'), testInfo);
  await press(button(page, '+1'), testInfo);
  await press(button(page, '+1'), testInfo);
  await expect(sheet(page).getByRole('textbox', { name: 'Cards to draw' })).toHaveValue('3');
  await expectStudioCount(page, 'Cards to draw', 'Draw 3');
  await press(button(page, 'Draw 3'), testInfo);
  await expect(sheet(page)).toHaveCount(0);
  await expect(page.getByRole('list', { name: 'p1 hand' }).locator('.count')).toHaveText('11 in hand');
});

test('Look at top N… is a Studio count panel and still opens the look view', async ({ page }, testInfo) => {
  await open(page, pile(page, 'Deck'), testInfo);
  await press(button(page, 'Look at top N…'), testInfo);
  await press(button(page, '+1'), testInfo);
  await expect(sheet(page).getByRole('textbox', { name: 'Cards to look at' })).toHaveValue('4');
  await expectStudioCount(page, 'Cards to look at', 'Look at the top 4');
  await press(button(page, 'Look at the top 4'), testInfo);
  const view = page.locator('.modal');
  await expect(view.locator('h2')).toHaveText('Top 4 of p1 deck');
  await expect(view.locator('.grid .card')).toHaveCount(4);
  // The view is a raised Studio panel over the scrim, full screen on a
  // phone.
  const v = await style(view);
  expect([v.background, v.color]).toEqual([STUDIO.panelRaised, STUDIO.ink]);
  expect(v.radius).toBe(isPhone(testInfo) ? '0px' : '16px');
  expect((await style(page.locator('.backdrop:has(+ .modal)'))).background).toBe(STUDIO.scrim);
});
