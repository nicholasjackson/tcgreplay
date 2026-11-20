<script>
  // A Pokémon slot: the evolution chain's top card, attachments tucked
  // underneath, and its readout: on the Bench, HP left and Energy dots
  // under the card; in the Active Spot, a panel edged in the player's
  // colour with the name, HP left over maximum, an HP bar, the damage tag
  // and the Energy. Special Conditions turn the card and mark it.
  import { app, cardInfo, openSheet } from '../lib/store.svelte.js';
  import { dropOnStack, stackMenu, attachmentMenu } from '../lib/actions.js';
  import { draggable, dropZone } from '../lib/pointer.svelte.js';
  import { act } from '../lib/ws.js';
  import Card from './Card.svelte';

  let { seat, slot, stack, label = '' } = $props();

  const spec = $derived(`${seat}:${slot}`);
  const active = $derived(slot === 'active');
  const top = $derived(stack?.pokemon?.at(-1));
  const conds = $derived(stack?.conditions || []);
  // Asleep turns the card counter-clockwise, Paralyzed clockwise, Confused
  // upside down, as at a real table.
  const rot = $derived(conds.includes('confused') ? 180 : conds.includes('asleep') ? -90 : conds.includes('paralyzed') ? 90 : 0);
  const info = $derived(top ? cardInfo(top.def) : null);
  const hp = $derived(stack?.hp || info?.hp || 0);
  const left = $derived(stack?.hp_left ?? (hp ? hp - (stack?.damage || 0) : null));
  const shown = $derived(left === null ? null : Math.max(left, 0));
  const damage = $derived(stack?.damage || 0);
  const pct = $derived(hp && shown !== null ? Math.round((shown / hp) * 100) : 0);

  // energyType is an attached Energy's type, from its name ("Basic Fighting
  // Energy"); a Special Energy shows as Colorless.
  const TYPES = ['grass', 'fire', 'water', 'lightning', 'psychic', 'fighting', 'darkness', 'metal'];
  function energyType(card) {
    const name = (cardInfo(card.def)?.name || card.name || '').toLowerCase();
    return TYPES.find((t) => name.includes(t)) || 'colorless';
  }
  const energy = $derived((stack?.energy || []).map((e) => ({ iid: e.iid, type: energyType(e) })));

  function menu(ev, pointerType) {
    if (stack) openSheet(ev, () => stackMenu(seat, slot, stack), pointerType);
  }
  const dmg = (amount) => act('place_damage', { target: spec, amount });
</script>

{#snippet dots(size)}
  {#if energy.length}
    <span class="dots {size}" aria-label="{energy.length} Energy">
      {#each energy as e (e.iid)}<i data-e={e.type}></i>{/each}
    </span>
  {/if}
{/snippet}

<div
  class="slot {seat}"
  class:empty={!stack}
  class:active
  {@attach dropZone(spec, (data) => dropOnStack(data, seat, slot, !!stack))}
  role="region"
  aria-label={label || spec}
>
  {#if stack}
    <div class="pile" {@attach draggable(app.opponent !== 'acting' && { kind: 'stack', from: spec })} role="group">
      {#if stack.tools?.length || stack.energy?.length}
      <div class="attachments">
        {#each stack.tools || [] as t (t.iid)}
          <div class="att tool">
            <Card card={t} size="xs" from={spec} role="tool" onmenu={(ev, type) => openSheet(ev, () => attachmentMenu(t, 'tool'), type)} />
          </div>
        {/each}
        {#each stack.energy || [] as e (e.iid)}
          <div class="att">
            <Card card={e} size="xs" from={spec} role="energy" onmenu={(ev, type) => openSheet(ev, () => attachmentMenu(e, 'energy'), type)} />
          </div>
        {/each}
      </div>
      {/if}
      <div class="main">
        <Card card={top} size={active ? 'lg' : 'md'} from={spec} rotate={rot} draggable={false} onmenu={menu} />
        {#if stack.pokemon.length > 1}
          <span class="chain" title={stack.pokemon.map((p) => p.name).join(' → ')}>{stack.pokemon.length}</span>
        {/if}
        <div class="badges">
          {#if stack.damage_turn}<span class="dmg" title="{stack.damage_turn} damage this turn; {stack.damage} in total">+{stack.damage_turn}</span>{/if}
          {#if conds.includes('poisoned')}<span class="marker poison" title="Poisoned">P</span>{/if}
          {#if conds.includes('burned')}<span class="marker burn" title="Burned">B</span>{/if}
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
          <span class="hp"><span class="hpleft" class:low={shown <= hp * 0.3}><b>{shown}</b><span class="of"><span class="slash">/</span>{hp}</span></span><span class="unit">HP</span></span>
          <span class="bar" aria-hidden="true"><span style:width="{pct}%"></span></span>
        {/if}
        <span class="tags">
          {#if damage}<span class="tag" title="{damage} damage">−{damage}</span>{/if}
          {@render dots('lg')}
          {#if energy.length}<span class="count">{energy.length} Energy</span>{/if}
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
     card's shape. --sw is the card width, --att-w an attachment's. */
  .slot {
    --sw: var(--cw);
    --att-w: calc(var(--cw) * 0.42);
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
  @media (pointer: coarse) {
    .slot { --sw: max(44px, var(--cw)); --att-w: max(44px, calc(var(--cw) * 0.42)); }
    /* Today's mat only: the phone has its own rule (app.css), and the
       landscape mat's cards are large enough to keep the attachments
       tucked in; hanging below them there, the attachments make its fit
       to the screen (fitBoard) stall for seconds after a resize. */
    :global(.app:not(.phone, .landscape)) .pile .attachments { margin-top: calc(var(--sw) * -0.3); }
  }
  /* The landscape mat's readouts are sized to its larger cards, a little
     under its text. */
  :global(.board) .readout { font-size: 0.8em; }
  /* The phone's Bench readouts are a slim line. */
  :global(.app.phone) .bench-readout { min-height: 12px; }
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
  /* Energy and Tools tuck over the bottom of the Pokémon, as at a table,
     instead of taking a row of their own; on a touch screen, where they are
     a fingertip wide, most of each shows below it, clear of the middle of
     the card a finger taps. */
  .attachments {
    order: 1;
    display: flex;
    justify-content: center;
    max-width: calc(var(--sw) * 1.4);
    margin-top: calc(var(--att-w) * -88 / 63 - 2px);
    position: relative;
    z-index: 2;
  }
  .att { margin-left: calc(var(--att-w) * -0.3); }
  .att:first-child { margin-left: 0; }
  .chain {
    position: absolute;
    top: calc(var(--space-1) * -1);
    left: calc(var(--space-1) * -1);
    background: var(--mat-strip);
    color: var(--mat-ink);
    border-radius: 50%;
    width: 1.6em;
    height: 1.6em;
    display: grid;
    place-items: center;
    font: var(--fw-num) max(var(--fs-num-s), 0.85em) var(--font-mono);
    box-shadow: 0 0 0 2px var(--accent);
  }
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
  .marker.poison { background: var(--poison); color: var(--on-color); }
  .marker.burn { background: var(--burn); color: var(--on-color); }
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
    .att:hover { z-index: 3; }
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
  .dots.lg { gap: 3px; }
  .dots.lg i { width: max(10px, 0.8em); }
  .dots i[data-e='grass'] { background: var(--energy-grass); }
  .dots i[data-e='fire'] { background: var(--energy-fire); }
  .dots i[data-e='water'] { background: var(--energy-water); }
  .dots i[data-e='lightning'] { background: var(--energy-lightning); }
  .dots i[data-e='psychic'] { background: var(--energy-psychic); }
  .dots i[data-e='fighting'] { background: var(--energy-fighting); }
  .dots i[data-e='darkness'] { background: var(--energy-darkness); }
  .dots i[data-e='metal'] { background: var(--energy-metal); }

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
  :global(.side.mat) .slot.active :global(.card.lg) { --w: calc(var(--cw) * 1.1); }
  .readout:not(.bench-readout) {
    width: max(150px, 8.5em);
    display: flex;
    flex-direction: column;
    gap: 0.25em;
    color: var(--mat-ink);
  }
  .name { font-size: max(var(--fs-ui-title), 1.15em); font-weight: var(--fw-ui-title); line-height: 1.2; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .hp { display: flex; align-items: baseline; gap: 0.3em; font-family: var(--font-mono); }
  .hpleft b { font-size: max(var(--fs-hp-xl), 2em); font-weight: var(--fw-hp-xl); line-height: var(--lh-hp-xl); }
  .hpleft .of, .unit { font-size: max(var(--fs-num), 1.08em); font-weight: 500; color: var(--mat-muted); }
  .slash { margin: 0 0.3em; }
  .bar { display: block; height: max(6px, 0.5em); border-radius: var(--radius-pill); background: var(--mat-track); overflow: hidden; }
  .bar span { display: block; height: 100%; border-radius: inherit; background: var(--hp); }
  .tags { display: flex; align-items: center; flex-wrap: wrap; gap: 0.6em; min-height: 1.4em; font-size: max(var(--fs-ui-s), 1em); color: var(--mat-muted); }
  .tag { padding: 1px 0.5em; border-radius: var(--radius-xs); background: var(--damage-bg); color: var(--damage-ink); font: var(--fw-num) max(var(--fs-ui-s), 1em) var(--font-mono); }
</style>
