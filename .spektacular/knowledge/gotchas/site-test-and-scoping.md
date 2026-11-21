---
tags: [site, astro, css, playwright, testing]
---

# Website: Astro scopes `*` too, and one Playwright run per worktree

**Astro's scoped CSS adds its scoping attribute to `*`.** A scoped rule `.terms > *` compiles to `.terms[data-astro-cid-…] > *[data-astro-cid-…]`, with specificity (0,3,0). It beats a scoped single-class rule such as `.toc[data-astro-cid-…]` (0,2,0), so a layout property set on `.toc` (here `grid-column: 2`) is silently overridden. Avoid broad `> *` rules for layout in `.astro` styles, or give the specific rule matching weight.

**Only one run of the site's Playwright suite can happen in a worktree at a time.** `site/playwright.config.js` starts its servers on fixed ports (7791 and 7792) with `reuseExistingServer: false`, so a second run fails with "already used". Runs also share `test-results/` and delete each other's trace files, which shows up as ENOENT failures. When two agents work on the website at once, either wait for the ports to free up, or give one run a temporary config on other ports and its own `--output` directory.
