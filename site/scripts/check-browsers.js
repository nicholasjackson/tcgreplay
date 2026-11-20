// check-browsers stops the browser suite with the install command when the
// browser Playwright needs is missing, rather than letting every test fail
// on a launch error.
import { existsSync } from 'node:fs';
import { chromium } from '@playwright/test';

if (!existsSync(chromium.executablePath())) {
  console.error("Playwright's Chromium browser is not installed.");
  console.error('Run: cd site && npx playwright install chromium');
  process.exit(1);
}
