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
  .target-banner {
    position: fixed;
    top: 0;
    left: 0;
    right: 0;
    z-index: 35;
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 10px 12px;
    background: #2b2410;
    color: #f2ead0;
    border-bottom: 2px solid var(--accent);
    box-shadow: 0 4px 16px #0008;
  }
  .target-banner :global(.card) { --cw: 34px; filter: none; opacity: 1; }
  .text { display: flex; flex-direction: column; flex: 1; min-width: 0; font-size: 13px; line-height: 1.3; }
  .hint { color: var(--accent); font-weight: 700; }
  button { min-height: 44px; padding: 0 16px; }
</style>
