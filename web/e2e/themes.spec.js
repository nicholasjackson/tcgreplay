// The interface theme (light or dark) and the mat (slate, felt or navy),
// chosen from ☰: light and slate on a first visit, any of the six pairs on
// request, colours only (nothing moves), remembered across a reload, and
// the defaults when the browser's storage is unavailable. The board's text
// is set in IBM Plex, served here from testdata/fonts in place of Google
// Fonts, with system fonts standing in when Google Fonts cannot be reached.
import { test, expect } from './fixtures.js';

const menu = (page) => page.getByRole('button', { name: 'Menu' });

// root reads the attributes and resolved colours that a theme sets, with
// short hex (the build minifies #FFFFFF to #FFF) written out in full.
function root(page) {
  return page.evaluate(() => {
    const css = getComputedStyle(document.documentElement);
    const hex = (name) => {
      const v = css.getPropertyValue(name).trim().toUpperCase();
      return /^#[0-9A-F]{3}$/.test(v) ? '#' + [...v.slice(1)].map((c) => c + c).join('') : v;
    };
    return {
      theme: document.documentElement.dataset.theme,
      mat: document.documentElement.dataset.mat,
      panel: hex('--panel'),
      surface: hex('--mat-surface'),
    };
  });
}

// plexLoaded reports whether the page has loaded an IBM Plex Sans face
// (document.fonts.check answers true for a family nothing declared).
function plexLoaded(page) {
  return page.evaluate(async () => {
    await document.fonts.ready;
    return [...document.fonts].some((f) => f.family.replace(/"/g, '') === 'IBM Plex Sans' && f.status === 'loaded');
  });
}

// choose opens ☰ and picks a radio by its name, then closes the menu.
async function choose(page, name) {
  await menu(page).click();
  await page.getByRole('radio', { name, exact: true }).check();
  await page.keyboard.press('Escape');
  if (await page.locator('.dropdown').isVisible()) await menu(page).click();
  await expect(page.locator('.dropdown')).toHaveCount(0);
}

// boxes records where the board's cards and controls are, settled (gotcha
// board-boxes-settle-before-comparing).
async function boxes(page) {
  await page.mouse.move(1, 1);
  await page.evaluate(() =>
    Promise.all(
      document
        .getAnimations()
        .filter((a) => a.effect?.getComputedTiming().endTime !== Infinity)
        .map((a) => a.finished.catch(() => {})),
    ),
  );
  return page.locator('header, main .card, .strip button, aside').evaluateAll((els) =>
    els.map((e) => {
      const r = e.getBoundingClientRect();
      return [e.tagName, e.getAttribute('title'), Math.round(r.x), Math.round(r.y), Math.round(r.width), Math.round(r.height)];
    }),
  );
}

test('a first visit shows the light interface with the slate mat', async ({ page }) => {
  expect(await root(page)).toEqual({ theme: 'light', mat: 'slate', panel: '#FFFFFF', surface: '#252A33' });
  await menu(page).click();
  await expect(page.getByRole('radio', { name: 'Light', exact: true })).toBeChecked();
  await expect(page.getByRole('radio', { name: 'Slate', exact: true })).toBeChecked();
});

test('each of the six interface-and-mat pairs can be chosen from ☰ and survives a reload', async ({ page }) => {
  const panel = { Light: '#FFFFFF', Dark: '#171B22' };
  const surface = { Slate: '#252A33', Felt: '#1D4430', Navy: '#14244A' };
  for (const theme of ['Light', 'Dark']) {
    for (const mat of ['Slate', 'Felt', 'Navy']) {
      await choose(page, theme);
      await choose(page, mat);
      const want = { theme: theme.toLowerCase(), mat: mat.toLowerCase(), panel: panel[theme], surface: surface[mat] };
      expect(await root(page), `${theme} with ${mat}`).toEqual(want);
      await page.reload();
      await expect(page.locator('header .dot.on')).toBeVisible();
      expect(await root(page), `${theme} with ${mat} after a reload`).toEqual(want);
    }
  }
});

test('changing the theme and the mat changes colours only: every element keeps its place and size', async ({ page }) => {
  const before = await boxes(page);
  expect(before.length).toBeGreaterThan(10);
  await choose(page, 'Dark');
  await choose(page, 'Navy');
  await expect.poll(() => boxes(page)).toEqual(before);
});

test('with the browser storage unavailable the board shows the defaults and still switches for the visit', async ({ page }) => {
  await page.addInitScript(() => {
    const deny = () => {
      throw new Error('storage disabled');
    };
    Object.defineProperty(window, 'localStorage', { get: deny });
  });
  await page.reload();
  await expect(page.locator('header .dot.on')).toBeVisible();
  expect(await root(page)).toMatchObject({ theme: 'light', mat: 'slate' });
  await choose(page, 'Felt');
  expect(await root(page)).toMatchObject({ mat: 'felt', surface: '#1D4430' });
});

test('the board is set in IBM Plex, and falls back to system fonts when Google Fonts cannot be reached', async ({ page }) => {
  expect(await plexLoaded(page)).toBe(true);
  expect(await page.evaluate(() => getComputedStyle(document.body).fontFamily)).toMatch(/^"IBM Plex Sans", system-ui/);

  await page.context().route('https://fonts.googleapis.com/**', (route) => route.abort('blockedbyclient'));
  await page.context().route('https://fonts.gstatic.com/**', (route) => route.abort('blockedbyclient'));
  await page.reload();
  await expect(page.locator('header .dot.on')).toBeVisible();
  expect(await plexLoaded(page)).toBe(false);
  // The stack names system fonts after Plex, so the board still reads.
  expect(await page.evaluate(() => getComputedStyle(document.body).fontFamily)).toContain('system-ui');
  await expect(page.getByRole('list', { name: 'p1 hand' })).toBeVisible();
});
