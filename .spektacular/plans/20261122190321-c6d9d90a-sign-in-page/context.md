---
created_date: "2026-11-23"
document_status: final
closed_date: "2026-11-23"
---

# Context: 20261122190321-c6d9d90a-sign-in-page

## Current State Analysis

- Sign-in is Google-only. Both entry points, the connector's `GET /authorize` (`internal/hostauth/oauth.go:258-298`) and the board's `GET /auth/login` (`internal/hostauth/board.go:20-27`), call `startGoogle` (`internal/hostauth/google.go:41-62`), which records a `pendingLogin` (`store.go:73-84`), sets a per-state login cookie scoped to `/auth/google/callback`, and redirects to Google.
- `serveGoogleCallback` (`google.go:68-112`) checks state and cookie, exchanges the code, and runs `signIn` (`google.go:217-235`: user cap via `Places.Admit`, board session cookie, telemetry). It then continues by kind: `continueConnector` (`oauth.go:326-336`) or redirect to `next`.
- `Account{ID, Email, EmailVerified}` (`hostauth.go:73-82`) is sealed into the board cookie and tokens as `grantClaims` (`seal.go:93-108`), and into `TokenInfo.Extra` by `verify` (`oauth.go:449-465`). Sealed grants survive restarts with `TCGREPLAY_SIGNING_KEY`.
- Reserved place: `usergames.Registry` admits one `Config.Reviewer` email, verified by Google, past the cap (`internal/usergames/usergames.go:44-49,122-182`), configured from `TCGREPLAY_REVIEWER` (`cmd/tcgreplay/main.go:212`, `hosted.go:339`).
- Admins are verified Google emails in `TCGREPLAY_ADMINS` (`admin.go:13-15`).
- The rate limiter is a per-account token bucket at N per minute (`limit.go:37-90`), swept after one idle minute (`store.go:164-168`).
- Pages are one plain `html/template` (`pages.go`), not in the Studio look. Studio tokens and mark live in `internal/design` (`tokens.css`, `mark.svg`).
- Test helpers in `internal/hostauth/hostauthtest/client.go` assume `/authorize` and `/auth/login` redirect straight to Google.
- The module has no `golang.org/x/crypto` today.

## Per-Task Technical Notes

All tasks are in the `tcgreplay` repo (root `/home/nicj/code/github.com/nicholasjackson/tcgreplay`, the only registered repo). Requirement → files resolution is given per task below.

### Task: Sign-in page in front of Google

Requirements: One sign-in page for every sign-in; Google stays available; One sign-in covers connecting and the board (Google half); No form without accounts; Room for more providers; Connection contract unchanged.

**File changes**
- `internal/hostauth/google.go:41-62` — split `startGoogle`. New `startSignIn(w, r, p *pendingLogin)` in a new file `internal/hostauth/signin.go`: `state := a.st.addPending(now, p)`, set the login cookie `loginCookieName(state)` with **Path `/auth/`** (was `googleCallbackPath`), MaxAge `pendingLifetime`, HttpOnly, Secure per `a.secure()`, SameSite Lax. Then `302` to `/auth/signin?state=<state>` with `Cache-Control: no-store`. Keep a `startGoogle(w, r, state string, p *pendingLogin)` that sets `p.nonce`/`p.verifier` on the existing entry and redirects to `a.google.AuthCodeURL(state, S256, nonce, prompt=select_account)` (no new pending, no new cookie).
- `internal/hostauth/signin.go` (new) — `pendingFor(w, r, state) *pendingLogin`: the shared guard. State non-empty, cookie `loginCookieName(state)` equals state, and `a.st.peekPending(now, state)` is live. Otherwise `errorPage(400, "This sign-in has expired or was opened in a different browser. Please start signing in again.")` and nil. Routes: `GET /auth/signin` renders the sign-in page (template from `pages.go`) with the state and the button providers. `GET /auth/google` runs the guard, then `startGoogle`. `finishSignIn(w, r, p, acct)` holds the body moved from `google.go:94-111` (signIn → busy/error page; `loginConnector` → `continueConnector`; default → redirect `p.next`). Provider table: `var buttonProviders = []buttonProvider{{ID: "google", Label: "Sign in with Google", StartPath: "/auth/google"}}`. Each button is rendered as `<a href="{{StartPath}}?state=…" data-provider="{{ID}}">`.
- `internal/hostauth/store.go:173-195` — add `peekPending(now, state) *pendingLogin` (look up without deleting, nil if expired). `takePending` stays for consumption (Google callback, and later the password success). `pendingLogin` (`store.go:73-84`) is unchanged.
- `internal/hostauth/google.go:68-112` — callback: same state/cookie/takePending checks. The tail becomes `a.finishSignIn(w, r, p, acct)`. The cookie deletion uses Path `/auth/`. Also delete any cookie still at the old path `/auth/google/callback` (in-flight sign-ins at deploy time) by matching on name only, since the browser sends both.
- `internal/hostauth/oauth.go:291-297` — `serveAuthorize` calls `a.startSignIn(...)` in place of `a.startGoogle(...)`. All checks above it are unchanged.
- `internal/hostauth/board.go:20-28` — `serveLogin` calls `a.startSignIn`. Register `GET /auth/signin` and `GET /auth/google` in `boardRoutes` (or `Routes`, `hostauth.go:142-147`).
- `internal/hostauth/pages.go` — add a `signinPage` template (title "Sign in", the provider buttons, the slot for the optional password block added in task 57986df5, `{{.Error}}`). Keep the existing `page` template. Both set `no-store`.
- `internal/hostauth/hostauthtest/client.go:63-77` — `FollowSignIn`: when the response is a 200 HTML page carrying `data-provider="google"`, read that link's href (resolve it against the page URL) and continue. Use a regexp: the template renders the button as `<a data-provider="google" href="…">` in that attribute order, so `data-provider="google" href="([^"]+)"` finds it. Unescape `&amp;` in the href (no HTML parser dependency). `BoardCookie` (`client.go:144-170`) goes through the same follower. `MCPTransport`/`Connect` inherit it.
- `internal/hostauth/google_test.go:71-91` — `harness.begin` expects `/start` → 302 to `/auth/signin`, then follows the page's Google link and returns Google's URL. `TestCookieAttributes` (`:413`) expects login cookie Path `/auth/`. Add tests: `/authorize` and `/auth/login` → the page; page has a `data-provider="google"` link and no `<input` for username or password and no `<form` when no accounts are configured; page has no "sign up", "register", "forgot", "reset" or "change password"; the guard refuses a missing state, an unknown state, a state with no cookie, or a cookie for another state.
- `cmd/tcgreplay/hosted_test.go:369-410` — `googleClientIDAt` follows `/authorize` → page → Google link before checking `accounts.google.com`.
- `grep -rn "accounts.google.com\|StatusFound\|/auth/google/callback" internal/hostauth cmd/tcgreplay --include=*_test.go` — update any other assertion that pinned the first redirect.

**Complexity**: High
**Token estimate**: ~70k tokens
**Agent strategy**: Parallel analysis, sequential integration. One agent changes the flow in hostauth and the hostauthtest follower. Once those compile, run `go test ./internal/hostauth/... ./cmd/tcgreplay/...` and fix the remaining assertions sequentially.

### Task: Studio look for the sign-in and problem pages

Requirement: Known risk "keep the page light and clear, make the Google button the obvious choice". Convention signed-off-design-wins (Studio tokens) and privacy-page-tracks-outside-services (no outside resource).

**File changes**
- `internal/design/design.go:17-27` — add `//go:embed tokens.css` → `var TokensCSS string` and `//go:embed mark.svg` → `var MarkSVG string` (exported, documented: for the hosted app's server-rendered pages). No token values change. `go generate` is not needed.
- `internal/hostauth/pages.go:10-31` — both templates take `<style>` from `template.CSS(design.TokensCSS)` plus a small page stylesheet that uses only `var(--…)` tokens (page background, panel, line, text and muted text, primary navy). Light theme only (no `data-theme`). Font `var(--font-sans)` (names IBM Plex Sans, falls back to system fonts; no Google Fonts link). The mark is inlined from `template.HTML(design.MarkSVG)` at 40px in a header. One centred column, max width ~24rem, 16px side gutter. The Google button sits first: white button, 1px `--line` border, Google "G" inline SVG, label "Sign in with Google", ≥44px tall. Problem, busy and signed-out pages use the same header and panel.
- `internal/hostauth/pages.go:33-38` — `renderPage` and the new sign-in render set `X-Frame-Options: DENY` and `Content-Security-Policy: default-src 'none'; style-src 'unsafe-inline'; img-src data:; form-action 'self'; frame-ancestors 'none'; base-uri 'none'`.
- `internal/hostauth/pages_test.go` (new) — the rendered page contains the mark's `<title>TCG Replay</title>` and the Google button first. Its HTML has no `http://` or `https://` URL in `src` or `href` attributes (outside resources), apart from none. Its headers include frame refusal and `no-store`.

**Complexity**: Low
**Token estimate**: ~20k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Website docs and news entry for the sign-in page

Requirement: player-visible change (convention whats-new-entries); docs accuracy.

**File changes**
- `site/src/content/docs/sign-in.md:7-22` — "The first sign-in": the browser opens TCG Replay's sign-in page; choose **Sign in with Google**, then Google's account chooser. Remove "there is no tcgreplay page in between". Mention the username and password form is only for reviewer and invited accounts. "Opening the board": another browser shows the sign-in page first. Update the frontmatter description.
- `site/src/pages/docs/connect.astro:27,43,70,88,106` — each assistant's steps: "TCG Replay's sign-in page opens: choose Sign in with Google, then pick your account". Line 31 ("You need a Google account to sign in") stays true for players.
- `site/src/pages/index.astro:18,63,135` — "sign in with Google" stays accurate. No change needed. Confirm when editing.
- `site/e2e/connect.spec.js:155-162` — the `/docs/sign-in` assertion expects the new wording (e.g. `/TCG Replay['’]s sign-in page/` and `/Sign in with Google/`) in place of only `Google['’]s account chooser`.
- `site/src/data/whats-new.yaml:13` (top of the list) — new entry: `date` = release day, `title: "A TCG Replay sign-in page"`, `refresh: false`, `changes`: connecting or opening the board now shows TCG Replay's sign-in page; choose Sign in with Google as before; same account and games. Quote lines containing `: `.

**Complexity**: Low
**Token estimate**: ~15k tokens
**Agent strategy**: Single agent, sequential execution. Build the site and run the connect and site specs only (fast-iteration convention), then the full site suite once at the end.

### Task: Password accounts from configuration

Requirements: Username and password for configured accounts; Several configured accounts; Credentials never in plain text; Kept apart from Google accounts (identity).

**File changes**
- `go.mod`, `go.sum` — add `golang.org/x/crypto` (bcrypt) with `go get golang.org/x/crypto@latest` and `go mod tidy`.
- `internal/hostauth/password.go` (new) — `type PasswordAccounts struct{ byName map[string]passwordAccount }` with `passwordAccount{hash []byte; fp string}`. `ParsePasswordAccounts(raw)` splits on commas and any whitespace and skips empties. Each entry is `name:hash`, cut at the first `:`. The name is lower-cased and must match `^[a-z0-9._+@-]{1,254}$`: a plain name or an email address, never containing `:`, `,` or whitespace (the list format). There is no check that an email-shaped name is a real address. The hash must satisfy `bcrypt.Cost(hash)` without error and cost ≥ 10. A duplicate name is refused. Errors name the entry's position (`entry 2`) and the rule broken, never the value or the username. `Len()`. `Check(username, password) (Account, bool)` lower-cases and trims the username and looks it up. Unknown usernames run `bcrypt.CompareHashAndPassword(dummyHash, …)` and return false. The dummy hash is generated at parse time at the highest cost among the configured hashes, so its timing matches a real comparison. Passwords over 72 bytes are refused without comparing (bcrypt would error) after the same dummy compare. On success it returns `Account{ID: "password:" + name, Email: name, Provider: ProviderPassword}`. `Fingerprint(name) (string, bool)` is the first 16 hex characters of SHA-256 of the configured hash, computed at parse time. `Valid(name, fp string) bool` reports whether the name is configured and the fingerprint is equal (constant-time compare).
- `internal/hostauth/hostauth.go:28-60,73-82` — `Config.PasswordAccounts *PasswordAccounts`. Constants `ProviderGoogle = "google"`, `ProviderPassword = "password"`. Add `Account.Provider string` and `func (a Account) Password() bool`. Neither is used outside this task's tests yet.
- `internal/hostauth/password_test.go` (new) — hashes generated in the test at cost 10 (`bcrypt.DefaultCost`, about 60 ms each; there is no test-only parse option). Cases: several accounts, including an email-shaped username (`reviewer@example.com`) and a plain one; comma-, newline- and space-separated lists; plain text (`alice:hunter2`) refused, with an error that contains neither `hunter2` nor `alice`; bad username (empty, 255 characters, containing a space or `:`) refused; mixed-case `Reviewer@Example.com` accepted as `reviewer@example.com`; duplicate (case-insensitive) refused; Check right and wrong; unknown username false; timing of unknown vs wrong within a loose bound (both ≥ the dummy compare: assert that both took at least the time of one bcrypt compare at the cost used); ID never equal to a Google-style sub or email, even for `reviewer@example.com`; Fingerprint changes when the hash changes; Valid false for a removed name or changed hash.

**Complexity**: Medium
**Token estimate**: ~25k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Accounts carry their provider through sign-ins and tokens

Requirements: Kept apart from Google accounts (sessions); Password accounts are not admins; Several accounts (removal by configuration); One sign-in covers connecting and the board (provider rides tokens).

**File changes**
- `internal/hostauth/seal.go:93-108` — `grantClaims` gains `P string \`json:"p,omitempty"\`` and `K string \`json:"k,omitempty"\``. `account()` sets `Provider: g.P` (empty = Google; normalise `""` to `ProviderGoogle` in `Account.Password()` checks only, not on the wire). Add a constructor helper `grantFor(acct, fp)` used by `addSession` and `issueTokens`.
- `internal/hostauth/store.go:13-60` — store gains `validGrant func(Account, fp string) bool` (nil = always valid), set by `New` from `cfg.PasswordAccounts` (`p.Valid(strings.TrimPrefix(acct.ID, "password:"), fp)`). `addSession` (`:198-208`) and `issueTokens` (`:388-399`) seal `P` and `K`; the fingerprint comes from a `fingerprint func(Account) string` hook (empty for Google). `session` (`:210-232`), `takeRefresh` (`:346-372`) and `accessToken` (`:401-409`) refuse a grant whose `P == password` when `validGrant` returns false (and refuse any password grant when no accounts are configured).
- `internal/hostauth/oauth.go:449-478` — `verify` adds `"provider": t.account.Provider` to `Extra`. `AccountFromToken` reads it.
- `internal/hostauth/hostauth.go` — `func (a Account) PlayerKey() string` returns `a.Email` for Google (empty provider included) and `"password:" + a.Email` for password accounts. Replace `Email: <acct>.Email` with `Email: <acct>.PlayerKey()` in every recorder call in hostauth: `google.go:95,233`, `board.go:134`, `limit.go:86`, `admin.go:44,53`. (The `telemetry.Event.Email` field keeps its name; its comment says it takes the player key.)
- `internal/usergames/usergames.go:134,145,156,223,297`, `internal/usergames/board.go:34`, `internal/usergames/telemetry.go:50,60,62,64,90` — same replacement (`acct.PlayerKey()`, `e.acct.PlayerKey()`). `Entry.acct` keeps the full account (provider included), so `entryLocked`'s email refresh must also keep `Provider`.
- `internal/hostauth/admin.go:13-15` — `IsAdmin` returns false when `acct.Password()`. `RequireAdmin` (`:41-48`) appends the "sign out and sign in again" hint only for Google accounts.
- `internal/hostauth/board.go:100-121` — `sessionInfo` gains `Provider string \`json:"provider"\`` (`"google"` when empty).
- `internal/hostauth/seal_test.go`, `verified_test.go` (pattern) — a grant sealed without `p` opens as Google (use a fixed pre-change sealed value made with the old struct shape). A password session, access token and refresh token round-trip with the provider. After rebuilding `Auth` with the same signing key and the account removed, or its hash changed, all three are refused. `AccountFromToken` carries the provider.
- `internal/hostauth/events_test.go`, `internal/usergames/events_test.go` (pattern) — a password account `reviewer@example.com` and a Google account with that email produce different `player` values in `sign_in`, `player_day` and `tool_call` records. A Google account's player ID is unchanged from before (hash of its email).
- `internal/hostauth/admin_test.go` — a password-account session gets 403 on GET and on POST to an admin route, also when `Config.Admins` lists the same email as the username. This task can only build that session through the store (signIn with a password account), since the form arrives in task 57986df5.
- `internal/hostauth/board_test.go` — `/auth/session` returns `provider`.

**Complexity**: Medium
**Token estimate**: ~35k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Throttle for failed password sign-ins

Requirement: Repeated failures are slowed down (with the existing rate limiting).

**File changes**
- `internal/hostauth/limit.go:29-65` — `bucket` gains `cap float64` and `window time.Duration`. `take(now, key, capacity int, window time.Duration)` refills at `capacity/window` and keeps the current return values. New `peek(now, key, capacity, window) (ok bool, wait time.Duration)` refills without spending. `allow` (`:68-80`) calls `take(now, "acct:"+acct.ID, a.cfg.RateLimit, time.Minute)`, so its behaviour is unchanged.
- `internal/hostauth/store.go:164-168` — the sweep forgets a bucket once `now.Sub(b.last) >= b.window` (it is full again), not after a fixed minute.
- `internal/hostauth/limit.go` — constants `loginUserCap = 10`, `loginClientCap = 30`, `loginWindow = 15 * time.Minute`. Helpers `(a *Auth) loginAllowed(now, user, client string) (bool, time.Duration)` (peek both) and `loginFailed(now, user, client)` (take both). Keys are `login-user:<lower-cased username>` (the attempted string, whether or not it exists) and `login-client:<addr>`. `clientAddr(r)` returns the first non-empty of `do-connecting-ip` or `CF-Connecting-IP` (trimmed, parsed with `net.ParseIP`), else the host part of `r.RemoteAddr`.
- `internal/hostauth/limit_test.go` — existing rate-limit tests unchanged and passing. New: cap 10 over 15m refuses the 11th take and allows again after 90s; peek never spends; keys are independent; the sweep keeps a 15-minute bucket past one minute; `clientAddr` header precedence and IPv6.

**Complexity**: Low
**Token estimate**: ~15k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Username and password sign-in on the page

Requirements: Username and password for configured accounts; No password change or recovery (note); One sign-in covers connecting and the board; Failed sign-ins reveal nothing; Repeated failures slowed; Credentials never in logs or telemetry.

**File changes**
- `internal/hostauth/pages.go` — the sign-in template's optional block (shown when `cfg.PasswordAccounts.Len() > 0`): a divider "or", then `<form method="POST" action="/auth/password">` with hidden `state`, `username` (`autocomplete="username"`, `autocapitalize="none"`, prefilled after a failure), `password` (`autocomplete="current-password"`), and a submit button "Sign in". Under the form, the note "This form is for reviewer and invited accounts. Players sign in with Google." No other links.
- `internal/hostauth/signin.go` — `POST /auth/password`: `r.Body = http.MaxBytesReader(w, r.Body, 8<<10)`, `ParseForm`, then `p := a.pendingFor(w, r, state)` (cookie check; a cross-site post carries no Lax cookie and is refused). If no accounts are configured → 404. `ok, _ := a.loginAllowed(...)`. If not ok, re-render the page with status 429 and "Too many sign-in attempts. Please try again later." without checking the password. Otherwise `acct, ok := cfg.PasswordAccounts.Check(user, pass)`. On failure, `a.loginFailed(...)`, record `telemetry.Event{Name: SignInRefused, Reason: "credentials"}` (no Email), `log.Printf("hostauth: refused password sign-in")` (no username, no password), and re-render with 401 and "That username and password don't match. Please try again." The pending sign-in is not taken, so the same state works again. On success, `a.st.takePending(now, state)` (nil → the expired problem page), delete the login cookie, then `a.finishSignIn(w, r, p, acct)`. Never pass `r.Form` or the attempted username to any log or recorder.
- `internal/hostauth/google.go:217-235` — `signIn` records `telemetry.Event{Name: SignIn, Email: acct.Email, Provider: acct.providerName()}`.
- `internal/telemetry/recorder.go:12-16,51-75,112-130` — `Event.Provider` and `line.Provider` (`json:"provider,omitempty"`). Update the `SignIn`/`SignInRefused` comments to cover any provider.
- `docs/observability.md:48,60-61` — the `sign_in` row gains `provider` (`google` or `password`). `sign_in_refused` lists the reasons `credentials` and `throttled` (password, no player) besides the cap refusal. Update the example line.
- `internal/hostauth/hostauthtest/client.go` — `FollowPasswordSignIn(browser, start, username, password)`: GET start, follow to the page, POST the form (state from the hidden input), return the final Location. `ConnectPassword(t, public, server, user, pass) Tokens`, `PasswordBoardCookie(t, public, server, user, pass)`, `PasswordMCPTransport(t, public, server, user, pass)`, mirroring the Google helpers.
- `internal/hostauth/password_signin_test.go` (new; harness from `google_test.go:30-60` with `Config.PasswordAccounts` set) — form and note shown with accounts and absent without. Connector: the code exchanges for tokens whose `AccountFromToken` is `password:alice` with provider password. The same browser then gets 200 at `/auth/session`. Board: `/auth/login?next=/board?board=x` → form → lands on `/board?board=x`. Unknown user vs wrong password: identical status and body (after removing the prefilled username) and no session cookie; the retry with the right password on the same state succeeds. Throttle: 10 failures then the right password → 429 page; advance the clock 2 minutes → allowed. 30 failures across 30 usernames from one client → the next is 429 for a new username. Cross-site: POST without the login cookie → problem page, no session. Logs (`captureLog`) and telemetry buffer after success and failure contain neither the password nor a password typed into the username field (`hunter2-typed-as-username`). `sign_in` records carry `"provider":"password"` and Google ones `"provider":"google"`.

**Complexity**: High
**Token estimate**: ~55k tokens
**Agent strategy**: Parallel analysis, sequential integration. One agent does the handler and template, one the telemetry field and the docs row (independent files). Then a single pass on the tests and hostauthtest helpers.

### Task: Hosted playmat reads password accounts from its settings

Requirements: Several configured accounts (configuration alone); Hosted only; end-to-end coverage of Configured account signs in, Two accounts stay separate, No crossover, Admin refused, Board opens signed in after connecting.

**File changes**
- `cmd/tcgreplay/main.go:60-110,200-215` — `config.passwordAccounts string`, read from `TCGREPLAY_PASSWORD_ACCOUNTS` only (never a flag, so it never shows in a process listing).
- `cmd/tcgreplay/hosted.go:121-137` — `hostedAuth` parses it with `hostauth.ParsePasswordAccounts`. On error it returns `fmt.Errorf("TCGREPLAY_PASSWORD_ACCOUNTS: %v", err)` (the parse error never contains the value) and passes `PasswordAccounts` into `hostauth.Config`. Add `passwordAccountsText(n int) string` ("no password accounts (TCGREPLAY_PASSWORD_ACCOUNTS)" / "N password accounts") to the start-up line (`hosted.go:489`).
- `cmd/tcgreplay/hosted_test.go:756-772` — `startHostedWith` lets tests set `c.passwordAccounts`. A helper builds the value from usernames and passwords with `bcrypt.GenerateFromPassword(…, bcrypt.DefaultCost)`.
- `cmd/tcgreplay/hosted_password_test.go` (new) — `alice` and `bob` configured. Each connects with `hostauthtest.PasswordMCPTransport`, alice imports the website example game (`readTestdata`, as `hosted_cap_test.go:126-133`), bob's `get_state` shows no game, and bob's board link for alice's handle is refused ("Not your game"). Crossover: also configure an email-shaped account `carol@example.com`. FakeGoogle accounts `{ID: "sub-carol", Email: "carol@example.com", verified}` (exact email match), `{ID: "sub-alice", Email: "alice@example.com", verified}` and `{Email: "alice"}` sign in and see none of the password accounts' games, get_account differs, and their telemetry player IDs differ from the password accounts'. One browser signs in as alice by password, then as Google alice, and `/auth/session` shows only the latest. Admin: alice's board cookie gets 403 at `/admin`. Board after connect: the browser that ran `FollowPasswordSignIn` for the connector gets 200 at `/board`. Start-up: a malformed value (`alice:plaintext`) makes `runHosted` fail with an error naming the setting and not containing `plaintext`. The start-up log says "2 password accounts" and contains neither `alice` nor `bob`.
- `cmd/tcgreplay/main_test.go` / `desktop` tests — confirm the desktop path never reads the setting (no change expected; a test asserting the desktop serves the board with no `/auth/` routes already exists — keep it green).

**Complexity**: Medium
**Token estimate**: ~40k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Board account label for password accounts

Requirement: board UI accuracy for password accounts (no false "Signed in with Google").

**File changes**
- `web/src/lib/session.js:16-22` — store `app.accountProvider = body.provider || 'google'` beside `app.account`.
- `web/src/lib/store.svelte.js` — add `accountProvider` to the `app` state.
- `web/src/App.svelte:437-439` — `title={app.accountProvider === 'password' ? 'Signed in with a reviewer account' : 'Signed in with Google'}`. `ownName()` (`:220-222`) already returns the whole username when there is no `@`.
- `web/e2e/header.spec.js` or `menus.spec.js` (whichever stubs `/auth/session`; grep `auth/session` in `web/e2e`) — a stubbed `{email: "openai-reviewer", provider: "password"}` shows `openai-reviewer` without the Google tooltip, and the Google stub is unchanged.
- `internal/webui/dist` — rebuild with `cd web && npm run build`.

**Complexity**: Low
**Token estimate**: ~10k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Reserved places for password accounts

Requirements: Configured accounts always get a place; retire `TCGREPLAY_REVIEWER`.

**File changes**
- `internal/usergames/usergames.go:44-49,79-82,100-104,122-140,159-182` — remove `Config.Reviewer` and its normalisation in `New`. Rename `Entry.reviewer` → `reserved`. `isReviewer(acct)` → `acct.Password()`. Update the comments (reserved place for configured password accounts).
- `internal/usergames/places_test.go:258-300` — the reviewer tests become password-account tests: `hostauth.Account{ID: "password:openai", Email: "openai", Provider: hostauth.ProviderPassword}` gets in at the cap and is not counted. A Google account with `Email: "openai"` (verified) is counted normally.
- `cmd/tcgreplay/main.go:76-79,212` — remove `config.reviewer`. If `os.Getenv("TCGREPLAY_REVIEWER") != ""`, `log.Printf("warning: TCGREPLAY_REVIEWER is no longer used; give the reviewer a password account in TCGREPLAY_PASSWORD_ACCOUNTS")`.
- `cmd/tcgreplay/hosted.go:218-225,339,489` — remove `reviewerText` and `Reviewer: c.reviewer`. The start-up line already carries the password-account count (task d111d5e9).
- `cmd/tcgreplay/hosted_cap_test.go:122-260` — `TestHostedReviewerPastCap` becomes `TestHostedPasswordAccountPastCap`: cap 1 held by a Google account, password account `openai` connects with `PasswordMCPTransport`, imports the example game, and its board sign-in returns 200. A third Google account gets the busy page, and a Google account with email `openai@…` gets busy. New `TestHostedPasswordAccountOneBelowCap`: cap 2 with one Google account, the password account signs in and imports, and a new Google account still joins (200). `TestHostedReviewerToolCallsPastCap` becomes the password version: idle clock, place lost, place taken by another account, then a password-account tool call is not refused as busy (gotcha `cap-bypass-needs-lost-place`; it must fail if `provider` is dropped from `verify`'s Extra). Remove `TestHostedReviewerUnsetKeepsTheCap` and `TestReviewerText`. Add a test that a leftover `TCGREPLAY_REVIEWER` (`helperCmd` env) logs the warning and the server becomes healthy.
- `cmd/tcgreplay/hosted_routes_test.go:182-222` — drop the "reviewer account set" assertions and assert the password-account count wording.
- `.do/app.yaml:166-173` — replace the `TCGREPLAY_REVIEWER` entry with `TCGREPLAY_PASSWORD_ACCOUNTS` (`scope: RUN_TIME`, `type: SECRET`). Comment: comma- or newline-separated `username:bcrypt-hash` entries made with `htpasswd -nBC 12 <username>`; each account signs in on the sign-in page, gets a reserved place, is never an admin; set it in the console, encrypted; never logged. Update any other comment in the file that mentions the reviewer (grep `reviewer`).

**Complexity**: Medium
**Token estimate**: ~35k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Reviewer materials and privacy page

Requirements: Reviewer materials describe the new sign-in; privacy convention.

**File changes** (the plugin files carry uncommitted, unrelated edits on the branch: edit in place, never overwrite)
- `plugin/listing.json:23,26,32` — test case 1: "Connect TCG Replay, sign in with the reviewer account (username and password from the review information) on the TCG Replay sign-in page, and import the example game…". Expected behaviour: "…asks the reviewer to connect TCG Replay; the TCG Replay sign-in page opens and the reviewer signs in with the reviewer account…". Test case 2: "…already signed in with the same reviewer account…". `description` (`:6`) stays ("Sign in with your Google account" is true for players).
- `plugin/demo-script.md:9-20` — use the reviewer password account (the one entered in OpenAI's review information) and the TCG Replay sign-in page. Drop `TCGREPLAY_REVIEWER` and the Google wording.
- `plugin/SUBMITTING.md:20-37` — step 4: create the reviewer account with `htpasswd -nBC 12 openai-reviewer` (or `docker run --rm -it httpd:2.4 htpasswd -nBC 12 openai-reviewer`), add the line to `TCGREPLAY_PASSWORD_ACCOUNTS` in the console (App-Level Environment Variables, encrypted; saving restarts the playmat), and check the start-up line's count. Step 5: Review details: login URL `https://tcgreplay.app`, the reviewer username and password, no MFA. Replace the Google sign-in in the MCP Inspector check (`:37`) with the reviewer account. Note one account per directory review (e.g. `claude-reviewer`).
- `site/src/pages/privacy.astro:72-94` — after the Google paragraph, add: "Reviewer and invited accounts sign in with a username and password that the operator sets up; for them the playmat keeps the username, and the password only as a one-way hash in its settings." Change the cookie line to "`tcgreplay_login_…`, which exists only while a sign-in is in progress." Update `site/e2e/privacy.spec.js` if it pins the old cookie wording (grep `tcgreplay_login`).
- `internal/plugin/plugin_test.go` (new test beside `TestBuildFromRealSources`, `:111`) — `TestReviewerMaterialsUseTheReviewerAccount`: read `plugin/listing.json` (test case prompts and expected behaviour), `plugin/demo-script.md` and `plugin/SUBMITTING.md`. Fail if any matches `(?i)sign(ed)? in with (a |the |your )?(reviewer'?s? )?google` or names `TCGREPLAY_REVIEWER`. Each must contain "reviewer account".

**Complexity**: Low
**Token estimate**: ~20k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Configure reviewer accounts on the live playmat

A person with access to the DigitalOcean console and the OpenAI Platform dashboard does this after the code is deployed (the setting is optional, so the order relative to the push does not matter).

1. Generate a strong random password per account (password manager), then run `htpasswd -nBC 12 openai-reviewer` and `htpasswd -nBC 12 claude-reviewer` (or the `httpd:2.4` Docker one-liner). Each prints `name:$2y$12$…`.
2. In the App Platform console, Settings → App-Level Environment Variables: add `TCGREPLAY_PASSWORD_ACCOUNTS` with both lines (comma-separated), tick **Encrypt**, and save (the playmat restarts and drops in-memory games). Edit only that entry and never paste the whole app spec (gotcha `app-spec-not-applied-by-deploy`).
3. Delete `TCGREPLAY_REVIEWER` from the console.
4. Check Runtime Logs: the start-up line says "2 password accounts" and there is no `TCGREPLAY_REVIEWER` warning.
5. In the OpenAI Platform dashboard's Review information: login URL `https://tcgreplay.app`, username `openai-reviewer`, and its password.

Checked by: the start-up line, a password sign-in on the live site with each account, and the dashboard showing the credentials. Plain-text passwords live only in the password manager.

**Complexity**: Low
**Token estimate**: n/a (human)
**Agent strategy**: n/a (human task)

## Testing Strategy

Per task (see each task's file changes for exact files):
- **Sign-in page in front of Google**: hostauth harness tests for both entry points landing on the page, the guard's refusals, no form without accounts, no sign-up or recovery controls. The existing Google, OAuth, board, cap, admin and hosted suites run through the updated `FollowSignIn`/`BoardCookie` helpers, and old sealed grants still open (regression for "Google works as before" and the unchanged connector contract).
- **Studio look**: rendered-page tests for the mark, the Google button first, no outside URLs, and frame-refusal and `no-store` headers. The look itself is a manual review.
- **Website docs and news entry**: site browser suite (`connect.spec.js` sign-in wording) and the What's new build check.
- **Password accounts from configuration**: unit tests for parsing, plain-text refusal, errors that never echo the value, Check, equal failure for unknown and wrong, ID separation, and fingerprint validity.
- **Provider through sign-ins and tokens**: seal round-trips, pre-change grants open as Google, revocation after a rebuild with the same key, AccountFromToken provider, admin 403 for password sessions, `/auth/session` provider.
- **Throttle**: bucket capacity/window, peek never spends, independent keys, sweep keeps 15-minute buckets, clientAddr precedence. The existing rate-limit tests stay green.
- **Password sign-in on the page**: harness tests for form and note visibility, connector and board completion, identical failures, retry on the same state, throttle refusal and recovery on a settable clock, cross-site refusal, and logs and telemetry free of passwords (including a password typed as the username). Provider on `sign_in` records.
- **Hosted settings**: hosted end-to-end tests with two accounts, kept separate, no crossover with same-named Google accounts, admin refused, board signed in after connecting, malformed setting refused without echo, start-up count without names.
- **Board label**: board e2e with a stubbed `/auth/session` for password and Google.
- **Reserved places**: registry unit tests. Hosted cap tests at the cap, one below the cap, and the tool-call path after the place is lost (per gotcha `cap-bypass-needs-lost-place`). Leftover `TCGREPLAY_REVIEWER` warning.
- **Reviewer materials and privacy**: a plugin test that fails on Google sign-in instructions or `TCGREPLAY_REVIEWER` in the reviewer materials. Privacy spec wording if pinned.
- **Manual (captured in the implementation test plan)**: page look and copy at phone and desktop width; real Google sign-in from ChatGPT and MCP Inspector on the deployed app; post-configuration live checks (start-up count, password sign-in, client-address header); OpenAI review outcome; Google sign-in volume over two weeks.

## Project References

- Repo: `tcgreplay`, root `/home/nicj/code/github.com/nicholasjackson/tcgreplay` (the only registered repo).
- Spec: `20261122190321-c6d9d90a-sign-in-page`.
- Designs: none referenced by the spec. `studio/design.md` (design source `design`) was read for colour roles and the mark.
- Knowledge (repo/tcgreplay): `gotchas/cap-bypass-needs-lost-place.md`, `gotchas/app-spec-not-applied-by-deploy.md`, `gotchas/disconnect-does-not-reach-playmat.md`, `gotchas/go-sdk-sessions-and-auth.md`, `gotchas/live-site-cf-bm-cookie.md`, `learnings/claude-ai-connector-oauth.md`, `learnings/publishing-to-chatgpt.md`. Conventions `whats-new-entries`, `privacy-page-tracks-outside-services`, `signed-off-design-wins`.

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

Two High tasks (sign-in page in front of Google, password sign-in on the page) carry most of the context. Run the Low tasks (throttle, password accounts, board label, docs) in parallel with them where dependencies allow.

## Migration Notes

- Sealed grants: new optional claims `p` (provider) and `k` (fingerprint). Grants without them open as Google, so no player is signed out by the release.
- Login cookie path moves from `/auth/google/callback` to `/auth/`. The callback also clears a cookie at the old path, so a sign-in in flight at deploy time still completes.
- `TCGREPLAY_REVIEWER` is retired: ignored with a warning, then deleted from the console (human task). `TCGREPLAY_PASSWORD_ACCOUNTS` is optional, so the push does not depend on it being set first.
- `.do/app.yaml` records the new secret, but the console holds the live value (gotcha `app-spec-not-applied-by-deploy`).

## Performance Considerations

- bcrypt at cost 12 takes about 250 ms of CPU per check. Throttles are checked before any bcrypt comparison, so floods of failures cost a cheap refusal, not a hash. Unknown usernames also pay one comparison (by design, for equal timing), and the per-client throttle bounds that.
- The sign-in page adds one redirect and one small server-rendered page before Google. There are no outside requests and no scripts.
