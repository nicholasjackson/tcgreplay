// Every coaching workflow the playmat offers has a page with at least one
// copyable example prompt, listed in the playmat's own order, showing the
// description from its guide; the rewind page explains branching with a
// prompt. The guides are read here from the playmat's files, independently
// of the site's own loader.
import { readFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';
import { parse } from 'yaml';
import { test, expect } from '@playwright/test';

const guidesDir = join(process.cwd(), '..', 'internal', 'guides');

function readGuides() {
  return readdirSync(guidesDir)
    .filter((f) => f.endsWith('.md'))
    .map((f) => {
      const m = /^---\n([\s\S]*?)\n---/.exec(readFileSync(join(guidesDir, f), 'utf8'));
      expect(m, `${f} frontmatter`).not.toBeNull();
      return parse(m[1]);
    });
}

function readOrder() {
  const src = readFileSync(join(guidesDir, 'guides.go'), 'utf8');
  const m = /var order = \[\]string\{([^}]*)\}/.exec(src);
  expect(m).not.toBeNull();
  return [...m[1].matchAll(/"([^"]+)"/g)].map((x) => x[1]);
}

test('the playmat has the seven workflows the site was written for', () => {
  // Hand-maintained: a new guide must get a page, and this list reminds us.
  expect(readOrder()).toEqual(['play-game', 'import-game', 'opponent-deck', 'review-game', 'replay-drill', 'before-you-move', 'numbers-drill']);
  expect(readGuides().map((g) => g.name).sort()).toEqual([...readOrder()].sort());
});

test('every workflow has a page with a copyable example prompt and its guide description', async ({ page }) => {
  for (const g of readGuides()) {
    const resp = await page.goto(`/docs/workflows/${g.name}`);
    expect(resp.status(), g.name).toBe(200);
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
    const prompts = page.locator('.copy-block.prompt');
    expect(await prompts.count(), `${g.name} prompts`).toBeGreaterThanOrEqual(1);
    await expect(prompts.first().getByRole('button', { name: /^Copy/ })).toBeVisible();
    await expect(page.locator('[data-guide-description]')).toContainText(g.description);
  }
});

test('the workflows page lists them in the playmat order with their guide descriptions', async ({ page }) => {
  await page.goto('/docs/workflows');
  const items = page.locator('[data-workflows] > li');
  const names = await items.evaluateAll((lis) => lis.map((li) => li.dataset.guide));
  expect(names).toEqual(readOrder());
  const guides = Object.fromEntries(readGuides().map((g) => [g.name, g.description]));
  for (const name of names) {
    const li = page.locator(`[data-workflows] > li[data-guide="${name}"]`);
    await expect(li).toContainText(guides[name]);
    await expect(li.getByRole('link')).toHaveAttribute('href', `/docs/workflows/${name}`);
  }
});

test('a workflow prompt copies exactly its text', async ({ context, page, baseURL }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'], { origin: new URL(baseURL).origin });
  await page.goto('/docs/workflows/review-game');
  const block = page.locator('.copy-block.prompt').filter({ hasText: 'Start with the turn where I lost tempo' });
  await block.getByRole('button', { name: /^Copy/ }).click();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe('Review this game with me. Start with the turn where I lost tempo.');
});

test('the rewind page explains rewinding to a chosen turn and playing another line, with a prompt', async ({ page }) => {
  const resp = await page.goto('/docs/rewind');
  expect(resp.status()).toBe(200);
  const prose = page.locator('.prose');
  await expect(prose).toContainText('branch');
  await expect(prose).toContainText("each player's Nth turn".replace("'", '’'));
  const prompts = prose.locator('.copy-block.prompt');
  expect(await prompts.count()).toBeGreaterThanOrEqual(1);
  await expect(prompts.first()).toContainText('Take me back to my third turn');
});
