// Card art from an ordered list of sources: the board tries each source
// in turn (a base path's address, then its fallback, with the size file
// appended; a complete image file as is), moves on when one fails, and
// only after the last shows the text stand-in or Energy token. The fake
// card pool has no art, so the tests give the cards the board asks for an
// image list, and decide whether each host answers.
import { test, expect, isTouch, pile } from './fixtures.js';

const CDN = 'https://cdn.test.invalid/images/en/sv/sv1';
const TCGDEX = 'https://assets.tcgdex.net/en/sv/sv1';
const MALIE = 'https://cdn.malie.io/file/malie-io/tcgl/cards/jpg/en/sv1';

// A 1x1 PNG; Chromium sniffs it whatever the content type says.
const PIXEL = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=', 'base64');

const region = (page, name) => page.getByRole('region', { name, exact: true });
const titled = (scope, title) => scope.locator(`.card[title="${title}"]`);
const riolu = (page) => titled(region(page, 'p1 bench 1'), 'Riolu (p1-1)');
const energy = (page) => titled(region(page, 'p1 Active'), 'Fighting Energy (p1-15)');

// serveList gives every card info the image list imagesFor(def) returns,
// makes every image host serve a pixel when ok(url) says so and a 404
// otherwise, and reloads the board so the card info is fetched again
// through the routes. It returns the image URLs requested, in order.
async function serveList(page, imagesFor, ok) {
  const requested = [];
  await page.route('**/api/card?def=*', async (route) => {
    const resp = await route.fetch();
    const info = await resp.json();
    const def = new URL(route.request().url()).searchParams.get('def');
    await route.fulfill({ response: resp, json: { ...info, images: imagesFor(def) } });
  });
  const host = async (route) => {
    const url = route.request().url();
    requested.push(url);
    if (ok(url)) await route.fulfill({ body: PIXEL, contentType: 'image/webp' });
    else await route.fulfill({ status: 404, body: 'not found' });
  };
  for (const h of ['https://cdn.test.invalid/**', 'https://assets.tcgdex.net/**', 'https://cdn.malie.io/**']) {
    await page.route(h, host);
  }
  await page.reload();
  await expect(page.locator('header .dot.on')).toBeVisible();
  return requested;
}

// serveImages gives every card info one base-path source: a CDN copy (with
// its content version, when given) and a TCGdex fallback; each host serves
// a pixel or fails as asked. It returns the image URLs requested from each
// host.
async function serveImages(page, { cdn, tcgdex, version }) {
  const extra = version ? { image_version: version } : {};
  const all = await serveList(
    page,
    (def) => [{ image: `${CDN}/${def}`, image_fallback: `${TCGDEX}/${def}`, ...extra }],
    (url) => (url.startsWith(CDN) ? cdn : tcgdex),
  );
  return {
    get cdn() {
      return all.filter((u) => u.startsWith(CDN));
    },
    get tcgdex() {
      return all.filter((u) => u.startsWith(TCGDEX));
    },
  };
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

// file is a malie.io card image file for def.
const file = (def) => `${MALIE}/${def}.jpg`;

test('a complete image file is loaded as is on the mat, in the zoom and in the preview', async ({ page }, testInfo) => {
  const requested = await serveList(page, (def) => [{ image: file(def), image_file: true }], () => true);
  const img = riolu(page).locator('img');
  await expect(img).toHaveAttribute('src', file('tst-1'));
  await loaded(img);
  await zoomRiolu(page);
  const zoom = page.locator('.zoom img');
  await expect(zoom).toHaveAttribute('src', file('tst-1'));
  await loaded(zoom);
  await page.mouse.click(5, 5);
  await expect(page.locator('.zoom')).toHaveCount(0);
  expect(requested.filter((u) => u.includes('tst-1'))).toEqual([file('tst-1')]);

  if (isTouch(testInfo)) return;
  await pile(page, 'Discard').click();
  await page.locator('.modal .card[title="Fighting Energy (p1-16)"]').hover();
  const preview = page.locator('img.preview');
  await expect(preview).toHaveAttribute('src', file('sve-006'));
  await loaded(preview);
});

test('when a base path fails at its copy and its original, the next source, a file, is shown', async ({ page }) => {
  const requested = await serveList(
    page,
    (def) => [{ image: `${CDN}/${def}`, image_fallback: `${TCGDEX}/${def}` }, { image: file(def), image_file: true }],
    (url) => url.startsWith(MALIE),
  );
  const img = riolu(page).locator('img');
  await expect(img).toHaveAttribute('src', file('tst-1'));
  await loaded(img);
  expect(requested.filter((u) => u.includes('tst-1'))).toEqual([`${CDN}/tst-1/low.webp`, `${TCGDEX}/tst-1/low.webp`, file('tst-1')]);
});

test('when a file fails, the next source, a base path, is loaded with its size', async ({ page }) => {
  await serveList(page, (def) => [{ image: file(def), image_file: true }, { image: `${TCGDEX}/${def}` }], (url) => !url.startsWith(MALIE));
  const img = riolu(page).locator('img');
  await expect(img).toHaveAttribute('src', `${TCGDEX}/tst-1/low.webp`);
  await loaded(img);
  await zoomRiolu(page);
  const zoom = page.locator('.zoom img');
  await expect(zoom).toHaveAttribute('src', `${TCGDEX}/tst-1/high.webp`);
  await loaded(zoom);
});

test('when every source fails, each is tried in order before the text stand-in', async ({ page }) => {
  const requested = await serveList(
    page,
    (def) => [{ image: file(def), image_file: true }, { image: `${CDN}/${def}`, image_fallback: `${TCGDEX}/${def}` }],
    () => false,
  );
  await expect(riolu(page).locator('.text-card')).toBeVisible();
  await expect(riolu(page).locator('img')).toHaveCount(0);
  await expect(energy(page).locator('.token')).toBeVisible();
  expect(requested.filter((u) => u.includes('tst-1/') || u.endsWith('tst-1.jpg'))).toEqual([file('tst-1'), `${CDN}/tst-1/low.webp`, `${TCGDEX}/tst-1/low.webp`]);
});

test('a card with no image sources shows its text stand-in', async ({ page }) => {
  const requested = await serveList(page, () => [], () => true);
  await expect(riolu(page).locator('.text-card')).toBeVisible();
  await expect(riolu(page).locator('img')).toHaveCount(0);
  expect(requested).toEqual([]);
});

test('the order of the sources decides which the board tries first', async ({ page }) => {
  let fileFirst = true;
  await serveList(
    page,
    (def) => {
      const list = [{ image: file(def), image_file: true }, { image: `${TCGDEX}/${def}` }];
      return fileFirst ? list : list.reverse();
    },
    () => true,
  );
  await expect(riolu(page).locator('img')).toHaveAttribute('src', file('tst-1'));
  fileFirst = false;
  await page.reload();
  await expect(page.locator('header .dot.on')).toBeVisible();
  await expect(riolu(page).locator('img')).toHaveAttribute('src', `${TCGDEX}/tst-1/low.webp`);
});
