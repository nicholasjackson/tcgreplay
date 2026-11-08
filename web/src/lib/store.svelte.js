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
  // Why there is no game, when the server says (an expired game); '' otherwise.
  notice: '',
  // What the assistant is doing: 'waiting' for the human's turn, 'acting'
  // (its own turn: the board is locked), 'deciding' (it answers a decision;
  // the human keeps playing what does not depend on it), 'stopped'
  // responding, or ''.
  opponent: '',
  // The decision it is making ('promote' or 'answer_card') while deciding,
  // or the one it left unanswered when it stopped; '' otherwise.
  decision: '',
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
  // Where the timeline sits: 'side' (right of the board) or 'bottom'
  // (below it, for tall screens). Remembered in this browser.
  panelAt: 'side',
  // The open action sheet: build() returns the menu model (see
  // actions.js), re-run as the board changes; pointerType picks its
  // container and x, y place the desktop popover.
  sheet: null, // {build, pointerType, x, y}
  // An action waiting for its destination to be tapped (lib/target.js).
  target: null, // {prompt, hint, card, candidates, run}
  // The screen: 'phone' (narrower than 640px), 'tablet' (a touch screen)
  // or 'desktop'; landscapePhone is a phone held sideways.
  layout: 'desktop',
  landscapePhone: false,
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
  // Play the assistant's moves on the board as they arrive, rather than
  // holding them for the playback bar. Remembered in this browser.
  autoplay: true,
  // Show "Knocked Out: take N prize cards" after a knock-out. Off for
  // practising without the hint. Remembered in this browser.
  prizeReminders: true,
  acting: false, // a request of the viewer's own is in flight
  deckListShow: 'all', // deck list filter: all, unplayed or undrawn
  lastLog: 0, // highest log id seen, to mark new lines
});

try {
  if (localStorage.getItem('panelAt') === 'bottom') app.panelAt = 'bottom';
  if (localStorage.getItem('prizeReminders') === 'off') app.prizeReminders = false;
} catch {
  // storage unavailable: keep the default
}

// The layout follows the viewport's width and the primary pointer.
const narrow = matchMedia('(max-width: 639px)');
const coarse = matchMedia('(pointer: coarse)');
// A phone held sideways is under 500px tall; a tablet never is.
const sideways = matchMedia('(orientation: landscape) and (max-height: 499px) and (pointer: coarse)');
function setLayout() {
  app.layout = narrow.matches ? 'phone' : coarse.matches ? 'tablet' : 'desktop';
  app.landscapePhone = sideways.matches;
}
for (const q of [narrow, coarse, sideways]) q.addEventListener('change', setLayout);
setLayout();

// lastPointer is the kind of pointer that last went down, and when. A
// click's own pointerType can't be trusted: iOS Safari reports a tap's
// click as "mouse", and older browsers report nothing.
let lastPointer = { type: 'mouse', at: 0 };
addEventListener('pointerdown', (ev) => (lastPointer = { type: ev.pointerType, at: ev.timeStamp }), { capture: true });

// pointerTypeOf is the kind of pointer behind an event (mouse, pen or
// touch): the pointer that just went down, else what the event says.
export function pointerTypeOf(ev) {
  if (ev.timeStamp - lastPointer.at < 1500) return lastPointer.type;
  return ev.pointerType || 'mouse';
}

// openSheet opens a card's actions from the event that asked for them: a
// tap gets the phone sheet or tablet panel, a mouse the popover.
export function openSheet(ev, build, pointerType = pointerTypeOf(ev)) {
  app.sheet = { build, pointerType, x: ev.clientX, y: ev.clientY };
}

// setPrizeReminders turns the knock-out prize reminder on or off.
export function setPrizeReminders(on) {
  app.prizeReminders = on;
  try {
    localStorage.setItem('prizeReminders', on ? 'on' : 'off');
  } catch {
    // storage unavailable: the choice lasts for this page only
  }
}

// setPanelAt moves the timeline to the side or the bottom.
export function setPanelAt(at) {
  app.panelAt = at;
  try {
    localStorage.setItem('panelAt', at);
  } catch {
    // storage unavailable: the choice lasts for this page only
  }
}

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

// A card image is tried in stages, one per candidate address, in the
// order info.images lists its sources. A base-path entry gives its address
// with the size file appended (with ?v=<image_version> when the server
// gives a copy's content version, so changed art gets a new address past
// browser and CDN caches), then its image_fallback with the size file (the
// card source, when the address is a CDN copy). A file entry
// (image_file) gives its address as is, for every size. When an image
// fails to load the board moves to the next stage; past the last one
// nothing is left to load, so it shows its text stand-in or Energy token.
// Each image spot keeps its own stage and resets it to 0 when it shows
// another card.

// imageCandidates lists the addresses to try for info's image at size
// (low.webp or high.webp), in order.
export function imageCandidates(info, size) {
  const out = [];
  for (const img of info?.images ?? []) {
    if (!img?.image) continue;
    if (img.image_file) {
      out.push(img.image);
      continue;
    }
    const v = img.image_version ? `?v=${encodeURIComponent(img.image_version)}` : '';
    out.push(`${img.image}/${size}${v}`);
    if (img.image_fallback) out.push(`${img.image_fallback}/${size}`);
  }
  return out;
}

// imageSrc is the address of the image file at size for info at stage, or
// null when the stage has nothing to load.
export function imageSrc(info, size, stage) {
  return imageCandidates(info, size)[stage] ?? null;
}

// nextImageStage is the stage after the image at stage failed to load.
// Every size has the same number of candidates, so the stage counts the
// same for the mat, the zoom and the preview.
export function nextImageStage(info, stage) {
  return stage + 1;
}

// ownerOf returns the seat that owns a card from its iid ("p1-12" -> p1).
export function ownerOf(iid) {
  return String(iid).split('-')[0];
}

// oppActing reports whether the assistant is acting, so the server refuses
// the human's game actions until it finishes.
export function oppActing() {
  return app.opponent === 'acting';
}

// isMyTurn reports whether the human may act now.
export function canAct() {
  const b = app.snap?.board;
  if (!b || oppActing()) return false;
  if (app.snap.mode === 'review') return true;
  return b.phase === 'setup' || (b.turn_open && b.current === app.human);
}
