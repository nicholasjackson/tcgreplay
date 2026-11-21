<script>
  // An attack, set up in the sheet: its printed damage, Weakness (×2) and
  // Resistance (−30) toggled on top of it, a target (the opponent's Active
  // unless one is tapped on the board), a note when the damage is enough to
  // Knock the target Out, and one button that says what will happen.
  // Without attack data the player names the attack.
  import { app } from '../lib/store.svelte.js';
  import { findCard, hpLeft, ownStacks, seatArgs } from '../lib/actions.js';
  import { startTarget } from '../lib/target.js';
  import { act } from '../lib/ws.js';
  import Stepper from './Stepper.svelte';
  import Card from './Card.svelte';

  let { seat, attack = null, card, close } = $props();

  const opp = $derived(seat === 'p1' ? 'p2' : 'p1');
  let name = $state(attack?.name ?? '');
  // base is the stepper's damage; Weakness doubles it, then Resistance
  // takes 30 off, never below 0. damage is what the attack deals.
  let base = $state(parseInt(attack?.damage, 10) || 0);
  let weak = $state(false);
  let resist = $state(false);
  const damage = $derived(Math.max(0, (weak ? base * 2 : base) - (resist ? 30 : 0)));
  // svelte-ignore state_referenced_locally
  let target = $state(`${opp}:active`);
  const candidates = $derived(ownStacks(opp));
  const chosen = $derived(candidates.find((c) => c.spec === target));
  const left = $derived(hpLeft(chosen?.stack));
  const where = $derived(target.endsWith(':active') ? 'Active' : `Bench ${Number(target.split(':')[2]) + 1}`);
  const oppName = $derived(app.board?.sides[opp]?.name || opp);
  // place is where the target is and, when known, its HP left.
  const place = $derived(`${oppName}'s ${where}${left === null ? '' : ` · ${Math.max(left, 0)} HP left`}`);
  // The cost dots of the attack's Energy, as on the menu.
  const cost = $derived((attack?.cost || []).map((c) => c.toLowerCase()));

  // change hides the sheet while the target is tapped on the board, then
  // brings it back with the choice.
  function change() {
    app.sheet.hidden = true;
    startTarget({
      prompt: `Target for ${name || 'the attack'}`,
      hint: "Tap one of your opponent's Pokémon",
      card,
      candidates,
      run: (spec) => (target = spec),
      done: () => app.sheet && (app.sheet.hidden = false),
    });
  }
  // confirm reads the panel before closing the sheet, which unmounts it.
  function confirm() {
    const args = { name: name.trim(), damage, target, ...seatArgs(seat) };
    close();
    act('attack', args);
  }
</script>

<div class="attack">
  {#if attack}
    <div class="ahead">
      <div class="aname">
        {#if cost.length}<span class="cost" aria-hidden="true">{#each cost as c, i (i)}<i class="dot" data-e={c}></i>{/each}</span>{/if}
        <h2>{attack.name}</h2>
        {#if attack.damage}<span class="printed">{attack.damage}</span>{/if}
      </div>
      {#if attack.effect}<p>{attack.effect}</p>{/if}
    </div>
  {:else}
    <label class="name">Attack name <input bind:value={name} placeholder="e.g. Aura Sphere" /></label>
  {/if}

  <section class="sec" role="group" aria-label="Damage">
    <div class="label">Damage</div>
    <div class="dmg">
      <Stepper value={base} label="Attack damage" set={(v) => (base = v)} />
      <div class="mods">
        <button class="tog" class:on={weak} aria-pressed={weak} onclick={() => (weak = !weak)}>×2 Weakness</button>
        <button class="tog" class:on={resist} aria-pressed={resist} onclick={() => (resist = !resist)}>−30 Resistance</button>
      </div>
    </div>
  </section>

  <section class="sec" role="group" aria-label="Target">
    <div class="label">Target</div>
    <div class="target">
      {#if chosen}<span class="thumb {opp}"><Card card={findCard(chosen.iid)} size="xs" draggable={false} lift={false} /></span>{/if}
      <div class="who">
        <strong>{chosen?.name ?? 'No Pokémon'}</strong>
        <span class="where">{place}</span>
      </div>
      <button class="change" onclick={change} disabled={candidates.length < 2}>Change ›</button>
    </div>
    {#if chosen && left !== null && damage >= left}
      <p class="ko">{damage} is enough to Knock Out {chosen.name}.</p>
    {/if}
  </section>

  <button class="primary go" onclick={confirm} disabled={!name.trim() || !chosen}>
    Attack {chosen?.name ?? ''} for {damage}
  </button>
</div>

<style>
  .attack { display: flex; flex-direction: column; gap: var(--space-1); padding: var(--space-2) var(--space-4) var(--space-4); font-family: var(--font-sans); }
  .ahead { display: flex; flex-direction: column; gap: var(--space-2); padding: var(--space-2) 0; }
  .aname { display: flex; align-items: center; gap: 10px; }
  h2 { margin: 0; flex: 1; min-width: 0; font: 600 var(--fs-22)/1.2 var(--font-sans); }
  .printed { font: 600 var(--fs-22) var(--font-mono); }
  .ahead p { margin: 0; font-size: var(--fs-14); line-height: 1.5; color: var(--muted); }
  .cost { display: flex; gap: 3px; }
  .dot { display: block; width: 14px; height: 14px; border-radius: 50%; background: var(--energy-colorless); }
  .dot[data-e='grass'] { background: var(--energy-grass); }
  .dot[data-e='fire'] { background: var(--energy-fire); }
  .dot[data-e='water'] { background: var(--energy-water); }
  .dot[data-e='lightning'] { background: var(--energy-lightning); }
  .dot[data-e='psychic'] { background: var(--energy-psychic); }
  .dot[data-e='fighting'] { background: var(--energy-fighting); }
  .dot[data-e='darkness'] { background: var(--energy-darkness); }
  .dot[data-e='metal'] { background: var(--energy-metal); }
  .name { display: flex; flex-direction: column; gap: var(--space-1); padding: var(--space-2) 0; font-weight: 600; }
  .name input {
    min-height: var(--touch-target);
    font: var(--fs-16) var(--font-sans);
    padding: 0 10px;
    border-radius: var(--radius-m);
    border: 1px solid var(--control-line);
    background: var(--panel-sunk);
    color: var(--ink);
  }
  .sec { display: flex; flex-direction: column; gap: var(--space-2); padding: var(--space-3) 0 var(--space-1); }
  .label {
    font: var(--fw-label) var(--fs-label)/var(--lh-label) var(--font-sans);
    letter-spacing: var(--ls-label);
    text-transform: uppercase;
    color: var(--muted);
  }
  /* Damage: the stepper, then Weakness and Resistance beside it, or on
     their own row on a phone. */
  .dmg { display: flex; align-items: center; flex-wrap: wrap; gap: var(--space-2) var(--space-3); min-width: 0; }
  .dmg :global(.stepper) { padding: 0; flex-wrap: nowrap; }
  .mods { display: flex; gap: 6px; }
  button {
    min-height: var(--touch-target);
    padding: 0 var(--space-4);
    border-radius: var(--radius-m);
    border: 1px solid var(--control-line);
    background: var(--panel-raised);
    color: var(--ink);
    font: 600 var(--fs-14) var(--font-sans);
    white-space: nowrap;
  }
  button:hover:not(:disabled) { border-color: var(--control-line); background: var(--hover); }
  button:focus-visible { outline: 2px solid var(--primary); outline-offset: 1px; }
  .tog.on,
  .tog.on:hover:not(:disabled) { background: var(--primary); border-color: var(--primary); color: var(--on-primary); }
  /* The target: its card ringed in its player's colour, name, place and
     HP left, and Change. */
  .target {
    display: flex;
    align-items: center;
    gap: 14px;
    padding: 10px 10px 10px var(--space-3);
    border-radius: var(--radius-l);
    border: 1px solid var(--control-line);
    background: var(--panel-sunk);
  }
  .thumb { --cw: 105px; display: flex; border-radius: var(--radius-xs); }
  .thumb.p1 { box-shadow: 0 0 0 2px var(--seat-p1-mat); }
  .thumb.p2 { box-shadow: 0 0 0 2px var(--seat-p2-mat); }
  .who { display: flex; flex-direction: column; gap: 2px; flex: 1; min-width: 0; }
  .who strong { font-size: var(--fs-16); font-weight: 600; }
  .where { font: 500 var(--fs-13) var(--font-mono); color: var(--muted); }
  .change { padding: 0 var(--space-3) 0 var(--space-4); }
  .ko { margin: 0; font-size: var(--fs-13); color: var(--muted); }
  .go,
  .go:hover:not(:disabled) {
    margin-top: var(--space-4);
    min-height: 56px;
    border-radius: var(--radius-l);
    border: 0;
    background: var(--primary);
    color: var(--on-primary);
    font: 700 var(--fs-16) var(--font-sans);
  }
  /* On a touch screen the controls grow to the panel's sizes. */
  :global(.sheet.tablet) .dmg :global(.stepper button),
  :global(.sheet.tablet) .mods button { min-height: 52px; }
  :global(.sheet.tablet) .dmg :global(.stepper button) { min-width: 56px; font-size: var(--fs-16); }
  :global(.sheet.tablet) .dmg :global(.stepper input) { width: 110px; min-height: 52px; font-size: var(--fs-22); }
  :global(.sheet.tablet) .ahead p { font-size: var(--fs-15); }
  :global(.sheet.tablet) .attack { padding: var(--space-5) var(--space-6) var(--space-6); }
  :global(.sheet.tablet) h2 { font-size: var(--fs-24); }
  :global(.sheet.phone) .dmg { flex-direction: column; align-items: stretch; }
  :global(.sheet.phone) .dmg :global(.stepper button) { min-height: 52px; min-width: 56px; }
  :global(.sheet.phone) .dmg :global(.stepper input) { min-height: 52px; font-size: var(--fs-22); }
  :global(.sheet.phone) .mods button { flex: 1; }
  :global(.sheet.phone) .where { font-size: var(--fs-12); }
  /* In the popover everything is compact: 28px controls, 13px text. */
  :global(.sheet.popover) .attack { padding: var(--space-1) 14px var(--space-3); }
  :global(.sheet.popover) h2,
  :global(.sheet.popover) .printed { font-size: var(--fs-14); }
  :global(.sheet.popover) .ahead p,
  :global(.sheet.popover) .where,
  :global(.sheet.popover) .ko { font-size: var(--fs-12); }
  :global(.sheet.popover) .dot { width: 10px; height: 10px; }
  :global(.sheet.popover) .dmg { gap: var(--space-1); }
  :global(.sheet.popover) .dmg :global(.stepper) { padding: 0; }
  :global(.sheet.popover) button { min-height: 28px; padding: 0 var(--space-2); border-radius: var(--radius-s); font-size: var(--fs-12); }
  :global(.sheet.popover) .target { gap: 10px; padding: var(--space-2); border-radius: var(--radius-m); }
  :global(.sheet.popover) .thumb { --cw: 72px; }
  :global(.sheet.popover) .who strong { font-size: var(--fs-13); }
  :global(.sheet.popover) .go { margin-top: var(--space-2); min-height: 36px; border-radius: var(--radius-s); font-size: var(--fs-13); }
</style>
