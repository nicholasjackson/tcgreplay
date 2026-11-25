// The home page: on a laptop, the one-sentence pitch, the picture of the
// board and "Get started" are all on the first screen, and the picture is
// the board drawn in its own tokens with text card faces, never card
// artwork or logos. Below the hero: How it works in three steps, What it
// does on a slate band, four guide cards with prompts from their workflow
// pages, and the address.
import { test, expect } from './fixtures.js';

test('the pitch, the board picture and Get started fit the first laptop screen', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== 'laptop', 'the above-the-fold check is for the 1366x768 laptop');
  await page.goto('/');
  expect(page.viewportSize()).toEqual({ width: 1366, height: 768 });
  const items = {
    heading: page.getByRole('heading', { level: 1 }),
    pitch: page.locator('[data-pitch]'),
    picture: page.locator('[data-board-picture]'),
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

test('the hero says who it works with, offers the example game and has its fine print', async ({ page }) => {
  await page.goto('/');
  const hero = page.locator('main .hero');
  await expect(hero).toContainText('Works with Claude and ChatGPT');
  await expect(hero.locator('.fineprint')).toHaveText('Sign in with Google · the board runs in your browser');
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

test('How it works is three steps: connect, import a game, review it on the board, with a link to the How it works page', async ({ page }) => {
  await page.goto('/');
  const how = page.getByRole('region', { name: 'How it works' });
  const steps = how.getByRole('listitem');
  await expect(steps).toHaveCount(3);
  await expect(steps.nth(0)).toContainText(/connect/i);
  await expect(steps.nth(1)).toContainText(/battle log/i);
  await expect(steps.nth(2)).toContainText(/review/i);
  for (let i = 0; i < 3; i++) await expect(steps.nth(i).getByRole('heading', { level: 3 })).toBeVisible();
  await expect(how.getByRole('link', { name: /needs no trained model/ })).toHaveAttribute('href', '/docs/how-it-works');
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

test('the picture is the board mid-review in its own style, with text card faces and no artwork or logos', async ({ page }) => {
  await page.goto('/');
  const picture = page.locator('[data-board-picture]');
  // One image to assistive technology, described; the board inside is hidden.
  const img = picture.getByRole('img');
  await expect(img).toHaveCount(1);
  await expect(img).toHaveAttribute('aria-label', /board/i);
  await expect(picture.locator('.mb')).toHaveAttribute('aria-hidden', 'true');
  await expect(picture.locator('figcaption')).toHaveText('The live board your assistant drives, mid-review: step 217 of an imported game.');
  // The board's no-art card look: text card faces on the slate mat, the
  // turn strip, and the timeline at step 217.
  expect(await picture.locator('.mb-card').count()).toBeGreaterThanOrEqual(10);
  await expect(picture.locator('.mb-tag')).toHaveText('T5');
  await expect(picture.locator('.mb-row.now')).toContainText('#217');
  const colour = (token) =>
    page.evaluate((t) => {
      const probe = document.createElement('div');
      probe.style.color = `var(--${t})`;
      document.body.append(probe);
      const c = getComputedStyle(probe).color;
      probe.remove();
      return c;
    }, token);
  expect(await picture.locator('.mb-mat').evaluate((el) => getComputedStyle(el).backgroundColor)).toBe(await colour('mat-surface'));
  expect(await picture.locator('.mb-card').first().evaluate((el) => getComputedStyle(el).backgroundColor)).toBe(await colour('card-face'));
  // The frame: radius-l corners, the hero shadow and the picture scaled to fit.
  const frame = await picture.locator('.frame').evaluate((el) => {
    const board = el.querySelector('.mb').getBoundingClientRect();
    return { width: el.getBoundingClientRect().width, boardWidth: board.width, shadow: getComputedStyle(el).boxShadow };
  });
  expect(Math.abs(frame.boardWidth - frame.width)).toBeLessThanOrEqual(1);
  expect(frame.shadow).not.toBe('none');
  // The mark in its header is drawn inline, hidden from assistive technology.
  await expect(picture.locator('svg.mark')).toHaveAttribute('aria-hidden', 'true');
  // No images at all in the picture, and nothing on the page from TCGdex or Pokémon sites.
  expect(await picture.locator('img, svg image, picture, video, iframe').count()).toBe(0);
  const bgImages = await picture.evaluate((root) =>
    [root, ...root.querySelectorAll('*')].map((el) => getComputedStyle(el).backgroundImage).filter((b) => b.includes('url(')),
  );
  expect(bgImages).toEqual([]);
  const srcs = await page.locator('img').evaluateAll((imgs) => imgs.map((i) => i.getAttribute('src') ?? ''));
  for (const src of srcs) expect(src).not.toMatch(/tcgdex|pokemon/i);
});
