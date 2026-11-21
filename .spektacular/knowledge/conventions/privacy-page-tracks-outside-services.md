---
tags: [privacy, site, webui, fonts]
---

# Update the privacy page with any new outside resource or remembered setting

The privacy page (`site/src/pages/privacy.astro`) makes two promises that ordinary feature work can quietly break:

- **The only thing the website loads from another site is Cloudflare's visitor-counting script.** Adding any outside resource makes that untrue: web fonts (Google Fonts, for example), a CDN script or stylesheet, an embed, or remote images beyond the card images.
- **It names the display settings the board remembers in the browser** (replay speed, autoplay, where the panel sits, prize reminders). A new setting, such as a theme choice, must be added to that list.

Rule: a change that adds an outside resource to the board or the website, or a new browser-remembered setting, must update the privacy page in the same change. The page's cookie wording must still say whose cookies they are (see the `__cf_bm` gotcha).

Surfaced while specifying the Studio redesign. IBM Plex loads from Google Fonts and the board gains interface and mat theme settings, and the privacy page promised neither.
