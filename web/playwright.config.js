import { defineConfig, devices } from '@playwright/test';

// The browser suite drives the built board (make web) against the board
// test server, a real game over the fake card pool from a fixed position.
// There is one shared game, so tests run one at a time and reset it first.
// The touch profiles take the iPhone and iPad sizes but run in Chromium's
// touch emulation: Playwright's WebKit needs Ubuntu's libraries and does
// not run on every developer's machine. Real iOS Safari is a manual check.
// A second board server, with its own game, adds Cloudflare's
// visitor-counting beacon to the board page as hosted mode does with a
// token; only e2e/analytics.spec.js uses it, and it stubs or blocks
// Cloudflare, so no test sends anything there.
const addr = '127.0.0.1:7790';
const analyticsAddr = '127.0.0.1:7789';
const analyticsToken = '0123456789abcdef0123456789abcdef';

export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  workers: 1,
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
    {
      name: 'phone',
      use: { ...devices['iPhone 13'], browserName: 'chromium', viewport: { width: 390, height: 844 } },
    },
    {
      name: 'tablet',
      use: { ...devices['iPad (gen 7) landscape'], browserName: 'chromium', viewport: { width: 1024, height: 768 } },
    },
    {
      name: 'desktop',
      use: { ...devices['Desktop Chrome'], viewport: { width: 1440, height: 900 } },
    },
    {
      // A big monitor: the board area beside the timeline is over 1600px
      // wide, so the board is the landscape mat.
      name: 'wide',
      use: { ...devices['Desktop Chrome'], viewport: { width: 2560, height: 1440 } },
    },
  ],
  webServer: [
    {
      command: `go run ./internal/webui/webtest/cmd/boardtest -addr ${addr}`,
      cwd: '..',
      url: `http://${addr}/`,
      reuseExistingServer: false,
      timeout: 120_000,
    },
    {
      command: `go run ./internal/webui/webtest/cmd/boardtest -addr ${analyticsAddr} -analytics-token ${analyticsToken}`,
      cwd: '..',
      url: `http://${analyticsAddr}/`,
      reuseExistingServer: false,
      timeout: 120_000,
    },
  ],
});
