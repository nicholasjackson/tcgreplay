# Every player-visible change ships with a What's new entry

Every change players can see ships, in the same change, with an entry in `site/src/data/whats-new.yaml`, the list behind the website's What's new page (`/docs/whats-new`). That covers website pages, the board, tools, guides, the playmat's instructions to assistants, and limits. Internal changes (refactors, tests, tooling, deploy plumbing) need none. Every push to `main` deploys, so each merge is a release.

- Add the entry at the top (the list is newest first): `date` is the day it ships (`YYYY-MM-DD`), plus a short `title`, `refresh` and `changes`. `changes` are short player-facing lines in the docs' plain voice, saying what changed for players rather than what changed in the code; `[text](/path)` and `[text](https://...)` links are allowed.
- Set `refresh: true` when the change adds, removes or renames a tool, changes a tool's description, title or parameters, or changes a guide or the server instructions, because assistants keep the tool list they fetched when they connected. Otherwise set `refresh: false`. Entries with `refresh: true` link players to the refresh steps (`/docs/connect#refresh`).
- Never name an old address outside the connecting page's move note (`data-moved-note`). Never say TCG Replay is in ChatGPT's Plugin Directory while `site/src/lib/listing.ts` has `listed: false`. The browser suite fails on either.

The build (`site/src/lib/whats-new.ts`) stops on a missing or malformed date, title, refresh or changes. It cannot tell whether a change was player-visible, so applying this rule is the author's job, and the maintainer checks it.
