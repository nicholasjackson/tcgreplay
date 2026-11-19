---
created_date: "2026-11-12"
document_status: final
closed_date: "2026-11-12"
epic: 20261111093348-e1b5c2b4-public-launch
---

# Feature: 20261112083238-29247b36-privacy-terms-disclaimer

## Overview

Before TCG Replay goes public and is submitted to ChatGPT, it needs the pages a public service and the ChatGPT review expect: a privacy page that stays truthful once visitor counting and diagnostics are added, terms of service, and a clear place to ask for help. Players meet the playmat in two places, the public website (home, docs, privacy, terms) and the board (the signed-in page where their games are shown), and in both they should see that it is an unofficial fan project with no connection to Nintendo or The Pokémon Company, and where its card data and images come from.

## Requirements

- [x] **Privacy page describes visitor counting**
  The privacy page explains the cookieless visitor counting on the website and the board: what it records (page, referrer, country, device), that it sets no cookies, and that it is never linked to a player's account. The promise that no analytics are collected is gone.
- [x] **Privacy page describes diagnostics**
  The privacy page explains the diagnostics the playmat keeps (the hosting platform's request and performance metrics, and the playmat's own error and usage records): what they contain, what they never contain, and how long they are kept.
- [x] **Privacy page stays accurate and dated**
  Every other statement on the privacy page still matches how the playmat works, and the page shows the date it was last updated.
- [x] **Terms of service are published**
  A terms of service page is published on the website, and every website page links to both the privacy page and the terms.
- [x] **Terms cover the essentials**
  The terms say who runs the playmat (see Constraints); that it is free and provided as is, with no guarantee it stays available; that games can be lost at any time; that the hosted limits apply (requests per minute, the number of players served at once, and idle games being dropped); that abuse is not allowed (overloading or attacking the service, automated scraping, getting round the limits, using it for anything unlawful); that players must be at least 13; that the terms and the service can change or end; where to get help; and they link to the privacy page and show the date they were last updated.
- [x] **Terms cover trademarks and card images**
  The terms have a section saying the playmat is an unofficial fan project, not affiliated with, endorsed, sponsored or approved by Nintendo, The Pokémon Company, Creatures or Game Freak, that Pokémon and its trademarks are theirs, and that card data and images come from TCGdex and malie.io.
- [x] **The board carries the notice and credit**
  The board shows the fan-project notice and the card-data credit where a player can find them, at every screen size, without covering cards or controls.
- [x] **Help is one click away**
  Every website page, the privacy page, the terms and the board link to the project's help channel (see Constraints).

## Constraints

- The terms name "the tcgreplay project" as running the playmat, not an individual (the maintainer's decision).
- Help is through the project's GitHub issues only; there is no support email (the maintainer's decision).
- The privacy page must describe everything the visitor-counting work (Cloudflare Web Analytics) and the diagnostics work (DigitalOcean's metrics and the playmat's own error and usage records) collect, using the retention periods those pieces of work set; nothing they collect may go undescribed.
- The privacy page, the terms and the help link must open at stable public addresses on tcgreplay.app without signing in, as the ChatGPT submission requires.

## Acceptance Criteria

- [x] **Visitor counting on the privacy page**
  The privacy page no longer says analytics are never collected, and has a section naming the visitor-counting service that lists page, referrer, country and device as what it records and says it sets no cookies and isn't linked to accounts.
- [x] **Diagnostics on the privacy page**
  The privacy page has a section on diagnostics naming the hosting platform's metrics and the playmat's own error and usage records, listing what they contain and what they never contain, and saying how long each is kept.
- [x] **Existing statements re-checked**
  Each statement already on the privacy page (sign-in data, cookies, browser settings, games, retention, logs, outside services, removing data) has been checked against the playmat's behaviour at release, and any that differed is corrected.
- [ ] **Privacy page dated**
  The privacy page's last-updated date is the date the updated page went live.
- [x] **Terms page live and linked**
  The terms page loads on the website without signing in, and the footer of every website page links to both the privacy page and the terms.
- [x] **Terms content**
  The terms page names the tcgreplay project as running the playmat and states each of: free and as is with no availability guarantee; games may be lost at any time; the hosted limits (requests per minute, players served at once, idle games dropped); the disallowed uses listed in the requirements; minimum age 13; terms and service may change or end; help through GitHub issues; a link to the privacy page; a last-updated date.
- [x] **Trademarks section**
  The terms page has a trademarks and card images section containing the non-affiliation notice naming Nintendo, The Pokémon Company, Creatures and Game Freak, and the TCGdex and malie.io credit.
- [x] **Notice on the board**
  On the board, at phone (390 px), tablet (820 px) and desktop (1440 px) widths, a player can reach the fan-project notice and the card-data credit, and showing them covers no card or control.
- [x] **Help links**
  The website footer, the privacy page, the terms page and the board each contain a link to the project's GitHub issues.

## Technical Approach

- The website footer's existing two-sentence notice is already listing-sized: prefer making it the one wording reused by the board, the terms and the ChatGPT listing, so they can't drift apart.
- On the board, prefer putting the notice and credit somewhere reached on demand (a menu or about panel) over a permanent strip, to keep the phone layout uncluttered.
- Keep the privacy page's and terms' figures (idle time, limits) drawn from the playmat's own configuration, as the privacy page already does, rather than typed in.
- Model the terms on what other community tools publish (for example Limitless) rather than seeking a bespoke legal text.

## Success Metrics

- The ChatGPT submission accepts the privacy, terms and support links, and its review raises no objection about missing or inaccurate policy pages.

## Non-Goals

- Building the visitor counting or the diagnostics: the visitor-analytics and observability specs of the public-launch epic.
- Writing the ChatGPT listing itself: the ChatGPT submission prep spec of the same epic.
- A lawyer's review of the terms or privacy page.
- Choosing a governing law for the terms.

