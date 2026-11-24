---
created_date: "2026-11-20"
document_status: draft
spec: 20261120140830-52277437-studio-redesign
specs:
    - 20261120140830-52277437-studio-redesign
---

# Studio: the look of the board and the website

The visual design for the tcgreplay board on every screen size and for the website, chosen from the "TCG Replay · Studio design" canvas (https://claude.ai/artifact/5esddCvKjxHT9WiFw9u7MJ). Artboards are named here as they're titled there.

Studio changes how things look, not how they behave. Two earlier designs still apply:
- **mobile-board/design.md** sets interaction: gestures, dragging, target mode, the menu model and its three containers, and the phone layout.
- **wide-board/design.md** sets the landscape mat for board areas 1600px wide or more.

Studio restyles both and adds the pieces listed below.

## The look

- **A light interface around a dark mat.** The header, timeline and menus use light panels. The game is played on a dark surface, so the cards stand out.
- **IBM Plex Sans for text, IBM Plex Mono for numbers.** Numbers include HP, damage, counts, step numbers, turn tags and branch names.
- **A new mark replaces `logo.png`:** a navy rounded square holding two cards and a replay arrow, in navy `#13285C`, cyan `#5CC8F2` and white. It's used in the board header, on the website, as the favicon and app icon, and in the plugin's assets. It contains no Pokémon imagery.
- **One set of colour tokens** is shared by the board (`web/`) and the website (`site/`). Today `site/` keeps a hand-copied `tokens.css`; that copy goes away.

### Colour roles

| Role | Light interface | Dark interface |
|---|---|---|
| Page background | `#F6F7F9` | `#0F1217` |
| Panel | `#FFFFFF` | `#171B22` |
| Line / control border | `#E2E5EA` / `#D5DAE1` | `#262C36` / `#333A46` |
| Text / muted text | `#14171C` / `#5B6370` | `#E6E9EE` / `#9AA3B2` |
| Primary (buttons, selected) | navy `#13285C`, white text | cyan `#5CC8F2`, `#04121F` text |
| Current timeline step | `#E0F2FE` | `#12324A` |
| Note | `#F1F5F9` on `#DCE3EC` | `#1E2530` on `#2E3745` |
| Warning | `#FFF7EA` on `#F3D19C`, text `#8A4B00` | `#2A2010` on `#5A4520`, text `#FFCB6B` |
| Danger (Knock Out) | `#B42318` | `#FF8A80` |

| Mat | Surface | Zone | Strip | Chip | Text / muted |
|---|---|---|---|---|---|
| Slate | `#252A33` | `#2D333D` | `#1E232B` | `#323945` | `#E6E9EE` / `#A7AFBC` |
| Felt | `#1D4430` | `#24513A` | `#163626` | `#2C5A43` | `#EEF3EF` / `#B4CBBD` |
| Navy | `#14244A` | `#1B2F5C` | `#0F1D3D` | `#22386A` | `#EAF1FF` / `#A9B9DA` |

These stay the same in every theme:
- **Player colours:** blue for p1 (`#60A5FA` on the mat, `#2563EB` on light panels) and orange for p2 (`#FB923C` and `#EA580C`).
- **End turn button:** `#38BDF8`.
- **HP bar fill:** `#4ADE80`.
- **Damage tag:** `#FFB4B4` on `#5B1E22`.
- **Energy dots:** the Energy type's colour.

## Themes

- **Two settings:** interface (light or dark) and mat (slate, felt or navy). The default is light with slate.
- **Set in the board's ☰ menu**, never the header. The browser remembers them alongside the board's other display settings, and they never leave the browser. The privacy page's list of display settings the board remembers must name them.
- **Only colour changes between themes.** Layout, sizes and components are identical, so a theme is a set of token values and nothing else.

## The board's parts

- **Header:** the mark, then the game name, branch picker, Play/Review switch, Reveal all, warnings, the connection dot and ☰.
- **Player block (one per side):**
  - name with a player-colour dot;
  - prize pips: six slots, filled for prizes still to take, with the count beside them;
  - pile chips: Hand, Deck, Discard and Lost, each with its count and opening that pile;
  - a thumbnail of the top card of the discard pile.
- **Bench:** a zone panel. Under each card, its HP left in mono (pink when damaged) and its Energy dots. An empty slot is a recessed box.
- **Active Pokémon:** a panel outlined in the player's colour, holding the card and a readout: name, HP left over maximum in large mono, an HP bar, the damage tag (for example −60), and the Energy dots with their count. A Special Condition is named as a chip in the readout's line, after the damage tag: Poisoned and Burned in their condition colours with white text, Asleep, Confused and Paralyzed in the mat's chip colour. The card itself is never turned for a condition: an upside-down card read as a fault.
- **Turn strip:**
  - the Stadium on the left: a single card, a Legendary Stadium (below), or an empty recessed box labelled "Stadium";
  - in the middle, the turn tag ("T5"), "Alice's turn" and the step;
  - on the right, Flip coin, End action, and End turn in the accent colour.
- **Legendary Stadium** (a Stadium played as a pair, such as Delta Reign's Legendary Stadiums). Its two halves stand in the Stadium spot as one card, as in TCG Live (mockup: `studio/legendary-stadium.html`):

  ```
  ┌─────────────┐
  │ ▭ upper  ▭ │  ← each half turned a quarter
  ├╌╌╌╌╌╌╌╌╌╌╌╌╌┤  ← hairline seam
  │ ▭ lower  ▭ │
  └─────────────┘
  ```

  - The halves sit in print order, joined edge to edge. They share one card outline and shadow, with a hairline seam between them, so the pair reads as one card. Only the lower half carries the basis mark.
  - The pair takes exactly the space of a single Stadium in every layout (desktop, landscape mat, docked timeline, tablet, phone). The strip and every other card keep their size whether a single Stadium, a pair or nothing is in play.
  - Each half is still its own card to tap, drag or zoom, and its menu acts on that half. The popover header's mono summary reads "Stadium · played as a pair".
  - The zoom view shows both halves side by side as one picture, with "Played as a pair:" and both card ids in mono below it.
- **Hand:** a row of cards, each labelled with its kind (Item, Supporter, Energy, Pokémon), and the count.
- **Timeline:**
  - **Controls:** previous, play and next; the step ("#217 / 247"); speed; Go live.
  - **Scrubber:** coloured by whose turn each stretch was.
  - **Turn headings:** turn tag, player dot and name, with Rewind and Play buttons for that player's half of the turn.
  - **Every line:** ▶ (play from here) and ↺ (rewind to just after this line), always visible rather than only on hover; then the event number and text.
  - **State:** the current step is highlighted and steps ahead are dimmed. Coaching notes sit in a note box under their line.

## Screen sizes

- **Desktop:** the mat with the timeline beside it (380px).
- **Board area 1600px or wider:** the landscape mat from the wide-board design, built from Studio's parts.
- **Narrow PC, with the timeline docked below:**
  - the board on top and the timeline below, about a third of the height, with a drag handle to resize;
  - one control row across the timeline: playback, scrubber, step, speed, Go live, dock;
  - a turn list down the timeline's left side: one row per turn, a rewind button for each player's half, and the prize score after that turn (for example "5–4").
- **Tablet and touch PC:** the desktop mat with every control at least 44px. The timeline is narrower (about 310px), and a header button hides it.
- **Phone:** the mobile-board layout, restyled:
  - counter bars of chips for Hand, Deck, Discard, Lost and Prizes;
  - Active Pokémon as panels with the readout;
  - the strip with Stadium, coin and End turn;
  - a hand that scrolls sideways with a "+n" hint;
  - the timeline as a full-screen sheet, with the same per-line and per-turn controls at 44px.

## Menus

The menu model, its sections and its three containers come from the mobile-board design. Studio styles them as follows.

- **Mouse popover** (264px, opens where you right-click):
  - a header with the name and a mono summary ("Active · 280/340 HP · 2 Energy");
  - small uppercase section labels and 34px rows;
  - attacks with Energy-cost dots, damage and a chevron; target actions marked with a target icon;
  - an inline damage control (−, the value, +, +30, +50) and toggle chips for conditions;
  - Other after a divider, with Knock Out last in red.
- **Card in hand, mouse popover:**
  - the main play action as a full-width button, with a hint such as "Goes to the first free Bench spot (4 of 5 taken)";
  - Move as a 2×2 grid (Discard, Lost Zone, Deck top, Deck bottom);
  - Reveal to opponent, then Card text.
- **Touch panel:**
  - centred over the dimmed board;
  - on the left, the card at 260px with a Card/Text switch;
  - on the right, the sections, with 44px controls that wrap, 52px attack buttons, and Knock Out in the danger style.
- **Attack panel** (inside the touch panel or the phone sheet):
  - ‹ Actions to go back;
  - the attack's name, cost, damage and its text from the card data;
  - a damage control starting at the printed damage, with ×2 Weakness and −30 Resistance toggles;
  - the target with a Change button;
  - when the damage is at least the target's HP left, the line "130 is enough to Knock Out Kadabra". This is arithmetic only, and nothing is enforced;
  - one primary button: "Attack Kadabra for 130".
- **Phone sheet:**
  - full screen, with the card at 110px beside the name, HP and Card text;
  - one row per section, with the conditions row scrolling sideways behind a fade;
  - the attack panel stacked to fit.
- **Not drawn yet:** target mode, the deck menu, and the Draw N and Look at top N panels. They follow the mobile-board design using these same parts.

## Website

The same tokens, light interface only.

- **Header:**
  - the mark and "TCG Replay", which links home;
  - links: Guides, Example, What's new, with the current page underlined;
  - Get started as a button.

  Home and Privacy leave the top bar; Privacy is in the footer.
- **Footer:** dark, with Docs and Project link columns, then the fan-project notice and the card-data credit, both taken from `internal/notice/notice.json`.
- **Home:**
  - **Hero:** a "Works with Claude and ChatGPT" badge, the headline and description, Get started and See an example game buttons, and the existing animated board scene (its self-drawn cards, log and coach note), redrawn in Studio colours, as the picture. It stays lightweight and keeps its own markup rather than embedding the real board.
  - **How it works:** three steps.
  - **What it does:** the three feature descriptions on a slate band.
  - **Ask in your own words:** four guide cards, each with a real example prompt.
  - **Address block:** the connector address with Copy.
  - Footer.
- **Docs layout:**
  - the sidebar, a breadcrumb, and an article up to 720px wide;
  - an "On this page" list on wide screens;
  - building blocks: an address copy block, a prompt bubble with a copy button, numbered steps, an amber callout, and a dark code block for terminal commands.
- **Coaching workflows:** cards grouped into After a game (Import a game, The opponent's likely deck, Review a game), Practice (Replay drill, Before you move, Numbers drill) and Play (Play a game). Each card shows the guide's name and an example prompt.
- **What's new:** entries on a timeline. An entry that needs a refresh gets an amber callout linking to the refresh steps. Each change sits on its own row.
- **Example game:** the two steps, the prompt, then the decklist (light) and the battle log (dark) side by side, each with a Copy button.
- **Privacy and Terms:** a text column with an "On this page" card and the last-updated date in mono.
- **404:** a small stack-of-cards picture, then Go to the home page and All docs.

Page content keeps coming from today's sources: the site facts, `notice.json`, `whats-new.yaml` and the docs content. The nav changes and the grouped guides mean the site's browser tests that check them must change.

## Not part of this design

- A dark theme for the website.
- Any change to how the board behaves, apart from the Knock Out hint, which only adds a line of text.
- How card images are drawn. The mockups use colour blocks; the board keeps showing the real card images.
