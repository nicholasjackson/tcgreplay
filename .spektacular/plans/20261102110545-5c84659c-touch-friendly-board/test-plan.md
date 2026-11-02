---
created_date: "2026-11-02"
document_status: final
closed_date: "2026-11-02"
---

# Test plan: touch-friendly board

The browser suite (`make test`, 203 tests over phone, tablet and desktop profiles) covers every behavioural success metric. It runs the touch profiles in **Chromium's touch emulation**, not WebKit, because Playwright's WebKit does not launch on the development machine (see the changelog for the first task). Everything below needs a real iPad and iPhone in Safari, or a person's judgement.

## Getting the board onto a real device

The board server only answers on loopback (knowledge `gotchas/board-loopback-locks.md`), so a phone on the LAN cannot open `make dev`'s board. Use one of:

- **The hosted playmat.** It is reachable from any device and is the only way to check Sign out.
  1. `export TCGREPLAY_GOOGLE_CLIENT_ID=… TCGREPLAY_GOOGLE_CLIENT_SECRET=…`
  2. `make docker-build && make docker-run PUBLIC_URL=https://<host the device can reach>`
  3. Or use the deployed playmat once this branch ships.
- **The iOS Simulator on a Mac.** The Simulator shares the Mac's loopback.
  1. Run `make dev` on the Mac.
  2. Open `http://127.0.0.1:7778/` in Safari on a simulated iPhone 13 (portrait) and iPad (10th gen, landscape).

  This is WebKit, but it is not a real touchscreen: do the gesture checks (M3) on hardware.

For a game to look at, ask the assistant to start a new play-mode game (or import a battle log), then open the board link it gives.

## Success metrics

### S1. A full play-mode game on an iPad by touch alone

- **Measure:** a whole game from setup to a winner on an iPad, with no keyboard, trackpad or mouse, and no step that needs one.
- **How:**
  1. On the hosted playmat (or the Simulator), start a play-mode game against the assistant. Play it to the end using only touch:
     - mulligan or start;
     - set the Active and bench;
     - attach Energy (tap the Energy, Attach, tap a Pokémon);
     - evolve, play Trainers by dragging them to the play area, and drag a Stadium onto the Stadium;
     - attack through the attack panel, changing the target at least once;
     - set damage with the stepper, toggle a Special Condition, and retreat;
     - take prizes, Draw N and Look at top N, end turns, and play on through the assistant's turns.
  2. Note any moment that needed a long press, a hover, a right-click or a browser dialog, or where a control was hard to hit.
- **Pass:** the game finishes with every action done by tap or drag, nothing needs another input, and no browser dialog appears.
- **Who / when:** the product owner, on a real iPad, before release.

### S2. Following and acting in a game on an iPhone in portrait

- **Measure:** a game followed and played on an iPhone held upright, with no pinch zoom and no sideways page scrolling.
- **How:**
  1. Open the same kind of game on an iPhone in portrait. Over at least three of your own turns:
     - drag a card from the hand to the bench;
     - attach by tapping;
     - attack;
     - open each counter chip: Deck, Discard, Lost, Prizes, and the opponent's;
     - scroll the bench row;
     - open ☰ and use the branch selector, Review mode, Reveal all, Deck list and Show timeline.
  2. Turn the phone sideways and back.
- **Pass:** no pinch zoom and no sideways page scroll is ever needed, and every action works. The sideways notice appears, and turning back shows the same board.
- **Who / when:** the product owner, on a real iPhone, before release.

## Manual reviews

### M1. Containers and phone board against the mockups (frames A–H)

- **Where:** the design mockups `mobile-board/mockups.html` in the `design` source (`spektacular design read --data '{"source":"design","path":"mobile-board/mockups.html"}'`), side by side with the board.
- **What to compare:**
  - **A, the phone board:** counter bars, the scrolling bench with its "+n ›" hint, the compact middle strip and the hand strip.
  - **B and E, the phone sheet:** for the Active and for a hand card. Check the header card, "Card text ›", the groups as rows, the edge fade on an overflowing row, the stepper and the toggle chips.
  - **C, the target banner:** banner, glowing candidates and the faded card.
  - **D, the attack panel:** damage stepper with ×2, target row with Change ›, and the pinned confirm button.
  - **F, the tablet panel:** card on the left, Card | Text switch, wrapping rows.
  - **G, the desktop popover:** right-click a Pokémon.
  - **H, touch dragging:** the ghost raised above the finger, zones outlined, the zone under the finger lit.
- **Pass:** each container matches its frame's structure, grouping, order and wording closely enough for the owner to accept it. Colour and spacing may differ where the board's light theme or existing tokens apply.
- **Who / when:** the product owner, before release.

### M2. No system callout or text selection on a long touch

- **Where:** a real iPhone and a real iPad in Safari.
- **What:** touch and hold for two seconds on:
  - a card in hand;
  - a Pokémon in play;
  - an attached Energy;
  - a card in the discard view.
- **Pass:** no text is selected, no callout, preview or "Save image" menu opens, and nothing else happens. Long press is not a gesture.
- **Who / when:** the product owner, on hardware. This can't be checked in emulation, because Chromium ignores `-webkit-touch-callout`.

### M3. Scroll versus drag on real iOS

- **Where:** a real iPhone in portrait.
- **What:**
  1. Make the hand overflow (draw several cards), then swipe it sideways.
  2. Drag a hand card upwards onto the bench.
  3. With a full bench (Area Zero Underdepths in play), swipe the bench row sideways, then drag a benched Pokémon up onto the Active Spot.
  4. In the discard view, scroll the list up and down, then drag a card out by starting the move sideways.
- **Pass:**
  - sideways swipes scroll the hand and the bench without lifting a card;
  - an upward drag lifts the card and drops it where released;
  - the discard list scrolls vertically, and a sideways start lifts the card.
  - If iOS fights the gesture (a swipe lifts a card, or a drag scrolls), record it; do not change the gesture model (no long press) without the owner's decision.
- **Who / when:** the product owner, on hardware, before release.

### M4. Sign out on the hosted playmat from a phone

- **Where:** the hosted playmat on an iPhone in portrait, signed in with Google.
- **What:** open ☰ and tap Sign out.
- **Pass:** the account name shows in ☰, Sign out returns to the sign-in page, and reloading does not sign back in on its own. The automated suite can't cover this, because the test server is the desktop board, which has no accounts.
- **Who / when:** the product owner, against the hosted playmat, before release.
