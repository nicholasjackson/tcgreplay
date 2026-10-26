<script>
  // The middle of the mat: Stadium, turn, coin, a play area to drop
  // Trainers on, lingering effects and the turn buttons.
  import { app, canAct, oppActing } from '../lib/store.svelte.js';
  import { dropOnZone, endTurn, readDrag, seatArgs } from '../lib/actions.js';
  import { act } from '../lib/ws.js';
  import Card from './Card.svelte';

  const b = $derived(app.board);
  const snap = $derived(app.snap);
  const setup = $derived(b.phase === 'setup');
  const myTurn = $derived(b.turn_open && b.current === app.human);
  let over = $state('');

  function zone(name) {
    return {
      ondragover: (ev) => {
        ev.preventDefault();
        over = name;
      },
      ondragleave: () => (over = ''),
      ondrop: async (ev) => {
        ev.preventDefault();
        over = '';
        const data = readDrag(ev);
        if (data) await dropOnZone(data, name);
      },
    };
  }
  async function flip() {
    const r = await act('coin_flip', { n: 1 });
    if (Array.isArray(r)) app.caption = `Coin: ${r.join(', ')}`;
    setTimeout(() => (app.caption = ''), 2000);
  }
  // In review mode the controls act for whoever's turn it is.
  const review = $derived(snap?.mode === 'review');
  const actor = $derived(review && b.current ? b.current : app.human);
  const nameOf = (seat) => b.sides?.[seat]?.name || (seat === app.human ? 'You' : app.assistant);
  function whoseTurn() {
    if (setup) return 'Setup';
    const who = b.current === app.human ? 'Your turn' : `${nameOf(b.current)}'s turn`;
    return `Turn ${b.turn} · ${who}`;
  }
</script>

<!-- No bar: a thin midline between the halves. The Stadium straddles it
     left of the Active Spots, the turn controls float on the right. -->
<div class="strip">
  <div class="stadium" class:over={over === 'stadium'} {...zone('stadium')} role="region" aria-label="Stadium">
    {#if b.stadium}
      <Card card={b.stadium} size="lg" from="stadium" />
    {:else}
      <span>Stadium</span>
    {/if}
  </div>

  <div class="controls">
    <div class="status">
      <div class="turn" class:mine={myTurn}>{whoseTurn()}</div>
      <button class="coin" onclick={flip} title="Flip a coin">
        {#if b.last_coins?.length}{b.last_coins.map((c) => (c === 'heads' ? 'H' : 'T')).join(' ')}{:else}Coin{/if}
      </button>
    </div>

    <div class="buttons">
      {#if setup}
        <button onclick={() => act('mulligan')}>Mulligan</button>
        <button class="primary" onclick={() => act('start_game', { first: app.human })}>I go first</button>
        <button onclick={() => act('start_game', { first: app.claude })}>{app.assistant} goes first</button>
      {:else}
        {#if snap?.open_action}
          <button onclick={() => act('end_action')} title="Close the open compound action" disabled={oppActing()}>End action {snap.open_action}</button>
        {/if}
        <button onclick={() => act('draw', { n: 1, ...seatArgs(actor) })} disabled={!canAct()}>
          {actor === app.human ? 'Draw' : `Draw for ${nameOf(actor)}`}
        </button>
        <button class="primary" onclick={() => endTurn(actor)} disabled={(!myTurn && !review) || oppActing()}>
          {actor === app.human ? 'End turn' : `End ${nameOf(actor)}'s turn`}
        </button>
      {/if}
    </div>

    {#if app.dragging?.kind === 'card'}
      <div class="play" class:over={over === 'play'} {...zone('play')} role="region" aria-label="Play area">
        Drop here to play
      </div>
    {/if}

    {#if b.effects?.length}
      <div class="effects">
        {#each b.effects as e (e.id)}
          <span class="effect" title="{e.id} from {e.source || '?'}">
            {e.text} · {e.seat}{#if e.until_turn} · until end of T{e.until_turn} · {nameOf(e.until_seat)}{/if}
            <button onclick={() => act('remove_effect', { name: e.id })} aria-label="Remove effect">×</button>
          </span>
        {/each}
      </div>
    {/if}
  </div>
</div>

<style>
  .strip {
    position: relative;
    height: calc(var(--cw) * 0.2);
    z-index: 2;
    color: #e8efe9;
    --muted: #9bb0a2;
  }
  /* The Active card is 1.25 card widths in a 1.45-wide slot. */
  .stadium {
    position: absolute;
    top: 50%;
    right: calc(50% + var(--cw) * 0.95);
    transform: translateY(-50%);
    width: calc(var(--cw) * 1.35);
    min-height: calc(var(--cw) * 1.25 * 88 / 63 + 8px);
    border: 2px dashed var(--slot-line);
    border-radius: 8px;
    display: grid;
    place-items: center;
    color: var(--muted);
    font-size: 0.85em;
    transition: background 0.15s;
  }
  .stadium:has(.card) { border-color: transparent; }
  .controls {
    position: absolute;
    top: 50%;
    left: calc(50% + var(--cw) * 0.95);
    transform: translateY(-50%);
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: calc(var(--cw) * 0.08);
    max-width: calc(var(--cw) * 2.8);
  }
  .status { display: flex; align-items: center; gap: calc(var(--cw) * 0.08); }
  .play {
    border: 2px dashed var(--accent);
    border-radius: 8px;
    padding: 0.8em 1.2em;
    color: var(--text);
    background: #0006;
    font-size: 0.85em;
  }
  .over { background: var(--drop); border-color: var(--accent); color: var(--text); }
  .turn {
    font-weight: 700;
    padding: 0.45em 0.9em;
    border-radius: 1.2em;
    background: #0006;
    white-space: nowrap;
  }
  .turn.mine { background: var(--accent); color: #1a1a1a; }
  .coin {
    width: 2.6em;
    height: 2.6em;
    padding: 0;
    flex: none;
    border-radius: 50%;
    border: 3px solid #b8892a;
    background: radial-gradient(circle at 35% 30%, #ffe48a, #d9a636);
    color: #4a3200;
    font-weight: 800;
    font-size: 0.9em;
    cursor: pointer;
  }
  .effects { display: flex; gap: 6px; flex-wrap: wrap; }
  .effect {
    background: #6a1b9a;
    color: white;
    font-size: 0.85em;
    padding: 0.3em 0.6em;
    border-radius: 1em;
  }
  .effect button { background: none; border: none; color: inherit; cursor: pointer; padding: 0 0 0 4px; }
  .buttons { display: flex; gap: 6px; flex-wrap: wrap; }
  .buttons button { font-size: 0.95em; padding: 0.4em 0.8em; }
</style>
