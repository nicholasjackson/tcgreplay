<script>
  // The menus' line icons, 1em square in the text colour: close (an X),
  // chevron, target (a ring with a dot: the action ends on the board), eye
  // (reveal) and text (a card's text). The pile views' actions (hand,
  // bench, attach, decktop, deckbottom, discard, lost, take) and their
  // header's search, shuffle and key carry more detail, so they are drawn
  // at a lighter 1.8 stroke, as in the PileActions design.
  let { name } = $props();
  const FINE = ['hand', 'bench', 'attach', 'decktop', 'deckbottom', 'discard', 'lost', 'take', 'search', 'shuffle', 'key'];
</script>

<svg class="icon" class:fine={FINE.includes(name)} viewBox="0 0 24 24" aria-hidden="true">
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
  {:else if name === 'hand'}
    <path d="M7 15.5V7.5a1.5 1.5 0 0 1 3 0V12" /><path d="M10 11.5V5.5a1.5 1.5 0 0 1 3 0v6" /><path d="M13 11.5V6.5a1.5 1.5 0 0 1 3 0V12" /><path d="M16 12V9a1.5 1.5 0 0 1 3 0v5.5c0 3.9-2.6 6.5-6.3 6.5h-1.2c-2.1 0-3.5-.7-4.8-2.1l-3.3-3.6a1.5 1.5 0 0 1 2.2-2L7 15.5" />
  {:else if name === 'bench'}
    <rect x="4" y="4" width="16" height="4.5" rx="1" /><rect x="2.5" y="12" width="19" height="3" rx="1" /><path d="M7.5 8.5V12M16.5 8.5V12M5.5 15v5M18.5 15v5" />
  {:else if name === 'attach'}
    <path transform="rotate(45 12 12)" d="M10.5 9v6.5a1.5 1.5 0 0 0 3 0V6a3 3 0 0 0-6 0v10a4.5 4.5 0 0 0 9 0V9" />
  {:else if name === 'decktop'}
    <rect x="3" y="10.5" width="12" height="9.5" rx="1.5" /><path d="M3 15.25h12" /><path d="M19.5 18V7.5A2.5 2.5 0 0 0 17 5H8" /><path d="M11 2 8 5l3 3" />
  {:else if name === 'deckbottom'}
    <rect x="3" y="4" width="12" height="9.5" rx="1.5" /><path d="M3 8.75h12" /><path d="M19.5 6v10.5a2.5 2.5 0 0 1-2.5 2.5H8" /><path d="M11 16l-3 3 3 3" />
  {:else if name === 'discard'}
    <path d="M4 7h16M9.5 7V4.5h5V7M6 7l1 13h10l1-13M10 11v5.5M14 11v5.5" />
  {:else if name === 'lost'}
    <path d="M9.4 12a2.6 2.6 0 0 1 5.2 0a5.3 5.3 0 0 1-10.6 0a8 8 0 1 1 8 8" />
  {:else if name === 'take'}
    <rect x="9.5" y="1.5" width="8.5" height="10.5" rx="1.3" /><path d="M2.5 16.5 6 13.6c.5-.4 1-.6 1.6-.6h4.9a1.75 1.75 0 0 1 0 3.5H9.5" /><path d="M6.5 20.5l1.3-1.1c.4-.3.9-.5 1.4-.5h4.4c.9 0 1.7-.3 2.3-.9l4-3.8a1.6 1.6 0 0 0-2.2-2.3l-3.4 3" />
  {:else if name === 'search'}
    <circle cx="10.5" cy="10.5" r="6" /><path d="M15 15l5 5" />
  {:else if name === 'shuffle'}
    <path d="M3 7h3.5c4.5 0 6.5 10 11 10H21" /><path d="M3 17h3.5c1.8 0 3.1-1.6 4.2-3.6M13.3 9.6C14.4 8.1 15.6 7 17.5 7H21" /><path d="M18.5 4.5 21 7l-2.5 2.5M18.5 14.5 21 17l-2.5 2.5" />
  {:else if name === 'key'}
    <rect x="3" y="5" width="18" height="14" rx="2" /><path d="M7 10h2M7 14h2M12 10h5M12 14h5" />
  {/if}
</svg>

<style>
  .icon { width: 1em; height: 1em; flex: none; fill: none; stroke: currentColor; stroke-width: 2; stroke-linecap: round; stroke-linejoin: round; }
  .fine { stroke-width: 1.8; }
  .solid { fill: currentColor; }
</style>
