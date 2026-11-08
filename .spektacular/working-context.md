# Working context: 20261108105504-9abfa10e-card-set-publishing

## Problem and motivation

- User: "We need to support the new delta reign set." Delta Reign = TCGdex `me06`,
  decklist code `DLR`, TCG Live log code `me6_<n>`, 141 cards (103 official),
  released 2026-11-06 (TCG Live 2026-11-05).
- Ids already resolve with no code change (live check: `Resolve("DLR", n)`,
  `LogCodeID("me6_84")`, `ByName` all hit `me06-…`).
- TCGdex data for the new set is incomplete (checked 2026-11-08): no images on all
  141; no `effect` on all 24 Trainers and 3 special Energy; no `evolveFrom` on all
  30 Stage 1/2; raw HTML `<span class="energy-symbol …">` in attack/ability text on
  19 cards; no `regulationMark`; `legal.standard` false.
- Cache freeze: `internal/cards/fresh.go` refreshes imageless cards every 24 h but
  never refreshes a card with an image, so a card whose art lands before its text
  is fixed keeps bad text forever (until admin clear).
- malie.io TCG Live export (`index.json` → `en-US` → `me6`, abbr `DLR`) had full
  text, regulation mark J and images for every card on day one. Fan project: ask
  on its Discord before relying on it.
- Knowledge entry written: repo/tcgreplay `gotchas/new-set-tcgdex-data-incomplete.md`.

## Legendary Stadiums (new mechanic)

- Legendary Pyroduct (094/095), Legendary Summit (096/097), Legendary Trench
  (098/099): two different prints form one Stadium.
- English rule text, from the user (verbatim): "You cannot play this card by
  itself. You must combine 2 different Legendary Pyroduct from your hand to play
  as 1 Stadium card." plus the normal Stadium reminder. Neither TCGdex nor malie
  carries this rule.
- Effects (malie English text): Pyroduct — Evolution Pokémon in play have no
  Abilities; Summit — a {C} Pokémon KO'd by attack damage gives 1 fewer Prize;
  Trench — healing doubled.
- Playmat today: `state.Stadium` holds one IID; `apply.go` refuses a second card
  ("stadium is occupied"); ~20 references across state, game, importer, webtest;
  board components Side/Strip/Modal/actions.js; importer handler
  `{P} played (.+) to the Stadium spot\.` (handlers.go:92).
- Open: how TCG Live logs a pair (one line or two). Asked the user for a sample
  battle log; not yet supplied.

## Decisions reached in discussion (user's words where key)

- Build sets offline instead of pulling card data on demand. User: "when a new set
  comes out we should have an offline process which builds the set." Images may
  still be pulled on demand.
- A project skill drives it. User: "we do what we can in code but it needs an agent
  to verify." Agent verifies each flagged card against its card image and sources.
- "The built deck should live with the code base so that we can change it." Repo
  holds import inputs, corrections file and built sets (reviewable, history).
- "There should be an import command that can update this and potentially bust the
  cache."
- Corrections live in their own file, separate from built output, so re-import
  never overwrites a hand fix; report flags corrections the sources now agree with.
- Distribution: first agreed commit+redeploy (embed), then user reversed: "we have
  the cdn so if we need to just update the set do we really need to redeploy. Think
  about things like illustration rares etc." Final principle, user verbatim:
  **"Clean separation is data in the bucket, logic and presentation in the binary."**
  - No card data in the binary, not even a fallback snapshot.
  - Publish to Spaces: content-addressed set files (immutable, cache forever) plus a
    short-cache manifest; only the manifest needs purging.
  - Playmats load the catalogue from the manifest and swap in changed sets without
    restart (periodic check plus admin trigger). Desktop keeps a local copy as data
    on disk; first run needs network once.
  - Data states what a card does (e.g. a "play as pair" field set by corrections);
    binary implements the general behaviour, never card-name special cases.
  - Proposed: GitHub Action publishes on merge to main (repo == served); open games
    keep the definitions they started with. Not yet confirmed by user.
- Languages: "maybe we want to support other languages in the future". Format should
  be a language-independent core per set plus per-language text, English only now.
  Japanese is a separate catalogue (different sets), not a translation. Effect
  reader stays English.
- One spec with milestones (user chose over two specs). Standalone, not in an epic
  (it replaces much of the tcgdex-spaces-cache epic's card-data caching).

## Sizes measured (for reference)

- Binary today 21.7 MB. All English: 221 sets, ~24,140 cards; ~12 MB minified JSON,
  ~1.4 MB gzip. One set ~130 KB minified, ~15 KB gzip. Pretty JSON in repo ~19 MB.

## Rejected alternatives

- Keep on-demand TCGdex with a fixed refresh rule only: cheap but leaves data
  unverified and Legendary rule missing.
- Embed built sets in binary (commit + redeploy): rejected by the user in favour of
  data in the bucket (promos, illustration rares, art arriving later change data
  often).
- Data folder shipped beside the binary: superseded by bucket principle.

## Workflow notes

- User: "Just run this through to the end" — drive every spec step without asking;
  sections drafted from the discussion, assumptions marked in interview.md.
- Fresh-eyes review: 24 findings, applied without asking (user said run to the end).
  Notable additions: "Every English set published" requirement + "Existing records
  still work" AC (retiring on-demand data would otherwise break old games); pair rule
  from data moved to Constraints; 10-minute pickup bound chosen for testability;
  pair-log import deferrable if no real TCG Live sample exists at build time.
- Split check passes (set publishing vs pair play), but the user had already chosen
  one spec with milestones, so no split was offered.
- User agreed to split ("This is great"). Epic 20261108105504-9abfa10e-card-set-publishing:
  card-set-import (20261108110847-d27a9f77, no deps); card-set-publishing (original
  spec, narrowed to publishing/loading/retiring, depends on import); paired-stadiums
  (20261108110847-055548e3, depends on both). Split review's 22 findings applied
  afterwards via spec file write (the split ran before the fixes were saved).

## Epic planning run (spek-plan-epic, 2026-11-08)

- Orchestrating planning of epic 20261108105504-9abfa10e-card-set-publishing.
- Order: card-set-import (ready) -> card-set-publishing -> paired-stadiums.
- Started child: 20261108110847-d27a9f77-card-set-import (start).
- card-set-import: DONE (summary kept in .spektacular/work/epic-plan-card-set-publishing/done-card-set-import.md).
- Started child: 20261108105504-9abfa10e-card-set-publishing (start).
- card-set-publishing: DONE (summary kept in .spektacular/work/epic-plan-card-set-publishing/done-card-set-publishing.md).
- Started child: 20261108110847-055548e3-paired-stadiums (start).
- Cross-plan disagreements spotted (to settle in step 6 after paired-stadiums):
  1. Print.Card image: import = first tcgdex image; publishing = first showable source (tcgdex or malie jpg).
  2. Publishing's source-scan test (no non-test Go file names the TCGdex API host) vs import's own TCGdex fetcher in internal/cardset.
- paired-stadiums QUESTION (verification step): importer pair recognition — A log-only (recommended) vs B read playedAsPair via pinned snapshot. Put to user.
  - User answered: A (importer recognises pairs from log lines only, never reads the mark).
- paired-stadiums: DONE.
- Step 6 disagreements to settle with user:
  1. Print.Card image (import: first tcgdex; publishing: first showable incl. malie).
  2. Source-scan TCGdex host test (publishing) vs import's TCGdex fetcher in internal/cardset.
  3. Pair rule text twice: import corrects effect text with printed rule; paired-stadiums Text() adds generated rule line.
  (Strict decode concern from paired-stadiums checked: publishing loader leaves snapshot in place on unknown format/failure — aligned, not raised.)
  - User answers: (2) host scan — accept proposal (host allowed only in internal/cardset fetcher). (3) rule text — accept proposal (drop generated Text() line; printed rule in data).
  - (1) image: user said "We should have multiple art sources" — clarifying what that means for Print.Card / board.
  - (1) clarified: board falls back through the ordered image list (cards.Card + /api/card carry it); not gated on malie's answer. Applying edits via 2 helpers (publishing+import note; paired-stadiums).
  - paired-stadiums rule-text decision applied (plan, context, research).
- Step 6 done: summary sections + decisions written; epic order added nothing. Step 7 review presented to user.
- Review: user approved all three plans as they stand ("ok looks good"). Epic ready to implement.

## Epic implement run (spek-implement-epic, 2026-11-08)

- Orchestrating implementing of epic 20261108105504-9abfa10e-card-set-publishing.
- Order: card-set-import (ready) -> card-set-publishing -> paired-stadiums (strict chain).
- Not dirty, no problems at start.
- Started child: 20261108110847-d27a9f77-card-set-import (start), trace span 05754fc173de7ff7e79f3b7cf1ee4132-62c1ef1033ca7321.
- card-set-import QUESTION: goal check exhausted on human task 'Ask malie.io about build-time use' (58104bf7). Options skip (recommended) / keep_fixing (if user has malie answer) / accept. Put to user.
  - User answered: skip (task left open for the maintainer).
  - 2026-11-08: user asked malie.io on Discord (build-time read, republish to own CDN, image fallback, credit, breaking-change notice). Awaiting reply; when it arrives, record in learnings/card-image-sources.md via spek-knowledge and close task 58104bf7.
- card-set-import QUESTION (task "Build and verify the older sets", 14/17 done): 
  Q1 four TCGdex sets list no cards (wp, sp, rc, jumbo): (a) empty built sets + doc note [rec], (b) omit, (c) hand-build (not possible).
  Q2 263 open older-set items: (A) all via parallel sub-agents (~200 image reads, several hundred k tokens) [rec if spend ok], (B) fill 77 unambiguous + acknowledge 186 as unverified, (C) stop partial. Put to user (user away swimming).
  - User answered: Q1 (a) keep empty + doc note; Q2 (B) fill 77 unambiguous, acknowledge 186 as unverified/left for maintainer.
- card-set-import QUESTION: goal check exhausted on 'Build and verify the older sets' (21d20a65): criterion 3 (every card has English name/text) not met because of the acknowledged gaps (196, not 186). Answered 'accept' as the direct consequence of the user's option-B decision (no new user question).
- card-set-import: DONE (commit 9c0453e) and MERGED. 16/17 tasks; malie ask task open; spec 23/25 checkboxes (Every English set built, English text everywhere unticked by option B). Knowledge: learnings/upstream-card-source-formats.md (updated), gotchas/tcgdex-card-data-errors.md (new).
- After merge: card-set-import not counted implemented (open human task 'Ask malie.io'), so publishing still blocked; publishing worktree already created (no child yet). Asking user: record ask now and close task (rec) vs start publishing anyway.
  - User answered: record ask (asked 2026-11-08, no reply yet) and close the task. Resuming import lane with a child.
- Started child: card-set-import (resume, close malie task), span 6772f977c0fa057815684669609538fb-3804cf29cc4b1da5. card-set-publishing worktree exists, child span 265a28879b5e7c532cf04dcf853e7bc5-268e09f37bc3a5f0 reserved, not started (waits for import done).
- card-set-import: malie task closed (asked, no reply, noted in learnings/card-image-sources.md); merged again; status done. Starting card-set-publishing child (span 265a28879b5e7c532cf04dcf853e7bc5-268e09f37bc3a5f0).
- card-set-publishing QUESTION at analyze: 4 agent tasks done (publisher, sets publish cmd, publish-cardsets.yml, deploy.yml + deploy_on_push:false). Next tasks human: secrets + CDN address (1e0644d4), publish every built set (e0a9670e). Options A (rec) continue 6 independent agent tasks via single-task runs, stop before 663730e9; B user gives CDN address + accepts pending; C stop, merge milestone 1 first. Put to user.
  - User answered: A (continue the 6 independent agent tasks; stop before 663730e9).
  - User keeps deploy.yml (explained why: card-data merges must not redeploy). User added deploy secrets to GitHub (2026-11-08). Still needed: confirm 5 TCGREPLAY_SPACES_* secrets, CDN address; after merge doctl apps update + run Deploy playmat once; publish.
- card-set-publishing QUESTION: 10/17 done (6 commits b055149..6a609da on branch). Stopped before 663730e9. Needs CDN address + publish (e0a9670e). Knowledge: architecture/games-read-cards-through-their-db.md (updated), gotchas/fakedb-embedding-bypasses-overrides.md (new). Put to user; suggesting local publish from worktree to break chicken-and-egg.
  - User answered: A (send CDN address + local publish from worktree). Waiting for address and publish result.
  - User published all 206 sets to bucket tcgreplay locally from worktree (2026-11-08); second run: 206 unchanged, manifest unchanged. First attempt failed 404 NoSuchKey: endpoint included bucket (virtual-hosted); follow-up: have child make sets publish reject such endpoints. Still need CDN address (region).
  - CDN address: https://tcgreplay.ams3.cdn.digitaloceanspaces.com (manifest verified 200 via curl). Resuming publishing child: tick e0a9670e, run 663730e9 onward, add endpoint-with-bucket validation.
- User: 'just let it run' — drive on without asking where a sensible default exists; only raise true user-only blockers.
- card-set-publishing: DONE (agent tasks) and MERGED (commits b055149..2881147). 15/17; open human tasks 1e0644d4 (doctl apps update + first Deploy playmat, post-merge) and 0cd658d7 (cleanup-legacy after deploy). Feature wrap-up (test plan, changelog, spec reconcile) runs when last human task closes. Knowledge: architecture/published-card-sets.md (new), gotchas/tcgdex-cache-never-expires.md (deleted), several updated.
- paired-stadiums formally blocked only by publishing's post-merge human tasks; its code deps are merged (main 5838bd9). Per 'just let it run', starting it now.
- paired-stadiums child FAILED at start: dependencies_unmet (publishing 15/17). CLI requires user consent for override_dependencies. Asking user.
  - User answered: wait for their deploy steps; don't override. Run paused: no child running. paired-stadiums worktree exists (unstarted). On resume: close publishing's 1e0644d4/0cd658d7 via a child (triggers wrap-up), merge, then start paired-stadiums.
- 2026-11-08: user turned off App Platform Autodeploy in DO console (equivalent to doctl apps update deploy_on_push:false). Remaining for 1e0644d4: push main / first Deploy playmat run, confirm 'card sets: loaded'. Then 0cd658d7 cleanup-legacy.
- 2026-11-08: user ran sets cleanup-legacy -delete: 401 tcgdex/ objects deleted, re-run 0 (0cd658d7 done). main not pushed yet (ahead 12) — first Deploy playmat run still pending for 1e0644d4.
