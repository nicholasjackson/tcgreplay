<script>
  // Every event on the current branch, grouped by turn and then by whose
  // half of the turn it is, with annotations. A turn counts both players:
  // turn 3 is each player's third turn. Each turn, each half and each line
  // can be rewound to (on a branch of its own) or played from. With rail,
  // a list of the turns beside the lines jumps to any player's half and
  // shows the prizes left after each turn (the docked timeline).
  import { app } from '../lib/store.svelte.js';
  import { act } from '../lib/ws.js';
  import { startReplay } from '../lib/playback.svelte.js';
  import { groupTurns, prizeScores, startOf } from '../lib/timeline.js';

  let { rail = false } = $props();

  let list = $state();
  const log = $derived(app.snap?.log || []);
  const lastId = $derived(log.at(-1)?.id);
  const turns = $derived(groupTurns(log));
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
        {#if t.turn === 0}
          <button onclick={() => jump(startOf(t), 'setup')} aria-label="Rewind to the end of setup" title="Rewind to the end of setup">
            {@render rewindIcon()}
          </button>
        {:else}
          {#each t.halves as h (h.seat)}
            {#if h.seat}
              <button onclick={() => rewindHalf(h)} aria-label="Rewind to {whose(h.seat)} turn {h.turn}" title="Rewind to {whose(h.seat)} turn {h.turn}">
                <span class="dot seat-{h.seat}"></span>
              </button>
            {/if}
          {/each}
        {/if}
      </div>
    {/each}
  </nav>
{/if}

<div class="timeline" bind:this={list}>
  {#each turns as t (t.turn)}
    <!-- One section per turn, so each heading sticks only while its turn is
         in view and the next heading pushes it out instead of piling on. -->
    <section>
      <h3>
        {#if t.turn === 0}
          <span class="title">Setup</span>
          <button class="head-btn" onclick={() => jump(startOf(t), 'setup')} aria-label="Rewind to the end of setup" title="Rewind to the end of setup">
            {@render rewindIcon()}Rewind
          </button>
          <button class="head-btn" onclick={() => playStart(t)} aria-label="Play from the end of setup" title="Play from the end of setup">
            {@render playIcon()}Play
          </button>
        {:else}
          <span class="title">Turn {t.turn}</span>
          <button class="head-btn" onclick={() => jump(startOf(t), `turn-${t.turn}`)} aria-label="Rewind to the start of turn {t.turn}" title="Rewind to the start of this turn">
            {@render rewindIcon()}Rewind
          </button>
          <button class="head-btn" onclick={() => playStart(t)} aria-label="Play from the start of turn {t.turn}" title="Play from the start of this turn">
            {@render playIcon()}Play
          </button>
        {/if}
      </h3>
      {#each t.halves as h (h.seat)}
        {#if h.seat}
          <!-- One sub-heading per player's half, each with its own rewind and play. -->
          <h4 class="seat-{h.seat}">
            <span class="dot seat-{h.seat}"></span>
            <span class="title">{nameOf(h.seat)}</span>
            <button
              class="head-btn"
              onclick={() => rewindHalf(h)}
              aria-label="Rewind to the start of {whose(h.seat)} turn {h.turn}"
              title="Rewind to the start of this player's turn"
            >
              {@render rewindIcon()}Rewind
            </button>
            <button class="head-btn" onclick={() => playStart(h)} aria-label="Play from the start of {whose(h.seat)} turn {h.turn}" title="Play from the start of this player's turn">
              {@render playIcon()}Play
            </button>
          </h4>
        {/if}
        {#each h.lines as l (l.id)}
          <div
            class="line seat-{l.seat}"
            class:fresh={fresh && l.id > fresh}
            class:ahead={ahead(l.id)}
            class:now={step && l.id >= step.first && l.id <= step.last}
          >
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
            {#each l.annotations || [] as a}
              <div class="note">
                <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M4 20h4L19 9l-4-4L4 16z" /></svg>
                <span>{a}</span>
              </div>
            {/each}
          </div>
        {/each}
      {/each}
    </section>
  {:else}
    <p class="empty">No events yet.</p>
  {/each}
</div>

<style>
  /* Text keeps today's size up to 1920px wide and grows with the screen
     above it (12px at 1920px is 0.625vw). */
  .timeline {
    overflow-y: auto;
    flex: 1;
    min-width: 0;
    min-height: 0; /* fill what the panel's heading leaves, and scroll */
    padding: 0 0 40px;
    font-size: max(var(--fs-12), 0.625vw);
    line-height: 1.4;
  }
  svg { width: 1em; height: 1em; flex: none; fill: none; stroke: currentColor; stroke-width: 2.2; stroke-linecap: round; stroke-linejoin: round; }
  svg.solid { fill: currentColor; stroke: none; width: 0.85em; height: 0.85em; }
  .dot { width: 8px; height: 8px; flex: none; border-radius: 50%; }
  .dot.seat-p1 { background: var(--seat-p1); }
  .dot.seat-p2 { background: var(--seat-p2); }
  h3,
  h4 {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin: 0;
    font-weight: 600;
  }
  h3 {
    position: sticky;
    top: 0;
    z-index: 2; /* lines are positioned too; keep the heading above them */
    background: var(--panel);
    padding: var(--space-2) var(--space-3) var(--space-1) 14px;
    border-bottom: 1px solid var(--line);
    font-size: max(var(--fs-11), 0.5729vw);
    text-transform: uppercase;
    letter-spacing: var(--ls-label);
    color: var(--muted);
  }
  h4 {
    padding: var(--space-2) var(--space-3) var(--space-1) 14px;
    font-size: max(var(--fs-12), 0.625vw);
    color: var(--muted);
  }
  h4 .title { color: var(--ink); }
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
    font: 500 max(var(--fs-11), 0.5729vw) var(--font-sans);
    text-transform: none;
    letter-spacing: normal;
  }
  .head-btn:hover:not(:disabled) { border-color: var(--control-line); background: var(--hover); }
  button:focus-visible { outline: 2px solid var(--primary); outline-offset: 1px; }
  /* A line: its play and rewind first, then its number and its text. */
  .line { padding: var(--space-1) 14px var(--space-1) 6px; }
  .line.fresh { background: var(--hover); }
  .line:hover { background: var(--hover); }
  .line.now { background: var(--current); }
  .line.ahead { opacity: 0.5; }
  .row { display: flex; align-items: baseline; gap: var(--space-2); }
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
    font-size: max(var(--fs-12), 0.625vw);
  }
  .acts button:hover:not(:disabled) { background: var(--panel-sunk); color: var(--ink); }
  .line.now .acts button { background: var(--panel); color: var(--ink); }
  .id { flex: none; min-width: 34px; color: var(--muted); font: 500 max(var(--fs-11), 0.5729vw) var(--font-mono); font-variant-numeric: tabular-nums; }
  .words { min-width: 0; overflow-wrap: anywhere; }
  .label { font-weight: 600; margin-right: 4px; }
  .label::after { content: ':'; }
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

  /* On a touch screen every button is a fingertip's 44px. */
  @media (pointer: coarse) {
    .acts { width: calc(2 * var(--touch-target) + 2px); }
    .acts button { width: var(--touch-target); height: var(--touch-target); }
    .row { align-items: center; }
    .note { margin-left: calc(2 * var(--touch-target) + 2px + 34px + 2 * var(--space-2)); }
    .head-btn { min-height: var(--touch-target); min-width: var(--touch-target); }
    .turn-row button { width: var(--touch-target); height: var(--touch-target); }
  }
</style>
