// The page for an address with no page: it answers 404 with the stack of
// cards and the two ways back, the home page and all the docs.
import { test, expect } from './fixtures.js';

test('a missing page says so and links to the home page and all the docs', async ({ page }) => {
  const resp = await page.goto('/no-such-page');
  expect(resp.status()).toBe(404);
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Page not found');
  await expect(page.locator('main')).toContainText('There is no page at this address.');
  const ways = page.locator('main [data-ways-back]');
  await expect(ways.getByRole('link', { name: 'Go to the home page' })).toHaveAttribute('href', '/');
  await expect(ways.getByRole('link', { name: 'All docs' })).toHaveAttribute('href', '/docs');
  // The picture is decoration: hidden from screen readers, and it fits the page.
  const stack = page.locator('main .stack');
  await expect(stack).toBeVisible();
  await expect(stack).toHaveAttribute('aria-hidden', 'true');
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await ways.getByRole('link', { name: 'All docs' }).click();
  await expect(page).toHaveURL(/\/docs\/?$/);
});
