<script>
  // An attack, set up in the sheet: its printed damage to adjust for
  // Weakness and Resistance, a target (the opponent's Active unless one is
  // tapped on the board), and one button that says what will happen.
  // Without attack data the player names the attack.
  import { app } from '../lib/store.svelte.js';
  import { findCard, ownStacks, seatArgs } from '../lib/actions.js';
  import { startTarget } from '../lib/target.js';
  import { act } from '../lib/ws.js';
  import Stepper from './Stepper.svelte';
  import Card from './Card.svelte';

  let { seat, attack = null, card, close } = $props();

  const opp = $derived(seat === 'p1' ? 'p2' : 'p1');
  let name = $state(attack?.name ?? '');
  let damage = $state(parseInt(attack?.damage, 10) || 0);
  // svelte-ignore state_referenced_locally
  let target = $state(`${opp}:active`);
  const candidates = $derived(ownStacks(opp));
  const chosen = $derived(candidates.find((c) => c.spec === target));
  const where = $derived(target.endsWith(':active') ? 'Active' : `Bench ${Number(target.split(':')[2]) + 1}`);
  const oppName = $derived(app.board?.sides[opp]?.name || opp);

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
    <div class="attack-card">
      <div class="head"><strong>{attack.name}</strong>{#if attack.damage}<strong>{attack.damage}</strong>{/if}</div>
      {#if attack.effect}<p>{attack.effect}</p>{/if}
    </div>
  {:else}
    <label class="name">Attack name <input bind:value={name} placeholder="e.g. Aura Sphere" /></label>
  {/if}

  <section class="sec" role="group" aria-label="Damage after Weakness and Resistance">
    <div class="label">Damage after Weakness and Resistance</div>
    <Stepper value={damage} label="Attack damage" set={(v) => (damage = v)}>
      <button onclick={() => (damage *= 2)}>×2</button>
    </Stepper>
  </section>

  <section class="sec" role="group" aria-label="Target">
    <div class="label">Target</div>
    <div class="target">
      {#if chosen}<Card card={findCard(chosen.iid)} size="xs" draggable={false} lift={false} />{/if}
      <div class="who">
        <strong>{chosen?.name ?? 'No Pokémon'}</strong>
        <span>{oppName}'s {where}</span>
      </div>
      <button onclick={change} disabled={candidates.length < 2}>Change ›</button>
    </div>
  </section>

  <button class="primary go" onclick={confirm} disabled={!name.trim() || !chosen}>
    Attack {chosen?.name ?? ''} for {damage}
  </button>
</div>

<style>
  .attack { display: flex; flex-direction: column; gap: 4px; padding-bottom: 16px; }
  .attack-card { margin: 10px 16px; padding: 12px 14px; border-radius: 12px; background: var(--input); border: 1px solid var(--line); }
  .head { display: flex; justify-content: space-between; font-size: 16px; }
  .head strong:last-child { color: var(--accent); }
  .attack-card p { margin: 6px 0 0; color: var(--muted); font-size: 12px; }
  .name { display: flex; flex-direction: column; gap: 4px; padding: 10px 16px; font-weight: 600; }
  .name input { min-height: 44px; font-size: 16px; padding: 0 10px; border-radius: 10px; border: 1px solid var(--line); background: var(--input); color: var(--text); }
  .sec { padding: 10px 0 6px; }
  .label { font-size: 11px; letter-spacing: 0.08em; text-transform: uppercase; color: var(--muted); font-weight: 700; padding: 0 16px 6px; }
  .target { display: flex; align-items: center; gap: 10px; padding: 0 16px; }
  .who { display: flex; flex-direction: column; flex: 1; min-width: 0; }
  .who span { color: var(--muted); font-size: 12px; }
  .target button { min-height: 44px; border-radius: 10px; padding: 0 14px; font-weight: 600; }
  .go { margin: 12px 16px 0; min-height: 52px; font-size: 16px; border-radius: 12px; }
</style>
