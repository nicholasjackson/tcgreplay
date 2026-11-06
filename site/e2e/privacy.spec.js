// The privacy page at /privacy: what the hosted playmat collects at
// sign-in, keeps while playing, how long, which outside services, and what
// it never collects. Its idle time follows the App Platform definition.
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { parse } from 'yaml';
import { test, expect } from '@playwright/test';

// idleFromAppSpec reads TCGREPLAY_IDLE_TIMEOUT from .do/app.yaml and renders
// it as the playmat does for people (minutes under two hours, else hours).
function idleFromAppSpec() {
  const spec = parse(readFileSync(join(process.cwd(), '..', '.do', 'app.yaml'), 'utf8'));
  const env = spec.services.find((s) => s.name === 'playmat').envs.find((e) => e.key === 'TCGREPLAY_IDLE_TIMEOUT');
  const m = /^(?:(\d+)h)?(?:(\d+)m)?(?:(\d+)s)?$/.exec(env.value);
  expect(m, `TCGREPLAY_IDLE_TIMEOUT ${env.value}`).not.toBeNull();
  const secs = (Number(m[1] ?? 0) * 60 + Number(m[2] ?? 0)) * 60 + Number(m[3] ?? 0);
  const plural = (n, unit) => (n === 1 ? `1 ${unit}` : `${n} ${unit}s`);
  if (secs < 60) return plural(secs, 'second');
  if (secs < 7200) return plural(Math.floor(secs / 60), 'minute');
  return plural(Math.floor(secs / 3600), 'hour');
}

test('the privacy page is at /privacy and linked from every page', async ({ page, request }) => {
  const resp = await request.get('/privacy');
  expect(resp.status()).toBe(200);
  await page.goto('/');
  await expect(page.getByRole('navigation', { name: 'Site' }).getByRole('link', { name: 'Privacy' })).toHaveAttribute('href', '/privacy');
});

test('it says what is collected at sign-in, kept while playing, for how long, by whom, and never', async ({ page }) => {
  await page.goto('/privacy');
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Privacy');
  const section = (name) => page.getByRole('region', { name });
  const signIn = section('When you sign in');
  await expect(signIn).toContainText('Google account ID');
  await expect(signIn).toContainText('email address');
  await expect(signIn).toContainText('verified');
  await expect(signIn).toContainText('tcgreplay_session');
  const playing = section('While you play');
  await expect(playing).toContainText('memory only');
  await expect(playing).toContainText('never written to disk');
  const howLong = section('How long it is kept');
  await expect(howLong).toContainText('30 days');
  await expect(howLong).toContainText('updated or restarted');
  const outside = section('Outside services');
  for (const name of ['Google', 'TCGdex', 'DigitalOcean', 'Cloudflare']) await expect(outside).toContainText(name);
  const never = section('What it never collects');
  for (const word of ['password', 'name', 'photo', 'Analytics']) await expect(never).toContainText(word);
});

test('its idle time is the one in the App Platform definition', async ({ page }) => {
  const want = idleFromAppSpec();
  await page.goto('/privacy');
  await expect(page.locator('[data-idle]')).toHaveText(want);
  await expect(page.getByRole('region', { name: 'How long it is kept' })).toContainText(`after ${want} without activity`);
});
