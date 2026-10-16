<script>
  // Right-click menu. Items are {label, run} or {sep: true}.
  import { app } from '../lib/store.svelte.js';

  let el = $state();
  let pos = $state({ x: 0, y: 0 });

  $effect(() => {
    if (!app.menu || !el) return;
    const { innerWidth: w, innerHeight: h } = window;
    const r = el.getBoundingClientRect();
    pos = { x: Math.min(app.menu.x, w - r.width - 8), y: Math.min(app.menu.y, h - r.height - 8) };
  });

  function choose(item) {
    app.menu = null;
    item.run();
  }
</script>

<svelte:window onkeydown={(ev) => ev.key === 'Escape' && (app.menu = null)} />

{#if app.menu}
  <div class="backdrop" onclick={() => (app.menu = null)} oncontextmenu={(ev) => { ev.preventDefault(); app.menu = null; }} role="presentation"></div>
  <div class="menu" bind:this={el} style:left="{pos.x}px" style:top="{pos.y}px" role="menu">
    <div class="title">{app.menu.title}</div>
    {#each app.menu.items as item, i (i)}
      {#if item.sep}
        <hr />
      {:else}
        <button role="menuitem" onclick={() => choose(item)} disabled={item.disabled}>{item.label}</button>
      {/if}
    {/each}
  </div>
{/if}

<style>
  .backdrop { position: fixed; inset: 0; z-index: 40; }
  .menu {
    position: fixed;
    z-index: 41;
    min-width: 200px;
    max-height: 80vh;
    overflow-y: auto;
    background: var(--panel);
    border: 1px solid var(--line);
    border-radius: 8px;
    box-shadow: 0 8px 30px #000a;
    padding: 4px;
  }
  .title { font-weight: 700; padding: 6px 10px; color: var(--muted); font-size: 12px; }
  button {
    display: block;
    width: 100%;
    text-align: left;
    background: none;
    border: none;
    color: var(--text);
    padding: 6px 10px;
    border-radius: 4px;
    cursor: pointer;
    font-size: 13px;
  }
  button:hover:not(:disabled) { background: var(--hover); }
  hr { border: none; border-top: 1px solid var(--line); margin: 4px 0; }
</style>
