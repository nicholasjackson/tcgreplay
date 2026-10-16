---
name: tcg-opponent-deck
description: Build a likely 60-card list for the opponent in an imported Pokémon TCG Live game and apply it to the tcgreplay playmat, so both decks are complete and the opponent's unseen prizes, hand and deck are dealt at random from it. Use right after import_log (or when importing, to pass opp_deck), when the reveal view shows unknown "?" cards, or when asked to guess, build or assume the opponent's deck.
---

# Build the opponent's likely deck

A battle log only proves part of the opponent's list. The playmat needs a full 60 so that rewound positions and sampled hands are realistic. Code handles the constraints and the dealing; you supply the judgement: which archetype this is and what the rest of the list probably was.

## 1. Gather the evidence

- `report.observed` from `import_log`, or `profile.observed` from `get_profile`: the **minimum** count of each card the log proves (the most copies seen at once). These are hard floors.
- The log itself: what the opponent attacked with, evolved into, searched for and played. Mulligan reveals show whole hands.
- The opponent's name and the date, for the format.

## 2. Name the archetype

Identify it from signature cards: the main attacker and its evolution line, and distinctive engines (Dragapult ex with Budew and Munkidori; Alakazam with Dudunsparce; N's Zoroark ex). Generic staples (Buddy-Buddy Poffin, Boss's Orders, Poké Pad) say little. State the archetype and how sure you are. If the player has pasted a reference list for it, start from that.

## 3. Write the list

Start from a typical current list for the archetype, then make it consistent with the evidence:

- **Floors:** every observed card at least at its observed count.
- **Ceilings:** at most 4 of any card by name, except Basic Energy; at most one ACE SPEC in the deck.
- **Lines:** enough Basics for the evolutions (a 2-2 Stage 2 line needs Rare Candy or a Stage 1); energy types that pay for the attackers seen.
- **Exactly 60 cards.**

Fill the slots the log does not prove using how decks are actually built:

- **Duplicates are the norm.** A card seen once is usually one of several copies. Draw Supporters (Lillie's Determination, Iono-style shuffles), search Items (Ultra Ball, Buddy-Buddy Poffin, Poké Pad, Nest Ball-style searches) and the main attacker's Basics run 3-4. Raise a staple seen once to its usual count rather than leaving it at 1.
- **True one-ofs are a short list:** the ACE SPEC, tech Pokémon with an Ability (Fezandipiti ex), a second Stadium, a situational Supporter (Boss's Orders often 2, Crispin and Rosa's Encouragement 1-2).
- **Check the totals against the norm.** Most lists run about 10-14 Supporters (or Supporter-like draw), 6-12 Energy depending on how energy-hungry the attackers are, and 12-20 Pokémon. If your guessed list falls outside that, adjust before applying it.
- **Read volume from the log.** How many Supporters were played turn after turn, how many Energy attached, how often the same search Item came up: a deck that played a draw Supporter every turn runs 4 of it.
- **Evolution lines come in shapes:** 4-3-2 or 3-2-2 for a Stage 2 main attacker (fewer middle stages with Rare Candy), 4-4 or 3-3 for a key Stage 1, 2-2 or 1-1 for support lines.
- **Names exactly as the log prints them** ("Basic Psychic Energy", "Lillie's Determination"). Set codes are optional; cards without them get their current print.

Use TCG Live export format:

```
Pokémon: 17
4 Dreepy
...

Trainer: 33
4 Buddy-Buddy Poffin
...

Energy: 10
4 Basic Fire Energy
...
```

## 4. Apply it

- **When importing:** pass the list as `opp_deck` to `import_log`. You can read the log yourself first to choose it.
- **For a game already imported:** `set_opponent_deck(deck)`. It rebuilds the game from the same log and seed, so the player's side is unchanged. If the game has branches it refuses to replace it: pass `new_id` (e.g. `<id>-deck`) to build a separate game instead, and tell the player the new id.
- Read the warnings. "Raised X from 2 to the 3 seen" or "added X, seen but not in the list" mean the list contradicted the log and was corrected. Fix your list if that reveals a misread archetype. A total other than 60 means it needs trimming.

## 5. Report

Tell the player, briefly: the archetype, the list grouped as **proven** (observed floors), **typical** (standard for the archetype) and **guessed** (flex slots), and any corrections. The list is an assumption: in reviews, say "they had" only for proven cards and "they likely had" for the rest.

## Later

To test a line against different hands at one position, use `sample_hidden` after `rewind`: it redeals only the cards the player cannot see at that point, using the stored list.
