<script>
  // A player's piles as counters, for the phone board: the deck, discard,
  // Lost Zone and prizes as chips (and the opponent's hand count). A chip
  // opens its pile, the deck's actions or the prizes, and the pile chips
  // take dropped cards as the piles do.
  import { app, openSheet } from '../lib/store.svelte.js';
  import { deckMenu, dropOnZone } from '../lib/actions.js';
  import { dropZone } from '../lib/pointer.svelte.js';

  let { seat, name } = $props();

  const side = $derived(app.board.sides[seat]);
  const pile = (zone) => dropZone(zone, (data) => dropOnZone(data, zone));
  const open = (kind, zone) => (app.modal = { kind, seat, zone });
</script>

<div class="counters" role="group" aria-label="{seat} counters">
  <span class="name">{name}</span>
  {#if seat !== app.human}
    <span class="chip static" aria-label="Hand {side.hand_count}">✋ {side.hand_count}</span>
  {/if}
  <button class="chip" {@attach pile('deck')} onclick={(ev) => openSheet(ev, () => deckMenu(seat))}>Deck {side.deck_count}</button>
  <button class="chip" {@attach pile('discard')} onclick={() => open('pile', 'discard')}>Discard {side.discard.length}</button>
  <button class="chip" {@attach pile('lost')} onclick={() => open('pile', 'lost')}>Lost {side.lost?.length ?? 0}</button>
  <button class="chip" onclick={() => open('prizes')}>Prizes {side.prizes_left}</button>
</div>

<style>
  .counters {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 4px 8px;
    flex-wrap: wrap;
    font-size: 13px;
  }
  .name { font-weight: 700; margin-right: auto; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .chip {
    min-height: 44px;
    padding: 0 10px;
    border-radius: 22px;
    font-size: 13px;
    font-weight: 600;
    white-space: nowrap;
    background: #0006;
    color: #f2f6f3;
    border: 1px solid #ffffff33;
  }
  .static { display: inline-flex; align-items: center; min-height: 44px; }
</style>
