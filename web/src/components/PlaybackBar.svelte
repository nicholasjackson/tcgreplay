<script>
  // The playback controls: previous step, play or pause, next step, the
  // board's position, a scrubber coloured by whose turn each stretch is,
  // the speed and Go live. where places them: 'side' heads the timeline
  // beside the board (one row over the scrubber), 'dock' is the docked timeline's one
  // control row, and 'overlay' floats over the board while the timeline is
  // hidden or a phone's sheet, showing only during a replay or held moves.
  // An autoplay of the assistant's moves can be paused and skipped, not
  // stepped or scrubbed.
  import { app } from '../lib/store.svelte.js';
  import { goLive, heldMoves, playOrPause, seekEvent, setAutoplay, setSpeed, speeds, stepBy } from '../lib/playback.svelte.js';
  import { groupTurns } from '../lib/timeline.js';

  let { where = 'overlay' } = $props();

  const r = $derived(app.replay);
  const held = $derived(app.held ? heldMoves() : 0);
  const head = $derived(app.snap?.head ?? 0);
  const at = $derived(app.viewHead);
  const live = $derived(!r && !app.held);
  const current = $derived(r && r.index >= 0 ? r.steps[r.index] : null);
  const turns = $derived(groupTurns(app.snap?.log || []));
  const touch = $derived(app.layout === 'tablet');

  // The scrubber's stops (the events it can move to, in order) and its
  // stretches, one per player's half, as long as the half has events:
  // played ones in the player's colour, the one the board is in split into
  // played (primary) and still ahead, and the rest ahead. Setup, often a
  // hundred events of dealing, is shortened to about one half's length,
  // stopping at evenly spaced events and always its last.
  const scrub = $derived.by(() => {
    const halves = turns.flatMap((t) => t.halves);
    const later = halves.filter((h) => h.seat);
    const avg = later.length ? Math.round(later.reduce((n, h) => n + h.lines.length, 0) / later.length) : 8;
    const stops = [];
    const stretches = [];
    for (const h of halves) {
      let ids = h.lines.map((l) => l.id);
      if (!h.seat && ids.length > avg) {
        const k = Math.max(2, avg);
        ids = Array.from({ length: k }, (_, j) => ids[Math.round((j * (ids.length - 1)) / (k - 1))]);
      }
      stops.push(...ids);
      const played = ids.filter((id) => id <= at).length;
      const kind = h.seat || 'setup';
      if (played === ids.length) stretches.push({ kind, n: ids.length });
      else if (played === 0) stretches.push({ kind: 'ahead', n: ids.length });
      else stretches.push({ kind: 'now', n: played }, { kind: 'ahead', n: ids.length - played });
    }
    return { stops, stretches, index: Math.max(0, stops.findLastIndex((id) => id <= at)) };
  });

  // The turn the step is in, counting both players, and whose half it is.
  function turnOf(board) {
    if (!board.turn) return 'Setup';
    const name = board.sides?.[board.current]?.name || (board.current === app.human ? 'You' : app.assistant);
    return `T${board.turn} · ${name}`;
  }
</script>

{#snippet scrubber()}
  <div class="scrub">
    <span class="stretches" aria-hidden="true">
      {#each scrub.stretches as s, i (i)}<span class="s-{s.kind}" style:flex-grow={s.n}></span>{/each}
    </span>
    <input
      type="range"
      min="0"
      max={scrub.stops.length - 1}
      value={scrub.index}
      disabled={r?.auto || scrub.stops.length < 2}
      aria-label="Position"
      aria-valuetext="#{at} of {head}"
      oninput={(ev) => seekEvent(scrub.stops[Number(ev.target.value)], false)}
      onchange={(ev) => seekEvent(scrub.stops[Number(ev.target.value)], true)}
    />
  </div>
{/snippet}

{#snippet speed()}
  <label class="speed" title="Playback speed">
    <span class="sr">Speed</span>
    <select value={app.replaySpeed} onchange={(ev) => setSpeed(Number(ev.target.value))}>
      {#each speeds as s (s)}
        <option value={s}>{s}×</option>
      {/each}
    </select>
  </label>
{/snippet}

{#snippet goLiveButton()}
  {#if r?.auto}
    <button class="text" onclick={goLive} title="Show the result without playing the rest">Skip to end</button>
  {:else}
    <button class="text" onclick={goLive} disabled={live} title={app.held ? 'Show the result without playing' : 'Back to the latest position'}>Go live</button>
  {/if}
{/snippet}

{#if where !== 'overlay' || r || held}
  <div class="bar {where}" role="toolbar" aria-label="Playback">
    <div class="row">
      <button class="icon" onclick={() => stepBy(-1)} disabled={r?.auto || (!r && at <= 0)} title="Previous step" aria-label="Previous step">
        <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6 5v14" /><path d="M18 5l-9 7 9 7z" /></svg>
      </button>
      {#if r?.playing}
        <button class="icon play" onclick={playOrPause} title="Pause" aria-label="Pause">
          <svg viewBox="0 0 24 24" aria-hidden="true" class="solid"><path d="M7 5h3.5v14H7zM13.5 5H17v14h-3.5z" /></svg>
        </button>
      {:else}
        <button class="icon play" onclick={playOrPause} disabled={!app.snap} title={app.held && !r ? 'Play the new moves' : 'Play'} aria-label="Play">
          <svg viewBox="0 0 24 24" aria-hidden="true" class="solid"><path d="M7 4.5v15l12-7.5z" /></svg>
        </button>
      {/if}
      <button
        class="icon"
        onclick={() => stepBy(1)}
        disabled={r ? r.auto || r.index >= r.steps.length - 1 : !app.held}
        title="Next step"
        aria-label="Next step"
      >
        <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M18 5v14" /><path d="M6 5l9 7-9 7z" /></svg>
      </button>
      <span class="pos" title={current ? turnOf(current.board) : ''}>#{at} <span class="of">/ {head}</span></span>
      {#if where === 'overlay' && current}<span class="turn">{turnOf(current.board)}</span>{/if}
      {#if where === 'dock' || where === 'overlay'}{@render scrubber()}{:else}<span class="push"></span>{/if}
      {@render speed()}
      <!-- A touch screen's timeline beside the board drops Go live while
           the board is live, where it has nothing to do. -->
      {#if !(where === 'side' && touch && live)}{@render goLiveButton()}{/if}
    </div>
    {#if where === 'side'}
      <!-- Beside the board the scrubber runs under the controls. -->
      {@render scrubber()}
    {/if}
    {#if r?.auto}
      <div class="row msg">{app.assistant} is playing</div>
    {:else if held && !r}
      <div class="row msg">
        <span>{held} new {held === 1 ? 'move' : 'moves'} from {app.assistant}</span>
        <label class="auto" title="Play the assistant's moves as they arrive">
          <input type="checkbox" checked={app.autoplay} onchange={(ev) => setAutoplay(ev.target.checked)} /> Autoplay
        </label>
      </div>
    {/if}
  </div>
{/if}

<style>
  /* App.svelte places the bar: atop the timeline, or over the board with
     the caption below it. */
  .bar {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    color: var(--ink);
    font-size: var(--fs-ui);
  }
  .bar.side { gap: 10px; padding: var(--space-3) 14px; border-bottom: 1px solid var(--line); }
  .bar.dock { flex: 1; min-width: 0; }
  .bar.overlay {
    padding: var(--space-2) var(--space-3);
    background: var(--panel);
    border: 1px solid var(--line);
    border-radius: var(--radius-l);
    box-shadow: var(--shadow-menu);
    max-width: calc(100vw - 32px);
  }
  .row { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; min-width: 0; }
  .bar.side .row, .bar.dock .row { flex-wrap: nowrap; }
  .push { flex: 1; }
  button {
    display: inline-grid;
    place-items: center;
    height: 30px;
    padding: 0 10px;
    border-radius: var(--radius-s);
    border: 1px solid var(--control-line);
    background: var(--panel);
    color: var(--ink);
    font: 600 var(--fs-12) var(--font-sans);
  }
  button:hover:not(:disabled) { border-color: var(--control-line); background: var(--hover); }
  button:focus-visible, select:focus-visible { outline: 2px solid var(--primary); outline-offset: 1px; }
  button.icon { width: 36px; height: 36px; padding: 0; }
  .bar.dock button.icon { width: 34px; height: 34px; }
  button.play { width: 44px; border-color: var(--primary); background: var(--primary); color: var(--on-primary); }
  button.play:hover:not(:disabled) { border-color: var(--primary); background: var(--primary); }
  button.text { color: var(--muted); white-space: nowrap; }
  svg { width: 16px; height: 16px; fill: none; stroke: currentColor; stroke-width: 2; stroke-linecap: round; stroke-linejoin: round; }
  svg.solid { fill: currentColor; stroke: none; }
  .pos { margin-left: var(--space-1); font: var(--fw-num) var(--fs-num) var(--font-mono); white-space: nowrap; font-variant-numeric: tabular-nums; }
  .of { color: var(--muted); font-weight: 500; white-space: nowrap; }
  .turn { color: var(--muted); white-space: nowrap; }
  /* The scrubber: the stretches, under a transparent native slider that
     takes the pointer and the keys. Beside the board it is a bar of its
     own under the controls. */
  .scrub { position: relative; flex: 1; min-width: 80px; height: 30px; display: flex; align-items: center; }
  .bar.side .scrub { flex: none; height: 8px; }
  .bar.overlay .scrub { flex: 1 1 160px; }
  .stretches { display: flex; gap: 2px; width: 100%; height: 8px; pointer-events: none; }
  .bar.dock .stretches { height: 10px; margin: 0 var(--space-2); }
  .stretches span { flex-basis: 0; min-width: 2px; border-radius: 2px; }
  .s-p1 { background: var(--scrub-p1); }
  .s-p2 { background: var(--scrub-p2); }
  .s-setup { background: var(--scrub-setup); }
  .s-now { background: var(--primary); }
  .s-ahead { background: var(--line); }
  .scrub input { position: absolute; inset: 0; width: 100%; height: 100%; margin: 0; opacity: 0; cursor: pointer; }
  .bar.side .scrub input { inset: -6px 0; height: auto; }
  .scrub input:disabled { cursor: default; }
  .scrub:has(input:focus-visible) { outline: 2px solid var(--primary); outline-offset: 2px; border-radius: var(--radius-s); }
  /* The speed: a select drawn as the design's "1×" button. */
  .speed select {
    appearance: none;
    height: 30px;
    padding: 0 9px;
    border-radius: var(--radius-s);
    border: 1px solid var(--control-line);
    background: var(--panel);
    color: var(--ink);
    font: 600 var(--fs-12) var(--font-mono);
    text-align: center;
    cursor: pointer;
  }
  .speed select:hover { background: var(--hover); }
  .msg { color: var(--muted); font-weight: 600; }
  .auto { display: flex; align-items: center; gap: var(--space-1); font-weight: 400; cursor: pointer; }
  .auto input { margin: 0; accent-color: var(--primary); }
  .sr { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
  /* On a touch screen every control is a fingertip's 44px. */
  @media (pointer: coarse) {
    button, button.icon, .bar.dock button.icon, .speed select { min-width: var(--touch-target); height: var(--touch-target); }
    button.play { width: 52px; }
    .scrub { height: var(--touch-target); }
    .bar.side .scrub { height: 10px; }
    /* The touch column (362px): the position wraps under its total only if it must. */
    .bar.side .pos { flex: 1; min-width: 0; white-space: normal; }
    .bar.side .stretches { height: 10px; }
    .bar.side .scrub input { inset: -17px 0; }
    .auto { min-height: var(--touch-target); min-width: var(--touch-target); }
  }
</style>
