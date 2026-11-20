---
created_date: "2026-11-08"
document_status: draft
spec: 20261108105504-9abfa10e-card-set-publishing
specs:
    - name: 20261108105504-9abfa10e-card-set-publishing
      depends_on:
        - 20261108110847-d27a9f77-card-set-import
    - name: 20261108110847-d27a9f77-card-set-import
      depends_on: []
    - name: 20261108110847-055548e3-paired-stadiums
      depends_on:
        - 20261108110847-d27a9f77-card-set-import
        - 20261108105504-9abfa10e-card-set-publishing
---

# Epic: 20261108105504-9abfa10e-card-set-publishing

## Overview

Card sets for the playmat will be built ahead of time instead of being pulled on demand while people play. The maintainer runs an import that does the mechanical work. An assistant then checks each card against its printed image, and the reviewed result is kept with the project and published to the playmat's online storage. Players get complete, correct card text and art for new sets, promos and illustration rares as soon as they are published, with no new release of the playmat. The epic also adds the paired Legendary Stadiums introduced in Delta Reign, which are two different cards played together as one Stadium.

## Specs

| # | Spec | Scope |
|---|---|---|
| 1 | 20261108105504-9abfa10e-card-set-publishing | Publish built sets to the bucket and make the playmat read card data only from there, retiring on-demand card data. |
| 2 | 20261108110847-d27a9f77-card-set-import | Build and verify card sets offline into the repository, with corrections and an agent verification skill. |
| 3 | 20261108110847-055548e3-paired-stadiums | Play Stadiums marked in card data as pairs, such as Delta Reign's Legendary Stadiums, on the board, by the assistant and in imported logs. |
