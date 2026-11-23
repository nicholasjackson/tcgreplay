---
created_date: "2026-11-23"
document_status: final
closed_date: "2026-11-23"
---

# Plan: 20261122190321-c6d9d90a-sign-in-page

<!-- Metadata -->
<!-- Created: 2026-11-23T09:19:58Z -->
<!-- Commit: b6f94e6 -->
<!-- Branch: studio-design-alignment -->
<!-- Repository: git@github.com:nicholasjackson/tcgreplay.git -->

## Overview

TCG Replay's hosted playmat gets its own sign-in page, shown whenever an assistant connects or a signed-out browser opens the board. It offers "Sign in with Google" for everyone and a username and password form for a few operator-configured accounts. This lets directory reviewers at OpenAI and Anthropic test TCG Replay with credentials the operator gives them, without a Google account that Google's security checks could block. Players keep signing in with Google to the same account and games, and the page leaves room for more providers later. Password accounts are hashed, throttled, kept apart from Google accounts, never admins, and get the reserved place the single Google reviewer had. The reviewer materials are updated to use them.

## Conventions

- **Every player-visible change ships with a What's new entry** — the sign-in page is on every player's path to Google, so `site/src/data/whats-new.yaml` gets an entry (`refresh: false`: no tool, description or instruction changes).
- **Update the privacy page with any new outside resource or remembered setting** — the sign-in page must not load Google Fonts or any other outside resource (it names IBM Plex only in its font stack). The privacy page's `tcgreplay_login_…` cookie line ("only while a Google sign-in is in progress") is reworded for any sign-in, and the data section notes the reviewer password accounts.
- **The signed-off design canvas wins** — the Studio design does not draw a sign-in page, so the page builds from Studio's tokens, mark and light interface rather than inventing a look. Any departure is flagged at the walkthrough.

## Architecture & Design Decisions

All work lands in the one registered repo, `tcgreplay`, mostly in `internal/hostauth`.

**One page where every sign-in starts.** Today both entry points, the connector's `GET /authorize` (`internal/hostauth/oauth.go:258-298`) and the board's `GET /auth/login` (`internal/hostauth/board.go:20-27`), call `startGoogle` (`google.go:41`), which redirects straight to Google. That call becomes `startSignIn`. It records the sign-in in progress exactly as now (a `pendingLogin` with its kind, its `next` path or its connector request, and a 10-minute life), ties it to the browser with the per-state login cookie, and redirects to the TCG Replay sign-in page at `GET /auth/signin?state=…`. The login cookie's path widens from `/auth/google/callback` to `/auth/` so that the page, the password form and the Google callback can all read it. The page is rendered from the pending sign-in. It shows a "Sign in with Google" button (`GET /auth/google?state=…`, which adds the nonce and PKCE verifier to that same pending entry and then redirects to Google as `startGoogle` does now). It shows the username and password form (`POST /auth/password`) only when password accounts are configured. Under the form is the note that it is for reviewer and invited accounts. Every route checks that the state matches this browser's login cookie, as the Google callback already does. This also makes a cross-site form post fail (the cookie is `SameSite=Lax`). The connector's OAuth contract (registration, `/authorize` parameters and checks, PKCE, `/token`) does not change: only what the browser sees between `/authorize` and the redirect back with a code changes, as the spec's constraint requires.

**Providers end in one completion.** The tail of `serveGoogleCallback` (`google.go:94-111`) becomes `finishSignIn(w, r, pending, account)`. It runs `signIn` (cap check through `Places.Admit`, board session cookie, telemetry), then continues the flow by kind: `continueConnector` issues the code to the assistant, and a board sign-in redirects to its `next` path. Google's callback and the password handler both end there. That is why one password sign-in covers connecting and the board, just as a Google sign-in does today. The page lists its button providers from a small table (`id`, label, start route). Google is the only entry now, and a later Facebook is one more entry plus its callback, so neither the password nor the Google code is reworked. Password sign-in is a form rather than a button, so it is a separate, optional block on the page and not a table entry.

**Password accounts: configured, hashed, throttled, and kept apart.** Accounts come from one encrypted env value, `TCGREPLAY_PASSWORD_ACCOUNTS`: comma- or newline-separated `username:bcrypt-hash` entries. This is the format `htpasswd -nBC 12 <username>` prints. It is read at start-up beside the other hosted secrets (`cmd/tcgreplay/hosted.go:121-137`). A malformed entry, a hash that is not bcrypt (so a plain-text password is refused), a duplicate username or an invalid username stops start-up with an error that never echoes the value. Unset means no accounts, and the page then shows only Google. A username is either a plain name or an email address (1–254 characters of letters, digits, `.`, `_`, `-`, `+` and `@`, matched case-insensitively). OpenAI's review form asks for a "username" without saying it must be an email, and an email-shaped username covers a form that does. Its shape therefore guarantees nothing about separation from Google, and separation rests on the provider instead. A password account signs in as `Account{ID: "password:<username>", Email: <username>, Provider: "password"}`. Google `sub` values are numeric, so the `password:` prefix keeps the account ID, board handle (`usergames.Handle`), registry entry and board session disjoint from every Google account's. That holds even when the username is the same email as a Google account. Telemetry names players by a keyed hash of what it is handed. Every recorder call therefore passes the account's player key (`Account.PlayerKey()`: the email for Google, unchanged so existing player IDs stay stable, and `password:<username>` for a password account), never the bare email. `IsAdmin` refuses any non-Google account outright, on top of its verified-email rule, so an email-shaped username listed in `TCGREPLAY_ADMINS` still gets no admin rights. `Account` gains a `Provider` field that rides everywhere the account does. It is sealed into the board cookie and both tokens as an optional claim (absent means Google, so every existing sign-in keeps working). It is put into `TokenInfo.Extra` by `verify` and read back by `AccountFromToken`, so the reserved place holds on tool calls too, not only at sign-in. A password grant also carries a short fingerprint of the account's configured hash. Opening any sealed grant for a password account checks that the username is still configured with that hash. Removing an account or changing its password in the console therefore revokes its cookies and tokens at the next restart, which saving settings causes anyway. Failed attempts go through the existing token-bucket limiter (`limit.go:37-65`), extended to take a capacity and a refill window. The limits are 10 failures per 15 minutes per username and 30 per 15 minutes per client address. The handler checks both buckets before it checks the password, so a throttled attempt is refused even with the right password. Each failure spends from both buckets. An unknown username is still compared against a dummy bcrypt hash, so its timing and message ("That username and password don't match.") are the same as for a wrong password. Neither the password nor the attempted username is ever logged or recorded. A successful sign-in records `sign_in` with `provider: password` under the username's player ID.

**Reserved places move from one email to every password account.** `usergames.Registry` drops `Config.Reviewer` and its verified-email match (`usergames.go:164`). An account whose provider is `password` is admitted at the cap and is not counted against `MaxUsers`. `TCGREPLAY_REVIEWER` is retired: if it is still set, start-up logs a warning and carries on. The start-up line reports "N password accounts" in place of "reviewer account set". The page uses the Studio colour tokens and the mark from `internal/design` (`tokens.css` and `mark.svg`, embedded), in the light interface. The website has no dark theme in the Studio design. Its font stack names IBM Plex Sans without loading Google Fonts, so it adds no outside resource (privacy-page convention). The Google button is the first, primary action and follows Google's button branding. The existing problem, busy and signed-out pages move onto the same styling. The board's account tooltip stops saying "Signed in with Google" for password accounts (`/auth/session` adds `provider`). Player-facing docs that promise "no tcgreplay page in between", the privacy page's login-cookie line, a What's new entry, and the reviewer materials (`plugin/listing.json` test cases, `plugin/demo-script.md`, `plugin/SUBMITTING.md`) are updated in the same change. This direction reuses the existing pending sign-in, cookie, completion and limiter, so the new surface is one page, two small handlers and an account list. A separate login flow, inline rendering at each entry point, or a local fake identity provider would each duplicate the completion or the state handling (see `research.md#alternatives-considered-and-rejected`).

## Component Breakdown

- **Sign-in start (changed, hostauth).** Replaces the direct-to-Google start used by both entry points (the connector's authorization step and the board's sign-in). It records the sign-in in progress, ties it to the browser with the per-state login cookie (now scoped to every sign-in route), and sends the browser to the sign-in page. The connector's authorization checks before it are unchanged.
- **Sign-in page (new, hostauth).** Renders the TCG Replay sign-in page for a sign-in in progress: the provider buttons (Google today), the username and password form when password accounts are configured, the reviewer-and-invited note, and an error line (wrong credentials, too many attempts). Styled from the shared design tokens and mark. It shares its template with the existing problem, busy and signed-out pages, which move to the same look.
- **Provider table (new, hostauth).** The list of button providers the page shows, each with an id, a label and a start route. Google is its one entry. Adding Facebook later means adding an entry and its callback handler, and nothing else changes.
- **Google provider (changed, hostauth).** Its start route takes an existing sign-in in progress (by state, checked against the login cookie), adds the nonce and PKCE verifier, and redirects to Google. Its callback keeps its checks and hands the confirmed account to the shared completion.
- **Password provider (new, hostauth).** Takes the form post for a sign-in in progress. It checks the login cookie and state, applies the failure throttles, verifies the username and password against the configured accounts (equal timing and an identical message for an unknown username or a wrong password), then hands the password account to the shared completion. A failure re-renders the page without using up the sign-in in progress.
- **Password accounts (new, hostauth).** The operator's configured accounts: parsing and validating the configured list (usernames, bcrypt hashes, no duplicates, no plain text), verifying a password, building the signed-in account (its own `password:` identity space, whether the username is a plain name or an email address), and answering whether a sealed grant for a password account is still valid (username still configured, same hash fingerprint).
- **Shared sign-in completion (changed, hostauth).** Extracted from the Google callback's tail. It applies the user cap through the places provider, starts the board session, records the sign-in (with its provider), then either issues the connector's one-time code back to the assistant or returns the browser to the board page it asked for. Every provider ends here.
- **Account and sealed grants (changed, hostauth).** The signed-in account gains its provider. Board cookies, access tokens and refresh tokens carry the provider and, for password accounts, the credential fingerprint. Grants sealed before this change open as Google. Token verification exposes the provider to tool calls. Opening a password grant consults the password accounts and refuses one that has been removed or re-passworded.
- **Failure throttle (changed, hostauth limiter).** The existing per-key token bucket gains a capacity and refill window (today it is fixed at a per-minute rate), so it can hold 10 failures per 15 minutes per username and 30 per client address. It also gains a check-without-spending for the throttled-before-verify rule. The request rate limit keeps its behaviour.
- **Admin guard (changed, hostauth).** Refuses any account that is not a verified Google admin, so password accounts never pass.
- **Board session info (changed, hostauth + board).** The session endpoint also reports the provider. The board's account label stops claiming "Signed in with Google" for password accounts.
- **Places registry (changed, usergames).** The reserved place moves from the single configured reviewer email to every password account, at sign-in and on tool calls. The reviewer setting is removed.
- **Hosted wiring (changed, cmd/tcgreplay).** Reads `TCGREPLAY_PASSWORD_ACCOUNTS` and hands the parsed accounts to the sign-in service. It warns about and ignores a leftover `TCGREPLAY_REVIEWER`, and reports the password-account count (never names) in the start-up line. The app spec declares the new secret in place of the old one.
- **Test sign-in helpers (changed, hostauthtest).** The fake-Google browser helpers click through the sign-in page's Google button, so existing suites keep driving Google sign-in. New helpers sign in with a username and password for the connector and the board.
- **Player docs, privacy page and What's new (changed, site).** The sign-in docs and connecting steps describe the TCG Replay page before Google. The privacy page rewords the login cookie and mentions reviewer password accounts. A What's new entry announces the page.
- **Reviewer materials (changed, plugin).** Test cases, demo script and submission checklist tell the reviewer to sign in with the configured reviewer account and explain how the operator creates it.

## Data Structures & Interfaces

**`hostauth.Account` (changed)**: the signed-in account every request acts for. It gains its provider, with a helper that the places registry and the admin guard ask.

```go
type Account struct {
    ID            string // Google "sub", or "password:<username>"
    Email         string // Google email, or the username for a password account
    EmailVerified bool   // Google only; always false for a password account
    Provider      string // "google" or "password"; "" reads as "google"
}
func (a Account) Password() bool   // Provider == "password": reserved place, never admin
func (a Account) PlayerKey() string // what telemetry hashes into the player ID: Email for Google (unchanged), "password:<username>" otherwise
```

**`hostauth.Config` (changed)**: gains the configured password accounts. Nil or empty means the page shows Google only.

```go
PasswordAccounts *PasswordAccounts
```

**`hostauth.PasswordAccounts` (new)**: the operator's accounts, parsed once at start-up. Its methods are the only way the rest of hostauth reaches usernames and hashes.

```go
func ParsePasswordAccounts(raw string) (*PasswordAccounts, error) // "user:$2y$12$…" entries, comma/newline separated; error never echoes the value
func (p *PasswordAccounts) Len() int
func (p *PasswordAccounts) Check(username, password string) (Account, bool) // constant-shape: dummy compare for unknown usernames
func (p *PasswordAccounts) Fingerprint(username string) (string, bool)        // short hash of the configured hash, sealed into grants
```

**`grantClaims` (changed, sealed wire format)**: the claims sealed into the board cookie, access tokens and refresh tokens gain two optional fields. Both are omitted for Google, so every grant sealed before this change opens unchanged, as Google.

```go
P string `json:"p,omitempty"` // provider; absent = google
K string `json:"k,omitempty"` // password accounts: credential fingerprint
```

**`auth.TokenInfo.Extra` (changed, MCP boundary)**: `verify` adds `"provider"` beside `"email"` and `"verified"`, and `AccountFromToken` reads it back. A token without it reads as Google.

**`hostauth.Places` (unchanged interface, changed meaning)**: `Admit(acct)` is now asked for every provider's sign-in, not only Google's.

**`usergames.Config` (changed)**: `Reviewer string` is removed. The registry reserves a place for any account with `acct.Password()`.

**Sign-in routes (new HTTP surface; the connector OAuth endpoints are unchanged)**:

| Route | Role |
|---|---|
| `GET /auth/signin?state=S` | the sign-in page for sign-in S (login cookie must match) |
| `GET /auth/google?state=S` | start Google for S → Google's chooser |
| `POST /auth/password` (form: `state`, `username`, `password`) | password sign-in for S → shared completion, or the page again with an error (401, or 429 when throttled) |
| `GET /auth/google/callback` | unchanged |
| `GET /auth/session` | gains `"provider"` in its JSON |

**Provider table (new, internal)**: what the page renders as buttons.

```go
type buttonProvider struct{ ID, Label, StartPath string } // Google: {"google", "Sign in with Google", "/auth/google"}
```

**Throttle bucket (changed, internal)**: `store.take` gains a capacity and refill window per call (the request limit passes `limit` per minute as now), plus a non-spending `peek`. Bucket keys are namespaced (`acct:<id>`, `login-user:<username>`, `login-client:<addr>`).

**Telemetry `sign_in` (changed record)**: gains a `provider` field (`google` or `password`), listed in `docs/observability.md`. Failed password attempts are recorded as `sign_in_refused` with a reason (`credentials` or `throttled`) and no player.

**Configuration (new)**: `TCGREPLAY_PASSWORD_ACCOUNTS`, an encrypted runtime secret. `TCGREPLAY_REVIEWER` is removed: it is ignored with a warning if still set.

## Implementation Detail

**Sign-in becomes two-phase: start, then choose.** Today a sign-in in progress is created at the moment the browser leaves for Google. After this change it is created first, independent of any provider, and lives through the page. The provider is chosen later, against that same entry, by its state. The pending entry keeps its single-use rule: it is consumed only by a successful completion (or by Google's callback, as now), never by a failed password or a re-render of the page. Every new route opens with the same guard the Google callback already uses: the state must be present, must match this browser's per-state login cookie, and must name a live pending entry. Otherwise the existing "start signing in again" problem page is shown. Developers reading the code will see one guard helper used by the page, the Google start, the password post and the callback, rather than four hand-rolled checks.

**One completion, many providers (a new seam).** The callback's tail is lifted into a shared completion that takes the pending entry and a confirmed account. Providers only establish who the account is: Google by its ID token, the password provider by a hash check. Everything after (cap admission, board session, telemetry, connector code or board redirect, busy page) happens once, in one place. This follows the existing pattern where `signIn` was already the single place the user cap is applied for both flows. It extends it to "the single place a sign-in finishes". Button providers are data (a small table the page ranges over), so the page template has no provider-specific branches beyond the optional password block.

**Accounts carry their provider end to end.** The provider joins the account the way the verified-email flag did in an earlier change. It is sealed into every grant as an optional claim and copied into token extras for tool calls, and older grants without it read as Google. This is the existing "add an optional sealed claim, default the absent case" pattern, reused. New: opening a password grant asks the password accounts whether the grant is still valid (username present, fingerprint equal). The store gets that question as a small hook from the sign-in service, so it never holds usernames or hashes itself. Google grants skip the hook entirely.

**Password accounts are a sealed-off module.** Parsing, validation, verification and fingerprinting live together behind a small type. The rest of hostauth sees only accounts and yes/no answers. No other code can read a hash, and no error message or log line quotes the configured value or an attempted credential. Verification has a constant shape: it always runs one bcrypt comparison (against a fixed dummy hash for unknown usernames) and returns one boolean, so the caller cannot accidentally write a branch that leaks which part failed.

**The limiter grows a shape parameter, not a twin.** The existing token bucket is generalised from "N per minute" to "capacity N refilled over window W", with sweeping based on each bucket's own window. A non-spending check is added. The request rate limit calls it with its current per-minute values and behaves as before. The password provider makes two checks (username and client) before verifying and spends from both on failure. Keys are namespaced so request and login buckets never mix.

**The page is a second template beside the first.** The existing tiny `html/template` page stays the pattern: inline CSS, no scripts, no outside requests, `no-store`, and now also refusing to be framed. The sign-in page joins it, and both take their colours and mark from the shared design package, so the hosted app's server-rendered pages finally match the board and website. The template stays deliberately simple: one column, the Google button first and most prominent, the form below it behind a divider, the note under the form.

**Tests move with the flow, not around it.** The fake-Google browser helpers learn to follow the page's Google button (found by a stable `data-provider` attribute), so the large existing body of sign-in tests keeps exercising Google unchanged in intent. Only assertions that pinned "the first redirect goes to Google" are updated to expect the page. Password helpers mirror the Google ones (connect, board cookie, MCP transport), so new tests read like the existing ones.

## Dependencies

- **Design documents this plan was built on: none.** The spec carries no design references. The Studio design (`studio/design.md` from the `design` source) was read for the colour roles and mark the page reuses, but it does not draw a sign-in page.
- **`golang.org/x/crypto/bcrypt` (new external module).** Slow, salted password hashing and comparison. Added to `go.mod`. It must be present before the password-accounts work.
- **`internal/hostauth` (changed).** Sign-in start, page, providers, completion, sealed grants, limiter, admin guard. Most of the work.
- **`internal/hostauth/hostauthtest` (changed).** The fake-Google browser helpers must click through the page before any existing sign-in test can pass again, so they change in the same task as the page.
- **`internal/usergames` (changed).** The reserved place moves to password accounts. Depends on `Account.Password()` existing.
- **`internal/design` (changed, small).** Exports the embedded `tokens.css` and `mark.svg` for the server-rendered pages. No token values change.
- **`internal/telemetry` and `docs/observability.md` (changed, small).** A `provider` field on records, and the catalogue documents it. The recorder itself is unchanged, but every caller in hostauth and usergames passes `Account.PlayerKey()` in place of `Account.Email`.
- **`cmd/tcgreplay` (changed).** Env parsing, start-up log and wiring; the hosted end-to-end tests.
- **`web/` board (changed, small).** The account label reads the provider from `/auth/session`. Needs a board rebuild (`internal/webui/dist`).
- **`site/` website (changed).** Sign-in and connect docs, privacy page, What's new. Needs a site rebuild (`internal/site/dist`) and the site browser suite.
- **`plugin/` reviewer materials (changed).** `listing.json` test cases, `demo-script.md`, `SUBMITTING.md`. These files hold uncommitted, unrelated edits on the current branch, so changes must be merged into them, not overwritten.
- **DigitalOcean App Platform (operator action).** The `TCGREPLAY_PASSWORD_ACCOUNTS` value is set in the console, encrypted. It is optional, so it can be set before or after the push. `TCGREPLAY_REVIEWER` is deleted from the console once the reviewer has a password account. `.do/app.yaml` records the change but does not apply it (knowledge gotcha `app-spec-not-applied-by-deploy`).
- **Prior plans/specs (already landed, no changes needed).** google-login (the OAuth server and Google flow this extends), per-user-games (registry and places), chatgpt-submission-prep (reviewer exception, `get_account`), observability (telemetry catalogue). Nothing must land before this plan starts.

## Testing Approach

**Shape.** Most coverage is Go tests in the existing style: hostauth unit and integration tests through `httptest` with the fake Google and a cookie-jar test browser, plus hosted end-to-end tests that start the real hosted handler and connect an MCP client through the library's own OAuth client. The hostauth package carries the risk (a security boundary and the connector contract), so it gets the deepest coverage. Small additions go to the site browser suite (docs wording) and the board suite (account label). No new test framework.

**Regression first.** The existing sign-in suites (Google flow, connector OAuth, board sessions, cap, admin, verified flag, events, rate limit, hosted end-to-end) keep running through the fake-Google helpers, which now click the page's Google button. These suites are the guarantee that "Google sign-in works as before" and that the connector contract did not change. Registration, `/authorize` checks, PKCE, code and token exchange, refresh rotation, and restart survival with a signing key are all still asserted. Only assertions that the first redirect goes to Google change, to expect the sign-in page.

**Load-bearing assertions** (one or more tests each, mapped to the acceptance criteria):
- Connecting and opening a board signed out both land on the TCG Replay sign-in page, not Google. A grant sealed before the change (no provider claim) still opens as the same Google account.
- Password sign-in completes the connector flow (code → tokens → tools run as `password:<username>`). The same browser then opens the board without signing in again. A board sign-in returns to the exact page asked for.
- Two configured accounts each sign in, and a game imported by one is invisible to the other.
- With no accounts configured the page has the Google button and no username or password input. The page never has sign-up, change-password or recovery controls, and it always shows the reviewer note.
- An unknown username and a wrong password produce byte-identical failure pages and status, and no session cookie.
- The 11th failure for one username inside 15 minutes is refused even with the correct password, with a "try again later" page. Failures spread over many usernames from one client are refused after the client limit. A settable clock shows both limits recover.
- At the player cap, a password account signs in and imports a game. One below the cap, a password account signs in and a new Google player can still join afterwards. The tool-call path is tested after the place is lost (idle out, place taken by another account), following the knowledge gotcha on cap bypasses.
- A Google account whose email equals a password account's email-shaped username (or whose local part equals a plain username) shares no game, board handle, session or telemetry player ID with it, and a browser signed in as one is never signed in as the other.
- A password account is refused by every admin page and action.
- A removed or re-passworded account's existing board cookie and tokens are refused after a restart with the same signing key.
- Configuration parsing rejects plain-text passwords, malformed entries, bad usernames and duplicates, without echoing the value. Captured logs and telemetry after successful and failed sign-ins contain neither the password nor the attempted password. A password typed into the username field is not logged either.
- Cross-site or cookie-less form posts are refused. The page and form responses are `no-store` and refuse framing.
- Start-up with a leftover `TCGREPLAY_REVIEWER` logs a warning and starts. The start-up line names the password-account count, never usernames.

**Docs and materials.** The site browser suite's existing check on the sign-in docs is updated to the new wording (the TCG Replay page, then Google). The What's new data build check covers the new entry. The reviewer-materials acceptance criterion is checked by a test over the plugin files: no "sign in with Google" instruction remains in the review test cases, demo script or checklist, and each names the configured reviewer account.

**Deliberate gaps.** No browser test drives real Google, ChatGPT or Claude: the fake Google and the MCP library's OAuth client cover the protocol. No load test of bcrypt cost.

**Success metrics.**
- *OpenAI's plugin review signs in with the configured reviewer account and runs the test cases without a "reviewers cannot connect" rejection.* **Manual — captured in the implementation test plan.** (The automated equivalent, a password account connecting through the real OAuth flow and running tools, is a behavioural test above.)
- *Claude's directory review can be given its own configured account without any code change.* Behavioural test: two accounts parsed from configuration alone, each signing in separately. Adding the real account in the console is **Manual — captured in the implementation test plan**.
- *Google sign-in completions in the two weeks after release stay at the level of the two weeks before, and no player reports of lost games or accounts.* **Manual — captured in the implementation test plan** (production telemetry: `sign_in` records with `provider: google`, which a behavioural test guarantees are emitted). **Depends on log forwarding.** App Platform sends only metrics to DigitalOcean Insights, not logs (knowledge gotcha `app-platform-logs-only-in-insights`), so today the `sign_in` records are not kept anywhere they can be counted over two weeks. Log forwarding is handled separately from this plan (it needs a provider choice and is billed). If it is not in place before release, the two-week comparison falls back to watching the live Runtime Logs and player reports, and the test plan must say so.

**Manual reviews.**
- Sign-in page look and copy against the Studio tokens, at phone and desktop width; Google button branding. **Manual — captured in the implementation test plan.**
- Real Google sign-in on the deployed playmat from ChatGPT and MCP Inspector, with an existing player keeping their games across the release. **Manual — captured in the implementation test plan.**
- After setting `TCGREPLAY_PASSWORD_ACCOUNTS` in the console: the start-up line reports the account count, a password sign-in works on the live site, and per-client throttling keys on real client addresses rather than the edge's. **Manual — captured in the implementation test plan.**

## Milestones & Tasks

### Milestone 1: Everyone signs in through a TCG Replay sign-in page

**What changes**: When an assistant connects TCG Replay, or a signed-out browser opens the board, the browser now shows TCG Replay's own sign-in page, in the Studio look, with a prominent "Sign in with Google" button, where it used to jump straight to Google's account chooser. Choosing Google behaves exactly as before: the same account, the same games, the connection completes, and the board opens signed in. The problem, busy and signed-out pages take the same look. The website's sign-in and connecting docs describe the new page, and a What's new entry announces it. With no password accounts configured (the state this milestone ships in), the page shows only the Google button, which is already the spec's behaviour for that case.

**Validation point**: Every existing Google sign-in suite passes through the page via the updated test helpers. New tests show both entry points land on the page, the page has no form when no accounts are configured, and old sealed sign-ins still open. The site browser suite passes with the new docs wording.

#### - [ ] Task: Sign-in page in front of Google
**Id:** 40150ba2-ff2b-4ea1-ad7e-1f49d3ce93dc
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Both ways a sign-in starts (an assistant connecting, a signed-out browser opening the board) record the sign-in in progress and show TCG Replay's sign-in page, where they used to redirect straight to Google. The page's "Sign in with Google" button starts Google for that same sign-in, and Google's answer finishes through a shared completion that every future provider will also use. The test helpers that drive Google sign-in learn to click through the page, so the existing sign-in suites keep proving Google works as before.

*Technical detail:* [context.md#task-sign-in-page-in-front-of-google](./context.md#task-sign-in-page-in-front-of-google)

**Acceptance criteria**:
- [ ] An assistant starting to connect, and a signed-out browser opening a board page, are both shown the TCG Replay sign-in page, not Google.
- [ ] Choosing "Sign in with Google" and completing Google's sign-in leaves the same account signed in, completes the assistant's connection or returns to the board page asked for, and signs that browser in to the board, exactly as before.
- [ ] With no password accounts configured, the page shows the Google button and no username or password fields. It has no sign-up, change-password or recovery control.
- [ ] A page, button or callback reached with a missing, expired or other browser's sign-in shows the existing "start signing in again" problem page.
- [ ] The connector's registration, authorization checks, PKCE and token exchange behave exactly as before, and sign-ins sealed before the change still work.
- [ ] Every existing sign-in, connector, board, cap, admin and hosted test passes.

#### - [ ] Task: Studio look for the sign-in and problem pages
**Id:** cec2eb15-9867-449f-a047-e6eeee98293c
**Repo:** tcgreplay
**Depends on:**
- 40150ba2-ff2b-4ea1-ad7e-1f49d3ce93dc — Sign-in page in front of Google
**Execution:** agent

Gives the server-rendered pages (the sign-in page, and the problem, busy and signed-out pages) the Studio look: the shared colour tokens and the TCG Replay mark, light interface, with the Google button as the obvious first choice in Google's standard style. The pages load nothing from other sites and cannot be framed by another site.

*Technical detail:* [context.md#task-studio-look-for-the-sign-in-and-problem-pages](./context.md#task-studio-look-for-the-sign-in-and-problem-pages)

**Acceptance criteria**:
- [ ] The sign-in, problem, busy and signed-out pages show the TCG Replay mark and use the Studio colour tokens, readable at phone and desktop width without sideways scrolling.
- [ ] The Google button is the first and most prominent action, labelled "Sign in with Google" with Google's mark.
- [ ] The pages request no font, script, stylesheet or image from any other site, and responses tell browsers not to frame them or cache them.

#### - [ ] Task: Website docs and news entry for the sign-in page
**Id:** 30c3d86e-10cd-4a2b-a186-eefccbccb16f
**Repo:** tcgreplay
**Depends on:**
- 40150ba2-ff2b-4ea1-ad7e-1f49d3ce93dc — Sign-in page in front of Google
**Execution:** agent

Updates the website's sign-in and connecting guides, which today promise that Google's account chooser opens with no TCG Replay page in between. They now describe the TCG Replay sign-in page and its Google button. Adds a What's new entry for the change.

*Technical detail:* [context.md#task-website-docs-and-news-entry-for-the-sign-in-page](./context.md#task-website-docs-and-news-entry-for-the-sign-in-page)

**Acceptance criteria**:
- [ ] The sign-in guide and the connecting steps for each assistant say a TCG Replay sign-in page opens first and players choose "Sign in with Google" there. None still says there is no TCG Replay page in between.
- [ ] A What's new entry dated the release day announces the sign-in page, with refresh set to false.
- [ ] The website builds and its browser suite passes.

### Milestone 2: Operator-configured accounts sign in with a username and password

**What changes**: The operator can list one or more username and password accounts in the hosted configuration, as bcrypt hashes and never in plain text, with no code change. The sign-in page then shows a username and password form with a note that it is for reviewer and invited accounts. A configured account signs in exactly as a Google account does: the assistant's connection completes, its tools run as that account, the same browser opens the board signed in, and a board sign-in returns to the page asked for. Each account has its own games and identity, never shared with any Google account, and is never an admin. A wrong username and a wrong password look identical, repeated failures are throttled per username and per client, and no password ever reaches logs or telemetry. Removing an account or changing its password in the configuration revokes its existing sign-ins. The board's account label no longer claims a password account signed in with Google.

**Validation point**: Tests for every password acceptance criterion pass (connect, board, two accounts kept apart, identical failures, throttling with recovery, no crossover with a same-named Google account, admin refused, revocation after restart, config validation, logs and telemetry free of passwords). The full Go suite and the board suite pass.

#### - [ ] Task: Password accounts from configuration
**Id:** 818852b8-40c3-4519-900b-cadbd4477c2a
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Adds the self-contained piece that reads the operator's password accounts (usernames with bcrypt hashes), refuses anything malformed or in plain text, checks a username and password, and gives each account its own identity that can never equal a Google account's. It also says whether an existing sign-in for a password account is still valid, so removing an account or changing its password ends that account's sign-ins.

*Technical detail:* [context.md#task-password-accounts-from-configuration](./context.md#task-password-accounts-from-configuration)

**Acceptance criteria**:
- [ ] A configuration with several `username:bcrypt-hash` entries yields that many accounts, each checked against its own password.
- [ ] A plain-text password, a malformed entry, an invalid username or a duplicate username is refused with an error that does not contain the configured value.
- [ ] A wrong password and an unknown username both fail in the same way and take about the same time.
- [ ] Usernames may be a plain name or an email address. A password account's identity never equals a Google account's, even when the username is exactly a Google account's email.
- [ ] A sign-in made before an account was removed, or before its password changed, is reported as no longer valid.

#### - [ ] Task: Accounts carry their provider through sign-ins and tokens
**Id:** a9cbf184-13e5-4e89-8247-0a25a77c0942
**Repo:** tcgreplay
**Depends on:**
- 818852b8-40c3-4519-900b-cadbd4477c2a — Password accounts from configuration
**Execution:** agent

The signed-in account records how it signed in (Google or password), and that travels with the board cookie, the assistant's tokens and every tool call. Sign-ins made before the change still read as Google. A password account's cookie or token is refused once its account is removed or re-passworded. Telemetry names password accounts under their own player key, so a password account with the same email as a Google account never shares its player ID. Admin pages refuse any account that is not a verified Google admin. The board's session information reports the provider.

*Technical detail:* [context.md#task-accounts-carry-their-provider-through-sign-ins-and-tokens](./context.md#task-accounts-carry-their-provider-through-sign-ins-and-tokens)

**Acceptance criteria**:
- [ ] A board cookie and the assistant's tokens for a password account are recognised as that password account on page loads and tool calls, including after a restart with the same signing key.
- [ ] Cookies and tokens issued before the change still open as the same Google account.
- [ ] After a restart in which a password account was removed or its password changed, that account's existing cookie and tokens are refused.
- [ ] A password account is refused by every admin page and action, even when its username is an admin's email.
- [ ] Telemetry records for a password account never carry the player ID of the Google account with the same email.
- [ ] The board's session information names the provider.

#### - [ ] Task: Throttle for failed password sign-ins
**Id:** cf7c79f1-835f-48ae-a5cb-b5f025433235
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Extends the playmat's existing request limiter so it can hold a slower allowance (a number of failures over a window of minutes) and can be checked without spending. The password sign-in uses it per username and per client address. The existing per-minute request limit keeps working exactly as it does today.

*Technical detail:* [context.md#task-throttle-for-failed-password-sign-ins](./context.md#task-throttle-for-failed-password-sign-ins)

**Acceptance criteria**:
- [ ] An allowance of 10 over 15 minutes refuses the 11th spend inside 15 minutes and allows spending again as time passes.
- [ ] Checking an allowance without spending never uses it up.
- [ ] Allowances under different names are independent, and the existing request limit behaves exactly as before.

#### - [ ] Task: Username and password sign-in on the page
**Id:** 57986df5-7ebc-4c03-a5e2-7e99be0d3bef
**Repo:** tcgreplay
**Depends on:**
- 40150ba2-ff2b-4ea1-ad7e-1f49d3ce93dc — Sign-in page in front of Google
- 818852b8-40c3-4519-900b-cadbd4477c2a — Password accounts from configuration
- a9cbf184-13e5-4e89-8247-0a25a77c0942 — Accounts carry their provider through sign-ins and tokens
- cf7c79f1-835f-48ae-a5cb-b5f025433235 — Throttle for failed password sign-ins
**Execution:** agent

When password accounts are configured, the sign-in page shows a username and password form with a note that it is for reviewer and invited accounts. A correct sign-in finishes through the same completion as Google, so the assistant connects or the board page opens and the browser is signed in. Failures show one message whatever went wrong. Repeated failures are throttled. No password or attempted password is logged or recorded, and sign-in records now say which provider was used. Test helpers for password sign-in are added beside the Google ones.

*Technical detail:* [context.md#task-username-and-password-sign-in-on-the-page](./context.md#task-username-and-password-sign-in-on-the-page)

**Acceptance criteria**:
- [ ] With accounts configured, the page shows the username and password form and the reviewer-and-invited note. Without them it shows neither.
- [ ] A configured account's username and password complete an assistant's connection (its tools run as that account) and leave the browser signed in to the board. A board sign-in returns to the page asked for.
- [ ] An unknown username and a wrong password show the same message and status, and sign nobody in. The sign-in can be retried on the same page.
- [ ] After 10 failures for one username within 15 minutes, further attempts for it are refused for a while, even with the right password, and the page says to try again later. Failures across many usernames from one client are refused too.
- [ ] A form post from another site, or from a browser that did not start this sign-in, is refused.
- [ ] Logs and telemetry after successful and failed password sign-ins contain neither the password nor the attempted password. Sign-in records carry the provider, and the telemetry catalogue documents it.

#### - [ ] Task: Hosted playmat reads password accounts from its settings
**Id:** d111d5e9-d4e5-4cfa-bb8b-b820e6fe75b8
**Repo:** tcgreplay
**Depends on:**
- 57986df5-7ebc-4c03-a5e2-7e99be0d3bef — Username and password sign-in on the page
**Execution:** agent

The hosted playmat reads its password accounts from a new encrypted setting at start-up and refuses to start on a malformed value without printing it. It reports how many accounts are configured, never who. End-to-end tests run the real hosted playmat with configured accounts to prove the spec's password criteria: two separate accounts, no crossover with a same-named Google account, admin refused, and the board opening signed in after connecting. The desktop playmat is untouched.

*Technical detail:* [context.md#task-hosted-playmat-reads-password-accounts-from-its-settings](./context.md#task-hosted-playmat-reads-password-accounts-from-its-settings)

**Acceptance criteria**:
- [ ] With accounts set, the hosted playmat offers the password form. Unset, it offers only Google. A malformed or plain-text value stops start-up with a message that does not contain the value.
- [ ] Two configured accounts each connect and sign in, and a game imported by one is not visible to the other.
- [ ] A Google account whose email matches an email-shaped password username, or whose name matches a plain one, sees none of that account's games, and vice versa. A browser signed in as one is never signed in as the other.
- [ ] A password account is refused by the admin pages, and after connecting with it the same browser opens the board without signing in again.
- [ ] The start-up line reports the number of password accounts and never a username. The desktop playmat has no sign-in, as before.

#### - [ ] Task: Board account label for password accounts
**Id:** 68ac197d-a17b-4a0e-9dc3-d11a43596a51
**Repo:** tcgreplay
**Depends on:**
- a9cbf184-13e5-4e89-8247-0a25a77c0942 — Accounts carry their provider through sign-ins and tokens
**Execution:** agent

The board's menu shows the signed-in account with a tooltip saying it signed in with Google. For password accounts it now shows the username with a tooltip that matches how they signed in.

*Technical detail:* [context.md#task-board-account-label-for-password-accounts](./context.md#task-board-account-label-for-password-accounts)

**Acceptance criteria**:
- [ ] Signed in with a password account, the board shows the username and does not say "Signed in with Google".
- [ ] Signed in with Google, the board's account label and tooltip are unchanged.

### Milestone 3: Reviewer accounts always get in, and the review materials use them

**What changes**: Every configured password account gets the reserved place the single Google reviewer had: it can sign in and play even when the hosted playmat is full, and it does not use up one of the limited places, on tool calls as well as at sign-in. The old single-reviewer email setting is retired. If it is still set, the playmat logs a warning and starts normally. The start-up line reports how many password accounts are configured, never who. The plugin's review test cases, demo script and submission checklist tell the reviewer to sign in with the configured reviewer account, and tell the operator how to create one. The privacy page's sign-in section covers reviewer accounts and the login cookie for any sign-in. The app spec records the new secret in place of the old one.

**Validation point**: Cap tests pass: a password account gets in at the cap, a Google player still joins when one place was free, and tool calls stay admitted after the place is lost. Start-up tests cover the leftover reviewer setting and the count. The reviewer-materials check passes. The full Go, board and site suites pass.

#### - [ ] Task: Reserved places for password accounts
**Id:** e761ba87-2115-4efd-aacc-819698474941
**Repo:** tcgreplay
**Depends on:**
- a9cbf184-13e5-4e89-8247-0a25a77c0942 — Accounts carry their provider through sign-ins and tokens
- d111d5e9-d4e5-4cfa-bb8b-b820e6fe75b8 — Hosted playmat reads password accounts from its settings
**Execution:** agent

Every configured password account gets the reserved place the single Google reviewer used to have: it gets in when the playmat is full and is not counted against the limit, at sign-in and on tool calls. The single-reviewer email setting is retired. If it is still set, the playmat logs a warning and starts. The app spec records the new secret in place of the old one.

*Technical detail:* [context.md#task-reserved-places-for-password-accounts](./context.md#task-reserved-places-for-password-accounts)

**Acceptance criteria**:
- [ ] With the playmat at its player limit, a password account signs in and imports a game.
- [ ] With the playmat one below its limit, a password account signs in and imports a game, and a new Google player can still join afterwards.
- [ ] A password account that lost its place (idle) is still admitted on its next tool call when the playmat is full.
- [ ] A leftover single-reviewer setting no longer grants anything. The playmat logs that it is ignored and starts normally.
- [ ] The app spec declares the password-accounts secret and no longer declares the reviewer setting.

#### - [ ] Task: Reviewer materials and privacy page
**Id:** 64cbb224-059f-419a-9f08-64398838b91e
**Repo:** tcgreplay
**Depends on:**
- e761ba87-2115-4efd-aacc-819698474941 — Reserved places for password accounts
**Execution:** agent

The plugin's review test cases, demo script and submission checklist tell the reviewer to sign in with the configured reviewer account instead of Google. The checklist tells the operator how to create a reviewer account and where to set it. The privacy page explains reviewer password accounts and that the login cookie exists during any sign-in. A check keeps the reviewer materials from drifting back to Google instructions.

*Technical detail:* [context.md#task-reviewer-materials-and-privacy-page](./context.md#task-reviewer-materials-and-privacy-page)

**Acceptance criteria**:
- [ ] The review test cases, demo script and submission checklist instruct signing in with the configured reviewer account and no longer tell the reviewer to use a Google account.
- [ ] The submission checklist explains creating a reviewer account (username and bcrypt hash), setting it in the console, and entering the credentials in OpenAI's review form.
- [ ] The privacy page describes reviewer and invited password accounts (what is kept for them) and says the login cookie exists only while a sign-in is in progress.
- [ ] An automated check fails if the reviewer materials tell the reviewer to sign in with Google. The plugin package still builds.

#### - [ ] Task: Configure reviewer accounts on the live playmat
**Id:** 3d00c733-cd31-4d99-a85f-82459ef1aad3
**Repo:** tcgreplay
**Depends on:**
- e761ba87-2115-4efd-aacc-819698474941 — Reserved places for password accounts
- 64cbb224-059f-419a-9f08-64398838b91e — Reviewer materials and privacy page
**Execution:** human — needs the DigitalOcean console and the OpenAI Platform dashboard, and creates secrets

Creates the reviewer accounts (one for OpenAI's review, one ready for Claude's), sets them as the encrypted password-accounts setting in the App Platform console, removes the old reviewer setting, and enters the OpenAI reviewer's username, password and login address in OpenAI's review information.

*Technical detail:* [context.md#task-configure-reviewer-accounts-on-the-live-playmat](./context.md#task-configure-reviewer-accounts-on-the-live-playmat)

**Acceptance criteria**:
- [ ] The live playmat's start-up line reports the configured password accounts, and the old reviewer setting is gone from the console.
- [ ] OpenAI's review information holds the reviewer account's username, password and the login address.
- [ ] The plain-text passwords are kept only in the operator's password manager, never in the repo or the console.

## Open Questions

- **Which header carries the real client address on the live app?** The per-client throttle reads `do-connecting-ip`, then `CF-Connecting-IP`, then the TCP peer. Which of these App Platform's Cloudflare edge actually sets for tcgreplay.app can only be seen on the deployed app. It depends on the live edge configuration. When implementing: keep that order and log the chosen source once at start-up of the first password attempt (the header name, never the address). If the post-deploy check shows every attempt keyed to the edge's address, STOP and ask the user before changing the trust order.
- **Does any existing test rely on the first redirect being Google in a way the helper change does not cover?** Discoverable only when the suites run after the page is inserted. When implementing: update assertions that pinned "302 to Google" to expect the page. If a test's intent was to check something only a direct redirect can show (beyond the redirect itself), STOP and ask rather than deleting the check.

## Out of Scope

- **Email of any kind**: no magic links, one-time codes or password-reset emails (spec Non-Goal).
- **Other sign-in providers** (Facebook and others): the page's provider table leaves room, and adding one is a future spec (spec Non-Goal).
- **Managing password accounts from a web page**: the operator edits the hosted setting. There is no admin UI for accounts (spec Non-Goal).
- **Sign-up, password change and password recovery**: excluded by the spec's requirements and constraints, not merely deferred.
- **A dark theme for the sign-in page**: the Studio design has no dark theme for the website, and the server-rendered pages follow it.
- **Loading IBM Plex on the sign-in page**: the page names the font but does not fetch it, to avoid a new outside resource.
- **A `hash-password` subcommand**: operators use `htpasswd -nBC 12` (or its Docker one-liner). A built-in command would need a no-echo terminal dependency.
- **A token revocation endpoint**: disconnecting an assistant still does not reach the playmat (knowledge gotcha `disconnect-does-not-reach-playmat`). Removing a password account from the setting is the revocation path for those accounts.
- **Changing the player-facing listing description**: "Sign in with your Google account" in the plugin listing stays true for players. Only the reviewer test cases, demo script and checklist change.
- **The desktop playmat**: it has no sign-in and is untouched (spec constraint).
- **Log forwarding for App Platform**: needed to keep `sign_in` records for the Google sign-in success metric (Insights holds no App Platform logs). It is handled separately because it needs a provider choice and is billed.
