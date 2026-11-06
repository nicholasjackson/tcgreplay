import { defineConfig, devices } from '@playwright/test';

// The browser suite drives the built site (make site) served through the
// Go site handler, as the hosted playmat serves it, with the playmat's own
// addresses answering 503. Chromium only, like the board's suite:
// Playwright's WebKit needs Ubuntu's libraries. A 1366x768 laptop and a
// 375px phone (touch emulation).
const addr = '127.0.0.1:7791';

export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  reporter: [['list']],
  use: {
    baseURL: `http://${addr}`,
    trace: 'retain-on-failure',
  },
  projects: [
    { name: 'laptop', use: { ...devices['Desktop Chrome'], viewport: { width: 1366, height: 768 } } },
    { name: 'phone', use: { ...devices['iPhone 13'], browserName: 'chromium', viewport: { width: 375, height: 812 } } },
  ],
  webServer: {
    command: `go run ./internal/site/sitetest/cmd/serve -addr ${addr}`,
    cwd: '..',
    url: `http://${addr}/`,
    reuseExistingServer: false,
    timeout: 120_000,
  },
});
