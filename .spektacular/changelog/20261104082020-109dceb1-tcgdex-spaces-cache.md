---
created_date: "2026-11-04"
document_status: final
closed_date: "2026-11-04"
---

# TCGdex card cache kept in DigitalOcean Spaces across deploys

## What was built

The hosted playmat can now keep its cached TCGdex card data in a DigitalOcean Spaces bucket as well as on local disk. The cached data is the set list, each set's card list and every card. A deploy or restart no longer throws that cache away.

- **A Spaces client (`internal/spaces`).** A small client for Spaces' S3 API, on the standard library only. It signs requests with AWS Signature Version 4, checked byte for byte against AWS's published GET Object example. It addresses objects path-style and reads and writes whole objects. A missing object is reported as "not found"; any other failure is a typed error carrying S3's status and code. Writes can set content type, cache-control and public-read, ready for the card-image spec. Settings come from `TCGREPLAY_SPACES_ENDPOINT`, `_BUCKET`, `_KEY` and `_SECRET`, with the signing region taken from the endpoint. Error messages name a setting but never echo its value.
- **A stand-in Spaces for tests (`internal/spaces/spacestest`).** An in-process fake of the S3 API that checks signatures with the real verifier and counts requests per key. It can be made to fail, refuse credentials, hang or go away. Every test of the shared tier uses it, and the image and admin-clearing specs can reuse it.
- **A Spaces card store.** It keeps each entry under `tcgdex/` at the disk path, in the same `{"fetched","data"}` envelope the disk cache writes. The disk store's path mapping and envelope code were factored out and shared, with no behaviour change.
- **A tiered store (`cards.NewTieredStore`).**
  - Reads come from disk first. A disk miss is served from Spaces and copied to disk with its original fetch time, so an entry that was stale before a restart is still stale after it.
  - Writes go to disk, then to Spaces.
  - Each Spaces request is bounded at 3 s. Any failure is absorbed and logged once, and Spaces is skipped for one minute before it is tried again; the recovery is logged once too.
- **Local-only reads where the network is forbidden.** The cached-only lookup the game makes under its lock, and every lookup in offline mode, read only the local tier. A cold miss reads Spaces once, not twice.
- **Hosted wiring.**
  - Hosted start-up reads the four settings. None set: it runs as before and logs one "local only" line. Some set or invalid: it logs a one-line warning and runs local-only. Offline: it does not use them. All four set and online: the card database gets the tiered store, and the card catalog is warmed in the background, so the first import does not wait for it and the health check answers at once.
  - Desktop, daemon and dev builds never read the settings.
- **Deployment and documentation.**
  - `.do/app.yaml` declares the four variables as encrypted secrets.
  - `make docker-run` passes them through.
  - The Dockerfile header and the design document (trial limits, settings table, operator setup) describe them.
  - The card cache knowledge entry describes the shared tier.
- **Tests.** The tests prove the spec end to end against the fake TCGdex and the stand-in Spaces:
  - a warm restart with an empty disk makes zero card-source requests and gives identical answers, at card-database level and at game level;
  - a card only in Spaces fills the local disk;
  - a new card lands in both tiers;
  - stale entries stay stale and are refreshed once;
  - every outage mode is absorbed and logged;
  - offline mode makes no Spaces requests.

## Why it matters

On App Platform the playmat has no persistent disk, so every deploy started with an empty card cache. The first import afterwards then had to fetch about 200 sets and every card from TCGdex. With the shared store configured, card data fetched once survives deploys and restarts, and only data the bucket lacks, or that has gone stale, is fetched again. Spaces is an optional cache: if it is missing, misconfigured or down, play carries on from the local cache and TCGdex with no error shown to players.

## Deviations from the plan

- **Outstanding human task.** "Create the Spaces bucket and set its secrets" was skipped by the agent run and remains open. Until an operator creates the bucket and access key and sets the four variables on the `playmat` component, the hosted playmat runs with a local-only cache, exactly as before. Its checks are the first item of the test plan.
- **Breaker and caller cancellation.** A shared request that ends because the caller's own context ended does not trip the breaker. Not in the plan; added because the client's fetch path hands the store a context bounded by its own timeout.
- **Single-flight recheck.** The recheck reads only the local tier, so a cold miss costs one Spaces read rather than two. `Card`, which reads the store directly, also follows the offline rule.
- **Error type placement.** The `Error` type lives in its own file and arrived with the signer, because the test-side verifier returns it. The verifier also refuses unsigned `x-amz-*` headers, as S3 does.
- **Path helper.** The shared key-to-path helper is `relPath(key) string`, not `keyPath` with an error. A multi-setting warning is joined onto one log line.
- **Test logging.** Hosted tests that read logs while background work runs use a synchronised log writer.
- **Knowledge tags.** The knowledge entry gained the `spaces` tag but kept the existing `hosting` tag rather than adding `hosted`.
- **Outside the plan.** `TestHostedGuidesMatchDesktop` now expects 7 desktop guides (a `play-game` guide had been added on main).
- **Known and left alone.** `go test -race ./...` can fail in `cmd/tcgreplay` `TestRelaySurvivesDaemonRestart` because of a data race in that test's own code. It is pre-existing and unrelated to this work.
