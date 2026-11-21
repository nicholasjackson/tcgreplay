// Shared setup for the website's browser suite: the site loads IBM Plex
// from Google Fonts, and every test serves those requests from the local
// copy in testdata/fonts instead, so no test reaches Google and every run
// renders the same. Specs import test and expect from here, not from
// @playwright/test; contexts a spec makes itself (crawl.js) call
// serveFonts.
import { test as base, expect } from '@playwright/test';
import { fileURLToPath } from 'node:url';
import { basename, join } from 'node:path';

// fontHosts are the only other hosts the website asks for anything: the
// Google Fonts stylesheet and the font files it names.
export const fontHosts = ['fonts.googleapis.com', 'fonts.gstatic.com'];

const fonts = fileURLToPath(new URL('../../testdata/fonts/', import.meta.url));

// serveFonts answers a browser context's Google Fonts requests from
// testdata/fonts, as the board's suite does (web/e2e/fixtures.js).
export async function serveFonts(context) {
  await context.route('https://fonts.googleapis.com/**', (route) =>
    route.fulfill({ path: join(fonts, 'css2.css'), contentType: 'text/css' }),
  );
  await context.route('https://fonts.gstatic.com/**', (route) =>
    route.fulfill({
      path: join(fonts, basename(new URL(route.request().url()).pathname)),
      contentType: 'font/woff2',
      headers: { 'Access-Control-Allow-Origin': '*' },
    }),
  );
}

export const test = base.extend({
  context: async ({ context }, use) => {
    await serveFonts(context);
    await use(context);
  },
});

export { expect };
