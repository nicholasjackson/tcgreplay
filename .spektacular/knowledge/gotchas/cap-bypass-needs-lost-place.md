---
tags: [testing, hosting, auth, oauth]
---

# A user-cap bypass on tool calls is only tested once the place is lost

The hosted playmat decides places in one spot, `usergames.Registry.Use`: an
account that already holds a place never reaches the "server full" check.
An account admitted at sign-in (Google, `Admit`) takes its place there, so
its following MCP tool calls all skip the cap check.

So anything that lets an account past the cap on tool calls, such as the
reviewer exception keyed on the verified-email flag sealed into access
tokens (`TokenInfo.Extra["verified"]` -> `hostauth.AccountFromToken`), is
never exercised by "sign in at a full cap, then call a tool". If the claim
were dropped from the token, that test would still pass.

To test the tool-call path, make the account lose its place first: run with
an idle time on a settable clock, let the place idle out, give the one place
to another account, then call a tool and check it is not refused as busy.
Example: `cmd/tcgreplay/hosted_cap_test.go` `TestHostedReviewerToolCallsPastCap`
(it fails when the `verified` claim is removed from `verify`). A tool that
needs no game answers "no game open" rather than succeeding, so check for
the busy message, not for success.
