<script>
  // One card: art from TCGdex when known, a named text card otherwise, and
  // a card back when hidden.
  import { app, cardInfo } from '../lib/store.svelte.js';
  import { startDrag, endDrag } from '../lib/actions.js';

  let {
    card,
    from = '',
    role = '',
    size = 'md',
    draggable = true,
    rotate = 0,
    lift = true,
    onmenu = null,
    onclick = null,
  } = $props();

  const info = $derived(card && !card.hidden ? cardInfo(card.def) : null);
  // Visible (e.g. in the reveal view) but never identified: the log never
  // named it and no list has been assumed for it.
  const unknown = $derived(card && !card.hidden && !card.name && !card.def);
  const lit = $derived(card && app.highlight.includes(card.iid));
  let broken = $state(false);

  // energyType pulls the type out of names like "Basic Fighting Energy".
  function energyType(name) {
    const m = /(Grass|Fire|Water|Lightning|Psychic|Fighting|Darkness|Metal)/i.exec(name || '');
    return m ? m[1][0].toUpperCase() + m[1].slice(1).toLowerCase() : 'Special';
  }

  function dragstart(ev) {
    startDrag(ev, { kind: 'card', iid: card.iid, def: card.def, from, role });
  }
  function click(ev) {
    ev.stopPropagation();
    if (onclick) onclick(ev);
    else if (!card.hidden) app.zoom = card;
  }
  function contextmenu(ev) {
    if (!onmenu) return;
    ev.preventDefault();
    ev.stopPropagation();
    onmenu(ev);
  }
</script>

{#if card}
  <div
    class="card {size}"
    class:lit
    class:lift
    class:back={card.hidden}
    style:--rot="{rotate}deg"
    draggable={draggable && !card.hidden}
    ondragstart={dragstart}
    ondragend={endDrag}
    onclick={click}
    oncontextmenu={contextmenu}
    onkeydown={(ev) => ev.key === 'Enter' && click(ev)}
    role="button"
    tabindex="-1"
    title={card.hidden ? card.iid : unknown ? `Unknown card (${card.iid}): never revealed` : `${card.name} (${card.iid})`}
  >
    {#if card.hidden}
      <div class="back-art"><span></span></div>
    {:else if unknown}
      <div class="back-art unknown"><span>?</span></div>
    {:else if info?.category === 'Energy' && (!info.image || broken)}
      <div class="token" data-type={energyType(info.name)} title={info.name}>{energyType(info.name).slice(0, 1)}</div>
    {:else if info?.image && !broken}
      <img src="{info.image}/low.webp" alt={card.name} loading="lazy" draggable="false" onerror={() => (broken = true)} />
    {:else}
      <div class="text-card" data-cat={info?.category ?? ''}>
        <strong>{card.name || card.iid}</strong>
        {#if info?.hp}<span class="hp">HP {info.hp}</span>{/if}
        <small>{info?.stage || info?.trainer_type || info?.energy_type || ''}</small>
      </div>
    {/if}
    {#if card.basis && card.basis !== 'decklist'}
      <span class="basis" title="assumed: {card.basis}">{card.basis[0]}</span>
    {/if}
  </div>
{/if}

<style>
  .card {
    --w: var(--cw);
    width: var(--w);
    aspect-ratio: 63 / 88;
    border-radius: calc(var(--w) * 0.05);
    position: relative;
    flex: none;
    transform: rotate(var(--rot)) scale(var(--hs, 1));
    transition: transform 0.25s ease, box-shadow 0.25s ease, outline-color 0.25s;
    box-shadow: 0 2px 6px #0007;
    cursor: grab;
    background: var(--card-bg);
    outline: 3px solid transparent;
    user-select: none;
  }
  .card.sm { --w: calc(var(--cw) * 0.62); }
  .card.xs { --w: calc(var(--cw) * 0.42); }
  .card.lg { --w: calc(var(--cw) * 1.25); }
  /* Rolling over a face-up card enlarges it to about two card widths,
     after a short pause so sweeping the mouse across the mat stays calm.
     Clicking still opens the full card view. */
  .card.lift:not(.back):hover {
    --hs: 1.8;
    z-index: 40;
    box-shadow: 0 10px 30px #000b;
    transition-delay: 0.15s;
  }
  .card.lift.lg:not(.back):hover { --hs: 1.55; }
  .card.lift.sm:not(.back):hover { --hs: 2.8; }
  .card.lift.xs:not(.back):hover { --hs: 4; }
  .card.lit {
    outline-color: var(--accent);
    box-shadow: 0 0 18px 4px var(--accent-glow);
  }
  img {
    width: 100%;
    height: 100%;
    border-radius: inherit;
    display: block;
    object-fit: cover;
    pointer-events: none;
  }
  .back-art {
    width: 100%;
    height: 100%;
    border-radius: inherit;
    background: radial-gradient(circle at 50% 50%, #e9eef3 0 13%, #232a33 13% 17%, transparent 17%),
      linear-gradient(#c8312b 0 47%, #232a33 47% 53%, #2a5bd7 53%);
    border: calc(var(--w) * 0.05) solid #1d3f9e;
    box-sizing: border-box;
  }
  .back-art.unknown {
    filter: grayscale(0.85) brightness(0.7);
    display: grid;
    place-items: center;
  }
  .back-art.unknown span {
    font: 800 calc(var(--w) * 0.45) / 1 system-ui;
    color: #fff;
    text-shadow: 0 2px 6px #000;
    filter: none;
  }
  .text-card {
    width: 100%;
    height: 100%;
    border-radius: inherit;
    box-sizing: border-box;
    padding: 6%;
    display: flex;
    flex-direction: column;
    gap: 4%;
    background: linear-gradient(160deg, #f4efe0, #d9d0b5);
    color: #222;
    font-size: calc(var(--w) * 0.12);
    line-height: 1.15;
    overflow: hidden;
    border: calc(var(--w) * 0.04) solid #c9a43c;
  }
  .text-card[data-cat='Trainer'] { border-color: #8a8f99; background: linear-gradient(160deg, #eef1f4, #cfd5dc); }
  .text-card[data-cat='Energy'] { border-color: #b8742a; }
  .text-card .hp { font-weight: 700; color: #a22; }
  .text-card small { opacity: 0.7; }
  .token {
    width: 100%;
    height: 100%;
    border-radius: inherit;
    display: grid;
    place-items: center;
    font: 800 calc(var(--w) * 0.5) / 1 system-ui;
    color: #fff;
    text-shadow: 0 1px 2px #0009;
    background: radial-gradient(circle at 50% 45%, var(--ec) 0 38%, #fff 39% 44%, var(--ec) 45%);
    --ec: #888;
  }
  .token[data-type='Fighting'] { --ec: #b5562b; }
  .token[data-type='Fire'] { --ec: #e2452b; }
  .token[data-type='Water'] { --ec: #2f86d6; }
  .token[data-type='Grass'] { --ec: #3f9a3a; }
  .token[data-type='Lightning'] { --ec: #e7b322; }
  .token[data-type='Psychic'] { --ec: #8c4bb5; }
  .token[data-type='Darkness'] { --ec: #2d3b45; }
  .token[data-type='Metal'] { --ec: #8b98a3; }
  .basis {
    position: absolute;
    top: 2px;
    left: 2px;
    background: #7a3fd0;
    color: white;
    font: 700 max(10px, calc(var(--w) * 0.11))/1 system-ui;
    padding: 0.2em 0.4em;
    border-radius: 3px;
    text-transform: uppercase;
  }
</style>
