<script>
  // The middle of the mat: Stadium, turn, coin, a play area to drop
  // Trainers on, lingering effects and the turn buttons.
  import { app, canAct } from '../lib/store.svelte.js';
  import { dropOnZone, endTurn, readDrag } from '../lib/actions.js';
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
  function whoseTurn() {
    if (setup) return 'Setup';
    const who = b.current === app.human ? 'Your turn' : b.current === app.claude ? "Claude's turn" : b.current;
    return `Turn ${b.turn} · ${who}`;
  }
</script>

<div class="strip">
  <div class="stadium" class:over={over === 'stadium'} {...zone('stadium')} role="region" aria-label="Stadium">
    {#if b.stadium}
      <Card card={b.stadium} size="sm" from="stadium" />
    {:else}
      <span>Stadium</span>
    {/if}
  </div>

  <div class="turn" class:mine={myTurn}>{whoseTurn()}</div>

  <button class="coin" onclick={flip} title="Flip a coin">
    {#if b.last_coins?.length}{b.last_coins.map((c) => (c === 'heads' ? 'H' : 'T')).join(' ')}{:else}Coin{/if}
  </button>

  <div class="play" class:over={over === 'play'} {...zone('play')} role="region" aria-label="Play area">
    Drop a Trainer here to play it
  </div>

  {#if b.effects?.length}
    <div class="effects">
      {#each b.effects as e (e.id)}
        <span class="effect" title="{e.id} from {e.source || '?'}">
          {e.text} · {e.seat}{#if e.until_turn} · until end of T{e.until_turn}{/if}
          <button onclick={() => act('remove_effect', { name: e.id })} aria-label="Remove effect">×</button>
        </span>
      {/each}
    </div>
  {/if}

  <div class="buttons">
    {#if setup}
      <button onclick={() => act('mulligan')}>Mulligan</button>
      <button class="primary" onclick={() => act('start_game', { first: app.human })}>I go first</button>
      <button onclick={() => act('start_game', { first: app.claude })}>Claude goes first</button>
    {:else}
      {#if snap?.open_action}
        <button onclick={() => act('end_action')} title="Close the open compound action">End action {snap.open_action}</button>
      {/if}
      <button onclick={() => act('draw', { n: 1 })} disabled={!canAct()}>Draw</button>
      <button class="primary" onclick={endTurn} disabled={!myTurn && snap?.mode !== 'review'}>End turn</button>
    {/if}
  </div>
</div>

<style>
  .strip {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 6px 14px;
    background: var(--strip);
    border-top: 1px solid var(--line);
    border-bottom: 1px solid var(--line);
    flex-wrap: wrap;
    color: #e8efe9;
    --muted: #9bb0a2;
  }
  .stadium,
  .play {
    border: 2px dashed var(--slot-line);
    border-radius: 8px;
    display: grid;
    place-items: center;
    color: var(--muted);
    font-size: 12px;
    transition: background 0.15s;
  }
  .stadium { min-width: calc(var(--cw) * 0.8); min-height: calc(var(--cw) * 0.62 * 1.4); padding: 3px; }
  .play { flex: 1; min-width: 140px; min-height: 44px; padding: 0 10px; }
  .over { background: var(--drop); border-color: var(--accent); color: var(--text); }
  .turn {
    font-weight: 700;
    padding: 6px 12px;
    border-radius: 16px;
    background: #0004;
    white-space: nowrap;
  }
  .turn.mine { background: var(--accent); color: #1a1a1a; }
  .coin {
    width: 46px;
    height: 46px;
    border-radius: 50%;
    border: 3px solid #b8892a;
    background: radial-gradient(circle at 35% 30%, #ffe48a, #d9a636);
    color: #4a3200;
    font-weight: 800;
    cursor: pointer;
  }
  .effects { display: flex; gap: 6px; flex-wrap: wrap; }
  .effect {
    background: #6a1b9a;
    color: white;
    font-size: 12px;
    padding: 4px 8px;
    border-radius: 12px;
  }
  .effect button { background: none; border: none; color: inherit; cursor: pointer; padding: 0 0 0 4px; }
  .buttons { display: flex; gap: 6px; margin-left: auto; flex-wrap: wrap; }
</style>
