# Working context: spec 20261106110254-f08e142d-project-website

## Run mode (user, 2026-11-06)
The user is away ("I am going to go for coffee so feel free to run through this
without me"). Drive the whole spec workflow without asking; settle open
questions with sensible defaults, record each one here, and report them at the
end. Do not plan or implement: the request was to "spec out" the site.

## Why (user's words)
- "the tool is amazing, I would love to share it."
- "let's spec out a website and docs"
- "I would like a simple site that shows how to use the tool. Maybe has an
  animation on the home page"

## Context gathered this session
- The hosted playmat is live at https://tcgreplay.demo.gs (MCP at /mcp, Google
  sign-in, per-account games, idle drop after 1h, games lost on deploy). It runs
  on DigitalOcean App Platform from .do/app.yaml (single instance), with a
  Spaces bucket + CDN for card data/images.
- Works with claude.ai custom connectors, Claude Code (HTTP MCP), Claude
  desktop (local binary, board on 127.0.0.1:7777), and now ChatGPT (custom MCP
  server; OAuth callback allowlist extended today). The user found ChatGPT
  "works even better".
- A README now exists (what it does, connecting, example prompts, dev), with a
  logo at docs/logo.png (transparent, Poké Ball on cards) and docs/icon.png
  (256px, <10 KB). Trademark note given: the Poké Ball is a Nintendo/TPC mark.
- A real example game (GatoruSoju, Mew ex/Bronzong/Lopunny, a close loss) plus
  the Mega Lucario ex v12 list in testdata worked well end to end in ChatGPT.
  It contains a real opponent's screen name: anonymise before public use.
- Repo is public on GitHub (nicholasjackson/tcgreplay).
- Coaching guides: import-game, opponent-deck, review-game, replay-drill,
  before-you-move, numbers-drill, play-game.

## Decisions made without the user (report at the end)
- Spec name `project-website`, standalone (not in an epic).
- Hosting: static site, GitHub Pages by default, separate from the playmat
  (playmat address must not change). Planner to check Google consent-screen
  domain verification for github.io; propose an owned domain if needed.
- Animation: lightweight, in the board's style, offline, looping, reduced-motion
  still; no official card art or logos.
- Docs in Markdown; README shrinks to intro + links (one source of truth).
- No analytics/cookies.
- Example game: GatoruSoju game + Mega Lucario ex v12, opponent anonymised.
- Success metrics invented: <10 min to connect + import; <2 s / <1.5 MB home;
  audit >= 90.
- Non-goals drafted while the user was away; the user confirmed the whole
  spec, non-goals and away-run decisions included, on 2026-11-06 ("I think
  this is good"). Split into two specs not taken up; stays one spec.
- Constraints: all four trace to the user (simple site), existing systems
  (connector address), platform (Google consent screen), and accuracy.
- Verification: fresh-eyes reviewer raised 23 findings; applied all but the
  H1 title (template-mandated), "site source must live in repo" (not a user
  rule; requirement reworded instead), and resolved the logo conflict by
  banning official card art + Pokémon/TCG logos while keeping the project logo
  until the user decides. Spec written to the store.
- Split check (moderate): passes — 21 requirements (weak) and a group that
  ships alone (the docs site with a still home image) vs the home animation
  (depends on the site). Offer NOT acted on: the user is away and a split needs
  their agreement. Offer it in the final report; a written spec can be split
  any time.
