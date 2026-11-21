<script>
  import { untrack } from 'svelte';
  import { openDeckList } from './lib/actions.js';
  import { app, benchSlots, setPanelAt, setPrizeReminders, setTheme, setMat, THEMES, MATS } from './lib/store.svelte.js';
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
  import markUrl from '../../internal/design/mark.svg';

  const snap = $derived(app.snap);
  const review = $derived(snap?.mode === 'review');
  // On a phone the board is compact, and the timeline is a full-screen
  // sheet opened from ☰ rather than a column beside the board.
  const phone = $derived(app.layout === 'phone');
  let timelineOpen = $state(false);
  // fit is how many pixels the phone board, the landscape mat or today's
  // mat above the docked timeline gives up so it never scrolls: the cards shrink when the board would run past
  // the screen (a two-row strip, Pokémon with many attachments) and grow
  // back as room returns. Only the rows in the flow count: the landscape
  // mat's peeking hand and corner name sit over it.
  let fit = $state(0);
  function fitBoard(main) {
    // A box laid out as display: contents (today's mat's halves) counts by
    // its children, which are the board's rows.
    const flat = (els) => els.flatMap((c) => (getComputedStyle(c).display === 'contents' ? flat([...c.children]) : [c]));
    const measure = () => {
      const cs = getComputedStyle(main);
      const pad = parseFloat(cs.paddingTop) + parseFloat(cs.paddingBottom);
      const rows = flat([...main.children]).filter((c) => getComputedStyle(c).position !== 'absolute');
      // The phone keeps a least gap between its rows, one more above the
      // first (its ::before, which spaces the rows from the header).
      const gaps = (parseFloat(cs.rowGap) || 0) * (rows.length - 1 + (getComputedStyle(main, '::before').content === 'none' ? 0 : 1));
      // Fractional heights, not offsetHeight's rounded ones, so a board a
      // fraction of a pixel too tall still shrinks rather than scrolling.
      const used = rows.reduce((h, c) => h + c.getBoundingClientRect().height, pad + gaps);
      const over = used - main.clientHeight;
      if (over > 0) fit += over;
      else if (over < -24 && fit > 0) fit = Math.max(0, fit + over);
    };
    const ro = new ResizeObserver(measure);
    const watch = () => {
      ro.disconnect();
      ro.observe(main);
      for (const c of flat([...main.children])) ro.observe(c);
      measure();
    };
    const mo = new MutationObserver(watch);
    mo.observe(main, { childList: true });
    // Untracked: the first measure runs inside the attachment's effect, and
    // reading fit there would make the attachment depend on it. Each fit it
    // wrote would then tear it down (fit back to 0) and attach it again, a
    // loop that froze a busy landscape mat for 8 seconds before Svelte gave
    // up (effect_update_depth_exceeded).
    untrack(watch);
    return () => {
      ro.disconnect();
      mo.disconnect();
      fit = 0;
    };
  }
  // measureBoard keeps app.wide in step with the board area's width. The
  // board area resizes with the window and when the timeline moves or
  // hides, so one observer covers every way it can cross 1600px.
  function measureBoard(main) {
    const ro = new ResizeObserver(() => (app.wide = main.clientWidth >= 1600));
    ro.observe(main);
    return () => {
      ro.disconnect();
      app.wide = false;
    };
  }
  // mainH is the board area's height: what is left under the header, any
  // banners (Reveal all, game over) and, docked below, the timeline. Every
  // layout's card size follows it (--main-h), so the board takes exactly
  // the room it has and the window never scrolls. The area's height comes
  // from the page's grid, never from the board in it, so measuring it
  // cannot feed back.
  let mainH = $state(0);
  // mainW is its width, which today's mat's card size also follows
  // (--main-w): the widest row, the player block, the bench and the top of
  // the discard, must fit it, so the bench never covers the block.
  let mainW = $state(0);
  function measureMain(main) {
    const ro = new ResizeObserver(() => {
      mainH = main.clientHeight;
      mainW = main.clientWidth;
    });
    ro.observe(main);
    return () => {
      ro.disconnect();
      mainH = 0;
      mainW = 0;
    };
  }
  // blockW is the wider player block's least width on today's mat: its
  // prize row and pile chips, which keep one line (the name may wrap). The
  // card size leaves room for it beside the bench (--block-w). It hardly
  // depends on the card size (the chips' text is a fixed size until the
  // cards are large), and it changes only past a whole pixel, so it settles.
  let blockW = $state(0);
  function measureBlocks(main) {
    // last is kept outside the state: the attachment must not read blockW,
    // or writing it would re-run the attachment, and so on.
    let last = 0;
    const measure = () => {
      let w = 0;
      for (const el of main.querySelectorAll('.seat-row .block .piles, .seat-row .block .prizes')) w = Math.max(w, el.getBoundingClientRect().width);
      w = Math.ceil(w);
      if (Math.abs(w - last) >= 1) {
        last = w;
        untrack(() => (blockW = w));
      }
    };
    const ro = new ResizeObserver(measure);
    const watch = () => {
      ro.disconnect();
      for (const el of main.querySelectorAll('.seat-row .block .piles, .seat-row .block .prizes')) ro.observe(el);
      measure();
    };
    const mo = new MutationObserver(watch);
    mo.observe(main, { childList: true, subtree: true });
    watch();
    return () => {
      ro.disconnect();
      mo.disconnect();
      untrack(() => (blockW = 0));
    };
  }
  // benchN is the longer bench's slot count: the landscape mat's widest
  // row, which its card size must fit.
  const benchN = $derived(app.board ? Math.max(...Object.values(app.board.sides).map((s) => benchSlots(s).length)) : 5);
  const showTimeline = $derived(!app.landscapePhone && (phone ? timelineOpen : app.panelOpen));
  const bottom = $derived(!phone && app.panelOpen && app.panelAt === 'bottom');
  // The playback controls head the timeline beside or below the board;
  // with it hidden, or a phone's sheet, they float over the board.
  const docked = $derived(showTimeline && !phone);

  // The docked timeline's height follows its handle, dragged or keyed,
  // within bounds that leave the board room for its smallest cards:
  // today's mat is about 540px tall at 40px cards, the landscape mat
  // about 6.9 card widths and 80px at 48px. A touch screen's mat (48px
  // cards) seldom fits under a docked timeline anyway, so there the board
  // scrolls and keeps at least half the room. bodyRoom is the height the
  // board and the timeline share. The height starts at a default (220px,
  // or 28% of the screen on the landscape mat), always within the bounds,
  // and is kept in memory only (app.dockHeight).
  const DOCK_MIN = 160;
  let screenH = $state(0);
  let bodyRoom = $state(0);
  function measureBody(body) {
    const ro = new ResizeObserver(() => {
      const cs = getComputedStyle(body);
      bodyRoom = body.clientHeight - parseFloat(cs.paddingTop) - parseFloat(cs.paddingBottom);
    });
    ro.observe(body);
    return () => ro.disconnect();
  }
  const boardMin = $derived(app.wide ? 6.9 * 48 + 80 : app.layout === 'tablet' ? bodyRoom / 2 : 540);
  const dockMax = $derived(Math.max(DOCK_MIN, Math.floor(bodyRoom - boardMin)));
  const dockDefault = $derived(app.wide ? Math.min(480, Math.max(200, screenH * 0.28)) : 220);
  const dockPx = $derived(Math.round(Math.min(dockMax, Math.max(DOCK_MIN, app.dockHeight ?? dockDefault))));
  function setDock(h) {
    app.dockHeight = Math.round(Math.min(dockMax, Math.max(DOCK_MIN, h)));
  }
  function dragDock(ev) {
    if (ev.button !== 0) return;
    ev.preventDefault();
    const handle = ev.currentTarget;
    handle.setPointerCapture(ev.pointerId);
    const [y, h] = [ev.clientY, dockPx];
    const move = (e) => setDock(h + y - e.clientY);
    const end = () => {
      handle.removeEventListener('pointermove', move);
      handle.removeEventListener('pointerup', end);
      handle.removeEventListener('pointercancel', end);
    };
    handle.addEventListener('pointermove', move);
    handle.addEventListener('pointerup', end);
    handle.addEventListener('pointercancel', end);
  }
  function keyDock(ev) {
    const by = { ArrowUp: 16, ArrowDown: -16, PageUp: 80, PageDown: -80 }[ev.key];
    if (by) setDock(dockPx + by);
    else if (ev.key === 'Home') setDock(DOCK_MIN);
    else if (ev.key === 'End') setDock(dockMax);
    else return;
    ev.preventDefault();
  }
  // menuOpen shows the header's menu of less frequent actions.
  let menuOpen = $state(false);
  // warnOpen shows the header's warnings under its pill.
  let warnOpen = $state(false);
  function menu(run) {
    menuOpen = false;
    run();
  }

  function setMode(ev) {
    act('set_mode', { mode: ev.target.checked ? 'review' : 'play' });
  }
  function pickMode(mode) {
    act('set_mode', { mode });
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
    deciding: 'Opponent is deciding',
    stopped: 'Opponent stopped responding',
  };

  // statusWord is the word after the opponent's name in their player
  // block: their connection's status when the playmat knows it (hosted
  // play), otherwise whether it is their turn.
  const statusWords = { waiting: 'waiting', acting: 'acting', deciding: 'deciding', stopped: 'stopped responding' };
  function statusWord() {
    if (app.opponent) return statusWords[app.opponent] || app.opponent;
    const b = app.board;
    if (!b || b.phase === 'setup') return '';
    return b.current === app.claude && b.turn_open ? 'playing' : 'waiting';
  }

  function nameOf(seat) {
    const n = app.board?.sides[seat]?.name;
    return n || (seat === app.human ? ownName() || 'You' : app.assistant);
  }
</script>

<!-- The board, top to bottom: the opponent's name, hand and half, the
     strip, your half, hand and name. -->
{#snippet board()}
  <!-- Today's mat names each player in their player block; the landscape
       mat in the corner and under the hand. -->
  {#if app.wide || (phone && app.opponent)}
    <div class="seat-name top">
      {#if app.wide}{nameOf(app.claude)} · {app.claude}{/if}
      {#if app.opponent}<span class="opp-status {app.opponent}">{oppStatus[app.opponent]}</span>{/if}
    </div>
  {/if}
  {#if app.wide}<Hand seat={app.claude} small />{/if}
  <Side seat={app.claude} name={nameOf(app.claude)} mirrored status={!phone && !app.wide ? statusWord() : ''} />
  <Strip />
  <Side seat={app.human} name={nameOf(app.human)} />
  <Hand seat={app.human} />
  {#if app.wide}<div class="seat-name">{nameOf(app.human)} · {app.human}</div>{/if}
{/snippet}

<!-- Like a dev tools dock button: the icon shows where the timeline goes
     next. -->
{#snippet dockButton()}
  {@const where = bottom ? 'beside' : 'below'}
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
{/snippet}

<!-- The warnings pill opens the warnings, listed under it: the header's
     on every layout, the phone's showing only the count. -->
{#snippet warnings()}
  {#if snap?.warnings?.length}
    {@const n = snap.warnings.length}
    <div class="warn-wrap">
      <button class="warn" title={snap.warnings.join('\n')} aria-label="{n} {n === 1 ? 'warning' : 'warnings'}" aria-haspopup="dialog" aria-expanded={warnOpen} onclick={() => (warnOpen = !warnOpen)}>
        <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 3l9.5 17h-19z" /><path d="M12 10v4" /><path d="M12 17.5v.01" /></svg>{n}<span class="warn-word" aria-hidden="true">{n === 1 ? ' warning' : ' warnings'}</span>
      </button>
      {#if warnOpen}
        <div class="backdrop" onclick={() => (warnOpen = false)} role="presentation"></div>
        <div class="dropdown warn-list" role="dialog" aria-label="Warnings">
          {#each snap.warnings as w, i (i)}
            <div class="warning">⚠ {w}</div>
          {/each}
        </div>
      {/if}
    </div>
  {/if}
{/snippet}

<svelte:window bind:innerHeight={screenH} onkeydown={(ev) => ev.key === 'Escape' && (menuOpen = warnOpen = false)} />

<div class="app" class:reveal={app.reveal} class:wide={!app.panelOpen || bottom} class:panel-bottom={bottom} class:panel-side={showTimeline && !phone && !bottom} class:landscape={app.wide} data-wide={app.wide} class:phone class:tablet={app.layout === 'tablet'} style:--fit="{fit}px" style:--main-h={mainH ? `${mainH}px` : null} style:--main-w={mainW ? `${mainW}px` : null} style:--block-w={blockW ? `${blockW}px` : null} style:--dock-h={bottom ? `${dockPx}px` : null}>
  <header>
    {#if phone}
      <!-- A phone's header: the connection, whose turn it is, the
           warnings and ☰, which holds the rest. -->
      <span class="dot" class:on={app.connected} title={app.connected ? 'Connected' : 'Reconnecting…'}></span>
      {#if app.board}
        {#if app.board.phase === 'setup'}
          <strong class="whose">Setup</strong>
        {:else}
          <span class="turn-tag">T{app.board.turn}</span>
          <strong class="whose">{app.board.current === app.human ? 'Your turn' : `${nameOf(app.board.current)}'s turn`}</strong>
        {/if}
      {/if}
      <span class="spacer"></span>
      {@render warnings()}
    {:else}
      <span class="brand"><img src={markUrl} alt="" /><strong>TCG Replay</strong></span>
    {/if}
    {#if snap && !phone}
      <!-- Where the game sits: "Games / Alice vs Bob". The playmat has no
           page of games, so "Games" is plain text; a narrow window or a
           tablet shows the game's name alone. -->
      <nav class="game" aria-label="Breadcrumb"><span class="crumb">Games</span><span class="crumb" aria-hidden="true">/</span><strong>{snap.game}</strong></nav>
      <label class="branch">
        <span class="branch-label">Branch</span>
        <select value={snap.branch} onchange={checkout} aria-label="branch">
          {#each snap.branches as b (b.name)}
            <option value={b.name}>{b.name} · #{b.head}</option>
          {/each}
        </select>
      </label>
      <!-- Play or Review: a segmented switch over two radios, each named
           for its mode. -->
      <div class="mode" role="radiogroup" aria-label="Mode">
        <label class:on={!review}><input type="radio" name="mode" aria-label="Play mode" checked={!review} onchange={() => pickMode('play')} />Play</label>
        <label class:on={review}><input type="radio" name="mode" aria-label="Review mode" checked={review} onchange={() => pickMode('review')} />Review</label>
      </div>
      {#if review}
        <label class="toggle reveal-toggle"><input type="checkbox" checked={app.reveal} onchange={setReveal} /> Reveal all</label>
      {/if}
    {/if}
    {#if !phone}
      <span class="spacer"></span>
      {#snippet conn()}
        <span class="conn"><span class="dot" class:on={app.connected} title={app.connected ? 'Connected' : 'Reconnecting…'}></span>{app.connected ? 'Live' : 'Reconnecting…'}</span>
      {/snippet}
      {#if app.layout === 'tablet'}
        <!-- A tablet: the connection, the warnings, then the button that
             hides and shows the timeline, as ☰ does. -->
        {@render conn()}
        {@render warnings()}
        <button class="panel-toggle" aria-label="{app.panelOpen ? 'Hide' : 'Show'} timeline" aria-pressed={app.panelOpen} onclick={() => (app.panelOpen = !app.panelOpen)}>
          <svg viewBox="0 0 24 24" aria-hidden="true">
            <rect x="3" y="4" width="18" height="16" rx="2" />
            {#if app.panelAt === 'bottom'}<path d="M3 14h18" />{:else}<path d="M15 4v16" />{/if}
          </svg>
        </button>
      {:else}
        {@render warnings()}
        {@render conn()}
        <!-- The theme button switches the interface between light and dark,
             as ☰'s Interface choice does. -->
        <button class="theme" aria-label={app.theme === 'dark' ? 'Use the light interface' : 'Use the dark interface'} title="Light or dark interface" onclick={() => setTheme(app.theme === 'dark' ? 'light' : 'dark')}>
          <svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="8.5" /><path d="M12 3.5a8.5 8.5 0 0 1 0 17z" class="fill" /></svg>
        </button>
      {/if}
    {/if}
    <div class="more">
      <button class="burger" aria-label="Menu" aria-haspopup="menu" aria-expanded={menuOpen} onclick={() => (menuOpen = !menuOpen)}>
        <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 7h16M4 12h16M4 17h16" /></svg>{#if phone && snap?.warnings?.length}<span class="badge" aria-label="{snap.warnings.length} warnings">{snap.warnings.length}</span>{/if}
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
                  <option value={b.name}>{b.name} · #{b.head}</option>
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
            {#if app.panelOpen}
              <!-- Beside the board the timeline's control row has no room for
                   the dock button, so it is here. -->
              {@const where = bottom ? 'beside' : 'below'}
              <button role="menuitem" onclick={() => menu(() => setPanelAt(bottom ? 'side' : 'bottom'))}>Move timeline {where} the board</button>
            {/if}
          {/if}
          <label class="menu-check" title="After a Knock Out, remind me how many prize cards to take">
            <input type="checkbox" checked={app.prizeReminders} onchange={(ev) => setPrizeReminders(ev.target.checked)} /> Prize reminders
          </label>
          <label class="menu-check" title="Play the assistant's moves on the board as they arrive">
            <input type="checkbox" checked={app.autoplay} onchange={(ev) => setAutoplay(ev.target.checked)} /> Autoplay {app.assistant}'s moves
          </label>
          <div class="sep"></div>
          <div class="menu-group" role="radiogroup" aria-label="Interface">
            <span class="menu-label">Interface</span>
            {#each THEMES as t (t)}
              <label class="menu-check"><input type="radio" name="theme" value={t} checked={app.theme === t} onchange={() => setTheme(t)} /> {t === 'light' ? 'Light' : 'Dark'}</label>
            {/each}
          </div>
          <div class="menu-group" role="radiogroup" aria-label="Mat">
            <span class="menu-label">Mat</span>
            {#each MATS as m (m)}
              <label class="menu-check"><input type="radio" name="mat" value={m} checked={app.mat === m} onchange={() => setMat(m)} /> {m[0].toUpperCase() + m.slice(1)}</label>
            {/each}
          </div>
          <!-- At every layout and in the desktop app alike, outside the hosted-only account block. -->
          <div class="sep"></div>
          <button role="menuitem" onclick={() => menu(() => (app.modal = { kind: 'about' }))}>About tcgreplay</button>
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
    <div class="reveal-banner">{phone ? 'REVEAL VIEW: every card face up. Post-mortem only.' : 'REVEAL VIEW: both hands, prizes and decks are face up. Post-mortem only.'}</div>
  {/if}

  <div class="body" class:panel={showTimeline && !phone} class:bottom {@attach measureBody}>
    {#if app.landscapePhone}
      <!-- A phone held sideways is too short for the board. -->
      <div class="upright" role="status">
        <span class="phone-icon" aria-hidden="true">📱</span>
        <p>Turn your phone upright to see the board.</p>
      </div>
    {:else}
    <main {@attach measureMain} {@attach phone ? fitBoard : measureBoard} {@attach !phone && !app.wide && fitBoard} {@attach !phone && !app.wide && measureBlocks}>
      {#if app.board && app.wide}
        <!-- The landscape mat sizes its cards from the board area, so it
             sits in a box whose container units measure <main>. -->
        <div class="board" style:--bench-n={benchN} {@attach fitBoard}>{@render board()}</div>
      {:else if app.board}
        {@render board()}
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
      <aside class:sheet={phone} aria-label="Timeline">
        {#if phone}
          <h2>Timeline</h2>
          <button class="close" aria-label="Close the timeline" onclick={() => (timelineOpen = false)}>✕</button>
          <Timeline />
        {:else}
          {#if bottom}
            <!-- Drag (or key) the handle to share the height between the
                 board and the timeline. A focusable separator is a
                 widget, which Svelte's check does not know. -->
            <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
            <div
              class="grip"
              role="separator"
              aria-orientation="horizontal"
              aria-label="Resize the timeline"
              aria-valuenow={dockPx}
              aria-valuemin={DOCK_MIN}
              aria-valuemax={dockMax}
              tabindex="0"
              onpointerdown={dragDock}
              onkeydown={keyDock}
            >
              <span></span>
            </div>
          {/if}
          <!-- Beside the board the playback controls head the timeline (the
               dock button is in ☰, the row having no room for it); docked
               below, one row under the handle ends with the dock button. -->
          {#if bottom}
            <div class="aside-head dock-row">
              <h2>Timeline</h2>
              <PlaybackBar where="dock" />
              {@render dockButton()}
            </div>
          {:else}
            <PlaybackBar where="side" />
          {/if}
          <div class="aside-body">
            <Timeline rail={bottom} />
          </div>
        {/if}
      </aside>
    {/if}
  </div>

  <!-- The caption stacks under the playback bar, however many rows the bar
       wraps to. With the timeline beside or below the board, the bar heads
       the timeline instead and only the caption floats. -->
  <div class="overlay">
    {#if !docked}<PlaybackBar />{/if}
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
  /* The app is one screen tall, never more: the header, any banners, then
     the body (the board and the timeline) in what is left. */
  .app {
    height: 100dvh;
    min-height: 0;
    display: flex;
    flex-direction: column;
    border: 0 solid transparent;
    box-sizing: border-box;
    /* The timeline column's width beside the board. --panel-w is the room
       it takes from the board area (0 when it is below or hidden), so the
       playback overlay can centre on the board rather than the window. */
    --panel-col: clamp(var(--timeline-width), 18vw, var(--timeline-width-max));
    --panel-w: 0px;
    /* The gutter of page background around the mat and the timeline,
       thinner above and below on a short screen, where the mat's cards
       (sized by the root --cw) need the height. */
    --gutter: var(--space-3);
    --gutter-y: var(--gutter);
    /* Today's mat with the timeline beside it: its cards fit the board
       area's measured height (about 11.1 card widths inside its 28px of
       padding and a little more), less --fit when the board would still
       scroll (fitBoard), and its width less the timeline. The window's
       height less 115px stays a ceiling: it is the size the mat was tuned
       to, and the measured room only binds when banners or a short screen
       take more. */
    --cw: clamp(48px, min((100vh - 115px) / 11.1, (var(--main-h, calc(100dvh - 80px)) - 35px - var(--fit, 0px)) / 11.1, (100vw - 360px) / 10, var(--cw-w)), 240px);
    --fs: clamp(var(--fs-12), calc(var(--cw) * 0.15), var(--fs-28));
  }
  /* --cw-w is the width's limit on today's mat: the widest row is the
     player block (--block-w, measured), the bench (about 4.4 card widths
     and 72px of gaps and padding), the top of the discard (its card, 0.62
     of a card width, and a little padding), two 20px gaps and main's 22px
     padding each side; 176px covers the fixed parts with a few pixels to
     spare. So the cards shrink before the bench can cover the block. */
  .app { --cw-w: calc((var(--main-w, calc(100vw - 360px)) - 176px - var(--block-w, 180px)) / 5); }
  @media (max-height: 820px) { .app { --gutter-y: var(--space-1); } }
  @media (max-height: 740px) { .app { --gutter-y: 0px; } }
  .app.panel-side { --panel-w: calc(var(--panel-col) + var(--gutter)); }
  /* A touch screen's timeline is 362px: the design's 310px plus one Play
     button's width (52px), so its playback row fits on one line (growing
     with a very wide screen, as the desktop's does). */
  .app.tablet { --panel-col: clamp(362px, 18vw, var(--timeline-width-max)); }
  .app.reveal { border: 6px solid var(--warn-line); }
  /* With no timeline beside the board, the board takes the full width (see
     --cw in app.css). With the timeline below, the board also keeps 200px of
     height for it, and the timeline fills whatever height the board leaves. */
  .app.wide {
    --cw: clamp(48px, min((100vh - 140px) / 11.1, (var(--main-h, calc(100dvh - 80px)) - 60px - var(--fit, 0px)) / 11.1, (100vw - 64px) / 10, var(--cw-w)), 240px);
    --fs: clamp(var(--fs-12), calc(var(--cw) * 0.15), var(--fs-28));
  }
  /* Docked below, the timeline takes --dock-h (its handle sets it) and the
     board the rest: its cards fit the height left, about 11.1 card widths
     under 120px of header, gutters and padding, less --fit when the board
     would still scroll (fitBoard). */
  .app.panel-bottom {
    --dock-h: 220px;
    --cw: clamp(40px, min((100vh - 120px - var(--dock-h)) / 11.1, (var(--main-h, calc(100dvh - 300px)) - 40px - var(--fit, 0px)) / 11.1, (100vw - 64px) / 10, var(--cw-w)), 240px);
  }
  @media (pointer: coarse) {
    .app.panel-bottom { --cw: clamp(48px, min((100vh - 120px - var(--dock-h)) / 11.1, (var(--main-h, calc(100dvh - 300px)) - 40px - var(--fit, 0px)) / 11.1, (100vw - 64px) / 10, var(--cw-w)), 240px); }
  }
  /* The landscape mat (app.wide) fills the window and sizes its cards
     from the board area: <main> is a size container, and .board inside it
     takes the largest card width at which the four rows fit main's height
     and the widest row (the longer bench, the prize fan and the piles)
     fits its width. */
  /* The landscape mat gives its cards the room of a thinner gutter. */
  .app.landscape { --gutter: var(--space-1); --gutter-y: var(--space-1); }
  .app.landscape main { container-type: size; }
  .app.landscape.panel-bottom { --dock-h: clamp(200px, 28vh, 480px); }
  /* The landscape mat's rows: the opponent's Bench row, the middle row
     (both Active Spots, with captions) and your Bench row, about 6.4 card
     widths with their least gaps. Both hands are strips in its top and
     bottom margins (Hand.svelte), taking no row of their own. The gaps
     scale with the card width (the mat-gap-* tokens) and are least sizes:
     spare height spreads evenly over them. A busy board (tucked strips
     over the Benches) gives up --fit (see fitBoard). */
  .board {
    --cw: max(48px, min((100cqh - 30px - var(--fit, 0px)) / 7.3, (100cqw - 80px) / (4.6 + 1.16 * var(--bench-n, 5))));
    --fs: clamp(var(--fs-12), calc(var(--cw) * 0.15), var(--fs-28));
    --gap-section: max(var(--mat-gap-section-min), calc(var(--cw) * var(--mat-gap-section)));
    --gap-edge: max(var(--mat-gap-edge-min), calc(var(--cw) * var(--mat-gap-edge)));
    --gap-side: max(var(--mat-gap-side-min), calc(var(--cw) * var(--mat-gap-side)));
    --strip-h: max(var(--touch-target), calc(var(--cw) * var(--mat-hand-strip)));
    font-size: var(--fs);
    position: relative;
    height: 100%;
    /* The opponent's hand shows its cards' top strip, about 31% of a card
       (its name, type band and the top of its art), in the board's top
       margin, at least a fingertip tall on a touch screen. Yours shows
       its strip in the bottom margin. */
    --peek-h: calc(var(--cw) * 0.43);
    padding: calc(var(--peek-h) + var(--gap-edge)) 0 calc(var(--strip-h) + var(--gap-edge));
    display: flex;
    flex-direction: column;
    justify-content: space-evenly;
    row-gap: var(--gap-section);
  }
  @media (pointer: coarse) { .board { --peek-h: max(var(--touch-target), calc(var(--cw) * 0.43)); } }
  /* The players' names, small in the corners: the opponent's top left,
     yours bottom left. */
  .board > .seat-name.top { position: absolute; top: 8px; left: 12px; z-index: 6; padding: 0; text-align: left; }
  .board > .seat-name:not(.top) { position: absolute; bottom: 8px; left: 12px; z-index: 7; padding: 0; text-align: left; }
  /* Phone: the board fits the screen and never scrolls; only the hand, a
     bench of more than five and the menus do. --cw is a benched card's
     width, the design's 56px, or smaller when the board area is short:
     the cards (two Benches, two Active cards and the hand, whose cards
     are your Active's size up to 70px) are about 7.79 --cw tall, beside
     284px of counters, strip, Bench readouts, panel padding, the 10px foot
     and the least gaps between the rows (290px on touch, whose Stadium is
     a fingertip wide); a busy board gives up --fit as well (fitBoard). */
  .app.phone {
    --phone-rest: 284px;
    --cw: clamp(40px, min(56px, (100vw - 72px) / 5, (var(--main-h, calc(100dvh - 52px)) - var(--phone-rest) - var(--fit, 0px)) / 7.79), 56px);
    --fs: var(--fs-13);
    --gutter: 0px;
    --gutter-y: 0px;
  }
  /* The board's eight rows spread evenly down the screen, as the design
     has them: the ::before is an empty first row, so the gap under the
     header matches the gaps between the rows, and the hand sits 10px off
     the foot. */
  @media (pointer: coarse) { .app.phone { --phone-rest: 290px; } }
  .app.phone main { justify-content: space-between; row-gap: var(--space-1); padding: 0 0 10px; overflow: hidden; border-radius: 0; }
  .app.phone main::before { content: ''; order: -1; }
  .app.phone main > .seat-name { order: 0; }
  /* A phone's header: a slim light bar of the dot, the turn tag and whose
     turn it is, then the warnings and ☰. */
  .app.phone header {
    flex-wrap: nowrap;
    min-height: calc(var(--space-12) + var(--space-1));
    gap: 10px;
    padding: 0 10px 0 14px;
    font-size: var(--fs-ui);
  }
  .app.phone header button { border-radius: var(--space-2); }
  .app.phone header .burger svg { width: 20px; height: 20px; }
  .app.phone header button.warn { gap: var(--space-1); padding: 0 10px; font-size: var(--fs-13); }
  .app.phone header button.warn svg { width: 16px; height: 16px; }
  .turn-tag {
    padding: 3px 7px;
    border-radius: var(--radius-xs);
    background: var(--brand-navy);
    color: var(--turn-tag-ink);
    font: var(--fw-num) var(--fs-ui-s) var(--font-mono);
  }
  .whose { font-size: var(--fs-ui-title); font-weight: var(--fw-ui-title); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  /* A short phone has no room for the bar: the dot and ☰ sit at the start
     of the opponent's counters (CounterBar leaves room for them), the
     warnings counted on ☰, and the turn tag and whose turn it is move to
     the strip, over the Stadium's name (Strip.svelte). */
  .badge { display: none; }
  @media (max-height: 739px) {
    .app.phone { position: relative; }
    /* No room to spare: the rows may close up entirely. */
    .app.phone main { row-gap: 0; }
    .app.phone header { position: absolute; top: 0; left: 0; z-index: 6; min-height: 0; gap: 6px; padding: var(--space-1) 4px var(--space-1) 8px; background: none; border: 0; }
    .app.phone header :is(.turn-tag, .whose, .spacer, .warn-wrap) { display: none; }
    .app.phone .dropdown { right: auto; left: 0; }
    .app.phone .badge {
      display: block;
      position: absolute;
      top: -6px;
      right: -6px;
      min-width: 18px;
      height: 18px;
      padding: 0 4px;
      border-radius: var(--radius-pill);
      border: 1px solid var(--warn-line);
      background: var(--warn-bg);
      color: var(--warn-ink);
      font: var(--fw-num) var(--fs-num-s)/16px var(--font-mono);
    }
  }
  header button.warn { width: auto; gap: var(--space-1); padding: 0 var(--space-2); border-color: var(--warn-line); background: var(--warn-bg); color: var(--warn-ink); font-weight: 600; }
  header button.warn:hover:not(:disabled) { border-color: var(--warn-line); background: var(--warn-bg); }
  .burger { position: relative; }
  .warning {
    margin: 2px 4px;
    padding: 6px 10px;
    max-width: 260px;
    border-radius: var(--radius-s);
    border: 1px solid var(--warn-line);
    background: var(--warn-bg);
    color: var(--warn-ink);
    font-size: var(--fs-ui-s);
  }
  aside.sheet {
    position: fixed;
    inset: 0;
    z-index: 65;
    max-height: none;
    border: 0;
    border-radius: 0;
  }
  aside.sheet .close { position: absolute; top: 4px; right: 8px; z-index: 3; width: 44px; height: 44px; padding: 0; border-radius: 50%; }
  .upright { background: var(--mat-surface); color: var(--mat-ink); font-size: var(--fs-16); flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 8px; padding: 16px; text-align: center; }
  .phone-icon { font-size: var(--fs-40); display: inline-block; transform: rotate(90deg); }

  /* The header: a light bar of the mark, the game, its branch, the mode,
     Reveal all, then warnings, the connection and ☰ at the end. Its sizes
     are in em, so the whole bar grows with its text above 1920px. */
  header {
    display: flex;
    align-items: center;
    gap: var(--space-2) var(--space-4);
    min-height: var(--header-height);
    padding: var(--space-1) var(--space-5);
    background: var(--panel);
    color: var(--ink);
    border-bottom: 1px solid var(--line);
    flex-wrap: wrap;
    /* Header text is today's 13px up to 1920px wide and grows a little
       above it, to 15px at 3840 (--fs-grow-13, app.css). */
    font-size: var(--fs-grow-13);
  }
  header button, header select { font-size: inherit; }
  header svg { width: 1.38em; height: 1.38em; fill: none; stroke: currentColor; stroke-width: 2; stroke-linecap: round; stroke-linejoin: round; }
  .brand { display: flex; align-items: center; gap: 0.77em; }
  .brand img { width: 2.3em; height: 2.3em; display: block; }
  .brand strong { font-size: 1.3em; font-weight: 700; }
  /* The breadcrumb: "Games /" in muted, the game's name in ink. */
  .game { display: flex; align-items: center; gap: 0.6em; min-width: 0; color: var(--muted); }
  .game strong { color: var(--ink); font-weight: var(--fw-ui-title); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .branch { display: flex; align-items: center; gap: 0.6em; color: var(--muted); font-size: 0.92em; }
  .branch select {
    height: 2.5em;
    padding: 0 0.77em;
    border-radius: var(--radius-s);
    border: 1px solid var(--control-line);
    background: var(--panel);
    color: var(--ink);
    font-family: var(--font-mono);
    font-weight: 500;
    font-size: 1.09em;
  }
  /* Play | Review: the mode in force is filled in primary. */
  .mode { display: flex; border: 1px solid var(--control-line); border-radius: var(--radius-s); overflow: hidden; }
  .mode label {
    position: relative;
    display: flex;
    align-items: center;
    justify-content: center;
    height: 2.3em;
    padding: 0 1.08em;
    background: var(--panel);
    color: var(--muted);
    font-weight: 600;
    cursor: pointer;
  }
  .mode label + label { border-left: 1px solid var(--control-line); }
  .mode label:hover { background: var(--hover); }
  .mode label.on { background: var(--primary); color: var(--on-primary); cursor: default; }
  .mode label:has(input:focus-visible) { outline: 2px solid var(--primary); outline-offset: -4px; }
  .mode input { position: absolute; opacity: 0; width: 1px; height: 1px; margin: 0; pointer-events: none; }
  .toggle { display: flex; gap: 0.6em; align-items: center; cursor: pointer; }
  .toggle input { width: 1.23em; height: 1.23em; margin: 0; accent-color: var(--primary); }
  .spacer { flex: 1; }
  header .warn {
    display: inline-flex;
    align-items: center;
    gap: 0.46em;
    height: 2.3em;
    padding: 0 0.77em;
    border-radius: var(--radius-s);
    border: 1px solid var(--warn-line);
    background: var(--warn-bg);
    color: var(--warn-ink);
    font-weight: 600;
    cursor: help;
  }
  header .warn svg { width: 1.15em; height: 1.15em; }
  .sr { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
  .conn { display: inline-flex; align-items: center; gap: 0.46em; color: var(--muted); }
  .dot { width: 8px; height: 8px; flex: none; border-radius: 50%; background: var(--danger); }
  .dot.on { background: var(--live); }
  header button {
    display: grid;
    place-items: center;
    width: 2.6em;
    height: 2.6em;
    padding: 0;
    border-radius: var(--radius-s);
    border: 1px solid var(--control-line);
    background: var(--panel);
    color: var(--ink);
  }
  header button:hover:not(:disabled) { border-color: var(--control-line); background: var(--hover); }
  header button[aria-pressed='true'] { border-color: var(--primary); background: var(--primary); color: var(--on-primary); }
  header button[aria-pressed='true']:hover:not(:disabled) { border-color: var(--primary); background: var(--primary); }
  /* The warnings pill: warn colours, a triangle, the count and the word. */
  header button.warn {
    display: inline-flex;
    align-items: center;
    gap: 0.46em;
    width: auto;
    height: 2.3em;
    padding: 0 0.77em;
    border-color: var(--warn-line);
    background: var(--warn-bg);
    color: var(--warn-ink);
    font-weight: 600;
    cursor: pointer;
  }
  .warn-wrap { position: relative; }
  .warn-list { max-width: min(420px, calc(100vw - 32px)); }
  .warn-list .warning { max-width: none; }
  header svg .fill { fill: currentColor; stroke: none; }
  /* A narrow window and a tablet: the game's name alone, the select
     without its label, the pill with only its count, and no theme button
     (☰'s Interface choice stays). */
  @media (max-width: 1199px) {
    header :is(.crumb, .branch-label, .warn-word, .theme) { display: none; }
  }
  .app.tablet header :is(.crumb, .branch-label, .warn-word) { display: none; }
  .more { position: relative; }
  .backdrop { position: fixed; inset: 0; z-index: 70; }

  /* ☰'s menu: a raised panel of rows. */
  .dropdown {
    position: absolute;
    right: 0;
    top: calc(100% + 6px);
    z-index: 71;
    min-width: var(--popover-width);
    display: flex;
    flex-direction: column;
    padding: var(--space-1);
    background: var(--panel-raised);
    color: var(--ink);
    border-radius: var(--radius-m);
    box-shadow: var(--shadow-menu);
    font-size: var(--fs-grow-13);
  }
  /* On a phone the menu scrolls if it is taller than the screen. */
  .app.phone .dropdown { max-height: calc(100dvh - 60px); overflow-y: auto; max-width: calc(100vw - 16px); }
  .dropdown button {
    display: flex;
    align-items: center;
    width: 100%;
    min-height: 34px;
    padding: 0 var(--space-3);
    text-align: left;
    font-size: inherit;
    border: 0;
    border-radius: var(--radius-s);
    background: none;
    color: var(--ink);
  }
  .dropdown button:hover:not(:disabled) { background: var(--hover); }
  .dropdown form { margin: 0; }
  .menu-check { display: flex; align-items: center; gap: var(--space-2); min-height: 34px; padding: 0 var(--space-3); cursor: pointer; border-radius: var(--radius-s); }
  .menu-check:hover { background: var(--hover); }
  .menu-check input { margin: 0; accent-color: var(--primary); }
  .menu-check select { height: 30px; padding: 0 var(--space-2); border-radius: var(--radius-s); border: 1px solid var(--control-line); background: var(--panel); color: var(--ink); font-family: var(--font-mono); font-size: var(--fs-ui); }
  .sep { height: 1px; background: var(--line); margin: var(--space-1) 0; }
  .menu-group { display: flex; flex-wrap: wrap; align-items: center; }
  .menu-label { flex-basis: 100%; padding: var(--space-2) var(--space-3) var(--space-1); color: var(--muted); font-size: var(--fs-label); font-weight: var(--fw-label); letter-spacing: var(--ls-label); text-transform: uppercase; }
  .account { color: var(--muted); padding: var(--space-1) var(--space-3); font-size: var(--fs-ui-s); }

  header, .over-banner, .reveal-banner { flex: none; }
  /* A phone's banners are one slim line where they can be, so the board
     keeps its room. */
  .app.phone :is(.over-banner, .reveal-banner) { padding: 2px var(--space-2); font-size: var(--fs-ui-s); letter-spacing: 0; line-height: 1.3; }
  .app.phone.reveal { border-width: 3px; }
  .over-banner {
    background: var(--primary);
    color: var(--on-primary);
    font-weight: 700;
    text-align: center;
    padding: var(--space-1) var(--space-3);
  }
  .reveal-banner {
    background: var(--warn-bg);
    color: var(--warn-ink);
    border-bottom: 1px solid var(--warn-line);
    font-weight: 700;
    text-align: center;
    padding: var(--space-1) var(--space-3);
    letter-spacing: 0.04em;
  }

  /* The mat and the timeline are rounded panels on the page background. */
  .body { flex: 1; display: grid; grid-template-columns: 1fr; min-height: 0; padding: var(--gutter-y) var(--gutter); gap: var(--gutter); }
  .body.panel { grid-template-columns: 1fr var(--panel-col); }
  /* safe: a board taller than its area scrolls from its top, rather than
     centring and cutting the top off out of reach. */
  main { overflow: auto; display: flex; flex-direction: column; justify-content: safe center; font-size: var(--fs); border-radius: var(--radius-l); }
  /* Today's mat spreads its six rows evenly down the mat: the opponent's
     block row, their Active, the strip, your Active, your block row and
     your hand (Side.svelte lays each half's rows out as the mat's own). */
  /* Its padding gives way once the board needs more room than shrinking
     its cards can give (--fit grows past what the cards' least size
     allows: a touch screen's busy mat under a banner), so the window
     still never scrolls. */
  .app:not(.landscape, .phone) main { padding: max(0px, 14px - var(--fit, 0px) / 4) 22px; justify-content: space-between; }
  .app:not(.landscape, .phone) main > :global(.hand) { order: 6; }
  aside {
    background: var(--panel);
    color: var(--ink);
    border: 1px solid var(--line);
    border-radius: var(--radius-l);
    display: flex;
    flex-direction: column;
    min-height: 0;
    max-height: 100%;
    position: sticky;
    top: var(--gutter-y);
    overflow: hidden;
  }
  /* Docked below, the board and the timeline join into one panel. */
  /* Rows of a set height, never auto: the landscape mat's <main> is a size
     container, which takes no height from its content. */
  .body.panel.bottom { grid-template-columns: 1fr; grid-template-rows: minmax(0, 1fr) var(--dock-h); gap: 0; }
  .body.bottom main { border-radius: var(--radius-l) var(--radius-l) 0 0; }
  .body.bottom aside {
    border-radius: 0 0 var(--radius-l) var(--radius-l);
    max-height: none;
    position: static;
  }
  aside h2 { font-size: var(--fs-grow-13); font-weight: var(--fw-ui-title); color: var(--muted); margin: 0; white-space: nowrap; }
  .aside-head { display: flex; align-items: center; justify-content: space-between; padding-right: var(--space-2); }
  aside .dock {
    display: grid; place-items: center; flex: none; width: 30px; height: 30px; padding: 0; margin-top: var(--space-2);
    background: var(--panel); border: 1px solid var(--control-line); border-radius: var(--radius-s); color: var(--ink); cursor: pointer;
  }
  /* Docked, one row: the heading, the playback controls and the dock
     button, under the handle. */
  .aside-head.dock-row { gap: var(--space-2); padding: 2px 14px var(--space-2); border-bottom: 1px solid var(--line); }
  .aside-head.dock-row h2 { margin: 0 var(--space-2) 0 0; }
  .aside-head.dock-row .dock { margin: 0; }
  .aside-body { flex: 1; min-height: 0; display: flex; }
  /* The handle: a strip with a centred grip, a fingertip tall on touch. */
  .grip { flex: none; height: 10px; display: flex; justify-content: center; align-items: center; cursor: row-resize; touch-action: none; }
  .grip span { width: 44px; height: 4px; border-radius: 2px; background: var(--control-line); }
  .grip:hover span, .grip:focus-visible span { background: var(--muted); }
  .grip:focus-visible { outline: 2px solid var(--primary); outline-offset: -2px; }
  aside .dock:hover, aside .dock:focus-visible { color: var(--ink); background: var(--hover); border-color: var(--control-line); }
  .seat-name { text-align: center; color: var(--mat-muted); font-size: 0.85em; padding: 0.15em; }
  .opp-status { margin-left: 0.6em; padding: 0.05em 0.6em; border-radius: var(--radius-pill); border: 1px solid var(--mat-slot-line); color: var(--mat-ink); }
  .opp-status.acting { border-color: var(--accent); color: var(--accent); }
  .opp-status.deciding { border-color: var(--accent); color: var(--accent); border-style: dashed; }
  .opp-status.stopped { border-color: var(--damage-ink); color: var(--damage-ink); }
  .landing { text-align: center; padding: 60px 20px; display: flex; flex-direction: column; gap: 8px; align-items: center; }
  .muted { color: var(--muted); }
  .overlay {
    position: fixed;
    top: calc(var(--header-height) + var(--gutter-y) + var(--space-2));
    /* Centred on the board area: the window less the timeline beside it. */
    left: calc(50% - var(--panel-w) / 2);
    transform: translateX(-50%);
    z-index: 25;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 8px;
    width: max-content;
    max-width: calc(100% - var(--panel-w) - 32px);
    pointer-events: none;
  }
  .overlay > :global(*) { pointer-events: auto; }
  .caption {
    white-space: pre-line;
    font-size: var(--fs-body-s);
    line-height: var(--lh-body-s);
    background: var(--panel-raised);
    color: var(--ink);
    padding: var(--space-2) var(--space-4);
    border-radius: var(--radius-m);
    box-shadow: var(--shadow-menu);
    max-width: min(80vw, calc(100vw - var(--panel-w) - 32px));
    text-align: center;
  }
  .toasts {
    position: fixed;
    right: var(--space-4);
    bottom: var(--space-4);
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    z-index: 60;
    max-width: min(420px, calc(100vw - 32px));
    /* A toast never takes a touch: on a phone it lies over the hand, where
       the card it talks about (a pair's second half) waits to be dragged. */
    pointer-events: none;
  }
  .toast {
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-m);
    border: 1px solid var(--line);
    background: var(--panel-raised);
    color: var(--ink);
    box-shadow: var(--shadow-menu);
    font-size: var(--fs-ui);
    font-weight: var(--fw-ui);
  }
  .toast.error { background: var(--danger-bg); border-color: var(--danger-line); color: var(--danger); }
  .toast.warn { background: var(--warn-bg); border-color: var(--warn-line); color: var(--warn-ink); }
  @media (max-width: 900px) {
    .body.panel { grid-template-columns: 1fr; }
    aside { max-height: 40vh; position: static; }
    .app.panel-side { --panel-w: 0px; }
  }
  /* On a touch screen every header control is at least a fingertip's 44px. */
  @media (pointer: coarse) {
    .app:not(.phone) header { min-height: 60px; }
    header button, .mode, .branch select { border-radius: var(--space-2); }
    header .warn, .mode { min-height: var(--touch-target); }
    header .warn, header button.warn { min-width: var(--touch-target); height: var(--touch-target); justify-content: center; }
    .mode label { height: auto; }
    header button { width: var(--touch-target); height: var(--touch-target); }
    .branch select { height: var(--touch-target); }
    aside .dock { width: var(--touch-target); height: var(--touch-target); }
    .grip { height: var(--touch-target); }
  }
</style>
