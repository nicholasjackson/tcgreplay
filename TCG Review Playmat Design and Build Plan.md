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

Because the app owns the process lifecycle, a restart rebuilds state from disk and the browser reconnects with a load-state-on-connect message. Fallback if this proves annoying: a long-running daemon with a thin stdio shim.

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

Phases 0–2 are built. Phase 3's importer is built: all 10 corpus logs in `testdata/logs` replay with no problems, and the `import_log` MCP tool saves a log as a review-mode game (it remembers the last decklist in `<data>/last-deck.txt`). Deck inference has started with the parts that need no outside data (`internal/infer`):

- **Proven minimums.** `Report.Observed` is the most copies of each opponent card Nic could see at once (public zones, plus cards revealed to him in hand). It is counted by name, never by card instance, so it is a sound lower bound; the old count of bound instances overcounted (6 Crushing Hammer, 3 Dudunsparce). Named opponent draws (searches) are now revealed to Nic in the event log. Corpus spot checks live in `testdata/index.json` under `observed`.
- **Four-copy cap.** The replay never binds a fifth copy of a card (Basic Energy aside); past four it reuses a copy already seen by swapping it in from the deck.
- **Profiles.** The setup event's profile holds the opponent, the proven minimums and, when a list is given, the reconciled list (counts raised to what was seen, cards tagged `observed` or `reference`). `get_profile` returns it with `seen_so_far`, the minimums at the current head.
- **Sampling.** `sample_hidden(deck?, seed?)` gives the opponent's hidden cards (hand, deck order, prizes) identities at the head, as an assume event with basis `sampled`. Cards Nic can see keep their identities; a card can only be a prize as often as the list allows after the copies Nic saw outside the prizes since the deal. A list given once is stored and reused on any branch. Review mode only. Workflow: `rewind(e)` → `sample_hidden` → play; repeat per sampled hand.

- **Complete opponent decks.** `import_log` takes `opp_deck`, a likely list Claude builds; the replay holds the opponent to its counts (falling back with a warning if the log contradicts it) and deals every unnamed card from what is left, so both sides are 60 cards. `set_opponent_deck` rebuilds an imported game with a list, from the same log and seed (saved as `<id>.import.json`); it will not replace a game with branches. Opponent cards get art by name lookup (newest non-Pocket print).
- **Skills.** `.claude/skills/tcg-import-game` (read the log, build the list, import, triage) and `tcg-opponent-deck` (archetype, floors from the log, duplicates and normal Supporter/Energy totals for the guessed slots). Candidates next: turn-by-turn review, line testing with `sample_hidden`.

**Runtime.** `tcgreplay` is a stdio shim relaying to one shared daemon (`tcgreplay -daemon`, socket `<data>/mcp.sock`), which owns the games and the UI on :7777. After `make install`, restart the desktop app; the first shim from the new build replaces the old daemon. `-no-daemon` runs everything in-process. Logs: `~/.local/share/tcgreplay/tcgreplay.log`. **Dev mode:** `make dev` builds and runs this checkout's daemon in the foreground with its own data (`.dev/`) and UI (:7778), beside the installed one; Claude Code in the repo reaches it as the `tcgreplay-dev` server in `.mcp.json`. `make dev-ui` adds Vite hot reload against it. After a rebuild, rerun `make dev`: shims reconnect to the new daemon and replay the MCP handshake, so the client stays connected (a call in flight at that moment fails with a retry message).

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
