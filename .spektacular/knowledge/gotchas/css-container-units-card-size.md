---
tags: [webui, css, layout, container, board]
---

# Container units in --cw resolve where the variable is used

The landscape mat (`app.wide`, `web/src/App.svelte`) sizes its cards from the board area, not the viewport. `<main>` gets `container-type: size`, and `--cw` is written in `cqw`/`cqh`. There are two traps:

- **Put the container-unit `--cw` on an element inside the container, never on the container itself.** `--cw` is an unregistered custom property, so its `cqh`/`cqw` are not resolved where it is declared. They resolve wherever `var(--cw)` is used, against the nearest size container of the element using it. Cards inside `main` measure `main`, but `main`'s own `font-size: var(--fs)` (with `--fs` derived from `--cw`) measures `main`'s ancestor container. There is none, so it falls back to the viewport. That is why App wraps the landscape board in a `.board` box inside `main` that declares `--cw`, `--fs` and `font-size`.
- **A size container has no intrinsic height.** In an `auto` grid row (the timeline-below layout, `.body.panel.bottom`) it collapses to zero. Give that layout explicit rows (`minmax(0, 1fr)` for the board) whenever `main` is a size container.

Also, the landscape mat's card size is limited by **height**, not width, at 2560×1440: four rows plus the hand. Anything that adds a row, or grows one, shrinks every card. Check the benched-card size test in `web/e2e/wide.spec.js` after any layout change.

**Today's mat with the timeline docked below does not use container units, and its vh formula runs short.** Its `--cw` comes from the viewport height less the timeline row (`--dock-h`, which the drag handle changes), divided by about 11.1 card widths. Measured, the mat is closer to 9.2 × `--cw` + 143px tall. That fits at ordinary sizes but overflows once cards get small, because the fixed part dominates. The docked mat therefore also runs `fitBoard` on `<main>` and subtracts its `--fit` correction, so it shrinks rather than scrolls. `fitBoard` measures layout boxes and not transforms, so the fanned hand's rotated cards can still poke a few pixels out. If you resize the board area some new way, feed the new height into both `--cw` and `fitBoard`.

