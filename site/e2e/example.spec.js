// The example game page: the complete anonymised battle log and its
// decklist, each copied exactly by its button; and the import page's
// directions.
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { test, expect } from './fixtures.js';

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

// luminance is a CSS colour's relative luminance, 0 (black) to 1 (white).
const luminance = (rgb) => {
  const [r, g, b] = rgb.match(/\d+(\.\d+)?/g).slice(0, 3).map(Number).map((c) => {
    const s = c / 255;
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
};

test('the decklist and the log sit in two copy panels, the decklist light and the log dark, each with its heading and Copy button', async ({ page }) => {
  await page.goto('/docs/example');
  const deck = page.locator('[data-panel="deck"]');
  const log = page.locator('[data-panel="log"]');
  await expect(deck.getByRole('heading', { level: 2 })).toHaveText('The decklist');
  await expect(log.getByRole('heading', { level: 2 })).toHaveText('The battle log');
  await expect(deck.getByRole('button', { name: 'Copy Example decklist' })).toBeVisible();
  await expect(log.getByRole('button', { name: 'Copy Example battle log' })).toBeVisible();
  // The decklist panel is light: nothing in it takes the dark theme.
  expect(await deck.evaluate((el) => el.closest('[data-theme="dark"]') === null && el.querySelector('[data-theme="dark"]') === null)).toBe(true);
  await expect(log).toHaveAttribute('data-theme', 'dark');
  const bg = (panel) => panel.locator('pre').evaluate((pre) => getComputedStyle(pre).backgroundColor);
  expect(luminance(await bg(deck)), 'the decklist is light').toBeGreaterThan(0.8);
  expect(luminance(await bg(log)), 'the log is dark').toBeLessThan(0.05);
  // The steps are numbered cards, the prompt in the second.
  await expect(page.locator('[data-steps] > li')).toHaveCount(2);
  await expect(page.locator('[data-steps] > li').nth(1).locator('.copy-block.prompt')).toHaveCount(1);
});

test('the example page keeps the space before its link to the import page', async ({ page }) => {
  await page.goto('/docs/example');
  await expect(page.locator('.prose')).toContainText('See importing a game for what to expect.');
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
