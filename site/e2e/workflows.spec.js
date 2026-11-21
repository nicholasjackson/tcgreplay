// Every coaching workflow the playmat offers has a page with at least one
// copyable example prompt and the description from its guide, listed in
// groups (after a game, practice, play) and in the playmat's own order
// within each group, each card with its guide name and an example prompt;
// the rewind page explains branching with a prompt. The guides are read here from the playmat's files, independently
// of the site's own loader.
import { readFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';
import { parse } from 'yaml';
import { test, expect } from './fixtures.js';

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

const pagesDir = join(process.cwd(), 'src', 'content', 'docs', 'workflows');

// readGroups maps each guide to the group its workflow page names, read
// from the pages' frontmatter independently of the site's loader.
function readGroups() {
  return Object.fromEntries(
    readdirSync(pagesDir)
      .filter((f) => f.endsWith('.md'))
      .map((f) => {
        const m = /^---\n([\s\S]*?)\n---/.exec(readFileSync(join(pagesDir, f), 'utf8'));
        expect(m, `${f} frontmatter`).not.toBeNull();
        const fm = parse(m[1]);
        return [fm.guide, fm.group];
      }),
  );
}

// The groups in the order the workflows page shows them, with their headings.
const groups = [
  { id: 'after-a-game', title: 'After a game' },
  { id: 'practice', title: 'Practice' },
  { id: 'play', title: 'Play' },
];

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

test('each workflow page names its group: import and review after a game, the drills practice, playing a game play', () => {
  // Hand-maintained, like the list of seven above: a new guide must be given a group.
  expect(readGroups()).toEqual({
    'import-game': 'after-a-game',
    'opponent-deck': 'after-a-game',
    'review-game': 'after-a-game',
    'replay-drill': 'practice',
    'before-you-move': 'practice',
    'numbers-drill': 'practice',
    'play-game': 'play',
  });
});

test('the workflows page lists them in groups, After a game, Practice and Play, each in the playmat order, with their guide names and a prompt', async ({ page }) => {
  await page.goto('/docs/workflows');
  const sections = page.locator('[data-workflow-group]');
  expect(await sections.evaluateAll((ss) => ss.map((s) => s.dataset.workflowGroup))).toEqual(groups.map((g) => g.id));
  const order = readOrder();
  const byGroup = readGroups();
  for (const g of groups) {
    const section = page.locator(`[data-workflow-group="${g.id}"]`);
    await expect(section.getByRole('heading', { level: 2 })).toHaveText(g.title);
    const names = await section.locator('[data-workflows] > li').evaluateAll((lis) => lis.map((li) => li.dataset.guide));
    expect(names, `${g.title}: the playmat order`).toEqual(order.filter((n) => byGroup[n] === g.id));
  }
  // Every guide is listed exactly once, across the groups.
  const items = page.locator('[data-workflows] > li');
  const names = await items.evaluateAll((lis) => lis.map((li) => li.dataset.guide));
  expect([...names].sort()).toEqual([...order].sort());
  for (const name of names) {
    const li = page.locator(`[data-workflows] > li[data-guide="${name}"]`);
    // The card names its guide and shows an example prompt from the
    // workflow's page; the guide's description is on that page.
    await expect(li).toContainText(name);
    await expect(li.locator('.bubble')).not.toBeEmpty();
    await expect(li.getByRole('link')).toHaveAttribute('href', `/docs/workflows/${name}`);
  }
  // As the Studio design draws it, the page has no docs sidebar and no On
  // this page rail: the cards use the full column.
  await expect(page.getByRole('navigation', { name: 'Docs', exact: true })).toHaveCount(0);
  await expect(page.getByRole('navigation', { name: 'On this page' })).toHaveCount(0);
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
