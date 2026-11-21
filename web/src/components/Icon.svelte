<script>
  // The menus' line icons, 1em square in the text colour: close (an X),
  // chevron, target (a ring with a dot: the action ends on the board), eye
  // (reveal) and text (a card's text).
  let { name } = $props();
</script>

<svg class="icon" viewBox="0 0 24 24" aria-hidden="true">
  {#if name === 'close'}
    <path d="M6 6l12 12M18 6L6 18" />
  {:else if name === 'chevron'}
    <path d="M9 6l6 6-6 6" />
  {:else if name === 'target'}
    <circle cx="12" cy="12" r="7" /><circle class="solid" cx="12" cy="12" r="2.5" />
  {:else if name === 'eye'}
    <path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12z" /><circle cx="12" cy="12" r="3" />
  {:else if name === 'text'}
    <rect x="5" y="3" width="14" height="18" rx="2" /><path d="M9 8h6M9 12h6M9 16h3" />
  {/if}
</svg>

<style>
  .icon { width: 1em; height: 1em; flex: none; fill: none; stroke: currentColor; stroke-width: 2; stroke-linecap: round; stroke-linejoin: round; }
  .solid { fill: currentColor; }
</style>
