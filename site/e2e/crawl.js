// The crawl both website suites share: every page reachable from / by
// same-origin links.
import { expect } from '@playwright/test';

// pagePath is a same-origin URL's path, without query or fragment.
export function pagePath(href, origin) {
  const u = new URL(href, origin);
  if (u.origin !== origin) return undefined;
  return u.pathname;
}

// crawl visits / and every same-origin link it reaches, breadth-first and
// deduplicated, returning each page's path and HTTP status. prepare, if
// given, sets up the crawl's browser context first (routes, say).
export async function crawl(browser, baseURL, prepare) {
  const origin = new URL(baseURL).origin;
  const context = await browser.newContext({ baseURL });
  if (prepare) await prepare(context);
  const page = await context.newPage();
  const seen = new Set(['/']);
  const queue = ['/'];
  const pages = [];
  try {
    while (queue.length) {
      const path = queue.shift();
      const resp = await page.goto(path);
      expect(resp, `a response for ${path}`).not.toBeNull();
      expect([200, 404], `${path} answers a page`).toContain(resp.status());
      pages.push({ path, status: resp.status() });
      const hrefs = await page.locator('a[href]').evaluateAll((as) => as.map((a) => a.href));
      for (const href of hrefs) {
        const p = pagePath(href, origin);
        if (p !== undefined && !seen.has(p)) {
          seen.add(p);
          queue.push(p);
        }
      }
    }
  } finally {
    await context.close();
  }
  return pages;
}
