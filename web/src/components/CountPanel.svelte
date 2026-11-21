<script>
  // Draw N or Look at top N: a count from 1 up to the cards left in the
  // deck, then Go.
  import { app } from '../lib/store.svelte.js';
  import { seatArgs } from '../lib/actions.js';
  import { act } from '../lib/ws.js';
  import Stepper from './Stepper.svelte';

  let { verb, seat, close } = $props();

  const max = $derived(app.board?.sides[seat]?.deck_count ?? 0);
  let n = $state(verb === 'look' ? 3 : 1);
  const count = $derived(Math.min(n, max));

  // go reads the panel before closing the sheet, which unmounts it.
  async function go() {
    const [whose, n, sa] = [seat, count, seatArgs(seat)];
    close();
    if (verb === 'draw') return act('draw', { n, ...sa });
    const cards = await act('look_top', { n, whose, ...sa });
    if (cards) app.modal = { kind: 'look', seat: whose, n, cards };
  }
</script>

<section class="sec" role="group" aria-label={verb === 'draw' ? 'Cards to draw' : 'Cards to look at'}>
  <div class="label">{verb === 'draw' ? 'Cards to draw' : 'Cards to look at from the top'}</div>
  <Stepper value={count} step={1} min={1} {max} label={verb === 'draw' ? 'Cards to draw' : 'Cards to look at'} set={(v) => (n = v)} />
  <p class="left"><span class="n">{max}</span> in the deck</p>
</section>
<button class="primary go" onclick={go} disabled={max < 1}>
  {verb === 'draw' ? `Draw ${count}` : `Look at the top ${count}`}
</button>

<style>
  /* Like the attack panel: a labelled count, then one filled button. */
  .sec { display: flex; flex-direction: column; gap: var(--space-2); padding: var(--space-3) 0 var(--space-1); }
  .label {
    font: var(--fw-label) var(--fs-label)/var(--lh-label) var(--font-sans);
    letter-spacing: var(--ls-label);
    text-transform: uppercase;
    color: var(--muted);
    padding: 0 var(--space-4);
  }
  .left { margin: 0; padding: 0 var(--space-4); font-size: var(--fs-13); color: var(--muted); }
  .n { font: 600 var(--fs-13) var(--font-mono); color: var(--ink); }
  .go,
  .go:hover:not(:disabled) {
    margin: var(--space-3) var(--space-4) var(--space-4);
    width: calc(100% - 2 * var(--space-4));
    min-height: 56px;
    border-radius: var(--radius-l);
    border: 0;
    background: var(--primary);
    color: var(--on-primary);
    font: 700 var(--fs-16) var(--font-sans);
  }
  .go:focus-visible { outline: 2px solid var(--primary); outline-offset: 2px; }
  :global(.sheet.popover) .sec { padding-top: var(--space-2); }
  :global(.sheet.popover) .label,
  :global(.sheet.popover) .left { padding: 0 14px; }
  :global(.sheet.popover) .left,
  :global(.sheet.popover) .n { font-size: var(--fs-12); }
  :global(.sheet.popover) .go,
  :global(.sheet.popover) .go:hover:not(:disabled) {
    margin: var(--space-2) 14px var(--space-3);
    width: calc(100% - 28px);
    min-height: 32px;
    border-radius: var(--radius-s);
    font-size: var(--fs-13);
    font-weight: 600;
  }
</style>
