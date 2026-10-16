<script>
  // Zoomed card with its full TCGdex text.
  import { app, cardInfo } from '../lib/store.svelte.js';

  const card = $derived(app.zoom);
  const info = $derived(card ? cardInfo(card.def) : null);
  let broken = $state(false);
  $effect(() => {
    card;
    broken = false;
  });
</script>

{#if card}
  <div class="backdrop" onclick={() => (app.zoom = null)} role="presentation">
    <div class="zoom" role="dialog" aria-label={card.name}>
      {#if info?.image && !broken}
        <img src="{info.image}/high.webp" alt={card.name} onerror={() => (broken = true)} />
      {/if}
      <div class="text">
        <h2>{card.name || card.iid}</h2>
        <p class="meta">{card.iid}{#if card.def} · {card.def}{/if}{#if card.basis} · assumed ({card.basis}){/if}</p>
        {#if info}
          <pre>{info.text}</pre>
        {:else if card.def}
          <p class="meta">Loading card text…</p>
        {:else}
          <p class="meta">No card definition (hidden or unresolved).</p>
        {/if}
      </div>
    </div>
  </div>
{/if}

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    background: #000b;
    z-index: 50;
    display: grid;
    place-items: center;
    padding: 16px;
  }
  .zoom {
    display: flex;
    gap: 20px;
    max-width: min(900px, 100%);
    max-height: 92vh;
    background: var(--panel);
    border-radius: 14px;
    padding: 16px;
    overflow: auto;
    flex-wrap: wrap;
  }
  img { width: min(360px, 80vw); border-radius: 14px; align-self: flex-start; }
  .text { flex: 1; min-width: 240px; }
  h2 { margin: 0 0 4px; }
  .meta { color: var(--muted); font-size: 12px; margin: 0 0 10px; }
  pre { white-space: pre-wrap; font: 14px/1.5 system-ui, sans-serif; margin: 0; }
</style>
