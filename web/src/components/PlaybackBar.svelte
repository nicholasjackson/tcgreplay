<script>
  // Controls for held moves and replays: play, pause, step, scrub, speed.
  import { app } from '../lib/store.svelte.js';
  import { goLive, heldMoves, pause, play, seek, setSpeed, speeds, startReplay, step } from '../lib/playback.svelte.js';

  const r = $derived(app.replay);
  const held = $derived(app.held ? heldMoves() : 0);
  const current = $derived(r && r.index >= 0 ? r.steps[r.index] : null);
</script>

{#if r || held}
  <div class="bar" role="toolbar" aria-label="Playback">
    {#if r}
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
      <span class="pos">{r.index + 1} / {r.steps.length}{#if current} · T{current.board.turn}{/if}</span>
      <button onclick={goLive} title="Back to the latest position">Live ⏭</button>
    {:else}
      <span class="msg">{held} new {held === 1 ? 'move' : 'moves'} from Claude</span>
      <button class="primary" onclick={() => startReplay(app.held.from)}>▶ Play</button>
      <button onclick={goLive} title="Show the result without playing">Skip to end</button>
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
  .bar {
    position: fixed;
    top: 52px;
    left: 50%;
    transform: translateX(-50%);
    z-index: 25;
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
  .speed { display: flex; align-items: center; gap: 4px; color: var(--muted); }
  .speed select { background: var(--input); color: var(--text); border: 1px solid var(--line); border-radius: 4px; padding: 3px; }
</style>
