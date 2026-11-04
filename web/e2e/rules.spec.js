// Board help for the rules the playmat doesn't enforce: costs, prizes,
// healing, reveals and the end of the game.
import { test, expect, act, isTouch, isPhone, pile } from './fixtures.js';

const region = (page, name) => page.getByRole('region', { name, exact: true });
const titled = (scope, title) => scope.locator(`.card[title="${title}"]`);
const sheet = (page) => page.locator('.sheet');
const button = (page, name) => sheet(page).getByRole('button', { name, exact: true });

// openActions taps a card (touch) or right-clicks it (mouse).
async function openActions(page, target, testInfo) {
  if (isTouch(testInfo)) await target.tap();
  else await target.click({ button: 'right' });
  await expect(sheet(page)).toBeVisible();
}

async function activate(control, testInfo) {
  if (isTouch(testInfo)) await control.tap();
  else await control.click();
}

test('a revealed card in the opponent\'s hand stays face down, marked with its name', async ({ page }, testInfo) => {
  test.skip(isPhone(testInfo), 'the phone board shows no opponent hand');
  const theirs = page.getByRole('list', { name: 'p2 hand' });
  await expect(theirs.locator('.known')).toHaveCount(0);
  await act(page, 'reveal', { iids: ['p2-2'], seat: 'p2' });
  const known = theirs.locator('.known');
  await expect(known).toHaveCount(1);
  await expect(known).toHaveText('Riolu');
  // Still a card back, not the card face.
  await expect(theirs.locator('.card.back')).toHaveCount(6);
});

test('Heal all heals every damage counter in one step', async ({ page }, testInfo) => {
  await act(page, 'place_damage', { target: 'p1:active', amount: 50 });
  const spot = region(page, 'p1 Active');
  await expect(spot.locator('.hpleft')).toHaveText('30/80');
  await openActions(page, titled(spot, 'Makuhita (p1-5)'), testInfo);
  await activate(button(page, 'Heal all (50 damage)'), testInfo);
  await expect(spot.locator('.hpleft')).toHaveText('80/80');
});

test('while Ultra Ball\'s cost is unpaid, a hand card\'s first action discards it for the cost', async ({ page }, testInfo) => {
  await act(page, 'play_trainer', { iid: 'p1-13', seat: 'p1' });
  const iono = titled(page.getByRole('list', { name: 'p1 hand' }), 'Iono (p1-7)');
  await openActions(page, iono, testInfo);
  const first = sheet(page).locator('section[role="group"] button').first();
  await expect(first).toContainText('Discard for Ultra Ball');
  await activate(first, testInfo);
  await expect(titled(page.getByRole('list', { name: 'p1 hand' }), 'Iono (p1-7)')).toHaveCount(0);
});

test('an Energy in the discard pile attaches straight onto a Pokémon', async ({ page }, testInfo) => {
  await (isTouch(testInfo) ? pile(page, 'Discard').tap() : pile(page, 'Discard').click());
  const view = page.locator('.modal');
  const row = view.locator('.card[title="Fighting Energy (p1-16)"]').locator('xpath=ancestor::*[.//button][1]');
  await activate(row.getByRole('button', { name: 'Attach', exact: true }), testInfo);
  await expect(view).toHaveCount(0);
  const bench = region(page, 'p1 bench 1');
  await activate(bench, testInfo);
  await expect(bench.locator('.card[title="Fighting Energy (p1-16)"]')).toHaveCount(1);
});

test('a Knock Out reminds which prizes to take, unless prize reminders are off', async ({ page }, testInfo) => {
  test.skip(isPhone(testInfo), 'the ☰ menu path differs on the phone');
  const toasts = page.locator('.toast');
  await openActions(page, titled(region(page, 'p2 Active'), 'Makuhita (p2-5)'), testInfo);
  await activate(button(page, 'Knock Out'), testInfo);
  await expect(toasts.filter({ hasText: 'Knocked Out: p1 takes 1 prize card for Makuhita.' })).toHaveCount(1);

  await (isTouch(testInfo) ? page.getByRole('button', { name: 'Menu' }).tap() : page.getByRole('button', { name: 'Menu' }).click());
  await page.getByLabel('Prize reminders').uncheck();
  await page.keyboard.press('Escape');
  await openActions(page, titled(region(page, 'p2 bench 1'), 'Riolu (p2-1)'), testInfo);
  await activate(button(page, 'Knock Out'), testInfo);
  // The game is over (no Pokémon left), but no prize reminder shows.
  await expect(page.locator('.over-banner')).toBeVisible();
  await expect(toasts.filter({ hasText: 'Riolu' })).toHaveCount(0);
});

test('taking the last prize card ends the game with a banner', async ({ page }) => {
  await expect(page.locator('.over-banner')).toHaveCount(0);
  for (let i = 0; i < 6; i++) await act(page, 'take_prize', { seat: 'p1' });
  await expect(page.locator('.over-banner')).toHaveText(/Game over: .* wins \(took all their prize cards\)/);
});
