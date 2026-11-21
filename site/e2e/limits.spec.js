// The hosted limits page: its numbers are the App Platform definition's,
// and it says what is kept, when games are lost, how to keep the record
// and where to go when the playmat is full. The developer page only points
// to the repository.
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { parse } from 'yaml';
import { test, expect } from './fixtures.js';

// appSpecLimits reads the three limits from .do/app.yaml, independently of
// the site's loader, rendering the idle time as the playmat does.
function appSpecLimits() {
  const spec = parse(readFileSync(join(process.cwd(), '..', '.do', 'app.yaml'), 'utf8'));
  const envs = spec.envs;
  const get = (key) => envs.find((e) => e.key === key).value;
  const m = /^(?:(\d+)h)?(?:(\d+)m)?(?:(\d+)s)?$/.exec(get('TCGREPLAY_IDLE_TIMEOUT'));
  const secs = (Number(m[1] ?? 0) * 60 + Number(m[2] ?? 0)) * 60 + Number(m[3] ?? 0);
  const plural = (n, unit) => (n === 1 ? `1 ${unit}` : `${n} ${unit}s`);
  const idle = secs < 60 ? plural(secs, 'second') : secs < 7200 ? plural(Math.floor(secs / 60), 'minute') : plural(Math.floor(secs / 3600), 'hour');
  return { rate: get('TCGREPLAY_RATE_LIMIT'), users: get('TCGREPLAY_MAX_USERS'), idle };
}

test('the limits match the App Platform definition', async ({ page }) => {
  const want = appSpecLimits();
  // Today's values, hand-maintained, so a silent change to the reader shows up.
  expect(want).toEqual({ rate: '300', users: '10', idle: '60 minutes' });
  await page.goto('/docs/limits');
  await expect(page.locator('[data-limit="idle"]')).toHaveText(want.idle);
  await expect(page.locator('[data-limit="users"]')).toHaveText(want.users);
  await expect(page.locator('[data-limit="rate"]')).toHaveText(want.rate);
});

test('the stat tiles show the limits, each value alone in its tile with what it counts', async ({ page }) => {
  const want = appSpecLimits();
  await page.goto('/docs/limits');
  const tiles = page.locator('[data-limit-tiles] > li');
  await expect(tiles).toHaveCount(4);
  const tile = (key) => tiles.filter({ has: page.locator(`[data-limit="${key}"]`) });
  await expect(tile('idle')).toHaveCount(1);
  await expect(tile('idle').locator('[data-limit]')).toHaveText(want.idle);
  await expect(tile('idle')).toContainText('without activity');
  await expect(tile('users').locator('[data-limit]')).toHaveText(want.users);
  await expect(tile('users')).toContainText('players signed in at once');
  await expect(tile('rate').locator('[data-limit]')).toHaveText(want.rate);
  await expect(tile('rate')).toContainText('requests a minute');
  await expect(tiles.first()).toHaveText(/^1\s*game per account at a time$/);
  // The text says the same numbers.
  const main = page.locator('main');
  await expect(main).toContainText(`After ${want.idle} without activity`);
  await expect(main).toContainText(`up to ${want.users} signed-in players`);
  await expect(main).toContainText(`up to ${want.rate} requests a minute`);
});

test('it says what is kept, that games are lost when idle or updated, how to keep the record, and the desktop option', async ({ page }) => {
  await page.goto('/docs/limits');
  const main = page.locator('main');
  await expect(main).toContainText('one game at a time');
  await expect(main).toContainText('in memory');
  await expect(main).toContainText('updated or restarted');
  await expect(main).toContainText('without activity');
  await expect(page.getByRole('heading', { name: 'Keep the record' })).toBeVisible();
  await expect(page.locator('.copy-block.prompt')).toContainText('record');
  await expect(main).toContainText('tcgreplay is busy');
  await expect(main).toContainText('too many requests');
  const desktop = page.locator('.prose').getByRole('link', { name: 'running it on your desktop' });
  await expect(desktop).toHaveAttribute('href', '/docs/desktop');
});

test('the limits page is in the navigation and the docs sidebar', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByRole('navigation', { name: 'Site' }).getByRole('link', { name: 'Limits' })).toHaveAttribute('href', '/docs/limits');
  await page.goto('/docs/connect');
  await expect(page.getByRole('navigation', { name: 'Guide' }).getByRole('link', { name: 'Limits' })).toHaveAttribute('href', '/docs/limits');
});

test('the developer page links to the repository and has no build or test instructions', async ({ page }) => {
  const resp = await page.goto('/docs/developers');
  expect(resp.status()).toBe(200);
  const prose = page.locator('.prose');
  await expect(prose.getByRole('link', { name: /source repository/ })).toHaveAttribute('href', 'https://github.com/nicholasjackson/tcgreplay');
  const text = await prose.innerText();
  for (const banned of ['make ', 'go test', 'npm ', 'go build', 'git clone']) expect(text).not.toContain(banned);
  expect(await prose.locator('pre').count()).toBe(0);
  // Linked from the docs sidebar.
  await page.goto('/docs/connect');
  await expect(page.getByRole('navigation', { name: 'Guide' }).getByRole('link', { name: 'For developers' })).toHaveAttribute('href', '/docs/developers');
});
