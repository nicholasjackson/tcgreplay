### Approach
Each layer clears itself for one scope (everything, or one TCGdex set id) through optional `Clear` interfaces, so `Store` does not change.
- **Card database:** `cards.Client.Clear` marks cards in memory as cleared rather than deleting them, so open games keep playing. `Cached` still returns the old copy while the new one is fetched; `Card` fetches now and falls back to the old copy.
- **Images:** `ImageCache.Clear` deletes the scope's copies and purges the CDN through the DigitalOcean API. Board image addresses carry `?v=<content hash>`, so browsers cannot keep old art past a clear.
- **Admin page:** `TCGREPLAY_ADMINS` names admins (verified Google emails), and `hostauth.RequireAdmin` guards a small server-rendered page in `internal/cacheadmin` that runs one clear at a time as a background job.
### Milestones and tasks
- M1, delete and purge: batch delete with checksums; the CDN purge client.
- M2, clear without interrupting play: clear the stores; clear the card database and name sets; prove an open game survives.
- M3, cleared images return as current art: version image copies; send versions to the board; clear copies and purge the CDN.
- M4, the admin page: admin guard; page and clear job; hosted wiring; settings and documentation; knowledge entries; admins and the CDN token (human).
### Tasks a person must do
- Name the admins in `TCGREPLAY_ADMINS`, and create a DigitalOcean API token with CDN access as `TCGREPLAY_SPACES_CDN_TOKEN`.
### Out of scope
- Clearing a single card, and other admin capabilities.
- Pushing cleared data to open boards.
- An admin page on desktop.
- Clearing TCGdex's own caches.
- Scheduled clears or an audit history.
- Re-warming after a clear.
### Drafting assumptions
- Image addresses carry `?v=` from the objects' ETags, on top of the purge.
- Cleared cards stay visible to `Cached`.
- One background clear at a time.
- Sign-ins from before this change count as unverified until the admin signs in again.
- Clears bypass the lookup breaker (30 s per request, 10 minutes per job).
- Partial failures are reported per tier, and clearing again is safe.
### Project-wide rules
- Adds `spaces.Client.Delete` (batch) and a `spaces.CDN` purge client.
- Extends `ListPage` with `ETags`.
- Extends `ImageCache` with versions and `Clear`; `webui.ImageSource.Image` gains a version, and `/api/card` gains `image_version`.
- Decides the settings `TCGREPLAY_SPACES_CDN_TOKEN` (secret) and `TCGREPLAY_ADMINS`.
- `hostauth.Account` gains `EmailVerified`.
### Manual checks
- Clear to corrected card in under a minute.
- Real admin sign-in and refusal for other accounts.
- A real clear of everything.
- Real set clears by code and by id, and an unknown code refused.
- A real CDN purge, with current art within 1 hour.
- An open game survives a real clear.
- Real Spaces batch delete, ETags and versioned CDN addresses.
