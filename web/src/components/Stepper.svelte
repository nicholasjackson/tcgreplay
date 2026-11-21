<script>
  // A number set without a dialog: −step and +step around the value, any
  // quick additions after a gap, and the value itself, a field to type in.
  // children adds buttons of the caller's own.
  let { value, step = 10, quick = [], min = 0, max = Infinity, label, set, children } = $props();

  const clamp = (v) => Math.min(max, Math.max(min, v));
  function typed(ev) {
    const v = parseInt(ev.target.value, 10);
    if (Number.isFinite(v)) set(clamp(v));
    else ev.target.value = value;
  }
</script>

<div class="stepper">
  <button onclick={() => set(clamp(value - step))} disabled={value <= min}>−{step}</button>
  <input inputmode="numeric" {value} aria-label={label} onchange={typed} onkeydown={(ev) => ev.key === 'Enter' && ev.currentTarget.blur()} />
  <button onclick={() => set(clamp(value + step))} disabled={value >= max}>+{step}</button>
  {#if quick.length}<span class="sp" aria-hidden="true"></span>{/if}
  {#each quick as q (q)}
    <button onclick={() => set(clamp(value + q))} disabled={value >= max}>+{q}</button>
  {/each}
  {@render children?.()}
</div>

<style>
  /* Numbers in mono: outlined buttons around a sunken field. */
  .stepper { display: flex; align-items: center; gap: var(--space-2); padding: 0 var(--space-4); flex-wrap: wrap; }
  .sp { width: var(--space-1); }
  .stepper :global(button) {
    min-height: var(--touch-target);
    min-width: 52px;
    padding: 0 var(--space-3);
    border-radius: var(--radius-m);
    border: 1px solid var(--control-line);
    background: var(--panel-raised);
    color: var(--ink);
    font: 600 var(--fs-15) var(--font-mono);
    white-space: nowrap;
    flex: none;
  }
  .stepper :global(button:hover:not(:disabled)) { border-color: var(--control-line); background: var(--hover); }
  input {
    width: 88px;
    min-height: var(--touch-target);
    box-sizing: border-box;
    border-radius: var(--radius-m);
    background: var(--panel-sunk);
    border: 1px solid var(--control-line);
    color: var(--ink);
    font: 600 var(--fs-18) var(--font-mono);
    text-align: center;
  }
  input:focus-visible { outline: 2px solid var(--primary); outline-offset: 1px; }
</style>
