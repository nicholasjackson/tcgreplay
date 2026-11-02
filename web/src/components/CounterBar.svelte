<script>
  // A player's piles as counters, for the phone board: the deck, discard,
  // Lost Zone and prizes as chips (and the opponent's hand count). A chip
  // opens its pile, the deck's actions or the prizes, and the pile chips
  // take dropped cards as the piles do.
  import { app, openSheet } from '../lib/store.svelte.js';
  import { deckMenu, dropOnZone } from '../lib/actions.js';
  import { dropZone } from '../lib/pointer.svelte.js';

  // inset leaves room at the start for the phone's ☰ and connection dot.
  let { seat, name, inset = false } = $props();

  const side = $derived(app.board.sides[seat]);
  const pile = (zone) => dropZone(zone, (data) => dropOnZone(data, zone));
  const open = (kind, zone) => (app.modal = { kind, seat, zone });
</script>

<div class="counters" class:inset role="group" aria-label="{seat} counters">
  <span class="name">{name}</span>
  {#if seat !== app.human}
    <span class="chip static" aria-label="Hand {side.hand_count}"><small>Hand</small> <b>{side.hand_count}</b></span>
  {/if}
  <button class="chip" {@attach pile('deck')} onclick={(ev) => openSheet(ev, () => deckMenu(seat))}><small>Deck</small> <b>{side.deck_count}</b></button>
  <button class="chip" {@attach pile('discard')} onclick={() => open('pile', 'discard')}><small>Discard</small> <b>{side.discard.length}</b></button>
  <button class="chip" {@attach pile('lost')} onclick={() => open('pile', 'lost')}><small>Lost</small> <b>{side.lost?.length ?? 0}</b></button>
  <button class="chip" onclick={() => open('prizes')}><small>Prizes</small> <b>{side.prizes_left}</b></button>
</div>

<style>
  /* One row: the name, then each count over its label. */
  .counters {
    display: flex;
    align-items: center;
    gap: 5px;
    padding: 2px 8px;
    font-size: 13px;
  }
  .counters.inset { padding-left: 76px; }
  .name { font-weight: 700; flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .chip {
    display: inline-flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    flex: none;
    min-height: 44px;
    min-width: 48px;
    padding: 2px 8px;
    border-radius: 12px;
    line-height: 1.1;
    white-space: nowrap;
    background: #0006;
    color: #f2f6f3;
    border: 1px solid #ffffff33;
  }
  .chip small { font-size: 10px; font-weight: 600; opacity: 0.8; }
  .chip b { font-size: 15px; }
</style>
