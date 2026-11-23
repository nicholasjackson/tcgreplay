---
created_date: "2026-11-23"
document_status: final
closed_date: "2026-11-23"
---

# Research: 20261122190321-c6d9d90a-sign-in-page

## Alternatives considered and rejected

- **Render the sign-in page directly in `/authorize` and `/auth/login` responses (no redirect).** Rejected: the form and the Google button both need the sign-in's state, and a page at its own address (`GET /auth/signin?state=…`) can be re-shown after a failed password without re-running `/authorize`. Both entry points already funnel through `startGoogle` (`internal/hostauth/google.go:41`, called from `oauth.go:291` and `board.go:27`), so one replacement there covers both.
- **Skip the page when no password accounts are configured (straight to Google, as today).** Rejected: contradicts the spec's "No form without accounts" requirement (page still appears with only the Google button) and the interview answer recorded in `.spektacular/working-context.md`.
- **A separate login flow for password accounts (own cookie, own completion).** Rejected: the spec's Technical Approach says both providers end in the same signed-in account and the same completion. The Google callback's tail (`google.go:94-111`: `signIn` then `continueConnector` or redirect to `next`) is the shared completion to extract.
- **Argon2id (golang.org/x/crypto/argon2) or stdlib PBKDF2 (`crypto/pbkdf2`, Go ≥1.24).** Rejected in favour of bcrypt. Argon2's PHC string contains commas (`m=65536,t=3,p=4`), which clash with a comma-separated env list. PBKDF2 has no standard string format, so it would need our own generator command. bcrypt hashes use only `./A-Za-z0-9$`, and `htpasswd -nBC 12 <user>` prints exactly `user:$2y$12$…`, the config entry, with no code of ours. Go's `x/crypto/bcrypt` accepts `$2a$`, `$2b$` and `$2y$`.
- **A `tcgreplay hash-password` subcommand.** Rejected for now: reading a password without echo needs `golang.org/x/term` (not in `go.sum`), and htpasswd (or `docker run --rm -it httpd:2.4 htpasswd -nBC 12 <user>`) already does it. Only `sets` is a subcommand today (`cmd/tcgreplay/main.go:167-169`).
- **Namespacing Google account IDs too (e.g. `google:<sub>`).** Rejected: it would change every Google player's ID, which `get_account` returns as OpenAI's stable profile id (`internal/mcpserver/account.go`), and change their board handle (`usergames.Handle`, `usergames.go`). That breaks "same account as before". Only password IDs get a prefix.
- **Fixed lockout counter for failed passwords.** Rejected in favour of extending the existing token bucket (`internal/hostauth/limit.go:37-65`), as the spec asks. A bucket of 10 refilled over 15 minutes refuses the 11th failure inside 15 minutes, which is the same guarantee.
- **Keeping `TCGREPLAY_REVIEWER` alongside password accounts.** Rejected by the spec (retire it). Failing start-up when it is still set was also rejected: the spec says the app must keep starting cleanly. It is ignored with a start-up warning instead.

## Chosen approach — evidence

- One shared start point: `startGoogle` (`internal/hostauth/google.go:41-62`) records a `pendingLogin` (`store.go:73-84`), sets a per-state login cookie (path `/auth/google/callback`), and redirects to Google. Replace it with `startSignIn`, which records the pending sign-in, sets the cookie at path `/auth/`, and redirects to `/auth/signin?state=…`. The Google button then goes to `GET /auth/google?state=…`, which adds the nonce and verifier to the same pending entry and redirects to Google with that state. The callback (`google.go:68-112`) is unchanged apart from the shared tail.
- Shared completion: the tail of `serveGoogleCallback` (`signIn` with cap refusal → busy page; then `loginConnector` → `continueConnector` (`oauth.go:326`) or redirect to `p.next`). `signIn` (`google.go:217-235`) already sets the board session cookie for both kinds, which gives "board opens signed in after connecting".
- Accounts in sealed state: `grantClaims` (`seal.go:93-108`) seals Sub, Email and Verified into the board cookie, access token and refresh token. Add a provider field (omitted = Google, so old seals still open as Google) and, for password grants, a credential fingerprint. Sealed grants are reopened after restart (`store.go` `session`, `takeRefresh`, `accessToken`), so a removed or re-passworded account is only revoked if opening checks the current config.
- Token → account on tool calls: `verify` (`oauth.go:449-465`) puts email/verified into `TokenInfo.Extra`, and `AccountFromToken` (`oauth.go:471-478`) reads them back. Provider must ride the same way, or the reserved place is lost on tool calls (gotcha `cap-bypass-needs-lost-place`).
- Reserved place: `usergames.Registry.Use`/`full`/`isReviewer` (`internal/usergames/usergames.go:122-182`). The reviewer is matched by verified email against `Config.Reviewer`. Replace this with a check that the account is a password account.
- Admin: `Auth.IsAdmin` (`internal/hostauth/admin.go:13-15`) requires `EmailVerified` and an email in `Config.Admins`. Password accounts are never verified. Usernames may be email addresses, so the explicit provider check is what keeps them out.
- Rate limiting: `store.take(now, id, limit)` (`limit.go:42-65`) is a per-key bucket refilled at `limit`/minute. `sweep` forgets a bucket after one untouched minute (`store.go:164-168`). This needs to become capacity-and-window aware so a 15-minute window is not forgotten after a minute.
- Pages: one `html/template` in `internal/hostauth/pages.go` (system font, simple light/dark). The sign-in page is a second template beside it.
- Identity separation: board handle = SHA-256 of account ID (`usergames.Handle`); registry keyed by account ID; telemetry player ID = keyed hash of `Event.Email` (`internal/telemetry/recorder.go:112-130`). Password account ID `password:<username>` (Google `sub` is numeric) and Email = username, where usernames cannot contain `@`. Together these make handle, registry entry, session and telemetry ID disjoint from every Google account.
- Board display: `/auth/session` returns `{email}` (`board.go:113-121`). The board header shows it with the tooltip "Signed in with Google" (`web/src/App.svelte:437-439`), and `ownName()` splits at `@` (`App.svelte:220-222`).
- Config and deploy: settings come from env in `cmd/tcgreplay/main.go:212` and `hosted.go:121-137`. Secrets are declared in `.do/app.yaml` as `type: SECRET` (the `TCGREPLAY_REVIEWER` entry is at `.do/app.yaml:166-173`). Gotcha `app-spec-not-applied-by-deploy`: env values are set in the console, a malformed or required setting rolls the deploy back, and saving settings restarts the app.

## Files examined

- internal/hostauth/hostauth.go:28-60 — Config (Google creds, SigningKey, RateLimit, Places, Admins, Recorder); Account{ID, Email, EmailVerified}; Routes registers google callback, oauth, board.
- internal/hostauth/google.go:41-62 — startGoogle: pending + login cookie (path /auth/google/callback) + redirect with PKCE/nonce/select_account.
- internal/hostauth/google.go:68-112 — serveGoogleCallback: state/cookie check, takePending, exchange, signIn, continue by kind.
- internal/hostauth/google.go:217-235 — signIn: Places.Admit (cap), addSession, session cookie, telemetry SignIn.
- internal/hostauth/board.go:20-27 — /auth/login → startGoogle(loginBoard, next). board.go:57-96 RequireSession redirects signed-out page loads to /auth/login?next=.
- internal/hostauth/board.go:113-121 — /auth/session returns {email}.
- internal/hostauth/oauth.go:258-298 — serveAuthorize validates client/redirect/PKCE/resource, then startGoogle(loginConnector).
- internal/hostauth/oauth.go:326-336 — continueConnector issues code, redirects to client.
- internal/hostauth/oauth.go:449-478 — verify and AccountFromToken (TokenInfo.Extra email/verified).
- internal/hostauth/admin.go:13-60 — IsAdmin and RequireAdmin (message about re-signing in when unverified).
- internal/hostauth/limit.go:37-90 — bucket/take/allow; ErrBusy/BusyError.
- internal/hostauth/store.go:13-60,140-170 — store maps, sweep (bucket forgotten after a minute), pendingLifetime 10m, sessions 30d.
- internal/hostauth/seal.go:76-108 — seal kinds; grantClaims {i,c,u,e,v,x}; account().
- internal/hostauth/pages.go — page template, renderPage/errorPage/busyPage.
- internal/hostauth/hostauthtest/client.go — FollowSignIn (expects 3xx chain to the client callback), Connect, MCPTransport, BoardCookie (/auth/login?next=/auth/session). All assume no page between the playmat and Google.
- internal/hostauth/hostauthtest/google.go — FakeGoogle (SetAccount, Prompts), Browser (cookie jar, alias transport).
- internal/hostauth/google_test.go:30-140 — harness: begin() expects /start → 302 to Google; signInOK; TestCookieAttributes checks login cookie path.
- internal/hostauth/{oauth,board,cap,admin,verified,events,limit}_test.go — suites that drive sign-in through hostauthtest/harness.
- internal/usergames/usergames.go:30-60,122-182 — Config.Reviewer, Use/Admit/isReviewer/full; Handle(acct.ID).
- internal/usergames/mcp.go — ResolveMCP/AccountID read AccountFromToken.
- internal/mcpserver/account.go — get_account returns account ID (OpenAI profile id, must be stable).
- internal/telemetry/recorder.go:12-130 — closed event catalogue (docs/observability.md lists fields); player = hash(Email).
- cmd/tcgreplay/main.go:76-79,167-169,212 — config.reviewer from TCGREPLAY_REVIEWER; only subcommand is `sets`.
- cmd/tcgreplay/hosted.go:121-137,218-225,319-340,489 — hostedAuth, reviewerText, Registry Reviewer wiring, start-up log line.
- cmd/tcgreplay/hosted_test.go:369-410,749-770 — googleClientIDAt expects /authorize → accounts.google.com; startHostedWith uses FakeGoogle.
- cmd/tcgreplay/hosted_cap_test.go — TestHostedReviewerToolCallsPastCap (model for the password-account cap test).
- .do/app.yaml:166-173 — TCGREPLAY_REVIEWER SECRET entry to replace.
- web/src/lib/session.js, web/src/App.svelte:218-222,437-439 — board account display.
- site/src/content/docs/sign-in.md — says Google's chooser opens with "no tcgreplay page in between".
- site/src/pages/docs/connect.astro:27,31,43,70,88,106 — "Google asks you to choose an account" steps.
- site/src/pages/privacy.astro:74-93 — sign-in data and the `tcgreplay_login_…` cookie ("only while a Google sign-in is in progress").
- site/e2e/connect.spec.js:155-162 — pins "Google's account chooser" wording on /docs/sign-in.
- plugin/listing.json:23-32, plugin/demo-script.md:9-20, plugin/SUBMITTING.md:22-37 — reviewer materials naming Google and TCGREPLAY_REVIEWER.
- go.mod/go.sum — no golang.org/x/crypto or x/term today; go 1.27.2.

## External references

- OpenAI plugin review requirements (via knowledge `learnings/publishing-to-chatgpt.md` [S5][S6]): reviewer credentials without MFA; "reviewers cannot connect" is the most common rejection. This is why the feature exists.
- claude.ai connector OAuth requirements (knowledge `learnings/claude-ai-connector-oauth.md`): the /authorize → code → /token contract that must not change; 10 s endpoint budget.
- golang.org/x/crypto/bcrypt: `CompareHashAndPassword`, `Cost`; 72-byte password limit; accepts 2a/2b/2y prefixes. Chosen hash.
- Apache `htpasswd -nBC 12 <user>`: prompts without echo and prints `user:$2y$12$…`, the config entry format.
- Google Sign-In branding guidelines: the "Sign in with Google" button should use Google's "G" mark and standard label.
- OWASP Authentication Cheat Sheet: generic failure messages, timing-equal checks for unknown users (compare against a dummy hash), throttling per account and per client.

## Prior plans / specs consulted

- Spec 20261122190321-c6d9d90a-sign-in-page (this plan's spec) and its working-context notes (interview answers 2026-11-23).
- Knowledge `gotchas/cap-bypass-needs-lost-place.md`: a cap bypass on tool calls is only tested after the place is lost. The password-account cap test must follow `TestHostedReviewerToolCallsPastCap`.
- Knowledge `gotchas/app-spec-not-applied-by-deploy.md`: env values go in the console; a bad value rolls back; never paste the whole spec.
- Knowledge `gotchas/disconnect-does-not-reach-playmat.md`: sealed grants outlive restarts, which is why removing an account must be checked when a grant opens.
- Knowledge `gotchas/go-sdk-sessions-and-auth.md`: look account state up per request from TokenInfo.
- Knowledge `gotchas/live-site-cf-bm-cookie.md`: Cloudflare edge in front of App Platform (relevant to client IP).
- Knowledge conventions: whats-new-entries (player-visible change → entry), privacy-page-tracks-outside-services, signed-off-design-wins (Studio tokens; the sign-in page is not drawn in the Studio design).

## Open assumptions

- Google account `sub` values never start with `password:`. Google documents `sub` as an opaque ASCII string, in practice all digits. If wrong, ID separation needs a stronger scheme: STOP and ask.
- DigitalOcean App Platform passes `$2y$12$…` in an encrypted env value literally. It interpolates only `${…}` bindable variables. Verify with the start-up log's "N password accounts" after setting it.
- The client address for per-client throttling is `do-connecting-ip`, else `CF-Connecting-IP`, else the TCP peer. App Platform's edge (Cloudflare) sets these and overwrites any value a client sends. Unverified on the live app. If neither header is present, all traffic shares the peer address of the edge, so per-client throttling would bite every user at once: STOP and ask if a deploy check shows this.
- The 72-byte bcrypt limit is acceptable: operator-generated passwords are short random strings.
- Saving the new env value in the console restarts the app and drops in-memory games (expected, per the gotcha).

## Drafting assumptions

### Chosen direction: one sign-in page at the shared start point, providers ending in one completion (architecture)
- **Decision**: Replace `startGoogle` at both entry points with `startSignIn` (pending sign-in + login cookie at path `/auth/` + redirect to `GET /auth/signin?state=`). Google starts from `GET /auth/google?state=` and password from `POST /auth/password`, and both end in a shared `finishSignIn` (the current callback tail). `Account` gains `Provider`, sealed into grants and token extras. Password accounts come from `TCGREPLAY_PASSWORD_ACCOUNTS` (bcrypt), are throttled by the extended token bucket, and get reserved places in place of `TCGREPLAY_REVIEWER`.
- **Rationale**: Reuses the existing pending sign-in, cookie check, completion and limiter. The OAuth contract is untouched. A new provider is one table entry.
- **Rejected**: Inline rendering at each entry point (state handling duplicated); a separate password login flow (second completion); a local fake OIDC provider standing in for Google (High effort, needs a second token exchange for no gain).

### Sign-in page styling (architecture)
- **Decision**: Studio colour tokens and mark (embedded from `internal/design`), light interface only, IBM Plex Sans named in the font stack but not loaded from Google Fonts. The existing problem, busy and signed-out pages move onto the same template styling.
- **Rationale**: The Studio design does not draw the page. The website has no dark theme. Not loading fonts keeps the privacy page's outside-services list true.
- **Rejected**: Leaving the old plain template (off-brand on every player's path); loading IBM Plex from Google Fonts (a new outside resource on the hosted app's own pages).

### Conventions selected (architecture)
- **Decision**: Applied whats-new-entries, privacy-page-tracks-outside-services and signed-off-design-wins. Dropped board-never-scrolls (no board layout change), fast-design-iteration (workflow, not design) and skills-text-and-packaging (no guide changes).
- **Rationale**: Only these three bear on a new player-visible auth page.
- **Rejected**: Listing every convention.

### Planned the sign-in-page spec without asking (overview)
- **Decision**: Planned 20261122190321-c6d9d90a-sign-in-page.
- **Rationale**: It is the only spec without a plan, finalised today.
- **Rejected**: Asking the user to pick from a one-item list.

### bcrypt via golang.org/x/crypto (discovery)
- **Decision**: Hash configured passwords with bcrypt (cost ≥ 10, 12 recommended), adding golang.org/x/crypto as a dependency. Operators generate entries with `htpasswd -nBC 12 <username>`.
- **Rationale**: Deliberately slow and salted, with a self-describing format. Its alphabet has no `,` or `:`, so `user:hash` entries fit a comma-separated env value, and htpasswd prints the exact entry.
- **Rejected**: argon2id (PHC string contains commas); stdlib PBKDF2 (no standard format, needs our own generator); a hash-password subcommand (needs x/term for no-echo input).

### Password accounts revoked when removed or re-passworded (discovery)
- **Decision**: Sealed board cookies and tokens for a password account carry a fingerprint of its configured hash. Opening one checks that the username is still configured with the same hash, and refuses it otherwise.
- **Rationale**: The spec lets the operator remove an account by config alone. Sealed grants survive restarts for up to 30 days, so removal would not take effect without this check.
- **Rejected**: Leaving old grants valid until they expire.

### Usernames may be a plain name or an email address (discovery; revised at walkthrough)
- **Decision**: Usernames are 1–254 characters of `[a-z0-9._+@-]`, compared after lower-casing, so `openai-reviewer` and `reviewer@example.com` both work. Account ID is `password:<username>`, Account.Email holds the username for display, and telemetry uses `Account.PlayerKey()` (`password:<username>`; Google keeps its email).
- **Rationale**: The user asked whether ChatGPT's submission needs an email as the username. OpenAI's submission and review pages (developers.openai.com/plugins/deploy/submission.md and app-review.md, read 2026-11-23) ask for "a dedicated test account, login URL, workspace or tenant details, and sign-in instructions" with no MFA, and say nothing on the username's format. Allowing both works with whatever the dashboard form expects. Separation from Google rests on the provider (ID prefix, provider-keyed telemetry, explicit admin refusal), not on the username's shape.
- **Rejected**: Plain names only (an earlier draft, which relied on "no `@`" for separation and would fail if the form requires an email); email-only usernames (an unnecessary restriction).

### TCGREPLAY_REVIEWER ignored with a warning (discovery)
- **Decision**: Remove the setting. If it is still set at start, log a warning that it is no longer used and start normally.
- **Rationale**: The spec says retire it and keep the app starting cleanly.
- **Rejected**: Refusing to start (would roll back the deploy); continuing to honour it.

### Throttle limits (discovery)
- **Decision**: Per username, 10 failures per 15 minutes (a token bucket of 10 refilled over 15 minutes). Per client address, 30 failures per 15 minutes. A throttled attempt is refused before the password is checked, even if the password is right.
- **Rationale**: Meets the acceptance criterion (at most 10 per username in 15 minutes, plus a per-client limit across usernames) by extending the existing bucket.
- **Rejected**: A separate lockout mechanism.

### Telemetry carries the provider (data_structures)
- **Decision**: `sign_in` records gain a `provider` field. Failed password attempts are `sign_in_refused` with reason `credentials` or `throttled` and no player.
- **Rationale**: The spec's success metric compares Google sign-in completions before and after release, which needs Google sign-ins told apart from password ones. Failures carry no player so that a password typed into the username field never becomes a player ID.
- **Rejected**: No provider field (metric unmeasurable); recording failures under the attempted username's player ID.

### Task split and executors (tasks)
- **Decision**: 11 agent tasks across 3 milestones plus 1 human task (console and OpenAI dashboard). Restyling, docs, the throttle and the board label are separate small tasks so they can run in parallel with the core flow.
- **Rationale**: Keeps the High-complexity flow change (page in front of Google, with the test-helper update) isolated. Each acceptance criterion maps to one task.
- **Rejected**: One task per milestone (too large to verify); making the live configuration an agent task (needs console access and secrets).

### Minimum bcrypt cost 10 (tasks)
- **Decision**: Parsing refuses hashes below cost 10. Tests generate at cost 10. The dummy hash for unknown usernames uses the highest configured cost.
- **Rationale**: Keeps hashes "deliberately slow" without a test-only bypass. The timing of unknown and known usernames matches.
- **Rejected**: A test-only parse option (an extra code path that could leak into production).

### Login cookie path widened to /auth/ (tasks)
- **Decision**: The per-state login cookie moves from path `/auth/google/callback` to `/auth/`. The callback also clears a cookie left at the old path by a sign-in in flight at deploy time.
- **Rationale**: The page, the Google start, the password post and the callback all need it.
- **Rejected**: Separate cookies per route (more state for no gain).

## Rehydration cues

- `spektacular spec file read 20261122190321-c6d9d90a-sign-in-page`
- `spektacular knowledge read` for repo/tcgreplay `gotchas/cap-bypass-needs-lost-place.md`, `gotchas/app-spec-not-applied-by-deploy.md`, `learnings/claude-ai-connector-oauth.md`.
- Re-read `internal/hostauth/google.go`, `oauth.go:258-336`, `board.go`, `seal.go:76-108`, `limit.go`, `store.go:13-170`, `pages.go`, `hostauthtest/client.go`; `internal/usergames/usergames.go:30-182`; `cmd/tcgreplay/hosted.go:121-340`.
- `grep -rn "accounts.google.com\|startGoogle\|FollowSignIn\|BoardCookie" internal cmd` to find every test that assumes a direct Google redirect.
