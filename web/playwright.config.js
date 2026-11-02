import { defineConfig, devices } from '@playwright/test';

// The browser suite drives the built board (make web) against the board
// test server, a real game over the fake card pool from a fixed position.
// There is one shared game, so tests run one at a time and reset it first.
// The touch profiles take the iPhone and iPad sizes but run in Chromium's
// touch emulation: Playwright's WebKit needs Ubuntu's libraries and does
// not run on every developer's machine. Real iOS Safari is a manual check.
const addr = '127.0.0.1:7790';

export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  workers: 1,
  forbidOnly: !!process.env.CI,
  reporter: [['list']],
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
  ],
  webServer: {
    command: `go run ./internal/webui/webtest/cmd/boardtest -addr ${addr}`,
    cwd: '..',
    url: `http://${addr}/`,
    reuseExistingServer: false,
    timeout: 120_000,
  },
});
