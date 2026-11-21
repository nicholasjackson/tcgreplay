<script>
  // Zoomed card with its full TCGdex text. A card attached to a Pokémon
  // (a Tool, a Special or Basic Energy) also offers its actions at the
  // foot, Move to, To hand and Discard: the board draws a Basic Energy as
  // a dot, so this view, opened from its Pokémon's Attached list, is how it
  // is moved.
  import { app, cardInfo, imageSrc, nextImageStage } from '../lib/store.svelte.js';
  import { attachmentOf, attachmentMenu } from '../lib/actions.js';
  import { startTarget } from '../lib/target.js';
  import Icon from './Icon.svelte';

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
  const attached = $derived(card ? attachmentOf(card) : null);
  const actions = $derived(attached ? attachmentMenu(card, attached.role).sections.flatMap((sec) => sec.items || []) : []);
  const close = () => (app.zoom = null);
  // act runs an attachment action: the view closes, then the action runs or
  // waits for its target on the board.
  function act(ev, item) {
    ev.stopPropagation();
    close();
    if (item.target) startTarget(item.target);
    else item.run();
  }
  $effect(() => {
    card;
    stage = 0;
    stages = [0, 0];
  });
</script>

{#if card}
  <div class="backdrop" onclick={close} role="presentation">
    <div class="zoom" class:pair class:attached={!!attached} role="dialog" aria-label={card.name}>
      {#if attached}
        <!-- An attached card's view has actions, so it is a modal with a
             header: its name, and the close button in the top-right
             corner, above the card on a phone as beside it elsewhere. -->
        <div class="bar">
          <h2>{card.name || 'Unknown card'}</h2>
          <button class="close" aria-label="Close" onclick={(ev) => { ev.stopPropagation(); close(); }}><Icon name="close" /></button>
        </div>
      {/if}
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
        {#if !attached}<h2>{card.name || 'Unknown card'}</h2>{/if}
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
        {#if attached}
          <section class="foot" role="group" aria-label="Attached">
            <div class="label">Attached</div>
            <p class="where">{attached.where}</p>
            <div class="acts">
              {#each actions as item, i (item.label)}
                <button class:primary={i === 0} onclick={(ev) => act(ev, item)}>
                  <span>{item.label}</span>{#if item.target}<Icon name="target" />{/if}
                </button>
              {/each}
            </div>
          </section>
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
  .bar { flex-basis: 100%; display: flex; align-items: flex-start; gap: var(--space-3); }
  .bar h2 { flex: 1; min-width: 0; margin: 0.2em 0 0; }
  .close {
    flex: none;
    display: grid;
    place-items: center;
    width: var(--touch-target);
    height: var(--touch-target);
    padding: 0;
    border-radius: var(--radius-m);
    border: 1px solid var(--control-line);
    background: var(--panel-raised);
    color: var(--ink);
    font-size: var(--fs-18);
  }
  /* An attached card's actions, at the foot of its text. */
  .zoom.attached .text { display: flex; flex-direction: column; }
  .foot { margin-top: auto; padding-top: var(--space-4); border-top: 1px solid var(--line); display: flex; flex-direction: column; gap: var(--space-2); }
  .zoom.attached pre { margin-bottom: var(--space-4); }
  .label { font: var(--fw-label) var(--fs-label)/var(--lh-label) var(--font-sans); letter-spacing: var(--ls-label); text-transform: uppercase; color: var(--muted); }
  .where { margin: 0; font: 500 var(--fs-13) var(--font-mono); color: var(--muted); }
  .acts { display: grid; grid-template-columns: repeat(auto-fit, minmax(7em, 1fr)); gap: var(--space-2); }
  .acts button {
    min-height: var(--touch-target);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 6px;
    padding: var(--space-1) var(--space-3);
    border-radius: var(--radius-m);
    border: 1px solid var(--control-line);
    background: var(--panel-raised);
    color: var(--ink);
    font: 600 var(--fs-14) var(--font-sans);
  }
  .acts button:hover:not(:disabled) { background: var(--hover); border-color: var(--control-line); }
  .acts button.primary { background: var(--primary); border-color: var(--primary); color: var(--on-primary); }
  .acts button.primary:hover:not(:disabled) { background: var(--primary); }
</style>
