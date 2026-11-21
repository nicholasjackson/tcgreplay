// Studio cards: every face-down card shows the Studio back (navy, a cyan
// frame, the TCG Replay mark) in place of the old red and blue back, and
// text cards and Energy tokens are painted in the Studio card colours.
// Colours are written out from internal/design/tokens.json, not read back
// from the page, so a token pointing at the wrong value fails here.
import { readFileSync } from 'node:fs';
import { test, expect, isPhone, isWide } from './fixtures.js';

const studio = {
  navy: '#13285C', // brand-navy
  cyan: '#5CC8F2', // brand-cyan
  face: '#F4EFE3', // card-face
  faceTrainer: '#E9ECF1', // card-face-trainer
  ink: '#1B1F26', // card-face-ink
  borderPokemon: '#C9A64A', // card-border-pokemon
  borderTrainer: '#AEB6C2', // card-border-trainer
  hp: '#B42318', // card-hp
  fighting: '#C8693F', // energy-fighting
};
const mark = readFileSync(new URL('../../internal/design/mark.svg', import.meta.url), 'utf8');

// backs reads every face-down card on the page: its back's hook, colours
// (as upper-case hex) and whether it carries the mark.
function backs(page) {
  return page.evaluate(() => {
    const hex = (v) => {
      const m = /rgba?\((\d+),\s*(\d+),\s*(\d+)/.exec(v || '');
      return m ? '#' + m.slice(1, 4).map((n) => Number(n).toString(16).padStart(2, '0')).join('').toUpperCase() : v;
    };
    return [...document.querySelectorAll('.card.back')].map((c) => {
      const art = c.querySelector('.back-art');
      const s = art && getComputedStyle(art);
      const img = art?.querySelector('img.mark');
      return {
        title: c.title,
        back: art?.dataset.back ?? null,
        background: s && hex(s.backgroundColor),
        image: s?.backgroundImage,
        frame: s && hex(s.boxShadow),
        mark: img ? img.getAttribute('src') : null,
        markShown: !!img && img.complete && img.naturalWidth > 0 && img.getBoundingClientRect().width > 0,
      };
    });
  });
}

// markOf decodes the src the board gives the mark (the build inlines it as
// a data: URI; the dev server serves its path) into the SVG it draws.
async function markOf(page, src) {
  if (src.startsWith('data:')) {
    const [, meta, body] = /^data:([^,]*),(.*)$/s.exec(src);
    return meta.endsWith(';base64') ? Buffer.from(body, 'base64').toString('utf8') : decodeURIComponent(body);
  }
  return (await page.request.get(new URL(src, page.url()).href)).text();
}

test('every face-down card on the board shows the Studio back with the mark', async ({ page }, testInfo) => {
  // The phone's board shows its piles and hands as counters, so its
  // face-down cards are the prizes, seen in the prize view.
  if (isPhone(testInfo)) {
    await page.getByRole('group', { name: 'p2 counters' }).getByRole('button', { name: /^Prizes / }).click();
    await expect(page.locator('.modal .card.back').first()).toBeVisible();
  } else if (!isWide(testInfo)) {
    // Today's mat counts the opponent's hand in a chip; its view shows the
    // face-down cards.
    await page.getByRole('region', { name: 'p2 side' }).getByRole('button', { name: 'Hand 6' }).click();
    await expect(page.locator('.modal .card.back').first()).toBeVisible();
  }
  const found = await backs(page);
  expect(found.length, 'the board shows some face-down cards').toBeGreaterThan(0);
  for (const b of found) {
    expect(b, `${b.title}: the Studio back`).toMatchObject({ back: 'studio', background: studio.navy, image: 'none', frame: studio.cyan, markShown: true });
  }
  // Each back's mark is the TCG Replay mark itself.
  const srcs = [...new Set(found.map((b) => b.mark))];
  expect(srcs).toHaveLength(1);
  const svg = await markOf(page, srcs[0]);
  for (const part of ['<rect width="30" height="30" rx="8" fill="#13285C"', 'M15.2 17.2a2.2 2.2 0 1 0 2.2-2.2h-1']) {
    expect(svg).toContain(part);
    expect(mark).toContain(part);
  }
});

test('the old red and blue back is gone from the board', async ({ page }) => {
  const old = await page.evaluate(() =>
    [...document.querySelectorAll('.card')].filter((c) => /gradient/.test(getComputedStyle(c.querySelector('.back-art') ?? c).backgroundImage)).map((c) => c.title),
  );
  expect(old).toEqual([]);
});

test('text cards and Energy tokens use the Studio card colours', async ({ page }) => {
  const read = (sel) =>
    page.locator(sel).first().evaluate((el) => {
      const hex = (v) => {
        const m = /rgba?\((\d+),\s*(\d+),\s*(\d+)/.exec(v || '');
        return m ? '#' + m.slice(1, 4).map((n) => Number(n).toString(16).padStart(2, '0')).join('').toUpperCase() : v;
      };
      const s = getComputedStyle(el);
      const hp = el.querySelector('.hp');
      return {
        background: hex(s.backgroundColor),
        image: s.backgroundImage,
        border: hex(s.borderTopColor),
        ink: hex(s.color),
        hp: hp ? hex(getComputedStyle(hp).color) : null,
      };
    });
  // A Pokémon (Makuhita, on p1's Active) and a Trainer (Iono, in p1's hand).
  expect(await read('[aria-label="p1 Active"] .text-card')).toEqual({
    background: studio.face,
    image: 'none',
    border: studio.borderPokemon,
    ink: studio.ink,
    hp: studio.hp,
  });
  expect(await read('[aria-label="p1 hand"] .text-card[data-cat="Trainer"]')).toEqual({
    background: studio.faceTrainer,
    image: 'none',
    border: studio.borderTrainer,
    ink: studio.ink,
    hp: null,
  });
  // A Fighting Energy token is drawn in the Studio's Fighting colour.
  const token = await page
    .locator('[aria-label="p1 hand"] .token[data-type="Fighting"]')
    .first()
    .evaluate((el) => getComputedStyle(el).getPropertyValue('--ec').trim());
  expect(token.toUpperCase()).toBe(studio.fighting);
});
