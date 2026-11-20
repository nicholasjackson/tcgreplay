// The connecting pages: "Get started" opens step-by-step instructions for
// claude.ai, ChatGPT and Claude Code, each ending with how to check it
// works; the playmat's address copies exactly; the sign-in and desktop
// pages are reachable and say what the plan promises. Addresses come from
// the site facts (TCGREPLAY_PUBLIC_URL in .do/app.yaml), not literals.
// The ChatGPT section names the plans that allow a custom connection, and
// starts with the Plugin Directory steps only once lib/listing.ts says TCG
// Replay is listed. One refresh section (#refresh) covers each assistant,
// and each assistant's troubleshooting text links to it.
import { test, expect } from '@playwright/test';
import { site } from '../src/lib/facts.ts';
import { chatgptDirectory, chatgptPlans } from '../src/lib/listing.ts';

// sectionsOf groups the connecting page's content by its h2 headings.
async function sectionsOf(page) {
  return page.locator('.prose').evaluate((prose) => {
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
          lists: els.filter((e) => e.tagName === 'OL').map((ol) => ol.textContent),
          text: els.map((e) => e.textContent).join('\n'),
          checkAt: els.findIndex((e) => /Check it works/.test(e.textContent)),
          last: els.length,
          prompt: els.some((e) => e.querySelector?.('code.language-prompt') || e.matches?.('.copy-block.prompt')),
          refreshLinks: els.reduce((n, e) => n + (e.matches?.('a[href="#refresh"]') ? 1 : 0) + (e.querySelectorAll?.('a[href="#refresh"]').length ?? 0), 0),
        },
      ]),
    );
  });
}

test('Get started on the home page opens the connecting instructions', async ({ page }) => {
  await page.goto('/');
  await page.locator('main').getByRole('link', { name: 'Get started' }).click();
  await expect(page).toHaveURL(/\/docs\/connect$/);
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Get started');
});

test('each assistant has its own numbered steps ending with a check that the connection works', async ({ page }) => {
  await page.goto('/docs/connect');
  const sections = await sectionsOf(page);
  for (const name of ['claude.ai', 'ChatGPT', 'Claude Code']) {
    const s = sections[name];
    expect(s, `${name} section`).toBeTruthy();
    expect(s.steps, `${name} numbered steps`).toBeGreaterThanOrEqual(2);
    expect(s.text, `${name} uses the playmat address`).toContain(site.mcpURL);
    expect(s.checkAt, `${name} has a "Check it works" step`).toBeGreaterThanOrEqual(0);
    expect(s.prompt, `${name} gives a prompt to check with`).toBe(true);
    expect(s.text, `${name} says what to expect`).toMatch(/should list/);
    expect(s.refreshLinks, `${name} troubleshooting links to the refresh steps`).toBeGreaterThanOrEqual(1);
  }
});

test('the ChatGPT section names the plans that allow a custom connection and creates it as a plugin', async ({ page }) => {
  await page.goto('/docs/connect');
  const chatgpt = (await sectionsOf(page)).ChatGPT;
  expect(chatgptPlans).toBe('Business, Enterprise or Edu');
  expect(chatgpt.text).toContain(`on a ${chatgptPlans} plan`);
  const custom = chatgpt.lists.find((l) => l.includes('Add custom MCP server'));
  expect(custom, 'custom connection steps').toBeTruthy();
  for (const step of ['Plugins', 'Server URL', 'OAuth', 'I understand and want to continue', 'Create as a plugin', 'Install']) {
    expect(custom, `custom steps: ${step}`).toContain(step);
  }
  expect(custom).toContain(site.mcpURL);
});

test('the ChatGPT section follows the Plugin Directory listing flag', async ({ page }) => {
  await page.goto('/docs/connect');
  const chatgpt = (await sectionsOf(page)).ChatGPT;
  const text = await page.locator('.prose').textContent();
  if (!chatgptDirectory.listed) {
    expect(chatgpt.lists[0], 'first ChatGPT steps are the custom ones').toContain('Add custom MCP server');
    expect(text, 'no mention of the Plugin Directory before it is listed').not.toMatch(/Plugin Directory/i);
  } else {
    expect(chatgpt.lists[0], 'first ChatGPT steps use the directory').toContain(chatgptDirectory.name);
    expect(chatgpt.text).toContain('Plugin Directory');
    expect(chatgpt.lists.slice(1).some((l) => l.includes('Add custom MCP server')), 'custom steps follow').toBe(true);
    expect((await sectionsOf(page))['Refresh after an update'].text).toMatch(/Plugin Directory, it updates on its own/);
  }
});

test('one refresh section gives steps for claude.ai, ChatGPT and Claude Code', async ({ page }) => {
  await page.goto('/docs/connect#refresh');
  const heading = page.locator('#refresh');
  await expect(heading).toHaveCount(1);
  await expect(heading).toHaveText('Refresh after an update');
  expect(await heading.evaluate((h) => h.tagName)).toBe('H2');
  const subs = await heading.evaluate((h) => {
    const out = [];
    for (let el = h.nextElementSibling; el && el.tagName !== 'H2'; el = el.nextElementSibling) {
      if (el.tagName === 'H3') out.push({ name: el.textContent.trim(), steps: 0 });
      else if (el.tagName === 'OL' && out.length) out[out.length - 1].steps += el.children.length;
    }
    return out;
  });
  expect(subs.map((s) => s.name)).toEqual(['claude.ai', 'ChatGPT', 'Claude Code']);
  for (const s of subs) expect(s.steps, `${s.name} refresh steps`).toBeGreaterThanOrEqual(2);
  await expect(page.locator('#refresh ~ ol').first()).toContainText(site.mcpURL);
});

test('the docs sidebar links to the refresh steps', async ({ page }) => {
  await page.goto('/docs/connect');
  await expect(page.getByRole('navigation', { name: 'Guide' }).getByRole('link', { name: 'Refreshing the connection' })).toHaveAttribute(
    'href',
    '/docs/connect#refresh',
  );
});

test("the playmat's address copies exactly the playmat's connection address", async ({ context, page, baseURL }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'], { origin: new URL(baseURL).origin });
  await page.goto('/docs/connect');
  const button = page.getByRole('button', { name: "Copy The playmat's address" });
  await expect(button).toBeVisible();
  await page.evaluate(() => navigator.clipboard.writeText(''));
  await button.click();
  await expect(button).toHaveText('Copied');
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(site.mcpURL);
});

test('the Claude Code command copies exactly, without a trailing newline', async ({ context, page, baseURL }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write'], { origin: new URL(baseURL).origin });
  await page.goto('/docs/connect');
  const block = page.locator('pre', { hasText: 'claude mcp add' });
  await block.locator('xpath=..').getByRole('button', { name: 'Copy' }).click();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
    `claude mcp add --transport http tcgreplay ${site.mcpURL}`,
  );
});

test('the connecting page has one note for old connectors, naming the address to reconnect at', async ({ page }) => {
  await page.goto('/docs/connect');
  const note = page.locator('[data-moved-note]');
  await expect(note).toHaveCount(1);
  await expect(note).toBeVisible();
  await expect(note).toContainText('tcgreplay.demo.gs');
  await expect(note).toContainText(site.mcpURL);
  await expect(note).toContainText(/remove it/);
  await expect(note).toHaveAttribute('id', 'moved');
});

test('the sign-in page is linked from the connecting page and explains Google and the board link', async ({ page }) => {
  await page.goto('/docs/connect');
  const link = page.locator('.prose').getByRole('link', { name: /signing in and opening the board/i }).first();
  await expect(link).toHaveAttribute('href', '/docs/sign-in');
  await link.click();
  await expect(page).toHaveURL(/\/docs\/sign-in$/);
  const prose = page.locator('.prose');
  await expect(prose).toContainText(/Google['’]s account chooser/);
  await expect(prose).toContainText(`${site.boardURL}?board=`);
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
