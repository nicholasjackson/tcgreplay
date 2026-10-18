---
tags: [webui, security, hosting]
---

# The board is locked to loopback in two places

The board server refuses non-local use twice, as protection against DNS rebinding:

1. `localOnly` in `internal/webui/server.go` rejects any request whose `Host` is not `localhost`, `127.0.0.1` or `::1`.
2. The websocket accept options in `internal/webui/hub.go` set `OriginPatterns` to `localhost:*`, `127.0.0.1:*` and `[::1]:*`.

Serving the board anywhere but loopback (hosted mode, a tunnel, a LAN address) has to deal with both. Fixing only the Host check still leaves the page loading with a live connection that is refused.
