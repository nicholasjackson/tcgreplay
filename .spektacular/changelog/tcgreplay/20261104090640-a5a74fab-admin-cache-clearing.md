---
created_date: "2026-11-04"
document_status: draft
project: tcgreplay
spec: 20261104090640-a5a74fab-admin-cache-clearing
plan: 20261104090640-a5a74fab-admin-cache-clearing
---

# Admins can clear the hosted card cache

Admins of the hosted playmat can now open a small admin page, after the usual Google sign-in, and clear the card cache: all of it, or one set named by its set code (such as MEG) or its id (such as me01). A clear reaches every cache the playmat keeps, including the copied card images and the image delivery network, so a correction from the card source shows on the board after a reload within a minute, without a redeploy. Games already open keep showing and playing their cards while the cache refills, and card images now carry a version in their address so browsers fetch changed art again.

> Derived from project tcgreplay (local), spec/plan 20261104090640-a5a74fab-admin-cache-clearing. See the project-level record for the full feature.

## What changed in this repo

- **Admin page and job** (`internal/cacheadmin`): `GET /admin` and `POST /admin/clear`, one background clear at a time, per-tier results on the page, start and end log lines.
- **Admin sign-in** (`internal/hostauth`): `Config.Admins`, `Account.EmailVerified` (read from Google's `email_verified`, sealed into the board cookie), `IsAdmin`, `AdminCount` and the `RequireAdmin` guard.
- **Clearing the card data** (`internal/cards`): `Scope` and `TierReport`; an optional `Clear` on the disk, Spaces and tiered stores; `Client.SetID` and `Client.Clear` with the cleared mark and the clear generation.
- **Clearing image copies** (`internal/cards/images.go`): content versions from ETags, `Image` returns the version, `ImageCache.Clear` deletes and purges.
- **Spaces** (`internal/spaces`): batch `Delete`, `ListPage.ETags`, and the `CDN` purge client; the `spacestest` fake learned both, plus a fake DigitalOcean CDN API.
- **Board** (`internal/webui`, `web/src/lib/store.svelte.js`): `/api/card` sends `image_version`, and the board loads `<image>/<size>?v=<version>` from the CDN.
- **Hosted wiring** (`cmd/tcgreplay`): reads `TCGREPLAY_ADMINS` and `TCGREPLAY_SPACES_CDN_TOKEN`, mounts `/admin`, logs the admin count and the CDN purge state.
- **Deploy and docs**: `.do/app.yaml`, `Dockerfile`, `Makefile` and the design document's hosted section and operator setup.
- Tests for each part, including hosted end-to-end tests over stand-ins for Google, TCGdex, Spaces and the DigitalOcean API, a game-level test that an open game survives a clear, and two Playwright cases.

## Why

The playmat cached card data and images with no way to refresh them short of deleting bucket objects by hand and restarting. This repo holds the whole playmat, so every part of the clear, the admin gate and the versioned image addresses lives here.

## Still to do (operator)

Set `TCGREPLAY_ADMINS` and an encrypted `TCGREPLAY_SPACES_CDN_TOKEN` (a DigitalOcean API token with CDN read and update only) on the App Platform component, make sure the Spaces key can list and delete, and redeploy. Until then nobody can open `/admin`.
