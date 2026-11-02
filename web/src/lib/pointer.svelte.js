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
  window.addEventListener('keydown', key);
  // A finger's drag runs on Touch Events: Safari may cancel a touch's
  // pointer events part way (taking it for a scroll, or when the card it
  // started on fades out), but not touch events the page has claimed.
  if (g.type === 'touch') {
    window.addEventListener('touchmove', touchMove, { passive: false });
    window.addEventListener('touchend', touchEnd, { passive: false });
    window.addEventListener('touchcancel', cancel);
    return;
  }
  window.addEventListener('pointermove', move);
  window.addEventListener('pointerup', up);
  window.addEventListener('pointercancel', cancel);
}

// SLOP is how far a finger moves before the page claims its gesture, so a
// shaky tap still taps.
const SLOP = 4;

// mayLift reports whether a move from where the press began can lift the
// card. In a row that scrolls sideways (the hand), a finger moving sideways
// or down scrolls it, so only a move mostly upward lifts the card; in a
// pile view, which scrolls up and down, only a sideways move lifts it.
function mayLift(dx, dy) {
  if (g.type === 'mouse') return true;
  if (g.el.closest('[data-scroll-x]')) return -dy > Math.abs(dx);
  if (g.el.closest('.modal')) return Math.abs(dx) > Math.abs(dy);
  return true;
}

// touchMove follows a finger. Its first real movement claims the gesture
// for a drag, which stops Safari scrolling under it, or lets it go to
// scroll the hand or a pile view.
function touchMove(ev) {
  if (!g) return;
  const t = ev.touches[0];
  const dx = t.clientX - g.x0;
  const dy = t.clientY - g.y0;
  if (!g.ghost) {
    const dist = Math.hypot(dx, dy);
    if (dist < SLOP) return;
    if (!mayLift(dx, dy)) {
      end();
      return;
    }
    ev.preventDefault();
    if (dist < THRESHOLD) return;
    g.x = t.clientX;
    g.y = t.clientY;
    start();
    return;
  }
  ev.preventDefault();
  g.x = t.clientX;
  g.y = t.clientY;
  if (!g.frame) g.frame = requestAnimationFrame(track);
}

function touchEnd(ev) {
  if (!g) return;
  if (!g.ghost) {
    // A tap: its click opens the card's actions.
    end();
    return;
  }
  ev.preventDefault();
  const t = ev.changedTouches[0];
  release(t.clientX, t.clientY);
}

function move(ev) {
  if (!g || ev.pointerId !== g.id) return;
  g.x = ev.clientX;
  g.y = ev.clientY;
  if (!g.ghost) {
    const dx = g.x - g.x0;
    const dy = g.y - g.y0;
    if (Math.hypot(dx, dy) < THRESHOLD) return;
    if (!mayLift(dx, dy)) {
      end();
      return;
    }
    start();
  }
  ev.preventDefault();
  if (!g.frame) g.frame = requestAnimationFrame(track);
}

function start() {
  // A mouse or pen is captured by the document, not the card: a captured
  // card would keep its hover enlargement and sit over the zones it is
  // dragged to. A touch is already captured by the card it started on, and
  // moving that capture can make Safari cancel the touch.
  if (g.type !== 'touch') {
    try {
      document.documentElement.setPointerCapture(g.id);
    } catch {
      // Not every pointer can be captured; the window listeners still follow it.
    }
  }
  const r = g.el.getBoundingClientRect();
  const ghost = g.el.cloneNode(true);
  ghost.classList.add('drag-ghost');
  ghost.removeAttribute('data-drop');
  // Placed inline, where no card style can override it: a copy of the
  // card fixed over the page, following the pointer.
  Object.assign(ghost.style, {
    position: 'fixed',
    left: '0',
    top: '0',
    margin: '0',
    zIndex: '100',
    pointerEvents: 'none',
    transition: 'none',
    width: `${r.width}px`,
    height: `${r.height}px`,
  });
  // Card sizes follow --cw, which the board sets below body.
  ghost.style.setProperty('--cw', getComputedStyle(g.el).getPropertyValue('--cw'));
  document.body.append(ghost);
  // A finger would hide the card, so its ghost rides half a card above it.
  g.ghost = ghost;
  g.home = r;
  g.dx = r.width / 2;
  g.dy = g.type === 'mouse' ? r.height / 2 : r.height;
  g.el.classList.add('drag-source');
  document.body.classList.add('dragging');
  app.dragging = g.payload;
  track();
  // The card lifts off the table.
  ghost.animate([{ scale: 1 }, { scale: 1.12 }], { duration: 120, easing: 'ease-out', fill: 'forwards' });
}

// track moves the ghost and lights the zone under the pointer, at most
// once a frame.
function track() {
  if (!g?.ghost) return;
  g.frame = 0;
  // translate, not transform: the lift's scale applies after transform
  // and would scale the offset too, carrying the ghost off the pointer.
  g.ghost.style.translate = `${g.x - g.dx}px ${g.y - g.dy}px`;
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
  release(ev.clientX, ev.clientY);
}

// release drops the card being dragged at a point.
function release(x, y) {
  let zone = zoneAt(x, y);
  const payload = g.payload;
  // A card dropped back on the pile it came from stays where it was.
  if (zone && samePlace(zone.dataset.drop, payload.from)) zone = null;
  // The ghost settles into the zone it lands on, or flies home on a miss.
  fly(g.ghost, zone ? zone.getBoundingClientRect() : g.home, !!zone);
  g.ghost = null;
  end();
  // The click that follows the release would zoom or open what lies
  // under it; a drag is not a click.
  window.addEventListener('click', swallow, { capture: true, once: true });
  setTimeout(() => window.removeEventListener('click', swallow, { capture: true }));
  if (zone) handlers.get(zone)(payload);
}

// samePlace reports whether a drop zone is the place a card came from: its
// own Pokémon slot, or its own hand, discard or Lost Zone. (A deck card
// dropped on the deck goes on top, a real move.)
function samePlace(spec, from) {
  if (!from) return false;
  return spec === from || (['hand', 'discard', 'lost'].includes(spec) && from.endsWith(`:${spec}`));
}

// fly moves a released ghost onto a rectangle, centred, then removes it:
// into the zone it was dropped on (fading as it lands), or back home.
function fly(ghost, rect, landed) {
  const w = ghost.offsetWidth;
  const h = ghost.offsetHeight;
  const to = `${rect.left + rect.width / 2 - w / 2}px ${rect.top + rect.height / 2 - h / 2}px`;
  const anim = ghost.animate(
    [
      { translate: ghost.style.translate, scale: 1.12, opacity: 1 },
      { translate: to, scale: landed ? 0.9 : 1, opacity: landed ? 0 : 0.6 },
    ],
    { duration: landed ? 160 : 220, easing: 'ease-in-out', fill: 'forwards' },
  );
  anim.onfinish = anim.oncancel = () => ghost.remove();
}

function cancel(ev) {
  if (g && (ev.type === 'touchcancel' || ev.pointerId === g.id)) end();
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
  window.removeEventListener('touchmove', touchMove);
  window.removeEventListener('touchend', touchEnd);
  window.removeEventListener('touchcancel', cancel);
  if (g.frame) cancelAnimationFrame(g.frame);
  // A drag cancelled before release (Escape, a lost pointer) flies home.
  if (g.ghost) fly(g.ghost, g.home, false);
  if (g.home) {
    g.zone?.classList.remove('drop-lit');
    g.el.classList.remove('drag-source');
    document.body.classList.remove('dragging');
    app.dragging = null;
  }
  g = null;
}
