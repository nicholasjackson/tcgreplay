// The home page animation: with the playmat's own addresses blocked it
// plays by itself through its beats in order and starts again; with
// reduced motion it is a still and nothing moves; the home page stays
// under 1.5 MB; and the scene uses nothing but the page's own drawing.
import { test, expect } from '@playwright/test';

const beats = ['log', 'board', 'coach', 'rewind', 'line'];

// blockPlaymat aborts every request for the playmat's own addresses.
async function blockPlaymat(page) {
  const blocked = [];
  await page.route(/\/(board|mcp|ws|api)(\/|\?|$)/, (route) => {
    blocked.push(route.request().url());
    return route.abort();
  });
  return blocked;
}

test('with the playmat blocked, the animation plays its beats in order and starts again', async ({ page }) => {
  test.setTimeout(45_000);
  const blocked = await blockPlaymat(page);
  await page.addInitScript(() => {
    window.__beats = [];
    new MutationObserver(() => {
      const el = document.querySelector('[data-playmat]');
      if (el) {
        const b = el.dataset.beat;
        if (window.__beats.at(-1) !== b) window.__beats.push(b);
      }
    }).observe(document, { subtree: true, attributes: true, attributeFilter: ['data-beat'] });
  });
  await page.goto('/');
  // The full loop and the start of the next one.
  await page.waitForFunction(() => window.__beats.filter((b) => b === 'log').length >= 2, null, { timeout: 30_000 });
  const seen = await page.evaluate(() => window.__beats);
  const first = seen.indexOf('log');
  expect(first).toBeGreaterThanOrEqual(0);
  expect(seen.slice(first, first + beats.length + 1)).toEqual([...beats, 'log']);
  // Nothing on the page needed the playmat.
  expect(blocked).toEqual([]);
});

test('each beat shows its moment: log, board, coach, rewind, the different line', async ({ page }) => {
  test.setTimeout(45_000);
  await page.goto('/');
  const scene = page.locator('[data-playmat]');
  const at = (beat) => page.waitForFunction((b) => document.querySelector('[data-playmat]').dataset.beat === b, beat, { timeout: 20_000 });
  const opacity = (loc) => loc.evaluate((el) => Number(getComputedStyle(el).opacity));
  await at('log');
  await page.waitForTimeout(1800);
  expect(await opacity(scene.locator('.log li').first())).toBe(1);
  expect(await opacity(scene.locator('.card').first())).toBe(0);
  await at('board');
  await page.waitForTimeout(1500);
  expect(await opacity(scene.locator('.text-card').first().locator('xpath=..'))).toBe(1);
  expect(await opacity(scene.locator('[data-line]'))).toBe(0);
  expect(await opacity(scene.locator('.coach'))).toBe(0);
  await at('coach');
  await page.waitForTimeout(800);
  expect(await opacity(scene.locator('.coach'))).toBe(1);
  await at('rewind');
  await page.waitForTimeout(1000);
  expect(await opacity(scene.locator('.sweep'))).toBe(1);
  await expect(scene.locator('[data-turn]')).toHaveText('Turn 3');
  await at('line');
  await page.waitForTimeout(1200);
  expect(await opacity(scene.locator('[data-line]'))).toBe(1);
  expect(await opacity(scene.locator('.log li[data-log="line"]'))).toBe(1);
});

test('with reduced motion, the scene is the still and nothing on the page moves', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.goto('/');
  const scene = page.locator('[data-playmat]');
  const frame = () =>
    scene.evaluate((el) => ({
      beat: el.dataset.beat,
      seen: el.dataset.beatSeen ?? '',
      turn: el.querySelector('[data-turn]').textContent,
      opacities: [...el.querySelectorAll('.card, .coach, .log li, .sweep')].map((n) => getComputedStyle(n).opacity).join(','),
    }));
  const before = await frame();
  expect(before.beat).toBe('line');
  expect(before.seen).toBe('');
  await page.waitForTimeout(4000);
  expect(await frame()).toEqual(before);
  expect(await page.evaluate(() => document.getAnimations().length)).toBe(0);
  // The still is the full scene: the better line and the coach are shown.
  await expect(scene.locator('[data-line]')).toBeVisible();
  await expect(scene.locator('.coach')).toBeVisible();
});

test('the home page, animation included, downloads under 1.5 MB', async ({ page }) => {
  let total = 0;
  page.on('response', async (resp) => {
    try {
      total += (await resp.body()).length;
    } catch {
      // Redirects and aborted requests have no body.
    }
  });
  await page.goto('/', { waitUntil: 'networkidle' });
  expect(total).toBeGreaterThan(0);
  expect(total).toBeLessThan(1.5 * 1024 * 1024);
});

test('the scene draws everything itself: no images and nothing from another host', async ({ page, baseURL }) => {
  await page.goto('/');
  const scene = page.locator('[data-playmat]');
  expect(await scene.locator('img, image, picture, video, iframe').count()).toBe(0);
  const urls = await scene.evaluate((root) =>
    [root, ...root.querySelectorAll('*')].flatMap((el) => {
      const cs = getComputedStyle(el);
      return [cs.backgroundImage, cs.maskImage, cs.borderImageSource].filter((v) => v && v.includes('url('));
    }),
  );
  expect(urls).toEqual([]);
  const origin = new URL(baseURL).origin;
  const scripts = await page.locator('script[src], link[href]').evaluateAll((els) => els.map((e) => e.src || e.href));
  for (const u of scripts) expect(new URL(u).origin).toBe(origin);
});
