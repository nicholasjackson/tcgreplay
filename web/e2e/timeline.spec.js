// The timeline's turn rewind and play: every line plays on from its event
// and rewinds to just after it; every turn's and every player's half's
// Rewind and Play act on its start (the turn begun and its opening draw in
// hand); the controls show without a hover and are a fingertip's 44px on a
// touch screen; and, docked below the board, the timeline shares the
// height with the board through its handle, and its turn list jumps to any
// player's half and shows the prizes left after each turn.
import { test, expect, act, isTouch, isPhone, mouseStart, touchStart } from './fixtures.js';

// snapshot reads the board's state from the server, as the board gets it:
// the branch, its head, its log and the branches.
function snapshot(page) {
  return page.evaluate(
    () =>
      new Promise((resolve) => {
        const ws = new WebSocket(`ws://${location.host}/ws`);
        ws.onmessage = (ev) => {
          const msg = JSON.parse(ev.data);
          if (msg.type !== 'state' || !msg.snapshot) return;
          ws.close();
          resolve(msg.snapshot);
        };
      }),
  );
}

// hit taps (touch) or clicks (mouse) a control.
function hit(locator, testInfo) {
  return isTouch(testInfo) ? locator.tap() : locator.click();
}

// openTimeline shows the phone's timeline sheet (behind ☰); elsewhere the
// timeline is already beside the board.
async function openTimeline(page, testInfo) {
  if (!isPhone(testInfo)) return;
  await hit(page.getByRole('button', { name: 'Menu' }), testInfo);
  await hit(page.getByRole('menuitem', { name: 'Show timeline' }), testInfo);
  await expect(page.locator('aside.sheet')).toBeVisible();
}

// bare reports whether the timeline runs without per-line and heading
// buttons, as the design has a touch screen's timeline beside the board:
// there tapping a line shows its Play and Rewind.
const bare = (testInfo) => testInfo.project.name === 'tablet';

// lineButton finds a line's Play or Rewind (by name), tapping the
// line first to show them where the timeline has no per-line buttons.
async function lineButton(l, name, testInfo) {
  if (bare(testInfo)) {
    const row = l.locator('.row');
    if ((await row.getAttribute('aria-expanded')) !== 'true') await hit(row, testInfo);
  }
  return l.getByRole('button', { name });
}

// lastLine is the id of the timeline's last line, the board's head.
async function lastLine(page) {
  return Number((await page.locator('.line .id').last().innerText()).slice(1));
}
const line = (page, text) => page.locator('.line').filter({ hasText: text });
const idOf = async (l) => Number((await l.locator('.id').innerText()).slice(1));

// fresh reloads the board so it shows the latest position live, with
// replays at half speed so each step holds still long enough to check.
// (Moves made over the test's own connection reach the board as another
// client's, which it would otherwise hold and autoplay.)
async function fresh(page) {
  await page.evaluate(() => localStorage.setItem('replaySpeed', '0.5'));
  await page.reload();
  await expect(page.locator('header .dot.on')).toBeVisible();
}

// playTwoTurns plays on from the fixed position (Alice's turn 1, her
// opening card already put back) through both players' turn 1 and turn 2,
// each half with a move after its opening draw; Alice takes a prize in
// turn 2, and Bob's turn 2 is in progress with one move made.
async function playTwoTurns(page) {
  await act(page, 'end_turn', { seat: 'p1' });
  await act(page, 'move', { iid: 'p2-2', to: 'bench:1', seat: 'p2' });
  await act(page, 'end_turn', { seat: 'p2' });
  await act(page, 'move', { iid: 'p1-2', to: 'bench:2' });
  await act(page, 'take_prize', { seat: 'p1' });
  await act(page, 'end_turn', { seat: 'p1' });
  await act(page, 'move', { iid: 'p2-7', to: 'discard', seat: 'p2' });
}

// The starts the timeline's Rewind and Play act on, read off the log by
// its text: a player's half starts at their opening draw, just after
// "turn N start (seat)"; a whole turn at its first player's (Alice's); and
// setup ends at the line before turn 1 starts.
function openingDraw(log, turn, seat) {
  const i = log.findIndex((l) => l.text === `turn ${turn} start (${seat})`);
  expect(i, `turn ${turn} start (${seat}) is logged`).toBeGreaterThan(0);
  expect(log[i + 1].text, `turn ${turn} ${seat} opens with a draw`).toMatch(new RegExp(`^${seat} drew `));
  return log[i + 1].id;
}
function endOfSetup(log) {
  const i = log.findIndex((l) => l.text === 'turn 1 start (p1)');
  return log[i - 1].id;
}
// next is the id of the line after id: the first step a replay from id shows.
const next = (log, id) => log[log.findIndex((l) => l.id === id) + 1].id;

// The starts, by control: [name of the heading's Rewind, of its Play, the
// start] for setup and each player's half of turns 1 and 2. The timeline
// has one heading per half, and none for a whole turn (the Studio design).
function starts(log) {
  return [
    ['Rewind to the end of setup', 'Play from the end of setup', endOfSetup(log)],
    ['Rewind to the start of your turn 1', 'Play from the start of your turn 1', openingDraw(log, 1, 'p1')],
    ["Rewind to the start of Bob's turn 1", "Play from the start of Bob's turn 1", openingDraw(log, 1, 'p2')],
    ['Rewind to the start of your turn 2', 'Play from the start of your turn 2', openingDraw(log, 2, 'p1')],
    ["Rewind to the start of Bob's turn 2", "Play from the start of Bob's turn 2", openingDraw(log, 2, 'p2')],
  ];
}

// lineAt is the timeline line with event id.
const lineAt = (page, id) => page.locator('.line').filter({ has: page.locator('.id', { hasText: new RegExp(`^#${id}$`) }) });

// expectHead waits for the board to be on a branch other than main whose
// head is id, its timeline ending at that line.
async function expectHead(page, id, what) {
  await expect.poll(async () => (await snapshot(page)).head, `${what}: the head`).toBe(id);
  const snap = await snapshot(page);
  expect(snap.branch, `${what}: a branch of its own`).not.toBe('main');
  await expect.poll(() => lastLine(page), `${what}: the timeline ends at #${id}`).toBe(id);
}

// backToMain checks main out again and waits for its timeline.
async function backToMain(page, head) {
  await act(page, 'checkout', { branch: 'main' });
  await expect.poll(() => lastLine(page)).toBe(head);
}

// The current step's first line, during a replay.
const nowLine = (page) => page.locator('.line.now .id').first();

// goLive leaves a replay with the playback controls' Go live. On a phone
// they float over the board, under the timeline sheet, which closes for
// them and opens again after.
async function goLive(page, testInfo) {
  if (isPhone(testInfo)) await hit(page.getByRole('button', { name: 'Close the timeline' }), testInfo);
  await hit(page.getByRole('toolbar', { name: 'Playback' }).getByRole('button', { name: 'Go live' }), testInfo);
  await expect(page.getByRole('toolbar', { name: 'Playback' })).toHaveCount(isPhone(testInfo) ? 0 : 1);
  await openTimeline(page, testInfo);
  await expect(page.locator('.line.now')).toHaveCount(0);
}

test('a line\'s Play from here plays on from its event, and its Rewind to here rewinds to just after it on a branch of its own', async ({ page }, testInfo) => {
  // Two moves after the fixed position: Riolu benched, then an Energy
  // discarded.
  await act(page, 'move', { iid: 'p1-2', to: 'bench:2' });
  await act(page, 'move', { iid: 'p1-17', to: 'discard' });
  await fresh(page);
  await openTimeline(page, testInfo);
  const riolu = line(page, 'p1-2 Riolu p1:hand → p1:bench:2');
  const energy = line(page, 'p1-17 Fighting Energy p1:hand → p1:discard');
  const id = await idOf(riolu);
  expect(await idOf(energy)).toBe(id + 1);

  // Play from here: a replay starts, its first step that line's event.
  await hit(await lineButton(riolu, `Play from #${id}`, testInfo), testInfo);
  await expect(nowLine(page)).toHaveText(`#${id}`);
  await expect(page.locator('.line.now')).toHaveCount(1);
  // The position reads that event, of the head.
  await expect(page.getByRole('toolbar', { name: 'Playback' }).locator('.pos')).toHaveText(`#${id} / ${id + 1}`);
  await goLive(page, testInfo);

  // Rewind to here: the board is at just after that event, on a new
  // branch, and the later move is gone from it.
  await hit(await lineButton(riolu, `Rewind to just after #${id}`, testInfo), testInfo);
  await expect.poll(() => lastLine(page)).toBe(id);
  await expect(energy).toHaveCount(0);
  const snap = await snapshot(page);
  expect(snap.head).toBe(id);
  expect(snap.branches.map((b) => b.name).sort()).toEqual(['at-' + id, 'main']);
  // The branch picker gains the branch, chosen, at that event (behind ☰
  // on a phone).
  if (isPhone(testInfo)) {
    await hit(page.getByRole('button', { name: 'Close the timeline' }), testInfo);
    await hit(page.getByRole('button', { name: 'Menu' }), testInfo);
  }
  const branch = page.getByRole('combobox', { name: 'branch' });
  await expect(branch.locator('option')).toHaveCount(2);
  await expect(branch).toHaveValue(`at-${id}`);
  await expect(branch.locator('option:checked')).toHaveText(`at-${id} · #${id}`);
});

test('each player\'s half\'s Rewind lands on its start, and setup\'s on the end of setup', async ({ page }, testInfo) => {
  await playTwoTurns(page);
  await fresh(page);
  await openTimeline(page, testInfo);
  const { log, head } = await snapshot(page);
  const tl = page.locator('.timeline');
  for (const [rewind, , start] of starts(log)) {
    // A touch screen's headings have no buttons: the start's own line
    // rewinds there.
    if (bare(testInfo)) await hit(await lineButton(lineAt(page, start), `Rewind to just after #${start}`, testInfo), testInfo);
    else await hit(tl.getByRole('button', { name: rewind, exact: true }), testInfo);
    await expectHead(page, start, rewind);
    await backToMain(page, head);
  }
});

test('each player\'s half\'s Play replays from its start, and setup\'s from the end of setup', async ({ page }, testInfo) => {
  test.setTimeout(60_000); // five replays, each waited on to its first step
  await playTwoTurns(page);
  await fresh(page);
  await openTimeline(page, testInfo);
  const { log } = await snapshot(page);
  const tl = page.locator('.timeline');
  for (const [, play, start] of starts(log)) {
    // A touch screen's headings have no buttons: the line after the start
    // plays from there.
    const first = next(log, start);
    if (bare(testInfo)) await hit(await lineButton(lineAt(page, first), `Play from #${first}`, testInfo), testInfo);
    else await hit(tl.getByRole('button', { name: play, exact: true }), testInfo);
    // The replay's first step is the event just after the start.
    await expect(nowLine(page), play).toHaveText(`#${next(log, start)}`);
    await goLive(page, testInfo);
  }
});

test('the timeline\'s play and rewind show without a hover, a fingertip\'s 44px on a touch screen, and the playback controls head it', async ({ page }, testInfo) => {
  await playTwoTurns(page);
  await fresh(page);
  // Park the pointer away from the timeline.
  if (!isTouch(testInfo)) await page.mouse.move(5, 5);
  await openTimeline(page, testInfo);
  const tl = page.locator('.timeline');
  const lines = tl.locator('.line');
  const n = await lines.count();
  expect(n).toBeGreaterThan(100);
  const touch = isTouch(testInfo);

  if (bare(testInfo)) {
    // A touch screen's timeline beside the board, as the design has it:
    // no per-line or heading buttons, each line at least 44px tall, and a
    // tapped line shows its Play and Rewind (an icon and one word, as on
    // the half headings), 44px each.
    await expect(tl.locator('.acts')).toHaveCount(0);
    await expect(tl.locator('.head-btn')).toHaveCount(0);
    await expect(tl.locator('h3')).toHaveCount(5);
    for (const l of [lines.first(), lines.nth(Math.floor(n / 2)), lines.nth(n - 2)]) {
      await l.scrollIntoViewIfNeeded();
      expect((await l.locator('.row').boundingBox()).height).toBeGreaterThanOrEqual(43.5);
      for (const title of ['Play from here', 'Rewind to here']) {
        const id = Number((await l.locator('.id').innerText()).slice(1));
        const name = title === 'Play from here' ? `Play from #${id}` : `Rewind to just after #${id}`;
        const c = await lineButton(l, name, testInfo);
        await expect(c).toBeVisible();
        await expect(c).toHaveCSS('opacity', '1');
        await expect(c).toHaveText(title === 'Play from here' ? 'Play' : 'Rewind');
        await expect(c).toHaveClass(/\bhead-btn\b/);
        const box = await c.boundingBox();
        expect(box.width, name).toBeGreaterThanOrEqual(43.5);
        expect(box.height, name).toBeGreaterThanOrEqual(43.5);
      }
    }
    // The last line has no rewind.
    await hit(lines.last().locator('.row'), testInfo);
    await expect(lines.last().getByTitle('Play from here')).toHaveCount(1);
    await expect(lines.last().getByTitle('Rewind to here')).toHaveCount(0);
  } else {
    await expect(lines.locator('.acts button[title="Play from here"]')).toHaveCount(n);
    // Every line but the last has its rewind.
    await expect(lines.locator('.acts button[title="Rewind to here"]')).toHaveCount(n - 1);
    // One heading per player's half, and setup's, each with Rewind and
    // Play; no heading for a whole turn.
    await expect(tl.locator('h3')).toHaveCount(5);
    await expect(tl.locator('h3 .head-btn')).toHaveCount(2 * 5);

    // Every control: opaque all the way up (no hover reveals it), shown,
    // and, on a touch screen, at least 44×44.
    const faults = await tl.evaluate((el, touch) => {
      const out = [];
      for (const b of el.querySelectorAll('.acts button, .head-btn')) {
        const name = b.getAttribute('aria-label');
        for (let e = b; e && e !== el; e = e.parentElement) {
          const cs = getComputedStyle(e);
          if (cs.opacity !== '1' || cs.visibility !== 'visible' || cs.display === 'none') out.push(`${name}: hidden (${e.className})`);
        }
        const r = b.getBoundingClientRect();
        if (touch && (r.width < 43.5 || r.height < 43.5)) out.push(`${name}: ${r.width}×${r.height}`);
        if (!touch && (r.width === 0 || r.height === 0)) out.push(`${name}: no size`);
      }
      return out;
    }, touch);
    expect(faults).toEqual([]);

    // Scrolled to, the first, a middle and the last lines' controls and
    // every heading's are on screen, at full opacity, with nothing hovered.
    const some = [lines.first(), lines.nth(Math.floor(n / 2)), lines.nth(n - 2), lines.last()];
    const controls = [...some.flatMap((l) => [l.getByTitle('Play from here'), l.getByTitle('Rewind to here')]), ...(await tl.locator('.head-btn').all())];
    for (const c of controls) {
      if (!(await c.count())) continue; // the last line has no rewind
      await c.scrollIntoViewIfNeeded();
      await expect(c).toBeVisible();
      await expect(c).toBeInViewport();
      await expect(c).toHaveCSS('opacity', '1');
      if (touch) {
        const box = await c.boundingBox();
        expect(box.width, await c.getAttribute('aria-label')).toBeGreaterThanOrEqual(43.5);
        expect(box.height, await c.getAttribute('aria-label')).toBeGreaterThanOrEqual(43.5);
      }
    }
  }

  const bar = page.getByRole('toolbar', { name: 'Playback' });
  if (isPhone(testInfo)) {
    // A phone's sheet holds the lines; its playback controls float over
    // the board only during a replay.
    await expect(bar).toHaveCount(0);
    await lines.last().getByTitle('Play from here').tap();
    await expect(bar).toBeVisible();
    for (const name of ['Previous step', 'Pause', 'Next step', 'Go live']) await expect(bar.getByRole('button', { name })).toBeVisible();
  } else {
    // Beside the board the playback controls head the timeline, live too.
    const aside = page.getByRole('complementary', { name: 'Timeline' });
    await expect(aside.getByRole('toolbar', { name: 'Playback' })).toBeVisible();
    for (const name of ['Previous step', 'Play', 'Next step']) await expect(bar.getByRole('button', { name, exact: true })).toBeVisible();
    // Go live: in the row, except on a touch screen while the board is live
    // (the design drops it there).
    await expect(bar.getByRole('button', { name: 'Go live', exact: true })).toHaveCount(bare(testInfo) ? 0 : 1);
    await expect(bar.getByRole('slider', { name: 'Position' })).toBeVisible();
    await expect(bar.getByRole('combobox')).toBeVisible();
    if (touch) {
      for (const name of ['Previous step', 'Play', 'Next step']) {
        const box = await bar.getByRole('button', { name, exact: true }).boundingBox();
        expect(box.width, name).toBeGreaterThanOrEqual(43.5);
        expect(box.height, name).toBeGreaterThanOrEqual(43.5);
      }
    }
  }
});

// dock moves the timeline below the board.
async function dock(page, testInfo) {
  // Beside the board, the dock button is in ☰ (the Studio design).
  await hit(page.getByRole('button', { name: 'Menu' }), testInfo);
  await hit(page.getByRole('menuitem', { name: 'Move timeline below the board' }), testInfo);
  await expect(page.locator('.body')).toHaveClass(/\bbottom\b/);
}
const grip = (page) => page.getByRole('separator', { name: 'Resize the timeline' });
const rail = (page) => page.getByRole('navigation', { name: 'Turns' });
const riolu = (page) => page.locator('.card[title="Riolu (p1-1)"]').first();
const height = async (loc) => (await loc.boundingBox()).height;

test('docked below, dragging the handle up gives the timeline height and the board less, its cards shrinking, and keys move it too; the height is not kept', async ({ page }, testInfo) => {
  test.skip(isPhone(testInfo), 'the phone timeline is a sheet');
  // A touch screen's cards are already at their 48px floor under a docked
  // timeline, so there the board scrolls (from its top) rather than its
  // cards shrinking.
  const touch = isTouch(testInfo);
  await dock(page, testInfo);
  const aside = page.getByRole('complementary', { name: 'Timeline' });
  const main = page.locator('main');
  const handle = grip(page);
  await expect(handle).toBeVisible();
  await expect(handle).toHaveAttribute('aria-orientation', 'horizontal');
  const min = Number(await handle.getAttribute('aria-valuemin'));
  const max = Number(await handle.getAttribute('aria-valuemax'));
  expect(max - min, 'room to share').toBeGreaterThan(100);

  const before = { aside: await height(aside), main: await height(main), card: (await riolu(page).boundingBox()).width };
  expect(Number(await handle.getAttribute('aria-valuenow'))).toBe(Math.round(before.aside));

  // Drag the handle up, as far as the board leaves room for (80px at most).
  const up = Math.min(80, max - Math.round(before.aside));
  expect(up, 'room to grow the timeline').toBeGreaterThanOrEqual(40);
  const box = await handle.boundingBox();
  const h = await (touch ? touchStart : mouseStart)(page, handle);
  await h.move({ x: box.x + box.width / 2, y: box.y + box.height / 2 - up });
  await h.end();
  await expect.poll(() => height(aside)).toBeGreaterThanOrEqual(before.aside + up - 2);
  await expect.poll(() => height(main)).toBeLessThanOrEqual(before.main - up + 2);
  if (!touch) await expect.poll(async () => (await riolu(page).boundingBox()).width).toBeLessThan(before.card);
  else {
    // The board scrolls from its top: nothing above the area is out of reach.
    const top = await main.evaluate((m) => {
      m.scrollTop = 0;
      // Today's mat lays each half out as display: contents, its rows the
      // mat's own: the first row is the first box.
      const flat = (els) => els.flatMap((c) => (getComputedStyle(c).display === 'contents' ? flat([...c.children]) : [c]));
      const first = flat([...m.children]).filter((c) => getComputedStyle(c).position !== 'absolute').sort((a, b) => a.getBoundingClientRect().top - b.getBoundingClientRect().top)[0];
      return first.getBoundingClientRect().top - m.getBoundingClientRect().top;
    });
    expect(top).toBeGreaterThanOrEqual(0);
  }
  const dragged = Number(await handle.getAttribute('aria-valuenow'));
  expect(dragged).toBe(Math.round(await height(aside)));

  // The keys: ArrowUp and ArrowDown 16px, End and Home to the bounds.
  await handle.focus();
  await page.keyboard.press('ArrowDown');
  await expect(handle).toHaveAttribute('aria-valuenow', String(dragged - 16));
  await page.keyboard.press('ArrowUp');
  await expect(handle).toHaveAttribute('aria-valuenow', String(dragged));
  await page.keyboard.press('Home');
  await expect(handle).toHaveAttribute('aria-valuenow', String(min));
  await page.keyboard.press('End');
  await expect(handle).toHaveAttribute('aria-valuenow', String(max));

  // The height is kept for this page only: nothing new in the browser's
  // storage, and a reload is back at the layout's own height, still docked.
  const settings = ['autoplay', 'panelAt', 'prizeReminders', 'replaySpeed'];
  const keys = await page.evaluate(() => Object.keys(localStorage));
  for (const k of keys) expect(settings, `localStorage key ${k}`).toContain(k);
  expect(await page.evaluate(() => sessionStorage.length)).toBe(0);
  await page.reload();
  await expect(page.locator('header .dot.on')).toBeVisible();
  await expect(page.locator('.body')).toHaveClass(/\bbottom\b/);
  await expect.poll(() => height(aside)).toBeCloseTo(before.aside, 0);
});

test('docked below, the turn list rewinds to any player\'s half and shows the prizes left after each turn', async ({ page }, testInfo) => {
  test.skip(isPhone(testInfo), 'the phone timeline is a sheet');
  await playTwoTurns(page);
  await fresh(page);
  await dock(page, testInfo);
  const turns = rail(page);
  await expect(turns).toBeVisible();
  const { log, head } = await snapshot(page);

  // The prizes left after each turn: Alice took one in turn 2.
  await expect(turns.getByLabel('Prizes left after setup: Alice 6, Bob 6')).toHaveText('6–6');
  await expect(turns.getByLabel('Prizes left after turn 1: Alice 6, Bob 6')).toHaveText('6–6');
  await expect(turns.getByLabel('Prizes left after turn 2: Alice 5, Bob 6')).toHaveText('5–6');

  // A button per player's half, each rewinding to that half's start, at
  // least 44px on a touch screen; setup has none (the design's TurnRail).
  await expect(turns.getByRole('button', { name: 'Rewind to the end of setup' })).toHaveCount(0);
  const halves = [
    ['Rewind to your turn 1', openingDraw(log, 1, 'p1')],
    ["Rewind to Bob's turn 1", openingDraw(log, 1, 'p2')],
    ['Rewind to your turn 2', openingDraw(log, 2, 'p1')],
    ["Rewind to Bob's turn 2", openingDraw(log, 2, 'p2')],
  ];
  await expect(turns.getByRole('button')).toHaveCount(halves.length);
  for (const [name, start] of halves) {
    const b = turns.getByRole('button', { name, exact: true });
    await expect(b).toBeVisible();
    if (isTouch(testInfo)) {
      const box = await b.boundingBox();
      expect(box.width, name).toBeGreaterThanOrEqual(43.5);
      expect(box.height, name).toBeGreaterThanOrEqual(43.5);
    }
    await hit(b, testInfo);
    await expectHead(page, start, name);
    await backToMain(page, head);
  }
});
