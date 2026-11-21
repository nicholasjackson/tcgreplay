<script>
  // Your hand: one flat row of upright cards, all the same size, centred on
  // the mat, with "7 in hand" just past the last card (it never pulls the
  // cards off centre). The art shows what each card is, so its kind is in
  // its accessible name only ("Boss's Orders, Supporter"). Face-up cards
  // are sorted Pokémon, Trainers, Energy and by name, with copies of a card
  // stacked together. On a phone the row scrolls sideways. On the landscape
  // mat both hands are strips along the mat's edges, taking no row of
  // their own (edge): yours along the bottom, cropped to each card's top
  // strip, a card lifting whole above it on roll-over or focus; the
  // opponent's (small) along the top, turned 180° to face their owner
  // across the table. A hand too long for the mat scrolls sideways, each
  // edge with cards past it fading with a count. Today's mat and the phone
  // count the opponent's hand in a chip instead (Side.svelte,
  // CounterBar.svelte).
  import { app, cardInfo, openSheet } from '../lib/store.svelte.js';
  import { dropOnZone, handMenu } from '../lib/actions.js';
  import { dropZone } from '../lib/pointer.svelte.js';
  import Card from './Card.svelte';

  let { seat, small = false } = $props();

  const side = $derived(app.board.sides[seat]);
  const order = { Pokemon: 0, Trainer: 1, Energy: 2 };
  // groupKey is what stacks cards in one slot: the name, so alternate
  // prints of a card share a slot, except for a card played as a pair,
  // whose two prints are different halves the player must choose between.
  const groupKey = (c) => (cardInfo(c.def)?.played_as_pair ? c.def : c.name);
  // groups is the hand as slots: one per distinct face-up card (its copies
  // stacked), then one per face-down card.
  const groups = $derived.by(() => {
    const known = [...(side.hand || [])].sort((a, b) => {
      const ca = order[cardInfo(a.def)?.category] ?? 3;
      const cb = order[cardInfo(b.def)?.category] ?? 3;
      return ca - cb || (a.name || '').localeCompare(b.name || '') ||
        (a.def || '').localeCompare(b.def || '', undefined, { numeric: true }) || a.iid.localeCompare(b.iid);
    });
    const out = [];
    for (const c of known) {
      const g = out.at(-1);
      const k = c.name && groupKey(c);
      if (g && k && g.group === k) g.cards.push(c);
      else out.push({ key: c.iid, name: c.name, group: k, cards: [c] });
    }
    const backs = Math.max(0, side.hand_count - known.length);
    for (let i = 0; i < backs; i++) {
      const iid = `${seat}-hidden-${i}`;
      out.push({ key: iid, cards: [{ iid, hidden: true }] });
    }
    return out;
  });
  // extra counts the stacked copies past each slot's first card: each shows
  // as a sliver beside its slot's top card.
  const extra = $derived(groups.reduce((n, g) => n + g.cards.length - 1, 0));

  // kind is a card's kind, labelled on it in the hand: Pokémon, Item,
  // Supporter, Tool, Stadium or Energy.
  function kind(c) {
    const info = cardInfo(c.def);
    if (!info) return '';
    if (info.category === 'Pokemon') return 'Pokémon';
    if (info.category === 'Trainer') return info.trainer_type || 'Trainer';
    return info.category || '';
  }
  const phone = $derived(app.layout === 'phone');
  const peek = $derived(app.wide && small);
  // edge: the landscape mat's strips, yours (bottom) or the opponent's.
  const edge = $derived(app.wide);
  // past is how many slots are scrolled off each end of an edge strip.
  let past = $state({ left: 0, right: 0 });
  // hidden counts the cards past the right edge of the phone's sideways
  // scrolling hand, for its "+n" hint.
  let hidden = $state(0);
  // avail is the room the mat's row has, so a long hand closes up its gaps
  // (the cards overlapping) rather than running off the mat.
  let avail = $state(0);
  function measure(row) {
    const update = () => {
      avail = row.clientWidth;
      const r = row.getBoundingClientRect();
      const slots = [...row.querySelectorAll('.slot')].map((c) => c.getBoundingClientRect());
      if (edge) past = { left: slots.filter((c) => c.right <= r.left + 8).length, right: slots.filter((c) => c.left >= r.right - 8).length };
      if (!phone) return;
      hidden = slots.filter((c) => c.left >= r.right - 8).length;
    };
    // On an edge strip a wheel turned up or down scrolls it sideways (a
    // trackpad's sideways swipe and shift-wheel already do).
    const wheel = (ev) => {
      if (!edge || row.scrollWidth <= row.clientWidth || Math.abs(ev.deltaY) <= Math.abs(ev.deltaX)) return;
      row.scrollLeft += ev.deltaY;
      ev.preventDefault();
    };
    row.addEventListener('wheel', wheel, { passive: false });
    const ro = new ResizeObserver(update);
    ro.observe(row);
    const mo = new MutationObserver(update);
    mo.observe(row, { childList: true, subtree: true });
    row.addEventListener('scroll', update, { passive: true });
    update();
    return () => {
      ro.disconnect();
      mo.disconnect();
      row.removeEventListener('scroll', update);
      row.removeEventListener('wheel', wheel);
      hidden = 0;
      past = { left: 0, right: 0 };
    };
  }

  // label is a face-up card's accessible name: its name and kind.
  const label = (c) => (c.hidden || !c.name ? '' : [c.name, kind(c)].filter(Boolean).join(', '));
</script>

{#snippet row()}
  <div
    class="hand"
    class:small
    class:peek
    class:phone
    class:edge
    data-scroll-x
    style:--n={groups.length}
    style:--extra={extra}
    style:--avail="{avail}px"
    {@attach dropZone('hand', (data) => dropOnZone(data, 'hand'))}
    {@attach measure}
    role="list"
    aria-label="{seat} hand"
  >
    <div class="cards">
      {#each groups as g, i (g.key)}
        <div class="slot" role="listitem">
          {#each g.cards as c (c.iid)}
            <div class="copy">
              <Card
                card={c}
                size="md"
                lift={false}
                label={label(c)}
                from="{seat}:hand"
                onmenu={c.hidden ? null : (ev, type) => openSheet(ev, () => handMenu(c), type)}
              />
            </div>
          {/each}
          {#if g.cards.length > 1}<span class="dup">×{g.cards.length}</span>{/if}
        </div>
      {/each}
      <span class="count">{side.hand_count} in hand</span>
    </div>
    {#if phone && hidden}<span class="more" aria-hidden="true">+{hidden}</span>{/if}
  </div>
{/snippet}

{#if edge}
  <!-- The landscape mat's strip: the row (a sideways scroller as tall as
       a lifted card, letting the pointer through but on its cards), the
       fades over its ends, and the count beside it. -->
  <div class="strip" class:top={small} class:bottom={!small}>
    {#if !small}<div class="band" aria-hidden="true" {@attach dropZone('hand', (data) => dropOnZone(data, 'hand'))}></div>{/if}
    {@render row()}
    {#if past.left}<span class="fade left" aria-hidden="true">‹ +{past.left}</span>{/if}
    {#if past.right}<span class="fade right" aria-hidden="true">+{past.right} ›</span>{/if}
    <span class="tally" aria-hidden="true">{side.hand_count} in hand</span>
  </div>
{:else}
  {@render row()}
{/if}

<style>
  /* --hw is a hand card's width: one mat card width (about 72px on the
     desktop mat, 102px on the narrow board, 70px on a tablet), 70px on a
     phone. --gap is the space between slots: the design's 8px, closing up
     (the cards overlapping) when a long hand would run past the mat. */
  .hand {
    --hw: var(--cw);
    --row: calc(var(--n) * var(--hw) + var(--extra) * var(--hw) * 0.18);
    --gap: max(calc(var(--hw) * -0.7), min(var(--space-2), (var(--avail) - var(--space-12) * 2 - 96px - var(--row)) / max(1, var(--n) - 1)));
    display: flex;
    justify-content: center;
    align-items: flex-end;
    padding: var(--space-2) var(--space-12) 0;
    position: relative;
    border-radius: var(--radius-m);
    transition: background 0.15s;
  }
  @media (pointer: coarse) { .hand { --hw: max(44px, var(--cw)); } }
  .hand .cards :global(.card) { --w: var(--hw); }
  /* The cards, centred on their own; the count hangs off their right end. */
  .cards { position: relative; display: flex; align-items: flex-end; gap: 0 var(--gap); }
  /* A finger swiping sideways scrolls the hand; one moving up lifts a
     card (see pointer.svelte.js). */
  .hand,
  .hand :global(.card) { touch-action: pan-x; }
  .slot {
    position: relative;
    display: flex;
    transition: transform 0.2s ease;
  }
  @media (hover: hover) {
    :global(body:not(.dragging)) .hand:not(.small, .edge) .slot:hover {
      transform: translateY(calc(var(--hw) * -0.3)) scale(1.5);
      transform-origin: 50% 100%;
      transition-delay: 0.1s;
      z-index: 5;
    }
  }
  /* Copies of a card stack with a sliver of each showing. */
  .copy { position: relative; }
  .copy + .copy { margin-left: calc(var(--hw) * -0.82); }
  .dup {
    position: absolute;
    top: -0.6em;
    right: -0.4em;
    z-index: 3;
    background: var(--accent);
    color: var(--on-accent);
    border-radius: var(--radius-pill);
    padding: 0.1em 0.5em;
    font: var(--fw-num) max(var(--fs-num-s), 0.85em) var(--font-mono);
    box-shadow: var(--shadow-badge);
  }
  /* "7 in hand": just past the last card, level with the cards' foot. */
  .count {
    position: absolute;
    left: calc(100% + var(--space-4));
    bottom: var(--space-1);
    white-space: nowrap;
    font: var(--fw-ui) max(var(--fs-ui-s), 0.85em) var(--font-mono);
    color: var(--mat-muted);
    pointer-events: none;
  }

  /* Phone: a flat strip of 70px cards along the bottom that scrolls
     sideways, under your counters; it stays on screen, the board fitting
     above it. On a short screen, or under banners, the cards shrink with
     the board's (1.25 --cw, as your Active card). */
  .hand.phone {
    --hw: min(70px, calc(var(--cw) * 1.25));
    --gap: var(--space-2);
    order: 8;
    flex: none;
    justify-content: safe center;
    align-items: center;
    height: calc(var(--hw) * 88 / 63);
    padding: 0 var(--space-3);
    overflow-x: auto;
    overflow-y: hidden;
    scrollbar-width: none;
    position: sticky;
    bottom: 0;
    z-index: 4;
    background: var(--mat-surface);
  }
  .hand.phone .cards :global(.card) { box-shadow: none; }
  /* The row is exactly a card tall, so a stack's count sits inside it, as
     on the opponent's cropped row (below). */
  .hand.phone .dup { top: 2px; right: 2px; }
  /* The phone counts the hand in your counters (CounterBar), so the count
     here is for screen readers only, in the strip's corner. */
  .hand.phone .cards { position: static; }
  .hand.phone .count { left: auto; right: var(--space-3); width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); }
  /* The phone hand's hint at the cards scrolled off to the right. */
  .more {
    position: sticky;
    right: calc(var(--space-3) * -1);
    align-self: stretch;
    flex: none;
    display: flex;
    align-items: center;
    justify-content: flex-end;
    width: 56px;
    margin-left: -56px;
    padding-right: var(--space-2);
    background: linear-gradient(90deg, transparent, var(--mat-surface) 70%);
    color: var(--mat-muted);
    font: var(--fw-num) var(--fs-num-s) var(--font-mono);
    pointer-events: none;
    z-index: 2;
  }

  /* The landscape mat: both hands are strips along the mat's edges, in
     its top and bottom margins (App.svelte reserves them), centred, with
     the count beside the row. Over a hand too long for the mat the row
     scrolls sideways rather than squeezing its cards: each end with cards
     past it fades into the mat, with how many ("‹ +3", "+9 ›"). */
  .strip {
    --strip-h: max(var(--touch-target), calc(var(--cw) * var(--mat-hand-strip)));
    --card-h: calc(var(--hw) * 88 / 63);
    --lift-gap: calc(var(--hw) * 0.08);
    --hw: var(--cw);
    position: absolute;
    left: 0;
    right: 0;
    z-index: 6;
    display: grid;
    grid-template-columns: minmax(var(--space-12), 1fr) minmax(0, max-content) minmax(var(--space-12), 1fr);
    pointer-events: none;
  }
  .strip.bottom { bottom: 0; align-items: end; }
  .strip.top { top: 0; height: var(--peek-h); align-items: start; }
  .strip > * { grid-row: 1; }
  /* The row: a sideways scroller that clips only what lies outside it.
     Yours is as tall as a lifted card, its cards standing at the foot so
     only their top strip shows above the mat's edge; the space above them
     lets the pointer through to the board. */
  .hand.edge {
    grid-column: 2;
    --gap: calc(var(--hw) * 0.1);
    min-width: 0;
    padding: 0;
    overflow-x: auto;
    overflow-y: hidden;
    scrollbar-width: none;
    overscroll-behavior-x: contain;
    pointer-events: none;
    border-radius: 0;
    justify-content: flex-start;
  }
  .hand.edge .slot { pointer-events: auto; }
  .strip.bottom .hand.edge { height: calc(var(--card-h) + var(--lift-gap) * 2); align-items: flex-start; }
  .strip.bottom .cards { margin-top: calc(var(--card-h) + var(--lift-gap) * 2 - var(--strip-h)); align-items: flex-start; }
  /* Rolled over or focused, a card of yours lifts whole above the strip,
     over the Bench if need be, and drops back on leaving; nothing lifts
     during a drag. */
  .strip.bottom .slot:focus-within,
  :global(body:not(.dragging)) .strip.bottom .slot:hover {
    transform: translateY(calc((var(--card-h) - var(--strip-h) + var(--lift-gap)) * -1));
    transition-delay: 0s;
    z-index: 5;
  }
  .strip.bottom .slot:hover :global(.card),
  .strip.bottom .slot:focus-within :global(.card) { box-shadow: var(--shadow-card-lift); }
  .strip .dup { top: 2px; right: 2px; }
  /* The counts in the list are for screen readers; the tally beside the
     strip shows it. */
  .hand.edge .count { left: auto; right: 0; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); }
  .tally {
    grid-column: 3;
    justify-self: start;
    display: flex;
    align-items: center;
    padding-left: var(--space-4);
    white-space: nowrap;
    font: var(--fw-ui) max(var(--fs-ui-s), 0.85em) var(--font-mono);
    color: var(--mat-muted);
  }
  .strip.bottom .tally { height: var(--strip-h); }
  .strip.top .tally { height: var(--peek-h); }
  /* The fades over the row's ends, as tall as the strip. */
  .fade {
    grid-column: 2;
    z-index: 2;
    display: flex;
    align-items: center;
    width: calc(var(--hw) * 0.7);
    padding: 0 var(--space-2);
    font: var(--fw-num) max(var(--fs-num-s), 0.85em) var(--font-mono);
    color: var(--mat-ink);
  }
  .fade.left { justify-self: start; background: linear-gradient(270deg, transparent, var(--mat-surface) 65%); }
  .fade.right { justify-self: end; justify-content: flex-end; background: linear-gradient(90deg, transparent, var(--mat-surface) 65%); }
  .strip.bottom .fade { height: var(--strip-h); }
  .strip.top .fade { height: var(--peek-h); }
  /* The band under your strip takes a card dropped between its cards. */
  .band { position: absolute; left: 0; right: 0; bottom: 0; height: var(--strip-h); pointer-events: auto; }
  /* The opponent's strip: each card turned 180°, facing across the table
     to its owner, and pushed up off the board's edge so only its top strip
     shows: the name and type band (upside down, as you'd see them across
     a real table) along the strip's lower edge. */
  .strip.top .hand.edge { height: var(--peek-h); }
  .peek .cards { align-items: flex-end; height: 100%; }
  .peek .copy { rotate: 180deg; }
</style>
