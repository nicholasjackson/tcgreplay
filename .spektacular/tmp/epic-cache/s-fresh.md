### Approach
`internal/cards` is split into three layers:
- **Store:** an exported `Store` interface (Get/Put of an `Entry{Data, Fetched}`) with a disk implementation. New files hold an envelope recording the fetch time; older files use their modification time.
- **Freshness, in `cards.Client`:**
  - Refresh ages: the set list 30 days, a set's card list 90 days, a card without art 24 hours, a card with art never.
  - Stale data is served at once and refreshed in the background, and the stale copy is kept if the refresh fails.
  - A miss (unknown set code, or a card number missing from a set) refreshes before answering, at most once every 5 minutes. Failed cards are remembered for 5 minutes.
  - Every TCGdex request has a 5 s limit, and simultaneous requests for the same data share one.
- **Catalog:** an in-memory snapshot of the set list, each set's cards and the two indexes, built outside the database lock and replaced whole.

`Game.card` no longer reaches the network while holding the game lock.
### Milestones and tasks
- M1, entries record their fetch time: add the store; route lookups through it.
- M2, the board never waits: share duplicate fetches and remember failures; build the catalog outside the lock; keep card fetches out of the game lock.
- M3, data refreshes on its own: background refresh of cards; set-data refresh and the miss rule; a game-level test with the card source down; update the knowledge entries.
### Tasks a person must do
None.
### Out of scope
- Card matching and print preference.
- The Spaces tier, image copies and admin clearing (later specs).
- Pushing refreshed data to boards already open.
- Remembering misses and failed cards across restarts.
- Configurable refresh ages.
### Drafting assumptions
- Fetch time is stored in a JSON envelope, with modification time as the fallback for old files.
- Downgrading after the upgrade is unsupported.
- `set-index-v2.json` is retired.
- Every TCGdex request has a 5 s limit.
- Duplicate requests are shared by an in-house helper.
- `Game.card` reads only the cached copy, starting a fetch in the background on a miss.
### Project-wide rules
- Decides the `Store` contract, the keys `sets`, `sets/<id>` and `cards/<id>`, and that `Fetched` is kept when entries are copied.
- Decides the on-disk envelope `{"fetched","data"}`.
- Freshness lives above the store; offline mode never contacts any source.
- No persisted set index.
- The `DB` interface is unchanged.
- Nothing under the game lock calls the network.
### Manual checks
- The 95%/200 ms board-read metric over a week of hosted use.
- An upgrade check against a real cache from the previous release.
- Live TCGdex: a new set resolves on the first import, and late art appears after a reload.
