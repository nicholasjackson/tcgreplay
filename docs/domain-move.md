# Moving the playmat to tcgreplay.app

The maintainer's runbook for the hosted playmat's address. The playmat's one
public address is `https://tcgreplay.app` (`TCGREPLAY_PUBLIC_URL` in
`.do/app.yaml`): sign-in, the assistant connection, board links, the board's
allowed origin and the website all use it. The old address,
`tcgreplay.demo.gs`, stays only to forward players:

- **Pages** on the old address are forwarded by a Cloudflare redirect rule on
  the demo.gs zone, keeping path and query.
- **The assistant connection** (`https://tcgreplay.demo.gs/mcp`) is left
  alone by that rule and reaches the playmat through the App Platform alias.
  Because `tcgreplay.demo.gs` is listed in `TCGREPLAY_MOVED_FROM`, the
  playmat answers it with no sign-in: it lists the usual tools, and every
  tool call, guide and the connection instructions say the playmat has moved
  and to add it again at `https://tcgreplay.app/mcp`. A redirect cannot do
  this: the assistant drops its token on a redirect to another host, and the
  new address's sign-in metadata names a different connection address, so
  the player would see only a generic failure.
- **Everything else** reaching the playmat for the old host (sign-in, the
  board, metadata) is redirected permanently to tcgreplay.app by the playmat
  itself, so the old address never signs anyone in, even if the Cloudflare
  rule changes.

Do the steps in order. Each has a **Check** with the result it should show.
Steps already done on the live app only need their check run. App
Platform steps are done in the control panel: the app's **Settings > App
Spec > Edit** holds the live definition, and `.do/app.yaml` records what it
should say.

## 1. Google OAuth client

Sign-in must work at the new address before anything else switches.

**Do:** Google Cloud console, project `tcgreplay` > APIs & Services >
Credentials > the Web application client. Under **Authorised redirect URIs**,
make sure `https://tcgreplay.app/auth/google/callback` is listed (keep
`http://localhost:8080/auth/google/callback` for local runs). If
**Authorised JavaScript origins** is set, it is `https://tcgreplay.app`.
Leave `https://tcgreplay.demo.gs/auth/google/callback` in place for now: it
is removed in step 9, once everything else is checked.

**Check:** the client's page lists `https://tcgreplay.app/auth/google/callback`.
Step 8's sign-ins complete.

## 2. OAuth consent screen

**Do:** Google Cloud console > Google Auth Platform > Branding (the OAuth
consent screen). Set the application home page to `https://tcgreplay.app/`,
the privacy policy to `https://tcgreplay.app/privacy`, and add `tcgreplay.app`
to the authorised domains.

**Check:** the Branding page shows those two links and `tcgreplay.app` among
the authorised domains. (The links open once step 4 is done; step 8 checks
them.)

## 3. Attach tcgreplay.app to the app

The new domain is attached before its DNS points at the app, and the
playmat keeps its current public address until step 6, so sign-in keeps
working at every step.

**Do:** App Platform > the app > Settings > Domains > Add domain:
`tcgreplay.app`, "You manage your domain". Do not change
`TCGREPLAY_PUBLIC_URL` here.

**Check:** Settings > Domains lists `tcgreplay.app` (it reads "Pending"
until step 4's record is in place) next to `tcgreplay.demo.gs`. The page
names the app's default ingress, `<app>.ondigitalocean.app`, for step 4.

## 4. Cloudflare DNS and certificate for tcgreplay.app

**Do:** Cloudflare > the `tcgreplay.app` zone > DNS. One record at the apex
(`tcgreplay.app`): a CNAME to the app's default ingress,
`<app>.ondigitalocean.app`, from step 3 (Cloudflare flattens a CNAME at the
apex). It is **proxied** (orange cloud), with SSL/TLS mode
**Full (strict)** and **Always Use HTTPS** on. There is **no `www` record**:
the playmat has no `www` name.

Risk: while the record is proxied, App Platform may be unable to issue its
own certificate for the domain. If Settings > Domains shows `tcgreplay.app`
stuck in "Pending" or "Configuring", switch the record to DNS only (grey
cloud) until it reads "Active", then proxy it again.

**Check:**

- App Platform > Settings > Domains shows `tcgreplay.app` as "Active".
- `curl -sI https://tcgreplay.app/` returns `200` and an `x-do-app-origin`
  header (App Platform behind Cloudflare).
- `curl -sv https://tcgreplay.app/ 2>&1 | grep -E 'subject:|expire date:'`
  shows a subject naming `tcgreplay.app` and an expiry in the future.
  Cloudflare > SSL/TLS > Edge Certificates shows an active certificate
  covering `tcgreplay.app`. A browser opens the site with no warning.
- `curl -sI http://tcgreplay.app/docs/connect/` returns `301` with
  `location: https://tcgreplay.app/docs/connect/`.
- `curl -s "https://cloudflare-dns.com/dns-query?name=www.tcgreplay.app&type=A" -H 'accept: application/dns-json'`
  has no `Answer` (status `NXDOMAIN` or an empty answer).

## 5. Cloudflare DNS for tcgreplay.demo.gs

**Do:** Cloudflare > the `demo.gs` zone > DNS. The `tcgreplay` record stays,
pointing at the same `<app>.ondigitalocean.app`, and stays **proxied**, so
the redirect rule in step 7 runs and the old `/mcp` reaches the App Platform
alias.

**Check:** the zone lists `tcgreplay` as a proxied record, and App Platform
> Settings > Domains shows `tcgreplay.demo.gs` as "Active".

## 6. App Platform app spec

The deploy workflow only asks App Platform for a new deployment; it never
applies `.do/app.yaml`. Domain and environment changes reach the platform
only when you make them in the console.

**Do:** in the app's **Settings > App Spec > Edit**, make the same domain and
`envs` entries as `.do/app.yaml` (edit only those entries; never paste the
whole file over the stored one, which would wipe the encrypted values), then
**Save**.

This makes `tcgreplay.app` the PRIMARY domain and `tcgreplay.demo.gs` an
ALIAS, and sets `TCGREPLAY_PUBLIC_URL=https://tcgreplay.app` and
`TCGREPLAY_MOVED_FROM=tcgreplay.demo.gs`. Sign-in now runs at tcgreplay.app,
which steps 1 and 4 have already made ready. An environment change restarts
the playmat, which drops every in-memory game, as any deploy does.

**Check:**

- Settings > App Spec shows both domains, `TCGREPLAY_PUBLIC_URL` and
  `TCGREPLAY_MOVED_FROM` as above.
- Settings > Domains shows `tcgreplay.app` as the primary domain, both
  "Active".
- The start-up line in the app's Runtime Logs tab contains `public https://tcgreplay.app, moved from tcgreplay.demo.gs`.
- `curl -s https://tcgreplay.app/.well-known/oauth-protected-resource/mcp`
  shows `"resource":"https://tcgreplay.app/mcp"`.

## 7. Cloudflare redirect rule on the demo.gs zone

The rule forwards old pages but must leave the assistant connection alone.

**Do:** Cloudflare > the `demo.gs` zone > Rules > Redirect Rules. Edit the
rule for `tcgreplay.demo.gs` (or create one):

- **When incoming requests match** (custom filter expression):
  `(http.host eq "tcgreplay.demo.gs" and http.request.uri.path ne "/mcp")`
- **Then:** dynamic redirect to
  `concat("https://tcgreplay.app", http.request.uri.path)`, status code
  `301`, **Preserve query string** on.

**Check:**

- `curl -sI "https://tcgreplay.demo.gs/board?x=1"` returns `301` with
  `location: https://tcgreplay.app/board?x=1`.
- `curl -sI https://tcgreplay.demo.gs/docs/connect/` returns `301` with
  `location: https://tcgreplay.app/docs/connect/`.
- `curl -sIL http://tcgreplay.demo.gs/docs/` ends at
  `https://tcgreplay.app/docs/` with `200`.
- The old assistant connection gets the moved answer, not a redirect:

  ```sh
  curl -s -X POST https://tcgreplay.demo.gs/mcp \
    -H 'content-type: application/json' \
    -H 'accept: application/json, text/event-stream' \
    -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"curl","version":"0"}}}'
  ```

  returns an `initialize` result (status `200`) whose `instructions` say the
  playmat has moved and name `https://tcgreplay.app/mcp`.

If that last check returns the normal `401` with tcgreplay.app's sign-in
metadata, or anything other than the moved instructions, App Platform is not
passing the original host (`tcgreplay.demo.gs`) through to the playmat. Stop
and raise it rather than changing code on the spot.

## 8. End to end

**Do and check:**

- Connect claude.ai, ChatGPT and Claude Code to `https://tcgreplay.app/mcp`
  (see the website's Get started page). Each completes Google sign-in, a
  simple request ("What can the tcgreplay playmat do?") succeeds, and a
  board link it hands out (import or start a game) starts with
  `https://tcgreplay.app/board` and opens the board signed in, with no error.
- In claude.ai and ChatGPT, a connector still set to
  `https://tcgreplay.demo.gs/mcp` shows, in the conversation or the
  connector's error, a message naming `https://tcgreplay.app/mcp`. If
  either shows only a generic failure, stop and raise it.
- A board link or sign-in started on `tcgreplay.demo.gs` never completes a
  sign-in there: it ends on tcgreplay.app.
- `https://tcgreplay.app/` and `https://tcgreplay.app/privacy`, the consent
  screen's links from step 2, open the home page and the privacy page.

## 9. Remove the old Google callback

**Do:** once step 8 passes, remove
`https://tcgreplay.demo.gs/auth/google/callback` from the Google client's
authorised redirect URIs (step 1's page).

**Check:** the client lists only the tcgreplay.app and localhost callbacks,
and a fresh sign-in at tcgreplay.app still completes.

## Retiring the old address

When the old address is no longer needed, remove it by configuration alone:
no code changes and no new release.

1. Cloudflare > the `demo.gs` zone: delete the `tcgreplay.demo.gs` redirect
   rule (Rules > Redirect Rules) and the `tcgreplay` DNS record.
2. In `.do/app.yaml`, remove the `tcgreplay.demo.gs` ALIAS under `domains`
   and the `TCGREPLAY_MOVED_FROM` entry, then make the same two removals in
   the console (Settings > App Spec > Edit, Save). This changes only
   domains and environment, so App Platform restarts the playmat without a
   new build (in-memory games are dropped, as on any restart). Removing
   `TCGREPLAY_MOVED_FROM` is optional: with no alias or DNS, nothing reaches
   the playmat for that host. Committing the `.do/app.yaml` edit to `main`
   keeps the file in step with the platform; like any push outside
   `cardsets/`, that push also runs the deploy workflow, which rebuilds and
   redeploys the same code (another restart). To avoid it, commit the edit
   together with the next real change.

**Check:**

- `curl -sI https://tcgreplay.app/` returns `200`.
- `curl -s -o /dev/null -w '%{http_code}\n' -X POST https://tcgreplay.app/mcp`
  prints `401`, and
  `curl -s https://tcgreplay.app/.well-known/oauth-protected-resource/mcp`
  names `https://tcgreplay.app/mcp`.
- `curl -sI https://tcgreplay.app/board` redirects to sign-in.
- An assistant connected at `https://tcgreplay.app/mcp` still works, and the
  board opens.
- `curl -sI https://tcgreplay.demo.gs/` no longer reaches the playmat.
