---
created_date: "2026-11-04"
document_status: final
closed_date: "2026-11-04"
epic: 20261104082020-109dceb1-tcgdex-spaces-cache
---

# Feature: 20261104082020-109dceb1-tcgdex-spaces-cache

<!--
  OVERVIEW
  A concise 2-3 sentence summary of the feature. Answer three questions:
    1. What is being built?
    2. What problem does it solve?
    3. Who benefits and why does it matter?
  Avoid implementation details — this should be readable by any stakeholder.
-->
## Overview

The hosted playmat keeps its cached card data in a shared store that lasts across restarts and deploys, instead of losing it each time and fetching everything again from the card source. Imports right after a deploy are as fast as any other, and a shared store outage never stops play. Desktop and dev builds keep their local cache and need no new settings.

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

- [x] **Cache survives restarts**
  When the shared store is configured, the hosted playmat must find card data it fetched before a restart or redeploy still cached afterwards, and must not refetch it from the card source while it is still fresh.
- [x] **Shared store is optional**
  Without the shared store configured, the system must cache on local disk only, as today, and need no new settings or credentials. This covers desktop and dev builds.
- [x] **Shared store fills the local cache**
  Card data found only in the shared store must be served without contacting the card source, and then saved to the local disk cache.
- [x] **Fetched data is shared**
  Card data the system fetches from the card source must be saved to local disk and, when configured, to the shared store.
- [x] **Fetch age kept across copies**
  Card data restored from the shared store must keep the age it had when first fetched from the card source, so a copy or a restart never makes stale data look fresh.
- [x] **Shared store failures do not stop play**
  If the shared store cannot be reached or rejects a request, the system must keep working using its local cache and the card source. It must log the failure, and must not show players an error because of it.
- [x] **Offline mode never contacts the shared store**
  With offline mode on, the system must never contact the shared store.

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

- The shared store must be DigitalOcean Spaces.
- It must run on the existing DigitalOcean App Platform deployment: one instance with no persistent volume, where local disk is wiped on every deploy or restart. Settings and credentials must arrive as App Platform environment variables, with the credentials as encrypted secrets.
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

- [x] **Warm restart**
  With the shared store configured, import a game, restart the playmat with an empty local disk cache, then import the same game again. The card source receives no requests for the set list, set card lists or cards involved, and the import gives the same result.
- [x] **No shared store, no change**
  Started without shared store settings, the playmat starts, imports a game and shows card text and art. It writes cached data only to the local cache directory, and its logs show no error about missing shared store settings.
- [x] **Shared store fills local**
  With the shared store configured and the local cache empty, looking up a card that is only in the shared store returns it without a request to the card source, and the card then exists in the local cache directory. A card in neither is fetched from the card source once, and afterwards exists both in the local cache directory and in the shared store.
- [x] **Shared store outage**
  With the shared store configured but unreachable or refusing its credentials, the playmat starts, imports a game and shows cards, fetching from the card source as needed. Its log records the shared store failure, and players see no error.
- [x] **Stale stays stale after a restart**
  An entry in the shared store that is past its refresh age is still treated as stale after a restart with an empty local cache: the next lookup refreshes it from the card source.
- [x] **Offline skips the shared store**
  In offline mode with the shared store configured, the playmat makes no requests to the shared store.

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

- Add the shared store as one more tier behind the card database's existing caching: memory, then local disk, then the shared store, then the card source. On the hosted instance, local disk acts as a per-run scratch copy.
- Use Spaces through its S3-compatible API.
- Store each entry's fetch time with it, since a file's modification time alone does not survive copying between local disk and the shared store.
- Expect this risk: The first deploy starts with an empty shared store, so the first cold build still fetches every set from the card source once.

<!--
  SUCCESS METRICS
  How you will know the feature is working well after delivery. Be specific:
    - Quantitative: "p99 latency < 200ms", "error rate < 0.1%"
    - Behavioural: "users complete the flow without support intervention"
  Format: one bullet point per metric.
  Leave blank if not applicable.
-->
## Success Metrics

- After any redeploy other than the first, which starts with an empty shared store, the first import makes zero requests to the card source for card data the shared store holds that is still fresh.
- After any restart other than the first deploy, the first import on the hosted playmat finishes within 2 seconds of a warm-cache import of the same game.

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

- Desktop and dev builds using the shared store is out of scope.
- Keeping games, game records or sign-ins in the shared store is out of scope. Only card data is cached here; card images are covered by card-image-cdn.
- Changing how cards are matched or which print is preferred is out of scope. Only where card data is kept changes.
