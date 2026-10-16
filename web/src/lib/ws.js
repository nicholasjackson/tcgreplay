// Websocket client: one connection, auto-reconnect, request/response by id.
import { app, toast } from './store.svelte.js';
import { goLive, showSnapshot } from './playback.svelte.js';

let sock = null;
let nextId = 1;
let backoff = 500;
const pending = new Map();

export function connect() {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws';
  sock = new WebSocket(`${proto}://${location.host}/ws`);
  sock.onopen = () => {
    app.connected = true;
    backoff = 500;
  };
  sock.onclose = () => {
    app.connected = false;
    for (const [, p] of pending) p.reject(new Error('disconnected'));
    pending.clear();
    setTimeout(connect, backoff);
    backoff = Math.min(backoff * 2, 5000);
  };
  sock.onmessage = (ev) => {
    let msg;
    try {
      msg = JSON.parse(ev.data);
    } catch {
      return;
    }
    if (msg.type === 'state') onState(msg);
    else if (msg.type === 'result') onResult(msg);
  };
}

function onState(msg) {
  app.human = msg.human;
  app.claude = msg.claude;
  app.reveal = !!msg.reveal;
  app.games = msg.games || [];
  if (msg.error) toast(msg.error, 'error');
  showSnapshot(msg.snapshot);
}

function onResult(msg) {
  const p = pending.get(msg.id);
  if (!p) {
    if (msg.error) toast(msg.error, 'error');
    return;
  }
  pending.delete(msg.id);
  p.resolve(msg);
}

// request sends a verb and resolves with the server's result.
export function request(verb, args = {}) {
  if (!sock || sock.readyState !== WebSocket.OPEN) {
    return Promise.reject(new Error('not connected'));
  }
  const id = nextId++;
  sock.send(JSON.stringify({ id, verb, args }));
  return new Promise((resolve, reject) => pending.set(id, { resolve, reject }));
}

const READS = new Set(['deck_list', 'search_deck', 'look_top', 'list_games']);

// act sends a verb, shows errors and warnings as toasts, and returns the
// result's data (or undefined on error).
export async function act(verb, args = {}) {
  // Changing the board from an earlier position would be confusing: show
  // the latest board first. Reads leave a replay alone.
  const read = READS.has(verb);
  if (!read && (app.replay || app.held)) goLive();
  if (!read) app.acting = true;
  try {
    const r = await request(verb, args);
    if (!r.ok) {
      toast(r.error || `${verb} failed`, 'error');
      return undefined;
    }
    for (const w of r.warnings || []) toast(w, 'warn');
    return r.data ?? true;
  } catch (e) {
    toast(e.message, 'error');
    return undefined;
  } finally {
    // Let the resulting snapshot arrive before clearing.
    setTimeout(() => (app.acting = false), 300);
  }
}
