// Shared reactive UI state. The server is authoritative: everything here is
// either a copy of what it pushed or purely local presentation state.
export const app = $state({
  connected: false,
  // Signed-in account's email on the hosted playmat; null on the desktop.
  account: null,
  // Set while a sign-out is on its way, so the closing live connection
  // does not send the user straight back through sign-in.
  signingOut: false,
  human: 'p1',
  claude: 'p2',
  // Name of the assistant playing the claude seat over MCP (Claude, Codex, Gemini).
  assistant: 'Assistant',
  reveal: false,
  // Latest snapshot from the server (game, branch, mode, board, log, ...).
  snap: null,
  // Board being displayed; differs from snap.board while animating frames.
  board: null,
  // Card definitions by TCGdex id, fetched lazily.
  cards: {},
  highlight: [],
  caption: '',
  toasts: [],
  panelOpen: true,
  menu: null, // {x, y, title, items}
  modal: null, // {kind, ...}
  zoom: null, // card view
  dragging: null,
  // Playback (see playback.svelte.js): viewHead is the event the board
  // shows; held marks moves by Claude not yet played; replay is a replay
  // in progress.
  viewHead: 0,
  held: null,
  replay: null,
  replaySpeed: 1,
  acting: false, // a request of the viewer's own is in flight
  deckListShow: 'all', // deck list filter: all, unplayed or undrawn
  lastLog: 0, // highest log id seen, to mark new lines
});

let toastId = 0;

// toast shows a transient message: kind is error, warn or info.
export function toast(text, kind = 'info') {
  const id = ++toastId;
  app.toasts.push({ id, text, kind });
  const ttl = kind === 'error' ? 7000 : kind === 'warn' ? 5500 : 3500;
  setTimeout(() => {
    const i = app.toasts.findIndex((t) => t.id === id);
    if (i >= 0) app.toasts.splice(i, 1);
  }, ttl);
}

const requested = new Set();

// cardInfo returns a card definition, fetching it on first use.
export function cardInfo(def) {
  if (!def) return null;
  if (app.cards[def]) return app.cards[def];
  if (!requested.has(def)) {
    requested.add(def);
    // no-cache also skips responses cached under an older max-age.
    fetch(`/api/card?def=${encodeURIComponent(def)}`, { cache: 'no-cache' })
      .then((r) => (r.ok ? r.json() : null))
      .then((c) => {
        if (c) app.cards[def] = c;
      })
      .catch(() => {});
  }
  return null;
}

// ownerOf returns the seat that owns a card from its iid ("p1-12" -> p1).
export function ownerOf(iid) {
  return String(iid).split('-')[0];
}

// isMyTurn reports whether the human may act now.
export function canAct() {
  const b = app.snap?.board;
  if (!b) return false;
  if (app.snap.mode === 'review') return true;
  return b.phase === 'setup' || (b.turn_open && b.current === app.human);
}
