// Turns drags and menu choices into server verbs. The server decides; these
// helpers only pick the verb a human most likely meant.
import { app, cardInfo, ownerOf, toast } from './store.svelte.js';
import { act } from './ws.js';

const DRAG_TYPE = 'application/x-tcgreplay';

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

export function startDrag(ev, data) {
  ev.dataTransfer.setData(DRAG_TYPE, JSON.stringify(data));
  ev.dataTransfer.effectAllowed = 'move';
  app.dragging = data;
}

export function endDrag() {
  app.dragging = null;
}

export function readDrag(ev) {
  try {
    return JSON.parse(ev.dataTransfer.getData(DRAG_TYPE));
  } catch {
    return null;
  }
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

export function knockOut(spec) {
  return act('knock_out', { target: spec });
}

// ownStacks lists the seat's occupied Pokémon slots as {spec, label}.
export function ownStacks(seat) {
  const s = app.board?.sides[seat];
  if (!s) return [];
  const out = [];
  if (s.active) out.push({ spec: `${seat}:active`, label: `Active (${s.active.pokemon.at(-1)?.name ?? '?'})` });
  s.bench.forEach((b, i) => {
    if (b) out.push({ spec: `${seat}:bench:${i}`, label: `Bench ${i + 1} (${b.pokemon.at(-1)?.name ?? '?'})` });
  });
  return out;
}

function ask(question, def = '') {
  const v = window.prompt(question, def);
  return v === null ? null : v.trim();
}

// handMenu builds the menu for a card in hand.
export function handMenu(card) {
  const seat = ownerOf(card.iid);
  const sa = seatArgs(seat);
  const info = cardInfo(card.def);
  const side = app.board.sides[seat];
  const items = [];
  const cat = info?.category;
  if (!cat || cat === 'Pokemon') {
    if (!side.active) items.push({ label: 'Set as Active', run: () => act('place_active', { iid: card.iid, ...sa }) });
    items.push({ label: 'Bench', run: () => act('bench', { iid: card.iid, ...sa }) });
    for (const t of ownStacks(seat)) {
      items.push({ label: `Evolve ${t.label}`, run: () => act('evolve', { iid: card.iid, target: t.spec, ...sa }) });
    }
  }
  if (!cat || cat === 'Trainer') {
    if (info?.trainer_type === 'Tool') {
      for (const t of ownStacks(seat)) {
        items.push({ label: `Attach to ${t.label}`, run: () => act('attach', { iid: card.iid, target: t.spec, as: 'tool', ...sa }) });
      }
    } else {
      items.push({ label: 'Play', run: () => act('play_trainer', { iid: card.iid, ...sa }) });
    }
  }
  if (!cat || cat === 'Energy') {
    for (const t of ownStacks(seat)) {
      items.push({ label: `Attach to ${t.label}`, run: () => act('attach', { iid: card.iid, target: t.spec, as: 'energy', ...sa }) });
    }
  }
  items.push({ sep: true });
  items.push({ label: 'Discard', run: () => act('move', { iid: card.iid, to: 'discard', ...sa }) });
  items.push({ label: 'To deck (top)', run: () => act('move', { iid: card.iid, to: 'deck:top', ...sa }) });
  items.push({ label: 'To deck (bottom)', run: () => act('move', { iid: card.iid, to: 'deck:bottom', ...sa }) });
  items.push({ label: 'Lost Zone', run: () => act('move', { iid: card.iid, to: 'lost', ...sa }) });
  items.push({ label: 'Reveal to opponent', run: () => act('reveal', { iids: [card.iid], ...sa }) });
  items.push({ label: 'Zoom', run: () => (app.zoom = card) });
  return { title: card.name || card.iid, items };
}

const CONDITIONS = ['asleep', 'confused', 'paralyzed', 'poisoned', 'burned'];

// stackMenu builds the menu for a Pokémon in play.
export function stackMenu(seat, slot, stack) {
  const spec = slotSpec(seat, slot);
  const mine = seat === app.human || app.snap?.mode === 'review';
  const sa = seatArgs(seat);
  const top = stack.pokemon.at(-1);
  const info = cardInfo(top?.def);
  const items = [];
  if (mine && slot === 'active') {
    for (const a of info?.attacks || []) {
      items.push({
        label: `Attack: ${a.name}${a.damage ? ` (${a.damage})` : ''}`,
        run: () => {
          const dmg = ask(`Damage for ${a.name} after Weakness/Resistance`, String(parseInt(a.damage) || 0));
          if (dmg === null) return;
          const target = ask('Target (active or bench:N on the opponent)', 'active');
          if (target === null) return;
          act('attack', { name: a.name, damage: Number(dmg) || 0, target, ...sa });
        },
      });
    }
    if (!info?.attacks?.length) {
      items.push({
        label: 'Attack…',
        run: () => {
          const name = ask('Attack name');
          if (!name) return;
          const dmg = ask('Damage', '0');
          if (dmg === null) return;
          act('attack', { name, damage: Number(dmg) || 0, ...sa });
        },
      });
    }
  }
  if (mine) {
    for (const ab of info?.abilities || []) {
      items.push({ label: `Ability: ${ab.name}`, run: () => act('use_ability', { target: spec, name: ab.name, ...sa }) });
    }
  }
  if (mine && slot === 'active') {
    const side = app.board.sides[seat];
    side.bench.forEach((b, i) => {
      if (!b) return;
      const name = b.pokemon.at(-1)?.name ?? '?';
      items.push({
        label: `Retreat to ${name} (bench ${i + 1})`,
        run: () => {
          const cost = info?.retreat ?? 0;
          const energy = (stack.energy || []).slice(0, cost).map((e) => e.iid);
          if (energy.length < cost) toast(`Retreat Cost is ${cost}; only ${energy.length} Energy attached`, 'warn');
          act('retreat', { to_slot: i, energy, ...sa });
        },
      });
      items.push({ label: `Switch with ${name} (no cost)`, run: () => act('switch', { to_slot: i, ...sa }) });
    });
  }
  if (mine && slot.startsWith('bench')) {
    const i = Number(slot.split(':')[1]);
    const empty = !app.board.sides[seat].active;
    items.push({ label: empty ? 'Promote to Active' : 'Switch into Active', run: () => act('switch', { to_slot: i, ...sa }) });
  }
  if (items.length) items.push({ sep: true });
  for (const d of [10, 30, 50, -10]) {
    items.push({ label: `${d > 0 ? '+' : ''}${d} damage`, run: () => act('place_damage', { target: spec, amount: d }) });
  }
  items.push({
    label: 'Set damage…',
    run: () => {
      const v = ask('Total damage on this Pokémon', String(stack.damage || 0));
      if (v === null) return;
      act('place_damage', { target: spec, amount: (Number(v) || 0) - (stack.damage || 0) });
    },
  });
  if (slot === 'active') {
    items.push({ sep: true });
    for (const c of CONDITIONS) {
      const on = (stack.conditions || []).includes(c);
      items.push({ label: `${on ? 'Clear' : 'Set'} ${c}`, run: () => act('set_condition', { target: spec, condition: c, on: !on }) });
    }
  }
  items.push({ sep: true });
  items.push({ label: 'Knock Out', run: () => act('knock_out', { target: spec }) });
  if (mine && top) {
    items.push({ label: `Return ${top.name} to hand`, run: () => act('move', { iid: top.iid, to: 'hand', ...sa }) });
    items.push({ label: `Discard ${top.name} (devolve)`, run: () => act('move', { iid: top.iid, to: 'discard', ...sa }) });
  }
  if (top) items.push({ label: 'Zoom', run: () => (app.zoom = top) });
  return { title: top?.name ?? spec, items };
}

// attachmentMenu builds the menu for an attached Energy or Tool.
export function attachmentMenu(card, role) {
  const seat = ownerOf(card.iid);
  const sa = seatArgs(seat);
  const items = [
    { label: 'Discard', run: () => act('move', { iid: card.iid, to: 'discard', ...sa }) },
    { label: 'To hand', run: () => act('move', { iid: card.iid, to: 'hand', ...sa }) },
  ];
  for (const t of ownStacks(seat)) {
    items.push({ label: `Move to ${t.label}`, run: () => act('move', { iid: card.iid, to: t.spec, as: role, ...sa }) });
  }
  items.push({ label: 'Zoom', run: () => (app.zoom = card) });
  return { title: card.name, items };
}

// deckMenu builds the menu for a deck.
export function deckMenu(seat) {
  const sa = seatArgs(seat);
  const items = [];
  if (seat === app.human || app.snap?.mode === 'review') {
    items.push({ label: 'Draw 1', run: () => act('draw', { n: 1, ...sa }) });
    items.push({
      label: 'Draw N…',
      run: () => {
        const n = Number(ask('Cards to draw', '1'));
        if (n > 0) act('draw', { n, ...sa });
      },
    });
    items.push({
      label: 'Look at top N…',
      run: async () => {
        const n = Number(ask('How many cards?', '3'));
        if (!(n > 0)) return;
        const cards = await act('look_top', { n, whose: seat, ...sa });
        if (cards) app.modal = { kind: 'look', seat, n, cards };
      },
    });
    items.push({ label: 'Search…', run: () => openSearch(seat) });
  }
  items.push({ label: 'Deck list…', run: () => openDeckList(seat) });
  items.push({ label: 'Shuffle', run: () => act('shuffle', { whose: seat, ...sa }) });
  return { title: `${seat} deck`, items };
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
  const data = await act('end_turn', seatArgs(seat));
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
