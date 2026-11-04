---
created_date: "2026-11-04"
document_status: final
closed_date: "2026-11-04"
epic: 20261104082020-109dceb1-tcgdex-spaces-cache
---

# Feature: 20261104090640-51bd1f36-card-cache-freshness

<!--
  OVERVIEW
  A concise 2-3 sentence summary of the feature. Answer three questions:
    1. What is being built?
    2. What problem does it solve?
    3. Who benefits and why does it matter?
  Avoid implementation details — this should be readable by any stakeholder.
-->
## Overview

The playmat's card database refreshes its cached card data on its own, at intervals suited to how often each kind changes. New sets, new promos and newly added artwork appear without anyone clearing the cache, and the last good copy is kept when the card source is down. Card lookups never freeze the board while data loads, and failed lookups are not retried over and over. Desktop and hosted players both benefit, and offline use works as before.

<!--
  REQUIREMENTS
  Specific, testable behaviours the feature must deliver.
  Format: bold title on the checkbox line, detail indented below.
  Rules:
    - Use active voice: "Users can...", "The system must..."
    - Each requirement should be independently verifiable
    - Focus on WHAT, not HOW — avoid prescribing implementation
    - Keep each item atomic — one behaviour per line
-->
## Requirements

- [ ] **Set list refreshes**
  The system must treat the cached list of sets as stale no later than 30 days after it was fetched.
- [ ] **Set card lists refresh**
  The system must treat each set's cached card list as stale no later than 90 days after it was fetched. A set barely changes once released; additions such as new promos are picked up by "Misses checked against fresh data".
- [ ] **Imageless cards refresh**
  The system must treat a card cached without artwork as stale no later than 24 hours after it was fetched.
- [ ] **Cards with art are kept**
  The system must keep a card cached with its artwork without refetching it on a schedule.
- [ ] **Stale data served while refreshing**
  A lookup that finds what it asked for in stale data must get the stale copy at once, and the system must then refresh that entry. Once the refresh finishes, later lookups must see the refreshed data.
- [ ] **Misses checked against fresh data**
  A lookup that does not find what it asked for in cached data (an unknown set code, or a card number missing from a set) must refresh that data before answering, unless the same miss already triggered a refresh in the last 5 minutes. A set released or a promo added since the data was fetched then resolves on the first try.
- [ ] **Stale data kept when the source fails**
  When a stale entry cannot be refreshed because the card source errors or takes longer than 5 seconds to answer, the system must keep serving the stale copy rather than failing.
- [ ] **Name lookup follows set changes**
  When a refresh changes the set list or a set's card list, by-name card lookups must reflect the change without a restart.
- [ ] **Existing caches carried over**
  Local caches written before this change must stay usable after the upgrade. An entry with no recorded fetch time takes its file's modification time as its fetch time, so it is refreshed only once that age passes its refresh age. Offline mode must keep serving these entries.
- [ ] **Board stays responsive while card data loads**
  Board reads, plays and lookups of cards that are already cached must not be delayed while the set list and set card lists are being fetched, or while other cards are being fetched.
- [ ] **One request for the same data**
  Simultaneous requests needing the same card data must result in only one request to the card source for that data.
- [ ] **Failed lookups remembered briefly**
  After the card source fails to return a card (it reports the card as unknown, errors, or does not answer), the system must not ask it for that card again for at least 5 minutes. Repeated reads of that card in the meantime must return at once with no card data, so the card shows by name only.
- [ ] **Offline mode unchanged**
  With offline mode on, the system must use only data already cached locally and never contact the card source, regardless of how old the data is.

<!--
  CONSTRAINTS
  Hard boundaries the solution must operate within. These are non-negotiable.
  Format: one bullet point per constraint.
  Examples:
    - Must integrate with the existing authentication system
    - Cannot introduce breaking changes to the public API
    - Must support the current minimum supported runtime versions
  Leave blank if there are no constraints.
-->
## Constraints

- Card lookups must keep the importer deterministic. Cache contents and timing may change only which card definition a card gets, never where a card is placed, so a game record still rebuilds the same game.

<!--
  ACCEPTANCE CRITERIA
  The specific, binary conditions that define "done".
  Format: bold title on the checkbox line, verifiable detail indented below.
  Each criterion must be:
    - Independently verifiable (pass/fail, not subjective)
    - Traceable back to a requirement above
    - Testable by someone who didn't write the code
-->
## Acceptance Criteria

- [ ] **New set resolves on first try**
  With a set list cached more than 30 days ago that lacks a set the card source now lists, a deck naming that set's code resolves on the first import, without anyone clearing the cache.
- [ ] **New promo resolves on first try**
  With a set's card list cached less than 90 days ago that lacks a card the card source now has in that set, a deck naming that card by set code and number resolves on the first import, without anyone clearing the cache.
- [ ] **New set resolves against a recent set list**
  With a set list cached less than 30 days ago that lacks a set the card source now lists, a deck naming that set's code resolves on the first import.
- [ ] **Repeated miss not refetched**
  Looking up the same unknown set code twice within 5 minutes makes exactly one request to the card source for the set list.
- [ ] **Late art appears**
  A card cached without artwork more than 24 hours ago, which the card source now shows with artwork, appears on the board with its artwork on a view after the first lookup's refresh has finished, without a restart or a cache clear.
- [ ] **Art kept**
  Looking up a card cached with artwork, however long ago, makes no request to the card source.
- [ ] **Stale served when source down**
  With the card source unreachable, or answering only after more than 5 seconds, and every cached entry past its refresh age, the playmat still imports a game, resolves deck codes and shows card text and art from the stale copies, with no error shown to the player.
- [ ] **Name lookup refreshed in place**
  After a refresh adds a new set to the set list while the playmat is running, a card known only by name (such as an opponent's card) resolves to that set's print without a restart.
- [ ] **Old cache still works**
  A local cache written by the previous release, opened by the new one in offline mode, serves every card it holds. Opened online, it makes no requests to the card source for entries whose file age is below their refresh age.
- [ ] **Set data kept between refreshes**
  Looking up cards in a set whose card list was cached less than 90 days ago, with a set list cached less than 30 days ago, makes no request to the card source for either.
- [ ] **No stall during a cold build**
  Starting with empty caches, while a first import is still fetching the set list and set card lists, a board read or card text request for a card the import has already cached returns within one second.
- [ ] **One request for the same card**
  Several simultaneous requests that need the same uncached card produce exactly one request to the card source for that card.
- [ ] **Failed lookups not retried at once**
  Repeatedly reading a board that holds a card whose lookup fails (it is unknown to the card source, or the source is unreachable) makes at most one request for that card to the card source in 5 minutes. Each repeated read returns within one second.
- [ ] **Offline untouched**
  In offline mode, with only stale local data cached, the playmat serves that data and makes no network requests to the card source.

<!--
  TECHNICAL APPROACH
  High-level technical direction to guide the planning agent. Include:
    - Key architectural decisions already made
    - Preferred patterns or technologies if known
    - Integration points with existing systems
    - Known risks or areas of uncertainty
  Format: one bullet point per direction/steer.
  Leave blank if you want the planner to propose the approach.
-->
## Technical Approach

- For expiry, prefer stale-while-revalidate with stale-if-error.
- Record when each entry was fetched, so its age survives restarts and later copying between cache tiers.
- Fetch from the network without holding the lock that guards in-memory lookups, and prefer keeping network calls outside the board's game lock, with a single-flight per key for fetches and index builds.

<!--
  SUCCESS METRICS
  How you will know the feature is working well after delivery. Be specific:
    - Quantitative: "p99 latency < 200ms", "error rate < 0.1%"
    - Behavioural: "users complete the flow without support intervention"
  Format: one bullet point per metric.
  Leave blank if not applicable.
-->
## Success Metrics

- Over a week of hosted use, 95% of board reads complete within 200 ms, and none waits on a card cache fetch for more than 1 second.

<!--
  NON-GOALS
  Explicitly state what this spec does NOT cover. This is as important as
  the requirements — it prevents scope creep and sets clear expectations.
  Format: one bullet point per exclusion.
  Examples:
    - "Mobile support is out of scope (tracked in #456)"
    - "Internationalisation will be addressed in a follow-up spec"
  Leave blank if there are no explicit exclusions to call out.
-->
## Non-Goals

- Changing how cards are matched or which print is preferred is out of scope. Only how cached data refreshes changes.
