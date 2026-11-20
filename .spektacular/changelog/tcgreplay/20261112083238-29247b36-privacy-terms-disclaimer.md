---
created_date: "2026-11-13"
document_status: draft
project: tcgreplay
spec: 20261112083238-29247b36-privacy-terms-disclaimer
plan: 20261112083238-29247b36-privacy-terms-disclaimer
---

# Terms of service, an updated privacy page and an About panel on the board

The website now has terms of service, and an up-to-date privacy page that explains visitor counting and diagnostics. Every page's footer links to both and to the project's GitHub issues for help. The board's ☰ menu has a new "About tcgreplay" panel. Like the website footer, it shows that tcgreplay is an unofficial fan project with no connection to Nintendo or The Pokémon Company, and that card data and images come from TCGdex and malie.io.

> Derived from project tcgreplay, spec/plan 20261112083238-29247b36-privacy-terms-disclaimer. See the project-level record for the full feature.

## What changed in this repo

- **Shared notice:** `internal/notice/notice.json` holds the fan-project notice, the card-data credit and the help address once. The website (`site/src/lib/notice.ts`, which fails the build on a broken file) and the board both read it, and the `Dockerfile`'s `web` and `site` stages copy it in.
- **Website footer** (`site/src/layouts/Base.astro`, `site/src/styles/site.css`): renders the shared notice and credit, and links Privacy, Terms, Get help and Source on GitHub.
- **Terms of service:** `site/src/pages/terms.astro` at `/terms`. The hosted limits are read from `.do/app.yaml`, and the trademarks section comes from the shared notice. `cmd/tcgreplay/hosted_routes_test.go` checks it is served signed out with no cookie.
- **Privacy page** (`site/src/pages/privacy.astro`):
  - new "Visitor counting" (Cloudflare Web Analytics) and "Diagnostics" (DigitalOcean metrics, player-ID usage and error records) sections, with `data-retention` figures;
  - corrected sign-in, disconnect, card-image and logs statements;
  - a cookie sentence naming `__cf_bm`;
  - a new last-updated date.
- **Board About panel** (`web/src/App.svelte`, `web/src/components/Modal.svelte`, `web/vite.config.js`): a ☰ item at every layout that opens a panel with the shared wording and links.
- **Tests:**
  - `site/e2e/notice.spec.js` (new);
  - `site/e2e/terms.spec.js` (new);
  - `site/e2e/privacy.spec.js`, including a check of the logs and metrics retention against `docs/observability.md`;
  - `site/e2e/site.spec.js` (the footer on every page);
  - `web/e2e/about.spec.js` (new: phone, 820px tablet, tablet and desktop).

## Why

These are the policy pages a public service and the ChatGPT review expect, and the privacy page had to describe visitor counting before it is switched on. Keeping the notice in one file means the website, the terms, the board and the ChatGPT listing always say the same thing. The diagnostics retention on the privacy page (7 days) is provisional until confirmed with DigitalOcean. When it is, change it together with `docs/observability.md`.
