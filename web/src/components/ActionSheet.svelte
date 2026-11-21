<script>
  // A card's actions, from a menu model (lib/actions.js), in one of three
  // containers: the full-screen phone sheet, the tablet panel with the card
  // beside its actions, or the desktop popover at the pointer. The model is
  // rebuilt as the board changes, so the damage and conditions shown stay
  // current while the sheet is open.
  import { app, cardInfo, imageSrc, nextImageStage } from '../lib/store.svelte.js';
  import { startTarget } from '../lib/target.js';
  import { energyType } from '../lib/actions.js';
  import Card from './Card.svelte';
  import Stepper from './Stepper.svelte';
  import Icon from './Icon.svelte';

  const s = $derived(app.sheet);
  const model = $derived(s ? s.build() : null);
  const mode = $derived(!s ? '' : s.pointerType === 'mouse' ? 'popover' : app.layout === 'phone' ? 'phone' : 'tablet');
  const info = $derived(model?.card ? cardInfo(model.card.def) : null);
  // showText swaps the actions (phone) or the card's art (tablet) for its text.
  let showText = $state(false);
  // panel is an item's sub-panel ({component, props}) shown in place of
  // the sections: the attack panel, or Draw N and Look at top N.
  let panel = $state(null);
  let el = $state();
  let pos = $state({ x: 0, y: 0 });
  // thumbStages is each row thumbnail's image stage (see imageSrc).
  let thumbStages = $state({});
  // pushed is set while the sheet has a history entry for Back to close.
  let pushed = false;

  $effect(() => {
    s;
    showText = false;
    panel = null;
  });
  // The card left play (a knock out, say): nothing is left to act on.
  $effect(() => {
    if (s && !model) close();
  });
  $effect(() => {
    if (s && mode !== 'popover' && !pushed) {
      history.pushState({ sheet: true }, '');
      pushed = true;
    }
  });
  $effect(() => {
    if (mode !== 'popover' || !el) return;
    const r = el.getBoundingClientRect();
    pos = { x: Math.max(8, Math.min(s.x, innerWidth - r.width - 8)), y: Math.max(8, Math.min(s.y, innerHeight - r.height - 8)) };
  });

  function close() {
    app.sheet = null;
    if (pushed) {
      pushed = false;
      history.back();
    }
  }
  function popstate() {
    if (!pushed) return;
    pushed = false;
    app.sheet = null;
  }
  // choose runs an item, or for one that needs a destination closes the
  // sheet and leaves the choice to a tap on the board.
  function choose(item) {
    if (item.panel) {
      panel = item.panel;
      return;
    }
    // Another card's menu (an attachment's, from its Pokémon's) takes the
    // sheet's place, keeping its history entry for Back to close.
    if (item.menu) {
      app.sheet = { ...s, build: item.menu };
      return;
    }
    close();
    if (item.target) startTarget(item.target);
    else item.run();
  }

  // A swipe down on the phone sheet's header closes it.
  let y0 = null;
  function swipeEnd(ev) {
    if (y0 !== null && ev.clientY - y0 > 60) close();
    y0 = null;
  }
</script>


<svelte:window onkeydown={(ev) => ev.key === 'Escape' && s && !s.hidden && close()} onpopstate={popstate} />

{#snippet text()}
  <pre class="text">{model.text ?? info?.text ?? (model.card.def ? 'Loading card text…' : 'No card text.')}</pre>
{/snippet}

<!-- A row's picture of its card (the Attached list): the card's small
     image, or its Energy colour or initial while it has none. Not a .card:
     it is part of its row's button, which is the target. -->
{#snippet thumb(c)}
  {@const ti = cardInfo(c.def)}
  <span class="thumb" aria-hidden="true">
    {#if imageSrc(ti, 'low.webp', thumbStages[c.iid] ?? 0)}
      <img src={imageSrc(ti, 'low.webp', thumbStages[c.iid] ?? 0)} alt="" draggable="false" onerror={() => (thumbStages[c.iid] = nextImageStage(ti, thumbStages[c.iid] ?? 0))} />
    {:else if ti?.category === 'Energy' || /energy/i.test(c.name || '')}
      <span class="tok" data-e={energyType(c)}>{energyType(c).slice(0, 1).toUpperCase()}</span>
    {:else}
      <span class="tok tile">{(c.name || '?').slice(0, 1)}</span>
    {/if}
  </span>
{/snippet}

{#snippet back()}
  <button class="back" onclick={() => (panel = null)}>‹ Actions</button>
{/snippet}

{#if model}
  <div class="backdrop {mode}" hidden={s.hidden} onclick={close} oncontextmenu={(ev) => { ev.preventDefault(); close(); }} role="presentation"></div>
  <div
    class="sheet {mode}"
    bind:this={el}
    style:left={mode === 'popover' ? `${pos.x}px` : null}
    style:top={mode === 'popover' ? `${pos.y}px` : null}
    role="dialog"
    aria-modal="true"
    aria-label={model.title}
    data-mode={mode}
    hidden={s.hidden}
  >
    {#if mode === 'tablet' && model.card}
      <div class="side">
        {#if showText}
          {@render text()}
        {:else}
          <div class="art"><Card card={model.card} draggable={false} lift={false} onclick={() => (showText = true)} /></div>
        {/if}
        <div class="seg" role="group" aria-label="Show the card or its text">
          <button class:on={!showText} aria-pressed={!showText} onclick={() => (showText = false)}>Card</button>
          <button class:on={showText} aria-pressed={showText} onclick={() => (showText = true)}>Text</button>
        </div>
      </div>
    {/if}
    <div class="main">
      {#if mode === 'popover'}
        <div class="phead">
          <div class="ptitle">{model.title}</div>
          {#if model.chain}<div class="chain">{model.chain}</div>{/if}
          {#if model.summary}<div class="psum">{model.summary}</div>{/if}
        </div>
      {:else}
        <header class:sub={!!panel} onpointerdown={(ev) => (y0 = ev.clientY)} onpointerup={swipeEnd} onpointercancel={() => (y0 = null)}>
          {#if mode === 'phone'}<span class="grab"></span>{/if}
          {#if panel}
            <!-- A panel's header: back to the actions, then the card it
                 belongs to (on a phone, where the card is not beside it). -->
            {@render back()}
            <span class="ctx">{mode === 'phone' ? model.title : ''}</span>
          {:else}
            {#if mode === 'phone' && model.card}
              <div class="art"><Card card={model.card} draggable={false} lift={false} onclick={() => (showText = !showText)} /></div>
            {/if}
            <div class="meta">
              <h3>{model.title}</h3>
              {#if model.chain}<div class="chain">{model.chain}</div>{/if}
              <div class="facts">
                <!-- Each fact stays on one line; the line wraps between them. -->
                {#if model.summary}<span class="sum">{#each model.summary.split(' · ') as part, i (i)}{i ? ' · ' : ''}<span class="part">{part}</span>{/each}</span>{/if}
                {#if model.damage || (mode === 'phone' && model.energy?.length)}
                  <span class="tags">
                    {#if model.damage}<span class="tag" aria-label="{model.damage} damage">−{model.damage}</span>{/if}
                    {#if mode === 'phone'}{#each model.energy || [] as e, i (i)}<i class="dot" class:special={e.special} data-e={e.type}></i>{/each}{/if}
                  </span>
                {/if}
              </div>
              {#if mode === 'phone' && model.card}
                <button class="linkish" aria-expanded={showText} onclick={() => (showText = !showText)}>{showText ? 'Actions ›' : 'Card text ›'}</button>
              {/if}
            </div>
          {/if}
          <button class="close" aria-label="Close" onclick={close}><Icon name="close" /></button>
        </header>
      {/if}
      <div class="body">
        {#if showText && mode === 'phone'}
          {@render text()}
        {:else if panel}
          <!-- A panel can close the sheet as it acts, which clears panel
               while it is still on screen. -->
          {@const Panel = panel?.component}
          {#if mode === 'popover'}{@render back()}{/if}
          <Panel {...panel?.props} {close} />
        {:else}
          {#each model.sections as sec (sec.label)}
            <section class="sec" class:other={sec.label === 'Other'} role="group" aria-label={sec.label}>
              <div class="label">{sec.label}</div>
              {#if sec.stepper}
                <Stepper {...sec.stepper} label="{sec.label} total" />
              {:else if sec.toggles}
                <div class="row">
                  {#each sec.toggles as t (t.label)}
                    <button class="tog" class:on={t.on} aria-pressed={t.on} onclick={() => t.set(!t.on)}>{t.label}</button>
                  {/each}
                </div>
              {:else}
                <div class="row" class:grid={sec.grid} class:list={sec.list}>
                  {#each sec.items as item (item.key ?? item.label)}
                    <!-- The label stays the first child: the cost, icon,
                         damage and chevron are placed around it by order. -->
                    <button class="item" class:primary={item.primary} class:danger={item.danger} class:opens={!!(item.panel || item.menu || item.opens)} class:thumbed={!!item.thumb} onclick={() => choose(item)}>
                      <span class="lbl">{item.label}</span>
                      {#if item.detail}<span class="detail">{item.detail}</span>{/if}
                      {#if item.cost?.length}<span class="cost" aria-hidden="true">{#each item.cost as c, i (i)}<i class="dot" data-e={c.toLowerCase()}></i>{/each}</span>{/if}
                      {#if item.icon}<span class="ico" aria-hidden="true"><Icon name={item.icon} /></span>{/if}
                      {#if item.thumb}{@render thumb(item.thumb)}{/if}
                      {#if item.target}<span class="go" aria-hidden="true"><Icon name="target" /></span>{/if}
                      {#if (item.panel && mode === 'popover') || item.menu || item.opens}<span class="chev" aria-hidden="true"><Icon name="chevron" /></span>{/if}
                    </button>
                  {/each}
                </div>
              {/if}
            </section>
          {/each}
        {/if}
      </div>
    </div>
  </div>
{/if}

<style>
  .backdrop { position: fixed; inset: 0; z-index: 40; }
  .backdrop.tablet { background: var(--scrim); }
  .sheet {
    position: fixed;
    z-index: 41;
    background: var(--panel-raised);
    color: var(--ink);
    font-family: var(--font-sans);
    display: flex;
  }
  /* Hidden while the attack panel's target is chosen on the board. */
  .sheet[hidden],
  .backdrop[hidden] { display: none; }
  .main { display: flex; flex-direction: column; flex: 1; min-width: 0; min-height: 0; }
  .body { flex: 1; min-height: 0; overflow-y: auto; }
  button:focus-visible { outline: 2px solid var(--primary); outline-offset: 1px; }

  /* Groups: an uppercase label over a row of buttons. */
  .label {
    font: var(--fw-label) var(--fs-label)/var(--lh-label) var(--font-sans);
    letter-spacing: var(--ls-label);
    text-transform: uppercase;
    color: var(--muted);
    padding: 0 var(--space-4) 6px;
  }
  .sec { padding: 10px 0 4px; }
  .row { display: flex; gap: var(--space-2); padding: 0 var(--space-4); }

  /* The touch buttons: 44px, outlined; the primary one filled. */
  .item,
  .tog {
    min-height: var(--touch-target);
    padding: 0 var(--space-4);
    border-radius: var(--radius-m);
    border: 1px solid var(--control-line);
    background: var(--panel-raised);
    color: var(--ink);
    font: 500 var(--fs-14) var(--font-sans);
    white-space: nowrap;
    flex: none;
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
  }
  .item:hover:not(:disabled),
  .tog:hover:not(:disabled) { border-color: var(--control-line); background: var(--hover); }
  .lbl { text-align: left; }
  .detail { font: 600 var(--fs-14) var(--font-mono); color: var(--muted); margin-left: auto; }
  .cost { order: -1; display: flex; gap: 3px; }
  .ico { order: -1; display: flex; }
  .go,
  .chev { display: flex; color: var(--muted); margin-left: auto; }
  .detail ~ .chev { margin-left: 0; }
  .dot { display: block; width: 12px; height: 12px; border-radius: 50%; background: var(--energy-colorless); }
  .dot[data-e='grass'] { background: var(--energy-grass); }
  .dot[data-e='fire'] { background: var(--energy-fire); }
  .dot[data-e='water'] { background: var(--energy-water); }
  .dot[data-e='lightning'] { background: var(--energy-lightning); }
  .dot[data-e='psychic'] { background: var(--energy-psychic); }
  .dot[data-e='fighting'] { background: var(--energy-fighting); }
  .dot[data-e='darkness'] { background: var(--energy-darkness); }
  .dot[data-e='metal'] { background: var(--energy-metal); }
  /* Energy of every type (Rainbow Energy) in every type's colour; a
     Special Energy's dots ringed, as on the board. */
  .dot[data-e='any'] { background: conic-gradient(var(--energy-grass), var(--energy-fire), var(--energy-lightning), var(--energy-fighting), var(--energy-psychic), var(--energy-water), var(--energy-darkness), var(--energy-metal), var(--energy-grass)); }
  .dot.special { outline: 1.5px solid var(--ink); outline-offset: 1px; margin: 0 1.5px; }
  /* The Attached list: one full-width row per card, its picture, name,
     kind and a chevron; the picture before the name by order. */
  .row.list { flex-direction: column; }
  .row.list .item { justify-content: flex-start; min-height: 52px; width: 100%; box-sizing: border-box; padding: var(--space-1) var(--space-3); gap: var(--space-3); white-space: normal; text-align: left; }
  .row.list .lbl { flex: 1; min-width: 0; }
  .row.list .detail { font: 500 var(--fs-13) var(--font-sans); margin-left: 0; white-space: nowrap; }
  .thumb { order: -1; flex: none; display: block; height: 40px; aspect-ratio: 63 / 88; border-radius: 3px; overflow: hidden; background: var(--panel-sunk); }
  .thumb img { width: 100%; height: 100%; object-fit: cover; display: block; }
  .tok { width: 100%; height: 100%; display: grid; place-items: center; font: 800 var(--fs-13) var(--font-sans); color: var(--glyph-ink); background: var(--energy-colorless); }
  .tok.tile { background: var(--card-face-trainer); color: var(--card-face-ink); }
  .tok[data-e='grass'] { background: var(--energy-grass); }
  .tok[data-e='fire'] { background: var(--energy-fire); }
  .tok[data-e='water'] { background: var(--energy-water); }
  .tok[data-e='lightning'] { background: var(--energy-lightning); }
  .tok[data-e='psychic'] { background: var(--energy-psychic); }
  .tok[data-e='fighting'] { background: var(--energy-fighting); }
  .tok[data-e='darkness'] { background: var(--energy-darkness); }
  .tok[data-e='metal'] { background: var(--energy-metal); }
  .item.primary { background: var(--primary); border-color: var(--primary); color: var(--on-primary); font-weight: 600; }
  .item.primary:hover:not(:disabled) { background: var(--primary); border-color: var(--primary); }
  .item.primary .detail,
  .item.primary .go { color: inherit; }
  .item.primary .dot { box-shadow: 0 0 0 1.5px var(--on-primary); }
  .item.danger { color: var(--danger); border-color: var(--danger-line); background: var(--danger-bg); font-weight: 600; }
  .item.danger:hover:not(:disabled) { border-color: var(--danger-line); background: var(--danger-bg); }
  /* An attack: taller, its cost, name and damage. */
  .sec[aria-label='Attack'] .item { min-height: 52px; min-width: 200px; font-size: var(--fs-15); font-weight: 600; }
  .tog { border-radius: var(--radius-pill); }
  .tog.on { background: var(--primary); border-color: var(--primary); color: var(--on-primary); }
  .tog.on:hover:not(:disabled) { background: var(--primary); border-color: var(--primary); }
  .text { white-space: pre-wrap; font: var(--fs-14)/1.5 var(--font-sans); margin: 0; padding: var(--space-3) var(--space-4); }

  /* The touch header: the title, a mono line of facts, the damage tag. */
  header { position: relative; display: flex; align-items: flex-start; gap: 14px; border-bottom: 1px solid var(--line); }
  .meta { display: flex; flex-direction: column; gap: 6px; min-width: 0; flex: 1; }
  h3 { margin: 0; font-weight: 600; line-height: 1.2; }
  .facts { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; font: 500 var(--fs-13) var(--font-mono); color: var(--muted); }
  .part { white-space: nowrap; }
  .tags { display: flex; align-items: center; gap: 6px; }
  .tag { padding: 0 6px; border-radius: var(--radius-xs); background: var(--damage-bg); color: var(--damage-ink); font-weight: 600; }
  .tags .dot { width: 10px; height: 10px; }
  .linkish {
    align-self: flex-start;
    min-height: var(--touch-target);
    padding: 0 var(--space-1);
    border: none;
    background: none;
    color: var(--primary);
    font: 600 var(--fs-14) var(--font-sans);
  }
  .close,
  .back {
    flex: none;
    min-height: var(--touch-target);
    border-radius: var(--radius-m);
    border: 1px solid var(--control-line);
    background: var(--panel-raised);
    color: var(--ink);
  }
  .close { display: grid; place-items: center; width: var(--touch-target); height: var(--touch-target); padding: 0; font-size: var(--fs-18); }
  .back { padding: 0 14px 0 10px; font: 600 var(--fs-14) var(--font-sans); }
  .close:hover:not(:disabled),
  .back:hover:not(:disabled),
  .linkish:hover:not(:disabled) { border-color: var(--control-line); background: var(--hover); }
  .linkish:hover:not(:disabled) { background: none; text-decoration: underline; }
  header.sub { align-items: center; }
  .ctx { flex: 1; min-width: 0; text-align: center; font: 600 var(--fs-14) var(--font-sans); color: var(--muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .art { flex: none; }

  /* Phone: the whole screen, each group one row of buttons that fill it
     or, past its width, scroll sideways, fading at the edge while more is
     off screen. */
  .sheet.phone { inset: 0; flex-direction: column; }
  .phone header { padding: 22px 12px 14px var(--space-4); touch-action: none; }
  .phone header.sub { padding: 22px 12px 12px; gap: var(--space-2); }
  .phone .grab { position: absolute; top: var(--space-2); left: 50%; translate: -50% 0; width: 40px; height: 5px; border-radius: 3px; background: var(--control-line); }
  .phone .art { --cw: 110px; }
  .phone h3 { font-size: var(--fs-19); margin-top: var(--space-1); }
  .phone .facts { flex-direction: column; align-items: flex-start; gap: 6px; font-size: var(--fs-12); }
  .phone .body { padding: var(--space-1) 0 30px; }
  /* Nothing in the phone sheet scrolls sideways: a row's buttons share
     its width, two to a line, a button whose label needs more taking the
     line and wrapping its label; three or more attacks one per line;
     Conditions three across, Other two across. */
  .phone .row { flex-wrap: wrap; }
  .phone .row .item { flex: 1 1 auto; min-width: calc(50% - var(--space-1)); max-width: 100%; box-sizing: border-box; justify-content: center; white-space: normal; text-align: center; padding-top: var(--space-1); padding-bottom: var(--space-1); }
  .phone .sec[aria-label='Attack'] .item { justify-content: flex-start; text-align: left; }
  .phone .sec[aria-label='Attack'] .row:has(> .item:nth-child(3)) .item { flex-basis: 100%; }
  .phone .row:has(> .tog) { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); }
  .phone .row:has(> .tog) .tog { min-width: 0; justify-content: center; padding: 0 var(--space-2); }
  .phone .sec.other .row { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .phone .row.grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .phone .row.list .item { justify-content: flex-start; text-align: left; min-height: 56px; }

  /* Tablet: a centred panel, the card on the left; only the actions scroll. */
  .sheet.tablet {
    left: 50%;
    top: 50%;
    translate: -50% -50%;
    width: min(900px, 94vw);
    height: min(660px, 90vh);
    border-radius: var(--radius-xl);
    overflow: hidden;
    box-shadow: var(--shadow-dialog);
  }
  .tablet .side {
    width: 320px;
    flex: none;
    padding: var(--space-6);
    background: var(--panel-sunk);
    border-right: 1px solid var(--line);
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    align-items: center;
    min-height: 0;
  }
  .tablet .side .text { overflow-y: auto; flex: 1; padding: 0; align-self: stretch; }
  .tablet .art { --cw: min(260px, calc((90vh - 140px) * 63 / 88)); }
  .tablet header { padding: var(--space-5) var(--space-5) 14px var(--space-6); }
  .tablet header.sub { padding: var(--space-4) var(--space-5) 14px var(--space-4); }
  .tablet h3 { font-size: var(--fs-22); }
  .tablet .body { padding: 6px var(--space-2) var(--space-5); }
  .tablet .sec { padding: var(--space-3) 0 2px; }
  .tablet .label { font-size: var(--fs-12); padding-bottom: var(--space-2); }
  .tablet .row { flex-wrap: wrap; }
  .seg { display: flex; border: 1px solid var(--control-line); border-radius: var(--radius-m); overflow: hidden; }
  .seg button {
    border: none;
    border-radius: 0;
    background: var(--panel-raised);
    color: var(--ink);
    padding: 0 22px;
    min-height: var(--touch-target);
    font: 600 var(--fs-14) var(--font-sans);
  }
  .seg button + button { border-left: 1px solid var(--control-line); }
  .seg button:hover:not(:disabled) { background: var(--hover); }
  .seg button.on,
  .seg button.on:hover:not(:disabled) { background: var(--primary); color: var(--on-primary); }

  /* Desktop popover: at the pointer, compact rows. */
  .sheet.popover {
    width: var(--popover-width);
    max-height: 80vh;
    border-radius: var(--radius-m);
    box-shadow: var(--shadow-menu);
    overflow: hidden;
    font-size: var(--fs-13);
  }
  .phead { padding: var(--space-3) 14px 10px; border-bottom: 1px solid var(--line); }
  .ptitle { font: var(--fw-ui-title) var(--fs-ui-title)/var(--lh-ui-title) var(--font-sans); }
  /* The evolution chain under the name ("Riolu → Lucario"), wrapping. */
  .chain { font: 500 var(--fs-13) var(--font-sans); color: var(--muted); overflow-wrap: anywhere; }
  .popover .chain { margin-top: 2px; font-size: var(--fs-12); }
  .psum { margin-top: 2px; font: 500 var(--fs-12) var(--font-mono); color: var(--muted); }
  .popover .body { padding-bottom: var(--space-1); }
  .popover .sec { padding: 0; }
  .popover .label { padding: var(--space-2) 14px 2px; }
  .popover .row { flex-direction: column; gap: 0; padding: 0; }
  .popover .item {
    min-height: 34px;
    min-width: 0;
    padding: 0 14px;
    border: none;
    border-radius: 0;
    background: none;
    font: 500 var(--fs-13) var(--font-sans);
    justify-content: flex-start;
  }
  .popover .item:hover:not(:disabled) { background: var(--hover); }
  .popover .sec[aria-label='Attack'] .item { min-height: 34px; min-width: 0; font-size: var(--fs-13); }
  .popover .sec[aria-label='Attack'] .item:not(.primary) { font-weight: 500; }
  .popover .detail { font: 600 var(--fs-12) var(--font-mono); }
  .popover .dot { width: 10px; height: 10px; }
  .popover .cost { gap: 2px; }
  .popover .ico,
  .popover .go,
  .popover .chev { font-size: var(--fs-14); }
  /* The common action: a full-width filled button. */
  .popover .item.primary:not(.opens) {
    min-height: 32px;
    margin: var(--space-1) 14px 6px;
    border-radius: var(--radius-s);
    justify-content: center;
    background: var(--primary);
    color: var(--on-primary);
    font-weight: 600;
  }
  /* The first attack: a highlighted row. */
  .popover .item.primary.opens { background: var(--hover); color: var(--ink); font-weight: 600; }
  .popover .item.primary.opens .detail { color: var(--muted); }
  .popover .item.primary .dot { box-shadow: none; }
  .popover .item.danger { color: var(--danger); background: none; font-weight: 600; }
  .popover .item.danger:hover:not(:disabled) { background: var(--hover); }
  /* Plain moves: a grid of small outlined buttons. */
  .popover .row.grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--space-1); padding: var(--space-1) 14px var(--space-2); }
  .popover .row.grid .item {
    min-height: 28px;
    padding: 0 6px;
    justify-content: center;
    border: 1px solid var(--control-line);
    border-radius: var(--radius-s);
    background: var(--panel-raised);
    font-size: var(--fs-12);
  }
  .popover .row.grid .item:hover:not(:disabled) { background: var(--hover); }
  /* Attached comes after Other, under a divider: 44px rows with a 32px
     picture. */
  .popover .sec[aria-label='Attached'] { margin-top: 6px; border-top: 1px solid var(--line); }
  .popover .row.list .item { min-height: 44px; padding: 0 14px; gap: 10px; border-radius: 0; }
  .popover .row.list .lbl { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .popover .row.list .detail { font: 500 var(--fs-12) var(--font-sans); color: var(--muted); }
  .popover .thumb { height: 32px; }
  /* Other comes under a divider, without its label. */
  .popover .sec.other { margin-top: 6px; border-top: 1px solid var(--line); }
  .popover .sec.other .label { display: none; }
  .popover .row:has(.tog) { flex-direction: row; flex-wrap: wrap; gap: var(--space-1); padding: var(--space-1) 14px var(--space-2); }
  .popover .tog { min-height: 26px; padding: 0 9px; font-size: var(--fs-12); }
  .popover .back { min-height: 28px; margin: var(--space-2) 14px 0; padding: 0 10px 0 var(--space-2); border-radius: var(--radius-s); font-size: var(--fs-12); }
  .popover :global(.stepper) { flex-wrap: nowrap; gap: var(--space-1); padding: var(--space-1) 14px 6px; }
  .popover :global(.stepper .sp) { flex: 1; width: auto; }
  .popover :global(.stepper button) { min-height: 28px; min-width: 30px; padding: 0 var(--space-2); border-radius: var(--radius-s); font-size: var(--fs-12); }
  .popover :global(.stepper input) { width: 52px; min-height: 28px; height: 28px; border-radius: var(--radius-s); font-size: var(--fs-13); }
  .phone :global(.stepper) { flex-wrap: nowrap; gap: 6px; }
  .phone :global(.stepper .sp) { display: none; }
  .phone :global(.stepper button) { min-height: 48px; min-width: 52px; padding: 0 var(--space-2); }
  /* The field takes the row's spare width; width 0 keeps its own width
     from pushing the row past the screen. */
  .phone :global(.stepper input) { flex: 1; min-width: 0; width: 0; min-height: 48px; font-size: var(--fs-19); }
</style>
