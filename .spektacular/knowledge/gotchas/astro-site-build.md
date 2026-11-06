---
tags: [site, astro, build, container, embed, node]
---

# The website's Astro build: paths, Node version, outDir and the embed

The public website (`site/`, Astro 7) builds into a Go package that embeds it (`internal/site/dist/app`). Four sharp edges:

- **Build-time file paths come from `process.cwd()`, never `import.meta.url`.** During `astro build`, the working directory is the Astro project (`site/`). Pages are bundled into `site/.astro/.prerender/…` before they are rendered, so `new URL('../x', import.meta.url)` resolves inside that bundle and fails with ENOENT. Read playmat files as `join(process.cwd(), '..', …)`, as `site/src/lib/facts.ts` does, or import them with `?raw`.
- **Astro 7 needs Node 22.23.4 or newer on the 22 line.** `bin/astro.mjs` refuses older versions outright ("Node.js vX is not supported by Astro!"). A floating or cached `node:22` image below that breaks the container build, so the Dockerfile's `site` stage pins an exact version. Check Astro's `engines` before you lower it.
- **Astro empties `outDir` on every build, dotfiles included.** Nothing can be committed in `internal/site/dist/app`: no `.gitkeep`, no placeholder. The Go package's fallback page lives beside it, in `internal/site/dist/placeholder`, and `dist/app` is gitignored.
- **Anything that embeds the site embeds whatever was built when it compiled.** `go run ./internal/site/sitetest/cmd/serve`, the hosted binary and `go test` all embed the current contents of `internal/site/dist/app`. Build the site first: `make site`, or `make site-test`, which builds and then runs the browser suite. Without a build, the binary serves the placeholder, and the test server refuses to start.
