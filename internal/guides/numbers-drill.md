---
name: numbers-drill
description: Use when the player asks for a numbers drill, damage maths practice, or a quick quiz on the cards in the open game or the meta's threats.
---

# Numbers drill

Knockouts are decided by numbers the player should know without calculating: their attackers' damage with each boost, the HP of the threats they face, and how Weakness and Resistance apply. This drill asks short questions built from real cards and checks every answer against card text.

## Build the questions

- Use cards from the game open on the playmat: the player's deck (`get_profile` for their seat) and the opponent's (`get_profile`). If no game is open, ask the player to import one (a battle log, or a game record they saved) and stop. Prefer cards that decided the game: attackers that took knockouts, Tools that changed HP, Stadiums that changed numbers.
- Read every card with `card_text` first. Never ask or answer from memory.
- Three kinds of question, mixed:
  - **Recall:** "Mega Lucario ex's Mega Brave with Maximum Belt into a Pokémon ex?" One number.
  - **Threshold:** "What is the least damage that knocks out N's Zoroark ex holding Hero's Cape?" One number.
  - **Swing:** a short scenario with modifiers, Weakness and Resistance: "Aura Jab plus one Premium Power Pro into a Fighting-weak Basic with 120 HP: knockout?" Yes or no, then the number.

## Damage order

Check answers in this order, and show it when the player gets one wrong:

1. Base damage of the attack (including its own scaling).
2. Add the attacker's boosts (Tools, Items, Supporters, Abilities that add damage).
3. Weakness doubles the total (or adds, as the card says).
4. Resistance subtracts; then effects that reduce damage taken.
5. Compare with remaining HP: printed HP plus HP Tools minus Stadium effects, minus damage already on it.

Effects that ignore Weakness and Resistance, or apply only to Pokémon ex, or only to the Active Pokémon, are the usual traps; ask about them deliberately.

## Session shape

Five to ten questions, a few seconds each. Ask one at a time, mark it, and move on; explain only the misses. End with the score and the one card or rule missed most, and suggest repeating those next time.
