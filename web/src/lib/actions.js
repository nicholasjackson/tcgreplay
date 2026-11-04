// Turns drags and menu choices into server verbs. The server decides; these
// helpers only pick the verb a human most likely meant.
import { app, cardInfo, ownerOf, toast } from './store.svelte.js';
import { act } from './ws.js';
import { startTarget, targetItem } from './target.js';
import AttackPanel from '../components/AttackPanel.svelte';
import CountPanel from '../components/CountPanel.svelte';

// seatArgs acts for the card's owner in review mode, so either side can be
// moved while demonstrating a line.
export function seatArgs(owner) {
  if (app.snap?.mode === 'review' && owner && owner !== app.human) return { seat: owner };
  return {};
}

// slotSpec renders a stack location: "p1:active", "p2:bench:3".
export function slotSpec(seat, slot) {
  return `${seat}:${slot}`;
}

// zoneOf strips the seat from "p1:bench:2" -> {seat, zone, slot}.
export function parseSpec(spec) {
  const [seat, zone, slot] = spec.split(':');
  return { seat, zone, slot: slot === undefined ? undefined : Number(slot) };
}

function roleFor(info) {
  if (!info) return undefined;
  if (info.category === 'Energy') return 'energy';
  if (info.category === 'Trainer') return 'tool';
  if (info.category === 'Pokemon') return 'pokemon';
  return undefined;
}

// findCard locates a card view by iid anywhere on the displayed board.
export function findCard(iid) {
  const b = app.board;
  if (!b) return null;
  for (const seat of ['p1', 'p2']) {
    const s = b.sides[seat];
    const piles = [s.hand, s.discard, s.lost || [], s.deck || []];
    for (const p of piles) for (const c of p) if (c.iid === iid) return c;
    for (const p of s.prizes) if (p.card?.iid === iid) return p.card;
    for (const st of [s.active, ...s.bench]) {
      if (!st) continue;
      for (const c of [...st.pokemon, ...(st.energy || []), ...(st.tools || [])]) if (c.iid === iid) return c;
    }
  }
  if (b.stadium?.iid === iid) return b.stadium;
  return null;
}

// dropOnStack handles a drop onto a Pokémon slot.
export async function dropOnStack(data, seat, slot, occupied) {
  const spec = slotSpec(seat, slot);
  const sa = seatArgs(seat);
  if (data.kind === 'stack') {
    const from = parseSpec(data.from);
    if (from.seat === seat && from.zone === 'bench' && slot === 'active') {
      return act('switch', { to_slot: from.slot, ...sa });
    }
    return toast('Drag a benched Pokémon onto the Active Spot to switch', 'info');
  }
  const info = cardInfo(data.def);
  const fromHand = data.from?.endsWith(':hand');
  if (fromHand && ownerOf(data.iid) === seat) {
    if (!occupied) {
      if (slot === 'active') return act('place_active', { iid: data.iid, ...sa });
      return act('bench', { iid: data.iid, slot: Number(slot.split(':')[1]), ...sa });
    }
    switch (info?.category) {
      case 'Pokemon':
        return act('evolve', { iid: data.iid, target: spec, ...sa });
      case 'Energy':
        return act('attach', { iid: data.iid, target: spec, as: 'energy', ...sa });
      case 'Trainer':
        return act('attach', { iid: data.iid, target: spec, as: 'tool', ...sa });
      default:
        return act('attach', { iid: data.iid, target: spec, ...sa });
    }
  }
  return act('move', {
    iid: data.iid,
    to: spec,
    as: occupied ? data.role || roleFor(info) : undefined,
    ...seatArgs(ownerOf(data.iid)),
  });
}

// dropOnZone handles a drop onto a pile or the play area.
export async function dropOnZone(data, zone) {
  if (data.kind === 'stack') {
    if (zone === 'discard') return knockOut(data.from);
    return toast('Use the Pokémon menu (right-click) for that', 'info');
  }
  const sa = seatArgs(ownerOf(data.iid));
  const info = cardInfo(data.def);
  // While an Item or Ability's discard cost is unpaid, a card dropped from
  // the hand pays it rather than being played (Ultra Ball's two cards).
  const cost = unpaidCost();
  if (zone === 'play' && cost && data.from?.endsWith(':hand')) {
    toast(`Discarded for ${cost.source}'s cost`, 'info');
    return act('move', { iid: data.iid, to: 'discard', ...sa });
  }
  if (zone === 'play') return act('play_trainer', { iid: data.iid, ...sa });
  if (zone === 'stadium') {
    if (data.from?.endsWith(':hand') && info?.trainer_type === 'Stadium') {
      return act('play_trainer', { iid: data.iid, ...sa });
    }
    return act('move', { iid: data.iid, to: 'stadium', ...sa });
  }
  if (zone === 'deck') return act('move', { iid: data.iid, to: 'deck:top', ...sa });
  return act('move', { iid: data.iid, to: zone, ...sa });
}

// unpaidCost is the open action's discard cost while cards are still to
// be discarded for it, else null.
export function unpaidCost() {
  const c = app.snap?.open_cost;
  return c && c.done < c.need ? c : null;
}

export function knockOut(spec) {
  return act('knock_out', { target: spec });
}

// ownStacks lists the seat's occupied Pokémon slots as target candidates:
// {spec, name, iid} of each one's top card.
export function ownStacks(seat) {
  const s = app.board?.sides[seat];
  if (!s) return [];
  const out = [];
  const add = (spec, st) => {
    const top = st.pokemon.at(-1);
    out.push({ spec, name: top?.name ?? '?', iid: top?.iid });
  };
  if (s.active) add(`${seat}:active`, s.active);
  s.bench.forEach((b, i) => b && add(`${seat}:bench:${i}`, b));
  return out;
}

// benchStacks lists the seat's benched Pokémon as candidates, as ownStacks.
function benchStacks(seat) {
  return ownStacks(seat).filter((c) => c.spec.includes(':bench:'));
}

// Menus are data: a builder returns {title, card, text, summary, sections}
// ActionSheet.svelte shows it as the phone sheet, the tablet panel or the
// desktop popover. A section holds items ({label, detail, primary, danger}
// and one of run, target or panel), a stepper ({value, step, quick, min, set}) or toggles ({label,
// on, set}); the groups follow the design's order and empty ones are left
// out.
function sections(...list) {
  return list.filter((s) => s.stepper || s.toggles?.length || s.items?.length);
}

// handMenu builds the menu for a card in hand.
export function handMenu(card) {
  const seat = ownerOf(card.iid);
  const sa = seatArgs(seat);
  const info = cardInfo(card.def);
  const side = app.board.sides[seat];
  const play = [];
  const cat = info?.category;
  if (!cat || cat === 'Pokemon') {
    if (!side.active) play.push({ label: 'Set as Active', run: () => act('place_active', { iid: card.iid, ...sa }) });
    play.push({ label: 'Bench', run: () => act('bench', { iid: card.iid, ...sa }) });
    play.push(
      targetItem({
        label: 'Evolve',
        one: (name) => `Evolve ${name}`,
        prompt: `Evolve into ${card.name}`,
        hint: 'Tap the Pokémon to evolve',
        card,
        candidates: ownStacks(seat),
        run: (target) => act('evolve', { iid: card.iid, target, ...sa }),
      }),
    );
  }
  // attach is the Attach item for a Tool or an Energy.
  const attach = (as) =>
    targetItem({
      label: 'Attach',
      one: (name) => `Attach to ${name}`,
      prompt: `Attach ${card.name}`,
      hint: 'Tap one of your Pokémon',
      card,
      candidates: ownStacks(seat),
      run: (target) => act('attach', { iid: card.iid, target, as, ...sa }),
    });
  if (!cat || cat === 'Trainer') {
    if (info?.trainer_type === 'Tool') play.push(attach('tool'));
    else play.push({ label: 'Play', run: () => act('play_trainer', { iid: card.iid, ...sa }) });
  }
  if (!cat || cat === 'Energy') play.push(attach('energy'));
  // An unpaid discard cost (Ultra Ball, Lunar Cycle) comes first.
  const cost = unpaidCost();
  if (cost) play.unshift({ label: `Discard for ${cost.source}`, detail: `cost ${cost.done + 1} of ${cost.need}`, run: () => act('move', { iid: card.iid, to: 'discard', ...sa }) });
  const playable = play.filter(Boolean);
  if (playable.length) playable[0].primary = true;
  const move = (to) => () => act('move', { iid: card.iid, to, ...sa });
  return {
    title: card.name || card.iid,
    card,
    summary: seat === app.human ? 'In your hand' : `In ${seat}'s hand`,
    sections: sections(
      { label: 'Play', items: playable },
      {
        label: 'Move',
        items: [
          { label: 'Discard', run: move('discard') },
          { label: 'Deck top', run: move('deck:top') },
          { label: 'Deck bottom', run: move('deck:bottom') },
          { label: 'Lost Zone', run: move('lost') },
        ],
      },
      { label: 'Other', items: [{ label: 'Reveal to opponent', run: () => act('reveal', { iids: [card.iid], ...sa }) }] },
    ),
  };
}

const CONDITIONS = ['asleep', 'burned', 'confused', 'paralyzed', 'poisoned'];
const capital = (w) => w[0].toUpperCase() + w.slice(1);

// stackMenu builds the menu for a Pokémon in play.
export function stackMenu(seat, slot, stack) {
  if (!stack) return null;
  const spec = slotSpec(seat, slot);
  const mine = seat === app.human || app.snap?.mode === 'review';
  const sa = seatArgs(seat);
  const top = stack.pokemon.at(-1);
  const info = cardInfo(top?.def);
  const attacks = [];
  if (mine && slot === 'active') {
    const panel = (attack) => ({ component: AttackPanel, props: { seat, attack, card: top } });
    for (const a of info?.attacks || []) attacks.push({ label: a.name, detail: a.damage || '', panel: panel(a) });
    if (!info?.attacks?.length) attacks.push({ label: 'Attack…', panel: panel(null) });
  }
  const abilities = mine
    ? (info?.abilities || []).map((ab) => ({ label: ab.name, run: () => act('use_ability', { target: spec, name: ab.name, ...sa }) }))
    : [];
  const move = [];
  if (mine && slot === 'active') {
    const bench = benchStacks(seat);
    const slotOf = (spec) => parseSpec(spec).slot;
    move.push(
      targetItem({
        label: 'Retreat',
        one: (name) => `Retreat to ${name}`,
        prompt: `Retreat ${top?.name ?? ''}`,
        hint: 'Tap the benched Pokémon to switch in',
        card: top,
        candidates: bench,
        run: (spec) => {
          const cost = info?.retreat ?? 0;
          const energy = (stack.energy || []).slice(0, cost).map((e) => e.iid);
          if (energy.length < cost) toast(`Retreat Cost is ${cost}; only ${energy.length} Energy attached`, 'warn');
          act('retreat', { to_slot: slotOf(spec), energy, ...sa });
        },
      }),
      targetItem({
        label: 'Switch, no cost',
        one: (name) => `Switch with ${name}, no cost`,
        prompt: `Switch ${top?.name ?? ''}, no cost`,
        hint: 'Tap the benched Pokémon to switch in',
        card: top,
        candidates: bench,
        run: (spec) => act('switch', { to_slot: slotOf(spec), ...sa }),
      }),
    );
  }
  if (mine && slot.startsWith('bench')) {
    const i = Number(slot.split(':')[1]);
    const empty = !app.board.sides[seat].active;
    move.push({ label: empty ? 'Promote to Active' : 'Switch into Active', run: () => act('switch', { to_slot: i, ...sa }) });
  }
  const damage = stack.damage || 0;
  const conds = stack.conditions || [];
  const other = [];
  if (mine && top) {
    other.push({ label: 'Return to hand', run: () => act('move', { iid: top.iid, to: 'hand', ...sa }) });
    other.push({ label: 'Devolve', run: () => act('move', { iid: top.iid, to: 'discard', ...sa }) });
  }
  // One step for "heal all damage" (Wally's Compassion), not one per 10.
  if (damage) other.push({ label: `Heal all (${damage} damage)`, run: () => act('place_damage', { target: spec, amount: -damage }) });
  other.push({ label: 'Knock Out', danger: true, run: () => act('knock_out', { target: spec }) });
  const where = slot === 'active' ? 'Active' : `Bench ${Number(slot.split(':')[1]) + 1}`;
  const hp = stack.hp || info?.hp;
  const energy = stack.energy?.length;
  return {
    title: top?.name ?? spec,
    card: top,
    summary: [
      seat === app.human ? where : `${seat} ${where}`,
      hp && `HP ${hp}`,
      damage ? `${damage} damage` : 'no damage',
      energy && `${energy} Energy`,
    ]
      .filter(Boolean)
      .join(' · '),
    sections: sections(
      { label: 'Attack', items: attacks },
      { label: 'Ability', items: abilities },
      { label: 'Move', items: move.filter(Boolean) },
      {
        label: 'Damage',
        stepper: {
          value: damage,
          step: 10,
          quick: [30, 50],
          min: 0,
          set: (v) => v !== damage && act('place_damage', { target: spec, amount: v - damage }),
        },
      },
      {
        label: 'Conditions',
        toggles:
          slot === 'active'
            ? CONDITIONS.map((c) => ({
                label: capital(c),
                on: conds.includes(c),
                set: (on) => act('set_condition', { target: spec, condition: c, on }),
              }))
            : [],
      },
      { label: 'Other', items: other },
    ),
  };
}

// holderOf names the Pokémon a card is attached to.
function holderOf(iid) {
  const seat = ownerOf(iid);
  const s = app.board?.sides[seat];
  const stacks = s ? [['Active', s.active], ...s.bench.map((b, i) => [`Bench ${i + 1}`, b])] : [];
  for (const [where, st] of stacks) {
    if (st && [...(st.energy || []), ...(st.tools || [])].some((c) => c.iid === iid)) return `${where} (${st.pokemon.at(-1)?.name ?? '?'})`;
  }
  return '';
}

// attachmentMenu builds the menu for an attached Energy or Tool.
export function attachmentMenu(card, role) {
  const seat = ownerOf(card.iid);
  const sa = seatArgs(seat);
  const items = [
    targetItem({
      label: 'Move to',
      one: (name) => `Move to ${name}`,
      prompt: `Move ${card.name}`,
      hint: 'Tap the Pokémon to move it to',
      card,
      candidates: ownStacks(seat),
      run: (to) => act('move', { iid: card.iid, to, as: role, ...sa }),
    }),
  ].filter(Boolean);
  items.push(
    { label: 'To hand', run: () => act('move', { iid: card.iid, to: 'hand', ...sa }) },
    { label: 'Discard', run: () => act('move', { iid: card.iid, to: 'discard', ...sa }) },
  );
  const holder = holderOf(card.iid);
  return {
    title: card.name,
    card,
    summary: `${role === 'tool' ? 'Tool' : 'Energy'}${holder ? ` on ${holder}` : ''}`,
    sections: sections({ label: 'Move', items }),
  };
}

// deckMenu builds the menu for a deck.
export function deckMenu(seat) {
  const sa = seatArgs(seat);
  const own = seat === app.human || app.snap?.mode === 'review';
  const draw = [];
  const look = [];
  if (own) {
    draw.push({ label: 'Draw 1', primary: true, run: () => act('draw', { n: 1, ...sa }) });
    draw.push({ label: 'Draw N…', panel: { component: CountPanel, props: { verb: 'draw', seat } } });
    look.push({ label: 'Look at top N…', panel: { component: CountPanel, props: { verb: 'look', seat } } });
    look.push({ label: 'Search…', run: () => openSearch(seat) });
  }
  const n = app.board?.sides[seat]?.deck_count ?? 0;
  return {
    title: seat === app.human ? 'Your deck' : `${seat} deck`,
    // The deck shows as its back, and its text says what it holds.
    card: { iid: 'deck', hidden: true },
    text: `${n} card${n === 1 ? '' : 's'} left, face down. Search lists them by category and name, never in deck order; the deck list shows every card the deck started with and where each copy is now.`,
    summary: `${n} cards`,
    sections: sections(
      { label: 'Draw', items: draw },
      { label: 'Look', items: look },
      {
        label: 'Other',
        items: [
          { label: 'Deck list…', run: () => openDeckList(seat) },
          { label: 'Shuffle', run: () => act('shuffle', { whose: seat, ...sa }) },
        ],
      },
    ),
  };
}

// openDeckList shows every card a seat owns, with where each copy is.
export async function openDeckList(seat) {
  const rows = await act('deck_list', { whose: seat, ...seatArgs(seat) });
  if (rows !== undefined) app.modal = { kind: 'decklist', seat, rows: rows === true ? [] : rows };
}
export async function openSearch(seat, filter = '') {
  const cards = await act('search_deck', { whose: seat, filter, ...seatArgs(seat) });
  if (cards !== undefined) app.modal = { kind: 'search', seat, filter, cards: cards === true ? [] : cards };
}

// pileCardActions are the buttons for a card listed in a modal.
export function pileCardActions(card, seat, from) {
  const sa = seatArgs(seat);
  const info = cardInfo(card.def);
  const out = [{ label: 'Hand', run: () => act('move', { iid: card.iid, to: 'hand', ...sa }) }];
  // Energy and Tools go straight from a pile onto a Pokémon (Aura Jab's
  // "from your discard pile"), without passing through the hand.
  if (info && (info.category === 'Energy' || info.trainer_type === 'Tool') && from !== 'deck') {
    const as = info.category === 'Energy' ? 'energy' : 'tool';
    out.push({
      label: 'Attach',
      run: () => {
        app.modal = null;
        startTarget({
          prompt: `Attach ${card.name} from the ${from === 'lost' ? 'Lost Zone' : from}`,
          hint: 'Tap one of your Pokémon',
          card,
          candidates: ownStacks(seat),
          run: (target) => act('move', { iid: card.iid, to: target, as, ...sa }),
        });
      },
    });
  }
  if (!info || info.category === 'Pokemon') out.push({ label: 'Bench', run: () => act('move', { iid: card.iid, to: 'bench:' + firstEmptyBench(seat), ...sa }) });
  if (from !== 'discard') out.push({ label: 'Discard', run: () => act('move', { iid: card.iid, to: 'discard', ...sa }) });
  if (from !== 'deck') out.push({ label: 'Deck', run: () => act('move', { iid: card.iid, to: 'deck:top', ...sa }) });
  if (from === 'deck') out.push({ label: 'Bottom', run: () => act('move', { iid: card.iid, to: 'deck:bottom', ...sa }) });
  if (from !== 'lost') out.push({ label: 'Lost', run: () => act('move', { iid: card.iid, to: 'lost', ...sa }) });
  return out;
}

// firstEmptyBench is past the shown slots when they are full; the server
// allows up to 8 and warns past the limit.
function firstEmptyBench(seat) {
  const bench = app.board.sides[seat].bench;
  const i = bench.findIndex((b) => !b);
  return i < 0 ? bench.length : i;
}

// endTurn ends the human's turn and copies the prompt for the assistant,
// unless the assistant is waiting and picks the turn up by itself.
export async function endTurn(seat = app.human) {
  const data = await act('end_turn', { ...seatArgs(seat), ...(app.prizeReminders ? {} : { ignore_prizes: true }) });
  if (data?.waiting) {
    toast('Turn ended.', 'info');
    return;
  }
  if (!data?.prompt) return;
  try {
    await navigator.clipboard.writeText(data.prompt);
    toast(`Turn ended. Prompt copied: paste it to ${app.assistant}.`, 'info');
  } catch {
    app.modal = { kind: 'prompt', text: data.prompt };
  }
}
