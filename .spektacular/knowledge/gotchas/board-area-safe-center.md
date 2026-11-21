---
tags: [board, webui, css, layout]
---

# A centred board area must use `safe center`, or an overflowing board loses its top

`<main>` in `web/src/App.svelte` is a flex column that centres the board vertically and scrolls (`overflow: auto`). With plain `justify-content: center`, a board taller than its area overflows equally above and below. The part above the top edge sits at negative scroll offsets, so it can never be scrolled to: the opponent's hand and bench are cut off out of reach. That happened with the timeline docked below on a tablet, where 48px touch cards can't shrink enough. Use `justify-content: safe center`, which centres while there is room and falls back to the start once the content overflows. Any other centred, scrolling container on the board needs the same.
