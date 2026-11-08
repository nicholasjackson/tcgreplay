---
created_date: "2026-11-08"
document_status: final
closed_date: "2026-11-08"
---

# Context: 20261108110847-055548e3-paired-stadiums

## Current State Analysis

All work is in the single registered repo `tcgreplay` (root `/home/nicj/code/github.com/nicholasjackson/tcgreplay`). This plan starts after the card-set-import and card-set-publishing plans have landed: built sets live in `cardsets/`, the playmat loads published sets through `internal/catalog`, `game.NewManager(dir, CardSource)` replaces `Manager.DB` with `Manager.CardDB()`/`Game.DB()`, and Delta Reign's Legendary Stadiums already carry their rule text by correction (no pair mark).

As of commit `1b8fc21`:
- `internal/state/state.go:84` — `Stadium IID`: the spot holds one card. `internal/state/apply.go:180-183` refuses any move into an occupied spot ("stadium is occupied by …; discard it first"). `Find` (`state.go:246`), `remove` (`apply.go:110`), `Locations` (`invariants.go:43`) and the view (`view.go:72,185-188`, `BoardView.Stadium *CardView`) all assume one card.
- `internal/state/events.go:93-104` — `MovePayload{IID, From, To, Face, As}` moves one card. `As` holds stack roles (`types.go:189-193`).
- `internal/game/verbs.go:428-457` — `PlayTrainer` replaces the one Stadium. `Move` (`:725-753`) moves one card, except `moveStack` (`:757-781`) for a Pokémon with its attachments.
- `internal/game/reads.go:433-489` — `turnWarnings` warns on "played N Stadiums this turn" by counting Stadium `trainer` actions. `benchWarnings` (`:498-515`) reads the Stadium's effect.
- `internal/game/reads.go:610-648` — `Undo` steps over whole actions. The board timeline (`web/src/components/Timeline.svelte:109-128`) shows one line per event.
- `internal/importer/handlers.go:92-106,494-511` — its own Stadium replace logic. A single `lastStadium` consumes the replaced card's "discarded" subline. Two consecutive same-name plays would be taken as a replacement, and one line naming two cards binds as one name (silently for the opponent).
- `internal/mcpserver/server.go:651-655` — `play_trainer` takes one iid (`IIDArg`). `card_text` (`:583-601`) returns `cards.Card.Text()`.
- `internal/cards/cards.go:72-93` — `Card` has no pair field.
- `web/src/components/Strip.svelte:41-47,103-118,191-192` — the spot renders one card. `web/src/lib/actions.js:105-110` sends `play_trainer` when a hand Stadium is dropped on the spot, and `move` otherwise.
- No test log contains Delta Reign or Legendary cards, or card codes (`testdata/logs/`, 11 logs).

Requirement → files:
- Pair-played Stadiums → `internal/cardset/format.go`, `internal/cardset/card.go`, `internal/cards/cards.go`.
- Play a Stadium pair → `internal/state/*`, `internal/game/verbs.go`, `internal/mcpserver/server.go`, `internal/webui/actions.go`.
- A pair moves as one → `internal/state/apply.go`, `internal/game/verbs.go`, `internal/game/reads.go`, `internal/game/ui.go`.
- Pair warnings → `internal/game/verbs.go`, `internal/game/reads.go`.
- Assistant knows the pair rule → `internal/cards/cards.go`, `internal/mcpserver/server.go`, `internal/guides/play-game.md`.
- Board shows a pair → `internal/state/view.go`, `web/src/components/Strip.svelte`, `web/e2e/*`, `internal/webui/webtest/webtest.go`.
- Import logs with a pair → `internal/importer/handlers.go`, `internal/importer/replay.go`, `testdata/`.
- Delta Reign's Legendary Stadiums marked → `cardsets/corrections/me06.json`, `cardsets/sets/me06.json`, `internal/cardset/committed_test.go`.
- Documentation → `site/src/content/docs/workflows/play-game.md`, `TCG Review Playmat Design and Build Plan.md`, `cardsets/README.md`.

## Per-Task Technical Notes

### Task: Pair mark in card data

Requirements carried: Pair-played Stadiums (data side), Assistant knows the pair rule (rule from data, no generated line). Repo: `tcgreplay`. Paths under `internal/cardset/` come from the card-set-import plan. Adapt to the names that landed, and STOP if the format, `Print.Card` or the correction apply is missing.

- `internal/cardset/format.go` (from the import plan, `Print` core) — add `PlayedAsPair bool \`json:"playedAsPair,omitempty"\`` to the language-independent core, after `EnergyType`/`RegulationMark` in the fixed key order. `omitempty` keeps every existing built set byte-identical. Do not bump `Format`.
- `internal/cardset/card.go` (from the import plan, `Print.Card(lang)`) — copy `PlayedAsPair` into `cards.Card.PlayedAsPair`.
- `internal/cardset/corrections.go` (from the import plan, `apply`) — confirm a correction at `/playedAsPair` with value `true` creates the absent member and decodes strictly into `Print`. The plan says the final segment "may create an object member". If it does not, fix it there with a test.
- `internal/cardset/format_test.go`, `internal/cardset/corrections_test.go` — round trip with the mark. `Card` conversion with and without it. A correction setting `/playedAsPair` lands in the built print. Writing a set with no marked print gives the same bytes as before.
- `internal/cards/cards.go:72-93` — add `PlayedAsPair bool \`json:"playedAsPair,omitempty"\`` to `Card`, with a comment: "two different prints of this name are played together as one card".
- `internal/cards/cards.go:102-149` (`Text()`) — no change. Do not append any line from `PlayedAsPair` and do not match text: the pair rule shows only through the card's effect text in the data. The board's `/api/card` `text` field (`internal/webui/server.go:161-225`) is built from `Text()`, so the zoom shows the data text with no further change. Leave both unchanged.
- `internal/cards/cards_test.go` (existing test file for `Text`, or add one) — a marked card's text is byte-identical to the same card without the mark (just its data text), and an unmarked card's text is byte-identical to before.
- `internal/cards/cardstest/cardstest.go:17-29` — add pair-played Stadium fixtures under a made-up name: `tst-10` "Twin Peak" (Stadium, `PlayedAsPair: true`, effect "You cannot play this card by itself. You must combine 2 different Twin Peak from your hand to play as 1 Stadium card. Each player's Pokémon get +10 HP.", so the fixture carries its rule in data as real pair-played prints must), `tst-11` "Twin Peak" (second print, same fields), and a non-pair Stadium already exists (`tst-5` Arena, `tst-8` Deep Zone). Pick unused ids. `internal/webui/lock_test.go:561` uses `tst-9` in its own copy, so avoid `tst-9`. `Resolve("TST", "10")` already maps to `tst-10`. Check that `ByName` (first match by map iteration) is not relied on for these two in any test: tests address them by id.

Complexity: Low. Token estimate: ~12k. Agent strategy: single agent, sequential.

### Task: Stadium spot holds a pair

Requirements carried: Play a Stadium pair (state), A pair moves as one (state and log), Board shows a pair (view shape). Repo: `tcgreplay`.

- `internal/state/state.go:84` — `Stadium []IID \`json:"stadium,omitempty"\``, with doc "at most 2: a Stadium, or the two halves of a pair, first played first". `Clone` (`:104-119`) must clone the slice (`slices.Clone`).
- `internal/state/state.go:246-248` (`Find`) — `if slices.Contains(s.Stadium, iid) && iid != ""`.
- `internal/state/types.go:189-193` — add `AsPair = "pair"` with a comment: "joins the lone card in the Stadium spot as its pair".
- `internal/state/events.go:93-104` (`MovePayload`) — add `Pair IID \`json:"pair,omitempty"\`` with doc: "the other half of a Stadium pair, moved with IID in this event to the same zone (each to its owner's side)". Extend the `As` doc with `pair`.
- `internal/state/apply.go:96-130` (`remove`) — the Stadium case deletes the iid from the slice (`slices.DeleteFunc`). Leave nil when it is empty.
- `internal/state/apply.go:132-260` (`applyMove`):
  - If `p.Pair != ""`, it must exist, differ from `p.IID`, and sit in the same zone as `p.IID` (`Find` equal on seat and zone, ignoring deck position and prize slot), or both must be in the Stadium spot. Validate the destination for two cards: Stadium empty (or holding exactly these two, for a no-op guard: refuse "already in"). Prizes and stacks do not take a pair, so return an error "a pair moves only into or out of the Stadium spot, or between piles" for stack destinations and prize slots. Remove both, place both: Stadium `[IID, Pair]`; Discard, Lost, Hand and Deck each to its own owner's side (`to.Seat` resolves per card owner, so do not apply the `to.Seat != card.Owner` check to the pair card; compute each card's seat). Update the knowledge bits for both with the same rules.
  - `case Stadium` validation (`:180-183`): if `len(s.Stadium) == 0` it is fine. If `p.As == AsPair && len(s.Stadium) == 1 && s.Stadium[0] != p.IID` it is fine. Otherwise keep the refusal "stadium is occupied by …; discard it first", naming the first card. `As` other than `""`/`pair` for a Stadium destination is an error.
  - Placement `:223-224`: `s.Stadium = append(s.Stadium, p.IID)`.
  - Moving one card out of a pair with no `Pair` stays legal at state level (the verbs never do it, but the importer or old paths might). The remaining half stays.
- `internal/state/invariants.go:43` — `add(Loc{Zone: Stadium}, s.Stadium...)`. Add a check problem when `len(s.Stadium) > 2`.
- `internal/state/view.go:60-75,185-188` — add `type StadiumView struct { CardView; Pair *CardView \`json:"pair,omitempty"\` }`. `BoardView.Stadium *StadiumView`. The projection fills `CardView` from `s.Stadium[0]` and `Pair` from `s.Stadium[1]` when present. Embedding keeps `stadium.iid/name/def` in JSON.
- `internal/game/reads.go:34-75` (`describe`, `MovePayload` case) — when `p.Pair != ""` render `"%s moved %s + %s %s → %s"` with both `cardLabel`s (the visibility rule is the same for both, and the pair card's visibility is computed the same way). Append `" as pair"` for `As == pair`, which the existing `p.As != AsPokemon` branch already does.
- `internal/game/ui.go:120-146` (`touched`) — `MovePayload`: return `[IID, Pair]` when `Pair` is set.
- `internal/game/reads.go:498-515` (`benchWarnings`) — read `g.st.Stadium[0]` when non-empty (both halves share a name and text).
- `internal/game/verbs.go:443-450` (`PlayTrainer` Stadium branch) — minimal change to compile and keep today's behaviour: loop over `t.st.Stadium` (cloned), moving each card to its owner's discard. The next task replaces this with pair-aware logic.
- `internal/importer/handlers.go:98-104,503` — compile changes only, keeping behaviour: replace every card in `r.st.Stadium` (loop, `lastStadium` holds the first). The attack-discard check uses the first card. Pair recognition is the importer task.
- `internal/importer/importer_test.go:358-359`, `internal/game/game_test.go:342`, `internal/webui/webtest/webtest_test.go:24-25,88-89` — update for the slice (`len(st.Stadium) != 0`, `slices.Equal(st.Stadium, []state.IID{"p2-18"})`, count `len(st.Stadium)`).
- `web/src/lib/actions.js:49` (`findCard`) — also match `b.stadium?.pair?.iid`. The front end already reads `b.stadium.iid/def`, so the nested shape needs no other change in this task.
- Tests, new in `internal/state/apply_test.go` (or beside `turns_test.go`; the state package has few tests, so add a table-driven file):
  - a pair placed in one event: `Check()` clean, `Find` both returns Stadium;
  - the `pair` role onto exactly one card succeeds, onto empty or two cards is refused;
  - a plain move into an occupied spot is refused;
  - a pair moved out to discard puts each half on its owner's side;
  - a pair move into a stack or prize slot is refused;
  - projection nesting and JSON shape (`stadium.pair.iid`);
  - fold of an event list without the new fields equals today's (covered by the existing corpus tests and `record_roundtrip_test`; run them).
  - `describe` renders one line for a pair move.

Complexity: Medium. Token estimate: ~30k. Agent strategy: single agent, sequential (state first, then the compile fixes in `game`, `importer` and `webtest`, then tests).

### Task: Play, replace and discard Stadium pairs

Requirements carried: Play a Stadium pair, A pair moves as one, Pair warnings, Pair rule comes from data (acceptance criterion). Repo: `tcgreplay`.

- `internal/game/verbs.go:426-457` — keep `PlayTrainer(seat, actor, iid)` as `return g.PlayTrainerWith(seat, actor, iid, "")`, the way `EndTurn`/`EndTurnWith` work (`:217-225`). New `PlayTrainerWith(seat, actor, iid, pair)`:
  - Choose the action spec first, under the lock. If `pair == ""`, the card is a pair-played Stadium (via `g.card`), the spot holds exactly one card with the same name (`cards.SameName`) and owner `seat`, and `g.open` is the play action whose `Source` is that lone half, then **join** it (no new action). Otherwise open a new `trainer` action as today. This needs a joining variant of `run`: add `actionSpec.joinAction string` (join this open action id if it is still `g.open`) or compute it inside `runLocked`. Keep `opener: true` semantics so the open action stays open afterwards.
  - `pair != ""`: both must be in `seat`'s hand. The first card must be a Trainer of type Stadium, otherwise error "only a Stadium is played as a pair". Warn (do not block) when the two are not the same name or the card lacks the mark: "%s and %s are not two halves of one pair-played Stadium". Replace whatever is in the spot (see below), then emit one move `{IID: iid, Pair: pair, To: Stadium}`.
  - Completion (`pair == ""` and the conditions above, whether joined or not): emit `{IID: iid, To: Stadium, As: AsPair}` and do not replace.
  - Otherwise a single Stadium: replace, then move as today.
  - Replace: if the spot holds two cards, emit one move `{IID: st[0], Pair: st[1], To: Discard}` (each lands on its owner's side via the state). If one, move it to its owner's discard as today.
  - The same-name warning, before replacing, for any Stadium: if any card in the spot has the same name as the played card and this is not a completion, warn "a Stadium named %s is already in play".
  - The label is the card's name, as today, so the log shows one entry named for the Stadium.
- `internal/game/verbs.go:725-753` (`Move`) — before `moveStack`:
  - If `iid` is in the Stadium spot with a partner and `to` is not the Stadium, emit one move `{IID: iid, Pair: other, To: to}` with `to.Seat` left for the state to resolve per owner (do not set `to.Seat = c.Owner` for the pair case, or pass the owner and let `applyMove` compute each card's seat; see the state task).
  - If `to` is the Stadium spot, the spot holds exactly one card of the same name and owner, and the moving card has the mark, emit `As: AsPair` (completion through the escape hatch, for the board's non-hand drops).
- `internal/game/reads.go:433-489` (`turnWarnings`) — do not count an action as a Stadium played when its only Stadium move is an `as: "pair"` join. Check the action's events for `"as":"pair"` the way `:461` checks `"as":"energy"`. A joined completion is already in the same action, so it is not counted twice.
- `internal/game/reads.go:498` — new `stadiumWarnings()` next to `benchWarnings`, appended in `turnWarnings` (which feeds `Result.Warnings`, `UISnapshot.Warnings` and `Warnings()`):
  - one card in the spot whose card has `PlayedAsPair`: "%s (%s) is played as a pair: play a second, different %s with it";
  - two cards with equal `Def`: "the Stadium pair %s is two copies of the same print (%s): a pair needs two different prints".
  - `turnWarnings` returns nil outside `PhasePlaying` (`:434`). Put `stadiumWarnings` so it runs in setup too (only possible in review mode), or accept playing-only and document it. Prefer appending outside the phase guard, like `g.st.Check()` in `UISnapshot`.
- `internal/game/game_test.go` (Stadium replacement test near `:330-345`, as a model) — new `internal/game/stadium_pair_test.go` using `cardstest.New()` with `tst-10`/`tst-11` "Twin Peak". Cases:
  - pair played together: spot `[a,b]`, one action, one move event;
  - lone half then completion, joined while open: undo once returns both to the hand;
  - completion after `EndAction`: own action, no "played 2 Stadiums" warning;
  - Arena over a pair: both in p1's discard in one event, undo returns both to the spot, the `Log` has one line for the discard;
  - `Move` of either half to the discard: both move, one action, undo returns both;
  - warnings for each of the three spec cases, with the action applied: lone half (standing, present in `Warnings()`, gone after completion); identical print (`tst-10` with a second `tst-10` copy, standing); a Twin Peak pair over a Twin Peak pair (same-name warning);
  - data, not names: `db := cardstest.New(); db["tst-10"].PlayedAsPair = false; db["tst-11"].PlayedAsPair = false` makes a second single play replace the first, with no lone-half warning.
  - Deck building: use the existing helpers that build a game from `cardstest` (see `applied_test.go` setup helpers) and put both Twin Peak prints in the hand with `Move`.
- `internal/webui/actions.go:348-349`, `internal/mcpserver/server.go:651-655` — unchanged in this task (they keep calling `PlayTrainer`). The next tasks switch them to `PlayTrainerWith`.

Complexity: High. Token estimate: ~40k. Agent strategy: parallel analysis (verbs and warnings), then sequential integration: verbs first, then warnings, then tests.

### Task: Assistant plays Stadium pairs

Requirements carried: Assistant knows the pair rule, Play a Stadium pair (assistant). Repo: `tcgreplay`.

- `internal/mcpserver/server.go:345-348` — add `PlayTrainerArg{SeatArg; IID string \`json:"iid"\`; Pair string \`json:"pair_iid,omitempty" jsonschema:"For a Stadium played as a pair: the second, different print to play together with iid"\`}`. Leave `IIDArg` for other tools.
- `internal/mcpserver/server.go:651-655` — `play_trainer` takes `PlayTrainerArg` and calls `g.PlayTrainerWith(seat, actor, iid(in.IID), iid(in.Pair))`. Description: append "For a Stadium played as a pair (its card text says so), pass the second print as pair_iid; playing one half onto a lone half completes the pair." Check the hold/lock path for the opponent seat (`OpenerNamesOpponent`) is unaffected.
- `internal/mcpserver/server.go:67-77` (`instructions`) — one line after the compound-action line (`:72`): "- A Stadium whose card_text says it is played as a pair goes down with play_trainer naming both halves (iid and pair_iid); the playmat keeps the two as one Stadium." Keep it short. OpenAI guarantees only the first 512 characters of the instructions (knowledge `gotchas/chatgpt-tool-discovery.md`), and the tool description carries the same hint. `TestInstructionsTeachTheLoop` (`internal/mcpserver/wait_test.go:386-404`) needs exactly one "your turn": do not add another.
- `internal/mcpserver/server.go:583-601` (`card_text`) — no code change: `c.Text()` is unchanged, and the pair rule reaches the assistant through the card's effect text in the data. It reads through the publishing plan's `Manager.CardDB()` (`Manager.DB` is removed there).
- `internal/guides/play-game.md` (section `## The player's turns`, next to the "Costs count" bullet, and the playing-your-turns section) — a bullet: a Stadium played as a pair is played with `play_trainer` giving both halves (`pair_iid`); a lone half shows a warning until its partner joins; a Stadium played over a pair takes both halves to the discard. Follow `conventions/skills-text-and-packaging.md` (no "Claude", no "Nic "), and `TestGuidesNameRealTools` (`pair_iid` is now a schema property). Keep it to the turn-routine guide only.
- `internal/mcpserver/server_test.go` — a round-trip test in the style of the existing verb tests:
  - with a game built from `cardstest` holding both Twin Peak prints, `play_trainer {iid, pair_iid}` succeeds, `get_state` shows `stadium.pair`, and `get_log` shows one move line;
  - `card_text` on a Twin Peak iid returns its data text, including the fixture's rule sentence ("combine 2 different Twin Peak"), with no generated line;
  - `play_trainer` without `pair_iid` on an Item behaves as before.

Complexity: Low. Token estimate: ~15k. Agent strategy: single agent, sequential.

### Task: Board shows and plays Stadium pairs

Requirements carried: Board shows a pair, Play a Stadium pair (board), Pair warnings (board). Repo: `tcgreplay`.

- `internal/webui/actions.go:21-53` (`args`) — add `Pair string \`json:"pair_iid,omitempty"\``.
- `internal/webui/actions.go:200-205` — include `a.Pair` in the iids checked for the held/decider lock.
- `internal/webui/actions.go:348-349` — `g.PlayTrainerWith(seat, human, state.IID(a.IID), state.IID(a.Pair))`.
- `web/src/components/Strip.svelte:29,41-47` — render `b.stadium` and, when `b.stadium.pair` is set, a second `<Card card={b.stadium.pair} from="stadium" …>`, both inside the `.stadium` region (the drop zone stays the region). Each card keeps its own click-to-zoom (Card.svelte:41-46). `theirs` still uses the first half's owner.
- `web/src/components/Strip.svelte:103-118` (desktop/tablet) — when the spot holds a pair (`class:pair`), fan the halves: the second card offset by about 0.3 of a card width and a few degrees of rotation, or side by side at a slightly smaller size. Widen `.stadium` for `.pair` so neither overlaps the Active spot or the turn chip. Keep hover enlargement behind `body:not(.dragging)` (knowledge `gotchas/playwright-touch-tests.md`).
- `web/src/components/Strip.svelte:191-192` (phone) — at `sm` size show the two halves side by side (`display:flex; gap:2px`). Keep `.theirs` positioning. Check that no sideways scroll appears at 390px.
- `web/src/app.css:69` — the hover z-index rule already covers `.stadium`. Confirm it still lifts the hovered half.
- `web/src/lib/actions.js:105-110` (`dropOnZone`, `stadium`) — no change needed: a hand Stadium dropped on the spot already sends `play_trainer`, and the verb completes a lone half. A non-hand drop sends `move`, which the verb also completes. Leave a short comment saying so.
- `internal/webui/webtest/webtest.go:23-146` — add the Twin Peak prints to the board deck (`Deck`) from `cardstest` `tst-10`/`tst-11` (p1 iids at the end of the list, adjusting the 60-card count), with both in p1's hand in `Position`. Keep `Stadium = "p1-11"` Arena as the default Stadium so existing e2e tests are untouched. Update `webtest_test.go` counts.
- `web/e2e/drag.spec.js:37-138` — new cases (all three projects):
  - drag Twin Peak A from the hand onto the Stadium region: the Arena-replaced lines and a toast or header warning containing "played as a pair";
  - then drag Twin Peak B onto the region: the timeline line "… as pair", both `.card[title="Twin Peak (…)"]` inside the region, and the header warning count no longer including the lone-half warning (desktop `title`, phone ☰ list; see `phone.spec.js:131`).
- `web/e2e/phone.spec.js:101-105` (seeding pattern) — a phone-only test seeding a pair with `act(page,'play_trainer',{iid, pair_iid})`, then checking both halves are in the viewport (`toBeInViewport`) and not overlapping the Active card.
- `web/e2e/smoke.spec.js:45` — unchanged (Arena).
- Run `make web` before Playwright (the board is embedded). Only Chromium is installed.

Complexity: Medium. Token estimate: ~30k. Agent strategy: 2 parallel agents (Go action and test seed; Svelte rendering), then sequential e2e tests.

### Task: Mark Delta Reign's Legendary Stadiums

Requirements carried: Delta Reign's Legendary Stadiums marked, Pair rule comes from data. Repo: `tcgreplay`.

- `cardsets/corrections/me06.json` (from the import plan) — for each Legendary Pyroduct, Legendary Summit and Legendary Trench print (two each; find their ids in `cardsets/sets/me06.json` by English name and `trainerType: Stadium`), add `{"card": "<id>", "path": "/playedAsPair", "value": true, "reason": "Legendary Stadium: the printed rule plays 2 different prints together as 1 Stadium"}`. Keep the existing `/text/en/effect` rule-text corrections. If any of the six lacks the rule text, add it the same way, quoting knowledge `gotchas/new-set-tcgdex-data-incomplete.md` with the card's own name.
- `cardsets/sets/me06.json`, `cardsets/reports/me06.json` — regenerate with `tcgreplay sets import me06` (never hand-edit). The diff must show only `"playedAsPair": true` on the six prints, and the report must have nothing open.
- `internal/cardset/committed_test.go` (from the import plan, `TestCommittedSets`, Delta Reign block) — assert that every `me06` print whose English name starts "Legendary " and whose `TrainerType` is `Stadium` has `PlayedAsPair` and English effect text containing "You cannot play this card by itself", that exactly six such prints exist, and that no other `me06` print has the mark. Card names in a data test do not break the constraint, which binds program code.
- Run `tcgreplay sets check` and the full Go suite.
- Publishing is automatic on merge (publishing plan's workflow). The hosted check is a manual test-plan item.

Complexity: Low. Token estimate: ~10k. Agent strategy: single agent, sequential.

### Task: Supply a real Stadium pair battle log

What the person does:
- Play, or find among their TCG Live history, a game in which a player plays a Legendary Stadium pair (any of Pyroduct, Summit or Trench). Ideally it also includes a Stadium played over a pair, or a pair discarded.
- Export the battle log as text from TCG Live, and note which seat the owner was and the owner's decklist (TCG Live export).
- Hand both to the implementer, for example pasted into the session or saved under `testdata/logs/` with a dated name following the corpus convention (`2026-MM-DD_<opponent>_<deck>.txt`), with the deck under `testdata/decks/`.

How the result is checked: the log contains a line in which a Legendary Stadium is played, and the implementer can see the line shape. If no such game exists when the importer task is reached, the person says so, and that answer is what the importer task records as the deferral.

### Task: Import battle logs with a Stadium pair

Requirements carried: Import logs with a pair. Repo: `tcgreplay`.

- First, read the real log's pair lines and the replacement or discard lines that follow (with `TRACE=<log> go test -run Trace ./internal/importer`, `internal/importer/trace_test.go`). If no real log was supplied, STOP and ask the user to supply one or confirm deferral. On deferral, write no importer change beyond the state task's compile fixes. Record the deferral in the implementation's changelog entry and test plan as "Import logs with a pair: deferred, no real TCG Live log available", and propose a follow-up spec in the closing summary.
- `internal/importer/handlers.go:92-106` — recognise a pair from the log alone (knowledge `architecture/importer-determinism.md`: no card-data lookup decides placement). Depending on the real shape:
  - one line naming both halves: split the names, `take` two distinct hand cards (pass `exclude` as `zoneTo` does, `handlers.go:738-760`), replace the spot, and emit one move `{IID: a, Pair: b, To: Stadium}`;
  - two consecutive play lines of the same name by the same player, the second while the spot holds only the first and that card is that player's: emit `As: AsPair` for the second instead of replacing. A same-name Stadium never legally replaces itself, so this is log-shape inference, not card data.
  Keep the existing single-Stadium path unchanged for every other line. Card codes: `codeFor` gives no code when one line names two prints (`parse.go:78-90`; knowledge `gotchas/tcg-live-log-card-codes.md`). That is expected, and later lines fill the codes in.
- `internal/importer/handlers.go:98-104`, `internal/importer/replay.go:117`, `internal/importer/importer.go:493` — `lastStadium` becomes `[]state.IID` (the replaced cards). Replacing a pair emits one move `{IID: old[0], Pair: old[1], To: owner discard}`.
- `internal/importer/handlers.go:494-499` — consume one "discarded <name>" subline per replaced card (remove it from the list), or a "discarded 2 cards" bullet form if the real log uses one.
- `internal/importer/handlers.go:500-505` — an attack discarding a pair: move both in one event.
- `testdata/logs/<real log>.txt`, `testdata/decks/…`, `testdata/index.json` — add the log and its oracle entry (`expected`, `features` including a `stadium-pair` feature tag if the index uses features, `observed`), following the existing entries.
- `internal/importer/importer_test.go:61-139,222` — `TestCorpus` and `TestCorpusSeeds` cover the new log (no problems, `Check()` clean). Add a focused test modelled on `TestAttackDiscardsStadium` (`:343-371`): after the pair line, `st.Stadium` holds both halves; after a replacement, both are in the owner's discard.
- If the real log's shape matches neither form above, implement the shape it shows under the same rule (log text only). If it needs card data to decide, STOP and ask the user, because that would contradict the importer-determinism knowledge entry.

Complexity: Medium. Token estimate: ~25k. Agent strategy: single agent, sequential.

### Task: Pair play documentation

Requirements carried: Documentation for pair play. Repo: `tcgreplay`.

- `site/src/content/docs/workflows/play-game.md` — a short paragraph for players: a pair-played Stadium (for example Delta Reign's Legendary Stadiums) is played by dragging one half onto the Stadium spot and then the second, different print onto it. A warning shows until the pair is complete. A Stadium played over a pair takes both halves to the discard, and undo returns both.
- `TCG Review Playmat Design and Build Plan.md:34-60` (`## Event schema`, zones table row `stadium` at `:54`) — Notes: "up to two cards: a Stadium, or the two halves of a pair". Document the move event's `pair` field and `as: "pair"` role.
- `TCG Review Playmat Design and Build Plan.md:109-140` (`## MCP tool contract`, Verbs row `:124`) — `play_trainer(iid, pair_iid?)`, and a sentence on completing a lone half.
- `TCG Review Playmat Design and Build Plan.md:146-186` (`## Playmat UI`) — the Stadium spot shows a pair as one Stadium with both halves, and dragging the second half completes it.
- `cardsets/README.md` (from the import plan) — in the format section, the `playedAsPair` field. In the corrections section, an example correction setting `/playedAsPair` for a Legendary Stadium print, and a note that a print marked `playedAsPair` must also carry its printed pair rule in its effect text, recorded as a separate `/text/en/effect` correction if the sources lack it (the program never generates or checks that text).
- `README.md` — no change (no gameplay detail there).
- The guide and server-instruction text is in the assistant task. Check here only that the docs and the guide agree.

Complexity: Low. Token estimate: ~8k. Agent strategy: single agent, sequential.

## Testing Strategy

Per task:
- **Pair mark in card data**: `cardset` format and correction tests (the mark round-trips, `/playedAsPair` correction lands, unmarked sets keep their bytes). `cards.Card.Text` test (the mark adds nothing to the text).
- **Stadium spot holds a pair**: a new table-driven state test (pair placement, the `pair` role, refusal of a plain move into an occupied spot, a pair moved out lands each half on its owner's side, a pair cannot enter a stack or prize slot, view nesting and JSON shape). `describe` renders one line. The existing corpus, record round-trip and board tests prove old records replay unchanged.
- **Play, replace and discard Stadium pairs**: `internal/game/stadium_pair_test.go` with the `cardstest` Twin Peak prints. Play together, complete a lone half (joined and separate), replace a pair, discard a pair through `Move`, undo for each, one log line per pair move, each of the three warnings with the action applied, standing warnings appear and clear, no "played 2 Stadiums" on completion, and removing the mark in a fresh pool makes an ordinary Stadium.
- **Assistant plays Stadium pairs**: MCP round trip (`play_trainer` with `pair_iid`, `get_state` shows `stadium.pair`, `card_text` returns the fixture's data text with its rule sentence and no generated line, a single Item is unchanged). The existing guide tests police the new guide text.
- **Board shows and plays Stadium pairs**: Playwright in desktop, tablet and phone projects. Drag the first half (lone-half warning), drag the second (pair complete, warning gone, both cards in the region). A phone-only seeded pair is in the viewport and not overlapping the Active.
- **Mark Delta Reign's Legendary Stadiums**: the committed-data guard asserts six marked Legendary Stadium prints with the rule text and no other marked `me06` print. `sets check` passes.
- **Import battle logs with a Stadium pair**: the real log joins `TestCorpus`/`TestCorpusSeeds`, plus a focused test after `TestAttackDiscardsStadium`. Without a real log there are no importer tests, and the deferral is recorded.
- **Pair play documentation**: no automated test. Reviewed by following it (manual test-plan item).

Success metric (real game, by hand and by the assistant, no workarounds): Manual — captured in the implementation test plan. Manual reviews (hosted Delta Reign check, real-log import or deferral, real-phone look, docs walk-through, ChatGPT tool refresh) are listed in plan.md § Testing Approach.

## Project References

- Spec: `20261108110847-055548e3-paired-stadiums` (`spektacular spec file read …`).
- Plans built on: `20261108110847-d27a9f77-card-set-import` (built format, `Print.Card`, corrections, committed-data guard, Delta Reign build) and `20261108105504-9abfa10e-card-set-publishing` (loader, `CardSource`, `CardDB`, publish on merge).
- Epic: `20261108105504-9abfa10e-card-set-publishing`.
- Design documents: none.
- Knowledge: `gotchas/new-set-tcgdex-data-incomplete.md`, `architecture/importer-determinism.md`, `architecture/games-read-cards-through-their-db.md`, `gotchas/playwright-touch-tests.md`, `gotchas/tcg-live-log-card-codes.md`, `gotchas/chatgpt-tool-discovery.md`, `conventions/skills-text-and-packaging.md`.
- Repo root: `tcgreplay` → `/home/nicj/code/github.com/nicholasjackson/tcgreplay`.

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

Per task: Pair mark (Low, ~12k), Stadium spot (Medium, ~30k), Play/replace/discard (High, ~40k), Assistant (Low, ~15k), Board (Medium, ~30k), Delta Reign marks (Low, ~10k), Importer (Medium, ~25k), Docs (Low, ~8k). The state task touches many compile sites. Keep it to one agent so the slice change lands consistently.

## Migration Notes

- No stored-data migration. `State` is never persisted: records and saved games hold events, and the new move-event parts are optional, so every existing log, record and saved game folds unchanged.
- Built sets: `playedAsPair` is optional with `omitempty`, and `format` stays `1`. Only Delta Reign's six Legendary Stadium prints change.
- Rollout: a playmat built before this change decodes built sets strictly and cannot read the rebuilt `me06`. Per the publishing plan, it keeps its current copy of the set until it is updated (see Open Questions). Hosted redeploys on the same merge.
- ChatGPT players must refresh the app's tools to see `pair_iid` (knowledge `gotchas/chatgpt-tool-discovery.md`).

## Performance Considerations

Negligible. A Stadium spot of at most two cards adds constant work to `Find`, `Locations` and the projection. The standing pair warnings read at most two cards per action. The board renders one extra card.
