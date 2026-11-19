---
tags: [playwright, testing, board, layout, webui]
---

# Board card positions move without anything changing: settle them before comparing

A board test that records card or control boxes (`boundingBox`, `getBoundingClientRect`), does something, and expects the boxes unchanged can fail with nothing wrong. Two things move cards on their own:

- **The hand fans out with a CSS transition** after the page loads. Boxes read straight after load are mid-transition.
- **Cards lift on hover** (`.card.lift:hover` and the hand's `.slot:hover` scale and rotate them, `web/src/components/Card.svelte` and `Hand.svelte`). After a click or a tap, the pointer often rests over a card, even in Chromium's touch emulation. Closing a modal leaves the pointer over whatever was under its button.

Before each measurement, move the pointer to a corner (`page.mouse.move(1, 1)`). Then wait for every finite animation to finish:

```js
await page.evaluate(() => Promise.all(document.getAnimations()
  .filter((a) => a.effect?.getComputedTiming().endTime !== Infinity)
  .map((a) => a.finished.catch(() => {}))));
```

Poll the after-measurement with `expect.poll`. `web/e2e/about.spec.js` (`boxes`) does this.
