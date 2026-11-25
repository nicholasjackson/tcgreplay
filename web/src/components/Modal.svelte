<script>
  // Modals: a pile (discard, Lost Zone), a hand, a deck search, the top N cards,
  // the end-of-turn prompt when the clipboard is unavailable, and About
  // (the fan-project notice, card-data credit and help link, from the
  // notice file the website shares).
  import { app, cardInfo, imageSrc, nextImageStage, openSheet, setActionKey } from '../lib/store.svelte.js';
  import { categoryOf, handMenu, openDeckList, openSearch, pileCardActions, pileCells, pileKey } from '../lib/actions.js';
  import { act } from '../lib/ws.js';
  import Card from './Card.svelte';
  import Icon from './Icon.svelte';
  import notice from '../../../internal/notice/notice.json';

  const m = $derived(app.modal);
  // On a touch screen a view closes like the action sheet, with an
  // outlined X in the corner; on a phone it fills the screen.
  const touch = $derived(app.layout !== 'desktop');
  const phone = $derived(app.layout === 'phone');
  // The card-list views (pile, prizes, deck search, top N) follow the
  // PileActions design: a fixed header with its own close, the key, then
  // rows that scroll inside the view.
  const listView = $derived(['pile', 'prizes', 'search', 'look'].includes(m?.kind));
  let filter = $state('');

  $effect(() => {
    if (m?.kind === 'search') filter = m.filter || '';
  });

  // A pile's cards top first: the board lists them bottom first, the top
  // card last. The view keeps the pile's order, never sorting it.
  const pileCards = $derived(m?.kind === 'pile' ? [...(app.board?.sides[m.seat]?.[m.zone] || [])].reverse() : []);
  // The prize view (the phone's Prizes counter): each slot, taken or not.
  const prizes = $derived(m?.kind === 'prizes' ? app.board?.sides[m.seat]?.prizes || [] : []);
  // The hand view: the cards the board knows of, then a back for each
  // card it does not.
  const handSide = $derived(m?.kind === 'hand' ? app.board?.sides[m.seat] : null);
  const handCards = $derived.by(() => {
    if (!handSide) return [];
    const known = handSide.hand || [];
    const backs = Array.from({ length: Math.max(0, handSide.hand_count - known.length) }, (_, i) => ({ iid: `${m.seat}-hidden-${i}`, hidden: true }));
    return [...known, ...backs];
  });
  const mayTake = $derived(m?.seat === app.human || app.snap?.mode === 'review');
  function takePrize(p) {
    const args = { slot: p.slot };
    if (m.seat !== app.human) args.seat = m.seat;
    act('take_prize', args);
  }

  // A deck search ends in a shuffle, as every searching card says: when
  // the view closes (Done, the X, Escape or anything else) after a card
  // left the deck and no one pressed Shuffle, the board shuffles. Plain
  // variables, not state, so the effect below does not track them.
  let search = { open: false, seat: null, took: false, shuffled: false };
  $effect(() => {
    const k = m?.kind;
    if (k === 'search' && !search.open) {
      search = { open: true, seat: m.seat, took: false, shuffled: false };
    } else if (k !== 'search' && search.open) {
      const { seat, took, shuffled } = search;
      search = { open: false, seat: null, took: false, shuffled: false };
      if (took && !shuffled) act('shuffle', { whose: seat });
    }
  });
  async function shuffleSearch() {
    await run(() => act('shuffle', { whose: m.seat }));
    search.shuffled = true;
  }

  async function run(fn) {
    if (m?.kind === 'search') search = { ...search, took: true, shuffled: false };
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
  // cardMenu opens a listed card's actions, the same moves as its buttons.
  function cardMenu(c, from) {
    return (ev, type) =>
      openSheet(
        ev,
        () => ({
          title: c.name || c.iid,
          card: c,
          summary: `In ${m.seat}'s ${from === 'lost' ? 'Lost Zone' : from}`,
          sections: [{ label: 'Move', grid: true, items: pileCardActions(c, m.seat, from).map((a) => ({ label: a.label, run: () => run(a.run) })) }],
        }),
        type,
      );
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
  // The large preview beside the view follows the mouse. A tap opens the
  // card's actions with the card (or, in the deck list, the card itself).
  function hover(ev, c) {
    if (ev.pointerType === 'mouse') preview = c;
  }
  const previewInfo = $derived(preview?.def ? cardInfo(preview.def) : null);
  // Which address the preview's art is loaded from (see imageSrc); reset
  // per card.
  let previewStage = $state(0);
  $effect(() => {
    preview;
    previewStage = 0;
  });
  $effect(() => {
    if (!m) preview = null;
  });
  // The dialog's name: its heading's words.
  const title = $derived.by(() => {
    if (m?.kind === 'pile') return `${m.seat} ${m.zone === 'lost' ? 'Lost Zone' : 'discard'}`;
    if (m?.kind === 'prizes') return `${m.seat} prizes`;
    if (m?.kind === 'search') return `Search ${m.seat} deck`;
    if (m?.kind === 'look') return `Top ${m.n} of ${m.seat} deck`;
    return null;
  });
</script>

<svelte:window onkeydown={(ev) => ev.key === 'Escape' && close()} />

<!-- The card-list views' pieces (the PileActions design). -->
{#snippet closeButton(word)}
  {#if touch}<button class="hb xb" aria-label="Close" onclick={close}><Icon name="close" /></button>{:else}<button class="hb" onclick={close}>{word}</button>{/if}
{/snippet}
<!-- With the key hidden, the header keeps a Key button to bring it back. -->
{#snippet keyButton()}
  {#if !app.actionKey}<button class="hb" aria-pressed="false" aria-label="Show the key" title="Show the key" onclick={() => setActionKey(true)}><Icon name="key" />Key</button>{/if}
{/snippet}
<!-- The key: each cell's icon with its word, in the same cells as the
     buttons when narrow, one line when wide. -->
{#snippet key(cells)}
  {#if app.actionKey}
    <div class="key" role="note" aria-label="Key to the action buttons">
      <span class="klab">Key</span>
      <span class="kcells">
        {#each cells as k (k.cell)}<span class="k" data-cell={k.cell}><Icon name={k.icon} />{k.word}</span>{/each}
      </span>
      <button class="hide" onclick={() => setActionKey(false)}>Hide key</button>
    </div>
  {/if}
{/snippet}
<!-- A listed card: the card, its name over its category (pos, a position
     in the pile or the top N, before it), then all six action cells, an
     action the card can't take disabled with the reason. The card or its
     name opens its actions in words. -->
{#snippet row(c, from, pos)}
  <div class="prow" role="listitem" onpointerenter={(ev) => hover(ev, c)} onpointerleave={() => (preview = null)}>
    <Card card={c} size="xs" from="{m.seat}:{from}" lift={false} onmenu={cardMenu(c, from)} />
    <button class="ptxt" onclick={(ev) => cardMenu(c, from)(ev)}>
      <span class="nm">{c.name || c.iid}</span>
      <span class="cat">{#if pos}<span class="ppos">{pos}</span>{/if}{(pos ? ' · ' : '') + categoryOf(c)}</span>
    </button>
    <div class="pa" role="group" aria-label={c.name || c.iid}>
      {#each pileCells(c, m.seat, from) as k (k.cell)}
        {#if k.run}
          <button class="pab" style:grid-area={k.cell} aria-label={k.name} title={k.name} onclick={() => run(k.run)}><Icon name={k.icon} /></button>
        {:else}
          <button class="pab" style:grid-area={k.cell} aria-label={k.name} title={k.reason} disabled aria-disabled="true"><Icon name={k.icon} /></button>
        {/if}
      {/each}
    </div>
  </div>
{/snippet}

{#if m}
  <div class="backdrop" onclick={close} role="presentation"></div>
  <div class="modal" class:touch class:phone class:lv={listView} role="dialog" aria-modal="true" aria-label={listView ? title : null}>
    {#if touch && !listView}<button class="x" aria-label="Close" onclick={close}><Icon name="close" /></button>{/if}
    {#if m.kind === 'pile'}
      <header><div class="hr"><h2>{m.seat} {m.zone === 'lost' ? 'Lost Zone' : 'discard'} · <span class="n">{pileCards.length}</span></h2>{@render keyButton()}{@render closeButton('Close')}</div></header>
      <div class="lvbody">
        {@render key(pileKey(m.zone))}
        <p class="note">{m.zone === 'lost' ? 'Top of the Lost Zone first (1 is the card on top). Cards keep the order they went in.' : 'Top of the discard first (1 is the card on top). Cards keep the order they were discarded in.'}</p>
        <div class="plist" role="list" aria-label={title}>
          {#each pileCards as c, i (c.iid)}
            {@render row(c, m.zone, i + 1)}
          {:else}
            <p class="empty">Empty.</p>
          {/each}
        </div>
      </div>
    {:else if m.kind === 'prizes'}
      <header><div class="hr"><h2>{m.seat} prizes · <span class="n">{prizes.filter((p) => p.card).length}</span> left</h2>{#if mayTake}{@render keyButton()}{/if}{@render closeButton('Close')}</div></header>
      <div class="lvbody">
        {#if mayTake}{@render key([{ cell: 'take', icon: 'take', word: 'Take' }])}{/if}
        <div class="grid prizegrid">
          {#each prizes as p (p.slot)}
            <div class="entry">
              {#if p.card}
                <Card card={p.card} size="sm" draggable={false} lift={false} onclick={() => mayTake && takePrize(p)} />
                {#if mayTake}<button class="pab take" aria-label="Take this prize into your hand" title="Take this prize into your hand" onclick={() => takePrize(p)}><Icon name="take" /></button>{/if}
              {:else}
                <span class="empty">Taken</span>
              {/if}
            </div>
          {/each}
        </div>
      </div>
    {:else if m.kind === 'hand'}
      <!-- A player's hand (the opponent's Hand chip): face down unless
           revealed, a known card marked with its name. -->
      <header><h2>{m.seat} hand · {handSide?.hand_count ?? 0}</h2>{#if !touch}<button onclick={close}>Close</button>{/if}</header>
      <div class="grid" role="list" aria-label="{m.seat} hand">
        {#each handCards as c (c.iid)}
          <div class="entry" role="listitem" onpointerenter={(ev) => !c.hidden && hover(ev, c)} onpointerleave={() => (preview = null)}>
            <Card card={c} size="sm" from="{m.seat}:hand" lift={false} onmenu={c.hidden ? null : (ev, type) => openSheet(ev, () => handMenu(c), type)} />
          </div>
        {:else}
          <p class="empty">Empty.</p>
        {/each}
      </div>
    {:else if m.kind === 'search'}
      <header class="search">
        <div class="hr"><h2>Search {m.seat} deck</h2>{#if phone}{@render keyButton()}{@render closeButton('Done')}{/if}</div>
        <div class="hr tools">
          <form class="field" role="search" onsubmit={(ev) => { ev.preventDefault(); openSearch(m.seat, filter); }}>
            <input bind:value={filter} aria-label="Filter the deck" placeholder={phone ? 'e.g. basic fighting' : 'e.g. basic fighting, supporter'} />
            <button type="submit" aria-label="Filter" title="Filter"><Icon name="search" /></button>
          </form>
          <button class="hb" onclick={shuffleSearch}><Icon name="shuffle" />Shuffle</button>
          {#if !phone}{@render keyButton()}{@render closeButton('Done')}{/if}
        </div>
      </header>
      <div class="lvbody">
        {@render key(pileKey('deck'))}
        <p class="note">Sorted by category and name, never in deck order. The deck is shuffled when you close this if you took a card.</p>
        <div class="plist" role="list" aria-label={title}>
          {#each m.cards as c (c.iid)}
            {@render row(c, 'deck', 0)}
          {:else}
            <p class="empty">No matching cards.</p>
          {/each}
        </div>
      </div>
    {:else if m.kind === 'look'}
      <header><div class="hr"><h2>Top {m.n} of {m.seat} deck</h2>{@render keyButton()}{@render closeButton('Done')}</div></header>
      <div class="lvbody">
        {@render key(pileKey('deck'))}
        <p class="note">Top card first.</p>
        <div class="plist" role="list" aria-label={title}>
          {#each m.cards as c, i (c.iid)}
            {@render row(c, 'deck', i + 1)}
          {/each}
        </div>
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
        {#if !touch}<button onclick={close}>Done</button>{/if}
      </header>
      <p class="note">
        Every card and where each copy is now.
        {#if !app.reveal}Prizes are face down, so they count as {m.seat === app.human ? '"deck or prizes"' : '"hidden"'}; turn on Reveal all to place them.{/if}
      </p>
      {#each sections as s (s.title)}
        <h3 class="section">{s.title} · {total(s.rows)}</h3>
        <div class="grid">
          {#each s.rows as r (r.name)}
            <div class="entry dl" onpointerenter={(ev) => hover(ev, r)} onpointerleave={() => (preview = null)} role="presentation">
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
      <header><h2>Paste this to {app.assistant}</h2>{#if !touch}<button onclick={close}>Close</button>{/if}</header>
      <textarea readonly rows="3" onfocus={(ev) => ev.target.select()}>{m.text}</textarea>
    {:else if m.kind === 'about'}
      <header><h2>About tcgreplay</h2>{#if !touch}<button onclick={close}>Close</button>{/if}</header>
      <div class="about">
        <p class="fan-notice">{notice.notice}</p>
        <p class="credit">
          {notice.credit.lead}{' '}
          {#each notice.credit.sources as s, i (s.url)}{#if i > 0}{i === notice.credit.sources.length - 1 ? ' and ' : ', '}{/if}<a href={s.url} target="_blank" rel="noopener">{s.name}</a>{/each}.
        </p>
        <p><a href={notice.help.url} target="_blank" rel="noopener">Get help</a> ({notice.help.label})</p>
      </div>
    {/if}
  </div>
  {#if imageSrc(previewInfo, 'high.webp', previewStage)}
    <img class="preview" src={imageSrc(previewInfo, 'high.webp', previewStage)} alt={preview.name} onerror={() => (previewStage = nextImageStage(previewInfo, previewStage))} />
  {/if}
{/if}

<style>
  .backdrop { position: fixed; inset: 0; background: var(--scrim); z-index: 30; }
  /* A card dragged out of a view lands on the board: the view lets the
     pointer through and fades while the drag lasts. Its cards still let a
     finger scroll the list. */
  :global(.dragging) .backdrop,
  :global(.dragging) .modal { pointer-events: none; opacity: 0.2; }
  .modal :global(.card) { touch-action: pan-y; }
  .modal, .preview { --mw: min(1000px, 94vw); }
  /* The touch close: outlined, in the top-right corner, staying in view as
     the view scrolls. */
  .x {
    position: sticky;
    top: 0;
    float: right;
    z-index: 2;
    display: grid;
    place-items: center;
    width: var(--touch-target);
    height: var(--touch-target);
    margin: -6px -8px 0 var(--space-2);
    padding: 0;
    border-radius: var(--radius-m);
    font-size: var(--fs-18);
  }
  .modal.phone {
    inset: 0;
    transform: none;
    width: 100vw;
    height: 100dvh;
    max-height: none;
    border: 0;
    border-radius: 0;
  }
  .modal {
    position: fixed;
    z-index: 31;
    inset: 6vh 50% auto auto;
    transform: translateX(50%);
    width: var(--mw);
    max-height: 86vh;
    overflow-y: auto;
    background: var(--panel-raised);
    color: var(--ink);
    font-family: var(--font-sans);
    border: 1px solid var(--line);
    border-radius: var(--radius-xl);
    padding: var(--space-5) var(--space-6) var(--space-6);
    box-shadow: var(--shadow-dialog);
  }
  header { display: flex; align-items: center; gap: var(--space-2); flex-wrap: wrap; padding-bottom: var(--space-3); border-bottom: 1px solid var(--line); }
  header h2 { margin: 0 auto 0 0; font: 600 var(--fs-18)/1.3 var(--font-sans); }
  form { display: flex; gap: var(--space-1); }
  /* Secondary buttons: outlined, like the menus'. */
  button {
    min-height: 28px;
    padding: 0 10px;
    border-radius: var(--radius-s);
    border: 1px solid var(--control-line);
    background: var(--panel-raised);
    color: var(--ink);
    font: 500 var(--fs-12) var(--font-sans);
  }
  header button { min-height: 32px; font-size: var(--fs-13); }
  /* On a touch screen every control stays a fingertip's 44px. */
  @media (pointer: coarse) {
    button,
    header button,
    select,
    input { min-height: var(--touch-target); min-width: var(--touch-target); }
  }
  button:hover:not(:disabled) { border-color: var(--control-line); background: var(--hover); }
  button:focus-visible,
  select:focus-visible,
  input:focus-visible,
  textarea:focus-visible { outline: 2px solid var(--primary); outline-offset: 1px; }
  select,
  input,
  textarea {
    min-height: 32px;
    background: var(--panel-sunk);
    color: var(--ink);
    border: 1px solid var(--control-line);
    border-radius: var(--radius-s);
    padding: 6px var(--space-2);
    font: inherit;
  }
  textarea { width: 100%; margin-top: 10px; box-sizing: border-box; font-family: var(--font-mono); font-size: var(--fs-13); }
  .about { max-width: 40rem; overflow-wrap: anywhere; }
  .about p { margin: 10px 0 0; line-height: 1.45; }
  .about a { color: var(--primary); }
  .note { color: var(--muted); font-size: var(--fs-12); margin: var(--space-2) 0 6px; }
  .grid { display: flex; flex-wrap: wrap; gap: var(--space-3); margin-top: var(--space-3); }
  .entry { display: flex; flex-direction: column; align-items: center; gap: var(--space-1); position: relative; }
  /* Copies in the deck list, on the card. */
  .count {
    position: absolute;
    top: -6px;
    right: 14px;
    z-index: 2;
    background: var(--primary);
    color: var(--on-primary);
    border-radius: var(--radius-s);
    padding: 1px 6px;
    font: 600 var(--fs-12) var(--font-mono);
  }

  /* The card-list views (pile, prizes, deck search, top N): the
     PileActions design. The header stays put and the rows scroll. */
  .modal { --pa: 36px; }
  @media (pointer: coarse) {
    .modal { --pa: var(--touch-target); }
  }
  .modal.lv { display: flex; flex-direction: column; overflow: hidden; padding-bottom: 0; }
  .lv header { flex-direction: column; align-items: stretch; flex-wrap: nowrap; flex: none; }
  .lv .hr { display: flex; align-items: center; gap: var(--space-2); min-width: 0; }
  .lv .hr h2 { flex: 1; min-width: 0; margin: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  h2 .n { font-family: var(--font-mono); font-weight: 600; }
  /* Wider than a phone, the deck search's header is one row: the title,
     the filter, Shuffle and the close. */
  .lv:not(.phone) header.search { flex-direction: row; align-items: center; }
  .lv:not(.phone) header.search .hr:first-child { flex: 0 1 auto; min-width: 8em; }
  .lv:not(.phone) header.search .tools { flex: 1 1 auto; justify-content: flex-end; }
  .lv:not(.phone) header.search .field { flex: 0 1 360px; }
  .lv.phone { padding: 0; }
  .lv.phone header { padding: var(--space-3) var(--space-4); gap: var(--space-2); }
  .lvbody { container: pview / inline-size; flex: 1 1 auto; min-height: 0; display: flex; flex-direction: column; gap: var(--space-2); padding-top: var(--space-3); }
  .lv.phone .lvbody { padding: var(--space-3) var(--space-4) 0; }
  .lv .note { margin: 0; line-height: 1.35; }
  .plist { flex: 1 1 auto; min-height: 0; overflow-y: auto; overscroll-behavior: contain; margin: 0 calc(-1 * var(--space-2)); padding-bottom: var(--space-4); }
  .lv .prizegrid { overflow-y: auto; min-height: 0; margin-top: var(--space-1); padding: 6px 0 var(--space-6); }

  /* Header buttons: outlined, 44px on touch, an icon beside a word. */
  .lv .hb { flex: none; display: inline-flex; align-items: center; justify-content: center; gap: 6px; min-height: 32px; padding: 0 12px; }
  .lv .hb :global(.icon) { width: 18px; height: 18px; }
  .lv .xb { width: var(--touch-target); height: var(--touch-target); padding: 0; border-radius: var(--radius-m); }
  .lv .xb :global(.icon) { width: 20px; height: 20px; }
  .field { display: flex; flex: 1; min-width: 0; gap: 0; }
  .field input { flex: 1; min-width: 0; width: 0; border-right: 0; border-radius: var(--radius-s) 0 0 var(--radius-s); }
  .field button { flex: none; display: grid; place-items: center; width: 32px; min-height: 32px; padding: 0; border-radius: 0 var(--radius-s) var(--radius-s) 0; }
  .field button :global(.icon) { width: 18px; height: 18px; }
  /* These scoped sizes outrank app.css's 44px touch minimum: restore it. */
  @media (pointer: coarse) {
    .lv .hb,
    .field button { min-height: var(--touch-target); }
    .lv .hb { border-radius: var(--radius-m); }
    .lv .hb :global(.icon) { width: 20px; height: 20px; }
    .field input { border-radius: var(--radius-m) 0 0 var(--radius-m); font-size: var(--fs-15); }
    .field button { width: var(--touch-target); border-radius: 0 var(--radius-m) var(--radius-m) 0; }
    .field button :global(.icon) { width: 20px; height: 20px; }
  }

  /* The key: narrow, a grid putting each word in its button's cell; wide,
     one line with a rule between the two rows' groups. */
  .key { flex: none; display: grid; grid-template-columns: minmax(0, 1fr) auto; align-items: center; row-gap: 6px; padding: 6px 12px 10px; background: var(--panel-sunk); border: 1px solid var(--line); border-radius: var(--radius-m); color: var(--ink-soft); font: 500 var(--fs-12)/1.35 var(--font-sans); }
  .klab { font: 600 var(--fs-11)/1.35 var(--font-sans); letter-spacing: 0.06em; text-transform: uppercase; color: var(--muted); }
  .kcells { grid-column: 1 / -1; grid-row: 2; display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 6px 8px; }
  .k { display: inline-flex; align-items: center; gap: 6px; white-space: nowrap; min-width: 0; }
  .k :global(.icon) { width: 16px; height: 16px; color: var(--ink); }
  .hide { grid-column: 2; grid-row: 1; min-height: 20px; margin: 0 -6px 0 0; padding: 0 6px; border: 0; background: none; color: var(--primary); font: 500 var(--fs-12) var(--font-sans); text-decoration: underline; text-underline-offset: 2px; }
  .hide:hover:not(:disabled) { background: none; color: var(--primary); }
  @container pview (min-width: 560px) {
    .key { display: flex; flex-wrap: wrap; gap: 4px 16px; padding: 9px 12px; }
    .kcells { display: flex; flex-wrap: wrap; gap: 4px 16px; }
    .k[data-cell='deck']::before { content: ''; width: 1px; height: 16px; margin-right: 10px; background: var(--control-line); }
    .hide { margin: -4px -6px -4px auto; }
  }
  @media (pointer: coarse) {
    .hide { min-height: var(--touch-target); margin-top: -12px; margin-bottom: -12px; }
  }
  @container pview (min-width: 560px) {
    @media (pointer: coarse) {
      .hide { margin-top: -14px; margin-bottom: -14px; }
    }
  }

  /* A row: the card, its name over its category, then the six cells. */
  .prow { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; gap: var(--space-3); padding: 6px var(--space-2); border-bottom: 1px solid var(--line); }
  .prow:last-child { border-bottom: 0; }
  .phone .prow { gap: 10px; padding: var(--space-2); }
  @media (hover: hover) and (pointer: fine) {
    .prow:hover { background: var(--hover); border-radius: var(--radius-s); }
  }
  .modal .prow :global(.card) { --w: 40px; }
  @media (pointer: coarse) {
    .modal .prow :global(.card) { --w: var(--touch-target); }
  }
  .ptxt { min-width: 0; width: 100%; min-height: 0; display: flex; flex-direction: column; align-items: flex-start; gap: 2px; padding: 0; border: 0; background: none; color: inherit; text-align: left; cursor: pointer; }
  .ptxt:hover:not(:disabled) { background: none; }
  @media (pointer: coarse) {
    .ptxt { min-height: var(--touch-target); }
  }
  .nm { max-width: 100%; font: 600 var(--fs-14)/1.4 var(--font-sans); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .cat { max-width: 100%; font: 500 var(--fs-12)/1.35 var(--font-sans); color: var(--muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .ppos { font: 500 var(--fs-11)/1.3 var(--font-mono); }

  /* PileActions: square icon buttons in fixed cells, two rows of three
     when the row is narrow, one row of six (a gap between the threes)
     when it is wide. */
  .pa { display: grid; gap: var(--space-1); grid-template-columns: repeat(3, var(--pa)); grid-template-rows: repeat(2, var(--pa)); grid-template-areas: 'hand bench attach' 'deck discard lost'; }
  @container pview (min-width: 560px) {
    .pa { grid-template-columns: repeat(3, var(--pa)) var(--space-2) repeat(3, var(--pa)); grid-template-rows: var(--pa); grid-template-areas: 'hand bench attach . deck discard lost'; }
  }
  .pab { box-sizing: border-box; width: var(--pa); height: var(--pa); min-width: 0; min-height: 0; display: grid; place-items: center; padding: 0; border-radius: var(--radius-s); }
  .pab :global(.icon) { width: 18px; height: 18px; }
  .pab:active:not(:disabled) { background: var(--selected); }
  /* Disabled: still in its cell and recognisable, but flat and faded. Its
     tooltip says why. */
  .pab:disabled { background: var(--panel-sunk); border-color: var(--line); color: var(--muted); cursor: not-allowed; }
  .pab:disabled :global(.icon) { opacity: 0.5; }
  @media (pointer: coarse) {
    .pab { border-radius: var(--radius-m); }
    .pab :global(.icon) { width: 20px; height: 20px; }
  }
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
    box-shadow: var(--shadow-dialog);
    pointer-events: none;
  }
  .section {
    margin: var(--space-4) 0 0;
    font: var(--fw-label) var(--fs-label)/var(--lh-label) var(--font-sans);
    letter-spacing: var(--ls-label);
    text-transform: uppercase;
    color: var(--muted);
  }
  .dl { width: calc(var(--cw) * 0.62 + 40px); }
  .where { display: flex; flex-wrap: wrap; gap: 3px; justify-content: center; }
  /* Where each copy is: a chip per place, tinted by kind of place. */
  .place {
    font: 500 var(--fs-11) var(--font-sans);
    padding: 1px 6px;
    border-radius: var(--radius-pill);
    border: 1px solid var(--control-line);
    background: var(--panel-sunk);
    color: var(--ink-soft);
    white-space: nowrap;
  }
  .place.active, .place.bench { background: var(--primary); border-color: var(--primary); color: var(--on-primary); }
  .place.hand { background: var(--selected); color: var(--ink); }
  .place.prizes { background: var(--warn-bg); border-color: var(--warn-line); color: var(--warn-ink); }
  .place.discard, .place.lost { background: var(--danger-bg); border-color: var(--danger-line); color: var(--danger); }
</style>
