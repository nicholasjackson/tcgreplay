<script>
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
  // fit is how many pixels the phone board, or the landscape mat, gives up
  // so it never scrolls: the cards shrink when the board would run past
  // the screen (a two-row strip, Pokémon with many attachments) and grow
  // back as room returns. Only the rows in the flow count: the landscape
  // mat's peeking hand and corner name sit over it.
  let fit = $state(0);
  function fitBoard(main) {
    const measure = () => {
      const pad = parseFloat(getComputedStyle(main).paddingTop) + parseFloat(getComputedStyle(main).paddingBottom);
      const rows = [...main.children].filter((c) => getComputedStyle(c).position !== 'absolute');
      const used = rows.reduce((h, c) => h + c.offsetHeight, pad);
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
  // benchN is the longer bench's slot count: the landscape mat's widest
  // row, which its card size must fit.
  const benchN = $derived(app.board ? Math.max(...Object.values(app.board.sides).map((s) => benchSlots(s).length)) : 5);
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
  {#if !phone}<Hand seat={app.claude} small />{/if}
  <Side seat={app.claude} name={nameOf(app.claude)} mirrored status={!phone && !app.wide && app.opponent ? oppStatus[app.opponent] : ''} />
  <Strip />
  <Side seat={app.human} name={nameOf(app.human)} />
  <Hand seat={app.human} />
  {#if app.wide}<div class="seat-name">{nameOf(app.human)} · {app.human}</div>{/if}
{/snippet}

<svelte:window onkeydown={(ev) => ev.key === 'Escape' && (menuOpen = false)} />

<div class="app" class:reveal={app.reveal} class:wide={!app.panelOpen || bottom} class:panel-bottom={bottom} class:panel-side={showTimeline && !phone && !bottom} class:landscape={app.wide} data-wide={app.wide} class:phone style:--fit="{fit}px">
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
      {#if snap?.warnings?.length}
        {@const n = snap.warnings.length}
        <button class="warn" aria-label="{n} {n === 1 ? 'warning' : 'warnings'}" title={snap.warnings.join('\n')} onclick={() => (menuOpen = true)}>
          <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 3l9.5 17h-19z" /><path d="M12 10v4" /><path d="M12 17.5v.01" /></svg>{n}
        </button>
      {/if}
    {:else}
      <span class="brand"><img src={markUrl} alt="" /><strong>tcgreplay</strong></span>
    {/if}
    {#if snap && !phone}
      <span class="game">{snap.game}</span>
      <label class="branch">
        Branch
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
      {#if snap?.warnings?.length}
        {@const n = snap.warnings.length}
        <span class="warn" title={snap.warnings.join('\n')}>
          <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 3l9.5 17h-19z" /><path d="M12 10v4" /><path d="M12 17.5v.01" /></svg>
          {n}<span class:sr={app.layout === 'tablet'}> {n === 1 ? 'warning' : 'warnings'}</span>
        </span>
      {/if}
      <span class="conn"><span class="dot" class:on={app.connected} title={app.connected ? 'Connected' : 'Reconnecting…'}></span>{app.connected ? 'Live' : 'Reconnecting…'}</span>
      {#if app.layout === 'tablet'}
        <!-- On a touch screen the timeline hides and shows from the header,
             as from ☰. -->
        <button class="panel-toggle" aria-label="{app.panelOpen ? 'Hide' : 'Show'} timeline" aria-pressed={app.panelOpen} onclick={() => (app.panelOpen = !app.panelOpen)}>
          <svg viewBox="0 0 24 24" aria-hidden="true">
            <rect x="3" y="4" width="18" height="16" rx="2" />
            {#if app.panelAt === 'bottom'}<path d="M3 14h18" />{:else}<path d="M15 4v16" />{/if}
          </svg>
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
    <main {@attach phone ? fitBoard : measureBoard}>
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
    /* The timeline column's width beside the board. --panel-w is the room
       it takes from the board area (0 when it is below or hidden), so the
       playback overlay can centre on the board rather than the window. */
    --panel-col: clamp(320px, 18vw, 960px);
    --panel-w: 0px;
    /* The gutter of page background around the mat and the timeline,
       thinner above and below on a short screen, where the mat's cards
       (sized by the root --cw) need the height. */
    --gutter: var(--space-3);
    --gutter-y: var(--gutter);
  }
  @media (max-height: 820px) { .app { --gutter-y: var(--space-1); } }
  @media (max-height: 740px) { .app { --gutter-y: 0px; } }
  .app.panel-side { --panel-w: calc(var(--panel-col) + var(--gutter)); }
  .app.reveal { border: 6px solid var(--warn-line); }
  /* With no timeline beside the board, the board takes the full width (see
     --cw in app.css). With the timeline below, the board also keeps 200px of
     height for it, and the timeline fills whatever height the board leaves. */
  .app.wide {
    --cw: clamp(48px, min((100vh - 140px) / 11.1, (100vw - 64px) / 10), 240px);
    --fs: clamp(var(--fs-12), calc(var(--cw) * 0.15), var(--fs-28));
  }
  .app.panel-bottom {
    height: 100vh;
    --cw: clamp(48px, min((100vh - 340px) / 11.1, (100vw - 64px) / 10), 240px);
  }
  /* The landscape mat (app.wide) fills the window and sizes its cards
     from the board area: <main> is a size container, and .board inside it
     takes the largest card width at which the four rows fit main's height
     and the widest row (the longer bench, the prize fan and the piles)
     fits its width. */
  /* The landscape mat gives its cards the room of a thinner gutter. */
  .app.landscape { height: 100vh; min-height: 0; --gutter: var(--space-1); --gutter-y: var(--space-1); }
  .app.landscape main { container-type: size; }
  .app.landscape .body.panel.bottom { grid-template-rows: minmax(0, 1fr) clamp(200px, 28vh, 480px); }
  /* The rows are about 6.8 card widths tall: two player rows, the middle
     row with its captions, your hand and the opponent's peeking hand above
     them; plus your name. A busy board (Energy under benched Pokémon) gives
     up --fit (see fitBoard). */
  .board {
    --cw: max(48px, min((100cqh - 50px - var(--fit, 0px)) / 6.9, (100cqw - 80px) / (4.6 + 1.16 * var(--bench-n, 5))));
    --fs: clamp(var(--fs-12), calc(var(--cw) * 0.15), var(--fs-28));
    font-size: var(--fs);
    position: relative;
    height: 100%;
    /* The opponent's hand peeks into this strip from above the board. */
    padding-top: calc(var(--cw) * 0.3);
    display: flex;
    flex-direction: column;
    justify-content: space-evenly;
  }
  /* The opponent's name and status, small in the top-left corner. */
  .board > .seat-name.top { position: absolute; top: 8px; left: 12px; z-index: 6; padding: 0; text-align: left; }
  /* Your hand's cards keep their size, with less felt above them. */
  .board :global(.hand:not(.small)) { min-height: 0; padding-top: calc(var(--cw) * 0.04); }
  /* Phone: the board fits the screen and never scrolls; only the hand, a
     bench of more than five and the menus do. Cards are five and a bit
     across, or smaller when the screen is short: the field (two benches
     and two Active Spots) is about 6.6 card widths tall, beside about
     372px of counters, strip, hand, padding and the Energy tucked under
     each Active. */
  .app.phone {
    --cw: max(40px, min(calc((100vw - 32px) / 5.6), calc((100dvh - 372px - var(--fit, 0px)) / 6.6)));
    --fs: var(--fs-13);
    --gutter: 0px;
    --gutter-y: 0px;
    height: 100dvh;
    min-height: 0;
  }
  .app.phone main { justify-content: flex-start; overflow: hidden; border-radius: 0; }
  /* A phone's header: a slim light bar of the dot, the turn tag and whose
     turn it is, then the warnings and ☰. */
  .app.phone header {
    flex-wrap: nowrap;
    min-height: calc(var(--space-12) + var(--space-1));
    gap: var(--space-2);
    padding: 0 var(--space-2) 0 var(--space-4);
    font-size: var(--fs-ui);
  }
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
    .app.phone header { position: absolute; top: 0; left: 0; z-index: 6; min-height: 0; gap: 6px; padding: 2px 4px 2px 8px; background: none; border: 0; }
    .app.phone header :is(.turn-tag, .whose, .spacer, .warn) { display: none; }
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
  aside.sheet .close { position: absolute; top: 4px; right: 8px; width: 44px; height: 44px; padding: 0; border-radius: 50%; }
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
    /* Header text is today's 13px up to 1920px wide and grows above it. */
    font-size: max(var(--fs-ui), 0.677vw);
  }
  header button, header select { font-size: inherit; }
  header svg { width: 1.38em; height: 1.38em; fill: none; stroke: currentColor; stroke-width: 2; stroke-linecap: round; stroke-linejoin: round; }
  .brand { display: flex; align-items: center; gap: 0.77em; }
  .brand img { width: 2.3em; height: 2.3em; display: block; }
  .brand strong { font-size: 1.3em; font-weight: 700; }
  .game { font-weight: var(--fw-ui-title); }
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
  .app.phone .dot { width: 10px; height: 10px; }
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
    font-size: max(var(--fs-ui), 0.677vw);
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
  main { overflow: auto; display: flex; flex-direction: column; justify-content: center; font-size: var(--fs); border-radius: var(--radius-l); }
  /* Today's mat keeps a margin of mat above the opponent's fanned hand and
     below yours, whose turned cards reach past their rows. */
  .app:not(.landscape, .phone) main { padding-block: var(--space-3); }
  aside {
    background: var(--panel);
    color: var(--ink);
    border: 1px solid var(--line);
    border-radius: var(--radius-l);
    display: flex;
    flex-direction: column;
    min-height: 0;
    max-height: calc(100vh - var(--header-height) - 2 * var(--gutter-y));
    position: sticky;
    top: var(--gutter-y);
    overflow: hidden;
  }
  /* Docked below, the board and the timeline join into one panel. */
  .body.panel.bottom { grid-template-columns: 1fr; grid-template-rows: auto minmax(200px, 1fr); gap: 0; }
  .body.bottom main { border-radius: var(--radius-l) var(--radius-l) 0 0; }
  .body.bottom aside {
    border-radius: 0 0 var(--radius-l) var(--radius-l);
    max-height: none;
    position: static;
  }
  aside h2 { font-size: max(var(--fs-ui), 0.677vw); font-weight: var(--fw-label); color: var(--muted); margin: var(--space-3) var(--space-3) 0; text-transform: uppercase; letter-spacing: var(--ls-label); }
  .aside-head { display: flex; align-items: center; justify-content: space-between; padding-right: var(--space-2); }
  aside .dock {
    display: grid; place-items: center; width: 28px; height: 24px; padding: 0; margin-top: var(--space-2);
    background: var(--panel); border: 1px solid var(--control-line); border-radius: var(--radius-s); color: var(--muted); cursor: pointer;
  }
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
    header .warn, .mode { min-height: var(--touch-target); }
    header .warn { min-width: var(--touch-target); justify-content: center; }
    .mode label { height: auto; }
    header button { width: var(--touch-target); height: var(--touch-target); }
    .branch select { height: var(--touch-target); }
  }
</style>
