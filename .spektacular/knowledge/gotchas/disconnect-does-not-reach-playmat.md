---
tags: [oauth, auth, connector, hosting, privacy]
---

# Disconnecting an assistant does not reach the hosted playmat

The hosted playmat's OAuth server has no token revocation endpoint (`internal/hostauth/oauth.go`, routes registered at about lines 53-71). So when a player removes or disconnects the tcgreplay connector in their assistant, the playmat is never told:

- The refresh-token record, and the account entry it holds (Google account ID, email, verified flag), stay in memory until the refresh token runs out: up to 30 days after its last refresh (`internal/hostauth/store.go`, the 30-day refresh lifetime and the expiry sweep).
- Board **Sign out** (`POST /auth/logout`) ends only that browser's board session; it does not end the assistant's connection.
- After a restart, a still-valid access token is accepted without counting a sign-in, and that account's registry entry is then removed only by the 24-hour purge in `internal/usergames`.

Sign-ins are not lost on restart: with `TCGREPLAY_SIGNING_KEY` set, the board cookie and the assistant's tokens are sealed copies (AES-GCM, `internal/hostauth/seal.go`) that rebuild the sign-in after a deploy. Games are lost on restart. The server-side records themselves are plain in-memory maps; only the copies handed out are encrypted.

Why it matters: any user-facing text (the privacy page, docs) must not say that disconnecting the assistant, or signing out, makes the playmat forget a player at once, or that sign-ins end on a restart. Say instead that the record runs out within 30 days of the connection's last use. Adding a revocation endpoint (also recommended for Gemini, `learnings/publishing-to-gemini.md`) would change this.
