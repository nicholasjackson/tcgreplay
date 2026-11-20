---
created_date: "2026-11-04"
document_status: draft
project: tcgreplay
spec: 20261104082020-109dceb1-tcgdex-spaces-cache
plan: 20261104082020-109dceb1-tcgdex-spaces-cache
---

# Card cache survives deploys on the hosted playmat

The hosted playmat can now keep its cached card data in a DigitalOcean Spaces bucket, so a deploy or restart no longer empties the cache, and the first import afterwards is as fast as any other. The feature is switched on by four environment settings. Without them, or if Spaces is down or misconfigured, the playmat behaves exactly as before and players never see an error from it. Desktop and dev builds are unchanged and need no new settings.

> Derived from project tcgreplay, spec/plan 20261104082020-109dceb1-tcgdex-spaces-cache. See the project-level record for the full feature.

## What changed in this repo

- **`internal/spaces` (new).** A minimal DigitalOcean Spaces client on the standard library.
  - It signs requests with AWS Signature Version 4 and checks them against AWS's published example.
  - It provides `Get` and `Put` with content type, cache-control and public-read options.
  - It maps a 404 to `ErrNotFound` and any other failure to `*Error` with S3's status and code.
  - `FromEnv` reads `TCGREPLAY_SPACES_ENDPOINT`, `_BUCKET`, `_KEY` and `_SECRET`, deriving the signing region from the endpoint.
- **`internal/spaces/spacestest` (new).** An in-process stand-in for Spaces for tests. It verifies signatures, counts requests, and can fail, refuse, hang or close.
- **`internal/cards`.**
  - New `NewSpacesStore`: entries under `tcgdex/`, in the disk envelope.
  - New `NewTieredStore`: disk first; Spaces hits are copied down with their fetch time; writes go to both; a 3 s bound per Spaces request; a one-minute breaker that logs the outage and the recovery once each.
  - New `Client.Warm`, which builds the catalog.
  - `Cached` and offline lookups read only the local tier.
  - The disk store's path and envelope code are now shared helpers, with no behaviour change.
- **`cmd/tcgreplay`.**
  - Hosted start-up builds the card database through `hostedCardDB`. No settings: local only, one log line. Partial or invalid: a warning, local only. Offline: not used. All four set: tiered, plus a background catalog warm.
  - `newHosted` takes the card database as a parameter.
  - Desktop paths are unchanged.
  - `TestHostedGuidesMatchDesktop` now expects 7 guides.
- **Deploy and docs.**
  - `.do/app.yaml` declares the four variables as encrypted secrets.
  - `make docker-run` passes them through.
  - The `Dockerfile` header and `TCG Review Playmat Design and Build Plan.md` (trial limits, settings table, operator setup) describe them.
- **Tests.** Card-, game- and hosted-level tests cover the warm restart, copy-down, stale entries, outages, offline mode and the hosted settings.

## Why

The hosted playmat runs on App Platform with no persistent disk, so its card cache was lost on every deploy and the first import afterwards refetched everything from TCGdex. Keeping the cache in Spaces, behind local disk, makes it outlast deploys, while keeping Spaces strictly optional so play never depends on it.

## Still to do

- An operator must create the private Spaces bucket and a bucket-limited read/write access key, and set the four variables (encrypted) on the `playmat` component. Until then the hosted playmat keeps a local-only cache.
- The real-Spaces checks (warm restart, the 2-second first import, failure modes, 404 for missing keys) are manual and are in the test plan.
