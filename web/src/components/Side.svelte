<script>
  // One player's half of the mat. The opponent's half is mirrored: bench
  // on the outside, Active Spot towards the middle.
  import { app, cardInfo, openSheet } from '../lib/store.svelte.js';
  import { deckMenu, dropOnZone } from '../lib/actions.js';
  import { dropZone } from '../lib/pointer.svelte.js';
  import { act } from '../lib/ws.js';
  import Card from './Card.svelte';
  import Stack from './Stack.svelte';
  import CounterBar from './CounterBar.svelte';

  let { seat, mirrored = false, name = seat } = $props();

  const side = $derived(app.board.sides[seat]);
  const mine = $derived(seat === app.human || app.snap?.mode === 'review');
  // A Stadium such as Area Zero Underdepths lets a bench hold 8: once the
  // shown slots are full, offer the next one to drop on.
  const expands = $derived(/\b8 Pokémon on (their|his or her) Bench/.test(cardInfo(app.board.stadium?.def)?.effect ?? ''));
  const slots = $derived(expands && side.bench.length < 8 && side.bench.every(Boolean) ? [...side.bench, null] : side.bench);
  // pile makes a deck, discard or Lost Zone pile a drop zone.
  const pile = (zone) => dropZone(zone, (data) => dropOnZone(data, zone));
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
  function deck(ev, pointerType) {
    openSheet(ev, () => deckMenu(seat), pointerType);
  }
  const topOf = (pile) => (pile && pile.length ? pile[pile.length - 1] : null);

  // offscreen counts the bench slots past the right edge of the phone's
  // scrolling bench row, for its "+n ›" hint.
  let hidden = $state(0);
  function offscreen(row) {
    const update = () => {
      const edge = row.getBoundingClientRect().right;
      hidden = [...row.children].filter((c) => c.getBoundingClientRect().left >= edge - 4).length;
    };
    const ro = new ResizeObserver(update);
    ro.observe(row);
    // A slot added or taken away changes the count without resizing the row.
    const mo = new MutationObserver(update);
    mo.observe(row, { childList: true });
    row.addEventListener('scroll', update, { passive: true });
    update();
    return () => {
      ro.disconnect();
      mo.disconnect();
      row.removeEventListener('scroll', update);
    };
  }
</script>

{#if app.layout === 'phone'}
  <!-- The phone board: counters for the piles and prizes, the Active
       centred, and a bench row that scrolls sideways. -->
  <section class="side phone" class:mirrored aria-label="{seat} side">
    <CounterBar {seat} {name} />
    <div class="bench-wrap">
      <div class="row bench" data-scroll-x {@attach offscreen}>
        {#each slots as b, i (i)}
          <Stack {seat} slot="bench:{i}" stack={b} label="{seat} bench {i + 1}" />
        {/each}
      </div>
      {#if hidden}<span class="more" aria-hidden="true">+{hidden} ›</span>{/if}
    </div>
    <div class="row active-row">
      <Stack {seat} slot="active" stack={side.active} label="{seat} Active" />
    </div>
  </section>
{:else}
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
      {#each slots as b, i (i)}
        <Stack {seat} slot="bench:{i}" stack={b} label="{seat} bench {i + 1}" />
      {/each}
    </div>
  </div>

  <div class="piles">
    <div
      class="pile deck"
      {@attach pile('deck')}
      onclick={deck}
      oncontextmenu={(ev) => { ev.preventDefault(); deck(ev, 'mouse'); }}
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
      {@attach pile('discard')}
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
      {@attach pile('lost')}
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
{/if}

<style>
  .side {
    display: grid;
    grid-template-columns: auto 1fr auto;
    align-items: center;
    gap: calc(var(--cw) * 0.18);
    padding: calc(var(--cw) * 0.08) 12px;
    /* Keep prizes and piles beside the field, not at the screen edges. */
    width: 100%;
    max-width: calc(var(--cw) * 9.6 + 80px);
    margin-inline: auto;
  }
  .field {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: calc(var(--cw) * 0.11);
  }
  .mirrored .field { flex-direction: column-reverse; }
  .row { display: flex; gap: calc(var(--cw) * 0.11); justify-content: center; }
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
    bottom: -1.5em;
    left: 0;
    right: 0;
    text-align: center;
    color: var(--muted);
    font-size: 0.85em;
  }
  .piles { display: flex; flex-direction: column; gap: calc(var(--cw) * 0.11); align-items: center; }
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
  .pile.deck :global(.card) { --w: calc(var(--cw) * 0.8); }
  .pile :global(.card:not(.sm)) { --w: calc(var(--cw) * 0.8); }
  .label { font-size: 0.8em; color: var(--muted); white-space: nowrap; }
  /* Phone: one column, the counters outermost and the Active towards the
     middle; the bench scrolls sideways, a finger swiping it as in the hand. */
  .side.phone { display: flex; flex-direction: column-reverse; align-items: stretch; gap: 4px; padding: 4px 0; max-width: none; }
  .side.phone.mirrored { flex-direction: column; }
  .bench-wrap { position: relative; min-width: 0; }
  .phone .bench {
    flex-wrap: nowrap;
    justify-content: safe center;
    overflow-x: auto;
    padding: 4px 16px;
    gap: 6px;
    scrollbar-width: none;
  }
  .phone .bench :global(.pile),
  .phone .bench :global(.card) { touch-action: pan-x; }
  .more {
    position: absolute;
    right: 0;
    top: 50%;
    translate: 0 -50%;
    padding: 4px 8px;
    border-radius: 12px 0 0 12px;
    background: #000a;
    color: #fff;
    font: 700 12px system-ui;
    pointer-events: none;
  }
  @media (pointer: coarse) {
    .pile { min-width: 44px; min-height: 44px; }
    .pile.deck :global(.card),
    .pile :global(.card:not(.sm)) { --w: max(44px, calc(var(--cw) * 0.8)); }
  }
</style>
