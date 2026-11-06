// The connecting pages: "Get started" opens step-by-step instructions for
// claude.ai, ChatGPT and Claude Code, each ending with how to check it
// works; the playmat's address copies exactly; the sign-in and desktop
// pages are reachable and say what the plan promises.
import { test, expect } from '@playwright/test';

test('Get started on the home page opens the connecting instructions', async ({ page }) => {
  await page.goto('/');
  await page.locator('main').getByRole('link', { name: 'Get started' }).click();
  await expect(page).toHaveURL(/\/docs\/connect$/);
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Get started');
});

test('each assistant has its own numbered steps ending with a check that the connection works', async ({ page }) => {
  await page.goto('/docs/connect');
  const sections = await page.locator('.prose').evaluate((prose) => {
    // Group the page's content by its h2 headings.
    const out = {};
    let current;
    for (const el of prose.children) {
      if (el.tagName === 'H2') {
        current = el.textContent.trim();
        out[current] = [];
      } else if (current) {
        out[current].push(el);
      }
    }
    return Object.fromEntries(
      Object.entries(out).map(([h, els]) => [
        h,
        {
          steps: els.filter((e) => e.tagName === 'OL').reduce((n, ol) => n + ol.children.length, 0),
          text: els.map((e) => e.textContent).join('\n'),
          checkAt: els.findIndex((e) => /Check it works/.test(e.textContent)),
          last: els.length,
          prompt: els.some((e) => e.querySelector?.('code.language-prompt') || e.matches?.('.copy-block.prompt')),
        },
      ]),
    );
  });
  for (const name of ['claude.ai', 'ChatGPT', 'Claude Code']) {
    const s = sections[name];
    expect(s, `${name} section`).toBeTruthy();
    expect(s.steps, `${name} numbered steps`).toBeGreaterThanOrEqual(2);
    expect(s.text, `${name} uses the playmat address`).toContain('https://tcgreplay.demo.gs/mcp');
    expect(s.checkAt, `${name} has a "Check it works" step`).toBeGreaterThanOrEqual(0);
    expect(s.prompt, `${name} gives a prompt to check with`).toBe(true);
    expect(s.text, `${name} says what to expect`).toMatch(/should list/);
  }
});

test("the playmat's address copies exactly https://tcgreplay.demo.gs/mcp", async ({ context, page, baseURL }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'], { origin: new URL(baseURL).origin });
  await page.goto('/docs/connect');
  const button = page.getByRole('button', { name: "Copy The playmat's address" });
  await expect(button).toBeVisible();
  await page.evaluate(() => navigator.clipboard.writeText(''));
  await button.click();
  await expect(button).toHaveText('Copied');
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe('https://tcgreplay.demo.gs/mcp');
});

test('the Claude Code command copies exactly, without a trailing newline', async ({ context, page, baseURL }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'], { origin: new URL(baseURL).origin });
  await page.goto('/docs/connect');
  const block = page.locator('pre', { hasText: 'claude mcp add' });
  await block.locator('xpath=..').getByRole('button', { name: 'Copy' }).click();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
    'claude mcp add --transport http tcgreplay https://tcgreplay.demo.gs/mcp',
  );
});

test('the sign-in page is linked from the connecting page and explains Google and the board link', async ({ page }) => {
  await page.goto('/docs/connect');
  const link = page.locator('.prose').getByRole('link', { name: /signing in and opening the board/i }).first();
  await expect(link).toHaveAttribute('href', '/docs/sign-in');
  await link.click();
  await expect(page).toHaveURL(/\/docs\/sign-in$/);
  const prose = page.locator('.prose');
  await expect(prose).toContainText(/Google['’]s account chooser/);
  await expect(prose).toContainText('https://tcgreplay.demo.gs/board?board=');
  await expect(prose).toContainText('same browser');
});

test('the desktop page lists prerequisites, install steps, the desktop app setting and the board address', async ({ page }) => {
  await page.goto('/docs/connect');
  await page.locator('.prose').getByRole('link', { name: /running it on your desktop|run it on your own computer/i }).first().click();
  await expect(page).toHaveURL(/\/docs\/desktop$/);
  const prose = page.locator('.prose');
  for (const text of ['Go 1.27', 'Node.js 22', 'make install', 'claude_desktop_config.json', 'mcpServers', 'restart the Claude desktop app', 'http://127.0.0.1:7777']) {
    await expect(prose).toContainText(text);
  }
});
