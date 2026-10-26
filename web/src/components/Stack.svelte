<script>
  // A Pokémon slot: the evolution chain's top card, attachments tucked
  // underneath, damage, HP left and Special Conditions.
  import { app, cardInfo } from '../lib/store.svelte.js';
  import { dropOnStack, readDrag, stackMenu, attachmentMenu, startDrag, endDrag } from '../lib/actions.js';
  import { act } from '../lib/ws.js';
  import Card from './Card.svelte';

  let { seat, slot, stack, label = '' } = $props();

  let over = $state(false);
  const spec = $derived(`${seat}:${slot}`);
  const top = $derived(stack?.pokemon?.at(-1));
  const conds = $derived(stack?.conditions || []);
  // Asleep turns the card counter-clockwise, Paralyzed clockwise, Confused
  // upside down, as at a real table.
  const rot = $derived(conds.includes('confused') ? 180 : conds.includes('asleep') ? -90 : conds.includes('paralyzed') ? 90 : 0);
  const info = $derived(top ? cardInfo(top.def) : null);
  const hp = $derived(stack?.hp || info?.hp || 0);
  const left = $derived(stack?.hp_left ?? (hp ? hp - (stack?.damage || 0) : null));

  function menu(ev) {
    if (!stack) return;
    app.menu = { x: ev.clientX, y: ev.clientY, ...stackMenu(seat, slot, stack) };
  }
  function dragover(ev) {
    ev.preventDefault();
    over = true;
  }
  async function drop(ev) {
    ev.preventDefault();
    over = false;
    const data = readDrag(ev);
    if (data) await dropOnStack(data, seat, slot, !!stack);
  }
  function dragStack(ev) {
    startDrag(ev, { kind: 'stack', from: spec });
  }
  const dmg = (amount) => act('place_damage', { target: spec, amount });
</script>

<div
  class="slot"
  class:over
  class:empty={!stack}
  class:active={slot === 'active'}
  ondragover={dragover}
  ondragleave={() => (over = false)}
  ondrop={drop}
  role="region"
  aria-label={label || spec}
>
  {#if stack}
    <div class="pile" draggable={app.opponent !== 'acting'} ondragstart={dragStack} ondragend={endDrag} role="group">
      <div class="attachments">
        {#each stack.tools || [] as t (t.iid)}
          <div class="att tool">
            <Card card={t} size="xs" from={spec} role="tool" onmenu={(ev) => (app.menu = { x: ev.clientX, y: ev.clientY, ...attachmentMenu(t, 'tool') })} />
          </div>
        {/each}
        {#each stack.energy || [] as e (e.iid)}
          <div class="att">
            <Card card={e} size="xs" from={spec} role="energy" onmenu={(ev) => (app.menu = { x: ev.clientX, y: ev.clientY, ...attachmentMenu(e, 'energy') })} />
          </div>
        {/each}
      </div>
      <div class="main">
        <Card card={top} size={slot === 'active' ? 'lg' : 'md'} from={spec} rotate={rot} draggable={false} onmenu={menu} />
        {#if stack.pokemon.length > 1}
          <span class="chain" title={stack.pokemon.map((p) => p.name).join(' → ')}>{stack.pokemon.length}</span>
        {/if}
        <div class="badges">
          {#if stack.damage_turn}<span class="dmg" title="{stack.damage_turn} damage this turn; {stack.damage} in total">{stack.damage_turn}</span>{/if}
          {#if left !== null && hp}<span class="hpleft" class:low={left <= hp * 0.3}>{Math.max(left, 0)}/{hp}</span>{/if}
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
  {:else}
    <span class="hint">{slot === 'active' ? 'Active' : ''}</span>
  {/if}
</div>

<style>
  .slot {
    width: calc(var(--cw) * 1.15);
    min-height: calc(var(--cw) * 1.4 * 1.1);
    border-radius: 8px;
    border: 2px dashed var(--slot-line);
    display: grid;
    place-items: center;
    position: relative;
    transition: background 0.15s, border-color 0.15s;
  }
  .slot.active { width: calc(var(--cw) * 1.45); min-height: calc(var(--cw) * 1.25 * 1.4 + 8px); border-style: solid; }
  .slot:not(.empty) { border-color: transparent; }
  .slot.over { background: var(--drop); border-color: var(--accent); }
  .hint { color: var(--muted); font-size: 0.85em; text-transform: uppercase; letter-spacing: 0.1em; }
  .pile { display: flex; flex-direction: column; align-items: center; gap: 4px; cursor: grab; }
  .main { position: relative; z-index: 1; order: 0; }
  /* Energy and Tools tuck over the bottom of the Pokémon, as at a table,
     instead of taking a row of their own. */
  .attachments {
    order: 1;
    display: flex;
    justify-content: center;
    max-width: calc(var(--cw) * 1.5);
    margin-top: calc(var(--cw) * -0.3);
    position: relative;
    z-index: 2;
  }
  .att { margin-left: calc(var(--cw) * -0.12); }
  .att:first-child { margin-left: 0; }
  .att:hover { z-index: 3; }
  .chain {
    position: absolute;
    top: -6px;
    left: -6px;
    background: #2b2f36;
    color: #fff;
    border-radius: 50%;
    width: 1.5em;
    height: 1.5em;
    display: grid;
    place-items: center;
    font: 700 0.8em system-ui;
    border: 2px solid var(--accent);
  }
  .badges {
    position: absolute;
    top: 4px;
    right: -8px;
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: 3px;
    pointer-events: none;
  }
  .badges .dmg { pointer-events: auto; cursor: help; }
  .badges span {
    font: 700 0.85em/1 system-ui;
    padding: 0.25em 0.5em;
    border-radius: 1em;
    box-shadow: 0 1px 3px #0008;
  }
  .dmg { background: #c62828; color: #fff; }
  .hpleft { background: #f5f5f5; color: #1b5e20; }
  .hpleft.low { color: #b71c1c; }
  .marker.poison { background: #7b1fa2; color: #fff; }
  .marker.burn { background: #ef6c00; color: #fff; }
  .controls {
    position: absolute;
    bottom: 4px;
    left: 50%;
    transform: translateX(-50%);
    display: flex;
    gap: 2px;
    opacity: 0;
    transition: opacity 0.15s;
    z-index: 50; /* above the card while it is enlarged on hover */
  }
  .main:hover .controls { opacity: 1; }
  .controls button {
    font: 600 0.8em system-ui;
    padding: 0.2em 0.45em;
    border-radius: 4px;
    border: none;
    background: #000c;
    color: #fff;
    cursor: pointer;
  }
</style>
