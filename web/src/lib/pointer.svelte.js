// One drag controller for mouse, pen and touch, on Pointer Events. Cards
// and stacks opt in with the draggable attachment and drop places with
// dropZone; a drop calls the zone's handler with today's payload, so what
// each drop does stays in dropOnStack and dropOnZone.
import { app } from './store.svelte.js';

// THRESHOLD is how far the pointer moves before a press becomes a drag;
// anything shorter is a tap, left to the element's click handler.
const THRESHOLD = 8;

// The gesture in progress: the pointer that went down on a draggable, and
// once it has moved far enough, the ghost and the zone under it.
let g = null;
const handlers = new WeakMap();

// draggable makes an element draggable with payload ({kind, iid, def,
// from, role} for a card, {kind, from} for a stack). A falsy payload
// leaves the element inert, so `{@attach draggable(cond && payload)}`
// turns dragging on and off.
export function draggable(payload) {
  if (!payload) return undefined;
  return (el) => {
    const down = (ev) => press(ev, el, payload);
    el.addEventListener('pointerdown', down);
    return () => el.removeEventListener('pointerdown', down);
  };
}

// dropZone marks an element as a place to drop on: data-drop carries its
// spec ("p1:bench:2", "discard", "play", ...) and onDrop gets the payload.
export function dropZone(spec, onDrop) {
  return (el) => {
    el.dataset.drop = spec;
    handlers.set(el, onDrop);
    return () => {
      delete el.dataset.drop;
      handlers.delete(el);
    };
  };
}

function press(ev, el, payload) {
  // The innermost draggable takes the press (an Energy, not its stack).
  // While an action waits for its target, a press is a choice, not a drag.
  if (g || app.target || (ev.pointerType === 'mouse' && ev.button !== 0)) return;
  g = { id: ev.pointerId, type: ev.pointerType, el, payload, x0: ev.clientX, y0: ev.clientY, x: ev.clientX, y: ev.clientY };
  window.addEventListener('pointermove', move);
  window.addEventListener('pointerup', up);
  window.addEventListener('pointercancel', cancel);
  window.addEventListener('keydown', key);
}

function move(ev) {
  if (!g || ev.pointerId !== g.id) return;
  g.x = ev.clientX;
  g.y = ev.clientY;
  if (!g.ghost) {
    const dx = g.x - g.x0;
    const dy = g.y - g.y0;
    if (Math.hypot(dx, dy) < THRESHOLD) return;
    // In a row that scrolls sideways (the hand), a finger moving sideways
    // or down scrolls it; only a move mostly upward lifts the card.
    if (g.type !== 'mouse' && g.el.closest('[data-scroll-x]') && !(-dy > Math.abs(dx))) {
      end();
      return;
    }
    start();
  }
  ev.preventDefault();
  if (!g.frame) g.frame = requestAnimationFrame(track);
}

function start() {
  // The document, not the card, captures the pointer: a captured card
  // would keep its hover enlargement and sit over the zones it is dragged to.
  try {
    document.documentElement.setPointerCapture(g.id);
  } catch {
    // Not every pointer can be captured; the window listeners still follow it.
  }
  const r = g.el.getBoundingClientRect();
  const ghost = g.el.cloneNode(true);
  ghost.classList.add('drag-ghost');
  ghost.removeAttribute('data-drop');
  ghost.style.width = `${r.width}px`;
  ghost.style.height = `${r.height}px`;
  // Card sizes follow --cw, which the board sets below body.
  ghost.style.setProperty('--cw', getComputedStyle(g.el).getPropertyValue('--cw'));
  document.body.append(ghost);
  // A finger would hide the card, so its ghost rides half a card above it.
  g.ghost = ghost;
  g.dx = r.width / 2;
  g.dy = g.type === 'mouse' ? r.height / 2 : r.height;
  g.el.classList.add('drag-source');
  document.body.classList.add('dragging');
  app.dragging = g.payload;
  track();
}

// track moves the ghost and lights the zone under the pointer, at most
// once a frame.
function track() {
  if (!g?.ghost) return;
  g.frame = 0;
  g.ghost.style.transform = `translate(${g.x - g.dx}px, ${g.y - g.dy}px)`;
  const zone = zoneAt(g.x, g.y);
  if (zone !== g.zone) {
    g.zone?.classList.remove('drop-lit');
    zone?.classList.add('drop-lit');
    g.zone = zone;
  }
}

function zoneAt(x, y) {
  const el = document.elementFromPoint(x, y)?.closest('[data-drop]');
  return el && handlers.has(el) ? el : null;
}

function up(ev) {
  if (!g || ev.pointerId !== g.id) return;
  if (!g.ghost) {
    end();
    return;
  }
  const zone = zoneAt(ev.clientX, ev.clientY);
  const payload = g.payload;
  end();
  // The click that follows the release would zoom or open what lies
  // under it; a drag is not a click.
  window.addEventListener('click', swallow, { capture: true, once: true });
  setTimeout(() => window.removeEventListener('click', swallow, { capture: true }));
  if (zone) handlers.get(zone)(payload);
}

function cancel(ev) {
  if (g && ev.pointerId === g.id) end();
}

function key(ev) {
  if (ev.key === 'Escape' && g?.ghost) end();
}

function swallow(ev) {
  ev.stopPropagation();
  ev.preventDefault();
}

function end() {
  window.removeEventListener('pointermove', move);
  window.removeEventListener('pointerup', up);
  window.removeEventListener('pointercancel', cancel);
  window.removeEventListener('keydown', key);
  if (g.frame) cancelAnimationFrame(g.frame);
  if (g.ghost) {
    g.ghost.remove();
    g.zone?.classList.remove('drop-lit');
    g.el.classList.remove('drag-source');
    document.body.classList.remove('dragging');
    app.dragging = null;
  }
  g = null;
}
