<script>
  // A fanned hand. The human's hand is face up; the opponent's shows known
  // cards face up and the rest as backs. Face-up cards are sorted Pokémon,
  // Trainers, Energy and by name, with copies of a card stacked together.
  import { app, cardInfo } from '../lib/store.svelte.js';
  import { dropOnZone, handMenu, readDrag } from '../lib/actions.js';
  import Card from './Card.svelte';

  let { seat, small = false } = $props();

  const side = $derived(app.board.sides[seat]);
  const order = { Pokemon: 0, Trainer: 1, Energy: 2 };
  // groups is the hand as fan slots: one per distinct face-up card (its
  // copies stacked), then one per face-down card.
  const groups = $derived.by(() => {
    const known = [...(side.hand || [])].sort((a, b) => {
      const ca = order[cardInfo(a.def)?.category] ?? 3;
      const cb = order[cardInfo(b.def)?.category] ?? 3;
      return ca - cb || (a.name || '').localeCompare(b.name || '') || a.iid.localeCompare(b.iid);
    });
    const out = [];
    for (const c of known) {
      const g = out.at(-1);
      if (g && c.name && g.name === c.name) g.cards.push(c);
      else out.push({ key: c.iid, name: c.name, cards: [c] });
    }
    const backs = Math.max(0, side.hand_count - known.length);
    for (let i = 0; i < backs; i++) {
      const iid = `${seat}-hidden-${i}`;
      out.push({ key: iid, cards: [{ iid, hidden: true }] });
    }
    return out;
  });
  let over = $state(false);

  // The human's hand fans up from the bottom edge; the opponent's, at the
  // top, hangs down from it, so its cards also enlarge downward.
  function angle(i, n) {
    const spread = Math.min(5, 50 / Math.max(n, 1));
    return (i - (n - 1) / 2) * spread * (small ? -1 : 1);
  }
  function lift(i, n) {
    const c = i - (n - 1) / 2;
    return c * c * (small ? -0.6 : 1.4);
  }
  async function drop(ev) {
    ev.preventDefault();
    over = false;
    const data = readDrag(ev);
    if (data) await dropOnZone(data, 'hand');
  }
</script>

<div
  class="hand"
  class:small
  class:over
  ondragover={(ev) => { ev.preventDefault(); over = true; }}
  ondragleave={() => (over = false)}
  ondrop={drop}
  role="list"
  aria-label="{seat} hand"
>
  {#each groups as g, i (g.key)}
    <div class="slot" style:--a="{angle(i, groups.length)}deg" style:--y="{lift(i, groups.length)}px" role="listitem">
      {#each g.cards as c (c.iid)}
        <div class="copy">
          <Card
            card={c}
            size={small ? 'sm' : 'md'}
            lift={false}
            from="{seat}:hand"
            onmenu={c.hidden ? null : (ev) => (app.menu = { x: ev.clientX, y: ev.clientY, ...handMenu(c) })}
          />
        </div>
      {/each}
      {#if g.cards.length > 1}<span class="dup">×{g.cards.length}</span>{/if}
    </div>
  {/each}
  <span class="count">{side.hand_count} in hand</span>
</div>

<style>
  .hand {
    display: flex;
    justify-content: center;
    align-items: flex-end;
    padding: calc(var(--cw) * 0.2) 40px calc(var(--cw) * 0.08);
    min-height: calc(var(--cw) * 1.4 + 24px);
    position: relative;
    border-radius: 10px;
    transition: background 0.15s;
  }
  .hand.small { min-height: calc(var(--cw) * 0.62 * 1.4 + 10px); padding: 4px 20px; align-items: flex-start; }
  .hand.over { background: var(--drop); }
  .slot {
    position: relative;
    display: flex;
    margin: 0 calc(var(--cw) * -0.16);
    transform: translateY(var(--y)) rotate(var(--a));
    transform-origin: 50% 120%;
    transition: transform 0.2s ease;
  }
  .small .slot { margin: 0 calc(var(--cw) * -0.2); transform-origin: 50% -20%; }
  .hand.small .slot:hover { transform: rotate(0deg) scale(2); z-index: 5; transition-delay: 0.1s; }
  .hand:not(.small) .slot:hover {
    transform: translateY(calc(var(--cw) * -0.3)) rotate(0deg) scale(1.6);
    transition-delay: 0.1s;
    z-index: 5;
  }
  /* Copies of a card stack with a sliver of each showing. */
  .copy + .copy { margin-left: calc(var(--cw) * -0.82); }
  .small .copy + .copy { margin-left: calc(var(--cw) * -0.5); }
  .dup {
    position: absolute;
    top: -0.6em;
    right: -0.4em;
    z-index: 3;
    background: var(--accent);
    color: #111;
    border-radius: 1em;
    padding: 0.1em 0.5em;
    font: 700 0.85em system-ui;
    box-shadow: 0 1px 3px #0008;
  }
  .count {
    position: absolute;
    right: 12px;
    bottom: 4px;
    font-size: 0.8em;
    color: var(--muted);
  }
</style>
