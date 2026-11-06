# Interview synthesis: project website

The user was away for this run and asked for it to go ahead without them, so
this is drawn from the session's conversation and the repo, with defaults
chosen for anything they did not say (marked **default**).

## What and why
- The user finds tcgreplay "amazing" and wants to share it. Today the only
  public description is the README on GitHub, which mixes user and developer
  material. They asked for "a simple site that shows how to use the tool.
  Maybe has an animation on the home page", plus docs.
- The site is for Pokémon TCG Live players who use an AI assistant (Claude or
  ChatGPT), not for developers. It explains what the playmat does, gets them
  connected, and shows them what to say to it.

## Who uses it
- **Players** arriving from a shared link: they want to know what it is in
  seconds, then get connected to the hosted playmat in a few minutes.
- **Developers** wanting to run it locally or contribute: a short section that
  points to the repo, not a full developer manual.

## Content the site needs
- **Home**: one-line pitch, an animation of the playmat in action, the core
  loop (import → review → rewind → drill, or play a game), and a clear
  "get started" path.
- **Get started**: connecting the hosted playmat (`https://tcgreplay.demo.gs/mcp`)
  from claude.ai, ChatGPT and Claude Code; running it on the desktop with the
  Claude desktop app; signing in with Google; opening the board link.
- **Using it**: importing a battle log (with your decklist), the opponent's
  likely deck, reviewing a game, rewinding and trying lines, drills
  (replay, before-you-move, numbers), and playing a game. Each with example
  prompts, matching the built-in guides.
- **Example game** to try: the GatoruSoju game and the Mega Lucario ex v12
  list worked end to end in ChatGPT today; publish it with the opponent's name
  anonymised.
- **Limits and expectations** of the hosted playmat: games kept in memory,
  dropped after an hour idle and on every deploy (keep the record), a cap on
  users served at once (10 today), a rate limit, Google sign-in.
- **Privacy**: what the hosted playmat keeps (Google account identity for
  sign-in, games in memory, the shared card cache) and what it does not.
- **Disclaimer**: an unofficial fan project, not affiliated with or endorsed
  by Nintendo, The Pokémon Company, Creatures or Game Freak; card data and
  images from TCGdex.

## Decisions taken without the user
- **default — hosting**: a static site, separate from the playmat, published
  from this repo (GitHub Pages). The playmat's own address keeps serving the
  board and MCP, so connectors already set up are untouched.
- **default — animation**: shows the real playmat's look (board, cards moving,
  a chat prompt driving it), plays without loading the live service, loops,
  stays small, and respects reduced-motion settings. It does not reproduce
  official Pokémon card artwork; it uses the board's own styling for cards.
- **default — one source for docs**: the site holds the user docs; the README
  shrinks to a short intro that links to the site, so the two do not drift.
- **default — no tracking**: no analytics, cookies or third-party scripts, so
  no consent banner is needed.
- **default — the logo** is the current project logo, kept in one place so it
  can be swapped. The Poké Ball in it is a trademark; whether to keep it for
  a public site is the user's call (flag it).

## Constraints
- Must work well on phones; many players will open a shared link on mobile.
- Accessible: readable contrast, alt text, keyboard navigation, reduced motion.
- No server: the site must keep working if the playmat is down.
- Content must match the real behaviour (tool names, guides, limits) and be
  easy to keep current as the playmat changes.
- Google sign-in for a public audience needs a public homepage and privacy
  policy link on the OAuth consent screen; the site provides both.

## Out of scope
- Changing the playmat, its sign-in, or its limits.
- Accounts, comments, search, a blog, versioned docs, translations.
- A developer manual or API reference beyond pointing at the repo.
- A custom domain (can be added later).
