---
tags: [spaces, cdn, hosting, publishing, digitalocean]
---

# Purging the Spaces CDN needs the cdn:delete scope, not cdn:update

The admin page's image clear, and `sets publish` (for the manifest), purge the bucket's CDN with a DigitalOcean personal
access token (`TCGREPLAY_SPACES_CDN_TOKEN`). The playmat makes two API calls:

- `GET /v2/cdn/endpoints` to find the bucket's endpoint by its origin: `cdn:read`.
- `DELETE /v2/cdn/endpoints/{id}/cache` to purge: `cdn:delete`.

DigitalOcean's OpenAPI spec (`specification/resources/cdn/cdn_purge_cache.yml`)
gates the purge on `cdn:delete`. Its scopes page describes `cdn:update` as
"modify CDN endpoints", which sounds right for a purge but is not; the project's
docs said `cdn:update` until 2026-11-06. A token with `cdn:read` and `cdn:update`
finds the endpoint and then fails to purge with a 403.

The token is a normal personal access token (API → Tokens → Generate New Token,
custom scopes), not a Spaces access key. Spaces keys cannot call the CDN API.
