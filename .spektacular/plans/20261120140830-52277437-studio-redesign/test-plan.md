---
created_date: "2026-11-21"
document_status: final
closed_date: "2026-11-21"
---

# Test plan: Studio redesign

The automated suites cover everything else: the Go token checks, the board and website browser suites, and both sets of screenshot baselines. This plan lists the one success metric and the five reviews the plan leaves to a person. Run them on the spec branch `spek/20261120140830-52277437-studio-redesign` before it is merged to main, because every push to main deploys.

Setup used below, from the repository root:

- Build both apps: `make web site`.
- Board: `make dev` serves the playmat at http://127.0.0.1:7778/. Start a game from your assistant, or import a battle log, so the board has cards.
- Website: `go run ./internal/site/sitetest/cmd/serve` serves the built website at http://127.0.0.1:7791/.

## Metric: changing one token restyles the board and the website together

- **What to measure.** Change one value in `internal/design/tokens.json`. Both apps must pick up the change, and no component file may need an edit.
- **How.**
  1. In `internal/design/tokens.json`, change the light value of `primary` from `#13285C` to an obvious colour such as `#B00020`.
  2. Run `go generate ./internal/design`, then `make web site`, then serve both as in the setup above.
  3. Look at the board's Play button in the timeline and the website's Get started button.
  4. Revert the change with `git checkout internal/design/tokens.json internal/design/tokens.css`, then rebuild.
- **Expected result.**
  - The board's Play button and the website's Get started button both turn red.
  - `git status` shows only `tokens.json` and `tokens.css` changed.
  - `go test ./internal/design/` fails only on a contrast pair, if at all, and on nothing else.
- **Who / when.** The maintainer, once, before the merge.

## Review: the board's Studio screenshots against the design canvas

- **What to look at.**
  - The recorded baselines in `web/e2e/__screenshots__/`:
    - `desktop/board.png`
    - `wide/board.png` (the landscape mat, 2560×1440)
    - `desktop/board-docked.png` (timeline below)
    - `tablet/board.png` (touch)
    - `phone/board.png`
    - `menu.png` for desktop, tablet and phone
  - Also the live board at those sizes. Use the browser's device toolbar for touch.
- **Compare with.**
  - The design canvas: https://claude.ai/artifact/5esddCvKjxHT9WiFw9u7MJ. Use its StudioBoard, C-Narrow, C-iPad, C-Phone, Studio-Dark, Studio-Felt, Studio-Navy and Menu-* boards.
  - The design system: https://claude.ai/artifact/8uqeTrkeXAc2YYTUvwtEjK.
- **What to look for.**
  - The mat's player blocks, Bench readouts, Active panel, turn strip and hand labels.
  - The navy card back.
  - The timeline's controls, scrubber and turn list.
  - The menus in each container.
  - Light and dark interfaces, and the slate, felt and navy mats (☰ → Interface and Mat).
  - The deliberate differences recorded in the plan's changelog entries, such as the fanned hands, the 1.1-width Active card and the overlay phone header below 740px. Accept or reject each one.
- **Passing.** The maintainer accepts the look at all five screen sizes and in each menu container. Anything rejected becomes a follow-up change.
- **Who / when.** The maintainer, before the merge.

## Review: every website page's Studio screenshots against the design canvas

- **What to look at.**
  - The baselines in `site/e2e/__screenshots__/laptop/` and `site/e2e/__screenshots__/phone/`, covering 22 pages each, the 404 page (`no-such-page.png`) included.
  - Also the served website at http://127.0.0.1:7791/.
- **Compare with.** The canvas's Main, Site-Home-Phone, SiteHeader, SiteFooter, DocsNav, Site-Docs, Site-Guides, Site-WhatsNew, Site-Example, Site-Limits, Site-Privacy and Site-404 boards.
- **What to look for.**
  - The header, with Get started as a button.
  - The dark footer.
  - The docs breadcrumb and On this page list.
  - The home scene and its sections.
  - The guides grouped into After a game, Practice and Play.
  - The What's new timeline, the Limits tiles, the Example panels, and the Privacy, Terms and 404 pages.
  - The deliberate differences in the changelog entries, such as Example's stacked panels and the 2×2 Limits tiles.
- **Passing.** The maintainer accepts every page at both widths.
- **Who / when.** The maintainer, before the merge.

## Review: menus, the timeline sheet and themes on a real iPhone and iPad

The automated touch tests emulate touch in Chromium, because WebKit can't run on the development machine. This review checks the same behaviour on real devices.

- **Where.**
  - Prefer a test deployment of the branch that the devices can reach.
  - Otherwise, run `make dev` with `DEV_UI=0.0.0.0:7778` on the local network. That exposes the board to the local network; stop it afterwards.
- **What to check.**
  - **iPhone:**
    - Tapping a card opens the full-screen action sheet, and its Card text toggle works.
    - An attack opens the attack panel, which shows the Knock Out hint at or above the target's HP left.
    - ☰ opens the timeline sheet. Play from here and Rewind to here work there, and the floating playback bar appears while a replay runs.
    - Switching Interface and Mat in ☰ changes the look at once, and the choice survives a reload.
  - **iPad:**
    - Tapping a card opens the centred panel, with the Card/Text switch.
    - Tapping the prize pips opens the prize view.
    - Docking the timeline below gives a drag handle that resizes it.
    - The timeline controls are easy to hit with a finger.
- **Passing.** Everything behaves as in the emulated tests, and no control is too small to tap.
- **Who / when.** The maintainer, before the merge or right after it, on the live site.

## Review: IBM Plex loads from Google Fonts, and falls back cleanly when blocked

- **Where.** The board at http://127.0.0.1:7778/ and the website at http://127.0.0.1:7791/. On the live site, use https://tcgreplay.app/ after the merge.
- **How.**
  1. Open the browser's network panel and reload. Expect one stylesheet from `fonts.googleapis.com`, and font files from `fonts.gstatic.com` only. There should be no other outside host except Cloudflare's beacon on the live site.
  2. Block both hosts, using the network panel's request blocking for `fonts.googleapis.com` and `fonts.gstatic.com`, then reload.
- **Expected result.**
  - With the hosts allowed, text renders in IBM Plex Sans, and numbers and ids in IBM Plex Mono.
  - With them blocked, the pages render at once in the system sans and mono fonts. Nothing is invisible, nothing is broken, and the layout doesn't collapse. The board stays fully usable.
- **Who / when.** The maintainer, before the merge, and once on the live site after it.

## Review: no shipped logo or icon shows Poké Ball imagery

- **What to look at.**
  - `internal/design/mark.svg` (the source).
  - `site/public/logo.png` and `site/public/icon.png`.
  - `docs/logo.png` and `docs/icon.png`.
  - `plugin/assets/icon.svg` and `plugin/assets/icon.png`.
  - The favicon embedded in `web/index.html`.
  - The board's card back. Open a game and look at any face-down card.
  - The home page's animated scene.
- **What to look for.** Each shows the Studio mark: a navy rounded square, two cards and a replay arrow. None shows a Poké Ball or any other Pokémon imagery. The automated check covers only trademark words in the listing icon's source.
- **Passing.** No Pokémon imagery appears in any of them.
- **Who / when.** The maintainer, before the merge.
