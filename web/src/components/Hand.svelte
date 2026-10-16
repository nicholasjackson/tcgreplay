<script>
  // A fanned hand. The human's hand is face up; the opponent's shows known
  // cards face up and the rest as backs.
  import { app } from '../lib/store.svelte.js';
  import { dropOnZone, handMenu, readDrag } from '../lib/actions.js';
  import Card from './Card.svelte';

  let { seat, small = false } = $props();

  const side = $derived(app.board.sides[seat]);
  const cards = $derived.by(() => {
    const known = side.hand || [];
    const backs = Math.max(0, side.hand_count - known.length);
    return [...known, ...Array.from({ length: backs }, (_, i) => ({ iid: `${seat}-hidden-${i}`, hidden: true }))];
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
  {#each cards as c, i (c.iid)}
    <div class="slot" style:--a="{angle(i, cards.length)}deg" style:--y="{lift(i, cards.length)}px" role="listitem">
      <Card
        card={c}
        size={small ? 'sm' : 'md'}
        lift={false}
        from="{seat}:hand"
        onmenu={c.hidden ? null : (ev) => (app.menu = { x: ev.clientX, y: ev.clientY, ...handMenu(c) })}
      />
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
  .count {
    position: absolute;
    right: 12px;
    bottom: 4px;
    font-size: 0.8em;
    color: var(--muted);
  }
</style>
