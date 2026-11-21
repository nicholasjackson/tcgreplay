// Screenshot baselines of every website page, at laptop and phone widths.
// They prove a change that should look the same does, and are re-recorded
// deliberately when the look changes
// (npx playwright test e2e/screenshots.spec.js --update-snapshots).
// Reduced motion keeps any transition from catching a page mid-change.
import { test, expect } from './fixtures.js';
import { crawl } from './crawl.js';

test.use({ reducedMotion: 'reduce' });

// shotName names a page's screenshot after its path: / is home.png,
// /docs/connect is docs-connect.png.
function shotName(path) {
  const slug = path.replace(/^\/+|\/+$/g, '').replace(/\//g, '-');
  return `${slug || 'home'}.png`;
}

test('every page looks as recorded', async ({ browser, page }, testInfo) => {
  test.setTimeout(180_000);
  // Every page the crawl reaches, and the 404 page.
  const paths = (await crawl(browser, testInfo.project.use.baseURL)).map((p) => p.path);
  if (!paths.includes('/no-such-page')) paths.push('/no-such-page');
  for (const path of paths) {
    await page.goto(path);
    await page.evaluate(() => document.fonts.ready);
    await page.mouse.move(1, 1);
    await expect.soft(page, path).toHaveScreenshot(shotName(path), { fullPage: true });
  }
});
