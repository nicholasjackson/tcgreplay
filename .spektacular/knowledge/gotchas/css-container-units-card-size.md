---
tags: [webui, css, layout, container]
---

# Container units in --cw resolve where the variable is used

The landscape mat (`app.wide`, `web/src/App.svelte`) sizes its cards from the board area, not the viewport. `<main>` gets `container-type: size`, and `--cw` is written in `cqw`/`cqh`. There are two traps:

- **Put the container-unit `--cw` on an element inside the container, never on the container itself.** `--cw` is an unregistered custom property, so its `cqh`/`cqw` are not resolved where it is declared. They resolve wherever `var(--cw)` is used, against the nearest size container of the element using it. Cards inside `main` measure `main`, but `main`'s own `font-size: var(--fs)` (with `--fs` derived from `--cw`) measures `main`'s ancestor container. There is none, so it falls back to the viewport. That is why App wraps the landscape board in a `.board` box inside `main` that declares `--cw`, `--fs` and `font-size`.
- **A size container has no intrinsic height.** In an `auto` grid row (the timeline-below layout, `.body.panel.bottom`) it collapses to zero. Give that layout explicit rows (`minmax(0, 1fr)` for the board) whenever `main` is a size container.

Also, the landscape mat's card size is limited by **height**, not width, at 2560×1440: four rows plus the hand. Anything that adds a row, or grows one, shrinks every card. Check the benched-card size test in `web/e2e/wide.spec.js` after any layout change.
