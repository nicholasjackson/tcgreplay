// Card art from two addresses: the board loads a card's image, switches
// once to its fallback when that fails, and only then shows the text
// stand-in or Energy token. The fake card pool has no art, so the test
// adds a CDN image and a TCGdex fallback to every card the board asks
// for, and decides whether each host answers.
import { test, expect, isTouch, pile } from './fixtures.js';

const CDN = 'https://cdn.test.invalid/images/en/sv/sv1';
const TCGDEX = 'https://assets.tcgdex.net/en/sv/sv1';

// A 1x1 PNG; Chromium sniffs it whatever the content type says.
const PIXEL = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=', 'base64');

const region = (page, name) => page.getByRole('region', { name, exact: true });
const titled = (scope, title) => scope.locator(`.card[title="${title}"]`);
const riolu = (page) => titled(region(page, 'p1 bench 1'), 'Riolu (p1-1)');
const energy = (page) => titled(region(page, 'p1 Active'), 'Fighting Energy (p1-15)');

// serveImages gives every card info a CDN image and a TCGdex fallback,
// makes each host serve a pixel or fail as asked, and reloads the board so
// the card info is fetched again through the routes. It returns the image
// URLs requested from each host.
async function serveImages(page, { cdn, tcgdex, version }) {
  const requested = { cdn: [], tcgdex: [] };
  await page.route('**/api/card?def=*', async (route) => {
    const resp = await route.fetch();
    const info = await resp.json();
    const def = new URL(route.request().url()).searchParams.get('def');
    const extra = version ? { image_version: version } : {};
    await route.fulfill({ response: resp, json: { ...info, image: `${CDN}/${def}`, image_fallback: `${TCGDEX}/${def}`, ...extra } });
  });
  const host = (name, ok) => async (route) => {
    requested[name].push(route.request().url());
    if (ok) await route.fulfill({ body: PIXEL, contentType: 'image/webp' });
    else await route.abort();
  };
  await page.route('https://cdn.test.invalid/**', host('cdn', cdn));
  await page.route('https://assets.tcgdex.net/**', host('tcgdex', tcgdex));
  await page.reload();
  await expect(page.locator('header .dot.on')).toBeVisible();
  return requested;
}

// loaded waits for img to finish loading with a picture.
async function loaded(img) {
  await expect.poll(() => img.evaluate((el) => el.complete && el.naturalWidth)).toBeGreaterThan(0);
}

// zoomRiolu opens the zoom of the benched Riolu: a click zooms a card in
// every profile.
async function zoomRiolu(page) {
  const box = await riolu(page).boundingBox();
  await page.mouse.click(box.x + box.width / 2, box.y + box.height / 2);
  await expect(page.locator('.zoom')).toBeVisible();
}

test('a card image that loads from its address is shown with no request to the fallback', async ({ page }) => {
  const requested = await serveImages(page, { cdn: true, tcgdex: true });
  const img = riolu(page).locator('img');
  await expect(img).toHaveAttribute('src', `${CDN}/tst-1/low.webp`);
  await loaded(img);
  await loaded(energy(page).locator('img'));
  await zoomRiolu(page);
  const zoom = page.locator('.zoom img');
  await expect(zoom).toHaveAttribute('src', `${CDN}/tst-1/high.webp`);
  await loaded(zoom);
  expect(requested.cdn).toContain(`${CDN}/tst-1/low.webp`);
  expect(requested.tcgdex).toEqual([]);
});

test('a copy with a content version is requested with it, and the fallback without it', async ({ page }, testInfo) => {
  const v = 'abc123def456';
  const requested = await serveImages(page, { cdn: true, tcgdex: true, version: v });
  const img = riolu(page).locator('img');
  await expect(img).toHaveAttribute('src', `${CDN}/tst-1/low.webp?v=${v}`);
  await loaded(img);
  await zoomRiolu(page);
  const zoom = page.locator('.zoom img');
  await expect(zoom).toHaveAttribute('src', `${CDN}/tst-1/high.webp?v=${v}`);
  await loaded(zoom);
  expect(requested.cdn).toContain(`${CDN}/tst-1/low.webp?v=${v}`);
  expect(requested.cdn).toContain(`${CDN}/tst-1/high.webp?v=${v}`);
  expect(requested.tcgdex).toEqual([]);
  await page.mouse.click(5, 5);
  await expect(page.locator('.zoom')).toHaveCount(0);

  // The preview beside a pile's view follows the mouse: desktop only.
  if (!isTouch(testInfo)) {
    await pile(page, 'Discard').click();
    await page.locator('.modal .card[title="Fighting Energy (p1-16)"]').hover();
    const preview = page.locator('img.preview');
    await expect(preview).toHaveAttribute('src', `${CDN}/sve-006/high.webp?v=${v}`);
    await loaded(preview);
  }
});

test('when a versioned copy fails, the fallback is requested without the version', async ({ page }) => {
  const v = 'abc123def456';
  const requested = await serveImages(page, { cdn: false, tcgdex: true, version: v });
  const img = riolu(page).locator('img');
  await expect(img).toHaveAttribute('src', `${TCGDEX}/tst-1/low.webp`);
  await loaded(img);
  expect(requested.cdn).toContain(`${CDN}/tst-1/low.webp?v=${v}`);
  expect(requested.tcgdex).toContain(`${TCGDEX}/tst-1/low.webp`);
  expect(requested.tcgdex.every((u) => !u.includes('?v='))).toBe(true);
});

test('when the image address fails, the card, the zoom and the preview load the fallback', async ({ page }, testInfo) => {
  await serveImages(page, { cdn: false, tcgdex: true });
  const img = riolu(page).locator('img');
  await expect(img).toHaveAttribute('src', `${TCGDEX}/tst-1/low.webp`);
  await loaded(img);
  await expect(img).toBeVisible();
  await expect(riolu(page).locator('.text-card')).toHaveCount(0);

  await zoomRiolu(page);
  const zoom = page.locator('.zoom img');
  await expect(zoom).toHaveAttribute('src', `${TCGDEX}/tst-1/high.webp`);
  await loaded(zoom);
  await page.mouse.click(5, 5);
  await expect(page.locator('.zoom')).toHaveCount(0);

  // The preview beside a pile's view follows the mouse: desktop only.
  if (isTouch(testInfo)) return;
  await pile(page, 'Discard').click();
  await page.locator('.modal .card[title="Fighting Energy (p1-16)"]').hover();
  const preview = page.locator('img.preview');
  await expect(preview).toHaveAttribute('src', `${TCGDEX}/sve-006/high.webp`);
  await loaded(preview);
});

test('when both addresses fail, a Pokémon shows its text stand-in and an Energy its token', async ({ page }) => {
  const requested = await serveImages(page, { cdn: false, tcgdex: false });
  await expect(riolu(page).locator('.text-card')).toBeVisible();
  await expect(riolu(page).locator('img')).toHaveCount(0);
  await expect(energy(page).locator('.token')).toBeVisible();
  await expect(energy(page).locator('img')).toHaveCount(0);
  // Each address was tried once before giving up.
  expect(requested.cdn).toContain(`${CDN}/tst-1/low.webp`);
  expect(requested.tcgdex).toContain(`${TCGDEX}/tst-1/low.webp`);
});
