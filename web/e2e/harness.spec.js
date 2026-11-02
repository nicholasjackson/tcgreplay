// The harness itself: each test starts from the fixed position, and a
// browser dialog fails the test that opened it.
import { test, expect, card, act } from './fixtures.js';

test.describe.configure({ mode: 'serial' });

test('a test changes the board', async ({ page }) => {
  await act(page, 'move', { iid: 'p1-2', to: 'discard' });
  await expect(page.getByRole('list', { name: 'p1 hand' }).locator('.count')).toHaveText('7 in hand');
});

test('the next test starts from the fixed position again', async ({ page }) => {
  await expect(card(page.getByRole('list', { name: 'p1 hand' }), 'Riolu')).toBeAttached();
  await expect(page.getByRole('list', { name: 'p1 hand' }).locator('.count')).toHaveText('8 in hand');
});

test('a browser dialog fails the test', async ({ page }) => {
  test.fail();
  await page.evaluate(() => setTimeout(() => alert('a dialog')));
  await page.waitForTimeout(300);
});
