---
created_date: "2026-10-18"
document_status: final
closed_date: "2026-10-18"
---

# Test plan: 20261017123240-21493a95-google-login

The automated suite (`make test`) covers the sign-in service, the hosted handler, the limits and the desktop regression, all against an in-process fake Google. Everything below needs the real Google, claude.ai, Cloudflare and App Platform, so it runs by hand on the deployed app at https://tcgreplay.demo.gs.

**Who / when:** the owner runs these once this branch is merged to `main` and App Platform has deployed it. Items 0–3 run straight after the deploy, as the gate for Milestone 1. Item 13 needs a week of production logs.

## 0. Deployment gate (the two open criteria of the human task)

These are the criteria "starts after a redeploy" and "reachable from outside without a Cloudflare challenge".

- **How:** watch the deployment in App Platform. Once it is live, set the health check path to `/healthz` under Settings → playmat → Health Checks and let it redeploy. Then run these from any machine outside DigitalOcean:
  - `curl -s https://tcgreplay.demo.gs/healthz`
  - `curl -s https://tcgreplay.demo.gs/.well-known/oauth-authorization-server`
  - `curl -s https://tcgreplay.demo.gs/.well-known/oauth-protected-resource/mcp`
  - `curl -si -X POST https://tcgreplay.demo.gs/register -H 'Content-Type: application/json' -d '{"redirect_uris":["https://claude.ai/api/mcp/auth_callback"]}'`
  - `curl -si -X POST https://tcgreplay.demo.gs/token -d grant_type=refresh_token -d client_id=nope -d refresh_token=nope`
- **Expected:**
  - The deployment reaches "Active". The runtime log has `tcgreplay … hosted: listening on …, public https://tcgreplay.demo.gs, … Google sign-in, 300 requests a minute per account, 10 users`.
  - `/healthz` answers `ok`.
  - The server metadata has `"issuer":"https://tcgreplay.demo.gs"` and `"code_challenge_methods_supported":["S256"]`.
  - The resource metadata has `"resource":"https://tcgreplay.demo.gs/mcp"`.
  - `/register` answers `201` with a `client_id`.
  - `/token` answers `401` with `{"error":"invalid_client",…}`.
  - No answer is a Cloudflare challenge page (HTML or a `cf-mitigated` header).
- If the deploy fails to start, check the log for `hosted mode needs Google sign-in`, which means the credentials are missing or misnamed.

## 1. Anonymous connector request refused (success metric: zero unsigned requests reach game actions, at deploy time)
- **How:** run `curl -si -X POST https://tcgreplay.demo.gs/mcp -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"new_game","arguments":{}}}'`.
- **Expected:**
  - `401` with `WWW-Authenticate: Bearer resource_metadata="https://tcgreplay.demo.gs/.well-known/oauth-protected-resource/mcp", scope="playmat"`.
  - The runtime log has `hostauth: refused POST /mcp: no bearer token`.

## 2. Connector flow in claude.ai (manual review: Connector flow)
- **How:** in claude.ai (Settings → Connectors), remove the authless connector from the web-mcp-docker trial. Add a custom connector with the URL `https://tcgreplay.demo.gs/mcp` and press Connect.
- **Expected:**
  - A Google account chooser for "tcgreplay" opens. After choosing an account, claude.ai shows the connector as connected and lists the playmat's tools.
  - In a chat, Claude can `ping` (answer `pong`) and import a real TCG Live battle log with `import_log`. The result has a `board` link `https://tcgreplay.demo.gs/`.
  - The log shows the steps in this order: `hostauth: registered client "Claude"` (or similar), then `hostauth: signed in <email>`.
- **If it fails** (claude.ai never reaches Google, or never calls `/token`): copy the log lines for `/register`, `/authorize`, `/auth/google/callback` and `/token`, along with what claude.ai showed. Stop and report before changing the flow; this is the plan's first open question.

## 3. Cookies and redirects through Cloudflare and App Platform (manual review)
- **How:** in a private window, open `https://tcgreplay.demo.gs/` and sign in with Google.
- **Expected:**
  - The sign-in returns to the board without looping. In the browser's dev tools, `tcgreplay_session` is set `HttpOnly; Secure; SameSite=Lax; Path=/`.
  - Reloading does not go back to Google.
  - A sign-in loop means a cookie is being dropped. Check Cloudflare's settings for the zone before changing anything else.

## 4. Two personal Google accounts (manual review: Any account)
- **How:** sign in to the board, and connect a claude.ai connector, with a second personal Google account that is not the owner's.
- **Expected:** both sign in. A Google error "access blocked: app in testing" means the consent screen is not "In production".

## 5. Board link while signed out, on a phone (manual review: Board link)
- **How:** on a phone that has never signed in, open the board link Claude returned, for example `https://tcgreplay.demo.gs/?x=1`.
- **Expected:** Google sign-in, then the same page with the board. The header shows the account's email and a **Sign out** button.

## 6. Stays signed in (manual review)
- **How:** use the connector and reload the board several times over a day with no deploy.
- **Expected:** no Google prompt in claude.ai or on the board. The connector keeps working past the one-hour access-token lifetime, because claude.ai refreshes it.

## 7. Restart and redeploy (manual review: Restart; success metric: signing back in after a deploy takes under a minute)
- **How:** with the connector connected and a board open, push a trivial change to `main`, or use App Platform → Actions → Force rebuild and deploy. Time the following from when the new deployment goes Active:
  - the open board, and a reload of it;
  - the next tool call from claude.ai.
- **Expected:**
  - The open board sends the user to Google and back to the board on its own, because its live connection is refused and `/auth/session` answers 401.
  - In claude.ai, the next tool call either prompts to reconnect or reconnects through Google.
  - Both are working again within **60 seconds** of the user starting to sign in.
- **Record exactly what claude.ai shows.** If it only shows errors and offers no way to sign in again short of removing and re-adding the connector, stop and report it; this is the plan's second open question.

## 8. Sign out (manual review: Sign out)
- **How:** on the board, press **Sign out**.
- **Expected:**
  - The page "You have signed out of tcgreplay." appears, and it is not an instant bounce back to Google.
  - In another tab, a board that was open under the same browser shows its connection dot red, then sends the user to Google.
  - Reloading `https://tcgreplay.demo.gs/` asks for Google sign-in.
  - The claude.ai connector still works, because sign-out ends the board session only.

## 9. Rate limit and user cap on the deployed app (manual review: Rate limit, User cap)
- **How:** in App Platform, set `TCGREPLAY_RATE_LIMIT=5` and `TCGREPLAY_MAX_USERS=1`, then wait for the redeploy.
  - **Rate limit:** sign in with account A and ask Claude to call `ping` ten times.
  - **Board:** move cards quickly on the board.
  - **User cap:** with A still signed in, open the board in a private window with account B.
- **Expected:**
  - Claude relays "too many requests: this account is limited to 5 requests a minute; try again in Ns" as a tool error. It is not a generic connector failure.
  - The board shows the same message as an error toast.
  - Account B sees "tcgreplay is busy" (503), and the log has `hostauth: refused sign-in for <B's email>: the server is busy …`.
- **Then:** restore `TCGREPLAY_RATE_LIMIT=300` and `TCGREPLAY_MAX_USERS=10`. After the restart both messages are gone. This also covers item 10.

## 10. Settings take effect without a rebuild (manual review: Settings)
- **How:** item 9 changes the limits in App Platform's environment variables only. Rotating the Google client secret in Google Cloud and App Platform would also do.
- **Expected:**
  - The new values show in the start-up log line after a restart, with no code change and no rebuild of the image. Use App Platform's restart without rebuild if offered; a redeploy of the same commit also counts.
  - A wrong client secret makes sign-in fail at the Google callback with "Google sign-in could not be confirmed".

## 11. Desktop regression (manual review: Desktop)
- **How:** on the laptop, use Claude Code in this repo (`make dev`, server `tcgreplay-dev`) and Claude Desktop with the installed `tcgreplay`. Run `ping`, `import_log`, and open the board at http://127.0.0.1:7777/ (or :7778 for dev).
- **Expected:**
  - No sign-in anywhere. The board header has no account and no Sign out button.
  - The board reconnects after the daemon restarts, as before.

## 12. Owner on phone and laptop (success metric: one Google sign-in each)
- **How:** connect the claude.ai connector on the laptop (web) and use it from the phone app (the connector is per claude.ai account). Open board links on both devices.
- **Expected:**
  - One Google sign-in per device's browser, and no further prompts until the next deploy.
  - The claude.ai connector needs one sign-in in total.

## 13. A week of production logs (success metric: zero unsigned requests reaching game actions in the first week)
- **How:** after a week live, export the App Platform runtime logs and search them:
  - `grep 'hostauth: refused'` for refusals;
  - `grep 'hostauth: signed in'` for sign-ins.
- **Expected:**
  - Every unsigned request to `/mcp`, `/ws` or `/api/card` shows up as a `hostauth: refused …` line, and none has a game action after it. The code guarantees this: refused requests never reach the MCP server or the board's dispatcher.
  - Every game action in the log falls in a period when some account was signed in.
  - Any unexpected traffic pattern goes to the owner.
