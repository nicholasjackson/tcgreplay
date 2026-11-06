// The home page: on a laptop, the one-sentence pitch, the playmat scene and
// "Get started" are all on the first screen, and the scene is the board's
// look with plain stand-in cards, never card artwork or logos.
import { test, expect } from '@playwright/test';

test('the pitch, the playmat scene and Get started fit the first laptop screen', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== 'laptop', 'the above-the-fold check is for the 1366x768 laptop');
  await page.goto('/');
  expect(page.viewportSize()).toEqual({ width: 1366, height: 768 });
  const items = {
    heading: page.getByRole('heading', { level: 1 }),
    pitch: page.locator('[data-pitch]'),
    scene: page.locator('[data-playmat]'),
    start: page.getByRole('link', { name: 'Get started' }).last(),
  };
  for (const [name, loc] of Object.entries(items)) {
    await expect(loc, name).toBeVisible();
    const box = await loc.boundingBox();
    expect(box, name).not.toBeNull();
    expect(box.y, `${name} top`).toBeGreaterThanOrEqual(0);
    expect(box.y + box.height, `${name} bottom`).toBeLessThanOrEqual(768);
  }
  await expect(items.start).toHaveAttribute('href', '/docs/connect');
});

test('the pitch is one sentence saying what tcgreplay does', async ({ page }) => {
  await page.goto('/');
  const pitch = (await page.locator('[data-pitch]').innerText()).trim();
  expect(pitch.split(/[.!?](\s|$)/).filter((s) => s && s.trim()).length).toBe(1);
  for (const word of ['battle log', 'assistant', 'board', 'rewind']) expect(pitch.toLowerCase()).toContain(word);
});

test('Get started opens the connecting instructions', async ({ page }) => {
  await page.goto('/');
  const start = page.locator('main').getByRole('link', { name: 'Get started' });
  await expect(start).toHaveAttribute('href', '/docs/connect');
});

test('the scene is the board in its own style, with stand-in cards and no artwork or logos', async ({ page }) => {
  await page.goto('/');
  const scene = page.locator('[data-playmat]');
  await expect(scene.getByRole('img')).toHaveAttribute('aria-label', /playmat/i);
  // The board's no-art card look: text cards, card backs and an energy token.
  expect(await scene.locator('.text-card').count()).toBeGreaterThanOrEqual(5);
  expect(await scene.locator('.back-art').count()).toBeGreaterThanOrEqual(1);
  expect(await scene.locator('.token').count()).toBeGreaterThanOrEqual(1);
  // The board's felt and seat colours.
  const felt = await scene.locator('.stage').evaluate((el) => getComputedStyle(el).backgroundImage);
  expect(felt).toContain('radial-gradient');
  // A battle log and the coach's message are part of the scene.
  await expect(scene.locator('.log li').first()).toBeVisible();
  await expect(scene.locator('.coach')).toContainText('Turn 3');
  // No images at all in the scene, and nothing on the page from TCGdex or Pokémon sites.
  expect(await scene.locator('img, svg image, picture, video').count()).toBe(0);
  const bgImages = await scene.evaluate((root) =>
    [root, ...root.querySelectorAll('*')].map((el) => getComputedStyle(el).backgroundImage).filter((b) => b.includes('url(')),
  );
  expect(bgImages).toEqual([]);
  const srcs = await page.locator('img').evaluateAll((imgs) => imgs.map((i) => i.getAttribute('src') ?? ''));
  for (const src of srcs) expect(src).not.toMatch(/tcgdex|pokemon/i);
});
