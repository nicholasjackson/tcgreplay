---
title: Rewind and try another line
description: Rewind an imported game to any turn and play out a different line on its own branch.
order: 60
---

Any imported game can be rewound to any point and played differently. The playmat keeps every line on its own **branch**, so trying something new never loses the original game, and you can switch between lines on the board's timeline.

## Naming turns

The playmat counts turns the way the battle log does: **turn N is each player's Nth turn**. "Turn 3" means the first player's third turn and the second player's third turn, so say whose turn you mean: "my third turn" or "turn 3, my opponent's turn".

## Try another line

Ask your assistant to take the board back to a turn. It rewinds onto a new branch and shows you the position, and you can play the new line yourself on the board or ask it to play one out:

```prompt
Take me back to my third turn and show me the line where I bench a second attacker first.
```

```prompt
Go back to the start of my opponent's fourth turn. What could they have done instead?
```

## Test it against different hands

The opponent's hidden cards come from their [likely deck](/docs/workflows/opponent-deck). To check a line holds up rather than relying on one guess, the assistant can deal their hidden hand several times, each on its own branch:

```prompt
Try that line against a few different opponent hands. Does it still work?
```

## Compare and keep

Ask to compare the boards at the end of each line, or switch between branches on the timeline yourself. The branches last for the session; to keep the game itself, ask for its record (see [the hosted playmat's limits](/docs/limits)).
