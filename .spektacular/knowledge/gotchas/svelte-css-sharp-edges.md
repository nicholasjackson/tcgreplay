---
tags: [board, webui, svelte, css, touch]
---

# Svelte and scoped-CSS sharp edges on the board

Traps from the Studio restyle of the board (`web/src`):

- **Svelte drops the leading space inside an `{#if}` block.** In `{a}{#if x} · {b}{/if}`, the space before "·" is lost, so "Bob's Active · 80" renders as "Bob's Active· 80" (the card zoom's id line once read "p1-2· tst-1"). Build such strings in JS instead: a derived value, or `[a, b].filter(Boolean).join(' · ')`.
- **A text input in a flex row needs `width: 0` as well as `flex: 1; min-width: 0`.** With `width: auto`, its built-in width still counts toward the row's minimum, and the row overflows sideways (the phone's attack panel scrolled).
- **Scoped component CSS can quietly undo the global 44px touch minimum.** A component rule such as `button { min-height: 28px }` is scoped by Svelte and outranks the `@media (pointer: coarse)` minimum in `app.css`. Any component that sets a control smaller than 44px needs its own coarse-pointer rule restoring `var(--touch-target)`. touch.spec's 44×44 check covers only the action sheet and the pile view, so it won't catch this elsewhere.
- **In `ActionSheet.svelte`, an attribute selector outranks the per-mode overrides.** `.sec[aria-label='Attack'] .item` beats `.popover .item` because the attribute adds specificity. A section-specific style needs matching `.popover`, `.tablet` and `.phone` variants, or the modes stop looking different.
- **A `@container` or `@media` override placed above a later rule of the same specificity loses to it.** At-rules add no specificity, so source order decides. The top-of-discard caption kept `display: flex` because its `@container (max-width: 140px) { .caption { display: none } }` came before the base `.caption` rule. In the same way, a coarse-pointer block restoring `min-height: var(--touch-target)` is undone by a later `min-height: 0` on the same selector. Put each override after the rule it overrides.
