<script>
  // One card: art from TCGdex when known, a named text card otherwise, and
  // a card back when hidden: the Studio back, navy with a cyan frame and
  // the TCG Replay mark.
  import { app, cardInfo, imageSrc, nextImageStage, pointerTypeOf } from '../lib/store.svelte.js';
  import { draggable as dragSource } from '../lib/pointer.svelte.js';
  import markUrl from '../../../internal/design/mark.svg';

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
    // label, if given, is the card's accessible name in place of its
    // name: the hand's "Boss's Orders, Supporter".
    label = '',
  } = $props();

  const info = $derived(card && !card.hidden ? cardInfo(card.def) : null);
  // Visible (e.g. in the reveal view) but never identified: the log never
  // named it and no list has been assumed for it.
  const unknown = $derived(card && !card.hidden && !card.name && !card.def);
  const lit = $derived(card && app.highlight.includes(card.iid));
  // The card an action is being played from fades while its target is chosen.
  const source = $derived(card && app.target?.card?.iid === card.iid);
  // Which address the art is loaded from (see imageSrc); reset per card.
  let stage = $state(0);
  $effect(() => {
    card?.def;
    stage = 0;
  });

  // energyType pulls the type out of names like "Basic Fighting Energy".
  function energyType(name) {
    const m = /(Grass|Fire|Water|Lightning|Psychic|Fighting|Darkness|Metal|Colorless)/i.exec(name || '');
    return m ? m[1][0].toUpperCase() + m[1].slice(1).toLowerCase() : 'Special';
  }

  // A tap opens the card's actions; a mouse click zooms it (or does what
  // the caller asks); a right-click opens the actions at the pointer.
  function click(ev) {
    ev.stopPropagation();
    if (onmenu && pointerTypeOf(ev) !== 'mouse') onmenu(ev);
    else if (onclick) onclick(ev);
    else if (!card.hidden) app.zoom = card;
  }
  function contextmenu(ev) {
    if (!onmenu) return;
    ev.preventDefault();
    ev.stopPropagation();
    // A long touch is not a gesture on the board.
    if (pointerTypeOf(ev) === 'touch') return;
    onmenu(ev, 'mouse');
  }
</script>

{#if card}
  <div
    class="card {size}"
    class:lit
    class:target-source={source}
    class:lift
    class:back={card.hidden}
    style:--rot="{rotate}deg"
    {@attach dragSource(draggable && !card.hidden && app.opponent !== 'acting' && { kind: 'card', iid: card.iid, def: card.def, from, role })}
    onclick={click}
    oncontextmenu={contextmenu}
    onkeydown={(ev) => ev.key === 'Enter' && click(ev)}
    role="button"
    tabindex="-1"
    aria-label={label || null}
    title={card.hidden ? (card.known ? `Known: ${card.known} (${card.iid})` : card.iid) : unknown ? `Unknown card (${card.iid}): never revealed` : `${card.name} (${card.iid})`}
  >
    {#if card.hidden}
      <div class="back-art" data-back="studio"><img class="mark" src={markUrl} alt="" draggable="false" /></div>
      <!-- A revealed card in another player's hand stays face down, marked. -->
      {#if card.known}<span class="known" aria-label="known: {card.known}">{card.known}</span>{/if}
    {:else if unknown}
      <div class="back-art unknown" data-back="studio"><img class="mark" src={markUrl} alt="" draggable="false" /><span>?</span></div>
    {:else if info?.category === 'Energy' && !imageSrc(info, 'low.webp', stage)}
      <div class="token" data-type={energyType(info.name)} title={info.name}>{energyType(info.name).slice(0, 1)}</div>
    {:else if imageSrc(info, 'low.webp', stage)}
      <img src={imageSrc(info, 'low.webp', stage)} alt={card.name} loading="lazy" draggable="false" onerror={() => (stage = nextImageStage(info, stage))} />
    {:else}
      <div class="text-card" data-cat={info?.category ?? ''}>
        <strong>{card.name || card.iid}</strong>
        {#if info?.hp}<span class="hp">HP {info.hp}</span>{/if}
        <small>{info?.stage || info?.trainer_type || info?.energy_type || ''}</small>
      </div>
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
    box-shadow: var(--shadow-card);
    cursor: grab;
    background: var(--mat-zone);
    outline: 3px solid transparent;
    /* A long touch selects nothing and opens no callout. */
    user-select: none;
    -webkit-user-select: none;
    -webkit-touch-callout: none;
  }
  .card.sm { --w: calc(var(--cw) * 0.62); }
  .card.xs { --w: calc(var(--cw) * 0.42); }
  .card.lg { --w: calc(var(--cw) * 1.25); }
  /* On a touch screen no card is smaller than a fingertip's 44px. */
  @media (pointer: coarse) {
    .card { --w: max(44px, var(--cw)); }
    .card.sm { --w: max(44px, calc(var(--cw) * 0.62)); }
    .card.xs { --w: max(44px, calc(var(--cw) * 0.42)); }
  }
  @media (hover: hover) {
    /* Rolling over a face-up card enlarges it to about two card widths,
       after a short pause so sweeping the mouse across the mat stays calm.
       Clicking still opens the full card view. Nothing enlarges during a
       drag, so an enlarged card never covers the zone under the pointer. */
    :global(body:not(.dragging)) .card.lift:not(.back):hover {
      --hs: 1.8;
      z-index: 40;
      box-shadow: var(--shadow-card-lift);
      transition-delay: 0.15s;
    }
    :global(body:not(.dragging)) .card.lift.lg:not(.back):hover { --hs: 1.55; }
    :global(body:not(.dragging)) .card.lift.sm:not(.back):hover { --hs: 2.8; }
    :global(body:not(.dragging)) .card.lift.xs:not(.back):hover { --hs: 4; }
  }
  .card.lit {
    outline-color: var(--accent);
    box-shadow: 0 0 18px 4px var(--highlight-glow);
  }
  img {
    width: 100%;
    height: 100%;
    border-radius: inherit;
    display: block;
    object-fit: cover;
    pointer-events: none;
  }
  .known {
    position: absolute;
    left: 50%;
    bottom: 6%;
    translate: -50% 0;
    max-width: 92%;
    padding: 0.05em 0.45em;
    border-radius: 1em;
    background: var(--primary);
    color: var(--on-primary);
    font: 700 max(var(--fs-9), calc(var(--w, var(--cw)) * 0.11)) var(--font-sans);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    pointer-events: none;
  }
  /* The Studio back: navy, framed in cyan inside a navy edge, with the
     mark in the middle (its own navy square melts into the back). */
  .back-art {
    width: 100%;
    height: 100%;
    border-radius: inherit;
    display: grid;
    place-items: center;
    background: var(--brand-navy);
    border: calc(var(--w) * 0.05) solid var(--brand-navy);
    box-shadow: inset 0 0 0 max(1px, calc(var(--w) * 0.025)) var(--brand-cyan);
    box-sizing: border-box;
    overflow: hidden;
  }
  .back-art > * { grid-area: 1 / 1; }
  .back-art .mark { width: 78%; height: auto; pointer-events: none; }
  /* Never identified: the back, dimmed, under a "?". */
  .back-art.unknown .mark { opacity: 0.35; }
  .back-art.unknown span {
    font: 800 calc(var(--w) * 0.45) / 1 var(--font-sans);
    color: var(--glyph-ink);
    text-shadow: var(--shadow-text-glyph);
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
    background: var(--card-face);
    color: var(--card-face-ink);
    font-size: calc(var(--w) * 0.12);
    line-height: 1.15;
    overflow: hidden;
    border: calc(var(--w) * 0.04) solid var(--card-border-pokemon);
  }
  .text-card[data-cat='Trainer'] { border-color: var(--card-border-trainer); background: var(--card-face-trainer); }
  .text-card[data-cat='Energy'] { border-color: var(--card-border-energy); }
  .text-card strong { font-weight: var(--fw-ui-title); }
  .text-card .hp { font-weight: 700; color: var(--card-hp); }
  .text-card small { opacity: 0.7; }
  .token {
    width: 100%;
    height: 100%;
    border-radius: inherit;
    display: grid;
    place-items: center;
    font: 800 calc(var(--w) * 0.5) / 1 var(--font-sans);
    color: var(--glyph-ink);
    text-shadow: var(--shadow-text-energy);
    background: radial-gradient(circle at 50% 45%, var(--ec) 0 38%, var(--energy-ring) 39% 44%, var(--ec) 45%);
    --ec: var(--energy-colorless);
  }
  .token[data-type='Fighting'] { --ec: var(--energy-fighting); }
  .token[data-type='Fire'] { --ec: var(--energy-fire); }
  .token[data-type='Water'] { --ec: var(--energy-water); }
  .token[data-type='Grass'] { --ec: var(--energy-grass); }
  .token[data-type='Lightning'] { --ec: var(--energy-lightning); }
  .token[data-type='Psychic'] { --ec: var(--energy-psychic); }
  .token[data-type='Darkness'] { --ec: var(--energy-darkness); }
  .token[data-type='Metal'] { --ec: var(--energy-metal); }
  .token[data-type='Colorless'] { --ec: var(--energy-colorless); }
</style>
