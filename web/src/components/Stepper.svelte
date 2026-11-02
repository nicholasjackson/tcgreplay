<script>
  // A number set without a dialog: −step and +step around the value, any
  // quick additions, and the value itself, which a tap opens for typing.
  // children adds buttons of the caller's own (×2 for Weakness).
  let { value, step = 10, quick = [], min = 0, max = Infinity, label, hint = true, set, children } = $props();

  const clamp = (v) => Math.min(max, Math.max(min, v));
  function typed(ev) {
    const v = parseInt(ev.target.value, 10);
    if (Number.isFinite(v)) set(clamp(v));
    else ev.target.value = value;
  }
</script>

<div class="stepper">
  <button onclick={() => set(clamp(value - step))} disabled={value <= min}>−{step}</button>
  <label class="val">
    <input inputmode="numeric" {value} aria-label={label} onchange={typed} onkeydown={(ev) => ev.key === 'Enter' && ev.currentTarget.blur()} />
    {#if hint}<small>tap to type</small>{/if}
  </label>
  <button onclick={() => set(clamp(value + step))} disabled={value >= max}>+{step}</button>
  {#each quick as q (q)}
    <button onclick={() => set(clamp(value + q))} disabled={value >= max}>+{q}</button>
  {/each}
  {@render children?.()}
</div>

<style>
  .stepper { display: flex; align-items: center; gap: 8px; padding: 0 16px; flex-wrap: wrap; }
  .stepper :global(button) {
    min-height: 44px;
    padding: 0 14px;
    border-radius: 10px;
    font-weight: 600;
    font-size: 14px;
    white-space: nowrap;
    flex: none;
  }
  .val { display: flex; flex-direction: column; align-items: center; }
  input {
    width: 72px;
    min-height: 44px;
    border-radius: 10px;
    background: var(--input);
    border: 1px solid var(--line);
    color: var(--text);
    font: 800 18px system-ui, sans-serif;
    text-align: center;
  }
  small { font-size: 9px; font-weight: 600; color: var(--muted); margin-top: 2px; }
</style>
