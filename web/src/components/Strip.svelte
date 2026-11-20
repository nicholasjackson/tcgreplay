<script>
  // The middle of the mat: Stadium, turn, coin, a play area to drop
  // Trainers on, lingering effects and the turn buttons. On the landscape
  // mat (app.wide) it is the middle row and holds both Active Spots too:
  // the Stadium, your Active, "vs", the opponent's Active, the controls.
  import { app, canAct, oppActing } from '../lib/store.svelte.js';
  import { dropOnZone, endTurn, seatArgs } from '../lib/actions.js';
  import { dropZone } from '../lib/pointer.svelte.js';
  import { act } from '../lib/ws.js';
  import Card from './Card.svelte';
  import Stack from './Stack.svelte';

  const b = $derived(app.board);
  const snap = $derived(app.snap);
  const setup = $derived(b.phase === 'setup');
  const myTurn = $derived(b.turn_open && b.current === app.human);
  // zone makes the Stadium or the play area a drop zone.
  const zone = (name) => dropZone(name, (data) => dropOnZone(data, name));
  async function flip() {
    const r = await act('coin_flip', { n: 1 });
    if (Array.isArray(r)) app.caption = `Coin: ${r.join(', ')}`;
    setTimeout(() => (app.caption = ''), 2000);
  }
  // In review mode the controls act for whoever's turn it is.
  const review = $derived(snap?.mode === 'review');
  const actor = $derived(review && b.current ? b.current : app.human);
  const nameOf = (seat) => b.sides?.[seat]?.name || (seat === app.human ? 'You' : app.assistant);
  // A phone's strip is one row; its header shows whose turn it is.
  const phone = $derived(app.layout === 'phone');
  const wide = $derived(app.wide && !phone);
  // spotName captions an Active Spot with whose it is.
  const spotName = (seat) => (b.sides?.[seat]?.name ? `${b.sides[seat].name}'s Active` : seat === app.human ? 'Your Active' : `${app.assistant}'s Active`);
  // The two halves of a pair-played Stadium form one picture, left half the
  // lower collector number, so they show in print order whichever was
  // played first: turned a quarter anticlockwise, the left half ends up
  // at the bottom.
  const halves = $derived.by(() => {
    if (!b.stadium?.pair) return [];
    const both = [b.stadium, b.stadium.pair];
    return both.sort((x, y) => (x.def || '').localeCompare(y.def || '', undefined, { numeric: true }));
  });
  // whoseTurn names the player whose turn it is: "Your turn".
  function whoseTurn() {
    return b.current === app.human ? 'Your turn' : `${nameOf(b.current)}'s turn`;
  }
  // coins is the last flip's result: "H T".
  const coins = () => b.last_coins.map((c) => (c === 'heads' ? 'H' : 'T')).join(' ');
  // compact shows the coin as an icon alone: on a phone or touch screen.
  const compact = $derived(phone || app.layout === 'tablet');
</script>

<!-- Today's mat and the phone: one strip across the mat, the Stadium at
     the left, the turn in the middle and the turn buttons at the right. The
     landscape mat's middle row also holds both Active Spots. -->
<div class="strip" class:phone class:wide class:bar={!phone && !wide}>
  <div class="stadium" class:pair={!!b.stadium?.pair} class:empty={!b.stadium} {@attach zone('stadium')} role="region" aria-label="Stadium">
    {#if b.stadium?.pair}
      <!-- A Stadium played as a pair stands as one card, as TCG Live shows
           it: its two halves joined edge to edge into one picture, turned a
           quarter so it fills a single Stadium's place, with one outline and
           shadow and a hairline seam. Each half is still its own card to
           tap, drag or zoom. -->
      <div class="joined" class:sm={!wide} aria-label="{b.stadium.name} (pair)" role="group">
        {#each halves as half, i (half.iid)}
          <div class="half" class:top={i === 1}>
            <Card card={half} rotate={-90} lift={false} from="stadium" />
          </div>
        {/each}
      </div>
    {:else if b.stadium}
      <Card card={b.stadium} size={wide ? 'lg' : 'sm'} from="stadium" />
    {:else}
      <span>Stadium</span>
    {/if}
  </div>
  {#if phone}
    <!-- A phone's Stadium name; a short phone, whose header has no room
         for the turn, shows the turn tag and whose turn it is over it. -->
    <span class="phone-mid">
      <span class="turn phone-turn" class:mine={myTurn}>{#if setup}<strong>Setup</strong>{:else}<span class="tag">T{b.turn}</span> <strong>{whoseTurn()}</strong>{/if}</span>
      {#if b.stadium}
        <span class="stadium-name"><small>Stadium</small><strong>{b.stadium.name}{#if b.stadium.pair}<span class="pair-note">{' · pair'}</span>{/if}</strong></span>
      {/if}
    </span>
  {:else if !wide && b.stadium}
    <span class="stadium-name"><small>Stadium</small><strong>{b.stadium.name}{#if b.stadium.pair}<span class="pair-note">{' · pair'}</span>{/if}</strong></span>
  {/if}

  {#if wide}
    <div class="actives">
      {#each [app.human, app.claude] as seat, i (seat)}
        {#if i}<span class="vs" aria-hidden="true">vs</span>{/if}
        <figure class="spot {seat}">
          <Stack {seat} slot="active" stack={b.sides[seat].active} label="{seat} Active" />
          <figcaption>{spotName(seat)}</figcaption>
        </figure>
      {/each}
    </div>
  {/if}

  <div class="controls">
    {#if !phone}
      <!-- The turn tag and whose turn it is (a phone shows them in its
           header, or beside the Stadium on a short phone). The landscape
           mat spells the turn out. -->
      <div class="status">
        {#if setup}
          <div class="turn" class:mine={myTurn}>Setup</div>
        {:else if wide}
          <div class="turn" class:mine={myTurn}><span class="tag">Turn {b.turn}</span> · <strong>{whoseTurn()}</strong></div>
        {:else}
          <div class="turn" class:mine={myTurn}><span class="tag">T{b.turn}</span> <strong>{whoseTurn()}</strong></div>
        {/if}
      </div>
    {/if}

    <div class="buttons">
      <button class="coin" onclick={flip} title="Flip a coin" aria-label={b.last_coins?.length ? `Flip coin (last: ${coins()})` : 'Flip coin'}>
        <svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="8.5" /><circle cx="12" cy="12" r="4" /></svg>
        {#if b.last_coins?.length}<span class="result">{coins()}</span>{:else if !compact}<span>Flip coin</span>{/if}
      </button>
      {#if setup}
        <button onclick={() => act('mulligan')}>Mulligan</button>
        <button class="primary" onclick={() => act('start_game', { first: app.human })}>I go first</button>
        <button onclick={() => act('start_game', { first: app.claude })}>{app.assistant} {phone ? 'first' : 'goes first'}</button>
      {:else}
        {#if snap?.open_action}
          <button onclick={() => act('end_action')} title="Close the open compound action ({snap.open_action})" disabled={oppActing()}>End action</button>
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
      <div class="play" {@attach zone('play')} role="region" aria-label="Play area">
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
    z-index: 2;
    color: var(--mat-ink);
    --muted: var(--mat-muted);
  }
  /* Today's mat: a strip across the mat between the two halves. */
  .strip.bar {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    margin: 0 var(--space-5);
    padding: var(--space-1) var(--space-3);
    border-radius: var(--radius-m);
    background: var(--mat-strip);
  }
  /* The Stadium spot: a recessed box labelled "Stadium" when empty. */
  .stadium {
    flex: none;
    display: grid;
    place-items: center;
    border-radius: var(--radius-xs);
    color: var(--mat-muted);
    font-size: max(var(--fs-label), 0.85em);
    transition: background 0.15s;
  }
  .stadium.empty {
    width: calc(var(--cw) * 0.62);
    aspect-ratio: 63 / 88;
    background: var(--mat-slot);
    box-shadow: inset 0 0 0 1px var(--mat-slot-line);
    font-size: max(var(--fs-9), 0.7em);
  }
  @media (pointer: coarse) { .stadium.empty { width: max(44px, calc(var(--cw) * 0.62)); } }
  .stadium-name { display: flex; flex-direction: column; min-width: 0; line-height: 1.3; }
  .stadium-name small { font-size: max(var(--fs-label), 0.92em); color: var(--mat-muted); }
  .stadium-name strong { font-size: max(var(--fs-ui), 1.08em); font-weight: var(--fw-ui-title); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .pair-note { font-weight: 400; color: var(--mat-muted); }
  /* A pair is one card the size of a single Stadium (--jw wide): each half,
     turned a quarter, is as tall as half the card, so its width is half the
     card's height (63/88 of a card is 0.698 of its width). The halves are
     centred a quarter and three quarters of the way down. One outline and
     shadow go round the pair, with a hairline seam between the halves. */
  .joined {
    --jw: calc(var(--cw) * 1.25);
    position: relative;
    width: var(--jw);
    aspect-ratio: 63 / 88;
    border-radius: calc(var(--jw) * 0.05);
    box-shadow: var(--shadow-card);
  }
  .joined::after {
    content: '';
    position: absolute;
    left: 0;
    right: 0;
    top: 50%;
    border-top: 1px solid var(--mat-slot-line);
    pointer-events: none;
  }
  .joined.sm { --jw: calc(var(--cw) * 0.62); }
  @media (pointer: coarse) { .joined.sm { --jw: max(44px, calc(var(--cw) * 0.62)); } }
  .joined .half { position: absolute; left: 50%; top: 75%; width: 0; height: 0; }
  .joined .half.top { top: 25%; }
  /* The pair is one card, so it carries one basis mark: the lower half's. */
  .joined .half.top :global(.basis) { display: none; }
  .joined :global(.card) {
    --w: calc(var(--jw) * 0.698);
    position: absolute;
    left: calc(var(--w) / -2);
    top: calc(var(--w) * 88 / 63 / -2);
    box-shadow: none;
  }
  .controls { display: flex; align-items: center; gap: var(--space-3); }
  .bar .controls { flex: 1; min-width: 0; }
  .status { display: flex; align-items: center; gap: var(--space-2); }
  .bar .status { flex: 1; justify-content: center; min-width: 0; }
  /* The turn tag in mono, then whose turn it is. */
  .turn { display: flex; align-items: center; gap: var(--space-2); font-size: max(var(--fs-ui-title), 1.15em); white-space: nowrap; }
  .turn strong { font-weight: var(--fw-ui-title); }
  .tag {
    padding: 0.2em 0.6em;
    border-radius: var(--radius-xs);
    background: var(--brand-navy);
    color: var(--turn-tag-ink);
    font: var(--fw-num) max(var(--fs-ui-s), 0.85em) var(--font-mono);
  }
  .buttons { display: flex; align-items: center; gap: var(--space-2); flex-wrap: wrap; justify-content: flex-end; }
  .buttons button {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    min-height: max(34px, 2.6em);
    padding: 0 0.9em;
    border-radius: var(--radius-s);
    border: 1px solid var(--mat-slot-line);
    background: var(--mat-zone);
    color: var(--mat-ink);
    font: 600 max(var(--fs-ui), 1.08em) var(--font-sans);
    white-space: nowrap;
  }
  .buttons button:hover:not(:disabled) { border-color: var(--mat-muted); }
  .buttons button.primary { border-color: var(--accent); background: var(--accent); color: var(--on-accent); font-weight: 700; }
  .buttons button.primary:hover:not(:disabled) { border-color: var(--accent); }
  .coin svg { width: 1.3em; height: 1.3em; flex: none; fill: none; stroke: currentColor; stroke-width: 2; }
  .coin .result { font-family: var(--font-mono); font-weight: var(--fw-num); }
  .play {
    position: absolute;
    inset: 0;
    z-index: 3;
    display: grid;
    place-items: center;
    border: 2px dashed var(--accent);
    border-radius: var(--radius-m);
    color: var(--mat-ink);
    background: var(--mat-strip);
    font-size: max(var(--fs-ui), 1em);
  }
  .effects { display: flex; gap: var(--space-1); flex-wrap: wrap; }
  .bar .effects { position: absolute; top: calc(100% + var(--space-1)); right: var(--space-3); z-index: 3; justify-content: flex-end; }
  .effect {
    background: var(--mat-chip);
    color: var(--mat-ink);
    box-shadow: inset 0 0 0 1px var(--mat-slot-line);
    font-size: max(var(--fs-ui-s), 0.92em);
    padding: 0.3em 0.6em;
    border-radius: var(--radius-pill);
  }
  .effect button { background: none; border: none; color: inherit; cursor: pointer; padding: 0 0 0 4px; font-size: inherit; }

  /* The landscape mat's middle row: the Stadium at the left, the two
     Active panels side by side, each edged in its seat's colour, and the
     turn controls at the right. The Actives are a little larger than the
     bench. */
  .strip.wide {
    display: grid;
    grid-template-columns: 1fr auto 1fr;
    align-items: center;
    gap: calc(var(--cw) * 0.3);
    /* Room above for the Active Spots' captions. */
    padding: 1.2em 12px 0;
  }
  .wide .stadium { justify-self: end; }
  .wide .stadium.empty { width: calc(var(--cw) * 1.1); }
  .wide .controls { position: relative; justify-self: start; flex-direction: column; align-items: flex-start; gap: calc(var(--cw) * 0.08); max-width: calc(var(--cw) * 2.8); }
  .wide .buttons { justify-content: flex-start; }
  /* The play area shows only during a drag; it floats under the controls
     so the row, and so every card, keeps its size. */
  .wide .play { position: absolute; inset: auto; top: calc(100% + 6px); left: 0; padding: 0.8em 1.2em; white-space: nowrap; }
  .wide :global(.card.lg) { --w: calc(var(--cw) * 1.1); }
  /* A pair-played Stadium matches a single Stadium on the landscape mat. */
  .wide .joined { --jw: calc(var(--cw) * 1.1); }
  .actives { display: flex; align-items: flex-start; gap: calc(var(--cw) * 0.2); }
  /* The caption sits on the spot's top edge, taking no row of its own. */
  .spot { margin: 0; position: relative; --seat-c: var(--seat-p1-mat); }
  .spot.p2 { --seat-c: var(--seat-p2-mat); }
  .spot figcaption {
    position: absolute;
    bottom: 100%;
    left: 50%;
    translate: -50% 0;
    font-size: 0.8em;
    font-weight: var(--fw-ui-title);
    color: var(--seat-c);
    white-space: nowrap;
  }
  .vs { align-self: center; font-weight: 700; font-size: 1.1em; color: var(--mat-muted); }

  /* Phone: one compact row between the halves: the Stadium and its name,
     the coin and the turn buttons. The play area covers the row while a
     card is dragged. */
  .strip.phone {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin: 0 var(--space-3);
    padding: 2px 2px 2px var(--space-2);
    border-radius: var(--radius-m);
    background: var(--mat-strip);
    font-size: var(--fs-ui-s);
  }
  .phone-mid { flex: 1; min-width: 0; display: flex; flex-direction: column; justify-content: center; }
  .phone .stadium-name small { font-size: var(--fs-10); }
  .phone .stadium-name strong { font-size: var(--fs-ui-s); }
  /* The turn shows here only on a short phone (App.svelte's header drops
     it below 740px tall), over the Stadium's name in place of its
     "Stadium" caption. */
  .phone-turn { display: none; min-width: 0; gap: var(--space-1); font-size: var(--fs-ui-s); line-height: 1.3; }
  .phone-turn strong { overflow: hidden; text-overflow: ellipsis; }
  .phone-turn .tag { flex: none; padding: 0 0.4em; font-size: var(--fs-num-s); }
  @media (max-height: 739px) {
    .phone-turn { display: flex; }
    .phone .stadium-name small { display: none; }
  }
  .phone .controls { margin-left: auto; gap: var(--space-1); }
  .phone .buttons { flex-wrap: nowrap; gap: var(--space-1); }
  .phone .buttons button { padding: 0 var(--space-2); font-size: var(--fs-ui); }
  .phone .buttons button.primary { padding: 0 var(--space-3); }
  .phone .effects { position: absolute; top: 100%; left: var(--space-2); right: var(--space-2); z-index: 3; }
  @media (pointer: coarse) {
    .buttons button { min-height: var(--touch-target); }
    .coin { justify-content: center; min-width: var(--touch-target); }
  }
</style>
