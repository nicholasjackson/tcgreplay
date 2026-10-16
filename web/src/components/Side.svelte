<script>
  // One player's half of the mat. The opponent's half is mirrored: bench
  // on the outside, Active Spot towards the middle.
  import { app } from '../lib/store.svelte.js';
  import { deckMenu, dropOnZone, readDrag } from '../lib/actions.js';
  import { act } from '../lib/ws.js';
  import Card from './Card.svelte';
  import Stack from './Stack.svelte';

  let { seat, mirrored = false } = $props();

  const side = $derived(app.board.sides[seat]);
  const mine = $derived(seat === app.human || app.snap?.mode === 'review');
  let over = $state('');

  function dragover(zone) {
    return (ev) => {
      ev.preventDefault();
      over = zone;
    };
  }
  function drop(zone) {
    return async (ev) => {
      ev.preventDefault();
      over = '';
      const data = readDrag(ev);
      if (data) await dropOnZone(data, zone);
    };
  }
  function prize(p) {
    if (!p.card) return;
    if (!mine) return;
    const args = { slot: p.slot };
    if (seat !== app.human) args.seat = seat;
    act('take_prize', args);
  }
  function openPile(zone) {
    app.modal = { kind: 'pile', seat, zone };
  }
  function deck(ev) {
    app.menu = { x: ev.clientX, y: ev.clientY, ...deckMenu(seat) };
  }
  const topOf = (pile) => (pile && pile.length ? pile[pile.length - 1] : null);
</script>

<section class="side" class:mirrored aria-label="{seat} side">
  <div class="prizes" title="{side.prizes_left} prizes left">
    {#each side.prizes as p (p.slot)}
      <div class="prize" class:taken={!p.card}>
        {#if p.card}
          <Card card={p.card} size="sm" draggable={false} onclick={() => prize(p)} />
        {/if}
      </div>
    {/each}
    <span class="count">{side.prizes_left}</span>
  </div>

  <div class="field">
    <div class="row active-row">
      <Stack {seat} slot="active" stack={side.active} label="{seat} Active" />
    </div>
    <div class="row bench">
      {#each side.bench as b, i (i)}
        <Stack {seat} slot="bench:{i}" stack={b} label="{seat} bench {i + 1}" />
      {/each}
    </div>
  </div>

  <div class="piles">
    <div
      class="pile deck"
      class:over={over === 'deck'}
      ondragover={dragover('deck')}
      ondragleave={() => (over = '')}
      ondrop={drop('deck')}
      onclick={deck}
      oncontextmenu={(ev) => { ev.preventDefault(); deck(ev); }}
      role="button"
      tabindex="0"
      onkeydown={(ev) => ev.key === 'Enter' && deck(ev)}
    >
      {#if side.deck_count}
        <Card card={{ iid: 'deck', hidden: true }} draggable={false} onclick={deck} />
      {/if}
      <span class="label">Deck {side.deck_count}</span>
    </div>
    <div
      class="pile"
      class:over={over === 'discard'}
      ondragover={dragover('discard')}
      ondragleave={() => (over = '')}
      ondrop={drop('discard')}
      role="button"
      tabindex="0"
      onclick={() => openPile('discard')}
      onkeydown={(ev) => ev.key === 'Enter' && openPile('discard')}
    >
      {#if topOf(side.discard)}
        <Card card={topOf(side.discard)} from="{seat}:discard" onclick={() => openPile('discard')} />
      {/if}
      <span class="label">Discard {side.discard.length}</span>
    </div>
    <div
      class="pile lost"
      class:over={over === 'lost'}
      ondragover={dragover('lost')}
      ondragleave={() => (over = '')}
      ondrop={drop('lost')}
      role="button"
      tabindex="0"
      onclick={() => openPile('lost')}
      onkeydown={(ev) => ev.key === 'Enter' && openPile('lost')}
    >
      {#if topOf(side.lost)}
        <Card card={topOf(side.lost)} size="sm" from="{seat}:lost" onclick={() => openPile('lost')} />
      {/if}
      <span class="label">Lost {side.lost?.length ?? 0}</span>
    </div>
  </div>
</section>

<style>
  .side {
    display: grid;
    grid-template-columns: auto 1fr auto;
    align-items: center;
    gap: 16px;
    padding: 8px 12px;
  }
  .field {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 10px;
  }
  .mirrored .field { flex-direction: column-reverse; }
  .row { display: flex; gap: 10px; justify-content: center; }
  .bench { flex-wrap: wrap; }
  .prizes {
    display: grid;
    grid-template-columns: repeat(2, auto);
    gap: 4px;
    position: relative;
  }
  .prize {
    width: calc(var(--cw) * 0.62);
    aspect-ratio: 63 / 88;
    border-radius: 5px;
    border: 1px dashed var(--slot-line);
    cursor: pointer;
  }
  .prize.taken { opacity: 0.35; cursor: default; }
  .prizes .count {
    position: absolute;
    bottom: -18px;
    left: 0;
    right: 0;
    text-align: center;
    color: var(--muted);
    font-size: 12px;
  }
  .piles { display: flex; flex-direction: column; gap: 10px; align-items: center; }
  .mirrored .piles { flex-direction: column-reverse; }
  .pile {
    min-width: calc(var(--cw) * 0.9);
    min-height: calc(var(--cw) * 0.62 * 1.4);
    border: 2px dashed var(--slot-line);
    border-radius: 8px;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 4px;
    padding: 4px;
    cursor: pointer;
  }
  .pile.over { background: var(--drop); border-color: var(--accent); }
  .pile.deck :global(.card) { --w: calc(var(--cw) * 0.8); }
  .pile :global(.card:not(.sm)) { --w: calc(var(--cw) * 0.8); }
  .label { font-size: 11px; color: var(--muted); white-space: nowrap; }
</style>
