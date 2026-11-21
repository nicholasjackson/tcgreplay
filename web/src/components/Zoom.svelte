<script>
  // Zoomed card with its full TCGdex text.
  import { app, cardInfo, imageSrc, nextImageStage } from '../lib/store.svelte.js';

  const card = $derived(app.zoom);
  const info = $derived(card ? cardInfo(card.def) : null);
  // Which address the art is loaded from (see imageSrc); reset per card.
  let stage = $state(0);
  // pair is both halves of the Stadium pair the zoomed card belongs to, in
  // print order (the left half of the joined picture first), else null:
  // the zoom then shows the whole picture, upright, with its one text.
  const pair = $derived.by(() => {
    const s = app.board?.stadium;
    if (!card || !s?.pair || (card.iid !== s.iid && card.iid !== s.pair.iid)) return null;
    return [s, s.pair].sort((x, y) => (x.def || '').localeCompare(y.def || '', undefined, { numeric: true }));
  });
  let stages = $state([0, 0]);
  $effect(() => {
    card;
    stage = 0;
    stages = [0, 0];
  });
</script>

{#if card}
  <div class="backdrop" onclick={() => (app.zoom = null)} role="presentation">
    <div class="zoom" class:pair role="dialog" aria-label={card.name}>
      {#if pair}
        <div class="joined">
          {#each pair as half, i (half.iid)}
            {@const hi = cardInfo(half.def)}
            {#if imageSrc(hi, 'high.webp', stages[i])}
              <img src={imageSrc(hi, 'high.webp', stages[i])} alt="{half.name} ({half.iid})" onerror={() => (stages[i] = nextImageStage(hi, stages[i]))} />
            {/if}
          {/each}
        </div>
      {:else if imageSrc(info, 'high.webp', stage)}
        <img src={imageSrc(info, 'high.webp', stage)} alt={card.name} onerror={() => (stage = nextImageStage(info, stage))} />
      {/if}
      <div class="text">
        <h2>{card.name || 'Unknown card'}</h2>
        {#if pair}
          <p class="meta id">Played as a pair: {pair.map((h) => `${h.iid}${h.def ? ` · ${h.def}` : ''}`).join(' + ')}</p>
        {:else}
          <p class="meta id">{[card.iid, card.def, card.basis && `assumed (${card.basis})`].filter(Boolean).join(' · ')}</p>
        {/if}
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
    background: var(--scrim);
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
    background: var(--panel-raised);
    border-radius: var(--radius-xl);
    box-shadow: var(--shadow-dialog);
    color: var(--ink);
    font-family: var(--font-sans);
    padding: clamp(16px, 2vh, 32px);
    overflow: auto;
    flex-wrap: wrap;
    font-size: clamp(var(--fs-14), 1.5vh, var(--fs-24));
  }
  img { width: var(--zw); max-width: 100%; border-radius: calc(var(--zw) * 0.045); align-self: flex-start; }
  /* A pair's halves side by side, edge to edge, as one picture: as wide as
     the screen allows on a phone, never past 1.6 single cards elsewhere. */
  .joined { display: flex; width: min(calc(var(--zw) * 1.6), 100%); align-self: flex-start; }
  .joined img { width: 50%; border-radius: 0; }
  .joined img:first-child { border-radius: calc(var(--zw) * 0.036) 0 0 calc(var(--zw) * 0.036); }
  .joined img:last-child { border-radius: 0 calc(var(--zw) * 0.036) calc(var(--zw) * 0.036) 0; }
  .zoom.pair { max-width: min(96vw, calc(var(--zw) * 3)); }
  .text { flex: 1; min-width: min(18em, 100%); max-width: 40em; }
  h2 { margin: 0 0 0.2em; font-size: 1.6em; font-weight: 600; line-height: 1.2; }
  .meta { color: var(--muted); font-size: 0.85em; margin: 0 0 0.8em; }
  .id { font-family: var(--font-mono); font-weight: 500; }
  pre { white-space: pre-wrap; font: 1em/1.5 var(--font-sans); margin: 0; }
</style>
