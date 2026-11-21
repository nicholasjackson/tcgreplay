---
tags: [playwright, testing, screenshots, board, site]
---

# Playwright's default screenshot threshold misses colour changes between dark shades

`toHaveScreenshot` compares pixels with a per-pixel colour `threshold` that defaults to 0.2, on a perceptual scale. Two dark colours fall within it, so a change between them does not count as a different pixel. Switching the whole board mat from dark green (`#2c5a3a`) to slate (`#252A33`) flagged only the bright buttons and dashed outlines: under 1% of the image, inside a `maxDiffPixelRatio` of 0.002 had the buttons not changed too. A "screenshots still match" check run with the default can therefore pass across a real restyle of dark surfaces.

Set `threshold: 0` and let `maxDiffPixelRatio` (0.002 in `web/` and `site/`) absorb rendering noise. At threshold 0, Chromium's run-to-run noise on this machine is small: website pages differ by at most 1/255 in a few dozen pixels, and the board's tablet and docked layouts by up to about 180 pixels in one small region, which also happens when the same build is compared with itself.

To check a refactor that should look identical, compare at `threshold: 0, maxDiffPixels: 0` with a throwaway config that spreads the real one (`export default { ...base, expect: { toHaveScreenshot: { ...base.expect.toHaveScreenshot, threshold: 0, maxDiffPixels: 0, maxDiffPixelRatio: undefined } } }`), and compare the old code against its own baselines the same way to tell noise from change.
