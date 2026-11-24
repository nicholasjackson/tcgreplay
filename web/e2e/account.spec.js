// The board menu's account line on the hosted playmat: the board reads
// /auth/session (stubbed here, as the test server is the desktop playmat,
// which has no sign-in) and shows the account with how it signed in.
import { test, expect } from './fixtures.js';

// withSession reloads the board with /auth/session answering body, and
// opens the menu.
async function withSession(page, body) {
  await page.route('**/auth/session', (route) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(body) }));
  await page.reload();
  await expect(page.locator('header .dot.on')).toBeVisible();
  await page.locator('header').getByRole('button', { name: 'Menu' }).click();
}

test('a password account shows its username, not a Google sign-in', async ({ page }) => {
  await withSession(page, { email: 'openai-reviewer', provider: 'password' });
  const account = page.locator('header .account[title]');
  await expect(account).toHaveText('openai-reviewer');
  await expect(account).toHaveAttribute('title', 'Signed in with a reviewer account');
  await expect(account).not.toHaveAttribute('title', /Google/);
});

test('a Google account keeps its label and tooltip', async ({ page }) => {
  await withSession(page, { email: 'player@example.com', provider: 'google' });
  const account = page.locator('header .account[title]');
  await expect(account).toHaveText('player@example.com');
  await expect(account).toHaveAttribute('title', 'Signed in with Google');
});

test('a session from before the provider was reported reads as Google', async ({ page }) => {
  await withSession(page, { email: 'player@example.com' });
  await expect(page.locator('header .account[title]')).toHaveAttribute('title', 'Signed in with Google');
});
