Decided by the card-image-cdn plan:
- spaces.Client.List(ctx, prefix, token) -> ListPage{Keys, Next} (ListObjectsV2,
  paged). admin-cache-clearing must reuse it.
- spaces.Config.PublicBase(): https://<bucket>.<region>.cdn.digitaloceanspaces.com,
  or <endpoint>/<bucket> for endpoints that are not DigitalOcean's.
- The fake gains paged listing, unsigned public reads, a 403 for private or
  missing objects, and Public(key).
- Image prefix images/<TCGdex asset path>/{low,high}.webp: public-read,
  image/webp, 7-day Cache-Control. tcgdex/ stays private.
- The breaker may move to internal/cards/breaker.go.
- hostedCardDB returns images *cards.ImageCache; newHosted takes it.
- /api/card gains image_fallback; webui.Server gains Images ImageSource.
- HUMAN task: enable the CDN on the bucket (after the spaces human task).
- Manual checks: the 95% CDN metric; real CDN end to end; tcgdex/ stays
  private; fallback with the CDN blocked; no shared store; a restart keeps
  copies; iOS Safari.
