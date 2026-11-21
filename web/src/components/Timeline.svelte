<script>
  // Every event on the current branch, grouped by turn and then by whose
  // half of the turn it is, with annotations. A turn counts both players:
  // turn 3 is each player's third turn. One heading per player's half
  // ("T4 · Bob"), and setup's; each half and each line can be rewound to
  // (on a branch of its own) or played from. With rail,
  // a list of the turns beside the lines jumps to any player's half and
  // shows the prizes left after each turn (the docked timeline).
  import { app } from '../lib/store.svelte.js';
  import { act } from '../lib/ws.js';
  import { startReplay } from '../lib/playback.svelte.js';
  import { groupTurns, prizeScores, startOf } from '../lib/timeline.js';

  let { rail = false } = $props();
  // bare drops the per-line and heading buttons, as the design has the
  // docked timeline and a touch screen's timeline beside the board: there
  // the turn list rewinds each half (docked), and tapping a line shows its
  // Play and Rewind. A phone's sheet keeps the buttons.
  const bare = $derived(rail || app.layout === 'tablet');
  let picked = $state(null);
  const pick = (id) => (picked = picked === id ? null : id);

  let list = $state();
  const log = $derived(app.snap?.log || []);
  const lastId = $derived(log.at(-1)?.id);
  const turns = $derived(groupTurns(log));
  const halves = $derived(turns.flatMap((t) => t.halves));
  const scores = $derived(prizeScores(turns));
  const nameOf = (seat) => app.snap?.board?.sides?.[seat]?.name || (seat === app.human ? 'You' : app.assistant);
  // whose names a player's turn in a label: "your" or "Bob's".
  const whose = (seat) => (seat === app.human ? 'your' : `${nameOf(seat)}'s`);
  // jump rewinds to just after an event on a branch of its own; the branch
  // left behind keeps its head (switch back in the branch picker).
  function jump(event, name) {
    act('rewind', { event, name });
  }
  // playFrom replays from just before event id (the previous line) to the
  // end of the branch.
  function playFrom(id) {
    const i = log.findIndex((l) => l.id === id);
    startReplay(i > 0 ? log[i - 1].id : 0);
  }
  // A turn's or a half's Play replays from its start, the position its
  // Rewind goes to.
  const playStart = (h) => startReplay(startOf(h));
  const rewindHalf = (h) => jump(startOf(h), `turn-${h.turn}-${h.seat}`);
  // The move on the board during a replay, and whether a line is still
  // ahead of the board (held or not yet replayed).
  const step = $derived(app.replay && app.replay.index >= 0 ? app.replay.steps[app.replay.index] : null);
  const ahead = (id) => (app.replay || app.held) && id > app.viewHead;
  // The turn the board is in, lit in the turn list.
  const nowTurn = $derived(log.findLast((l) => l.id <= app.viewHead)?.turn ?? 0);
  // Keep the move being replayed in view.
  $effect(() => {
    if (step) queueMicrotask(() => list?.querySelector('.line.now')?.scrollIntoView({ block: 'nearest', behavior: 'smooth' }));
  });

  let seen = 0;
  let fresh = $state(0);

  $effect(() => {
    const last = log.at(-1)?.id ?? 0;
    if (last !== seen) {
      fresh = seen;
      seen = last;
      queueMicrotask(() => list?.scrollTo({ top: list.scrollHeight, behavior: 'smooth' }));
    }
  });
</script>

{#snippet rewindIcon()}
  <svg class="rw" viewBox="0 0 24 24" aria-hidden="true"><path d="M4 12a8 8 0 1 0 2.4-5.7" /><path d="M4 4v4.5h4.5" /></svg>
{/snippet}
{#snippet playIcon()}
  <svg class="solid" viewBox="0 0 24 24" aria-hidden="true"><path d="M6 4v16l14-8z" /></svg>
{/snippet}

{#if rail}
  <nav class="rail" aria-label="Turns">
    {#each turns as t (t.turn)}
      {@const s = scores.get(t.turn)}
      <div class="turn-row" class:current={t.turn === nowTurn}>
        <span class="turn-label">{t.turn === 0 ? 'Setup' : `T${t.turn}`}</span>
        <span class="score" aria-label="Prizes left after {t.turn === 0 ? 'setup' : `turn ${t.turn}`}: {nameOf('p1')} {s.p1}, {nameOf('p2')} {s.p2}">{s.p1}–{s.p2}</span>
        {#each t.halves as h (h.seat)}
          {#if h.seat}
            <button onclick={() => rewindHalf(h)} aria-label="Rewind to {whose(h.seat)} turn {h.turn}" title="Rewind to {whose(h.seat)} turn {h.turn}">
              <span class="dot seat-{h.seat}"></span>
            </button>
          {/if}
        {/each}
      </div>
    {/each}
  </nav>
{/if}

<div class="timeline" class:bare bind:this={list}>
  {#each halves as h (h.lines[0].id)}
    <!-- One section per player's half of a turn (and one for setup), so
         each heading sticks only while its half is in view and the next
         heading pushes it out instead of piling on. -->
    <section>
      <h3>
        {#if h.seat}
          <span class="tag">T{h.turn}</span>
          <span class="dot seat-{h.seat}"></span>
          <span class="title">{nameOf(h.seat)}</span>
          {#if !bare}
            <button class="head-btn" onclick={() => rewindHalf(h)} aria-label="Rewind to the start of {whose(h.seat)} turn {h.turn}" title="Rewind to the start of this player's turn">
              {@render rewindIcon()}Rewind
            </button>
            <button class="head-btn" onclick={() => playStart(h)} aria-label="Play from the start of {whose(h.seat)} turn {h.turn}" title="Play from the start of this player's turn">
              {@render playIcon()}Play
            </button>
          {/if}
        {:else}
          <span class="tag">Setup</span>
          <span class="title"></span>
          {#if !bare}
            <button class="head-btn" onclick={() => jump(startOf(h), 'setup')} aria-label="Rewind to the end of setup" title="Rewind to the end of setup">
              {@render rewindIcon()}Rewind
            </button>
            <button class="head-btn" onclick={() => playStart(h)} aria-label="Play from the end of setup" title="Play from the end of setup">
              {@render playIcon()}Play
            </button>
          {/if}
        {/if}
      </h3>
      {#each h.lines as l (l.id)}
        <div
          class="line seat-{l.seat}"
          class:fresh={fresh && l.id > fresh}
          class:ahead={ahead(l.id)}
          class:now={step && l.id >= step.first && l.id <= step.last}
          class:picked={bare && picked === l.id}
        >
          {#if bare}
            <!-- Without per-line buttons (a touch screen beside the board,
                 or docked below), tapping a line shows its Play and Rewind,
                 the half headings' buttons: an icon and one word. -->
            <div class="row" role="button" tabindex="0" aria-expanded={picked === l.id} onclick={() => pick(l.id)} onkeydown={(ev) => (ev.key === 'Enter' || ev.key === ' ') && (ev.preventDefault(), pick(l.id))}>
              <span class="id">#{l.id}</span>
              <span class="words">
                {#if l.label}<span class="label">{l.label}</span>{/if}
                <span class="text">{l.text}</span>
              </span>
            </div>
            {#if picked === l.id}
              <div class="line-acts">
                <button class="head-btn" onclick={() => playFrom(l.id)} aria-label="Play from #{l.id}" title="Play from here">{@render playIcon()}Play</button>
                {#if l.id !== lastId}
                  <button class="head-btn" onclick={() => jump(l.id)} aria-label="Rewind to just after #{l.id}" title="Rewind to here">{@render rewindIcon()}Rewind</button>
                {/if}
              </div>
            {/if}
          {:else}
            <div class="row">
              <span class="acts">
                <button onclick={() => playFrom(l.id)} aria-label="Play from #{l.id}" title="Play from here">{@render playIcon()}</button>
                <!-- Rewinding to just after the last event would change nothing. -->
                {#if l.id !== lastId}
                  <button onclick={() => jump(l.id)} aria-label="Rewind to just after #{l.id}" title="Rewind to here">{@render rewindIcon()}</button>
                {/if}
              </span>
              <span class="id">#{l.id}</span>
              <span class="words">
                {#if l.label}<span class="label">{l.label}</span>{/if}
                <span class="text">{l.text}</span>
              </span>
            </div>
          {/if}
          {#each l.annotations || [] as a}
            <div class="note">
              <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 20h4L19 9l-4-4L4 16z" /></svg>
              <span>{a}</span>
            </div>
          {/each}
        </div>
      {/each}
    </section>
  {:else}
    <p class="empty">No events yet.</p>
  {/each}
</div>

<style>
  /* Text keeps the design's 13px up to 1920px wide and grows a little
     above it, to 1.15 times at 3840px (--fs-grow-*, app.css). */
  .timeline {
    overflow-y: auto;
    flex: 1;
    min-width: 0;
    min-height: 0; /* fill what the panel's heading leaves, and scroll */
    padding: 0 0 40px;
    font-size: var(--fs-grow-13);
    line-height: 1.4;
  }
  svg { width: 1em; height: 1em; flex: none; fill: none; stroke: currentColor; stroke-width: 2.2; stroke-linecap: round; stroke-linejoin: round; }
  svg.solid { fill: currentColor; stroke: none; width: 0.85em; height: 0.85em; }
  .dot { width: 8px; height: 8px; flex: none; border-radius: 50%; }
  .dot.seat-p1 { background: var(--seat-p1); }
  .dot.seat-p2 { background: var(--seat-p2); }
  /* A half's heading: the turn tag in mono ink, the player's dot and name
     in muted, then its Rewind and Play. */
  h3 {
    position: sticky;
    top: 0;
    z-index: 2; /* lines are positioned too; keep the heading above them */
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin: 0;
    padding: 10px 10px var(--space-1) 14px;
    background: var(--panel);
    font-size: var(--fs-grow-12);
    font-weight: 600;
    color: var(--muted);
  }
  .bare h3 { padding-block: 10px 6px; }
  .tag { font-family: var(--font-mono); color: var(--ink); }
  .title { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  /* A heading's Rewind and Play: small outlined buttons, icon and word. */
  .head-btn {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    height: 26px;
    padding: 0 var(--space-2);
    border-radius: var(--radius-s);
    border: 1px solid var(--control-line);
    background: var(--panel);
    color: var(--ink);
    font: 500 var(--fs-grow-11) var(--font-sans);
    white-space: nowrap;
  }
  .head-btn:hover:not(:disabled) { border-color: var(--control-line); background: var(--hover); }
  button:focus-visible, .row:focus-visible { outline: 2px solid var(--primary); outline-offset: 1px; }
  /* A line: its play and rewind first, then its number and its text. */
  .line { padding: var(--space-1) 14px var(--space-1) 6px; }
  .bare .line { padding-left: 14px; }
  .line.fresh { background: var(--hover); }
  .line:hover { background: var(--hover); }
  .line.now { background: var(--current); }
  .line.ahead { opacity: 0.5; }
  .row { display: flex; align-items: center; gap: var(--space-2); }
  .bare .row { cursor: pointer; border-radius: var(--radius-xs); }
  .acts { flex: none; display: flex; gap: 2px; align-self: flex-start; width: 50px; }
  .acts button {
    display: grid;
    place-items: center;
    width: 24px;
    height: 24px;
    padding: 0;
    border: 0;
    border-radius: 5px;
    background: none;
    color: var(--muted);
    font-size: var(--fs-grow-11);
  }
  .acts button:hover:not(:disabled) { background: var(--panel-sunk); color: var(--ink); }
  .line.now .acts button { background: var(--panel); color: var(--ink); }
  .id { flex: none; width: 34px; color: var(--muted); font: 500 var(--fs-grow-11) var(--font-mono); font-variant-numeric: tabular-nums; }
  .words { min-width: 0; overflow-wrap: anywhere; color: var(--ink); }
  .label { font-weight: 600; margin-right: 4px; }
  .label::after { content: ':'; }
  /* A picked line's Play and Rewind, under its text. */
  .line-acts { display: flex; flex-wrap: wrap; gap: var(--space-2); margin: var(--space-1) 0 2px calc(34px + var(--space-2)); }
  .note {
    display: flex;
    gap: var(--space-2);
    margin: var(--space-1) 0 2px calc(50px + 34px + 2 * var(--space-2));
    padding: var(--space-2) 10px;
    background: var(--note-bg);
    border: 1px solid var(--note-line);
    border-radius: var(--radius-s);
    color: var(--note-ink);
    line-height: 1.45;
  }
  .bare .note { margin-left: calc(34px + var(--space-2)); }
  .note svg { margin-top: 0.15em; stroke: var(--primary); stroke-width: 2; }
  .empty { color: var(--muted); padding: 0 var(--space-3); }

  /* The docked timeline's turn list: a row per turn with its prizes left
     and a rewind per player's half, in play order. */
  .rail {
    flex: none;
    width: 200px;
    overflow-y: auto;
    padding: var(--space-2);
    border-right: 1px solid var(--line);
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .turn-row { display: flex; align-items: center; gap: 6px; padding: 6px var(--space-2); border-radius: var(--radius-s); }
  .turn-row.current { background: var(--current); }
  .turn-label { flex: 1; font: 600 var(--fs-13) var(--font-mono); color: var(--muted); }
  .turn-row.current .turn-label { color: var(--ink); }
  .score { font: 500 var(--fs-11) var(--font-mono); color: var(--muted); font-variant-numeric: tabular-nums; }
  .turn-row button {
    display: grid;
    place-items: center;
    width: 26px;
    height: 24px;
    padding: 0;
    border-radius: 5px;
    border: 1px solid var(--control-line);
    background: var(--panel);
    color: var(--ink);
    font-size: var(--fs-12);
  }
  .turn-row button:hover:not(:disabled) { border-color: var(--control-line); background: var(--hover); }

  /* On a touch screen every button is a fingertip's 44px, and every line
     at least as tall. */
  @media (pointer: coarse) {
    .acts { width: calc(2 * var(--touch-target) + 2px); }
    .acts button { width: var(--touch-target); height: var(--touch-target); }
    .note { margin-left: calc(2 * var(--touch-target) + 2px + 34px + 2 * var(--space-2)); }
    .bare .note { margin-left: calc(34px + var(--space-2)); }
    .bare .row { min-height: var(--touch-target); }
    .head-btn { min-height: var(--touch-target); min-width: var(--touch-target); border-radius: var(--radius-m); }
    .turn-row button { width: var(--touch-target); height: var(--touch-target); }
  }
</style>
