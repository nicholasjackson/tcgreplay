# TCG Review Playmat: Design and Build Plan

Oct 15, 2026 · @Nic Jackson

## Goal and review loop

Build a local virtual playmat that Claude can drive through MCP, so we can review Pokémon TCG Live games, rewind to key decisions, demonstrate better lines and drill the underlying concepts. It is a playmat, not a rules engine: state is plain data, so any position can be built or rewound, and legality is policed by the players.

The loop, one game at a time:

1. **Triage.** Import the battle log. Games with few decisions and a loss traceable to the opening hand go to a deck-building bucket, not the review queue.
2. **Review.** Claude steps through Nic's turns and records findings: errors *and* strengths. Decision quality is rated separately from outcome, so a good play that lost is still a strength and a bad play that worked is still an error.
3. **Ask first.** For notable plays, Claude asks why Nic made them before giving a view; the answer is stored with the finding.
4. **Rewind and demonstrate.** Fork a branch before the decision, play the better line (or, for strengths, the tempting alternative), animate it with captions, and compare boards at end of turn. Test the line against several sampled opponent hands.
5. **Drill.** Generate a scenario that captures the concept, not the exact position: key cards fixed, the rest randomised, some variants where the obvious fix is wrong. Nic plays it; the attempt is graded.
6. **Repeat.** Failed concepts resurface; passed ones fade. Move to the next game.

The Notion game ledger stays the record of games; this tool stores reviews, branches, scenarios and attempts, linked by game ID.

## Architecture

One Go binary, run locally on the desktop, with two faces onto one in-memory state.

- **MCP over stdio.** The Claude desktop app launches the binary as a local MCP server. Claude reads and changes the board through tools.
- **Board UI on localhost.** The same process serves a Svelte playmat on `127.0.0.1`, pushing state over a websocket. The UI is a dumb, server-authoritative renderer: every drag or click becomes an event sent to the server.
- **Event-sourced store.** State is a pure fold over an append-only event log, persisted as one JSONL file per game. Rewind, branching and crash recovery all come from replaying the log.
- **Card data.** Card text and images from TCGdex, cached locally. Claude quotes card text from the cache rather than memory.
- **Static assets** embedded with `go:embed`, so the whole thing ships as a single executable.

Because the app owns the process lifecycle, games live in memory for the session only: a restart starts with no game open (a game comes back by importing its record), and the browser reconnects with a load-state-on-connect message. Fallback if this proves annoying: a long-running daemon with a thin stdio shim.

No remote hosting, auth or HTTPS. The trade-off is that play happens at the desktop, not from the phone.

## Event schema

Three layers: card instances, primitive events that change state, and actions that group primitives into something a player did. Annotations and warnings sit alongside and never alter state.

**Card instances.** Every physical card has a stable instance ID separate from its definition, so duplicates are distinguishable as they move.

```json
{"iid": "p1-37", "def": "sv04.5-012", "owner": "p1"}
```

Hidden cards from an import get an `iid` with `def: null` until an `assume` event fills them.

**Zones**, addressed as `{seat, zone, slot?, pos?}`:

| Zone | Kind | Notes |
| --- | --- | --- |
| `deck` | Ordered | Position matters |
| `prizes` | Ordered | Six slots |
| `hand`, `discard`, `lost` | Unordered |  |
| `active`, `bench[0..n]` | Stack | Evolution chain (top card current) plus attachments tagged `energy` or `tool`; bench size configurable |
| `stadium` | Shared |  |

**Event envelope**

```json
{
  "id": 142,
  "parent": 141,
  "action": "a-58",
  "turn": 4,
  "actor": "claude",
  "seat": "p2",
  "type": "move",
  "payload": {
    "iid": "p2-12",
    "from": {"zone": "hand"},
    "to": {"zone": "bench", "slot": 2}
  }
}
```

- `parent` makes the log a tree. A branch is a named pointer to a head event; rewinding creates a new head and nothing is deleted.
- `actor` is `human`, `claude`, `system` or `import`.

**Primitive event types**

| Type | Purpose |
| --- | --- |
| `setup` | Load decks with instance IDs |
| `move` | Any card, any zone, optional `face` up or down |
| `shuffle` | Stores the resulting order, not a seed |
| `coin` | Stores the result |
| `damage` | Set the counter on a stack |
| `condition` | Add or clear a Special Condition |
| `effect` | Lingering effect with source, target and expiry, e.g. Items locked until end of p1's next turn |
| `reveal` | A card shown to the opponent |
| `turn` | Start or end |
| `assume` | Fill hidden information, with provenance: decklist subtraction, reference list or manual |

Recording random outcomes rather than seeds makes replay deterministic with no RNG, and a branch from before a shuffle gets a genuinely new shuffle.

**Actions** group primitives into one player action, giving the UI one animation, the timeline one line and rewind a sensible boundary:

```json
{"id": "a-58", "label": "Ultra Ball", "events": [139, 140, 141, 142]}
```

**Side channels** (outside the fold): annotations `{event, author, text}`; warnings derived from state when computed, never stored.

**Views.** Events hold the full truth. Per-seat projections redact hidden zones; `reveal` lifts redaction; the reveal view applies none.

**Storage.** One JSONL file per game, all branches in it, with a `schema` version in the header line.

**Invariants**, checked after every import and on load: each seat holds exactly 60 cards, and hand sizes match the counted draws and plays.

## MCP tool contract

Claude plays a fixed seat (`p2` by default), inferred by the server rather than passed as a parameter. Claude acts through verbs that emit primitive actions, with `move` as an escape hatch.

**Design rules**

- Cards are referenced by `iid`, never by name.
- Every write returns `{action_id, events, delta}`, so Claude needn't re-read the board after each move.
- Errors are structured and cover location, not rules: "p2-12 is not in hand" is rejected; "second Supporter this turn" passes with a warning.
- A turn lock means a seat's verbs are only accepted on its turn. Cross-seat effects (Iono, Boss's Orders) are performed by the acting player.
- Each read logs which view it used, so coaching that leaks hidden information is auditable.

| Group | Tools |
| --- | --- |
| Reading | `get_state(view)` with `seat`, `coach` (Nic's hand plus public board) or `reveal` (review mode only); `get_log(since)`; `get_warnings()`; `card_text(def)` |
| Verbs | `draw(n)`, `bench(iid, slot)`, `evolve(iid, slot)`, `attach(iid, slot)`, `play_trainer(iid)`, `use_ability(slot, name)`, `retreat(to_slot, energy_iids)`, `switch(to_slot)`, `attack(name, target, damage)`, `knock_out(slot)`, `take_prize(prize_slot)`, `set_condition(slot, cond)`, `place_damage(slot, amount)`, `end_turn()` |
| Deck and randomness | `search_deck(filter?)` returns the deck sorted, never in true order; `look_top(n)`; `shuffle()`; `coin_flip(n)` |
| Compound actions | `play_trainer` and `use_ability` open an action; following verbs join it until `end_action()` |
| Escape hatch | `move(iid, to, face?)` |
| Game management | `new_game(deck_p1, deck_p2)` (TCG Live export format), `import_log(text, assumptions)`, `list_branches()`, `rewind(event_id)`, `checkout(branch)`, `set_mode(play or review)` |
| Review loop | `next_game()`, `flag_finding(...)`, `create_scenario(spec)`, `start_scenario(id)`, `grade_attempt(attempt_id)`, `get_concept_stats()` |
| Coaching | `annotate(event_id, text)`: inline on the timeline and as a toast on the board |

An Ultra Ball as a compound action:

```
play_trainer(p2-08)   → opens a-58
move(p2-31, discard)
move(p2-44, discard)
search_deck("Basic")
bench(p2-12, 2)
shuffle()
end_action()
```

Turn sync: Claude does not receive pushed events. Nic plays in the UI, presses **End turn** (which copies a prompt to the clipboard), and pastes it into chat; Claude then calls `get_log`, plays its turn and annotates.

## Playmat UI

A mirrored playmat, opponent on top, rendering server state and sending every interaction back as an event. Built fresh in Svelte, reusing CABT Viewer's card rendering and deck-search modal where practical (MIT licence); CABT's interaction model is engine prompts, so the drag-and-drop layer is new.

```
[Prizes]  [Bench x5]        [Deck][Disc]
          [ Active ]           [Lost]
---------- Stadium · Turn · Coin ----------
          [ Active ]           [Lost]
[Prizes]  [Bench x5]        [Deck][Disc]
[            Your hand (fanned)          ]
```

A collapsible side panel holds the timeline and annotations.

**Card interactions**

- Drag between zones for anything; a context menu for attach, evolve, retreat, play to bench, discard.
- Damage controls (±10, ±50, set) with an HP-remaining badge.
- Special Conditions as at a real table: rotate for Asleep, Confused, Paralysed; markers for Poisoned and Burned.
- Attached Energy and Tools tucked under the Pokémon, expandable on tap.
- Lingering effects (e.g. an Item lock) shown as a badge with expiry.
- Tap any card to zoom with full TCGdex text.

**Deck and zone actions:** draw, look at top N, search (sorted by type), shuffle, coin flip, all executed server-side. Clicking a prize takes it; the card is unknown until it lands in hand.

**Views**

- **Seat view** (default): what Nic could legitimately see.
- **Reveal view**: both hands and prizes face up, for post-mortems only, with a loud coloured border so it is never left on during play.

**Animation and playback**

- Claude's actions animate one at a time with a highlight, never as a sudden jump.
- Solution lines play with captions from annotations, with play, pause and step controls.
- Side-by-side comparison of end-of-turn boards: original line against the alternative.

**Timeline panel:** every action by turn, findings marked by colour (strength or error), scrubbing in read-only mode, "branch here" to fork, and branches shown as a tree.

**Turn flow:** an End turn button logs the event and copies a prompt for Claude. Soft warnings (second Energy attachment, second Supporter, Supporter by the first player on turn one) are shown and logged, never blocking. An optional checklist panel shows Nic's current focus points before ending the turn.

## Log importer and opponent deck inference

Nic's side of an imported game is always exact; the opponent's side is a plausible reconstruction. Rewound positions mean "if they held a typical hand", not a true replay.

**What a TCG Live log gives us**, based on the 15 October game against LeDjo23:

| Recoverable | Not recoverable |
| --- | --- |
| Turn structure, coin flip, who went first | Opponent's hand and draws (15 unnamed cards in two turns) |
| Nic's opening hand and every draw, named | Both players' prizes |
| Public board: Basics, evolutions, attachments, retreats, damage, knockouts | Deck order for both sides |
| Revealed searches and discards | Bench slot positions |
| Prize counts | Lingering effects (Itchy Pollen's Item lock never appears) |

**Parser hazards seen in that log**

- **Ownership mislabelled.** "LeDjo23 put 14 damage counters on LeDjo23's Solrock" targets Nic's Solrock. Resolve targets against the board, not the name prefix.
- **Inconsistent verbs.** "LeDjo23 played Abra" followed by Abra shuffled into the deck is an effect, not a card being played; interpreting it needs card text.
- **Nested sub-events.** Triggered effects appear as their own lines with `-` and `•` indentation; parse the nesting as structure.
- **Ambiguous card names** across sets; mapping to TCGdex definitions needs a decklist.

**Filling hidden information**

- **Nic's side:** decklist minus every named card gives the exact pool for deck and prizes; split randomly but consistently with everything seen.
- **Opponent's side:** build an inferred deck (below), then draw their hand from the unplaced pool at the size the log implies.

**Opponent deck inference: constraints from the log, priors from the meta**

1. **Hard constraints.** Each revealed card sets a minimum; simultaneous presence tightens it. In the example game, four Abra were in play at once, so Abra ≥ 4.
2. **Archetype match.** Weight signature cards heavily (Alakazam with Dusknoir is distinctive; Budew and Poffin say little).
3. **Consensus list.** Aggregate several reference lists for the archetype into inclusion rates and typical counts; raise counts to observed minimums; trim the least common flexible slots back to 60.
4. **Tag every card** `observed`, `reference` or `filler`.
5. **Sample, don't guess.** Replay a position with several sampled hands to test whether a line holds up against what the opponent probably held.

Reference lists come from Limitless TCG tournament results (check its terms before automating), and from an archetype profile built from Nic's own imported games, which reflects ladder play rather than tournaments. Rogue decks fall back to a list Claude proposes, flagged as entirely assumed.

```json
{
  "archetype": "Alakazam Dusknoir",
  "as_of": "2026-10-15",
  "sources": ["limitless:…", "ledger:12 games"],
  "cards": [
    {"def": "…", "count": 4, "min_observed": 4, "basis": "observed"}
  ]
}
```

The deck profile lives in the `setup` or `assume` event, so it is part of the game's history and reproducible. Code handles constraints, sampling and invariants; Claude handles archetype judgement, reference lists and rogue cases.

## Findings, scenarios and drills

**Findings** rate decision quality and outcome separately, so the review learns from decisions rather than noisy results.

|  | Good outcome | Bad outcome |
| --- | --- | --- |
| **Good decision** | Strength | Unlucky, still a strength |
| **Poor decision** | Lucky, still an error | Error |

```json
{
  "game": "game-0412",
  "event": 142,
  "kind": "error",
  "category": "sequencing",
  "decision": "poor",
  "outcome": "bad",
  "severity": "game-losing",
  "explanation": "…",
  "player_reasoning": "…",
  "branch": "fix-1"
}
```

- A **strength** needs a non-obvious decision where a plausible alternative was worse. Routine plays are not findings.
- Every finding names the principle so it transfers, and links a **counterfactual branch**: the better line for an error, the tempting alternative for a strength.
- Categories map to Nic's existing play rules and focus points.
- Where several lines are acceptable, the finding says so rather than inventing one right answer.

**Scenarios** capture a concept, not an exact position:

```json
{
  "concept": "don't overbench into Alakazam",
  "source": "game-0412/fix-1",
  "deck": "lucario-v7",
  "opponent": "Alakazam Dusknoir",
  "fixed": ["key cards in hand and play"],
  "randomised": ["rest of hand", "prizes", "deck order"],
  "objective": "…",
  "check": "mechanical | judged"
}
```

- Fixed elements recreate the decision; randomised elements change each attempt.
- Some variants make the obvious fix wrong, so Nic learns the principle, not a reflex.
- Confirmation variants test strengths in an unfamiliar context.
- Checks are mechanical where possible (prizes taken, bench size, damage dealt), judged by Claude otherwise.

**Attempts** are recorded per concept, counting successes and failures, so stats show a rate ("handles Powerful Hand pressure in 7 of 10") rather than a list of failures. Failed concepts resurface; passed ones fade.

## Status (2026-10-16)

Phases 0–2 are built. Phase 3's importer is built: all 10 corpus logs in `testdata/logs` replay with no problems, and the `import_log` MCP tool turns a log into a review-mode game held in memory for the session, replacing any game open (it remembers the last decklist for the session), and returns the game's record. Deck inference has started with the parts that need no outside data (`internal/infer`):

- **Proven minimums.** `Report.Observed` is the most copies of each opponent card Nic could see at once (public zones, plus cards revealed to him in hand). It is counted by name, never by card instance, so it is a sound lower bound; the old count of bound instances overcounted (6 Crushing Hammer, 3 Dudunsparce). Named opponent draws (searches) are now revealed to Nic in the event log. Corpus spot checks live in `testdata/index.json` under `observed`.
- **Four-copy cap.** The replay never binds a fifth copy of a card (Basic Energy aside); past four it reuses a copy already seen by swapping it in from the deck.
- **Profiles.** The setup event's profile holds the opponent, the proven minimums and, when a list is given, the reconciled list (counts raised to what was seen, cards tagged `observed` or `reference`). `get_profile` returns it with `seen_so_far`, the minimums at the current head.
- **Sampling.** `sample_hidden(deck?, seed?)` gives the opponent's hidden cards (hand, deck order, prizes) identities at the head, as an assume event with basis `sampled`. Cards Nic can see keep their identities; a card can only be a prize as often as the list allows after the copies Nic saw outside the prizes since the deal. A list given once is stored and reused on any branch. Review mode only. Workflow: `rewind(e)` → `sample_hidden` → play; repeat per sampled hand.

- **Complete opponent decks.** `import_log` takes `opp_deck`, a likely list Claude builds; the replay holds the opponent to its counts (falling back with a warning if the log contradicts it) and deals every unnamed card from what is left, so both sides are 60 cards. `set_opponent_deck` rebuilds the current imported game with a list, from its in-memory record (same log and seed), and returns the updated record; it refuses a game with branches unless `discard_branches` is passed, since the rebuild replaces the game and its branches are lost. Opponent cards get art by name lookup (newest non-Pocket print).
- **Game records.** The playmat keeps no games: every imported game carries a record instead, a small JSON object holding the raw game only (`tcgreplay_record` version marker, the player's name `me`, the `seed` as a decimal string, `deck`, `opp_deck` as passed, the battle `log`) and a `check`, a SHA-256 over those values. `import_log` and `set_opponent_deck` return it and `get_record` returns it for the current game; a game started with `new_game` has none. Passing a record to `import_log` as `log` rebuilds the identical game, card for card at every point of the timeline, because the importer's only randomness comes from the seed. The check covers values, not layout, so re-indenting is harmless; curly quotes, non-breaking spaces and Windows line ends added by a note app are undone and accepted only if the check then matches, and anything else is refused as damaged. Branches, notes and findings are not in the record and last only for the session.
- **Guides (any assistant).** The coaching workflows are markdown in `internal/guides` (embedded in the binary): `import-game`, `opponent-deck`, `review-game` (ask first, rewind to the point, test lines on branches, record `Finding (<category>, ...)` annotations using the ledger's error categories), `replay-drill` (findings as puzzles with the answer hidden), `before-you-move` (three-question routine on a real board), `numbers-drill` (damage order from real cards). Written for any player and any assistant: bare tool names, no client-specific tools. They reach every client from the playmat itself, hosted and desktop alike: the `guide` tool lists them or reads one by name, each is also an MCP prompt for clients that show prompts as commands, and the server instructions end with one line per guide saying when to use it and to read it first. The text checks run in the guides' and the MCP server's tests (format, no assistant or player names, turn naming, cross-references, only real tools). Clients register the playmat with their own tools (for example `claude mcp add`, or a claude.ai custom connector for hosted). The playmat labels the assistant's seat with the connected MCP client's name. Later: a quiz game mode.

**Runtime.** `tcgreplay` is a stdio shim relaying to one shared daemon (`tcgreplay -daemon`, socket `<data>/mcp.sock`), which owns the current game (in memory only) and the UI on :7777. No mode writes or reads game files; files an older build left in the data directory are left untouched and ignored. After `make install`, restart the desktop app; the first shim from the new build replaces the old daemon. `-no-daemon` runs everything in-process. Logs: `~/.local/share/tcgreplay/tcgreplay.log`. **Dev mode:** `make dev` builds and runs this checkout's daemon in the foreground with its own data (`.dev/`) and UI (:7778), beside the installed one; Claude Code in the repo reaches it as the `tcgreplay-dev` server in `.mcp.json`. `make dev-ui` adds Vite hot reload against it. After a rebuild, rerun `make dev`: shims reconnect to the new daemon and replay the MCP handshake, so the client stays connected (a call in flight at that moment fails with a retry message).

**Hosted mode.** `tcgreplay -hosted` runs everything in-process and serves it on one plain-HTTP port, for a container behind an HTTPS proxy: MCP over Streamable HTTP at `/mcp`, and the board on every other path (`/`, `/ws`, `/api/card`). Settings: `-listen` (default `:$PORT`, else `:8080`) and `-public-url` (default `$TCGREPLAY_PUBLIC_URL`; scheme and host only), the address users reach it at. The public URL is the only source for the `board` link that `import_log`, `new_game` and `set_opponent_deck` return in hosted mode, and for the board's allowed websocket origin; the request's `Host` is never trusted. Without it, hosted mode falls back to `http://localhost:<port>` with a warning. Logs go to stderr. The board is pinged every 30 s so proxies (Cloudflare drops idle connections after ~100 s) keep it open. Locally: `make docker-build`, then `make docker-run` (override `PUBLIC_URL`; export the Google credentials below first), and the board is at http://localhost:8080/. Deployment: `.do/app.yaml` defines one DigitalOcean App Platform service built from the `Dockerfile`, deployed on every push to `main` (other branches do not deploy), with `TCGREPLAY_PUBLIC_URL` set to the app's own URL and the health check on `/healthz`; replace its placeholder domain with yours and create the app once with `doctl apps create --spec .do/app.yaml`. In claude.ai, add a custom connector at `https://<domain>/mcp`. **Trial limits:** every account's game is lost on every deploy or restart (sign-ins survive one when `TCGREPLAY_SIGNING_KEY` is set). So is the card cache, unless the shared store is configured (the `TCGREPLAY_SPACES_*` settings below): then the cached card data is kept in a DigitalOcean Spaces bucket under `tcgdex/`, and only the local disk copy is per run. With the shared store, card images are served from the bucket's CDN too: each image the board shows is copied byte for byte from TCGdex (the only image source, never resized or recompressed) to `images/<TCGdex path>/low.webp` and `high.webp`, public-read with `Cache-Control: public, max-age=604800`, and kept until an admin clears it; the bucket and its `tcgdex/` card data stay private. `/api/card` answers a copied image with its CDN address (`https://<bucket>.<region>.cdn.digitaloceanspaces.com/images/…`) and the TCGdex address as `image_fallback`, and an image without a copy with the TCGdex address, copying it in the background so the next page load uses the CDN; the board tries the CDN, then TCGdex, then its text stand-in. At start-up the playmat lists `images/` to learn which copies exist (`card images: N copies found in the shared store`), and never writes over an object already in the bucket. Each copy's address carries a content version, `?v=<12 hex>` (`/api/card` sends it as `image_version`; the board adds it to the CDN address only, never to the TCGdex fallback), taken from the copy's two objects' ETags (the MD5 of their bytes), so it is the same across restarts and changes exactly when the bytes do: changed art gets a new address that no browser or CDN edge has cached, despite the week-long `Cache-Control`. Every hour with activity it logs `card images: last hour N from the shared store, M from the card source (P% shared), K copied`, the measure of how much of the board's art comes from the CDN. Games are kept only in memory in every mode, so the player keeps each game's record to bring it back. The desktop modes above are unchanged.

**Card cache admin page.** Accounts named in `TCGREPLAY_ADMINS` (Google account emails, any case, only when Google marked the email verified) can open `/admin` on the hosted playmat after the usual Google sign-in. Anyone else gets a "Not an admin" page (403) and a signed-out browser is sent through sign-in; a direct clear request that is signed out gets 401, from a non-admin 403, and one not sent from the playmat's own pages (Origin check) 403. A sign-in from before this page existed counts as unverified, so an admin signs out and in once. The page clears the card cache, either **everything** or **one set** named by its decklist code (`MEG`) or TCGdex id (`me01`, any case); a name that matches no set, even after refreshing the set list, is refused and nothing is cleared. A clear reaches every tier: memory, the local disk cache, the shared store's `tcgdex/` card data (`sets/<id>` and every `cards/<id>-…`) and the `images/` copies (for a set, those whose TCGdex path has the set id as its third segment; borrowed Crown Zenith Energy art belongs to `swsh12.5`), and, with `TCGREPLAY_SPACES_CDN_TOKEN` set, purges the bucket's CDN for those paths (`images/*`, or `images/<lang>/<serie>/<set>/*`). It runs as one background job at a time; the page refreshes while it runs, then shows what each tier removed, whether the CDN was purged and any error (a failed tier can be cleared again; clears are idempotent). Every clear is logged when it starts and ends, with the admin and the scope. Open games keep playing: cleared cards stay visible to the board from memory while fresh copies are fetched in the background, and every card, deck-code and name lookup after the clear asks TCGdex again. An open board shows the fresh card data and art after a reload. Without the shared store the page clears memory and disk only. Desktop builds have no admin page; their cache is cleared by deleting the cache directory.

**Hosted sign-in.** Hosted mode requires a Google account for everything; the desktop modes never ask for sign-in. Any Google account can sign in (no allowlist). The playmat is its own small OAuth sign-in service for MCP clients and hands the login itself to Google (`internal/hostauth`):
- *Connector:* an anonymous request to `/mcp` gets 401 with a pointer to `/.well-known/oauth-protected-resource/mcp`. That leads claude.ai to the playmat's sign-in metadata (`/.well-known/oauth-authorization-server`); claude.ai then registers itself (`/register`, Dynamic Client Registration) and sends the user through `/authorize`, which always shows Google's account chooser. Afterwards claude.ai exchanges a one-time code at `/token` (PKCE S256) for an access token, which lasts an hour, and a refresh token, which lasts 30 days and is replaced each time it is used. Only Claude's callbacks (`https://claude.ai/api/mcp/auth_callback`, `https://claude.com/api/mcp/auth_callback`), ChatGPT's callbacks (`https://chatgpt.com/connector_platform_oauth_redirect`, and `https://chatgpt.com/connector/oauth/{callback_id}`, which ChatGPT uses because the playmat does not advertise RFC 9207 issuer identification) and loopback addresses (Claude Code) may be registered.
- *Board:* opening a board page while signed out goes through Google and comes back to that page; a signed-out live connection (`/ws`) or card lookup (`/api/card`) is refused. A browser that signed in for the connector is already signed in to the board. The header shows the account and a **Sign out** button. Signing out ends that browser's board session and closes its live connections at once, but leaves the connector connected; disconnect that in claude.ai.
- *Restart:* nothing about sign-in is written anywhere. Instead, the client IDs, access and refresh tokens and the board cookie the playmat hands out are sealed (encrypted and authenticated, AES-GCM) with `TCGREPLAY_SIGNING_KEY`, so after a restart or redeploy with the same key everyone stays signed in: claude.ai's access token still works, its refresh still renews it, and the board cookie (30 days) starts its session again. Without the key, one is made up at start and a restart signs everyone out: the board sends the user back to Google, and claude.ai's next refresh is answered `invalid_client`, its signal to register and ask the user to sign in again. *Trade-off:* signing out, and a refresh token's single use, are remembered only by the instance that saw them, so a cookie or refresh token copied before then works again after a restart until it expires; changing the key signs everyone out.
- *Limits:* one per-account allowance counts MCP requests, board actions and card lookups. Over it, Claude gets a tool error that names the limit, for example "too many requests: this account is limited to 300 requests a minute; try again in 1s"; the board shows the same message, and a card lookup gets 429. Other accounts carry on. The user cap counts *places* (see per-account games below): once every place is held, a new account sees a "tcgreplay is busy" page (503), whichever way it signs in. An account holding a place can always sign in again.
- *Logs:* every sign-in, every refused unsigned request (path and reason, never a token), and every rate-limit and busy refusal goes to stderr.

| Setting | Flag | Default |
|---|---|---|
| `TCGREPLAY_GOOGLE_CLIENT_ID` | `-google-client-id` | none: hosted mode will not start |
| `TCGREPLAY_GOOGLE_CLIENT_SECRET` | environment only | none: hosted mode will not start |
| `TCGREPLAY_SIGNING_KEY` (keeps sign-ins across restarts; at least 32 characters, e.g. `openssl rand -base64 48`) | environment only | none: a restart signs everyone out; a shorter key stops start-up |
| `TCGREPLAY_RATE_LIMIT` (requests a minute per account) | `-rate-limit` | 300; `0` turns it off |
| `TCGREPLAY_MAX_USERS` (places: accounts served at once) | `-max-users` | 10; `0` turns it off |
| `TCGREPLAY_IDLE_TIMEOUT` (idle time before a game is dropped; Go duration, e.g. `2m`, `1h`) | `-idle-timeout` | `60m`; `0` never drops; an invalid value stops start-up |
| `TCGREPLAY_SPACES_ENDPOINT` (shared card cache: Spaces endpoint, e.g. `https://ams3.digitaloceanspaces.com`) | environment only | none: card cache kept on local disk only (lost on restart); some but not all four set → warning, local only |
| `TCGREPLAY_SPACES_BUCKET` (shared card cache: the private bucket) | environment only | none: card cache kept on local disk only (lost on restart); some but not all four set → warning, local only |
| `TCGREPLAY_SPACES_KEY` (shared card cache: Spaces access key id) | environment only | none: card cache kept on local disk only (lost on restart); some but not all four set → warning, local only |
| `TCGREPLAY_SPACES_SECRET` (shared card cache: Spaces secret key) | environment only | none: card cache kept on local disk only (lost on restart); some but not all four set → warning, local only |
| `TCGREPLAY_ADMINS` (cache admins: comma-separated Google account emails allowed to use `/admin`) | environment only | none: nobody can use the admin page |
| `TCGREPLAY_SPACES_CDN_TOKEN` (DigitalOcean API token with CDN read and update access; purges the bucket's CDN when an admin clears image copies) | environment only | none: clears delete the copies but do not purge the CDN; ignored without the four `TCGREPLAY_SPACES_*` settings |

Settings are read at start-up, so a change needs only a restart, not a rebuild. The platform health check is `GET /healthz`, which needs no sign-in, since `/` now redirects to Google.

*One-time setup for an operator:*
1. In the Google Cloud console, open APIs & Services → OAuth consent screen. Choose user type **External**, give the app a name and support email, and request only the `openid` and `email` scopes. Then **Publish app** so its status is "In production". These scopes need no verification, and while the app is in "Testing" only listed test users can sign in.
2. Go to Credentials → Create credentials → OAuth client ID and choose **Web application**. Add the authorised redirect URIs `https://<domain>/auth/google/callback` and, for `make docker-run`, `http://localhost:8080/auth/google/callback`. No JavaScript origins are needed.
3. Set `TCGREPLAY_GOOGLE_CLIENT_ID`, `TCGREPLAY_GOOGLE_CLIENT_SECRET` and `TCGREPLAY_SIGNING_KEY` in the App Platform component's environment variables, encrypted, and set the health check path to `/healthz`. `.do/app.yaml` declares these, but `doctl apps update` does not carry secret values, and an app made in the console needs them set there. For a local container, export both variables and run `make docker-run`.
4. If Cloudflare's bot protection is on for the domain, make sure it does not challenge `/.well-known/*`, `/register`, `/token` or `/mcp`, which claude.ai calls from Anthropic's servers (`160.79.104.0/21`).
5. Remove any connector added before sign-in existed and add it again at `https://<domain>/mcp`, so claude.ai finds the sign-in.
6. Optional, to keep the card cache across deploys: in the DigitalOcean control panel create a Spaces bucket in a region near the app, with file listing restricted (private); the card cache does not need the CDN (card images do: step 7). Create a Spaces access key limited to that bucket with read and write, and set `TCGREPLAY_SPACES_ENDPOINT` (`https://<region>.digitaloceanspaces.com`), `TCGREPLAY_SPACES_BUCKET`, `TCGREPLAY_SPACES_KEY` and `TCGREPLAY_SPACES_SECRET` on the component, encrypted (as with the Google secrets, `doctl apps update` does not carry their values). The start-up log then says `card cache: … backed by shared store …`, followed by `card cache: catalog warmed in …`. A shared-store outage is logged and skipped, never shown to players. Desktop builds never read these settings. To force fresh card data, an admin clears it from `/admin` (step 8).
7. Optional, with step 6, to serve card images from the bucket's CDN: in the Spaces bucket's Settings → CDN, enable the CDN with an edge cache TTL (1 week, the longest, suits images that never change), keeping file listing restricted. The step-6 access key must be able to list the bucket and write objects with a public-read ACL (a "Limited access" key with read and write on the bucket can); `tcgdex/` objects stay private. Redeploy or restart; the start-up log says `card images: … served from https://<bucket>.<region>.cdn.digitaloceanspaces.com/images/` and `card images: N copies found in the shared store`. Copies are made even before the CDN is enabled; until then the board's CDN addresses fail and it falls back to TCGdex, so players see no difference. To replace copies with the current TCGdex art, an admin clears their set (or everything) from `/admin` (step 8).
8. Optional, for the card cache admin page: set `TCGREPLAY_ADMINS` on the component to the admins' Google account emails, comma-separated. To have clears purge the CDN too (with steps 6 and 7), create a DigitalOcean API token (API → Tokens → Generate New Token, custom scopes: CDN read and update only, `cdn:read` and `cdn:update`, with an expiry that suits you) and set it as `TCGREPLAY_SPACES_CDN_TOKEN`, encrypted (`doctl apps update` does not carry its value). The playmat finds the bucket's CDN endpoint itself, by its origin. The step-6 access key must be able to list and delete objects in the bucket (a "Limited access" key with read and write on the bucket can). Redeploy or restart; the start-up log ends with `N cache admins` and says `card images: CDN purge on clear: on`.

**Per-account games.** In hosted mode every signed-in Google account has its own game, held in memory (`internal/usergames`): its own game manager and its own board server, created the first time the account is seen. All of an account's claude.ai conversations, board tabs and devices see and act on that one game, so an import in one replaces it for all of them; one account's imports, moves and rewinds never touch another's. Each MCP tool call finds the caller's game from its access token, and the board's routes go to the signed-in account's own board server, whose live connection carries only that game's changes. Where a game does not name the player's side, the hosted board labels it with the signed-in account's name (the email before the `@`) instead of "You".
- *Board links:* `import_log`, `new_game` and `set_opponent_deck` return `<public URL>/?board=<handle>`, where the handle is a stable, opaque tag derived from the Google account id (the same across restarts and imports, revealing nothing). The link opens only for its owner: another signed-in account sees a "Not your game" page (403) with nothing of the game and a Sign out button, and a signed-out visitor is sent through Google sign-in first. `/` with no handle opens the signed-in account's own board, so links handed out before per-account games still work.
- *Idle drop:* a game with no activity for longer than `TCGREPLAY_IDLE_TIMEOUT` (default 60 minutes; `0` never drops) is cleared from memory. Activity is any tool call from Claude, any move on the board, or a board page load; a board tab left open (its live connection, reconnects, card lookups) does not count. Expiry is checked on every lookup, so an account's next request always sees it, and by a background sweep (every quarter of the idle time, at most every 30 s) for games nobody touches. Afterwards every tool call or board action that needs the game says it expired after the idle time and asks for its saved game record to be imported again, and an open board shows the same message at once (if the server has filled up meanwhile, the account is told it is busy instead). Importing the record (or starting a game) brings it back as usual.
- *Places:* `TCGREPLAY_MAX_USERS` is the number of places, accounts served at once. A place is taken at sign-in, or by an account's first action after losing one, and freed when the account's game is dropped as idle (an account that signs in and never plays gives its place back after the same idle time), when the account's last sign-in ends (a board sign-out with no connector connected), or on restart (a sign-in kept across the restart takes a place again on its next action). An account acting without a place while every place is held is told the server is busy. Once the server is full, a new account can sign in as soon as someone else's game expires. Accounts without a place and without activity for a day are forgotten, so memory stays bounded by the places and the idle time. The registry logs each expiry and, on each sweep that changed anything, the number of accounts, places held and games.
- *One instance:* every account's game lives in one process's memory, so the hosted service must run as a single instance (`instance_count: 1` in `.do/app.yaml`); a second copy would not see the first one's games or sign-ins.
- *Desktop:* the desktop modes build none of this. They keep one game, no accounts, no idle drop and no board-link handle.

**Debugging the importer.** `TRACE=<log path> go test ./internal/importer -run Trace -v` prints each line with a board summary. Handlers must never mutate the parsed log: it is shared across the search's replay attempts, and mutating it silently breaks the backtracking.

**Next.** Try the sampling workflow on a real review with a list Claude proposes. Then the reference-data half of Phase 3: archetype match and consensus lists. Still open: whether reference lists come from Limitless TCG (check its terms or API first) or from lists Nic pastes in. Trimming a reconciled list back to 60 needs inclusion rates, so it waits for reference data; today Reconcile only warns when the total is off.

## Build plan

Six phases, each usable on its own. Tomorrow's session covers Phase 0 and as much of Phase 1 as time allows; Phase 1 alone lets us play and rewind games through chat.

**Phase 0: Scaffold**

- [x] Go module, repo layout (`cmd/`, `internal/state`, `internal/mcp`, `internal/cards`, `web/`)
- [x] Choose an MCP library: the official Go SDK or an established community one
- [x] Register the binary as a local MCP server in the desktop app; a `ping` tool round-trips

**Phase 1: State, events and MCP (no UI)**

- [x] Card instances, zones and stacks
- [x] Event envelope, primitive events, actions; append-only JSONL with schema header
- [x] Fold to state; `rewind`, `branch`, `checkout`
- [x] Server-side shuffle, coin flip, prize setup
- [x] Deck import from TCG Live export text
- [x] TCGdex lookup and local cache; `card_text`
- [x] Verbs, compound actions, `move` escape hatch, per-seat views, turn lock
- [x] Invariant checks

* **Done when:** a full game can be played through chat, and rewinding to any event reproduces that state exactly.

**Phase 2: Playmat UI**

- [x] Svelte app embedded with `go:embed`, websocket with reconnect and load-on-connect
- [x] Mirrored layout, drag and drop, context menus, damage, conditions, effect badges
- [x] Deck actions, card zoom, seat and reveal views
- [x] Animated actions, End turn button with clipboard prompt, soft warnings

* **Done when:** Nic plays his seat in the browser while Claude plays through MCP, with no divergence.

**Phase 3: Importer and deck inference**

- [x] Log parser with nesting, target resolution and card-name mapping
- [x] Placeholder cards, `assume` events with provenance
- [x] Constraint extraction (proven minimums, four-copy cap, prize exclusion) and hand sampling
- [ ] Archetype match and consensus list from reference data

* **Done when:** the 15 October LeDjo23 log imports with invariants passing and every assumption flagged.

**Phase 4: Review**

- [ ] Findings schema and `flag_finding`, strengths and errors
- [ ] Timeline panel with colour-coded findings, scrubbing, branch tree
- [ ] Captioned playback of solution lines, side-by-side end-of-turn comparison
- [ ] Testing a line against several sampled opponent hands
- [ ] Triage: auto-flag opening-hand losses into the deck-building bucket

**Phase 5: Drills**

- [ ] Scenario schema, `create_scenario`, `start_scenario` with fresh variants
- [ ] Mechanical and judged grading; attempts per concept; resurfacing
- [ ] Link reviews and concept stats to the Notion ledger by game ID

**Phase 6: Light validation**

- [ ] Rule checks on verbs (one Energy attachment, one Supporter, first-turn rules), still as warnings

## Open questions and risks

- [ ] **Name.** The project needs one.
- [ ] **Hidden information when Claude plays both roles.** Seat views and read logging make leaks auditable, but Claude still sees both hands in context. Accept this, or run the opponent as a separate sub-agent with only the `seat` view.
- [ ] **Rules accuracy.** Without an engine, Claude adjudicates rules and can be wrong. Mitigations: quote card text from the cache, log warnings, Nic challenges anything that looks off.
- [ ] **Log format stability.** TCG Live log wording may change between client versions; keep the parser table-driven and test against a growing corpus of real logs.
- [ ] **Reference data terms.** Confirm TCGdex and Limitless TCG terms allow automated use and local caching of text and images.
- [x] **Process lifecycle.** The app launched several server processes, so the UI could show a different game than Claude drove. Resolved 2026-10-16: the binary is now a stdio shim relaying to one shared daemon (Unix socket in the data dir), which owns games and the UI and is replaced automatically after a reinstall.
- [ ] **Deck-building bucket.** Lone-Basic openings (as in the LeDjo23 game) point at list construction; decide whether opening-hand analysis is in scope here or a separate tool.
- [ ] **Ledger integration.** Agree the game ID shared between this tool and the Notion ledger.
