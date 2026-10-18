<script>
  import { openDeckList } from './lib/actions.js';
  import { app } from './lib/store.svelte.js';
  import { act } from './lib/ws.js';
  import Side from './components/Side.svelte';
  import Hand from './components/Hand.svelte';
  import Strip from './components/Strip.svelte';
  import Timeline from './components/Timeline.svelte';
  import PlaybackBar from './components/PlaybackBar.svelte';
  import ContextMenu from './components/ContextMenu.svelte';
  import Modal from './components/Modal.svelte';
  import Zoom from './components/Zoom.svelte';

  const snap = $derived(app.snap);
  const review = $derived(snap?.mode === 'review');

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
  function nameOf(seat) {
    const n = app.board?.sides[seat]?.name;
    return n || (seat === app.human ? ownName() || 'You' : app.assistant);
  }
</script>

<div class="app" class:reveal={app.reveal}>
  <header>
    <span class="dot" class:on={app.connected} title={app.connected ? 'Connected' : 'Reconnecting…'}></span>
    <strong>tcgreplay</strong>
    {#if snap}
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
      {#if snap.warnings?.length}
        <span class="warn" title={snap.warnings.join('\n')}>⚠ {snap.warnings.length}</span>
      {/if}
    {/if}
    {#if snap}
      <button class="decklist" onclick={() => openDeckList(app.human)} title="Every card in your deck and where each copy is">Deck list</button>
    {/if}
    <button class="panel-toggle" onclick={() => (app.panelOpen = !app.panelOpen)}>{app.panelOpen ? 'Hide' : 'Show'} timeline</button>
    {#if app.account}
      <span class="account" title="Signed in with Google">{app.account}</span>
      <form class="signout" method="POST" action="/auth/logout" onsubmit={() => (app.signingOut = true)}><button type="submit">Sign out</button></form>
    {/if}
  </header>

  {#if app.reveal}
    <div class="reveal-banner">REVEAL VIEW: both hands, prizes and decks are face up. Post-mortem only.</div>
  {/if}

  <div class="body" class:panel={app.panelOpen}>
    <main>
      {#if app.board}
        <div class="seat-name top">{nameOf(app.claude)} · {app.claude}</div>
        <Hand seat={app.claude} small />
        <Side seat={app.claude} mirrored />
        <Strip />
        <Side seat={app.human} />
        <Hand seat={app.human} />
        <div class="seat-name">{nameOf(app.human)} · {app.human}</div>
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
    {#if app.panelOpen}
      <aside>
        <h2>Timeline</h2>
        <Timeline />
      </aside>
    {/if}
  </div>

  <PlaybackBar />
  {#if app.caption}
    <div class="caption">{app.caption}</div>
  {/if}

  <div class="toasts" aria-live="polite">
    {#each app.toasts as t (t.id)}
      <div class="toast {t.kind}">{t.text}</div>
    {/each}
  </div>

  <ContextMenu />
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
  .panel-toggle { margin-left: auto; }
  .account { color: var(--muted); }
  .signout { margin: 0; }
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
  aside h2 { font-size: 13px; margin: 10px 12px 0; text-transform: uppercase; letter-spacing: 0.08em; }
  .seat-name { text-align: center; color: var(--muted); font-size: 0.85em; padding: 0.15em; }
  .landing { text-align: center; padding: 60px 20px; display: flex; flex-direction: column; gap: 8px; align-items: center; }
  .muted { color: var(--muted); }
  .caption {
    position: fixed;
    top: 112px;
    white-space: pre-line;
    font-size: 15px;
    line-height: 1.45;
    left: 50%;
    transform: translateX(-50%);
    background: #000d;
    color: #fff;
    padding: 8px 16px;
    border-radius: 20px;
    border: 2px solid var(--accent);
    z-index: 20;
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
