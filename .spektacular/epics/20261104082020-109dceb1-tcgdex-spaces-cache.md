---
created_date: "2026-11-04"
document_status: draft
spec: 20261104082020-109dceb1-tcgdex-spaces-cache
specs:
    - name: 20261104082020-109dceb1-tcgdex-spaces-cache
      depends_on:
        - 20261104090640-51bd1f36-card-cache-freshness
    - name: 20261104090640-51bd1f36-card-cache-freshness
      depends_on: []
    - name: 20261104090640-25f673ed-card-image-cdn
      depends_on:
        - 20261104082020-109dceb1-tcgdex-spaces-cache
    - name: 20261104090640-a5a74fab-admin-cache-clearing
      depends_on:
        - 20261104082020-109dceb1-tcgdex-spaces-cache
        - 20261104090640-25f673ed-card-image-cdn
---

# Epic: 20261104082020-109dceb1-tcgdex-spaces-cache

## Overview

The hosted playmat keeps its card database (card text, set lists and card images) in a shared store that lasts across restarts and deploys, instead of losing it each time and fetching everything again. Cached data refreshes on its own at intervals suited to how often each kind changes, so new sets, new promos and newly added artwork show up without anyone clearing the cache, and an admin can clear it at once to pick up a correction, and the playmat keeps serving its last good copy if the card source is down. Players get faster imports and boards that never freeze while the cache builds. Desktop users keep working offline as they do today.

## Specs

| # | Spec | Scope |
|---|---|---|
| 1 | 20261104082020-109dceb1-tcgdex-spaces-cache | The hosted card cache lives in DigitalOcean Spaces and survives restarts. |
| 2 | 20261104090640-51bd1f36-card-cache-freshness | Cached card data refreshes on its own, never freezes the board, and keeps working offline. |
| 3 | 20261104090640-25f673ed-card-image-cdn | The hosted board loads card images from our own copy on the Spaces CDN. |
| 4 | 20261104090640-a5a74fab-admin-cache-clearing | Admins clear the hosted card cache, all of it or one set, from a page. |
