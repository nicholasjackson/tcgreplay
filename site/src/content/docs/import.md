---
title: Importing a game
description: Import a Pokémon TCG Live battle log with your decklist, check the import is complete, and keep the game.
order: 40
---

To review a game, your assistant needs two things from Pokémon TCG Live: the game's **battle log** and the **decklist** you played. Paste both into the chat and ask it to import the game.

## Where to copy them in TCG Live

- **The battle log.** After a game, open it from your match history and copy its game log. It starts with `Setup` and ends with the result, such as "… wins.".
- **The decklist.** Open the deck you played in the deck editor and export it; TCG Live copies the list as text, starting with `Pokémon:` and ending with `Total Cards: 60`.

The exact menu names can differ between TCG Live versions; look for the game log in your match history and the export option in the deck editor.

## What to paste

Paste the whole battle log and your decklist in one message, then ask:

```prompt
Here's my last game from TCG Live and my decklist. Import it and review it with me.
```

If you have no game to hand, use [the example game](/docs/example).

## What a complete import looks like

The assistant reports the game: who went first, the result and how it ended, the turns played, and **no problems**, meaning every line of the log was replayed. It gives you a link to the board, where you can step through every turn.

If it reports problems, a few are usually harmless: it says which log lines couldn't be replayed and what that affects. Many problems mean the import isn't reliable; check the log was complete and the decklist is the one you played.

## If lines are missing

Some chat apps shorten a long paste, showing something like "141 lines hidden" and sending only part of the log. A shortened log imports badly: the game stops early or the assistant reports problems.

- Paste the whole log again, and check the chat shows it in full.
- Or save the log to a text file and attach the file instead of pasting.
- Check the log starts with `Setup` and ends with the result line.

## Keeping the game

Imported games don't last forever on the hosted playmat (see [its limits](/docs/limits)). To keep one, ask:

```prompt
Give me this game's record so I can keep it.
```

Save the record somewhere safe, such as a note. Pasting it back later and asking to import it restores the same game.
