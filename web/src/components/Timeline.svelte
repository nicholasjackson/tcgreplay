<script>
  // Every event on the current branch, grouped by turn and then by whose
  // half of the turn it is, with annotations. A turn counts both players:
  // turn 3 is each player's third turn.
  import { app } from '../lib/store.svelte.js';
  import { act } from '../lib/ws.js';
  import { startReplay } from '../lib/playback.svelte.js';

  let list = $state();
  const log = $derived(app.snap?.log || []);
  const turns = $derived.by(() => {
    const out = [];
    for (const l of log) {
      let t = out.at(-1);
      if (!t || t.turn !== l.turn) out.push((t = { turn: l.turn, lines: [], halves: [] }));
      t.lines.push(l);
      let h = t.halves.at(-1);
      const seat = l.turn_seat || '';
      if (!h || h.seat !== seat) t.halves.push((h = { turn: l.turn, seat, lines: [] }));
      h.lines.push(l);
    }
    return out;
  });
  const nameOf = (seat) => app.snap?.board?.sides?.[seat]?.name || (seat === app.human ? 'You' : app.assistant);
  const whose = (seat) => (seat === app.human ? 'Your turn' : `${nameOf(seat)}'s turn`);
  // startOf is where a player's turn's review begins (h is setup or one
  // player's half of a turn): the turn has started and the opening draw is
  // in hand, before the player's first choice. For setup, the end of setup.
  function startOf(h) {
    const lines = h.lines;
    if (h.turn === 0) return lines.at(-1).id;
    let i = lines.findIndex((l) => /^turn \d+ start/.test(l.text));
    if (i < 0) return lines[0].id;
    const through = (action) => {
      while (i + 1 < lines.length && lines[i + 1].action === action) i++;
    };
    through(lines[i].action);
    // Imported games log the opening draw as its own action.
    if (lines[i + 1]?.text.includes(' drew ')) {
      i++;
      through(lines[i].action);
    }
    return lines[i].id;
  }
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
  // The move on the board during a replay, and whether a line is still
  // ahead of the board (held or not yet replayed).
  const step = $derived(app.replay && app.replay.index >= 0 ? app.replay.steps[app.replay.index] : null);
  const ahead = (id) => (app.replay || app.held) && id > app.viewHead;
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

<div class="timeline" bind:this={list}>
  {#each turns as t (t.turn)}
    <!-- One section per turn, so each heading sticks only while its turn is
         in view and the next heading pushes it out instead of piling on. -->
    <section>
      <h3>
        {#if t.turn === 0}
          <button class="turn" onclick={() => jump(startOf(t), 'setup')} title="Rewind to the end of setup">
            Setup <span class="rw">↺</span>
          </button>
        {:else}
          Turn {t.turn}
        {/if}
        <button class="play" onclick={() => playFrom(t.lines[0].id)} title="Play from the start of this turn">▶</button>
      </h3>
      {#each t.halves as h (h.seat)}
        {#if h.seat}
          <!-- One sub-heading per player's half, each with its own rewind and play. -->
          <h4 class="seat-{h.seat}">
            <button
              class="turn"
              onclick={() => jump(startOf(h), `turn-${h.turn}-${h.seat}`)}
              title="Rewind to the start of this player's turn"
            >
              {whose(h.seat)} <span class="rw">↺</span>
            </button>
            <button class="play" onclick={() => playFrom(h.lines[0].id)} title="Play from the start of this player's turn">▶</button>
          </h4>
        {/if}
        {#each h.lines as l (l.id)}
          <div
            class="line seat-{l.seat}"
            class:fresh={fresh && l.id > fresh}
            class:ahead={ahead(l.id)}
            class:now={step && l.id >= step.first && l.id <= step.last}
          >
            <span class="acts">
              <button onclick={() => playFrom(l.id)} title="Play from here">▶</button>
              <button onclick={() => jump(l.id)} title="Rewind to just after #{l.id}">↺</button>
            </span>
            <span class="id">#{l.id}</span>
            {#if l.label}<span class="label">{l.label}</span>{/if}
            <span class="text">{l.text}</span>
            {#each l.annotations || [] as a}
              <div class="note">✎ {a}</div>
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
  .timeline {
    overflow-y: auto;
    flex: 1;
    min-height: 0; /* fill what the panel's heading leaves, and scroll */
    padding: 8px 10px 40px;
    font-size: 12px;
    line-height: 1.35;
  }
  h3 {
    position: sticky;
    top: -8px;
    z-index: 2; /* lines are positioned too; keep the heading above them */
    background: var(--panel);
    margin: 10px 0 4px;
    padding: 4px 0;
    border-bottom: 1px solid var(--line);
    font-size: 12px;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    color: var(--muted);
  }
  h4 {
    margin: 6px 0 2px;
    padding: 2px 6px;
    border-left: 3px solid transparent;
    font-size: 11px;
    font-weight: 600;
    color: var(--muted);
  }
  h4.seat-p1 { border-left-color: var(--p1); }
  h4.seat-p2 { border-left-color: var(--p2); }
  h3 .turn,
  h4 .turn {
    all: unset;
    cursor: pointer;
    text-transform: inherit;
    letter-spacing: inherit;
  }
  h3 .turn:hover,
  h4 .turn:hover { color: var(--accent); }
  .rw { opacity: 0; transition: opacity 0.15s; }
  h3 .turn:hover .rw,
  h4 .turn:hover .rw { opacity: 1; }
  .line {
    position: relative;
    padding: 3px 6px;
    border-left: 3px solid transparent;
    border-radius: 3px;
  }
  .line:hover { background: var(--hover); }
  .acts {
    position: absolute;
    top: 2px;
    right: 2px;
    display: flex;
    gap: 2px;
    opacity: 0;
    transition: opacity 0.15s;
  }
  .acts button { padding: 0 6px; font-size: 12px; }
  .line:hover .acts { opacity: 1; }
  .line.ahead { opacity: 0.4; }
  .line.now { background: var(--fresh); outline: 1px solid var(--accent); }
  h3 .play,
  h4 .play { all: unset; cursor: pointer; margin-left: 8px; opacity: 0.5; }
  h3 .play:hover,
  h4 .play:hover { opacity: 1; color: var(--accent); }
  .line.seat-p1 { border-left-color: var(--p1); }
  .line.seat-p2 { border-left-color: var(--p2); }
  .line.fresh { background: var(--fresh); }
  .id { color: var(--muted); margin-right: 4px; font-variant-numeric: tabular-nums; }
  .label { font-weight: 600; margin-right: 4px; }
  .label::after { content: ':'; }
  .note {
    margin: 3px 0 2px 10px;
    padding: 4px 8px;
    background: var(--note);
    border-radius: 4px;
    font-style: italic;
  }
  .empty { color: var(--muted); }
</style>
