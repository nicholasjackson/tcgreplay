// Playback. The server is authoritative and pushes snapshots; the board
// shows the latest one, except:
// - Moves made by Claude (or the system) on the branch being watched are
//   held, so the board does not jump. With autoplay on (the default) they
//   play out on their own; with it off, the playback bar offers to play them.
// - A replay steps through moves one action at a time, at a chosen speed,
//   with highlights and captions; it can be paused, stepped and scrubbed.
import { app } from './store.svelte.js';
import { request } from './ws.js';

// BASE_MS is how long one move shows at 1×; moves with notes stay longer.
const BASE_MS = 1400;
const NOTE_FACTOR = 2.5;
let timer = null;

export const speeds = [0.5, 1, 2, 4];
try {
  const s = Number(localStorage.getItem('replaySpeed'));
  if (speeds.includes(s)) app.replaySpeed = s;
} catch {
  // storage unavailable: keep the default
}

export function setSpeed(s) {
  app.replaySpeed = s;
  try {
    localStorage.setItem('replaySpeed', String(s));
  } catch {
    // storage unavailable: the choice lasts for this page only
  }
}

try {
  if (localStorage.getItem('autoplay') === 'off') app.autoplay = false;
} catch {
  // storage unavailable: keep the default
}

// setAutoplay turns autoplay of the assistant's moves on or off; turned on,
// it plays any moves already held.
export function setAutoplay(on) {
  app.autoplay = on;
  try {
    localStorage.setItem('autoplay', on ? 'on' : 'off');
  } catch {
    // storage unavailable: the choice lasts for this page only
  }
  autoplayHeld();
}

// autoStarting is set while an autoplay replay is being fetched, so moves
// arriving meanwhile do not start a second one.
let autoStarting = false;

// autoplayHeld plays the held moves when autoplay is on.
function autoplayHeld() {
  if (!app.autoplay || !app.held || app.replay || autoStarting) return;
  autoStarting = true;
  startReplay(app.held.from, 0, true).finally(() => (autoStarting = false));
}

export function showSnapshot(snap) {
  const prev = app.snap;
  app.snap = snap;
  if (!snap) {
    stopTimer();
    app.board = null;
    app.replay = null;
    app.held = null;
    app.viewHead = 0;
    return;
  }
  const sameBranch = prev && prev.game === snap.game && prev.branch === snap.branch;
  // A replay owns the board until it ends or the viewer leaves it; moves
  // arriving meanwhile are held when it ends. A different game or branch
  // ends it.
  if (app.replay) {
    if (!sameBranch) goLive();
    return;
  }
  const frames = snap.frames || [];
  // The system's moves that follow the viewer's own (the next turn's start
  // and draw after End turn) are shown at once, like the viewer's moves.
  const mine = (f) => f.actor === 'human' || (f.actor === 'system' && app.acting);
  const theirs = frames.length ? frames.some((f) => !mine(f)) : !app.acting;
  if (sameBranch && snap.head > app.viewHead && theirs) {
    app.held = { from: app.viewHead, to: snap.head };
    autoplayHeld();
    return;
  }
  app.board = snap.board;
  app.viewHead = snap.head;
  app.held = null;
}

// heldMoves counts the timeline lines waiting to be played.
export function heldMoves() {
  if (!app.held || !app.snap) return 0;
  return app.snap.log.filter((l) => l.id > app.held.from).length;
}

// startReplay plays the current branch from just after event from to event
// until (0 for the head). auto marks an autoplay of held moves, which the
// viewer can skip but not scrub.
export async function startReplay(from, until = 0, auto = false) {
  stopTimer();
  let r;
  try {
    r = await request('replay', { from, until });
  } catch (e) {
    return;
  }
  if (!r.ok || !r.data?.steps?.length) return;
  // The viewer went live (or acted) while an autoplay was being fetched.
  if (auto && !app.held) return;
  app.held = null;
  app.replay = { ...r.data, index: -1, playing: false, auto };
  show(-1);
  play();
}

export function play() {
  const r = app.replay;
  if (!r) return;
  if (r.index >= r.steps.length - 1) show(-1); // at the end: play again
  r.playing = true;
  schedule(BASE_MS / app.replaySpeed / 2);
}

export function pause() {
  if (app.replay) app.replay.playing = false;
  stopTimer();
}

export function step(delta) {
  const r = app.replay;
  if (!r) return;
  pause();
  show(Math.max(-1, Math.min(r.steps.length - 1, r.index + delta)));
}

export function seek(index) {
  if (!app.replay) return;
  pause();
  show(index);
}

// goLive leaves any replay or held moves and shows the latest board.
export function goLive() {
  stopTimer();
  app.replay = null;
  app.held = null;
  app.highlight = [];
  app.caption = '';
  if (app.snap) {
    app.board = app.snap.board;
    app.viewHead = app.snap.head;
  }
}

function show(index) {
  const r = app.replay;
  r.index = index;
  if (index < 0) {
    app.board = r.start;
    app.viewHead = r.from;
    app.highlight = [];
    app.caption = '';
    return;
  }
  const s = r.steps[index];
  app.board = s.board;
  app.viewHead = s.last;
  app.highlight = s.touched || [];
  const who = app.board.sides?.[s.seat]?.name || s.seat || '';
  const what = s.label || s.lines[0] || '';
  app.caption = [who && what ? `${who}: ${what}` : what, ...(s.notes || [])].join('\n');
}

function schedule(ms) {
  stopTimer();
  timer = setTimeout(tick, ms);
}

function tick() {
  const r = app.replay;
  if (!r?.playing) return;
  if (r.index >= r.steps.length - 1) {
    r.playing = false;
    const last = r.steps.at(-1).last;
    if (app.snap && app.snap.head > last) {
      // Moves arrived during the replay: play or offer them next.
      app.replay = null;
      app.held = { from: last, to: app.snap.head };
      autoplayHeld();
      return;
    }
    // Played up to the latest move: rejoin the live board.
    setTimeout(() => app.replay && !app.replay.playing && goLive(), BASE_MS);
    return;
  }
  show(r.index + 1);
  const notes = r.steps[r.index].notes?.length;
  schedule((BASE_MS / app.replaySpeed) * (notes ? NOTE_FACTOR : 1));
}

function stopTimer() {
  if (timer) clearTimeout(timer);
  timer = null;
}
