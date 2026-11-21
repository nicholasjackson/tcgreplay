// Screenshot baselines of the board in every layout: phone, tablet,
// desktop and wide (the landscape mat) from the fixed position, plus the
// desktop with the timeline docked below, and a card's actions in each of
// their three containers: the desktop popover, the tablet panel and the
// phone sheet. They prove a change that should
// look the same does, and are re-recorded deliberately when the look
// changes (npx playwright test e2e/screenshots.spec.js --update-snapshots).
// Captures park the pointer and wait for finite animations, so they repeat.
import { test, expect, isTouch } from './fixtures.js';

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
        // Smooth scrolling can come to rest a pixel either side of the end,
        // so the capture pins the timeline to its very bottom.
        if (list) list.scrollTop = list.scrollHeight;
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

// A card's actions, opened on your Active Makuhita: right-clicked open as
// the popover on desktop, tapped open as the panel on a tablet and the
// full-screen sheet on a phone. The landscape mat's popover is the
// desktop's, so it is not recorded twice.
test('a card\'s actions look as recorded in their container', async ({ page }, testInfo) => {
  test.skip(testInfo.project.name === 'wide', 'the same popover as desktop');
  const active = page.getByRole('region', { name: 'p1 Active', exact: true }).locator('.card[title="Makuhita (p1-5)"]');
  if (isTouch(testInfo)) await active.tap();
  else await active.click({ button: 'right' });
  const sheet = page.locator('.sheet');
  await expect(sheet).toHaveAttribute('data-mode', isTouch(testInfo) ? testInfo.project.name : 'popover');
  await settle(page);
  await expect(page).toHaveScreenshot('menu.png');
});
