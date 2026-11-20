---
tags: [spaces, hosting, cdn, images, publishing]
---

# The Spaces endpoint is the region address, not the bucket's "Origin Endpoint"

`TCGREPLAY_SPACES_ENDPOINT` must be the region endpoint,
`https://<region>.digitaloceanspaces.com` (e.g. `https://ams3.digitaloceanspaces.com`),
with the bucket named only in `TCGREPLAY_SPACES_BUCKET`.

The DigitalOcean control panel shows each bucket's own address,
`https://<bucket>.<region>.digitaloceanspaces.com`, labelled "Origin Endpoint". It
is the natural thing to paste, and it is wrong here.

`spaces.FromEnv` / `Config.normalized()` (`internal/spaces/config.go`) now refuse
an endpoint whose host starts with `<bucket>.` or ends in
`.cdn.digitaloceanspaces.com`, naming the setting and the expected form, so
`sets publish` exits 2 and hosted mode logs a `warning: card images: …` line
instead of failing obscurely. Before that check, the bucket form passed and:

- The client addresses objects path-style (`<endpoint>/<bucket>/<key>`), so
  against the bucket host the bucket name becomes the first segment of every key.
  Every read 404s (`spaces: 404 NoSuchKey`): the start-up image listing reads
  like an outage, and a `sets publish` (first real run, 2026-11-08) failed with
  `list published set 2011bw: spaces: 404 NoSuchKey` after its dry run had
  wrongly reported every set as never published (the manifest GET 404'd the
  same way).
- The signing region is the endpoint's first host label, so it becomes the bucket
  name instead of the region.
- `Config.PublicBase()` recognises only `<region>.digitaloceanspaces.com`, so the
  CDN address is not derived and image copies would be served from
  `<endpoint>/<bucket>` instead of `https://<bucket>.<region>.cdn.digitaloceanspaces.com`.

How to tell it is right: the start-up log shows `card images: copied from the
card source to the shared store https://<region>.digitaloceanspaces.com/<bucket>,
served from https://<bucket>.<region>.cdn.digitaloceanspaces.com/images/`,
followed by `card images: N copies found in the shared store` with no `shared
store unavailable` line. If the bucket form was ever deployed, delete the stray
`<bucket>/` folder it left in the bucket.
