---
created_date: "2026-11-04"
document_status: final
closed_date: "2026-11-04"
epic: 20261104082020-109dceb1-tcgdex-spaces-cache
---

# Feature: 20261104090640-a5a74fab-admin-cache-clearing

<!--
  OVERVIEW
  A concise 2-3 sentence summary of the feature. Answer three questions:
    1. What is being built?
    2. What problem does it solve?
    3. Who benefits and why does it matter?
  Avoid implementation details — this should be readable by any stakeholder.
-->
## Overview

Admins of the hosted playmat can clear its card cache, all of it or one set at a time, from a small page behind the existing sign-in. A correction from the card source can then be picked up at once. Open games keep working while the cache refills.

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

- [x] **Admins can clear the whole cache**
  An admin signed in to the hosted playmat can clear, in one action, all card data and images in every cache the hosted playmat keeps. Afterwards, every lookup is answered from fresh data fetched from the card source, without a restart.
- [x] **Admins can clear one set**
  An admin can clear a single set, named either by its decklist set code (such as MEG) or by its card source set id (such as me01): its card list, its cards and their images. Other sets stay cached. A code or id that names no set is refused with a message, and nothing is cleared.
- [x] **Only admins can clear the cache**
  Anyone who is not signed in as an admin must be refused, both on the page and on any request that clears the cache, and nothing may be cleared.
- [x] **Clearing does not interrupt play**
  Games open while the cache is cleared must keep working, with their cards refetched as needed.

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
- Admins must be named by their Google accounts in an App Platform environment setting, and must sign in with the hosted playmat's existing Google sign-in.

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

- [x] **Admin clears everything**
  Signed in with an account named as an admin, the admin uses the clear-all action. The shared store then holds no cached card data or images, the next card lookup makes a request to the card source, and the next import shows its cards correctly, with no restart.
- [x] **Admin clears one set by code**
  An admin clears one set by its set code. The next lookup of a card in that set makes a request to the card source, while a lookup of a card in another cached set does not.
- [x] **Admin clears one set by id**
  An admin clears one set by its set id, with the same result as clearing it by code.
- [x] **Unknown set refused**
  An admin asks to clear a set code that names no set. The page reports it, and the shared store still holds the same data.
- [ ] **Cleared images refreshed**
  After an admin clears a set, the board shows each of that set's card images as the card source currently has it within 1 hour.
- [x] **Non-admins refused**
  Signed in with an account not named as an admin, or not signed in, the admin page is refused and a direct request to clear the cache is refused. Afterwards the shared store still holds the same data.
- [x] **Open game survives a clear**
  With a game open on the board, an admin clears all cached data. The board keeps showing the game, and playing on it works, with card text and art.

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

- Clear every cache tier for the chosen data (memory, local disk and the shared store), and purge the image delivery network's cache for cleared images so old copies stop being served.

<!--
  SUCCESS METRICS
  How you will know the feature is working well after delivery. Be specific:
    - Quantitative: "p99 latency < 200ms", "error rate < 0.1%"
    - Behavioural: "users complete the flow without support intervention"
  Format: one bullet point per metric.
  Leave blank if not applicable.
-->
## Success Metrics

- An admin can make a corrected card or set show on the board within a minute, without a redeploy.

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

- Clearing a single card on its own is out of scope. An admin clears its whole set instead.
- Any other admin capability (managing users, games or settings) is out of scope.
