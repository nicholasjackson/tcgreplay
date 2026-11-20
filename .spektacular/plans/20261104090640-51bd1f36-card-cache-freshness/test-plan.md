---
created_date: "2026-11-04"
document_status: final
closed_date: "2026-11-04"
---

# Test plan: 20261104090640-51bd1f36-card-cache-freshness

Everything else in the spec is covered by automated Go tests in `internal/cards` (`store_test.go`, `store_client_test.go`, `flight_test.go`, `catalog_test.go`, `fresh_test.go`, `setfresh_test.go`) and `internal/game` (`cardlock_test.go`, `cardsource_test.go`), run with `go test -race ./internal/cards ./internal/game`. The three items below need a person, real data or real traffic.

## 1. Success metric: board reads stay fast on the hosted playmat

**What to measure.** Over a week of hosted use, 95% of board reads complete within 200 ms, and none waits on a card cache fetch for more than 1 second.

**How.** The server has no built-in latency metric, so use two sources:
1. The hosting platform's request log for the hosted app (`tcgreplay -hosted`): take the duration of every `GET /api/card` request (the board's card endpoint, `internal/webui/server.go`) for the week.
2. A synthetic probe: from a signed-in MCP client, call the `get_state` tool against an open game once a minute for the week and record each call's round-trip time. `get_state` is a board read through `Game.View`, the path this spec made independent of card fetches.
Deploy on a fresh instance (empty cache) at the start of the week, so the first imports exercise a cold catalog build, and import at least one game whose deck names a set released in the last month.

**Expected result.** p95 of probe round trips ≤ 200 ms and max ≤ 1 s, excluding samples during a platform outage. `/api/card` requests: p95 ≤ 200 ms; any request over 1 s must be the first fetch of an uncached card (bounded by the 5 s source timeout), never a repeat for the same card within 5 minutes.

**Who / when.** The maintainer, during the first week after the release reaches the hosted instance.

## 2. Manual review: upgrade with a real previous-release cache

**What to check.** A cache written by the previous release keeps working after the upgrade, offline and online, and young entries cause no requests.

**How.**
1. Copy a real previous-release cache: `cp -a ~/.cache/tcgreplay /tmp/tcgreplay-old-cache` (it holds bare files such as `sets.json`, `sets/<id>.json`, `cards/<id>.json`, and maybe `set-index-v2.json`). Keep one untouched copy for comparison.
2. Offline: run `tcgreplay -cache /tmp/tcgreplay-old-cache -offline` and, through the playmat or MCP, import a recent battle log and start a game from a decklist with set codes. Open the board and the `card_text` tool for several cards.
3. Online, with network access to TCGdex: run `tcgreplay -cache /tmp/tcgreplay-old-cache` with outbound traffic logged (e.g. `strace -f -e trace=connect` or a proxy via `HTTPS_PROXY`), and repeat step 2.
4. Inspect the cache afterwards: `head -c 40 /tmp/tcgreplay-old-cache/cards/<id>.json` shows `{"fetched":` only for entries that were refreshed.

**Expected result.** Offline: every card the old cache holds shows with its text and art, deck codes and names resolve as before, and no connection to `api.tcgdex.net` is made. Online: no request for any card file younger than 24 hours, no set list request if `sets.json` is younger than 30 days, no set request for set files younger than 90 days; older imageless cards and older set data are refetched in the background and rewritten as envelopes. `set-index-v2.json` is left untouched and unused.

**Who / when.** The maintainer, before tagging the release.

## 3. Manual review: live TCGdex freshness

**What to check.** Against the real TCGdex API, an aged set list does not stop a recent set resolving, and late artwork reaches the board.

**How.**
1. In a scratch cache (`-cache /tmp/tcgreplay-live`), run the app once online and resolve any deck so the set list and sets are cached.
2. Age the set list: rewrite `sets.json`'s envelope `fetched` to 40 days ago (or, for a bare file, `touch -d '40 days ago' sets.json`), and remove the newest set's entry from its body so the cached list lacks it.
3. Restart and import a deck naming a card from that newest set by set code and number.
4. Pick a card TCGdex now shows with artwork, write its `cards/<id>.json` without its `image` field and `fetched` 2 days ago, restart, open a game holding it on the board, then reload the board once a few seconds have passed.

**Expected result.** Step 3 resolves on the first import with no warning for that card. Step 4: the first board load shows the card's text stand-in, the reload shows its artwork, and `cards/<id>.json` now holds an envelope with a current `fetched` and an `image`.

**Who / when.** The maintainer, before tagging the release, with network access to `api.tcgdex.net`.
