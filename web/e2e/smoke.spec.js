// The board loads the fixed position in every profile.
import { test, expect, card, isTouch } from './fixtures.js';

test('touch profiles have a coarse touch pointer, desktop a fine mouse', async ({ page }, testInfo) => {
  const media = await page.evaluate(() => ({
    coarse: matchMedia('(pointer: coarse)').matches,
    hover: matchMedia('(hover: hover)').matches,
    touchPoints: navigator.maxTouchPoints,
  }));
  if (isTouch(testInfo)) {
    expect(media).toMatchObject({ coarse: true, hover: false });
    expect(media.touchPoints).toBeGreaterThan(0);
  } else {
    expect(media).toMatchObject({ coarse: false, hover: true });
  }
});

test('loads the fixed position', async ({ page }) => {
  // Each name shows by its seat, or on a phone in the seat's counters.
  // Each player is named: in their player block on today's mat, by name
  // and seat on the landscape mat, and in their counters on a phone.
  const named = (seat, name) =>
    page
      .getByRole('region', { name: `${seat} side` }).locator('.seat-name strong', { hasText: name })
      .or(page.locator('.seat-name').filter({ hasText: `${name} · ${seat}` }))
      .or(page.getByRole('group', { name: `${seat} counters` }).getByText(name, { exact: true }));
  await expect(named('p1', 'Alice')).toBeVisible();
  await expect(named('p2', 'Bob')).toBeVisible();

  const hand = page.getByRole('list', { name: 'p1 hand' });
  for (const name of ['Riolu', 'Lucario', 'Fighting Energy', 'Band', 'Iono', 'Deep Zone', 'Ultra Ball']) {
    await expect(card(hand, name)).toBeAttached();
  }
  await expect(hand.locator('.count')).toHaveText('8 in hand');
  // The opponent's hand: on the landscape mat its row of backs; on today's
  // mat the Hand chip in their player block; on a phone their counter.
  await expect(
    page
      .getByRole('list', { name: 'p2 hand' }).locator('.count')
      .or(page.getByRole('region', { name: 'p2 side' }).getByRole('button', { name: 'Hand 6' }))
      .or(page.getByRole('group', { name: 'p2 counters' }).getByLabel('Hand 6')),
  ).toHaveText(/6 in hand|6/);

  const inPlay = {
    'p1 Active': 'Makuhita (p1-5)',
    'p1 bench 1': 'Riolu (p1-1)',
    'p1 bench 2': 'Makuhita (p1-6)',
    'p2 Active': 'Makuhita (p2-5)',
    'p2 bench 1': 'Riolu (p2-1)',
  };
  for (const [region, title] of Object.entries(inPlay)) {
    await expect(page.getByRole('region', { name: region, exact: true }).locator(`.card[title="${title}"]`)).toBeVisible();
  }
  await expect(page.getByRole('region', { name: 'Stadium' }).locator('.card[title="Arena (p1-11)"]')).toBeAttached();
});
