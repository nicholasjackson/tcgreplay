# Working context: studio-redesign

## Problem and motivation
- The user wanted to update tcgreplay's design. Their reasons, all four: the general look feels dated, mobile/touch, brand consistency, and board readability.
- The brand doesn't match: logo.png is navy and cyan with a Poké Ball, but the app is green felt with gold.
- Code today: colour tokens are in web/src/app.css, hand-copied to site/src/styles/tokens.css ("Keep in step with that file"). Colours only: no type, spacing or radius tokens. The Svelte components contain about 94 hard-coded hex colours and about 53 fixed px font sizes.
- Considered and rejected:
  - `/design-sync` to Claude Design: React-only, and web/ is Svelte 5.
  - Porting to React: the wrong motive. Svelte's fine-grained updates suit the live board, and the 97 Playwright e2e tests are framework-agnostic anyway.
  - Offering all three design directions as user choices: tripled maintenance, and it dilutes the brand.

## Decisions (user-confirmed)
- Direction: **Studio**, a light interface around a dark mat, IBM Plex Sans and Mono.
- Themes: interface light/dark × mat slate/felt/navy. The default is light + slate. Set in the board's ☰ menu, not the header. Only colour changes between themes.
- Logo: the new card-stack mark replaces logo.png, with no Pokémon imagery.
- Menu proposals kept: the Knock Out hint in the attack panel (arithmetic only), and the "Card text" item in the hand-card mouse popover.
- Website: accept the nav changes (Get started as a button, What's new in the top links, Home and Privacy out of the top bar) and the guides grouped into After a game / Practice / Play. The site's browser tests need updating.
- The timeline must keep play-from-here and rewind on every line, plus per-turn and per-half rewind/play. The user flagged this as already possible today ("this is currently possible").
- Narrow PC: the user has a vertical second monitor with the timeline docked below (panelAt=bottom in their browser). The new turn-list rail in the timeline was accepted with the narrow board.
- Order the user agreed earlier: tokens first, with components moved onto tokens without visual change, then apply Studio.

## Interview answers (spec workflow)
- Ship as ONE release, with tokens-first and no-visual-change as an internal checkpoint proven by screenshots.
- New Studio card back replaces the Poké Ball-like back.
- One mark everywhere: redraw plugin/assets/icon.svg/png and the site/docs icons from the Studio mark.
- Proof: screenshot comparisons + a stray colour/font-size test + a contrast check + existing suites passing (updated only for nav/guide changes).

## Artifacts
- Canvas: https://claude.ai/artifact/5esddCvKjxHT9WiFw9u7MJ (board layouts, themes, menus, website pages, components).
- Design doc: `studio/design.md` in the `design` source (authored, draft). Earlier designs mobile-board/design.md and wide-board/design.md still govern behaviour and layout.
- Design system artifact: https://claude.ai/artifact/8uqeTrkeXAc2YYTUvwtEjK (tokens.json, README brand book, Logos asset, 12 components).
- Dev playmat game `design-narrow-check` imported into .dev for inspection.

## Workflow preference
- The user asked to see the full spec at once rather than section by section ("just show me the full spec"). The full draft was approved ("Yes, finish it"); all section working files hold the approved content. Advance through the section steps without re-asking.
- Fonts: the user said "I think it is ok to load fonts", so IBM Plex loads from Google Fonts (not self-hosted). The privacy page must name Google Fonts as an outside service (today it says only the Cloudflare script loads from another site). The design system README's "from Google Fonts" is therefore correct.
- Split offered at the split step (3 specs: tokens, board, website); the user declined and kept one spec.

## Plan workflow (studio-redesign), discovery decisions from the user
- Fonts: "Change the tests, web fonts are fine". IBM Plex loads from Google Fonts on the board and the site; update site tests (site.spec.js 150-207, analytics.spec.js 72-98, animation.spec.js 118-120) to allow Google Fonts hosts only, and reword the privacy page.
- Home hero: restyle the existing animated Playmat scene in Studio (keep its tests); do NOT embed the real board. Design doc studio/design.md updated to match.
- Wide landscape mat: "Land it first". The user commits and merges the uncommitted wide-board work (worktree ../tcgreplay-wide-board, branch spek/20261104090653-...) before Studio starts; the plan restyles it. A precondition, not a plan task.
- Test edits: allowed where a spec requirement forces them (privacy "four display settings" count, the Cloudflare-only sentence), each recorded in the plan. Spec constraint "Existing suites" amended to say so.
- Plan 20261120140830-52277437-studio-redesign approved by the user at the walkthrough ("plan is good").
