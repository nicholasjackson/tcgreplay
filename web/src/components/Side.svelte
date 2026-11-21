<script>
  // One player's half of the mat. The opponent's half is mirrored: the
  // player block, bench and discard on the outside, the Active panel
  // towards the middle. On the landscape mat
  // (app.wide) a half is one row of prizes, bench and piles, and the Strip
  // holds both Active Spots.
  import { app, benchSlots, openSheet } from '../lib/store.svelte.js';
  import { deckMenu, dropOnZone, tuckedOf, TUCK_CAP } from '../lib/actions.js';
  import { dropZone } from '../lib/pointer.svelte.js';
  import { act } from '../lib/ws.js';
  import Card from './Card.svelte';
  import Stack from './Stack.svelte';
  import CounterBar from './CounterBar.svelte';

  // status is the opponent's status badge ("Opponent is waiting"), if any.
  let { seat, mirrored = false, name = seat, status = '' } = $props();

  const side = $derived(app.board.sides[seat]);
  const mine = $derived(seat === app.human || app.snap?.mode === 'review');
  const slots = $derived(benchSlots(side));
  // tucks is the most tucked strips (Tools, Special Energy) any benched
  // Pokémon shows: every slot on the Bench leaves that much room above its
  // card, so the row's cards stay level. The phone draws none.
  const tucks = $derived(app.layout === 'phone' ? 0 : Math.max(0, ...slots.map((b) => Math.min(TUCK_CAP, tuckedOf(b).length))));
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
  // pips are today's mat's six prize pips: the prizes still to take first,
  // each tapped to take that prize as its card once was, then the taken.
  const pips = $derived([...side.prizes.filter((p) => p.card), ...side.prizes.filter((p) => !p.card)]);
  const openPrizes = () => (app.modal = { kind: 'prizes', seat });
  const openHand = () => (app.modal = { kind: 'hand', seat });

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

<!-- The landscape mat's Deck, Discard and Lost Zone: recessed boxes in a
     row at the end of its player row. -->
{#snippet piles()}
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
      <span class="label">Deck <b>{side.deck_count}</b></span>
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
      <span class="label">Discard <b>{side.discard.length}</b></span>
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
      <span class="label">Lost <b>{side.lost?.length ?? 0}</b></span>
    </div>
  </div>
{/snippet}

{#if app.layout === 'phone'}
  <!-- The phone board: counters for the piles and prizes, a Bench row
       that scrolls sideways, and the Active panel. -->
  <section class="side phone" class:mirrored aria-label="{seat} side">
    <CounterBar {seat} {name} inset={mirrored} />
    <div class="bench-wrap">
      <div class="row bench zone" data-scroll-x {@attach offscreen}>
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
{:else if app.wide}
  <!-- The landscape mat: one row of the prize fan, the bench and the
       piles; the opponent's row runs the other way, as across a table. The
       fan shows only the prizes left, so it closes up as they are taken. -->
  <section class="side wide" class:mirrored aria-label="{seat} side">
    <div class="prizes fan" title="{side.prizes_left} prizes left">
      {#each side.prizes.filter((p) => p.card) as p (p.slot)}
        <div class="prize">
          <Card card={p.card} size="sm" draggable={false} onclick={() => prize(p)} />
        </div>
      {/each}
      <span class="count">{side.prizes_left}</span>
    </div>
    <div class="row bench zone">
      {#each slots as b, i (i)}
        <Stack {seat} slot="bench:{i}" stack={b} label="{seat} bench {i + 1}" reserve={tucks} />
      {/each}
    </div>
    {@render piles()}
  </section>
{:else}
<!-- Today's mat: the Active panel towards the middle, then a row of the
     player block (name, prize pips, pile chips), the bench and the top of
     the discard pile; the opponent's half mirrored. -->
<section class="side mat" class:mirrored aria-label="{seat} side">
  <div class="field">
  <div class="row active-row">
    <Stack {seat} slot="active" stack={side.active} label="{seat} Active" />
  </div>

  <div class="seat-row">
    <div class="block">
      <!-- The name line: the opponent's dot, name and status ("Bob
           waiting"); yours "you", your name and dot. -->
      <div class="seat-name">
        {#if mirrored}<span class="seat-dot" aria-hidden="true"></span><strong>{name}</strong>{#if status}<span class="status {app.opponent}">{status}</span>{/if}{:else}<span class="you">you</span><strong>{name}</strong><span class="seat-dot" aria-hidden="true"></span>{/if}
      </div>
      <!-- Six pips, filled for the prizes still to take: with a mouse,
           clicking one takes that prize, as clicking its card did; the count
           opens the prize cards. On a touch screen six pips are too narrow
           to tap one reliably (and which prize matters once they are
           revealed), so the row is one fingertip-sized button that opens
           the prize cards, each a full card to tap. -->
      <div class="prizes" role="group" title="{side.prizes_left} prizes left" aria-label="Prizes: {side.prizes_left} left">
        <span class="what">Prizes</span>
        {#if app.layout === 'tablet'}
          <button class="pips" aria-label="Open the prizes: {side.prizes_left} left" onclick={openPrizes}>
            {#each pips as p (p.slot)}<span class="pip"><span class="prize" class:taken={!p.card}></span></span>{/each}
          </button>
        {:else}
          <span class="pips">
            {#each pips as p (p.slot)}<button class="pip" disabled={!p.card || !mine} aria-label={p.card ? `Take prize ${p.slot + 1}` : `Prize ${p.slot + 1} taken`} onclick={() => prize(p)}><span class="prize" class:taken={!p.card}></span></button>{/each}
          </span>
        {/if}
        <button class="count" aria-label="Open the prizes: {side.prizes_left} left" onclick={openPrizes}>{side.prizes_left}<span class="unit">{' prizes'}</span></button>
      </div>
      <!-- Pile chips: the opponent's hand (it opens the hand view), then the
           deck, the discard and the Lost Zone, each with its count; each
           pile takes a dropped card. Your hand is on the mat. -->
      <div class="piles">
        {#if mirrored}
          <button class="chip hand-chip" onclick={openHand} aria-label="Hand {side.hand_count}"><span class="label">Hand <b>{side.hand_count}</b></span></button>
        {/if}
        <button
          class="chip pile deck"
          {@attach pile('deck')}
          onclick={deck}
          oncontextmenu={(ev) => { ev.preventDefault(); deck(ev, 'mouse'); }}
        ><span class="label">Deck <b>{side.deck_count}</b></span></button>
        <button class="chip pile discard" {@attach pile('discard')} onclick={() => openPile('discard')}><span class="label">Discard <b>{side.discard.length}</b></span></button>
        <button class="chip pile lost" {@attach pile('lost')} onclick={() => openPile('lost')}><span class="label">Lost <b>{side.lost?.length ?? 0}</b></span></button>
      </div>
    </div>

    <div class="row bench zone">
      {#each slots as b, i (i)}
        <Stack {seat} slot="bench:{i}" stack={b} label="{seat} bench {i + 1}" reserve={tucks} />
      {/each}
    </div>

    <!-- The top of the discard pile: its card and name. It opens the
         pile, takes a dropped card, and its card drags off. -->
    <div
      class="discard-top"
      {@attach pile('discard')}
      role="button"
      tabindex="0"
      onclick={() => openPile('discard')}
      onkeydown={(ev) => ev.key === 'Enter' && openPile('discard')}
    >
      {#if topOf(side.discard)}
        <Card card={topOf(side.discard)} size="sm" from="{seat}:discard" onclick={() => openPile('discard')} />
      {:else}
        <span class="slot-box" aria-hidden="true"></span>
      {/if}
      <span class="caption">
        <span class="what">Top of discard</span>
        <strong>{topOf(side.discard)?.name || 'Empty'}</strong>
      </span>
    </div>
  </div>
  </div>
</section>
{/if}

<style>
  .row { display: flex; gap: calc(var(--cw) * 0.11); justify-content: center; }
  /* A zone: the panel a bench sits on. */
  .zone { background: var(--mat-zone); border-radius: var(--radius-m); }

  /* Today's mat: the Active row, then the player row of the block, the
     bench and the discard, spread to the mat's edges. */
  /* Today's mat: the half's two rows (the Active, and the player block,
     bench and discard) are rows of the mat itself, so the mat spreads all
     six of its rows evenly (App.svelte): the section and its field take no
     box of their own. Each row's order puts the opponent's half the other
     way up: their block row at the top, their Active towards the middle. */
  .side.mat, .mat .field { display: contents; }
  .mat.mirrored .seat-row { order: 1; }
  .mat.mirrored .active-row { order: 2; }
  .mat:not(.mirrored) .active-row { order: 4; }
  .mat:not(.mirrored) .seat-row { order: 5; }
  /* The bench is centred while there is room. The block's column never
     gets narrower than the block (min-content: its prize row and chips, which
     keep one line), nor the discard's narrower than its card, so when room
     is short the bench moves off centre rather than covering either (the
     card size, --cw-w in App.svelte, shrinks first). */
  .seat-row {
    --disc-min: calc(var(--cw) * 0.62 + 2 * var(--space-1));
    display: grid;
    grid-template-columns: minmax(var(--disc-min), 1fr) auto minmax(min-content, 1fr);
    grid-template-areas: 'discard bench block';
    align-items: center;
    gap: var(--space-5);
  }
  .mirrored .seat-row { grid-template-columns: minmax(min-content, 1fr) auto minmax(var(--disc-min), 1fr); grid-template-areas: 'block bench discard'; }
  @media (pointer: coarse) {
    .seat-row { --disc-min: calc(max(44px, var(--cw) * 0.62) + 2 * var(--space-1)); }
  }
  .mat .bench {
    grid-area: bench;
    flex-wrap: wrap;
    align-items: stretch;
    gap: var(--space-3);
    padding: 10px var(--space-3);
    border-radius: var(--radius-l);
  }
  /* The player block: name, prize pips, pile chips; at the mat's edge,
     the opponent's reading from the left, yours from the right. */
  .block {
    grid-area: block;
    justify-self: end;
    min-width: 0;
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: var(--space-2);
  }
  .mirrored .block { justify-self: start; align-items: flex-start; }
  .seat-name {
    display: flex;
    justify-content: flex-end;
    flex-wrap: wrap;
    align-items: center;
    gap: 0 var(--space-2);
    font-size: max(var(--fs-ui-title), 1.15em);
    color: var(--mat-ink);
  }
  .mirrored .seat-name { justify-content: flex-start; }
  .seat-name strong { font-weight: var(--fw-ui-title); }
  .seat-dot { width: 0.7em; height: 0.7em; border-radius: 50%; background: var(--seat-p1-mat); }
  .mirrored .seat-dot { background: var(--seat-p2-mat); }
  /* "you" before your name; the opponent's status after theirs. */
  .you, .status { font-size: max(var(--fs-ui-s), 0.85em); color: var(--mat-muted); }
  .status.acting, .status.deciding { color: var(--accent); }
  .status.stopped { color: var(--damage-ink); }
  /* Prize pips: six slots in the player's colour, filled for each prize
     still to take, with the count on their inner side and the label
     "Prizes" outside them. Each pip is a button a little larger than the
     pip it draws. */
  .mat .prizes {
    --seat-c: var(--seat-p1-mat);
    display: flex;
    flex-direction: row-reverse;
    align-items: center;
    gap: var(--space-1);
    color: var(--mat-ink);
  }
  .mat.mirrored .prizes { --seat-c: var(--seat-p2-mat); flex-direction: row; }
  .mat .prizes .what { min-width: 3.8em; font-size: max(var(--fs-ui-s), 0.85em); color: var(--mat-muted); }
  .mat:not(.mirrored) .prizes .what { text-align: right; }
  .pips { display: flex; }
  .pip {
    display: grid;
    place-items: center;
    padding: 0 var(--space-1);
    border: 0;
    border-radius: var(--radius-xs);
    background: none;
  }
  .pip:hover:not(:disabled) { border: 0; box-shadow: inset 0 0 0 1px var(--mat-slot-line); }
  /* A taken pip, or the other player's in play, is drawn as it is, not
     faded as a disabled button. */
  .pip:disabled { opacity: 1; }
  .mat .prize {
    width: max(12px, 0.85em);
    aspect-ratio: 3 / 4;
    border-radius: 3px;
    background: var(--seat-c);
    box-shadow: inset 0 0 0 1.5px var(--seat-c);
  }
  .mat .prize.taken { background: transparent; }
  /* The count opens the prize cards. */
  .mat .prizes .count {
    padding: 0.15em 0.3em;
    border: 0;
    border-radius: var(--radius-xs);
    background: none;
    color: var(--mat-ink);
    font: var(--fw-num) max(var(--fs-num), 1em) var(--font-mono);
    white-space: nowrap;
  }
  .mat .prizes .count:hover:not(:disabled) { border: 0; box-shadow: inset 0 0 0 1px var(--mat-slot-line); }
  .mat .prizes .unit { display: none; }
  /* Pile chips: the opponent's hand, then the deck, the discard and the
     Lost Zone, each with its count. */
  .mat .piles { display: flex; gap: 6px; }
  .chip {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-height: max(28px, 2.3em);
    padding: 0 0.75em;
    border-radius: var(--radius-s);
    border: 0;
    background: var(--mat-chip);
    color: var(--mat-ink);
    font: var(--fw-ui) max(var(--fs-ui-s), 0.85em) var(--font-sans);
    white-space: nowrap;
  }
  .chip:hover:not(:disabled) { border: 0; box-shadow: inset 0 0 0 1px var(--mat-slot-line); }
  .chip .label { display: inline-flex; align-items: center; gap: 0.35em; font-size: inherit; color: inherit; }
  .chip b { font-family: var(--font-mono); font-weight: var(--fw-num); }
  /* The top of the discard pile, at the other end of the row: its card,
     captioned "Top of discard" over the card's name. */
  /* It fills its column (a container, so its caption can give way when
     the column is down to the card: the bench never covers it). */
  .discard-top {
    grid-area: discard;
    justify-self: stretch;
    container-type: inline-size;
    min-width: 0;
    max-width: 100%;
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-1);
    border-radius: var(--radius-s);
    cursor: pointer;
  }
  .mirrored .discard-top { flex-direction: row-reverse; text-align: right; }
  .discard-top :global(.card) { box-shadow: none; }
  .slot-box {
    flex: none;
    width: calc(var(--cw) * 0.62);
    aspect-ratio: 63 / 88;
    border-radius: var(--radius-xs);
    background: var(--mat-slot);
    box-shadow: inset 0 0 0 1px var(--mat-slot-line);
  }
  .caption { display: flex; flex-direction: column; gap: 2px; min-width: 0; font-size: max(var(--fs-ui-s), 0.85em); }
  .caption .what { color: var(--mat-muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  /* Down to its card, the column drops the caption rather than spill it. */
  @container (max-width: 140px) {
    .caption { display: none; }
  }
  .caption strong { font-weight: var(--fw-ui-title); color: var(--mat-ink); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  /* The narrow board (timeline docked below) and a touch screen: the
     count spelled out beside the pips ("4 prizes"), no "Prizes" label, and
     the chips in a grid, each label at the left and its count at the
     right: the opponent's four two by two, your three in a row. */
  :global(.app.panel-bottom) .mat .prizes .what { display: none; }
  :global(.app.panel-bottom) .mat .prizes .unit { display: inline; }
  :global(.app.panel-bottom) .mat .piles { display: grid; grid-template-columns: repeat(3, auto); }
  :global(.app.panel-bottom) .mat.mirrored .piles { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  :global(.app.panel-bottom) .chip .label { flex: 1; justify-content: space-between; gap: 0.8em; }
  :global(.app.panel-bottom) .chip { min-height: max(30px, 2.5em); }
  @media (pointer: coarse) {
    .mat .prizes .what { display: none; }
    .mat .prizes .unit { display: inline; }
    .mat .piles { display: grid; grid-template-columns: repeat(3, auto); gap: var(--space-1); }
    .mat.mirrored .piles { grid-template-columns: repeat(2, minmax(0, 1fr)); }
    .chip .label { flex: 1; justify-content: space-between; gap: 0.8em; }
    .chip, .mat .prizes .count { min-height: var(--touch-target); min-width: var(--touch-target); border-radius: var(--space-2); }
    .chip { padding: 0 var(--space-3); }
    /* The name row reaches into the empty corner beside the Active panel
       (above your row, below the opponent's), so the taller block does
       not push the hand further down the screen. */
    .mat:not(.mirrored) .block { margin-top: calc(var(--touch-target) * -1); }
    .mat.mirrored .block { margin-bottom: calc(var(--touch-target) * -1); }
    /* The pips are one button, a fingertip tall, that opens the prizes. */
    .mat .pips {
      align-items: center;
      gap: 0;
      min-height: var(--touch-target);
      padding: 0;
      border: 0;
      border-radius: var(--space-2);
      background: none;
    }
    .mat .pips:hover:not(:disabled) { border: 0; box-shadow: inset 0 0 0 1px var(--mat-slot-line); }
    .pip { padding: 0 3px; }
    .mat .prize { width: 11px; }
    .slot-box { width: max(44px, calc(var(--cw) * 0.62)); }
  }

  /* The landscape mat: one row, the bench on its zone in the middle, the
     prize fan at one end and the piles at the other. Smaller piles and
     prizes leave the bench and the Active Spots the room. */
  .side.wide {
    display: grid;
    grid-template-columns: 1fr auto 1fr;
    grid-template-areas: 'prizes bench piles';
    align-items: center;
    /* The side columns keep at least mat-gap-side from the Bench
       (App.svelte's --gap-side). */
    gap: var(--gap-side, calc(var(--cw) * 0.24));
    padding: 0 12px;
    width: 100%;
  }
  .side.wide.mirrored { grid-template-areas: 'piles bench prizes'; }
  .wide .bench { grid-area: bench; flex-wrap: nowrap; align-items: stretch; padding: calc(var(--cw) * 0.03) calc(var(--cw) * 0.05); }
  .wide .bench :global(.slot) { width: calc(var(--cw) * 1.05); }
  .wide .prizes { grid-area: prizes; display: flex; align-items: center; gap: 0; justify-self: start; }
  .wide.mirrored .prizes { justify-self: end; }
  .wide .prize { cursor: pointer; }
  .wide .prize + .prize { margin-left: calc(var(--cw) * -0.42); }
  .wide .prizes .count { margin-left: calc(var(--cw) * 0.12); font: var(--fw-num) 1.1em var(--font-mono); color: var(--mat-ink); }
  .wide .piles { grid-area: piles; display: flex; gap: calc(var(--cw) * 0.11); justify-self: end; align-items: stretch; }
  .wide.mirrored .piles { flex-direction: row-reverse; justify-self: start; }
  /* A pile is a recessed box holding its top card, its count under it. */
  .wide .pile {
    min-width: calc(var(--cw) * 0.75);
    min-height: calc(var(--cw) * 0.62 * 1.4 + 8px);
    border-radius: var(--radius-s);
    background: var(--mat-slot);
    box-shadow: inset 0 0 0 1px var(--mat-slot-line);
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 4px;
    padding: 4px;
    cursor: pointer;
  }
  .wide .pile.deck :global(.card),
  .wide .pile :global(.card:not(.sm)) { --w: calc(var(--cw) * 0.7); }
  .label { font-size: 0.8em; color: var(--mat-muted); white-space: nowrap; }
  .wide .label b { font-family: var(--font-mono); font-weight: var(--fw-num); color: var(--mat-ink); }

  /* Phone (the PhoneBoard design): each half's three rows (the counters
     outermost, the Bench, the Active towards the middle) are rows of the
     board itself, so the board spreads all eight of its rows evenly
     (App.svelte), as today's mat does: the section takes no box of its
     own. Each row is 12px in from the screen's edges. The Bench sits on a
     12px-cornered zone, 8px inside and between the cards, its empty spots
     shown, and scrolls sideways, a finger swiping it as in the hand. */
  .side.phone { display: contents; }
  .phone.mirrored > :global(.counters) { order: 1; }
  .phone.mirrored > .bench-wrap { order: 2; }
  .phone.mirrored > .active-row { order: 3; }
  .phone:not(.mirrored) > .active-row { order: 5; }
  .phone:not(.mirrored) > .bench-wrap { order: 6; }
  .phone:not(.mirrored) > :global(.counters) { order: 7; }
  .bench-wrap { position: relative; min-width: 0; margin: 0 var(--space-3); }
  .phone .bench {
    flex-wrap: nowrap;
    justify-content: safe center;
    align-items: flex-start;
    overflow-x: auto;
    padding: var(--space-2);
    gap: var(--space-2);
    border-radius: 12px;
    scrollbar-width: none;
  }
  .phone .bench :global(.pile),
  .phone .bench :global(.card) { touch-action: pan-x; }
  .phone .active-row { margin: 0 var(--space-3); }
  .more {
    position: absolute;
    right: 0;
    top: 50%;
    translate: 0 -50%;
    padding: 4px 8px;
    border-radius: var(--radius-m) 0 0 var(--radius-m);
    background: var(--mat-strip);
    color: var(--mat-ink);
    font: var(--fw-num) var(--fs-num-s) var(--font-mono);
    pointer-events: none;
  }
  @media (pointer: coarse) {
    .wide .pile { min-width: 44px; min-height: 44px; }
  }
</style>
