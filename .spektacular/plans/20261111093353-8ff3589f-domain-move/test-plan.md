---
created_date: "2026-11-12"
document_status: final
closed_date: "2026-11-12"
---

# Test plan: 20261111093353-8ff3589f-domain-move

The automated checks cover these: the website's address coming from `.do/app.yaml` (`site/e2e/facts.spec.js`), the docs and the old-address/`www` scan (`site/e2e/connect.spec.js`, `site/e2e/site.spec.js`), the moved MCP answer (`internal/mcpserver/moved_test.go`), and the moved-host setting and guard (`cmd/tcgreplay/hosted_test.go`, `cmd/tcgreplay/hosted_moved_test.go`). Run them with `go test ./...` and `make site-test`.

Everything below is manual, because it runs against the live deployment, the external consoles or the real assistants. Run it in this order: first merge `spek/20261111093353-8ff3589f-domain-move` to `main` (the "Deploy playmat" workflow deploys it), then follow `docs/domain-move.md` step by step. **Who:** the maintainer. **When:** right after the merge deploys, unless stated otherwise. `<app-id>` is the App Platform app id (`doctl apps list`).

## Still to do after merge (the human task's accepted shortfall)

On 2026-11-12 tcgreplay.app was already live as the public address. These runbook steps were not yet done, and the checks in sections A to G need them:

1. Runbook step 6: `doctl apps update <app-id> --spec .do/app.yaml`, which applies `TCGREPLAY_MOVED_FROM=tcgreplay.demo.gs` and the domains.
2. Runbook step 7: narrow the demo.gs Cloudflare redirect rule to `(http.host eq "tcgreplay.demo.gs" and http.request.uri.path ne "/mcp")`.
3. Runbook step 2: set the consent-screen home page to `https://tcgreplay.app/` and the privacy policy to `https://tcgreplay.app/privacy`, with `tcgreplay.app` among the authorised domains.
4. Runbook step 9: once section C passes, remove `https://tcgreplay.demo.gs/auth/google/callback` from the Google client.

## A. Everything answers at the new address (acceptance criterion)

- **How:**
  - `curl -s -o /dev/null -w '%{http_code}\n' https://tcgreplay.app/`
  - `curl -s -o /dev/null -w '%{http_code}\n' https://tcgreplay.app/docs/`
  - open `https://tcgreplay.app/board` in a browser
  - `curl -s https://tcgreplay.app/.well-known/oauth-protected-resource/mcp`
  - `curl -s https://tcgreplay.app/.well-known/oauth-authorization-server`
- **Expected:**
  - `/` and `/docs/` print `200`;
  - `/board` sends you to Google sign-in and then opens the board;
  - both metadata documents name only `https://tcgreplay.app` addresses: `"resource":"https://tcgreplay.app/mcp"`, issuer `https://tcgreplay.app`, and the endpoints under it.

## B. Valid HTTPS, plain HTTP redirected, no www (acceptance criteria)

- **How:**
  - open `https://tcgreplay.app` in a browser;
  - `curl -sv https://tcgreplay.app/ 2>&1 | grep -E 'subject:|expire date:'`;
  - `curl -sIL http://tcgreplay.app/docs/connect/ | grep -iE '^HTTP|^location'`;
  - `curl -s "https://cloudflare-dns.com/dns-query?name=www.tcgreplay.app&type=A" -H 'accept: application/dns-json'`.
- **Expected:**
  - no browser warning;
  - the certificate subject names `tcgreplay.app` and its expiry is in the future;
  - the HTTP request ends at `https://tcgreplay.app/docs/connect/` with `200`;
  - the DNS answer has no `Answer`.

## C. Full connect-and-play at the new address (acceptance criterion)

- **How:** in each of claude.ai (Settings → Connectors → Add custom connector), ChatGPT (custom MCP server, OAuth) and Claude Code (`claude mcp add --transport http tcgreplay https://tcgreplay.app/mcp`, then `/mcp` → Authenticate), add a fresh connection to `https://tcgreplay.app/mcp`, sign in with Google, ask "What can the tcgreplay playmat do?", then ask it to start a new game and open the board link it gives.
- **Expected, in all three:**
  - sign-in completes;
  - the assistant lists the coaching workflows;
  - the board link starts with `https://tcgreplay.app/board`;
  - it opens the board already signed in;
  - no error appears at any point.

## D. Old pages redirect permanently, keeping path and query (acceptance criterion)

- **How:**
  - `curl -sI https://tcgreplay.demo.gs/docs/connect/ | grep -iE '^HTTP|^location'`
  - `curl -sI "https://tcgreplay.demo.gs/board?x=1" | grep -iE '^HTTP|^location'`
  - `curl -sIL http://tcgreplay.demo.gs/docs/ | grep -iE '^HTTP|^location'`
- **Expected:**
  - the first two return `301` with `location: https://tcgreplay.app/docs/connect/` and `location: https://tcgreplay.app/board?x=1`;
  - the third ends at `https://tcgreplay.app/docs/` with `200`.

## E. Old assistant connection shows the moved message (acceptance criterion; needs "Still to do" 1 and 2)

- **How:**
  - first, the protocol check:

    ```sh
    curl -s -X POST https://tcgreplay.demo.gs/mcp \
      -H 'content-type: application/json' \
      -H 'accept: application/json, text/event-stream' \
      -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"curl","version":"0"}}}'
    ```

  - then, in claude.ai and in ChatGPT, use a connector still set to `https://tcgreplay.demo.gs/mcp` (keep an existing one, or add one at that address). Ask the assistant to do something with the playmat, for example "Start a new game on the tcgreplay playmat".
- **Expected:**
  - the curl returns `200` with an `initialize` result whose `instructions` read: "The tcgreplay playmat has moved to a new address. This connection no longer works: remove it, then add the playmat again at https://tcgreplay.app/mcp and sign in." It is not a `301` and not a `401`.
  - In both assistants, the conversation or the connector's error shows that message naming `https://tcgreplay.app/mcp`, not just a generic failure.
- **If it fails:** a `401` or the normal instructions from the curl means App Platform is not passing the `tcgreplay.demo.gs` Host through. A generic failure in an assistant is a separate problem. In both cases, stop and raise it (plan Open Questions). Do not change code on the spot.

## F. The old address signs no one in (acceptance criterion; needs "Still to do" 1)

- **How:**
  - `curl -sI "https://tcgreplay.demo.gs/authorize?response_type=code&client_id=x&redirect_uri=https%3A%2F%2Fclaude.ai%2Fcb" | grep -iE '^HTTP|^location'`
  - `curl -sI https://tcgreplay.demo.gs/auth/login | grep -iE '^HTTP|^location'`
  - open an old board link such as `https://tcgreplay.demo.gs/board?board=…` in a signed-out browser.
- **Expected:**
  - both curls return `301` to the same path and query on `https://tcgreplay.app`;
  - the browser ends on tcgreplay.app (Google sign-in there, then the board on tcgreplay.app) and is never signed in on tcgreplay.demo.gs.

## G. The start-up log names the address and the moved host (needs "Still to do" 1)

- **How:** `doctl apps logs <app-id> --type run | grep 'hosted: listening'`.
- **Expected:** the line contains `public https://tcgreplay.app, moved from tcgreplay.demo.gs`.

## H. Retiring the old address needs no release (acceptance criterion)

- **Rehearsal (now):** read the "Retiring the old address" section of `docs/domain-move.md` and confirm every step is a Cloudflare or App Platform configuration change, with no code change and no new build.
- **Live check (when the maintainer retires the address):** follow that section. With demo.gs removed from Cloudflare and App Platform and no new build deployed, run its checks:
  - `curl -sI https://tcgreplay.app/` returns `200`;
  - `curl -s -o /dev/null -w '%{http_code}\n' -X POST https://tcgreplay.app/mcp` prints `401`;
  - the protected-resource metadata names `https://tcgreplay.app/mcp`;
  - `/board` redirects to sign-in;
  - an assistant connected at `https://tcgreplay.app/mcp` still works.

## I. Console steps are written and checkable (acceptance criterion; manual review: runbook review)

- **What:** `docs/domain-move.md`, read beside the live Google Cloud, Cloudflare and DigitalOcean consoles.
- **Look for:**
  - steps 1 to 9 are in an order that never breaks sign-in, with the Google callback first and the old callback removed last;
  - each step names a console page or command and the result it should show;
  - the console menus and labels match what the consoles show today;
  - the DNS steps say the records are proxied and how to confirm the certificate.
- **Pass:** each step can be followed as written, and each Check shows the stated result. Fix any wording that no longer matches a console.

## J. Moved-message wording (manual review)

- **What:** the moved text as it appears in claude.ai and ChatGPT during section E. The source is `internal/mcpserver/moved.go` (`MovedText`).
- **Look for:** it reads clearly to a player, names `https://tcgreplay.app/mcp` in full, and says to remove the old connection and add the new one.
- **Pass:** the maintainer is happy with the wording as shown in both assistants.

## Success metrics

- **ChatGPT plugin submission names tcgreplay.app with no later address change.**
  - **How:** when the `chatgpt-submission-prep` spec submits, check every address in the submission (MCP URL, privacy, terms, website).
  - **Expected:** all are on `https://tcgreplay.app`, and the address never has to change after submission.
  - **Who/when:** maintainer, at submission.
- **No player asks for help reconnecting in the first two weeks.**
  - **How:** for 14 days after section E passes, watch the support channels (GitHub issues, email and any community channel the maintainer uses) for reconnect or "connector broken" requests.
  - **Expected:** zero requests.
  - **Who/when:** maintainer, daily for two weeks.
- **No server error at tcgreplay.app caused by the address change in the first week.**
  - **How:** for 7 days after the deploy, review the App Platform runtime logs (`doctl apps logs <app-id> --type run`) and the Insights page.
    - Look for 5xx responses.
    - Look for sign-in failures: Google callback errors, `redirect_uri_mismatch`, token or authorize errors.
    - Look for MCP connection errors.
    - Look for board websocket origin rejections.
  - **Expected:** zero errors attributable to the address (any error naming `tcgreplay.demo.gs` as an issuer, origin or callback counts).
  - **Who/when:** maintainer, daily for one week.
