import { defineConfig, devices } from '@playwright/test';

// The browser suite drives the built site (make site) served through the
// Go site handler, as the hosted playmat serves it, with the playmat's own
// addresses answering 503. Chromium only, like the board's suite:
// Playwright's WebKit needs Ubuntu's libraries. A 1366x768 laptop and a
// 375px phone (touch emulation). A second server on its own port adds
// Cloudflare's visitor-counting beacon to every page, as hosted mode does
// with a token; only e2e/analytics.spec.js uses it, and it stubs or blocks
// Cloudflare, so no test sends anything there.
const addr = '127.0.0.1:7791';
const analyticsAddr = '127.0.0.1:7792';
const analyticsToken = '0123456789abcdef0123456789abcdef';

export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  reporter: [['list']],
  // Screenshot baselines (e2e/screenshots.spec.js) live per project under
  // e2e/__screenshots__, Chromium only, taken on the developer's machine.
  snapshotPathTemplate: '{testDir}/__screenshots__/{projectName}/{arg}{ext}',
  expect: { toHaveScreenshot: { maxDiffPixelRatio: 0.002, scale: 'css', animations: 'disabled', caret: 'hide' } },
  use: {
    baseURL: `http://${addr}`,
    trace: 'retain-on-failure',
  },
  projects: [
    { name: 'laptop', use: { ...devices['Desktop Chrome'], viewport: { width: 1366, height: 768 } } },
    { name: 'phone', use: { ...devices['iPhone 13'], browserName: 'chromium', viewport: { width: 375, height: 812 } } },
  ],
  webServer: [
    {
      command: `go run ./internal/site/sitetest/cmd/serve -addr ${addr}`,
      cwd: '..',
      url: `http://${addr}/`,
      reuseExistingServer: false,
      timeout: 120_000,
    },
    {
      command: `go run ./internal/site/sitetest/cmd/serve -addr ${analyticsAddr} -analytics-token ${analyticsToken}`,
      cwd: '..',
      url: `http://${analyticsAddr}/`,
      reuseExistingServer: false,
      timeout: 120_000,
    },
  ],
});
