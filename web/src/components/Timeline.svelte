<script>
  // Every event on the current branch, grouped by turn, with annotations.
  import { app } from '../lib/store.svelte.js';

  let list = $state();
  const log = $derived(app.snap?.log || []);
  const turns = $derived.by(() => {
    const out = [];
    for (const l of log) {
      let t = out.at(-1);
      if (!t || t.turn !== l.turn) out.push((t = { turn: l.turn, lines: [] }));
      t.lines.push(l);
    }
    return out;
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
    <h3>{t.turn === 0 ? 'Setup' : `Turn ${t.turn}`}</h3>
    {#each t.lines as l (l.id)}
      <div class="line seat-{l.seat}" class:fresh={fresh && l.id > fresh}>
        <span class="id">#{l.id}</span>
        {#if l.label}<span class="label">{l.label}</span>{/if}
        <span class="text">{l.text}</span>
        {#each l.annotations || [] as a}
          <div class="note">✎ {a}</div>
        {/each}
      </div>
    {/each}
  {:else}
    <p class="empty">No events yet.</p>
  {/each}
</div>

<style>
  .timeline {
    overflow-y: auto;
    height: 100%;
    padding: 8px 10px 40px;
    font-size: 12px;
    line-height: 1.35;
  }
  h3 {
    position: sticky;
    top: -8px;
    background: var(--panel);
    margin: 10px 0 4px;
    padding: 4px 0;
    font-size: 12px;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    color: var(--muted);
  }
  .line {
    padding: 3px 6px;
    border-left: 3px solid transparent;
    border-radius: 3px;
  }
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
