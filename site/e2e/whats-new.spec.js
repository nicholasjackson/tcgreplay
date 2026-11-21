// What's new: the dated list of player-visible changes
// (src/data/whats-new.yaml) is read and checked while the site builds, so
// a missing or malformed entry stops the build with a message naming the
// entry and the field; the page lists the entries newest first, each
// saying whether a refresh is needed and linking to the refresh steps when
// it is; the first entry is the public launch. The page is linked from the
// docs sidebar and the docs index.
import { readFileSync } from 'node:fs';
import { test, expect } from './fixtures.js';
import { loadWhatsNew, changeHTML, whatsNew } from '../src/lib/whats-new.ts';

const realFile = () => readFileSync(new URL('../src/data/whats-new.yaml', import.meta.url), 'utf8');

const entry = ({ date = '2026-12-01', title = 'An update', refresh = 'false', changes = '\n    - "A change"' } = {}) =>
  [
    '-',
    date === null ? '' : `  date: ${date}`,
    title === null ? '' : `  title: ${title}`,
    refresh === null ? '' : `  refresh: ${refresh}`,
    changes === null ? '' : `  changes:${changes}`,
  ]
    .filter(Boolean)
    .join('\n')
    .replace('-\n  ', '- ');

test.describe('the What\'s new list', () => {
  test('the project list loads newest first and starts (oldest entry) with the public launch, which needs a refresh', () => {
    const entries = loadWhatsNew(realFile());
    expect(entries.length).toBeGreaterThan(0);
    const dates = entries.map((e) => e.date);
    expect(dates).toEqual([...dates].sort().reverse());
    expect(entries.at(-1).title).toMatch(/public/i);
    expect(entries.at(-1).refresh).toBe(true);
    expect(whatsNew).toEqual(entries);
  });

  test('the launch entry links the move note, names neither the old address nor the Plugin Directory, and lists the terms, privacy and About panel', () => {
    const launch = loadWhatsNew(realFile()).at(-1);
    const text = launch.changes.join('\n');
    expect(text).toContain('(/docs/connect#moved)');
    expect(text).toContain('https://tcgreplay.app');
    expect(text).not.toContain('demo.gs');
    expect(text).not.toMatch(/plugin directory/i);
    expect(text).toContain('(/terms)');
    expect(text).toContain('(/privacy)');
    expect(text).toMatch(/About tcgreplay panel/);
  });

  test('entries out of order come back newest first, same-day entries in file order', () => {
    const yaml = [
      entry({ date: '2026-11-20', title: 'Oldest' }),
      entry({ date: '2027-01-05', title: 'Newest' }),
      entry({ date: '2026-12-01', title: 'Middle A' }),
      entry({ date: '2026-12-01', title: 'Middle B' }),
    ].join('\n');
    expect(loadWhatsNew(yaml).map((e) => e.title)).toEqual(['Newest', 'Middle A', 'Middle B', 'Oldest']);
  });

  test('a well-formed entry keeps its fields', () => {
    expect(loadWhatsNew(entry({ refresh: 'true', changes: '\n    - "One"\n    - "Two"' }))).toEqual([
      { date: '2026-12-01', title: 'An update', refresh: true, changes: ['One', 'Two'] },
    ]);
  });

  const broken = [
    { name: 'missing date', fixture: { date: null }, want: /entry 2 \("An update"\) has no date/ },
    { name: 'a day that does not exist', fixture: { date: '2026-02-30' }, want: /entry 2 \("An update"\) has date "2026-02-30", which is not a real day/ },
    { name: 'a date in another format', fixture: { date: '"1 Dec 2026"' }, want: /entry 2 \("An update"\) has date "1 Dec 2026"/ },
    { name: 'missing title', fixture: { title: null }, want: /entry 2 \("2026-12-01"\) has no title/ },
    { name: 'refresh written as a word', fixture: { refresh: '"yes"' }, want: /entry 2 \("An update"\) has refresh "yes"; it must be true or false/ },
    { name: 'missing refresh', fixture: { refresh: null }, want: /entry 2 \("An update"\) has refresh missing; it must be true or false/ },
    { name: 'an empty change list', fixture: { changes: ' []' }, want: /entry 2 \("An update"\) has no changes/ },
    { name: 'missing changes', fixture: { changes: null }, want: /entry 2 \("An update"\) has no changes/ },
    { name: 'an empty change', fixture: { changes: '\n    - ""' }, want: /entry 2 \("An update"\) has an empty change \(change 1\)/ },
  ];
  for (const { name, fixture, want } of broken) {
    test(`an entry with ${name} fails, naming the entry and the field`, () => {
      const yaml = [entry({ date: '2027-01-01', title: 'Fine' }), entry(fixture)].join('\n');
      expect(() => loadWhatsNew(yaml, 'whats-new.yaml')).toThrow(/^what's new: whats-new\.yaml /);
      expect(() => loadWhatsNew(yaml, 'whats-new.yaml')).toThrow(want);
    });
  }

  test('an empty list or a non-list fails', () => {
    expect(() => loadWhatsNew('[]')).toThrow(/at least one entry/);
    expect(() => loadWhatsNew('date: 2026-12-01')).toThrow(/at least one entry/);
  });

  test('change lines are escaped, and only site paths and https links become links', () => {
    expect(changeHTML('a <b> & "c"')).toBe('a &lt;b&gt; &amp; &quot;c&quot;');
    expect(changeHTML('see [Get started](/docs/connect)')).toBe('see <a href="/docs/connect">Get started</a>');
    expect(changeHTML('[OpenAI](https://openai.com)')).toBe('<a href="https://openai.com">OpenAI</a>');
    expect(changeHTML('[bad](javascript:alert(1))')).not.toContain('<a');
    expect(changeHTML('[plain](http://example.com)')).toBe('[plain](http://example.com)');
  });
});

test.describe("the What's new page", () => {
  test('lists every entry, dated newest first, each saying whether a refresh is needed', async ({ page }) => {
    const resp = await page.goto('/docs/whats-new');
    expect(resp.status()).toBe(200);
    await expect(page.getByRole('heading', { level: 1 })).toHaveText("What's new");
    const entries = page.locator('[data-entry]');
    await expect(entries).toHaveCount(whatsNew.length);
    const dates = await page.locator('[data-entry] h2 time[datetime]').evaluateAll((ts) => ts.map((t) => t.getAttribute('datetime')));
    expect(dates).toEqual([...dates].sort().reverse());
    expect(dates).toEqual(whatsNew.map((e) => e.date));
    await expect(entries.first().locator('h2')).toHaveText(new RegExp(`— ${whatsNew[0].title}$`));
    for (let i = 0; i < whatsNew.length; i++) {
      const refresh = entries.nth(i).locator('[data-refresh]');
      const text = (await refresh.textContent()).trim();
      expect(text, `entry ${i + 1}`).toMatch(/^(Refresh needed|No refresh needed)/);
      if (text.startsWith('Refresh needed')) {
        await expect(refresh.locator('a[href="/docs/connect#refresh"]'), `entry ${i + 1} links the refresh steps`).toHaveCount(1);
      }
      expect(text.startsWith('Refresh needed'), `entry ${i + 1} matches its refresh flag`).toBe(whatsNew[i].refresh);
    }
  });

  test('the launch entry links the move note and the refresh steps, and shows its date in words', async ({ page }) => {
    await page.goto('/docs/whats-new');
    const launch = page.locator('[data-entry]').last();
    await expect(launch.locator('a[href="/docs/connect#moved"]')).toHaveCount(1);
    await expect(launch.locator('[data-refresh]')).toContainText('Refresh needed');
    await expect(launch.locator('time')).toHaveText(/^\d{1,2} [A-Z][a-z]+ \d{4}$/);
    // Links keep the space before them.
    await expect(launch).toContainText('see the note on connecting');
  });

  test('the entries hang on a timeline, newest at the top, each dated above its title', async ({ page }) => {
    await page.goto('/docs/whats-new');
    const entries = page.locator('[data-whats-new] > li[data-entry]');
    await expect(entries).toHaveCount(whatsNew.length);
    const shape = await entries.evaluateAll((lis) =>
      lis.map((li) => {
        const dot = getComputedStyle(li, '::before');
        const rail = getComputedStyle(li, '::after');
        const time = li.querySelector('h2 time').getBoundingClientRect();
        const title = li.querySelector('h2').getBoundingClientRect();
        return {
          top: li.getBoundingClientRect().top,
          date: li.dataset.date,
          dot: dot.content !== 'none' && dot.borderRadius === '50%' && parseFloat(dot.width) > 0,
          rail: rail.content !== 'none' && parseFloat(rail.width) > 0 && rail.position === 'absolute',
          // The date is its own line above the title, inside the heading.
          dateAbove: time.bottom <= title.bottom - 1 && time.height < title.height,
        };
      }),
    );
    expect(shape.map((e) => e.date)).toEqual(whatsNew.map((e) => e.date));
    for (const [i, e] of shape.entries()) {
      expect(e.dot, `entry ${i + 1} has its dot`).toBe(true);
      expect(e.rail, `entry ${i + 1} has its rail`).toBe(true);
      expect(e.dateAbove, `entry ${i + 1} is dated above its title`).toBe(true);
      if (i > 0) expect(e.top, `entry ${i + 1} is below entry ${i}`).toBeGreaterThan(shape[i - 1].top);
    }
    // A refresh-needed entry shows it as a callout, its link kept.
    const needed = page.locator('[data-entry] [data-refresh].needed').first();
    if (whatsNew.some((e) => e.refresh)) {
      await expect(needed).toBeVisible();
      await expect(needed.locator('a[href="/docs/connect#refresh"]')).toHaveCount(1);
      expect(await needed.evaluate((el) => getComputedStyle(el).backgroundColor)).not.toBe('rgba(0, 0, 0, 0)');
    }
  });

  test('the docs sidebar, the docs index and the refresh steps link to it', async ({ page }) => {
    await page.goto('/docs/connect');
    await expect(page.getByRole('navigation', { name: 'Guide' }).getByRole('link', { name: "What's new" })).toHaveAttribute('href', '/docs/whats-new');
    await expect(page.locator('#refresh + p a[href="/docs/whats-new"]')).toHaveCount(1);
    await page.goto('/docs');
    await expect(page.locator('[data-docs-index] a[href="/docs/whats-new"]')).toHaveText("What's new");
  });
});
