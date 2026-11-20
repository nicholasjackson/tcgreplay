// Screenshot baselines of the board in every layout: phone, tablet,
// desktop and wide (the landscape mat) from the fixed position, plus the
// desktop with the timeline docked below. They prove a change that should
// look the same does, and are re-recorded deliberately when the look
// changes (npx playwright test e2e/screenshots.spec.js --update-snapshots).
// Captures park the pointer and wait for finite animations, so they repeat.
import { test, expect } from './fixtures.js';

// settle parks the pointer clear of the board, waits for every finite
// animation to finish (gotcha board-boxes-settle-before-comparing), and
// waits for the timeline's smooth scroll to the newest line to stop:
// animations: 'disabled' does not stop smooth scrolling, and a capture
// mid-scroll shifts the timeline's text by a pixel.
async function settle(page) {
  await page.mouse.move(1, 1);
  await page.evaluate(() =>
    Promise.all(
      document
        .getAnimations()
        .filter((a) => a.effect?.getComputedTiming().endTime !== Infinity)
        .map((a) => a.finished.catch(() => {})),
    ),
  );
  await page.evaluate(
    () =>
      new Promise((resolve) => {
        const list = document.querySelector('.timeline');
        let last = list?.scrollTop;
        let still = 0;
        const tick = () => {
          const now = list?.scrollTop;
          still = now === last ? still + 1 : 0;
          last = now;
          if (still >= 10) resolve();
          else requestAnimationFrame(tick);
        };
        requestAnimationFrame(tick);
      }),
  );
}

test('the board looks as recorded', async ({ page }) => {
  await settle(page);
  await expect(page).toHaveScreenshot('board.png');
});

test('the board with the timeline docked below looks as recorded', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== 'desktop', 'the docked timeline is a desktop layout');
  await page.evaluate(() => localStorage.setItem('panelAt', 'bottom'));
  await page.reload();
  await expect(page.locator('header .dot.on')).toBeVisible();
  await expect(page.locator('.body.panel.bottom')).toBeVisible();
  await settle(page);
  await expect(page).toHaveScreenshot('board-docked.png');
});
