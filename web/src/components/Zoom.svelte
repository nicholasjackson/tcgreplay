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
        <h2>{card.name || 'Unknown card'}</h2>
        <p class="meta">{card.iid}{#if card.def} · {card.def}{/if}{#if card.basis} · assumed ({card.basis}){/if}</p>
        {#if info}
          <pre>{info.text}</pre>
        {:else if card.def}
          <p class="meta">Loading card text…</p>
        {:else if !card.name}
          <p class="meta">Unknown card: the battle log never revealed it. Ask {app.assistant} to assume an opponent list (sample_hidden) to give it a plausible identity.</p>
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
    padding: 2vh 2vw;
    /* Art as tall as the screen allows, beside the text; TCGdex's large
       images are 600px wide, so past ~760px they only blur. */
    --zw: clamp(240px, min(calc(84vh * 63 / 88), 44vw), 760px);
  }
  .zoom {
    display: flex;
    gap: clamp(16px, 2.5vh, 40px);
    max-width: min(96vw, calc(var(--zw) * 2.4));
    max-height: 94vh;
    background: var(--panel);
    border-radius: 14px;
    padding: clamp(16px, 2vh, 32px);
    overflow: auto;
    flex-wrap: wrap;
    font-size: clamp(14px, 1.5vh, 24px);
  }
  img { width: var(--zw); max-width: 100%; border-radius: calc(var(--zw) * 0.045); align-self: flex-start; }
  .text { flex: 1; min-width: min(18em, 100%); max-width: 40em; }
  h2 { margin: 0 0 0.2em; font-size: 1.6em; }
  .meta { color: var(--muted); font-size: 0.85em; margin: 0 0 0.8em; }
  pre { white-space: pre-wrap; font: 1em/1.5 system-ui, sans-serif; margin: 0; }
</style>
