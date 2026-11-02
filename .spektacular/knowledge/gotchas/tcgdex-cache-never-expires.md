---
tags: [cards, tcgdex, cache, images, hosting]
---

# The TCGdex cache never expires

`internal/cards` caches every TCGdex response on disk the first time it is
fetched and never re-fetches it: `sets.json` (the set list), `sets/<id>.json`
(each set's card list, which feeds the by-name index and its "has art"
preference), `set-index.json` (set codes to set ids) and `cards/<id>.json` (each
print). `getJSON` returns the cached file whenever it exists; there is no
max-age, ETag or refresh.

Why it bites: TCGdex keeps changing after the first fetch. It adds new sets,
adds images to prints it first listed without one (fresh releases and promos),
and corrects text. A cache built before then keeps the old answer, so:

- a set released after `sets.json` was cached is unknown to name lookup and
  set-code resolution;
- a print first cached without an image keeps showing the board's text
  stand-in after TCGdex adds the art;
- by-name resolution keeps preferring whichever print had art at index time.

Where it bites: the desktop and dev builds keep the cache in
`cards.DefaultCacheDir()` (`~/.cache/tcgreplay`), so it lasts until someone
deletes it. The hosted image has no volume for `/home/nonroot`, so its cache
starts empty on each redeploy, but goes stale over a long-running instance.

To get fresh data, delete the cache directory (or the affected files) and
restart. A lasting fix needs an expiry: at least for `sets.json` and the set
files, and for any card cached without an image.

Card images are not cached by the playmat at all. The board loads them straight
from `assets.tcgdex.net` (`<image>/low.webp`), so only the viewer's browser
caches them. `/api/card` is served `no-cache`, so the board revalidates it
every load.
