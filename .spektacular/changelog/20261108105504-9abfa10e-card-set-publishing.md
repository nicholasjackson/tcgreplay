---
created_date: "2026-11-08"
document_status: final
closed_date: "2026-11-08"
---

# Card set publishing

## What was built

The project's built card sets are now published to the playmat's existing DigitalOcean Spaces bucket and CDN. Every playmat, hosted and desktop, loads its card definitions only from those published sets.

**Publishing** (`internal/cardset`, `tcgreplay sets publish`)

- Each built set is stored as an unchangeable gzip file named by a hash of its contents: `cardsets/sets/<id>/<16 hex>.json.gz`.
- A small manifest (`cardsets/manifest.json`) names each set's current file. Set files are cached for a year; the manifest for 60 seconds, and it is purged from the CDN after every publish that changes it.
- Only changed sets are uploaded, and earlier versions stay in the bucket, so a rollback is a republish of an older commit.
- Publishing refuses while the set check reports open items, or when a set already published is missing from the repository.
- `-dry-run` shows what would be uploaded. `-to <dir>` publishes to a local directory.

**Workflows**

- "Publish card sets" (`.github/workflows/publish-cardsets.yml`) checks and publishes on every push to `main` that changes `cardsets/`, and can be started by hand.
- "Deploy playmat" (`.github/workflows/deploy.yml`) deploys on pushes that change anything else. App Platform's own deploy-on-push is off, so a card-data-only merge no longer restarts the hosted playmat and drops open games.

**Loading** (`internal/catalog`)

- An immutable snapshot of the whole catalogue answers every card lookup by the same index rules as before. The full English catalogue (206 sets, 21,439 prints) takes about 21 MB.
- A loader fetches the manifest, downloads only changed sets, verifies them, and swaps the snapshot atomically.
- It checks every 5 minutes (`-cardsets-interval`), and at once from the admin page's "Check for published card sets now".
- It keeps a verified local copy under the cache directory, so desktop plays offline after first use.
- Hosted and in-process modes wait up to 30 seconds at start. The daemon serves its local copy at once and checks in the background.

**Open games keep their cards.** Each game pins the snapshot it started with. New, imported or reopened games use the latest.

**Images**

- A card now carries the print's whole ordered image list.
- The board tries each source in turn and falls back on any load error. This shows art TCGdex lacks, such as malie.io files for Delta Reign.
- TCGdex images still go through the hosted CDN copy.

**On-demand card data retired**

- The TCGdex card client and its disk and bucket cache are deleted.
- A source-scan test keeps the TCGdex API host out of the playmat.
- The admin page now clears images only.
- `tcgreplay sets cleanup-legacy [-delete]` removed the old `tcgdex/` objects from the bucket.

**Settings and documentation**

- `TCGREPLAY_SPACES_ENDPOINT` values that name the bucket's own host or the CDN are now refused.
- Updated: `cardsets/README.md` (new "Publishing" section), `README.md`, the design doc, `.do/app.yaml`, the `Dockerfile` and the `Makefile`.
- On the website: the desktop page and the privacy page.
- The knowledge base's card-cache entries were replaced by `architecture/published-card-sets.md` and related rewrites.

**Done by the maintainer**

- Published all 206 sets.
- Added the seven repository secrets.
- Turned off App Platform Autodeploy.
- Deployed the switch: deployment `80f3ccd9`, ACTIVE on 2026-11-08 at 18:42 UTC.
- Deleted the 401 legacy `tcgdex/` objects from the bucket.

## Why it matters

New sets, promos, illustration rares and text fixes now reach players as soon as a reviewed change is merged, with no new release or redeploy of the playmat. Games in progress are not disturbed by a publish. Players never see half-finished upstream data, because the playmat no longer reads TCGdex while running. The program itself carries no card data: data lives in the bucket, logic and presentation in the binary.

## Deviations from the plan

- **The secrets and deploy task closed with an accepted shortfall.** Criterion "A hand-started deploy succeeds" rests on the maintainer's hand re-run of the push-triggered deploy run 42510960863 (attempt 2, success), not on a `workflow_dispatch` run. The goal check judged it not met three times, and the maintainer accepted the shortfall rather than trigger another deploy. The deploy's first attempt failed with `invalid uuid` because `TCGREPLAY_APP_ID` was wrong; the secret was corrected before the re-run.
- **Autodeploy was turned off in the DigitalOcean console** instead of with `doctl apps update --spec .do/app.yaml`. It is the same change: `deploy_on_push: false` is the only non-comment difference.
- **The first full publish was run by hand from the feature branch.** The publish workflow did not exist on `main` yet. The maintainer's first attempt failed because `TCGREPLAY_SPACES_ENDPOINT` named the bucket's own host, and such endpoints are now refused up front.
- **The daemon does not wait 30 seconds at start.** It serves its local copy at once and checks in the background, because its client handshake has a 10-second deadline.
- **Minor changes:**
  - The corpus regression test lives in `internal/game`, to avoid an import cycle.
  - Basic Energy's borrowed art is applied as a whole image list in the snapshot builder.
  - The test-only `-cdn-api` flag was added to `sets publish`.
  - The optional `make dev-cardsets` target was not added.
  - A native App Platform path filter does not exist, so the deploy workflow was used as planned.
