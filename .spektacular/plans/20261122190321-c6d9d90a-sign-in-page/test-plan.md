---
created_date: "2026-11-24"
document_status: final
closed_date: "2026-11-24"
---

# Test plan: TCG Replay sign-in page and reviewer password accounts

Manual checks for the success metrics no automated test can cover, and for the manual reviews the plan lists. Everything else (the page in front of Google, password sign-in, throttling, revocation, reserved places, separation from Google accounts, no secrets in logs) is covered by `go test ./...`, the site suite and the board suite.

Order: run the reviews (R1, R2) after deploying the release, then configure the accounts (the human task, `plugin/SUBMITTING.md` steps 4 and 5) and run R3, then submit and watch M1. M2 and M3 run over the following weeks.

## Manual reviews

### R1. Sign-in page look and copy (operator, right after deploy)

- **Where:** open `https://tcgreplay.app/board` in a private window (signed out). It redirects to `/auth/signin?state=…`. Also open `/auth/signed-out` (sign out from the board's ☰ menu) and a stale sign-in link (reload the sign-in page after 10 minutes: the "start signing in again" problem page).
- **Look at, at desktop width (1366px) and phone width (375px, browser dev tools):**
  - the TCG Replay mark and "TCG Replay" header, the light Studio colours (page `--bg`, white panel, navy primary), IBM Plex Sans if installed locally (system font otherwise; nothing is fetched);
  - "Sign in with Google" as the first, full-width button with Google's four-colour "G", at least 44px tall;
  - no sideways scrolling at 375px;
  - with no password accounts configured: no form, no "or" divider, no note; with accounts configured (after the human task): the "or" divider, Username and Password fields, a navy "Sign in" button, and the note "This form is for reviewer and invited accounts. Players sign in with Google.";
  - no sign-up, password change or recovery link anywhere;
  - the problem, busy and signed-out pages use the same header and panel.
- **Pass:** all of the above hold. Flag any departure from the Studio look at the walkthrough (convention: the signed-off design canvas wins; the Studio design does not draw this page).

### R2. Real Google sign-in on the deployed playmat (operator, right after deploy)

- **How:**
  1. In ChatGPT (TCG Replay connected before the release), start a new chat and ask "What can the tcgreplay playmat do?". If ChatGPT asks to reconnect, the TCG Replay sign-in page should open; choose Sign in with Google and pick the same Google account as before.
  2. In MCP Inspector, connect to `https://tcgreplay.app/mcp` with OAuth: the browser shows the TCG Replay sign-in page; choose Sign in with Google; then list tools and call `get_account` and `get_state`.
  3. Open a board link from an existing game in the same browser.
- **Pass:** each sign-in goes sign-in page, Google chooser, back to the assistant. `get_account` returns the same account ID as before the release. An existing game (if one was held in memory before the deploy, which restarts the playmat and drops in-memory games) is not expected to survive; the account and its board handle are. The board opens signed in without a second sign-in. Sessions and tokens from before the release still work with the same signing key (no forced re-sign-in).

### R3. Password accounts live (operator, after the human task's console step)

- **How:**
  1. In App Platform Runtime Logs, find the start-up line `tcgreplay … hosted: listening on …`. It must contain `2 password accounts` (one for OpenAI, one for Claude) and no username. There must be no `warning: TCGREPLAY_REVIEWER is no longer used` line (the old setting deleted).
  2. In a private window, open `https://tcgreplay.app/board`, enter `openai-reviewer` and its password in the form: the board opens, and the ☰ menu shows `openai-reviewer` with the tooltip "Signed in with a reviewer account".
  3. Enter a wrong password once: "That username and password don't match. Please try again." with the username kept.
  4. In Runtime Logs, find `hostauth: password sign-in client addresses come from <source>` (logged once, at the first password attempt).
- **Pass:** the count is right and names nobody; the sign-in works; the failure message is as above; `<source>` is `do-connecting-ip` or `CF-Connecting-IP`. If it is `peer`, every attempt is keyed to the edge's address, so per-client throttling would refuse everyone after 30 failures in total: STOP and decide the client-address trust order before relying on it (plan open question).

## Success metrics

### M1. OpenAI's plugin review signs in with the reviewer account and runs the test cases (operator, through the review)

- **How:** after R3, enter the reviewer credentials in the OpenAI Platform dashboard (Review information > Reviewer credentials > Username and password: login URL `https://tcgreplay.app`, username `openai-reviewer`, its password, and the sign-in steps from `plugin/SUBMITTING.md` step 5), then submit. Watch Runtime Logs or Grafana for `sign_in` records with `"provider":"password"` during the review window.
- **Expected result:** the review completes without a "reviewers cannot connect" rejection. Pass: no rejection email naming sign-in or connection; at least one password `sign_in` record during the review.

### M2. Claude's directory review gets its own account with no code change (operator, when submitting to Claude's directory)

- **How:** add the `claude-reviewer` line (made with `htpasswd -nBC 12 claude-reviewer`, same cost as the others) to `TCGREPLAY_PASSWORD_ACCOUNTS` in the console, save (the playmat restarts), and check the start-up line's count rises by one; sign in with it once as in R3 step 2.
- **Expected result:** the count is one higher, the account signs in, and no build or deploy of new code was needed. (The parsing of several accounts from configuration alone is covered by the automated tests.)

### M3. Google sign-ins keep succeeding (operator, two weeks after release)

- **What:** Google sign-in completions in the two weeks after release stay at the level of the two weeks before, and no player reports lost games or accounts.
- **How:** the Grafana observability release (merged before this one) sends the playmat's records to Grafana Cloud (logs kept 14 days) and its usage counters (daily totals kept six weeks, see `docs/observability.md`). In Grafana, count `sign_in` records with `provider` `google` per day for the 14 days before and after the release date (logs), or use the sign-in counter's daily totals for the longer window. Also check the GitHub issues and support channel for reports of lost accounts.
- **Expected result:** the post-release daily average of Google sign-ins is at least about the pre-release average (allow normal week-to-week variation; a drop of more than a third needs investigating), and there are no reports of lost accounts. Note: the 14-day log retention means the comparison must be made within 14 days of the release from logs, or from the counters' daily totals afterwards.
