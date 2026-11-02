// Shared setup for the browser suite: every test starts from the board
// test server's fixed position, and any browser dialog fails it.
import { test as base, expect } from '@playwright/test';

export const test = base.extend({
  page: async ({ page, request }, use) => {
    const reset = await request.post('/test/reset');
    expect(reset.ok(), 'reset the board test server').toBeTruthy();
    const dialogs = [];
    page.on('dialog', (d) => {
      dialogs.push(`${d.type()}: ${d.message()}`);
      // The failure is reported after the test; dismissing only unblocks
      // the page, and may race the page closing.
      d.dismiss().catch(() => {});
    });
    await page.goto('/');
    await expect(page.locator('header .dot.on')).toBeVisible();
    await use(page);
    expect(dialogs, 'no browser dialog opens').toEqual([]);
  },
});

export { expect };

// isTouch reports whether the test runs in a touch profile.
export function isTouch(testInfo) {
  return !!testInfo.project.use.hasTouch;
}

// card finds a face-up card by its name (its title is "Name (iid)").
export function card(scope, name) {
  return scope.locator(`.card[title^="${name} ("]`).first();
}

// tapCard taps (touch) or clicks (mouse) a card by name.
export async function tapCard(page, name, testInfo) {
  const c = card(page, name);
  if (isTouch(testInfo)) await c.tap();
  else await c.click();
}

function centre(box) {
  return { x: box.x + box.width / 2, y: box.y + box.height / 2 };
}

// point is a locator's centre, or a point given as {x, y}, scrolled into
// view first so a finger or the mouse can reach it.
async function point(target) {
  if (!target.boundingBox) return target;
  await target.scrollIntoViewIfNeeded();
  return centre(await target.boundingBox());
}

// lerp walks from a to b in steps, calling step at each point.
async function lerp(a, b, steps, step) {
  for (let i = 1; i <= steps; i++) {
    await step({ x: a.x + ((b.x - a.x) * i) / steps, y: a.y + ((b.y - a.y) * i) / steps });
  }
}

// touchStart puts a finger down on a locator (or point) with real touch
// input through the DevTools protocol, so the browser applies
// touch-action and scrolls natively, as on a device. It returns the
// finger: move it to a locator or point, then lift it.
export async function touchStart(page, at) {
  const cdp = await page.context().newCDPSession(page);
  let p = await point(at);
  const touch = (type, q) => cdp.send('Input.dispatchTouchEvent', { type, touchPoints: q ? [{ x: q.x, y: q.y, id: 1 }] : [] });
  await touch('touchStart', p);
  return {
    async move(to, { steps = 12 } = {}) {
      const b = to.boundingBox ? centre(await to.boundingBox()) : to;
      await lerp(p, b, steps, (q) => touch('touchMove', q));
      p = b;
    },
    async end() {
      await touch('touchEnd');
      await cdp.detach();
    },
  };
}

// mouseStart presses the mouse on a locator (or point), returning the
// same handle as touchStart.
export async function mouseStart(page, at) {
  const p = await point(at);
  await page.mouse.move(p.x, p.y);
  await page.mouse.down();
  return {
    async move(to, { steps = 12 } = {}) {
      const b = to.boundingBox ? centre(await to.boundingBox()) : to;
      await page.mouse.move(b.x, b.y, { steps });
    },
    async end() {
      await page.mouse.up();
    },
  };
}

// LIFT is how far a drag first moves up from the card: in the hand only
// a mostly upward move lifts a card. The target is found after the lift,
// so it can be the play area, which shows only while a card is dragged.
const LIFT = 30;

// dragWith lifts a card with a pointer handle, then takes it to a target.
async function dragWith(start, page, from, to, steps) {
  const h = await start(page, from);
  const a = centre(await from.boundingBox());
  await h.move({ x: a.x, y: a.y - LIFT }, { steps: 4 });
  await h.move(to, { steps });
  await h.end();
}

// touchDrag drags from one locator to another with a finger.
export async function touchDrag(page, from, to, { steps = 12 } = {}) {
  await dragWith(touchStart, page, from, to, steps);
}

// mouseDrag drags from one locator to another with the mouse.
export async function mouseDrag(page, from, to, { steps = 12 } = {}) {
  await dragWith(mouseStart, page, from, to, steps);
}

// drag drags by touch in the touch profiles and by mouse on the desktop.
export async function drag(page, from, to, testInfo) {
  if (isTouch(testInfo)) await touchDrag(page, from, to);
  else await mouseDrag(page, from, to);
}

// press puts a finger down (touch profiles) or presses the mouse
// (desktop) on a locator or point, returning a handle to move and end.
export function press(page, at, testInfo) {
  return isTouch(testInfo) ? touchStart(page, at) : mouseStart(page, at);
}

// act sends a board verb over the page's own websocket, as the board does.
export async function act(page, verb, args) {
  return page.evaluate(
    ({ verb, args }) =>
      new Promise((resolve, reject) => {
        const ws = new WebSocket(`ws://${location.host}/ws`);
        ws.onopen = () => ws.send(JSON.stringify({ id: 1, verb, args }));
        ws.onmessage = (ev) => {
          const msg = JSON.parse(ev.data);
          if (msg.type === 'result') {
            ws.close();
            msg.ok ? resolve(msg) : reject(new Error(msg.error));
          }
        };
      }),
    { verb, args },
  );
}

// timelineLines returns the timeline's lines, oldest first. On a phone
// the timeline is a sheet behind ☰, so while it is closed the lines come
// from the board's own state, the log the timeline shows.
export async function timelineLines(page) {
  const shown = page.locator('.line .text');
  if (await shown.count()) return shown.allInnerTexts();
  return page.evaluate(
    () =>
      new Promise((resolve) => {
        const ws = new WebSocket(`ws://${location.host}/ws`);
        ws.onmessage = (ev) => {
          const msg = JSON.parse(ev.data);
          if (msg.type !== 'state' || !msg.snapshot) return;
          ws.close();
          resolve((msg.snapshot.log || []).filter((l) => !/^turn \d+ start/.test(l.text)).map((l) => l.text));
        };
      }),
  );
}

// pile finds a seat's deck, discard or Lost Zone by name ("Deck"): the
// pile beside the mat, or on a phone its counter chip. pileLabel is the
// text that counts it ("Discard 1").
export function pile(page, name, seat = 'p1') {
  const re = new RegExp(`^${name} `);
  const mat = page.getByRole('region', { name: `${seat} side` }).locator('.pile', { has: page.locator('.label', { hasText: re }) });
  const chip = page.getByRole('group', { name: `${seat} counters` }).getByRole('button', { name: re });
  return mat.or(chip);
}
export function pileLabel(page, name, seat = 'p1') {
  const re = new RegExp(`^${name} `);
  const mat = page.getByRole('region', { name: `${seat} side` }).locator('.pile .label', { hasText: re });
  const chip = page.getByRole('group', { name: `${seat} counters` }).getByRole('button', { name: re });
  return mat.or(chip);
}

// isPhone reports whether the test runs at phone size.
export function isPhone(testInfo) {
  return testInfo.project.name === 'phone';
}
