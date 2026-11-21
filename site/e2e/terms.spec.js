// The terms of service at /terms: open without signing in, linked from
// every page's footer, naming the tcgreplay project, covering each point
// the spec lists, with the hosted limits from the App Platform definition
// and the trademarks section from the shared notice file.
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

// The shared wording, read straight from its file, not through the site's loader.
const shared = JSON.parse(readFileSync(join(process.cwd(), '..', 'internal', 'notice', 'notice.json'), 'utf8'));
const companies = ['Nintendo', 'The Pokémon Company', 'Creatures', 'Game Freak'];

const sections = [
  'Who runs tcgreplay',
  'Free, and provided as is',
  'Your games',
  'Hosted limits',
  'Acceptable use',
  'Age',
  'Trademarks and card images',
  'Privacy',
  'Changes',
  'Help',
];

test('the terms are at /terms, open signed out with no cookie, and linked from the home page footer', async ({ page, request, context }) => {
  const resp = await request.get('/terms');
  expect(resp.status()).toBe(200);
  expect(resp.headers()['set-cookie']).toBeUndefined();
  await page.goto('/');
  const footer = page.locator('footer').getByRole('navigation', { name: 'Footer' });
  await footer.getByRole('link', { name: 'Terms', exact: true }).click();
  await expect(page).toHaveURL(/\/terms\/?$/);
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Terms of service');
  expect(await context.cookies()).toEqual([]);
});

test('it has a section for each point the spec lists', async ({ page }) => {
  await page.goto('/terms');
  for (const name of sections) {
    await expect(page.getByRole('region', { name, exact: true }), `section ${name}`).toBeVisible();
  }
});

test('it names the tcgreplay project and says free, as is, no availability guarantee, and games can be lost', async ({ page }) => {
  await page.goto('/terms');
  const section = (name) => page.getByRole('region', { name, exact: true });
  await expect(section('Who runs tcgreplay')).toContainText('run by the tcgreplay project');
  const asIs = section('Free, and provided as is');
  await expect(asIs).toContainText('free');
  await expect(asIs).toContainText('provided as is');
  await expect(asIs).toContainText('no guarantee that it stays available');
  const games = section('Your games');
  await expect(games).toContainText('can be lost at any time');
  await expect(games).toContainText('updated or restarted');
});

test('its hosted limits are the App Platform definition\'s', async ({ page }) => {
  const want = appSpecLimits();
  // Today's values, hand-maintained, so a silent change to the reader shows up.
  expect(want).toEqual({ rate: '300', users: '10', idle: '60 minutes' });
  await page.goto('/terms');
  const limits = page.getByRole('region', { name: 'Hosted limits' });
  await expect(limits.locator('[data-limit="rate"]')).toHaveText(want.rate);
  await expect(limits.locator('[data-limit="users"]')).toHaveText(want.users);
  await expect(limits.locator('[data-limit="idle"]')).toHaveText(want.idle);
  await expect(limits).toContainText(`${want.rate} requests a minute`);
  await expect(limits).toContainText(`${want.users} signed-in players at once`);
  await expect(limits).toContainText(`games idle for ${want.idle} are dropped`);
});

test('it lists the disallowed uses, the minimum age of 13, and that the terms and service may change or end', async ({ page }) => {
  await page.goto('/terms');
  const section = (name) => page.getByRole('region', { name, exact: true });
  const use = section('Acceptable use');
  for (const words of ['overload or attack the service', 'automated scraping', 'get round the limits', 'unlawful']) {
    await expect(use, words).toContainText(words);
  }
  await expect(section('Age')).toContainText('at least 13');
  await expect(section('Changes')).toContainText('may change or end');
});

test('its trademarks section carries the shared notice and the card-data credit', async ({ page }) => {
  await page.goto('/terms');
  const tm = page.getByRole('region', { name: 'Trademarks and card images' });
  await expect(tm.locator('.fan-notice')).toHaveText(shared.notice);
  for (const c of companies) await expect(tm, `names ${c}`).toContainText(c);
  for (const { name, url } of shared.credit.sources) {
    await expect(tm.getByRole('link', { name }), `credit links ${name}`).toHaveAttribute('href', url);
  }
  expect(shared.credit.sources.map((s) => s.name)).toEqual(['TCGdex', 'malie.io']);
});

test('it links to the privacy page and to the GitHub issues for help, and shows a last-updated date', async ({ page }) => {
  await page.goto('/terms');
  await expect(page.getByRole('region', { name: 'Privacy', exact: true }).getByRole('link', { name: 'privacy page' })).toHaveAttribute('href', '/privacy');
  const help = page.getByRole('region', { name: 'Help', exact: true });
  await expect(help.getByRole('link')).toHaveAttribute('href', shared.help.url);
  await expect(help).toContainText('GitHub issues');
  await expect(help).toContainText('no support email');
  expect(new URL(shared.help.url).host).toBe('github.com');
  await expect(page.locator('main [data-updated]')).toHaveText(/^Last updated \d{1,2} [A-Z][a-z]+ \d{4}\.$/);
});

test('an On this page card lists every section, in order, linking to it', async ({ page }) => {
  await page.goto('/terms');
  const toc = page.getByRole('navigation', { name: 'On this page' });
  await expect(toc).toBeVisible();
  const links = toc.getByRole('link');
  await expect(links).toHaveText(sections);
  for (const name of sections) {
    const href = await toc.getByRole('link', { name, exact: true }).getAttribute('href');
    expect(href, name).toMatch(/^#/);
    await expect(page.locator(`main ${href}`).getByRole('heading', { level: 2 }), `${href} is the ${name} section`).toHaveText(name);
  }
  // A card: its own white surface, apart from the text column.
  expect(await toc.evaluate((el) => getComputedStyle(el).backgroundColor)).not.toBe('rgba(0, 0, 0, 0)');
});

test('it keeps the space before its links', async ({ page }) => {
  await page.goto('/terms');
  await expect(page.getByRole('region', { name: 'Who runs tcgreplay' })).toContainText('Its source is on GitHub.');
  await expect(page.getByRole('region', { name: 'Help', exact: true })).toContainText("the project's GitHub issues");
});
