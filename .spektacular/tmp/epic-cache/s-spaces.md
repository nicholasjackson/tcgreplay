### Approach
On the hosted instance, a tiered `Store` (local disk, then DigitalOcean Spaces) goes into the freshness plan's store slot.
- **Reads:** a disk miss is served from Spaces and copied to disk with `Fetched` kept.
- **Writes:** go to disk, then to Spaces.
- **Failures:** a breaker (3 s per request, Spaces skipped for 1 minute) absorbs outages, and the log records each outage once.
- **No-network reads:** `Local()` keeps `Cached` (under the game lock) and offline mode on local disk.
- **S3 client:** a new standard-library client in `internal/spaces` (SigV4 signing, path-style addresses), with an in-process fake in `spacestest`.
- **Start-up warm:** `Client.Warm` builds the catalog in the background at hosted start-up.
### Milestones and tasks
- M1, the playmat can read and write Spaces: request signing and settings; the client and its fake.
- M2, card data outlasts restarts: the Spaces card store; the tiered store with its breaker; local-only reads under the game lock and offline; restart acceptance tests.
- M3, hosted keeps its cache across deploys: hosted wiring with `Warm`; settings and documentation; the knowledge entry; the bucket (human).
### Tasks a person must do
- Create the Spaces bucket and access key, and set the four `TCGREPLAY_SPACES_*` secrets in App Platform.
### Out of scope
- Desktop and dev builds using Spaces.
- Games, game records or sign-ins in Spaces.
- Card images and clearing (later specs).
- Avoiding the first cold build.
- Queuing writes made during an outage.
- The CDN.
### Drafting assumptions
- A standard-library S3 client rather than minio-go or the AWS SDK.
- Settings are read from the environment in hosted mode only; partial settings warn and run local-only.
- Writes to Spaces are synchronous.
- Card data uses the fixed prefix `tcgdex/`, private.
- `Warm` runs only when Spaces is configured and the playmat is online.
### Project-wide rules
- Decides optional `Local() Store` on tiered stores, found by type assertion.
- Decides the bucket layout: `tcgdex/` at the disk paths, private, `application/json`.
- Decides `internal/spaces` (Config, FromEnv, New, Get, Put with PutOptions, ErrNotFound, *Error) as the shared client, with `spacestest` as the shared fake.
- Decides the settings `TCGREPLAY_SPACES_ENDPOINT`, `_BUCKET`, `_KEY`, `_SECRET`, environment and hosted only.
- Decides the breaker: 3 s per request, 1 minute skip, edge-triggered logs.
### Manual checks
- The 2 s first-import-after-restart metric.
- Real Spaces end to end: import, redeploy, import again.
- Real failures: a wrong secret and an unreachable endpoint.
- A missing key on the real bucket is a 404, not a breaker trip.
- A hosted start with no settings logs one local-only line.
