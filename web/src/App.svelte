<script>
  import { openDeckList } from './lib/actions.js';
  import { app, setPanelAt, setPrizeReminders } from './lib/store.svelte.js';
  import { act } from './lib/ws.js';
  import { setAutoplay } from './lib/playback.svelte.js';
  import Side from './components/Side.svelte';
  import Hand from './components/Hand.svelte';
  import Strip from './components/Strip.svelte';
  import Timeline from './components/Timeline.svelte';
  import PlaybackBar from './components/PlaybackBar.svelte';
  import ActionSheet from './components/ActionSheet.svelte';
  import TargetBanner from './components/TargetBanner.svelte';
  import Modal from './components/Modal.svelte';
  import Zoom from './components/Zoom.svelte';

  const snap = $derived(app.snap);
  const review = $derived(snap?.mode === 'review');
  // On a phone the board is compact, and the timeline is a full-screen
  // sheet opened from ☰ rather than a column beside the board.
  const phone = $derived(app.layout === 'phone');
  let timelineOpen = $state(false);
  // fit is how many pixels the phone board gives up so it never scrolls:
  // the cards shrink by fit / 6.6 when the board would run past the
  // screen (a two-row strip, Pokémon with many attachments) and grow back
  // as room returns.
  let fit = $state(0);
  function fitBoard(main) {
    const measure = () => {
      const used = [...main.children].reduce((h, c) => h + c.offsetHeight, 0);
      const over = used - main.clientHeight;
      if (over > 0) fit += over;
      else if (over < -24 && fit > 0) fit = Math.max(0, fit + over);
    };
    const ro = new ResizeObserver(measure);
    const watch = () => {
      ro.disconnect();
      ro.observe(main);
      for (const c of main.children) ro.observe(c);
      measure();
    };
    const mo = new MutationObserver(watch);
    mo.observe(main, { childList: true });
    watch();
    return () => {
      ro.disconnect();
      mo.disconnect();
      fit = 0;
    };
  }
  const showTimeline = $derived(!app.landscapePhone && (phone ? timelineOpen : app.panelOpen));
  const bottom = $derived(!phone && app.panelOpen && app.panelAt === 'bottom');
  // menuOpen shows the header's menu of less frequent actions.
  let menuOpen = $state(false);
  function menu(run) {
    menuOpen = false;
    run();
  }

  function setMode(ev) {
    act('set_mode', { mode: ev.target.checked ? 'review' : 'play' });
  }
  function setReveal(ev) {
    act('set_reveal', { reveal: ev.target.checked });
  }
  function checkout(ev) {
    act('checkout', { branch: ev.target.value });
  }
  // ownName is the signed-in account's name (its email before the @) on
  // the hosted playmat, or '' on the desktop, which has no account.
  function ownName() {
    return app.account ? app.account.split('@')[0] : '';
  }
  // oppStatus is the badge beside the opponent's name for each status.
  const oppStatus = {
    waiting: 'Opponent is waiting',
    acting: 'Opponent is acting',
    stopped: 'Opponent stopped responding',
  };

  function nameOf(seat) {
    const n = app.board?.sides[seat]?.name;
    return n || (seat === app.human ? ownName() || 'You' : app.assistant);
  }
</script>

<svelte:window onkeydown={(ev) => ev.key === 'Escape' && (menuOpen = false)} />

<div class="app" class:reveal={app.reveal} class:wide={!app.panelOpen || bottom} class:panel-bottom={bottom} class:phone style:--fit="{fit}px">
  <header>
    <span class="dot" class:on={app.connected} title={app.connected ? 'Connected' : 'Reconnecting…'}></span>
    {#if !phone}<strong>tcgreplay</strong>{/if}
    {#if snap && !phone}
      <span class="game">{snap.game}</span>
      <label class="branch">
        branch
        <select value={snap.branch} onchange={checkout}>
          {#each snap.branches as b (b.name)}
            <option value={b.name}>{b.name} (#{b.head})</option>
          {/each}
        </select>
      </label>
      <label class="toggle"><input type="checkbox" checked={review} onchange={setMode} /> Review mode</label>
      {#if review}
        <label class="toggle reveal-toggle"><input type="checkbox" checked={app.reveal} onchange={setReveal} /> Reveal all</label>
      {/if}
    {/if}
    {#if snap?.warnings?.length && !phone}
      <span class="warn" title={snap.warnings.join('\n')}>⚠ {snap.warnings.length}</span>
    {/if}
    <div class="more">
      <button class="burger" aria-label="Menu" aria-haspopup="menu" aria-expanded={menuOpen} onclick={() => (menuOpen = !menuOpen)}>
        ☰{#if phone && snap?.warnings?.length}<span class="badge" aria-label="{snap.warnings.length} warnings">{snap.warnings.length}</span>{/if}
      </button>
      {#if menuOpen}
        <div class="backdrop" onclick={() => (menuOpen = false)} role="presentation"></div>
        <div class="dropdown" role="menu">
          {#if snap && phone}
            <!-- The header's controls, which a phone's header has no room for. -->
            <div class="account">{snap.game}</div>
            {#each snap.warnings || [] as w, i (i)}
              <div class="warning">⚠ {w}</div>
            {/each}
            <label class="menu-check">
              Branch
              <select value={snap.branch} onchange={checkout} aria-label="branch">
                {#each snap.branches as b (b.name)}
                  <option value={b.name}>{b.name} (#{b.head})</option>
                {/each}
              </select>
            </label>
            <label class="menu-check"><input type="checkbox" checked={review} onchange={setMode} /> Review mode</label>
            {#if review}
              <label class="menu-check"><input type="checkbox" checked={app.reveal} onchange={setReveal} /> Reveal all</label>
            {/if}
            <div class="sep"></div>
          {/if}
          {#if snap}
            <button role="menuitem" onclick={() => menu(() => openDeckList(app.human))} title="Every card in your deck and where each copy is">Deck list</button>
          {/if}
          {#if phone}
            <button role="menuitem" onclick={() => menu(() => (timelineOpen = true))}>Show timeline</button>
          {:else}
            <button role="menuitem" onclick={() => menu(() => (app.panelOpen = !app.panelOpen))}>{app.panelOpen ? 'Hide' : 'Show'} timeline</button>
          {/if}
          <label class="menu-check" title="After a Knock Out, remind me how many prize cards to take">
            <input type="checkbox" checked={app.prizeReminders} onchange={(ev) => setPrizeReminders(ev.target.checked)} /> Prize reminders
          </label>
          <label class="menu-check" title="Play the assistant's moves on the board as they arrive">
            <input type="checkbox" checked={app.autoplay} onchange={(ev) => setAutoplay(ev.target.checked)} /> Autoplay {app.assistant}'s moves
          </label>
          {#if app.account}
            <div class="sep"></div>
            <div class="account" title="Signed in with Google">{app.account}</div>
            <form method="POST" action="/auth/logout" onsubmit={() => (app.signingOut = true)}><button role="menuitem" type="submit">Sign out</button></form>
          {/if}
        </div>
      {/if}
    </div>
  </header>

  {#if app.board?.winner}
    <div class="over-banner" role="status">Game over: {nameOf(app.board.winner)} wins ({app.board.win_reason})</div>
  {/if}
  {#if app.reveal}
    <div class="reveal-banner">REVEAL VIEW: both hands, prizes and decks are face up. Post-mortem only.</div>
  {/if}

  <div class="body" class:panel={showTimeline && !phone} class:bottom>
    {#if app.landscapePhone}
      <!-- A phone held sideways is too short for the board. -->
      <div class="upright" role="status">
        <span class="phone-icon" aria-hidden="true">📱</span>
        <p>Turn your phone upright to see the board.</p>
      </div>
    {:else}
    <main {@attach phone && fitBoard}>
      {#if app.board}
        {#if !phone || app.opponent}
          <div class="seat-name top">
            {#if !phone}{nameOf(app.claude)} · {app.claude}{/if}
            {#if app.opponent}<span class="opp-status {app.opponent}">{oppStatus[app.opponent]}</span>{/if}
          </div>
        {/if}
        {#if !phone}<Hand seat={app.claude} small />{/if}
        <Side seat={app.claude} name={nameOf(app.claude)} mirrored />
        <Strip />
        <Side seat={app.human} name={nameOf(app.human)} />
        <Hand seat={app.human} />
        {#if !phone}<div class="seat-name">{nameOf(app.human)} · {app.human}</div>{/if}
      {:else}
        <div class="landing">
          <h1>No game open</h1>
          {#if app.notice}
            <p class="notice">{app.notice}</p>
          {:else}
            <p>Ask {app.assistant} to import a battle log or a saved game record, or to start a new game.</p>
          {/if}
          {#if !app.connected}<p class="muted">Connecting to tcgreplay…</p>{/if}
        </div>
      {/if}
    </main>
    {/if}
    {#if showTimeline}
      <aside class:sheet={phone}>
        {#if phone}
          <h2>Timeline</h2>
          <button class="close" aria-label="Close the timeline" onclick={() => (timelineOpen = false)}>✕</button>
        {:else}
          <!-- Like a dev tools dock button: the icon shows where the
               timeline goes next. -->
          {@const where = bottom ? 'beside' : 'below'}
          <div class="aside-head">
            <h2>Timeline</h2>
            <button class="dock" aria-label="Move timeline {where} the board" title="Move timeline {where} the board" onclick={() => setPanelAt(bottom ? 'side' : 'bottom')}>
              <svg viewBox="0 0 16 16" width="16" height="16" aria-hidden="true">
                <rect x="1.5" y="2.5" width="13" height="11" rx="1.5" fill="none" stroke="currentColor" />
                {#if bottom}
                  <rect x="10" y="3" width="4" height="10" fill="currentColor" />
                {:else}
                  <rect x="2" y="9.5" width="12" height="3.5" fill="currentColor" />
                {/if}
              </svg>
            </button>
          </div>
        {/if}
        <Timeline />
      </aside>
    {/if}
  </div>

  <!-- The caption stacks under the playback bar, however many rows the bar
       wraps to. -->
  <div class="overlay">
    <PlaybackBar />
    {#if app.caption}
      <div class="caption">{app.caption}</div>
    {/if}
  </div>

  <div class="toasts" aria-live="polite">
    {#each app.toasts as t (t.id)}
      <div class="toast {t.kind}">{t.text}</div>
    {/each}
  </div>

  <TargetBanner />
  <ActionSheet />
  <Modal />
  <Zoom />
</div>

<style>
  .app {
    min-height: 100vh;
    display: flex;
    flex-direction: column;
    border: 0 solid transparent;
    box-sizing: border-box;
  }
  .app.reveal { border: 6px solid var(--reveal); }
  /* With no timeline beside the board, the board takes the full width (see
     --cw in app.css). With the timeline below, the board also keeps 200px of
     height for it, and the timeline fills whatever height the board leaves. */
  .app.wide {
    --cw: clamp(48px, min((100vh - 115px) / 11.1, (100vw - 40px) / 10), 240px);
    --fs: clamp(12px, calc(var(--cw) * 0.15), 28px);
  }
  .app.panel-bottom {
    height: 100vh;
    --cw: clamp(48px, min((100vh - 315px) / 11.1, (100vw - 40px) / 10), 240px);
  }
  /* Phone: the board fits the screen and never scrolls; only the hand, a
     bench of more than five and the menus do. Cards are five and a bit
     across, or smaller when the screen is short: the field (two benches
     and two Active Spots) is about 6.6 card widths tall, beside about
     372px of counters, strip, hand, padding and the Energy tucked under
     each Active. */
  .app.phone {
    --cw: max(40px, min(calc((100vw - 32px) / 5.6), calc((100dvh - 372px - var(--fit, 0px)) / 6.6)));
    --fs: 13px;
    height: 100dvh;
    min-height: 0;
  }
  .app.phone main { justify-content: flex-start; overflow: hidden; }
  /* No header bar on a phone: the dot and ☰ sit at the start of the
     opponent's counters (CounterBar leaves room for them). */
  .app.phone { position: relative; }
  .app.phone header {
    position: absolute;
    top: 0;
    left: 0;
    z-index: 6;
    gap: 6px;
    padding: 2px 4px 2px 8px;
    background: none;
    border: 0;
  }
  .badge {
    position: absolute;
    top: -2px;
    right: -2px;
    min-width: 18px;
    height: 18px;
    padding: 0 4px;
    border-radius: 9px;
    background: #ffb300;
    color: #222;
    font: 700 11px/18px system-ui;
  }
  .burger { position: relative; }
  .warning { padding: 6px 12px; font-size: 12px; color: var(--muted); max-width: 260px; }
  aside.sheet {
    position: fixed;
    inset: 0;
    z-index: 65;
    max-height: none;
    border: 0;
  }
  aside.sheet .close { position: absolute; top: 4px; right: 8px; width: 44px; height: 44px; padding: 0; border-radius: 50%; }
  .upright { background: var(--felt); color: #f2f6f3; font-size: 16px; flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 8px; padding: 16px; text-align: center; }
  .phone-icon { font-size: 40px; display: inline-block; transform: rotate(90deg); }
  header {
    display: flex;
    align-items: center;
    gap: 14px;
    padding: 6px 14px;
    background: var(--panel);
    border-bottom: 1px solid var(--line);
    flex-wrap: wrap;
    font-size: 13px;
  }
  .dot { width: 10px; height: 10px; border-radius: 50%; background: #c62828; }
  .dot.on { background: #43a047; }
  .game { color: var(--muted); }
  .branch select { margin-left: 4px; }
  .toggle { display: flex; gap: 4px; align-items: center; cursor: pointer; }
  .reveal-toggle { color: var(--reveal); font-weight: 700; }
  .warn { color: #ffb300; cursor: help; }
  .more { margin-left: auto; position: relative; }
  .burger { font-size: 16px; line-height: 1; padding: 4px 9px; }
  .backdrop { position: fixed; inset: 0; z-index: 70; }
  .dropdown {
    position: absolute;
    right: 0;
    top: calc(100% + 6px);
    z-index: 71;
    min-width: 230px;
    display: flex;
    flex-direction: column;
    padding: 4px;
    background: var(--panel);
    border: 1px solid var(--line);
    border-radius: 8px;
    box-shadow: 0 6px 20px #0008;
  }
  /* On a phone ☰ sits at the left, so its menu opens rightwards, and
     scrolls if it is taller than the screen. */
  .app.phone .dropdown { right: auto; left: 0; max-height: calc(100dvh - 60px); overflow-y: auto; max-width: calc(100vw - 16px); }
  .dropdown button { width: 100%; text-align: left; border-color: transparent; background: none; }
  .dropdown button:hover { background: var(--hover); }
  .dropdown form { margin: 0; }
  .menu-check { display: flex; align-items: center; gap: 6px; padding: 5px 10px; font-size: 13px; cursor: pointer; border-radius: 6px; }
  .menu-check:hover { background: var(--hover); }
  .sep { height: 1px; background: var(--line); margin: 4px 0; }
  .account { color: var(--muted); padding: 4px 10px; font-size: 12px; }
  .over-banner {
    background: var(--accent);
    color: #111;
    font-weight: 800;
    text-align: center;
    padding: 4px;
  }
  .reveal-banner {
    background: var(--reveal);
    color: #fff;
    font-weight: 800;
    text-align: center;
    padding: 4px;
    letter-spacing: 0.04em;
  }
  .body { flex: 1; display: grid; grid-template-columns: 1fr; min-height: 0; }
  .body.panel { grid-template-columns: 1fr clamp(320px, 18vw, 480px); }
  main { overflow: auto; display: flex; flex-direction: column; justify-content: center; font-size: var(--fs); }
  aside {
    background: var(--panel);
    border-left: 1px solid var(--line);
    display: flex;
    flex-direction: column;
    min-height: 0;
    max-height: calc(100vh - 44px);
    position: sticky;
    top: 0;
  }
  .body.panel.bottom { grid-template-columns: 1fr; grid-template-rows: auto minmax(200px, 1fr); }
  .body.bottom aside {
    border-left: 0;
    border-top: 1px solid var(--line);
    max-height: none;
    position: static;
  }
  aside h2 { font-size: 13px; margin: 10px 12px 0; text-transform: uppercase; letter-spacing: 0.08em; }
  .aside-head { display: flex; align-items: center; justify-content: space-between; padding-right: 8px; }
  aside .dock {
    display: grid; place-items: center; width: 26px; height: 22px; padding: 0; margin-top: 8px;
    background: none; border: 1px solid transparent; border-radius: 4px; color: var(--muted); cursor: pointer;
  }
  aside .dock:hover, aside .dock:focus-visible { color: var(--text); border-color: var(--line); }
  .seat-name { text-align: center; color: var(--muted); font-size: 0.85em; padding: 0.15em; }
  .opp-status { margin-left: 0.6em; padding: 0.05em 0.6em; border-radius: 999px; border: 1px solid var(--line); color: var(--text); }
  .opp-status.acting { border-color: var(--accent); color: var(--accent); }
  .opp-status.stopped { border-color: var(--reveal); color: var(--reveal); }
  .landing { text-align: center; padding: 60px 20px; display: flex; flex-direction: column; gap: 8px; align-items: center; }
  .muted { color: var(--muted); }
  .overlay {
    position: fixed;
    top: 52px;
    left: 50%;
    transform: translateX(-50%);
    z-index: 25;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 8px;
    width: max-content;
    max-width: calc(100vw - 32px);
    pointer-events: none;
  }
  .overlay > :global(*) { pointer-events: auto; }
  .caption {
    white-space: pre-line;
    font-size: 15px;
    line-height: 1.45;
    background: #000d;
    color: #fff;
    padding: 8px 16px;
    border-radius: 20px;
    border: 2px solid var(--accent);
    max-width: 80vw;
    text-align: center;
  }
  .toasts {
    position: fixed;
    right: 16px;
    bottom: 16px;
    display: flex;
    flex-direction: column;
    gap: 6px;
    z-index: 60;
    max-width: min(420px, calc(100vw - 32px));
  }
  .toast { padding: 8px 12px; border-radius: 8px; box-shadow: 0 4px 16px #0008; font-size: 13px; }
  .toast.error { background: #b71c1c; color: #fff; }
  .toast.warn { background: #ffb300; color: #222; }
  .toast.info { background: #263238; color: #fff; }
  @media (max-width: 900px) {
    .body.panel { grid-template-columns: 1fr; }
    aside { max-height: 40vh; position: static; }
  }
</style>
