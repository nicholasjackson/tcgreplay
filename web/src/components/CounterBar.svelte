<script>
  // A player's piles as counters, for the phone board: the deck, discard,
  // Lost Zone and prizes as chips, and each hand's count. A chip
  // opens its pile, the deck's actions or the prizes, and the pile chips
  // take dropped cards as the piles do. The Prizes chip is tinted with its
  // player's colour, and sits at your bar's start, the opponent's end.
  import { app, openSheet } from '../lib/store.svelte.js';
  import { deckMenu, dropOnZone } from '../lib/actions.js';
  import { dropZone } from '../lib/pointer.svelte.js';

  // inset leaves room at the start for the phone's dot and ☰ when, on a
  // short screen, they sit over the opponent's counters.
  let { seat, name, inset = false } = $props();

  const side = $derived(app.board.sides[seat]);
  const mine = $derived(seat === app.human);
  const pile = (zone) => dropZone(zone, (data) => dropOnZone(data, zone));
  const open = (kind, zone) => (app.modal = { kind, seat, zone });
</script>

<!-- Each chip names its pile, then counts it; the count shows on top. -->
{#snippet prizes()}
  <button class="chip prize-chip {seat}" onclick={() => open('prizes')}><small>Prizes</small> <b>{side.prizes_left}</b></button>
{/snippet}

{#snippet hand()}
  <button class="chip" aria-label="Hand {side.hand_count}" onclick={() => open('hand')}><small>Hand</small> <b>{side.hand_count}</b></button>
{/snippet}

<div class="counters" class:inset role="group" aria-label="{seat} counters">
  <span class="name">{name}</span>
  {#if mine}{@render prizes()}{/if}
  {#if !mine}
    {@render hand()}
  {/if}
  <button class="chip" {@attach pile('deck')} onclick={(ev) => openSheet(ev, () => deckMenu(seat))}><small>Deck</small> <b>{side.deck_count}</b></button>
  <button class="chip" {@attach pile('discard')} onclick={() => open('pile', 'discard')}><small>Discard</small> <b>{side.discard.length}</b></button>
  <button class="chip" {@attach pile('lost')} onclick={() => open('pile', 'lost')}><small>Lost</small> <b>{side.lost?.length ?? 0}</b></button>
  {#if !mine}{@render prizes()}{/if}
  <!-- Your hand's count ends your bar, as the phone design has it, clear
       of the hand's cards. -->
  {#if mine}{@render hand()}{/if}
</div>

<style>
  /* One row of five chips sharing the width, 6px apart, each its count
     over its label. The player's name is for screen readers: the bar's
     place on the mat says whose it is. */
  .counters {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 0 var(--space-3);
    font-size: var(--fs-ui);
    color: var(--mat-ink);
  }
  @media (max-height: 739px) { .counters.inset { padding-left: 76px; } }
  .name { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
  .chip {
    display: inline-flex;
    flex-direction: column-reverse;
    align-items: center;
    justify-content: center;
    gap: 1px;
    flex: 1 1 0;
    min-height: var(--touch-target);
    min-width: 44px;
    padding: 0 2px;
    border-radius: var(--space-2);
    line-height: 1;
    white-space: nowrap;
    background: var(--mat-chip);
    color: var(--mat-ink);
    border: 0;
  }
  .chip:hover:not(:disabled) { border: 0; }
  .prize-chip.p1 { background: var(--prize-chip-p1); }
  .prize-chip.p2 { background: var(--prize-chip-p2); }
  .chip small { font-size: var(--fs-10); font-weight: var(--fw-ui); color: var(--mat-muted); }
  .chip b { font: var(--fw-num) var(--fs-14) var(--font-mono); }
</style>
