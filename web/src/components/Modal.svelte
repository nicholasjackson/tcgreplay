<script>
  // Modals: a pile (discard, Lost Zone), a deck search, the top N cards and
  // the end-of-turn prompt when the clipboard is unavailable.
  import { app, cardInfo } from '../lib/store.svelte.js';
  import { openSearch, pileCardActions } from '../lib/actions.js';
  import { act } from '../lib/ws.js';
  import Card from './Card.svelte';

  const m = $derived(app.modal);
  let filter = $state('');

  $effect(() => {
    if (m?.kind === 'search') filter = m.filter || '';
  });

  const pileCards = $derived(m?.kind === 'pile' ? app.board?.sides[m.seat]?.[m.zone] || [] : []);

  async function run(fn) {
    await fn();
    // Searches and peeks are snapshots; refresh them after a change.
    if (m?.kind === 'search') openSearch(m.seat, filter);
    if (m?.kind === 'look') {
      const cards = await act('look_top', { n: m.n, whose: m.seat });
      if (cards) app.modal = { ...m, cards };
    }
  }
  function close() {
    app.modal = null;
  }
  const kindOf = (c) => {
    const i = cardInfo(c.def);
    return c.kind || i?.stage || i?.trainer_type || i?.energy_type || '';
  };
</script>

<svelte:window onkeydown={(ev) => ev.key === 'Escape' && close()} />

{#if m}
  <div class="backdrop" onclick={close} role="presentation"></div>
  <div class="modal" role="dialog" aria-modal="true">
    {#if m.kind === 'pile'}
      <header><h2>{m.seat} {m.zone === 'lost' ? 'Lost Zone' : 'discard'} · {pileCards.length}</h2><button onclick={close}>Close</button></header>
      <div class="grid">
        {#each pileCards as c (c.iid)}
          <div class="entry">
            <Card card={c} size="sm" from="{m.seat}:{m.zone}" />
            <div class="acts">
              {#each pileCardActions(c, m.seat, m.zone) as a}
                <button onclick={() => run(a.run)}>{a.label}</button>
              {/each}
            </div>
          </div>
        {:else}
          <p class="empty">Empty.</p>
        {/each}
      </div>
    {:else if m.kind === 'search'}
      <header>
        <h2>Search {m.seat} deck</h2>
        <form onsubmit={(ev) => { ev.preventDefault(); openSearch(m.seat, filter); }}>
          <input bind:value={filter} placeholder="e.g. basic fighting, supporter" />
          <button type="submit">Filter</button>
        </form>
        <button onclick={() => run(() => act('shuffle', { whose: m.seat }))}>Shuffle</button>
        <button onclick={close}>Done</button>
      </header>
      <p class="note">Sorted by category and name, never in deck order. Remember to shuffle.</p>
      <div class="list">
        {#each m.cards as c (c.iid)}
          <div class="row">
            <Card card={c} size="xs" from="{m.seat}:deck" />
            <span class="name">{c.name}</span>
            <span class="kind">{kindOf(c)}</span>
            <span class="acts">
              {#each pileCardActions(c, m.seat, 'deck') as a}
                <button onclick={() => run(a.run)}>{a.label}</button>
              {/each}
            </span>
          </div>
        {:else}
          <p class="empty">No matching cards.</p>
        {/each}
      </div>
    {:else if m.kind === 'look'}
      <header><h2>Top {m.n} of {m.seat} deck</h2><button onclick={close}>Done</button></header>
      <p class="note">Top card first.</p>
      <div class="grid">
        {#each m.cards as c, i (c.iid)}
          <div class="entry">
            <span class="pos">{i + 1}</span>
            <Card card={c} size="sm" from="{m.seat}:deck" />
            <div class="acts">
              {#each pileCardActions(c, m.seat, 'deck') as a}
                <button onclick={() => run(a.run)}>{a.label}</button>
              {/each}
            </div>
          </div>
        {/each}
      </div>
    {:else if m.kind === 'prompt'}
      <header><h2>Paste this to Claude</h2><button onclick={close}>Close</button></header>
      <textarea readonly rows="3" onfocus={(ev) => ev.target.select()}>{m.text}</textarea>
    {/if}
  </div>
{/if}

<style>
  .backdrop { position: fixed; inset: 0; background: #0009; z-index: 30; }
  .modal {
    position: fixed;
    z-index: 31;
    inset: 6vh 50% auto auto;
    transform: translateX(50%);
    width: min(900px, 94vw);
    max-height: 86vh;
    overflow-y: auto;
    background: var(--panel);
    border: 1px solid var(--line);
    border-radius: 12px;
    padding: 14px 18px 18px;
    box-shadow: 0 20px 60px #000c;
  }
  header { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
  header h2 { margin: 0 auto 0 0; font-size: 16px; }
  form { display: flex; gap: 4px; }
  input, textarea {
    background: var(--input);
    color: var(--text);
    border: 1px solid var(--line);
    border-radius: 6px;
    padding: 6px 8px;
    font: inherit;
  }
  textarea { width: 100%; margin-top: 10px; box-sizing: border-box; }
  .note { color: var(--muted); font-size: 12px; margin: 6px 0; }
  .grid { display: flex; flex-wrap: wrap; gap: 12px; margin-top: 10px; }
  .entry { display: flex; flex-direction: column; align-items: center; gap: 4px; position: relative; }
  .pos { position: absolute; top: -6px; left: -6px; z-index: 2; background: var(--accent); color: #111; border-radius: 50%; width: 18px; height: 18px; display: grid; place-items: center; font: 700 11px system-ui; }
  .acts { display: flex; flex-wrap: wrap; gap: 3px; justify-content: center; max-width: calc(var(--cw) * 1.4); }
  .list { display: flex; flex-direction: column; gap: 4px; margin-top: 8px; }
  .row { display: grid; grid-template-columns: auto 1fr auto auto; gap: 10px; align-items: center; padding: 3px 4px; border-radius: 6px; }
  .row:hover { background: var(--hover); }
  .row .acts { max-width: none; justify-content: flex-end; }
  .name { font-weight: 600; }
  .kind { color: var(--muted); font-size: 12px; }
  .empty { color: var(--muted); }
  button { font-size: 12px; }
</style>
