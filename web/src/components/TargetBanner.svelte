<script>
  // The banner across the top while an action waits for its target: the
  // card, what is being done and Cancel (lib/target.js).
  import { app } from '../lib/store.svelte.js';
  import { endTarget } from '../lib/target.js';
  import Card from './Card.svelte';

  const t = $derived(app.target);
</script>

{#if t}
  <div class="target-banner" role="region" aria-label="Choose a target" aria-live="polite">
    {#if t.card}<Card card={t.card} size="xs" draggable={false} lift={false} />{/if}
    <div class="text">
      <strong>{t.prompt}</strong>
      <span class="hint">{t.hint}</span>
    </div>
    <button onclick={endTarget}>Cancel</button>
  </div>
{/if}

<style>
  /* A raised bar at the top centre, edged in the primary colour: the card,
     the action and what to tap, and Cancel. .target-banner is what
     lib/target.js lets a click through to. */
  .target-banner {
    position: fixed;
    top: var(--space-2);
    left: 50%;
    translate: -50% 0;
    width: min(560px, calc(100vw - 2 * var(--space-2)));
    z-index: 35;
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-2) var(--space-2) var(--space-2) var(--space-3);
    border-radius: var(--radius-l);
    border-left: 4px solid var(--primary);
    background: var(--panel-raised);
    color: var(--ink);
    font-family: var(--font-sans);
    box-shadow: var(--shadow-menu);
  }
  .target-banner :global(.card) { --cw: 34px; filter: none; opacity: 1; }
  .text { display: flex; flex-direction: column; gap: 2px; flex: 1; min-width: 0; line-height: 1.3; }
  strong { font: var(--fw-ui-title) var(--fs-ui-title)/var(--lh-ui-title) var(--font-sans); }
  .hint { font-size: var(--fs-13); color: var(--ink-soft); }
  button {
    flex: none;
    min-height: var(--touch-target);
    padding: 0 var(--space-4);
    border-radius: var(--radius-m);
    border: 1px solid var(--control-line);
    background: var(--panel-raised);
    color: var(--ink);
    font: 600 var(--fs-14) var(--font-sans);
  }
  button:hover:not(:disabled) { border-color: var(--control-line); background: var(--hover); }
  button:focus-visible { outline: 2px solid var(--primary); outline-offset: 1px; }
</style>
