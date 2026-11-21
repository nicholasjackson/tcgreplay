---
tags: [site, webui, css, layout, svg, playwright]
---

# Website home page: full-width bands and inline SVG in the scene

**A full-width band inside the website's `main` uses a spread shadow, not negative margins.** `main` is held to `--content-width` with gutters. To run a band, such as the home page's slate "What it does" section, edge to edge, give it `box-shadow: 0 0 0 100vmax <colour>` plus `clip-path: inset(0 -100vmax)`. The usual `margin-inline: calc(50% - 50vw)` counts the scrollbar's width in `100vw`, so the page scrolls sideways, and the 375px no-sideways-scroll test fails.

**Inline SVG in the home scene must be `aria-hidden="true"` and coloured through CSS.** The home tests ban `img`, `image`, `picture`, `video` and `iframe` in the scene, but allow an inline `<svg>` of `rect`/`path`. This is how the Studio card back draws the mark there. Without `aria-hidden`, Playwright counts the `<svg>` as an image role, and `home.spec.js`'s `scene.getByRole('img')` stops finding exactly one. Set its colours with CSS `fill`/`stroke: var(--token)`, not hex attributes, or the tokens-only scan fails.
