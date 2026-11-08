---
created_date: "2026-11-08"
document_status: final
closed_date: "2026-11-08"
epic: 20261108105504-9abfa10e-card-set-publishing
---

# Feature: 20261108105504-9abfa10e-card-set-publishing

<!--
  OVERVIEW
  A concise 2-3 sentence summary of the feature. Answer three questions:
    1. What is being built?
    2. What problem does it solve?
    3. Who benefits and why does it matter?
  Avoid implementation details — this should be readable by any stakeholder.
-->
## Overview

Built card sets are published to the playmat's online storage, and the playmat reads card data only from there. New sets, promos, illustration rares and text fixes reach players as soon as they are published, with no new release of the playmat. Games in progress are not disturbed, and desktop players can keep playing offline. Pulling card data from upstream sources while people play is retired, so players never see half-finished upstream data.

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

- [ ] **Publish sets**
  Built sets can be published to the playmat's online storage. Only sets that changed are uploaded, and earlier published versions of a set stay retrievable.
- [ ] **Playmat reads card data only from published storage**
  The playmat gets every card definition from published sets, never from the upstream card sources while running, and the released program carries none.
- [ ] **Published changes picked up while running**
  A running playmat picks up newly published or changed sets without a restart, both on a regular check and when an admin asks for it from the admin page.
- [ ] **Open games keep their cards**
  A game in progress keeps the card definitions it started with until it ends, even when a newer version of a set is picked up. A game started, imported or reopened afterwards uses the current published definitions.
- [ ] **Desktop works offline after first use**
  The desktop playmat keeps a local copy of the published sets it has loaded, and uses that copy when it cannot reach online storage.
- [ ] **Card images still shown**
  Card images keep showing on the board as they do today, from the image source each built print names.
- [ ] **A publish reaches every viewer**
  After a publish, every playmat and every request for a changed set gets the new version, and any image whose source changed is shown from the new source rather than an old copy.
- [ ] **On-demand card data retired**
  The playmat no longer fetches, refreshes or clears card definitions from upstream sources. A one-off maintainer clean-up removes the card data left over from that approach in online storage, and the playmat ignores any left in local caches.
- [ ] **Every built set published before the switch**
  Every built set is published before on-demand card data is retired, so saved games, game records and imported logs from before the switch still show full card definitions.
- [ ] **Documentation for publishing**
  The project documentation describes how sets are published and how playmats pick up a publish.

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

- **Data in the bucket; logic and presentation in the program.** In the user's words: "Clean separation is data in the bucket, logic and presentation in the binary." The released program carries no card definitions, not even as a fallback.
- **Use the existing online storage and CDN.** Published sets live in the Spaces bucket and CDN the playmat already uses ("we have the cdn"), not a new hosting service.
- **New data must not need a redeploy.** Publishing a new or changed set reaches running playmats without releasing or redeploying the program. Code changes, such as pair play itself, still ship by release.

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

- [ ] **Publish uploads only what changed**
  Publishing after changing one set uploads no other set's data. A previously published version of that set is still retrievable, byte for byte, after the publish.
- [ ] **Playmat makes no upstream card requests**
  A playmat running a full game (import a log, play turns, read card text) makes no requests to the upstream card data sources. Its card definitions match the published sets.
- [ ] **No card data in the program**
  The released program contains no card definitions. A playmat started with no access to published storage and no local copy shows cards as unknown rather than with definitions.
- [ ] **Published change appears without restart**
  After a set is published with a changed card, a running hosted playmat shows the changed card in a new game:
  - within 10 minutes, with no restart;
  - at once after an admin asks for a check from the admin page.

  A running desktop playmat that is online shows the changed card in a new game within 10 minutes, with no restart.
- [ ] **Open game unaffected by a new version**
  A game started before a set's new version is picked up still shows the card text it started with for the rest of that game. A game started or reopened after the pickup shows the new text.
- [ ] **Desktop offline after first use**
  A desktop playmat that has loaded sets once, then started with no network, plays a game with full card text from its local copy.
- [ ] **Images still shown**
  On a hosted board, a Delta Reign card with an image source shows its image. Where the hosted image copy exists, the image is served from the content delivery network.
- [ ] **Publish reaches every viewer**
  After a publish that changes a set, a fresh request for that set's current version, through the content delivery network, returns the new version, not the old one. An image whose source changed in that publish is shown from the new source.
- [ ] **Old card-data path gone**
  The admin page no longer offers to clear card definitions, while clearing images is still offered. After the maintainer clean-up, online storage holds no card data from the on-demand approach. A playmat whose local cache still holds card data from that approach does not use it, and its definitions match the published sets.
- [ ] **Existing records still work**
  A game record saved and a battle log imported before the switch, using cards from sets other than Delta Reign, load after the switch with full card text for every card.
- [ ] **Docs describe publishing**
  The documentation describes publishing a set and how quickly playmats pick it up.

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

- **Publish layout.** Content-addressed, immutable set files in the bucket, plus a small manifest naming each set's current file. Only the manifest needs a short cache lifetime or a CDN purge, so publishing needs no other cache-busting.
- **Who publishes.** Prefer a GitHub Action that publishes changed sets on merge to `main`, so what is served matches the reviewed repo. Keep a manual publish command for the maintainer as well.
- **Loading at runtime.** The playmat loads the manifest at start, re-checks it on an interval and on an admin trigger, fetches only changed sets, and swaps the catalogue atomically. The set-code index and by-name index are built from the loaded sets. Desktop keeps its downloaded copy on disk.
- **Retire on-demand card data.** Remove the TCGdex fetch, refresh ages, the Spaces tier for card definitions, request de-duplication and failure memory, warming, and the card-data scope of the admin clear. Keep the image cache and its CDN copies, and keep the image half of the admin clear.
- **Known risks:**
    - A first desktop run needs network access.
    - The manifest check interval sets how quickly hosted playmats see a publish; the acceptance criteria assume at most 10 minutes.

<!--
  SUCCESS METRICS
  How you will know the feature is working well after delivery. Be specific:
    - Quantitative: "p99 latency < 200ms", "error rate < 0.1%"
    - Behavioural: "users complete the flow without support intervention"
  Format: one bullet point per metric.
  Leave blank if not applicable.
-->
## Success Metrics

- The median time from a set change being merged to being live on the hosted playmat is under one hour.
- Zero player reports of cards showing as unknown or without text after the switch.

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

- **Changing how images are fetched or stored**, beyond showing a changed image source after a publish.
