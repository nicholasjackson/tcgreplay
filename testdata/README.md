# TCG Live replay test data

10 full Pokémon TCG Live battle logs (verbatim, UTF-8, as exported from the game) played by NicJackson on 10–11 Oct 2026 with Mega Lucario ex v12, plus a JSON index and the deck list.

## Layout

- `logs/*.txt` – raw logs, unedited. File name: `<date>_<opponent>_<archetype>.txt`.
- `decks/mega-lucario-ex-v12.txt` – PTCGL import/export format (60 cards).
- `decks/mega-lucario-ex-v12.json` – same list with set/number, stage, HP, type, weakness, kind, ACE SPEC flag, key attacks/abilities.
- `index.json` – one entry per game.

## index.json (schema_version 1)

Per game:

- `file`, `bytes`, `lines`, `sha256` – integrity checks against the raw log.
- `player`, `opponent`, `player_deck`, `opponent_archetype`, `family`.
- `expected` – assertion-ready oracle for a parser: `coin_toss_winner`, `went_first`, `turns` (per player), `total_turns` (combined turns: the higher per-player count), `prizes_taken` (per player), `winner`, `end_reason` (`all_prizes_taken` | `opponent_conceded` | `self_conceded` | `bench_out`). Prize counts are cross-checked against a hand-kept game ledger.
- `features` – log constructs present in that game (mulligan, weakness/resistance lines, poison checkup, devolve, borrowed attacks, simultaneous KO, etc.). Use to pick fixtures per parser feature.
- `parser_quirks` – known oddities in that specific log.
- `borrowed_attacks`, `ledger_result`, `note` – context.

Top level `corpus_parser_notes` lists quirks common to every log.

## Quirks worth a test each

1. Damage-counter lines can name the acting player as owner of the target's Pokémon.
2. Same-name switches ("X's Mega Lucario ex was switched with X's Mega Lucario ex"); copies are not uniquely identified.
3. Curly (U+2019) and straight apostrophes are mixed.
4. Opponent draws are hidden ("drew 3 cards"); own draws are itemised.
5. Logs can end mid-turn on a concession.
6. Mega Lopunny's Gale Thrust prints the +170 breakdown line twice; Resentful Refrain has no base-damage line; resistance appears as a double negative.
