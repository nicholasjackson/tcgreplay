// Target mode: an action that needs a destination ends on the board. The
// candidates glow, a banner names the action, and the next tap or click
// either lands on a candidate, which runs the action, or cancels it.
import { app } from './store.svelte.js';

// targetItem is a menu item for an action with candidates ({spec, name,
// iid} of the Pokémon it can go to). One candidate runs at once, labelled
// with one(name); several enter target mode; none leaves the item out.
export function targetItem({ label, one, prompt, hint, card, candidates, run }) {
  if (!candidates.length) return null;
  if (candidates.length === 1) return { label: one(candidates[0].name), run: () => run(candidates[0].spec) };
  return { label, target: { prompt, hint, card, candidates, run } };
}

// startTarget enters target mode for a request from targetItem, or one of
// a caller's own ({prompt, hint, card, candidates, run, done}); done, if
// given, runs once target mode ends, whether or not a candidate was chosen.
export function startTarget(req) {
  app.target = req;
  app.highlight = req.candidates.map((c) => c.iid);
  document.body.classList.add('targeting');
}

// endTarget leaves target mode with nothing more done.
export function endTarget() {
  const t = app.target;
  app.target = null;
  app.highlight = [];
  document.body.classList.remove('targeting');
  t?.done?.();
}

// While targeting, every click is the choice: a candidate runs the action,
// anything else (but the banner's Cancel) ends target mode, and neither
// reaches what was tapped.
addEventListener(
  'click',
  (ev) => {
    const t = app.target;
    if (!t || ev.target.closest('.target-banner')) return;
    ev.stopPropagation();
    ev.preventDefault();
    const spec = ev.target.closest('[data-drop]')?.dataset.drop;
    if (t.candidates.some((c) => c.spec === spec)) t.run(spec);
    endTarget();
  },
  { capture: true },
);
addEventListener(
  'contextmenu',
  (ev) => {
    if (!app.target) return;
    ev.stopPropagation();
    ev.preventDefault();
    endTarget();
  },
  { capture: true },
);
// Escape ends target mode and nothing else: a sheet waiting behind it
// stays open.
addEventListener('keydown', (ev) => {
  if (ev.key !== 'Escape' || !app.target) return;
  ev.stopImmediatePropagation();
  endTarget();
});
