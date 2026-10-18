---
tags: [auth, oauth, mcp, claude-ai, connector]
---

# What claude.ai requires of a connector's OAuth

Source: claude.com/docs/connectors/building/authentication (read while planning google-login, 2026-10-17). Applies to any MCP server added to claude.ai as a custom connector with sign-in.

- An anonymous request to the MCP endpoint must get `401` with a `WWW-Authenticate` header carrying `resource_metadata` (RFC 9728).
- The protected-resource metadata's `resource` must equal the MCP URL exactly. Only the first `authorization_servers` entry is used.
- claude.ai identifies itself through Dynamic Client Registration, which creates a public client (PKCE, no secret).
- Refresh tokens must be rotated on use.
- A dead refresh token gets `invalid_grant`. A `401 invalid_client` makes claude.ai register again and re-run sign-in, which is the signal to use after a restart wipes in-memory clients.
- `/token` takes form-encoded bodies; `/register` takes JSON.
- Endpoints get a 10 s budget (30 s for refresh).
- Callback addresses: `https://claude.ai/api/mcp/auth_callback` and `https://claude.com/api/mcp/auth_callback`. Claude Code uses a loopback callback on any port.
- Anthropic's servers call from `160.79.104.0/21`; a WAF or bot check in front of the sign-in paths must not challenge that range.
