// The example game page: the complete anonymised battle log and its
// decklist, each copied exactly by its button; and the import page's
// directions.
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { test, expect } from '@playwright/test';

const logFile = readFileSync(join(process.cwd(), 'src', 'content', 'example', 'battle-log.txt'), 'utf8');
const deckFile = readFileSync(join(process.cwd(), '..', 'testdata', 'decks', 'mega-lucario-ex-v12.txt'), 'utf8');
const noFinalNewline = (s) => s.replace(/\n$/, '');

async function copied(page, label) {
  const button = page.getByRole('button', { name: `Copy ${label}` });
  await expect(button).toBeVisible();
  await page.evaluate(() => navigator.clipboard.writeText(''));
  await button.click();
  await expect(button).toHaveText('Copied');
  return page.evaluate(() => navigator.clipboard.readText());
}

test('the battle log copies exactly, from Setup to the result, with no real opponent name', async ({ context, page, baseURL }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'], { origin: new URL(baseURL).origin });
  await page.goto('/docs/example');
  const text = await copied(page, 'Example battle log');
  expect(text).toBe(noFinalNewline(logFile));
  const lines = text.split('\n');
  expect(lines).toHaveLength(266);
  expect(lines[0]).toBe('Setup');
  expect(lines.at(-1)).toBe('Opponent took all of their Prize cards. Rival wins.');
  expect(text).toContain('Rival');
  expect(text).not.toMatch(/GatoruSoju/i);
  await expect(page.locator('main')).not.toContainText(/GatoruSoju/i);
});

test('the decklist copies exactly the corpus decklist', async ({ context, page, baseURL }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'], { origin: new URL(baseURL).origin });
  await page.goto('/docs/example');
  const text = await copied(page, 'Example decklist');
  expect(text).toBe(noFinalNewline(deckFile));
  expect(text.startsWith('Pokémon: 16')).toBe(true);
  expect(text.endsWith('Total Cards: 60')).toBe(true);
});

test('the example page suggests a prompt and the long log scrolls inside its box', async ({ page }) => {
  await page.goto('/docs/example');
  await expect(page.locator('.copy-block.prompt')).toContainText('Import it and review it with me');
  const box = await page.locator('.example-log pre').evaluate((pre) => ({
    scroll: pre.scrollHeight > pre.clientHeight,
    overflowY: getComputedStyle(pre).overflowY,
  }));
  expect(box.scroll).toBe(true);
  expect(['auto', 'scroll']).toContain(box.overflowY);
});

test('the import page says where to copy, what a complete import looks like and what to do about missing lines', async ({ page }) => {
  await page.goto('/docs/import');
  const prose = page.locator('.prose');
  await expect(page.getByRole('heading', { name: 'Where to copy them in TCG Live' })).toBeVisible();
  await expect(prose).toContainText('match history');
  await expect(prose).toContainText('deck editor');
  await expect(page.getByRole('heading', { name: 'What a complete import looks like' })).toBeVisible();
  await expect(prose).toContainText('no problems');
  await expect(page.getByRole('heading', { name: 'If lines are missing' })).toBeVisible();
  await expect(prose).toContainText('lines hidden');
  await expect(prose).toContainText('attach');
  await expect(prose.getByRole('link', { name: 'the example game' })).toHaveAttribute('href', '/docs/example');
});
