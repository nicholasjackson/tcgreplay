### Approach
The playmat decides each image's address.
- **Image cache:** a new `cards.ImageCache` keeps an in-memory list of images with a public copy under `images/` in Spaces, filled by listing the bucket at start-up.
- **Card endpoint:** `/api/card` returns the CDN address with the TCGdex address in `image_fallback`. An image not yet copied gets the TCGdex address and a background copy.
- **Board:** each image tries `image`, then `image_fallback`, then the text stand-in.
### Milestones and tasks
- M1, the board shows an image from a second address: the image fallback on the card endpoint; the board fallback with a Playwright test.
- M2, public copies: list the bucket and derive its public address; copy images in the background; find existing copies at start-up and count image answers.
- M3, hosted images from the CDN: hosted wiring; operator documentation; knowledge entries; enable the CDN (human).
### Tasks a person must do
- Enable the CDN on the Spaces bucket and check the access key can list and write public objects.
### Out of scope
- Resizing images, and any image source other than TCGdex.
- Clearing and purging (admin-cache-clearing).
- The CDN on desktop.
- Pushing new addresses to boards already open.
- A custom CDN domain.
- Bulk-copying images no board has shown.
### Drafting assumptions
- The CDN address is derived as `https://<bucket>.<region>.cdn.digitaloceanspaces.com`, with no new setting.
- Both `low.webp` and `high.webp` are copied, with a 7-day Cache-Control.
- The copier runs 2 at a time, with a queue of 256.
- An hourly log line counts CDN and TCGdex answers.
### Project-wide rules
- Decides `spaces.Client.List(ctx, prefix, token) -> ListPage{Keys, Next}` and `Config.PublicBase()`.
- Decides the image prefix `images/<asset path>/{low,high}.webp`, public-read, 7-day cache; `tcgdex/` stays private.
- The fake gains listing, unsigned public reads and `Public(key)`.
- `/api/card` gains `image_fallback`, and `webui.Server` gains `Images`.
### Manual checks
- The 95% CDN metric over a week.
- Real CDN end to end.
- `tcgdex/` objects stay private.
- Images fall back with the CDN blocked.
- No shared store: images load from TCGdex as today.
- A restart keeps the copies.
- Real iOS Safari.
