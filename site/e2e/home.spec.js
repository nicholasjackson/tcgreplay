// The home page: on a laptop, the one-sentence pitch, the playmat scene and
// "Get started" are all on the first screen, and the scene is the board's
// look with plain stand-in cards, never card artwork or logos. Below the
// hero: How it works in three steps, What it does on a slate band, four
// guide cards with prompts from their workflow pages, and the address.
import { test, expect } from './fixtures.js';

test('the pitch, the playmat scene and Get started fit the first laptop screen', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== 'laptop', 'the above-the-fold check is for the 1366x768 laptop');
  await page.goto('/');
  expect(page.viewportSize()).toEqual({ width: 1366, height: 768 });
  const items = {
    heading: page.getByRole('heading', { level: 1 }),
    pitch: page.locator('[data-pitch]'),
    scene: page.locator('[data-playmat]'),
    start: page.locator('main').getByRole('link', { name: 'Get started' }),
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

test('Get started opens the connecting instructions, and is the only Get started on the page itself', async ({ page }) => {
  await page.goto('/');
  const start = page.locator('main').getByRole('link', { name: 'Get started' });
  // One in main (the header has its own): the address block below is not a second one.
  await expect(start).toHaveCount(1);
  await expect(start).toHaveAttribute('href', '/docs/connect');
});

test('the hero says who it works with and offers the example game', async ({ page }) => {
  await page.goto('/');
  const hero = page.locator('main .hero');
  await expect(hero).toContainText('Works with Claude and ChatGPT');
  await expect(hero.getByRole('link', { name: 'See an example game' })).toHaveAttribute('href', '/docs/example');
  // In order: the badge, the heading, the pitch, then the actions.
  const order = await hero.evaluate((el) =>
    [el.querySelector('.badge'), el.querySelector('h1'), el.querySelector('[data-pitch]'), el.querySelector('a[href="/docs/connect"]')].map(
      (n) => n && n.getBoundingClientRect().top,
    ),
  );
  expect(order.every((y) => y !== null)).toBe(true);
  expect([...order].sort((a, b) => a - b)).toEqual(order);
});

test('How it works is three steps: connect, import a game, review it on the board', async ({ page }) => {
  await page.goto('/');
  const how = page.getByRole('region', { name: 'How it works' });
  const steps = how.getByRole('listitem');
  await expect(steps).toHaveCount(3);
  await expect(steps.nth(0)).toContainText(/connect/i);
  await expect(steps.nth(1)).toContainText(/battle log/i);
  await expect(steps.nth(2)).toContainText(/review/i);
  for (let i = 0; i < 3; i++) await expect(steps.nth(i).getByRole('heading', { level: 3 })).toBeVisible();
});

test('What it does sits on a slate band, the mat colour, the width of the window', async ({ page }) => {
  await page.goto('/');
  const band = page.getByRole('region', { name: 'What it does' });
  await expect(band.getByRole('listitem')).toHaveCount(3);
  const { bg, slate, shadow } = await band.evaluate((el) => {
    const probe = document.createElement('div');
    probe.style.background = 'var(--slate-surface)';
    document.body.append(probe);
    const slate = getComputedStyle(probe).backgroundColor;
    probe.remove();
    const cs = getComputedStyle(el);
    return { bg: cs.backgroundColor, slate, shadow: cs.boxShadow };
  });
  expect(bg).toBe(slate);
  // The band's colour reaches past main's gutters (a spread shadow, clipped to the band's height).
  expect(shadow).toContain(slate);
});

test('four guide cards each show a workflow and one of its own example prompts', async ({ page }) => {
  await page.goto('/');
  const guides = page.getByRole('region', { name: 'Ask in your own words' });
  await expect(guides.getByRole('link', { name: 'All coaching workflows' })).toHaveAttribute('href', '/docs/workflows');
  const cards = guides.locator('a.guide-card');
  await expect(cards).toHaveCount(4);
  const shown = await cards.evaluateAll((els) =>
    els.map((a) => ({ href: a.getAttribute('href'), title: a.querySelector('.guide-title').textContent, prompt: a.querySelector('.guide-prompt').textContent })),
  );
  expect(new Set(shown.map((c) => c.href)).size).toBe(4);
  for (const c of shown) {
    expect(c.href).toMatch(/^\/docs\/workflows\/[a-z-]+$/);
    // The prompt is one the workflow's own page offers, word for word.
    await page.goto(c.href);
    await expect(page.getByRole('heading', { level: 1 })).toHaveText(c.title);
    const prompts = await page.locator('[data-copy]').evaluateAll((els) => els.map((el) => el.dataset.copyText ?? el.textContent));
    expect(prompts, `${c.href}: ${c.prompt}`).toContain(c.prompt);
  }
});

test('the scene is the board in its own style, with stand-in cards and no artwork or logos', async ({ page }) => {
  await page.goto('/');
  const scene = page.locator('[data-playmat]');
  await expect(scene.getByRole('img')).toHaveAttribute('aria-label', /playmat/i);
  // The board's no-art card look: text cards, Studio card backs and an energy token.
  expect(await scene.locator('.text-card').count()).toBeGreaterThanOrEqual(5);
  expect(await scene.locator('.back-art').count()).toBeGreaterThanOrEqual(1);
  expect(await scene.locator('.token').count()).toBeGreaterThanOrEqual(1);
  // The Studio card back, as the board draws it: navy, framed in cyan, with
  // the mark drawn inline (hidden from assistive technology), never an image.
  const colour = (token) =>
    page.evaluate((t) => {
      const probe = document.createElement('div');
      probe.style.color = `var(--${t})`;
      document.body.append(probe);
      const c = getComputedStyle(probe).color;
      probe.remove();
      return c;
    }, token);
  const back = scene.locator('.back-art').first();
  const backStyle = await back.evaluate((el) => ({ bg: getComputedStyle(el).backgroundColor, frame: getComputedStyle(el).boxShadow }));
  expect(backStyle.bg).toBe(await colour('brand-navy'));
  expect(backStyle.frame).toContain(await colour('brand-cyan'));
  await expect(back.locator('svg.mark')).toHaveAttribute('aria-hidden', 'true');
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
