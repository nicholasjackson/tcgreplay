---
tags: [testing, go, http, cookies]
---

# A host-rewriting RoundTripper leaks the rewritten URL into resp.Request

`http.Transport` sets `resp.Request` to the request it actually sent. A test RoundTripper that serves a public address from a local test server by cloning the request and rewriting its host (as `hostauthtest`'s `alias` does) therefore hands back responses whose `resp.Request.URL` names the loopback server, not the public address the browser asked for.

**Why it bites.** Anything that resolves a relative link against `resp.Request.URL` (following a button on a server-rendered page, for example) goes to the loopback host. The test browser's cookie jar holds its cookies under the public host, so the follow-up request carries none, and a cookie-guarded route refuses it. Redirects were never affected, because `http.Client` resolves `Location` against its own request, so the bug stayed hidden until a test followed a link on a page.

**What to do.** In such a RoundTripper, set `resp.Request` back to the original request before returning (`hostauthtest/google.go`, fixed 2026-11-24 for the sign-in page).
