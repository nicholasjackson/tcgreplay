Decided by the freshness plan (other plans must build on these):
- Store contract (internal/cards): Get(ctx,key)(Entry,error), returning
  ErrNotCached when absent; Put(ctx,key,Entry); Entry{Data []byte; Fetched
  time.Time}. Keys are `sets`, `sets/<id>` and `cards/<id>` (raw ids).
  Fetched is kept as is when an entry is copied between tiers.
- On-disk envelope {"fetched": RFC 3339, "data": <body>}. Legacy files fall
  back to their modification time.
- Freshness lives above the store: tiers hold no expiry rules, and offline
  mode never contacts any source.
- There is no persisted set index; the catalog is derived in memory.
- The DB interface is unchanged.
- Nothing under the game lock calls the network.
- Changelog: default implement workflow.
- Knowledge to be rewritten during implementation:
  gotchas/tcgdex-cache-never-expires.md, gotchas/tcg-live-log-card-codes.md.
- Manual checks: the 95%/200ms week metric; an upgrade check against a real
  cache; live TCGdex checks (new set on first import, late art after reload).
- Knowledge written: gotchas/board-card-info-per-page-load.md.
