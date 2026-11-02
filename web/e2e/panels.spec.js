// Panels in place of browser dialogs: an attack set up in the sheet with
// its damage and target, and Draw N and Look at top N by a count on
// screen. The fixture fails any test that opens a browser dialog.
import { readFileSync, readdirSync } from 'node:fs';
import { test, expect, act, isTouch, timelineLines, pile } from './fixtures.js';

const region = (page, name) => page.getByRole('region', { name, exact: true });
const titled = (scope, title) => scope.locator(`.card[title="${title}"]`);
const sheet = (page) => page.locator('.sheet');
const button = (page, name) => sheet(page).getByRole('button', { name, exact: true });
const active = (page) => titled(region(page, 'p1 Active'), 'Makuhita (p1-5)');

// open taps (touch) or right-clicks (desktop) for a card's actions.
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
async function expectNewLines(page, before, lines) {
  await expect.poll(async () => (await timelineLines(page)).slice(before.length)).toEqual(lines);
}

test('an attack shows its printed damage, takes a changed damage and target, and records them', async ({ page }, testInfo) => {
  const before = await timelineLines(page);
  await open(page, active(page), testInfo);
  await press(button(page, 'Body Slam · 60').or(sheet(page).getByRole('button', { name: /^Body Slam/ })), testInfo);

  const damage = sheet(page).getByRole('textbox', { name: 'Attack damage' });
  await expect(damage).toHaveValue('60');
  await expect(button(page, 'Attack Makuhita for 60')).toBeVisible();
  // Adjust: +10 then ×2 for Weakness.
  await press(button(page, '+10'), testInfo);
  await press(button(page, '×2'), testInfo);
  await expect(damage).toHaveValue('140');

  // Change the target on the board: the sheet steps aside, the
  // opponent's Pokémon light, and the panel comes back with the choice.
  await press(button(page, 'Change ›'), testInfo);
  await expect(sheet(page)).toBeHidden();
  await expect(page.getByRole('region', { name: 'Choose a target' })).toBeVisible();
  expect(await page.locator('main .card.lit').evaluateAll((cs) => cs.map((c) => c.title).sort())).toEqual(['Makuhita (p2-5)', 'Riolu (p2-1)']);
  await press(titled(region(page, 'p2 bench 1'), 'Riolu (p2-1)'), testInfo);
  await expect(sheet(page)).toBeVisible();
  await expect(damage).toHaveValue('140');
  await expect(sheet(page).getByText("Bob's Bench 1")).toBeVisible();

  await press(button(page, 'Attack Riolu for 140'), testInfo);
  await expect(sheet(page)).toHaveCount(0);
  await expectNewLines(page, before, ['p1 used attack Body Slam from p1:active (Makuhita)', 'p1 set damage on p2:bench:0 (Riolu): 0 → 140']);
});

test('a typed damage and the default target, the opponent\'s Active', async ({ page }, testInfo) => {
  const before = await timelineLines(page);
  await open(page, active(page), testInfo);
  await press(sheet(page).getByRole('button', { name: /^Slap/ }), testInfo);
  const damage = sheet(page).getByRole('textbox', { name: 'Attack damage' });
  await damage.fill('50');
  await damage.press('Enter');
  await press(button(page, 'Attack Makuhita for 50'), testInfo);
  await expectNewLines(page, before, ['p1 used attack Slap from p1:active (Makuhita)', 'p1 set damage on p2:active (Makuhita): 0 → 50']);
});

test('cancelling the target choice keeps the panel and its target', async ({ page }, testInfo) => {
  await open(page, active(page), testInfo);
  await press(sheet(page).getByRole('button', { name: /^Slap/ }), testInfo);
  await press(button(page, 'Change ›'), testInfo);
  await page.keyboard.press('Escape');
  await expect(sheet(page)).toBeVisible();
  await expect(button(page, 'Attack Makuhita for 20')).toBeVisible();
});

test('‹ Actions goes back from a panel to the actions', async ({ page }, testInfo) => {
  await open(page, active(page), testInfo);
  await press(sheet(page).getByRole('button', { name: /^Slap/ }), testInfo);
  await press(button(page, '‹ Actions'), testInfo);
  await expect(sheet(page).locator('section[role="group"][aria-label="Attack"]')).toBeVisible();
});

test('a Pokémon with no attack data names its attack and damage in the sheet', async ({ page }, testInfo) => {
  // Every Pokémon in the fake pool has attacks, so the Active becomes a
  // card with no attack data: the Makuhita steps to the bench and an
  // Ultra Ball is set down as a Pokémon.
  await act(page, 'move', { iid: 'p1-5', to: 'bench:2' });
  await act(page, 'move', { iid: 'p1-13', to: 'active', as: 'pokemon' });
  await expect(titled(region(page, 'p1 Active'), 'Ultra Ball (p1-13)')).toBeVisible();
  const before = await timelineLines(page);
  await open(page, titled(region(page, 'p1 Active'), 'Ultra Ball (p1-13)'), testInfo);
  await press(button(page, 'Attack…'), testInfo);
  await expect(button(page, 'Attack Makuhita for 0')).toBeDisabled();
  await sheet(page).getByRole('textbox', { name: 'Attack name' }).fill('Tackle');
  await press(button(page, '+10'), testInfo);
  await press(button(page, 'Attack Makuhita for 10'), testInfo);
  await expectNewLines(page, before, ['p1 used attack Tackle from p1:active (Ultra Ball)', 'p1 set damage on p2:active (Makuhita): 0 → 10']);
});

test('Draw N draws the count chosen on screen', async ({ page }, testInfo) => {
  const before = await timelineLines(page);
  await open(page, pile(page, 'Deck'), testInfo);
  await press(button(page, 'Draw N…'), testInfo);
  await press(button(page, '+1'), testInfo);
  await press(button(page, '+1'), testInfo);
  await expect(sheet(page).getByRole('textbox', { name: 'Cards to draw' })).toHaveValue('3');
  await press(button(page, 'Draw 3'), testInfo);
  await expect(page.getByRole('list', { name: 'p1 hand' }).locator('.count')).toHaveText('11 in hand');
  await expect.poll(async () => (await timelineLines(page)).slice(before.length)).toEqual([
    expect.stringMatching(/^p1 drew p1-\d+ /),
    expect.stringMatching(/^p1 drew p1-\d+ /),
    expect.stringMatching(/^p1 drew p1-\d+ /),
  ]);
});

test('Look at top N shows the count chosen on screen', async ({ page }, testInfo) => {
  await open(page, pile(page, 'Deck'), testInfo);
  await press(button(page, 'Look at top N…'), testInfo);
  await press(button(page, '+1'), testInfo);
  await expect(sheet(page).getByRole('textbox', { name: 'Cards to look at' })).toHaveValue('4');
  await press(button(page, 'Look at the top 4'), testInfo);
  await expect(page.locator('.modal h2')).toHaveText('Top 4 of p1 deck');
  await expect(page.locator('.modal .grid .card')).toHaveCount(4);
});

test('the count cannot go past the cards left in the deck', async ({ page }, testInfo) => {
  // p1's deck holds 40 cards.
  await open(page, pile(page, 'Deck'), testInfo);
  await press(button(page, 'Draw N…'), testInfo);
  const count = sheet(page).getByRole('textbox', { name: 'Cards to draw' });
  await count.fill('500');
  await count.press('Enter');
  await expect(count).toHaveValue('40');
  await expect(button(page, '+1')).toBeDisabled();
  await expect(button(page, 'Draw 40')).toBeVisible();
});

test('the built board calls no browser dialog', async () => {
  const dir = new URL('../../internal/webui/dist/app/assets/', import.meta.url);
  const js = readdirSync(dir).filter((f) => f.endsWith('.js')).map((f) => readFileSync(new URL(f, dir), 'utf8')).join('\n');
  expect(js).not.toMatch(/\b(prompt|confirm|alert)\(/);
});
