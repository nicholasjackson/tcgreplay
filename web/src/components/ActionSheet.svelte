<script>
  // A card's actions, from a menu model (lib/actions.js), in one of three
  // containers: the full-screen phone sheet, the tablet panel with the card
  // beside its actions, or the desktop popover at the pointer. The model is
  // rebuilt as the board changes, so the damage and conditions shown stay
  // current while the sheet is open.
  import { app, cardInfo } from '../lib/store.svelte.js';
  import { startTarget } from '../lib/target.js';
  import Card from './Card.svelte';
  import Stepper from './Stepper.svelte';

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

  // fade marks a row that scrolls sideways while more of it is off screen.
  function fade(row) {
    const update = () => row.classList.toggle('more', row.scrollLeft + row.clientWidth < row.scrollWidth - 1);
    const ro = new ResizeObserver(update);
    ro.observe(row);
    row.addEventListener('scroll', update, { passive: true });
    update();
    return () => {
      ro.disconnect();
      row.removeEventListener('scroll', update);
    };
  }
</script>

<svelte:window onkeydown={(ev) => ev.key === 'Escape' && s && !s.hidden && close()} onpopstate={popstate} />

{#snippet text()}
  <pre class="text">{model.text ?? info?.text ?? (model.card.def ? 'Loading card text…' : 'No card text.')}</pre>
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
        <div class="ptitle">{model.title}</div>
        {#if model.summary}<div class="psum">{model.summary}</div>{/if}
      {:else}
        <header onpointerdown={(ev) => (y0 = ev.clientY)} onpointerup={swipeEnd} onpointercancel={() => (y0 = null)}>
          {#if mode === 'phone'}<span class="grab"></span>{/if}
          {#if mode === 'phone' && model.card}
            <div class="art"><Card card={model.card} draggable={false} lift={false} onclick={() => (showText = !showText)} /></div>
          {/if}
          <div class="meta">
            <h3>{model.title}</h3>
            {#if model.summary}<div class="sum">{model.summary}</div>{/if}
            {#if mode === 'phone' && model.card}
              <button class="linkish" aria-expanded={showText} onclick={() => (showText = !showText)}>{showText ? 'Actions ›' : 'Card text ›'}</button>
            {/if}
          </div>
          <button class="close" aria-label="Close" onclick={close}>✕</button>
        </header>
      {/if}
      <div class="body">
        {#if showText && mode === 'phone'}
          {@render text()}
        {:else if panel}
          <!-- A panel can close the sheet as it acts, which clears panel
               while it is still on screen. -->
          {@const Panel = panel?.component}
          <button class="back" onclick={() => (panel = null)}>‹ Actions</button>
          <Panel {...panel?.props} {close} />
        {:else}
          {#each model.sections as sec (sec.label)}
            <section class="sec" role="group" aria-label={sec.label}>
              <div class="label">{sec.label}</div>
              {#if sec.stepper}
                <Stepper {...sec.stepper} label="{sec.label} total" hint={mode !== 'popover'} />
              {:else if sec.toggles}
                <div class="row" {@attach mode === 'phone' && fade}>
                  {#each sec.toggles as t (t.label)}
                    <button class="tog" class:on={t.on} aria-pressed={t.on} onclick={() => t.set(!t.on)}>{t.label}</button>
                  {/each}
                </div>
              {:else}
                <div class="row" {@attach mode === 'phone' && fade}>
                  {#each sec.items as item (item.label)}
                    <button class="item" class:primary={item.primary} class:danger={item.danger} onclick={() => choose(item)}>
                      <span>{item.label}</span>
                      {#if item.detail}<span class="detail">{item.detail}</span>{/if}
                      {#if item.target}<span class="go" aria-hidden="true">›</span>{/if}
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
  .backdrop.tablet { background: #000a; backdrop-filter: blur(1.5px); }
  .sheet {
    position: fixed;
    z-index: 41;
    background: var(--panel);
    color: var(--text);
    display: flex;
  }
  /* Hidden while the attack panel's target is chosen on the board. */
  .sheet[hidden],
  .backdrop[hidden] { display: none; }
  .main { display: flex; flex-direction: column; flex: 1; min-width: 0; min-height: 0; }
  .body { flex: 1; min-height: 0; overflow-y: auto; }
  .label { font-size: 11px; letter-spacing: 0.08em; text-transform: uppercase; color: var(--muted); font-weight: 700; padding: 0 16px 6px; }
  .sec { padding: 10px 0 6px; }
  .row { display: flex; gap: 8px; padding: 0 16px; }
  .sec button {
    min-height: 44px;
    padding: 0 14px;
    border-radius: 10px;
    font-weight: 600;
    font-size: 14px;
    white-space: nowrap;
    flex: none;
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }
  .item.danger { color: #ff8a80; border-color: #6b2b2b; background: #3a1f1f; }
  .detail { font-weight: 800; color: var(--accent); }
  .item.primary .detail { color: #1b1b1b; }
  .go { opacity: 0.7; font-size: 15px; }
  .tog { border-radius: 22px; min-height: 40px; background: transparent; }
  .tog.on { background: #6a3fa833; border-color: #a77be0; color: #e6d4ff; }
  /* The check is decoration; aria-pressed carries the state. */
  .tog.on::before { content: '✓ ' / ''; }
  .text { white-space: pre-wrap; font: 14px/1.5 system-ui, sans-serif; margin: 0; padding: 12px 16px; }
  header { position: relative; display: flex; gap: 14px; padding: 16px 64px 12px 16px; border-bottom: 1px solid var(--line); }
  .meta { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
  h3 { margin: 0; font-size: 19px; line-height: 1.2; }
  .sum { color: var(--muted); font-size: 12px; }
  .linkish { align-self: flex-start; background: none; border: none; padding: 0; color: var(--accent); font-weight: 600; font-size: 13px; min-height: 44px; }
  .close { position: absolute; right: 12px; top: 12px; width: 44px; height: 44px; border-radius: 50%; border: none; background: #ffffff12; font-size: 18px; padding: 0; }
  .art { flex: none; }

  /* Phone: the whole screen, each group one row that scrolls sideways,
     fading at the edge while more is off screen. */
  .sheet.phone { inset: 0; flex-direction: column; }
  .phone header { padding-top: 24px; touch-action: none; }
  .phone .grab { position: absolute; top: 8px; left: 50%; translate: -50% 0; width: 40px; height: 5px; border-radius: 3px; background: #ffffff33; }
  .phone .art { --cw: 104px; }
  .phone .body { padding: 6px 0 30px; }
  .phone .row { overflow-x: auto; scrollbar-width: none; }
  .phone .row:global(.more) { mask-image: linear-gradient(90deg, #000 85%, transparent); -webkit-mask-image: linear-gradient(90deg, #000 85%, transparent); }

  /* Tablet: a centred panel, the card on the left; only the actions scroll. */
  .sheet.tablet {
    left: 50%;
    top: 50%;
    translate: -50% -50%;
    width: min(900px, 94vw);
    height: min(620px, 90vh);
    border-radius: 18px;
    overflow: hidden;
    box-shadow: 0 20px 60px #000c;
  }
  .tablet .side {
    width: 340px;
    flex: none;
    padding: 24px;
    background: var(--input);
    display: flex;
    flex-direction: column;
    gap: 12px;
    align-items: center;
    min-height: 0;
  }
  .tablet .side .text { overflow-y: auto; flex: 1; padding: 0; }
  .tablet .art { --cw: min(280px, calc((90vh - 140px) * 63 / 88)); }
  .tablet header { padding: 22px 64px 10px 20px; border-bottom: none; }
  .tablet h3 { font-size: 22px; }
  .tablet .body { padding: 0 4px 16px; }
  .tablet .row { flex-wrap: wrap; }
  .seg { display: flex; border: 1px solid var(--line); border-radius: 10px; overflow: hidden; }
  .seg button { border: none; border-radius: 0; background: none; padding: 8px 18px; min-height: 44px; font-weight: 600; }
  .seg button.on { background: #ffffff18; }

  /* Desktop popover: at the pointer, a compact list. */
  .sheet.popover {
    width: 264px;
    max-height: 80vh;
    border: 1px solid var(--line);
    border-radius: 10px;
    box-shadow: 0 12px 30px #000b;
    padding: 6px 0 8px;
    font-size: 13px;
  }
  .ptitle { padding: 6px 12px 4px; font-weight: 800; }
  .psum { padding: 0 12px 6px; color: var(--muted); font-size: 11px; }
  .popover .sec { padding: 0; }
  .popover .label { font-size: 10px; padding: 8px 12px 2px; }
  .popover .row { flex-direction: column; gap: 0; padding: 0; }
  .popover .item {
    min-height: 0;
    padding: 5px 12px;
    border: none;
    border-radius: 0;
    background: none;
    font-weight: 400;
    font-size: 13px;
    justify-content: space-between;
    text-align: left;
  }
  .popover .item:hover { background: var(--hover); }
  .popover .item.primary { background: none; color: var(--text); font-weight: 700; }
  .popover .item.danger { background: none; }
  .popover .detail { color: var(--muted); font-weight: 400; }
  .popover .row:has(.tog),
  .popover :global(.stepper) { flex-direction: row; flex-wrap: wrap; gap: 4px; padding: 4px 12px; }
  .popover .sec button.tog,
  .popover :global(.stepper button) { min-height: 0; padding: 2px 7px; border-radius: 6px; font-weight: 400; font-size: 13px; }
  .popover .tog.on::before { content: none; }
  .popover :global(.stepper input) { width: 46px; min-height: 0; font-size: 13px; border-radius: 6px; padding: 2px 0; }
  .back { background: none; border: none; color: var(--accent); font-weight: 600; min-height: 44px; padding: 0 16px; }
</style>
