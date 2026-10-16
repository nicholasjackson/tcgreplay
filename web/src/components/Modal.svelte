<script>
  // Modals: a pile (discard, Lost Zone), a deck search, the top N cards and
  // the end-of-turn prompt when the clipboard is unavailable.
  import { app, cardInfo } from '../lib/store.svelte.js';
  import { openDeckList, openSearch, pileCardActions } from '../lib/actions.js';
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
  // Deck list: rows grouped like a decklist, filtered by name.
  let listFilter = $state('');
  // Which copies the deck list shows. Hidden copies are in the deck or
  // prizes (for an opponent, possibly the hand).
  const shows = {
    all: { label: 'All cards', places: null },
    unplayed: { label: 'Unplayed (deck, prizes, hand)', places: ['deck', 'prizes', 'hidden', 'hand'] },
    undrawn: { label: 'Not drawn yet (deck, prizes)', places: ['deck', 'prizes', 'hidden'] },
  };
  // shown narrows a row to the copies the filter keeps.
  function shown(r) {
    const keep = shows[app.deckListShow]?.places;
    if (!keep) return r;
    const where = {};
    let count = 0;
    for (const p of keep) {
      if (r.where[p]) {
        where[p] = r.where[p];
        count += r.where[p];
      }
    }
    return { ...r, where, count };
  }
  const sections = $derived.by(() => {
    if (m?.kind !== 'decklist') return [];
    const f = listFilter.trim().toLowerCase();
    const groups = [
      { title: 'Pokémon', cat: 'Pokemon', rows: [] },
      { title: 'Trainer', cat: 'Trainer', rows: [] },
      { title: 'Energy', cat: 'Energy', rows: [] },
      { title: 'Other', cat: '', rows: [] },
    ];
    for (const row of m.rows) {
      const r = shown(row);
      if (!r.count) continue;
      if (f && !(r.name || 'unknown').toLowerCase().includes(f)) continue;
      (groups.find((g) => g.cat && g.cat === r.category) || groups[3]).rows.push(r);
    }
    return groups.filter((g) => g.rows.length);
  });
  const total = (rows) => rows.reduce((n, r) => n + r.count, 0);
  const places = ['active', 'bench', 'hand', 'discard', 'lost', 'stadium', 'deck', 'prizes', 'hidden'];
  function placeLabel(p) {
    if (p !== 'hidden') return p;
    return m.seat === app.human ? 'deck or prizes' : 'hidden';
  }
  // The card under the mouse, shown large beside the modal: enlarging it
  // in place would be clipped by the modal's scrolling.
  let preview = $state(null);
  const previewInfo = $derived(preview?.def ? cardInfo(preview.def) : null);
  $effect(() => {
    if (!m) preview = null;
  });
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
          <div class="entry" onmouseenter={() => (preview = c)} onmouseleave={() => (preview = null)}>
            <Card card={c} size="sm" from="{m.seat}:{m.zone}" lift={false} />
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
          <div class="row" onmouseenter={() => (preview = c)} onmouseleave={() => (preview = null)}>
            <Card card={c} size="xs" from="{m.seat}:deck" lift={false} />
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
          <div class="entry" onmouseenter={() => (preview = c)} onmouseleave={() => (preview = null)}>
            <span class="pos">{i + 1}</span>
            <Card card={c} size="sm" from="{m.seat}:deck" lift={false} />
            <div class="acts">
              {#each pileCardActions(c, m.seat, 'deck') as a}
                <button onclick={() => run(a.run)}>{a.label}</button>
              {/each}
            </div>
          </div>
        {/each}
      </div>
    {:else if m.kind === 'decklist'}
      <header>
        <h2>{m.seat === app.human ? 'Your deck' : `${m.seat} deck`} · {total(sections.flatMap((s) => s.rows))}</h2>
        <select bind:value={app.deckListShow} aria-label="Which cards to show">
          {#each Object.entries(shows) as [key, s] (key)}
            <option value={key}>{s.label}</option>
          {/each}
        </select>
        <input bind:value={listFilter} placeholder="Find a card, e.g. meowth" />
        <button onclick={() => openDeckList(m.seat)}>Refresh</button>
        <button onclick={close}>Done</button>
      </header>
      <p class="note">
        Every card and where each copy is now.
        {#if !app.reveal}Prizes are face down, so they count as {m.seat === app.human ? '"deck or prizes"' : '"hidden"'}; turn on Reveal all to place them.{/if}
      </p>
      {#each sections as s (s.title)}
        <h3 class="section">{s.title} · {total(s.rows)}</h3>
        <div class="grid">
          {#each s.rows as r (r.name)}
            <div class="entry dl" onmouseenter={() => (preview = r)} onmouseleave={() => (preview = null)} role="presentation">
              <span class="count">×{r.count}</span>
              <Card card={{ iid: r.iid, name: r.name, def: r.def }} size="sm" draggable={false} lift={false} />
              <div class="where">
                {#each places.filter((p) => r.where[p]) as p}
                  <span class="place {p}">{r.where[p]} {placeLabel(p)}</span>
                {/each}
              </div>
            </div>
          {/each}
        </div>
      {:else}
        <p class="empty">No matching cards.</p>
      {/each}
    {:else if m.kind === 'prompt'}
      <header><h2>Paste this to Claude</h2><button onclick={close}>Close</button></header>
      <textarea readonly rows="3" onfocus={(ev) => ev.target.select()}>{m.text}</textarea>
    {/if}
  </div>
  {#if previewInfo?.image}
    <img class="preview" src="{previewInfo.image}/high.webp" alt={preview.name} />
  {/if}
{/if}

<style>
  .backdrop { position: fixed; inset: 0; background: #0009; z-index: 30; }
  .modal, .preview { --mw: min(1000px, 94vw); }
  .modal {
    position: fixed;
    z-index: 31;
    inset: 6vh 50% auto auto;
    transform: translateX(50%);
    width: var(--mw);
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
  select {
    background: var(--input);
    color: var(--text);
    border: 1px solid var(--line);
    border-radius: 6px;
    padding: 6px 8px;
    font: inherit;
  }
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
  /* Beside the modal on wide screens; over its right edge on narrow ones. */
  .preview {
    position: fixed;
    z-index: 32;
    top: 6vh;
    left: min(calc(50% + var(--mw) / 2 + 16px), calc(100vw - var(--pw) - 16px));
    --pw: clamp(240px, calc(var(--cw) * 3), 600px);
    width: var(--pw);
    border-radius: calc(var(--pw) * 0.045);
    box-shadow: 0 20px 60px #000c;
    pointer-events: none;
  }
  .section { font-size: 13px; margin: 14px 0 0; color: var(--muted); text-transform: uppercase; letter-spacing: 0.06em; }
  .dl { width: calc(var(--cw) * 0.62 + 40px); }
  .count { position: absolute; top: -6px; right: 14px; z-index: 2; background: var(--accent); color: #111; border-radius: 10px; padding: 1px 6px; font: 700 12px system-ui; }
  .where { display: flex; flex-wrap: wrap; gap: 3px; justify-content: center; }
  .place { font-size: 11px; padding: 1px 6px; border-radius: 8px; background: #0005; white-space: nowrap; }
  .place.active, .place.bench { background: #2e7d32; color: #fff; }
  .place.hand { background: #1565c0; color: #fff; }
  .place.prizes { background: #ad1457; color: #fff; }
  .place.discard, .place.lost { background: #5d4037; color: #fff; }
  button { font-size: 12px; }
</style>
