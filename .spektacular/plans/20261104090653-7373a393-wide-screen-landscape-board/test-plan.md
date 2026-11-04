---
created_date: "2026-11-04"
document_status: final
closed_date: "2026-11-04"
---

# Test plan: 20261104090653-7373a393-wide-screen-landscape-board

The automated suite (`cd web && npm run build && npx playwright test`, with phone, tablet, desktop and the new 2560×1440 `wide` profile) covers every acceptance criterion and the "every existing board e2e check passes" success metric. What follows needs a person.

## Setup (for every procedure)

Build and run this branch's board, without touching the installed daemon:

```
cd <checkout of branch spek/20261104090653-7373a393-wide-screen-landscape-board>
make dev            # builds the UI into the Go package and runs the dev daemon
```

The playmat is at http://127.0.0.1:7778/. Open a real game: import a battle log through the `tcgreplay-dev` MCP server (`import_log`), or start one with `new_game`. A mid-game position works best, with a full-ish bench, Energy attached and some prizes taken.

## 1. Success metric: the whole game fits a 3840-wide monitor without browser zoom

- **What to measure**: on the player's own 3840-wide monitor, the board, the timeline beside it and the header are all on screen at 100% browser zoom, with no scroll bars and nothing cut off apart from the opponent's peeking hand.
- **How**: open the playmat in a maximised browser window on the 3840-wide monitor, with browser zoom reset (Ctrl+0). Keep the timeline beside the board (its dock button shows "Move timeline below the board"). Check that:
  - the board is the landscape mat: both Actives side by side in the middle row;
  - there is no scroll bar on the page or the board;
  - every bench slot, pile, prize fan, the Stadium, the turn controls and your whole hand are visible.

  Repeat with the timeline moved below the board, then hidden (☰ → Hide timeline).
- **Expected result**: all three timeline positions fit. Benched cards are clearly larger than before: at least 1.6× today's width, so about 191px or more at 2560×1440 and proportionally more at 3840. The header and timeline text are clearly larger than at 1920px and comfortable to read.
- **Who / when**: the user (Nic), on their own monitor, before merging the branch.

## 2. Manual review: the landscape mat against mockup C

- **What the reviewer looks at**: the landscape mat at 2560×1440 or larger, against artboard "C · Landscape mat" in the "Large-screen board layouts" canvas (https://claude.ai/artifact/HK5VjHeETLarQsFE5d5p5n) and the design `wide-board/design.md`.
- **Where**: http://127.0.0.1:7778/ in a window whose board area is at least 1600px wide.
- **What to look for**:
  - **Arrangement**:
    - The opponent's row: piles at the left, bench in the middle, prize fan at the right.
    - The middle row: Stadium, your Active, "vs", the opponent's Active, then the turn pill, coin and buttons.
    - Your row: prize fan at the left, bench, piles at the right.
    - Your hand along the bottom, with "N in hand" beside it and your name centred below.
  - **Peek**: the opponent's hand hangs off the top edge with about a third of each card back showing, enough to count, and "N in hand" sits beside it. Hovering a face-up opponent card (for example with Reveal all on) enlarges it downward into view.
  - **Labels**:
    - The opponent's name and status are small in the top-left corner, with no bar across the board.
    - Each Active is edged in its seat's colour, with a caption ("<name>'s Active") on its top edge.
    - Pile and seat labels are readable.
  - **Readability**: card names, HP badges and damage counters read at a glance. The Active cards are slightly larger than the bench. Prize and pile cards are smaller but countable.
  - **Busy board**: with Energy and Tools under several benched Pokémon, or a bench of 8 with Area Zero Underdepths, the board still fits. Cards shrink a little rather than scroll.
  - **Switching**: drag the window narrower than about 2000px (timeline beside) and back. The board switches to today's mat and back at once, with no reload and nothing lost.
- **Passing**: the arrangement matches mockup C and the design. Nothing overlaps unreadably, nothing is cut off apart from the peek, and the switch is clean.
- **Who / when**: the user, before merging the branch.

## 3. Manual review: a replay on the landscape mat with the playback bar over the board

- **What the reviewer looks at**: the playback bar and move caption during a replay on the landscape mat.
- **Where**: http://127.0.0.1:7778/ at 2560 wide or more. In the timeline, press a line's ▶ ("Play from here"), let a few moves play, then pause and step with ◀ / ▶|.
- **What to look for**: the bar and caption are horizontally centred over the board, not the window, with the timeline beside, below and hidden. Where they cover cards, usually the opponent's bench row, the covered cards are still identifiable, or the pause and step controls make it easy to see the move. The caption text is readable and wraps sensibly. Live ⏭ returns to the latest position.
- **Passing**: the bar and caption read well over the board in all three timeline positions, and nothing important stays hidden behind them while you step through a replay.
- **Who / when**: the user, before merging the branch.
