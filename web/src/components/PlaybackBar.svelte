<script>
  // Controls for held moves and replays: play, pause, step, scrub, speed.
  // An autoplay of the assistant's moves shows only what is playing, with a
  // skip and the speed; the full controls are for replays.
  import { app } from '../lib/store.svelte.js';
  import { goLive, heldMoves, pause, play, seek, setAutoplay, setSpeed, speeds, startReplay, step } from '../lib/playback.svelte.js';

  const r = $derived(app.replay);
  const held = $derived(app.held ? heldMoves() : 0);
  const current = $derived(r && r.index >= 0 ? r.steps[r.index] : null);
  // The turn the step is in, counting both players, and whose half it is.
  function turnOf(board) {
    if (!board.turn) return 'Setup';
    const name = board.sides?.[board.current]?.name || (board.current === app.human ? 'You' : app.assistant);
    return `T${board.turn} · ${name}`;
  }
</script>

{#if r || held}
  <div class="bar" role="toolbar" aria-label="Playback">
    {#if r?.auto}
      <span class="msg">{app.assistant} is playing</span>
      <span class="pos">{Math.max(r.index + 1, 1)} / {r.steps.length}</span>
      <button onclick={goLive} title="Show the result without playing the rest">Skip to end ⏭</button>
    {:else if r}
      <button onclick={() => seek(-1)} title="To the start" aria-label="To the start">⏮</button>
      <button onclick={() => step(-1)} title="Previous move" aria-label="Previous move">◀</button>
      {#if r.playing}
        <button class="primary" onclick={pause} title="Pause" aria-label="Pause">⏸</button>
      {:else}
        <button class="primary" onclick={play} title="Play" aria-label="Play">▶</button>
      {/if}
      <button onclick={() => step(1)} title="Next move" aria-label="Next move">▶|</button>
      <input
        type="range"
        min="-1"
        max={r.steps.length - 1}
        value={r.index}
        oninput={(ev) => seek(Number(ev.target.value))}
        aria-label="Position"
      />
      <span class="pos">{r.index + 1} / {r.steps.length}{#if current} · {turnOf(current.board)}{/if}</span>
      <button onclick={goLive} title="Back to the latest position">Live ⏭</button>
    {:else}
      <span class="msg">{held} new {held === 1 ? 'move' : 'moves'} from {app.assistant}</span>
      <button class="primary" onclick={() => startReplay(app.held.from)}>▶ Play</button>
      <button onclick={goLive} title="Show the result without playing">Skip to end</button>
      <label class="auto" title="Play the assistant's moves as they arrive">
        <input type="checkbox" checked={app.autoplay} onchange={(ev) => setAutoplay(ev.target.checked)} /> Autoplay
      </label>
    {/if}
    <label class="speed">
      Speed
      <select value={app.replaySpeed} onchange={(ev) => setSpeed(Number(ev.target.value))}>
        {#each speeds as s (s)}
          <option value={s}>{s}×</option>
        {/each}
      </select>
    </label>
  </div>
{/if}

<style>
  /* App.svelte positions the bar, with the caption below it. */
  .bar {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 14px;
    background: var(--panel);
    border: 1px solid var(--accent);
    border-radius: 12px;
    box-shadow: 0 6px 24px #000a;
    font-size: 14px;
    max-width: calc(100vw - 32px);
    flex-wrap: wrap;
  }
  .bar button { font-size: 15px; min-width: 2.4em; }
  input[type='range'] { width: clamp(120px, 18vw, 360px); accent-color: var(--accent); }
  .pos { color: var(--muted); font-variant-numeric: tabular-nums; white-space: nowrap; }
  .msg { font-weight: 600; }
  .auto { display: flex; align-items: center; gap: 4px; color: var(--muted); cursor: pointer; }
  .speed { display: flex; align-items: center; gap: 4px; color: var(--muted); }
  .speed select { background: var(--input); color: var(--text); border: 1px solid var(--line); border-radius: 4px; padding: 3px; }
</style>
