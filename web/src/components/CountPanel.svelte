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
  <div class="label">{verb === 'draw' ? 'Cards to draw' : 'Cards to look at from the top'} · {max} in the deck</div>
  <Stepper value={count} step={1} min={1} {max} label={verb === 'draw' ? 'Cards to draw' : 'Cards to look at'} set={(v) => (n = v)} />
</section>
<button class="primary go" onclick={go} disabled={max < 1}>
  {verb === 'draw' ? `Draw ${count}` : `Look at the top ${count}`}
</button>

<style>
  .sec { padding: 10px 0 6px; }
  .label { font-size: 11px; letter-spacing: 0.08em; text-transform: uppercase; color: var(--muted); font-weight: 700; padding: 0 16px 6px; }
  .go { margin: 12px 16px 16px; min-height: 52px; font-size: 16px; border-radius: 12px; width: calc(100% - 32px); }
</style>
