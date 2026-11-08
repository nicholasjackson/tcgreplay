---
tags: [webui, cards, cache, images, cdn, spaces, admin]
---

# The board fetches each card's data once per page load

The board front end asks the server for a card's definition (`/api/card?def=…`)
the first time it needs it, and never again in that page.
`cardInfo` in `web/src/lib/store.svelte.js` keeps every answer in `app.cards`, and
keeps every requested def in a module-level `requested` set, including the ones
whose fetch failed.

Consequences:

- A change to a card on the server (a publish picked up, borrowed Energy art)
  reaches an open board only after the page reloads, and only for a game started
  or reopened after the pickup: `/api/card` answers through the open game's own
  catalogue snapshot, so an open game keeps its cards until it ends. The request
  is sent with `cache: 'no-cache'` and the server answers `Cache-Control:
  no-cache`, so a reload always gets the server's current copy.
- A card whose fetch failed (not in the game's catalogue) stays on its text
  stand-in for the rest of that page, even after the server could answer it.
  A reload retries it.
- The image list in a card's answer is fixed for the page too. On the hosted
  playmat with the shared store, an image copied to the bucket while the page
  is open is loaded from the CDN only after a reload; until then the board
  keeps the TCGdex address it was given. Each image spot (card, zoom, preview)
  tries the card's candidate addresses in order once each (`imageCandidates`,
  `imageSrc`, `nextImageStage` in `store.svelte.js`): for a TCGdex entry its
  `image` with the size file, then its `image_fallback` (TCGdex, present only
  when `image` is a CDN copy), for a file entry (`image_file`) its address as is,
  then the next entry; after the last, the text stand-in or Energy token.
- A CDN copy's address carries a content version: `/api/card` sends
  `image_version` (a 12-hex hash of the copy's two objects' ETags), and
  the board loads `<image>/<size>?v=<image_version>`, never adding it to the
  TCGdex fallback. Copies are served with `Cache-Control: public,
  max-age=604800`, so browsers and the CDN keep them a week without asking;
  the version is what makes changed art (re-copied after an admin clears the
  image copies) reach them, because its address is new. Unchanged art keeps its
  version, across restarts too, and so its caches.
- After an admin clears image copies (`/admin`), an open board keeps the image
  addresses it already loaded; it shows the fresh art after a reload.

So "the board shows the new data" means "on the next load of the board, in a
game started after the change". Pushing card updates to boards already open
would need a change to the board and its update protocol.
