---
created_date: "2026-11-02"
document_status: draft
specs:
    - 20261102110545-5c84659c-touch-friendly-board
---

# Touch-friendly board

The board must be fully usable by touch on an iPad and an iPhone. Mouse use on the desktop stays as it is, except where a shared change makes it better (grouped menus, target mode). One build serves every device. The mockups that go with this design are in `mobile-board/mockups.html` in this source; frames are referred to by letter.

## Why touch fails today

- Cards move by HTML5 drag and drop (`draggable`, `ondragstart`, `ondrop`), which a touch does not start in Safari on iOS or iPadOS.
- Menus open on `contextmenu` (right-click). iOS fires nothing equivalent on a long press.
- Cards enlarge on `:hover`, and pile previews use `mouseenter`.
- Attacks, "Set damage…", "Draw N…" and "Look at top N…" ask through `window.prompt`.
- The mat is about 10 card widths wide and `--cw` floors at 48px, so an iPhone (390px wide) overflows.

## Principles

1. **One input path.** Pointer Events handle mouse, pen and touch. What a gesture does depends on the event's `pointerType`, not on the device, so an iPad with a trackpad works both ways at once.
2. **One menu model, three containers.** The menu builders in `web/src/lib/actions.js` describe actions as sections. The container (phone sheet, tablet panel or desktop popover) decides how to show them.
3. **Choose a target on the board, not from a list.** An action that needs a destination becomes one button followed by a tap on the board.
4. **No `window.prompt`.** Numbers are entered in the sheet with steppers.
5. **The playmat stays a playmat.** Target mode highlights the same candidates today's menus list. Nothing new enforces rules.

## Gestures

| Gesture | Touch | Mouse |
|---|---|---|
| Tap or click a face-up card | opens the action sheet (B, E, F) | zooms, as today |
| Right-click a card | — | opens the action popover (G) |
| Drag a card | moves it (H) | moves it, using the same code |
| Tap or click a pile | opens the pile view | the same |
| Tap or click the deck | opens the deck's actions | the same |

- The sheet replaces the Zoom view for touch. On the desktop, Zoom stays for a left click.
- A card with no actions (the opponent's hand, a face-down card) opens nothing on a tap.
- The opponent's in-play Pokémon get what `stackMenu` gives them today: damage, conditions, Knock Out and the card itself.
- Hover enlargement applies only under `@media (hover: hover)`.
- Long press is not used. Cards set `-webkit-touch-callout: none` and `user-select: none`, so iOS shows neither its callout nor a text selection.
- Tap targets are at least 44×44px under `@media (pointer: coarse)`.

## Dragging (frame H)

- A drag starts once the pointer has moved 8px from where it went down, and the pointer is captured. A tap that moves less is a tap.
- The hand scrolls sideways on narrow screens. Inside it, a mostly horizontal move scrolls and a mostly upward move starts a drag (`touch-action: pan-x` on the hand). On the mat, cards are `touch-action: none`.
- While dragging:
  - a ghost of the card follows the pointer, raised half a card above it so the finger doesn't hide it;
  - every drop zone is outlined;
  - the zone under the pointer is lit.
- Drop zones carry a `data-drop` attribute with today's drop spec (`p1:active`, `p1:bench:2`, `discard`, `deck`, `lost`, `stadium`, `play`, `hand`), found with `document.elementFromPoint`.
- The payload stays today's `{kind, iid, def, from, role}`, so what each drop does is unchanged.
- Letting go outside a zone, or pressing Escape, cancels.
- This replaces `startDrag`, `endDrag` and `readDrag`, and every `ondragover` and `ondrop` handler. The mouse uses the same path, so there is one implementation.

## Target mode (frame C)

**Used by:** Attach (Energy and Tools), Evolve, Retreat, Switch, moving an attached card, and choosing an attack's target. Bench still takes the first free slot (`FreeBenchSlot`), so it needs no target.

1. The sheet closes.
2. A banner across the top names the action and the card, for example "Attach Fighting Energy · Tap one of your Pokémon", with Cancel.
3. The candidates glow in the accent used by `app.highlight`, and everything else dims. The card being played is faded where it lies.
4. Tapping a candidate runs the action and ends target mode. Tapping anything else, Cancel or Escape ends it with nothing done.

**Candidates** are exactly the targets today's menus list:
- `ownStacks` for Attach, Evolve and Move;
- the occupied bench slots for Retreat and Switch;
- the opponent's Active and bench for an attack.

**One candidate:** the button runs at once, and its label names the target ("Retreat to Riolu").

Target mode works the same way with a mouse.

## Menu model

A builder returns:

```js
{
  title: 'Mega Lucario ex',
  card,                                   // shown in the sheet or panel, with its text
  summary: 'Active · HP 340 · 60 damage · 2 Energy',
  sections: [
    { label: 'Attack', items: [{ label: 'Aura Jab', detail: '130', panel: attackPanel }] },
    { label: 'Move', items: [{ label: 'Retreat', target: { prompt, candidates, run } }] },
    { label: 'Damage', stepper: { value: 60, step: 10, quick: [30, 50], set } },
    { label: 'Conditions', toggles: [{ label: 'Asleep', on: false, set }] },
    { label: 'Other', items: [{ label: 'Knock Out', danger: true, run }] },
  ],
}
```

- An item has exactly one of:
  - `run`, which acts at once;
  - `target`, which enters target mode;
  - `panel`, which opens a sub-panel inside the sheet.
- `primary` marks the common action, and `danger` marks a destructive one.
- A section with nothing in it is left out.
- The "Zoom" items go, because every container shows the card.

### Sections for each menu

These replace today's flat lists.

**A Pokémon in play** (`stackMenu`; frames B and F):
- **Attack** (your Active): one item per attack, with the printed damage as its detail, each opening the attack panel. "Attack…" when the card has no attacks.
- **Ability:** one item per Ability, which runs at once.
- **Move:**
  - your Active: Retreat (target: a benched Pokémon; the Energy discarded for the Retreat Cost is chosen as it is today) and Switch, no cost (target);
  - your bench: Switch into Active, or Promote to Active when the Active spot is empty.
- **Damage:** a stepper with −10 and +10 around the value and quick +30 and +50. Tapping the value opens the number pad to type a total, which replaces "Set damage…".
- **Conditions** (Active only): five toggle chips.
- **Other:** Return {top card} to hand, Discard {top card} (devolve), then Knock Out last, marked `danger`.

**A card in your hand** (`handMenu`; frame E):
- **Play:**
  - a Pokémon: Set as Active (when the Active spot is empty), Bench, and Evolve (target);
  - a Tool: Attach (target);
  - any other Trainer: Play;
  - an Energy: Attach (target).

  The first of these is `primary`.
- **Move:** Discard, Deck top, Deck bottom, Lost Zone.
- **Other:** Reveal to opponent.

**An attached card** (`attachmentMenu`):
- **Move:** Move to (target), To hand, Discard.

**The deck** (`deckMenu`):
- **Draw:** Draw 1; Draw N (panel).
- **Look:** Look at top N (panel); Search.
- **Other:** Deck list, Shuffle.

**A card in a pile view** (`pileCardActions`): stays a row of buttons on the card inside `Modal.svelte`, with the buttons growing to 44px under a coarse pointer.

### Panels (frame D)

**The attack panel** replaces both prompts in today's attack flow. It shows:
- the attack's name and text;
- a damage stepper starting at the printed damage, with ×2 for Weakness. The player adjusts it for Weakness and Resistance, as the prompt asks today.
- a target, defaulting to the opponent's Active. *Change ›* enters target mode over the opponent's side and then returns to the panel.
- one primary button that spells out the result: "Attack Kadabra for 130".

‹ Actions returns to the sections.

**Draw N and Look at top N** each have a panel with a stepper (from 1 up to the deck's size) and Go.

## Containers

Each opening chooses its container from the event's `pointerType` and the viewport's width.

**Phone sheet** (touch, viewport narrower than 640px; frames B, D, E):
- It covers the whole screen.
- Header:
  - the card at about 30% of the width;
  - the name and summary;
  - *Card text ›*, which swaps the sections for the card's full text while it is open;
  - ✕.
- Body: one row per section, with its label above it. A row's buttons sit on a single line that scrolls sideways when it overflows, with a fade at the edge to show there's more. Steppers and toggles fill their row.
- Rows stack downwards, and the sheet only scrolls if they run past the screen.
- It closes with ✕, a swipe down on the header, or the browser's back (the sheet pushes a history entry).

**Tablet panel** (touch, 640px or wider; frame F):
- A centred dialog up to 900px wide and 90vh tall over the dimmed board.
- Left: the card at a large size, with a Card | Text switch under it.
- Right: the sections, whose rows wrap instead of scrolling sideways. The right column scrolls on its own when a menu is long.
- Tapping the backdrop or ✕ closes it.

**Desktop popover** (mouse; frame G):
- It opens at the pointer, as `ContextMenu.svelte` does today: 264px wide, scrolling past 80vh.
- Section labels are small headers, items are a list, and the stepper and chips sit inline.
- It has no card image, because the card is on screen and a click zooms it.

One renderer serves all three. It takes the menu model and a container mode, so the sections, items, stepper and toggles are written once.

## Board layout

**Tablet** (640px or wider): today's mat. At iPad sizes `--cw` already gives cards of 58px or more. Changes:
- hover enlargement is off without hover;
- the header wraps, as it does today;
- the timeline can still sit beside or below the board.

**Phone** (narrower than 640px, portrait; frame A): a compact mat.
- A counter bar on each side (Deck, Discard, Lost and Prizes as chips) replaces the piles column and the prize grid. Tapping a chip opens that pile, the deck's actions or the prize view.
- The opponent's hand shows as a count in their counter bar.
- The Active is centred.
- The bench is one row that scrolls sideways when it holds more than fit (about 5 at 56px; 8 with Area Zero Underdepths), with a "+n ›" hint at the edge.
- The middle strip keeps the Stadium, the turn, the coin and End turn. Draw and End action appear there when they apply.
- Your hand is a strip along the bottom with larger cards (72px), scrolling sideways.
- The header keeps only the connection dot, the warnings and ☰. The game name, the branch, the Review mode and Reveal all toggles, and the timeline move into ☰. The timeline opens as a full-screen sheet.
- Card size is set from the width: `--cw: calc((100vw - 32px) / 5.6)`.

**Phone landscape** shows a notice to turn the phone upright. A 390px-tall screen cannot fit the mat.

## Testing

The repo has no browser tests today. This design adds Playwright tests in `web/` using its iPhone and iPad device profiles (`hasTouch`), covering:
- a tap opening the sheet;
- Attach through target mode;
- dragging from the hand to the bench;
- the damage stepper;
- the attack panel;
- the desktop popover with a mouse.

They run against the dev build with a fixed game record.

## Out of scope

- Rules enforcement for target candidates.
- A native app, or installing the board as a web app.
- Long-press gestures.
- A landscape phone layout.
