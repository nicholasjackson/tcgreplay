// The privacy page at /privacy: what the hosted playmat collects at
// sign-in, keeps while playing, what visitor counting and diagnostics
// record, how long, which outside services, and what it never collects.
// Its idle time follows the App Platform definition, and its diagnostics
// retention the observability runbook (docs/observability.md).
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { parse } from 'yaml';
import { test, expect } from '@playwright/test';

// idleFromAppSpec reads TCGREPLAY_IDLE_TIMEOUT from .do/app.yaml and renders
// it as the playmat does for people (minutes under two hours, else hours).
function idleFromAppSpec() {
  const spec = parse(readFileSync(join(process.cwd(), '..', '.do', 'app.yaml'), 'utf8'));
  const env = spec.envs.find((e) => e.key === 'TCGREPLAY_IDLE_TIMEOUT');
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
  for (const word of ['password', 'name', 'photo']) await expect(never).toContainText(word);
  // Visitor counting is described below, so the old "no analytics" promise is gone.
  await expect(never).not.toContainText('Analytics');
  await expect(never).toContainText('Advertising or cross-site tracking data');
  await expect(never).toContainText('anything that identifies you in visitor counts');
});

test('it describes visitor counting: Cloudflare Web Analytics, what it records, no cookies, not linked to accounts, how long', async ({ page }) => {
  await page.goto('/privacy');
  const counting = page.getByRole('region', { name: 'Visitor counting' });
  await expect(counting).toContainText('Cloudflare Web Analytics');
  for (const word of ['the page', 'referrer', 'country', 'device', 'browser', 'operating system', 'how quickly the page loaded']) {
    await expect(counting, word).toContainText(word);
  }
  await expect(counting).toContainText('sets no cookies and stores nothing in your browser');
  await expect(counting).toContainText('never linked to your Google account');
  await expect(counting.locator('[data-retention="visits"]')).toHaveText(/\S/);
  await expect(counting).toContainText('Cloudflare keeps the counts for');
});

test('it describes diagnostics: DigitalOcean metrics and usage and error records by player ID, never-list and retention', async ({ page }) => {
  await page.goto('/privacy');
  const diag = page.getByRole('region', { name: 'Diagnostics' });
  await expect(diag).toContainText('DigitalOcean');
  await expect(diag).toContainText('request and performance metrics');
  await expect(diag).toContainText('usage and error records');
  for (const word of ['sign-ins', 'refused sign-ins', 'which tool', 'imports and new games', 'limits reached', 'games expiring', 'accounts removed', "another player's board", 'admin checks', 'errors']) {
    await expect(diag, word).toContainText(word);
  }
  await expect(diag).toContainText('player ID');
  await expect(diag).toContainText('cannot be turned back into your email address');
  const never = diag.locator('p', { hasText: 'never contain' });
  // The runbook's "What is never recorded" list.
  for (const word of ['email address', 'name', 'Google tokens', "the playmat's own tokens and cookies", 'battle logs', 'decklists', 'card data']) {
    await expect(never, word).toContainText(word);
  }
  await expect(diag.locator('[data-retention="logs"]')).toBeVisible();
  await expect(diag.locator('[data-retention="metrics"]')).toBeVisible();
});

// retentionSection is the observability runbook's Retention section: from
// its heading to the next heading of the same level.
function retentionSection() {
  const text = readFileSync(join(process.cwd(), '..', 'docs', 'observability.md'), 'utf8');
  const m = /^(#+) Retention\s*$/m.exec(text);
  expect(m, 'docs/observability.md has a Retention heading').not.toBeNull();
  const rest = text.slice(m.index + m[0].length);
  const next = new RegExp(`^${m[1]} `, 'm').exec(rest);
  return next ? rest.slice(0, next.index) : rest;
}

test('its logs and metrics retention appear in the observability runbook\'s Retention section', async ({ page }) => {
  const section = retentionSection();
  await page.goto('/privacy');
  for (const kind of ['logs', 'metrics']) {
    const figure = (await page.locator(`[data-retention="${kind}"]`).textContent()).trim();
    expect(figure, `${kind} retention`).not.toBe('');
    expect(section, `the runbook's Retention section states the ${kind} figure "${figure}"`).toContain(figure);
  }
});

test('its older statements match the playmat: sign-ins, disconnecting, card images, logs, browser settings', async ({ page }) => {
  await page.goto('/privacy');
  const main = page.locator('main');
  // False statements are gone.
  await expect(main).not.toContainText('loads nothing from other sites');
  await expect(main).not.toContainText('writes your email address');
  await expect(main).not.toContainText('kept encrypted, in the playmat');
  await expect(main).not.toContainText('the playmat forgets you');

  const section = (name) => page.getByRole('region', { name });
  const signIn = section('When you sign in');
  await expect(signIn).toContainText('keeps your sign-ins in its memory');
  await expect(signIn).toContainText('encrypted');
  await expect(signIn).toContainText('stay signed in when the playmat is updated');
  await expect(signIn).toContainText('four display settings');
  for (const word of ['replay speed', 'autoplay', 'where the panel sits', 'prize reminders']) await expect(signIn, word).toContainText(word);

  const howLong = section('How long it is kept');
  await expect(howLong).toContainText('does not tell the playmat');
  await expect(howLong).toContainText('within those 30 days');
  await expect(howLong).toContainText('hold no email address');
  await expect(section('Removing your data')).toContainText('within 30 days');

  const outside = section('Outside services');
  await expect(outside).toContainText('fetches them from TCGdex and malie.io');
  await expect(outside).toContainText('DigitalOcean Spaces');
  await expect(outside).toContainText('Cloudflare Web Analytics');
  await expect(outside).toContainText('The playmat sends nothing about you to TCGdex or malie.io');
  await expect(outside).toContainText('when your browser loads an image from them, they see that request');
  // The assistant is given the Google account ID (get_account), never the email.
  await expect(outside.locator('[data-assistant-id]')).toHaveText(
    'Google account ID (never your email address), so it can tell your accounts apart',
  );
});

test('its cookie statement names whose cookies they are, and the only outside script', async ({ page }) => {
  await page.goto('/privacy');
  const intro = page.locator('main article > p', { hasText: '__cf_bm' });
  await expect(intro).toContainText('sets no cookies on this website');
  await expect(intro).toContainText('two sign-in cookies');
  await expect(intro).toContainText('short-lived bot-protection cookie');
  await expect(intro).toContainText('not used for visitor counting and is not tied to your account');
  await expect(intro).toContainText("only thing this website may load from another site is Cloudflare's visitor-counting script");
});

test('it shows a last-updated date and links to the GitHub issues for questions', async ({ page }) => {
  const shared = JSON.parse(readFileSync(join(process.cwd(), '..', 'internal', 'notice', 'notice.json'), 'utf8'));
  await page.goto('/privacy');
  await expect(page.locator('main [data-updated]')).toHaveText(/^Last updated \d{1,2} [A-Z][a-z]+ \d{4}\.$/);
  await expect(page.getByRole('region', { name: 'Questions' }).getByRole('link')).toHaveAttribute('href', shared.help.url);
  expect(new URL(shared.help.url).pathname.endsWith('/issues')).toBe(true);
});

test('its idle time is the one in the App Platform definition', async ({ page }) => {
  const want = idleFromAppSpec();
  await page.goto('/privacy');
  await expect(page.locator('[data-idle]')).toHaveText(want);
  await expect(page.getByRole('region', { name: 'How long it is kept' })).toContainText(`after ${want} without activity`);
});
