// The Studio mat: each Pokémon's HP left, damage and Energy, and each
// player's prizes, read straight off the mat, no card or menu opened; the
// player blocks, the Active panels, the turn strip and the hand's kind
// labels laid out as the Studio design has them; and the Studio's colours
// painted on every mat (slate, felt, navy) in both interface themes. The
// pixel match is the recorded screenshots' job (screenshots.spec.js).
import { test, expect, act, drag, isTouch, isPhone, isWide } from './fixtures.js';

const region = (page, name) => page.getByRole('region', { name, exact: true });
const side = (page, seat) => region(page, `${seat} side`);
const hand = (page) => page.getByRole('list', { name: 'p1 hand' });
const counters = (page, seat) => page.getByRole('group', { name: `${seat} counters` });
const chip = (page, seat, name) => counters(page, seat).getByRole('button', { name: new RegExp(`^${name} `) });
const titled = (scope, title) => scope.locator(`.card[title="${title}"]`);
// opened finds anything a card or a menu opens: the zoom or prize view
// (a modal) and the action sheet.
const opened = (page) => page.locator('.modal, .sheet');

// hurt puts 30 damage on p1's Active Makuhita (80 HP), 20 on its benched
// Riolu (70 HP) with an Energy attached, and takes one of p1's prizes.
async function hurt(page) {
  await act(page, 'place_damage', { target: 'p1:active', amount: 30 });
  await act(page, 'place_damage', { target: 'p1:bench:0', amount: 20 });
  await act(page, 'move', { iid: 'p1-17', to: 'bench:0', as: 'energy' });
  await act(page, 'take_prize', { seat: 'p1', slot: 0 });
  await expect(region(page, 'p1 Active').locator('.hpleft')).toHaveText('50/80');
}

test('HP left, damage and Energy read off the Active panel and the Bench, no card opened', async ({ page }) => {
  await hurt(page);
  const active = region(page, 'p1 Active');
  // The Active: its name, HP left over maximum, the HP bar at 50/80, the
  // damage tag and the Energy as dots and a count.
  await expect(active.locator('.readout .name')).toHaveText('Makuhita');
  await expect(active.locator('.hpleft')).toBeVisible();
  await expect(active.locator('.bar span')).toHaveAttribute('style', /width: 63%/);
  await expect(active.locator('.readout .tag')).toHaveText('−30');
  await expect(active.locator('.readout .tag')).toBeVisible();
  await expect(active.locator('.readout .count')).toHaveText('1 Energy');
  await expect(active.locator('.dots')).toHaveAttribute('aria-label', '1 Energy');
  await expect(active.locator('.dots i')).toHaveCount(1);
  // The opponent's undamaged Active: full HP, no damage tag, no Energy.
  const theirs = region(page, 'p2 Active');
  await expect(theirs.locator('.hpleft')).toHaveText('80/80');
  await expect(theirs.locator('.readout .tag')).toHaveCount(0);
  await expect(theirs.locator('.dots')).toHaveCount(0);

  // The Bench: HP left under each card, marked when damaged, and the dots.
  const riolu = region(page, 'p1 bench 1');
  await expect(riolu.locator('.bench-readout .left')).toHaveText('50');
  await expect(riolu.locator('.bench-readout .left')).toHaveClass(/\bhurt\b/);
  await expect(riolu.locator('.bench-readout .left')).toBeVisible();
  await expect(riolu.locator('.dots i')).toHaveCount(1);
  const makuhita = region(page, 'p1 bench 2');
  await expect(makuhita.locator('.bench-readout .left')).toHaveText('80');
  await expect(makuhita.locator('.bench-readout .left')).not.toHaveClass(/\bhurt\b/);
  await expect(makuhita.locator('.dots')).toHaveCount(0);
  await expect(region(page, 'p2 bench 1').locator('.bench-readout .left')).toHaveText('70');

  await expect(opened(page)).toHaveCount(0);
});

test('prizes left read off the mat: pips and a count, the landscape fan, or the phone\'s chip', async ({ page }, testInfo) => {
  await hurt(page);
  if (isPhone(testInfo)) {
    // The Prizes chip counts them, first on your bar and last on theirs.
    await expect(chip(page, 'p1', 'Prizes')).toHaveText('Prizes 5');
    await expect(chip(page, 'p2', 'Prizes')).toHaveText('Prizes 6');
    await expect(counters(page, 'p1').getByRole('button').first()).toHaveText('Prizes 5');
    await expect(counters(page, 'p2').getByRole('button').last()).toHaveText('Prizes 6');
  } else if (isWide(testInfo)) {
    // The landscape mat fans the prize cards still to take, with a count.
    await expect(side(page, 'p1').locator('.prizes .prize')).toHaveCount(5);
    await expect(side(page, 'p1').locator('.prizes .count')).toHaveText('5');
    await expect(side(page, 'p2').locator('.prizes .prize')).toHaveCount(6);
    await expect(side(page, 'p2').locator('.prizes .count')).toHaveText('6');
  } else {
    // Today's mat: six pips, filled for each prize still to take, and the
    // count ("5 prizes" on a touch screen, which drops the "Prizes" word).
    for (const [seat, left] of [['p1', 5], ['p2', 6]]) {
      const prizes = side(page, seat).getByRole('group', { name: `Prizes: ${left} left` });
      await expect(prizes.locator('.prize').first()).toBeVisible();
      await expect(prizes.locator('.prize')).toHaveCount(6);
      await expect(prizes.locator('.prize:not(.taken)')).toHaveCount(left);
      const count = prizes.locator('.count');
      await expect(count).toBeVisible();
      if (isTouch(testInfo)) await expect(count).toHaveText(`${left} prizes`);
      else await expect(count).toHaveText(`${left}`, { useInnerText: true });
      await expect(prizes.locator('.what')).toBeVisible({ visible: !isTouch(testInfo) });
    }
  }
  await expect(opened(page)).toHaveCount(0);
});

test('the player blocks, the turn strip and the Stadium read as the Studio design lays them out', async ({ page }, testInfo) => {
  const strip = page.locator('.strip');
  if (isPhone(testInfo)) {
    // The phone's header bar: the turn tag and whose turn it is.
    await expect(page.locator('header .turn-tag')).toHaveText('T1');
    await expect(page.locator('header .whose')).toHaveText('Your turn');
    await expect(strip.locator('.turn')).toBeHidden();
    // A short phone's header has no room for them: they show in the strip,
    // over the Stadium's name, so the turn is always on screen.
    for (const size of [{ width: 390, height: 664 }, { width: 375, height: 667 }]) {
      await page.setViewportSize(size);
      const at = `${size.width}×${size.height}`;
      await expect(page.locator('header .turn-tag'), at).toBeHidden();
      await expect(strip.locator('.turn .tag'), at).toHaveText('T1');
      await expect(strip.locator('.turn .tag'), at).toBeInViewport();
      await expect(strip.locator('.turn strong'), at).toHaveText('Your turn');
      await expect(strip.locator('.turn strong'), at).toBeInViewport();
      await expect(strip.locator('.stadium-name strong'), at).toHaveText('Arena');
    }
    await page.setViewportSize({ width: 390, height: 844 });
  } else if (isWide(testInfo)) {
    // The landscape mat spells the turn out and names each player in the
    // corner and under the hand.
    await expect(strip.locator('.turn')).toHaveText('Turn 1 · Your turn');
    await expect(strip.locator('.turn .tag')).toHaveText('Turn 1');
    await expect(page.locator('.seat-name.top')).toHaveText('Bob · p2');
    await expect(page.locator('.seat-name:not(.top)')).toHaveText('Alice · p1');
  } else {
    // Today's mat: each player block names its player and seat, then the
    // Deck, Discard and Lost chips with their counts; at the row's other
    // end the top of the discard: its card, captioned "Top of discard"
    // over the card's name.
    await expect(side(page, 'p1').locator('.seat-name')).toHaveText('Alice · p1');
    await expect(side(page, 'p2').locator('.seat-name')).toHaveText('Bob · p2');
    const chips = (seat) => side(page, seat).locator('.piles .chip');
    await expect(chips('p1')).toHaveText(['Deck 40', 'Discard 1', 'Lost 0']);
    await expect(chips('p2')).toHaveText(['Deck 46', 'Discard 0', 'Lost 0']);
    for (const [seat, top] of [['p1', 'Fighting Energy'], ['p2', 'Empty']]) {
      const block = side(page, seat).locator('.discard-top');
      await expect(block.locator('.caption .what')).toHaveText('Top of discard');
      await expect(block.locator('.caption strong')).toHaveText(top);
    }
    await expect(titled(side(page, 'p1').locator('.discard-top'), 'Fighting Energy (p1-16)')).toBeVisible();
    // The strip: the turn tag, whose turn, then the turn buttons.
    await expect(strip.locator('.turn .tag')).toHaveText('T1');
    await expect(strip.locator('.turn strong')).toHaveText('Your turn');
  }
  for (const name of ['Draw', 'End turn']) await expect(strip.getByRole('button', { name, exact: true })).toBeVisible();
  await expect(strip.getByRole('button', { name: 'Flip coin' })).toBeVisible();
  // Off the landscape mat, the Stadium's thumbnail is captioned with its name.
  if (!isWide(testInfo)) {
    await expect(strip.locator('.stadium-name small')).toHaveText('Stadium');
    await expect(strip.locator('.stadium-name strong')).toHaveText('Arena');
  }
});

test('a Stadium played as a pair shows as one card, captioned "· pair" off the landscape mat', async ({ page }, testInfo) => {
  const pair = ['p1-59', 'p1-60'];
  for (const iid of pair) await act(page, 'move', { iid, to: 'hand' });
  await act(page, 'play_trainer', { iid: pair[0], pair_iid: pair[1] });
  const stadium = page.getByRole('region', { name: 'Stadium' });
  await expect(stadium.getByRole('group', { name: 'Twin Peak (pair)' })).toHaveCount(1);
  if (isWide(testInfo)) {
    await expect(page.locator('.strip .stadium-name')).toHaveCount(0);
  } else {
    await expect(page.locator('.strip .stadium-name strong')).toHaveText('Twin Peak · pair');
  }
});

test('each card in the hand is labelled with its kind, once per stack, and the opponent\'s hand is not', async ({ page }, testInfo) => {
  const kinds = await hand(page).locator('.kind').allTextContents();
  expect(kinds.sort()).toEqual(['Energy', 'Item', 'Pokémon', 'Pokémon', 'Stadium', 'Supporter', 'Tool']);
  await expect(hand(page).locator('.kind').first()).toHaveCSS('text-transform', 'uppercase');
  // The two Fighting Energy stack, the label on the top copy only.
  const energy = hand(page).locator('.slot', { has: page.locator('.card[title^="Fighting Energy ("]') });
  await expect(energy.locator('.copy')).toHaveCount(2);
  await expect(energy.locator('.kind')).toHaveCount(1);
  await expect(energy.locator('.copy').last().locator('.kind')).toHaveText('Energy');
  if (!isPhone(testInfo)) await expect(page.getByRole('list', { name: 'p2 hand' }).locator('.kind')).toHaveCount(0);
});

// The Studio's colours, written out from internal/design/tokens.json: the
// ones every mat shares, then each mat's own.
const studio = {
  seatP1: '#60A5FA',
  seatP2: '#FB923C',
  accent: '#38BDF8',
  onAccent: '#04121F',
  brandNavy: '#13285C',
  turnTagInk: '#BFE7FA',
  hp: '#4ADE80',
  damageBg: '#5B1E22',
  damageInk: '#FFB4B4',
  prizeChipP1: '#1E3A66',
  prizeChipP2: '#5A2E14',
  fighting: '#C8693F',
};
const mats = {
  slate: { zone: '#2D333D', strip: '#1E232B', chip: '#323945', track: '#1C2027', ink: '#E6E9EE' },
  felt: { zone: '#24513A', strip: '#163626', chip: '#2C5A43', track: '#10291C', ink: '#EEF3EF' },
  navy: { zone: '#1B2F5C', strip: '#0F1D3D', chip: '#22386A', track: '#0B1630', ink: '#EAF1FF' },
};

// painted reads the colours the mat's pieces are painted in, as upper-case
// hex ("transparent" for none); a box-shadow gives its colour.
function painted(page) {
  return page.evaluate(() => {
    const hex = (v) => {
      const m = /rgba?\((\d+),\s*(\d+),\s*(\d+)(?:,\s*([\d.]+))?\)/.exec(v || '');
      if (!m) return v;
      if (m[4] !== undefined && Number(m[4]) === 0) return 'transparent';
      return '#' + m.slice(1, 4).map((n) => Number(n).toString(16).padStart(2, '0')).join('').toUpperCase();
    };
    const el = (sel) => document.querySelector(sel);
    const css = (sel, prop) => {
      const e = el(sel);
      return e ? hex(getComputedStyle(e)[prop]) : null;
    };
    const p1Active = '[aria-label="p1 Active"]';
    const p2Active = '[aria-label="p2 Active"]';
    const phone = !!el('.app.phone');
    const wide = !!el('.app.landscape');
    const tag = phone ? 'header .turn-tag' : '.strip .turn .tag';
    const out = {
      p1ActiveEdge: css(p1Active, 'boxShadow'),
      p1ActivePanel: css(p1Active, 'backgroundColor'),
      p2ActiveEdge: css(p2Active, 'boxShadow'),
      hpBar: css(`${p1Active} .bar span`, 'backgroundColor'),
      hpTrack: css(`${p1Active} .bar`, 'backgroundColor'),
      damageTag: css(`${p1Active} .readout .tag`, 'backgroundColor'),
      damageTagInk: css(`${p1Active} .readout .tag`, 'color'),
      energyDot: css(`${p1Active} .dots i`, 'backgroundColor'),
      benchHurt: css('[aria-label="p1 bench 1"] .bench-readout .left', 'color'),
      benchWhole: css('[aria-label="p1 bench 2"] .bench-readout .left', 'color'),
      benchZone: css('[aria-label="p1 side"] .bench.zone', 'backgroundColor'),
      endTurn: css('.strip button.primary', 'backgroundColor'),
      endTurnInk: css('.strip button.primary', 'color'),
      turnTag: css(tag, 'backgroundColor'),
      turnTagInk: css(tag, 'color'),
    };
    if (!wide) out.strip = css('.strip', 'backgroundColor');
    if (phone) {
      out.prizeChipP1 = css('[aria-label="p1 counters"] .prize-chip', 'backgroundColor');
      out.prizeChipP2 = css('[aria-label="p2 counters"] .prize-chip', 'backgroundColor');
      out.deckChip = [...document.querySelectorAll('[aria-label="p1 counters"] .chip')]
        .filter((c) => c.textContent.startsWith('Deck'))
        .map((c) => hex(getComputedStyle(c).backgroundColor))[0];
    } else if (!wide) {
      out.p1Pip = css('[aria-label="p1 side"] .prizes .prize:not(.taken)', 'backgroundColor');
      out.p1PipTaken = css('[aria-label="p1 side"] .prizes .prize.taken', 'backgroundColor');
      out.p2Pip = css('[aria-label="p2 side"] .prizes .prize:not(.taken)', 'backgroundColor');
      out.p1Dot = css('[aria-label="p1 side"] .seat-dot', 'backgroundColor');
      out.p2Dot = css('[aria-label="p2 side"] .seat-dot', 'backgroundColor');
    }
    return out;
  });
}

// want is what painted should read on a mat, in this layout.
function want(mat, testInfo) {
  const m = mats[mat];
  const out = {
    p1ActiveEdge: studio.seatP1,
    p1ActivePanel: m.zone,
    p2ActiveEdge: studio.seatP2,
    hpBar: studio.hp,
    hpTrack: m.track,
    damageTag: studio.damageBg,
    damageTagInk: studio.damageInk,
    energyDot: studio.fighting,
    benchHurt: studio.damageInk,
    benchWhole: m.ink,
    benchZone: m.zone,
    endTurn: studio.accent,
    endTurnInk: studio.onAccent,
    turnTag: studio.brandNavy,
    turnTagInk: studio.turnTagInk,
  };
  if (!isWide(testInfo)) out.strip = m.strip;
  if (isPhone(testInfo)) Object.assign(out, { prizeChipP1: studio.prizeChipP1, prizeChipP2: studio.prizeChipP2, deckChip: m.chip });
  else if (!isWide(testInfo)) Object.assign(out, { p1Pip: studio.seatP1, p1PipTaken: 'transparent', p2Pip: studio.seatP2, p1Dot: studio.seatP1, p2Dot: studio.seatP2 });
  return out;
}

test('the mat is painted in the Studio\'s colours on every mat, in both interface themes', async ({ page }, testInfo) => {
  await hurt(page);
  for (const theme of ['light', 'dark']) {
    for (const mat of ['slate', 'felt', 'navy']) {
      await page.evaluate(({ theme, mat }) => {
        localStorage.setItem('theme', theme);
        localStorage.setItem('mat', mat);
      }, { theme, mat });
      await page.reload();
      await expect(page.locator('header .dot.on')).toBeVisible();
      await expect(page.locator('html')).toHaveAttribute('data-mat', mat);
      await expect(page.locator('html')).toHaveAttribute('data-theme', theme);
      await expect.poll(() => painted(page), { message: `${theme} interface, ${mat} mat` }).toEqual(want(mat, testInfo));
    }
  }
});

test('a click on a prize pip takes that prize, and the count opens the prize view, on today\'s mat', async ({ page }, testInfo) => {
  test.skip(isPhone(testInfo), 'the phone\'s Prizes chip (phone.spec.js)');
  test.skip(isWide(testInfo), 'the landscape mat\'s prize fan (wide.spec.js)');
  test.skip(isTouch(testInfo), 'a touch screen\'s pips open the prize view (next test)');
  const prizes = side(page, 'p1').getByRole('group', { name: /^Prizes: \d left$/ });
  // A click on a filled pip takes that prize to the hand, as a click on
  // its card did.
  await prizes.getByRole('button', { name: 'Take prize 3' }).click();
  await expect(prizes).toHaveAccessibleName('Prizes: 5 left');
  await expect(prizes.getByRole('button', { name: 'Prize 3 taken' })).toBeDisabled();
  await expect(prizes.locator('.prize.taken')).toHaveCount(1);
  await expect(page.getByRole('list', { name: 'p1 hand' }).locator('.count')).toHaveText('9 in hand');
  await expect(page.locator('.modal')).toHaveCount(0);
  // The count opens the prize view, which takes a prize too.
  await prizes.getByRole('button', { name: 'Open the prizes: 5 left' }).click();
  const modal = page.locator('.modal');
  await expect(modal.locator('h2')).toHaveText('p1 prizes · 5 left');
  await modal.getByRole('button', { name: 'Take' }).first().click();
  await expect(modal.locator('h2')).toHaveText('p1 prizes · 4 left');
  await modal.getByRole('button', { name: 'Close' }).click();
  await expect(modal).toHaveCount(0);
  await expect(prizes.locator('.prize.taken')).toHaveCount(2);
});

test('on a touch screen the pips are one fingertip-sized button that opens the prize view, where a tap takes a prize', async ({ page }, testInfo) => {
  test.skip(!isTouch(testInfo) || isPhone(testInfo) || isWide(testInfo), 'today\'s mat on a touch screen');
  const prizes = side(page, 'p1').getByRole('group', { name: /^Prizes: \d left$/ });
  const row = prizes.locator('button.pips');
  await expect(row).toHaveAccessibleName('Open the prizes: 6 left');
  const box = await row.boundingBox();
  expect(box.height).toBeGreaterThanOrEqual(43.5);
  expect(box.width).toBeGreaterThanOrEqual(43.5);
  // No single pip is a tap target of its own.
  await expect(prizes.getByRole('button', { name: /^Take prize/ })).toHaveCount(0);
  await row.tap();
  const modal = page.locator('.modal');
  await expect(modal.locator('h2')).toHaveText('p1 prizes · 6 left');
  await modal.getByRole('button', { name: 'Take' }).first().tap();
  await expect(modal.locator('h2')).toHaveText('p1 prizes · 5 left');
  await modal.getByRole('button', { name: 'Close' }).tap();
  await expect(modal).toHaveCount(0);
  await expect(prizes).toHaveAccessibleName('Prizes: 5 left');
  await expect(prizes.locator('.prize.taken')).toHaveCount(1);
  await expect(page.getByRole('list', { name: 'p1 hand' }).locator('.count')).toHaveText('9 in hand');
});

test('a hand card\'s kind label lets a tap or drag through to the card', async ({ page }, testInfo) => {
  const label = hand(page).locator('.copy', { has: page.locator('.card[title="Riolu (p1-2)"]') }).locator('.kind');
  await expect(label).toHaveText('Pokémon');
  // A finger or the mouse on the label lands on the card under it (the
  // tablet's hand starts below the fold, so it is scrolled to first).
  await label.scrollIntoViewIfNeeded();
  const under = await label.evaluate((el) => {
    const r = el.getBoundingClientRect();
    return document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2)?.closest('.card')?.getAttribute('title');
  });
  expect(under).toBe('Riolu (p1-2)');
  // Dragged from its label, the card goes to the bench.
  await drag(page, label, region(page, 'p1 bench 3'), testInfo);
  await expect(titled(region(page, 'p1 bench 3'), 'Riolu (p1-2)')).toBeVisible();
  await expect(hand(page).locator('.count')).toHaveText('7 in hand');
});
