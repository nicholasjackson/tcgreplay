// Playback of pushed snapshots. Changes made by someone other than the
// human (Claude, the system) step through one event at a time with a
// highlight and caption, so the board never jumps.
import { app } from './store.svelte.js';

const STEP_MS = 550;
const queue = [];
let running = false;

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

export function showSnapshot(snap) {
  queue.push(snap);
  if (!running) drain();
}

async function drain() {
  running = true;
  while (queue.length) {
    const snap = queue.shift();
    if (!snap) {
      app.snap = null;
      app.board = null;
      continue;
    }
    const frames = (snap.frames || []).filter((f) => f.actor !== 'human');
    // Keep the previous snapshot's meta while frames play, so controls do
    // not flicker to the final state early.
    for (const f of frames) {
      if (queue.length > 3) break; // falling behind: skip to the end
      app.board = f.board;
      app.highlight = f.touched || [];
      app.caption = f.text;
      await sleep(STEP_MS);
    }
    app.snap = snap;
    app.board = snap.board;
    if (frames.length) {
      await sleep(STEP_MS);
      app.caption = '';
      app.highlight = [];
    }
  }
  running = false;
}
