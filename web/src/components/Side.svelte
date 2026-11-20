<script>
  // One player's half of the mat. The opponent's half is mirrored: the
  // player block, bench and discard on the outside, the Active panel
  // towards the middle. On the landscape mat
  // (app.wide) a half is one row of prizes, bench and piles, and the Strip
  // holds both Active Spots.
  import { app, benchSlots, openSheet } from '../lib/store.svelte.js';
  import { deckMenu, dropOnZone } from '../lib/actions.js';
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
  <!-- The phone board: counters for the piles and prizes, the Active
       centred, and a bench row that scrolls sideways. -->
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
        <Stack {seat} slot="bench:{i}" stack={b} label="{seat} bench {i + 1}" />
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
      <div class="seat-name">
        {#if mirrored}<span class="seat-dot" aria-hidden="true"></span>{/if}<strong>{name}</strong><span class="seat-id">{` · ${seat}`}</span>{#if !mirrored}<span class="seat-dot" aria-hidden="true"></span>{/if}
        {#if status}<span class="opp-status {app.opponent}">{status}</span>{/if}
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
      <!-- Pile chips: the deck, the discard and the Lost Zone, each with its
           count; each takes a dropped card. -->
      <div class="piles">
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
        <Stack {seat} slot="bench:{i}" stack={b} label="{seat} bench {i + 1}" />
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
  .side.mat { width: 100%; padding: 0 var(--space-5); }
  .mat .field {
    display: flex;
    flex-direction: column;
    align-items: stretch;
    gap: calc(var(--cw) * 0.08);
  }
  .mat.mirrored .field { flex-direction: column-reverse; }
  .seat-row {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr);
    grid-template-areas: 'discard bench block';
    align-items: center;
    gap: var(--space-5);
  }
  .mirrored .seat-row { grid-template-areas: 'block bench discard'; }
  .mat .bench {
    grid-area: bench;
    flex-wrap: wrap;
    align-items: stretch;
    gap: var(--space-3);
    padding: var(--space-1) var(--space-3);
  }
  /* The player block: name, prize pips, pile chips; at the mat's edge. */
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
  .seat-id { font-size: max(var(--fs-ui-s), 0.85em); color: var(--mat-muted); }
  .seat-dot { width: 0.7em; height: 0.7em; border-radius: 50%; background: var(--seat-p1-mat); }
  .mirrored .seat-dot { background: var(--seat-p2-mat); }
  .opp-status { padding: 0.05em 0.6em; border-radius: var(--radius-pill); border: 1px solid var(--mat-slot-line); color: var(--mat-muted); font-size: max(var(--fs-ui-s), 0.85em); }
  .opp-status.acting, .opp-status.deciding { border-color: var(--accent); color: var(--accent); }
  .opp-status.deciding { border-style: dashed; }
  .opp-status.stopped { border-color: var(--damage-ink); color: var(--damage-ink); }
  /* Prize pips: six slots in the player's colour, filled for each prize
     still to take, with the count. Each pip is a button a little larger
     than the pip it draws. */
  .mat .prizes {
    --seat-c: var(--seat-p1-mat);
    display: flex;
    flex-direction: row-reverse;
    align-items: center;
    gap: var(--space-2);
    color: var(--mat-ink);
  }
  .mat.mirrored .prizes { --seat-c: var(--seat-p2-mat); flex-direction: row; }
  .mat .prizes .what { font-size: max(var(--fs-ui-s), 1em); color: var(--mat-muted); }
  .pips { display: flex; }
  .pip {
    display: grid;
    place-items: center;
    padding: 3px;
    border: 0;
    border-radius: var(--radius-xs);
    background: none;
  }
  .pip:hover:not(:disabled) { border: 0; box-shadow: inset 0 0 0 1px var(--mat-slot-line); }
  /* A taken pip, or the other player's in play, is drawn as it is, not
     faded as a disabled button. */
  .pip:disabled { opacity: 1; }
  .mat .prize {
    width: max(12px, 1em);
    aspect-ratio: 3 / 4;
    border-radius: 3px;
    background: var(--seat-c);
    box-shadow: inset 0 0 0 1.5px var(--seat-c);
  }
  .mat .prize.taken { background: transparent; }
  /* The count opens the prize cards. */
  .mat .prizes .count {
    padding: 0.15em 0.35em;
    border: 0;
    border-radius: var(--radius-xs);
    background: none;
    color: var(--mat-ink);
    font: var(--fw-num) max(var(--fs-num), 1.08em) var(--font-mono);
    white-space: nowrap;
  }
  .mat .prizes .count:hover:not(:disabled) { border: 0; box-shadow: inset 0 0 0 1px var(--mat-slot-line); }
  /* Pile chips: the deck, the discard and the Lost Zone, each with its
     count. */
  .mat .piles { display: flex; gap: var(--space-2); }
  .chip {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 0.4em;
    min-height: max(28px, 2.3em);
    padding: 0 0.75em;
    border-radius: var(--radius-s);
    border: 0;
    background: var(--mat-chip);
    color: var(--mat-ink);
    font: var(--fw-ui) max(var(--fs-ui-s), 1em) var(--font-sans);
    white-space: nowrap;
  }
  .chip:hover:not(:disabled) { border: 0; box-shadow: inset 0 0 0 1px var(--mat-slot-line); }
  .chip .label { font-size: inherit; color: inherit; }
  .chip b { font-family: var(--font-mono); font-weight: var(--fw-num); }
  /* The top of the discard pile, at the other end of the row: its card,
     captioned "Top of discard" over the card's name. */
  .discard-top {
    grid-area: discard;
    justify-self: start;
    min-width: 0;
    max-width: 100%;
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-1);
    border-radius: var(--radius-s);
    cursor: pointer;
  }
  .mirrored .discard-top { justify-self: end; flex-direction: row-reverse; text-align: right; }
  .slot-box {
    width: calc(var(--cw) * 0.62);
    aspect-ratio: 63 / 88;
    border-radius: var(--radius-xs);
    background: var(--mat-slot);
    box-shadow: inset 0 0 0 1px var(--mat-slot-line);
  }
  .caption { display: flex; flex-direction: column; gap: 2px; min-width: 0; font-size: max(var(--fs-ui-s), 1em); }
  .caption .what { color: var(--mat-muted); }
  .caption strong { font-weight: var(--fw-ui-title); color: var(--mat-ink); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .mat .prizes .unit { display: none; }
  /* A touch screen's narrower block, after the iPad design: a grid of the
     name, then the six pips across the block (one button, opening the
     prize cards), then the chips two to a row, "4 prizes" (tinted in the
     player's colour) filling the last cell. */
  @media (pointer: coarse) {
    .block, .mirrored .block {
      justify-self: stretch;
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: var(--space-1) calc(var(--space-1) * 1.5);
    }
    /* The name row reaches into the empty corner beside the Active panel
       (above your row, below the opponent's), so the taller block does
       not push the hand further down the screen. */
    .mat:not(.mirrored) .block { margin-top: calc(var(--touch-target) * -1); }
    .mat.mirrored .block { margin-bottom: calc(var(--touch-target) * -1); }
    .seat-name, .pips { grid-column: 1 / -1; }
    .mat .prizes, .mat .piles { display: contents; }
    .mat .prizes .what { display: none; }
    .mat .pips { justify-content: space-between; }
    .mat.mirrored .pips { justify-content: flex-start; }
    .mat:not(.mirrored) .pips { justify-content: flex-end; }
    /* The row is one button, a fingertip tall, that opens the prizes. */
    .mat .pips {
      align-items: center;
      gap: 0;
      min-height: var(--touch-target);
      padding: 0;
      border: 0;
      border-radius: var(--radius-s);
      background: none;
    }
    .mat .pips:hover:not(:disabled) { border: 0; box-shadow: inset 0 0 0 1px var(--mat-slot-line); }
    .pip { flex: 0 1 var(--touch-target); min-width: 0; padding: 0; }
    .mat .prize { width: 11px; }
    /* Each chip's count over its name, as on the phone's counters: a
       sixth of a narrow block holds them side by side no more. */
    .chip, .mat .prizes .count {
      min-width: 0;
      min-height: var(--touch-target);
      padding: 0 2px;
      border-radius: var(--radius-s);
      line-height: 1;
    }
    .chip .label, .mat .prizes .count { display: flex; flex-direction: column-reverse; align-items: center; justify-content: center; gap: 1px; }
    .chip .label { font-size: var(--fs-10); color: var(--mat-muted); }
    .chip b, .mat .prizes .count { font: var(--fw-num) var(--fs-num) var(--font-mono); color: var(--mat-ink); }
    .mat .prizes .count { order: 1; flex-direction: column; background: var(--prize-chip-p1); }
    .mat.mirrored .prizes .count { background: var(--prize-chip-p2); }
    .mat .prizes .unit { display: inline; font: var(--fw-ui) var(--fs-10) var(--font-sans); color: var(--mat-muted); }
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
    gap: calc(var(--cw) * 0.24);
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

  /* Phone: one column, the counters outermost and the Active towards the
     middle; the bench sits on its zone and scrolls sideways, a finger
     swiping it as in the hand. */
  .side.phone { display: flex; flex-direction: column-reverse; align-items: stretch; gap: var(--space-1); padding: 0; }
  .side.phone.mirrored { flex-direction: column; }
  .bench-wrap { position: relative; min-width: 0; margin: 0 var(--space-3); }
  .phone .bench {
    flex-wrap: nowrap;
    justify-content: safe center;
    align-items: stretch;
    overflow-x: auto;
    padding: 2px var(--space-2);
    gap: var(--space-2);
    scrollbar-width: none;
  }
  .phone .bench :global(.pile),
  .phone .bench :global(.card) { touch-action: pan-x; }
  .phone .active-row { margin: 0 var(--space-3); }
  .phone .active-row :global(.slot.active) { flex: 1; justify-content: center; padding: 2px var(--space-3); }
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
