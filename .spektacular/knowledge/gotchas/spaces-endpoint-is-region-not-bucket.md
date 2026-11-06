---
tags: [spaces, hosting, cdn, images, cache]
---

# The Spaces endpoint is the region address, not the bucket's "Origin Endpoint"

`TCGREPLAY_SPACES_ENDPOINT` must be the region endpoint,
`https://<region>.digitaloceanspaces.com` (e.g. `https://ams3.digitaloceanspaces.com`),
with the bucket named only in `TCGREPLAY_SPACES_BUCKET`.

The DigitalOcean control panel shows each bucket's own address,
`https://<bucket>.<region>.digitaloceanspaces.com`, labelled "Origin Endpoint". It
is the natural thing to paste, and it is wrong here.

Why it is easy to miss:

- `spaces.FromEnv` / `Config.normalized()` (`internal/spaces/config.go`) only check
  that the endpoint is an http(s) URL with a host and no path, so the bucket form
  passes and start-up still logs `card cache: ... backed by shared store
  https://<bucket>.<region>.digitaloceanspaces.com/<bucket>`. No settings warning
  appears.
- The client addresses objects path-style (`<endpoint>/<bucket>/<key>`), so
  against the bucket host the bucket name becomes the first segment of every key:
  card data lands under `<bucket>/tcgdex/...` inside the bucket, and the start-up
  image listing fails with `card images: shared store unavailable (list images/:
  spaces: 404 NoSuchKey: ); using the card source, retrying in 1m0s`. It reads
  like an outage, not a bad setting.
- The signing region is the endpoint's first host label, so it becomes the bucket
  name instead of the region.
- `Config.PublicBase()` recognises only `<region>.digitaloceanspaces.com`, so the
  CDN address is not derived and image copies would be served from
  `<endpoint>/<bucket>` instead of `https://<bucket>.<region>.cdn.digitaloceanspaces.com`.

How to tell it is right: the start-up log shows `backed by shared store
https://<region>.digitaloceanspaces.com/<bucket>` and `served from
https://<bucket>.<region>.cdn.digitaloceanspaces.com/images/`, followed by
`card images: N copies found in the shared store` with no `shared store
unavailable` line. If the bucket form was ever deployed, delete the stray
`<bucket>/` folder it left in the bucket.
