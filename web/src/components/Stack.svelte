<script>
  // A Pokémon slot: the evolution chain's top card (the chain itself is in
  // its sheet's header), its Tools and Special
  // Energy tucked behind it, and its readout: on the Bench, HP left and
  // Energy dots under the card; in the Active Spot, a panel edged in the
  // player's colour with the name, HP left over maximum, an HP bar, the
  // damage tag, the Energy and the Tools' names. Special Conditions turn
  // the card and mark it.
  import { app, openSheet, pointerTypeOf } from '../lib/store.svelte.js';
  import { dropOnStack, stackMenu, attachmentMenu, tuckedOf, providedEnergy, hpLeft, hpOf, TUCK_CAP } from '../lib/actions.js';
  import { draggable, dropZone } from '../lib/pointer.svelte.js';
  import { act } from '../lib/ws.js';
  import Card from './Card.svelte';

  // reserve is how many tucked strips a benched slot leaves room for above
  // its card: the most any slot in its Bench row shows, so the row's cards
  // stay level. The Active leaves room for its own.
  let { seat, slot, stack, label = '', reserve = 0 } = $props();

  const spec = $derived(`${seat}:${slot}`);
  const active = $derived(slot === 'active');
  const top = $derived(stack?.pokemon?.at(-1));
  const conds = $derived(stack?.conditions || []);
  // Each Special Condition is named in the Active's readout. The card is
  // not turned as at a real table: an upside-down card read as a fault.
  const CONDITION_LABELS = [['asleep', 'Asleep'], ['burned', 'Burned'], ['confused', 'Confused'], ['paralyzed', 'Paralyzed'], ['poisoned', 'Poisoned']];
  const condLabels = $derived(CONDITION_LABELS.filter(([c]) => conds.includes(c)));
  const hp = $derived(hpOf(stack));
  const left = $derived(hpLeft(stack));
  const shown = $derived(left === null ? null : Math.max(left, 0));
  const damage = $derived(stack?.damage || 0);
  const pct = $derived(hp && shown !== null ? Math.round((shown / hp) * 100) : 0);

  // One dot per Energy provided (a Double Colorless Energy gives two), a
  // Special Energy's ringed.
  const energy = $derived(providedEnergy(stack));
  // The phone draws no attachments on the board, as its design has it: the
  // Energy shows as dots, and every attachment is listed in the Pokémon's
  // sheet (stackMenu's Attached section), which reaches its own menu.
  const phone = $derived(app.layout === 'phone');
  // Elsewhere a Basic Energy is a dot only, and the Tools and Special
  // Energy are tucked behind the card, a strip of each showing above it
  // (at most TUCK_CAP; the Pokémon's Attached list has every one).
  const strips = $derived(phone || !stack ? [] : tuckedOf(stack).slice(0, TUCK_CAP));
  const tucks = $derived(phone ? 0 : active ? strips.length : reserve);
  const tools = $derived(stack?.tools || []);

  function menu(ev, pointerType) {
    if (stack) openSheet(ev, () => stackMenu(seat, slot, stack), pointerType);
  }
  // With a mouse a tucked card's strip is its own: a click opens its view
  // (the zoom, with its actions), a right-click its menu, and it drags off.
  // On a touch screen the strips are part of the Pokémon (CSS lets taps
  // through them to the pile): a tap opens the Pokémon's sheet, whose
  // Attached list reaches each card.
  const tuckMenu = (t) => (ev, pointerType) => {
    if ((pointerType ?? pointerTypeOf(ev)) === 'mouse') openSheet(ev, () => attachmentMenu(t.card, t.role), 'mouse');
    else menu(ev, pointerType);
  };
  // pileTap opens the Pokémon's sheet for a tap on its strips (they let a
  // touch through to the pile), never for a tap that reached a card or a
  // button inside it.
  function pileTap(ev) {
    if (ev.target === ev.currentTarget && strips.length && pointerTypeOf(ev) !== 'mouse') menu(ev);
  }
  const dmg = (amount) => act('place_damage', { target: spec, amount });
</script>

{#snippet dots(size)}
  {#if energy.length}
    <span class="dots {size}" aria-label="{energy.length} Energy">
      {#each energy as e (e.key)}<i data-e={e.type} class:special={e.special}></i>{/each}
    </span>
  {/if}
{/snippet}

<div
  class="slot {seat}"
  class:empty={!stack}
  class:active
  class:reserve={tucks > 0}
  style:--tucks={tucks || null}
  {@attach dropZone(spec, (data) => dropOnStack(data, seat, slot, !!stack))}
  role="region"
  aria-label={label || spec}
>
  {#if stack}
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <div class="pile" {@attach draggable(app.opponent !== 'acting' && { kind: 'stack', from: spec })} role="group" onclick={pileTap}>
      <div class="main">
        <Card card={top} size={active ? 'lg' : 'md'} from={spec} draggable={false} onmenu={menu} />
        {#if strips.length}
          <!-- Tucked behind the card, as at a table: each strip is a box
               the height of what shows of its card, the card hanging down
               from it behind the Pokémon. -->
          <div class="tucks">
            {#each strips as t, i (t.card.iid)}
              <div class="tuck" class:first={i === 0} style:--i={i} style:z-index={TUCK_CAP - i}>
                <Card card={t.card} from={spec} role={t.role} onmenu={tuckMenu(t)} onclick={() => (app.zoom = t.card)} />
              </div>
            {/each}
          </div>
        {/if}
        <div class="badges">
          {#if stack.damage_turn}<span class="dmg" title="{stack.damage_turn} damage this turn; {stack.damage} in total">+{stack.damage_turn}</span>{/if}
        </div>
        <div class="controls">
          <button onclick={() => dmg(-10)} title="Heal 10">−</button>
          <button onclick={() => dmg(10)} title="+10 damage">+10</button>
          <button onclick={() => dmg(50)} title="+50 damage">+50</button>
          <button onclick={menu} title="More">⋯</button>
        </div>
      </div>
    </div>
    {#if active}
      <!-- The Active's readout: name, HP left over maximum, the HP bar, the
           damage tag and the Energy, readable without opening the card. -->
      <div class="readout">
        <span class="name">{top?.name || ''}</span>
        {#if hp && shown !== null}
          <span class="hp"><span class="hpleft" class:low={shown <= hp * 0.3}><b>{shown}</b><span class="of"><span class="slash">/</span>{hp}</span></span>{#if !phone}<span class="unit">HP</span>{/if}</span>
          <span class="bar" aria-hidden="true"><span style:width="{pct}%"></span></span>
        {/if}
        <span class="tags">
          {#if damage}<span class="tag" title="{damage} damage">−{damage}</span>{/if}
          {#each condLabels as [c, name] (c)}<span class="cond" data-c={c}>{name}</span>{/each}
          {@render dots('lg')}
          {#if energy.length && !phone}<span class="count">{energy.length} Energy</span>{/if}
          {#if !phone}{#each tools as t (t.iid)}<span class="tool" title="Tool: {t.name}">{t.name}</span>{/each}{/if}
        </span>
      </div>
    {:else}
      <!-- A benched Pokémon's readout: HP left, pink when damaged, and its
           Energy as dots. -->
      <div class="readout bench-readout">
        {#if hp && shown !== null}<span class="left" class:hurt={damage > 0} title="{shown}/{hp} HP">{shown}</span>{/if}
        {@render dots('sm')}
      </div>
    {/if}
  {:else}
    <span class="hint">{active ? 'Active' : ''}</span>
  {/if}
</div>

<style>
  /* A slot is as wide as its card; an empty one is a recessed box in the
     card's shape. --sw is the slot's width, --pw its Pokémon card's. */
  .slot {
    --sw: var(--cw);
    --pw: var(--sw);
    --seat-c: var(--seat-p1-mat);
    width: var(--sw);
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--space-1);
    position: relative;
    border-radius: var(--radius-s);
    transition: background 0.15s, box-shadow 0.15s;
  }
  .slot.p2 { --seat-c: var(--seat-p2-mat); }
  /* Today's mat: a benched card is a little under a mat card width (62px
     beside the desktop's 72px hand), the Active a little over it. */
  :global(.side.mat) .slot:not(.active) { --sw: calc(var(--cw) * 0.88); }
  :global(.side.mat) .slot:not(.active) .main :global(.card) { --w: var(--sw); }
  @media (pointer: coarse) {
    .slot { --sw: max(44px, var(--cw)); }
    :global(.side.mat) .slot:not(.active) { --sw: max(44px, calc(var(--cw) * 0.88)); }
  }

  /* Tucked cards (Tools and Special Energy). Each is 84% of the Pokémon
     card's width, centred behind it; the first shows a strip 0.42 of the
     Pokémon's width tall above it (the name band and the top of the art),
     the second a band 0.2 of the width higher. --tuck-r is the room a slot
     leaves above its card for --tucks strips: on the Bench, the most any
     slot in the row shows (Side.svelte), so the row's cards stay level. */
  .slot {
    --tuck-w: calc(var(--pw) * 0.84);
    --tuck-s: calc(var(--pw) * 0.42);
    --tuck-b: calc(var(--pw) * 0.2);
    --tuck-r: calc(var(--tuck-s) + (var(--tucks, 1) - 1) * var(--tuck-b));
  }
  .slot.reserve:not(.empty) .pile { padding-top: var(--tuck-r); }
  .slot.empty.reserve:not(.active) { margin-top: var(--tuck-r); }
  /* The tucks share the card's box (and turn with it), behind the card. */
  .tucks {
    position: absolute;
    inset: 0;
    z-index: -1;
  }
  /* A strip: the box of what shows of its card, the card hanging from it. */
  .tuck {
    position: absolute;
    left: 50%;
    width: var(--tuck-w);
    translate: -50% 0;
    top: calc(-1 * (var(--tuck-s) + var(--i) * var(--tuck-b)));
    height: var(--tuck-b);
  }
  .tuck.first { height: var(--tuck-s); }
  .tuck :global(.card) { --w: var(--tuck-w); position: absolute; top: 0; left: 0; }
  /* On a touch screen the strips are not targets of their own: a tap goes
     through them to the pile, which opens the Pokémon's sheet. */
  @media (pointer: coarse) {
    .tucks { pointer-events: none; }
  }
  /* With a mouse, a strip rolled over comes out in front, enlarged. */
  @media (hover: hover) {
    :global(body:not(.dragging)) .tucks:has(:global(.card.lift:hover)) { z-index: 2; }
  }
  /* The landscape mat's readouts are sized to its larger cards, a little
     under its text. */
  :global(.board) .readout { font-size: 0.8em; }
  .slot.empty {
    align-self: flex-start;
    aspect-ratio: 63 / 88;
    justify-content: center;
    background: var(--mat-slot);
    box-shadow: inset 0 0 0 1px var(--mat-slot-line);
  }
  .hint { color: var(--mat-muted); font-size: max(var(--fs-label), 0.85em); font-weight: var(--fw-label); text-transform: uppercase; letter-spacing: var(--ls-label); }
  .pile { display: flex; flex-direction: column; align-items: center; cursor: grab; touch-action: none; }
  .main { position: relative; z-index: 1; order: 0; }
  .badges {
    position: absolute;
    top: var(--space-1);
    right: var(--space-1);
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: 3px;
    pointer-events: none;
  }
  .badges .dmg { pointer-events: auto; cursor: help; }
  .badges span {
    font: var(--fw-num) max(var(--fs-num-s), 0.85em)/1 var(--font-mono);
    padding: 0.25em 0.45em;
    border-radius: var(--radius-xs);
    box-shadow: var(--shadow-badge);
  }
  .dmg { background: var(--damage-bg); color: var(--damage-ink); }
  .controls {
    position: absolute;
    bottom: var(--space-1);
    left: 50%;
    transform: translateX(-50%);
    display: flex;
    gap: 2px;
    opacity: 0;
    transition: opacity 0.15s;
    z-index: 50; /* above the card while it is enlarged on hover */
  }
  /* The quick damage buttons need a hover; on a touch screen the action
     sheet's Damage stepper does their job. */
  @media (hover: hover) {
    .main:hover .controls { opacity: 1; }
  }
  @media (hover: none) {
    .controls { display: none; }
  }
  .controls button {
    font: var(--fw-num) max(var(--fs-num-s), 0.8em) var(--font-mono);
    padding: 0.2em 0.45em;
    border-radius: var(--radius-xs);
    border: 1px solid var(--mat-slot-line);
    background: var(--mat-strip);
    color: var(--mat-ink);
    cursor: pointer;
  }

  /* Energy dots, in each Energy type's colour. */
  .dots { display: inline-flex; flex-wrap: wrap; gap: 2px; }
  .dots i { width: max(8px, 0.65em); aspect-ratio: 1; border-radius: 50%; background: var(--energy-colorless); }
  .dots.lg { gap: 3px; flex-wrap: nowrap; }
  .dots.lg i { width: max(10px, 0.8em); }
  .dots i[data-e='grass'] { background: var(--energy-grass); }
  .dots i[data-e='fire'] { background: var(--energy-fire); }
  .dots i[data-e='water'] { background: var(--energy-water); }
  .dots i[data-e='lightning'] { background: var(--energy-lightning); }
  .dots i[data-e='psychic'] { background: var(--energy-psychic); }
  .dots i[data-e='fighting'] { background: var(--energy-fighting); }
  .dots i[data-e='darkness'] { background: var(--energy-darkness); }
  .dots i[data-e='metal'] { background: var(--energy-metal); }
  /* Energy of every type (Rainbow Energy) in every type's colour. */
  .dots i[data-e='any'] { background: conic-gradient(var(--energy-grass), var(--energy-fire), var(--energy-lightning), var(--energy-fighting), var(--energy-psychic), var(--energy-water), var(--energy-darkness), var(--energy-metal), var(--energy-grass)); }
  /* A Special Energy's dots are ringed, apart from the Basic ones. */
  .dots i.special { outline: 1.5px solid var(--mat-ink); outline-offset: 1px; margin: 0 1.5px; }

  /* The Bench readout: HP left in mono, pink when damaged, and the dots. */
  /* It sits at the slot's foot, so a row's readouts line up whether or
     not a card has attachments showing below it. */
  .bench-readout {
    margin-top: auto;
    align-self: stretch;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-1);
    min-height: max(14px, 1.2em);
    padding: 0 2px;
  }
  .left { font: var(--fw-num) max(var(--fs-num-s), 0.92em)/1 var(--font-mono); color: var(--mat-ink); }
  .left.hurt { color: var(--damage-ink); }

  /* The Active Spot: a zone panel edged in the player's colour, the card
     on the left and the readout beside it. */
  .slot.active {
    --pw: calc(var(--cw) * 1.25);
    width: auto;
    flex-direction: row;
    align-items: center;
    gap: var(--space-3);
    padding: calc(var(--space-1) * 1.5) var(--space-3);
    border-radius: var(--radius-m);
    background: var(--mat-zone);
    box-shadow: inset 0 0 0 1.5px var(--seat-c);
  }
  .slot.active.empty { aspect-ratio: auto; justify-content: center; min-width: calc(var(--sw) * 1.25 + var(--space-6)); min-height: calc(var(--sw) * 1.25 * 88 / 63 + var(--space-4)); }
  /* Today's mat gives the Active a little less than its full size, so
     both halves and the strip fit the screen. */
  :global(.side.mat) .slot.active,
  :global(.strip.wide) .slot.active { --pw: calc(var(--cw) * 1.1); }
  :global(.side.mat) .slot.active .main > :global(.card.lg) { --w: var(--pw); }
  /* The readout is as wide as its damage tag, dots and "2 Energy" on one
     line, and at least the design's 150px; a long name does not widen it
     (contain), but ends in an ellipsis. */
  .readout:not(.bench-readout) {
    width: max-content;
    min-width: max(150px, 8.5em);
    display: flex;
    flex-direction: column;
    gap: 0.25em;
    color: var(--mat-ink);
  }
  .name { contain: inline-size; font-size: max(var(--fs-ui-title), 1.15em); font-weight: var(--fw-ui-title); line-height: 1.2; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .hp { display: flex; align-items: baseline; gap: 0.3em; font-family: var(--font-mono); }
  .hpleft b { font-size: max(var(--fs-hp-xl), 2em); font-weight: var(--fw-hp-xl); line-height: var(--lh-hp-xl); }
  .hpleft .of, .unit { font-size: max(var(--fs-num), 1.08em); font-weight: 500; color: var(--mat-muted); }
  .slash { margin: 0 0.3em; }
  .bar { display: block; height: max(6px, 0.5em); border-radius: var(--radius-pill); background: var(--mat-track); overflow: hidden; }
  .bar span { display: block; height: 100%; border-radius: inherit; background: var(--hp); }
  .tags { display: flex; align-items: center; gap: 0.6em; white-space: nowrap; min-height: 1.4em; font-size: max(var(--fs-ui-s), 1em); color: var(--mat-muted); }
  /* A Tool's name, so the Active's readout says what it holds. */
  .tool { padding: 1px 0.5em; border-radius: var(--radius-xs); background: var(--mat-chip); color: var(--mat-ink); font: var(--fw-ui) max(var(--fs-ui-s), 0.92em) var(--font-sans); }
  /* A Special Condition's name: Poisoned and Burned in their colours, the
     others in the mat's chip. */
  .cond { padding: 1px 0.5em; border-radius: var(--radius-xs); background: var(--mat-chip); color: var(--mat-ink); font: var(--fw-ui) max(var(--fs-ui-s), 0.92em) var(--font-sans); }
  .cond[data-c='poisoned'] { background: var(--condition-poisoned); color: var(--glyph-ink); }
  .cond[data-c='burned'] { background: var(--condition-burned); color: var(--glyph-ink); }
  .tag { padding: 1px 0.5em; border-radius: var(--radius-xs); background: var(--damage-bg); color: var(--damage-ink); font: var(--fw-num) max(var(--fs-ui-s), 1em) var(--font-mono); }

  /* The phone (the PhoneBoard design): flat cards. A benched card is one
     --cw (56px on a 390 × 844 phone) with a 14px line under it, 3px below
     the card: HP left in 10px mono and 7px Energy dots. The Active panel
     spans the width, 8px by 12px inside, with the card on the left (yours
     1.25 --cw, 70px; the opponent's 60px) and the readout beside it: the
     name in 13px, HP left in 22px over "/ 80" in 12px, a 5px bar, then the
     damage tag in 11px and 9px dots, 5px between the lines. */
  :global(.app.phone) .slot :global(.card) { box-shadow: none; }
  :global(.app.phone) .slot:not(.active) { gap: 3px; }
  :global(.app.phone) .bench-readout { min-height: 14px; height: 14px; padding: 0 1px; }
  :global(.app.phone) .left { font: var(--fw-num) var(--fs-10)/1 var(--font-mono); }
  :global(.app.phone) .dots.sm i { width: 7px; }
  :global(.app.phone) .dots i.special { outline-width: 1px; margin: 0 1px; }
  :global(.app.phone) .slot.active {
    flex: 1;
    justify-content: center;
    padding: var(--space-2) var(--space-3);
    border-radius: 12px;
  }
  :global(.app.phone .side.mirrored) .slot.active .main :global(.card.lg) { --w: calc(var(--cw) * 60 / 56); }
  :global(.app.phone) .readout:not(.bench-readout) { flex: 1; width: auto; min-width: 0; gap: 5px; }
  :global(.app.phone) .name { font-size: var(--fs-13); line-height: normal; }
  :global(.app.phone) .hp { gap: var(--space-1); line-height: 1; }
  :global(.app.phone) .hpleft b { font-size: var(--fs-22); line-height: 1; }
  :global(.app.phone) .hpleft .of { font-size: var(--fs-12); }
  :global(.app.phone) .slash { margin: 0 var(--space-1); }
  :global(.app.phone) .bar { height: 5px; border-radius: 3px; }
  :global(.app.phone) .tags { gap: 6px; min-height: 0; font-size: var(--fs-11); line-height: normal; }
  :global(.app.phone) .tag, :global(.app.phone) .cond { padding: 0 5px; font-size: var(--fs-11); line-height: normal; }
  :global(.app.phone) .dots.lg { gap: 6px; }
  :global(.app.phone) .dots.lg i { width: 9px; }
</style>
