// The docs index: the docs' own address (/docs/ and /docs) answers with a
// page listing every docs page, not the 404 page; the index lists the same
// pages as the docs sidebar, in the same order, each with its one-line
// description; and every docs page's sidebar starts with a link to it.
import { readFileSync } from 'node:fs';
import { test, expect } from '@playwright/test';

// The docs sidebar, in order, after its "All docs" link.
const sidebar = [
  { name: 'Get started', href: '/docs/connect' },
  { name: 'Refreshing the connection', href: '/docs/connect#refresh' },
  { name: 'Signing in and opening the board', href: '/docs/sign-in' },
  { name: 'Running it on your desktop', href: '/docs/desktop' },
  { name: 'Importing a game', href: '/docs/import' },
  { name: 'Coaching workflows', href: '/docs/workflows' },
  { name: 'Rewind and try another line', href: '/docs/rewind' },
  { name: 'Example game', href: '/docs/example' },
  { name: 'Limits', href: '/docs/limits' },
  { name: "What's new", href: '/docs/whats-new' },
  { name: 'For developers', href: '/docs/developers' },
];

const notFound = readFileSync(new URL('../../internal/site/dist/app/404.html', import.meta.url), 'utf8');

test('the docs address answers with the docs index, with or without a trailing slash', async ({ request }) => {
  for (const path of ['/docs/', '/docs']) {
    const resp = await request.get(path);
    expect(resp.status(), path).toBe(200);
    expect(resp.headers()['content-type'], path).toBe('text/html; charset=utf-8');
    const body = await resp.text();
    expect(body, `${path} is not the 404 page`).not.toBe(notFound);
    expect(body, `${path} is the docs index`).toContain('data-docs-index');
  }
});

test('the docs index lists every page the sidebar lists, in the same order, with descriptions', async ({ page }) => {
  await page.goto('/docs/');
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Docs');
  const index = page.locator('[data-docs-index] > li');
  await expect(index).toHaveCount(sidebar.length);
  for (const [i, { name, href }] of sidebar.entries()) {
    const link = index.nth(i).getByRole('link');
    await expect(link, `index ${i}`).toHaveText(name);
    await expect(link, `index ${i}`).toHaveAttribute('href', href);
    await expect(index.nth(i).locator('p'), `${name} has a one-line description`).not.toBeEmpty();
  }
  // And the index matches the sidebar on the same page, link for link.
  const nav = page.getByRole('navigation', { name: 'Guide' });
  const navHrefs = await nav.getByRole('link').evaluateAll((as) => as.map((a) => a.getAttribute('href')));
  const indexHrefs = await page.locator('[data-docs-index] a').evaluateAll((as) => as.map((a) => a.getAttribute('href')));
  expect(navHrefs[0]).toBe('/docs');
  expect(indexHrefs).toEqual(navHrefs.slice(1));
});

test("every docs page's sidebar starts with a link to the docs index, then the docs pages in order", async ({ page }) => {
  for (const path of ['/docs', ...sidebar.map((s) => s.href).filter((h) => !h.includes('#'))]) {
    await page.goto(path);
    const links = page.getByRole('navigation', { name: 'Guide' }).getByRole('link');
    await expect(links, path).toHaveCount(sidebar.length + 1);
    await expect(links.first(), `${path}: first link`).toHaveText('All docs');
    await expect(links.first(), `${path}: first link`).toHaveAttribute('href', '/docs');
    for (const [i, { name, href }] of sidebar.entries()) {
      await expect(links.nth(i + 1), `${path}: link ${i + 1}`).toHaveText(name);
      await expect(links.nth(i + 1), `${path}: link ${i + 1}`).toHaveAttribute('href', href);
    }
  }
  // The index is marked as the current page only on the index.
  await page.goto('/docs/');
  await expect(page.getByRole('navigation', { name: 'Guide' }).getByRole('link', { name: 'All docs' })).toHaveAttribute('aria-current', 'page');
  await page.goto('/docs/connect');
  await expect(page.getByRole('navigation', { name: 'Guide' }).getByRole('link', { name: 'All docs' })).not.toHaveAttribute('aria-current', 'page');
});
