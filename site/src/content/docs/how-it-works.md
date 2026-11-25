---
title: How it works
description: Why tcgreplay works with an everyday AI assistant, with no model trained to play Pokémon.
order: 15
---

tcgreplay doesn't teach an AI to play the Pokémon TCG. It gives the assistant you already use, such as ChatGPT or Claude, a virtual playmat it can see and use. The assistant brings what it's already good at: reading, following instructions and reasoning through a turn. The playmat brings everything else.

## The playmat holds the game

Every card in both decks lives on the playmat: hands, decks, benches, prizes, discard piles, damage, Energy, Special Conditions and whose turn it is. The assistant never has to remember the board. It asks the playmat for the current position and gets an exact answer, with every card named by its own ID, so it can't lose track of a card halfway through a long game.

## It reads the real card text

The playmat carries the card data for each set. When the assistant needs to know what a card does, it looks up the printed text, the way you'd pick a card up and read it, rather than relying on what it remembers. When a new set comes out, it works as soon as the playmat's card data includes it. The assistant itself doesn't change.

## The assistant judges the rules, the playmat keeps it honest

Like a playmat on a table, tcgreplay isn't a rules engine. It checks that a card really is where the assistant says it is, so it can't attach Energy from a hand that doesn't hold any. It flags likely mistakes as warnings but never blocks a move. The assistant decides what's legal by reading the card text, as two players at a table would. That's also why new mechanics don't need new code.

It can still misread a ruling now and then, just as a player can. When that happens, tell it, and it can undo the move.

## Hidden cards stay hidden

When you play a game against the assistant, it sees only what its own seat would see: its hand and the public board. It can't look at your hand or the order of your deck. The playmat enforces this, so you don't have to trust the assistant to behave.

## Written guides teach each coaching task

Each kind of session has a short written guide the assistant reads before it starts: importing a battle log, building the opponent's likely deck, reviewing a game, running drills and playing a game. A guide reads like the instructions you'd give a new coach, such as "read the log first, build the opponent's 60, then summarise the game". Improving the coaching means editing a guide, not training a model. The [coaching workflows](/docs/workflows) page lists them all.

## You watch it on the board

Everything the assistant does appears on the board in your browser, live. You can follow along, make your own moves, and, for an imported game, step through the timeline, [rewind to any turn](/docs/rewind) and try another line on its own branch.

## Why that matters

- **It works with the assistant you already have.** There's no special model to wait for or pay for.
- **New sets and new coaching need no retraining.** New card data and an edited guide are enough.
- **The board is always right.** The assistant may get a decision wrong, but it never loses track of the game.
